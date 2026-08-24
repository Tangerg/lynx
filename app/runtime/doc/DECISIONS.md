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
- 决策：`Run`、`Segment` 属于 Runtime；`Process`、`Execution` 属于 Agent Framework。二者只能在 `adapter/agentexec` 映射。
- 后果：Application 保存应用侧不透明 executor identity，不保存 Agent Framework concrete handle 或 Strategy payload。

## ADR-RT-004：`domain/execution` 改名为 `domain/run`

- 状态：已接受。
- 背景：当前 package 实际拥有产品 Run，但 Agent Framework 已把 Execution 定义为正式框架术语。
- 决策：重构阶段一次性改名并重新裁定子上下文所有权，不保留 alias 或 forwarding package。
- 后果：代码、测试、SQLite adapter、协议 projection 和文档统一使用 Run 语言。

## ADR-RT-005：Conversation、Transcript、Knowledge 和 WorkingContext 是不同真相源

- 状态：已接受。
- 决策：Conversation 是产品模型历史；Transcript 是 Items-and-Runs 权威观察记录；Knowledge 是用户可编辑长期知识；WorkingContext 是 Agent Framework Process 私有恢复状态。
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

## ADR-RT-010：Agent Framework 只通过 `adapter/agentexec` 消费

- 状态：已接受。
- 决策：Agent Framework import 收敛到 `adapter/agentexec` 及明确的 framework-integration 叶子；Domain、Application、Delivery、Infra 和通用 Toolset 不认识 Agent Framework。
- 后果：Process、Signal、Effect、Deployment、TreeSnapshot concrete type 和 Strategy payload 不越过防腐层。

## ADR-RT-011：根聊天使用 Interaction

- 状态：已接受。
- 背景：普通聊天没有 Planning world state、预测 Action 或 replan 语义。
- 决策：删除“GOAP 单 Action 包裹完整 Interaction”，直接部署 `interaction.Definition`。
- 后果：Planning/GOAP 只在真实规划用例被产品选择时组合，不作为默认聊天中心。

## ADR-RT-012：Agent Framework Engine 是唯一执行生命周期所有者

- 状态：已接受。
- 决策：不迁移 `TurnProcess`、第二 controller、外置 ToolLoop、child scheduler 或 continuation state machine。Runtime 只通过 Agent Framework 公共合同驱动和观察 Process tree。
- 后果：现有 `adapter/agentexec/turn` 中重复 Framework 责任的代码全部删除，而不是换 import path。

## ADR-RT-013：每棵 root Run tree 使用独立 Engine

- 状态：已接受。
- 决策：按 Run 冻结 exact root/child Deployments、resolver、limits、capabilities、listeners 和 admitter。
- 后果：Runtime 不建立全局 Agent facade 或跨 Run 的共享 mutable Agent lifecycle registry；agentexec 可以在一个明确 owner 的 per-root tree session 内保存 Engine/Process handle 和 reconciliation state，Application 可以保存 Run command routing/pump state。Process lifecycle 仍由对应 Engine 唯一拥有。

## ADR-RT-014：Agent Framework Platform 不是迁移前置条件

- 状态：已接受。
- 决策：当前使用 caller-owned exact resolver。只有产品出现真实 Deployment 发布、版本路由和治理用例时才接入 Platform。
- 后果：不为了“完整平台感”提前引入第二 catalog、selector 或治理层。

## ADR-RT-015：TreeSnapshot 对 Runtime 内环不透明

- 状态：已接受。
- 决策：`adapter/agentexec` 负责 Agent Framework capture/restore；Application checkpoint 只保存 Host metadata 和 opaque payload。
- 后果：Runtime 不解析 ExecutionState、Interaction phase、mailbox、child protocol 或 snapshot private JSON。

## ADR-RT-016：恢复只使用 Process 自有 snapshot

- 状态：已接受。
- 决策：Conversation 只用于新 Process seed；恢复不能从当前 Conversation 重新计算 WorkingContext。
- 后果：外部世界变化通过 BuildID/workspace revision/产品 policy 决定拒绝恢复、清理或新开 Process，不修改 snapshot 内容。

## ADR-RT-017：Framework Delta 不是权威记录

- 状态：已接受。
- 决策：模型 chunk、ToolCall、Tool result、usage 和 pricing 在 model/tool 调用链内同步投影。Agent Framework Event/Delta 只承载生命周期与临时流。
- 后果：listener 丢失、panic 隔离或 Delta drop 不得改变最终结果和 Transcript 正确性。

## ADR-RT-018：Invocation context 是执行归因边界

- 状态：已接受。
- 决策：Runtime decorator 使用 Agent Framework `ModelInvocation`/`ToolInvocation` 获取 ProcessRelation、exact DeploymentRef、EffectID、Step/model-call/ToolCall sequence。
- 后果：不恢复旧 ProcessContext、全局 dependency scope 或 Engine 反查 API。

## ADR-RT-019：Managed Delegate child 是产品 child Run 的明确来源

- 状态：已接受。
- 决策：ProcessAdmitter 在 child 发布前完成 Application-owned durable admission；Delegate child key 在模型观察和 admission 两端使用 Interaction 同一算法。
- 后果：Agent Framework 不认识 Run；Framework-internal Planning/Workflow child 是否投影为 Run，由 agentexec 的产品语义映射决定。

## ADR-RT-020：Waiting subtree cancellation 使用 plan/commit/apply

- 状态：核心顺序保留，具体边界已由 ADR-RT-042 收紧。
- 决策：先取得 Framework prospective change，再提交 Application write-set，成功后 Apply 到同一 live tree。
- 后果：Runtime 不改写 Interaction state；Agent Framework 不持有 Host transaction。Baseline 9 的 pure plan 无法在 transaction 期间防止 source 漂移，因此不能直接作为最终实现，必须采用 ADR-RT-042 的 one-shot prepared change。

## ADR-RT-021：Steer 和 interrupt answer 是产品命令，不是通用 Signal API

- 状态：已接受。
- 决策：Application 暴露 `SteerRun`、回答 Interrupt 等语义命令；agentexec 在边界构造对应 Signal。
- 后果：Application/Delivery 不出现任意 Signal payload 提交入口，也不形成第二个推进通道。

## ADR-RT-022：Toolset 保持 Framework-neutral

- 状态：已接受。
- 决策：通用 Toolset 只理解 Tool、产品端口和执行 capability。Interaction-specific deferred advertisement 放在 agentexec decorator 或通过准确回调注入。
- 后果：不能让 `search_tools` 把 Agent Framework import 扩散到整个 Toolset，也不能动态增加未冻结的执行权限。

## ADR-RT-023：Adapter 与 Infra 不整体合并

- 状态：已接受，P9 已实施。
- 决策：Adapter 翻译/组合应用语义；Infra 提供低层技术 mechanism。Adapter 可以依赖 Infra，Infra 不反向依赖 Adapter/Application。
- 后果：每个包已按真实 import graph 与调用职责逐一审计；Adapter 保留应用语义翻译、组合和外部 SDK 防腐，Infra 保留可复用技术机制。重复的物理路径判定已统一归 `infra/pathidentity`；未发现需要整体合并的环或仅为目录对称存在的生产 wrapper。永久 DAG guard 防止 Infra 反向依赖 Adapter/Application。目录名不能替代职责证明。

## ADR-RT-024：Delivery Server 与 Dispatch 分离

- 状态：已接受。
- 决策：协议方法实现/projection 与通用 JSON-RPC registry/router 保持不同 package；保留准确的 `server` 和 `dispatch` 名称，不为目录对称改成语义更宽的 `api`/`rpc`，也不合包。
- 后果：HTTP transport 复用 RPC dispatch，业务入口只驱动 Application。已删除无生产消费者且位于 `internal`、无法充当公共 SDK 的 in-process 原型；未来新增公共 binding 仍必须复用同一 dispatch/Application 路径。

## ADR-RT-025：消除 `component` 杂物分类

- 状态：已接受，P9 已实施；其中三个 Application mechanism 的物理 owner 由 ADR-RT-052 收紧。
- 决策：单一 owner 的原语回归 owner；真正被多个平级消费者复用的中性原语以准确能力名存在于 `internal`，不保留通用 `component/common/core/utils` 收纳层。
- 后果：Run replay cursor 的业务 shape 回归 `application/runs`；teardown/path identity 归 Infra，secret masking 归 Application，notification relay 归 Adapter。`component` 目录和 temporary exception 已删除。pagination、opaque-token 与 taskgroup 的准确环内位置见 ADR-RT-052。

## ADR-RT-026：Protocol 机器制品是外部合同真相源

- 状态：已接受，P10 已实施。
- 决策：方法、字段、错误和 union 的机器真相在 `contract/`；`API.md`、`TRANSPORT.md` 和 `AUX_API.md` 解释语义，不复制 generated catalog。
- 后果：breaking change 只产生一个新 shape；服务端 `2026-08-10` / Artifact v15 已直接替换旧版本，replay scope 只保留 `runtimeInstanceRootSegment`。canonical samples 必须同时通过 Go round-trip 与同类型 strict validator。服务端和消费者可以分阶段接线，但不保留双字段/双方法兼容。

## ADR-RT-027：SQLite 只有当前精确 schema epoch

- 状态：已接受。
- 决策：开发阶段 schema 变化直接提升 epoch，拒绝旧数据库；不写 migration、dual read/write 或猜测修复。
- 后果：每次持久化 breaking change 同批更新 schema、codec、测试和 baseline。

## ADR-RT-028：产品 accounting 不下沉到 Framework Usage

- 状态：已接受。
- 决策：模型 token、provider/model、USD cost 和产品 budget 属于 Runtime accounting；Agent Framework Usage 只保留框架中性资源事实。
- 后果：agentexec 负责归因和翻译，不修改 Agent Framework Usage schema 迎合产品报表。

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

## ADR-RT-032：原框架实现最终删除，重写实现取得唯一 `agent` 模块名

- 状态：已接受，P11 已实施。
- 决策：App 所有原框架 consumer 迁移完成后，整体删除原实现，并把绿色重写实现原子安装为唯一 `agent` 目录/module path。
- 后果：临时孵化路径已经退休，不能进入 Runtime protocol、Domain 语言或产品配置；禁止 alias、replace compatibility 和双 framework path 回流。

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

## ADR-RT-036：Run 终态由 Application intent 与 Agent Framework Termination 共同决定

- 状态：已接受。
- 决策：不从 `error`、`context.Canceled` 或 Delta 推断终态。Completion、cancellation、deadline 和 classified failure 首先按 Agent Framework Termination 读取；Runtime budget、已提交 teardown、recovery loss 等产品意图再决定精确 Run outcome。
- 后果：deadline 保留独立 TimedOut outcome；未知 Engine kill fail closed；已提交终态后的 executor teardown 不得覆盖产品结果；映射在 agentexec/Application 边界集中测试。

## ADR-RT-037：Unknown Effect 当前 fail closed，不开放通用裁决 API

- 状态：已接受。
- 决策：live 或 recovery 发现 Agent Framework unknown Effect 时都不自动重放，不从 UI/协议接收 arbitrary Settlement，也不伪造 Interaction owner payload。agentexec 在 Dispatcher 返回 indeterminate error 前标记并唤醒 per-tree reconciliation；Run pump 仍通过公共 `UnknownEffectIDs` 对账，listener 丢失由有界 reconciliation tick 兜底。
- 决策：Application 先原子提交 incomplete 诊断、RunLost、checkpoint invalidation 和 cleanup intent，成功后才 Kill/release tree；事务失败则让 Process 保持 Framework 的 unknown wait 并重试。unknown 与尚未终结的 cancel/deadline 竞争时 RunLost 优先，控制意图保留为诊断；已提交 terminal 仍 first-wins。
- 后果：当前无需泄露 `ResolveEffect`。未来人工裁决必须由 Strategy owner 的 typed resolution contract 和独立产品 ADR 驱动。

## ADR-RT-038：Application executor port 由纵切消费者逐步发现，P8 冻结

- 状态：已接受，P8 已完成。
- 背景：P3 尚未有 Agent Framework 真实消费者，若一次设计 Start/Wait/Steer/Subtree 全部方法，只会把现有宽接口换一套名字，违背 consumer-owned interface。
- 决策：P3 只建立 root start/observe/cancel 所需的最小候选；P4 验证并修订 root shape，P6/P7 分别在 waiting/restore 与 child/subtree 消费者出现时增加准确能力。P8 原子生产切换前完成整体命名、参数、error 和 GoDoc 审计并冻结。
- 后果：P4–P7 允许的 breaking 演进已经结束。生产端口以 P8 真实 Bootstrap consumer 为准；后续只有新的独立用例和 superseding ADR 才能扩展，不能重新制造宽 Framework facade。

## ADR-RT-039：初版不启用单 Process prepared-step durability acknowledgment

- 状态：已接受，P6 已实施并验证。
- 背景：Agent Framework `PreparedStepAcknowledger` 只提供一个 Process `Snapshot`，而包含 child relation/child wait 的树只能从完整 `TreeSnapshot` 恢复。Runtime 无权拼接 Agent Framework private tree wire。
- 决策：初版 EngineConfig 不配置 acknowledger。Runtime 只承诺从 Application 已原子提交的 quiescent complete-tree checkpoint 恢复；active tree/step 在进程崩溃且没有该边界时以 `RunLost` 收口。
- 后果：不宣称 Effect-level crash durability。未来只有 Agent Framework 先提供中性 tree-wide durability contract，且产品有真实 pre-dispatch crash recovery 需求时才启用；Framework 仍不取得 Runtime Store/transaction。

## ADR-RT-040：外部调用与 authoritative projection 使用 dispatch-attempt 协议

- 状态：已接受。
- 决策：agentexec outer Dispatcher 为每个 EffectRequest 创建独立、并发安全并按 EffectID 校验的 attempt tracker，通过 context 传给 model/tool decorators，不使用全局表。外部调用前必须 durable 写入 started fact；失败则不调用外部能力并返回 definite failure。外部调用已经开始后，最终响应/Tool result/usage 的 durable write 失败会把 attempt 标记为 indeterminate，outer Dispatcher 丢弃 inner definite settlement 并返回 error，使 Engine 记录 unknown。
- 决策：模型 chunk/progress 是 best-effort 临时投影，drop 可观察但不改变 settlement；完整 final 必须独立持久化。Tool batch 中任一已执行调用的 post-call write 失败使整个 Effect unknown；后续未开始的串行调用停止，并行 in-flight 调用先结算再汇总。Transcript 保留 started/incomplete 事实，不伪造 final/result。
- 决策：canonical Tool result 提交后，可重新读取的 product state projection 与 post-Tool hook 只属于 observation；其失败进入 tracing，不得反向把已确定 settlement 改为 unknown。Runtime 自己的 bounded Delta queue drop 同样产生 OTel event，不能静默宣称流完整。
- 后果：Event/Delta listener 不参与 authoritative transaction；recorder failure 不会被 Interaction 吞成普通 Tool error，也不会触发不安全自动重试。P5 已通过 pre/post-call、chunk drop、并发 partial commit、lost wake 与 terminal retry 反例验证该协议。

## ADR-RT-041：Durable Process admission 必须有 conclusive start outcome

- 状态：已接受，P7 已实施并验证；Agent Framework 中性合同形成 Baseline 10。
- 背景：Agent Framework Baseline 9 在 ProcessAdmitter 成功后仍会执行可失败的 Definition.Start、initial capture/restore 和 register。root 失败可由直接 `Engine.Start` 调用者收口；child 失败只返回不含 prospective ProcessID 的 parent result，没有 post-admission aborted fact。Runtime 若已 durable 创建 child Run，会留下无法确定的 Opening 记录。
- 决策：Application 先 durable 创建 root Opening Run/Segment；root admission 只绑定 prospective executor identity，Started fact 后转 Running，直接 start error 或启动崩溃分别收口为 start failure/RunLost。Delegate child 的 model ToolCall 必须先提交，child admission 只创建不可见 opening reservation/binding；收到 Framework conclusive started 后才公开 Run。
- 决策：Agent Framework 以中性 `ProcessStartOutcomeAcknowledger` 提供带 prospective identity 的 admitted→started/aborted 结果。Runtime 不使用 timeout、private ID derivation、Effect 顺序或 tree wire猜测 child outcome；root/child 共用同一 reservation→conclusive outcome 协议。
- 后果：本项推翻“Agent Framework 当前没有已知迁移 blocker”的旧判断。Framework 合同只描述 Process start lifecycle，不出现 Run、Store、transaction 或产品幂等。

## ADR-RT-042：Waiting subtree 应用原子性需要 one-shot prepared tree change

- 状态：已接受，P7 已实施并验证；补充并收紧 ADR-RT-020，Agent Framework 合同形成 Baseline 14。
- 背景：Baseline 9 的 pure `WaitingSubtreeCancellationPlan` 在返回前释放 quiescence。Application durable commit 期间 sibling/tree 仍可能推进，随后 Apply 可 stale；此时已提交的 resulting checkpoint 与 live 外部副作用无法证明一致。
- 决策：Agent Framework 提供中性的 one-shot prepared change，在 `Apply`/`Discard` 前保持 source tree frozen，并暴露 resulting TreeSnapshot 与 canceled/paused Process IDs。agentexec concrete capability 持有 Framework value；Application 只看 canceled/paused member 和 opaque checkpoint，并在当前 use case 内通过准确的小 capability 调用 Apply/Discard，不序列化 plan 或 lock。所有可失败、可取消 staging 都在 Prepare 返回前完成；Agent Framework `Apply()` 不接受 context，因为 application transaction 提交后已经不存在合法的请求取消点。Runtime execution ACL 将状态安装 `Apply` 与移除最后外部边界后的 Process `Continue(ctx)` 分离，二者失败不能混称。
- 后果：capability one-shot，Discard 幂等，并绑定只作用于 preparation/transaction 的 Host-owned deadline；transaction failure 调用 Discard，零 Framework mutation。commit 后 contextless Apply，崩溃则恢复 committed resulting checkpoint。任何仍无法证明的 apply failure 都先释放旧 owner并通过 Application `WaitingExecutionRestorer` 恢复 exact resulting checkpoint；恢复失败才 RunLost。不得复活旧通用 Mutation lease，也不得仅靠文档假定 plan 不会 stale。

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
- 后果：P4 真实 Agent Framework root consumer 可以修订 root candidate，但不得重新合并 product Cancel 与 resource Release，也不得把 Framework `Process` 语言带回 Application。P6/P7 的 provisional legacy seams 必须在对应纵切中被真实 consumer shape 替换。

## ADR-RT-046：Fresh WorkingContext 由 Application 组装，final assistant message 来自 Result

- 状态：已接受，P4 已实施。
- 背景：旧 root request 只携带拆开的 prompt text/media，完整历史由旧 Agent middleware 隐式反查。这既会丢失多模态消息顺序，也把产品 Conversation 读取藏进 executor。另一方面，Agent Framework Delta 是 best-effort observation，不能作为完整 assistant output 的真相源。
- 决策：Application 在 fresh Run admission gate 内读取已验证 Host Conversation，追加当前 canonical user message，并把完整 provider-neutral `WorkingContext` seed 交给 `StageRoot`。agentexec 必须拥有该 seed 的副本；Process 开始或恢复后不再回读 mutable Conversation。成功终态由 `Process.Await` 的 `Result.Output` 投影一个 Application-owned `AssistantMessageCompleted`；text/reasoning Delta 只改善实时体验，Reducer 以 final message 覆盖 partial/missing streaming observation。
- 后果：Conversation 仍是产品模型历史，WorkingContext 仍是 executor 私有运行/恢复状态，二者没有共享可变所有权。当前旧生产 owner 为 P8 parallel cutover 暂时继续消费 legacy text/media 字段，但 Agent Framework 路径只接受完整 WorkingContext；P8 删除旧 owner 时同批删除 legacy request 表达，不保留 dual API。

## ADR-RT-047：Invocation journal 与 semantic Transcript 分离，并发 Tool final 按模型顺序成批提交

- 状态：已接受，P5 已实施。
- 背景：model/tool 外部调用需要 durable started/terminal 边界，但 Tool 并发完成顺序不是模型声明顺序。若 Tool start 预先插入 Transcript，就会用外部调度时序抢占用户可见 Item 顺序；若每个完成结果独立写入，则后完成的前序调用失败时会留下无法证明因果完整性的部分批次。
- 决策：SQLite model/tool invocation journal 只记录 operational attempt state，不复制 final assistant message、Tool result 或 Run accounting。Model final + cumulative usage/pricing + Run progress 在一个 Application write-set 中提交。Tool start 只进入 invocation journal；Tool final Item 按 `(modelCallSequence, toolCallIndex)` 排序，Run pump 暂存乱序 completion，只提交最长连续前缀并一起完成该前缀所有 receipt。
- 决策：canonical Tool batch 任一写失败，speculative completion 全部丢弃，started journal 保留；outer Dispatcher 将整个 Effect 标为 unknown，Application 原子提交 incomplete Items 与 `RunLost`。稳定 Runtime call identity、provider source-call identity和模型位置分别保存，不能互相解析或替代。
- 后果：Transcript insertion order 重新只表达产品语义；并发 Tool 不需要全局串行化。SQLite shape 直接提升到 epoch 61，无 migration、dual journal 或兼容列；P8 已冻结相关 consumer port，不能重新合并 operational 与 semantic truth。

## ADR-RT-048：Interaction waiting 只经 pending-input ACL，不兼容解释旧 suspension

- 状态：已接受，P6 已实施；codec 的物理拆包决定由 ADR-RT-051 取代。
- 背景：产品 ask-user、approval 和 plan-exit 共享 `runs.Interrupt` 语义，但旧生产 owner 使用 old Agent suspension。若 Interaction 复用旧 package、双读 old private JSON，或让 Toolset import Agent Framework，就会把迁移兼容性变成新的永久边界。
- 决策：framework-neutral `interruptcodec` 只编码产品 prompt/resolution；`interactioninput` 是唯一 Agent Framework ACL，负责 capability freeze、Tool continuation state digest、public pending input 和 response Signal。旧 private suspension adapter 已在 P8 删除；Toolset 通过 `runs.InterruptFunc` 注入 pending-input capability，保持对 Framework 零依赖。
- 决策：Interactive approval 首次进入时冻结 effective arguments、policy prompt 与 logical call identity；restore 直接解析该 prompt 并 resolve，不能重跑 pre-hook/authorization plan。Interaction 自己的 deferred advertisement 留在 TreeSnapshot 内，Runtime 不建第二份 advertised-tool 状态。
- 后果：真实 `ask_user`、approval restore、deferred advertisement、corrupt prompt/state 与 capability mismatch 可分别测试；生产切换未迁移产品 Interrupt 或 Tool schema。

## ADR-RT-049：取消是控制请求，观察与资源释放保持分离

- 状态：已接受，P8 已实施。
- 背景：若用户 cancel 直接取消 Run pump 的 observation context，Application 会在 Agent Framework 到达安全边界并给出确定 Termination 前失去唯一事实流，也会把请求方 context 生命周期误当成执行终态。
- 决策：Application 先 durable 记录 cancel intent，再调用 `RunningRootCancellationRequester` 请求 Framework 停止；Run pump 保持附着，直到 terminal/unknown write-set 成功提交。`ExecutionReleaser` 只在确定终态后释放 tree，不决定产品 outcome。
- 后果：cancel 生效延迟等于当前 Strategy safe step；取消、deadline、unknown 与 terminal 继续由 first-wins matrix 裁决，请求 context 超时只结束调用等待，不静默杀死产品 Run。

## ADR-RT-050：Tool 可见性保持 Framework-neutral，广告能力在执行边界注入

- 状态：已接受，P8 已实施。
- 背景：让通用 Toolset 返回 Agent Framework ToolGroup、持有 delegate Tool，或 fallback 到某个 ToolLoop，会把执行策略和 private lifecycle 扩散进产品工具目录，并形成第二份 visible/deferred authority。
- 决策：`toolset.Resolver` 只返回 framework-neutral `Manifest{Visible, Deferred}`；需要动态广告的 Tool 只依赖最小 `ToolAdvertiser` capability。agentexec 在真实 Interaction invocation context 中绑定 `AdvertiseTools`，无绑定时 fail closed，不保留 legacy fallback。
- 后果：Tool schema、authorization 与 visibility 仍由 Runtime 单一 owner 管理；Agent Framework 只在冻结的 Deployment 内执行/广告工具，Toolset 对 Agent Framework 零 import。

## ADR-RT-051：Interaction input ACL 统一拥有 continuation codec

- 状态：已接受，P12 已实施；取代 ADR-RT-048 中把 prompt/resolution codec 拆成独立子包的物理组织，不改变 waiting 领域边界。
- 背景：`interruptcodec` 只有 `interactioninput` 一个生产消费者，却额外形成一个单边 package，并让两个 package 各自维护一套 strict JSON decoder。P12 fuzz 进一步证明标准 `encoding/json` 会把 `Answers` 当作 `answers` 接受，并让空 answers 在第一次与第二次 round-trip 间产生 non-nil/nil 双表示；“拆包保持 framework-neutral”没有换来独立变化轴，反而分裂了同一个 continuation contract。
- 决策：`interactioninput` 唯一拥有 capability freeze、Tool continuation identity、prompt/resolution wire、pending input 和 response Signal。其 decoder 递归拒绝大小写别名、未知字段和 trailing value，成功值在进入产品 `Interrupt`/`Resolution` 前规范化空集合。删除 `interruptcodec` package、转发函数和第二 decoder；Toolset 仍只依赖 `runs.InterruptFunc`，Domain/Product 值仍不认识 Agent Framework 或 JSON。
- 后果：一个 package 对一个变化原因负责，pending continuation、prompt 与 resolution 共用同一 strict contract；单元测试和三个 fuzz owner 同时守住 exact field name、canonical round-trip 与任意输入不 panic。这个收敛没有把 Runtime persistence、transaction、Run 或 UI 抽象带入 Agent Framework。

## ADR-RT-052：共享引用不产生跨环所有权

- 状态：已接受，P18 已实施。
- 背景：P17 把 pagination、opaque-token framing 和 task group 作为根级 shared capability 保留，但完整调用图显示分页策略和后台任务启动都只由 Application use case 决定；Delivery 只消费分页结果/错误，Bootstrap 只构造和关闭 Application task group。opaque-token framing 的两个语义 owner 也都是 Application continuation：paged read 与 Run replay。把引用方误计为行为 owner，会让根级 package 逃避已经明确的环归属。
- 决策：`pagination`、`taskgroup`、`opaquetoken` 分别归 `application/pagination`、`application/taskgroup`、`application/opaquetoken`。Pagination 继续拥有 keyset anchor、query binding、limit 和 `Page[T]`；Taskgroup 继续拥有 Application component 的 request-detached work；opaque-token 只拥有 Application continuation 的 strict URL-safe framing。标准库 Base64 在这里是 continuation contract 的纯算法，不是媒体内容 transport codec；门禁只为该精确 package 区分二者，不建立宽泛 encoding 例外。
- 后果：根级 shared capability 只保留真正跨环的 completion、HTTP origin 与 idempotency。Delivery/Bootstrap 可以向内依赖 Application owner，但不能因此把机制提升成无环归属的 `internal` primitive；旧三个根路径永久禁止，不保留 alias、shim、`util` 或 `common`。

## ADR-RT-053：公开类型化 `protocol` 与嵌入式 Runtime binding

- 状态：已接受；取代 ADR-RT-024 中“当前没有公共同进程 binding”的现状结论，不改变 `server` 与 HTTP envelope dispatch 分责。
- 背景：`app/cli` 已成为真实的同进程消费者。让它经 loopback HTTP 使用同一进程会额外引入 listener、鉴权 token、JSON 编解码和 SSE framing；复制 Session、Run、Event、Interrupt DTO 又会形成第二协议真相。此前删除的 `internal/delivery/transport/inprocess` 只是 JSON-RPC envelope 的 channel transport，既不能被外部 module 导入，也不是稳定的类型化 Runtime API。
- 决策：把 binding-neutral 的 DTO、枚举、请求/响应、事件、客户端可见错误、版本和严格验证原子移动到公共 `runtime/protocol`。稳定错误由 sentinel 支持 `errors.Is`，结构化恢复事实由窄 `ProblemError` 支持 `errors.As`；HTTP 与公共 `runtime/embedded` 只消费这一份合同。旧 `internal/delivery/protocol` 物理删除，不保留 alias、forwarding package 或双份类型。
- 决策：公共 `embedded.Open` 返回 concrete `*embedded.Runtime`。它公开 Runtime 的完整类型化能力与显式 command/subscription options，不导出胖 `Runtime` interface，也不暴露 Application concrete type、Host、Store、Engine、Router、context private key 或 JSON-RPC envelope。消费方按需要自行定义窄接口。
- 决策：新增私有 binding-neutral `internal/delivery/operation`，唯一拥有 operation catalog、严格 request validation、静态/动态 capability gate、幂等 claim/fingerprint/complete/replay、transport-neutral problem projection 与 run-stream replay attachment。HTTP `dispatch` 只负责 JSON-RPC envelope、method routing、numeric error code 和 frame encoding；embedded 直接调用同一 typed operation，不经过 HTTP、JSON-RPC 或 SSE。
- 决策：`RequestMeta` 是公共请求自描述值，但其 context 传播属于私有 operation 实现。`AfterEventID` 与 `IdempotencyKey` 在 embedded options 中显式表达；HTTP adapter 分别从 `Last-Event-Id` 与 `Idempotency-Key` 投影到同一 operation options。numeric JSON-RPC error code、HTTP status 与 transport problem 不进入公共 protocol。
- 决策：HTTP 进程宿主与 embedded binding 共用同一个 Runtime instance builder。该 owner 在打开 SQLite/recovery 前取得 canonical data directory 的进程级独占锁，按顺序组装 stores、Host、恢复器、operation endpoint 和 Runtime workers；`Close` 停止新请求、解除订阅、取消并 join 后台任务、逆序关闭资源，最后释放锁。请求 context 只约束当前调用/订阅，已接受 Run 继续由 Runtime lifecycle 拥有。
- 决策：`contract/go-api.json` 由 contract generator 从 `protocol + embedded` 的真实 Go type information 派生，完整冻结公共 package、constant、variable、function、type、field、method 与 visible import。架构门禁拒绝第三个公共 package 和任何 public signature 的 `internal` 类型，不用手写 API 清单形成第二真相源。
- 后果：Runtime 是唯一服务端合同；CLI、前端和 TUI 后续按新公共面 breaking 接线，Runtime 不迁就旧消费者接口。一个数据目录同一时间只能由一个 HTTP 或 embedded Runtime owner 打开；需要多进程共享时必须使用单独宿主进程或其他 IPC，不能绕过独占锁。

## ADR-RT-054：WorkingContext 来源由 Agent execution adapter 类型化归因

- 状态：已接受并实施，P22 完成；不改变 Runtime Protocol、Artifact、SQLite 或 Agent Framework 合同。
- 背景：fresh root prompt 已组合 base prompt、用户/项目 Knowledge、pinned/recalled Memory、AGENTS.md cascade、Session Plan 与 lifecycle hook context。此前最终只剩文本，checkpoint 虽能自包含恢复，却无法回答某段上下文来自哪个 owner、应作为 instruction 还是 data，也无法证明预算裁剪后模型实际看到了哪些 Memory/Agent document。把这些产品来源提升进 Agent Framework 或公共 Runtime Protocol 都会泄露 owner。
- 决策：`adapter/agentexec` 在唯一 WorkingContext composition boundary 内建立私有 typed provenance：来源 kind、精确可用 reference、`instruction`/`data` purpose 与 schema version。prompt 继续按原有顺序和文本渲染；只有实际通过预算并写入消息的 pinned Memory 与 Agent document 才进入来源集合。recalled Memory 单独标记为 data；SessionStart/UserPromptSubmit hook context 标记在其注入的 user Part 上。
- 决策：provenance 通过 `core/chat` 已有 JSON-safe Message/Part metadata 随完整 WorkingContext 一起进入 Interaction checkpoint，不建立第二 store、索引或 protocol DTO。Agent Framework 仍只把 WorkingContext 当 Strategy-owned opaque value；Application、Delivery、Infra、CLI 和 provider policy 都不解释这些私有 kind。
- 决策：metadata 只描述来源，不复制内容、不改变 provider-facing 文本、不绕过 Message validation。若未来需要公共诊断 surface，必须由真实 consumer 另行定义安全投影，不能直接暴露 checkpoint/private metadata 或让 Runtime Protocol 依赖 Adapter 类型。
- 后果：恢复后的上下文继续自包含且文本语义不变，同时内部诊断可精确解释实际可见来源。知识读取失败、memory recall 失败和 hook policy 仍遵守既有 best-effort/blocking 决策；本 ADR 不改变其错误语义。

## ADR-RT-055：Goal provenance 使用 objective incarnation，不使用 drive lease

- 状态：已接受并实施，P32 完成；取代旧 Goal 设计中把 objective identity 与 process-local drive ownership 合并为 lease 的现状，不改写历史阶段记录。
- 背景：一个 Goal Run 可以在 HITL waiting 后由客户端恢复并继续终结。若 `Goal.Resume` 更换 durable lease，已被旧 lease 接纳的 Run 会变成“外来 Run”：它的 terminal accounting 和模型终态不能再作用于同一目标；新 drive 又可能从 Resume 前快照启动额外 Run，造成预算漏计、已完成目标继续运行。process-local drive 已由显式 cancel/join 边界管理，不需要借 durable Goal identity 表达所有权。
- 决策：`Goal.IncarnationID` 是一个 objective incarnation 的不可变身份，只在 fresh `Start` 创建；Pause、Resume、Stop 与 boot Reconcile 均保留它。Run、Pending、Interrupt、execution scope 和 checkpoint 只携带 `GoalIncarnationID` 作为来源戳；同一 parked Run 恢复后仍可向原目标提交 terminal accounting/outcome，fresh objective 则以新 incarnation 确定性隔离旧 Run。
- 决策：Goal drive 的 goroutine、cancellation 与 completion 仍是 Application process-local ownership，由 `quiesceDrive`/join 关闭。`WaitSessionStartable` 只是观察边界，不是 reservation；每次等待返回后、尝试 Run admission 前，driver 必须重读权威 Goal 并先结算 budget/terminal 状态。
- 决策：SQLite 直接提升至 epoch 68，列统一为 `incarnation_id`/`goal_incarnation_id`；executor checkpoint policy 直接提升至 v2。旧 lease 列、字段和 v1 policy 均确定性拒绝，不提供 migration、alias 或 dual codec。
- 后果：HITL Resume 不再切断 outstanding Run 与目标的账务/终态关系，等待中的 drive 也不能基于陈旧状态多启动 Run；fresh objective 仍能隔离 straggler。改动只重塑 Runtime Domain/Application/Infra/Adapter 内部 provenance，没有把 Goal、Run、Store、checkpoint 或 drive 类型泄露给 Agent Framework、Protocol 或消费者。

## ADR-RT-056：Question 的已接受响应是 Transcript 事实

- 状态：已接受并实施，P34 完成。
- 背景：Interrupt row 的 hidden answer audit 只服务 continuation claim，Run resume 后会删除；Question Transcript 只保存 prompt。两个客户端并发提交不同答案时，失败客户端只能继续显示本地草稿，重载后又完全丢失答案；取消也可能把未接受草稿伪装成最终响应。
- 决策：`domain/transcript.Question` 以 optional `Answers` 不可变补充唯一已接受响应；nil 精确表示没有已接受响应。Application 从已验证 Pending + resolution 形成 expected/replacement，`runsegment` 在 exact resume claim 的同一事务内替换 Transcript，再推进 checkpoint/Pending。SQLite 只实现 CAS/codec，不解释答案；Delivery、Artifact v18、Protocol 和 Desktop 只投影该事实。
- 决策：本地前端 settle 只作为 claim 返回与 Runtime 投影间的延迟桥。Runtime 一旦给出 settled Item，前端必须丢弃本地答案；取消/无答案显示已关闭但未回答。不得用客户端 winner 推断、interrupt audit 反查或 UI sanitizer 形成第二真相源。
- 后果：并发 answer/resume 仍只有一个线性化 winner，所有客户端和重放最终显示相同响应；事务中任一 Transcript/claim/checkpoint 写失败都会整体回滚。该语义不进入 Agent Framework，Framework 仍只接收 typed answer Signal。

## ADR-RT-057：Goal 冻结并继承 Runtime Run capabilities

- 状态：已接受并实施，P34 完成。
- 背景：普通 Run 在 Delivery admission 时协商 question/approval/child 能力，但 Goal loop 曾刻意构造空 capability Run，导致同一 Runtime 的 `ask_user` 在自治 Goal 中被拒绝。仅在 Goal 内无条件打开能力会绕过客户端协商，Resume 换客户端时也会静默提升权限。
- 决策：fresh `Goal.Start` 冻结 Delivery 已协商并 canonicalize 的 Run capabilities，SQLite epoch 69 精确持久化。Goal 的每个自治 Run 复制该集合；`Goal.Resume` 的调用方重新协商能力并必须覆盖冻结集合，否则返回结构化 capability gap。Goal 内 `create_goal` 通过 Runtime adapter-owned execution context 继承当前 Run capabilities。
- 决策：Goal 可在 owned Run 仍 parked 时先恢复 drive。`application/runs.WaitSessionStartable` 必须同时观察 process-local admission 与 durable non-terminal Run，并由 committed Run lifecycle change 唤醒后重读；它不把等待当 reservation，也不以重试延迟掩盖 durable conflict。
- 决策：capability carrier 位于 `adapter/executionctx`，只由 `adapter/agentexec` 在 Runtime/Framework 防腐边界写入、Tool adapter 读取；Domain/Application 值不依赖 context，Toolset 不依赖 Agent Framework，Agent module 不认识 Runtime capability、Goal、Run 或 Store。
- 后果：Goal 的 HITL/child 能力与创建目标时的真实客户端承诺一致，冷恢复和嵌套 Goal 不降级也不扩权；边界仍保持 `adapter/agentexec` 是唯一 Framework concrete import island。

## ADR-RT-058：Goal 完成声明到最终结算是可观察的读模型状态

- 状态：已接受并实施，P37 完成；不改变 Goal Domain 状态机、SQLite shape、Artifact 或 Agent Framework 合同。
- 背景：模型通过 terminal-outcome boundary 将 Goal 原子推进到 Domain `complete` 后，owning Run 仍可能继续提交最终消息、usage 与 Goal 记账；Application driver 只有在该 Run terminal 后才条件清除 Goal。Goal store 的每次成功写都会发布 `goals.changed`，因此客户端在完成写与清除之间回读是合法时序。旧 Protocol 却声称 complete 一定在客户端观察前清除，Delivery 因而拒绝这个有效快照，使一次正确 invalidation 确定性导向 `goals.get` 错误。
- 决策：Domain 保留 `StatusComplete` 作为 objective 已完成、等待 owner settlement 的内部事实；Delivery 在公共 Goal read model 中将它投影为 `status:"completing"`。该词描述消费者此刻能做什么，而不复制 Domain transition 名。`completing` 保持 Goal 占位但不可 stop/resume/start；最终 accounting 与条件清除仍只由 Application drive 拥有，下一次 invalidation 后读取 `null`。
- 决策：生成的 Go/JSON Schema/TypeScript enum、Desktop vendored binding 与 Goal context 自有 read model 原子同步。Desktop banner 本地化显示收尾状态且不暴露 lifecycle command；组件不 import Runtime client，gateway/provider 边界不改变。不得以吞掉读取错误、投影为 active、提前返回 null 或客户端重试延迟掩盖该状态。
- 后果：`goals.changed → goals.get` 在完成窗口内保持闭合，客户端不会提前开放 fresh Goal launcher 或错误地恢复/停止终态目标。Runtime Domain/Application/Delivery、Desktop context 与 Agent Framework 的抽象边界保持不变。

## ADR-RT-059：多个内嵌 Runtime 共享数据库时按业务身份分配所有权

- 状态：已接受并实施，P84 完成；取代 ADR-RT-053 的 data-directory 全生命周期独占结论，并把 ADR-RT-055 的 Goal drive process-local 所有权扩展为跨进程单 owner；不改变 Goal incarnation 的领域语义。
- 背景：产品部署是 CLI 与 Desktop 各自内嵌一个 Runtime，一个 client 只请求一个 Runtime，但二者可能同时打开用户私有目录中的同一 SQLite 数据库并操作相同 cwd。以整个 data directory 为进程级独占单位会迫使本地应用引入无必要的常驻宿主/IPC；仅删除该锁又会让 Session Run、Goal drive、恢复器和破坏性 workspace mutation 在两个进程中各自认为自己是唯一 owner。
- 决策：canonical data directory 强制为 `0700`，只在 store/schema/config seeding 期间持有短期 setup lease，之后允许少量同版本 Runtime instance 共享 SQLite WAL。Application 仍决定冲突语义；`adapter/runtimeownership` 只把 identity 映射到 OS advisory lease。每个 Session 同时只有一个 mutation/Run writer；active Run 同时持有 physical working-tree shared lease，rollback/restore 等破坏性 mutation 取得 exclusive lease；Goal autonomous drive 同一 Session 只有一个进程 owner。业务冲突继续投影既有 `session_busy`，不保留 `embedded.ErrDataDirectoryInUse` 或单宿主 fallback。
- 决策：内核 lease 是本机进程存活的唯一真相，进程退出或强杀后自动释放；不增加 heartbeat、TTL、owner row 或 wall-clock expiry。boot 与存活期 recovery 先竞争全局 sweep lease，单一 winner 固定先结算 Run/Goal accounting、再处理 Goal lifecycle；新 Runtime 在服务 admission 前等待当前 winner并再做一轮复核，存活期则非阻塞跳过。每个策略随后仍竞争目标 Session/Goal 的同一 live-owner lease并重读权威 facts。只有实际接管的 Session 可以进入 checkpoint/child-reservation cleanup，禁止全库 sweep。Schedule firing 和 HITL answer 继续使用既有数据库 claim/事务单赢家，不复制成文件锁。
- 决策：进程内 invalidation 仍发布精确 topic；Persistence 观察 connection-local `PRAGMA data_version`，只在另一个 SQLite connection commit 时向本 Runtime 发布覆盖全部 topic 的 scoped `resync`。消费者收到后重读 durable projection；不建立跨进程事件日志或假装复制细粒度因果事件。
- 后果：CLI 与 Desktop 可各自嵌入 Runtime、共享用户数据库，并在不同 Session 或同 cwd 的非破坏性 Run 上并存；同 Session 写入、Goal drive 和 workspace rollback/restore 仍 fail closed。强杀 owner 后存活 Runtime 无需重启即可接管恢复。该修订没有新增协议 wire 或 SQLite epoch，也没有把 Runtime、RPC、持久化、ownership lease 或 Desktop 抽象泄露进 Agent Framework。

## ADR-RT-060：ToolCall 的已接受人工审批是 Transcript 事实

- 状态：已接受并实施，P106 完成；补充 ADR-RT-056 对 Question accepted answer 的事实所有权，不改变 Agent Framework 的 approval Signal 合同。
- 背景：Tool approval Pending/Interrupt 只负责一次 continuation claim，resume 后会被消费；Desktop 的 `approval-request/result` 也是 renderer 内瞬时 timeline。Runtime 重启、冷启动或 authoritative snapshot replacement 后，completed ToolCall 虽然仍存在，唯一已接受的 approve/deny 却无 durable owner，Run Summary 因而丢失历史；另一客户端代答时，持有 request 的客户端还可能持续显示 pending。当前 approval policy、Tool outcome 和本地 optimistic result 都不能证明一次历史人工决定。
- 决策：exact running `domain/transcript.ToolCall` 以 optional immutable approval decision 持有唯一已接受的 allow/deny；nil 精确表示没有已接受的人类审批事实。只有已验证 Pending + accepted answer 可以形成 resolution；`runsegment` 在 exact answer claim 的同一 SQLite 事务内核对 Session/Run/Item/occurrence/invocation、CAS 替换 Transcript，再与 checkpoint 删除、Pending 消费和 commit receipt 共同提交。任一步失败都整体回滚，重复决定、非法值或 identity mismatch 均 fail closed。
- 决策：resume continuation 把该 resolution 绑定到 exact Item identity 和原始 reviewed prompt。answer claim 以原始 prompt 核对被回答的边界；后续 Tool completion、failure 或 abandonment 只能从带有决定的 running Item 生成 terminal replacement，不得由 reducer 重建覆盖已经提交的决定。edited arguments 是用户实际批准并执行的输入，因此 terminal Tool Item 投影实际执行参数；Item/provider call identity 而非可变参数保证历史连续性。
- 决策：Protocol 一次性提升到 `2026-08-17`，Session Artifact 提升到 v19；Delivery、Artifact、生成合同与 Desktop vendored binding 只投影该 Transcript 事实，不保留旧版本兼容路径。Desktop 以 live request/result 为首选；durable ToolCall decision 只补齐冷恢复或 exact request 被另一客户端结算的决定，并按 exact reference 去重，不能从 policy/outcome 猜测。
- 后果：所有客户端、重放、Runtime restart 与 renderer replacement 最终显示同一个审批决定；事务失败不会留下 decision/Pending/checkpoint 半提交，terminal reducer 也不会抹除已接受决定。Runtime 的 Run、Transcript、SQLite、RPC 与 Desktop 抽象仍不进入 Agent Framework；Framework 继续只接收中性 typed approval Signal，Runtime→Agent concrete import 仍限于 `adapter/agentexec`。

## ADR-RT-061：审批恢复身份使用精确 provider Tool CallID

- 状态：已接受并实施，P107 完成；细化 ADR-RT-060 的 continuation identity，不改变 Transcript 或公共协议事实。
- 背景：approval Tool 在 tree barrier 时刻意不进入 `Continuation.DrainedTools`，其 provider CallID 因而没有 durable owner。旧 resume 只能先按原 name/arguments、再按唯一 name 猜测 Item；同一模型轮次存在同名 drained sibling，且用户编辑获批参数时，两种关联都失效，恢复执行会铸造新 Item，把 accepted decision 与真实执行拆开并留下原 Item running。该反例只需要一个 client 和一次并行 Tool 输出。
- 决策：Application-private `InterruptBinding` 同时绑定 interrupt Item、executor member/request 与 approval provider CallID。tree barrier 从 validated `ApprovalPrompt` 写入；`Pending` 要求审批 CallID canonical、逐 member 唯一且不与 drained/committed Tool 重叠，Question 必须为空。answer claim 在消费 Pending 的同一事实链中把 CallID 带入 `ToolApprovalResolution`，private tree continuation 和 reducer 优先以它复用原 Item；edited arguments 成为实际获批的执行输入和 terminal Tool 投影，但不参与恢复身份判定。
- 决策：SQLite interrupt binding private JSON 增加 exact `toolCallId`，唯一 schema 直接前移至 epoch 75；旧 epoch/缺字段行确定性拒绝，不提供 migration、dual codec 或 name/arguments compatibility branch。Transcript、Protocol、Artifact 与 Desktop 不暴露 provider CallID，Agent Framework 的 typed approval Signal 和 checkpoint 也不承担 Runtime continuation ownership。
- 后果：单客户端并行同名 Tool、参数编辑、Runtime restart 与 SQLite round-trip 后仍只有一个 approved Tool Item lifecycle；损坏 hand-off 在恢复前 fail closed。Runtime 的私有事务/持久化身份不会越过 `adapter/agentexec` 进入 Agent 内层，公共消费者无需同步形状。

## ADR-RT-062：挂载读模型与启动恢复共享 Pending 投影闭包

- 状态：已接受并实施，P108 完成；补强 P83-22 的 transactionally coherent material snapshot 与 ADR-RT-060/061 的 HITL identity，不改变公共协议或持久化 shape。
- 背景：`sessions.snapshot` 已在一个 SQLite read transaction 内读取 Session、Run、Transcript Item、open Pending 与 Plan，但 Application 只校验 Pending 的 root Run；它虽然建立 Item identity 索引，却没有证明 open Interrupt 指向的 Item 存在、属于同一 Run、保持同一 occurrence 和 payload。于是数据库事务可以原子返回一组彼此矛盾的事实，Desktop 会得到没有 Transcript owner 的 HITL，或得到已结算 approval 仍声称 Pending 的部分状态。启动恢复另有更强检查，在线挂载与冷恢复因此可能对同一 durable state 作出不同裁决。
- 决策：`application/runs.Pending.ValidateProjection` 唯一拥有 parked tree 的跨聚合闭包：Continuation 必须完整覆盖同 root 的全部非终态 Run，且 admission facts、累计 metrics、limits、lineage、capabilities 与 Goal incarnation 精确相等；每个 Interrupt 必须解析到同 Session/Run/Item/occurrence，Question 对应 completed unanswered prompt，Approval 对应尚无 accepted decision 的 running ToolCall 且 invocation 相等；所有 running Item 必须被 Interrupt 或 drained Tool 唯一认领，drained/committed hand-off 也必须解析到同一 Transcript fact。Runtime 启动恢复与 `sessions.snapshot` 同时调用这一规则，不再维护两套强弱不同的校验。
- 决策：Material Snapshot 另外拒绝 waiting Run 没有 root-owned Pending、terminal Run 仍持有 running Item，以及不闭合的 Run lineage/spawning Item。校验发生在 Application→Delivery 边界，失败时整体拒绝响应；Desktop 不负责修补、裁剪或猜测。SQLite transaction 仍负责时间一致性，Application 闭包负责领域一致性，两者不能互相替代。
- 后果：挂载、重连、Runtime restart 与真实崩溃恢复对 HITL、Run 与 Tool 使用同一 durable truth；损坏或部分状态在到达 wire 前 fail closed。Protocol、Artifact、SQLite epoch、Desktop Agent inner ring 与 Agent Framework 合同均未变化，也没有增加兼容读取或第二 read model 路径。

## ADR-RT-063：内部边界以唯一行为 owner 和合法构造为准

- 状态：已接受，P113 已完成；取代 ADR-RT-025 中“notification relay 归 Adapter”的物理落点结论，并修订 P14/P17 以后把历史文件位置当作长期架构事实的门禁方式；不改变六环依赖 DAG、公共 Protocol、SQLite 或 Agent Framework 合同。
- 背景：P14–P18 曾按当时调用图消除大量 umbrella 和伪共享 package，但随后继续演进暴露出新的反例：`adapter/toolname` 只有常量却单独成为 package，而 `domain/tool` 已明确拥有 model-facing Tool vocabulary；通用 `adapter/notification.Relay` 没有外部语义翻译或 SDK 防腐，只被 Bootstrap 构造两次；runs/sessions/runsegment 的构造参数与对象字段镜像且允许无效依赖延迟到运行期；operation 用 87 方法 `Service` 让 registry 依赖实现者全集；核心协调器和 interaction session 又把不同并发不变量放进同一字段/锁面。与此同时，架构测试保留大量历史名称和精确文件位置，使安全重构需要同步维护旧实施台账。
- 决策：package 必须拥有独立领域词汇、外部防腐、技术机制或可单独验证的不变量。仅供一个相邻 owner 使用、没有独立变化原因的常量、转发器、配置镜像和一方法接口直接收回该 owner；不通过 alias、forwarder 或新 `common` package 保留旧路径。Runtime 内建 Tool identity 归 `domain/tool` 的唯一 model-facing vocabulary；进程内通知连接是 Bootstrap composition closure，producer 与 Delivery 分别只取得 publish/observe 函数，不再建立 Adapter package 或 implementor-owned interface。
- 决策：构造函数必须建立可运行对象或返回错误。required collaborator 在构造阶段验证，optional capability 必须由显式 nil 语义或独立 constructor 表达；构造参数分组只用于提高调用可读性，不能原样存成运行时 facade。消费端口按调用点定义；operation catalog 绑定 method-specific typed closure，不要求 Server 实现所有 operation 的胖接口。
- 决策：God object 的拆解边界以锁、生命周期和不变量为依据，优先在现有 package 内建立 package-private component；没有独立变化轴时不新建 package。请求 Context 不存入长期对象，不在内部调用点把 nil 静默改成 `context.Background()`；只有进程/instance owner 可以创建 lifecycle root，并必须拥有 cancel 与 join。
- 决策：architecture fitness test 只守长期语义：允许/禁止 import DAG、唯一 vocabulary/codec/state owner、公共 API/持久化 shape、构造合法性和生命周期边界。已删除 package 可以由准确 owner 的 absence guard 防止第二真相源回流，但不得继续以成百历史标识符、局部变量名或“某声明必须位于某文件”充当风格检查；复杂度预算只约束可执行编排行为，声明式 catalog、生成物和本质递归 validator 必须有明确范围。
- 后果：P113 可以进行内部 breaking change并删除无价值边界，同时保持 Domain/Application/Adapter/Infra/Delivery/Bootstrap DAG。每个纵切必须删掉旧 owner、更新当前事实文档并通过目标包、race、全量 Runtime 和静态门禁；不得以拆出更多微包或增加 façade 来降低单文件指标。

## ADR-RT-064：Session 持久拥有精确 provider/model 身份

- 状态：已接受并实施，P143 完成；Protocol 前移到 `2026-08-24`，Session Artifact 前移到 v23，SQLite 前移到 epoch 78，不保留旧 shape reader。
- 背景：Session 过去只保存 `model`，而 provider 只存在于一次 Run 选择或 Runtime 全局默认中。两个 provider 发布同名 model 时，Desktop 只能按 model id 选择目录中的首项；`runs.start` 省略选择时又在读取 Session 前回落全局默认。于是公共语义声称 Session 是下一次 Run 的模型 owner，实际却无法证明 provider，恢复、fork 与 Context window 都可能指向另一模型。该缺陷不是 Delivery 缺字段，而是 Domain owner 不完整。
- 决策：`domain/session.Session` 直接持有既有 immutable `modelref.Selection`，并把 configured exact pair 设为聚合不变量；zero selection 不能构造、恢复、编辑或持久化 Session。Runtime 默认只在 Session Application admission 时安装一次，Runs Application 不再拥有第二份默认选择。省略 Run 选择时读取 Session pair；显式完整 pair 通过 executor staging 后与 Run opening 在同一 write-set 原子替换 Session pair。provider/model 缺一、空值或外围空白一律 fail closed，provider 永不由 model id 推断。
- 决策：fresh create、scheduled admission、fork、restore、Artifact v23、SQLite epoch 78、公共 Session/UpdateSession 与生成 Go/Schema/TypeScript 合同一次性携带 pair。fork 继承父 Session 的 exact pair；归档 import 必须提供 pair；SQLite 列非空且 strict codec 重新建立 Domain 不变量。旧 wire、v22 artifact 与 epoch 77 database 确定性拒绝，不加 alias、双写、双读、fallback 或 migration。
- 决策：Desktop 的 consumer-owned `AgentSessionSummary`、Composer resolution 与 Context gauge 都以 provider+model 比较；同名 model 反例必须命中 exact provider。React render 直接派生该 pair，只有用户明确选择才写 preference，不通过 effect 复制第二 selection store。`app/cli` 本批不修改，其迁移缺口只记录在 consumer handoff，不能倒逼 Runtime 恢复 model-only surface。
- 后果：Session、Run staging、持久化、归档和 Desktop presentation 对“下一次 Run 使用哪个模型”只有一个可恢复答案。采用 app2 的 exact identity 与纵切经验，但拒绝 opaque JSON、额外 public package、god facade、能力删减和低覆盖；原 Runtime 的严格 Protocol、完整恢复矩阵、公共 `protocol`/`embedded` 边界与资源生命周期保持不变。

## ADR-RT-065：Session Workspace 是精确领域值，filesystem admission 是外部事实

- 状态：已接受并实施，P144 完成；SQLite 前移到 epoch 79，Protocol `2026-08-24` 与 Artifact v23 shape 不变。
- 背景：Session 私有状态、Draft/Patch/Snapshot、SQLite 与 Application/Desktop read model 都把同一 workspace 退化为裸 `cwd` 字符串或多份平行字段。Domain 只检查非空和外围空白，因此绕开 Application 即可构造相对或词法不规范路径；SQLite 的空默认值又与 Session 必填不变量冲突。这不是补一处校验能解决的问题，而是 workspace identity 没有唯一技术表示和投影边界。
- 决策：`domain/session.Workspace` 是 immutable exact value，拥有不接触 filesystem 的纯不变量：路径必填、绝对且等于其 lexical clean 形式。Session 的 Draft、Patch、Snapshot、restore、fork 与 relocation installation 只接收该值。Application filesystem port 先验证目录存在并解析物理 canonical identity，再构造 Domain Workspace；Domain 不解析 symlink、不查询存在性，也不复制 app2 的 filesystem-root 禁令，因而保留原 Runtime admission 语义。
- 决策：Application Session read model 将 path、project root 与 availability 收进一个 consumer-owned Workspace 投影；Desktop adapter 再投影唯一 workspace 对象，React 直接派生 path/missing，不维护镜像状态。SQLite 只保存非空 `workspace_path`，shape change 直接提升 epoch，旧 `cwd` 列不双读、不迁移。Protocol/Artifact 已经使用 `WorkspaceRef`，机器 shape 不变时不虚增版本。
- 后果：任何进入 Session、SQLite 或 Desktop projection 的 workspace 都可追溯到同一已准入值；执行用例中表示 sandbox/命令工作目录的 `CWD` 仍是不同技术事实，不做全仓术语替换。`app/cli` 留待独立 handoff，不能要求 Runtime 保留旧内部 shape。

## ADR-RT-066：operation catalog 拥有带外调用元数据的适用性

- 状态：已接受并实施，P146 完成。
- 背景：HTTP adapter 可把 `Idempotency-Key` 带到 query，operation 却静默忽略并正常执行；embedded query 的 `CallOptions` 根本不能表达该 key。同样，通用 HTTP header bag 可把 namespace 或 `Last-Event-Id` 投给不会消费它们的方法。binding 因而既不等价，也可能让客户端误以为一次调用拥有实际不存在的 replay 保证。
- 决策：Contract Registry 在 operation/idempotency 之外发布 `ReplayCursorPolicy`；run-opening command 与 `runs.subscribe` 由 registration factory 获得 run cursor 能力，`runtime.subscribe` 等其他 stream 明确为 none。operation Endpoint 在 capability 与 handler admission 前统一拒绝非 replay 方法的 idempotency key、无 key 的 namespace、无 cursor 能力的方法携带 `AfterEventID`，以及没有 idempotency key 的 run-command cursor。
- 决策：manifest、OpenRPC 与生成 TypeScript method policy 只投影 Registry 的同一事实；Desktop 低层 RPC client 在 transport send 前消费该生成策略。embedded surface guard 同样按 policy 选择 option 类型，不再按 `runs.subscribe` 方法名维护第二列表。HTTP/embedded 只负责把本地表示投影成 operation options，不自行决定承诺是否成立。
- 后果：此前被静默忽略的带外元数据现在 breaking 地返回 `invalid_params` / 本地 `TypeError`；合法 command replay、run reattach、Runtime invalidation subscription 的业务、stream lifecycle 与 wire DTO 不变。没有兼容 fallback、双策略、第二 writer 或 transport-specific admission。

## ADR-RT-067：资源关闭 settlement 与 diagnostic 必须正交

- 状态：已接受并实施，P148 完成；细化 ADR-RT-063 的合法构造与唯一 lifecycle owner，不改变公共 Protocol、Artifact、SQLite shape 或 Agent Framework 合同。
- 背景：Host 过去把任意 `Shutdown` error 都解释为“资源仍未关闭”，保留该 step 并阻止依赖关闭。但生产 A2A、LSP、Shell 与 SQLite closer 都由 `sync.Once` 拥有终态：第一次调用即使报告诊断，后续只会永久返回同一缓存错误。于是一个已经完成的 close error 会让 Host 永久停在 stopping，Store 等下层依赖也永远得不到释放；旧 `Bundle.Shutdown(ctx)` 还把“预算已过期、根本没有调用 Close”伪装成同一种 error。
- 决策：P148 先让 Infra teardown step 显式区分 `Terminal` 与 `Retryable`，并把 settlement 与 diagnostic 正交；Terminal action 一旦返回便冻结 settlement，Host 记录诊断但继续逆序关闭依赖。Config 只接收具有 one-shot `Close` 语义的 `TerminalResource`，SQLite 删除没有独立消费者且会混淆状态的 context-aware Shutdown adapter。P149 随后证明唯一 Retryable 生产消费者 MCP 也属于 terminal；P150 在 ADR-RT-069 中删除因此失去消费者的 Retryable/settlement 双态，并把 caller wait 与整张 terminal resource graph 的执行 generation 分开。
- 后果：一次性 close error 只执行一次、只向实际观察它的 Close 调用报告，不能形成永久错误回放或资源保活；超时、未完成的 Close 与非协作第三方 closer 仍有界并由同一在途 generation 持有。关闭顺序、并发 Host copy 幂等性、失败 Assembly 回滚和公共 `embedded.Runtime.Close` 入口不变，没有兼容层、第二清理图或后台重试循环。

## ADR-RT-068：MCP ClientSession 关闭错误是 terminal diagnostic

- 状态：已接受并实施，P149 完成；纠正 P148 对唯一生产 `Retryable` step 的暂定分类；P150 进一步删除该失去消费者的原语。不改变公共 Protocol、Artifact、SQLite shape 或 Agent Framework 合同。
- 背景：Runtime 的 MCP ledger 曾在 `ClientSession.Close` 返回错误后继续保留 session，并让下一次 `Shutdown` 建立新一代 close attempt。实际 SDK 会在 connection 进入 closing 后等待任务、调用底层 transport closer，并无论该 closer 成败都将其置空；后续 `ClientSession.Close` 只能返回同一个缓存诊断，不能再执行底层释放。Runtime 因而把已经终结的 session 误作活资源永久保留，也让 Bootstrap 的 terminal 依赖关闭被一个无法推进的假 retry 阻塞。
- 决策：每个 MCP session close attempt 返回后都从 ownership ledger 删除；异步 Detach 的 attempt/diagnostic 由独立 retirement ledger 保存到 `Shutdown` 汇总，不继续持有 session。加入当代 `Shutdown` generation 的 caller 收到聚合诊断，已经完成的 `Shutdown` 再调用则为幂等 no-op。并发 caller 继续 join 同一个在途 attempt，caller timeout 也不创建并行 Close。Bootstrap 将 MCP pool 注册为 terminal step，并以 `context.WithoutCancel` 让内部 action 跑到真实终态；有界等待由外层 teardown owner 拥有，超时不重放 SDK action。
- 后果：MCP close diagnostic 不再形成永久资源保活或错误回放，下层 SQLite/host resource 可按逆序继续释放；仍在途或不协作的 SDK Close 由同一 terminal generation 持有，Host deadline 只约束 caller 等待。P150 再把该 generation 提升为整张资源图的 Sequence；不新增重试循环、兼容分支或第二清理图。

## ADR-RT-069：terminal resource graph 必须独立于 caller timeout 自行完成

- 状态：已接受并实施，P150 完成；细化 ADR-RT-063/067/068 的构造失败所有权，不改变公共 Protocol、Artifact、SQLite shape、Desktop 或 Agent Framework 合同。
- 背景：Assembly 构造失败时会先做一次有界 rollback，`OpenInstance` defer 再做一次；两次都可能在同一个 non-cooperative terminal closer 上超时。旧 Step 虽继续拥有该单个 action，但 action 稍后返回时，没有 owner 继续遍历其下方的 MCP/A2A/LSP/SQLite 等依赖。失败 Open 又不会返回 Host，调用方无法发起第三次 Close，结果是“closer 有 owner、逆序图无 owner”的永久泄漏。P149 同时证明生产树已没有真正 Retryable resource，保留通用多 generation 状态只服务测试。
- 决策：Infra 只保留 one-shot terminal Step；删除 Retryable constructor、settlement 双状态、失败前缀切片和对应 test-only surface。Bootstrap 将先取得的 host resources 与后取得的 tool resources 按 creation order 合并进唯一 `teardown.Sequence`。Sequence 第一次 Shutdown 启动一个 `context.WithoutCancel` 的 reverse-order generation并聚合全部 terminal diagnostics；caller context 只限制等待。timeout 返回后 Sequence 继续执行，后续 Close 只能 join 同一 immutable result，不能启动第二代或重放 Close。
- 后果：失败 Assembly/Open 即使不再有外部 cleanup handle，已启动的资源图仍会在迟到 closer 终结后继续释放所有依赖；正常 Host Close 的 deadline、并发 copy 串行化、reverse order 与诊断聚合不变。真正需要多 generation 的 Interaction/MCP 等 subsystem 继续在自己的 owner/ledger 内表达，不借通用 resource wrapper 猜状态；没有后台重试循环、兼容路径或第二清理图。

## ADR-RT-070：Host shutdown generation 必须独立于 caller wait

- 状态：已接受并实施，P151 完成；将 ADR-RT-069 的 terminal resource 所有权上移到完整 Host 关闭图，不改变公共 Protocol、Artifact、SQLite shape、Desktop 或 Agent Framework 合同。
- 背景：P150 让已进入 terminal Sequence 的 closer 在 caller timeout 后继续遍历依赖，但 Host 在此前仍以同一 caller-owned deadline 同步加入 Goal/MCP/codebase/Run task groups 与 Interaction executor。`BuildAssembly` 成功转交 Host 后，`OpenInstance` 的 startup recovery 仍可失败；它的 defer 只能调用一次 Host Close，而失败 Open 不返回 Host。若 component 在该等待预算后才结束，旧 Host 虽未提前关闭它的依赖，却也没有 owner 再推进 executor 与 resource Sequence；结果是 component 已停、下层 Store 仍永久保留。
- 决策：`hostLifetime` 持有唯一 active shutdown attempt。第一个 Close 建立 package-private generation，广播所有 component cancellation，并用注入 Runtime lifetime 的 values + `context.WithoutCancel` 依次加入 component、run-effect tasks、executor 与 terminal Sequence。每个 caller 另建有界 wait context；超时只停止该 caller 等待，不取消、替换或并行复制 owner generation。已完成且返回 unsettled component/executor error 的 attempt 仍允许下一次显式 Close 开新 generation；terminal resource diagnostic 会关闭 Host，不重放。
- 后果：Host/Instance Close 仍有界，并发 Host copy 只 join 同一 attempt；post-transfer 失败 Open 即使不返回 cleanup handle，迟到完成的 component 也会自然触发 executor 与资源逆序释放。没有 timer、backoff、fire-and-forget retry loop、第二清理图或 caller context 伪 lifetime；Bootstrap 仍只拥有进程组合/生命周期行为，不增加业务 facade。

## ADR-RT-071：Instance shutdown generation 连续拥有 delivery、workers 与 Host

- 状态：已接受并实施，P152 完成；将 ADR-RT-070 的 Host owner 上移到完整 Runtime Instance，不改变公共 `embedded.Runtime.Close` 签名、Protocol、Artifact、SQLite shape、Desktop 或 Agent Framework 合同。
- 背景：P151 保证 Host component 在 caller timeout 后迟到结束仍会进入 executor/resource teardown，但 `Instance.Close` 仍在单个 caller-owned context 内同步加入 operation Endpoint、scheduler、database observer 与 recovery worker；其中任一项超时都不会调用 Host。CLI 以一个 defer 调用 `Instance.Close`，公共 embedded Close 也只转发一次，不存在外部 retry 承诺。真实反例证明已接受 operation 在 caller deadline 后返回时，旧 Instance 既没有提前关闭 Host resource，也没有 owner 在 operation 结束后继续关闭它。
- 决策：Instance 持有唯一 active shutdown attempt。第一个 Close 建立一个从 Host 已注入 Runtime lifetime 派生、不继承 caller cancellation 的 generation；它只广播一次 delivery admission close 与 Runtime cancellation，然后依次加入 Endpoint、scheduler/database/recovery workers，最后通过 package-private Host attempt 入口加入 P151/P150 关闭图。每个 Instance Close caller 单独使用有界 wait context；并发 caller 只 join 同一 attempt。已完成且返回明确 phase error 时，后续显式 Close 可开新 attempt。
- 后果：已接受 operation 仍在依赖关闭前完整退出；即使唯一 public/CLI Close caller 先返回 timeout，operation 迟到结束也会自然推进 workers 和 Host resource 释放。Instance 不复制 Runtime context 字段，而是消费 Host 唯一 lifetime owner；没有 timer/backoff/retry loop、第二 Host graph、兼容路径或新公共 surface。

## ADR-RT-072：删除独立 Codebase 向量索引纵切

- 状态：已接受并实施，P153 完成；允许 Protocol、公共 Go API、SQLite 与直接消费者 breaking change，不提供兼容期。
- 背景：Agent `codebase_search` 已在早期批次删除，真实代码发现由 grep/glob/read/shell/LSP 承担；余下 Codebase 只有 Desktop Context Dock 与 CLI 手动状态、搜索、重建入口。其实现是固定行块、cosine-only 的单路 dense-vector 检索，没有 lexical fusion、symbol/graph、query rewrite 或 rerank，却持续拥有三项 operation、一个 feature/topic、三张表、后台 lifecycle、生成合同和两端 UI/命令。Agent Memory 则仍真实使用 embedding，并有关键词 fallback，二者不能机械捆绑删除。
- 决策：Runtime、Desktop 与经用户授权的 CLI direct consumer 一次性删除 Codebase Domain/Application/Adapter/Infra/Delivery/Protocol、feature/topic/schema、RPC/gateway/query/view/command/port/test/locales/docs；SQLite 直接提升 epoch 80，生成合同只保留当前 shape。Embedding resolver 与 vector codec 收归 Agent Memory owner。Git/workspace source discovery 同批把 cancellation、unborn repository 与非仓 fallback 明确分型；exit 128 不能一律伪装成非仓。
- 后果：不存在 disabled registration、旧 `@codebase` mention、compat facade、双 schema、空 package 或隐藏命令。客户端使用现有 workspace grep/file 能力，Agent 继续使用可组合代码工具；Memory semantic ranking 保留且关闭 embedding 时仍可用。Protocol 精确版本仍为 `2026-08-24`，因为仓库只接受同批生成的唯一当前版本，不承诺同日旧 shape。

## ADR-RT-073：Agent Memory 向量缓存绑定 exact space 与 content digest

- 状态：已接受并实施，P154 完成；SQLite 前移到 epoch 81，公共 Protocol、Artifact、Desktop 与 Agent Framework 合同不变。
- 背景：P153 将仍有真实消费者的 embedding/vector codec 收归 Agent Memory，但旧 `agent_memory_items.embedding` 只保存裸 BLOB。`models.setEmbeddingRole` 会立即切换 query embedder，历史 corpus vector 却没有 producer identity；两个模型维度相同时，旧 cosine 不会 fail closed，而会在新坐标系中静默错排。旧 Run-boundary Curation backfill 还只按 item id 写入；若用户在 embedding I/O 期间编辑内容，迟到结果会越过 `UpdateContent` 的清空，把旧内容 vector 重新附到新内容。
- 决策：embedding 是 `application/agentmemory.Searcher` 唯一拥有的可丢弃派生缓存，不再属于 Curation 或 Run maintenance。每个 search cache 同时保存 non-empty embedding space 与 finite vector；space 是 provider/model/custom endpoint 等非秘密 client input 的稳定指纹，不包含 API key。Searcher 先取得当前 embedder/query vector，只复用 space 相等且维度相同的 corpus vector，其余内容在该次搜索中批量重算并立即参与排名。resolve/embed/cache 任一失败不污染语义，仍以 keyword signal 返回。
- 决策：cache write 携带 item id、由 exact content 生成的 digest、space 与 defensive-copy vector；SQLite 只在目标仍为 active 且 digest 未变时写入。schema 增加非空成对约束的 `embedding_space`，reader 严格拒绝空/半对、非 4-byte 编码和非有限数值；内容编辑同时清空 space/vector。SQLite 直接提升 epoch 81，不读取 epoch 80 裸向量，也不在 role mutation、后台 worker 或 Desktop 建立第二 invalidation/rebuild owner。
- 后果：同维度 role 切换、维度变化、首次配置、内容编辑与并发搜索都在下一次真实 search 收敛到当前空间；迟到 cache 只能失去条件写，但本次请求仍使用自己已经取得的 exact vector。未配置或不健康的 embedder 继续 keyword-only；公共 embedding role、Agent Memory 用户能力和 Runtime 生命周期不变。

## ADR-RT-074：Agent Memory recall 联合 project 与 user scope

- 状态：已接受并实施，P155 完成；公共 Protocol、Artifact、SQLite shape、Desktop 与 Agent Framework 合同不变。
- 背景：Agent Memory 管理面允许创建 active user-scope item，系统 prompt 也只会始终注入 pinned 的 project/user items；但 per-turn recall 和 `search_memory` 都把 Searcher 固定调用为 project scope。结果是未 pinned 的用户偏好在 SQLite 与 Desktop 中可见，却没有任何 Agent 消费路径；工具 definition 仍声称会搜索 user preferences，形成公开行为与真实 corpus 的冲突。
- 决策：Searcher 的用例语义收敛为“当前项目上下文可见的联合 corpus”，删除调用方可选 scope 参数。SQLite 用一次 query 读取 exact project 的 active items 与全局 user-scope active items；Searcher 对该快照只生成一次 query embedding、执行一次 keyword/vector fusion 与一个全局 top-k，并在同一缓存 owner 下刷新两类 item。prompt recall 与 `search_memory` 只提交 project identity/query/limit，不分别查询或合并 scope。
- 后果：未 pinned 的 user memory 与 project memory 现在公平竞争同一 recall budget，相关用户偏好能进入 per-turn system reminder 和显式工具结果；pinned items 仍由 always-on prompt owner 注入并在 per-turn block 中过滤。显式管理 API 的 scope、SQLite table/epoch、embedding cache 条件写、Desktop UI 与 keyword fallback 均不改变；没有第二 query、双 top-k、客户端 merge 或兼容 facade。

## ADR-RT-075：Agent Memory 内容边界由 Domain 与 whole-item prompt budget 共同闭合

- 状态：已接受并实施，P156 完成；Protocol 当前 shape、生成消费者与 SQLite 直接 breaking 前移，Artifact 和 CLI 不变。
- 背景：Agent Memory 的 user add、curation proposal、编辑和 strict read 只校验非空，单条内容没有上限。`newPinnedMemoryPrompt` 又只在已经写入首项后检查 4096-token budget，因此任意大的首条 pinned item 会完整进入 system prompt；未 pinned item 也可经 per-turn recall 整条注入。管理面、SQLite 与 wire 接受的 durable value 因而能让模型请求超出上下文，而在三个消费者分别截断会制造不同内容身份和第二套规则。
- 决策：Domain 以 `MaxContentCharacters = 4096` 唯一规定 item 与 ledger fact 的 canonical valid-UTF-8 内容上限，单位是 JSON Schema/Go/TypeScript 都能精确表达的 Unicode code point。构造、fact normalization、内容编辑、embedding identity、SQLite strict read 与 fresh schema 均投影该不变量；SQLite 直接提升 epoch 82。Contract registry 对 `agentMemory.add`、可选 update content 与 `AgentMemoryItem` 输出生成相同 `maxLength(4096)`，Desktop 的生成 request/result validator 消费它，不另写 UTF-16/字节长度常量。
- 决策：Agent adapter 对 pinned core 和 per-turn recall 分别执行保留整条 item 的 4096-token aggregate budget；首项也必须检查，超预算即停止较低优先级 material。Domain 上限按现有保守估算保证任一合法单项自身可装入该预算；显式 `search_memory` 仍经过统一 Tool-result offload 生命周期，不建立 memory 专用截断/分页协议。
- 后果：任意单条持久 memory 不再能放大为无界 prompt/request/result，corrupt 或绕过构造的首项也不能穿透 adapter budget。旧 epoch 81 database 与同日旧 generated shape 不读取；没有 silent truncation、dual validator、consumer-owned content cap、compat shim 或 CLI 改动。

## ADR-RT-076：Auxiliary model request 必须拥有显式资源包络

- 状态：已接受并实施，P157 完成；只改变 Runtime internal adapter/maintenance，公共 Protocol、Artifact、SQLite、Desktop、Agent Framework 与 CLI 合同不变。
- 背景：title、compaction、Agent Memory consolidation 与 Skill mining 共用 `utilitymodel.Complete`，但旧 helper 没有设置 `chat.Options.MaxTokens`；Skill/Memory 还选择 uncapped tool results，compaction 虽限制单条结果却没有总 transcript 上限。失败反例用 128 条 8KiB tool result 得到约 1.05MiB 的普通维护输入与约 518KiB 的 compaction 输入，并证明 provider request 的 output token limit 为 nil。把更多 cap 分散到每个 worker 会继续保留可绕过的公共 helper 和第二套资源规则。
- 决策：`adapter/utilitymodel.Prompt` 是唯一辅助请求包络，强制正数 `MaxInputBytes` / `MaxOutputTokens`、在 provider I/O 前校验 aggregate bytes，并唯一投影 `chat.Options.MaxTokens`。Session title 使用 20KiB/64-token envelope；Run maintenance 使用 512KiB hard input envelope，conversation transcript 在其中最多 384KiB、每条 message 最多 24KiB，按 message 公平保留 rune-safe head/tail；compaction 与 Skill 默认最多 4096 output tokens，Memory 默认最多 2048。删除 uncapped renderer 与 per-tool-result-only policy；compaction trigger 改为无 materialization 地测量原始 transcript bytes。
- 决策：Memory curation 为 current auto memory 与 pending ledger 分配独立 whole-entry byte budget。Ledger 只取从 watermark 后开始、能够完整进入当次 prompt 的 sequence 前缀，事务只推进到该前缀最后一项；未进入 prompt 的事实留给后续 fold，不能因输入裁剪而被跳过。
- 后果：所有内部辅助调用在发送前都有可证明的输入/输出上限，大工具输出不再线性放大 provider request，压缩触发仍观察真实历史体积。采纳 app2 的显式 `MaxTokens`、384KiB aggregate transcript 与 fair-share 思路；保留原 Runtime 的 utility-role resolver、维护纵切与纯文本 proposal contract，不复制 app2 `runtimehost` facade、公共协议或第二模型 owner。

## ADR-RT-077：Agent Memory complete-list 与负历史必须共同有界

- 状态：已接受并实施，P158 完成；只改变 Runtime internal Domain/Application/SQLite/Delivery 行为，公共 Protocol shape、Artifact、SQLite schema、Desktop、Agent Framework 与 CLI 合同不变。
- 背景：`agentMemory.list`、prompt Items 与联合 search corpus 都完整读取 target，但旧 Add/curation 没有 target capacity，33 条以上的单次模型结果也可进入一个事务；被拒绝的 proposal 永久保留。失败反例证明 project target 可写入并返回 513 个 visible item、corrupt overfull target 不报错、显式 user add 只取回 rejected auto tombstone 而没有恢复用户事实，连续拒绝会留下 2049 个 tombstone。单独给查询加 `LIMIT` 会静默丢失 complete-list material，不能修复集合所有权。
- 决策：Domain 唯一定义每 target 512 visible、2048 rejected、每 extraction/curation 32 条与每 ledger fold page 128 条。SQLite Add 在一个事务内先解析 digest/lifecycle 再计数插入；同 digest active 幂等，pending/rejected 经显式 user add 原地提升并保持 id。Curation 先删除过期 pending，再按模型声明的重要性顺序发布剩余容量可容纳的 proposal 前缀；complete List/Items/SearchCorpus 读取上限加一并拒绝 corrupt overfull target。Review reject 在同一事务中保留最新 tombstone 并删除目标内更旧的超额负历史；Delivery 将正常 target-full 映射为 `invalid_params`。
- 后果：所有 public、prompt 与 search complete-list material 都有由写侧证明的有限上界，不靠 consumer 静默截断；自动维护在满 target 上仍能推进 durable watermark，不会永久重试同一 fold。旧 rejected 抑制记录可按确定 retention 淘汰，显式用户意图可以覆盖拒绝历史。该批没有改变列、表或 CHECK，SQLite epoch 保持 82；没有分页兼容面、第二容量常量、迁移链、后台 GC 或 CLI 修改。

## ADR-RT-078：Skill Proposal queue 以名称保留一个有界 current revision

- 状态：已接受并实施，P159 完成；只改变 Runtime internal Skill Domain/Tool adapter/filesystem storage 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：旧 proposal 以完整文档 digest 作为目录名；同一 Skill 每次改稿都会新增一个永久并列的待审目录。`skills.proposals.list` 又一次返回 project/user 两个 scope 的全部正文，没有文档大小或队列基数上限。失败优先反例 `cf2cbc99e` 证明同名两版同时可审、超过 1 MiB 的文档可写入、同一 scope 第 129 个不同名称仍可进入完整列表。单纯给响应截断会让被隐藏 proposal 继续占用 durable review state，也会破坏 exact review 语义。
- 决策：proposal retention identity 改为 `(scope, name)`，存储槽位一次性切换为 `_proposals/<name>/SKILL.md`；同名提交用原子文件替换保留一个 current revision。`ProposalRef` 继续以 scope/name/完整文档 SHA-256 绑定审阅字节，旧句柄读取当前槽位时返回 `ErrProposalChanged`。同一 library 的既有 directory advisory lease 统一包围 capacity check、replacement、active/archive mutation、usage maintenance 与 exact review decision；两个 Runtime 共享目录时只有一个 durable winner，不另建 proposal lock/claim 状态机。
- 决策：Domain 唯一定义完整 authored `SKILL.md` 1 MiB 与每 scope 128 个待审名称。Proposal raw validation、rendered document、active/archive/proposal lifecycle reader 与 Agent Tool schema 共同守住文档边界；读取先检查 stat，再用 limit+1 读取。队列枚举只取 capacity+1，正常写入在创建新名称前校验容量，corrupt overfull state 整体拒绝，不把静默截断伪装成完整页面。
- 后果：连续改稿不再制造无消费者的 pending history，公共 review 页面最多返回 project/user 各 128 份且单份大小有限；stale decision 与 concurrent replacement 仍由 exact content CAS 确定性隔离。该批采纳 app2 的“每 workspace/name 一个 pending row”和 1 MiB authored document envelope，但保留原版 filesystem confinement、精确 review ref、原子发布/归档与既有跨进程 lease owner；不复制 app2 SQLite proposal owner、胖 facade 或迁移链。

## ADR-RT-079：LSP document synchronization 拥有固定输入包络

- 状态：已接受并实施，P160 完成；只改变 Runtime internal LSP filesystem adapter 与测试文档，公共 Protocol、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：原版 `ensureOpen` 对 workspace document 直接调用 `os.ReadFile`，文件大小没有上限；读取完成后还会复制到 SHA-256 输入、Go string 与 JSON-RPC `didOpen/didChange` payload。失败优先反例 `c6ff8f3a0` 证明超过 8 MiB 的生成文件仍被完整接纳，甚至在 digest 与已有 open-state 相同、不需要通知时也先付出无界读取成本。请求取消只能在整份文件读完后由后续 RPC 看见。
- 决策：采纳 app2 codeintel 的 8 MiB 单文档 envelope，并把它放在原版唯一 `ensureOpen` 读取边界。reader 在打开前检查 cancellation，以 stat 快拒绝已超限文件，再通过 cancellation-aware `limit+1` 读取覆盖并发增长；超限以稳定 `ErrDocumentTooLarge` 返回，既不计算 digest，也不修改 client state 或通知语言服务器。
- 后果：LSP 输入内存与 JSON-RPC payload 取得确定上界，exact-boundary 文档仍可使用；取消在分块读取过程中保持可见。该限制只属于语言服务器同步消费者，不改变 workspace file read 的 caller-defined window，不引入截断、partial document、兼容 fallback、配置旋钮或第二同步路径。

## ADR-RT-080：MCP 远端工具目录在 Domain 统一限制 material

- 状态：已接受并实施，P161 完成；只改变 Runtime internal MCP Domain/Infra 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：原版只验证 MCP input schema 是 object，并在 live commit 时拒绝 public tool-name collision；远端 `tools/list` 的数量、description bytes 与 schema bytes 都没有上限。失败优先反例 `ee80f6901` 证明模型目录接受超过 64 KiB 的 description，`mcp.tools.list` 接受超过 1 MiB 的 schema，commit gate 接受同 server 2049 个工具。远端 material 会被保留到每次模型请求并完整投影到管理面，不能依赖 provider context 或 HTTP body limit 间接兜底。
- 决策：采纳 app2 已验证的 per-server 2048 tools、per-description 64 KiB 与 per-schema 1 MiB envelope，并由 MCP Domain 唯一发布常量和稳定错误。description 同时要求有效 UTF-8；input schema 在 JSON decode 前先验 encoded bytes，并在 canonical normalization 后重验 owned representation，防转义扩张越界。模型目录在 session verification 后、publication 前验证完整 tool set；管理目录逐 descriptor 验证并拒绝 nil/empty/duplicate name。commit gate 再守一次廉价 count invariant，并继续原子检查跨 server public-name collision。
- 后果：一个远端 server 不能把无界 descriptor material 带入 live projection、模型工具目录或非分页管理页面；任何越界使该 server 的完整 catalog 失败，不截断、不发布前缀、不降级为空 schema。该批保留原版 generation-safe reconnect、session ownership、tool policy 与 public-name identity，不复制 app2 Runtime/Service facade、SQLite owner 或兼容路径。

## ADR-RT-081：Knowledge Domain 拥有 `LYRA.md` 完整文档包络

- 状态：已接受并实施，P162 完成；只改变 Runtime internal Knowledge Domain/Application/Infra/Delivery 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：`LYRA.md` 同时进入非分页管理面和 fresh Run prompt，但原版 Domain/Application 没有 content bound，filesystem store 还直接 `io.ReadAll` 外部文件。失败优先反例 `756a86456` 证明 Application 会把超过 1 MiB 的更新交给 persistence port，Store 会原子写入同一内容，也会把外部放置的超大文档完整读入 revision/string/prompt 路径；补充反例 `2cfc5f58a` 证明 prompt composer 会丢弃整个 Knowledge cascade 的读取错误，让管理面失败与模型侧静默缺指令形成双重语义。HTTP request body 的 transport cap 既不保护 embedded/direct store consumer，也不表达单文档所有权。
- 决策：采纳 app2 的 1 MiB authored-knowledge threshold，但把常量与稳定 `ErrDocumentTooLarge` 放在 Knowledge Domain。Application 在 store 之前验证完整 content；filesystem Store 的 direct Update 复验，read 则先检查 caller cancellation 与 stat，再以 cancellation-aware `limit+1` reader 覆盖读取期间增长。Delivery 只把命令侧越界映射为 `invalid_params`，外部 corrupt file 保持读取失败；管理面与 prompt composer 都对完整 cascade fail closed。Agent Memory、AGENTS.md 与 Plan 的既有 best-effort 策略不随本批改变。
- 后果：Knowledge 管理页、revision 计算与模型 prompt 不再接纳无界 `LYRA.md`；exact 1 MiB 文档仍可读写。原有 content CAS、跨进程 directory lease、in-scope symlink identity、atomic rename、权限与 crash recovery 全部保留；不增加 truncation、skip、transport-only guard、配置旋钮、兼容 reader 或第二存储路径。

## ADR-RT-082：Lifecycle Hook 配置是有界完整策略级联

- 状态：已接受并实施，P163 完成；只改变 Runtime internal Hook Domain/filesystem adapter 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：`hooks.json` 同时进入 `hooks.list` 非分页管理面与每次 fresh Run 的 trusted policy binding，但原版 loader 直接 `os.ReadFile`，单文件 bytes、hook 数量和完整 global + project cascade 都没有上限；Domain 也允许任意长 matcher、command/inject 与 timeout。失败优先反例 `df00e958f` 证明超过 256 KiB 的文件、单文件 129 条、完整级联 257 条、非法 UTF-8，以及超过 256-byte matcher、8-KiB action、5-minute timeout 均会被接受。HTTP request cap、客户端页面和命令执行 timeout 都不拥有本地策略 material。
- 决策：采纳 app2 的 256 KiB/128-per-file/256-per-cascade、256-byte matcher、8-KiB action 与 5-minute timeout 阈值，但由原版 Hook Domain 唯一发布常量与稳定 `ErrConfigurationTooLarge`/`ErrInvalidHook`。filesystem loader 改为 open + stat fast rejection + cancellation-aware `limit+1`，同时覆盖外部超大文件、读取期间增长与 caller cancellation；完整 bytes 必须是有效 UTF-8，JSON decode 后在 Hook materialization 前验证单文件数量，追加前验证完整级联数量。
- 后果：管理面与 Run execution policy 只观察同一份有限且完整的级联；任何越界整体失败，不截断、不跳过、不发布 partial active policy。exact boundary 配置继续可用，既有 global-before-project 顺序、project trust、event matching、first-deny/first-rewrite 和 watcher invalidation 保持不变；不复制 app2 hookflow facade、第二 resolver、协议兼容面或配置旋钮。Hook command invocation/output 的进程级资源包络属于后续独立所有权，不在本批用配置 cap 冒充。

## ADR-RT-083：Lifecycle Hook command 拥有有界输出与完整进程树

- 状态：已接受并实施，P164 完成；只改变 Runtime internal Hook process adapter 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：原版 Hook shell adapter 把 stdout/stderr 接到无界 `bytes.Buffer`，只靠 command timeout；子进程继承管道时，`exec.CommandContext` 只杀顶层 shell，调用可在 deadline 后继续等待后代退出。stdout JSON 又通过忽略错误的 `json.Unmarshal` 和 unknown-verdict-to-allow 映射，超大、畸形、未知或多值输出都会静默成为 allow/partial decision。失败优先反例 `295eacc5d` 证明 70 KiB stdout 会完整进入 injected context、70 KiB stderr 会完整驻留、三类非法 decision 均被接受，40 ms timeout 的 `sleep 5 & wait` 仍阻塞约 5 秒；补充反例 `7574fcacc` 进一步证明 JSON `null` 会成功解入结构体并被误当作空 allow，而不是所要求的 decision object。
- 决策：采纳 app2 hookprocess 的 64 KiB per-stream bounded-drain、strict decision decode、Unix process group 与 2-second `WaitDelay`，适配到原版单一 `adapter/hooks.Shell` owner。writer 对 child 永远报告完整消费，避免 cap 反向堵塞进程，只保存固定前缀并标记 stdout overflow；stdout 为空或严格单一 UTF-8 object，unknown field/verdict、trailing value、decode failure 与 overflow 都进入现有 `CommandResult.Err`，由 Application `onError` 观察且不贡献 decision。exit code 2 继续优先形成 deny，并只使用有界 decision/stderr reason。Unix cancellation/timeout 与 command return 后都对独立 process group 执行幂等 kill；Windows/other 保留平台可拥有的 process kill。
- 后果：不受信任的 Hook command 无法再用输出或遗留后代突破 Runtime 的内存/等待包络，broken output 也不再静默改变执行策略。empty/valid allow-deny-ask JSON、first-deny/first-rewrite、timeout 默认值和非 2 错误 non-blocking 语义保持；private stdout contract 直接 breaking 严格化，没有 legacy parser、truncation-to-valid-JSON、unknown-to-allow fallback、第二 executor 或配置旋钮。Hook invocation/stdin material 当时保留为独立边界，后由 ADR-RT-084 闭合。

## ADR-RT-084：Lifecycle Hook command input 由 Domain projection 与 Shell envelope 共同拥有

- 状态：已接受并实施，P165 完成；只改变 Runtime internal Hook Domain/Application/process JSON 与测试文档，公共 Protocol、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：P164 封闭输出与进程树后，原版仍把 `Input` 的 prompt、Tool arguments/result、reason、Subagent material 与 identities 直接交给 `json.Marshal`，没有字段或 stdin 总上限，也没有截断事实。失败优先反例 `a47c9c7c8` 证明超过 256 KiB 的 prompt/arguments、超过 128 KiB 的 result、超过 8 KiB 的 reason 与超过 512 KiB 的聚合 identity 都会被完整编码并启动 shell。
- 决策：Hook Domain 唯一拥有 prompt 256 KiB、lossless arguments 256 KiB、result 128 KiB、reason/description/error 8 KiB 的 command material contract，并返回 ownership-isolated projection；自由文本先成为有效 UTF-8，再以 marker 标明 prompt/result 的前缀投影，arguments 超界整体拒绝。Application 只在至少一个 command hook 实际匹配时懒投影一次并复用于该事件的所有 command，不让 declarative inject 经过无关转换。Shell 在 marshal 前以 512 KiB raw-material budget 拒绝任意大输入，marshal 后再以 512 KiB exact JSON envelope 拒绝 escaping expansion，随后才创建 timeout 与 process。
- 后果：Hook command stdin 的驻留、复制和外部可见语义都有明确 owner；大型 prompt/result 不改变原始 Runtime consumer，只给 hook marked prefix，Tool arguments 不会被静默截断成另一动作。projection/encoding failure 沿既有 observable non-blocking broken-hook path 收敛，纯 declarative context 继续独立生效。没有复制 app2 `hookflow`/`hookprocess` facade、第二 invocation type、配置旋钮、legacy unbounded wire 或 producer-by-producer cap；private JSON marker 是允许的 breaking change。

## ADR-RT-085：Authored prompt sources 是有限完整文档与级联

- 状态：已接受并实施，P166 完成；只改变 Runtime internal Workspace Application、promptsource/agentexec adapter 与测试文档，公共 Protocol shape、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 合同不变。
- 背景：Knowledge、Skill Proposal 与 Hook 配置已有完整文档包络，但 AGENTS.md 和 Recipe 仍直接 `os.ReadFile`/`os.ReadDir`。失败优先反例 `b15d7d746` 证明超过 1 MiB 或非法 UTF-8 的两类文件都会被接纳，单 scope 129 份及超过 8 MiB 的 Recipe 会完整 materialize；一份超过 32 KiB 的 AGENTS.md 又会被 model projection 静默整份删除，使管理发现与真实 Run 指令分裂。复核反例 `fc54339ef` 进一步证明 broken 高优先级 AGENTS symlink 会被误判为 missing 并回退低优先级副本。Recipe body 还是 Desktop slash command 的直接用户输入，不能让 HTTP 或 provider context 间接承担容量。
- 决策：不为两种薄能力新建 Domain/facade，由既有 `application/workspace` 唯一发布 1 MiB valid-UTF-8 document、64 documents/4 MiB Agent cascade、128 recipes/scope 与 256 recipes/8 MiB complete catalog 合同及稳定错误。`adapter/promptsource` 共用 stat + cancellation-aware `limit+1` reader，在 string/YAML 前复验读取期间增长，并以 1024-entry sentinel 限制 Recipe directory；project-over-global shadowing 只发生在高优先级完整文档通过准入后，existing invalid source 整体失败。Application 对 direct port 再验证，不能用测试替身或新 adapter 绕过。
- 决策：Agent WorkingContext 的 32 KiB consumer budget 继续只选择 most-specific complete-document tail；每份文档连同 provenance 必须能独立装入预算，否则 fresh Run 明确失败。缺失/空文件仍是不存在的 source；read/UTF-8/size 或 projection failure 不再被跳过，ADR-RT-081 当时保留给 Agent documents 的 best-effort 语义由本 ADR supersede。
- 后果：`agentDocs.list`、`recipes.list`、Desktop slash command 与 fresh Run 只观察有限、完整且一致的 authored facts；任意大的本地文件、目录或 aggregate 不再先进入 Go string/YAML/JSON/model material。合法 precedence、malformed-frontmatter-as-body、Recipe wire 与 AGENTS provenance shape 保持；没有 partial document、低优先级 fallback、分页伪装、配置旋钮、兼容 reader 或第二 discovery path。

## ADR-RT-086：Workspace VCS read model 必须在外部进程到 Application 逐层有界

- 状态：已接受并实施，P167 完成；只改变 Runtime internal workspace Application/adapter/Git process contracts，公共 Protocol、Artifact、SQLite、公共 Go API、Desktop source、Agent Framework 与 CLI 不变。
- 背景：失败优先反例 `a4560d57b` 证明 `workspace.changes.list` 可接纳 10,001 个完整条目，structured diff 的第一文件可用 5,001 行绕过 limit，raw aggregate 与单次 Git stdout 可超过 64 MiB，untracked symlink stat 还会跟随 workspace 外 referent。补充反例 `f88b11f51`、`4ae8d127a` 与 `45d8ba314` 又证明 Application direct port 可用单行 64 MiB+ material 绕过下层限制，file-count truncation 后 parser 仍会进入下一文件，binary/quoted Git path 会丢失或保留错误编码。旧 Desktop 查询没有发送 limit，而 Runtime 把零解释为无限，transport 和最终 slice 都不能成为隐式资源 owner。
- 决策：`application/workspace` 唯一发布 10,000 changes、5,000 files、默认/最高 5,000 rows 与 64 MiB diff material，并把 limit 传入 internal Git port 后对 direct result 再次按完整文件复验。所有 Git observation 统一经 `gitprocess.Run`：stdout 64 MiB、stderr 64 KiB bounded drain、稳定 locale/env 与 caller cancellation；watcher 的 background observation 另有 10 秒 lifetime。Git diff 禁用 pager/external diff/textconv/color，tracked + per-untracked no-index patch 的 aggregate 不能超过调用预算；parser 使用 byte iteration，在进入下一文件或保留下一行前检查预算，只返回完整文件并恢复 binary/C-quoted path。Changes/status 与 file list 在保留 cardinality 越界项前失败；untracked regular file streaming 统计，symlink 只读 link 自身语义。

## ADR-RT-087：Mutation stamp 与 formatter 必须共同服从 adapter-owned 资源包络

- 状态：已接受并实施，P168 完成；只改变 Runtime internal Tool adapter，公共 Protocol、Artifact、SQLite、公共 Go API、Desktop source、Agent Framework 与 CLI 不变。
- 背景：失败优先反例 `280391534` 证明受支持的 8 MiB+ JSON 会被 `os.ReadFile` 完整接纳并重写，外部 formatter 的 128 KiB stderr 会完整进入 Tool result；read-before-mutation tracker 在成功删除后仍保留旧 SHA-256，外部用相同 bytes 重建路径时可继承已经删除资源的修改授权。旧 fingerprint 还会为每次 read/mutation 额外 materialize 完整文件并忽略读取失败。caller context 与最终 Tool-result offload 不能修复这些 admission 和 identity 缺口。
- 决策：Tool adapter 只 auto-format 最多 8 MiB 的完整 regular file，以 stat + cancellation-aware `limit+1` 覆盖既有超限与读取期间增长。Go 改用 `go/format`、JSON 改用进程内 indent；Prettier 只接收有界 stdin 并返回最多 8 MiB stdout，不再获得 in-place write 权，stderr 用 64 KiB bounded-drain buffer，进程使用 caller context 与 1 秒 `WaitDelay`。所有分支只有在完整输出成功后才走既有 atomic replace。bounded writer 使用私有 buffer 组合，不能嵌入 `bytes.Buffer` 暴露 `ReadFrom` 绕过 `Write`。
- 决策：Mutation fingerprint 改为 caller-cancellable、64 KiB chunk 的 streaming SHA-256；read stamp、mutation precheck 与 post-refresh 对当前 regular file 的 inspect/read failure 全部 fail closed。删除后显式 forget stamp，创建/修改后只记录新 fingerprint；post-refresh 失败先撤销旧 stamp，再返回可观察错误，不保留可能错误的授权。
- 后果：大文件、formatter 噪声和 read-before-mutation safety 不再依赖进程内存、Tool result 或偶然的相同 bytes。Go auto-format 少一个外部进程，Prettier 不能在输出验证前部分改写目标；没有 formatter facade、兼容签名、配置旋钮、后台 hash worker或第二 mutation owner。

## ADR-RT-088：Runtime file read 必须拥有固定输入、完整行结果与稳定 stamp 区间

- 状态：已接受并实施，P169 完成；只改变 Runtime internal Tool adapter，公共 Tool name/request/response shape、Protocol、Artifact、SQLite、Desktop source、Agent Framework 与 CLI 不变。
- 背景：失败优先反例 `1467db325` 证明 model/direct `read` 会接纳并完整 materialize 8 MiB+ 文件、默认返回 1.2 MiB 两行且把 `Truncated` 标为 false、忽略已取消 context。更严重的是旧 tracking 只在 read 返回后 fingerprint：若 Tool 已读到旧内容、文件在 stamp 前被替换，新内容的 digest 会被记录，后续 mutation 因而获得模型从未观察过的授权。Tool-result offload 发生在这些分配和错误 stamp 之后，不能充当 read owner。
- 决策：Runtime 用一个消费端 `runtimeReadExecutor` 覆盖通用 fs executor 的 Read，model 与 direct registry 共用。完整 regular file 最多 8 MiB，单行最多 1 MiB；64 KiB buffered reader 以 cancellation-aware `limit+1` 流式扫描，拒绝读取期间增长、invalid UTF-8/NUL 和 unpageable line，同时保留 BOM/CRLF/trailing-empty-line 语义。默认结果预算固定 1 MiB，只返回完整行；`StartLine/EndLine/TotalLines/Truncated` 精确描述所见 window，下一次从 `EndLine+1` 继续。
- 决策：read tracking 在调用前后分别以同一个 8 MiB hard cap 做 streaming SHA-256，只有 file 在整个 Tool call 区间存在且 digest 不变才提交 stamp；任一 fingerprint/admission/cancellation 失败或 generation 变化都 forget 并要求重读。Mutation precheck/post-refresh 继续允许超过 8 MiB 的既有目标用 bounded-memory streaming digest 守住安全，但 model read 不会授权自己无法完整准入的文件。
- 后果：少量行请求不再通过 `os.ReadFile` 复制完整文件，大文件不能借 offload/line paging 进入无界 materialization，取消在扫描期间可见；read-before-mutation 的授权与模型实际读取区间线性一致。保留通用 `tools/fs` 给其他模块的既有行为，不修改它、CLI 或公共 wire，也不新增第二 Read request、byte-offset compatibility API 或配置旋钮。
- 后果：采用 app2 已验证的 64 MiB Git output、64 KiB stderr 与 5,000-row default，但补上其仍会完整 materialize oversized current file、缺少 changes/file-count owner、跟随或复制 untracked content 等缺口。`workspace.diff.get` 的 wire shape 与 Desktop `truncated` presentation 保持，零 limit 从隐式无限直接 breaking 为默认 5,000；oversized complete facts fail closed，不用 partial raw patch、filesystem fallback、兼容别名、配置旋钮或第二 Git runner 掩盖。后台 observation 与外部进程都有可终结 lifetime，本批未启动浏览器或留下临时进程。
