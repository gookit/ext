//go:build windows

package servicemgr

import (
	"os"
	"os/user"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func realSupported(t *testing.T) bool { return true }

// realProgram is a program that exits immediately, so Start leaves nothing running.
func realProgram() (string, []string) {
	exe := os.Getenv("ComSpec")
	if exe == "" {
		exe = `C:\Windows\System32\cmd.exe`
	}
	return exe, []string{"/c", "exit", "0"}
}

// plantUnmarked registers a task through COM directly with no Description, i.e. a
// task this package did not create. It is deleted again in t.Cleanup.
func plantUnmarked(t *testing.T, name string) {
	t.Helper()
	cur, err := user.Current()
	assert.Require(t, assert.NoErr(t, err))
	exe, args := realProgram()
	text, err := buildTaskXML(Spec{Name: name, Owner: "gksm-bare", Exe: exe, Args: args, WorkDir: t.TempDir()}, cur.Username)
	assert.Require(t, assert.NoErr(t, err))
	// drop the marker so the task looks hand made
	bare := strings.Replace(text, "<Description>"+ownerMarker("gksm-bare")+"</Description>", "", 1)
	assert.Require(t, assert.NotEq(t, text, bare))
	assert.Require(t, assert.NoErr(t, registerScheduledTask(name, bare, cur.Username, false)))
	t.Cleanup(func() {
		if err := deleteScheduledTask(name); err != nil {
			t.Errorf("leaked unmarked test task %q: %v", name, err)
		}
	})
}
