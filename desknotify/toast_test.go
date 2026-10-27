package desknotify

import (
	"errors"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestToastXML(t *testing.T) {
	xml := toastXML(Notification{Title: `A & B <C>`, Body: "line one\nline two"})
	assert.Contains(t, xml, `<text>A &amp; B &lt;C&gt;</text>`)
	assert.Contains(t, xml, `<text>line one</text>`)
	assert.Contains(t, xml, `<text>line two</text>`)
}

func TestToastResult(t *testing.T) {
	assert.NoError(t, toastResult(toastExitOK, ""))

	err := toastResult(toastExitDisabled, "setting=DisabledForUser")
	var skip *SkipError
	assert.True(t, errors.As(err, &skip))
	assert.Contains(t, skip.Reason, "DisabledForUser")

	err = toastResult(2, "boom")
	assert.Error(t, err)
	assert.False(t, errors.As(err, &skip))
	assert.Contains(t, err.Error(), "exit 2")
}

func TestToastScriptUsesEnvironmentXML(t *testing.T) {
	assert.Contains(t, toastScript, "$env:"+toastXMLEnv)
	assert.Contains(t, toastScript, "$notifier.Setting")
	assert.True(t, strings.Index(toastScript, "$notifier.Setting") < strings.Index(toastScript, "$notifier.Show("))
}
