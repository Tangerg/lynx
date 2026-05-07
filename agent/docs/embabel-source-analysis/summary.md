# 架构总结与评估

> 2026-04-20 · 0.4.0-SNAPSHOT

## 1. 全景图

```
┌────────────────────────────────────────────────────────────────────┐
│                           用户代码                                    │
│  @Agent / @Action / @Goal 注解类   · Kotlin agent { ... } DSL        │
│  HTTP(webmvc)  · Spring Shell  · A2A 远端调用  · MCP 客户端          │
└──────────────────────────────┬─────────────────────────────────────┘
                                ▼
┌────────────────────────────────────────────────────────────────────┐
│                       Autonomy 执行入口                              │
│    runAgent(input, opts, agent)   classifyAndRun(...)  openMode(...)│
└──────────────────────────────┬─────────────────────────────────────┘
                                ▼
┌────────────────────────────────────────────────────────────────────┐
│                     AgentPlatform 运行时                             │
│                                                                     │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐ │
│  │ AgentProcess     │  │ AStarGoapPlanner │  │ AgenticEvent     │ │
│  │ (OODA 循环)       │←→│ (A* + 启发式     │  │ 多播广播          │ │
│  │ Simple/Concurrent│  │  + 前后向剪枝)    │  │ + 日志 / 指标 /   │ │
│  └────────┬─────────┘  └──────────────────┘  │   OTel 追踪       │ │
│           ▼                                  └──────────────────┘ │
│  ┌──────────────────────────────────────┐                          │
│  │       Blackboard(类型化共享内存)       │                         │
│  │  · 只追加  · 类型层级匹配  · 受保护绑定  │                         │
│  │  · 三值逻辑条件存储                     │                         │
│  └──────────────────────────────────────┘                          │
└──────────────────────────────┬─────────────────────────────────────┘
                                ▼
┌────────────────────────────────────────────────────────────────────┐
│                     SPI 扩展层(全部可替换)                            │
│  PlannerFactory · BlackboardProvider · AgentProcessRepository       │
│  ContextRepository · OperationScheduler · Asyncer                   │
│  ToolGroupResolver · ToolDecorator · ToolLoopFactory                │
│  ToolInjectionStrategy · ToolNotFoundPolicy                         │
│  LlmService · LlmMessageSender · LlmMessageStreamer(新)            │
│  TokenCountEstimator(新) · ByokFactory<T>(泛型化)                 │
│  UserInputGuardRail · AssistantMessageGuardRail                     │
│  EmbeddingService · ChunkingContentElementRepository                │
│  SkillScriptExecutionEngine(新) · DirectorySkillDefinitionLoader(新)│
└──────────────────────────────┬─────────────────────────────────────┘
                                ▼
┌────────────────────────────────────────────────────────────────────┐
│                       基础设施 / 集成层                               │
│  ┌──────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌──────────────┐  │
│  │ Spring AI│ │  RAG   │ │  MCP   │ │  A2A   │ │  Observability│  │
│  │ 13 Pr.   │ │ Lucene │ │ Server │ │ JSON-RPC│ │ OTel+Micrometer│ │
│  │ (OpenAI, │ │ /Tika/ │ │ +Client│ │ +SSE    │ │ +MDC +AspectJ │  │
│  │ Anthropic│ │ Neo4j  │ │        │ │        │ │                │  │
│  │ Gemini,  │ │ 嵌入:   │ │        │ │        │ │                │  │
│  │ Bedrock, │ │ ONNX 本地│ │        │ │        │ │                │  │
│  │ ...)     │ │ /远程    │ │        │ │        │ │                │  │
│  └──────────┘ └────────┘ └────────┘ └────────┘ └──────────────┘  │
│  ┌────────────────────────────────────────────────────────────┐    │
│  │   Skills(0.4.0 新):YAML 规格 · 懒加载 · Docker/Process 脚本│    │
│  └────────────────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────────────┘
```

## 2. 0.3.5 → 0.4.0 的质变

| 维度 | 0.3.5 | 0.4.0 |
|------|--------|--------|
| Skills 能力 | 无独立模块 | **`embabel-agent-skills`** 独立模块,支持 YAML 规格 / 懒加载 / GitHub 分发 / Docker 执行 |
| 工具循环流式 | Spring AI 耦合 | `LlmMessageStreamer` 厂商中立 SPI |
| Token 计数 | 无统一入口 | `TokenCountEstimator<T>` SPI |
| BYOK | 仅 LLM | `ByokFactory<T>` 泛型,可验证任意服务 |
| 工具元数据 | 无 | `Tool.Definition.metadata: Map<String,Any>` 应用级路由 |
| 失败域控制 | 统一抛异常 | `TerminationScope`(AGENT / ACTION)控制工具缺失失败类型 |
| 成本聚合 | 忽略子进程 | `cost()` 递归累加子进程(#368/#1615) |
| SpEL 条件 | 缺失变量 → UNKNOWN | 缺失变量 → FALSE(避免规划污染) |
| `@Agent` 构造器 | 允许 `OperationContext` 注入 | 启动期直接报错,强制 action 作用域 |
| 自治绑定 | 仅绑 "it" | 同时按类型衍生名绑定(YAML action 友好) |
| 工具循环历史 | 长对话可能 400 | `SlidingWindowTransformer` 保留 tool call/result 配对 |
| Guardrail 范围 | text 响应 | 扩展到 `ThinkingResponse<*>` 与结构化对象 |
| RAG 嵌入 | 内建于分块 repo | 抽出 `EmbeddingAwareChunkingContentElementRepository` 便于单独定制 |

## 3. 架构优势

| 维度 | 评价 |
|------|------|
| **关注点分离** | 核心 / SPI / 注解 / DSL / 集成层都有独立边界,几乎没有循环依赖 |
| **可扩展性** | **30+** SPI 覆盖规划、持久化、调度、工具循环、守护、嵌入、脚本执行;默认实现全部可替换 |
| **类型安全** | 从 `IOBinding` 到 `Transformation<I, O>` 全链路强类型;Kotlin 数据类 + 密封接口让状态建模可穷举 |
| **平台中立** | **13** 个 LLM provider 并存;一套 `LlmOptions` 可移植;本地(ONNX / Ollama / LM Studio / Docker)和云端对等 |
| **Spring 原生** | 利用 `@AutoConfiguration` / `@ConditionalOnMissingBean` / `@ConfigurationProperties`,零侵入融入任何 Spring Boot 应用 |
| **测试优先** | 三层测试支持模块;Mock LLM 基类;IT 集成测试针对 parallel tool loop / guardrail;ArchUnit 自动化 |
| **双 DSL** | 两种编程模型产生**同一套**领域对象;可混合 |
| **事件驱动可观测性** | 丰富事件层次 + 多播;OTel / Micrometer / MDC / 主题日志任选 |
| **生产就绪** | 进程持久化、`ActionQos` 重试、`TerminationScope` 失败域、Netty CVE 固版、JaCoCo / SpotBugs 质量门禁 |
| **非 LLM 智能** | GOAP / 三值逻辑 / 启发式搜索都**不**依赖 LLM;LLM 只在 Action 内出现,降低成本和延迟,便于测试 |

## 4. 关键设计决策

1. **GOAP 优于静态工作流**:行动序列由规划器动态决定。环境变化、重规划、并发多路径都自然支持。
2. **黑板解耦 Action**:Action 成为"输入→输出"的变换。测试、替换、组合都简单。
3. **三值逻辑 UNKNOWN 一等公民**:规划器可以主动选择"减少不确定性"的 Action;且 0.4.0 修复缺失变量的 SpEL 语义后,规划噪声大幅降低。
4. **SPI 彻底抽象**:核心运行时不知道 Spring AI 存在——是 autoconfigure 层把 Spring AI 的 `ChatModel` 适配成 `LlmService`。这让核心可以单独演进、测试。
5. **Spring 原生而非另起炉灶**:复用 DI / autoconfigure / retry / shell / web,降低"学习一个新 Java 框架"的心智负担。
6. **action-scoped `OperationContext`**:作为 bean 属性注入会悄悄计费到错误的"占位进程";0.4.0 fail-fast 校验杜绝了这个隐蔽缺陷。
7. **成本聚合递归**:现代 Agent 常嵌套 Subagent;父进程必须对子进程的账单负责,否则成本控制失效。
8. **终止域分层**:`TerminationScope` 让"工具缺失应该停整个进程还是只停当前动作"由业务决定,而不是硬编码。

## 5. 对比同类框架

| 特性 | Embabel Agent | LangChain4j | Spring AI | LlamaIndex | LangGraph |
|------|:---:|:---:|:---:|:---:|:---:|
| GOAP / A* 规划 | ✅ | ❌ | ❌ | ❌ | 部分(DAG) |
| Spring 原生 | ✅ | ✅ | ✅ | ❌ | ❌ |
| 类型安全数据流 | ✅ | 部分 | 部分 | ❌ | 部分 |
| 双编程模型(注解 + DSL) | ✅ | ❌ | ❌ | ❌ | ❌ |
| 三值逻辑 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 内置 RAG(多后端) | ✅ | ✅ | ✅ | ✅ | ❌ |
| MCP 服务端 | ✅ | ❌ | ✅ | ❌ | ❌ |
| A2A 协议 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 自研工具循环 | ✅ | 部分 | ❌ | ❌ | ✅ |
| 子进程成本聚合 | ✅ | ❌ | ❌ | ❌ | 部分 |
| Skills 规范实现 | ✅(agentskills.io) | ❌ | ❌ | ❌ | ❌ |
| 本地嵌入 | ✅(ONNX) | ✅ | ✅ | ✅ | 依赖外部 |

## 6. 适用场景

**最适合**:
- **复杂多步骤任务**:步骤序列不能预先确定,需要动态规划
- **企业 Spring Boot 集成**:已有 Spring 基础,希望平滑加入 Agent 能力
- **多 LLM 并存 / 切换**:对比 provider、成本敏感、或合规要求不同 provider
- **生产级 Agent**:需要重试、持久化、成本统计、追踪、守护
- **代码智能 / 文档理解**:`embabel-agent-code`(JavaParser + JGit) / `embabel-agent-rag`
- **远端可分发技能**:0.4.0 的 Skills 模块正瞄准这个场景
- **多 Agent 互操作**:A2A(对外)+ MCP(对内)+ Subagent(进程内嵌套)

**暂不适合**:
- 极简单的"提示词 + 一次 LLM 调用"场景(体量过重)
- 纯 Python 生态(框架是 JVM)
- 完全静态、不需要规划的线性工作流(可以用,但不会受益于 GOAP)

## 7. 学习路径建议

```
第1步:读 core-architecture.md 理解 OODA + GOAP + Blackboard 三大核心
   ↓
第2步:实际写一个 @Agent 例子,含 2-3 个 @Action 和 1 个 @AchievesGoal
   ↓
第3步:读 execution-flow.md + execution-flow-deep.md,理解一次 runAgent 的完整控制流
   ↓
第4步:读 design-patterns.md 的 SPI 表;决定你会需要替换哪些扩展点
   ↓
第5步:查 core-files-and-classes.md 找到具体文件;阅读源码
   ↓
第6步(可选):看 testing.md,用集成基类写端到端测试验证你的 Agent 行为
```

## 8. 未来可预见的演化方向

观察到的几个信号:
- **`TokenCountEstimator`** 被标为 `@ApiStatus.Experimental`:意味着团队在为 Token 预算与"大小 prompt 自动折叠"打基础
- **`ByokFactory<T>`** 泛型化:暗示会有更多需要 Key 验证的第三方服务集成(e.g., Stripe、DataDog)
- **`Tool.Definition.metadata`**:为"按 metadata 路由不同 QoS 通道"留口子
- **Skills 作为一等公民**:技能生态 + GitHub 分发可能演变为更完整的"Agent App Store"
- **并行工具循环 + 守护测试补强**:#1589 后,并行语义的正确性已经是一个独立的 SLA

## 9. 一句话评价

**Embabel Agent 不是"又一个 LLM 链式调用框架",而是一个把"规划"视为一等公民、拥抱 Spring 生态、并在 0.4.0 把可观测性 / 失败域 / 跨进程成本 / 流式抽象 / 技能分发等"生产级"维度逐一补强的 JVM Agent 框架。** 若你在 JVM 上构建非平凡的多步骤 Agent,它的 GOAP + Blackboard + SPI 三位一体会让你比任何线性编排框架走得更远。
