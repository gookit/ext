# localauth

用于 HTTP 服务的本机 Bearer token 认证。

## 功能特性

- 使用 32 字节密码学安全随机数，编码为无填充 base64url token。
- 读写 token 文件时使用 Unix-like 系统的 owner-only `0600` 权限。
- 只接受 `Authorization: Bearer <token>`，不接受查询参数或 Cookie。
- 校验 `Host` 是否为 `127.0.0.1:<port>` 或 `localhost:<port>`，并拒绝不在白名单中的 `Origin`。
- 对 token 摘要使用 `subtle.ConstantTimeCompare` 比较。

## 快速开始

```go
token, err := localauth.GenerateToken()
if err != nil {
	return err
}
if err := localauth.WriteToken("/path/to/auth/token", token); err != nil {
	return err
}

handler := localauth.Middleware(localauth.Options{
	Token: token,
	Port: 43127,
}, applicationHandler)
```

`WriteToken` 在 Unix-like 系统使用 `0600`。Windows 的 mode bits 不表示完整
ACL，因此应把 token 放在用户目录下并依赖用户目录默认 ACL；如果机器有多个本地
用户，调用方应额外核验 ACL。

## 运行测试

```bash
go test ./localauth/
```

## 许可证

MIT
