// Package daemon turns a foreground process into a detached background process.
// Go cannot fork() safely (the runtime owns multiple threads), so the "-d" path
// re-execs the SAME binary with an env sentinel (EnvSentinel) set: the parent
// spawns the detached child and exits; the child sees the sentinel, skips
// daemonization and runs the real command. A pidfile records the child PID for
// duplicate-start detection and for stop subcommands. The platform-specific
// detach / signal mechanics live in daemon_unix.go and daemon_windows.go.
package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnvSentinel marks that the current process is already the detached child, so
// re-exec is not attempted again (would otherwise fork-bomb). The parent sets
// it to "1" on the child's environment; Daemonized() reads it.
const EnvSentinel = "GOKIT_DAEMONIZED"

// ErrAlreadyRunning is returned by Spawn when the pidfile points at a live
// process — a second start is refused rather than spawning a duplicate.
var ErrAlreadyRunning = errors.New("already running")

// Options describes one daemonization: a diagnostic name plus the runtime file
// locations (resolved by the caller, typically a config runtime path).
type Options struct {
	Name    string // diagnostic label, e.g. "serve" / "worker-<id>"
	PIDPath string // pidfile absolute path
	LogPath string // child stdout/stderr sidecar output target (for example *.out.log)

	// Settle is how long Spawn watches the new child before reporting success: a child
	// that exits within it makes Spawn fail with the tail of its output (*ExitedError).
	// Zero = do not wait (the pre-existing fire-and-forget behaviour).
	Settle time.Duration
	// Ready, when set, is polled during Settle; returning true ends the wait early
	// (the child is healthy). Without it the child just has to survive Settle.
	Ready func() bool
}

// ExitedError is returned by Spawn when the child exited during the settle window,
// i.e. the background start did not succeed. Tail is the last lines the child wrote.
type ExitedError struct {
	Name string
	PID  int
	Exit string // how it ended, e.g. "exit status 1"
	Tail string
}

func (e *ExitedError) Error() string {
	msg := fmt.Sprintf("%s: background process exited during startup (pid=%d, %s)", e.Name, e.PID, e.Exit)
	if e.Tail != "" {
		msg += ", log tail:\n" + e.Tail
	}
	return msg
}

// Session reports which OS session a process runs in and whether that is a session a
// user can reach. On Unix it is the zero value.
type Session struct {
	ID          uint32
	Interactive bool
	Console     bool
}

// SessionInfo returns the session of the current process. It is implemented per
// platform (daemon_unix.go / daemon_windows.go).

// Daemonized reports whether the current process is the detached child (the
// env sentinel is set). The command layer calls this to decide whether to
// re-exec (parent) or run the real command (child).
func Daemonized() bool { return os.Getenv(EnvSentinel) == "1" }

// Spawn re-execs the current binary as a detached background child and returns
// its PID. It refuses to start when the pidfile already points at a live process
// (ErrAlreadyRunning). A stale pidfile (dead PID) is silently overwritten. Only
// the parent calls Spawn; the child (Daemonized()==true) never reaches here.
func Spawn(o Options) (int, error) {
	if pid, err := ReadPIDFile(o.PIDPath); err == nil && PIDAlive(pid) {
		return 0, fmt.Errorf("%s: %w (pid=%d, pidfile=%s)", o.Name, ErrAlreadyRunning, pid, o.PIDPath)
	}

	// Ensure runtime dirs exist before the child opens its log / we write the pidfile.
	if err := os.MkdirAll(filepath.Dir(o.LogPath), 0o755); err != nil {
		return 0, fmt.Errorf("create log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(o.PIDPath), 0o755); err != nil {
		return 0, fmt.Errorf("create pid dir: %w", err)
	}

	var logStart int64
	if fi, err := os.Stat(o.LogPath); err == nil {
		logStart = fi.Size()
	}
	cmd, err := reexecDetached(o.LogPath)
	if err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Reap the child when it eventually exits. A caller that outlives it would
	// otherwise keep an unreaped child: on Unix that child is a zombie.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	if err := WritePIDFile(o.PIDPath, pid); err != nil {
		// The child is already detached and running; surface the pidfile failure
		// but do not kill it — losing the pidfile only breaks stop.
		return pid, fmt.Errorf("child started (pid=%d) but writing pidfile failed: %w", pid, err)
	}
	if o.Settle > 0 {
		if werr, died := waitSettled(o, exited); died {
			if cur, err := ReadPIDFile(o.PIDPath); err == nil && cur == pid {
				RemovePIDFile(o.PIDPath)
			}
			how := "exit status 0"
			if werr != nil {
				how = werr.Error()
			}
			return pid, &ExitedError{Name: o.Name, PID: pid, Exit: how, Tail: tailFile(o.LogPath, logStart, 20)}
		}
	}
	return pid, nil
}

// waitSettled watches the child for o.Settle. died reports that it exited inside
// the window (err is its exit error). It returns early with died=false once Ready is true.
func waitSettled(o Options, exited <-chan error) (err error, died bool) {
	deadline := time.NewTimer(o.Settle)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			return err, true
		case <-deadline.C:
			return nil, false
		case <-tick.C:
			if o.Ready != nil && o.Ready() {
				return nil, false
			}
		}
	}
}

// tailFile returns the last n lines of path written after byte offset from.
func tailFile(path string, from int64, n int) string {
	b, err := os.ReadFile(path)
	if err != nil || from < 0 || from > int64(len(b)) {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b[from:]), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Claim records the current process in pidPath so stop commands can find it,
// whether it was started detached or in the foreground.
//
// A pidfile already held by another live process is left untouched: owned is
// false and the returned release is a no-op. release re-reads the file and
// removes it only while it still holds our pid.
func Claim(pidPath string) (release func(), owned bool) {
	noop := func() {}
	if pid, err := ReadPIDFile(pidPath); err == nil && pid != os.Getpid() && PIDAlive(pid) {
		return noop, false
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		slog.Warn("daemon.pidfile_claim_failed", "pidfile", pidPath, "error", err)
		return noop, false
	}
	if err := WritePIDFile(pidPath, os.Getpid()); err != nil {
		slog.Warn("daemon.pidfile_claim_failed", "pidfile", pidPath, "error", err)
		return noop, false
	}
	self := os.Getpid()
	return func() {
		if pid, err := ReadPIDFile(pidPath); err == nil && pid == self {
			RemovePIDFile(pidPath)
		}
	}, true
}

// Takeover points pidPath at the current process unconditionally and returns
// the matching release. It is the pidfile half of a binary upgrade handover.
func Takeover(pidPath string) (release func()) {
	noop := func() {}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		slog.Warn("daemon.pidfile_takeover_failed", "pidfile", pidPath, "error", err)
		return noop
	}
	if err := WritePIDFile(pidPath, os.Getpid()); err != nil {
		slog.Warn("daemon.pidfile_takeover_failed", "pidfile", pidPath, "error", err)
		return noop
	}
	self := os.Getpid()
	return func() {
		if pid, err := ReadPIDFile(pidPath); err == nil && pid == self {
			RemovePIDFile(pidPath)
		}
	}
}

// WritePIDFile writes pid to path atomically (write temp + rename) so a reader
// never observes a half-written file.
func WritePIDFile(path string, pid int) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadPIDFile reads and parses the PID stored in path. A missing file or
// non-numeric content is an error (callers treat any error as "not running").
func ReadPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pidfile %s: %w", path, err)
	}
	return pid, nil
}

// RemovePIDFile deletes the pidfile, ignoring a missing file. The detached child
// calls this on graceful shutdown so a stale pidfile never lingers.
func RemovePIDFile(path string) { _ = os.Remove(path) }
