# vNext 定稿评审意见（第八轮 · 放行）

> **评审对象**：[`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md) 第八版冻结稿（新增 §0.7 七轮裁决 · `StateSnapshotScope` / `StateSnapshotWriter` · typed 闭合 `StateSnapshot` · scoped state `revision` · 第九个 topic `state.changed` · final snapshot fence · per-Run-tree profile 措辞 · 跨层 metrics 无定义规则）。
>
> **本轮性质**：这是**放行评审**，不是继续挑刺。我只做两件事：核实第七轮 6 项是否真的落进 shape，以及给出**能不能冻结**的判断。
>
> **结论：可以冻结。没有阻塞项，也没有值得为它推迟定稿的建议项。**

---

## 1. 第七轮 6 项全部落地，且两处超出我提的范围

核实结果：

- `StateSnapshotCapability{key, recoveryMethod, scope, writer}` —— scope 与 writer 都进了 Registry **和** discovery；命名从我提的 `projector` 收紧成 `writer`（避免与 materializer 组件混淆，这个改名是对的）；
- `todos` 定为 **session-scoped / rootRun-writer**，并且明确**不给 child `todo_write`**，也不留"自动合并"或"child 私有清单"的隐式语义 —— 三种可能的暗规则一次全关掉；
- **final snapshot fence**：每次 committed replacement 触发快照，变化过的 key 在 **writer Segment 的 `segment.finished` 之前**建立 fence；
- state 失效通道**没有二选一**，而是分责：Run stream 的 fence 保证"当前任务能 fold 正确"，新增的 `state.changed{key, sessionIds?, runIds?}` 保证"没订阅该 Run 的窗口/进程能 refetch"。这比我推荐的 (a) 完整——我那条只解决了前者；
- 跨层 metrics：root 是 **as-of-root-boundary 的 subtree snapshot**，跨层相加、相减**均无定义**；
- profile 明确为 **per-Run-tree**，同一 Session 的后续 Run 重新协商。

**两处超出我提的范围**：

1. **`revision`（我没看到的竞态）**：tail-first cold recovery 会让"query 已返回较新 state"与"缓冲里迟到的旧快照"相遇——旧快照会把 reducer 覆盖回去。事件侧靠 `eventId` 相等去重解决了同类问题，**state 侧没有 eventId，所以必须自带单调 `revision`**，且 query 与 event 复用同一 typed shape、只在同一 `{key, scope identity}` 内比较。这个洞是我第七轮提出 fence 时该顺手想到的，没想到。
2. **state 载荷从 map 收成 typed 闭合联合**：`StateSnapshot = TodoState`、`TodoState` 自带 `type:"todos"` 判别字段。原来的 `Record<string, unknown>` state bag 被直接删掉——这让 state 这条缝和其它所有联合一样 frame-checkable。

> 附一条自查：本轮我原本怀疑"state 联合没有判别字段，加第二个 key 时会被迫破坏法则 #4"。核对后 `TodoState.type` 已经在——**法则没有被破坏，这个疑点不成立**，不作为意见提出。

---

## 2. 为什么现在可以冻结

八轮下来，定稿在四个层面都已经闭合，且每一层都有机器或事务在守：

| 层 | 闭合方式 |
|---|---|
| **语义** | Session / Run / Segment / Item 四层，每层职责表；Run 状态机三态 + 全部转换（含 child cancel 复活边）；Interrupt 是等待而非终态 |
| **树化** | Segment / metrics / cancel / Interrupt 归属 / suspended / profile 全部下沉到每个 Run，且 root 与 child 的差异都由**推导出来的边界**表达（不是划出来的） |
| **可检查性** | frame-checkable 约束由 Registry 生成 `if/then` + 双侧 `Validate()`；跨资源不变量具名、由事务与集成测试守；`Validate()` 禁止查存储 |
| **无债** | 单版本一次切换、fresh store、无 migration / 无回填 / 无 dual-read / 无 alias / 无降级路径；"生成不了就是没做完"，不留"失败则退化"的后门 |

按你关心的四个维度给判断：

- **优雅**：一个概念一个 wire 表达、可推导的不上 wire、非法状态不可表达——这三条不只是写在 §2，是真的被用来做决定（删 `RunResult` / 删 `channel` / 删 `retryable` / 删 wall-clock duration / `interrupt` 与 `suspended` 分变体 / `ItemListScope` 用互斥联合）。
- **务实**：cursor 编码、replay retention 的具体数值、连接预算、慢消费者、event coalescing 都落到了可测的数字与可执行规则上，而不是原则口号。
- **扩展性**：真扩展缝（工具名 / feature key / error type / custom）保持开放；伪扩展缝（plugin state key、plugin topic、无 query 的 durable custom）被明确删除——**留下的每一条缝都有 query 与 schema 闭环**。这是我最认可的一处判断力：删掉只有名字的扩展点，比留着更有扩展性。
- **功能性**：child cancel、resume 携带 input、run 维度历史查询、per-child 时间线、state 的 scope/writer——这几轮不是在削减能力，是在补齐能力的同时把边界说清。

---

## 3. 实现期要盯的三处（不是协议缺陷，是容易做错的地方）

写在这里是为了 Batch C 有人对着测，不是要求改文本。

1. **`revision` 的所有者与提交时刻**：必须由写入方在**同一个 durable 事务**里分配并随 state 一起提交，否则 query 与 event 会给出打平甚至回退的 revision——那会让 §0.7 刚关掉的竞态从另一头回来。测法：同一次变更同时经 `todos.get` 与 `state.snapshot` 到达，reducer 结果必须与到达顺序无关。
2. **fence 与并行分支的顺序**：final snapshot fence 属于 **writer Segment**（`todos` 是 rootRun-writer，所以是 root 段），而"child-before-parent、root 最后"的 suspended 顺序已定——两条顺序规则在并行分支上要同时成立。测法：一次并行子分支挂起，断言 root 的 fence 在它自己的 `segment.finished` 之前、且所有 child 的事件更早。
3. **双通道的幂等**：同一次 state 变更会同时经 Run stream 的 fence 与 `state.changed` 通报。这是**有意的分责**，但客户端必须靠 `revision` 做到幂等——两条都到、只到一条、乱序到，最终态必须一致。测法：三种投递组合各跑一遍。

---

## 4. 结论

**放行。** 我没有阻塞项，也没有"值得为它推迟定稿"的建议项；§3 三条是实现期的测试清单，不是文本修改要求。

建议的落地顺序不变：**Batch A（文档事实对齐）→ Batch B（Registry 与生成体系）→ Batch B′（权威读面 + fresh store）→ Batch C（一个 release、一条 cutover 分支、栈式全绿提交，整体合并才切版本）→ Batch D（数值调优）**。Batch C 之前 §1 的所有 shape 都已经在文本里，reducer 与 validator 可以直接照它写。

> 本篇按前几轮约定**未提交、未推送**。定稿一旦标记为最终版，我把这份放行意见与 `PROTOCOL_DESIGN.md §19` 的决策账本一起提交，作为这八轮演进的收尾记录。
