# `vectorstores/` — Review 阅读顺序

`vectorstores/` 实现 `core/vectorstore.Store` 接口的多种后端：托管服务
(Pinecone / Qdrant / Chroma / Milvus / OpenSearch / S3Vectors / 各家云)、
关系型 + 向量扩展 (pgvector / cockroachdb / mariadb / oracle)、文档库
(MongoDB / Couchbase / Redis)、以及内存版 (inmemory)。约 20+ 后端。

## 阅读顺序

1. **先回到** `core/vectorstore/` 看接口契约 (`Add` / `Search` /
   `Delete` / `Filter` 表达式等)。
2. `internal/` 是所有后端共享的基础设施 — **先读这里**：
   - `tracing/` — OTel span 装饰。
   - `filterhelp/` — Filter 表达式 → 后端原生查询的通用工具。
   - `docio/` — Document ↔ store row 互转。
   - `ident/` — id 标识 / namespace 规范。
   - `storetest/` — 跨后端共享的测试矩阵（很有价值，复查后端用）。
3. `inmemory/` — 参考实现，验证接口契约最快。**先读这个**再去看
   外部后端。
4. 按你的部署目标挑后端：
   - `pgvector/` — Postgres + pgvector，常见 self-hosted。
   - `qdrant/` / `milvus/` — 主流向量数据库。
   - `chroma/` — 开发/原型常用。
   - `pinecone/` — 托管服务。
   - `redis/` — Redis Stack。
   - 其他按需。

## 每个后端 review 重点

- **接口完整性**：是否实现了全部 `Store` 方法？特殊 filter 是否降级
  正确？
- **批量写入**：`Add` 是否做了 batching？批大小可调否？
- **错误归一**：传输层错误 → `core/vectorstore` 错误的映射。
- **索引初始化**：第一次 Add 时建索引是否原子（避免重复建）。
- **测试**：是否走了 `internal/storetest/` 的契约矩阵？
- **OTel**：是否套了 `internal/tracing/`？

## 跨模块提醒

- 上游使用方：`rag/document_retriever_vectorstore.go`。
- Filter 表达式 → 各后端方言的差异要看 `internal/filterhelp/` 是否覆盖
  到，否则 RAG 高级查询会失效。

## 体检命令

- `go test ./vectorstores/inmemory/...` — 基础契约。
- `go test ./vectorstores/internal/storetest/...` — 共享测试矩阵。
- 后端测试通常需要真实服务，CI 矩阵跑得起就好。
