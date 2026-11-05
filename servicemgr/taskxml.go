package servicemgr

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// This file holds the pure Task Scheduler XML and Windows command-line handling.
// It has no build tag so the generation and parsing rules are tested on every
// platform; only the COM calls live in the windows-only files.

// taskDocument models the subset of the Task Scheduler 1.4 schema that is written
// and read back. Settings booleans are not omitempty on purpose: an explicit
// false is what turns the battery restrictions off.
type taskDocument struct {
	XMLName      xml.Name `xml:"Task"`
	XMLNS        string   `xml:"xmlns,attr"`
	Version      string   `xml:"version,attr"`
	Registration struct {
		Description string `xml:"Description"`
		URI         string `xml:"URI"`
	} `xml:"RegistrationInfo"`
	Triggers struct {
		Logon struct {
			Enabled bool   `xml:"Enabled"`
			UserID  string `xml:"UserId"`
		} `xml:"LogonTrigger"`
	} `xml:"Triggers"`
	Principals struct {
		Principal struct {
			ID        string `xml:"id,attr"`
			UserID    string `xml:"UserId"`
			LogonType string `xml:"LogonType"`
			RunLevel  string `xml:"RunLevel"`
		} `xml:"Principal"`
	} `xml:"Principals"`
	Settings struct {
		MultipleInstances string `xml:"MultipleInstancesPolicy"`
		DisallowBattery   bool   `xml:"DisallowStartIfOnBatteries"`
		StopOnBattery     bool   `xml:"StopIfGoingOnBatteries"`
		ExecutionLimit    string `xml:"ExecutionTimeLimit"`
	} `xml:"Settings"`
	Actions struct {
		Context string `xml:"Context,attr"`
		Exec    struct {
			Command          string `xml:"Command"`
			Arguments        string `xml:"Arguments"`
			WorkingDirectory string `xml:"WorkingDirectory"`
		} `xml:"Exec"`
	} `xml:"Actions"`
}

// buildTaskXML renders the task for a validated spec. user is the account the logon
// trigger and the principal are bound to (the current user, DOMAIN\name form).
// There is deliberately no RestartOnFailure element: a failed start is not retried.
func buildTaskXML(s Spec, user string) (string, error) {
	var doc taskDocument
	doc.XMLNS = "http://schemas.microsoft.com/windows/2004/02/mit/task"
	doc.Version = "1.4"
	doc.Registration.Description = ownerMarker(s.Owner)
	doc.Registration.URI = `\` + s.Name
	doc.Triggers.Logon.Enabled = true
	doc.Triggers.Logon.UserID = user
	doc.Principals.Principal.ID = "Author"
	doc.Principals.Principal.UserID = user
	doc.Principals.Principal.LogonType = "InteractiveToken"
	doc.Principals.Principal.RunLevel = "LeastPrivilege"
	doc.Settings.MultipleInstances = "IgnoreNew"
	doc.Settings.DisallowBattery = false
	doc.Settings.StopOnBattery = false
	doc.Settings.ExecutionLimit = "PT0S" // no time limit
	doc.Actions.Context = "Author"
	doc.Actions.Exec.Command = s.Exe
	doc.Actions.Exec.Arguments = composeWinArgs(s.Args)
	doc.Actions.Exec.WorkingDirectory = s.WorkDir
	// encoding/xml escapes every text field. The result carries no XML declaration:
	// RegisterTask takes a UTF-16 BSTR and a UTF-8 declaration makes it reject the
	// document as malformed.
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("servicemgr: build task xml: %w", err)
	}
	return string(data), nil
}

// parseTaskXML turns the XML a task returns into a Status. Task Scheduler hands the
// document back with a UTF-16 declaration, which is stripped because the BSTR was
// already decoded to a Go string.
func parseTaskXML(xmlText string) (Status, error) {
	trimmed := strings.TrimSpace(xmlText)
	if strings.HasPrefix(trimmed, "<?xml") {
		end := strings.Index(trimmed, "?>")
		if end < 0 {
			return Status{}, fmt.Errorf("servicemgr: invalid task xml declaration")
		}
		trimmed = trimmed[end+2:]
	}
	var doc taskDocument
	if err := xml.Unmarshal([]byte(trimmed), &doc); err != nil {
		return Status{}, fmt.Errorf("servicemgr: decode task xml: %w", err)
	}
	args, err := splitWinArgs(doc.Actions.Exec.Arguments)
	if err != nil {
		return Status{}, err
	}
	return Status{
		Installed: true,
		Owner:     parseOwnerMarker(doc.Registration.Description),
		Exe:       strings.Trim(strings.TrimSpace(doc.Actions.Exec.Command), `"`),
		Args:      args,
	}, nil
}

// composeWinArgs joins args into one Windows command-line string using the
// CommandLineToArgvW rules (the same quoting as syscall.EscapeArg): an argument is
// quoted when it is empty or holds a space, tab or quote, quotes are backslash
// escaped, and backslashes directly before a quote are doubled.
func composeWinArgs(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(escapeWinArg(a))
	}
	return b.String()
}

func escapeWinArg(s string) string {
	if s == "" {
		return `""`
	}
	needQuote := strings.ContainsAny(s, " \t\"")
	if !needQuote {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			slashes++
			b.WriteByte(c)
		case c == '"':
			// n pending backslashes were already written; write n more (2n) and then
			// \" so the quote is literal and the earlier n backslashes stay literal
			for ; slashes > 0; slashes-- {
				b.WriteByte('\\')
			}
			b.WriteString(`\"`)
		default:
			slashes = 0
			b.WriteByte(c)
		}
	}
	// backslashes before the closing quote must be doubled too
	for ; slashes > 0; slashes-- {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}

// splitWinArgs is the inverse of composeWinArgs (CommandLineToArgvW rules for an
// argument string that does not include the program name).
func splitWinArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inQuote, have := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			n := 0
			for i < len(s) && s[i] == '\\' {
				n++
				i++
			}
			if i < len(s) && s[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 {
					cur.WriteByte('"') // escaped quote
				} else {
					inQuote = !inQuote
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
				i-- // reprocess the non-backslash byte
			}
			have = true
		case c == '"':
			inQuote = !inQuote
			have = true
		case (c == ' ' || c == '\t') && !inQuote:
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("servicemgr: unterminated quote in task arguments")
	}
	if have {
		args = append(args, cur.String())
	}
	return args, nil
}
