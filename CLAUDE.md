# CLAUDE.md — lynx monorepo 项目级上下文

> Go monorepo，13 个 sub-module 各自有 `go.mod`：`core` / `agent` / `models` / `vectorstores` / `tools` / `pkg` / `rag` / `chatmemory` / `documentreaders` / `mcp` / `a2a` / `otel` / `lyra`。每个模块的具体形态见各自的 `CLAUDE.md`，本文件只放**跨模块共用**的约定。

---

## 第一法则 —— 绝不为一时方便留历史债务

> **最高优先级，凌驾于本文件其余所有约定之上。**

**项目处于快速开发 / 试错阶段，没有历史债务、也没有外部兼容包袱** —— store schema、暴露的 API（exported 函数 / 类型 / 字段 / 协议 wire shape）、命名，**全都可以调整**。正因如此：

- ❌ **绝不为"少改几处 / 降低前期开发量 / 避免迁移 / 赶进度"留下任何历史债务** —— 迁就外部库或旧名字的命名、为兼容留的字段、推测性 shim、"以后再清"的 TODO，一律不留。
- ✅ **发现设计不对，就在源头改对**，不在错的设计上叠补丁。**现在改成本最低，往后只会更贵。**
- 命名 / shape 按**本质第一性**决定；参考业界（Anthropic / Codex 等）**只取思想、不作命名锚** —— 名字恰好相同，只因它在独立评估下最优，不为兼容或省迁移。
- **唯一允许背的"债"是"设计还没想清楚"本身**；绝不允许"明知更好、却为省事不改"。
- 这条**不豁免**"破坏性公开 API 改动须先咨询用户"的约定（见下）：**先咨询，但默认倾向改对、而非将就。**

---

## 一句话定位

`lynx` 是一套**面向 AI agent / RAG / LLM 集成的 Go 基础设施**：`core` 定义协议、`models` 适配 38 个 LLM provider、`vectorstores` 适配 27 个向量库、`tools` 提供工具集、`agent` 跑 planner 驱动的 agent runtime、`lyra` 是 in-house 的 backend runtime（实现 Lyra Runtime Protocol）。所有模块共享一套**设计原则 + 重构节奏 + Go idiom 纪律**，下文规约。

## 子模块导览

| 模块 | LOC | 角色 |
|---|---|---|
| [`core/`](core/CLAUDE.md) | ~13k | Document / Message / Store / CallHandler 协议层 |
| [`agent/`](agent/CLAUDE.md) | ~13k | Planner / Blackboard / Runtime / Workflow |
| [`models/`](models/CLAUDE.md) | ~15k | 38 个 LLM provider 适配器 |
| [`vectorstores/`](vectorstores/CLAUDE.md) | ~21k | 27 个向量 DB 后端 |
| [`tools/`](tools/CLAUDE.md) | ~7k | LLM-callable 工具集 |
| [`pkg/`](pkg/CLAUDE.md) | ~9k | Generics / 并发 / 流式 工具库 |
| [`rag/`](rag/CLAUDE.md) | ~1.6k | 5 阶段 RAG pipeline |
| [`chatmemory/`](chatmemory/CLAUDE.md) | ~1.6k | Chat history DB 后端 |
| [`mcp/`](mcp/CLAUDE.md) | ~1.3k | MCP client + server 桥 |
| `a2a/` | ~0.5k | A2A (Agent-to-Agent) client + server 桥（wraps a2a-go SDK） |
| [`documentreaders/`](documentreaders/CLAUDE.md) | ~0.7k | Markdown / HTML / PDF readers |
| [`otel/`](otel/CLAUDE.md) | ~0.3k | OTel dev exporter |
| [`lyra/`](lyra/CLAUDE.md) | ~10k | Lyra Runtime backend |

## 共用强约定（违反 = 回归）

- **Go 1.26.3 统一版本**：所有模块 `go.mod` 同步；用 `iter.Seq2` / `slices.*` / `maps.*` / `atomic.Int32` 等现代 stdlib
- **依赖接口，不依赖具体类型**：跨包消费一定走 interface，**接口在消费方定义**（不在被消费方）。如果一个新模块要拿到 `*Engine` / `*Platform` / `*Service` 整体，先停下来想能不能拆成只用的几个方法
- **ISP 切碎接口**：典型例子 `approval.Console` vs `approval.Gate`（lyra），`tool.ToolSource` 一方法接口（lyra）。消费者只 import 自己用的那侧
- **`errors.New` 优先于 `fmt.Errorf("constant")`**。`fmt.Errorf` 只在真要格式化时用，包装其他错误必须 `%w` 才能 `errors.Is/As`
- **没有 Java 味**：禁 `impl.go` 文件 / `Impl` / `Service` / `Manager` / `Helper` / `Handler` 这种空白后缀 / `GetX/SetX` getter / `NewBuilder().With().Build()` 链。文件名描述内容（`inmemory.go` / `engine.go` / `sqlite/session.go`），struct 名描述本质
- **现代 Go**：`atomic.Int32` / `atomic.Pointer[T]` / `sync.Map` 优先于自家 atomic wrapper；`slices.*` / `maps.*` 替代手写 loop；`iter.Seq2` 替代 channel-based 流
- **可观测性走 OTel 三驾马车，全部 sink 到 `log/slog`（2026-06，vendor-neutral）**：观测 = **Traces（span）+ Metrics（instrument）**,用全局 `otel.Tracer("lynx/...")` / `otel.Meter("lynx/...")`,零 DI。全局 provider 在 app startup（`lyra/cmd/lyra/observability.go::setupObservability`）一次绑到 `otel/slog` 的三个 exporter（`NewSpanExporter` / `NewMetricExporter` / `NewLogExporter`）+ W3C propagator。规约：
  - **不在业务代码里撒 `slog`**——一个事件该被观测,就开 span（带 attr）/ 记 metric,**不是**加一行 `slog.InfoContext`。Span 经 sink 渲染成日志行,已覆盖"Logs"那一驾。错误走 `span.RecordError` + `SetStatus(codes.Error)`。这条对**库和应用一视同仁**（lyra 也不撒）。
  - **Logs 仍是一等 OTel 信号**：`slog.Default` 经 contrib `otelslog` bridge → `LoggerProvider` → `NewLogExporter`。意义是**可替换性**——生产把 exporter 换 OTLP（→ Datadog 等）即把 span/metric/log 全导到云,业务零改;同时兜住 stdlib `log` 重定向与第三方 slog。**不是**邀请你到处 `slog.InfoContext`。
  - **attr key 去品牌**：semconv 优先（`gen_ai.*` / `db.*` / `rpc.method`），否则裸 domain（`run.*` / `agent.*` / `rag.*`），无 `lynx.*` / `lyra.*` 前缀。instrumentation scope 名（`otel.Tracer("lynx/lyra/...")`）保留 `lynx/` 路径——那是库标识不是数据。
  - **全链路**：trace_id 在入口（HTTP transport）生成（提 W3C traceparent → 开 server span），脱钩后台 goroutine 用 `context.WithoutCancel` 保住 span（不是 `context.Background()`）。
  - **例外**：(a) `otel/slog/` 子包本身是 OTel→slog 的 bridge;(b) 公开 API 接受 `slog.Level` 等 stdlib 类型作入参（如 `mcp.LogToClient`）;(c) `core/model/chat/middleware.NewSlogLogger` 是给用户的 optional convenience provider。
  - 详见 [`doc/OBSERVABILITY.md`](doc/OBSERVABILITY.md) + [`otel/CLAUDE.md`](otel/CLAUDE.md)。**旧规则"禁 `slog.Default()`"已废（slog 是 sink,blessed）;但"观测优先用 span 而非 slog 行"这条仍然成立。**
- **设计原则**（高内聚低耦合 / KISS / DRY / SOLID / YAGNI）—— 是判断标准，详见下方"## 设计原则"段
- **目前开发阶段，公开 API 可以调整**：不写 legacy 兼容代码、不写 migration、schema / exported type / 函数签名变了直接换；注释里不提"Legacy …"。**但任何破坏性公开 API 改动必须先咨询用户**（不只是大重构 —— 改一个 exported 函数签名 / 删一个 exported 类型 / 改 struct field 也算），列清楚 scope + 影响面 + 备选方案，等用户确认再动手。这条规则适用于**所有 sub-module**
- **加文档？先问** —— 每个 sub-module 已有 `CLAUDE.md`，本根级也已有。其他默认不写

## 共用强反向不变量（已知错的方向）

- ❌ **加 retry layer**：SDK 内部已有 retry 就够，不在 `pkg/retry` 引入 Transient / NonTransient 分类
- ❌ **structured output 自己开 converter 链**：`chat.JSONParser[T]` / `ListParser` / `MapParser` 已覆盖 spring-ai converter family，Reasoning 是 first-class
- ❌ **`DefaultOptions` 返回 `*Options` 指针**：必须返值（intentional immutability）
- ❌ **手写 `fmt.Errorf("xxx is nil")`**：换 `errors.New`。包装 err 一律 `%w`
- ❌ **新增模块直接 import `*Engine` / `*Platform` / `*Service` 整体**：定义自己包内的窄接口
- ❌ **接口里塞所有 method**：subscriber 只用 3 个、producer 用另外 2 个 —— 拆 ISP
- ❌ **复制公共类型**（典型：enum 双份）：留一份，import 一下
- ❌ **stub interface placeholder**（`// M5 wires this` 这种）：真要做时再定义；当前删
- ❌ **给 LLM provider 加 OAuth / token refresh**：用户填 API key、err 401 让 UI 提示重填

## 设计原则

判断"这段代码该不该这么写 / 这个 PR 该不该 merge"的硬尺子。每条都配 lynx 真实命中的例子。

> **本段是速查红线版。** 这些原则背后的**组织哲学 + 包设计规范 + 编码规范的"为什么"**(薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 库优于框架,经 embabel convergent design 与 Go 团队 MCP SDK 双重印证)见 [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md)。设计新能力 / 新包 / 改公开 API 前,先用它的 §1 试金石与 §6 自检清单过一遍。

### 高内聚低耦合（High Cohesion / Low Coupling）

- **高内聚** = 一个 package / struct 内的东西**为同一个目的服务**。`internal/service/session/` 全是会话生命周期、`agentdoc/` 全是 AGENTS.md 发现 —— 这就是高内聚
- **低耦合** = **应用层**跨包依赖**通过最小接口**而不是具体类型。lyra chat 服务依赖 `chat.Engine`（5 方法）不直接抱 `*engine.Engine`。（**SDK 库内部例外**：单实现依赖直接用具体类型，见 ISP 段「库 vs 应用」）
- **二者矛盾时**：宁可包多一点（更高内聚）也别让一个包横跨多个 domain；宁可接口多一点（更低耦合）也别让一个具体类型变成跨包枢纽
- ❌ 反例：`chat.New(*engine.Engine)` —— 一处改 engine API 整个 chat 测试都得重写

### SRP — Single Responsibility（单一职责）

- 一个 struct / 一个函数**只有一个变化的理由**。`compactor` 只在 compaction 算法变时改；`extractor` 只在 LYRA.md 提取规则变时改；`planner` 只在 plan 生成 prompt 变时改 —— engine 把它们组合，不自己实现
- 信号：struct field > 8 + method > 10 / 函数 > 80 行 / 文件 > 500 行 —— 通常 SRP 信号，考虑拆
- ❌ 反例：原 `ProcessContext` 把 chat client / guardrails / tool resolver / cancel hook 等 11 个字段混在一起 —— 分析后是 inherent complexity，用**字段分区**注释表达 SRP 而非物理拆分

### OCP — Open/Closed（开放扩展、关闭修改）

- **加新能力靠加新类型，不靠改老 dispatch loop**。`runtime.collectExtensions[T any]([]Extension)` 是范例：要加新 ToolDecorator / ActionMiddleware / GoalApprover —— 实现接口就自动被 dispatch 发现
- Go 里通常用 **interface + type assertion / generic 类型分发** 实现 OCP，不用继承
- ❌ 反例：想拆掉 `EarlyTerminationPolicy` 的 `Extension` embed —— 后来发现 embed 是承重的（`collectExtensions` 靠它），删了反而要新加注册路径，**OCP 反而被破坏** → audit 误判，保留

### LSP — Liskov Substitution（可替换）

- 实现一个接口就要**完整满足语义**，不能某些方法实现某些不实现 / 某些参数支持某些不支持
- Go 里用 `var _ Iface = (*Impl)(nil)` 编译期断言（如 lyra `var _ memory.Service = (*FileMemoryService)(nil)`）+ 测试用 stub 实现接口双重保证
- ❌ 反例：`Service.Update(ctx, x)` 实现里悄悄忽略 ctx —— 调用方 cancel 不生效。LSP 不只看签名，还看行为契约

### ISP — Interface Segregation（接口隔离）

- **接口里只放调用方真用的方法**。`approval.Console`（4 方法）给 client side；`approval.Gate`（2 方法）给 producer side；`Service = Console + Gate` union 给 runtime 装配
- Rob Pike: "The bigger the interface, the weaker the abstraction." 大接口逼实现方塞 stub、逼 caller 多了解不该知道的
- **库 vs 应用（关键）**：消费方窄接口是给**应用层**的（lyra：多实现 + 可测 + 跨模块边界）。**SDK 库内部**（agent）的**单实现**依赖直接用具体类型——抽窄接口是 YAGNI 仪式（按「单实现接口→内联」）。例：agent `autonomy` 已从 2 方法 `platform` 窄接口**内联回** `*runtime.Platform`。窄接口在库里只留给**公开 SPI**（`Planner`/`Ranker`/`Extension` 子接口）
- ❌ 反例：原 `approval.Service` 把 5 方法揉一个接口，chat 只用其中 2 个 —— 已拆 Console / Gate

### DIP — Dependency Inversion（依赖倒置）

- **高层模块依赖抽象，不依赖低层具体类型**。chat → `chat.Engine` interface（在 chat 包内定义）→ `*engine.Engine` 隐式满足。chat 包**不 import** engine 包的具体类型（除了共享 wire types）
- 接口定义**放消费方**，不放被消费方（典型 Go idiom）
- ❌ 反例（**应用层**）：lyra chat / tool 曾直接 import `*engine.Engine` 整体 —— 已改窄接口。（**SDK 库内部反而有意**直接用 `*runtime.Platform`：见 ISP 段「库 vs 应用」）

### DRY — Don't Repeat Yourself

- 同样的逻辑 / 类型 / 字符串出现 **3 次以上**才考虑抽象（小于 3 次抽象比重复更糟，参考 KISS）
- DRY 的目的是消除"改一处必须连改 N 处"的脆性，**不是单纯消除字符**。如果两段相似代码会因不同原因独立演化，**不要 DRY**（虚假 DRY 比重复更糟）
- ✅ 例：`config.MCPTransport` 和 `engine.MCPTransport` enum 双定义 → 合一份；`fmt.Errorf("constant")` 58 处 → `errors.New(...)`
- ❌ 反例：把所有 provider adapter 的 `requestHelper` 强行抽公共基类 —— SDK shape 差异 > 相似度，会成累赘

### KISS — Keep It Simple, Stupid

- **简单 > 巧妙**。读代码的人 90% 时间在维护，写代码的人 10% 时间在创造
- 信号：嵌套泛型 > 2 层 / 反射 / `interface{}` / type-switch 长尾 / 函数闭包嵌套 —— 通常都是"我可以这么写"但不该这么写
- ✅ 例：SDK 库内部不为单实现依赖抽窄接口（agent `autonomy` 直接持 `*runtime.Platform`，而非引入 2 方法 `platform` 接口）—— 少一层就少一层
- ❌ 反例：approval 的 `atomicMode{value int32}` 自家 wrapper —— `atomic.Int32` 一句话搞定（已删）

### YAGNI — You Aren't Gonna Need It

- **不为未来不存在的需求做抽象 / 留 hook / 加配置 / 留接口**。每个推测性"为以后扩展留的"位都是债
- 信号：`// TODO: M5 wires this` / `// stub for later` / `Service` interface 只有一个 impl 且不打算多 / 配置字段 default = 永远不改 —— **删 / 内联 / 删字段**
- ✅ 例：删 `trace.Service` 整个 stub 包；删 `Platform()` / `ChatAgent()` 零调用方 getter；`ServiceProvider` modernize 提案 —— 一看零 caller，整个 modernize 都不值得做
- 风险：YAGNI ≠ "永远不为未来打算"。**已经发生过 3 次的扩展需求 ≠ 推测**，那叫预见

### Go-specific 几条

- **Accept interfaces, return structs**：函数入参用接口（最大兼容性）、返回值用具体类型（最大信息量）。`chat.New(eng Engine, ...) Service` —— 入接口出接口是因为这是 service 边界
- **Make zero values useful**：struct 零值能用就别要构造函数。`bytes.Buffer{}` / `sync.Mutex{}` 都是范例
- **Composition over inheritance**：Go 没有继承，只有 embed + interface。embed 是 has-a 不是 is-a，慎用（容易让方法集变得难追）
- **Smaller interfaces are better**（Rob Pike）：1 方法的 interface 经常是最有用的。`io.Reader` / `io.Writer` / `tool.ToolSource`（1 方法）都是这条
- **接口在消费方定义**（不在被消费方）：chat 包定义 `Engine` interface 给自己用；engine 包**不主动 export** 任何"给 chat 用的接口"。这条跟 DIP 配对

### 原则冲突时怎么办

- **DRY vs 低耦合**：抽公共 helper 引入了不想要的跨包依赖时，**宁可重复**（虚假 DRY 比重复贵）
- **ISP vs KISS**：拆 2 个接口 vs 接口里多 1 个方法 —— 看调用方实际数。1 个 caller 用 5 方法别拆；3 个 caller 各用 2 方法该拆
- **YAGNI vs OCP**：留扩展点 vs 不为未来抽象 —— 扩展是不是已经发生过？发生过 = OCP（保留）；只是猜 = YAGNI（删）
- **判断标准**：永远倾向"现在这样写没问题，要扩展时改"而不是"现在多写一层，万一以后用得上"。**未来可以重构，过度抽象很难逆转**

## 重构策略（参考前端 CLAUDE.md，Go 化）

> **重构要求的泛化规范见 [`REFACTORING.md`](REFACTORING.md)** —— 命名 / 注释 / 指针vs值 / nil 守卫 / 自由函数vs方法 / 卫语句降圈复杂度 / 现代 Go / 就近原则组织 / Go idiom 硬规则 / 节奏纪律,全部抽象表述、跨模块通用。重构前先过一遍它。本段保留本仓特有的**两档节奏 + 触发信号 + Fowler 式清单**作为补充。

**重构是节奏，不是可选项 —— 分两档**：

- **小型重构（每 3-5 轮 feature）**：聚焦最近改动的那几个文件。扫一遍：
  - 单文件超 300 行没？
  - 局部 3+ 重复 pattern？
  - 最近加的注释里有 what-说明可删？
  - 最近 rename 漂移没（两个名字指同一个东西）？
  - 最近新增的 exported API 破坏既有抽象没？
  - **产出**：抽 1-2 个 helper / 删几条死注释 / 精修 1-2 个名字 / 改 1 个文件的字段分区 —— **净变化 < 100 LOC、touch < 5 文件**
  - **不需要咨询用户**（除非碰到破坏性公开 API），直接做完跑 `go build && go vet && go test ./...` 全绿 commit

- **大型重构（每 15-20 轮 feature）**：跨整个模块扫，参考 lyra A-E / agent A-D 范例
  - 跑 `go vet ./...` + `staticcheck ./...`（如装了）找未引用的 exports / dead branches
  - 找 > 500 行的文件考虑拆 SRP
  - 找跨包的 3+ 重复（不只是局部）—— typical 例子：MCP enum 双定义、`fmt.Errorf("constant")` 散落
  - 找 god struct（field > 8 + method > 10）考虑组合化
  - 找具体类型跨包暴露 —— 是不是应该收窄成 interface
  - 考虑是否要拆 / 合 package
  - **产出**：multi-batch 重构计划（A/B/C/D/E），用户确认后逐批 commit；每批之间跑 `go build && go vet && go test ./...` 全绿

- **共同做法**（lyra / agent 重构 session 验证过）：
  - 上来**先深度审计**（grep / Explore agent / 读文件），不要直接动手
  - 把发现分类（Java-isms / coupling / cohesion / SOLID / DRY / 命名 / 现代 Go），按 impact 排序
  - 给 3-5 项候选 batch + 每项的"动 vs 不动"权衡
  - **等用户确认再动**，每批一个 commit、可独立 revert
  - 重构跑完**承认 audit 过度 call 的项**（lyra C+E、agent C+E 都最终 skip 了，因为深入看发现 audit 误判）—— 这是正常 false positive，不是失败
  - 每批 commit message 写清"why"，把 audit 的发现 + skip 理由都记下来

- **目的**：小型重构防局部熵增（每个文件不至于失控），大型重构防架构熵增（整体不至于尾大不掉）

- **触发信号**（任何一项命中就该考虑）：
  - 单文件 > 500 行（god file）
  - 同 struct field > 8 + method > 10（god object）
  - 一个 type 的方法被多个包消费、但消费方各自只用 2-3 个（→ 该收窄成 interface）
  - `addXxx / removeXxx` 或 `getXxx / setXxx` 模式 > 3 处未抽象
  - 命名漂移（两个名字指同一个东西，或一个名字指两个东西）
  - 最近 commit 里有反复改同一段代码（说明抽象方向错了）
  - 加新 feature 需要改多个文件的同一类样板代码（说明缺一层抽象）
  - `// TODO: M5 wires this` / `// stub for later` 这种推测性占位（YAGNI 信号）

**重构清单**（Fowler《重构》的 Go 实践版，每轮扫的不止拆分还包含）：

- **(a) 死代码清理** —— 跑 `go vet` + `staticcheck`（如装了）+ 全文 grep 一遍 exported 符号的调用方。零调用方 = 删，**不留"将来可能用"**（`ServiceProvider`、`Platform()` getter 都是这条命中删的）
- **(b) 卫语句替代嵌套 if** —— `if !ok { return }` 比层层缩进的 if/else 链可读 10 倍
- **(c) 查表法替代条件链** —— 3+ `switch case` 或嵌套 `if/else if` 通常应该是 `map[K]V` 查表（除非用 generic 类型分发，如 `collectExtensions[T]`）
- **(d) 精准命名** —— `idCounter` → `nextCompositeKeyId`；`x` / `tmp` / `data` / `result` / `obj` 不算名字；文件名 `impl.go` 是 Java 味（→ `inmemory.go` / `engine.go` / `sqlite.go`）
- **(e) 注释清理** —— 大段解释 what 的删（代码自身说明）；过期的迁移注释删（"Legacy …" 这种）；误导性的"为什么这样写"（实际不再这样了）删。留下来的只解释 _why_ 而不是 _what_。`// M5 wires this` 这种推测性占位删
- **(f) 现代 Go 扫描** —— `sync/atomic` 用 `atomic.Int32` 不用手写 wrapper / `sync.Map` 替代 `mutex + map`（适合 write-rare）/ `slices.*` `maps.*` helper 替代手写 loop / `iter.Seq2` 替代 channel-based 流 / `errors.New("...")` 替代 `fmt.Errorf("constant")` / `%w` 包装错误
- **(g) 接口收窄扫描** —— **仅应用层 / 多实现**：跨模块传递的具体类型（lyra `*engine.Engine` 等），消费方只用几个方法就收窄成自定义窄接口。**SDK 库内部单实现依赖不收窄**（YAGNI，见设计原则 ISP「库 vs 应用」；agent `autonomy` 已把 `platform` 窄接口内联回 `*runtime.Platform`）
- **(h) 性能扫描** —— 热路径上 `sync.RWMutex` vs `sync.Map` 是否合适？循环里有 N² `slices.Index` / `Contains`？大 struct 是 copy 还是 pointer 传递？SSE / stream 路径有没有 buffering 把 flush 搞砸？

## Go idiom 纪律（写代码 / review 时必看）

近期重构沉淀的硬规则：

- **错误构造**：`fmt.Errorf("constant string")` 是浪费 → `errors.New("...")`。错误包装一律 `%w`，没有 `%v`
- **接口在消费方定义**：典型例子 `chat.Engine` 定义在 chat 包内，`*engine.Engine` 隐式满足。被消费的具体类型**不主动暴露接口**给消费者 import —— 消费者自己写
- **应用层测试用 stub 接口**（多实现 + 隔离），编译期断言 `var _ Iface = (*Impl)(nil)` 防接口漂移。**SDK 库内部用真实具体依赖测**——agent `autonomy` 测试直接构造 `*runtime.Platform`，不为测试抽窄接口
- **`impl.go` 是 Java 味**：实现文件按本质命名 —— `inmemory.go`（单进程内存实现）/ `engine.go`（engine-backed）/ `sqlite/session.go`（特定 backend）
- **`atomic.Int32` 直接用**，别再包一层 `atomicXxx` wrapper（`Store(int32(v))` / `Load()` 就够）
- **结构体字段超过 6 个**：用注释 `// --- xxx ---` 分区。读起来一目了然
- **跨包用 generic 类型分发**：`runtime.collectExtensions[T any]([]Extension)` 是 lynx codebase 的核心 pattern。需要类型路由的场景优先用 generic，不用 type switch
- **dead code 立刻删**：发现 `Platform()` 这种零调用方的 exported getter —— 删。哪天真需要时再加，那时已经知道签名该长什么样
- **`sync.Map` 适合 write-once / read-many**：`FileMessageStore` 的 per-conversation lock 是典型场景。不适合 write-heavy
- **不要测一个具体类型有没有实现接口**：编译期断言 `var _ Service = (*inMemory)(nil)` 比运行时检查好

## 沟通约定

- **中文回复**（用户偏好）
- 代码 / 注释保持英文
- 大重构前先给批次方案 + 权衡，等用户确认再动；每批一个 commit，可独立 revert
- **公开 API 改动前先咨询用户**（exported 函数 / 类型 / 字段签名变化，跨包消费者会受影响）—— dev 阶段允许改，但不允许"擅自"改。列出 scope + 影响面 + 备选方案，等"动"再动
- 改动后跑 `go build && go vet && go test ./...` 全绿才 commit
- commit message 写清"why"而不仅是"what"
- commit trailer 用 `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`
