//go:build linux

package servicemgr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// fakeCtl records the systemctl calls and fails the ones listed in failOn (matched
// by the first argument, e.g. "enable").
type fakeCtl struct {
	calls  [][]string
	failOn map[string]bool
}

func (f *fakeCtl) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.failOn[args[0]] {
		return []byte("boom"), errors.New("exit status 1")
	}
	return nil, nil
}

func (f *fakeCtl) verbs() []string {
	var v []string
	for _, c := range f.calls {
		v = append(v, c[0])
	}
	return v
}

// newTestBackend points XDG_CONFIG_HOME at a temp dir so nothing under the real
// ~/.config/systemd/user can be touched, and returns the unit dir.
func newTestBackend(t *testing.T) (*systemd, *fakeCtl, string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	f := &fakeCtl{failOn: map[string]bool{}}
	return &systemd{run: f.run}, f, filepath.Join(cfg, "systemd", "user")
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	assert.NoErr(t, err)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func TestSystemd_noBus(t *testing.T) {
	d, f, dir := newTestBackend(t)
	f.failOn["show-environment"] = true
	ctx := context.Background()
	s := goodSpec()

	assert.ErrIs(t, d.register(ctx, s), ErrUnsupported)
	assert.ErrIs(t, d.unregister(ctx, s.Name, s.Owner), ErrUnsupported)
	assert.ErrIs(t, d.start(ctx, s.Name, s.Owner), ErrUnsupported)
	_, err := d.query(ctx, s.Name)
	assert.ErrIs(t, err, ErrUnsupported)
	// nothing written, not even the directory
	_, statErr := os.Stat(filepath.Dir(dir))
	assert.True(t, errors.Is(statErr, os.ErrNotExist))
	assert.Eq(t, 0, len(listFiles(t, dir)))
}

func TestSystemd_register(t *testing.T) {
	ctx := context.Background()
	s := goodSpec()

	t.Run("create writes unit and enables", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		assert.Eq(t, []string{s.Name + ".service"}, listFiles(t, dir))
		assert.Eq(t, []string{"show-environment", "daemon-reload", "enable"}, f.verbs())
		assert.Eq(t, []string{"enable", s.Name + ".service"}, f.calls[2])

		st, err := d.query(ctx, s.Name)
		assert.NoErr(t, err)
		assert.True(t, st.Installed)
		assert.Eq(t, s.Owner, st.Owner)
		assert.Eq(t, s.Exe, st.Exe)
		assert.Eq(t, s.Args, st.Args)
	})

	t.Run("same owner updates", func(t *testing.T) {
		d, _, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		s2 := s
		s2.Args = []string{"changed"}
		assert.NoErr(t, d.register(ctx, s2))
		st, err := d.query(ctx, s.Name)
		assert.NoErr(t, err)
		assert.Eq(t, []string{"changed"}, st.Args)
		assert.Eq(t, 1, len(listFiles(t, dir)))
	})

	t.Run("other owner and unmarked are refused", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		before, err := os.ReadFile(filepath.Join(dir, s.Name+".service"))
		assert.NoErr(t, err)

		o := s
		o.Owner = "someone-else"
		assert.ErrIs(t, d.register(ctx, o), ErrOwnedByOther)
		assert.ErrIs(t, d.start(ctx, s.Name, "someone-else"), ErrOwnedByOther)
		assert.ErrIs(t, d.unregister(ctx, s.Name, "someone-else"), ErrOwnedByOther)
		after, err := os.ReadFile(filepath.Join(dir, s.Name+".service"))
		assert.NoErr(t, err)
		assert.Eq(t, string(before), string(after))

		bare := filepath.Join(dir, "bare.service")
		assert.NoErr(t, os.WriteFile(bare, []byte("[Service]\nExecStart=/bin/true\n"), 0o644))
		b := s
		b.Name = "bare"
		assert.ErrIs(t, d.register(ctx, b), ErrOwnedByOther)
		assert.ErrIs(t, d.unregister(ctx, "bare", s.Owner), ErrOwnedByOther)
		assert.ErrIs(t, d.start(ctx, "bare", s.Owner), ErrOwnedByOther)
		data, err := os.ReadFile(bare)
		assert.NoErr(t, err)
		assert.Eq(t, "[Service]\nExecStart=/bin/true\n", string(data))
		for _, v := range f.verbs() {
			assert.NotEq(t, "disable", v)
			assert.NotEq(t, "start", v)
		}
	})

	t.Run("failed enable on create removes the new unit", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		f.failOn["enable"] = true
		err := d.register(ctx, s)
		assert.Err(t, err)
		assert.StrContains(t, err.Error(), "boom")
		assert.Eq(t, 0, len(listFiles(t, dir)))
	})

	t.Run("failed reload on update restores the previous unit", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		before, err := os.ReadFile(filepath.Join(dir, s.Name+".service"))
		assert.NoErr(t, err)

		f.failOn["daemon-reload"] = true
		s2 := s
		s2.Args = []string{"changed"}
		assert.Err(t, d.register(ctx, s2))
		after, err := os.ReadFile(filepath.Join(dir, s.Name+".service"))
		assert.NoErr(t, err)
		assert.Eq(t, string(before), string(after))
		assert.Eq(t, 1, len(listFiles(t, dir)))
	})

	t.Run("invalid spec writes nothing", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		bad := s
		bad.Args = []string{"a\nb"}
		assert.ErrIs(t, d.register(ctx, bad), ErrInvalidSpec)
		assert.Eq(t, 0, len(f.calls))
		assert.Eq(t, 0, len(listFiles(t, dir)))
	})
}

func TestSystemd_unregisterStartQuery(t *testing.T) {
	ctx := context.Background()
	s := goodSpec()

	t.Run("not installed", func(t *testing.T) {
		d, _, _ := newTestBackend(t)
		assert.ErrIs(t, d.unregister(ctx, s.Name, s.Owner), ErrNotInstalled)
		assert.ErrIs(t, d.start(ctx, s.Name, s.Owner), ErrNotInstalled)
		st, err := d.query(ctx, s.Name)
		assert.NoErr(t, err)
		assert.False(t, st.Installed)
	})

	t.Run("start", func(t *testing.T) {
		d, f, _ := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		assert.NoErr(t, d.start(ctx, s.Name, s.Owner))
		assert.Eq(t, []string{"start", s.Name + ".service"}, f.calls[len(f.calls)-1])
	})

	t.Run("unregister removes unit", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		assert.NoErr(t, d.unregister(ctx, s.Name, s.Owner))
		assert.Eq(t, 0, len(listFiles(t, dir)))
		assert.Eq(t, []string{"disable", "--now", s.Name + ".service"}, f.calls[len(f.calls)-2])
		assert.Eq(t, "daemon-reload", f.calls[len(f.calls)-1][0])
	})

	t.Run("failed disable keeps the unit", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		f.failOn["disable"] = true
		assert.Err(t, d.unregister(ctx, s.Name, s.Owner))
		assert.Eq(t, 1, len(listFiles(t, dir)))
	})

	t.Run("failed reload after removal is reported", func(t *testing.T) {
		d, f, dir := newTestBackend(t)
		assert.NoErr(t, d.register(ctx, s))
		f.failOn["daemon-reload"] = true
		assert.Err(t, d.unregister(ctx, s.Name, s.Owner))
		assert.Eq(t, 0, len(listFiles(t, dir)))
	})
}

func TestUserArgs(t *testing.T) {
	assert.Eq(t, []string{"--user", "enable", "x.service"}, userArgs("enable", "x.service"))
	assert.Eq(t, []string{"--user"}, userArgs())
}

func TestUnitDir(t *testing.T) {
	t.Run("xdg", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
		dir, err := unitDir()
		assert.NoErr(t, err)
		assert.Eq(t, filepath.Join("/tmp/xdg", "systemd", "user"), dir)
	})

	t.Run("default under home", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "/tmp/fakehome")
		dir, err := unitDir()
		assert.NoErr(t, err)
		assert.Eq(t, filepath.Join("/tmp/fakehome", ".config", "systemd", "user"), dir)
	})
}
