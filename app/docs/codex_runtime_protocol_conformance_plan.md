# Lyra Runtime API 最终一致性收口计划

> 作者：Codex
> 状态：`A-TRACK DONE / B1.4 DONE · B1.5 IN PROGRESS`
> 建档日期：2026-07-29
> 审计基线：`main@f4dd8193c`
> 收口基线：A7 原子提交（见 §17）
> 当前已提交基线：`main@6460bede9`；W3.2 随本原子 slice 完成，W3.3 READY
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

> **A-track 最终协议一致性已经闭环；完整 child Run 执行能力已进入 B1 独立实施轨道。**

A1–A7 已把旧实施计划未单独追踪的跨层语义逐项修正，并用 Registry、生成物、
运行时边界、真实 SQLite 生命周期、前端 consumer 和 canonical docs 的交叉证据
完成复核。B1.1 已补齐 Run tree 的 durable identity，但
`features.subagents=false` 仍是有意的诚实能力声明：producer、barrier、tree cancel、
cold recovery 与前端 tree reducer 全部门槛通过前，不得写成完整 child Run 能力已交付。

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
| B1 | 完整 child Run producer / tree cancel / barrier | `IN PROGRESS` | B1.5 / W3.3 restart / cold recovery conformance | B1.1–B1.4、W3.0–W3.2 已完成；启用条件见 §11 |

A1–A7 已全部完成，当前没有 A-track slice 处于 `IN PROGRESS`。B1 保持独立项目，
已按 breaking-first 策略开始实施；不得通过打开 feature flag、复用 root identity
冒充 child，或附加兼容路径缩短门槛。

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

### 11.4 实施切片

切片顺序服从事实依赖：没有 durable tree identity 就不能定义 subtree transaction；
没有 first-class child producer 就无法证明 barrier/cancel；没有 cold recovery 与前端
consumer 闭环就不能诚实发布 capability。

| Slice | 边界 | 状态 | 完成定义 |
|---|---|---|---|
| B1.1 | durable Run-tree identity 与 root admission | `DONE` | 三条 child edge 单一领域不变量、SQLite epoch 41、root-only active index、root-owned profile、Artifact/tree validation |
| B1.2 | first-class child Run producer 与 source routing | `DONE` | causal identity、acknowledged opening、独立 Segment/Item/metrics、递归/并发/失败 conformance 全部闭环 |
| B1.3 | tree interrupt barrier 与 resume | `DONE` | barrier、整树 resume、restart / race / failure / ordering conformance 全部闭环 |
| B1.4 | unified root/child cancellation | `DONE` | root/child arbiter、Running/Waiting subtree、failure/restart/query/race/hygiene conformance 全部闭环 |
| B1.5 | durable query、subscribe 与 cold recovery | `IN PROGRESS` | descendant paging、child/subtree items、root stream replay scope、restart tree recovery；child subscribe 必须拒绝 |
| B1.6 | frontend Run-tree consumer | `TODO` | reducer 按 source Run fold、树折叠/状态/取消交互、cold refetch 与 replay 恢复 |
| B1.7 | conformance sweep 与 capability enablement | `TODO` | §11.3 全部门槛、race/多分支集成/生成物/docs 全绿后才将 feature 改为 true |

B1.1 只建立后续行为不可绕开的权威事实，不把 synthetic child persistence 写成执行能力。
B1.2 起仍须保持 capability 关闭；任何 slice 失败都不得通过把 child event 重新归到 root
或只修改一行状态来降级“完成”。

B1.2 内部原子切片：

| Slice | 状态 | 边界 |
|---|---|---|
| B1.2a | `DONE` | AgentTool child 的 durable `SpawnCallID`、Process/Snapshot/Event/adapter causal projection、provider call → canonical parent Item 精确映射 |
| B1.2b1 | `DONE` | application-owned executor source envelope；Process causal identity 端到端保真；delta 只在同 source 内合并；未准入 child fail closed |
| B1.2b2 | `DONE` | acknowledged child opening；parent running Item + child Run admission 同事务；commit 成功前 child 不执行 |
| B1.2c | `DONE` | per-child reducer、独立 Segment/Item/metrics、root Journal 多 source publication |
| B1.2d | `DONE` | 并发 sibling/嵌套 child/失败回滚 conformance 与 B1.2 收口 |

B1.3 内部原子切片：

| Slice | 状态 | 边界 |
|---|---|---|
| B1.3a | `DONE` | Agent Runtime 发现完整 direct suspension set；application 将任一 source suspension 提升为 root-owned tree barrier；所有 active Runs 后序原子挂起；non-source Run 投影 `suspended`；SQLite epoch 42 持久化 continuations 与 item→suspension 精确绑定 |
| B1.3b | `DONE` | application 将 response set 规范化为 exact item→process/suspension answers；pending 整体 consume；全部 Run 后序原子打开新 Segment；adapter 在一次已接受集合内自动推进 sibling waits；SQLite epoch 43 持久化完整双拓扑 |
| B1.3c | `DONE` | resume / terminal / cancel 竞态、重复响应、失败补偿、跨进程重启、确定性发布顺序与完整 conformance sweep |

B1.4 内部原子切片：

| Slice | 状态 | 边界 |
|---|---|---|
| B1.4a | `DONE` | 任意 root/child identity 单次读取完整 durable tree；领域 `RunTree` 验证拓扑并生成 canonical postorder/subtree；application 冻结 Run、Pending、Turn 与 executor bindings 为 immutable cancel plan；root handle 串行化 child/root/interrupt owner |
| B1.4b | `DONE` | Running child 精确停止其 executor process subtree；Agent Runtime 后序发布 killed terminal；application join target terminal 与父 spawning Item 的唯一 `child_run_canceled`；root/兄弟不被取消；同步返回 exact child + unchanged root snapshot |
| B1.4c | `DONE` | Waiting child 的 Pending/Continuation/checkpoint ownership、subtree terminal、parent Item 与必要的 surviving-tree resume 单事务变换 |
| B1.4d | `DONE` | W2.1 ownership、W2.2 failure/rollback、W2.3 restart/query/publication/quiescence 与 W2.4 race/hygiene/full closure 全部完成 |

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
8. 若 B1 未完成，台账必须将其标为 `DEFERRED` 或 `IN PROGRESS`，并逐项保留未完成
   门槛，不得写成完整功能已交付。

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
| C15 | executor 只报告 `ProcessID/ParentID/SpawnCallID`；process → Run/Segment/Item 的映射由 application Coordinator 独占 | `ACCEPTED` |
| C16 | 文本 delta 的合并键是 `(source, event kind)`；来源不同即使事件类型相同也必须保持边界与顺序 | `ACCEPTED` |
| C17 | child source 在 durable admission 前不得进入 root reducer；完整 acknowledged opening 就绪前一律 fail closed | `ACCEPTED` |
| C18 | child admission 是有序 executor stream 上的 application control handshake；Agent Runtime 只在 Coordinator 原子提交父 spawning Item 与 child Run 后发布并执行 child | `ACCEPTED` |
| C19 | `runs.resume` 的领域值是完整 `SuspensionAnswer[]`；application 拥有 item coverage 与 canonical ordering，executor adapter 只消费已验证的 process/suspension binding | `ACCEPTED` |
| C20 | 一个 application resume 只对应一个 tree opening transaction；Agent Runtime 的中间 waiting segment 属于 adapter 内部推进细节，不得重新暴露成第二个 application barrier | `ACCEPTED` |

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
| event stream 丢失 process source 或跨 source 合并 delta | child 内容被错误归入 root/其他 sibling | application-owned source envelope + `(source, kind)` 合并边界 + 回归测试 |
| child 在 durable opening 前开始执行 | crash 后出现无 Run 的外部副作用或孤儿事件 | acknowledged opening；application commit 成功后才向 executor 放行 |
| 多 suspension response 被压成单值或逐个外露 | 丢答案、重复交互或 pending 已消费却只恢复部分 Run | complete `SuspensionAnswer[]` + adapter auto-drive + sibling integration |
| 整树 resume 逐 Run 提交 | crash/失败后同时存在 Running、Interrupted 与已消费 Pending | `TreeResumeDraft` + Pending consume + 全部 Run transition 同一 SQLite transaction |
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

### 2026-07-30 — B1.1

- 状态：`DONE`
- Commit：`78711d04a`（`feat(runtime): make run tree lineage durable`）
- 目标：把 child Run 的三条 identity edge、tree admission 与 profile ownership
  落到同一个 durable 事实模型，为 producer、barrier、cancel 和 cold recovery
  提供不可猜测的树边界。
- 关键裁决：
  - 新增领域值 `execution.RunLineage`，root 必须三字段全空，child 必须
    `spawnedByItemId/parentRunId/rootRunId` 同时存在，并拒绝 self-parent/self-root；
    `transcript.Run` 与 `execution.RunDraft` 只保存这三条语义不同的边，不增加 alias；
  - SQLite schema epoch 直接前进到 `41`；`runs` 增加 immutable
    `parent_run_id/root_run_id` 与 all-or-none `CHECK`，不写 migration、兼容列或双写；
  - durable admission 从“一个 Session 一个非终态 Run row”修正为“一个 Session
    一个非终态 root tree”：partial unique index 只约束 root，child/grandchild
    共享 root admission；
  - child admission 在写入前验证 parent/root 存在、同 Session、parent 属于目标
    root 且两者仍非终态；断链、跨 Session、跨 tree 与 child-owned profile 均明确失败；
  - protocol profile 只由 root row 持久化；child read 通过 root edge 物化继承，
    缺 root 或 child 自带 profile 时 fail closed，不保留可分叉副本；
  - root/parent 索引直接服务后续 subtree traversal；默认 `runs.list` 继续只读 root，
    不从 transcript Item 反推 root 身份；
  - Artifact export/import、canonical Snapshot 与 presenter 全量保留三条 child edge；
    archive 只在 root 携带 profile，restore 在写入前稳定拓扑排序为 parent-first，
    Snapshot 同时验证 spawning Item 属于直接 parent、root 真实为 root、父链无环且
    最终恰好到达声明的 `rootRunId`；
  - `features.subagents.enabled` 保持 false；本 slice 没有 child executor producer、
    tree barrier、subtree cancel 或前端行为，不以持久化 fixture 冒充功能启用。
- 生成物：
  - wire shape 未变化，Registry 生成物无需更新；
  - SQLite schema epoch `40 → 41`，按 dev 阶段单 epoch 策略硬切换。
- 验证：
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/domain/execution/... ./internal/infra/storage/sqlite/... ./internal/application/sessions/... ./internal/delivery/server/...`
    → `PASS`
  - root + child + grandchild 共享 admission、第二 root 被拒、跨树/断链拒绝、
    root-owned profile materialization、Artifact lineage export fixtures → `PASS`
- 残余风险：
  - `adapter/agentexec` 仍把 child process observation 从 Run event stream 中过滤，
    尚无 process→Run identity 与 child opening transaction；
  - Interrupted Run 仍隐含“自身持有 Interrupt”的 root-only 假设，尚不能表达
    non-source child 的 `suspended`；进入 B1.3 前必须拆开 Segment outcome 与
    root PendingInterruptSet；
  - recovery、cancel 与 live registry 仍按单 root segment owner 工作，分别进入
    B1.4/B1.5，不能提前打开 capability。

### 2026-07-30 — B1.2a

- 状态：`DONE`
- Commit：`4a5771f1d`（`feat(runtime): preserve child process causal identity`）
- 目标：删除 child 与父 `task` Item 之间依赖事件时序猜测的可能，使并发 sibling
  也能用一条不可变执行因果边精确建立 `spawnedByItemId`。
- 关键裁决：
  - `core.ProcessView`、`runtime.Process`、`event.ProcessCreated` 与
    `core.ProcessSnapshot` 统一新增 `SpawnCallID`；它表示生成 child 的父级 AgentTool
    call，root 必须为空，direct `RunChild` 可为空，AgentTool child 必须由 AgentTool
    在 admission 时一次写入；
  - Process snapshot epoch `13 → 14`，未知旧 epoch 继续 fail closed；不增加兼容
    decoder、旧字段 alias 或旁路 metadata map；
  - capture/restore 原样保留 causal edge，避免 restart 后重新猜测；Agent wire golden
    与 exported API baseline 同步接受该 breaking contract；
  - `agentexec.ProcessRef/SubagentProjection` 只投影 executor-owned causal identity，
    不携带 application Run/Item identity，保持 adapter 不反向依赖应用领域；
  - observer 同时携带 provider `SourceCallID` 与 Runtime collision-proof `CallID`：
    前者只用于 causal join，后者继续拥有 transcript lifecycle；二者不互为 alias，
    不解析字符串；
  - application reducer 在 open tool 集合内按 `SourceCallID` 精确解析 canonical Item；
    缺失或重复一律报协议错误，不按最新事件、参数相等或 tool 名猜测。
- 生成物：
  - Agent process snapshot epoch `13 → 14`；
  - Agent wire golden 与完整 exported API baseline 已更新；
  - Runtime public wire、Registry 与 SDK 生成物未变化。
- 验证：
  - `cd agent && go test ./... && go vet ./...` → `PASS`
  - `cd app/runtime && go test ./... && go vet ./...` → `PASS`
  - `cd agent && go test -race ./core ./event ./runtime ./internal/toolcall` → `PASS`
  - `cd app/runtime && go test -race ./internal/adapter/agentexec/... ./internal/application/runs/...`
    → `PASS`
  - AgentTool create/event/capture/restore causal edge、root/whitespace rejection、
    provider call → Item 唯一/缺失/歧义 fixtures → `PASS`
- 残余风险：
  - causal identity 已可靠，但 child opening 仍未被 application 确认，child 执行
    尚未等待 durable admission；进入 B1.2b；
  - child observation 仍被 adapter 过滤，且尚无 per-source reducer/Segment；
    进入 B1.2c；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.2b1

- 状态：`DONE`
- Commit：`3c954d331`（`refactor(runtime): make executor event sources explicit`）
- 目标：让 driven executor port 中的每个事件显式携带真实 producer identity，为
  acknowledged child opening 与后续 per-child reducer 建立不丢失、不猜测的路由边界。
- 关键裁决：
  - application port 从裸 `EngineEvent` 硬切换为
    `ExecutorEvent{Source, Payload}`；`ExecutorSource` 只包含 executor-owned
    `ProcessID/ParentID/SpawnCallID`，不泄漏 application Run、Segment 或 Item identity；
  - `ExecutorSource.Validate` 确定性校验空值、首尾空白、自父引用和 root 携带
    spawn-call 等非法组合；完全空 source 只保留给 process 创建前失败的 root terminal；
  - turn adapter 用 `emitRootEvent` 与 `emitProcessEvent` 区分 turn-owned 和
    process-owned 事件，避免含糊的 `emit/emitFrom` 命名；observer 原样保留具体
    process source；
  - event coalescer 只合并 source 与 event kind 同时相同的连续文本 delta；不同
    process 的同类 delta 必须作为顺序边界 spill，不能把 sibling/root 内容粘连；
  - Coordinator 在路由前校验完整 envelope。acknowledged opening 尚未安装时，
    任何 child source 都明确中止并合成 error terminal，绝不降级进 root reducer；
  - 当前 child observations 仍由 adapter 的既有 Minimal Profile 过滤器挡住；
    fail-closed 分支是防漂移护栏，不代表 child producer 已启用。
- 生成物：
  - public wire、Registry、SQLite schema 与 frontend artifact 均未变化；
  - 变更仅发生在 application-owned driven port 和 executor adapter 内部契约。
- 验证：
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./...` → `PASS`
  - `cd app/runtime && staticcheck ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/adapter/agentexec/turn ./internal/application/runs ./internal/delivery/server`
    → `PASS`
  - source validation、adapter identity preservation、跨 process delta 隔离、未准入
    child fail-closed fixtures → `PASS`
- 残余风险：
  - source 已可传输，但 child 仍不能在 durable opening 前等待 application 确认；
    进入 B1.2b2；
  - child Run identity 分配、parent running Item + child admission 原子事务、
    admission failure 对 executor 的回传与 rollback 仍未实现；
  - per-child reducer、独立 Segment/Item/metrics 和 root Journal 多 source publication
    进入 B1.2c；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.2b2

- 状态：`DONE`
- Commits：
  - `444ad2b31`（`feat(agent): add acknowledged child admission gate`）
  - `153b74d71`（`feat(runtime): atomically admit child runs`）
- 目标：让 AgentTool child 在任何发布或执行前等待 application 的 durable
  opening 结果，并在一个事务中建立 parent Item → child Run 的权威因果边。
- 关键裁决：
  - Agent Runtime 新增窄接口 `ChildAdmitter`；调用点位于 child process identity、
    parent/spawn cause 稳定之后，`ProcessCreated` 和首个 tick 之前。拒绝或 panic
    会回收 registry、budget reservation 与 deployment，不留下半创建 process；
  - 只有具备 `SpawnCallID` 的 AgentTool child 跨越 application Run 边界；
    direct `Engine.RunChild` 仍是 SDK 内部 child，不靠猜测补造 spawning Item；
  - adapter 将 `ProcessID/ParentID/SpawnCallID/StartedAt` 投影为不可变
    `ChildProcess` 值，不携带或分配 application Run/Segment/Item identity；
  - `ExecutorPayload` 成为有序 executor stream 的闭合 payload family；
    `EngineEvent` 仍是其可归约子集，`ChildOpeningRequest` 只属于 control payload，
    因此不可能误入 reducer；
  - opening confirmation 使用 single-use claim 状态机：executor context 只可在
    Coordinator claim 前取消；事务开始后必须等待权威结果，避免 executor 认为失败
    而 durable child Run 已经提交的歧义；
  - Coordinator 是 process → Run/Segment/Item 映射的唯一作者。它按 exact
    source route 找到父 reducer 和唯一 open spawning call，自行生成 child Run /
    Segment identity，并以一次 `Effects.CommitOpening` 同时提交父级 running Item
    与 child `RunDraft`；
  - route 只在事务成功后安装，确认也只在成功提交后返回；失败会原样回传 executor、
    回滚 SQLite 写集并终止 root turn；
  - `ChildRunAdmissionEnabled` 精确表达当前内部 seam，默认关闭。已准入 child route
    在 B1.2c 前没有 reducer，任何过早 child payload 都 fail closed，绝不折叠进 root。
- 生成物：
  - Agent exported API baseline 与 `EXTENSION_DESIGN.md` 已同步；
  - runtime public wire、Contract Registry、SQLite schema 与 frontend artifact
    均未变化；本切片只建立内部执行/应用事务边界。
- 验证：
  - `cd agent && go test ./... && go vet ./...` → `PASS`
  - Agent child admission 的 block / reject / panic / process-override 与 cleanup
    fixtures → `PASS`
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./... && staticcheck ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/application/runs ./internal/adapter/agentexec ./internal/adapter/agentexec/turn ./internal/adapter/runsegment`
    → `PASS`
  - child handshake / atomic opening failure fixtures 连续 10 次 → `PASS`
  - 真实 SQLite 中 child Run + parent spawning Item 同时提交、注入失败时同时回滚
    → `PASS`
- 残余风险：
  - child route 尚无独立 reducer / publisher，adapter 的 child observation filter
    尚不能打开；进入 B1.2c；
  - concurrent sibling、nested child 与完整失败回滚矩阵进入 B1.2d；
  - tree barrier、cancel、cold recovery 与 frontend consumer 仍分别属于 B1.3–B1.6；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.2c

- 状态：`DONE`
- Commits：
  - `c9363f44d`（`feat(runtime): give child runs independent segments`）
  - `ee2f565c2`（`feat(runtime): project independent child execution`）
- 目标：让每个已确认准入的 child process 拥有独立 Run/Segment reducer、Item
  lifecycle、累计 subtree metrics 与 terminal，同时让 root Journal 只作为整棵树的
  replay scope，事件 envelope 始终保留真实 source Run/Segment。
- 关键裁决：
  - 每条 executor route 持有不可变 process lineage、Run/Segment identity、继承的
    model/limits/profile 与独立 reducer；child route 仍只在 opening transaction
    成功后安装，完成后的 source 继续发事件会 fail closed；
  - parent spawning Item 与 child Run/Segment opening 仍在同一事务提交；child
    opening events 在 confirmation 返回前进入 root Journal，保证执行方收到成功时
    durable history 已可重放；
  - publisher 从 root-only 改为 source-aware：一个 root Journal 可以承载整棵树，
    但 commit、RunEvent envelope、Item、Run record 与 invalidation 都必须属于当前
    route；完整 reduction batch 在首个副作用前做 ownership 校验；
  - child terminal 只关闭自身 Segment，不关闭 root stream；root 在 active child
    尚未 terminal 时不得结束。stream failure/drain 按 reverse admission（天然
    descendant-first）合成未完成 child terminal，再处理 root；任一 child cleanup
    提交失败时不伪造 root 已完成；
  - adapter 的 `executionObserver` 只在 `ChildRunAdmissionEnabled` 且 process 具有
    `SpawnCallID` 时把 AgentTool child 投影为 application Run；direct SDK child
    继续保持内部实现细节，不能意外泄漏到 Run stream；
  - `processProjection` 在 Agent terminal event 上产生唯一 `ChildCompletion`；
    message/reasoning/tool/todo/usage/terminal 全部携带真实 process source，child
    terminal 先于 parent `task` tool result；
  - usage ledger 同时维护 root-tree aggregate 与每个 process 的 direct model
    accounting，child 边界按 lineage 动态求 subtree，root 边界读取整树；
    child/sibling 不互相污染，root 不再把 child 终态误写成自己的 terminal；
  - `steps` 与 `maxSteps` 统一为已记账模型调用数。usage/cost/steps 从同一 ledger
    产生，多个并行工具调用不再虚增 step；resume 接收累计 snapshot 而非再次相加；
    step/usage 回退、per-model aggregate 不一致、overflow 与 lineage 漂移全部明确
    拒绝。
- 生成物：
  - public wire、Contract Registry、SQLite schema、OpenRPC 与 frontend artifact
    均未变化；
  - 内部 executor port 的 `UsageReported/TurnUsage` 增加 authoritative
    `Steps/ByModel`，用于构造既不猜测也不丢明细的 Run 累计 snapshot；
  - `codex_runtime_protocol_vnext_final.md` 明确冻结 `steps` 的模型调用口径。
- 验证：
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./... && staticcheck ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/domain/execution/accounting ./internal/adapter/agentexec ./internal/adapter/agentexec/turn ./internal/application/runs ./internal/adapter/runsegment`
    → `PASS`
  - 真实 AgentTool delegation 的 admission → child progress → child terminal →
    parent tool result 顺序、child 一次调用/root 三次调用 subtree metrics → `PASS`
  - 独立 child source envelope、lineage/profile/limits、early root terminal cleanup、
    parallel tools 单 model-step、resume cumulative accounting 与 malformed aggregate
    fail-closed fixtures → `PASS`
- 残余风险：
  - concurrent siblings、nested child、opening/terminal 提交失败与 stream failure 的
    完整矩阵尚未统一成为 B1.2 conformance gate；进入 B1.2d；
  - child waiting/interrupt 仍明确拒绝，tree barrier/resume 进入 B1.3；
  - tree cancel、cold recovery/query/subscribe 与 frontend consumer 分别进入
    B1.4–B1.6；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.2d

- 状态：`DONE`
- Commit：`0d45e5839`（`feat(runtime): validate recursive child run trees`）
- 目标：用真实递归 AgentTool producer 与 application 失败矩阵证明 B1.2 的
  first-class child Run 路由、归属、顺序和计量在并发及异常路径下仍然成立。
- 关键裁决：
  - `task` 是 root/child 共享的通用执行能力；subtask role 可以继续委派 descendant，
    Agent Runtime 的 tree-wide budget 与 `MaxChildDepth` 共同约束递归。schedule、
    goal update 和 skill authoring 等产品根操作继续只属于 coding root，不因开放递归
    一并下放；
  - 真实 root → child → grandchild 链路必须按 grandchild、child、root 的 postorder
    结束；每层 `steps/usage/byModel` 都是精确 subtree snapshot，grandchild terminal
    必须先于 parent `task` completion；
  - 同一模型响应中的多个 `task` 调用可以并发执行。每个 sibling 保留不同的
    `SpawnCallID`、process/Run/Segment identity 和独立 direct usage；完成顺序可以由
    调度决定，但每个 child 必须先 terminal，随后其对应 `task` invocation 才能结束，
    root 聚合不能造成 sibling 间计量污染；
  - application conformance 固定 sibling payload 隔离、nested exact lineage、
    descendant-first terminal、stream drain 的 reverse-admission cleanup，以及
    early root terminal 的 fail-closed 行为；
  - child terminal commit 的一次性失败由 cleanup 重新写入 internal-error terminal，
    且必须先关闭 descendant 再关闭 root；第二个 sibling opening 回滚时，只清理已经
    durable admission 的第一个 sibling，失败的 draft 不获得伪造 terminal；
  - 测试桩按结构化 `ToolResult` 判别 continuation，不把忽略 tool part 的
    `Message.Text()` 当成完整消息序列，避免测试自己绕过协议形状。
- 生成物：
  - public wire、Contract Registry、SQLite schema、OpenRPC 与 frontend artifact
    均未变化；
  - 变化仅限 Agent tool-role policy、真实递归 producer 与 application conformance
    fixtures。
- 验证：
  - nested/sibling producer fixtures 连续 30 次 → `PASS`
  - sibling/nested/drain/terminal-failure/opening-rollback application fixtures
    连续 50 次 → `PASS`
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./... && staticcheck ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/adapter/agentexec ./internal/adapter/toolset ./internal/application/runs`
    → `PASS`
- 残余风险：
  - child waiting/interrupt 仍明确拒绝，tree barrier 与 resume 进入 B1.3；
  - tree cancel、cold recovery/query/subscribe 与 frontend consumer 分别进入
    B1.4–B1.6；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.3a

- 状态：`DONE`
- Commit：`882bffb7f`（`feat(runtime): atomically suspend active run trees`）
- 目标：把任一 executor source 的直接 suspension 提升为一份 root-owned
  Run-tree barrier，并在发布任何等待事实前原子挂起全部 active Runs。
- 关键裁决：
  - Agent Runtime 的 `PendingSuspensions(rootProcessID)` 只返回真实 direct waits，
    排除 parent 为传播控制流而持有的 suspension 副本；结果保持稳定的 tool /
    nested placement 顺序，Prompt 与 Schema 通过 ownership-isolated copy 投影；
  - turn adapter 不再把某个 root `Suspension()` 伪装成整棵树的等待原因，而是发出
    `TreeInterrupted{ProcessID,SuspensionID,Interrupt}[]` 控制 payload；
    reducer-only `TurnInterrupted` 不再允许 executor adapter 直接发出；
  - application route 在 barrier 时收集完整 active tree，并按 descendants before
    ancestors、siblings by Run ID、root last 的确定性 postorder 归约。direct source
    Run 持有自己的 Interrupt；ancestor、sibling 等 non-source Run 进入
    `Interrupted` 但不复制 payload，wire 因而自然投影为 Segment outcome
    `suspended`；
  - `interrupts.Pending` 从 root-only 续跑字段改为一份完整 tree hand-off：
    `Continuations[]` 显式保存每个 Run/process 的身份、process lineage、model、drained
    tools、createdAt、metrics 与 limits；`Suspensions[]` 双向完整绑定 client-visible
    item 与 executor suspension。校验拒绝重复、断链、跨 Run 绑定、缺 root 和不完整
    coverage，不在 resume 时猜测；
  - `Effects.CommitTreeBarrier` 成为唯一挂起写入口；一个事务同时写入 root-owned
    pending set、所有 transcript projection 和全部 Run suspend transition。旧的
    `EventCommit.Interrupt`、per-Run `StateSuspend` 提交及运行时 process lookup
    全部删除，不双写、不保留兼容入口；
  - SQLite schema epoch `41 → 42`，interrupt row 只存 root key 与完整
    continuations / suspension bindings；`root_process_id` 仅作派生唯一约束且读取时
    与 root continuation 交叉校验。Run query 对每个 child 只物化属于该 Run 的
    direct Interrupt，non-source suspended Run 的空集合是合法领域状态；
  - barrier 时间从 durable opening 后、executor activation 前开始计量，waiting
    时间不计入 active duration；root/child 都使用自身 segment start；
  - 系统不变量从
    `parked_run_has_exactly_one_open_interrupt_set` 治本更名为
    `parked_tree_has_exactly_one_open_interrupt_set`，Contract Registry 的
    API Reference 与 manifest 已同步生成；
  - conformance fixture 构造 root → child → grandchild 与并发 sibling，故意逆序
    上报 direct suspensions，证明 durable commit、continuation 与公开事件仍统一
    postorder；注入 barrier commit 失败时不发布 question 或 Interrupted Run，
    executor 进入受控终止。
- 生成物：
  - SQLite schema epoch `41 → 42`；
  - `contract/API_REFERENCE.md`、`contract/manifest.json` 更新 tree-level
    invariant；
  - public runtime wire、OpenRPC 与 frontend artifact 未改；本切片改变内部
    executor/application hand-off 与 durable implementation。
- 验证：
  - `cd app/runtime && go test ./... -count=1` → `PASS`
  - `cd app/runtime && go vet ./...` → `PASS`
  - `cd app/runtime && staticcheck ./...` → `PASS`
  - `cd app/runtime && go test -race ./internal/application/runs ./internal/adapter/agentexec/turn ./internal/adapter/runsegment ./internal/infra/storage/sqlite -count=1`
    → `PASS`
  - Agent Runtime concurrent/nested suspension discovery、SQLite boot recovery、
    complete tree postorder、commit failure no-publication、cancel/park arbitration
    fixtures → `PASS`
- 残余风险：
  - 当前 `runs.resume` 仍只接受并传递一个 `Resolution`，不能消费多 suspension
    response set，也只为 root Run 打开一个 continuation Segment；进入 B1.3b；
  - restart reconciliation 尚未逐一验证并恢复所有 continuation；resume / cancel /
    terminal 的完整树级竞态矩阵进入 B1.3c；
  - parked child subtree cancel、durable child query/replay 与 frontend tree fold
    仍分别属于 B1.4–B1.6；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.3b

- 状态：`DONE`
- Commit：`6a22abeb8`（`feat(runtime): atomically resume suspended run trees`）
- 目标：让一次 `runs.resume` 完整接受 root-owned barrier 的全部答案，并在任何
  executor side effect 前原子恢复所有 suspended Runs 与各自的新 Segment。
- 关键裁决：
  - 删除 application、TurnControl、dispatcher 与 `TurnProcess` 的单
    `Resolution` Resume 形状，唯一领域输入改为 `SuspensionAnswer[]`。每个答案同时
    保存 client-visible `InterruptItemID` 与 executor-owned
    `ProcessID/SuspensionID`；不存在单响应 overload、fallback 或兼容 wrapper；
  - application 先按 item 校验 response variant、完整 coverage、unknown/duplicate
    item，再按 Pending 的 canonical suspension order 生成答案。请求数组顺序不影响
    durable / executor 顺序，部分集合或错误绑定在进入 opening 前即失败；
  - Agent Runtime 仍一次推进当前 promoted suspension，但 adapter 在同一已接受答案
    集中自动驱动中间 waiting segments：每次先提交新的 process-tree checkpoint，再
    精确匹配下一个 direct process/suspension；只有答案集合耗尽后出现的新 wait 才
    能成为新的 application `TreeInterrupted`；
  - unsupported interrupt kind 的自动拒绝同样覆盖完整 suspension set，不再因
    multiple waits 直接把整棵树判为 internal error；
  - `Continuation` 新增不可推导的 application `RunLineage`，与
    `ParentProcessID/SpawnCallID` 组成双拓扑。`Pending.Validate` 交叉验证 process
    parent 与 Run parent、root 一致性、连通无环、唯一 root，并只接受
    descendants-before-ancestors、siblings-by-Run-ID、root-last 的一种 postorder；
    `Interrupts[i]` 与 `Suspensions[i]` 也只允许同一 canonical item order；
  - resume 在 activation 前为 Pending 中每个 Run 预建精确 executor route、独立
    reducer 与 fresh Segment；root 使用 response 返回的 Segment ID，descendants
    各自生成新 Segment，旧 process source 直接命中预装 route，不重新走 child
    admission；
  - `TreeResumeDraft` 取代 root-only `ResumeDraft`。`CommitOpening` 一次 consume
    root Pending、逐项核对 continuation postorder、将所有 Run 从 Interrupted
    转为 Running，并写入全树 opening projections；任一 Run 失败时 SQLite 回滚此前
    已恢复 descendants，同时恢复 Pending；
  - opening events 与后续 terminal events 均按相同 postorder 进入 root Journal；
    commit 成功前不发布任何 `SegmentStarted`，成功后统一设置 segment start boundary，
    最后才交付答案并启动 executor；
  - SQLite schema epoch `42 → 43`，continuation row 显式持久化
    `spawnedByItemId/parentRunId/rootRunId`，不读取旧 shape、不做 migration 或回填。
- 生成物：
  - SQLite schema epoch `42 → 43`；
  - public wire、Contract Registry、OpenRPC 与 frontend artifact 未变化：
    `runs.resume.responses[]` 原本已是完整集合，本 slice 修复其 application /
    executor / persistence 语义；
  - 新增 domain、application、Agent adapter、SQLite transaction 与 round-trip
    conformance fixtures。
- 验证：
  - `cd app/runtime && go test ./...` → `PASS`
  - `cd app/runtime && go vet ./...` → `PASS`
  - `cd app/runtime && staticcheck ./...` → `PASS`
  - sibling child 完整 answer set 集成测试：一次 barrier / 一次 resume / 无第二次
    barrier → `PASS`
  - 四节点 root/child/grandchild/sibling 整树 opening 与 terminal postorder
    application test → `PASS`
  - SQLite descendant-first partial success 后 root failure：全部 transition 与 Pending
    回滚 → `PASS`
  - `go test -race` 覆盖上述 Agent adapter、Coordinator 与 runsegment 三条关键路径
    → `PASS`
- 残余风险：
  - process restart 后完整 answer set 的 rehydrate + auto-drive、resume/cancel/
    terminal 竞态、重复提交与 checkpoint failure 矩阵进入 B1.3c；
  - parked child subtree cancel、durable child query/replay 与 frontend tree fold
    分别属于 B1.4–B1.6；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.3c

- 状态：`DONE`
- Commit：`bd1a69a4c`（`feat(runtime): harden run-tree resume conformance`）
- 目标：在真实持久化重启、并发仲裁和每个关键失败点下，证明 B1.3a/b 的整树
  barrier / resume 只有一个获胜边界、一个确定顺序和一份可恢复的 durable truth。
- 关键裁决：
  - cross-restart fixture 使用真实 SQLite 文件、两套独立数据库连接、Agent Engine、
    ProcessStore 与 MessageStore。第一套 runtime 持久化并退出当前执行视角，第二套
    runtime 重建 root + sibling process tree，一次接受完整 answer set，自动跨过
    中间 runtime wait，且不重放第二份 application barrier；
  - application rehydrate 根据 Pending 中的完整 continuation tree 恢复
    child-source admission / projection observer。判断依据是 durable tree 事实
    `len(Continuations) > 1`，不解析或猜测 feature string；root-only continuation
    不安装无意义的 child seam；
  - `TurnProcess.Resume` 先验证完整 suspension coverage，并预编码全部 response，
    只有整套静态边界全部成立后才设置 pending response set、启动第一个 runtime
    continuation。后一个 response 的编码错误不能让前一个 response 先产生 executor
    side effect；
  - 非 context-cancel 的 resume admission failure 先终止仍 parked 的 process，再
    发布 canonical error terminal；context canceled / deadline exceeded 仍回退
    park claim，保留可重试 checkpoint，不把客户端超时升级成不可逆失败；
  - auto-drive 每消费一个 response 后若落到中间 Waiting，必须先写完整 process-tree
    checkpoint 再推进下一个 response。checkpoint save failure 只产生一个 error
    terminal，不发布残缺的新 barrier，也不留下 active process；
  - 正常 opening、barrier、terminal 与异常 terminal synthesis 统一调用同一个
    canonical postorder：descendants before ancestors、siblings by Run ID、root
    last。删除“逆 admission 即足够”的第二套顺序算法；root terminal 的 active-child
    检查只判断数量，不再借 cleanup helper 表达；
  - resumed tree activation failure 发生在 opening transaction 已提交之后，因此
    全部已打开 Run 必须按 canonical postorder 写入 error terminal；opening commit
    failure 则不得激活 executor、消费 Pending 或发布 opening event；
  - SQLite `CommitOpening` 的第二次相同 resume 在 Pending 已消费后必须失败，并保持
    transcript 与 Running 状态逐字不变；不引入“重复成功”的内部兼容分支。
- 仲裁表：

  | 竞争 / 故障 | 线性化点 | 唯一结果 |
  |---|---|---|
  | duplicate `runs.resume` | `CommitOpening` 消费 root Pending | 第一次提交整树 opening；后续提交失败且不追加 Item、不改 Run |
  | resume vs root cancel（application） | root Session / cwd admission | 持有者进入 durable transaction；loser 返回 `session_busy`，不能越过 Pending |
  | resume vs cancel（parked executor） | dispatcher park claim | resume 或 cancel 至多一个取得 parked ownership；最终恰好一个 terminal |
  | cancel during multi-answer auto-drive | active segment / lifecycle owner | completed 或 canceled 二选一；不重放 barrier、不重复 terminal |
  | response encode failure | 完整 answer-set 预编码 | 零 continuation side effect；parked process 终止并发布一个 error terminal |
  | intermediate checkpoint save failure | checkpoint commit | 已接受 opening 不倒退；执行 fail closed 为一个 error terminal，无第二 barrier |
  | opening commit failure | tree `CommitOpening` transaction | Pending 与全部 Interrupted Run 保持原样；零 activation、零 opening publication |
  | activation failure | 已提交 tree opening | 全部 resumed Run 按 canonical postorder error terminalize |
  | executor stream drain | pump terminal synthesis | 每个 unfinished Run 仅一次 terminal，canonical postorder，root last |
  | natural terminal vs cancel | terminal transaction / live handle join | 已提交 terminal 获胜时 cancel 返回 finished；否则 cancel outcome 唯一 |
- 生成物：
  - public wire、Contract Registry、OpenRPC、frontend artifact 与 SQLite schema
    均未变化；B1.3c 收紧 executor/application lifecycle 与 conformance 证据；
  - 新增真实 SQLite restart、完整 answer-set/cancel race、checkpoint/encoding
    failure、duplicate resume、rehydrate projection 与 whole-tree activation failure
    fixtures。
- 验证：
  - restart / checkpoint failure / encoding failure / cancel race 关键 Agent fixtures
    连续 20 次 → `PASS`
  - resume tree opening / rehydrate / activation failure / stream drain application
    fixtures 连续 50 次 → `PASS`
  - SQLite whole-tree / duplicate resume transaction fixtures 连续 30 次 → `PASS`
  - `cd app/runtime && go test -race ./internal/adapter/agentexec/turn ./internal/application/runs ./internal/adapter/runsegment ./internal/infra/storage/sqlite`
    → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
- 残余风险：
  - B1.3 已完整收口；Running / Waiting child subtree cancel 与 parent
    `child_run_canceled` 结果进入 B1.4；
  - durable child query/replay、frontend tree fold 与 capability enablement 分别属于
    B1.5–B1.7；
  - `features.subagents.enabled` 继续保持 false。

### 2026-07-30 — B1.4a–b

- 状态：`DONE`
- Commit：`62c859475`（`feat(runtime): cancel running child subtrees`）
- 目标：为 root/child `runs.cancel` 建立唯一 tree plan 与仲裁边界，并完整交付
  Running child subtree cancel，不用 root teardown、状态补丁或第二套 API 冒充。
- 关键裁决：
  - `RunProjection.RunTree` 由任意 root/child ID 一次解析完整 aggregate；SQLite 用
    单条 CTE read 固定 root 与 descendants。application 将 durable Runs、root-owned
    Pending、TurnRef 和 live/parked executor bindings 一次冻结成
    `cancellationPlan`，再由领域 `execution.RunTree` 验证单 root、连通、无环、
    lineage 与 canonical postorder。缺 Run、断 parent、跨 Session、重复 process、
    Pending/continuation 不一致均在 executor side effect 前 fail closed；
  - root live handle 保存 immutable Run→executor-process binding；child opening 在
    durable commit 前先预留 binding，commit 失败只撤回同一 reservation，消除
    “child row 已可见但进程不可寻址”的窗口；
  - root cancellation、child cancellation 与 interrupt commit 由同一 handle
    arbiter 串行化；loser 返回 busy，不写 cancel reason、不停止 root，也不等待一个
    自己没有赢得的 terminal boundary；
  - `TurnControl.CancelSubtree` 是 application 的窄 driven port；Agent adapter 必须
    沿 parent chain 证明 target 是该 Turn root 的严格 descendant，才允许调用
    Agent Runtime `Kill(targetProcessID)`。跨 Turn、root 自身、缺失 ancestor 均零
    side effect 拒绝；
  - Agent Runtime 对 target 与 live descendants 仍在一个 process-tree mutation
    ownership 内原子标记 killed，但 `ProcessKilled` 改为 descendants-first、
    siblings lexical 的 canonical postorder。application 等到 target terminal
    durable commit，并等父 spawning Item durable commit 后才返回成功，因此
    executor teardown、subtree terminal 与 parent result 构成一个 command join；
  - `ToolCallEnd` 删除平行的 `Denied bool` / `Err string`，只保留一个
    application-owned `Problem`。Running child cancel 在 target canceled terminal
    已 durable 后，把 AgentTool 的普通失败精确重分类为
    `child_run_canceled`；父 Item 只完成一次，root 与 surviving sibling 保持原
    Segment 继续执行；
  - handle 按职责拆为 core lifecycle、cancellation arbiter 与 executor bindings；
    没有引入通用 manager/repository facade 或跨层领域接口。
- 生成物：
  - 新增 Artifact `childRunCanceled` problem variant，并由同一 protocol enum 生成
    `contract/schema.json`、frontend wire type 与 validator；
  - public `runs.cancel` method、root/child response union、OpenRPC method shape 与
    SQLite schema epoch 不变；本 slice 实现既有 vNext contract 的 Running 语义；
  - `features.subagents.enabled` 继续保持 false。
- 验证：
  - `go generate ./...` → `PASS`，schema 与 frontend wire 生成物已同步；
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`；
  - `go test -race ./internal/application/runs ./internal/adapter/agentexec/turn -count=1`
    → `PASS`；
  - `go test ./agent/runtime -count=1 && go vet ./agent/runtime` → `PASS`；
  - Coordinator 贯穿 fixture 证明 child terminal 恰好一次、parent
    `child_run_canceled` 恰好一次、root 仍 live 并最终自然 completed；
  - Agent Runtime fixture 证明 killed publication 为 child-before-root postorder；
    handle fixture 证明 root/child cancellation 至多一个 tree owner。
- 残余风险：
  - Waiting child 仍由 B1.4c 完成一个 durable tree transaction；当前明确 fail
    closed，不会只改 child 状态；
  - root cancel vs child cancel、resume vs child cancel、natural terminal、重复
    cancel、teardown failure、SQLite restart 与 exact query 的完整矩阵进入 B1.4d；
  - durable child query/replay、frontend tree fold 与 capability enablement 分别属于
    B1.5–B1.7。

### 2026-07-30 — B1.4c

- 状态：`DONE`
- Commit：`a4e153fd4`（`feat(runtime): commit waiting subtree cancellation atomically`）
- 目标：让 Interrupted tree 中的任意 child subtree cancellation 成为一个
  App-owned durable transaction，并在不伪造外部 Resume 的前提下精确保持或继续
  surviving tree。
- 关键裁决：
  - application 在 root/worktree admission 内重取 immutable cancel plan，然后让
    Agent adapter prepare 并冻结 runtime mutation；纯
    `waitingCancellationTransformation` 在 I/O 前确定 canceled postorder、parent
    Item replacement、reduced Pending/tree continuation 与 checkpoint write；
  - runsegment transaction 先以 frozen Pending 做 exact CAS，再依次提交 replacement
    process checkpoint、parent spawning Item 的唯一 `child_run_canceled`、target
    subtree terminal Runs；仍有外部 boundary 时写 reduced Pending，最后 boundary
    被移除时在同一 transaction 内 Resume 所有 surviving Runs 并写 opening Items；
  - parent continuation 将对应 `DrainedTool` 转为 `CommittedTool`。reducer 只投影
    已提交结果，不重放 `ToolCallStart`、不重复创建 transcript Item；
  - durable transaction 失败调用 `Abort`，live runtime 零变化；成功后 Agent
    `Commit` 只应用预验证内存变换。最终 boundary 通过 `ContinueAsync` 推进 ready
    checkpoint，不构造 suspension response；
  - BuildID、ProcessStore、usage ledger、CAS 和 SQLite transaction 均留在 App
    adapter；Agent public API 未增加任何产品持久化概念，也未增加兼容 reader、双写或
    legacy shim。
- 生成物：
  - protocol method、OpenRPC 与 Artifact schema 无变化；这是既有
    `runs.cancel` child response 与 `child_run_canceled` problem 的执行一致性实现；
  - SQLite Interrupt payload 持久化 committed tools；TranscriptStore 新增 exact Item
    read/replace CAS，Run resume 使用单一 transaction timestamp；
  - `features.subagents.enabled` 继续保持 false。
- 验证：
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`；
  - `MODULE=agent FAST=1 scripts/check.sh build vet test lint` → `PASS`；
  - `go test -race ./internal/application/runs ./internal/adapter/agentexec ./internal/adapter/agentexec/turn ./internal/adapter/runsegment ./internal/infra/storage/sqlite -count=1`
    → `PASS`；
  - Coordinator remaining/final boundary、transaction failure Abort、restart rehydrate、
    committed-tool reducer 与 real SQLite success/rollback fixtures → `PASS`；
  - application/domain forbidden import 与 Agent persistence/BuildID leakage 扫描、
    receiver 命名及 `git diff --check` → `PASS`。
- 残余风险：
  - root cancel vs child cancel、resume vs child cancel、natural terminal、duplicate
    cancel、teardown failure 与 exact query 的完整交错矩阵属于 B1.4d；
  - durable child query/replay、frontend tree fold 与 capability enablement 分别属于
    B1.5–B1.7。

### 2026-07-30 — B1.4d / W2.1

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：用确定性交错证明 parked/live tree 的 root、child、resume、natural terminal 和
  duplicate mutation 只有一个 owner。
- 关键裁决：
  - parked tree 不新增专用 coordinator，由既有 Session/worktree admission 统一裁决
    root cancel、Waiting child cancel 与 resume；
  - Running tree 不增加跨层锁，由既有 root handle 统一裁决 root/child/terminal；
  - loser mutation 统一返回可判定 `ErrSessionBusy`；已提交自然终态返回
    `ErrRunFinished`；
  - nested target fixture 同时包含 descendant 与 surviving sibling，避免把 tree
    arbitration 错写成单 Run 特例。
- 生成物：public wire、Contract Registry、OpenRPC、Agent API/wire、Artifact 与 SQLite
  schema 均未变化。
- 验证：
  - application ownership matrix 普通测试 `-count=10` → `PASS`
  - application ownership matrix race `-count=10` → `PASS`
  - `MODULE=app/runtime FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - `MODULE=agent FAST=1 scripts/check.sh build vet test lint` → `PASS`
  - 两个 module 的 `GOWORK=off go mod tidy -diff` → `PASS`
- 残余风险：
  - stale Pending、checkpoint、parent Item、terminal Run、Pending/opening transaction 与
    teardown/Continue failure 进入 W2.2；
  - SQLite restart、exact query/publication/quiescence 进入 W2.3；
  - capability 继续保持 disabled。

### 2026-07-30 — B1.4d / W2.2

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：把 waiting-subtree cancellation 的 stale、逐点 transaction failure、
  post-commit Continue 与 executor teardown 全部变成可复核的单一事实。
- 关键裁决：
  - stale Pending CAS 使用 `ErrSessionBusy`，不会进入 checkpoint 或其他写点；
  - checkpoint、parent Item、terminal Run、reduced Pending、tree Resume、opening Item
    和 transaction completion 任一点失败，完整 SQLite write-set 一起 rollback；
  - durable opening 已提交后的 Continue/activation failure 不倒写历史，由现有 segment
    pump 按 child-before-root error-terminalize surviving tree；
  - subtree/discard teardown failure 不释放错误 owner；claim 可重试，turn registry 由
    shutdown retry 后释放；
  - 错误统一携带 operation、target/root/turn/process identity，并以 `%w` 保留原因。
- 生成物：public wire、Registry、OpenRPC、Agent API/wire、Artifact、SQLite schema 与
  `features.subagents` 均未变化。
- 验证：
  - real SQLite pre-commit failure matrix → `PASS`
  - application/turn post-commit failure matrix → `PASS`
  - 高风险 race `-count=10` → `PASS`
  - Agent / Runtime build、vet、全量 test、lint、tidy diff 与 diff check → `PASS`
- 残余风险：
  - file-backed restart、exact target/root query、invalidation read set 与 canceled
    subtree quiescence 进入 W2.3；
  - 全量 race、canonical ordering 与 hygiene sweep 进入 W2.4；
  - capability 继续保持 disabled。

### 2026-07-30 — B1.4d / W2.3

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明 waiting child cancellation 的 command result、durable query、boot recovery、
  invalidation 与 event quiescence 共享同一 committed truth。
- 关键裁决：
  - canceled subtree 的 Running interrupt Items 是取消 transaction 的必要终态事实，不能
    留成永久 Running 的历史投影；
  - `ReconcileOrphans` 的恢复单位是 root-owned 完整活动 Run tree；Pending continuation
    必须与每个 active member 的 lineage/model/time 精确匹配，并校验所有 member 的
    interrupt/drained/committed Item；
  - remaining boundary 在 restart 后保持 Interrupted；final boundary 只证明 committed
    Running truth，boot 随后按既有 `run_lost` policy 收口，完整 Running cold recovery 留给
    B1.5/W3。
- 生成物：public wire、Registry、OpenRPC、Agent API/wire、Artifact、SQLite schema 与
  `features.subagents` 均未变化。
- 验证：
  - real file close/reopen、target/root exact query、child-addressed tree query → `PASS`
  - checkpoint BuildID/usage/process tree 与 reduced Pending exact round-trip → `PASS`
  - tree-aware boot reconciliation、exact invalidation 与 Journal quiescence → `PASS`
  - application/runs、adapter/runsegment、SQLite recovery race `-count=10` → `PASS`
- 残余风险：
  - 全量 canonical ordering、receiver/命名/错误/接口/兼容债审计与所有模块 gate 进入 W2.4；
  - capability 继续保持 disabled。

### 2026-07-30 — B1.4d / W2.4

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：以重复 race、canonical ordering、hygiene 与全量质量门禁完成 unified
  root/child cancellation 的最终 conformance。
- 关键裁决：
  - 所有写入、恢复与发布顺序继续由领域 `RunTree.Postorder/SubtreePostorder` 唯一决定；
  - Agent 公开语义只描述 caller-coordinated execution replacement，不借用 App 的
    Run/Item/Interrupt/Segment 或 durable transaction 心智；
  - consumer ports 已与真实用例同宽；大而内聚的 cancellation/recovery 文件不因行数
    机械拆分；
  - `features.subagents` 在 B1.5/B1.6 完成前继续 disabled。
- 生成物：public wire、Registry、Schema、OpenRPC、Agent API/wire、Artifact、SQLite
  schema 与 capability 均未变化。
- 验证：
  - Agent runtime/toolloop、Runtime application/runs、adapter/runsegment、SQLite
    `-race -count=10` → `PASS`
  - Agent / Runtime build、vet、全量 test、lint、tidy diff → `PASS`
  - contract/arch drift、receiver/静态错误/兼容债扫描、`git diff --check` → `PASS`
- 残余风险：
  - 通用 descendant paging、root multi-source stream conformance 与完整 Running tree
    cold recovery 进入 B1.5/W3；
  - capability 继续保持 disabled。

### 2026-07-30 — B1.5 / W3.0

- 状态：`DONE`
- Commit：随本次文档审计提交
- 目标：将 B1.5 重新对齐冻结契约和当前实现，冻结可直接执行的 W3.1–W3.4。
- 关键裁决：
  - `runs.subscribe` 只接受 active root Segment；child 必须返回 `run_not_root`，因此
    不新增 child Journal、child replay scope 或 child subscribe capability；
  - root Journal 已是整棵树多 source event 的唯一 live stream；
  - W3.1 的真实实现差距是 runs descendant page、items subtree page、page-local
    direct Run + ancestor enrichment 及其 cursor identity；
  - boot recovery 继续以 durable Run tree 为单位：完整 Waiting tree 可保留，不可恢复的
    Running/non-terminal tree 按 canonical postorder 收口为 `run_lost`。
- 生成物：只更新实施台账；public wire、Registry、schema、OpenRPC、Go/TS 类型、
  Artifact、SQLite epoch 与 capability 均不变。
- 验证：
  - 冻结契约 §5.4、§6.3、§8.1、§14.2、§14.4 → `PASS`
  - server/application/SQLite query、Journal/Subscribe、ReconcileOrphans 只读审计
    → `PASS`
- 残余风险：W3.1–W3.4 尚未实施；`features.subagents` 继续 disabled。

### 2026-07-30 — B1.5 / W3.1

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：完成 runs descendant page、items exact/subtree page、direct Run + ancestor
  summary 三条 durable query 契约。
- 关键裁决：
  - application `ItemScope` 是 Session / exact Run / Run subtree 的闭合值，不再公开两个
    可冲突 subject；
  - runs/items 的 `includeDescendants` 都属于 cursor identity；
  - subtree 与 ancestor closure 只使用 durable `runs.parent_run_id`；
  - page enrichment 由一次 recursive CTE 完成，不做 N+1 或 Session 全量读取。
- 生成物：public wire、Registry、schema、OpenRPC、Go/TS 类型、Artifact、Store epoch、
  Agent API/wire 与 capability 均未变化。
- 验证：
  - application queries、delivery server、SQLite targeted tests → `PASS`
  - 上述三个 package race `-count=10` → `PASS`
  - Runtime build、vet、全量 test、lint、tidy diff、arch/contract drift、diff check
    → `PASS`
- 残余风险：root multi-source stream、child subscribe refusal/profile gate 与 tail-first
  terminal race 进入 W3.2；`features.subagents` 继续 disabled。

### 2026-07-30 — B1.5 / W3.2

- 状态：`DONE`
- Commit：随本原子 slice 提交
- 目标：证明一个 root Journal 完整承载 tree-wide source event，并关闭 tail-first
  terminal 的 query/subscribe 竞态。
- 关键裁决：
  - source event envelope 保留各自 Run/Segment；sequence/cursor 只由 root Journal
    分配并绑定 root Run/Segment scope；
  - child subscribe 返回 `run_not_root`，不创建 child replay scope；
  - caller profile 不覆盖时整条 stream 拒绝，不做事件删减；
  - terminal 在 tail attach 前由 durable query 恢复，attach 后由 buffered stream
    恢复；commit-before-publish 排除“event 可见但 query truth 未提交”。
- 生成物：只增强 application/delivery conformance tests；production、public wire、
  Registry、schema、OpenRPC、Go/TS 类型、Artifact、Store epoch、Agent API/wire 与
  capability 均未变化。
- 验证：
  - nested tree shared root cursor + child subscription refusal → `PASS`
  - profile coverage + tail-first terminal 三种线性化 → `PASS`
  - application/runs、delivery/server race `-count=10` → `PASS`
  - Runtime build、vet、全量 test、lint、arch/contract drift、tidy diff、diff check
    → `PASS`
- 残余风险：complete Running-tree restart settlement、Waiting-tree preservation 与
  old-epoch replay → cold query convergence 进入 W3.3；capability 继续 disabled。

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

A-track 与 B1.1–B1.4 已收口。下一阶段是：

```text
B1.5 / W3 — durable query / subscribe / cold recovery
```

B1.5 内部原子切片：

| Slice | 状态 | 边界 | 完成定义 |
|---|---|---|---|
| W3.0 | `DONE` | 契约/实现差距冻结 | child subscribe 误述已纠正；三项 query 缺口与既有 stream/recovery 基础已定位 |
| W3.1 | `DONE` | durable descendant query | runs descendant stable page + bound cursor；items exact/subtree page + bound cursor；direct Run + ancestor summaries |
| W3.2 | `DONE` | root stream/replay conformance | root Journal 多 source、child refusal、profile coverage、tail-first terminal race |
| W3.3 | `READY` | restart/cold recovery conformance | file-backed complete Running-tree loss settlement、Waiting-tree preservation、old-epoch replay → cold query convergence |
| W3.4 | `TODO` | B1.5 full closure | race/full gates、contract drift、architecture/hygiene/compatibility sweep、docs/commit/push |

B1.4 的四个内部原子切片已经按事实依赖完成：

| Slice | 状态 | 边界 | 完成定义 |
|---|---|---|---|
| B1.4a | `DONE` | root-owned tree arbiter 与 cancel plan | root / child target 统一解析为 immutable subtree plan；root cancel、child cancel、interrupt/terminal 共用 root owner；删除“authorized child cancellation unavailable”分支 |
| B1.4b | `DONE` | Running subtree cancel | executor 精确停止并 join target process subtree；所有 descendant Run canonical postorder terminalize；父 `task` 只提交一次结构化 `child_run_canceled`；surviving sibling 与 root 继续执行 |
| B1.4c | `DONE` | Waiting subtree cancel | 一个 transaction 删除 target subtree 的 Interrupt / Continuation / snapshot ownership 并关闭对应 Run；root set 非空时 surviving tree 继续 Waiting，集合为空时一次打开全部 surviving suspended Run 的新 Segment 并恢复执行 |
| B1.4d | `DONE` | race / restart / query conformance | W2.1–W2.4 的 ownership、failure、restart/query、race/hygiene 全部闭环 |

B1.4d 不再修改 B1.4c 的事实所有权：仍由 B1.4a plan + root admission 冻结 target，
Agent prepared mutation 只决定 execution replacement，App transformation 决定完整 durable
write-set，runsegment transaction 是唯一提交者。W2.3 已证明这些边界在进程重启、cold
query、publication read set 与 subtree quiescence 下成立；W2.4 又完成全量竞态、
canonical ordering、hygiene 与兼容债收口。B1.5 只扩展 durable read/subscribe/recovery，
不得重新分配 B1.4 的事实所有权。

实现期间继续坚持：

- 不新增 `runs.cancelChild`；root 与 child 都使用现有 `runs.cancel`；
- 不让 delivery、Agent adapter 和 SQLite 分别决定 subtree 含义；
- 不通过“只把 child Run 改成 Canceled”冒充 process 已停止和父 tool 已完成；
- 不允许 parent `task` 同时收到 canceled result 与普通 tool result；
- 不把 root cancel 循环调用 child cancel；整棵树只有一个 transaction / teardown
  owner；
- 不保留 root-only cancel helper 作为兼容入口。新 tree boundary 验证完成后直接
  删除旧表达；
- 所有 terminal publication 继续使用 B1.3 冻结的 canonical postorder。

`features.subagents=false` 必须保持到 producer、tree transaction、interrupt barrier、
cold recovery、前端 tree reducer 和 §11.3 完整 gates 同时成立。仍采用 breaking-first
策略，不增加过渡 alias、兼容 decoder 或双写路径。
