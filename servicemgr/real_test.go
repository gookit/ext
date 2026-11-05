package servicemgr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// TestRealLifecycle registers a real start-at-login entry, so it only runs when
// GOOKIT_SERVICEMGR_REAL=1 and always uses random "gksm-test-<hex>" names that are
// unregistered in t.Cleanup. The per-OS helpers live in real_*_test.go.
func TestRealLifecycle(t *testing.T) {
	if os.Getenv("GOOKIT_SERVICEMGR_REAL") != "1" {
		t.Skip("set GOOKIT_SERVICEMGR_REAL=1 to register a real entry")
	}
	if !realSupported(t) {
		t.Skip("no real backend available in this environment")
	}
	ctx := context.Background()
	name := randomName(t)
	owner, other := "gksm-a", "gksm-b"
	exe, args := realProgram()
	spec := Spec{Name: name, Owner: owner, Exe: exe, Args: args, WorkDir: t.TempDir()}

	// Cleanup runs even when an assertion fails halfway; Unregister of an
	// already-removed entry is just ErrNotInstalled and is ignored here.
	t.Cleanup(func() { removeReal(t, ctx, name, owner, other) })

	st, err := Query(ctx, name)
	assert.NoErr(t, err)
	assert.False(t, st.Installed)

	t.Run("register and query", func(t *testing.T) {
		assert.NoErr(t, Register(ctx, spec))
		st, err := Query(ctx, name)
		assert.NoErr(t, err)
		assert.True(t, st.Installed)
		assert.Eq(t, owner, st.Owner)
		assert.Eq(t, spec.Exe, st.Exe)
		assert.Eq(t, spec.Args, st.Args)
	})

	t.Run("same owner updates", func(t *testing.T) {
		spec2 := spec
		spec2.Args = append(append([]string{}, args...), "with space", `q"uote`)
		assert.NoErr(t, Register(ctx, spec2))
		st, err := Query(ctx, name)
		assert.NoErr(t, err)
		assert.Eq(t, spec2.Args, st.Args)
	})

	t.Run("other owner is refused", func(t *testing.T) {
		o := spec
		o.Owner = other
		assert.ErrIs(t, Register(ctx, o), ErrOwnedByOther)
		assert.ErrIs(t, Start(ctx, name, other), ErrOwnedByOther)
		assert.ErrIs(t, Unregister(ctx, name, other), ErrOwnedByOther)
		st, err := Query(ctx, name)
		assert.NoErr(t, err)
		assert.Eq(t, owner, st.Owner)
	})

	t.Run("unmarked entry is refused", func(t *testing.T) {
		bare := randomName(t)
		plantUnmarked(t, bare)
		st, err := Query(ctx, bare)
		assert.NoErr(t, err)
		assert.True(t, st.Installed)
		assert.Eq(t, "", st.Owner)

		b := spec
		b.Name = bare
		assert.ErrIs(t, Register(ctx, b), ErrOwnedByOther)
		assert.ErrIs(t, Start(ctx, bare, owner), ErrOwnedByOther)
		assert.ErrIs(t, Unregister(ctx, bare, owner), ErrOwnedByOther)
	})

	t.Run("start", func(t *testing.T) {
		assert.NoErr(t, Start(ctx, name, owner))
	})

	t.Run("unregister", func(t *testing.T) {
		assert.NoErr(t, Unregister(ctx, name, owner))
		st, err := Query(ctx, name)
		assert.NoErr(t, err)
		assert.False(t, st.Installed)
		assert.ErrIs(t, Unregister(ctx, name, owner), ErrNotInstalled)
		assert.ErrIs(t, Start(ctx, name, owner), ErrNotInstalled)
	})
}

// randomName returns gksm-test-<8 hex>, a valid service name.
func randomName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	_, err := rand.Read(b)
	assert.Require(t, assert.NoErr(t, err))
	return "gksm-test-" + hex.EncodeToString(b)
}

// removeReal unregisters name under whichever owner holds it; failures other than
// "not installed" are reported so a leaked entry is visible in the test log.
func removeReal(t *testing.T, ctx context.Context, name string, owners ...string) {
	t.Helper()
	for _, o := range owners {
		err := Unregister(ctx, name, o)
		if err == nil {
			return
		}
	}
	if st, err := Query(ctx, name); err == nil && st.Installed {
		t.Errorf("leaked test entry %q; remove it manually", name)
	}
}
