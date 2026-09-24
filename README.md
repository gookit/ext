# Gookit Ext Packages

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/gookit/ext?style=flat-square)
[![Go Report Card](https://goreportcard.com/badge/github.com/gookit/ext)](https://goreportcard.com/report/github.com/gookit/ext)
[![Go Reference](https://pkg.go.dev/badge/github.com/gookit/ext.svg)](https://pkg.go.dev/github.com/gookit/ext)

> **[中文说明](README.zh-CN.md)**

`gookit/ext` provides some commonly used toolkits with usage scenarios. If you need to use the basic toolkit, please see [gookit/goutil](https://github.com/gookit/goutil)

## Install

```bash
go get github.com/gookit/goutil/ext
```

## Package: lcache 

`lcache` provides a simple, goroutine-safe local cache implementation with TTL support.

> [!NOTE]
> More information please see: [./lcache](./lcache/README.md)

## Package: tomlkit

Comment-preserving TOML/INI merging: combine a freshly serialized document with
the file on disk, keeping untouched comments, key order and formatting.

> [!NOTE]
> More information please see: [./tomlkit](./tomlkit/README.md)


## License

MIT
