# Pi：紧凑工具循环与未完成的持久化 Harness

证据基线：`pi` 提交 `853a80d26c90a14c1886f0ebb8ffaae133ca2185`。

## 框架层判断

Pi 不是一个单体“Coding Agent 框架”。仓库中至少应拆成三个层次看：

- `pi-ai`：统一多提供商模型接口。
- `pi-agent-core`：模型—工具循环、状态型 Agent 和正在建设的 Harness。
- `pi-coding-agent`：交互式编码应用。

本轮只比较前两层。Coding Agent 的终端、编辑、会话 UI 和内置工具属于应用维度，不计入框架内核结论。

Pi 已交付部分最强的地方，是以很小的接口形成可嵌入、可插入生命周期回调、支持多提供商的工具循环。它与 Scope 的核心目标并不相同：Pi 优先优化直接执行与交互生产力，Scope 优先优化受管副作用与中断恢复。

## 实际核心

### 模型层

`packages/ai/src/types.ts` 定义统一的 `Model`、消息、工具调用、流事件和请求选项；`packages/ai/src/models.ts` 提供 provider registry 与动态模型目录。

这层的优点是调用体验统一，thinking、tool call、usage 和错误都能通过共同流协议表达。代价是公共类型保留了若干提供商特有字段，`pi-ai` 包也直接依赖 Anthropic、Bedrock、Google 和 OpenAI SDK。它是务实的统一层，不是完全供应商无关的领域内核。

### 低层 Agent 循环

`packages/agent/src/types.ts` 与 `agent-loop.ts` 的关键抽象包括：

- 注入式 `StreamFn`，将模型实现与循环解耦。
- `AgentLoopConfig`，允许消息转换、上下文裁剪、密钥解析、停止条件、下一轮准备和工具前后回调。
- TypeBox 工具 schema，支持顺序或并行执行。
- steering 与 follow-up 队列。
- 可扩展的 `AgentMessage`，应用可通过 TypeScript declaration merging 加入自定义消息类型。

循环会直接调用模型和工具、更新 transcript，并把进度发成事件。它还拒绝执行参数被截断的 tool call，这是一个值得保留的真实性与安全边界。

### 状态型 Agent

`packages/agent/src/agent.ts` 在低层循环外提供状态、事件订阅、活动运行控制和消息队列。`AgentState` 同时包含消息与工具，也包含 streaming message、pending tool calls 和错误等交互状态。

这让交互式应用非常容易订阅完整状态，但“可持久化领域状态”与“瞬时运行/UI 状态”没有像 Scope 的 Host/Execution 那样天然分开。

## Harness：设计方向不能当成交付能力

`packages/agent/src/harness/agent-harness.ts` 及相关类型已经描述了一套明显更强的运行模型：

- operation log 与 operation outcome；
- lane、branch 和 session snapshot；
- `OperationStarted`、`StepAttempt`、`ToolStarted`、`OperationFinished`、`UsageRecord` 等记录；
- 工具 replay 策略 `never | safe`；
- JSONL、SQLite、内存存储接口；
- resume、compaction、tree navigation、queued operation 和 action。

这些类型说明 Pi 团队已经意识到低层循环不足以承担持久化运行和重放。但在本次提交中，已有记录时的 `create` 恢复路径会抛出 `HarnessNotImplemented`，`prompt`、`resume`、`compact`、树导航和多项 operation API 也仍走未实现分支。

因此，本轮结论必须分成两句：

1. Pi 有清晰、值得关注的 durable harness 设计方向。
2. Pi 当前可依赖的主能力仍是低层直接执行循环，不能把 Harness 类型表面算成完整恢复实现。

## 八维对照

| 维度 | Pi 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 统一多提供商体验，公共类型容纳提供商特性 | Scope 核心更纯；Pi 适配体验更直接 |
| 最小契约 | `StreamFn` + tools 即可运行 | Pi 更轻；Scope 从入口就要求恢复语义 |
| 状态所有权 | Agent 持有 transcript 和运行中状态 | Scope 将产品状态留给 Host、执行状态留给 Execution |
| 副作用 | 循环直接调用模型和工具 | Scope 先描述 Effect，再由运行时执行 |
| 编排 | 低层以连续工具循环为主 | Scope 有受管子 Process/Workflow |
| 恢复 | 低层无完整 durable recovery；Harness 尚未完成 | Scope 的 snapshot/restore 属于当前内核 |
| 扩展 | 事件、具名 hooks、自有 telemetry schema | Scope 更偏 middleware/listener + OTel 适配 |
| 包边界 | AI、agent、coding app 分包；agent-core 导出面较宽 | Scope 叶子模块隔离更细，也更难治理 |

## Scope 应该借鉴什么

1. **把简单路径做成真正的一等入口。** Pi 证明了注入模型流函数和工具即可形成高质量循环，不必先理解完整运行时。
2. **消息扩展机制。** 应用消息与模型消息可以分层，并在进入模型前转换，避免产品事件污染基础模型协议。
3. **清晰的流事件。** 模型增量、工具开始/结束、turn 开始/结束和错误都能被调用方直接消费。
4. **截断参数不执行。** 协议不完整时拒绝副作用，比猜测或容错执行更可靠。
5. **重放策略显式化。** Harness 的 `never | safe` 虽未完整落地，但比默认假设所有工具都可重放更诚实。

## Scope 不应照搬什么

1. 不应让提供商 SDK 或专有字段回流到 `core`。
2. 不应把 UI 邻近状态加入持久化 Execution 快照。
3. 不应为了简单 API 放弃 Effect 身份；长期任务的恢复语义正是 Scope 的设计中心。
4. 不应把尚未实现的持久化 API 提前扩大成稳定公共面。
5. 不应让 coding 场景的搜索、session 和工具集合逐步挤入通用 Agent 内核。

## 最终定位

Pi 是 Scope 最有价值的新对照之一，因为它揭示的不是“少了哪些功能”，而是另一种框架成功标准：最短的可用路径、极强的交互事件和务实的提供商统一。

Scope 比 Pi 当前低层运行时拥有更完整的受管恢复语义；Pi 比 Scope 拥有更自然的短工具循环嵌入体验。两者应互相校正，而不应被压成一个总分。
