package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ProcessIdentity binds a PID to its image, OS owner and creation identity.
// StartID prevents a reused PID from matching a previously recorded process.
type ProcessIdentity struct {
	PID     int    `json:"pid,omitempty"`
	Exe     string `json:"exe,omitempty"`
	Owner   string `json:"owner,omitempty"`
	StartID string `json:"start_id,omitempty"`
}

// CurrentProcessIdentity returns the identity of the current process.
func CurrentProcessIdentity() (ProcessIdentity, error) { return InspectProcess(os.Getpid()) }

// SameExecutable compares executable paths, using file identity where possible.
func SameExecutable(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if ai, err := os.Stat(a); err == nil {
		if bi, err := os.Stat(b); err == nil && os.SameFile(ai, bi) {
			return true
		}
	}
	a, aerr := filepath.Abs(a)
	b, berr := filepath.Abs(b)
	if aerr != nil || berr != nil {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
