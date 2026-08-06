# Agent Framework 公共合同基线

> 状态：Baseline 5 已冻结
> 冻结日期：2026-08-06
> 适用范围：`agent2` 根 package、`agent2/interaction`、`agent2/planning`、`agent2/planning/goap`、`agent2/workflow`、`agent2/otel`、`agent2/platform`、Process Snapshot v5、TreeSnapshot v3、child/framework-effect protocol v2、Interaction state/protocol v4/v2、Planning state/protocol v3/v1、Workflow state v2、Event/Delta observation wire

本文只记录已经由 P3 真实 Interaction、P4 child composition、八个独立 command consumer、P5 真实 Planning/GOAP、P6 managed Workflow、P8 Platform 与恢复合同共同证明的公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

## 1. 基线的含义

Baseline 5 不是兼容承诺或发布版本。仓库仍允许 breaking change，但任何公共名称、参数名、签名、GoDoc、sentinel error、Framework/Strategy recovery wire 或 observation wire 的变化都必须是显式设计决策：

1. 先用真实 Strategy 或 consumer 证明变化必要；
2. 更新或追加 ADR，不保留 alias、双读、双写或兼容 shim；
3. 同一提交更新本基线与自动守卫；
4. 重新执行 standalone build/vet/staticcheck/test/race、相关 fuzz 和所有 examples。

无意变化必须被测试直接阻止，不能靠 review 人工记忆当前 API。

## 2. 已冻结的公共窄腰

根 package 的合同按 owner 分为以下单一概念：

- Definition/Execution/ExecutionState：不可变定义、有界纯 Step、Strategy-owned portable state。
- Descriptor/Deployment/DeploymentRef/DeploymentResolver：权威 schema、冻结运行绑定与 exact resolution。
- Engine/Process/Result/Termination：唯一生命周期 owner、Engine-issued handle 与稳定终态事实。
- Signal/SignalRequest/WaitID/WaitKey：唯一入站信封、外部投递请求、Engine identity 与 Strategy logical identity。
- Transition/Effect/EffectRequest/Settlement：Step 候选意图、边界外操作、dispatcher 请求与确定/未知结算。
- Snapshot/TreeSnapshot：单独 root 与完整 Process tree 的 portable capture；不包含 Host persistence 抽象。
- StartChild/WaitForChildren/ChildOutcome：跨 Strategy 组合的最小 Framework Effect/Signal 协议。
- Budget/Limits/TreeLimits/CapabilitySet/Usage：本地工作上限、tree expansion、authority attenuation 与事实计数。
- ProcessAdmission/ProcessAdmitter：根与子 Process 共用、只读且 decision-only 的启动准入边界。
- Event/Delta：自足携带 exact execution attribution 的权威已发生事实与 best-effort 临时增量；listener 无 veto/error 通道。
- Typed/EncodeInput/DecodeOutput：只存在于类型擦除边缘的人体工程学 adapter。

`interaction` package 冻结为一个原生 Strategy：Definition 拥有 WorkingContext、模型/Tool 状态机、exact managed Delegate、typed Delegate artifacts 与可选 pure completion validator，Dispatcher 只拥有模型和普通 Tool I/O。Delegate 将目标 Descriptor Input 暴露为模型 Tool schema，但只能由 Execution 经 Framework `StartChild`/`WaitForChildren` 推进；成功 child Output 经 exact schema 复验后进入 immutable validator view，validator 读取防御性复制的当前 WorkingContext/candidate/artifacts，拒绝候选时以有界 feedback 进入下一模型轮次；HITL/steer/stream/tool concurrency 仍通过根窄腰表达。它不拥有 Process lifecycle、产品 history、artifact store、UI 或 approval policy。

`planning` package 冻结为另一个原生 Strategy：Goal、Condition、Truth、WorldState、Action、Plan 和 Planning outcome 全部由 Planning 拥有；Planner 只做无副作用搜索，Observer/ActionExecutor 只存在于 dispatcher 边界。Execution 每次只执行 Plan 的第一个 Action，随后重新观察、确认预测效果并重新规划。dispatcher Action 与 child Process Action 共用同一预测模型，但不共享执行 capability；unreachable/stuck 是 Planning Output，不是共同 Process 状态。

`planning/goap` 只实现确定、有界的 uniform-cost search。HTN、Utility、Reactive、registry、默认 Planner 和 Planning telemetry 均不在基线中。

`workflow` package 冻结为 managed child Process 的确定性有序编排 Strategy。Definition 只接受 sealed `Transform`、`Call`、`Switch`、`Fork`、`Map`、`Loop` Stage；泛型只在构造边缘建立 schema，Execution 仍通过 erased 根窄腰推进。Call、被选择 case、branch、item 和 iteration 都是 exact child Process。Fork/Map 的 `WindowSize` 精确表示固定执行窗口：整窗结算后才开下一窗，不承诺滑动补位。Workflow 不拥有 dispatcher I/O、Graph/Registry、Store/Journal、第二 scheduler 或 `flow` adapter。

## 3. 自动守卫

`baseline_test.go` 对七个已冻结公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败；AST 守卫还要求所有公开声明/字段有精确 GoDoc、公开 callable 的参数有语义名称，并禁止 error cause 通过 `%v` 丢失 `errors.Is/As` 链。Baseline 5 public digest：

- root kernel：`73b6f2270e4010886234cb080c277ba1960dcb09c8c123b550ba0c0a8ff6fb90`
- interaction：`b5fdabbea94d2b9aa346446a7cafb4accde928b23489012ba2763c77a517da91`
- planning：`15c48c52b7d4765ba86da2e5fd11822669c163c01e98dd4cb3668f71f7c5f30a`
- planning/goap：`dd5a007a20ddbeac2112bbed10718f5256fe2449376fd7dcc1400e25578253ec`
- workflow：`a0e4815dc7c9bb69f215702434b8547e2abdb446400efc445d3fdb35ff752094`
- otel：`0725fbef9fbd28ba9b6999ab8b427dd9a4376f83aef9839a9f15f60c16422016`
- platform：`748f5ea1ef3b09c702a792ab6e16a3b4ae6be9776ef1a4ba51e856757abff078`

Kernel 测试独立冻结其全部 production `*Wire`、Framework Event payload 与 schema version；每个 Strategy package 冻结自己的私有 ExecutionState 和 Effect/Signal/Delta protocol。覆盖守卫要求新增 production wire 或私有 JSON struct 必须进入所有者 baseline，Kernel 始终只保存 opaque `ExecutionState.Payload`，不会递归解释 Strategy shape。Baseline 5 wire digest：

- Kernel snapshot/protocol wire：`1b93af3cb1f0fcb8267b2c160a38e61317397c7c40c3b2a24c51f4f61eeb4066`
- Framework Event/Delta observation wire：`4006d3d24440922ba1e1ba9616bde3a88352fcc2625d918b839e1de283b631cb`
- Interaction state/protocol wire：`fdc75d1030debefa49d42ffd02c460e143ddc2aee09375630760e799f8ee220a`
- Planning state/protocol wire：`2d23947979849161f5d997b17c7510a95bc15274c78dc7aae70c21af1beee439`
- Workflow state wire：`d6666ac8046b8b1e14de9eb322abbea21ba98bf696a1cebed6e55a68f3583078`

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

P8-06 依据 ADR-A2-052 修订根 observation 合同：Event 新增 exact DeploymentRef/ProcessRelation，统一 Framework Event 名称常量，补齐 Step 与两类 Effect 的真实 started/finished、target/status/duration；EventListener/DeltaListener 删除无效 error 返回并增加 Func adapter。新增独立 `agent2/otel` official-API adapter，Kernel 不 import OTel，production adapter 不 import SDK。根与 OTel API 以及 Event/Delta wire 显式冻结；四个 Strategy API 和 snapshot/tree wire 不变。

P8-07 依据 ADR-A2-053 用第八个公开 command 同时运行 embedded 与 Platform 两条路径：同一 root/worker/Input、admission、Event listener 和 Engine 产生相同 Output、Status、Usage、exact tree 与稳定 Process/Step/Effect observation 投影。Platform 没有第二 Start/Run/runtime。冻结前删除单字段 Config 和 executable ActiveDeployments 副视图，构造收敛为 `New(deployments...)` 且零值可用；发现只保留 non-executable DeploymentCandidates。只表示 nil 的错误精确命名为 ErrNilPlatform/ErrNilDeploymentSelector。`platform` API/GoDoc 现纳入 digest 守卫；其余六个 API 与全部共同 wire 不变。

P9-01 依据 ADR-A2-054 对七个 package 的公开/私有词汇、参数、字段、GoDoc、error 与 wire 名称做独立终审，并一次性形成 Baseline 4。公共面不再以 `Reference`、`Index`、`Deployment` 等宽词表示 `DeploymentRef`、Effect batch index 或 exact child ref；Signal consumption、Process/Event/Effect sequence 与 mailbox cursor 全部以 owner-qualified 名称表达。Planning output 的 `Attempt.ActionName` 和 dispatcher `ActionExecutors` 同步修正；完整 error cause 统一 `%w`。这批变化故意不保留 alias、旧 tag 或双读：Process Snapshot 升为 v4、TreeSnapshot 升为 v2、child/framework-effect protocol 升为 v2、Planning ExecutionState 升为 v2，旧格式明确不属于当前合同。

P9-02 依据 ADR-A2-055 独立复核后发现 Baseline 4 只冻结了共同 snapshot 外层，没有冻结其中 opaque Strategy payload 的所有者协议；Framework Event payload 也由匿名 struct 生成而未进入 observation digest。Baseline 5 将这两个漏口治本关闭：Interaction、Planning、Workflow 分别拥有并冻结自己的 state/protocol，Kernel 只冻结共同 envelope；Framework Event payload 收敛为命名 wire，并使用 `process_status`、`termination_cause`、`step_status`、`effect_target`、`settlement_status`、`dropped_delta_count` 等准确字段。Process Snapshot 升为 v5、TreeSnapshot 升为 v3、Interaction state/protocol 升为 v4/v2、Planning state 升为 v3、Workflow state 升为 v2；旧版本与旧 tag 直接拒绝，七个 public digest 不变。

## 4. 明确不在基线中的能力

Baseline 5 暂不冻结 P10 应用迁移 API 或最终模块替换路径。`flow` 保持独立 in-process 库，不形成 Agent adapter API 或依赖；Workflow 吸收其显式拓扑、确定顺序和有界组合思想，但不强求复用或建立 adapter。未来编辑器图只能在更高层编译成已验证的 Workflow Definition，不能反向扩张 Kernel 或恢复 wire。
