# Lyra Runtime 合同基线

> 状态：P0 Baseline 0
>
> 基线日期：2026-08-08
>
> 适用范围：Runtime Protocol 制品、持久化 shape、Agent2 消费边界和重构期间的内部防腐合同

本文只记录可被机器比较的边界事实和版本。它不是向旧消费者承诺兼容；仓库允许 breaking change，但任何变化必须显式、一次性、可验收。

## 1. 基线含义

Runtime 是应用而不是公开 Go library。`internal` exported identifiers 主要服务模块内组合，不构成外部兼容 API。因此本基线不冻结全部内部 `go doc`，而冻结四类真实合同：

1. 外部 Runtime Protocol 和生成制品；
2. SQLite/artifact/checkpoint 等持久 shape；
3. Application 与 Agent adapter 的防腐边界；
4. Clean Architecture import DAG 和外部 SDK isolation。

任何基线变化必须：

- 有对应 ADR 或已授权阶段；
- 同批更新 owner codec/schema/GoDoc/tests；
- 直接替换旧 shape，不保留 alias、双读或兼容 shim；
- 更新本文件和自动守卫；
- 运行该 owner 的 strict round-trip、malformed input、integration 和 consumer tests。

Digest 只用于发现未审计漂移，不能替代语义测试。

当前 Runtime module 使用 Go `1.26.5`。重构代码使用该版本已经提供的标准库和测试能力，不引入为旧 Go 版本服务的兼容写法；Go 版本随仓库统一升级时同步更新本基线。

## 2. Runtime Protocol Baseline 0

机器真相源位于 [`../contract`](../contract)：

| 制品 | SHA-256 |
|---|---|
| `contract/manifest.json` | `0c12c0bd4964b5742043c18b5a403487812d201e573e99367a830487e63fc4fd` |
| `contract/openrpc.json` | `e460299ebcbcbd404e8e790ebfd5ad328a6bd5a7266d5340d27cc17549628a5a` |
| `contract/schema.json` | `4b40da69aa355e3bdb28795d3db49fcd3824dd67931c63d5bbf750ee1814ce7a` |

TypeScript generated files 是派生制品，不单独定义语义。它们必须由同一个 contract generator 产生且 diff-free；当前前端/TUI/CLI 是否已经消费最新 shape，由 P10/P12 的 consumer handoff 记录，不通过兼容字段掩盖。

人读语义 owner：

- [`API.md`](API.md)：业务方法、Run/Item/Event 语义与跨方法不变量；
- [`TRANSPORT.md`](TRANSPORT.md)：HTTP/SSE 与 in-process binding、流、重放和安全；
- [`AUX_API.md`](AUX_API.md)：VCS、MCP、审批等旁路能力。

本文件不复制 method、field、error 或 example catalog。

## 3. 持久化 Baseline 0

### 3.1 SQLite

- 当前 `schemaEpoch = 58`；
- 一个 build 只接受一个精确 epoch；
- 没有运行时 migration chain、dual schema read 或 compatibility column；
- 重构产生 shape 变化时直接提升 epoch，并同步 fresh-schema tests、store codec、contract expectation 和本基线。

### 3.2 Executor checkpoint

当前 checkpoint 的产品语义已经是 Host envelope + executor payload，但 payload 仍属于旧 Agent Framework。

目标合同：

- Application owns checkpoint identity、BuildID、Session/Run identity、model selection、limits、capabilities、accounting 和 child Run binding；
- `adapter/agentexec` owns Agent2 TreeSnapshot encode/decode/restore；
- payload 对 Application、Domain、Delivery、SQLite 和 Protocol 不透明；
- root Process tree 是 executor payload 的不可拆分恢复单位；
- checkpoint replacement 只能推进 frozen identity/limits 和 monotonic usage；
- terminalization 与 checkpoint deletion 由 Application write-set 原子决定。

目标 checkpoint wire version 在 P6 真实纵切证明后冻结。P0 不提前发明 version、字段或 codec。

### 3.3 Artifact 与 Transcript

Artifact、Transcript Item 和 ToolCall timing 的当前机器 shape 仍由 Runtime contract/store codec 拥有。P10 若因 Run/Segment/Interrupt 词汇发生 breaking change，直接产生唯一新 version；不从旧 artifact 猜测缺失事实。

## 4. Agent2 消费 Baseline

Runtime 迁移使用 Agent2 [`API_BASELINE.md`](../../../agent2/doc/API_BASELINE.md) 的 Baseline 9：

- root Kernel、Interaction、Planning、Planning/GOAP、Workflow、OTel、Platform 七个 public package 已冻结；
- Process Snapshot v6、TreeSnapshot v4；
- Interaction state/protocol v5/v3；
- WaitingSubtreeCancellationPlan、context-aware ProcessAdmitter、ModelInvocation/ToolInvocation、DelegateChildKey、DeferredTools/AdvertiseTools 已存在；
- Agent2 Event 是 Framework 已发生事实，Delta 是 best-effort 临时输出；
- Strategy payload 和 TreeSnapshot private state 对 Runtime 不透明。

Runtime 只把 Agent2 public API 当合同。旧 `agent`、Agent2 tests/private wire、当前 `agentexec` API 都不是新实现兼容基线。

Baseline 9 对 root Interaction、ordinary model/tool、waiting TreeSnapshot/restore 和 steer 已足够，但当前有两个 P7 前置缺口，以及一个明确不启用的可选合同：

- child `ProcessAdmitter` 成功后仍可能在 Definition.Start/capture/restore/register 失败，Framework 没有按 prospective identity 发布 conclusive aborted outcome；durable child admission 不得在该缺口关闭前启用；
- `WaitingSubtreeCancellationPlan` 返回后不再冻结 source tree，不能覆盖 Application transaction；P7 需要 Agent2 one-shot prepared change 在 Apply/Discard 前保持同一 safe cut；
- `PreparedStepAcknowledger` 只回调单 Process Snapshot，Runtime 初版不启用。durable recovery baseline 只有已提交 quiescent complete-tree checkpoint。

这些需求必须以 Runtime 真实 consumer tests 证明后，在 Agent2 中以 Framework-neutral ADR/API baseline 实现；不得把 Run、Store、transaction、产品 ID 或 private tree wire带入 Agent2。

### 4.1 允许的 import 边界

目标 production allowlist：

```text
internal/adapter/agentexec/** -> agent2, agent2/interaction
```

只有真实接入 Planning/Workflow/OTel/Platform 时，才分别通过 ADR 增加精确 package edge。默认禁止：

```text
internal/domain/**      -> agent2/**
internal/application/** -> agent2/**
internal/delivery/**    -> agent2/**
internal/infra/**       -> agent2/**
internal/adapter/toolset/** -> agent2/**
```

迁移期间旧 `agent` allowlist 只能单调缩小；P11 归零后删除整个例外。

## 5. Application/Agent 防腐基线

### 5.1 Application 可以表达

- Run execution start/observe/cancel；
- opaque executor root/member identity；
- opaque checkpoint payload 和 Host expectation；
- product Interrupt/answer、SteerRun；
- child Run admission facts；
- waiting subtree 的应用事务输入/结果；
- executor lifecycle facts 和稳定 product outcome。

### 5.2 Application 不得表达

- Agent2 `Process`、`Execution`、`Deployment`、`Signal`、`Effect`、`WaitID` concrete types；
- `TreeSnapshot` field、ExecutionState payload、Interaction phase 或 mailbox；
- arbitrary Signal submission；
- model/tool lifecycle 从 Delta 推断；
- Agent2 Engine/Dispatcher/Resolver handle；
- arbitrary EffectID/Settlement/ResolveEffect endpoint；
- Framework Store、transaction 或 product metadata extension。

Unknown Effect 的产品合同是 live/recovery 一致的 fail closed：Application/Delivery 不得到 Settlement payload 构造权；agentexec 只向 Application 投影 indeterminate executor fact/identity。RunLost write-set 提交前 Process 保持 unknown wait，提交后才 Kill/release。

P3 只通过 fake-backed Application consumer 建立最小 root 候选；P4–P7 的真实 Agent2 consumers 可以按 consumer-discovered interface 原则直接修订并扩展它。P8 production cutover 前才冻结精确内部 port shape。在此之前，本节只冻结语义边界，不冻结当前 `ExecutionControl` 方法或候选新名称。

## 6. Clean Architecture 边界基线

目标允许边：

```text
domain      -> stdlib + approved stable values/pure domain strategy contracts
application -> domain + consumer-owned ports + approved observation API
infra       -> domain immutable values + technical SDKs/mechanisms
adapter     -> application + domain + infra + capability SDKs
delivery    -> application + domain projection values + protocol/transport
bootstrap   -> config + application + adapter + infra + delivery
cmd         -> bootstrap + config
```

目标禁止边：

```text
domain      -/> application|adapter|infra|delivery|bootstrap
application -/> adapter|infra|delivery|bootstrap|Agent2|driver SDKs
infra       -/> application|adapter|delivery|bootstrap
adapter     -/> delivery|bootstrap
delivery    -/> adapter|infra|bootstrap|Agent2
all rings   -/> bootstrap composition objects
```

同一 ring 内的 package 仍必须形成 DAG；不能用同层身份为循环或 god package 辩护。

## 7. 自动守卫计划

P1–P10 应逐步建立：

- protocol artifact digest/drift test；
- SQLite schema epoch 和 prior-version rejection test；
- checkpoint envelope strict codec、size、copy、round-trip 和 prior-version rejection；
- exact Agent2 import allowlist；
- old Agent import monotonic-non-increase/zero guard；
- target package DAG；
- Domain/Application/Delivery external SDK denylist；
- Agent2 type/name leakage AST guard；
- no `component/common/core/utils` package guard；
- no alias/dual codec/legacy path guard；
- exported contract GoDoc/parameter/error wrapping guard where the contract is intentionally frozen。

## 8. 不在 Baseline 0 中

- 尚未由 P4–P7 真实 consumers 证明、并由 P8 cutover 冻结的 executor port 方法和参数；
- Runtime 对 Agent2 Platform 的接入；
- 前端/TUI/CLI 新 consumer API；
- Delivery `server`/`dispatch` 保持现名；未由真实职责变化证明时不做目录改名；
- 未由使用图裁决的 `component` 最终位置；
- 未来数据库 epoch、artifact version 或 checkpoint wire version。

这些内容不能以 placeholder、预留字段或空接口提前进入代码；真实阶段完成后再冻结。
