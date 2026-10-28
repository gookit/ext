# imbot

`imbot` sends one-way notifications without coupling callers to an application config package.
It supports generic signed webhooks, DingTalk markdown bots, and Feishu text bots.

## Usage

```go
provider, err := imbot.NewProvider(imbot.KindFeishu, target,
    imbot.Options{Secret: secret, Keyword: "安全", AllowedHosts: []string{"open.feishu.cn"}})
if err != nil { return err }
err = provider.Send(ctx, imbot.Message{
    Title: "任务完成", Text: "构建已完成", Link: "https://example.test/jobs/1",
    Level: imbot.LevelInfo,
})
```

Set `Options.Keyword` for DingTalk or Feishu keyword-mode bots; it is appended to the message body (and to generic webhook `text`) without duplication.

Targets must use HTTPS (or explicitly enabled HTTP), be listed in `AllowedHosts`, and
resolve only to public addresses. A send is bounded by `Options.Timeout` and can retry
once for transport, throttling, and other classified transient failures.
