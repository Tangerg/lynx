# Lyra Runtime 重构实施计划

> 状态：P5 权威 model/tool observation 与 Tool 接线已完成；下一阶段 P6
>
> 工作方式：原模块内治本重构，按可验证纵切分批完成；不创建完整 `runtime2`

本文是唯一实施计划和进度台账。架构、ADR、工程标准和能力事实由各自文档拥有，这里只记录阶段目标、依赖、验收和完成事实。

## 1. 当前授权范围

当前实施 goal 已授权：

- 严格按本文 P1–P12 的依赖顺序重构 `app/runtime`；
- 每完成一个可独立验收批次，同步本计划和 Capability Ledger，统一验证、提交并推送；
- 允许服务端内部与 Runtime Protocol breaking change，不建立兼容路径；
- 读取旧 `agent` 与 `agent2` 作为实现证据，但 Runtime 对 Framework 只依赖 Agent2 公共合同。

本 goal 不修改前端、TUI、CLI，也不为它们保留兼容字段；消费端接线在服务端完成后专项处理。跨出 `app/runtime` 的 Framework 合同变化仍需单独满足 P7 的证据与授权约束，不能在 Runtime 内伪造替代合同。

## 2. 全程约束

- 只修改 `app/runtime` 及该阶段不可避免的直接后端爆炸半径；前端、TUI、CLI 在独立 consumer 阶段处理；
- breaking change 允许，禁止 alias、shim、dual read/write 和两套长期路径；
- 每个阶段按依赖方向从内到外推进，但每个提交必须形成可运行纵切，不允许长期红仓；
- 旧实现可以作为事实证据，不能成为新 API 兼容规范；
- Agent2 Baseline 9 是当前 Framework 合同；Runtime 不读取其 private state；
- 每批完成后更新本计划和 Capability Ledger，运行对应质量门禁，提交并推送；
- 如果发现 Agent2 缺口，先证明它是中性 Framework 能力且已有真实 Runtime consumer，再单独走 Agent2 ADR/baseline；禁止在 Runtime 侧补第二套内核；
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
| P6 | waiting、checkpoint、restore、resume、steer | P5 | 未开始 |
| P7 | Delegate child Run 与 waiting subtree | P6 + 两项 Agent2 中性合同 | 未开始 |
| P8 | terminal、recovery 与跨聚合一致性收口 | P7 | 未开始 |
| P9 | Adapter/Infra/共享原语/Delivery 结构收敛 | P8 | 未开始 |
| P10 | 协议、生成物与服务端 API 收口 | P9 | 未开始 |
| P11 | 旧 Agent 删除与唯一模块名替换 | P10 | 未开始 |
| P12 | 全量质量验收与消费者接线移交 | P11 | 未开始 |

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
- 明确 Agent2/Runtime、Run/Process、Conversation/Transcript/WorkingContext 边界；
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
- 新增 Agent2 import allowlist，默认只允许 `adapter/agentexec` integration leaf；
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

清除产品 `execution` 与 Framework `Execution` 冲突，建立可以独立于 Agent2 表达的产品领域模型。

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
- Domain 对 Agent/Agent2、I/O framework 和 Delivery 零依赖；
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

- fake executor 可以证明全部应用状态机而不导入 Agent2；
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
- P6/P7 尚未纵切的 continuation、steer、subtree 能力只作为隔离的旧生产路径消费 seam 存在，不属于 P3 root baseline，也不约束后续 Agent2 consumer shape；
- architecture tests 已锁定当前 root candidate、Application Framework vocabulary 禁区、opaque checkpoint 字段和 storage exact shape。

## 8. P4 — 原生 Interaction root 纵切

### 目标

用 Agent2 原生 Interaction 跑通最小真实 root Run：start、模型输出、terminal、cancel。

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

- 真实 Agent2 Engine + Interaction 完成一次 root Run；
- normal completion、model failure、deadline、cancel、panic isolation 有稳定映射；
- final Output 不依赖 Delta 拼接；
- 旧 Agent import 数量不增加，P8 原子切换后归零；
- 不存在两个同时控制同一 Run 的 execution loop。

### 完成事实

- 在现有 `adapter/agentexec` 内新增 `InteractionExecutor`，每个 staged root 独占一个 Agent2 Engine、exact Deployment resolver、root admission guard、显式 Framework/Tree limits 和 bounded Delta listener；未创建 `agentexec2`、GOAP wrapper、`TurnProcess` 或第二执行 loop；
- P3 root candidate 经真实 consumer 验证继续成立：Stage 只解析完整 WorkingContext、选择 client 并组装 Definition/Dispatcher/Deployment/Engine/Input，零模型/Tool 调用；Observe 单消费者先 attach，durable opening 后 Begin 才调用 `Engine.Start`；
- Application 增加准确的 `ConversationReader`，在 admission gate 内读取 Host Conversation 并追加当前已校验 user message，形成 fresh `WorkingContext` seed；Agent adapter 不读取 Conversation store，Process 开始后也不回读可变 Host history；
- 成功终态从 `Process.Await`/`Result.Output` 投影 `AssistantMessageCompleted`，Reducer 用完整 final message 覆盖 partial/missing Delta；Delta 只投影临时 text/reasoning increments；
- Agent2 Termination 集中映射为 Completed/Canceled/TimedOut/Failed/MaxSteps，模型 external failure、Host cancel/deadline、Framework panic cause 和 dispatcher panic isolation 均有确定测试；
- 初版 Engine 明确不配置 `PreparedStepAcknowledger`；P4 新路径仅由独立真实 harness 消费，Bootstrap 生产 wiring 仍使用旧 Agent owner，精确旧 import/delete ledger 未增长并保留到 P8 原子切换。

## 9. P5 — 权威 model/tool observation 与 Tool 接线

### 目标

恢复普通非挂起路径的完整产品 Transcript、usage、pricing、activity、自动 approval decision 和 hooks，同时不污染 Framework。需要用户输入的 approval/HITL 属于 P6。

### 工作项

- 使用 `ModelInvocation`/`ToolInvocation` 做 Process/Effect/ToolCall 精确归因；
- model stream wrapper best-effort 投影 chunk，并同步 durable 投影完整 final/usage；
- Tool decorator 同步投影 ToolCall、result、timing、presentation、approval 和 hooks；
- 为每个 EffectRequest 建立 context-scoped dispatch-attempt tracker：pre-call write 失败不外呼，post-call authoritative write 失败由 outer Dispatcher 返回 error 形成 unknown settlement；
- 将通用 Toolset 与 Agent2 import 解耦；
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
- Toolset production 对 Agent2 零 import。

### 完成事实

- outer Dispatcher 为每个 Agent2 Effect 建立 context-scoped、按 EffectID 校验且并发安全的 dispatch-attempt tracker；model/Tool started 必须先完成 Application receipt 才跨过外部调用边界，post-call final/result/usage commit 失败统一使整个 Effect 进入 unknown，pre-call 失败保证零外呼；
- Application Run pump 仍是唯一 reducer/persistence writer。authoritative fact 使用同一 executor stream 的 commit/receipt handshake 和 speculative reducer，完整 Transcript、invocation journal、Run metrics 与 live publication 只有在一个 write-set 成功后才生效；
- SQLite epoch 61 新增 model/tool invocation operational journals。它们只保存 attempt started/completed/failed/unknown/incomplete 边界，不复制语义 final/result；完整 model final + cumulative usage/pricing + Run progress 同事务提交，Tool final Items 与 invocation terminal 同事务提交；
- 并发 Tool 的 start 不占用 Transcript 顺序；pump 暂存乱序完成结果，只在形成模型声明顺序的连续前缀时批量提交，并一起结算所有 receipt。批量失败会丢弃 speculative results，保留 started journal，随后以 incomplete Tool Items + `RunLost` 原子收口；
- Toolset 使用唯一 framework-neutral `toolset.Manifest`；现有 `toolset.Resolver` 直接满足 native Interaction 的消费端口，Runtime execution scope 同时绑定到 manifest resolution 和实际 Tool context。`search_tools` 通过精确 callback 调用 `AdvertiseTools`，Toolset 对 Agent2 零 import；
- 真实 Tool decorator 保留 safety、自动 allow/deny、argument rewrite、activity/presentation、hooks、result offload、mutation paths 与 doom-loop policy。canonical Tool result 是 settlement truth；可重新读取的 live projection 和 post-hook 失败只进入观测，不把已经确定的 Effect 改成 unknown；
- model chunk 经过有界 best-effort 队列；Runtime 本地 drop 产生 OTel event，完整 final/usage 独立于 Delta 提交。慢消费反例证明 chunk 丢失不损坏 final 或 usage；
- live unknown 使用 Dispatcher 直接 wake 与有界 public `UnknownEffectIDs` polling 双通道。Run pump 在 release 前原子提交 started/incomplete diagnostic 与 `RunLost`，终态写失败持续重试；丢 wake、写失败重试和最终 release 顺序均有行为测试；
- P5 新路径仍只由真实 Agent2 harness 消费，Bootstrap 生产 owner 保持旧 Agent 路径，等待 P8 原子切换；旧 owner 没有扩散，P5 没有引入第二执行 loop 或兼容路径。

## 10. P6 — Waiting、Checkpoint、Restore、Resume 与 Steer

### 目标

用 Agent2 公共 pending-input 和 snapshot 合同替换旧 suspension/continuation 私有解释。

### 工作项

- 通过 Interaction public helper 识别 pending tool input；
- 在 tree quiescent boundary 捕获 TreeSnapshot；
- 组装 Application checkpoint metadata + opaque payload；
- 按 exact Deployments 和 BuildID/Host expectation 恢复 tree；
- 将产品 interrupt answer 编码为 Interaction response signal；
- 将 SteerRun 编码为 steer signal，并明确下一安全 Step 的生效语义；
- 接入需要用户输入的 approval/ask-user HITL，并验证 deferred advertisement 在 wait/restore 前后保持 owner state；
- 新路径不读取旧 suspension private JSON、ProcessSnapshot codec 或 Continue/Resume 猜测路径；仍服务生产旧路径的对应文件加入 P8 原子删除清单；
- 初版明确不配置 Agent2 `PreparedStepAcknowledger`；只持久化 quiescent complete-tree checkpoint，不声明 active-step crash recovery；
- 回答 Interrupt 时同一事务 claim answer 并将旧 checkpoint 标记为 Resuming/不可自动恢复；成功后才 RestoreTree + deliver semantic Signal；
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

## 11. P7 — Delegate child Run 与 waiting subtree

### 目标

让 managed Delegate child Process 与产品 child Run 形成唯一、可恢复的因果映射。

### 工作项

- P7 的第一项是补齐并冻结两个中性 Agent2 前置合同：admitted child 的 conclusive started/aborted outcome，以及在 Apply/Discard 前冻结 source tree 的 one-shot prepared change；在此之前不得接 durable child/subtree production path；
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

## 12. P8 — Terminal、Recovery 与跨聚合一致性收口

### 目标

统一正常终态、取消、run_lost、启动恢复、online checkpoint loss 和 rollback 清理路径。

### 工作项

- 汇总 P2 taxonomy、P4 root、P6 waiting/recovery、P7 child 逐步验证结果，用 Agent2 Termination + Application control intent 完成并行为冻结完整 product outcome matrix；
- 收敛 terminal write-set、checkpoint deletion、Pending cleanup、Transcript repair、Goal reporting；
- 将 P5 live unknown 收口扩展到完整 root/child tree，并与 cancel/deadline/terminal first-wins matrix 合并；
- 重审 isolated workspace、BuildID、Session cwd/isolation 和 rollback 的恢复 policy；
- 启动 recovery 只读取 Application facts，通过 agentexec probe opaque checkpoint；
- 删除 root-only 与 tree-wide 重复 terminal transaction；
- 在全部执行能力纵切通过后一次切换 Bootstrap 到 Agent2 实现，并删除旧 root GOAP wrapper、Engine facade、TurnProcess、turn controller 和其余生产 execution path；
- 覆盖 crash point：quiescent capture 前后、waiting commit 前后、answer claim 前后、subtree commit 后 apply 前、terminal commit 前后；不包含未启用的 prepared-step ack。

### 行为验收

- 每种终态 cause 有唯一产品映射；
- terminal transaction 失败保留可重试的完整 aggregate；
- run_lost 清理不留下孤儿 Pending/checkpoint/child Run；
- rollback/delete/restore 会清理其作用域内 parked Process；
- boot recovery 与 online recovery 使用同一领域不变量。

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

## 15. P11 — 旧 Agent 删除与唯一模块名替换

### 目标

完成 Agent2 项目最后的消费者迁移和模块替换，不长期保留两个 Agent Framework。

### 工作项

- workspace 搜索并迁移所有剩余旧 `agent` consumer；
- 删除旧 Agent module；
- 将 `agent2` directory/module path 原子改回 `agent`；
- 更新 Runtime import、workspace metadata、文档、baseline 和 architecture guards；
- 不保留 `agent2` alias module 或 replace compatibility。

### 验收

- workspace 只有一个 Agent Framework module/path；
- 旧 Agent symbols、imports、docs 和 module metadata 为零；
- Agent Framework standalone 全门禁和 Runtime 全门禁同时通过。

## 16. P12 — 全量质量验收与消费者移交

### 目标

证明重构后的 Runtime 自洽、无旧债，并把准确协议变化移交给消费者专项。

### 验收矩阵

- `go mod tidy`/workspace diff；
- 全 workspace build/vet/staticcheck；
- Runtime 普通测试、race、相关 fuzz；
- Agent Framework standalone 普通测试、race、fuzz、examples；
- SQLite fresh schema、HTTP/SSE、in-process、recovery、rollback；
- architecture DAG、API/contract/wire baseline；
- 旧名称、旧 import、compat codec、空目录、TODO 和漂移注释扫描；
- 前端/TUI/CLI breaking surface handoff 文档。

完成 P12 只表示服务端和 Agent Framework 自洽。消费者接线完成后才能宣称整个产品迁移完成。

## 17. 进度记录

| 日期 | 阶段 | 完成事实 | 验证 |
|---|---|---|---|
| 2026-08-08 | P0 | 只读盘点 Runtime 当前 package、旧 Agent import、Agent2 Baseline 9、协议制品与 SQLite schema epoch；确认选择原模块内局部绿色重写，不创建 runtime2 | 生产代码未修改；事实写入 Capability Ledger 与 Contract Baseline |
| 2026-08-08 | P0 | 建立并交叉收口六份核心文档，冻结 DDD/Clean Architecture 边界、Agent2 防腐合同、P1–P12 依赖、parallel harness/P8 cutover、恢复与副作用失败语义；识别并裁决 P7 两项 Agent2 中性前置合同 | 独立 Go spec review 结论 Approved/Ready；本 goal 未修改生产代码；本地链接检查与 `git diff --check` 通过 |
| 2026-08-08 | P1 | 冻结目标六环 DAG；Delivery 开始禁止 concrete Adapter；Agent2 只允许从 `adapter/agentexec` 导入两个已批准 public package；旧 Agent import、Domain context I/O ports、`component` umbrella、旧 private snapshot decoder 和唯一旧 lifecycle owner 全部进入精确 Temporary 台账 | 错误 Delivery→Adapter fixture 被稳定拒绝；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P2 | 一次性删除 `domain/execution`：Run、Accounting、Conversation、Transcript、Interrupt 与 ToolResult 按 bounded context 提升；executor ref/checkpoint、pending continuation、workspace mutation 归还 Application；Approval、Agent Memory、Codebase、Hooks、Provider 的十个 context I/O port 全部移到真实 Application consumer，Domain 生产与测试均禁止向外依赖 | Domain context I/O port 从精确十项例外降为零；旧 path、alias、空目录为零；Run 状态/lineage/capabilities、Conversation seed/truncate、usage monotonicity 与 checkpoint expectation 行为测试通过 |
| 2026-08-08 | P2 | SQLite executor checkpoint、pending interrupt 和 workspace mutation 改为 technical records，由 `adapter/persistence` 显式映射 Application values，清除 Infra→Application 反向依赖；终态统一为 Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost，并同步服务端 protocol/schema/generated artifacts | architecture target DAG、Domain test isolation、strict storage codecs、outcome round-trip 与 compatibility differ 通过；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P3 | 删除 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖边界；建立 root stage/commit/begin、observe/release 的消费方端口；Run pump 在所有非 Waiting 边界统一释放；Application executor identity 统一为 Member；SQLite epoch 59 一次性采用 `root_member_id`/`memberId` | fake-backed ordering/race/release/waiting tests 与 architecture vocabulary/port-shape guards 通过；`go test ./...`、`go vet ./...`、`go build ./...` 及 runs/sessions/runsegment/SQLite targeted race 通过 |
| 2026-08-08 | P4 | 在原 `agentexec` 内完成 Agent2 native Interaction root harness；每 root 独立 Engine + exact Deployment；Application 组装完整 Conversation seed；Result final 与 Delta 分离；集中映射 completion/model failure/cancel/deadline/panic；生产旧 owner 保留到 P8 | real Engine/Interaction integration、stage-zero-side-effect、complete WorkingContext、final-without-Delta、termination/panic/release tests 通过；architecture exact old-import ledger 未增长；`go test ./...` 通过 |
| 2026-08-09 | P5 | 建立 per-Effect dispatch-attempt 与 authoritative commit receipt；model/tool invocation journal、Transcript final、usage/pricing 和 Run progress 原子提交；并发 Tool 乱序完成按模型顺序批量结算；接通唯一 Toolset Manifest、scope、deferred advertisement、approval/hooks/presentation/offload/doom-loop；live unknown 由 wake + polling 收口为 durable RunLost | pre/post-call failure、chunk drop、并发逆序与 batch rollback、lost wake、RunLost retry-before-release、scope propagation、best-effort projection/hook、SQLite transaction/order integration tests 通过；`go mod tidy` diff-free，`go test ./...`、`go vet ./...`、`go build ./...`、`staticcheck ./...` 与 agentexec/runs/runsegment/SQLite targeted race 全绿 |

## 18. 当前下一步

P5 已完成。下一批进入 P6：只使用 Agent2 public pending-input、TreeSnapshot、RestoreTree 与 Interaction semantic Signal 建立 waiting/checkpoint/restore/resume/steer 纵切；初版仍不配置 `PreparedStepAcknowledger`，只承诺恢复 Application 已提交的 quiescent complete-tree checkpoint，并严格实现 answer claim 后旧 checkpoint 不可恢复的线性化合同。
