# app2 Lifecycle Hooks

本文档定义用户编写的 `hooks.json` 与 Hook command 私有进程契约。它不是第二套 Lyra RPC：management 仍只有既有
`hooks.list` / `hooks.setTrust`，资源失效仍只有 `hooks.changed`。

## 1. Sources 与 trust

Runtime 按以下顺序发现配置：

1. `~/.lyra/hooks.json`；
2. resolved project root 到当前 workspace 的每层 `.lyra/hooks.json`。

global source 默认可信；project source 默认 inert，只有 `hooks.list` 返回的 canonical `projectRoot` 经显式 `hooks.setTrust` 后才进入执行集合。
未信任 project 文件可由管理查询解析审计，但 Run 不打开它。文件通过 confined root 读取；symlink escape、非 regular file、超过 256 KiB、
unknown/trailing JSON、超过 128 entries 都拒绝。整条 effective cascade 最多 256 entries。

非空文件必须是：

```json
{
  "hooks": [
    {
      "event": "PreToolUse",
      "matcher": "shell",
      "command": "./scripts/check-command"
    },
    {
      "event": "UserPromptSubmit",
      "inject": "Follow this project's release checklist."
    }
  ]
}
```

每项必须且只能提供 `command` 或 `inject`。`inject` 只允许有 context consumer 的 Pre/PostToolUse、UserPromptSubmit、SessionStart 与
PreCompact，避免接受永远不会生效的配置。`matcher` 是只用于 Pre/PostToolUse 的 Go path glob；`timeoutMillis` 只用于 command，默认 30000，
最大 300000。event closed set 是 `PreToolUse`、`PostToolUse`、`UserPromptSubmit`、`SessionStart`、`SubagentStart`、`SubagentStop`、
`PreCompact`、`Stop`、`Notification`。

## 2. Command input

Unix 使用 `/bin/sh -c`，Windows 使用 `cmd.exe /d /s /c`；working directory 是 resolved workspace。Runtime 在 stdin 写一个 JSON object：

```json
{
  "event": "PreToolUse",
  "sessionId": "ses_...",
  "runId": "run_...",
  "workspace": "/absolute/project",
  "tool": {
    "name": "shell",
    "arguments": {"command": "go test ./..."}
  }
}
```

按 event 还可出现：

- `prompt` / `promptTruncated`；
- `tool.result` / `tool.error` / `tool.resultTruncated`；
- `subagent.runId`、`parentRunId`、`description`、`prompt`、`status`、`result`、`error` 及 truncation markers；
- `reason`，用于 PreCompact、Notification 与 Stop 的可读说明；Tool / Subagent failure 使用各自的 `error`。

`UserPromptSubmit` 对应用户输入发起 fresh root Run 的 admission；Lyra 的 live `runs.steer` 是独立控制语义，不伪装成第二次 prompt admission。
自治 Goal 指令同样不会伪装成用户提交。

输入只含 Lyra identities 与 provider-neutral material，不含 Framework Process、member、checkpoint 或 provider response。stdin 最大 512 KiB；
Tool result、prompt、arguments 与 reason 还各自受更小的领域上限。

## 3. Command output 与 control

空 stdout 等价于 allow。非空 stdout 必须是一个无 unknown/trailing fields 的 JSON object：

```json
{
  "decision": "ask",
  "reason": "Review this deployment command.",
  "injectContext": "Deployment policy was evaluated.",
  "rewriteArguments": {"command": "./scripts/deploy-staging"}
}
```

- `decision`: `allow | deny | ask`；`ask` 只允许 PreToolUse；
- `rewriteArguments`: JSON object，只允许 PreToolUse；
- `injectContext`: bounded trusted context；PostToolUse 的 injection 会附到 Tool result，prompt injection 进入 system context；
- exit 0 应用合法 stdout decision；exit 2 在 gated event 始终 deny，优先用 stdout reason、否则 stderr；observe-only event 返回 control
  仍属于无效输出；
- timeout、spawn failure、oversized/malformed stdout 与其他 non-zero exit 记录 warning 并视为该 command 无决定，不能冻结 Runtime；唯一例外是
  gated event 的 exit 2 即使 stdout 损坏也保持 deny，并退回 stderr / 默认 reason。

PreToolUse deny 在 effect 前返回 recoverable Tool error；UserPromptSubmit/SessionStart deny 让已 admission 的 Run 明确失败。PostToolUse、
SubagentStart/Stop、Notification、Stop 是 observe-only，control verdict 无效。PreCompact 可 deny candidate，但只有真正的 compaction producer
到达 candidate boundary 才会触发。

## 4. Approval 与 lifecycle ownership

Hook `ask` 不创建第二套 approval。Runtime 将 Hook verdict 与 Lyra safety policy 合并为同一个 durable interrupt；弹窗、path/Plan gates、
实际 Tool call 与 Transcript 使用同一 effective arguments。恢复 durable approval 时会在 effect 前按当前 trusted cascade 重新执行 PreToolUse；
因此 policy command 必须可安全重入。用户编辑参数或 Hook 再 rewrite 且最终参数改变时，最终参数会再次展示后才执行。

Stop/Notification/Subagent commands 在相关 transaction commit 后进入有界单 worker。队列冻结当时的 trusted cascade；Runtime Close 会 cancel
正在运行的 command、丢弃尚未开始的 observe-only work并 join worker。Unix command 使用独立 process group，正常完成、取消和超时都会清理残留子进程。
