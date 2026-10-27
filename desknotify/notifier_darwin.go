//go:build darwin

package desknotify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gookit/ext/procattr"
)

type systemNotifier struct{}

// NewSystemNotifier returns the macOS osascript implementation.
func NewSystemNotifier() Notifier { return systemNotifier{} }

// PlatformSupported reports whether this build has a desktop notifier
// implementation. A missing osascript binary is reported by Notify.
func PlatformSupported() bool { return true }

func (systemNotifier) Notify(ctx context.Context, n Notification) error {
	exe, err := exec.LookPath("osascript")
	if err != nil {
		return &SkipError{Reason: "osascript not found"}
	}
	script := "display notification " + appleScriptString(n.Body) + " with title " + appleScriptString(n.Title)
	cmd := exec.CommandContext(ctx, exe, "-e", script)
	procattr.Background(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript display notification: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appleScriptString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
