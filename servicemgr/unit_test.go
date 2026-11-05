package servicemgr

import (
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestUnitQuote(t *testing.T) {
	assert.Eq(t, `"plain"`, unitQuote("plain"))
	assert.Eq(t, `"a b"`, unitQuote("a b"))
	assert.Eq(t, `"a\\b"`, unitQuote(`a\b`))
	assert.Eq(t, `"q\"uote"`, unitQuote(`q"uote`))
	assert.Eq(t, `"100%%"`, unitQuote("100%"))
	assert.Eq(t, `"$$HOME"`, unitQuote("$HOME"))
}

func TestUnitText(t *testing.T) {
	s := goodSpec()

	t.Run("layout", func(t *testing.T) {
		text := unitText(s)
		lines := strings.Split(text, "\n")
		assert.Eq(t, "# gookit-servicemgr:v1:owner=demo-owner", lines[0])
		assert.StrContains(t, text, "[Service]\nType=simple\n")
		assert.StrContains(t, text, "Restart=on-failure\n")
		assert.StrContains(t, text, "[Install]\nWantedBy=default.target\n")
		assert.StrContains(t, text, "WorkingDirectory="+s.WorkDir+"\n")
	})

	t.Run("round trip with hostile arguments", func(t *testing.T) {
		args := []string{
			"serve", "has space", `q"uote`, `back\slash`, "100%", "$HOME", `${X}`,
			`a;b|c&d`, `'single'`, "", "#not-a-comment", "trail\\",
		}
		sp := s
		sp.Args = args
		sp.Exe = "/opt/with space/demo"
		st := parseUnit(unitText(sp))
		assert.True(t, st.Installed)
		assert.Eq(t, "demo-owner", st.Owner)
		assert.Eq(t, sp.Exe, st.Exe)
		assert.Eq(t, args, st.Args)
	})

	t.Run("a value cannot add directives", func(t *testing.T) {
		sp := s
		sp.Args = []string{`x" ExecStartPost=/bin/sh "y`}
		text := unitText(sp)
		assert.Eq(t, 1, strings.Count(text, "ExecStart="))
		assert.Eq(t, sp.Args, parseUnit(text).Args)
	})
}

func TestParseUnit(t *testing.T) {
	t.Run("unmarked hand written unit", func(t *testing.T) {
		st := parseUnit("[Service]\nExecStart=/bin/true --flag\n")
		assert.True(t, st.Installed)
		assert.Eq(t, "", st.Owner)
		// not in the quoted form this package writes: reported as unknown
		assert.Eq(t, "", st.Exe)
	})

	t.Run("marker must be the first line", func(t *testing.T) {
		st := parseUnit("[Unit]\n# gookit-servicemgr:v1:owner=abc\n")
		assert.Eq(t, "", st.Owner)
	})

	t.Run("other comment", func(t *testing.T) {
		assert.Eq(t, "", parseUnit("# something else\n").Owner)
	})

	t.Run("splitUnitWords rejects unquoted and unterminated", func(t *testing.T) {
		_, ok := splitUnitWords(`bare`)
		assert.False(t, ok)
		_, ok = splitUnitWords(`"open`)
		assert.False(t, ok)
	})
}
