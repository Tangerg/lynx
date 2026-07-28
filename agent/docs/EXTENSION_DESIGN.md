# Agent Extension 设计

本文定义当前公开 SPI 的装配、顺序和所有权规则。签名以 GoDoc 与 exported API baseline 为准。

## 1. 单一发现机制

所有可插拔行为先实现：

```go
type Extension interface {
    Name() string
}
```

同一个普通 Go 值可以再实现一个或多个 capability interface。Engine 不维护字符串到行为的
map，也不扫描类型标签；Runtime 使用 type assertion 收集能力，方式类似
`http.ResponseWriter` 与可选的 `http.Pusher`。

`Name` 只用于同一作用域去重、诊断和观测归因。nil、typed nil、空名字或同一作用域重复名字
都是配置错误，Engine 构造或 Process 启动会 fail fast。一个实例可能被多个 Process 并发
调用；可变状态的同步由实现负责，Runtime 不为扩展调用增加串行、重试或分布式协调。

## 2. 作用域与合并

- Engine scope：`runtime.Config.Extensions`，对该 Engine 创建的 Process 生效；
- Process scope：`core.ProcessOptions.Extensions`，只接受执行期能力并只对当前 Process 生效。

`AgentValidator`、`IDGenerator` 和 Blackboard prototype 只属于 Engine scope；单个 Process 的
Blackboard 通过 `ProcessOptions.Blackboard` 显式提供。无当前作用域能力的值会在构造边界报错，
不会作为静默 no-op 留在 registry。

多值能力按声明顺序组合；singleton 能力取最近的匹配项；Planner 通过
`AgentConfig.PlannerName` 与 extension name 精确选择。Process scope 的选择型能力优先于
Engine scope，但同名并不是跨作用域冲突，而是显式 override。任何规则都不能依赖 map
遍历顺序。

## 3. Capability 语义

| Capability | Scope | 分发语义 | 所有权 |
|---|---|---|---|
| `planning.Planner` | Engine / Process | 按 Agent 的 PlannerName 选择一个 | 规划算法 |
| `core.ActionMiddleware` | Engine / Process | onion chain；先注册者最外层 | Action 横切行为 |
| `core.ToolMiddleware` | Engine / Process | wrap chain；先注册者最内层 | Tool 横切行为 |
| `core.AgentValidator` | Engine | 全部执行并聚合错误 | deploy-time 验证 |
| `core.GoalApprover` | Engine / Process | 全部同意才保留 Goal | Goal 选择策略 |
| `core.ToolGroupResolver` | Engine / Process | 第一个 `ok=true` 的 resolver 获胜 | role 到工具组解析 |
| `core.ChatProvider` | Engine / Process | Process 优先，首个非 nil Model 获胜 | 每进程模型选择 |
| `core.StopPolicy` | Engine / Process | 任一返回 stop 即终止 | tick 边界策略 |
| `core.IDGenerator` | Engine | 最近注册者获胜 | Process ID |
| `core.Blackboard` | Engine | 最近注册的 prototype 获胜，`Clone` 新实例 | Process 状态 |
| `runtime.EventListener` | Engine / Process | Engine scope 观察全部 Process；Process scope 仅观察本 Process | 局部观测与投影 |
| `runtime.SubtreeEventListener` | Process | 显式沿委派子树传播 | 子树观测与投影 |

默认 Chat、`ChatMiddleware`、Prompt tool-round limit 和 child-depth limit 是
`runtime.Config` 的稳定执行配置，不因为“可替换”就进入 Extension registry。动态领域依赖
使用 typed `core.Dependencies`，也不伪装成行为扩展。

存储、事务、产品 Session、幂等、重试和 snapshot 提交策略不属于 Config 或 Extension。
Agent 只生产和消费 `ProcessSnapshot` / `ProcessSnapshotTree` 值；Host 在自己的消费边界定义
最小存储接口。

## 4. Middleware 边界

`ActionMiddleware.RunAction` 包围单次 Action 调用，但只接收不可执行的
`ActionDescriptor`；唯一执行权是 `next`。`next` 与 middleware 都返回
`(ActionStatus, error)`：status 表达 Waiting/Paused 等生命周期结果，error 表达失败或
replan。Middleware 不得自行运行 Planner，也不能把错误改藏到 context 或 Blackboard。

`GoalApprover` 接收 `GoalDescriptor`，`ProcessView.Goal` 也返回该值；审批与观察代码不能调用
Goal score policy。完整 `*Goal` 仅留在 Planner 和 Runtime 的受信执行路径。

`ToolMiddleware.WrapTool` 只包装已解析的 `tools.Tool`。鉴权、redaction、tracing 和明确的
调用级 retry 可以放在这里；模型轮次、HITL、usage、checkpoint 与 tool-loop 终止属于
framework-managed interaction。包装器只需声明 `tools.WrappingTool`（`Unwrap`），被包装
工具声明的全部可选能力即自动保留 —— 逐个重新实现只会漏掉忘记的那个；要刻意收窄调度或
continuation 语义，就由包装器自己声明该能力（外层优先），或不声明 `Unwrap` 完全隐藏。

Chat 横切行为直接使用 `core/chat.CallMiddleware` 与 `StreamMiddleware`，由 Runtime 在
选定 ChatCapability 后组合。不要再为同一调用边界增加 Advisor/Hook/Interceptor 的平行链。

## 5. ToolGroupResolver

Action 通过 `ActionConfig.ToolGroups []string` 只声明抽象 role。
`ToolGroupResolver.Resolve(ctx, role)` 返回 `ToolGroup`；
Runtime 在调用 `Tools` 前校验：

1. group 非 nil；
2. 返回工具均非 nil 且定义有效。

Resolver 与 ToolGroup 的实现属于装配层：静态列表、远程 registry、plugin catalog 或 MCP
session 各自决定发现、缓存、重试、并发和连接生命周期。Framework 不提供带 map 或
`sync.Once` 策略的默认实现。权限、沙箱、审批和产品策略由 Host 选择 resolver、筛选工具或
安装 `ToolMiddleware` 时实现；这些策略不进入 Agent 声明与 deployment digest。可执行 Tool
不进入 provider wire DTO。

## 6. ChatProvider

`ChatProvider.Chat` 返回 provider-neutral `core.ChatCapability`：

- `Model` 为 nil 表示 defer 到下一个 provider；
- `Streamer` 可选，但不能在没有 Model 时单独返回；
- 全部 defer 时回退到 `runtime.Config.Chat`；
- Runtime 只组合 Host 提供的 ChatMiddleware；产品 conversation 的 context 投影、history store
  与 middleware 选择完全属于 Host。middleware 可从 context 的 `core.ProcessViewFrom` 取得当前
  进程标识。

Provider adapter 只需实现 `core/chat.Model` 和可选 `Streamer`，不应依赖 Agent、Blackboard、
history 或 Extension registry。

## 7. 新增 capability 的门槛

新增公开接口前必须同时满足：

1. 至少有两个独立实现，或存在明确的库外实现需求；
2. Runtime 有稳定且唯一的分发边界；
3. 普通 Action、middleware、workflow、config 字段或函数参数无法清楚表达；
4. 接口由消费方定义，并保持最小；
5. 测试可锁定顺序、scope、nil、错误和并发语义。

只为单一内部实现造接口、使用字符串注册行为、把 SDK client 放进协议 DTO，或创建与现有
middleware 重叠的扩展链，都不接受。

## 8. 外部状态边界

需要恢复能力的 Host 应先从自己的 backend 加载完整树，再调用
`Engine.ValidateRestoreTree` / `Engine.RestoreTree`。捕获路径调用 `Engine.SnapshotTree` 后，
由 Host 决定是否以及如何提交。Agent 不提供 store conformance package，因为接口、事务和失败
策略都应由真实消费方拥有并测试。

ChatProvider 与 ToolGroupResolver 的 shape 会在真实 dispatch 边界 fail closed 校验；目前不
额外暴露只为测试而存在的 provider contract package。
