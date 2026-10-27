//go:build !windows

package procattr

import "os/exec"

func background(*exec.Cmd) {}
