package tomlkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

// testDecode is a tiny TOML/INI reader for the fixtures below: sections, simple
// "key = value" lines and "#"/";" comments. Keeping it local is the point of
// injecting Decode — the package itself needs no parser dependency.
func testDecode(text string) (map[string]any, error) {
	root := map[string]any{}
	current := root
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = root
			path := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			for _, part := range strings.Split(path, ".") {
				node, ok := current[part].(map[string]any)
				if !ok {
					node = map[string]any{}
					current[part] = node
				}
				current = node
			}
			continue
		}
		index := strings.Index(line, "=")
		if index <= 0 {
			return nil, fmt.Errorf("bad line %q", line)
		}
		key := strings.TrimSpace(line[:index])
		current[key] = testValue(strings.TrimSpace(line[index+1:]))
	}
	return root, nil
}

func testValue(value string) any {
	if index := strings.Index(value, "#"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number
	}
	return value
}

const mergeFixture = `# my config
# second header line
[global]
# keep this comment
target = "/opt/bin" # inline note
user_agent = "eget/1.0"

[packages.fd]
repo = "sharkdp/fd"

[cache_mirror]
enable = false
`

// renderedFixture mirrors what a serializer produces after changing
// global.target and dropping nothing else.
const renderedFixture = `[global]
  target = "/usr/local/bin"
  user_agent = "eget/1.0"

[packages.fd]
repo = "sharkdp/fd"

[cache_mirror]
  enable = false
`

func options(defaults string) Options {
	return Options{Decode: testDecode, Defaults: defaults}
}

func TestMergeKeepsCommentsAndUntouchedTables(t *testing.T) {
	merged, err := Merge(mergeFixture, renderedFixture, options(""))
	assert.NoErr(t, err)

	// File-level comments survive.
	assert.StrContains(t, merged, "# my config")
	assert.StrContains(t, merged, "# second header line")
	// The changed key keeps its own comments and takes the new value.
	assert.StrContains(t, merged, "# keep this comment")
	assert.StrContains(t, merged, `target = "/usr/local/bin"`)
	assert.StrContains(t, merged, "# inline note")
	// An untouched key inside a changed table keeps its original line.
	assert.StrContains(t, merged, "\nuser_agent = \"eget/1.0\"\n")
	// Untouched tables are kept byte for byte.
	assert.StrContains(t, merged, "[packages.fd]\nrepo = \"sharkdp/fd\"")
	assert.StrContains(t, merged, "[cache_mirror]\nenable = false")
}

func TestMergeAddsAndRemovesTables(t *testing.T) {
	rendered := `[packages.bat]
repo = "sharkdp/bat"

[cache_mirror]
  enable = false
`
	merged, err := Merge(mergeFixture, rendered, options(""))
	assert.NoErr(t, err)

	if strings.Contains(merged, "[packages.fd]") {
		t.Fatalf("removed table should not survive:\n%s", merged)
	}
	assert.StrContains(t, merged, "[packages.bat]")
	assert.StrContains(t, merged, "sharkdp/bat")
	assert.StrContains(t, merged, "# my config")
}

func TestMergeSkipsDefaultAndHeaderOnlyTables(t *testing.T) {
	// A serializer renders every known table, including ones that only hold
	// defaults or exist purely as parents.
	rendered := `[global]
  target = "/usr/local/bin"
  user_agent = "eget/1.0"

[packages]
  [packages.fd]
  repo = "sharkdp/fd"

[cache_mirror]
  enable = false

[api_cache]
  enable = true
`
	defaults := "[api_cache]\nenable = true\n"

	merged, err := Merge(mergeFixture, rendered, options(defaults))
	assert.NoErr(t, err)

	assert.False(t, strings.Contains(merged, "[api_cache]"))
	assert.False(t, strings.Contains(merged, "\n[packages]\n"))
	assert.StrContains(t, merged, "[global]")
}

func TestMergeKeepsUnmergeableTableRendered(t *testing.T) {
	// A table this package cannot split line by line (a value continued on the
	// next line) is taken from the rendered document as a whole.
	old := "[global]\nfallbacks = [\"a\",\n  \"b\"]\n"
	rendered := "[global]\n  fallbacks = [\"c\"]\n"
	// The decoder only needs to report that something changed; the block shape is
	// what this test is about.
	lenient := func(text string) (map[string]any, error) {
		return map[string]any{"global": map[string]any{"raw": text}}, nil
	}

	merged, err := Merge(old, rendered, Options{Decode: lenient})
	assert.NoErr(t, err)
	assert.Eq(t, rendered, merged)
}

func TestMergeRequiresDecode(t *testing.T) {
	_, err := Merge("", "", Options{})
	assert.Err(t, err)
	assert.StrContains(t, err.Error(), "Decode is required")
}

func TestMergeSupportsINIStyleComments(t *testing.T) {
	old := "; ini header\n[main]\n; keep me\nport = 8080\nhost = \"localhost\"\n"
	rendered := "[main]\n  port = 9090\n  host = \"localhost\"\n"

	merged, err := Merge(old, rendered, Options{Decode: testDecode, CommentPrefixes: "#;"})
	assert.NoErr(t, err)

	assert.StrContains(t, merged, "; ini header")
	assert.StrContains(t, merged, "; keep me")
	assert.StrContains(t, merged, "port = 9090")
	assert.StrContains(t, merged, "\nhost = \"localhost\"\n")
}

func TestMergeFileWritesAndFallsBack(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "nested", "app.toml")
	assert.NoErr(t, MergeFile(missing, renderedFixture, options("")))
	assert.StrContains(t, readString(t, missing), "[global]")

	// A document that cannot be decoded at all is replaced instead of merged.
	broken := filepath.Join(dir, "broken.toml")
	assert.NoErr(t, os.WriteFile(broken, []byte("this is not a toml document\n"), 0o644))
	assert.NoErr(t, MergeFile(broken, renderedFixture, options("")))
	assert.Eq(t, renderedFixture, readString(t, broken))
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NoErr(t, err)
	return string(data)
}
