# app2 领域模型

> 状态：R0 accepted。本文是 app2 业务语义的唯一 owner；wire、row、组件和 Agent Framework
> 类型必须映射到本文，而不是反向定义本文。

## 1. North Star

Lyra 是一个以工作目录为上下文、让用户与 agent 持续协作的桌面工作台。用户看到的是一段可审计、
可中断、可恢复的工作；模型看到的是为当前执行组装的有界上下文；操作系统看到的是一组受约束的
外部作用。这三种视图相互关联，但不是同一份数据。

核心层级为：

```text
Workspace value
  └─ Session
      ├─ Goal (0..1 current incarnation)
      ├─ Transcript
      ├─ Root Run tree (0..1 open)
      │   ├─ Root Run
      │   │   ├─ Segment 1..n
      │   │   ├─ Item 0..n
      │   │   ├─ ToolCall 0..n
      │   │   └─ Interrupt 0..n
      │   └─ Delegated Run 0..n
      └─ Plan projection (0..1 current root plan)
```

`Workspace > Session > Run > Item` 是产品导航层级；`Segment` 是执行/恢复边界，不是顶级导航资源。

## 2. 限界上下文

| 上下文 | 拥有的语言与规则 | 不拥有 |
| --- | --- | --- |
| Session | Session identity、title、workspace、revision、archive/fork/rollback/import/export | agent 进程、文件内容、UI tab |
| Execution | Run tree、Segment、admission、steer/cancel、termination、recovery | provider SDK 类型、Wails、HTTP |
| Transcript | 可见 Item、phase、source Run、顺序、tool/HITL 审计事实 | provider request history |
| Interaction | approval/question Interrupt、claim、answer、waiting barrier | 客户端草稿、风险猜测 |
| Goal | objective incarnation、lifecycle、continuation ownership | UI editor、Plan step |
| Planning | root Plan steps、revision、progress | Goal lifecycle、通用 state registry |
| Workspace | 路径解析、files/diff/git/search/index、workspace event | active Project 聚合 |
| Capability | skills、recipes、agent docs、knowledge、agent memory、tools、hooks | Run 内部状态 |
| Integration | providers、models、MCP server/tool/auth attempt、secret policy | 表单 UI state |
| Operations | schedules、usage、feedback、runtime discovery/health | 业务聚合的写规则 |

上下文允许在同一个 SQLite transaction 中协作，但 write-set 必须由一个具名 application use case
拥有。上下文之间只传稳定 identity/value 或 published read model，不共享可变对象。

## 3. Workspace

`Workspace` 是规范化的绝对目录值：

```text
Workspace {
  path
  projectRoot?
  availability = available | missing
}
```

规则：

- identity 是经过平台规则规范化的绝对路径；大小写、symlink 与 volume 语义由 path adapter 决定；
- `projectRoot` 是发现结果，不是第二 identity；
- Project 是按 cwd/projectRoot 派生的导航分组，不存在可写 Project aggregate 或 activeProject；
- Session 永久记录创建时选择的 Workspace；Runtime 默认目录不能冒充 Session workspace；
- workspace missing 是可恢复的可见状态，不删除 Session，也不静默回落到其他目录。

## 4. Session

`Session` 是用户持续工作的聚合根：

```text
Session {
  id
  title
  workspace
  modelRef
  revision
  favorite
  lifecycle = active | archived
  createdAt
  updatedAt
}
```

不变量：

- update、archive、rollback 等条件写必须携带 `expectedRevision`；过期写返回 revision conflict；
- Session status 是由 open Run/Interrupt 派生的 `running | waiting | idle`，不是独立可写字段；
- 一个 Session 同时最多有一个 open root Run tree；
- 删除 Session 原子删除其 Run、Item、Interrupt、checkpoint、Goal、Plan、usage attribution 和 owner claim；
- fork 产生新 Session identity 并复制被选择边界之前的语义历史；
- rollback 明确选择 `history | files | both`，并返回被丢弃 Run 的原用户输入，便于重新编辑；
- import 是导入 app2 artifact，不接受旧 app artifact。identity 冲突是明确拒绝，不自动改名。

## 5. Run 与 Segment

`Run` 是一次产品级 agent 工作，包含 root 与 delegated 两种角色：

```text
Run {
  id
  sessionId
  parentRunId?
  spawnedByItemId?
  role = root | delegated
  status = opening | running | waiting | canceling | succeeded | failed | canceled | lost
  outcome?
  admittedCapabilities
  providerRef
  modelRef
  contextTokens?
  createdAt
  finishedAt?
}
```

`Segment` 是同一 Run 在一次连续 engine execution 中的区间：fresh start 建立首段；HITL 回答、恢复或
明确 resume 建立后继段。Segment 不能被 UI 当成新的对话 turn，也不能重置累计 accounting。

Run tree 不变量：

1. Session 至多一个 open root Run tree；
2. child Run 必须同时拥有 `parentRunId` 和 `spawnedByItemId`，lineage 不从工具名、文本或顺序猜；
3. admitted capabilities、provider/model identity、创建时间、累计 accounting 在后继 Segment 中不可改写；
4. terminal Run 必须说明 outcome；failed/lost 还必须包含稳定 Problem；
5. waiting tree 恰好对应一个完整、开放的 Interrupt set 和一个 quiescent recovery point；
6. continuation 必须与 Run 的 immutable facts、Interrupt occurrence 和 checkpoint identity 精确匹配；
7. 已回答 Interrupt 的旧 checkpoint 在同一事务失效，crash 不得回退到旧等待点；
8. dropped Run 不留下 Item、Interrupt、checkpoint、tool result、admission slot 或 callback；
9. cancel 是控制意图，不是客户端先写 terminal；只有权威 termination 提交终态；
10. active-step crash 没有完整 quiescent checkpoint 时收敛为 `lost`，不得伪造可恢复。

Recovery 的原子边界也是整棵 root tree，而不是逐个 Run：同一 predecessor generation 的所有 running member 必须在一笔
transaction 内 child-before-parent 收敛为 `lost`，并以 root 的旧 Segment 作为 carrying stream。任何遗漏、异树成员、失踪 parent
或代际 CAS 失败都会拒绝整笔提交；完整 waiting tree 不属于 recovery lost 路径。

当 tree 中任一 member 产生 Interrupt，checkpoint 是整棵 tree 的一致性边界，不是该 child 的局部快照：
所有仍 running 的 member 必须先停在执行框架的 safe boundary；产生输入的 source Run 以 `interrupt` 结束当前
Segment，其余非终态 member 以 `suspended` 结束。所有 Run 状态、source-owned material、tree stream events、完整
Interrupt set 与 opaque checkpoint 必须在一笔事务提交，且 root 最后关闭；缺少任一 open member 时整笔拒绝。
恢复同样以整棵 tree 为原子：每个 waiting/suspended Run 保留稳定 `runId` 并获得自己的新 `segmentId`；答案只送回
提出对应 Interrupt 的 source member，paused sibling 从同一 checkpoint 恢复。root `runs.resume` 不把 child 折叠成
新的 root turn，也不重复执行已 terminal 的 snapshot member。

running cancel 先进入 execution owner：root target 取消整棵 Framework tree；child target 只取消 exact subtree，
descendant terminal 必须先于 target，target settlement 与 parent `delegate_task` Item 同事务。API 只有看到这些 durable
facts 后才返回 canceled Run；child response 的 root snapshot 来自同一 command boundary。waiting tree 的 child cancel
必须转换并重新持久化完整 checkpoint，不能只改产品 Run 或删一个 Interrupt。

Transcript 的 Delegate Item 保存 Lyra 产品语义（child `runId`、reply、Problem）；provider Conversation 只保存模型继续推理所需的
provider-neutral ToolResult。两者由同一 durable child/parent fact 投影但不共用 wire shape。只要 assistant `delegate_task`
ToolCall 已进入 Conversation，child terminal/cancel 就必须在同一事务或 enclosing tree/segment boundary 闭合对应结果；后继 fresh
Run 不得读到悬空 ToolCall。

## 6. Item、Transcript、Conversation 与 WorkingContext

`Item` 是持久、可回放、可归因的历史事实。最小闭合族：

- user message；
- agent message，显式 `commentary | finalAnswer` phase；
- reasoning；
- tool call 及其 lifecycle/material；
- approval request/decision；
- question/answer；
- compaction boundary；
- delegated Run disclosure；
- run terminal notice。

所有 Item 都有稳定 `itemId`、`sessionId`、`runId`、顺序和时间。child-owned Item 不拼进 root
assistant row；final answer 独立于 commentary/tool work，并且只有 final answer 拥有消息 actions。

四份相似数据必须分开：

| 名称 | 目的 | 可见性 | owner |
| --- | --- | --- | --- |
| Transcript | 用户阅读、历史、审计、恢复 UI | 用户可见 | Transcript context |
| Conversation | provider 下一次调用的消息历史 | 可能含内部控制输入 | Execution application |
| WorkingContext | 某次调用实际选择的有界输入 | 诊断可见，不直接编辑 | Agent adapter + application assembly |
| Runtime event stream | 推送进度和失效 | 短暂；不是真相源 | committed facts 的 transport projection |

fresh autonomous Goal 的控制提示只进入 Conversation，不创建 user Transcript Item。事件断线后必须从
durable snapshot/items 恢复，不能把 event buffer 当历史。

## 7. Interrupt

app2 只有两个 Interrupt variant：

```text
Interrupt = Approval | Question
```

共同 identity：`interruptId + occurrence + sessionId + runId + itemId`。回答必须匹配完整 identity；
仅匹配 itemId 或 tool name 不足以取得提交权。

Approval：

- 绑定 exact provider Tool CallID、原始参数、可选 edited arguments；
- accepted decision 是 Transcript 事实；
- `allow once` 不携带记忆 scope；只有显式选择 Session/Project/Global 才建立 allow rule；
- deny 永不携带 allow scope；
- 客户端只呈现 Runtime 提供的 reason/material，不用正则推断风险、可逆性或权限。

Question：

- fields 和 answers 保持同长同序；
- `[]` 是该 field 的显式 Skip，不是缺失；
- accepted answer 是 Transcript 事实；
- UI local draft 在提交成功前不是权威 answer。

回答用例原子完成：claim exact interrupt、持久 answer/decision、失效旧 checkpoint、提交 continuation intent。

## 8. ToolCall 与外部作用

`ToolCall` 是 Run-owned lifecycle，而不是字符串日志：

```text
ToolCall {
  callId
  itemId
  runId
  toolIdentity
  status = proposed | awaitingApproval | running | succeeded | failed | denied | canceled | unknown
  arguments
  presentation
  startedAt?
  finishedAt?
  executionDuration?
  resultPreview?
  durableResultRef?
}
```

规则：

- start 顺序服从模型声明顺序；并发完成后 canonical transcript commit 仍保持该顺序；
- executionDuration 排除 approval 等待；不能证明时保持缺席；
- result preview 有界，完整正文由 durable result owner 持有并按需读取；
- patch/file change material 只能来自 exact ToolCall 的真实回执，不从工作区 diff 反向猜；
- canceled、denied、failed、unknown 是不同状态；
- external dispatch 在 authoritative commit 前记录 attempt identity；回执丢失用 exact marker/receipt 证明，
  不盲目重放 non-idempotent effect。

## 9. Goal 与 Plan

一个 Session 至多一个 current Goal incarnation：

```text
Goal {
  sessionId
  incarnationId
  objective
  status = active | paused | blocked | completing
  activeRunId?
  reason?
  budget { maxRuns?, maxCostUsd?, maxSteps? }
  used { runs, costUsd, steps }
  revision
  createdAt
  updatedAt
}
```

- update objective 先静止 exact owned drive，再以 fresh incarnation 条件提交；
- pause/resume/clear 只能作用于 exact incarnation；
- Goal 不得晚于 Session 存活；
- `activeRunId` 是唯一 durable continuation owner；Run 不保存第二份 Goal provenance；
- `blocked` 必须携带闭合 reason code，`active/completing` 不携带 stop reason；
- capability、provider/model 等执行事实由开始时冻结，后继自动 Run 不悄悄换语义；
- `completing` 是可观察结算窗；final owned Run 成功结算后删除 Goal，不再制造第二个 `complete` 状态。

Plan 是一等资源，不是通用 state registry 的一个 key：

```text
Plan {
  sessionId
  revision
  updatedAt
  steps[] { id, description, status = pending | in_progress | completed }
}
```

同一时刻至多一个 `in_progress` step。Plan 是 Session-scoped current resource；写它的 root Run 通过
`plan.updated` 发布 exact committed snapshot，但 Run 不另存第二份 Plan ownership。`plan.changed` 只做冷读失效，
不携带 Plan payload。Desktop 显示紧凑进度，完整步骤在需要时展开，但 UI 展开状态不回写 Plan。

Plan mode 是独立的 Session-scoped durable safety policy，不是 Plan aggregate 字段，也不替换全局 Approval policy：
它阻止 write/exec/network effect，直到用户批准 exact stored Plan。进入/退出 mode 本身不伪造 Plan revision。

每个本 Runtime 正常 terminalize 的 root Run 同事务捕获一个 Plan Boundary。fork 取所选 root boundary，而不是
父 Session 的 live Plan；history rollback 把 known boundary 作为一次新的 Plan replacement，所以 live revision 继续
单调前进。Artifact 只携带 live Plan，不伪造历史 boundary：imported Run 的 boundary 缺席表示 unknown，与 recorded
empty 明确不同。boundary 由 Run FK 拥有，Run 被回滚删除时自动消失。

## 10. 配置与能力资源

以下资源各自有 owner，不合并成通用 Settings map：

- Provider 与 credential reference；
- Model catalog、utility role、embedding role；
- MCP server、tool、authorization attempt；
- Skill discovery/library/proposal；
- Recipe、AgentDoc、Knowledge entry、AgentMemory；
- Hook trust；
- Approval mode/rule；
- Schedule；
- Tool catalog；
- Codebase index；
- Usage 与 Feedback。

配置写使用闭合 draft/command 和 revision；secret 写采用 `set | clear | keep` 的显式三态，读侧只返回
masked/source metadata。作用域改变时不得自动搬运 secret。

Knowledge 是人类编辑的 file-backed capability，不是 AgentMemory 或 generic settings：home、projectRoot、cwd 按宽到窄组成
有效 cascade，正文和 revision 的 owner 都是各自 LYRA.md。相同物理文件在 list 与 Desktop 只能出现一次；conditional update
绑定物理 target identity 和 content revision，symlink 不得逃逸 scope，也不得因原子替换而意外改写为普通文件。fresh root Run
读取一次有效 cascade；checkpoint 之后外部变化只影响后续 fresh Run，不改变 waiting/resume 的历史上下文。

AgentMemory 是 Agent 维护、用户复核的长期事实集合。target 是 closed `user | project` partition；条目以 content digest 去重，
`pending → active | rejected` 是单向 review lifecycle。rejected 是不进入 API/prompt/search 的 tombstone，阻止相同自动事实反复提案；
user-authored 条目立即 active。只有 active 可 pinned、注入或搜索。管理命令的内部 revision 与 transaction 保护原子 read-modify-write，
但不擅自扩张现有 Lyra wire；visible collection 与 rejected tombstone retention 分别有硬上限。fresh root Run 冻结 effective
user+project recall，resume 不因后续 review 漂移。

自动维护只消费已 durable commit 为 `completed` 的 root Run，不把 signal payload 当事实源。每个 Run 以 extraction receipt 保证至多一次提交；
即使没有可保留事实，也必须提交空 receipt，避免重启后无限重复推理。输入只取该 Run 的有界 Conversation，省略 system、private reasoning
和 tool result body；输出经数量、单项 UTF-8 bytes、总 bytes 三重边界。receipt 与 project daily ledger 在一个 transaction 提交；Run/source
与真实 extractor selection 是不同事实：前者由 Run 保有，后者可选地归一化在 receipt，ledger fact 通过 join 获得 Session/day/model
provenance，不重复存一份可漂移 metadata。每个 project 的 curation watermark 通过 CAS 前进；策展只能从未折叠 ledger 产生新的
`pending` proposal，不得修改或删除
active/user memory。watermark 与 proposal 在同一 transaction 提交，digest duplicate 和 rejected tombstone 都是负证据。terminal Run outcome 与
event publication 先于非阻塞维护信号，抽取/策展失败只保留 durable backlog，不反向改变 Run 结果。

LifecycleHook 是用户编写的生命周期策略，不属于 generic Capability envelope。global `~/.lyra/hooks.json` 与 projectRoot→cwd
`.lyra/hooks.json` 形成宽到窄的有序 cascade；每个文件、单个 matcher/action/timeout 及整条 cascade 都有硬边界。Hook event、scope、
matcher 与 exactly-one `command | inject` 在进入 application 前成为 closed domain value。global 配置属于用户本人；project 配置默认 inert，
只有 canonical resolved project root 的显式 durable trust 才能激活。管理 inspection 会解析未信任项目以供审计；Run execution 不打开未信任
项目文件，使恶意或损坏的 cloned config 不能破坏有效 global policy。trust false 由记录缺席表达，重复 trust/revoke 不发布伪 change。
hooks.json 是 file-owned source；Runtime 只发布 invalidation，不建立第二份可漂移配置镜像。

## 11. Runtime 生命周期与事件

`RuntimeInstance` 是进程代际，不是业务聚合：每次启动获得新 `instanceId`，同一 durable store 可跨代保持；
通用 store identity 是 Runtime 内部持久化事实，不擅自加入现有 wire。client 只使用 discover 发布的幂等
namespace 判断 command journal 是否仍可重放。进程内 response、stream、listener、owner lease 只属于创建它的
generation。

Runtime event 只承担两类工作：

1. Run/Item 的增量通知；
2. durable resource 的 invalidation/resync 信号。

事件 sequence 在一个 instance/topic 内单调；gap 触发相应 topic 全量重拉。事件不能写第二份业务事实。

## 12. 关键跨上下文用例

### Start Run

```text
validate intent
  -> load Session + prove revision/admission
  -> assemble Conversation/WorkingContext
  -> stage engine execution
  -> atomically commit Run + Segment + user Item + command marker
  -> begin staged execution
  -> publish committed events
```

### Answer Interrupt

```text
validate exact identity and answer shape
  -> claim Interrupt
  -> stage continuation
  -> atomically commit answer + Transcript fact + old-checkpoint invalidation + new Segment intent
  -> apply continuation
  -> publish committed events
```

### Mount Session

```text
read one consistent snapshot
  -> Session + runs + source-owned items + open interrupts + Goal + Plan + context footprint
  -> Desktop replaces one immutable AgentSessionView generation
```

### Recover Runtime

```text
open exact schema
  -> elect durable ownership winner
  -> classify each nonterminal tree from committed facts
  -> resume only complete quiescent checkpoints
  -> terminalize unprovable active effects as lost/unknown
  -> publish new runtime generation and resync topics
```

## 13. 禁止模型

- `Message[]` 同时承担 Transcript、Conversation、WorkingContext 和 stream buffer；
- 可写 `Session.status`；
- active Project 聚合；
- 通用 `State{key, scope, writer, payload}`；
- 通用 Signal API 替代 steer/answer/cancel；
- `running: boolean` 替代完整 Run/Tool 状态机；
- 按文本、tool name、数组位置猜 lineage、final answer、approval 或 tool result；
- UI optimistic terminal；
- 事件总线作为跨聚合事务；
- 旧 checkpoint 在回答后继续可恢复；
- 通过 TTL/heartbeat 判定已被操作系统释放的本地进程 owner。
