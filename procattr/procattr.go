// Package procattr holds the one platform-specific tweak every NON-interactive child
// process needs: on Windows a process without a console that starts a console
// program makes Windows open a fresh console window for every such child.
// Background suppresses that.
//
// It is the only supported way to start a background helper process. A guard test
// can keep new exec.Command call sites from reintroducing the Windows console flash.
package procattr

import "os/exec"

// Background configures cmd to run without opening a console window on Windows. It
// MERGES into an existing SysProcAttr / CreationFlags instead of replacing them and
// is a no-op on every other platform. Call it before cmd.Start/Run/Output.
//
// Do NOT use it on a detached daemon start: CREATE_NO_WINDOW and DETACHED_PROCESS are
// mutually exclusive (daemon_windows.go owns that path). Interactive children that
// should share the user's console also must not use it.
func Background(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	background(cmd)
}
