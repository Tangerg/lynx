# CLAUDE.md — chatmemory module

> Persistent `chat.Message` history backends — Postgres / Redis / Mongo / Cassandra / Neo4j / Cosmos DB.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

`core/model/chat/memory.Store` 的多 DB 后端实现。`lyra/internal/storage/FileMessageStore` 走文件 / SQLite，**这个模块给真上 production 的 chat history 提供后端选择**。

## 技术栈

- Go 1.26.4
- DB clients：`jackc/pgx` v5（Postgres）、`redis/go-redis` v9、`mongo-driver` v2、`gocql`（Cassandra）、`neo4j-go-driver` v5、`azcosmos`
- 依赖 `core/model/chat/memory`（Store interface）+ `core/model/chat`（Message 类型 + 序列化）
- ~1.6k LOC，多个 backend 平行实现

## 核心架构

- **`postgres/store.go`** —— pgx pool + JSONB schema + 自动 schema init + conversation_id 索引（**主推 backend**）
- **`{redis,mongodb,cassandra,neo4j,cosmosdb}/`** —— 各自实现同一 `memory.Store` 接口（Write/Read/Clear）
- **`internal/tracing/`** —— OTel span helpers，Read/Write/Clear 各打一个 span

## 关键接口/类型

- `memory.Store`（来自 `core/model/chat/memory`）—— `Write(ctx, convID, msgs...) error` / `Read(ctx, convID) ([]Message, error)` / `Clear(ctx, convID) error`
- 每 backend 一个 `StoreConfig`（DB client / table 名 / 可选选项）+ `NewStore(config)` 工厂

## 强约定

- **canonical JSON envelope**：所有 backend 走 `chat.Message` 的 `MarshalJSON` / `chat.UnmarshalMessage` —— 换 backend 数据可跨迁移
- **按 conversation_id 分区**：每个会话独立查询路径，避免跨会话扫表
- **SQL identifier 验证**：自定义 table 名时必须走 identifier validation，防 SQL injection（pgx 用 quote_ident，cassandra 等手写）
- **顺序保证**：用 global seq 字段或 list append（Redis RPUSH）；不依赖 timestamp 排序（高并发会乱）
- **schema 自动初始化是开关**：`InitializeSchema bool` config 字段；production 通常预先 migrate，关闭自动建表

## 关键目录

```
chatmemory/
├── postgres/        BIGSERIAL + JSONB（首选 backend，schema 完整）
├── redis/           RPUSH / LRANGE per-conversation list
├── mongodb/         document-per-message + index on conv_id
├── cassandra/       partition key = conv_id
├── neo4j/           planned/early
├── cosmosdb/        planned/early
└── internal/tracing/  Read/Write/Clear OTel wrappers
```

## 常用命令

```bash
go build ./...
go test ./postgres/...   # 各 backend 独立测，可能需要 docker-compose
```

## 修改任何东西之前

- **改 `chat.Message` 序列化**：所有 backend 同步更新 —— 它们都靠 canonical JSON envelope
- **加新 backend**：实现 `memory.Store`，用 conversation_id 分区，序列化走 `chat.Message` JSON
- **schema 变更**：写 migration（不在本模块，本模块约定 schema 由调用方 migrate）
- **不要做跨 backend 数据迁移工具** —— 这是 ops 的事，不是 SDK 的事
