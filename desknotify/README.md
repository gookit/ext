# desknotify

`desknotify` provides best-effort desktop notifications without linking a GUI
library. It uses Windows PowerShell Toast, macOS `osascript`, and Linux
`notify-send`; unsupported environments return `*SkipError`.

```go
err := desknotify.Notify(ctx, desknotify.Notification{
    Title: "Approval required",
    Body:  "Review the pending action",
    Kind:  "approval",
})
```

`Kind` is an application-defined grouping key for `Dispatcher`; notification
wording remains entirely the caller's responsibility.
