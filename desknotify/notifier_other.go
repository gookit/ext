//go:build !windows && !darwin && !linux

package desknotify

import "context"

type unsupportedNotifier struct{}

// NewSystemNotifier returns an inert notifier on unsupported platforms.
func NewSystemNotifier() Notifier { return unsupportedNotifier{} }

func PlatformSupported() bool { return false }

func (unsupportedNotifier) Notify(context.Context, Notification) error {
	return &SkipError{Reason: "desktop notifications are not supported on this platform"}
}
