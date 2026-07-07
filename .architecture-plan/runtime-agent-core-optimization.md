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
- 已完成三模块回归验证：
  - `go test ./...`（`core`）通过（第十一轮后复跑）。
  - `go test ./...`（`agent`）通过（第十一轮后复跑）。
  - `go test ./...`（`app/runtime`）通过（第十一轮后复跑）。
  - `go vet ./...`（`core`）通过（第十一轮后复跑）。
  - `go vet ./...`（`agent`）通过（第十一轮后复跑）。
  - `go vet ./...`（`app/runtime`）通过（第十一轮后复跑）。
  - `go build ./...`（`core`）通过（第十一轮后复跑）。
  - `go build ./...`（`agent`）通过（第十一轮后复跑）。
  - `go build ./...`（`app/runtime`）通过（第十一轮后复跑）。
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
