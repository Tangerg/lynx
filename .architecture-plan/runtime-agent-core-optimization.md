# Runtime / Agent / Core 架构优化计划

## 1. 当前理解

### 1.1 app/runtime

`app/runtime` 是应用层模块，当前包含命令入口、配置、delivery、domain、infra、adapter、kernel、runtime 等包。它负责 Lyra runtime 的启动、服务暴露、运行时编排、配置加载、依赖装配、用例流程组织，以及把 `agent`、LLM provider、toolset、MCP/A2A、存储、工作区等能力装配成应用行为。

当前第一印象：该模块已经按 application/domain/infra/delivery 的方向拆分，但包体量较大，需要重点确认应用编排、领域规则、基础设施细节、传输协议之间是否有反向耦合或职责混杂。

### 1.2 agent

`agent` 是 Agent 框架/基础库模块，当前包含顶层门面、`agent/core`、`agent/runtime`、`planning`、`toolloop`、`toolpolicy`、`workflow`、`event`、`hitl` 等包。它应提供 Agent 生命周期、执行框架、计划执行、工具循环、事件、人工介入等可复用机制。

当前第一印象：该模块应保持只向下依赖 `core` 和同层基础模块，不应包含 `app/runtime` 的应用语义。需要重点检查 `agent/core` 与顶层 `core` 的命名边界、`runtime` 包的职责密度、公共接口是否过宽，以及是否存在为 Lyra 应用场景定制的框架 API。

### 1.3 core

`core` 是基础协议与底层通用能力模块，当前包含文档、媒体、模型协议、tokenizer、vectorstore、evaluation 等包。它应像基础库一样小而稳定，提供通用协议和正交能力，不承载 Agent 专属或应用层概念。

当前第一印象：`core` 当前不应依赖 `agent` 或 `app/runtime`。需要重点检查是否存在 Agent 语义、应用语义、过宽协议、过早抽象或不必要外部依赖。

### 1.4 依赖方向

必须遵守的依赖方向：

```text
app/runtime -> agent -> core
```

明确禁止：

- `core -> agent`
- `core -> app/runtime`
- `agent -> app/runtime`
- 反向依赖
- 循环依赖
- 通过接口、回调、配置对象伪装出的反向依赖
- 底层模块出现上层命名或语义污染

---

## 2. 待检查问题

### 2.1 app/runtime

- 应用层是否承担过多底层机制实现。
- delivery、domain、infra、adapter、kernel 的依赖方向是否稳定。
- 是否存在传输协议、存储、工具实现、LLM provider 细节泄露到用例编排层。
- 是否存在过大的类型、函数、文件或包。
- 是否存在隐式全局状态、隐藏初始化、过宽端口接口。
- 是否存在可上移到应用层或下沉到 `agent`/`core` 的边界错位。

### 2.2 agent

- 是否只依赖 `core` 以及必要的基础支撑模块。
- 是否存在 `app/runtime` 或 Lyra 应用语义泄露。
- Agent 生命周期模型和执行模型是否清晰。
- 公共 API 是否适合作为框架/库长期演进。
- `agent/core`、`agent/runtime`、`toolloop`、`planning`、`workflow` 的职责是否清晰。
- 是否存在过宽接口、推测性扩展点、全局状态或隐式依赖。

### 2.3 core

- 是否存在 `agent` 或 `app/runtime` 的上层语义污染。
- 基础协议是否过宽或过早抽象。
- 是否存在不必要依赖。
- 是否有更适合上移到消费层的具体能力。
- 是否符合基础库式的小接口、低依赖、正交组合。

---

## 3. 优化原则

- 严格遵守依赖方向。
- 保持高内聚、低耦合。
- 优先小步重构，避免无目标大范围重写。
- 避免过度抽象和推测性 hook。
- 允许必要的破坏性调整，但必须服务于正确架构。
- 不为了兼容错误设计而牺牲长期架构。
- 不为了省事而进行粗暴破坏。
- 避免应用层语义污染底层模块。
- 公共 API 的破坏性调整先确认 scope、影响面和备选方案。
- 目标已更新：开发阶段不为了旧接口留兼容 shim；后续发现公开 API 本身不合理时，倾向直接改到正确形态，但具体破坏性 scope 仍需先咨询并记录。
- 每轮关键修改后进行验证，并记录无法验证的原因。

---

## 4. 执行计划

### 阶段 1：项目结构与依赖扫描

目标：

- 阅读项目结构。
- 梳理 `app/runtime`、`agent`、`core` 的包结构。
- 检查依赖方向。
- 识别反向依赖和循环依赖。

验证方式：

- 构建检查。
- 测试检查。
- 依赖关系检查。

### 阶段 2：core 模块治理

目标：

- 清理不属于 `core` 的上层语义。
- 收敛基础协议。
- 拆分过宽接口。
- 减少不必要依赖。

验证方式：

- 编译通过。
- 相关测试通过。
- 确认 `core` 不依赖上层模块。

### 阶段 3：agent 模块治理

目标：

- 明确 Agent 框架边界。
- 优化生命周期模型。
- 优化核心接口。
- 清理应用层概念泄露。
- 保持只依赖 `core`。

验证方式：

- 编译通过。
- 相关测试通过。
- 确认 `agent` 不依赖 `app/runtime`。

### 阶段 4：app/runtime 模块治理

目标：

- 明确应用层编排职责。
- 清理底层细节泄露。
- 优化用例组织和依赖装配。
- 降低与底层实现的耦合。

验证方式：

- 编译通过。
- 相关测试通过。
- 应用启动路径正常。

### 阶段 5：整体回归与记录更新

目标：

- 运行完整测试或构建。
- 更新执行记录。
- 记录未完成事项。
- 给出后续演进建议。

验证方式：

- 全量测试。
- 全量构建。
- 静态检查，如项目已有相关工具。

---

## 5. 执行记录

### 5.1 已完成

- 已阅读目标文件。
- 已阅读根目录结构、`go.work`、目标模块 `go.mod`。
- 已阅读 `core/CLAUDE.md`、`agent/CLAUDE.md`、`app/runtime/CLAUDE.md` 与 `app/runtime/Makefile`。
- 已确认本仓库为多 Go module workspace，统一 Go 版本为 `1.26.4`。
- 已确认根目录没有通用 Makefile/Taskfile；`app/runtime` 有模块内 Makefile，后续会检查其目标。
- 已创建本计划文件。
- 已完成第一轮直接 import 扫描：
  - `core` 未发现 import `github.com/Tangerg/lynx/agent` 或 `github.com/Tangerg/lynx/app/runtime`。
  - `agent` 未发现 import `github.com/Tangerg/lynx/app/runtime`。
  - `go list -deps ./...` 在 `core`、`agent`、`app/runtime` 三个模块均能完成，未暴露编译级 import cycle。
- 已阅读现有架构测试：
  - `agent/internal/arch/arch_test.go` 约束 agent 模块内部依赖阶梯。
  - `app/runtime/internal/arch/arch_test.go` 约束 Lyra Clean Architecture 依赖方向，并额外约束 `domain/hooks` 的纯度。
- 已完成 `core` 基线验证：`go test ./...` 通过。
- 已阅读 `app/runtime/doc/ARCHITECTURE_REVIEW.md`、`app/runtime/doc/GREENFIELD_DESIGN.md` 的关键结论：原 P0 的 `delivery/server` pump/rollback 编排泄漏已于 2026-07-06 落地，当前代码中的 `kernel/runsegment` 与 `kernel/lifecycle` 已承接这些用例编排。
- 已阅读 `agent/docs/ARCHITECTURE_REVIEW.md`、`agent/docs/GREENFIELD_DESIGN.md` 的关键结论：`agent` 是 SDK 库，不按应用 Clean Arch 拆环；`runtime/mcp.go` 保持在 `runtime` 包内、`ServiceProvider` 保持开放注册表，均已有明确裁决。
- 已新增机器防腐：
  - `core/internal/arch/arch_test.go`：防止 `core` 生产代码 import `core`/`pkg` 之外的 lynx 上层模块。
  - `agent/internal/arch/arch_test.go`：补充防止 `agent` 生产代码 import `app/*` 模块。
- 已清理 `app/runtime` 内部 MCP 命名债：移除 `McpToolInfo` / `McpServerStatus` 拼写别名，并统一调用点使用 `MCPToolInfo` / `MCPServerStatus`。
- 已完成一轮上层语义污染扫描：
  - `core` Go 文件未命中 `Lyra`、`app/runtime`、`workspace.*` 等应用层语义。
  - `agent` Go 文件未命中 `Lyra`、`app/runtime`、`workspace.*` 等应用层语义。
- 已将 `agent/runtime/chat_client_provider_test.go` 中一个测试注释从 Lyra 应用场景改为通用框架描述。
- 已完成第二轮卫生清理：
  - `core/model/control_flow_test.go`：常量错误从 `fmt.Errorf` 改为 `errors.New`。
  - `app/runtime/internal/domain/mcpserver/registry.go`：去掉容易被误读为兼容债的 `Legacy` 注释措辞。
  - `app/runtime/internal/delivery/server`：将内部 `notImpl` 改名为 `capabilityNotNegotiated`，按协议语义命名。
  - `app/runtime/internal/arch/arch_test.go`：补强 exact root ring 分类，避免未来根包生产代码绕过依赖规则。
  - `agent/internal/arch/arch_test.go`：将 `agent` 顶层便利门面纳入外层 rung，防止内层包 import 顶层门面。
- 已完成第三轮目标模块卫生清理：
  - `app/runtime/internal/adapter/hooks`：`trusted` 回调改为接收调用时的 `context.Context`，避免信任检查用 `context.Background()` 断开取消与 trace 语义。
  - `app/runtime/cmd/lyra`：hook trust store 查询改为使用 resolver 传入的调用 ctx。
  - `app/runtime/cmd/lyra`：`serve` 关停路径改为从命令上下文派生 `context.WithoutCancel`，避免 shutdown flush / HTTP drain 重新起裸 `context.Background()` 造成 trace/baggage values 断链。
  - `agent/event/multicast.go`：修正 panic observability 注释断句。
  - `agent/runtime`、`agent/runtime/autonomy`、`agent/workflow`：`helpers_test.go` 改名为 `deploy_support_test.go`，去掉泛文件名与旧文件名注释。
  - `core/model/chat/middleware`：测试夹具从 `fakeHandler` / `recordingHandler` / `toolCallHandler` 改为模型语义命名，避免泛 Handler 测试名继续扩散。
- 已完成第四轮目标模块结构清理：
  - `core/vectorstore`：包文档不再锚定仓库内具体 provider adapter 路径，只说明 concrete provider implementations 位于接口包之外，避免 `core` 文档层知道上层模块布局。
  - `app/runtime/internal/infra/mcp`：将原单体 `mcp.go` 拆为 `doc.go`、`config.go`、`status.go`、`tools.go`、`probe.go`、`connections.go`，按包文档、配置别名、状态模型、工具投影、探测、连接生命周期分责，保持 public/internal API 不变。
  - `app/runtime/internal/kernel/turn`：按既有文件职责清单继续拆分，将事件订阅 + delta coalescing 移入 `event_stream.go`，pre-turn lifecycle hooks 移入 `prompt_hooks.go`，`newTurnState` 贴近 `turnState` 定义。
- 已完成第五轮目标模块结构清理：
  - `app/runtime/internal/runtime`：将包级架构说明移入 `doc.go`，将 `Config` 及 construction-time 端口/输入类型移入 `config.go`，让 `runtime.go` 聚焦 Runtime 聚合结构、构造流程和基础 accessor，保持类型名与构造签名不变。
- 已按用户要求撤回越界的 `models` 改动；本计划后续只记录 `core`、`agent`、`app/runtime`。
- 已完成第六轮目标模块结构清理：
  - `core/model/chat`：将原 `client.go` 拆为门面入口、`client_request.go`、`client_call.go`、`client_stream.go`，让 `client.go` 只保留 handler alias、middleware chain constructor 与 `Client` sticky default 门面。
  - `ClientRequest` 负责请求状态、clone、消息/options 归一化与最终 `Request` 构造。
  - `ClientCaller` 负责同步调用路径、structured parser 注入与响应文本解析。
  - `ClientStreamer` 负责流式调用路径、stream span/metrics 生命周期与文本 delta 投影。
  - 导出的类型名、构造函数、方法签名和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第七轮目标模块结构清理：
  - `app/runtime/internal/kernel/turn`：将原 `dispatcher.go` 中混在一起的请求校验、Dispatcher contract、事件模型拆为 `request.go`、`dispatcher.go`、`event.go`。
  - `request.go` 承接 `StartTurnRequest` / `RehydrateRequest`、请求级 sentinel errors 与 options 校验。
  - `dispatcher.go` 回到包入口、per-turn client resolver seam 与 `Dispatcher` 接口契约。
  - `event.go` 承接 turn event sealed sum、`BaseEvent`、事件 stamp 实现、`TurnEndReason` 与 usage alias。
  - 同步更新 `inmemory.go` 的包内职责清单，避免结构注释滞后。
  - 导出的类型名、错误值、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第八轮目标模块结构清理：
  - `app/runtime/internal/kernel/turn`：继续收敛 `turn.go`，将 per-turn 状态对象与并发不变量移入 `state.go`。
  - 将 mid-run steering source 与 terminal steering flush 移入 `steering.go`，让 steering 队列的两条消费路径相邻。
  - 将 terminal event 映射、teardown、`turnEndPlan` / fallback planning 移入 `terminal.go`。
  - `turn.go` 现在聚焦 run/drive/interrupt 主生命周期和 post-turn maintenance；同步更新 `inmemory.go` 的包内职责清单。
  - 导出的类型名、错误值、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第九轮目标模块结构清理：
  - `app/runtime/internal/kernel/lifecycle`：将原 `coordinator.go` 中混杂的 run admission、rollback、interrupt resume/cancel、fork、session mutation 拆为按 use-case 聚合的文件。
  - `coordinator.go` 现在只保留包文档、`Stores` 消费端端口、`Coordinator` 聚合根、顶层 sentinel errors 和构造函数。
  - `admission.go` 承接 session single-writer slot 与 start/resume/mutation admission。
  - `rollback.go` 承接 rollback boundary resolution 和 rollback write-set。
  - `interrupt.go` 承接 parked interrupt cancel/resume、rehydrate fallback 与 turn handle 投影。
  - `fork.go` 承接 fork boundary 与 fork write-set。
  - `session_mutation.go` 承接 delete/restore/subtree purge 与 interrupt cleanup。
  - 导出的类型名、错误值、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十轮目标模块结构清理：
  - `app/runtime/internal/infra/mcp`：将原 `connections.go` 中混杂的启动拨号、运行时重连/授权、registry 查询、工具刷新和关闭逻辑按连接生命周期拆开。
  - `connections.go` 现在只保留 live server state、`Connections` 聚合状态、tool sink、shared client 和 locked lookup。
  - `dial.go` 承接 boot-time `Dial` 与拨号 observability。
  - `reconnect.go` 承接 `Reconnect` / `Configure` / `Authorize` 以及 shared `dialAndSwap` 和 status 写回。
  - `registry.go` 承接 `Statuses` / `Tools` / `Remove` / `refreshTools` / `Close`。
  - 导出的类型名、错误值、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十一轮目标模块结构清理：
  - `app/runtime/internal/delivery/server`：将 stream translator 的 resume 绑定、问题完成与 item id 复用逻辑移入 `translator_resume.go`。
  - `translator_simple_items.go` 承接 opening user message、mid-run steer、todo snapshot、compaction 这类内容一次性完整可知的 item 投影。
  - `translator.go` 回到 translator 状态、item id 生成、run segment open 与 turn event dispatch，不再混放 resume bookkeeping 和简单 item 构造细节。
  - 导出的类型名、错误值、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十二轮目标模块结构清理：
  - `app/runtime/internal/adapter/toolset`：将原 `resolver.go` 中混杂的工具装饰器、workdir 工具构建、online 工具构建与 resolver 主流程拆开。
  - `decorated.go` 承接 tool decorator 基础设施，保留 definition / concurrency / returns-direct 透传语义。
  - `workdir_tools.go` 承接 cwd-bound filesystem 工具组装及 edit diagnostics/read guard/path lock/path guard 组合。
  - `online_tools.go` 承接 `OnlineConfig`、network-reaching tool opt-in 构建与条件注册 helper。
  - `resolver.go` 回到平台级 tool group resolver、动态 MCP 工具过滤、per-turn tool slice 解析主流程。
  - 导出的类型名、构造函数、接口方法和调用行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十三轮目标模块结构清理：
  - `app/runtime/internal/delivery/dispatch`：将原 `dispatch.go` 中混杂的连接状态、method table、notification 分发、stream frame 适配、response helpers、params helpers 拆开。
  - `method_table.go` 承接 JSON-RPC method → handler 查表和 unknown-method fallback。
  - `stream.go` 承接 `StreamFrame` 与 context-aware stream adapter。
  - `params.go` 承接 lenient unmarshal 与单 string 参数解码。
  - `notifications.go` 承接 notification 分发、run/workspace event envelope 编码与 frame 投影。
  - `reply.go` 承接 response/error/streaming result 构造与 handler 共享 reply spine。
  - `dispatch.go` 回到 per-connection handshake state、入口 envelope 校验、initialize gate 与 request dispatch 主流程。
  - 导出的类型名、方法名、JSON-RPC method table、错误映射和 stream 语义保持不变；本轮无公共 API 破坏性调整。
- 已完成第十四轮目标模块结构清理：
  - `core/model/chat`：将原 `message.go` 中混杂的消息协议根、assistant 投影视图/构造、system/user/tool 具体消息构造拆开。
  - `message.go` 现在只保留 `MessageType`、sealed `Message`、`ToolReturn`、`MessageParams` 与 `NewMessage` 分发。
  - `message_assistant.go` 承接 `AssistantMessage`、part iterator、flat view helper、blank/tool/reasoning 判断与 assistant 构造输入归一化。
  - `message_roles.go` 承接 `SystemMessage`、`UserMessage`、`ToolMessage` 及对应构造器。
  - 导出的类型名、构造函数、JSON wire shape 和 message 行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十五轮目标模块结构清理：
  - `app/runtime/internal/infra/lsp`：将原 `client.go` 中混杂的 LSP child process 生命周期、server→client handler、document sync、query RPC、diagnostics wait 拆开。
  - `client.go` 现在只保留 live client 状态模型、opened document 版本/hash 和 diagnostics cache 结构。
  - `client_lifecycle.go` 承接 server process 启停、JSON-RPC stdio bridge、initialize/initialized 握手和 graceful shutdown。
  - `client_handler.go` 承接 jsonrpc2 server→client 请求/通知处理与 diagnostics push cache。
  - `client_sync.go` 承接 didOpen/didChange document sync 及版本/hash 并发不变量。
  - `client_queries.go` 承接 definition/references/implementation/call hierarchy/hover/symbol queries。
  - `client_diagnostics.go` 承接 didSave nudge、fresh diagnostics wait 和 best-effort timeout fallback。
  - 导出的 API、ServerSpec/Servers facade 和 LSP wire behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第十六轮目标模块结构清理：
  - `agent/toolloop`：将原 `invoker.go` 中混杂的 tool round 入口、并发分段调度、单工具调用和 tracing 边界拆开。
  - `invoker.go` 现在聚焦 registry 入口、tool-call round orchestration、`invocationResult` 绑定和结果校验入口。
  - `invoker_segments.go` 承接 concurrency-safe segment 划分、bounded parallel execution、interrupt/abort 分类和 parked done-set 投影。
  - `invoker_tool.go` 承接单个 tool call 的 unknown-tool/recoverable-error/control-flow 分类，以及 OTel span、panic containment。
  - `agent/toolloop`：将原 `middleware.go` 中混在一起的配置入口、同步 call loop 和 streaming loop 拆开。
  - `middleware.go` 现在只保留公开 `Config`、`NewMiddleware`、loop state 和 middleware 聚合配置。
  - `middleware_call.go` 承接 synchronous tool loop entry / recursion / interrupt outcome / loop detection nudge。
  - `middleware_stream.go` 承接 streaming entry / response accumulation / synthetic tool-message emission / recursive stream loop。
  - 导出的 Config、NewMiddleware、ConcurrentTool、ParkStore、error types、call/stream 行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十七轮目标模块结构清理：
  - `core/vectorstore/filter/parser`：将原 `parser.go` 中混杂的 parser 状态/构造、token plumbing、Pratt 主循环、原子表达式解析和运算符表达式解析拆开。
  - `parser.go` 现在保留 `ParseError`、`Parser` 状态、`NewParser` operator table 注册、`Parse` 入口和 package-level `Parse` convenience。
  - `tokens.go` 承接 lexical error propagation、prefix/infix handler registration、token advance 与 expected-kind 消费。
  - `expression.go` 承接 Pratt precedence-climbing 主循环。
  - `atom.go` 承接 identifier/literal/grouping/list literal 解析。
  - `operators.go` 承接 unary/binary、`NOT IN`、`IS [NOT] NULL`、index expression 解析。
  - 导出的 `ParseError`、`Parser`、`NewParser`、`Parse`、filter AST shape 和语法行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十八轮目标模块结构清理：
  - `core/vectorstore/filter/lexer`：将原 `lexer.go` 中混杂的 lexer 状态/构造、token emission、cursor reader、literal scanning、operator dispatch 和 public token APIs 拆开。
  - `lexer.go` 现在保留 `Lexer` 状态模型、`NewLexer`、`Scan` / `Token` / `Tokens` / `Reset` 公开 API。
  - `emit.go` 承接 token start marking、EOF/error/illegal/kind/literal/ident token emission。
  - `cursor.go` 承接 rune peek/consume、cursor position advance、buffer append、expected-rune consumption 与 whitespace skip。
  - `literals.go` 承接 string escape、string literal、number/negative number、identifier/keyword scanning。
  - `dispatch.go` 承接 one/two-character operator scanning 与 current-rune token dispatch。
  - 导出的 `Lexer`、`NewLexer`、`Scan`/`Token`/`Tokens`/`Reset` 和 token stream 行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第十九轮目标模块结构清理：
  - `app/runtime/cmd/lyra`：将原 `app.go` 中混杂的 CLI App 本体、lazy runtime bootstrap、config-to-runtime 投影、sqlite/file stores 装配和 first-run seed 逻辑拆开。
  - `app.go` 现在保留 package documentation、`App`、`NewApp`、`Run`、runtime/config accessors 和 cobra error exit helpers。
  - `runtime_bootstrap.go` 承接 `ensureRuntime`、默认 chat client 构建、provider env fallback 包装、hook resolver 装配和 `lyraruntime.New` composition root。
  - `config_projection.go` 承接 config loading/provider resolution，以及 MCP/A2A/LSP config → runtime/domain DTO projection。
  - `stores.go` 承接 SQLite/file storage backend wiring 和 `Stores` bundle。
  - `seeds.go` 承接 configured provider 与 utility model first-run seeding。
  - CLI command API、lazy runtime behavior、storage backends、runtime wiring 和 cobra error behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十轮目标模块结构清理：
  - `app/runtime/internal/kernel/turn`：继续收敛 `inmemory.go`，将 in-process dispatcher 的构造/共享状态与各类 public method 路径拆开。
  - `inmemory.go` 现在只保留 `Dependencies`、`New`、`inMemory` 状态模型和包内文件职责索引。
  - `turn_start.go` 承接 `StartTurn` admission、turn id/span 初始化和 prompt hook 前置执行。
  - `turn_control.go` 承接 `Cancel` / `Resume` / `resumeAndDrive` 的 same-process interrupt 控制路径。
  - `rehydrate.go` 承接 persisted process snapshot 的 cross-restart resume 路径。
  - `live_registry.go` 承接 live turn lookup、interrupt kind negotiation gate 与 `ProcessID` snapshot key lookup。
  - `event_emit.go` 承接 stamped event delivery 与 backpressure/cancellation 语义。
  - `steering.go` 同步收纳 `InjectSteering` 入口，让 steering 队列写入、mid-run drain 和 terminal flush 相邻。
  - 导出的 `Dispatcher` 行为、错误值、turn id 语义、event ordering/backpressure 和 rehydrate/resume 行为保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十一轮目标模块结构清理：
  - `agent/runtime`：将原 `child.go` 中混杂的子进程同步 spawn、异步 background spawn、top-level fresh run、terminal error 和 child process preparation 拆开。
  - `child.go` 现在只保留 sub-agent delegation depth backstop 和 `ErrMaxSpawnDepth`。
  - `child_sync.go` 承接 `SpawnChild`、`SpawnChildProtectedOnly`、`SpawnChildFresh` 三个同步 child spawn 入口。
  - `child_async.go` 承接 `SpawnChildAsync` 的 background task 创建和 done channel 返回。
  - `child_spawn.go` 承接 `childSpawn` preparation、parent lookup、blackboard inheritance、session lineage link 和失败清理。
  - `run_fresh.go` 承接 `RunFresh` top-level process 启动入口。
  - `terminal_error.go` 承接 `AgentProcess.TerminalError` terminal status formatting。
  - 导出的函数/方法签名、错误值、budget depth guard、blackboard inheritance、session lineage 和 async child lifecycle 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十二轮目标模块结构清理：
  - `app/runtime/internal/config`：将原 `config.go` 中混杂的配置 DTO、Viper `Load` 入口、online credentials、LSP yaml table、MCP env parser、A2A env parser 拆开。
  - `config.go` 现在保留 package documentation、配置源说明和 `Load` 的 source-order orchestration。
  - `types.go` 承接 `Config`、`ServerConfig`、`OnlineConfig`、`MCPServerConfig`、`LSPServerConfig`、`A2AAgentConfig` 与 MCP transport constants。
  - `online.go` 承接 optional network-reaching tool credentials 与 `LYRA_HTTP_ALLOWED_HOSTS` parsing。
  - `lsp.go` 承接 `lsp.servers` yaml table loading。
  - `mcp_env.go` 承接 `LYRA_MCP_SERVERS` descriptor parsing、stdio/http dispatch、per-server token env normalization。
  - `a2a_env.go` 承接 `LYRA_A2A_AGENTS` descriptor parsing。
  - 导出的 config shape、`Load()` 行为、source precedence、env var names、MCP/A2A/LSP parsing errors 和 default server settings 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十三轮目标模块结构清理：
  - `app/runtime/internal/delivery/protocol`：将原 `workspace.go` 中混杂的 workspace method group、workspace event stream、file/read/grep DTO、diff/change DTO、catalog DTO 和 MCP registry/status DTO 拆开。
  - `workspace.go` 现在只保留 `Workspace` method group interface。
  - `workspace_events.go` 承接 `workspace.subscribe` request/ack、watch spec 和 `WorkspaceEvent` union。
  - `workspace_files.go` 承接 common `WorkspaceQuery`、file head、grep、list/read file request/result DTO。
  - `workspace_diff.go` 承接 diff mode/format、`GetDiffRequest`、`Diff`、`FileStatus`、`FileDiff`、`WorkspaceFileChange`。
  - `workspace_catalog.go` 承接 workspace catalog shapes：`Skill` / `SkillSource`、`AgentDoc` / `AgentDocScope`。
  - `workspace_mcp.go` 承接 MCP status/auth/tool/config/test request/result shapes。
  - JSON wire tags、method signatures、golden protocol shapes、workspace event union 和 MCP editable-vs-observed shape 分离保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十四轮目标模块结构清理：
  - `app/runtime/internal/infra/storage/sqlite`：将原 `session.go` 中混杂的 `SessionStore` 本体、row codec、read queries、lineage/fork/subtask、mutation/update helper 拆开。
  - `session.go` 现在只保留 `SessionStore`、`session.Store` 编译期断言和构造函数。
  - `session_codec.go` 承接 session row column list、row decode 和 metadata JSON encode/decode。
  - `session_read.go` 承接 `List` / `Get` / `Children` 查询路径。
  - `session_lineage.go` 承接 `Fork` 和 `CreateSubtask` 的 lineage write paths。
  - `session_mutation.go` 承接 `Create` / `Restore` / `Delete`、session field mutations、`updateByID`、insert helpers。
  - `session.Store` 行为、SQLite SQL、error wrapping、metadata encoding、fork/subtask derivation、idempotent delete 和 transaction behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十五轮目标模块结构清理：
  - `app/runtime/internal/infra/checkpoint`：将原 `checkpoint.go` 中混杂的 package docs、`Store` state、snapshot staging/commit/tagging、restore reset、shadow repo bootstrap/seeding、git command helpers 拆开。
  - `checkpoint.go` 现在只保留 package documentation 和 `ErrUnavailable` sentinel error。
  - `store.go` 承接 `Store`、per-worktree locking、`NewStore`、`DropSession` 和 shadow git dir resolution。
  - `snapshot.go` 承接 `Snapshot`、staged change collection、size cap 和 commit decision。
  - `restore.go` 承接 `Restore` 的 availability check、pre-restore recovery commit 和 reset behavior。
  - `repo.go` 承接 lazy shadow repo initialization、real-repo seeding、common excludes、tag sanitization 和 repo existence check。
  - `git.go` 承接 shadow git invocation、real-repo git query、file existence and copy helpers。
  - Shadow repo layout、per-worktree serialization、common excludes、2 MiB staging cap、tag naming, git identity/config isolation, seed-from-real-repo best effort behavior and restore reversibility 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十六轮目标模块结构清理：
  - `app/runtime/internal/domain/codebaseindex`：将原 `indexer.go` 中混杂的 `Indexer` state/constructor、availability/search/status、reconcile build pass、embedding batching 和 cosine ranking 拆开。
  - `indexer.go` 现在只保留 `Indexer` 状态、constructor、cache/build constants、loaded corpus 状态和 per-cwd build lock helper。
  - `search.go` 承接 `Available`、`Search`、`Status` 和 freshness debounce check。
  - `reconcile.go` 承接 `EnsureIndexed`、`Reindex`、incremental rebuild、removed-file cleanup、meta persistence、status transitions。
  - `embedding.go` 承接 chunk batch embedding 和 vector-count validation。
  - `scoring.go` 承接 top-k cosine similarity ranking。
  - `Index` behavior、per-cwd serialization、debounce semantics、incremental hash reuse、embedding batch size、status/error transitions 和 top-k ordering 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十七轮目标模块结构清理：
  - `app/runtime/internal/runtime`：将原 `mcp.go` 中混杂的 MCP registry 用例、live-connection config projection、env seed 和 per-call tool gating 原子发布拆开。
  - `mcp.go` 现在只保留 Runtime 的 MCP registry/live-connection 用例入口、registry 持久化后 apply/gating 的顺序约束和 engine facade delegation。
  - `mcp_config.go` 承接 enabled server lookup、registry server → kernel live descriptor 投影和 deterministic stdio env flattening。
  - `mcp_gating.go` 承接 `mcpGating` / `mcpEnvironment`、startup gating load、enabled-server tool gating projection 和 runtime registry mutation 后的 atomic refresh。
  - `mcp_seed.go` 承接 env-sourced MCP server first-run seeding，保持 persisted runtime edits 优先。
  - Registry API、live connection apply order, disabled/auto-approve gating semantics, boot-time enabled configs, env flattening order and seed behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十八轮目标模块结构清理：
  - `app/runtime/internal/infra/git`：将原 `diff.go` 中混杂的 diff public API、tracked source collection、base branch resolution、untracked-file all-added diff projection 和 unified diff parser 拆开。
  - `diff.go` 现在只保留 diff mode/data model、`Diff` 和 `RawDiff` 入口。
  - `diff_source.go` 承接 tracked `git diff` argument construction、repo/git availability checks、base-mode merge-base/default branch resolution 和 fresh-repo empty diff fallback。
  - `diff_untracked.go` 承接 `status --porcelain` untracked path collection、relPath scoping、binary heuristic 和 all-added untracked `DiffFile` projection。
  - `diff_parse.go` 承接 unified patch → structured `DiffFile` / `Row` parsing、hunk line-number extraction 和 added/removed counters。
  - Worktree/base diff semantics, ErrUnavailable/ErrNotRepo/ErrNoBase mapping, untracked raw/parsed projection, binary detection, rename/path parsing and row line numbers 保持不变；本轮无公共 API 破坏性调整。
- 已完成第二十九轮目标模块结构清理：
  - `app/runtime/internal/adapter/maintenance`：将原 `compaction.go` 中混杂的 compaction worker 主流程、配置默认值、token-footprint trigger 和 summary LLM prompt/call 拆开。
  - `compaction.go` 现在只保留 `Compactor` 状态、constructor 和 `MaybeCompact` 的 read/guard/cutoff/summarize/replace 生命周期。
  - `compaction_config.go` 承接 `CompactionConfig`、message/token/recent defaults、context-window-relative trigger defaults 和 summary tool result cap。
  - `compaction_trigger.go` 承接 message-count/token-footprint compaction predicate、chars→tokens heuristic 和 transcript-based estimate。
  - `compaction_summary.go` 承接 summary prompt、direct client resolution、`askDirect` 调用和 summary system-message construction。
  - MaxMessages/MaxTokens/KeepRecent defaults, context-window-relative trigger, pre-compact veto timing, user-boundary cutoff, short-history skip, summary prompt, history.Replace rewrite and result counters 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十轮目标模块结构清理：
  - `core/model/chat/middleware/safeguard`：将原 `safeguard.go` 中混杂的 public config/error surface、call/stream middleware wrapping、input/output scanning/block error construction 和 substring matcher 拆开。
  - `safeguard.go` 现在只保留 `ErrUnsafeContent`、`Matcher`、`Scope`、`Options`、`NewMiddleware` 和 middleware state/passthrough entry。
  - `middleware.go` 承接 call/stream wrapper 两条 execution path。
  - `scan.go` 承接 input/output text selection、skip assistant/tool prior-turn messages 的输入规则和 block error construction。
  - `substring.go` 承接 `SubstringMatcherOptions`、`NewSubstringMatcher`、term normalization 和 case-sensitive/hidden-match lookup。
  - Exported API, nil matcher passthrough, default ScopeBoth behavior, OnBlock callback timing, input/output scan rules, ErrUnsafeContent wrapping, HideMatch and case sensitivity 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十一轮目标模块结构清理：
  - `agent/planning/planner/htn`：将原 `htn.go` 中混杂的 HTN domain model、task library registry、planner entry/tracing 和 recursive decomposition algorithm 拆开。
  - `task.go` 承接 `Task` / `Method` / primitive 判定，聚焦 HTN hierarchy model。
  - `library.go` 承接 `Library`、task shape validation、duplicate detection、`MustAdd` 和 lookup。
  - `planner.go` 承接 `Planner`、constructor、extension name 和 `PlanToGoal` 的 input validation / tracing / root task dispatch。
  - `decompose.go` 承接 recursive task expansion、method backtracking、excluded action handling、world-state threading 和 method precondition matching。
  - `tracing.go` 承接 HTN planner instrumentation scope。
  - Exported API, validation errors, tracing attributes/status, nil-plan fallback, structural error behavior, max recursion guard, excluded-action backtracking and action ordering 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十二轮目标模块结构清理：
  - `core/vectorstore/filter`：将原 `syntax.go` 中混杂的 synthetic token factory、identifier operand factory、literal/list factory、comparison/matching factory、logical factory 和 index factory 拆开。
  - `syntax_token.go` 承接 synthetic token construction。
  - `syntax_operand.go` 承接 `NewIdent`、identifier/index operand normalization 和 builder 共享 operand adapter。
  - `syntax_literal.go` 承接 `NewLiteral`、`NewLiterals`、`NewListLiteral`、basic Go value → AST literal/list node conversion。
  - `syntax_comparison.go` 承接 `EQ` / `NE` / `LT` / `LE` / `GT` / `GE` / `In` / `Like` factories。
  - `syntax_logic.go` 承接 `And` / `Or` / `Not` factories。
  - `syntax_index.go` 承接 nested index expression factory。
  - Exported constructors, generic constraints, synthetic token positions, literal formatting, list conversion, builder shared helpers, comparison/logical/index AST shape and panic-on-impossible-constraint-violation behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十三轮目标模块结构清理：
  - `core/vectorstore`：将原 `store.go` 中混杂的 request validation sentinels、retrieval request/port、create request/port、delete request/ports、store metadata 和 document writer adapter 拆开。
  - `errors.go` 承接 request-shape validator sentinel errors。
  - `retrieval.go` 承接 similarity defaults、`RetrievalRequest`、top-k/min-score/filter options、validation 和 `Retriever` port。
  - `create.go` 承接 `CreateRequest` validation 和 `Creator` port。
  - `delete.go` 承接 `DeleteRequest` validation、metadata-filter `Deleter` port 和 optional `IDDeleter` capability。
  - `store.go` 现在只保留 `Store` composition and `StoreMetadata` identity surface。
  - `document_writer.go` 承接 `Creator` → `document.Writer` adapter。
  - 同步去掉 `Store` godoc 中对 sibling provider module path 的锚定，改为“interface package 之外”的基础层表述。
  - Exported API, JSON tags, validation errors, filter analysis, similarity defaults, document writer behavior and Store composition 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十四轮目标模块结构清理：
  - `core/model/chat`：将原 `modelinfo.go` 中混杂的 model identity metadata、pricing/rate-card 计算和 model capability descriptors 拆开。
  - `model_metadata.go` 承接 `ModelMetadata`、`ModelInfo` 和 `ModelInfo.IsZero`。
  - `model_pricing.go` 承接 `Pricing`、banded `CostOf` 和 per-band `Pricing.Cost`。
  - `model_capabilities.go` 承接 `Reasoning`、`Limits`、`Modality` 和 `Modalities` capability surface。
  - `message_transcript.go` 承接 message transcript projection 与 `MessageList.Strings`，让 `message_list.go` 聚焦 list filtering、merge 和 request augmentation helpers。
  - Exported API, JSON tags, pricing semantics, modality helpers, transcript output and MessageList merge behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十五轮目标模块结构清理：
  - `core/model/audio/tts`：将原 `client.go` 中混杂的 middleware aliases、fluent request builder、synchronous call path、streaming path 和 sticky client facade 拆开。
  - `client.go` 现在只保留 handler/middleware aliases、`NewMiddlewareChain` 和 `Client` sticky facade。
  - `client_request.go` 承接 `ClientRequest`、request options/text/params builder、clone、request assembly 和 `Call` / `Stream` entrypoints。
  - `client_call.go` 承接 `ClientCaller.Response` 和 `ClientCaller.Speech`。
  - `client_stream.go` 承接 `ClientStreamer.Response` 和 `ClientStreamer.Speech`。
  - Exported API, middleware chain behavior, option merge semantics, params cloning, stream error propagation and speech bytes projection 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十六轮目标模块结构清理：
  - `core/model/embedding`：将原 `client.go` 中混杂的 middleware aliases、fluent request builder、call execution/tracing/metrics 和 sticky client facade 拆开。
  - `client.go` 现在只保留 handler/middleware aliases、`NewMiddlewareChain` 和 `Client` sticky facade。
  - `client_request.go` 承接 `ClientRequest`、request options/texts/params builder、clone、request assembly 和 `Call` entrypoint。
  - `client_call.go` 承接 `ClientCaller.Response`、single embedding projection 和 batch embeddings projection。
  - Exported API, middleware behavior, option merge semantics, params/text cloning, OTel span/metrics timing and empty-results guard 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十七轮目标模块结构清理：
  - `core/model/moderation`：将原 `client.go` 中混杂的 middleware aliases、fluent request builder、call execution/projection 和 sticky client facade 拆开。
  - `client.go` 现在只保留 handler/middleware aliases、`NewMiddlewareChain` 和 `Client` sticky facade。
  - `client_request.go` 承接 `ClientRequest`、request options/texts/params builder、clone、request assembly 和 `Call` entrypoint。
  - `client_call.go` 承接 `ClientCaller.Response`、single categories projection 和 all-categories projection。
  - Exported API, middleware behavior, option merge semantics, params/text cloning, response projection semantics and existing nil-result behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十八轮目标模块结构清理：
  - `core/model/audio/transcription`：将原 `client.go` 中混杂的 middleware aliases、fluent request builder、call execution/projection 和 sticky client facade 拆开。
  - `client.go` 现在只保留 handler/middleware aliases、`NewMiddlewareChain` 和 `Client` sticky facade。
  - `client_request.go` 承接 `ClientRequest`、request options/audio/params builder、clone、request assembly 和 `Call` entrypoint。
  - `client_call.go` 承接 `ClientCaller.Response` 和 text projection。
  - Exported API, middleware behavior, option merge semantics, params cloning, audio reference sharing and text projection 保持不变；本轮无公共 API 破坏性调整。
- 已完成第三十九轮目标模块结构清理：
  - `core/model/image`：将原 `client.go` 中混杂的 middleware aliases、fluent request builder、call execution/projection 和 sticky client facade 拆开。
  - `client.go` 现在只保留 handler/middleware aliases、`NewMiddlewareChain` 和 `Client` sticky facade。
  - `client_request.go` 承接 `ClientRequest`、request options/prompt/params builder、clone、request assembly 和 `Call` entrypoint。
  - `client_call.go` 承接 `ClientCaller.Response` 和 image projection。
  - Exported API, middleware behavior, option merge semantics, params cloning, prompt seeding and image projection 保持不变；本轮无公共 API 破坏性调整。
- 已完成第四十轮目标模块结构清理：
  - `core/model/chat`：将原 `parser.go` 中混杂的 structured parser interface/shared markdown fence cleanup、list parser、map parser、generic JSON parser 和 type-erased any parser 拆开。
  - `parser.go` 现在只保留 `StructuredParser` contract 和 shared markdown code fence cleanup。
  - `parser_list.go` 承接 comma-separated list parser。
  - `parser_map.go` 承接 dynamic JSON object parser。
  - `parser_json.go` 承接 schema-backed generic JSON parser。
  - `parser_any.go` 承接 type-erased parser adapter。
  - Exported parser API, instruction text, markdown fence cleanup, JSON schema generation, parse error context and type-erased delegation behavior 保持不变；本轮无公共 API 破坏性调整。
- 已完成第四十一轮目标模块测试结构清理：
  - `app/runtime/internal/kernel`：将 `engine_test.go` 中混杂的 engine 行为用例和测试夹具拆开。
  - `engine_test.go` 现在聚焦 engine turn execution、tool registration、history、budget、restore/options 等行为用例。
  - `engine_fixtures_test.go` 承接 shared history store factory、assembled engine factory、recording observer、approval observer、JSON process store、option restore stub 和 per-run client override stub。
  - 测试断言、stub behavior、observer behavior、process snapshot round-trip behavior and toolset assembly path 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十二轮目标模块测试结构清理：
  - `agent/toolloop`：将 `tool_test.go` 中混杂的 tool registry / invocation / middleware / history 行为用例和测试夹具拆开。
  - `tool_test.go` 现在聚焦 registry lifecycle、tool invocation branching、recursive call/stream middleware、HITL resume and persisted history validity 行为用例。
  - `tool_fixtures_test.go` 承接 fake chat model、tool/response/request builders、stream collector、tool-history assertion helpers and test halt errors。
  - 测试断言、fake model scripting, tool-call response shape, continuation result extraction, history validation and HITL test error behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十三轮目标模块结构清理：
  - `app/runtime/internal/runtime`：将原 `runtime.go` 中混杂的 runtime aggregate、construction assembly、transaction helper、default model/provider accessors and skills facade 拆开。
  - `runtime.go` 现在聚焦 `Runtime` aggregate fields and ownership comments。
  - `runtime_assembly.go` 承接 `New` construction flow and half-built capability cleanup。
  - `tx.go` 承接 transactional write-set helper。
  - `defaults.go` 承接 default provider/model accessors。
  - `skills.go` 承接 workspace skills facade。
  - `session.go` 承接 session-facing forget hook alongside other session facade methods。
  - Construction validation, engine config wiring, maintenance port wiring, MCP/tool environment ownership, dispatcher/registry construction, default accessors, skills delegation and transaction fallback behavior 保持不变；本轮无公共 API 或行为调整。
- 已完成第四十四轮目标模块测试结构清理：
  - `app/runtime/internal/kernel/turn`：将 `inmemory_test.go` 中混杂的 in-memory dispatcher behavior tests and test fixtures 拆开。
  - `inmemory_test.go` 现在聚焦 dispatcher event ordering, seq monotonicity, steering, approval gate, cancel, deny, yolo and validation behavior。
  - `inmemory_fixtures_test.go` 承接 dispatcher/engine test assembly, event drain/name helpers, stub chat models and turn constructor unwrap helper。
  - 测试断言、stub model responses, toolset-backed engine assembly, event sequence helpers, HITL resume counter and history-aware message counting behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十五轮目标模块测试结构清理：
  - `app/runtime/internal/kernel`：将 `engine_test.go` 中的 engine tool inventory / custom tool registration tests 拆出。
  - `engine_test.go` 继续聚焦 chat turn execution, delegation, history, usage, budget, streaming, options and restore behavior。
  - `engine_tools_test.go` 承接 offline/online/partial-online tool inventory tests, custom-tools-without-resolver test and tool name projection helper。
  - 工具清单断言、custom tool preservation, no-resolver task injection behavior, online credential gating and tool name projection 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十六轮目标模块测试结构清理：
  - `app/runtime/internal/kernel`：将 `engine_test.go` 中的 history / process persistence behavior tests 拆出。
  - `engine_test.go` 继续聚焦 chat turn execution, delegation, usage, budget, streaming, options, restore and per-run client override behavior。
  - `engine_history_test.go` 承接 process snapshot persistence, multi-turn history loading, persistent history store round-trip and no-session isolation tests。
  - 测试断言、history store wiring, process snapshot persistence and session isolation behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十七轮目标模块测试结构清理：
  - `agent/toolloop`：将 `tool_test.go` 中的 registry / direct invocation behavior tests 拆出。
  - `tool_test.go` 继续聚焦 middleware loop, streaming, max-iteration, empty-response feedback, interrupt/resume and persisted-history validity behavior。
  - `tool_invocation_test.go` 承接 tool registry lifecycle, duplicate registration, invoke result continuation, return-direct short-circuit, unknown-tool feedback, recoverable failure feedback, direct-return decision and abort propagation tests。
  - 测试断言、fake tool setup, continuation request shape, error feedback semantics and abort behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十八轮目标模块测试结构清理：
  - `app/runtime/internal/delivery/server`：将 `sessions_test.go` 中被多个 server 测试复用的 runtime/server/session fixture 拆出。
  - `sessions_test.go` 继续聚焦 session update/delete/fork handler behavior。
  - `sessions_fixtures_test.go` 承接 shared `stubRuntime`, `newTestServer`, `newTestServerWithInfo`, turn/lifecycle/runsegment stubs and `newSessionServer` assembly。
  - 测试断言、runtime port stub behavior, lifecycle delegation, MCP/tool fixture responses and session store setup 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第四十九轮目标模块测试结构清理：
  - `app/runtime/internal/delivery/transport/http`：将 `server_test.go` 中的 shared HTTP transport fixture and stream/SSE behavior tests 拆出。
  - `server_test.go` 继续聚焦 sidecar info/health, URL RPC method form, JSON-RPC error mapping, notifications, body/media validation and method-not-allowed behavior。
  - `server_fixtures_test.go` 承接 shared `fakeRuntime`, `newTestServer`, JSON-RPC error-code decoder and response-body diagnostic helper。
  - `server_stream_test.go` 承接 streamable `runs.start`, SSE frame parsing and `runs.subscribe` Last-Event-Id propagation tests。
  - 测试断言、HTTP status mapping, JSON-RPC envelope shape, SSE framing and reconnect cursor behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十轮目标模块测试结构清理：
  - `app/runtime/internal/kernel`：将 `engine_test.go` 中的 token usage, pricing and budget-limit tests 拆出。
  - `engine_test.go` 继续聚焦 chat turn execution, observer wiring, unknown-tool recovery, delegation, cwd/subtask behavior, streaming, options, restore and per-run client override behavior。
  - `engine_accounting_test.go` 承接 per-turn usage roll-up, per-model usage breakdown, pricing cost roll-up, token budget stop and cost budget stop tests。
  - 测试断言、usage aggregation, pricing hook behavior, budget stop semantics and per-model accounting 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十一轮目标模块测试结构清理：
  - `app/runtime/internal/kernel/turn`：将 `engine_test.go` 中的 turn engine test fixtures 拆出。
  - `engine_test.go` 继续聚焦 stub engine driven turn, terminal discard, budget stop, clean cancel, rehydrate/resume and start-turn request propagation behavior。
  - `engine_fixtures_test.go` 承接 stub turn process, stub engine, slow cancel-aware engine, fake client resolver and sentinel chat model fixtures。
  - 测试断言、narrow engine seam coverage, rehydrate error handling, process discard tracking, per-run client/cwd/options propagation 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十二轮目标模块测试结构清理：
  - `app/runtime/internal/delivery/server`：将 `translator_test.go` 中的 continuation / resume behavior tests 拆出。
  - `translator_test.go` 继续聚焦 run open, tool terminal, command output projection, error classification, compaction, usage progress, steer message, todos and outcome projection behavior。
  - `translator_resume_test.go` 承接 edited-args proposal reuse, continuation run open, resumed tool original item reuse and resumed question terminal completion tests。
  - 测试断言、resume binding reuse semantics, continuation parent run projection, original item id/runId preservation and one-shot completion behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十三轮目标模块测试结构清理：
  - `app/runtime/internal/delivery/server`：将 `workspace_test.go` 中的 MCP status/tool wire behavior tests 与 VCS wire behavior tests 拆出。
  - `workspace_test.go` 继续聚焦 workspace path jail, file read/head, grep, skills/recipes listing, subscribe and agent-doc scope behavior。
  - `workspace_mcp_status_test.go` 承接 MCP server status projection, reconnect event ordering and MCP tool listing tests。
  - `workspace_vcs_test.go` 承接 VCS unavailable handling and git diff/file-change wire mapping tests。
  - 测试断言、MCP event ordering, tool-count projection, git status/diff mapping and workspace path/file behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十四轮目标模块测试结构清理：
  - `agent/toolloop`：将 `tool_test.go` 中的 interrupt/resume behavior test 与 persisted-history validity test 拆出。
  - `tool_test.go` 继续聚焦 recursive loop, direct return, streaming tool-message yield, max-iteration cap, unknown-tool feedback, empty-response feedback and no-tool passthrough behavior。
  - `tool_resume_test.go` 承接 interrupted tool round tail projection, pending-call resume and completed-call non-reexecution test。
  - `tool_history_test.go` 承接 multi-round tool loop plus history middleware persisted provider-sequence validity test。
  - 测试断言、resume tail shape, model re-entry count, completed/pending tool execution counts and persisted tool-history validity 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十五轮目标模块测试结构清理：
  - `app/runtime/internal/kernel/lifecycle`：将 `coordinator_test.go` 中的 rollback/fork history resolver tests 与 shared fake coordinator stores/turns 拆出。
  - `coordinator_test.go` 继续聚焦 parked-run cancel, session admission, mutation admission, resume admission and claimed interrupt resume behavior。
  - `coordinator_history_test.go` 承接 rollback boundary and fork history prefix resolver tests。
  - `coordinator_fixtures_test.go` 承接 coordinator store, interrupt store, session claimer, cancel turns and resume turns fixtures；fixture 命名从 cancel-specific 收敛到 coordinator 语义。
  - 测试断言、session claim/release semantics, interrupt consume/delete behavior, resume rehydrate fallback and history boundary/prefix behavior 保持不变；本轮无公共 API 或生产行为调整。
- 已完成第五十六轮目标模块生产代码边界清理：
  - `agent/event`：为 `Multicast` 增加 `OnEventContext(ctx, e)`，旧 `OnEvent(e)` 保持为兼容入口并委托到 context-aware 路径。
  - `agent/runtime`：将 run/tick/action/terminal 等内部事件发布路径改为携带当前运行 ctx，使 listener panic span 能继承当前 trace，而不是从 `context.Background()` 断链。
  - 对确实没有调用 ctx 的公开方法（如 `RecordLLMInvocation`、`AwaitInput`）保留旧路径，避免为了观测性强行破坏公开 API；action 内 `ProcessContext.Publish` 已在第五十七轮以兼容方式接入当前 action ctx。
  - 新增 `runtime` 集成测试覆盖：带 parent span 的 `RunAgent` 中，panic listener 产生的 `agent.listener.panic` span 必须保持在同一 trace。
  - 事件 listener 接口、已有 `OnEvent(e)` 调用方、事件类型和 runtime 行为保持兼容；本轮无公共 API 破坏性调整。
- 已完成第五十七轮目标模块生产代码边界清理：
  - `agent/core`：新增 `ContextEventPublisher` 与 `ProcessContext.PublishContext(ctx, event)`，保留旧 `EventPublisher` 与 `ProcessContext.Publish(event)` 入口。
  - `ProcessContext.ExecuteSafely` 在 action 执行期间记录当前 action ctx，旧 `ProcessContext.Publish(event)` 会自动使用该 ctx；action 不需要改调用代码即可保留 trace 链路。
  - `agent/runtime`：`buildProcessContext` 同时注入旧 publisher 与 context-aware publisher；`publishAnyContext` 将 action-emitted event 接入 `AgentProcess.publishEvent(ctx, e)`。
  - 新增 runtime 集成测试覆盖：action 内 `pc.Publish(event)` 触发的 listener panic span 必须保持在 `RunAgent` 的 parent trace 内；测试使用单次安装的 OTel exporter，避免全局 tracer provider 反复替换导致假阴性。
  - 本轮是兼容性 API 增量，不改变既有 `Publish(event)`、`EventPublisher`、listener 接口或事件类型；无公共 API 破坏性调整。
- 已完成第五十八轮目标模块生产代码边界清理：
  - `agent/runtime`：为 `AgentProcess.RecordUsage`、`RecordLLMInvocation`、`RecordEmbeddingInvocation` 增加 context-aware companion 方法，旧方法保持兼容并委托到 `context.Background()` 路径。
  - `agent/core`：将 `ProcessContext.Record*Invocation` 从 await 相关文件中收拢到 invocation 专属文件，并让既有 `ProcessContext.Record*` 自动使用当前 action ctx；同时提供显式 `Record*Context` 入口给拥有更精确 ctx 的调用方。
  - `agent/core` 未扩宽 `Process` 接口，而是通过可选 context-aware recorder 能力向下传递 ctx，避免让外部 `core.Process` 实现者被迫适配。
  - `app/runtime/internal/kernel`：turn loop 记录每轮 LLM usage 时改用 `pc.RecordLLMInvocationContext(ctx, inv)`，继续保持应用层只依赖 `ProcessContext` 编排入口。
  - 新增 runtime 集成测试覆盖：action 内 `pc.RecordLLMInvocation(...)` 触发的 invocation listener panic span 必须保持在 `RunAgent` 的 parent trace 内。
  - 本轮是兼容性 API 增量，不改变既有 `RecordUsage`、`RecordLLMInvocation`、`RecordEmbeddingInvocation` 或 `core.Process` 接口；无公共 API 破坏性调整。
- 已完成第五十九轮目标模块生产代码边界清理：
  - `agent/runtime`：为 `AgentProcess.AwaitInput` 增加 context-aware companion 方法，旧 `AwaitInput(req)` 保持兼容并委托到 `context.Background()` 路径。
  - `agent/core`：`ProcessContext.AwaitInput(req)` 自动使用当前 action ctx，并新增显式 `AwaitInputContext(ctx, req)` 给拥有更精确 ctx 的调用方。
  - `agent/core` 继续不扩宽 `Process` 接口，而是通过可选 context-aware awaiter 能力下传 ctx，避免让外部 `core.Process` 实现者被迫适配。
  - 新增 runtime 集成测试覆盖：action 内旧入口 `pc.AwaitInput(...)` 触发的 `ProcessWaiting` listener panic span 必须保持在 `RunAgent` 的 parent trace 内。
  - 本轮是兼容性 API 增量，不改变既有 `AwaitInput(req)` 或 `core.Process` 接口；无公共 API 破坏性调整。
- 已完成第六十轮目标模块生产代码卫生治理：
  - `agent/core`：统一 `ProcessContext` 工具相关入口的 nil ctx 语义，`PublishContext`、`ResolveTools`、`ActionTools`、`ToolCallContext` 均先归一到 `context.Background()`。
  - 修复 `ToolCallContext(nil)` 直接 panic 的 API 易误用点；resolver 也不再收到 nil ctx，避免把框架入口的不稳定状态透传给下游工具解析器。
  - 新增 `agent/core` 单元测试覆盖 nil ctx 下的 tool resolver 调用、action tool resolver 调用、tool-call cancel/release 行为。
  - 本轮不新增兼容旧接口层，不改变公开签名；属于现有 API 语义收紧和难误用治理。
- 已完成第六十一轮 `app/runtime` internal API 收敛：
  - 删除 `kernel.Engine.RunTurn` 同步 wrapper，生产 turn 执行入口收敛为 `StartTurn(ctx, req) -> TurnProcess`。
  - `internal/runtime` 的 A2A adapter 改为直接使用 `StartTurn`，等待 `TurnProcess.Done()` 后读取 `TurnProcess.Output()`；不为旧同步入口保留兼容层。
  - `kernel` 包测试迁移到 test-only `runTurnSync` helper，测试仍覆盖同步等待语义，但生产 API 不再暴露重复入口。
  - 清理 `Engine.RunTurn` 相关注释，避免文档继续指向已删除接口。
- 已完成第六十二轮 `app/runtime` internal 命名与执行模型收敛：
  - `kernel.RunTurnRequest` 直接重命名为 `kernel.TurnRequest`，与已删除的 `Engine.RunTurn` 脱钩，不保留 type alias。
  - 同步迁移 `kernel.Engine.StartTurn`、turn dispatcher、A2A adapter、kernel tests 和相关注释。
  - 清理测试错误信息中的旧 `RunTurn` 文案；后续第六十三、六十四轮已继续收敛 lifecycle 领域类型和取消用例命名。
- 已完成第六十三轮 `app/runtime` lifecycle 领域命名收敛：
  - `lifecycle.RunTurn` 直接重命名为 `RunTurnBinding`，表达它是 protocol run id 与 turn handle 的绑定，而不是执行动作或旧同步入口。
  - `RunTurnBinding` 增加内部 `handle()` 转换方法，取消逻辑通过绑定对象生成 `turn.TurnHandle`，减少跨函数重复组装。
  - 同步迁移 runtime/server/lifecycle 调用点和测试，不保留旧类型别名。
- 已完成第六十四轮 `app/runtime` lifecycle 用例命名收敛：
  - `CancelRunTurn` 直接重命名为 `CancelRunBinding`，与 `RunTurnBinding` 的领域对象对齐，不再把取消用例命名成旧 `RunTurn` 动作。
  - 同步迁移 lifecycle coordinator、runtime facade、server lifecycle port 和测试 fixture，不保留旧方法别名。
  - 该调整仍位于 `app/runtime/internal`，不影响跨模块公开 API。
- 已完成第六十五轮 `app/runtime` server MCP 端口 ISP 收敛：
  - 将 delivery/server 的胖 `mcpAccess` 拆为 `mcpStatusAccess`、`mcpToolCatalogAccess`、`mcpConnectionAccess`、`mcpRegistryAccess`。
  - `RuntimePort` 仍是组合口，但 `runtimeBindings` 按 status/tools/connections/registry 保存最小能力，MCP handler 只依赖各自实际用到的端口字段。
  - 同步迁移 workspace MCP status/tool/config/reconnect request helpers；不保留旧 `mcpAccess` 聚合字段。
- 已完成第六十六轮 `app/runtime` server provider 端口 ISP 收敛：
  - 将 delivery/server 的胖 `providerAccess` 拆为 `providerRegistryAccess`、`providerCatalogAccess`、`providerDefaultAccess`。
  - providers handler 依赖 registry/catalog，models role validation 依赖 registry/catalog，usage aggregation 只依赖 default provider。
  - 同步迁移 runtime binding 与模型角色测试 fixture，不保留旧 `providerAccess` 聚合字段。
- 已完成第六十七轮 `app/runtime` server session 端口 ISP 收敛：
  - 将 delivery/server 的胖 `sessionAccess` 拆为 `sessionCatalogAccess`、`sessionMutationAccess`、`sessionDefaultModelAccess`。
  - session read/export/rollback/resume/workspace discovery 依赖 catalog，create/update/delete/scheduler 依赖 mutation，session wire/usage 默认模型归因只依赖 default model。
  - 同步迁移 runtime binding 和所有 server handler 调用点，不保留旧 `sessionAccess` 聚合字段。
- 已完成第六十八轮 `app/runtime` server schedule 端口 ISP 收敛：
  - 将 delivery/server 的胖 `scheduleAccess` 拆为 `scheduleCatalogAccess`、`scheduleMutationAccess`、`scheduleRunRecorderAccess`、`scheduleWorkerAccess`。
  - schedule CRUD handlers 依赖 catalog/mutation，manual run 只额外依赖 run recorder，background scheduler entry 只依赖 worker。
  - 同步迁移 runtime binding、schedules handler 和 scheduler entry，不保留旧 `scheduleAccess` 聚合字段。
- 已完成第六十九轮 `app/runtime` server memory 端口 ISP 收敛：
  - 将 delivery/server 的 `knowledgeAccess` 拆为 `memoryAvailabilityAccess` 与 `memoryStoreAccess`。
  - memory handlers 使用 availability 作为 feature gate，读写路径只依赖 store；capability snapshot 不再通过 `runtimeBindings.capabilities` 聚合字段回到胖口。
  - 同步迁移 runtime binding 和 memory handler，不保留旧 `knowledgeAccess` 聚合字段。
- 已完成第七十轮 `app/runtime` server transcript 端口 ISP 收敛：
  - 将 delivery/server 的 `transcriptAccess` 拆为 `transcriptContentAccess` 与 `transcriptRunAccess`。
  - session export/items/rollback/fork 依赖完整 transcript content，usage aggregation 只依赖 run 列表。
  - 同步迁移 runtime binding 和 server handler 调用点，不保留旧 `transcriptAccess` 聚合字段。
- 已完成第七十一轮 `app/runtime` server settings 端口 ISP 收敛：
  - 将 delivery/server 的 `toolAccess` 拆为 `toolCatalogAccess` 与 `toolInvocationAccess`。
  - 将 delivery/server 的 `approvalAccess` 拆为 `approvalModeAccess` 与 `approvalRuleAccess`。
  - tools list 只依赖工具目录，direct invoke 只依赖工具调用；approval mode endpoints 与 persisted rule endpoints 各依赖自己的最小端口。
- 已完成第七十二轮 `app/runtime` server lifecycle 端口 ISP 收敛：
  - 将 delivery/server 的 `lifecycleAccess` 拆为 `runAdmissionAccess`、`sessionMutationAdmissionAccess`、`runResumeAccess`、`runCancellationAccess` 与 `sessionLifecycleMutationAccess`。
  - runs.start/resume 的 run admission、rollback/delete 的 mutation admission、runs.cancel、runs.resume continuation、session rollback/fork/import 各依赖自己的生命周期用例端口。
  - 同步迁移 runtime binding、run/session handlers 和测试调用点，不保留旧 `lifecycleAccess` 聚合字段。
- 已完成第七十三轮 `app/runtime` server turn 端口 ISP 收敛：
  - 将 delivery/server 的 `turnAccess` 拆为 `turnStartAccess`、`turnStreamAccess`、`turnSteeringAccess` 与 `turnInterruptPolicyAccess`。
  - `RuntimePort` 不再要求 `ResumeTurn`、`RehydrateTurn`、`TurnProcessID` 这些 server 不直接调用的 turn 能力；resume/rehydrate/process lookup 继续藏在 lifecycle/runsegment runtime facade 后面。
  - runs.start、run pump、runs.steer、runtime.initialize 各依赖自己的最小 turn 能力，不保留旧 `turnAccess` 聚合字段。
- 已完成第七十四轮 `app/runtime` server workspace/codebase 端口 ISP 收敛：
  - 将 delivery/server 的 `workspaceCatalogAccess` 拆为 `skillCatalogAccess` 与 `recipeCatalogAccess`。
  - 将 delivery/server 的 `codebaseAccess` 拆为 `codebaseAvailabilityAccess`、`codebaseSearchAccess`、`codebaseStatusAccess` 与 `codebaseReindexAccess`。
  - workspace skills/recipes、codebase availability/search/status/reindex 各依赖自己的最小能力，不保留旧 workspace/codebase 聚合字段。
- 已完成第七十五轮 `app/runtime` server hook/model-role 端口 ISP 收敛：
  - 将 delivery/server 的 `hookAccess` 拆为 `hookInspectionAccess` 与 `hookTrustAccess`。
  - 将 delivery/server 的 `modelRoleAccess` 拆为 `utilityRoleAccess` 与 `embeddingRoleAccess`。
  - hooks list/trust toggle、utility role、embedding role 各依赖自己的最小能力，不保留旧 hooks/modelRoles 聚合字段。
- 已完成第七十六轮 `app/runtime` server MCP registry 端口 ISP 收敛：
  - 将 delivery/server 的 `mcpRegistryAccess` 拆为 `mcpRegistryCatalogAccess`、`mcpRegistryMutationAccess` 与 `mcpRegistryProbeAccess`。
  - MCP config list/request authorization preservation、configure/remove/setEnabled、test probe 各依赖自己的最小 registry 能力。
  - 同步迁移 runtime binding、workspace MCP config handlers 和 request helper，不保留旧 `mcpRegistryAccess` 聚合字段。
- 已完成第七十七轮 `app/runtime` server provider registry 端口 ISP 收敛：
  - 将 delivery/server 的 `providerRegistryAccess` 拆为 `providerRegistryCatalogAccess`、`providerRegistryMutationAccess` 与 `providerRegistryProbeAccess`。
  - providers list/configure/test 与 model role configured-provider validation 各依赖自己的最小 provider registry 能力。
  - 同步迁移 runtime binding、providers handler、models role validation 和测试 fixture，不保留旧 `providerRegistryAccess` 聚合字段。
- 已完成第七十八轮 `app/runtime` server session lifecycle 端口 ISP 收敛：
  - 将 delivery/server 的 `sessionLifecycleMutationAccess` 拆为 `sessionRollbackAccess`、`sessionForkAccess` 与 `sessionRestoreAccess`。
  - rollback、fork、import restore 三条 session lifecycle 写用例各依赖自己的最小能力。
  - 同步迁移 runtime binding、rollback/session/session import handlers 和 restore 测试，不保留旧 `sessionLifecycleMutationAccess` 聚合字段。
- 已完成第七十九轮 `app/runtime` server admission 端口 ISP 收敛：
  - 将 delivery/server 的 `runAdmissionAccess` 拆为 `runSlotAdmissionAccess` 与 `workingTreeRunAdmissionAccess`。
  - 将 delivery/server 的 `sessionMutationAdmissionAccess` 拆为 `sessionMutationSlotAccess` 与 `workingTreeMutationAccess`。
  - runs.start/sessions.import、runs.resume、sessions.delete/rollback、file-restore rollback 各依赖自己的最小 admission 能力，不保留旧 admission 聚合字段。
- 已完成第八十轮 `app/runtime` server session mutation 端口 ISP 收敛：
  - 将 delivery/server 的 `sessionMutationAccess` 拆为 `sessionCreationAccess`、`sessionDeletionAccess` 与 `sessionUpdateAccess`。
  - schedule runner 与 sessions.create 只依赖创建能力，sessions.delete 只依赖 deletion cascade，sessions.update 只依赖 patch update。
  - 同步迁移 runtime binding、scheduler 和 sessions handler，不保留旧 `sessionMutationAccess` 聚合字段。
- 已完成第八十一轮 `app/runtime` server memory store 端口 ISP 收敛：
  - 将 delivery/server 的 `memoryStoreAccess` 拆为 `memoryListAccess`、`memoryReadAccess` 与 `memoryWriteAccess`。
  - memory.list、memory.get、memory.update 各依赖自己的最小 memory store 能力。
  - 同步迁移 runtime binding 和 memory handler，不保留旧 `memoryStoreAccess` 聚合字段。
- 已完成第八十二轮 `app/runtime` server approval 端口 ISP 收敛：
  - 将 delivery/server 的 `approvalModeAccess` 拆为 `approvalModeReadAccess` 与 `approvalModeMutationAccess`。
  - 将 delivery/server 的 `approvalRuleAccess` 拆为 `approvalRuleCatalogAccess` 与 `approvalRuleMutationAccess`。
  - approval.getMode、approval.setMode、approval.listRules、approval.forgetRule 各依赖自己的最小端口，不保留旧 approval 聚合字段。
- 已完成第八十三轮 `app/runtime` server MCP connection 命令端口 ISP 收敛：
  - 将 delivery/server 的 `mcpConnectionAccess` 拆为 `mcpReconnectAccess` 与 `mcpAuthorizationAccess`。
  - workspace.mcp.reconnect 与 workspace.mcp.authorize 共享状态事件 helper，但各自依赖自己的 runtime 命令能力，不再通过同一个连接聚合字段互相可见。
- 已完成第八十四轮 `app/runtime` server schedule 用例端口 ISP 收敛：
  - 将 delivery/server 的 `scheduleCatalogAccess` 拆为 `scheduleListAccess` 与 `scheduleReadAccess`。
  - 将 delivery/server 的 `scheduleMutationAccess` 拆为 `scheduleCreationAccess`、`scheduleUpdateAccess` 与 `scheduleDeletionAccess`。
  - schedules.list、create、update、delete、runNow 各自依赖自己的最小 schedule 用例能力，不保留旧 schedule 聚合字段。
- 已完成第八十五轮 `app/runtime` server provider registry/catalog 端口 ISP 收敛：
  - 将 delivery/server 的 `providerRegistryCatalogAccess` 拆为 `providerRegistryListAccess` 与 `providerRegistryReadAccess`。
  - 将 delivery/server 的 `providerCatalogAccess` 拆为 `providerSupportCatalogAccess` 与 `providerMetadataAccess`。
  - providers.list、providers.configure/test、model role validation 各自依赖自己的 provider registry/catalog read 能力，不保留旧 provider 聚合字段。
- 已完成第八十六轮 `app/runtime` server MCP registry 用例端口 ISP 收敛：
  - 将 delivery/server 的 `mcpRegistryCatalogAccess` 拆为 `mcpRegistryListAccess` 与 `mcpRegistryReadAccess`。
  - 将 delivery/server 的 `mcpRegistryMutationAccess` 拆为 `mcpRegistryConfigureAccess`、`mcpRegistryRemoveAccess` 与 `mcpRegistryEnableAccess`。
  - workspace.mcp.listConfigs、configure、remove、setEnabled、test-token-preservation 各自依赖自己的最小 registry 能力，不保留旧 MCP registry 聚合字段。
- 已完成第八十七轮 `agent` context-first 公开 API 破坏性收敛：
  - 用户确认方案 A 后，将 event listener、`core.Process`、`core.ProcessContext` 与 `runtime.AgentProcess` 的 publish / await / invocation 记录入口统一为显式 `context.Context` first。
  - 删除前序兼容期的 `PublishContext`、`AwaitInputContext`、`Record*Context`、`OnEventContext` companion 公开入口和无 ctx wrapper；不再通过 `ProcessContext` 隐式保存 action ctx。
  - 同步迁移 `app/runtime/internal/kernel` HITL / turn loop 调用、agent runtime tests、examples 和 agent docs，避免公开文档继续指向旧 API。
- 已完成第八十八轮 `app/runtime` server session catalog 端口 ISP 收敛：
  - 将 delivery/server 的 `sessionCatalogAccess` 拆为 `sessionListAccess` 与 `sessionReadAccess`。
  - sessions.list、usage.summary、workspace.projects 只依赖 session list read model；sessions.get/export/import/rollback/resume/workspace stream 只依赖 single-session lookup。
  - 同步迁移 runtime binding 和 handler 调用，不保留旧 `sessionCatalogAccess` 聚合字段。
- 已完成第八十九轮 `app/runtime` server model role 端口读写分离：
  - 将 delivery/server 的 `utilityRoleAccess` 拆为 `utilityRoleReadAccess` 与 `utilityRoleMutationAccess`。
  - 将 delivery/server 的 `embeddingRoleAccess` 拆为 `embeddingRoleReadAccess` 与 `embeddingRoleMutationAccess`。
  - models.get*Role 只依赖 read 端口；models.set*Role 依赖 mutation 写入后再通过 read 端口读回 stored role，不保留旧读写混合字段。
- 已完成定向验证：
  - `go test ./internal/arch`（`core`）通过。
  - `go test ./internal/arch`（`agent`）通过。
  - `go test ./internal/kernel/... ./internal/runtime/... ./internal/delivery/server/...`（`app/runtime`）通过。
  - `go test ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/adapter/hooks ./cmd/lyra`（`app/runtime`）通过。
  - `go test ./cmd/lyra`（`app/runtime`）通过。
  - `go test ./model`（`core`）通过。
  - `go test ./event`（`agent`）通过。
  - `go test ./runtime ./runtime/autonomy ./workflow`（`agent`）通过。
  - `go test ./model/chat/middleware/logger ./model/chat/middleware/history ./model/chat/middleware/safeguard`（`core`）通过。
  - `go test ./internal/infra/mcp`（`app/runtime`）通过。
  - `go test ./internal/kernel/turn`（`app/runtime`）通过。
  - `go test ./internal/runtime`（`app/runtime`）通过。
  - `go test ./model/chat`（`core`）通过。
  - `go test ./internal/kernel/turn ./internal/runtime ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/kernel/...`（`app/runtime`）通过。
  - `go test ./internal/kernel/turn ./internal/kernel/... ./internal/runtime ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/kernel/lifecycle ./internal/kernel/... ./internal/runtime ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/infra/mcp ./internal/infra/... ./internal/runtime ./internal/adapter/toolset/... ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/adapter/toolset/...`（`app/runtime`）通过。
  - `go test ./internal/delivery/dispatch`（`app/runtime`）通过（包内无测试文件，编译通过）。
  - `go test ./internal/infra/lsp`（`app/runtime`）通过。
  - `go test ./toolloop`（`agent`）通过。
  - `go test ./vectorstore/filter/parser ./vectorstore/filter/...`（`core`）通过。
  - `go test ./vectorstore/filter/lexer ./vectorstore/filter/...`（`core`）通过。
  - `go test ./cmd/lyra`（`app/runtime`）通过。
  - `go test ./internal/kernel/turn`（`app/runtime`）通过。
  - `go test ./runtime`（`agent`）通过。
  - `go test ./internal/config`（`app/runtime`）通过。
  - `go test ./internal/delivery/protocol`（`app/runtime`）通过。
  - `go test ./internal/delivery/dispatch ./internal/delivery/server`（`app/runtime`）通过。
  - `go test ./internal/infra/storage/sqlite`（`app/runtime`）通过。
  - `go test ./internal/infra/checkpoint`（`app/runtime`）通过。
  - `go test ./internal/domain/codebaseindex`（`app/runtime`）通过。
  - `go test ./internal/runtime`（`app/runtime`）通过（第二十七轮后复跑）。
  - `go test ./internal/infra/git`（`app/runtime`）通过（第二十八轮后复跑）。
  - `go test ./internal/adapter/maintenance`（`app/runtime`）通过（第二十九轮后复跑）。
  - `go test ./model/chat/middleware/safeguard`（`core`）通过（第三十轮后复跑）。
  - `go test ./planning/planner/htn`（`agent`）通过（第三十一轮后复跑）。
  - `go test ./vectorstore/filter/...`（`core`）通过（第三十二轮后复跑）。
  - `go test ./vectorstore`（`core`）通过（第三十三轮后复跑）。
  - `go test ./model/chat`（`core`）通过（第三十四轮后复跑）。
  - `go test ./model/audio/tts`（`core`）通过（第三十五轮后复跑）。
  - `go test ./model/embedding`（`core`）通过（第三十六轮后复跑）。
  - `go test ./model/moderation`（`core`）通过（第三十七轮后复跑）。
  - `go test ./model/audio/transcription`（`core`）通过（第三十八轮后复跑）。
  - `go test ./model/image`（`core`）通过（第三十九轮后复跑）。
  - `go test ./model/chat`（`core`）通过（第四十轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第四十一轮后复跑）。
  - `go test ./toolloop`（`agent`）通过（第四十二轮后复跑）。
  - `go test ./internal/runtime`（`app/runtime`）通过（第四十三轮后复跑）。
  - `go test ./internal/kernel/turn`（`app/runtime`）通过（第四十四轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第四十五轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第四十六轮后复跑）。
  - `go test ./toolloop`（`agent`）通过（第四十七轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第四十八轮后复跑）。
  - `go test ./internal/delivery/transport/http`（`app/runtime`）通过（第四十九轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第五十轮后复跑）。
  - `go test ./internal/kernel/turn`（`app/runtime`）通过（第五十一轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第五十二轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第五十三轮后复跑）。
  - `go test ./toolloop`（`agent`）通过（第五十四轮后复跑）。
  - `go test ./internal/kernel/lifecycle`（`app/runtime`）通过（第五十五轮后复跑）。
  - `go test ./event ./runtime`（`agent`）通过（第五十六轮后复跑）。
  - `go test ./core ./event ./runtime`（`agent`）通过（第五十七轮后复跑）。
  - `go test ./core ./event ./runtime`（`agent`）通过（第五十八轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第五十八轮后复跑）。
  - `go test ./core ./event ./runtime`（`agent`）通过（第五十九轮后复跑）。
  - `go test ./runtime -run 'TestProcessContextAwaitInputKeepsActionTrace|TestTypedActionAwaitInputSuspendsAndResumes'`（`agent`）通过（第五十九轮后复跑）。
  - `go test ./core -run 'TestProcessContextTool'`（`agent`）通过（第六十轮后复跑）。
  - `go test ./core`（`agent`）通过（第六十轮后复跑）。
  - `go test ./internal/kernel`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go test ./internal/runtime`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go test ./internal/runtime -run TestA2AAgent_RunYieldsReply`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go test ./internal/kernel ./internal/kernel/turn ./internal/runtime`（`app/runtime`）通过（第六十二轮后复跑）。
  - `go test ./internal/kernel/lifecycle ./internal/runtime ./internal/delivery/server`（`app/runtime`）通过（第六十三轮后复跑）。
  - `go test ./internal/kernel/lifecycle ./internal/runtime ./internal/delivery/server`（`app/runtime`）通过（第六十四轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第六十五轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第六十六轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第六十七轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第六十八轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第六十九轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十一轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十二轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十三轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十四轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十五轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十六轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十七轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十八轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第七十九轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十一轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十二轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十三轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十四轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十五轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十七轮 public API 迁移后先跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十八轮后复跑）。
  - `go test ./internal/delivery/server`（`app/runtime`）通过（第八十九轮后复跑）。
- 已完成三模块回归验证：
  - `go test ./...`（`core`）通过（第四十五轮后复跑）。
  - `go test ./...`（`agent`）通过（第四十五轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第四十五轮后复跑）。
  - `go vet ./...`（`core`）通过（第四十五轮后复跑）。
  - `go vet ./...`（`agent`）通过（第四十五轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第四十五轮后复跑）。
  - `go build ./...`（`core`）通过（第四十五轮后复跑）。
  - `go build ./...`（`agent`）通过（第四十五轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第四十五轮后复跑）。
  - `go test ./...`（`core`）通过（第四十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第四十六轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第四十六轮后复跑）。
  - `go vet ./...`（`core`）通过（第四十六轮后复跑）。
  - `go vet ./...`（`agent`）通过（第四十六轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第四十六轮后复跑）。
  - `go build ./...`（`core`）通过（第四十六轮后复跑）。
  - `go build ./...`（`agent`）通过（第四十六轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第四十六轮后复跑）。
  - `go test ./...`（`core`）通过（第四十七轮后复跑）。
  - `go test ./...`（`agent`）通过（第四十七轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第四十七轮后复跑）。
  - `go vet ./...`（`core`）通过（第四十七轮后复跑）。
  - `go vet ./...`（`agent`）通过（第四十七轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第四十七轮后复跑）。
  - `go build ./...`（`core`）通过（第四十七轮后复跑）。
  - `go build ./...`（`agent`）通过（第四十七轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第四十七轮后复跑）。
  - `go test ./...`（`core`）通过（第四十八轮后复跑）。
  - `go test ./...`（`agent`）通过（第四十八轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第四十八轮后复跑）。
  - `go vet ./...`（`core`）通过（第四十八轮后复跑）。
  - `go vet ./...`（`agent`）通过（第四十八轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第四十八轮后复跑）。
  - `go build ./...`（`core`）通过（第四十八轮后复跑）。
  - `go build ./...`（`agent`）通过（第四十八轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第四十八轮后复跑）。
  - `go test ./...`（`core`）通过（第四十九轮后复跑）。
  - `go test ./...`（`agent`）通过（第四十九轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第四十九轮后复跑）。
  - `go vet ./...`（`core`）通过（第四十九轮后复跑）。
  - `go vet ./...`（`agent`）通过（第四十九轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第四十九轮后复跑）。
  - `go build ./...`（`core`）通过（第四十九轮后复跑）。
  - `go build ./...`（`agent`）通过（第四十九轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第四十九轮后复跑）。
  - `go test ./...`（`core`）通过（第五十轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十轮后复跑）。
  - `go build ./...`（`core`）通过（第五十轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十轮后复跑）。
  - `go test ./...`（`core`）通过（第五十一轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十一轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十一轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十一轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十一轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十一轮后复跑）。
  - `go build ./...`（`core`）通过（第五十一轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十一轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十一轮后复跑）。
  - `go test ./...`（`core`）通过（第五十二轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十二轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十二轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十二轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十二轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十二轮后复跑）。
  - `go build ./...`（`core`）通过（第五十二轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十二轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十二轮后复跑）。
  - `go test ./...`（`core`）通过（第五十三轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十三轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十三轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十三轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十三轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十三轮后复跑）。
  - `go build ./...`（`core`）通过（第五十三轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十三轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十三轮后复跑）。
  - `go test ./...`（`core`）通过（第五十四轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十四轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十四轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十四轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十四轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十四轮后复跑）。
  - `go build ./...`（`core`）通过（第五十四轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十四轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十四轮后复跑）。
  - `go test ./...`（`core`）通过（第五十五轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十五轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十五轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十五轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十五轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十五轮后复跑）。
  - `go build ./...`（`core`）通过（第五十五轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十五轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十五轮后复跑）。
  - `go test ./...`（`core`）通过（第五十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十六轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十六轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十六轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十六轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十六轮后复跑）。
  - `go build ./...`（`core`）通过（第五十六轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十六轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十六轮后复跑）。
  - `go test ./...`（`core`）通过（第五十七轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十七轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十七轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十七轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十七轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十七轮后复跑）。
  - `go build ./...`（`core`）通过（第五十七轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十七轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十七轮后复跑）。
  - `go test ./...`（`core`）通过（第五十八轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十八轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十八轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十八轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十八轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十八轮后复跑）。
  - `go build ./...`（`core`）通过（第五十八轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十八轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十八轮后复跑）。
  - `go test ./...`（`core`）通过（第五十九轮后复跑）。
  - `go test ./...`（`agent`）通过（第五十九轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第五十九轮后复跑）。
  - `go vet ./...`（`core`）通过（第五十九轮后复跑）。
  - `go vet ./...`（`agent`）通过（第五十九轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第五十九轮后复跑）。
  - `go build ./...`（`core`）通过（第五十九轮后复跑）。
  - `go build ./...`（`agent`）通过（第五十九轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第五十九轮后复跑）。
  - `go test ./...`（`core`）通过（第六十轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十轮后复跑）。
  - `go build ./...`（`core`）通过（第六十轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十轮后复跑）。
  - `go test ./...`（`core`）通过（第六十一轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十一轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十一轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十一轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go build ./...`（`core`）通过（第六十一轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十一轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十一轮后复跑）。
  - `go test ./...`（`core`）通过（第六十二轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十二轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十二轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十二轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十二轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十二轮后复跑）。
  - `go build ./...`（`core`）通过（第六十二轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十二轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十二轮后复跑）。
  - `go test ./...`（`core`）通过（第六十三轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十三轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十三轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十三轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十三轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十三轮后复跑）。
  - `go build ./...`（`core`）通过（第六十三轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十三轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十三轮后复跑）。
  - `go test ./...`（`core`）通过（第六十四轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十四轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十四轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十四轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十四轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十四轮后复跑）。
  - `go build ./...`（`core`）通过（第六十四轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十四轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十四轮后复跑）。
  - `go test ./...`（`core`）通过（第六十五轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十五轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十五轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十五轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十五轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十五轮后复跑）。
  - `go build ./...`（`core`）通过（第六十五轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十五轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十五轮后复跑）。
  - `go test ./...`（`core`）通过（第六十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十六轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十六轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十六轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十六轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十六轮后复跑）。
  - `go build ./...`（`core`）通过（第六十六轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十六轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十六轮后复跑）。
  - `go test ./...`（`core`）通过（第六十七轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十七轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十七轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十七轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十七轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十七轮后复跑）。
  - `go build ./...`（`core`）通过（第六十七轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十七轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十七轮后复跑）。
  - `go test ./...`（`core`）通过（第六十八轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十八轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十八轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十八轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十八轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十八轮后复跑）。
  - `go build ./...`（`core`）通过（第六十八轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十八轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十八轮后复跑）。
  - `go test ./...`（`core`）通过（第六十九轮后复跑）。
  - `go test ./...`（`agent`）通过（第六十九轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第六十九轮后复跑）。
  - `go vet ./...`（`core`）通过（第六十九轮后复跑）。
  - `go vet ./...`（`agent`）通过（第六十九轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第六十九轮后复跑）。
  - `go build ./...`（`core`）通过（第六十九轮后复跑）。
  - `go build ./...`（`agent`）通过（第六十九轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第六十九轮后复跑）。
  - `go test ./...`（`core`）通过（第七十轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十轮后复跑）。
  - `go build ./...`（`core`）通过（第七十轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十轮后复跑）。
  - `go test ./...`（`core`）通过（第七十一轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十一轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十一轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十一轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十一轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十一轮后复跑）。
  - `go build ./...`（`core`）通过（第七十一轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十一轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十一轮后复跑）。
  - `go test ./...`（`core`）通过（第七十二轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十二轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十二轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十二轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十二轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十二轮后复跑）。
  - `go build ./...`（`core`）通过（第七十二轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十二轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十二轮后复跑）。
  - `go test ./...`（`core`）通过（第七十三轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十三轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十三轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十三轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十三轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十三轮后复跑）。
  - `go build ./...`（`core`）通过（第七十三轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十三轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十三轮后复跑）。
  - `go test ./...`（`core`）通过（第七十四轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十四轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十四轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十四轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十四轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十四轮后复跑）。
  - `go build ./...`（`core`）通过（第七十四轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十四轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十四轮后复跑）。
  - `go test ./...`（`core`）通过（第七十五轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十五轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十五轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十五轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十五轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十五轮后复跑）。
  - `go build ./...`（`core`）通过（第七十五轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十五轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十五轮后复跑）。
  - `go test ./...`（`core`）通过（第七十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十六轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十六轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十六轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十六轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十六轮后复跑）。
  - `go build ./...`（`core`）通过（第七十六轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十六轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十六轮后复跑）。
  - `go test ./...`（`core`）通过（第七十七轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十七轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十七轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十七轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十七轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十七轮后复跑）。
  - `go build ./...`（`core`）通过（第七十七轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十七轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十七轮后复跑）。
  - `go test ./...`（`core`）通过（第七十八轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十八轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十八轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十八轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十八轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十八轮后复跑）。
  - `go build ./...`（`core`）通过（第七十八轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十八轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十八轮后复跑）。
  - `go test ./...`（`core`）通过（第七十九轮后复跑）。
  - `go test ./...`（`agent`）通过（第七十九轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第七十九轮后复跑）。
  - `go vet ./...`（`core`）通过（第七十九轮后复跑）。
  - `go vet ./...`（`agent`）通过（第七十九轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第七十九轮后复跑）。
  - `go build ./...`（`core`）通过（第七十九轮后复跑）。
  - `go build ./...`（`agent`）通过（第七十九轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第七十九轮后复跑）。
  - `go test ./...`（`core`）通过（第八十轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十轮后复跑）。
  - `go build ./...`（`core`）通过（第八十轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十轮后复跑）。
  - `go test ./...`（`core`）通过（第八十一轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十一轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十一轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十一轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十一轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十一轮后复跑）。
  - `go build ./...`（`core`）通过（第八十一轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十一轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十一轮后复跑）。
  - `go test ./...`（`core`）通过（第八十二轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十二轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十二轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十二轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十二轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十二轮后复跑）。
  - `go build ./...`（`core`）通过（第八十二轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十二轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十二轮后复跑）。
  - `go test ./...`（`core`）通过（第八十三轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十三轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十三轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十三轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十三轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十三轮后复跑）。
  - `go build ./...`（`core`）通过（第八十三轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十三轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十三轮后复跑）。
  - `go test ./...`（`core`）通过（第八十四轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十四轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十四轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十四轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十四轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十四轮后复跑）。
  - `go build ./...`（`core`）通过（第八十四轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十四轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十四轮后复跑）。
  - `go test ./...`（`core`）通过（第八十五轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十五轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十五轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十五轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十五轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十五轮后复跑）。
  - `go build ./...`（`core`）通过（第八十五轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十五轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十五轮后复跑）。
  - `go test ./...`（`core`）通过（第八十六轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十六轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十六轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十六轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十六轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十六轮后复跑）。
  - `go build ./...`（`core`）通过（第八十六轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十六轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十六轮后复跑）。
  - `go test ./...`（`core`）通过（第八十七轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十七轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十七轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十七轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十七轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十七轮后复跑）。
  - `go build ./...`（`core`）通过（第八十七轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十七轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十七轮后复跑）。
  - `go test ./...`（`core`）通过（第八十八轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十八轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十八轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十八轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十八轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十八轮后复跑）。
  - `go build ./...`（`core`）通过（第八十八轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十八轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十八轮后复跑）。
  - `go test ./...`（`core`）通过（第八十九轮后复跑）。
  - `go test ./...`（`agent`）通过（第八十九轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第八十九轮后复跑）。
  - `go vet ./...`（`core`）通过（第八十九轮后复跑）。
  - `go vet ./...`（`agent`）通过（第八十九轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第八十九轮后复跑）。
  - `go build ./...`（`core`）通过（第八十九轮后复跑）。
  - `go build ./...`（`agent`）通过（第八十九轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第八十九轮后复跑）。
- 已完成目标模块低误伤异味扫描：
  - 常量 `fmt.Errorf("...")` 未命中。
  - `TODO` / `FIXME` / `HACK` 未命中。
  - `McpToolInfo` / `McpServerStatus` / `notImpl` 未命中。
  - `helpers_test.go` / 泛测试文件名未命中。
  - `core` Go 文件未命中 `Lyra`、`app/runtime`、`vectorstores`、`workspace.*` 等上层 / sibling 模块语义。
  - `agent` Go 文件除架构测试自身的禁止规则外，未命中 `Lyra`、`app/runtime`、`workspace.*` 等上层应用语义。
- 已运行 `git diff --check`，未发现 whitespace/error marker 问题。

### 5.2 未完成

- 第八十七轮已完成 `agent` 前序 context-aware companion 公开入口清理；后续若继续调整其他 exported shape，仍需先确认 scope、影响面和备选方案。
- 本批只处理 `core`、`agent`、`app/runtime`；前端和其他模块不纳入本轮质量结论。

### 5.3 暂不处理的问题

- `agent/runtime/mcp.go` 直接承载 MCP 便利集成：已有 agent 架构评审明确裁决为“关注点不同但紧密依赖 `*Platform`/`*AgentProcess`，当前保持在 runtime 包内”。本轮不拆。
- `agent/core.ServiceProvider` 是开放 service locator：已有 agent 架构评审裁决为 SDK 库的 architecture tax，可接受。本轮不改。
- `app/runtime` 的 `delivery/server` 总 LOC 较大：已有文档裁决协议方法 1:1 绑定是健康的，且原 use-case 泄漏 P0 已落地。本轮不按文件数拆。

### 5.4 风险与注意事项

- 本任务允许破坏性调整，且目标已更新为不为旧接口留兼容 shim；第八十七轮公共 API 调整已按用户确认的 A 方案执行，后续其他公共 API 破坏性修改仍需先确认 scope、影响面和备选方案。
- `agent` 和 `app/runtime` 体量较大，需要分批推进，避免无目标重写。
- 多模块 workspace 中的模块版本引用可能由 `go.work` 覆盖，依赖治理需要同时看 `go.mod` 和实际 import。
- `core` 与 `agent` 均依赖 `pkg` 工具模块；后续只治理有明确收益、能减少真实耦合的用法，不为了“依赖更少”而机械复制 helper。
- 近期 internal 破坏性调整已记录在第 7 节；新增 arch tests 会在未来违反依赖边界时让测试失败，这是预期的防腐行为。

---

## 6. 后续演进方向

- 根据依赖扫描结果补充架构测试或依赖规则检查。
- 根据模块细读结果判断是否需要拆包、接口收窄或公共 API 收敛。
- 记录无法在本轮完成但值得后续治理的问题。

---

## 7. 破坏性调整记录

### 7.1 调整项模板

- 调整对象：
- 调整前问题：
- 破坏性原因：
- 新设计：
- 架构收益：
- 影响范围：
- 已完成适配：
- 验证结果：
- 后续风险：

### 7.2 本轮记录

第六十一轮包含 `app/runtime/internal/kernel.Engine` 的 internal 破坏性调整：

- 调整对象：`kernel.Engine.RunTurn`。
- 调整前问题：`Engine` 同时暴露 `StartTurn` 与 `RunTurn` 两条 turn 执行入口，后者只是同步 wrapper，生产侧 A2A 与测试继续依赖它，导致应用层编排 API 重复。
- 破坏性原因：目标已更新为不保留旧接口兼容层；`RunTurn` 是 internal 包 API，不是跨模块公开 API，直接删除比继续维护 wrapper 更符合单一入口和低耦合。
- 新设计：生产代码统一使用 `StartTurn` 返回的 `TurnProcess`；同步等待仅在测试中通过 test-only helper 表达。
- 架构收益：turn 生命周期控制（cancel/resume/status/output）只通过 `TurnProcess` 暴露，A2A、turn dispatcher、runtime 共享同一执行模型。
- 影响范围：`app/runtime/internal/runtime/a2aagent.go` 与 `app/runtime/internal/kernel` 包测试。
- 已完成适配：A2A adapter 已迁移到 `StartTurn`；kernel 测试改用 `runTurnSync` helper。
- 验证结果：`go test ./internal/kernel`、`go test ./internal/runtime`、`go test ./internal/runtime -run TestA2AAgent_RunYieldsReply` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；若后续有新的同步调用需求，应在调用方显式等待 `TurnProcess`，不要恢复生产 wrapper。

第六十二轮包含 `app/runtime/internal/kernel` 的 internal 破坏性重命名：

- 调整对象：`kernel.RunTurnRequest`。
- 调整前问题：`Engine.RunTurn` 删除后，核心 turn 请求类型仍保留旧同步入口命名，概念上继续暗示不存在的生产 API。
- 破坏性原因：目标已更新为不保留旧接口兼容层；该类型位于 `app/runtime/internal`，直接改名比保留 alias 更能避免历史包袱。
- 新设计：统一使用 `kernel.TurnRequest` 表达 Engine turn 输入；`turn.StartTurnRequest` 继续作为 dispatcher/protocol-adjacent 的入口请求。
- 架构收益：kernel 层类型命名回到领域对象本身，不再绑定旧方法名；StartTurn/TurnProcess 成为唯一生产执行模型。
- 影响范围：`app/runtime/internal/kernel`、`app/runtime/internal/kernel/turn`、`app/runtime/internal/runtime` 内部调用点和测试。
- 已完成适配：所有 `RunTurnRequest` 调用点已迁移；未保留 type alias。
- 验证结果：`go test ./internal/kernel ./internal/kernel/turn ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；若后续暴露外部 API，应避免把方法名编码进可复用领域类型。

第六十三轮包含 `app/runtime/internal/kernel/lifecycle` 的 internal 破坏性重命名：

- 调整对象：`lifecycle.RunTurn`。
- 调整前问题：该类型实际是 protocol run id 与 turn handle 的绑定，但名称像执行动作，也容易与已删除的 `Engine.RunTurn` 概念混淆。
- 破坏性原因：目标已更新为不保留旧接口兼容层；该类型位于 `app/runtime/internal`，直接重命名能消除历史命名负担。
- 新设计：使用 `RunTurnBinding` 表达绑定语义，并由绑定对象生成内部 `turn.TurnHandle`。
- 架构收益：lifecycle 层的领域模型更准确，取消逻辑不再在底层函数内手动拼装 turn handle。
- 影响范围：`app/runtime/internal/kernel/lifecycle`、`app/runtime/internal/runtime`、`app/runtime/internal/delivery/server` 内部调用点和测试。
- 已完成适配：所有 `RunTurn` 类型引用已迁移；未保留 type alias。
- 验证结果：`go test ./internal/kernel/lifecycle ./internal/runtime ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；后续若还看到 `RunTurn` 命中，应优先判断是否仍在表达旧执行入口，不恢复旧类型名。

第六十四轮包含 `app/runtime/internal/kernel/lifecycle` 的 internal 破坏性方法重命名：

- 调整对象：`lifecycle.Coordinator.CancelRunTurn` 及 runtime/server 内部端口同名方法。
- 调整前问题：第六十三轮已将数据对象收敛为 `RunTurnBinding`，但取消用例仍叫 `CancelRunTurn`，容易继续把 run/turn 绑定误读为旧执行入口动作。
- 破坏性原因：目标已更新为不保留旧接口兼容层；该方法链全部位于 `app/runtime/internal`，直接重命名比保留旧方法更符合领域语言闭环。
- 新设计：使用 `CancelRunBinding` 表达“取消某个 run/turn 绑定所指向的 turn，并删除对应 durable interrupt record”。
- 架构收益：lifecycle coordinator、runtime facade、server port 的用例语言与领域对象一致，旧 `RunTurn` 动作名不再残留在 internal API 上。
- 影响范围：`app/runtime/internal/kernel/lifecycle`、`app/runtime/internal/runtime`、`app/runtime/internal/delivery/server` 内部调用点和测试 fixture。
- 已完成适配：所有 `CancelRunTurn` 调用点已迁移；未保留旧方法别名。
- 验证结果：`go test ./internal/kernel/lifecycle ./internal/runtime ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；后续若继续处理 `agent` context-aware companion API，需要先按公共 API 破坏性调整确认 scope。

第六十五轮包含 `app/runtime/internal/delivery/server` 的 internal 端口破坏性拆分：

- 调整对象：`mcpAccess` 与 `runtimeBindings.mcp`。
- 调整前问题：单个 `mcpAccess` 同时承载 live status、tool catalog、connection action、registry CRUD/probe，MCP handlers 通过一个胖字段访问所有能力，不符合 consumer-side ISP。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，直接拆分比保留聚合口更能减少 handler 对无关 MCP 能力的认知耦合。
- 新设计：按用例切成 `mcpStatusAccess`、`mcpToolCatalogAccess`、`mcpConnectionAccess`、`mcpRegistryAccess`，`RuntimePort` 只负责组合这些小端口。
- 架构收益：workspace MCP status/projection、tool listing、reconnect/authorize、config CRUD/probe 分别依赖最小端口，server runtime binding 更贴近实际用例边界。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、workspace MCP handlers/projection/request helpers。
- 已完成适配：所有 `s.mcp` 调用已迁移到更窄字段；未保留旧 `mcpAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；如后续继续拆其他 server 端口，应优先按 handler 实际消费语义拆，不按文件名机械拆。

第六十六轮包含 `app/runtime/internal/delivery/server` 的 internal provider 端口破坏性拆分：

- 调整对象：`providerAccess` 与 `runtimeBindings.providers`。
- 调整前问题：单个 `providerAccess` 同时承载 provider registry CRUD/probe、provider catalog metadata、default provider lookup，providers/models/usage handlers 通过同一个胖字段访问无关能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按消费语义拆分能消除 handler group 之间的无意义耦合，不需要为旧聚合字段保留兼容层。
- 新设计：使用 `providerRegistryAccess`、`providerCatalogAccess`、`providerDefaultAccess` 三个窄端口，`RuntimePort` 只组合这些能力。
- 架构收益：provider 配置、model role 校验、usage 默认归因各自依赖最小 provider 能力，server binding 的语义更贴近用例边界。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、providers/models/usage handlers 和相关测试 fixture。
- 已完成适配：所有 `s.providers` 调用已迁移到更窄字段；未保留旧 `providerAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；后续可继续按同一标准评估 schedule/session 等端口是否真的过宽。

第六十七轮包含 `app/runtime/internal/delivery/server` 的 internal session 端口破坏性拆分：

- 调整对象：`sessionAccess` 与 `runtimeBindings.sessions`。
- 调整前问题：单个 `sessionAccess` 同时承载 session list/read、create/update/delete 和 default model lookup，usage/session projection/workspace discovery/scheduler 都通过同一个胖字段访问无关能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按消费语义拆分可以消除默认模型归因、session 查询和 session 变更之间的无意义耦合。
- 新设计：使用 `sessionCatalogAccess`、`sessionMutationAccess`、`sessionDefaultModelAccess` 三个窄端口，`RuntimePort` 只组合这些能力。
- 架构收益：session read/export/rollback/resume/workspace discovery、session mutation/scheduler、usage/session wire default-model projection 各自依赖最小能力，server binding 更贴近用例边界。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、session/usage/scheduler/rollback/resume/workspace/sessionio handlers。
- 已完成适配：所有 `s.sessions` 调用已迁移到更窄字段；未保留旧 `sessionAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；后续若继续拆 transcript/schedule 端口，应以真实 handler 消费差异为依据。

第六十八轮包含 `app/runtime/internal/delivery/server` 的 internal schedule 端口破坏性拆分：

- 调整对象：`scheduleAccess` 与 `runtimeBindings.schedules`。
- 调整前问题：单个 `scheduleAccess` 同时承载 schedule list/read、create/update/delete、manual-run timestamp record 和 background worker startup，协议 CRUD 与进程入口共用一个胖字段。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按消费语义拆分可以让 protocol handlers 与 scheduler process entry point 依赖不同能力。
- 新设计：使用 `scheduleCatalogAccess`、`scheduleMutationAccess`、`scheduleRunRecorderAccess`、`scheduleWorkerAccess` 四个窄端口，`RuntimePort` 只组合这些能力。
- 架构收益：schedule CRUD、manual run 记录、background worker startup 各自依赖最小能力，server binding 对后台进程入口和协议 handler 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、schedules handler 和 scheduler entry。
- 已完成适配：所有 `s.schedules` 调用已迁移到更窄字段；未保留旧 `scheduleAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；后续如果拆 tool/knowledge/approval 端口，应继续要求有明确消费差异，不做纯形式拆分。

第六十九轮包含 `app/runtime/internal/delivery/server` 的 internal memory 端口破坏性拆分：

- 调整对象：`knowledgeAccess`、`runtimeBindings.knowledge` 与 `runtimeBindings.capabilities`。
- 调整前问题：`knowledgeAccess` 同时承载 memory feature availability 和 memory store 读写；`runtimeBindings.capabilities` 又把 memory/provider capability 聚回一个聚合字段。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 feature gate 与 store operation 拆分能避免 capability snapshot 和 memory CRUD 共享胖口。
- 新设计：使用 `memoryAvailabilityAccess` 表达 feature gate，使用 `memoryStoreAccess` 表达 list/get/update；`runtimeBindings.HasMemory` 直接读取 availability 端口，provider capability 继续读取 `providerCatalogAccess`。
- 架构收益：memory feature flag、memory handler gating、memory store operation 的依赖边界更清晰，同时删除了额外的 `runtimeBindings.capabilities` 聚合层。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、memory handlers 和 capability snapshot。
- 已完成适配：所有 `s.knowledge` 调用已迁移到更窄字段；未保留旧 `knowledgeAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险；memory handler 仍显式保留无 store 时 list 返回空页、get/update 返回 capability_not_negotiated 的既有行为。

第七十轮包含 `app/runtime/internal/delivery/server` 的 internal transcript 端口破坏性拆分：

- 调整对象：`transcriptAccess` 与 `runtimeBindings.transcript`。
- 调整前问题：单个端口同时承载完整 transcript item/run projection 和 run-only usage aggregation，usage 读取路径被迫依赖更宽的 transcript content surface。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，直接拆分可以避免为旧聚合字段留下 internal 兼容层。
- 新设计：使用 `transcriptContentAccess` 提供 `ListTranscript`，使用 `transcriptRunAccess` 提供 `ListTranscriptRuns`。
- 架构收益：session export/items/rollback/fork 与 usage aggregation 各自依赖最小能力，server binding 对 transcript content 和 run accounting 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、items/rollback/sessionio/sessions/usage handlers。
- 已完成适配：所有 `s.transcript` 调用已迁移到更窄字段；未保留旧 `transcriptAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十一轮包含 `app/runtime/internal/delivery/server` 的 internal settings 端口破坏性拆分：

- 调整对象：`toolAccess`、`approvalAccess`、`runtimeBindings.tools` 与 `runtimeBindings.approvals`。
- 调整前问题：`toolAccess` 同时承载工具目录读取和直接工具执行；`approvalAccess` 同时承载 runtime approval mode 与 persisted approval rule 管理，handler 通过聚合字段访问不同用例的能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按实际 use case 拆分能直接消除聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `toolCatalogAccess` / `toolInvocationAccess` 分离工具目录与直接调用；使用 `approvalModeAccess` / `approvalRuleAccess` 分离全局执行姿态和 remembered rule 管理。
- 架构收益：tools.list、tools.invoke、approval.get/setMode、approval.list/forgetRules 各自依赖最小能力，server binding 对读目录、执行动作、策略状态和规则存储的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、tools handler 和 approval handler。
- 已完成适配：所有 `s.tools` 与 `s.approvals` 调用已迁移到更窄字段；未保留旧 `toolAccess` / `approvalAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十二轮包含 `app/runtime/internal/delivery/server` 的 internal lifecycle 端口破坏性拆分：

- 调整对象：`lifecycleAccess` 与 `runtimeBindings.lifecycle`。
- 调整前问题：单个 `lifecycleAccess` 同时承载 run admission、working-tree admission、resume claim/continue、run cancel、rollback/fork/import write-set，run handlers 和 session handlers 通过同一个字段访问不同生命周期用例。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按用例边界拆分能删除旧聚合字段，避免 delivery handler 对无关 lifecycle 能力形成认知耦合。
- 新设计：使用 `runAdmissionAccess`、`sessionMutationAdmissionAccess`、`runResumeAccess`、`runCancellationAccess`、`sessionLifecycleMutationAccess` 分别表达 run admission、destructive mutation admission、resume continuation、cancel/abandon、session history lifecycle mutation。
- 架构收益：runs.start/resume、runs.cancel、sessions.rollback/delete、sessions.fork/import 各自依赖最小生命周期能力，server binding 对运行中 run 与 destructive session mutation 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、runs start/resume/control handlers、rollback/sessionio/sessions handlers 和相关测试调用点。
- 已完成适配：所有 `s.lifecycle` 调用已迁移到更窄字段；未保留旧 `lifecycleAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十三轮包含 `app/runtime/internal/delivery/server` 的 internal turn 端口破坏性拆分：

- 调整对象：`turnAccess` 与 `runtimeBindings.turn`。
- 调整前问题：单个 `turnAccess` 同时承载 turn planning/start、event subscription/cancel、steering、resume/rehydrate、process id lookup 和 interrupt-kind negotiation；其中 resume/rehydrate/process lookup 并不是 server handler 直接消费的能力，只是 lifecycle/runsegment facade 内部会用。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按直接消费语义拆分可以让 `RuntimePort` 不再暴露间接能力，删除旧聚合字段比保留兼容层更符合 consumer-side port 约束。
- 新设计：使用 `turnStartAccess`、`turnStreamAccess`、`turnSteeringAccess`、`turnInterruptPolicyAccess` 分别承载 runs.start、run pump、runs.steer 和 runtime.initialize interrupt negotiation；`ResumeTurn`、`RehydrateTurn`、`TurnProcessID` 不再属于 server `RuntimePort`。
- 架构收益：delivery/server 只依赖自己直接调用的 turn surface，resume/rehydrate/process lookup 留在 lifecycle/runsegment runtime facade 之后，避免 inbound adapter 对 kernel turn 细节的抽象泄露。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、runs.start、run pump、runs.steer 和 initialize handler。
- 已完成适配：所有 `s.turn` 调用已迁移到更窄字段；未保留旧 `turnAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十四轮包含 `app/runtime/internal/delivery/server` 的 internal workspace/codebase 端口破坏性拆分：

- 调整对象：`workspaceCatalogAccess`、`codebaseAccess`、`runtimeBindings.workspaceCatalog` 与 `runtimeBindings.codebase`。
- 调整前问题：`workspaceCatalogAccess` 同时承载 skill catalog 与 recipe catalog；`codebaseAccess` 同时承载 availability gate、semantic search、status query 和 manual reindex，workspace/codebase handlers 通过聚合字段访问不同用例能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，避免 delivery/server 对无关 workspace/codebase 能力形成认知耦合。
- 新设计：使用 `skillCatalogAccess` / `recipeCatalogAccess` 分离 workspace skills 与 recipes；使用 `codebaseAvailabilityAccess` / `codebaseSearchAccess` / `codebaseStatusAccess` / `codebaseReindexAccess` 分离 availability、search、status 和 reindex。
- 架构收益：workspace.skills、workspace.recipes、codebase.search/status/reindex 各自依赖最小能力；codebase status/reindex 不再被绑到 search 的 availability gate 聚合字段上。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、workspace discovery handler 和 codebase handler。
- 已完成适配：所有 `s.workspaceCatalog` 与 `s.codebase` 调用已迁移到更窄字段；未保留旧 `workspaceCatalogAccess` / `codebaseAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十五轮包含 `app/runtime/internal/delivery/server` 的 internal hook/model-role 端口破坏性拆分：

- 调整对象：`hookAccess`、`modelRoleAccess`、`runtimeBindings.hooks` 与 `runtimeBindings.modelRoles`。
- 调整前问题：`hookAccess` 同时承载 hook inspection read model 和 project trust mutation；`modelRoleAccess` 同时承载 utility role 与 embedding role 两条独立配置路径，handler 通过聚合字段访问不同用例能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `hookInspectionAccess` / `hookTrustAccess` 分离 hooks list 与 trust toggle；使用 `utilityRoleAccess` / `embeddingRoleAccess` 分离 maintenance utility model role 与 codebase embedding model role。
- 架构收益：workspace.hooks.list、workspace.hooks.setTrust、models.get/setUtilityRole、models.get/setEmbeddingRole 各自依赖最小能力，server binding 对 read view、state mutation 和两条 model-role 配置路径表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、hooks handler、models handler 和相关测试 fixture。
- 已完成适配：所有 `s.hooks` 与 `s.modelRoles` 调用已迁移到更窄字段；未保留旧 `hookAccess` / `modelRoleAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十六轮包含 `app/runtime/internal/delivery/server` 的 internal MCP registry 端口破坏性拆分：

- 调整对象：`mcpRegistryAccess` 与 `runtimeBindings.mcpRegistry`。
- 调整前问题：单个端口同时承载 registry catalog read、registry mutation 和 candidate probe，workspace MCP config handlers 通过聚合字段访问不同用例能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `mcpRegistryCatalogAccess` / `mcpRegistryMutationAccess` / `mcpRegistryProbeAccess` 分离 registry list/get、configure/remove/setEnabled、test probe。
- 架构收益：workspace.mcp.listConfigs/request authorization preservation、configure/remove/setEnabled、test 各自依赖最小能力，server binding 对 registry read/write/probe 边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、workspace MCP config handler 和 request helper。
- 已完成适配：所有 `s.mcpRegistry` 调用已迁移到更窄字段；未保留旧 `mcpRegistryAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十七轮包含 `app/runtime/internal/delivery/server` 的 internal provider registry 端口破坏性拆分：

- 调整对象：`providerRegistryAccess` 与 `runtimeBindings.providerRegistry`。
- 调整前问题：单个端口同时承载 provider registry read、credential mutation 和 provider probe；models role validation 只需要确认 provider 已配置，却被绑到 configure/probe 能力所在的聚合字段上。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `providerRegistryCatalogAccess` / `providerRegistryMutationAccess` / `providerRegistryProbeAccess` 分离 provider registry list/get、configure、probe。
- 架构收益：providers.list/configure/test 与 models.get/set role validation 各自依赖最小能力，server binding 对 provider registry read/write/probe 边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、providers handler、models handler 和相关测试 fixture。
- 已完成适配：所有 `s.providerRegistry` 调用已迁移到更窄字段；未保留旧 `providerRegistryAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十八轮包含 `app/runtime/internal/delivery/server` 的 internal session lifecycle 端口破坏性拆分：

- 调整对象：`sessionLifecycleMutationAccess` 与 `runtimeBindings.sessionLifecycle`。
- 调整前问题：单个端口同时承载 rollback、fork 和 import restore 三条 session lifecycle 写用例；rollback handler 会通过聚合字段获得 fork/restore 能力，import restore 也被绑到 rollback/fork 能力上。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `sessionRollbackAccess` / `sessionForkAccess` / `sessionRestoreAccess` 分离 rollback resolved write-set、fork creation、artifact restore。
- 架构收益：sessions.rollback、sessions.fork、sessions.import 各自依赖最小 lifecycle mutation 能力，server binding 对 destructive history rewrite、branch creation 和 artifact restore 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、rollback handler、sessions fork handler、session import handler 和 restore 测试。
- 已完成适配：所有 `s.sessionLifecycle` 调用已迁移到更窄字段；未保留旧 `sessionLifecycleMutationAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第七十九轮包含 `app/runtime/internal/delivery/server` 的 internal admission 端口破坏性拆分：

- 调整对象：`runAdmissionAccess`、`sessionMutationAdmissionAccess`、`runtimeBindings.runAdmissions` 与 `runtimeBindings.mutationAdmissions`。
- 调整前问题：run admission 将 session run slot 与 working-tree run lock 绑在一起；mutation admission 将 session mutation slot 与 working-tree mutation lock 绑在一起。sessions.import 只需要 session run slot，runs.resume 只需要 working-tree run lock，sessions.delete 只需要 session mutation slot，却都通过聚合字段看到无关能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 admission 资源类型拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `runSlotAdmissionAccess` / `workingTreeRunAdmissionAccess` / `sessionMutationSlotAccess` / `workingTreeMutationAccess` 分别表达 session run slot、working-tree run lock、session mutation slot、working-tree mutation lock。
- 架构收益：runs.start、runs.resume、sessions.import、sessions.delete、file-restore rollback 各自依赖自己实际需要的 admission 能力，server binding 对 session 单写入与 working-tree 写入互斥边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、runs.start、runs.resume、sessions.import、sessions.delete 和 rollback handler。
- 已完成适配：所有 `s.runAdmissions` 与 `s.mutationAdmissions` 调用已迁移到更窄字段；未保留旧 `runAdmissionAccess` / `sessionMutationAdmissionAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十轮包含 `app/runtime/internal/delivery/server` 的 internal session mutation 端口破坏性拆分：

- 调整对象：`sessionMutationAccess` 与 `runtimeBindings.sessionMutations`。
- 调整前问题：单个端口同时承载 session create、update 和 destructive delete cascade；scheduler 只需要创建 session 却被绑到 update/delete 能力，sessions.update 和 sessions.delete 也共享无关 mutation surface。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `sessionCreationAccess` / `sessionDeletionAccess` / `sessionUpdateAccess` 分离 create、delete cascade、patch update。
- 架构收益：schedule runner、sessions.create、sessions.delete、sessions.update 各自依赖最小 session mutation 能力，server binding 对普通会话变更和 destructive lifecycle deletion 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、scheduler 和 sessions handler。
- 已完成适配：所有 `s.sessionMutations` 调用已迁移到更窄字段；未保留旧 `sessionMutationAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十一轮包含 `app/runtime/internal/delivery/server` 的 internal memory store 端口破坏性拆分：

- 调整对象：`memoryStoreAccess` 与 `runtimeBindings.memoryStore`。
- 调整前问题：单个端口同时承载 memory entry 列表、单 scope 内容读取和内容写入；memory.list、memory.get、memory.update 三个 handler 通过聚合字段看到无关 store 能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `memoryListAccess` / `memoryReadAccess` / `memoryWriteAccess` 分离 memory entries listing、scope content read、scope content update。
- 架构收益：memory.list、memory.get、memory.update 各自依赖最小 memory store 能力，server binding 对 read model、content read 和 write mutation 的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding 和 memory handler。
- 已完成适配：所有 `s.memoryStore` 调用已迁移到更窄字段；未保留旧 `memoryStoreAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十二轮包含 `app/runtime/internal/delivery/server` 的 internal approval 端口破坏性拆分：

- 调整对象：`approvalModeAccess`、`approvalRuleAccess`、`runtimeBindings.approvalModes` 与 `runtimeBindings.approvalRules`。
- 调整前问题：approval mode 的查询/变更与 remembered approval rule 的列表/删除仍分别聚合在读写混合端口中；approval.getMode、approval.setMode、approval.listRules、approval.forgetRule 通过聚合字段看到无关能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按查询/命令用例拆分能直接删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `approvalModeReadAccess` / `approvalModeMutationAccess` 分离 runtime approval mode 的读取与设置；使用 `approvalRuleCatalogAccess` / `approvalRuleMutationAccess` 分离 persisted rule catalog 和 forget mutation。
- 架构收益：四个 approval handler 各自依赖最小能力，server binding 对执行姿态状态和 remembered rule 存储的读写边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding 和 approval handler。
- 已完成适配：所有 `s.approvalModes` 与 `s.approvalRules` 调用已迁移到更窄字段；未保留旧 `approvalModeAccess` / `approvalRuleAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十三轮包含 `app/runtime/internal/delivery/server` 的 internal MCP connection 命令端口破坏性拆分：

- 调整对象：`mcpConnectionAccess` 与 `runtimeBindings.mcpConnections`。
- 调整前问题：workspace.mcp.reconnect 的普通重拨命令和 workspace.mcp.authorize 的 OAuth 人工授权命令共享同一个 runtime capability 字段；两个 handler 虽然共用状态事件发布 helper，但不应因此互相看到对方命令能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按命令语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `mcpReconnectAccess` 承载 `ReconnectMCPServer`，使用 `mcpAuthorizationAccess` 承载 `AuthorizeMCPServer`；`runMCPConnectionAction` 继续作为 delivery 层状态事件模板方法复用。
- 架构收益：reconnect 和 authorize 两条 workspace MCP command path 各自依赖最小 runtime 能力，避免“共享事件模板”被误建模成“共享业务端口”。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding 和 workspace MCP command handlers。
- 已完成适配：所有 `s.mcpConnections` 调用已迁移到更窄字段；未保留旧 `mcpConnectionAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十四轮包含 `app/runtime/internal/delivery/server` 的 internal schedule 用例端口破坏性拆分：

- 调整对象：`scheduleCatalogAccess`、`scheduleMutationAccess`、`runtimeBindings.scheduleCatalog` 与 `runtimeBindings.scheduleMutations`。
- 调整前问题：schedule list/detail lookup 聚合在同一个 catalog 端口；create/update/delete 聚合在同一个 mutation 端口。schedules.create 会通过聚合字段看到 update/delete，delete 会看到 create/update，runNow 也被绑到 list capability。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 schedule 用例拆分能直接删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `scheduleListAccess` / `scheduleReadAccess` 分离列表 read model 和单条 lookup；使用 `scheduleCreationAccess` / `scheduleUpdateAccess` / `scheduleDeletionAccess` 分离创建、全量替换和删除命令；`scheduleRunRecorderAccess` 继续单独承载 runNow 的 off-cycle firing 记录。
- 架构收益：schedules.list、create、update、delete、runNow 各自依赖最小 runtime 能力，避免 CRUD 聚合端口掩盖 create/update/delete 的不同业务语义。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding 和 schedules handler。
- 已完成适配：所有 `s.scheduleCatalog` 与 `s.scheduleMutations` 调用已迁移到更窄字段；未保留旧 `scheduleCatalogAccess` / `scheduleMutationAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十五轮包含 `app/runtime/internal/delivery/server` 的 internal provider registry/catalog 端口破坏性拆分：

- 调整对象：`providerRegistryCatalogAccess`、`providerCatalogAccess`、`runtimeBindings.providerRegistryCatalog` 与 `runtimeBindings.providerCatalog`。
- 调整前问题：provider registry list 和 single-entry lookup 聚合在同一个端口；静态 supported-provider list 和 per-provider metadata lookup 也聚合在同一个端口。providers.list 需要列表 read model，providers.configure/test 与 model-role validation 需要单条 lookup/metadata，却通过同一字段互相暴露能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 read model 与 validation lookup 拆分能直接删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `providerRegistryListAccess` / `providerRegistryReadAccess` 分离 runtime registry list 与 single provider lookup；使用 `providerSupportCatalogAccess` / `providerMetadataAccess` 分离 supported-provider list 和 per-provider metadata lookup。
- 架构收益：providers.list、providers.configure、providers.test、models.setUtilityRole、models.setEmbeddingRole 各自依赖最小 provider registry/catalog 能力，避免列表展示能力与配置/校验能力串联。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、providers handler、models role validation 和相关测试 fixture。
- 已完成适配：所有 `s.providerRegistryCatalog` 与 `s.providerCatalog` 调用已迁移到更窄字段；未保留旧 `providerRegistryCatalogAccess` / `providerCatalogAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十六轮包含 `app/runtime/internal/delivery/server` 的 internal MCP registry 用例端口破坏性拆分：

- 调整对象：`mcpRegistryCatalogAccess`、`mcpRegistryMutationAccess`、`runtimeBindings.mcpRegistryCatalog` 与 `runtimeBindings.mcpRegistryMutations`。
- 调整前问题：workspace.mcp.listConfigs 的配置列表 read model 和 configure/test 的单条读取聚合在同一个 catalog 端口；configure/remove/setEnabled 三条不同命令聚合在同一个 mutation 端口。共享设置页领域不代表 handler 应互相看到无关命令能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 registry read model、single lookup 和具体 command 拆分能直接删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `mcpRegistryListAccess` / `mcpRegistryReadAccess` 分离配置列表和单条读取；使用 `mcpRegistryConfigureAccess` / `mcpRegistryRemoveAccess` / `mcpRegistryEnableAccess` 分离 upsert、delete 和 enablement toggle；`mcpRegistryProbeAccess` 继续单独承载 test probe。
- 架构收益：workspace.mcp.listConfigs、configure、remove、setEnabled、test-token-preservation 各自依赖最小 registry 能力，避免设置页 CRUD 聚合端口掩盖命令语义差异。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、workspace MCP config handlers 和 request helper。
- 已完成适配：所有 `s.mcpRegistryCatalog` 与 `s.mcpRegistryMutations` 调用已迁移到更窄字段；未保留旧 `mcpRegistryCatalogAccess` / `mcpRegistryMutationAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十七轮包含 `agent` context-first 公开 API 破坏性收敛：

- 调整对象：`agent/event.Listener`、`agent/event.ListenerFunc`、`agent/event.NamedListener`、`agent/event.Multicast.OnEvent`、`agent/runtime.EventListener`、`agent/core.EventPublisher`、`agent/core.PlatformHooks.Publish`、`agent/core.Process` 的 await / invocation 记录方法、`agent/core.ProcessContext` 的 publish / await / invocation 记录方法、`agent/runtime.AgentProcess` 对应实现，以及 `agent/hitl.HandleInterrupt` / `app/runtime/internal/kernel.HandleInterrupt`。
- 调整前问题：第五十六至五十九轮为兼容保留了无 ctx 入口与 `*Context` companion 入口两套公开语义；`ProcessContext` 还需要隐式保存 action ctx 来让旧调用继承 trace。公开 API 同时表达“调用方显式传 ctx”和“框架替调用方找 ctx”，容易误用，也让 runtime/observability contract 变宽。
- 破坏性原因：用户已确认方案 A；继续保留旧入口会形成长期兼容 shim，违背本任务“不为旧接口留兼容债”的目标。ctx 是 blocking / observable operation 的调用上下文，应该在公开签名中显式出现，而不是藏在 `ProcessContext` 内部状态或 companion fallback。
- 新设计：保留一套 context-first 入口：listener 使用 `OnEvent(ctx, event)`；`EventPublisher` 使用 `func(ctx, event)`；`ProcessContext.Publish/AwaitInput/RecordUsage/RecordLLMInvocation/RecordEmbeddingInvocation` 与 `core.Process` / `AgentProcess` 对应方法均以 `context.Context` 为第一参数；删除 `PublishContext`、`AwaitInputContext`、`Record*Context`、`OnEventContext` 以及无 ctx wrapper。
- 架构收益：公开 API 的 ctx 传递方向单一、可见且难误用；`ProcessContext` 不再承担“缓存当前 action ctx”的隐式状态职责；agent runtime 的 event / await / invocation observability 都由调用点明确传递上下文，降低框架内部隐式耦合。
- 影响范围：`agent/core`、`agent/event`、`agent/runtime`、`agent/hitl` 的公开签名；`app/runtime/internal/kernel` 的 turn loop / HITL parking 调用；agent runtime tests、examples 与 agent docs。
- 已完成适配：目标仓库内所有旧 `OnEventContext`、`PublishContext`、`AwaitInputContext`、`Record*Context`、无 ctx `pc.Publish` / `pc.AwaitInput` / `pc.Record*` 调用均已迁移；agent docs 和 examples 已同步更新。
- 验证结果：`go test ./...`（`core` / `agent` / `app/runtime`）、`go vet ./...`（`core` / `agent` / `app/runtime`）、`go build ./...`（`core` / `agent` / `app/runtime`）均通过。
- 后续风险：存在面向仓库外调用方的编译期迁移成本；迁移方式明确为在 action / request / listener 调用点传入已有 `ctx`。仓库内未保留兼容 alias 或 wrapper。

第八十八轮包含 `app/runtime/internal/delivery/server` 的 internal session catalog 端口破坏性拆分：

- 调整对象：`sessionCatalogAccess` 与 `runtimeBindings.sessionCatalog`。
- 调整前问题：sessions.list / usage.summary / workspace.projects 需要的是 session list read model；sessions.get、export/import、rollback、resume、workspace stream 需要的是 single-session lookup。旧 `sessionCatalogAccess` 将两个 read concern 聚合在同一个字段，列表消费者被迫依赖单会话 lookup，lookup 消费者也被迫依赖列表能力。
- 破坏性原因：该端口位于 `app/runtime/internal/delivery/server`，按 handler 实际消费语义拆分能直接删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `sessionListAccess` 承载 `ListSessions`，使用 `sessionReadAccess` 承载 `SessionByID`；`runtimeBindings` 分别持有 `sessionList` 与 `sessionRead`。
- 架构收益：session 列表 read model 与单会话 lookup 的依赖边界清晰分离，delivery handler 不再通过同一个 catalog 字段互相可见无关能力。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、sessions、usage、workspace discovery、session import/export、rollback、resume 和 workspace stream handler。
- 已完成适配：所有 `s.sessionCatalog` 调用已迁移到 `s.sessionList` 或 `s.sessionRead`；未保留旧 `sessionCatalogAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第八十九轮包含 `app/runtime/internal/delivery/server` 的 internal model role 读写端口破坏性拆分：

- 调整对象：`utilityRoleAccess`、`embeddingRoleAccess`、`runtimeBindings.utilityRole` 与 `runtimeBindings.embeddingRole`。
- 调整前问题：models.getUtilityRole / models.getEmbeddingRole 只需要读取当前 role；models.setUtilityRole / models.setEmbeddingRole 需要写入 role，并在写入后读回 stored role。旧端口把 read 和 mutation 聚合在同一字段中，纯读取 handler 也依赖写能力。
- 破坏性原因：这些端口位于 `app/runtime/internal/delivery/server`，按 read/mutation 语义拆分能删除旧聚合字段，不需要为旧 internal port 留兼容层。
- 新设计：使用 `utilityRoleReadAccess` / `utilityRoleMutationAccess` 分离 utility model role 读取与写入；使用 `embeddingRoleReadAccess` / `embeddingRoleMutationAccess` 分离 embedding model role 读取与写入。
- 架构收益：models.get*Role 与 models.set*Role 各自依赖自己真实需要的 role 能力；server binding 对配置读取和配置变更的边界表达更清楚。
- 影响范围：`app/runtime/internal/delivery/server` 的 runtime port、runtime binding、models handler 和 model role handler tests。
- 已完成适配：所有 `s.utilityRole` 与 `s.embeddingRole` 调用已迁移到 read / mutation 字段；未保留旧 `utilityRoleAccess` / `embeddingRoleAccess` 聚合字段。
- 验证结果：`go test ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。
- 后续风险：无跨模块公开 API 风险。

第九十轮包含 `app/runtime/internal/kernel` 对 agent runtime 依赖口的内部破坏性收窄：

- 调整对象：`agentRuntime`、`processRunner` 与 `Engine.platform`。
- 调整前问题：`Engine` 持有一个聚合后的 `agentRuntime`，该端口同时包含 agent deploy、turn start、process restore/continue 和 turn lifecycle control。构造期才需要的 `Deploy` 也被保留在 Engine 的运行期字段边界内，导致 turn 启动、恢复和句柄控制共享同一“平台总口”。
- 破坏性原因：这些接口位于 `app/runtime/internal/kernel`，属于应用层内部端口；删除旧聚合口并按消费语义重建字段，可以避免继续为内部旧 shape 留兼容债。
- 新设计：删除 `agentRuntime` / `processRunner` 聚合口；使用 `processStarter` 表达 fresh turn start，使用 `processRestorer` 表达 restore + re-tick，使用 `processControl` 表达 `TurnProcess` 的 cancel/resume/discard 生命周期控制。`Deploy` 保持在 `New` 构造流程里直接调用 concrete platform，不再作为运行期 Engine 端口。
- 架构收益：`Engine.StartTurn`、`Engine.RestoreTurn` 与 `turnProcess` 各自依赖自己实际消费的 agent runtime 能力；构造期部署能力不再泄漏到运行期 Engine state；kernel 与 `agent/runtime.Platform` 的耦合从“平台总口”收窄为用例口。
- 影响范围：`app/runtime/internal/kernel/agent_runtime.go`、`engine.go`、`turnrun.go`。
- 已完成适配：`Engine` 字段迁移为 `turnStarter`、`turnRestorer`、`turnControl`；`StartTurn`、`RestoreTurn` 与 `turnProcess` 创建点已迁移到对应窄口；未保留旧 `agentRuntime` / `processRunner`。
- 验证结果：`go test ./internal/kernel` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十一轮包含 `app/runtime/internal/kernel/lifecycle` 与 `runsegment` 的 session 持久化依赖口收窄：

- 调整对象：`lifecycle.Stores.Session()`、`runsegment.Stores.Session()`、`app/runtime/internal/runtime` 的 store adapter，以及 delivery/server session 测试 fixture。
- 调整前问题：`lifecycle.Coordinator` 与 `runsegment.Effects` 都通过 `Session() session.Store` 拿到完整 domain session store；但 lifecycle 只需要 fork/rename/restore/delete/children，runsegment 只需要 get/rename-if-untitled。完整 Store 穿透到用例层导致 coordinator/effects 理论上可访问无关 session CRUD，测试 fake 也被迫实现一串 unused 方法。
- 破坏性原因：这些接口位于 `app/runtime/internal/kernel` 与测试 fixture，属于内部用例端口；按 consumer-side port 收窄能删除旧宽接口，不需要为旧 internal shape 留兼容层。
- 新设计：`lifecycle` 定义自己的 `SessionStore`（`Fork` / `Rename` / `Restore` / `Children` / `Delete`）；`runsegment` 定义自己的 `SessionStore`（`Get` / `RenameIfUntitled`）。runtime 侧拆分为 `lifecycleStores` 与 `runSegmentStores` 两个 adapter；server 测试 fixture 同样拆为 `stubLifecycleStores` 与 `stubRunSegmentStores`。
- 架构收益：完整 `domain/session.Store` 留在 runtime 组合层，kernel 用例只看见自己真实消费的 session 能力；lifecycle 和 runsegment 不再共享一个万能 store adapter；runsegment 测试 fake 删除了全部 unused session methods，测试替身更能反映真实依赖。
- 影响范围：`app/runtime/internal/kernel/lifecycle`、`app/runtime/internal/kernel/runsegment`、`app/runtime/internal/runtime`、`app/runtime/internal/delivery/server` 测试 fixture。
- 已完成适配：production runtime 和 delivery/server fixture 均迁移到用例专属 store adapter；旧 `runtimeStores` 不再作为 lifecycle/runsegment 共享 adapter；runsegment fake session 只保留 `Get` 与 `RenameIfUntitled`。
- 验证结果：`go test ./internal/kernel/lifecycle ./internal/kernel/runsegment ./internal/runtime ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十二轮包含 `app/runtime/internal/kernel/lifecycle` 与 `runsegment` 的 transcript / interrupt 持久化依赖口收窄：

- 调整对象：`lifecycle.Stores.Transcript()`、`lifecycle.Stores.Interrupts()`、`runsegment.Stores.Transcript()`、`runsegment.Stores.Interrupts()`、runtime adapter、delivery/server session fixture、runsegment 测试 fake。
- 调整前问题：上一轮收窄 session 之后，lifecycle/runsegment 仍通过完整 `transcript.Store` 与 `interrupts.Store` 访问持久化层。runsegment 只追加 item / upsert run / put interrupt，却理论上可读 transcript、删 run、claim/delete interrupt；测试 fake 也被迫实现 `List` / `ListRuns` / `DeleteRun` / `DeleteSession` / `Get` / `Consume` / `Delete` 等 unused 方法。
- 破坏性原因：这些接口位于 `app/runtime/internal/kernel` 与测试 fixture，属于内部用例端口；按写集/claim 语义收窄能删除旧宽接口，不需要为旧 internal shape 留兼容层。
- 新设计：`lifecycle` 定义 `TranscriptStore`（`AppendItem` / `PutRun` / `DeleteRun` / `DeleteSession`）与 `InterruptStore`（`List` / `Get` / `Consume` / `Delete`）；`runsegment` 定义 `TranscriptStore`（`AppendItem` / `PutRun`）与 `InterruptStore`（`Put`）。runtime adapter 和 server fixture 返回对应用例口。
- 架构收益：runsegment 只拥有“记录直播流副作用”的写能力，不再看见 rollback/import/resume 的读删能力；lifecycle 只拥有 claim/delete 与 transcript write-set 能力，不再看见 interrupt creation 或 transcript read projections；fake store 删除 unused 方法后测试替身更贴近真实依赖。
- 影响范围：`app/runtime/internal/kernel/lifecycle`、`app/runtime/internal/kernel/runsegment`、`app/runtime/internal/runtime`、`app/runtime/internal/delivery/server` 测试 fixture。
- 已完成适配：production runtime 与 delivery/server fixture 均迁移到 transcript/interrupt 用例专属 store adapter；runsegment fake transcript/interrupt 只保留实际调用的方法。
- 验证结果：`go test ./internal/kernel/lifecycle ./internal/kernel/runsegment ./internal/runtime ./internal/delivery/server` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十三轮包含 `app/runtime/internal/kernel/turn` 对 engine 用例依赖口的内部破坏性拆分：

- 调整对象：`turn.Dependencies.Engine`、`engineDep`、`inMemory.engine`。
- 调整前问题：turn dispatcher 的 fresh start、cross-restart restore、steering flush、post-turn maintenance 四条路径共享一个 `engineDep` 聚合口。任一子流程都能看见完整 engine surface，`Dependencies.Engine` 也把构造意图表达成“注入整台 engine”，而不是注入 turn dispatcher 真正消费的用例能力。
- 破坏性原因：该接口位于 `app/runtime/internal/kernel/turn`，属于内部应用层用例端口；删除旧 `Engine` 聚合字段可以避免继续把宽口作为默认注入方式。
- 新设计：删除 `engineDep` 与 `Dependencies.Engine`；新增 `turnStarter`、`turnRestorer`、`steeringSink`、`maintenanceRunner` 四个 consumer-side port。`Dependencies` 分别要求 `Starter`、`Restorer`、`Steering`、`Maintenance`；`inMemory` 按路径持有对应字段。
- 架构收益：fresh turn、rehydrate、steering flush、maintenance 的依赖边界在类型上分开；runtime 装配仍可把同一个 `*kernel.Engine` 作为实现注入，但 turn dispatcher 不再把它建模成一个单体 engine dependency；测试 helper 也明确表达 stub 同时满足四个端口。
- 影响范围：`app/runtime/internal/kernel/turn`、`app/runtime/internal/runtime/runtime_assembly.go`、turn package tests。
- 已完成适配：runtime 装配迁移到四个字段；turn dispatcher 调用点分别使用 `starter`、`restorer`、`steering`、`maintenance`；turn tests 使用 `turnDeps` helper 组装 stub ports；未保留旧 `Engine` 字段或 `engineDep`。
- 验证结果：`go test ./internal/kernel/turn ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十四轮包含 `app/runtime/internal/kernel` live MCP 控制端口的内部破坏性拆分：

- 调整对象：`toolport.MCPControl`、`kernel.Config.MCP`、`Engine.mcp` 与 `toolset.Built.MCP`。
- 调整前问题：workspace.mcp 的 server status 读取、tool catalog 查询、reconnect/authorize 连接命令、probe/configure/remove registry 命令共享一个 `MCPControl` 聚合口。纯读取路径可以看见写命令，连接 lifecycle 命令也和 registry apply/probe 命令绑在一起，`Engine` 对 live MCP 的依赖边界被建模成一个万能 facade。
- 破坏性原因：这些端口位于 `app/runtime/internal/kernel` 与 `adapter/toolset` 的内部装配边界；继续保留旧 `MCPControl` 会让后续 workspace.mcp 用例默认依赖宽口，不符合 consumer-side port 和 ISP 的目标。
- 新设计：删除 `MCPControl`；新增 `MCPStatusReader`、`MCPToolCatalog`、`MCPConnectionCommands`、`MCPRegistryCommands` 四个窄端口。`toolset.Built` 和 `kernel.Config` 分别暴露四个字段，`Engine` 按具体 workspace.mcp 方法调用对应端口；`mcpControl` 作为 adapter 实现四个端口并用编译期断言固定契约。
- 架构收益：workspace.mcp read model、tool catalog、live connection command、registry command 的依赖边界在类型上分开；kernel 不再持有 live MCP 万能控制面；装配层仍可复用同一个 infra adapter，但不会把 adapter 的完整能力泄漏给所有消费者。
- 影响范围：`app/runtime/internal/kernel/toolport`、`kernel.Config`、`Engine` 的 MCP 方法、`adapter/toolset.Built`、runtime 组合根和相关测试 fixture。
- 已完成适配：所有 `built.MCP` / `cfg.MCP` / `Engine.mcp` 调用已迁移到四个窄端口；未保留旧 `MCPControl` alias 或兼容字段。
- 验证结果：`go test ./internal/kernel ./internal/kernel/turn ./internal/adapter/toolset ./internal/runtime ./internal/domain/tool` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十五轮包含 `app/runtime/internal/runtime` provider credential 读口收窄与必填依赖 fail-fast：

- 调整对象：`clientResolver.providers`、`embeddingResolver.providers`、`buildEmbeddingEnvironment` / `newEmbeddingResolver` 参数，以及 `Runtime.New` 的 required dependency 检查。
- 调整前问题：chat client resolver 与 embedding resolver 只需要按 provider id 读取凭据，却直接依赖完整 `provider.Registry`，从模型解析路径理论上可见 provider list/configure 能力；同时 `Config` 注释标记 `ProviderRegistry`、`MCPRegistry`、`SessionStore`、`InterruptStore`、`TranscriptStore` 为 required，但构造期只显式检查了 `TranscriptStore`，其他 nil 会推迟到后续流程里以更模糊的方式失败。
- 破坏性原因：这些类型位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；删除 resolver 对完整 registry 的依赖并补齐 required 检查，不需要为内部旧构造 shape 留兼容债。
- 新设计：新增 consumer-side `providerCredentialLookup`（仅 `Get`）；`clientResolver` 与 `embeddingResolver` 只依赖该读口，完整 `provider.Registry` 只留在 runtime bundle 和 provider 配置用例面。`Runtime.New` 在组装任何子系统前 fail-fast 检查 required dependencies，并用测试锁住错误语义。
- 架构收益：模型 client 构建路径只拥有 provider credential read capability，不再耦合 provider CRUD；构造契约从注释变成可验证行为，减少 nil 依赖穿透到更深层装配流程的隐式失败。
- 影响范围：`app/runtime/internal/runtime` 的 resolver、embedding environment、runtime assembly 和 focused constructor tests。
- 已完成适配：runtime assembly 继续把同一个 provider registry 作为 `providerCredentialLookup` 注入，但 resolver 类型边界已收窄；新增 required dependency table test；同步修正 `Config.Engine` 注释里的 live-MCP 字段描述。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险；内部测试若构造 `Runtime.New` 时漏传 required dependency 会收到更早、更明确的错误。

第九十六轮包含 `app/runtime/internal/runtime` MCP registry 用例端口收窄：

- 调整对象：`Runtime.mcpRegistry`、`buildMCPEnvironment`、`enabledConfigs`、`buildMCPGating` 与 `refreshMCPGating`。
- 调整前问题：runtime bundle 持有一个完整 `mcpserver.Registry` 字段，workspace.mcp 的 list/read/configure/remove/setEnabled 以及 boot/gating 投影都通过同一个字段访问，导致只读路径和 gating projection 理论上可见 configure/remove/enable 等写能力。
- 破坏性原因：这些字段和 helper 位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 MCP registry 用例拆分字段能直接删除旧总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 `mcpServerList`、`mcpServerRead`、`mcpServerConfigure`、`mcpServerRemove`、`mcpServerEnable` 五个 consumer-side port；`Runtime` 分别持有对应字段，boot/gating/refresh 只依赖 `mcpServerList`，workspace.mcp 方法按用例调用对应端口。
- 架构收益：MCP registry 的 read model、single lookup、upsert、remove、enable toggle 在 runtime 内部类型边界上分开；gating 和 enabled config projection 不再耦合 registry mutation 能力；完整 registry 只在 composition root 注入时出现。
- 影响范围：`app/runtime/internal/runtime` 的 MCP runtime facade、MCP gating/config helpers、Runtime struct 和 runtime assembly。
- 已完成适配：runtime assembly 继续把同一个 `cfg.MCPRegistry` 注入到五个窄口字段；所有 `r.mcpRegistry.*` 调用已迁移到对应端口；未保留旧 `Runtime.mcpRegistry` 字段。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十七轮包含 `app/runtime/internal/runtime` provider registry 用例端口收窄：

- 调整对象：`Runtime.providers` 与 provider runtime facade 的 list/read/configure 调用。
- 调整前问题：runtime bundle 持有一个完整 `provider.Registry` 字段，providers.list、providers.configure 读回、models role validation、provider credential resolver 等不同路径容易共享同一个 registry 总口。第九十五轮已把模型 client resolver 收窄到 credential lookup，但 `Runtime` 自身仍通过一个字段暴露 list/get/configure 全部能力。
- 破坏性原因：这些字段和 helper 位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 provider registry 用例拆分字段能直接删除旧总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 `providerRegistryList`、`providerRegistryRead`、`providerRegistryConfigure` 三个 consumer-side port；`Runtime` 分别持有对应字段，`ListRegisteredProviders`、`RegisteredProvider`、`ConfigureProvider` 只调用各自端口。模型 client 构建继续使用独立的 `providerCredentialLookup`。
- 架构收益：provider registry 的列表 read model、单条 lookup、credential configure 在 runtime 内部类型边界上分开；完整 registry 只在 composition root 注入时出现，provider CRUD 不再作为默认 runtime state 字段穿透。
- 影响范围：`app/runtime/internal/runtime` 的 provider runtime facade、Runtime struct 和 runtime assembly。
- 已完成适配：runtime assembly 继续把同一个 `cfg.ProviderRegistry` 注入到三个窄口字段；所有 `r.providers.*` runtime facade 调用已迁移到对应端口；未保留旧 `Runtime.providers` 字段。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十八轮包含 `app/runtime/internal/runtime` schedule registry 用例端口收窄与 worker 依赖收窄：

- 调整对象：`Runtime.schedules`、schedule runtime facade 的 list/read/create/update/delete/runNow/worker 调用，以及 `schedule.NewWorker` 的 registry 参数。
- 调整前问题：runtime bundle 持有一个完整 `schedule.Registry` 字段，schedules.list/get/create/update/delete/runNow 和 scheduler worker 都通过同一个总口访问；更深一层，schedule worker 自身只需要 due query 与 guarded cursor advance，却要求完整 Registry，理论上可见管理 CRUD 和 manual run recording。
- 破坏性原因：这些类型位于 `app/runtime/internal/runtime` 与 internal domain `schedule`，属于应用层内部装配/用例边界；按 consumer-side port 收窄能删除旧总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `scheduleList`、`scheduleRead`、`scheduleCreate`、`scheduleUpdate`、`scheduleDelete`、`scheduleRunRecorder` 六个端口，并让 `Runtime` 分别持有对应字段；新增 domain `schedule.WorkerStore`（`Due` / `MarkFired`），`schedule.NewWorker` 只接受 worker 所需能力。
- 架构收益：schedule management read/write、manual run-now 记录、scheduler worker cursor advance 在类型边界上分开；worker 不再依赖完整 Registry，Runtime 也不再把 schedule CRUD 总口作为默认 state 字段穿透。
- 影响范围：`app/runtime/internal/domain/schedule` 的 worker 构造契约，`app/runtime/internal/runtime` 的 schedule facade、Runtime struct 和 runtime assembly。
- 已完成适配：runtime assembly 继续把同一个 `cfg.ScheduleRegistry` 注入到各窄口字段；所有 `r.schedules.*` runtime facade 调用已迁移到对应端口；`schedule.NewWorker` 调用迁移到 `scheduleWorker` 端口；未保留旧 `Runtime.schedules` 字段。
- 验证结果：`go test ./internal/domain/schedule ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第九十九轮包含 `app/runtime/internal/runtime` session store 用例端口收窄与子会话 adapter 依赖收窄：

- 调整对象：`Runtime.session`、session runtime facade 的 list/read/create/update/start-turn/approval lookup 调用、lifecycle/runsegment store adapter，以及 `childSessionStore` 的构造契约。
- 调整前问题：runtime bundle 持有一个完整 `session.Store` 字段，用户 session CRUD、turn 启动绑定、显式模型写入、approval rule cwd lookup、lifecycle 写集、runsegment 终端维护和 agent 子会话适配都通过同一个总口访问；纯读取和单字段写入路径理论上可见 fork/restore/delete/subtask 等无关能力，测试 fake 也通过嵌入完整 store 掩盖真实依赖。
- 破坏性原因：这些字段和 adapter 位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 consumer-side port 收窄能删除旧总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `sessionList`、`sessionRead`、`sessionCreate`、`sessionPatchWriter`、`sessionModelWriter` 端口，并让 lifecycle/runsegment 分别持有它们自己的 `lifecycle.SessionStore` / `runsegment.SessionStore` 字段；`childSessionStore` 改为依赖只含 `List` / `Get` / `CreateSubtask` / `Delete` 的子会话持久化端口。
- 架构收益：用户 session read/write、turn model persistence、approval cwd lookup、跨域 lifecycle 写集、runsegment maintenance 和 agent 子会话 SPI 在类型边界上分开；完整 `domain/session.Store` 只在 composition root 注入时出现，Runtime 不再把 session 万能 store 作为默认 state 字段穿透。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、session/turn/approval facade、runtime assembly、lifecycle/runsegment adapters、child session adapter 和 focused runtime tests。
- 已完成适配：runtime assembly 继续把同一个 `cfg.SessionStore` 注入到各窄口字段；所有 `r.session.*` 调用已迁移到对应端口；测试 fixture 删除了嵌入完整 `session.Store` 的宽 fake；未保留旧 `Runtime.session` 字段。
- 验证结果：`go test ./internal/runtime` 与 `go test ./internal/runtime ./internal/kernel/lifecycle ./internal/kernel/runsegment` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百轮包含 `app/runtime/internal/runtime` transcript / interrupt store 用例端口收窄：

- 调整对象：`Runtime.interrupts`、`Runtime.transcript`、pending interrupt / transcript runtime facade、lifecycle/runsegment store adapter 和 focused runtime tests。
- 调整前问题：kernel 的 lifecycle/runsegment 已在第九十二轮拆出了各自的 transcript/interrupt consumer-side port，但 Runtime 组合层仍持有完整 `interrupts.Store` 与 `transcript.Store` 字段，再分发给 pending interrupt list、items/runs read projection、lifecycle 写集和 runsegment 写集；读投影路径理论上可见 put/consume/delete 等写能力，runsegment 写入路径理论上可见读取和删除投影能力。
- 破坏性原因：这些字段和 adapter 位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 read projection 与写集用例拆分能删除旧总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `interruptList`、`transcriptContent`、`transcriptRuns` 读投影端口，并让 lifecycle/runsegment 分别持有自己的 `lifecycle.InterruptStore` / `lifecycle.TranscriptStore` 与 `runsegment.InterruptStore` / `runsegment.TranscriptStore` 字段；`ListTranscript` / `ListTranscriptRuns` 不再保留 nil-store 空结果分支，依赖 `Runtime.New` 的 required store 契约。
- 架构收益：pending interrupt listing、items.list 内容投影、runs-only 投影、lifecycle resume/delete/rollback 写集、runsegment stream side effects 在类型边界上分开；完整 domain stores 只在 composition root 注入时出现，Runtime 不再把 transcript/interrupt 万能 store 作为默认 state 字段穿透。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、interrupt/transcript facade、runtime assembly、lifecycle/runsegment adapters 和 focused runtime tests。
- 已完成适配：runtime assembly 继续把同一个 `cfg.InterruptStore` / `cfg.TranscriptStore` 注入到对应窄口字段；所有 `r.interrupts.*` 与 `r.transcript.*` 调用已迁移到对应端口；测试 fixture 删除了嵌入完整 store 的宽 fake；未保留旧 `Runtime.interrupts` / `Runtime.transcript` 字段。
- 验证结果：`go test ./internal/runtime ./internal/kernel/lifecycle ./internal/kernel/runsegment` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零一轮包含 `app/runtime/internal/runtime` registered-tool catalog / invocation 端口收窄：

- 调整对象：`Runtime.tools`、registered-tool list/invoke runtime facade、runtime assembly 和 focused runtime tests。
- 调整前问题：`domain/tool` 已经提供 `Catalog` / `Invoker` 窄接口，但 Runtime 仍持有完整 `tool.Registry` 字段；`ListRegisteredTools` 理论上可见 direct invoke 能力，`InvokeRegisteredTool` 也通过同一字段可见 catalog list 能力，组合层没有延续 domain 已经设计好的读/命令分离。
- 破坏性原因：这些字段位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；直接删除旧总口字段并使用已有窄接口，不需要为内部旧 shape 留兼容层。
- 新设计：`Runtime` 改为分别持有 `toolsvc.Catalog` 与 `toolsvc.Invoker`；runtime assembly 继续把同一个 engine-backed registry 注入到两个字段；list 和 direct invoke facade 分别调用自己的端口。
- 架构收益：registered-tool catalog projection 与 out-of-turn diagnostic invocation 在 Runtime 类型边界上分开；组合层复用 domain/tool 既有接口设计，避免把完整 registry 作为默认 runtime state 字段穿透。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、tool facade、runtime assembly 和 focused tool facade tests。
- 已完成适配：所有 `r.tools.*` 调用已迁移到 `r.toolCatalog` 或 `r.toolInvocations`；新增 focused tests 锁住两个 facade 只需各自端口；未保留旧 `Runtime.tools` 字段。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零二轮包含 `app/runtime/internal/runtime` approval mode / rule management 端口收窄：

- 调整对象：`Runtime.approval`、approval mode/rule runtime facade、runtime assembly 和 focused runtime tests。
- 调整前问题：`Runtime` 持有完整 `approval.Policy` 字段，而 runtime facade 只需要 mode read、mode mutation、rule list 和 rule forget；完整 policy 还包含 tool-call gate 的 `Decide` / `Remember`，这属于 turn dispatcher 的真实执行路径，不该暴露给 settings/management facade。
- 破坏性原因：这些字段位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 management 用例拆分能删除旧总口字段，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `approvalModeReader`、`approvalModeWriter`、`approvalRuleLister`、`approvalRuleDeleter` 四个端口；runtime assembly 继续把同一个 `approval.Policy` 注入到四个字段；turn dispatcher 和 tool environment 仍接收完整 policy，因为它们确实需要 gate / remember 语义。
- 架构收益：approval settings facade 不再可见 tool-call decision/remember 能力；mode 读写与 rule catalog/delete 在 Runtime 类型边界上分开，同时保留 turn gate 对完整 policy 的合理依赖。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、approval facade、runtime assembly 和 focused approval tests。
- 已完成适配：所有 `r.approval.*` runtime facade 调用已迁移到对应端口；测试 fixture 删除了嵌入完整 `approval.Policy` 的宽 fake；未保留旧 `Runtime.approval` 字段。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零三轮包含 `app/runtime/internal/runtime` memory list/read/write 端口收窄：

- 调整对象：`Runtime.knowledge`、memory runtime facade、runtime assembly 和 focused memory tests。
- 调整前问题：delivery 层已经把 memory 管理面拆成 availability、list、read、write 四个访问口，但 Runtime 内部仍持有完整 `knowledge.Store`；list facade 理论上可见 read/write，read facade 理论上可见 write/list，组合层没有延续消费方端口边界。
- 破坏性原因：这些字段位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；删除旧 `Runtime.knowledge` 总口可以避免 memory settings/management facade 继续依赖完整知识库存储契约，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `memoryList`、`memoryRead`、`memoryWrite` 三个 consumer-side port；`Runtime.HasMemory` 只在三个端口均存在时声明 memory capability 可用；list/read/write facade 分别调用自己的端口。kernel prompt 和 maintenance extraction 仍沿用 `knowledge.Store`，因为它们确实消费长记忆读写语义。
- 架构收益：memory catalog projection、单 scope 读取、单 scope 更新在 Runtime 类型边界上分开；完整 `knowledge.Store` 只在 composition root 注入时出现，不再作为默认 runtime state 字段穿透到 management facade。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、memory facade、runtime assembly 和 focused memory tests。
- 已完成适配：runtime assembly 继续把同一个 `cfg.Engine.Knowledge` 注入到三个窄口字段；所有 `r.knowledge.*` memory facade 调用已迁移到对应端口；focused test 覆盖 unavailable 与 list/read/write 转发；未保留旧 `Runtime.knowledge` 字段。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零四轮包含 `app/runtime` codebase index tool/management 端口收窄：

- 调整对象：`Runtime.codebaseIndex`、codebase runtime facade、toolset `CodebaseIndex` 装配口、`codebasesearch.New` 和 focused codebase tests。
- 调整前问题：`codebaseindex.Index` 同时包含 search、ensure/reconcile、forced reindex、status、availability。Runtime 的 UI/RPC facade 只需要 search/status/reindex/availability，`codebase_search` 工具只需要 search，但它们都持有完整 `Index`，理论上可见 `EnsureIndexed` 等内部生命周期能力。
- 破坏性原因：这些字段和构造参数位于 `app/runtime/internal`，属于应用层内部装配边界；按实际消费路径拆掉完整 `Index` 依赖可以删除旧总口形状，不需要为内部旧 API 留兼容层。
- 新设计：Runtime 新增 `codebaseIndexAvailability`、`codebaseIndexSearch`、`codebaseIndexStatus`、`codebaseIndexReindex` 四个 consumer-side port；toolset 新增只组合 search + availability 的 `CodebaseIndex` 端口；`codebasesearch.New` 只接收 search 端口。domain `codebaseindex.Index` 仍保留为 `Indexer` 自身的完整实现契约。
- 架构收益：codebase semantic search、status projection、manual reindex 和 tool availability gate 在类型边界上分开；`EnsureIndexed` 不再泄漏给 Runtime management facade 或 tool adapter，完整 index 只在 composition root 注入时出现。
- 影响范围：`app/runtime/internal/runtime`、`app/runtime/internal/adapter/toolset`、`app/runtime/internal/adapter/toolset/codebasesearch` 和 focused runtime tests。
- 已完成适配：runtime assembly 继续把同一个 `embeddingEnv.index` 注入到四个 Runtime 窄口字段；toolset build/resolver 改为依赖 search+availability 组合口；`codebasesearch.New` 改为只接收 search 端口；focused tests 删除完整 `codebaseindex.Index` 嵌入 fake 并覆盖 search/status/reindex 转发。
- 验证结果：`go test ./internal/runtime ./internal/adapter/toolset ./internal/adapter/toolset/codebasesearch` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零五轮包含 `app/runtime/internal/runtime` utility / embedding role persistence 读写端口收窄：

- 调整对象：`UtilityRoleStore`、`EmbeddingRoleStore`、`Runtime.utilStore`、`Runtime.embeddingStore`、role environment builders 和 focused runtime tests。
- 调整前问题：utility / embedding role 持久化接口同时包含 load 与 save；构造期只需要 load 以初始化 live role cell，运行期 `models.set*Role` 只需要 save，但 `Runtime` 字段持有完整读写 store，setter 理论上可见不需要的 load 能力。
- 破坏性原因：这些接口和字段位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按构造读口与运行期写口拆分能删除运行期总口字段，不需要为内部旧 shape 留兼容层。
- 新设计：新增 `utilityRoleLoader` / `utilityRoleSaver` 与 `embeddingRoleLoader` / `embeddingRoleSaver` 四个 consumer-side port；Config 仍接受组合根提供的完整 store，build environment 只消费 loader，Runtime state 只保存 saver。
- 架构收益：模型角色启动加载、运行期持久化写入和 live role cell 读面在类型边界上分离；运行期 setter 不再携带 load capability，focused tests 也不再通过嵌入完整 role store 掩盖真实依赖。
- 影响范围：`app/runtime/internal/runtime` 的 utility / embedding role code、Runtime struct、runtime assembly 和 focused tests。
- 已完成适配：`buildUtilityEnvironment` / `buildEmbeddingEnvironment` 参数收窄到各自 loader；`Runtime.utilStore` / `Runtime.embeddingStore` 收窄到 saver；runtime assembly 继续把同一个组合根 store 分别注入到构造读口和运行期写口；新增 focused tests 覆盖 loader 初始化与 saver 写入。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零六轮包含 `app/runtime/internal/runtime` hook inspection 端口收窄：

- 调整对象：`Runtime.hookResolver`、workspace hooks inspection facade、runtime assembly 和 focused hooks tests。
- 调整前问题：hook resolver 同时包含 turn 执行所需的 `For` 和 workspace 管理面所需的 `Inspect`；turn dispatcher 已只依赖 `For`，但 Runtime state 仍保存完整 `HookResolver`，使 `workspace.hooks.list` 管理 facade 理论上可见执行绑定能力。
- 破坏性原因：这些字段位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按执行面与管理面消费路径拆分字段能删除运行期总口，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `hookInspector` consumer-side port；Runtime 只保存 `hookInspection` 并在 `InspectHooks` 中 nil-safe 返回空 inspection。Config 仍接受组合根提供的完整 resolver，assembly 分别把它交给 turn dispatcher 的 `For` 口和 Runtime 的 `Inspect` 口。
- 架构收益：lifecycle hook execution binding 与 workspace hook review/projection 在 Runtime 类型边界上分离；管理面不再携带执行 hook binding capability，未配置 hooks 时 facade 也符合注释语义返回空结果。
- 影响范围：`app/runtime/internal/runtime` 的 hook facade、Runtime struct、runtime assembly 和 focused runtime hooks tests。
- 已完成适配：`Runtime.hookResolver` 替换为 `hookInspection hookInspector`；runtime assembly 继续把同一个 `cfg.HooksResolver` 注入给 turn dispatcher 和 Runtime inspection 口；`InspectHooks` 增加 nil-safe 空结果；focused tests 用只实现 `Inspect` 的 fake 锁住管理面窄口。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零七轮包含 `app/runtime/internal/runtime` live MCP engine dependency 端口收窄：

- 调整对象：`Runtime.engine` 在 MCP runtime facade 中的 status/tools/reconnect/authorize/probe/configure/remove 调用、Runtime struct、runtime assembly 和 focused MCP live tests。
- 调整前问题：MCP registry 持久化面已拆成 list/read/configure/remove/enable 端口，但 live MCP facade 仍通过 `r.engine` 访问整台 kernel engine；`workspace.mcp.*` 管理路径理论上可见 turn execution、skills、close、A2A 等无关 engine 能力。
- 破坏性原因：这些字段和调用点位于 `app/runtime/internal/runtime`，属于应用层内部装配边界；按 live MCP 消费路径拆掉对完整 engine 的依赖，不需要为内部旧 shape 留兼容层。
- 新设计：新增 runtime 内部 `mcpLiveStatusReader`、`mcpLiveToolCatalog`、`mcpLiveConnectionCommands`、`mcpLiveRegistryCommands` 四个 consumer-side port；MCP facade 只通过这些端口访问 live MCP 会话，runtime assembly 继续把同一个 `*kernel.Engine` 注入到这些窄口字段。
- 架构收益：live MCP status projection、tool catalog、connection lifecycle、registry apply/probe 与完整 turn engine 在 Runtime 类型边界上分离；`Runtime.engine` 继续服务 close/skills/A2A 等其他真实用途，但不再是 MCP facade 的默认依赖。
- 影响范围：`app/runtime/internal/runtime` 的 MCP facade、Runtime struct、runtime assembly 和 focused runtime MCP tests。
- 已完成适配：所有 MCP live status/tool/reconnect/authorize/probe/configure/remove 调用已迁移到四个窄口字段；runtime assembly 继续把同一个 `*kernel.Engine` 注入到这些端口；focused tests 用只实现 live MCP 端口的 fake 锁住 facade 依赖。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零八轮包含 `app/runtime/internal/runtime` 剩余 full engine state 字段移除：

- 调整对象：`Runtime.engine` 对 close、workspace.listSkills、A2A adapter 的剩余用途，Runtime struct、runtime assembly 和 focused runtime port tests。
- 调整前问题：第一百零七轮后 MCP facade 已不再依赖完整 engine，但 `Runtime` 本体仍持有 `*kernel.Engine` 字段；Close、ListSkills、A2AAgent 三条路径理论上都能借这个字段看见 turn execution、MCP live registry、tools 等不属于各自 facade 的能力，Runtime state 仍把完整 engine 当作默认共享依赖。
- 破坏性原因：`Runtime.engine` 是 `app/runtime/internal/runtime` 内部装配字段；删除它并按消费路径注入窄口能直接消除最后的 full-engine state 持有点，不需要保留旧字段或兼容 shim。
- 新设计：新增 `runtimeCloser`、`skillCatalog` 和既有 `chatRunner` 三个 consumer-side port；`Runtime.Close`、`Runtime.ListSkills`、`Runtime.A2AAgent` 分别只使用对应端口。runtime assembly 继续把同一个 `*kernel.Engine` 注入给这些端口，但 Runtime 类型边界不再保存完整 engine。
- 架构收益：Runtime bundle 从“持有整台 kernel engine”改为“按用例持有关闭、技能目录、A2A chat runner 能力”；管理面/协议 adapter 不再默认携带 turn engine 的完整 surface，composition root 仍保持单一 engine 实例和资源所有权。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、close/skills/A2A facade、runtime assembly 和 focused runtime tests。
- 已完成适配：删除 `Runtime.engine` 字段；`Close`、`ListSkills`、`A2AAgent` 已迁移到对应窄口；focused tests 用只实现 closer/skills/chat-runner 的 fake 锁住端口依赖。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。

第一百零九轮包含 `app/runtime/internal/runtime` maintenance title adapter state 收窄：

- 调整对象：`Runtime.titler`、`Runtime.GenerateTitle`、runtime assembly 和 focused maintenance title tests。
- 调整前问题：`Runtime` struct 直接持有具体 `*maintenance.Titler`，使应用层 runtime bundle 的 state 边界依赖 adapter 层具体类型；而 Runtime facade 和 runsegment side effects 只需要“给首条用户消息生成标题”的单方法能力。
- 破坏性原因：该字段位于 `app/runtime/internal/runtime` 内部装配边界；删除具体 adapter 字段并改为 consumer-side title port 能减少 adapter 类型穿透，不需要保留旧字段名或兼容 shape。
- 新设计：新增 `titleGenerator` 端口（仅 `Generate`）；`Runtime` 保存 `titles titleGenerator`，`GenerateTitle` 通过端口调用，并在未配置时保持 best-effort 空标题；runtime assembly 继续用 `maintenance.NewTitler` 作为实现注入。
- 架构收益：具体 maintenance adapter 只出现在 composition/wiring 层；Runtime state 和 runsegment title side effect 只依赖标题生成用例能力，应用层 bundle 与 adapter 实现进一步解耦。
- 影响范围：`app/runtime/internal/runtime` 的 Runtime struct、maintenance facade、runtime assembly 和 focused maintenance tests。
- 已完成适配：`Runtime.titler` 替换为 `titles titleGenerator`；`GenerateTitle` 迁移到端口并补充 nil-safe best-effort 分支；focused tests 用只实现 `Generate` 的 fake 锁住端口依赖。
- 验证结果：`go test ./internal/runtime` 通过；三模块 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；`golangci-lint run`（`app/runtime`）通过；`git diff --check` 通过。
- 后续风险：无跨模块公开 API 风险。
