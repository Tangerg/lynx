# Lyra 的端口与可替换性边界

Lyra 使用接口有两个理由：

1. 内层消费者需要定义一片稳定能力面，隔离外层技术实现；
2. 产品确实允许组合根注入另一种策略或后端。

Run 生命周期、Agent process 装配、turn 状态机、协议 dispatch 和资源关闭顺序是产品
机制，不因为测试方便就抽象成插件。整体分层见
[`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md)。

## 1. 判断准则

新增 interface 前依次判断：

- 谁是真正消费者？接口必须定义在该 package，而不是实现方或“大公共 ports”目录；
- 消费者是否只需要一个小而稳定的语义面？
- 边界是否跨架构环，或是否存在真实可注入的第二策略？
- 参数是否只包含 domain/application 语义，而没有 SDK、transport 或 persistence 细节？

仅包装一个具体实现、只制造底层不可能状态、或只是为了 mock 的接口应删除。包内实现
细节优先使用具体类型；跨包但不需公开替换的接口保持 unexported。

## 2. 当前端口

| 能力 | 消费方端口 | 默认实现/来源 |
|---|---|---|
| 长期知识 | `application/workspace.KnowledgeStore`（写/列表）与 `adapter/agentexec.KnowledgeReader`（提示词读取） | `infra/storage.FileKnowledgeStore` |
| 自治目标 | `application/goals.Store` 与 `application/goals.State` | SQLite goal store |
| 计划任务 | `application/schedules.ManagementStore`、`RunNowStore`、`WorkerStore` | SQLite schedule store |
| 待办 | `todotool.Store`、`agentexec.TodoReader`、session cleanup port | SQLite todo store |
| turn steering | `adapter/agentexec/turn.SteeringSink` | conversation/message adapter |
| 上下文压缩 | `adapter/agentexec/turn.Compactor` | `adapter/maintenance.Compactor` |
| 事实提取 | `adapter/agentexec/turn.Extractor` | `adapter/maintenance.Extractor` |
| utility chat model validation | `application/models.ChatModelValidator` | `adapter/modelclient` + `infra/llm` |
| Chat provider | `core/chat.Model` / optional `Streamer` | provider adapters |
| Chat history | `chathistory.Store` | SQLite message store |
| Tool capability | `tools.Tool` + Agent `core.ToolGroupResolver` | `adapter/toolset` |
| MCP 状态/目录/连接/注册表 | `application/integrations.MCP*` 四片端口 | `adapter/toolset` + `infra/mcp` |
| workspace skills/hooks/recipes | `application/workspace` 的 consumer ports | bootstrap prompt/hook/recipe adapters |
| 代码索引 | `domain/codebaseindex` ports | codebase adapter + vector/embedding backend |
| Run/Session 持久化 | 对应 `application/*` / `domain/*` 窄端口 | `infra/storage/sqlite` |

这些端口不等于都要暴露配置开关。单一默认后端仍应服从依赖方向；是否允许用户选择
实现是产品决策，不是 interface 自动带来的承诺。

## 3. Execution 边界

- `adapter/agentexec.Engine` 是具体 Agent SDK 防腐对象，直接持有具体
  `*agent/runtime.Engine`；不再为 start/restore/control 套单实现接口；
- `adapter/agentexec/turn` 自己定义 unexported 两方法 `engineDep`，因为具体 turn control
  才是 Start/Restore 的消费者；
- `turn.New` 返回具体的进程内 turn control；`turn.Executor` 在消费侧定义
  `executorDispatcher` 窄端口，把所需控制能力投影为 application/runs ports。不要再在
  application、delivery 或 bootstrap 外面套一层 Manager/Facade；
- tool catalog、MCP ports、maintenance 和 closers 不经 Engine 中转。

这类接口用于守住消费方形状，不表示用户可替换整套 Agent runtime 或 Run state machine。

## 4. 内部具体类型

以下组件保持具体：

- `application/runs.Coordinator` 与 journal/pump/admission；
- `application/sessions`、queries、models、workspace 等 use-case coordinator；
- `adapter/agentexec.Engine`、`adapter/agentexec/turn.Executor` 及其私有具体 turn control；
- `adapter/toolset.Resolver` 与 diagnostic registry；
- `delivery/server`、dispatch 与 HTTP/inprocess transport；
- `bootstrap.Stack`、`bootstrap.Host`、`hostLifetime` 与 wiring；
- 单一 SQLite backend 内部的 transaction/store 组合。

“具体类型”不代表允许越层依赖。Delivery 仍只驱动 Application；Application 不 import
Agent SDK、toolset、SQLite、MCP SDK 或 protocol DTO。

## 5. 注入与生命周期规则

- 可选策略采用 nil-default：显式注入 steering/compactor/extractor 时 Bootstrap 不覆盖；
- 必需依赖在构造时验证，禁止拖到第一条请求才 panic；
- 注入接口前消除 typed-nil，避免“接口非 nil、动态值为 nil”；
- provider/model 必须显式配对，不从 model 字符串猜 provider；
- auth、trace、session metadata 经 `context.Context` 传播，不塞进业务 DTO；
- executable tool 留在 `tools.Registry` / Resolver，wire 只传 tool definition 或调用参数；
- capability closer 从创建起由 Bootstrap staged guard 暂管，Host 成功后成为唯一 owner；
- Host 按反依赖顺序幂等关闭；Engine 不提供空壳 Close。

## 6. 禁止项

- 不给 coordinator、Engine、managed interaction、transport 或具体 turn control 再套换名 facade；
- 不把多个小端口重新聚合成 `Manager` / `Service` / `RuntimeDeps` 胖接口；
- 不引入全局 service locator、DI container 或 string-key handler registry；
- 不把具体 maintenance/SQLite/provider/MCP adapter import 到 Application 用例；
- 不把 credential、SDK client、logger、registry 或闭包塞进协议 DTO；
- 不用兼容 wrapper、别名字段或双路径同时维护旧新 SPI。
