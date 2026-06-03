# `vectorstores/` 命名 review

扫完 20+ 个后端 (inmemory / pgvector / qdrant / chroma / milvus /
pinecone / weaviate / redis / mongo / elasticsearch / opensearch /
s3vectors / pgcrdb / cassandra / oracle / mariadb / supabase /
azureaisearch / azurecosmos / bedrockkb / couchbase / clickhouse 等)。

**结论：大体干净**。横向命名一致，少量后端有特殊枚举类型。

---

## 1. `Store` / `StoreConfig` / `Visitor` 命名横向一致 ✅

```go
// vectorstores/<backend>/store.go — 每个后端都是
type Store       struct { ... }
type StoreConfig struct { ... }
type Visitor     struct { ... }   // filter AST 访问器，按需有
```

**评价**：与 `chatmemory/` 同设计，跨 20+ 后端零差异，行业级一致性 ✓

---

## 2. `DistanceMetric` / `IndexType` string 枚举 ✅

`vectorstores/qdrant/store.go:21` / `vectorstores/milvus/store.go:42`

```go
type DistanceMetric string
const (
    DistanceCosine DistanceMetric = "Cosine"
    ...
)
type IndexType string  // milvus 特有
```

**评价**：string typedef + 前缀化常量，Go 习惯 ✓

---

## 3. `Similarity` 函数类型 ✅

`vectorstores/inmemory/inmemory.go:10`

```go
type Similarity func(a, b []float64) float64
```

**评价**：函数类型直接命名（不必加 `Func` 后缀，单字就够），符合
`http.HandlerFunc` 类风格 ✓

---

## 4. SDK 透传的 `Id` 字段（不是 lynx 命名）⚠️ 不算问题

`vectorstores/pinecone/store.go:117` / `vectorstores/qdrant/store.go:197`

```go
// 直接给 Pinecone / Qdrant SDK 的字段
&pineconegrpc.Vector{ Id: uuid.NewString(), ... }
qdrant.NewID(id)
```

**说明**：Pinecone Go SDK 用 `Id`（小写 d），Qdrant Go SDK 也是
`Id` — 这是**外部 SDK 字段命名**，lynx 这边没法控。**不算 lynx 的
命名问题**。

仅在赋值时不可避免地写 `Id:`，无需调整。

---

## 5. `internal/` 共享层 ✅

`vectorstores/internal/`：
- `docio/` — document ↔ store row 互转
- `filterhelp/` — Filter 表达式 → 后端原生查询
- `ident/` — id / namespace 规范
- `storetest/` — 跨后端共享测试矩阵
- `tracing/` — OTel 装饰

**评价**：各子包命名清晰，专职单一 ✓

---

## 不动 / 已经 OK 的

- 横向 20+ 后端命名完全统一
- 零 Get/Set / ToString
- 零 stutter（包名 = 后端名，类型 `Store` 不口吃）
- 零 Java suffix
- 所有数据载体 (`StoreConfig`、`Visitor`) public 字段

---

## 优先级建议

**无 P0~P2 调整项**。本包是 lynx 仓库**命名最大规模一致性范例**。

---

## 体检命令

- `go test ./vectorstores/inmemory/...` — 接口契约
- `grep -rE "^type (Store|StoreConfig)\b" vectorstores/*/` —
  应每个后端都各有一个，无遗漏
