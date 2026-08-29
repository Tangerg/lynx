# trpc-agent-go（Tencent）—— 电池全包的一体化 runtime

> 实测：`go 1.21`，1,765 个生产 `.go` 文件（已排除内置应用 `openclaw`），**492,436 行生产代码**，30+ 个 `go.mod`。
> 依赖：openai-go、OTel 全家桶（含 5 个 exporter）、zap、grpc、protobuf、ants 协程池、sqlite3、gse 中文分词、godocx/docxlib（Word 解析）、goldmark、cos-sdk、jsonschema…

---

## 0. 一句话定位

**trpc-agent-go 是这七个项目里唯一一个"框架 + 全部电池 + 一个成品应用"打包在同一个仓库里的**。它的野心不是"提供 Agent 抽象"，而是"你需要的一切都在这里，包括一个能直接跑的多渠道 Agent 网关"。它是 scope 的**规模镜像**：同一个领域，走完全相反的边界策略。

体量对照：

| | trpc-agent-go | scope |
|---|---|---|
| 生产代码 | 492,436 行 | 97,808 行 |
| 生产文件 | 1,765 | 756 |
| 单文件最大 | `agent/invocation.go` **2,428 行** | — |
| 单包最大 | `graph/` 25,385 行 | `agent/` 22,743 行（含 4 个子包） |
| module 数 | 30+ | 85 |
| 内含成品应用 | ✅ `openclaw/`（含浏览器扩展、cron、渠道网关、admin） | ❌ 应用在独立 Flame 仓库 |

---

## 1. 全景

```
agent/       Agent 接口 + 8 种内置 Agent（chain/cycle/graph/llm/a2a/dify/codex/claudecode）
runner/      运行器 + bestofn + ralph_loop + plugin + candidate selector
graph/       LangGraph 移植：State/StateSchema/Reducer/Checkpoint/时间旅行  (25K 行)
model/       8 家 provider + failover + hedge + registry
tool/        30+ 内置工具（含 arxiv/duckduckgo/google/email/openapi/mcp/codeexec/hostexec）
session/     9 个后端（inmemory/redis/mysql/postgres/mongodb/sqlite/clickhouse/pgvector/noop）
memory/      11 个后端 + extractor + mem0
knowledge/   完整 RAG：chunking/embedder/reranker/vectorstore/graphstore/ocr/searchfilter
artifact/    inmemory/s3/cos
codeexecutor/ local/container/e2b/jupyter/sandbox/codeact
planner/     react / builtin / a2ui
evaluation/  evalset/evaluator/metric/score/usersimulation/workflow（独立 module）
evolution/   Agent 自进化：session review → 提炼 SKILL.md → gate → publish
skill/       Agent Skills 仓库
team/        swarm
telemetry/   OTel + langfuse + semconv
server/      a2a / agui / openai-compatible / trpcagent / evaluation
plugin/      guardrail/identity/toolsearch/messagemerger/debuglog/…
openclaw/    ⬅ 一个完整的产品（独立 module，含 browser-extension、gwproto、admin、cron）
```

---

## 2. D1 协议层

`model.Message` 是 OpenAI 形状的中立结构（Role + Content + ToolCalls + ContentParts）。有 `message_validator.go`、`message_compare.go`、`file_downloader.go`。provider 层有 `failover`（按序试）和 `hedge`（并发对冲）两种复合 Model —— 这是 scope 没有的能力，且是**真实生产需求**。

⚠️ scope 的反向不变量是「❌ 加 retry layer」，理由是 SDK 内部已有 retry。但 **failover（换 provider）与 hedge（并发对冲降尾延迟）不是 retry** —— 它们是跨 provider 的可用性/延迟策略，SDK 内部的 retry 覆盖不到。这是一个 scope 值得重新审视的边界（见 §9 缺口）。

---

## 3. D2 执行内核：god struct `Invocation`

```go
type Agent interface {
	Run(ctx context.Context, invocation *Invocation) (<-chan *event.Event, error)
	Tools() []tool.Tool
	Info() Info
	SubAgents() []Agent
	FindSubAgent(name string) Agent
}
```

先看接口本身：`Run` + `Tools` + `Info` + `SubAgents` + `FindSubAgent` 五个方法。**后三个是编排关注点，不是执行关注点** —— 任何自定义 Agent 都被迫实现 `SubAgents()`/`FindSubAgent()`，哪怕它根本没有子 Agent。这是典型的胖接口，违反 ISP。

真正的问题在 `*Invocation`（`agent/invocation.go`，**2,428 行**）：

```go
type Invocation struct {
	Agent            Agent
	AgentName        string
	InvocationID     string
	Branch           string
	ParentMetadata   *ParentInvocationMetadata
	EndInvocation    bool
	Session          *session.Session
	SessionService   session.Service      // ← 服务
	Model            model.Model          // ← 服务
	Message          model.Message
	RunOptions       RunOptions
	TransferInfo     *TransferInfo
	Plugins          PluginManager        // ← 服务
	StructuredOutput *model.StructuredOutput
	StructuredOutputType reflect.Type     // ← 运行期反射类型
	MemoryService    memory.Service       // ← 服务
	MemoryReader     memory.Reader
	ArtifactService  artifact.Service     // ← 服务
	...
}
```

`RunOptions` 里还有 30+ 个字段：`MaxLLMCalls`、`MaxToolIterations`、`RuntimeState map[string]any`、`SkillLoads`、`KnowledgeFilter map[string]any`、`InjectedContextMessages`、`LateContextMessages`、`UserMessageRewriter`、`Resume`、`PersistInterruptedAssistant *bool`、`GraphEmitFinalModelResponses`、`GraphTerminalMessagesOnly`、`DisableGraphCompletionEvent`、`DisableGraphExecutorEvents`、`StreamModes`、`DisableTracing`…

注意 `GraphEmitFinalModelResponses` / `GraphTerminalMessagesOnly` / `DisableGraphExecutorEvents`：**Graph 编排器的专属开关被放进了所有 Agent 共享的 RunOptions**。这正是 scope `DESIGN_PHILOSOPHY.md §2.5` 定义的反模式：

> **反模式**：为"共享"或"看着该归属基类型"，把消费方特有 / 具体的东西往下塞进最底层。后果：底层变胖、被单个消费方的关注点污染，还逼**所有其他**消费者背上用不到的概念。

### 与 scope 的对照

scope 的 `ARCHITECTURE.md §6.1` 直接写了这条：

> 如果子 Process 能力需要注入 Execution，应由真实消费包定义最小接口，**不能把完整 `*Engine` 或不断膨胀的 `ExecutionContext` 传给所有策略**。

scope 的窄腰是两个接口共 4 个方法：

```go
type Definition interface {
	Descriptor() Descriptor
	Start(Input) (Execution, error)
	Restore(ExecutionState) (Execution, error)
}
type Execution interface {
	Step(context.Context, []Signal) (Transition, error)
	Snapshot() (ExecutionState, error)
}
```

Session、Model、Memory、Artifact 这些**在 scope 里全部属于 Host Application**（`ARCHITECTURE.md §5.3`）。Framework 只知道 ProcessID、DeploymentRef、Budget、CapabilitySet、Status、Signal、Effect。

`*Invocation` 是"把宿主的一切塞进框架合同"的极致形态。它的直接后果：

- **无法可靠 snapshot**。里面有 `Agent`（接口值）、`model.Model`（含 HTTP client）、`session.Service`、`reflect.Type`。这些都不可序列化。trpc 的 graph checkpoint 因此只能快照 `graph.State`，不能快照"整次调用"。
- **无法做能力衰减**。子 Agent 拿到的 Invocation 是父的克隆（`context_cloner.go`、`runner_clone_test.go`），能力=父的全部。scope 的「子能力只能是父能力的子集，不能递归提权」在这个结构里没有落点。
- **测试要构造整个世界**。

---

## 4. D3 编排：`State map[string]any`（LangGraph 移植）

```go
type State map[string]any
type StateReducer func(existing, update any) any
type StateField struct { ... }
type StateSchema struct { ... }

func GetStateValue[T any](s State, key string) (T, bool)
```

外加一堆字符串 key 常量：

```go
StateKeyModelCallbacks   = "model_callbacks"
StateKeyAgentCallbacks   = "agent_callbacks"
StateKeyCurrentNodeID    = "current_node_id"
StateKeyParentAgent      = "parent_agent"   // ← 把 Agent 实例塞进 State
const currentTraceStepIDStateKey = "__current_trace_step_id__"
```

checkpoint 里有魔法 channel 名：

```go
InterruptChannel = "__interrupt__"
ResumeChannel    = "__resume__"
ErrorChannel     = "__error__"
ScheduledChannel = "__scheduled__"
```

`Checkpoint` 结构本身设计不差（`ChannelValues`/`ChannelVersions`/`VersionsSeen`/`PendingSends`/`BarrierSets`/`ParentCheckpointID` —— 完整的 Pregel 语义 + 分支），但载荷全是 `map[string]any`。

### 与 scope 的对照

scope 的红线（根 `CLAUDE.md`）：

> **禁止魔法**：稳定词汇使用有语义的 named string value object 或常量并由 owner 校验；协议数值、默认值、超时、版本与 attribute key 必须具名。**匿名 `map[string]any` 只允许停在真实动态 wire / 第三方 SDK 边界，进入领域后立即转成 typed model，不能作为内部配置、领域状态或跨层参数袋。**

`graph.State` 就是"跨层参数袋"的教科书实例：节点 A 写 `state["x"]`，节点 B 读 `GetStateValue[T](state, "x")`，**契约只存在于两处字符串常量必须相等**。改一个 key、改一个类型，编译器完全沉默。

scope 的 Workflow 对应设计（`ARCHITECTURE.md §6.3`）：

> 一个 Stage 消费当前 immutable、schema-validated value 并产生下一 value；**相邻 schema 在构造时精确衔接**。ExecutionState 只保存 Stage/value/case/window/item/iteration 游标和 child/wait/result 身份，**不保存函数、Deployment concrete value、Engine、goroutine、Store/Journal 或 Host 数据**。

以及封闭词汇：`Transform` / `Call` / `Switch` / `Fork` / `Map` / `Loop` 六个，不做任意 DAG。

**判断**：`State map[string]any` 是 LangGraph 在 Python 里的自然形态（Python 本来就是 dict 世界）。移植到 Go 时**没有付出把它类型化的代价**，于是 Go 的最大优势（编译期契约）在整个编排层被放弃。这是"照抄别语言的架构"最典型的代价。

---

## 5. D4 持久化：三套并存的状态概念

trpc 里"状态"至少有三个不相交的载体：

1. `session.Session` —— 产品会话，9 个后端持久化，含 events、state、summary
2. `graph.Checkpoint` —— 图执行位置，`ChannelValues map[string]any`
3. `memory.Service` —— 长期记忆，11 个后端

再加 `artifact.Service`（文件）、`skill` 仓库、`evolution` 的 revision store。

**没有一个统一的"运行实例"概念**。`InvocationID` 是个 string，不是有生命周期、有终态、有预算、有父子关系的实体。因此：

- 谁负责在崩溃后清理孤儿 —— 未定义
- 子任务的预算从哪来 —— `RunOptions.MaxLLMCalls` 是每次调用各自的数，不是从父划拨
- 树级取消 —— 靠 context 传播，`cancel_recursive_test.go` 之类在 eino 有，trpc 靠 channel + ctx

### 与 scope 的对照

scope 把这些**收敛到一个概念**：Process。

```
ProcessID / RootProcessID / ParentProcessID
DeploymentRef
Status + terminal cause + 当前等待条件
通用 usage/budget
pending Signal 信封 + 到达序 + 消费游标
WaitID ↔ Strategy logical wait key 映射
prepared step envelope（含 pending Effect + EffectID + 逐项 settlement）
Execution state envelope
```

以及一条被明确写进架构的责任线（`§5.3`）：Session / Conversation / Turn / Store / 事务 / 计费 / 审计 **全部归 Host**。

**这是两个项目最根本的分歧**：trpc 认为"框架应该拥有会话和记忆"，scope 认为"框架只拥有执行生命周期，会话和记忆是产品身份"。

trpc 的选择带来即战力（装上就有 9 种 session 后端），代价是框架被产品概念绑死 —— 当你的产品的"会话"不是它的 `Session` 形状时，你要么迁就，要么绕过整个 session 层。

---

## 6. D5 扩展机制：四套并存

| 机制 | 位置 | 形态 |
|---|---|---|
| `callbacks` | `agent/callbacks.go`、`model/callbacks.go`、`tool/callbacks.go`、`graph/callbacks.go` | Before/After 各一份，**四个包各有一套** |
| `plugin` | `plugin/` | guardrail / identity / toolsearch / messagemerger / toolerror / debuglog… |
| `hook` | `session/hook.go` | session 专用 |
| `middleware` | `runner/runner_middleware_test.go` | runner 层 |

四个扩展机制，语义重叠。scope 的 `DESIGN_PHILOSOPHY.md §2.3`：

> 优先"**一个**同质机制 + 类型 / 中间件分发"，而不是"为每种扩展开一个具名插槽"。插槽多 = 表面积大 + 心智负担。

以及 `core/CLAUDE.md`：❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链。

---

## 7. D6 模块边界：30+ module，但边界不是依赖岛

trpc 的多 module 拆分逻辑是"可选后端各自一个"（`memory/redis`、`memory/pgvector`、`agent/dify`、`server/agui`…），这与 scope 的叶子 module 策略同构，✅。

但根 module 本身极重：openai-go + OTel 全家桶（含 5 个 exporter，其中 grpc exporter 拉进整个 gRPC + protobuf）+ zap + ants + sqlite3(cgo!) + gse 中文分词 + godocx/docxlib + goldmark + cos-sdk。

**任何 import `trpc-agent-go` 的程序都背上这一整套**，包括 cgo 的 sqlite3 和一个中文分词词典。

对照 scope：`core` 的约束是「标准库优先；Core 不 import sibling module、provider SDK、具体 tokenizer 词表或 OTel」，且有 architecture gate 机器验证。ETL 的 HTML/PDF 重解析器单独成叶子 module，`tokenizers/tiktoken` 单独一个 module。

### `openclaw/` —— 框架仓库里的成品应用

`openclaw/` 是一个完整产品：`browser-extension/`、`gwproto/`（gRPC 协议）、`admin/`、`channel/`、`conversation/`、`croncmd/`、`delivery/`、`registry/`、`plugins/`，带 `install.sh`、`Dockerfile`、`RELEASE.md`。

scope 刚刚做了**相反方向**的动作 —— 最近一次 commit 就是 `f006f7237 refactor: remove migrated application`，根 `CLAUDE.md` 现在写着：

> 完整应用由独立 Flame 仓库拥有；本仓库不再包含 `app` 模块，也不把应用层纳入库架构分层。

且 `agent/CLAUDE.md` 有硬约束：**do not make the Framework depend on a Host application**，并有 architecture gate 永久禁止 Host application 依赖回流。

**这是一个刚刚做过的、方向明确的裁决。** trpc 的做法给出了不这么做的后果：`agent/claudecode`、`agent/codex`、`agent/dify`、`tool/claudecode`、`tool/okf`、`tool/openviking` —— 具体产品集成渗进了框架的核心目录。

---

## 8. D7/D8 可观测性与治理

### 可观测性 ✅ 最强项

- OTel trace + metric 完整，`telemetry/semconv` 跟 GenAI semconv
- `telemetry/langfuse` 专门集成
- `telemetry/tracetransform`、`appid`、`errs` 分层
- `evaluation/` 是独立 module：evalset / evaluator / metric / score / **usersimulation** / workflow

`usersimulation`（模拟用户做多轮评测）是 scope 的 `evaluation` module 目前没有的能力，且是真实需求。

### 自进化 `evolution/` —— 独一无二

```
submission → reviewer → gates → redaction → promotion → publisher → revision
+ approval_service / auto_expire / reconcile / optimization
```

把 session 复盘提炼成可复用的 `SKILL.md`，经过闸门、脱敏、审批后发布。这是七个项目里**唯一**把"Agent 从自己的运行历史里学习"做成一等公民的。它跨越了框架与产品的边界（需要审批流、需要 revision store），但作为能力设计值得研究。

### 治理

`AGENTS.md` 的第一条工程原则：

> **Preserve syntax, semantic, behavioral, serialization, persistence, and protocol compatibility unless the task explicitly requires a documented change.**
> New behavior should preserve existing defaults and should be opt-in when practical.

这是与 scope **第一法则完全相反的极点**。scope：

> ✅ **发现设计不对，就在源头改对**，不在错的设计上叠补丁。**现在改成本最低，往后只会更贵。**

trpc 的原则在有真实外部用户的项目里是正确的；它的代价可以直接从 `RunOptions` 的 30+ 个 `Disable*` / `Enable*` 布尔开关里读出来 —— 每一个都是"某次行为变更必须 opt-in"留下的化石。

---

## 9. 结论

### 同构

| 取舍 | trpc | scope |
|---|---|---|
| 可选后端各自一个 module | ✅ | ✅ |
| OTel 官方 API，不自造抽象 | ✅ | ✅ |
| 评估独立成 module | ✅ | ✅ |
| Agent Skills 作为一等能力 | ✅ `skill/` | ✅ `skills/` + `tools/skills` |
| Tool 层与 backend 分离 | 部分（`tool/*` 直接实现） | ✅ 两层 SPI 明确 |

### 分歧（scope 明确不走的路）

1. **god struct `Invocation`（2,428 行 / 40+ 字段 / 含服务与 reflect.Type）** —— scope: 4 方法窄腰 + Host 拥有 Session/Model/Memory/Artifact。
2. **`State map[string]any` + 字符串 key 契约 + `__magic__` channel** —— scope: schema-validated typed value + 封闭 Stage 词汇。
3. **框架仓库内含成品应用** —— scope 刚刚删掉了 `app`，并用 architecture gate 锁死。
4. **四套并存的扩展机制** —— scope: 一个 middleware + 观察型 listener。
5. **框架拥有 Session/Memory** —— scope: 那是 Host 的产品身份。
6. **"保持兼容除非明确要求变更"** —— scope: 咨询后直接改对。
7. **根 module 依赖极重（含 cgo sqlite3 + 中文分词词典）** —— scope: Core 只用标准库 + 经评估的通用库，有 gate。

### 真实缺口（scope 应认真对待）

| # | 能力 | 说明 | 建议 |
|---|---|---|---|
| 1 | **Model failover / hedge** | 换 provider 容错、并发对冲降尾延迟。**不是 retry**，SDK 内部 retry 覆盖不到 | scope 的正确落点是 `chatclient` 的 `CallMiddleware`（组合多个 `chat.Model`），完全符合"参数化/组合/装饰"三形态的第 ② 形态。值得做 |
| 2 | **用户模拟评测（usersimulation）** | 多轮对话质量评测必须模拟用户 | `evaluation` module 的自然扩展，与现有 judge/metric 同层 |
| 3 | **代码执行沙箱（codeexecutor）** | local/container/e2b/jupyter 多后端 | `tools` module 的两层 SPI 天然容得下：Tool 层 + Backend Port（local/container/远程）。scope 目前只有 `tools/shell` |
| 4 | **Agent 自进化（evolution）** | session 复盘 → 提炼 SKILL.md → 闸门 → 发布 | 跨框架/产品边界。scope 若做，`skills` module 只该拥有"读"，"写与审批"归 Host。**记账，暂不做** |
| 5 | **时间旅行 / checkpoint 分支** | `ParentCheckpointID` 支持从任一历史点分叉重跑 | scope 的 `TreeSnapshot` 已有 capture/restore，但没有"从历史 checkpoint 分叉"的语义。**这是真实的调试与 A/B 需求，值得评估** |

### 判词

> **trpc-agent-go 证明了"什么都给你"在起步速度上无可匹敌，也证明了它的代价会以 god struct、字符串状态袋和 30 个 Disable 开关的形式全额支付。**
> 它是 scope 每一条反向不变量的**活体对照组** —— 几乎每一条禁令都能在这个仓库里找到它想避免的具体形态。
> 但它在**可观测性、评估、代码执行沙箱、模型容错**四个方向上确实跑在前面，这四项应当被认真评估，且都能在 scope 现有边界内以"组合"形态落地，不需要新造机器。
