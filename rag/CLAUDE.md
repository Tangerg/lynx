# CLAUDE.md — rag module

> 小接口 + 组合函数的 RAG 基础库。同一 RAG 域的 contracts、vectorstore adapter、LLM-backed transforms、chat middleware 先放在根包。
> 项目级约定见 `../CLAUDE.md`。

## 一句话定位

`rag` 不提供固定 Pipeline。调用方用 `Retriever` 作为窄腰，通过 `WithTransformers`、`WithExpander`、`WithRefiners` 显式组合需要的能力；具体 adapter 也在根包内以具体名字暴露。

## 技术栈

- Go 1.26.4
- `core/document`
- `core/model/chat`
- `core/vectorstore`
- `go.opentelemetry.io/otel` 1.43

## 核心架构

- contracts：`Transformer`、`Expander`、`Retriever`、`Refiner`、`Augmenter`
- 组合函数：`Retrieve`、`Multi`、`WithTransformers`、`WithExpander`、`WithRefiners`
- vectorstore adapter：`VectorStoreConfig`、`NewVectorStoreRetriever`
- LLM-backed transforms：`NewRewriteTransformer`、`NewCompressionTransformer`、`NewTranslationTransformer`、`NewMultiQueryExpander`
- augmentation / chat：`NewContextualAugmenter`、`NewMiddleware`

## 强约定

- **单包优先**：不要把 `rag/vectorstore`、`rag/llm`、`rag/ragchat` 拆回来；根包用具体类型名表达职责。
- **不恢复 PipelineConfig/Pipeline**：组合用 Go 函数完成，不用框架式配置。
- **只有 fan-out retrieval 并行**：`Multi` 和 `WithExpander` 并发收集；transform/refine 顺序明确。
- **不做 QueryRouter/DocumentJoiner 阶段**：路由写自定义 `Retriever`，合并写 `Refiner`。
- **Query.Extra 是 per-call metadata**：跨组件传 filter/history/tenant 等上下文。

## 常用命令

```bash
go build ./...
go test ./...
```

## 修改任何东西之前

- **新增能力优先问：是否属于 RAG 域？** 属于就先放根包，除非它是明显独立的底层通用库。
- **不要新增大 Config/Builder**：小接口 + 函数组合优先。
- **新增 concrete adapter 用普通 struct + 具体构造名**，例如 `NewXRetriever(XConfig)`；只有真实可选项才进 Config。
