# CONSUMER_FOOTPRINT.md — agent 能力被主消费者(lyra)行使的足迹

> 定位:从 **agent 模块自己**的角度看 —— 我导出的能力面,被唯一的生产消费者 `lyra` 行使了多少?哪些休眠?休眠的是"为未来留的扩展点(OCP)"还是"零调用方的债(YAGNI)"?
>
> 配套:消费方视角(lyra 怎么用、能自举什么)见 [`../../lyra/doc/AGENT_LEVERAGE.md`](../../lyra/doc/AGENT_LEVERAGE.md);模块**内部**自举(a+b=c)见 [`SELF_BOOTSTRAP.md`](./SELF_BOOTSTRAP.md)。本模块约定见 [`../CLAUDE.md`](../CLAUDE.md),项目级约定见 [`../../CLAUDE.md`](../../CLAUDE.md)。
> 结论基于 2026-06 全 monorepo grep(每条带 `file:line` 或调用方清单)。

---

## 0. 一句话结论(TL;DR)

> **agent 的核心脊梁被 lyra 充分行使;休眠面几乎全是合理的 OCP 扩展点,不是过度抽象。**
> lyra 行使的部分(Platform 生命周期 / Process / GOAP 默认调度 / HITL / event / ToolDecorator / ToolGroupResolver / `AsChatToolFromAgent`)恰好是 agent 的"承重墙" —— 这些 exported 签名**不能随便动**,改了直接砸 lyra。
> 休眠面里:`goap`/`reactive` 是**生产默认**(被 Platform 内部行使);`htn`/`utility`/`workflow`/`toolpolicy` 是**有测试、是 OCP 扩展点**(模块身份的一部分,留);**唯一真正值得 YAGNI 复核的是 `runtime/autonomy`** —— 全仓零非测试调用方。
> 反过来看一个反直觉的事实:agent 已经把 background / sub-agent / subtree-budget / snapshot 这些"高阶能力"造齐了,**是 lyra 只接了一半** —— 证明 agent 没有"造了没人用"的债,而是消费者还没接满。

---

## 1. 行使面 vs 休眠面(按子包)

判定:**✅ 被 lyra 行使** / **🔩 被 agent 内部行使(生产路径)** / **🧪 仅测试行使(OCP 扩展点,留)** / **⚠️ 零非测试调用方(YAGNI 复核)**。

| 子包 / 能力 | 档 | 证据 |
|---|---|---|
| `core` 原语(Action/Agent/Goal/Blackboard/Process/ProcessContext/Condition/Determination) | ✅ | lyra `engine/*` 全程消费 |
| `core.Extension`:`ToolDecorator` | ✅ | lyra `observer.go` |
| `core.Extension`:`ToolGroupResolver` + `StaticToolGroupResolver` | ✅ | lyra `engine.go:162` |
| `core.ActionQoS` / `ProcessOptions` / `ProcessStore`(接口) | ✅ | lyra `engine.go`(QoS 用,ProcessStore 接线已穿透) |
| `core.Extension`:`ActionMiddleware` / `AgentValidator` / `GoalApprover` / `IDGenerator` | 🧪 | 仅 agent 内部 + 测试;lyra 未用 |
| `runtime.Platform` 生命周期(Start/Resume/Continue/Kill/Deploy) | ✅ | lyra `engine.go:484-488` 等 |
| `runtime.AsChatToolFromAgent`(阻塞 sub-agent 转工具) | ✅ | lyra `engine.go:200-202` |
| `runtime.AsBackgroundChatTool`(spawn/collect 非阻塞) | ⚠️→latent | `subagent_background.go:34`,**有测试**,但无生产消费者;lyra 应接(见 lyra E1) |
| `runtime.Platform.ActiveProcesses()` | ⚠️→latent | `platform.go:154`,无调用方;lyra 应接(lyra E2) |
| `runtime.SaveProcess/RestoreProcess` + `AutoSnapshot` | 🔩/latent | 机制就位;lyra 接线穿透但生产 store=nil(lyra E3) |
| `event`(类型 + `NamedListener` + `Multicast`) | ✅(部分) | lyra 只用终端事件 + `NamedListener`;`ActionExecutionStart/Result`、`PlanFormulated`、`GoalAchieved` 等休眠 |
| `hitl`(`TypedRequest`/`NewConfirmation`/`ResponseImpact`) | ✅(部分) | lyra 仅 plan 审批用 `NewConfirmation` |
| `planning/planner/goap` | 🔩 | **生产默认**:`platform_process.go:90` `PlannerName=""→"goap"`,行使于每个 process(含 lyra) |
| `planning/planner/reactive` | 🔩 | `platform_process.go:11` import,作为 resolvable planner |
| `planning/planner/htn` | 🧪 | 仅 `htn_test.go`;OCP 备选算法,留 |
| `planning/planner/utility` + `hybrid` | 🧪 | 仅 `utility_test.go`;OCP 备选算法,留 |
| `workflow.{Sequence,Loop,RepeatUntil,Parallel,ScatterGather,Consensus,Supervisor}` | 🧪 | 仅各自 `*_test.go`;无 example、无生产、无 lyra |
| `toolpolicy.{OnceOnly,Unlocked}` | 🧪 | 仅 `toolpolicy_test.go`;`core/model/chat/tool.go:51` 只在**注释**里引用(非 import) |
| `runtime/autonomy`(`Ranker`/`Autonomy`/`Choice`) | ⚠️ | **全仓零非测试调用方**;无 example、无生产 → §3 复核 |

> 补充事实:`agent/examples/{blog,blogllm,hello,mcpagent,mcpbridge,supervisor}` **全部只 import `agent` / `core` / `runtime` 顶层**,未演示 `workflow`/`planner 子包`/`autonomy`/`toolpolicy`/`hitl`。即这些子包在全 monorepo 内**连 example 都没行使**,只有各自的单元测试。

---

## 2. 休眠面的三种性质(别一刀切删)

agent 自己的 CLAUDE.md 有强 YAGNI 不变量(「零调用方 = 删」「stub placeholder 删」)。但**这条只杀"推测性占位",不杀"已成型的 OCP 扩展点"**。逐类判:

### 2.1 🔩 生产默认 —— 不是休眠,是承重
`goap` / `reactive` 经 `platform_process.go` 的 planner 解析进入**每一个** AgentProcess。lyra 没显式选 planner,但它的 chat agent 就是被 GOAP 调度的。**动 `Planner` 接口签名 = 砸生产路径。**

### 2.2 🧪 OCP 扩展点 —— 留(YAGNI vs OCP 的 OCP 侧)
`htn` / `utility` / `hybrid` / `workflow.*` / `toolpolicy.*` / `ActionMiddleware` / `AgentValidator` / `GoalApprover`:
- 全部**有完整实现 + 单元测试**,不是 `// TODO: M5 wires this` 那种 stub。
- 它们是 agent **模块身份**的一部分 —— agent 的一句话定位就是「**多种 planning 算法** + workflow builder」。"能换 planner / 能组合 workflow"本身就是卖点,**这是"扩展已经被设计进去",不是"猜未来"**。
- 按 CLAUDE.md「YAGNI vs OCP:扩展是不是已经发生过?发生过=OCP(保留)」—— 多算法/多 builder 是模块的既定能力面,**保留**。
- ✅ 结论:**不删**。它们休眠是因为 lyra 这一个消费者只需要单 chat agent;换一个消费者(如多 agent 编排后端)就会点亮。

### 2.3 ⚠️ 真正值得复核 —— `runtime/autonomy`
- **全仓零非测试调用方**:无 example、无生产、无 lyra,只有 `autonomy/*_test.go`。
- 它和 §2.2 的区别:planner/workflow 至少是"模块定位明写的能力";autonomy(LLM/plan Ranker 选 goal)是**更上层的可选路由**,既没进任何 example,也没进定位描述的核心。
- 但它**也不是 stub** —— 有 `plan_ranker` / `llm_ranker` 两套实现 + 测试,语义自洽。
- ✅ 建议:**不自动删,标记为 YAGNI 复核项交用户判**。判断点 = "多目标自主选择"这个扩展,是"已发生过的需求"还是"纯推测"?若纯推测,按 CLAUDE.md 可考虑下沉/移除;若是预见(agent 想做 autonomous 多目标 runtime),则保留并补一个 example 把它从 ⚠️ 提到 🧪。

---

## 3. lyra footprint 对 agent API 的两条启示

### 3.1 承重墙清单(改签名前必看)
被 lyra 行使的这些 exported 符号,任何签名改动都会砸 lyra,属 CLAUDE.md「破坏性公开 API 改动必须先咨询用户」:
- `runtime.Platform`:`StartAgent` / `ResumeProcess` / `ContinueProcessAsync` / `KillProcess`
- `runtime.AsChatToolFromAgent`
- `core`:`Action`/`NewAction`、`Agent`/`NewAgent`/`GoalProducing`、`ProcessContext`(`ChatWithActionTools`/`AwaitInput`/`RecordLLMInvocation`)、`ToolDecorator`、`ToolGroupResolver`/`StaticToolGroupResolver`、`ProcessOptions`、`ProcessStore`、`Condition`/`Blackboard.Condition`/`SetCondition`
- `event`:终端事件类型(`ProcessCompleted/Failed/Killed/Stuck/Terminated`)、`NamedListener`/`NewNamedListener`
- `hitl`:`NewConfirmation`/`ResponseImpact`

### 3.2 agent **没有**过度抽象的债 —— 反而是消费者没接满
一个反直觉但重要的判断:按 agent 自己的 YAGNI 尺子,"造了没人用"是债。但本次 footprint 显示,休眠的高阶能力(`AsBackgroundChatTool` / `ActiveProcesses` / snapshot / subtree-budget)**不是无主孤儿,而是恰好对上 lyra 协议里那批 ◻ 未接的方法**(background / runs.list / runs.subscribe)。也就是说:
- 这些不是"agent 拍脑袋造的",是"runtime 该有、消费者迟早要"的能力;
- **债不在 agent 侧(造了没人用),在 lyra 侧(该接没接)** —— 修复方向是 lyra 接线,不是 agent 删能力。
- 详见 [`../../lyra/doc/AGENT_LEVERAGE.md`](../../lyra/doc/AGENT_LEVERAGE.md) §4 的 E1/E2/E3/E8。

---

## 4. 行动建议(agent 侧)

| 项 | 建议 | 依据 |
|---|---|---|
| `goap`/`reactive`/`htn`/`utility`/`workflow.*`/`toolpolicy.*` | **保留**,不动签名 | OCP 扩展点 + 模块身份(§2.1/§2.2) |
| `AsBackgroundChatTool` / `ActiveProcesses` / snapshot | **保留**,等 lyra 接线点亮 | 对应协议 ◻,非孤儿(§3.2) |
| `runtime/autonomy` | **不自动删**,提交用户做 YAGNI 复核;若保留建议补一个 example | 全仓零非测试调用方(§2.3) |
| 承重墙符号(§3.1) | 改签名前先咨询用户 + 对前端协议 | CLAUDE.md 破坏性 API 规则 |

---

## 一句话收尾

**agent 行使面=承重墙、休眠面=合理 OCP 扩展点,几乎没有"造了没人用"的债。** 唯一例外 `runtime/autonomy` 值得一次 YAGNI 复核。更准确的整体判断是:不是 agent 抽象过度,而是 lyra 作为单 chat 后端只用到了 runtime 的一档 —— 高阶肌肉已就位,等消费者接线(见 [`../../lyra/doc/AGENT_LEVERAGE.md`](../../lyra/doc/AGENT_LEVERAGE.md))。
