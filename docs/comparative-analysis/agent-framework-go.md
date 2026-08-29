# agent-framework-go（Microsoft MAF）—— 企业级工作流内核

> 实测：`go 1.26.0`，325 个生产 `.go` 文件，73,995 行生产代码，**单 module**。
> 依赖：openai-go/v3、anthropic-sdk-go、genai、azure-sdk、copilot-sdk、a2a-go/v2、ag-ui、mcp-go-sdk、OTel、jsonschema-go、gofrs/flock。
> 状态：public preview，`docs/dotnet-go-sdk-feature-comparison.md` 显式列出「Go 尚未实现」的功能（Handoff orchestration、Declarative Agents、DevUI、AF Labs、Foundry-hosted deployment）。

---

## 0. 一句话定位

**MAF Go 是七个项目里工程质量最高、且与 scope 设计取向最接近的一个。** 它的 `workflow` 包是一个认真的类型化 Executor 图内核（Pregel 超步 + 类型路由 + checkpoint + HITL port + 子工作流 + 所有权令牌），它的 `agent` 包用了和 scope **逐行同构**的 middleware 组合算法，它的 `message` 包用了和 scope 同构的 Content 多态建模。

它的问题不在设计能力，而在**跨语言对齐的税** —— 它是 .NET MAF 的移植，语义必须与 .NET 一致，于是 Go 侧背上了一些不属于 Go 的形状。

---

## 1. D1 协议层：`Content` 接口 + 20+ 具体类型 ✅

```go
type ContentHeader struct { ... }
type Content interface { ... }
type ToolCallContent interface { ... }
type Contents []Content

type Message struct {
	Role     Role
	Source   Source        // ← 消息来源（含 SourceTypeMiddleware）
	Contents Contents
	...
}
```

具体 Content 类型（实测 20+）：

```
TextContent  TextReasoningContent  DataContent  URIContent  ErrorContent  RawContent
FunctionCallContent  FunctionResultContent
ToolApprovalRequestContent  ToolApprovalResponseContent  AlwaysApproveToolApprovalResponseContent
MCPServerToolCallContent  MCPServerToolResultContent
CodeInterpreterToolCallContent  CodeInterpreterToolResultContent
HostedFileContent  HostedVectorStoreContent
UsageContent + UsageDetails
```

每个类型自带 `serializedXxx` 影子结构做 JSON 编解码（`serializedDataContent`、`serializedFunctionCallContent`、`serializedToolApprovalRequestContent`…）。

### 与 scope 的对照 —— 高度同构

scope `core/chat` 的 `Message` 持有 `[]Part`，`Part` 用公开 discriminator 分辨类型。两者是同一个模式：**tagged value 的多态内容块**，而不是 OpenAI 那种 `Content string + ToolCalls []` 的扁平结构。

三处值得注意的差异：

1. **`TextReasoningContent` 是一等公民** —— 与 scope 的「Reasoning 是 first-class」完全一致，且两者都是独立于普通文本的内容类型，不是 metadata 字段。
2. **`ToolApprovalRequestContent` / `ToolApprovalResponseContent` 进了协议层** —— MAF 把 HITL 工具审批做成了**消息内容类型**。scope 把审批放在装配边界（`tools/CLAUDE.md`：「权限、sandbox、once-only、产品审批、事务和业务幂等属于具体装配边界」），协议层不认识"审批"。**MAF 的做法有一个真实好处**：审批状态随消息一起序列化、一起进历史、一起回放，不需要第二套状态。**代价**是 `core/chat` 级别的协议被塞进了一个只有部分 provider/runtime 关心的概念。scope 的选择更符合 §2.5「只有一个消费方需要的东西不往下沉」。
3. **`MCPServerToolCallContent` / `HostedFileContent` / `HostedVectorStoreContent`** —— 这些是 OpenAI Responses API 的服务端托管工具概念，被提升成了通用协议类型。这是 scope 明确禁止的：「❌ 把只由单一 provider 支持、或跨 provider 语义不同的 option/taxonomy 提升为 Core 固定字段」。

`Message.Source` 字段（含 `SourceTypeMiddleware`）是个巧思：**消息知道自己是谁注入的**。scope 目前没有对应物 —— `interaction` 的 WorkingContext 里，middleware 注入的消息和模型产生的消息在类型上不可区分。这是一个小但真实的可观测性缺口。

---

## 2. D2 执行内核：Agent 是 struct，不是 interface

```go
type RunFunc = func(ctx context.Context, messages []*message.Message, options ...Option) iter.Seq2[*ResponseUpdate, error]

type ProviderConfig struct {
	ProviderName  string
	Run           RunFunc                                    // ← provider 只提供一个函数
	Middlewares   []Middleware
	Format        func(v any) (ResponseFormat, error)
	Unmarshal     func(format ResponseFormat, data []byte, v any) error
	CreateSession func(ctx context.Context, session *Session, options ...Option) error
	ServiceDoesNotManageHistory bool
}

type Agent struct {          // ← 具体 struct，不是接口
	id, name, description string
	provider              ProviderConfig
	providerPipeline      RunFunc
	runPipeline           RunFunc
	historyProvider       HistoryProvider
	contextProviders      []ContextProvider
	...
}
func New(prov ProviderConfig, cfg Config) *Agent
```

**"accept interfaces, return structs" 的彻底执行**：扩展点是 `RunFunc`（一个函数类型），不是 `Agent` 接口。任何 provider 只需提供一个 `func(...) iter.Seq2[*ResponseUpdate, error]`。

这比 adk-go 的 sealed interface 和 trpc 的 5 方法胖接口都干净：**没有"实现 Agent 接口"这件事，只有"提供一个 Run 函数"**。

### 两处 .NET 移植的疤痕

```go
type Config struct {
	ThrowOnHistoryProviderConflict *bool   // ← 三态 bool
	WarnOnHistoryProviderConflict  *bool
	ClearOnHistoryProviderConflict *bool
	...
}
```

三个 `*bool` 表达"未设置 / true / false"三态，语义是「配置的 HistoryProvider 与服务端托管历史冲突时怎么办」。注释明说是 "matching the .NET clear-on-conflict semantics"。

Go 的惯用法是 options struct + 有意义的零值，或一个枚举 `HistoryConflictPolicy`。三个独立的三态布尔互相耦合（Throw 优先于 Clear）是典型的**配置项爆炸**。scope 的对应红线：`config 使用 options struct，不使用 builder 链和大量 variadic WithXxx`，且「make zero values useful」。

`Agent` struct 里还有：

```go
historyCleared       atomic.Bool   // 运行期状态
hasConfiguredHistory bool
hasDefaultHistoryProvider bool
throwOnHistoryConflict    bool
warnOnHistoryConflict     bool
clearOnHistoryConflict    bool
providerDoesNotManageHistory bool
```

**7 个字段全部服务于同一个问题**："history 到底该由谁管"。这个问题之所以这么复杂，是因为 MAF 要同时支持 client-managed history 和 service-managed history（Foundry / OpenAI Assistants 那种服务端存会话），而两者可能同时被配置。

scope 没有这个问题，因为 `history` 是独立能力（`core/history`），Agent Framework 的 `interaction` 拥有自己的 WorkingContext，且架构明写「Host 的产品历史不是恢复时可静默重建它的第二真相源」—— **一个 Execution 只有一个历史真相源，不存在冲突**。

---

## 3. D5 扩展机制：`Middleware` —— 与 scope 逐行同构 ⭐

```go
// MAF: agent/middleware.go
func compileRunChain(fn RunFunc, middlewares []Middleware) RunFunc {
	for _, mw := range slices.Backward(middlewares) {
		if mw == nil { continue }
		...
	}
}
```

```go
// scope: core/chat/middleware.go
func compose[T any, M ~func(T) T](endpoint T, middlewares []M) T {
	wrapped := endpoint
	for _, middleware := range slices.Backward(middlewares) {
		if middleware != nil {
			wrapped = middleware(wrapped)
		}
	}
	return wrapped
}
```

**同一个算法、同一个 stdlib 函数、同一个 nil 跳过策略、同一个 outermost-first 语义。** 两边的文档措辞也几乎一样：

- MAF：`Middleware implementations must treat input message and option slices, and existing messages, as read-only. To modify the downstream invocation, clone the slices and any message being changed.`
- scope `core/chat/model.go`：`must not retain or mutate request, and transfers ownership of the returned response to the caller.`

这是本次对比中**最直接的独立收敛证据**：两个团队在完全无关的语境下，为"模型调用的横切扩展"选了同一个原语和同一套所有权契约。

MAF 还有第二层扩展 `ContextProvider`（请求前注入 / 响应后持久化上下文），文档明确划了分界：

> Use middleware when an extension needs **direct control over provider invocation**, streaming updates, option propagation, or error handling **beyond the request/response message hooks** exposed by ContextProvider.

即：**Middleware = 控制调用；ContextProvider = 只碰消息**。这是一个诚实的两层划分，比 eino/adk 的"一个 callback 什么都能干"清楚得多。

不过 scope 仍更严格：scope 的观察面（`EventListener`/`DeltaListener`）是**无错误返回**的，从签名上就不可能改变事实。MAF 的 `ContextProvider` 仍能改消息。

---

## 4. D3 编排：`workflow` —— MAF 的皇冠

这是七个项目里**最完整的工作流内核**。

### 4.1 Executor：可选行为的组合体

```go
type Executor struct {
	ID               string
	ImplementationID string
	AutoSendMessageHandlerResultObject *bool
	AutoYieldOutputHandlerResultObject *bool
	CrossRunShareable                  bool

	ConfigureProtocol func(*ProtocolBuilder) (*ProtocolBuilder, error)
	InitializeFunc    func(*Context) error
	AttachRuntimeFunc func(runtime any) error
	ResetFunc         func() error
	CloseFunc         func(context.Context) error

	OnCheckpointFunc          func(*Context) error
	OnCheckpointRestoredFunc  func(*Context) error
	OnMessageDeliveryStartingFunc func(*Context) error
	OnMessageDeliveryFinishedFunc func(*Context) error
	...
}
```

11 个可选回调字段。文档写「零值没有行为；必须至少有一个非 nil route 或 lifecycle callback」。

这是 scope 会判为**具名插槽过多**的形态（`DESIGN_PHILOSOPHY.md §2.3`：插槽多 = 表面积大 + 心智负担）。但要公允：这些回调**语义各不相同**（初始化 / 重置 / 关闭 / checkpoint 前 / checkpoint 后 / 超步开始 / 超步结束），不是同一件事的多个入口。这更接近"生命周期钩子"而非"扩展链"。

### 4.2 类型化协议：`AttrYieldsOutput[T]` / `AttrSendsMessage[T]`

```go
type AttrYieldsOutput[T any] struct{ _ [0]*T }
type AttrSendsMessage[T any] struct{ _ [0]*T }
```

零大小字段做**类型级声明**：把它作为 struct 字段嵌入，框架用反射读出 `T`，从而知道这个 Executor 会发出/产出什么类型。配合 `ProtocolDescriptor` 和 `DescribeProtocol()`，图在构造期就能校验"A 发的类型 B 能收"。

这是一个精巧的 Go 技巧（用零大小 phantom type 把类型信息编码进 struct 布局），但也是**反射 + 编译期不校验**的组合：类型不匹配在 `Build()` 时才发现，不是 `go build` 时。

scope 的等价物是 Descriptor 携带的 JSON Schema + 「相邻 schema 在构造时精确衔接」。两者都是构造期校验，scope 用 schema（可序列化、可进 Deployment digest），MAF 用 `reflect.Type`（进程内有效，不可序列化）。

**scope 的选择更适合"跨进程恢复"**：`DeploymentRef` 里的 schema 能在另一个进程里重新校验，`reflect.Type` 不能。MAF 用 `PortableValue` 补这个洞：

```go
type PortableValue struct { ... }
func AnyPortableValue(v any) PortableValue
func PortableValueAs[T any](v PortableValue) (T, bool)
func (v PortableValue) MarshalJSON() ([]byte, error)
// 内部：TypeID → json.RawMessage，decodeKnownPortableType / decodeBuiltinPortableType / decodePortableJSONType
```

`TypeID` 由 `typeid.go` + `typeid_reflect2_gc.go`（**依赖 Go 运行时内部布局的 build-tag 文件**）生成。这是一个危险的实现细节 —— 它把跨进程可移植性押在了 Go 编译器的内部表示上。

### 4.3 HITL：`RequestPort`

```go
type RequestPort struct {
	ID       string
	Request  reflect.Type
	Response reflect.Type
}
type ExternalRequest  struct { PortInfo RequestPortInfo; RequestID string; Data PortableValue; ... }
type ExternalResponse struct { PortInfo RequestPortInfo; RequestID string; Data PortableValue }
```

**工作流可以声明"我需要外部回答"的端口，带类型化的请求/响应契约和稳定的 RequestID。**

### 与 scope 的对照 —— 高度同构

scope 的对应机制（`ARCHITECTURE.md §6.6`）：

```
WaitID 由 Engine 铸造，Execution 不能自行生成外部等待身份。
Execution 先通过 Transition 声明 logical wait；
Engine 在 Effect settlement/finalize 时原子保存 WaitID 与 logical wait 映射并入队包含 WaitID 的内部 Signal，
Execution 在下一 Step 将它写入私有状态并显式进入 Waiting。
```

`RequestPort.ID` ≈ scope 的 logical wait key，`RequestID` ≈ `WaitID`，`ExternalResponse` ≈ 带 WaitID 的 Signal。

**但 scope 多了三条 MAF 没有的不变量**：
1. WaitID **由 Engine 铸造**，Execution 不能自己造外部身份（防止策略伪造等待目标）；
2. Signal 投递合同：同一 SignalID 重复提交只产生一次逻辑消费；Waiting Process **只接受当前 WaitID 允许的输入**，过期/已结算/错目标输入确定失败；
3. 契约用 **JSON Schema 而非 `reflect.Type`**，因此可序列化、可跨进程、可交给非 Go 的 Host。实测 `agent/interaction/pending_input.go`：

```go
type PendingToolInput struct {
	waitID         agent.WaitID
	prompt         json.RawMessage
	responseSchema json.RawMessage   // ← 权威 JSON Schema
}
// ResponseSignal 先按 responseSchema 本地校验，通过后才铸出 WaitID 寻址的 SignalRequest
```

MAF 的 `RequestID` 是 `uuid.New()`，去重语义未在类型层表达；`Request/Response reflect.Type` 不可序列化，要靠 `PortableValue` + 依赖运行时布局的 `TypeID` 补。**这一条 scope 领先。**

### 4.4 Checkpoint

```go
type Store[T any] interface {
	CreateCheckpoint(ctx, sessionID string, data T, parent *workflow.CheckpointInfo) (workflow.CheckpointInfo, error)
	RetrieveCheckpoint(ctx, sessionID string, info workflow.CheckpointInfo) (T, error)
	RetrieveIndex(ctx, sessionID string, withParent *workflow.CheckpointInfo) ([]workflow.CheckpointInfo, error)
}
```

注释里有一句非常关键：

> The framework **serialises internal checkpoint state before calling the store**, so store implementations never need to understand the checkpoint structure.

**框架先序列化，Store 只负责耐久存储。** 这与 scope 的分层完全一致（`ARCHITECTURE.md §12`）：

> Agent Framework 负责捕获一致的 Process/tree 执行状态、校验 schema/DeploymentRef/父子关系/状态机不变量。
> Host 负责 Store、transaction、CAS、lease、幂等、retention。
> **Agent 不定义 Store/Repository 来假装拥有持久化。**

⚠️ 一处差异：MAF **定义了 `Store[T]` 接口**（虽然是泛型且语义极窄）。scope 明确不定义 Store —— `Engine` 只提供 capture/restore 能力，Host 拿到 `TreeSnapshot` 自己决定存哪。

MAF 的 `Store[T]` 带 `parent *CheckpointInfo` 和 `RetrieveIndex(withParent)` —— **checkpoint 是一棵树，支持从任一历史点分叉**。这是 scope 目前没有的能力（scope 的 `TreeSnapshot` 是 Process 树的快照，不是 checkpoint 的历史树）。

### 4.5 所有权令牌

```go
func (w *Workflow) CheckOwnership(token any) bool
func (w *Workflow) TakeOwnership(token any, newToken any, subworkflow bool) error
func (w *Workflow) ReleaseOwnership(token any) error
func (w *Workflow) ReleaseOwnershipTo(token any, targetToken any) error
func (w *Workflow) AllowConcurrent() bool
func (w *Workflow) TryReset() bool
```

用 `any` 令牌 + `ownershipTokenIdentity(token) (reflect.Type, uintptr, bool)` 判身份 —— 拿指针地址当身份。

这是一个**运行期动态所有权系统**：一个 `*Workflow` 值同一时间只能被一个 run 拥有，除非 `CrossRunShareable`。scope 的等价保证是**类型层面的**：`Definition 创建后不可变，并发共享安全`、`每次 Start 创建独立 Execution`、`Execution 不被多个 Process 并发推进`。不需要运行期令牌，因为不可变性从构造期就成立。

MAF 需要令牌，是因为 `Executor` 有 `ResetFunc`/`InitializeFunc` —— **Executor 是有状态、可复用的对象**，不是不可变定义。这是根因差异。

### 4.6 `agentworkflow`：预制编排

```
sequential.go / concurrent.go / groupchat.go / hosting.go / message_merger.go / builders.go
```

用 workflow 内核搭出的现成模式。`docs/dotnet-go-sdk-feature-comparison.md` 注明 **Handoff orchestration 尚未实现**。

scope 的对应物是 `agent/examples` + `ARCHITECTURE.md §10` 的 Anthropic 模式覆盖表 —— **不提供预制 Agent 类型，只提供可运行示例证明组合能表达该模式**：

> 模式名称是组合词汇，不是必须各建一个 Strategy/package/type 的清单。
> 验收同时要求可运行示例和行为断言；只有 topology 类型或文档中的模式名不算实现。

两种都合理。MAF 的预制件降低起步成本；scope 的示例避免了"每个模式一个类型"的表面积膨胀。

---

## 5. D6 模块边界：单 module 的代价

**一个 `go.mod`，直接依赖**：openai-go/v3 + anthropic-sdk-go + genai + azure-sdk(azcore+azidentity) + copilot-sdk + a2a-go/v2 + ag-ui + mcp-go-sdk + grpc + OTel。

任何 `import "github.com/microsoft/agent-framework-go/agent"` 的程序，都拉进 OpenAI + Anthropic + Google + Azure + GitHub Copilot **五家 SDK** 加 gRPC。

`provider/` 下有 8 个子包（a2a/agui/anthropic/copilot/foundry/gemini/openai/otel），但**它们都在同一个 module 里** —— 子包不能隔离依赖，只有 module 能。

scope 的对应约束（根 `CLAUDE.md`）：

> **Module 与 package 各司其职**：module 只表达独立依赖集合、发布周期和版本边界；package 表达职责。相同依赖与生命周期不为形式一致拆 module，**不同重型 SDK 也不硬塞进聚合 module**。

scope 的 `models/` 是 30 个叶子 module，用户只装用得上的。这是两者最大的工程差异，且 scope 这边明显更优 —— MAF 的单 module 没有任何设计上的必要性，纯粹是"还没拆"。

---

## 6. D7 可观测性

- `workflow/observability/opentelemetry/` —— 工作流侧
- `provider/otelprovider/` —— agent 侧，装饰型
- `workflow/telemetry.go`、`internal/otelx`、`internal/telemetry`

`workflow.Context` 有 `telemetry() *observability.Context` 和 `traceContextStrings() map[string]string`（跨 checkpoint 传播 trace context —— 这是个好细节：**恢复后的执行能接回原 trace**）。

scope 的 `otel/agent` adapter 只消费 Framework Event，不进 Kernel。MAF 的 workflow 内核里有 telemetry 类型。两种取向的老问题，scope 这边的 gate 更硬。

**值得吸收的细节**：`traceContextStrings` —— checkpoint 里保存 W3C trace context，恢复后继续同一条 trace。scope 的 `ARCHITECTURE.md §13` 目前只说「trace_id 在入口生成，脱钩的后台 goroutine 用 `context.WithoutCancel` 保住 span」，**没有说跨 snapshot/restore 的 trace 延续**。这是一个真实缺口。

---

## 7. D8 工程治理

- **public preview**，明确的功能差距表（`dotnet-go-sdk-feature-comparison.md`）—— 诚实，不假装完整。
- **.NET 是语义源** —— 与 adk-go 的 Python 同构，同样的负自由度。
- 依赖 `gofrs/flock`（文件锁）、`internal/hashmap` / `internal/maphash` / `internal/jsonx` / `internal/concurrent` —— **自建了不少基础设施**。

`internal/hashmap` 和 `internal/maphash` 是为了 `ScopeKey.Hash(h *maphash.Hash)` 这类值语义 map key。scope 的对应态度：「**通用能力不手写**：标准库优先；标准库缺失时允许使用经过评估的成熟三方库」。MAF 这几个 internal 包是标准库确实缺的（Go 的 `maphash` 直到 1.24 才有泛型 `Comparable`），可以理解。

---

## 8. 结论

### 同构（本次对比中最密集的一组）

| 取舍 | MAF | scope | 强度 |
|---|---|---|---|
| **middleware 用 `slices.Backward` 反向组合、nil 跳过、outermost-first** | ✅ | ✅ | ⭐ 逐行同构 |
| **`iter.Seq2` 做流式** | ✅ | ✅ | ⭐ 七项目中仅两家 |
| **多态 Content 建模，Reasoning 一等** | ✅ 20+ Content 类型 | ✅ `Part` tagged value | ⭐ |
| **accept interfaces, return structs（provider 只给一个函数）** | ✅ `RunFunc` | ✅ `ModelFunc`/`StreamerFunc` | ⭐ |
| **框架序列化，Store 只存字节** | ✅ | ✅ | ⭐ |
| **HITL 用类型化 port + 稳定 RequestID** | ✅ `RequestPort` | ✅ `WaitID` + Signal | ⭐ |
| **中间件必须视输入为只读、要改就 clone** | ✅ | ✅ | |
| 现代 Go 版本（1.26） | ✅ | ✅ 1.27 | |
| 观察与控制分层（ContextProvider vs Middleware） | ✅ | ✅（更严格：观察面无 error 返回） | |
| 显式功能差距表，不假装完整 | ✅ | ✅ ADR + API baseline | |

### 分歧

1. **单 module 捆绑 5 家 SDK + gRPC** —— scope: 叶子 module 依赖岛。**这是 MAF 最明确的弱点，且无设计必要性。**
2. **`TypeID` 依赖 Go 运行时内部布局（`typeid_reflect2_gc.go`）** —— scope: JSON Schema + `DeploymentRef`，可跨进程校验。
3. **`reflect.Type` 做协议契约** —— 不可序列化，跨进程恢复靠 `PortableValue` 补。
4. **Executor 有状态可复用（Init/Reset/Close）→ 需要运行期所有权令牌** —— scope: Definition 不可变，每次 Start 新 Execution，不变量在构造期成立。
5. **三态 `*bool` 配置 + 7 个 history 字段** —— scope: options struct + 有用零值 + 单一历史真相源。
6. **provider 专属概念进协议层**（`MCPServerToolCallContent` / `HostedFileContent` / `ToolApprovalRequestContent`）—— scope: 只有单一 provider 支持的 taxonomy 不进 Core。
7. **workflow 内核内嵌 telemetry** —— scope: `otel/*` 反向装饰 + Kernel gate 禁 OTel import。

### 真实缺口（scope 应认真对待）

| # | 能力 | 说明 | 建议 |
|---|---|---|---|
| 1 | **checkpoint 分支树**（`parent *CheckpointInfo` + `RetrieveIndex(withParent)`） | 从任一历史点分叉重跑；调试、A/B、时间旅行的基础 | 与 trpc 的 `ParentCheckpointID` 是同一需求，**两个独立项目都做了**。scope 的 `TreeSnapshot` 是 Host 持久化的值，分支语义本可完全归 Host（Host 存多份 snapshot 即可）。**先确认 Host 侧能否自足；不能则考虑在 capture 边界暴露 parent 身份** |
| 2 | **trace context 跨 checkpoint 延续**（`traceContextStrings`） | 恢复后的执行接回原 trace，否则一次长任务在 APM 里断成 N 条 | **确实缺口**。落点：`otel/agent` adapter 消费 restore 事件时重建 span link；或 Framework snapshot envelope 允许 Host 附带不透明 trace carrier。**倾向前者**（不污染 Kernel） |
| 3 | **`Message.Source`（消息知道自己是谁注入的）** | 区分模型产出 / middleware 注入 / 工具结果 | 小而实的可观测性改善。scope `core/chat.Message` 加一个 owner-validated `Source` 值对象是"参数化"形态，成本低 |
| 4 | **`ToolApprovalRequest/Response` 作为消息内容** | 审批状态随消息序列化、进历史、可回放 | **不吸收**。scope 的审批归装配边界（`tools/CLAUDE.md`）与 HITL Signal，进协议层会让所有 provider 背上只有部分 runtime 用的概念（违反 §2.5） |
| 5 | **预制编排（sequential/concurrent/groupchat）** | 降低起步成本 | **不吸收**。scope 用 `examples` + 行为断言证明组合能表达，避免"每个模式一个类型" |

### 判词

> **MAF Go 是与 scope 设计取向最接近的项目 —— 接近到 `middleware` 的组合算法逐行相同、流式原语相同、Content 建模相同、checkpoint 分层相同、HITL 的 port/WaitID 模式相同。**
> 它是本次对比中最有力的 convergent design 证据：**这些不是趣味，是被问题域逼出来的答案。**
> 它与 scope 的全部实质分歧，都能追溯到两个 scope 没有的约束：**.NET 语义对齐**（三态 bool、history 冲突七字段、provider 概念进协议）和**单 module 未拆**（5 家 SDK 捆绑）。
> 从它这里，scope 该拿走的是两条具体缺口：**trace 跨 restore 的延续**，和 **checkpoint 分支身份**。
