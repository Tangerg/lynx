# Lyra Runtime 重构实施计划

> 状态：P1–P12 已完成；服务端与 Agent Framework 自洽，消费者接线留给独立专项
>
> 工作方式：原模块内治本重构，按可验证纵切分批完成；不创建完整 `runtime2`

本文是唯一实施计划和进度台账。架构、ADR、工程标准和能力事实由各自文档拥有，这里只记录阶段目标、依赖、验收和完成事实。

## 1. 当前授权范围

当前实施 goal 已授权：

- 严格按本文 P1–P12 的依赖顺序重构 `app/runtime`；
- 每完成一个可独立验收批次，同步本计划和 Capability Ledger，统一验证、提交并推送；
- 允许服务端内部与 Runtime Protocol breaking change，不建立兼容路径；
- 仓库历史与能力台账中的原框架实现只作为证据；Runtime 对 Framework 只依赖当前 `agent` 公共合同。

本 goal 不修改前端、TUI、CLI，也不为它们保留兼容字段；消费端接线在服务端完成后专项处理。跨出 `app/runtime` 的 Framework 合同变化仍需单独满足 P7 的证据与授权约束，不能在 Runtime 内伪造替代合同。

## 2. 全程约束

- 只修改 `app/runtime` 及该阶段不可避免的直接后端爆炸半径；前端、TUI、CLI 在独立 consumer 阶段处理；
- breaking change 允许，禁止 alias、shim、dual read/write 和两套长期路径；
- 每个阶段按依赖方向从内到外推进，但每个提交必须形成可运行纵切，不允许长期红仓；
- 旧实现可以作为事实证据，不能成为新 API 兼容规范；
- Agent Framework Baseline 15 是当前 Framework 合同；Runtime 不读取其 private state；
- 每批完成后更新本计划和 Capability Ledger，运行对应质量门禁，提交并推送；
- 如果发现 Agent Framework 缺口，先证明它是中性 Framework 能力且已有真实 Runtime consumer，再单独走 Agent Framework ADR/baseline；禁止在 Runtime 侧补第二套内核；
- 阶段完成不以文件数量或目录形状判断，只以验收合同和旧 owner 删除判断。

## 3. 阶段总览

| 阶段 | 目标 | 依赖 | 状态 |
|---|---|---|---|
| P0 | 文档、事实和边界基线 | 无 | 已完成 |
| P1 | 目标依赖 DAG 与迁移守卫 | P0 | 已完成 |
| P2 | Run 领域语言与 bounded contexts | P1 | 已完成 |
| P3 | Application root use cases 与候选消费端口 | P2 | 已完成 |
| P4 | 原生 Interaction 的 root 纵切 | P3 | 已完成 |
| P5 | 权威 model/tool observation 与 Tool 接线 | P4 | 已完成 |
| P6 | waiting、checkpoint、restore、resume、steer | P5 | 已完成 |
| P7 | Delegate child Run 与 waiting subtree | P6 + 两项 Agent Framework 中性合同 | 已完成 |
| P8 | terminal、recovery 与跨聚合一致性收口 | P7 | 已完成 |
| P9 | Adapter/Infra/共享原语/Delivery 结构收敛 | P8 | 已完成 |
| P10 | 协议、生成物与服务端 API 收口 | P9 | 已完成 |
| P11 | 原框架实现删除与唯一模块名替换 | P10 | 已完成 |
| P12 | 全量质量验收与消费者接线移交 | P11 | 已完成 |
| P13 | 重写后精修与双向边界复审 | P12 | 进行中（3/4） |

## 4. P0 — 文档、事实和边界基线

### 目标

在任何生产改动前，使两个独立实现者读完文档后会得到同一所有权、同一术语、同一阶段顺序和同一完成标准。

### 工作项

- P0-01：审计当前 Runtime package 面、旧 Agent imports、协议制品、schema epoch 和现有架构门禁；
- P0-02：建立目标架构和统一语言；
- P0-03：建立 ADR 台账；
- P0-04：建立工程实施标准；
- P0-05：建立本阶段计划；
- P0-06：建立能力迁移台账；
- P0-07：建立 contract/boundary baseline；
- P0-08：更新 `CLAUDE.md` 与 `doc/README.md` 路由，标记旧文档的当前/历史地位；
- P0-09：执行独立 Go spec review，清除 blocker、TBD、冲突链接和多重真相源。

### 验收

- 六份核心文档职责互斥且互相可导航；
- 明确不创建 `runtime2`；
- 明确 Agent Framework/Runtime、Run/Process、Conversation/Transcript/WorkingContext 边界；
- 每个生产能力都有 retain/refactor/rewrite/remove verdict；
- 实施阶段有依赖、输出、行为验收和删除条件；
- 本轮由本 goal 产生的变更只有 Runtime 文档；工作区中并存的用户改动保持未触碰、未纳入；
- 无 TODO、TBD、placeholder 或尚未裁决的 blocker。

## 5. P1 — 目标依赖 DAG 与迁移守卫

### 目标

先让架构测试表达目标边界和迁移期间允许的最小例外，防止实现一边迁移一边扩散旧依赖。

### 工作项

- 将当前 architecture tests 按 Target/Temporary 两类整理；
- 冻结 Domain、Application、Adapter、Infra、Delivery、Bootstrap 的允许边；
- 新增 Agent Framework import allowlist，默认只允许 `adapter/agentexec` integration leaf；
- 禁止 Delivery import concrete adapters，禁止 Infra import Application/Adapter；
- 建立旧 `agent` import census guard，迁移期间只允许数量单调不增，P8 切换后归零；
- 建立旧术语、compat path、private snapshot decoding 和第二 lifecycle owner 的静态守卫；
- 为最终移除 `component` 杂物层建立逐包 owner ledger，不先机械搬迁。

### 验收

- 新增一个错误 import 的 fixture 会稳定失败；
- allowlist 不使用过宽 prefix 掩盖未来扩散；
- 当前代码在明确 temporary exception 下通过；
- 每个 exception 绑定删除阶段和确切 owner。

## 6. P2 — Run 领域语言与 bounded contexts

### 目标

清除产品 `execution` 与 Framework `Execution` 冲突，建立可以独立于 Agent Framework 表达的产品领域模型。

### 工作项

- `domain/execution` 一次性改为 `domain/run`；
- 将 Conversation、Transcript、Interrupt、Accounting 按目标 ownership 提升为准确 package；
- 保持 Knowledge 独立；
- 收敛 Run、Segment、Outcome、Lineage、Limits 和 Capabilities；把 executor bridge value 留给 P3 Application boundary；
- 删除 Domain 内的 Store/Client/context-based I/O port，将真实消费接口移到 Application；
- 让 entity 自己保护状态迁移、terminal first-wins、usage monotonicity 和 checkpoint expectation；
- 将 deadline 建模为独立 TimedOut outcome，并区分 Completed、Canceled、Failed、MaxBudget、MaxSteps 与 recovery Lost；不把这些细因压成 error 字符串；
- 修正 Goal、Session、Plan、Schedule 等下游引用，不复制 ID/value type；
- 按新 owner 更新 SQLite mapping 和 protocol projection 编译面，但不改变外部 shape，除非新语义必须。

### 行为验收

- Run 跨 Segment 保持 identity；
- 非法状态迁移、usage regression、lineage/capability 矛盾在 Domain 边界失败；
- Conversation truncate/seed 与 Transcript rollback 是独立行为；
- Domain 对 Agent/Agent Framework、I/O framework 和 Delivery 零依赖；
- 旧 package path 和 terminology 全部删除。

## 7. P3 — Application root execution use cases 与候选消费端口

### 目标

从产品用例重新推导最小 root executor boundary，而不是把当前 `ExecutionControl` 换一个实现或提前设计尚无消费者的全部能力。

### 工作项

- 重新建模 Start、Observe、Cancel 和 Release 的 root 用例；Wait/AnswerInterrupt/Steer/Recover、child/subtree 只冻结应用语义与禁止泄露边界，精确 port 分别由 P6/P7 真实消费者发现；
- 将当前 `PrepareStart`、`Activate`、`Prepare` 等宽泛阶段拆成有准确事务语义的命令或删除；
- 定义 Application-owned executor reference、member identity、checkpoint envelope 和 executor fact；
- 明确 root admission、child admission、terminal、checkpoint 和 publish 的顺序；
- 为 waiting subtree 记录纯应用输入/输出和 transaction invariant，不在 P3 预造 capability shape；
- 将 Run pump 保持为产品 fact reducer，不让它推进 Framework；
- 收窄 Session/Run/Transcript/Checkpoint persistence ports，删除胖 transaction facade。
- 为保持每批可运行，旧 Agent adapter 可以直接实现新的 Application port，但不得增加中间 compatibility facade；该 concrete implementation 明确在 P8 随旧执行路径删除。

### 行为验收

- fake executor 可以证明全部应用状态机而不导入 Agent Framework；
- executor error、cancel source、deadline、lost checkpoint 映射为稳定产品结果；
- transaction failure 不会发布未提交 Run event；
- cancel/resume/terminal 竞争只有一个合法结果；
- Application 不读取 wire DTO、SQLite type 或 Framework state。
- P3 不冻结完整 executor port；root 候选必须允许 P4 真实 consumer 修订，完整 shape 到 P8 production cutover 才冻结。

### 完成事实

- root seam 已拆为 `RootExecutionStarter`、`ExecutionObserver` 与 `ExecutionReleaser`；启动顺序固定为 validate → side-effect-free stage → attach observation → durable opening → register → begin；任何 admission 前失败只释放 executor resource，不伪造产品取消；
- Run pump 保持唯一 application fact reducer；非 Waiting 的每个终止边界恰好 release 一次，Waiting tree 继续由 active owner 持有；
- `SessionLifecycle` 与 `Effects` 胖接口分别拆为真实 use case 消费的 reader/committer ports；组合 struct 只服务 Bootstrap，Coordinator 不保存 facade；
- Application 统一采用 `ExecutorMember`/`MemberID`，旧 Framework `ProcessID` 只留在待 P8 删除的 adapter 内部；SQLite technical record 同步采用 `root_member_id`/`memberId`，schema epoch 提升到 59，不保留旧列或 dual codec；
- P6/P7 尚未纵切的 continuation、steer、subtree 能力只作为隔离的旧生产路径消费 seam 存在，不属于 P3 root baseline，也不约束后续 Agent Framework consumer shape；
- architecture tests 已锁定当前 root candidate、Application Framework vocabulary 禁区、opaque checkpoint 字段和 storage exact shape。

## 8. P4 — 原生 Interaction root 纵切

### 目标

用 Agent Framework 原生 Interaction 跑通最小真实 root Run：start、模型输出、terminal、cancel。

### 工作项

- 在现有 `adapter/agentexec` 路径内建立新实现，不创建长期 `agentexec2`；
- 直接构造 Interaction Definition/Dispatcher/Deployment；
- 使用每 Run 独立 Engine、exact resolver、limits、capabilities、Event/Delta listeners；
- 从 Host Conversation 构造初始 Input；
- 把 Result/Termination 映射为 Application executor facts；
- 新路径从一开始就不实现 root GOAP 单 Action wrapper、TurnProcess 或第二 controller；
- 建立旧 owner 的精确删除清单，但生产旧路径仍被使用的文件留到 P8 原子切换同批删除；
- 先用独立真实 consumer harness 装配新路径；P4–P7 期间生产路径继续使用旧实现，直到 P8 能力齐备后一次切换，不能让半能力 adapter 接管真实 Run。

### 行为验收

- 真实 Agent Framework Engine + Interaction 完成一次 root Run；
- normal completion、model failure、deadline、cancel、panic isolation 有稳定映射；
- final Output 不依赖 Delta 拼接；
- 旧 Agent import 数量不增加，P8 原子切换后归零；
- 不存在两个同时控制同一 Run 的 execution loop。

### 完成事实

- 在现有 `adapter/agentexec` 内新增 `InteractionExecutor`，每个 staged root 独占一个 Agent Framework Engine、exact Deployment resolver、root admission guard、显式 Framework/Tree limits 和 bounded Delta listener；未创建 `agentexec2`、GOAP wrapper、`TurnProcess` 或第二执行 loop；
- P3 root candidate 经真实 consumer 验证继续成立：Stage 只解析完整 WorkingContext、选择 client 并组装 Definition/Dispatcher/Deployment/Engine/Input，零模型/Tool 调用；Observe 单消费者先 attach，durable opening 后 Begin 才调用 `Engine.Start`；
- Application 增加准确的 `ConversationReader`，在 admission gate 内读取 Host Conversation 并追加当前已校验 user message，形成 fresh `WorkingContext` seed；Agent adapter 不读取 Conversation store，Process 开始后也不回读可变 Host history；
- 成功终态从 `Process.Await`/`Result.Output` 投影 `AssistantMessageCompleted`，Reducer 用完整 final message 覆盖 partial/missing Delta；Delta 只投影临时 text/reasoning increments；
- Agent Framework Termination 集中映射为 Completed/Canceled/TimedOut/Failed/MaxSteps，模型 external failure、Host cancel/deadline、Framework panic cause 和 dispatcher panic isolation 均有确定测试；
- 初版 Engine 明确不配置 `PreparedStepAcknowledger`；P4 新路径仅由独立真实 harness 消费，Bootstrap 生产 wiring 仍使用旧 Agent owner，精确旧 import/delete ledger 未增长并保留到 P8 原子切换。

## 9. P5 — 权威 model/tool observation 与 Tool 接线

### 目标

恢复普通非挂起路径的完整产品 Transcript、usage、pricing、activity、自动 approval decision 和 hooks，同时不污染 Framework。需要用户输入的 approval/HITL 属于 P6。

### 工作项

- 使用 `ModelInvocation`/`ToolInvocation` 做 Process/Effect/ToolCall 精确归因；
- model stream wrapper best-effort 投影 chunk，并同步 durable 投影完整 final/usage；
- Tool decorator 同步投影 ToolCall、result、timing、presentation、approval 和 hooks；
- 为每个 EffectRequest 建立 context-scoped dispatch-attempt tracker：pre-call write 失败不外呼，post-call authoritative write 失败由 outer Dispatcher 返回 error 形成 unknown settlement；
- 将通用 Toolset 与 Agent Framework import 解耦；
- 使用 Interaction `Tools`/`DeferredTools` 冻结 manifest；
- 用 agentexec integration 调用 `AdvertiseTools`，保持 discovery Tool 通用；
- 对比现有 doom-loop/offload/history partition 行为，保留产品语义、删除旧 Framework 补偿。
- 接入 live unknown reconciliation：Dispatcher 直接唤醒 + 有界 public `UnknownEffectIDs` 对账，先提交 RunLost/incomplete/cleanup intent，再 Kill/release；P5 完成 ordinary root/tool 路径，P8 统一 tree-wide terminal transaction。

### 行为验收

- Delta drop 不丢 Transcript 或 usage；
- Tool 并发完成乱序不改变 canonical ToolCall 顺序和因果身份；
- deferred advertisement 在普通成功/失败路径正确提交；需要 HITL 的 rollback/restore 移到 P6；
- 自动 approval allow/deny、hook、pricing 保持 Runtime-owned；
- model started write failure、Tool started write failure、外部成功后 final/result/usage write failure、chunk drop 和并行 Tool batch partial write failure逐项有行为测试；
- 外部调用后 authoritative write 失败留下 incomplete/unknown Transcript fact，不伪造 Tool result，Effect 不自动重放；
- live unknown 不永久挂起：listener 丢失仍被 reconciliation 发现；RunLost transaction 失败可重试且不会提前 Kill；unknown 与 cancel/deadline 竞争有唯一 Lost-first 映射；
- Toolset production 对 Agent Framework 零 import。

### 完成事实

- outer Dispatcher 为每个 Agent Framework Effect 建立 context-scoped、按 EffectID 校验且并发安全的 dispatch-attempt tracker；model/Tool started 必须先完成 Application receipt 才跨过外部调用边界，post-call final/result/usage commit 失败统一使整个 Effect 进入 unknown，pre-call 失败保证零外呼；
- Application Run pump 仍是唯一 reducer/persistence writer。authoritative fact 使用同一 executor stream 的 commit/receipt handshake 和 speculative reducer，完整 Transcript、invocation journal、Run metrics 与 live publication 只有在一个 write-set 成功后才生效；
- SQLite epoch 61 新增 model/tool invocation operational journals。它们只保存 attempt started/completed/failed/unknown/incomplete 边界，不复制语义 final/result；完整 model final + cumulative usage/pricing + Run progress 同事务提交，Tool final Items 与 invocation terminal 同事务提交；
- 并发 Tool 的 start 不占用 Transcript 顺序；pump 暂存乱序完成结果，只在形成模型声明顺序的连续前缀时批量提交，并一起结算所有 receipt。批量失败会丢弃 speculative results，保留 started journal，随后以 incomplete Tool Items + `RunLost` 原子收口；
- Toolset 使用唯一 framework-neutral `toolset.Manifest`；现有 `toolset.Resolver` 直接满足 native Interaction 的消费端口，Runtime execution scope 同时绑定到 manifest resolution 和实际 Tool context。`search_tools` 通过精确 callback 调用 `AdvertiseTools`，Toolset 对 Agent Framework 零 import；
- 真实 Tool decorator 保留 safety、自动 allow/deny、argument rewrite、activity/presentation、hooks、result offload、mutation paths 与 doom-loop policy。canonical Tool result 是 settlement truth；可重新读取的 live projection 和 post-hook 失败只进入观测，不把已经确定的 Effect 改成 unknown；
- model chunk 经过有界 best-effort 队列；Runtime 本地 drop 产生 OTel event，完整 final/usage 独立于 Delta 提交。慢消费反例证明 chunk 丢失不损坏 final 或 usage；
- live unknown 使用 Dispatcher 直接 wake 与有界 public `UnknownEffectIDs` polling 双通道。Run pump 在 release 前原子提交 started/incomplete diagnostic 与 `RunLost`，终态写失败持续重试；丢 wake、写失败重试和最终 release 顺序均有行为测试；
- P5 新路径仍只由真实 Agent Framework harness 消费，Bootstrap 生产 owner 保持旧 Agent 路径，等待 P8 原子切换；旧 owner 没有扩散，P5 没有引入第二执行 loop 或兼容路径。

## 10. P6 — Waiting、Checkpoint、Restore、Resume 与 Steer

### 目标

用 Agent Framework 公共 pending-input 和 snapshot 合同替换旧 suspension/continuation 私有解释。

### 工作项

- 通过 Interaction public helper 识别 pending tool input；
- 在 tree quiescent boundary 捕获 TreeSnapshot；
- 组装 Application checkpoint metadata + opaque payload；
- 按 exact Deployments 和 BuildID/Host expectation 恢复 tree；
- 将产品 interrupt answer 编码为 Interaction response signal；
- 将 SteerRun 编码为 steer signal，并明确下一安全 Step 的生效语义；
- 接入需要用户输入的 approval/ask-user HITL，并验证 deferred advertisement 在 wait/restore 前后保持 owner state；
- 新路径不读取旧 suspension private JSON、ProcessSnapshot codec 或 Continue/Resume 猜测路径；仍服务生产旧路径的对应文件加入 P8 原子删除清单；
- 初版明确不配置 Agent Framework `PreparedStepAcknowledger`；只持久化 quiescent complete-tree checkpoint，不声明 active-step crash recovery；
- 回答 Interrupt 时同一事务记录 exact answer claim、把 interrupt row 变为 `resuming`/普通读取不可见并删除旧 checkpoint；成功后才 stage live tree 或 RestoreTree，next-Segment opening transaction 证明 claim 后才 deliver semantic Signal；
- 恢复探测 unresolved unknown Effect 时拒绝自动恢复并返回准确不可恢复事实，不调用任意 `ResolveEffect`。

### 行为验收

- 未回答 waiting、已回答待继续、restore 后 resume/continue 均有真实测试；
- corrupt payload、wrong BuildID、wrong DeploymentRef、外部 workspace 失效 fail closed；
- unresolved unknown Effect 不重放、不伪造结算，进入 RunLost recovery；
- checkpoint 与 Pending/Run write-set 原子；
- conversation 在等待期间变化不会被静默用于恢复；
- steer 只在 Strategy 安全边界消费且延迟可观察。
- answer claim 前 crash 仍可恢复 waiting checkpoint；claim 后、RestoreTree 后、Signal accepted 后到下一 quiescent checkpoint 前 crash 均 RunLost，绝不重放旧 answer；
- active step crash 不从单 Process Snapshot 猜测 tree recovery。

### 完成事实

- Native Interaction 只通过 public pending-input helper、`TreeSnapshot`/`RestoreTree`、typed response/steer Signal 实现 waiting/restore；P12 将只有单一消费者的 codec 子包及重复 decoder 收回唯一 `interactioninput` ACL，新路径对旧 Agent 零 import；
- quiescent waiting reconciliation 从 public Process status + Strategy helper 捕获完整 tree，并将 opaque TreeSnapshot、Pending、Run/Items 作为 Application tree barrier 原子提交；Runtime 未配置 `PreparedStepAcknowledger`，unknown Effect 无法被捕获为 recovery point；
- `ClaimResume` 在 SQLite epoch 62 内原子记录答案、`open -> resuming` 并删除旧 checkpoint。Continuation opening 必须在事务内 `RequireResumeClaim`；下一 barrier 只有 exact Session/executor/root-member owner 可以替换，terminal/boot recovery 删除 hidden claim；
- continuation 使用 `StageContinuation`/`BeginContinuation`：live waiting tree 必须匹配已提交 checkpoint，cold restore 必须匹配 BuildID、Host scope、TreeSnapshot root/status、exact DeploymentRef 与 active WaitID；Application opening commit 早于 Signal；
- post-claim pre-opening failure 先提交 root `RunLost` 再 release staged tree。RunLost write 失败保持 tree/claim；release 失败与原 cause 一起报告。boot 对任何无 checkpoint 的 claimed-resume tree 都确定收口为 Lost；
- 真实 Runtime `ask_user`、interactive approval、approval argument/remember resolution、doom-loop HITL 与 deferred advertisement 均复用产品 Interrupt contract。Approval restore 不重跑 pre-hook 或 plan；
- steer 使用 `RunningExecutionSteerer.SubmitSteer`，Agent Framework 在当前 model call 之后的下一 safe boundary 消费；产品 `SteerMessage` 在首个能看到它的 `ModelCallStarted` 前提交；
- corrupt TreeSnapshot、wrong BuildID/DeploymentRef、missing/isolated workspace、capability mismatch、claim result drift、conversation change、unknown checkpoint prohibition 和 answer/release ordering 均有真实行为或 SQLite transaction test。

## 11. P7 — Delegate child Run 与 waiting subtree

### 目标

让 managed Delegate child Process 与产品 child Run 形成唯一、可恢复的因果映射。

### 工作项

- P7 的第一项是补齐并冻结两个中性 Agent Framework 前置合同：admitted child 的 conclusive started/aborted outcome，以及在 Apply/Discard 前冻结 source tree 的 one-shot prepared change；在此之前不得接 durable child/subtree production path；
- 实现 context-aware `ProcessAdmitter` adapter；
- root/child admission 使用 prospective Process identity 和 stable StartedAt；
- Delegate ToolCall durable commit 必须早于 child opening reservation；admission 只创建不可见 Opening，conclusive started fact 后才公开 Running；
- 使用 `DelegateChildKey` 关联父 ToolCall 和 child Run；
- 持久化 child Run binding，但不复制 Framework topology/snapshot；
- restore 已有 binding 时不重复 admission；
- agentexec concrete 持有 prepared tree change；Application 只消费 canceled/paused member、opaque resulting checkpoint 和 Apply/Discard capability，执行 prepare → transaction → Apply/Discard；
- 区分 Delegate product child 与 Workflow/Planning framework-internal child；
- 新路径不实现旧 ChildOpeningRequest、Blackboard、configure-child 或自建 subtree controller；仍服务生产旧路径的 owner 加入 P8 原子删除清单。

### 行为验收

- admission 拒绝前零 Process 发布；admission 成功后的 started/aborted 对同一 prospective identity 完整且无 ghost Opening；
- child start/terminal/cancel 与父 ToolCall 因果稳定；
- 多 child、嵌套 child、restore、并发 completion 不产生重复 Run；
- waiting child cancel 的 durable state 与 applied tree 完全一致；
- transaction failure Discard 且 live tree 不变；commit 后 Apply 精确一致，commit 后 crash 恢复 resulting checkpoint，无法证明的 apply failure 丢弃旧 tree并恢复该 checkpoint，失败才 RunLost；
- Framework 仍对 Run/transaction/store 零感知。

### 完成事实

- Agent Framework 以中性 `ProcessStartOutcomeAcknowledger` 闭合 accepted admission 后的 started/aborted 结论；Runtime 在 Delegate ToolCall durable commit 后创建不可见 child opening reservation，收到同一 prospective member 的 conclusive started fact 后才公开 Running，aborted 则只闭合 reservation；
- `InteractionExecutor` 从 Interaction-owned typed active-child inspector 投影稳定 model-call/tool-index/ChildKey/Process identity，不读取 Strategy private state；多 child、nested child、sibling 乱序完成与 restore 已验证不重复创建产品 child Run；
- child opening persistence 使用单一 transaction boundary，public `CommitStartedChildRun` 不在外层 transaction 内重入另一个 public transaction method；non-reentrant transaction 反例已锁定该所有权；
- Agent Framework Baseline 14 将 prepared waiting-subtree 变换收敛为 contextless one-shot `Apply()`：全部可失败、可取消的 staging 在 Prepare 内完成，Host durable decision 之后请求取消不能撤销提交；Agent Framework 仍不感知产品 Run、Store 或 transaction；
- agentexec concrete 独占 prepared Framework capability；Application 只消费 canceled/paused member projection、opaque resulting checkpoint 与 Apply/Discard/Continue。transaction failure Discard；commit 后 Apply 只安装 resulting state；移除最后边界时独立 Continue 才激活已提交 Segment。无法证明 Apply 成功时先 release obsolete owner，再通过 `WaitingExecutionRestorer` 精确恢复 committed checkpoint，恢复失败才 durable `RunLost`；
- P7 重复门禁暴露并治本修复旧 turn shutdown 的 stale-attempt 竞争：caller deadline 只结束当次等待，`turnState.done` 才是资源释放真相；旧 attempt 的 context error 不再污染已经完成的后续 join；
- P7 新增 child integration 没有堆进新的巨型文件：production 按 Delegate binding、child admission、child projection 分文件收敛，集成测试按核心执行、restore、waiting subtree 与 fixture 分离；全部保持同一 `agentexec` package，不为文件拆分制造新 package 或接口；
- P7 的 parallel harness 已在 P8 原子切为生产 owner；该阶段保留的旧 child controller、GOAP wrapper 和 suspension codec 已全部删除。

## 12. P8 — Terminal、Recovery 与跨聚合一致性收口

### 目标

统一正常终态、取消、run_lost、启动恢复、online checkpoint loss 和 rollback 清理路径。

### 工作项

- 汇总 P2 taxonomy、P4 root、P6 waiting/recovery、P7 child 逐步验证结果，用 Agent Framework Termination + Application control intent 完成并行为冻结完整 product outcome matrix；
- 收敛 terminal write-set、checkpoint deletion、Pending cleanup、Transcript repair、Goal reporting；
- 将 P5 live unknown 收口扩展到完整 root/child tree，并与 cancel/deadline/terminal first-wins matrix 合并；
- 重审 isolated workspace、BuildID、Session cwd/isolation 和 rollback 的恢复 policy；
- 启动 recovery 只读取 Application facts，通过 agentexec probe opaque checkpoint；
- 删除 root-only 与 tree-wide 重复 terminal transaction；
- 在全部执行能力纵切通过后一次切换 Bootstrap 到 Agent Framework 实现，并删除旧 root GOAP wrapper、Engine facade、TurnProcess、turn controller 和其余生产 execution path；
- 覆盖 crash point：quiescent capture 前后、waiting commit 前后、answer claim 前后、subtree commit 后 apply 前、terminal commit 前后；不包含未启用的 prepared-step ack。

### 行为验收

- 每种终态 cause 有唯一产品映射；
- terminal transaction 失败保留可重试的完整 aggregate；
- run_lost 清理不留下孤儿 Pending/checkpoint/child Run；
- rollback/delete/restore 会清理其作用域内 parked Process；
- boot recovery 与 online recovery 使用同一领域不变量。

### 完成事实

- Bootstrap、boot recovery、Run opening/continuation/cancel/release 已统一装配 `InteractionExecutor`；旧 Agent module dependency、root GOAP wrapper、Engine facade、TurnProcess、turn controller、private tree/suspension codec 及其旧测试已物理删除，Runtime 对旧 Agent import 为零；
- fresh root 由 Application `WorkingContextComposer` 读取产品 Conversation、Knowledge、Plan、Memory 与 hooks，输出完整模型上下文；Agent Framework 只接收中性 WorkingContext/Tool/Deployment，不读取产品 Store，也没有新增 Runtime 抽象；
- 产品取消先提交 control intent，再由 `RunningRootCancellationRequester` 请求 Framework 在安全边界停止；Run pump 持续观察到确定终态，`ExecutionReleaser` 只释放资源，不再把请求 context 取消混成产品终态；
- terminal matrix 已覆盖 completion、root/parent/host deadline、root/parent cancellation、model-call limit、strategy/external/contract/panic failure 与无意图 Engine kill；live/recovery unknown 均在 release 前 durable 收口为 `RunLost`；
- checkpoint 只接受带 exact build/deployment/workspace/model/limits/capabilities 的 Agent Framework public complete-tree snapshot；answer claim 后旧恢复点失效，active-step crash 不伪装为 effect-level durable recovery；
- Toolset 只暴露 framework-neutral `Manifest` 与精确 `ToolAdvertiser` capability；Agent Framework advertisement 由 agentexec 在调用边界注入，通用 Toolset 对两个 Framework 均零依赖；
- child opening 只保留 admission reservation → conclusive start outcome 的生产路径；旧 `ChildOpeningRequest`/rehydrate shadow path 删除，持久化测试改用 Runtime 自有 opaque executor-tree fixture；
- 删除旧执行路径后留下的空目录、漂移术语、只服务旧路径的 fixtures 与 temporary architecture exception 已清零。

## 13. P9 — 外环结构收敛

### 目标

在执行语义稳定后清除目录和包装坏味道，不把结构调整与核心生命周期调试混在一起。

### 工作项

- 逐包审计 Adapter/Infra：删除纯转发，保留真实 translation/mechanism；
- 逐包处置 `component`：归还 owner 或提升为准确共享 capability package；
- 收敛 `agentexec` 内部文件/package，只有证明独立变化原因才拆子包；
- 审计 application package 的 Coordinator/Service/Manager 口吃和胖接口；
- 复核 Delivery `server`/`dispatch` 的依赖与职责，保留准确现名并删除越界行为；
- 删除空目录、旧路径、历史 fixture 和 architecture temporary exceptions。

### 验收

- import graph 满足最终 DAG；
- 无 `component/common/core/utils` 杂物分类；
- 无一层只转发同名方法的 wrapper；
- package 名和 exported type 无口吃；
- 目录只反映真实 owner，不为对称存在。

## 14. P10 — 协议、生成物与服务端 API 收口

### 目标

让 Runtime Protocol 精确反映新的 Run/Segment/Interrupt/executor 语义，同时保持 transport 独立。

### 工作项

- 审计所有 execution/turn/process 相关 wire 名称；
- 更新 Go contract registry、OpenRPC、JSON Schema、manifest、examples 和 server projection；
- 一次性升级 artifact/schema version；
- 保持 protocol/server/dispatch/transport 职责分离；
- 记录尚未接线的前端/TUI/CLI breaking surface，不添加兼容字段；
- 更新 API/TRANSPORT/AUX_API 语义规范。

### 验收

- contract generator 零漂移；
- strict validator、canonical samples、artifact round-trip 全绿；
- HTTP 与 in-process 行为一致；
- server 无旧字段/method/event；
- consumer backlog 精确列出，不伪装为整体完成。

## 15. P11 — 原框架实现删除与唯一模块名替换

### 目标

完成 Agent Framework 项目最后的消费者迁移和模块替换，只保留一个 canonical module。

### 工作项

- [x] workspace 搜索并确认原框架实现没有剩余 consumer 或独有能力；
- [x] 删除原框架实现；
- [x] 将重写实现原子安装为唯一 `agent` directory/module path；
- [x] 更新 Runtime import、workspace metadata、文档、baseline 和 architecture guards；
- [x] 完成 Agent Framework standalone、Runtime standalone 与 workspace 最终门禁；
- [x] 不保留 alias module、replace compatibility 或双 framework path。

### 验收

- workspace 只有一个 Agent Framework module/path；
- 原框架 symbols、imports、docs 和 module metadata 为零；
- Agent Framework standalone 全门禁和 Runtime 全门禁同时通过。

## 16. P12 — 全量质量验收与消费者移交

### 目标

证明重构后的 Runtime 自洽、无旧债，并把准确协议变化移交给消费者专项。

### 验收矩阵

- [x] Agent Framework 与 Runtime standalone `go mod tidy -diff` 为零，workspace 与 standalone 解析同一 canonical Framework source；
- [x] root/Agent Framework/Runtime 受影响后端 module 的 workspace build/vet/test；Agent Framework 与 Runtime standalone staticcheck、完整 lint；
- [x] Runtime 禁用缓存普通测试、完整 race 和 continuation/prompt/resolution 三个 strict-codec fuzz owner；
- [x] Agent Framework standalone 禁用缓存普通测试、完整 race、13 个 fuzz owner和 8 个真实 command examples；
- [x] SQLite fresh schema、HTTP/SSE、in-process、waiting/restore/recovery/rollback 高风险矩阵在 race 下重复验证；
- [x] architecture DAG、Agent API/wire baseline、Runtime protocol/contract/schema digest 和 generator 零漂移；
- [x] 旧名称、旧 import、compat codec、空目录、tracked 空文件、TODO/FIXME/HACK、死代码、漂移注释与失效本地文档链接扫描；
- [x] 前端/TUI/CLI breaking surface 由 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md) 精确移交，未修改消费者实现。

完成 P12 只表示服务端和 Agent Framework 自洽。消费者接线完成后才能宣称整个产品迁移完成。

## 17. P13 — 重写后精修与双向边界复审

### 目标

在不恢复迁移路径、不修改消费者的前提下，对已经自洽的 Runtime 做第二轮实现级反证审计；只处理真实职责混杂、重复 owner、并发所有权、命名和可变事实漂移，不以指标驱动制造细碎抽象。

### 工作项

- [x] P13-01 清除 Architecture/Capability Ledger/Contract Baseline 的可变阶段与版本漂移，并建立单一版本 owner 的永久门禁；
- [x] P13-02 精修 Application/Domain 的状态变换、聚合行为和 use-case orchestration；
- [x] P13-03 精修 Adapter/Infra/Delivery 的转换、技术机制与协议边界；
- [ ] P13-04 执行 Runtime/Agent Framework 双向抽象泄露复审、全量 race/fuzz/contract/standalone 门禁并冻结事实。

## 18. 进度记录

| 日期 | 阶段 | 完成事实 | 验证 |
|---|---|---|---|
| 2026-08-09 | P13-03（Adapter/Infra/Delivery） | Agent Framework ACL 将 continuation answer 的全量翻译校验置于 Signal 投递前，并统一 streaming 失败的权威投影结算；Run persistence 将 opening admission、model/tool invocation、progress 与 waiting-subtree 原子写集收敛到准确的私有 owner，`Pending` 的持久值相等行为归还 Application，Adapter 不再理解 SQLite 表示差异；Delivery contract validation 按 identity/problem/operation/shape/capability 和 union validation state 分责，清除稳定注释中的阶段、具体存储与具体消费者词汇；archive extraction 以平台无关规则拒绝反斜杠和 Windows drive path；skill lifecycle move 保留同一 capability root 内的精确 `os.Root.Rename`，只分离 replay/outcome reconciliation，避免通用 move 的冲突改名语义破坏稳定 destination identity | 受影响 Application/Adapter/Infra/Delivery packages 的 test/race 全绿；Runtime 全量 test 除精确 owner 门禁随重构迁移后通过，vet/staticcheck/完整 lint/deadcode 零问题；架构 DAG、checkpoint transaction owner、空目录、坏味道标记和外环词汇扫描零违规 |
| 2026-08-09 | P13-02（Application/Domain final audit） | 将 MCP connection replacement 从一个混合 HTTP/stdio/authorization/headers/environment 的条件树收敛为 transport-specific resolver 和三种精确 secret-change 行为，没有引入不透明泛型；全 Application/Domain 复核确认剩余高分支点均是单一状态机、聚合不变量或顺序敏感 saga，继续拆分会产生第二 owner。Domain 生产代码保持 context/I/O/Framework import 为零，Application 端口继续由真实消费者拥有 | MCP 全量测试通过；Application/Domain hygiene 与依赖扫描零违规；Runtime 全量质量门禁通过后冻结 P13-02 |
| 2026-08-09 | P13-02（Application recovery / parked lifecycle） | 将 boot recovery 收敛为一次启动快照专属 planner：库存读取、Pending/checkpoint 唯一性、Session 级缓存、tree preservation/loss 和最终写集各有明确阶段；等待 child cancellation 的长 saga 拆为 admission 后重读、checkpoint continuation、stays-waiting commit/apply 与 resumes-running 四个行为；Session parked terminalization 从 Coordinator I/O 编排中抽出纯写集行为对象，并修正 `Snapshot` 过窄注释。恢复策略仍属于 Application，Run 聚合没有接收 I/O，executor checkpoint 仍完全 opaque | `application/runs`、`application/sessions` 全量 test/race、Runtime vet/staticcheck/lint 全绿；全量回归发现并修正 parked terminalization owner 的硬编码架构门禁后，arch 与全 Runtime tests 通过 |
| 2026-08-09 | P13-02（Application Run lifecycle） | 精修取消与 executor fact 归约链路：以 command-bound source 收敛树读取、live/pending owner 仲裁和计划构造；等待子树取消由纯 builder 分阶段生成 Run/Item/Continuation/Pending 原子写集，删除同一 prepared payload 的重复校验；Run reducer 的入口只编排 fact reduction、projection 与 durable observation，模型调用、工具调用和 Segment 终止分别拥有自身状态行为。所有新增 owner 均为 `application/runs` 私有实现，没有新增端口、协议或 Framework 类型依赖 | `application/runs` 全量 test/race、vet、staticcheck、lint 全绿；等待取消、并发仲裁、reducer 既有回归矩阵全部通过 |
| 2026-08-09 | P13-01 | 反证发现 Architecture 仍携带 P11 实施状态及历史 Agent Baseline，Capability Ledger 的“当前事实”同时保留 P10、Baseline 14 和 SQLite epoch 62，而唯一 Contract Baseline 已是 Baseline 15/epoch 64。治本删除稳定架构中的阶段/版本事实，把当前能力事实同步到唯一合同，并新增文档事实门禁：稳定文档不得出现 phase 状态，Architecture 不得拥有 Framework version，Capability Ledger 的全部当前 baseline/epoch 必须与 Contract Baseline 一致 | 新门禁先稳定复现三类漂移，修正后通过；`git diff --check` 通过。未修改协议、持久化、生产代码或消费者 |
| 2026-08-09 | P12（final acceptance） | 完成 Runtime/Agent Framework 最终质量矩阵；将已完成工具规范从迁移名收敛为唯一 `TOOL_SYSTEM.md`，清除源码/测试中的模糊 vNext 术语；发现并修正 Contract Baseline 的 SQLite epoch 62→64 漂移并建立永久测试守卫；把单一消费者 `interruptcodec`、转发函数和重复 decoder 收回唯一 `interactioninput` ACL。strict continuation codec 现在拒绝大小写 alias、duplicate/unknown field 和 trailing value，并规范化空集合；fuzz 找到的两个反例作为 regression corpus 保留 | Runtime standalone tidy-diff/build/vet/staticcheck、完整 lint、禁用缓存全量 test/race 全绿；3 个 Runtime fuzz owner 单轮执行约 84 万次。Agent Framework standalone 同等门禁全绿，13 个 fuzz owner单轮约 320 万次、8 个 examples 全部实跑。SQLite/HTTP/SSE/in-process/recovery/rollback 关键矩阵在 race 下重复 10 次；root module build/vet/test、contract generator/digest、architecture/API/wire baseline、文档本地链接和最终 hygiene scan 全部通过。P12 完成；消费者仍按 handoff 独立接线 |
| 2026-08-09 | P12（zero-debt audit） | 完整 lint 发现并清除两处嵌入字段冗余选择器和一处复合字面量格式漂移；删除 1935 行已完成架构清洗台账及其索引，把历史实施事实归还 Git，避免已删除类型以“历史文档”形式继续形成第二真相源。当前六份重构 owner、工具规范与带日期外部证据各守唯一职责 | Runtime/Agent Framework `gofmt` 与 `golangci fmt` 零漂移；Runtime `golangci-lint` 零问题、`deadcode -test` 零内部死代码；tracked production TODO/FIXME/HACK、旧 Framework path/类型、旧 replay scope、空文件和空目录扫描为零。P12 继续执行全量行为矩阵 |
| 2026-08-09 | P11（publication completion） | canonical source commit 发布后，Runtime standalone 依赖绑定真实 `github.com/Tangerg/lynx/agent v0.0.0-20260809043847-2590dbc81a1f`；关闭 workspace 与本地 workspace 均消费同一 Baseline 15 源码，未建立 `replace`、alias module、临时 path 或双 Framework 路径 | Runtime `GOWORK=off` tidy-diff/build/vet/staticcheck/test/race 全绿；Agent Framework standalone 与 Runtime workspace 门禁全绿，P11 完成 |
| 2026-08-09 | P11（canonical source publication） | 删除原框架 module，把绿色重写实现安装为唯一 `agent` module；Runtime imports、workspace metadata、architecture guards、Baseline 15 和直接受影响文档同步。删除已完成迁移后仍描述 `agent/runtime`、`agentexec/turn` 的 execution/port 快照，不保留第二套现状 | Agent Framework standalone tidy-diff/build/vet/staticcheck/test/race 全绿；Runtime workspace 全量 test 通过。Runtime standalone 依赖将在 canonical source commit 推送后立即绑定其真实 pseudo-version，故 P11 当前为 4/5 |
| 2026-08-08 | P0 | 只读盘点 Runtime 当前 package、旧 Agent import、Agent Framework Baseline 9、协议制品与 SQLite schema epoch；确认选择原模块内局部绿色重写，不创建 runtime2 | 生产代码未修改；事实写入 Capability Ledger 与 Contract Baseline |
| 2026-08-08 | P0 | 建立并交叉收口六份核心文档，冻结 DDD/Clean Architecture 边界、Agent Framework 防腐合同、P1–P12 依赖、parallel harness/P8 cutover、恢复与副作用失败语义；识别并裁决 P7 两项 Agent Framework 中性前置合同 | 独立 Go spec review 结论 Approved/Ready；本 goal 未修改生产代码；本地链接检查与 `git diff --check` 通过 |
| 2026-08-08 | P1 | 冻结目标六环 DAG；Delivery 开始禁止 concrete Adapter；Agent Framework 只允许从 `adapter/agentexec` 导入两个已批准 public package；旧 Agent import、Domain context I/O ports、`component` umbrella、旧 private snapshot decoder 和唯一旧 lifecycle owner 全部进入精确 Temporary 台账 | 错误 Delivery→Adapter fixture 被稳定拒绝；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P2 | 一次性删除 `domain/execution`：Run、Accounting、Conversation、Transcript、Interrupt 与 ToolResult 按 bounded context 提升；executor ref/checkpoint、pending continuation、workspace mutation 归还 Application；Approval、Agent Memory、Codebase、Hooks、Provider 的十个 context I/O port 全部移到真实 Application consumer，Domain 生产与测试均禁止向外依赖 | Domain context I/O port 从精确十项例外降为零；旧 path、alias、空目录为零；Run 状态/lineage/capabilities、Conversation seed/truncate、usage monotonicity 与 checkpoint expectation 行为测试通过 |
| 2026-08-08 | P2 | SQLite executor checkpoint、pending interrupt 和 workspace mutation 改为 technical records，由 `adapter/persistence` 显式映射 Application values，清除 Infra→Application 反向依赖；终态统一为 Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost，并同步服务端 protocol/schema/generated artifacts | architecture target DAG、Domain test isolation、strict storage codecs、outcome round-trip 与 compatibility differ 通过；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P3 | 删除 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖边界；建立 root stage/commit/begin、observe/release 的消费方端口；Run pump 在所有非 Waiting 边界统一释放；Application executor identity 统一为 Member；SQLite epoch 59 一次性采用 `root_member_id`/`memberId` | fake-backed ordering/race/release/waiting tests 与 architecture vocabulary/port-shape guards 通过；`go test ./...`、`go vet ./...`、`go build ./...` 及 runs/sessions/runsegment/SQLite targeted race 通过 |
| 2026-08-08 | P4 | 在原 `agentexec` 内完成 Agent Framework native Interaction root harness；每 root 独立 Engine + exact Deployment；Application 组装完整 Conversation seed；Result final 与 Delta 分离；集中映射 completion/model failure/cancel/deadline/panic；生产旧 owner 保留到 P8 | real Engine/Interaction integration、stage-zero-side-effect、complete WorkingContext、final-without-Delta、termination/panic/release tests 通过；architecture exact old-import ledger 未增长；`go test ./...` 通过 |
| 2026-08-09 | P5 | 建立 per-Effect dispatch-attempt 与 authoritative commit receipt；model/tool invocation journal、Transcript final、usage/pricing 和 Run progress 原子提交；并发 Tool 乱序完成按模型顺序批量结算；接通唯一 Toolset Manifest、scope、deferred advertisement、approval/hooks/presentation/offload/doom-loop；live unknown 由 wake + polling 收口为 durable RunLost | pre/post-call failure、chunk drop、并发逆序与 batch rollback、lost wake、RunLost retry-before-release、scope propagation、best-effort projection/hook、SQLite transaction/order integration tests 通过；`go mod tidy` diff-free，`go test ./...`、`go vet ./...`、`go build ./...`、`staticcheck ./...` 与 agentexec/runs/runsegment/SQLite targeted race 全绿 |
| 2026-08-09 | P6 | 以 Agent Framework public pending input/TreeSnapshot/RestoreTree/typed Signal 完成 native waiting、exact restore、answer claim、resume、ask-user/interactive approval、deferred advertisement 与 safe-boundary steer；SQLite epoch 62 引入 hidden `resuming` answer audit，opening 强制证明 claim；旧 suspension 仅保留为 P8 production delete owner | live/cold resume、real ask_user、approval hook-once、advertisement restore、corrupt/build/deployment/workspace/capability failure、unknown no-checkpoint、conversation isolation、steer ordering、claim rollback/audit/replacement/terminal/boot cleanup、post-claim RunLost-before-release tests通过；`go mod tidy -diff`、`go test ./...`、`go vet ./...`、`go build ./...`、`staticcheck ./app/runtime/...` 与 Agent Framework/runs/runsegment/runrecovery/SQLite targeted race 全绿 |
| 2026-08-09 | P7 | 以 Agent Framework conclusive start outcome 和 one-shot prepared waiting-subtree change 完成 durable Delegate child Run、nested/sibling causal binding、restore attribution、waiting child cancellation 与 resulting checkpoint recovery；Application 与 Agent Framework 之间只传中性 member projection 和 opaque checkpoint | accepted→started/aborted、admission reject、multi/nested/restore、non-reentrant child commit、prepare/transaction/Apply-or-Discard、Apply/Continue 分相、commit-after-Apply-failure exact restore、restore failure RunLost 与 canceled-request-after-commit tests 通过；旧 turn shutdown 竞争目标测试 100 次、整包 10 次稳定；Runtime/Agent Framework `go mod tidy -diff`、全量 test/vet/build/staticcheck、Agent Framework 全量 race 与 Runtime 高风险 race 全绿 |
| 2026-08-09 | P8 | 原子切换 Bootstrap/boot recovery 到 Agent Framework 原生 Interaction；Application-owned WorkingContext composition、request-cancel/observe/release、tree-wide termination/recovery 与 Tool advertisement 均完成最终纵切；旧 Agent module dependency、GOAP/TurnProcess/turn/suspension/private-tree/duplicate-child 路径及临时例外全部删除；standalone module dependency 提升到实际消费的 Agent Framework Baseline 14 commit | terminal cause matrix、native delegation/waiting/restore/cancellation/unknown、cold restart、opaque checkpoint capability、Toolset advertiser 与 protocol lifecycle tests 通过；`go mod tidy -diff`、全量 test/vet/build/staticcheck、`deadcode -test`、standalone GOWORK=off 全门禁、Agent Framework/runs/runsegment/runrecovery/bootstrap race、cold restart 与 native lifecycle 各 10 次重复验证全绿 |
| 2026-08-09 | P9.1 | 依据真实 import graph 清零 `component` umbrella：path identity、secret masking、notification relay 分别归 Infra/Application/Adapter；pagination/replay cursor、completion/HTTP origin/idempotency/shutdown/taskgroup 以准确 capability 存在；Bootstrap 中长期同步行为移出 composition root；并发 Tool attribution test 改为按稳定 model-call/index 断言而非 goroutine 到达序 | component path/empty dir/temporary ledger 为零；shared capability purity、content-codec boundary、Bootstrap no-business-method、inner-ring comment gates 全绿；`go mod tidy -diff`、全量 test/vet/build/staticcheck、`deadcode -test`、相关 owner/ring race 通过，并发 attribution 100 次重复稳定 |
| 2026-08-09 | P9.2 | 完成 Adapter/Infra/Application/Delivery 逐包职责审计；workspace physical path identity 收敛到唯一 Infra mechanism；删除空的 temporary architecture 台账并把旧 Agent 禁止与 Domain no-context-I/O 变为永久 framework boundary guard；确认 agentexec 按真实变化原因组织且没有第二 lifecycle owner或虚构子包 | Adapter→Infra 单向图与目标六环 DAG 全绿；Delivery concrete Adapter/Infra import、Infra 反向 import、Application outward import、纯转发 wrapper、package/type 口吃、空目录和 temporary exception 均为零；workspacepath/pathidentity/arch targeted tests 与全量质量门禁通过 |
| 2026-08-09 | P10 | Runtime Protocol 一次性提升到 `2026-08-09`、Session Artifact 到 v14；删除 wire 中实现泄露的 `processRootSegment`，只保留准确的 `runtimeInstanceRootSegment`；Go registry、validator、manifest、OpenRPC、JSON Schema、TypeScript binding、canonical samples 与人读 API/Transport/Aux 文档同步；新增精确 consumer handoff | canonical artifact samples 中漏存的 v12 与旧 `outcome.error` 被 strict sample gate 暴露并治本修正；生成器零漂移、旧 wire token/版本归零、strict validator/round-trip、HTTP/in-process、全量质量门禁通过；Desktop backlog 已记录但未改消费者 |

## 19. 当前下一步

P1–P12 服务端重构和 P13 Runtime 分环精修已经完成，当前执行 P13-04。下一批只做 Runtime/Agent Framework 双向边界反证、两个模块的最终实现复核和全量质量冻结；前端、TUI、CLI 与兼容路径不进入本 goal。
