# Lyra — 产品路线图

> 配合 [`ARCHITECTURE.md`](./ARCHITECTURE.md) 阅读。本文档定义实施顺序和 milestone 边界。

---

## 总体策略

Lyra 是产品，不是 framework。**每个 milestone 都要能跑、能演示**。绝不做"先把 10 个抽象写好再用"。

每个 milestone 完成后能交付一个**可执行的 lyra 二进制**，体验在每个 milestone 都有可见提升。

---

## M0 — 骨架（已完成 ✅）

**目标**：仓库就位，可编译，可跑 hello world。

- [x] `lyra/` Go module + `go.work` 注册
- [x] `cmd/lyra/main.go` 占位 main
- [x] `doc/ARCHITECTURE.md` + `doc/ROADMAP.md`

---

## M1 — Walking Skeleton（**第一个能演示的版本**）

**目标**：单轮对话。用户输入 → LLM 回复 → 退出。无 tool、无 memory、无 session。

**用户体验**：
```bash
$ lyra chat "what is the capital of france"
Paris.
$
```

**实现**：
- `internal/config/` — 读 `~/.lyra/config.toml`（model / api_key / system_prompt path）
- `internal/agent/definition.go` — 构造最小 `*core.Agent`（一个 chat-only Action）
- `cmd/lyra/chat.go` — cobra subcommand，单轮 prompt → 走 lynx Platform.RunAgent → 打印回复

**完成判定**：
- 能配置 anthropic + openai 至少一家
- 流式输出（不是等完整 response 再打）
- 配置错误有友好提示（不是 panic）

**预计**：1–2 天

---

## M2 — Tool 集 v1（**Lyra 开始"做事"**）

**目标**：内置 7 个 tool，agent loop 能 read/edit code、跑 bash。

**用户体验**：
```bash
$ lyra chat "what version of go does this repo use"
<reads go.mod>
This repo uses Go 1.26.3.
$
```

**实现**：
- `internal/tools/fs/` — `read_file`, `write_file`, `edit_file`（diff-based 修改，参考 Claude Code 的 Edit tool）
- `internal/tools/bash/` — `bash`（带 timeout、cwd、output 截断）
- `internal/tools/grep/` — `grep`（shell out 到 ripgrep）
- `internal/tools/glob/` — `glob`（filepath.Walk + doublestar）
- `internal/tools/webfetch/` — `web_fetch`（HTTP + html → text）
- `internal/agent/` — Action 自动注入 tool middleware
- 走 lynx `chat.NewToolMiddleware()` 实现 tool loop

**关键决策**：
- 每个 tool 是 `core.AgentTool` + metadata（safety / sandbox / idempotency）
- Tool 执行用 `chat.NewToolMiddleware()`（lynx-core 已有）
- Bash 在 M2 **不带沙箱**（M4 才加），先用 `os/exec` + cwd 限制
- 全部 tool 显示在 system prompt 里（让模型知道有什么可用）

**完成判定**：
- 用 Lyra 修一个真实 bug：先 grep 找文件，read 看代码，edit 改，bash 跑测试
- Token 不会爆（自动截断长 output）
- Tool 错误能 graceful 反馈给模型，让它 retry

**预计**：1–2 周

---

## M3 — TUI + Streaming + Steering（**产品化 UX**）

**目标**：交互式 TUI，模型流式输出，用户能中途插话。

**用户体验**：
```bash
$ lyra
┌─ lyra ─────────────────────────────────────┐
│ ▶ what does this codebase do                │
│                                              │
│   Reading README.md... ●                     │
│   This codebase is...                        │
│   |                                          │
└──────────────────────────────────────────────┘
[steering] _
```

**实现**：
- `internal/tui/` — bubbletea 主循环
  - markdown 渲染（glamour 或 lipgloss）
  - tool call 状态显示
  - streaming text 增量更新
  - 中断（Ctrl+C 停止当前 turn，不退出）
- `internal/stream/` — Event channel（`iter.Seq2[Event, error]`）
  - `TurnStart` / `MessageDelta` / `ToolCallStart` / `ToolCallEnd` / `TurnEnd` / `Error`
- Steering 队列（参考 pi-mono）
  - 用户在 agent 工作时按 Enter → 下一个 tool boundary 注入消息
- `--no-tui` 降级到 plain text streaming

**完成判定**：
- TUI 真的好用（不卡、不闪、不偷字）
- Ctrl+C 立刻响应（不会等 LLM 完成）
- Steering 实测：用户能在 tool 跑一半时改方向

**预计**：1–2 周

---

## M4 — Sandbox + Permission（**安全可控**）

**目标**：Bash / 文件写入受沙箱保护，permission 三档可切。

**用户体验**：
```bash
$ lyra --mode balanced
> delete all .log files in /tmp
[lyra] I'd like to run: rm -f /tmp/*.log
       (allow once / allow always / deny)
> a
[lyra] Deleted 23 files.
```

**实现**：
- `internal/sandbox/` — 抽象 `Profile` 接口
  - macOS: Seatbelt (`sandbox-exec`)
  - Linux: bwrap
  - Windows: 先 stub，输出"无沙箱"警告
- `internal/approval/` — Tool-level approval
  - 三档预设（`safe` / `balanced` / `yolo`）
  - `(tool_name, normalized_arg) → allow/deny` cache
  - LLM classifier（balanced 模式下 Bash 的智能审批）—— **M5 才加**，M4 先做规则
- `--mode` flag + config.toml

**完成判定**：
- macOS 上 Bash 在 seatbelt 里跑，FS 写出 lyra 工作区被拒
- Linux 上 bwrap 等效
- 三档模式行为差异明显

**预计**：1–2 周

---

## M5 — Context Compaction + Memory（**长对话不爆**）

**目标**：token > 60% 自动压缩；学到的东西写进 `LYRA.md`。

**用户体验**：用户感知不到，但可以连续聊 100 轮不爆。`LYRA.md` 文件会自动出现并维护。

**实现**：
- `internal/compaction/`
  - token-ratio 监测（用 lynx 的 `LLMInvocation` 历史）
  - 触发后调 LLM 生成 conversation summary
  - 保留最近 N 条 + summary + LYRA.md
- `internal/memory/`
  - 项目级：`<cwd>/LYRA.md`（agent 学到的项目特定知识）
  - 用户级：`~/.lyra/LYRA.md`（跨项目偏好）
  - 自动提取：每次 turn end 检查"这一轮有什么值得记的"
  - 自动加载：每次 session 开始注入 system prompt
- `internal/instructions/`
  - LYRA.md / AGENT.md 级联加载（cwd 向上 + 全局）

**完成判定**：
- 连续聊 100 轮不爆 context
- 重启 lyra 后能"记得"上次学到的偏好
- 用户可以手动编辑 LYRA.md，下次会读

**预计**：2 周

---

## M6 — Session 树 + 持久化（**探索性工作流**）

**目标**：每个 session 是一棵树，可 fork、可回放。

**用户体验**：
```
$ lyra session list
[12:34] refactor auth module (23 messages)
[09:12] add logging (8 messages)
$ lyra session resume <id>
$ /fork    # 在当前 message 创建分支
$ /tree    # 看完整树状结构
```

**实现**：
- `internal/session/`
  - JSONL 持久化（每 session 一个文件）
  - 树结构（message 带 `parent_id`）
  - `/fork`, `/tree`, `/checkout` 命令
- 集成 lynx `core.SessionStore`（filesystem backend）

**完成判定**：
- 100 个 session 切换流畅
- fork 后两条线互不影响
- 树深度可视化

**预计**：1–2 周

---

## M7 — MCP 集成（**生态接入**）

**目标**：Lyra 既可当 MCP client 用外部 server，也可被当 MCP server 调用。

**用户体验**：
```bash
$ cat ~/.lyra/config.toml
[[mcp_servers]]
name = "github"
command = "mcp-github"

$ lyra
> list my open prs
<calls github mcp's list_prs tool>
...
```

**实现**：
- 直接用 lynx-mcp 客户端
- Lyra 启动时连接配置的 MCP servers
- 把 MCP tools 合并进 Lyra 内置 tool 列表
- 可选：把 Lyra 自己 expose 成 MCP server（让 Claude Code 等调用 Lyra 当子 agent）

**完成判定**：
- 配置 github MCP 后能在 Lyra 里调用 GitHub 操作
- 配置 filesystem MCP 后能用其文件能力

**预计**：1 周

---

## M8 — Planner 集成 + Plan Mode（**lynx 独门**）

**目标**：让用户体验到"5 planner 可切"和 Plan Mode。

**用户体验**：
```bash
> /plan refactor the auth module to use jwt
[lyra] Planning...
       Step 1: read auth module
       Step 2: identify session-based auth points
       Step 3: introduce jwt library
       Step 4: convert each handler
       Step 5: update tests
       (proceed / edit / abort)
```

**实现**：
- `/plan` 命令切到 goap planner + plan mode
- HTN library 提供 "refactor" / "bug-fix" / "feature-add" 几个高频任务的预定义分解
- Plan 预览 + 用户确认 + 执行

**完成判定**：
- 真实任务里 HTN 比 reactive 更稳定
- 用户能编辑 plan 后再执行

**预计**：2 周

---

## M9 — 观测 + Trace Viewer（**生产可调试**）

**目标**：每个 session 都有完整 trace；本地 viewer 可看 timeline。

**用户体验**：
```bash
$ lyra trace --session <id>
<打开 TUI 时间线，看每个 tool / LLM call 的耗时 / token / cost>
```

**实现**：
- `internal/trace/`
  - 启动内嵌的 OTel collector（in-memory）
  - 收集 lynx 的 span（agent / planner / action / tool / chat）
  - JSONL 持久化
- `lyra trace` subcommand：TUI timeline viewer
- `lyra trace --export-otlp` 推送到外部 collector

**完成判定**：
- 一个 session 后能看清"哪一步慢"
- 能看 token / cost 分布

**预计**：1–2 周

---

## M10 — v0.1 发布

**目标**：能在外人面前演示，文档齐全。

- [ ] README（含 demo gif）
- [ ] 安装指南（brew / go install）
- [ ] config.toml 文档
- [ ] LYRA.md 写法指南
- [ ] 5 个典型用例（refactor / bug-fix / explain / test-gen / web-research）
- [ ] CHANGELOG.md
- [ ] CI（GitHub Actions：build / test / lint）
- [ ] 跨平台 release（darwin / linux / windows）

**预计**：1 周

---

## 后续路线（v0.2+，不在 v0.1 范围）

- IDE bridge（VSCode extension）
- Web UI
- 团队/云同步
- Plugin marketplace
- Skill 系统（Markdown + SKILL.md）
- LSP 集成（通过 MCP）
- 多 agent 编排 UI（基于 lynx RunInScope）
- Memory 向量化检索（基于 lynx vectorstore）
- 评估 framework（agent benchmark / regression test）

---

## 总时长估算

| Milestone | 预计耗时 | 累计 |
|---|---|---|
| M0 | 完成 | — |
| M1 walking skeleton | 1–2 d | 2 d |
| M2 tool 集 v1 | 1–2 w | 16 d |
| M3 TUI + steering | 1–2 w | 30 d |
| M4 sandbox + permission | 1–2 w | 44 d |
| M5 compaction + memory | 2 w | 58 d |
| M6 session 树 | 1–2 w | 72 d |
| M7 MCP | 1 w | 79 d |
| M8 planner | 2 w | 93 d |
| M9 trace viewer | 1–2 w | 107 d |
| M10 v0.1 发布 | 1 w | 114 d |

**~4 个月做到 v0.1**（按一人 full-time 估）。

---

## Milestone 顺序的几个理由

1. **M1 → M2**：先能聊天再做事。倒过来会被 tool 系统复杂度拖死。
2. **M3 在 M2 后**：先把核心能力做出来，再做 UX。倒过来会做漂亮的玩具。
3. **M4 在 M3 后**：sandbox 不影响功能演示，但影响"敢不敢真用"。所以 M3 之后立刻加。
4. **M5 是分水岭**：M5 之前 lyra 是"聊天 + 工具"，M5 之后是"长期助手"。
5. **M8 放后面**：planner 是 lynx 的独门，但短链路 coding 用 reactive 就够。MVP 阶段 planner 是 nice-to-have。

---

## 衡量产品价值的指标（每个 milestone 后追踪）

- **first-token latency**：从用户敲回车到第一个字出现
- **task success rate**：典型任务（修 bug / 加 feature）通过率
- **context efficiency**：完成同一任务消耗的 token
- **interruption smoothness**：Ctrl+C / steering 的响应时间
- **session persistence**：能恢复多深的中断点

每个 milestone 完成时跑一遍指标，对比上个 milestone。指标退化 = 不能合并。
