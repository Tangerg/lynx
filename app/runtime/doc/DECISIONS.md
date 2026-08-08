# Lyra Runtime 架构决策记录

> 状态：持续维护
>
> 范围：`app/runtime` 重构的已接受决策；后续改变结论必须追加明确取代关系的 ADR，不能静默改写历史理由

本文只记录为什么选择某个方向及其后果。目标结构由 [`ARCHITECTURE.md`](ARCHITECTURE.md) 拥有，实施顺序和进度由 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 拥有，当前事实由 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有。

## ADR-RT-001：原模块治本重构，不创建完整 `runtime2`

- 状态：已接受。
- 背景：Runtime 拥有 Session、Run、Transcript、Store、协议和大量产品集成。平行模块会制造双领域模型、双数据库和双协议真相；它不像独立 Framework 那样可以脱离真实消费者完成验证。
- 决策：在现有 `app/runtime` 中保留成熟产品能力，对执行纵切面进行绿色重写。允许使用独立分支或 worktree 隔离开发，但最终只有一个 Runtime module/path。
- 后果：从零重新设计，不从零复制全部代码；旧实现按纵切替换后立即删除。

## ADR-RT-002：Run 是应用中心

- 状态：已接受。
- 决策：Run 是一次用户可见的完整逻辑执行；Segment 是 Run 的连续输出区段。Application 以 Run 生命周期组织 admission、waiting、resume、terminal 和事件提交。
- 后果：Agent loop、transport request、model call 和 Process 都不能成为产品主聚合。

## ADR-RT-003：产品 Run 与 Framework Process/Execution 分离

- 状态：已接受。
- 决策：`Run`、`Segment` 属于 Runtime；`Process`、`Execution` 属于 Agent2。二者只能在 `adapter/agentexec` 映射。
- 后果：Application 保存应用侧不透明 executor identity，不保存 Agent2 concrete handle 或 Strategy payload。

## ADR-RT-004：`domain/execution` 改名为 `domain/run`

- 状态：已接受。
- 背景：当前 package 实际拥有产品 Run，但 Agent2 已把 Execution 定义为正式框架术语。
- 决策：重构阶段一次性改名并重新裁定子上下文所有权，不保留 alias 或 forwarding package。
- 后果：代码、测试、SQLite adapter、协议 projection 和文档统一使用 Run 语言。

## ADR-RT-005：Conversation、Transcript、Knowledge 和 WorkingContext 是不同真相源

- 状态：已接受。
- 决策：Conversation 是产品模型历史；Transcript 是 Items-and-Runs 权威观察记录；Knowledge 是用户可编辑长期知识；WorkingContext 是 Agent2 Process 私有恢复状态。
- 后果：它们不能相互重建或混合持久化。Conversation truncate/fork 不允许静默改变已存在 Process 的恢复状态。

## ADR-RT-006：采用 Clean Architecture 的单向依赖 DAG

- 状态：已接受。
- 决策：Domain 最内；Application 只依赖 Domain 和消费方端口；Adapter/Infra 实现外部能力；Delivery 驱动 Application；Bootstrap 是唯一组合根。
- 后果：任何向外 import、跨环 service locator 或 concrete Engine/Store 泄露都是架构回归，并由 fitness tests 阻止。

## ADR-RT-007：以领域 package 抑制层级过度设计

- 状态：已接受。
- 决策：保留 `domain/application/adapter/infra/delivery` 作为复杂应用的明确依赖环，但每一环内部按领域或真实能力命名，默认一层深；不建立 `service/repository/manager/impl` 套娃。
- 后果：Clean Architecture 不成为接口和目录生成器；新 package 必须证明独立变化原因和依赖切断价值。

## ADR-RT-008：接口由消费者定义

- 状态：已接受。
- 决策：Application use case 在自身 package 定义实际消费的窄端口；Adapter 返回 concrete implementation。单实现且无边界价值的内部协作直接使用 concrete type。
- 后果：禁止通用 Repository、Store、Manager、Policy、Provider 大接口和 implementor-owned god SPI。

## ADR-RT-009：跨聚合事务由具体 Application use case 拥有

- 状态：已接受。
- 决策：Run admission、waiting barrier、terminal tree、rollback 等各自拥有准确 write-set 和顺序；Domain 不执行 I/O，Framework 不提供 Store/transaction SPI。
- 后果：不建立通用 UnitOfWork、Saga、CQRS 或 EventBus。业务幂等由真实应用入口/adapter 按稳定业务身份实现。

## ADR-RT-010：Agent2 只通过 `adapter/agentexec` 消费

- 状态：已接受。
- 决策：Agent2 import 收敛到 `adapter/agentexec` 及明确的 framework-integration 叶子；Domain、Application、Delivery、Infra 和通用 Toolset 不认识 Agent2。
- 后果：Process、Signal、Effect、Deployment、TreeSnapshot concrete type 和 Strategy payload 不越过防腐层。

## ADR-RT-011：根聊天使用原生 Interaction

- 状态：已接受。
- 背景：普通聊天没有 Planning world state、预测 Action 或 replan 语义。
- 决策：删除“GOAP 单 Action 包裹完整 Interaction”，直接部署 `interaction.Definition`。
- 后果：Planning/GOAP 只在真实规划用例被产品选择时组合，不作为默认聊天中心。

## ADR-RT-012：Agent2 Engine 是唯一执行生命周期所有者

- 状态：已接受。
- 决策：不迁移 `TurnProcess`、第二 controller、外置 ToolLoop、child scheduler 或 continuation state machine。Runtime 只通过 Agent2 公共合同驱动和观察 Process tree。
- 后果：现有 `adapter/agentexec/turn` 中重复 Framework 责任的代码全部删除，而不是换 import path。

## ADR-RT-013：每棵 root Run tree 使用独立 Engine

- 状态：已接受。
- 决策：按 Run 冻结 exact root/child Deployments、resolver、limits、capabilities、listeners 和 admitter。
- 后果：Runtime 不建立全局 Agent facade 或跨 Run 的共享 mutable Agent lifecycle registry；agentexec 可以在一个明确 owner 的 per-root tree session 内保存 Engine/Process handle 和 reconciliation state，Application 可以保存 Run command routing/pump state。Process lifecycle 仍由对应 Engine 唯一拥有。

## ADR-RT-014：Agent2 Platform 不是迁移前置条件

- 状态：已接受。
- 决策：当前使用 caller-owned exact resolver。只有产品出现真实 Deployment 发布、版本路由和治理用例时才接入 Platform。
- 后果：不为了“完整平台感”提前引入第二 catalog、selector 或治理层。

## ADR-RT-015：TreeSnapshot 对 Runtime 内环不透明

- 状态：已接受。
- 决策：`adapter/agentexec` 负责 Agent2 capture/restore；Application checkpoint 只保存 Host metadata 和 opaque payload。
- 后果：Runtime 不解析 ExecutionState、Interaction phase、mailbox、child protocol 或 snapshot private JSON。

## ADR-RT-016：恢复只使用 Process 自有 snapshot

- 状态：已接受。
- 决策：Conversation 只用于新 Process seed；恢复不能从当前 Conversation 重新计算 WorkingContext。
- 后果：外部世界变化通过 BuildID/workspace revision/产品 policy 决定拒绝恢复、清理或新开 Process，不修改 snapshot 内容。

## ADR-RT-017：Framework Delta 不是权威记录

- 状态：已接受。
- 决策：模型 chunk、ToolCall、Tool result、usage 和 pricing 在 model/tool 调用链内同步投影。Agent2 Event/Delta 只承载生命周期与临时流。
- 后果：listener 丢失、panic 隔离或 Delta drop 不得改变最终结果和 Transcript 正确性。

## ADR-RT-018：Invocation context 是执行归因边界

- 状态：已接受。
- 决策：Runtime decorator 使用 Agent2 `ModelInvocation`/`ToolInvocation` 获取 ProcessRelation、exact DeploymentRef、EffectID、Step/model-call/ToolCall sequence。
- 后果：不恢复旧 ProcessContext、全局 dependency scope 或 Engine 反查 API。

## ADR-RT-019：Managed Delegate child 是产品 child Run 的明确来源

- 状态：已接受。
- 决策：ProcessAdmitter 在 child 发布前完成 Application-owned durable admission；Delegate child key 在模型观察和 admission 两端使用 Interaction 同一算法。
- 后果：Agent2 不认识 Run；Framework-internal Planning/Workflow child 是否投影为 Run，由 agentexec 的产品语义映射决定。

## ADR-RT-020：Waiting subtree cancellation 使用 plan/commit/apply

- 状态：核心顺序保留，具体边界已由 ADR-RT-042 收紧。
- 决策：先取得 Framework prospective change，再提交 Application write-set，成功后 Apply 到同一 live tree。
- 后果：Runtime 不改写 Interaction state；Agent2 不持有 Host transaction。Baseline 9 的 pure plan 无法在 transaction 期间防止 source 漂移，因此不能直接作为最终实现，必须采用 ADR-RT-042 的 one-shot prepared change。

## ADR-RT-021：Steer 和 interrupt answer 是产品命令，不是通用 Signal API

- 状态：已接受。
- 决策：Application 暴露 `SteerRun`、回答 Interrupt 等语义命令；agentexec 在边界构造对应 Signal。
- 后果：Application/Delivery 不出现任意 Signal payload 提交入口，也不形成第二个推进通道。

## ADR-RT-022：Toolset 保持 Framework-neutral

- 状态：已接受。
- 决策：通用 Toolset 只理解 Tool、产品端口和执行 capability。Interaction-specific deferred advertisement 放在 agentexec decorator 或通过准确回调注入。
- 后果：不能让 `search_tools` 把 Agent2 import 扩散到整个 Toolset，也不能动态增加未冻结的执行权限。

## ADR-RT-023：Adapter 与 Infra 不整体合并

- 状态：已接受。
- 决策：Adapter 翻译/组合应用语义；Infra 提供低层技术 mechanism。Adapter 可以依赖 Infra，Infra 不反向依赖 Adapter/Application。
- 后果：每个包逐一审计；纯透传 wrapper 合并，真实复用机制保留。目录名不能替代职责证明。

## ADR-RT-024：Delivery Server 与 Dispatch 分离

- 状态：已接受。
- 决策：协议方法实现/projection 与通用 JSON-RPC registry/router 保持不同 package；保留准确的 `server` 和 `dispatch` 名称，不为目录对称改成语义更宽的 `api`/`rpc`，也不合包。
- 后果：HTTP 和 in-process transport 复用 RPC dispatch，业务入口只驱动 Application。

## ADR-RT-025：消除 `component` 杂物分类

- 状态：已接受。
- 决策：单一 owner 的原语回归 owner；真正被多个平级消费者复用的中性原语以准确能力名存在于 `internal`，不保留通用 `component/common/core/utils` 收纳层。
- 后果：迁移按 import graph 和所有权逐个完成，不机械内联或制造反向依赖。

## ADR-RT-026：Protocol 机器制品是外部合同真相源

- 状态：已接受。
- 决策：方法、字段、错误和 union 的机器真相在 `contract/`；`API.md`、`TRANSPORT.md` 和 `AUX_API.md` 解释语义，不复制 generated catalog。
- 后果：breaking change 只产生一个新 shape；服务端和消费者可以分阶段接线，但不保留双字段/双方法兼容。

## ADR-RT-027：SQLite 只有当前精确 schema epoch

- 状态：已接受。
- 决策：开发阶段 schema 变化直接提升 epoch，拒绝旧数据库；不写 migration、dual read/write 或猜测修复。
- 后果：每次持久化 breaking change 同批更新 schema、codec、测试和 baseline。

## ADR-RT-028：产品 accounting 不下沉到 Framework Usage

- 状态：已接受。
- 决策：模型 token、provider/model、USD cost 和产品 budget 属于 Runtime accounting；Agent2 Usage 只保留框架中性资源事实。
- 后果：agentexec 负责归因和翻译，不修改 Agent2 Usage schema 迎合产品报表。

## ADR-RT-029：Application 允许官方 OTel API，不允许 OTel SDK

- 状态：已接受。
- 决策：拥有用例语义的 Application/Adapter/Delivery 可以直接使用官方 trace/metric API；exporter、provider 生命周期和 SDK 只在 composition/observability adapter。
- 后果：telemetry 不能成为业务输入，不能自造 tracer/meter facade，也不能在领域代码散落日志。

## ADR-RT-030：breaking change 一步到位

- 状态：已接受。
- 决策：内部 API、目录、wire 和 schema 选择正确形态后直接替换；不保留 deprecated alias、compat package、双读、双写或 `v2` 后缀。
- 后果：每批必须控制爆炸半径、同步真实消费者、全绿后提交，而不是用兼容债换短期编译。

## ADR-RT-031：Server 与前端消费者分阶段改造

- 状态：已接受。
- 决策：本轮 Runtime 重构只处理服务端及直接后端爆炸半径；前端、TUI、CLI 的协议接线作为后续独立阶段。
- 后果：服务端 contract 仍必须同步生成并记录漂移，不能用兼容字段维持旧消费者；阶段完成事实必须明确哪些 consumer 尚未接线。

## ADR-RT-032：旧 `agent` 最终删除，`agent2` 恢复唯一模块名

- 状态：已接受。
- 决策：App 所有旧 Agent consumer 迁移完成后，删除旧模块并把 `agent2` 目录/module path 原子改回 `agent`。
- 后果：`agent2` 只是重构期间路径，不能进入长期 Runtime protocol、Domain 语言或产品配置。

## ADR-RT-033：架构规则必须机器执行

- 状态：已接受。
- 决策：import DAG、Agent SDK isolation、wire owner、goroutine lifecycle、API digest、schema version、命名禁区和无兼容路径使用 architecture/baseline tests 守卫。
- 后果：文档约定若可被静态或行为测试精确表达，就不能只依赖 review 记忆。

## ADR-RT-034：不引入应用框架式抽象

- 状态：已接受。
- 决策：不引入 DI container、service locator、EventBus、Mediator、generic Repository、generic policy chain、CQRS/Saga framework 或 extension registry。
- 后果：扩展点只在真实消费者处以一个准确的小接口存在；单实现内部依赖优先 concrete type。

## ADR-RT-035：阶段内完成语义，不提交半成品

- 状态：已接受。
- 决策：阶段可以拆成多个可验证纵切批次，但每个提交必须自洽、可构建、无 TODO/stub/死路径；P4–P7 的新 adapter 只由真实 harness 消费，能力齐备前不接管生产 Run，P8 一次切换并删除旧路径。临时迁移类型必须在其授权阶段结束前删除。
- 后果：进度由 Execution Plan 记录，不能以“以后补”解释已知错误边界。

## ADR-RT-036：Run 终态由 Application intent 与 Agent2 Termination 共同决定

- 状态：已接受。
- 决策：不从 `error`、`context.Canceled` 或 Delta 推断终态。Completion、cancellation、deadline 和 classified failure 首先按 Agent2 Termination 读取；Runtime budget、已提交 teardown、recovery loss 等产品意图再决定精确 Run outcome。
- 后果：deadline 保留独立 TimedOut outcome；未知 Engine kill fail closed；已提交终态后的 executor teardown 不得覆盖产品结果；映射在 agentexec/Application 边界集中测试。

## ADR-RT-037：Unknown Effect 当前 fail closed，不开放通用裁决 API

- 状态：已接受。
- 决策：live 或 recovery 发现 Agent2 unknown Effect 时都不自动重放，不从 UI/协议接收 arbitrary Settlement，也不伪造 Interaction owner payload。agentexec 在 Dispatcher 返回 indeterminate error 前标记并唤醒 per-tree reconciliation；Run pump 仍通过公共 `UnknownEffectIDs` 对账，listener 丢失由有界 reconciliation tick 兜底。
- 决策：Application 先原子提交 incomplete 诊断、RunLost、checkpoint invalidation 和 cleanup intent，成功后才 Kill/release tree；事务失败则让 Process 保持 Framework 的 unknown wait 并重试。unknown 与尚未终结的 cancel/deadline 竞争时 RunLost 优先，控制意图保留为诊断；已提交 terminal 仍 first-wins。
- 后果：当前无需泄露 `ResolveEffect`。未来人工裁决必须由 Strategy owner 的 typed resolution contract 和独立产品 ADR 驱动。

## ADR-RT-038：Application executor port 由纵切消费者逐步发现，P8 才冻结

- 状态：已接受。
- 背景：P3 尚未有 Agent2 真实消费者，若一次设计 Start/Wait/Steer/Subtree 全部方法，只会把现有宽接口换一套名字，违背 consumer-owned interface。
- 决策：P3 只建立 root start/observe/cancel 所需的最小候选；P4 验证并修订 root shape，P6/P7 分别在 waiting/restore 与 child/subtree 消费者出现时增加准确能力。P8 原子生产切换前完成整体命名、参数、error 和 GoDoc 审计并冻结。
- 后果：P4–P7 允许有意的 breaking 演进，但每个阶段的当期能力必须完整、受真实 harness 消费且无 placeholder。Contract Baseline 在 P8 前只冻结禁止泄露的语义边界，不冻结精确方法集。

## ADR-RT-039：初版不启用单 Process prepared-step durability acknowledgment

- 状态：已接受，P6 已实施并验证。
- 背景：Agent2 `PreparedStepAcknowledger` 只提供一个 Process `Snapshot`，而包含 child relation/child wait 的树只能从完整 `TreeSnapshot` 恢复。Runtime 无权拼接 Agent2 private tree wire。
- 决策：初版 EngineConfig 不配置 acknowledger。Runtime 只承诺从 Application 已原子提交的 quiescent complete-tree checkpoint 恢复；active tree/step 在进程崩溃且没有该边界时以 `RunLost` 收口。
- 后果：不宣称 Effect-level crash durability。未来只有 Agent2 先提供中性 tree-wide durability contract，且产品有真实 pre-dispatch crash recovery 需求时才启用；Framework 仍不取得 Runtime Store/transaction。

## ADR-RT-040：外部调用与 authoritative projection 使用 dispatch-attempt 协议

- 状态：已接受。
- 决策：agentexec outer Dispatcher 为每个 EffectRequest 创建独立、并发安全并按 EffectID 校验的 attempt tracker，通过 context 传给 model/tool decorators，不使用全局表。外部调用前必须 durable 写入 started fact；失败则不调用外部能力并返回 definite failure。外部调用已经开始后，最终响应/Tool result/usage 的 durable write 失败会把 attempt 标记为 indeterminate，outer Dispatcher 丢弃 inner definite settlement 并返回 error，使 Engine 记录 unknown。
- 决策：模型 chunk/progress 是 best-effort 临时投影，drop 可观察但不改变 settlement；完整 final 必须独立持久化。Tool batch 中任一已执行调用的 post-call write 失败使整个 Effect unknown；后续未开始的串行调用停止，并行 in-flight 调用先结算再汇总。Transcript 保留 started/incomplete 事实，不伪造 final/result。
- 决策：canonical Tool result 提交后，可重新读取的 product state projection 与 post-Tool hook 只属于 observation；其失败进入 tracing，不得反向把已确定 settlement 改为 unknown。Runtime 自己的 bounded Delta queue drop 同样产生 OTel event，不能静默宣称流完整。
- 后果：Event/Delta listener 不参与 authoritative transaction；recorder failure 不会被 Interaction 吞成普通 Tool error，也不会触发不安全自动重试。P5 已通过 pre/post-call、chunk drop、并发 partial commit、lost wake 与 terminal retry 反例验证该协议。

## ADR-RT-041：Durable Process admission 必须有 conclusive start outcome

- 状态：已接受，Agent2 前置合同待补齐。
- 背景：Agent2 Baseline 9 在 ProcessAdmitter 成功后仍会执行可失败的 Definition.Start、initial capture/restore 和 register。root 失败可由直接 `Engine.Start` 调用者收口；child 失败只返回不含 prospective ProcessID 的 parent result，没有 post-admission aborted fact。Runtime 若已 durable 创建 child Run，会留下无法确定的 Opening 记录。
- 决策：Application 先 durable 创建 root Opening Run/Segment；root admission 只绑定 prospective executor identity，Started fact 后转 Running，直接 start error 或启动崩溃分别收口为 start failure/RunLost。Delegate child 的 model ToolCall 必须先提交，child admission 只创建不可见 opening reservation/binding；收到 Framework conclusive started 后才公开 Run。
- 决策：P7 开始前，Agent2 必须提供中性、带 prospective identity 的 admitted→started/aborted 结果，或把 admission 放入一个批准后 publication 不再失败的 Framework reservation。Runtime 不使用 timeout、private ID derivation、Effect 顺序或 tree wire猜测 child outcome。
- 后果：本项推翻“Agent2 当前没有已知迁移 blocker”的旧判断。Framework 合同只描述 Process start lifecycle，不出现 Run、Store、transaction 或产品幂等。

## ADR-RT-042：Waiting subtree 应用原子性需要 one-shot prepared tree change

- 状态：已接受，Agent2 前置合同待补齐；补充并收紧 ADR-RT-020。
- 背景：Baseline 9 的 pure `WaitingSubtreeCancellationPlan` 在返回前释放 quiescence。Application durable commit 期间 sibling/tree 仍可能推进，随后 Apply 可 stale；此时已提交的 resulting checkpoint 与 live 外部副作用无法证明一致。
- 决策：Agent2 必须提供中性的 one-shot prepared change，在 `Apply`/`Discard` 前保持 source tree frozen，并暴露 resulting TreeSnapshot 与 canceled/paused Process IDs。agentexec concrete capability 持有 Framework value；Application 只看 canceled/paused member 和 opaque checkpoint，并在当前 use case 内通过准确的小 capability 调用 Apply/Discard，不序列化 plan 或 lock。
- 后果：capability one-shot，Discard 幂等，并绑定 Host-owned deadline；agentexec 取得后立即 `defer Discard`。transaction failure 调用 Discard，零 Framework mutation；commit 后 Apply，崩溃则恢复 committed resulting checkpoint。跨过 apply gate 后必须完成；任何仍无法证明的 apply failure 都丢弃旧 live tree并恢复 resulting checkpoint，失败则 RunLost。不得复活旧通用 Mutation lease，也不得仅靠文档假定 plan 不会 stale。

## ADR-RT-043：回答 waiting Interrupt 会原子作废旧恢复点

- 状态：已接受，P6 已实施并验证。
- 决策：Application 在同一事务中 claim exact answer set、把 interrupt row 变为普通读取不可见的 `resuming`，并删除旧 waiting checkpoint。事务成功后 agentexec 才 stage exact live tree 或 `RestoreTree`；Application 的 next-Segment opening transaction 必须再次证明该 root 存在 durable claim，commit 成功后才提交 WaitID-addressed semantic Signal。从 claim 线性化点到下一个 quiescent checkpoint 提交前，进程崩溃一律 `RunLost`，不能再次恢复旧 snapshot 重放 answer 或 Effect。
- 决策：`resuming` row 保留答案审计和 crash diagnosis，下一 waiting barrier 只能由相同 Session/executor/root-member owner 原子替换，terminal/recovery 删除。claim 后、opening 前任一 validation/restore/observe/opening failure 先 durable `RunLost` 再 release live tree；若 terminal write 失败则保持 tree 与 hidden claim，不能先假装清理。
- 后果：claim 前崩溃仍可使用 waiting checkpoint；claim 后、restore 后、Signal accepted 后的崩溃都不能回退。新 checkpoint 提交后恢复能力重新建立。P6 已覆盖 claim transaction rollback、hidden answer audit、boot loss、post-claim fail-before-release 与 next-barrier replacement。

## ADR-RT-044：Interrupt 纯语义与 pending continuation envelope 分层

- 状态：已接受，P2 已实施。
- 背景：一个 durable pending hand-off 同时携带产品 Interrupt、Run tree continuation、opaque executor member binding 和恢复元数据。把完整 envelope 放进 Domain 会迫使 `interrupt` 认识 Application/executor；把 Interrupt 语义留在 Application 又会让领域语言失去 owner。
- 决策：`domain/interrupt` 只拥有 Kind、Key、Resolution 和纯决策语义；`domain/transcript` 拥有可观察 question/approval Item projection；`application/runs.Pending` 是跨聚合、跨 executor 的 root-tree continuation envelope。SQLite 只持 technical record，`adapter/persistence` 负责 Application value 与 record 的双向映射。
- 后果：Domain 不出现 Store、context 或 executor binding；Infra 不反向 import Application；P6 可以重写 waiting/restore port，而无需破坏 Interrupt 领域语言或伪造第二套 Framework snapshot。

## ADR-RT-045：Root execution 使用 stage/commit/begin，Release 不承担产品 Cancel

- 状态：已接受，P3 已实施；精确方法 shape 仍受 ADR-RT-038 约束，到 P8 才冻结。
- 背景：旧 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 把启动、观察、产品取消、资源清理、读取和多个 write-set 混成实现镜像。`CancelExecution` 同时被用于产品取消和无产品事实的 cleanup，使调用顺序与 owner 不清晰；`ProcessID` 又把 Framework 术语带入 Application 和 technical storage。
- 决策：root start 当前分为 `ValidateRootStart`、不跨 model/tool side-effect boundary 的 `StageRoot`、durable opening commit 和 commit 后 `BeginRoot`；Observation 必须在 opening 前成功 attach，失败或 opening reject 只调用 `Release`。Application 的 product Cancel 先决定并提交 Run outcome，随后才 Release executor resource；自然终态和失败终态同样 Release，Waiting boundary 不 Release。
- 决策：Coordinator 只保存各 use case 实际消费的窄端口；`SessionPorts`/`ProjectionPorts` 仅是 Bootstrap 参数分组，不成为运行时 facade。executor tree 的 Application 语言统一为 `ExecutorMember`/`MemberID`，SQLite epoch 59 直接采用 `root_member_id`/`memberId`，不保留 `ProcessID` alias、旧列或 dual codec。
- 后果：P4 真实 Agent2 root consumer 可以修订 root candidate，但不得重新合并 product Cancel 与 resource Release，也不得把 Framework `Process` 语言带回 Application。P6/P7 的 provisional legacy seams 必须在对应纵切中被真实 consumer shape 替换。

## ADR-RT-046：Fresh WorkingContext 由 Application 组装，final assistant message 来自 Result

- 状态：已接受，P4 已实施。
- 背景：旧 root request 只携带拆开的 prompt text/media，完整历史由旧 Agent middleware 隐式反查。这既会丢失多模态消息顺序，也把产品 Conversation 读取藏进 executor。另一方面，Agent2 Delta 是 best-effort observation，不能作为完整 assistant output 的真相源。
- 决策：Application 在 fresh Run admission gate 内读取已验证 Host Conversation，追加当前 canonical user message，并把完整 provider-neutral `WorkingContext` seed 交给 `StageRoot`。agentexec 必须拥有该 seed 的副本；Process 开始或恢复后不再回读 mutable Conversation。成功终态由 `Process.Await` 的 `Result.Output` 投影一个 Application-owned `AssistantMessageCompleted`；text/reasoning Delta 只改善实时体验，Reducer 以 final message 覆盖 partial/missing streaming observation。
- 后果：Conversation 仍是产品模型历史，WorkingContext 仍是 executor 私有运行/恢复状态，二者没有共享可变所有权。当前旧生产 owner 为 P8 parallel cutover 暂时继续消费 legacy text/media 字段，但 Agent2 路径只接受完整 WorkingContext；P8 删除旧 owner 时同批删除 legacy request 表达，不保留 dual API。

## ADR-RT-047：Invocation journal 与 semantic Transcript 分离，并发 Tool final 按模型顺序成批提交

- 状态：已接受，P5 已实施。
- 背景：model/tool 外部调用需要 durable started/terminal 边界，但 Tool 并发完成顺序不是模型声明顺序。若 Tool start 预先插入 Transcript，就会用外部调度时序抢占用户可见 Item 顺序；若每个完成结果独立写入，则后完成的前序调用失败时会留下无法证明因果完整性的部分批次。
- 决策：SQLite model/tool invocation journal 只记录 operational attempt state，不复制 final assistant message、Tool result 或 Run accounting。Model final + cumulative usage/pricing + Run progress 在一个 Application write-set 中提交。Tool start 只进入 invocation journal；Tool final Item 按 `(modelCallSequence, toolCallIndex)` 排序，Run pump 暂存乱序 completion，只提交最长连续前缀并一起完成该前缀所有 receipt。
- 决策：canonical Tool batch 任一写失败，speculative completion 全部丢弃，started journal 保留；outer Dispatcher 将整个 Effect 标为 unknown，Application 原子提交 incomplete Items 与 `RunLost`。稳定 Runtime call identity、provider source-call identity和模型位置分别保存，不能互相解析或替代。
- 后果：Transcript insertion order 重新只表达产品语义；并发 Tool 不需要全局串行化。SQLite shape 直接提升到 epoch 61，无 migration、dual journal 或兼容列；P8 切生产前仍可按真实 consumer 修订内部 port 名称，但不能重新合并 operational 与 semantic truth。

## ADR-RT-048：Native waiting 只经 Interaction pending-input ACL，不兼容解释旧 suspension

- 状态：已接受，P6 已实施。
- 背景：产品 ask-user、approval 和 plan-exit 共享 `runs.Interrupt` 语义，但旧生产 owner 使用 old Agent suspension。若 native Interaction 复用旧 package、双读 old private JSON，或让 Toolset import Agent2，就会把迁移兼容性变成新的永久边界。
- 决策：framework-neutral `interruptcodec` 只编码产品 prompt/resolution；`interactioninput` 是唯一 Agent2 ACL，负责 capability freeze、Tool continuation state digest、public pending input 和 response Signal。旧 `suspension` adapter 只复用 product codec 并保留在 P8 精确删除台账，新 Interaction files 对旧 Agent import 为零；Toolset 通过 `runs.InterruptFunc` 注入 native capability，保持对两个 Framework 都零依赖。
- 决策：Interactive approval 首次进入时冻结 effective arguments、policy prompt 与 logical call identity；restore 直接解析该 prompt 并 resolve，不能重跑 pre-hook/authorization plan。Interaction 自己的 deferred advertisement 留在 TreeSnapshot 内，Runtime 不建第二份 advertised-tool 状态。
- 后果：真实 `ask_user`、approval restore、deferred advertisement、corrupt prompt/state 与 capability mismatch 可分别测试；P8 删除旧 owner 时只换 composition owner，不需要迁移产品 Interrupt 或 Tool schema。
