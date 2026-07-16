# Agent Framework 架构审查

> 审查日期：2026-07-16
> Module：`github.com/Tangerg/lynx/agent`
> 状态：开发期破坏性重构基线已通过完整门禁；尚未发布或承诺兼容

## 1. 结论

`agent` 的角色是可嵌入应用的框架，不是 `core` 的工具集合，也不是对 Embabel 类型系统的逐项翻译。当前架构已经形成完整所有权：

- `core` 定义稳定领域语言和最小 SPI；
- `runtime.Engine` 拥有部署、Process 状态机、执行循环、恢复和资源协调；
- planner、routing、tool-loop、HITL 与 workflow 作为明确策略或组合能力存在；
- 根 `agent` 包只提供高频标准路径；
- provider、transport、数据库和产品策略留在应用 adapter。

设计采用 DDD 的聚合与统一语言、整洁架构的依赖方向，但不复制分层样板目录。判断一个抽象是否保留，优先看 Go 调用点是否自然、是否减少状态所有者、是否存在真实替换需求，而不是看原项目是否有同名接口。

## 2. 当前公共基线

| 项目 | 当前值 |
|---|---:|
| 公开 package | 16 |
| exported API declaration | 628 |
| 根 façade | 46 / 50 |
| 受管 exported JSON struct | 14 |
| wire fixture | 456 行 |
| 外部实现 contract package | `storetest` |

公开 package 是根包、`core`、`event`、`hitl`、`interaction`、`planning`、四个 planner、`routing`、`runtime`、`storetest`、`toolloop`、`toolpolicy` 和 `workflow`。新增公开 package 必须同时进入依赖梯级与 API baseline。

`storetest` 是故意公开的测试契约包，角色类似标准库 `fstest`、`slogtest`。它服务于库外 `ProcessStore` 实现，不应移动到 `internal`。不存在只为形式对称而保留的 `providertest`；provider/resolver 的完整语义在真实 runtime dispatch 边界验证。

## 3. 依赖梯级

```text
core/, interaction/
        ↓
planning/, event/, hitl/, toolpolicy/, toolloop/
        ↓
runtime/, routing/
        ↓
agent root, workflow/, storetest/
        ↓
host application and adapters
```

约束如下：

1. `core` 不依赖 runtime、应用或 transport SDK。
2. 策略与协议包可以依赖内核，但不能控制 Engine。
3. `runtime` 是唯一 Process 生命周期 owner，不依赖根 façade、workflow 或测试契约包。
4. workflow 生成普通 Agent，不创建第二套执行器。
5. 应用可以依赖 Agent；Agent 不得引用应用的 Turn、SQLite、SSE、UI 或租户类型。

这是一条 framework dependency ladder，不是 application/domain/infrastructure 的同构目录树。

## 4. 聚合与所有权

| 对象 | 所有者 | 不变量 |
|---|---|---|
| `core.Agent` | definition author / Engine | 构造后只读；集合返回防御性副本；拥有 durable blackboard type codec |
| `core.Goal` | Agent / planner | 只读目标值；拥有满足判断与工具元数据 |
| `planning.Domain` | runtime / planner | 不可变 capability universe；拥有 plans、best-plan 与 prune 派生行为 |
| `planning.Plan` | planner | 不可变 Action 序列与 Goal |
| `runtime.Deployment` | Engine catalog | 编译后不可变；具有精确 `DeploymentRef` |
| `runtime.Process` | Engine | 永久绑定一个 Deployment；单 active execution owner；拥有 continuation 状态判断 |
| `core.ProcessContext` | runtime | 只向当前 Action 暴露必要的写、交互和控制能力 |
| `interaction.Suspension` | Process | JSON-safe、可验证、不保存闭包 |
| `toolloop.Checkpoint` | interaction | 只保存可恢复的模型/工具轮次协议状态 |
| `core.ProcessSnapshot` | ProcessStore | schema、revision、部署身份和 durable state 的唯一事实 |

`Engine.Deploy` 对 Agent 做验证和编译。相同定义重复部署幂等；同名不同定义必须 `Replace`。历史 Deployment 保留，已运行 Process 和 snapshot 不随活动路由漂移。child、workflow 和 agent-as-tool 都捕获同一 Engine 中的精确 Deployment，而不是在执行时按名称重新猜测定义。

## 5. API 设计规则

### 5.1 接受能力，返回具体值

构造输入依赖窄接口或 capability，框架返回由自己拥有的具体对象。例如 `runtime.Config.Chat` 接受 `core.ChatCapability`，`NewEngine` 返回 `*Engine`，`Deploy` 返回 `*Deployment`。接口放在消费方，不为单一实现创建 `Impl`、`Service` 或 `Manager`。

### 5.2 让零值有安全含义

- `RetryPolicy{}` 表示只尝试一次；
- `StuckDecision` 的零值是停止，而不是隐式继续；
- `toolloop.Config{}` 使用受限默认轮数；
- 可选 capability 为 nil 时要么明确禁用，要么回退，不能形成半有效状态。

### 5.3 减少中间 DTO 与 builder

- 普通配置使用 struct literal，不提供 fluent builder；
- `toolloop.Runner.Run(ctx, request, resolver)` 直接接收运行输入，不再要求构造只承载两个字段的 `Invocation`；
- `hitl.Interrupt[R]` 返回 `(R, error)`，恢复状态通过统一 suspension error 表达；
- Action 自定义 binding 只走 `Inputs`、`Outputs` 与 `NewBinding[T]`，不保留重复快捷字段。

### 5.4 用所属包消除命名重复

优先 `runtime.Engine`、`planning.Domain`、`utility.GoalFirst`、`routing.ModelConfig`、`workflow.ScatterGather`，避免 `Platform`、`PlanningSystem`、`GoalFirstPlanner`、`ModelRankerConfig` 一类上下文已经表达过的重复命名。短接收者如 `e`、`p`、`r` 属于 Go 惯例；跨多行承担业务含义的局部值使用 `engine`、`process`、`deployment`、`request` 等完整名称。

当行为只有一个明确 owner 时，调用点直接从对象出发：`agent.EncodeBlackboard`、`domain.BestPlan`、`engine.GoalToolsFor`、`requirement.Allows`。构造器、跨类型组装、wire decode、按多种类型实例化的泛型算法继续保留自由函数；receiver 数量不是目标，行为归属才是目标。

私有实现同样遵循这条规则：`FuncAction[In,Out]` 自己读取 `In`，`DependencyKey[T]` 自己校验并处理对应类型的 nil，`runnerState` 自己验证输入、消费 resume 并构造 continuation，`Process` 自己调度 action middleware、goal approver、chat scope、child listener 与 interaction owner。Deployment 的冻结快照、canonical encoding 和 digest 由持有 `buildID` 的私有 `deploymentCompiler` 聚合；它是有真实状态和单一职责的编译器，不是为挂方法制造的 service 壳。

当前保留的包级私有函数主要是构造器、wire decode、跨类型 projection、slice/map 共享算法、对称 codec、外部 SPI guard 与需要按多种 `T` 实例化的泛型函数。生产包级私有自由函数从 140 收敛为 92，但该数量只用于证明审计覆盖，不作为继续方法化的目标。

## 6. 执行、交互与恢复

`Action.Execute` 返回 `(ActionStatus, error)`：status 表达生命周期结果，error 表达失败、replan 或 suspension。Runtime 统一持有 panic recovery、retry、事件和状态迁移，不通过 Context scratch 或 Blackboard 旁路传错。

Action 内模型调用走 `ProcessContext.Prompt`、`PromptJSON` 或 `Interact`。Framework 负责：

- 绑定 Process、Deployment 和 Action identity；
- 发布 model/tool boundary 事件；
- 记录 model call、embedding call、usage、cost 和 budget；
- 执行轮数、步数和预算限制；
- 将 tool/HITL pause 统一为 Suspension；
- 保存 checkpoint，恢复时不重复已完成工具调用。

`toolloop.Runner` 是可独立复用的叶子协议执行器。直接使用它的调用方自行承担 Process、事件、usage 和持久化；Host 不用它复制第二套 Agent runtime。

## 7. 扩展与动态依赖

所有行为扩展先实现 `Extension.Name()`，Runtime 再通过最小 capability interface 发现。Engine scope 与 Process scope 有明确合并顺序；nil、空名和同 scope 重名在构造边界失败。

动态领域依赖使用 typed `Dependencies`：

- `DependencyKey[T]` 表达键和值类型；
- `RegisterDependency` 单次注册；
- `LookupDependency` 向父 scope 查找；
- 运行前 freeze。

它不是全局 DI 容器。静态 Action 依赖仍优先构造函数、字段或闭包注入。

## 8. 持久化边界

`ProcessSnapshot` 是唯一 durable Process 事实，`ProcessStore.Save` 使用 expected revision CAS 防止 lost update。CAS 不负责跨节点执行所有权；分布式 handoff 仍需 Host lease/fencing。

Blackboard 普通值默认 durable，函数、channel、client 和 runtime handle 必须显式 transient。未知 schema、未知字段、无效 enum、DeploymentRef 不匹配或 checkpoint correlation 错误一律 fail closed。开发期不为旧 snapshot 增加双读 decoder。

## 9. 当前兼容与发布边界

项目仍在开发期，本轮明确不保留兼容层：

- 不保留 alias、deprecated wrapper、builder shim 或 dual path；
- 不恢复旧名称来降低迁移成本；
- API / wire baseline 是审查工具，不代表已经发布稳定承诺；
- 已形成可回退的开发提交；未创建 tag、push 或 release。

当前 receiver 精修基线的 Agent 全量 build、vet、test、race、lint、tidy、API/wire/architecture gate、workspace 84 项常规门禁，以及 App 高风险 race 均已通过。公开 API 与 wire hash 均未变化。正式发布仍需在内部依赖改为精确 tag 后，以 `GOWORK=off` 重跑发布门禁，并由维护者单独授权发布动作。

## 10. 维护规则

1. 根 façade 保持高频路径，复杂协议留在 owner package。
2. 新公开接口必须有稳定分发边界和至少两个真实实现或明确库外需求。
3. 能用函数参数、普通配置或小接口表达的能力，不进入 registry。
4. 文件名描述职责或领域对象，不使用 `common`、`util`、`helper`、`manager`、`impl`。
5. 新 adapter 优先运行公开 contract suite；只为测试存在且不服务库外实现的代码留在 `_test.go` 或 `internal`。
6. 任何 API/wire baseline 更新都必须先解释语义，而不是把 golden 刷新当作修复。
