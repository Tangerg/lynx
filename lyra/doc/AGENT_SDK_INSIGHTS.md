# Claude Agent SDK (TypeScript) —— 对 lynx `agent` / `lyra` 的启发

> 来源：<https://code.claude.com/docs/en/agent-sdk/typescript>
> 本文是**第三轮**分析（2026-05-30 增补）。第一轮写于 `agent` 大改之前；第二轮（2026-05-29）整体重写，因 `agent` 落地持久化 / Supervisor / 成本 / 预算 / metrics 而结论反转。**第三轮不整体重写**（§4/§5 实施记录保留），只做：核对真实代码状态（两个 Explore agent 扫 `agent`/`core`/`runtime` + `lyra`，不信文档）+ 拉取 SDK 最新 API（发现 SDK 这半年也长出 background/task/deferred 等新能力）→ 局部更新对照表 + 新增 [§A 第三轮 gap 复盘](#a-第三轮-gap-复盘2026-05-30单方面可补项清零)。
>
> **第三轮的四个变化点**（相对第二轮）：(1) 全新 **model metadata catalog**（21 provider / 911 model，banded pricing + reasoning + modalities + limits + 下架日期，`models.dev` 生成器）—— SDK 完全没有这个概念；(2) **cost 端到端（lyra）从 🔴 → ✅** 全通；(3) **durable plan-mode HITL** 从内存 channel → 真 `AwaitInput`→`Waiting`→`Resume`；(4) **跨重启恢复框架前置** 修复（`RestoreProcess` determiner bug）+ 测试证明 restore-Waiting→resume。
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
| （SDK 无对应物）**model metadata catalog** | ✅ `chat.ModelInfo`/`ModelMetadata`（banded `Pricing` + `Reasoning` + `Modalities` + `Limits` + 下架日期）+ `models/internal/catalog`（21 provider / 911 model，`models.dev` 生成器） | ✅ 经 `BuildChatClient` 读 `Metadata().Pricing` 建 cost 导管 | **SDK 完全没有**——它只报 usage，不知道模型能干嘛/多少钱。这轮拉开的最大身位 |
| session 持久化 / resume / `sessionStore` | ✅ `core.ProcessStore` + `persistence.FileStore` + `PlatformConfig.AutoSnapshot`（每 tick 落盘） | 🟡 导管接了（`ProcessStore`→`AutoSnapshot`），**未做开机 restore** | 进程级 typed 快照，restore 后**重新 plan**（非续跑 in-flight action）。restore-Waiting→resume 框架已证明（`e79cd99`） |
| subagent 编排（`agents` / Task / **background**） | ✅ `Supervisor` + `SubagentTools`/`AsChatTool`（同步）+ `SpawnChild`；**`SpawnChildAsync`/`AsBackgroundChatTool`/`KillChildren`（async，`44ce90c`）** | 🟡 同步 `task` 工具已用（`6f7d59d`）；**background 工具框架就绪、lyra 未接**（FE-gated） | 同步子 agent 继承 blackboard + subtree budget 并入；**background 已补**（spawn/collect + stopTask 语义），lyra 消费等 `lyra.background.*` 协议 |
| `total_cost_usd` / `modelUsage` / `usage` | ✅ `core.LLMInvocation`（model+provider+**cost(USD)**+prompt/completion/reasoning/**cache** tokens+action）逐调用记录 | ✅ catalog→`invocationFrom`填 Cost→`TurnEnd.CostUSD`+`UsageByModel`（`1b8f3a2`） | **比 SDK `modelUsage` 还细**（带 cache-token 拆分） |
| `maxBudgetUsd` / `taskBudget` | ✅ `process_budget` 子树聚合 + `BudgetPolicy` 早停 | ✅ token + **cost** ceiling（`MaxBudget` / `MaxCostUSD`，`f48c423`） | 已对齐 `maxBudgetUsd`（subtree-inclusive，round-boundary 停）。`taskBudget`（API 侧 token pacing）⛔ 做不了，需 model API 原生支持 |
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

- **model metadata catalog（这轮拉开的最大身位）**：SDK **完全没有**模型能力/定价的概念——它只在 result 回 usage 数字。我们有 21 provider / 911 model 的 banded pricing（含 cache 读写费率 + 长上下文分层阈值）+ reasoning levels + modalities（text/image/audio/video/pdf）+ context/output limits + 下架日期，`models.dev` 驱动的 Go 生成器可重现。这能驱动 UI 选型、成本预估、能力门控——SDK 用户得自己维护这张表。
- **可观测性**：`LLMInvocation` 带 **cache-token 读写拆分** + OTel metrics，比 SDK 的 `modelUsage` 细且 production-native。SDK 只在 `result` 里回 usage。
- **状态快照**：存的是 typed planner state + blackboard + budget + 条件集，能恢复"agent 在想什么"；SDK `sessionStore` 只存对话 transcript（"说了什么"）。
- **planner + ReAct 混合**：`Supervisor` 是 ReAct，`workflow` 是 GOAP 聚散/共识/RepeatUntilAcceptable(best-of-N)；比 SDK 纯 ReAct 灵活。
- **Extension 类型分发**（`ActionMiddleware` / `ToolDecorator` / `GoalApprover` / `EventListener` …，靠 `collectExtensions[T]`）是比 SDK stringly-typed `hooks` **更干净的钩子系统**——加能力实现接口即被 dispatch 发现，不动 dispatch loop。

---

## A. 第三轮 gap 复盘（2026-05-30）：单方面可补项清零

> gap 重心**第二次转移**。第一轮：「agent 内核 vs SDK」。第二轮：「lyra 没吃上框架肌肉」。第三轮：**lyra 已吃上绝大部分肌肉**（cost / 双预算 / durable HITL / task 委派 / tool-loop 自纠 / 4 档 approval / 11 类事件全消费），剩余 gap 几乎全落在两类——(B) SDK 2026 新增的 background/task 异步生态，(C) 前端协议未定的项。**单方面能做的差不多做完了。**

### A.B 真实剩余 gap（SDK 有、我们缺或半成品）

| gap | 现状 | 能否单方面做 |
|---|---|---|
| ~~**background/async subagent** + `stopTask`~~ | ✅ **框架层已落地**（`44ce90c`）：`SpawnChildAsync` + `AsBackgroundChatTool`（spawn/collect）+ `KillChildren`/`KillProcess`(=stopTask)。见 [§A.实施方案](#a实施方案框架级-async--background-subagentfocused-round-框架层已落地-44ce90c) | lyra 消费 `task_progress`/通知仍需 FE 协议（`lyra.background.*` 仅预留） |
| **跨重启自动恢复**（lyra Tier 2） | 框架前置已就绪+证明（`e79cd99`）；lyra 只差「开机 restore Waiting → 重建 turnState → list 暴露 → 重连」 | 🔴 FE-gated（需 list/reconnect 协议） |
| **真·mid-turn steering**（`interrupt`/`streamInput`） | lyra steering 落到**下一轮**，非当前轮中断；框架有 `TerminateAction/ToolCall` 但无「注入消息续跑」 | 🔴 FE-gated（`lyra.interrupt/resume` 无 spec） |
| 运行时 `setModel`/`setPermissionMode` | 框架 model 锁定在 ProcessContext，无运行时切换 | 🟡 框架可做，但 YAGNI 信号强（无明确需求） |
| `ArtifactTool`（结构化工具产物） | 框架**有**（`chat.ArtifactTool`/`ToolResult{Content,Artifact}`），lyra **零消费** | ✅ lyra 单方面可接，但弱需求（工具来自共享集） |
| `messages.edit` + checkpoint + `rewindFiles` | lyra stub 返回 NotImplemented；无 checkpoint 模型 | 🔴 FE-gated（先定 rewind 语义） |
| `tagSession` | 没接 | 🔴 wire-shape 改动，先对 FE |

### A.C SDK 2026 新增（第二轮文档未覆盖）—— 逐项裁决

> 这半年 SDK 自己也长了不少。逐个判适配性，避免「SDK 有我们就得有」的盲目对标。

| SDK 新能力 | 判断 |
|---|---|
| `taskBudget`（API 侧 token 预算，模型自己 pacing 收尾） | ⛔ **做不了**——需 model API 原生「预算感知收尾」，我们只能客户端 round-boundary 硬停。SDK 包 CLI+Anthropic API 的红利 |
| `background` agents + `origin` + `forwardSubagentText` | background 见 A.B。`forwardSubagentText`（子 agent 文本透传父）我们**故意反向**（task 子 agent 只回最终答案，不污染父 turn）——设计选择差异，非缺陷 |
| `agentProgressSummaries` / `SDKTaskProgressMessage` | 依附 background；background 做了再说 |
| `deferred_tool_use` + `permissionDecision:"defer"` | ⛔ 第一轮已判不做——in-process 同步 gate 已实现工具边界审批+不重跑 LLM，deferred 是 SDK 跨进程才需要的 |
| `skills`（`AgentDefinition.skills`） | 🟡 lyra 有 **AGENTS.md 级联发现**（功能近似：给 agent 注入能力/上下文），形态不同，视为已有对应物 |
| `thinking`（adaptive/extended）+ `effort`（low..max） | ✅ 已查实（2026-05-30）：**框架侧不是 gap**——`chat.Options` 无可移植 effort 字段是**有意设计**，provider 原生参数（anthropic `Thinking` budget / openai `ReasoningEffort` enum）走 `Extra`+`GetParams` 逃生通道（两家形态差太多无法统一），catalog 的 `Reasoning{Levels,DefaultLevel}` 是展示/选型/成本用，不自动注入请求。**lyra 侧有个未接的小 config 旋钮**：`BuildChatClient` 只建 `NewOptions(model)`，reasoning 模型一律跑 provider 默认 effort；lyra 能解析 reasoning 输出但从不请求特定深度。属 FE-gated 产品旋钮（effort 是用户选择），无前端需求不单方面加 config（YAGNI） |
| `onElicitation` / `SDKElicitationCompleteMessage` | 🟡 MCP elicitation 回调——MCP 集成有，这条没接，弱需求 |
| `output_style` / `promptSuggestions` / `fastModeState` / `applyFlagSettings` | ⛔ 噪音（CLI/UI 或账户特性，与 in-process 库无关） |
| `SDKMessage` union 膨胀到 **30+ 变体** | 反面教材**再次印证**——更坚定不学 mega-union（见 §6） |

### A.结论

**lyra 单方面能补的 SDK gap 已清零。** 原本"唯一还值得在框架层投入"的 **background/async subagent** 也已落地（`44ce90c`，见 §A.实施方案）。**剩余全是 FE-gated 项**：lyra 消费 background（`lyra.background.*` 事件）/ cross-restart 恢复（list/reconnect 协议）/ mid-turn steering（`lyra.interrupt/resume`）/ edit-checkpoint / session tag —— 全卡在前端协议未定，不能单方面动 wire。而我们在 **model metadata catalog** 上已反超 SDK 一个身位。**下一步必须等前端 spec——后端侧已没有不依赖前端、还值得做的结构性项。**

### A.实施方案：框架级 async / background subagent（focused-round，✅ 框架层已落地 `44ce90c`）

> 对标 SDK `AgentDefinition.background` + `stopTask` + `task_progress`。**框架层已做完**（agent 模块，纯增量、不碰同步路径）；lyra 暴露给前端仍 FE-gated（需 `lyra.background.*` 事件，目前仅预留）。落地：`SpawnChildAsync`（child.go，`context.WithoutCancel` 跑后台、返回 taskID+done）+ `AsBackgroundChatTool[In,Out]`（subagent.go，spawn/collect 工具对，collect 报 running/waiting/done/failed）+ `Platform.KillChildren(parentID)`（turn 退出清理，复用 KillProcess + IsTerminal 守卫）。测试 `runtime -race` 全绿：done-channel 钉死时序 + 工具往返 + unknown-task + KillChildren 清扫。下文为设计记录。

**核心发现：gap 比预期窄，大半原语已就位。** 审计同步路径（`subagent.go`+`child.go`）后：后台跑进程（`Platform.ContinueProcessAsync` 返回 done channel、goroutine 跑）、子进程继承 blackboard + budget 并树（`CreateChildProcess`+`budget.addChild`）、查状态/取结果（`ProcessByID(id).Status()/Output()`）、取消（`KillProcess(id)` = SDK `stopTask`）、生命周期事件（child 已 emit `ProcessCreated/Completed/Failed`）——**全在**。`spawnChildOptions` 之所以同步，仅因它最后调 `ContinueProcess`（阻塞）而非 `ContinueProcessAsync`。所以这是**组装现有原语 + 加一个 LLM 面向工具形态**，纯增量、合 OCP。

**缺的四样：**
1. **async child spawn helper**：`SpawnChildAsync(ctx, platform, agent, in) (taskID, done, err)` —— `CreateChildProcess`+`Bind`+`ContinueProcessAsync`，立即返回 child id（对照同步 `SpawnChild`）。
2. **LLM 面向 spawn/collect 工具对**：把现在"一个 `task` 工具阻塞到完成"拆成 `spawn_task(prompt)→{task_id}`（非阻塞）+ `collect_task(task_id)→{status, result?}`（轮询收割）。模型 spawn 多个 → 继续干别的 → 回头 collect。
3. **parent 侧 task 反查**：列"我 spawn 的后台任务" —— **复用 `platform.procs`+parentID 反查**（child 已带 parentID），零新状态。
4. **收尾约定**：parent turn 结束/取消 → kill 未完成 child；result 走 `collect_task` 读 child `Output()` 显式收割。

**planner-driven 的天然契合**：SDK 靠往对话注入 message（`SDKTaskNotificationMessage`+`origin`）把后台结果塞回主循环；我们有 blackboard——后台 child 完成写 parent blackboard 的 task-keyed 槽，下一 planning tick 自然看见，比消息注入干净。`CreateChildProcess` 给 child 的是 parent blackboard 的 `Spawn()` **副本**（隔离），后台写不污染 parent，收割显式 → 无共享可变状态竞态。

**API 增量（最小集）**：
```go
// runtime/child.go —— 异步孪生（不改同步 SpawnChild）
func SpawnChildAsync(ctx, platform, agentDef, in) (taskID string, done <-chan error, err error)
// runtime/subagent.go —— LLM 面向工具对
func AsBackgroundChatTool[In, Out any](platform, agentDef) (spawn, collect chat.Tool, err error)
// KillProcess 已存在 → 直接当 stopTask，无需新增
```

**已定决策**：task registry = 复用 `platform.procs`+parentID（YAGNI，零新状态）；收割 = 显式 `collect_task` 轮询（自动写 blackboard 是后续增强）；parent 退出 = 自动 kill 未完成 child（避免泄漏，符合 subtree 语义）。

**范围/风险**：纯增量（~2 exported fn + 1 background tool 类型 + 测试），**不碰同步路径、不动 lyra/wire**，可独立 revert。风险在 goroutine 生命周期 vs parent turn 边界同步——先写 `TestSpawnChildAsync_CollectAfterParentContinues` 钉死时序。**不做**：`taskBudget`（API 侧 pacing，做不了）/ `forwardSubagentText`（子任务故意不透传）/ 进度 summary（依附 background，之后再说）。

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
- **Tier 2 跨重启自动恢复 —— 框架前置已补齐（`e79cd99`，agent/core），剩 FE 协议**。进度：
  - 框架**已就绪 + 已证明**：`ProcessStore.List` + `Platform.RestoreProcess` 在;`FileStore` 实现了 List。
  - **核心机制原本是坏的,已修**:审计 restore-Waiting→resume 时发现 `RestoreProcess` 调 `newAgentProcess` 后**没 wire determiner**(只有 `createProcess` 补了),restore 出来的进程一 re-tick 就 nil-panic。bug 一直潜伏因为既有 restore 测试只 load terminal snapshot 只读检查。已抽 `wireRuntimeDeps` 给两条构造路径共用,新增 `TestPlatform_RestoreWaitingProcess_ResumesToCompletion` 端到端证明:fresh platform 上 save Waiting → restore → continue(re-park) → resume(true) → continue → Completed。这是 lyra 作为最佳实践**又一次暴露并反哺框架缺口**的范例。
  - **resume 契约已文档化**:pending awaitable 不 round-trip(带 handler 闭包),所以恢复的 Waiting 进程**必须先 re-tick 一次**让 await action 重新 `AwaitInput`,再 resume。await action 要幂等(decision condition 未设→re-park,已设→proceed)。lyra 的 `planGate` 正好满足这个形状。4 步序列写在 `RestoreProcess` doc + `ProcessSnapshot` 第三条 limitation。
  - **真正价值仍 FE-gated**:有用的形态是"客户端重启后发现并重连在飞的 plan 审批",需要 protocol 加 "list 可恢复 turn" + 重连 —— 动 wire 先对前端(本节末尾"前端协调"同类)。
  - lyra-only 能单方面做的只是"开机把 Waiting snapshot restore 进 platform",但没 turnState 无人驱动、`ContinuePlan` 够不到 —— 半个功能,YAGNI。
  - **结论**:两个前置之一(agent/core 的 restore-Waiting 验证)已完成。剩 FE 的 list/reconnect 协议 —— 待前端定 spec 后,lyra 这侧(开机 restore Waiting snapshot → 重建 turnState → 通过 list 暴露 → 重连驱动)单独一轮做。

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
- ❌ **`SDKMessage` mega-union（第三轮已膨胀到 30+ 变体）**：这半年又长出 hook/task/plugin/auth/ratelimit/elicitation/promptSuggestion 各种 message —— 更坐实反面教材。我们 lyra chat `Event` 按 domain 分类型、agent `event` 是 `Multicast`+窄 `Listener`——加类型不撑大任何 interface，比 mega-union 更符合 ISP（"接口越大抽象越弱"——Rob Pike）。

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
- **其他**：`outputFormat: json_schema` / `thinking`(adaptive/extended/disabled) / `effort`(low/medium/high/xhigh/max) / `enableFileCheckpointing`+`rewindFiles` / `includePartialMessages` / `SDKCompactBoundaryMessage` / `SDKMemoryRecallMessage` / `onElicitation`。
- **2026 新增（第三轮拉取，裁决见 §A.C）**：`taskBudget`（API 侧 token pacing，alpha）/ `agentProgressSummaries`+`SDKTaskProgressMessage` / `AgentDefinition.background`+`stopTask`+`origin`+`forwardSubagentText` / `deferred_tool_use`+`permissionDecision:"defer"` / `AgentDefinition.skills` / `terminal_reason`（12 值枚举）/ `applyFlagSettings` / `fastModeState` / `promptSuggestions` / `resolveSettings`+`settingSources` 来源溯源 / `setMcpServers`+`reconnectMcpServer`+`toggleMcpServer`。`SDKMessage` 变体现 30+。
