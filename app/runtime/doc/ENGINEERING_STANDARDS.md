# Lyra Runtime 重构工程标准

> 状态：强制执行
>
> 适用范围：`app/runtime` 及其直接 Desktop 合同爆炸半径的设计、实现、测试、评审、提交和文档维护

本文把仓库级规则具体化为 Runtime 重构的完成标准。目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)，已接受的取舍见 [`DECISIONS.md`](DECISIONS.md)。任何阶段都不能以“迁移中”为理由违反这里的边界。

## 1. 总原则

### 1.1 治本而非补丁

每个问题必须在真实 owner 和根因层修复。上层 `if`、兼容 adapter、额外 retry、影子状态、字段猜测或 error coercion 只让症状消失，不能作为完成方案。

### 1.2 不保留已知债

开发阶段允许 breaking change。已经知道更准确的名称、所有权、API 或 wire shape 时，当前阶段直接改对；不保留 alias、deprecated path、dual read/write、compat decoder 或“下一轮再删”的死抽象。

### 1.3 恰当抽象

抽象成立必须至少满足一项：

- 切断一个真实不应存在的依赖；
- 表达一个稳定领域概念或不变量；
- 存在两个以上真实可替换实现；
- 被一个真实消费者以更窄合同使用；
- 隔离第三方 SDK/wire/存储形态。

只有一个实现、一个调用者且没有边界价值的接口默认删除。三个名字不同但语义相同的 facade 默认收敛。相似代码若因不同原因变化，不为了 DRY 强行共享。

### 1.4 不泄露

任何跨包类型都要回答：它是谁的语言、谁能改变它、谁负责验证它。以下均视为泄露：

- Domain/Application 出现 Agent Framework Process、Signal、Effect、Deployment 或 snapshot payload；
- Agent Framework 出现 Run、Session、pricing、approval、Store 或 Runtime protocol；
- Delivery 持有 executor handle、Run pump、Store 或 SDK client；
- Infra 决定应用事务、Run terminal policy 或 UI projection；
- Toolset 取得完整 Engine/Stack 或 Interaction private state；
- Bootstrap 暴露 god object 让下游随取随用。

### 1.5 不过度设计和过度防御

产品只有一个 Desktop actor 和一个逻辑 Runtime。Runtime 可以经 HTTP、socket 或同进程 binding 接入；进程重启、连接重建和 binding 变化只是同一逻辑 Runtime 的实现生命周期，不据此建模“旧服务端”“后继服务端”或多客户端协调协议。Desktop 与 CLI 共享同一数据目录是独立的存储并发场景，只在已经存在的 SQLite、文件和工作区 owner 边界处理。

- 只为当前产品中可发生的状态、替换点和失败窗口建模，不为假设中的多窗口、多服务端、多 transport 组合或未来部署形态预埋层次；
- 输入、required dependency 和 immutable identity 在其边界验证一次。只有相关事实可能在异步间隙变化时才重新证明，不能把重复校验当成安全感；
- 不为不可达状态增加 fallback、retry、锁、lease、heartbeat、timeout、generation、恢复分支或专用类型；
- 一个直接对象、函数或事务能完整表达不变量时，不再包一层 port、owner、coordinator、registry 或 state machine；
- 防御机制必须对应一个可复现反例，并说明没有该机制会写错哪份事实、泄漏哪项资源或破坏哪条事务不变量。

## 2. DDD 实施标准

### 2.1 Ubiquitous language

- 一个概念只有一个术语；Plan/Todo、Run/Turn、Session/Conversation 不得混作同义词；
- 包、类型、字段、方法、错误、JSON tag 和协议字段必须使用同一 owner 的语言；
- Framework 术语只在防腐层和 Framework integration 中出现；
- 改名必须覆盖代码、测试、GoDoc、schema、fixture、文档和生成物，不留漂移注释。

### 2.2 Rich domain model

Entity 自己保护状态迁移和不变量。Application 不得通过一串 setter 或公开字段过程式拼装非法中间态。

优先：

- `run.Complete(...)`、`run.Interrupt(...)`、`pending.Answer(...)` 这类领域行为；
- 构造时验证 immutable value object；
- 返回新值表达不可变转换；
- 用 sentinel error 暴露调用方必须分支的领域失败。

避免：

- `ValidateX` 与真正 mutation 分散、调用方必须记住顺序；
- data bag + application 巨型 switch；
- entity 暴露 Store、clock、SDK client、context 或 transport DTO；
- 为“充血”把跨聚合事务塞进单个 entity。

### 2.3 Aggregate boundary

- aggregate 内部行为必须原子、一致；
- aggregate 之间只通过稳定 identity/value 和 Application coordination 关联；
- 事务可以因应用不变量跨多个 aggregate，但 write-set owner 必须是明确 use case；
- projection/read model 不获得修改 domain 的能力；
- 不复制另一个 aggregate 的可变字段作为第二事实源。

### 2.4 Domain package

- 默认纯 Go 值和行为，无 I/O；
- 不定义为了 SQLite/HTTP/Agent SDK 服务的字段；
- 可以定义由领域行为真实消费的纯策略接口；不使用 `context.Context` 包装 Repository/Store，消费 I/O 的接口一律放到 Application；
- package 名是领域名，不是 `service`、`manager`、`model`、`entity`、`impl`；
- 跨 domain import 必须是单向 DAG，遇到 cycle 先重审所有权，不能抽 `common` 解环。

## 3. Clean Architecture 实施标准

### 3.1 Domain

- 禁止 Application、Adapter、Infra、Delivery、Bootstrap、Agent Framework、SQLite、JSON-RPC 和外部驱动 SDK import；
- 领域错误不引用协议 code 或 HTTP status；
- 时间、随机数等不确定输入由 Application 提供准确值，领域方法消费值而不是隐式读全局；
- 纯计算使用 value receiver 时保持值语义，mutable entity 使用 pointer receiver 并集中修改。

### 3.2 Application

- 一个 package 围绕一组高度相关 use cases；
- 接口定义在消费者文件附近，通常 1–3 个方法；
- I/O 和长任务首参数是 `context.Context`，不存入 struct；
- 明确写出 validation、claim、durable commit、external apply、publish 的顺序；
- 失败路径必须说明哪些事实尚未改变、哪些已经提交、如何 fail closed；
- 不导入具体 Adapter/Infra/Delivery/Agent Framework；
- 不通过 callback 把应用事务交给 Adapter 决定；
- 同一 use case 的 goroutine、channel、claim 和 shutdown 必须由一个 concrete owner 收敛。

### 3.3 Adapter

- Adapter 只实现消费端口或翻译外部语义；
- 构造函数返回 concrete type，调用方靠结构化接口满足端口；
- SDK error 在边界转换为应用可分支 error，保留 `%w` cause；
- 不把 SDK DTO 原样塞进 Domain；
- 不持有自己不拥有的 transaction、lifecycle 或 product policy；
- 两层 adapter 若只是参数转发，删除其中一层；
- decorator 必须有一个精确目的，例如 authoritative observation、pricing 或 Tool presentation，不能形成万能 middleware chain。

### 3.4 Infra

- 只提供技术 mechanism，不组织业务用例；
- 不 import Application、Adapter、Delivery 或 Bootstrap；
- 文件、进程、网络和数据库操作必须接受 context、明确 timeout/cancel 和资源关闭；
- SQL transaction 只执行上层已经决定的 write-set，不发明业务顺序；
- path、command、URL 和外部输入在其信任边界完成准确验证；
- 不建立可配置后端矩阵，除非已有两个真实生产消费者。

### 3.5 Delivery

- request decode、protocol validation、application command、application error projection、response encode 五步清晰；
- 不直接读 Store、调用 agentexec 或修改 Domain entity；
- Server/API 不持有 active Run map、pump、executor tree 或 connection registry；
- operation 是严格验证、能力门禁、幂等、problem 投影和 replay attachment 的唯一 binding-neutral owner；
- RPC dispatch 不解释业务字段；Transport 不解释 RPC method；
- HTTP/SSE 与 embedded 使用同一 operation/Application entrypoint；binding 不复制业务或可靠性规则；
- 协议类型只在 delivery/contract 边界，Application 不返回 wire DTO。

### 3.6 Bootstrap

- 依赖创建和 wiring 明确、静态、可搜索；
- 不使用反射 DI container、service locator 或 package globals；
- 所有 resource closers 只有一个 owner，且只能是 one-shot terminal action；返回 error 是诊断，不是“尚未关闭”，不得提供通用 Retryable wrapper。host/tool resources 按 acquisition 顺序一次性交给唯一逆序 Sequence；Sequence 的 generation 不受 caller deadline 取消，失败 Open 返回后也必须继续释放依赖，后续 Close 只能 join、不能重放。真正需要多 generation 推进的 subsystem 在自己的 owner/ledger 内表达，不能下沉为通用 closer；
- Host 整体 shutdown generation 与 caller wait 必须正交：第一个 Close 广播 cancellation 并由 `hostLifetime` 持有 component/executor join 直到进入 terminal Sequence，caller timeout 只返回等待诊断，不能把图留给一个在失败 Open 中根本拿不到 Host 的外层。并发 Close 只 join 同一 generation；只有 component 已返回明确 unsettled error 时，后续显式 Close 才能启动下一 generation，禁止 timer retry loop；
- 后台任务加入明确 task group，Host shutdown 等待它们结束；
- required collaborator 和 lifetime 在 constructor boundary 完整验证；constructor 必须返回可运行对象或 error，不能把半初始化对象和延迟 `unavailable` 分支交给用例；
- optional capability 只在真实配置关闭时为 nil/absent；多个依赖共同构成一项能力时用一个显式 capability group 表达，不能允许半启用。
- `bootstrap.OpenInstance` 创建每个 Runtime 唯一的 context root，并拥有 cancel 与 join；Assembly、operation、Interaction、Toolset、LSP、MCP/OAuth 和 workers 只消费注入 lifetime，不另造 immortal root。
- HTTP host 与 embedded 共用同一 Runtime instance builder；共享目录 setup、所有权恢复、后台任务与资源关闭不能各装配一套。
- canonical data directory 必须是 `0700` 私有目录；setup lease 只包围 store 打开与 schema/config seeding，不能扩张为 Runtime 全生命周期单实例锁。失败 Open 要逆序回滚；caller timeout 只停止等待，已启动的 Host generation 必须继续 join component/executor 并自行走完 terminal resource Sequence，不能要求拿不到 Host/Assembly 的外层调用方再次 Close。terminal diagnostic 不得让 Host 永久停在 stopping。
- 每次 Session mutation/Run 必须取得跨进程 Session writer lease；Run 同时取得 physical working tree shared lease，rollback/restore 等破坏性操作取得 exclusive lease。Goal drive 和恢复器必须竞争同一 owner identity；恢复先选举一个跨进程 sweep winner并固定 Run-before-Goal 顺序，startup 必须等待 winner 后复核，存活期可以非阻塞跳过，cleanup 只能作用于已取得的 Session。
- 不用 heartbeat/TTL 推断本机 owner 死亡；以 OS advisory lease 的持有/释放为真相。来自其他 SQLite connection 的提交必须触发 read-model resync，消费者收到后重读 durable projection。

### 3.7 公共 Go binding

- `protocol` 只公开 binding-neutral values、strict validation 与客户端可见 problem，不公开服务端接口、context key、numeric JSON-RPC code 或 generator internals；
- `embedded.Open` 返回 concrete `*embedded.Runtime`，不导出胖 interface；消费方在自己一侧定义窄接口；
- embedded command/subscription metadata 使用准确 option 类型，不用 header 名、`map[string]any` 或 bool bag；
- 已接受 Run 脱离请求取消并归 Runtime lifecycle 所有；subscription context 只结束该订阅；
- stream 必须显式定义终止、错误、背压和 Close 行为，不用 goroutine 泄漏换取看似异步的 API；
- 公共 API 的参数、返回值、GoDoc、零值和 error 分支必须可从调用点直接理解，不暴露内部 composition object。
- `contract/go-api.json` 必须由真实 Go type information 生成且零漂移；公共 package 只允许 `protocol` 与 `embedded`，public signature 不得引用 `internal` 类型；operation catalog 与 Runtime method 必须一一对应。
- 稳定失败用 `errors.Is` 匹配 `protocol` sentinel；需要恢复动作或字段错误时用 `errors.As` 取得 `protocol.ProblemError`，不公开私有 error concrete，也不让消费者解析字符串。

## 4. Agent Framework 接线标准

### 4.1 Agent Framework public API only

- 只调用 Agent Framework 已冻结的公共合同；
- 不读取 private JSON、未导出的 wire 或测试 helper；
- 不把旧 Agent 行为当兼容规范；
- 需要新 Framework 能力时，先证明是中性框架缺口，而不是 Runtime 产品策略，再单独修改 Agent Framework ADR/baseline。

### 4.2 一个执行循环

- Agent Framework Engine 唯一驱动 Process；
- agentexec 不建立第二 controller、scheduler、ToolLoop、mailbox 或 child registry；
- Application 可以拥有 Run pump，但 pump 只归约 executor facts 和提交产品状态，不推进 Framework internal state；
- `Turn` 不越过 agentexec 内部；目标完成后不保留 Turn lifecycle abstraction。

### 4.3 Snapshot opacity

- TreeSnapshot 只在 agentexec 编解码；
- Application checkpoint payload 必须 defensive copy，不共享可变 byte slice；
- Host metadata 与 Framework snapshot 分栏校验；
- restore 必须重建 exact Deployment 集并验证 ref/digest；
- snapshot 损坏、BuildID 不匹配或外部事实失效按明确产品 policy 失败，不猜测修复。
- 初版不配置只接收单 Process Snapshot 的 `PreparedStepAcknowledger`；只有完整 quiescent TreeSnapshot 可以成为 durable recovery point；
- continuation payload 直接使用 Agent Framework public TreeSnapshot，不创建 Runtime private tree wire、拼装 Process Snapshot 或第二 payload version；
- answer claim 必须原子记录 exact answer、隐藏 `resuming` row 并删除旧 waiting recovery point。next-Segment opening 必须在同一事务中证明 durable claim，commit 后才允许提交 semantic Signal；
- 进入 `resuming` 后直到新 quiescent checkpoint 提交，任何 crash 都不能回退旧 snapshot。pre-opening failure 必须先 durable `RunLost` 再 release；若 terminal write 失败，tree/claim 保持供 recovery 收口；
- 下一 barrier 只能由同一 Session/executor/root-member owner 替换 `resuming` row；terminal/recovery 负责删除，普通查询不得把 answer audit 当 open input；
- Interaction input ACL 不 import、探测或 fallback 到旧 suspension。Product prompt/resolution codec 可以复用，Framework continuation/Signal owner 不可复用。

### 4.4 Observation

- Event/Delta listener 不作为 veto、transaction callback 或唯一应用真相源，只能唤醒对账和承载临时流；
- Delta 丢失不得破坏最终 Output 或 Transcript；
- model/tool authoritative facts 在真实调用返回前后同步记录；
- attribution 只通过 invocation context，不通过全局 map 或 Process handle lookup；
- listener、decorator 和 projection 都必须有界且并发安全。
- 每个 EffectRequest 使用 context-scoped、EffectID-bound、并发安全的 dispatch-attempt tracker。pre-call write 失败时不得外呼；post-call authoritative write 失败时 outer Dispatcher 必须返回 error 形成 unknown settlement；
- Tool batch 的 tracker 覆盖整个 Effect。一个已执行 Tool 的 post-call write 失败后，未开始的串行 Tool 停止，并行 in-flight Tool 结算后整个 Effect 仍为 unknown；
- per-Run pump 最终通过 Process result/status、Strategy public helper 和 complete-tree capture 对账；listener 丢失不能永久遗漏 waiting/terminal。
- outer Dispatcher 返回 indeterminate error 前必须先标记并唤醒 per-tree reconciliation；Run pump 还要用有界 tick 查询所有已知 Process 的 `UnknownEffectIDs`，不能依赖 EffectFinished Event 必达；
- live unknown 的 durable RunLost/incomplete/cleanup-intent transaction 成功前不得 Kill。提交失败保持 Framework unknown wait 并重试；unknown 与未提交的 cancel/deadline 竞争时 Lost 优先。

### 4.5 Waiting、resume 与 steer

- waiting 事实必须从 Strategy public pending-input helper + quiescent complete-tree capture 联合证明；Event 只能唤醒，不能单独制造 Pending；
- exact restore 依次校验 Host envelope、BuildID、scope、public TreeSnapshot、root/status、DeploymentRef 与 pending input；任一失败都返回不可恢复事实，不“尽量继续”；
- WorkingContext 由 snapshot 自足恢复，waiting 期间 Conversation 变化不得进入已存在 Process；isolated workspace 在 executor loss 后 fail closed；
- answer 只允许投递到当前 active WaitID，Signal identity 由 exact item/member/request/response 稳定导出；Application/Delivery 不暴露 arbitrary Signal API；
- ask-user 与 interactive approval 都走同一个 product Interrupt/answer contract。approval plan/pre-hook 首次调用后冻结，恢复不得重复执行；
- steer 只能接受 canonical user content，经 Strategy typed steer Signal 在下一安全 model boundary 生效；产品投影必须排在首个可见该 steer 的 model-call fact 之前；
- `PreparedStepAcknowledger` 未启用时不得声称 active-step crash recovery；unknown Effect 也不能成为可提交的 quiescent checkpoint。

### 4.6 Child admission

- root/child prospective identity 在发布前完成 admission；
- Application admission 允许有界 I/O，但不得重入 Engine/Process；
- 同一 prospective identity 重试的业务正确性由 Runtime adapter 负责；
- child Run binding 使用稳定因果 key，不依赖 goroutine 完成顺序；
- restore 不重复创建已存在 child Run；
- admission 只创建 Opening/reservation，Framework conclusive started 后才发布 Running；
- admission success 后的每一个 Framework 失败点都必须产生同一 prospective identity 的 aborted outcome，或由 Framework 保证批准后 publication 不再失败。没有该合同不得启用 durable child admission；
- 不用 timeout、private identity derivation、Event 顺序或 parent Effect payload 推断 ghost child。

### 4.7 Waiting subtree change

- Application 不接触 `WaitingSubtreeCancellationPlan`、ProcessID、Engine lock 或 source digest；
- agentexec concrete one-shot capability 唯一持有 prepared Framework change；Application 只看 member projections、opaque resulting checkpoint 和准确的 Apply/Discard capability；
- prepared change 在 Apply/Discard 前保持 source tree frozen；Application transaction 不持有自己不拥有的 Framework lock；
- capability 必须 one-shot、Discard 幂等、有 Host-owned deadline；agentexec 获取后立即 `defer Discard`，禁止遗漏清理导致 tree 无限冻结；
- transaction failure 必须 Discard；commit 后必须调用不接受请求 context 的 Apply。若最终外部边界被移除，Process activation 必须由独立 `Continue(ctx)` 表达，不能塞进 Apply 或把 activation failure 伪装成 apply failure；commit 后 crash 恢复 resulting checkpoint；无法证明的 apply failure 先释放旧 owner并从该 checkpoint 精确恢复，只有恢复失败才提交 RunLost。

## 5. Go API 标准

### 5.1 Naming

- package 名简短、准确、无 `runtime.RuntimeX` 或 `toolset.XTool` 口吃；
- 类型名描述本质，不使用 `Impl`、`Service`、`Manager`、`Helper`；
- 方法使用动作语义，不使用含糊 `Handle`、`Process`、`Prepare`，除非该词本身是领域概念且 GoDoc 定义边界；
- accessor 不加 `Get`；
- bool 参数改为准确 option/value type，避免调用点无法阅读；
- 时间字段使用 `StartedAt`、`FinishedAt`、`Duration`/`DurationMillis` 的唯一语义。能从同一连续区间精确派生的时长不重复存；生命周期含审批、暂停等空档而无法推出真实活动时长时，只允许拥有外部边界的单一 owner 计算并保存该独立事实。

### 5.2 Types

- 零值有清晰语义；无合法零值的类型只能通过构造函数创建；
- required dependency、typed nil、非法 limit/path/identity 必须在构造返回前拒绝；构造参数分组不能原样保存在对象中形成 config façade；
- immutable 小值优先按值返回；拥有 mutex、resource 或 identity 的类型用 pointer；
- slice/map/`[]byte` 跨边界 defensive copy；
- 不用 `any`、反射或 map bag 逃避领域建模；wire type erasure 只在协议/Framework owner 明确的边界；
- generics 只消除真实重复算法，不建立 generic repository/entity hierarchy。

### 5.3 Interfaces

- consumer-owned；
- 尽量 1–3 个方法；
- 不嵌入无关 capability 形成胖接口；
- 不为测试而制造生产接口；测试可以使用 concrete fake dependency；
- 接受接口、返回 struct；
- typed nil 在构造边界明确拒绝或拥有准确零语义。

### 5.4 Errors

- 常量错误使用 `errors.New`；需要调用方分支的错误提供稳定 sentinel/type；
- 添加操作上下文时使用 `fmt.Errorf("...: %w", err)`；
- 不用 `%v` 丢失 cause；
- 不同时 log 并 return 同一错误；
- panic 只在外部 callback/listener/SDK 边界隔离，不能把程序错误伪装成普通业务失败；
- Delivery 是把 domain/application error 映射为 protocol error 的唯一位置。

### 5.5 Context 与超时

- I/O、锁等待、admission、stream、long-running operation 首参数为 context；
- 不把 context 存进 entity/config/snapshot；
- process/component lifetime 可以由资源 owner 保存，但必须来自 constructor 显式注入；request/startup context 不得存成共享资源 lifetime；
- Provider/network/process 必须有明确 timeout 或上层 deadline；
- 后台任务需要继承 trace values 但脱离 request cancel 时使用 `context.WithoutCancel`，并由 Host 生命周期另行取消；
- 内部 API 收到 nil Context 必须明确返回错误或按 Go 合同暴露 programmer error，不得静默替换为 `context.Background()`；
- 只有 Runtime Instance/Host 和真正的 process transport owner 可以创建 `context.Background()` root，其余组件从 owner lifetime 派生；
- Run execution 必须使用 Run owner 的 lifecycle context；Delivery request context 只约束 admission/request handling，不能在响应返回时误杀已接受的 Run。

### 5.6 Concurrency

- 每个 goroutine 在代码和测试中都能回答谁启动、谁取消、谁等待；
- fan-out 使用显式正数 limit；
- channel 的关闭方唯一；
- mutex 保护明确状态，不在持锁期间调用未知 callback/I/O；
- terminal first-wins、cancel-vs-wait、resume claim 等线性化点写成行为测试；
- 测试禁用 `time.Sleep` 等待并发结果。

## 6. Wire、Schema 与持久化标准

- 所有 JSON object 默认严格拒绝未知字段和尾随值；
- union 必须有明确 discriminator，不依据字段猜类型；
- schema、Go codec、validation、example 和生成物同批更新；
- 新 wire shape 直接提升 owner version/epoch，不双读旧版本；
- 时间统一 UTC，外部格式使用协议规定；
- SQLite schema epoch、artifact version、checkpoint envelope version 各有唯一 owner；
- raw payload 必须有大小上限、defensive copy 和 owner-specific validation；
- 不把数据库列名、JSON tag 或 SDK enum 当成 Domain API。

## 7. 测试标准

### 7.1 测试层次

按改动风险选择最小充分的测试组合。只有改动触及对应边界时才增加该层测试；结构合并、命名调整或局部对象收敛不需要机械补齐 race、fuzz、HTTP 和真实 Engine 纵切。

可选层次包括：

1. Domain table-driven invariant tests；
2. Application use-case tests，使用最小 handwritten fakes；
3. Adapter contract/translation tests；
4. SQLite/HTTP integration tests；
5. Agent Framework 真实 Engine 纵切 consumer tests；
6. architecture/static baseline tests；
7. concurrency race tests；
8. 对 snapshot、protocol、strict codec 的 fuzz/round-trip tests。

### 7.2 真实纵切

Agent Framework 迁移不能只用 mock Engine 证明。必须运行真实 Interaction + Engine，覆盖：

- start/stream/terminal；
- model ToolCall 与 authoritative projection；
- waiting snapshot/restore/answer；
- steer 安全边界；
- Delegate child admission 与 child Run 映射；
- deferred advertisement；
- waiting subtree prepare/commit/apply-or-discard；
- cancel、deadline、provider/tool failure 和 corrupt checkpoint。

### 7.3 Architecture fitness tests

机器守卫只保护稳定依赖方向和已经发生过的高代价回归，不固化历史目录、文件数量、对象字面量形状或某次重构的语法特征。可用守卫包括：

- 精确 package DAG；
- Agent Framework import allowlist；
- Application/Domain/Delivery 禁止外环 SDK；
- Delivery 不持有 Run/executor lifecycle state；
- Infra 不 import Application/Adapter/Delivery；
- 不存在 `component/common/core/utils` 杂物层；
- 不存在旧 Agent import、旧路径、旧术语和 compat codec；
- public/protocol/wire baseline 未经 ADR 不漂移；
- exported GoDoc、参数名和 error wrapping 合同。

若一次重构删除了被守卫的实现形态，同批删除或上移对应静态规则。不能让 AST 形状测试替代真实消费者、依赖边界或行为测试。

## 8. 编码与变更规范

### 8.1 文件与 package 粒度

- 按 change reason、领域边界和生命周期拆分，不按行数、类型数量或“每个概念一个文件”机械拆分；
- 只有一个调用者，且没有独立不变量、资源生命周期、替换价值或防腐边界的文件/package，默认吸回真实 owner；`toolname` 这类只为 `toolset` 服务的维护单元不能独立存在；
- 单消费者不自动等于必须合并：strict codec、生成合同、平台 adapter 或清晰的领域 value object 即使只有一个消费者，只要边界本身稳定，仍可独立；
- 一个文件因多个不同原因变化时才拆分；一个对象的状态、行为、join/dispose 和不变量不能为了缩短文件被摊成过程式 helper 集合；
- 禁止 `common`、`utils`、`helpers`、`types`、`misc` 等无 owner 杂物包。共享代码必须以它拥有的语义命名；
- 新增 package、interface、facade、singleton accessor 或 forwarding wrapper，必须在评审中说明它切断了什么依赖或保护了什么不变量。

### 8.2 对象、构造与行为

- 优先让 concrete object 持有自己需要的 identity、状态、不变量和生命周期；方法在对象内部完成合法状态转换，不让调用方拼接 `validate → mutate → cleanup` 隐式协议；
- `Owner`、`Lifecycle`、`Generation`、`Transaction` 等名称只用于真的拥有资源或提交能力的对象，不能作为 `Manager`/`Service` 的时髦替代词；
- required dependency 在 constructor 或 start boundary 一次证明。禁止 typed nil、迟到 locator、动态全局 accessor、插件安装顺序和运行期 panic 充当依赖注入；
- 构造函数返回可立即使用或可明确 `Start` 的合法对象；失败不得留下半注册、半启动或需要调用方猜测 cleanup 的实例；
- public mutable fields、setter 串、map bag 和跨层 config façade 默认禁止。不可变值按值传递，持有 mutex/resource/identity 的对象按指针传递；
- 一层只转发参数、改名或为测试暴露内部对象的 façade 直接删除。Decorator 必须拥有精确、可单测的策略。

### 8.3 异步与 generation 提交权

- 异步 workflow 只有跨越真实可替换 owner 时，才在 admission 捕获 exact client、entity identity、generation 和所需 capability；普通局部异步代码持有直接依赖即可；
- `await`、goroutine receive、RPC response、stream callback 或异步 teardown 之后，只有下一步会修改“当前实例”共享状态，且 owner 可能在等待期间被替换时，才重新证明 exact owner；写入 workflow 自己拥有的局部状态不需要重复 guard；
- `AbortSignal`/context cancellation 只负责尽快停止工作，不构成“迟到结果一定不会提交”的证明。非协作依赖仍须由 commit guard 拒绝；
- 进程内 owner replacement 先发布新实例，再同步退休旧实例。旧 disposer 只能 take/join/clear 自己的资源，不能按 name、key 或全局 predicate 命中当前实例；该规则不把 Runtime 进程重启解释为逻辑服务端更换；
- final close 是不可逆状态，重复 dispose 共享同一 settlement；不得在 teardown continuation 中重新创建 root/client/port；
- 禁止 `sleep`、debounce、延迟 invalidate、轮询次数或“再 refresh 一次”掩盖竞态。需要排序时优先使用现有 owner 的直接顺序或事务；只有真实存在多个合法阶段时才引入状态机。

### 8.4 TypeScript、React 与 Desktop 状态

- React component 负责渲染和局部交互，不拥有 Runtime transport、跨 Session material、全局 mutation queue 或 renderer lifecycle；这些状态进入对应 Application owner/context；
- server state 的 standing value 只有一个 authoritative writer。Query、event、optimistic cache 和 material snapshot 必须明确主从关系，不能都能提交完整 read model；
- component local state 只保存短命 presentation intent（如 hover、展开、pending feedback），并绑定 exact entity + generation；props/query 追平、失败、切换或 retirement 时有确定释放规则；
- 不在 render 中写状态，不用 effect 模拟 command，不以缺失 dependency 或 stale closure 保留旧代对象；副作用必须能说明 install、replace、dispose 的完整时序；
- 跨 context 只走 published port，禁止从 UI 深层 import adapter、container、QueryClient singleton 或另一个 context 的 private store；
- TypeScript 状态优先使用判别联合表达合法阶段，禁止 boolean 组合制造非法状态；`any`、`as never` 和非空断言不能绕过当前合同；
- selector 返回稳定最小投影；memoization 用于已证明的 identity/render 问题，不承担状态正确性；stream delta 不应无意义地重建稳定 footer、Dock 或全页面对象；
- Wails/Plugin composition root 必须显式声明启动依赖，生产冷启动不得依赖测试注入、开发热更新或偶然注册顺序。

### 8.5 变更要求

- 从失败的产品反例开始：先证明当前 owner 在何种真实交错下违反不变量，再修改生产代码；
- breaking change 必须一次删除旧 contract、调用方、codec、测试和文档，不保留 alias、fallback、dual read/write 或 shadow owner；
- 只有改动涉及异步交接、事务或 replacement 时，才说明 admission identity、linearization point、durable winner 和失败行为；普通局部重构只需说明真实 owner、被删除边界和可观察结果；
- 后端参考 `/Users/tangerg/Desktop/study/codex-server/codex-rs`，前端主参考 `/Users/tangerg/Desktop/study/codex`，zcode/minimax 只作补充。参考只提供反证与机制证据，不授权复制 package、状态流或多 connection 产品设计；
- 测试优先覆盖一个 Desktop 与一个逻辑 Runtime 内的真实交错、SQLite 不变量、同一 Runtime 的进程重启、SIGKILL/回执丢失、renderer replacement、长对话和异步泄露。Desktop 与 CLI 共享目录时，只测试已经存在的存储并发合同；不扩张成通用多客户端 race；
- 精确暂存本批文件，提交前确认 `app/cli` 和无关改动未进入 diff；使用浏览器自动化后关闭本批会话和 daemon，不关闭用户已有 Wails/Vite/Chrome 进程。

## 9. 文档纪律

- [`ARCHITECTURE.md`](ARCHITECTURE.md) 只记录稳定目标；
- [`DECISIONS.md`](DECISIONS.md) 只记录裁决与理由；
- [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 只记录实施标尺；
- [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 只记录当前授权、长期约束、里程碑索引和下一阶段准入；
- [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 只记录当前能力事实、owner、verdict 和验收证据；
- [`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 只记录当前冻结边界/digest/version。

可变的 count、version、digest、阶段状态和当前实现事实只能有一份 owner 文档，其他文档只引用。稳定不变量可以为使合同自洽而简短复述，但不能复制一份会独立演进的字段/状态清单。实现改变后先更新 owner 文档，再修正引用；注释若已可由命名和结构表达则删除，不能保留“旧实现曾经……”的漂移历史。

逐批命令输出、文件清单和提交叙述由 Git 历史拥有，不继续追加到 owner 文档。阶段完成时压缩为里程碑结论；若详细审计材料有长期价值，保存独立、不可变的设计/事故记录并由正文引用。

## 10. 批次完成定义

一个实施批次只有同时满足以下条件才算完成：

- 该批授权的语义纵切全部实现，无 TODO、stub、dead path 或临时 alias；
- 该批已经接管的生产纵切，其旧实现和旧测试已删除，不存在双 owner。P4–P7 是经 ADR-RT-035 特批的 parallel harness program：每批只要求新路径当期无重复 owner/stub，并维护 P8 原子切换删除清单；仍服务生产旧路径的 owner 统一在 P8 同批删除；
- 受影响 package 命名、GoDoc、schema、fixture 和文档同步；
- 受影响 build、vet、staticcheck 和行为测试全绿；race、fuzz、contract、SQLite、Frontend 或真实产品验收按该批实际风险选择，里程碑封板再运行完整门禁；
- `git diff --check`、`go mod tidy`/workspace diff 无意外漂移；
- Capability Ledger 写入真实验收证据；
- Execution Plan 更新进度和下一步；
- 形成一个可独立 revert 的提交并及时推送。

测试失败、consumer 尚未接线或外部阶段明确延后时，必须准确记录未完成边界，不能把“主体完成”写成完成。
