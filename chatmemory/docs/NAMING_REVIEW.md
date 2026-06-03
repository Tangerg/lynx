# `chatmemory/` 命名 review

扫完结论：**chatmemory 全部子包（redis/postgres/mongodb/cassandra/
cosmosdb/neo4j）命名干净**。零问题。

---

## 1. 各后端的 `Store` / `StoreConfig` ✅

每个子包：

```go
// chatmemory/redis/redis.go
type Store       struct { ... }
type StoreConfig struct { ... }

// chatmemory/postgres/postgres.go — 同上结构
// ...
```

**评价**：
- 包名 = 后端名（`redis` / `postgres` / etc.），类型 `Store` —
  无口吃 ✓
- `StoreConfig` 配套 — 数据载体直接 public 字段 ✓
- 各后端命名统一一致 ✓

外部读：`redis.Store` / `postgres.Store` — 标准 Go 风格。

---

## 2. `internal/tracing/` 装饰器 ✅

各后端共享的 OTel 包装层。包名 `tracing`，类型也按角色起，无问题。

---

## 不动 / 已经 OK 的

- 命名一致（横向 6 个后端结构对称）
- 零 Get/Set 前缀
- 零 ToString
- 零 stutter
- 零 Java suffix（Manager / Helper / Util）
- 数据载体全 public 字段

---

## 优先级建议

**无任何 P0~P2 项目**。本包是 lynx 仓库**命名最干净的子模块之一**。

---

## 体检命令

- `go test ./chatmemory/...`
- `grep -rnE "^type [A-Z]" chatmemory/*/` —  应只看到 `Store` 和
  `StoreConfig` 重复
