# Lyra — 路线图（核心运行时优先 / v3）

> 配合 [`ARCHITECTURE.md`](./ARCHITECTURE.md) 阅读。

---

## 总体策略

**Phase 1 — 核心运行时**（M1~M7）：transport-agnostic Go service 接口 + 完整业务能力。in-process Go API 测试。可作 SDK 形态发布 v0.1。

**Phase 2 — Transport 层**（M8+）：HTTP+SSE / IPC stdio / gRPC / MCP 四个 adapter，全部映射到同一份 service 接口。v0.2。

核心运行时是产品问题（agent 行为），transport 是工程问题（协议 marshal）。先把前者做透，后者随时可加。

---

## M0 — 骨架（已完成 ✅）

- [x] `lyra/` Go module + `go.work`
- [x] `cmd/lyra/main.go` 占位
- [x] ARCHITECTURE.md / ROADMAP.md

---

## M1 — Service 接口 + 最小 Engine（**Walking Skeleton**）

**目标**：6 个 service 的 Go interface 冻结，最小 engine 跑通**单轮 chat**（in-process Go API）。

**为什么 service 接口先行**：transport 后续要加 HTTP/IPC/gRPC adapter，每个 transport 都映射到同一份 service 接口。接口稳了，transport 才能并行加。

**实现**：
- `internal/service/session/service.go` — `SessionService` Go interface（不实现）
- `internal/service/chat/service.go` — `ChatService` + `Event` 类型（**M1 重点**）
- `internal/service/tool/service.go` — `ToolService` 接口
- `internal/service/approval/service.go` — `ApprovalService`
- `internal/service/memory/service.go` — `MemoryService`
- `internal/service/trace/service.go` — `TraceService`
- `internal/engine/engine.go` — 整合 lynx Platform / chat.Client
- `internal/engine/agent.go` — 构造最小 `*core.Agent`（chat-only，无 tool）
- `internal/service/chat/impl.go` — 实现 `ChatService.StartTurn`，走 lynx Platform，lynx event 总线 → Lyra event channel
- `internal/service/session/inmemory.go` — 内存 session store（M3 才上 JSONL）
- `internal/config/config.go` — `~/.lyra/config.toml` 加载（最简：model + api_key）
- `cmd/lyra/main.go` — 改成 `lyra chat "..."` 直接用 service 接口

**关键决策**：
- **Event 类型先冻结**（M1 版本）：`TurnStart` / `MessageDelta` / `TurnEnd` / `Error`
  - M2 加 `ToolCallStart` / `ToolCallEnd`
  - M4 加 `ToolCallApproval`
  - M6 加 `ReasoningDelta` / `PlanGenerated`
- **service 接口只用 Go std 类型**（context / channel / error） — 不引 proto
- 至少支持 anthropic + openai 两家

**完成判定**：
- `lyra chat "what is the capital of france"` 流式打印 LLM 回复
- 业务代码全部在 `internal/service/`，main.go 只做 wiring
- Service 接口完全 transport-agnostic（M8 加 HTTP adapter 时不改 service 代码）

**预计**：1-2 周

---

## M2 — Tool 集 v1（**Lyra 开始"做事"**）

**目标**：7 个内置 tool 全部可用。

**实现**：
- `internal/engine/tools/fs/` — `read_file`, `write_file`, `edit_file`
- `internal/engine/tools/bash/` — `bash`（timeout + cwd + output 截断，**M4 才上沙箱**）
- `internal/engine/tools/grep/` — `grep`（ripgrep wrapper）
- `internal/engine/tools/glob/` — `glob`
- `internal/engine/tools/webfetch/` — `web_fetch`
- `internal/engine/tools/task/` — `task`（子任务委派，用 lynx `AsChatTool`）
- `internal/service/tool/impl.go` — `ListTools` 返回所有内置 tool metadata
- `internal/service/chat/impl.go` — agent definition 自动注入 tool middleware
- `ChatService` event 加 `ToolCallStart` / `ToolCallEnd`

**完成判定**：
- `lyra chat "find all TODO in this repo"` 能 grep + 输出
- `lyra chat "fix bug X"` 能 grep + read + edit + bash 跑测试

**预计**：1-2 周

---

## M3 — Session 持久化 + 树 + Fork（**多轮 + 探索性工作流**）

**目标**：JSONL 持久化 session，多轮对话 + branching。

**实现**：
- `internal/storage/session_store.go` — JSONL 持久化（实现 lynx `core.SessionStore`）
- 树结构（message 带 `parent_id`）
- `SessionService.List` / `Create` / `Resume` / `Fork(id, at_message_id)`
- `cmd/lyra/main.go` — 加 `--session <id>` flag 续上次会话
- 每条 event 持久化（断线/重启可恢复）

**完成判定**：
- 重启 lyra 后能 `--session <id>` 续聊
- Fork 后两条线互不影响
- session 列表存的是树，不是平铺

**预计**：1-2 周

---

## M4 — Sandbox + Permission（**安全可控**）

**目标**：sandbox 三平台 + 三档 permission 模式。

**实现**：
- `internal/engine/sandbox/` — macOS Seatbelt / Linux bwrap / Windows stub
- `internal/service/approval/impl.go` — Tool-level approval cache（bbolt 持久化）
- 三档预设：`safe` / `balanced` / `yolo`
- `ChatService` event 加 `ToolCallApproval`，业务流：engine 发请求 → 客户端（CLI）询问 → 客户端调 `ApprovalService.Decide` → engine 继续

**完成判定**：
- macOS 上 Bash 跑在 seatbelt 内
- 三档模式行为差异明显（用 `--mode yolo` 跳过所有审批）
- approval cache 跨重启保留

**预计**：1-2 周

---

## M5 — Context Compaction + Memory（**长对话不爆**）

**目标**：token > 60% auto-compact，写回 LYRA.md。

**实现**：
- `internal/engine/compaction/` — token-ratio 监测 + LLM summary
- `internal/service/memory/impl.go` — `<cwd>/LYRA.md` + `~/.lyra/LYRA.md` 级联
- 自动提取：每次 turn end 检查"这一轮有什么值得记的"
- 启动时自动注入 LYRA.md 到 system prompt

**完成判定**：
- 连续聊 100 轮不爆
- LYRA.md 跨 session 保留
- `MemoryService.GetLYRAMd` / `UpdateLYRAMd` 可读写

**预计**：2 周

---

## M6 — Planner + Plan Mode（**lynx 独门**）

**目标**：体现"5 planner 可切"和 Plan Mode。

**实现**：
- `ChatService.StartTurn` 接受 `PlanMode` 参数
- Plan mode 走 goap planner，先产 plan event，等用户确认后才执行
- HTN library 提供 `refactor` / `bug-fix` / `feature-add` 预设
- `ChatService` event 加 `PlanGenerated` / `ReasoningDelta`

**完成判定**：
- `lyra chat --plan "refactor X to use Y"` 看到 plan 预览
- 用户可在 plan 预览阶段 abort / continue

**预计**：2 周

---

## M7 — MCP Client + Trace + v0.1 发布

**目标**：MCP 生态接入 + 观测 + 发布。

**实现**：
- `internal/engine/mcp_client.go` — 启动连配置的 MCP servers，工具合并进 tool 列表
- `internal/service/trace/impl.go` — 内嵌 OTel collector，trace 持久化到 JSONL
- `TraceService.ListTraces` / `GetTrace` / `StreamLiveSpans`
- v0.1 发布物：README + 安装指南 + config 文档 + 跨平台 binary

**完成判定**：
- 配置 github MCP 后能在 Lyra 里用 github 操作
- 一个 turn 完成后能查到完整 span tree
- v0.1 二进制能在 macOS / Linux 跑通

**预计**：2 周

---

## Phase 2 — Transport 层（v0.2+，先别管）

M8+：HTTP+SSE → IPC stdio → gRPC → MCP server（让其他 agent 把 Lyra 当 subagent）

每个 transport 都是同一份 service 接口的 adapter，**核心运行时一行代码不改**。

---

## 总时长估算

| Milestone | 预计耗时 | 累计 |
|---|---|---|
| M0 骨架 | 完成 | — |
| M1 service 接口 + walking skeleton | 1-2 w | 14 d |
| M2 tool 集 | 1-2 w | 28 d |
| M3 session 持久化 + 树 | 1-2 w | 42 d |
| M4 sandbox + permission | 1-2 w | 56 d |
| M5 compaction + memory | 2 w | 70 d |
| M6 planner + plan mode | 2 w | 84 d |
| M7 MCP client + trace + 发布 | 2 w | 98 d |

**~3 个月做到 v0.1（核心运行时形态）**，一人 full-time。

Phase 2 transport 层视情况另算。

---

## Milestone 顺序的几个理由

1. **M1 接口先行**：service 接口稳了后续 transport 才能并行加。即使现在不做 transport，接口设计也要 transport-agnostic。
2. **M2 tool 先于 sandbox**：先把功能跑起来再加沙箱，否则 tool 调试都难。
3. **M3 session 先于 compaction**：先有多轮，才有"长对话"这个问题。
4. **M5 → M6 顺序**：先解决"上下文不爆"再追求"规划好"。
5. **M7 收尾**：MCP / trace / 发布三件事一起做，作为 v0.1 收尾。

---

## 价值指标（每 milestone 跑一遍）

- **first-token latency**：从 `StartTurn` 调用到首个 `MessageDelta` 的时间
- **task success rate**：典型任务通过率（修 bug / 加 feature / 解释代码）
- **token efficiency**：完成同任务消耗的 token
- **session 持久化恢复时间**：100 个 session 列表 + 续会
- **service-impl 单测覆盖**：≥80%

指标退化 = 不能合并。
