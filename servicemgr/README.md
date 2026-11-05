# servicemgr

Register a per-user "start at login" entry for a program, start it once on demand,
query it, and remove it. Current-user level only: no elevation, no system services.
There is no `Stop`; stop the program through its own graceful-stop path (a systemd
unit that exits normally is not restarted).

```go
spec := servicemgr.Spec{
    Name:    "my-daemon",                 // ^[a-z][a-z0-9-]{0,63}$
    Owner:   "my-app",                    // same charset, written to the ownership marker
    Exe:     "/opt/my-app/bin/daemon",    // absolute path
    Args:    []string{"serve", "--home", "/home/me/.my-app"}, // never passed through a shell
    WorkDir: "/home/me/.my-app",          // absolute path
}
err := servicemgr.Register(ctx, spec)             // create, or update when the owner matches
err = servicemgr.Start(ctx, "my-daemon", "my-app")  // run once now
st, err := servicemgr.Query(ctx, "my-daemon")     // Installed, Owner, Exe, Args
err = servicemgr.Unregister(ctx, "my-daemon", "my-app")
```

## Behaviour

| Call | Result |
|---|---|
| `Register` | missing: create; same owner: update; other owner or no marker: `ErrOwnedByOther` |
| `Unregister`, `Start` | missing: `ErrNotInstalled`; other owner or no marker: `ErrOwnedByOther` |
| `Query` | missing: `Status{Installed: false}, nil`; present without marker: `Installed: true, Owner: ""` |
| invalid name, owner, path, or a newline/NUL in any field | `ErrInvalidSpec` |
| no backend (other OS, or no systemd user bus) | `ErrUnsupported` |

The ownership marker text is `gookit-servicemgr:v1:owner=<owner>`: the task description
on Windows, the first comment line of the unit on Linux.

## Windows

A Task Scheduler task (registered through COM, no `schtasks` or PowerShell):

- logon trigger and principal bound to the current user (`LogonType=InteractiveToken`,
  `RunLevel=LeastPrivilege`);
- `ExecutionTimeLimit=PT0S` (no limit), runs on battery and is not stopped when switching
  to battery, no restart on failure, `MultipleInstancesPolicy=IgnoreNew`;
- `Start` runs the task immediately.

The action runs a console program, so a console window may flash when the task starts.

## Linux

A unit `$XDG_CONFIG_HOME/systemd/user/<name>.service` (default `~/.config/systemd/user`):
`Type=simple`, `Restart=on-failure`, `WantedBy=default.target`; `ExecStart` words are quoted
and `\`, `"`, `%`, `$` escaped. All commands are `systemctl --user ...` run without a shell.

- Every call first runs `systemctl --user show-environment`; if it fails (for example WSL
  without systemd) the call returns `ErrUnsupported` and writes nothing.
- A user unit stops when the user's last session ends unless lingering is on:
  `loginctl enable-linger $USER`.
- `Register` runs `daemon-reload` and `enable`; if either fails the previous unit file is
  restored (or the new one removed).

## Tests

Pure parts (validation, XML and unit generation, quoting, ownership parsing) run on every
platform. The real Windows round trip registers random `gksm-test-<hex>` tasks and only
runs with `GOOKIT_SERVICEMGR_REAL=1`:

```bash
GOOKIT_SERVICEMGR_REAL=1 go test -count=1 ./servicemgr/
```
