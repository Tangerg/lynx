# Lynx 文档

> Go 语言的 LLM、agent 与 RAG 基础设施。仓库按稳定基础、可组合能力和外部集成分区；目录、module 与发布语义保持一致。

## 仓库拓扑

仓库根目录只承载 `go.work`、治理文档与跨模块开发工具，不发布空的根 module。

```text
lynx/
├── core/                    stdlib-only 基础 module
│   ├── chat, embedding ...  多模态协议与最小 Model SPI
│   ├── chatclient/          Chat 调用、流式累积与 typed output
│   ├── embeddingclient/     文本/Document 到向量的便利层
│   ├── tool/                Tool、typed function、schema 与 Registry
│   ├── tokenizer/           tokenizer 能力契约
│   ├── chathistory/         history contract、middleware 与内存实现
│   └── vectorstore/         store/filter 契约、测试套件与内存实现
├── agent, a2a, mcp, rag/    可组合的上层能力 module
├── tools/                   具体工具与 webfetch/websearch 中立 SPI
├── documentpipeline/        文档变换、切分、批处理与 ID
├── documentreaders/         轻量 reader 基础 module；重依赖 reader 独立 module
├── skills, otel/            Skills 基础能力与 OTel adapter
├── models/
│   ├── protocol/            可复用 OpenAI/Anthropic wire module
│   └── <provider>/          每个模型 provider 一个叶子 module
├── vectorstores/<provider>/ 每个外部向量库一个叶子 module
├── chathistory/<provider>/  每个持久化后端一个叶子 module
├── tokenizers/tiktoken/     tokenizer 具体实现 module
├── examples/                最外层示例消费者
├── dev/                     架构守卫与跨 provider conformance
└── app/                     待迁出的消费应用，不参与库模块分层
```

依赖只允许由外向内：示例/开发工具 → provider/能力模块 → protocol/core → 标准库。`models`、`vectorstores`、`chathistory` 等复数目录是 provider 命名空间，不是聚合 module。模块边界表达真实的依赖与发布单元；同一依赖预算、同一生命周期的基础能力通过 Core 内的 package 边界保持语义清晰，不为形式一致额外拆 module。

## 文档地图

治理规范：

- [`../CLAUDE.md`](../CLAUDE.md) — 跨模块法则、依赖方向与重构纪律
- [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md) — 薄核、窄腰、扩展机制与生命周期设计
- [`../REFACTORING.md`](../REFACTORING.md) — 代码异味、抽象裁决与验证标准

Core 与迁移：

- [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md) — 同步/流式 Chat、typed Tool、structured output 与 Agent Interaction
- [`CORE_VS_SPRING_AI.md`](./CORE_VS_SPRING_AI.md) — Core 与 Spring AI 的设计对照
- [`CORE_CHAT_PROVIDER_MAPPING.md`](./CORE_CHAT_PROVIDER_MAPPING.md) — Chat 协议到 provider wire 的映射
- [`CHAT_OUTPUT_FORMAT_MIGRATION.md`](./CHAT_OUTPUT_FORMAT_MIGRATION.md) — `OutputFormat` 原生能力与回退规则
- [`TOKENIZER_MODULE_MIGRATION.md`](./TOKENIZER_MODULE_MIGRATION.md) — tokenizer contract 与实现的新边界
- [`VECTORSTORE_MIGRATION.md`](./VECTORSTORE_MIGRATION.md) — VectorStore 能力与 filter 模型迁移
- [`GO127_METHOD_GENERICS_MIGRATION.md`](./GO127_METHOD_GENERICS_MIGRATION.md) — Go 1.27 方法泛型的 owner 边界
- [`OBSERVABILITY.md`](./OBSERVABILITY.md) — OTel 三信号与装饰边界

领域文档：

- [`../agent/doc/`](../agent/doc/) — Agent Framework 架构、ADR 与能力台账
- [`../app/runtime/doc/`](../app/runtime/doc/) — 当前消费应用的 Runtime 文档；应用迁仓前保持独立维护
- 每个 module family 的 `CLAUDE.md` — 该区域的职责、依赖预算与反向不变量
