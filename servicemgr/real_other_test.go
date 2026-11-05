//go:build !windows

package servicemgr

import "testing"

// realSupported is false off Windows: a real systemd flow cannot be isolated from
// the user's own ~/.config (the user manager reads its own XDG_CONFIG_HOME), so
// only the fake-runner tests in systemd_linux_test.go cover Linux.
func realSupported(t *testing.T) bool { return false }

func realProgram() (string, []string) { return "/bin/true", nil }

func plantUnmarked(t *testing.T, name string) { t.Fatal("unreachable") }
