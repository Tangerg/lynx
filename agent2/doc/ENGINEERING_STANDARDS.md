# Agent Framework 工程实施标准

> 状态：强制实施基线
> 建立日期：2026-08-06
> 最后更新：2026-08-06
> 适用范围：`agent2` 的设计、编码、测试、评审和提交；最终替换后适用于 `agent`

本文定义新 Agent Framework 应当以什么技术标准实施。它不描述目标架构、不记录 ADR、不记录进度：

- 目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 架构决策见 [`DECISIONS.md`](DECISIONS.md)。
- 能力取舍与真实消费者证据见 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md)。
- 阶段与实际进度见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。
- 仓库通用规则见 [`../../CLAUDE.md`](../../CLAUDE.md)、[`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) 和 [`../../REFACTORING.md`](../../REFACTORING.md)。

本文比通用规则更具体，但不能放宽上位约束。发生冲突时，先按更严格且更接近根因的规则处理；仍无法裁决时，停止实现并更新 ADR。

---

## 1. 总标准：增量实施，阶段内一步到位

绿色重构没有旧 API、旧 wire 或旧目录结构的兼容包袱，因此每项已经决定实施的能力都必须从根因和正确层次上完成：

- 不为了少改代码保留错误抽象。
- 不用 adapter、alias、fallback 或特殊分支掩盖模型不正确。
- 不先提交“能跑”的简化版，再把正确语义留成 TODO。
- 不因旧 `agent` 已经这样实现，就默认复制其命名、类型或包结构。
- 不为迁移方便让 `agent2` 依赖旧 `agent`。
- 设计错误时允许删除和重写当前阶段代码，不在错误基础上继续叠加。

“一步到位”不等于一次实现全部未来能力，也不等于预先建立所有可能的接口和配置：

> 每一阶段都只实现已经被真实需求证明的范围，但这个范围内的语义、边界、错误、恢复、测试和文档必须完整，不留下已知债务。

允许本地 spike 用来验证接口；spike 中的临时抽象、硬编码和未完成路径不能作为阶段完成结果进入主线。阶段结束时只保留经多个真实实现证明的最小正确设计。

---

## 2. 裁决优先级

设计原则冲突时按以下顺序裁决：

1. **正确性与不变量**：状态不能非法、恢复不能重复副作用、错误不能被吞。
2. **职责与所有权**：行为和状态必须属于正确层次，不能抽象泄漏。
3. **依赖方向**：底层不能反向依赖高层，Framework 不能依赖 Host。
4. **清晰与简单**：读者应能沿正常控制流一次读懂，不以技巧换行数。
5. **API 人体工程学**：正确用法自然，错误用法困难，常用路径短而明确。
6. **可扩展性**：只为已经证明的变化轴提供扩展，不为猜测造 hook。
7. **复用与去重**：只有相同知识和相同变化原因才抽取。
8. **性能**：先保证语义，再依据 benchmark/profile 优化。

任何后项都不能用来牺牲前项。例如：不能为了 DRY 产生反向依赖，不能为了性能缓存无法可靠失效的派生状态，不能为了 API 简短隐藏重要失败语义。

---

## 3. 宏观架构标准

### 3.1 治本而非补丁

发现问题时必须先回答：

1. 被破坏的不变量是什么？
2. 谁真正拥有这个不变量？
3. 最早在哪个边界允许了错误状态进入？
4. 修复后同类问题是否仍能从相邻路径出现？

修复落在不变量 owner，而不是最先观察到症状的调用点。以下做法直接拒绝：

- 在消费端加 if 绕过上游非法状态。
- 用 retry、fallback、coerce 或默认值掩盖合同错误。
- 日志记录错误后继续运行不可信状态。
- 要求“调用者记得”维护本可由类型或构造边界保证的不变量。
- 为一个失败用例增加特例，却不修正产生该失败的状态模型。

### 3.2 每层职责必须能用一句话说明

每个 package、主要类型和接口都应有单一、稳定的变化原因。无法用一句话说明其职责，或说明中出现多个用“以及”连接的独立领域，通常意味着职责混杂。

层级检查：

- 最底层只保存所有 Strategy 都必须遵守的中性生命周期语义。
- Strategy package 只保存该策略的状态、算法和合同。
- Engine 只拥有 Process 调度和生命周期。
- Platform 只拥有多 Deployment 目录、路由和治理。
- Host 只通过中性合同消费框架，并拥有产品身份、持久化和交付协议。

把类型放入更底层不代表它更通用。只有所有消费者都必须理解的概念才能进入共同 Kernel。

### 3.3 依赖必须形成可验证 DAG

依赖只能从具体、外部、可替换的一侧指向稳定、抽象、内聚的一侧：

```text
Host → Platform/Engine → Process contracts
Strategy packages → Process contracts
Concrete planner → Planning contracts
Interaction → chatclient/tool/core protocols
Integration adapter → Agent contracts
```

强制禁止：

- Agent import 任意 Host application module 或产品类型。
- Kernel import Interaction、Planning、具体编排 adapter 或 Supervisor。
- Engine 按具体 Strategy 做 type switch 来决定主控制流。
- 基础模块 import Agent 以服务单一框架需求。
- 通过共享 `common`/`utils` package 隐藏双向概念耦合。

包依赖规则必须由 architecture tests 守卫，不能只写在文档里。

门禁必须覆盖完整生产图，而不是让每个 package 各自维护一份容易漏项的 denylist：生产 package 集合和所有允许的 Agent 内部直连边集中声明；任何新增 package、内部依赖边或反向依赖都默认失败。Host/旧模块、`flow`、logging backend、OpenTelemetry 与 Interaction 专属协议等具有架构含义的外部依赖也必须按 owner 集中约束；各 package 的本地 architecture test 只守自己独有的类型所有权和实现限制，避免同一规则多处漂移。

### 3.4 抽象必须恰好足够

#### 抽象成立的证据

至少满足一项：

- 两个以上真实实现需要被同一消费者替换。
- 多个实现共享一个不可分割的生命周期不变量。
- 跨模块消费需要隔离具体实现。
- 同一知识已经在三个以上位置重复，并且变化原因一致。
- 类型边界能阻止真实发生过的错误状态。

#### 抽象不足的信号

- 高层直接依赖具体 Planner、Strategy、Store 或 Client。
- 新增 Strategy 必须修改 Engine 中心 switch。
- 多个 package 复制 Process loop、event bus 或 snapshot 逻辑。
- 共同 Process 暴露 Goal、Messages 或具体编排 cursor 等策略状态。
- 状态转移规则散落在多个自由函数或调用方。
- 调用者需要理解被调用包的内部字段才能正确使用 API。

#### 过度抽象的信号

- 只有一个内部实现、没有外部消费者，却先定义接口和 factory。
- 接口与实现一一对应，只增加跳转层。
- `any`、反射、string key 或巨大 config 承载尚未理解的变化。
- 一个抽象需要大量“不适用于本实现”的空方法或可选字段。
- `Manager`、`Service`、`Context`、`Common` 等泛名类型不断吸收职责。
- 为未来可能存在的模式创建空 package、stub 或 hook。
- 新增一层后没有切断依赖，也没有封装不变量。

#### 最终判据

恰当抽象应同时满足：

- 消费者只看到它真正需要的语义。
- 实现者无需承诺无关行为。
- 新增同类实现主要通过增加新类型完成，不修改稳定 Kernel。
- 删除某一具体实现不会迫使共同模型变化。
- 接口比实现更小，且行为合同可以独立测试。

### 3.5 Framework 语义不能被应用职责污染

Agent Framework 可以定义并执行自身生命周期，但不能拥有下游产品的业务协调。

`agent2` 是可独立导入、可在任意 Go 程序中装配的库，不是某个产品后端的内部实现。任何公共抽象都必须能脱离具体 Host 完整说明；只能用某个产品名、数据库表或 UI 流程解释的类型没有进入 Framework 的资格。

Framework 不得引入：

- 产品 Session、Conversation、Turn、Workspace 或 User。
- Repository、数据库 schema、transaction、CAS、lease、retention。
- 业务幂等表、outbox、inbox、补偿事务或分布式锁。
- provider 价格表、USD 账单、订阅或产品配额。
- HTTP/SSE/WebSocket/desktop wire、UI 状态和展示文案。
- 产品审批角色、租户权限模型或审计存储。

Framework 可以提供中性扩展边界，使下游接入这些行为，但不能定义其应用模型或取得所有权。

Host 自有事实被销毁、回滚、替换或恢复时，Host 负责同步终止和清理失效关联的 Process/snapshot/continuation。Framework 只能暴露中性 lifecycle/capture 能力；产品历史水位、删除集合、应用 revision 和 write-set 不进入共同 Process。Strategy 确需校验外部事实时，在自己的 ExecutionState 保存 opaque revision/digest，并由自己的 provider 解释。

### 3.6 事务、幂等和扩展点的准确边界

事务和业务幂等属于下游应用或具体 Action/Tool integration：

- Host 决定 snapshot 与应用 write-set 如何原子提交。
- 具有外部副作用的 Action/Tool 由下游 adapter、decorator 或 middleware 实现幂等、事务和补偿。
- Agent 可以发布中性生命周期事实，供 Host 在自身事务内投影和提交。
- Agent 不暴露 `Transaction`、`UnitOfWork`、`IdempotencyStore` 或 `Repository` 作为 Kernel SPI。
- Engine 可以为 Effect 生成稳定 EffectID、保证 child/wait 等 Framework 实体不重复创建，并记录自身 settlement；这不是外部系统事务或业务 exactly-once 承诺。
- 外部 Effect 无法证明可去重时默认不自动重投；不得以 Engine 拥有稳定 ID 为由隐式 retry 模型、Tool 或 Action。
- dispatcher 必须声明 replay contract；未知结算只有在同一 EffectID 可证明代表同一逻辑操作时才允许自动重投，否则进入可观察、待显式裁决的状态。

必须区分两类概念：

- **框架自身正确性不变量**：例如同一 Step 不被并发推进、恢复不能重复创建同一个 child request。这些由 Engine 直接保证，不能降级为可选 Extension。
- **外部业务副作用策略**：例如付款、工单、文件写入的幂等和事务。这些由下游通过扩展/装饰边界实现，不能进入共同 Process。

Extension 只承载可选横切行为。忽略后会破坏所有实现正确性的语义必须进入 Kernel；只有某类消费者需要的具体行为留在对应消费层。

### 3.7 一个生命周期、一个扩展机制

- Engine 是唯一 Process loop owner。
- Strategy 只推进有界 Step，不创建第二套 runtime。
- Workflow 不复制 event/snapshot/child scheduler；它只通过 Framework Effects 编排真实 child Process。
- Extension 使用一个同质机制和结构化能力分发，不同时引入 Plugin、Hook、Advisor、Interceptor 等重叠体系。
- Strategy 是主控制流，不伪装成 Extension。
- 一个 Process 只有一个顶层 Execution；Process 构造只属于 Engine，跨 Strategy 或独立生命周期组合使用 child Process。

### 3.8 Signal、Transition、Effect、Event 和 Delta 不得混用

- Signal 是唯一入站执行输入；SignalID 负责投递去重。
- WaitID 是 Engine 铸造的等待目标身份，不与 SignalID 或 strategy logical key 共用字段。
- Transition 是候选状态和生命周期意图，不是已发生事实。
- Effect 是待执行操作；Engine 只解释封闭的 Framework Effect，Strategy Effect payload 整体由其 dispatcher 解释。
- Event 是已发生事实，区分 attempt 与 committed。
- Delta 是非权威临时输出，允许有界、显式可观察的丢弃；最终 Output 必须独立完整。

共同 Process 可以保存这些协议的不透明信封、顺序、游标和 settlement，但不能 import Strategy 类型或解析 payload。Signal 共同信封不含 Strategy kind/schema；所有 wire bytes 必须 defensive copy 并受大小限制，严格解码与 payload 版本校验由 schema owner 完成。

每个 schema owner 还必须独立冻结自己的完整 wire：Kernel baseline 只覆盖共同 snapshot/protocol/Event，Strategy baseline 只覆盖自己的 ExecutionState 与 Effect/Signal/Delta payload。共同 baseline 不递归解释 opaque Strategy bytes；Strategy 也不能把自己的 phase/cursor 提升进共同 Process。baseline 必须同时检测已登记 shape 漂移和新增 production wire 未登记，任何 breaking revision 都要在同一提交升级 owner schema version、增加 prior-version rejection，并禁止 dual-read/dual-write。

### 3.9 Step 提交纪律

- Step 只消费 Signal、归约 Execution state 并产生 Transition/Effect，不直接调用模型、Tool、Action、Store 或其他 I/O。
- Step 对相同 ExecutionState 与 Signal 序列必须产生相同候选语义；clock、random 和外部变化先编码为 Signal，不允许 reducer 隐式读取。
- Engine 执行封闭的 Framework Effect；Deployment 绑定 Strategy-owned dispatcher 解释其余 opaque Effect。dispatcher 不修改 Execution，只返回 Delta 和最终 settlement Signal。
- Engine 在 Step 前保留 last-stable state。Step、Transition 或 candidate snapshot 失败时丢弃不可信实例、保留 failure，且不自动重放失败 Step。
- prepare 原子记录候选状态、拟消费 Signal 范围、Transition、稳定 EffectID 和冻结 payload，但不推进权威 state/cursor；prepare 前不得 dispatch Effect。
- 若调用方要求跨进程崩溃恢复，dispatch 前必须允许其取得 prepared snapshot 并确认 durability boundary；该握手只传中性 snapshot/ack，不引入 Store、transaction 或应用 write-set SPI。未确认时不得宣称该 Effect 已具备 durable recovery。
- finalize 原子推进 candidate state、成功消费的 Signal 游标、Effect settlement、结果 Signal 入队和 Process transition，并清除 prepared step；这不宣称与 Host persistence 或外部系统原子。
- 提交前的模型/Tool/Effect started/finished 是 attempt facts；只有状态和 settlement 成功推进后才发布 committed facts。
- 固定顺序是 validate/capture/prepare → dispatch/settle → finalize → publish committed Event；snapshot 必须能保存 prepared step。每个崩溃点都必须有 contract test，未知外部结算按 dispatcher replay contract 处理。

### 3.10 阶段化交付不允许半成品

每个阶段可以只覆盖一个垂直切片，但必须同时包含：

- 正常路径。
- 输入与构造校验。
- 错误和取消语义。
- 相关状态不变量。
- 必要的恢复/并发行为。
- 单元、contract 和集成测试。
- API/GoDoc/示例。
- 架构守卫和残留扫描。

不提交只有 happy path、后续再补错误处理的“演示实现”。

---

## 4. 微观代码标准

### 4.1 浅显、直接、可局部理解

- 正常路径保持靠左，先用 guard clause 处理错误和边界。
- 函数只承担一个可命名行为；复杂分支按领域行为拆分，不按行数机械切碎。
- 同一不变量的读取、验证和状态变化尽量就近。
- 避免深层闭包、长 type switch、反射、隐藏控制流和嵌套泛型。
- 不用聪明技巧减少几行代码；优先让下一位读者一次看懂。
- 文件按具体职责命名并让相关类型、行为、测试就近放置。
- 大但内聚的算法可以保留在一个 package；不为目录整齐破坏内聚。

### 4.2 命名必须准确且唯一

- 名字描述本质，不描述临时实现或历史来源。
- 同一概念全模块只用一个术语；不同概念不能共用一个模糊动词。
- 类型用名词，行为用准确动词，bool 用可直接读成判断的问题。
- 避免 `Do`、`Handle`、`Process`、`Manage` 等脱离上下文无法判断行为的动词。
- 避免 package stutter、`GetX/SetX`、`Impl/Service/Manager/Helper` 和泛文件名。
- 字段名、JSON tag、事件字段和文档术语保持一致。
- sequence、cursor、index、reference 等相对词必须带 owner 或作用域；只保存 exact identity 的值不得借用完整 behavior binding 的名称。
- 共同协议只使用 `SignalID`、`WaitID`、`EffectID` 等具名身份；不得用 `Correlation`、`Token`、`Key` 同时表示投递、等待和副作用身份。
- 模型可见的 Tool 名称、描述和参数名按模型视角编写，明确边界、格式和错误条件。
- 重命名后同步修改注释、测试、wire 和文档，不留漂移引用。

### 4.3 采用 Go 风格的充血模型

优先把不依赖 I/O 的领域行为收敛到真正拥有它的实体或值对象：

- 不变量由构造器和实体方法维护。
- 派生值由 owner 计算，不在调用方重复拼装。
- 合法状态转移由 Process/Execution 等 owner 表达，不散落在 orchestration 函数。
- Definition 自己维护不可变和创建 Execution 的合同。
- Deployment 自己维护身份一致性和冻结后的查询行为。
- 值对象负责自身验证、比较和规范化。

以下类型保持数据形态是正确的，不强行增加方法：

- config 和 options DTO。
- wire request/response。
- snapshot payload。
- 纯事件记录和不可变结果。

需要 I/O、跨聚合协调或外部原子性的行为不塞进实体。充血模型表示“行为归正确 owner”，不表示建立 repository/service/domain-event 等仪式化 DDD 层。

### 4.4 使用 OOP 思想，但不用 Java 形态

在 Go 中采用：

- **封装**：私有字段、受控构造、由方法维护不变量。
- **多态**：由消费方定义的小行为接口。
- **组合**：具体类型组合能力，避免继承和基类层次。
- **消息传递**：通过明确方法和 Transition 表达意图，不直接改其他聚合内部状态。
- **对象生命周期**：Definition、Execution、Process、Deployment 各自拥有明确创建和终止语义。

禁止把 OOP 误解为：

- 每个 struct 都配一个 interface。
- 为单实现建立 `XxxImpl`。
- 继承式 embedding、抽象基类或深层 type hierarchy。
- getter/setter 数据袋。
- service object 承载所有实体行为。
- 通过 DI container 或 service locator 隐式获取依赖。

自由函数仍适合构造、解析、跨类型纯算法和没有单一 owner 的组合；不能为了“面向对象”制造伪 receiver。

### 4.5 SOLID、DRY、KISS、YAGNI 的落地

- **SRP**：以变化原因和不变量 owner 判断，不以文件行数机械拆分。
- **OCP**：新增 Strategy/Planner 主要增加类型，不修改 Kernel dispatch。
- **LSP**：实现接口必须满足完整行为合同，不能靠“不支持”逃避部分方法。
- **ISP**：接口只包含消费者实际使用的方法；内部单实现直接依赖 concrete type。
- **DIP**：应用依赖中性 Agent 合同，接口在消费方定义。
- **DRY**：相同知识出现三次且变化原因一致才抽取；跨边界宁可少量重复，也不制造错误依赖。
- **KISS**：选择能完整表达语义的最简单方案，不选择语义残缺的最短代码。
- **YAGNI**：不实现未经证明的能力，但已经进入当前阶段的能力不能做成半成品。

### 4.6 设计模式只在真实结构出现时命名

允许并鼓励使用与问题精确匹配的模式：

| 模式 | 合适位置 | 使用边界 |
|---|---|---|
| Strategy | Execution Strategy、Planner | 真实存在替换，不为单实现造接口 |
| State | Process 合法状态转换 | 状态较少时直接方法/表驱动，不建立类层次 |
| Composite | `flow` 的同进程 Node、Workflow 的有序 Stage | 组合必须共享真实输入输出和生命周期合同；不能跨越 Process owner 假装同层 |
| Adapter | managed Delegate、外部集成 | 只转换边界，不吸收业务策略 |
| Decorator/Middleware | Tool、Action、模型调用横切行为 | 顺序、副作用和错误语义明确 |
| Factory | Definition 创建/恢复 Execution | 无替换边界时返回 concrete value；窄腰处遵守 Execution 合同，不建立抽象工厂家族 |
| Specification | Planning Condition | 仅在组合和可解释评估确有价值时使用 |

模式是已存在结构的名称，不是实施目标。无法指出被封装的变化和被保护的不变量时，不得引入模式。

### 4.7 API 人体工程学

公共 API 必须让正确路径自然、可发现、难误用：

- 常用路径从根 package 完成，高级策略从具名 package 显式导入。
- 必填参数直接表达，可选项放入语义明确的 config/options struct。
- 不用连续 bool 参数；互斥策略使用有名值或专用类型。
- 不用 string 代替已有领域值对象、枚举或引用类型。
- 构造边界一次性验证并取得输入容器所有权。
- 零值只有在语义诚实且安全时才可用；否则要求显式构造。
- 小型不可变值优先按值返回；有身份、可变或较大的对象按指针返回。
- `context.Context` 是可能阻塞/I/O 操作的首参数，不存入 struct 或 snapshot。
- 流式拉取优先 `iter.Seq2`；不要用 channel 伪装普通迭代。
- Framework 窄腰保持非泛型：Definition/Execution 的输入、输出和 snapshot wire 使用受大小限制的 `json.RawMessage`，Descriptor 提供权威 schema、版本和 digest；泛型 `Typed[I, O]` 只存在于注册与调用边缘。
- Engine 依据目标 Definition 的 Descriptor 校验输入、输出和 child result，不用 Go 类型断言、反射或 `map[string]any` 代替 wire 合同。
- raw JSON 的解释权必须唯一：共同层只复制、限长、校验 envelope/schema digest，不窥探 Strategy payload。
- 不使用 fluent builder、全局注册表、隐式默认 Strategy 或 package-global Engine。
- 同一行为只保留一个权威入口；便利 API 必须确实减少概念，而不是制造同义入口。
- exported GoDoc 写清行为、参数、返回、错误、副作用、并发和恢复合同。
- exported callable 参数必须具名；每个 exported struct field 独立说明自身语义，不能用一条漂移注释覆盖多个字段。

### 4.8 Error 与失败语义

- 常量错误使用 `errors.New`，包装原因使用 `%w`。
- 可判断错误提供稳定 sentinel 或具体 error type，不解析字符串。
- 构造失败返回 nil/零值和 error，不返回可继续使用的半成品。
- 普通运行时失败返回 error，不 panic；`MustXxx` 只用于明确的启动期程序员错误。
- Tool/业务失败与框架内部错误保持边界，不能全部压成字符串。
- Process 终态由 Engine 已记录的控制意图、deadline 来源和 Step 错误分类共同映射；不得仅凭 `context.Canceled` 或 `context.DeadlineExceeded` 决定 `Killed`、`Cancelled`、`TimedOut` 或 `Failed`。
- Engine 发起 kill、父 Process 取消、Host context 到期和外部 Effect 取消是不同 cause；终态与 cause 都必须稳定、可测试、可恢复。
- deadline 终止保留独立语义，不压成普通 `Failed`；合同违约、外部失败和 panic 使用不同失败分类。
- 不同时记录日志并逐层返回同一错误；在真正的系统边界观测一次。
- 错误必须包含当前操作语境，但不能泄露 secret、prompt 或未经授权的 payload。

### 4.9 数据、状态与所有权

- 长生命周期对象不保留调用方可变 slice/map/config 的引用。
- 不存储能可靠即时计算的派生状态。
- 不让多个 owner 修改同一 Execution state。
- 一个 Process snapshot 只包含一个顶层 ExecutionState；跨 Strategy 状态只能属于 child Process，不能塞入联合状态或旁路字段。
- snapshot 必须严格、版本化、可判别，并拒绝未知或非法状态。
- 时间字段语义准确：`StartedAt`、`FinishedAt` 来自对应生命周期边界，`Duration` 与二者一致。
- 不把 transient 连接、闭包、context、mutex、goroutine 或 provider client 放入 snapshot。
- 不把产品 metadata 塞入通用 `map[string]any` 绕过层次边界。

### 4.10 并发与取消

- 每个 goroutine 都有明确 owner、停止条件和 join 点。
- Execution 默认单写者推进；并发分支隔离状态并确定性聚合。
- Signal 只在 Strategy 声明并经过 contract test 的安全边界消费；steer 的最早生效点是当前不可中断 Effect 结算后的下一安全 Step，公开 GoDoc 必须说明这一延迟合同。
- 需要独立 Agent 生命周期的并发分支必须是有界 fan-out 的 child Process；每个分支拥有独立身份、snapshot 和预算，不在单一 Execution 或 `flow` goroutine 内伪造多个生命周期。普通 in-process 并发留在 `flow` 或有界 Effect batch，并准确说明它没有 child Process 语义。
- 锁只能解决 data race，不能代替业务冲突语义。
- 并发度显式有界，取消和 deadline 沿 Process tree 传播。
- event、tool result 和 branch output 的协议顺序不依赖调度完成顺序。
- Delta listener 失败不得使 Step、Effect 或 Process 失败；缓冲必须有界，丢弃必须产生可观测事实，恢复后不补播历史 Delta。
- 测试使用 channel、barrier 或 `testing/synctest`，禁止用 `time.Sleep` 猜测并发完成。

### 4.11 注释与文档

- 代码优先通过命名和结构自解释。
- 注释只写 why、外部约束、并发/安全合同和非显然算法，不复述 what。
- exported API 的 GoDoc 是合同，必须与实现同步。
- 不保留迁移注释、review 说明、过期 TODO 或指向已删除符号的文字。
- 代码变更同步更新架构事实、ADR 或进度记录中真正受影响的那一份文档，避免跨文档复制。

---

## 5. 测试即设计证明

测试不只是回归门禁，还要证明抽象成立：

- 每个接口至少由真实消费者 contract test 验证，而不是只做 compile assertion。
- 每个 Strategy 用同一 Process contract test 证明共同语义，用自身测试证明专属语义。
- Step contract 在无取消条件下验证相同 state/Signal 输入产生规范化等价的 candidate/Transition/Effect，并禁止隐式 clock/random/global state。
- 状态机覆盖所有合法和非法转换。
- snapshot 覆盖每个合法挂起边界的 capture → restore → continue。
- Signal contract 覆盖乱序到达、重复投递、未知 WaitID、消费游标与 candidate state 同步提交，以及失败 Step 不吞信号。
- Effect contract 覆盖 prepare 前无 dispatch、durable recovery 启用时 acknowledgment 前无 dispatch、prepared snapshot 恢复、稳定 EffectID、dispatcher 恢复、settlement 去重、部分 batch 结算、确定性结果顺序、不可重试副作用和 attempt/committed 事实边界。
- typed adapter contract 证明 schema 校验发生在边缘且 erased Engine 可以同构持有异构 Definition。
- 终态矩阵覆盖 Engine/parent/Host/Effect 取消来源、deadline、合同违约、外部失败和 panic。
- Delta contract 覆盖 listener 失败隔离、有界缓冲、显式丢弃、恢复不补播，以及 final Output 不依赖 Delta 重建。
- child Process 覆盖递归、预算耗尽、取消、部分失败、祖先等待拒绝和恢复去重。
- 并行路径验证稳定结果顺序，并运行 race tests。
- wire/snapshot codec 使用 golden 和 fuzz 验证严格性。
- 错误测试使用 `errors.Is/As`，不比较脆弱完整字符串。
- 旧 `agent` 测试作为历史反例和行为证据参考，但新测试围绕新合同重写，不机械复制。
- example 必须使用正式公共 API，并能作为最小集成测试运行。

无法为一个抽象写出独立行为合同，通常说明抽象尚未成立。

---

## 6. 每批完成定义

每个提交批次必须同时满足：

### 6.1 设计

- [ ] 问题和根因明确，修复落在正确 owner。
- [ ] 没有新增抽象泄漏或反向依赖。
- [ ] 所有公共类型都能脱离当前产品独立解释，没有应用身份、历史水位、存储协议或 UI 语义。
- [ ] 新接口有真实消费者和替换理由。
- [ ] 新类型、方法、字段和参数名称语义唯一准确。
- [ ] 旧实现已完成“保留思想 / 重新实现 / 移除”裁决。

### 6.2 实现

- [ ] 正常、错误、取消、边界和恢复语义完整。
- [ ] 领域行为收敛到正确实体或值对象。
- [ ] 没有 stub、TODO、FIXME、HACK、兼容 shim、死代码或空目录。
- [ ] 没有无 owner goroutine、无界并发或随机提交顺序。
- [ ] 注释、GoDoc、wire 和代码一致。

### 6.3 验证

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] 相关 contract、race、fuzz、golden 和 architecture tests。
- [ ] `go mod tidy` 后无非预期 diff。
- [ ] `git diff --check` 通过。
- [ ] 进度和执行日志只记录已经完成的事实。

### 6.4 提交

- [ ] 一个批次可以独立回滚。
- [ ] 不包含工作区其他模块的并行改动。
- [ ] commit message 说明 why 和被保护的不变量。
- [ ] 提交后推送，避免进度丢失。

---

## 7. 评审时直接拒绝的信号

出现以下任一情况，默认不进入下一阶段：

- “先这样，以后再改正确”。
- “为了兼容旧模块暂时保留”。
- “调用方应该不会这样用”。
- “加个 retry/fallback 就不会报错”。
- “所有模式先放进一个 enum/config，之后再拆”。
- “只有一个实现，但以后可能有多个，所以先抽接口”。
- “放到 core/common 以后大家都能用”。
- “事务/Session/Store 放进 Agent 会更方便”。
- “加锁后结果就是确定的”。
- “复制完整父上下文给 child 最省事”。
- “测试只覆盖 happy path 就够开始下一阶段”。
- “这个名字虽然不准确，但改起来麻烦”。
- “注释先不改，代码能跑”。
- “模式越多、抽象越复杂，框架上限越高”。

最终判据只有一个：

> 这个实现是否在正确层次，用最小而完整的设计守住真实不变量，并让下一位 Go 开发者无需了解历史就能正确理解和使用？
