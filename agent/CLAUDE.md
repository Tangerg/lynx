# CLAUDE.md — agent module

> Goal-oriented agent runtime — planner / world-state / blackboard / HITL / 多种 planning 算法.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

把"agent 定义"（goals + actions + conditions）编译成"可观察可暂停可恢复的运行进程"。**Planner-driven 而非 ReAct-loop**：每个 tick 让 planner 看世界状态 + goal，产出下一步 action plan。Lyra 后端用它跑 chat turn 的工具循环。

## 技术栈

- Go 1.26.3
- 内部依赖：`core` / `mcp` / `pkg`
- 外部依赖：
  - `modelcontextprotocol/go-sdk` —— MCP 集成
  - `google/uuid`
  - `Masterminds/semver/v3`
  - `go.opentelemetry.io/otel` —— trace 注入
  - `golang.org/x/sync` —— WaitGroup pattern
- ~13k LOC / 112 文件 / 18 个子目录（不计 examples）

## 核心架构（三大支柱）

1. **`core/` —— 原语层**
   - `Action` / `TypedAction[In, Out]`（generics）/ `Agent` / `Goal` / `Condition` / `Blackboard` / `Process` / `Extension`
   - **`Determination` 三值逻辑**（Unknown / True / False）—— 条件可以是"还不知道"，不是 nil bool

2. **`runtime/` —— 执行引擎**
   - `Platform` —— agent registry + process 生命周期
   - `AgentProcess` —— 状态机 tick 循环（plan → observe → act）
   - 并发 child spawn / event multicast / HITL resume API
   - 通过 `collectExtensions[T any](extensions []Extension)` 做**类型分发** —— 加新能力（middleware / decorator / validator / approver）就实现对应 interface，不改 dispatch loop

3. **`service` 切片**
   - `planning/` —— Planner 接口 + WorldState；具体算法在 `planning/planner/{goap,htn,reactive,utility}` 各包
   - `workflow/` —— 高阶 builder（Sequence / Loop / Parallel / ScatterGather / RepeatUntil / Consensus）—— 都产出普通 GOAP agent
   - `event/` —— lifecycle 事件 + 多播 listener
   - `hitl/` —— typed `Awaitable[T]`，把 process 挂在 StatusWaiting，operator 回复后恢复
   - `toolpolicy/` —— chat tool 装饰器（OnceOnly / Unlocked 等 policy）

## 关键接口/类型

1. **`Action`** + `NewAction[In, Out](name, fn, config)` —— 最小 planning 单位；框架按 name+type 从 blackboard 自动绑入参，写回 output
2. **`Agent`** —— deployable bundle（config + lazy condition cache），fluent Builder 构造
3. **`Process`** —— action 看到的 read surface（ID / Status / Goal / Blackboard / LastWorldState）+ control（TerminateAgent / AwaitInput / RecordUsage）
4. **`Platform`** —— registry + factory + HITL resume；config-time + per-process Extensions 在 dispatch 期合并
5. **`Blackboard` (Reader / Writer)** —— 按 `name + type` 查（`"it"` = 最新该类型实例）；`condition` 走 bool；`BindProtected` 跨子进程传递
6. **`Extension`** —— marker；7 个能力子接口（ActionMiddleware / ToolDecorator / AgentValidator / GoalApprover / ToolGroupResolver / IDGenerator / Blackboard/Planner factory）
7. **`Determination`** —— Unknown / True / False，自带 And / Or / Not，零值是 Unknown（uninit 友好）

## 强约定

- **Blackboard 读写边界**：观察方拿 `BlackboardReader`，修改方拿 `BlackboardWriter`，实现自动两者都满足
- **Typed action 优先**：`NewAction[In, Out]` 一定优先于 untyped Action，框架自动 in/out 绑定
- **Extension Name 唯一性**：Platform.New 在 nil / empty / 重名时**直接 panic** —— deploy-time 暴露问题
- **Action QoS retry 在 runtime 层**：`ActionQoS{MaxAttempts, BaseDelay, MaxDelay}` 委托 `pkg/retry`；不要每个 action 自己写重试
- **HITL 是 first-class**：`AwaitInput` 把进程切 StatusWaiting，状态存 blackboard，`Resume` 拿到回复后重入
- **Event JSON 单向**：lifecycle event 的 marshal 会把 Action / WorldState 这种接口字段降级成 lossy summary；要 round-trip 在内存里直接 type-assert
- **依赖窄接口（已经踩过坑）**：`autonomy.platform` 接口就是这个 pattern —— 上层别 import `*runtime.Platform` 整体，定义自己的窄 interface

## 关键目录

```
agent/
├── core/              原语：Action / Agent / Goal / Condition / Blackboard / Process / Extension / Determination
├── runtime/           Platform + AgentProcess tick 循环 + dispatch
│   └── autonomy/      Ranker-based goal 选择
├── planning/          Planner 接口 + WorldState
│   └── planner/{goap,htn,reactive,utility}/   具体 planning 算法
├── event/             Lifecycle event + Multicast listener + JSON marshaler
├── workflow/          高阶 builder（Sequence / Loop / Parallel / Consensus...）
├── hitl/              Awaitable[T] + 暂停 / 恢复
├── toolpolicy/        chat tool 装饰器（OnceOnly / Unlocked）
└── examples/          demo（**不在 production 路径上，refactor 时 ignore**）
```

## 常用命令

```bash
go build ./...
go vet ./...
go test ./...
go test ./runtime/... -race    # 并发 + child spawn 必跑 race
```

## 修改任何东西之前

- **改 `Extension` 子接口签名**：`collectExtensions[T]` 类型分发会断；每个实现方都受影响
- **改 `Process` 接口**：所有 action 函数都拿它；改了得扫全 agent + lyra 业务代码
- **改 `Blackboard` 的 name+type 协议**：框架的自动绑入参靠它
- **加新 planner**：放 `planning/planner/<name>/`，实现 Planner 接口；不要改 runtime
- **加新 workflow builder**：放 `workflow/<name>.go`，输出普通 GOAP agent

## 强反向不变量

- ❌ **绕过 Blackboard 让 action 之间直接传值**：违反 planner 可见性，调度会坏
- ❌ **Extension 用 string-key 注册（非类型分发）**：`collectExtensions[T]` 现在的 pattern 更好，加新能力时不改 dispatch loop
- ❌ **新模块直接拿 `*runtime.Platform` 整体**：定义自己的窄接口（参考 `runtime/autonomy/platform.go` interface）
- ❌ **examples/ 里的代码当 reference**：那是 demo，约定不一定跟主线一致

## 已经做过的大重构（lyra 重构 session 期）

- ✅ `autonomy.Autonomy` 解耦 `*runtime.Platform`，改用包内 `platform` 窄接口
- ✅ `ProcessContextConfig` / `ProcessContext` 字段按 concern 分区（per-process state / platform-wired hooks / per-action state）
- ✅ `fmt.Errorf("constant")` → `errors.New(...)` 全模块清扫
