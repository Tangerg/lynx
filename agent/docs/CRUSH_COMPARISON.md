# lynx/agent vs charmbracelet/crush — 架构对比与启发

> **2026-05-09**。基线：lynx/agent (Go) HEAD `3767d83` / crush HEAD（路径 `/Users/tangerg/Desktop/crush`）。
>
> 系列文档：
> - [`./EMBABEL_GAP_ANALYSIS.md`](./EMBABEL_GAP_ANALYSIS.md) — 同源框架（Kotlin/Spring）逐项 gap 打分
> - [`./ADK_COMPARISON.md`](./ADK_COMPARISON.md) — 异范式 LLM-runtime（Google ADK）的跨范式启发
> - **本文** — vs Crush（**产品级 TUI coding assistant**，非框架）
>
> **核心定位差异**：lynx 是 framework，Crush 是 **应用产品**（"your new coding bestie in your terminal"）。所以本文不是逐项对齐打分，也不是"我们能不能借这个"——大部分 Crush 的能力是**产品 UX feature**，不是 framework 抽象，**不应该**进 lynx。**有价值的启发面比 ADK 还窄**。

---

## 0. TL;DR — 范式 + 定位双重差异

| 维度 | lynx/agent | crush |
|---|---|---|
| **形态** | Go 库（嵌进微服务/Lambda/CLI 用） | **完整 TUI 应用产品**（Bubble Tea v2 + SQLite + LSP + MCP + multi-model） |
| **决策引擎** | GOAP planner（A* / HTN / Reactive） | **LLM 单驱**（fantasy 库）+ tool loop；零 planner |
| **核心抽象** | `Agent` / `Goal` / `Action` / `Condition` / `Plan`（编译期 typed） | `Session` / `Message`（SQLite 持久化）+ stateless `Agent` |
| **持久化** | 无（按设计；`BlackboardFactory` 扩展点） | **一等公民**：SQLite + sqlc，sessions/messages/files 全持久化 |
| **HITL** | tool-level `RequireType[T]` / `WithConfirmation` 装饰器 | 全 framework 级 `permission.Service` + per-session 缓存 + hook pre-approve |
| **多模型** | `chat.Client` 在 chat 中间件层做 | **核心配置**：mid-session 切模型、large/small 双层、per-provider config |
| **MCP** | client + server bridge（`lynx/mcp` + `runtime.MCPToolGroupResolver`） | client only（external MCP servers as tools），无 server export |
| **LSP** | ❌（不在 framework 范围） | **集成**：`internal/lsp` 通过 powernap 起 LSP client，工具走它拿代码上下文 |
| **TUI / Server** | ❌（库，无 UI） | Bubble Tea v2 整套 TUI，imperative components + pub/sub |
| **Skills** | ❌ | **支持**：`SKILL.md` frontmatter 标准（agentskills.io），目录扫描+inject 到 system prompt |
| **Hooks** | `ActionInterceptor` (action-level) + `ToolDecorator` (tool-level) | `PreToolUse` shell-command hook + 并行执行 + JSON input rewrite |
| **代码量（非测试）** | **9319 LOC** | **~50k+ LOC**（产品级，含 TUI / DB / 配置 / OAuth / 多 backend / 整套 LSP 集成） |

**一句话**：Crush ∋ 一个特定 LLM-driven agent loop（fantasy 库）+ 一整套 coding-assistant 产品周边（TUI / 持久化 / 多模型 / LSP / MCP / Skills / 权限系统 / hooks / OAuth / 多 backend）。lynx 只对应其"特定 agent loop"中的一小块——**且范式不同**（lynx GOAP-planned，Crush LLM-driven）。两者**几乎没有重叠的设计空间**，启发面主要在"Crush 在产品工程中暴露的 plumbing 模式"，不在架构模型本身。

---

## 1. 形态差异 — 库 vs 产品

### lynx
单一 import 路径 `github.com/Tangerg/lynx/agent`，零运行时副作用。用户写：
```go
platform := agent.NewPlatform(...)
platform.Deploy(myAgent)
proc, _ := platform.RunAgent(ctx, myAgent, bindings, opts)
```
嵌进任何 Go 程序就完。

### Crush
```bash
brew install charmbracelet/tap/crush
crush  # 启动 TUI
```
全栈应用：拉起 Bubble Tea TUI → 加载 `crush.json` → 启动 LSP clients / MCP clients → SQLite 起 session → 用户在 textarea 输入 → 流式渲染 LLM 回复。

### 启发
**不可借** —— lynx 是库的定位是核心价值，没有任何"嫁接到 Crush 范式"的合理路径。

唯一可以观察的工程模式：Crush 的 [`internal/app/app.go`](file:///Users/tangerg/Desktop/crush/internal/app/app.go) 把所有 service（session / message / permission / agent coordinator / LSP / pub-sub）在 `New()` 一处装配 —— 是个标准的"应用初始化入口"模式。lynx **没有等价物也不需要**：用户的应用代码自己装。

---

## 2. 决策引擎 — planner vs LLM 单驱

### lynx
Action 有 `Pre / Post / Effects` 元数据；planner（GOAP A* / HTN / Reactive）搜索 action 序列；编译期可推 goal reachability。

### Crush
完全没有 planner。一次"turn"的逻辑：
1. user 在 textarea 敲消息 → 加进 messages 表
2. agent 调 `fantasy.Agent.Stream(ctx, call)` 把 messages + tools + system prompt 喂给 LLM
3. fantasy 库内部跑 tool loop（LLM 调 tool → 执行 → 喂回 LLM → 直到 LLM 不再调 tool）
4. 流式回写 message parts 到 DB + pub/sub 通知 UI

**LLM 决定一切**：调哪些工具、调多少次、什么时候停。与 ADK 范式相同。

### 启发
**不借**。理由同 ADK：lynx 的 GOAP 内核是核心价值。

---

## 3. Session-centric vs Process-centric

### lynx
`AgentProcess` per-run，进程终止时 in-memory 状态释放。多 turn 对话由用户在 chat 中间件层实现（`core/model/chat` 已有 history/store middleware）。

### Crush
`Session` 是**第一公民**：
- SQLite 表 `sessions` + `messages`，append-only event log
- 每个 message 是 `{role, parts[], model, provider, ...}`，parts 是 union（Text / Image / ToolCall / ToolResult / Reasoning / Finish）
- mid-session 可换模型（`UpdateModels(ctx)` 后 SetModels），上下文保留
- Session 类型分三种：normal / title-XXX（生成标题用，小模型一次性）/ `messageID$$toolCallID`（sub-agent 跑出来的 task session，cost 回滚到父）
- `Todos []Todo` 字段（LLM 生成的 task list）也住在 session 里

### 启发
**不借** —— [ADK_COMPARISON.md §3](./ADK_COMPARISON.md) 已明确：session/multi-turn 是 chat 中间件层职责，**不在 lynx framework**。

但 Crush 的"task session 嵌套"（`parentMessageID$$toolCallID` 格式的 ID）是个有意思的命名约定 —— sub-agent 调用 ⇒ 自动产 task session ⇒ cost roll-up。lynx 的等价是 `Platform.CreateChildProcess` 的父子 process 关系 + budget 聚合树（[runtime/agent_process.go:125](../runtime/agent_process.go) 已有递归 Usage()）。功能一样，只是 lynx 不持久化。

---

## 4. Permission / HITL — Crush 显著领先

### lynx
[`hitl/`](../hitl) 包：
- `TypedRequest[P, R]` 通用 awaitable
- `WithAwaiting / WithConfirmation / RequireType[T]` 三个 chat-tool decorator
- `core.Awaitable` + `Process.AwaitInput` 进 `StatusWaiting` 等待 `Platform.ResumeProcess`
- 每次工具调用都要走 awaitable 流程 —— **没有缓存**：用户批准过一次 bash，下次还要重新批

### Crush
[`internal/permission/permission.go`](file:///Users/tangerg/Desktop/crush/internal/permission/permission.go) `permission.Service`：
- **Allow-list**（config 静态，`Permissions.AllowedTools = ["bash:ls", "edit"]`）
- **Hook pre-approve**（PreToolUse hook 通过 ctx marker 给后续 permission check 开绿灯）
- **Session 级 auto-approve flag**（"YOLO 模式"）
- **Session-scoped approval cache** —— 用户批准 `bash:run` 一次后，整个 session 内 bash run 全自动通过；session 结束清空
- `Request(ctx, opts)` 内部按上述顺序 short-circuit；都没命中就 `pubsub.Publish(PermissionRequest{...})` + 阻塞 channel 等 UI `Grant()` / `Deny()`

### 启发 — **可以适度借鉴 session-scoped 缓存模式**

虽然 lynx 不做 session abstraction，但 **per-process** 的近似缓存是合理且有用的：

> 用户对同一个工具同一种参数模式（或工具名）批准过一次 → 在**同一个 process 生命周期内**自动通过后续相同请求。

具体设计（提议，可选）：
- 在 `hitl/` 加一个新装饰器 `WithRememberedConfirmation(tool, scope func(args) string)`：缓存 key 由 scope 函数算出（"bash" 整工具粒度 / "bash + ls" 子命令粒度都行）
- 缓存住在 process 的 ServiceProvider 或 blackboard protected key 里，process 结束自然清
- 不实现 cross-process / cross-session 缓存——那是 chat 中间件 / 应用层职责

这个比 ADK 那篇里都还小（约 60 LOC + 测试），单纯解决"反复弹窗"的真实痛点。**可以列入 P2 候选**。

---

## 5. Hooks — Crush 的 PreToolUse 是新模式

### lynx
[`core/extension.go`](../core/extension.go) 9 个 capability 接口里：
- `ActionInterceptor` —— action-level onion wrap（`InterceptAction(ctx, process, action, next)`）
- `ToolDecorator` —— tool-level wrap（`DecorateTool(process, action, tool) → tool`）

两者**都返回新对象/新闭包**式的 wrap，不是 callback 风格。

### Crush
[`internal/hooks/`](file:///Users/tangerg/Desktop/crush/internal/hooks/) `PreToolUse` hook（也只支持这一种）：
- 配置在 `crush.json.hooks.PreToolUse`
- 每个 hook 是一个 **shell command**（`command: "scripts/check.sh"`）
- 环境变量传入：tool name / input JSON / session ID / message ID / ...
- 多 hook **并行执行**（限 N 并发）；结果聚合：deny > allow > none，halt 是 sticky
- Hook 可以**改写 tool input**（返回 JSON patch，shallow merge 到原 input）
- Hook approve → ctx 标记 `WithHookApproval` → permission.Service 看到 marker 直接 short-circuit 通过

### 启发 — **input-rewriting 模式可借鉴；shell-command hook 不借**

**不借**：
- Shell-command hook —— Crush 是产品，hook 是用户写脚本插进来；lynx 是库，用户写 Go interface 实现就好
- 并行 hook 执行 —— ToolDecorator chain 是 onion 顺序，语义清晰，不需要并行

**可以考虑**：
- **Tool input rewriting** —— 当前 `ToolDecorator.DecorateTool` 包出新 tool，新 tool 的 Call 内部可以改 args 再调 inner（Go closure 完全能做）。这是**已有能力**，Crush 没什么新颖。**已闭合**。

结论：hooks 这一节没有真正可落地的差距。

---

## 6. Tool 系统 — 量级差异，模型相近

### lynx
- `core.AgentTool = chat.Tool` 类型别名
- `ToolGroup` 按 role 聚合，`ToolGroupResolver` extension 解析
- `runtime.MCPToolGroupResolver` 把 MCP server 的工具桥过来
- 用户工具走 `chat.Tool` 接口写

### Crush
- 内置 **66+ 工具**：bash / view / edit / multiedit / grep / glob / ls / write / fetch / web_search / sourcegraph / lsp_restart / diagnostics / references / todos / job_kill / job_output / download / read_mcp_resource / multiedit / crush_info / crush_logs / ...
- 每个工具一对 `.go` + `.md`（描述文件）
- Tool wrap 链：`hooked_tool`（hooks）→ permission（HITL）→ 真正工具
- MCP 工具动态注入到 fantasy agent

### 启发
**不借**。Crush 的 66 个工具是 **coding assistant 专属**（grep / lsp_restart / sourcegraph / ...），lynx 作为 framework 不预设领域。MCP integration 已有，互补。

---

## 7. Skills — Crush 特有，不进 lynx

### Crush
[`internal/skills/skills.go`](file:///Users/tangerg/Desktop/crush/internal/skills/skills.go)：
- 实现 `agentskills.io` open standard（开源 skill 共享标准）
- Skill = 目录，含 `SKILL.md`（YAML frontmatter: name / description / compatibility / metadata + markdown 指令）+ 配套文件
- 启动时扫 `pwd/` 和 `~/.crush/`，dedupe，filter active
- 把 active skills 的 markdown 指令**注入 system prompt**

效果：用户拿一个 skill 包（"react-component-author" / "go-project-bootstrap" 之类），放进项目就有了对应能力。

### 启发
**不借**。这是 chat-driven coding tool 的产品特性 —— "system prompt 模板库"。lynx 完全可以让用户在自己的应用代码里实现：扫文件、读 markdown、塞进 `pc.Chat()` 的 system prompt 即可。**不属于 framework 抽象**。

---

## 8. 多模型 / Provider — 全在 chat 包

### Crush
- `internal/config` 列 providers（OpenAI / Anthropic / Gemini / Bedrock / Azure / ...）+ 每 provider 的 models
- 每个 session 选 large / small 两个模型
- mid-session 切模型：`UpdateModels(ctx)` 后调 `SetModels()`，messages 保留
- `fantasy` 库（Charm 内部 LLM 抽象）封装 provider 协议差异

### lynx
- `core/model/chat.Client` 是开放接口；具体 provider impl（OpenAI / Anthropic / ...）由用户挂
- 多模型 = 多个 `*chat.Client` 实例 + `core.ServiceProvider` 注册多个 service key
- mid-process 切模型：用户在 action body 里挑 service key

### 启发
**不借** —— 模型抽象是 chat 包职责，**不属于 agent framework**。Crush 的 model switching 是产品 UX feature。

---

## 9. 持久化 / DB — Crush 重，lynx 按设计不做

### Crush
SQLite + `sqlc.dev/sqlc`：
- `sessions` 表（id / title / message_count / tokens / cost / summary_id / todos JSON）
- `messages` 表（id / session_id / role / parts JSON / model / provider / finished_at）
- `session_files` 表（per-session file modification log）
- migrations 在 `internal/db/migrations`
- 生成代码 in-package（`Queries` struct + 每个 SQL 一个 method）

### lynx
- 仅 `BlackboardFactory` 扩展点（[core/extension.go](../core/extension.go)）
- 默认 in-memory；用户写自己的 Redis / SQL backend
- 持久化策略文档：[`./PERSISTENCE.md`](./PERSISTENCE.md)

### 启发
**不借** —— 已确定：[EMBABEL_GAP_ANALYSIS.md §12](./EMBABEL_GAP_ANALYSIS.md) 早就明确"持久化开箱不做、仅出抽象"。

---

## 10. csync — 并发原语，可观察

### Crush
[`internal/csync/`](file:///Users/tangerg/Desktop/crush/internal/csync/) 自家 lock-free 并发原语：
- `csync.Value[T]` —— `atomic.Pointer` 包装的 RW value
- `csync.Map[K, V]` —— `sync.Map` 的 typed wrapper
- `csync.Slice[T]` —— atomic slice swap

用法：agent 配置（tools / models / system prompt）跑时可热改，读多写少 → 用 `csync.Value` 写时整体 swap、读时无锁。

### lynx
- `runtime.AgentProcess` 用 `sync.Mutex` 配 `processState` / `processBudget` / `processSignals`
- 读时持锁（`getStatus()` 等）；写时持锁（`setStatus()` 等）
- `core.Agent.knownConditions` 用 `atomic.Pointer[map]` + `sync.Once`（同 Crush 模式）

### 启发
**轻度借鉴 / 已对齐** —— lynx 在确实需要 lock-free 的地方（`Agent.KnownConditions` 的 cache）已经用同样的 `atomic.Pointer` swap 模式。**没有具体差距**。

---

## 11. Pub/Sub message 防御性 cloning

### Crush
[`internal/message/message.go`](file:///Users/tangerg/Desktop/crush/internal/message/message.go) 在 pub/sub publish 前 clone 整个 Message struct（line 56 / 94 / 135）。原因：streaming 中 message.Parts 还在被 LLM 回调改写，subscriber（UI）拿到的是 immutable 快照，避免读时被改。

### lynx
[`event/multicast.go`](../event/multicast.go) 的 `Multicast.OnEvent` 直接传 `Event` interface 值给 listener。lynx 的 event 类型多是值类型（含小切片 / map），但**没有 deep clone**。

### 启发 — **lynx 大多数 event 是值类型，不是问题；个别可能需要 clone**

lynx 的 event 类型在 [`event/`](../event)：`AgentDeployedEvent / ProcessCreatedEvent / ProcessCompletedEvent / ActionExecutionStartEvent / ...` 多数字段是 string / time / int，少数挟带 `core.WorldState` map / `*plan.Plan` slice 等可变引用。

**潜在问题场景**：listener A 拿到 `ReadyToPlanEvent.World`（`core.WorldState`），同时 runtime 在下一 tick 改写 world。如果 listener A 异步处理慢，可能读到不一致。

但是：lynx `event.Multicast.OnEvent` 是同步串行 deliver（[event/multicast.go:60-69](../event/multicast.go)），listener 慢就拖延后续 listener。listener 体内若把 event 异步搬到自己的 goroutine，那 race 是 listener 的责任。

**结论**：lynx 已有正确的 deliver 语义；deep clone 不是必须。**不借**，但记下来：将来如果 event 字段引入更多可变引用、且需要异步 listener 的场景，再考虑 per-listener clone（或让 Listener 自己 copy）。

---

## 12. Sub-agent / 委托 — Crush 是工具化，lynx 是 first-class

### Crush
Sub-agent 通过 `run_sub_agent` 工具实现：
- LLM 调 `run_sub_agent({prompt: "...", ...})`
- 工具体内部 `coordinator.runSubAgent(params)`：建一个 task session（id 形如 `parentMsgID$$toolCallID`）+ 用 non-interactive mode 跑 agent + 把 cost roll up + return response
- 单层（无 sub-sub-agent）

### lynx
- `runtime.AsChatTool[In, Out]` —— LLM 决定调（supervisor，typed end-to-end）
- `runtime.AsChatToolFromAgent[In, Out]` —— 同上但 agent 实例直传
- `workflow.SequenceAgents / ParallelAgents / LoopAgent` —— 确定性 agent-level 编排（**第八轮新增**）
- 任意嵌套（child 内可再 spawn）；budget 聚合树自动；branch isolation 通过 `SpawnChildFresh`

### 启发
**lynx 显著更强**：typed + 多种调用模式 + 嵌套 + branch isolation 都领先。**没有可借**。

---

## 13. TUI / 流式 UI — 完全 product-only

### Crush
- Bubble Tea v2 model
- 子组件 imperative（不是 Bubble Tea 子 model）—— 主 model `Update()` 直接调子组件方法
- 通过 pub/sub 把 agent event 转 `tea.Cmd` 喂回 main loop
- 主面板含 message stream / tool call / tool result / 实时 token 流

### lynx
**库**，无 UI。

### 启发
**不做** —— 已在 [ADK_COMPARISON.md §8](./ADK_COMPARISON.md) 明确：HTTP / SSE / TUI 全部不在 framework 范围。

---

## 14. Crush 的 Crush-only 特性（lynx 永远不做）

| 特性 | Crush | 理由不做 |
|---|---|---|
| TUI（Bubble Tea v2） | ✅ | lynx 是库 |
| SQLite 持久化 + sqlc | ✅ | 抽象在 BlackboardFactory，impl 是用户的 |
| LSP 集成（powernap） | ✅ | coding-tool 专属，与 framework 无关 |
| OAuth flows | ✅ | 应用层 |
| 多 backend（Azure / Bedrock / Vertex / ...） | ✅ | chat 中间件层 |
| TUI 内嵌 commands（`/help`, `/sessions`, `/skills`） | ✅ | 应用层 |
| Cost tracking + 显示 | ✅ | lynx Process.Usage 已有数据；UI 是 Crush 责任 |
| Skills（SKILL.md frontmatter） | ✅ | 用户应用代码 5 行扫文件就实现 |
| File tracker（per-session modified files） | ✅ | 应用层 |
| Diff 引擎（`internal/diff` / `diffdetect`） | ✅ | coding-tool 专属 |

---

## 15. 综合启发：lynx 的下一步

**Crush 跨范式给 lynx 真实可借鉴的只有 1 个 P2 候选**（其它要么 lynx 已有等价 / 要么 Crush 是产品特性不属于 framework）：

| 优先级 | 项目 | 借鉴自 Crush | 改动量 | 备注 |
|---|---|---|---|---|
| **P2 候选** | **`hitl.WithRememberedConfirmation(tool, scope)`** —— per-process 缓存"已批准"的 confirmation，在同一个 process 生命周期内同 scope 的请求自动通过 | `permission.Service` 的 session-scoped approval cache | ~60 LOC + 测试 | 解决"反复弹窗"痛点；scope 函数让用户决定缓存粒度（按工具名 / 按工具+一级参数 / 按 scope key） |

**所有其它项目**：

- **Session model / 多 turn 持久化** —— [ADK_COMPARISON.md §3](./ADK_COMPARISON.md) 已明确不做（chat 中间件层职责）
- **TUI / SSE / HTTP server** —— [ADK_COMPARISON.md §8](./ADK_COMPARISON.md) 已明确不做（lynx 是库不是 framework）
- **多模型 mid-session 切换** —— chat 包职责
- **LSP 集成** —— coding-tool 专属，与 framework 无关
- **MCP server export（按 goal 自动发布）** —— lynx 已有 `runtime.AsMCPTool[In, Out]`，闭合
- **Skills（SKILL.md frontmatter）** —— 用户应用代码 5 行扫文件就行
- **PreToolUse hook with input rewriting** —— `ToolDecorator` 闭包形态已等价
- **csync 并发原语** —— lynx 已有合适的 `atomic.Pointer + sync.Once` 用法（如 `Agent.KnownConditions` cache）
- **defensive event cloning** —— lynx event 多数值类型，listener 同步 deliver，无并发问题
- **SQLite 持久化** —— `BlackboardFactory` SPI 已开放，impl 是用户的

### 顺序建议

1. **观察是否有真实 HITL 反复弹窗痛点** —— 如果有，落 `hitl.WithRememberedConfirmation`
2. **embabel 那 3 个 P0 + ADK 那 2 个已落地后**，lynx 的 framework-side 缺口本批已基本闭合

---

## 16. 一句话总结

Crush 是**产品**，lynx 是**库**。两者范式 + 定位都不同，**架构上几乎没有重叠**——Crush 的大多数能力（TUI / SQLite / LSP / Skills / multi-backend）都是 chat-driven coding-tool 产品特性，**正确地不属于 framework 层**。

跨过这层范式 + 定位差，Crush 给 lynx 的唯一一个真实可落地启发是 **per-process 已批准 confirmation 缓存**（避免反复弹窗）—— 一个 P2 候选，~60 LOC，等真实痛点出现再落地。

其它都是 lynx 已有等价能力 / 或主动让位给 chat 中间件 / 或属于用户应用代码自己实现的范畴。**职责边界比 ADK 那次更明确：Crush 是产品的产品 UX，不是 framework 抽象**。
