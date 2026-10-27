package desknotify

import "strings"

const toastXMLEnv = "DESKNOTIFY_TOAST_XML"

func toastXML(n Notification) string {
	var b strings.Builder
	b.WriteString(`<toast><visual><binding template="ToastGeneric">`)
	b.WriteString(`<text>`)
	b.WriteString(xmlText(clampRunes(n.Title, 120)))
	b.WriteString(`</text>`)
	written := 0
	for _, line := range strings.Split(n.Body, "\n") {
		if written >= 3 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(`<text>`)
		b.WriteString(xmlText(clampRunes(line, 240)))
		b.WriteString(`</text>`)
		written++
	}
	b.WriteString(`</binding></visual></toast>`)
	return b.String()
}

func xmlText(s string) string { return xmlEscape(s) }

func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\n', '\r', '\t':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
