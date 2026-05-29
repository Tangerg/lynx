# Claude Agent SDK (TypeScript) —— 对 lynx `agent` / `lyra` 的启发

> 来源：<https://code.claude.com/docs/en/agent-sdk/typescript>
> 视角：我们已经有一套 planner-driven 的 `agent` runtime + `lyra` backend。这份文档**不是抄 SDK**，而是把它在生产里验证过的"控制面 / API 形态学"挑出来，对照我们现状，给出**采纳 / 吸收改造 / 明确不做**三档建议。
> 配合 [`ARCHITECTURE.md`](./ARCHITECTURE.md) / [`ROADMAP.md`](./ROADMAP.md) 阅读。

---

## 0. 先划清边界：哪些根本不适用

Claude Agent SDK 的本质是**包一个 `claude-code` CLI 子进程**，对外暴露 async-generator API。它和我们最大的结构差异：

| 维度 | Claude Agent SDK | lynx `agent` / `lyra` |
|---|---|---|
| 运行形态 | 包 CLI **子进程**，跨进程 control protocol | 纯 **in-process Go 库**，无 subprocess |
| 决策循环 | ReAct-loop（model 自己决定下一步） | **Planner-driven**（每 tick planner 看 world-state + goal 产出 plan） |
| 工具协议 | 一切走 MCP（含内置工具） | `core` 原语 + MCP 适配，typed action |
| API 形态 | 一个 `query()` 返回 async generator of messages | `StartAgent` → `(*AgentProcess, <-chan error)` + event multicast + blackboard result |

**因此下列 SDK 特性对我们是噪音，直接排除**，不必纠结：

- `startup()` / `WarmQuery` —— 子进程预热，把 spawn 成本移出关键路径。我们无 subprocess，N/A。
- `spawnClaudeCodeProcess` / `executable: bun|deno|node` / `pathToClaudeCodeExecutable` —— 进程管理细节。N/A。
- `plugins` / `settingSources` / `managedSettings` 这套四层 settings 解析 —— claude-code 专有的配置装配，和 `lyra` 的 `LYRA_*` env + `config.toml` 不是一个量级的问题。N/A。

**还有两处是"反面教材"，明确不学**（见 §4）：SDK 的 60+ 字段 `Options` god-bag、40+ 变体的 `SDKMessage` mega-union —— 正好踩我们 CLAUDE.md 的 god-config / 大接口红线。

排除噪音后，剩下的才是真金。

---

## 1. 核心形态对照表

> 状态图例：✅ 我们已有且不差 · 🟡 有雏形可吸收 SDK 思路 · 🔴 缺口值得补 · ⛔ 不适用/不学

| SDK 概念 | lynx 对应 | 状态 | 一句话评价 |
|---|---|---|---|
| `query()` → `AsyncGenerator<SDKMessage>` | `Engine.StartChat` → `ChatProcess` + chat `Event` chan | ✅ | 我们刚把 chat turn 做成真 `AgentProcess`，方向一致 |
| `Query` 控制句柄（`interrupt` / `setModel` / `setPermissionMode` …） | `ChatProcess`（`Cancel` / `Done` / `Status` / `Output`） | 🟡 | 句柄上**只挂了取消**，steering 方法可以补 |
| 流式输入 `prompt: AsyncIterable` + `shouldQuery:false` | `Engine.InjectUserMessage` | 🟡 | "追加但不触发新 turn"这个语义值得显式化 |
| `canUseTool` 返回 `updatedInput` / `updatedPermissions` | `approval.Gate` / `Console`（allow/deny only） | 🔴 | 我们只能 allow/deny，**不能改写入参、不能记住决策** |
| `permissionMode: plan/auto/acceptEdits/dontAsk` | lyra `safe/balanced/yolo` + plan mode（M6） | 🟡 | `auto`（分类器审批）和 `dontAsk` 语义可借鉴 |
| **Hook `defer` → `tool_deferred` + resume** | （搁置中的 hitl 方案） | 🔴 | **正好回答上次卡住的"工具审批 = LLM re-run"难题** |
| `agents: Record<string, AgentDefinition>` + `background:true` | Platform agent registry + child spawn + `workflow/` | ✅🟡 | 原语都有，缺"声明式注册 + 后台任务进度"封装 |
| `parent_tool_use_id` 串联 subagent 输出 | `BindProtected` 跨子进程传值 + event scope | 🟡 | 父子关系有，但事件流里没显式 parent 标记 |
| `resumeSessionAt(messageUUID)` / `forkSession` / `tagSession` | lyra session fork（M3） | 🟡 | fork 有计划，"resume-at-message"和 tag 可加 |
| `sessionStore` 外部后端接口 | lyra `FileSessionService` / sqlite | ✅ | 我们已是可换后端，理念一致 |
| `SDKCompactBoundaryMessage` / `SDKMemoryRecallMessage` | lyra compactor / extractor（M5） | 🔴 | **compaction / memory recall 应作为可观测事件吐出** |
| `maxBudgetUsd` / `taskBudget` + `modelUsage` | `Process.RecordUsage` + chat `TurnEnd.Usage` | 🟡 | usage 有，但**预算上限不是一等入参** |
| `enableFileCheckpointing` + `rewindFiles(msgId)` | 无 | ⛔(标记) | 强能力但 YAGNI —— 先记下不建 |
| `outputFormat: json_schema` + retry | `core/model/chat.JSONParser[T]` | ✅ | 已 closed-by-design，不动（见根 memory） |
| `effort` / `thinking` config | chat Reasoning first-class | ✅ | 已覆盖 |
| `toolAliases`（built-in→MCP 重定向）/ `setMcpServers` 运行时重配 | MCP client（M7）+ `ToolGroupResolver` extension | 🟡 | 运行时热重配 MCP 是个好点子 |
| 60+ 字段 `Options` bag | —— | ⛔ | god-config，**反面教材**，见 §4 |
| 40+ 变体 `SDKMessage` union | lyra 分类型 chat `Event` | ⛔ | mega-union，**反面教材**，我们的 ISP 切分更好 |

---

## 2. 高价值启发（建议采纳 / 吸收改造）

每条格式：**SDK 怎么做 → 我们现状 → 缺口 → 建议动作 → 踩哪条原则**。凡涉及 exported API 改动的，都标注 `⚠️ 破坏性，需先咨询`。

### A. 控制句柄上挂 steering 方法（`Query.interrupt/setModel/…`）

- **SDK**：`query()` 返回的 `Query` 不只是一个 stream，它还是个**控制面**：`interrupt()` / `setPermissionMode()` / `setModel()` / `setMaxThinkingTokens()` / `stopTask()` / `streamInput()` / `mcpServerStatus()` 全挂在同一个句柄上。一个对象同时是"输出流"和"遥控器"。
- **我们现状**：`lyra` 刚做的 `ChatProcess` 句柄（`internal/engine/engine.go`）只有 `Done/Status/Failure/Output/Cancel`——纯查询 + 取消。底层 `runtime.AgentProcess` 其实已经有 `KillProcess/ResumeProcess/ContinueProcess`，能力是有的，只是没在句柄上暴露成"遥控器"。
- **缺口**：turn 跑起来后想"换模型 / 切 permission mode / 插一条 steering 消息"，目前没有统一入口。
- **建议**：保持我们的"句柄是具体类型、入参是接口"风格，给 `ChatProcess` 渐进加少量 steering 方法（按真实需求，不要一次铺满）。**别学 SDK 把十几个方法全塞一个 interface**——按 §4 的 ISP，谁用 steering 谁定义自己那侧的窄接口。
- **原则**：✅ 这是 OCP/已发生的扩展需求（chat 已经要 cancel 了，steering 是同一族）。⚠️ 给 `ChatProcess` interface 加方法是破坏性的，**需先咨询**。

### B. ⭐ Deferred tool（`hook defer → tool_deferred → resume`）—— 直接解上次搁置的 hitl 难题

- **SDK**：工具审批它**不在工具边界同步阻塞**，也**不重跑 planner**。`PreToolUse` hook 返回 `{permissionDecision: "defer"}` → 本次 turn 直接以 `stop_reason: "tool_deferred"` 结束，结果里带 `deferred_tool_use: {id, name, input}` → 上层拿到后异步决策 → 用**同一个 `session_id` resume**，从那个工具调用点**继续**，而不是从头 re-run。
- **我们现状（为什么上次卡住）**：上一轮讨论 tool 审批接 `agent/hitl` 时，结论是 hitl 的 action-exit 模型会导致"每次审批 = 一次完整 LLM re-run"，还得加 approval cache 去 dedup。成本和复杂度都高，所以 `lyra` 把这条**搁置**了。
- **SDK 给的启发**：`tool_deferred` 是第三条路——**冻结在工具边界**，而不是冻结在 action 边界。turn 干净退出、把"待决策的工具调用"作为结构化输出交出去；resume 时**直接执行那个被批准的工具**（带可能被改写的 input），planner / LLM 不重跑。这正好绕开了"re-run + cache"的代价。
- **建议**：重启那条搁置的 hitl/approval 工作时，**优先评估能否在 `agent/runtime` 的 tick 循环里支持"工具边界冻结 + 携带 pending tool-use 退出 + resume 续跑该工具"**，而不是套 action-exit 的 `AwaitInput`。如果 runtime 能原生支持工具级 defer，lyra 的 `ToolCallApproval` 事件 + `approval.Decide` 就能拼成一条不重跑的链。
- **原则**：这是把上次的 A/B/C 选项**新增了一个更优的 D 选项**。⚠️ 涉及 `agent/runtime` 的 tick 循环和 `core.Process` 语义，是核心改动，**必须先出方案 + 咨询**后才动。这条是本文档**最高价值**的一条，但也最重，留给用户决定何时启动。

### C. 审批回调能"改写入参 + 记住决策"（`canUseTool` 的 `updatedInput` / `updatedPermissions`）

- **SDK**：`canUseTool(toolName, input, opts)` 返回的不只是 allow/deny：
  - `behavior:"allow"` 可带 `updatedInput`（**改写工具入参后再放行**，例：把 `rm -rf /` 收窄成 `rm -rf ./tmp`）和 `updatedPermissions`（**把这次决策固化成规则**，下次同类自动放行）。
  - `behavior:"deny"` 可带 `interrupt:true`（不只是拒这个工具，直接中断整个 turn）。
- **我们现状**：`lyra` 的 `approval.Gate`/`Console` 是纯 allow/deny 二值。
- **缺口**：(1) 不能"批准但修正参数"；(2) 不能"记住这类决策"（M4 的 approval cache 计划接近 `updatedPermissions`，但还没做成审批回调的返回值）；(3) deny 不能选择是否中断 turn。
- **建议**：M4 做 approval 时，把审批结果设计成**富返回**而不是 bool——至少支持 `updatedInput`（改写）和"记住规则"。`interrupt` 语义可对齐 §B 的 turn 退出。
- **原则**：DIP/ISP——审批结果类型定义在消费侧（approval 包）。这是 M4 的设计输入，不是破坏性改动（M4 还没实现）。

### D. permission mode 的语义梯度（`plan` / `auto` / `acceptEdits` / `dontAsk`）

- **SDK**：permission 不是开关而是**梯度**：`plan`（只读，先规划）/ `acceptEdits`（自动批编辑）/ `auto`（**模型分类器**逐个判定）/ `dontAsk`（不问，未预批的直接拒）/ `bypassPermissions`（全过）。
- **我们现状**：lyra 三档 `safe/balanced/yolo`（M4）+ plan mode（M6）。
- **可吸收的两点**：
  - `auto`（model classifier 审批）——比"静态规则表"更灵活，适合"大部分自动、危险的才问"。是 `balanced` 的一个更聪明的实现策略。
  - `dontAsk`——"非交互但安全"场景（CI / 批处理）：未预批即拒，不阻塞等人。lyra 跑 headless 时有用。
- **建议**：M4/M6 设计 mode 时把这两个语义纳入考量，别只做"全问/不问"两极。
- **原则**：OCP——mode 是策略，加 mode 不应改审批 dispatch。映射到我们 `collectExtensions[T]` 的 `GoalApprover` / tool decorator 思路。

### E. 声明式 subagent + 后台任务 + 进度摘要

- **SDK**：`agents: Record<string, AgentDefinition>` 声明式注册子 agent（各自 tools/model/prompt/maxTurns）；`background:true` 让 subagent **非阻塞跑**，完成时发 `SDKTaskNotificationMessage`；`agentProgressSummaries` 给后台任务一行摘要；`parent_tool_use_id` 把子 agent 的输出串回父 transcript。
- **我们现状**：`agent` 有 Platform registry + 并发 child spawn + `BindProtected` 跨进程传值 + `workflow/`（ScatterGather/Consensus/Parallel）。原语**比 SDK 更强**（我们能做共识、聚散）。lyra M2 有 `task` 工具委派子任务。
- **缺口**：(1) 没有"声明式 agent 定义"的薄封装——目前是 fluent Builder 逐个构造；(2) 后台任务**完成通知 + 进度摘要**没有成型的事件；(3) 事件流里没有显式的 `parent_tool_use_id` 等价物去标记"这条 delta 来自哪个子 agent"。
- **建议**：
  - lyra 做 `task` 工具的多任务编排时，把"后台子任务完成"做成一类 chat `Event`（对齐 SDK `SDKTaskNotificationMessage`），让前端能渲染并行子任务进度。
  - 子 agent 的 event 带上 parent process id（我们 event 系统有 `ProcessID()`，把 parent 关系补进去即可），前端就能画嵌套 transcript。
- **原则**：YAGNI 警戒——不要为"声明式 agent registry"提前造抽象，**等 lyra 真有多 agent 场景再封**。后台通知事件是已发生需求（M2 `task` 工具就要），可以做。

### F. session：resume-at-message / tag / 外部 store

- **SDK**：`resume`（按 id）+ `resumeSessionAt(messageUUID)`（**从某条消息续**，配合 fork = 时间旅行）+ `forkSession` + `tagSession` + 外部 `sessionStore` 后端。
- **我们现状**：lyra M3 计划 `Fork(id, at_message_id)` + 树结构 + 可换 store（file/sqlite）。**已经覆盖大头**。
- **可补的小点**：
  - `tagSession`（给 session 打标签：archived / important）——轻量，列表管理有用。
  - `resumeSessionAt` 我们 fork 已经带 `at_message_id`，但"在同一条 session 上 resume 到某点"（非 fork）也值得显式支持。
- **原则**：✅ 方向一致，只是补两个小 API。M3 设计时纳入即可，非破坏性。

### G. 把 compaction / memory recall 作为可观测事件吐出

- **SDK**：上下文压缩会在 stream 里吐一条 `SDKCompactBoundaryMessage`；记忆召回吐 `SDKMemoryRecallMessage`。**内部维护动作对上层可见**。
- **我们现状**：lyra 有 `compactor` / `extractor`（M5），但它们是 turn 内部的"暗箱"操作——前端看不到"刚发生了一次压缩 / 注入了哪条记忆"。
- **缺口**：可观测性。用户/前端不知道"为什么上下文突然短了""这个回答用到了哪条 LYRA.md 记忆"。
- **建议**：M5 做 compaction/memory 时，定义两类 chat `Event`：`CompactBoundary`（压缩前后 token 数 + 摘要）和 `MemoryRecall`（命中的记忆条目）。这天然契合我们刚做的"chat turn = AgentProcess + event 监听"架构——compactor/extractor 通过 `agent/event` 发事件，lyra 监听器转成 chat `Event`。
- **原则**：✅ 高内聚 + 可观测性，且**正好复用我们刚建立的 event listener 通路**（`turnLifecycle` 那套）。非破坏性，是 M5 的事件设计输入。

### H. budget 作为一等入参 + per-model usage 分解

- **SDK**：`maxBudgetUsd`（client 侧成本上限，到顶停）+ `taskBudget:{total}`（API 侧 token 预算，模型自己节流）+ 结果里 `modelUsage:{[model]:ModelUsage}`（**按模型分解**用量）。
- **我们现状**：`core.Process.RecordUsage` + lyra `TurnEnd.Usage`——usage 有记录，但**预算上限不是入参**，也没有 per-model 分解（多 provider 混用时看不出谁烧了多少）。
- **建议**：
  - lyra `StartTurnRequest` 可加可选 `MaxBudget`（到顶则 `TurnEnd{Reason: BudgetExceeded}`）。这对 headless / 自动化跑批是刚需。
  - `TurnEnd.Usage` 扩成按 model 分解（子 agent / compaction 可能用不同 model）。
- **原则**：YAGNI 判断——预算上限是"已经能预见"的需求（跑批必然要），不是纯臆测，可做。⚠️ 改 `TurnEnd` / `StartTurnRequest` 是 exported wire 类型，**需先咨询**。

---

## 3. 一句话主线结论

> **我们 `agent` 框架的原语比 SDK 更干净**（planner-driven 而非 ReAct、typed action、Determination 三值逻辑、Extension 类型分发、workflow 聚散/共识）。**SDK 的价值不在内核，在控制面 / API 形态学 / 可观测性 edge。**

所以 `lyra` 作为"agent 框架最佳实践"的正确姿势是：

1. **内核照旧用我们自己的**——别因为 SDK 用 ReAct 就动摇 planner-driven。
2. **吸收 SDK 验证过的控制面 ergonomics**：steering 句柄（A）、deferred tool（B）、富审批结果（C）、permission 梯度（D）。
3. **补可观测性事件**：后台任务通知（E）、compaction/memory recall（G）——这几条正好复用我们刚做的 "chat turn = AgentProcess + event listener" 通路，是顺水推舟。
4. **session / budget 小补丁**（F/H）随各自 milestone 做。

**最高优先级的单点是 §B 的 deferred tool**——它给上次搁置的 hitl 难题提供了一条"不重跑 planner"的新解法，但因为动 `agent/runtime` tick 循环，必须先出方案再动手。

---

## 4. 反面教材：SDK 这两处明确不学

诚实记录——SDK 不是处处可抄，有两处正好踩我们 CLAUDE.md 的红线：

### ❌ 60+ 字段的 `Options` god-bag

SDK 把所有配置（`model` / `permissionMode` / `hooks` / `mcpServers` / `maxBudgetUsd` / `thinking` / `sandbox` / …60 多个）塞进**一个** `Options` 对象。这是 TS/JS 生态的 functional-options 极端形态，但对我们是 **god-config 反模式**：

- 我们的纪律是**链式调用 + 按 concern 分区**（agent fluent Builder、lyra service 切片）。
- 一个 60 字段的 bag 无法表达"谁用哪几个"，正好违反 ISP。
- 维护者 90% 时间在读，god-bag 让"这个字段影响什么"无从追溯。

**保持我们的做法**：配置按 domain 分散到各 service / builder，跨包只传窄接口。

### ❌ 40+ 变体的 `SDKMessage` mega-union

SDK 的 `query()` 吐**一个** `SDKMessage` union，里面 40+ 个变体（assistant / user / result / system / hook×3 / task×4 / 各种 status…）。消费者得 type-switch 一个巨型 union。

- 我们 lyra 的 chat `Event` 是**按 domain 分类型**的（TurnStart / MessageDelta / ToolCall* / TurnEnd / …），消费者只关心自己那几类。
- agent `event` 是 `Multicast` + 窄 `Listener` 接口，加事件类型不撑大任何 interface。

**保持我们的做法**：分类型事件 + 窄 listener，比 mega-union 更符合 ISP（"接口越大抽象越弱"——Rob Pike）。

---

## 附：SDK 完整 API 速查（备查，不代表建议采纳）

- **入口**：`query()`（async generator）/ `startup()`+`WarmQuery`（预热）/ `listSessions` / `getSessionMessages` / `getSessionInfo` / `renameSession` / `tagSession` / `resolveSettings`。
- **`Query` 句柄方法**：`interrupt` / `rewindFiles` / `setPermissionMode` / `setModel` / `setMaxThinkingTokens` / `applyFlagSettings` / `streamInput` / `stopTask` / `close` / `supportedModels` / `supportedAgents` / `mcpServerStatus` / `setMcpServers` / `reconnectMcpServer` / `toggleMcpServer`。
- **工具/MCP**：`tool(name, desc, zodSchema, handler, {annotations})` / `createSdkMcpServer` / `McpServerConfig`（stdio/sse/http/sdk/proxy）/ `toolAliases` / `allowedTools` / `disallowedTools`。
- **权限**：`permissionMode`（default/acceptEdits/bypassPermissions/plan/dontAsk/auto）/ `canUseTool`（返回 allow+updatedInput+updatedPermissions 或 deny+interrupt）。
- **Hook**：`hooks: Record<HookEvent, …>` / `includeHookEvents` / **`defer` → `tool_deferred` + `deferred_tool_use` + resume**。
- **Subagent**：`agents: Record<string, AgentDefinition>` / `agent`（主线）/ `background` / `parent_tool_use_id` / `agentProgressSummaries` / `forwardSubagentText` / `SDKTaskNotificationMessage`。
- **Session**：`persistSession` / `sessionId` / `continue` / `resume` / `resumeSessionAt` / `forkSession` / `sessionStore` / `sessionStoreFlush`。
- **预算/用量**：`maxBudgetUsd` / `taskBudget` / 结果 `total_cost_usd` / `usage` / `modelUsage`。
- **其他**：`outputFormat: json_schema`（结构化输出 + retry）/ `thinking`（adaptive/extended/disabled）/ `effort` / `enableFileCheckpointing`+`rewindFiles` / `includePartialMessages` / `SDKCompactBoundaryMessage` / `SDKMemoryRecallMessage` / `onElicitation`。
