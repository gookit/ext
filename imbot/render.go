package imbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxTextRunes    = 4096
	DingTalkFeishuMaxBytes = 18000
	truncationMarker       = "…（已截断，完整内容见链接）"
)

// RenderMessage builds the provider payload. Provider-specific signatures are
// applied by Send immediately before the request is made.
// Keyword-mode configuration is applied by NewProvider when a message is sent.
func RenderMessage(kind string, m Message) ([]byte, error) {
	return renderMessageWithLimit(kind, m, "", DefaultMaxTextRunes)
}

func RenderMessageWithLimit(kind string, m Message, maxRunes int) ([]byte, error) {
	return renderMessageWithLimit(kind, m, "", maxRunes)
}

func renderMessageWithLimit(kind string, m Message, keyword string, maxRunes int) ([]byte, error) {
	if maxRunes <= 0 {
		maxRunes = DefaultMaxTextRunes
	}
	kind = NormalizeKind(kind)
	m, keyword = withKeyword(m, keyword, kind != KindGeneric)
	switch kind {
	case KindDingTalk:
		return renderDingTalk(m, maxRunes, keyword)
	case KindFeishu:
		return renderFeishu(m, maxRunes, keyword)
	default:
		return renderGeneric(m)
	}
}

func renderGeneric(m Message) ([]byte, error) {
	out := map[string]any{"title": m.Title, "text": m.Text}
	if m.Link != "" {
		out["link"] = m.Link
	}
	if m.Level != "" {
		out["level"] = m.Level
	}
	return json.Marshal(out)
}

func renderDingTalk(m Message, maxRunes int, keyword string) ([]byte, error) {
	return fitProviderPayload(m, maxRunes, keyword, func(text string) ([]byte, error) {
		var body strings.Builder
		body.WriteString("### ")
		body.WriteString(m.Title)
		body.WriteString("\n\n")
		if text != "" {
			body.WriteString("> ")
			body.WriteString(strings.ReplaceAll(text, "\n", "\n> "))
			body.WriteString("\n\n")
		}
		if m.Link != "" {
			fmt.Fprintf(&body, "[查看详情](%s)", m.Link)
		}
		return json.Marshal(map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": m.Title, "text": body.String()},
		})
	})
}

func renderFeishu(m Message, maxRunes int, keyword string) ([]byte, error) {
	return fitProviderPayload(m, maxRunes, keyword, func(text string) ([]byte, error) {
		var body strings.Builder
		body.WriteString(m.Title)
		if text != "" {
			body.WriteString("\n\n")
			body.WriteString(text)
		}
		if m.Link != "" {
			body.WriteString("\n\n查看详情：")
			body.WriteString(m.Link)
		}
		return json.Marshal(map[string]any{
			"msg_type": "text",
			"content":  map[string]string{"text": body.String()},
		})
	})
}

// withKeyword appends a missing keyword to the text; the renderer keeps it
// outside the truncatable rune budget for keyword-mode bots.
func withKeyword(m Message, keyword string, includeTitle bool) (Message, string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || (includeTitle && strings.Contains(m.Title, keyword)) {
		return m, ""
	}
	if strings.Contains(m.Text, keyword) {
		return m, keyword
	}
	if strings.TrimSpace(m.Text) == "" {
		m.Text = keyword
	} else {
		m.Text = strings.TrimSpace(m.Text) + "\n" + keyword
	}
	return m, keyword
}

func fitProviderPayload(m Message, maxRunes int, keyword string, build func(string) ([]byte, error)) ([]byte, error) {
	text := []rune(strings.TrimSpace(m.Text))
	keywordSuffix := []rune("\n" + keyword)
	if keyword != "" && string(text) == keyword {
		text = nil
	} else if keyword != "" && len(text) >= len(keywordSuffix) && string(text[len(text)-len(keywordSuffix):]) == string(keywordSuffix) {
		text = text[:len(text)-len(keywordSuffix)]
	}
	truncated := len(text) > maxRunes
	if truncated {
		text = text[:maxRunes]
	}
	lo, hi := 0, len(text)
	var best []byte
	for lo <= hi {
		mid := (lo + hi) / 2
		candidateText := string(text[:mid])
		if mid < len(text) || truncated {
			candidateText += truncationMarker
		}
		// Truncation can remove a keyword already present in the source text.
		if keyword != "" && !strings.Contains(candidateText, keyword) {
			candidateText += "\n" + keyword
		}
		candidate, err := build(candidateText)
		if err != nil {
			return nil, err
		}
		if len(candidate) <= DingTalkFeishuMaxBytes {
			best = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best != nil {
		return best, nil
	}
	if keyword != "" {
		// Keep the keyword even when the provider's byte ceiling leaves no room for the original text.
		candidate, err := build(truncationMarker + "\n" + keyword)
		if err != nil {
			return nil, err
		}
		if len(candidate) > DingTalkFeishuMaxBytes {
			return nil, fmt.Errorf("keyword payload exceeds %d bytes", DingTalkFeishuMaxBytes)
		}
		return candidate, nil
	}
	return build("")
}

// ApplyProviderAuth adds the time-sensitive native provider signature.
func ApplyProviderAuth(kind, rawURL string, body []byte, secret string, now time.Time) (string, []byte, error) {
	kind = NormalizeKind(kind)
	if secret == "" || kind == KindGeneric {
		return rawURL, body, nil
	}
	switch kind {
	case KindDingTalk:
		ts := strconv.FormatInt(now.UnixMilli(), 10)
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", nil, fmt.Errorf("dingtalk: invalid url: %w", err)
		}
		q := u.Query()
		q.Set("timestamp", ts)
		q.Set("sign", hmacB64([]byte(secret), ts+"\n"+secret))
		u.RawQuery = q.Encode()
		return u.String(), body, nil
	case KindFeishu:
		ts := strconv.FormatInt(now.Unix(), 10)
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
			if err == nil {
				err = fmt.Errorf("body is not a JSON object")
			}
			return "", nil, fmt.Errorf("feishu: body is not a JSON object: %w", err)
		}
		obj["timestamp"] = ts
		obj["sign"] = hmacB64([]byte(ts+"\n"+secret), "")
		signed, err := json.Marshal(obj)
		if err != nil {
			return "", nil, fmt.Errorf("feishu: re-encode body: %w", err)
		}
		return rawURL, signed, nil
	}
	return rawURL, body, nil
}

func hmacB64(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func Sign(body []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + fmt.Sprintf("%x", mac.Sum(nil))
}

// CheckProviderResponse maps HTTP-200 business refusals to ProviderError.
func CheckProviderResponse(kind string, body []byte) error {
	var env struct {
		ErrCode       json.RawMessage `json:"errcode"`
		ErrMsg        string          `json:"errmsg"`
		Code          json.RawMessage `json:"code"`
		Msg           string          `json:"msg"`
		StatusCode    json.RawMessage `json:"StatusCode"`
		StatusMessage string          `json:"StatusMessage"`
	}
	kind = NormalizeKind(kind)
	if kind == KindGeneric || json.Unmarshal(body, &env) != nil {
		return nil
	}
	var raw json.RawMessage
	message := ""
	if kind == KindDingTalk {
		raw, message = env.ErrCode, env.ErrMsg
	} else {
		raw, message = env.Code, env.Msg
		if len(raw) == 0 {
			raw, message = env.StatusCode, env.StatusMessage
		}
	}
	code, present := parseProviderCode(raw)
	if !present || code == 0 {
		return nil
	}
	return &ProviderError{Endpoint: kind, Code: code, Message: message, Permanent: permanentCode(kind, code)}
}

func permanentCode(kind string, code int64) bool {
	switch kind {
	case KindDingTalk:
		return code == 300001 || code == 310000
	case KindFeishu:
		return code == 19001 || code == 19021 || code == 19024
	default:
		return false
	}
}
