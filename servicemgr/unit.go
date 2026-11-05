package servicemgr

import (
	"fmt"
	"strings"
)

// This file holds the pure systemd unit generation and parsing. It has no build tag
// so the quoting rules are tested on every platform; only the systemctl calls live
// in systemd_linux.go.

// unitQuote wraps one ExecStart word in double quotes for systemd. Inside quotes
// systemd honours C-style escapes, so backslash and quote are escaped; "%" starts a
// specifier and "$" an environment substitution, so both are doubled to stay literal.
func unitQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '%':
			b.WriteString("%%")
		case '$':
			b.WriteString("$$")
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// unitText renders the unit for a validated spec. The first line is the ownership
// marker comment.
func unitText(s Spec) string {
	words := make([]string, 0, len(s.Args)+1)
	words = append(words, unitQuote(s.Exe))
	for _, a := range s.Args {
		words = append(words, unitQuote(a))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", ownerMarker(s.Owner))
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n\n", s.Name)
	b.WriteString("[Service]\nType=simple\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(words, " "))
	// WorkingDirectory is one raw value to the end of the line; only "%" needs escaping.
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", strings.ReplaceAll(s.WorkDir, "%", "%%"))
	b.WriteString("Restart=on-failure\n\n")
	b.WriteString("[Install]\nWantedBy=default.target\n")
	return b.String()
}

// parseUnit reads back a unit written by unitText. Owner is "" when the first line
// is not a marker; Exe and Args stay empty when ExecStart is absent or not in the
// quoted form this package writes (e.g. a hand-written unit).
func parseUnit(text string) Status {
	st := Status{Installed: true}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if strings.HasPrefix(first, "#") {
			st.Owner = parseOwnerMarker(strings.TrimPrefix(first, "#"))
		}
	}
	for _, ln := range lines {
		if rest, ok := strings.CutPrefix(ln, "ExecStart="); ok {
			if words, ok := splitUnitWords(rest); ok && len(words) > 0 {
				st.Exe, st.Args = words[0], words[1:]
			}
			break
		}
	}
	return st
}

// splitUnitWords is the inverse of unitQuote for a space separated list of quoted
// words. It reports false on any input outside that form.
func splitUnitWords(s string) ([]string, bool) {
	var words []string
	i := 0
	for i < len(s) {
		if s[i] == ' ' {
			i++
			continue
		}
		if s[i] != '"' {
			return nil, false
		}
		i++
		var b strings.Builder
		closed := false
		for i < len(s) && !closed {
			switch c := s[i]; {
			case c == '\\' && i+1 < len(s):
				b.WriteByte(s[i+1])
				i += 2
			case c == '%' && i+1 < len(s) && s[i+1] == '%':
				b.WriteByte('%')
				i += 2
			case c == '$' && i+1 < len(s) && s[i+1] == '$':
				b.WriteByte('$')
				i += 2
			case c == '"':
				closed = true
				i++
			default:
				b.WriteByte(c)
				i++
			}
		}
		if !closed {
			return nil, false
		}
		words = append(words, b.String())
	}
	return words, true
}
