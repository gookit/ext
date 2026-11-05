package servicemgr

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestBuildTaskXML(t *testing.T) {
	const user = `HOST\alice`
	s := goodSpec()

	t.Run("security and settings", func(t *testing.T) {
		text, err := buildTaskXML(s, user)
		assert.NoErr(t, err)

		var doc taskDocument
		assert.NoErr(t, xml.Unmarshal([]byte(text), &doc))
		// per-user, no elevation
		assert.Eq(t, user, doc.Principals.Principal.UserID)
		assert.Eq(t, user, doc.Triggers.Logon.UserID)
		assert.Eq(t, "InteractiveToken", doc.Principals.Principal.LogonType)
		assert.Eq(t, "LeastPrivilege", doc.Principals.Principal.RunLevel)
		assert.True(t, doc.Triggers.Logon.Enabled)
		// run settings
		assert.Eq(t, "PT0S", doc.Settings.ExecutionLimit)
		assert.Eq(t, "IgnoreNew", doc.Settings.MultipleInstances)
		assert.StrContains(t, text, "<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>")
		assert.StrContains(t, text, "<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>")
		assert.StrNotContains(t, text, "RestartOnFailure")
		assert.StrNotContains(t, text, "<?xml")
		// ownership marker and action
		assert.Eq(t, "gookit-servicemgr:v1:owner=demo-owner", doc.Registration.Description)
		assert.Eq(t, s.Exe, doc.Actions.Exec.Command)
		assert.Eq(t, s.WorkDir, doc.Actions.Exec.WorkingDirectory)
	})

	t.Run("round trip with hostile arguments", func(t *testing.T) {
		args := []string{
			"serve", "-d", "has space", `q"uote`, `back\slash`, `trail\`, `trail space\`,
			`two\\"quotes`, "100%", "$HOME", `<&>'|;`, "", "tab\tx", `C:\Program Files\x y\`,
		}
		sp := s
		sp.Args = args
		sp.Exe = absPath("Program Files", "demo <&>")
		text, err := buildTaskXML(sp, user)
		assert.NoErr(t, err)

		st, err := parseTaskXML(text)
		assert.NoErr(t, err)
		assert.True(t, st.Installed)
		assert.Eq(t, "demo-owner", st.Owner)
		assert.Eq(t, sp.Exe, st.Exe)
		assert.Eq(t, args, st.Args)
	})

	t.Run("xml markup is escaped", func(t *testing.T) {
		sp := s
		sp.Args = []string{`</Arguments><Exec><Command>evil</Command></Exec>`}
		text, err := buildTaskXML(sp, user)
		assert.NoErr(t, err)
		assert.StrNotContains(t, text, "<Command>evil</Command>")
		assert.Eq(t, 1, strings.Count(text, "<Exec>"))
	})
}

func TestParseTaskXML(t *testing.T) {
	t.Run("utf-16 declaration and unmarked task", func(t *testing.T) {
		text := `<?xml version="1.0" encoding="UTF-16"?>
<Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><RegistrationInfo><Description>hand made</Description></RegistrationInfo>
<Actions><Exec><Command>"C:\x\a.exe"</Command><Arguments>-a "b c"</Arguments></Exec></Actions></Task>`
		st, err := parseTaskXML(text)
		assert.NoErr(t, err)
		assert.True(t, st.Installed)
		assert.Eq(t, "", st.Owner)
		assert.Eq(t, `C:\x\a.exe`, st.Exe)
		assert.Eq(t, []string{"-a", "b c"}, st.Args)
	})

	t.Run("task without description has no owner", func(t *testing.T) {
		st, err := parseTaskXML(`<Task><Actions><Exec><Command>x</Command></Exec></Actions></Task>`)
		assert.NoErr(t, err)
		assert.Eq(t, "", st.Owner)
	})

	t.Run("errors", func(t *testing.T) {
		_, err := parseTaskXML("<Task><unclosed>")
		assert.Err(t, err)
		_, err = parseTaskXML(`<?xml version="1.0"`)
		assert.Err(t, err)
		_, err = parseTaskXML(`<Task><Actions><Exec><Arguments>"open</Arguments></Exec></Actions></Task>`)
		assert.Err(t, err)
	})
}

func TestWinArgs(t *testing.T) {
	cases := [][]string{
		nil,
		{"a"},
		{"a b", "c"},
		{`\`},
		{`a\`, `b`},
		{`a b\`},
		{`"`},
		{`\"`},
		{`\\"`},
		{``},
		{``, ``},
	}
	for _, args := range cases {
		got, err := splitWinArgs(composeWinArgs(args))
		assert.NoErr(t, err)
		if len(args) == 0 {
			assert.Eq(t, 0, len(got))
			continue
		}
		assert.Eq(t, args, got)
	}
	assert.Eq(t, `"a b" c`, composeWinArgs([]string{"a b", "c"}))
}
