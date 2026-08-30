# Agent 模块内核复审（2026-08-30）

范围：`agent` module 全量生产代码与其契约文档。基线：`46a8e8b0e`（`fix(interaction): redact tool authorization causes`）。
验证：`go build ./...` + `go test ./...` 全绿，15 个包。

本文只记录本轮复审的事实与待修条目，不复述架构设计意图（见 [`../agent/doc/ARCHITECTURE.md`](../agent/doc/ARCHITECTURE.md)）。
条目前缀 `AG` 与三轮审计（`A/B/C/O/E/S/V/M`）和框架对比（`G`）互不重叠。

---

## 结论

本轮把 `agent` 从**设计上可恢复**推进到**逐个崩溃点被证明可恢复**：durable tree commit protocol、incarnation fencing、10 点崩溃前缀矩阵、公开契约套件，四者合起来是质变而非增量。

待修的 5 条中，AG1/AG2/AG4 是收尾级别，AG3 是本轮新增并发面之后**明确欠下**的不变量注释，AG5 是唯一需要一次显式裁决的设计点。

---

## 一、本轮已建立的能力（复核记录）

### 1.1 durable tree commit protocol

`TreeDurability` 是闭合的 Host 提交端口（`tree_durability.go:304`）：

```go
type TreeDurability interface {
	ProcessStartOutcomeAcknowledger
	ActivateTree(ctx context.Context, activation TreeActivation) error
	CommitEffect(ctx context.Context, boundary EffectBoundary) error
	CommitCheckpoint(ctx context.Context, checkpoint TreeCheckpoint) error
}
```

- **`effectPhase` 是持久化程序计数器**（`effect_phase.go`）：`planned → pending → settled`，并以 `resolveUnknown` 表达 `settled(unknown) → settled(known)` 的单调裁决。对应公开的 `EffectBoundaryPending / Settled / Resolved`。
- **`TreeIncarnationID` 是 fencing token**（`tree_incarnation.go`）：`"incarnation:"` + 16 字节 crypto-random。每次 restore 在发布任何 Process 前铸新代际，Host 对 `(previousIncarnationID, previousTreeDigest)` 做 activation CAS；旧 writer 的迟到 commit 被 fence。
- **边界自校验**：`EffectBoundary.Valid()` 通过 `matchesProspectiveTree()` 反查 prospective snapshot 中该 Process 的 prepared step，逐项比对 effect ID、payload（JSON 字节级）、phase 与 settlement。不自洽的边界无法构造。`TreeCheckpoint.matchesSafeCut()` 同理：Parked 仅在每个非终态 Process 处于 `Waiting`/`Paused` 或持有 unknown 结算时成立。
- **端口是纯写的**：接口无任何读方法；恢复由 Host 自行加载 `TreeSnapshot` 后交给 `Engine.RestoreTree`。durable 模式下 `CaptureTree` 返回 `ErrTreeCaptureUnavailable`。因此「Agent 不定义 Store/Repository」（`ARCHITECTURE.md:564`）仍然成立，且区分是可执行的。
- **两档模式**：`TreeDurability == nil` 为零配置 ephemeral；非 nil 为 durable，并与 `ProcessStartOutcomeAcknowledger` 构造期互斥。

### 1.2 崩溃前缀矩阵

`agenttest/tree_durability_crash_test.go:246` 的 `TestTreeDurabilityCrashPrefixMatrix` 在 Host callback 的 before/after 精确位置注入模拟崩溃，覆盖 10 个前缀：

```
root outcome before commit
root outcome after commit before Process publication
pending before commit
pending after commit before dispatch
after dispatch before settled commit
settled after commit before memory apply
parked after commit before Event publication
terminal after commit before Result publication
activation after CAS before Process publication
admin after product commit before Apply
```

其中第四点断言 `Dispatcher ran before pending state became authoritative`，即写前日志纪律的可执行证明：副作用发出前，"将要发出"这一事实必须已经持久。

### 1.3 `agenttest` 公开契约套件

- `RunDefinitionConformance` —— Descriptor 稳定、每次 Start 产独立 Execution、Restore 精确、无隐藏可变输入、无共享 state。套件自带反例测试（`TestDefinitionConformanceDetectsHiddenMutableInput`、`TestDefinitionConformanceDetectsSharedExecutionState`），证明它能抓到违规。
- `RunTreeDurabilityConformance` —— base head 创建、同内容重试幂等、三种 Effect boundary、Parked/Terminal checkpoint、旧 writer fencing、迟到 commit 输给 activation。
- `MemoryTreeDurability` —— 参考适配器，注释明确其定位为教学与测试，生产 Host 应以自有存储实现同一事务。
- `ScriptedDispatcher` —— 测试替身。

该套件把「第二个 Host 自证边界」的成本降到数行，是本轮对「单消费者无法证明边界切分正确」这一风险的正面解法。

### 1.4 其他

- **typed observation facts**：`Event` 提供 7 个 typed 访问器（`ProcessFinished` / `SignalAccepted` / `StepFinished` / `StepCommitted` / `EffectStarted` / `EffectFinished` / `DeltaDropped`），各返回 `(XxxFact, bool)`；Event 携带 `TreeIncarnationID()`。`otel/agent` 消费 typed fact，不解析 raw JSON。
- **`ReplayPolicy` 成为显式合同**（`dispatcher.go:11`）：`Never` / `SameIdentity`，纯函数、无 I/O、构造期校验、panic 归一为 `errInvalidReplayPolicy`。
- **未知结算裁决是公开操作**：`Process.UnknownEffectIDs` 与 `Process.ResolveUnknownEffect`。
- **`testing/synctest`** 4 处使用，并有 gate `TestTimeSleepIsScopedToSynctestCallback` 限制 `time.Sleep` 只出现在 synctest 回调内。
- **新增 `performance_benchmark_test.go`**，闭合审计条目 V1 在本模块的缺口。
- **新增 `contract_policy_test.go`**（`TestPublicInterfacesAreDocumentedAndParametersNamed` / `TestManagedExecutionVocabularyIsUnambiguous` / `TestErrorCausesAreWrapped`），闭合审计条目 V3 在本模块的缺口。

### 1.5 规模

| | 上一轮 | 本轮 |
|---|---:|---:|
| 生产代码（不含 `examples`） | 22,743 | **24,672** |
| 测试代码 | 16,966 | **20,152**（88 文件） |
| Go 包数 | 6 | **7**（新增 `agenttest`） |
| `tree_*` 家族 | 2 文件 | **10 文件 / 3,845 行** |

包分布：root 12,988 / `interaction` 4,516 / `planning` 2,361 / `workflow` 2,249 / `agenttest` 1,817 / `platform` 489 / `planning/goap` 252。

---

## 二、待修条目

| 条目 | 概要 | 影响面 | 破坏性 API |
|---|---|---|---|
| **AG1** | `agenttest` 未登记进架构文档的生产 package 集合 | 文档 | 无 |
| **AG2** | 两处规则指向已删除的 ADR 文档 | 文档 | 无 |
| **AG3** | `Engine` 两把锁无归属与次序注释 | 注释 | 无 |
| **AG4** | `samber/lo` 在本模块 7 处非测试使用 | 依赖 | 无 |
| **AG5** | durable 模式下 wire 变更缺少政策与版本信号 | 需裁决 | 待定 |

---

### AG1 · `agenttest` 未登记进生产 package 集合

**现状**：架构门禁与架构文档对同一件事给出两个答案。

- `agent/package_dag_test.go:18` 已登记 `"agenttest": {".": {}}`，门禁承认它是一个包。
- `agent/doc/ARCHITECTURE.md:639` 声明「当前生产 package 集合**精确为**」，`:643–650` 列出的条目不含 `agenttest`。
- `agenttest` 实际持有 1,817 行非测试代码。

**为什么要紧**：这同时违反了同一份文档 `:659` 的规则 ——「新 package 只有在独立变化原因和真实消费者已被证明、ADR 已更新生产 package 集合与允许边后才能建立」。文档正在描述一个与代码不同的仓库。

**建议**：按 `core/CLAUDE.md` 对 `modeltest` / `vectorstore/storetest` 的既有处理方式，在 `:643–650` 的包集合中显式增加一行：

```text
├── agenttest/               可复用契约套件与参考适配器（不进入 Host 生产依赖闭包）
```

并在其后的约束段落说明它与 `examples/` 同属非生产依赖，只提供测试 suite / fixture / 参考实现，不被生产代码 import（此约束已由 `package_dag_test.go` 的边表实际执行）。

---

### AG2 · 两处规则指向已删除的 ADR 文档

**现状**：

- `agent/doc/ARCHITECTURE.md:659`：「……**ADR 已更新**生产 package 集合与允许边后才能建立」
- `agent/doc/ENGINEERING_STANDARDS.md:8`：「发生冲突时，先按更严格且更接近根因的规则处理；仍无法裁决时，停止实现并**更新 ADR**」

但 `agent/doc/DECISIONS.md` 已删除，且 `agent/CLAUDE.md` 明确声明 *decision ledgers … do not live in the current contract set*。

**为什么要紧**：`ENGINEERING_STANDARDS.md:8` 是**原则冲突的最终裁决出口** —— 「裁决不了时该做什么」现在指向一个不存在的文档，这条规则实际上是断的。`ARCHITECTURE.md:659` 则让新增 package 的准入条件无法被满足（AG1 正是它的实例）。

**建议**：既已裁定 ADR 不属于当前合同集，把两处出口改指现行文档：

- `:659` → 「……并在本文的生产 package 集合与允许边中登记后才能建立」
- `ENGINEERING_STANDARDS.md:8` → 「……仍无法裁决时，停止实现并先更新 `ARCHITECTURE.md` 的对应条款」

---

### AG3 · `Engine` 两把锁无归属与次序注释

**现状**（`agent/engine.go:76`）：

```go
type Engine struct {
	durability               TreeDurability
	startOutcomeAcknowledger ProcessStartOutcomeAcknowledger
	resolver                 DeploymentResolver
	admitter                 ProcessAdmitter
	observation              *observationBus
	limits                   Limits
	treeLimits               TreeLimits
	capabilities             CapabilitySet
	treeOperationsMu         sync.Mutex
	treeOperations           map[ProcessID]*treeOperation

	mu                      sync.RWMutex
	processes               map[ProcessID]*processController
	trees                   map[ProcessID]*treeRuntime
	startReservations       map[ProcessID]processStartReservation
	treeRestoreReservations map[ProcessID]*treeRestoration
	children                map[childIdentity]ProcessID
	childStartReservations  map[childIdentity]ProcessID
	closed                  bool
}
```

空行完成了字段分区，但没有任何注释说明：`treeOperationsMu` 守护哪些字段、`mu` 守护哪些字段、两者能否同时持有、若能则次序为何。

**为什么要紧**：本轮新增了 commit goroutine、`commitDone` channel、`deferDuringCommit` 命令延迟、freeze/incarnation fence 与后台 restore reservation，并发面显著扩大。这条不变量违反时不产生编译错误，只在生产暴露。

根 `CLAUDE.md` 注释纪律第 5 条正是本条的依据：「并发 / 事务 / 安全约束：goroutine 所有权与生命周期、**锁持有顺序**、channel 关闭方、ctx 取消语义、信任边界 —— 违反不报编译错、只在生产炸。」

对应三轮审计的 **B3**（锁归属注释）与 **B7**（god struct 字段分区），本模块的形态已由本轮变更升级为明确欠债。

**建议**：在两个字段分区前各写一段注释，至少表达：

1. 每把锁守护的字段集合；
2. 两把锁是否允许同时持有，若允许则唯一合法次序；
3. 持锁期间禁止调用的边界（Host callback、Dispatcher、listener 均不得在持锁时进入）。

同时建议为 `treeRuntime`（`tree_runtime.go:14`）补一段所有权注释：单写者 owner line 的范围、`commitDone` 的关闭方、命令在 commit 在途时的延迟规则。B1 的 `quiescedTree` 已经证明这类不变量写下来有价值。

---

### AG4 · `samber/lo` 在本模块 7 处非测试使用

**现状**：

```
agent/deployment.go            agent/execution_boundary.go     agent/engine.go
agent/planning/dispatcher.go   agent/planning/definition.go
agent/interaction/dispatcher.go
agent/platform/selection.go
```

全部用途为 `lo.IsNil`（typed-nil 检测）。

**为什么要紧**：typed-nil 检测是 Framework 构造期边界校验的组成部分，属于本模块自有语义，不宜外包给第三方通用库。对应三轮审计条目 **A2**（该条目的 owner 落点问题在本模块内不存在：agent 可以直接持有一个未导出 helper）。

**建议**：在根包实现一个未导出的 `isTypedNil(value any) bool`，替换 7 处调用并移除依赖。跨模块的 A2 收敛可另行处理，本模块无需等待其 owner 裁决。

---

### AG5 · durable 模式下 wire 变更缺少政策与版本信号（需裁决）

**现状**：整条持久化链上没有任何版本字段。

```go
// agent/execution_state.go:15
type ExecutionState struct {
	kind    string
	payload json.RawMessage
}
```

`TreeSnapshot` 与 `ProcessSnapshot` 同样没有版本字段。而 `agent/doc/ARCHITECTURE.md:583` 仍是：

> snapshot schema 在开发阶段直接 breaking，不保留长期 dual-read。

**为什么要紧**：这句政策成立于 ephemeral 时代 —— 快照只存在于内存，schema 变更的代价是重跑一次。durable 模式改变了这个前提：Host 的 authoritative head 中持有 `TreeSnapshot` 的 JSON，一次 Framework wire 变更等于所有在途 durable tree 无法恢复。

需要公允记录两点缓解事实：

1. **失败是响亮的**。`wireJSON.normalize` 与未知字段拒绝使解码直接报错，不产生静默腐蚀。
2. **Strategy payload 有兜底**。恢复要求精确 `DeploymentRef`，Deployment digest 覆盖 schema 与影响恢复语义的配置。

因此缺的不是安全性，而是两样：

- **政策条款**：durable 模式下 Framework 升级对在途树意味着什么（要求 Host 先 drain？显式作废并记录？）当前 `:583` 未作区分。
- **版本信号**：Host 无法区分「Framework 升级导致解码失败」与「存储损坏」。

**建议**（二选一，需裁决）：

- **方案 A（政策化）**：在 `:583` 后补一条 durable 限定，明确 wire 变更要求 Host 在升级前 drain 或显式作废在途 durable tree，并说明 Framework 不提供跨版本恢复。成本最低，与 pre-1.0 立场一致。
- **方案 B（版本化）**：为 tree envelope 增加一个 Framework-owned 版本号，恢复时对不匹配版本返回可区分的 sentinel error。成本略高，但让 Host 能把「升级」与「损坏」分流到不同的运维动作。

倾向 A：pre-1.0 阶段不为跨版本恢复付复杂度，但必须把代价写下来而不是留在默认。若未来有 Host 声明需要滚动升级不中断在途树，再按真实消费者升级到 B。

---

## 三、修复批次建议

| 批次 | 条目 | 说明 |
|---|---|---|
| 1 | **AG1 + AG2** | 纯文档，一个 commit。修完 `ARCHITECTURE.md` 与代码对齐，裁决出口恢复可用 |
| 2 | **AG3** | 纯注释，一个 commit。`Engine` 两把锁 + `treeRuntime` 所有权 |
| 3 | **AG4** | 一个未导出 helper 替换 7 处调用并移除依赖 |
| — | **AG5** | 先裁决方案 A / B，再单独成批。不与前三批合并 |

前三批均无破坏性公开 API 变更，可直接实施；每批 `go build && go vet && go test ./...` 全绿后提交。

---

## 四、复核方式

```sh
# 全量验证
cd agent && go build ./... && go vet ./... && go test ./...

# AG1：门禁与文档的包集合差异
grep -n 'agenttest' agent/package_dag_test.go
sed -n '639,660p' agent/doc/ARCHITECTURE.md

# AG2：悬空 ADR 引用
grep -n 'ADR' agent/doc/ARCHITECTURE.md agent/doc/ENGINEERING_STANDARDS.md

# AG3：锁字段无注释
sed -n '76,98p' agent/engine.go

# AG4：依赖点
grep -rn 'samber/lo' --include='*.go' agent/ | grep -v _test

# AG5：持久化链版本字段（当前无输出即为无版本）
grep -rn 'SchemaVersion' --include='*.go' agent/
```
