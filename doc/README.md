# Lynx 文档

> Go 语言的 LLM / agent / RAG 基础设施，对标 Spring AI / langchain4j，但坚持 Go 风格、克制设计。

---

## 模块拓扑

```
lynx/
├── core/            稳定协议 + 最小 SPI（metadata / media / document / modalities / vectorstore）
├── pkg/             通用工具库（collections / encoding / sync / retry / ptr ...）
├── models/          LLM provider 适配器（anthropic / openai / google / 兼容端点）
├── vectorstores/    向量库适配器（qdrant / milvus / pinecone / weaviate / chroma ...）
├── tools/           Tool 实现
├── chatclient/      高层 Chat 调用便利层
├── embeddingclient/ 向量提取便利层（文本/Document → 独立向量）
├── documentpipeline/文档 formatter / transformer / batcher / ID
├── tokenizer/       tokenizer SPI 与 tiktoken 实现
├── mcp/             Model Context Protocol 桥接
├── a2a/             Agent-to-Agent 协议桥接
├── chathistory/      聊天历史后端
├── documentreaders/ 文档读取器（html / markdown / pdf）
├── skills/          Agent Skills 基础能力（只读 SKILL.md 仓）
├── otel/            Core/运行时 OTel wrapper + 开发导出器
├── agent/           planner-driven Agent Framework（Engine 生命周期 + planning + tool-loop + Extension SPI）
└── app/
    ├── runtime/     Lyra Runtime 后端（消费 agent，JSON-RPC over HTTP+SSE）
    └── desktop/     Wails 桌面壳 + 前端（独立工作区）
```

**当前依赖方向**：外层 → core → Go 标准库。Core 生产代码只依赖标准库和 Core 自身包；OTel、MCP、provider SDK、vector DB driver 与 tokenizer 实现全部位于外挂模块。

---

## 文档地图

**治理规范（全仓适用，判断"该不该这么写"的硬尺子）**
- [`../CLAUDE.md`](../CLAUDE.md) — 跨模块法则：两条最高法则（不留债 / 治本）+ 设计原则（SOLID / DRY / KISS / YAGNI）+ 共用强约定
- [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md) — 设计哲学的"为什么"：薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 基础能力优先库化 + 生命周期框架显式化
- [`../REFACTORING.md`](../REFACTORING.md) — 落手重构的标尺（触发信号 + Fowler 式清单 + 节奏纪律）

**移植对照（本目录）**
- [`CORE_VS_SPRING_AI.md`](./CORE_VS_SPRING_AI.md) — `core` 对 Spring AI（`spring-ai-model`/`commons`/`client-chat`/`vector-store`）的逐块对照：泛型骨架/builder/Advisor 链/ANTLR/retry 分类 → tagged-value/值语义/`iter.Seq2`/能力对称 middleware/手写 scanner/边界 `Validate`；收敛处与分歧处及**为什么**
- [`AGENT_VS_EMBABEL.md`](./AGENT_VS_EMBABEL.md) — `agent` 对 Embabel 的逐块对照：GOAP 规划主干收敛，工程形态全盘重决策（注解扫描/Spring 容器/SpEL/ThreadLocal → 显式装配/类型分发 Extension/Go 泛型/`context.Context`/外科手术式 resume），并校准双方 planner、多态 dataflow 与并发能力差异

**框架设计（本目录）**
- [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md) — 当前 API 的最小同步/流式/typed tool/tool-loop/pause-resume/structured output 上手路径
- [`AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](./AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md) — Agent Framework 的唯一执行基准：Engine 所有权、Deployment、managed interaction、durable Process、阶段任务与进度
- [`OBSERVABILITY.md`](./OBSERVABILITY.md) — 可观测性设计：OTel 三驾马车 → `log/slog`、语义规范、埋点清单、桥接 exporter

**各模块上下文**：每个 sub-module 自带 `CLAUDE.md`（形态 / 关键类型 / 模块特有反向不变量）。
- Agent Framework → [`../agent/docs/`](../agent/docs/)：当前 `GUIDE` 与 `EXTENSION_DESIGN`（SPI）；本目录另有 Agent Framework 的执行基准计划
- Lyra Runtime（应用）→ [`../app/runtime/doc/`](../app/runtime/doc/)：`EXECUTION_CENTERED_ARCHITECTURE`（唯一架构基准）+ `EXTENSIBILITY`
- 桌面前端 → [`../app/desktop/`](../app/desktop/)：`CLAUDE.md` / `frontend/DESIGN.md`（视觉规范）/ `docs/protocol/`（Lyra Runtime Protocol 契约：API / TRANSPORT / AUX_API）
