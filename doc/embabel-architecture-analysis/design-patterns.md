# 设计模式与扩展机制

> 0.4.0-SNAPSHOT

本文档分两部分:A. **框架级设计模式**(GOAP / 黑板 / 三值逻辑 / 事件驱动等);B. **SPI 扩展点目录**(可替换接口和默认实现)。

---

# A. 设计模式

## A.1 GOAP(Goal-Oriented Action Planning)

**来源**:游戏 AI(Jeff Orkin 为 *F.E.A.R.* 提出)

**核心**:不让开发者硬编码"先做 A,再做 B,再做 C",而是让 Agent 自己找路:
```
Action 声明    preconditions(需要什么) + effects(产生什么)
Goal 声明      preconditions(目标达成意味着什么)
Planner        从当前 WorldState 出发,用 A* 在 Action 图上找到达 Goal 的最短路径
```

**A\*** 的 f = g + h:
- `g` = 累计 `action.cost()`(可动态:`@Cost` 方法或 `costMethod`)
- `h` = 启发式 = goal 还有几个前置条件未满足

**value(效用)**:`Goal.value` 是一个函数 `(WorldState) -> Double`,规划器在多个可达 Goal 之间按 `value - cost` 排序。这让 Agent 可以权衡**更贵但更有价值**的目标。

## A.2 黑板(Blackboard)

黑板把 Action 之间的通信从"直接调用"改为"写入 / 查询共享内存":

```
Action A 输出 UserInput   →   Blackboard   →   Action B 需要 UserInput
     └ 不感知 B 的存在                                └ 不感知 A 的存在
```

**关键不变量**:
- **append-only**(只能加,不能删;只能 `hide()` 过滤)
- **类型层级匹配**(声明 `Person` 能匹配到黑板里的 `Employee`)
- **受保护绑定**(跨 `clear()` 保留)

这让 Action 成为"纯函数式"的:输入来自黑板,输出进入黑板。副作用通过显式的工具调用完成。

## A.3 三值逻辑 + UNKNOWN 作为一等公民

`ConditionDetermination = TRUE | FALSE | **UNKNOWN**`

**UNKNOWN 的两个战略意义**:
1. **规划层**:让规划器可以主动选择"能消除不确定性"的 Action(如"查询数据库"先于"决策")
2. **执行层**:SpEL 条件引用不存在的黑板变量时,0.4.0 改为返回 **FALSE**(#950b32ce);避免不相关 Goal 的前置条件被当作"未知而潜在可达",拖慢规划

## A.4 双编程模型

注解模型和 DSL 产生**完全相同的**领域对象:

```
@Agent + @Action + @Goal          agent { action { ... } goal { ... } }
           │                                      │
           ▼                                      ▼
   AgentMetadataReader                    AgentBuilder.build()
           │                                      │
           └──────────→ Agent(actions, goals, conditions) ←────────┘
```

内部是同一条执行路径,选择只是风格偏好。`AgentBuilder` 在 DSL 内部也复用 `AgentMetadataReader` 的一些构建逻辑。

## A.5 事件驱动

**事件层次**:
```
AgenticEvent
├── AgentPlatformEvent       (平台级:Agent 部署 / 卸载)
└── AgentProcessEvent        (进程级:20+ 具体事件)
    ├── ProcessCreated/Ready/Completed/Failed/Terminated/Killed
    ├── ActionExecutionStart/End
    ├── GoalAchieved
    ├── ReplanRequested
    ├── LlmCall*(多个细粒度事件)
    ├── ToolCallRequest
    └── ...
```

**多播**:`MulticastAgenticEventListener` 把事件分发给所有 `AgenticEventListener` bean。

**内置监听器**:
- `LoggingAgenticEventListener`(默认控制台)
- 主题日志:`ColossusLoggingAgenticEventListener` / `HitchhikerLoggingAgenticEventListener` / `SeveranceLoggingAgenticEventListener` / `StarWarsLoggingAgenticEventListener`
- 运维级:`AgenticEventListenerToolsStats` · `EmbabelMetricsEventListener`(Micrometer) · `EmbabelFullObservationEventListener`(OTel) · `MdcPropagationEventListener`

## A.6 OODA 显式分层

和纯状态机不同,Embabel 把 OODA 四个阶段**显式分离**:
```
Observe   WorldStateDeterminer.determineWorldState()
Orient    Planner.bestValuePlanToAnyGoal()
Decide    选中 plan.actions[0](或并发的 achievable subset)
Act       executeAction() → 更新黑板
```
每一环都可替换(SPI)。

## A.7 Subagent / 组合

Agent 可以作为另一个 Agent 的 Action 使用,两种方式:

1. **`Subagent` 工具**(推荐,`api.tool.Subagent`):把子 Agent 封装为 LLM 可调用的 Tool;子进程与父进程共享黑板(`ofClass` / `byName` / `ofInstance` / `ofAnnotatedInstance`)。
2. **`RunSubagent` 异常式**(`api.annotation.RunSubagent`):在 Action 内调用 `RunSubagent.instance(agent, ResultType::class.java)`,**永远抛出** `SubagentExecutionRequest`,框架捕获后运行子进程;**调用点之后的代码不可达**(#1390 修复后 IDE 会给出警告)。

## A.8 工具循环(Tool Loop)

`ToolLoop` 接口把"LLM 请求调用 → 执行工具 → 把结果喂回 LLM"整条链路抽象,**不依赖 Spring AI 的内部循环**:

```
DefaultToolLoop        串行执行
ParallelToolLoop       基于 Asyncer SPI,并发执行独立工具
```

**为什么自研?**:Spring AI 的工具循环不暴露足够的钩子(消息历史修剪、动态注入、可观测性)。自研后可以:
- 用 `SlidingWindowTransformer` 做历史窗口(#1569 支持保留 tool call/result 配对)
- 用 `ToolInjectionStrategy` 按对话进度动态增/减工具(MatryoshkaTool 俄罗斯套娃模式)
- 用 `ToolNotFoundPolicy` 处理 LLM 幻觉工具名(如 `AutoCorrectionPolicy` 做模糊匹配)
- 用 `GuardRail` 在每轮验证 LLM 输出

## A.9 Guardrails 分层

```
UserInputGuardRail        用户输入进 → 先验证
       │
       ▼
    LLM 调用
       │
       ▼
AssistantMessageGuardRail 模型输出 → 后验证(0.4.0 起支持 ThinkingResponse<*> 和结构化对象响应,#1582)
```

两种守护都基于同一个 `GuardRail` sealed interface,最终落到 `ValidationResult`。

---

# B. SPI 扩展点目录

| SPI | 文件 | 抽象 | 默认实现 |
|-----|------|------|----------|
| **PlannerFactory** | `spi/PlannerFactory.kt` | 创建 Planner | `DefaultPlannerFactory`(GOAP / Utility / Condition) |
| **WorldStateDeterminer** | `plan.common.condition.WorldStateDeterminer` | 从外部(通常是黑板)计算 WorldState | `BlackboardWorldStateDeterminer` |
| **BlackboardProvider** | `spi/BlackboardProvider.kt` | 创建黑板 | 创建 `InMemoryBlackboard` |
| **AgentProcessRepository** | `core.AgentProcessRepository` | 进程持久化 (CRUD + `findByParentId`) | `InMemoryAgentProcessRepository` |
| **ContextRepository** | `spi/ContextRepository.kt` | 会话 / 上下文持久化 | `InMemoryContextRepository` |
| **OperationScheduler** | `spi/OperationScheduler.kt` | Action 调度(延迟 / 暂停) | `ProntoOperationScheduler`(立即) / `ProcessOptionsOperationScheduler` |
| **Asyncer** | `spi/config/spring/AsyncConfiguration.kt` | 并发 / 线程 / 上下文传播 | 基于 Kotlin 协程 |
| **ToolGroupResolver** | `spi/ToolGroupResolver.kt` | 按角色解析 ToolGroup | `RegistryToolGroupResolver` |
| **ToolDecorator** | `spi/ToolDecorator.kt` | 为 Tool 包装埋点 | `DefaultToolDecorator`(no-op) |
| **AgentProcessIdGenerator** | `spi/AgentProcessIdGenerator.kt` | 进程 ID 生成 | `RANDOM` / `DefaultAgentProcessIdGenerator` |
| **LlmService<T>** | `spi/LlmService.kt` | 厂商中立的 LLM 抽象(builder 风格) | `SpringAiLlmService` |
| **LlmMessageSender** | `spi/loop/LlmMessageSender.kt` | 非流式单次调用 | 由 `LlmService.createMessageSender()` 产出 |
| **LlmMessageStreamer**(新,#1581) | `spi/loop/streaming/LlmMessageStreamer.kt` | 厂商中立的流式调用 | `SpringAiLlmMessageStreamer` |
| **AutoLlmSelectionCriteriaResolver** | `spi/AutoLlmSelectionCriteriaResolver.kt` | "auto" 模型的选择规则 | `DefaultAutoLlmSelectionCriteriaResolver` |
| **StreamingLlmOperations** | `spi/streaming/StreamingLlmOperations.kt` | 流式文本 / 对象 / thinking 流 | `StreamingChatClientOperations` |
| **ToolLoop** | `spi/loop/ToolLoop.kt` | LLM ↔ 工具互动循环 | `DefaultToolLoop` / `ParallelToolLoop` |
| **ToolLoopFactory** | `spi/loop/ToolLoopFactory.kt` | 创建 ToolLoop | `ConfigurableToolLoopFactory` |
| **ToolInjectionStrategy** | `spi/loop/ToolInjectionStrategy.kt` | 对话中动态增删工具 | `ChainedToolInjectionStrategy`(含 Matryoshka) |
| **ToolNotFoundPolicy** | `spi/loop/ToolNotFoundPolicy.kt` | LLM 叫了不存在的工具时的策略 | `AutoCorrectionPolicy`(模糊匹配)/ `ImmediateThrowPolicy` |
| **TerminationScope**(新,#1593) | `api.common.TerminationSignal` | `ToolGroupRequirement.terminationScope` 控制缺失工具时抛的异常类型 | `null / AGENT / ACTION` |
| **UserInputGuardRail** | `api.validation.guardrails.UserInputGuardRail` | 输入预校验(多轮 / 多模态) | — |
| **AssistantMessageGuardRail**(增强,#1582) | `api.validation.guardrails.AssistantMessageGuardRail` | LLM 输出校验(含 `ThinkingResponse<*>`) | — |
| **AgentValidator** | `spi/validation/AgentValidator.kt` | Agent 定义合法性 | `AgentStructureAgentValidator` / `PathToCompletionAgentValidator` |
| **ValidationPromptGenerator** | `spi/validation/ValidationPromptGenerator.kt` | 把 JSR-380 约束描述进提示词 | 默认实现 |
| **AgenticEventListener** | `api.event.AgenticEventListener` | 事件消费 | 多个日志 / 指标 / 追踪实现 |
| **ColorPalette** | `spi/logging/ColorPalette.kt` | 终端日志主题 | 多个主题 |
| **TokenCountEstimator<T>**(新,#1536) | `embabel-agent-common/agent-ai/.../TokenCountEstimator.kt` | Token 计数 | `CharacterHeuristicTokenCountEstimator`(4 char/token) / `NOOP` |
| **ByokFactory<T>**(泛型化,#1609) | `embabel-agent-byok/.../ByokFactory.kt` | 验证 API Key 后返回服务 | 各 LLM 提供方各自实现 |
| **ChunkingContentElementRepository** | `rag-core/.../store/*` | RAG 分块 + 持久化 | Lucene / Neo4j 各自 |
| **EmbeddingAwareChunkingContentElementRepository**(抽出,#1574) | 同上 | 分块 + **嵌入**后持久化 | 由具体后端实现 `persistChunksWithEmbeddings` |
| **SkillScriptExecutionEngine** | `skills/script/*` | 执行 Skill 脚本 | `ProcessSkillScriptExecutionEngine` / `DockerSkillScriptExecutionEngine` |
| **DirectorySkillDefinitionLoader** | `skills/support/*` | 从目录加载 Skill | `DefaultDirectorySkillDefinitionLoader` + `GitHubSkillDefinitionLoader` |
| **SkillFrontMatterFormatter** | `skills/support/*` | Claude / Cursor 等不同 front-matter 风格 | `ClaudeFrontMatterFormatter` · `CursorFrontMatterFormatter` |
| **A2ARequestHandler / AgentCardHandler / AgentSkillFactory** | `a2a.server.*` | A2A 协议端点 | `AutonomyA2ARequestHandler` · `EmbabelServerGoalsAgentCardHandler` · `FromGoalsAgentSkillFactory` |
| **EmbeddingService** | `agent-ai/.../EmbeddingService.kt` | 嵌入 | `SpringAiEmbeddingService`(远程)/ `OnnxEmbeddingService`(本地) |

## 关键 SPI 组合场景

**替换持久化层**:
```kotlin
@Bean
fun agentProcessRepository(ds: DataSource): AgentProcessRepository = 
    JdbcAgentProcessRepository(ds)

@Bean
fun contextRepository(jedis: Jedis): ContextRepository = 
    RedisContextRepository(jedis)
```

**替换并发策略**:
```kotlin
@Bean
fun asyncer(): Asyncer = VirtualThreadAsyncer()   // JDK 21 虚拟线程
```

**替换嵌入为本地**:
```kotlin
# application.yml
embabel.agent.onnx.models.embedding-model: sentence-transformers/all-minilm-l6-v2
# starter-onnx 自动把 OnnxEmbeddingService 注册为 EmbeddingService
```

**自定义守护**:
```kotlin
@Component
class ToxicityGuardRail : UserInputGuardRail { ... }
@Component
class ComplianceGuardRail : AssistantMessageGuardRail { ... }
```

**自定义工具注入**:
```kotlin
class PhasedInjectionStrategy : ToolInjectionStrategy {
    override fun evaluate(ctx: ToolInjectionContext): ToolInjectionResult =
        if (ctx.iteration == 3) add(premiumTools) else noChange()
}
```

---

# C. 近期变更图(0.3.5 → 0.4.0)

| PR/Commit | 影响维度 | 说明 |
|-----------|---------|------|
| #1536 | 新 SPI | `TokenCountEstimator<T>` 作为 Token 计数入口 |
| #1538/#1618 | 合法性校验 | `@Agent` 构造器禁止注入 `OperationContext`(会绑定到占位进程) |
| #1568 | 数据刷新 | OpenAI / Gemini / Bedrock / Mistral 模型与定价 2026 更新 |
| #1569 | 工具循环 | `SlidingWindowTransformer` 保留 tool call/result 配对 |
| #1572 | 一致性 | 平台 `actionQos` 默认同时作用于 DSL / workflow 构造的 Action |
| #1574 | RAG | 抽出 `EmbeddingAwareChunkingContentElementRepository` |
| #1581 | 新 SPI | `LlmMessageStreamer` 厂商中立流式接口 |
| #1582 | 守护修复 | `AssistantMessageGuardRail` 支持结构化对象响应 |
| #1589 | 测试补强 | Gemini 守护集成测试 + 并行工具循环验证 |
| #1593 | 失败处理 | `TerminationScope` 附在 `ToolGroupRequirement` |
| #1596 | 文档 / 行为明确化 | `RunSubagent` 确认总是抛异常,调用后代码不可达 |
| #1599 | 重规划 | `ReplanRequestedException` 支持扩展到 `ConcurrentAgentProcess` |
| #1603 | 工具元数据 | `Tool.Definition.metadata: Map<String,Any>` 应用级路由 |
| #1609 | 泛型化 | `ByokFactory<T>` 不再只用于 LLM |
| #1615/#368 | 成本正确性 | 父进程递归累加子进程成本 |
| 306385b1 | 自治绑定 | `Autonomy.runAgent` 同时按 `"it"` + 类型衍生名绑定 |
| 950b32ce | 规划正确性 | 缺失变量的 SpEL 条件返回 FALSE 而非 UNKNOWN |
| **skills 模块迁移** | 新能力 | `embabel-agent-skills` 保留历史地从旧仓库迁入 |
