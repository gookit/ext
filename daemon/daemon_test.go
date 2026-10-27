package daemon

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

func TestDaemonized(t *testing.T) {
	t.Setenv(EnvSentinel, "")
	assert.False(t, Daemonized())
	t.Setenv(EnvSentinel, "1")
	assert.True(t, Daemonized())
}

func TestPIDFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.pid")
	assert.NoErr(t, WritePIDFile(path, 4242))
	got, err := ReadPIDFile(path)
	assert.NoErr(t, err)
	assert.Eq(t, 4242, got)
	RemovePIDFile(path)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestReadPIDFileMissing(t *testing.T) {
	if _, err := ReadPIDFile(filepath.Join(t.TempDir(), "nope.pid")); err == nil {
		t.Fatal("expected error reading a missing pidfile")
	}
}

func TestReadPIDFileInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pid")
	assert.NoErr(t, os.WriteFile(path, []byte("not-a-number"), 0o644))
	_, err := ReadPIDFile(path)
	assert.Err(t, err)
}

func TestPIDAlive(t *testing.T) {
	assert.True(t, PIDAlive(os.Getpid()))
	assert.False(t, PIDAlive(0))
	pid := deadChildPid(t)
	assert.False(t, PIDAlive(pid))
}

func TestClaimOwnsAndReleasesOnlyOwnPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claim.pid")
	release, owned := Claim(path)
	assert.True(t, owned)
	got, err := ReadPIDFile(path)
	assert.NoErr(t, err)
	assert.Eq(t, os.Getpid(), got)
	// A second claim by this process is idempotent.
	_, owned = Claim(path)
	assert.True(t, owned)

	parent := os.Getppid()
	assert.NoErr(t, WritePIDFile(path, parent))
	release()
	got, err = ReadPIDFile(path)
	assert.NoErr(t, err)
	assert.Eq(t, parent, got)
	_, owned = Claim(path)
	assert.False(t, owned)
}

func TestTakeoverRepointsPidfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "takeover.pid")
	assert.NoErr(t, WritePIDFile(path, os.Getppid()))
	release := Takeover(path)
	got, err := ReadPIDFile(path)
	assert.NoErr(t, err)
	assert.Eq(t, os.Getpid(), got)
	release()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestTerminateDeliversToSelf(t *testing.T) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	defer signal.Stop(ch)
	NotifyStop(ch)

	if err := Terminate(os.Getpid()); err != nil {
		t.Fatalf("Terminate(self): %v", err)
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("Terminate(self) delivered nothing to NotifyStop")
	}
}

// re-execs this test binary as the helper child, records its readiness in the
// sidecar log, then stops it through Terminate.
func TestSpawnDetachedRoundTrip(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{os.Args[0], "-test.run=^TestHelperDaemonChild$"}

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "t.pid")
	logPath := filepath.Join(dir, "t.out.log")
	pid, err := Spawn(Options{Name: "test", PIDPath: pidPath, LogPath: logPath})
	assert.NoErr(t, err)
	assert.True(t, pid > 0)
	defer func() { _ = Terminate(pid) }()
	waitForLogText(t, logPath, "child-ready", 10*time.Second)
	assert.True(t, PIDAlive(pid))
	got, err := ReadPIDFile(pidPath)
	assert.NoErr(t, err)
	assert.Eq(t, pid, got)
	assert.NoErr(t, Terminate(pid))
	waitForLogText(t, logPath, "child-stopped", 5*time.Second)
}

// TestHelperDaemonChild is the detached child of TestSpawnDetachedRoundTrip.
func TestHelperDaemonChild(t *testing.T) {
	if !Daemonized() {
		t.Skip("helper for TestSpawnDetachedRoundTrip; runs only in the detached child")
	}
	_, _ = os.Stdout.WriteString("child-ready\n")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)
	NotifyStop(sig)
	select {
	case <-sig:
		_, _ = os.Stdout.WriteString("child-stopped\n")
	case <-time.After(30 * time.Second):
		t.Fatal("helper daemon child: no stop signal within 30s")
	}
}

func deadChildPid(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit 0")
	} else {
		cmd = exec.Command("true")
	}
	assert.NoErr(t, cmd.Start())
	pid := cmd.Process.Pid
	assert.NoErr(t, cmd.Wait())
	return pid
}

func waitForLogText(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("log %s did not contain %q within %s (err=%v, content=%q)", path, want, timeout, err, string(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSameExecutable(t *testing.T) {
	self, err := os.Executable()
	assert.NoErr(t, err)
	assert.True(t, SameExecutable(self, self))
	assert.False(t, SameExecutable("", self))
}

func TestSpawnRefusesLivePID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.pid")
	assert.NoErr(t, WritePIDFile(path, os.Getpid()))
	_, err := Spawn(Options{Name: "test", PIDPath: path, LogPath: filepath.Join(t.TempDir(), "out.log")})
	assert.True(t, errors.Is(err, ErrAlreadyRunning))
}

func TestPIDFileText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pid")
	assert.NoErr(t, WritePIDFile(path, 123))
	data, err := os.ReadFile(path)
	assert.NoErr(t, err)
	assert.Eq(t, "123\n", string(data))
	assert.Eq(t, strconv.Itoa(123)+"\n", string(data))
}
