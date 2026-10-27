package imbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const (
	EventHeader     = "X-Gofer-Event"
	SignatureHeader = "X-Gofer-Signature"
)

type webhookProvider struct {
	kind   string
	target string
	opts   Options
}

func (p *webhookProvider) Send(ctx context.Context, message Message) error {
	ctx, cancel := context.WithTimeout(ctx, p.opts.Timeout)
	defer cancel()
	body, err := RenderMessageWithLimit(p.kind, message, p.opts.MaxTextRunes)
	if err != nil {
		return &DeliveryError{Kind: "render", Err: err}
	}
	target, body, err := ApplyProviderAuth(p.kind, p.target, body, p.opts.Secret, now())
	if err != nil {
		return &DeliveryError{Kind: "auth", Err: err}
	}
	// Revalidate on every send so a DNS change cannot turn an allowlisted name
	// into a private destination after provider construction.
	if err := ValidateWebhookURL(target, p.opts.AllowedHosts, p.opts.AllowHTTP); err != nil {
		return &DeliveryError{Kind: "validation", Err: err}
	}
	attempts := 2
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		last = p.post(ctx, target, body)
		if last == nil {
			return nil
		}
		var de *DeliveryError
		if !asDeliveryError(last, &de) || !de.Retryable || attempt+1 == attempts {
			return last
		}
	}
	return last
}

var now = func() time.Time { return time.Now() }

func (p *webhookProvider) post(ctx context.Context, target string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return &DeliveryError{Kind: "transport", Retryable: false, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if p.kind != KindGeneric {
		req.Header.Set(EventHeader, p.kind)
	}
	if sig := Sign(body, p.opts.Secret); sig != "" && p.kind == KindGeneric {
		req.Header.Set(SignatureHeader, sig)
	}
	client := *p.opts.Client
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return &DeliveryError{Kind: "transport", Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &DeliveryError{Kind: "http", StatusCode: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
			Err: fmt.Errorf("webhook POST status %d", resp.StatusCode)}
	}
	if err := CheckProviderResponse(p.kind, respBody); err != nil {
		pe := &DeliveryError{Kind: "provider", Retryable: true, Err: err}
		var providerErr *ProviderError
		if errors.As(err, &providerErr) {
			pe.Retryable = !providerErr.Permanent
		}
		return pe
	}
	return nil
}

func asDeliveryError(err error, dst **DeliveryError) bool {
	return errors.As(err, dst)
}

func hostAllowed(host string, allow []string) bool {
	for _, candidate := range allow {
		if strings.EqualFold(strings.TrimSpace(candidate), host) {
			return true
		}
	}
	return false
}

func resolveHostIPs(host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	ips := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		if ip, ok := netip.AddrFromSlice(addr); ok {
			ips = append(ips, ip.Unmap())
		}
	}
	return ips, nil
}

func isPublicIP(ip netip.Addr) bool {
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

// parseProviderCode accepts a provider code delivered as a JSON number or as a
// numeric string; a non-numeric code is kept as -1 so it counts as a refusal
// (unknown / retryable) instead of decoding as success.
func parseProviderCode(raw json.RawMessage) (int64, bool) {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || value == "null" {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1, true
	}
	return n, true
}
