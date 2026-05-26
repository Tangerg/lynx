# `rag/` 命名 review

扫完 RAG 流水线全部组件。**整体干净**，主要小问题是 `*TransformerConfig`
/ `*ExpanderConfig` / `*AugmenterConfig` 这套命名稍长。

---

## 1. `*TransformerConfig` / `*ExpanderConfig` / `*AugmenterConfig` 偏长

```go
// rag/query_transformer_compression.go
type CompressionTransformerConfig struct { ... }
type CompressionTransformer       struct { ... }

// rag/query_transformer_rewrite.go
type RewriteTransformerConfig struct { ... }
type RewriteTransformer       struct { ... }

// rag/query_transformer_translation.go
type TranslationTransformerConfig struct { ... }
type TranslationTransformer       struct { ... }

// rag/query_expander_multi.go
type MultiQueryExpanderConfig struct { ... }
type MultiQueryExpander       struct { ... }

// rag/query_augmenter_contextual.go
type ContextualAugmenterConfig struct { ... }
type ContextualAugmenter       struct { ... }
```

**问题**：从外部看
```go
rag.NewCompressionTransformer(rag.CompressionTransformerConfig{ ... })
```
读起来一长串。`Transformer` 后缀在 `New<X>Transformer` + 类型名中
都重复。

**判断**：
- 接口 `QueryTransformer` / `QueryExpander` / `QueryAugmenter`
  必须保留（rag.go 包级抽象）
- 实现类型 `CompressionTransformer` 其实可以叫 `Compression`，外部
  调用 `rag.NewCompression(...)` — 但和"压缩"这个动词混淆 ❌
- 改 `Compress` / `Rewrite` / `Translate` 这种动词命名也不行（与
  函数命名空间冲突）

**保留判定**：实际**没有更好的写法**。`*TransformerConfig` 后缀稍长
但读得清楚谁是谁的 config。⚪ 保留。

---

## 2. 接口 `Query*` 命名 ✅

`rag/interface.go`

```go
type QueryExpander    interface { ... }
type QueryTransformer interface { ... }
type QueryAugmenter   interface { ... }
type DocumentRetriever interface { ... }
type DocumentRefiner   interface { ... }
```

**评价**：领域前缀（`Query` / `Document`）+ 动作后缀（`Expander` /
`Transformer` 等），命名清楚分层 ✓

---

## 3. `rag.Pipeline` / `PipelineConfig` ✅

`rag/pipeline.go:19,71`

```go
type Pipeline       struct { ... }
type PipelineConfig struct { ... }
```

**评价**：Go 风格，零问题 ✓

---

## 4. `rag.Nop` 占位 ✅

`rag/nop.go:21`

```go
type Nop struct{}
```

**评价**：简洁占位实现，符合 Go 习惯（`testing.T` 的 nop 风格）✓

---

## 5. `rag.VectorStoreRetriever` / `VectorStoreRetrieverConfig` ✅

`rag/document_retriever_vectorstore.go:22,76`

```go
type VectorStoreRetrieverConfig struct { ... }
type VectorStoreRetriever       struct { ... }
```

**评价**：明示这是 `vectorstore` 后端的 retriever，跨包命名清晰 ✓

---

## 6. `DeduplicationRefiner` / `RankRefiner` ✅

`rag/document_refiner_deduplication.go:26` / `document_refiner_rank.go:16`

```go
type DeduplicationRefiner struct{}
type RankRefiner          struct { ... }
```

**评价**：领域 + 角色，命名一致 ✓

---

## 不动 / 已经 OK 的

- 零 Get/Set 前缀
- 零 ToString / InfoString
- 零 stutter (`rag.RagFoo` 没有)
- 零 Java suffix (Manager / Helper / Util)
- 接口分层清楚 (Query* / Document*)
- 所有 `*Config` 数据载体 public 字段 ✓

---

## 优先级建议

**无 P0~P2 调整项**。

唯一可斟酌的是 `*TransformerConfig` 偏长，但**没有更好的替代**。
保留。

---

## 体检命令

- `go test ./rag/...`
- `grep -rnE "^type (Query|Document)\w+" rag/` — 接口分层一览
