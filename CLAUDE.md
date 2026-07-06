# CLAUDE.md — lynx monorepo 项目级上下文

> Go monorepo，多个 sub-module 各自有 `go.mod`。本文件只放**跨模块共用的法则 —— 只宏观、不写具体**（具体案例、符号、文件名活在代码 / git / 各模块自己的 `CLAUDE.md` 里，会随演化变动，不进本则）。设计哲学的"为什么"见 [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md)，落手重构的标尺见 [`REFACTORING.md`](REFACTORING.md)。

---

## 第一法则 —— 绝不为一时方便留历史债务

> **最高优先级，凌驾于本文件其余所有约定之上。**

项目处于快速开发 / 试错阶段，没有历史债务、也没有外部兼容包袱 —— store schema、暴露的 API（exported 函数 / 类型 / 字段 / 协议 wire shape）、命名，全都可以调整。正因如此：

- ❌ **绝不为"少改几处 / 降低前期开发量 / 避免迁移 / 赶进度"留下任何历史债务** —— 迁就外部库或旧名字的命名、为兼容留的字段、推测性 shim、"以后再清"的 TODO，一律不留。
- ✅ **发现设计不对，就在源头改对**，不在错的设计上叠补丁。**现在改成本最低，往后只会更贵。**
- 命名 / shape 按**本质第一性**决定；参考业界**只取思想、不作命名锚** —— 名字恰好相同，只因它在独立评估下最优，不为兼容或省迁移。
- **唯一允许背的"债"是"设计还没想清楚"本身**；绝不允许"明知更好、却为省事不改"。
- 这条**不豁免**"破坏性公开 API 改动须先咨询用户"的约定：**先咨询，但默认倾向改对、而非将就。**

---

## 第二法则 —— 修理问题必须治本，绝不治标

> **与第一法则并列的最高优先级。** 第一法则管"别欠债"，第二法则管"修就修到根"。

修任何 bug / 问题，都在它的**根因和正确的层**上修，绝不在症状点打补丁、绝不 hacky。一个 workaround 让现象消失但根因还在 —— 那不是修复，是把债（第一法则）藏进"看起来修好了"的假象里，下一个相邻 bug 会从同一个根因再冒出来。

- ❌ **绝不治标**：症状点加 if 绕过、只在消费侧 workaround 而留源头不动、"加行日志 / 别吞错误就算修了"、reactive 地 coerce / retry / 兜底去掩盖上游的错误状态、把不变量交给"调用方记得别犯错"。
- ✅ **治本**：找到根因落在哪一层，就在那一层改对。根因常在更下 / 更内的层（SDK 原语、接口契约、store schema），治本往往要动公开 API 或 schema —— 这正是第一法则允许的；**先按"破坏性公开 API 改动须先咨询"咨询，但默认倾向治本、而非在上层将就。**
- **判据一句话**：问"根因消除了吗，还是只是这个现象不出现了？"—— 只让现象消失的，是治标，打回重修。

---

## 定位与导览

`lynx` 是一套面向 AI agent / RAG / LLM 集成的 Go 基础设施：`core` 定协议、`models` 适配 LLM provider、`vectorstores` 适配向量库、`tools` 提供工具集、`agent` 跑 planner 驱动的 runtime、`lyra` 是 in-house backend runtime；其余模块（`pkg` / `rag` / `chathistory` / `mcp` / `a2a` / `skills` / `documentreaders` / `otel`）各司一域。每个 sub-module 的形态、关键类型、模块特有反向不变量见其自己的 `CLAUDE.md`。所有模块共享下文这套法则。

---

## 共用强约定（违反 = 回归）

- **统一 Go 版本**：所有模块 `go.mod` 同步；用当前版本的现代 stdlib（`iter.Seq2` / `slices.*` / `maps.*` / 类型化 atomic 等）。
- **依赖接口、不依赖具体类型**：跨包消费走 interface，且**接口在消费方定义**（不在被消费方）。要拿整个 `*Engine` / `*Platform` / `*Service` 时，先停下来想能不能只依赖用到的那几个方法。
- **ISP 切碎接口**：接口只放调用方真用的方法；胖接口拆成按需组合的子接口，装配处 union、消费者各依赖自己那片。
- **错误**：`errors.New` 优先于 `fmt.Errorf("常量")`；`fmt.Errorf` 只在真要格式化时用，包装错误一律 `%w`（才能 `errors.Is/As`）。
- **没有 Java 味**：禁空白后缀类型名（`Impl` / `Service` / `Manager` / `Helper` / `Handler`）、禁泛文件名（`impl.go` 等）、禁 `GetX/SetX` getter、禁 builder 链。文件名描述内容、struct 名描述本质。
- **现代 Go**：类型化 atomic / `sync.Map`（write-rare）优先于自家 wrapper；`slices.*` / `maps.*` 替手写 loop；`iter.Seq2` 替 channel 流。
- **可观测性 = OTel 三驾马车，sink 到 `log/slog`（vendor-neutral）**：观测 = Traces（span）+ Metrics（instrument）+ Logs，用全局 `otel.Tracer` / `otel.Meter`、零 DI，组合根在启动时一次性绑定 exporter + W3C propagator。规约：
  - **不在业务代码里撒 `slog`** —— 一个事件该被观测，就开 span（带 attr）/ 记 metric，而非加一行日志行；错误走 `span.RecordError` + `SetStatus`。库和应用一视同仁。
  - **Logs 仍是一等 OTel 信号**（slog 经 bridge 进 LoggerProvider）—— 意义是**可替换性**（生产换 OTLP exporter 即把 span/metric/log 全导云端、业务零改），不是邀请到处写日志。
  - **attr key 去品牌**：semconv 优先，否则裸 domain 名，无项目前缀（instrumentation scope 名保留库路径 —— 那是库标识不是数据）。
  - **全链路**：trace_id 在入口生成，脱钩的后台 goroutine 用 `context.WithoutCancel` 保住 span（不是 `context.Background()`）。
  - 细节见 [`doc/OBSERVABILITY.md`](doc/OBSERVABILITY.md) + [`otel/CLAUDE.md`](otel/CLAUDE.md)。
- **设计原则**（高内聚低耦合 / SOLID / DRY / KISS / YAGNI）见下「设计原则」段 —— 是判断标准，不是口号。
- **公开 API 可调、但不可擅自调**：dev 阶段不写 legacy 兼容 / 不写 migration、schema·exported type·签名变了直接换、注释不留"Legacy …"；**但任何破坏性公开 API 改动（含改一个签名 / 删一个类型 / 改一个字段）必须先咨询用户**，列清 scope + 影响面 + 备选方案，等确认再动。适用所有 sub-module。
- **加文档先问**：每个 sub-module 已有 `CLAUDE.md`，其他默认不写。

## 共用强反向不变量（已知错的方向）

- ❌ 加 retry layer / Transient·NonTransient 分类 —— SDK 内部已有 retry 就够。
- ❌ structured output 另起 converter 链 —— 既有 parser family 已覆盖，Reasoning 是 first-class。
- ❌ 让本应返值的不可变构造器返指针。
- ❌ 手写 `fmt.Errorf("xxx is nil")` —— 用 `errors.New`；包装一律 `%w`。
- ❌ 新模块直接 import 整个 `*Engine` / `*Platform` / `*Service` —— 定义自己包内的窄接口。
- ❌ 胖接口塞所有方法 —— 按消费者拆 ISP。
- ❌ 复制公共类型（典型 enum 双份）—— 留一份 import。
- ❌ stub interface / 推测性占位（"以后接"）—— 真要做时再定义。
- ❌ 给 LLM provider 加 OAuth / token refresh —— 用户填 key，401 让 UI 提示重填。

## 设计原则

判断"这段代码该不该这么写 / 这个 PR 该不该 merge"的硬尺子。**背后的"为什么"（薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 库优于框架）见 [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md)；落手重构的标尺见 [`REFACTORING.md`](REFACTORING.md)。**

- **高内聚低耦合**：一个 package / struct 内的东西为同一个目的服务（高内聚）；应用层跨包依赖走最小接口而非具体类型（低耦合）。二者矛盾时宁可包多一点、接口多一点，也别让一个包横跨多个 domain、或一个具体类型变成跨包枢纽。
- **SRP（单一职责）**：一个 struct / 函数只有一个变化的理由。信号：字段过多 + 方法过多 / 函数过长 / 文件过大 —— 通常是 SRP 信号；但先判断是不是固有复杂度（是则用字段分区注释表达，而非硬拆）。
- **OCP（开放扩展、关闭修改）**：加新能力靠加新类型，不靠改老 dispatch loop。Go 里用 interface + 类型断言 / 泛型类型分发实现，不用继承。
- **LSP（可替换）**：实现一个接口就要完整满足其语义与**行为契约**（不只是签名），不能某些方法 / 参数实现某些不实现。用编译期断言 + stub 双重保证。
- **ISP（接口隔离）**：接口只放调用方真用的方法。**库 vs 应用（关键）**：消费方窄接口是给**应用层**（多实现 + 可测 + 跨模块边界）的；**SDK 库内部单实现依赖直接用具体类型** —— 抽窄接口是 YAGNI 仪式。窄接口在库里只留给公开 SPI。
- **DIP（依赖倒置）**：高层依赖抽象、不依赖低层具体类型；接口定义放消费方，不放被消费方。
- **DRY**：同样的逻辑 / 类型 / 字符串出现 **3 次以上**才考虑抽象（更少时抽象比重复糟）。DRY 是消除"改一处要连改 N 处"的脆性，不是单纯消字符 —— 会因不同原因独立演化的相似代码**不要 DRY**（虚假 DRY 比重复贵）。
- **KISS**：简单 > 巧妙（维护占 90% 时间）。信号：嵌套泛型 > 2 层 / 反射 / `interface{}` / type-switch 长尾 / 闭包深嵌 —— 常是"能写"但不该写。
- **YAGNI**：不为不存在的需求做抽象 / 留 hook / 加配置 / 留接口。信号：推测性占位、单实现且不打算多的接口、永不改的默认字段 —— 删 / 内联。但 YAGNI ≠ 永不为未来打算：**已发生过多次的扩展是预见、不是推测**。
- **Go-specific**：accept interfaces, return structs；make zero values useful；composition over inheritance（embed 是 has-a，慎用）；smaller interfaces are better（1 方法接口常最有用）；接口在消费方定义。

**原则冲突时**：DRY vs 低耦合 —— 抽 helper 引入不想要的跨包依赖时宁可重复；ISP vs KISS —— 看调用方实际数（1 个 caller 用全部方法别拆，多个 caller 各用一片该拆）；YAGNI vs OCP —— 扩展是否已真实发生过（发生过 = 保留扩展点，只是猜 = 删）。**判断基线**：永远倾向"现在这样写没问题、要扩展时再改"，而非"现在多写一层、万一以后用得上" —— 未来可重构，过度抽象难逆转。

## 重构

**重构是节奏，不是可选项**，分两档：

- **小型（每几轮 feature）**：聚焦最近改动的文件，扫超长文件 / 局部重复 / 死注释 / 命名漂移 / 新增 exported API 是否破坏既有抽象 —— 产出小（净变化小、touch 少）；无破坏性公开 API 时直接做完跑全绿 commit。
- **大型（每十几~二十轮 feature）**：跨模块扫死代码 / 超大文件拆 SRP / 跨包重复 / god struct / 具体类型跨包暴露该不该收窄 / 包拆合 —— 产出 multi-batch 计划，用户确认后逐批 commit、每批之间全绿。

**目的**：小型防局部熵增（每个文件不失控），大型防架构熵增（整体不尾大不掉）。**触发信号、Fowler 式重构清单（死代码 / 卫语句 / 查表 / 接口收窄 / 性能扫描…）、命名·注释·指针vs值·nil 守卫·就近组织·节奏纪律的完整标尺见 [`REFACTORING.md`](REFACTORING.md)** —— 重构前先过一遍。

## 注释纪律：轻易不要写注释

优先让代码通过**命名、结构、抽象**自解释；想写注释先问"能不能改名 / 抽函数 / 调结构让它不言自明"。注释是表达力用尽后的**最后手段**。只在以下情况写（写的是代码无法表达的信息）：

1. **接口定义（godoc）**：语义、参数、返回值、error 语义 / sentinel、副作用与调用约束、并发安全 —— exported 符号的 doc comment 是公开契约；unexported 才适用"轻易不写"。
2. **特殊约定**：业务规则、历史原因、兼容要求、外部系统约束（协议条款、第三方 SDK 怪癖）—— 不在代码里、只在上下文里。
3. **特殊算法**：思路、边界条件、复杂度、非显然优化。
4. **反直觉实现**：为什么**不能**用更常见更简单的写法（防下一个人"好心"改回去）。
5. **并发 / 事务 / 安全约束**：goroutine 所有权与生命周期、锁持有顺序、channel 关闭方、ctx 取消语义、信任边界 —— 违反不报编译错、只在生产炸。

- **判据一句话**：注释只写 _why_ 与_约束_，不写 _what_ 与 _how_ —— 复述代码的注释在代码变更时必然腐烂成误导。
- 改代码必同步改注释，宁可删不留过期；注释对**下一个读者**说话，不对 reviewer 说话（"此处修复 bug X" / "按 review 调整"一律不写）。

## 沟通约定

- **中文回复**（用户偏好）；代码 / 注释保持英文。
- 破坏性或结构性改动前先给 scope + 影响面 + 备选方案，等用户确认再动；每批一个可独立 revert 的 commit。
- 改动后 `go build && go vet && go test ./...` 全绿才 commit；commit message 写清 _why_ 而非仅 _what_；commit 后默认推送。
- commit trailer：`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。
