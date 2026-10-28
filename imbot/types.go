// Package imbot sends one-way notifications to generic webhooks, DingTalk and Feishu.
// It has no callback or provider-specific configuration dependency.
package imbot

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	KindGeneric  = "generic"
	KindDingTalk = "dingtalk"
	KindFeishu   = "feishu"
)

// Level is an optional notification severity supplied by the caller.
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

// Message is the provider-neutral one-way notification.
type Message struct {
	Title string
	Text  string
	Link  string
	Level Level
}

// Provider sends a notification. Implementations do not expose callbacks.
type Provider interface {
	Send(context.Context, Message) error
}

// Options configures a provider without depending on an application config package.
type Options struct {
	Secret       string
	Keyword      string
	AllowedHosts []string
	AllowHTTP    bool
	Timeout      time.Duration
	Client       *http.Client
	MaxTextRunes int
}

func (o Options) withDefaults() Options {
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	if o.MaxTextRunes <= 0 {
		o.MaxTextRunes = DefaultMaxTextRunes
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	return o
}

// NewProvider validates target and creates the requested one-way provider.
func NewProvider(kind, target string, opts Options) (Provider, error) {
	kind = NormalizeKind(kind)
	if !ValidKind(kind) {
		return nil, fmt.Errorf("unsupported IM bot kind %q", kind)
	}
	opts = opts.withDefaults()
	if err := ValidateWebhookURL(target, opts.AllowedHosts, opts.AllowHTTP); err != nil {
		return nil, err
	}
	return &webhookProvider{kind: kind, target: target, opts: opts}, nil
}

// ValidateWebhookURL rejects untrusted destinations and private address ranges.
func ValidateWebhookURL(raw string, allowedHosts []string, allowHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.User != nil {
		return fmt.Errorf("webhook url userinfo is not allowed")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && !(scheme == "http" && allowHTTP) {
		return fmt.Errorf("webhook url scheme %q not allowed (https required)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook url has no host")
	}
	if !hostAllowed(host, allowedHosts) {
		return fmt.Errorf("webhook host %q not in allowlist", host)
	}
	ips, err := resolveHostIPs(host)
	if err != nil {
		return fmt.Errorf("webhook host %q resolve: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("webhook host %q resolved to no addresses", host)
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("webhook host %q resolves to non-public address %s", host, ip)
		}
	}
	return nil
}

func ValidKind(kind string) bool {
	switch NormalizeKind(kind) {
	case KindGeneric, KindDingTalk, KindFeishu:
		return true
	default:
		return false
	}
}

func NormalizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return KindGeneric
	}
	return kind
}

// DeliveryError categorizes failures so callers can decide whether to alert or retry.
type DeliveryError struct {
	Kind       string
	Retryable  bool
	StatusCode int
	Err        error
}

func (e *DeliveryError) Error() string {
	if e.Err == nil {
		return e.Kind
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// ProviderError is a business rejection returned in a successful HTTP response.
type ProviderError struct {
	Endpoint  string
	Code      int64
	Message   string
	Permanent bool
}

func (e *ProviderError) Error() string {
	kind := "retryable"
	if e.Permanent {
		kind = "permanent"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = "no error description"
	}
	return fmt.Sprintf("%s rejected message: code %d: %s (%s)", e.Endpoint, e.Code, msg, kind)
}
