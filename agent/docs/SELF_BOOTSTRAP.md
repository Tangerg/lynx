# SELF_BOOTSTRAP.md — agent 模块内部的自举(a+b=c)

> 定位:纯看 **agent 模块内部** —— 哪些高阶能力是(或该是)由 agent 自己的低阶原语**组合**出来的,而不是各造一套机器?模板:`异步并发 = 同步 + go 原语`。
>
> 配套:消费者(lyra)行使了多少能力见 [`CONSUMER_FOOTPRINT.md`](./CONSUMER_FOOTPRINT.md);本模块约定见 [`../CLAUDE.md`](../CLAUDE.md)。
> 结论基于 2026-06 的代码事实(每条带 `file:line`)。

---

## 0. 一句话结论(TL;DR)

> **agent 模块本身就是"一切高阶能力都自举成一个普通 GOAP agent"的范例 —— 这正是它没有过度抽象债的原因。**
> 几乎每个 workflow builder / agent-tool 变体都踩在共享原语上(`core.Agent` + 子进程 `Spawn` + GOAP + `ResultOfType` + `processStarter` 策略)。Tier 0 列出这些**已经自举**的事实做校准。
> 唯一**遗漏**:`Parallel` 没踩到 `ScatterGather` 的 fan-out 内核,把 errgroup 机器抄了一遍 —— 已在本轮收口(Tier 1,**DONE**)。
> 其余是 YAGNI 闸后的**潜在 shim**(MapReduce / Race / Fallback,Tier 2),以及一组**看着像自举其实别合**的反例(Loop≠RepeatUntil、4 planner、workflow retry)。

---

## 1. Tier 0 —— 已经自举的(校准:什么叫干净的 a+b=c)

不是建议,是代码**已经**这么写的。列出来是为了说明"自举"在本模块是既定纪律:

| C(高阶能力) | = A + B | 证据 |
|---|---|---|
| `workflow.Consensus` | = `ScatterGather` + 多数票 Joiner | `consensus.go:62` 直接 `return ScatterGather(...)` |
| `workflow.RepeatUntilAcceptable` | = `RepeatUntil` + Feedback 形状的 Accept + best-of-N History | `repeat_until_acceptable.go`(doc 自述 "just RepeatUntil with a Feedback-shaped Accept") |
| `AsChatTool` / `AsChatToolFromAgent` / `AsMCPTool` | = `newTypedAgentTool` 内核 + 一个 `processStarter` 策略(`SpawnChild` vs `RunFresh`) | `subagent.go:139-187` —— 3 个公开工具 1 个内核 + 策略函数,零重复 |
| `AsBackgroundChatTool` | = `SpawnChildAsync` + collect(`ResultOfType` 轮询 child 状态) | `subagent_background.go:54,124` |
| `workflow.Supervisor` | = 单 action `core.Agent` + `SubagentTools` + chat tool loop | `supervisor.go:72-120`("perfectly ordinary single-action GOAP agent") |
| **所有 `workflow.*`** | = `core.Agent`(1~2 action)+ `SpawnChildFresh` + GOAP 调度 + `GoalProducing[Result]` | 每个 builder 末尾都 `return core.NewAgent(...)`,**零新 runtime 概念** |
| **`autonomy.Autonomy`** | = `Platform.Agents()` 枚举 + `Ranker` + `Platform.RunAgent` + per-process `GoalApprover` | `autonomy.go:169-202` —— `targetGoalApprover` 就是个 `GoalApprover` extension,锁定选中 goal |
| `autonomy.LLMPlanRanker` | = `chat.Client` + plan 摘要 prompt + 复用 `Plan.NetValue/Cost/Value` | `plan_ranker.go:57-105` —— 不重算价值,只让 LLM 在既有 cost/value 上重排 |

> **启示**:`autonomy` 全仓零非测试调用方(见 [`CONSUMER_FOOTPRINT.md`](./CONSUMER_FOOTPRINT.md) §2.3),但它是**纯组合**——没引入任何新机器,删了等于放弃"用现成 Ranker+RunAgent+GoalApprover 拼目标路由"这个能力点。它是 a+b=c 的产物,不是 stub 债。

---

## 2. Tier 1 —— 该自举但漏了的(真 DRY · 本轮已收口 ✅)

### `Parallel` = `ScatterGather` + (agent→generator 适配器) —— **DONE**

**问题(重构前)**:`parallel.go` 和 `scatter_gather.go` 的 fan-out 内核**逐行重复**:同样的 `make([]Element,len)` + `errgroup.WithContext` + `SetLimit(MaxConcurrency)` + `g.Go` 写 `results[i]` + `g.Wait` + 包 `ResultList`。唯一差异是 goroutine 体 —— Parallel 是 `SpawnChildFresh + ChildError + ResultOfType[Element]`,ScatterGather 是 `gen(gctx, pc, in)`。而 ScatterGather 的 `Generators []func(...)` 正是"把变化的那段抽成函数参数"。

**修法**:`Parallel` 改为构造一组 generator(每个 = 对一个 sub-agent 做 `SpawnChildFresh → ChildError → ResultOfType`,错误里带 agent 名做归因),然后 `return ScatterGather(ScatterGatherConfig{Generators: ..., Joiner: spec.Joiner, ...})`。

**收益 / 安全性**:
- fan-out 机器(errgroup / 并发上限 / 槽位顺序 / produces-Result goal)从此**只活在 ScatterGather 一处**;`Parallel` 只剩 ~12 行适配器,删掉 `errgroup` import。
- **耦合不增**:`ScatterGather` 仍只 import `core`;适配器 generator 住在 `parallel.go`(本就 import `runtime`)。
- **行为不变**:`MaxConcurrency`、顺序对齐、失败取消、`singleAttempt` QoS 全由 ScatterGather 保留;错误经 `"scatter generator N: agent %q: ..."` 仍含 agent 名;`Parallel` 的入参校验(nil platform / 空 Agents / nil Joiner)保留在委托前,错误前缀仍是 `workflow.Parallel:`。
- 5 个 `TestParallel_*` 全过(含 `-race`);全 agent 模块 `build / vet / test` 全绿。

> 这是教科书级 "fan-out 沉成原语,N 个变体踩上去":Consensus 早就踩了,现在 Parallel 也踩上了。`ScatterGather` 成为唯一的并行 fan-out 内核。

---

## 3. Tier 2 —— 能自举的潜在新特性(YAGNI 闸 · 有需求再做)

都是踩在 §2 的 fan-out 内核 / `Sequence` 链上的**薄 shim**,和 Consensus 同级。**当前无需求,先记账,别预造**:

| 潜在 C | = 现成 A + B | 形状 |
|---|---|---|
| `MapReduce[In,Item,Element,Result]` | = `ScatterGather`,Generators 由 `items []Item` + `mapFn` 派生 | 像 Consensus 一样的 shim,省手写一组 func |
| `Race` / first-wins | = 同一 fan-out 内核,但"首个成功即 cancel 其余"(errgroup + cancel-on-first) | Parallel 现在等全部;Race 等首个 |
| `Fallback` / try-chain | = `Sequence` 的兄弟:遍历 agents 返回首个 `Completed`,全失败才失败 | 现在 Sequence 首个 child 失败即整链失败 |
| `Timeout` / deadline | = **已可表达**:`ProcessOptions.Budget`(cost/token/action)+ `ctx` deadline(墙钟) | **无需新 builder**,只需文档化 |

---

## 4. 反向不变量 —— 看着像自举,其实**别合**(虚假 DRY / KISS 违反)

- ❌ **`Loop` 合并进 `RepeatUntil`**:`Loop` 是 *agent 级*(每轮 `SpawnChildFresh`,**clean blackboard**,body 看不到自己上轮输出,`loop.go`);`RepeatUntil` 是 *action 级*(inline 闭包重跑,history 累积在 blackboard,body 看得到上轮,`repeat_until.go`)。粒度 + 状态模型都不同,合了两边都坏。
- ❌ **4 个 planner(goap/htn/reactive/utility)抽公共 search 核**:不同状态空间的不同算法,不是彼此的组合。抽出来是虚假 DRY。
- ❌ **workflow 的 `singleAttempt` "统一"成 `ActionQoS` 重试**:`types.go:7-13` 明确:编排失败是确定性结果,重试只会把"保证失败"摊成几分钟;域内 action(用户的 generator/task)各自保留自己的 retry。这是有意的,不是漏抽象。
- ⚠️ **`PromptCondition`(LLM yes/no)/ `LLMPlanRanker`(plan 打分)/ autonomy `Ranker`(candidate 打分)抽公共 "LLM 决策器"**:三者都"问 LLM 做判断",但输出形状不同(三值 `Determination` / `[]*Plan` 重排 / `[]Choice` 打分)。强抽一层违反 KISS。**先观察,真正全等用法 < 3 处,别合。**

---

## 一句话收尾

**agent 侧"该自举的几乎都自举完了"——这就是它没有过度抽象债的原因。** 本轮补上的唯一遗漏是 `Parallel ← ScatterGather`;其余要么是 YAGNI 闸后的潜在 shim(Tier 2),要么是必须保持分立的真差异(§4)。下一个自举点的判据不变:**新能力能不能踩在现有原语上?能 = 写个 shim;不能但"看着像" = 大概率是 §4 的虚假 DRY。**
