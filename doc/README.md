# Lynx 文档

> Go 语言的 LLM 框架，对标 Spring AI / langchain4j，但坚持 Go 风格、克制设计。
>
> **基线**：HEAD `4da6a37`（main，已含 reasoning 一等公民、MCP v1 桥接、Usage cache tokens）。

---

## 模块拓扑

```
lynx/
├── core/          抽象 + 协议（chat / embedding / image / audio / moderation / RAG / vectorstore 接口 / document pipeline / tokenizer）
├── pkg/           通用工具库（23 个子包：collections / encoding / sync / retry / ptr ...）
├── models/        Provider 适配器（anthropic / openai / google）
├── vectorstores/  向量库适配器（qdrant / milvus / pinecone / weaviate / chroma）
├── tools/         Tool 实现（fakeweatherquery 示例）
├── mcp/           Model Context Protocol 桥接（v1，单包）
└── otelbridge/    OpenTelemetry SpanExporter（slog / stdlib log 两种）
```

**依赖方向**：外层 → core → pkg。重依赖（OTel SDK、MCP SDK、provider SDK、vector DB driver）全部走外挂模块，core 保持最小。

---

## 文档索引

### 顶层文档

| 文档 | 主题 |
|-----|------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | 系统架构、核心抽象、分层、可扩展性、已知技术债 |
| [MCP.md](./MCP.md) | MCP 桥接设计与实现、与 Spring AI MCP 的对比 |
| [REASONING.md](./REASONING.md) | Reasoning 一等公民设计、provider 映射、多轮回放 |
| [MIDDLEWARE.md](./MIDDLEWARE.md) | 中间件框架现状、Call/Stream 双写痛点、AroundMiddleware 提案 |
| [OBSERVABILITY.md](./OBSERVABILITY.md) | 可观测性设计（直接用 OTel）、语义规范、埋点清单、桥接 exporter |
| [SPRING_AI_COMPARISON.md](./SPRING_AI_COMPARISON.md) | 与 Spring AI 的多角度对比（哲学 / 范式 / API / 生态 / 中间件 / 子系统深入 / 集成 / 可观测 / 战略 gap） |

### 评审与排查报告

| 文档 | 主题 |
|-----|------|
| [bug-analysis-lyra-agent.md](./bug-analysis-lyra-agent.md) | lyra + agent 两模块全代码 bug 深度排查报告（排除 pkg/） |
| [design-patterns-analysis.md](./design-patterns-analysis.md) | lynx 仓 13 个代码模块的设计模式调研分析（排除 pkg/） |

### 子目录

| 目录 | 主题 |
|-----|------|
| [agent/](./agent/) | Agent 框架设计文档（embabel-agent 的 Go 移植；运行时已落地在 `/agent/`，含 HTN / Reactive / GOAP 三套 planner） |
| [embabel-architecture-analysis/](./embabel-architecture-analysis/) | 上游 embabel-agent 框架的架构分析（参考材料） |

---

## 阅读建议

- **想了解整体设计** → `ARCHITECTURE.md`
- **要接 MCP server / 做 agentic 工具** → `MCP.md`
- **关心 reasoning / thinking 模型支持** → `REASONING.md`
- **想写自定义中间件** → `MIDDLEWARE.md`
- **要做生产可观测性** → `OBSERVABILITY.md`
- **评估与 Spring AI 的对比 / gap** → `SPRING_AI_COMPARISON.md`
- **关心 agent 框架** → `agent/README.md`（设计文档）+ `/agent/`（运行时实现）
