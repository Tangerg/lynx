# `rag/` — Review 阅读顺序

`rag/` 是检索增强生成的可组合流水线。每个组件 (Query / Retriever /
Refiner / Augmenter) 都是接口 + 多份实现，最后由 `pipeline` 串起来。

## 阅读顺序

1. `doc.go` — 包说明，先看。
2. `interface.go` **[精读]** — 整套抽象的总入口：
   `Query` / `QueryTransformer` / `QueryAugmenter` / `QueryExpander` /
   `DocumentRetriever` / `DocumentRefiner`。
3. `query.go` — `Query` 数据结构。
4. `pipeline.go` **[精读]** — 把各组件串起来的执行器。
5. `pipeline_middleware.go` — middleware 钩子。
6. `nop.go` — 占位实现，看接口的最小满足形态。
7. `tracing.go` — OTel span 装饰。

## Query 处理（按使用频率排）

8. `query_transformer_rewrite.go` — LLM 改写。
9. `query_transformer_compression.go` — 压缩。
10. `query_transformer_translation.go` — 翻译。
11. `query_expander_multi.go` — 一问拆多查。
12. `query_augmenter_contextual.go` — 上下文注入。

## Retriever（当前只有一个内置实现）

13. `document_retriever_vectorstore.go` — 从 `vectorstores` 查。

## Refiner（后处理）

14. `document_refiner_deduplication.go` — 去重。
15. `document_refiner_rank.go` — 重排序。

## 关注点

- **接口分层**：每个组件接口应**只做一件事**，不要在 Refiner 里偷做
  Retriever 的事。
- **middleware**：`pipeline_middleware.go` 是 trace / metric / 缓存的
  挂载点。
- **错误传播**：单步失败是否阻塞整管线？还是降级？
- **测试**：`llm_components_test.go` / `refiners_test.go` /
  `retriever_vectorstore_test.go` 是否覆盖每个变换的核心 case？

## 跨模块提醒

- 下层依赖 `core/document` + `vectorstores/` + `core/model/chat`。
- 与 embabel RAG 的差异已经对比过（参见仓库 root `/doc/`）。
- 与 Spring AI 的 RAG 也有对照（同上）。

## 体检命令

- `go test ./rag/...`
- `grep -n "func New" rag/*.go` — 每个 component 的构造器风格统一性。
