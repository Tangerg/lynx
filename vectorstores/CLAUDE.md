# CLAUDE.md — vectorstores module

> 27 个 vector database 后端的统一适配层 —— pgvector / Qdrant / Weaviate / Pinecone / Chroma / Milvus / Elasticsearch / MongoDB / Neo4j / Redis / Oracle / MariaDB ...
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

`core/vectorstore.Store` 接口的 27 个具体实现。每后端用 **Visitor 模式**把 lynx 统一的 filter AST 编译成本地查询方言。新加 backend = 实现 Store 接口 + 写一个 ast.Visitor。

## 技术栈

- Go 1.26.3
- 各 DB client 直接依赖（按需）：
  - `pgvector/pgvector-go`（PostgreSQL）/ `qdrant/go-client` / `weaviate/weaviate-go-client` / `pinecone-io/go-pinecone`
  - `chroma-core/chroma-go` / `milvus-io/milvus-sdk-go` / `elastic/go-elasticsearch` / `opensearch-project/opensearch-go`
  - `mongo-driver` / `neo4j-go-driver` / `redis/go-redis` / `gocql`（Cassandra）/ ClickHouse / Couchbase
  - AWS SDK（Bedrock KB / S3 Vectors）/ Azure SDK（Cosmos / AI Search）
- ~21k LOC / 98 文件 / 35 子目录

## 核心架构

- **`core/vectorstore/`**（不在本模块，在 `core/`）—— `Store` interface（Creator + Retriever + Deleter + Metadata）、`RetrievalRequest` / `CreateRequest` / `DeleteRequest`、filter mini-language（lexer → parser → AST）
- **`vectorstores/<backend>/`** —— 每后端固定结构：
  - `store.go`（实现 Store interface）
  - `visitor.go`（实现 `ast.Visitor`，AST → 后端查询方言）
  - `doc.go`（包文档）
  - `errors.go`（可选）
- **`internal/`** —— 共享工具：
  - `docio` —— 文档 ID 生成 / 元数据序列化
  - `filterhelp` —— AST 节点解析工具
  - `tracing` —— OTel 钩子
  - `storetest` —— visitor 符合性测试套件（共享 fixture）
  - `ident` —— SQL identifier 验证

## 关键接口/类型

1. **`vectorstore.Store`** —— `Creator + Retriever + Deleter + Metadata()`
2. **`document.Document`** —— ID / Score / Text / Media / Metadata / Formatter
3. **`vectorstore.RetrievalRequest`** —— Query + TopK + MinScore + Filter ast.Expr，链式构造
4. **`vectorstore.CreateRequest` / `DeleteRequest`** —— 各自带 Validate
5. **`ast.Visitor`** —— AST 遍历接口，每后端实现，编译为本地条件表达式
6. **`embedding.Model`** —— text → []float32，调用方注入（框架不提供）
7. **`document.Batcher`** —— 文档批处理器（embedding 优化）

## 强约定

- **Config struct 固定形状**：`StoreConfig{Context, Client (必), EmbeddingModel, DocumentBatcher (必), InitializeSchema bool (可选)}` + `Validate()`
- **AST 过滤通过 visitor 转方言**：pgvector → `metadata->>'key'` JSONB；Chroma → 扁平字典；ES → 嵌套路径
- **向量编码因 DB 而异**：SQL 系（pgvector / MariaDB / Oracle / Cassandra）用文本 `[v1,v2,...]`；gRPC（Qdrant / Milvus）用二进制；REST（Chroma / ES）用 JSON 数组
- **元数据序列化**：JSON marshal + `nullSentinel`（pgvector `null` / 其他 `{}`），`docio` 统一工具
- **ID 生成**：缺省 UUID（`docio.EnsureID`），提供则原样
- **Visitor 错误**：`Result()` / `Error()` 对，AST 遍历累积到 `visitor.err`，最后一并返
- **Distance metric 归一化**：pgvector cosine `[0, 2]` 内部转 `[0, 1]`；ES / Chroma 原生 `[0, 1]`；统一靠 `pkg/math`

## 关键目录 / 成熟度

| Backend | DB | 状态 | 备注 |
|---|---|---|---|
| **pgvector** / **cockroachdb** / **tidb** / **oracle** / **mariadb** | Postgres + vec / CRDB / TiDB / Oracle DB / MariaDB | ✅ production | 各 SQL 方言 + VECTOR 类型 |
| **qdrant** / **pinecone** / **weaviate** / **chroma** / **milvus** | 专用 vector DB | ✅ production | 各自原生 gRPC / REST |
| **elasticsearch** / **opensearch** | 搜索引擎 + dense_vector | ✅ production | knn 查询 + nested metadata |
| **mongodb** | Atlas Vector Search | ✅ production | 必须 Atlas |
| **redis** | Redis Stack 7 | ✅ production | Hash + Hashes index |
| **cassandra** | Cassandra 5 SAI | ✅ production | Storage Attached Index |
| **couchbase** | Couchbase 7.6 Search | ✅ production | SQL++ SEARCH() k-NN |
| **neo4j** | Neo4j 5.13 | ✅ production | VECTOR INDEX on nodes |
| **inmemory** | map + RLock | ✅ production | 测试 / demo 用 |
| **azureaisearch** / **azurecosmos** | Azure 托管 | 🔬 experimental | 需预先创 index |
| **bedrockkb** / **s3vectors** | AWS 托管 | 🔬 experimental | Bedrock KB 不支持 Create/Delete |
| **vectara** / **typesense** / **vespa** / **clickhouse** / **supabase** | 各家服务 | 🔬 experimental | API 不稳定 |

## 特殊点

- **Filter mini-language**：`author == "Alice"` / `year >= 2020` / `tag IN ("a", "b")` / `NOT (x)` / `AND` / `OR` —— 类 SQL WHERE，编译成 AST，每后端 visitor 转方言（pgvector jsonb 路径 / Chroma 字典 / Redis 自定义 query 等）
- **Batch upsert 策略**：调用方注入 `DocumentBatcher`，store 分批调 `EmbeddingModel.Embed()` 避免 token 溢出；backend 根据 API 限制再做切分（Pinecone 1000 / Weaviate 128 等）
- **向量维度协商**：`StoreConfig.Dimensions` 优先 → `EmbeddingModel.Dimensions()` → `DefaultDimensions = 1536`
- **Schema 初始化开关**：`InitializeSchema=true` 创建表 / 索引；`false` 假设已 provisioned；Bedrock / Azure AI Search 不支持 Create，返 `Unsupported`
- **Visitor 符合性套件**：`internal/storetest.VisitorConformance()` 覆盖所有过滤形状（string / number / bool / IN / LIKE / 嵌套），新 backend 在 `visitor_test.go` 注册一次免费拿到覆盖（验证遍历不报错，不验证输出相等）

## 常用命令

```bash
go build ./...
go test ./inmemory/...   # 不依赖 docker，最快
go test ./pgvector/...   # 通常需要 docker-compose
```

## 修改任何东西之前

- **改 filter AST**：所有 27 个 visitor 都受影响；先看 `internal/storetest` 跑全
- **改 `Store` 接口**：core/vectorstore 的契约 —— 改了所有 backend + 所有调用方（如 rag）受影响
- **加新 backend**：复制 `inmemory/` 当模板，实现 `Store` + `Visitor`，注册到 `storetest.VisitorConformance()`
- **改向量编码 / metric**：单 backend 内部细节，但要更新 `Validate()` 拒收无效输入

## 强反向不变量

- ❌ **跨 backend 数据迁移工具**：不是 SDK 职责，给调用方 / ops
- ❌ **filter AST 加业务概念节点**（如 "session_id"）：AST 是通用 filter，业务字段走 metadata
- ❌ **vector 维度自适应**：维度协商靠 Config / EmbeddingModel，**不在 store 端 reshape**
- ❌ **改后端 schema 不告 caller**：`InitializeSchema` 关掉时 store 假设 schema 已有；不要静默 ALTER TABLE
