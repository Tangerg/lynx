# DESIGN_PHILOSOPHY.md — lynx 设计哲学 · 包设计规范 · 编码规范(的"为什么")

> 定位:[`CLAUDE.md`](CLAUDE.md) 给的是**速查红线**(强约定 / 反向不变量 / 重构策略),回答"能不能这么写";本篇给的是这些红线**背后的组织哲学**,回答"**该不该这么设计、为什么**"。设计新能力 / 新包 / 改公开 API 前,先用本篇的判断框架对一遍。
>
> 这套哲学不是一家之言 —— 它经**两个外部权威独立印证**:
> - **embabel-agent**(Kotlin/Spring,更老的 GOAP agent 框架)与 lynx **convergent design**,详见 [`agent/docs/EMBABEL_ORGANIZING_PRINCIPLES.md`](agent/docs/EMBABEL_ORGANIZING_PRINCIPLES.md);
> - **MCP Go SDK 设计文档**(Go 核心团队 + Anthropic 合写的官方 SDK)逐条命中同样的取舍。
>
> 模块内自举的活样本见 [`agent/docs/SELF_BOOTSTRAP.md`](agent/docs/SELF_BOOTSTRAP.md);消费者视角见 [`agent/docs/CONSUMER_FOOTPRINT.md`](agent/docs/CONSUMER_FOOTPRINT.md) / [`lyra/doc/AGENT_LEVERAGE.md`](lyra/doc/AGENT_LEVERAGE.md)。

---

## 0. 总纲(一句话)

> **薄而不可再分的核 + 一切能力归约到核 + 没有谁为某个能力重造基础设施。**

MCP Go SDK 把它写成需求清单的最后一条:

> "the SDK should be **minimal**. However, it should admit **extensibility using simple interfaces, middleware, or hooks**."

embabel 用"虚拟 Goal/Action/Condition 编译成普通 Agent"实现它;lynx 用"`workflow.*` 全部 `return core.NewAgent(...)`"实现它。三者同一条原则。

---

## 1. 能力的三种变体形态 + 试金石

加一个新能力时,它必然是核心的某种"再表达",而不是新机器。三种形态(按优先级):

| 形态 | 变的是什么 | 模式 | lynx 例子 |
|---|---|---|---|
| **① 参数化** | 一个参数/策略 | Strategy | sub-agent = 同一 agent − 一个 tool role;agent-tool = `newTypedAgentTool` + `processStarter` 策略;planner = `PlannerName` |
| **② 组合** | 把原语拼起来,编译回核 | Composition | `workflow.*`(全 `return core.NewAgent`);`autonomy` = `Agents`+`Ranker`+`RunAgent`+`GoalApprover` |
| **③ 装饰** | 包裹一个已有物 | Decorator | `toolpolicy`、`ToolDecorator`、`ActionMiddleware` |

**试金石(设计新能力时按顺序问):**
1. 能**拨一个钮**实现吗?→ ①(最好)
2. 拨不动,但能**拼现有原语**吗?→ ②(写个 shim 编译回核)
3. 都不行,只是**想包一层**吗?→ ③
4. **以上都套不上、需要为它新造一套 runtime/调度/状态载体?** → ⚠️ **设计错误信号**,先停下来问"这块是不是其实该在核里",而不是另起炉灶。

> 这条直接对应"组合大于继承",并在 Go 里推到极端:**组合大于继承,且库大于框架,且静态大于动态**(Go 无继承、lynx 无 DI 容器、用泛型而非反射)。

---

## 2. 包设计规范(Package Design)

### 2.1 内部:严格 DAG,不允许反向依赖
- 跨包依赖必须是单向无环的 DAG。**接口在消费方定义**(`chat.Engine` 在 chat 包内,`*engine.Engine` 隐式满足;`autonomy.platform` 窄接口不抱 `*runtime.Platform`)。
- **可机器验证**:`go list` 导出内部边,任何指向上层的边都是回归。agent 模块当前是干净 DAG(`core` → `planning`/`hitl` → `event`/`planner/*` → `runtime` → `autonomy`/`workflow`/`agent`)。
- **组合根集中装配具体实现**:执行核只依赖抽象,具体实现在组合根注入。范例:`runtime` 只依赖 `planning.Planner` 接口,goap/reactive 默认 planner 由组合根 `agent.NewPlatform` 注册(2026-06 重构)——这样加/删算法零波及 runtime,且所有算法一视同仁。

### 2.2 用户面:倾向"单一可发现门面"
- MCP Go SDK 把几乎整个用户 API 放进**一个 `mcp` 包**:

  > "a single package **aids discoverability**… avoids arbitrary decisions about package structure that may be **rendered inaccurate by future evolution**."

  类比 `net/http` / `net/rpc` / `grpc`。
- **lynx 的调和**:内部多包(§2.1 的分层 DAG,保留)≠ 用户面多包。常用类型应能从**一个门面包**(如顶层 `agent`)re-export,让常见场景 import 一个包而非五个。
- **YAGNI 闸**:门面化在出现**第二个外部消费者**时才做;只有一个消费者(如今天 lyra)时痛感不足,先记账。

### 2.3 一个扩展机制 优于 一堆 hook/SPI
- MCP Go SDK 用**一个** `Middleware func(MethodHandler) MethodHandler`,明确拒绝 mcp-go 的 **24 个 hook**("rarely used")。
- lynx 用**一个** `collectExtensions[T]` 类型分发(7 个子接口),对照 embabel 的 30+ 异质 Spring SPI。
- **规范**:需要可扩展时,优先"一个同质机制 + 类型/中间件分发",而不是"为每种扩展开一个具名插槽"。插槽多 = 表面积大 + 心智负担。

### 2.4 包大 ≠ god package
- **固有内聚的大包不强拆**。`runtime` 26 文件是因为它就是执行引擎;候选抽取要么破坏公开 API 换负收益(agent-tool 簇仍硬依赖 Platform/spawn),要么纯低价值整洁(blackboard impl)。
- 判据:抽出去能**真正切断耦合**且**不破坏公开 API**才抽;否则保留(2026-06 审计结论)。拆包信号见 [`CLAUDE.md`](CLAUDE.md) 的重构触发信号。

---

## 3. 编码规范(原则层 —— 具体红线见 [`CLAUDE.md`](CLAUDE.md) "Go idiom 纪律")

下面是**原则**;它们的强制版(`errors.New` / `%w` / 无 Java 味 / `atomic.Int32` / 字段分区 / OTel logging…)是 CLAUDE.md 的红线,本篇不复述,只给"为什么",并标注 MCP SDK 的同款印证。

| 原则 | 为什么 | 印证 |
|---|---|---|
| **options struct 优于 variadic `WithXxx`** | 更可读、包文档更简单;加字段不破坏 | MCP SDK 显式弃用 variadic 选项(369) |
| **accept interfaces, return structs** | 入参最大兼容、返回值最大信息量;接口是边界,struct 是实现 | `Transport`/`Connection` 接口 + `CommandTransport{}` 具体 struct |
| **make zero values useful** | 少构造函数、少出错 | MCP transport 都是导出字段 struct,零值可用 |
| **`iter.Seq`/`iter.Seq2` 优于 channel** | 拉模型、ctx 可在循环前检查、无 goroutine 泄漏 | MCP SDK 分页用 `iter.Seq2[T,error]` |
| **藏协议/传输细节,业务看不到 JSON-RPC** | "hide JSON-RPC details when not relevant to business logic" | lyra transport 同构;MCP SDK 把 jsonrpc2 设为 internal |
| **最小接口** | "the bigger the interface, the weaker the abstraction";低层接口更易实现 | MCP `Transport` 只 `Connect`;`Connection` 只 `Read/Write/Close` |
| **错误分层** | 协议错误带 code、工具错误进 result 不进 Go error | MCP:`JSONRPCError{Code}` + tool error in result |
| **核无状态,差异作参数** | 同一核服务多连接/多会话,per-session 靠工厂注入 | MCP n:1 Server + `getServer` 工厂;lyra session-as-param |

---

## 4. 生命周期视角:现在"破坏式改",稳定后才上"永不破坏"技巧

- **lynx 当前 pre-1.0**:公开 API 可调,**破坏性改动咨询后直接换**,不写 legacy 兼容(见 CLAUDE.md)。所以以下"永不破坏"技巧**现在不需要**。
- **MCP SDK 是 post-stability**,它的 future-proof 技巧是 lynx **稳定锁 API 时的现成模板**:
  - **统一 spec-method 签名** `(ctx, *XxxParams) (*XxxResult, error)`,即使 Params 暂时多余也保留 —— spec 加字段不破坏调用方;
  - **`nil` 参数永远合法**(`session.Ping(ctx, nil)`);
  - options struct 加字段、单包避免重构。
- **规范**:lyra 协议层将来锁定 wire 契约时,采纳"统一签名 + nil 友好 + options struct"这套,把破坏性挡在 spec 演进之外。

---

## 5. 原则冲突时(裁决)

沿用 [`CLAUDE.md`](CLAUDE.md) "原则冲突时怎么办",并补两条本篇相关的:

- **YAGNI vs future-proof**:pre-1.0 倾向"现在干净地破坏";只有锁 API 后才值得上 §4 的 never-break 技巧。别在 pre-1.0 提前付永不破坏的复杂度。
- **可发现性(单门面)vs 低耦合(多包分层)**:二者不矛盾 —— 内部保持多包 DAG(低耦合),用户面用门面 re-export(可发现)。冲突只在"要不要现在就门面化",答案由消费者数量定(§2.2)。

---

## 6. 设计自检清单(新能力 / 新包 / 改公开 API 前)

- [ ] 这个能力能归约到核(`core.Agent` 跑在 `Platform` 上)吗?走 §1 试金石定形态(① / ② / ③),还是触发了"重造地基"信号?
- [ ] 引入的跨包依赖**没有**指向上层?(`go list` 验 DAG)
- [ ] 消费方依赖的是**窄接口**(自己定义)还是抱了具体类型整体?
- [ ] 扩展点是**一个同质机制**,还是又开了一个具名插槽?
- [ ] 配置用 **options struct**,不是 variadic / builder 链?
- [ ] 流式用 **iterator**,不是 channel?
- [ ] 公开 API 改动:pre-1.0 可破坏,但**已咨询用户**?锁定后是否需要 §4 的 future-proof 形态?
- [ ] 这是真 DRY 还是虚假 DRY?抽象会让"两段因不同原因独立演化的代码"被迫同步吗?(宁可重复)

---

## 一句话收尾

**lynx 的设计不是个人趣味,是一套被 embabel(convergent)和 Go 团队的 MCP SDK(authoritative)双重印证的组织原则:薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 库优于框架。** 写代码前用 §1 试金石与 §6 清单各过一遍,就不会偏。
