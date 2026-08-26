# CLAUDE.md — documentreaders module

> 把不同格式(markdown / HTML / PDF …)解析成统一的 `core/document.Document` 流,供 RAG 索引消费。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体解析库 / 依赖版本以各 reader 的 go.mod 为准 —— 本则只讲宏观。

---

## 定位

- **格式各异 → 统一 Document**:每个 reader 把一种格式解析成 core 的 Document,下游 RAG pipeline 只见统一形态。
- **每 reader 一个独立子包**：`documentreaders` 只是命名空间，不存在根包。纯标准库或仅依赖 Core 的轻量 reader（`text`、`json`）留在根 module；引入重解析器依赖的格式使用独立 go.mod，消费方只拉自己用到的依赖。

## 架构心智

- **统一结果**:每个 reader 公开具体 `Read(context.Context)` 方法并返回 Core `Document`;构造用显式 `Config` 表达格式专属行为(如按标题切分)。Core 不拥有无消费者的通用 Reader 接口。
- **元数据 key 带 reader 前缀**:各格式的元数据落在自己的命名空间,跨 reader 不冲突。
- **全量读进内存,不做流式**:面向小文档;大文档的分块由调用方负责。
- **结构化格式保留层级**:如标题层级构成路径,给 LLM 提供上下文定位。

## 模块特有反向不变量

- ❌ **在 `documentreaders` 根目录新增 Go 包** —— 每个格式必须有明确 owner。
- ❌ **让重依赖 reader 共用一个 go.mod** —— 独立 go.mod 隔离可选解析库；轻量 reader 不为形式上的一致额外建 module。
- ❌ **元数据 key 不带前缀** —— 会与其他 reader 撞名,下游按 key 取值会错乱。

## 改动前必看(波及面)

- **改元数据 key 命名**:下游 RAG pipeline 可能直接按 key 读,跨 reader 协调后再改。
- **加新格式**：新建独立子包并返回 Core `Document`，元数据使用格式前缀；仅标准库/Core 的轻量实现留在 `documentreaders` module，引入可选解析库时才建立叶子 module。
