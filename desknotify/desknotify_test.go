package desknotify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gookit/ext/desknotify"
	"github.com/gookit/goutil/x/assert"
)

func TestDispatcherMergesByCallerKind(t *testing.T) {
	rec := &captureNotifier{}
	d := desknotify.NewDispatcher(rec, time.Hour)
	d.Push(desknotify.Notification{Kind: "approval", Title: "Review A", Body: "first"})
	d.Push(desknotify.Notification{Kind: "approval", Title: "Review B", Body: "second"})
	d.Push(desknotify.Notification{Kind: "reminder", Title: "Reminder"})
	d.Flush()

	got := rec.notifications()
	assert.Len(t, got, 2)
	assert.Equal(t, "2 notifications: Review A", got[0].Title)
	assert.Contains(t, got[0].Body, "Review A · first")
	assert.Contains(t, got[0].Body, "Review B · second")
	assert.Equal(t, "reminder", got[1].Kind)
}

func TestDispatcherCloseStopsPushes(t *testing.T) {
	rec := &captureNotifier{}
	d := desknotify.NewDispatcher(rec, time.Hour)
	d.Push(desknotify.Notification{Kind: "a", Title: "one"})
	d.Close()
	d.Push(desknotify.Notification{Kind: "a", Title: "two"})
	assert.Len(t, rec.notifications(), 1)
}

func TestDispatcherReportsSkipOnce(t *testing.T) {
	rec := &captureNotifier{err: &desknotify.SkipError{Reason: "no desktop"}}
	d := desknotify.NewDispatcher(rec, 0)
	d.Push(desknotify.Notification{Title: "one"})
	d.Push(desknotify.Notification{Title: "two"})
	assert.Len(t, rec.notifications(), 0)
}

func TestNotificationValidation(t *testing.T) {
	rec := &captureNotifier{}
	d := desknotify.NewDispatcher(rec, 0)
	d.Push(desknotify.Notification{Kind: "empty"})
	assert.Len(t, rec.notifications(), 0)
}

type captureNotifier struct {
	mu   sync.Mutex
	seen []desknotify.Notification
	err  error
}

func (r *captureNotifier) Notify(_ context.Context, n desknotify.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.seen = append(r.seen, n)
	return nil
}

func (r *captureNotifier) notifications() []desknotify.Notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]desknotify.Notification(nil), r.seen...)
}

func TestSkipError(t *testing.T) {
	err := &desknotify.SkipError{Reason: "missing notifier"}
	assert.True(t, errors.Is(err, err))
	assert.True(t, strings.Contains(err.Error(), "missing notifier"))
}
