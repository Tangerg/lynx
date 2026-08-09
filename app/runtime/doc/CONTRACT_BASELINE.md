# Lyra Runtime 合同基线

> 状态：P7 Managed Delegate Tree Baseline 6
>
> 基线日期：2026-08-09
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

- 当前 `schemaEpoch = 62`；
- executor checkpoint 与 pending interrupt 的技术身份列为 `root_member_id`，continuation/input-request binding JSON 使用 `memberId`/`requestId`；
- `model_invocations` 与 `tool_invocations` 是 operational attempt journals，只保存 exact Run/Segment/call identity、state 与 started/finished time；semantic assistant final、Tool result 和 usage 仍只由 Transcript/Run owners 保存；
- `interrupts.state` 只有 `open`/`resuming`：`open` 不得携带 answer/claimedAt，`resuming` 必须携带两者；普通列表/读取只返回 `open`，continuation opening 必须在事务内证明 exact root 的 `resuming` claim；
- 下一 quiescent barrier 只能由相同 Session/executor/root-member owner 替换 `resuming` row；terminal 与 recovery write-set 删除该 row。不存在 open row overwrite、answer rollback、dual state codec 或兼容列；
- Tool start 不占用 Transcript insertion order；同一 model Tool batch 的 completed Items 与 invocation terminals 按模型声明位置形成一个 canonical write-set；
- 一个 build 只接受一个精确 epoch；
- 没有运行时 migration chain、dual schema read 或 compatibility column；
- 重构产生 shape 变化时直接提升 epoch，并同步 fresh-schema tests、store codec、contract expectation 和本基线。

### 3.2 Executor checkpoint

当前 checkpoint 的产品语义是 Host envelope + opaque Agent2 complete-tree payload。生产 Bootstrap 只保存 Agent2 public TreeSnapshot v4 JSON，由 `adapter/agentexec` 唯一解释；Application/Store 不分支解析。

目标合同：

- Application owns checkpoint identity、BuildID、Session/Run identity、model selection、limits、capabilities、accounting 和 child Run binding；
- `adapter/agentexec` owns Agent2 TreeSnapshot encode/decode/restore；
- payload 对 Application、Domain、Delivery、SQLite 和 Protocol 不透明；
- root Process tree 是 executor payload 的不可拆分恢复单位；
- checkpoint replacement 只能推进 frozen identity/limits 和 monotonic usage；
- terminalization 与 checkpoint deletion 由 Application write-set 原子决定。

P7 延续的 native payload baseline 是 Agent2 TreeSnapshot v4 本身，不再包一层 Runtime 自创 payload version。Agent2 public parser 校验 snapshot version/shape，exact DeploymentRef 校验策略实现与配置，Host BuildID 校验当前二进制/adapter expectation；任一不一致都 fail closed。Host envelope 的技术 codec 仍由 Runtime 当前唯一 SQLite epoch 拥有。

### 3.3 Artifact 与 Transcript

Artifact、Transcript Item 和 ToolCall timing 的当前机器 shape 仍由 Runtime contract/store codec 拥有。P10 若因 Run/Segment/Interrupt 词汇发生 breaking change，直接产生唯一新 version；不从旧 artifact 猜测缺失事实。

## 4. Agent2 消费 Baseline

Runtime 使用 Agent2 [`API_BASELINE.md`](../../../agent2/doc/API_BASELINE.md) 的 Baseline 14。P8 已把 P4–P7 验证的 root start/result、authoritative model/tool、waiting/restore/answer/steer、managed Delegate child 和 prepared waiting-subtree合同切为生产 Bootstrap 唯一 owner：

- root Kernel、Interaction、Planning、Planning/GOAP、Workflow、OTel、Platform 七个 public package 已冻结；
- Process Snapshot v6、TreeSnapshot v4；
- Interaction state/protocol v5/v3；
- context-aware ProcessAdmitter、conclusive ProcessStartOutcome、ModelInvocation/ToolInvocation、DelegateChildKey、ActiveDelegateChild inspector、DeferredTools/AdvertiseTools 与 contextless PreparedWaitingSubtreeCancellation Apply 已存在；
- Agent2 Event 是 Framework 已发生事实，Delta 是 best-effort 临时输出；
- Strategy payload 和 TreeSnapshot private state 对 Runtime 不透明。

Runtime 只把 Agent2 public API 当合同。旧 `agent`、Agent2 tests/private wire、当前 `agentexec` API 都不是新实现兼容基线。

P7 的两个前置缺口已经由真实 Runtime consumer 在 Agent2 中以 Framework-neutral 合同关闭：accepted admission 通过 prospective identity 的 started/aborted outcome闭合；waiting subtree 通过 one-shot prepared capability 持有同一 safe cut，全部 fallible staging 位于 Prepare，durable commit 后只调用 contextless Apply。Run、Store、transaction、产品 ID 和 private tree wire均未进入 Agent2。

`PreparedStepAcknowledger` 仍只回调单 Process Snapshot，Runtime 初版不启用。durable recovery baseline 只有已提交 quiescent complete-tree checkpoint；active-step crash 不伪装为可恢复。

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

旧 `agent` import 已永久禁止；不存在迁移 allowlist。

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

P8 已冻结生产 executor port：`RootExecutionStarter` 负责 validate/stage/begin，`ExecutionObserver` 负责只读事实流，`RunningRootCancellationRequester` 只提交 Framework cancel request，`ExecutionReleaser` 只负责 resource lifecycle；`WorkingContextComposer` 在 Application 边界组装完整 fresh-root context。Application opening durable 后 Begin 才 Start Process；cancel intent durable 后请求停止，pump 继续观察到确定终态才 release。

P5 已验证 authoritative model/tool candidate：executor producer 只能通过同一有序 observation stream 提交 Application-owned closed fact 并等待 receipt；它不取得 Store、transaction 或 reducer。Application Run pump 在 speculative reducer 上计算 write-set，只有 persistence 全部成功才替换 live reducer 并完成 receipt。model/tool post-call receipt failure 必须返回 Agent2 Dispatcher 形成 unknown；pre-call failure禁止外呼。Toolset 的唯一 visibility value 是 framework-neutral `toolset.Manifest`，通用 Toolset 对 Agent2 零 import。

P6 已验证 continuation candidate：`WaitingExecutionContinuer.StageContinuation` 只 stage 一棵 exact live waiting tree，或按 opaque TreeSnapshot + exact Deployment/BuildID/Host scope 恢复；它不读取 Conversation，也不重算 WorkingContext。Application 先原子记录 exact answers、隐藏 interrupt row 并删除旧 checkpoint，再 stage/restore；next-Segment opening transaction 必须证明 durable `resuming` claim，成功后 `BeginContinuation` 才投递 WaitID-addressed semantic Signal。claim 后到下一 quiescent checkpoint 前没有 fallback recovery point，crash/boot recovery 一律 `RunLost`。

Product Interrupt/prompt/answer 使用 framework-neutral strict codec；native `interactioninput` ACL 是唯一把它映射到 Agent2 pending-input/Signal 的 owner。旧 private suspension adapter 已删除。真实 Runtime `ask_user`、interactive approval、deferred advertisement restore 与 steer 均走唯一生产路径。

P7 已验证 child/subtree candidate：Delegate ToolCall authoritative commit 先于不可见 child start reservation；Agent2 conclusive started 后才公开 child Run，aborted 只闭合 reservation。多 child、嵌套 child与乱序 sibling completion 使用稳定 parent/model-call/tool-index 因果顺序；恢复归因只调用 Interaction owner 的 typed inspector。waiting child cancellation 执行 prepare → application transaction → contextless Apply/Discard；移除最后边界时，Apply 只安装 resulting state，独立 Continue 才激活已提交 Segment。Apply 异常先释放旧 owner并由 `WaitingExecutionRestorer` 从 committed resulting checkpoint精确恢复，恢复失败才 RunLost。

P8 已冻结这些内部消费端口及防腐语义：Application 单写者、operational journal 与 semantic Transcript 分离、final 独立于 Delta、并发 Tool canonical prefix 原子提交、unknown 在 release 前 durable `RunLost` 收口、answer claim → stage/restore → durable opening → semantic Signal，以及 Delegate reservation → conclusive start → public child Run 的唯一顺序。

Fresh root input 的防腐合同是 Application `WorkingContextComposer` 读取 Host Conversation 并追加当前 user message，再组装 Knowledge、Plan、Memory 与 hooks，形成完整 `WorkingContext` seed；agentexec 不读取产品 Store。成功 assistant final 由 Agent2 Result 投影 `AssistantMessageCompleted`，不从 Delta 拼接。

Application executor tree identity 统一为 `ExecutorMember`/`MemberID`。Framework `ProcessID` 只能由 execution adapter 在边界内映射，不能重新进入 Application field、port 参数、持久化 technical field 或 Runtime Protocol。

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

## 7. 自动守卫

P1 已建立：

- production target package DAG，以及会被稳定拒绝的 Delivery → Adapter 反例 fixture；
- Agent2 importing leaf 与 imported public package 的双重 exact allowlist；
- 旧 Agent import 的逐文件、逐数量、逐 owner 和删除阶段台账；
- Domain、Application 与 Delivery 既有 external SDK denylist；
- context-based Domain I/O port、旧 private snapshot decoder 和旧 lifecycle owner 的精确 Temporary 台账；
- compatibility/legacy/versioned source directory 禁令。

机器 owner 是 `internal/arch/target_architecture_test.go` 与 `internal/arch/temporary_architecture_test.go`；本文件不复制易漂移的逐文件集合。

P2–P10 继续逐步建立：

- protocol artifact digest/drift test；
- SQLite schema epoch 和 prior-version rejection test；
- checkpoint envelope strict codec、size、copy、round-trip 和 prior-version rejection（P6 已覆盖 native TreeSnapshot parser、copy、corrupt/wrong-build/deployment；P8 随 production owner 收口剩余 envelope guard）；
- Agent2 type/name leakage AST guard；
- no `component/common/core/utils` package guard（P9 已建立准确 shared-capability purity allowlist）；
- no alias/dual codec/legacy path guard；
- exported contract GoDoc/parameter/error wrapping guard where the contract is intentionally frozen。

## 8. 不在 Baseline 0 中

- 尚未由 P7 child/subtree real consumer 证明、并由 P8 cutover 冻结的完整 executor port 方法和参数；
- Runtime 对 Agent2 Platform 的接入；
- 前端/TUI/CLI 新 consumer API；
- Delivery `server`/`dispatch` 保持现名；未由真实职责变化证明时不做目录改名；
- 未来数据库 epoch、artifact version 或 Agent2 TreeSnapshot version。

这些内容不能以 placeholder、预留字段或空接口提前进入代码；真实阶段完成后再冻结。
