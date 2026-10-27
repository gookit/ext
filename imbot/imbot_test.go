package imbot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestRenderMessagePayloads(t *testing.T) {
	message := Message{Title: "构建完成", Text: "全部测试通过", Link: "https://example.test/job/1", Level: LevelInfo}
	t.Run("generic", func(t *testing.T) {
		body, err := RenderMessage(KindGeneric, message)
		assert.NoErr(t, err)
		var got map[string]any
		assert.NoErr(t, json.Unmarshal(body, &got))
		assert.Eq(t, "构建完成", got["title"])
		assert.Eq(t, "https://example.test/job/1", got["link"])
	})
	t.Run("dingtalk", func(t *testing.T) {
		body, err := RenderMessage(KindDingTalk, message)
		assert.NoErr(t, err)
		var got struct {
			MsgType  string `json:"msgtype"`
			Markdown struct {
				Title string `json:"title"`
				Text  string `json:"text"`
			} `json:"markdown"`
		}
		assert.NoErr(t, json.Unmarshal(body, &got))
		assert.Eq(t, "markdown", got.MsgType)
		assert.Eq(t, message.Title, got.Markdown.Title)
		assert.True(t, strings.Contains(got.Markdown.Text, message.Link))
	})
	t.Run("feishu", func(t *testing.T) {
		body, err := RenderMessage(KindFeishu, message)
		assert.NoErr(t, err)
		var got struct {
			MsgType string            `json:"msg_type"`
			Content map[string]string `json:"content"`
		}
		assert.NoErr(t, json.Unmarshal(body, &got))
		assert.Eq(t, "text", got.MsgType)
		assert.True(t, strings.Contains(got.Content["text"], message.Link))
	})
}

func TestProviderAuthAndBusinessErrors(t *testing.T) {
	now := time.Unix(1700000000, 0)
	body := []byte(`{"msg_type":"text","content":{"text":"hi"}}`)
	t.Run("dingtalk signature keeps token", func(t *testing.T) {
		target, signed, err := ApplyProviderAuth(KindDingTalk, "https://oapi.dingtalk.com/robot/send?access_token=tok", body, "secret", now)
		assert.NoErr(t, err)
		assert.Eq(t, string(body), string(signed))
		parsed, err := url.Parse(target)
		assert.NoErr(t, err)
		assert.Eq(t, "tok", parsed.Query().Get("access_token"))
		assert.Eq(t, "1700000000000", parsed.Query().Get("timestamp"))
		assert.True(t, parsed.Query().Get("sign") != "")
	})
	t.Run("feishu signature is in body", func(t *testing.T) {
		_, signed, err := ApplyProviderAuth(KindFeishu, "https://open.feishu.cn/hook/x", body, "secret", now)
		assert.NoErr(t, err)
		var got map[string]any
		assert.NoErr(t, json.Unmarshal(signed, &got))
		assert.Eq(t, "1700000000", got["timestamp"])
		assert.True(t, got["sign"] != nil)
	})
	cases := []struct {
		name      string
		kind      string
		body      string
		permanent bool
	}{
		{"dingtalk keyword", KindDingTalk, `{"errcode":310000,"errmsg":"keywords not in content"}`, true},
		{"dingtalk rate limit", KindDingTalk, `{"errcode":130101,"errmsg":"send too fast"}`, false},
		{"feishu string code", KindFeishu, `{"code":"19024","msg":"Key Words Not Found"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckProviderResponse(tc.kind, []byte(tc.body))
			assert.Err(t, err)
			var providerErr *ProviderError
			assert.True(t, errors.As(err, &providerErr))
			assert.Eq(t, tc.permanent, providerErr.Permanent)
		})
	}
}

func TestValidateWebhookURLRejectsSSRF(t *testing.T) {
	assert.NoErr(t, ValidateWebhookURL("https://203.0.113.10/hook", []string{"203.0.113.10"}, false))
	for _, raw := range []string{"http://127.0.0.1/hook", "https://10.0.0.1/hook", "https://[::1]/hook"} {
		t.Run(raw, func(t *testing.T) {
			assert.Err(t, ValidateWebhookURL(raw, []string{"127.0.0.1", "10.0.0.1", "::1"}, true))
		})
	}
	assert.Err(t, ValidateWebhookURL("https://example.test/hook", nil, false))
}

func TestProviderSendRetriesOnceAndUsesPayload(t *testing.T) {
	var calls atomic.Int32
	var gotBody []byte
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		gotBody, _ = io.ReadAll(r.Body)
		if calls.Load() == 1 {
			return response(http.StatusBadGateway, ""), nil
		}
		return response(http.StatusOK, `{"errcode":0,"errmsg":"ok"}`), nil
	})}
	provider, err := NewProvider(KindDingTalk, "https://203.0.113.10/hook", Options{
		Secret: "secret", AllowedHosts: []string{"203.0.113.10"}, Client: client,
	})
	assert.NoErr(t, err)
	assert.NoErr(t, provider.Send(context.Background(), Message{Title: "title", Text: "text"}))
	assert.Eq(t, int32(2), calls.Load())
	assert.True(t, strings.Contains(string(gotBody), "title"))
}

func TestProviderSendPermanentBusinessErrorDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(http.StatusOK, `{"errcode":310000,"errmsg":"keywords not in content"}`), nil
	})}
	provider, err := NewProvider(KindDingTalk, "https://203.0.113.10/hook", Options{
		AllowedHosts: []string{"203.0.113.10"}, Client: client,
	})
	assert.NoErr(t, err)
	err = provider.Send(context.Background(), Message{Title: "title"})
	assert.Err(t, err)
	assert.Eq(t, int32(1), calls.Load())
}

func TestRenderMessageTruncation(t *testing.T) {
	// gofer's rune-budget and byte-ceiling cases, kept as characterization: a
	// bot rejects a payload that is too long, so the text is shortened and marked.
	t.Run("rune budget", func(t *testing.T) {
		message := Message{Title: "title", Text: strings.Repeat("长", 40)}
		body, err := RenderMessageWithLimit(KindFeishu, message, 12)
		assert.NoErr(t, err)
		assert.Eq(t, 12, strings.Count(string(body), "长"))
		assert.True(t, strings.Contains(string(body), truncationMarker))
	})
	t.Run("byte ceiling", func(t *testing.T) {
		message := Message{Title: "title", Text: strings.Repeat("界", 20000)}
		body, err := RenderMessageWithLimit(KindDingTalk, message, 50000)
		assert.NoErr(t, err)
		assert.True(t, len(body) <= DingTalkFeishuMaxBytes)
		assert.True(t, strings.Contains(string(body), truncationMarker))
	})
}

func TestNewProviderRejectsUnknownKind(t *testing.T) {
	_, err := NewProvider("slack", "https://203.0.113.10/hook", Options{AllowedHosts: []string{"203.0.113.10"}})
	assert.Err(t, err)
}

func TestPostPayloadAndSignaturesWithHTTPTestServer(t *testing.T) {
	message := Message{Title: "title", Text: "text", Link: "https://example.test/1"}
	cases := []struct {
		name   string
		kind   string
		secret string
		check  func(*testing.T, *http.Request, []byte)
	}{
		{
			name: "generic HMAC header", kind: KindGeneric, secret: "secret",
			check: func(t *testing.T, r *http.Request, body []byte) {
				assert.Eq(t, Sign(body, "secret"), r.Header.Get(SignatureHeader))
			},
		},
		{
			name: "dingtalk JSON and query signature", kind: KindDingTalk, secret: "secret",
			check: func(t *testing.T, r *http.Request, body []byte) {
				assert.Eq(t, hmacB64([]byte("secret"), r.URL.Query().Get("timestamp")+"\nsecret"), r.URL.Query().Get("sign"))
				assert.True(t, strings.Contains(string(body), `"msgtype":"markdown"`))
			},
		},
		{
			name: "feishu JSON body signature", kind: KindFeishu, secret: "secret",
			check: func(t *testing.T, _ *http.Request, body []byte) {
				var got map[string]any
				assert.NoErr(t, json.Unmarshal(body, &got))
				assert.True(t, got["timestamp"] != nil)
				assert.True(t, got["sign"] != nil)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoErr(t, err)
				tc.check(t, r, body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			body, err := RenderMessage(tc.kind, message)
			assert.NoErr(t, err)
			target, body, err := ApplyProviderAuth(tc.kind, server.URL, body, tc.secret, time.Unix(1700000000, 0))
			assert.NoErr(t, err)
			provider := &webhookProvider{kind: tc.kind, target: target, opts: Options{Secret: tc.secret, Client: server.Client()}}
			assert.NoErr(t, provider.post(context.Background(), target, body))
		})
	}
}

func TestProviderSendTimeout(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	provider, err := NewProvider(KindGeneric, "https://203.0.113.10/hook", Options{
		AllowedHosts: []string{"203.0.113.10"}, Client: client, Timeout: 10 * time.Millisecond,
	})
	assert.NoErr(t, err)
	assert.Err(t, provider.Send(context.Background(), Message{Title: "title"}))
}
