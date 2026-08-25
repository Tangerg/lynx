# Agent Framework 公共合同基线

> 状态：Baseline 24 已冻结
> 冻结日期：2026-08-26
> 适用范围：`agent` 根 package、`agent/agenttest`、`agent/interaction`、`agent/planning`、`agent/planning/goap`、`agent/workflow`、`agent/otel`、`agent/platform`、Process Snapshot v6、TreeSnapshot v4、child/framework-effect protocol v2、Interaction state/protocol v7/v6、Planning state/protocol v3/v1、Workflow state v2、Event/Delta observation wire

本文只记录已经由 P3 真实 Interaction、P4 child composition、八个独立 command consumer、P5 真实 Planning/GOAP、P6 managed Workflow、P8 Platform 与恢复合同共同证明的公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

## 1. 基线的含义

Baseline 24 不是兼容承诺或发布版本。仓库仍允许 breaking change，但任何公共名称、参数名、签名、GoDoc、派生 JSON Schema、sentinel error、Framework/Strategy recovery wire 或 observation wire 的变化都必须是显式设计决策：

1. 先用真实 Strategy 或 consumer 证明变化必要；
2. 更新或追加 ADR，不保留 alias、双读、双写或兼容 shim；
3. 同一提交更新本基线与自动守卫；
4. 重新执行 standalone build/vet/staticcheck/test/race、相关 fuzz 和所有 examples。

无意变化必须被测试直接阻止，不能靠 review 人工记忆当前 API。

## 2. 已冻结的公共窄腰

根 package 的合同按 owner 分为以下单一概念：

- Definition/Execution/ExecutionState：不可变定义、有界纯 Step、Strategy-owned portable state。
- Descriptor/Deployment/DeploymentRef/DeploymentResolver：权威 schema、冻结运行绑定与 exact resolution。
- Engine/Process/Result/Termination：唯一生命周期 owner、Engine-issued handle、提交式 cancellation request、best-effort Delta ordering barrier 与稳定终态事实。
- Signal/SignalRequest/WaitID/WaitKey：唯一入站信封、外部投递请求、Engine identity 与 Strategy logical identity。
- Transition/Effect/EffectRequest/Settlement：Step 候选意图、边界外操作、dispatcher 请求与确定/未知结算。
- Snapshot/TreeSnapshot：单独 root 与完整 Process tree 的 portable capture；不包含 Host persistence 抽象。
- StartChild/WaitForChildren/ChildOutcome：跨 Strategy 组合的最小 Framework Effect/Signal 协议。
- PreparedWaitingSubtreeCancellation：冻结等待非 root Process 的 source tree、暴露 exact prospective result，并以恰好一次 Apply/Discard 结束的 prepared tree 变换。
- Budget/Limits/TreeLimits/CapabilitySet/Usage：本地工作上限、tree expansion、authority attenuation 与事实计数。
- ProcessAdmission/ProcessAdmitter：根与子 Process 共用、只读、context-aware 且不能修改 Framework 资源的启动准入边界。
- ProcessStartOutcome/ProcessStartOutcomeAcknowledger：每个 accepted admission 唯一的 started/aborted 初始化结论，以及 Process 发布前的同步中性 acknowledgment。
- Event/Delta：自足携带 exact execution attribution 的权威已发生事实与 best-effort 临时增量；listener 无 veto/error 通道。
- Descriptor.EncodeInput/DecodeOutput 与 Input/Output.Decode：只存在于类型擦除边缘的 Go 1.27 方法泛型；不改变非泛型 Engine 窄腰。

`interaction` package 冻结为一个 Strategy：Definition 拥有 WorkingContext、模型/Tool 状态机、exact managed Delegate、typed Delegate artifacts 与可选 pure completion validator，Dispatcher 只拥有模型和普通 Tool I/O。Delegate 将目标 Descriptor Input 暴露为模型 Tool schema，但只能由 Execution 经 Framework `StartChild`/`WaitForChildren` 推进；成功 child Output 经 exact schema 复验后进入 immutable validator view，validator 读取防御性复制的当前 WorkingContext/candidate/artifacts，拒绝候选时以有界 feedback 进入下一模型轮次。`ModelInvocation.AppliedSteerSignalIDs` 只归因首次进入当前模型请求的 steer Signal；`ActiveDelegateChildrenFromSnapshot` 只解释 Interaction 自己的已提交 opaque state，并以不可变 `ActiveDelegateChild` 公开模型调用序号、ToolCall 位置和值、ChildKey 与 ProcessID。两者都不暴露 private wire，也不保存 Engine handle 或 Host identity。`HostFailure` 只标记 Host 在尚未跨越模型或 Tool 外部边界前已经拒绝的自有前置条件；Dispatcher 必须把它结算为确定的 terminal failure，不能伪装成 provider 错误或模型可见 Tool output。普通模型/Tool 错误的既有语义不变。HITL/steer/stream/tool concurrency 仍通过根窄腰表达。Interaction 不拥有 Process lifecycle、产品 history、artifact store、UI 或 approval policy。

`planning` package 冻结为另一个 Strategy：Goal、Condition、Truth、WorldState、Action、Plan 和 Planning outcome 全部由 Planning 拥有；Planner 只做无副作用搜索，Observer/ActionExecutor 只存在于 dispatcher 边界。Execution 每次只执行 Plan 的第一个 Action，随后重新观察、确认预测效果并重新规划。dispatcher Action 与 child Process Action 共用同一预测模型，但不共享执行 capability；unreachable/stuck 是 Planning Output，不是共同 Process 状态。

`planning/goap` 只实现确定、有界的 uniform-cost search。HTN、Utility、Reactive、registry、默认 Planner 和 Planning telemetry 均不在基线中。

`workflow` package 冻结为 managed child Process 的确定性有序编排 Strategy。Definition 只接受 sealed `Transform`、`Call`、`Switch`、`Fork`、`Map`、`Loop` Stage；泛型只在构造边缘建立 schema，Execution 仍通过 erased 根窄腰推进。Call、被选择 case、branch、item 和 iteration 都是 exact child Process。Fork/Map 的 `WindowSize` 精确表示固定执行窗口：整窗结算后才开下一窗，不承诺滑动补位。Workflow 不拥有 dispatcher I/O、Graph/Registry、Store/Journal、第二 scheduler 或 `flow` adapter。

## 3. 自动守卫

`baseline_test.go` 对八个已冻结公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败；AST 守卫还要求所有公开声明/字段有精确 GoDoc、公开 callable 的参数有语义名称，并禁止 error cause 通过 `%v` 丢失 `errors.Is/As` 链。Baseline 24 public digest：

P21 以 Go 1.27 方法泛型替换无消费者的 typed wrapper，并冻结此前未审计的 ExecutionObserver：

- root kernel：`41349ae9445b03de660e4f1f1360c92aa17ed4f7ca7de97fc553406831002125`
- agenttest：`4c549417607c1a4e8044357c6defa1135ce420d48a28d5f574cceeb9cead5490`
- interaction：`24c0f438579ea0b1af1323500090916db82c29e6b64b2fb4dee30df418da1518`
- planning：`48dcc733364cf5345332aeb0f3fd64aeefd2c21e7f0585759e44278b050eb50a`
- planning/goap：`4aa78b677748784182313d25a187b0074e49ea972c75db2e041c82a0f5f82529`
- workflow：`82dd31a06d26b01877f1c3df631083921fe59f58b0472f39e272897d2231b231`
- otel：`aeed9c638fae1729c2965b4bccd466edf858dd9a4cf49e9611386f910d4c5d60`
- platform：`5d2140197e3ac09ebf62a156b308b0327197716888974706c338cd14b9b9b21b`

Kernel 测试独立冻结其全部 production `*Wire`、Framework Event payload 与 schema version；每个 Strategy package 冻结自己的私有 ExecutionState 和 Effect/Signal/Delta protocol。覆盖守卫要求新增 production wire 或私有 JSON struct 必须进入所有者 baseline，Kernel 始终只保存 opaque `ExecutionState.Payload`，不会递归解释 Strategy shape。Baseline 24 wire digest：

P21 不改变 JSON 字段、schema version 或协议语义；下列 digest 显式切换到 Go 1.27 标准库呈现的 `jsontext.Value` 类型名：

- Kernel snapshot/protocol wire：`b5cb67c9b840addb0785fe97d96de0fbc6a7ce8278e93b1724eb6a5e74892c54`
- Framework Event/Delta observation wire：`4167463188bdc4fcfac6cbadc94abb8bf81d2bb0da3b78870b5953237d137353`
- Interaction state/protocol wire：`73a91aca91d2a968636d90aebd11041c149e0e06afc2f8efc0eac6f4b42b64de`
- Planning state/protocol wire：`dc6f02ca28f1fbb9e14899bd3103a781b4f5341a397cd8d8bbef279d198a784e`
- Workflow state wire：`2d4d33d0c9996077abb594f3d2aab47d37059c6e126ad5e971ee8d484bb4442d`

Digest 只用于发现未审计漂移，不替代 strict codec、round-trip、malformed input、restore、fuzz 或 consumer behavior tests。

Baseline 2 对根 Kernel 的唯一公开修订是 `ChildWaitOpened.Spec`：Planning 的真实 child Action 证明 Strategy 必须校验 Engine 确认的完整 child-wait request，而不能只相信 WaitID。Baseline 3 最初加入 Workflow；P7-02 依据 ADR-A2-040 对 Interaction 做了一次显式审计修订：新增 `Delegate`/`DelegateConfig`/`ErrInvalidDelegate`，`DefinitionConfig` 新增 `Delegates`，`NewDispatcher` 必须接收 exact `*Definition` 以冻结同一模型 manifest。Interaction 私有 ExecutionState 从 v1 直接升级为 v2，不双读旧状态。根公开 API、Process Snapshot v3、TreeSnapshot v1 及其 wire digest 均未变化。

P7-03 依据 ADR-A2-041 再次审计修订 Interaction：新增 opaque `Artifact`/`Artifacts`/`CompletionCandidate`、`CompletionDecision`、`CompletionValidator`、`DecodeArtifact[T]` 与 `ErrInvalidArtifact`，`DefinitionConfig` 新增可选 `CompletionValidator`。这些 API 只服务已实现的 exact Delegate typed evidence 与显式完成反馈，不形成产品 store 或新 Strategy。Interaction 私有 ExecutionState 从 v2 直接升级为 v3，不双读旧状态；根、Planning、GOAP、Workflow API 与共同 snapshot/tree wire 均未改变。

P7-04 依据 ADR-A2-042 用独立消费者证明两种 orchestrator-worker 组合：Interaction 直接选择 exact Planning Delegates；以及 decomposer Interaction 输出 typed task list、Workflow Map 确定调度 exact workers、synthesizer Interaction 聚合结果。实现没有增加 Supervisor/Worker/Task 公共抽象，也没有修改任何生产 package、ExecutionState 或共同 wire，因此五个 API digest 与 snapshot/tree digest 全部保持不变。

P7-05 依据 ADR-A2-043 用独立消费者证明 evaluator-optimizer 可完全派生自 Workflow Loop、exact optimizer/evaluator child 和 consumer-owned typed state。显式 threshold/limit、feedback、history、best-so-far 与 accepted/exhausted 均未形成 Framework 公共类型；实现没有修改生产 package、ExecutionState 或共同 wire，因此全部 API/wire digest 继续保持不变。

P7-06 依据 ADR-A2-044 用 `workflow_patterns` 补齐 prompt chaining、routing、parallel sectioning/voting 的联合 consumer 证据，并与既有六个 command 形成完整 Anthropic 模式矩阵。模式名称没有变成 Framework package/type/kind；实现仍只组合冻结 API，因此全部 API/wire digest 继续保持不变。

P7-07 依据 ADR-A2-045 完成 direct `chatclient`、普通 Go/`flow`、单 Process Interaction、managed Workflow/tree 的复杂度—收益终审。Delegate/artifact/validator 与六个 Workflow Stage 均有独立语义和真实消费，当前没有应为制造 diff 而删除的生产抽象；也没有新增 facade。P7 最终仍保持全部 API/wire digest 不变。

P8-01 依据 ADR-A2-046 从真实 child start 与 tree restore 消费点修订根合同：`DeploymentResolver.Resolve` 移除误导性的 `context.Context`，只表达同步、有界、确定、无远程 I/O 的 exact binding lookup；路由和调用方选择必须先于该合同完成。Engine 继续复验 exact reference、隔离 resolver panic、绕过 same-reference child lookup，并在 tree restore 中按 distinct reference 缓存且维持 all-or-nothing。完整 race 门禁又依据 ADR-A2-047 修正 `Process.Await` 的线性化点：返回 terminal Result 前，Engine 已提交该终止直接触发的父子 bookkeeping，不再给 caller 留出越过 parent termination propagation 的窗口。根 API/GoDoc digest 更新为当前值；四个 Strategy package 与共同 snapshot/tree wire digest 不变。

P8-02 依据 ADR-A2-048 新增尚未冻结的 `platform.Catalog` 候选 API，并以它的 exact missing/duplicate 诊断证明 `DeploymentRef` 需要稳定 String 表示。根 package 新增 `DeploymentRef.String()`，输出 `name@version+complete-digest` 且不冒充 wire encoding；根 API/GoDoc digest 显式更新。Platform package 要到 P8 完整变更、路由、治理与真实消费者验证后才进入公共 digest 守卫；既有四个 Strategy package 和 snapshot/tree wire 仍不变。

P8-03 依据 ADR-A2-049 在 exact Catalog 之上增加尚未冻结的 Platform deployment aggregate：Config/New、ActiveDeployments、Deploy/Replace/Undeploy、exact Resolve 与精确 conflict/not-active errors。active slot 是 name + SemVer，历史 exact binding 只增不误删。该候选面没有修改根或四个 Strategy package，也没有改变共同 wire；P8-04～07 仍可依据真实 routing/governance consumer 治本修订它，不提供兼容 shim。

P8-04 依据 ADR-A2-050 为尚未冻结的 Platform 增加唯一 Definition selection 合同：non-executable DeploymentCandidate、DeploymentSelector/func adapter、DeploymentCandidates 与 SelectDeployment。没有 Router/Ranker/Choice/Confidence 近义层；request-specific policy 留在 selector。选择结果严格属于当次 active snapshot，并在并发 route change 后返回 captured exact Deployment。根、四个 Strategy package 与共同 wire 均未变化。

P8-05 依据 ADR-A2-051 修订根 Engine 合同：新增 immutable `ProcessAdmission`、单一 `ProcessAdmitter`/func adapter、`ErrProcessAdmissionRejected` 与 EngineConfig consumption point。一个 admitter 同构覆盖 root/child start，只能拒绝，不能修改由 Engine 唯一拥有的 Budget、TreeLimits 或 CapabilitySet attenuation；其合同同步、有界、无 I/O、不重入 Process，因此没有误导性 context，恢复也不重复 admission。没有增加 Policy/Guard/Extension registry、通用 StopPolicy 或 Platform-owned runtime，四个 Strategy API 与共同 snapshot/tree wire 均未变化。

P8-06 依据 ADR-A2-052 修订根 observation 合同：Event 新增 exact DeploymentRef/ProcessRelation，统一 Framework Event 名称常量，补齐 Step 与两类 Effect 的真实 started/finished、target/status/duration；EventListener/DeltaListener 删除无效 error 返回并增加 Func adapter。新增独立 `agent/otel` official-API adapter，Kernel 不 import OTel，production adapter 不 import SDK。根与 OTel API 以及 Event/Delta wire 显式冻结；四个 Strategy API 和 snapshot/tree wire 不变。

P8-07 依据 ADR-A2-053 用第八个公开 command 同时运行 embedded 与 Platform 两条路径：同一 root/worker/Input、admission、Event listener 和 Engine 产生相同 Output、Status、Usage、exact tree 与稳定 Process/Step/Effect observation 投影。Platform 没有第二 Start/Run/runtime。冻结前删除单字段 Config 和 executable ActiveDeployments 副视图，构造收敛为 `New(deployments...)` 且零值可用；发现只保留 non-executable DeploymentCandidates。只表示 nil 的错误精确命名为 ErrNilPlatform/ErrNilDeploymentSelector。`platform` API/GoDoc 现纳入 digest 守卫；其余六个 API 与全部共同 wire 不变。

P9-01 依据 ADR-A2-054 对七个 package 的公开/私有词汇、参数、字段、GoDoc、error 与 wire 名称做独立终审，并一次性形成 Baseline 4。公共面不再以 `Reference`、`Index`、`Deployment` 等宽词表示 `DeploymentRef`、Effect batch index 或 exact child ref；Signal consumption、Process/Event/Effect sequence 与 mailbox cursor 全部以 owner-qualified 名称表达。Planning output 的 `Attempt.ActionName` 和 dispatcher `ActionExecutors` 同步修正；完整 error cause 统一 `%w`。这批变化故意不保留 alias、旧 tag 或双读：Process Snapshot 升为 v4、TreeSnapshot 升为 v2、child/framework-effect protocol 升为 v2、Planning ExecutionState 升为 v2，旧格式明确不属于当前合同。

P9-02 依据 ADR-A2-055 独立复核后发现 Baseline 4 只冻结了共同 snapshot 外层，没有冻结其中 opaque Strategy payload 的所有者协议；Framework Event payload 也由匿名 struct 生成而未进入 observation digest。Baseline 5 将这两个漏口治本关闭：Interaction、Planning、Workflow 分别拥有并冻结自己的 state/protocol，Kernel 只冻结共同 envelope；Framework Event payload 收敛为命名 wire，并使用 `process_status`、`termination_cause`、`step_status`、`effect_target`、`settlement_status`、`dropped_delta_count` 等准确字段。Process Snapshot 升为 v5、TreeSnapshot 升为 v3、Interaction state/protocol 升为 v4/v2、Planning state 升为 v3、Workflow state 升为 v2；旧版本与旧 tag 直接拒绝，七个 public digest 不变。

P9-03 依据 ADR-A2-056 将七个生产 package 及其允许内部直连边冻结为单一可执行 DAG，并集中守卫 Host/旧模块、`flow`、logging backend、OTel 与 Interaction-owned protocol 的外部归属。该验收只改变 architecture tests 与文档；七个 exported API digest、全部 owner wire digest 和 Baseline 5 版本均不变。

P9-06 依据 ADR-A2-057 完成最终零债清扫，并形成 Baseline 6。公开状态名从非项目语言规范的 `StatusCancelled` 治本替换为 `StatusCanceled`，JSON wire 同步只接受 `"canceled"`；没有 alias 或旧拼写双读。Process Snapshot 升为 v6、TreeSnapshot 升为 v4，立即拒绝含旧状态拼写的既有格式。P1 disposable spike 测试在正式 Strategy、Engine 与恢复合同已经接管其证据后删除，普通测试不再跨文件借用 spike helper；其余六个 public package、Strategy protocol 和 observation wire 均不变。

P10-03 第一纵切依据 ADR-A2-058 形成 Baseline 7。`ProcessAdmitter.Admit` 新增语义必要的 `context.Context`，`ProcessAdmission.StartedAt` 暴露 admission 成功后使用的同一 UTC 生命周期时间；实现可协调 caller-owned 外部 admission，但仍不能重入 Engine/Process 或修改 Framework 资源。`EffectRequest` 新增 exact `DeploymentRef` 与 `ProcessRelation`，使 Strategy dispatcher 无需 ProcessContext 就能完整归因。root/child 都在 Definition.Start 与发布前 admission，restore 不重复；Snapshot/TreeSnapshot、Framework/Strategy protocol、Event/Delta 与其余六个 public package 均未改变。

P10-03 第二纵切依据 ADR-A2-058 形成 Baseline 8。Interaction 新增只在实际调用期间可读的 immutable `ModelInvocation`/`ToolInvocation`，自足携带 ProcessRelation、exact DeploymentRef、EffectID、Step sequence、model-call sequence 与 ToolCall index/value；新增唯一 `DelegateChildKey` 供 caller 以同一算法投影 child 因果关系。Dispatcher 将冻结普通工具明确分成初始可见 `Tools` 与初始隐藏但已具执行权限的 `DeferredTools`；`AdvertiseTools` 只能在成功 Tool 调用内按 exact 已绑定名称单调广告，失败、panic 或当前 HITL pause 均不提交。广告状态随完整 settlement、checkpoint 和 ExecutionState 原子推进并恢复，Interaction state/protocol 因而直接升级为 v5/v3，不双读旧格式。Kernel、其他 Strategy、共同 snapshot/tree 和 observation wire 均未变化。

P10-03 第三纵切依据 ADR-A2-058/A2-059 形成 Baseline 9。Kernel 新增唯一 immutable `WaitingSubtreeCancellationPlan` 与 `Engine.PlanWaitingSubtreeCancellation`/`ApplyWaitingSubtreeCancellation`：非 root Waiting target 的活动子树保留为准确 host-/parent-canceled 终态，关闭等待而不回收永久预算；Kernel child outcome 使直接父级在消费前进入 Paused。计划只持有纯 source identity/digest、确定 resulting TreeSnapshot 与 canceled/paused IDs；Apply 先完成 same-Engine/source/staged projection 验证，stale 零修改，跨 apply gate 后保留既有 Process handle 并完成 finalization。Process Snapshot v6、TreeSnapshot v4、Kernel/Strategy protocol 与 observation wire 无需升级，其余六个 public digest 不变。

P10-04 依据 ADR-A2-060 形成 Baseline 10。Kernel 新增 immutable `ProcessStartOutcome`、`ProcessStartOutcomeStatus` 和唯一 `ProcessStartOutcomeAcknowledger`；每个 accepted root/child admission 在 initial Definition/Execution snapshot 自证后只闭合为 started，任一初始化失败只闭合为带稳定 Failure 的 aborted。Engine 私有 start reservation 保证 acknowledgment 前零发布、ack error/panic 零发布、Close/identity/tree-limit 并发一致；admission reject 与 restore 不产生 outcome。该合同不包含 Host identity、持久化、transaction、应用 disposition 或 callback capability。Process Snapshot v6、TreeSnapshot v4、Kernel/Strategy protocol、observation wire 与其余六个 public digest 不变。

P10-05 依据 ADR-A2-061 形成 Baseline 11。Kernel 以一次性 `PreparedWaitingSubtreeCancellation` 取代 pure value plan 与后续 Engine apply：Prepare 在完整 quiescent cut 上冻结 source root tree，并返回 exact resulting TreeSnapshot 与 canceled/paused IDs；caller 必须恰好一次 Apply 或 Discard。Apply gate 前任何错误自动释放且 live tree 零修改，跨 gate 后独立于调用 context 完成；不同 root tree 的 operation 彼此独立。旧 plan/apply、Engine identity、source digest 和 stale/foreign errors 全部删除。capability 只拥有 Framework state，未加入 Runtime transaction/checkpoint/lease 等 Host 抽象。Process Snapshot v6、TreeSnapshot v4、Kernel/Strategy protocol、observation wire 与其余六个 public digest 不变。

P10-06 的真实 Runtime tree-restore consumer 依据 ADR-A2-062 形成 Baseline 12。Interaction 新增 immutable `ActiveDelegateChild` 与 `ActiveDelegateChildrenFromSnapshot`，让 Host 在恢复完整树后从 Strategy owner 读取当前 model ToolCall 到 ChildKey/ProcessID 的精确归因，而不复制 private ExecutionState wire 或把 SpawnCallID 写成第二真相源。helper 只读取 snapshot 的已提交 Interaction state；非 Interaction 或没有活跃 Delegate segment 返回 `found=false`，不一致 state 明确失败。根 Kernel、其余六个 public package、Process Snapshot v6、TreeSnapshot v4、全部 owner wire与 observation wire不变。

P10-06 Runtime 的完整 waiting Delegate tree 反证依据 ADR-A2-063 形成 Baseline 13。`PendingToolInputFromSnapshot` 现在区分合法的 Tool-input wait 与 Delegate-child wait：前者返回 typed pending input，后者在 owner validation 和 outer/committed WaitID 一致后返回 `found=false`；不相容 phase 或身份错配仍明确失败。没有增加通用 wait union、Kernel 状态或 Host 分支，七个 public digest 与全部 owner wire digest均不变。

P10-06 Runtime 的 durable prepared-change consumer 依据 ADR-A2-064 形成 Baseline 14。`PreparedWaitingSubtreeCancellation.Apply(ctx)` 治本式收敛为 `Apply()`：所有可能失败或取消的 Process projection staging 已在 `PrepareWaitingSubtreeCancellation` 返回 capability 前完成；caller 的 durable decision 存在后，Apply 只跨越不可撤销的 Framework gate并完成既有 finalization，因此请求 context 不是合法输入。该变化未加入 transaction、checkpoint、Run 或 application disposition；Process Snapshot v6、TreeSnapshot v4、全部 Strategy/observation wire与其余六个 public digest不变。

P11 依据 ADR-A2-065 形成 Baseline 15。原框架实现整体删除，绿色重写实现从临时孵化位置原子安装为唯一 `github.com/Tangerg/lynx/agent` module；Runtime consumer、workspace metadata、package docs、examples 和 architecture guards 同步切换。没有 alias module、replace compatibility、转发 package 或双路径。模块路径切换不改变七个公共 package 的语义，也不改变 Process Snapshot v6、TreeSnapshot v4、Strategy protocol 或 observation wire；GoDoc digest 以 canonical package path 重新冻结。

P14-01 依据 ADR-A2-066 形成 Baseline 16。`PreparedWaitingSubtreeCancellation` 的 GoDoc 明确一次性 authority 由所有别名共享；私有 resolution identity 使误复制的值也不能获得第二次 Apply/Discard 权限。公开名称、方法签名和行为方向不变，Process Snapshot v6、TreeSnapshot v4、全部 Strategy/observation wire 与其他六个 public digest 均未变化。

P14-02 依据 ADR-A2-067/A2-069 形成 Baseline 17。Engine 与 OTel Observer 的 GoDoc 明确它们是禁止使用后复制的单一 mutable owner；Process GoDoc 明确 command ctx 只界定提交与响应等待，Engine loop 接收后不撤销命令。公开名称和签名不变。ADR-A2-068 同批修正 Process Event sequence 与 OTel 数值投影，但不改变 Event/Delta wire：sequence attribute 统一以十进制字符串无损表达 `uint64`，Delta drop 到 OTel `Int64Counter` 的窄化使用显式饱和。

P15-01 依据 ADR-A2-070/A2-071 形成 Baseline 18。根 Process 以 `RequestCancellation` 取代会让 caller 误读同步响应的 `Cancel`：nil 只表示有效请求已经进入 Engine 单写者队列，终态仍由安全边界和 `Result` 给出；旧名称不保留 alias。Interaction 的 `ModelInvocation.AppliedSteerSignalIDs` 精确返回首次进入该模型请求的 steer Signal 身份，Dispatcher protocol 同步携带这一 Strategy-owned attribution；pending steer 将消息与有序 SignalID 作为一个不可拆分状态恢复。Interaction ExecutionState/protocol 直接升级为 v6/v4，旧格式不双读；Process Snapshot v6、TreeSnapshot v4、Kernel wire、其他 Strategy 与 observation wire 均未改变。

P15-02b 依据 ADR-A2-072 对 Baseline 18 只做语义精确化：Interaction、Planning 与 Workflow 统一直接使用 Execution 合同，只有与独立 in-process `flow` 对照时才称 managed Workflow；prepared Step 的 GoDoc 只描述候选状态与固定 Effects，不借用 Host 持久化概念。公开 identifier、签名、行为和全部 wire 均未变化；root GoDoc digest 已显式更新，自动词汇守卫防止无对照限定词与 Host 概念再次漂入 Framework。

P16 依据 ADR-A2-073 形成并冻结 Baseline 19 的派生 Schema 修订。`SchemaFor` 对 `json.RawMessage` 使用任意合法 JSON value 合同，对 `[]byte` 使用 null/base64 string 合同，准确匹配 `encoding/json`；其他字段仍按具体 Go 类型严格派生。`TestSchemaForMatchesEncodingJSONWireTypes` 与外部 Interaction consumer 回归直接冻结行为，package DAG 继续禁止任何 `app/runtime` production import。七个 public/GoDoc digest、全部 snapshot/Strategy/observation wire digest 与 schema version 均不变，但依赖修正后 JSON Schema 的 Descriptor/Deployment digest 会按内容变化，不提供旧 digest 兼容。

P17 依据 ADR-A2-074 形成 Baseline 20。新增 `agenttest` 叶子 package，冻结 scripted Effect dispatch、Event/Delta recording 和 prepared-step acknowledgment fault injection；外部 Engine consumer 验证它们只消费公共窄腰。Workflow 新增完整、无函数、独立持有 slice 的 `Topology` 投影并由现有 command 消费，覆盖六种 Stage、exact child contract 与显式界限。根、Interaction、Planning、GOAP、OTel、Platform、全部共同/Strategy wire 与 schema version 均未变化。

P18 只精修 Baseline 20 已冻结能力的私有行为所有权：scripted dispatch 的匹配、发射与 settlement 归私有 frozen step；Workflow Topology 的投影归 sealed Stage/binding/kind。公开声明、GoDoc、JSON shape、digest 输入、snapshot/Strategy/observation wire 与 schema version 均未变化，因此不形成 Baseline 21。

P19 依据 ADR-A2-075 形成 Baseline 21。Kernel 新增 `Engine.FlushDeltas(ctx)` 这一唯一 observation ordering barrier：它只等待调用前已被有界队列接受的 best-effort Delta 完成 listener delivery，不恢复已经丢弃的增量，也不承诺后续增量或持久化。真实流式 consumer 用它阻止权威完成值越过已接收的增量；Runtime 只消费公共 barrier，Kernel 不认识 Run、Item、协议或产品状态。root API/GoDoc digest 显式更新；Event/Delta wire、snapshot、Strategy protocol 与其他七个 public package 均未变化。

P91 依据 ADR-A2-076 形成 Baseline 22。Interaction 新增中性的 `ErrHostFailure`/`HostFailure` 标记，供真实 Host 在模型或 Tool 尚未被调用、其自有前置边界已经拒绝时要求确定终止。Dispatcher protocol 从 v4 直接升级到 v5，模型与 Tool settlement 以互斥的 `host_error` 模式承载该事实；Execution 统一产出 `interaction.host.failed`，不把它误判为 provider failure 或模型可见 Tool output。普通模型/Tool 错误与已经跨越外部调用后的 unknown settlement 均不改变；Process Snapshot v6、TreeSnapshot v4、Interaction ExecutionState v6、Kernel/其他 Strategy/observation wire 和另外七个 public package 全部不变。

P20 依据 ADR-A2-077 形成 Baseline 23。Core Chat 将从未被真实能力支持的复数 `Choice` 模型收敛为唯一 `Response.Result`；Interaction 的 ExecutionState、model settlement 与 stream Delta 都嵌入该 Request/Response wire，因此 state/protocol 从 v6/v5 直接升级为 v7/v6并拒绝旧版本。Interaction 直接读取单 Result，删除多候选循环与仲裁分支。Agent 公共 API、Process Snapshot v6、TreeSnapshot v4、Kernel/Planning/Workflow/observation wire 和另外七个公共 package均不改变。

P21 依据 ADR-A2-078 形成 Baseline 24。Go 1.27 方法泛型让 schema owner `Descriptor` 直接提供 typed encode/decode，`Input`、`Output` 与 `Artifact` 也各自拥有自己的严格解码；没有生产消费者的 `Typed[I,O]`、`NewTyped`、`ErrInvalidTypedAdapter` 以及三个自由解码函数整体删除，不保留 alias。Workflow 的 Map/Fork 泛型 codec 同步收回有状态 owner。此前已进入 HEAD 但未通过文档守卫的 `ExecutionObserver` 参数和 `ToolSettlement` 字段完成准确 GoDoc/命名审计。Go 1.27 标准库类型名使四组 wire digest 重新冻结，但 JSON 字段、版本和行为不变。

## 4. 明确不在基线中的能力

Baseline 24 保持唯一 `agent` module、一次性 prepared authority、mutable owner pointer identity、提交式取消请求、Interaction-owned steer 归因与准确 JSON wire schema 合同。Delta barrier 只表达 Framework-owned observation ordering，不接收 Host callback、事务或资源 identity；Interaction host-failure 标记只表达外部调用前的 Host 拒绝，不携带 Runtime、RPC、数据库或产品终态。Core Chat 的单 Result 是 Interaction 唯一模型响应合同，不提供复数候选兼容读取。派生规则只认识标准库 JSON 语义，不认识 Runtime/provider；模块路径变化不引入 alias、replace compatibility 或旧 wire 双读。八个公共 package及全部 snapshot、Strategy protocol 和 observation wire 的语义仍由各自 digest 守卫。`agenttest` 不模拟 Framework 生命周期；`flow` 保持独立 in-process 库，不形成 Agent adapter API 或依赖；Workflow Topology 只是 sealed algebra 的静态投影，不是可执行图。未来编辑器图只能在更高层编译成已验证的 Workflow Definition，不能反向扩张 Kernel 或恢复 wire。
