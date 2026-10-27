//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows has no setsid, and a process without a console receives no signals, so
// the three daemon primitives are built out of different pieces:
//
//   - detach: DETACHED_PROCESS means the child inherits NO console, so closing the
//     terminal (or Ctrl+C in it) cannot take it down. CREATE_BREAKAWAY_FROM_JOB
//     additionally escapes a job object the terminal host put us in (Windows
//     Terminal and some CI runners kill the whole job when the window closes). A
//     job that forbids breakaway refuses that flag with ERROR_ACCESS_DENIED, and
//     reexecDetached then retries without it.
//   - liveness: OpenProcess + GetExitCodeProcess (PIDAlive). No image-name check:
//     a reused pid is caught by the stop path, which refuses to guess (below).
//   - stop: a NAMED EVENT, not a console control event. GenerateConsoleCtrlEvent
//     only reaches processes that share the caller's console, and a detached
//     process has none, so stop sets Global\gokit-stop-<pid> and the target's
//     WaitForSingleObject goroutine turns that into a SIGTERM on its own signal
//     channel (NotifyStop). The event also makes stopping SAFE: the event's
//     existence proves the target is the very process this build started (a reused
//     pid has no event), which is why Terminate never falls back to
//     TerminateProcess — a hard kill would be free to shoot an unrelated process
//     that inherited the pid. The stale pidfile is a human decision (KillHint).
//
// The event lives in the Global\ namespace so a stop issued from another session
// (e.g. a job running under the session-0 service) still reaches a process running
// on the user's desktop. Cross-user stops are impossible (default DACL): the
// operator gets the taskkill hint instead.

// stillActive is STILL_ACTIVE (winnt.h) — the exit code GetExitCodeProcess reports
// while a process is still running. x/sys/windows does not export it.
const stillActive = 259

const (
	stopEventPrefix   = `Global\gokit-stop-`
	reloadEventPrefix = `Global\gokit-reload-`
)

// noConsoleSession is what WTSGetActiveConsoleSessionId returns when no session is
// attached to the console.
const noConsoleSession = 0xFFFFFFFF

// detachFlags are on every attempt; CREATE_BREAKAWAY_FROM_JOB is added on the
// first attempt only (see startDetached).
const detachFlags = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP

func stopEventName(pid int) string   { return fmt.Sprintf("%s%d", stopEventPrefix, pid) }
func reloadEventName(pid int) string { return fmt.Sprintf("%s%d", reloadEventPrefix, pid) }

// reexecDetached starts a copy of the current binary detached from our console,
// with stdin closed and stdout/stderr appended to the sidecar output file. The
// sentinel guard makes the child skip its own daemonization.
func reexecDetached(logPath, sentinel string) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return StartDetached(self, os.Args[1:], append(os.Environ(), sentinel+"=1"), logPath)
}

// StartDetached starts exe detached from our console (see the file comment), with
// stdin closed and stdout/stderr appended to logPath, in the environment env.
func StartDetached(exe string, args, env []string, logPath string) (*exec.Cmd, error) {
	return startDetached(exe, args, env, logPath, false)
}

// StartDetachedStrict starts an independent upgrade helper. Unlike the normal
// daemon path it never retries inside the caller's job object when breakaway is
// denied. A failed Start leaves no child running.
func StartDetachedStrict(exe string, args, env []string, logPath string) (*exec.Cmd, error) {
	return startDetached(exe, args, env, logPath, true)
}

func startDetached(exe string, args, env []string, logPath string, strict bool) (*exec.Cmd, error) {
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// A fresh Cmd per attempt: a failed Start must not be retried on the same Cmd.
	var cmd *exec.Cmd
	start := func(flags uint32) error {
		cmd = exec.Command(exe, args...)
		cmd.Env = env
		cmd.Stdin = nil // NUL
		cmd.Stdout = lf
		cmd.Stderr = lf
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		return cmd.Start()
	}

	if err := start(detachFlags | windows.CREATE_BREAKAWAY_FROM_JOB); err != nil {
		if strict {
			_ = lf.Close()
			return nil, fmt.Errorf("detached breakaway: %w", err)
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			_ = lf.Close()
			return nil, err
		}
		// A caller job that forbids breakaway forces the child to remain in that
		// job. Keep the compatibility retry, but make the lifecycle risk visible:
		// the caller can take the background process down when its job ends.
		slog.Warn("daemon.breakaway_denied", "error", err,
			"warning", "background process stays in the caller's job and ends with it")
		if err := start(detachFlags); err != nil {
			_ = lf.Close()
			return nil, err
		}
	}
	// The child inherited the handle; the parent no longer needs it.
	_ = lf.Close()
	return cmd, nil
}

// PIDAlive reports whether a live process with pid exists. ERROR_ACCESS_DENIED
// means it exists but belongs to another user / a higher integrity level, so it is
// alive; every other open error (notably ERROR_INVALID_PARAMETER, "no such pid")
// means it is not. A process that has exited keeps its pid until the handle is
// closed, hence the exit-code check rather than a successful open.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// NotifyStop creates this process's stop event and delivers a SIGTERM to ch when a
// stop command sets it (see the file comment). A failure to create the event is not
// fatal — it only means this process cannot be stopped through the event.
func NotifyStop(ch chan<- os.Signal) {
	name := stopEventName(os.Getpid())
	h, err := windows.CreateEvent(nil, 1 /* manual reset */, 0 /* not signaled */, windows.StringToUTF16Ptr(name))
	if err != nil {
		slog.Warn("daemon.stop_event_unavailable", "event", name, "error", err)
		return
	}
	go func() {
		defer windows.CloseHandle(h)
		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
			slog.Warn("daemon.stop_event_wait_failed", "event", name, "error", err)
			return
		}
		// The caller's channel is buffered and read by one goroutine; a full
		// channel means a stop is already pending, so dropping is correct.
		select {
		case ch <- syscall.SIGTERM:
		default:
		}
	}()
}

// NotifyReload waits for this process's named reload event and maps it to the
// same SIGHUP path used on Unix. The event is per-pid so a reused pid cannot
// receive a reload intended for another process.
func NotifyReload(ch chan<- os.Signal) {
	h, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(reloadEventName(os.Getpid())))
	if err != nil {
		slog.Warn("daemon.reload_event_unavailable", "event", reloadEventName(os.Getpid()), "error", err)
		return
	}
	go func() {
		defer windows.CloseHandle(h)
		for {
			if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
				slog.Warn("daemon.reload_event_wait_failed", "event", reloadEventName(os.Getpid()), "error", err)
				return
			}
			select {
			case ch <- syscall.SIGHUP:
			default:
			}
			// Manual-reset events stay signaled. Reset before waiting for the next
			// request so each command produces one reload notification.
			if err := windows.ResetEvent(h); err != nil {
				slog.Warn("daemon.reload_event_reset_failed", "event", reloadEventName(os.Getpid()), "error", err)
				return
			}
		}
	}()
}

// RequestReload asks a Windows process to reload through its named event.
func RequestReload(pid int) error {
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(reloadEventName(pid)))
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("reload event not found for pid=%d: not a process started by this build, or the pid was reused", pid)
		}
		return fmt.Errorf("open reload event %s: %w", reloadEventName(pid), err)
	}
	defer windows.CloseHandle(h)
	if err := windows.SetEvent(h); err != nil {
		return fmt.Errorf("set reload event %s: %w", reloadEventName(pid), err)
	}
	return nil
}

// Terminate asks the process pid to shut down gracefully by setting its stop
// event. A missing event means pid is not a process this build daemonized (or the
// pid was reused) — a hard kill is then the operator's call, so the error carries
// KillHint rather than terminating anything here.
func Terminate(pid int) error {
	name := stopEventName(pid)
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(name))
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("stop event not found for pid=%d: not a process started by this build, or the pid was reused; if it is really gone remove the pidfile, otherwise %s", pid, KillHint(pid))
		}
		return fmt.Errorf("open stop event %s: %w", name, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.SetEvent(h); err != nil {
		return fmt.Errorf("set stop event %s: %w", name, err)
	}
	return nil
}

// KillHint is the manual hard kill an operator can run when a graceful stop
// failed or was refused.
func KillHint(pid int) string { return fmt.Sprintf("taskkill /PID %d /F", pid) }

// SessionInfo reports the Windows session of the current process, so startup logs
// say whether this process can reach a user desktop.
//
// Interactive is "session is not 0": session 0 is the service session, which by
// design cannot touch a desktop, while EVERY other session is a user session a
// desktop belongs to — RDP included. Comparing against the console session instead
// under-reported on RDP-only hosts, where the user's session is e.g. 2 while
// WTSGetActiveConsoleSessionId() is the idle session 1. Console keeps that narrower
// fact (this process is the one sitting on the physical console) for whoever needs
// it: Console implies Interactive by construction.
func SessionInfo() Session {
	var id uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &id); err != nil {
		return Session{}
	}
	console := windows.WTSGetActiveConsoleSessionId()
	return Session{
		ID:          id,
		Interactive: id != 0,
		Console:     id != 0 && console != noConsoleSession && id == console,
	}
}

// KillDetached hard-kills a process started by StartDetached. Windows has no process
// group to signal here (the child broke away from our job object on purpose), so only
// the process itself is ended.
func KillDetached(p *os.Process) error { return p.Kill() }
