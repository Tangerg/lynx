# 06. 分阶段落地计划

> 从零到可用的 agent 框架，按最小可用（MVP）→ 生产可用 → 生态完整的节奏。
> 每阶段有明确验收标准，阶段间可独立产出价值。

---

## 阶段 M0：骨架搭建（约 3-5 天）

**目标**：建立 `agent/` 顶层 module，跑通空框架。

### 交付物
- [ ] `agent/go.mod`（新 module，依赖 `core/`、`pkg/`）
- [ ] `go.work` 里 `use ./agent`
- [ ] 目录骨架：`core/`、`plan/`、`planner/goap/`、`runtime/`、`dsl/`、`event/`
- [ ] 空的 `core.Agent`、`core.Action`、`core.Goal`、`core.Condition` 接口/struct 定义
- [ ] `runtime.Platform` 骨架（Deploy / RunAgent 空实现）
- [ ] 最简冒烟测试：能构造 Agent，调用 Platform 返回 nil

### 验收
```go
platform := agent.NewPlatform()
a := agent.New("test").Build()
platform.Deploy(a)
_, err := platform.RunAgent(ctx, a, nil)
// err 可以是 "not implemented"，但程序能编译通过
```

---

## 阶段 M1：核心原语 + 顺序执行（约 1-2 周）

**目标**：**能跑一个最简单的 Agent**，单 Action 单 Goal。

### 交付物
- [ ] `core.Determination` 三值逻辑 + 完整单测
- [ ] `core.IoBinding` 结构 + `NewIoBinding[T]` 泛型工厂
- [ ] `core.Blackboard` 接口
- [ ] `runtime.InMemoryBlackboard` 实现（线程安全、类型匹配）
- [ ] `core.NewAction[In, Out]` 泛型构造器
- [ ] `core.Goal` + `GoalProducing[T]` 工厂
- [ ] `plan.WorldState` + `plan.ConditionWorldState`
- [ ] `runtime.BlackboardDeterminer` （世界状态推导，暂不支持逻辑表达式）
- [ ] `runtime.AgentProcess` + 状态机
- [ ] `runtime.SimpleAgentProcess` 顺序执行
- [ ] `dsl.Builder` 流式 API

**暂不做**：
- GOAP A*（M2 做）—— 先用**退化规划器**：按 action 声明顺序尝试
- 事件系统 —— 空监听器
- 观测集成
- QoS 重试 —— 先单次尝试

### 验收
跑通一个最小 agent：

```go
a := agent.New("Hello").
    Action(core.NewAction("upper",
        func(ctx context.Context, pc *core.ProcessContext, in string) (int, error) {
            return len(strings.ToUpper(in)), nil
        })).
    Goal(core.GoalProducing[int]("Count upper")).
    Build()

proc, _ := platform.RunAgent(ctx, a, map[string]any{"it": "hello"})
n, _ := core.ResultOfType[int](proc)
// n == 5
```

---

## 阶段 M2：GOAP A* 规划器（约 1-2 周）

**目标**：**规划真的工作**——多个 action 组合达成目标。

### 交付物
- [ ] `plan.Planner` 接口
- [ ] `plan.PlanningSystem` 结构
- [ ] `planner/goap.AStarPlanner` 完整实现
  - 优先队列（`container/heap`）
  - 启发式 `heuristic(state, goal)` = 未满足条件数
  - 可达性预检 `isGoalReachable`
  - `backwardPlanningOptimization` / `forwardPlanningOptimization`
- [ ] `core.AbstractAction`（`computePreconditionsAndEffects` 自动推导）
- [ ] `Action.canRerun` / `hasRun_` 条件机制
- [ ] replan 黑名单（防循环）
- [ ] 覆盖测试：
  - 线性计划（A→B→C）
  - 分支计划（目标需要多个独立输入）
  - 不可达 goal 的剪枝
  - canRerun=false 的正确性

### 验收
博客 agent 示例（`05-integration-and-examples.md` §6）能跑通：输入 Topic，经过 `research + outline + write` 三阶段得到 BlogPost。

---

## 阶段 M3：集成 Lynx 生态（约 1 周）

**目标**：**能调 LLM、能上报观测**。

### 交付物
- [ ] `core.ProcessContext.LLM()` 返回 `*chat.Client`
- [ ] `core.ServiceProvider` 聚合（chat / rag / vectorstore / observation）
- [ ] `runtime.Platform` 的 `WithChatClient` / `WithObservation` / `WithServices` options
- [ ] 在 `AgentProcess.Tick` / `executeAction` / `AStarPlanner.PlanToGoal` 埋点
- [ ] `core.ResolveTools(roles ...string)` 便捷方法
- [ ] ToolGroup 解析器
- [ ] 示例：一个调用 LLM 的 agent

### 验收
跑 intent 分类 agent（`04-user-api.md` §A），用真实 OpenAI/Anthropic 模型分类输入，观测可以看到：
```
agent.tick {agent.name=IntentReceptionAgent}
  ├─ agent.planner.astar {iterations=5}
  └─ agent.action {agent.action.name=classifyIntent}
       └─ gen_ai.chat {gen_ai.system=openai}
```

---

## 阶段 M4：QoS、事件、并发（约 1-2 周）

**目标**：**生产可用**。

### 交付物
- [ ] `core.ActionQos` + `executeAction` 重试（指数退避）
- [ ] `core.ReplanRequest` + `errors.As` 传递
- [ ] `event.Event` 接口 + 完整事件层次
- [ ] `event.Multicast` 多播监听器
- [ ] 在关键路径发布事件（ReadyToPlan / PlanFormulated / ActionStart/Result / GoalAchieved / ProcessCompleted 等）
- [ ] `runtime.ConcurrentAgentProcess`（用 `errgroup` + `sync`）
- [ ] `StuckHandler` 接口 + 处理
- [ ] `EarlyTerminationPolicy`（Budget 触发、最大 action 数触发）
- [ ] `event.ObservationBridge` 自动桥接

### 验收
- 博客 agent 用 `ConcurrentProcess`，`research` 和 `outline` 并行，执行时间显著缩短
- 注入一个会失败 2 次的 action，验证重试成功到第 3 次
- 故意让 goal 永远不可达，验证 `Stuck` 状态 + `StuckHandler` 回调

---

## 阶段 M5：用户 API 二三种风格（约 1 周）

**目标**：**Struct+反射注册能用**。

### 交付物
- [ ] `agent/reflect/register.go` —— 扫描 struct 方法生成 Agent
- [ ] 支持约定：
  - `_ struct{} \`agent:"name=...,description=..."\`` 顶部标签
  - 方法签名识别 Action / Condition / AchievesGoal
- [ ] 完整文档 + 示例（同一 agent 两种写法对照）
- [ ] 性能测试：反射注册 vs DSL 的启动时间对比（应该都 < 10ms）

**Codegen（方式 C）**延到 M6 之后再做，看需求。

### 验收
`04-user-api.md` §B 的 `IntentAgent` struct 版本能正常跑，行为与 DSL 版本完全等价。

---

## 阶段 M6：HITL & 子 Agent（约 1-2 周）

**目标**：支持**人机交互**和**Agent 组合**。

### 交付物
- [ ] `agent/hitl/` 子包
  - `Awaitable` 接口
  - `ConfirmationRequest`
  - `FormRequest`
- [ ] `ProcessContext.AwaitInput(req)` 返回 `ActionWaiting`
- [ ] `Platform.ResumeProcess(id, input)` 恢复等待的进程
- [ ] `Platform.CreateChildProcess(...)` 子黑板继承
- [ ] Sub-agent 示例：一个 Agent 调用另一个 Agent 完成子任务

### 验收
一个订单审批 agent，在关键步骤 `AwaitInput(&ConfirmationRequest{...})`，外部 REST API 调用 `ResumeProcess` 提交用户决策。

---

## 阶段 M7：外挂扩展（按需）

**目标**：**生态完整度**。这些是独立顶层 module `agents/` 下的内容，**不强求同步做**。

### 可能的交付物
- [ ] `agents/a2a/` —— Agent-to-Agent 协议（参考 embabel-agent-a2a）
- [ ] `agents/mcp/` —— MCP 服务端（把 agent 暴露给 Claude Desktop 等）
- [ ] `agents/shell/` —— 交互式 REPL（参考 embabel-agent-shell，用 `charmbracelet/bubbletea` 或 `go-prompt`）
- [ ] `cmd/lynx-agent-gen/` —— 代码生成工具
- [ ] `agents/monitoring/` —— Prometheus exporter / Grafana dashboard JSON

每个子目录是独立 module，用户按需引入。

---

## 时间线总览

| 阶段 | 目标 | 周数 | 依赖 |
|-----|------|-----|-----|
| M0 | 骨架 | 0.5-1 | — |
| M1 | 核心原语 + 顺序执行 | 1-2 | M0 |
| M2 | GOAP A* | 1-2 | M1 |
| M3 | Lynx 集成 | 1 | M2 |
| M4 | QoS + 事件 + 并发 | 1-2 | M3 |
| M5 | 反射注册 | 1 | M4 |
| M6 | HITL + 子 Agent | 1-2 | M4 |
| M7 | 外挂（a2a/mcp/shell） | 视需求 | M4+ |

**核心路径 M0-M4 约 5-8 周**能出一个**MVP 可生产**的 Go agent 框架。

---

## 质量门槛（每阶段都要过）

### 代码质量
- [ ] `go vet ./...` 无告警
- [ ] 关键路径（Planner / Blackboard / AgentProcess）100% 单测覆盖
- [ ] `go test -race ./...` 无数据竞争
- [ ] `golangci-lint run` 主要 linter 通过

### 文档
- [ ] 每个公开类型有 godoc 注释
- [ ] `agent/README.md` 有快速上手指南
- [ ] `examples/` 目录有至少 3 个能跑的示例

### 依赖
- [ ] `agent/go.mod` 依赖面最小化：`core/` + `pkg/` + 少量标准库 + `golang.org/x/sync`
- [ ] 不引入任何 Spring AI 类框架风格的 DI / 反射框架（如 `dig` / `wire`）

---

## 风险与对策

| 风险 | 对策 |
|-----|------|
| GOAP A* 在大规模 action 集（>100 个）下超时 | M2 阶段加 benchmark + 调整 `maxIterations`；引入 utility planner 作 fallback |
| 反射注册（M5）与泛型构造器（M1）行为不一致 | 两条路径都走同一 Action 接口；共享单测用例 |
| 并发执行时黑板竞争 | `InMemoryBlackboard` 所有方法走 RWMutex；单测明确覆盖多 goroutine 同时写 |
| LLM 返回非预期格式导致 action 卡住 | 所有 LLM 调用走 Lynx 的 `StructuredParser` + 校验；失败计入重试 |
| embabel 后续演进带来新特性 | 以 M0-M4 的最小核心为准；新特性按功能价值评估是否吸收，不追 parity |

---

## 成功标准（MVP 完成时）

一位 Go 开发者能在 **30 分钟内**：
1. 定义一个新 agent（DSL 或反射方式）
2. 挂到 Lynx 的 chat client 上跑起来
3. 看到 observation 里的 span
4. 通过事件监听器拿到执行进度

并且这段代码**在生产环境能稳定运行**（有重试、有超时、有取消、有事件、有观测）。

这就是从 embabel 那里学到的、**值得移植到 Go 世界**的东西。

---

## 与 embabel 维护同步的策略

1. **不追 feature parity**。embabel 功能面很广（a2a / mcp / shell / onnx / code / shell / rag 多种实现）。Lynx 只做**核心 agent 运行时**，其他可以让用户**按需扩展**。
2. **关注架构级变更**。embabel 如果改 GOAP 算法、Blackboard 语义、事件模型这些**核心设计**，要同步评估。Lynx 保留 embabel 的**心智模型**，但 API 表面是 Go 风格、不必追 1:1。
3. **开源发布节奏**：M4 完成时发 v0.1.0 —— **核心工作都在这一版**，后续都是功能扩展。

---

**收尾感悟**：embabel 为 Spring AI 补上了最重要的一块拼图——agent 运行时。Lynx 已经有 Spring AI 核心能力的 Go 等价物，再加上这套 agent 框架，就是 Go 世界里**第一个完整的 GOAP-based 生产级 agent 平台**。这个定位值得认真做。
