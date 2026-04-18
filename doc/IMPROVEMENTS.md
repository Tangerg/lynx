# Lynx 改进清单

> 对 `Tangerg/lynx` 多模块 Go LLM 框架（core / models / vectorstores / tools / pkg）的深度审查结果。
> 审查日期：2026-04-17
> 审查范围：约 203 个 Go 源文件，5 个子模块

---

## 总览

| 分类 | 数量 | 主要风险 |
|-----|------|---------|
| 🔴 **高优先级（阻塞性）** | 9 | 运行时 panic、测试缺失、跨模块版本漂移 |
| 🟠 **中优先级（质量）** | 18 | API 不一致、并发隐患、错误处理 |
| 🟡 **低优先级（打磨）** | 15 | 文档缺失、性能优化、风格 |
| **合计** | **42** | — |

---

## 🔴 高优先级

### H-1 跨模块版本漂移

- [ ] `models/go.mod:6` 引用 `core v0.0.0-20260413...`，`vectorstores/go.mod:6` 引用 `core v0.0.0-20260416...`，两者指向不同 commit。
  - **风险**：同时使用 models + vectorstores 的下游用户会拉到不一致的 core，公共接口演进后会编译失败。
  - **建议**：在 CI 里加一条检查强制所有子模块 `require` 的 core/pkg 版本一致；或迁移到 tag + semantic version，脱离 pseudo-version 漂移。

### H-2 `core` 零测试覆盖

- [ ] 整个 `core/` 下 16k+ 行代码没有任何 `_test.go` 文件。
  - **最低优先补**：
    - `core/model/chat/client_test.go` — 覆盖 `ClientRequest.Clone()`、`WithMessages` 语义
    - `core/model/chat/message_test.go` — 覆盖 `MergeMessages` / `FilterMessages` / `MergeAdjacentSameTypeMessages`
    - `core/rag/pipeline_test.go` — 并发 retrieve 的 race 检测（`go test -race`）
    - `core/document/transformer_splitter_test.go` — 覆盖切分边界

### H-3 `models/`、`vectorstores/`、`tools/` 零测试覆盖

- [ ] 三个模块合计 0 个 `_test.go` 文件。
  - **建议**：每个 provider/store 加最小契约测试（mock SDK client）：
    - 请求/响应映射正确性
    - 错误分支（nil req、上游错误）
    - streaming 累积（models）
    - filter DSL → 原生过滤器转换（vectorstores）
    - `fakeweatherquery/generate.go`（1305 行复杂业务）至少要覆盖季节/气候分带逻辑

### H-4 `core/model/chat/message.go:485` panic 违反 Go 习惯

- [ ] `FilterMessages` 在 predicate 为 nil 时 `panic("...")`。
  - **建议**：改为返回 `([]Message, error)`，或在 nil 时退化为 identity。库代码不应对调用方输入 panic。

### H-5 `core/tokenizer/tiktoken.go:32` 构造函数 panic

- [ ] `NewTiktokenWithCL100KBase` 在初始化失败时 `panic`，破坏 Go "返回 error" 契约。
  - **建议**：改为 `(*Tiktoken, error)`。调用方可选择 `Must...` 包装。

### H-6 `core/model/chat/client.go:157-160` Clone() 潜在 nil panic

- [ ] `ClientRequest.Clone()` 对 `middlewareManager / options / userPromptTemplate / systemPromptTemplate` 直接调用 `Clone()`，而这些字段可能为 nil（如 `middlewareManager` 在第 147-150 行是懒初始化的）。
  - **建议**：所有 Clone() 实现统一 `if x == nil { return nil }`；或在 `NewClientRequest` 中 eager 初始化。

### H-7 `core/model/chat/client.go:121` `WithMessages` 语义冲突

- [ ] 方法名暗示「追加」，实际实现是 `r.messages = messages`（替换）。同文件中 `WithTools`（第 138 行）却是 append。
  - **建议**：统一语义——要么全部 `With* == 追加`、要么 `With* == 替换 + Set* 存在`。推荐后者，文档化。

### H-8 `core/model/chat/response_accumulator.go` 非线程安全但未声明

- [ ] `ResponseAccumulator.AddChunk` 无锁；若 stream middleware 中两个 goroutine 同时 push 会 race。
  - **建议**：加 `sync.Mutex`，或在类型文档显式声明「非线程安全，调用方保证顺序」。

### H-9 过滤器 DSL 无输入保护

- [ ] `core/vectorstore/filter/parser` 与 `lexer` 对 token 数量、嵌套深度均无上限。
  - **风险**：恶意 / 畸形输入导致 stack overflow、OOM、ReDoS。
  - **建议**：加 `MaxTokens`、`MaxDepth` 常量，parser 超限返回 error。过滤 DSL 作为 API 边界必须视为不可信输入。

---

## 🟠 中优先级

### 架构 / 模块边界

- [ ] **A-1** `models/internal/options/options.go` 只提供极简 `Get()` helper，每个 provider 重复相同包装。建议把共通 options merge / validation 提升到 `core/model`。
- [ ] **A-2** `vectorstores/{qdrant,milvus,weaviate,pinecone,chroma}/visitor.go` 各自实现一套 filter AST visitor，差异多数在叶子节点。建议 `core/vectorstore/filter` 提供 base visitor（模板方法模式），各 store 组合扩展。
- [ ] **A-3** `pkg/` 下 25 个子包结构扁平（maps / sets / slices / stream / json / xml / text / strings / math / random / ptr / cast …）。建议聚合：`pkg/collections`、`pkg/encoding`、`pkg/util`，便于导航。
- [ ] **A-4** `models/openai/chat.go` 与 `models/anthropic/chat.go` 都有 `requestHelper` / `responseHelper` 各约 200 行，结构高度相似。建议在 `core/model/chat` 抽象共用接口，按 mixin 或 composition 减少重复。

### API 一致性

- [ ] **C-1** Streaming 返回类型不统一：Anthropic `*ssestream.Stream`（无 error）、OpenAI `(stream, error)`、Google `iter.Seq2`。建议统一成 `iter.Seq2[T, error]`（Go 1.23+ 原生迭代器），所有 provider 对齐。
- [ ] **C-2** `core/model/chat/client.go` 的 `Structured()` 接收 `StructuredParser[any]`，泛型参数被 any 吃掉，丢失类型安全。建议改为顶层函数 `Structured[T](c *Client, parser StructuredParser[T]) (T, *Response, error)`。
- [ ] **C-3** Vectorstore ID 语义不统一：Qdrant/Chroma 内部用 `uuid.NewString()` 覆盖 `doc.ID`，导致 upsert 身份丢失；其他 store 又各有不同策略。建议：统一为「优先用 `doc.ID`，空则生成」，并在 `VectorStoreConfig` 中暴露 `IDGenerator` 接口。
- [ ] **C-4** `models/google/api.go:36` 在 `NewApi` 里用 `context.Background()` 初始化 client；无法传入 caller 的 context/timeout。建议 `NewApi(ctx, cfg)`。
- [ ] **C-5** `models/openai/chat.go:169` 用 `cast.ToString(refusal)` 粗暴转换 metadata，可能丢精度或无声失败。建议类型化 metadata schema，或显式断言。
- [ ] **C-6** `models/anthropic/api.go:57-60` 在 `req == nil` 时静默返回 nil stream，调用方拿到空指针无错误提示。建议返回 `(stream, err)`。

### 并发 / 错误处理

- [ ] **E-1** `core/rag/pipeline.go:114-130` 手写 mutex 包裹 append，代码正确但不 idiomatic。建议改用 `sync.Map` 或预分配索引写入，加注释说明为何需要锁。
- [ ] **E-2** `core/model/chat/client.go:296` Stream 链路未说明「调用方必须消费完迭代器」，否则 middleware 中的 goroutine 可能泄漏。建议在迭代器 doc 上显式写明，或用 `context` 主动取消。
- [ ] **E-3** 整个库缺少包级 sentinel error。示例：`message.go:424` 的 `"at least one tool return required"` 是字符串字面量，调用方无法 `errors.Is`。建议：
  ```go
  var (
      ErrEmptyToolMessage = errors.New("chat: tool message requires at least one return")
      ErrEmptyDocument    = errors.New("document: must contain text or media")
      ErrNilRequest       = errors.New("request is nil")
  )
  ```
- [ ] **E-4** `core/model/chat/message.go:714-720` `MergeToolMessages` 失败时静默 fallback 到原消息。建议向上传播 error。
- [ ] **E-5** `core/model/chat/client.go:260` `buildRequest()` 直接透传 `getMessages()` 的 error，无上下文。建议 `fmt.Errorf("build request: %w", err)`。
- [ ] **E-6** `models/openai/chat.go:109-140` `buildUserMsg` 遍历 media 时遇错直接 `continue`，丢失「哪条失败」信息。建议收集部分错误，通过 `errors.Join` 返回。

### 一致性 / 参考完整性

- [ ] **P-1** `vectorstores/milvus/store.go:32` `maxContentLength = 65535` 硬编码，其他 store 没有等价限制。建议放到 `core/vectorstore` 并在每个 store doc 说明 provider 上限。
- [ ] **P-2** `vectorstores/pinecone` 的 store 实现未审阅但可能与 Qdrant/Chroma 在 ID/metadata 处理上有差异。**Action**：补一个跨 store 契约测试（table-driven + 相同 Document 集）。
- [ ] **P-3** `core/document/nop.go` 等 Nop 实现无 `var _ Interface = (*Nop)(nil)` 断言，重构时容易静默失效。建议每个 Nop 文件加接口契约断言。

### 安全

- [ ] **S-1** `models/openai/chat.go:170` `cast.ToString(metadata["refusal"])` 没有长度上限，恶意/异常 payload 可能造成大字符串拷贝。建议截断 + 告警。
- [ ] **S-2** `vectorstores/pinecone/visitor.go` `visitLiteral / extractFieldValue` 未验证字段类型，可能构造出不合法但服务端错误解析的 filter。建议 schema 驱动的类型校验。

---

## 🟡 低优先级

### 文档

- [ ] **D-1** 缺 `doc.go` 包说明：
  - `core/model/chat/doc.go`
  - `core/model/doc.go`
  - `core/document/doc.go`
  - `core/tokenizer/doc.go`
- [ ] **D-2** 导出类型无 godoc：
  - `core/model/chat/response.go:144` `Response` 及其字段
  - `core/model/chat/response.go:70` `Result`、:43 `ResultMetadata`
  - `core/model/chat/request.go:16-46` `Options` 的各字段（注释现为行内非 godoc 格式）
  - `core/document/document.go:11` `Document`
- [ ] **D-3** `models/openai/chat.go`、`models/anthropic/chat.go` 中 `requestHelper` / `responseHelper` / `ChatModel` 完全无注释。
- [ ] **D-4** `vectorstores/chroma/store.go:169` `distanceToScore` 有注释，但距离度量枚举（cosine/ip/l2）对应的换算公式未文档化。
- [ ] **D-5** `models/internal/options/options.go` `GetParams` 无 godoc；nil-coalescing 行为对调用方不透明。
- [ ] **D-6** `core/model/chat/tool.go:37` `ToolMetadata.ReturnDirect` 语义（失败时是否仍 return direct）未文档化。
- [ ] **D-7** `core/model/chat/message.go:71` `Message.message()` marker method 无注释说明其「密封接口」意图。

### 性能

- [ ] **F-1** `core/model/chat/client.go:457-458` `c.Chat()` 每次调用 `defaultRequest.Clone()` 做全量深拷贝，高 QPS 下分配压力大。考虑 copy-on-write 或 lazy clone。
- [ ] **F-2** `core/model/chat/response_accumulator.go:61/102/134` 多次 `EnsureIndex` 重分配 Results 切片。建议 `NewResponseAccumulator` 按预估容量预分配。
- [ ] **F-3** `core/model/chat/response_accumulator.go:116/150` `maps.Copy` 对每 chunk 的 metadata 全量拷贝。建议仅首 chunk 拷贝、后续选择性 merge。
- [ ] **F-4** `core/model/chat/request.go:125` `MergeOptions` 先 `ptr.Clone` 整个 Options，再覆盖字段，存在可见冗余。建议只拷贝被 override 的字段。
- [ ] **F-5** `vectorstores/chroma/store.go:271-289` 批量 embed 在紧循环里同步调用，无并发 / 限流。建议使用 `DocumentBatcher` + 信号量控制。
- [ ] **F-6** `tools/fakeweatherquery/generate.go:201` 每次请求 `rand.New(rand.NewPCG(xia...))` 新建 RNG。建议 `sync.Pool` 复用。

### 风格 / 未完成

- [ ] **X-1** `pkg/xml/utils.go:595` 代码中有 TODO：`update for full element like <xml attr='value'></xml>`，`isValidElementSyntax` 未处理成对标签。
- [ ] **X-2** `core/model/chat/client.go` 的 `ClientCaller`、`ClientStreamer` 为具体类型而非接口，不可 mock/替换。建议抽 `Caller` / `Streamer` 接口方便测试与扩展。

---

## 建议的执行顺序

1. **第 1 周**：H-1（版本对齐）、H-4/H-5/H-6（去掉库代码 panic）、E-3（sentinel errors）。
2. **第 2–3 周**：H-2/H-3（补最小测试集），先 `core/model/chat` 与 `core/rag` 的 race 测试。
3. **第 4 周**：C-1/C-2（统一 streaming 与 Structured 泛型），A-2（filter visitor 抽象）。
4. **持续**：D-* 文档增补、F-* 性能打磨作为低水位 PR 插入。

---

## 审查依据

- 文件/行号均基于审查当时的 `main` 分支（HEAD = `a1b4083`）。
- 本清单由两份并行深度审查合并去重而来，覆盖 core/models/vectorstores/tools/pkg 五个模块。
- 未评估性能数字（无 benchmark 基线），性能建议均基于代码阅读推断。
