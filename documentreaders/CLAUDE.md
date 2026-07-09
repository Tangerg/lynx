# CLAUDE.md — documentreaders module

> 把不同格式(markdown / HTML / PDF …)解析成统一的 `core/document.Document` 流,供 RAG 索引消费。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体解析库 / 依赖版本以各 reader 的 go.mod 为准 —— 本则只讲宏观。

---

## 定位

- **格式各异 → 统一 Document**:每个 reader 把一种格式解析成 core 的 Document,下游 RAG pipeline 只见统一形态。
- **每 reader 一个独立子包 + 独立 go.mod**:解析器依赖重,隔离后消费方只拉自己用到的格式。

## 架构心智

- **统一契约**:每个 reader 实现 core 的 `document.Reader`;构造用 functional options 配格式专属行为(如按标题切分)。
- **元数据 key 带 reader 前缀**:各格式的元数据落在自己的命名空间,跨 reader 不冲突。
- **全量读进内存,不做流式**:面向小文档;大文档的分块由调用方负责。
- **结构化格式保留层级**:如标题层级构成路径,给 LLM 提供上下文定位。

## 模块特有反向不变量

- ❌ **让所有 reader 共用一个 go.mod** —— 独立 go.mod 是有意的,避免把重解析器依赖强加给不用它的消费方。
- ❌ **元数据 key 不带前缀** —— 会与其他 reader 撞名,下游按 key 取值会错乱。

## 改动前必看(波及面)

- **改元数据 key 命名**:下游 RAG pipeline 可能直接按 key 读,跨 reader 协调后再改。
- **加新格式**:新建独立子包 + go.mod,实现 `document.Reader`,元数据用本格式前缀。
