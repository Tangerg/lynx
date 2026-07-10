# 架构评审 —— 资深架构师视角下的 Lyra（向 Clean Arch + DDD 方向）

> **日期**：2026-06-18。**视角**：资深架构师命题作文 —— "以 Linus（反仪式、看代码不看口号）+ Uncle Bob（Clean Arch 依赖规则、ports）的合体视角，评审 lyra 现状；如果从零写，系统架构与文件组织怎么设计，向 clean arch + DDD 方向？"
> **状态**：**批判性评审（review），非新基准**。与 [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md) 的关系：那份是**唯一架构基准**（"从零重写"的应然设计，已落地）；本文是**对现状的一次独立体检** —— 验证基准是否被遵守、指出真债、并裁决"DDD 方向"里哪些是该做的、哪些是仪式。本文不推翻基准，90% 收敛于它。
> **方法**：第一手通读 lyra 现状（rings / ports / 组合根 / `arch_test` / `delivery/server` 的 pump+rollback / 各 domain 包的贫血 vs 充血）+ 对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) §2.5（具体度向上流动）与 [`../../REFACTORING.md`](../../REFACTORING.md) §5.1（单后端不叠 DDD 层）。
>
> **结论先行**：
> 1. **Lyra 已经是机器强制的 Clean Arch + 六边形，不是"要往那个方向走"的项目。** `internal/arch/arch_test.go` 用 `go/parser` 解析 import、机器断言依赖方向 —— 这比任何文档/review 规范都硬。等级 **B+ → A-（差一步到 A）**。
> 2. **"向 DDD 方向"的真正问题不是"怎么走过去"，而是教科书 DDD 的哪些部分对这个系统是对的、哪些是仪式。** 项目自己的 `DESIGN_PHILOSOPHY §2.5` + `REFACTORING §5.1` 已回答过 —— 明确反对为单后端叠 DDD 层（repository / application-service / 聚合根 / domain-events 框架），视为 YAGNI 仪式。**资深架构师的诚实裁决：这套反仪式立场对这个系统是对的。**
> 3. **（本文写作时）真正欠的债只有一处：`delivery/server` 里藏了 use-case 编排（`pump.go` + `rollback.go`）。** "协议层薄"的口号（见 [`../CLAUDE.md`](../CLAUDE.md) 一句话定位）在这两个文件上不成立。把那两块的多服务协调搬进 kernel，就到 A。
>
> ---
>
> **状态更新（2026-07-06，`codex/runtime-architecture-refactor` 分支）—— 结论 #3 的两处债已全部落地，架构到 A：**
> - **rollback（原 P0-1）✅**：跨服务写集已下沉 `internal/kernel/lifecycle` 的 `Coordinator.Rollback` / runtime 的 `RollbackResolved`（`TruncateMessages` + `DeleteRun` + `Interrupts.Delete` + 子树 purge 在一个 `RunInTx` 里）。`delivery/server/rollback.go` 只剩 wire 解码 → 抬升 `RunNode` → `transcript.Timeline.BoundaryAt` → `RollbackResolved` → workspace checkpoint 恢复（刻意留适配器侧，见 coordinator 包 doc）→ 拼响应，**零 inline 写集**。
> - **pump（原 P0-2）✅**：多服务协调已下沉 `internal/kernel/runsegment` 的 effects（`BeforeLive`/`AfterLive`/`Finish` 做持久化 + 中断登记 + 文件变更通知 + snapshot）。`pump.go` 只剩 `translator → 分配 eventId → hub.Append → 调 effects`。
> - 下文 §2.5 / §3.2 / §4 差异②③ / §5 P0 描述的是**落地前**的现状，保留作评审考古；其 actionable P0 现已 DONE。

---

## 0. 怎么读这份文档

- **作者评审视角**：本文是体检报告，用来定位"现状哪里是经得起推敲的好设计、哪里是真债、哪里是被反仪式立场正确拒绝的仪式"。它**不引入新债、不要求改名**（与 GREENFIELD §0 同纪律）。
- **与既有文档的关系**：GREENFIELD 是"从零应然"的**基准**；本文是"现状实然"的**评审**。两份合起来 = "该长成什么样" + "现在离那个样还差什么"。落地动作只认本文 §5 的 P0/P1；P2 是设计取向，等触发再做。
- **核实声明**：本文所有 file:line 引用均经第一手通读核实（非推断）。`delivery/server` 实测 **28 个非测试 .go 文件 / 4264 LOC**；`pump.go` 209 LOC、`rollback.go` 216 LOC 的编排内容均逐行确认。

---

## 1. 现状裁决

**等级：B+ → A-（差一步到 A）。** 在一个 pre-1.0 单体 Go 项目里看到这套骨骼，少见。

### 做得好的 4 件事

1. **`arch_test.go` 就是防腐层本体**（`internal/arch/arch_test.go`）—— 用 `go/parser` 解析 import、机器断言依赖方向。Go 没有强制分层，这个测试就是答案。比任何文档、任何 code review 规范都硬。
2. **`turn.Dispatcher` 是 use-case 接口的正确抽象级别**（`internal/kernel/turn/dispatcher.go`）—— "跑一个 turn"的状态机 / 事件流 / 中断恢复 / 取消 / steering 全在一个接口里，消费方（`delivery/server`）只依赖它。这是 exact right level of abstraction。
3. **`translator` 把 wire↔domain 翻译集中在一处**（`internal/delivery/server/translator.go`）—— 一个 run 一个 translator，持有 open item 的状态机，把 `turn.Event` 的 delta 流转成 `protocol.StreamEvent`。翻译逻辑不在 pump、不在 runs.go、不在各 handler 里撒。干净。
4. **`transcript.Timeline.BoundaryAt` 是纯领域不变量**（`internal/domain/transcript/transcript.go`）—— rollback/fork 的"在哪条 run 边界切开时间线"是纯领域算法，不依赖 SQL、不依赖 wire、不依赖 session。零 I/O 可单测。这正是 `REFACTORING §5.1` 要的"充血"。

### 真正的 3 处债

1. ~~**`delivery/server` 里藏了非 delivery 的编排逻辑** —— `pump.go:pumpRun` 协调 turn 事件 → translator → persist → hub → snapshot 五个服务跨层编排；`rollback.go:RollbackSession` 编排 transcript + conversation + workspace + interrupts + subtask purge。**"协议层薄"不成立。**~~（详见 §2.5、§5 P0）**✅ 已于 2026-07-06 落地，见顶部状态更新** —— pump 编排 → `kernel/runsegment` effects，rollback 编排 → `kernel/lifecycle.Coordinator`。
2. **`domain/workspace` 和 `domain/codeintel` 是 domain facade 盖在 infra 上** —— 它们 import `infra/git` / `infra/lsp` / `infra/checkpoint`，本质是"领域化适配器"，叫 `domain` 名实不符。（详见 §2.1）
3. **个别 domain 包是纯贫血数据袋 + Store interface**（`provider` / `interrupts` / `knowledge`）—— 但这些领域本质就是 CRUD / 注册表，**贫血是对的，不是债**。（详见 §2.4）

---

## 2. 与教科书 Clean Arch / DDD 的偏差逐条裁决

### 2.1 `domain/workspace` & `domain/codeintel` import infra 直接 —— 混合债（结构性）

现状已收敛到 `internal/adapter/workspace` import `infra/git` + `infra/checkpoint`，`internal/adapter/codeintel` import `infra/lsp`。这是文档化的设计选择：adapter wraps an infra capability。动机是让 delivery 不直接 import infra —— delivery 通过 `workspace` package functions / `workspace.Checkpoints` 间接用 git。依赖方向合规：`delivery → adapter → infra`。

**但问题是**：如果 `workspace` 叫 `domain`，那它不能同时 import `infra` 又保持 "domain" 语义。Clean Architecture 中 domain 层应零外部依赖（只依赖语言标准库 + 核心抽象）。`workspace` 的正确归属是 **adapter 环**，不是 domain 环 —— 它是 git/checkpoint 的领域化包装，位于 domain 和 infra 之间的灰色地带。

**裁决**：**混合债（结构性，非功能性）**。功能上没问题，名实上不准。修法见 §4 差异①（移到 `internal/adapter/`，或保留在 `domain/` 但接受"复合包"定位）。**不紧急** —— 纯机械 churn，零行为变化。

### 2.2 `maintenance` import `kernel` 拿 DTO —— 归属应在 adapter

`maintenance` import `kernel` 只用 `kernel.CompactionResult` / `kernel.ExtractionResult`。这些是 `kernel/port.go` 定义的 DTO —— `kernel` 是 port 的 owner（消费方定义接口），`maintenance` 是实现方。**实现方 import port owner 的类型是六边形架构的标准方向**：adapter 依赖于 port 所在的层。

**裁决**：import 方向正当，但包归属应是 `internal/adapter/maintenance`，不是 domain ring。归属修正后，`arch_test.go` 不再需要 domain → kernel 的 documented exception。若未来这些 DTO 被多个实现方 import，可抽到独立 types 包；目前单实现方，抽是 YAGNI。

### 2.3 `kernel` 在 delivery 和 domain 之间作为独立环 —— 正当，命名准确

`kernel` 的角色：定义 port（`kernel/port.go`）→ 消费 domain 实现 → 被 delivery 驱动。在 Clean Architecture 中这就是 use-case 层的角色。`kernel/turn.Dispatcher` IS the use-case 接口。GREENFIELD §5.3 明确不做独立 application-service 环，理由正确：单交付运行时下，独立 app 环是 `delivery/server` 的 1:1 影子，YAGNI。

**裁决**：**正当**。`kernel` 这个名字比 `usecase` 好 —— 它描述"这是一个微内核，定义 port、驱动 agent loop"，而不是空洞的"这里是 use case"。`REFACTORING §1` 要求名字符合本质，`kernel` 符合。

### 2.4 `domain/*` 贫血 vs 充血 —— 基本正确，个别可改进

按 `REFACTORING §5.1` 标尺（数据就该是数据的不算贫血；"把规则收回类型"而非"往上加层"）逐包裁决：

| 包 | 有领域行为？ | 裁决 |
|---|---|---|
| `session` | ✅ `EffectiveModel()` / `Fork()` / `NewSubtask()` | 充血 |
| `transcript` | ✅ `BoundaryAt()` / `RunNode.IsRoot()` | 充血 |
| `todo` | ✅ `Validate()` / `Status.Valid()` / `Render()` | 充血（对小领域足够） |
| `editguard` | ✅ `Tracker.Record/Check/Refresh` / `Result.Message()` | 充血（模范） |
| `conversation` | ✅ `Messages.InjectUser` 封装 session 键控 + 原子截断 | 正确（薄但非贫血） |
| `maintenance` | ✅ `Compactor.MaybeCompact` 有触发阈值 + LLM 决策 | 充血 |
| `knowledge` | ❌ 纯 `Store` interface + file-backed 实现 | 数据就该是数据 —— 正确 |
| `provider` | ❌ 纯注册表 CRUD | 注册表 —— 正确 |
| `interrupts` | ❌ 纯 `Store` CRUD | 持久化记录 —— 正确 |
| `approval` | ✅ `ToolCallInput.Plan` / `GateFor`·`RiskFor` / `ruleSet.decide`（specificity 排序） | 充血（审批策略模型；本文写作后长出，已非"只切 mode"的贫血） |
| `workspace` | ❌ 基本是 git/checkpoint 的 pass-through | 见 §2.1 |
| `agentdoc` | ❌ 发现 + 渲染，基本无状态 | 正确 —— 这是算法，不是实体 |

**裁决**：**lyra 的 domain 层不是贫血的**。该有行为的实体有行为（Session / RunNode / Tracker / Item），该是数据的就是数据（Provider / Pending）。`REFACTORING §5.1` 的标尺在这里被遵守了。**不需要往上加 DDD 层。**

### 2.5 `delivery/server` 太大，"协议层薄"不成立 —— 结构性债（需要动，但不需要加层）

`delivery/server/` 28 个非测试文件 / 4264 LOC。LOC 总量大本身不是问题（协议方法多，适配器天然大）。**问题在 `pump.go` 和 `rollback.go` 里的跨服务编排不在 delivery 层。**

**`pump.go:pumpRun`**（`internal/delivery/server/pump.go:67`）的 `emit` 闭包 + defer 做的事：

| 行 | 做的事 | 属于 delivery？ |
|---|---|---|
| `tr.translate(ev)` → `protocol.RunEvent{...}` | wire↔domain 翻译 + eventId 分配 | ✅ |
| `hub.Append(re)`（L114） | 推送 per-run hub（streamable HTTP 基础设施） | ✅ |
| `s.recordInterrupt(...)`（L94） | 持久化 HITL 中断记录 | ⚠️ 编排 |
| `s.persistStreamEvent(...)`（L110） | 持久化 durable history 到 transcript | ⚠️ 编排 |
| `s.emitToolFileChange(...)`（L113） | 通知 workspace 订阅者 | ⚠️ 编排 |
| `go s.snapshotCheckpoint(...)`（L154） | run 边界文件快照 | ⚠️ 编排 |

**`rollback.go:RollbackSession`**（`internal/delivery/server/rollback.go:79`）做的事：串联 `Session.Get` → `Transcript.List` → `transcript.Timeline.BoundaryAt`（领域算法）→ `restoreCheckpoint`（workspace）→ `TruncateMessages`（conversation）→ `DeleteRun` + `Interrupts.Delete` → `purgeSubtasksAfter`（级联 purge 子会话）。**这是横跨 6 个 domain 服务的 use-case，不是协议适配。**

**裁决**：**结构性债**。修法见 §5 P0 —— 不是把 pump/rollback 整体搬走（translator / hub / wire helpers 是 delivery 的），而是把其中的**多服务协调**抽到 kernel 层的编排器，delivery 只剩 decode → 调编排器 → present。

> **✅ 已落地（2026-07-06，见顶部状态更新）**：rollback 的多服务协调在 `internal/kernel/lifecycle.Coordinator`（`Rollback`/`RollbackResolved`），pump 的在 `internal/kernel/runsegment` effects（`BeforeLive`/`AfterLive`/`Finish`）。delivery 侧两文件现只剩 wire 翻译 + workspace 适配 + 调用编排器 —— 裁决预期的形态已达成。

### 2.6 `adapter/toolset/build.go` god 构造器 —— 正当（不是债）

`build.go` 构造 7+ 个 capability adapter（codeintel / exec / MCP / A2A / ask_user / exit_plan / todo / skills），组装 resolver，产出 `Built`。这是**单一装配点** —— 所有工具能力在一个地方初始化，依赖关系一目了然。拆成多个构造器会散落装配逻辑，增加心智负担。GREENFIELD §7 明确拒绝 DI 容器，组合根手写是最清晰的方案。`build.go` 就是 toolset 的组合根。

**裁决**：**正当**。单一装配点是优点，不是缺点。**不动。**

### 2.7 `protocol.Runtime` mega-interface（11 子接口 union）—— 正当（不是债）

`delivery/protocol/runtime.go` 的 `Runtime` interface 嵌入 11 个子接口（Lifecycle / Sessions / Runs / Items / Workspace / Approval / Providers / Models / Tools / Memory / Feedback）。ISP 判据：是否有消费方只需要部分方法？

- **Transport dispatch**：需要全部方法（按 JSON-RPC method 路由到不同 handler）。它是唯一全量消费方。
- **HTTP transport**：通过 `protocol.Runtime` 嵌入 `Server`，也是全量消费。
- **测试**：可以只依赖子接口（如 `Runs`），ISP 已满足。

子接口已经存在，`Runtime` 只是 union 它们给 dispatch 用。若将来出现只消费部分方法的 transport（如 readonly health 端点），它可用子接口。

**裁决**：**正当**。当前设计正确。

---

## 3. 如果从零写：系统架构

### 3.1 环层结构和依赖规则 —— 不重新发明

现状的 4 环 + 组合根是正确的，与 GREENFIELD §1-§5 收敛：

```
cmd/lyra (组合根最外壳)
  └─ internal/
       ├─ runtime/   组合根：装配所有环，nil-default SPI 注入
       ├─ config/    纯数据（env / yaml 解析）
       ├─ delivery/  协议适配器：wire decode → 调内核 → wire present
       ├─ kernel/    微内核：定义 port + 驱动 agent loop + "跑一个 turn" 用例
       ├─ domain/    限界上下文：实体 + 领域规则 + 持久化 port（Store interface）
       └─ infra/     技术适配器：实现 domain 的 Store/Registry/Policy port
```

依赖方向：`delivery → kernel → domain → infra`（一律向内），`delivery` 也可直接依赖 `domain`（读 session、查 transcript）。**与 GREENFIELD 几乎无差异。**

### 3.2 Use Case / Application Service 层 —— 不需要独立环

**不需要独立的 use-case 环。** `kernel/turn.Dispatcher` IS the use-case 接口，`kernel.Engine` IS the use-case 实现的门面。理由同 GREENFIELD §5.3：单交付运行时下，独立 app 环是 `delivery/server` 的 1:1 影子，YAGNI。

**但从零会做得更硬的一件事**：从第一天起，任何多服务协调逻辑（pump 编排、rollback 编排）**一开始就放 kernel，不许进 delivery**。这是 GREENFIELD §6② 说的"adapter 里只准有 wire↔domain 翻译 + 编排调用"，现状没做到是因为 pump 和 rollback 是后来长出来的。

### 3.3 Aggregate Root & 充血模型 —— 不设显式标记，行为收回实体

**不设显式 Aggregate Root**（`REFACTORING §5.1`：对单后端是 YAGNI 仪式）。但每个 domain 包内的核心类型自带行为（见 §2.4 表）。**从零不会改变这里。** `Session.Fork()` 已经是聚合根行为，不需要 `AggregateRoot` interface 来"声明"。

### 3.4 Repository 模式 —— 不引入 "Repository" 命名

**不需要引入 "Repository" 命名。** 现状的 `Store` / `Registry` interface 就是 Repository，只是名字更短、更诚实：

| 现状名 | 等价 Repository | 为什么不改名 |
|---|---|---|
| `transcript.Store` | `TranscriptRepository` | Store = 存储，比 Repository 短 4 字符，含义相同 |
| `interrupts.Store` | `InterruptRepository` | 同上 |
| `session.Store` | `SessionRepository` | Store 包含 Create/Get/List/Delete，确是会话持久化 |
| `provider.Registry` | `ProviderRepository` | Registry 描述运行态 provider 目录，比 Repository 更具体 |
| `todo.Store` | `TodoRepository` | 只有 List+Replace，Store 名实相符 |

`REFACTORING §1` 要求名字符合本质。`Store` 描述"持久化存储"，`Registry` 描述"运行态目录"，`Policy` 描述"决策规则"。`Repository` 是 DDD 术语，对单团队项目不增加信息量。**不发生改名。**

### 3.5 Domain Events —— 永不

**不需要。** Lyra 是 request/response + streaming runtime，不是带 side-effect 的 CRUD 应用。"turn 结束 → 触发 compaction + extraction + snapshot checkpoint"已在 turn 的 post-turn maintenance 里显式编排。引入事件总线对单进程是 overengineering（GREENFIELD §7 明确不做 event-bus / Mediator）。**永不。**

### 3.6 如何保持微内核真薄

`kernel/port.go` 的窄 port 表是正确的（`SteeringSink` / `Compactor` / `Extractor` 等单方法 port）。**唯一风险点**：若未来有人把 `codeintel` / `exec` / `mcp` / `a2a` 塞进 kernel port，核就胖了。当前它们被 `toolset` 包在工具装配层，核只见 `ToolResolver` 一个 port。**保持这条纪律。**

---

## 4. 如果从零写：文件组织

### 目标树（与现状的差异只有 2 处结构性改动，其余不动）

```
lyra/
├── cmd/lyra/                         server-only main.go（组合根最外壳）
│
├── internal/
│   ├── config/                       纯数据：LYRA_* env + Config struct
│   ├── runtime/                      组合根：拼 kernel + domain + infra，nil-default SPI 注入
│   │
│   ├── delivery/                     交付 / 协议适配器
│   │   ├── protocol/                 冻结 wire 契约：Runtime 接口 + 子接口 + wire types
│   │   ├── server/                   in-process Runtime 实现（⚠️ 从零会更薄，见下）
│   │   │   ├── server.go             Server struct + 构造 + 协议方法
│   │   │   ├── translator.go         wire↔domain 翻译（turn.Event → StreamEvent/Item）
│   │   │   ├── hub.go                per-run event hub（streamable HTTP）
│   │   │   ├── sessions.go / runs.go / items.go / memory.go / ...
│   │   │   │                         每个协议方法组一个文件，纯 decode → call → present
│   │   │   └── runtime.go            RuntimePort 窄接口（消费方定义）
│   │   ├── dispatch/                 JSON-RPC 方法路由（表驱动）
│   │   └── transport/{http,inprocess}/
│   │
│   ├── kernel/                       微内核（定义 port + 驱动 agent loop）
│   │   ├── port.go                   核定义的窄 port + DTO
│   │   ├── engine.go                 Engine：装配 prompt/tool/agent，驱动 loop
│   │   ├── config.go                 kernel.Config（SPI 注入点）
│   │   ├── prompt.go                 system prompt 骨架装配
│   │   ├── turn/                     "跑一个 turn" 用例
│   │   └── stream/                   [从零新增] run segment pump 的编排部分（见差异②）
│   │       └── pump.go               turn.Event → 持久化 + 中断登记 + snapshot 协调
│   │
│   ├── domain/                       限界上下文（纯领域 + Store port，零 delivery/infra 依赖）
│   │   ├── session/ transcript/ conversation/ maintenance/ approval/
│   │   ├── tool/ editguard/ interrupts/ provider/ todo/ skills/ agentdoc/
│   │   └── （workspace / codeintel 移走，见差异①）
│   │
│   ├── adapter/                      [从零新增环] 领域化适配器（domain + infra 复合，非纯领域）
│   │   ├── workspace/                ← 从 domain/workspace 移来
│   │   └── codeintel/                ← 从 domain/codeintel 移来
│   │
│   └── infra/                        技术设施（零领域，不依赖上层）
│       ├── storage/{sqlite, FileKnowledgeStore}
│       └── {git,lsp,checkpoint,exec,mcp,a2a}/
│
└── doc/                              GREENFIELD_ARCHITECTURE.md（架构基准）
```

### 与现状的差异（逐条注明）

#### 差异①：`adapter/` 环（新增）—— `workspace` / `codeintel` 移出 domain

**改了什么**：`domain/workspace` 和 `domain/codeintel` 移到 `internal/adapter/`。

**为什么**：这两个包 import `infra/git` / `infra/lsp` / `infra/checkpoint`，本质是"领域化适配器"—— 对消费方（delivery）提供领域语义，对内包装 infra 实现细节。放在 `domain/` 下名实不符（叫了 domain 但依赖 infra）。`adapter/` 环准确描述它们的本质，且让 `domain/` 真正纯净（零 infra 依赖）。

**为什么可能不值得现在做**：纯机械 churn（import 路径重写），零行为变化。且 `workspace` / `codeintel` 对外暴露的确实是领域语义（`ListChanges` / `Diff` / `Snapshot`），放 `domain/` 对消费方而言是合理的抽象。这是**设计取向**，不是紧急重构（见 §5 P2）。

#### 差异②：`kernel/stream/` —— pump 的编排部分从 delivery 搬来

**改了什么**：`delivery/server/pump.go` 里 `emit` 闭包的 **persist + recordInterrupt + emitToolFileChange + snapshotCheckpoint** 协调，移到 `kernel/stream/`（或 `kernel/turn/`）的编排器。

**为什么**：pump 的核心工作是协调 turn 事件流 → 持久化 → 中断登记 → snapshot → 推 hub。其中持久化 / 中断 / snapshot 是 use-case 编排，不是协议适配。

**关键 nuance（不要整体搬）**：pump 里的 `translator`（产出 `protocol.StreamEvent`）和 `hub`（`protocol.RunEvent`）是 **delivery 层特有的类型**，搬到 kernel 会逼 kernel 认识 wire 类型 —— 反向依赖，违规。**正确拆法**：pump 保持在 `delivery/server` 作为"事件泵"（translator → hub.Append + eventId），但调一个 kernel 层的编排器（如 `RunSegmentCoordinator`）来做"持久化 + 中断登记 + snapshot"。pump 仍是 delivery 的流式管道，真正的跨服务协调在 kernel。

#### 差异③：`rollback.go` 的核心协调移到 kernel

**改了什么**：`delivery/server/rollback.go:RollbackSession` 里 `Transcript.List → BoundaryAt → restoreCheckpoint → TruncateMessages → DeleteRun/Delete → purgeSubtasks` 的编排序列，移到 kernel（作为 rollback 用例）。

**为什么**：这是横跨 transcript + conversation + workspace + interrupts + session 五个 domain 的 use-case，该和"开始一个 turn"住在同层（kernel）。

**关键 nuance**：`runNodes` / `wireBoundaryErr` / `openingUserInput` / `sessionToWire` 这些 helper 是**真正的 delivery 职责**（wire↔domain 翻译），留在 delivery。只搬编排序列。

#### 不动的部分

| 现状 | 从零不变 | 理由 |
|---|---|---|
| `kernel/port.go` | 同 | 窄 port 设计正确 |
| `kernel/turn/` | 同 | use-case 接口 + 实现放在 kernel 内是正确的 |
| `adapter/toolset/build.go` | 同 | 单一装配点 |
| `delivery/protocol/` | 同 | 冻结 wire 契约 |
| `delivery/dispatch/` | 同 | 表驱动路由 |
| `delivery/transport/` | 同 | HTTP/inprocess 双 transport |
| `delivery/server/translator.go` | 同 | wire↔domain 翻译层 |
| `delivery/server/runtime.go`（RuntimePort） | 同 | 消费方定义窄接口 |
| `internal/runtime/` | 同 | 组合根 |
| `domain/session` / `transcript` / `editguard` / `todo` | 同 | 充血模型正确 |
| 所有 `infra/` 包 | 同 | 技术设施位置正确 |

### 命名裁决：`kernel` / `domain` / `delivery` / `infra` vs `usecase` / `application` / `adapter`

| 现状 | 替代方案 | 谁更好 | 理由 |
|---|---|---|---|
| `kernel` | `usecase` / `orchestration` | **kernel** | 描述"微内核"的本质：定义 port + 驱动 loop。`usecase` 是通用术语，不传达这个特定架构模型 |
| `domain` | `entity` / `core` | **domain** | DDD 术语，准确描述"限界上下文" |
| `delivery` | `adapter` / `interface` / `api` | **delivery** | "交付"比"适配器"更形象 —— 这是把内核能力交付给外界的层 |
| `infra` | `adapter` / `driver` | **infra** | 短、明确、Go 社区通用 |
| `turn` | `chat` / `run` | **turn** | "turn"是 Lyra 的 ubiquitous language（前端 API.md 的 "turn" = 一次交互），比 "chat" 准 |

现状命名是 2026-06-14 重构后的成果（`engine→kernel` / `service→domain` / `rpc→delivery`），名字诚实、不 Java 味。**不需要再改名。**

---

## 5. 落地建议（按 impact/risk 排序）

### P0：现在做，低风险，高收益（做了 B+ → A）—— ✅ 两项均已于 2026-07-06 落地（见顶部状态更新）

| # | 改动 | 类型 | scope |
|---|---|---|---|
| **1** | **✅ DONE — 搬 rollback 编排 → kernel** | 结构性重构 | `delivery/server/rollback.go:RollbackSession` 的编排序列（Transcript.List → BoundaryAt → restoreCheckpoint → TruncateMessages → DeleteRun/Delete → purgeSubtasks）→ `kernel`（新包或 `kernel/turn/`）。delivery 只做 decode → 调 kernel 编排器 → present；wire helper（`runNodes` / `wireBoundaryErr` / `openingUserInput` / `sessionToWire`）留 delivery。**需咨询用户**（kernel 公开 API 变更）。 |
| **2** | **✅ DONE — 抽离 pump 的多服务协调 → kernel** | 结构性重构 | `delivery/server/pump.go:emit` 闭包里的 `persistStreamEvent` + `recordInterrupt` + `emitToolFileChange` + `snapshotCheckpoint` → kernel 层编排器（如 `RunSegmentCoordinator`）。pump 保留为 delivery 的流式管道（translator → hub.Append + eventId）。不改 pump 对外行为，只改内部调用链。**需咨询用户**（kernel 新接口）。 |

### P1：值得做，中等风险

| # | 改动 | 类型 | scope |
|---|---|---|---|
| **3** | **✅ DONE — `workspace` / `codeintel` 移出 domain** | 结构性重构 | 已移到 `internal/adapter/`；`domain ↛ infra` 规则现已打开且通过。 |
| **4** | **✅ DONE — 给纯贫血 domain 包加 package doc** | 文档改进 | `provider` / `interrupts` / `knowledge` / `run` 已加 package doc 标明形态。`approval` 从此列移除 —— 已长成充血策略模型（见 §2.4），不在贫血之列。 |

### P2：设计取向，等触发再做

| # | 改动 | 触发条件 |
|---|---|---|
| **5** | `kernel` port DTO 移到独立 `kernel/types` | 有第二个 port 实现方需要这些 DTO |
| **6** | `delivery/server` 拆成子包（`server/runs/` / `server/sessions/` 等） | 当 server 超过 ~6000 LOC 或出现需要单独测试的复杂 handler |

### 明确不建议做的（YAGNI 戒律）

| 不建议做 | 理由 |
|---|---|
| 加 Repository 接口层 | 现状 `Store`/`Registry` interface 已表达持久化/目录语义。改名为 Repository 是加字符的仪式 |
| 建独立的 Application Service 层 | `kernel/turn.Dispatcher` + `kernel.Engine` 就是应用层。再加一层是 1:1 影子 |
| 建 Domain Events 总线 | 单进程无异步 side-effect 需求。post-turn maintenance 的显式编排比事件总线清晰 10 倍 |
| 拆 `delivery/server` 的 protocol 方法（sessions.go / runs.go...） | 方法数 = 协议方法数，1:1 绑定是健康的。拆散才是低内聚 |
| 给 `toolset/build.go` 拆构造器 | 单一装配点是优点，不是缺点 |
| 给 domain 实体加 Aggregate Root 显式标记 | 单后端不需要。`Session.Fork()` 已经是聚合根行为，不需要 interface 来"声明" |
| `kernel` 改名 `usecase` | `kernel` 比 `usecase` 更准确（描述微内核本质）。改名降准确度 |

### 最大杠杆的两件事

**1. 搬 rollback 编排 → kernel（P0-1）**：让 delivery 回归"协议层薄"的承诺。这是一个用例（"用户请求回滚到某个 run 边界"），它应该和"用户请求开始一个 turn"住在同一层。

**2. 抽离 pump 的多服务协调 → kernel（P0-2）**：`pump.go:emit` 闭包里做了 persist + recordInterrupt + emitToolFileChange + snapshotCheckpoint，这些不是 delivery 的事。把它们抽成一个 kernel 层编排器，pump 只负责"事件 → translator → hub.Append"。

这两件事做了，`delivery/server` 就从"里面有 use-case"变成真正的"协议层薄"。架构等级从 B+ 进到 A。**✅ 二者均已于 2026-07-06 落地（rollback → `kernel/lifecycle.Coordinator`，pump → `kernel/runsegment` effects）。**

---

## 一句话收尾

**Lyra 的架构骨骼是教科书级别对的，别被"要往 DDD 走"的冲动带去加层 —— 项目哲学的反仪式立场（`DESIGN_PHILOSOPHY §2.5` + `REFACTORING §5.1`）对这个系统是对的。真正欠的债只有一处：`delivery/server` 里藏了 use-case 编排（pump + rollback）。~~把那两块搬进 kernel，就到 A。~~ ✅ 已于 2026-07-06 落地（rollback → `kernel/lifecycle.Coordinator`，pump → `kernel/runsegment` effects），现已到 A。**
