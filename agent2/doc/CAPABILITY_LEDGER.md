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
| StuckPolicy/ProcessPolicy | GOAP 无计划、运行限制 | Planning policy / Engine Limits、ProcessAdmitter 与显式 control | 拆分重写 | P2、P5、P8 | Stuck 不进入共同状态；无每 Step 通用 StopPolicy |

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
| managed Delegate | 模型选择 exact framework worker | Interaction 与 child Process 组合 | 已重新实现；取代不可成立的通用 Action-to-Tool | P7 | 模型名称/描述/schema 准确；Execution 经 Framework Effects 启动 child |

## 5. 扩展、依赖与观察

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| `core.Dependencies` typed scope chain | 动态 Action、应用装配 | 构造/闭包优先；经真实 Strategy 证明后才建局部 typed provider | 移除共同 scope，局部需求重新实现 | P1、对应 Strategy | 无全局 DI、无 service locator、无共同 god scope |
| Extension marker/capability dispatch | policy、middleware、listener、planner | 按消费位置命名的小接口；无 marker/registry | 拆分重写 | P2、P8 | admission 与 observation 不共用动态分派；主 Strategy 移出 |
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
- `Supervisor` 不建立独立 Workflow kind/package；继续遵守 ADR-A2-020，在 P7 由 Interaction、Workflow、managed Delegate 和 child Process 组合。
- `Team` 将多个 Agent 的 Action/Goal/Condition 合并为一个共享生命周期，会重新引入名称冲突、状态串扰和 GOAP 中心化，整体移除；组合只通过显式节点和 child Process。
- 旧 `routing.Router` 对活动 Deployment catalog 的 Candidate/Ranker/Confidence 选择属于 P8 Platform routing，不进入 Workflow。P6 Switch 只在一个已冻结 Definition 的显式分支表内选择，不扫描 catalog。

### 10.3 P6 实现约束

- Workflow 节点合同使用 erased、schema-validated immutable wire value；泛型只在节点构造边缘负责 Go 类型 codec，不让泛型进入 Engine 窄腰。
- 每个独立 branch/iteration 是真实 child Process，拥有 exact DeploymentRef、ProcessID、snapshot、预算和 attenuated capabilities。fan-out 必须按显式并发窗口分批启动，结果按声明/item 顺序聚合。
- Join、Reduce、Gate、Switch、Vote key 和 Loop predicate 是 Step 内的确定性纯函数，不得执行 I/O；需要模型、Tool 或其他副作用时必须建成 child Definition。
- Workflow state 只保存节点身份、当前 immutable value、branch/item 游标、child identity、wait identity、迭代数和已结算结果；不保存 Engine、Deployment、callback、context、goroutine、Host identity 或 persistence 协议。
- branch failure 不由 Kernel 自动改写 parent。Workflow 首版等待当前已启动窗口全部收口，再按最低声明索引稳定失败；不提供 fail-fast、partial、fallback 或容错 enum，也不静默遗弃已启动 child。新的失败策略只有经真实消费者证明后才能新增。

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

- spike 只使用 Baseline 2 的公开 API，自行实现最小有序状态机：先串行启动并等待一个 prepare child，再以固定执行窗口大小 2 编排三个 fork child。它没有访问 Engine 私有字段、解析 Framework 私有 Effect/Signal wire，或把 Engine/Deployment concrete value 写入 ExecutionState。
- 前两路 child 在副作用前进入 Paused，root 进入带 Engine-minted WaitID 的 Waiting。完整 TreeSnapshot 此时包含 prepare child 与前两路 fork child，明确不含第三路，证明窗口调度不是依赖 Engine 拒绝超额的软约定。
- 原树被显式销毁后，在新 Engine 通过 exact resolver 恢复；第二路先于第一路 Resume/Completed，Workflow 仍按声明索引保存结果。第一窗口全部终止后才创建第三路，最终 tree 恰好包含四个 child，没有丢失或重复创建。
- 定向行为测试连续 20 次、race 连续 20 次通过；spike 随后整体删除，没有临时类型、测试 package 或依赖进入生产模块。
- 证据证明 Workflow 的独立状态是 Stage/value/window/child/wait/result 游标，并且能完全建立在现有 StartChild/WaitForChildren/TreeSnapshot 窄腰上；它不需要 Strategy dispatcher、Kernel 字段、`flow` Store/Journal 或第二 scheduler。

据此唯一实施边界确定为：普通同进程组合继续直接使用 `flow`；managed Workflow 是只编排真实 child Process 的原生 Definition/Execution。正式词汇收敛为 Transform、Call、Switch、Fork、Map、Loop；Sequence、Gate、Vote、evaluator-optimizer 等可组合语义不再各建节点 kind。

### 10.6 Transform/Call 正式纵切面

- `workflow` 已作为独立 Strategy package 实现，不改 Kernel。sealed Stage 只有包内 concrete behavior；首轮公开构造器为 typed pure Transform 和 `Call(CallConfig)`，没有 Node interface、builder、Registry 或通用 callback SPI。
- Definition 冻结有序 Stage slice，要求唯一稳定 ID，并以规范化 schema 的精确相等验证每一条相邻数据边。Descriptor 只暴露首个输入和末个输出合同；调用方的 Deployment configuration digest 必须覆盖完整 Stage 配置、纯函数与 child binding。
- Call 构造时消费 concrete Deployment 仅用于验证并提取 DeploymentRef、input/output schema；Stage 不保存 Deployment、Dispatcher、Engine 或 resolver。每次调用必须显式给出正数 Budget，CapabilitySet 零值准确表示无权限。
- ExecutionState v1 只保存 phase、Stage index、当前 raw value、ChildProcessID 与 WaitID。Call 通过 Framework Effects 推进 child start、wait registration 和 completion；zero-state Workflow Dispatcher 对任何 dispatcher Effect 都返回协议违约。
- 真实 Engine consumer 已跑通 Transform→Call→Transform，并通过每个 Step 的 capture/restore 候选重建。child start/终态/Output 违约由 Workflow 自己给出带 Stage 语义的稳定失败码，不把 child failure 无上下文地冒充 parent failure。

### 10.7 Switch/Fork 正式纵切面

- Switch selector 是 typed edge 上的纯函数，只返回一个已声明 case ID；每个 case 是 exact child Deployment。构造期同时验证所有 case 的输入与 Switch 输入精确相等、输出彼此精确相等，未选择 case 不会创建 Process。
- Fork 首版明确是 homogeneous fan-out + reducer：所有 branch 接受同一 I 并产生同一 B，reducer 按声明顺序把 `[]B` 变成 O。需要异构内部步骤的 branch 必须先用 child Workflow 暴露共同 B，不能把 `any`、共享 Store 或隐式 binding 带回父级。
- Fork `WindowSize` 是显式正数固定窗口大小且不得超过 branch 数；Execution 只启动当前窗口，等待该窗口全部已启动 child 终止后才启动下一窗口。它不是完成一个就补位一个的滑动并发池。start failure 不会遗弃同窗已经启动的 child，所有失败按最低声明索引稳定归因。
- indexed fan-out state 用 member index、ProcessID、Failure 与 nullable raw Output 槽位区分未启动、已启动、已失败和已结算；字面 JSON `null` 不能冒充一个未结算槽位。窗口与单 child 的 ChildKey/WaitKey 都由完整逻辑身份稳定哈希，避免合法组合超出 Kernel identity 长度。
- 真实 dispatcher-backed child 测试证明 window=2 不会提前启动第三路、逆序完成不改变 reducer 顺序、同窗多失败不受完成时序影响。Switch/Fork 没有增加 Kernel API、dispatcher 协议、goroutine scheduler 或 `flow` 依赖。

### 10.8 Map 正式纵切面

- Map 的唯一公共语义是 `[]I → []O`：一个 exact Deployment 对每个 item 执行 I→O，输出严格按原 item index 排列。它不暴露动态 Graph、Generator、共享 Store、keyed binding 或隐式 flatten。
- `WindowSize` 与 `ItemLimit` 都是显式正数，且 window size 不得超过 item limit。item limit 在声明任何 StartChild Effect 前检查；空输入准确完成为空切片，不创建一个伪 child 或返回 JSON null。
- Fork 与 Map 共用包内 sealed indexed fan-out 生命周期和同一 ExecutionState 字段；差异只存在于 Stage-owned binding/input/collect 行为。该收敛没有形成公开 fan-out interface，新增 Stage 不能绕过封闭代数。
- 每个窗口只解码一次原始 `[]I`，只为当前窗口编码 item Input；snapshot 继续保存权威原始 value、窗口游标和已结算 Output，不保存 Go slice、Deployment concrete value 或第二份 Journal。
- 真实 Engine 测试覆盖多窗口稳定顺序、空输入、超限先拒绝和构造期 schema/limit 违约；Fork 原有逆序完成与最低索引失败合同在共用状态机后保持不变。

### 10.9 Loop 正式纵切面

- Loop 的唯一公共语义是 T 经至少一个 exact T→T Body child 迭代后，输出 `LoopResult[T]`。Predicate 只在 child 成功且 Output 通过 T schema 后执行，不在首轮前短路，也不执行 I/O。
- MaxIterations 必须显式为正；达到上限仍未满足时准确输出最新 Value、实际 Iterations 与 `Satisfied=false`，由后续 Stage/consumer 决定 fallback 或失败。共同 Process 不增加 exhausted/stuck 状态。
- 每轮 Budget 都是独立永久 child 划拨，每轮 ChildKey/WaitKey 纳入一基 iteration。LoopIteration 是 Strategy-owned state；Body Deployment concrete value、predicate、context、goroutine 和调用栈不进入 snapshot。
- Loop 复用单 child 的 start/wait/completion 状态机，只在上轮完整结算后声明下一轮 StartChild Effect。Body failure、非完成终态和 Output 违约均保留 Loop Stage/iteration 归因。
- 满足、耗尽和“初值已满足仍执行一次”三个真实 Engine 测试的 TreeSnapshot 都严格包含 root + Iterations 个 Process；这证明 Loop 没有把 body 降级成同 Process 内调用。

### 10.10 恢复、协议与资源边界验收

- Workflow ExecutionState 仍使用单一 versioned strict codec；未知字段、错误 kind/version、完成游标伪装、Stage 不允许的 child/fan-out/loop 进度都确定拒绝。所有缺失或错配的 Framework settlement Signal 统一归入 `ErrInvalidProtocol`，zero-state Dispatcher 对任意 dispatcher Effect 也只返回该协议错误。
- 真实 Fork tree 在 root Waiting、两个第一窗口 child Paused 时完整 capture；原 Engine 销毁后，新 Engine 通过 exact Deployment resolver 恢复相同 Process identity。第一窗口逆序恢复后，第二窗口 child 恰好创建一次，结果仍按声明顺序，最终 tree 恰好包含 root 与三个 child。
- Host cancel 在 managed Call 等待 Paused child 时，root 终态来源是 host cancellation，child 终态来源是 parent cancellation。Workflow 请求超过 parent 剩余 Budget 或父级不持有的 Capability 时，由共同 Engine 的 child allocation invariant 拒绝，不存在 Strategy 绕过路径。
- tree 合同暴露并修复了共同 Kernel 的一个既存缺口：Process 在未消费 child completion 前终止时，mailbox 中的 child wait 曾保持 open，而 Engine registration 已删除，导致 terminal TreeSnapshot 自相矛盾。终止提交现在统一关闭全部 wait，并以稳定顺序注销内部 child wait；未新增 Workflow snapshot 字段、Store、Journal 或第二恢复真相源。
- Stage 代数由 AST 架构守卫持续封闭：生产 `workflow` 不允许 exported behavior interface，`Stage` 必须是无公开可变字段的 value；Host/legacy/`flow`/Store/Journal/Graph/Scheduler 禁入规则继续生效。全量 standalone race、50 次 Workflow race、50 次 Kernel 终态 wait 回归和 260682 次 state fuzz 均通过。

### 10.11 独立消费者与 Baseline 3

- `examples/workflow` 是与生产 package 分离的 command consumer，只消费公开根 API 与 `workflow` API。它装配 Call→Fork 的 root Definition，三个被调行为 exact child Workflow Deployment；最终 TreeSnapshot 的四个 Process 证明 Call 与 branch 没有被降级为本地函数或隐藏 goroutine。
- API 人体工程学审计将 Fork/Map 参数从容易暗示滑动补位的 `Concurrency` 治本改名为 `WindowSize`；GoDoc 明确整窗 start/settle 后才进入下一窗。代码、测试、架构、ADR、台账和示例只保留这一术语，没有 alias 或兼容字段。
- 无真实消费者的 `StageKind`、Stage metadata getters 与 `Definition.Stages` 已收回包内。它们只能形成不完整反射面，无法重建 pure function、child binding 或 configuration digest，也没有资格预占未来 P8 catalog/编辑器合同。Stage 仍只暴露 sealed value、typed constructors 与零值有效性判断。
- Baseline 3 新增 workflow 完整 exported API/GoDoc digest `0493f8f7ae6e4cc5a3190735c5d02952ec0e0fdb230794bbb01735b8ecfae055`；root、Interaction、Planning、GOAP 与 snapshot/tree wire digest 沿用 Baseline 2 并继续通过。P6 没有为 Workflow 扩张 Kernel 公开合同或 wire。

## 11. P7 组合模式专项审计证据

### 11.1 Embabel supervisor/utility/hybrid 与旧模块裁决

审计基线是 clean `/Users/tangerg/Desktop/embabel-agent` main `e2d7b987c`，以及当前仓库旧 `agent` 的 supervisor、utility、AgentTool 与 RepeatUntilAcceptable 实现。上游 targeted tests 的源代码一并核对；本机没有 Maven wrapper 或 `mvn` 可执行文件，因此本轮不伪报运行了 Embabel 测试。Anthropic 的 [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents) 继续作为模式定义证据：orchestrator-workers 的差异是子任务由模型按输入动态决定，evaluator-optimizer 只有在评价标准清晰且反馈能可测改善结果时才值得增加复杂度。

| 证据 | 保留的语义 | 治本拒绝的实现形态 | P7 owner |
|---|---|---|---|
| Embabel `HybridUtilityPlanner` 与旧 `planning/utility` | goal-satisfied 必须先于候选评分；同分保持声明顺序；非法 cost/value 必须失败 | NIRVANA sentinel、多 Goal 暗中仲裁、单步 greedy 被包装成新 Strategy、未有消费者就恢复 utility package | 继续作为未来 `planning.Planner` 证据；P7 不实现 |
| Embabel `SupervisorInvocation`/`SupervisorAction` | schema-informed worker 描述、typed goal、每轮重看已结算结果、显式正数上限 | 把全部 worker 藏进一个 synthetic Action、Action 内模型 while-loop、hard-coded 10、到限只日志、按 Blackboard 类型存在猜完成 | Interaction + managed Delegate + completion validator |
| Embabel `CurriedActionTool`/typed supervisor tests | 模型参数应尽量只含尚需提供的任务语义，名称/描述/schema 必须针对模型设计 | mutable Blackboard、thread-local AgentProcess、按运行时类型 currying 并改变 Tool schema、直接 `Action.execute` | exact Delegate 的静态 Tool schema；已知 artifact 进入模型上下文，不改写冻结 manifest |
| Embabel artifact sinks | Tool/worker 结果可作为后续模型决策的结构化证据 | `Any` sink、全局 Blackboard、按 class name 过滤、把产品 artifact store 下沉 Framework | Strategy-owned immutable typed artifact state |
| 旧 Go `workflow.Supervisor`/`runtime.AgentTool` | 模型可以选择 worker；worker 应有 child Process identity、预算、取消和恢复 | builder 持有 Engine、Tool callback 从 context 反查 parent、直接 RunChild、GOAP 外壳套 ReAct | Interaction Execution 经 StartChild/WaitForChildren；Tool/Dispatcher 无 Engine |
| 旧 `RepeatUntilAcceptable` | attempt/feedback history、显式 threshold/limit、best-so-far 不被较差后续结果覆盖 | Task/Evaluator I/O 藏在 Action、Blackboard binding、默认 limit、把 loop 编译成 GOAP | Workflow Loop + optimizer/evaluator child + typed state |

关键结论：当前代码没有一个“通用 framework Action”。`planning.Action` 是预测搜索值，缺少 Tool 的 JSON I/O 和执行行为；从它自动产生 Tool 在类型和副作用语义上都不成立。P7-02 因此按 ADR-A2-040 改为 exact child Deployment 的 managed Delegate。该桥接必须把模型友好的 name/description 与 Engine-owned child identity 分开，参数直接采用目标 Input schema，结果验证目标 Output schema；它不引入 Supervisor package、动态 registry 或第二 Process 启动入口。

### 11.2 Managed Delegate 实现证据

- `interaction.NewDelegate` 从 concrete target Deployment 冻结 model-facing name/description、exact DeploymentRef、权威 Input/Output schema、每次调用 Budget 与 attenuated Capabilities。目标 Input 不是 JSON object，Description 为空、未裁剪或过长，预算非法或 Deployment 不完整都会在构造期拒绝；`DefinitionConfig.Delegates` 拒绝同名项，`NewDispatcher(definition, config)` 拒绝 Delegate 与普通 Tool 重名。
- Interaction ExecutionState v2 以 `NextCall`、`ActiveCallEnd`、`SettledResults`、`DelegateSegment`、child key/ProcessID/WaitID 表达完整恢复状态。模型 batch 按连续普通 Tool/Delegate 区段推进；普通 Tool 保留既有并发和 HITL checkpoint，多个 Delegate 以同一 Framework Effect batch 启动并 wait-all，最终 ToolResult 始终恢复为原模型调用顺序。
- 空参数按标准空 object 处理；非法 JSON/输入 schema、definite child-start failure 和 child 非 Completed 终态都形成有界 `IsError` ToolResult，允许模型在下一轮改选。Completed child 必须存在 Output 且再次通过该 Delegate 冻结的 Output schema；身份、wait spec、结果顺序或 schema 错配按合同错误终止父 Process。
- 真实跨 Strategy 测试以 Workflow child 验证普通 Tool + 两个同区段 Delegate + 普通 Tool 的混合 batch、exact manifest、声明顺序结果、独立 Process、每 child Budget/Capability。另一个测试覆盖非法参数与 resolver 不可用不会创建幽灵 child；等待态测试在 parent Waiting/child Paused 时 capture 整棵树，销毁原树后 exact restore、恢复既有 child 并完成，最终仍只有 root + one child。
- `interaction/dispatcher.go` 的 AST guard 禁止 Engine、Process、`StartChild`、`WaitForChildren`，证明 schema exposition 没有长出第二生命周期入口。strict restore fuzz 覆盖 awaiting-start 与 waiting-child 状态，race 和完整 examples 共同保护恢复与并发边界。

### 11.3 Typed artifact 与 completion validator 实现证据

- Interaction ExecutionState v3 新增严格有序的私有 artifact records。每条记录只在 exact Delegate child Completed、Output 存在且再次通过冻结 schema 后一次性追加；同模型轮次按原 ToolCall index 严格递增，重复 call identity、未知 Delegate、错序或错误 schema 在 restore 时确定拒绝。普通 Tool、失败 Delegate 与应用 artifact store 没有进入该状态。
- validator 只看到 opaque `CompletionCandidate` 的 defensive-copy Interaction WorkingContext、Output 与 immutable `Artifacts`；WorkingContext 明确不是 Host conversation/transcript 且不含当前候选。`Artifact` 只公开 exact Delegate name、immutable erased Output 和 typed `DecodeArtifact[T]`。ProcessID、ChildKey、CallID、Go reflect type、Blackboard binding、Host identity 与 mutable slice 均没有进入公共完成合同。
- 真实跨 Strategy 测试先由 Workflow child 产生 typed Delegate output，再让模型给出过早答案；validator 成功严格解码 artifact 后拒绝，下一模型请求精确保留原 assistant candidate 并追加 actionable user feedback，第三轮被接受。测试同时篡改 `Artifacts.All()` 返回切片，证明无法反向修改 Execution state。
- direct-result 测试证明同一 validator 能拒绝 `CompletionSourceDirectToolResults`，并精确保留 assistant ToolCall、ToolResult、feedback 顺序；模型最终响应随后正常完成。零值/矛盾 decision 进入稳定 contract failure，validator error 与 panic 分别保留 execution/panic 分类；拒绝后达到 `MaxModelCalls` 进入既有稳定 limit failure，不泄漏未接受候选。
- strict restore 单测拒绝未知 Delegate、错误 Output schema、重复 ToolCall identity 与逆序 artifact；state fuzz 新增携带 settled artifact 的 awaiting-model seed。实现只修改 Interaction 私有 state 和 Interaction API，没有新增 Kernel 字段、Supervisor package、产品 Store 或第二 dispatcher/lifecycle。

### 11.4 Orchestrator-worker 组合实现证据

- `examples/orchestrator_workers` 是独立可运行 command consumer，不依赖 test-only helper 或内部协议。一个 decomposer Interaction 按输入生成 consumer-owned `[]workerTask`，Workflow Map 以 `ItemLimit=8`、`WindowSize=2` 创建三个 exact worker child 并按任务顺序收集 typed results，另一个 synthesizer Interaction 生成最终 consumer-owned report。最终 TreeSnapshot 恰好包含 root + decomposer + 三个 worker + synthesizer 六个 Process。
- 同一 consumer 的行为测试证明另一条正交路径：Interaction 将同一个 exact Planning Deployment 暴露为 Delegate，模型同轮选择两个不同 typed task；两个 GOAP child 各自 observe/plan/act/reobserve 并达到 Goal，返回两个 `planning.Output`。模型从有序 ToolResult 综合答案，completion validator 从 immutable artifacts 独立严格解码并确认两个 Outcome 均为 Achieved；最终树恰好包含 root + 两个 exact Planning child。
- 两条路径都只经 Engine `StartChild`/`WaitForChildren`、exact DeploymentRef、Descriptor schema 和既有 Strategy state 推进。代码没有增加 Supervisor/Worker/Task/Team package 或公共类型，没有让 Tool/Dispatcher 持有 Engine，没有让 Workflow 解析模型语义，也没有让父级检查或遥控 Planning 的 Plan/Action。
- 组合审计确认：动态选择少量 known worker 用 Delegate；动态任务列表需要确定的限量、窗口、顺序和恢复时，用 Interaction 输出 typed plan，再交给 Workflow Map；Planning 只服务可机器验证 Goal 的 exact worker。业务 task/result schema、prompt、综合规则和 artifact persistence 始终属于消费者。
- 该纵切面只增加 example 与行为测试，五个 package API digest、Interaction ExecutionState v3、Process Snapshot v3 和 TreeSnapshot v1 全部不变。它证明现有公开 API 已足够人体工程学，没有用 speculative convenience API 掩盖装配边界。

### 11.5 Evaluator-optimizer 组合实现证据

- 只读审计旧 Go 与 Embabel `RepeatUntilAcceptable`，保留 original input、ordered attempt/feedback history、latest feedback、best-so-far、显式 acceptance criteria 和 hard iteration cap；移除 GOAP/Action 包装、mutable Blackboard、runtime type binding、timestamp、默认 threshold/limit，以及把“上限停止”和“达到标准”混成同一个 acceptable 条件。`flow.Loop` 只作为 bounded value threading/失败保留上一合法 value 的设计证据，不形成依赖或第二 Journal。
- `examples/evaluator_optimizer` 只使用公开 Workflow API。root Loop 的 consumer-owned `optimizationState` 显式携带 objective、history、current、best 和 accepted；每轮 body 是 exact Workflow child，内部按 Call 顺序启动 exact optimizer 和 evaluator child。optimizer 把最新 feedback 写入下一候选，evaluator 追加 attempt，并且只有严格更高 score 才替换 best，因此同分稳定保留最早结果。
- score schedule、归一化 `[0,1]` 合同、threshold、feedback 文本与 report 都属于示例消费者；threshold、score schedule、max iterations 和 exact child refs 进入对应 Deployment configuration identity。Framework 没有新增 Score、Feedback、Attempt、Evaluator、Optimizer 或 RepeatUntil 类型，也不假设任何业务评分方向。
- 行为测试覆盖三轮达到阈值、第一轮提前接受、三轮耗尽仍返回中间最高分而非最后低分、同分保留最早、feedback 确实进入下一候选，以及 zero iteration、score 数量错配、zero/NaN threshold、infinite score 的构造期拒绝。达到阈值时 `Satisfied=true`；耗尽时 `Accepted=false`，没有把停止冒充成功评价。
- 三轮场景 TreeSnapshot 恰好十个 Process：root + 三个 iteration body + 三个 optimizer + 三个 evaluator；首轮达标场景按 exact Deployment 名称验证四 Process。worker 与 body 都沿用既有 child budget、schema、失败、取消和恢复合同，没有第二 scheduler、共享 history store 或隐藏 callback loop。
- 该纵切面没有生产 package 变化，五个 API digest、Workflow state、Process Snapshot v3 和 TreeSnapshot v1 均不变。外部/模型 evaluator 将来只需替换 exact worker Deployment；不得把 I/O 放进 Transform、Loop predicate 或 completion validator。

### 11.6 Anthropic 模式覆盖证据

| 模式 | 可运行 consumer | 关键行为断言 |
|---|---|---|
| Augmented LLM | `direct_vs_managed`、`autonomous` | direct/managed 共用 chat contract；Tool 结果进入 WorkingContext 后模型停止；retrieval 沿用 Tool/provider 而非专用 Strategy |
| Prompt Chaining | `workflow_patterns` | normalize 与 summarize 两个 exact child 串行，后者只能读取前者 typed output |
| Routing | `workflow_patterns` | urgent/standard 两种输入都只创建被选 exact route child，未选 branch 不出现在 tree |
| Parallel Sectioning | `workflow_patterns`、`workflow` | facts/risks 与 clarity/safety exact child 按显式窗口执行，结果始终按声明顺序归位 |
| Parallel Voting | `workflow_patterns` | 四个 exact voter 消费两份 typed section evidence 并分两个 window；2–2 同票由 consumer reducer 稳定保留最早声明 choice |
| Orchestrator-workers | `orchestrator_workers` | 模型动态任务列表→有界 Map→综合；模型同轮选择两个 exact Planning Delegates |
| Evaluator-optimizer | `evaluator_optimizer` | latest feedback 进入下一候选；提前接受；exhausted best-not-last；同分保留最早 |
| Autonomous Agent | `autonomous` | model→Tool→model final，模型决定 Tool/停止，Definition 提供 hard `MaxModelCalls` |
| Pattern Composition | `composition`、`orchestrator_workers`、`evaluator_optimizer` | Interaction/Planning/Workflow 经同一 child Process 窄腰嵌套，无第二 runtime |

- `workflow_patterns` 每次运行的 TreeSnapshot 恰好十个 Process：root、两个 prompt-chain child、一个 selected route、两个 section、四个 voter。行为测试按 exact Deployment name 验证 standard route 不创建 urgent child，并验证 urgent 完整树；稳定 output 是 chain、route、facts/risks、approve 2/4 和 process count。
- 七个 command 都是公开 API consumer，不共享 test-only/internal protocol helper，使用 deterministic local component，可在 `GOWORK=off` 下无凭据运行。一个 command 可以覆盖多个模式，但表中每一行都有独立行为断言；只有类型存在或文档名称不算覆盖。
- 模式名称没有进入生产 package/type/kind。新增 consumer 只组合已有 Call/Switch/Fork/Map/Loop、Interaction/Tool/Delegate 和 child Process；retrieval 是 Tool/provider，WorkingContext 不是长期 memory。全部五个 package API digest、Strategy state 与 snapshot/tree wire 保持不变。

### 11.7 P7 direct-first 与抽象删除终审

| 层级 | 实际成本/边界 | 选择条件 |
|---|---|---|
| direct `chatclient` | 0 Process；调用方自己拥有上下文、错误和重试 | 一次/少量模型调用，无托管生命周期 |
| 普通 Go / `flow` | 0 Agent Process/node；同进程 typed control flow，可有自己的局部 run/journal 语义 | 节点不需要 Agent identity、预算、取消传播或 tree recovery |
| Interaction | 1 Process；模型/Tool Effect、WorkingContext、HITL/steer/snapshot/limit | 自主循环需要托管但没有独立 worker tree |
| managed Workflow | root + 每个真实 child Process；示例实际为 4/6/10 Process | 分支/迭代确实需要独立身份、预算、能力、取消和恢复 |
| Platform | 启动前选择 + 同一个 Engine；示例仍为 2 Process | 需要多 Deployment catalog、版本选择和统一治理；不增加第二 runtime |

- `go mod why github.com/Tangerg/flow` 证明 `flow` 不是 agent2 依赖；standalone package DAG 继续是 root ← Interaction/Planning/Workflow，GOAP 只依赖 Planning。生产源码扫描没有模式专用 Supervisor/PromptChain/Router/Vote/EvaluatorOptimizer/Team 类型，没有旧 `agent`/应用 import、Store/Blackboard 或第二 runtime。
- P7 public surface 逐项有 owner/consumer：Delegate 由模型选择 exact child；Artifact/Artifacts/CompletionCandidate 分别承载 schema-owned evidence、稳定集合与两种完成候选；completion validator 提供纯反馈边界。Workflow 的 Transform/Call/Switch/Fork/Map/Loop 各有不同状态/恢复语义并被 command 或 contract tests 消费；Sequence/Gate/Vote/PromptChain 均由现有代数派生，没有近义入口。
- 七个 command consumer 不合并：最小 direct/managed、自主 Tool loop、自定义 Definition/heterogeneous composition、最小 managed Workflow、动态 worker、评价循环、完整确定模式矩阵各自承担不同公开 API 证据。复杂示例明确是 topology fixture；纯函数业务默认应留在 Go/`flow`，不是为了制造 tree 而创建 Process。
- 本轮没有发现应删除的当前生产抽象。P6/P7 前序已经实际移除了 StageKind/metadata getter、独立 Supervisor、通用 Action-to-Tool、Blackboard、pattern-specific kinds 等无收益设计；终审不为制造 diff 误删已被真实消费证明的能力，也不新增 facade/shared example helper。

## 12. P8 Platform 与治理专项证据

### 12.1 DeploymentResolver 消费合同

- 真实消费点只有跨 Deployment child start 与完整 tree restore；两处输入都已经是 exact DeploymentRef，不需要也不允许 resolver 再做路由。same-reference child 复用当前 Deployment，tree restore 以 root Deployment 为首个绑定并按 distinct reference 缓存其余绑定。
- `DeploymentResolver.Resolve(DeploymentRef)` 是并发安全、同步有界、确定、无远程 I/O 的本地绑定查询。移除 `context.Context` 是语义修复：旧参数全部被忽略且 Engine 主动剥离取消，它只会暗示 tenant/context routing 或远程发现可以潜入恢复窄腰。
- Engine 对 resolver 返回值执行 Valid 与 exact reference 双重校验；错绑定和缺失绑定不会创建 child，panic 被收敛为确定解析失败。tree restore 在全部 Deployment、Snapshot 与 child wait 都准备成功后才注册，解析失败不留下部分 Process。
- Platform catalog 将实现该窄腰，但 Engine 不依赖 Platform concrete type。可取消的远程发布同步、调用方/租户选择、版本范围路由和业务权限属于 catalog 构造或更高层选择，不得塞回 resolver。
- 本轮完整 race 门禁暴露旧 Await 先于 `processFinished` 返回的窗口。现 Await 以 termination bookkeeping barrier 为线性化点：Result 已提交且 direct parent/child 通知已完成，但不等待全部后代终止；因此父终态事实不能再被 caller 抢跑成孤儿 child 的正常完成。

### 12.2 不可变 exact Deployment catalog

- 旧 deployment catalog 的有效证据是：replacement 后历史 exact Deployment 必须继续可恢复；并发读需要一致快照；枚举必须稳定。拒绝复制它把 active route、历史 binding、retain/forget、mutable registry 和 Engine lifecycle 混成一个 owner 的实现。
- `platform.Catalog` 只收敛 exact bindings。零值为空；构造 all-or-nothing；无效 Deployment 或重复 exact reference 明确失败。同 name/version 的不同完整 digest 可以共存，证明 Catalog 没有暗中选择“当前版本”。
- `Resolve` 对 invalid ref 与 valid-but-absent ref 分别返回准确 sentinel，不做 name/version fallback；同一 Catalog 可直接作为 Engine DeploymentResolver。`Deployments` 返回独立 slice，并按 name、SemVer、digest 排序，供后续路由从同一已提交快照投影候选。
- Catalog 没有 mutex 或写方法，复制值后只读，天然支持并发解析/枚举。它不序列化 Definition/Dispatcher，不声称 durable，不依赖 Interaction/Planning/Workflow、旧 agent 或 Host package；architecture gate 固化该 DAG。

### 12.3 显式部署变化与版本槽位

- `platform.Platform` 只拥有 deployment aggregate，不拥有 Process。初始 Deployments 一次性验证；每次本地变化在单一临界区构造并发布完整 Catalog + active map + stable active list，不让读者看到半变更。两个 Platform 实例完全隔离，无 package-global registry。
- active identity 是 name + canonical SemVer，不是裸 name。1.x/2.x 可同时 active；相同 name/version 的不同 complete digest 必须 Deploy conflict → 显式 Replace。Replace 一个版本不影响其他版本，旧 exact Deployment 永久保留供 resolver/restoration。
- Deploy 当前 exact binding 是无变化；Replace 不存在的版本返回 not-active，不能借 Replace 偷建新版本；Undeploy 要求 current exact ref，stale ref 返回 Active/Requested conflict，成功后只移除 active route 而不删除 history。
- initial construction conflict、invalid Platform/Deployment/Ref、not-found exact binding、not-active slot 和 occupied/stale conflict 各有独立 error 语义。并发测试覆盖不同版本的 Deploy、Replace 与同时 Candidate/Catalog read，所有 active ref 在任一已读 snapshot 后都仍可 exact Resolve。
- 不实现 Forget/retention count、database transaction/CAS、idempotency key、remote deploy 或同步 listener。外部持久发布、历史清理政策和生命周期落库属于 Host；Framework observation 边界留给 P8-06。

### 12.4 active Definition discovery 与 exact selection

- 旧 Router/Ranker/Candidate/Choice/Confidence 只保留“stable candidates + caller-owned policy + exact result validation”思想，不复制 Goal catalog、固定 score/rationale、filter 组合或 Engine-owned router。新唯一入口是 `DeploymentSelector.Select`，函数实现可用 `DeploymentSelectorFunc` 直接适配。
- `DeploymentCandidate` 只包含 exact ref 与 frozen Descriptor；selector 无法取得 Dispatcher/Definition、Engine 或 Process。candidate list 来自一次 stable active snapshot，按 name/SemVer/digest 排序并与 caller mutation 隔离；historical Catalog binding 不进入发现。
- Selector 自己封装 request-specific typed/text input、model、threshold/filter 与 rationale，因此 Framework 不定义 `any`/RawMessage 路由 payload，也不假设 confidence 范围。外部 I/O 允许且受 caller context；nil/typed nil、panic、ordinary error、invalid/unoffered ref 都有明确合同。
- Platform 在 selector 返回后从原 captured Deployment map 取值，而不是重新查 active route。并发 replacement 测试在 selector 阻塞期间切换同版本 binding，最终仍返回旧 exact selected Deployment，同时新 binding 保持 active、旧 binding 继续可 Resolve。
- 该能力只选择，不运行。Engine 不 import Platform concrete type，Selector 不拥有 Process lifecycle；动态模型 worker 的 Delegate 与全局 Definition discovery 没有被混成同一个概念。

### 12.5 根与子 Process 的统一启动准入

- P4 已完整实现跨 Process 的 Budget 永久划拨、TreeLimits、CapabilitySet subset attenuation 与 Effect required-capability enforcement；P8 不复制这些状态，也不允许 policy 扩充它们。一个专项测试证明即使 admitter 返回批准，capability escalation 仍在 admitter 调用前被 Engine 拒绝。
- 唯一公共术语是 `ProcessAdmission`/`ProcessAdmitter`。Admission 只有 private ProcessRelation、exact DeploymentRef、Descriptor、Budget、CapabilitySet；architecture reflection gate 锁定字段数量、类型和私有性，Input、Execution、Dispatcher、Engine 与 Host 产品概念均不能渗入。
- 根与 child 使用同一 EngineConfig admitter。真实跨 Strategy child 测试证明 root 收到完整默认 Budget/最大 capability，child 收到 Strategy 申请且已衰减的 20/20/40 Budget/空 capability；两者 relation、exact ref 与 Descriptor 都准确。拒绝发生在 Definition.Start 与 Engine publication 前，root 保留 ordinary cause，child 返回稳定 `engine.child.admission.rejected` 且不留下幽灵 Process。
- typed nil 与 panic 都不会逃逸 Engine。admitter 必须同步、有界、无外部 I/O、不重入 Process、并发安全且 decision-only，并容忍 prepared child start recovery 后对同一稳定身份重判；它不是 durable charge、Store 或幂等协议。root 远端审批在 Start 前完成，child 远端审批先显式结算 Dispatcher Effect，准入不偷成第二副作用入口。
- Restore 不调用 admitter。恢复严格复用 snapshot 中已准入的 exact Deployment、Budget、Capabilities 与 tree relation；live authorization 由 Host 在 restore 前决定，不能潜入 deterministic restoration。没有通用 StopPolicy：资源终止、外部控制与 Strategy completion 分别由现有唯一 owner 处理。

### 12.6 自足 Framework Event 与 OTel adapter

- 原 Event 缺 exact Deployment 与 ProcessRelation，`step.prepared/committed` 也不是 Execution.Step 的 started/finished；Dispatcher 与 Framework Effect 的观察面不对称。P8-06 没有用 OTel 猜补这些事实，而是先修正 Framework owner 的 Event 合同。
- Event envelope 现在自足携带 Process-local sequence、ProcessID、exact DeploymentRef、ProcessRelation、可选 StepSequence/EffectID、name、phase、OccurredAt 与 immutable payload。strict JSON round-trip 验证 exact binding/relation，独立 observation wire digest 固化 Event/Delta shape。
- 根发布点只使用统一 Event 常量。真实两 Step/一 Effect 运行的顺序固定为 Process started → Step started/finished/prepared → Effect started/finished → Step committed → Step started/finished/prepared/committed → Process finished；每个 Event 的 sequence、exact ref、root relation 都有行为断言。Framework StartChild 另有测试证明使用同一 Effect lifecycle 且 target=framework。
- Event/Delta listener 治本删除永远被忽略的 error 返回值并新增 Func adapter；panic 仍隔离。Event 实现承担 bounded/no-reentry 合同，Delta 继续使用 bounded async queue，drop 同时进入 Usage 与 Event。
- `agent2/otel` 是单向 adapter，不是第二观察总线。生产代码只 import 根 Framework 与官方 OTel API；Kernel 禁止 OTel，adapter 禁止 SDK、Strategy、旧 agent 与 Host。SDK 只在测试中证明真实 spans、parenting、exact deployment attributes、status/target，以及 Process starts/exits、Step/Effect duration metrics。
- raw payload、Input/Output、产品身份、日志 backend、exporter 生命周期都不进入 Framework 或默认 telemetry attributes。模型/Tool/Action 的 Strategy-specific observability 由相应 dispatcher/adapter owner 提供，Kernel 不通过 opaque Effect 猜测。

### 12.7 embedded Engine 与完整 Platform 共用一个 runtime

- 新增第八个独立公开 command `embedded_vs_platform`。同一个 Workflow root 通过 exact child Call 调用同一个 worker：嵌入式路径使用 caller-owned resolver 并直接提交 root；Platform 路径从 non-executable active candidates 选择 exact root，再把 Platform 本身作为 Engine resolver。两条路径没有共享 test helper，也没有调用 internal protocol。
- 两次运行都通过同一个 EngineConfig 位置装配 ProcessAdmitter 与 EventListener。行为比较覆盖 typed Output、Completed、Usage、root/child exact Deployment tree、root/child admission facts，以及按 deployment/depth/step/target/status 归一化的 Process/Step/Effect lifecycle；专项普通 100 次与 race 20 次证明一致。
- 真实运行也证明完整 Event sequence 不能成为跨运行等价条件：child 若在 wait 注册前完成，父可直接继续；若在注册后完成，父会提交 Waiting 并接收 Signal。该调度差异不改变 Result/tree/Usage/实际 Step 与 Effect 语义，文档不把 `signal.accepted`、中间 running/waiting、ID 或时间误写为 deterministic output。
- 冻结前删除单字段 Config，构造收敛为 `New(deployments...)`；Platform 零值可直接 Deploy/Resolve/select empty。删除 executable `ActiveDeployments` 枚举，active discovery 只保留 immutable DeploymentCandidate，阻止 Dispatcher/Definition behavior 从发现面泄漏。Catalog/Resolve 继续服务 exact history，SelectDeployment 是 active executable binding 的唯一出口；nil Platform/selector 只返回精确的 ErrNilPlatform/ErrNilDeploymentSelector，不再冒充一般 Invalid。
- Platform 不持有或包装 Engine，不提供 Start/Run/Restore/Process facade，不拥有 ProcessAdmitter、EventListener、OTel、Store 或 Host policy。Platform architecture gate 进一步禁止 OTel 与所有 Strategy/legacy/Host imports；完整形态只是 Host 对 selection + resolver + 同一个 Engine 的显式组合。

## 13. P9 独立完整性验收证据

### 13.1 名称、参数、GoDoc 与 error 合同

- 七个公共 package 已逐项通过 `go doc -all` 和源码 AST 复审。完整 behavior binding 只叫 Deployment，exact identity 只叫 DeploymentRef；同理 Process/Event/Effect/Signal 的 sequence、cursor、consumption 和 batch index 都带 owner，不再靠调用位置猜含义。
- 所有公开 callable 参数都有语义名称，所有公开 declaration/struct field 都有以 exact identifier 开头的 GoDoc。门禁直接扫描 production source，后续匿名 interface 参数、合并字段注释或 promoted callable 文档缺失都会失败。
- error sentinel 按真实失败条件命名；普通 cause 全部用 `%w` 保留 `errors.Is/As`，panic recovered value 和数字等非 error 值才允许 `%v`。静态门禁会阻止 `err`、`*Err`、`*Error` 再通过 `%v` 断链。
- 私有实现同一标尺收敛：Engine-owned goroutine 是 processLoop，不叫 runtime；controller identity、last-stable state、pending control、final output 和 terminal snapshot 都直接写明 owner/提交含义。职责文件同步按 input/output/capability/error/name/json/typed-nil 拆分，没有恢复 `types.go`、`util.go` 或 `helpers.go` 杂物桶。
- wire 改名是显式 breaking revision：Process Snapshot v4、TreeSnapshot v2、child/framework-effect protocol v2 与 Planning ExecutionState v2 只读当前精确字段，不保留旧 tag、alias 或双读。Event/Delta 使用 ProcessSequence/EffectSequence 与 exact DeploymentRef，并由 observation wire digest 独立守卫。

### 13.2 API 与完整 wire baseline 覆盖

- 独立复核证明 Baseline 4 的 public `go doc` 守卫完整，但共同 snapshot digest 无法看见 opaque Strategy payload 内部 shape；这不是 Kernel 应该取得解释权，而是各 Strategy schema owner 缺少自己的恢复/协议 baseline。
- Interaction baseline 现覆盖 ExecutionState v4、dispatcher Effect/Signal protocol v2、model Delta、Tool checkpoint、Delegate settlement 与 artifact provenance；Planning baseline 覆盖 ExecutionState v3、observe/action protocol v1、Condition/WorldState/Plan wire；Workflow baseline 覆盖 ExecutionState v2 与 fan-out child window。各包新增 private JSON struct 未登记时直接失败。
- Kernel baseline 现覆盖全部 production `*Wire`，包含 Descriptor、Framework wait/child Effect/Signal、Process Snapshot v5、TreeSnapshot v3 及其嵌套值；新增 production wire 未登记时直接失败。Kernel 仍只校验 opaque ExecutionState envelope，不 import 或解析任何 Strategy phase/state。
- Framework-owned Event payload 从匿名 struct 收敛为命名合同并进入 observation baseline；OTel adapter 只按 Event 名称投影 `process_status`、`termination_cause`、`step_status`、`effect_target`、`settlement_status` 与 `dropped_delta_count`，没有反向进入 Kernel。
- prior-version tests 明确拒绝 Process v4、Tree v2、Interaction state v3/protocol v1、Planning state v2 与 Workflow state v1；没有兼容 alias、双 tag、dual-read 或迁移 helper。七个 public API digest 未变化，P9-02 只修复真实 baseline 覆盖缺口。

### 13.3 完整 package DAG 与边界门禁

- 当前生产 package 集合精确锁定为 root、Interaction、Planning、Planning/GOAP、Workflow、OTel 与 Platform；新增目录中出现 production Go package 会直接失败，不再依赖人工发现。
- 唯一允许的 agent2 内部直连边是 Interaction/Planning/Workflow/OTel/Platform → root，以及 GOAP → Planning；声明图还会独立检查无环。因而 Kernel 反向 import Strategy、Strategy 横向耦合、GOAP 绕过 Planning contract、adapter 取得 Platform/Strategy 所有权都会失败。
- Host `app`、旧 `agent`、`flow` 和 logging backend 对全部生产 package 禁入；OTel 只属于 `otel` adapter，chatclient/tool/core-chat 只属于 Interaction。OTel SDK、Host 抽象名称、Dispatcher lifecycle 和 sealed Workflow algebra 等 owner-specific 规则仍留在各自本地门禁，没有重复维护内部 Strategy 清单。
- examples 故意作为图外公开消费者，可以组合多个 Strategy/Platform；它们不豁免独立编译和实跑。Workflow 仅吸收 `flow` 的显式组合、确定顺序与有界 fan-out 思想，代码依赖保持为零。
