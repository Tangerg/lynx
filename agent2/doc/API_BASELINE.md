# Agent Framework 公共合同基线

> 状态：Baseline 2 已冻结
> 冻结日期：2026-08-06
> 适用范围：`agent2` 根 package、`agent2/interaction`、`agent2/planning`、`agent2/planning/goap`、Process Snapshot v3、TreeSnapshot v1

本文只记录已经由 P3 真实 Interaction、P4 child composition、三个独立 command consumer、P5 真实 Planning/GOAP 与恢复合同共同证明的公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

## 1. 基线的含义

Baseline 2 不是兼容承诺或发布版本。仓库仍允许 breaking change，但任何公共名称、参数名、签名、GoDoc、sentinel error、Process snapshot wire 或 tree snapshot wire 的变化都必须是显式设计决策：

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

`interaction` package 冻结为一个原生 Strategy：Definition 拥有 WorkingContext 与模型/Tool 状态机，Dispatcher 拥有 I/O，HITL/steer/stream/tool concurrency 都通过根窄腰表达。它不拥有 Process lifecycle、产品 history、Store、UI 或 approval policy。

`planning` package 冻结为另一个原生 Strategy：Goal、Condition、Truth、WorldState、Action、Plan 和 Planning outcome 全部由 Planning 拥有；Planner 只做无副作用搜索，Observer/ActionExecutor 只存在于 dispatcher 边界。Execution 每次只执行 Plan 的第一个 Action，随后重新观察、确认预测效果并重新规划。dispatcher Action 与 child Process Action 共用同一预测模型，但不共享执行 capability；unreachable/stuck 是 Planning Output，不是共同 Process 状态。

`planning/goap` 只实现确定、有界的 uniform-cost search。HTN、Utility、Reactive、registry、默认 Planner 和 Planning telemetry 均不在基线中。

## 3. 自动守卫

`baseline_test.go` 对四个公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败。Baseline 2 digest：

- root kernel：`52a43f16c1b71a099a660ddde520c345476de124855f55c65221f46911486146`
- interaction：`6861bf74171fb6ece89bdda06627ef6cb06ab3e7353c68f95ebd4c3ce4d91e26`
- planning：`bb3a3fee5315afba3cc1f70ecc0486b4b91f88d4d4160aa93bf896b09ffc28a1`
- planning/goap：`da348e298e6976318b317873b44ec60829020fdea82947fae4bbc8e0d865b419`

同一测试对 Process Snapshot v3 与 TreeSnapshot v1 的 schema version、字段名、JSON tag、字段类型和嵌套 wire shape 做独立校验。Baseline 2 沿用的 wire digest：

- snapshot/tree wire：`6f4a919ed0c681e9fb021f5571de0cdaf4e97d0cad8e4170fede3453a31c0c9d`

Digest 只用于发现未审计漂移，不替代 strict codec、round-trip、malformed input、restore、fuzz 或 consumer behavior tests。

Baseline 2 对根 Kernel 的唯一修订是 `ChildWaitOpened.Spec`：Planning 的真实 child Action 证明 Strategy 必须校验 Engine 确认的完整 child-wait request，而不能只相信 WaitID。它返回 defensive copy，没有改变 snapshot wire。

## 4. 明确不在基线中的能力

Baseline 2 不冻结 Workflow、Platform catalog/routing、OTel decorator 或应用迁移 API。P6 的 disposable consumer 已证明 Workflow 可以完全建立在现有 child Process 与 TreeSnapshot 窄腰上，因此不修改本基线；Workflow 自身的 Stage/Definition/state API 只有在正式实现、恢复合同和独立 consumer 通过后才进入后续 baseline。`flow` 保持独立 in-process 库，不形成 Agent adapter API 或依赖。
