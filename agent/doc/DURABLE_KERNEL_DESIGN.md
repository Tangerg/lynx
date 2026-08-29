# Agent durable kernel 设计

> 状态：已接受，实施中（见 ADR-A2-089）
> 范围：`scope/agent` 内核；Flame 是 harness 与首个 durable Host
> 日期：2026-08-29
> 读者：Scope/Flame 的实现者、评审者与 durable Host adapter 作者

## 1. 结论

`agent` 最适合被定义为：

> **一个以 root Process tree 为一致性单元、以单一提交线维护状态、以完整当前状态恢复、以 effect sandwich 隔离外部副作用、以显式 capability 约束权限的 Agent execution kernel。**

它不是 Erlang/OTP、Temporal 和 capability security 的机械乘积。三者只分别验证了三个方向：

- OTP 验证树形身份、消息驱动和故障域的价值；
- Temporal 与 pi v2 验证 durable program counter 和 intent → effect → settlement 的价值；
- capability security 验证显式授予、逐层衰减和执行前门禁的价值。

Scope 只吸收与自身场景匹配的语义，不复制它们的完整系统：

- 不引入通用 supervisor restart policy；
- 不引入 workflow history、activity registry 或 Store；
- 不声称字符串 capability 能替代 OS sandbox、凭据隔离或网络策略；
- 不把 session、model、tool、transcript、tenant、lease 和产品事务下沉到内核。

最终边界是：

```text
Flame / other harness
  product state · session · model/tool · store · transaction · lease · policy
                         │
                         │ stable inputs / durable acknowledgments / observations
                         ▼
Scope agent kernel
  tree ownership · deterministic reduction · effect protocol · recovery state
                         │
                         │ explicit EffectRequest + attenuated authority
                         ▼
Dispatcher / external world
```

本设计优先级固定为：

1. 语义正确；
2. 一个事实只有一个权威表示；
3. 普通用法保持简单；
4. 扩展发生在明确边缘；
5. 最后才是减少 API 数量和优化性能。

## 2. 设计约束

### 2.1 必须成立

1. **恢复不重复已确认的外部 Effect。**
2. **无法判断的 Effect 结果必须显式成为 Unknown。**
3. **同一 root tree 只有一个 Framework 状态提交顺序。**
4. **恢复后的新 writer 必须 fence 掉旧 writer。**
5. **父子 topology、mailbox、budget、capability 和 Effect 状态必须原子一致。**
6. **Definition/Execution 不读时钟、随机数、环境变量或全局可变状态。**
7. **Host 持久化失败不能被伪装成 Strategy 业务失败。**
8. **没有 durability 配置时，`EngineConfig{}` 仍是正确的内存执行配置。**
9. **Flame 可以把 Scope snapshot 与产品事实放进同一事务。**
10. **第三方 Host 可以用公开 conformance suite 验证协议。**

### 2.2 明确不做

- exactly-once 外部世界；
- 多 writer 共享一棵活树；
- 内核自带数据库、journal、lease、scheduler 集群或事务抽象；
- 自动恢复任意未声明幂等性的 Effect；
- 通用 hook/plugin 总线；
- 通用 actor framework；
- 自动重启失败的 Process；
- 把 observation 当作恢复源；
- 用 capability label 宣称进程级安全隔离；
- 在没有测量前引入增量 snapshot、batch commit 或并行 batch Effect。

### 2.3 骨架与 harness 的边界

Scope 是骨架，Flame 是 harness。这不是部署偶然，而是设计边界：

| Scope 必须拥有 | Flame / Host 必须拥有 |
|---|---|
| Process/tree identity 与 topology | Run、Session、Interaction 等产品 identity |
| Strategy state 与 mailbox | Transcript、模型上下文、工具产品投影 |
| EffectID、phase、Settlement、Unknown | model/tool 调用记录、usage、billing |
| TreeSnapshot schema 与恢复验证 | Store、事务、compare-and-swap（CAS）、lease、重试、告警 |
| tree mutation ordering | worker placement、boot scan、RunLost 策略 |
| capability attenuation 与 pre-dispatch gate | sandbox、credential、network/filesystem policy |
| Event/Delta 事实 | UI、遥测、搜索和产品事件 |

判断规则很简单：

> 没有这个概念，Scope 是否仍能安全恢复并继续执行？

- 如果不能，它属于内核协议；
- 如果能，但产品无法提供用户体验或运维，它属于 harness；
- 如果两边都想拥有，必须先消除双重真相，而不是加同步代码。

## 3. 核心不变量

### 3.1 一致性单元是 root tree

单个 Process snapshot 无法独立解释：

- parent/child relation；
- child wait registration；
- parent budget reservation；
- capability attenuation；
- child start settlement；
- termination propagation。

因此恢复单元不是 Process，而是完整 root tree：

```text
TreeSnapshot = complete current Framework state of one root tree
```

`ProcessSnapshot` 可以保留为诊断和 inspector value，但不能再作为 `Engine.Restore` 的输入。

### 3.2 状态只有一条权威提交线

每棵 root tree 有一个内部 `treeRuntime`。它是以下事实的唯一 owner：

- Process 状态和 topology；
- mailbox 与 signal cursor；
- current ExecutionState；
- Effect phase 与 Settlement；
- child waits、budget 和 capability；
- tree incarnation 与 last acknowledged head；
- control intent 和 termination bookkeeping。

所有公开 Process 操作都变成发给该 owner 的 command。不同 root tree 独立运行；同一 root tree 的 Framework commit 严格有序。

“单一提交线”不等于“所有工作都在一个 goroutine 同步执行”。详见 §4。

### 3.3 durable state 是完整当前状态

每个 acknowledged `TreeSnapshot` 都是完整、独立可解释的 program counter：

- 不需要读取上一份 snapshot 才能恢复；
- 不从缺失记录推断阶段；
- 不依赖 Event/Delta 重放；
- 不包含 Host revision、transaction ID 或产品状态。

这吸收了 pi v2 的关键启发，但不把 pi 的 entry/register/ledger storage model 带入 Scope。Host 可以把 opaque `TreeSnapshot` 保存到任何原子存储。

### 3.4 外部 Effect 使用 sandwich

对每个 Dispatcher Effect：

```text
planned
  │ CommitEffect(pending): durable intent
  ▼
pending
  │ Dispatcher.Dispatch: uncertain external world
  ▼
pending + observed result
  │ CommitEffect(settled): durable settlement
  ▼
settled
```

Unknown 不是异常旁路，而是一种已确认 settlement：

```text
pending ── dispatch error / invalid result ──> settled(Unknown)
settled(Unknown) ── explicit reconciliation ──> resolved(definite)
```

### 3.5 authority 必须显式流动

- root 只获得 Engine 配置允许的最大 capability set；
- child 只能获得 parent capability 的子集；
- Dispatcher Effect 声明所需 capability；
- Engine 在生成 durable intent 之前拒绝越权 Effect；
- Dispatcher 不从 context、全局 registry 或 payload 暗中获得额外授权。

这是一套 capability-oriented authorization，不是完整 object-capability runtime，更不是 sandbox。

## 4. 运行模型：提交线与工作面分离

### 4.1 `treeRuntime` 是提交者，不是计算瓶颈

目标内部结构：

```text
                           ┌─ Step job for Process A ─┐
commands ──> treeRuntime ──┼─ Dispatch job Effect B ─┼──> completions
                           └─ Step job for Process C ─┘
                                  │
                                  ▼
                         treeRuntime validates
                         and commits in one order
```

`treeRuntime` 只做有界的 Framework 工作：

- 校验 command 和 completion；
- 选择下一个可运行 Process；
- 创建 job 与 attempt token；
- 形成 prospective tree state；
- 调用 durable boundary；
- 成功后原子安装状态；
- 更新只读 view 和 observation sequence。

纯 Strategy 计算和外部 I/O 不占用 owner line：

- 同一 Process 最多一个 Step job；
- 不同 sibling 的 Step 可以并行；
- 不同 Process 的 Dispatcher 调用可以并行；
- 首版保留一个 Process 内 Effect batch 的既有 declaration-order 语义；
- completion 必须携带 tree incarnation、Process attempt token 和 EffectID；过期 completion 被丢弃。

这样既保留 actor/OTP 风格的单 owner，又不会让一个较慢但“有界”的目标导向行动规划（goal-oriented action planning，GOAP）Step 饿死整棵树的 sibling。

### 4.2 Step job 的所有权

Step job 临时独占一个 Process 的 `Execution` 实例：

1. owner 从 last stable ExecutionState 恢复或移交当前 isolated Execution；
2. job 调用 `Step`；
3. 校验 Transition；
4. 调用 `Execution.Snapshot`；
5. 把 candidate state、Transition 和 attempt token 返回 owner；
6. owner 再决定是否采用。

job 不能直接修改 mailbox、Process status、tree topology 或 durable head。

TreeSnapshot 不持久化 in-flight job。形成 snapshot 时，其他 Process 的 job 可以继续计算，但 snapshot 只记录它们的 last stable state。completion 在 callback 后重新校验 attempt token；进程崩溃时这些纯计算从 head 重算。

如果 job 被取消，或者返回时 attempt token 已过期：

- Transition 和 error 一律丢弃；
- 该 Execution 实例不再可信，直接丢弃；
- owner 从 last stable ExecutionState 重建；
- job 无权发布 Event、Effect 或状态。

### 4.3 `Execution.Step` 的 context

保留：

```go
Step(context.Context, []Signal) (Transition, error)
```

但 Runtime 传入的是自己构造的 cancellation-only context：

```go
stepCtx, cancel := context.WithCancel(context.Background())
```

合同为：

- 无 Value；
- 无 Deadline；
- 无 Cause；
- 只表达“本次候选计算已无资格提交”；
- Strategy 仍必须自身有界并响应 cancellation。

这样保留慢纯计算的逃生口，同时切断 ambient lookup、clock 和请求 deadline 三个非确定性注入面。

owner line 之外允许一个极窄的 interrupt plane：它只能调用当前 job 的 cancel function，不能修改任何 Framework state。真正的 Pause/Kill/Cancel 决议仍由 owner line 排序。

### 4.4 公平性与顺序

首版使用简单 round-robin runnable queue：

- 一个 Process 的 completion 只使它重新进入队尾；
- control command 优先于启动新的 Step；
- durable callback 在同一树内最多一个 in flight；
- 不相关 root tree 不共享 tree lock；
- 不承诺跨 Process 的全局 Event 顺序，只承诺每棵树和每个 Process 的 documented sequence。

暂不公开 scheduler policy。没有第二个真实调度策略前，导出接口只会制造伪扩展点。

## 5. Strategy 合同

### 5.1 Definition 与 Execution

保留当前两层：

```go
type Definition interface {
	Descriptor() Descriptor
	Start(Input) (Execution, error)
	Restore(ExecutionState) (Execution, error)
}

type Execution interface {
	Step(context.Context, []Signal) (Transition, error)
	Snapshot() (ExecutionState, error)
}
```

这是正确的扩展边缘：新 Agent behavior 只需实现这两个小接口，不需要理解 Engine、Store 或 Flame。

强制合同：

- `Definition` immutable，可并发用于不同 Process；
- `Start`/`Restore` 不执行外部 I/O；
- `Step` 是 current state + ordered Signal prefix 的确定性 reduction；
- 所有外部动作返回为 Effect；
- `Snapshot` 返回完整、自有、versioned state；
- 任一错误都使当前 Execution 实例不可复用；
- 同一 state 和 Signal 必须产生等价 Transition、candidate state 与 Effect identity。

### 5.2 Signal 必须是稳定输入

当前 `Signal.ReceivedAt()` 由 Engine 在接收时读取 wall clock。它没有生产消费者，却使同一 `SignalID` 在崩溃重投时产生不同 Strategy 输入，破坏确定性。

目标设计删除 Strategy-visible `Signal.ReceivedAt()` 及其 wire 字段：

```go
type Signal struct {
	// ID, optional WaitID, opaque payload
}
```

- 如果业务时间影响行为，Host 必须把稳定时间显式编码进 payload；
- 如果只需观测接收延迟，Event 自己携带 observation timestamp；
- 同一 `SignalRequest` 重投必须生成字节等价的 Signal。

这是比“每接收一个 Signal 都强制写 snapshot”更小、更可靠的修复。

### 5.3 确定性边界

以下输入不得从 Strategy 暗中读取：

- clock、random；
- environment 和 cwd；
- model/tool registry；
- request context values；
- global mutable singleton；
- Host product identity；
- goroutine completion order。

需要它们时，先由 Host/Dispatcher 形成显式 Effect Settlement 或 Signal，再进入 Step。

## 6. Effect 状态机

### 6.1 phase

每个 prepared batch 中的 Effect 具有明确 phase：

| Phase | 是否可能已派发 | 是否有 Settlement | 终止时是否 unresolved |
|---|---:|---:|---:|
| `planned` | 否 | 否 | 否，可丢弃 |
| `pending` | 是 | 否 | 是 |
| `settled` | 是 | 是，可能 Unknown | 仅 Unknown 是 |

`resolved` 是 durable boundary kind，不需要成为长期第四个 phase；成功决议后仍是 `settled`，只是 Settlement 从 Unknown 替换为 definite。

一个 batch 按 declaration order 推进：当前 Effect 变成 settled 后，下一个 planned Effect 才能进入 pending。这样不会把尚未尝试的不可重放 Effect 错报为 Unknown。

### 6.2 pending boundary

Runtime 完成这些动作后才能请求 pending commit：

1. Step 成功；
2. Transition、signal consumption、budget 和 capability 校验成功；
3. candidate ExecutionState 已捕获并可 Restore；
4. 整个 batch 的 EffectID 已稳定派生；
5. 当前 Effect 从 planned 变成 pending；
6. prospective full TreeSnapshot 已形成。

Host 返回 `nil` 前，不得调用 Dispatcher。

pending commit 失败意味着 Effect 确定未由该 Runtime 派发；它不进入 unresolved 集合。当前 candidate 被丢弃，tree 以 durability fault 停止。

### 6.3 dispatch 与 settlement

Dispatcher 只收到不可变 request：

```go
type Dispatcher interface {
	Dispatch(context.Context, EffectRequest, DeltaEmitter) (Settlement, error)
	ReplayPolicy(Effect) ReplayPolicy
}
```

规则：

- 返回的 Settlement 必须地址匹配 EffectID；
- panic、error、无效 Settlement 或 ID 不匹配都转换为 Unknown；
- Delta 是 best-effort observation，不影响 settlement；
- Dispatcher 不得保留 DeltaEmitter；
- Dispatcher 必须有界、并发安全，不创建 ownerless goroutine；
- context 不是授权载体，所有权限来自 request 与部署绑定。

Dispatcher 返回后，Runtime 形成 settled prospective TreeSnapshot。Host 在 settlement callback 中原子提交产品结果与 snapshot。成功后内核才应用 Settlement。

### 6.4 crash recovery

恢复遇到 pending Effect：

- `ReplayPolicySameIdentity`：仅使用原 EffectID 重派；
- `ReplayPolicyNever`：不派发，形成 Unknown Settlement；
- 无效或变化的 replay policy：contract fault；
- 任何重派结果仍必须走 settled commit。

ReplayPolicy 只表示“相同 EffectID 是相同逻辑外部操作”，不表示数据库 exactly-once，也不允许新 identity 重试旧操作。

### 6.5 Framework Effect

不是所有 Framework Effect 都需要两次 durable commit：

- wait ID、child wait registration 等纯内核动作可从最近 head 确定性重算；
- `StartChild` 跨越 `ProcessAdmitter`，其稳定 child ProcessID 由 parent ProcessID、Step sequence、batch index/EffectID 派生；恢复会以同一身份重投 admission；
- child started/aborted outcome 必须把 admission conclusion、parent Effect settlement、budget/topology 和 prospective tree 原子提交，见 §9；
- Framework Effect 不得访问其他外部世界。需要外部工作时必须改为 Dispatcher Effect。

这避免为纯 identity minting 支付无意义的双事务，同时不牺牲 child start 的幂等闭环。

## 7. durable program counter

### 7.1 为什么不能只提交 start 与 Effect

旧方案只有 activation、Process start 和 Effect 三类 callback，缺少一个真实消费场景：

- Process 可以在纯 Step 后进入 Waiting/Paused；
- 一个无 Dispatcher Effect 的 Process 可以纯计算后 Terminal；
- Flame 必须把 waiting tree checkpoint 与 Interrupt/Pending 产品事实原子提交；
- durable mode 又不能依赖一个释放冻结后才由 Host 随意保存的 `CaptureTree`。

因此 durable 协议必须包含第四类 runtime-driven boundary：`TreeCheckpoint`。

### 7.2 checkpoint 触发条件

Runtime 在以下 canonical safe cut 自动形成 checkpoint：

1. **Parked**：没有 Step/Dispatch/durability job in flight，没有 unresolved Effect，没有 runnable Process，所有非终态 Process 均为 Waiting 或 Paused；
2. **Terminal**：整棵树的 topology、child completion、budget 归还和终态 bookkeeping 已稳定。

相同 tree digest 不重复提交。checkpoint 是状态边界，不是定时 autosave。

纯 `Continue` 链不逐 Step 落盘：它没有外部副作用，可以从最近 acknowledged head 重算。若实际 trace 证明重算成本不可接受，再讨论显式 checkpoint Effect 或 measured interval；不能先把每个纯赋值变成 Host transaction。

### 7.3 Flame waiting boundary

目标流程：

1. Flame 发现 root 处于产品可寻址的 human-input wait；
2. Flame 通过 Process control 暂停仍在运行的 sibling；
3. treeRuntime 到达 Parked safe cut；
4. `CommitCheckpoint(Parked)` 得到完整 prospective TreeSnapshot；
5. Flame adapter 从 snapshot 派生 interruption projection；
6. 同一产品事务提交 checkpoint、Pending/Interrupt 和 Run waiting facts；
7. callback 返回 `nil`，tree 保持 waiting/paused。

callback 期间 tree owner 不推进该树。adapter 可以把 boundary 交给 application transaction coordinator 并有界等待，但不能 re-enter 该 tree。

这替代当前“poll status → CaptureTree → 稍后保存”的旁路。

### 7.4 durable input

Signal 和 control 是 Host → Kernel 输入，不是 Kernel → World Effect：

1. Host 先持久化输入及其顺序；
2. 用稳定 SignalID 或幂等 control intent 提交给 Process；
3. 如果在新 head 前崩溃，Host 从 input log 重投；
4. mailbox 以 SignalID 去重；
5. 进入 Effect 或 Parked/Terminal checkpoint 后，input consumption 成为新 durable program counter 的一部分。

Unknown reconciliation 同理：Host 先记录对账证据和 definite Settlement，再调用 `ResolveUnknownEffect`；Scope 的 resolved boundary 只提交 Framework 消费结果。

## 8. durable Host port

### 8.1 公开形态

目标接口使用四个语义明确的方法，而不是一个要求 type switch 的万能 `Commit(any)`：

```go
type TreeDurability interface {
	ProcessStartOutcomeAcknowledger
	ActivateTree(context.Context, TreeActivation) error
	CommitEffect(context.Context, EffectBoundary) error
	CommitCheckpoint(context.Context, TreeCheckpoint) error
}
```

`ProcessStartOutcomeAcknowledger` 是独立且已有真实消费者的 admission lifecycle 能力。ephemeral Host 可以单独使用它；完整 `TreeDurability` 嵌入同一个方法，在 durable mode 中把 outcome 与 tree head 原子提交。start outcome 因而始终只有一个出口，也不会把“只需闭合准入”误写成 active recovery。

其余方法集合故意闭合。新增 kernel durable boundary 是语义变化，应通过 ADR 和 breaking API 明确发生，不能伪装成 plugin extension。

目标相关 value：

```go
type TreeIncarnationID struct { /* opaque */ }

func ParseTreeIncarnationID(string) (TreeIncarnationID, error)
func (i TreeIncarnationID) String() string
func (i TreeIncarnationID) Valid() bool

type EffectBoundaryKind string
const (
	EffectBoundaryPending  EffectBoundaryKind = "pending"
	EffectBoundarySettled  EffectBoundaryKind = "settled"
	EffectBoundaryResolved EffectBoundaryKind = "resolved"
)

type TreeCheckpointKind string
const (
	TreeCheckpointParked   TreeCheckpointKind = "parked"
	TreeCheckpointTerminal TreeCheckpointKind = "terminal"
)
```

所有会推进 tree head 的 boundary value：

- immutable；
- accessor 返回 defensive copy；
- 包含 root identity、previous tree digest 和 prospective full TreeSnapshot；
- 包含该 boundary 所需的 typed facts；
- 不暴露 Store、transaction、lease 或产品 revision。

ephemeral lifecycle callback 的 tree accessor 返回 false。durable root started 没有 previous head但有 prospective base snapshot；durable root aborted 两者都没有；durable child started/aborted 两者都有。`ProcessStartOutcome.Valid` 校验 status 与生命周期字段，Engine 的 mode-specific validation 再强制这些 snapshot 组合。

建议 accessor：

```go
func (b EffectBoundary) Kind() EffectBoundaryKind
func (b EffectBoundary) Request() EffectRequest
func (b EffectBoundary) Settlement() (Settlement, bool)
func (b EffectBoundary) PreviousTreeDigest() Digest
func (b EffectBoundary) TreeSnapshot() TreeSnapshot

func (c TreeCheckpoint) Kind() TreeCheckpointKind
func (c TreeCheckpoint) PreviousTreeDigest() Digest
func (c TreeCheckpoint) TreeSnapshot() TreeSnapshot

func (t TreeSnapshot) Digest() Digest
func (t TreeSnapshot) IncarnationID() (TreeIncarnationID, bool)

func (o ProcessStartOutcome) StartedAt() (time.Time, bool)
func (o ProcessStartOutcome) PreviousTreeDigest() (Digest, bool)
func (o ProcessStartOutcome) TreeSnapshot() (TreeSnapshot, bool)

func (a TreeActivation) PreviousIncarnationID() TreeIncarnationID
func (a TreeActivation) PreviousTreeDigest() Digest
func (a TreeActivation) IncarnationID() TreeIncarnationID
func (a TreeActivation) TreeSnapshot() TreeSnapshot
```

### 8.2 配置语义

`EngineConfig.TreeDurability` 替换 `PreparedStepAcknowledger`，并复用 `ProcessStartOutcomeAcknowledger` 的生命周期方法：

- `nil`：zero-config ephemeral mode；
- 只配置 `ProcessStartOutcomeAcknowledger`：ephemeral execution，但 accepted admission 具有 conclusive lifecycle callback；
- 配置 `TreeDurability`：durable mode，Engine 使用嵌入的 outcome method 与另外三个方法；
- 两个字段同时 non-nil：`ErrInvalidEngineConfig`，避免同一 outcome 有两个出口；
- typed-nil：`ErrInvalidEngineConfig`；
- 不增加 `DurabilityMode` enum；
- 不要求普通用户填写 no-op implementation。

Flame 自己必须在构造时 fail closed：声明 active recovery 的 Runtime 不能传 nil。通用 Scope 无法猜测调用者产品意图。

### 8.3 callback context

Runtime 传给 durability callback 的 context：

- 不继承触发请求的 cancellation；
- 可以保留 Runtime 明确写入的 tracing value，但不得作为业务授权；
- Host implementation 必须使用自身配置的 deadline；
- callback 必须有界、并发安全，不 re-enter 所代表的 tree；
- 同一 tree 最多一个 callback in flight，不同 tree 可并发。

用户请求取消不能撕裂一个已经开始的持久化决议。

### 8.4 原子提交合同

除 root aborted 外，每个 callback 必须原子完成：

```text
compare current head = (RootID, incarnation, previous digest)
write idempotency fact for this logical boundary
write product facts owned by Host
replace head with prospective TreeSnapshot
```

不能出现“产品结果已提交但 tree head 未推进”或相反的半状态。

root started 以“head 不存在”为 compare 条件。root aborted 只原子关闭 admission，不创建 head。

逻辑幂等键：

| Boundary | Key |
|---|---|
| root started/aborted | `(RootID, ProcessID)` |
| child started/aborted | `(RootID, ProcessID)` |
| Effect pending/settled/resolved | `(RootID, EffectID, Kind)` |
| Parked/Terminal checkpoint | `(RootID, snapshot digest, Kind)` |
| activation | `(RootID, new TreeIncarnationID)` |

incarnation 故意不进入 Process/Effect 逻辑键：它表示 writer authority，不是新的逻辑工作。如果旧 writer 已提交，head 已包含结果；如果没有，新 incarnation 可以闭合同一 logical boundary。所有 lookup 前仍必须先验证 proposed incarnation 是 current incarnation，迟到旧 writer 因而不能覆盖新 head。

同一 key 的内容不同必须包装 `ErrDurabilityConflict`；current incarnation/head 不匹配必须包装 `ErrTreeIncarnationConflict`。

普通 boundary 的 callback 重试顺序固定为：

1. 验证 proposed incarnation 仍是 current incarnation；
2. 查询 logical idempotency key；
3. 已有字节等价记录且 head 已是 prospective digest 时返回 `nil`；
4. 已有记录但内容不同，返回 `ErrDurabilityConflict`；
5. 无记录时才 CAS previous head 并原子写入记录、产品事实和 prospective head。

incarnation 校验必须先于 idempotency success。否则旧 writer 可能把“历史上已提交”误当成自己仍有继续执行的权力。

### 8.5 ambiguous commit

callback 遇到 commit timeout 时不能假设“未提交”：

1. 在 callback deadline 内按幂等键与 head 对账；
2. 确认已提交且内容相同，返回 `nil`；
3. 确认未提交，返回原错误；
4. 仍无法确认，返回 fatal error，当前 Runtime 永久停止推进该树；
5. 后续 recovery 重新读取 authoritative head 并 activation。

迟到旧 transaction 与新 activation 必须竞争同一个 head CAS。Scope 不增加 Store-specific poison/abort API。

### 8.6 callback 失败

Engine 不在 callback 外重试 Store，也不把 durability error 转成普通 Settlement：

- activation 失败：`RestoreTree` 不发布任何 Process；
- root started 失败：`Start` 不发布 Process；
- root aborted 失败：仍不发布 Process，`Start` 返回 initialization 与 callback error；
- Effect pending 失败：确定未派发，丢弃 candidate，不加入 unresolved；
- Effect settled/resolved 失败：外部动作可能已发生，停止整树并保留 EffectID；authoritative head 仍按 Host 对账结果解释；
- child outcome 失败：停止整树，不伪造 child business failure；
- Parked/Terminal checkpoint 失败：不发布对应 Host boundary，停止当前 Runtime，后续从 authoritative head 恢复；
- incarnation conflict：按 stale Runtime 处理，见 §10.4。

已经运行的树遇到最终 callback error 后，不再启动新 Step/Effect。Runtime cancel 有界 job，形成本地 tree-scoped durability termination，供 Host 清理和 RunLost 决议；它不能假定未确认的内存状态已经 durable。

## 9. Process start

### 9.1 admission 与 outcome

`ProcessAdmitter` 保持一个独立、小而准确的 Host port。它可以做外部准入协调，但必须：

- 按 stable ProcessID 幂等；
- 对 relation、DeploymentRef、Budget、CapabilitySet 内容冲突 fail closed；
- 有界并尊重 ctx；
- 不创建 Process、不改变 Framework allocation；
- 不执行无法按 ProcessID 对账的不可逆动作。

accepted admission 必须有一个 conclusive started/aborted outcome。

### 9.2 `StartedAt` 的归属

`ProcessAdmission` 只包含跨恢复稳定的事实：

- Process relation/identity；
- DeploymentRef/Descriptor；
- Budget；
- CapabilitySet。

删除 `ProcessAdmission.StartedAt()`。

Runtime 在 admission accepted 后生成 tentative UTC `StartedAt`，仅放入 started `ProcessStartOutcome`。只有 outcome 与 prospective tree 原子提交后，它才成为权威生命周期时间。aborted outcome 没有 StartedAt。

否则 child 在旧 incarnation admission 后、outcome 前被恢复接管时，会为同一 ProcessID 产生不同 wall-clock admission 内容，破坏幂等。

### 9.3 root outcome

- root started：创建第一个 `TreeIncarnationID`，提交单节点 base TreeSnapshot 后才发布 Process；
- root aborted：关闭 admission，不创建 tree head，不发布 Process；
- root outcome callback 失败：`Start` 返回 error，不发布 Process。

root admission accepted 后若 Host 崩溃、base head 从未建立，不存在可恢复的 Framework tree。Host 必须按产品 admission 记录对账并标记 RunLost；Scope 不为这个窗口虚构空 snapshot。

### 9.4 child outcome

child outcome 是一个 tree transaction：

- started snapshot 同时包含 child、parent budget reservation、topology 和 parent StartChild Effect definite settlement；
- aborted snapshot 同时包含“无 child”、reservation release 和 parent Effect definite failure settlement；
- callback 成功后才发布 child/应用 parent settlement；
- callback 失败使整棵活树进入 durability fault，而不是把基础设施失败伪装成普通 child business failure。

同一 child ProcessID 的恢复重投必须得到内容等价 outcome；否则是 definition/deployment nondeterminism 或 Host protocol conflict。

## 10. fencing 与 restore

### 10.1 TreeIncarnationID

每个 durable tree snapshot 携带一个 Engine-minted、crypto-random `TreeIncarnationID`：

- root started 时生成首个 ID；
- 普通 boundary 沿用 current ID；
- 每次 restore 生成全新 ID；
- ephemeral checkpoint 没有 ID。

是否存在 valid incarnation 同时是 snapshot mode 的唯一权威表示，不再增加 durability boolean。

### 10.2 activation handshake

`RestoreTree` 流程：

1. 严格解析 candidate authoritative head；
2. 解析所有 exact DeploymentRef；
3. 验证 snapshot mode 与 Engine mode；
4. 生成新的 random incarnation；
5. 构造只替换 incarnation 的 prospective snapshot；
6. 调用 `ActivateTree`；
7. Host CAS previous `(digest, incarnation)` 并安装 new head/lease；
8. callback 成功后才发布任何 Process。

两个并发 restore 从同一 old head 生成不同 ID，因此最多一个 CAS 成功。旧 Engine 的迟到 callback、Event 和 Delta 均携带 old ID，可被拒绝。

只做 `generation = N + 1` 不能 fencing：两个并发 restore 会得到同一个 N+1。

### 10.3 mode mismatch

- 有 incarnation 的 snapshot 只能进入 durable Engine；
- 无 incarnation 的 snapshot 只能进入 ephemeral Engine；
- 任一方向错误都返回 `ErrTreeDurabilityMismatch`；
- 不在普通 Restore 中隐式“升级为 durable”或“降级为 ephemeral”。

若未来要把临时执行提升成 durable Run，必须设计显式 import handshake，原子建立首个 head。

### 10.4 stale Runtime

活 Runtime 收到 `ErrTreeIncarnationConflict`：

- 立即停止启动新的 Step/Effect；
- cancel in-flight jobs；
- 以 tree-scoped external durability fault 结束本地执行；
- 保留所有 pending/Unknown EffectID；
- 本地 Result/Event 仅供清理和诊断，不能更新 current Run。

activation 阶段 conflict 则 `RestoreTree` 原样返回，不发布 Process。

## 11. snapshot 与 Host 主动树变换

### 11.1 API 收敛

目标 public surface：

- `Snapshot` 重命名为 `ProcessSnapshot`；
- `Process.Capture(ctx)` 重命名为 `Process.Snapshot(ctx)`；
- `Process.ResolveEffect(ctx, settlement)` 重命名为 `Process.ResolveUnknownEffect(ctx, settlement)`；
- 删除 standalone `Engine.Restore`；
- 保留 `TreeSnapshot`、`ParseTreeSnapshot`、`Engine.RestoreTree`；
- `TreeSnapshot` 是唯一恢复单位。

ProcessSnapshot 仍可用于：

- status/usage/unknown diagnostics；
- Strategy-specific inspector；
- Event/debug tooling；
- 测试断言。

它不能启动一个脱离原 topology 的 Process。

### 11.2 CaptureTree

`CaptureTree` 只服务 ephemeral quiescent checkpoint：

- ephemeral Engine：保留现有 capture/restore；
- durable Engine：返回稳定 `ErrTreeCaptureUnavailable`，不 quiesce、不返回“可解析但禁止恢复”的假 snapshot；
- durable authoritative head 只来自 callback 或下节定义的线性 administrative transform。

如果未来需要 whole-tree live inspection，应新增不带 restore 语义的 `TreeView`，不能滥用 TreeSnapshot。

### 11.3 为什么 prepared cancellation 是合法的第二条路径

Flame 已真实使用：

```text
PrepareWaitingSubtreeCancellation
    → inspect exact prospective tree
    → commit product cancellation + checkpoint
    → Apply / Discard
```

它与 runtime-driven callback 的 ownership 不同：

- callback：Runtime 主动推进，Host 被动确认；
- prepared cancellation：Host 主动发起产品事务，Runtime 必须先冻结 source tree。

强行把两者塞进一个通用 callback 会颠倒事务发起权。正确设计是保留现有具体 capability，不泛化为 `PreparedTreeChange`。

durable mode 下增加两个精确 accessor/合同：

```go
func (p *PreparedWaitingSubtreeCancellation) SourceTreeDigest() Digest
func (p *PreparedWaitingSubtreeCancellation) ResultingSnapshot() TreeSnapshot
```

Prepare 返回前必须：

- source tree 已达到 acknowledged head；必要时先完成 Parked checkpoint；
- source tree 持续冻结；
- resulting snapshot 沿用 current incarnation；
- 所有可能失败的 Framework staging 已完成。

Host transaction 必须 CAS `(current incarnation, SourceTreeDigest)`，原子写产品 cancellation 与 resulting snapshot。事务成功后调用 contextless `Apply()`；确定失败则 `Discard()`。

`Apply()` 在同一个 owner-line gate 中安装 resulting Framework state，并把 treeRuntime 的 last acknowledged head 更新为 resulting digest。它不再调用 durability callback，因为 Host transaction 已经完成该边界。下一次 runtime-driven boundary 必须以 resulting digest 为 previous head。

若事务结果不确定，Host 必须 reread head：

- head == resulting digest：Apply，或销毁旧 executor 后从 resulting head 恢复；
- head == source digest：Discard；
- 其他值：incarnation/protocol conflict，fail closed。

崩溃发生在 Host commit 后、Apply 前时，恢复 resulting head 即可。prepared capability 本身绝不持久化。

### 11.4 不提前泛化

当前只有 waiting subtree cancellation 一个真实 Host-driven Framework mutation。继续保留具体 API：

```go
PrepareWaitingSubtreeCancellation(...) (*PreparedWaitingSubtreeCancellation, error)
```

只有第二个结构不同、却共享相同生命周期的真实 consumer 出现后，才评估私有公共实现或公开 `PreparedTreeChange`。现在泛化只会暴露无语义的 mutation primitive。

### 11.5 TreeSnapshot 的恢复事实

目标 wire 至少包含：

- schema version、RootID 与可选 TreeIncarnationID；
- canonical ordered ProcessSnapshot 集合；
- relation、DeploymentRef、status 与 lifecycle time；
- last stable ExecutionState 与 committed Step sequence；
- mailbox、signal cursor、pending control；
- budget、usage、tree limits 与 capability set；
- active child waits 和 topology accounting；
- 每个 Process 最多一个 prepared batch；
- batch 的 candidate ExecutionState、Transition、EffectID、planned/pending/settled phase 与可选 Settlement；
- terminal Output/Failure/Termination；unresolved EffectIDs 只存于 Termination。

wire 不包含：

- in-flight Step/Dispatch/callback job；
- goroutine、channel、mutex、timer 或 context；
- Event/Delta delivery cursor；
- Dispatcher、Definition 或 credential 实例；
- Store revision、transaction、lease、Run/Session/Interaction identity；
- Host input log、reconciliation evidence 或产品 retry policy。

编码必须 canonical。`ParseTreeSnapshot` 严格拒绝 unknown fields、duplicate identity、非法 topology/attenuation、无效 phase 组合、无效 DeploymentRef、超限 payload 和 trailing JSON。`RestoreTree` 再要求每个 DeploymentRef 都能 exact resolve。`TreeSnapshot.Digest()` 对 canonical JSON 使用现有 `Digest` 规则；相同语义必须得到相同 digest。

## 12. Unknown、终止与对账

### 12.1 Unknown 是权威状态

Unknown 表示：

> Effect 可能发生，但当前系统没有足够证据提交 succeeded 或 failed。

它不是 retry hint，也不是普通 error。只允许三种处理：

1. 相同 EffectID 且 ReplayPolicy 允许时恢复重派；
2. Host 对账后提交 definite Settlement；
3. Host 明确放弃整棵树，并保留 unresolved EffectIDs。

### 12.2 活树 resolution

`Process.ResolveUnknownEffect(ctx, settlement)`：

- 只接受当前 Process 拥有的 Unknown EffectID；
- 只接受 succeeded/failed definite Settlement；
- 在 owner line 与 Kill/control 排序；
- 形成 `EffectBoundaryResolved`；
- callback 成功后才替换 Unknown 并恢复调度；
- 同一 resolution 重投幂等。

### 12.3 terminal 后对账

终态 Process 返回 `ErrProcessFinished`，不再允许修改 Framework state。

终态后的对账属于 Host reconciliation：更新 usage、invoice、审计或 RunLost 证据，但不伪造一个“重新活过来”的 Scope Process。

owner line 顺序只有两种：

- resolution 先提交，再处理 Kill；
- Kill 先提交，resolution 返回 `ErrProcessFinished`。

不存在第三条半决议路径。

### 12.4 termination 表

| 当前 Effect | graceful terminal | Kill / durability fault |
|---|---|---|
| planned | 丢弃 | 丢弃 |
| pending | 不允许越过 | 保留为 unresolved |
| settled definite | 正常消费 | 不 unresolved |
| settled Unknown | 不允许越过 | 保留为 unresolved |

`Termination.UnresolvedEffectIDs()` 是唯一 Framework 表示，返回 canonical order 的 defensive copy。

Unknown retention、人工升级、容量上限和最终 RunLost 生存时间（time to live，TTL）属于 Flame。不同模型调用、支付工具和只读查询不应共享一个内核常量。

### 12.5 Engine.Close

`Engine.Close` 只有在以下条件全部满足时成功：

- 无 start reservation；
- 无非终态 tree；
- 无 prepared administrative capability；
- 无仍在运行的 Step/Dispatch/durability job；
- observation worker 可安全结束。

Kill 会把 pending/Unknown EffectID 冻结进 immutable `Termination`。这些 unresolved facts 不再需要活 Runtime；terminal checkpoint、Result 和 Host 记录负责保留它们，因此不会永久阻塞 `Close`。

`Close` 不是“帮我杀掉所有工作”。强制关闭必须由 Host 先明确 Kill/RunLost。durability fault 已使本地树终止时，Host 可以在记录清理决议后关闭旧 Engine，再从 authoritative head 恢复或结束产品 Run。

## 13. capability-oriented authorization

### 13.1 当前模型合理的部分

保留：

- immutable `Capability` / canonical `CapabilitySet`；
- Engine 配置 root ceiling；
- child `Capabilities ⊆ parent.Capabilities`；
- Effect 声明 `RequiredCapabilities`；
- Engine 在 durable pending 前 gate；
- snapshot/restore 复验 attenuation。

这对 Agent kernel 很合适：权限跟随 tree 委派，而不是从全局 registry 猜测。

### 13.2 必须诚实的边界

字符串 label 只证明 Framework 做过授权判断，不能阻止一个错误 Dispatcher：

- 直接读任意文件；
- 使用超出声明的 credential；
- 访问任意网络；
- 通过共享全局对象越权。

Flame 必须把 capability 映射到真实执行约束：独立 tool binding、credential scope、workspace sandbox、network policy。Scope GoDoc 必须使用“authorization/gating”，不能使用“sandbox/security boundary”作无条件承诺。

### 13.3 不制造 Handle 爆炸

不为每个动作增加 `CanPauseHandle`、`CanKillHandle`、`CanSignalHandle`。Process handle 继续表达 Engine-issued identity；动作权限由：

- 谁持有 handle；
- Host application ACL；
- Engine state validation；
- Effect capability gate；

共同决定。只有需要可传递、可衰减、可撤销的一次性 authority，才使用 prepared capability object。

## 14. 扩展模型：闭合内核，开放边缘

| 扩展需求 | 正确入口 | 不应做 |
|---|---|---|
| 新 Agent 行为 | `Definition` / `Execution` | 修改 Engine switch |
| 新外部动作 | Strategy Effect + `Dispatcher` | 在 Step 中 I/O |
| 新部署绑定 | `DeploymentResolver` | 全局动态 registry |
| 新准入策略 | `ProcessAdmitter` | 把 tenant 放进 snapshot |
| 新 durable Host | `TreeDurability` | 把 Store 放进 agent |
| 新观察/遥测 | Event/Delta listener | 用 observer veto 状态 |
| 新产品工作流 | Flame application use case | 新增 Framework Effect |
| 新内核组合语义 | 证据 + ADR + typed Effect/helper | 通用 hook |

判断一个新扩展点是否该进入 `agent`：

1. 它是否改变 recovery safety 或 tree invariant？
2. 是否已有至少两个结构不同的真实消费者？
3. 能否由现有 Effect/Dispatcher 或 Host composition 完成？
4. 接口是否由 consumer 定义且方法足够少？
5. 删除它后是否仍能正确实现当前场景？

只有 1 为真，或 2 为真且 3 为假，才考虑新增内核公开面。

## 15. 易用性

### 15.1 四层使用路径

| 场景 | 用户看到的 API | 需要理解 durability 吗 |
|---|---|---:|
| 单次执行 | `NewEngine` + `Run` | 否 |
| 长生命周期内存执行 | `Start` + Process control/Await | 否 |
| 临时 checkpoint | ephemeral `CaptureTree/RestoreTree` | 只需 tree snapshot |
| durable harness | `TreeDurability` + `RestoreTree` | 是 |

普通用法继续短：

```go
engine, err := agent.NewEngine(agent.EngineConfig{})
if err != nil { /* handle */ }
defer engine.Close()

result, err := engine.Run(ctx, deployment, input)
```

不新增 Builder、Harness、Session、Supervisor 或 functional options facade。Flame 已经是 harness。

### 15.2 durable Host 的最小心智模型

Host 作者只需记住五句话：

1. 保存完整 TreeSnapshot，不解析后自行拼装；
2. pending 前保存 intent，settled 时与产品结果同事务；
3. Parked/Terminal checkpoint 是纯状态 program counter；
4. restore 先 CAS 新 incarnation，再发布执行；
5. Unknown 不猜、不自动抹掉。

`agenttest` 提供 recorder、crash gate、head model 和 conformance driver，避免每个 Host 自己理解 20 个 crash window。

### 15.3 错误分类

建议稳定 sentinel：

```go
var ErrDurabilityConflict = errors.New("agent: durability identity conflict")
var ErrTreeIncarnationConflict = errors.New("agent: tree incarnation conflict")
var ErrTreeDurabilityMismatch = errors.New("agent: tree durability mismatch")
var ErrTreeCaptureUnavailable = errors.New("agent: tree capture unavailable")
```

- content conflict：`FailureKindContract / engine.tree.durability_conflict`；
- Host callback failure：`FailureKindExternal / engine.tree.durability_failed`；
- stale writer：`FailureKindExternal / engine.tree.incarnation_conflict`；
- Strategy nondeterminism 证据应包含 stored/proposed digest 与 boundary identity；
- 错误 message bounded，不嵌入 snapshot payload、prompt、credential 或任意 Host body。

### 15.4 文档与示例

落地时必须同时提供：

- 一个 30 行以内的 ephemeral Run example；
- 一个 CaptureTree/RestoreTree example；
- 一个内存 `TreeDurability` teaching adapter；
- 一张 durable lifecycle/crash matrix；
- Flame adapter 的 integration contract；
- 每个 exported type 的 package-level usage link。

## 16. observation 不参与状态决议

Event 和 Delta 仍是非权威 observation：

- 不 veto；
- 不 acknowledgment；
- 不恢复；
- listener panic/slow consumer 与 Process state 隔离；
- Delta 可丢，Event 按现有可靠性合同处理。

durable Engine 的 Event/Delta 必须携带 current `TreeIncarnationID`；ephemeral value 返回 false。持久化 sink 先比较 current head：

- old incarnation Event 只能进入 stale-tagged diagnostics；
- old incarnation Delta 直接丢弃；
- 二者都不能更新 current Run。

identity 放在 value 本身，避免 Engine-global listener 暗中维护 `ProcessID → incarnation` 映射。

## 17. Flame 迁移

### 17.1 当前状态

Flame 当前只保存 quiescent whole-tree checkpoint，没有使用 per-Process prepared-step acknowledgment；active crash 保守映射为 RunLost。这在旧合同下是正确的 fail-closed 行为。

新协议落地前不能提前宣称 active recovery。

### 17.2 target adapter

Flame 的 `TreeDurability` adapter 负责：

- authoritative head 表；
- `(incarnation, previous digest)` CAS；
- boundary idempotency records；
- pending intent 与 model/tool attempt；
- settlement、usage、Transcript/Item 同事务；
- Parked checkpoint 与 Pending/Interrupt 同事务；
- root/child admission outcome 与 Run/member 投影；
- activation 与 product lease 接管；
- stale Event/Delta filtering。

Scope 不知道这些表和产品类型。

Dispatcher 不应先发布权威产品结果再返回 Settlement。Flame 可以按 EffectID 暂存调用结果：小结果放在有界内存 staging，大结果写入不可见的 provisional record。`CommitEffect(Settled)` 在同一事务中消费 staging、发布产品结果并推进 tree head；commit 前崩溃时，head 仍是 pending，恢复按 Unknown/ReplayPolicy 处理。

### 17.3 boot recovery

Flame 只恢复同时满足以下条件的 active tree：

- 有合法 durable head/incarnation；
- exact Deployment/world/build 可解析；
- workspace 与 capability binding 可重建；
- Unknown policy允许恢复；
- product lease policy 允许接管。

顺序：

1. 读 authoritative head；
2. 验证 Host scope；
3. 构造 exact DeploymentResolver/Dispatcher/TreeDurability；
4. 调用 `RestoreTree`；
5. activation CAS 成功后注册 live executor；
6. replay 尚未进入 head 的 durable Host inputs；
7. 对 pending Effect 按 ReplayPolicy 处理。

任一条件缺失仍 durable 标记 RunLost，不做 best-effort 猜测。

### 17.4 waiting subtree cancellation

保留当前正确顺序：

```text
prepare Framework change and freeze source
build product transformation
commit product facts + resulting head CAS
Apply()
if in-memory apply fails: tear down and restore committed head
```

新协议只补 source digest/incarnation 约束，不把 application transaction 移入 Scope。

## 18. 实施切片

每个切片必须能独立 review，不把内部重构、wire breaking 和 Flame migration 混成一个大提交。

### Slice 0：接受语义

- 把本设计拆成 ADR；
- 明确 accepted API baseline 变化；
- 冻结 boundary kind、error 和 crash matrix；
- 不改生产行为。

### Slice 1：treeRuntime 内部 owner

- 先保持现有 public API 与 snapshot wire；
- Process loop command 收敛到 root-tree owner；
- 引入 Step/Dispatch job + attempt token；
- sibling fairness、stale completion、race tests 先通过。

### Slice 2：唯一恢复单元

- `Snapshot` → `ProcessSnapshot`；
- `Process.Capture` → `Process.Snapshot`；
- `Process.ResolveEffect` → `Process.ResolveUnknownEffect`；
- 删除 `Engine.Restore`；
- TreeSnapshot 增加 digest/incarnation/effect phase；
- 严格 parse、canonical encode、size bounds。

### Slice 3：稳定输入与生命周期

- 删除 Signal ReceivedAt；
- `StartedAt` 从 admission 移到 started outcome；
- child outcome 形成 atomic prospective tree；
- 更新 interaction/planning/workflow inspector。

### Slice 4：TreeDurability

- 删除 `PreparedStepAcknowledger`，让完整接口嵌入 start-outcome lifecycle 能力；
- 实现 root base、Effect pending/settled/resolved；
- 实现 Parked/Terminal checkpoint；
- durable CaptureTree 拒绝；
- 加入 conformance suite。

### Slice 5：fencing 与 Unknown

- activation CAS；
- stale writer behavior；
- Unknown termination preservation；
- live resolution 与 terminal reconciliation 分离；
- Engine.Close 收紧。

### Slice 6：prepared administrative transform

- SourceTreeDigest；
- durable source-head requirement；
- ambiguous commit reconciliation tests；
- 保持具体 waiting cancellation API。

### Slice 7：Flame adapter

- 先实现 schema/transaction/head CAS；
- 再接 Effect settlement；
- 再接 Parked checkpoint；
- 最后开启 boot active recovery；
- feature flag 下与 RunLost fallback 并行验证。

### Slice 8：删除与文档

- 删除旧 acknowledger、standalone restore 和兼容 shim；
- 删除旧 quiescent durable capture path；
- 更新 Architecture/Engineering/API baseline/ADRs/examples；
- 运行 API digest 与 forbidden-symbol tests。

## 19. 验证恢复与并发语义

### 19.1 状态机模型

至少覆盖：

- planned → pending → settled definite；
- planned → pending → settled Unknown → resolved；
- pending restore safe replay / never replay；
- child admitted → started/aborted atomic tree；
- Parked/Terminal checkpoint；
- control 与 resolution owner-line 排序；
- old incarnation completion/callback 拒绝。

每条随机 trace 后断言：

- topology 与 relation 完整；
- budget 不超分且 accounting 守恒；
- child capabilities 是 parent 子集；
- Effect phase 单调；
- 一个 EffectID 不产生两个不同 committed request/settlement；
- acknowledged head 是 valid full TreeSnapshot；
- unresolved IDs 与 pending/Unknown facts 精确一致。

### 19.2 crash prefix matrix

| Crash 位置 | authoritative state | 恢复动作 |
|---|---|---|
| root outcome commit 前 | 无 tree head | product admission 对账 / RunLost |
| root commit 后、publish 前 | base head | activation restore |
| pending commit 前 | previous head | 重算纯 Step |
| pending commit 后、dispatch 前 | pending | replay policy |
| dispatch 中/后、settled 前 | pending | replay policy / Unknown |
| settled commit 后、memory apply 前 | settled | 不重派，继续 |
| Parked commit 后、publish event 前 | parked head | 恢复 waiting facts |
| Terminal commit 后、product publish 前 | terminal head | 幂等发布结果 |
| activation CAS 后、publish 前 | new incarnation head | 再次 activation |
| admin product commit 后、Apply 前 | resulting head | restore resulting tree |

测试不能只 crash goroutine；必须在 Host commit 的 before/after gate 精确切断。

### 19.3 并发与 race

- sibling Step job 并发但 commit 顺序单一；
- slow Step cancellation 不阻塞 sibling command；
- completion 与 Kill/Resolve/Pause 竞争；
- old attempt completion 被丢弃；
- 两个 restore 只有一个 activation 成功；
- old callback 与 new activation CAS 竞争；
- prepared Apply/Discard 恰好一个成功，值复制不复制 authority；
- unrelated root trees 无全局 head-of-line blocking；
- `go test -race` 无数据竞争。

### 19.4 TreeDurability conformance

`agenttest` driver 允许 adapter 测试实现：

- seed/load authoritative head；
- 构造 adapter；
- 注入 before/after commit ambiguity；
- 观察 committed idempotency facts。

公共 suite 验证：

- exact head CAS；
- same-content retry；
- conflicting-content rejection；
- incarnation fencing；
- root/child outcome atomicity；
- Effect 三类 boundary；
- Parked/Terminal checkpoint coalescing；
- delayed transaction 与 activation 只有一个赢家。

Scope 不能证明第三方数据库真的支持 CAS，但可以让“不实现合同的 adapter”无法通过官方 suite。

### 19.5 Flame integration

- model/tool 产品结果与 settled head 同事务；
- waiting checkpoint 与 Interrupt/Pending 同事务；
- child start 与 member/Run projection 同事务；
- active crash 不重复已 settled tool；
- pending never-replay 进入 Unknown；
- stale observation 不更新 current Run；
- missing build/workspace/capability binding 仍 RunLost；
- prepared waiting cancellation 的所有 crash prefix 可恢复。

### 19.6 工程检查

每个切片至少运行：

```bash
go test ./agent/... -count=1
go test -race ./agent/... -count=1
go vet ./agent/...
go test ./agent/... -run 'Architecture|Baseline|Forbidden|ExternalAPI'
```

Flame migration 同时运行其 Runtime/application/adapter 集成测试。性能变化先 benchmark，再优化。

## 20. 性能纪律

正确性首版的成本是：

- 每个 Dispatcher Effect 通常 2 次 durable commit；
- Unknown resolution 再 1 次；
- root/child start outcome 1 次；
- Parked/Terminal 各 1 次 canonical checkpoint；
- restore 1 次 activation；
- full TreeSnapshot 的 encode/hash/store。

需要测量：

- tree size 与 snapshot bytes；
- encode/hash p50/p95/p99；
- callback latency；
- 同 tree boundary queue time；
- pure Step replay cost；
- sibling runnable fairness；
- Flame transaction size 与 lock time。

只有真实 trace 证明瓶颈后，按顺序考虑：

1. 避免重复 clone/encode；
2. immutable subtree structural sharing（内部实现，不改 wire）；
3. Host blob dedup/compression；
4. 明确的 boundary batching ADR；
5. 最后才是增量 snapshot。

任何优化都必须继续产生一个有序、原子的 authoritative tree prefix。

## 21. 被否决的替代方案

### 21.1 把 Scope 定义成 OTP × Temporal × capability security

拒绝。它容易把类比误写成需求，带来 supervisor、history、activity、sandbox 等不属于骨架的概念。本文直接从 Scope/Flame 的 crash、ownership 和授权约束推导。

### 21.2 保留每个 Process 一个独立状态 owner

拒绝。跨 Process 的 child start、budget、wait 与 tree snapshot 需要额外 quiescence/lock 协调，恢复单元和 mutation owner 不一致。

### 21.3 在 tree owner goroutine 内同步运行 Step

拒绝。较慢但有界的纯计算会饿死 sibling 和 control。Step job 可以并行计算，但只有 owner 能提交。

### 21.4 从 `Execution.Step` 删除 context

拒绝。有界不等于快。保留 Runtime 构造的 cancellation-only context，同时切断 value/deadline/clock 注入。

### 21.5 把 Storage/transaction/lease 放进 agent

拒绝。它会把 Flame harness 语义污染到通用骨架，并限制第三方 Host。

### 21.6 append-only event journal 作为恢复源

拒绝。Scope 当前需要的是完整当前 program counter，不需要 history query、workflow replay 和 compaction 系统。Event 继续非权威。

### 21.7 只保留 start + Effect 三类 durability callback

拒绝。它无法覆盖纯 Step 后的 Parked/Terminal，也无法替代 Flame waiting checkpoint。

### 21.8 一个通用 `Commit(context.Context, any)`

拒绝。Go 调用方只能 type switch，invalid field combination 增多，未来新增 kind 会静默改变实现义务。四个 typed method 更清楚；内核 boundary 本就应闭合。

### 21.9 durable Engine 继续开放 CaptureTree

拒绝。释放冻结后再由 Host 保存会制造非权威或过期 head；返回“能 parse 但不能 restore”的 snapshot 更糟。durable head 来自 callback 或冻结中的 prepared administrative transform。

### 21.10 立即泛化 `PreparedTreeChange`

拒绝。现在只有 waiting subtree cancellation 一个真实 Host-driven mutation；通用 mutation capability 会绕开 typed tree invariants。

### 21.11 每个纯 Step 都提交 snapshot

拒绝。纯计算可从最近 head 重算；无证据的高频事务会显著放大模型/工具运行成本。只提交 Effect 和 canonical Parked/Terminal cut。

### 21.12 自动重放所有 pending Effect

拒绝。不可幂等的外部操作可能重复。只遵守 Dispatcher 的 same-identity ReplayPolicy，否则 Unknown。

### 21.13 通用 supervisor restart policy

拒绝。哪些失败可重启、是否换模型、是否 RunLost 是 harness 产品策略。Scope 只提供精确 failure/termination/recovery facts。

### 21.14 用递增 generation fencing

拒绝。两个并发 restore 从同一 N 得到同一 N+1。random unique incarnation + previous-head CAS 才能区分 writer。

### 21.15 把 StartedAt 保留在 ProcessAdmission

拒绝。同一 child identity 在跨 incarnation 重投时会得到不同 admission 内容。StartedAt 只有在 started outcome commit 后才成立。

### 21.16 保留 Signal.ReceivedAt 并依赖 Host 重投

拒绝。同一 SignalID 会因为 wall clock 变成不同 Strategy 输入。业务时间放 payload，观测时间放 Event。

### 21.17 capability label 等于 sandbox

拒绝。Framework gate 与真实资源隔离是两层合同；混称会产生错误安全承诺。

## 22. 接受标准

只有同时满足以下条件，本设计才可进入 accepted baseline：

- root tree 是唯一恢复和 Framework consistency unit；
- treeRuntime 拥有唯一 commit line，slow Step 不阻塞 sibling；
- Process-level restore 已删除；
- Signal 不再包含 Engine wall-clock Strategy input；
- Dispatcher Effect 具备 planned/pending/settled/Unknown 完整语义；
- Parked/Terminal checkpoint 闭合 durable program counter；
- child outcome 原子闭合 admission、topology、budget 与 parent settlement；
- unique incarnation + previous-head CAS 在 publication 前完成；
- stale writer callback/observation 可识别并拒绝；
- Unknown 在 Kill/durability fault 后仍可见；
- durable CaptureTree 被明确拒绝；
- prepared waiting cancellation 在 durable head 上有 source/result CAS 合同；
- `EngineConfig{}` 仍是 zero-config 正确路径；
- Scope 没有 Store、Run、Session、model、tool 或 lease 抽象；
- capability GoDoc 不宣称 sandbox；
- agenttest conformance 与 crash-prefix suite 完整；
- Flame 真实事务证明 product facts 与 tree head 原子；
- Architecture、Engineering Standards、API Baseline、ADR、examples 与实现同步；
- 无 compatibility shim、双重恢复路径或重复权威字段残留。

达到这些条件后，对 Scope 更准确的描述不是“像 OTP/Temporal”，而是：

> **Scope agent 是一个小而完整的 durable effect kernel；Flame 在其上实现可运营的 AI agent harness。**
