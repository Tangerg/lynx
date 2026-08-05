# CLAUDE.md — agent module

> Goal-oriented agent runtime:把 agent 定义(goals + actions + conditions)编译成可观察、可暂停、可恢复的运行进程。**Planner-driven,不是 ReAct-loop** —— 每个 tick 让 planner 看世界状态 + goal,产出下一步 action。lyra 后端用它跑 chat turn 的工具循环。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。关键类型 / 文件布局 / 依赖版本以代码与 godoc 为准 —— 本则只讲宏观。
> Agent 的长期目标、阶段任务和进度以 [`../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md) 为唯一执行基准。

---

## 定位

- **把声明式 agent 编译成进程**:输入是 goals + actions + conditions,输出是一个有状态机的进程 —— 可观察(event)、可暂停(HITL)、可恢复(resume)、可派生子进程。
- **Agent 是 execution framework,不是应用运行时**:Root Core 提供稳定协议库;Agent Engine 只拥有部署、主循环、状态迁移、挂起恢复、进程树执行状态的捕获与重建。存储、事务、幂等、重试、保留策略、Host build compatibility、checkpoint 提交元数据和产品 Session 生命周期全部由消费方拥有。框架仍显式装配、可嵌入,不引入扫描、注解或全局 DI 容器。
- 是 lyra 的执行内核:一次 chat turn 的工具循环就是一个 agent 进程。

## 架构心智(三支柱)

- **原语层**:Action / Agent / Goal / Condition / Blackboard / Process / Extension;条件用**三值逻辑**(True / False / Unknown),"还不知道" 是一等状态,不是 nil bool。
- **执行引擎**:Engine 管 deployment/process registry 与进程生命周期;进程是 observe → plan → act 的 tick 状态机,支持结构化子进程派生、事件多播、HITL 恢复。**加能力靠类型分发**(一个泛型 collector 按接口收集 Extension),而不是改 dispatch loop —— 这是本模块的 OCP 落点。
- **能力切片**:planning(Planner 接口 + 各算法各一包)、routing、workflow(高阶组合器,但都**编译回普通 agent**,不是新 runtime)、event、interaction、hitl、toolloop。
- **Blackboard 是 planner 可见性的枢纽**:action 之间**不直接传值**,一律按 name + type 读写黑板 —— 绕过它调度就坏;读写用分离的 reader / writer 面。
- **HITL 是 first-class**:等待输入把进程切到 Waiting,operator 通过 Respond 写入可恢复 response,再由 Continue 从 action 入口重入;已回答 Waiting 恢复后直接 Continue。Suspension 只有 `FrameworkState` 一处 continuation，且不带"谁来回答"的分类（Framework 从不据此分支）；ToolLoop checkpoint 必须跳过已完成的模型轮次和工具。框架不尝试恢复任意 Go 调用栈。
- **协议与执行状态分离**:`toolloop.Runner` 直接消费 `core/chat.Request` 与消费方 `ToolResolver`；model/tool/pause/resume 通过 `toolloop.Event` 表达，不向 provider Response 塞运行时状态。Runner 是唯一工具循环：工具默认独占，显式安全的调用按 resource key 有界并发，但 event、continuation、checkpoint 始终按模型调用顺序提交；无自动 retry，可 checkpoint/resume。
- **委派子进程默认使用干净黑板**,不继承父黑板 —— 全继承会预满足子 agent 的产出目标、让它静默不干活。只有两档明确语义:默认干净状态,或调用 `RunChildWithWorkingState` 显式复制完整工作状态。宿主请求上下文走 Go context,不进入 Blackboard。
- **框架内部用具体类型**:内聚子系统的单实现依赖直接用具体类型;窄接口留给公开 SPI(Planner / Extension 子接口等)、跨模块消费边界和真实替换点。不能因为 Agent 是 framework 就为每个内部 struct 造 interface。
- **构造边界取得配置所有权**:`runtime.Config` / `ProcessOptions` 是调用方 DTO,Engine/Process 只保留校验并快照后的私有状态;不能让 caller 后续修改 Extensions slice 或 ChatMiddleware 改写运行中的语义。模型调用所需的产品上下文由 Host 作为普通 Call/Stream middleware 注入；Agent 不定义 conversation/session 标识或绑定钩子。
- **资源计数与产品账本分离**:Agent 只保存执行预算需要的直接/子树聚合计数（opaque cost、tokens、model calls、actions），不保存 provider/model 明细、USD 账单、embedding 调用、时间线或审计记录。Host 从 managed-interaction event 建立自己的账本，并与 process tree 在自己的事务中提交。Agent 的零值 Budget 无限制；成本单位和默认阈值是 Host policy。

## 模块特有反向不变量

- ❌ **绕过 Blackboard 让 action 之间直接传值** —— 破坏 planner 可见性,调度会坏。
- ❌ **用 string-key 注册 Extension** —— 类型分发才能加能力不改 dispatch loop(OCP)。
- ❌ **框架内部为单实现依赖机械抽接口** —— 内聚实现用具体类型,窄接口留给公开 SPI、跨模块消费边界与真实替换点。
- ❌ **把公开 config struct 直接挂到长生命周期对象上** —— 先校验、复制容器并投影到私有运行状态;接口实现本身再遵守其并发/生命周期合同。
- ❌ **把存储协调泄漏进 Agent** —— `agent` 可以捕获、验证和重建 portable process snapshot,但不得定义 Store/Repository、事务、幂等、重试、保留策略、Host BuildID、提交时间、产品 Session 或持久化失败策略；这些接口必须由外部消费方定义。
- ❌ **把产品计费/审计泄漏进 Agent** —— 不得定义 provider/model invocation ledger、USD 字段、embedding 账本或产品默认预算。框架只记录 managed execution 所需的通用聚合 `Usage`；明细投影和持久化由 Host 拥有。
- ❌ **把工具发布与执行策略泄漏进 Agent** —— Goal 只表达规划目标；外部工具发布、权限/沙箱、once-only、幂等、事务、补偿和产品审批由 Host 在工具装配边界实现。框架只提供显式 child AgentTool、ToolGroup role 解析与中间件原语。
- ❌ **把 examples/ 当 reference** —— 那是 demo,约定不保证跟主线一致。

## 改动前必看(波及面)

- **动 Extension 子接口签名**:类型分发的收集会断,每个实现方都受影响。
- **动 ProcessView / ProcessContext**:所有 policy、extension 或 action 函数都会受影响,爆炸半径 = 全 agent + lyra 业务代码。
- **动 Blackboard 的 name+type 协议**:框架自动绑定入参靠它。
- **加 planner / workflow builder**:planner 新起一包实现接口、不改 runtime;workflow builder 输出普通 agent。
