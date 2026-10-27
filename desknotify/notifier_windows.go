//go:build windows

package desknotify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/gookit/ext/procattr"
	"github.com/gookit/goutil/envutil"
	"golang.org/x/sys/windows"
)

type systemNotifier struct{}

// NewSystemNotifier returns the Windows PowerShell toast implementation.
func NewSystemNotifier() Notifier { return systemNotifier{} }

// PlatformSupported reports whether this build has a desktop notifier
// implementation. Session 0 and missing PowerShell are reported by Notify.
func PlatformSupported() bool { return true }

type toastExecutor func(context.Context, map[string]string) (code int, out string, started bool)

var runToast toastExecutor = execToast

func (systemNotifier) Notify(ctx context.Context, n Notification) error {
	if err := requireInteractiveSession(); err != nil {
		return err
	}
	return showToast(ctx, runToast, n)
}

func showToast(ctx context.Context, run toastExecutor, n Notification) error {
	code, out, started := run(ctx, map[string]string{toastXMLEnv: toastXML(n)})
	if !started {
		return &SkipError{Reason: "powershell.exe not found"}
	}
	return toastResult(code, out)
}

func execToast(ctx context.Context, env map[string]string) (int, string, bool) {
	exe, err := exec.LookPath("powershell.exe")
	if err != nil {
		return 0, "", false
	}
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-EncodedCommand", toastScriptEncoded)
	procattr.Background(cmd)
	environment := envutil.Environ()
	for key, value := range env {
		environment[key] = value
	}
	cmd.Env = make([]string, 0, len(environment))
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return toastExitOK, string(out), true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out), true
	}
	return toastExitStartFailed, err.Error() + ": " + string(out), true
}

func requireInteractiveSession() error {
	var sid uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sid); err != nil {
		return fmt.Errorf("query session id: %w", err)
	}
	if sid == 0 {
		return &SkipError{Reason: "windows session 0 (service session, no interactive desktop)"}
	}
	return nil
}
