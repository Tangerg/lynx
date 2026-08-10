# Agent Framework 架构决策记录

> 状态：持续维护
> 建立日期：2026-08-06
> 最后更新：2026-08-09

本文只记录影响长期结构的架构决策及其理由，不复述目标架构，不记录任务进度。

- 目标设计见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 能力取舍与消费者证据见 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md)。
- 工程实施标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。
- 实施进度见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

改变已接受决策时，不直接改写历史结论。应追加新的 ADR，注明被取代的旧 ADR，并同步更新目标架构。

---

## ADR-A2-001：采用平行模块绿色重构

- 状态：已接受，迁移已完成。
- 决策：重写在隔离模块中独立完成，不在原框架实现上继续根本性修补；消费者完成纵切后，原实现整体删除。
- 原因：隔离架构验证与应用消费迁移，消除历史模型对新执行窄腰的约束。
- 后果：隔离只服务重写过程；当前不允许双模块、交叉依赖或兼容入口回流。

## ADR-A2-002：孵化路径不是永久版本概念

- 状态：已接受，迁移已完成。
- 决策：临时孵化路径已经删除，重写实现以唯一的 `agent` 目录和 module path 取代原实现。
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

## ADR-A2-007：Workflow 使用 Execution

- 状态：已接受。
- 决策：Workflow 不编译成 GOAP Agent。
- 原因：固定控制流与目标搜索是不同问题；managed Workflow 才能准确表达 fork/join/gate/loop 和恢复。

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

- 状态：已接受；由 ADR-A2-040 按真实 P5/P7 类型模型细化。
- 决策：`planning.Action` 是 Planning 的预测搜索操作，Tool 是模型可调用协议；两者不共享含混基类型，也不存在通用自动 adapter。exact child worker 通过 Interaction-owned Delegate 暴露为模型能力。
- 原因：二者的消费者、描述要求和治理边界不同；预测 Preconditions/Effects/Cost 无法推出 Tool 的 JSON I/O 或执行行为。

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

## ADR-A2-016：原实现是历史证据，不是当前规范

- 状态：已接受，迁移已完成。
- 决策：绿色重写期间曾直接参考原实现的代码、测试和文档，但没有建立 import、兼容或共享混合抽象；切换后只保留仓库历史和能力台账作为证据。
- 原因：迁移时复用经过生产验证的知识，同时避免历史结构反向决定新设计；完成后删除双轨真相源。
- 执行要求：当前能力只由公共基线与测试验收，历史实现不得作为兼容依据恢复。

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
- 决策：Supervisor 由 Interaction、Workflow、managed Delegate、typed artifacts、validator 和 child Process 组合，不预建独立 ExecutionState kind 或 package。
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
- 决策：Engine 显式 kill 进入 Killed；父 Process 或 Host context 取消进入 Canceled；Process deadline 或被提升为 Process 终止原因的 Effect deadline 进入 TimedOut；普通 Step error、外部失败、panic 和合同违约进入 Failed；已提交终态 first-terminal-wins。每个终态同时保留稳定 cause，不能仅凭返回 error 推断。
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

## ADR-A2-036：Workflow 使用 schema 化 DAG 与受限节点

- 状态：已接受。
- 决策：Workflow 是 Definition/Execution，不编译成 GOAP，也不建立自己的 runtime。Definition 冻结一个以稳定 `NodeID` 标识的有向无环图；每个节点声明精确 input/output schema，Definition 构造时验证入口 schema、每条边的 schema、可达性和所有 terminal output。普通图禁止环，唯一循环能力由有显式正数迭代上限的 `Loop` 节点内部拥有。
- 决策：P6 的唯一节点词汇是 `Transform`、`Gate`、`Switch`、`Call`、`Fork`、`Map`、`Vote` 和 `Loop`。Prompt Chaining 是连续 `Call` 的 Sequence 用法，不建立新节点；Vote 是具名的稳定多数聚合，不公开 Consensus 别名；Fork 同时拥有 Join，不公开 Parallel/ScatterGather；Map 同时拥有 Reduce；Loop 不公开 Repeat/RepeatUntil；Workflow 内不使用 Router，以免和 P8 Deployment routing 混淆。
- 决策：节点之间只传递 immutable、schema-validated JSON value。公开泛型构造器只在边缘把 Go 类型严格转换为 erased value；ExecutionState 保存 raw value、当前 NodeID、branch/item 游标、child key/ProcessID/wait、已结算结果和 loop iteration，不保存 callback、Deployment concrete value、Engine、context、goroutine 或 Host 数据。
- 决策：Transform、Gate、Switch、Join、Map expansion/Reduce、Vote key 和 Loop predicate 是 `Step` 内有界、确定、无 I/O 的纯函数。任何模型、Tool、网络、文件或其他外部行为必须是一个 exact child Deployment；`Call`、Fork 的每个 branch、Map 的每个 item 和 Loop 的每轮 body 都启动普通 child Process。
- 决策：Fork/Map 使用显式正数固定执行窗口；Map 另有显式正数 item limit。Execution 分批启动 child、等待本批全部终止，再启动下一批；由此活跃上限是正常调度语义，而不是靠 Engine 拒绝超额 child。结果始终按 branch/item 声明顺序聚合，不能按完成顺序排列。P6 的首个明确失败策略是任何 child 非 Completed 或缺少 Output 都使 Workflow 失败；Execution 等待已启动批次收口后按最低 index 归因，不静默遗弃副作用。新的容错/部分聚合策略必须有真实消费者后另行设计，不能预留 enum。
- 决策：Loop 至少执行一次。predicate 满足时产生带最终 value、iteration count 和 `Satisfied=true` 的 typed result；达到上限仍未满足时以 `Satisfied=false` 完成，而不是谎称 predicate 成功或制造共同 Process 状态。调用方可用后续 Gate 明确选择接受、fallback 或失败。
- 决策：Workflow 不产生 Strategy dispatcher Effect，但当前共同 Deployment 合同要求精确 dispatcher binding，因此 `workflow` 提供只拒绝意外 dispatcher Effect 的包内 Dispatcher；它是协议守卫，不拥有第二套执行能力。
- 原因：Workflow 的真实状态是节点游标、分支等待和迭代恢复，不是 Goal search。schema DAG 可以在构造期消除隐式 Blackboard 数据流和无界环；真实 child Process 保持组合闭包、预算、能力、取消与 snapshot 只有一个 owner；明确的纯函数边界避免在 Step 内重新泄漏 I/O。
- 后果：P6 可以逐个增加节点 concrete behavior，而不修改 Kernel。节点集合是当前经过验收的封闭语义，不是允许任意用户节点绕过 Process/Effect 边界的 plugin SPI；未来新增节点必须先证明独立状态与恢复语义。

## ADR-A2-037：暂停 Workflow Strategy 设计并先验证 `flow` 复用边界

- 状态：已被 ADR-A2-038 取代；曾取代 ADR-A2-006、ADR-A2-007、ADR-A2-020、ADR-A2-024 中预先确认 Workflow 为一等 Strategy 的部分，并整体取代 ADR-A2-036。旧结论只保留为历史决策，不再是实施合同。
- 事实：独立仓库 `github.com/Tangerg/flow` 已经提供 typed `Node[I, O]`、派生组合子、runtime-defined `workflow.Graph`/`Spec`、有界并发、稳定顺序、Store/Journal、挂起恢复、流式输出和严格 JSON 解码。仓库自身明确定位为无中心 orchestrator、无后台 scheduler 的 in-process control-flow library；它不依赖 Agent、Host、持久化或产品抽象。
- 决策：P6 的 Workflow API、节点词汇、ExecutionState 和 package 结构立即暂停设计与实施。当前不创建 `agent/workflow`，不把 `flow` 加入 Kernel 依赖，也不以旧 P6 计划为由复制一套 Sequence/Fork/Map/Loop。
- 决策：`flow.Node.Run` 可以执行任意工作，`workflow.Graph` 会启动 goroutine，`Journal` 自行记录 Step 边界；这些行为不能在只允许纯状态归约的 `Execution.Step` 中直接运行。将编译后的 `flow` Step 直接包进 Agent Execution 会形成第二执行循环、第二恢复事实源，并允许副作用绕过 prepared Effect/EffectID，因此明确禁止。
- 决策：已证明安全的复用形态只有两类候选，而且当前均不升级为 Workflow Strategy。Host 可以直接使用 `flow` 组合普通 Go 能力；未来可由独立可选 adapter 把一个完整 `flow.Node` 调度成单个 dispatcher-owned Effect。后一形态只有外层 Effect 粒度的 Process identity、恢复、预算和 replay contract，不能宣称内部节点各自拥有 child Process 语义。
- 决策：是否还需要 managed deterministic orchestration Strategy，必须由真实消费和 disposable adapter spike 证明。只有当需求明确要求图内每个 Agent 分支拥有独立 ProcessID、snapshot、预算、能力衰减、取消和 tree recovery，且 `flow` 的外层封装无法满足时，才能重新设计；届时还要在“扩展 `flow` 的中性编译边界”与“最小 Agent-owned Strategy”之间重新裁决，不能默认选择后者。
- 原因：`flow` 与 Agent Kernel 的职责都已经清晰，重复实现会制造两套控制流语言；强行直接复用运行时又会破坏 Engine 单写者与 Effect 提交语义。先冻结问题而不是冻结方案，才能同时避免重复建设、抽象泄露和为了复用而扭曲两个独立库。

## ADR-A2-038：恢复 managed Workflow，并采用最小有序 Stage 代数

- 状态：已接受；整体取代 ADR-A2-037 的暂停结论，并取代 ADR-A2-036 的任意 DAG 与八类节点方案。ADR-A2-037 对 `flow.Node.Run` 不得进入纯 Step、不得建立双恢复真相的边界继续有效。
- 证据：一个只依赖冻结公共 API 的 disposable consumer 已实现串行 child Call、固定三路 Fork、并发窗口 2、child pause、完整 TreeSnapshot capture/restore 和逆序完成。快照时第三路尚未创建；恢复后前两路逆序完成仍按声明顺序聚合，第二窗口只创建一次。专项测试与 race 各连续运行 20 次通过，随后 spike 已整体删除。
- 决策：`flow` 继续作为普通 Go/AI 同进程控制流的首选库，也是组合代数、定义/运行分离、有界并发和稳定顺序的设计证据；`agent` 不依赖或封装其 runtime。只有当每项工作必须拥有独立 ProcessID、DeploymentRef、snapshot、预算、能力、取消和 tree recovery 时，才使用 managed Workflow Strategy。
- 决策：Workflow Definition 冻结一个有序 Stage 序列，不建立可任意连边的 visual-editor DAG、共享 Store、Journal、Node registry 或第二 scheduler。一个 Stage 消费当前 immutable、schema-validated value 并产生下一个 value；相邻 Stage 的 schema 必须在构造时精确衔接。嵌套或多阶段分支通过调用另一个 Workflow Deployment 获得组合闭包，不在同一 Process 内嵌第二个 Execution。
- 决策：P6 只允许六个具有独立语义的 Stage：`Transform`、`Call`、`Switch`、`Fork`、`Map` 和 `Loop`。定义中的有序 Stage 本身就是 Sequence；Prompt Chaining 是连续 Call；Gate 由 Transform 校验或 Switch 表达；Vote 是 Fork 后的确定性 reducer；evaluator-optimizer 是 Loop 组合。它们不再各占一个节点 kind 或近义入口。
- 决策：Transform、Switch selector、Fork reducer 和 Loop predicate 必须是有界、确定、无 I/O 的纯函数，只能在 `Execution.Step` 内归约一次。Call、Switch 被选分支、Fork 每个 branch、Map 每个 item 和 Loop 每轮 body 都是 exact Deployment 对应的真实 child Process。Workflow 不产生 dispatcher Effect；包内 zero-state Dispatcher 只拒绝协议违约，不成为第二执行能力。
- 决策：Fork 和 Map 必须显式配置正数 `WindowSize`；Map 另有正数 item limit。一个固定执行窗口内的 child 全部终止后才能启动下一窗口，输出按 branch/item 声明顺序归位。公开名称不使用 `Concurrency`，因为首版不是持续补位的滑动并发池。child start failure、非 Completed 终态、缺失 Output 或 schema 不匹配按最低声明索引稳定归因并使 Workflow 失败；首版不预留 fail-fast/partial/fallback enum。
- 决策：Loop 至少调用一次 child，必须有正数最大迭代数。predicate 满足与迭代上限耗尽都以带最终值、迭代数和 `Satisfied` 的策略 Output 完成；耗尽不伪装成 predicate 成功，也不创造共同 Process 状态。
- 决策：ExecutionState 只保存 Stage 索引、当前 raw value、所选 case、窗口/item/iteration 游标、稳定 ChildKey/ProcessID、WaitID 和已按声明位置结算的 raw Output。它不保存函数、Deployment concrete value、Engine、resolver、context、goroutine、Store/Journal 或 Host 数据。Definition/Deployment 的 exact code/config digest 仍是恢复这些纯函数与 child binding 的唯一行为身份。
- 原因：真实 spike 证明 managed orchestration 的差异不是“另一套画图 API”，而是 Engine-owned child lifecycle 和一致 tree recovery。最小有序代数吸收 `flow` 的可组合性与可派生性，同时避免复制其 runtime、重新引入 GOAP 编译、或用八类节点表达本可组合得到的同一语义。
- 后果：P6 先按 Call/Transform 纵切面实现共同状态机，再增加 Switch/Fork、Map 和 Loop；完整恢复、race、fuzz、architecture gate 和真实 command consumer 通过后才冻结 Workflow 公共 API。未来若需要编辑器 DAG，应在 Platform/Host 侧把外部定义编译成已验证的 Workflow Deployment，不能把 Registry、Store 或应用图协议下沉进 Kernel。

## ADR-A2-039：以真实 managed consumer 冻结 Workflow 公共基线

- 状态：已接受；将 ADR-A2-038 的冻结条件落实为 Baseline 3。
- 证据：六类 sealed Stage、strict ExecutionState、完整 tree capture/restore、逆序窗口结算、无重复 child、取消传播、预算/能力衰减、malformed protocol、race、fuzz 和 architecture gate 全部通过。独立 `examples/workflow` command 只消费公开 API，装配 Call→Fork 的 root Workflow 与三个 exact child Workflow Deployment，并以最终四 Process tree 证明没有隐藏生命周期或第二 runtime。
- 决策：Baseline 3 在 Baseline 2 的 root、Interaction、Planning、GOAP、Process Snapshot v3 与 TreeSnapshot v1 之上新增 `workflow` package 的完整 exported API/GoDoc digest。P6 没有改变根公开 API 或 snapshot/tree wire；terminal child wait 修复只闭合既有私有不变量。
- 决策：Fork/Map 的公开窗口参数最终冻结为 `WindowSize`，因为实现是整窗结算后再开下一窗，不是持续补位的并发池。Stage 继续是不可由用户实现的 sealed value；未被真实消费者证明的 `StageKind`、Stage metadata getters 与 `Definition.Stages` 全部收回包内，避免把半套反射 API 误冻成编辑器合同。保留的 Definition、typed constructors、zero-state Dispatcher 与四类 sentinel error 都由真实实现和 consumer 直接使用或审计，不再增加 builder、registry、adapter、alias 或 convenience wrapper。
- 限定：Baseline 3 仍不是发布兼容承诺。开发阶段允许有证据的治本 breaking change，但必须先追加决策、同步基线守卫并重跑所有消费者；P7 组合 helper、P8 Platform 和 P10 应用迁移不能借此向 Workflow 注入 Host、Store、Journal、Graph 或 scheduler 抽象。

## ADR-A2-040：拒绝通用 Action-to-Tool，并以 managed Delegate 组合 worker

- 状态：已接受；细化 ADR-A2-011 与 ADR-A2-020，并纠正目标架构中尚未被代码证明的通用 Action 假设。
- 证据：当前 `planning.Action` 已由 P5/Baseline 3 冻结为纯预测搜索值，只拥有名称、描述、Preconditions、预测 Effects 和 Cost；执行能力单独存在于 dispatcher/child `ActionBinding`。它没有 Tool 所需的参数/result schema，也没有可在 Tool 调用中诚实执行的函数。旧 Go 模块的 Supervisor/AgentTool 与 Embabel `SupervisorAction`/`CurriedActionTool` 都通过 Engine/ProcessContext/Blackboard/thread-local 反向获得执行能力并直接驱动 child 或 Action，正是新架构已移除的第二生命周期入口与共享可变状态。
- 决策：Framework 不新增通用可执行 `Action`、不把 `planning.Action` 改装成 Tool，也不保留 `Action-to-Tool` 名称。普通外部能力直接实现 Tool；Planning 继续只经 `ActionBinding` 执行。模型选择 framework worker 的唯一 P7 桥接命名为 `Delegate`，它是 Interaction-owned 的 immutable composition value，而不是新 Strategy 或通用基类。
- 决策：Delegate 必须显式冻结模型友好的 Tool 名称和描述、exact child Deployment、每次调用 Budget 与 attenuated Capabilities；参数 schema 取自目标 Descriptor Input，成功结果必须由目标 Descriptor Output 验证。Interaction Execution 经 Framework child Effect 启动、等待和恢复 child，Dispatcher、Tool callback 和 adapter 不持有 Engine 或直接创建 Process。首版只支持静态 exact Delegate；动态 catalog/routing 留给 P8。
- 决策：模型一次返回的 ToolCall batch 不人为禁止普通 Tool 与 Delegate 混用。Interaction 按原顺序把连续普通 Tool/Delegate 切成区段，前者走既有 Dispatcher batch，后者批量 `StartChild` 并 `WaitForChildren(all)`，最终结果仍合并为一个原顺序 Tool message。Delegate 参数违约、确定的 start failure 和 child 非 Completed 终态作为 `IsError` ToolResult 回给模型；Framework identity/protocol 错配或 Completed child 的 Output schema 违约使父 Interaction 合同失败。该状态进入 Interaction-owned ExecutionState v2；不兼容读取 v1，也不改变共同 Process Snapshot/TreeSnapshot wire。
- 决策：`NewDispatcher` 必须显式接收其 exact `*interaction.Definition`，把普通 executable Tool 与 Definition 的 Delegate manifest 一次性校验并冻结；同名能力在 Process 创建前拒绝。Dispatcher 可以把 Delegate schema 发给模型，但生产代码的架构守卫禁止其引用 Engine、Process、`StartChild` 或 `WaitForChildren`。
- 决策：Embabel 值得保留的思想是 schema-informed worker manifest、显式 typed goal、每轮根据已结算 artifact 决策、单次选择一个动作和正数 iteration limit。拒绝 mutable Blackboard、按 type-name 猜完成、运行时 currying 改写 Tool schema、一个 Action 内 while-loop/直接执行其他 Action、hard-coded limit 以及把到达 limit 只写日志。typed artifacts 和 completion validator 必须是 Strategy-owned portable state/显式结果，不进入 Kernel。
- 限定：如果未来真实实现证明多个模型 Tool 需要共享一个非 child 的通用可执行 operation，必须以新的名称和独立消费证据申请；不能重新占用 Action 或让一个 adapter 同时拥有 Planning、Tool、Process lifecycle 和 Host policy。

## ADR-A2-041：typed artifact 与完成判断只属于 Interaction 状态

- 状态：已接受；落实 ADR-A2-040 中尚未实现的 typed artifact 与 completion validator 合同。
- 证据：真实 Interaction→Delegate→Workflow child 测试证明，同一个已通过 child Descriptor Output schema 的结构化结果同时需要进入模型 ToolResult 和确定性完成判断；仅扫描文本 WorkingContext 会丢失权威 schema 归属，而复制 Embabel 的 Blackboard、`Any` sink 或 class-name 查询会引入共享可变状态和运行时类型猜测。另一个 direct-result 测试证明完成判断必须覆盖两种 Interaction completion source，而不能绑死模型文本响应。
- 决策：成功 Delegate 结算时，Interaction 以模型调用序号、原 ToolCall 位置/ID、exact Delegate name 和 immutable `agent.Output` 追加一条私有 artifact record。只有 Completed child 且 Output 再次通过该 Delegate 冻结 schema 才能进入；普通 Tool、错误 ToolResult、start failure 和非 Completed child 不进入。公开 validator 视图只暴露稳定顺序、Delegate name、immutable erased Output 与 `DecodeArtifact[T]` 边缘，不暴露 Process identity、产品 storage identity 或 Go runtime type name。
- 决策：`CompletionValidator` 是 Definition-owned 纯函数，输入是 defensive-copy 的当前 Interaction WorkingContext、候选 `interaction.Output` 与 immutable `Artifacts`，输出是显式 `CompletionDecision{Accepted, Feedback}`。WorkingContext 不含当前候选，也不冒充 Host conversation/transcript。接受必须无 feedback；拒绝必须有裁剪且有界的 actionable feedback。拒绝模型最终响应时保留该 assistant message，拒绝 direct-Tool 候选时保留原 assistant ToolCall 和有序 ToolResult，随后统一追加一条 user feedback 再请求模型。无 validator 等价于接受；validator error 是稳定 execution failure，非法 decision 是 contract failure，panic 保留 panic 分类，三者都不降级成模型可见结果。
- 决策：validator 不接收 context、Engine、Process、Dispatcher 或 Host capability，不允许 I/O、时钟、随机数、共享写入或 goroutine；其实现身份必须进入 Deployment ConfigurationDigest。需要外部评价的逻辑必须以 exact managed child Definition 组合。`MaxModelCalls` 继续是唯一局部轮次硬上限；validator 拒绝后已耗尽时沿用 `interaction.limit.model_calls`，不会返回未接受的候选。
- 决策：artifact record 进入 Interaction 私有 ExecutionState v3，strict restore 验证顺序、同轮 ToolCall identity、Delegate 存在性与 Output schema；v2 不兼容读取，不增加 Kernel、Process Snapshot v3 或 TreeSnapshot v1 字段。最终 `interaction.Output` 不自动变成产品 artifact store；需要向下游输出哪些业务结果由组合 Definition 自己声明。
- 原因：artifact 是 Interaction 推进和完成判断所需的 typed evidence，不是共同生命周期事实；completion rule 是 Strategy policy，不是 Engine terminal status。把两者留在 Interaction 可以保住 schema、恢复与模型反馈语义，同时避免重新创造 Blackboard、Supervisor Strategy 或第二执行入口。

## ADR-A2-042：orchestrator-worker 只形成既有 Strategy 的组合合同

- 状态：已接受；以真实消费者落实 ADR-A2-020，并约束 ADR-A2-040/041 的组合边界。
- 证据：独立 `examples/orchestrator_workers` command 只使用 Baseline 3 公开 API，完成 decomposer Interaction → typed task list → 有界 Workflow Map → 三个 exact worker child → synthesizer Interaction；最终 tree 恰好是 root、两个 Interaction child 和三个 worker child。另一个公开 API 行为测试由同一 Interaction 在一个 ToolCall batch 中选择两个 exact Planning Delegates，两个 GOAP child 分别达到可观察 Goal，模型读取有序结果后完成，validator 再从两个 typed `planning.Output` artifacts 独立确认成功；tree 恰好是 Interaction root 和两个 Planning child。
- 决策：不增加 `supervisor` package、Strategy、ExecutionState kind、dispatcher、scheduler、registry 或 helper facade。模型直接选择已知 worker 时使用 exact Delegate；模型先动态产生任务集合而调度必须确定时，任务分解和综合分别是 Interaction child，Workflow Map 只负责显式 item limit、固定 window、exact child lifecycle 与声明顺序聚合。两者可以嵌套，但不形成第二 Process 创建或恢复入口。
- 决策：通用 Task/Result/Worker 接口不进入 Framework。task schema、result schema、拆分 prompt、worker 语义、综合规则和 completion policy 均由组合消费者拥有；Framework 只校验各 Definition Descriptor 和相邻 Workflow Stage schema。若未来 P8 需要动态 catalog 选择，必须另行定义版本、权限与 exact DeploymentRef 决策，不能把本轮 exact binding 偷换成字符串 worker registry。
- 决策：Planning 只有在子任务拥有可机器观察的 WorldState、Goal 与诚实 Action 预测时才可作为 exact worker。父 Interaction/Workflow 只看其 Input/Output 和 Process 终态，不读取 Plan/Action 内部状态，不驱动 Planning Step，也不把 Planning 当作开放式文本转换器。开放式 worker 继续使用 Interaction 或消费方 Definition。
- 决策：本轮没有新公共 API、私有 ExecutionState 或 snapshot/tree wire。组合恢复、窗口与 Delegate artifact 的正确性分别由 P4/P6/P7-02～03 已有合同承担；本轮消费者证明这些合同可以无特权地跨 Strategy 闭合，不复制同一状态机。
- 原因：orchestrator-worker 的独特之处是路径由模型按输入决定，不是独立的生命周期。既有 Interaction 已拥有模型决策，Workflow 已拥有确定调度，Engine 已拥有 child Process；再包一层 Supervisor 只会重复术语、状态和恢复事实源。

## ADR-A2-043：evaluator-optimizer 是显式 typed Loop 组合

- 状态：已接受；落实 ADR-A2-038 对 evaluator-optimizer 的派生组合裁决。
- 证据：旧 Go `RepeatUntilAcceptable` 与 Embabel 同名实现都保留了 input、attempt/feedback history、latest feedback、best-so-far、acceptance criteria 与 iteration cap，这些是有效领域语义；但二者也把循环编译进 GOAP/Action，依赖 mutable Blackboard/runtime type binding，提供隐式默认上限/阈值，并让耗尽条件与真正 acceptable 共用一个停止事实。独立 `flow` 的 Loop 进一步证明 bounded iteration、失败保留上一合法 value 和停止判断属于通用控制流，但其 in-process runtime/Journal 不能成为 Agent tree 的第二恢复源。
- 证据：独立 `examples/evaluator_optimizer` command 只用 Baseline 3 公开 API，把 consumer-owned typed state 交给 Workflow Loop。每轮 exact body Workflow 顺序创建 optimizer 与 evaluator 两个 exact child Process；optimizer 消费最新 feedback，evaluator 追加不可变 attempt 并只在严格更高分时替换 best。三轮成功树恰好包含 root、三个 body、三个 optimizer 和三个 evaluator 共十个 Process；首轮达标树恰好四个 Process。
- 决策：Framework 不增加 `evaluator_optimizer`/`repeat_until_acceptable` package、Stage kind、Feedback/Score/AttemptHistory 公共类型或 builder。该模式唯一实现边界是既有 Workflow `Loop` + exact body/worker Deployments + consumer-owned typed state；optimizer/evaluator 可以换成任意满足 exact Descriptor 的 Definition，Workflow 不读取其业务语义。
- 决策：consumer 必须显式给出正数 `MaxIterations`、评价合同和 acceptance threshold，并将所有影响行为的配置纳入 Deployment ConfigurationDigest。Framework 不假设分数一定是 `[0,1]`、越大越好或某个默认阈值；示例的归一化分数只是该消费者合同。
- 决策：每轮只把 evaluator 已提交的 feedback 提供给下一轮 optimizer；history 保持执行顺序；best 只在严格更优时更新，因此同分稳定保留最早 attempt。达到标准映射为 `LoopResult.Satisfied=true`；上限耗尽映射为 `Satisfied=false` 的正常策略完成，最终仍返回 best，不把“停止”伪装成“已接受”。worker failure 沿用 Workflow child failure，不能拿半轮 candidate 覆盖上一个合法 state。
- 决策：pure Transform、Loop predicate 和 Interaction completion validator 都不得执行外部 evaluator I/O。需要模型、Tool、网络或其他外部判断时，evaluator 必须装配为 exact managed child Process；是否值得增加该循环必须由清晰评价标准和效果评测证明，而不是因为模式目录存在。
- 后果：本轮只新增 consumer/test/documentation，没有公共 API、Strategy state 或 snapshot/tree wire 变化；P7-07 仍需用直接实现对比该十 Process 组合的成本并删除没有实益的抽象。

## ADR-A2-044：Anthropic 模式是组合验收词汇而非类型目录

- 状态：已接受；完成 P7 模式覆盖矩阵，但不改变 ADR-A2-007 的最小模式代数和 ADR-A2-038 的 Workflow Stage 代数。
- 证据：P3～P7 已有 `direct_vs_managed`、`autonomous`、`composition`、`workflow`、`orchestrator_workers`、`evaluator_optimizer` 六个独立 command consumer，分别证明 direct/managed augmented model、model-owned Tool loop、heterogeneous child composition、managed Call/Fork、model-directed worker 与 evaluator feedback Loop。Workflow 自身已有 Switch/Fork/Map/Loop 的选择、窗口、顺序、恢复和失败合同，但 prompt chaining、routing、parallel sectioning/voting 尚缺一个统一的公开 API command 证据。
- 证据：新增 `examples/workflow_patterns`，用一个 root Workflow 顺序 Call normalize/summarize 两个 exact child，Switch urgent/standard exact route，Fork facts/risks exact section，再把两份 typed section evidence 交给四个 exact voter。urgent 与 standard 行为测试都证明未选 case 不创建；section 保持声明顺序；voter 以 window=2 执行，最终 2–2 时 consumer-owned reducer 稳定选择最早声明的 approve。每次 tree 恰好 root + 2 chain + 1 route + 2 section + 4 voter 共十个 Process。
- 决策：Augmented LLM、Prompt Chaining、Routing、Parallel Sectioning、Parallel Voting、Orchestrator-workers、Evaluator-optimizer、Autonomous Agent 和 Pattern Composition 只作为架构映射与验收词汇。Framework 不按文章标题增加 `chain`、`router`、`vote`、`supervisor`、`evaluator_optimizer` 或 `autonomous` package/Strategy/kind；它们必须由 `chatclient`、Interaction、Planning、Workflow、Tool 和 child Process 的既有职责组合。
- 决策：retrieval 是普通 Tool/provider contract，不建立 Framework Retrieval Strategy；Interaction WorkingContext 是单 Execution 的模型上下文，不冒充产品长期 memory；跨 Process/长期 memory 继续由明确的 consumer/Host owner 管理。Pattern 表不能成为把产品知识库、conversation store 或 memory service 下沉 Kernel 的入口。
- 决策：managed pattern 示例使用 deterministic exact worker 来隔离并证明 topology、typed value、Process identity 和稳定聚合；将 worker 换成模型/Tool 实现时必须保持 exact Descriptor，并通过 consumer-owned adapter 显式转换。模式覆盖不要求为每种 topology 复制一套模型调用代码。
- 决策：公开示例可以一项覆盖多个模式，只要每项都有独立行为断言。P7-06 的七个 command consumers 与生产 contract tests 已覆盖完整矩阵；本轮不新增生产 API/state/wire。P7-07 仍必须比较 direct/`flow`/managed 成本，删除没有可测收益的复杂组合或抽象。
- 原因：文章的价值是帮助选择控制流，不是提供类名清单。把每个图都升格为 Framework 概念会制造术语重复、恢复状态重复和模型/API 混淆，反而降低正确性。

## ADR-A2-045：P7 以 direct-first 复杂度阶梯收口且不新增 facade

- 状态：已接受；完成 P7-07 最终简化审计。
- 证据：`go mod why github.com/Tangerg/flow` 明确显示 `agent` 不依赖 `flow`；standalone package DAG 只有根 Kernel、Interaction、Planning/GOAP、Workflow 与独立 examples，根不反向依赖任何 Strategy。生产源码没有 Supervisor、PromptChain、Router、Vote、EvaluatorOptimizer、Team 或模式目录；P7 新增的 Delegate/artifact/validator 与 Workflow 六个 Stage 均有真实 command 或 contract consumer。无空目录、原框架实现/应用 import、共享 Store/Blackboard 或第二 runtime。
- 证据：真实 command tree 给出复杂度下限：direct `chatclient` 为零 Process，managed Interaction 为一个；最小 managed Workflow 为四个；orchestrator-worker 为六个；三轮 evaluator-optimizer 和 workflow-patterns 各十个。`flow` 的普通 Node 在同进程运行，不为每节点提供 Agent Process identity；它与 managed tree 不是两个等价 API 的重复实现。
- 决策：选择顺序固定为 direct `chatclient` → 普通 Go/`flow` → 单 Process Interaction → Workflow/exact child tree → P8 Platform。只有下一层新增的独立暂停/恢复、Signal/Effect、预算/能力、取消、Process tree 或版本治理价值被需求证明时才升级；不能因为“agent 应该高级”或模式名称存在而跳级。
- 决策：保留七个独立 command consumer。`direct_vs_managed` 是零/一 Process 对照；`autonomous` 证明模型拥有 Tool/停止决策；`composition` 证明自定义 Definition 的嵌入与异构 child；`workflow` 是最小 managed Call/Fork；其余三个分别承担动态 worker、评价循环和确定模式矩阵。复杂示例不能替代最小示例，抽取共享 example helper 又会掩盖每个 command 对公开 API 的独立编译证据，因此不做假 DRY。
- 决策：保留 `Artifact`/`Artifacts`/`CompletionCandidate` 的最小只读面。`WorkingContext`、candidate Output、ordered artifacts 和 typed decode 分别由完成反馈、两种 completion source、不可变/顺序和 schema 归属合同使用；`Len` 避免只为计数复制集合。保留 Transform/Call/Switch/Fork/Map/Loop，因为它们分别拥有纯变换、单 child、单选 child、静态 fan-out、动态 fan-out、可恢复迭代的独立状态语义；Sequence/Gate/Vote/PromptChain 等继续只由组合派生。
- 决策：本轮没有为了满足“删除”动作而误删已证明能力，也没有增加 helper/facade。此前 P6/P7 已实际删除 StageKind/metadata getters、Supervisor Strategy、通用 Action-to-Tool、Blackboard、pattern-specific kinds 等无收益抽象；当前审计的零新增删除是这些前置清洗成功的事实，不是跳过审查。
- 后果：P7 完成。P8 只能增加经过真实 Engine consumer 证明的 catalog/routing/governance 能力，不能重新引入本阶段拒绝的 facade、全局 registry 或第二生命周期。

## ADR-A2-046：精确 Deployment 解析是无上下文的本地绑定查询

- 状态：已接受；完成 P8-01。
- 证据：Engine 只有两个解析消费点：跨 Deployment child start 与完整 tree restore。二者收到的都是已经冻结在 ChildSpec 或 Snapshot 中的 exact DeploymentRef；same-reference child 直接复用当前 Deployment，restore 也按 distinct reference 缓存。此前 `Resolve(context.Context, DeploymentRef)` 的实现全部忽略 context，Engine 又主动剥离取消；该参数既不控制真实生命周期，也会错误暗示 resolver 可以按 tenant/context value 改变绑定或进行远程 I/O，使相同 snapshot 的恢复行为不再确定。
- 决策：`DeploymentResolver` 唯一方法治本改为 `Resolve(DeploymentRef) (Deployment, error)`。实现必须并发安全、同步有界、确定、无远程 I/O，且不得重入任何 Process。路由、调用方/租户选择、权限判断和远程发布发现必须在更高层完成并产出 exact DeploymentRef；它们不属于 resolver。
- 决策：Engine 始终复验返回 Deployment 的有效性与 exact reference。resolver panic 被隔离为确定的解析失败；same-reference child 不调用 resolver；tree restore 对每个 distinct exact reference 至多调用一次，并维持整树注册 all-or-nothing。resolver 不创建 Process、不持有 Engine，也不取得生命周期所有权。
- 原因：DeploymentRef 是恢复和 child composition 的行为身份，不是待补充条件的查询。将 context 留在窄腰只会制造第二个隐式路由入口，并让确定性依赖不可见调用上下文；取消也无法使一个违反“有界本地查询”合同的实现变得正确。
- 后果：P8 Platform catalog 必须直接实现这一最小接口；可取消的远程同步、动态路由和 Host policy 只能发生在 exact reference 被选定或部署快照被构造之前。根公开 API/GoDoc 基线显式修订，Process Snapshot v3 与 TreeSnapshot v1 wire 不变。

## ADR-A2-047：Await 在线性化终态的直接父子 bookkeeping 后返回

- 状态：已接受；P8-01 完整 race 门禁发现并关闭既有时间窗口。
- 证据：Process loop 先把 immutable Result 写入 controller 并关闭 `done`，随后才调用 Engine 的 `processFinished` 注销父 wait、通知等待该 child 的父级，并向所有活动 direct child 同步投递 parent termination。旧 `Await` 只等待 `done`，因此 caller 在拿到父级 terminal Result 后可以立即释放 child 的 in-flight Effect；child 偶发先正常完成，越过尚未投递的 parent cancellation。race 门禁真实复现该违约。
- 决策：`Process.Await` 等待该 controller 的 tree-settlement barrier：terminal Result 已提交，并且 `processFinished` 对这次终止直接触发的 wait 注销、父完成通知与 direct-child termination 投递已经完成。它不等待所有后代真正达到终态，也不把 Process Result 与整棵树完成混成一个概念；`Status` 仍可在 barrier 前观察到已提交终态。
- 限定：parent termination 只投递给扫描时仍 active 的 child；已经先行完成的 child 保留 Completion。未显式 WaitForChildren 的父子 run loop 不承诺谁先完成，relation 测试只能接受“先完成”或“仍 active 后 parent-cancel”两个合法结果；需要 child 结果的 Strategy 必须建立 wait，不得把 scheduler timing 当协议。
- 原因：Await 是 caller 继续操作的同步边界。若它早于 Engine 自身的终止 bookkeeping 返回，公开终态就不是可组合的线性化事实，父级取消传播会取决于 caller 在纳秒级窗口中的动作。
- 后果：父终止后 caller 可以安全处理 direct children，不会抢在控制意图投递前推动其下一安全边界。改动只收紧 Await 的返回时点与 GoDoc，不改变状态、Result、snapshot/tree wire 或 child 的异步终止语义。

## ADR-A2-048：Catalog 是 exact binding 的不可变快照

- 状态：已接受；完成 P8-02。
- 证据：旧模块的 deploymentCatalog 用一个可变 RWMutex 聚合 active route、全部历史 Deployment、Process retain count、forget policy 与 exact lookup，并由 Engine 直接暴露 Deploy/Replace/Undeploy。它证明历史 exact definition、稳定枚举和并发读取是真需求，也证明把变更命令、生命周期引用和解析放进同一个目录会让 Engine 与治理互相泄漏。新 Engine 已由 P8-01 证明只消费 context-free exact lookup。
- 决策：新增上层 `platform.Catalog`，仅表示 exact Deployment bindings 的不可变内存快照并直接实现 `DeploymentResolver`。零值为空；构造一次性验证全部 Deployment；重复 exact DeploymentRef 使构造整体失败，不覆盖、不幂等吞掉。不同 exact reference 即使 name/version 相同也可共存，以保留 replacement/historical definition。
- 决策：Catalog 只提供 exact `Resolve` 和 ownership-isolated `Deployments` 枚举。解析不按 name/version fallback；枚举按 Definition name、语义版本、完整 digest 稳定排序。Catalog 没有 mutex、active 标记、Deploy/Replace/Undeploy、Process retain count、remote discovery、Store 或 global singleton；P8-03/04 必须在 Catalog 之上分别建立显式变更与路由，而不能反向污染 exact snapshot。
- 决策：`DeploymentRef.String` 返回 `name@version+complete-digest` 的稳定诊断文本，无效值返回明确 placeholder；它不是 JSON wire encoding。Catalog 的 not-found/duplicate 诊断真实消费该表示，避免仅打印 name 而掩盖 exact identity。
- 原因：运行与恢复需要的是不可变 exact binding 集合，部署变更和路由需要的是其上层原子状态迁移。先把两者拆开，读路径可天然并发安全，Engine 仍只依赖最小接口，后续治理也不会重新创造 Engine-owned registry。
- 后果：Catalog 保存包含 Go behavior binding 的 Deployment，不可序列化，也不承担 durable publication。Host/adapter 可从自己的发布事实构造 Catalog。根 API/GoDoc 因 `DeploymentRef.String` 显式修订；Platform API 在 P8 完整真实 consumer 验证前不提前冻结，snapshot/tree wire 不变。

## ADR-A2-049：部署变化按 name/version 槽位显式推进并保留 exact 历史

- 状态：已接受；完成 P8-03。
- 证据：旧模块只有 `name -> active DeploymentRef`，因此发布新 SemVer 会强制替换旧版本，路由无法同时选择多个版本；同时 Undeploy 只收 name，陈旧调用可以下线已被并发 Replace 的新 binding。它正确保留了 replaced/undeployed exact history，但把状态直接放在 Engine mutable catalog 中。P8-02 已提供与 active route 解耦的不可变 exact Catalog。
- 决策：`platform.Platform` 是部署聚合 owner，初始 Config all-or-nothing，并以本地临界区一次性替换完整 immutable deployment state；不同 Platform 实例绝不共享 package-global 状态。它直接实现 exact DeploymentResolver，但不创建、注册或运行 Process。
- 决策：active slot 唯一键是 `(Definition name, canonical semantic version)`。不同版本可同时 active；`Deploy` 在空槽激活，重复同一 exact ref 保持不变，同槽不同 ref 返回带 Active/Requested 的 `DeploymentConflictError`。`Replace` 只改变 candidate 自身已存在的同 name/version 槽位；新版本必须另行 Deploy。所有被替换 exact refs 继续留在 Catalog，因此已有 snapshot 恢复不跟随 active route。
- 决策：`Undeploy` 接收并校验当前 exact DeploymentRef，而不是裸 name/version。槽位为空返回 `ErrDeploymentNotActive`；引用陈旧则返回 conflict，不能误下线 replacement。成功只删除 active 选择，exact binding 继续可解析。首版不提供 Forget/Remove 历史 binding；Host 在证明外部 snapshot retention 后可重建新的 Platform，Framework 不猜 durable reachability。
- 决策：Deploy/Replace/Undeploy 是同步、有界、无 context、无 I/O 的本地领域操作。临界区只保证一个 Platform 实例内状态快照的一致发布，不对 Host database 声明 transaction/CAS，不引入 idempotency key。生命周期 Event 与 OTel 留给 P8-06 的外部 decorator，不能反向让 Catalog 或 Engine 拥有 observation backend。
- 后果：Platform API 仍是 P8 候选面，P8-04 路由只能读取一次 `ActiveDeployments` 快照并返回 exact ref；不能绕过版本槽位、扫描 historical Catalog 或把 active 选择塞进 Engine。根 API 和 snapshot/tree wire 本轮不变。

## ADR-A2-050：Definition 选择只使用 active Candidate snapshot 与一个 Selector 合同

- 状态：已接受；完成 P8-04。
- 证据：旧 routing package 同时公开 Router、Ranker、Candidate、Choice、Confidence、Rationale、agent filter 与 goal filter，并从 Engine mutable catalog 动态读取；这些类型把一种评分实现误写成 Framework 路由本体。新 Descriptor 没有旧 Goal catalog，P7 也已裁决模型 worker selection 可以由 Interaction Delegate 完成。P8 的真实缺口只有：发现 active Definition 静态合同、让 caller policy 选择、验证 exact identity，并在并发部署变化下保持同一次选择一致。
- 决策：唯一词汇为 `DeploymentCandidate`、`DeploymentSelector` 与 `SelectDeployment`，不再增加 Router/Ranker/Choice/Confidence 或另一套 registry。Candidate 只暴露 exact DeploymentRef 与 Descriptor，不暴露 Deployment behavior、Dispatcher、Engine 或 Process；`DeploymentCandidates` 返回 stable、ownership-isolated active snapshot，replaced/undeployed historical bindings 明确排除。
- 决策：Selector 接收 caller context 和一次候选 slice，返回 exact DeploymentRef。request text、typed input、model、threshold、rationale、filter 与业务授权全部由 selector implementation/adapter 拥有，Framework 不发明通用 routing payload 或固定 `[0,1]` score。Selector 可以外部 I/O 且必须 honor context；共享实现必须并发安全。nil/typed nil、panic、invalid ref 与 unoffered ref 分别被稳定拒绝，普通 selector error 保留 cause。
- 决策：SelectDeployment 在调用 selector 前冻结 active Deployment values 与候选 membership，在 Platform lock 外执行 selector，完成后只从该 captured set 返回 exact Deployment。并发 Replace/Undeploy 后旧 ref 仍由 Catalog 保留，所以选择不会 TOCTOU 跟随新 active route，也不会因为 route 变化变成不可执行。Selector 不能返回调用期间新部署但未被 offered 的 ref。
- 原因：选择政策是可替换扩展，候选快照与 exact identity validation 才是 Platform invariant。用一个最小 selector 合同可以承载代码、模型或其他实现，同时避免把某种 scoring vocabulary、产品请求或执行入口固化进 Framework。
- 后果：Engine 继续只消费 exact resolver，不依赖 Platform 或 selector。P8-05 guard 只能约束候选/启动边界，不能把权限字段塞进 Candidate 或让 selector 创建 Process。Platform API 仍待 P8-07 真实 command consumer 后冻结；根/Strategy API 与共同 wire 不变。

## ADR-A2-051：ProcessAdmitter 是根与子启动共用的唯一准入合同

- 状态：已接受；完成 P8-05。
- 证据：P4 已经由 Engine 单独实现并持久化每 Process Budget、父子永久划拨、TreeLimits、root 最大 CapabilitySet、child subset attenuation 与 Dispatcher Effect capability enforcement；这些是正确性不变量，不能在 P8 再造一份可变 policy state。旧模块的 `StopPolicy` 在每个 tick 读取胖 ProcessView，`ChildAdmitter` 只覆盖 child 且通过通用 Extension registry 动态分派；前者混淆资源限制、Strategy stop rule 与 Host control，后者让 root/child 使用不同准入术语和入口。
- 决策：公共词汇只增加 `ProcessAdmission` 与 `ProcessAdmitter`，不同时增加 Policy、Guard、Middleware 或 Extension marker。ProcessAdmission 是 Engine-only constructed immutable value，只暴露 ProcessRelation、exact DeploymentRef、frozen Descriptor、Budget 和 CapabilitySet；所有字段私有，不包含 Input、Execution、Dispatcher、Engine、用户/租户、订阅、价格、Store、transaction 或幂等键。一个 EngineConfig 只接收一个 admitter，因此每个 prospective Process 只有一个准入裁决者。
- 决策：同一个 admitter 消费 root Start 与 Framework StartChild。root 在 Definition.Start 和注册前请求准入；child 在 exact resolution、输入合同验证、预算可用性与 capability subset 检查后、Definition.Start 和注册前请求准入。拒绝 root 返回 `ErrProcessAdmissionRejected` 并保留 ordinary cause；拒绝 child 形成 External `engine.child.admission.rejected` settlement，释放预留预算且不发布 Process。panic 与 typed nil 被稳定隔离，admitter 不能批准 capability escalation 或扩充 Budget/TreeLimits。
- 决策：admitter 必须同步、有界、无外部 I/O、不得重入 Process、支持并发调用并保持 decision-only，因此其方法不接收误导性的 context。prepared child Step 在 crash restore 后可能以同一稳定 Process identity 再次请求准入，所以实现不能把调用本身当成一次 durable charge 或产生不可逆副作用。root 的远端审批由 Host 在 Start 前完成；child 的远端审批必须由 Strategy 先声明 Dispatcher Effect，再根据 settlement 声明 StartChild，不能藏进 Framework Effect 的本地准入路径。已经 capture 的 root/tree restore 不重复 admission：snapshot 中的 exact binding、预算、能力和关系就是已准入事实；若 Host 当前政策禁止恢复，应在调用 restore 前拒绝，或用明确控制终止，而不是让 Kernel 恢复取决于隐藏 live policy。
- 决策：不重建通用 StopPolicy。Framework 资源耗尽由 Limits/Budget 直接终止，外部撤销由 Process Cancel/Kill，Planning no-plan 与 Interaction completion 各归 Strategy policy。ProcessAdmitter 只拥有 start boundary，不在每 Step 重复判断，不观察 Event，也不取得生命周期所有权。Platform selection policy 继续属于 Selector；P8-07 只把 Platform resolver 与同一个 EngineConfig 装配起来，不建立第二 runtime。
- 原因：准入是正确性路径，观察不是；但准入扩展也不能成为第二资源 owner。一个中性、只读、根子同构的消费侧接口提供必要可替换点，同时保住 Engine 的单写者资源/权限不变量和 Framework/Host 边界。
- 后果：根公共 API/GoDoc 显式修订，四个 Strategy API 与 Snapshot v3/TreeSnapshot v1 wire 不变。P8-06 只补 Event/OTel decorator，不得把 observation failure 或 generic extension registry 混入 admission。

## ADR-A2-052：Framework Event 自足归因，OTel 只存在于外部 adapter

- 状态：已接受；完成 P8-06。
- 证据：P2 Event 原 envelope 只有 ProcessID，child observer 无法知道 exact Deployment、parent/root/depth；只有 `step.prepared/committed`，无法区分 Execution.Step 本身的调用时间；Dispatcher Effect 有 started/finished，Framework Effect 没有，导致同一个 Effect 窄腰产生两套观察语义。EventListener/DeltaListener 的 error 永远被丢弃，公开返回值错误暗示 observer 可以影响执行。原框架实现把 OTel tracer/meter 直接 import 到 runtime、planner 与 event multicast，证明行为可观测有价值，也证明 instrumentation backend 进入 Kernel 会污染 DAG 并散落 schema。
- 决策：Event envelope 新增 exact DeploymentRef 与 immutable ProcessRelation，且 relation.ProcessID 必须等于 envelope ProcessID；JSON strict codec 同步携带 deployment/relation。根 package 提供唯一 Framework Event 名称常量；发布点不再写裸字符串。Process-local sequence、可选 StepSequence/EffectID、phase、OccurredAt 与 bounded immutable payload 保持不变，Event/Delta observation wire 进入独立 digest gate。
- 决策：Execution.Step 调用前后发布 attempt `EventStepStarted/Finished`，finished payload 是 `status` + 同 owner 测量的非负 `duration_ms`；prepared 与 committed 继续分别表示候选状态和权威提交。实际执行的 Framework/Dispatcher Effect 统一发布 attempt started/finished，payload 固定带 target，finished 再带 settlement status/duration。恢复时因 ReplayPolicy 被跳过的 Dispatcher Effect 没有发生新 attempt，因此只形成 unknown settlement，不伪造 started/finished。
- 决策：EventListener 与 DeltaListener 都改为无 error 返回；新增对应 Func adapter。panic 继续隔离，Event listener 必须同步有界且不得重入 Process，Delta listener 仍由有界异步队列隔离并以 dropped Event/Usage 暴露丢失。观察没有 veto、retry、ack 或第二状态提交入口。
- 决策：新增独立 `agent/otel.Observer`，只实现 EventListener，直接依赖官方 OTel trace/metric API。Config 可注入 TracerProvider/MeterProvider，nil 使用官方 global provider，typed nil 明确失败。Observer 以 Event timestamp 建立 Process/Step/Effect span，以 relation 在 parent span 已可见时建立 child parentage，并始终记录 exact root/parent/depth/deployment attributes；只记录 bounded status/target/count，不导出 raw Event payload、Input/Output 或 Host identity。metric 固定为 Process starts/exits、Step/Effect duration 与 Delta drops。
- 决策：Kernel production source 由 architecture gate 禁止 `go.opentelemetry.io/otel`；OTel adapter production source 禁止 SDK、Strategy、原框架实现与 Host imports。SDK trace/metric 只用于真实 provider 测试，证明一个两 Step/一 Dispatcher Effect Process 产生一个 Process span、两个 Step span、一个 Effect span及 1/1/2/1 对应 metric observations。adapter panic 仍由 Engine listener boundary 隔离，不影响 Result。
- 原因：Event 必须先成为自足、准确、中性的事实，telemetry 才能是可替换投影。让 Kernel 直接操作 span 或让 OTel adapter解析应用对象都会反转依赖；让 observer 返回 error 又会制造虚假的控制权。
- 后果：根与新 OTel package API/GoDoc、Event/Delta observation wire 显式冻结；四个 Strategy API 与 Process Snapshot v3/TreeSnapshot v1 wire 不变。P8-07 只验证 Platform/embedded 装配共享同一个 Engine 语义，不再增加第二观察总线。

## ADR-A2-053：Platform 只治理 Deployment，完整形态仍显式装配同一个 Engine

- 状态：已接受；完成 P8-07，P8 完成。
- 证据：公开 `embedded_vs_platform` command 用同一 exact Workflow root、worker、Input、ProcessAdmitter 与 EventListener 运行两次：嵌入式路径直接提交 root 并使用 caller-owned exact resolver；完整路径先从 active DeploymentCandidate snapshot 选择 root，再把 `platform.Platform` 作为同一 EngineConfig 的 DeploymentResolver。两者产生相同 Output、Completed、Usage、root/child exact tree、两次 admission 和稳定 Process/Step/Effect observation 投影。
- 决策：Platform 不增加 Start、Run、Restore、Process handle、Engine facade、scheduler 或 observation bus。选择只产生 captured exact root Deployment，解析只服务 child/restore；生命周期始终由根 Engine 拥有。ProcessAdmitter 与 EventListener 继续直接装配进 EngineConfig，Platform 不代理、不复制也不扩充它们。
- 决策：冻结前删除无行为价值的单字段 `Config`，`New` 直接接收 `deployments ...Deployment`；Platform 零值成为可用空 aggregate，不再维护 initialized 分支。删除公开 executable `ActiveDeployments` 副视图；发现只公开 non-executable `DeploymentCandidates`，执行 binding 只能来自 validated SelectDeployment，精确历史来自 Catalog/Resolve。只表示 nil/typed-nil 的 sentinel 收紧为 `ErrNilPlatform`/`ErrNilDeploymentSelector`，不再用过宽的 Invalid。没有 alias、旧构造器或双枚举兼容层。
- 决策：跨两次独立运行只比较稳定语义，不伪造 deterministic scheduling。child 在 wait 注册前完成时父 Process 可继续 running；注册后完成时会出现 Waiting/Signal。`signal.accepted`、中间 running/waiting、ProcessID、wall-clock 和全局交错不是 Platform 等价合同；每 Process 的事实仍按自身 sequence 有序，最终 Result/tree/Usage 与实际 Step/Effect lifecycle 必须一致。
- 原因：Platform 的价值是多 Deployment catalog、版本、selection 和治理，不是第二 runtime。真实 consumer 同时证明最小嵌入与完整装配只相差启动前的选择和 exact resolver 来源，因而没有理由新增 facade；冻结前删除冗余候选面也避免把可执行对象泄露给 discovery。
- 后果：`platform` 公开 API/GoDoc 纳入 Baseline 3；根、Interaction、Planning、GOAP、Workflow、OTel API 与 Process Snapshot v3、TreeSnapshot v1、Event/Delta wire 不变。P8 7/7 完成，下一阶段只做 P9 独立完整性验收。

## ADR-A2-054：P9 以 owner-qualified 词汇一次性替换宽泛名称

- 状态：已接受；完成 P9-01。
- 证据：P1–P8 的真实消费者已经证明领域边界稳定，但独立 `go doc -all`、wire shape 和私有实现审计仍发现同一概念存在不同精度：完整 `Deployment` 与仅含 exact identity 的配置都叫 `Deployment`；Deployment/Candidate 对 exact identity 使用宽泛 `Reference`；Effect batch index 只叫 `Index`；Event、Delta、Signal mailbox 与 prepared Step 的多种 sequence/cursor 缺 owner；Transition consumption 在 API 与 wire 上没有明确指向 Signal；若干公开接口参数匿名、公开字段缺独立 GoDoc，error cause 仍有 `%v` 断链。私有 loop/controller 也有 `runtime`、`id`、`lastStable`、`control`、`output` 等无法单独说明所有权或提交状态的宽词。
- 决策：词汇按 owner-qualified 语义一次性收敛，不保留近义入口。完整行为绑定始终叫 `Deployment`；exact value identity 始终叫 `DeploymentRef`，因此公开方法为 `Deployment.DeploymentRef`/`DeploymentCandidate.DeploymentRef`，Planning child 配置字段为 `DeploymentRef`。Effect request 使用 `StepSequence`/`BatchIndex`；Event 使用 `ProcessSequence`/`StepSequence`，Delta 使用 `EffectSequence`；mailbox 使用 `ArrivalSequence`/`SignalCursor`；Transition 使用 `ConsumedSignals`。私有 Kernel 统一为 `processLoop`、`processID`、`lastStableState`、`pendingControl`、`finalOutput` 和 `terminalSnapshot`。
- 决策：公开 callable 的每个参数必须有语义名称；所有公开声明和公开 struct field 必须以 exact identifier 开头的 GoDoc 说明 owner、单位或零值；AST 门禁自动阻止匿名参数、漂移字段注释和 `%v` 格式化 error cause。sentinel 同样使用准确结果词汇，例如 `ErrEngineQuiescenceUnavailable`、`ErrProcessAlreadyExists`、`ErrNoDeploymentCandidates`、`ErrExpansionLimitReached`，不以 Busy/Exists/Limit 等宽词掩盖条件。
- 决策：wire 名称同步一次性修正，不双写/双读旧 tag。Process Snapshot schema 从 v3 升为 v4，TreeSnapshot 从 v1 升为 v2，child 与 Framework Effect protocol 从 v1 升为 v2；Planning `Attempt.ActionName` 进入 output/state wire，其 ExecutionState 从 v1 升为 v2。Event/Delta 是严格 observation wire，直接使用新 owner-qualified tag 并由独立 digest 固化。旧 snapshot/protocol/state 明确拒绝，不增加 alias、compat codec 或迁移 helper。
- 原因：名称是 Framework 的可执行合同，而不是装饰。相同词必须表达相同抽象强度，不同 owner 的序号/游标不能共享一个模糊名称；否则使用者和模型会把 exact ref 当完整 binding、把 arrival sequence 当 committed cursor，或忽略 error cause。绿色重写尚无稳定发布兼容负担，此时一次性治本比长期保留双术语更便宜也更安全。
- 后果：七个公共 package、Process/tree/protocol 与 observation wire 显式修订为 Baseline 4。P9-02 仍需独立验证 baseline 守卫覆盖完整且不存在未纳入的持久/观察 wire；本 ADR 不改变 Framework/Host 边界、Process 状态机、Strategy 行为或 Workflow 的 `flow` 独立边界。

## ADR-A2-055：每个 schema owner 独立冻结自己的完整 wire

- 状态：已接受；完成 P9-02。
- 证据：Baseline 4 的根 snapshot digest 能冻结 `ExecutionState` envelope，却只看到 opaque payload 的类型是 `json.RawMessage`；Interaction/Planning/Workflow 修改私有恢复字段或 Effect/Signal envelope 时，根 baseline 不会失败。Framework Event/Delta 外层同样已冻结，但 Process/Step/Effect/Signal/Delta-drop payload 仍由散落匿名 struct 生成，字段变化也不会进入 observation digest。仅有 strict decoder 和当前行为测试不足以发现这类未经审计的协议漂移。
- 决策：wire baseline 按所有权分层。Kernel 只冻结自己拥有的全部 production `*Wire`、snapshot/tree、Framework child/effect protocol、Event/Delta envelope 与命名 Framework Event payload；Interaction、Planning、Workflow 各自在包内冻结私有 ExecutionState、Effect/Signal/Delta protocol 和所有支撑 wire。Kernel 不 import Strategy package、不反射递归 Strategy payload，也不把具体 phase/cursor 提升成共同类型。
- 决策：baseline 必须同时防止“已覆盖 shape 漂移”和“新增 shape 未纳入”。根 AST 守卫要求新增 production `*Wire`/`*EventPayload` 进入 Kernel baseline；三个 Strategy 的 AST 守卫要求每个新增 private JSON struct 进入对应 owner baseline。digest 变化仍必须结合 strict codec、prior-version rejection、round-trip、restore、fuzz 与 consumer behavior 审计，不能以更新 hash 代替设计。
- 决策：审计中发现的宽泛持久字段一次性改为 owner-qualified 语义：Process event sequence、child-wait parent ProcessID、mailbox WaitKey/WaitID；Interaction WorkingContext/model/Tool/delegate/artifact 游标与记录；Planning WorldState/excluded/current Action/confirmation；Workflow Stage/current value/case/fan-out window。Framework Event payload 也明确区分 process、step、effect settlement 与 dropped Delta 事实。旧 tag 不双读，旧 state/snapshot 版本不兼容恢复。
- 后果：形成 Baseline 5。Process Snapshot v5、TreeSnapshot v3、Interaction ExecutionState/protocol v4/v2、Planning ExecutionState v3、Workflow ExecutionState v2；child/framework-effect protocol 保持 v2，Planning protocol 保持 v1。七个公开 API digest 不变，说明本轮只关闭 owner 私有恢复/观察协议漏口。Workflow 继续是 managed Strategy，`flow` 只提供显式拓扑、typed composition、确定顺序和有界 fan-out 等设计参考，不形成强制依赖或复用目标。

## ADR-A2-056：生产 package 集合与允许依赖边构成单一可执行合同

- 状态：已接受；完成 P9-03。
- 证据：P1–P8 的局部 architecture tests 能禁止原框架实现、Host、具体 Strategy 或 OTel SDK 等若干 import，却没有一个守卫能枚举全部生产 package、证明 Kernel 无内部出边，或让新增未审计 package/边立即失败。Platform 与 OTel 又各自复制具体 Strategy denylist，同一边界会随新增 Strategy 漂移。`go list` 显示当前真实生产图只有 root、Interaction、Planning、GOAP、Workflow、OTel 和 Platform 七个 package；examples 是外部式组合消费者，不是生产依赖层。
- 决策：建立唯一完整生产 package 图：Interaction、Planning、Workflow、OTel、Platform 只可直连 root；GOAP 只可直连 Planning；root 不可 import 任何 agent 子 package。门禁遍历所有非测试生产 `.go` 源码而不按 build tag 隐藏文件，精确比对 package 集合和允许的内部直连边，并验证声明图自身无环。新增 package 或边必须先以真实职责和消费者通过 ADR 修改该合同，不能通过更新散落 denylist 偷渡。
- 决策：同一全图门禁集中禁止所有生产包 import 原框架实现、Host `app`、`flow` 与 `log/slog`；OpenTelemetry 只能由 `otel` adapter import，`chatclient`、`tool` 与 `core/chat` 只能由 Interaction import。OTel package 的 SDK 禁入、Kernel/Strategy 的 Host 抽象命名、Dispatcher 不拥有 Process lifecycle、Workflow sealed Stage 等 package-specific 所有权继续由本地门禁负责；已被全图合同覆盖的重复 import 扫描删除。
- 决策：examples 明确排除于生产 package 集合，因为它们的职责就是像真实消费者一样组合多个公开 package；它们仍由独立编译、执行和 public API baseline 验收。Workflow 继续吸收 `flow` 已证明的设计思想，但生产代码不依赖、包装或复制 `flow` runtime；未来若真实 adapter 被证明，必须作为新 package/边重新裁决。
- 后果：架构门禁从“已知坏边黑名单”升级为“唯一允许图 + owner-specific 限制”。本轮只修改测试和架构事实，不改变七个 public API digest、任何 snapshot/state/protocol/Event wire 或运行语义。

## ADR-A2-057：取消状态使用 Go 生态一致的唯一拼写并显式升级恢复合同

- 状态：已接受；完成 P9-06，P9 完成。
- 证据：最终完整项目 lint 发现根公共 API 使用 `StatusCancelled` 与 JSON `"cancelled"`，既不符合仓库统一美式英文，也与 Go 标准库 `context.Canceled` 的既有术语冲突。同一轮还发现 P1 的两个 disposable spike 在正式实现和行为合同已完整接管后仍留在主测试集合，普通测试甚至借用了 spike 文件中的 helper；稳定架构文档也残留已完成阶段的未来时态。它们分别属于公共词汇不一致、验证资产生命周期失守和事实文档漂移，不能以关闭 lint、保留别名或改写历史解释掩盖。
- 决策：公共状态只叫 `StatusCanceled`，wire 只接受 `"canceled"`。不提供 `StatusCancelled` alias，不接受旧 JSON 拼写，不建立迁移 decoder。由于 Status 嵌入 Process Snapshot/TreeSnapshot，schema 分别从 v5/v3 升为 v6/v4；prior-version 与 prior-spelling tests 直接证明旧格式被拒绝。其余 Strategy protocol 与 Event/Delta wire 没有变化，不为统一版本号而无意义升级。
- 决策：P1 disposable spike 在正式 Interaction、Engine、prepared Step、snapshot/restore 与 unknown Effect 合同全部有 owner 测试后整体删除。任何仍需要的普通测试装配只能由真实测试 owner 自己拥有；同 package 的重复 Interaction Deployment 构造收敛为一个已存在 helper，不形成生产测试框架。稳定架构文档只陈述当前冻结事实，阶段性计划、spike 与验收过程只进入执行计划和能力台账。
- 决策：阶段完成门禁使用项目完整 `golangci-lint` 配置，而不把 `staticcheck` 当作其替代。额外 duplicate/complexity 指标只用于发现证据：重复职责必须修复；sealed union validator、状态机和测试 fixture 的完整分支不为追逐任意阈值拆散领域不变量。
- 后果：形成 Baseline 6，根 public digest 与 Kernel snapshot/protocol digest 显式更新；其余六个 public package、Strategy owner wire、Event/Delta wire 与 production package DAG 保持不变。P9 结束时不再有 disposable spike、跨 spike helper、事实过期的稳定设计语句或已知静态问题。

## ADR-A2-058：P10 只按真实 consumer 证据重开六个中性合同

- 状态：已接受；完成 P10-02，实施与 baseline 更新在 P10-03/P10-04 分批发生。
- 证据：P10-01 重新扫描确认 `app/runtime` 的 54 个旧 Framework import 与 P1 相同，且全部真实 Framework 认知仍集中在 execution adapter、直接 toolset 装配及其测试。现有 Baseline 6 已覆盖 Process、tree、Signal/WaitID、Effect、capture/restore、Interaction、Delegate 和 observer 主体；迁移没有证明 Run、Segment、Store、transaction、history、approval、pricing 或 UI 应进入 Framework。它只暴露了 P8 decision-only admission 过窄、Dispatcher 缺完整归因、Interaction 无 deferred manifest、Host 不能把 best-effort Delta 当 transcript、waiting child cancellation 不能由 Host 改 private state，以及 Delegate ChildKey 派生未公开六个具体缺口。
- 决策：`ProcessAdmitter` 的唯一方法修订为 `Admit(ctx context.Context, admission ProcessAdmission) error`；`ProcessAdmitterFunc` 使用相同签名。`ProcessAdmission` 新增只读 `StartedAt()`，该 UTC 时间在调用前生成，admission 成功后就是 Process 的真实开始时间。root 使用 Start context，child 使用执行 Framework Effect 的 context；实现可以完成 caller-owned 外部 admission，但必须尊重 context、保持有界、并发安全、不重入 Engine/Process，且对可能以同一 prospective ProcessID 重放的调用自行保证业务正确性。Framework 仍不定义 Store、transaction、CAS、lease、charge 或幂等 SPI；restore 仍不重复 admission。
- 决策：`EffectRequest` 补充 `DeploymentRef()` 与 `Relation()`，与已有 ProcessID、StepSequence、BatchIndex、EffectID、Effect 一起构成 Dispatcher 的完整 Framework 归因。它们都是 Engine 已拥有的不可变事实，不新增 Host metadata。Interaction 在自己的 package 提供 `ModelInvocationFromContext` 与 `ToolInvocationFromContext`；对应不可变值只暴露 ProcessRelation、exact DeploymentRef、EffectID、StepSequence、ModelCallSequence，以及 Tool 专属的 ToolCallIndex/ToolCall 防御性快照。Dispatcher 只在实际 chatclient/Tool 调用范围附加值，调用结束后 context 不成为控制入口；不恢复 ProcessContext、dependency scope 或 Engine handle。
- 决策：Interaction model Effect 明确携带一基 `ModelCallSequence`，Tool batch Effect 还携带该序号与原模型响应中的零基 `FirstToolCallIndex`；二者是 Strategy protocol，不进入 Kernel。`DelegateChildKey(modelCallSequence, toolCall)` 成为实际 managed Delegate 使用的唯一公开确定派生函数，Runtime 的 ToolCall observation 与 child `ProcessAdmission.Relation().ChildKey()` 使用同一值建立因果关系。Kernel 不新增 SpawnCallID、ToolCall、summary 或产品 metadata。
- 决策：`DispatcherConfig.Tools` 精确表示初始可执行且 model-visible 的普通工具，新增 `DeferredTools` 表示同一冻结 Deployment 内可执行但初始不广告的普通工具；两组与 Delegate 名称全局唯一。Interaction 提供 `AdvertiseTools(ctx, names...) error`，只允许正在执行的 Tool 请求把已绑定 deferred name 从下一模型调用开始单调加入 manifest；缺少调用能力、空/未知/非 deferred name 都明确失败，不在 Agent 外静默 no-op。每个 Tool outcome 返回的新增 name 按模型 ToolCall 顺序稳定合并进 ExecutionState，checkpoint 与 restore 保留结果；它不能加载新 Tool、替换 executable、扩大 capability 或访问 live registry。
- 决策：Runtime 的权威 Message/Reasoning/ToolCall/ToolResult/usage 投影继续由 caller-owned chat middleware 与 Tool decorator同步产生，利用上述 invocation context 做 Process 归因和 backpressure。Framework Delta 仍是有界 best-effort，Event listener 仍是隔离观察；不提升可靠等级、不让 observer veto、不从丢失 Delta 重建 transcript。
- 决策：Kernel 新增唯一 `WaitingSubtreeCancellationPlan`，由 `Engine.PlanWaitingSubtreeCancellation(ctx, rootID, targetID, reason)` 在完整 tree quiescent cut 上计算。目标必须是同树的非 root Waiting Process；计划只拥有 source identity/digest、确定 resulting `TreeSnapshot`、parent-before-child 的 `CanceledProcessIDs` 与为阻止父级提前消费 child completion 而形成的 `PausedProcessIDs`，不持有 live lock 或外部资源。结果将目标活动子树以准确 canceled/parent-canceled 终态留在 tree snapshot，保留永久 child budget allocation；Kernel 关闭其等待、生成自己拥有的 ChildOutcome Signal，并把直接父级停在消费该 Signal 之前，不解析或改写 opaque ExecutionState。
- 决策：`Engine.ApplyWaitingSubtreeCancellation(ctx, plan)` 只接受同一 Engine 产生且 source tree 未变化的计划，并把 live tree线性化到计划的 exact Framework state；过期时零修改失败。调用方若决定继续，只能对 `PausedProcessIDs` 使用普通 `Process.Resume`；若仍有外部等待则保留 paused checkpoint。计划/应用 API 不出现 commit、transaction、Run、Pending、checkpoint store 或 disposition。Strategy owner 的公开 helper 负责从 resulting snapshot读取 surviving external waits；Host 仍负责在自己的事务中原子保存 opaque tree、产品记录与删除/终态集合。
- 决策：实施按依赖方向分四个批次：先补 Kernel admission/attribution 与 Interaction invocation/deferred/Delegate 合同及 owner tests；再以 Interaction 重写 root turn；随后迁移 child admission、HITL、tree mutation、toolset discovery 和剩余直接消费者；最后删除应用侧旧 GOAP wrapper、generic orchestration 与全部旧 import。前端、TUI、CLI 不在 P10 修改范围。每个批次先更新对应 public/wire baseline，不提供 alias、dual-read、dual-write 或临时 bridge。
- 原因：这些 seam 都能脱离 Lyra 产品词汇独立说明，也分别由 admission correctness、Strategy adapter attribution、冻结 manifest 状态与 Engine-owned tree invariant 证明；因此是合理框架能力。反过来复制 Runtime DTO、把 transcript 建在 Delta 上、让 Host 改 Interaction wire 或保留旧 engine 作为旁路，都会重新制造抽象泄漏和双重所有权。
- 后果：Baseline 6 将按实现批次显式升级。Admission/EffectRequest 改根 public API但不必修改 snapshot wire；Interaction public API、ExecutionState 与 dispatcher protocol 必须升级；waiting subtree plan增加根 public API，并以现有 Kernel owner wire 表达 resulting state，只有实现证明确需新持久字段时才升级 snapshot schema。P10-03 开始前不再新增迁移候选合同。

## ADR-A2-059：等待子树取消使用纯计划与同一栅栏内的 staged projection

- 状态：已接受；完成 ADR-A2-058 剩余 Kernel 合同，形成 Baseline 9。
- 证据：等待中的非 root Process 可能同时拥有外部 WaitID、活动 descendant、父级 child wait、已永久划拨预算和既有 `Process` handle。直接删除节点会破坏完整 tree、预算与结果事实；让调用方改 snapshot private wire 会泄露 Kernel/Strategy 所有权；用普通 `Cancel`/`Pause` 串联又无法先得到确定结果、证明 source 未变化或让 stale 失败保持零修改。实现还必须让生成的结果 TreeSnapshot 能独立 restore，而不能只在当前 goroutine 中看似成立。
- 决策：`WaitingSubtreeCancellationPlan` 只保存不可变 Engine value identity、source root/digest、resulting TreeSnapshot 与防御性复制的 canceled/paused Process ID 列表；不保存 Engine pointer、lock、callback、Execution 或 caller resource。规划只在完整 quiescent cut 上投影 Kernel-owned lifecycle/mailbox/child-wait state；opaque `ExecutionState` 原样保留。target 使用 host-cancellation cause，活动 descendants 使用 parent-cancellation cause，所有等待关闭，预算划拨不回收；达到 child wait 条件时生成稳定 ChildOutcome Signal并把直接父级停在消费前。
- 决策：应用复用 tree capture 的唯一 barrier。完整 source digest 比对和每个 Process source snapshot 复验都发生在任何修改前；随后所有 projection 只暂存在对应单写者 loop 中。Engine 先把同树 child-wait registration 收敛到计划结果，再关闭一个共享 apply gate；各 loop 只安装已验证的 Framework state，终态仍走既有 finish/bookkeeping，现有 controller 与 Process handle 不替换。stale、foreign、malformed 或 stage 失败均在 gate 前返回，因此 live Framework state 零修改；跨过 gate 后即使调用 context 取消也完成有界 finalization。
- 决策：共同 Process Snapshot v6、TreeSnapshot v4、child/framework protocol 与 Event/Delta wire 足以表达全部结果，不新增 cancellation-plan wire、第二 snapshot 版本或 Strategy 字段。quiescence 期间新到 Host context 终止只暂存在 barrier，释放后再进入原控制面，避免在 source capture 与 apply gate 之间形成隐藏第二写者。计划 API 不出现 commit、transaction、Run、Pending、checkpoint store 或 disposition。
- 后果：根 public API/GoDoc 形成 Baseline 9；其余六个 public package 和全部 owner wire digest 不变。合同测试证明 planning 无副作用、parent-before-child 终态 cause、外部 wait 关闭、Kernel child outcome、parent pause/resume、exact live/result equality、foreign/stale rejection 零修改，以及 resulting TreeSnapshot 在新 Engine 中不接触 private Strategy state 即可恢复和继续。

## ADR-A2-060：已接受准入必须以中性 start outcome 闭合

- 状态：已接受；完成 P10-04，形成 Baseline 10。
- 证据：`ProcessAdmitter` 的 durable consumer 可以先接受 prospective root/child identity，随后 `Definition.Start`、initial `Execution.Snapshot` 或 `Definition.Restore` 仍然失败。原合同只把错误返回直接调用方；child failure Signal 又故意不暴露 prospective ProcessID，best-effort Event 也没有 error/veto 通道，因此 caller 无法把“已接受但从未发布”的 identity 与精确失败闭合。把产品 Run 状态、Store transaction、reconcile callback 或 child-specific durable handle带入 Kernel 会修复症状却污染 Framework；让 Event 承担正确性又违反观察合同。
- 决策：根公共面增加唯一 `ProcessStartOutcomeAcknowledger`，不增加 StartObserver、LifecycleHook、AdmissionPermit 或通用 callback registry。对每个返回 nil 的 `ProcessAdmitter.Admit`，Engine 在同一次 live start 中构造且只构造一个 immutable `ProcessStartOutcome`：它只包含原 `ProcessAdmission`、`ProcessStartOutcomeStatus` 和可选 Framework `Failure`。初始化及 initial capture/restore 全部完成时 status 为 `started`；任一边界失败时 status 为 `aborted` 且 Failure 使用稳定 kind/code/message。admission reject 与 tree/process restore 都不产生 outcome。
- 决策：outcome acknowledgment 是同步 correctness boundary，不是 Event。Engine 在 admission 前为 prospective Process 建立私有 start reservation，并让 Close、duplicate identity、child key 与 tree-limit 检查看到它；外部 admitter/acknowledger 调用期间不持有 Engine lock。started outcome acknowledgment 返回 nil 是发布线性化门，随后只执行 reservation 到 controller 的无失败内部迁移并启动 loop；返回 error 或 panic 时不发布，也不再发送互相矛盾的 aborted outcome。aborted outcome acknowledgment 后始终丢弃 reservation且永不发布；ack error 与原初始化 error 一并保留。caller 可为同一 prospective identity 实现幂等 acknowledgment，但 Framework 不定义其 Store、transaction、charge、Run、lease 或 reconcile 状态。
- 决策：`ProcessStartOutcomeAcknowledger` 只接收 detached-cancellation、保留 value 的 context，必须有界、并发安全、不得重入 Engine/Process；typed nil 和 panic 都明确拒绝或隔离。start reservation 是 Engine 私有并发状态，不进入 Snapshot/TreeSnapshot、Event、Process handle 或任何 public wire。完整 tree restore 在原子注册前同时拒绝 live Process 与 pending start identity/key 冲突。
- 原因：准入、初始化结论与观察是三种不同职责。中性 outcome 正好补足 Framework 自己已经知道、而 caller 无法可靠推断的一项生命周期事实；reservation 保证 acknowledgement 与发布之间没有可恢复业务分支，同时没有让 Kernel 认识任何下游应用聚合。
- 后果：根 public API/GoDoc 形成 Baseline 10；Process Snapshot v6、TreeSnapshot v4、Framework/Strategy protocol、Event/Delta wire与其余六个 public package不变。owner tests 覆盖 root/child started、root/child aborted、ack error/panic、typed nil、admission reject/restore 零 outcome、ack 前零发布、pending start 阻止 Close，以及 architecture reflection 对 outcome 私有字段的精确锁定。

## ADR-A2-061：等待子树变更以一次性 prepared capability 持有源边界

- 状态：已接受；完成 P10-05，形成 Baseline 11；取代 ADR-A2-058/A2-059 中 pure plan、source digest 和后续 Engine apply 的合同；其中 `Apply(ctx)` 的取消语义已由 ADR-A2-064 取代。
- 证据：Runtime 的正确调用顺序是先获得确定 resulting tree，在自己的原子边界保存该结果，再把同一变更提交到 live Framework tree。Baseline 9 的 value plan 在计算后立即释放 quiescence；调用方提交期间 source tree 可以继续接收 Signal、控制请求或 child completion，随后 `ApplyWaitingSubtreeCancellation` 只能以 stale 失败。此时 caller-owned durable result 已提交但 live tree 未改变，无法用重试、lease、补偿或 private snapshot mutation 在 Framework 外治本。把 Runtime transaction、checkpoint、revision 或 opaque app token 放进 Kernel 又会造成明确抽象泄漏。
- 决策：根公共面只保留 `Engine.PrepareWaitingSubtreeCancellation(ctx, rootID, targetID, reason)`，返回指针语义的一次性 `PreparedWaitingSubtreeCancellation`。Prepare 在一个完整 Strategy-safe cut 上计算 exact resulting `TreeSnapshot`、canceled Process IDs 和 paused Process IDs，并持续持有该 source root tree 的 operation/quiescence。capability 不可复制，必须且只能由 `Apply(ctx)` 或 `Discard()` 结束；第二次 resolution 返回稳定 resolved error。resulting snapshot 和防御性复制的 ID 列表在 resolution 后仍可读取。
- 决策：`Apply` 先在所有既有 Process 单写者中暂存并复验 prepared Framework projection；共享 gate 前的 context 取消、stage failure 或内部不一致自动 Discard，释放 source tree且 live state 零修改。child-wait projection 写入后关闭唯一 gate；跨过该边界后 finalization 独立于调用 context 有界完成，保留原 controller/Process handle并复用既有终态 bookkeeping。`Discard` 不应用任何 prospective state，只释放边界。旧 `WaitingSubtreeCancellationPlan`、Engine plan/apply、Engine identity token、source digest 与 stale/foreign errors 全部删除，不提供 alias 或 shim。
- 决策：tree operation 以 root ProcessID 为粒度协调 CaptureTree、RestoreTree 和 prepared cancellation；同一 root 串行，不相关 root 可独立运行。capability 只持有 Engine、tree operation、quiescence、TreeSnapshot、Process identity、Kernel projection 和 resolution primitives，并由 architecture reflection 精确守卫；不得出现 Run、transaction、Store、checkpoint、lease、产品 revision、application callback 或任何 Host wire。Process/Tree snapshot 与 Strategy/Event/Delta wire均不增加字段。
- 原因：prepared capability 是 Kernel 能表达、又不替 Host 拥有事务的最小完整语义：Framework 冻结自己的 source，Host 原子保存自己的事实，之后只选择 Apply 或 Discard。它消除了 value plan 在跨 owner 两阶段协调中的时间裂缝，也没有把应用协议提升为通用抽象。按 root 隔离 operation 则避免一个真实长事务无关地冻结整个 Engine。
- 后果：根 public API/GoDoc 形成 Baseline 11；Process Snapshot v6、TreeSnapshot v4、Framework/Strategy protocol、Event/Delta wire与其余六个 public package不变。owner tests 覆盖 source freeze、Apply/Discard 恰好一次、并发 resolution、gate 前取消零修改、wait-all 保留、exact live/result equality、其他 root 独立和 resulting snapshot 跨 Engine restore；architecture gate 锁定 capability 只能拥有 Framework state。

## ADR-A2-062：Delegate 恢复归因由 Interaction 提供只读 typed view

- 状态：已接受；作为 P10-06 的 Framework 支撑形成 Baseline 12。
- 证据：完整 Process tree 恢复后，Host 必须重新建立模型 Delegate ToolCall 与既有 child Process 的观察归因，否则 child 后续事实无法准确关联原 ToolCall。Kernel 的 TreeSnapshot 已提供中性的 Process relation，但 ToolCall、model-call sequence 与 Delegate segment 都属于 Interaction opaque state。Host 若复制 Strategy wire，或把 SpawnCallID 另存进自己的 checkpoint，会分别造成抽象泄漏和第二真相源；Kernel 若新增 ToolCall 字段则会污染所有 Strategy 的共同窄腰。
- 决策：只在 `interaction` package 增加 immutable `ActiveDelegateChild` 与 `ActiveDelegateChildrenFromSnapshot`。值精确暴露一基 ModelCallSequence、零基 ToolCallIndex、原始 ToolCall、parent-scoped ChildKey 和 Engine-minted child ProcessID；不暴露 Deployment、Engine/Process handle、continuation、private phase、Store identity 或 Host metadata。目标 Deployment 仍由 caller 依据 parent Snapshot 的 exact DeploymentRef 和自己已装配的 manifest 解析，不复制到视图。
- 决策：helper 只解释 Snapshot 的 committed Interaction ExecutionState，保持 Kernel 对 payload 不透明。非 Interaction 或没有 active Delegate segment 返回 `found=false`；Interaction state 的 phase、cursor、pending response、segment settlement、WaitID 或 ChildKey 派生不一致时以 `ErrInvalidExecutionState` 失败。结果保持模型 ToolCall 顺序并只包含已经获得 ProcessID、尚未结算的 child；pending start 没有伪造 child identity。
- 原因：Strategy owner 的 typed inspection helper 与既有 `PendingToolInputFromSnapshot` 是同一层次的恢复人体工程学。它提供恢复必需的最小语义事实，同时避免 Host 解释 wire、Kernel 认识模型协议、或应用 checkpoint 复制执行真相。
- 后果：Interaction public API/GoDoc 形成 Baseline 12；Interaction state/protocol v5/v3、Process Snapshot v6、TreeSnapshot v4、Kernel、其他 Strategy 和 observation wire均不变。真实 waiting Delegate tree 测试证明 capture 后可恢复 exact ToolCall/ChildKey/ProcessID 且原 child 不重启。

## ADR-A2-063：Interaction typed inspector 必须区分合法等待原因

- 状态：已接受；作为 P10-06 Runtime 真实恢复反证形成 Baseline 13。
- 证据：Interaction Process 的 `Waiting` 共同状态既可能表示普通 Tool 正在等待外部输入，也可能表示父 Interaction 正在等待 managed Delegate children。`PendingToolInputFromSnapshot` 原实现把所有 `Waiting` Interaction 都按 Tool checkpoint 校验，因此合法的 Delegate wait 被误报为损坏状态。让 Runtime 读取 phase、探测错误文本或忽略所有 inspector error 都会把 Strategy private state 泄漏到 Host，并掩盖真正的 snapshot/state 不一致。
- 决策：`PendingToolInputFromSnapshot` 仍是只读 Tool-input inspector，不扩张为通用 wait union。它只在 `waiting_input` 返回 `PendingToolInput`；对完整合法的 `waiting_delegates` 返回 `found=false`，并分别使用各自 owner validator 复验 Strategy state。其他与共同 `Waiting` 不相容的 Interaction phase，以及 Engine-minted outer WaitID 与 Strategy committed WaitID 不一致，继续返回 `ErrInvalidPendingToolInput`。
- 决策：等待原因仍只属于 Interaction private state，不进入 Kernel Status、Snapshot wire或新的公共枚举。Delegate 恢复归因继续由 `ActiveDelegateChildrenFromSnapshot` 提供；Tool 输入继续由 `PendingToolInputFromSnapshot` 提供。两者不是近义入口，而是各自拥有不同结果类型、验证合同和消费者的正交 typed view。
- 原因：Kernel 只应知道 Process 正在等待哪个 WaitID，等待的业务/策略原因必须由 Strategy owner 解释。精确区分“不是该 inspector 的能力”与“该 owner state 已损坏”，既保持窄腰不透明，也避免 Host 通过错误分支猜 private phase。
- 后果：形成 Baseline 13。七个 public API digest、Process Snapshot v6、TreeSnapshot v4、Interaction state/protocol v5/v3、其余 owner wire与 observation wire全部不变；新增行为测试证明合法 Delegate wait 返回 `found=false`，真实 Runtime 完整 tree restore 不需要绕过错误。

## ADR-A2-064：Prepared tree Apply 不接受虚假的请求取消

- 状态：已接受；作为 P10-06 Runtime durable consumer 的 Framework 支撑形成 Baseline 14。
- 证据：Runtime 只有在自身 application transaction 已提交 resulting checkpoint 后才调用 prepared Apply。原 `Apply(ctx)` 允许 gate 前请求取消，意味着持久真相已经改变但 Framework source tree仍保持旧状态，迫使 Host 重试一个本应不可撤销的内存提交；这不是有用的协作取消，而是错误的 API 承诺。所有真正可失败的 controller lookup、source validation 与 projection staging 已经在 `PrepareWaitingSubtreeCancellation` 返回前完成。
- 决策：`PreparedWaitingSubtreeCancellation.Apply` 移除 `context.Context`，只保留 `Apply()`。Prepare 继续接收 context 并在 acquire/quiesce/capture/project/stage 任一失败时释放 source tree、返回零 capability；成功返回后，Apply 在 resolution lock 下复验 capability 内部完整性，跨越唯一 gate并独立完成 state installation、quiescence release与既有终态 bookkeeping。nil、损坏或已经 resolved 仍返回稳定 misuse error；正常 caller cancellation 不再是 Apply 的状态分支。
- 决策：该变化只描述 Framework prepared state 的不可撤销提交点，不引入 durability、transaction、checkpoint、Run、application disposition、retry policy 或 Host callback。是否先持久化、崩溃后恢复哪个 snapshot，仍完全属于 Host。
- 后果：根 public API/GoDoc 形成 Baseline 14；Process Snapshot v6、TreeSnapshot v4、Framework/Strategy protocol、Event/Delta wire与其余六个 public package不变。owner tests证明 Apply/Discard 并发仍恰好一个获胜，Apply 后 live tree精确等于 ResultingSnapshot，且不存在请求取消造成的半决策状态。

## ADR-A2-065：绿色重写实现原子取得唯一 Agent module 身份

- 状态：已接受，P11 已实施。
- 证据：Runtime 已在 P8 完成 Interaction、waiting/restore、Delegate child tree、termination 与 recovery 的生产纵切；workspace 搜索证明原框架实现没有剩余 consumer 或独有能力。继续并存只会制造两个模块身份、两套文档和依赖解析歧义。
- 决策：整体删除原框架实现，把绿色重写实现安装为唯一 `github.com/Tangerg/lynx/agent` module，并同步所有真实 consumer、workspace metadata、文档和 architecture guards。禁止 alias module、replace compatibility、转发 package、双读 wire 或旧 package 名。
- 后果：模块发布需要先形成 canonical source commit，再由 Runtime standalone dependency 引用该 commit；这个发布边界不引入第二路径。Baseline 15 重新冻结 canonical GoDoc digest，Framework API 语义和全部 wire version 不变。

## ADR-A2-066：一次性 prepared authority 必须由所有别名共享同一 resolution identity

- 状态：已接受；P14-01 已实施，形成 Baseline 16。
- 证据：`PreparedWaitingSubtreeCancellation` 是 exported struct，Go 调用方即使拿到指针，仍可执行 `duplicate := *prepared`。当互斥锁与 `resolved` 布尔值直接内嵌在该值中时，两个副本拥有不同的 resolution 状态，却共享同一 apply gate、tree operation 与 quiescence；并发 Apply/Discard 因而可能各自通过未解决判断、重复释放资源或关闭同一 gate。仅用“不得复制”的 GoDoc 约束不能证明 Kernel 已承诺的恰好一次语义。
- 决策：prepared value 继续是唯一公开 capability，不新增 token、registry、Engine apply 旁路或 Host 协议；其私有 resolution 状态改为一个共享指针 identity，identity 内只含互斥锁和 resolved 事实。所有 Apply/Discard alias 必须先在线性化锁下检查并推进同一状态；value 内其余 frozen tree 资源仍由原 capability 持有，resolution 后不得再次访问。zero/nil value 始终是 invalid，不通过第一次错误调用伪造 resolved 状态。
- 决策：公开 GoDoc 继续要求调用方不复制 capability，同时准确说明意外副本不会复制 authority。architecture reflection gate 分别锁定 capability 与 resolution identity 都只拥有 Framework primitive；不得加入 Run、Store、transaction、checkpoint、lease、产品 revision 或应用 callback。
- 后果：根 GoDoc digest 升为 Baseline 16；公开名称、参数和签名、Process Snapshot v6、TreeSnapshot v4、Strategy protocol、Event/Delta wire 与其他六个 public digest 不变。新增并发反例对原值和显式值副本同时 Apply/Discard，并证明竞态下恰好一个成功、另一个稳定返回 resolved。

## ADR-A2-067：有状态公共 owner 只具有一个 pointer identity

- 状态：已接受；P14-02 已实施，形成 Baseline 17。
- 证据：Engine、Platform 与 OTel Observer 都拥有互斥锁、共享 map、worker/span 或关闭状态。它们的构造函数返回指针，Platform GoDoc 已禁止使用后复制，但 Engine 与 Observer 未陈述同一约束；调用方值复制会分裂锁和 closed 状态却共享底层资源，形成第二 lifecycle/observation owner。把全部状态搬入可复制 facade 的额外间接层不能增加框架能力，只会掩盖误用。
- 决策：三类 mutable owner 保持 concrete pointer API，不增加 interface facade、registry 或共享 service object。Engine 必须由 `NewEngine` 构造，Observer 必须由 `otel.New` 构造，二者与 Platform 都不得在首次使用后复制；公共 GoDoc 与 digest 直接冻结该约束。不可变值、Deployment/Definition 与只读 handle 继续可安全复制，不把 no-copy 规则泛化到整个 Framework。
- 后果：根与 OTel GoDoc 形成 Baseline 17，公开名称、方法、wire 与 package DAG 不变。现有 mutex 也让 downstream `go vet copylocks` 能识别具体值复制；Framework 不为非法复制提供兼容状态同步。

## ADR-A2-068：观察序号不得伪造或回绕，OTel 必须显式处理无符号范围

- 状态：已接受；P14-02 已实施。
- 证据：Process 原先先递增 `processEventSequence` 再构造 Event，内部构造拒绝会留下没有对应 fact 的幽灵序号；从极值 snapshot 恢复后再次发布还会从 `MaxUint64` 回绕到 0。OTel adapter 又把 Process/Step sequence 直接转成 `int64`，高位会变成负数；Delta drop payload 是 `uint64`，解码进 `int64` 时超界会整体失败并因观察侧忽略 error 静默记成 0。
- 决策：Kernel 先计算候选序号并构造完整 Event，只有成功后才安装序号并同步投递；达到 `MaxUint64` 后保持饱和且不伪造新 Event，绝不回绕或改变 Process 语义。OTel 的 Process/Step sequence attribute 统一使用十进制字符串无损表达 `uint64`；Delta drop payload 按 owner wire 解码为 `uint64`，投影到官方 `Int64Counter` 时显式饱和到 `MaxInt64`。Observer 在 Close 后的调用一律于分派前退出，不再写 span 或 metric。
- 决策：这些是 observation 投影规则，不升级 Process Snapshot v6、TreeSnapshot v4 或 Event/Delta wire，不加入 Host telemetry schema、可靠投递或 observer veto。表示上限后的 Event 不足不能反向终止 Process；观察能力仍不拥有业务正确性。
- 后果：新增无效 Event 不推进序号、极值不回绕、完整 `uint64` attribute、metric 饱和和 Close 后零记录测试。OTel attribute 的 sequence value type 从有损 `int64` 直接切换为准确 string，不保留双属性或旧类型兼容路径。

## ADR-A2-069：Process command context 只界定提交与响应等待

- 状态：已接受；P14-02 以准确 GoDoc 冻结既有语义，形成 Baseline 17。
- 证据：Process control/query 方法通过单写者 loop 提交命令。ctx 可能在命令进入有界 channel 前取消，也可能在 loop 已接收但尚未回复时取消；后一种情况下撤销命令会引入第二个并发写入口，并使已经接收的 Signal、Cancel、Kill 或 ResolveEffect 出现半撤销。`SignalID` 去重和幂等 control intent 已为调用方处理不确定响应提供稳定语义。
- 决策：ctx 同时界定命令提交等待和响应等待；一旦 Engine loop 接收命令，ctx 取消只让 caller 停止等待，不撤销已接收命令。Await 只等待 tree-settled terminal result，Start context 则仍是 Process 的 Host cancellation/deadline 来源，二者不与 command 语义混称。Framework 不增加 request lease、rollback、ack registry 或第二 cancel-command 入口。
- 后果：Process 公共 GoDoc 形成 Baseline 17；实现、方法签名、Signal/wire 与状态机均不变。调用方若收到 ctx error，不得据此断言命令未执行；可依靠稳定 SignalID、状态查询或幂等 control 重试收敛。

## ADR-A2-070：取消请求必须以队列提交而非同步处理结果为合同

- 状态：已接受；P15-01 已实施，形成 Baseline 18。
- 证据：ADR-A2-069 已准确说明普通 command 的 ctx error 不能证明命令未执行。真实 caller 又证明取消若继续复用“提交后等待 loop response”的通用 helper，就会在 Effect 长时间结算时无谓阻塞，并诱使 caller 将超时误解为未接受后回滚自己的决定。取消是 first-wins terminal intent，请求方只需知道有效请求是否已经进入唯一单写者队列；Process 的安全边界和最终 `Result` 才拥有实际提交事实。
- 决策：删除 `Process.Cancel`，以 `Process.RequestCancellation(ctx, reason)` 作为唯一 caller-owned 取消入口。Framework 在入队前同步校验 reason；nil 精确表示请求已提交到 Engine command queue，不表示已经处理、到达安全边界或终止。ctx 只界定入队等待，成功入队后取消 ctx 不撤销请求；竞争中先完成的既有终态仍可优先于尚未处理的请求。Kill/Pause/Resume/ResolveEffect 等需要同步验证或返回处理事实的 command 保持响应式合同，不制造通用 fire-and-forget API。
- 决策：该合同只有 Process、Engine command queue、host-cancellation intent 与终态语义，不增加 Run、Store、transaction、journal、ack registry、lease 或应用回调。旧 `Cancel` 不保留 alias 或兼容转发。
- 后果：根 public API/GoDoc 形成 Baseline 18；Process Snapshot v6、TreeSnapshot v4、Kernel/Strategy protocol 与 observation wire 均不改变。阻塞 Effect 行为测试证明请求先返回、Effect 仍结算、随后在安全边界得到 `StatusCanceled`；已取消的提交 context 确定不入队。

## ADR-A2-071：模型调用必须携带本轮实际应用的 steer Signal 身份

- 状态：已接受；P15-01 已实施，形成 Baseline 18。
- 证据：一个 steer Signal 可在模型、Tool 或 Delegate Effect 期间被 Engine 接受，但只在下一个 Strategy-safe 模型边界进入 WorkingContext。仅观察 `DeliverSignal` 的同步返回不能证明某次外部模型请求是否已经看到该输入；只传消息值又无法把具体 Signal 与具体模型调用作无歧义关联。身份属于 Interaction 对自身输入应用时点的语义事实，不属于 Kernel，也不应由 Host 解析 opaque ExecutionState 猜测。
- 决策：`ModelInvocation` 新增防御性复制的 `AppliedSteerSignalIDs()`，严格按对应 steer 首次进入该模型请求的顺序返回身份；空值只表示本次没有新应用的 steer，不表示 WorkingContext 中没有先前消息。Interaction 将 pending steer 的消息和有序唯一 SignalID 收敛为一个共同校验、共同恢复、共同清除的 Strategy-owned value，并把身份写进同一个 model Effect payload；Dispatcher 只在实际模型调用 context 中公开该 attribution。
- 决策：模型/Tool/Delegate settlement 同批到达的 steer 都按 mailbox 顺序收集；Delegate wait-opened collector 只消费包含 steer 与唯一 opened acknowledgment 的前缀，不能假设首个 Signal 必为 opened，也不能吞掉随后已就绪的 ChildrenCompleted。身份不得进入 Kernel Signal、Event、snapshot 外层或通用 `EffectRequest`，Framework 不认识 consumer projection、message store 或事务。
- 后果：Interaction public API/GoDoc 形成 Baseline 18；ExecutionState 从 v5 升为 v6，dispatcher protocol 从 v3 升为 v4，旧版本直接拒绝且不双读。Process Snapshot v6、TreeSnapshot v4、其他 Strategy、Kernel 与 observation wire 均不变。恢复、顺序、防御性复制、模型/Tool 边界和 Delegate 信号交错测试冻结该语义。

## ADR-A2-072：托管执行只保留一个准确词汇

- 状态：已接受；P15-02b 已实施，保持 Baseline 18。
- 证据：旧实现删除后，Interaction、Planning 与 Workflow 都只实现同一个 `Execution` 合同，事实文档却仍给其中部分策略增加没有可替换对象的实现限定词；prepared Step 的注释又借用了 Host 持久化词汇描述 Framework 候选状态。这两组称谓都没有增加可辨别语义，反而暗示不存在的第二执行合同和事务所有权。
- 决策：Interaction、Planning 与 Workflow 直接称为 Strategy/Execution，不增加无对照限定词。只有与独立 in-process `flow` 的生命周期形成真实对照时，Workflow 才称 managed Workflow。prepared Step 只称候选状态与固定 Effects；`EventStepPrepared`、Limits/Usage GoDoc 和事实文档不得把它描述成 Host transaction。
- 决策：权威文档与 Framework production source 由自动词汇守卫共同约束；外部框架比较材料不是当前合同，不进入该守卫。守卫只拒绝精确词项，不把其他包含相同字母的普通英文单词误判为执行术语。
- 原因：一个概念只应有一个名称；限定词只有在区分真实可替换对象时才有信息量。Framework 内部状态提交与 Host 持久化事务属于不同 owner，借词会把应用职责反向泄露进 Kernel 语义。
- 后果：公开 identifier、参数、签名、行为和全部 wire 均不变；根 GoDoc digest 在 Baseline 18 内显式更新。后续若出现新的真实执行实现或生命周期对照，必须先用消费者证据重新定义词汇，而不能先增加近义标签。
