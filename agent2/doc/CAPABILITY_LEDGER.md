# Agent Framework 能力裁决台账

> 状态：持续维护的实施事实
> 建立日期：2026-08-06
> 最后更新：2026-08-06

本文只追踪旧能力是否被新 Framework 认领、归谁拥有、如何裁决和在哪一阶段验收。它不定义目标架构、不复制 ADR、不记录逐提交进度。

- 目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 长期决策见 [`DECISIONS.md`](DECISIONS.md)。
- 工程标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。
- 阶段与完成事实见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

## 1. 记录规则

1. 每项旧能力必须裁决为保留思想、重新实现或移除，不能因未被某阶段提及而静默消失。
2. `真实消费者` 只记录需求证据，不决定新 API、包名或 owner。
3. `新 owner` 必须是 Framework/Strategy/基础库/Host 中准确的一层；应用领域名称不得进入 Framework 公共合同。
4. `验收` 必须是可执行行为或 architecture gate，不能只写“已迁移”。
5. 阶段开始时补充代码证据，阶段完成后才把裁决状态改为已验证。

## 2. Kernel 与执行生命周期

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| Agent definition、descriptor、deployment digest | 旧 Engine、长期恢复 | Definition/Deployment | 重新实现 | P1–P2 | 不可变、schema 进入 digest、精确恢复 |
| Process 状态机 | 所有托管执行 | Engine/Process | 保留思想并重新实现 | P1–P2 | 全状态矩阵、first-terminal-wins |
| ProcessView | policy、listener、Host adapter | 按消费者拆分的只读小接口 | 移除旧形态并按需重写 | P1 | 不保留共同胖视图；每个接口有真实消费者 |
| ProcessContext | Action、interaction、tool、控制 | Signal/Transition/Effect 与消费侧小接口 | 移除旧形态 | P1–P5 | 不出现 god context；Engine 保持唯一 owner |
| Process snapshot/tree snapshot | pause、restart、child tree | Framework capture/restore | 重新实现 | P2–P4 | last-stable/prepared 边界、pre-dispatch durability gate、严格 codec、tree restore |
| Respond/Resume/Continue | HITL、parked tool、child completion | Signal/WaitID 控制面 | 重新实现 | P1–P4 | 排序、去重、过期、Running/Waiting 投递 |
| Engine deployment catalog | deploy、restore | Engine resolver consumer + Platform catalog | 拆分重写 | P2、P8 | 单一真相源、精确 DeploymentRef |
| child Process 与递归 | delegation、workflow、supervisor composition | Engine Process tree | 保留思想并重新实现 | P4 | 恢复、取消、预算、深度、fan-out |
| usage、budget、limit | 所有托管执行 | Engine 中性资源事实 | 重新实现 | P2、P4 | 单调累计、父子划拨、终止原因准确 |
| StuckPolicy/ProcessPolicy | GOAP 无计划、运行限制 | Planning policy / Engine 中性 policy | 拆分重写 | P2、P5 | Stuck 不进入共同状态 |

## 3. Interaction、Tool 与人机协作

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| `interaction` + `toolloop` 两层 | 聊天、编码、工具自主调用 | Interaction | 合并重写 | P3 | 单一公共概念，多轮/工具/停止完整 |
| Tool checkpoint/PendingCall | Tool pause、精确 resume | Interaction ExecutionState | 保留思想并重写 | P3 | 每个合法挂起点 capture→restore→continue |
| Suspension/HITL | approval、question、child wait | WaitID + Signal；typed helper 按需 | 重新实现 | P3–P4 | schema、批量回答、重复/过期拒绝 |
| StreamCall/OnDelta | desktop/CLI 实时输出 | 有界非权威 Delta | 重新实现 | P3 | 顺序、背压、丢弃可观测、final 独立 |
| steering middleware | 运行中追加输入 | Interaction Signal | 移除 middleware 绕行并重写 | P3 | 下一安全边界生效，不中断不安全 Effect |
| ChatMiddleware | history、prompt/context 注入 | `chatclient` 或 Interaction 的具体 pre-model seam | 按能力拆分 | P3 | Framework 不拥有产品历史；顺序可测试 |
| ToolMiddleware | 权限、审批、观测、业务策略 | `tool`/Host adapter/decorator | 保留基础库能力，不进入 Kernel | P3 | Tool 合同不依赖 Agent/Host 类型 |
| conversation/history compaction | 跨执行产品上下文维护 | Host/独立 history 能力 | 从 Framework 移除 | P3 consumer spike | Agent 公共 API 无产品 history/compaction |
| WorkingContext | Tool loop、resume、模型请求 | Interaction ExecutionState | 重新实现 | P3 | 与产品历史术语分离；恢复自足有界 |
| InteractionCostProjector | 产品价格与成本投影 | Host observer/accounting | 从 Framework Kernel 移除 | P3、P10 | Framework 只发中性 usage，不认识价格表 |

## 4. Planning、确定性编排与组合

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| Goal/Condition/Truth/WorldState/Blackboard | GOAP/HTN/Utility | Planning | Goal/Condition/Truth/WorldState 保留思想并重写；Blackboard 移除 | P5 | Planning 状态不进入共同 Process/snapshot；无全局可变 Blackboard |
| GOAP planner | 多路线、动态重规划 | `planning/goap` | 保留算法思想并重写 | P5 | 搜索、成本、reobserve/replan |
| HTN planner | 层次任务规划 | 未来 Planning 扩展 | 当前重写移除，仅保留已审计算法证据 | P5 | 不预建 `planning/htn`；真实消费者出现后按同一 Planner SPI 重新申请 |
| Utility planner | utility selection | 未来 Planning 扩展或代码选择 | 当前重写移除，仅保留已审查的排序证据 | P5、P7 | 不预建 `planning/utility`；先由真实选择语义证明 owner |
| Reactive planner | 旧 planning/reactive | 无 | 移除旧形态 | P5 | 不建立 `reactive` package；单步规则选择不是 ReAct，也未证明独立生命周期 |
| routing Ranker/Candidate/Choice | Definition/worker selection | 代码选择或 Platform routing | 拆分裁决 | P6、P8 | 不建立重复 router abstraction |
| sequence/gate/switch/loop | 代码确定路径 | `flow`（in-process）或 Workflow（managed Process） | 按生命周期拆分重写 | P6 | Stage 顺序/Transform/Switch/Loop；不在 Step 内执行 `flow.Node` |
| fork/map/join | 并行 section/vote/reduce | `flow`（in-process）或 Workflow + child Process（managed） | 按生命周期拆分重写 | P4、P6 | 显式窗口、真实 child、声明顺序和 tree restore |
| Supervisor | 动态 worker、结果综合 | Interaction + child Process composition | 移除独立 Strategy 预设 | P7 | 无独立 kind/package；组合行为测试 |
| evaluator-optimizer | Anthropic pattern | `flow.Loop` 或 Workflow Loop + evaluator child | 延后重写 | P7 | 明确终止条件、质量门槛和 limit exhausted 结果 |
| Action-to-Tool adapter | 模型选择 framework Action | Interaction 与 child Process 组合 | 重新实现 | P7 | 名称/schema/结果准确，不混淆 Action/Tool |

## 5. 扩展、依赖与观察

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| `core.Dependencies` typed scope chain | 动态 Action、应用装配 | 构造/闭包优先；经真实 Strategy 证明后才建局部 typed provider | 移除共同 scope，局部需求重新实现 | P1、对应 Strategy | 无全局 DI、无 service locator、无共同 god scope |
| Extension marker/capability dispatch | policy、middleware、listener、planner | 单一横切扩展机制；主 Strategy 移出 | 拆分重写 | P2、P8 | 忽略扩展不破坏 Kernel 正确性 |
| Event multicast | runtime observer、Host projection | Framework Event | 重新实现 | P2 | attempt/committed 区分、顺序、listener 隔离 |
| model/tool lifecycle event | UI、usage、observability | Strategy dispatcher → Framework Event | 重新实现 | P2–P3 | 时间语义、Process/Step/Effect identity 完整 |
| OTel instrumentation | tracing/metrics/logs | 外部 `otel` decorator | 保留边界 | P8 | Kernel 不 import OTel，不重造 tracer SPI |
| Action/Agent validator | deploy/admission | Definition/Deployment 构造；横切 guardrail 按需 | 拆分重写 | P1–P2 | 非法定义不能进入 Engine |
| typed dependency/environment access | Action/Condition 执行 | Strategy-owned dispatcher/constructor | 重新实现或移除 | P3–P5 | Kernel 不认识产品依赖 |

## 6. Host 专属能力

以下能力是迁移验收证据，不是 Agent Framework 待抽象的领域模型：

| 能力 | 新 owner | Framework 只提供 | 验收阶段 |
|---|---|---|---|
| 产品身份、历史、记录和展示投影 | Host | Process/Event/Output 中性事实 | P3 spike、P10 |
| Store、transaction、CAS、lease、retention | Host | Snapshot capture/restore value | P2、P10 |
| 销毁、回滚、替换、导入导致的执行清理 | Host | Kill/Capture 和稳定 Process identity；持久记录删除仍归 Host | P3 spike、P10 |
| 业务权限、订阅、计费和价格表 | Host/adapters | capability/usage 中性合同 | P4、P8、P10 |
| 外部副作用事务、补偿和业务幂等 | Action/Tool adapter 或 Host | EffectID 与 settlement 事实 | 对应 Strategy、P10 |

这些名称不得成为 Kernel 类型、字段、package 或 snapshot 字段。Host 的真实消费审计可以补充本表证据，但不能以迁移方便反向决定 Framework API。

## 7. P1 只读审计证据

### 7.1 旧 Framework 证据

- 旧共同 `ProcessView` 同时暴露 Planning 的 Goal、Blackboard、WorldState 和 Interaction 的挂起信息；旧 `ProcessContext` 又合并依赖访问、树控制、状态推进和策略执行。这证明两者不能作为新窄腰保留，共同层必须改成明确方向的 Signal/Transition/Effect 和按消费点形成的小接口。
- 旧共同 Process snapshot 直接持有 Interaction 的挂起类型，证明“看似中性的挂起字段”仍会让策略抽象反向进入 Kernel；新共同 snapshot 只能记录 WaitID、Signal envelope、游标和 Execution 自有 opaque state。
- 旧 Interaction/tool loop 已证明有价值的行为包括稳定工具顺序、有界并行、checkpoint、pause/resume 和精确继续；这些行为保留，但旧 package 划分、类型所有权和同步 listener 合同不复制。
- 旧 GOAP 搜索、成本、条件与动态重规划是 P5 的算法证据，不是共同 Process 模型；Planning 术语只能留在 Planning。
- 旧事件路径把具体观测实现带入 listener 且同步传播失败；新 Event 必须保持 Framework 中性，观察失败不能改变 Step 结果，具体 OTel 只能在外部 decorator。

### 7.2 真实 Host 消费证据

- 只读扫描确认 Host 有 54 个 Go 源文件直接引用旧 Framework。真实共性需求是异步 start/await、根或子树取消、HITL 恢复、等待查询、opaque capture/restore 和执行清理；这些能力必须由中性 Engine 生命周期覆盖。
- 当前根聊天执行把一个完整 Interaction 包装成单 Action 的 GOAP 计划。这个形态只证明 Host 需要原生 Interaction Definition，不证明普通聊天应依赖 Planning，也不能成为新架构的默认组合。
- Host 必须继续拥有产品身份、历史与上下文、工作空间与隔离、模型选择、价格投影、业务限制、持久 write-set 和展示投影；Framework 只提供 Process identity、opaque snapshot、usage、Event/Output 和 prepared durability boundary。
- 当前 steering 绕经聊天 middleware，迁移目标是由 Interaction 在声明的安全边界解释 opaque Signal；Engine 只排序、去重、投递并推进游标。
- 当前 Host 将 executor checkpoint 当 opaque value 使用，这支持 Framework capture/restore 边界；Host 不应解析 Strategy state。子执行准入和持久提交仍是 Host 策略，不能升级成 Framework 的 Store、transaction、lease 或幂等模型。

以上证据只约束行为和边界，不授权将任何 Host DTO、存储协议、产品生命周期或交付模型复制进 `agent2`。

## 8. P3 Interaction 专项审计证据

### 8.1 保留的领域规则

- 旧 `toolloop` 已证明模型产生的 ToolCall 次序是后续 WorkingContext 的权威顺序；即使安全工具并行结算，ToolResult 也必须按原 ToolCall 顺序稳定归并。
- 并行不能由 Runner 根据工具名或参数猜测。未声明的工具默认独占；只有工具显式声明可并行且资源键不冲突时，才能在配置上限内并行。
- 暂停点必须携带已结算结果、待执行后缀、当前等待请求和完整 WorkingContext，从而在恢复后不重调模型、不重执行已结算工具，并继续产生确定的下一轮请求。
- `chat.ResponseAccumulator` 是流式响应得到最终语义响应的权威机制；Delta 只是 best-effort 观察，不是恢复时的真相源。
- `chatclient` 已经拥有请求快照、默认选项和 call/stream middleware，`tool` 已经拥有工具定义、严格入参解码和 decorator capability 查找；Interaction 必须复用这两个基础包，不复制 Model、Message、Tool 或 Schema 协议。

### 8.2 治本式移除的旧设计

- 不再同时公开 `interaction` 数据协议和 `toolloop.Runner` 两层概念；工具循环是 Interaction Execution 的内部推进机制，不是第二个生命周期入口。
- 不复制旧 `Suspension.ID`/`PauseError.ID` 由 Tool 或 Strategy 铸造等待身份的做法；新 Interaction 只声明私有 WaitKey，WaitID 必须由 Engine 根据 prepared Effect 稳定铸造。
- 不保留 `Continue`/`ContinuePaused`/`Resume` 多个 Runner 推进入口；所有正常推进只经 `Execution.Step`，外部回答和 steer 都是 Engine mailbox 中的 opaque Signal。
- 不保留通过聊天 middleware 侧路注入 steer 的做法；Interaction 只在已开始的 model Effect 或 Tool batch 全部结算后、下一 model Effect 前的安全 Step 消费 steer。
- 不保留同步 observer 失败或 panic 可以杀死运行的合同；Event/Delta listener 继续由 Engine 隔离，恢复不补播历史 Delta。
- 不把产品 conversation/history、价格投影、run/segment、存储 checkpoint 或业务审批 DTO 带入 Interaction；其 ExecutionState 只持有精确恢复所需的 WorkingContext 和策略私有检查点。

### 8.3 P3 实现边界

- Model call 和 Tool batch 都是 dispatcher-owned Effect；Execution 只解释对应 settlement Signal 并产生下一个 Transition。
- 工具要求外部输入时，Tool batch settlement 保存确定的已结算前缀和 Strategy-owned 等待 payload；随后的 Framework wait Effect 只负责返回 Engine-minted WaitID。
- 对一个已经返回 definite settlement 的 Effect 绝不重投；恢复时处于 unknown 的不可证明副作用继续由 Process 显式裁决，Interaction 不擅自重试。
- 工具的可恢复人机交互必须在产生副作用前请求输入，或自行使用其私有 continuation state 证明重新进入不会重复副作用；Framework 不虚构外部事务或补偿保证。

## 9. P5 Planning 专项审计证据

### 9.1 保留的领域规则与算法性质

- `Truth` 必须区分 Unknown、False 和 True；WorldState 是一次不可变观察，模拟效果只能产生新状态，不能修改原观察。用于搜索去重的 state key 必须由规范排序后的 condition truth 唯一导出，不能包含 clock、pointer 或 map 迭代顺序。
- Goal 是 Planning 独占的目标条件集合；Condition 是可验证的命名事实，不是携带 Process、Blackboard、模型或 Host dependency 的 I/O evaluator。外部世界如何产生完整 WorldState 属于 Planning dispatcher 的 observation 边界。
- GOAP 保留非负有限边权上的 uniform-cost search。动态 Action cost 在每条候选边的源 WorldState 上求值；相同累计成本按 Action 声明顺序稳定展开；仅在严格更低成本时替换已知路径；无状态变化的 Action 必须跳过；搜索必须响应 context cancellation 并受显式 expansion limit 约束。
- 旧 GOAP 的 direct-producer 检查只是一项安全的保守短路，不是完整可达性证明；真正的 reachable/unreachable 结论仍由搜索得到。成本函数 panic、NaN、Infinity 或负值都必须被归因并拒绝，不能静默改写排序。
- Planning Execution 必须在每次 Action 结算后重新观察真实环境，并从新 WorldState 重新规划；预测 Effects 只服务搜索，不能冒充 Action 已实际成功。环境变化、Action 失败与无进展都通过排除已失败候选并 reobserve/replan 收口。
- Action 的规划描述与一次计划中的 `PlannedAction` 必须分层：前者定义身份、前置条件、预测效果和成本；后者只是 Planner 输出的稳定引用。执行能力由显式 dispatcher binding 或 child Process spec 提供，不进入 Planner 的纯搜索模型。

### 9.2 治本式移除的旧设计

- 不复制 `Action.Execute(ctx, *ProcessContext)`。旧 Action 同时读取依赖、Blackboard、WorkingState、Tool roles、binding 和 child 控制，是 Planning 与共同 Kernel/Host 装配的聚合泄漏；新 Step 只声明 dispatcher Effect 或 Framework child Effect。
- 不复制可执行 `Condition`、`ConditionEnv` 或 `ConditionResolver`。观察 I/O 不在 Planner 内按 evaluation cost 隐式发生；Observer 一次产生自足的 WorldState，Planner 对同一 Problem 保持纯且确定。
- 不复制全局 Blackboard、typed binding、动态 dependency scope、Agent identity、Domain-for-Agent 或 planner Extension registry。Planning Definition 构造时显式冻结 Goal、Actions、Planner 与执行 binding，不按名称发现或选择默认 Planner。
- 不复制 Planning core 内的 OTel tracer、span attribute 或 logger。Planner 只返回 plan/no-plan/error；框架中性 Event 与后续外部 decorator 负责观察。
- 不把旧 `StuckPolicy` 或无计划结果提升成共同 Process Status。Goal 初始不可达和尝试后无进展是 Planning-owned terminal Output；合同违约、Planner error 和 dispatcher protocol error 才是 Process failure。
- 不保留同时携带 Action、Goal、Condition registry 和运行时 provider 的可变 Domain service。构造边界冻结最小 Planning Definition，搜索输入是按值、不可变的 Problem。

### 9.3 暂不重建的 planner 变体

- 旧 HTN 已证明的顺序分解、method fallback、状态穿行、循环/深度保护是未来证据；当前仓库没有真实 consumer 证明需要公开 `planning/htn`。预建可变 task registry 或以 Goal name 隐式选择根 task 都会增加第二套领域语言，因此 P5 不实现。
- 旧 Utility 与 GoalFirst 只是单步适用 Action 的 value-cost 排序，并依赖动态 Goal value 与 planner name registry。选择能力可能属于未来 Planning policy，也可能只是普通代码选择；owner 未被真实消费证明前不实现 `planning/utility`。
- 旧 Reactive 是按固定优先级返回第一个适用 Action 的单步 planner，不是 ReAct，也没有独立推进/恢复语义。该 package 直接移除；未来若出现规则选择需求，必须以真实消费点决定落在 Planner、`flow` 组合、Platform routing 或 Interaction。

以上裁决保留能力证据而不保留旧 API。未来新增 HTN、Utility 或其他 Planner 只能消费 P5 由 GOAP 证明的最小 Planner SPI；不得要求共同 Process 增加 Goal、Plan、Blackboard、Agent identity、registry 或策略专属 snapshot 字段。

### 9.4 P5 验证结论

- Goal/Condition/Truth/WorldState、predictive Action/PlannedAction、Problem/Plan 与最小 Planner SPI 已在 `planning` 独立实现；GOAP 已在 `planning/goap` 以确定、有界的 uniform-cost search 实现。
- managed Planning 已证明 dispatcher Action 与真实 child Process Action 两条执行路径；两者均遵守每个 Action 后 reobserve/replan，预测 Effects 不作为现实真相。
- 多路线、动态环境未确认、definite failure、初始不可达、尝试后 stuck、attempt 上限、非法 Planner 输出、观察失败、能力拒绝、unknown Effect 显式裁决和 snapshot restore 均有行为合同。
- 旧 `Action.Execute(ProcessContext)`、Blackboard、Condition evaluator/provider、Domain registry、Planner registry/default、Agent identity、Planning OTel 与 disposable Planning spike 均未进入正式实现；HTN、Utility、Reactive 保持未实现且没有占位 package。
- Planning 专属 completion outcome 只存在于 Planning Output；共同 Status 和 snapshot wire 没有新增 Planning 字段。P5 台账项已验证完成。

## 10. P6 确定性编排专项审计证据

10.1–10.3 记录旧 `agent` 提供过的组合语义和第一版候选约束；10.4 记录 `flow` 的独立边界。ADR-A2-038 已依据 10.5 的真实 spike 恢复 managed Workflow，但采用更小的有序 Stage 代数，不恢复任意 DAG 和八类节点设计。

### 10.1 保留的组合语义

- Sequence 保留声明顺序、前一节点 Output 显式成为后一节点 Input、任一节点失败可归因，以及每个 Agent 节点使用真实 child Process 的语义；不保留“从 Blackboard 最新对象猜下一个输入”的隐式传值。
- Fork/Join 与 Map/Reduce 保留独立 branch identity、声明顺序聚合、显式并发上限、失败不受完成顺序影响、取消后等待已启动 branch 收口，以及输出只经 Join/Reduce 越过分支边界。动态 Map 还必须有独立 item 上限，不能把 Engine tree limit 当作正常调度器。
- Gate 与 Switch 保留确定、无 I/O 的本地选择；Workflow 内只使用 `Switch` 表示基于当前值选择下一节点，`Router` 留给 P8 的 Deployment 选择，避免同一概念两套术语。
- Vote 保留按稳定 key 计数、最高票获胜和同票按声明顺序裁决；公共合同只使用 `Vote`，不再同时暴露 Consensus 别名。
- Loop 保留至少执行一次、显式最大迭代数、每轮 body 使用新 child Process、最新 Output 成为下一轮 Input、终止谓词与完整 snapshot；公共合同只使用 `Loop`，不再同时公开 Repeat/RepeatUntil 近义入口。
- Prompt Chaining 是 Sequence 中连续 child Agent 调用的组合用法，不建立 PromptChain Strategy、Process 状态或第二套执行器。Agent call 是 Workflow 的 `Call` 节点，不建立 SubAgent 类型。

### 10.2 治本式移除的旧实现形态

- 不再把 Workflow 编译成普通 GOAP Agent。Sequence、Fork、Map、Switch、Vote 和 Loop 拥有分支游标、child wait、聚合与迭代恢复语义，直接由原生 Workflow ExecutionState 表达；Planning 不再承担确定性工作流控制。
- 不复制 builder 接收 `*runtime.Engine`、构造期调用 Deploy、Action 内调用 `RunChild` 的反向依赖。Workflow Definition 只冻结 exact child DeploymentRef、schema、预算和能力；Engine 仍是唯一 Process 创建与调度 owner。
- 移除 ScatterGather/Generator 的裸 goroutine 分支，以及 Parallel 对它的第二层包装。需要独立生命周期的 branch 一律是真实 child Process；无生命周期的纯转换不冒充 Fork/Map。
- 移除 Sequence 的 `LatestObjectBindingName`、Loop/Repeat 的 Blackboard History/Binding/computed Condition、Team 的 Definition 合并和 snapshot binding 拼接。数据流必须是严格 schema 的 immutable wire value，跨 Strategy 状态只存在于 child Process。
- 移除 Parallel/ScatterGather、Loop/RepeatUntil、Vote/Consensus 等重复公共入口；每组只保留一个准确术语。旧默认迭代次数不复制，Loop 的终止上限必须显式。
- `RepeatUntilAcceptable`/Feedback/AttemptHistory 不在 P6 建立独立节点；其真实语义属于 P7 evaluator-optimizer，由 Loop、child evaluator Definition 与 typed artifact 组合。
- `Supervisor` 不建立独立 Workflow kind/package；继续遵守 ADR-A2-020，在 P7 由 Interaction、Workflow、Action-to-Tool 和 child Process 组合。
- `Team` 将多个 Agent 的 Action/Goal/Condition 合并为一个共享生命周期，会重新引入名称冲突、状态串扰和 GOAP 中心化，整体移除；组合只通过显式节点和 child Process。
- 旧 `routing.Router` 对活动 Deployment catalog 的 Candidate/Ranker/Confidence 选择属于 P8 Platform routing，不进入 Workflow。P6 Switch 只在一个已冻结 Definition 的显式分支表内选择，不扫描 catalog。

### 10.3 P6 实现约束

- Workflow 节点合同使用 erased、schema-validated immutable wire value；泛型只在节点构造边缘负责 Go 类型 codec，不让泛型进入 Engine 窄腰。
- 每个独立 branch/iteration 是真实 child Process，拥有 exact DeploymentRef、ProcessID、snapshot、预算和 attenuated capabilities。fan-out 必须按显式并发窗口分批启动，结果按声明/item 顺序聚合。
- Join、Reduce、Gate、Switch、Vote key 和 Loop predicate 是 Step 内的确定性纯函数，不得执行 I/O；需要模型、Tool 或其他副作用时必须建成 child Definition。
- Workflow state 只保存节点身份、当前 immutable value、branch/item 游标、child identity、wait identity、迭代数和已结算结果；不保存 Engine、Deployment、callback、context、goroutine、Host identity 或 persistence 协议。
- branch failure 不由 Kernel 自动改写 parent。Workflow 节点按自己的明确策略决定 fail-fast、聚合或容错；任何策略都必须确定、可恢复且不静默遗弃已启动 child。

### 10.4 独立 `flow` 仓库复用审计

审计基线是 `/Users/tangerg/Desktop/flow` 的 clean `main`，提交 `6280bfc4cb8d15876422ac8b136ede10a755cd54` 与 `origin/main` 一致。模块 `github.com/Tangerg/flow` 使用 Go 1.26；build、vet、staticcheck、全量 test、race 全绿，`flow`、`flowx`、`workflow`、`workflow/expr` 和 `workflow/diagram` 的 statement coverage 均为 100%。本轮没有修改该仓库。

已确认的能力与边界：

- 根 `flow` 以 `Node[I, O].Run(context.Context, I) (O, error)` 为唯一协议，提供 Then、Switch、Loop、Map、Race；`flowx` 只提供可由根原语派生的 FanOut、Combine、Chain、Fallback。定义与单次运行状态分离，泛型边缘清晰，没有 Agent、Host、Store provider、transaction、lease 或产品身份泄漏。
- `workflow` 提供 copy-on-write Store、named ports、flat Graph、nested Spec、Registry、config JSON Schema、条件 gate、dependency-driven scheduling、Subgraph、同步 Event/Chunk、Journal 和 checkpoint-and-restart。并发输出按声明顺序合并，Map/Race 等启动的 goroutine 有 owner 且等待收口；仓库准确声明不提供 distributed scheduler、durable timer、exactly-once 或定义迁移。
- `flow` 的恢复真相是 Store + Journal + application-owned active suspensions。Journal 在 Leaf 完成后写入结果，不能覆盖“外部副作用已成功但结果尚未记录”的崩溃窗口；这与 Agent prepared Effect/EffectID/unknown settlement 是不同强度和不同 owner 的合同，不得混称。

复用形态裁决：

| 形态 | 裁决 | 原因 |
|---|---|---|
| Host 直接使用 `flow` 组合普通 Go/AI 能力 | 保留 | 完全符合 `flow` 的 in-process 定位，不需要 Agent Process、snapshot 或 child tree 时复杂度最低 |
| 可选 adapter 将一个完整 `flow.Node` 作为一个 dispatcher-owned Effect 执行 | 可做 disposable spike | I/O 位于 Step 之外，能够继承外层 EffectID、取消、Delta 和 definite/unknown settlement；但内部节点没有独立 Process identity、预算、能力或 tree snapshot，replay contract 默认必须是 never |
| 在 `Execution.Step` 内直接调用 `flow.Node.Run` 或编译后的 `workflow.Step.Run` | 移除 | Node 可执行任意 I/O，Graph 另起 goroutine scheduler，Journal 另写恢复事实；会同时破坏纯 Step、Engine 单写者和 Effect 事务边界 |
| 直接把 `workflow.Graph`/`Spec` 当成 managed Agent Workflow Definition | 暂不成立 | 现有端口类型只是可选的粗粒度 edit-time `ValueType`，Store 持有 `any` 借用值；NodeFactory 产出可执行 Step，branch/condition 允许 I/O；并发零值可无界，分支共享一个 run/Store/Journal，不是独立 child Process |
| 为 Agent 编写自定义 compiler/interpreter 消费 `flow` Graph/Spec | 延后裁决 | 技术上可行，但会绕过大部分现有 `workflow` runtime 并重新实现 validation/scheduling/recovery，已经不是“直接封装”；只有真实 managed child Process 消费点才能证明值得做 |

由此，`flow` 直接承担应用侧普通控制流，不能直接替代 Engine-managed child Process 编排。用户随后明确恢复 Workflow 设计，并要求只吸收 `flow` 的部分思想、不强求复用；因此不再实施 coarse-grained adapter，也不修改 `flow` 或把它加入 `agent2` 依赖。

### 10.5 managed Workflow disposable consumer 证据

- spike 只使用 Baseline 2 的公开 API，自行实现最小有序状态机：先串行启动并等待一个 prepare child，再以 concurrency window 2 编排三个 fork child。它没有访问 Engine 私有字段、解析 Framework 私有 Effect/Signal wire，或把 Engine/Deployment concrete value 写入 ExecutionState。
- 前两路 child 在副作用前进入 Paused，root 进入带 Engine-minted WaitID 的 Waiting。完整 TreeSnapshot 此时包含 prepare child 与前两路 fork child，明确不含第三路，证明 concurrency 不是依赖 Engine 拒绝超额的软约定。
- 原树被显式销毁后，在新 Engine 通过 exact resolver 恢复；第二路先于第一路 Resume/Completed，Workflow 仍按声明索引保存结果。第一窗口全部终止后才创建第三路，最终 tree 恰好包含四个 child，没有丢失或重复创建。
- 定向行为测试连续 20 次、race 连续 20 次通过；spike 随后整体删除，没有临时类型、测试 package 或依赖进入生产模块。
- 证据证明 Workflow 的独立状态是 Stage/value/window/child/wait/result 游标，并且能完全建立在现有 StartChild/WaitForChildren/TreeSnapshot 窄腰上；它不需要 Strategy dispatcher、Kernel 字段、`flow` Store/Journal 或第二 scheduler。

据此唯一实施边界确定为：普通同进程组合继续直接使用 `flow`；managed Workflow 是只编排真实 child Process 的原生 Definition/Execution。正式词汇收敛为 Transform、Call、Switch、Fork、Map、Loop；Sequence、Gate、Vote、evaluator-optimizer 等可组合语义不再各建节点 kind。
