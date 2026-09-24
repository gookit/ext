# TOMLKit

Comment-preserving TOML merging: combine a freshly serialized document with the
file on disk, keeping everything that did not change (comments, key order,
formatting) and applying only the real edits.

> **[中文说明](README.zh-CN.md)**

## Why

Tools that own a config file usually serialize the whole document back to disk.
That drops comments, reorders keys and invents default tables (empty sections,
default values). TOMLKit keeps the untouched parts:

- a table whose decoded content did not change is kept byte for byte;
- inside a changed table, lines of untouched keys (comments included) are kept
  and only changed/new keys are rendered;
- tables removed from the document are dropped whole;
- tables that only hold defaults are not written into a file that never had them.

The same logic applies to INI documents, which share the `[section]` and
`key = value` shape.

## Install

```bash
go get github.com/gookit/ext/tomlkit
```

## Quick start

```go
import (
    "github.com/gookit/ext/tomlkit"
    "github.com/BurntSushi/toml" // or any TOML parser
)

decode := func(text string) (map[string]any, error) {
    data := map[string]any{}
    if _, err := toml.Decode(text, &data); err != nil {
        return nil, err
    }
    return data, nil
}

// rendered is the whole document produced by your serializer
err := tomlkit.MergeFile("app.toml", rendered, tomlkit.Options{
    Decode:   decode,
    Defaults: pristineText, // optional: a pristine config, same serializer
})
if err != nil {
    log.Fatal(err)
}
```

Use `Merge` when you want the text only and write the file yourself:

```go
merged, err := tomlkit.Merge(oldText, renderedText, tomlkit.Options{Decode: decode})
```

## Merge rules

1. Table level: a table that decodes to the same value keeps its original text.
2. Key level: inside a changed table, untouched keys keep their lines, changed
   keys keep their leading and inline comments and only swap the value, and new
   keys come from the rendered document.
3. Removal: tables missing from the rendered document are dropped whole.
4. Defaults: a table that is absent from the file and matches `Defaults` is not
   written (`Defaults` is the pristine document from the same serializer; both
   sides go through the same `Decode`, so the comparison is shape-consistent).
5. Multi-line values: `key = [` … `]` counts as one key — kept as-is when it did
   not change, and replaced (with the comments above it) when it did.
6. Header block: top-level keys such as `paths = [...]` are compared and updated
   like a table, while the comments around them are kept.
7. Fallback: a block that cannot be analysed line by line (sub-tables, inline
   tables) is taken from the rendered document as-is.

## Keeping unknown tables

With `Options.KeepExtraTables` set, tables that exist in the file but not in the
rendered document are kept as they are. That suits config files a user edits by
hand (a project `.xenv.toml`, say). By default those tables are dropped.

## Safety nets

- `MergeFile` decodes the merged result again before writing and falls back to
  the rendered text when it would not parse, so a broken merge cannot corrupt
  the file;
- writes are atomic (temp file in the same directory + rename);
- a missing file is written as-is.

## INI support

`Options.CommentPrefixes` defaults to `#`; pass `"#;"` for INI files.

## Test

```bash
cd tomlkit
go test -v
go test -race -cover
```

## License

MIT
