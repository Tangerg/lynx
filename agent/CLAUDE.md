# CLAUDE.md — agent module

> Goal-oriented agent runtime:把 agent 定义(goals + actions + conditions)编译成可观察、可暂停、可恢复的运行进程。**Planner-driven,不是 ReAct-loop** —— 每个 tick 让 planner 看世界状态 + goal,产出下一步 action。lyra 后端用它跑 chat turn 的工具循环。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。关键类型 / 文件布局 / 依赖版本以代码与 godoc 为准 —— 本则只讲宏观。

---

## 定位

- **把声明式 agent 编译成进程**:输入是 goals + actions + conditions,输出是一个有状态机的进程 —— 可观察(event)、可暂停(HITL)、可恢复(resume)、可派生子进程。
- 是 lyra 的执行内核:一次 chat turn 的工具循环就是一个 agent 进程。

## 架构心智(三支柱)

- **原语层**:Action / Agent / Goal / Condition / Blackboard / Process / Extension;条件用**三值逻辑**(True / False / Unknown),"还不知道" 是一等状态,不是 nil bool。
- **执行引擎**:Platform 管 registry 与进程生命周期;进程是 plan → observe → act 的 tick 状态机,支持并发子进程派生、事件多播、HITL 恢复。**加能力靠类型分发**(一个泛型 collector 按接口收集 Extension),而不是改 dispatch loop —— 这是本模块的 OCP 落点。
- **service 切片**:planning(Planner 接口 + 各算法各一包)、workflow(高阶 builder,但都**编译回普通 agent**,不是新 runtime)、event、hitl、toolpolicy(chat tool 装饰器)。
- **Blackboard 是 planner 可见性的枢纽**:action 之间**不直接传值**,一律按 name + type 读写黑板 —— 绕过它调度就坏;读写用分离的 reader / writer 面。
- **HITL 是 first-class**:等待输入把进程切到 Waiting、状态落黑板,operator 回复后原地重入(不重跑整个 turn)。
- **协议与执行状态分离**:`toolloop.Invocation` 并置 `core/chat.Request` 与消费方 `ToolResolver`；model/tool/pause/resume 通过 `toolloop.Event` 表达，不向 provider Response 塞运行时状态。`agent/toolloop.Runner` 是唯一工具循环，采用串行、无自动 retry、可 checkpoint/resume 的明确策略。
- **委派子进程默认只带 ambient(protected)项**,不全盘继承父黑板 —— 全继承会预满足子 agent 的产出目标、让它静默不干活。三档梯度:全继承 / 仅 ambient / 全空,按编排需要选。
- **库内部用具体类型**:agent 是 SDK 库,内部包之间直接依赖具体类型;窄接口只留给公开 SPI(Planner / Extension 子接口等)和应用层消费方 —— 库内单实现还抽窄接口是 YAGNI 仪式。

## 模块特有反向不变量

- ❌ **绕过 Blackboard 让 action 之间直接传值** —— 破坏 planner 可见性,调度会坏。
- ❌ **用 string-key 注册 Extension** —— 类型分发才能加能力不改 dispatch loop(OCP)。
- ❌ **库内部为单实现依赖抽消费方窄接口** —— 具体类型即可,窄接口留给公开 SPI 与应用层。
- ❌ **把 examples/ 当 reference** —— 那是 demo,约定不保证跟主线一致。

## 改动前必看(波及面)

- **动 Extension 子接口签名**:类型分发的收集会断,每个实现方都受影响。
- **动 Process 接口**:所有 action 函数都拿它,爆炸半径 = 全 agent + lyra 业务代码。
- **动 Blackboard 的 name+type 协议**:框架自动绑定入参靠它。
- **加 planner / workflow builder**:planner 新起一包实现接口、不改 runtime;workflow builder 输出普通 agent。
