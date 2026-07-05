# Lynx Agent Extension System Design

> **状态：已落地**。代码见 `agent/core/extension.go`、`agent/runtime/extension.go`、`agent/runtime/dispatch.go`、`agent/runtime/platform.go`、`agent/core/process_options.go`，测试见 `agent/runtime/extension_test.go`。
>
> 配套文档：[`./README.md`](./README.md) / [`./GUIDE.md`](./GUIDE.md) / [`./EMBABEL_DEEP_COMPARISON.md`](./EMBABEL_DEEP_COMPARISON.md)

---

## 0. TL;DR

| 维度 | 决策 |
|---|---|
| **风格** | `http.Pusher` 式 — 一个 base interface (`core.Extension`) + 多个 capability interface，框架内部 `type-assert` 检测 |
| **注册入口** | `PlatformConfig.Extensions []core.Extension` + `ProcessOptions.Extensions []core.Extension`，**无 `Use` 方法** |
| **生命周期** | platform-scoped（boot 期，永生）+ process-scoped（per-call，自动 GC），无显式 remove |
| **去重** | 按 `Extension.Name() string` 唯一性约束，重复在 platform 层 panic |
| **取消的方案** | ❌ functional options ❌ `Use(...)` 链式调用 ❌ `Drop(ext)` ❌ cancel func handle |
| **现有取代项** | listener / tool resolver / ID generator / planner / blackboard 等可插拔能力下沉为 `Extensions` 中的 capability。`*Factory` 包装层全部去除——`planning.Planner` / `core.Blackboard` 自己嵌入 `core.Extension`。 |
| **保留独立 API** | HITL `ResumeProcess(id, response any)`（类型化），命令式动作（`Deploy / RunAgent / KillProcess / ContinueProcess` 等），per-agent 策略（`core.Agent.StuckHandler`）|

---

## 1. 设计目标

1. **单一注册入口** — 所有可插拔的能力（事件订阅、动作 middleware、工具装饰、agent 校验、目标审批、resolver、ID 生成器、planner、blackboard 原型、chat client provider、提前终止策略）共用同一种注册方式。
2. **Go 风格** — 不引入 functional options、不引入注解扫描、不引入 DI 容器；仅靠 struct config + interface type assertion 完成。
3. **类型安全** — 不让用户直接面对 `any`；`Extension` 接口至少提供 `Name() string` 作为身份标识。
4. **作用域天然** — 平台级扩展终身存活；请求级扩展跟着 `AgentProcess` 走，process 退出自动 GC。
5. **可观测** — `Name()` 让 dispatch 链路上每一步都能知道"是哪个扩展在跑"。
6. **fail-fast** — boot 期重复名 / 空名 / nil → panic；不偷偷覆盖、不静默忽略。

---

## 2. 业界对照

| 框架 | 注册方式 | 移除支持 | 作用域 | 启发 |
|---|---|---|---|---|
| **embabel-agent** | Spring `@Bean`，DI 收集 | ❌ 无 | `@Bean` 终身 + `ProcessOptions.listeners` 请求级 | "boot-time wiring + per-process scope" 够用 |
| **pi-mono** | factory function，`pi.on(eventName, handler)` | ❌ 无（除按 name key 删 provider） | session 级，session 切换整把销毁 | 工厂内多次注册多种能力 |
| **net/http** | `http.Pusher` / `Flusher` 等 optional capability | N/A | request scope | 单接口 + capability assertion |
| **Spring `@Bean`** | DI container，重名 = 错误 | N/A | container scope | name-based dedup，重名 fail |

**结论**：三大先例都没有显式 remove，都靠"作用域生命周期"自然管理。lynx 跟随这个共识。

---

## 3. 核心抽象

### 3.1 Extension marker

```go
// agent/core/extension.go

// Extension is the marker every capability shares. Name gives each
// extension a stable identity used for de-duplication (重名 panic),
// logging (拦截链上能告诉你"audit 跑了 12ms"), and introspection
// (Platform.Extensions() 能列出所有已注册扩展).
type Extension interface {
    Name() string
}
```

### 3.2 Capability interfaces — 全部嵌套 `Extension`

按 dispatch 形态分三档：

#### A. Multi-instance fan-out（多实例展开）

| 接口 | 触发位置 | 链路语义 |
|---|---|---|
| `EventListener` | `event.Multicast.OnEvent` | FIFO，listener panic 隔离 |
| `ActionMiddleware` | `executeAction` 内，包 `pc.ExecuteSafely` | onion chain，首注册者最外层 |
| `ToolDecorator` | `pc.ActionTools` 解析后 | wrap chain，首注册者最内层 |
| `AgentValidator` | `Platform.Deploy` 内 | FIFO 串行，第一个 err 立即返回 |
| `GoalApprover` | `formulatePlan` 前过滤 `system.Goals` | FIFO，AND（任一 false 即否决）|

#### B. First-hit resolver（首位非 nil 赢）

| 接口 | 触发位置 | 语义 |
|---|---|---|
| `ToolGroupResolver` | `pc.ResolveTools` / `pc.ActionTools` 内部 | 按注册顺序问，首位返回非 nil tool group 的赢 |
| `ChatClientProvider` | `ProcessContext.Chat` / `ChatWithActionTools` 前 | process-scope 优先，首位返回非 nil client 的赢 |

#### C. Last-wins singleton（最后注册者赢，框架默认 fallback）

| 接口 | 触发位置 | 默认 fallback |
|---|---|---|
| `IDGenerator` | `Platform.createProcess` 生成 process id | UUID v4 |
| `planning.Planner`（按 `AgentConfig.PlannerName` dispatch） | `Platform.createProcess` 挑 planner | goap / reactive 内置 |
| `core.Blackboard`（prototype 模式 + `Spawn()`） | `Platform.createProcess` 生成 blackboard | in-memory |

### 3.3 完整签名

```go
// agent/core/extension.go

package core

import (
    "context"
)

type Extension interface {
    Name() string
}

// --- Multi-instance fan-out ---

type EventListener interface {
    Extension
    OnEvent(e event.Event) // EventListener lives in runtime to avoid core → event
}

type ActionMiddleware interface {
    Extension
    InterceptAction(
        ctx     context.Context,
        process Process,
        action  Action,
        next    func() ActionStatus,
    ) ActionStatus
}

type ToolDecorator interface {
    Extension
    DecorateTool(
        process Process,
        action  Action,
        tool    AgentTool,
    ) AgentTool
}

type AgentValidator interface {
    Extension
    ValidateAgent(agent *Agent) error
}

type GoalApprover interface {
    Extension
    ApproveGoal(process Process, goal *Goal) bool
}

// --- First-hit resolver ---

type ToolGroupResolver interface {
    Extension
    Resolve(ctx context.Context, requirement ToolGroupRequirement) (ToolGroup, error)
}

// --- Last-wins singleton ---

type IDGenerator interface {
    Extension
    Next() string
}

// planning.Planner（在 agent/planning 包）直接嵌入 core.Extension，无需 factory 包装。
// runtime 按 AgentConfig.PlannerName 匹配 Name() 来挑：
//
//   type Planner interface {
//       core.Extension
//       PlanToGoal(ctx, start, system, goal, options) (*Plan, error)
//   }
//
// 内置实现：goap.AStarPlanner ("goap")、reactive.Planner ("reactive")、
// htn.Planner ("htn")。

// core.Blackboard 直接嵌入 core.Extension，用 prototype 模式而非 factory：
//
//   type Blackboard interface {
//       Extension
//       BlackboardReader; BlackboardWriter
//       Spawn() Blackboard
//       Clear()
//   }
//
// runtime 在 createProcess 时调注册实例的 Spawn() 拿 per-process 实例；
// 注册实例本身作模板使用，不读不写。

// core.EarlyTerminationPolicy 也是 Extension 子接口（OR-composable）：
// 注册多个，runtime 每 tick 全问一遍，任意 true 即终止。
// Budget 从 ProcessOptions.Budget 隐式参与，无需显式注册。
```

---

## 4. 注册与作用域

### 4.1 Platform 级（boot 期，终身）

```go
// agent/runtime/platform.go

type PlatformConfig struct {
    // Extensions registered at platform construction. Lives for the
    // platform's lifetime; cannot be removed.
    //
    // Empty Name or duplicate Name panics — boot-time configuration is
    // expected to be deterministic and any conflict signals a real bug.
    Extensions []core.Extension
}

func NewPlatform(config PlatformConfig) *Platform
```

### 4.2 Process 级（per-call，自动 GC）

```go
// agent/core/process_options.go

type ProcessOptions struct {
    ...
    // Extensions live for one process's lifetime — they're merged with
    // platform extensions at AgentProcess creation, dispatch sees both,
    // and they fall out of scope when the process terminates.
    //
    // Process-level Name 可以与 platform-level Name 重复，且 process
    // 层覆盖 platform 层（合理的 scope override）；process 内部依然
    // 不允许重名。
    Extensions []core.Extension
    ...
}
```

### 4.3 没有 Use 方法

**理由**：
- struct config 一致性 — 整个 lynx 是 struct-config 风格；多一个 `Use` 是混搭
- 单一可发现入口 — IDE/godoc 一眼看完所有可配置项
- boot-time 注册足够 — embabel / pi-mono 都验证过了
- 想做"按条件注册"用 slice 拼装即可：
  ```go
  exts := []core.Extension{baseExt}
  if debug { exts = append(exts, debugExt) }
  platform := agent.NewPlatform(runtime.PlatformConfig{Extensions: exts})
  ```

### 4.4 没有 Drop / 移除

**理由**：embabel / pi-mono / Spring 都没有，三大先例形成共识。

少数边缘场景：
- 测试 cleanup → 创建 fresh `Platform` 实例（Go 习惯，类比 `httptest.NewServer`）
- 动态 toggle → 用户用 wrap pattern（atomic flag 包一层）；rare case 不污染 API
- 命名实体的"删除" → 通过现有 `Platform.Undeploy(name)` / `RemoveProcess(id)` 等命令式 API

---

## 5. Dispatch 语义详解

### 5.1 Onion chain（`ActionMiddleware`）

```go
func (p *Platform) interceptAction(ctx, proc, action, base) core.ActionStatus {
    var run func(i int) core.ActionStatus
    run = func(i int) core.ActionStatus {
        for ; i < len(extensions); i++ {
            if h, ok := extensions[i].(core.ActionMiddleware); ok {
                return h.InterceptAction(ctx, proc, action, func() core.ActionStatus {
                    return run(i + 1)
                })
            }
        }
        return base()
    }
    return run(0)
}
```

- 首注册者最外层（先 enter / 后 exit）
- `next()` 不调即短路（业务逻辑可主动跳过）
- 拦截器自身 panic 由 framework `recover()` 兜底，转为 `ActionFailed` + `LastError`

### 5.2 Wrap chain（`ToolDecorator`）

```go
func (p *Platform) decorateTool(proc, action, tool) core.AgentTool {
    for _, ext := range extensions {
        if d, ok := ext.(core.ToolDecorator); ok {
            tool = d.DecorateTool(proc, action, tool)
        }
    }
    return tool
}
```

- 首注册者最内层（先包 → 被后续装饰器再包）
- 返回 nil tool 视作"丢弃"（按 zero-value AgentTool 处理或跳过）

### 5.3 Validator chain（fail-fast）

```go
for _, ext := range extensions {
    if v, ok := ext.(core.AgentValidator); ok {
        if err := v.ValidateAgent(agent); err != nil {
            return fmt.Errorf("agent validation by %q: %w", v.Name(), err)
        }
    }
}
```

- FIFO 串行
- 第一个 err 立即返回，错误消息带上 validator 的 Name

### 5.4 Approver chain（AND）

```go
for _, ext := range extensions {
    if a, ok := ext.(core.GoalApprover); ok {
        if !a.ApproveGoal(proc, goal) { return false }
    }
}
return true
```

- 任一 approver 返回 false → 整体否决
- 否决后该 goal 从 planner 候选集中过滤

### 5.5 First-hit resolver（`ToolGroupResolver`）

```go
for _, ext := range extensions {
    if r, ok := ext.(core.ToolGroupResolver); ok {
        group, err := r.Resolve(ctx, req)
        if err != nil { return nil, err }
        if group != nil { return group, nil }
    }
}
return nil, nil
```

### 5.6 Last-wins singleton（`IDGenerator` 等）

```go
func (p *Platform) idGen() core.IDGenerator {
    for i := len(p.extensions) - 1; i >= 0; i-- {
        if g, ok := p.extensions[i].(core.IDGenerator); ok {
            return g
        }
    }
    return defaultIDGenerator  // 框架内置 UUID
}
```

- 反向扫描，最后注册的扩展获胜
- 没注册任何 IDGenerator → 回落框架默认

### 5.7 Process-level 与 Platform-level 合流

```go
func (p *AgentProcess) effectiveExtensions() []core.Extension {
    // process extensions 后 append → 在 onion chain 里"更内层"
    return append(append([]core.Extension(nil), p.platform.extensions...), p.options.Extensions...)
}
```

具体顺序约定：
- **Onion chain (Interceptor)**：platform 先 enter，process 内层。例如外层加 tracing，内层加请求级业务上下文
- **Wrap chain (Decorator)**：platform 先包 → process 后包，process 装饰在 platform 之上
- **Validator**：platform 先验，process 后验（process 一般不会在请求时新增 validator，但允许）
- **Approver**：两层 AND（任一层任一 approver 否决均否决）
- **Resolver / Singleton**：process > platform > 默认（process 优先级最高）

---

## 6. 错误处理 / 边界

| 情形 | 行为 |
|---|---|
| `Extension.Name()` 返回空串 | `NewPlatform` 时 panic |
| `PlatformConfig.Extensions` 内重名 | `NewPlatform` 时 panic（boot-time fail-fast）|
| `nil` 出现在 Extensions slice | `NewPlatform` 时 panic |
| `ProcessOptions.Extensions` 内 process 层重名 | `RunAgent / StartAgent / ContinueProcess` 时返回 error |
| `ProcessOptions.Extensions` 与 platform 同名 | **允许** — process 层覆盖 platform 层（scope override）|
| Interceptor / Decorator panic | framework `recover()`，转 ActionFailed，错误消息附扩展 Name |
| Validator panic | 转 `error`，Deploy 返回 |
| Resolver panic | 视作返回 error，链中断 |
| Listener panic | 已有 `safeDeliver` 兜底，不影响其他 listener |

---

## 7. 使用范例

### 7.1 单扩展实现多 capability

```go
type ObservabilityExt struct {
    logger *slog.Logger
    meter  metric.Meter
}

func (*ObservabilityExt) Name() string { return "observability" }

// EventListener
func (o *ObservabilityExt) OnEvent(e event.Event) {
    o.logger.Debug("event", "type", e.EventName(), "process", e.ProcessID())
}

// ActionMiddleware
func (o *ObservabilityExt) InterceptAction(ctx context.Context, p core.Process, a core.Action, next func() core.ActionStatus) core.ActionStatus {
    start := time.Now()
    status := next()
    o.meter.Histogram("action_duration").Record(ctx, time.Since(start).Seconds(),
        attribute.String("action", a.Metadata().Name),
        attribute.String("status", status.String()),
    )
    return status
}

// ToolDecorator
func (o *ObservabilityExt) DecorateTool(p core.Process, a core.Action, t core.AgentTool) core.AgentTool {
    inner := t.Call
    t.Call = func(ctx context.Context, args string) (string, error) {
        start := time.Now()
        result, err := inner(ctx, args)
        o.logger.Info("tool", "name", t.Name, "took", time.Since(start), "err", err)
        return result, err
    }
    return t
}

// 一行注册 → 三种能力同时启用
platform := agent.NewPlatform(runtime.PlatformConfig{
    Extensions: []core.Extension{
        &ObservabilityExt{logger: slog.Default(), meter: meter},
    },
})
```

### 7.2 多扩展协作

```go
platform := agent.NewPlatform(runtime.PlatformConfig{
    Extensions: []core.Extension{
        &ObservabilityExt{...},          // tracing + metrics + audit
        &SecurityExt{...},               // ActionMiddleware 包 SecurityContext
        &MCPResolver{addr: "..."},       // ToolGroupResolver
        snowflake.New(nodeID),           // IDGenerator (override default UUID)
        &SLAValidator{minTimeout: 10*time.Second}, // AgentValidator
    },
})

// 部署
platform.Deploy(myAgent)

// per-request 扩展
platform.RunAgent(ctx, myAgent, bindings, core.ProcessOptions{
    Extensions: []core.Extension{
        &TenantGate{user: currentUser},   // GoalApprover
        &DebugListener{traceID: trace},   // EventListener
    },
})
```

### 7.3 同种扩展多实例（按 Name 区分）

```go
platform := agent.NewPlatform(runtime.PlatformConfig{
    Extensions: []core.Extension{
        NewTimingDecorator("decorator:openai"),     // 一个 ToolDecorator 实例
        NewTimingDecorator("decorator:anthropic"),  // 同 struct，不同 Name → OK
    },
})
```

---

## 8. 取消的设计选择 — Rationale

### 8.1 为何不要 `Use(ext...)` 链式方法

- struct config 一致性更重要
- IDE/godoc 单点可发现性 > 调用风格自由度
- "动态注册"用 slice 拼装即可
- embabel / pi-mono 都没有

### 8.2 为何不要 `Drop(ext)` / cancel func

- 三大先例都没有
- 真正动态的需求由 process 级扩展（自动 GC）覆盖
- 测试用 fresh Platform；toggle 用 wrap pattern
- pointer-identity remove 在 Go slice 里有重新分配陷阱

### 8.3 为何不要 functional options

- 用户明确表态过"我推崇现在这种 [struct config]"
- 字段集中管理 + applyDefaults 比 ~10 个 `WithXxx` 更克制

### 8.4 为何 `Extension` 接口仅一个方法

- 最小 surface
- `Name()` 单独承担身份职责，capability 接口承担行为职责，关注点分离
- 不强迫所有扩展实现"生命周期方法"等多余 API

### 8.5 为何重复 Name → panic 而非软警告

- boot-time 配置错误必须看见
- 软警告容易在 production 被滤掉
- panic 强制用户立即修，不会带 bug 上线

### 8.6 为何 platform 内部不暴露 `Use` 即使内部需要批量注册

- 内部用方法 `registerExtension(ext)`（不导出）
- 外部入口只有 `PlatformConfig.Extensions` 一个

---

## 9. 实现规划

### 9.1 文件改动

| 文件 | 动作 | 行数估计 |
|---|---|---|
| `agent/core/extension.go` | **新增** — Extension marker + 9 capability 接口 | ~100 |
| `agent/core/process_options.go` | 新增 `Extensions []Extension` 字段 | +5 |
| `agent/runtime/platform.go` | 改造 PlatformConfig 缩瘦为 `Extensions []Extension`；新增内部 dispatch helpers (`idGen / plannerFactory / blackboardFactory / interceptAction / decorateTool / resolveToolGroup / validateAgent / approveGoal`) | +120 / -60 |
| `agent/runtime/extension_dispatch.go` | **新增** — 集中所有 dispatch 链实现 | ~150 |
| `agent/runtime/execute_action.go` | 接入 ActionMiddleware 链 | +15 |
| `agent/core/process_context.go` | 接入 ToolDecorator 链 + Resolve chain | +25 |
| `agent/runtime/run.go` | `formulatePlan` 接 GoalApprover 过滤 | +12 |
| `agent/runtime/extension_test.go` | **新增** — 全面测试 | ~250 |
| `agent/examples/*` | 更新到新 API | ~20 |

### 9.2 删除清单

- `PlatformConfig.Listeners []event.Listener` → 用 EventListener 扩展替代
- `PlatformConfig.Tools core.ToolGroupResolver` → 用 ToolGroupResolver 扩展替代
- `PlatformConfig.IDGenerator core.IDGenerator` → 用 IDGenerator 扩展替代
- `PlatformConfig.PlannerFactory PlannerFactory` → 直接注册 `planning.Planner` extension，按 name dispatch
- `PlatformConfig.Services *core.ServiceProvider` → 内部自建，外露 `Platform.Services()` 直接使用
- `Platform.AddListener / RemoveListener` → 用 EventListener 扩展替代

### 9.3 保留清单

- `Platform.Deploy / Undeploy / RunAgent / StartAgent / ContinueProcess / ContinueProcessAsync / KillProcess / ResumeProcess / RemoveProcess / PruneTerminalProcesses` — 命令式动作，不是扩展
- `Platform.GetProcess / Agents / FindAgent / ActiveProcesses / Services()` — 查询/访问 API
- `core.Agent.StuckHandler` — per-agent 策略，不是平台级扩展
- `ProcessOptions.Blackboard` — per-call 覆盖比 prototype 注册更高优先级（依然保留，直接传实例而非 Spawn）
- `event.Multicast / event.Listener / event.ListenerFunc` — 内部加速 + 公开适配器，仍然存在

### 9.4 总改动

约 **+400 / -100 LOC**，净增 ~300 行（含测试 250 行）。API 表面减少 6 个字段 + 2 个方法。

---

## 10. 决策点（已锁定）

| 问题 | 决策 | 落地位置 |
|---|---|---|
| `Extension` 接口定义在 `core` 还是 `runtime`？ | **`core`** | `agent/core/extension.go` |
| `Name()` 还是 `ID()`？ | **`Name()`** | `core.Extension.Name() string` |
| 重复 Name 行为？ | platform 层 **panic**（boot-time fail-fast）；process 层**返回 error**（runtime configurations） | `runtime.extensionRegistry.register` panics; `Platform.createProcess` validates and returns error |
| `Planner` 怎么避免 core → planning 反向依赖？ | **Planner 接口留在 `planning` 包**，嵌入 `core.Extension`；agent 选 planner 用 `PlannerName string` 而非直接持有 planner 实例 | `agent/planning/planner.go` |
| EventListener 接口归属？ | **`runtime`**（依赖 `event.Event`；core 不感知 event 包） | `agent/runtime/extension.go`，satisfies `event.Listener` so it slots into `event.Multicast` directly |
| Per-process EventListener 范围？ | 仅看本 process 的事件（`AgentProcess.processEvents` 多播）；不看跨 process / platform 级事件 | `runtime/agent_process.go::publishEvent` 双写两 multicast |
| Per-process Singleton（IDGenerator / Planner / Blackboard）？ | **支持** —— 注册到 `ProcessOptions.Extensions` 即 override platform-scope。Planner 还按 Name() 分组（同名 process-scope 胜出） | `Platform.resolvePlanner` / `lastExtension[T]` |
| Per-process AgentValidator？ | **不支持** —— validator 在 Deploy 期触发，process 不存在 | `Platform.Deploy` reads `p.extensions` only |

---

## 11. 后续迭代方向（不在 v1 范围）

- `core.GoalChoiceApprover` 的"日志/事件"友好版本（让 audit 知道哪个 approver 拒绝了）
- `core.AwaitDecider` —— HITL 自动批准/跳过（embabel `AwaitDecider` 对应物，等真实需求出现再加）
- `Platform.Extensions() []core.Extension` 内省 API（按 Name 查询）
- `Extension.Description() string`（可选）让 `/help` 之类的工具能列扩展能力
- 单元测试中可注入 mock `IDGenerator` / `planning.Planner` 时的便利封装
