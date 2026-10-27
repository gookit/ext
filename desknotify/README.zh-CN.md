# desknotify

`desknotify` 提供尽力而为的桌面通知，不链接 GUI 库：Windows 使用
PowerShell Toast，macOS 使用 `osascript`，Linux 使用 `notify-send`。
桌面不可用时返回 `*SkipError`，不会阻断调用方的主要操作。

```go
err := desknotify.Notify(ctx, desknotify.Notification{
    Title: "需要审批",
    Body:  "请检查待执行操作",
    Kind:  "approval",
})
```

`Kind` 是调用方定义的、供 `Dispatcher` 合并通知的分组键；通知文案完全由
调用方传入。
