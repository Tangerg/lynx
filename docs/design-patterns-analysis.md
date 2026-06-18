# lynx 仓库设计模式调研分析

> 分析范围：13 个代码模块（排除 `pkg/`）
> 分析时间：2026-06-17
> 分析原则：基于 [`DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md) 的能力三形态（参数化 / 组合 / 装饰）和薄核哲学

---

## 0. 执行摘要

lynx 仓库**高度模式化**，13 个模块中广泛使用了 Gang of Four（GoF）经典设计模式和 Go 社区惯用模式。最核心的架构模式是：

1. **薄核 + 多后端适配器** —— 每个领域（LLM 调用、向量存储、聊天记忆…）都是一个稳定接口 + N 个后端实现
2. **管道-过滤器** —— RAG 的 5 阶段流水线 + document pipeline
3. **策略模式 + 插件** —— agent 的 Planner 族 + Extension 类型分发系统
4. **访问者模式** —— vectorstores 的 filter AST → 方言编译器

**可优化领域**：主要集中在 `models/`（Provider 注册发现）、`chatmemory/`（CQRS 读写分离）、`tools/`（命令模式 Undo/Redo）、`core/`（Message Visitor 消除散落 type switch）四个方面。

---

## 1. 跨模块 / 架构级设计模式

### 1.1 薄核 + 适配器（Thin Core + Adapter）

**贯穿仓库的组织原则**。每个领域模块遵循：

```
core/  → 定义稳定接口（协议层）
models/ | vectorstores/ | chatmemory/ | documentreaders/  → 具体实现（适配层）
```

**已使用处**：

| 核心接口（core/ 定义） | 适配器模块 | 实现数量 |
|---|---|---|
| `core/model/chat.Model` (`CallHandler`) | `models/` | 38 个 provider |
| `core/vectorstore.Store` | `vectorstores/` | 27 个后端 |
| `core/model/chat/memory.Store` | `chatmemory/` | 6 个后端 |
| `core/document.Reader` | `documentreaders/` | 3 种格式 |
| `core/model/CallHandler` / `StreamHandler` | `models/` | 38 个 provider |

**设计哲学**：对应 `DESIGN_PHILOSOPHY.md` §1 的形态 ②（组合）：把能力归约到核协议，外圈负责映射。

---

### 1.2 消费方定义接口（ISP — 接口隔离原则）

仓库最显著的架构纪律：**接口在所有消费方包内定义，被消费方不主动 export "给你用的接口"**。

**已使用处**：

| 消费方 | 接口名 | 被消费方 |
|---|---|---|
| `a2a/` | `Agent` (`Run(ctx, input) iter.Seq2`) | `lyra/internal/kernel/` |
| `mcp/` | `chat.Tool` (来自 `core/model/chat`) | `tools/*` |
| `skills/` | `Source` (三类方法) | `tools/skills/` |
| `lyra/internal/kernel/turn/` | `engineDep` 窄接口 | `lyra/internal/kernel.Engine` |
| `lyra/internal/domain/tool/` | 自定义 source 窄接口 | 不 import kernel |

**好处**：模块只依赖它真用的那部分方法，替换 / 测试 / mock 成本低。完全符合 Go 社区 "accept interfaces, return structs" 原则。

---

### 1.3 插件 / 扩展点机制（Plugin / Extension Point）

`agent/core/Extension` 是整个 agent runtime 的单一扩展机制。不是为每种能力开一个具名插槽，而是用一个 marker interface + 7 个可选能力子接口 + `collectExtensions[T]` 泛型类型分发。

**已使用处**：
- `agent/core/Extension` —— marker interface
- 7 个能力子接口：`ActionMiddleware` / `ToolDecorator` / `AgentValidator` / `GoalApprover` / `ToolGroupResolver` / `IDGenerator` / Blackboard & Planner factory
- `agent/runtime/` 中的 `collectExtensions[T any](extensions []Extension)` 类型分发

**对应哲学**：`DESIGN_PHILOSOPHY.md` §2.3 "一个扩展机制优于一堆 hook / SPI"。

---

### 1.4 管道-过滤器（Pipes and Filters）

**已使用处**：

1. **`rag/pipeline.go`** —— 5 阶段顺序流水线：
   ```
   QueryTransformer → QueryExpander → DocumentRetriever(并行) → DocumentRefiner → QueryAugmenter
   ```
   - 每个阶段一个接口（`stages.go`）
   - 只有 Retrieve 阶段 **并行** fan-out（`sync.WaitGroup`，部分失败容忍）
   - `pipeline_middleware.go` 把整条 pipeline 包装成 chat middleware

2. **`core/document/`** —— 文档处理流水线：
   ```
   Reader → Transformer → Batcher → Writer → Formatter
   ```
   每个阶段一个接口，可组合

---

### 1.5 事件驱动松耦合（Observer / Event-Driven）

**已使用处**：

- `agent/event/` —— 生命周期事件 + 多播 listener：`AgentProcess` 状态变化 → 事件 → 所有订阅 listener
- `lyra/internal/kernel/turn/` —— observer pattern：turn 生命周期状态机发出事件
- `mcp/provider.go` —— `tools/list_changed` notification 触发 cache invalidate

---

### 1.6 函数式中间件链（Middleware Chain / Chain of Responsibility）

**已使用处**：

- `core/model/MiddlewareManager[Req, Resp]` —— `CallMiddleware` 和 `StreamMiddleware` 各一条链
- `core/model/middleware/` —— 具体 middleware 实现（logger / safeguard）
- `rag/pipeline_middleware.go` —— 把 pipeline 变成 chat middleware
- MCP SDK 原生 middleware chain（协议层，不在本仓库）

**特点**：一个 middleware 可同时满足 Call + Stream 两条链（通过 `UseMiddlewares(...any)` + 运行时 type-assert），减少了接口膨胀。

---

### 1.7 泛型类型分发（Generic Type Dispatch）

**已使用处**：

- `agent/runtime/collectExtensions[T]` —— 从 `[]Extension` 中按能力类型收集相应子接口
- `core/model/CallHandler[Req, Resp]` / `StreamHandler[Req, Resp]` —— 泛型骨架，各模态具化
- `agent/core/TypedAction[In, Out]` —— 泛型 action，框架自动类型匹配

**设计决策**：用 Go 泛型在编译期验证类型安全，避免 `interface{}` + runtime type-assert。

---

### 1.8 Sealed Interface（密封接口）

**已使用处**：

- `core/model/chat.Message` —— 通过未导出方法 `message()` 封口
  - 子类型：`SystemMessage` / `UserMessage` / `AssistantMessage` / `ToolMessage`
  - 外部无法新增子类型，保证 type switch 穷尽

**Go 惯用法**：Go 没有 `sealed` 关键字，用未导出方法模拟。

---

### 1.9 状态机（State Machine）

**已使用处**：

- `agent/runtime/AgentProcess` —— tick 循环：plan → observe → act
  - 状态：submitted → working → waiting(HITL) → completed / failed
- `lyra/internal/kernel/turn/` —— turn 生命周期状态机
- `a2a/executor.go` —— A2A task 生命周期：submitted → working → artifact deltas → completed / failed

---

## 2. 逐个模块分析

### 2.1 `core/` —— 协议定义层

**定位**：整个 lynx 生态的接口契约。接口在此定义，实现在外圈。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **Sealed Interface** | `core/model/chat/message.go` | `Message` 接口 + 未导出 `message()` 方法 |
| **泛型骨架** | `core/model/model.go` | `CallHandler[Req, Resp]` / `StreamHandler[Req, Resp]` |
| **中间件链** | `core/model/middleware.go` | `MiddlewareManager` + CallMiddleware / StreamMiddleware |
| **函数形适配器** | `core/model/model.go` | `CallHandlerFunc[Req, Resp]` 类比 `http.HandlerFunc` |
| **责任分离** | `core/vectorstore/store.go` | `Store = Creator + Retriever + Deleter + Metadata` |
| **管道** | `core/document/` | Reader → Transformer → Batcher → Writer → Formatter |
| **组合模式** | `core/evaluation/composite.go` | `Composite` Evaluator |
| **选项模式** | `core/model/chat/options.go` | Options 字段为 pointer（nil = 用默认） |
| **filter DSL** | `core/vectorstore/filter/` | lexer → parser → AST → analyzer 迷你语言 |

#### 可优化机会

1. **Message Visitor 模式**
   - **问题**：`chat.Message` sealed interface 导致消费方散落大量 type switch（每个 models provider、每个 chatmemory backend 各自 type switch 一遍）
   - **方案**：在 `core/model/chat/` 中新增 `MessageVisitor` 接口 + 每个子类型的 `Accept` 方法，但保持 sealed（不在公开 API）
   - **收益**：provider / chatmemory 的 type switch 集中在一处，新增子类型时（虽然 sealed 不允许外部加，但内部修改）的触及面缩小
   - **风险**：违反 YAGNI？目前子类型只有 4 个且极少新增，type switch 复杂度低。**优先级：低**

2. **Request/Response 统一构造（Builder / Spec 模式）**
   - **问题**：不同模态（chat / embedding / image / audio）的 Request 构造方式各异，但共享很多共性（model、options、metadata）
   - **方案**：在当前 Options 模式基础上，为各模态提供链式构造器（`chat.NewRequest().WithModel("x").WithTemperature(0.7)`），但**保留 options struct 为主路径**（符合 DESIGN_PHILOSOPHY §3 的 "options struct 优于 builder 链"）
   - **收益**：降低消费者心智负担，但设计哲学明确倾向 options struct。**优先级：不做**

---

### 2.2 `models/` —— 38 个 LLM provider 适配层

**定位**：每个 provider 一个独立子包，全部实现 `core/model/` 中的统一接口。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **适配器** | 每个 `*/chat.go` | 把各 SDK 的 request/response 映射到 `chat.Request` / `chat.Response` |
| **模板方法** | 每个 provider 的固定文件结构 | `api.go` → `chat.go`(Config + Model + requestHelper + responseHelper) → `embedding.go` → ... |
| **工厂方法** | 每个 `*/chat.go` | `New<Provider>ChatModel(cfg) (*ChatModel, error)` |
| **外观** | 每个 `*/chat.go` | Metadata 覆盖：`Config.Metadata` 非 nil 用它，否则用包级默认 |
| **累加器** | 每个 `*/chat.go` | `chunkAccumulator`：SSE delta → Response chunk；`chat.ResponseAccumulator` stitch 完整消息 |
| **选项合并** | `internal/options/` | `GetParams[T](opts, OptionsKey)` 泛型提取 provider 专属参数 |
| **兼容层** | `moonshot/` / `openrouter/` | 同一 provider 双 API（OpenAI 形状 + Anthropic 形状） → 策略模式 |

#### 可优化机会

1. **Provider Registry / Service Locator**
   - **问题**：38 个 provider 各自独立子包，没有统一的 provider 发现机制。消费方（lyra）需要手动 `switch providerID { case "anthropic": ... case "openai": ... }`
   - **方案**：在 `models/` 中新增 `registry` 包，用 `init()` 自注册模式或显式 Registry：
     ```go
     // models/registry/registry.go
     type Factory func(ctx context.Context, cfg ProviderConfig) (chat.Model, error)
     var registry = map[string]Factory{}
     func Register(id string, f Factory) { registry[id] = f }
     func Build(ctx context.Context, id string, cfg ProviderConfig) (chat.Model, error)
     ```
   - **收益**：加新 provider 只需新建子包 + `init()` 注册，消费方零修改
   - **兼容**：无需破坏现有 API，注册表与直接构造并行，逐步迁移
   - **注意**：DESIGN_PHILOSOPHY §2.3 倾向"一个通用机制"而非具名插槽——Registry 正符合。
   - **优先级：高**。lyra 的 `config.BuildClient(ClientSpec)` 目前用 switch 映射，随着 provider 增长维护成本上升。

2. **Provider 能力声明**
   - **问题**：消费方无法在编译期或运行期知道某 provider 支持哪些模态（chat? embedding? image?），全靠 `models/catalog/` 的运行时查表
   - **方案**：每个 provider 实现 `Capabilities() []Modality` 接口方法，Registry 聚合
   - **收益**：消费方可以运行时做能力匹配，而非靠"试一下看瞎不瞎"
   - **优先级：中**

3. **反模式警示 — 无**
   - 当前适配器结构高度统一，模板方法模式执行到位
   - 每 provider 独立 requestHelper / responseHelper 而非共享基类是正确的 Go 选择（不同 SDK shape 差异大于相似度，强抽基类反而不利）

---

### 2.3 `vectorstores/` —— 27 个向量数据库后端

**定位**：`core/vectorstore.Store` 的 27 个具体实现。核心设计：**Visitor 模式** 把统一 filter AST 编译为各后端方言。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **访问者 (Visitor)** | 每个 `*/visitor.go` | `ast.Visitor` 接口遍历 filter AST，编译为后端方言 |
| **适配器** | 每个 `*/store.go` | 实现 `vectorstore.Store = Creator + Retriever + Deleter` |
| **工厂方法** | 每个 `*/store.go` | `NewStore(cfg *StoreConfig) (Store, error)` |
| **责任链** | `core/vectorstore/filter/` AST | lexer → parser → AST → visitor（链式处理） |
| **模板方法** | 每个后端的固定目录结构 | `store.go` + `visitor.go` + `doc.go` + `errors.go` |
| **共享夹具** | `internal/storetest/` | `VisitorConformance()` 测试套件，新后端注册即覆盖 |

#### 可优化机会

1. **Visitor 泛型化**
   - **问题**：27 个 visitor 实现中，大部分节点处理逻辑重复（如 AND/OR/NOT 组合节点在所有 visitor 中几乎相同）
   - **方案**：提供一个 `BaseVisitor`（struct embed，可选 override）处理通用节点，各后端只 override 特定方言部分（如值编码、字段引用）
   - **收益**：新加后端时从 `~200 行 visitor` 减少到 `~50 行`，大幅降低维护成本
   - **注意**：这是经典 Visitor 模式的改进版（模板方法 + Visitor 结合）。Go 中通过 struct embed 实现，不破坏接口。
   - **优先级：高**。27 个后端的维护一致性是真实痛点。

2. **反模式警示 — 无**
   - Visitor 模式的选择和实现质量很高
   - `internal/storetest` 共享测试套件是优秀实践
   - 维度协商、Batch upsert 策略各后端独立处理，隔离良好

---

### 2.4 `agent/` —— 目标导向 agent 运行时

**定位**：Planner-driven（非 ReAct-loop）agent 运行时。三大支柱：原语层 `core/` + 执行引擎 `runtime/` + 扩展点 `Extension`。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **策略模式** | `planning/planner/{goap,htn,reactive,utility}/` | `Planner` 接口 + 4 种算法实现 |
| **建造者 (Builder)** | `agent.go` | fluent Builder 构造 Agent |
| **工作流编排** | `workflow/` | Sequence / Loop / Parallel / ScatterGather / RepeatUntil / Consensus |
| **观察者** | `event/` | 生命周期事件 + 多播 listener |
| **装饰器** | `toolpolicy/` | OnceOnly / Unlocked 等 chat tool 装饰器 |
| **插件 / 扩展点** | `core/Extension` + `runtime/collectExtensions[T]` | 单一扩展机制 + 泛型类型分发 |
| **黑板 (Blackboard)** | `core/Blackboard` | 按 name+type 存取，观察方拿 Reader、修改方拿 Writer |
| **状态机** | `runtime/AgentProcess` | tick 循环，状态转换 |
| **泛型 Action** | `core/TypedAction[In, Out]` | 编译期类型安全的 action |
| **三值逻辑** | `core/Determination` | Unknown / True / False，带 And/Or/Not |
| **HITL 一等待** | `hitl/Awaitable[T]` | 暂停/恢复模式 |

#### 可优化机会

1. **Specification 模式（规约）用于 Condition**
   - **问题**：Goal 的 Condition 目前用 `Determination` 三值逻辑直接判断。复杂场景下（如"所有文件都已关闭 AND 用户已确认"）难以组合
   - **方案**：引入规约模式：
     ```go
     type Condition interface {
         IsSatisfiedBy(ctx context.Context, bb BlackboardReader) Determination
         And(Condition) Condition
         Or(Condition) Condition
         Not() Condition
     }
     ```
     `Determination` 的 And/Or/Not 保留为值级运算
   - **收益**：可组合的 goal 条件，支持复杂业务规则
   - **优先级：中**。当前简单场景下三值逻辑够用，但 agent 能力增长后很快就会需要。

2. **Command 模式包装 Action**
   - **问题**：`Action` 直接持有 `func(Process) error`，没有天然的 undo / replay 支持
   - **方案**：Action 新增 `Rollback` 可选方法，让 framework 在 planner 取消 plan 时能回滚
   - **收益**：agent 执行失败的恢复能力
   - **优先级：低**。当前 planner 不做 undo/rollback，YAGNI。

3. **反模式警示**
   - ❌ 不存在：workflow builders（Sequence/Loop 等）产出普通 GOAP agent，而非独立运行时 —— 这是正确的（对应 DESIGN_PHILOSOPHY 形态 ②：组合）
   - ❌ 不存在：Extension 用 string-key 注册（已采用更优的类型分发模式）

---

### 2.5 `rag/` —— RAG 流水线

**定位**：5 阶段可组装的 RAG 管道。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **管道-过滤器** | `pipeline.go` + `stages.go` | 5 个阶段接口顺序执行 |
| **策略模式** | 每个阶段接口 | 各有多实现：QueryTransformer（rewrite/compression/translation）、DocumentRefiner（dedup/rank） |
| **Fan-Out / 并行** | `pipeline.go` 的 Retrieve 阶段 | WaitGroup 并行跑多个 retriever，部分失败容忍 |
| **中间件** | `pipeline_middleware.go` | 把 pipeline 包装成 chat middleware |
| **空对象** | `nop.go` | 每个阶段的 Nop 实现，方便部分使用 |

#### 可优化机会

1. **Reactive / Stream 按需拉取**
   - **问题**：当前全量执行 5 阶段后才返回结果。在 LLM 只需第一个检索结果时就够用的场景，流水线做了多余工作
   - **方案**：用 `iter.Seq2` 让每个阶段返回 iterator（lazy），下游按需拉取
   - **收益**：LLM 可提前退出，减少 token+延迟
   - **优先级：低**。大多数 RAG 场景需要完整结果集，提前退出是优化而非问题。

2. **反模式警示 — 无**
   - `Query.Extra` 元数据跨阶段传递是好的设计决策
   - Router / Joiner 阶段的有意不做是成熟的 YAGNI 判断

---

### 2.6 `lyra/` —— 后端运行时（Clean Architecture）

**定位**：Go agent runtime backend，实现 Lyra Runtime Protocol。**严格 Clean Architecture**（delivery → kernel → domain → infra，依赖向内）。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **Clean Architecture** | `internal/` 的 4 层 | delivery / kernel / domain / infra，`arch_test.go` 机器强制 |
| **外观 (Facade)** | `internal/kernel/` | 装配 system prompt + 工具集 + model client |
| **组合根** | `cmd/lyra/app.go:buildStores()` | 集中装配具体实现 |
| **领域服务** | `internal/domain/*/service.go` | 一域一包，interface + 实现按本质命名 |
| **仓储 (Repository)** | `internal/infra/storage/` | 单一 SQLite 后端，实现各 domain service 接口 |
| **策略模式** | `internal/domain/maintenance/` | Compactor / Extractor / Planner 各策略 |
| **观察者 / 发布-订阅** | `internal/delivery/server/hub.go` | per-run hub 做 SSE 事件源 |
| **桥接** | `internal/infra/mcp/` + `internal/infra/a2a/` | MCP/A2A 到内部工具集 |
| **CQRS-lite** | `internal/domain/transcript/` + `internal/domain/conversation/` | items+runs 时间线 vs 喂 LLM 的消息上下文 |
| **Transport 抽象** | `internal/delivery/transport/` | HTTP / inprocess 实现同一 Transport 接口 |

#### 可优化机会

1. **Domain Events / Event Bus**
   - **问题**：domain service 之间通过直接调用耦合。如 `maintenance.Compactor` 压缩后需要通知 `transcript`、`conversation` 更新视图，目前靠 kernel 编排手动调用
   - **方案**：引入轻量级 domain event bus（module 内，不跨模块）：
     ```go
     type EventBus interface {
         Publish(ctx context.Context, event DomainEvent)
         Subscribe(eventType string, handler EventHandler)
     }
     ```
   - **收益**：domain service 之间解耦，加新副作用不改 kernel 编排
   - **注意**：不是全量 Event Sourcing，只是模块内解耦。对外部的通知（SSE push）已有 hub 机制。
   - **优先级：中**。当前 domain 数量尚少（~12 个），编排复杂度可控，但持续增长后会需要。

2. **Saga / 补偿事务**
   - **问题**：跨 domain service 操作无事务保证（如：create session + init transcript + init conversation），某个步骤失败需要回滚
   - **方案**：每个 domain service 提供 `Compensate` 方法，kernel 编排失败时反向补偿
   - **优先级：低**。单步操作失败率低，SQLite 的 ACID 已覆盖大多数场景。

3. **反模式警示**
   - ❌ 不存在：`internal/domain/agentdoc/` 有独立测试，架构约束 `arch_test.go` 防依赖反向
   - ❌ 不存在：过度的抽象层（Clean Arch 4 层恰到好处，非教科书式 6-7 层）

---

### 2.7 `chatmemory/` —— 聊天记忆持久化

**定位**：`memory.Store` 接口的 6 个数据库后端实现。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **适配器** | 每个 `*/store.go` | 实现 `memory.Store`（Write/Read/Clear） |
| **工厂方法** | 每个 `*/store.go` | `NewStore(config) (Store, error)` |
| **统一序列化** | 所有后端 | canonical JSON envelope（`chat.Message.MarshalJSON` / `chat.UnmarshalMessage`） |
| **跨度追踪** | `internal/tracing/` | Read/Write/Clear 各打一个 OTel span |

#### 可优化机会

1. **CQRS 读写分离**
   - **问题**：`memory.Store` 接口混合读写——`Write`、`Read`、`Clear` 在一个接口里。大并发下，读密集 vs 写密集可能有不同优化策略
   - **方案**：拆成 `MessageReader` + `MessageWriter`（+ `MessageDeleter`），消费方可只依赖其一
     ```go
     type Reader interface { Read(ctx, convID) ([]Message, error) }
     type Writer interface { Write(ctx, convID, msgs...) error }
     type Store = Reader & Writer & Deleter  // 组合回去
     ```
   - **收益**：ISP 优化 —— 只读场景（如 summary 看历史）不依赖 Write；有独立 read-replica 时注入不同实现
   - **兼容**：保留 `Store = Reader & Writer & Deleter` 组合类型别名，不破坏现有消费
   - **优先级：中**

2. **事务 / Unit of Work**
   - **问题**：多会话并发写时无事务保证，`Write(msgs...)` 批量写入非原子
   - **方案**：新增 `BatchedWriter` 接口（或直接在 `Write` 保证原子性）
   - **优先级：低**。当前单会话内顺序写入，原子性足够。跨会话事务场景不存在。

3. **反模式警示 — 无**
   - 每个 backend 走统一 JSON envelope 是正确的设计决策
   - SQL identifier 验证防注入在每个 backend 中独立实现，因方言不同，正确

---

### 2.8 `tools/` —— LLM 可调用工具集

**定位**：实现 `chat.Tool` 接口的工具集合。**两层 SPI**：Tool 层（JSON in/out + schema）+ Executor 层（真正执行）。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **两层 SPI** | `tools/` 全模块 | Tool 层 + Executor/Provider 层 |
| **策略模式** | `websearch/` / `webfetch/` | 多个 Provider 实现同一接口（Tavily / Brave / Exa / Jina / Firecrawl …） |
| **适配器** | 每个工具 | 具体操作 → `chat.Tool` JSON 接口 |
| **Command 模式** | 每个工具 | JSON arguments in → action → JSON result out |
| **守卫 / 代理** | `httpreq/Client` | AllowHosts + AllowedMethods 守卫执行 |
| **工厂方法** | 每个 `NewXxxTool(executor)` | nil executor → 默认 LocalExecutor（或返错） |
| **装饰器** | `toolpolicy/` (agent 模块) | Outside this module but decorates tools |
| **选项模式** | `fs.LocalExecutor.Root` | 相对路径锚点 |
| **自动 Schema** | `pkg/json.StringDefSchemaOf` | 从 Input struct 推导 JSON Schema |

#### 可优化机会

1. **Undo 能力（Command 模式的另一半）**
   - **问题**：`chat.Tool.Call()` 是单向命令。LLM 调用 `write` 或 `bash` 后无法撤销，agent 失败恢复靠外部 checkpoint
   - **方案**：为写操作工具新增可选 `Undo` 接口：
     ```go
     type UndoableTool interface {
         chat.Tool
         UndoCall(ctx context.Context, arguments string, previousResult string) (string, error)
     }
     ```
     框架通过 type-assert 检测是否支持 Undo
   - **收益**：agent 出错后能精确回滚单个工具调用，而非依赖整个文件 checkpoint
   - **注意**：有些操作天然不可逆（HTTP 请求），Undo 是可选能力
   - **优先级：中**。当前靠 lyra 的 editguard + git checkpoint 覆盖，但工具级 Undo 更精确。

2. **Tool Composition（链式组合工具）**
   - **问题**：多个工具调用之间无内置编排（如：先 glob → 再 read → 最后 write），全靠 agent planner 按 step 分别调用
   - **方案**：提供 `ToolPipeline` 机制：把多个工具串成序列，前一个的输出作为后一个的输入
   - **优先级：低**。这是 agent planner 的职责，工具层不该管编排。YAGNI。

3. **反模式警示**
   - ❌ 不存在：全局 tool registry（有意为之 —— 多 agent/多 process 各自管理 toolset）
   - ❌ 不存在：Tool 层做业务逻辑（所有业务在 Executor，Tool 只是 JSON ↔ Go 转换）

---

### 2.9 `mcp/` —— MCP 协议桥接

**定位**：`modelcontextprotocol/go-sdk` 的 lynx 适配层。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **适配器** | `tool.go` | 远端 MCP tool → lynx `chat.Tool` |
| **代理 (Proxy)** | `provider.go` | `Provider.Tools()` 缓存 + `tools/list_changed` invalidate |
| **双检查锁定** | `provider.go` | `Tools()` cache 的 double-checked locking |
| **外观** | `server.go` | `RegisterTools` 统一注册入口 |
| **观察者** | `provider.go` | notification → invalidate |

#### 可优化机会

1. **与 a2a 的统一 Tool Provider 抽象**
   - **问题**：`mcp.Provider` 和 `a2a.DialAll` 都是"从远端获取 chat.Tool 列表"的机制，但接口形状不同（mcp 有 cache + invalidate，a2a 是静态一次性）
   - **方案**：两者各自差异是有真实理由的（MCP 工具列表可变、A2A agent 静态），**不做强行统一**（虚假 DRY）
   - **优先级：不做**。A2A CLAUDE.md 已记录此分叉是故意的。

2. **反模式警示 — 无**
   - MCP primitive 取舍（只做 Tools + 反向辅助，不做 Resources/Prompts/Roots）有明确文档和零消费者理由 —— 成熟 YAGNI

---

### 2.10 `a2a/` —— Agent-to-Agent 协议桥接

**定位**：`a2aproject/a2a-go/v2` 的 lynx 适配层（mcp 的姊妹模块）。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **适配器** | `agent_tool.go` | 远端 A2A agent → `chat.Tool` |
| **门面** | `transport.go` | `DialAll` 一次拨号多个 agent 产出 `[]chat.Tool` |
| **窄接口** | `executor.go` | `Agent` 接口（包内定义）→ `a2asrv.AgentExecutor` |
| **状态机** | `executor.go` | task 生命周期（submitted → working → artifact → completed/failed） |

#### 可优化机会

1. **反模式警示 — 无**
   - 与 mcp 的结构差异（无 Provider 包装层）有真实理由（A2A agent 静态、MCP 工具列表可变）
   - 协议层不重新发明，依赖官方 SDK

---

### 2.11 `skills/` —— Agent Skills 基础能力层

**定位**：Agent Skills 规范（agentskills.io）的解析/校验/取用层。**零业务依赖**。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **渐进式披露** | `source.go` 三方法 | Level 1: `List` → Level 2: `Load` → Level 3: `LoadResource` |
| **策略模式** | `source.go` | `Source` 接口 → FS / embedded / remote / fake 多实现 |
| **懒加载** | `source.go` | per-call 读，不缓存不预扫 |
| **规范校验** | `frontmatter.go` | `Parse`（拆）+ `Validate`（校验）分离 |

#### 可优化机会

1. **反模式警示 — 无**
   - 零业务依赖（不 import `core` / `chat`）是优秀的模块隔离
   - 不执行 `scripts/`（交给 agent 的 bash 工具）是 KISS 实践
   - 路径逃逸防护到位

---

### 2.12 `documentreaders/` —— 文档阅读器

**定位**：不同格式 → `document.Document` 流的适配层。3 个 reader 各独立子包 + 独立 go.mod。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **策略模式** | 各 reader | `document.Reader` 接口 → markdown / html / pdf 实现 |
| **功能选项** | `markdown/` | `WithHeadingSplit()` / `WithSourceName(str)` |
| **命名空间元数据** | 各 reader | `markdown.*` / `html.*` / `pdf.*` 前缀防冲突 |

#### 可优化机会

1. **反模式警示 — 无**
   - 每个 reader 独立 go.mod（解析器依赖大，不让消费方拉不用的）—— 合理设计
   - 3 个 reader 不多，无过度抽象

---

### 2.13 `otel/` —— OpenTelemetry 开发导出器

**定位**：把 OTel 三驾马车（Traces / Metrics / Logs）的 dev sink 统一成 `log/slog`。

#### 已使用的设计模式

| 模式 | 位置 | 说明 |
|---|---|---|
| **适配器** | `slog/` | `SpanExporter` / `MetricExporter` / `LogExporter` → slog |
| **三信号统一** | `slog/` | 三个 exporter，每种 OTel 信号一份 |
| **Dev/Prod 切换** | 全局 provider 绑定 | dev 用 slog exporter，生产换 OTLP exporter，业务代码零改 |

#### 可优化机会

1. **反模式警示 — 无**
   - 模块定位明确（dev sink），不负责生产 exporter
   - `ExportSpans` 永远返回 nil 的错误处理策略适合 dev 场景

---

## 3. 仓库级优化机会（按优先级排序）

### 🔴 优先级高

| # | 机会点 | 模块 | 收益 |
|---|---|---|---|
| 1 | **Provider Registry** | `models/` | 加新 provider 无需改消费方，消除 lyra 的 switch 分支 |
| 2 | **BaseVisitor** | `vectorstores/` | 27 个 visitor 的通用节点逻辑共享，新后端从 200 行→50 行 |

### 🟡 优先级中

| # | 机会点 | 模块 | 收益 |
|---|---|---|---|
| 3 | **CQRS 读写分离** | `chatmemory/` | ISP 优化，读密集场景可独立注入 read-replica |
| 4 | **Domain Event Bus** | `lyra/internal/domain/` | domain service 间解耦，新副作用不改 kernel 编排 |
| 5 | **Provider 能力声明** | `models/` | 运行时能力发现，不靠试错 |
| 6 | **Specification 模式** | `agent/core/` | 组合条件表达复杂 goal 条件 |
| 7 | **Tool Undo 能力** | `tools/` | agent 出错的精确回滚，不依赖全量 checkpoint |

### 🟢 优先级低

| # | 机会点 | 模块 | 收益 |
|---|---|---|---|
| 8 | **Message Visitor 消除 type switch** | `core/model/chat/` | 子类型新增时触及面缩小，但当前仅 4 个子类型 |
| 9 | **Saga / 补偿事务** | `lyra/internal/` | 跨 domain 事务保证，但单步失败率低 |
| 10 | **Reactive Stream** | `rag/` | LLM 提前退出，但大多数场景需完整结果 |
| 11 | **ToolPipeline** | `tools/` | 工具链式编排，但这是 planner 的职责 |
| 12 | **Action Undo / Rollback** | `agent/core/` | agent 执行失败恢复，当前无此需求 |

---

## 4. 总结矩阵

| 模块 | 已用模式 | 可优化机会 |
|---|---|---|
| **core/** | Sealed Interface, 泛型骨架, 中间件链, 函数适配器, 责任分离, 管道, 组合, 选项, DSL | Message Visitor (低) |
| **models/** | 适配器, 模板方法, 工厂方法, 外观, 累加器, 策略 | **Provider Registry (高)**, Provider 能力声明 (中) |
| **vectorstores/** | **Visitor**, 适配器, 工厂方法, 责任链, 共享夹具 | **BaseVisitor (高)** |
| **agent/** | 策略, 建造者, 工作流, 观察者, 装饰器, 插件, 黑板, 状态机, 泛型, 三值逻辑, HITL | Specification 模式 (中), Action Undo (低) |
| **rag/** | 管道-过滤器, 策略, Fan-Out, 中间件, 空对象 | Reactive Stream (低) |
| **lyra/** | Clean Architecture, 外观, 组合根, 领域服务, 仓储, 桥接, CQRS-lite, Transport 抽象 | Domain Event Bus (中), Saga (低) |
| **chatmemory/** | 适配器, 工厂方法, 统一序列化, 跨度追踪 | CQRS 读写分离 (中) |
| **tools/** | 两层 SPI, 策略, 适配器, Command, 守卫/代理, 工厂, 选项, 自动 Schema | Tool Undo (中), ToolPipeline (低) |
| **mcp/** | 适配器, 代理, 双检查锁定, 外观, 观察者 | 不做（与 a2a 的差异有真实理由） |
| **a2a/** | 适配器, 门面, 窄接口, 状态机 | 无（结构成熟） |
| **skills/** | 渐进式披露, 策略, 懒加载, 校验分离 | 无（模块简洁） |
| **documentreaders/** | 策略, 功能选项, 命名空间元数据 | 无（规模合适） |
| **otel/** | 适配器, 三信号统一, Dev/Prod 切换 | 无（定位明确） |

---

## 5. 设计哲学一致性评估

> 对照 `DESIGN_PHILOSOPHY.md` 的六项自检清单评估当前状态：

| 自检项 | 状态 |
|---|---|
| 能力能归约到核吗？ | ✅ 所有模块都遵循（models → core/model, vectorstores → core/vectorstore, chatmemory → core/chat/memory） |
| 跨包依赖无反向？ | ✅ Clean Arch 四层 + arch_test 机器强制，consumer-defined interfaces |
| 消费方用窄接口？ | ✅ a2a.Agent, skills.Source, turn.engineDep 全部是窄接口 |
| 扩展点是同质机制？ | ✅ agent 的 Extension + collectExtensions[T] 是单一机制 |
| 配置用 options struct？ | ✅ core 层全部使用，models 也一致 |
| 流式用 iterator？ | ✅ 全仓库统一 `iter.Seq2`，不用 channel |

**结论**：仓库在设计模式使用和设计哲学一致性上都处于优秀水平。13 个模块中 9 个已高度成熟无需大改，4 个有明确的优化机会（全部有真实需求支撑，非推测性）。

---

## 附录：Go 惯用模式清单

以下模式是 Go 语言惯用法，lynx 中一致使用，不单独列为"设计模式"但值得记录：

- **accept interfaces, return structs** —— 全仓库
- **make zero values useful** —— `Determination`（零值 Unknown）、`Config` 等
- **small interfaces** —— `a2a.Agent`（1 方法）、`skills.Source`（3 方法）
- **options struct 优于 variadic/builder** —— `core/model/chat.Options` 等
- **泛型替代 interface{}** —— `CallHandler[Req, Resp]`、`TypedAction[In, Out]`
- **错误分层** —— sentinel error + `%w` 包装，`ToolCallError` 区分业务/传输错误
- **interface 在消费方定义** —— 全仓库（a2a、skills、lyra 均有此纪律）
- **struct embed 实现组合** —— agent workflow 等
- **零依赖模块** —— skills（零业务依赖）、otel（仅 OTel SDK）
- **编译期断言** —— `var _ Source = (*FS)(nil)` 等
- **独立 go.mod** —— documentreaders 每 reader 独立 go.mod
