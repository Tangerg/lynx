# Agent Extension 设计

本文定义 `agent` 当前公开 SPI 的使用规则。它不是未来功能清单；符号签名以 GoDoc
和编译器为准。

## 1. 单一发现机制

所有可插拔能力都先实现：

```go
type Extension interface {
    Name() string
}
```

同一个值可以再实现一个或多个 capability interface。Platform 不维护 string-keyed
handler map，也不要求注册类型标签；runtime 使用普通 type assertion 收集能力，做法与
`http.ResponseWriter`/`http.Pusher` 相同。

`Name` 用于同一注册作用域内去重和错误归因。nil、空名字或重复名字属于启动配置错误，
Platform 构造时 fail fast。

## 2. 注册作用域

- Platform scope：`runtime.PlatformConfig.Extensions`，对所有进程生效。
- Process scope：`core.ProcessOptions.Extensions`，只对当前进程生效。

多值能力按确定的注册顺序合并；singleton 能力由 runtime 选择最近的匹配项；planner
按 `AgentConfig.PlannerName` 与 extension name 精确解析。不要依赖 map 遍历顺序。

## 3. Capability 规则

| Capability | 分发语义 | 主要所有权 |
|---|---|---|
| `planning.Planner` | 按名字选择一个 | 规划算法 |
| `core.ActionMiddleware` | onion chain；先注册者最外层 | action 横切行为 |
| `core.ToolDecorator` | 依注册顺序包装；先注册者最内层 | tool 横切行为 |
| `core.AgentValidator` | 全部运行并聚合错误 | deploy-time 验证 |
| `core.GoalApprover` | 全部同意才保留 goal | goal 选择策略 |
| `core.ToolGroupResolver` | 第一个 `ok=true` 的 resolver 获胜 | role 到 tools 的解析 |
| `core.ChatClientProvider` | process scope 优先，首个非 nil client 获胜 | 每进程 LLM 选择 |
| `core.EarlyTerminationPolicy` | 任一 policy 请求终止即终止 | tick 边界策略 |
| `core.IDGenerator` | 最近注册的实现获胜 | process ID |
| `core.Blackboard` | 最近注册的 prototype 获胜，`Spawn` 新实例 | 进程状态存储 |
| `runtime.EventListener` | 全部订阅者接收事件 | 观测与投影 |

`core.ProcessStore`、`core.SessionStore`、`chatclient.Client` 和 Guardrails 是明确的
Platform 配置依赖，不因为“可替换”就都塞进 extension registry。

## 4. Middleware 与 decorator

`ActionMiddleware` 包围单个 action 执行，可做审计、租户上下文、计时或明确的
short-circuit。它必须通过 `next` 保留 action 的状态机语义，不能私自运行 planner。

`ToolDecorator` 只包装 `tools.Tool`。鉴权、redaction、tracing 和调用级 retry 可在这里
实现；模型轮次、HITL 和 tool-loop 终止属于 `agent/toolloop`/host runtime，不属于
typed tool adapter。

Chat 的横切行为使用 `core/chat.CallMiddleware` 与 `StreamMiddleware`，由
`chatclient.Client` 组合。不要再为同一边界增加 Advisor/Hook/Interceptor 的第二条链。

## 5. ToolGroupResolver 与权限

Agent action 声明 `ToolGroupRequirement`，只表达需要的 role 和允许的权限。Resolver
返回带 metadata 的 `ToolGroup`；runtime 验证 group 声明的权限没有超过 requirement，
然后才加载工具。调用方可使用 `core.StaticToolGroupResolver` 和
`core.NewLazyToolGroup`，大型部署可实现远程 registry resolver。

ToolGroup 最终返回 `[]tools.Tool`。模型 request 只接收这些工具的
`core/chat.ToolDefinition`；可执行对象不会进入 wire DTO。

## 6. 每进程模型选择

`core.ChatClientProvider` 返回当前 process 使用的 `*chatclient.Client`。Process scope
provider 先于 Platform scope provider，全部返回 nil 时回退到
`PlatformConfig.ChatClient`。

这个 SPI 解决“一个 Platform 服务多个 provider/model”的装配问题。provider adapter
仍只实现 `core/chat.Model`；它不需要知道 Agent、Blackboard 或 extension registry。

## 7. 新增 capability 的门槛

新增接口前必须同时满足：

1. 至少有两个独立实现或明确的库外实现需求；
2. runtime 有稳定且唯一的分发边界；
3. 普通 action、middleware、decorator、workflow 或函数参数无法清晰表达；
4. 接口可保持最小，调用方不需要伪造不支持的方法；
5. 测试能锁定顺序、scope、nil、错误和并发语义。

只为单一内部实现增加接口、用字符串注册行为、把 SDK client 塞进协议 DTO，或新建与
现有 middleware 重叠的扩展链，都不接受。
