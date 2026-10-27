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

## Package: daemon

`daemon` provides cross-platform detached process startup, graceful stop signaling,
pid-file ownership and stale-process protection for local daemons.

> [!NOTE]
> More information please see [./daemon](./daemon).

## Package: procattr

`procattr` configures non-interactive child processes to avoid opening a console
window on Windows; it is a no-op on other platforms.

> [!NOTE]
> More information please see [./procattr](./procattr).

## Package: desknotify

`desknotify` sends best-effort desktop notifications using the platform's native helper and degrades cleanly when no desktop is available.

> [!NOTE]
> More information please see [./desknotify](./desknotify).

## Package: tomlkit

Comment-preserving TOML/INI merging: combine a freshly serialized document with
the file on disk, keeping untouched comments, key order and formatting.

> [!NOTE]
> More information please see: [./tomlkit](./tomlkit/README.md)


## Package: localauth

`localauth` provides local bearer-token authentication, token-file storage, and Host/Origin checks for HTTP services.

> [!NOTE]
> More information please see: [./localauth](./localauth/README.md)

## Package: imbot

One-way notifications for generic webhooks, DingTalk and Feishu, with allowlisted
public targets, provider signatures, bounded timeout, one retry, and business-error
classification.

> [!NOTE]
> More information please see: [./imbot](./imbot/README.md)

## License

MIT
