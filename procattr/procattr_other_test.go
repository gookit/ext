//go:build !windows

package procattr

import (
	"os/exec"
	"testing"
)

func TestBackgroundNoopOffWindows(t *testing.T) {
	cmd := exec.Command("true")
	Background(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatalf("Background must not touch SysProcAttr off Windows, got %+v", cmd.SysProcAttr)
	}
	Background(nil) // nil-safe
}
