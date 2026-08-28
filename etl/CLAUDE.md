# CLAUDE.md — etl module

> 把外部内容提取为 `core/document.Document`，再以显式策略完成格式化、切分、ID 分配、批处理与落地。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体解析库与依赖版本以各 module 的 go.mod 为准。

---

## 定位

- **ETL 是完整能力域，不是 reader 命名空间**：根包拥有通用 transform/load 策略；`text`、`json`、`markdown`、`html`、`pdf` 按格式拥有 extract 逻辑。
- **统一中间形态**：所有 reader 只产出 Core `Document`；RAG、索引与评估消费统一文档，不感知源格式。
- **module 只隔离真实依赖边界**：轻量 `text/json` 与共享 Reader/Splitter 责任的 `markdown` 属于基础 module；HTML、PDF 的较重可选解析器分别留在叶子 module。

## 架构心智

- **Extract**：每个 reader 公开具体 `Read(context.Context)` 方法并返回 Core `Document`；构造用显式 `Config` 表达格式专属行为。Core 不拥有无消费者的通用 Reader 接口。
- **Transform**：Formatter、Splitter、IDAssigner、TokenCountBatcher 各自拥有并校验自己的策略；不以魔法 map 或一次性 option closure 传配置。
- **Load**：`TextFileWriter` 明确表达文本文件目标；向量库写入仍由 `core/vectorstore` contract 与 `vectorstores/*` provider 负责。
- **元数据 key 带 reader 前缀**:各格式的元数据落在自己的命名空间,跨 reader 不冲突。
- **有预算地全量读进内存，不做隐式截断**：whole-source reader 面向小文档，统一通过 `SourceBudget` 设定硬上限；零值使用有界默认值，超限返回 `ErrSourceTooLarge` 且不产出部分文档。大文档必须由调用方显式切分或提高预算。
- **Markdown 单一 owner**：读取与结构感知切分共用 `etl/markdown`，不再由两个 module 重复拥有同一格式解析器。

## 模块特有反向不变量

- ❌ **为每个处理阶段发明统一胖接口** —— 接口由真实消费者拥有，具体类型直接暴露自己的丰富行为。
- ❌ **为轻量子包机械增加 go.mod** —— module 是依赖/发布边界，不是目录装饰。
- ❌ **让重依赖 reader 污染基础 module** —— HTML/PDF 等可选解析器继续作为独立叶子 module。
- ❌ **元数据 key 不带前缀** —— 会与其他 reader 撞名,下游按 key 取值会错乱。

## 改动前必看(波及面)

- **改元数据 key 命名**:下游 RAG pipeline 可能直接按 key 读,跨 reader 协调后再改。
- **加新格式**：新建独立子包并返回 Core `Document`，元数据使用格式前缀；只有依赖重量或独立发布节奏足以形成真实边界时才建立叶子 module。
