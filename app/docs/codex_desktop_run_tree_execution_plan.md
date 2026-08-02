# Desktop Run-tree Consumer 治本执行方案

> 作者：Codex
> 状态：`DONE`
> 建档日期：2026-07-30
> 当前阶段：`W4.0 DONE · W4.1 DONE · W4.2 DONE · W4.3 DONE · W4.4 DONE`
> W4.1 实施提交：`40cffd81e`
> W4.2 实施提交：`cc85d3039`
> W4.3 实施提交：`ca0949949`
> W4.4 协议收口提交：`49b6494bd`
> W4.4 Desktop surface 收口提交：`fcbf8f558`
> W4.4 deterministic projection 收口提交：`34a875d29`
> W4.0 文档提交：`26e43fd0e`
> 对应总任务：`B1.6 / W4 — Desktop Run-tree consumer`
> 约束：允许 breaking change；不保留旧 view shape、alias、双写或兼容 reducer

## 0. 文档职责

本文是 Desktop B1.6 的专项执行卡，统一记录：

- 当前 Desktop 对 Run tree 的真实消费能力与缺口；
- Agent bounded context 内唯一目标读模型；
- live stream、cold query、replay、invalidation 与 cancel 的收敛方式；
- breaking change 的爆炸半径、原子实施顺序、完成定义与进度。

权威关系：

1. [`API.md`](../desktop/docs/protocol/API.md)、
   [`AUX_API.md`](../desktop/docs/protocol/AUX_API.md)、
   [`TRANSPORT.md`](../desktop/docs/protocol/TRANSPORT.md) 与 Contract Registry 生成物
   决定现行 wire、生命周期、错误、capability 与恢复语义；
2. [`codex_architecture_execution_master_plan.md`](codex_architecture_execution_master_plan.md)
   是跨 Agent / Runtime / Desktop 的总台账；
3. 本文
   记录 Desktop B1.6 当时如何消费冻结协议；
4. [`../desktop/frontend/ARCHITECTURE.md`](../desktop/frontend/ARCHITECTURE.md)、
   [`../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md`](../desktop/docs/FRONTEND_PLUGIN_CONTEXTS.md) 与
   [`../desktop/docs/FRONTEND_AGENT_WORKSPACE_MODEL.md`](../desktop/docs/FRONTEND_AGENT_WORKSPACE_MODEL.md)
   决定前端 bounded context、依赖方向与 UI 心智模型；
5. generated wire、源码、测试与 Git 证明当前实现。

本文不修改 Runtime 契约，不重新设计 child subscribe，也不提前打开
`features.subagents`。

---

## 1. 目标

### 1.1 一句话目标

让 Desktop 把一条 root Segment stream 正确消费为：

```text
Session-scoped Agent Narrative
  + durable Run tree lifecycle
  + source-Run-owned Items / tools / plans / interrupts
```

并使实时、重放、冷启动、取消和 runtime invalidation 最终收敛到同一份
plugin-owned read model。

### 1.2 成功标准

1. **树完整**
   - root、child、sibling、nested child 都有独立 Run 状态；
   - tree identity 只来自 `RunRef.parentRunId/rootRunId/spawnedByItemId`；
   - Item、tool、plan、timeline、interrupt 都能定位 source Run。
2. **主叙事清楚**
   - root transcript 不被 child 的 interleaved Item 拼进同一 assistant bubble；
   - child narrative 挂在父 `task` Item 的语义锚点下；
   - 用户可展开 delegated work，但默认不让内部并发淹没主叙事。
3. **生命周期准确**
   - 不再用一个 `running: boolean` 代表整棵树；
   - `running / waiting / finished` 与 terminal outcome 分开表达；
   - active Segment、progress、committed metrics 各有明确语义。
4. **恢复闭环**
   - live stream 只提供实时 delta；
   - replay 补短暂断线；
   - replay 不可用、进程重启或 runtime resync 回到 durable
     `runs.list/get + items.list + interrupts.list`；
   - query snapshot 与 live event 通过明确 epoch/revision 规则合并。
5. **交互务实**
   - root 和 child 都只调用现有 `runs.cancel`；
   - `CancelRunResponse` 直接合并 committed facts；
   - waiting child cancel 若恢复 root，则使用返回的 root `activeSegmentId` 重订；
   - UI 不伪造 terminal、tree membership 或 parent tool result。
6. **架构干净**
   - fold 与 read model 归 Agent bounded context；
   - RPC 只在 adapter；
   - 其他 context 只消费 Agent `public/`；
   - components/pages 不接触 wire、client、replay cursor 或 capability preflight。

### 1.3 非目标

- 不新增、修改 Runtime method 或 wire field；
- 不创建 child 独立 stream；
- 不复制一份完整 `AgentViewState` 给每个 Run；
- 不把整棵 child transcript 默认摊平进 root transcript；
- 不在 UI 从 Item 到达顺序、tool 名称或文本内容猜 lineage；
- 不为旧 `view.run`、旧 selector 或 synthetic event 保留 alias；
- 不在 W4 内做 Synara 像素级视觉复刻；视觉总收口属于 W7；
- 不在 W4 完成前打开 `features.subagents`。

---

## 2. W4.0 当前仓库审计

### 2.1 已成立的基础

| 层              | 当前事实                                                                                                | 结论                           |
| --------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------ |
| generated wire  | `RunRef` 已有 `parentRunId/rootRunId/spawnedByItemId/status/activeSegmentId/metrics/outcome`            | 目标模型不需要新 wire          |
| RPC stream      | `RunTree` 已按 root Segment + descendant Run membership 过滤、去重，并只在 root Segment finished 时关闭 | tree-wide transport 已成立     |
| pump            | 每帧 `runId/segmentId` 已传给 batcher；child terminal 不关闭 root stream                                | source identity 已到应用边界   |
| replay          | 断流会从最后 folded `eventId` 重新 `runs.subscribe`                                                     | 短断线基础已成立               |
| cold history    | `items.list{scope:session}` 已能回放 durable Items；page 自带连接树所需 Run summaries                   | durable transcript 基础已成立  |
| runtime events  | 已订阅 `runs.changed`、`interrupts.changed`、`resync`                                                   | change signal 已到 Desktop     |
| plugin boundary | Agent 已有 domain/application/adapters/presentation/public 分层和 context gate                          | 改造可留在单一 bounded context |
| frontend gates  | `npm run check` 全绿：178 files / 1078 tests                                                            | W4.0 有稳定基线                |

### 2.2 核心缺口

| 缺口                  | 当前行为                                                                                  | 风险                                                     |
| --------------------- | ----------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| 单 Run readout        | `AgentViewState.run` 只有一个 root-centric `RunState`                                     | 无法表达 child/sibling/nested                            |
| child lifecycle       | child start/finish 只写 `"subagent"` timeline；child progress 直接丢弃                    | UI 看不到真实状态、计量和 outcome                        |
| global turn cursor    | `turnMessageId` 是 session 全局单值                                                       | interleaved child Item 会进入 root assistant bubble      |
| Item 归属不完整       | user/system message 有 `runId`，assistant turn shell 没有                                 | child narrative 无法稳定选择                             |
| tool 归属不完整       | `ToolCall` 没有 `runId`                                                                   | tree UI 只能扫描 timeline 或猜父子                       |
| plan 归属错误         | `plan` 是 session 全局单值                                                                | child plan 可覆盖 root plan                              |
| timeline 归属错误     | tool Item handler 未传 `item.runId`，默认回退 `state.run.runId`                           | child tool 审计会被记到 root                             |
| timeline 不可重建     | `Date.now()` + module-global sequence 生成 id/时间                                        | reload/replay 后时间和 identity 漂移                     |
| optional fold source  | `reduce(state,event,runId?,segmentId?)`                                                   | synthetic/cold path 可绕过 source identity               |
| synthetic wire        | optimistic message 与 recovery 构造假的 StreamEvent，使用空 RunID、空 profile、零 metrics | presentation 在伪造协议事实                              |
| cold Run 丢失         | recovery 只回放 Items，running query 只找第一个 root；不建立 durable Run tree             | reload 后 terminal/waiting/child lifecycle 不完整        |
| Run history 截断      | application port 把 Run summary 缩成 `{id,spawnedByItemId}`                               | lineage、status、outcome 在 adapter 边界被丢弃           |
| replay refetch 非原子 | replay lost 时先 reset view，再回放 Items                                                 | 可能出现空白中间态，且仍不恢复 Run tree                  |
| invalidation 未闭环   | `runs.changed` 只 invalidates sessions/usage                                              | mounted Agent view 不读取新的 Run facts                  |
| cancel 只认 root      | pump 只保存 `currentRunId`，本地直接把单 `view.run` 置 idle                               | child cancel 无入口，root cancel 忽略 committed response |
| 错误混合              | start channel-a error 与 terminal Run error共用 session `error`                           | owner、dismiss 与恢复语义不清楚                          |

### 2.3 根因

根因不是 reducer 少写几个 child case，而是当前 projection 把两个不同层级压成了一个：

```text
Session narrative state
        +
one current root Run readout
        ↓
AgentViewState.run / plan / turnMessageId / error
```

当协议从单 Run 演进为 Run tree 后，这个 shape 无法通过条件分支继续扩展。继续在
`runHandlers.ts` 增加 child map，同时保留 `view.run`，只会形成双写和“哪个是权威”的
新债务。

---

## 3. 冻结的设计裁决

### D1 — 使用 Session projection + normalized Run tree，不复制整份 view

一个 Session 只有一份 narrative/material projection；Run lifecycle 按 RunID 规范化。
禁止 `Record<RunId, AgentViewState>`。

### D2 — `view.run` 直接删除

不保留 `view.run` alias，也不让 reducer 同时维护 single-run 与 tree shape。所有旧
selector 改为从 tree 显式派生：

- latest root；
- executing root；
- selected Run；
- any executing root；
- Run by id。

### D3 — lifecycle 使用精确状态，不使用 `running: boolean`

Run node 使用协议同构的：

```text
status = running | waiting | finished
outcome = terminal-only
activeSegmentId = running-only
```

`waiting` 不是“not running”，`finished(error)` 也不是 session-level boolean。

### D4 — lineage 只认 durable Run identity

内部规范化：

- root：`parentRunId = null`，`rootRunId = id`，`spawnedByItemId = null`；
- child：三个 lineage 字段全部来自 wire；
- children、roots、depth 与 ancestry 全部由 selector 派生；
- 不把 `childRunIds` 和 `rootRunIds` 作为第二份事实写进 store。

排序使用 durable `createdAt`，相同时间以 RunID 稳定打破平局。

### D5 — fold 输入必须携带完整 provenance

live path 直接折叠完整 `RunEvent` envelope，不再把 `runId? / segmentId?` 作为四散的
可选参数。fold 能读取：

- `runId`；
- `segmentId`；
- `eventId`；
- `timestamp`；
- `event`。

history snapshot、Run snapshot 与 local optimistic mutation 使用各自明确入口，不伪装成
live StreamEvent。

### D6 — 不允许 synthetic protocol facts

删除：

- `runId: ""` 的假 Item；
- unknown protocol profile 的假 `segment.started`；
- zero metrics 的假 `segment.finished`；
- 用 `Date.now()` 重建 runtime timeline。

本地 optimistic message 是 local view mutation；cold recovery 是 snapshot hydration。

### D7 — 所有 durable Item 都保留 source Run

- `Message.runId` 对 runtime message 必填；仅 optimistic local message 可为 `null`；
- assistant turn cursor 按 RunID 分桶；
- `ToolCall.runId` 必填；
- plan 按 RunID 保存；
- timeline entry 的 `runId` 必填，session-only marker 才允许 `null`；
- item delta 使用 envelope RunID，并校验已知 Item ownership。

### D8 — root narrative 与 child narrative 分开投影

store 保留所有 source-owned material，但 selector 决定展示：

- Agent Narrative 默认选择 root messages；
- child messages/reasoning/tools/plan 挂到其 `spawnedByItemId` 对应的父 tool card；
- nested child 递归使用同一规则；
- session Timeline 可以跨 Run 按 timestamp 展示，但每条 entry 保留准确 RunID。

不按事件到达顺序把不同 Run 的 assistant Items合并成一个 turn。

### D9 — committed metrics 与 live progress 分开

每个 Run node 分别保存：

- `metrics`：最近一次 durable/terminal 累计快照；
- `progress`：当前 Segment 的 activity、preview step/usage、context tokens；
- 新 Segment 只重置该 Run 的 segment-local progress；
- cold RunRef 可覆盖 committed metrics，不用 progress 冒充 durability。

root metrics 的 subtree-inclusive 口径直接消费协议事实；Desktop 不自行累加 children。

### D10 — 错误按 owner 分开

- runs.start/resume 等 channel-a 拒绝进入 `commandError`；
- terminal Run error 存在对应 Run node 的 outcome/problem；
- tool failure 留在 ToolCall；
- public selector决定当前 banner 展示哪个错误，不复制错误内容。

### D11 — cold projection 原子替换

读取完整 durable snapshot 后一次提交：

```text
runs.list/get
  + items.list
  + interrupts.list
  + state recovery
        ↓
AgentSessionSnapshot
        ↓
pure snapshot fold
        ↓
atomic replace if epoch still matches
```

不得先清空 mounted view，再异步逐条补回。

### D12 — capability 条件是显式分支，不是 silent fallback

- negotiated `subagents`：`runs.list{includeDescendants:true}`；
- 未 negotiated：只请求 root scope，且该 build 不应产生 child；
- 显式 descendant 请求被拒绝时，不捕获后偷偷当 false；
- B1.7 打开 capability 后，同一代码路径自然读取完整 tree。

### D13 — runtime event 只触发权威刷新

`runs.changed / interrupts.changed / resync` 通过 Agent public application command 刷新
mounted Session projection。event payload 不直接 patch Run 状态。

### D14 — cancel 合并 response，不做 optimistic terminal

- `cancel(runId)` 接受 root 或 child；
- response 中 target Run 是 committed Finished(canceled)；
- child response 的 `rootRun` 同一提交边界合并；
- waiting child cancel 若 root 恢复 Running，按返回的 `activeSegmentId` 重订 root stream；
- failure 保留原 projection，并显示 typed action error。

### D15 — UI 状态不进入领域树

expanded/collapsed、selected node、scroll/focus 属于 presentation/workspace state；Run tree
只保存协议事实和稳定投影，不保存某个组件是否展开。

---

## 4. 目标读模型

以下是语义草图，不是 wire 复制，也不是要求机械使用全部字段名。实施时命名必须保持同一
语义：

```ts
interface AgentSessionView {
  messages: Message[];
  assistantTurnByRunId: Record<string, string>;
  toolCalls: Record<string, ToolCall>;
  plansByRunId: Record<string, PlanItem[]>;
  runsById: Record<string, AgentRunView>;
  timeline: TimelineEntry[];
  pendingInterrupts: PendingInterruptGroup[];
  commandError: AgentCommandError | null;
  shared: Record<string, unknown>;
}

interface AgentRunView {
  id: string;
  sessionId: string;
  parentRunId: string | null;
  rootRunId: string;
  spawnedByItemId: string | null;
  status: "running" | "waiting" | "finished";
  activeSegmentId: string | null;
  outcome: RunOutcomeView | null;
  metrics: RunMetricsView;
  progress: RunProgressView | null;
  createdAt: string;
  finishedAt: string | null;
}
```

约束：

- `runsById` 是 lineage/lifecycle 唯一作者；
- `messages/toolCalls/plans/timeline` 通过 RunID 归属，但不复制 lifecycle；
- optimistic message 未得到 server Item 前使用明确 local identity，不伪造 RunID；
- selector 可以返回 tree rows、root narrative、child material、latest digest 和 attention；
- view type 使用真实 session 语义命名。若实施确认 `AgentViewState` 已失真，则在 W4.1
  一次性改名，不留 type alias。

### 4.1 为什么不保存 `childrenByParent`

child 集合可由 `runsById` 的 `parentRunId` 确定。当前 Session 的 Run 数量远未达到需要
双索引优化的测量门槛；保存两个可变索引会让 lineage 出现双事实。若后续 profile 证明派生
是瓶颈，再在 selector 内做 memoized index，不改变事实 owner。

### 4.2 为什么 narrative 不是 `Record<RunId, FullView>`

Session 仍有共享的：

- 用户主叙事顺序；
- session state；
- pending sets；
- optimistic command error；
  - 跨 Run timeline。

复制完整 view 会制造 shared state、interrupt 与 message identity 的多作者。正确做法是让
每种 material 自己携带 RunID，再由 selector组合。

---

## 5. 目标数据流

### 5.1 Live

```text
root runs.start/resume/subscribe
  -> one tree-wide RunEvent stream
  -> RPC envelope validation + membership + eventId dedupe
  -> per-frame batch
  -> foldRunEvent(AgentSessionView, RunEvent)
  -> update source Run only
  -> selectors derive root narrative / child tree / attention
```

关键不变量：

- envelope RunID 与 `segment.started.run.id` 必须一致；
- Item.runId 与 envelope RunID 必须一致；
- child finish 只结束 source child；
- root Segment finish 才结束当前 root stream；
- parent tool result 由自己的 Item 完成事件提交，Desktop 不从 child terminal 伪造。

### 5.2 Cold open / rollback / import

```text
capture view epoch
  -> read Run summaries/refs + Items + Pending + state
  -> validate connected lineage and Item ownership
  -> build snapshot off-store
  -> compare epoch/interacted state
  -> one atomic replace
  -> attach the unique Running root, if any
```

不得：

- 选择“第一个看起来像 root”的 active Run；
- 为 Waiting set 合成未知 RunRef；
- 只回放 Items 而丢失 terminal Run lifecycle；
- 在 await 期间覆盖用户新发起的 Run。

### 5.3 Replay loss

```text
subscribe(lastEventId)
  -> replay_unavailable
  -> cold snapshot refresh
  -> subscribe tail-only to authoritative active root segment
```

snapshot refresh 与 tail attach 之间继续使用 epoch/head discipline，保证：

- missed durable Items 由 query 补齐；
- missed lifecycle 由 RunRef 补齐；
- tail event 不会被 snapshot 旧值回滚；
  - 同一 eventId 不重复折叠。

### 5.4 Cancel

```text
user cancel(targetRunId)
  -> Agent application command
  -> adapter runs.cancel(targetRunId)
  -> merge committed CancelRunResponse
  -> invalidate/refetch affected durable scopes
  -> if returned root is Running on a new segment: attach it
```

UI 只显示 pending action，不把 Run 先改成 canceled。root composer Stop 仍是同一 command 的
root target 变体，不保留第二套 cancel 规则。

---

## 6. Breaking blast radius

### 6.1 必改区域

| 区域                 | 变化                                                                 |
| -------------------- | -------------------------------------------------------------------- |
| Agent view types     | single `RunState` → normalized Run tree；material 增加 Run ownership |
| fold/reducer         | 完整 envelope input；source-specific lifecycle/item fold             |
| SDK fold seam        | 删除 optional provenance；不得再默认 `state.run.runId`               |
| store/selectors      | 删除 `view.run`；提供明确 root/run/tree selectors                    |
| recovery             | snapshot hydration；完整 Run lineage/status/outcome；原子 replace    |
| runtime gateway      | consumer port 保留真实 Run summary/ref，不做 `{id,...}` 截断         |
| run pump/reattach    | merge snapshot/replay epoch；target-aware cancel                     |
| public Agent surface | current root、Run by id、tree rows、cancel command、refresh command  |
| chat                 | root narrative selector；task child disclosure；plan 选择 source Run |
| workspace            | Run tree/timeline projection按 lineage，而非相邻 timeline 分组       |
| shell/navigation     | running/waiting/settlement 从 root selector 派生                     |
| docs/tests           | 更新 single-run 叙述和假“subagent isolation = complete consumer”表述 |

### 6.2 明确不保留

- `AgentViewState.run`；
- `RunState.running`；
- `patchRun`；
- `useAgentRunId/useAgentSegmentId` 的模糊 single-run 语义；
- `reduce(..., runId?, segmentId?)`；
- `appendTimelineEntry` 对 current run 的隐式回退；
- `AgentRunHistoryRef` 的截断 shape；
- synthetic `segment.started/finished` recovery；
- `runId: ""` optimistic Item；
- child-only `"subagent"` timeline summary 作为 lifecycle 替代品。

---

## 7. 原子实施计划

### W4.0 — 审计与冻结

状态：`DONE`

完成：

- 审计 generated wire、RPC RunTree、pump、fold、store、selector、recovery、
  invalidation、cancel 与 UI consumers；
- 定位 single-run projection、source ownership、synthetic wire、cold hydration 与
  invalidation 五类根因；
- 冻结 D1–D15 和本执行计划；
- 建立 Desktop 全门禁基线。

验证：

```text
cd app/desktop/frontend && npm run check
  -> PASS
  -> 178 test files / 1078 tests
```

已知非阻塞告警：

- Tailwind 生成无效 `shadow-[var(--shadow-*)]` rule；
- Lightning CSS 对 `::highlight(...)` 的识别告警；
- bundle 大 chunk 提示。

这些不属于 Run-tree 语义，但必须在 W7 / UI full closure 前有明确处理结果。

### W4.1 — Projection core 与 source-owned fold

状态：`DONE`

范围：

1. 建立 `AgentSessionView + runsById`；
2. 删除 single `view.run` 与旧 patch helper；
3. fold 接受完整 RunEvent provenance；
4. root/child/sibling/nested 独立折叠 lifecycle/progress/metrics；
5. message/tool/plan/timeline/assistant turn 全部按 source Run；
6. 删除 synthetic wire mutation；
7. 一次性迁移 selectors/ports/旧测试，不保留 alias。

必须先写的 conformance：

- interleaved root + child message 不合并 turn；
- sibling progress 互不覆盖；
- nested child lineage 可从任意到达顺序稳定重建；
- child plan/tool/timeline 不污染 root；
- child terminal 不结束 root；
- interrupt/suspended 将各 source Run 置 Waiting；
- terminal RunRef cold snapshot 与 live terminal fold 收敛；
- duplicate replay event 幂等；
- malformed/missing source 不静默归给 root。

完成定义：

- 旧 single-run shape 在 production/tests/docs 无残留；
- reducer exhaustive tests 全绿；
- context/layer/published boundary 无退化；
- 独立 commit/push。

### W4.2 — Durable recovery、invalidation 与 cancel

状态：`DONE`

范围：

1. gateway 返回完整 Run summary/ref；
2. 建立 pure `AgentSessionSnapshot` builder；
3. cold open/rollback/import 原子 replace；
4. negotiated descendant query 与 root-only capability branch；
5. replay lost 后 snapshot + tail attach；
6. `runs.changed/interrupts.changed/resync` 触发 Agent authoritative refresh；
7. root/child cancel 合并 exact `CancelRunResponse`；
8. waiting child cancel 恢复 root 时重订新的 active Segment；
9. typed failure 与 stale epoch 不覆盖新交互。

必须覆盖：

- running root + running child cold open；
- complete Waiting tree cold open；
- process boot 已将 orphan 收为 `run_lost`；
- replay unavailable；
- refresh 与 live event 并发；
- cancel child with surviving sibling；
- cancel waiting child 导致 root 继续 Waiting；
- cancel last waiting child 导致 root Running + new segment；
- root cancel；
- duplicate/stale/failure 不产生 optimistic terminal。

完成定义：

- live/replay/cold/invalidation/cancel 五条路径收敛；
- 无 synthetic RunRef/metrics/profile；
- 独立 commit/push。

### W4.3 — Tree presentation 与交互

状态：`DONE`

范围：

1. root narrative selector；
2. parent `task` 下的 child/nested disclosure；
3. Run tree timeline/read model；
4. running/waiting/finished/error/canceled 状态；
5. child cancel、展开/折叠、定位 parent tool；
6. shell/navigation attention 与 settlement selector；
7. keyboard/focus/aria/motion preference；
8. 不改变协议，不让组件触碰 RPC。

交互原则：

- root narrative 是默认阅读主线；
- delegated work 默认摘要、按需展开；
- waiting action 留在 Agent Narrative first；
- 完整审计可进入 Context Dock；
- tree row 的 cancel target 始终是该 RunID；
- UI 不显示协议无法证明的“完成”“恢复中”或计量值。

完成定义：

- root/child/sibling/nested/waiting/cancel/reconnect 的组件与 view-model 测试；
- 所有跨 context import 只走 `public/`；
- 独立 commit/push。

### W4.4 — Full closure

状态：`DONE`

完成：

- `StartRunResponse` 与 `ResumeRunResponse` 分离：start 的 `userItemId` 必填，
  resume 只在请求携带新 input 时返回；
- 普通 send 只按 exact `userItemId` 对账，删除内容匹配 fallback；
- `item.completed` 若仍是 running 直接 fail closed，不替 Runtime 猜 terminal；
- local optimistic identity 从 domain/public surface 移到 application view；
- 旧 `activeRun.ts` 按读取、命令、root attention 三个变化轴拆分；
- public API 名称明确区分 current root、active Session 与 exact Run；
- stop 接受结果由 Session-owned action 唯一判定，Store action types 不重复定义；
- `agentStore` Zustand callback receiver 统一为 `state`；
- Frontend architecture、workspace model、plugin context 与执行台账同步为完成态。

完整门禁：

```text
cd app/desktop/frontend && npm run check
git diff --check
contract/generated drift checks
compatibility / stale terminology / TODO-FIXME-HACK scans
```

B1.6 已完成；本段记录的是其交付时仍保持 disabled 的历史边界。后续 B1.7 已在
Runtime/Desktop 同一原子切片启用 `features.subagents`。

---

## 8. 验收矩阵

| 场景                      | Live | Replay | Cold | Invalidation |       UI |
| ------------------------- | ---: | -----: | ---: | -----------: | -------: |
| root only                 | 必测 |   必测 | 必测 |         必测 |     必测 |
| root + child              | 必测 |   必测 | 必测 |         必测 |     必测 |
| siblings                  | 必测 |   必测 | 必测 |         必测 |     必测 |
| nested child              | 必测 |   必测 | 必测 |         必测 |     必测 |
| child completes           | 必测 |   必测 | 必测 |         必测 |     必测 |
| child errors              | 必测 |   必测 | 必测 |         必测 |     必测 |
| child canceled            | 必测 |   必测 | 必测 |         必测 |     必测 |
| tree waiting              | 必测 |   必测 | 必测 |         必测 |     必测 |
| resume new segment        | 必测 |   必测 | 必测 |         必测 |     必测 |
| root terminal             | 必测 |   必测 | 必测 |         必测 |     必测 |
| replay unavailable        |    — |   必测 | 必测 |            — |     必测 |
| process restart/run_lost  |    — |      — | 必测 |         必测 |     必测 |
| malformed source identity | 必测 |   必测 | 必测 |            — | 不误展示 |

额外不变量：

- child Item 永不进入 root turn；
- child metrics 永不覆盖 root subtree metrics；
- Desktop 永不自行求和 root + child usage；
- child terminal 先于 parent task completion 时，UI 仍只消费各自事实；
- cancel response 与随后 stream/query 重复到达保持幂等；
- cold snapshot 未通过 lineage/ownership 校验时 fail closed，不造半棵树。

---

## 9. 风险与处理

| 风险                                     | 处理                                                                                                   |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| 一次 breaking 改动触及 selector 众多     | W4.1 以 compiler + tests 做完整迁移，不保留 alias                                                      |
| live event 与 cold snapshot 竞态         | epoch + off-store snapshot + atomic replace                                                            |
| child narrative 导致中心区过载           | root-first selector + task disclosure + Context Dock audit                                             |
| capability disabled 阶段无法发真实 child | fixture negotiation + reducer conformance；production 仍诚实 root-only                                 |
| RunRef 与 Item page summaries 丰富度不同 | canonical merge 规则：完整 RunRef 可补充 summary，旧 snapshot 不回滚新 lifecycle                       |
| timeline id/time 重建                    | 优先 runtime eventId/timestamp、Run/Item durable timestamps；不使用 wall clock 伪造                    |
| SDK event seam 暴露 Agent state          | W4.1 只允许一个 canonical projection contract；若现有 owner 与架构文档冲突，直接迁移 owner，不双层包装 |
| UI 阶段顺手复制业务规则                  | presentation 只消费 Agent selectors/commands，组件测试禁止 RPC imports                                 |

---

## 10. 进度记录

### 2026-07-30 — W4.0

- 实现审计基线：`main@b75d2a1d9`，worktree clean，审计时与 `origin/main` 一致；
- 文档提交：`26e43fd0e`（`docs(desktop): freeze run tree execution plan`）；
- 目标：完成 Desktop Run-tree consumer 的 read-only blast-radius audit；
- 关键结论：
  - transport 已正确承载 tree，缺口集中在 Agent projection/recovery/control；
  - 当前 child isolation 只是“不污染 single root readout”，不是 first-class consumer；
  - single `view.run`、global turn/plan/error、implicit timeline owner 与 synthetic recovery
    必须一起删除，不能打补丁；
  - target 是 Session narrative + normalized Run tree + source-owned material；
  - Runtime frozen wire 足够，不需要新增协议；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 178 test files / 1078 tests；
  - circular/context/published-boundary/layer/token/chrome/locale/bootstrap/bundle gates
    → `PASS`；
- 残余风险：
  - W4.1–W4.4 尚未实施；
  - `features.subagents` 必须继续 disabled；
  - 已知 CSS/bundle warnings 继续保留在 W7 hygiene 账本。

### 2026-07-30 — W4.1

- 实施基线：`main@bf9814b2e`；
- canonical projection：
  - `AgentSessionView` 直接替换失真的 `AgentViewState`；
  - 删除 single `view.run`、global plan、global assistant-turn cursor 与混合 error；
  - 建立 normalized `runsById`、`plansByRunId`、`assistantTurnByRunId`；
  - message、tool、plan、timeline 与 interrupt 全部保留 source Run；
- fold seam：
  - live fold 只接受完整 `RunEvent` envelope；
  - event ID、timestamp、RunID 与 SegmentID 不再 optional/fallback；
  - history Item、RunRef snapshot、PendingInterruptSet snapshot 与 local optimistic
    message 使用各自显式入口；
  - 删除空 RunID、unknown profile、zero metrics 等 synthetic wire；
- 语义证明：
  - root、siblings、nested child lifecycle/progress/outcome 互不覆盖；
  - interleaved message、plan、tool、assistant turn 与 timeline owner 不串线；
  - exact terminal replay 幂等；
  - live terminal fold 与 durable RunRef snapshot 收敛；
  - Item/envelope owner 冲突 fail closed，不回退到 root；
- breaking migration：
  - store、ports、selectors、public surface、SDK state helper 与 consumers 同步迁移；
  - 不保留 alias、dual write 或 compatibility reducer；
  - Run digest 明确按 selected root 过滤，不被 child timeline boundary 抢占；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 178 test files / 1076 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - 已知 CSS/bundle warnings 与 W4.0 基线一致，无新增 blocking warning；
- 残余工作：
  - W4.2 尚需把 cold/replay/invalidation 收敛成原子 snapshot replace；
  - cancel 仍需改为 committed `CancelRunResponse` merge，禁止 optimistic terminal；
- `features.subagents` 继续 disabled。

### 2026-07-30 — W4.2

- 实施基线：`main@40cffd81e`；
- durable projection：
  - gateway 一次并发读取完整 `items + runs + pending interrupt sets + optional state`；
  - negotiated `subagents` 时显式请求 `includeDescendants: true`，未协商时保持
    root-only query；
  - pure `AgentSessionSnapshot → AgentSessionView` 在 Store 外构建；
  - mounted view 在读取期间保持可见，只有完整投影通过
    `refreshSequence + viewRevision` CAS 后才原子替换；
  - history rewrite 额外提升 stream epoch，丢弃替换前已经排队的事件；
- 五路收敛：
  - cold open、rollback rehydrate、replay unavailable、runtime invalidation 与 cancel
    reconciliation 共用 authoritative refresh；
  - `runs.changed / interrupts.changed / state.changed / resync` 只触发查询，不把
    event 当成事实；
  - 同一 Session 的新 refresh 使旧请求失效，期间 live/local write 使旧 snapshot
    无权覆盖；
  - session driver remount 不再先清空已经 materialized 的 view；
- committed cancel：
  - 删除本地 `cancelRunningRun` 与 synthetic canceled terminal；
  - root/child 取消只折叠 exact `CancelRunResponse`；
  - child response 同时合并 post-cancel root `RunRef`，随后以完整 snapshot 对账；
  - waiting child 取消使 root 切换 Segment 时，旧订阅被取消并按新
    `activeSegmentId` 重订；
  - cancel failure 保留 Run lifecycle，并把结构化 ProblemData 投影为可读错误；
- restart 与时间线：
  - cold snapshot 覆盖 Running root + Running/Waiting child、pending HITL 与
    boot-settled `run_lost`；
  - durable `runs.list` 的 newest-first 顺序不再污染时间线：timeline 按 runtime
    timestamp 稳定插入，并始终保留最新 500 条；
- breaking cleanup：
  - 删除增量 history/state/run snapshot Store API 与独立 state recovery 路径；
  - `resetSession` 改为语义准确的 `ensureSession`；
  - `reduceCompletedItem` 改为能准确表达 query 输入的 `reduceDurableItem`；
  - RPC retry delay 统一在 trust boundary 做类型检查；
  - 无 alias、dual read/write、fallback decoder 或 compatibility reducer；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 182 test files / 1098 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - `git diff --check` → `PASS`；
- 已知非阻塞告警与 W4.0/W4.1 基线一致：
  - 无效 `shadow-[var(--shadow-*)]` 生成 rule；
  - Lightning CSS 对 `::highlight(...)` 的识别告警；
  - bundle large-chunk 提示；
- 残余工作：
  - W4.3 实现 root-first Run-tree presentation 与 root/child cancel 交互；
  - W4.4 做最终 architecture/hygiene/docs closure；
  - `features.subagents` 继续 disabled。

### 2026-07-30 — W4.3

- 实施基线：`main@cc85d3039`；
- root-first narrative：
  - root narrative selector 展示 Session 内全部 root-owned messages 与 local optimistic
    messages，不再把历史根运行误裁成“仅当前 root”；
  - child material 不进入 root transcript，由 `spawnedByItemId` 精确锚定 parent task；
  - sibling 以 durable `createdAt + RunID` 稳定排序，nested child 递归使用同一锚定规则；
- delegated disclosure：
  - running / finished / error / canceled / limit 默认摘要，waiting 默认展开，使审批、提问等
    HITL action 保持 narrative-first；
  - disclosure 使用 source Run 的 messages、plan、progress、committed metrics 与 outcome，
    不从 task 文本或事件顺序猜测状态；
  - child cancel 始终调用 exact RunID command，不做 optimistic terminal；
  - 完整审计由每个 disclosure 进入 Context Dock Timeline；
- lineage audit：
  - Timeline 从“相邻事件同 RunID”分组改为 Run tree preorder；同一 Run 的事件即使被 child
    event 穿插也仍归回同一 source group；
  - 无 timeline event 的 durable Run 仍展示 lifecycle；未知 Run event 明确保留为 audit
    group，不静默丢证据；
  - child row 可定位 parent task：原子切回 Chat、选择并展开 tool，随后滚动且聚焦 header；
  - root/child row 都展示准确状态、activity、step facts；cancel target 始终是该 row RunID；
- shell attention：
  - root attention 使用 `idle / running / waiting / finished` + exact RunID，不再退化为 boolean；
  - settlement 只接受同一 root 的 Running/Waiting → Waiting/Finished 转换；
  - 通知明确区分 needs-input、finished、error、canceled 与 limit；
  - selector snapshot 保持引用稳定，避免 React external-store 重渲染环，并有回归测试；
- ergonomics 与视觉：
  - 对照 `~/Desktop/synara` 的 compact subagent row：状态 dot 承载主色，终态文字保持安静；
  - disclosure header、cancel 与 audit 都提供键盘焦点、ARIA relationship 与至少 40px
    独立触达面；
  - expand/collapse 复用无测量的 `grid-template-rows: 0fr ↔ 1fr`，只 transition
    `grid-template-rows / transform / color`，并尊重 reduced-motion；
  - dynamic steps 使用 tabular numerals，长 activity 使用 truncate + title 保留完整内容；
- breaking cleanup：
  - 删除 `select/useCurrentRootMessages` 与 `useCurrentRootRunning` 的失真语义；
  - 不新增 alias、compat selector、secondary lineage index 或 component-local domain state；
  - Agent public presentation surface 保持 wire-free；Chat/Workspace component 不接触 RPC；
- 验证：
  - `cd app/desktop/frontend && npm run check` → `PASS`
  - 187 test files / 1125 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - targeted root/child/sibling/nested/waiting/cancel/reconnect、lineage timeline、parent locate
    与 settlement tests → `PASS`；
  - `git diff --check` → `PASS`；
- 已知非阻塞告警与 W4.0–W4.2 基线一致：无效 shadow utility、Lightning CSS
  `::highlight(...)` 识别与 large-chunk 提示；
- 残余工作：
  - W4.4 执行 full architecture/hygiene/docs/contract closure；
  - `features.subagents` 继续 disabled。

### 2026-07-30 — W4.4

- 状态：`DONE`
- 实施提交：
  - `49b6494bd`（`fix(protocol): distinguish run opening acknowledgements`）；
  - `fcbf8f558`（`refactor(desktop): clarify run application surface`）；
  - `34a875d29`（`refactor(desktop): make interrupt settlement deterministic`）；
- 协议与对账：
  - start/resume 使用不同 response shape，不再用 optional 字段掩盖不同命令语义；
  - start ack 对 RunID、SegmentID、UserItemID 全部 fail closed；
  - resume ack 的 UserItemID 为“字段缺席或非空”，生成器新增通用 optional text
    constraint，不手写特殊 validator；
  - optimistic user message 只按 server ItemID relabel；普通 send 不再内容匹配；
- Run application surface：
  - `runReadModel.ts` 只发布 current-root 与 active-Session reads；
  - `runCommands.ts` 只发布 current-root stop、active-Session exact cancel/problem command；
  - `rootAttention.ts` 只负责跨 Session root running 与 settlement subscription；
  - 删除 `activeRun` 文件与全部旧 alias；
  - Store actions 使用 consumer-owned、语义完整的 action types，receiver 统一为 `state`；
  - HITL settlement timestamp 在 resume 成功确认边界显式产生，纯 view mutation
    不再读取 wall clock；
- 架构与债务审查：
  - local optimistic ID 不再污染 Agent domain 或 public published language；
  - fold 不再把非法 running completed Item 修成 incomplete；
  - Runtime idempotency cache 只解码 start/resume 共同拥有的 stream address，不依赖错误的
    response reuse；
  - 无 compat alias、dual read/write、fallback decoder、synthetic protocol fact 或
    component RPC import；
- 验证：
  - Runtime `go build ./... && go vet ./... && go test ./...` → `PASS`；
  - Desktop `npm run check` → `PASS`；
  - 188 test files / 1127 tests；
  - type/lint/format/knip/circular/context/published-boundary/layer/token/chrome/locale/
    bootstrap/bundle gates → `PASS`；
  - Contract Registry / generated Go+TS / schema / OpenRPC drift 与 `git diff --check`
    → `PASS`；
  - package-local Go production receiver 命名一致性与 production debt marker
    扫描 → `PASS`；
- 已知非阻塞告警仍仅为既有 shadow utility、Lightning CSS `::highlight(...)` 与
  large-chunk 提示；
- B1.6 无残余实现工作；其后的 B1.7 已完成原子、fail-closed、无兼容路径的
  capability enablement，当前唯一下一任务见总台账 W6.0。

---

## 11. B1.7 交接记录

```text
B1.7 — Final conformance + capability enablement — DONE
```

完成结果：

1. Runtime composition 已广告 `features.subagents.enabled=true`；
2. Desktop `CLIENT_CAPABILITIES` 同时声明 `subagents:true`，并由 request metadata
   回归测试锁定；
3. 既有 source-owned fold、durable snapshot、root-first tree UI 和 exact cancel
   直接消费 negotiated tree，没有新增 component RPC 或第二条 recovery path；
4. child subscribe 仍明确拒绝，未保留 disabled fallback 或旧 capability 分支；
5. Desktop `npm run check` 全绿：189 files / 1128 tests。

后续工作只从总台账的 W6.0 开始；本专项计划保留为 Desktop Run-tree 的完成证据，
不再承担活动任务管理。
