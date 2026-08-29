# 总对比：七个参照项目与 scope

> 日期：2026-08-29。基于逐仓库读源码，非宣传材料。
> 单项分析见 [`README.md`](README.md) 的索引。

---

## 1. 规模坐标

| 项目 | 语言 | 生产代码 | 生产文件 | module 数 | 语言版本 | 内含成品应用 |
|---|---|---:|---:|---:|---|---|
| **scope** | Go | **97,808** | 756 | **85** | **1.27** | ❌（Flame 独立仓） |
| trpc-agent-go | Go | 492,436 | 1,765 | 30+ | 1.21 | ✅ openclaw |
| GitNexus | TypeScript | 301,972 | 943 | 3 pkg | — | ✅ CLI + Web |
| spring-ai | Java | 173,530 | 1,315 | ~60 | 17+ | ❌ |
| embabel-agent | Kotlin | 101,701 | 796 | 20 | 21+ | ✅ shell |
| agent-framework-go | Go | 73,995 | 325 | **1** | 1.26 | ❌ |
| adk-go | Go | 61,795 | 343 | 2 | 1.26.6 | ✅ CLI + web 容器 |
| eino | Go | 59,032 | 202 | 1 | **1.18** | ❌（ext 独立仓） |

scope 的 85 个 module / 97.8K 行分布：`core` 12.6K、`agent` 22.7K（+17K 测试）、`models/*` 22.8K（30 个叶子）、`vectorstores/*` 20.8K（23 个叶子）、`tools` 5.5K、`etl` 2.6K、`rag` 2.5K、`otel` 2.2K、`evaluation` 1.3K、`mcp` 0.9K、`a2a` 0.9K、`skills` 0.9K。

**读法**：scope 的核心（core + agent = 35.4K 行）比任何一家的核心都小，而外围（provider 叶子 = 43.6K 行）不进任何人的依赖树。trpc 的 492K 行里，用户装一个包就全背上。

---

## 2. 八维矩阵

图例：**✅** 与 scope 同构 · **⚠️** 部分同构/有条件 · **❌** 与 scope 分歧 · **⭐** 领先 scope

### D1 协议层

| | 消息模型 | 中立性 | Reasoning | 流式原语 |
|---|---|---|---|---|
| **scope** | `Message` + `[]Part` tagged value | 中立，wire 分层到 `models/protocol/*` | first-class | `iter.Seq2` |
| eino | ❌ **两套并存**（`Message` + `AgenticMessage`）+ 泛型 union | 中立 | 有 | `StreamReader[T]`（channel，手动 Close） |
| trpc | OpenAI 形状中立结构 | 中立 | 有 | `<-chan *Event` |
| adk-go | ❌ `genai.Content`（Gemini wire 即领域模型） | 绑定 Gemini | 依赖 genai | ✅ `iter.Seq2` |
| MAF | ✅ `Content` 接口 + 20+ 具体类型 | 中立 | ✅ `TextReasoningContent` | ✅ `iter.Seq2` |
| spring-ai | `Message` 层级 | 中立 | 有 | `Flux`（Reactor） |
| embabel | 委托 Spring AI | 中立 | 委托 | `Flux` |
| GitNexus | — | — | — | — |

**关键**：`iter.Seq2` 只有 adk-go 和 MAF 选中 —— 两个都是 Go 1.26 起步的项目。eino（1.18）和 trpc（1.21）锁死语言版本用不上。这是**语言版本选择直接决定 API 形状**的实例。

### D2 执行内核

| | 执行契约 | 上下文传递 | Step 纯度 | 状态所有权 |
|---|---|---|---|---|
| **scope** | `Definition{Descriptor,Start,Restore}` + `Execution{Step,Snapshot}`（**4 方法**） | `Signal` 序列 | **纯归约，禁读 clock/random/global** | 归 owner，`ExecutionState` 信封 |
| eino | `Runnable[I,O]`（4 方法） + `TypedAgent[M]`（3 方法） | ❌ state 藏 `context` | 无约束 | ❌ `State any` + gob 全局注册表 |
| trpc | ❌ `Agent`（5 方法）+ **`*Invocation` god struct（2,428 行）** | ❌ god struct | 无约束 | ❌ `State map[string]any` + 字符串 key |
| adk-go | ❌ sealed `Agent`（`internal()`，6 方法） | ❌ `InvocationContext` **嵌** `context.Context` | 无约束 | Session 即状态 |
| MAF | ✅ `RunFunc`（一个函数类型） | 参数 | 无约束 | Executor 有状态 → 需运行期所有权令牌 |
| spring-ai | `Advisor` 链 | `ChatClientRequest` | 无约束 | 无（同步栈） |
| embabel | `Action`（❌ 五路接口继承） | ❌ **Blackboard**（`String→Any` 共享可变） | 无约束 | ❌ Blackboard |

**这一列是 scope 与全场差距最大的地方。** 没有第二家做到「Step 是纯归约、副作用只能以 Effect 意图声明、由 Engine 分配稳定身份后 dispatch、再原子 finalize」。

### D3 编排

| | 拓扑词汇 | 确定时机 | 同进程 vs managed 分界 |
|---|---|---|---|
| **scope** | **封闭 6 个**（Transform/Call/Switch/Fork/Map/Loop） | 构造期冻结 | ✅ `flow`（普通）/ `workflow`（真实 child Process） |
| eino | 开放 DAG + 按组件类型开 `AddXxxNode` | Compile 期反射校验 | ❌ 只有 Graph |
| trpc | 开放 DAG（LangGraph 移植） | 运行期 | ❌ |
| adk-go | ❌ **两套并存**（workflowagents + workflow） | 构造期 | ❌ |
| MAF | 开放 Executor 图 + 类型路由 | Build 期反射校验 | ⚠️ subworkflow 有，但同一 runtime |
| spring-ai | Advisor 有序链 | ❌ `Ordered` 魔法整数 | ❌ |
| embabel | GOAP 搜索（**拓扑由系统生成**） | 运行期重规划 | ❌ |
| GitNexus | ✅ 静态 19-phase DAG，**无插件** | 构造期 Kahn 校验 | — |

### D4 持久化与恢复

| | 快照单位 | 序列化 | 副作用一致性 | 谁持久化 | 分支 |
|---|---|---|---|---|---|
| **scope** | **Process / TreeSnapshot** | 严格 JSON + owner schema | ✅ prepare/finalize + EffectID replay contract | Host | ❌ |
| eino | 图执行位置 | ❌ gob + 全局注册表 | ❌ | Store 接口 | ❌ |
| trpc | 图 checkpoint | JSON + `map[string]any` | ❌ | Store | ✅ `ParentCheckpointID` ⭐ |
| adk-go | Session（重放 events） | JSON | ❌ | SessionService | ❌ |
| MAF | workflow checkpoint | ✅ 框架序列化，Store 只存字节 | ❌ | Store | ✅ `parent *CheckpointInfo` ⭐ |
| spring-ai | 无 | — | — | — | — |
| embabel | `AgentProcessRepository` | Jackson | ❌ | Repository | ❌ |

**scope 独有**：`ARCHITECTURE.md §6.2` 的三阶段提交 + §12 的 TreeSnapshot quiescent cut + 预算总和/能力衰减/父子关系的恢复校验。
**scope 落后**：checkpoint 分支（两个独立项目都有）。

### D5 扩展机制

| | 机制数 | 观察与控制是否分离 | 全局注册表 |
|---|---|---|---|
| **scope** | **1**（`func(next Model) Model`）+ 观察型 listener（**无 error 返回**） | ✅ 彻底分离 | ❌ 无 |
| eino | 1（callbacks）+ 全局 | ❌ 混（OnStart 返回 ctx 可带状态） | ❌ `AppendGlobalHandlers` |
| trpc | ❌ **4**（callbacks×4 包 / plugin / hook / middleware） | ❌ 混 | 有 |
| adk-go | ❌ 2（callback + plugin） | ❌ 混（Before 返回非 nil 短路） | plugin manager |
| MAF | ✅ 2（Middleware 控制 / ContextProvider 碰消息，**边界写在文档里**） | ⚠️ ContextProvider 仍能改消息 | ❌ 无 |
| spring-ai | ❌ 11 个 Advisor 相关接口 + `Ordered` | ❌ 混 | Spring 容器 |
| embabel | 注解 + Spring AOP | ❌ 混 | ❌ classpath 扫描 |

**MAF 的 `compileRunChain` 与 scope 的 `compose` 逐行同构**（`slices.Backward` + nil 跳过 + outermost-first）—— 本次对比中最直接的独立收敛证据。

### D6 模块边界

| | module = 依赖岛 | provider 隔离 | 机器验证依赖方向 |
|---|---|---|---|
| **scope** | ✅ 85 个 | ✅ 每 provider 一个叶子 module | ✅ `dev/repoarch` + `core/internal/arch` + 各 module `architecture_test.go` |
| eino | ❌ 单 module | ⚠️ provider 在另一个**仓库** | ❌ |
| trpc | ⚠️ 可选后端拆了，根 module 极重（cgo sqlite3 + 中文分词） | ⚠️ 部分 | ❌ |
| adk-go | ✅ 2 个，CI guardrail 禁根依赖子 module | ❌ 全在根 | ✅ guardrail job |
| MAF | ❌ **单 module 捆 5 家 SDK + gRPC** | ❌ | ❌ |
| spring-ai | ✅ ~60 个 | ⚠️ 每 provider **3 个** module（model+autoconfig+starter） | ❌ |
| embabel | ✅ 20 个 | ✅ | ❌ |
| GitNexus | ⚠️ 3 个包 | — | ✅ runner 过滤 deps map |

### D7 可观测性

| | 是否自造抽象 | 领域代码是否 import 遥测 | 有无 gate |
|---|---|---|---|
| **scope** | ❌ 直接用官方 OTel | ❌ 由 `otel/*` decorator 反向装饰 | ✅ Kernel gate 禁 OTel import |
| eino | ⚠️ 自造 callback 协议 | ❌ | ❌ |
| trpc | ❌ 官方 OTel + semconv + langfuse | ✅ 直接 import | ❌ |
| adk-go | ❌ 官方 OTel | ✅ 直接 import | ❌ |
| MAF | ❌ 官方 OTel | ✅ workflow 内核内嵌 telemetry | ❌ |
| spring-ai | ✅ Micrometer Observation 抽象层 | ✅ 各领域包内 | ❌ |
| embabel | 独立 `-observability` module | 部分 | ❌ |

### D8 工程治理

| | 破坏性变更政策 | 债务处理 | 文档所有权 |
|---|---|---|---|
| **scope** | pre-1.0，**咨询后直接改对** | **绝不留**；旧框架整体删除，只留 git 历史 | ✅ 四层 + ADR 追加不改写 + API baseline 机器守卫 |
| eino | 兼容优先 | ❌ 44 处 Deprecated 不删 + **"NOT RECOMMENDED" 常驻** | 部分 |
| trpc | ❌ 「保持兼容除非明确要求变更」 | ❌ 30+ `Disable*/Enable*` 开关化石 | AGENTS.md |
| adk-go | ❌ **"Never break the public API"** | sealed interface 封口、两套编排并存 | ✅ AGENTS.md + DoD 6 条可执行清单 ⭐ |
| MAF | preview，**显式功能差距表** ⭐ | .NET 语义疤痕（三态 bool、7 个 history 字段） | ✅ 功能对比表 |
| spring-ai | 语义化版本 | `spring-ai-retry`（时代产物） | design/*.adoc |
| embabel | 兼容 | `bindProtected` 之类补丁 | CLAUDE.md + .embabel/coding-style.md |
| GitNexus | — | — | ✅ **GUARDRAILS "Signs"（Trigger→Do→Why）** ⭐ |

---

## 3. 独立收敛证据（最有说服力的部分）

以下取舍在**互不参考**的项目中被独立选中，说明它们是被问题域逼出来的，而不是 scope 的个人趣味：

| 取舍 | 独立选中者 | 强度 |
|---|---|---|
| **middleware 用反向组合、nil 跳过、outermost-first** | MAF（`slices.Backward`，与 scope 逐行相同） | ⭐⭐⭐ |
| **`iter.Seq2` 做流式** | adk-go、MAF（两个 Go 1.26 起步的项目） | ⭐⭐⭐ |
| **多态 Content 建模 + Reasoning 一等** | MAF（20+ Content 类型）、eino（正在迁往此形态） | ⭐⭐⭐ |
| **框架序列化，Store 只存字节** | MAF（注释逐字相同的分层） | ⭐⭐ |
| **类型化 HITL 契约（port/schema + 稳定 ID）** | MAF `RequestPort`、embabel `ux/form` | ⭐⭐ |
| **accept interfaces, return structs（provider 只给一个函数）** | MAF `RunFunc` | ⭐⭐ |
| **静态拓扑 + 构造期校验 + 无插件** | GitNexus（19-phase DAG + Kahn） | ⭐⭐ |
| **依赖显式声明由机制强制** | GitNexus（runner 过滤 deps map）、adk-go（guardrail CI） | ⭐⭐ |
| **部分失败不伪装成完整结果** | GitNexus（零行或全集）、eino（stream 首错终止） | ⭐⭐ |
| **能力衰减 / 最小权限** | GitNexus（只读 MCP 模式） | ⭐ |
| **module = 依赖岛** | spring-ai、embabel、adk-go（guardrail） | ⭐ |
| **领域语言 Goal/Action/Condition/Process/Planner/Platform** | embabel（scope 的直系来源） | ⭐（继承非收敛） |
| **不为 provider 加 OAuth / token refresh** | 全部七家 | ⭐ |
| **不抽 provider 公共基类（shape 差异 > 相似度）** | eino、trpc、spring-ai | ⭐⭐ |

---

## 4. scope 独有的东西（无人达到）

按重要性排序：

### 4.1 执行窄腰只有 4 个方法

```go
type Definition interface { Descriptor() Descriptor; Start(Input) (Execution, error); Restore(ExecutionState) (Execution, error) }
type Execution  interface { Step(context.Context, []Signal) (Transition, error); Snapshot() (ExecutionState, error) }
```

对照：trpc 的 `Agent`(5) + `*Invocation`(40+ 字段)、adk-go 的 sealed `Agent`(6) + god `InvocationContext`、embabel 的 `Action`(五路继承) + `AgentProcess`(六路继承)。

**没有第二家把 Agent 的执行契约压到这么小**，且这个小是有代价的 —— Session/Model/Memory/Artifact 全部被推给 Host。这个代价 scope 明确付了（`ARCHITECTURE.md §5.3`）。

### 4.2 Step 是纯归约

> Step 必须是对相同 ExecutionState 与 Signal 序列产生相同候选语义的纯归约；**不得读取 clock/random/global state**，所需变化必须先成为 Signal。
> 任何 Strategy 的 Step 都不能执行模型、Tool、Action 或其他外部 I/O。

**独一份。** 其余六家的"一步"都可以直接打网络、读时钟、改共享状态。这条约束是快照可重放、Step 可测、恢复可信的全部基础。

adk-go 用 `platform/` 包做 time/uuid 的可替换 seam 来逼近同一目标 —— scope 的做法（禁止读，变化必须先成为 Signal）是更强的形态。

### 4.3 Effect + prepare/finalize + replay contract

```
1. 验证 Transition，捕获候选 ExecutionState，为 Effect 分配稳定 EffectID，记录只读 prepared step
2. 按 prepared EffectID 调度并取得 settlement
3. 原子 finalize 候选状态、Signal 游标、settlement、结果入队、Process transition
```

外加一条极其诚实的条款：

> 只有已证明同一 EffectID 重放仍是同一逻辑操作时才允许自动重投。**无法证明时，未知结算必须停留为可观察、待显式裁决的状态，不得静默重放或假装成功。**

**独一份。** 其余六家在"工具调用发出去了但结果没回来"这个场景下，要么静默重试（可能重复扣款），要么当作失败（可能丢结果）。

### 4.4 表驱动终态矩阵 + first-terminal-wins

六行矩阵 + 「终态由已记录的控制意图决定，不能只按 error 文本或 `context.Canceled` 推断」+ 「已提交终态 first-terminal-wins」。

对照 embabel 的 `TERMINATED` / `KILLED` 二值、trpc 的 `StopError`、eino 的 `AgentAction{Exit, Interrupted, BreakLoop}`。

### 4.5 预算划拨 + 能力衰减 + 树恢复校验

> 子预算从父剩余预算中划拨，不能复制完整预算；子能力只能是父能力的子集，不能递归提权。
> tree restore 先校验 root/parent/depth/ChildKey、**预算总和、能力衰减**、tree limits、活动 child wait 和每个精确 DeploymentRef，再原子注册完整树。

**独一份。** 递归委派的资源与权限安全，其余六家都没有。

### 4.6 Platform 不拥有生命周期

> Platform 不包装、不创建也不代理 Engine。……因此 Platform 没有第二套 Start/Run、Process handle、scheduler 或 observation bus。
> `DeploymentCandidate` 没有 Dispatcher、Engine 或 Process capability。

对照 embabel 的 `AgentPlatform`（目录 + 注册表 + 创建者 + 杀手 + 服务定位器）。

### 4.7 85 个 module 的依赖岛 + 机器验证

`core/internal/arch` 有 `api_test.go`（exported API baseline）、`wire_contracts_test.go` + `wire_fixture_test.go`（JSON wire baseline，新 struct 未登记 fixture 直接失败）、`packages_test.go`、`documentation_test.go`；`dev/repoarch` 有跨 module 的 `models_provider_dependencies_test.go`、`provider_surfaces_test.go`。

对照：只有 adk-go 有一条 CI guardrail（根不依赖子 module）。

### 4.8 治理文档的所有权分离

四层（红线 / 哲学 / 重构标尺 / ADR）+ 明确的所有权规则：

> Do not copy progress into architecture, copy architecture into the execution log, ... or rewrite accepted decisions without a superseding ADR.

对照：其余六家的文档基本是"一份 AGENTS.md 什么都写"。

---

## 5. scope 的真实缺口（汇总去重）

按「是否需要新造机器」和「有几方独立证明」排序。

### 🔴 高优先级 —— 有多方独立证明，且能在现有边界内以组合形态落地

| # | 缺口 | 证据方 | 落点 | 是否需要新机器 |
|---|---|---|---|---|
| **G1** | **`chatclient` 层开箱即用的 tool loop** | spring-ai `ToolCallingAdvisor`、MAF 默认 func-auto-call middleware、eino `ChatModelAgent` | `chatclient` 的一个 `CallMiddleware` | ❌ 纯组合（形态②） |
| **G2** | **Model failover / hedge** | trpc `model/failover` + `model/hedge` | `chatclient` 的 `CallMiddleware`，组合多个 `chat.Model` | ❌ 纯组合 |
| **G3** | **HTTP cassette 回归测试** | adk-go `internal/httprr` + `//go:generate` 分区 | `core/modeltest` 增加 cassette 支持，或 `dev/` 工具 | ❌ 纯测试基础设施 |
| **G4** | **trace context 跨 restore 延续** | MAF `traceContextStrings` | `otel/agent` adapter 消费 restore 事件时建 span link | ❌ 在 adapter 层 |

**G1 特别说明**：`ARCHITECTURE.md §7.1` 把这件事悬置，理由是「是否暴露更小的直接 Runner，必须由**独立消费者**证明，不能仅为了迁移旧代码保留」。现在有三个独立项目提供了它 —— **悬置条件已满足**。落地时必须明确：这个 middleware **没有** snapshot / steer / budget / child Process 语义，需要这些就升级到 `agent/interaction`。这正是 §2.1 复杂度阶梯的第二级。

**G2 特别说明**：这不违反「❌ 加 retry layer」。failover 是换 provider，hedge 是并发对冲降尾延迟，SDK 内部的重试覆盖不到。但应在根 `CLAUDE.md` 补一句区分，防止未来被误读成放开 retry。

### 🟡 中优先级 —— 有证据，但需先确认能否由 Host 自足

| # | 缺口 | 证据方 | 判断 |
|---|---|---|---|
| **G5** | **checkpoint 分支树** | trpc `ParentCheckpointID`、MAF `parent *CheckpointInfo` | 两方独立证明。但 scope 的 `TreeSnapshot` 是交给 Host 的值 —— **Host 存多份 snapshot 就等于分支**。先确认 Host 侧能否自足；不能则只在 capture 边界暴露 parent 身份，**不引入 Framework Store** |
| **G6** | **用户模拟评测（usersimulation）** | trpc `evaluation/usersimulation` | `evaluation` module 的自然扩展，与 judge/metric 同层 |
| **G7** | **代码执行沙箱** | trpc `codeexecutor`（local/container/e2b/jupyter） | `tools` 的两层 SPI 天然容得下：Tool 层 + Backend Port。scope 目前只有 `tools/shell` |
| **G8** | **`SIGNS.md`：运行期失败模式索引** | GitNexus GUARDRAILS（Trigger→Do→Why） | scope 有"别这么设计"和"曾发现什么"，缺"看到 X 现象根因通常是 Y"。**只在同一错误重复出现时追加，不预写** |
| **G9** | **"我不知道" ≠ "没有" 的统一原则** | GitNexus（"an invented one is a lie"） | scope 已有四处分身（rag/core/tools/etl），缺统一判词。**建议收进 `DESIGN_PHILOSOPHY.md`**，成本为零 |

### 🟢 低优先级 / 记账

| # | 缺口 | 判断 |
|---|---|---|
| G10 | `SafeGuardAdvisor` 式内容安全前置 | scope 有 `core/moderation`，缺 chatclient 层 guard middleware。低成本纯组合 |
| G11 | `UsageAccumulator`（跨多轮累计 usage） | 与 G1 绑定 |
| G12 | `StructuredOutputValidationAdvisor` | ⚠️ 注意不要成为 `interaction` completion validator 的第二套 |
| G13 | `Message.Source`（消息知道是谁注入的） | MAF 的小巧思，可观测性改善 |
| G14 | Action 级 retry/QoS 配置落点 | 归 `ActionBinding`/Dispatcher，等真实消费者 |
| G15 | 索引陈旧性检测 | **不下沉 `core/vectorstore`**（违反 §2.5）。落点是 `etl` reader 在 metadata 记源版本 |
| G16 | `document-readers` 格式覆盖面 | 非结构性，按需增补 |
| G17 | 交互式开发 shell | Host/工具层，非框架缺口 |

### ❌ 明确不吸收（附理由）

| 能力 | 来源 | 不吸收理由 |
|---|---|---|
| Blackboard / 共享可变状态袋 | embabel、trpc `State map[string]any` | 快照/投影/版本/校验四条不变量都不成立 |
| `Advisor extends Ordered`（魔法整数排序） | spring-ai | 顺序归装配处，不归全局常量 |
| auto-configuration / 注解扫描 / DI 容器 | spring-ai、embabel | Go 的组合根就是 `main()`，「透明胜过魔法」 |
| 泛型 `Model<TReq,TRes>` 基接口 | spring-ai、eino `BaseModel[M]` | 共同祖先不承载行为，只制造类型体操 |
| 双消息协议并存 | eino | 第一法则：不为兼容留债 |
| `STUCK` 共同状态 | embabel | 只有 Planning 需要，不下沉（§2.5） |
| provider 专属概念进协议层（`HostedFileContent` 等） | MAF | 只有部分 provider 支持的 taxonomy 不进 Core |
| `ToolApprovalRequest/Response` 作为消息内容 | MAF | 审批归装配边界，进协议会污染所有 provider |
| 预制编排类型（sequential/concurrent/groupchat） | MAF `agentworkflow`、adk `workflowagents` | scope 用 examples + 行为断言证明组合能表达 |
| 框架仓库内含成品应用 | trpc `openclaw` | 刚删掉 `app`，architecture gate 锁死 |
| `Field-level mapping` 拓扑（`FieldMapping`/`FieldPath`） | eino `Workflow` | 会把 schema 知识带进拓扑层，收益不足以打破 Stage 边界 |
| ToolGroup / Resolver | embabel | DI 容器产物，Go 里显式传 toolset |
| retry layer / Transient 分类 | spring-ai `spring-ai-retry` | SDK 已内置重试，再加一层是乘法 |

---

## 6. 七个项目各自的一句话，以及它们照出的 scope

| 项目 | 一句话 | 它照出 scope 的什么 |
|---|---|---|
| **eino** | Go 泛型能表达多好的编排图 + 协议选错的复利成本 | **第一法则的价值**：44 处 Deprecated 和常驻的 "NOT RECOMMENDED" 是"不留债"的反证 |
| **trpc-agent-go** | "什么都给你"的起步速度，与 god struct / 字符串状态袋 / 30 个开关的全额账单 | **几乎每条反向不变量的活体对照组**；但可观测/评估/沙箱/容错四项真的领先 |
| **adk-go** | 负自由度下把 Go 写到最现代 | **流式原语被独立印证**；"永不破坏 API"的代价被完整展示 |
| **agent-framework-go** | 与 scope 最接近的设计取向，接近到 middleware 逐行相同 | **最强的 convergent design 证据**：这些不是趣味，是问题域逼出来的答案 |
| **GitNexus** | 跨语言跨领域的独立收敛，加上"喂 LLM 的数据不能说谎"这条第一原则 | 管线/依赖/所有权/原子性/权限五处独立同构；缺口在"不完整性的诚实建模" |
| **spring-ai** | 定义了 AI 集成层这个层次本身 | **词汇表的来源**，也是大部分反向不变量的具体所指 |
| **embabel-agent** | Agent 是规划器，计划由系统搜索得出 | **直系祖先**：领域语言全盘继承，状态所有权与执行契约全面重写 |

---

## 7. 最终判断

### scope 在赛道上的位置

**执行内核（D2）与恢复语义（D4）：领先，且不是一点。**
Step 纯归约、Effect 两阶段提交、EffectID replay contract、终态矩阵、预算划拨与能力衰减、TreeSnapshot 的树级校验 —— 这六项没有第二家做到，且它们互相咬合成一个整体：**因为 Step 纯，所以快照可重放；因为 Effect 分离，所以副作用有稳定身份；因为有稳定身份，所以恢复能诚实处理未知结算。**

**模块边界（D6）与治理（D8）：领先。**
85 个依赖岛 + 三层机器验证（API baseline / wire fixture / package DAG）+ 四层文档所有权分离，赛道内无出其右。

**协议（D1）与扩展（D5）：与 MAF 并列最优。**
两者独立收敛到同一答案，互为印证。

**编排（D3）：不同路线，各有取舍。**
scope 的封闭 6 词汇 + `flow`/`workflow` 双边界比开放 DAG 更严谨，但起步成本更高。eino 的 `Runnable` 四范式和 MAF 的类型化 Executor 图在**使用体验**上更成熟。

**易用性：明确落后。**
这是最诚实的结论。G1（开箱即用的 tool loop）是这个落后的集中体现：spring-ai 一个 Advisor、MAF 一个默认 middleware、eino 一个 `ChatModelAgent` —— scope 要么手写循环，要么上整个 Engine。

### 三条应当执行的动作

1. **补 G1 + G2**（`chatclient` 层的 tool loop 与 failover/hedge）。两者都是纯组合形态，不新造机器，直接填上 scope 与全场最刺眼的易用性差距。G1 的悬置条件（「需独立消费者证明」）已由三个项目满足。

2. **补 G3 + G4**（httprr cassette 回归测试、trace 跨 restore 延续）。两条都在现有边界内，纯增益，无设计争议。

3. **把 GitNexus 的两句话收进 `DESIGN_PHILOSOPHY.md`**：
   > "A missing route is a coverage limit; **an invented one is a lie**."
   > "A confident empty answer is **worse than a failure**, because it looks like a result."

   scope 已有这条原则的四个分身（`rag` 的"不把不完整结果伪装成完整命中"、`core` 的"不让探测错误以 0/空值静默返回"、`tools` 的"截断而非报错但带标记"、`etl` 的"超限不产出部分文档"）。给它一句统一判词，成本为零，收益是把四条散落条款收成一条可推理的原则 —— 这正是 `DESIGN_PHILOSOPHY.md` 的定位。

### 一句话收尾

> **scope 在"一个 Agent 执行实例到底是什么"这个问题上，给出了这七个项目里唯一一个经得起崩溃、恢复、递归委派和权限衰减推敲的答案；代价是它在"我只想让模型自动调个工具"这个最常见的问题上，还没给出足够短的路径。**
> 前者无法靠时间补上，后者可以 —— 而且按 scope 自己的设计哲学，它只需要一个组合函数。
