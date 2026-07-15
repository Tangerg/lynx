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
├── agent/           planner-driven agent 运行时（库：core 原语 + runtime 引擎 + planning 策略 + Extension SPI）
└── app/
    ├── runtime/     Lyra Runtime 后端（消费 agent，JSON-RPC over HTTP+SSE）
    └── desktop/     Wails 桌面壳 + 前端（独立工作区）
```

**当前依赖方向**：外层 → core → Go 标准库。Core 生产代码只依赖标准库和 Core 自身包；OTel、MCP、provider SDK、vector DB driver 与 tokenizer 实现全部位于外挂模块。重构前依赖快照见 [`CORE_BASELINE.md`](./CORE_BASELINE.md)。

---

## 文档地图

**治理规范（全仓适用，判断"该不该这么写"的硬尺子）**
- [`../CLAUDE.md`](../CLAUDE.md) — 跨模块法则：两条最高法则（不留债 / 治本）+ 设计原则（SOLID / DRY / KISS / YAGNI）+ 共用强约定
- [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md) — 设计哲学的"为什么"：薄核 + 三形态变体 + 窄腰 + 一个扩展机制 + 库优于框架
- [`../REFACTORING.md`](../REFACTORING.md) — 落手重构的标尺（触发信号 + Fowler 式清单 + 节奏纪律）

**框架设计（本目录）**
- [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md) — 当前 API 的最小同步/流式/typed tool/tool-loop/pause-resume/structured output 上手路径
- [`CORE_ARCHITECTURE_EXECUTION_PLAN.md`](./CORE_ARCHITECTURE_EXECUTION_PLAN.md) — Core 长期架构演进的唯一执行基准：目标边界、阶段任务、验收标准、进度、风险与 ADR
- [`CORE_V1_ARCHITECTURE_REVIEW.md`](./CORE_V1_ARCHITECTURE_REVIEW.md) — Core v1 最终架构审查：冻结范围、依赖图、无兼容债审计、质量证据与发布边界
- [`CORE_V1_MIGRATION.md`](./CORE_V1_MIGRATION.md) — 从旧 Spring AI 移植形态直接切换到 Core v1 的源码、职责和历史数据迁移指南
- [`CORE_V1_RELEASE_NOTES.md`](./CORE_V1_RELEASE_NOTES.md) — Core v1.0.0 的发布内容、破坏性变化、门禁、风险与 SemVer 政策
- [`CORE_V1_RELEASE_RUNBOOK.md`](./CORE_V1_RELEASE_RUNBOOK.md) — 多 module 真实依赖 DAG、协调发布波次、tag 规则和回滚手册
- [`CORE_API_INVENTORY.md`](./CORE_API_INVENTORY.md) — Core 重构前公共 API、workspace 消费热度及 P4/P6 provider/backend 迁移子清单
- [`CORE_BASELINE.md`](./CORE_BASELINE.md) — P0 build/vet/test/lint、coverage、race 与 Core 依赖预算基线
- [`OBSERVABILITY.md`](./OBSERVABILITY.md) — 可观测性设计：OTel 三驾马车 → `log/slog`、语义规范、埋点清单、桥接 exporter

**各模块上下文**：每个 sub-module 自带 `CLAUDE.md`（形态 / 关键类型 / 模块特有反向不变量）。
- agent 框架（库）→ [`../agent/docs/`](../agent/docs/)：当前 `GUIDE` 与 `EXTENSION_DESIGN`（SPI）
- Lyra Runtime（应用）→ [`../app/runtime/doc/`](../app/runtime/doc/)：`EXECUTION_CENTERED_ARCHITECTURE`（唯一架构基准）+ `EXTENSIBILITY` + 已完成历史计划
- 桌面前端 → [`../app/desktop/`](../app/desktop/)：`CLAUDE.md` / `frontend/DESIGN.md`（视觉规范）/ `docs/protocol/`（Lyra Runtime Protocol 契约：API / TRANSPORT / AUX_API）
