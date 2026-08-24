# Lyra Runtime Domain 模型精修设计

> 状态：已接受专项设计；实施授权与完成事实由 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 和 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 唯一记录
>
> 适用范围：`app/runtime/internal/domain` 及为归还领域行为所有权所必需的 Application、Adapter、Persistence、Delivery 服务端爆炸半径

本文细化 [`ARCHITECTURE.md`](ARCHITECTURE.md) 已接受的 Domain/Application 边界，回答两个问题：当前 Domain 是否因小 package 而“头重脚轻”，以及怎样在不把事务、I/O、Agent Framework 或跨聚合编排塞进 Domain 的前提下，让领域实体真正拥有自己的状态和不变量。

本文不是第二份总体架构或进度台账：稳定边界仍由 Architecture 与 ADR 拥有，强制标尺仍由 Engineering Standards 拥有，实施批次和完成事实仍只进入 Execution Plan 与 Capability Ledger。这里唯一拥有的是 Domain 行为所有权专项的诊断、目标模型和实施验收设计。

## 1. 结论

专项启动时的问题不是 Domain package 数量多，也不是单文件 package 天然错误。真实问题是行为分布不均：

- `run`、`goal`、`approval`、`agentmemory`、`accounting`、`schedule`、`conversation`、`tool` 已拥有真实状态机、决策或纯算法；
- `modelref`、`knowledge`、`feedback`、`interrupt`、`provider`、`toolresult` 等主要表达稳定值，保持精简是正确形态；
- `run` 与 `transcript` 的 Run 所有权曾在实现中分裂；
- `transcript.Item` 曾允许外部先拼装任意字段组合，再靠事后校验拒绝非法状态；
- `plan` 主要是数据结构和包级校验，完整用例落到了 Tool Adapter；
- `session` 只有局部行为，创建、编辑和恢复后的状态变化仍有一部分由 Store 或 Application 直接拼装。

因此，Runtime 要解决的是**局部贫血模型**，不是“整个 Domain 都应当变大”。优化目标也不是提高 Domain 行数占比，而是让每条领域不变量和状态修改只有一个真实 owner。已完成纵切不会在本设计里改写成另一套进度台账；当前事实以 Execution Plan 和 Capability Ledger 为准。

## 2. 判断行为归属的规则

一段逻辑同时满足以下条件时，默认属于 Domain：

1. 它回答的是产品领域问题，而不是技术执行问题；
2. 它只依赖实体当前状态和调用方提供的确定值；
3. 它必须对所有调用入口、Store 和协议保持一致；
4. 它改变单一 aggregate 内的状态，或者计算一个纯领域结果；
5. 它不需要 `context.Context`、I/O、事务、锁、goroutine、SDK 或当前时间的隐式读取。

一段逻辑满足以下任一条件时，保留在 Application：

- 同时读取或修改多个 aggregate；
- 决定 claim、durable commit、external apply、publish、cleanup 的顺序；
- 组织 Run tree、Pending、checkpoint、Goal、Conversation 或 Transcript 的原子 write-set；
- 调用 Store、executor、文件系统、网络或其他外部端口；
- 处理并发仲裁、请求生命周期、重试收口或进程恢复。

Adapter 只负责外部语义翻译和消费端口实现；Persistence 只映射技术记录并执行上层已经决定的 write-set；Delivery 只做协议投影。任何一层都不得因为 Domain API 不完整而成为实体状态的第二修改者。

Domain 构造和状态行为使用显式 `error` 拒绝非法值、非法迁移和 revision/计数溢出；调用方需要稳定分支的失败必须提供准确 sentinel 或 error type，并保持 `errors.Is/As`。只有无失败语义的纯查询才返回 bool。Application 在保留 cause 的前提下补充用例上下文，Delivery 仍是映射 protocol error 的唯一位置。

## 3. 不以大小判断 package

Domain package 是否成立只看以下证据：

- 是否拥有独立、稳定的领域词汇；
- 是否有独立变化原因；
- 是否被多个真实消费者作为同一个语义值使用；
- 合并后是否会制造不必要的横向依赖或循环；
- 拆开后是否仍能完整表达一个行为或不变量。

以下情况不构成合包理由：

- 只有一个生产文件；
- 代码行数少；
- 主要由 enum、identity、validation 或纯值组成；
- 当前行为简单但语义边界稳定。

以下情况才是合包信号：

- package 没有独立词汇，只是另一个领域的字段容器；
- 生命周期永远与另一个 aggregate 同步变化；
- 所有生产消费者都必须同时 import 两个 package 才能完成一个基本行为；
- 为避免循环而复制类型、建立转发函数或使用不准确的中性名称；
- package 只有一个消费者且没有独立变化轴或依赖隔离价值。

本专项先归还行为所有权，再重新审计 package。禁止先按目录大小机械合并，因为那会掩盖真正的边界错误。

## 4. 当前模型裁决

### 4.1 保持充血方向

以下上下文已经拥有真实领域行为，后续只做局部 API 和命名审计，不进行结构性重写：

| Package | 已有核心所有权 | 裁决 |
|---|---|---|
| `run` | lifecycle、outcome、lineage、limits、capabilities、tree topology | 保留并接回完整 Run entity |
| `goal` | objective incarnation、budget、progress、pause/block/complete | 保留 |
| `approval` | rule matching、specificity、risk decision、remember policy | 保留 |
| `agentmemory` | memory fold、review、ranking、search | 保留 |
| `accounting` | usage validation、aggregation、monotonic advance | 保留，并成为 Run metrics 的值来源 |
| `conversation` | seed、truncate、fork watermark | 保留 |
| `schedule` | cadence 与下一触发决策 | 保留 |
| `tool` | canonical invocation/result、安全与风险语义 | 保留 |
| `mcpserver` | server/tool policy 与 canonical schema/name | 保留 |

### 4.2 保持精简值对象

`modelref`、`knowledge`、`feedback`、`interrupt`、`provider`、`toolresult`、`codebaseindex` 等 package 不因方法少而自动判为贫血。只要它们拥有稳定值、不变量或纯算法，并且被真实消费者共享，就保持独立。

不得为这些值对象添加无意义的 setter、Manager、Service 或 Repository，使代码看起来“更 DDD”。没有状态转换的概念不需要伪造状态转换。

### 4.3 必须精修的区域

| 优先级 | 区域 | 根因 |
|---:|---|---|
| 1 | `run` / `transcript` | Run lifecycle owner 与 durable Run carrier 分裂，Application 手工拼接状态和终态事实 |
| 2 | `transcript.Item` | tagged union 可被外部构造为非法组合，状态变化散落在 Application |
| 3 | `plan` | aggregate 行为不足，Tool Adapter 越过 Application 直接验证和持久化 |
| 4 | `session` | 创建、编辑、relocation/restore 后的实体变化未完全由 Session 行为表达 |

### 4.4 Package 边界冻结

逐包复审必须得到可以用一句领域语言解释的 owner，而不是用目录大小解释。当前边界冻结如下；新增 package 必须重新通过同一组词汇、变化原因、消费者和 import DAG 证据，不能把未归属类型放入新的收纳目录。

| Package | 唯一职责与独立存在理由 |
|---|---|
| `accounting` | 模型 token、cost 与累计快照的纯值和单调聚合；被 Run、pricing、usage 共同消费 |
| `agentmemory` | Agent 维护的长期记忆、review lifecycle、fact fold 与 ranking；与人类维护的 Knowledge 分离 |
| `approval` | Tool call gate、remembered rule、scope/specificity 与会话 permission policy |
| `codebaseindex` | code chunk、index status、similarity hit 与纯 ranking；corpus build lifecycle 和持久化不在此处 |
| `conversation` | 唯一的模型上下文 message sequence、seed 和 truncate watermark；不与 Transcript/Run 合并 |
| `feedback` | 可独立保存的 immutable interaction quality signal |
| `goal` | 跨多个 Run 的 autonomous Goal、objective incarnation、budget、progress 和 terminal reason |
| `hooks` | 与 `hooks.json` 一致的 lifecycle Hook vocabulary、matching 和 decision fold；I/O 与 trust orchestration 在外部 |
| `interrupt` | durable HITL kind、stable key 和 semantic resolution；open-set、request validation 和 continuation routing 在 Application |
| `knowledge` | 人类维护的 LYRA.md scope/document value；与 AgentMemory 的所有权和写入来源不同 |
| `mcpserver` | MCP server descriptor、canonical tool identity/schema 与 per-tool policy；connection lifecycle 在外部 |
| `modelref` | zero-or-complete provider/model selection，被 Run、Goal、Schedule 和 model use case 共享 |
| `plan` | Session-scoped ordered Plan replacement、step invariant、revision 和 updated time |
| `provider` | model-provider identity、credential configuration、effective provenance 与 patch vocabulary |
| `run` | 单个产品 Run 的完整 lifecycle、lineage、metrics、limits/capabilities 与 terminal facts |
| `schedule` | cron cadence、saved headless-run intent、occurrence 与下一触发纯决策；Runtime availability/CWD admission 不属于它 |
| `session` | 多 Run conversation identity、lineage、editable aggregate state 与 revision/time；live filesystem projection 不属于它 |
| `skills` | managed Skill library 的 proposal identity、review provenance、lifecycle 与 safety classification |
| `tool` | model-facing Tool、canonical arguments/result、failure 与 safety/workspace mutation vocabulary |
| `toolresult` | 从 inline context 移出的 Tool result identity、stage、ref 与 portable blob lifecycle |
| `transcript` | 用户可见、可审计 Item history、ToolCall settlement、Run timeline 和 rollback/fork boundary |

这份裁决明确保留小而稳定的值 package。`session.WorkspaceIdentity` 一类 live filesystem projection 必须留在 Workspace Application owner；`ErrCWDUnavailable` 也只由该 owner 定义。某项 Runtime 能力是否装配（例如 scheduling unavailable）同样是 Application 事实，不得借错误常量倒灌 Domain。

## 5. Run 与 Transcript 的目标边界

### 5.1 唯一 Run aggregate

`domain/run` 必须成为完整 Run aggregate 的唯一 owner，而不只拥有 `State`、`Outcome` 和若干辅助值。目标实体使用领域本名 `run.Run`；这里宁可接受准确的 Go selector，也不引入 `Instance`、`Data`、`Model`、`Aggregate` 等弱化语义的替代词。

`run.Run` 拥有：

- Session/Run identity 与 immutable lineage；
- frozen model selection、Goal incarnation provenance、limits 和 capabilities；
- lifecycle state 与 active Segment identity；
- cumulative metrics；
- terminal outcome、Run failure、detail、finished time 和 conversation message watermark；
- creation/update time；
- 单个 Run 内所有合法状态转换和终态一致性。

`run.Run` 不拥有：

- executor member、Agent Framework Process、checkpoint payload；
- Conversation messages 或 WorkingContext；
- Transcript Item 顺序和 Item payload；
- open Interrupt 集合、answer claim 或 Pending continuation envelope；
- Store、transaction、clock、context 或 publish 行为。

当前 Run carrier 中的 open `Interrupts` 不应继续成为 Run aggregate 的第二份可变事实。产品 Interrupt 语义由 `domain/interrupt` 拥有；一个 root tree 的 open set、answer claim、executor binding 与 continuation 仍由 `application/runs.Pending` 这个跨聚合 hand-off owner 组合。Run 只表达自己处于 Waiting，不复制 Pending 的完整内容。查询层需要展示 Interrupt 时显式 join，不通过 Run 字段维持影子集合。

### 5.2 Run 行为

Run 的外部可用行为应使用领域动作，而不是暴露字段修改顺序。最终方法名和参数以真实迁移调用点复核，但语义必须覆盖：

- admission/open：从已验证 draft 创建第一个 Running Segment；
- suspend：Running 在合法 interrupt boundary 进入 Waiting，并清除 active Segment；
- resume：Waiting 打开新的 Segment，身份、limits 和 capabilities 不变；
- terminate：以合法 outcome 和 terminal facts 结束 Running Run；
- cancel waiting：Waiting 只允许进入 Canceled；
- recover lost：不可恢复的 Running/Waiting Run 进入 Lost 对应终态；
- advance metrics：累计 usage、steps 和 active duration，只允许单调前进；
- restore：从持久化值重建并完整验证，不允许绕开 aggregate invariant。

这些行为优先采用 value receiver 返回新值，使 reducer、write-set builder 和失败回滚天然使用不可变转换。调用方提供 identity、时间、message watermark、failure 等确定输入；Domain 不隐式调用时钟或 ID generator。

禁止新增 `SetState`、`SetOutcome`、`MarkDone(bool)`、通用 `Update(fields)` 或接收 map/option bag 的状态 API。

### 5.3 Run failure 与 accounting

当前同时承载 Run 和 Tool 问题的通用 Problem 必须按 owner 拆开：

- Run terminal failure 归 `run.Failure` 及其精确 kind；
- Tool invocation failure 归 `tool.Failure`；`toolresult` 只继续拥有大结果 offload identity/stage/blob，不成为通用错误包；
- Delivery 在协议边界把不同领域 failure 投影为 wire problem；
- 不为复用协议 shape 建立 `domain/problem` 公共杂物包。

产品 token、cost 和 model-call 数据继续由 `accounting` 拥有。`run.Metrics` 只组合累计 Run 消耗，并通过 accounting 的稳定值和算法推进；Application 不再维护第二套 usage monotonicity 实现。

### 5.4 Transcript 的准确职责

`domain/transcript` 继续拥有用户可见、可审计的 Item 历史、稳定顺序、rollback/fork boundary 和 ToolCall 时间事实，但不再充当 Run aggregate 的可变 carrier。

Transcript 可以在快照或查询结果中包含 `run.Run` 值，但只能引用权威 Run aggregate 结果，不定义第二套 Run state/outcome 字段，不自行推进 Run lifecycle。

这保持三个事实源的永久分离：

- Run：产品执行生命周期；
- Transcript：用户可见、已发生的观察记录；
- Agent Framework snapshot：执行恢复状态。

## 6. Transcript Item 的充血方式

### 6.1 保留一个 Item 值，不引入类型层级

Go 中不需要为每种 Item 建一套继承式层级。保留统一 `Item` 值是合理的，但其非法组合必须在 Domain 构造边界被关闭，而不是让所有调用方填写公开字段后再记得调用 `Validate`。

Domain 提供按语义命名的构造入口，例如：

- user message；
- agent message；
- reasoning；
- question；
- tool call start；
- compaction。

这些入口接收各自真正需要的参数，并立即验证 identity、time 和 payload。禁止 `NewItem(kind, fields...)` 这种把 tagged union 重新暴露给调用方的通用构造器。

### 6.2 Item 状态行为

只有真实具有生命周期的 Item 才获得状态方法：

- running ToolCall 以结果完成；
- running ToolCall 以精确 Tool failure 结束；
- recovery/cancel 将尚未结算的 running Item 标记为 incomplete；
- 不允许 terminal Item 再次结算；
- `FinishedAt` 只能由 ToolCall 的 terminal 行为设置，且不得早于 `OccurredAt`；
- Tool 真正进入 executor 后，由 Reducer 提供 execution start 与 finish，ToolCall 保存精确 execution duration；审批等待属于可见 Item lifecycle 而不属于执行时间；
- recovery 无法证明 execution interval 时 duration 保持 unknown，不从 `FinishedAt - OccurredAt` 猜测。

Question、message、reasoning、compaction 等一次形成即完成的事实，不为了 API 对称而制造 Start/Complete 方法。

### 6.3 表示与持久化

领域字段应尽可能私有，由构造器、行为方法和准确 accessor 保护。Persistence 使用自己的 technical record 显式映射，不要求 Domain 为 SQL scanner、JSON tag 或 protocol DTO 暴露可写字段。

如果一次性私有化所有字段造成不可审计的巨大纵切，可以按 aggregate 分批完成，但每批结束时被接管类型必须不存在外部直接 mutation；不允许长期保留“新方法 + 公开字段”双写入口。

## 7. Plan 的目标模型

### 7.1 Plan aggregate

`domain/plan.State` 继续表示一个 Session 当前完整 Plan，拥有：

- 有序 Steps；
- revision；
- updated time；
- 完整 replacement 语义；
- step description/status invariant；
- 同时最多一个 in-progress Step；
- empty replacement 表示 clear。

Step 必须通过领域构造或 Plan replacement 验证进入 aggregate；外部不能构造未验证的 Step 切片后直接写 Store。切片跨边界 defensive copy，读取者不能修改 aggregate 内部状态。

revision 是 optimistic concurrency 的领域事实，但 CAS 和事务仍属于 Application/Persistence。Application 读取当前 State，Domain 计算下一 State，Persistence 以 expected revision 原子保存；数据库不得自行发明另一套 Plan transition。

### 7.2 Application 用例

新增准确的 `application/plans` 用例边界，拥有：

1. 接收显式 Session identity；
2. 读取当前 Plan；
3. 调用 Domain replacement；
4. 通过消费方定义的窄端口完成 CAS 保存；
5. 发布已提交的 Plan change；
6. 返回应用结果供 Tool/Delivery 展示。

Tool Adapter 只负责模型参数 decode、从 Tool execution context 提取 Session identity、调用 Application 用例和结果 presentation。Application 不 import Adapter 的 execution context。Tool Adapter 不得 import persistence writer、直接调用 Plan Store、决定 revision 或成为业务错误 owner。

## 8. Session 的目标模型

`domain/session.Session` 应完整拥有单个 Session 内的行为：

- fresh 和 scheduled Session 的构造；
- title/exact provider+model selection/workspace/isolation/favorite edit；
- fork inheritance；
- relocation/restore 后 canonical workspace identity 的安装；
- revision 单调推进和实体时间更新；
- immutable identity、parent 和 started time。

Session 的模型身份不是可缺省字符串，而是 configured `modelref.Selection`。Domain 构造、restore 与 patch
都要求 provider/model 同时存在且无外围空白；zero selection 对 Session 非法。Runtime 全局默认只由
Session Application 在 fresh/scheduled admission 时解析并安装，之后 read model、Run、fork、SQLite 与
Artifact 都读取聚合内同一 pair，不再在消费处补默认或按 model id 推断 provider。显式 Run pair 只有在
executor staging 成功后，才随 Run opening 原子替换 Session pair；失败不得留下半更新。fork 继承父
Session 的 exact pair，而不是清空后等待另一层重新猜默认。

Application 仍负责：

- 通过 filesystem port 解析并验证 workspace；
- ClaimIdleSession/ClaimSessionMutation；
- root Run admission 的跨聚合唯一性；
- CAS transaction；
- rollback、restore、sandbox/checkpoint cleanup；
- publish 和 post-commit resource release。

因此正确顺序是：Application 取得并验证外部值，再调用 `Session` 行为得到下一个合法值，最后请求 Store 持久化。Store 不应同时承担 normalize、business patch 和 persistence 三种职责。

`Patch` 可以保留为领域命令值，但 normalization 与 apply 必须属于同一个 Domain 调用路径，避免调用方先 Normalize、再忘记 Apply 前置条件。不得增加逐字段 setter。

## 9. Application 保留的复杂度

Domain 充血不意味着 Application 应当变薄到只剩转发。以下复杂度是 Runtime 的应用本质，必须保留在准确 use case：

- executor fact 到产品命令的翻译；
- Run tree topology 和 postorder traversal 的跨聚合使用；
- model/tool authoritative commit receipt；
- waiting checkpoint + Pending + Run/Transcript write-set；
- answer claim、resume、subtree cancellation 的 plan/commit/apply 顺序；
- unknown Effect 的 durable RunLost 收口；
- terminal tree + Goal outcome + cleanup intent；
- rollback/fork 与 live executor release；
- publication、subscriber backpressure 和 lifecycle ownership。

`application/runs` reducer 继续存在，但职责收敛为：

1. 接收并验证 executor facts；
2. 选择要调用的 Domain 行为；
3. 生成跨聚合 write-set 和产品事件；
4. 在 durable commit 成功后发布或执行外部 cleanup。

它不再手工修改 Run/Item/Metrics 字段，也不复制 Domain validation。等待子树 builder、boot recovery planner 和 parked terminalization 可以继续作为 Application 私有行为对象，因为它们处理的是跨 aggregate/write-set，而不是单实体行为。

## 10. 明确禁止的错误方向

本专项禁止：

- 把整个 Run reducer、waiting saga 或 recovery planner 移进 Domain；
- 让 Domain import `context.Context`、Store、Agent Framework、SQLite、OTel 或 protocol DTO；
- 为每个 entity 建 Repository/Service/Manager/Factory 层；
- 建 generic aggregate、generic repository、通用 transition engine 或 event-sourcing framework；
- 为“充血”增加只包装字段赋值的方法；
- 同时保留公开可写字段和新行为方法作为两个 mutation 入口；
- 让 Transcript 再定义一套可独立推进的 Run lifecycle；
- 让 Run 复制 Pending 的 open Interrupt 集；
- 让 Persistence 用 SQL transaction 决定业务状态；
- 让 Tool Adapter 直接成为 Plan 或 Goal use case；
- 按文件数量合并 bounded context；
- 建 `common`、`model`、`entity`、`policy`、`domainservice` 等收纳包；
- 为旧内部 API、wire 或 SQLite shape 保留兼容路径。

## 11. 实施顺序

真正实施时，必须先在 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 建立新的授权阶段。建议按以下完整纵切推进，每批独立验证、提交和推送。

### Batch 1：Run aggregate 统一

- 在 `domain/run` 建立完整 Run entity、failure、metrics 行为；
- 把 Run-specific accounting 复用收敛到 `accounting`；
- 删除 `transcript` 中第二个可变 Run carrier；
- Application、Persistence、Delivery projection 改为消费唯一 Run value；
- 删除外部 Run 字段 mutation 和重复 validation；
- 保持 Agent Framework/Runtime 防腐合同不变。

Batch 1 是后续批次的前置，因为 Item recovery、Session parked terminalization 和 Plan/Goal 记录都需要唯一 Run owner。

### Batch 2：Transcript Item 行为统一

- 为各 Item kind 建准确构造入口；
- 收回 ToolCall complete/fail/incomplete 行为；
- 删除 Application 中直接 Item status/time/error mutation；
- Persistence 只映射 technical record；
- 保持 Transcript 顺序和 canonical Tool batch commit 语义不变。

### Batch 3：Plan 纵切

- 让 `plan.State` 拥有 replacement/revision/invariant；
- 建立 `application/plans` 用例和消费方 Store port；
- Tool Adapter 退回参数翻译与 presentation；
- 删除 Adapter 直连 Store 的旧 owner；
- 同步服务端 contract/projection，只在确有 shape 改变时提升版本。

### Batch 4：Session 行为统一

- 建立 fresh/scheduled construction 和 Apply/Fork/relocation 行为；
- Store 改为持久化已决定的下一 Session，而不是内部执行业务 patch；
- 删除 Application 直接构造或修改 Session 字段；
- 保持跨聚合 claim、restore write-set 和 cleanup 在 Application。

### Batch 5：Domain package 复审与永久门禁

- 按独立词汇、变化原因、消费者和 import DAG 重新裁决每个小 package；
- 合并真实无独立 owner 的 package，保留精简但稳定的值 package；
- 删除空目录、转发层、别名、失真命名和漂移注释；
- 将可以由编译器、AST 或 architecture test 表达的行为所有权变成永久门禁。

不得将五个 batch 合成一次无法定位回归的超大提交；也不得把一个 batch 拆成长期存在新旧 mutation owner 的半成品。

## 12. 测试与架构门禁

### 12.1 Domain tests

- Run admission/suspend/resume/terminate/cancel/lost 的合法与非法矩阵；
- terminal outcome、failure、finished time、message watermark 的共同不变量；
- metrics usage/steps/duration 单调性和 overflow；
- Item 各 variant 的构造矩阵；
- ToolCall complete/fail/incomplete 的 terminal first-wins；
- Plan replacement、clear、revision advance/overflow 和最多一个 in-progress；
- Session construct/apply/fork/relocate 的 identity、revision 和 time 不变量；
- slices/maps 的 defensive-copy 行为。

### 12.2 Application tests

- reducer 对相同 executor facts 产生与重构前等价的产品事实；
- recovery、parked terminalization、waiting child cancellation 只通过 Domain 行为修改单实体；
- transaction rollback 不泄露 speculative Domain 值；
- Plan Tool 不再直连 Store，Plan CAS conflict 由 Application 准确返回；
- Session 外部 workspace 验证失败时 Domain 和 Store 均不改变；
- cancel/wait/resume/terminal/unknown 竞争保持既有 first-wins 合同。

### 12.3 Adapter、Persistence 与 Protocol tests

- 每种 Domain aggregate 与 technical record 严格 round-trip；
- invalid record 在 rehydrate 边界失败，不把非法 entity 交给 Application；
- Tool Adapter 只依赖 Application Plan capability；
- Delivery 只读取 immutable Domain/Application projection；
- wire shape 未改变时 contract generator 零漂移；确有 breaking shape 时只存在一个新版本，不双读旧版本。

### 12.4 Architecture fitness tests

必须新增或强化机器守卫：

- Domain 继续对 Application/Adapter/Infra/Delivery/Bootstrap/Agent Framework 零 import；
- Domain 不出现 context-based I/O port；
- Run、Item、Plan、Session 的可变字段不能在 Domain 外直接赋值；
- Tool Adapter 不能直接依赖 Plan persistence port；
- Persistence 不拥有领域 transition；
- 不出现第二 Run/Item state enum；
- 不出现 `common/model/entity/service/manager/repository` 收纳层；
- 当前 Agent Framework import island、Protocol baseline 和 SQLite epoch 守卫保持通过。

## 13. 完成定义

本专项只有同时满足以下条件才算完成：

- Run entity、lifecycle、failure、metrics 只有 `domain/run` 一个 mutation owner；
- Transcript 不再拥有可独立推进的第二 Run 状态；
- Item 非法 variant 不能从正常生产 API 构造；
- Application 不直接修改 Run、Item、Plan、Session 的领域状态；
- Plan Tool 只调用 Application capability，不直接验证并持久化 aggregate；
- Session Store 不再兼任领域 patch engine；
- 跨聚合 write-set、事务、Agent Framework 接线和 lifecycle 仍完整留在 Application/Adapter 正确 owner；
- 每个 Domain package 都能用一句领域语言说明存在理由；
- 没有为了充血新建的空抽象、setter、generic framework 或 Java 式层级；
- 受影响 GoDoc、架构文档、ADR、Execution Plan、Capability Ledger、contract、schema、fixture 与生成物无漂移；
- Runtime 全量 build、vet、staticcheck、lint、deadcode、test、race、相关 fuzz、contract generator 和 architecture gates 全绿；
- 每批形成可独立 revert 的提交并及时推送，工作区中的前端、TUI、CLI 和用户并行改动保持未触碰。

## 14. 预期结果

完成后，Runtime 不追求“Domain 更大”，而应呈现以下结构：

```text
Delivery        协议投影
    |
Application     用例、事务、跨聚合写集、并发生命周期
    |
Domain          实体行为、不变量、纯领域决策
    ^
Adapter/Infra   外部能力翻译与技术机制
```

Application 仍然会是 Runtime 中代码量和固有复杂度最大的层，但它的复杂度只来自编排；Domain 的代码量可能仍然不大，却完整拥有所有单聚合行为。衡量成功的标准不是目录或行数对称，而是：**任何领域状态变化，都能从一个准确命名的 Domain 行为进入；任何跨聚合副作用，都只能由一个准确命名的 Application 用例组织。**
