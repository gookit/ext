// Package desknotify sends best-effort notifications to the user's desktop.
//
// Each platform uses a short child process (PowerShell toast, osascript, or
// notify-send), so desktop notification support has no linked GUI dependency.
// Missing desktop facilities are reported as SkipError rather than making the
// caller's primary operation fail.
package desknotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Notification is one desktop notification. Kind identifies notifications that
// may be coalesced by a Dispatcher; its wording is supplied by the caller.
type Notification struct {
	Title string
	Body  string
	Kind  string
}

// Notifier shows one notification on the local desktop.
type Notifier interface {
	Notify(context.Context, Notification) error
}

// SkipError reports an environment condition that prevents showing a popup.
// Retrying the same event cannot change the condition.
type SkipError struct{ Reason string }

func (e *SkipError) Error() string { return "desktop notification skipped: " + e.Reason }

// Notify sends one notification using the current platform's implementation.
// Desktop notification is best effort; callers may use errors.As to treat a
// SkipError as an unavailable optional channel.
func Notify(ctx context.Context, n Notification) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(n.Title) == "" && strings.TrimSpace(n.Body) == "" {
		return nil
	}
	return NewSystemNotifier().Notify(ctx, n)
}

// DefaultMergeWindow is the coalescing window used by NewDispatcher.
const DefaultMergeWindow = 10 * time.Second

// Dispatcher coalesces notifications of the same Kind. It is safe for
// concurrent use and is useful when several events arrive at once.
type Dispatcher struct {
	notifier Notifier
	window   time.Duration

	mu      sync.Mutex
	pending []Notification
	timer   *time.Timer
	logged  map[string]bool
	closed  bool
}

// NewDispatcher builds a dispatcher around notifier. A non-positive window
// sends each notification immediately.
func NewDispatcher(notifier Notifier, window time.Duration) *Dispatcher {
	if window < 0 {
		window = 0
	}
	return &Dispatcher{notifier: notifier, window: window, logged: make(map[string]bool)}
}

// Push queues n. It uses a background context because the delivery may happen
// later when the merge window closes.
func (d *Dispatcher) Push(n Notification) {
	if d == nil || d.notifier == nil || (strings.TrimSpace(n.Title) == "" && strings.TrimSpace(n.Body) == "") {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.pending = append(d.pending, n)
	if d.window > 0 {
		if d.timer == nil {
			d.timer = time.AfterFunc(d.window, d.Flush)
		}
		d.mu.Unlock()
		return
	}
	batch := d.pending
	d.pending = nil
	d.mu.Unlock()
	d.show(batch)
}

// Flush shows everything queued right now.
func (d *Dispatcher) Flush() {
	if d == nil {
		return
	}
	d.mu.Lock()
	batch := d.pending
	d.pending = nil
	d.timer = nil
	d.mu.Unlock()
	d.show(batch)
}

// Close flushes queued notifications and stops accepting new ones.
func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.closed = true
	batch := d.pending
	d.pending = nil
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.mu.Unlock()
	d.show(batch)
}

func (d *Dispatcher) show(batch []Notification) {
	for _, n := range mergeBatch(batch) {
		if err := d.notifier.Notify(context.Background(), n); err != nil {
			d.report(err)
			continue
		}
		d.clearSkips()
	}
}

func (d *Dispatcher) report(err error) {
	var skip *SkipError
	if !errors.As(err, &skip) {
		slog.Warn("notify.desktop_failed", "err", err)
		return
	}
	d.mu.Lock()
	seen := d.logged[skip.Reason]
	d.logged[skip.Reason] = true
	d.mu.Unlock()
	if !seen {
		slog.Warn("notify.desktop_skipped", "reason", skip.Reason)
	}
}

func (d *Dispatcher) clearSkips() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.logged) > 0 {
		d.logged = make(map[string]bool)
	}
}

func mergeBatch(batch []Notification) []Notification {
	if len(batch) <= 1 {
		return batch
	}
	order := make([]string, 0, len(batch))
	groups := make(map[string][]Notification, len(batch))
	for _, n := range batch {
		if _, ok := groups[n.Kind]; !ok {
			order = append(order, n.Kind)
		}
		groups[n.Kind] = append(groups[n.Kind], n)
	}
	out := make([]Notification, 0, len(order))
	for _, kind := range order {
		out = append(out, mergeGroup(groups[kind]))
	}
	return out
}

func mergeGroup(items []Notification) Notification {
	if len(items) == 1 {
		return items[0]
	}
	first := items[0]
	lines := make([]string, 0, len(items)+1)
	for i, item := range items {
		if i == 8 {
			lines = append(lines, fmt.Sprintf("…and %d more", len(items)-i))
			break
		}
		line := firstLine(item.Title)
		if extra := firstLine(item.Body); extra != "" && extra != line {
			if line == "" {
				line = extra
			} else {
				line += " · " + extra
			}
		}
		lines = append(lines, clampRunes(line, 140))
	}
	return Notification{Kind: first.Kind, Title: fmt.Sprintf("%d notifications: %s", len(items), first.Title), Body: strings.Join(lines, "\n")}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func clampRunes(s string, max int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max]) + "…"
}
