# Agent Framework 公共合同基线

> 状态：Baseline 3 已冻结
> 冻结日期：2026-08-06
> 适用范围：`agent2` 根 package、`agent2/interaction`、`agent2/planning`、`agent2/planning/goap`、`agent2/workflow`、Process Snapshot v3、TreeSnapshot v1

本文只记录已经由 P3 真实 Interaction、P4 child composition、七个独立 command consumer、P5 真实 Planning/GOAP、P6 managed Workflow 与恢复合同共同证明的公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

## 1. 基线的含义

Baseline 3 不是兼容承诺或发布版本。仓库仍允许 breaking change，但任何公共名称、参数名、签名、GoDoc、sentinel error、Process snapshot wire 或 tree snapshot wire 的变化都必须是显式设计决策：

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
- Event/Delta：权威已发生事实与 best-effort 临时增量。
- Typed/EncodeInput/DecodeOutput：只存在于类型擦除边缘的人体工程学 adapter。

`interaction` package 冻结为一个原生 Strategy：Definition 拥有 WorkingContext、模型/Tool 状态机、exact managed Delegate、typed Delegate artifacts 与可选 pure completion validator，Dispatcher 只拥有模型和普通 Tool I/O。Delegate 将目标 Descriptor Input 暴露为模型 Tool schema，但只能由 Execution 经 Framework `StartChild`/`WaitForChildren` 推进；成功 child Output 经 exact schema 复验后进入 immutable validator view，validator 读取防御性复制的当前 WorkingContext/candidate/artifacts，拒绝候选时以有界 feedback 进入下一模型轮次；HITL/steer/stream/tool concurrency 仍通过根窄腰表达。它不拥有 Process lifecycle、产品 history、artifact store、UI 或 approval policy。

`planning` package 冻结为另一个原生 Strategy：Goal、Condition、Truth、WorldState、Action、Plan 和 Planning outcome 全部由 Planning 拥有；Planner 只做无副作用搜索，Observer/ActionExecutor 只存在于 dispatcher 边界。Execution 每次只执行 Plan 的第一个 Action，随后重新观察、确认预测效果并重新规划。dispatcher Action 与 child Process Action 共用同一预测模型，但不共享执行 capability；unreachable/stuck 是 Planning Output，不是共同 Process 状态。

`planning/goap` 只实现确定、有界的 uniform-cost search。HTN、Utility、Reactive、registry、默认 Planner 和 Planning telemetry 均不在基线中。

`workflow` package 冻结为 managed child Process 的确定性有序编排 Strategy。Definition 只接受 sealed `Transform`、`Call`、`Switch`、`Fork`、`Map`、`Loop` Stage；泛型只在构造边缘建立 schema，Execution 仍通过 erased 根窄腰推进。Call、被选择 case、branch、item 和 iteration 都是 exact child Process。Fork/Map 的 `WindowSize` 精确表示固定执行窗口：整窗结算后才开下一窗，不承诺滑动补位。Workflow 不拥有 dispatcher I/O、Graph/Registry、Store/Journal、第二 scheduler 或 `flow` adapter。

## 3. 自动守卫

`baseline_test.go` 对五个公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败。Baseline 3 digest：

- root kernel：`6d113e12dcdc2f87e0bd064d8ad25da4342b8d88b3144ca55db9a0868818b1b0`
- interaction：`9678f94265b227e7d085cc18a264ad3be4cac98709d94638f47c9ee7960e3fee`
- planning：`bb3a3fee5315afba3cc1f70ecc0486b4b91f88d4d4160aa93bf896b09ffc28a1`
- planning/goap：`da348e298e6976318b317873b44ec60829020fdea82947fae4bbc8e0d865b419`
- workflow：`0493f8f7ae6e4cc5a3190735c5d02952ec0e0fdb230794bbb01735b8ecfae055`

同一测试对 Process Snapshot v3 与 TreeSnapshot v1 的 schema version、字段名、JSON tag、字段类型和嵌套 wire shape 做独立校验。Baseline 3 沿用的 wire digest：

- snapshot/tree wire：`6f4a919ed0c681e9fb021f5571de0cdaf4e97d0cad8e4170fede3453a31c0c9d`

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

## 4. 明确不在基线中的能力

Baseline 3 不冻结 P8 Platform catalog/routing/OTel decorator、P10 应用迁移 API 或最终模块替换路径。`flow` 保持独立 in-process 库，不形成 Agent adapter API 或依赖；未来编辑器图只能在更高层编译成已验证的 Workflow Definition，不能反向扩张 Kernel 或恢复 wire。
