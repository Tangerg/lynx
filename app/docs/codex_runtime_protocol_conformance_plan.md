# Lyra Runtime API 最终一致性收口计划

> 作者：Codex
> 状态：`A-TRACK DONE`（B1 独立延期）
> 建档日期：2026-07-29
> 审计基线：`main@f4dd8193c`
> 收口基线：A7 原子提交（见 §17）
> 目标协议：`protocol.current = protocol.minSupported = "2026-07-27"`
> 目标 Artifact：`SessionArtifactVersion = 7`

## 0. 文档职责

本文是“冻结协议 → 当前实现”的一致性收口台账，记录：

- 最终目标与不可退让的工程原则；
- 当前已经完成的能力与尚未闭环的偏差；
- 后续实施顺序、每个 slice 的边界和验收标准；
- 进度、证据、决策与风险。

文档职责按以下优先级划分：

1. [`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md)
   是唯一目标契约，回答“最终协议必须是什么”。
2. 本文是当前实施台账，回答“距离目标还差什么、怎么完成、做到哪了”。
3. Contract Registry、生成物和代码是实施事实，证明“当前实际上是什么”。
4. [`VNEXT_IMPLEMENTATION_PLAN.md`](VNEXT_IMPLEMENTATION_PLAN.md)
   保留为上一轮切换过程和历史决策记录；其中的 `DONE` 表示对应实施 slice
   已经交付，不再等同于“冻结契约逐条一致性已经证明”。
5. `app/desktop/docs/protocol/` 描述已交付协议。每个 slice 完成时必须随代码和生成物
   同步更新，禁止长期领先或落后于实现。

若本文与冻结契约冲突，修改本文；若代码与冻结契约冲突，修改代码。不得以当前实现、
已有测试或生成物为理由弱化目标。

---

## 1. 总体判断

新的 API 不是只完成了表面改名，而是已经完成了主体重构：

- Contract Registry 已经统一方法、流、幂等、能力规则和错误声明；
- manifest、JSON Schema、OpenRPC、API Reference、TypeScript wire 类型、method
  map、sample 与运行时 validator 已可生成；
- Session / Run / Segment / Item 四层模型、Run 三态和 durable projection 已落地；
- Run stream、runtime invalidation stream、replay cursor、冷恢复和前端 reducer
  已形成闭环；
- 协议与 Artifact 已执行单版本硬切换，没有保留旧协议 decoder 或 alias；
- runtime 与 frontend 的现有质量门均为绿色。

截至 A7 收口，可以判定为：

> **A-track 最终协议一致性已经闭环；完整 child Run 执行能力仍按 B1 独立延期。**

A1–A7 已把旧实施计划未单独追踪的跨层语义逐项修正，并用 Registry、生成物、
运行时边界、真实 SQLite 生命周期、前端 consumer 和 canonical docs 的交叉证据
完成复核。`features.subagents=false` 仍是有意的诚实能力声明：它不是 A-track
遗留兼容，也不应被写成已交付的完整 child Run 能力。

本计划不使用粗略完成百分比。一个完整的 Run-tree cancel 与一个 `minItems`
约束不能按相同权重计算；只记录逐项状态和可复核证据。

---

## 2. 目标

### 2.1 协议目标

完成冻结协议与以下六层事实的一致性：

```text
冻结语义
  → Contract Registry
  → Go wire / application behavior
  → schema / OpenRPC / manifest
  → TypeScript types / validators / client
  → canonical protocol docs
```

任何一层不同步，都不得把对应 slice 标记为 `DONE`。

### 2.2 人体工程学目标

客户端和 Agent 只需要理解：

```text
Session → Run → Segment → Item
```

- `start` 发起新的用户意图；
- `resume` 继续一个 Waiting Run；
- `steer` 向当前明确的 Segment 注入新的 `ContentBlock[]`；
- `cancel` 取消被寻址的 Run，并同步返回已经提交的状态；
- stream 提供实时体验，query 永远可以恢复权威状态；
- capability 被拒绝时显式返回可行动错误，不静默降级。

不允许客户端理解 executor process、goroutine、内部 registry、journal 存储方式或
为了兼容历史形状而存在的第二套调用方式。

### 2.3 工程目标

- 一个概念只有一个名字、一个 wire shape 和一个事实作者；
- 所有闭合联合、条件必填、非空集合和数值下界均可机器验证；
- 业务行为位于 domain/application，delivery 只负责 wire 翻译；
- 接口由消费方定义，保持小而明确，不为单一实现提前制造抽象；
- 所有长任务和 goroutine 都有清晰所有者、停止路径与 join 边界；
- 错误显式返回、保留因果链，并给客户端稳定的恢复动作；
- 不新增 `utils`、`common`、兼容 adapter 或仅为未来想象准备的扩展层。

### 2.4 明确非目标

- 不兼容旧 protocol version、旧 Artifact 或旧 TypeScript 调用形状；
- 不提供 deprecated alias、双字段、双读写、fallback decoder 或临时 shim；
- 不借协议收口顺手重写无关模块；
- 不在完整 Run-tree 行为落地前把 `features.subagents` 打开；
- 不把“测试当前实现通过”误写成“最终契约已经符合”。

Breaking change 已获授权。发现错误 shape 时直接替换并删除旧表达，不安排迁移期。

---

## 3. 当前基线

### 3.1 机器契约

| 项目 | 当前事实 |
|---|---|
| protocol current / min | `2026-07-27` / `2026-07-27` |
| 已注册方法 | 85 |
| streaming methods | `runs.start`、`runs.resume`、`runs.subscribe`、`runtime.subscribe` |
| runtime topics | 9 个业务 topic + `resync` event |
| Contract 产物 | manifest、schema、OpenRPC、API Reference |
| Frontend 产物 | wire types、method map、samples、validators |
| 工作树 | 基线审计时干净，`main == origin/main` |

### 3.2 已完成能力

以下内容已经通过代码、生成物和测试交叉核实，本计划不重复实现：

| 能力 | 状态 | 说明 |
|---|---|---|
| 单一 method registry 与直接 dispatch | `DONE` | 方法元数据、handler 和生成输入同处 |
| Run 三态与 RunSummary / RunRef | `DONE` | Running / Waiting / Finished |
| durable `runs.get/list` 与 `items.list` | `DONE` | 不再依赖 live registry 作为历史真相 |
| PendingInterruptSet 与 `interrupts.list` | `DONE` | 完整集合分页、能力覆盖检查 |
| typed todos state、revision、cold query | `DONE` | `todos.get` + snapshot fold |
| root Segment subscribe / steer 前置条件 | `DONE` | segment identity 与 stale detection 已有 |
| opaque replay cursor 与有界 retention | `DONE` | process epoch + run/segment + sequence |
| runtime invalidation stream | `DONE` | 九 topic、resync、前端 refetch |
| Error Registry 与 RecoveryAction | `DONE` | 结构化恢复动作已生成 |
| 前端断线恢复与 exhaustive reducer | `DONE` | replay 不可用时走 cold recovery |
| hard cutover / Artifact v7 | `DONE` | 无历史兼容分支 |
| Store receiver 命名一致性 | `DONE` | 当前生产代码中的 `*Store` receiver 统一为 `s` |

### 3.3 基线质量证据

2026-07-29 在 `main@f4dd8193c` 上验证：

```text
MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint
  → all green

cd app/desktop/frontend && npm run check
  → 178 test files passed
  → 1070 tests passed
  → typecheck / lint / format / knip / circular / layers /
    published boundaries / bundle gate 全部通过
```

这组证据证明当前实现稳定、自洽；它不替代下文新增的一致性验收。

### 3.4 当前偏差的代码入口

后续实施优先从事实作者进入，不从生成文件反向修改：

| 偏差 | 当前事实入口 |
|---|---|
| cancel method kind / errors | [`contract_runs.go`](../runtime/internal/delivery/dispatch/contract_runs.go) |
| cancel / steer wire shape | [`runs.go`](../runtime/internal/delivery/protocol/runs.go) |
| cancel / steer server translation | [`runs_control.go`](../runtime/internal/delivery/server/runs_control.go) |
| cancel application boundary | [`usecases.go`](../runtime/internal/application/runs/usecases.go) |
| run event classification | [`events.go`](../runtime/internal/delivery/protocol/events.go) |
| client / server capabilities | [`capabilities.go`](../runtime/internal/delivery/protocol/capabilities.go) |
| Run / Runtime event unions | [`contract_shapes_wire.go`](../runtime/internal/delivery/dispatch/contract_shapes_wire.go) |
| method metadata validation | [`contract.go`](../runtime/internal/delivery/dispatch/contract.go) |
| Registry ownership | [`registry.go`](../runtime/internal/delivery/dispatch/registry.go) |
| 当前生成 TS | [`wire.generated.ts`](../desktop/frontend/src/rpc/wire.generated.ts) |

---

## 4. 偏差总表与进度

状态只使用：

- `TODO`：尚未开始；
- `IN PROGRESS`：当前正在实施；
- `DONE`：目标、实现、生成物、文档和测试全部闭环；
- `DEFERRED`：有明确原因和重新进入条件，不表示完成；
- `BLOCKED`：存在不能由当前实施者消除的外部阻塞。

| ID | 事项 | 当前状态 | 下一动作 | 完成证据 |
|---|---|---|---|---|
| A0 | 当前实现与冻结协议重新审计 | `DONE` | 固化本文 | 本文 §3、§4 |
| A1 | `runs.cancel` response、错误与 root 行为收口 | `DONE` | 2026-07-30 完成 | typed response、exact committed snapshot、全量 gates |
| A2 | `runs.steer` 改为 `ContentBlock[]` | `DONE` | 2026-07-30 完成 | typed command/port、multimodal live+fallback、全量 gates |
| A3 | 删除 `custom.durable`，收紧事件可靠性语义 | `DONE` | 2026-07-30 完成 | type-owned policy、closed opt-out、全量 gates |
| A4 | 收紧 machine-contract value constraints | `DONE` | 2026-07-30 完成 | single Registry metadata、output boundaries、全量 gates |
| A5 | capability gate 与 disabled-subagent seam 收口 | `DONE` | 2026-07-30 完成 | shared policy、durable identity gates、全量 gates |
| A6 | Registry fail-closed 与 SSOT 清理 | `DONE` | 2026-07-30 完成 | defensive views、closed metadata、effective errors、全量 gates |
| A7 | canonical docs 与最终 conformance sweep | `DONE` | 2026-07-30 完成 | §12.4 conformance matrix、全量 gates |
| B1 | 完整 child Run producer / tree cancel / barrier | `DEFERRED` | 独立排期 | 启用条件见 §11 |

A1–A7 已全部完成，当前没有 A-track slice 处于 `IN PROGRESS`。B1 仍是独立项目，
不得通过打开 feature flag 或附加兼容路径并入本轮收口。

---

## 5. A1 —— `runs.cancel` 最终收口

### 5.1 目标 shape

```ts
interface CancelRunRequest {
  runId: RunId;
  reason?: string;
}

type CancelRunResponse =
  | { type: "root"; run: RunRef }
  | { type: "child"; run: RunRef; rootRun: RunRef };
```

`runs.cancel` 已由无结果 `UnaryAck` 改为普通 typed unary；成功 response 是同一
command boundary 实际提交的 durable snapshot，不通过事后查询重新拼装。

### 5.2 实施范围

- 在 protocol 与 UnionSpec 中登记 `CancelRunResponse` 闭合联合；
- registry 将 `runs.cancel` 从 `UnaryAck` 改为 `Unary`；
- application command 返回领域结果，不直接返回 wire 类型；
- server presenter 负责将领域结果翻译成 root / child wire variant；
- live cancel 直接保留 terminal transaction 已提交的 Run snapshot；
- parked cancel 的原子写集直接返回它刚提交的 Run snapshot；
- 同一 Idempotency-Key 重放必须逐字节返回第一次成功 response；
- Finished run 统一返回 `run_finished`；
- 删除 `ErrRunAlreadyDone`、`run_already_finished` 及对应 code/spec/文档；
- 不保留 void overload、旧错误 alias 或客户端兼容包装；
- 同步更新生成物、前端 client 签名和 canonical docs。

child variant 必须进入 schema 和 TypeScript 闭合联合，但 `features.subagents=false`
期间不得伪造 child 成功结果。真实 child transaction 归 B1。

### 5.3 分层约束

- Run 取消、terminal commit、executor teardown 和并发裁决属于
  `application/runs` 与 domain；
- durable Run projection 由 query port 读取；
- `delivery/server` 只做错误与结果翻译；
- `delivery/dispatch` 只声明 method metadata、union 与生成信息；
- 不允许 presenter 补造状态、猜测 root/child 或拼装未提交结果。

### 5.4 验收

- root Running cancel 返回 `type:"root"` 和 Finished(canceled) RunRef；
- root Waiting cancel 原子关闭 Interrupt set，并返回相同提交边界后的 RunRef；
- Finished cancel 返回 `run_finished`，不返回 `run_not_found`；
- 真正不存在的 id 才返回 `run_not_found`；
- cancel 与 terminal、resume 的竞态只有一个 durable 方案获胜；
- response 晚于关键 teardown 与 terminal commit，不等待非关键维护任务；
- request context 取消后，已开始的关键清理仍由有界 owner context 完成；
- idempotency replay 不执行第二次 cancel；
- Go schema validator、TS validator 对 root/child variant 做同构验证；
- 生产代码、当前生成物和 canonical docs 中不再出现协议标识
  `run_already_finished` 或 `ErrRunAlreadyDone`；上一发布版 baseline fixture
  继续原样保留，用于证明本轮 breaking diff。

---

## 6. A2 —— `runs.steer` 使用 `ContentBlock[]`

### 6.1 目标

```ts
interface SteerRunRequest {
  runId: RunId;
  expectedSegmentId: SegmentId;
  input: ContentBlock[];
}
```

### 6.2 实施范围

- 删除 `message: string`，直接替换为 `input: ContentBlock[]`；
- 复用 `runs.start/resume` 已有的 ContentBlock shape、校验与领域转换；
- application command 和 executor consumer 接收 typed content，不在中间层重新压回 string；
- 纯文本 UI 显式构造 text block；多模态输入保持结构化；
- 前端 runtime gateway、ports、测试和调用方同批修改；
- 保留 `expectedSegmentId` 必填和 stale precondition；
- 不增加 `steerText`、string overload 或 `message` alias。

### 6.3 验收

- text、image/file 等协议允许的 ContentBlock 均沿用同一验证规则；
- 空或非法 block 在 wire 边界返回准确的 `invalid_params` field errors；
- Waiting / Finished / stale segment 仍分别返回现有稳定错误；
- 前端不通过 JSON stringify 或拼接文本损失 block 类型；
- 生成 TS 中 `SteerRunRequest` 只有 `input`，不存在 `message`。

---

## 7. A3 —— 事件可靠性语义收口

### 7.1 根问题

冻结契约已经删除 `custom.durable`。A3 前的实现仍允许事件发送方通过布尔字段自称
durable，并在内部将 durable 同时解释成 authoritative、replayable 和 persisted。
这三个概念不能混为一谈：

- authoritative：客户端能否把该事件当作事实；
- replayable：当前 process / Segment 的窗口是否会重放它；
- persisted：事实是否已经进入可跨进程恢复的 projection。

live event 被 replay 不等于 event 本身被持久化。

### 7.2 实施范围

- 从 Go wire、UnionSpec、schema、TS、fixtures 和文档中删除
  `custom.durable`；
- `custom` 固定 non-authoritative、non-replayable，无 SSE id、不进 replay window；
- 删除 `StreamEvent.IsDurable()` / `AlwaysEphemeral()`；journal 与 SSE id 只询问
  `Replayable()`，manifest 独立读取 `Authoritative()`；
- persisted 继续由 application transaction、query 与 store 事实表达，不挂到 event
  policy 上；
- 用 `UnionSpec.Forbidden` 表达已从 Go shape 删除、但开放对象仍必须显式拒绝的
  protocol negative invariant；它不是兼容 decoder；
- `excludedEphemeralEvents` 静态收紧为
  `"item.delta" | "segment.progress"` 的 `SuppressibleRunEventType` 闭合类型；
- 不允许用完整 `StreamEventType[]` 再依赖运行时兜底；
- 同步清理 canonical API / TRANSPORT 中 sender-declared durability 的描述。

### 7.3 验收

- Go wire shape 无 `durable`，schema 与生成 TS validator 对携带该字段的
  `custom` 帧均拒绝；
- `custom` 永不带 SSE id，断线后不被 replay；
- authoritative completion/state 事件仍按冻结表进入 replay window；
- 丢弃全部 ephemeral event 后，客户端仍能通过 authoritative events 或 cold query
  收敛；
- 代码注释不再声称 replay journal 等同于持久化存储。

---

## 8. A4 —— Machine-contract value constraints

### 8.1 需要补齐的约束

| Shape | 最终约束 | 当前偏差 |
|---|---|---|
| `RuntimeEvent.sequence` | `>= 1` | schema 当前允许 0 |
| `resync.topics` | 必填且非空 | 当前可省略或为空 |
| `files.changed.paths` | 必填且非空 | 当前只保证字段存在 |
| runtime scope arrays | 出现时非空 | 多数字段只声明 array |
| `RuntimeLimits.runReplay` | 必填 | 当前 Go/TS/schema 可选 |
| `PendingInterruptSet.interrupts` | 必填且非空 | 当前允许空集合 |
| `ProblemData.requiredCapabilities` | capability error 时必填且非空 | schema 未完整表达 |

### 8.2 实施原则

- 继续复用现有 `FieldConstraintSpec` 机制，不创建第二套输出 validator；
- 将它从“request value constraints”提升为“所有 wire shape 的 value constraints”；
- required、non-empty、positive、unique 和 conditional requirement
  都从同一份 metadata 生成；
- Go validator、JSON Schema 与 TS validator 必须来自同一声明；
- constructor 仍应从结构上生产合法值，但不能用“生产代码目前不会生成非法帧”
  代替边界 validator；
- `RunReplay` 改为必填值类型；composition 构造不完整 capabilities 时应尽早失败，
  不能通过 `omitempty` 隐瞒。

### 8.3 验收

- 每条约束同时拥有合法 fixture 与最小非法 fixture；
- schema 和两侧 validator 对相同 fixture 给出相同结论；
- `resync` 无 topics 或空 topics 均被拒绝；
- sequence 0 被拒绝，首个真实 runtime event 仍为 1；
- capabilities 永远公布实际执行的 replay scope / maxEvents / maxBytes；
- 错误信息包含 shape 与字段路径，例如
  `RuntimeEvent.topics must contain at least one topic`，不只返回笼统的
  `invalid params`。

---

## 9. A5 —— Capability gate 与 disabled-subagent seam

### 9.1 当前边界

当前 composition 诚实广告：

```text
features.subagents.enabled = false
```

因此没有 child Run producer。这个状态可以保持，但不能把“功能关闭”当成“扩展缝已经
证明可用”。

### 9.2 本 slice 必须完成的协议可启用性

- `runs.get(child)` 要求调用方协商 `features.subagents`；
- `runs.cancel(child)` 要求同一能力，并预留 typed child response；
- `items.list` 的 child run scope 与 `includeDescendants:true` 都要求该能力；
- `runs.list.includeDescendants:true` 继续显式拒绝，不静默解释为 false；
- state-dependent gate 在解析出 durable Run identity 后执行；不能强行伪装成只看
  request shape 的静态规则；
- capability 拒绝统一返回 `capability_not_negotiated`，并携带非空
  `requiredCapabilities`；
- `features.subagents` 保持 false，直到 B1 全部验收通过。

### 9.3 验收

- 显式请求 disabled capability 永不返回看似完整的降级结果；
- root-only 请求不被不必要地要求 subagent capability；
- `clientOptIn` 同时约束 dispatcher、state-dependent gate 与 SDK preflight；typed SDK
  对一次调用只读取一次 request metadata，并以同一快照发包；
- method metadata、运行时 state-dependent gate 与 discovery 不相互矛盾；
- future composition 将 feature 打开时，不需要修改公开 shape 或增加新方法。

---

## 10. A6 —— Registry 与命名坏味道清理

### 10.1 目标

让 Contract Registry 不仅能生成当前合同，还能对自身非法状态 fail closed。

### 10.2 实施范围

- `Registry.Names()` 返回 clone，不暴露内部可变 slice；
- `MethodKind`、`IdempotencyPolicy`、`ConditionOperator`、`ConstraintKind`
  等闭合枚举显式验证边界；
- 未知枚举的 `String()` 不得伪装成合法的 `unary`、`none` 或 `present`；
- 可声明的 method problem types 从 Error Registry 的明确属性派生，删除容易漂移的
  第二份手写名字表；
- 错误信息包含 method、field、operator 和非法值，便于直接定位注册错误；
- receiver、局部变量、方法与文件名按职责命名，不使用含义模糊的
  `base`、`data`、`info`、`helper`；
- 不因为只有一个实现就新增接口，不把生成器需求反向泄漏进运行时层。

### 10.3 验收

- 修改 `Names()` 返回值不会影响 Registry；
- 任一未知 metadata enum 在启动/生成阶段确定性失败；
- 新增 RPC business error 只需在一个 registry 事实源登记 wire behavior；
- lint 不再出现 receiver 命名不一致或错误信息不可读的告警；
- architecture tests 继续保证 delivery/application/domain 依赖方向。

---

## 11. B1 —— 完整 child Run 能力

### 11.1 定位

B1 是独立的功能实现轨道，不是 A-track 核心协议一致性的兼容补丁。

在 `features.subagents=false` 时，A-track 可以完成并保持协议诚实；但若要达到冻结契约
定义的完整顶级功能，必须继续完成 B1，不能仅把 feature flag 改为 true。

### 11.2 必需行为

- child Run 有 durable `parentRunId`、`rootRunId`、`spawnedByItemId`；
- 每个 child Run 拥有自己的 Segment 与 Item 时间线；
- root PendingInterruptSet 聚合完整 tree，但每个 Interrupt 保留 source `runId`；
- root cancel 赢得 tree cancellation boundary，停止并 join 全部 active descendants；
- Running child cancel 只终止目标 subtree，并向父 tool-call 提交
  `child_run_canceled`；
- Waiting child cancel 原子关闭目标 subtree Interrupt：
  - root set 仍非空：surviving tree 继续 Waiting；
  - root set 为空：所有 surviving suspended Runs 从 durable continuation
    创建新 Segment 并恢复 Running；
- child cancel response 同步返回 target child 和同一提交边界后的 root snapshot；
- resume / root cancel / child cancel / terminal boundary 在同一 root tree 上串行化；
- 发布顺序、quiescence、父 tool-call commit 和关键 teardown 都有明确 owner 与 join。

### 11.3 启用门槛

只有以下项目全部通过，才能把 `features.subagents.enabled` 改为 true：

- Running child cancel；
- Waiting child cancel：有剩余 Interrupt / 无剩余 Interrupt 两条路径；
- root cancel 与 child cancel 竞态；
- child terminal 与 root cancel 竞态；
- subtree 不再发布事件的 quiescence 证明；
- parent tool-call `child_run_canceled` 结构化结果；
- child get/list/items/subscribe 的 capability 与 profile 覆盖；
- process restart 后 durable tree query；
- Go race tests 与多分支集成测试；
- 前端 reducer 可折叠 root stream 中不同 source Run 的事件。

不新增 `runs.cancelChild`，不通过“只改 child 状态”冒充取消完成。

---

## 12. A7 —— 最终收口

### 12.1 Conformance sweep

逐项比较：

- 冻结契约；
- Registry metadata；
- Go wire types；
- application behavior；
- schema / OpenRPC / manifest / API Reference；
- TS types / validators / client；
- canonical API / AUX_API / TRANSPORT；
- frontend reducer 与 recovery behavior。

重点执行目标化残留扫描：

```text
runs.cancel 仍为 UnaryAck
run_already_finished / ErrRunAlreadyDone
SteerRunRequest.message
StreamEvent.durable / custom.durable
StreamEvent.IsDurable
RuntimeLimits.runReplay 可选
RuntimeEvent.resync.topics 可选
excludedEphemeralEvents: StreamEventType[]
```

这些字符串只在迁移说明或历史文档中出现时可以保留；生产代码、生成物和 canonical
协议中必须消失。

### 12.2 全量验证

最低验证集：

```text
MODULE=app/runtime scripts/check.sh build vet test lint vuln
cd app/runtime && go test -race ./internal/application/runs/... ./internal/delivery/...
cd app/desktop/frontend && npm run check
```

另外必须验证：

- contract generation 可重复，第二次生成无 diff；
- baseline compatibility differ 正确识别本轮 breaking shape；
- protocol version、schema、samples、canonical docs 中的版本一致；
- 没有加入旧字段 decoder、alias、fallback 或双写；
- fresh store 可以完成 start → steer → wait/resume → cancel → cold recovery；
- 工作树仅包含本轮有意修改。

### 12.3 完成定义

A-track 只有同时满足以下条件才能标记 `DONE`：

1. A1–A7 全部 `DONE`；
2. 冻结契约与六层事实没有已知偏差；
3. 所有生成物由 Registry/shape metadata 产生，不手改生成文件；
4. runtime 与 frontend 全量门禁通过；
5. canonical docs 描述实际交付行为；
6. 没有任何历史兼容路径；
7. `features.subagents=false` 时所有相关意图显式拒绝；
8. 若 B1 未完成，台账必须继续将其标为 `DEFERRED`，不得写成完整功能已交付。

### 12.4 最终 conformance matrix

| 事实面 | 核对内容 | 结论 / 证据 |
|---|---|---|
| 冻结契约 | `codex_runtime_protocol_vnext_final.md` 的方法、shape、能力、错误与版本 | `PASS` |
| Registry / Go wire | method、union、presence、value、feature、error metadata 与 DTO | `PASS`；Interrupt/set id 非空约束进入 Registry |
| application / delivery | start → steer → wait/resume → cancel；presenter 与 durable projection | `PASS`；真实 composition root + fresh SQLite + cold restart |
| wire boundaries | request、response、Problem、RunEvent、RuntimeEvent 的嵌套 DTO | `PASS`；统一 `ValidateWireTree` 递归组合生成规则 |
| artifacts | manifest、schema、OpenRPC、API Reference、Go validator | `PASS`；二次生成无 diff |
| TypeScript / client | wire type、validator、method map、SDK metadata 与 preflight | `PASS`；SDK 测试使用生成的 `PROTOCOL_VERSION` |
| canonical docs | API / AUX_API / TRANSPORT 与目标协议版本、行为 | `PASS`；均为 `2026-07-27` |
| compatibility | 上一发布 baseline → 当前产物 | `PASS`；70 breaking / 95 compatible，版本 bump gate 通过 |
| 历史债务扫描 | alias、旧 decoder、fallback、dual read/write、旧字段 | `PASS`；仅历史 baseline、拒绝旧 shape 的 negative fixture 和迁移说明保留 |
| disabled capability | `features.subagents=false` 下的静态与 durable-identity intent | `PASS`；全部显式 `capability_not_negotiated` |
| 全量质量门 | runtime build/vet/test/lint/vuln、race、frontend check | `PASS` |

残留字符串扫描的允许项是封闭的：

- `run_already_finished`、旧协议版本只存在于上一发布 baseline 和明确的版本拒绝测试；
- `SteerRunRequest.message`、`StreamEvent.durable` 只存在于“必须拒绝旧 shape”的
  schema / TS negative fixture；
- canonical docs 中 `custom.durable` 只用于声明该字段被禁止；
- 生产代码、当前生成物和正向 sample 中不存在这些旧表达。

---

## 13. Slice 执行纪律

每个 slice 遵循同一顺序：

1. 将本表对应项改为 `IN PROGRESS`，记录开始日期和目标 commit；
2. 先增加能证明冻结语义的失败测试或 contract fixture；
3. 从事实所有者所在层实施根因修复；
4. 更新 Registry / shape metadata；
5. 重新生成全部 contract 与 frontend artifacts；
6. 修改客户端和 canonical docs；
7. 跑该 slice 的 targeted tests；
8. 跑 runtime 与 frontend 全量门禁；
9. 在本文记录 commit、测试命令、关键裁决和残余风险；
10. 只有证据完整后改为 `DONE`。

每个 slice 必须独立可审查、可 bisect、全绿。不把多个互不依赖的 breaking change
压成无法定位的巨型提交，也不为了提交变小而把一个原子语义拆成暂时说谎的中间状态。

若 persistence shape 改变：

- dev store 直接 bump epoch / fresh rebuild；
- 不写 migration；
- 不做旧行回填；
- 不保留 dual-read / dual-write。

---

## 14. 分层与命名守则

| 层 | 应当拥有 | 不应拥有 |
|---|---|---|
| `delivery/protocol` | wire request/response/event/error shape | 事务、存储查询、executor 生命周期 |
| `delivery/dispatch` | method registry、union、constraints、生成 metadata | 业务裁决、durable projection 拼装 |
| `delivery/server` | wire ↔ application 翻译、错误映射 | Run 状态机、tree cancel、恢复策略 |
| `application/runs` | command orchestration、admission、owner context、join | JSON 字段、SSE、TS 形状 |
| `domain/execution` | Run/Segment/Interrupt 不变量与状态转换 | transport capability 表达 |
| query/store ports | durable read/write 语义 | presenter 需要什么就临时补什么 |
| frontend rpc | generated wire types 与传输调用 | 手写第二份协议联合 |
| frontend application | 用户意图、reducer、cold recovery | 数字错误码镜像、transport 内部状态 |

命名以领域含义为准：

- wire 使用冻结契约中的 `runId`、`expectedSegmentId`、`input`；
- application 可以使用 `CancelCommand`、`SteerCommand`、`CancelResult` 等领域名；
- replay 判断命名为 `Replayable`，不再借用含混的 `Durable`；
- 同一 receiver 全包统一，`Store` 使用 `s`；
- 错误应写成“操作 + 对象 + 原因”，并使用 `%w` 保留可判断的因果链；
- 不使用 `Manager`、`Helper`、`Data`、`Info` 等不能说明职责的泛化名，除非它们在
  领域中确实有稳定含义。

---

## 15. 决策记录

| 决策 | 内容 | 状态 |
|---|---|---|
| C1 | 冻结契约是目标真相，当前实现不是反向修改契约的理由 | `ACCEPTED` |
| C2 | breaking change 直接替换，不留 compatibility debt | `ACCEPTED` |
| C3 | 现有绿色 gates 证明内部自洽，不等于冻结协议 conformance | `ACCEPTED` |
| C4 | `runs.cancel` typed response 与 `run_finished` 是首个收口 slice | `ACCEPTED` |
| C5 | event reliability 三分：authoritative / replayable / persisted；`custom` 固定前两者为 false，persisted 不属于 event flag | `ACCEPTED` |
| C6 | output/event value constraints 与 request constraints 使用同一机制 | `ACCEPTED` |
| C7 | subagent 拆成协议可启用性 A5 与真实能力 B1，不用 flag 掩盖进度 | `ACCEPTED` |
| C8 | 生成物、实现、前端与 canonical docs 必须在同一 slice 更新 | `ACCEPTED` |
| C9 | 不新增仅有单实现或未来假设支撑的接口/包/抽象 | `ACCEPTED` |
| C10 | cancel response 不做 post-commit re-query；直接返回获胜事务提交的 snapshot | `ACCEPTED` |
| C11 | parked cancel 先取得 Session admission，再解析 Interrupt，以同一 gate 串行化 resume | `ACCEPTED` |
| C12 | steer 的 live drain 与 terminal fallback 共用同一条 canonical queue；fallback 同时发布 transcript Item 并写 conversation history | `ACCEPTED` |
| C13 | 客户端 opt-out 由 `SuppressibleRunEventType` 闭合集合表达，不用完整 event enum 加运行时兜底 | `ACCEPTED` |
| C14 | `UnionSpec.Forbidden` 只表达当前协议的负面不变量，不承担历史兼容解码 | `ACCEPTED` |

---

## 16. 风险台账

| 风险 | 影响 | 控制 |
|---|---|---|
| cancel response 在 terminal commit 前构造 | 返回过时或非法 RunRef | write-set 返回/保留 exact committed snapshot + 集成测试 |
| cancel 与 terminal/resume 并发双提交 | 重复 terminal、丢 Interrupt 或错误 response | root/tree admission + CAS + race tests |
| idempotency 只缓存 ack、不缓存 typed result | 同 key 得到不同状态 | 缓存首个完整 response |
| ContentBlock 在 application 中退化为 string | 多模态语义丢失 | 端到端 typed command/port |
| 把 replay window 称作持久化 | 客户端错误依赖内存事件 | `Replayable` 命名 + cold query 测试 |
| output constraints 仍只存在于注释 | 生成 validator 接受非法 frame | 单一 constraint metadata + 双侧 fixtures |
| state-dependent capability 被静态 shape gate 冒充 | child id 可能绕过协商 | durable identity resolution 后显式检查 |
| 为打开 feature 只改布尔值 | 暴露未完成 child 语义 | B1 启用门槛测试 |
| 顺手重构扩大爆炸半径 | 难审查、难回滚 | slice 边界与 targeted diff |

---

## 17. 进度更新模板

### 2026-07-30 — A1

- 状态：`DONE`
- Commit：本记录所在的 `feat(runtime): finalize typed runs.cancel contract` 原子提交
- 目标：把 `runs.cancel` 从空 ack 收口为返回 exact committed Run snapshot 的 typed unary。
- 关键裁决：
  - live terminal transaction 成功后，由 run handle 保留同一份 committed snapshot；
  - parked cancel write-set 直接返回刚提交的 terminal Run，不做事后 query；
  - cancel 先读 durable Run，准确区分 unknown / Finished / Waiting / Running / child；
  - parked 路径在解析 open Interrupt 前取得 Session admission，与 resume 共用一个
    串行化边界；
  - child shape 已发布但 feature 仍关闭；未协商的 child intent 返回带
    `features.subagents` 缺口的 `capability_not_negotiated`；
  - 删除 `run_already_finished`、旧 code 和 sentinel，不保留 alias；
  - 质量门发现新披露的两个可修复依赖漏洞后，直接升级到
    `golang.org/x/text v0.39.0`、`google.golang.org/grpc v1.82.1`，不加入 allowlist。
- 生成物：
  - `manifest.json`、`schema.json`、`openrpc.json`、`API_REFERENCE.md`；
  - `wire.generated.ts`、`wire.methods.generated.ts`、
    `wire.validate.generated.ts`。
- 验证：
  - targeted application / sessions / server / dispatch / HTTP / arch tests → `PASS`
  - `go test ./...` → `PASS`
  - `go test -race ./...` → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - contract generation 第二次执行 hash 不变 → `PASS`
- 残余风险：
  - child producer、tree transaction 和 child response 的真实生产路径仍属于 B1；
  - `run.rootRunId == rootRun.id` 等跨对象 value constraint 继续由 A4 收口；
  - 上一发布版 baseline fixture 有意保留旧 error，用于 breaking compatibility differ，
    不是运行时兼容路径。

### 2026-07-30 — A2

- 状态：`DONE`
- Commit：本记录所在的 `feat(runtime): carry structured content through runs.steer` 原子提交
- 目标：将 `runs.steer` 从裸 string 硬切换为端到端保真的
  `input: ContentBlock[]`。
- 关键裁决：
  - wire 只保留 `{runId, expectedSegmentId, input}`，删除 `message`，不提供 alias
    或 string overload；
  - application command、consumer-owned `TurnControl` port 与 executor adapter
    均传递 canonical `[]transcript.ContentBlock`；
  - `MaterializeUserMessage` 成为 start/steer 共用的唯一 MIME、base64 与
    provider-neutral user message 转换；steer 直接保留 block 顺序，start 在同一次
    验证后投影为 opening prompt + image attachments；
  - turn queue 同时持有 canonical content 与一次验证后的 provider-neutral message；
    live drain 与 terminal fallback 互斥消费同一队列；
  - terminal fallback 不再只写模型 history：它同时发布 `SteerMessage`，从而生成
    durable user Item 并让前端乐观气泡完成对账；
  - nested block 与媒体错误携带精确字段路径，例如 `input[0].data`，统一成为
    structured `invalid_params`；
  - `start.input`、可选但出现的 `resume.input`、`steer.input` 均由 Registry 生成
    `minItems:1`；Go validator 同时从 DTO 的 `omitempty` 派生 requiredness，
    必填数组缺失报 `is required`、已发送空数组报 `must not be empty`，不让运行时
    比 schema / TS 更宽松；
  - frontend application port 保留 `AgentInput`，只在 runtime gateway adapter
    转换 wire block；typed SDK 的 `Promise<void>` 也不再泄漏底层 ack `{}`。
- 生成物：
  - `schema.json`、`openrpc.json`、Go request validator；
  - `wire.generated.ts`、`wire.validate.generated.ts`。
- 验证：
  - targeted application / agent adapter / conversation / server / dispatch tests
    → `PASS`
  - structured text+image live/fallback test 连续 20 次 → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/runtime && go test -race ./...` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1074 tests PASS`
  - contract generation 第二次聚合 hash
    `267f6f24612d2437bc7b09fc996153de6c69c5840df2d38bce9b462b1a54acfc`
    不变 → `PASS`
- 残余风险：
  - 当前 `ContentBlock` 闭合联合只包含 text/image；新增 file 必须作为独立协议
    设计进入 Registry，不能复用 image 字段或塞 opaque JSON；
  - 事件 reliability 术语与 `custom.durable` 的剩余债务进入 A3。

### 2026-07-30 — A3

- 状态：`DONE`
- Commit：本记录所在的 `refactor(runtime): make run event reliability type-owned`
  原子提交
- 目标：删除 sender-controlled `custom.durable`，把 authoritative、replayable
  与 persisted 三种保证从 shape、命名和执行路径上彻底分开。
- 关键裁决：
  - `StreamEvent` 不再携带任何 reliability flag；`custom` 固定
    non-authoritative、non-replayable，未知 event type 同样 fail closed；
  - `Authoritative()` 只生成 `manifest.runEventPolicy` 的客户端折叠事实；
    `Replayable()` 只控制 application Journal retention 与 HTTP SSE id；
    persisted 只由 projection commit、query 和 store 表达；
  - application Journal 的 `Durable()`、`queuedDurable` 及测试词汇全部改为
    `Replayable()` / `queuedReplayable`，不再把进程内窗口称作持久化存储；
  - `excludedEphemeralEvents` 的 Go/Schema/TS 类型收紧为
    `SuppressibleRunEventType = "segment.progress" | "item.delta"`；`custom` 的
    namespaced payload 不能被按共享 envelope tag 整类屏蔽；
  - `UnionSpec.Forbidden` 成为 Registry 的协议级负面不变量：`durable` 已从 Go
    shape 删除，schema 与生成 TS validator 仍对所有 StreamEvent variant 显式拒绝，
    且 Registry 校验空名、嵌套名、重复名和仍存在于 DTO 的字段；
  - canonical API / TRANSPORT 和 frontend recovery 注释统一使用
    authoritative、replayable、persisted，不再以 durable 同时指代三种语义；
  - 不保留旧字段、alias、fallback decoder 或双重 reliability policy。
- 生成物：
  - `manifest.json`、`schema.json`、`API_REFERENCE.md`；
  - `wire.generated.ts`、`wire.validate.generated.ts`。
- 验证：
  - targeted protocol / dispatch / Journal / transport / schema / TS validator tests
    → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/runtime && go test -race ./...` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1075 tests PASS`
  - contract generation 第二次聚合 hash
    `79613f99bc095a62536e42744724cd0eae78ff49e26b40bfe12e0caf9ee6200a`
    不变 → `PASS`
- 残余风险：
  - authoritative 与 replayable 当前核心表恰好同值，但 manifest 分列、实现分函数，
    后续不得因当前值相等重新合并；
  - runtime event output 的非空集合、正数与 conditional constraints 进入 A4。

### 2026-07-30 — A4

- 状态：`DONE`
- Commit：本记录所在的 `feat(runtime): enforce wire constraints across boundaries`
  原子提交
- 目标：把 request-only validation 提升为双向 wire contract，并让输出的正数、非空、
  唯一、联合与条件 presence 约束由同一 Registry 事实生成和执行。
- 关键裁决：
  - `request_constraints*` 被硬切换为 `wire_constraints*`；
    `WireValidator.ValidateWire()` 同时服务请求 decode 和输出边界，不保留旧接口、
    `Validate()` alias 或兼容文件；
  - `FieldConstraintSpec`、`UnionSpec`、`ObjectConstraintSpec` 共同生成 Go
    `ValidateWire`、JSON Schema 与 TypeScript validator；新增架构门禁证明每个注册
    shape 都有 Go validator，manifest/API Reference 直接发布 `valueConstraints`；
  - `ConstraintError` 保留客户端可寻址的相对 `FieldError.field`，同时在诊断文本中
    加入 `Shape.field`；manual Run input 错误显式包裹 `ErrInvalidParams`，输出 shape
    错误不再伪装成客户端参数错误；
  - `RuntimeEvent.sequence >= 1`；`files.changed.paths` 与 `resync.topics` 必填非空；
    所有 narrowing array 出现时非空且无重复；联合 variant 继续禁止无关字段；
  - `PendingInterruptSet.interrupts` 必填非空；
    `capability_not_negotiated.requiredCapabilities` 必填、非空、按 `{type,name}` 唯一；
  - `RuntimeLimits.runReplay` 改为必填值，server 构造时直接读取并验证必需 coordinator
    的 retention；删除“coordinator 可缺失所以 discovery 可省略”的虚假分支；
  - malformed runtime invalidation 不会被 encoder 静默丢弃：hub 将其扩大为该订阅
    全部 topic 的合法 `resync`；畸形 structured problem fail closed 为
    `internal_error`；PendingInterruptSet presenter 则显式失败；
  - capability gate 必须构造 typed `CapabilityGap`，不能只包
    `ErrCapabilityNotNeg` 却遗漏 requiredCapabilities；本 slice 的输出校验把这条旧
    谎言转成红灯并治本修复；
  - TypeScript `oneOf` 在 tag 已匹配时优先报告该 variant 的字段错误，避免
    `{type:"resync"}` 错报成 `expected "skills.changed"`；
  - 删除 runtime.subscribe 对 non-empty / unique topics 的手写重复判断，单一规则
    由生成 request validator 拥有；不保留旧协议 decoder 或兼容 shim。
- 生成物：
  - `manifest.json`（新增 `valueConstraints`）、`schema.json`、
    `API_REFERENCE.md`；
  - `wire.generated.ts`、`wire.validate.generated.ts`；
  - `wire_constraints.generated.go`。
- 验证：
  - targeted protocol / dispatch / server / arch / AJV / TS validator tests
    → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/runtime && go test -race ./...` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1076 tests PASS`
  - contract generation 第二次聚合 hash
    `e1d97877fe8e111b053281fb1a464d6eb7e3904299bd9a875b0d9ecc32f6fd4f`
    不变 → `PASS`
- 残余风险：
  - capability 的 state-dependent seam 与 disabled subagent 路径进入 A5；
  - Registry fail-closed 与剩余 SSOT 审计进入 A6。

### 2026-07-30 — A5

- 状态：`DONE`
- Commit：本记录所在的 `feat(runtime): unify capability gates across request boundaries`
  原子提交
- 目标：让静态请求门禁、durable identity 门禁与 SDK preflight 使用同一能力判定，
  并证明 subagent 关闭时不返回伪完整降级结果、root emergency stop 不被误伤。
- 关键裁决：
  - `protocol.MissingFeatureRequirements` 成为 server support + per-request
    `clientOptIn` 的共同判定；dispatcher 与 server state-dependent seam 不再各写一套
    “enabled”逻辑，所有拒绝均构造带非空 `requiredCapabilities` 的 typed
    `CapabilityGap`；
  - `runs.get(child)` 与 `items.list` 的 child run scope 在读取 durable Run 后才要求
    `features.subagents`；root 读保持 Minimal profile 可用，session item scope 始终返回
    包含 child item 的完整恢复历史；
  - `runs.cancel` 不再先协商整份 Run profile：application 先解析 durable target，
    root cancel 永远可用，child cancel 才使用 `AllowChildRun` 拒绝未协商调用方；
  - shape-dependent `includeDescendants:true` 仍由 Registry capability rule 在 dispatch
    前拒绝；不存在的资源由 durable lookup 返回 not-found，不用参数外形猜身份；
  - typed SDK preflight 同时读取 discovery 的 `clientOptIn` 与本请求声明；一次调用只
    解析一次动态 request metadata，并把同一快照用于预检与发包。未由 typed factory
    拥有 metadata 时不覆盖底层 `RpcClient` 的 provider；
  - `features.subagents.enabled` 继续诚实保持 false；本 slice 只打通未来启用所需 seam，
    不用 feature flag 冒充 B1 producer 已完成；
  - state-dependent / Run-profile capability errors 已进入 method metadata 与生成
    OpenRPC；静态 capability rule 的 effective-error 派生统一留给 A6 的 Registry
    SSOT 收口。
- 生成物：
  - `manifest.json`、`openrpc.json`、`API_REFERENCE.md`；
  - canonical `API.md` 与 typed SDK capability preflight。
- 验证：
  - capability policy / dispatch / server targeted tests → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/runtime && go test -race ./...` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1078 tests PASS`
  - contract generation 第二次聚合 hash
    `8e0da223d597d76936cb915ebec31f369a05109f5a1a1b2dd38c9864ac293c3c`
    不变 → `PASS`
- 残余风险：
  - effective method errors 与 error registry 的 SSOT 派生、metadata enum fail-closed、
    Registry clone/命名坏味道进入 A6。

### 2026-07-30 — A6

- 状态：`DONE`
- Commit：本记录所在的 `refactor(runtime): make contract registries fail closed`
  原子提交
- 目标：让 Contract / Shape / Feature / Error registries 对非法 metadata
  fail closed，并让所有生成制品消费同一份 effective method contract。
- 关键裁决：
  - `MethodKind`、`IdempotencyPolicy`、`ConditionOperator`、`ConstraintKind`、
    `Stability` 与 `RecoveryAction` 都有闭合边界；未知值的 `String()` 输出带类型和值的
    诊断，不再伪装成 `unary` / `none` / `present` / `nonEmpty`；
  - method name、kind、retry、stability、problem type、capability condition field /
    operator / value / feature 在注册时统一验证，错误点名 method、JSON field、非法值与
    合法集合；
  - union、presence rule、value constraint、state key、carried shape 注册拒绝重复
    spec、重复 tag/field/condition、required+optional/forbidden 冲突、未知
    constraint/scope/writer/stability/feature；生成器的 switch 同样在不可达默认分支
    panic，不把未知 metadata 当默认值；
  - `Registry.Names/Metas/Lookup`、所有 Shape views 与 `protocol.Features()` 返回深度
    足够的 defensive snapshot；dispatcher 使用 package-private immutable lookup，
    不为每个请求付出公开快照分配；
  - RPC Error Registry 从无序 map 改为经过自校验的有序 specs：problem type、numeric
    code、recovery action 与 retryAfter 组合唯一且一致，多 sentinel error chain 的
    wire 解析顺序确定；
  - `methodDeclarable` 成为 Error Registry 内的明确属性，删除
    `knownProblemTypes` 第二份名单；`MethodMeta.ProblemTypes()` 统一派生
    `Errors + static CapabilityRules => capability_not_negotiated`，manifest、OpenRPC、
    API Reference 与 error registry 的 method 集全部消费它；
  - 命名审计将 `base` / `helper` 等模糊局部名替换为
    `commonItemFields` / `genericName` / `unionSchema` / `validatorName`；
    store receivers 经 lint 与仓库扫描保持同一类型统一命名。
- 生成物：
  - `manifest.json`、`openrpc.json`、`API_REFERENCE.md` 补齐所有静态
    capability refusal；
  - canonical `API.md` 补充 Registry 自校验与 effective-error SSOT 规则。
- 验证：
  - Registry / Shape / Error / contractgen red-to-green tests → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `cd app/runtime && go test -race ./...` → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1078 tests PASS`
  - contract generation 第二次聚合 hash
    `608a7618f423891768ac1fc3fac77210deab79fa47abe62716952485373fff81`
    不变 → `PASS`
- 残余风险：
  - canonical docs、生成制品与完整实现面的最终交叉审计进入 A7；
  - child Run producer 仍按计划留在独立 B1，不在 A-track 伪装启用。

### 2026-07-30 — A7

- 状态：`DONE`
- Commit：本记录所在的 `feat(runtime): enforce end-to-end wire conformance`
  原子提交
- 目标：完成冻结契约、Registry、producer/consumer、生成制品、canonical docs 与
  冷启动生命周期的最终交叉审计，消除最后的跨层协议漏检。
- 关键裁决：
  - residual scan 逐项排除 `UnaryAck cancel`、旧 finished error、steer message、
    sender-controlled durable、可选 replay/resync 和宽泛 opt-out；历史 baseline、
    negative fixture 与“明确禁止”文档不属于兼容路径，保留其证明职责；
  - fresh SQLite 生命周期测试使用真实 persistence bundle、composition root、
    delivery server 和可控 model，连续完成 start → structured steer → waiting
    question → resume → live cancel → process close/reopen → startup recovery →
    `runs.get/items.list` cold read；
  - 该测试发现 `transcript.Interrupt.RunID` 已持久化但 presenter 未投影。唯一映射点
    补齐 `runId`，并在 query presenter test 固定 child source Run 语义；
  - 根因不是单个字段，而是 generated `ValidateWire()` 有意只校验本 DTO，边界却曾
    把它当成整棵 wire value。新增 `ValidateWireTree` 递归组合 Registry 生成的节点
    规则，并统一用于 request、response、Problem、RunEvent、RuntimeEvent 与 server
    output seam；`any` extension payload 保持 opaque，不误把实现类型变成协议；
  - `Interrupt.itemId/runId` 与 `PendingInterruptSet.rootRunId/sessionId` 的非空约束
    进入 Registry，Go validator、schema `minLength`、API Reference、manifest 与 TS
    validator 同源生成，不在 presenter 手写第二份规则；
  - SDK metadata 测试删除旧版本字面量，直接消费 generated `PROTOCOL_VERSION`；
    版本拒绝测试仍有意使用上一版本。
- 生成物：
  - `manifest.json`、`schema.json`、`API_REFERENCE.md`；
  - `wire.validate.generated.ts`、`wire_constraints.generated.go`。
- 验证：
  - `go test ./...` → `PASS`
  - `MODULE=app/runtime scripts/check.sh build vet test lint vuln` → `PASS`
  - `go test -race ./internal/application/runs/... ./internal/delivery/... ./internal/bootstrap`
    → `PASS`
  - `cd app/desktop/frontend && npm run check` → `178 files / 1078 tests PASS`
  - compatibility differ → `70 breaking / 95 compatible`，version bump gate `PASS`
  - contract generation 连续两次聚合 hash
    `ecf9ff3ce7ea6be4e68e719bdc53938a90e991444a4bce07cf6602ed7b64bdda`
    不变 → `PASS`
- 残余风险：
  - A-track 无已知 conformance 偏差；
  - 完整 child Run producer、tree cancel transaction 与 barrier 仍只属于 B1；
    `features.subagents` 在 B1 全部门槛满足前继续保持 false。

每完成一个 slice，在 §4 表格填写完成证据，并追加一条记录：

```md
### YYYY-MM-DD — A?

- 状态：DONE
- Commit：<sha>
- 目标：<一句话>
- 关键裁决：<实施中出现且已经确定的语义>
- 生成物：<manifest/schema/OpenRPC/TS 等>
- 验证：
  - `<command>` → PASS
  - `<command>` → PASS
- 残余风险：无 / <明确进入哪个后续 slice>
```

不得只记录“测试通过”。记录必须足以让下一次会话判断：

- 改了哪个事实作者；
- 为什么这样改；
- 哪些生成物受影响；
- 哪个验收项证明语义已成立；
- 是否仍有明确未完成项。

---

## 18. 下一步

A-track 已收口，没有自动开始的下一 slice。后续若决定交付完整 subagent 执行能力，
按独立项目进入：

```text
B1 — 完整 child Run producer / tree cancel / barrier
```

B1 必须从 §11 的启用门槛开始，保持 `features.subagents=false` 直到 producer、
tree transaction、interrupt barrier、cold recovery、前端 tree reducer 和完整 gates
同时成立。仍采用 breaking-first 策略，不增加过渡 alias、兼容 decoder 或双写路径。
