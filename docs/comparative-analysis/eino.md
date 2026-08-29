# eino（cloudwego）—— 类型化编排图

> 实测：`go 1.18`，202 个生产 `.go` 文件，59,032 行生产代码，44 处 `Deprecated:` 标记。
> 依赖：sonic（JSON）、gonja（Jinja 模板）、pyfmt、go-ordered-map、uuid、gomock。

---

## 0. 一句话定位

**eino 是"把 LangChain 的组合能力翻译成 Go 泛型"的最认真的一次尝试**，它的独特贡献不在 Agent，而在 `compose`：一个**编译期校验连线类型**的编排图，外加把"流 / 非流"从四个方向抹平的 `Runnable[I, O]`。它的 Agent 层（`adk`）是后来贴上去的，正在经历一次痛苦的语义换代。

---

## 1. 两层结构

```
adk/         Agent 层：ChatModelAgent / DeepAgent / Runner / interrupt-resume / sub-agents
  ↑
compose/     编排层：Graph / Chain / Workflow → Runnable[I,O]，checkpoint、state、branch
  ↑
components/  能力契约：ChatModel / Tool / Retriever / Indexer / Embedder / Loader / Prompt
  ↑
schema/      协议：Message / AgenticMessage / StreamReader / Document / ToolInfo
callbacks/   横切：全局 Handler，5 个 timing
```

这个分层与 scope 的 `core → 领域模块 → agent` 高度同构：**协议在最底、能力契约次之、编排在上、Agent 最上**。差异从第二层开始。

---

## 2. D1 协议层：正在换代的双消息模型

eino 目前**同时存在两套消息协议**：

```go
// schema/message.go —— OpenAI 风格：Role + Content + ToolCalls
type Message struct { Role RoleType; Content string; ToolCalls []ToolCall; ... }

// schema/agentic_message.go —— Anthropic 风格：ContentBlock 数组
type AgenticMessage struct { ... }
```

并用一个 **sealed 泛型 union** 把两者统一：

```go
type MessageType interface { *schema.Message | *schema.AgenticMessage }
type TypedAgent[M MessageType] interface { ... }
type Agent = TypedAgent[*schema.Message]
```

于是 `components/model` 里出现了：

```go
type BaseModel[M messageType] interface { Generate(...); Stream(...) }
type BaseChatModel  = BaseModel[*schema.Message]
type AgenticModel   = BaseModel[*schema.AgenticMessage]
```

**代价是全链路双份**：`TypedAgentEvent[M]`、`TypedMessageVariant[M]`、`TypedRunner[M]`、`AddChatModelNode` vs `AddAgenticModelNode`、`AddToolsNode` vs `AddAgenticToolsNode`、`AddChatTemplateNode` vs `AddAgenticChatTemplateNode`。文档里还得写「For `M = *schema.AgenticMessage`，cancel monitoring 和 retry 尚未接通」—— 两条路径的能力并不对等。

同时旧接口在原地弃用而不删：

```go
// Deprecated: Use [ToolCallingChatModel] instead.
type ChatModel interface { BaseChatModel; BindTools(...) error }  // BindTools 就地改实例，并发有竞态
```

### 与 scope 的对照

scope `core/chat` 从第一天就选了 **Part 化的 tagged value**：`Message` 持有 `[]Part`，`Part` 用公开 discriminator 分辨 text / image / tool_call / tool_result / reasoning，Reasoning 是 first-class 而不是后补。这正是 eino 现在要花两套类型迁移过去的形态。

scope 的对应红线（`core/CLAUDE.md`）：

- ❌ 用泛型 Model/StreamingModel 模拟继承
- ❌ 让 Model 强制 DefaultOptions/Metadata/Stream
- `Model` 只有 `Call`；`Streamer` 是独立能力

eino 的 `BaseModel[M]` 恰好是"用泛型模拟继承"的实例：它不是为了多态，而是为了让两套 wire 共享一个名字。**在有兼容承诺的项目里这是唯一出路；在 pre-1.0 项目里这是第一法则明令禁止的债。**

**判据**：eino 的双消息模型是一份很好的反面教材 —— 它证明了「协议形状选错，代价会以泛型爆炸的形式在整棵调用树上收利息」。

---

## 3. D2/D3 执行内核与编排：`Runnable[I,O]` 与四范式

eino 最有价值的设计在这里。

```go
type Runnable[I, O any] interface {
	Invoke(ctx, I, ...Option) (O, error)
	Stream(ctx, I, ...Option) (*schema.StreamReader[O], error)
	Collect(ctx, *schema.StreamReader[I], ...Option) (O, error)
	Transform(ctx, *schema.StreamReader[I], ...Option) (*schema.StreamReader[O], error)
}
```

四个方法 = 输入{值,流} × 输出{值,流} 的完整笛卡尔积。**用户只实现其中一个，框架自动补全另外三个**（流拼成值、值包成单元素流）。这是一个真正聪明的抽象：它把"这个组件支持流吗"这个问题从 API 层面彻底消灭了。

编排三态：

- `Chain[I,O]` —— 线性，最简；
- `Graph[I,O]` —— 有向图，两种运行模式（`pregel.go` 超步 BSP / `dag.go` 拓扑序）；
- `Workflow[I,O]` —— 图 + **字段级映射**（`AddInput(from, FieldMapping...)`、`SetStaticValue(FieldPath, value)`），可以把上游 struct 的某个字段接到下游 struct 的另一个字段。

节点类型是**枚举式的**：`AddChatModelNode` / `AddToolsNode` / `AddRetrieverNode` / `AddEmbeddingNode` / `AddIndexerNode` / `AddLoaderNode` / `AddDocumentTransformerNode` / `AddLambdaNode` / `AddGraphNode` / `AddPassthroughNode` / `AddBranch`。

### 代价：类型擦除 + 反射

编译期泛型只保住了**用户面**。内部立刻擦除：

```go
type invoke    func(ctx context.Context, input any, opts ...any) (output any, err error)
type transform func(ctx context.Context, input streamReader, opts ...any) (output streamReader, err error)

type composableRunnable struct {
	i invoke
	t transform
	inputType  reflect.Type   // ← 运行期靠 reflect.Type 做连线校验
	outputType reflect.Type
	optionType reflect.Type
	...
}
```

也就是说：**"编译期类型安全的图"其实是"构图期反射校验的图"**。`AddEdge` 时比对 `reflect.Type`，不匹配就在 `Compile` 时返错。这是 Go 泛型能力边界内的合理妥协（Go 没有异构类型列表），但它意味着 `compose` 包内部是一层厚厚的 `any` + `reflect` 机器。

### 与 scope 的对照

scope 在同一问题上走了**分层不同的路**：

| | eino | scope |
|---|---|---|
| 同进程确定编排 | `compose.Graph/Chain/Workflow` | 普通 Go 或独立 `flow` 库 |
| 需要独立身份/预算/恢复的编排 | 同一个 `Graph` + checkpoint | `agent/workflow` Strategy，每个分支是**真实 child Process** |
| 拓扑词汇 | 开放（任意 DAG + 按组件类型开 AddXxxNode） | 封闭（`Transform`/`Call`/`Switch`/`Fork`/`Map`/`Loop` 六个） |
| 类型安全 | 泛型门面 + 内部 reflect | Descriptor 持 JSON Schema，边缘 typed adapter 做 `I ↔ raw` |

scope 的架构文档明确写了这条分界（`ARCHITECTURE.md §6.3`）：

> 普通 Go/AI 同进程控制流可以直接写 Go，也可以选择独立 `flow`；每项工作需要独立 ProcessID、DeploymentRef、snapshot、预算、能力、取消和 tree recovery 时使用 managed Workflow。

**eino 没有这条分界** —— 一切都是 `Graph`。好处是心智模型只有一个；坏处是"我只想把两个函数串起来"和"我需要每个分支独立预算与恢复"付同样的复杂度税，而后者 eino 其实也给不了（见 D4）。

**值得吸收**：`Runnable` 四范式的"降级兼容"思想。scope 目前 `Model.Call` / `Streamer.Stream` 是两个独立能力，消费方要自己判断有没有流。eino 证明了「让框架补齐缺失方向」在使用体验上是巨大的胜利。scope 若要吸收，正确落点是 **`chatclient` 层的组合函数**（把 `Streamer` 拼成 `Model`、把 `Model` 包成单元素 `Streamer`），**不是**把它塞回 `core/chat.Model` 接口 —— 那会立刻违反"最小能力接口"。

---

## 4. D4 持久化与恢复：checkpoint + gob

```go
type checkpoint struct {
	Channels          map[string]channel
	Inputs            map[string]any
	State             any
	SkipPreHandler    map[string]bool
	RerunNodes        []string
	SubGraphs         map[string]*checkpoint
	InterruptID2Addr  map[string]Address
	InterruptID2State map[string]core.InterruptState
}
```

序列化用 **gob + 全局类型注册表**：

```go
schema.RegisterName[*checkpoint]("_eino_checkpoint")
schema.RegisterName[*dagChannel]("_eino_dag_channel")
// 用户自定义类型也必须注册：
func RegisterSerializableType[T any](name string) error  // Deprecated → schema.RegisterName
```

State 存在 **context 里**：

```go
type stateKey struct{}
type internalState struct { state any; mu sync.Mutex; parent *internalState }
```

节点通过 `StatePreHandler[I,S]` / `StatePostHandler[O,S]` 读写，框架加锁。

### 问题

1. **`State any` + gob 全局注册表**，等于把恢复正确性押在"用户记得注册每一个进入 state 的类型"上。忘记注册 = 运行期恢复失败，编译期无信号。
2. **state 藏在 context**：违反"context 只传取消、deadline 和请求范围值"。它使得"哪些状态属于这次执行"无法从类型上读出，只能从运行期 `getState[S](ctx)` 的成功与否发现。
3. **没有 Process 概念**。checkpoint 是"图的执行位置"，不是"一个有身份、有预算、有父子关系、有终态的运行实例"。因此 eino 无法表达：子任务独立预算、能力衰减、树级取消、父终态传播、孤儿清理。
4. `StreamReader` 本质是 channel + goroutine，**必须手动 `Close()`**，文档反复警告不 Close 会泄漏整条 pipeline 的 goroutine。callbacks 的 stream handler 每个都要 Close 自己那份 copy。

### 与 scope 的对照

scope 的对应设计（`ARCHITECTURE.md §6.5 / §12`）：

```go
type ExecutionState struct {
	Kind          string
	SchemaVersion uint16
	Payload       json.RawMessage   // ← 严格 JSON，owner 自持 schema，无全局注册表
}
```

- **无全局 registry**：恢复靠精确 `DeploymentRef` 找回 Definition，架构文档明写「禁止全局 `kind → factory` 巨型 switch」。
- **prepare/finalize 两阶段提交**：Step 只产候选状态 + Effect 意图，Engine 记录 prepared step → 调度 Effect → 原子 finalize。eino 没有这个边界，interrupt 发生在节点之间，节点内部的副作用一旦发出就无迹可寻。
- **TreeSnapshot**：一旦形成 Process tree，恢复单位必须是完整树，且要校验预算总和、能力衰减、父子关系。eino 的 `SubGraphs map[string]*checkpoint` 是嵌套的图位置，不承载这些不变量。
- **流用 `iter.Seq2`**：调用方停止迭代时实现必须同步释放资源，不留 goroutine。没有"忘记 Close 就泄漏"这个失败模式。

**判据**：eino 的 checkpoint 是"图执行位置的快照"，scope 的 snapshot 是"进程树一致状态的快照"。两者不是同一件事的两个实现，是两个不同的问题。eino 解决的是"长图跑一半崩了能接着跑"，scope 解决的是"一棵有预算、有权限、有父子等待关系的进程树能被搬到另一个进程里继续"。

---

## 5. D5 扩展机制：全局 callbacks

```go
type Handler = callbacks.Handler  // OnStart / OnEnd / OnError / OnStartWithStreamInput / OnEndWithStreamOutput
func AppendGlobalHandlers(...)    // 全局
func InitCallbacks(...)           // 注入 ctx
```

Handler 靠 `RunInfo{Name, Type, Component}` 自己过滤关心的事件。文档明确写：**handler 之间没有顺序保证，ctx 不在 handler 之间传递**。

三个问题：

1. **全局注册表**。`AppendGlobalHandlers` 让"这段代码会不会被观测"依赖进程全局状态，与 scope 的"不使用 package-global registry"直接冲突。
2. **靠 RunInfo 字符串过滤**。`Component: "ChatModel"` 是 `components.Component`（string 别名）。观测方要判断"这是不是我关心的东西"必须匹配字符串常量，而不是类型。
3. **CallbackInput/Output 是 `any`**，各组件包自带 `ConvCallbackInput` 做 type assert，不匹配返回 nil。文档还得警告「不要修改 Input/Output —— 所有下游节点和 handler 共享同一指针」。

### 与 scope 的对照

scope 的对应红线：

- ❌ 新增全局 registry/cache/state（`core/CLAUDE.md`）
- ❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链（`core/CLAUDE.md`）
- `EventListener`/`DeltaListener` 是**无错误返回的观察接口**，返回不可变 typed snapshot，panic 被隔离并以饱和计数暴露（`ARCHITECTURE.md §13`）

scope 用**两个正交机制**覆盖 eino 一个 callbacks 的职责：
- 想改变行为 → `chat.CallMiddleware func(next Model) Model`（一个同质机制）
- 想观察事实 → Framework Event / Delta listener，或直接在 `otel/*` decorator 里用官方 OTel API

eino 把两者混在一个 Handler 里（`OnStart` 返回 ctx，可以携带状态影响后续），这正是 scope 的「Signal 进入 Execution，Transition 表达意图，Event 记录事实；三者不得互相冒充」要防的模糊地带。

---

## 6. D6 模块边界

**单 module**。`components/*`、`compose`、`adk`、`schema`、`callbacks` 全部在 `github.com/cloudwego/eino` 里。所有 provider 实现在**另一个仓库** `eino-ext`。

`go.mod` 声明 `go 1.18` —— 但代码里用了泛型 sealed union（1.18 可以）、没用 `iter.Seq2`（1.23+）、没用 `slices`/`maps`（1.21+）。这是一个**为了最大兼容性锁死语言版本**的选择，代价是整个框架不能用近三年的 stdlib。

对比 scope：85 个 module，`go 1.27`，每个 provider 一个叶子 module，可独立选择/发布/升级。scope 的 `models/CLAUDE.md` 明写「`models` 只是命名空间：不存在聚合 `models` module」。

**这个差异是根本性的**：eino 用户装一个 `eino` 就带上 sonic、gonja、pyfmt、ordered-map；scope 用户只装自己用的那几个 provider module。

---

## 7. D7 可观测性

eino **不 import OTel**。观测靠 callbacks，用户自己在 Handler 里打点。`eino-ext` 里有 langfuse/apmplus 之类的 callback 实现。

scope 的路线（`otel/CLAUDE.md`）：领域模块不 import OTel，由独立 `otel` module 反向依赖并 decorator 装饰，wrapper 内**直接用官方 OTel API**，不自造 tracer/meter 抽象。

两者的共同点是**领域代码不 import 遥测**。差异在于：eino 提供的是自造的 callback 协议（观测方要学 eino 的 RunInfo/CallbackInput 词汇），scope 提供的是标准 OTel（观测方什么都不用学）。scope 这一侧更接近 vendor-neutral 的本意。

---

## 8. D8 工程治理

- 44 处 `Deprecated:`，且**弃用后不删**：`ChatModel.BindTools`、`InitCallbackHandlers`、`RegisterSerializableType`…
- 更严重的是**"NOT RECOMMENDED" 而不弃用**：`TransferToAgentAction`、`NewTransferToAgentAction`、`NewExitAction`、`RunStep`、`AgentEvent.RunPath`、`OnSubAgents` 全部挂着：

  > NOT RECOMMENDED: Agent transfer with full context sharing between agents has not proven to be more effective empirically. Consider using ChatModelAgent with AgentTool or DeepAgent instead.

  这是一个**诚实但危险**的状态：核心多 Agent 机制（transfer）被作者自己判定为无效，却因为兼容承诺留在公开 API 里，还得继续维护它在 checkpoint（`RunStep` 有 `GobEncode`/`GobDecode`）、事件（`RunPath`）、agent tool 边界（action scoping）里的全部语义。

### 与 scope 的对照

这是 scope **第一法则**（绝不为一时方便留历史债务）最直接的外部证据。scope 的对应处理是：

> ❌ **绝不为"少改几处 / 降低前期开发量 / 避免迁移 / 赶进度"留下任何历史债务**
> 已删除的旧 package、alias、bridge 和 generic framework 不得重新引入。

scope 的 agent module 就是这条法则的实践：整个原 GOAP 框架被删除重写，只在 git 历史里作为证据保留，`ARCHITECTURE.md §1.1` 明写「原实现只保留为历史证据，不是兼容规范」。

**如果 scope 走 eino 的路线**：今天的 `Interaction` / `Planning` / `Workflow` 三种 Strategy 里，一旦哪个被证明无效，就会变成 API 里一个永久的 "NOT RECOMMENDED" 疤痕，并且它的 snapshot payload 还得永远解码。

---

## 9. 结论

### 同构（独立收敛到同一取舍）

| 取舍 | eino | scope |
|---|---|---|
| 协议在最底层，能力契约与编排分层 | ✅ schema → components → compose | ✅ core → 领域模块 → agent |
| 领域代码不 import 遥测 | ✅ | ✅ |
| 编排拓扑在构造期冻结、编译/构图期校验 | ✅ Compile 校验 reflect.Type | ✅ Descriptor schema + 构造期校验 |
| 接口尽量小、能力可选（BaseTool/InvokableTool/StreamableTool） | ✅ | ✅ Indexer/Searcher/IDDeleter/FilterDeleter |
| 拒绝为 provider 加 retry layer | ✅（无 retry 层，adk 里有 retry_chatmodel 是显式 wrapper） | ✅ 明确反向不变量 |

### 分歧（scope 明确不走的路）

1. **双消息协议并存** —— scope: 一个能力一个出口，`Part` 从第一天就是 tagged value。
2. **State 藏 context + `any` + gob 全局注册表** —— scope: `ExecutionState{Kind, SchemaVersion, json.RawMessage}`，owner 自持 schema。
3. **全局 callbacks registry + 字符串过滤** —— scope: 无全局 registry，middleware 改行为、Event 记事实，两者不混。
4. **单 module 捆绑全部依赖** —— scope: module = 依赖岛 + 发布边界。
5. **Deprecated / NOT RECOMMENDED 原地保留** —— scope: 咨询后直接删。
6. **只有一种编排（Graph），不区分同进程组合与 managed Process** —— scope: `flow`（普通）/ `workflow`（managed child Process）两条边界。

### 真实缺口（eino 有、scope 值得看的）

1. **`Runnable` 四范式的降级补全** —— 使用体验上的真实胜利。scope 的正确落点是 `chatclient` 层的组合函数，不是 `core/chat.Model`。
2. **`Workflow` 的字段级映射（`FieldMapping` / `FieldPath` / `SetStaticValue`）** —— scope 的 Workflow Stage 目前是"整个 value 进、整个 value 出 + `Transform` 纯函数"。eino 允许在拓扑层声明 `上游.FieldA → 下游.FieldB`，省掉大量胶水 Transform。**但**这会把 schema 知识带进拓扑层，与 scope「相邻 schema 在构造时精确衔接、Transform 是有界确定纯函数」的边界冲突 —— 收益不足以打破边界，记账不做。
3. **`ext` / `eino-ext` 的分仓策略** —— provider 完全另仓，主仓依赖极干净。scope 用 85 个叶子 module 达到了同样效果且粒度更细，无需吸收。

### 判词

> **eino 在"Go 泛型能表达多好的编排图"这个问题上给出了目前最完整的答案，也同时给出了"协议选错的复利成本"最完整的教材。**
> 它的 `compose` 值得研究；它的 `adk` 是一个仍在寻找形状的层（transfer 被自己判死、双消息模型未收口）；它的治理方式（弃用不删、NOT RECOMMENDED 常驻）是 scope 第一法则最有力的外部反证。
