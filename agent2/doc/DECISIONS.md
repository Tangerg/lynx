# Agent Framework 架构决策记录

> 状态：持续维护
> 建立日期：2026-08-06
> 最后更新：2026-08-06

本文只记录影响长期结构的架构决策及其理由，不复述目标架构，不记录任务进度。

- 目标设计见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 能力取舍与消费者证据见 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md)。
- 工程实施标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。
- 实施进度见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

改变已接受决策时，不直接改写历史结论。应追加新的 ADR，注明被取代的旧 ADR，并同步更新目标架构。

---

## ADR-A2-001：采用平行模块绿色重构

- 状态：已接受。
- 决策：在 `agent2` 独立实施，不在旧 `agent` 上继续根本性修补。
- 原因：隔离架构验证与应用消费迁移，消除历史模型对新执行窄腰的约束。
- 后果：开发期间允许有限的实现重复，但禁止新旧模块互相依赖。

## ADR-A2-002：`agent2` 是临时路径，不是永久版本概念

- 状态：已接受。
- 决策：最终删除旧模块，并把新模块目录和 module path 改回唯一的 `agent`。
- 原因：一个领域只保留一个术语和一套公共 API。

## ADR-A2-003：直接 AI 能力保持独立

- 状态：已接受。
- 决策：继续使用 `chatclient`、`embeddingclient` 和 `tool`，Agent 不复制 Client、Model、Message、Embedding 或 Tool 协议。
- 原因：渐进式集成的真实最小入口是基础库，不是所谓 Embedded Agent Mode。

## ADR-A2-004：共同 Process 与 Planning 解耦

- 状态：已接受。
- 决策：Goal、WorldState、Blackboard 和 Plan 不进入共同 Process/Snapshot。
- 原因：这些是 Planning 状态；放入共同抽象会迫使其他 Strategy 伪装。

## ADR-A2-005：Definition/Execution 是执行窄腰

- 状态：已接受，精确 Go 签名待 P1 验证。
- 决策：不可变 Definition 创建或恢复 per-Process Execution，Engine 只推进有界 Step。
- 原因：统一生命周期而不统一各 Strategy 的内部状态。

## ADR-A2-006：Execution Strategy 是一等替换点

- 状态：已接受。
- 决策：Interaction、Planning、Workflow 和 Supervisor 是平等 Definition，不是 Mode 枚举或 Extension。
- 原因：它们拥有不同状态推进语义，强行压入同一配置会形成 god abstraction。

## ADR-A2-007：Workflow 使用原生 Execution

- 状态：已接受。
- 决策：Workflow 不编译成 GOAP Agent。
- 原因：固定控制流与目标搜索是不同问题；原生 Workflow 才能准确表达 fork/join/gate/loop 和恢复。

## ADR-A2-008：GOAP 保留但不作为默认中心

- 状态：已接受。
- 决策：GOAP 是 Planning 下的 Planner，服务于可验证目标、多路线和动态重规划场景。
- 原因：保留 Embabel 最有价值的思想，同时不让它限制 ReAct、Workflow 和未来策略。

## ADR-A2-009：子 Agent 是 Process 关系，不是新类型

- 状态：已接受。
- 决策：统一使用 Child Process；同一 Definition 可以递归创建新的 Process。
- 原因：保持实体模型最小，并让所有 Strategy 共享组合能力。

## ADR-A2-010：递归由 Engine 调度和治理

- 状态：已接受。
- 决策：父 Process 以 waiting/resume 组合子 Process，不依赖递归 Go 调用栈；预算、权限、深度和取消由 Engine 强制。
- 原因：支持持久化恢复，并防止成本指数扩张、权限提升和失控递归。

## ADR-A2-011：Action 与 Tool 分离

- 状态：已接受。
- 决策：Action 是框架类型化操作，Tool 是模型可调用协议；通过 adapter 组合，不共享含混基类型。
- 原因：二者的消费者、描述要求和治理边界不同。

## ADR-A2-012：Framework snapshot 与 Host persistence 分层

- 状态：已接受。
- 决策：Agent 负责执行状态捕获、验证和恢复，Host 负责 Store、事务、CAS、lease 和产品 write-set。
- 原因：防止应用持久化抽象泄漏进 Framework，也防止 Host 解析 Strategy 内部状态。

## ADR-A2-013：Engine 与 Platform 分层

- 状态：已接受，精确 API 待实现验证。
- 决策：Engine 是最小 Process 执行内核；Platform 是可选的多 Deployment 目录、路由和治理容器。
- 原因：同一框架同时支持局部嵌入和完整 Agent 应用，又不把 Engine 做成 god object。

## ADR-A2-014：只保留一个主生命周期循环

- 状态：已接受。
- 决策：Engine 是唯一 Process loop；Strategy 只实现有界 Step，不各自复制 runtime、event bus 和恢复系统。
- 原因：统一生命周期是 Framework 成立的根本，重复循环会造成状态和恢复语义分叉。

## ADR-A2-015：不自动注册或默认选择执行策略

- 状态：已接受。
- 决策：Definition 显式装配所需 Strategy；Engine 不按名称自动注册 GOAP/ReAct/Workflow，也不设置普适默认 Planner。
- 原因：调用方必须清楚选择了何种控制流，避免方便性掩盖错误语义。

## ADR-A2-016：旧模块是并存期参考实现，不是新模块规范

- 状态：已接受。
- 决策：实施时直接参考旧 `agent` 的代码、测试和文档，但不建立 import、兼容或共享混合抽象。
- 原因：并存使经过生产验证的细节无需从 Git 历史恢复；同时必须避免旧结构反向决定新设计。
- 执行要求：每个能力阶段明确裁决旧实现是保留思想、重新实现还是移除，并用新合同重新验收。

## ADR-A2-017：阶段内一步到位不等于提前实现未来能力

- 状态：已接受。
- 决策：每个阶段只实施已经被真实需求证明的范围，但范围内的语义、错误、恢复、测试和文档必须完整，不允许半成品进入主线。
- 原因：同时避免妥协式实现和以“高标准”为名的过度设计。

## ADR-A2-018：事务和业务幂等由下游扩展边界实现

- 状态：已接受。
- 决策：Host 或具体 Action/Tool adapter 通过 decorator、middleware 或其他消费侧扩展实现事务、业务幂等和补偿；Agent Kernel 不定义相关 Store/SPI。
- 原因：这些语义依赖具体应用和外部副作用，不是所有 Execution Strategy 的共同生命周期。
- 限定：Engine 防止同一 Step 并发推进、恢复重复创建 child 等属于框架自身正确性不变量，必须由 Engine 直接保证，不能变成可选扩展。

## ADR-A2-019：采用 Go 风格充血模型和组合式 OOP

- 状态：已接受。
- 决策：无 I/O 的不变量、派生值和状态转移归属领域实体/值对象；多态通过消费侧小接口，复用通过组合；不引入继承层次、单实现接口和 service object 数据袋。
- 原因：让行为与 owner 收敛，同时保持 Go 的简单、显式和低耦合。

## ADR-A2-020：Supervisor 先作为组合方式而非独立 Strategy

- 状态：已接受；取代 ADR-A2-006 中将 Supervisor 预先列为平等 Strategy 的部分，Interaction、Planning 和 Workflow 的一等 Strategy 地位不变。
- 决策：Supervisor 由 Interaction、Workflow、Action-to-Tool、typed artifacts、validator 和 child Process 组合，不预建独立 ExecutionState kind 或 package。
- 原因：当前没有证据证明 Supervisor 拥有无法由既有 Strategy 表达的独立推进与恢复语义；预先升格违反新 Strategy 准入规则。
- 后果：只有真实实现证明独立生命周期后，才能追加 ADR 重新申请准入。

## ADR-A2-021：Signal、Transition、Effect、Event 和 Delta 各有唯一语义

- 状态：已接受。
- 决策：Signal 是 Execution 的唯一入站输入；Transition 是 Step 的候选状态与生命周期意图；Effect 是 Step 之外待执行的 Framework 或 dispatcher 操作意图；Event 是已发生事实；Delta 是非权威临时流输出。
- 原因：命令、状态意图、外部操作、事实和展示增量混用会产生第二状态写入口、恢复歧义和错误的持久化依赖。
- 限定：Signal 的共同信封只含 SignalID、可选 WaitID 路由、接收时间和 raw payload；kind/schema 属于 payload owner，不能成为共同 Process 类型。Effect 只允许区分封闭的 Framework 目标与 opaque dispatcher 目标，不能把具体 Strategy kind 提升进 Kernel。

## ADR-A2-022：Step 只归约状态，外部操作通过策略 Effect dispatcher 执行

- 状态：已接受，精确 Go SPI 待 P1–P3 prototype 验证。
- 决策：Step 不直接调用模型、Tool、Action 或其他 I/O，只消费 Signal 并产生候选状态、Transition 和 Effect。Engine 只执行封闭的 Framework Effect；Deployment 绑定 Strategy-owned dispatcher 解释其余 opaque Effect。Engine 生成稳定 EffectID、调度、记录 settlement，并把结果作为 Signal 送回 Execution。
- 原因：这样才能让 Signal 游标和 Execution state 保持单写者提交，并在失败时拒绝捕获半提交状态，同时不让 Engine 解析 Strategy payload。
- 限定：Engine 的 EffectID 只保证 Framework 寻址和自身实体去重，不自动赋予外部业务事务、补偿或幂等；不能证明可安全重投的 Effect 默认不重放。

## ADR-A2-023：公共 I/O 类型擦除，泛型只在边缘提供人体工程学

- 状态：已接受，精确 value/codec API 待 P1 验证。
- 决策：Engine、catalog 和根 Definition 使用严格、可移植的 JSON wire value；Descriptor 携带权威 input/output schema，schema 进入 Deployment identity。泛型 adapter 只负责 Go 类型与 wire value 的双向转换。
- 原因：Engine 必须同构保存异构 Definition；泛型根接口无法进入统一目录和 Process 恢复合同。
- 限定：Engine 只验证结构合同，Definition/typed adapter 负责 Go 类型和语义不变量；不把任意产品 JSON 或 `any` 塞入窄腰。

## ADR-A2-024：一个 Process 只有一个顶层 Execution

- 状态：已接受。
- 决策：Process 只能由 Engine 构造，只拥有一个顶层 Execution 和 ExecutionState envelope；跨 Strategy 或需要独立生命周期的组合通过 child Process。
- 原因：同 Process 嵌套 Execution 会递归化 snapshot/version/owner，并形成多个生命周期驱动者。
- 后果：Workflow Fork/Map 的独立分支使用真实 child Process，必须在实现前冻结 branch deployment identity、fan-out、预算和聚合 snapshot 语义；无独立生命周期的轻量并发属于节点 Effect batch，不借 Fork/Map 名称绕过该不变量。

## ADR-A2-025：WaitID 由 Engine 铸造且只能通过 Signal 写回 Execution

- 状态：已接受。
- 决策：Execution 先声明 logical wait；Engine 铸造 WaitID 并用内部 Signal 返回；Execution 在下一 Step 保存它并进入 Waiting。SignalID、WaitID、EffectID 和 logical key 分离命名。
- 原因：Engine 在 Step 返回后直接回调或修改 Execution state 会破坏单写者；由 Execution 自行生成外部 WaitID 又会让框架失去稳定寻址和去重所有权。

## ADR-A2-026：终态由已记录控制意图和执行结果共同裁决

- 状态：已接受。
- 决策：Engine 显式 kill 进入 Killed；父 Process 或 Host context 取消进入 Cancelled；Process deadline 或被提升为 Process 终止原因的 Effect deadline 进入 TimedOut；普通 Step error、外部失败、panic 和合同违约进入 Failed；已提交终态 first-terminal-wins。每个终态同时保留稳定 cause，不能仅凭返回 error 推断。
- 原因：同一个 `context.Canceled` 可能来自不同 owner，单看 error 无法还原终止语义。
- 限定：使用小型、表驱动的明确矩阵，不引入通用 ErrorClassifier、Transient/NonTransient 或 retry taxonomy。

## ADR-A2-027：Delta 是有界且非权威的临时输出

- 状态：已接受。
- 决策：Delta 按调用内顺序发布、缓冲有界、允许显式可观察的丢弃、恢复后不重放；最终 Output 必须独立完整。观察 listener 失败不改变 Process 结果。
- 原因：UI 或流消费者断开不能杀死执行，snapshot 也不能因 token chunk 无界增长；最终结果不能依赖可能丢失的增量拼装。
- 限定：policy、admission、Effect settlement 和持久提交不是观察 listener，不能借该规则吞掉正确性错误。

## ADR-A2-028：外部事实失效由 Host 清理，不进入共同 snapshot

- 状态：已接受。
- 决策：Strategy 精确恢复所需状态自足地保存在私有 ExecutionState；确需引用可变外部事实时，只保存 opaque revision/digest 并由 Strategy provider 校验。Host 对自身事实执行销毁、回滚、替换或恢复时，负责终止并清理失效关联的 Process/snapshot/continuation。
- 原因：应用身份、历史水位、删除集合和 write-set 都不是通用 Agent Framework 语义；下沉会重演抽象泄漏。
- 后果：外部事实已变化时不得静默重建旧 Execution；从新事实继续属于新 Process。Framework 只暴露中性 lifecycle/capture 能力。

## ADR-A2-029：Engine 消费 DeploymentResolver，Platform 实现目录治理

- 状态：已接受，精确 API 待 P2 验证。
- 决策：Engine 的恢复路径依赖由消费位置定义的最小 DeploymentResolver，或显式接收已解析 Deployment；Platform 可以实现 durable catalog、版本路由和治理。
- 原因：恢复必须精确定位 Deployment，但 Engine 与 Platform 不能各自拥有一份权威目录。

## ADR-A2-030：公共合同在真实 Interaction 和 child composition 验证后冻结

- 状态：已接受；细化 ADR-A2-005 的验证门禁。
- 决策：P1 只形成 candidate contract；P3 用真实 streaming/tool/HITL/steer 与 disposable consumer spike 验证，P4 再用 child Process/递归/跨 Strategy 组合验证，之后才冻结根窄腰。
- 原因：不含 Tool 的 Interaction 和两个 Action 的 Planning 原型无法触达入站 Signal、流、checkpoint、外部上下文和递归组合这些最硬需求。

## ADR-A2-031：Effect 调度使用 prepared step 与 finalize 两个内部原子边界

- 状态：已接受；补强 ADR-A2-022 的提交顺序。
- 决策：Engine 在 dispatch 前原子记录 Prepared Step，其中包含 last-stable identity、候选 ExecutionState、拟消费 Signal 范围、Transition、稳定 EffectID 和冻结 payload，但不推进权威 state/cursor。取得 settlement 后再原子 finalize 状态、游标、settlement、结果 Signal 和 Process transition。
- 原因：如果 Effect 先发生而 pending intent 尚无可恢复记录，崩溃后无法区分未执行与已执行但结算未知；Prepared Step 使未知结算可定位，又不把 Host Store/transaction 引入 Framework。
- 限定：跨进程恢复只对 Host 已持久化 snapshot 生效；需要 durable recovery 时，Engine 必须在 dispatch 前允许 Host 同步确认 prepared boundary，但不定义 Store/transaction SPI。Engine 不承诺未持久化状态。Step 必须是确定性纯归约，dispatcher replay 仍受 ADR-A2-018/A2-022 的业务幂等边界约束。

## ADR-A2-032：Process tree 使用结构化生命周期

- 状态：已接受；补强 ADR-A2-009、ADR-A2-010 和 ADR-A2-026。
- 决策：父 Process 一旦进入任意终态，Engine 就在每个直接子 Process 的下一安全边界记录父级终止意图，并由各子级继续向下传播；父级 deadline 映射为 child 的 parent deadline，其他父级终态映射为 parent cancellation。Engine kill 只描述被直接 kill 的 Process，不冒充后代的终止原因。正常完成的父级同样不能遗留脱离 Process tree 的活动孤儿。
- 决策：child failure 不自动改写 parent 终态，而是作为 `ChildOutcome` 交给 parent Execution 显式裁决；child wait 只能引用直接 child，不能跨层等待 descendant 或 ancestor。
- 原因：树形身份必须对应树形所有权。让子级脱离已终止父级会破坏预算、能力、恢复和清理边界；另一方面，把任一 child failure 硬编码成 parent failure 会抹掉 all/any/quorum、fallback 和容错编排的策略语义。
- 限定：传播不抢占正在结算的 Effect。in-flight 副作用先按 prepared-step 合同取得 definite/unknown settlement，然后才在安全边界提交后代终态；Framework 不提供静默放弃副作用的旁路。

## ADR-A2-033：Process tree 是不可拆分的恢复单位

- 状态：已接受；补强 ADR-A2-012、ADR-A2-024 和 ADR-A2-031。
- 决策：单 `Snapshot` 只恢复没有 child relation、未划拨 child budget、没有活动 child wait 的独立 root；一旦形成 Process tree，必须通过包含全部 Process snapshot 与 Engine-owned 活动 direct-child wait 的 `TreeSnapshot` capture/restore。不能把 child snapshot 伪装成新 root，也不能只恢复父级并丢弃其预算和后代。
- 决策：一致 tree capture 使用 Engine 私有、不可观察为 Process 状态的 quiescence barrier。每个 Process 在 prepared Effect 按既有 settlement 合同收口后的安全边界停止新 Step；barrier 期间仍接受 child completion 与 parent termination，其他控制命令延后并在释放后保持到达顺序。tree restore 在任何 goroutine 启动前完成全部 schema、exact Deployment、relation、attenuation、budget、limit 与 wait 校验，并在一个 Engine 临界区原子注册整棵树。
- 原因：逐个 Process 的独立 snapshot 无法单独表达 Engine 拥有的 parent/child identity 和 active child wait，顺序 capture 还可能在 child 已终态而 parent completion 尚未入 mailbox 的间隙永久丢信。将这些事实留在完整 tree wire，并让内部通知在 barrier 中排空，才能得到可重放的一致 cut。
- 限定：`TreeSnapshot` 仍只是 Framework portable value，不拥有 Store、transaction、CAS、revision、lease、retention 或恢复调度政策。prepared dispatcher Effect 的业务幂等仍遵守原 replay contract；tree snapshot 不把稳定 EffectID 夸大为外部 exactly-once。

## ADR-A2-034：冻结首个经过多策略和多消费者证明的公共基线

- 状态：已接受；落实 ADR-A2-030。
- 决策：P3 真实 Interaction、P4 child composition 和第二个独立 consumer 全部通过后，将根 kernel、`interaction` package、Process Snapshot v3 与 TreeSnapshot v1 冻结为 Baseline 1。完整 exported signature、参数名、字段、GoDoc 和 snapshot/tree wire shape 由自动 digest 守卫，稳定合同说明独立记录在 `API_BASELINE.md`。
- 原因：候选窄腰已经同时承载 direct Engine、模型/Tool/HITL/steer Interaction、同 Definition 递归、跨 Strategy child、完整 tree recovery 和三个独立 command consumer；继续不设基线会让无意命名/wire 漂移混入后续 Planning/Workflow 实施，反过来污染已证明的共同层。
- 限定：Baseline 1 不是兼容层或发布承诺。开发阶段仍允许治本 breaking change，但必须先有真实实现/consumer 证据，追加 ADR，并在同一提交更新基线和全部门禁；禁止 alias、dual-read、dual-write 或预留未来字段。尚未实施的 Planning、Workflow、Platform 与应用迁移 API 不在本基线，不能因此提前占位。

## ADR-A2-035：Planning 只执行一步预测并以现实重观测为权威

- 状态：已接受。
- 决策：Planning 的 Goal、Condition、Truth、WorldState、Action、Plan、attempt 和 completion outcome 全部留在 `planning`；共同 Process 只看到 opaque ExecutionState、Effect、Signal 和普通终态。Planner 只对不可变 Problem 搜索，Action 只描述预测语义；执行能力通过 dispatcher binding 或精确 child Deployment binding 单独冻结。
- 决策：Planning Execution 每次规划只执行 Plan 的第一个 Action，随后必须重新观察现实、确认预测 Effects 并重新规划。definite failure 与未确认的 Action 在当前 Process 中排除；首次完整搜索无计划输出 `unreachable`，发生过尝试后无计划或达到 attempt 上限输出 `stuck`，两者都以 Completed Process 的 Planning Output 表达。Planner error、非法 Plan、协议违约和观察失败才进入共同 failure。
- 决策：side-effect-free observation Effect 可以按相同 EffectID 重投；dispatcher Action Effect 不自动重投，未知外部结果必须由调用方使用原 EffectID 和 Planning 自有的 definite Action settlement 显式裁决。Framework 的稳定身份不提供外部业务 exactly-once。
- 决策：child Action 是普通真实 Process，拥有独立 DeploymentRef、snapshot、预算和能力；parent 不把 child Output 当作现实状态，而在 child 终态后重新观察。Planning 的真实消费证明 Strategy 必须核对 child-start 的 DeploymentRef 和 child-wait acknowledgement 的完整 spec，因此根合同增加只读、defensive-copy 的 `ChildWaitOpened.Spec`。
- 原因：预测效果不是真实事实；批量执行旧计划会在环境变化后继续沿过期路径。把执行 capability 放入 Action 或把 no-plan 变成共同状态会重新污染 Kernel。只校验 child WaitID 又无法证明 Engine 确认的是 Strategy 实际声明的等待集合。
- 后果：P5 完成后冻结 Baseline 2，覆盖 root、Interaction、Planning 与 GOAP；HTN、Utility、Reactive、Planner registry/default、Blackboard、Planning telemetry 和 Host persistence 仍明确不实现。

## ADR-A2-036：Workflow 使用 schema 化 DAG 与受限原生节点

- 状态：已接受。
- 决策：Workflow 是原生 Definition/Execution，不编译成 GOAP，也不建立自己的 runtime。Definition 冻结一个以稳定 `NodeID` 标识的有向无环图；每个节点声明精确 input/output schema，Definition 构造时验证入口 schema、每条边的 schema、可达性和所有 terminal output。普通图禁止环，唯一循环能力由有显式正数迭代上限的 `Loop` 节点内部拥有。
- 决策：P6 的唯一节点词汇是 `Transform`、`Gate`、`Switch`、`Call`、`Fork`、`Map`、`Vote` 和 `Loop`。Prompt Chaining 是连续 `Call` 的 Sequence 用法，不建立新节点；Vote 是具名的稳定多数聚合，不公开 Consensus 别名；Fork 同时拥有 Join，不公开 Parallel/ScatterGather；Map 同时拥有 Reduce；Loop 不公开 Repeat/RepeatUntil；Workflow 内不使用 Router，以免和 P8 Deployment routing 混淆。
- 决策：节点之间只传递 immutable、schema-validated JSON value。公开泛型构造器只在边缘把 Go 类型严格转换为 erased value；ExecutionState 保存 raw value、当前 NodeID、branch/item 游标、child key/ProcessID/wait、已结算结果和 loop iteration，不保存 callback、Deployment concrete value、Engine、context、goroutine 或 Host 数据。
- 决策：Transform、Gate、Switch、Join、Map expansion/Reduce、Vote key 和 Loop predicate 是 `Step` 内有界、确定、无 I/O 的纯函数。任何模型、Tool、网络、文件或其他外部行为必须是一个 exact child Deployment；`Call`、Fork 的每个 branch、Map 的每个 item 和 Loop 的每轮 body 都启动普通 child Process。
- 决策：Fork/Map 使用显式正数 concurrency window；Map 另有显式正数 item limit。Execution 分批启动 child、等待本批全部终止，再启动下一批；由此并发上限是正常调度语义，而不是靠 Engine 拒绝超额 child。结果始终按 branch/item 声明顺序聚合，不能按完成顺序排列。P6 的首个明确失败策略是任何 child 非 Completed 或缺少 Output 都使 Workflow 失败；Execution 等待已启动批次收口后按最低 index 归因，不静默遗弃副作用。新的容错/部分聚合策略必须有真实消费者后另行设计，不能预留 enum。
- 决策：Loop 至少执行一次。predicate 满足时产生带最终 value、iteration count 和 `Satisfied=true` 的 typed result；达到上限仍未满足时以 `Satisfied=false` 完成，而不是谎称 predicate 成功或制造共同 Process 状态。调用方可用后续 Gate 明确选择接受、fallback 或失败。
- 决策：Workflow 不产生 Strategy dispatcher Effect，但当前共同 Deployment 合同要求精确 dispatcher binding，因此 `workflow` 提供只拒绝意外 dispatcher Effect 的包内 Dispatcher；它是协议守卫，不拥有第二套执行能力。
- 原因：Workflow 的真实状态是节点游标、分支等待和迭代恢复，不是 Goal search。schema DAG 可以在构造期消除隐式 Blackboard 数据流和无界环；真实 child Process 保持组合闭包、预算、能力、取消与 snapshot 只有一个 owner；明确的纯函数边界避免在 Step 内重新泄漏 I/O。
- 后果：P6 可以逐个增加节点 concrete behavior，而不修改 Kernel。节点集合是当前经过验收的封闭语义，不是允许任意用户节点绕过 Process/Effect 边界的 plugin SPI；未来新增节点必须先证明独立状态与恢复语义。
