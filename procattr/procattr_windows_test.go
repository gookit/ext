//go:build windows

package procattr

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBackgroundSetsFlags(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit 0")
	Background(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatalf("HideWindow not set: %+v", cmd.SysProcAttr)
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CREATE_NO_WINDOW not set: %#x", cmd.SysProcAttr.CreationFlags)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestBackgroundMergesExisting(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	Background(cmd)
	f := cmd.SysProcAttr.CreationFlags
	if f&windows.CREATE_NEW_PROCESS_GROUP == 0 || f&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("flags not merged: %#x", f)
	}
	if f&windows.DETACHED_PROCESS != 0 {
		t.Fatalf("must never combine with DETACHED_PROCESS")
	}
}
