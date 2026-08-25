# Lynx 文档

> Go 语言的 LLM / agent / RAG 基础设施，对标 Spring AI / langchain4j，但坚持 Go 风格、克制设计。

---

## 模块拓扑

```
lynx/
├── core/            稳定协议 + 最小 SPI（metadata / media / document / modalities / vectorstore）
├── models/          LLM provider 适配器（anthropic / openai / google / 兼容端点）
├── vectorstores/    向量库适配器（qdrant / milvus / pinecone / weaviate / chroma ...）
├── tool/            Tool 最小契约 + decorator 能力发现 + 实例 Registry
├── tools/           typed function adapter 与具体 Tool 实现
├── chatclient/      高层 Chat 调用便利层
├── embeddingclient/ 向量提取便利层（文本/Document → 独立向量）
├── documentpipeline/ 文档 formatter / transformer / batcher / ID；Markdown 结构化切分为可选子模块
├── tokenizer/       stdlib-only tokenizer SPI
│   └── tiktoken/    tiktoken 具体实现（独立模块）
├── mcp/             Model Context Protocol 桥接
├── a2a/             Agent-to-Agent 协议桥接
├── chathistory/      聊天历史后端
├── documentreaders/ 文档读取器（html / markdown / pdf）
├── skills/          Agent Skills 基础能力（只读 SKILL.md 仓）
├── otel/            Core/运行时 OTel wrapper + 开发导出器
├── agent/           可插拔策略 Agent Framework（Engine/Process + interaction/planning/workflow）
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
- Agent Framework 的 Embabel/GOAP 来源、保留与移除裁决由 [`../agent/doc/ARCHITECTURE.md`](../agent/doc/ARCHITECTURE.md) 和 [`../agent/doc/CAPABILITY_LEDGER.md`](../agent/doc/CAPABILITY_LEDGER.md) 共同记录；旧实现对比不再冒充当前合同

**框架设计（本目录）**
- [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md) — 当前 API 的最小同步/流式/typed Tool、managed Interaction 与 structured output 上手路径
- [`TOOL_FOUNDATION_MIGRATION.md`](./TOOL_FOUNDATION_MIGRATION.md) — `tools` 契约下沉到 `tool` 的 breaking migration
- [`TOKENIZER_MODULE_MIGRATION.md`](./TOKENIZER_MODULE_MIGRATION.md) — tokenizer SPI 与 tiktoken 实现的独立发布边界
- [`GO127_METHOD_GENERICS_MIGRATION.md`](./GO127_METHOD_GENERICS_MIGRATION.md) — Go 1.27 方法泛型的 owner 边界与 breaking API 迁移
- [`../agent/doc/`](../agent/doc/) — Agent Framework 当前架构、ADR、公共基线、能力台账与执行计划的唯一文档集合
- [`OBSERVABILITY.md`](./OBSERVABILITY.md) — 可观测性设计：OTel 三驾马车 → `log/slog`、语义规范、埋点清单、桥接 exporter

**各模块上下文**：每个 sub-module 自带 `CLAUDE.md`（形态 / 关键类型 / 模块特有反向不变量）。
- Agent Framework → [`../agent/doc/`](../agent/doc/)：当前架构、ADR、公共合同基线、能力裁决与执行计划
- Lyra Runtime（应用）→ [`../app/runtime/doc/`](../app/runtime/doc/)：Clean Architecture/DDD 目标架构、ADR、合同基线与实施台账
- 桌面前端 → [`../app/desktop/`](../app/desktop/)：`CLAUDE.md` / `frontend/DESIGN.md`（视觉规范）/ `docs/protocol/`（Lyra Runtime Protocol 契约：API / TRANSPORT / AUX_API）
