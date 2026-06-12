# CLAUDE.md — rag module

> 5-stage RAG pipeline：query transform → expand → retrieve（并行）→ refine → augment.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

可组装的 RAG 管线：每一阶段一个接口，可换实现可串可并。**有意不做** Router / Joiner 阶段——路由放到自定义 Retriever 里，合并由 dedup + rank 的 Refiner 处理。

## 技术栈

- Go 1.26.4
- `go.opentelemetry.io/otel` 1.43（每阶段 span）
- 依赖 `core/document`（Document 类型）+ `pkg`（工具）+ 由调用方注入 vectorstore + embedding model

## 核心架构

5 个阶段接口（`stages.go`）顺序串成 pipeline：

1. **QueryTransformer** —— rewrite / compression / translation（链式按 config 顺序跑）
2. **QueryExpander** —— 多 query（一个变多个），下游 fan-out
3. **DocumentRetriever** —— **并行**跑多个 retriever（`sync.WaitGroup`，不连坐取消——部分失败保留部分结果），结果 union
4. **DocumentRefiner** —— union 后的 dedup（by ID）+ rank（top-K by score）
5. **QueryAugmenter** —— 把检索到的 docs 当 system context 注入回 query

`pipeline.go` 做装配 + 错误包装；`pipeline_middleware.go` 把整套 pipeline 包成 chat middleware 给 LLM 调用前自动注入上下文。

## 关键接口/类型

- 5 个阶段接口（在 `stages.go`），各有 `Nop` 空实现（`nop.go`）方便部分使用
- `Query{Text, Extra map[string]any}` —— Extra 元数据跟着流过所有阶段
- `PipelineConfig` —— validators + `ApplyDefaults`；`At least one DocumentRetriever required`
- `Pipeline.Execute(ctx, query)` → `(augmentedQuery, refinedDocs, error)`

## 强约定

- **至少一个 Retriever 必填**（其他阶段都可空）
- **只有 Retrieve 阶段并行**（WaitGroup fan-out，部分失败容忍），其他阶段顺序
- **Query.Extra 流过所有阶段**（不丢，可写 / 可读）—— 跨阶段传 metadata 用这个
- **每阶段一个 OTel span**，错误包装 `fmt.Errorf("stage X: %w", err)` 给上层定位
- **不做的事**（设计决策）：
  - ❌ QueryRouter 阶段 —— 路由放调用方实现 Retriever 时做
  - ❌ DocumentJoiner 阶段 —— Refiner 的 dedup + rank 已经覆盖

## 关键目录

```
rag/
├── stages.go                          5 个阶段接口
├── pipeline.go                        装配 + 顺序 / 并行 + 错误处理
├── pipeline_middleware.go             pipeline → chat.Middleware
├── query.go                           Query{Text, Extra}
├── nop.go                             各阶段 no-op 默认
├── query_transformer_*.go             rewrite / compression / translation
├── query_expander_multi.go            一个 query → N 个 query
├── document_retriever_vectorstore.go  vectorstore 后端（分数 + 过滤）
├── document_refiner_*.go              dedup（by ID）/ rank（top-K）
└── query_augmenter_contextual.go      注入 docs 当 system context
```

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **加新阶段**：先想清楚为什么不能放到现有 5 个之一。Router / Joiner 已经明确不做（理由见上）
- **改 pipeline 错误包装**：所有 downstream 都靠 `errors.Is/As` 定位失败阶段
- **加新 QueryTransformer / Expander 等**：写一个 stateless struct 实现接口；不要在阶段内部做 IO 缓存（用 `pkg/sync` 工具）
