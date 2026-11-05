//go:build linux

package servicemgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runner executes one systemctl --user command and returns its combined output.
// It is a field (not a package variable) so tests can substitute a fake.
type runner func(ctx context.Context, args ...string) ([]byte, error)

// systemd is the Linux backend. The zero value is not usable; use newSystemd.
type systemd struct{ run runner }

func newSystemd() *systemd { return &systemd{run: runSystemctl} }

// runSystemctl runs "systemctl --user <args>" directly (no shell).
func runSystemctl(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "systemctl", userArgs(args...)...).CombinedOutput()
}

// userArgs prefixes --user, so no command can reach the system manager.
func userArgs(args ...string) []string { return append([]string{"--user"}, args...) }

// Register creates or updates the user unit for s and enables it.
func Register(ctx context.Context, s Spec) error { return newSystemd().register(ctx, s) }

// Unregister disables and removes the unit.
func Unregister(ctx context.Context, name, owner string) error {
	return newSystemd().unregister(ctx, name, owner)
}

// Start starts the unit once via systemctl --user start.
func Start(ctx context.Context, name, owner string) error {
	return newSystemd().start(ctx, name, owner)
}

// Query reports the unit's registration.
func Query(ctx context.Context, name string) (Status, error) {
	return newSystemd().query(ctx, name)
}

// unitDir is $XDG_CONFIG_HOME/systemd/user, or ~/.config/systemd/user.
func unitDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("servicemgr: locate home: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user"), nil
}

// probe checks that a systemd user manager is reachable. It runs before anything is
// written; failure (no bus, e.g. WSL without systemd) is reported as ErrUnsupported.
func (d *systemd) probe(ctx context.Context) error {
	if out, err := d.run(ctx, "show-environment"); err != nil {
		return fmt.Errorf("%w: systemctl --user unavailable: %v: %s", ErrUnsupported, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *systemd) systemctl(ctx context.Context, args ...string) error {
	if out, err := d.run(ctx, args...); err != nil {
		return fmt.Errorf("servicemgr: systemctl --user %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readUnit returns the existing unit text; exists is false when there is no file.
func readUnit(path string) (text string, exists bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("servicemgr: read unit: %w", err)
	}
	return string(data), true, nil
}

// writeUnit writes text to path through a temp file in the same directory and a
// rename, so a reader never sees a half-written unit.
func writeUnit(path, text string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gksm-*.tmp")
	if err != nil {
		return fmt.Errorf("servicemgr: create temp unit: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		return errors.Join(fmt.Errorf("servicemgr: write temp unit: %w", err), tmp.Close(), os.Remove(tmpName))
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(fmt.Errorf("servicemgr: close temp unit: %w", err), os.Remove(tmpName))
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return errors.Join(fmt.Errorf("servicemgr: chmod temp unit: %w", err), os.Remove(tmpName))
	}
	if err := os.Rename(tmpName, path); err != nil {
		return errors.Join(fmt.Errorf("servicemgr: install unit: %w", err), os.Remove(tmpName))
	}
	return nil
}

// locate probes the bus and returns the unit path plus its current text.
func (d *systemd) locate(ctx context.Context, name string) (path, text string, exists bool, err error) {
	if err = d.probe(ctx); err != nil {
		return
	}
	dir, err := unitDir()
	if err != nil {
		return
	}
	path = filepath.Join(dir, name+".service")
	text, exists, err = readUnit(path)
	return
}

func (d *systemd) register(ctx context.Context, s Spec) error {
	if err := s.validate(); err != nil {
		return err
	}
	path, old, existed, err := d.locate(ctx, s.Name)
	if err != nil {
		return err
	}
	if existed {
		if err := checkOwner(s.Name, parseUnit(old).Owner, s.Owner); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("servicemgr: create unit dir: %w", err)
	}
	if err := writeUnit(path, unitText(s)); err != nil {
		return err
	}
	// daemon-reload then enable; if either fails put the previous state back so a
	// failed update never leaves a half-registered unit.
	err = d.systemctl(ctx, "daemon-reload")
	if err == nil {
		err = d.systemctl(ctx, "enable", s.Name+".service")
	}
	if err == nil {
		return nil
	}
	var restoreErr error
	if existed {
		restoreErr = writeUnit(path, old)
	} else if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		restoreErr = fmt.Errorf("servicemgr: remove unit: %w", rmErr)
	}
	if restoreErr != nil {
		restoreErr = fmt.Errorf("servicemgr: restore previous unit: %w", restoreErr)
	}
	return errors.Join(err, restoreErr)
}

func (d *systemd) unregister(ctx context.Context, name, owner string) error {
	if err := validateRef(name, owner); err != nil {
		return err
	}
	path, text, exists, err := d.locate(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrNotInstalled, name)
	}
	if err := checkOwner(name, parseUnit(text).Owner, owner); err != nil {
		return err
	}
	// A failed disable keeps the file so the caller can retry.
	if err := d.systemctl(ctx, "disable", "--now", name+".service"); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("servicemgr: remove unit: %w", err)
	}
	return d.systemctl(ctx, "daemon-reload")
}

func (d *systemd) start(ctx context.Context, name, owner string) error {
	if err := validateRef(name, owner); err != nil {
		return err
	}
	_, text, exists, err := d.locate(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %q", ErrNotInstalled, name)
	}
	if err := checkOwner(name, parseUnit(text).Owner, owner); err != nil {
		return err
	}
	return d.systemctl(ctx, "start", name+".service")
}

func (d *systemd) query(ctx context.Context, name string) (Status, error) {
	if err := validateName(name); err != nil {
		return Status{}, err
	}
	_, text, exists, err := d.locate(ctx, name)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{}, nil
	}
	return parseUnit(text), nil
}
