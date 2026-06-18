# AGENT_LEVERAGE.md — lyra 对 agent 模块肌肉的利用率 + 自举(a+b=c)清单

> 定位:回答两个问题 ——
> **Q1** lyra 有没有充分用上 `agent` 模块提供的肌肉?
> **Q2** lyra 和 agent 之间有没有"a+b=c"式的自举能力(已有原语 + 已有接线 = 还没造的新特性)?
>
> 配套:能力面证据见 [`../../agent/docs/CONSUMER_FOOTPRINT.md`](../../agent/docs/CONSUMER_FOOTPRINT.md)(agent 侧视角)。
> 协议方法状态表见前端仓 `docs/PROTOCOL.md`;心智模型见 [`WORKSPACE_MODEL.md`](./WORKSPACE_MODEL.md)。项目级约定见 [`../../CLAUDE.md`](../../CLAUDE.md)。
> 结论基于 2026-06 的代码事实(下面每条都带 `file:line`)。

---

## 0. 一句话结论(TL;DR)

> **lyra 用 agent 的姿势是"对的",但只接了一半。**
> 它把 agent 当成"**单 agent / 单 goal / 两 action 的进程骨架**"用:跑通了 runtime 生命周期、HITL、事件、工具装饰/解析、sub-agent-as-tool,连 GOAP 调度都隐式在用 —— 对一个 chat 后端,这一层接得很到位。
> 真正**休眠的肌肉**(background 任务、active-run 列表、崩溃重连、多模型 consensus、skill=agent、直接 invoke 工具)里,**大部分不是要从零造,而是 agent 已经造好、lyra 只差把线接上** —— 而这些恰好一一对应 `PROTOCOL.md` 里那批 ◻(已定义未接)的方法。
> 所以"利用率不足"的正确读法不是"浪费",而是:**下一档协议特性,多数是现成肌肉等接线,不是新工程。**

---

## 1. lyra 当前的消费面(用到了什么)

lyra import 的 agent 子包(import path 前缀 `github.com/Tangerg/lynx/agent`):

| 子包 | lyra 用它做什么 | 关键落点 |
|---|---|---|
| `agent` (top) | `New` / `NewAction` / `NewPlatform` / `GoalProducing` 构造 chat agent | `engine/agent.go` / `engine/engine.go` |
| `agent/core` | `Action`/`TypedAction`、`Agent`/`Goal`、`Blackboard`(plan 审批条件)、`Process`/`ProcessContext`、`Condition`、`Extension`、`ToolDecorator`、`ToolGroupResolver`、`ActionQoS`、`ProcessOptions` | `engine/*.go` / `service/chat/turn.go` |
| `agent/runtime` | `Platform` + `StartAgent` / `ResumeProcess` / `ContinueProcessAsync` / `KillProcess`、**`AsChatToolFromAgent`**(子 agent 转工具) | `engine/engine.go:173-184,484-488`、`engine/engine.go:200-202` |
| `agent/event` | `NewNamedListener` + 终端事件(`ProcessCompleted/Failed/Killed/Stuck/Terminated`)映射 TurnEnd 原因 | `service/chat/lifecycle.go:25-39`、`turn.go:203-219` |
| `agent/hitl` | `NewConfirmation` —— **plan-mode 审批**(唯一的 HITL 落点) | `engine/agent.go:212-215` |

**心智图**:

```
  lyra chat turn  ─────►  runtime.Platform.StartAgent(e.agent, {it: ChatInput}, opts)
       │                        │
       │                        ├─ (隐式) GOAP planner 调度 e.agent 的 2 个 action:
       │                        │     "chat"(主循环) + "task"(子 agent 转的工具)
       │                        ├─ ToolGroupResolver  → coding / subtask 两个工具角色
       │                        ├─ ToolDecorator      → observer.go 拦截每次 tool call
       │                        ├─ hitl.AwaitInput    → plan-mode 暂停,等 ResumeProcess
       │                        └─ event.NamedListener→ 终端事件 → TurnEnd 原因
       ▼
  chat.Service.Events()(协议层事件流,和 agent event 垂直独立)
```

---

## 2. 利用率矩阵(Q1)

每行一个 agent 能力簇,判定为三档:**✅ 已用** / **🟡 休眠-latent(有现成肌肉,对得上未接的协议特性)** / **⬜ 休眠-YAGNI(当前 chat 后端不需要,别接)**。

| agent 能力 | 档 | 证据 / 理由 |
|---|---|---|
| `runtime.Platform` 生命周期(Start/Resume/Continue/Kill) | ✅ | `engine.go:484-488` 等,chat turn 就是真 AgentProcess |
| `core.Process` / `ProcessContext`(ChatWithActionTools / RecordLLMInvocation) | ✅ | action 在 `engine/agent.go` 内消费 |
| **GOAP planner(默认调度器)** | ✅(隐式) | 没显式选,但 `PlannerName=""→"goap"`(agent `platform_process.go:90`)调度 lyra 的 2 个 action。**注意**:这和 `engine/planner.go` 不是一回事 —— 见 §3 的"两个 planner 正交" |
| `core.ToolGroupResolver` / `ToolGroup` | ✅ | `engine.go:162` `NewStaticToolGroupResolver`,coding/subtask 两角色 |
| `core.ToolDecorator` | ✅ | `observer.go:73-93` 拦截 tool start/end → UI observer |
| `event` 终端事件 + `NamedListener` | ✅ | `lifecycle.go` / `turn.go:203-219` |
| `hitl`(plan 审批) | ✅(部分) | 只在 plan-mode 用 `NewConfirmation`;tool 审批走**另一条**路(`service/approval/*`)→ 见 E4 |
| `runtime.AsChatToolFromAgent`(子 agent 转工具,**阻塞版**) | ✅ | `engine.go:200-202` 的 task 工具 |
| `runtime.AsBackgroundChatTool`(spawn/collect,**非阻塞版**) | 🟡 | `subagent_background.go:34` 已造好,lyra 只接了阻塞版 → **E1** |
| `runtime.Platform.ActiveProcesses()` | 🟡 | `platform.go:154` 存在,lyra 从不调 → **E2** |
| `Platform.SaveProcess/RestoreProcess` + `AutoSnapshot` | 🟡 | `engine.go:182-183` **接线已就位但生产 `ProcessStore=nil`** → **E3** |
| `core.ProcessOptions.Budget`(subtree 聚合预算) | 🟡 | `engine.go:473` `opts := core.ProcessOptions{}` 未设;lyra 的 MaxBudget 走 chat 中间件层 → **E8** |
| `workflow.Consensus` / `Parallel`(多模型) | 🟡 | 现成 builder + lyra 多 provider → **E5**(非当前需求) |
| `core.Agent` 可部署性 + `Platform.Deploy/Agents` 当 "skill" | 🟡 | 对应协议 `workspace.skills` ◻ → **E6** |
| `toolpolicy.OnceOnly/Unlocked` | 🟡 | lyra 已实现 `ToolDecorator`,再叠一个 decorator 即可 → **E7'** |
| `workflow.Sequence/Loop/Loop/RepeatUntil/ScatterGather` | ⬜ | 多 agent 编排;chat 后端无此需求,**别接**(YAGNI) |
| `planning/planner/{htn,utility,hybrid}` | ⬜ | 备选规划算法;lyra 的 2-action 空间 GOAP 足矣,**别接** |
| `runtime/autonomy`(Ranker 选 goal) | ⬜ | 单 goal,无多目标竞争;**别接**(也见 agent 侧 footprint 的 YAGNI 复核) |
| `core` 扩展点 `ActionMiddleware` / `AgentValidator` / `GoalApprover` | ⬜ | 无 per-action 审计 / deploy 校验 / 多 goal 门禁需求,**别接** |

---

## 3. 没有"重复造轮子"(澄清两组看似撞车的能力)

审计里最容易误判成"lyra 重造了 agent 已有能力"的两处,实际都是**正交**,不是重复:

### 3.1 两个 "planner" 正交
- **agent 的 planner**(GOAP):`platform_process.go` —— *action 调度器*,回答"下一步执行哪个 action"。lyra 隐式在用。
- **lyra 的 `engine/planner.go`**:*内容生成器* —— 用 LLM 起草一段给人看的计划文本,喂给 plan-mode HITL 审批。回答"给用户看什么计划"。
- 二者维度不同,**不冲突、不该合并**。lyra 不需要 agent 的 GOAP 之外的 planner,也不该把 LLM 草稿塞进 agent 的 planner 接口。

### 3.2 两套 "事件" 正交
- **agent event**(`ProcessCompleted` 等):runtime 内部观测,in-memory,interface 字段。
- **lyra chat 事件**(`MessageDelta` / `TurnEnd`):协议层 wire 形态,JSON-RPC 序列化给前端。
- lyra 在 listener 里把前者**翻译**成后者(`turn.go:203-219`),是转接层,不是重造。

### 3.3 两套 "审批" 接近但当前分立
- **agent HITL**(`AwaitInput`/`ResumeProcess`):流程级暂停点,plan-mode 在用。
- **lyra approval service**(`service/approval/*`):工具级门禁,client-facing。
- 这是**唯一**语义接近、值得考虑统一的地方 → 见 **E4**(可统一,非必须)。

---

## 4. 自举清单:a+b=c(Q2)

格式:**C(还没造的特性)= A(agent 现成肌肉)+ B(lyra 现成接线)**。后面括号是协议方法状态(取自 `PROTOCOL.md`)和工作量直觉。

### E1. 后台长任务 = `AsBackgroundChatTool` + 把 subtask 工具换成 spawn/collect 对
- **A**:`runtime.AsBackgroundChatTool[In,Out]`(spawn/collect 双工具,`SpawnChildAsync` + `KillChildren`,join parent budget 子树)—— `subagent_background.go:34`,**已造好、有测试**。
- **B**:lyra 现在用阻塞版 `AsChatToolFromAgent` 注册 task 工具(`engine.go:200-202`);改/加注册一对 spawn/collect 即可。
- **C**:parent LLM 启动长任务后**不阻塞**,继续推理,稍后 collect —— 对应协议 `background.list/stop/subscribe` ◻。
- **工作量**:小。主要是注册 + 暴露 `background.*` 方法,核心逻辑零新写。

### E2. 活跃/等待 run 列表 = `Platform.ActiveProcesses()` + 已有 status 翻译
- **A**:`Platform.ActiveProcesses() []*AgentProcess` —— `platform.go:154`,**已存在,lyra 从不调**。
- **B**:lyra 已有 turn→run 的 status 映射(`turn.go` 的 terminalPlan 把 agent 状态翻成 TurnEnd)。
- **C**:`runs.list`(活跃/等待态 run,崩溃后枚举)◻ = 调一个已存在方法 + 复用已有 status 翻译。
- **工作量**:很小。

### E3. 崩溃重连认领 = `SaveProcess/RestoreProcess` + 一个持久 ProcessStore
- **A**:`Platform.SaveProcess/RestoreProcess` + `AutoSnapshot`;lyra 已把 `ProcessStore` 一路穿到 platform(`runtime.go:130` → `engine.go:182-183`,`AutoSnapshot: cfg.ProcessStore != nil`)。
- **B**:但**生产装配从不构造持久 ProcessStore**(`cmd/lyra/app.go` 没设,生产 `ProcessStore=nil` → AutoSnapshot 实际关闭)。lyra 已有 file / sqlite 两套 storage 模式可照搬实现一个 `core.ProcessStore`。
- **C**:`runs.subscribe`(重连认领在飞的 run)+ 崩溃恢复 ◻ = 接一个持久 ProcessStore + 重 attach event listener。
- **工作量**:中。要写一个持久 `ProcessStore` 实现(套 `internal/storage` 已有模式),再补 replay。
- **注意**:`ProcessStore` 是 `core` 的 exported 接口,实现放 lyra 侧不动 agent —— 无破坏性 API 改动。

### E4. tool 审批统一到 HITL = `hitl` 机制 + lyra 已实现的 `ToolDecorator`
- **A**:`hitl.NewConfirmation` + `core.AwaitInput` + `Platform.ResumeProcess` —— plan-mode 已经这么跑(`agent.go:212`)。
- **B**:lyra 已实现 `ToolDecorator`(`observer.go`);在 `tool.Call` 前插一个 `AwaitInput` 即可。
- **C**:tool 审批 = 复用**同一套** HITL,而不是 `service/approval/*` 那条独立路 —— 减一套机制。
- **工作量**:中。**这是"减重复"型自举,不是新特性**;且 approval service 是 client-facing 协议门禁、HITL 是流程暂停,语义接近但不全等。**属于破坏性取舍,需用户拍板**(见 §6)。

### E5. 多模型 consensus / 并行采样 = `workflow.Consensus`/`Parallel` + lyra 多 provider
- **A**:`workflow.Consensus[In,Element]` / `Parallel` —— 现成 builder,产出普通 GOAP agent。
- **B**:lyra 已有多 provider(`models/anthropic` + `models/openai`,按 `LYRA_PROVIDER`)。
- **C**:"问 N 个模型取多数" / "并行采样取最优" = voters 用同 prompt 发不同 provider。
- **工作量**:小(若有需求)。**但属 🟡 latent —— 当前无产品需求,先记账。**

### E6. skill = 可部署的 agent = `core.Agent` + `Platform.Deploy/Agents`
- **A**:`core.Agent` 是可部署 bundle;`Platform.Deploy/FindAgent/Agents()` 已存在。
- **B**:lyra 已有 agentdoc 级联发现(AGENTS.md)+ tool-group 装配。
- **C**:一个 "skill" = 一个可部署的 `core.Agent`;`workspace.skills` ◻ = `Platform.Agents()` 的派生视图。
- **工作量**:中(需定义 skill→agent 的映射约定)。🟡 latent。

### E7. 直接 invoke 工具 = `ToolGroupResolver.Resolve` + `tools.*` 方法
- **A**:`ToolGroupResolver.Resolve` → `ToolGroup.Tools` —— lyra 已用 `StaticToolGroupResolver`(`engine.go:162`)。
- **B**:lyra 补 `tools.list` / `tools.invoke` 协议方法。
- **C**:`tools.invoke`(不经 LLM 直接调工具)◻ = 解析 tool group 拿 tool 直接 `Call`。
- **工作量**:小。

### E7'. 幂等工具 = `toolpolicy.OnceOnly` + 已有 ToolDecorator 链
- **A**:`toolpolicy.OnceOnly` / `Unlocked`(现成 policy)。
- **B**:lyra 已有 `ToolDecorator` 链(observer)。
- **C**:某些工具"一轮只能调一次" / "满足条件才解锁" = 再叠一个 decorator。
- **工作量**:很小。🟡(按需)。

### E8. 预算并入子树 = `core.ProcessOptions.Budget` + lyra 的 MaxBudget
- **A**:`core.ProcessOptions.Budget` 是 **subtree 聚合**的(`Usage()` 把 children 用量并入)。
- **B**:lyra 的 `MaxBudget/MaxCostUSD` 现在走 **chat 中间件层**(`engine.go:473` 的 `opts.Budget` 没设),**只算主循环**。
- **C**:把 per-turn 预算喂进 `core.Budget` = background 子任务(E1)的用量自动并入预算树,堵住"子 agent 用量不计入预算"的潜在漏算。
- **工作量**:小。**E1 落地时顺带做,否则后台子任务会绕过预算。**

---

## 5. 自举依赖图(谁该先做)

```
  E3 持久 ProcessStore ──┐
                          ├──► E2 runs.list ──► E1 background 任务 ──► E8 预算并子树
  (写一个 core.ProcessStore 实现) │            (ActiveProcesses)   (AsBackgroundChatTool)  (必须配 E1)
                          └──► E3 runs.subscribe(重连)

  E7 tools.invoke / E6 skills / E5 consensus / E7' toolpolicy  —— 互相独立,按产品需求各自接
  E4 tool 审批统一 HITL —— 破坏性取舍,单独决策(见 §6)
```

最划算的起手:**E2 + E1 + E8**(都小、都靠现成肌肉、直接点亮一批 ◻ 协议方法);**E3** 紧随(给崩溃恢复打底)。

---

## 6. 哪些**不要**自举(YAGNI 边界)

按 [`../../CLAUDE.md`](../../CLAUDE.md) 的 YAGNI/OCP 取舍,以下明确**别接**(接了是债):

- ❌ `workflow.Sequence/Loop/RepeatUntil/ScatterGather` —— chat 后端不是多 agent 编排器,没有"固定流水线"需求。
- ❌ `planner/{htn,utility,hybrid}` —— lyra 的 action 空间只有 2 个,GOAP 足矣;换 planner 是纯增复杂度。
- ❌ `runtime/autonomy` Ranker —— 单 goal,无目标竞争。
- ❌ `ActionMiddleware` / `AgentValidator` / `GoalApprover` —— 无对应需求点。

**需用户拍板(破坏性)**:
- **E4(tool 审批统一 HITL)** —— 会动 `service/approval/*` 的现有形态,且 approval 是 client-facing 协议门禁;统一前要先确认协议侧语义不丢。按 CLAUDE.md「破坏性公开 API 改动必须先咨询用户」。
- **E3 的持久 `ProcessStore` 实现** —— 实现新增不破坏 API,但若要进协议(`runs.subscribe` wire 形态)需前后端对齐。

---

## 一句话收尾

**agent 给的肌肉没有过度抽象,是 lyra 这个消费者只接了"单 chat agent"那一档。** 下一档协议特性(background / runs.list / 重连 / skills / tools.invoke)里,多数是 **agent 已造好、lyra 差一根接线** —— 这正是把能力沉在共享 runtime、消费方按需点亮的回报。先做 §5 里"小且现成"的 E2/E1/E8,再补 E3。
