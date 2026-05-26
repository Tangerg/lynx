# `chatmemory/` — Review 阅读顺序

`chatmemory/` 是 `core/model/chat/memory.Store` 的持久化后端集合：每个
子目录实现一种存储 (Redis / Postgres / Mongo / Cassandra / Cosmos /
Neo4j)。所有实现必须满足同一份 `Store` 契约 (Read / Write / Clear)。

## 阅读顺序

1. **先回到 `core/model/chat/memory/memory.go`** 看清 `Store` 接口契约
   再来 review 实现。
2. `doc.go` — 包说明。
3. `internal/tracing/` — 公共的 OTel span 装饰，所有后端共享。
4. 各后端按你最关心的部署形态挑：
   - `redis/` — 最常用，最先读。注意 Pipeline 优化。
   - `postgres/` — 关系型，验证事务边界 + 索引。
   - `mongodb/` — 文档型；BSON 字段映射核对。
   - `cassandra/` — 写密集场景，关注 partition key 设计。
   - `cosmosdb/` — Azure 部署用，关注 RU 成本注释。
   - `neo4j/` — 图存储，关注节点/关系建模。

## 每个后端 review 重点

- **接口契约一致性**：`Read` 是否保留顺序？`Write` 是否原子？`Clear` 是
  否幂等？
- **超大对话**：long conversation 下的查询计划 / 索引。
- **错误映射**：传输层错误 → `core` 抽象错误是否一致。
- **测试**：是否有 integration test（需要 docker / 真实端点）？
- **OTel**：是否走了 `internal/tracing` 的统一 wrapper？

## 跨模块提醒

- Lyra 用的是 `lyra/internal/storage/message_store.go` 的 JSONL 文件后端，
  不在这里 — 但实现的是同一个 `Store` 接口，可以对比参考。
- 中间件层（消息加载 / 去重 / Save marker）在 `core/model/chat/memory/
  middleware.go`，不在 backend 这一层 — 这里只做 KV 存储。

## 体检命令

- `go test ./chatmemory/...` — 大部分 integration test 需要真实后端，
  可能 skip。
- `grep -l "Read\|Write\|Clear" chatmemory/*/` — 每个后端都应有这三个
  方法。
