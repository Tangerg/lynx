# Agent Framework 公共合同基线

> 状态：Baseline 3 已冻结
> 冻结日期：2026-08-06
> 适用范围：`agent2` 根 package、`agent2/interaction`、`agent2/planning`、`agent2/planning/goap`、`agent2/workflow`、Process Snapshot v3、TreeSnapshot v1

本文只记录已经由 P3 真实 Interaction、P4 child composition、四个独立 command consumer、P5 真实 Planning/GOAP、P6 managed Workflow 与恢复合同共同证明的公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

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

`interaction` package 冻结为一个原生 Strategy：Definition 拥有 WorkingContext、模型/Tool 状态机与 exact managed Delegate，Dispatcher 只拥有模型和普通 Tool I/O。Delegate 将目标 Descriptor Input 暴露为模型 Tool schema，但只能由 Execution 经 Framework `StartChild`/`WaitForChildren` 推进；HITL/steer/stream/tool concurrency 仍通过根窄腰表达。它不拥有 Process lifecycle、产品 history、Store、UI 或 approval policy。

`planning` package 冻结为另一个原生 Strategy：Goal、Condition、Truth、WorldState、Action、Plan 和 Planning outcome 全部由 Planning 拥有；Planner 只做无副作用搜索，Observer/ActionExecutor 只存在于 dispatcher 边界。Execution 每次只执行 Plan 的第一个 Action，随后重新观察、确认预测效果并重新规划。dispatcher Action 与 child Process Action 共用同一预测模型，但不共享执行 capability；unreachable/stuck 是 Planning Output，不是共同 Process 状态。

`planning/goap` 只实现确定、有界的 uniform-cost search。HTN、Utility、Reactive、registry、默认 Planner 和 Planning telemetry 均不在基线中。

`workflow` package 冻结为 managed child Process 的确定性有序编排 Strategy。Definition 只接受 sealed `Transform`、`Call`、`Switch`、`Fork`、`Map`、`Loop` Stage；泛型只在构造边缘建立 schema，Execution 仍通过 erased 根窄腰推进。Call、被选择 case、branch、item 和 iteration 都是 exact child Process。Fork/Map 的 `WindowSize` 精确表示固定执行窗口：整窗结算后才开下一窗，不承诺滑动补位。Workflow 不拥有 dispatcher I/O、Graph/Registry、Store/Journal、第二 scheduler 或 `flow` adapter。

## 3. 自动守卫

`baseline_test.go` 对五个公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败。Baseline 3 digest：

- root kernel：`52a43f16c1b71a099a660ddde520c345476de124855f55c65221f46911486146`
- interaction：`086f0c7c0897ddb8a851c9ec45266ee6b97eddfb9c29f495c132f074109e4ff8`
- planning：`bb3a3fee5315afba3cc1f70ecc0486b4b91f88d4d4160aa93bf896b09ffc28a1`
- planning/goap：`da348e298e6976318b317873b44ec60829020fdea82947fae4bbc8e0d865b419`
- workflow：`0493f8f7ae6e4cc5a3190735c5d02952ec0e0fdb230794bbb01735b8ecfae055`

同一测试对 Process Snapshot v3 与 TreeSnapshot v1 的 schema version、字段名、JSON tag、字段类型和嵌套 wire shape 做独立校验。Baseline 3 沿用的 wire digest：

- snapshot/tree wire：`6f4a919ed0c681e9fb021f5571de0cdaf4e97d0cad8e4170fede3453a31c0c9d`

Digest 只用于发现未审计漂移，不替代 strict codec、round-trip、malformed input、restore、fuzz 或 consumer behavior tests。

Baseline 2 对根 Kernel 的唯一公开修订是 `ChildWaitOpened.Spec`：Planning 的真实 child Action 证明 Strategy 必须校验 Engine 确认的完整 child-wait request，而不能只相信 WaitID。Baseline 3 最初加入 Workflow；P7-02 依据 ADR-A2-040 对 Interaction 做了一次显式审计修订：新增 `Delegate`/`DelegateConfig`/`ErrInvalidDelegate`，`DefinitionConfig` 新增 `Delegates`，`NewDispatcher` 必须接收 exact `*Definition` 以冻结同一模型 manifest。Interaction 私有 ExecutionState 从 v1 直接升级为 v2，不双读旧状态。根公开 API、Process Snapshot v3、TreeSnapshot v1 及其 wire digest 均未变化。

## 4. 明确不在基线中的能力

Baseline 3 不冻结 Platform catalog/routing、OTel decorator、应用迁移 API 或 P7-03 之后尚未实现的组合 helper。`flow` 保持独立 in-process 库，不形成 Agent adapter API 或依赖；未来编辑器图只能在更高层编译成已验证的 Workflow Definition，不能反向扩张 Kernel 或恢复 wire。
