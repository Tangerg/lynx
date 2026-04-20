# Embabel Agent 框架深度架构分析

> 分析日期:2026-04-20
> 项目版本:0.4.0-SNAPSHOT(自 2026-03-23 分析以来,从 0.3.5-SNAPSHOT 升级)

## 目录

| 文件 | 内容 |
|------|------|
| [modules.md](./modules.md) | 模块拓扑(核心 / 领域 / 集成 / 自动配置 / Starter / 测试支持) |
| [core-architecture.md](./core-architecture.md) | 核心运行时架构:OODA 循环、GOAP、黑板、三值逻辑 |
| [core-files-and-classes.md](./core-files-and-classes.md) | 关键文件与类索引(带文件路径和行号) |
| [execution-flow.md](./execution-flow.md) | 执行流程总览:从入口到完成 |
| [execution-flow-deep.md](./execution-flow-deep.md) | 执行流程深入:tick/run/executeAction、并发、终止、重规划 |
| [design-patterns.md](./design-patterns.md) | 设计模式与扩展机制(SPI、多值逻辑、事件驱动) |
| [dependencies.md](./dependencies.md) | 外部依赖与版本策略 |
| [testing.md](./testing.md) | 测试基础设施与最佳实践 |
| [summary.md](./summary.md) | 架构总结与评估 |

## 项目定位

**Embabel Agent** 是一个基于 **Spring Boot 3 + Kotlin(含 Java)** 构建的企业级 **AI Agent 框架**。它与市面上多数"LLM 编排框架"的本质差异在于:

1. **规划而非编排**:采用游戏 AI 经典算法 **GOAP(目标导向行动规划)**,以 **A\*** 搜索在 Action-Goal 图上寻找最低代价的行动序列;行动序列由规划器在运行时动态确定,而非开发者硬编码。
2. **黑板(Blackboard)架构**:Action 之间通过类型化的共享黑板解耦,不直接依赖彼此。
3. **三值逻辑(TRUE / FALSE / UNKNOWN)**:原生支持"不确定状态",使规划器可在信息不完整时做出合理决策。
4. **双编程模型**:`@Agent` / `@Action` / `@Goal` 注解模型(面向 Spring 开发者),以及 Kotlin DSL `agent { ... }`(面向 Kotlin 原生体验);两者产生完全相同的领域模型对象。
5. **非 LLM 的"智能"**:规划、目标打分、条件求值都**不**需要 LLM;LLM 仅出现在 Action 的实现中。这让框架具备 LLM 中立性与更好的可测试性。

## 核心能力一览

| 能力 | 实现 | 0.4.0 变更 |
|------|------|-----------|
| 多 LLM 提供方 | OpenAI、Anthropic、Bedrock、Gemini、Google GenAI、Mistral、DeepSeek、MiniMax、Ollama、LM Studio、Docker Models | 定价表全量刷新(#1568) |
| 本地推理 | ONNX Runtime(嵌入 / 重排)、Ollama、LM Studio、Docker Models | — |
| RAG | Lucene、Tika、Neo4j(占位) | `EmbeddingAwareChunkingContentElementRepository` 抽取(#1574) |
| MCP | 客户端(消费)+ 服务端(暴露 `@Goal` 为 MCP 工具) | — |
| A2A(Agent-to-Agent) | JSON-RPC 2.0 + SSE 流 | — |
| Skills(新) | **embabel-agent-skills** 模块:YAML 规格 / 懒加载 / Docker / Process 脚本执行引擎 / GitHub 加载 | **全新模块** |
| 可观测性 | OpenTelemetry、Micrometer、SLF4J MDC | — |
| 工具循环 | 自研 `ToolLoop`(串行 / 并行)+ Spring AI 后备路径;`LlmMessageStreamer` 厂商中立流接口 | `LlmMessageStreamer`(#1581)、`TokenCountEstimator` SPI(#1536)、`ByokFactory<T>` 泛型化(#1609) |
| 守护(Guardrails) | `UserInputGuardRail` + `AssistantMessageGuardRail`(支持结构化对象响应与 thinking 响应) | 结构化对象守护修复(#1582)、并行工具循环验证(#1589) |
| 失败域控制 | `TerminationScope`(AGENT / ACTION)附在 `ToolGroupRequirement` 上,控制工具缺失时抛出的异常类型 | **新机制**(#1593) |
| 注解合法性校验 | `@Agent` 类构造器拒绝注入 `OperationContext`(会绑定到占位进程) | **新校验**(#1538/#1618) |
| 子进程成本聚合 | `AgentProcess.cost()` 递归累加所有子进程 | **bug 修复**(#368/#1615) |

## 如何阅读本分析

- 第一次接触:`summary.md` → `core-architecture.md` → `modules.md`
- 查找实现细节:`core-files-and-classes.md`(带文件:行号)
- 调试执行问题:`execution-flow-deep.md`
- 做二次开发 / 扩展:`design-patterns.md` 中的 SPI 章节
