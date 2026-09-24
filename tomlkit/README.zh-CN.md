# TOMLKit

带注释保留的 TOML 文档合并：把"重新序列化后的完整文档"与"磁盘上的原文件"合并，未变更的部分保留原文（注释、键顺序、格式），只应用真正的改动。

> **[English README](README.md)**

## 为什么需要

工具类程序通常把配置整体序列化后写回。这一步会丢掉注释、重排键顺序，并把默认配置表（空 section、默认值）凭空写进文件。TOMLKit 只保留未变更的部分：

- 解析内容未变的表：整块保留原文；
- 变更的表：内部未变的键行（连同注释）原样保留，只有变更/新增的键重新渲染；
- 从文档中移除的表：整块删除；
- 只包含默认值的表：不会写进本来没有它的文件。

同一套逻辑也适用于 INI 文档（同为 `[section]` + `key = value` 结构）。

## 安装

```bash
go get github.com/gookit/ext/tomlkit
```

## 快速开始

```go
import (
    "github.com/gookit/ext/tomlkit"
    "github.com/BurntSushi/toml" // 或任何 TOML 解析器
)

decode := func(text string) (map[string]any, error) {
    data := map[string]any{}
    if _, err := toml.Decode(text, &data); err != nil {
        return nil, err
    }
    return data, nil
}

// rendered 是把当前配置整体序列化后的文本
err := tomlkit.MergeFile("app.toml", rendered, tomlkit.Options{
    Decode:   decode,
    Defaults: pristineText, // 可选：默认配置序列化后的文本
})
if err != nil {
    log.Fatal(err)
}
```

只想拿到合并结果、自己负责写盘时用 `Merge`：

```go
merged, err := tomlkit.Merge(oldText, renderedText, tomlkit.Options{Decode: decode})
```

## 合并规则

1. 表级：解码后内容相同的表整块保留原文，注释与格式都在。
2. 键级：表发生变化时逐键处理——未变的键保留原行，变更的键保留其前置注释与行内注释、只替换值，新增的键使用渲染结果。
3. 删除：渲染结果中不存在的表会被整体移除。
4. 默认抑制：文件里本来没有、且内容与 `Defaults` 一致的表不会写入（`Defaults` 请传**同一序列化器**产出的默认配置文本，两边会走同一个 `Decode` 比较）。
5. 跨行值：`key = [` … `]` 这类跨行值视为一个键——值未变则原文照留，变了只替换该键并保留它上方的注释。
6. 文件头部：顶层键（如 `paths = [...]`）同样参与比较与更新，头部注释保留。
7. 降级：无法逐行分析的块（子表、内联表等）该表整体使用渲染结果。

## 保留未知表

`Options.KeepExtraTables` 为 `true` 时，文件里存在、但渲染结果中没有的表会原样保留，适用于用户会在配置文件里写自定义 section 的场景（如项目内的 `.xenv.toml`）。默认为 `false`，即删除这些表。

## 安全网

- `MergeFile` 在写入前会用 `Decode` 再解析一次合并结果，失败则退化为直接写入渲染文本，因此不会写出损坏的配置；
- 写入是原子的：同目录临时文件 + rename；
- 原文件不存在时直接写入渲染结果。

## INI 支持

`Options.CommentPrefixes` 默认为 `#`；INI 文件传 `"#;"`。

## 测试

```bash
cd tomlkit
go test -v
go test -race -cover
```

## 许可证

MIT
