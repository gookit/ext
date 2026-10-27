# localauth

Local bearer-token authentication for HTTP services.

## Features

- Generates 32-byte cryptographically random tokens encoded with unpadded base64url.
- Reads and writes token files with owner-only `0600` permissions on Unix-like systems.
- Requires `Authorization: Bearer <token>`; query parameters and cookies are never accepted.
- Checks `Host` against `127.0.0.1:<port>` and `localhost:<port>` and rejects unexpected `Origin` values.
- Compares token digests with `subtle.ConstantTimeCompare`.

## Quick start

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

`WriteToken` uses `0600` on Unix-like systems. Windows mode bits do not represent
full ACLs, so keep the token below the user's profile and rely on the profile's
default ACL; callers should verify that ACL if the deployment has multiple local
users.

## Testing

```bash
go test ./localauth/
```

## License

MIT
