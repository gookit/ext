# Gookit Ext Packages

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/gookit/ext?style=flat-square)
[![Go Report Card](https://goreportcard.com/badge/github.com/gookit/ext)](https://goreportcard.com/report/github.com/gookit/ext)
[![Go Reference](https://pkg.go.dev/badge/github.com/gookit/ext.svg)](https://pkg.go.dev/github.com/gookit/ext)

> **[EN README](README.md)**

`gookit/ext` 提供的一些常用的带有使用场景的工具包。如果需要使用基础工具包，请查看使用 [gookit/goutil](https://github.com/gookit/goutil)

## Install

```bash
go get github.com/gookit/goutil/ext
```

## Package: lcache 

`lcache` 简单、协程安全的本地内存缓存包，提供 TTL 过期支持、LRU 淘汰策略和文件持久化功能。

> [!NOTE]
> 更多信息请查看: [./lcache](./lcache/README.md)

## Package: tomlkit

带注释保留的 TOML/INI 文档合并：把重新序列化后的完整文档与磁盘上的原文件合并，未变更的部分保留原文（注释、键顺序、格式），只应用真正的改动。

> [!NOTE]
> 更多信息请查看: [./tomlkit](./tomlkit/README.zh-CN.md)

## License

MIT