package servicemgr

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// absPath returns a path that is absolute on the current OS.
func absPath(elem ...string) string {
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	return filepath.Join(append([]string{root}, elem...)...)
}

func goodSpec() Spec {
	return Spec{
		Name: "demo-svc", Owner: "demo-owner",
		Exe: absPath("opt", "demo"), Args: []string{"serve", "--home", absPath("data")},
		WorkDir: absPath("data"),
	}
}

func TestSpec_validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoErr(t, goodSpec().validate())
	})

	t.Run("invalid", func(t *testing.T) {
		cases := []struct {
			name string
			mut  func(*Spec)
		}{
			{"empty name", func(s *Spec) { s.Name = "" }},
			{"upper name", func(s *Spec) { s.Name = "Demo" }},
			{"digit first", func(s *Spec) { s.Name = "1demo" }},
			{"name with space", func(s *Spec) { s.Name = "de mo" }},
			{"name with newline", func(s *Spec) { s.Name = "demo\nx" }},
			{"name too long", func(s *Spec) { s.Name = "a" + string(make([]byte, 64)) }},
			{"owner with colon", func(s *Spec) { s.Owner = "a:b" }},
			{"empty owner", func(s *Spec) { s.Owner = "" }},
			{"relative exe", func(s *Spec) { s.Exe = "bin/demo" }},
			{"relative workdir", func(s *Spec) { s.WorkDir = "data" }},
			{"exe with newline", func(s *Spec) { s.Exe = s.Exe + "\nExecStop=x" }},
			{"workdir with NUL", func(s *Spec) { s.WorkDir = s.WorkDir + "\x00" }},
			{"arg with newline", func(s *Spec) { s.Args = []string{"a", "b\nExecStartPost=/bin/sh"} }},
			{"arg with CR", func(s *Spec) { s.Args = []string{"a\rb"} }},
			{"arg with NUL", func(s *Spec) { s.Args = []string{"a\x00b"} }},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				s := goodSpec()
				c.mut(&s)
				assert.ErrIs(t, s.validate(), ErrInvalidSpec)
			})
		}
	})

	t.Run("args with shell metacharacters are accepted", func(t *testing.T) {
		s := goodSpec()
		s.Args = []string{`a b`, `q"uote`, `100%`, `$HOME`, `<&>|;`, ``}
		assert.NoErr(t, s.validate())
	})
}

func TestValidateRef(t *testing.T) {
	assert.NoErr(t, validateRef("demo", "owner-1"))
	assert.ErrIs(t, validateRef("Demo", "owner"), ErrInvalidSpec)
	assert.ErrIs(t, validateRef("demo", "bad owner"), ErrInvalidSpec)
	assert.ErrIs(t, validateName("a\n"), ErrInvalidSpec)
}

func TestOwnerMarker(t *testing.T) {
	assert.Eq(t, "gookit-servicemgr:v1:owner=abc", ownerMarker("abc"))

	t.Run("parse", func(t *testing.T) {
		assert.Eq(t, "abc", parseOwnerMarker("gookit-servicemgr:v1:owner=abc"))
		assert.Eq(t, "abc", parseOwnerMarker("  gookit-servicemgr:v1:owner=abc \r\n"))
		assert.Eq(t, "", parseOwnerMarker(""))
		assert.Eq(t, "", parseOwnerMarker("some other description"))
		assert.Eq(t, "", parseOwnerMarker("gookit-servicemgr:v1:owner=Bad Owner"))
		assert.Eq(t, "", parseOwnerMarker("x gookit-servicemgr:v1:owner=abc"))
	})
}

func TestCheckOwner(t *testing.T) {
	assert.NoErr(t, checkOwner("n", "a", "a"))
	assert.ErrIs(t, checkOwner("n", "a", "b"), ErrOwnedByOther)
	// an entry without a marker is never ours, even for an empty want
	assert.ErrIs(t, checkOwner("n", "", "a"), ErrOwnedByOther)
	assert.ErrIs(t, checkOwner("n", "", ""), ErrOwnedByOther)
}
