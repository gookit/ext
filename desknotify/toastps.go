package desknotify

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf16"
)

const toastAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

const (
	toastExitOK          = 0
	toastExitDisabled    = 3
	toastExitStartFailed = -1
)

const toastSettingPrefix = "setting="
const toastDisabledHint = "Windows 设置 → 系统 → 通知：打开通知总开关与 Windows PowerShell"

const toastScript = `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument
$doc.LoadXml($env:` + toastXMLEnv + `)
$toast = New-Object Windows.UI.Notifications.ToastNotification $doc
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + toastAppID + `')
$setting = [string]$notifier.Setting
if ($setting -ne 'Enabled') {
  Write-Output ('` + toastSettingPrefix + `' + $setting)
  exit 3
}
$notifier.Show($toast)
`

var toastScriptEncoded = encodePSCommand(toastScript)

func toastResult(code int, out string) error {
	out = strings.TrimSpace(out)
	switch code {
	case toastExitOK:
		return nil
	case toastExitDisabled:
		return &SkipError{Reason: toastDisabledReason(toastSettingValue(out))}
	case toastExitStartFailed:
		return fmt.Errorf("powershell toast: %s", out)
	default:
		return fmt.Errorf("powershell toast: exit %d: %s", code, out)
	}
}

func toastSettingValue(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), toastSettingPrefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func toastDisabledReason(setting string) string {
	if setting == "" {
		setting = "not Enabled"
	}
	return "windows notification setting " + setting + " (ToastNotifier.Setting) — " + toastDisabledHint
}

func encodePSCommand(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		buf = append(buf, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}
