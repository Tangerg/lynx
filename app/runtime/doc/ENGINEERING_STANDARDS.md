# Lyra Runtime 重构工程标准

> 状态：强制执行
>
> 适用范围：`app/runtime` 重构的设计、实现、测试、评审、提交和文档维护

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

- Domain/Application 出现 Agent2 Process、Signal、Effect、Deployment 或 snapshot payload；
- Agent2 出现 Run、Session、pricing、approval、Store 或 Runtime protocol；
- Delivery 持有 executor handle、Run pump、Store 或 SDK client；
- Infra 决定应用事务、Run terminal policy 或 UI projection；
- Toolset 取得完整 Engine/Stack 或 Interaction private state；
- Bootstrap 暴露 god object 让下游随取随用。

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

- 禁止 Application、Adapter、Infra、Delivery、Bootstrap、Agent2、SQLite、JSON-RPC 和外部驱动 SDK import；
- 领域错误不引用协议 code 或 HTTP status；
- 时间、随机数等不确定输入由 Application 提供准确值，领域方法消费值而不是隐式读全局；
- 纯计算使用 value receiver 时保持值语义，mutable entity 使用 pointer receiver 并集中修改。

### 3.2 Application

- 一个 package 围绕一组高度相关 use cases；
- 接口定义在消费者文件附近，通常 1–3 个方法；
- I/O 和长任务首参数是 `context.Context`，不存入 struct；
- 明确写出 validation、claim、durable commit、external apply、publish 的顺序；
- 失败路径必须说明哪些事实尚未改变、哪些已经提交、如何 fail closed；
- 不导入具体 Adapter/Infra/Delivery/Agent2；
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
- RPC dispatch 不解释业务字段；Transport 不解释 RPC method；
- HTTP/SSE 与 in-process 使用同一个 application entrypoint；
- 协议类型只在 delivery/contract 边界，Application 不返回 wire DTO。

### 3.6 Bootstrap

- 依赖创建和 wiring 明确、静态、可搜索；
- 不使用反射 DI container、service locator 或 package globals；
- 所有 closers 只有一个 owner，逆序、幂等关闭；
- 后台任务加入明确 task group，Host shutdown 等待它们结束；
- optional capability 只在真实配置关闭时为 nil/absent，不通过半可用对象推迟错误。

## 4. Agent2 接线标准

### 4.1 Agent2 public API only

- 只调用 Agent2 已冻结的公共合同；
- 不读取 private JSON、未导出的 wire 或测试 helper；
- 不把旧 Agent 行为当兼容规范；
- 需要新 Framework 能力时，先证明是中性框架缺口，而不是 Runtime 产品策略，再单独修改 Agent Framework ADR/baseline。

### 4.2 一个执行循环

- Agent2 Engine 唯一驱动 Process；
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
- answer claim 必须原子作废旧 waiting recovery point。进入 Resuming 后直到新 quiescent checkpoint 提交，任何 crash 都不能回退旧 snapshot。

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

### 4.5 Child admission

- root/child prospective identity 在发布前完成 admission；
- Application admission 允许有界 I/O，但不得重入 Engine/Process；
- 同一 prospective identity 重试的业务正确性由 Runtime adapter 负责；
- child Run binding 使用稳定因果 key，不依赖 goroutine 完成顺序；
- restore 不重复创建已存在 child Run；
- admission 只创建 Opening/reservation，Framework conclusive started 后才发布 Running；
- admission success 后的每一个 Framework 失败点都必须产生同一 prospective identity 的 aborted outcome，或由 Framework 保证批准后 publication 不再失败。没有该合同不得启用 durable child admission；
- 不用 timeout、private identity derivation、Event 顺序或 parent Effect payload 推断 ghost child。

### 4.6 Waiting subtree change

- Application 不接触 `WaitingSubtreeCancellationPlan`、ProcessID、Engine lock 或 source digest；
- agentexec concrete one-shot capability 唯一持有 prepared Framework change；Application 只看 member projections、opaque resulting checkpoint 和准确的 Apply/Discard capability；
- prepared change 在 Apply/Discard 前保持 source tree frozen；Application transaction 不持有自己不拥有的 Framework lock；
- capability 必须 one-shot、Discard 幂等、有 Host-owned deadline；agentexec 获取后立即 `defer Discard`，禁止遗漏清理导致 tree 无限冻结；
- transaction failure 必须 Discard，commit 后必须 Apply；commit 后 crash 恢复 resulting checkpoint，无法证明的 apply failure 丢弃旧 live tree并从该 checkpoint 恢复，失败则 RunLost。

## 5. Go API 标准

### 5.1 Naming

- package 名简短、准确、无 `runtime.RuntimeX` 或 `toolset.XTool` 口吃；
- 类型名描述本质，不使用 `Impl`、`Service`、`Manager`、`Helper`；
- 方法使用动作语义，不使用含糊 `Handle`、`Process`、`Prepare`，除非该词本身是领域概念且 GoDoc 定义边界；
- accessor 不加 `Get`；
- bool 参数改为准确 option/value type，避免调用点无法阅读；
- 时间字段使用 `StartedAt`、`FinishedAt`、`Duration`/`DurationMillis` 的唯一语义，不存可派生重复事实。

### 5.2 Types

- 零值有清晰语义；无合法零值的类型只能通过构造函数创建；
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
- Provider/network/process 必须有明确 timeout 或上层 deadline；
- 后台任务需要继承 trace values 但脱离 request cancel 时使用 `context.WithoutCancel`，并由 Host 生命周期另行取消；
- 不用 `context.Background()` 掩盖 owner 缺失。
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

每个阶段至少覆盖：

1. Domain table-driven invariant tests；
2. Application use-case tests，使用最小 handwritten fakes；
3. Adapter contract/translation tests；
4. SQLite/HTTP/in-process integration tests；
5. Agent2 真实 Engine 纵切 consumer tests；
6. architecture/static baseline tests；
7. concurrency race tests；
8. 对 snapshot、protocol、strict codec 的 fuzz/round-trip tests。

### 7.2 真实纵切

Agent2 迁移不能只用 mock Engine 证明。必须运行真实 Interaction + Engine，覆盖：

- start/stream/terminal；
- model ToolCall 与 authoritative projection；
- waiting snapshot/restore/answer；
- steer 安全边界；
- Delegate child admission 与 child Run 映射；
- deferred advertisement；
- waiting subtree prepare/commit/apply-or-discard；
- cancel、deadline、provider/tool failure 和 corrupt checkpoint。

### 7.3 Architecture fitness tests

至少机器守卫：

- 精确 package DAG；
- Agent2 import allowlist；
- Application/Domain/Delivery 禁止外环 SDK；
- Delivery 不持有 Run/executor lifecycle state；
- Infra 不 import Application/Adapter/Delivery；
- 不存在 `component/common/core/utils` 杂物层；
- 不存在旧 Agent import、旧路径、旧术语和 compat codec；
- public/protocol/wire baseline 未经 ADR 不漂移；
- exported GoDoc、参数名和 error wrapping 合同。

## 8. 文档纪律

- [`ARCHITECTURE.md`](ARCHITECTURE.md) 只记录稳定目标；
- [`DECISIONS.md`](DECISIONS.md) 只记录裁决与理由；
- [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 只记录实施标尺；
- [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 只记录阶段、授权范围、批次和进度；
- [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 只记录当前能力事实、迁移 verdict 和验收证据；
- [`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 只记录当前冻结边界/digest/version。

可变的 count、version、digest、阶段状态和当前实现事实只能有一份 owner 文档，其他文档只引用。稳定不变量可以为使合同自洽而简短复述，但不能复制一份会独立演进的字段/状态清单。实现改变后先更新 owner 文档，再修正引用；注释若已可由命名和结构表达则删除，不能保留“旧实现曾经……”的漂移历史。

## 9. 批次完成定义

一个实施批次只有同时满足以下条件才算完成：

- 该批授权的语义纵切全部实现，无 TODO、stub、dead path 或临时 alias；
- 该批已经接管的生产纵切，其旧实现和旧测试已删除，不存在双 owner。P4–P7 是经 ADR-RT-035 特批的 parallel harness program：每批只要求新路径当期无重复 owner/stub，并维护 P8 原子切换删除清单；仍服务生产旧路径的 owner 统一在 P8 同批删除；
- 受影响 package 命名、GoDoc、schema、fixture 和文档同步；
- build、vet、staticcheck、普通测试、race 及该批相关 fuzz 全绿；
- `git diff --check`、`go mod tidy`/workspace diff 无意外漂移；
- Capability Ledger 写入真实验收证据；
- Execution Plan 更新进度和下一步；
- 形成一个可独立 revert 的提交并及时推送。

测试失败、consumer 尚未接线或外部阶段明确延后时，必须准确记录未完成边界，不能把“主体完成”写成完成。
