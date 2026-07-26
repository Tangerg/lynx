# vNext 定稿评审意见

> **评审对象**：[`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md)（`protocolVersion 2026-07-27` / `SessionArtifactVersion 7` 冻结稿）。
>
> **方法**：不复述设计。只做三件事——把定稿依赖的**仓库事实逐条到代码核实**、指出**照它实现会立刻撞墙的地方**、以及**明确支持哪些决定不要在实现期动摇**。
>
> **结论分级**：**【阻塞】** 不补齐就无法完成 Batch C（附代码证据）· **【建议】** 不阻塞，但现在定比事后改便宜 · **【支持】** 我同意且实现期不要退让。
>
> 相关：决策账本在 [`PROTOCOL_DESIGN.md`](PROTOCOL_DESIGN.md) §19，证据与对照在同篇 §2/§15；本篇不复制它们。

---

## 0. 总评

**设计通过，可以照它实现。** outcome/metrics 正交化、tail-first cold recovery、union metadata 不给降级后门、Artifact 同批切换——这四条都是比前几轮更好的答案，不是折中。

但定稿有一个系统性盲区：**它按"目标 wire 形状"写完了，没有对照"这些事实今天是否 durable、是否有查询面"**。核实结果是——vNext 把 `waiting` 提为一等状态、把 `segmentId` 提为寻址参数、把 `metrics` 提为必填字段，而这三样**在存储层与查询面都还不存在或不完整**。下面 7 条阻塞项里有 5 条属于这一个根因。

一句话：**语义定稿已经足够，缺的是"权威读面（projection + query）与新语义对齐"这一层的定稿。**

---

## 1. 阻塞项

### 1.1 【阻塞】Segment 身份不 durable —— `stale_segment` / `expectedSegmentId` / `subscribe{segmentId}` 在冷路径上无法实现

**证据【已核实】**：`runs` 表的列是 `run_id, session_id, state, provider, model, outcome, started_at, updated_at`——**没有 segment 列**；`SegmentID` 在 `internal/domain/execution` 与 `internal/infra` 里只出现在一句 doc comment 中，实际只存在于 `application/runs` 的进程内 live registry。

**后果**：定稿要求 `runs.subscribe{runId, segmentId}` 必填、`runs.steer{expectedSegmentId}` 必填、并要求区分 `stale_segment`。但——

1. 进程重启后，一个 `waiting` run 的最后 segmentId 无人知道，**无法判定客户端给的 segmentId 是"过期"还是"伪造"**；
2. 更直接的死结：`RunRef`（§4.2）**没有任何 segment 字段**，而 §6.4 要求客户端收到 `stale_segment` 后"获取当前 Run 状态"——**可是没有任何方法会告诉它当前 segmentId 是什么**。`runs.list` 返回 `RunRef`，里面没有；`interrupts.list` 返回 interrupt。客户端在这里无路可走，实现时必然会被"顺手"退回成 runId-only subscribe——把定稿刚建立的并发前置条件废掉。

**建议改法（二选一，倾向前者）**：

- **A**：`RunRef` 增 `currentSegmentId?: SegmentId`，**`status:"running"` 时必给**、`waiting`/`finished` 时省略；同时 `runs` 表加 `segment_id` 列，与 state 同事务提交。客户端拿到 `stale_segment` → `runs.list`/`runs.get` → 用 `currentSegmentId` 重订。变体约束表里补这一行。
- **B**：允许 `segmentId` 省略表示"当前段"，ack 里回真实 segmentId。**代价是重新引入定稿明确拒绝的竞态**（reconnect 与 resume 抢跑时订到错流），不推荐。

> 附带一个语义决定要落地：**Segment 变成 durable 概念之后，它就不再只是"流的作用域"**。建议明确 `runs` 表只存 `current_segment_id`（当前/最后一段），**不建 segment 历史表**——历史属于 Item 时间线，segment 序列不是用户可回看的产物。

### 1.2 【阻塞】`RunRef.metrics` 是必填，但非终态 run 的 metrics 不 durable

**证据【已核实】**：`runs` 表无 metrics 列；`history_runs.payload`（JSON blob）只在 run **finished** 后写入。

**后果**：§4.2 的变体约束表要求 `running` / `waiting` / `finished` **三种状态都必须有 `metrics`**。冷读一个 `waiting` run（这正是 §3.1 新要求 `runs.list` 读 projection 的场景）时，metrics 无处可取。若改为"读时从 message/usage ledger 聚合"，则与两条不变量冲突：ledger 可能包含**未提交尾段**的行（§4.4 明确"不伪造未提交尾段"），且"后续 snapshot 不得回退"无法在聚合口径下保证。

**建议改法**：`runs` 表增 committed metrics 列（`steps` / `usage_json` / `active_duration_ms`），**与 run state 同一事务写**——这正是 §4.3 提交顺序里"metrics transaction commit"该落的地方。定稿只说了要在同一事务提交，没说提交到哪；补上"提交到非终态 run 的 projection 行"这一句，实现就没有歧义。

### 1.3 【阻塞】`runs.list` 今天读进程内 registry，且硬编码 `running`

**证据【已核实】**：`delivery/server/runs_query.go` 的 `ListRuns` 用 `s.coordinator.List()`（live registry）并对每条写死 `Status: protocol.RunStatusRunning`。

**后果**：这不是"改成读 projection"那么轻——今天 `waiting` run **根本不出现在 `runs.list` 里**，重启后列表为空，且所有返回值都自称 `running`。**`waiting` 一等化如果不落到查询面，等于没做**：前端仍然只能靠 `interrupts.list` 猜"这个 run 在等我"。

**建议**：把它写进 §3.1 的验收（"`runs.list` 返回 waiting run 且 status 正确"）与 §14.1，否则这条极易被当成纯 presenter 改动而漏掉。

### 1.4 【阻塞】`state.snapshot`（todos）没有冷恢复查询面

**证据【已核实】**：`todos` 表存在（per-session，items JSON）——**数据是 durable 的**；但 `dispatch/method_names.go` 里**没有任何 todos 查询方法**，它只作为 `state.snapshot` 的 payload 出现在 run 流里。

**后果**：定稿 §6.3 把 subscribe 的"无 cursor"语义从"从段头重放"改成 **tail-only**（这个改动本身是对的，见 §3），于是重连后**不会**再收到一份 `state.snapshot`；而 §6.2 说 `state.snapshot` 的冷恢复来源是"key 对应 query/projection"——**这个 query 不存在**。结果：冷恢复后 todo 面板空白，直到模型下一次写 todo 才恢复。这是定稿两个正确决定（tail-only + snapshot 靠 query）叠加出的新洞。

**建议改法**：给每个 first-party state key 一个查询面（本例：把 todos 挂到某个真实协议根下的读方法，数据已在表里，成本很低），**并把"注册 state key 必须声明冷恢复来源"变成 Registry 的必填字段 + CI 检查**——否则下一个 first-party key 会再犯一次。§11.3 的生成物里已有 "run event policy"，把 state key 的 recovery source 并进去即可。

### 1.5 【阻塞】`headEventId` 的唯一合法用途没写明，与"客户端不得比较 eventId"表面矛盾

§6.1 说 eventId 完全 opaque、**不得比较/递增/解析**；§6.3 的 `SubscribeRunResponse` 又给了 `headEventId`。读者无法从定稿推出它能干什么——不能比较，就无法用它判断"我是否已收到 head 之前的全部事件"。

我认为它有**一个**正当用途：**流在收到任何事件前就断了时，用它作为下次重连的 `Last-Event-Id`**（当 cursor 用，不做比较）。这是合理的，但必须写出来，否则要么被当成"可以比大小的水位线"误用，要么按"§3 可推导/无消费者不上 wire"被删掉。

**建议**：在 §6.3 明确"`headEventId` 只可作为后续重连的 cursor 使用，不得参与任何比较"；或者删除它（客户端在没收到事件时直接 tail-only 重订也完全正确）。**倾向删除**——少一个容易被误用的字段。

### 1.6 【阻塞】`RuntimeEvent.sequence` 与服务端 coalescing 的关系未定

§7.3 同时说了两件事：**"sequence 按单次订阅严格递增，gap → 全量 refetch"** 和 **"服务端可合并相邻 invalidation"**。如果 sequence 在事件**生成时**分配、合并时丢弃被并掉的号，客户端就会看到 gap → 触发一次不必要的全量 refetch；而 gap 本该是"你漏了东西"的信号，现在变成噪声。

**建议**：明确 **sequence 在"投递时"分配、必须连续无洞**；合并只改变 payload（把两次 `files.changed` 的 paths 并起来），不消耗号。这样 gap 恢复"只表示真的漏了"这一个含义。

### 1.7 【阻塞】`interrupts.list` 的分页粒度与存储粒度不一致

**证据【已核实】**：`interrupts` 表是 `run_id TEXT PRIMARY KEY`——**一个 run 一行**，多个 interrupt 装在 `payload` JSON 里。

而 §3.1 的 `ListInterruptsRequest` 带统一分页、§14 要求"待处理收件箱"心智。**按 interrupt 分页**就要求行级粒度（或读出后在内存里切分——那样 cursor 稳定性靠 payload 顺序，脆）。

**建议**：明确 `interrupts.list` 的分页单位。要么改成一 interrupt 一行（推荐，`(run_id, item_id)` 主键，天然支持按 itemId 稳定分页），要么在定稿里写明"分页单位是 run，一个 run 的 interrupts 整组返回"。**别留给实现期即兴决定**——它决定 cursor 语义。

---

## 2. 建议（不阻塞）

### 2.1 Batch C 的形态：一个 **release** ≠ 一个 **commit**

§13 要求 Batch C 是"一个可独立 revert 的 cutover"，理由（不能出现对外宣称支持 vNext 的中间态）成立。但它把生命周期 + metrics + outcome + cursor + `interrupts.list` + ProblemData + capability + `runtime.subscribe` 替换 + Artifact v7 + SDK + 前端 reducer + fixtures + 文档 + 生成物压成**一个 commit**——那会是一个无法 review、无法 bisect 的巨块。

**建议**：目标改成"**一个 release、一条分支**"：分支内栈式提交（每个 commit 自身全绿、可 bisect），**分支整体合并**才切版本号。既保住"main 上不存在半个 vNext"，又保住可审查性。这与我们既有的分支/合并纪律一致。

### 2.2 "整个 app 只有一条 runtime stream" 应表述为 **per client process**

§7.1 的连接预算论证是对的（6 格 HTTP/1.1 预算），但 §14.4 写成"整个 app 只建立一条"。多窗口 / 第二个浏览器标签 / 未来 CLI 同时在跑时，每个客户端进程各一条是**正确且不可避免**的。**建议**：验收改成"每个客户端进程至多一条，且同进程内不得为不同视图重复建流"。

### 2.3 `run_lost` 终态的 metrics 语义写进验收

§14.1 覆盖了 restart → `finished(error:run_lost)`，§14.2 没说这个终态的 metrics 是什么。按 §4.4"不伪造未提交尾段"，它应当**等于最后一次 committed snapshot**。补一条验收，否则实现可能顺手补一个"到崩溃时刻"的估算值——那正是 §4.4 禁止的。

### 2.4 cursor 的 `processEpoch` 可复用已有的进程身份

`interrupts` 表已有 `process_id` 列，说明"进程身份"这个概念在存储层已经存在。cursor 的 `processEpoch` **建议复用同一个来源**，而不是再造一个 epoch 计数——两个进程身份来源会漂。

### 2.5 `maxSteps` / `maxBudgetUsd` 跨 resume 延续需要 durable 限额

§5.2 要求 resume 时"preserve cumulative metrics and budgets"，但限额是 `runs.start` 的请求参数、resume 请求里没有。**建议**在 §4.2 明确限额随 Run durable（落 `runs` 表或与 metrics 同行），并加一条验收："resume 后 `maxBudgetUsd` 仍按原值判定，而不是无限"。这是安全相关项——丢了限额等于预算失效。

### 2.6 Error Registry 的 "recovery action" 建议做成闭合枚举

§9.2 / §9.3 要求每个 error type 在 Registry 声明"客户端恢复动作"。若那是自由文本，它就无法被 CI 校验、也无法生成 SDK 分支。**建议**定成闭合枚举（`refetch` / `coldRecover` / `resubscribe` / `reauth` / `waitRetryAfter` / `promptUser` / `stop`），SDK 直接按它生成默认处理。

---

## 3. 支持（实现期不要退让的四条）

1. **outcome / metrics 正交化（删 `RunResult`）**。"为什么停"和"花了多少"是两个问题，合在一个结构里就必然出现"interrupt 分支没有 result 所以拿不到计量"这种今天已经存在的洞（`presenter_run.go` 的 `Interrupted` 分支确实不带 Result【已核实】）。
2. **tail-first cold recovery**（先建订阅并缓冲 → 再查 → 再按 eventId 去重 fold）。它把"无 cursor = 从段头重放"这个隐含契约换成了**订阅与查询之间没有丢事件窗口**的显式模型——比旧语义正确，而且顺带让 replay 保留窗口可以做得很小。
3. **删 `custom.durable`**。"发送方自称 durable"从来不能替代 projection，这是把一个可表达的非法态从协议里去掉。
4. **union metadata 不给降级后门**（生成不了 `oneOf + discriminator` 就是 Registry 没做完）。上一轮我给过"失败则退回只生成方法表"的退路，定稿把它删了——**这是对的**，退路会被当成默认路径。同理支持 **Artifact v7 与 live wire 同批**：否则第二套旧语义立刻诞生。

---

## 4. 风险登记（实现时最容易走偏的三处）

| 风险 | 走偏的样子 | 防线 |
|---|---|---|
| Segment 身份没补 durable 就开工（§1.1） | 悄悄退回 runId-only subscribe，或让 `stale_segment` 永不触发 | 先落 `current_segment_id` + `RunRef.currentSegmentId`，再动 subscribe 签名 |
| 非终态 metrics 没落列（§1.2） | 冷读时从 ledger 聚合，"不得回退"随即失效 | metrics 列与 state 同事务；加"waiting run 冷读 metrics 与最后一次 committed 相等"的测试 |
| Batch C 压成一个巨 commit（§2.1） | 无法 bisect，出问题只能整批回退 | 分支内栈式提交，整体合并才 bump 版本 |

---

## 5. 结论

**语义与 shape 我没有异议**——七条阻塞项没有一条是"我想换个设计"，全部是"照这个设计实现会在这里断掉"。其中 §1.1 / §1.2 / §1.3 / §1.4 / §1.7 有代码证据，§1.5 / §1.6 是定稿内部两条规则相互抵触。

**建议的批次微调**：在 Batch B（生成基础设施）与 Batch C（原子切换）之间插一个 **Batch B′ — 权威读面对齐**，只做不改 wire 的存储与查询面准备：`runs` 表加 `current_segment_id` + metrics 列并与 state 同事务提交、`runs.list` 改读 projection、interrupts 行级化、todos 查询面。这批全部是**加列 + 换读源**，不动 wire，可以独立验证；做完之后 Batch C 才有可能真的一次切干净。
