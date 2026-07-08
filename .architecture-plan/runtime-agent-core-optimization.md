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

### 5.1 历史摘要（第一轮至第一百一十二轮）

此前 112 轮已完成多批真实架构优化，详细展开记录已按本轮要求压缩为本摘要。历史主线如下：

- 基线与防腐：建立/补强 `core/internal/arch`、`agent/internal/arch`、`app/runtime/internal/arch` 等架构测试，持续验证 `app/runtime -> agent -> core` 依赖方向、agent 内部 dependency ladder 和 app/runtime Clean Architecture ring。
- `core` 治理：清理上层路径/语义锚点，保持协议层文档和模型 API 不感知 provider/app 布局；拆分 `core/model/chat` client 调用/流式/请求构造职责；清理错误、命名和测试夹具卫生。
- `agent` 治理：确认 SDK 库内部直接依赖具体类型的设计裁决，补强 agent 不依赖 app/runtime 的机器防腐；清理 Lyra 场景化测试注释、泛文件名和泛测试替身命名；完成 context-first 等公开 API 收敛。
- `app/runtime` delivery/server 治理：按 workspace、provider、session、schedule、memory、transcript、approval、MCP、lifecycle、turn、codebase、hook/model role 等用例拆分 RuntimePort 访问面，避免 delivery server 持有宽 facade。
- `app/runtime/internal/kernel` 治理：收窄 Engine、turn dispatcher、lifecycle、runsegment、toolport 和 live MCP 控制依赖口；删除 `kernel` 顶层 `toolport` alias，让工具/MCP 端口类型归属 `kernel/toolport`。
- `app/runtime/internal/runtime` 治理：持续把完整 domain/service/adapter state 拆成 consumer-side ports，包括 provider、MCP registry、schedule、session、transcript、interrupt、tool catalog/invocation、approval、memory、codebase index、role persistence、hook inspection、live MCP、engine close/skills/A2A、title generation、conversation history、utility chat client resolver 等。
- `app/runtime` adapter/infra 结构治理：拆分 `infra/mcp`、`kernel/turn`、`kernel/lifecycle`、delivery translator、toolset resolver 等大文件，使文件职责贴近连接生命周期、turn 生命周期、用例边界或工具组装边界。
- 验证纪律：历史各轮以 targeted tests + 三模块 `go test ./...`、`go vet ./...`、`go build ./...` 为主要门槛；部分轮次额外执行 `golangci-lint run` 与 `git diff --check`。

### 5.2 当前轮记录

- 已读取新的 objective 文件：`/Users/tangerg/.codex/attachments/248682d6-1703-4dd0-876b-dde472455b0d/goal-objective.md`。
- 已按 objective 要求将此前冗长历史记录压缩为本摘要；后续本文件只保留里程碑级摘要和本轮必要细节。
- 已扫描 `core/model/embedding.GetDimensions` 命名债务；该 API 被 `models/*` 与 `vectorstores/*` 广泛调用，超出本轮 `core` / `agent` / `app/runtime` 调整边界，暂不作为当前里程碑。
- 已扫描 `agent` 的 panic / `ProcessOptions` / service registry 路径；未发现比现有设计裁决更高收益的小步，保留 SDK 库内部单实现直接依赖具体类型的裁决。
- 已修复 `app/runtime` scheduling 可选依赖契约：`Config.ScheduleRegistry == nil` 时，runtime 管理端口改接 disabled registry 并返回 `schedule.ErrUnavailable`，scheduler worker no-op；delivery/server 将该 domain error 映射为 `capability_not_negotiated`，避免 schedules.* 在禁用配置下 nil panic。
- 已补测试：runtime 覆盖缺失 schedule registry 的禁用行为；delivery/server 覆盖 schedule unavailable 到 protocol capability error 的映射。
- 已完成完整验证：`core`、`agent`、`app/runtime` 三个模块的 `go test ./...`、`go vet ./...`、`go build ./...` 均通过。

### 5.3 未完成

- 当前轮仍需提交并推送本轮架构优化；用户要求跟踪的 staged `AGENTS.md` 与 frontend example 文件需继续排除在架构提交之外。

### 5.4 暂不处理的问题

- 不为了“每个模块都动一下”而做形式化重构；若 `core` 或 `agent` 没有高收益小步，将只记录扫描结果。
- 用户上轮要求加入跟踪的 `AGENTS.md` 与 `app/desktop/frontend/example/*.html` 已处于 staged 状态，但不属于本轮架构优化范围；后续提交架构改动时需避免混入。

### 5.5 风险与注意事项

- 用户已批准公共 API 调整可不逐项询问，但破坏性调整仍必须记录原因、影响、收益、适配和验证结果。
- 三个目标模块是独立 Go module；跨模块 API 调整需要同步所有本地调用方并分别验证。
- 计划文件压缩会减少逐轮展开细节；若需要追溯完整历史，以 git history 中压缩前版本为准。

---

## 6. 后续演进方向

- 继续优先查找宽接口、总口 state、顶层 facade re-export、上层语义泄漏、职责混杂和测试替身掩盖真实依赖的问题。
- `app/runtime/internal/runtime` 已大量端口收窄，下一步可重点检查仍留在组合根或 server facade 的宽依赖是否有实际消费方分裂空间。
- `agent` 若继续调整，应尊重 SDK 库内部“单实现直接用具体类型”的裁决，把窄接口留给公开 SPI 或应用层消费方。
- `core` 若继续调整，应保持基础协议层薄核定位，避免为 app/runtime 或 agent 的单一场景下沉具体概念。

---

## 7. 破坏性调整记录

### 7.1 记录规范

后续破坏性调整按以下字段记录：

- 调整对象：
- 调整前问题：
- 破坏性原因：
- 新设计：
- 架构收益：
- 影响范围：
- 已完成适配：
- 验证结果：
- 后续风险：

### 7.2 历史破坏性调整摘要（第六十一轮至第一百一十二轮）

- `app/runtime/internal/delivery/server`：按 provider/session/schedule/memory/transcript/approval/MCP/lifecycle/turn/workspace 等用例切碎 server runtime access ports，删除多个宽 `RuntimePort` 访问面。
- `app/runtime/internal/kernel`：拆分 Engine/turn/lifecycle/runsegment 的内部依赖口，收窄 live MCP 控制面，移除 `kernel` 顶层 `toolport` alias；总体方向是让 kernel orchestration 只看见实际消费的端口。
- `app/runtime/internal/runtime`：删除多个完整 service/adapter state 字段，改为 per-use-case consumer-side ports；完整实现只留在 composition root 注入处。
- `app/runtime` adapter/infra/domain：对 schedule worker、codebase index、hook inspection、role persistence、MCP connections 等内部 API 做过按消费路径的破坏性收窄。
- `agent`：完成过 context-first 公开 API 收敛，并补强防止 app/runtime 反向依赖与内层 import 顶层 facade 的规则。
- `core`：历史调整以文档/职责/文件结构和卫生清理为主，避免下沉上层具体概念。

### 7.3 当前轮破坏性调整

暂无。本轮新增 `schedule.ErrUnavailable` 并补齐禁用 scheduling 的错误语义，属于向内补强可选依赖契约；未删除或改签名公开 API。
