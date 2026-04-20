# 模块拓扑

> 版本:0.4.0-SNAPSHOT(父 POM: `embabel-build-parent:0.1.13-SNAPSHOT`)

Embabel Agent 是 Maven 多模块项目,共 **17 个根模块**、展开后近 **80 个子模块**。模块按职责分七层:

```
┌────────────────────────────────────────────────────────────┐
│  (7) 测试支持 / embabel-agent-test-support/*               │
├────────────────────────────────────────────────────────────┤
│  (6) Starter 聚合 / embabel-agent-starters/*(21 个)       │
├────────────────────────────────────────────────────────────┤
│  (5) Spring Boot 自动配置 / embabel-agent-autoconfigure/*  │
├────────────────────────────────────────────────────────────┤
│  (4) 集成层 / a2a / mcp / shell / openai / onnx /          │
│      observability / skills                                │
├────────────────────────────────────────────────────────────┤
│  (3) 领域层 / rag / code / domain / skills                 │
├────────────────────────────────────────────────────────────┤
│  (2) 核心 API / embabel-agent-api + embabel-agent-common/* │
├────────────────────────────────────────────────────────────┤
│  (1) BOM / embabel-agent-dependencies                      │
└────────────────────────────────────────────────────────────┘
```

---

## (1) BOM 层

### embabel-agent-dependencies(pom)

Bill of Materials。统一管理所有 `embabel-agent-*` 模块、Spring AI、OpenTelemetry、Jackson、Lucene、Tika、Kotlin 的版本。被根 `pom.xml` 以 `<scope>import</scope>` 方式引入;同时导入 `embabel-common-dependencies`、`embabel-common-test-dependencies`、`netty-bom`(为修复 CVE 固定在 4.1.132.Final)。

---

## (2) 核心 API 层

### embabel-agent-api(jar,**核心**)

整个框架的"心脏"。包名和职责:

| 包 | 职责 |
|----|------|
| `api.annotation.*` | `@Agent` / `@Action` / `@Goal` / `@Condition` / `@AchievesGoal` / `@RequireNameMatch` / `@EmbabelComponent` / `@State` / `@LlmTool` / `@Provided` / `@MatryoshkaTools` / `@AgentCapabilities` |
| `api.annotation.support` | `AgentMetadataReader`(注解扫描建模)、`ActionMethodArgumentResolver`、`ActionMethodManager` |
| `api.dsl` | `AgentBuilder`、`agent { ... }` Kotlin DSL 入口 |
| `api.common` | `OperationContext`、`ActionContext`、`Actor`、`Transformation`、`Asyncer`、`StuckHandler`、`PromptRunnerOperations` |
| `api.common.workflow` | 工作流构建器:`ScatterGather` / `RepeatUntil` / `RepeatUntilAcceptable` / `Consensus` |
| `api.common.autonomy` | `Autonomy`(自主执行入口)、`AgentProcessExecution`、`GoalChoiceApprover`、`BindingsFormatter`、`PlanLister` |
| `api.common.streaming` | `StreamingPromptRunner`、`StreamingCapabilityDetector` |
| `api.common.thinking` | 思考过程运行器(Thinking 响应处理) |
| `api.invocation` | `AgentInvocation` / `TypedInvocation` / `SupervisorInvocation` / `ScopedInvocation` |
| `api.event` | `AgentPlatformEvent` / `AgentProcessEvent` / `AgenticEventListener` |
| `api.models` | 模型目录:`DeepSeekModels` / `GoogleGenAiModels` / `DockerLocalModels` 等静态常量 |
| `api.tool` | `Tool` / `Subagent` / 工具回调 / `SlidingWindowTransformer` |
| `api.reference` | `LlmReference`(可被 LLM 引用的外部能力) |
| `api.validation` | 验证 SPI / Guardrail(`UserInputGuardRail` / `AssistantMessageGuardRail`) |
| `api.termination` | 终止策略 `TerminationScope` / `TerminationSignal` / `TerminationSignalPolicy` |
| `core` | 核心领域模型:`Agent` / `Action` / `Goal` / `Condition` / `Blackboard` / `AgentProcess` / `ProcessContext` / `ProcessOptions` / `ActionQos` / `IoBinding` |
| `core.support` | `AbstractAgentProcess` / `SimpleAgentProcess` / `ConcurrentAgentProcess` / `InMemoryBlackboard` / `BlackboardWorldStateDeterminer` |
| `core.hitl` | Human-In-The-Loop 支持(`AwaitableResponseException` 等) |
| `core.deployment` | 部署工具 |
| `spi` | 所有可扩展点(见 `design-patterns.md` 的 SPI 章节) |
| `plan` | 规划子模块:`Planner` / `PlanningSystem` / `Plan` 泛型抽象 |
| `plan.goap.astar` | **A\* GOAP 规划器实现**(`AStarGoapPlanner`) |
| `plan.common.condition` | `ConditionWorldState` / `ConditionDetermination`(TRUE/FALSE/UNKNOWN) |

**直接依赖**:`embabel-agent-ai`、`embabel-common-core/util/textio`、Spring AI(mcp-client、chat、vector-store、retry)、Spring Boot、Jackson、Kotlin stdlib/reflect/coroutines、`a2a-java-sdk-spec`、moby-names-generator、Apache Commons Text。

### embabel-agent-common(pom 聚合)

- **embabel-agent-byok**:"Bring Your Own Key"——`ByokFactory<T>` SPI(0.4.0 泛型化,#1609),用于"验证 API Key 后返回经校验的服务"。默认针对 LLM,但现在可扩展到任何第三方服务。
- **embabel-agent-ai**:AI 抽象层——`EmbeddingService` / `TextToSpeechService` / `SpeechToTextService` / `PromptRunnerFactory` / `TokenCountEstimator`(#1536 新 SPI) / `AiModel` / `LlmOptions` / `PricingModel`。
- **embabel-agent-webmvc**:Spring Web MVC 集成,提供 Agent 暴露为 HTTP endpoint 的支持。

---

## (3) 领域层

### embabel-agent-domain(jar)

与 API 解耦的**领域类型**集合(如 `UserInput` 等),减少核心 API 的"业务味"。

### embabel-agent-code(jar)

面向代码理解 / 分析任务的领域库,集成:
- **JavaParser 3.26.2**(`javaparser-core` + `symbol-solver-core`)
- **JGit 7.0.1**
- **ClassGraph 4.8.181**

用于代码仓库级任务(如"总结某 repo"、"基于 git diff 审查"等)。

### embabel-agent-skills(jar,**0.4.0 新模块**)

实现 [agentskills.io 规范](https://agentskills.io/specification) 的 Skills 框架。核心思想:**懒加载**——系统提示中只放 skill 元数据,LLM 调用 `activate(name)` 工具时才加载完整指令。

关键包:
- `skills` — `Skill`(`LlmReference` 实现)、`LoadedSkill`
- `skills.script` — `ProcessSkillScriptExecutionEngine`、`DockerSkillScriptExecutionEngine`、`ScriptTool`、`SkillScript`
- `skills.spec` — `SkillDefinition` / `SkillMetadata`(YAML 解析)
- `skills.support` — `DirectorySkillDefinitionLoader`、`GitHubSkillDefinitionLoader`、`ClaudeFrontMatterFormatter`、`CursorFrontMatterFormatter`、`SkillValidator`

---

## (3b) RAG 子树

### embabel-agent-rag(pom)

五个子模块:

| 子模块 | 职责 |
|--------|------|
| **embabel-agent-rag-core** | 抽象:`ContentElement` / `Retrievable` / `NavigableDocument` / `ChunkingContentElementRepository` / `EmbeddingAwareChunkingContentElementRepository`(#1574) / `ContentChunker` / `ChunkTransformer` / `Ingester` / `ContentRefreshPolicy` / `ToolishRag` / `TokenBudgetRetrievableResultsFormatter` |
| **embabel-agent-rag-pipeline** | 事件驱动的 RAG 管线编排 |
| **embabel-agent-rag-lucene** | Lucene 9.11.1 后端(BM25 + 向量) |
| **embabel-agent-rag-tika** | Tika 文档解析 / 1000+ 格式 |
| **embabel-agent-rag-neo** | Neo4j 图存储(占位) |

---

## (4) 集成层

| 模块 | 形态 | 用途 |
|------|------|------|
| **embabel-agent-a2a** | jar | A2A 协议(JSON-RPC 2.0 + SSE 流):`A2ARequestHandler` / `AgentCardHandler` / `AgentSkillFactory` / `TaskStateManager` / `EmbabelServerGoalsAgentCardHandler`(从 `@Goal` 生成 AgentCard) |
| **embabel-agent-mcp**(pom) | — | 下含 `embabel-agent-mcpserver`(将 Agent 暴露为 MCP Server,通过 `McpToolExport` / `PerGoalMcpExportToolCallbackPublisher`)与 `embabel-agent-mcp-security`(Spring Security + AspectJ 切面保护 MCP 工具) |
| **embabel-agent-shell** | jar | Spring Shell REPL 入口,交互式调用 Agent |
| **embabel-agent-openai** | jar | OpenAI 兼容 API 的适配工具 |
| **embabel-agent-onnx** | jar | `OnnxModelLoader`(从 HuggingFace 下载 + 本地缓存)、`OnnxEmbeddingService`(实现 `EmbeddingService`);DJL Tokenizers(无 Python) |
| **embabel-agent-observability** | jar | Micrometer + OpenTelemetry 集成:`EmbabelMetricsEventListener`、`EmbabelFullObservationEventListener`、`EmbabelTracingObservationHandler`、`MdcPropagationEventListener`、`@Tracked` / `TrackedAspect` |

---

## (5) 自动配置层 / embabel-agent-autoconfigure

一个聚合 pom,下含两大类共 **20 个** 自动配置模块:

### 平台与基础设施

- `embabel-agent-platform-autoconfigure`(`AgentPlatformAutoConfiguration`)
- `embabel-agent-a2a-autoconfigure`
- `embabel-agent-shell-autoconfigure`
- `embabel-agent-observability-autoconfigure`
- `embabel-agent-mcpserver-autoconfigure`
- `embabel-agent-mcpserver-security-autoconfigure`
- `embabel-agent-netty-client-autoconfigure`(Netty 4.1.132.Final 强制版本,修 CVE)

### LLM 提供方(`models/` 下)

13 个提供方各一个:anthropic / openai / openai-custom / bedrock / ollama / lmstudio / dockermodels / deepseek / gemini / google-genai / minimax / mistral-ai / onnx。

每个 provider 典型包含:
- `XyzModelLoader`(基于 YAML 的模型目录加载)
- `XyzModelDefinition`(实现 `LlmAutoConfigMetadata`,含 pricing、knowledge cutoff)
- `XyzModelsConfig`(Spring `@Configuration`)
- `resources/models/xyz-models.yml`(模型目录)

---

## (6) Starter 层 / embabel-agent-starters

21 个 Spring Boot Starter POM,作用只是"一行 Maven 依赖打包所有相关 autoconfigure + 运行时依赖":

### 基础
- `embabel-agent-starter`(父)
- `embabel-agent-starter-platform`

### 特性
- `starter-shell` / `starter-observability` / `starter-mcpserver` / `starter-mcpserver-security` / `starter-a2a` / `starter-webmvc` / `starter-onnx`

### LLM 提供方
- `starter-anthropic` / `starter-openai` / `starter-openai-custom` / `starter-bedrock` / `starter-ollama` / `starter-lmstudio` / `starter-dockermodels` / `starter-deepseek` / `starter-gemini` / `starter-google-genai` / `starter-minimax` / `starter-mistral-ai`

---

## (7) 测试支持 / embabel-agent-test-support

- **embabel-agent-test**:公开测试基类(`EmbabelMockitoIntegrationTest`),依赖 `embabel-agent-api`。
- **embabel-agent-test-common**:共享工具 + MockK(Kotlin 模拟)。
- **embabel-agent-test-internal**:内部测试工具,依赖 `api` / `domain` / `test` / `test-common`。

---

## 可选模块

- **embabel-agent-docs**:文档模块,需要 `-P embabel-agent-docs` 激活,非默认构建。

---

## 模块依赖图(简化)

```
                  embabel-agent-dependencies (BOM)
                            │
      ┌─────────────────────┼──────────────────────────┐
      ↓                     ↓                          ↓
   agent-api          agent-common/*            embabel-common-*
      │                     │
      ├─→ agent-domain      ├─→ agent-byok
      ├─→ agent-code        ├─→ agent-ai ←─ agent-onnx
      ├─→ agent-skills      └─→ agent-webmvc
      ├─→ agent-a2a
      ├─→ agent-mcp/*
      ├─→ agent-shell
      ├─→ agent-openai
      ├─→ agent-observability ←─ agent-rag-pipeline
      └─→ agent-rag-core ─→ pipeline ─→ lucene / tika / neo

   所有上述 ←─ autoconfigure/* ←─ starters/*
```

## 关键洞察

1. **"api + dependencies" 承担 80% 的概念**——其他模块都是可选的集成或 provider 绑定。
2. **Provider 对称**:每一个 LLM 提供方都遵循 "provider / autoconfigure / starter / YAML 模型目录" 四件套模板,新增 provider 几乎零认知成本。
3. **Skills 是 0.4.0 的旗舰新特性**:独立模块而非 Api 子包,说明团队将"远程可分发技能"视为独立产品维度。
4. **BYOK 从 LLM 专用变为通用**:`ByokFactory<T>` 泛型化(#1609)暗示未来会有更多"需要 Key 验证"的第三方服务。
