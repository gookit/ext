// Package servicemgr registers a per-user "start at login" entry for a program and
// can start it once on demand.
//
// Scope is deliberately small: current-user level only (no elevation, no system
// services), and there is no Stop, because stopping is best done by the program's
// own graceful-stop path. Backends:
//
//   - Windows: Task Scheduler (COM), a logon trigger running with the user's
//     interactive token and least privilege.
//   - Linux: a systemd --user unit. Without a reachable user bus every operation
//     returns ErrUnsupported and nothing is written.
//   - other systems: every operation returns ErrUnsupported.
//
// Every entry carries an ownership marker "gookit-servicemgr:v1:owner=<owner>"
// (task description on Windows, first comment line of the unit on Linux). Register,
// Unregister and Start refuse to touch an entry whose marker is missing or names a
// different owner, so a caller can never modify a service it did not create.
package servicemgr

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Sentinel errors, test with errors.Is.
var (
	// ErrUnsupported means the platform or environment has no usable backend.
	ErrUnsupported = errors.New("servicemgr: unsupported platform or environment")
	// ErrNotInstalled means the named entry does not exist.
	ErrNotInstalled = errors.New("servicemgr: service not installed")
	// ErrOwnedByOther means the entry exists but has no marker or a different owner.
	ErrOwnedByOther = errors.New("servicemgr: service is owned by another owner")
	// ErrInvalidSpec means a name, owner, path or argument failed validation.
	ErrInvalidSpec = errors.New("servicemgr: invalid spec")
)

// Spec describes one start-at-login entry.
type Spec struct {
	Name    string   // service name, must match ^[a-z][a-z0-9-]{0,63}$
	Owner   string   // caller identity stored in the ownership marker, same charset as Name
	Exe     string   // absolute path of the executable
	Args    []string // fixed arguments, never passed through a shell
	WorkDir string   // absolute working directory
}

// Status is what Query reports about an entry.
type Status struct {
	Installed bool
	Owner     string   // empty when the entry exists but has no ownership marker
	Exe       string   // registered executable
	Args      []string // registered arguments
}

// markerPrefix is the fixed text before the owner in the ownership marker.
const markerPrefix = "gookit-servicemgr:v1:owner="

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSpec, fmt.Sprintf(format, a...))
}

// checkText rejects control characters that could break out of a config line or an
// XML/command-line field: NUL, CR and LF.
func checkText(field, s string) error {
	if strings.ContainsAny(s, "\x00\r\n") {
		return invalid("%s contains a newline or NUL", field)
	}
	return nil
}

func validateName(name string) error {
	if !nameRe.MatchString(name) {
		return invalid("name %q must match %s", name, nameRe)
	}
	return nil
}

func validateOwner(owner string) error {
	if !nameRe.MatchString(owner) {
		return invalid("owner %q must match %s", owner, nameRe)
	}
	return nil
}

// validateRef checks the (name, owner) pair used by Unregister and Start.
func validateRef(name, owner string) error {
	if err := validateName(name); err != nil {
		return err
	}
	return validateOwner(owner)
}

// validate checks every field of s. It is the single gate in front of anything that
// is written to the task scheduler or a unit file.
func (s Spec) validate() error {
	if err := validateRef(s.Name, s.Owner); err != nil {
		return err
	}
	paths := []struct{ field, p string }{{"Exe", s.Exe}, {"WorkDir", s.WorkDir}}
	for _, it := range paths {
		if err := checkText(it.field, it.p); err != nil {
			return err
		}
		if !filepath.IsAbs(it.p) {
			return invalid("%s %q must be an absolute path", it.field, it.p)
		}
	}
	for i, a := range s.Args {
		if err := checkText(fmt.Sprintf("Args[%d]", i), a); err != nil {
			return err
		}
	}
	return nil
}

// ownerMarker is the full marker text for owner.
func ownerMarker(owner string) string { return markerPrefix + owner }

// parseOwnerMarker extracts the owner from text that is exactly a marker. It
// returns "" when the text is not a marker, so unmarked entries read as ownerless.
func parseOwnerMarker(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, markerPrefix) {
		return ""
	}
	owner := strings.TrimPrefix(text, markerPrefix)
	if !nameRe.MatchString(owner) {
		return ""
	}
	return owner
}

// checkOwner maps an existing entry's owner to the error callers get: the entry may
// be modified only when its marker names owner.
func checkOwner(name, have, want string) error {
	if have != want || have == "" {
		return fmt.Errorf("%w: %q (marker owner %q)", ErrOwnedByOther, name, have)
	}
	return nil
}
