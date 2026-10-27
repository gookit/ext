//go:build linux

package desknotify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gookit/ext/procattr"
)

type systemNotifier struct{}

// NewSystemNotifier returns the Linux notify-send implementation.
func NewSystemNotifier() Notifier { return systemNotifier{} }

// PlatformSupported reports whether this build has a desktop notifier
// implementation. A missing desktop session or binary is reported by Notify.
func PlatformSupported() bool { return true }

func (systemNotifier) Notify(ctx context.Context, n Notification) error {
	if err := requireDesktopSession(); err != nil {
		return err
	}
	exe, err := exec.LookPath("notify-send")
	if err != nil {
		return &SkipError{Reason: "notify-send not installed (libnotify)"}
	}
	cmd := exec.CommandContext(ctx, exe, "--app-name=desknotify", n.Title, strings.TrimSpace(n.Body))
	procattr.Background(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notify-send: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func requireDesktopSession() error {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return nil
	}
	return &SkipError{Reason: "no desktop session (DISPLAY / WAYLAND_DISPLAY / DBUS_SESSION_BUS_ADDRESS unset)"}
}
