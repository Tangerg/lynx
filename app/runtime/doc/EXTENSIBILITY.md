# Lyra 的可替换性边界

Lyra 只为外部实现确实可能替换的能力定义接口。Run 生命周期、turn 状态机、协议
dispatch 和资源装配是产品本身，直接使用具体类型。整体分层见
[`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md)。

## 1. 判断准则

抽端口前先问：另一个独立实现是否会由组合根注入，而且消费方是否只需要一个稳定的
小能力面？答案为是才定义 interface。只有一个内部实现、仅为测试 mock、或接口会暴露
transport/SDK 细节时，使用具体类型更清晰。

Port 定义在消费方 package。实现放在 `adapter` 或 `infra`，Bootstrap 负责装配。

## 2. 当前可替换端口

| 能力 | 消费方端口 | 默认实现/来源 |
|---|---|---|
| 长期知识 | `domain/knowledge.Store` | file-backed LYRA.md adapter |
| 上下文压缩 | `adapter/agentexec.Compactor` | `adapter/maintenance` |
| 事实提取 | `adapter/agentexec.Extractor` | `adapter/maintenance` |
| turn steering | `adapter/agentexec.SteeringSink` | conversation/message adapter |
| per-run model | `application/models.ClientResolver` | `adapter/modelclient` + `infra/llm` |
| Chat provider | `core/chat.Model` / optional `Streamer` | `models/*` provider adapter |
| Chat history | `chathistory.Store` | SQLite message store |
| Tool execution | `tools.Tool` / `toolloop.ToolResolver` | built-ins、MCP、A2A |
| 代码索引 | `domain/codebaseindex.Store` | vector index adapter |
| 进程/文件/Git/MCP/LSP | 对应 domain/application 窄端口 | `infra/*` |

这些接口不意味着必须提供配置开关。默认产品可以只有一个实现，但边界允许独立后端在
不改变用例代码的前提下注入。

## 3. 内部具体类型

以下组件没有外部实现者，保持具体类型：

- `application/runs.Coordinator` 与其 journal/pump/lifecycle；
- `adapter/agentexec.Engine` 和 `adapter/agentexec/turn.Executor`；
- `delivery/server`、dispatch 与 HTTP/inprocess transport；
- `application/sessions`、queries 等用例 coordinator；
- `bootstrap.Stack`、`bootstrap.Host` 和 wiring；
- 单一 SQLite backend 内部的具体 transaction/store 组合。

“不对外可替换”不等于可以跨环 import。Agentexec 仍通过 Application 的消费方事件/handle
边界接入，Delivery 仍只调用 Application，而不会越层直接驱动 turn executor。

## 4. 注入规则

- 可选依赖采用 nil-default：调用方显式注入时，Bootstrap 不覆盖；
- 必需依赖在构造时验证，禁止运行到第一条请求才 panic；
- 接口参数只包含领域/应用语义，auth、trace、session metadata 经 context 传播；
- provider/model 必须显式配对，不从 model 字符串猜 provider；
- executable tool 保存在 `tools.Registry`，wire request 只含 `ToolDefinition`；
- 关闭责任跟资源创建责任放在同一 composition owner。

## 5. 禁止项

- 不给 coordinator、engine、turn loop、transport、dispatcher 再套单实现接口；
- 不引入全局 service locator、DI container 或 string-key handler registry；
- 不把具体 maintenance/SQLite/provider 类型直接 import 到消费方用例；
- 不把 credential、SDK client、logger、registry 或闭包塞进协议 DTO；
- 不用兼容 wrapper 同时维护旧新两个 SPI。
