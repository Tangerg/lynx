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
- 已完成目标模块低误伤异味扫描：
  - 常量 `fmt.Errorf("...")` 未命中。
  - `TODO` / `FIXME` / `HACK` 未命中。
  - `McpToolInfo` / `McpServerStatus` / `notImpl` 未命中。
  - `helpers_test.go` / 泛测试文件名未命中。
  - `core` Go 文件未命中 `Lyra`、`app/runtime`、`vectorstores`、`workspace.*` 等上层 / sibling 模块语义。
  - `agent` Go 文件除架构测试自身的禁止规则外，未命中 `Lyra`、`app/runtime`、`workspace.*` 等上层应用语义。
- 已运行 `git diff --check`，未发现 whitespace/error marker 问题。

### 5.2 未完成

- 本批未做破坏性公共 API 重构；如后续继续收敛 exported shape，需要先确认 scope、影响面和备选方案。
- 本批只处理 `core`、`agent`、`app/runtime`；前端和其他模块不纳入本轮质量结论。

### 5.3 暂不处理的问题

- `agent/runtime/mcp.go` 直接承载 MCP 便利集成：已有 agent 架构评审明确裁决为“关注点不同但紧密依赖 `*Platform`/`*AgentProcess`，当前保持在 runtime 包内”。本轮不拆。
- `agent/core.ServiceProvider` 是开放 service locator：已有 agent 架构评审裁决为 SDK 库的 architecture tax，可接受。本轮不改。
- `app/runtime` 的 `delivery/server` 总 LOC 较大：已有文档裁决协议方法 1:1 绑定是健康的，且原 use-case 泄漏 P0 已落地。本轮不按文件数拆。
- `agent/event.Multicast` 的 listener panic span 目前没有 caller context 可用；根治需要给 `Listener.OnEvent` 引入 context 或新增带 context 的 delivery surface，属于公共 API 破坏性调整。本轮只记录，不在未确认 scope 前直接修改。

### 5.4 风险与注意事项

- 本任务允许破坏性调整，但公共 API 破坏性修改仍需先确认 scope、影响面和备选方案。
- `agent` 和 `app/runtime` 体量较大，需要分批推进，避免无目标重写。
- 多模块 workspace 中的模块版本引用可能由 `go.work` 覆盖，依赖治理需要同时看 `go.mod` 和实际 import。
- `core` 与 `agent` 均依赖 `pkg` 工具模块；后续只治理有明确收益、能减少真实耦合的用法，不为了“依赖更少”而机械复制 helper。
- 本轮尚无破坏性调整；新增 arch tests 会在未来违反依赖边界时让测试失败，这是预期的防腐行为。

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

暂无破坏性调整。
