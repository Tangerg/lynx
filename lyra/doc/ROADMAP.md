# Lyra — 路线图（CS 架构 / v2）

> 配合 [`ARCHITECTURE.md`](./ARCHITECTURE.md) 阅读。本文档定义实施顺序和 milestone 边界。

---

## 总体策略

Lyra 是 **server runtime**，前端在独立 repo。本 repo 的每个 milestone 都要交付：
1. **可启动的 server binary**（`lyra serve`）
2. **稳定的 protocol contract**（`lyra/proto/`）
3. **至少一个 transport 跑通**（HTTP / gRPC / IPC）

不交付 client UI（前端 repo 单独节奏）。

---

## M0 — 骨架（已完成 ✅）

- [x] `lyra/` Go module + `go.work` 注册
- [x] `cmd/lyra/main.go` 占位
- [x] `doc/ARCHITECTURE.md`（v2 CS 架构）
- [x] `doc/ROADMAP.md`

---

## M1 — Protocol Contract + Service 接口（**协议先行**）

**目标**：定义所有 6 个 service 的 Go interface + proto v1 框架。**还不实现业务，先确定接口**。

**为什么先做协议**：前端在独立 repo 已经在写代码。协议越早稳定，前端越早能 mock 联调。

**实现**：
- `lyra/proto/lyra/v1/session.proto` — `SessionService` 全部 RPC + 消息类型
- `lyra/proto/lyra/v1/chat.proto` — `ChatService` + 全部 Event 类型
- `lyra/proto/lyra/v1/tool.proto` — `ToolService`
- `lyra/proto/lyra/v1/approval.proto` — `ApprovalService`
- `lyra/proto/lyra/v1/memory.proto` — `MemoryService`
- `lyra/proto/lyra/v1/trace.proto` — `TraceService`
- `lyra/proto/openapi.yaml` — HTTP 映射（从 proto 注解生成）
- `lyra/internal/service/*/service.go` — 对应 Go interface（不实现）
- `lyra/doc/PROTOCOL.md` — 协议规则（版本、错误码、event 序列、断线重连）

**关键决策**：
- **Event 模型先冻结**：`TurnStart` / `MessageDelta` / `ToolCallRequest` / `ToolCallApproval` / `ToolCallStart` / `ToolCallOutput` / `ToolCallEnd` / `ReasoningDelta` / `PlanGenerated` / `TurnEnd` / `Error`
- **错误码标准化**：`SESSION_NOT_FOUND` / `SESSION_IN_USE` / `TURN_ALREADY_RUNNING` / `INVALID_APPROVAL` 等
- **断线重连协议**：每个 event 有 `seq`，客户端传 `resume_from_seq` 续传
- **proto v1 冻结**：v0.1 发布前内部可改，发布后只能加字段

**完成判定**：
- proto 编译通过（`buf lint && buf generate`）
- 所有 service Go interface 文件存在但方法体是 `panic("not implemented")`
- 前端 repo 能通过 git submodule / buf 消费 proto

**预计**：3-5 天

---

## M2 — HTTP Transport + Walking Skeleton（**第一个能演示的版本**）

**目标**：实现 `ChatService.StartTurn` + `StreamEvents`，HTTP+SSE transport 跑通单轮 chat。

**用户体验（前端视角）**：
```bash
# 前端发请求
$ curl -X POST localhost:8080/v1/sessions/<id>/turns \
    -d '{"message": "what is the capital of france"}'

# SSE stream
event: TurnStart
data: {"turn_id":"...","timestamp":...}

event: MessageDelta
data: {"text":"Paris"}

event: TurnEnd
data: {"tokens":42,"cost_usd":0.0001}
```

**实现**：
- `internal/transport/http/` — gin 或 chi-based router
- `internal/transport/http/sse.go` — SSE event encoder
- `internal/service/chat/impl.go` — 走 lynx `Platform.RunAgent`，把 lynx 的 event 总线转成 Lyra event stream
- `internal/service/session/impl.go` — 内存 session store（M6 才上 JSONL 持久化）
- `internal/engine/agent.go` — 构造最小 `*core.Agent`（chat-only，无 tool）
- `internal/config/` — `~/.lyra/config.toml`（model / api_key）
- `cmd/lyra/serve.go` — cobra subcommand

**关键决策**：
- M2 不上 sandbox / approval / compaction —— 先把 transport 链路打通
- 至少支持 anthropic + openai 两家

**完成判定**：
- `lyra serve --listen :8080` 启动 < 100ms
- 前端通过 HTTP+SSE 完成一轮 chat
- Ctrl+C 退出后端，下次启动还能用（session id 持久不重）
- proto v1 各 message 都能 marshal/unmarshal（test 覆盖）

**预计**：1-2 周

---

## M3 — IPC Transport（**本地嵌入式部署**）

**目标**：stdio JSON-RPC transport，让前端可 spawn lyra-server 子进程通信。

**用户体验**：前端启动时：
```ts
const server = spawn("lyra", ["serve", "--stdio"]);
const client = new LyraClient(server.stdio);
await client.session.create({...});
```

**实现**：
- `internal/transport/ipc/` — stdio JSON-RPC server
  - 一行一个 message（newline-delimited JSON）
  - 同一份 service 方法 dispatch（不重复实现）
  - bidirectional：服务端可主动推 event 到 client（不是只 reply）
- `cmd/lyra/serve.go` 加 `--stdio` flag

**关键决策**：
- JSON-RPC 2.0 标准（method / params / id / result / error）
- Event push 走 notification（无 id 的 message）
- 协议头里带 `lyra-version` 用于版本协商

**完成判定**：
- 前端可以 spawn server 跑完整 chat 流程
- IPC 延迟 < 5ms（本地）
- Server 进程退出时干净关闭（无僵尸）

**预计**：1 周

---

## M4 — Tool 集 v1（**Lyra 开始"做事"**）

**目标**：7 个内置 tool 全部可用。

**实现**：
- `internal/engine/tools/fs/` — `read_file`, `write_file`, `edit_file`
- `internal/engine/tools/bash/` — `bash`（带 timeout + cwd + output 截断，**无沙箱**）
- `internal/engine/tools/grep/` — `grep`（ripgrep wrapper）
- `internal/engine/tools/glob/` — `glob`
- `internal/engine/tools/webfetch/` — `web_fetch`
- `internal/engine/tools/task/` — `task`（子任务委派，用 lynx `AsChatTool`）
- `internal/service/tool/impl.go` — `ListTools` 返回所有内置 tool 的 metadata
- agent definition 自动注入 tool middleware

**完成判定**：
- 前端调用一轮 chat，模型能 call tool，event stream 推 `ToolCallStart` / `ToolCallOutput` / `ToolCallEnd`
- 真实场景：让 lyra 修一个 bug（grep + read + edit + bash）

**预计**：1-2 周

---

## M5 — gRPC Transport（**高吞吐场景**）

**目标**：gRPC + bidirectional stream。

**实现**：
- `internal/transport/grpc/` — `google.golang.org/grpc`
- Bidirectional stream 实现 ChatService（一个 stream 同时跑 request + event push）
- TLS 支持（远端部署用）

**完成判定**：
- 同一份业务逻辑跑在 gRPC / HTTP / IPC 三个 transport 上行为一致
- benchmark：gRPC 吞吐 > HTTP 2x（同等负载）

**预计**：1 周

---

## M6 — Sandbox + Permission（**安全可控**）

**目标**：sandbox 三平台 + 三档 permission 模式。

**实现**：
- `internal/engine/sandbox/` — macOS Seatbelt / Linux bwrap / Windows stub
- `internal/service/approval/impl.go` — Tool-level approval cache（bbolt 持久化）
- 三档预设 `safe` / `balanced` / `yolo`
- 服务端发 `ToolCallApproval` event → 客户端 UI 询问 → 客户端调 `ApprovalService.Decide`

**完成判定**：
- macOS 上 Bash 在 seatbelt 沙箱内
- 三档模式行为差异明显
- approval cache 跨重启保留

**预计**：1-2 周

---

## M7 — Context Compaction + Memory（**长对话不爆**）

**目标**：token > 60% auto-compact，写回 LYRA.md。

**实现**：
- `internal/engine/compaction/` — token-ratio 监测 + LLM summary
- `internal/service/memory/impl.go` — `<cwd>/LYRA.md` + `~/.lyra/LYRA.md` 级联
- 自动提取：每次 turn end 检查"这一轮有什么值得记的"

**完成判定**：
- 连续聊 100 轮不爆
- LYRA.md 跨 session 保留
- 前端可通过 `MemoryService.GetLYRAMd` / `UpdateLYRAMd` 编辑

**预计**：2 周

---

## M8 — Session 持久化 + 树 + Fork（**探索性工作流**）

**目标**：JSONL 持久化 + session 树 + branching。

**实现**：
- `internal/storage/session_store.go` — JSONL 持久化（实现 lynx `core.SessionStore`）
- 树结构（message 带 `parent_id`）
- `SessionService.Fork(id, at_message_id)` 创建分支
- `SessionService.List` 返回树结构
- 断线重连：客户端传 `resume_from_seq`，服务端从持久化日志续传 event

**完成判定**：
- 重启 server 后 session 状态完整恢复
- Fork 后两条线互不影响
- 100 个 session 列表渲染 < 50ms

**预计**：1-2 周

---

## M9 — MCP Transport + MCP Client（**生态双向接入**）

**目标**：
1. Lyra-server 把自己暴露成 MCP server（让其他 agent 当 subagent 调）
2. Lyra-server 作为 MCP client 消费外部 MCP server（github / filesystem / lsp 等）

**实现**：
- `internal/transport/mcp/` — 走 lynx-mcp server 实现
  - 把 6 个 Lyra service 映射成 MCP tool（如 `lyra.chat.start_turn`）
- `internal/engine/mcp_client.go` — 启动时连接配置的 MCP servers，工具合并进 tool 列表

**完成判定**：
- 配置 github MCP 后能在 Lyra 里用 github 操作
- Claude Code 等 agent 把 Lyra 当 MCP server 调
- 三种 transport（HTTP / gRPC / IPC）+ MCP 全部一致

**预计**：1-2 周

---

## M10 — Planner + Plan Mode（**lynx 独门**）

**目标**：让前端能体验到"5 planner 可切"和 Plan Mode。

**实现**：
- `ChatService.StartTurn` 接受 `plan_mode: bool`
- Plan mode 下走 goap planner，先产 plan event，等用户确认后才执行
- HTN library 提供 `refactor` / `bug-fix` / `feature-add` 预设
- `PlanGenerated` event 推 plan 给前端预览

**完成判定**：
- 前端发 `plan_mode: true` 能看到 plan 预览
- 用户编辑 plan 后再执行

**预计**：2 周

---

## M11 — Trace Service（**观测**）

**目标**：完整 trace 收集 + 查询 API。

**实现**：
- `internal/service/trace/impl.go` — 内嵌 OTel collector
- 持久化到 JSONL（同 session 目录）
- `TraceService.ListTraces` / `GetTrace` / `StreamLiveSpans`（实时观看当前 turn）

**完成判定**：
- 前端能渲染 trace timeline
- 一个 turn 完成后能查到完整 span tree（agent / planner / action / tool / chat）

**预计**：1-2 周

---

## M12 — v0.1 发布

**目标**：能演示，文档齐全，proto v1 锁定。

- [ ] README（含 demo 视频链接）
- [ ] 安装指南（brew / go install / docker）
- [ ] config.toml 文档
- [ ] proto v1 freeze + CHANGELOG
- [ ] CI（build / lint / proto-lint / e2e）
- [ ] 跨平台 release（darwin / linux / windows）
- [ ] **PROTOCOL.md + CLIENT.md 完整**
- [ ] 跟前端 repo 联调通过 v0.1 acceptance test

**预计**：1 周

---

## 总时长估算

| Milestone | 预计耗时 | 累计 |
|---|---|---|
| M0 骨架 | 完成 | — |
| M1 protocol + service 接口 | 3-5 d | 5 d |
| M2 HTTP transport + walking skeleton | 1-2 w | 19 d |
| M3 IPC transport | 1 w | 26 d |
| M4 tool 集 | 1-2 w | 40 d |
| M5 gRPC transport | 1 w | 47 d |
| M6 sandbox + permission | 1-2 w | 61 d |
| M7 compaction + memory | 2 w | 75 d |
| M8 session 持久化 + 树 | 1-2 w | 89 d |
| M9 MCP transport + client | 1-2 w | 103 d |
| M10 planner + plan mode | 2 w | 117 d |
| M11 trace service | 1-2 w | 131 d |
| M12 v0.1 发布 | 1 w | 138 d |

**~4.5 个月做到 v0.1**（一人 full-time）。

---

## 跟前端 repo 的协作时间线

```
M1 完成 ──────► 前端可拿 proto mock 联调
M2 完成 ──────► 前端可跑真实 HTTP+SSE 链路（chat）
M3 完成 ──────► 前端可 embed lyra-server 走 stdio
M4 完成 ──────► 前端能展示 tool 调用流（spinner / 输出）
M6 完成 ──────► 前端能弹 approval 对话框
M7 完成 ──────► 前端能展示 LYRA.md 编辑器
M8 完成 ──────► 前端能展示 session 树 / fork UI
M11 完成 ──────► 前端能展示 trace timeline
M12 ──────► 双 repo 联合发布
```

---

## Milestone 顺序的几个理由

1. **M1 协议先行**：CS 架构最大风险是协议反复改，前端已开始写，协议要先冻
2. **M2 → M3 顺序**：先 HTTP（最通用）再 IPC（特殊优化）
3. **M5 gRPC 放中间**：等 HTTP 接口稳了再加 gRPC，避免双份 maintain
4. **M6 sandbox 后置**：先把 tool 跑起来再加沙箱，否则 tool 调试都难
5. **M7 → M8 顺序**：先压缩（短期内存）再持久化（长期存储）
6. **M9 MCP**：等业务稳定再做生态接入，避免协议震荡
7. **M10 planner**：lynx 独门优势，留到中后期作为差异化卖点
8. **M11 trace 接近收尾**：先保证功能，最后做可观测

---

## 价值指标（每 milestone 跑一遍）

- **first-event latency**：HTTP POST 到首个 SSE event 的时间
- **task success rate**：典型任务通过率
- **token efficiency**：完成同任务消耗的 token
- **server boot time**：`lyra serve` 到 ready
- **transport parity**：HTTP / gRPC / IPC 三个 transport 行为一致性测试

指标退化 = 不能合并。

---

## 跟前端 repo 的协议同步流程

1. **协议改动 → 在 lyra repo 提 PR**
2. **CI 自动跑 `buf breaking`** 检测破坏性变更
3. **前端 repo 维护者 review**
4. **同步 merge** + 双方 release
5. **breaking change 必须升 v2**，v1 保留至少 6 个月

每个 milestone 完成的同时，**proto / openapi / docs 必须同步更新**。
