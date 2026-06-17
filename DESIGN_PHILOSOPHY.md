# DESIGN_PHILOSOPHY.md — lynx 设计哲学（的"为什么"）

> 定位：lynx 的设计文档分三层，各回答一个问题 ——
> - [`CLAUDE.md`](CLAUDE.md)：**速查红线**（法则 / 反向不变量 / 触发信号）—— "**能不能**这么写"；
> - [`REFACTORING.md`](REFACTORING.md)：**重构标尺**（命名 / 注释 / 指针vs值 / nil 守卫 / 自由函数vs方法 / 卫语句 / 就近组织 / 节奏）—— 重构时"**改什么、怎么改**"；
> - **本篇**：这些红线**背后的组织哲学** —— "**该不该**这么设计、为什么"。
>
> 设计新能力 / 新包 / 改公开 API 前，先用本篇的判断框架对一遍；落手重构时对 [`REFACTORING.md`](REFACTORING.md)。
>
> 本篇只写**原则**，不绑具体实现（具体案例随演化变动，活在代码 / git 里）。这套哲学不是一家之言 —— 它经**两个外部权威独立印证**：与更老的 GOAP agent 框架 **embabel-agent** 形成 convergent design；又逐条命中 **MCP Go SDK 设计文档**（Go 核心团队 + Anthropic 合写）的同款取舍。

---

## 0. 总纲（一句话）

> **薄而不可再分的核 + 一切能力归约到核 + 没有谁为某个能力重造基础设施。**

MCP Go SDK 把它写成需求清单的最后一条："the SDK should be **minimal**. However, it should admit **extensibility using simple interfaces, middleware, or hooks**." —— 最小核 + 简单扩展点，而非为每种能力造新机器。

---

## 1. 能力的三种变体形态 + 试金石

加一个新能力时，它必然是核心的某种"再表达"，而不是新机器。三种形态（按优先级）：

| 形态 | 变的是什么 | 模式 |
|---|---|---|
| **① 参数化** | 一个参数 / 策略 | Strategy |
| **② 组合** | 把原语拼起来，编译回核 | Composition |
| **③ 装饰** | 包裹一个已有物 | Decorator |

**试金石（设计新能力时按顺序问）：**
1. 能**拨一个钮**实现吗？→ ①（最好）
2. 拨不动，但能**拼现有原语**吗？→ ②（写个 shim 编译回核）
3. 都不行，只是**想包一层**吗？→ ③
4. **以上都套不上、需要为它新造一套 runtime / 调度 / 状态载体？** → ⚠️ **设计错误信号**：先停下来问"这块是不是其实该在核里"，而不是另起炉灶。

> 这条对应"组合大于继承"，并在 Go 里推到极端：**组合大于继承，且库大于框架，且静态大于动态**（Go 无继承、无 DI 容器、用泛型而非反射）。

---

## 2. 包设计规范（Package Design）

### 2.1 内部：严格 DAG，不允许反向依赖
- 跨包依赖必须是单向无环的 DAG。**接口在消费方定义**（消费方包内声明窄接口，被消费的具体类型隐式满足，不主动 export"给你用的接口"）。
- **可机器验证**：导出内部依赖边，任何指向上层的边都是回归。
- **组合根集中装配具体实现**：执行核只依赖抽象，具体实现在组合根注入 —— 加 / 删一个具体实现对核零波及，且所有实现一视同仁。

### 2.2 用户面：倾向"单一可发现门面"
- 把常用 API 收进**一个门面包**提升可发现性，避免"按未来可能失准的结构武断分包"（类比 `net/http` / `grpc`）。
- **lynx 的调和**：内部多包（§2.1 的分层 DAG，保留）≠ 用户面多包。常用类型从一个门面包 re-export，让常见场景 import 一个包而非五个。
- **YAGNI 闸**：门面化在出现**第二个外部消费者**时才做；只有一个消费者时痛感不足，先记账。

### 2.3 一个扩展机制 优于 一堆 hook / SPI
- 优先"**一个**同质机制 + 类型 / 中间件分发"（如一个 `Middleware func(Handler) Handler`、一个泛型类型分发器），而不是"为每种扩展开一个具名插槽"。
- 插槽多 = 表面积大 + 心智负担。MCP Go SDK 用一个 middleware、明确拒绝几十个 rarely-used hook，是同一取舍。

### 2.4 包大 ≠ god package
- **固有内聚的大包不强拆**（执行引擎、解析器族这类天然就大）。判据：抽出去能**真正切断耦合**且**不破坏公开 API**才抽；否则保留 —— 为整洁而拆是负收益。拆包信号见 [`CLAUDE.md`](CLAUDE.md) / [`REFACTORING.md`](REFACTORING.md) 的触发信号。

---

## 3. 编码规范（原则层 —— 强制红线见 [`CLAUDE.md`](CLAUDE.md)，落手细则见 [`REFACTORING.md`](REFACTORING.md)）

下面是**原则**（回答"为什么"），跨模块通用。强制红线（`errors.New` / `%w` / 无 Java 味 / 现代 Go / OTel logging…）在 CLAUDE.md；重构时怎么改成这样在 REFACTORING.md。本篇不复述，只给"为什么"，并标注与 MCP SDK 的同款印证。

| 原则 | 为什么 |
|---|---|
| **options struct 优于 variadic `WithXxx` / builder 链** | 更可读、文档更简单；加字段不破坏调用方。 |
| **accept interfaces, return structs** | 入参最大兼容、返回值最大信息量；接口是边界、struct 是实现。 |
| **make zero values useful** | 少构造函数、少出错；导出字段 struct 零值可用。 |
| **`iter.Seq` / `iter.Seq2` 优于 channel** | 拉模型、ctx 可在循环前检查、无 goroutine 泄漏。 |
| **藏协议 / 传输细节，业务看不到 wire 形态** | 业务逻辑不该感知 JSON-RPC 等传输；envelope I/O 与业务解耦。 |
| **最小接口** | "the bigger the interface, the weaker the abstraction"；低层接口更易实现、更易替换。 |
| **错误分层** | 协议错误带 code、工具 / 业务错误进 result 不进 Go error。 |
| **核无状态，差异作参数** | 同一核服务多连接 / 多会话，per-session 靠工厂 / 参数注入，不为每会话造实例。 |

---

## 4. 生命周期视角：现在"破坏式改"，稳定后才上"永不破坏"技巧

- **lynx 当前 pre-1.0**：公开 API 可调，破坏性改动**咨询后直接换**、不写 legacy 兼容（见 CLAUDE.md）。所以下列"永不破坏"技巧**现在不需要**。
- **稳定锁 API 时的现成模板**（post-stability 的 future-proof 套路）：
  - 统一 spec-method 签名 `(ctx, *XxxParams) (*XxxResult, error)`，即使 Params 暂时多余也保留 —— spec 加字段不破坏调用方；
  - `nil` 参数永远合法；
  - options struct 加字段、单包避免重构。
- **规范**：协议层将来锁定 wire 契约时，采纳"统一签名 + nil 友好 + options struct"，把破坏性挡在 spec 演进之外。别在 pre-1.0 提前付永不破坏的复杂度。

---

## 5. 原则冲突时（裁决）

沿用 [`CLAUDE.md`](CLAUDE.md) "原则冲突时"，并补两条本篇相关的：

- **YAGNI vs future-proof**：pre-1.0 倾向"现在干净地破坏"；只有锁 API 后才值得上 §4 的 never-break 技巧。
- **可发现性（单门面）vs 低耦合（多包分层）**：二者不矛盾 —— 内部保持多包 DAG（低耦合），用户面用门面 re-export（可发现）。冲突只在"要不要现在就门面化"，答案由消费者数量定（§2.2）。

---

## 6. 设计自检清单（新能力 / 新包 / 改公开 API 前）

> 本节是**设计新东西**时的自检；**重构既有代码**另有清单（命名 / 注释 / 指针vs值 / nil 守卫 / 卫语句 / 就近组织 / 节奏），见 [`REFACTORING.md`](REFACTORING.md)。

- [ ] 这个能力能**归约到核**吗？走 §1 试金石定形态（① / ② / ③），还是触发了"重造地基"信号？
- [ ] 引入的跨包依赖**没有**指向上层？（DAG 不破）
- [ ] 消费方依赖的是**窄接口**（自己定义）还是抱了具体类型整体？
- [ ] 扩展点是**一个同质机制**，还是又开了一个具名插槽？
- [ ] 配置用 **options struct**，不是 variadic / builder 链？
- [ ] 流式用 **iterator**，不是 channel？
- [ ] 公开 API 改动：pre-1.0 可破坏，但**已咨询用户**？锁定后是否需要 §4 的 future-proof 形态？
- [ ] 这是真 DRY 还是虚假 DRY？抽象会让"两段因不同原因独立演化的代码"被迫同步吗？（宁可重复）

---

## 一句话收尾

**lynx 的设计不是个人趣味，是一套被 embabel（convergent）和 Go 团队 MCP SDK（authoritative）双重印证的组织原则：薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 库优于框架。** 设计前用 §1 试金石与 §6 清单各过一遍（回答"该不该"），重构时对 [`REFACTORING.md`](REFACTORING.md)（回答"怎么改"），就不会偏。
