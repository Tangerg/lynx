# Agent Framework 公共合同基线

> 状态：Baseline 1 已冻结
> 冻结日期：2026-08-06
> 适用范围：`agent2` 根 package、`agent2/interaction`、Process Snapshot v3、TreeSnapshot v1

本文只记录已经由 P3 真实 Interaction、P4 child composition、三个独立 command consumer 和恢复合同共同证明的首个公共合同基线。目标架构、ADR、工程标准和实施进度仍由各自文档拥有；这里不复制它们。

## 1. 基线的含义

Baseline 1 不是兼容承诺或发布版本。仓库仍允许 breaking change，但任何公共名称、参数名、签名、GoDoc、sentinel error、Process snapshot wire 或 tree snapshot wire 的变化都必须是显式设计决策：

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

## 3. 自动守卫

`baseline_test.go` 对两个公共 package 的完整 `go doc -all` 输出做 SHA-256 校验，因此 exported identifier、参数名、字段、签名和 GoDoc 的任何漂移都会失败。Baseline 1 digest：

- root kernel：`3a53aee10161912baf58b506bc7d3b24f47c09d3e274e910034a422109ac0fd8`
- interaction：`6861bf74171fb6ece89bdda06627ef6cb06ab3e7353c68f95ebd4c3ce4d91e26`

同一测试对 Process Snapshot v3 与 TreeSnapshot v1 的 schema version、字段名、JSON tag、字段类型和嵌套 wire shape 做独立校验。Baseline 1 wire digest：

- snapshot/tree wire：`6f4a919ed0c681e9fb021f5571de0cdaf4e97d0cad8e4170fede3453a31c0c9d`

Digest 只用于发现未审计漂移，不替代 strict codec、round-trip、malformed input、restore、fuzz 或 consumer behavior tests。

## 4. 明确不在基线中的能力

Baseline 1 不提前冻结尚未实施的 Planning/GOAP、Workflow、Platform catalog/routing、OTel decorator 或应用迁移 API。它们必须继续通过后续阶段的真实实现与消费者证明，不能为了“保持 baseline”塞入当前窄腰或用预留字段占位。
