# adk-go（Google）—— 多语言一致的 Agent 语义

> 实测：`go 1.26.6`（七个项目里最新），343 个生产 `.go` 文件，61,795 行生产代码，2 个 module。
> 依赖：`google.golang.org/genai`、a2a-go、mcp-go-sdk、openai-go/v3、cloud.google.com/go 全家、OTel、cobra、gorilla/mux、jsonschema-go、safehtml。

---

## 0. 一句话定位

**adk-go 不是一个独立设计的 Go 框架，而是 ADK 概念模型的第 5 个语言实现**（Python / Java / Kotlin / TypeScript / Go），且 `AGENTS.md` 明写：

> **adk-python is the source of truth for feature behavior.** When porting or validating a feature, check parity with the Python implementation.

理解 adk-go 的一切取舍，都要先接受这条约束：**它的设计自由度是负的** —— 概念模型由 Python 定，Go 侧只能在"如何用 Go 表达它"上做选择。有意思的是，在这个极窄的空间里，它做出了七个项目中**最现代的 Go 惯用法**。

---

## 1. D1 协议层：Gemini 协议即协议

这是 adk-go 最激进、也最容易被误读的决定：

```go
// model/llm.go
type LLM interface {
	Name() string
	GenerateContent(ctx context.Context, req *LLMRequest, stream bool) iter.Seq2[*LLMResponse, error]
}

type LLMRequest struct {
	Model    string
	Contents []*genai.Content                  // ← google.golang.org/genai
	Config   *genai.GenerateContentConfig      // ← google.golang.org/genai
	Tools    map[string]any `json:"-"`
}

type LLMResponse struct {
	Content             *genai.Content
	CitationMetadata    *genai.CitationMetadata
	GroundingMetadata   *genai.GroundingMetadata
	UsageMetadata       *genai.GenerateContentResponseUsageMetadata
	LogprobsResult      *genai.LogprobsResult
	InputTranscription  *genai.Transcription
	OutputTranscription *genai.Transcription
	Partial             bool
	TurnComplete        bool
	Interrupted         bool
	SessionResumptionHandle string
	FinishReason        genai.FinishReason
	AvgLogprobs         float64
	...
}
```

**Gemini 的 wire 类型就是框架的领域模型。** `openaimodel` provider 的工作是把 OpenAI 响应翻译成 `genai.Content`。

### 这是"model-agnostic"吗

README 说 "model-agnostic, optimized for Gemini"。准确的说法是：**接口 agnostic，协议不 agnostic**。任何非 Gemini provider 都要做有损翻译 —— `GroundingMetadata`、`LogprobsResult`、`InputTranscription`、`SessionResumptionHandle` 这些字段在 OpenAI/Anthropic 侧要么没有，要么语义不同。反过来，Anthropic 的 reasoning signature（续流必需）在 `genai.Content` 里没有精确落点。

### 与 scope 的对照

scope 的根 `CLAUDE.md` 有一条直接命中的红线：

> 外部协议/provider 的品牌只在表达客观集成事实时出现，**不能成为 Scope 自有概念的命名锚**。

以及 `models/CLAUDE.md`：

> **wire protocol 是更低一层的实现**：跨 provider 复用的 OpenAI 与 Anthropic wire 分别位于独立 `models/protocol/openai`、`models/protocol/anthropic` module。**公开 Config、DTO 和方法签名不得泄露 wire/SDK 类型。**

scope 的结构是三层：

```
core/chat.Message / Part          ← 中立协议（Part 是 tagged value）
models/protocol/{openai,anthropic} ← 两种共享 wire 的实现（独立 module）
models/<provider>                  ← 30+ 个叶子 module
```

`models/CLAUDE.md` 还专门为 reasoning 差异写了条款：

> **能力差异按 provider 填空**：reasoning signature（续流必需）有的家有、有的没有，适配层用中性字节承载，不强求统一。

**判断**：adk-go 的选择在 Google 的语境下是理性的（Gemini 是一等公民，其余是兼容）。但它使得"换一家模型"不是配置问题而是保真度问题。scope 承担了额外的中立层成本（`core/chat` 12,579 行里相当部分是协议建模 + wire fixture），换来的是**任何 provider 都不比另一个更一等**。

这是一个必须付、且已经付了的成本 —— adk-go 反过来证明了不付这个成本的样子。

---

## 2. D2 执行内核：sealed Agent + god InvocationContext

### 2.1 Sealed interface

```go
type Agent interface {
	Name() string
	Description() string
	Run(InvocationContext) iter.Seq2[*session.Event, error]
	SubAgents() []Agent
	FindAgent(name string) Agent
	FindSubAgent(name string) Agent

	internal() *agent      // ← 未导出方法：外部包无法实现此接口
}
```

注释坦白了这是权宜之计：

> NOTE: in future releases we will allow just implementing this interface. For now `agent.New` is a correct solution to create custom agents.

`AGENTS.md` 里也写：

> **Add an agent type:** follow the `agent/workflowagents/*` packages; construct agents via `llmagent.New` / `agent.New`, **not by implementing `agent.Agent` directly**.

**这是一个 sealed hierarchy**：接口用未导出方法封口，扩展只能通过 `agent.New(Config{Run: func(...)})` 注入函数。

scope `core/CLAUDE.md` 的对应红线：

> **Tagged value，而非 sealed hierarchy**：Message/Part 使用公开 discriminator 与普通值；未知类型返回可诊断错误，**不依赖未导出方法封口**。

scope 的 `Definition`/`Execution` 是完全开放的接口，任何包都能实现。代价是框架不能假设"所有 Definition 都持有某个内部结构"，好处是**准入规则写在文档和 gate 里，而不是靠编译器锁死**。scope 用 `ARCHITECTURE.md §7.6 新策略准入` 来管这件事 —— 一条明确的、可以被论证推翻的规则，而不是一个不可绕过的语言技巧。

### 2.2 `InvocationContext` 就是 `context.Context`

```go
type InvocationContext interface {
	context.Context          // ← 直接嵌入

	Agent() Agent
	Artifacts() Artifacts
	Memory() Memory
	Session() session.Session
	InvocationID() string
	Branch() string
	IsolationScope() ...
	EndInvocation() ...
	...
}
```

它同时是取消信号载体和服务定位器。scope 的红线（`ARCHITECTURE.md §15.1`）：

> `context.Context` **只传取消、deadline 和请求范围值**，不进入 snapshot。

这个设计的连锁后果：

- 无法 snapshot（含 Agent 接口值、Session 服务）
- 无法在类型上表达"这个 Agent 只需要 Session，不需要 Memory"
- ISP 无从谈起：所有自定义 Agent 都拿到全部能力

不过 adk-go 有一个补偿设计值得注意：`agent/common_context_delta.go` 的 `InvocationContextDelta` —— 用**增量**表达上下文变化而不是就地改，这比 trpc 的整体 clone 干净。

### 2.3 `iter.Seq2` ✅

```go
Run(InvocationContext) iter.Seq2[*session.Event, error]
```

`AGENTS.md`：

> **Streaming:** agent runs return `iter.Seq2[*session.Event, error]`; consume with `for event, err := range … {}`. **Don't collect events into a slice.**

**这是七个项目里唯一一个在 Agent 主循环上用 `iter.Seq2` 的**（MAF 在 `RunFunc` 上也用了，见该文档）。eino 用 `AsyncIterator` + channel，trpc 用 `<-chan *event.Event`。

scope 的 `core/CLAUDE.md`：

> **流式使用 `iter.Seq2`**：不自定义 iterator，不用 channel 冒充拉模型；调用方提前停止、context cancel 和首错终止必须有测试。

**完全同构。** 这是本次对比中最强的一个独立收敛证据：Go 团队自己的 ADK 实现和 scope 在没有互相参考的情况下选了同一个流式原语，而两个更早启动的项目（eino go1.18、trpc go1.21）因为锁死了语言版本用不上。

---

## 3. D3 编排：两套并存

adk-go 里有**两个不相交的编排系统**：

| 系统 | 位置 | 形态 |
|---|---|---|
| Workflow Agents | `agent/workflowagents/` | `sequentialagent` / `parallelagent` / `loopagent`，Agent 组合 Agent |
| Workflow Engine | `workflow/` | `graph.go` / `base_node.go` / `agent_node.go` / `function_node.go` / `join_node.go` / `branch.go` / `edgebuilder.go` / `dynamic_node.go` / `dynamic_scheduler.go` |

`workflow/` 里还有 `conformance_parity_test.go`（与 Python 对齐）、`hitl_test.go`、`branch_isolation_test.go`。

同时 `agent/dynamic_scheduler.go` 和 `workflow/dynamic_scheduler.go` 各有一份。

**这是"两个概念一个术语"的反面**：`sequentialagent` 和 `workflow` 的顺序节点做的是同一件事，用户要在两套 API 里选。

scope 的对应原则（`ARCHITECTURE.md §3.2`）：

> **一个概念只有一个术语。** 不同时保留 plan/todo、run/execution、sub-agent/child-agent 等近义公共概念。

以及 §14 的包集合约束：`agent/` 下**精确只有** `interaction/` `planning/` `planning/goap/` `workflow/` `otel/` `platform/` `examples/` `doc/`，且「新 package 只有在独立变化原因和真实消费者已被证明、ADR 已更新生产 package 集合与允许边后才能建立」，并有全图 architecture test 锁定包集合。

---

## 4. D4 持久化：Session 即状态

adk-go 没有独立的执行快照概念。状态载体是 `session.Session`（events + state），后端有 `inmemory` / `database`(sqlite/gorm) / `vertexai`。恢复 = 重放 session events。

`agent/live.go` + `LLMResponse.SessionResumptionHandle` 表明 **bidi/live 场景的恢复是委托给 Gemini 服务端的**（Gemini Live API 有 session resumption）。这是一个很清楚的取舍：**不自己做恢复，用平台的**。

### 与 scope 的对照

| | adk-go | scope |
|---|---|---|
| 恢复单位 | Session（产品会话） | Process / TreeSnapshot（执行实例树） |
| 恢复方式 | 重放 events | 恢复 `ExecutionState` envelope + prepared step |
| 副作用一致性 | 无框架级保证 | prepare/finalize 两阶段 + EffectID replay contract |
| 谁持久化 | 框架（SessionService） | Host（Framework 只保证一致 capture 点） |
| 预算/能力衰减 | ❌ | ✅ 子从父划拨，能力只能衰减 |

scope 的 `ARCHITECTURE.md §12` 有一条 adk-go 完全没有对应物的条款：

> Interaction 默认把精确恢复所需的 WorkingContext **自足地**保存在私有 ExecutionState；**Host 的产品历史不是恢复时可静默重建它的第二真相源。**

adk-go 恰恰相反：产品历史（Session）**就是**唯一真相源。这在纯对话场景成立，在"工具调用打到一半崩了"的场景就不成立了 —— session 里没有"哪些 tool call 已发出、结算未知"这个事实。

---

## 5. D5 扩展机制：callback + plugin 两套

```go
type BeforeAgentCallback func(Context) (*genai.Content, error)
type AfterAgentCallback  func(Context) (*genai.Content, error)
// 同型的还有 Before/AfterModelCallback、Before/AfterToolCallback
```

语义：**返回非 nil 就短路**。

外加 `plugin/`：`plugin.New(plugin.Config{...})` 注册 `Before*`/`After*` 全套，内置 `loggingplugin` / `retryandreflect` / `functioncallmodifier` / `agentanalytics`（独立 module）。

`AGENTS.md`：

> **Add cross-cutting behavior:** register a `plugin.New(plugin.Config{...})` hook (`Before*`/`After*` for run/agent/model/tool) instead of editing the loop.

**问题**：callback 和 plugin 是同一个机制的两个入口（一个 per-agent、一个 global）。而且"返回非 nil 短路"把**观察**和**改变行为**焊死在一个签名里 —— 你想加个日志，签名却允许你悄悄换掉模型响应。

scope 的两分法（`ARCHITECTURE.md §13`）：

> `ProcessAdmitter` 只负责启动准入；`ProcessStartOutcomeAcknowledger` 只负责接受结论；`EventListener`/`DeltaListener` **只负责观察**。它们语义不同，**不合并成 Policy/Guard/Middleware 近义层**。
> EventListener 与 DeltaListener 都是**无错误返回**的观察接口：返回值既不会改变事实，也不应制造"可否决执行"的误解。

这条对比很能说明问题：adk-go 的 `BeforeModelCallback` 一个签名同时可以做「记日志」「改 prompt」「直接伪造响应」「否决执行」。scope 把这四件事拆到四个不同的、语义单一的位置。

---

## 6. D6/D7 模块与可观测性

**2 个 module**：根 + `plugin/agentanalytics`。`AGENTS.md` 有一条 CI 强制的护栏：

> The root `go.mod` does not require an in-repo submodule (enforced by the `guardrail` CI job).

✅ 这与 scope 的依赖方向 gate 同构（scope 用 `dev/repoarch` + `core/internal/arch` + 各 module 的 `architecture_test.go`）。

**可观测性**：`telemetry/` + `internal/telemetry`，OTel trace + log exporter，`detectors/gcp` 做 GCP 资源检测。agent 包直接 `import go.opentelemetry.io/otel/trace`。

对照 scope：**领域模块不 import OTel**，由独立 `otel` module 反向装饰，且 Kernel architecture gate 禁止任何 OTel import。adk-go 是直接内嵌型。

两种都能工作。scope 这条的收益在 `agent/otel` 的 gate 里写得很清楚：adapter 只消费 Framework Event，**不把 raw payload、Input、Output 或产品身份写进 telemetry**。内嵌型没有这道结构性隔离，防泄漏只能靠 review。

---

## 7. D8 工程治理 —— adk-go 最强的一面

`AGENTS.md` 的 Boundaries 段：

```
Ask first:
  - Adding or upgrading a dependency (go.mod)
  - Changing a high-fan-in package (session, agent, model, tool, runner)
  - Any change to the public API surface, and any breaking change
Never:
  - Break the public API — keep changes backward-compatible
  - Add tests that make live LLM or network calls
```

**Definition of done** 是可执行的 6 条清单（build / test -race -shuffle / lint 每 module / `go mod tidy -diff` 每 module / 新行为有测试 / 每个新文件有 license header）。

### 测试基础设施：httprr cassette

```
Tests run offline by default: LLM HTTP traffic is replayed from testdata/*.httprr
-httprecord takes a regexp matched against the cassette's file path
每个 package 的 //go:generate 指令被 scoped，使每个 cassette 恰好被录一次
（由 TestHTTPRecordDirectivesPartitionCassettes 强制）
```

**这是七个项目里最好的 LLM 测试方案。** 它解决的问题很实在：既要测真实 provider 行为，又不能让 CI 打真实 API。

对照 scope：`core/modeltest` 提供跨 provider 行为契约 suite、`core/vectorstore/storetest` 提供 store conformance、`dev/providerconformance` 做跨 provider 构造/API 一致性。scope 的 suite 是**契约级**（"任何 chat.Model 必须满足这些行为"），adk-go 的 cassette 是**回归级**（"这次调用的真实响应长这样"）。

**两者不冲突且互补**：scope 目前缺的正是回归级 —— 一次真实 provider 响应的固化记录。当某家 provider 悄悄改了流式分块方式，contract suite 里的 fake 不会发现，cassette 会。

### 与 scope 治理的根本分歧

| | adk-go | scope |
|---|---|---|
| 公开 API | **Never break** | pre-1.0，咨询后直接改 |
| 概念来源 | adk-python 是 source of truth | 本质第一性，业界只取思想不作命名锚 |
| 破坏性变更 | Ask first + 尽量 additive | 默认倾向改对而非将就 |

adk-go 的 `internal() *agent` 封口、两套编排并存、`InvocationContext` 嵌 `context.Context` —— 这三个都是"Never break the public API"的产物：一旦发出去就改不动了，只能在旁边加新的。

---

## 8. 结论

### 同构（最强的独立收敛）

| 取舍 | adk-go | scope | 备注 |
|---|---|---|---|
| **`iter.Seq2` 做流式** | ✅ | ✅ | 七项目中仅两家（另一家 MAF），最强证据 |
| 现代 Go 版本 | ✅ 1.26.6 | ✅ 1.27 | eino 1.18 / trpc 1.21 用不上现代 stdlib |
| 接口优先、具体实现在子包 | ✅ | ✅ | |
| 依赖方向 CI gate | ✅ guardrail job | ✅ repoarch + arch tests | |
| `%w` 包装，sentinel 优先 | ✅ | ✅ | adk 甚至警告"不要把 `%w` 改成 `%v`" |
| Toolset 可按 invocation 返回不同 tools | ✅ | ✅ `tool.Registry` 实例化，无全局 | |
| 单向 module 依赖（根不依赖子 module） | ✅ | ✅ | |

### 分歧

1. **Gemini 协议即领域模型** —— scope: 中立 `core/chat` + `models/protocol/*` 分层。
2. **`internal()` sealed interface** —— scope: tagged value / 开放接口，准入靠 ADR 而非编译器封口。
3. **`InvocationContext` 嵌 `context.Context` 且是服务定位器** —— scope: ctx 只传取消/deadline。
4. **两套编排并存（workflowagents vs workflow）** —— scope: 一个概念一个术语 + 包集合被 gate 锁定。
5. **Session 即恢复单位** —— scope: Process/TreeSnapshot，Host 持久化。
6. **callback 返回非 nil 短路（观察与改行为同签名）** —— scope: 观察接口无错误返回。
7. **领域包直接 import OTel** —— scope: `otel/*` 反向装饰。
8. **Never break the public API** —— scope: pre-1.0 咨询后直接改对。

### 真实缺口（scope 应认真对待）

| # | 能力 | 说明 | 建议 |
|---|---|---|---|
| 1 | **HTTP cassette 回归测试（httprr）** | 离线回放真实 provider 响应；`//go:generate` 分区保证每个 cassette 恰好录一次 | **强烈建议吸收**。scope 有 `core/modeltest`（契约级），缺回归级。落点：`core/modeltest` 增加 cassette 支持，或独立 `dev/` 工具。这不违反任何边界，纯增益 |
| 2 | **`InvocationContextDelta`（增量上下文）** | 用 delta 表达上下文变化而非整体 clone | scope 的 `Signal`/`Transition` 已经是更强的形态（意图与事实分离），无需吸收 |
| 3 | **`platform/` 可替换 seam（time/uuid）** | 为确定性测试把 `time.Now`/`uuid.New` 做成可覆盖点 | scope 的 `Step` 已经禁止读 clock/random（「不得读取 clock/random/global state，所需变化必须先成为 Signal」），这是**更强**的解法。无需吸收 |
| 4 | **A2A / Agent Registry 集成** | `agentregistry/` 对接 Google Cloud Agent Registry | scope 的 `a2a` module 是薄适配；registry 属于 Host。无需吸收 |

### 判词

> **adk-go 是"在负自由度下能把 Go 写得多现代"的样本。**
> `iter.Seq2`、go 1.26、per-module lint/tidy gate、httprr cassette —— 这些是它在概念模型被 Python 锁死之后，在剩下的空间里做到的极限，且水平很高。
> 而它的每一处结构性问题（sealed interface、god context、双编排、Gemini 协议内嵌）都可以追溯到同一个根因：**"Never break the public API" + "Python 是真理源"**。
> 对 scope 的意义是双向的：**流式原语的选择被独立印证了**；**"永不破坏"的代价被完整展示了**。唯一值得直接搬的是 httprr 那套 cassette 测试基础设施。
