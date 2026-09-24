// Package tomlkit merges TOML documents while keeping everything that did not
// change byte for byte.
//
// It exists for tools that own a configuration file and write it back through a
// serializer: re-serializing the whole document loses comments, reorders keys
// and re-invents default tables. Merge takes the freshly rendered document plus
// the file on disk and keeps the original text for everything that still means
// the same.
//
// The same logic applies to INI documents, which share the "[section]" header
// and "key = value" line shape.
package tomlkit

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// Decode parses a document into nested data. Callers inject their own parser
// (BurntSushi/toml, gookit/config, ...); it is what lets Merge tell whether a
// table really changed.
type Decode func(text string) (map[string]any, error)

// Options configures a merge.
type Options struct {
	// Decode parses both the old and the rendered document. Required.
	Decode Decode
	// Defaults is a pristine document rendered by the same serializer as
	// renderedText. A table that is missing from the file but equal to its
	// Defaults entry consists of defaults only and is not written into that file.
	// Comparing it through the same Decode keeps both sides in the same shape.
	Defaults string
	// CommentPrefixes lists the characters that start a line comment. Empty means
	// "#" (TOML); use "#;" for INI files.
	CommentPrefixes string
	// KeepExtraTables keeps tables that exist in the file but not in the rendered
	// document, instead of dropping them. Use it when the file may hold user
	// content the serializer does not know about.
	KeepExtraTables bool
}

func (o Options) commentPrefixes() string {
	if o.CommentPrefixes == "" {
		return "#"
	}
	return o.CommentPrefixes
}

// block is one textual chunk of a document: the keys before the first table
// header (name "") or one table with everything that follows it.
type block struct {
	name string
	text string
}

// Merge returns the merged document:
//
//   - a table whose decoded content did not change keeps the old text byte for
//     byte, comments and formatting included;
//   - inside a changed table, lines of untouched keys (with the comments around
//     them) are kept and only changed/new keys come from the rendered document;
//   - tables that are gone from the rendered document are dropped whole;
//   - tables that only hold defaults are not added to a file that never had them.
func Merge(oldText, renderedText string, opts Options) (string, error) {
	if opts.Decode == nil {
		return "", errors.New("tomlkit: Options.Decode is required")
	}
	oldData, err := opts.Decode(oldText)
	if err != nil {
		return "", err
	}
	newData, err := opts.Decode(renderedText)
	if err != nil {
		return "", err
	}
	// The defaults go through the same decoder, so "equal to the defaults" is a
	// meaningful comparison rather than a shape mismatch.
	var defaultData map[string]any
	if opts.Defaults != "" {
		if defaultData, err = opts.Decode(opts.Defaults); err != nil {
			return "", err
		}
	}

	prefixes := opts.commentPrefixes()
	renderedBlocks := splitBlocks(renderedText, prefixes)
	renderedByName := make(map[string]string, len(renderedBlocks))
	for _, item := range renderedBlocks {
		renderedByName[item.name] = item.text
	}

	var out strings.Builder
	kept := make(map[string]bool, len(renderedBlocks))
	for _, item := range splitBlocks(oldText, prefixes) {
		next, exists := renderedByName[item.name]
		if !exists {
			// The table left the document. The header block (keys and comments
			// before the first [table]) has no counterpart in a rendered document,
			// so it is always kept; an unknown table is kept on request.
			if item.name == "" || opts.KeepExtraTables {
				out.WriteString(item.text)
			}
			continue
		}
		kept[item.name] = true
		if reflect.DeepEqual(tableData(oldData, item.name), tableData(newData, item.name)) {
			out.WriteString(item.text)
			continue
		}
		out.WriteString(mergeTable(item.text, next, prefixes))
	}

	for _, item := range renderedBlocks {
		if kept[item.name] {
			continue
		}
		// A header-only table (its content lives in sub-tables such as
		// [packages.fd]) is not worth writing on its own.
		if !hasKeyLines(item.text, prefixes) {
			continue
		}
		if reflect.DeepEqual(tableData(defaultData, item.name), tableData(newData, item.name)) {
			continue
		}
		out.WriteString(item.text)
	}
	return out.String(), nil
}

// MergeFile reads path, merges it with renderedText and writes the result back
// atomically. A missing document is written as-is, and a merge that fails (or
// that would not decode again) falls back to the rendered text, so a broken
// merge cannot corrupt the file.
func MergeFile(path, renderedText string, opts Options) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeFile(path, renderedText)
		}
		return err
	}

	merged, err := Merge(string(existing), renderedText, opts)
	if err != nil {
		return writeFile(path, renderedText)
	}
	// Never write a document that would not parse back.
	if _, err := opts.Decode(merged); err != nil {
		return writeFile(path, renderedText)
	}
	return writeFile(path, merged)
}

// mergeTable keeps the original lines of keys whose value stayed the same (and
// the comments around them) and takes changed or new keys from the rendered
// table. A key that only exists in the old table was removed and is dropped.
// Anything that cannot be analysed line by line falls back to the rendered text.
func mergeTable(oldText, newText, prefixes string) string {
	oldTable, okOld := parseKeyLines(oldText, prefixes)
	newTable, okNew := parseKeyLines(newText, prefixes)
	if !okOld || !okNew {
		return newText
	}

	var out strings.Builder
	out.WriteString(oldTable.header)
	for _, key := range newTable.order {
		line := newTable.keys[key]
		old, existed := oldTable.keys[key]
		switch {
		case existed && keyValue(old.line, prefixes) == keyValue(line.line, prefixes):
			// Untouched key: original text, comment and all.
			out.WriteString(old.leading)
			out.WriteString(old.line)
		case existed:
			// The value changed: keep the comments that belonged to the key and
			// swap only the value line in.
			out.WriteString(old.leading)
			out.WriteString(withInlineComment(line.line, old.line, prefixes))
		default:
			// New key.
			out.WriteString(line.leading)
			out.WriteString(line.line)
		}
	}
	return out.String()
}

// withInlineComment carries a trailing " # note" over from the previous version
// of the line, if it had one.
func withInlineComment(newLine, oldLine, prefixes string) string {
	comment, ok := inlineComment(oldLine, prefixes)
	if !ok {
		return newLine
	}
	body := strings.TrimRight(newLine, "\r\n")
	suffix := "\n"
	if strings.HasSuffix(newLine, "\r\n") {
		suffix = "\r\n"
	}
	return body + " " + comment + suffix
}

type keyLine struct {
	leading string
	line    string
}

type table struct {
	header string
	order  []string
	keys   map[string]keyLine
}

// parseKeyLines splits a table into its header line and one entry per
// "key = value" pair, keeping a value that continues on the following lines (a
// multi-line array, for instance) with its key. It reports false when a line
// cannot be handled that way, because the caller must then avoid merging.
func parseKeyLines(text, prefixes string) (table, bool) {
	parsed := table{keys: map[string]keyLine{}}
	var leading strings.Builder
	headerSeen := false
	lines := strings.SplitAfter(text, "\n")

	for index := 0; index < len(lines); index++ {
		raw := lines[index]
		trimmed := strings.TrimSpace(raw)
		switch {
		case !headerSeen && strings.HasPrefix(trimmed, "["):
			parsed.header += raw
			headerSeen = true
			continue
		case trimmed == "" || isComment(trimmed, prefixes):
			leading.WriteString(raw)
			continue
		}

		key, ok := simpleKeyName(trimmed)
		if !ok {
			return table{}, false
		}
		block := raw
		for unbalancedValue(keyValue(block, prefixes)) {
			if index+1 >= len(lines) {
				return table{}, false
			}
			index++
			block += lines[index]
		}
		parsed.order = append(parsed.order, key)
		parsed.keys[key] = keyLine{leading: leading.String(), line: block}
		leading.Reset()
	}
	return parsed, true
}

func simpleKeyName(line string) (string, bool) {
	index := strings.Index(line, "=")
	if index <= 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:index])
	if key == "" || strings.ContainsAny(key, " \t\"'") {
		return "", false
	}
	return key, true
}

// keyValue returns everything after the first "=" of a key block with a trailing
// comment removed, so callers can compare values or see whether a value is
// finished.
func keyValue(block, prefixes string) string {
	index := strings.Index(block, "=")
	if index <= 0 {
		return strings.TrimSpace(block)
	}
	value := strings.TrimSpace(block[index+1:])
	if comment, ok := inlineComment(value, prefixes); ok {
		value = strings.TrimSpace(strings.TrimSuffix(value, comment))
	}
	return value
}

// inlineComment returns the trailing comment of a line, ignoring comment
// characters inside quoted values.
func inlineComment(line, prefixes string) (string, bool) {
	inString := false
	var quote byte
	for index := 0; index < len(line); index++ {
		char := line[index]
		if inString {
			if char == quote {
				inString = false
			}
			continue
		}
		switch {
		case char == '"' || char == '\'':
			inString = true
			quote = char
		case strings.IndexByte(prefixes, char) >= 0:
			return strings.TrimSpace(line[index:]), true
		}
	}
	return "", false
}

func isComment(line, prefixes string) bool {
	return len(line) > 0 && strings.IndexByte(prefixes, line[0]) >= 0
}

// unbalancedValue reports a value that looks like it continues on another line
// (an unterminated array, inline table or string).
func unbalancedValue(value string) bool {
	depth := 0
	inString := false
	var quote byte
	for index := 0; index < len(value); index++ {
		char := value[index]
		if inString {
			if char == quote {
				inString = false
			}
			continue
		}
		switch char {
		case '"', '\'':
			inString = true
			quote = char
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		}
	}
	return inString || depth != 0
}

// splitBlocks splits a document at table headers, so each block can be kept or
// replaced as a unit.
func splitBlocks(text, prefixes string) []block {
	blocks := []block{}
	var current strings.Builder
	name := ""
	flush := func() {
		if current.Len() > 0 {
			blocks = append(blocks, block{name: name, text: current.String()})
			current.Reset()
		}
	}

	for _, line := range strings.SplitAfter(text, "\n") {
		if header, ok := headerName(strings.TrimSpace(line)); ok {
			flush()
			name = header
		}
		current.WriteString(line)
	}
	flush()
	return blocks
}

func headerName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	// Arrays of tables and INI's inline comments after a header are not handled.
	if strings.HasPrefix(line, "[[") {
		return "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
	if inner == "" {
		return "", false
	}
	return inner, true
}

// hasKeyLines reports whether a rendered block holds any key line below its
// header.
func hasKeyLines(text, prefixes string) bool {
	headerSeen := false
	for _, raw := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(raw)
		if !headerSeen && strings.HasPrefix(trimmed, "[") {
			headerSeen = true
			continue
		}
		if trimmed == "" || isComment(trimmed, prefixes) {
			continue
		}
		return true
	}
	return false
}

// tableData returns the data a block covers: for a table path that is simply the
// table, for the header block (empty path) the top-level keys, since sub-tables
// are blocks of their own.
func tableData(data map[string]any, path string) any {
	if path != "" {
		return lookupPath(data, path)
	}
	scalars := map[string]any{}
	for key, value := range data {
		if _, isTable := value.(map[string]any); isTable {
			continue
		}
		scalars[key] = value
	}
	return scalars
}

// lookupPath walks a dotted table path ("packages.fd") through decoded data.
func lookupPath(data map[string]any, path string) any {
	if path == "" || data == nil {
		return nil
	}
	var current any = data
	for _, part := range strings.Split(path, ".") {
		node, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = node[part]
		if !ok {
			return nil
		}
	}
	return current
}

// writeFile replaces path with content in one step: a temp file in the same
// directory is renamed over the target.
func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
