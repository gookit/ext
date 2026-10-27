//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestSessionInfoInteractiveIsAnyUserSession pins the distinction between a user
// session and the physical console session: RDP is interactive even without a console.
func TestSessionInfoInteractiveIsAnyUserSession(t *testing.T) {
	s := SessionInfo()
	if want := s.ID != 0; s.Interactive != want {
		t.Fatalf("SessionInfo() = %+v: Interactive must be %v for session %d", s, want, s.ID)
	}
	if s.Console && !s.Interactive {
		t.Fatalf("SessionInfo() = %+v: Console=true must imply Interactive=true", s)
	}
}

// A helper in a Job Object with no BREAKAWAY_OK exercises the real Windows
// refusal. Strict must leave no child; ordinary daemon detach keeps its fallback.
func TestStartDetachedStrictRefusesForbiddenJob(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gate := filepath.Join(dir, "gate")
	result := filepath.Join(dir, "result")
	strictMarker := filepath.Join(dir, "strict-child")
	normalMarker := filepath.Join(dir, "normal-child")
	cmd := exec.Command(self, "-test.run=^TestStrictBreakawayHelper$")
	cmd.Env = append(os.Environ(), "GOKIT_STRICT_HELPER=1", "GOKIT_STRICT_GATE="+gate,
		"GOKIT_STRICT_RESULT="+result, "GOKIT_STRICT_MARKER="+strictMarker,
		"GOKIT_NORMAL_MARKER="+normalMarker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
		LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
	}}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		t.Fatal(err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		t.Skipf("host prevents nested test Job Object: %v", err)
	}
	if err := os.WriteFile(gate, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result)
	if err != nil || string(data) != "ok" {
		t.Fatalf("strict/fallback helper result %q, %v", data, err)
	}
	if _, err := os.Stat(strictMarker); !os.IsNotExist(err) {
		t.Fatalf("strict denial left a child marker: %v", err)
	}
	if _, err := os.Stat(normalMarker); err != nil {
		t.Fatalf("normal detach fallback did not start its child: %v", err)
	}
}

func TestStrictBreakawayHelper(t *testing.T) {
	if os.Getenv("GOKIT_STRICT_HELPER") != "1" {
		return
	}
	result := os.Getenv("GOKIT_STRICT_RESULT")
	fail := func(err error) { _ = os.WriteFile(result, []byte(err.Error()), 0o600); t.Fatal(err) }
	gate := os.Getenv("GOKIT_STRICT_GATE")
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, err := os.Stat(gate); err == nil {
			break
		}
		if time.Now().After(deadline) {
			fail(fmt.Errorf("timed out waiting for Job Object assignment"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	self, err := os.Executable()
	if err != nil {
		fail(err)
	}
	args := []string{"-test.run=^TestDetachedMarkerHelper$"}
	strictMarker := os.Getenv("GOKIT_STRICT_MARKER")
	logPath := filepath.Join(filepath.Dir(result), "detached.log")
	if child, err := StartDetachedStrict(self, args, append(os.Environ(), "GOKIT_DETACHED_MARKER="+strictMarker), logPath); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if child != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
		fail(fmt.Errorf("strict detach = %v, want access denied", err))
	}
	if _, err := os.Stat(strictMarker); !os.IsNotExist(err) {
		fail(fmt.Errorf("strict child ran: %v", err))
	}
	normalMarker := os.Getenv("GOKIT_NORMAL_MARKER")
	child, err := StartDetached(self, args, append(os.Environ(), "GOKIT_DETACHED_MARKER="+normalMarker), logPath)
	if err != nil {
		fail(fmt.Errorf("normal fallback: %w", err))
	}
	if err := child.Wait(); err != nil {
		fail(fmt.Errorf("normal child: %w", err))
	}
	if _, err := os.Stat(normalMarker); err != nil {
		fail(fmt.Errorf("normal marker: %w", err))
	}
	if err := os.WriteFile(result, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetachedMarkerHelper(t *testing.T) {
	if marker := os.Getenv("GOKIT_DETACHED_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("ran"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTerminateMissingTargetReportsHint(t *testing.T) {
	pid := deadChildPid(t)
	err := Terminate(pid)
	if err == nil {
		t.Fatalf("Terminate(%d) without a stop event should error", pid)
	}
	if !strings.Contains(err.Error(), "taskkill") {
		t.Fatalf("error must carry the manual-stop hint, got: %v", err)
	}
}
