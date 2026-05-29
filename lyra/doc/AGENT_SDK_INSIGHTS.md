# Claude Agent SDK (TypeScript) —— 对 lynx `agent` / `lyra` 的启发

> 来源：<https://code.claude.com/docs/en/agent-sdk/typescript>
> 本文是**第二轮**分析（2026-05-29 重写）。第一轮写于 `agent` 模块大改之前；这几小时 `agent` 落地了持久化 / Supervisor / per-call 成本 / 预算 / OTel metrics / tool-loop 恢复 / artifact 工具，使对标结论发生**反转**，故整体重写。
> 配合 [`ARCHITECTURE.md`](./ARCHITECTURE.md) / [`ROADMAP.md`](./ROADMAP.md) 阅读。

---

## 0. 先划清边界：哪些 SDK 部分根本不适用

Claude Agent SDK 的本质是**包一个 `claude-code` CLI 子进程**，对外 async-generator API。结构差异：

| 维度 | Claude Agent SDK | lynx `agent` / `lyra` |
|---|---|---|
| 运行形态 | 包 CLI **子进程**，跨进程 control protocol | 纯 **in-process Go 库** |
| 决策循环 | ReAct-loop | **planner-driven（GOAP/HTN…）+ ReAct（Supervisor）混合** |
| 工具协议 | 一切走 MCP | `core` 原语 + MCP 适配，typed action |

**直接排除为噪音**（in-process 不需要）：`startup()`/`WarmQuery`、`spawnClaudeCodeProcess`、`plugins`、四层 `settingSources`。

---

## 1. 核心反转：gap 从「agent vs SDK」变成「lyra vs agent」

第一轮的结论是"我们内核比 SDK 干净，但缺一些控制面 / 持久化 / 编排"。这几小时 `agent` 框架**把那些缺口大多补上了，而且比 SDK 更狠**（OTel-native、typed、planner+ReAct 混合）。

**但 `lyra` 几乎一个新能力都没用上**——它现在仍是个"薄 chat 包装"：只 `StartAgent` 一个单 chat action，外加我这轮接的 tool-loop 恢复。snapshot / Supervisor / sub-agent / invocation 成本 / BudgetPolicy / metrics 一概没消费。

> 这正好绕回最初那句 **"自己的东西自己都不用，别人更没说服力"**。第二轮最大的启发不是"给框架加东西"，而是 **让 lyra 真正吃上框架的新肌肉** —— 这同时补齐 lyra 对标 SDK 的 gap，又兑现"lyra = agent 框架最佳实践"。

---

## 2. 现状对照表（SDK 能力 → agent 框架现状 → lyra 是否消费）

> 图例：✅ 有且不差 · 🟡 有雏形 · 🔴 缺 · ⛔ 不适用/不学

| SDK 能力 | agent 框架现状（今天） | lyra 消费？ | 评价 |
|---|---|---|---|
| session 持久化 / resume / `sessionStore` | ✅ `core.ProcessStore` + `persistence.FileStore` + `PlatformConfig.AutoSnapshot`（每 tick 落盘） | 🔴 没接 | 进程级 typed 快照，restore 后**重新 plan**（非续跑 in-flight action） |
| subagent 编排（`agents` / Task / background） | ✅ `workflow.Supervisor[In,Out]`（LLM 编排）+ `SubagentTools` / `AsChatTool`（子 agent 当工具）+ `SpawnChild` | 🔴 没接 | 子 agent **同步**跑、继承 blackboard；HITL-aware（waiting→JSON 回给 LLM）。无 background/async |
| `total_cost_usd` / `modelUsage` / `usage` | ✅ `core.LLMInvocation`（model+provider+**cost(USD)**+prompt/completion/reasoning/**cache** tokens+action）逐调用记录 | 🔴 没接（链路未 record） | **比 SDK `modelUsage` 还细**（带 cache-token 拆分） |
| `maxBudgetUsd` / `taskBudget` | ✅ `process_budget` 子树聚合 + `BudgetPolicy` 早停 | ✅ token + **cost** ceiling（`MaxBudget` / `MaxCostUSD`，`f48c423`） | 已对齐 SDK `maxBudgetUsd`（subtree-inclusive，round-boundary 停） |
| tool 错误恢复 / `maxTurns` | ✅ `FeedbackOnUnknownTool` / `FeedbackOnEmptyResponse` + max-iter cap | ✅ **接了**（`ActionConfig.ToolLoop`） | lyra 幻觉工具名会自纠不 abort |
| 结构化 tool 结果 / content blocks | ✅ `ArtifactTool` → `ToolResult{Content, Artifact}`（artifact 不进模型） | 🔴 没用 | 适合产物型工具（diff / 图片 / 结构化数据） |
| 可观测（usage in result） | ✅ OTel metrics（tick/action/plan/exit 计数+直方图）+ invocation 审计 | 🟡 自动 emit，未专门消费 | **lynx 领先**：production-native，非 result 里塞 usage |
| permission 梯度（plan/auto/dontAsk） | — | ✅ lyra `safe/balanced/yolo/readonly` | D 已做（3-way gate） |
| `query()` 流式 | — | ✅ lyra chat = AgentProcess + event 通路 | 已对齐 |
| `outputFormat: json_schema` | ✅ `chat.JSONParser[T]` | — | closed-by-design |
| `canUseTool`(updatedInput/remembered) | — | 🔴→⛔ | 违反前端 CLOSED 决议（wire 只 approve/deny），**不做** |
| steering（`interrupt`/`setModel`/`setPermissionMode`） | — | 🔴 缓 | 前端只预留 `lyra.interrupt/resume`，无 spec，不单方面加 wire |
| `enableFileCheckpointing` / `rewindFiles` | — | ⛔ | YAGNI（snapshot 是状态级非文件级） |
| 60+ 字段 `Options` bag / 40+ 变体 `SDKMessage` union | — | ⛔ | 反面教材，见 §6 |

---

## 3. 我们现在反而**领先** SDK 的地方（别妄自菲薄）

- **可观测性**：`LLMInvocation` 带 **cache-token 读写拆分** + OTel metrics，比 SDK 的 `modelUsage` 细且 production-native。SDK 只在 `result` 里回 usage。
- **状态快照**：存的是 typed planner state + blackboard + budget + 条件集，能恢复"agent 在想什么"；SDK `sessionStore` 只存对话 transcript（"说了什么"）。
- **planner + ReAct 混合**：`Supervisor` 是 ReAct，`workflow` 是 GOAP 聚散/共识/RepeatUntilAcceptable(best-of-N)；比 SDK 纯 ReAct 灵活。
- **Extension 类型分发**（`ActionMiddleware` / `ToolDecorator` / `GoalApprover` / `EventListener` …，靠 `collectExtensions[T]`）是比 SDK stringly-typed `hooks` **更干净的钩子系统**——加能力实现接口即被 dispatch 发现，不动 dispatch loop。

---

## 4. 这轮已经做了的（lyra 侧，实施记录）

| 项 | 结局 | commit |
|---|---|---|
| 组织重构（dispatch 查表、chat inmemory 拆分、OnlineConfig 去重） | ✅ | `03adf01` |
| 可观测事件（`CompactBoundary` / `MemoryUpdated`） | ✅ | `9b4266b` |
| approval 对齐 wire（删死的 `AllowAlways`、enum→approve/deny）+ `ModeReadOnly` | ✅ | `8645edd` |
| per-action 工具循环 config + lyra 开 `FeedbackOnUnknownTool`（跨 agent） | ✅ | `73ddaf1` |
| token budget 上限（`MaxBudget`，工具循环边界停） | ✅（混入 `231cd21`） | — |
| `messages.list`（wire-addressable 消息层，稳定 `m<n>` id） | ✅ | `c7d8605` |
| message-level fork（子会话拷贝父历史前缀） | ✅ | `d676e46` |
| **C1 用框架 invocation ledger**（record per-round → 读回 total + per-model `UsageByModel`，弃私有 tally） | ✅ | `9b73fb6` |
| **C3 `task` 子 agent 委派**（`AsChatToolFromAgent`/`SpawnChild`，两 role 防递归，subtree usage 自动并入） | ✅ | `6f7d59d` |

**deferred-tool（第一轮的"最高价值"项）：判定不做 ✅。** 第二轮专门复核了新 snapshot——`ProcessSnapshot` 只存进程级状态，**不存 mid-action 执行点**，restore 是重新 plan。所以"工具边界冻结 + 不重跑 LLM resume"在 runtime 层做不到（要做就是 mid-action snapshot，巨大侵入）。而 lyra 现有的 **in-process 同步 gate**（`observedTool.Call` 在工具执行前 park 在 `approval.Decide` 的 channel 上）**已经做到了工具边界审批 + 不重跑 LLM**。新持久化没有改变这个结论。

**事件命名遗留**：`compact_boundary` / `memory_updated`（及既有的 `plan_generated` / `tool_call_approval`）尚未对齐前端 `lyra.*` CUSTOM 命名约定。按你决定**留到后端整体 cutover 时统一改**，不在这轮动。

---

## 5. 还能做的：让 lyra 吃上 agent 新肌肉（= 同时补 lyra 的 SDK gap）

### C1. per-call usage ledger → `TurnEnd`（✅ 已做 `9b73fb6`）
lyra chat action 不再私攒 token tally：每轮经 `pc.RecordLLMInvocation` 记进框架的 invocation ledger（model + prompt/completion/reasoning/cache），turn 结束从 `pc.Process.LLMInvocations()` 读回 total + per-model `UsageByModel`（SDK `modelUsage` 的对标）；budget 检查读 `pc.Process.Usage()`。cost(USD) 仍为 0，待 pricing 层填 `LLMInvocation.Cost`（映射已就位）。

### C3. `task` 子 agent 委派（✅ 已做 `6f7d59d`）
`AsChatToolFromAgent[TaskInput,string]` + `SpawnChild` 把"委派子任务"做成 `task` 工具；两 tool role（coding=leaf+task / subtask=leaf）防递归；子 agent 无 observer（工作对父 turn 不透传，只回最终答案），其 LLM 轮次经 subtree budget 自动并入父 turn 的 per-model usage（C1 协同）。对标 SDK Task/subagents。

### C2. cost 全通了（✅ 导管 `1b8f3a2` + 定价表/Metadata `c77a051`）
- 导管：`engine.Config.Pricing` → `invocationFrom` 填 `LLMInvocation.Cost` → `chatOutput` 汇总 → `ChatOutput.CostUSD` + per-model → `TurnEnd.CostUSD`。
- **定价表（仿 catwalk）**：`models/pricing` 嵌入式 per-provider JSON 目录(`configs/anthropic.json`/`openai.json`,input/output/cache 每-1M 费率)+ `Lookup`。维护 = 改 JSON。
- **经 Metadata 暴露**:`chat.Pricing` 类型(放 `core/model/chat`,因为 token 费率是 chat-model 概念,不进泛型顶层包)+ `chat.ModelMetadata.Pricing`;adapter 的 `Metadata()` 从目录填本模型费率;lyra `BuildChatClient` 读 `llm.Metadata().Pricing` 建导管。conduit 吃全量 `*chat.Usage`(cache-aware)。
- 未配/模型不在表 → cost=0(不臆造)。`maxBudgetUsd` 现可基于 `CostUSD` 加(token 上限已够用)。

### C4. 持久化 / durable HITL（分层进行中）
- **Tier 1 持久化导管 ✅（`5403b9e`）**：`engine.Config.ProcessStore`（经 `runtime.Config` 透传）→ `PlatformConfig.AutoSnapshot`，给 store 就每 tick 落盘,不给零开销。snapshot 是进程级（status/blackboard/history/budget），可落盘审计 + 是 durable HITL 的地基。
- **前置 unblock ✅（`4d7104a`，agent/core）**：typed action 之前不能 `AwaitInput`(`typedAction.Execute` 永远返回 Succeeded)。已补：fn park awaitable → wrapper 返回 `ActionWaiting`。这是 lyra 作为最佳实践**暴露并反哺框架缺口**的范例——typed HITL 现在整个框架可用。
- **Tier 3 durable plan-mode HITL ✅（`265cb4e`）**：plan-mode 暂停已从 chat service 的内存 channel 挪进 agent 进程的 `AwaitInput`→`StatusWaiting`(被 Tier 1 持久化),`ContinuePlan`→`ResumeProcess`+`ContinueProcessAsync`。action 里 `planGate` 先产 plan→`OnPlanGenerated`→park,resume 后按 blackboard decision 执行/拒绝/(NO_PLAN 直接跑);`Engine.GeneratePlan` 删、`runTurn` 改 resume 循环、`ChatProcess.Resume` 加。既有 plan-mode 服务测试(`buildPlanService` = 真 engine)现在跑的就是真 Waiting/resume 路径,全绿。下方"实施方案"段为落地记录。
- **Tier 2 跨重启自动恢复**:启动时扫 store、重连 Waiting 进程到新 event stream —— 最远、当前 ROI 最低,Tier 3 落地后再议。

#### C4 Tier 3 —— focused-round 实施方案（已审计，待执行）

**现状**：plan-mode 是两次独立 agent 调用 + 一个内存 channel。`runTurn`(turn.go) 先调 `s.engine.GeneratePlan`(engine.go:235,一次性 LLM 产 plan 字符串)→ emit `PlanGenerated` → block 在 `turnState.planDecision`(buffered chan,turn.go:40)上 → `ContinuePlan`(inmemory.go:170)往 channel 发 decision → 批准后才 `StartChat` 真正执行。暂停**不持久化**。

**目标形态**（mirror `agent/runtime/typed_await_test.go` —— 这就是 `4d7104a` 解锁的范式）：plan 的生成+暂停+执行合成**一个** agent 进程,暂停走 `pc.AwaitInput`→`StatusWaiting`(被 Tier 1 的 `AutoSnapshot` 落盘),`ContinuePlan`→`ResumeProcess`+`ContinueProcess`。

**逐文件改动**：

1. `engine/agent.go` —— plan-aware 的 `chat` action：
   - `ChatInput` 加 `PlanMode bool`。
   - action body：先查 blackboard 的 decision condition(如 `"plan.approved"`)。
     - **未决**：产 plan(把 `GeneratePlan` 的 planner 调用内联进来)→ 经 observer emit `PlanGenerated` → `pc.AwaitInput(hitl.NewConfirmation(plan, func(ok bool){ pc.Blackboard.SetCondition("plan.approved", ok); return core.ImpactUpdated }))` → typed wrapper 见 `InputAwaited` 返 `ActionWaiting` → 进程 `StatusWaiting`。
     - **已批准**：跑现有 `runChatTurn`。
     - **已拒绝**：返 `ChatOutput{PlanRejected: true}`。
   - `NO_PLAN`(空 plan)分支:不 await,直接执行(保留现有 trivial-request 跳过审批语义)。
2. `engine/engine.go` —— `ChatProcess` 接口加 `Resume(approved bool) error`;`chatProcess.Resume` = `platform.ResumeProcess(id, approved)` + `platform.ContinueProcess(ctx, id)`。`RunChatRequest` 加 `PlanMode bool` → 透传进 `ChatInput`。
3. `service/chat/engine.go` —— 窄接口 `Engine`：**删 `GeneratePlan`**(plan 现在在 action 内);`ChatProcess` 多 `Resume`。
4. `service/chat/turn.go` —— **删 `runPlanMode`**;`runTurn` 改 **resume 循环**：`StartChat`(带 PlanMode)→ `for { <-proc.Done(); if proc.Status()==StatusWaiting { 标记 plan-pending,跳出等 ContinuePlan } else break }`。`PlanGenerated` 由 action 经 observer 发,不再由 runTurn 发。
5. `service/chat/turn.go` `turnState` —— **删 `planDecision` channel / `waitDecision`**;`ContinuePlan`(inmemory.go)改成调 `st.proc.Resume(decision==PlanApprove)`,resume 后进程续跑到 `Done()`(完成或再 Waiting),`runTurn` 循环捕获终态 → `emitTurnEnd`。
6. `service/chat/turn.go` `emitTurnEnd` —— 处理 `ChatOutput{PlanRejected}` → 一个干净的 TurnEnd reason(复用或加 `TurnEndPlanRejected`)。
7. **测试**(回归重灾区,逐一重写)：
   - `service/chat/engine_test.go` 的 **stubEngine**：去掉 `GeneratePlan`;stub `ChatProcess` 要能 fake "首个 `Done()` 时 `Status()==Waiting`(plan 待批)→ `Resume` 后再 `Done()` 且 `Completed`"。
   - plan-mode 三测(inmemory_test.go / engine_test.go / translator_test.go 相关):驱动新 resume 流,断言 PlanGenerated→approve→执行、reject→PlanRejected、NO_PLAN→直接执行。

**风险**：回归面全在 working plan-mode + turn 生命周期(`emitTurnEnd` 的终态映射)。`runTurn` 的 resume 循环与 `ContinuePlan`(外部触发 Resume)的同步是最 fiddly 处——`ResumeProcess`+`ContinueProcess` 同步驱动 tick,要确保 `Done()` 在 park 与 resume 后各 fire 一次且不竞态。建议每步 `go test ./internal/...` 全绿再进下一步。

### 仍卡在前端协调上（非 lyra 单方面可推）
- **`messages.edit`** —— 协议要返 `{runId, checkpoint}`，但 lyra 无 checkpoint 模型（capability 标 false，前端仅预留）。要先定 rewind/checkpoint 语义。
- **session tag**（SDK `tagSession`）—— `Session` 加 tag 是 wire-shape 改动，前后端先对齐。
- **steering RPC**（`lyra.interrupt/resume`）/ **后台任务通知** —— 等前端 spec / 等 lyra 有后台特性。

---

## 6. 反面教材：SDK 这两处明确不学

- ❌ **60+ 字段的 `Options` god-bag**：违反我们"链式 + 按 concern 分区 + 窄接口"的纪律，也违反 ISP（一个 bag 表达不了"谁用哪几个"）。保持配置按 domain 分散。
- ❌ **40+ 变体的 `SDKMessage` mega-union**：我们 lyra chat `Event` 按 domain 分类型、agent `event` 是 `Multicast`+窄 `Listener`——加类型不撑大任何 interface，比 mega-union 更符合 ISP（"接口越大抽象越弱"——Rob Pike）。

---

## 7. 协议契约约束（动这些前必看）

`lyra` 的 wire 形态是**前后端共享的 CLOSED 契约**（前端仓 `docs/API.md` + `PROTOCOL_ALIGNMENT_2026-05-28.md`）。第二轮踩实的几条硬约束：

- **CUSTOM 事件用 `lyra.*` 前缀**（`lyra.plan` / `lyra.approval` / 预留 `lyra.compaction.*` / `lyra.interrupt` / `lyra.resume` / `lyra.background.*`）。后端 translator 当前命名未对齐，待整体 cutover。
- **`ApprovalDecision` = `approve` | `deny` 两值闭集**。"记住选择 / 永久允许"是**前端 UI**，不进 wire / 后端。
- **动 `rpc/protocol/` 任何方法或 shape，前后端先对齐**（CLAUDE.md 硬规矩）。这就是 §5 里 steering / tag / edit 都"缓"的原因。

---

## 附：SDK 完整 API 速查（备查，不代表建议采纳）

- **入口**：`query()`（async generator）/ `startup()`+`WarmQuery` / `listSessions` / `getSessionMessages` / `renameSession` / `tagSession` / `resolveSettings`。
- **`Query` 句柄**：`interrupt` / `rewindFiles` / `setPermissionMode` / `setModel` / `setMaxThinkingTokens` / `streamInput` / `stopTask` / `setMcpServers` / `reconnectMcpServer` / `supportedAgents` / `mcpServerStatus`。
- **工具/MCP**：`tool(name, desc, zodSchema, handler, {annotations})` / `createSdkMcpServer` / `McpServerConfig`(stdio/sse/http/sdk/proxy) / `toolAliases` / `allowedTools` / `disallowedTools`。
- **权限**：`permissionMode`(default/acceptEdits/bypassPermissions/plan/dontAsk/auto) / `canUseTool`(allow+updatedInput+updatedPermissions | deny+interrupt)。
- **Hook**：`hooks` / `includeHookEvents` / `defer` → `tool_deferred` + `deferred_tool_use` + resume。
- **Subagent**：`agents: Record<string, AgentDefinition>` / `agent` / `background` / `parent_tool_use_id` / `agentProgressSummaries` / `SDKTaskNotificationMessage`。
- **Session**：`persistSession` / `resume` / `resumeSessionAt` / `forkSession` / `sessionStore`。
- **预算/用量**：`maxBudgetUsd` / `taskBudget` / `total_cost_usd` / `usage` / `modelUsage`。
- **其他**：`outputFormat: json_schema` / `thinking`(adaptive/extended/disabled) / `effort` / `enableFileCheckpointing`+`rewindFiles` / `includePartialMessages` / `SDKCompactBoundaryMessage` / `SDKMemoryRecallMessage` / `onElicitation`。
