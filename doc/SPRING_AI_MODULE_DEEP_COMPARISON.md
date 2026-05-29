# lynx vs spring-ai —— 模块级深度对比（逐文件、贴源码）

> **基线**
> - lynx HEAD `e480bc7`（branch `lyra`），Go 1.26.3，go.work 编入 12 个 module
> - spring-ai HEAD `7969b4d97`（branch `main`，2026-05），Spring Boot 4 + Spring Framework 7 + Reactor + Micrometer 路线
> - 生成时间：2026-05-29
>
> **本文与既有报告的关系**：`doc/SPRING_AI_COMPARISON.md`（2026-05-21）是上一版，偏"心智模型对齐 + vendor 数量盘点"。本文**重做思路**——不数 vendor，而是**贴着双方源码逐文件、逐接口、逐方法**深挖抽象设计、idiom、职责切分，找出真实的领先点 / 落后点 / gap。上一版可作角度参考，结论以本文为准。
>
> **对比范围**：lynx 除 `pkg` / `agent` / `lyra` 外的所有模块 —— `core`（拆 chat / 非 chat 模态 / document-tokenizer-eval-vectorstore接口 三块）、`models`、`vectorstores`、`tools`、`rag`、`chatmemory`、`documentreaders`、`mcp`、`otel`。`agent` 的对比另见 `doc/ABSTRACTIONS_COMPARISON.md`（对 embabel）。

---

## 0. TL;DR

### 0.1 心智模型仍然对得上

两边在概念层一一对应：Message / Document / Media / ChatModel / EmbeddingModel / Image / Audio / Moderation / Tool / VectorStore / Filter-DSL / RAG-pipeline / ChatMemory / MCP / Evaluation / Observability。**分歧全在工程形态**：spring-ai 是 *batteries-included framework*（autoconfigure / starter / BOM / Micrometer 抽象 / DI 容器），lynx 是 *thin library*（窄接口 + 显式构造 + 原生 OTel + Go 泛型）。

### 0.2 跨模块复现的几条主线（不是孤立巧合）

1. **lynx 的接口纪律系统性领先**：ISP 切碎（`vectorstore.Creator/Retriever/Deleter`、`memory.Reader/Writer/Clearer`、`tokenizer` 5 接口、`mcp.tool.ToolSource` 单方法）对 spring-ai 的单胖接口；`context.Context` + 显式 `error` 贯穿所有 SPI 对 spring-ai 的函数式接口 + `throw RuntimeException`。
2. **Reasoning 一等公民的红利在多个模块兑现**：`chat.ReasoningPart{Text, Signature}` + `Usage.ReasoningTokens *int64` 不只让 chat 模型更准，还连带让 `ResponseAccumulator`（无需 vendor 魔法字符串）、structured-output parser（无需 `<think>` 清理）、chatmemory 序列化（完整 round-trip）、models 适配器（signature 续流契约）全部受益。spring-ai 至今靠 `MessageAggregator` 硬编码 Google `"isThought"` + `ThinkingTagCleaner` 提 Nova/Qwen 打补丁。
3. **组合 + 泛型分发 完胜 继承模板方法**：vectorstores 的 `filterhelp.DispatchBinary[T]`（同一骨架既出 SQL 字符串又出 `*qdrant.Filter` SDK 对象）对 spring-ai `AbstractFilterExpressionConverter` 锁死 `StringBuilder`；models 的 stream-stitching 上提到 core `ResponseAccumulator` 对 spring-ai 每 vendor 自带 `*StreamFunctionCallingHelper`。
4. **lynx 把"协议作为维度"做出了 spring-ai 没有的能力**：`models/<x>/chat_openai.go` + `chat_anthropic.go` 让同一模型按用户手头 SDK 选 wire protocol；`openaicompat/baseurls.go` 让 20+ 长尾平台连子包都免了。
5. **spring-ai 在"产品化兜底"和"成熟度细节"上系统性领先**：自动 id 生成、chunk 溯源 metadata、PDF 版面/TOC 解析、Tika 万能 reader、cardinality 分层、典型 SQL dialect 广度、`@Tool`/`@McpTool` 零样板注解、async 一等公民。（注：早期版本列的"自动 id / chunk 溯源 / metrics"已由 lynx 补齐，见 §11/§3。）

### 0.3 各模块裁决一览

| 模块 | 总体裁决 | lynx 最强项 | spring-ai 最强项 |
|---|---|---|---|
| **core / chat** | **lynx 显著领先** | 有序 `[]OutputPart` 多块模型、Reasoning 一等、type-agnostic accumulator、零 vendor 泄漏 | structured-output cleaner 链鲁棒、PromptTemplate 可插拔 renderer |
| **core / 非chat模态** | 互有胜负 | `Usage` 三态 breakdown、Moderation 单 struct + 批量、可移植 Options 表面宽 | 维度预置表、image 多生成、result 回链原文档 |
| **core / 基础设施** | **lynx 领先** | VectorStore ISP、Tokenizer ISP（暴露 encode/decode）、Evaluation 连续分 + Composite、全 SPI 带 ctx+error | 自动 id、chunk 溯源 metadata、`Content`/`MediaContent` 父抽象 |
| **models** | **lynx 结构性领先** | OpenAI-compat 复用（5 行 vs 千行）、跨协议适配器、reasoning first-class | typed `*ChatOptions`、autoconfigure/starter/AOT、vendor 专属能力 100% 覆盖 |
| **vectorstores** | **lynx 架构领先** | 组合+泛型 dispatch、参数化防注入、ISP、零运行时依赖手写 Pratt parser、`IS NULL`+`NOT IN`+按 id 删除（✅ 已补）、AST 优化器 | 日期归一化、SQL 数值类型细分 |
| **tools** | **lynx 降维领先** | 出货 bash/fs/websearch/webfetch/httpreq + 可插拔 backend strategy | `@Tool` 零样板、内建错误降级策略、`ToolContext`、resolver+DI |
| **rag** | **lynx 领先（移植+加固）** | 编排/接入解耦、后处理有实现、两层并行+容错、stage 级 span | Query 强类型 history、保留 per-query 维度（为 RRF 留路） |
| **chatmemory** | **lynx 领先** | **序列化保真度**（完整 round-trip Parts/tool）、排序键正确性、ctx 取消、OTel span、会话枚举（`memory.Lister` ✅ 已补） | 关系型 SQL dialect 广度、dialect 自动探测 |
| **documentreaders** | **互有胜负** | SPI 干净、markdown heading-path 栈、HTML 剔除 script、依赖轻 | **PDF 版面/TOC 解析**、**Tika 万能 reader**、code 语义分类 |
| **mcp** | **lynx 领先（聚焦）** | 1.3k LOC 单包 vs ~185 文件、错误分流精确、零反射、sampling 借用 client LLM | resources/prompts/complete 全覆盖、async 一等、`@McpTool` DX |
| **otel** | **lynx 领先** | 直连 OTel 无 indirection、log 即 span、trace 覆盖 RAG/MCP/chatmemory、GenAI metrics（token usage + duration histogram，✅ 已补） | cardinality 分层、Convention 可覆盖、vendor-neutral |

### 0.4 lynx 真实 gap 汇总（按性价比，本文末有完整表）

最该补的 5 项：(1) ~~OTel metrics（token usage counter）~~ ✅ 已闭合（2026-05-29）；(2) ~~embedding token-aware batching~~ —— 非缺口，已由 document 层 `TokenCountBatcher` 覆盖；(3) ~~Document 自动 id + chunk 溯源 metadata~~ ✅ 已闭合；(4) ~~vectorstore filter `IS NULL`/`IS NOT NULL`~~ ✅ 已补（语言层 + 10 backend）—— ~~vectorstore 按 id 删除~~ ✅ 已补（`IDDeleter`，20 backend）；(5) ~~chatmemory `findConversationIds()`~~ ✅ 已补（`memory.Lister`，6 backend + core）—— 关系型 dialect 广度仍可补。其余多为 spring-ai 的成熟度细节或 lynx 有意识的 YAGNI 取舍。

> **2026-05-29 更新**：(3) 已在 `core/document` 落地——新增 `Document.EnsureID` / `IDAssigner` transformer / splitter `IDGenerator` 选项 + chunk lineage 元数据（`parent_document_id`/`chunk_index`/`chunk_total`）+ score 透传。(2) 经复核**不是缺口**：lynx 在 document 层已有 `Batcher` 接口 + `TokenCountBatcher`（token-aware 切批，算法与 Spring `TokenCountBatchingStrategy` 同源），正确用法是先 `Batch` 再逐批 embed——与 Spring 把 batching 焊进 `EmbeddingModel` 是分层差异而非能力缺失，不在 embedding 侧重复造抽象（避免 DRY 反模式）。

---

# 第一部分 · core 协议层

## 1. `core/model/chat` vs Spring AI Chat Abstraction：逐层深挖

本节做一次真正贴着源码的对比。所有结论基于实际读到的文件，不基于印象。两边都是"provider-agnostic 的 chat 协议层 + 客户端门面 + 工具调用 + 结构化输出 + prompt 模板"，但在**消息模型的形状**、**reasoning 的地位**、**vendor 知识的隔离**三处出现了根本性的设计分歧。

### 1.1 消息模型：有序 `[]OutputPart` vs 扁平 `content + media + toolCalls`

这是两边最深的分歧，也是 lynx 最强的反超点。

**Spring AI** 的 `AssistantMessage`（`messages/AssistantMessage.java`）是一个**扁平多字段**结构：

```java
public class AssistantMessage extends AbstractMessage implements MediaContent {
    private final List<ToolCall> toolCalls;   // record ToolCall(id, type, name, arguments)
    protected final List<Media> media;
    // textContent + metadata 继承自 AbstractMessage
}
```

也就是说一条 assistant 消息被拆成了**三个平行容器**：`textContent`（单一字符串）、`toolCalls`（列表）、`media`（列表），外加 `metadata` map。三者之间**没有顺序关系**。这对 OpenAI Chat Completions 的经典 `{content, tool_calls}` 形状是够用的，但对 Claude / Gemini / OpenAI Responses API 那种 `text → tool_use → text → thinking → text` 的**块级交织（interleaving）就地丢失了顺序信息**——你拿到的是"所有文本拼成一坨 + 所有 tool call 堆一个 list"，无法还原模型实际的输出时间线。

**lynx** 的 `AssistantMessage`（`message.go:128`）是**单一有序列表**：

```go
type AssistantMessage struct {
    Parts    []OutputPart   // 有序，verbatim 保留交织
    Metadata map[string]any
}
```

`OutputPart` 是一个 sealed union（`part.go:39`），靠未导出方法 `appendDelta` 封口，v1 宇宙恰好三种具体类型：`TextPart{Text}`、`ReasoningPart{Text, Signature}`、`ToolCallPart{ID, Name, Arguments}`。注释明确写着"text↔tool_use interleaving from Claude / Gemini / OpenAI Responses API is preserved verbatim"。需要扁平视图时再用 `JoinedText()` / `ToolCalls() iter.Seq[*ToolCallPart]` / `partsOf[T]` 派生——也就是 lynx 把"有序"当 source of truth，"扁平"当派生视图；Spring 反过来把扁平当 source of truth，顺序无法派生。

**谁领先**：lynx 明显领先，且是**结构性**领先而非工程细节。一旦 provider 是 Claude/Gemini 这类块级模型，Spring 的模型在 round-trip 时就有信息损失（thinking 块的位置、多段 text 与 tool_use 的相对次序）。lynx 的 `[]OutputPart` 是 Anthropic Messages API / Gemini parts / OpenAI Responses 的**最小公共上界**，而 Spring 的扁平形状是 OpenAI Chat Completions 的**直接镜像**——这是 2023 年的建模债。

**idiom 差异**：lynx 用 sealed interface + `kind` 判别式（`marshalKindedPart` 把 `{"kind":"text",...}` 拼进 JSON，一遍解码，`part.go:159`）做封闭多态；Spring 用 Java 的多字段 + builder。lynx 的 `appendDelta` 还顺手承担了**流式增量合并**的语义（同类型相邻 delta 原地 merge，类型变化或 tool call ID 变化则起新块，`part.go:62/87/121`），这是 Spring 在 `MessageAggregator` 里用一堆 `AtomicReference<StringBuilder>` 外置实现的——见 1.4。

### 1.2 消息类型：四种角色，两边一致但封口方式不同

两边都是 System / User / Assistant / Tool 四类。

- **Spring**：`Message extends Content`，具体类 `SystemMessage` / `UserMessage` / `AssistantMessage` / `ToolResponseMessage` 继承 `AbstractMessage`（`MessageType` 枚举）。`ToolResponseMessage` 用 `record ToolResponse(id, name, responseData)`。User/Assistant 实现 `MediaContent`。**开放继承**——任何人可以 `extends AbstractMessage` 造新类型，`instanceof` switch 没有穷尽性保证（`Prompt.instructionsCopy()` 里就有个 `else throw IllegalArgumentException`）。
- **lynx**：`Message` 是 **sealed interface**（`message.go:45`），靠未导出 `message()` 方法封口，外部包无法新增子类型——planner 的 type switch 因此**编译期穷尽**。`ToolMessage` 携带 `[]*ToolReturn{ID, Name, Result}`，与 Spring 的 `ToolResponse` 字段一一对应。

lynx 多了一层**构造期校验**：`NewToolMessage` 在零 tool-return 时返回 error（"a tool message with no results is meaningless"），Spring 的 `ToolResponseMessage` 允许空 list。lynx 还内置了一组消息代数：`MergeSystemMessages` / `MergeUserMessages` / `MergeToolMessages` / `MergeAdjacentSameTypeMessages`（`message.go:471-594`），Spring 把这类操作放在 `Prompt.augmentSystemMessage / augmentUserMessage`（`Prompt.java:230-286`，每次返回新 `Prompt`，函数式 copy-on-write）。两种风格各有取舍：Spring 的 `augment*` 不可变更安全；lynx 的 `MergeAdjacentSameTypeMessages` 更贴近 planner 需要的"规整后喂给 provider"的形状。

**谁领先**：消息类型本身平手；**封口机制 lynx 领先**（sealed + 编译期穷尽 vs 开放继承 + 运行时 throw）。这是 Go idiom 对 Java idiom 的胜利，不是设计高下。

### 1.3 Request/Prompt + Options：合并语义与 side-channel

**Spring** 的 `Prompt`（`Prompt.java:45`）= `List<Message> messages` + `@Nullable ChatOptions chatOptions`。Options 是**接口** `ChatOptions`，8 个标准 getter 全是 `@Nullable` 包装类型，nil = 未设。合并发生在 `ChatModel.buildRequestPrompt()`：`getDefaultOptions().mutate().combineWith(promptOptions.mutate()).build()`——builder 的 `combineWith` 取 other 的非 null 值、保留 this 的其余值。Provider-specific 参数靠**子类型**（`OpenAiChatOptions implements ToolCallingChatOptions implements ChatOptions`）承载，不是一个 map。

**lynx** 的 `Request`（`request.go:182`）= `Messages []Message` + `Tools []Tool` + `Options *Options` + `Params map[string]any`。三个刻意的设计决策：

1. **Options 是具体 struct 不是接口**（`request.go:17`），8 个标准字段全用**指针**，nil = 用 provider 默认。Provider-specific 参数走 `Extra map[string]any`，而**不是** Spring 的子类型路线。
2. **`Tools` 提到 Request 层、且 `json:"-"`**（`request.go:190`）——注释："tools are capability, not sampling configuration"，且 tools 持有 runtime 闭包不该上 wire。Spring 把 tool callbacks 塞进 `ToolCallingChatOptions`（options 子类型里），lynx 认为这是职责错配。
3. **`Params` 是显式的请求级 side-channel**（`request.go:197`）——user id / trace id / feature flags，给 middleware 读。Spring 没有对等的请求级 side-channel，靠 advisor chain 的 context map。

合并语义：lynx 的 `MergeOptions`（`request.go:120`）clone base 然后逐个 override 左到右；标量非零覆盖、**`Stop` 切片 replace 不 append**（注释专门解释 append 会让 merge 非幂等）、`Extra` map last-write-wins。

**谁领先**：互有胜负。Options 表示法上 Spring 的接口+子类型对 provider 参数有编译期类型安全，lynx 的 `Extra map[string]any` 更轻、跨 provider 统一但失去类型（KISS vs 类型安全的取舍；注意 lynx 的 `DefaultOptions()` **返值不返指针**，intentional immutability）。**Tools 的位置 lynx 领先**（capability 非 sampling config，`json:"-"`）。**side-channel** lynx 的显式 `Params` 比借 advisor context 更直接。

### 1.4 ChatModel/Client 接口形状 + 流式 + 累积

**接口形状**：Spring `ChatModel extends Model<Prompt, ChatResponse>, StreamingChatModel`，`call` + `stream(Prompt): Flux<ChatResponse>`。lynx `Model interface { model.Model[*Request,*Response]; model.StreamingModel[*Request,*Response]; DefaultOptions() Options; Metadata() ModelMetadata }`，底层是泛型 `CallHandler[Req,Resp]` / `StreamHandler[Req,Resp]`。

**流式**：Spring 用 **Reactor `Flux<ChatResponse>`**（背压、operator、调度器）。lynx 用 **`iter.Seq2[*Response, error]`**（Go 1.23+ range-over-func）——零依赖、单 goroutine 消费、`for resp, err := range stream`。

**响应累积**（最关键的对比）：

- **Spring `MessageAggregator.aggregate()`**（`MessageAggregator.java:51`）：一个方法里开了 **11 个 `AtomicReference`**，在 `doOnNext` 里手动 append。**第 104 行**硬编码 Google Vertex 约定字符串 key `"isThought"`：

  ```java
  if (metadata != null && metadata.containsKey("isThought")) {
      var isThought = Boolean.parseBoolean(metadata.get("isThought").toString());
      if (isThought) thoughtsRef.get().append(...); else outputWithoutThoughtsRef.get().append(...);
  }
  ```

  框架核心的 aggregator 据此把 thinking 内容分流——这是 vendor 知识泄漏进 provider-agnostic 核心的教科书案例。

- **lynx `ResponseAccumulator`**（`response_accumulator.go:24`）：`AddChunk` 委托给一个 **type-agnostic 的 `partAccumulator`**，只做 `if 末尾 part.appendDelta(delta) { merge } else { append }`。**从不 type-switch 具体 part 类型**，加新 part kind 不需要改这里。reasoning 的区分**根本不靠字符串 key**——`ReasoningPart` 本来就是独立类型，provider adapter 解析时就把 thinking 路由进去。

**谁领先**：lynx 在累积器设计上**显著领先**。Spring 的 11-AtomicReference + 硬编码 `isThought` 是高耦合、违反 OCP、vendor-leaky。lynx 把"如何合并"下沉到每个 `OutputPart.appendDelta`（OCP 正解），accumulator 只是 dumb 的 ordered-merge 循环。流式机制 `iter.Seq2` vs `Flux` 是生态选择。

### 1.5 工具调用：递归 middleware + ShouldReturnDirect vs ToolCallback + ToolCallingManager

**Spring**：`ToolCallback` = `getToolDefinition()` + `getToolMetadata()` + `String call(String toolInput)`。编排在 `ToolCallingManager`（2 方法接口）。tool 注册在 `ToolCallingChatOptions`。

**lynx**：`Tool`（`tool.go:51`）= `Definition()` + `Metadata()` + `Call(ctx, arguments) (string, error)`——字段几乎一一对应 Spring。差异在编排：

1. **tool loop 是一条 middleware**（`tool_middleware.go`），`NewToolMiddleware()` 一次返回 call + stream 两半，靠 `MiddlewareManager` 的 type-assert 自动各入两条链。
2. **递归而非循环**：`executeCallRecursively`——call → 有 tool call 则 invoke → `ShouldReturn()` 收尾、否则 `BuildContinueRequest()` 递归。
3. **`ShouldReturnDirect` 是 first-class flow control**：当最后一条 ToolMessage 中**每个 tool 都 `ReturnDirect=true`**，直接合成 response 返回不再 prompt LLM。lynx 把"整批是否全部 return-direct"提成 `ToolInvocationResult.allReturnDirect` 的集合判断。
4. **流式工具循环的可观测性**：lynx 在 stream 工具往返中段**合成一个只填 `Result.ToolMessage`、`AssistantMessage=nil` 的 Response yield 出去**（`newToolMessageResponse`），让流消费者看到真实时间线。Spring 流路径没显式建模。
5. **不为 HITL/异步建专门 tool 类型**：人审批/前端委派/异步分发不建模为单独类型，由上层 wrap Tool + sentinel error 透出 control-flow（KISS）。

**谁领先**：核心契约平手；**编排 lynx 略领先**（可插拔 middleware 覆盖 call+stream + 流式中段时间线建模）。

### 1.6 结构化输出：`JSONParser[T]` 泛型 vs `BeanOutputConverter` 反射

**Spring** `StructuredOutputConverter<T>`，成员 `BeanOutputConverter<T>` / `MapOutputConverter` / `ListOutputConverter`。`BeanOutputConverter` 用 Jackson + `ParameterizedTypeReference<T>`（绕泛型擦除）生成 schema，`convert(text)` 先过 **`CompositeResponseTextCleaner`**（`WhitespaceCleaner` → `ThinkingTagCleaner` → `MarkdownCodeBlockCleaner` → `WhitespaceCleaner`）再反序列化。

**lynx** `StructuredParser[T] interface { Instructions() string; Parse(raw) (T, error) }`，成员 `JSONParser[T]` / `ListParser` / `MapParser` / `AnyParser`。`JSONParser[T]` 构造期用 `pkgjson.StringDefSchemaOf(zero)` 生成 schema（Go 真泛型不需要 `ParameterizedTypeReference`），`Parse` 用 `removeMarkdownCodeBlockDelimiters` 剥代码围栏。

**两个关键差异**：

1. **thinking-tag 清理的归属**：Spring 在默认 cleaner 链里塞 `ThinkingTagCleaner`（"removes thinking tags from models like Amazon Nova and Qwen"）——又一处 vendor 知识泄漏（这次在 converter 层）。**lynx 明确拒绝在 parser 里剥 reasoning**：注释写"reasoning wrappers (`<think>`, ...) 由 provider adapter 处理，会在 parser 见到文本前路由进 `ReasoningPart`"。lynx 靠 `ReasoningPart` 架构红利**绕开了这个问题**。
2. **流式结构化**：lynx 诚实标注"Structured parsing on streams is not yet implemented"（`client.go:319`），Spring 同样不支持。

**谁领先**：互有取舍。Spring 的 cleaner 链对脏输入容错更强（代价是烤进 vendor 假设）；lynx 靠 `ReasoningPart` 绕开，更干净但单看 parser 输入容错弱些。CLAUDE.md 记录这族 "closed-by-design"。

### 1.7 PromptTemplate

- **Spring** `PromptTemplate`，默认 renderer 是 **StringTemplate（ST4）** `StTemplateRenderer`，可换 `TemplateRenderer` 接口；能 `createMessage()` / `create(ChatOptions)`，`render` 特殊处理 `Resource` 变量（读文件）；有 `SystemPromptTemplate` / `AssistantPromptTemplate` 子类。
- **lynx** `PromptTemplate`（`prompt_template.go:23`）包一个 `text.Renderer`（**Go `text/template`**，`{{.name}}`），`CreateSystemMessage()` / `CreateUserMessage()`（后者带 media，前者刻意不带），多一个 `RequireVariables` 校验。

**谁领先**：Spring 略领先在功能广度（可插拔 renderer、Resource 自动读取、按角色分子类）；lynx 更 KISS。常规成熟度差距。

### 1.8 Usage / cache token 的三态

**Spring** `Usage`：`getPromptTokens()/getCompletionTokens()` 返回 `Integer`。Cache 是 2.0.0 才加的 `getCacheReadInputTokens(): @Nullable Long` / `getCacheWriteInputTokens(): @Nullable Long`，默认 `null`。**没有** reasoning token 维度。

**lynx** `Usage`（`core/model/usage.go`）：`PromptTokens/CompletionTokens int64` 是值；breakdown 三件套全用**指针三态**：

```go
ReasoningTokens       *int64  // nil = provider 没报这个维度，≠ 0
CacheReadInputTokens  *int64
CacheWriteInputTokens *int64
OriginalUsage         any     // 对应 Spring getNativeUsage()
```

注释精确区分"provider reported zero" vs "did not surface this dimension"，breakdown **故意不进 `TotalTokens()`** 避免重复计数，配 `HasReasoningTokens()` 等 predicate。

**谁领先**：lynx 领先两点：(1) **ReasoningTokens 是 first-class usage 维度**，Spring 没有；(2) **三态语义统一**贯彻到 reasoning/cache 全维度，Spring 三种维度三种处理且缺 reasoning。

### 1.9 Provider 知识泄漏：核心是否 grep 到 vendor 字符串

直接 grep `core/model/chat` 全包：**所有 vendor 字符串 100% 出现在 doc 注释和示例里，零运行时分支**。`ModelMetadata{Provider string}` 是通用 tag 字段，不是 switch 条件。

对比 Spring `MessageAggregator.java:104` 在**核心累积路径**硬编码 `"isThought"`（Google 约定）；`BeanOutputConverter` 的 `ThinkingTagCleaner`（Nova/Qwen）是第二处。

**谁领先**：lynx **干净地领先**。靠 `ReasoningPart` first-class 类型，在架构层面就避免了"用魔法字符串区分 thinking"的需求——这正是 lynx "core 是协议层、vendor 知识在外圈" 约束的兑现，spring-ai 在这条线上有真实违反。

### 本节小结

- **lynx 反超**：有序 `[]OutputPart` 多块模型（最重要）；Reasoning first-class（连带 accumulator/parser/usage 全受益）；type-agnostic `ResponseAccumulator`（OCP）；零 vendor 泄漏；Usage 三态统一；Tools 提到 Request 层 + 显式 `Params`；tool loop 可插拔 middleware；sealed `Message` 编译期穷尽。
- **spring-ai 反超**：structured-output cleaner 链更鲁棒；PromptTemplate 功能更广（可插拔 renderer + Resource + 角色子类）；Options 子类型路线对 provider 参数有类型安全；`Flux` 响应式栈能力面更宽；整体周边（observation/advisor）更成熟。
- **真实 gap**：流式结构化输出未实现（自陈 TODO，Spring 也不支持）；结构化解析输入容错弱于 composite cleaner（部分被 ReasoningPart 红利抵消）；provider-specific 参数无强类型通道（设计取舍）。

---

## 2. 非聊天模态抽象：embedding / image / audio / moderation

> 范围：`embedding` / `image` / `audio (tts + transcription/STT)` / `moderation` 四模态及其共享的 base model 抽象。聚焦**抽象设计**而非 vendor 数量。

### 2.1 Base model 抽象：泛型骨架 vs 泛型 + 强制 wrapper 层级

**lynx**（`core/model/model.go` / `handler.go`）把 base 压到极致：

```go
type CallHandler[Request, Response any]   interface { Call(ctx, req) (Response, error) }
type StreamHandler[Request, Response any] interface { Stream(ctx, req) iter.Seq2[Response, error] }
type Model[Req, Resp any]          interface { CallHandler[Req, Resp] }   // 零方法 compose
type StreamingModel[Req, Resp any] interface { StreamHandler[Req, Resp] }
```

`Request`/`Response` 是**裸 `any`**，无上界约束；`Model`/`StreamingModel` 是不加方法的 compose alias（目的只是让 `func(m Model[...])` 自我说明）；`ctx` 头等参数，stream 用 `iter.Seq2`。

**Spring AI** 用四层有界泛型层级：`Model<TReq extends ModelRequest<?>, TRes extends ModelResponse<?>>`、`ModelRequest<T>`（getInstructions+getOptions）、`ModelResponse<T extends ModelResult<?>>`（getResult/getResults/getMetadata）、`ModelResult<T>`、`ModelOptions`（marker）。强制结构契约，无 `ctx`，stream 是 `Flux<T>`。

**谁领先**：哲学差异。Spring 强制结构一致性（框架层能统一处理任意 `ModelResponse`），代价是每模态被迫实现 5 接口 + 一堆手写 equals/hashCode。lynx 裸 `any` 几乎零约束，每模态自定义 flat struct，代价是 base 层无法统一处理（但 lynx 故意不在 base 做，observability 下沉各模态 `tracing.go`）。**核心 idiom 差异**：lynx 每模态顶部一段 type-alias 把通用 `CallHandler/Middleware` reify 成模态本地名字——**可组合 middleware chain 是 lynx 独有的一层**，Spring 没有 model-level middleware（靠 Advisor/Observation）。

### 2.2 Embedding

| | lynx `embedding.Model` | Spring `EmbeddingModel` |
|---|---|---|
| 核心方法 | `Call(ctx, *Request)(*Response,err)` | `EmbeddingResponse call(EmbeddingRequest)` |
| 便利重载 | 无（统一走 `Client` builder） | `embed(String)`/`embed(Document)`/`embed(List<String>)` 全是 default |
| 维度探测 | `Dimensions(ctx) int64` + 包级缓存 | `dimensions()` + `AbstractEmbeddingModel` 缓存 |

**维度探测**：lynx 用包级 `sync.Map` 缓存 `provider:model`，首调发 `"test"` 请求；Spring 先查 classpath `embedding-model-dimensions.properties`（**预置维度表**），miss 才探测——**Spring 领先**（免一次网络往返）。

**Batching（分层差异，非能力缺失）**：Spring `BatchingStrategy` 是头等接口，`TokenCountBatchingStrategy` 用 JTokkit 按 token 切批（默认 8191 上界、10% reserve），**焊进 `EmbeddingModel`**。lynx 把同样的 token-aware 切批放在 **document 层**——`document.Batcher` 接口 + `document.TokenCountBatcher`（算法与 Spring 同源：8191 上界 + 10% reserve + 贪心装箱保序 + 单文档超限报错），正确用法是先 `Batch` 再逐批 embed。embedding `Request.Texts []string` 本身不再切批是刻意的：切批责任在 document 摄取阶段，embedding 模型层只管把给定输入发出去。**这是分层选择而非缺口**——lynx 不在 embedding 侧重复造抽象（避免 DRY 反模式）。

**Result**：lynx `float64`，Spring `float32`（省内存）；Spring `EmbeddingResultMetadata` 带 `documentId/documentData` 回链原文档，lynx 只有 `Index`——**Spring 元数据更丰富**。

### 2.3 Image 生成

lynx `Request{Prompt string, Options, Params}` 单 prompt、**单图 per call**（注释明说 N>1 用 provider SDK）；`image.Options` 字段比 Spring `ImageOptions` 多（NegativePrompt/Quality/Seed/OutputFormat first-class）。Spring `ImagePrompt` 支持 `List<ImageMessage>`（带权重多段 prompt）+ 原生 `List<ImageGeneration>`（N 张）。**lynx 可移植 option 表面更宽**，**Spring 多 message prompt + N 张更通用**（lynx 砍单图是 YAGNI 取舍，是明确能力缺口）。

### 2.4 Audio：TTS + Transcription(STT)

**广度**：lynx 两子包齐全；Spring `spring-ai-model` 主模块**两个接口都有**（`TextToSpeechModel` + `StreamingTextToSpeechModel`、`TranscriptionModel`）——"Spring main 无 STT"的预设在**抽象层不成立**，差距在 provider 实现广度。

- **TTS**：两边都 call+stream 合一（lynx `iter.Seq2[[]byte]` vs Spring `Flux<byte[]>`），基本对等。
- **STT**：lynx `Request.Audio *media.Media` 统一容器，Options 最全（Language/Prompt/Temperature/ResponseFormat/**TimestampGranularity**/Extra）；Spring `AudioTranscriptionOptions` **只有 `getModel()`**，其余靠 provider 子类。**lynx 在 STT option 可移植性上明显领先**。两边都无 streaming STT。

### 2.5 Moderation

lynx 单 `Moderation` struct 平铺 18 个 `Category{Flagged, Score}` + `Flagged()` 聚合 + **批量 `Texts []string`** + 多覆盖 Illicit/IllicitViolent。Spring 把 `Categories`（boolean）和 `CategoryScores`（分数）**拆成两个 class**，各自 builder（几百行 setter），单 message。**lynx 领先**：更内聚、零样板、批量审核更实用；Spring 是典型 Java 过度拆分。

### 2.6 Streaming 与 Usage 报告

- **Streaming**：lynx **只有 chat 和 tts 暴露 stream**，其余模态显式不生成 stream alias（精确裁剪）；Spring `StreamingModel` 独立接口，TTS 实现。
- **Usage（关键不一致）**：lynx `model.Usage` 全模态共享（指针式 breakdown + RateLimit），但**只有 embedding 的 `ResponseMetadata` 挂了 `Usage`**，image/tts/transcription/moderation 都没有 Usage 字段。Spring 同样偏薄。**lynx 的 `Usage` 类型本身明显更先进**，缺口是没铺满模态。

### 本节小结

- **lynx 反超**：`Usage` 类型最先进（指针 breakdown 区分"未报告 vs 零" + cache + reasoning + RateLimit）；Moderation 单 struct + 批量 + 多维度；可移植 Options 表面宽（image/STT first-class 字段）；模态本地 middleware chain；无 Java 味样板。
- **spring-ai 反超**：维度预置表免探测；image 多生成 + 带权重多段 prompt；embedding result 回链原文档；base 层统一结构契约。（注：embedding token-aware batching 两边都有——Spring 焊进 `EmbeddingModel`，lynx 放在 document 层 `TokenCountBatcher`，是分层差异非缺口。）
- **gap**：Usage 未铺满模态（仅 embedding）；streaming STT 两边都没有；observability 仅 embedding 有 span。

---

## 3. core 基础设施：Document / Tokenizer / Evaluation / VectorStore 接口 / Media

> 对比范围：Document 模型 / Reader-Writer-Transformer SPI / Splitter / Formatter / Tokenizer / Evaluation / VectorStore 接口 / Media / 批处理。

### 3.1 Document 模型 + ID 生成

| 维度 | lynx (`document.go`) | Spring (`Document.java`) |
|---|---|---|
| 类型 | `struct{ID, Score, Text, Media, Formatter, Metadata}` | `final class` + 私有构造 + `Builder` |
| 可变性 | 全 exported 可直接改 | 全 `final`，改字段走 `mutate()` 返新 builder |
| Text vs Media | "至少一个非空"，**允许同时持有** | `text ^ media`（**严格异或**） |
| Score | `float64`（0.0 无法区分"未评分") | `@Nullable Double`（null=未评分） |
| ID 默认 | **不自动生成**（`ID=""`） | builder 默认 `RandomIdGenerator`，零配置就有 UUID |

**ID 生成器**：lynx `Generator.Generate(ctx, objects...) (string, error)`——**带 ctx + error + salt**，JSON 序列化（**跨语言可移植**）。Spring `IdGenerator.generateId(Object...) String`——无 ctx 无 error，用 Java `ObjectOutputStream`（**锁死 JVM**）。

**谁领先**：lynx 反超 ctx+error+salt+跨语言；Spring 反超零配置自动 id + `text^media` 严格异或 + `@Nullable Double score` 表达"未评分" + `mutate()` copy-on-write。lynx Document **没有 embedding 字段**（embedding 是独立 Response，不污染 Document），比 Spring 干净。

### 3.2 Reader / Writer / Transformer SPI

Spring 三个 SPI **全部复用 JDK 函数式接口**（`Supplier`/`Consumer`/`Function`），好处是 lambda 即 reader，坏处是**无 ctx、无 checked exception**（全部 catch IOException 再 `throw RuntimeException`）。lynx 三接口全部**首参 `ctx` + 末位 `error`**，可取消、`%w` 包装。**lynx 反超**（结构性，`TokenCountBatcher.Batch` 循环里查 `ctx.Err()`）；**Spring 反超**便利性（与 JDK Stream 无缝组合）。lynx 额外有 `Nop` 单例满足全部 5 接口。

### 3.3 Splitter

token 切分算法**几乎逐行同构**（同源参数 chunkSize 800 / minChunkSizeChars 350 等）。架构差异：lynx `Splitter` 持有 `SplitFunc func(ctx,text)([]string,error)` 一等函数（**策略模式 + 组合**），Spring `abstract class TextSplitter` + `protected abstract splitText`（**继承**）。**chunk metadata（Spring 更完整）**：Spring 注入 `parent_document_id`/`chunk_index`/`total_chunks` + 透传 score；lynx 只 `maps.Clone(metadata)`，**不注入溯源、不透传 score**——**实质 gap**。

### 3.4 Formatter / MetadataMode

`MetadataMode` 四常量两边对应。Spring `DefaultContentFormatter` 模板化（可配 `metadataTemplate`/`textTemplate`），lynx `SimpleFormatter` 硬编码格式（KISS）。lynx `filterMetadataByMode` 全程 `maps.Clone` 防御性拷贝，Spring `ALL` 模式直接返回内部 map 引用——**lynx 防御性更好**。

### 3.5 Tokenizer

| | lynx | Spring |
|---|---|---|
| 接口拆分 | **5 个 ISP 接口**：TextEstimator/MediaEstimator/Estimator/Encoder/Decoder/Tokenizer | **1 个** `TokenCountEstimator` |
| 编解码 | **暴露 `Encode/Decode`** | **不暴露**（splitter 内部自己 new Encoding） |

**lynx 明显领先**：ISP 切碎 + 暴露 encode/decode 让 `TokenSplitter` 通过注入 `Tokenizer` 接口解耦；Spring 没有 Encoder/Decoder 抽象，`TokenTextSplitter` 把 JTokkit 写死（**抽象泄漏 + 不可替换**）。

### 3.6 VectorStore 接口 —— ISP 对比（本节核心）

**lynx**（`vectorstore/store.go`）拆 4 接口：`Creator` / `Retriever` / `Deleter` / `Store = Creator+Retriever+Deleter+Metadata()`。**Spring** 单接口 `VectorStore extends DocumentWriter, VectorStoreRetriever`（add/3×delete/search/getNativeClient 全塞）。

关键对比：(1) **ISP 颗粒度** lynx 完胜——RAG 检索器只依赖 `Retriever` 一方法，ingestion 只依赖 `Creator`；Spring 任何消费方被迫认识全部能力。(2) lynx 每操作有专门 Request + `Validate()`（`RetrievalRequest.Validate` 会跑 `filter.Analyze` 静态分析）。(3) **删除语义**：Spring 有"按 id 列表删"+ "按 filter 删"；lynx 原先只有 by-filter，**现已补齐**——可选 `vectorstore.IDDeleter{ DeleteByIDs(ctx, ids) }`（刻意不入 `Store` union，ISP-pure 非破坏，消费方 type-assert），20 个 backend 落地。(4) **桥接**：lynx `NewDocumentWriter(creator)` 单向适配；Spring 让 `VectorStore extends DocumentWriter`（**继承式耦合**）。(5) **observation/batching**：Spring 焊进接口契约；lynx 在外层组合，接口保持纯净。

### 3.7 Media / Content 模型

lynx `Media{ID, Name, MimeType, Data any, Metadata}` + **自定义 `MarshalJSON`/`UnmarshalJSON` + `data_encoding` 判别符**（base64 vs text 无损 round-trip）+ `DataAsBytes/String` 返 error + 自带 metadata。Spring `Media` 无自定义序列化、`getDataAsByteArray` 类型不符 `throw`、**无 metadata 字段**；但有 `Content`/`MediaContent` 父抽象（Document 和 Message 共享内容契约，`estimate(MediaContent)` 通吃）+ `Media.Format` 30+ MIME 常量。**lynx 反超** JSON round-trip + error + metadata；**Spring 反超** `Content` 父抽象 + MIME 常量便利。

### 3.8 Evaluation

| | lynx | Spring |
|---|---|---|
| 接口 | `Evaluate(ctx, *Request)(*Response, error)` | `evaluate(EvaluationRequest)`（无 ctx/error） |
| 内置实现位置 | **全在 commons 级**：llmEvaluator/FactChecking/Relevancy/Composite | commons **只有空接口**；具体实现在 client-chat |
| 评分模型 | **连续分 [0,1]** + 可配阈值 | FactChecking **只回 yes/no**（score 恒 0，二元） |
| 组合 | `CompositeEvaluator`（AND-of-Pass / avg-Score / namespace） | **无** |

**lynx 大幅领先**：连续评分 > 二元；commons 自带完整三件套（Spring commons 是空壳）；`CompositeEvaluator` 独有；ctx+error。

### 本节小结

- **lynx 反超**：VectorStore ISP（全文最大架构优势）；Tokenizer ISP + 暴露 encode/decode；Evaluation 连续分 + Composite + commons 自带实现；全 SPI 带 ctx+error；Media JSON round-trip；ID 生成器 ctx+error+salt+跨语言；Splitter 策略模式；防御性 metadata clone。
- **spring-ai 反超**：Document 零配置自动 id；`text^media` 严格异或 + `@Nullable score`；Splitter 注入溯源 metadata + 透传 score；`Content`/`MediaContent` 父抽象；VectorStore 按 id 删除；`Media.Format` MIME 常量 + Formatter 模板化。
- **真实 gap**：切分**无 chunk 溯源 metadata**（实质功能缺口）；VectorStore **无按 id 删除**；Document **无自动 id**；**无 `Content` 统一抽象**（Document/Message 内容估算各写各）。

---

# 第二部分 · 模型与存储生态

## 4. models 适配器架构

> 聚焦**适配器是怎么搭起来的**（不是数 vendor）。读了 lynx `openai/anthropic/ollama/bedrock/xai/deepseek/zhipu/moonshot` 与 Spring `openai/anthropic/deepseek/minimax/ollama/bedrock-converse`。

### 4.1 适配器构造模式：wire format ↔ core chat types

**lynx**：每 provider 一子包，固定 `requestHelper` / `responseHelper` / `ChatModel` 三件套。响应侧把 SDK message 拆回 `[]chat.OutputPart`（ReasoningPart→TextPart→ToolCallPart）。关键：**stream 复用 non-stream 路径**——每个 SSE chunk 起一个全新 accumulator 喂一个 delta，再交给同一个 `buildChatResponse`，上层 `chat.ResponseAccumulator` 负责跨 chunk 缝合。

**Spring AI**：`OpenAiChatModel`（~600 行单类）把请求构造、流式工具调用合并、metadata 组装、observation、`ToolCallingManager` 全塞一个类；流式工具累积手写 `builders.computeIfAbsent(key, ...).merge(tc)`，每 vendor 各自重写（DeepSeek/MiniMax 各有 `*StreamFunctionCallingHelper`）。

**差异本质**：lynx 把 stream stitching 上提到 core 的 `ResponseAccumulator`，provider 只产 delta、更薄更同构；Spring 让每 vendor 自己缝。

### 4.2 OpenAI-compat 复用策略（lynx 最大结构性优势）

**lynx**：新增一个 OpenAI 兼容 vendor ≈ 一个 `chat.go`，纯转发到 `openai.ChatModel`：

```go
// models/xai/chat.go —— 整个文件就这点
func NewOpenAIChatModel(cfg OpenAIChatModelConfig) (*openai.ChatModel, error) {
    baseURL := cmp.Or(cfg.BaseURL, BaseURL)
    reqOpts := append([]option.RequestOption{option.WithBaseURL(baseURL)}, cfg.RequestOptions...)
    return openai.NewChatModel(openai.ChatModelConfig{
        APIKey: cfg.APIKey, DefaultOptions: cfg.DefaultOptions, RequestOptions: reqOpts,
        Metadata: &chat.ModelMetadata{Provider: Provider}, // observability 打真实品牌
    })
}
```

三个支点：(a) `option.WithBaseURL` 改 endpoint；(b) `ChatModelConfig.Metadata` 覆盖 Provider 字符串；(c) `Options.Extra` + `options.GetParams[T]` 透传 vendor 字段。`deepseek/xai/groq/together/fireworks/perplexity` 全是这 5 行模板。更极致的是 `openaicompat/baseurls.go`——**一整个文件全是 base-URL 常量**（Volcano/Qianfan/SiliconFlow/Groq/Together/NVIDIA/SambaNova/GitHub Models...20+ 个），连子包都不开。

**Spring AI**：**没有 groq/together/xai/fireworks/perplexity/nvidia/zhipu/moonshot 任何模块**。要么用户自己填 `OpenAiChatOptions.baseUrl`，要么——像 DeepSeek/MiniMax——**整包 hand-roll**（`DeepSeekApi.java` 953 行，不 import OpenAiApi，各带一份 `*StreamFunctionCallingHelper`）。一个 OpenAI 兼容 vendor 成本 ≈ 1000 行 + 独立 artifact + starter + autoconfigure。

**结论**：同样接入 xAI/Groq，lynx 是 5 行转发或一行 base-URL 常量；Spring 要么逼用户裸配 base-url（丢品牌化 observability + vendor 字段），要么复制近千行。**数量级优势**。

### 4.3 跨协议适配器（lynx 独有，Spring 完全没有）

lynx 的洞察：**一个 provider 可被多种 wire protocol 说话**：

| 包 | 文件 | 协议 | 委托给 |
|---|---|---|---|
| `anthropic` | `chat.go` / `chat_openai.go` | 原生 `/v1/messages` / OpenAI-compat 端点 | 自己 / `openai.ChatModel` |
| `ollama` | `chat.go` / `chat_openai.go` | `/api/chat` / `/v1/chat/completions` | ollama SDK / `openai.ChatModel` |
| `zhipu` | `chat.go` / `chat_anthropic.go` | OpenAI-compat / Anthropic-compat | `openai` / `anthropic` |
| `moonshot` | `chat.go` / `chat_anthropic.go` | OpenAI / Anthropic 双协议 | 两个 base 包 |

同一个 GLM-4.6，用户按手头 SDK 选协议（Claude Code 集成选 Anthropic 形状，OpenAI 工具链选 OpenAI 形状）。**Spring AI 没有任何对应物**——模型类与协议 1:1 死绑。这个"协议作为可选维度"的抽象 Spring 整个架构不存在。

### 4.4 Reasoning / thinking 的逐 provider 解析

lynx 把 reasoning 做成 `chat.ReasoningPart{Text, Signature}`：

- **Anthropic**：`thinking` block → `ReasoningPart{Text, Signature}`；回放时**没有 signature 的 ReasoningPart 直接 skip**（Anthropic replay 硬约束）。
- **OpenAI Chat Completions**：从 `msg.JSON.ExtraFields["reasoning_content"]` 抠原始 JSON（接 DeepSeek-R1/vLLM），无签名。
- **OpenAI Responses API**：原生 `output[]` 有序数组，`reasoning` item 1:1 映射，连 `usage...ReasoningTokens` 也抽进 `Usage.ReasoningTokens`。

**Spring AI**：thinking 不是 first-class part，`AnthropicChatModel.java:458` 塞进 `ChatGeneration` metadata：`thinkingProperties.put("thinking", TRUE)`。signature 续流的跨 provider 统一抽象 Spring 没有。

### 4.5 SDK 依赖策略

lynx 一致"包官方 SDK + 薄 API wrapper"（openai-go/v3、anthropic-sdk-go、ollama/api、aws-sdk Converse；DeepSeek/MiniMax **复用 openai-go** 零新 SDK）。Spring 混合（OpenAI 已转官方 `com.openai` SDK，但 DeepSeek/MiniMax/Ollama/Anthropic 仍 hand-roll `RestClient`，953 行/包）。

### 4.6 打包成本 + provider 专属 options

- **打包**：lynx 一个 sibling 目录、多模态同居（`openai/` 一个包 8 个模态文件）、无 starter/autoconfigure/AOT。Spring 每 vendor **artifact 三件套** + `*RuntimeHints` 喂 GraalVM AOT，embedding 甚至单拆 artifact。
- **options**：lynx 统一 `Options.Extra map[string]any`，vendor 把原生 SDK 请求结构体整个塞进去用 `GetParams[T]` 取出（**任何 SDK 字段零适配透传**）。Spring 每 vendor typed `*ChatOptions`（强类型、IDE 友好，但新增字段要改类、SDK 没暴露的够不着）。

### 本节小结

- **lynx 反超（结构性）**：OpenAI-compat 复用（数量级）；跨协议适配器（Spring 缺失维度）；reasoning first-class + signature 续流；stream stitching 上提 core；per-vendor 打包成本低。
- **spring-ai 反超**：typed `*ChatOptions` 编译期类型安全 + IDE 补全；autoconfigure/starter/AOT 开箱即用 + GraalVM；同步+异步双客户端。
- **gap**：compat 复用代价是**下沉到最小公分母**——`anthropic/chat_openai.go` 自陈 "bridge is wire-format-only"，cache_control/computer-use/citations 在 OpenAI 形状下够不着；Spring hand-roll 虽贵但 100% 覆盖 vendor 专属能力（`AnthropicCitationDocument`/`AnthropicSkill`/`BedrockCacheStrategy` 有专门类，lynx 只能靠 Extra 透传）。`Extra` map 把"是否支持"推到运行时，缺 typed options 的发现性。

---

## 5. vectorstores：Filter 迷你语言 + per-store adapter

### 5.1 体量速览（广度持平，架构才是看点）

| 维度 | lynx | Spring AI |
|---|---|---|
| Backend | 27 目录 / 25 `visitor.go` | `vector-stores/` 22 模块 / 20 `*FilterExpressionConverter` |
| Filter DSL | 手写 lexer + Pratt parser + AST + analyzer，~2k LOC，**零运行时依赖** | ANTLR4 生成（~3.2k 生成 LOC）+ `antlr4-runtime.jar` 运行时依赖 |
| AST → 方言 | per-store `ast.Visitor`（组合 + 共享 `internal/filterhelp` 泛型 dispatch） | per-store `extends AbstractFilterExpressionConverter`（继承模板方法） |
| observation/batching | 组合在外 | 烘焙进 `AbstractObservationVectorStore` 基类 |

### 5.2 Filter DSL 实现：手写编译器 vs ANTLR4 生成

**lynx**（`core/vectorstore/filter/`）是教科书式小编译器：`token/`（Kind 用 bitmask 分类）→ `lexer/`（手写扫描器，暴露 `iter.Seq[token.Token]`）→ `parser/`（**Pratt parser / precedence climbing**，`prefixHandlers`/`infixHandlers` 注册式）→ `ast/`（**sealed AST**，`Expr` 接口三层 marker 封口）→ `visitors/analyzer.go`（**独立语义分析 pass**，与解析分离）。

**Spring** 整套 parser 由 `Filters.g4`（126 行语法）ANTLR4 生成，手写的只有 `FilterExpressionVisitor` glue。

**算符对照（真正分叉处）**：

| 能力 | lynx | Spring |
|---|---|---|
| EQ/NE/GT/GTE/LT/LTE/AND/OR | ✅ | ✅（多别名 `&&`/`||`/大小写） |
| NOT | ✅ 真 AST 节点 | parse 后被德摩根 `negate()` 下推消除 |
| IN | ✅ | ✅ |
| **NIN（NOT IN）** | ✅ 已补（2026-05-29，`x NOT IN (...)` 中缀语法，复用 `NOT`+`IN` 两 token → `UnaryExpr{NOT, BinaryExpr{IN}}`，每个 backend 的 NOT+IN handler 免费翻译）| ✅ 一等公民 + `expandNin` fallback |
| **IS NULL / IS NOT NULL** | ✅ 已补（2026-05-29，`is`+`null` keyword token，复用 `BinaryExpr`+`NOT`）| ✅ 一等公民 |
| **LIKE** | ✅ 一等公民 | ❌ |
| **索引/嵌套路径** | ✅ `metadata['a']['b']`、`arr[0]` | ⚠️ 仅单层 `a.b` |
| 日期 | ❌ | ✅ ISO-8601 自动转 Date |
| 数值类型 | NUMBER 统一 float64 | Long/Integer/Decimal 细分 |

**Spring 更"数据库化"**（日期/Long），**lynx 更"通用过滤化"**（LIKE + 任意深度索引）—— IS NULL / NOT IN 已对齐。

### 5.3 AST → 方言翻译：组合(Go) vs 继承(Java)（全篇最尖锐对比）

**Spring**：`AbstractFilterExpressionConverter` abstract base，子类继承 override `doExpression`/`doKey`/`doSingleValue`，输出**始终是 `String`**（写进共享 `StringBuilder`）。

**lynx**：每 backend 独立 struct 实现 `ast.Visitor`（1 方法接口），**无继承**。共性靠 `internal/filterhelp` 的**泛型 dispatch**：

```go
func DispatchBinaryErr(expr *ast.BinaryExpr,
    onLogical, onComparison, onIn, onLike func(*ast.BinaryExpr) error) error
```

**关键：emit type 不锁死成 String**。`DispatchBinary[T any]` 泛型 T 可以是任何东西：pgvector emit `String` + `args []any`（参数化）、redis emit RediSearch 方言、**qdrant emit `*qdrant.Filter` SDK struct**（`filter.Must`/`Should`/`MustNot`，完全不经过字符串）。Spring 因锁死 `StringBuilder`，qdrant 这种"翻译成 SDK 原生 struct"要绕过基类。这是 **composition + generics 对 inheritance 的实质胜利**。

**NOT 处理**：Spring 用德摩根律展开下推；lynx 当 AST 节点直接翻译（pgvector `(NOT ...)`、redis `-(...)`、qdrant `MustNot`），更直接。

### 5.4 Store 接口 / SQL 注入安全

接口对比：lynx ISP 三分（Creator/Retriever/Deleter）+ `RetrievalRequest.Validate()` 跑 `filter.Analyze`（**请求级语义校验前置**）；Spring 单大接口 + builder setter `Assert`。**删除**：Spring `delete(List<String> ids)` 支持按 id；lynx 只 by-filter（gap）。

**SQL 注入（lynx 更硬）**：lynx 三道防线——词法层 `IsIdentifier`、`internal/ident` 正则校验表名/列名、**值层全走 `$N` 占位 + `args []any`**（从不拼进 SQL）。Spring 是 jsonpath 字符串拼接 + `replace("'","''")` 转义——**lynx 参数化比字符串转义更抗注入**。

### 5.5 observation/batching + inmemory 参考实现

Spring 把 observation+batching **烘焙进 `AbstractObservationVectorStore` 基类**（继承即免费，但关不掉只能 NOOP）。lynx **组合在外**（`internal/tracing` + 调用方注入 `DocumentBatcher`），接口纯净，靠 `internal/storetest.VisitorConformance` 共享一致性套件兜底。

lynx `inmemory/filter.go`（356 行）是 AST 的**真正求值器**（短路求值 + SQL LIKE 通配 + NULL 三值逻辑 + 拒绝隐式 coerce），是一份**语义黄金标准**；Spring `SimpleVectorStore` 复用通用 converter 路径。

### 本节小结

- **lynx 反超**：组合+泛型 dispatch 完胜继承模板方法（SQL-string 与 SDK-struct backend 共享骨架）；`$N` 参数化防注入更硬；ISP 切碎；请求级语义校验前置；sealed AST 穷尽；零运行时依赖手写 Pratt parser；observation 解耦；inmemory 语义黄金标准。
- **spring-ai 反超**：~~`IS NULL`/`IS NOT NULL`~~ ✅ **已补齐（2026-05-29）** —— 新增 `is`+`null` 两个 keyword token，`field IS NULL` 复用 `BinaryExpr{IS, field, NULL}`、`field IS NOT NULL` 复用 `UnaryExpr{NOT, …}`（每个 backend 的 NOT handler 免费给出 NOT NULL）；语言层 + inmemory 参考语义 + 14 个 backend 落地：SQL/文档系（pgvector/cockroachdb/supabase/mariadb/oracle/tidb/clickhouse/couchbase/mongodb/neo4j）+ vector-native（qdrant `IsNull` / weaviate `IsNull` / elasticsearch+opensearch `NOT _exists_`）。其余确实不可表达 IS NULL 的 backend（redis/cassandra/pinecone/chroma/milvus 等）按既有惯例优雅拒绝（"unsupported operator"，与"redis 不支持 IN"同机制）。**`NOT IN` 也已补齐**（2026-05-29，`x NOT IN (...)` 中缀，复用 `NOT`+`IN` → `UnaryExpr{NOT, BinaryExpr{IN}}`，所有支持 NOT+IN 的 backend 免费翻译）。德摩根下推 + `expandIn/Nin` 代数 fallback；日期归一化；按 id 删除；数值类型细分；AND/OR 多别名仍是 Spring 领先。
- **取舍而非高下**：backend 广度持平；DSL 表达力是不同子集（lynx 偏通用过滤，Spring 偏 SQL WHERE）。

---

## 6. chatmemory

### 6.1 拓扑对照

| | lynx | Spring AI |
|---|---|---|
| 抽象层 | `core/.../memory.Store` + `InMemoryStore` + `MessageWindowStore` | `ChatMemory` + `ChatMemoryRepository` + `InMemoryChatMemoryRepository` + `MessageWindowChatMemory` |
| 后端 | `chatmemory/{postgres,redis,mongodb,cassandra,neo4j,cosmosdb}/` 各自 go.mod | `memory/repository/{jdbc,cassandra,neo4j,redis,mongodb}/` 各自 maven |

两边哲学一致：**抽象层零 DB 依赖，每后端把 driver 关在自己模块**。

### 6.2 Memory 接口：ISP 切动作 vs 切层级

**lynx** 按*动作* ISP：`Reader`(Read) + `Writer`(Write) + `Clearer`(Clear) → `Store`。**Spring** 按*抽象层级*切：`ChatMemory`(add/get/clear 策略层) + `ChatMemoryRepository`(findConversationIds/findByConversationId/saveAll/deleteByConversationId 持久层)。

关键差异：(1) ~~Spring `findConversationIds()` 能枚举所有会话，lynx 不能~~ —— **已补齐（2026-05-29）**：新增可选的 `memory.Lister{ Conversations(ctx) ([]string, error) }`。**刻意不并入 `Store` union**（保持非破坏 + ISP-pure：消费方 type-assert，不能/不该枚举的 backend 仍满足 `Store`），core `InMemoryStore` + `MessageWindowStore`（转发）+ 6 个 DB backend 全实现（pg `SELECT DISTINCT`、redis `SCAN`、mongo `Distinct`、neo4j `MATCH … RETURN DISTINCT`、cassandra `SELECT DISTINCT`(partition key 高效)、cosmos 跨分区 `SELECT DISTINCT VALUE`）。这是 ops 级显式扫描，hot read/write 路径仍按 conversationID 分区（CLAUDE.md 不变量未破）。(2) Spring `saveAll` 是**全量替换**（先 delete 再 batch insert），lynx `Write` 是 **append**。

### 6.3 Windowing 策略放哪

**lynx**：窗口是**装饰器 Store**（`MessageWindowStore` wrap 另一个 `Store`），**只在 Read 时**滑窗（合并 system + 取最近 N 个非 system），Write 透传——**底层永远全量，窗口只是读时视图**。**Spring `MessageWindowChatMemory`**：窗口在**写时落库**（超 `maxMessages` 物理删除再覆盖）。lynx 的"读时视图 + 底层全量"更安全（不丢历史、可换窗口大小重读），system 消息合并更讲究——**lynx 略胜**。

### 6.4 与 chat 集成：middleware vs advisor

lynx `memory/middleware.go`（call+stream 一对）：conversationID 从 `req.Params` 取，**没 key 就 short-circuit**；用 `SavedMarkerKey`（零大小 struct 写进 metadata）**去重**；流式**消费者中途 cancel 则不持久化半截 AssistantMessage**（"half-streamed content saved as history would lie"）。Spring `MessageChatMemoryAdvisor`：conversationID 从 advisor context 取，缺失 `Assert.hasText` **抛异常**；靠 `before()` 只 add 最后一条 user 消息 + `HashSet` 判重。lynx 去重 marker 更显式、缺 key 更宽容。（注：Spring 这版只剩 `MessageChatMemoryAdvisor`，`PromptChatMemoryAdvisor` 已移除。）

### 6.5 消息序列化到 DB —— lynx 反超 Spring 最大的一处

**lynx**：所有 6 后端走**统一 canonical JSON envelope**（`internal/codec` + 各 Message 类型 `MarshalJSON`），AssistantMessage 把**有序 Parts 数组**带 `kind` discriminator 整个序列化。结果：**TextPart/ReasoningPart/ToolCallPart 顺序、signature、tool call id/name/args、ToolMessage ToolReturns、每条 metadata 全部 round-trip**，且**可跨后端迁移**。

**Spring JDBC repo**：表只有 `content TEXT`（存 `getText()` 纯文本）+ `type VARCHAR(10)` 两个数据列。源码自承：**ToolResponseMessage content 永远存空**（"The content is always stored empty for ToolResponseMessages"，tool 结果丢失）、**AssistantMessage tool calls 丢失**、**metadata 丢失**。Cassandra repo 同样只存 `getText()`。

**结论**：lynx 富 Parts 模型配了真正能 round-trip 它的序列化；**Spring JDBC/Cassandra 是有损存储**（type+text 两列），多模态/reasoning/tool call/return/metadata 全丢。对 agent 场景（tool 历史完整喂回下一轮）这是 Spring 硬伤。

### 6.6 SPI 形态 / 排序键 / ctx

lynx 每后端 `var _ memory.Store = (*Store)(nil)` + `StoreConfig.Validate/ApplyDefaults` + `InitializeSchema` 开关 + `identPattern` 防注入 + **全局单调排序键**（postgres BIGSERIAL / cassandra TIMEUUID / mongo UnixNano+i，显式 ORDER BY seq）+ 每方法首行 `ctx.Err()` + OTel span。Spring builder + dialect 自动探测 + 固定表名，但**排序键用 `epochSecond++`（秒级，脆弱）**、**无 ctx 取消**、用 slf4j 日志。**lynx 反超**排序键正确性 + ctx + 可观测性；**Spring 反超** dialect 自动探测的开箱体验。

### 6.7 后端广度

lynx 6 个独立后端（含 **Cosmos DB**——Spring 已删，Redis 带 TTL）。Spring 的 **jdbc 一个实现靠 dialect 覆盖 8 种 SQL**（postgres/mysql/mariadb/sqlite/h2/hsqldb/oracle/sqlserver）+ redis/cassandra/neo4j/mongo——**关系型 SQL 广度更大**。

### 本节小结

- **lynx 反超**：**序列化保真度（最大优势）**——完整 round-trip Parts/ReasoningPart/ToolCall/ToolReturns/metadata，Spring JDBC&Cassandra 有损（tool 结果丢失，源码自承）；排序键正确性（BIGSERIAL/TIMEUUID vs `epochSecond++`）；ctx 协作取消；OTel span；windowing 不丢历史；SQL 注入防护 + Redis TTL + Cosmos DB。
- **spring-ai 反超**：关系型 SQL 广度（单 jdbc + dialect 覆盖 8 库）；dialect 自动探测 + builder 开箱省事。（注：`findConversationIds` 会话枚举已由可选 `memory.Lister` 补齐，见 §6.2。）
- **gap**：lynx 缺"列举会话 id / 全量替换"能力；关系型只押 Postgres 一个（追 SQL 广度需 dialect 抽象，但与"每后端独立 go.mod"拓扑冲突）。

---

# 第三部分 · 工具、检索、读取、协议、观测

## 7. tools

### 7.1 根本定位：batteries-included vs SPI-only

- **Spring AI 只发 SPI**。`spring-ai-model/.../tool/` 全是机制层：`ToolCallback` + `ToolDefinition`/`ToolMetadata` + `@Tool`/`@ToolParam` + `MethodToolCallback`/`FunctionToolCallback` + `JsonSchemaGenerator` + `ToolCallbackResolver` + `ToolExecutionExceptionProcessor`。**全仓库 grep 不到任何内置可执行工具**（`AnthropicWebSearchTool` 是描述符 POJO，声明 Anthropic 服务端原生 search，Spring 自己不执行）。立场："工具内容你自己写，我只负责接到 LLM"。
- **lynx 把具体工具当产品发**。`tools/` 独立 module 出货 **bash / fs(read+write+edit+glob+grep) / websearch(7 backend) / webfetch(4 backend) / httpreq / fakeweather** 一整套开箱即用、对标 Claude Code 的实现。**最大差异点**。

### 7.2 工具定义 & schema 生成

**Spring：反射 + 注解，运行时生成**。`@Tool` 标普通方法，`JsonSchemaGenerator.generateForMethodInput(Method)` 用 victools+Jackson+Swagger 反射方法签名生成 schema。优点零样板，代价是重度反射 + 编译期无保证 + 依赖 `-parameters`。

**lynx：显式 struct + 编译期 schema**。每工具定义 LLM 朝向的 `Request` struct（字段用 `jsonschema:"required"` tag），**包级 `var` 初始化时**算出 schema：

```go
var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})   // bash/tool.go:31
```

`Definition()` 手写 Name/Description（精心打磨的 prompt，照搬 Claude Code 风格），`Call` 自己 unmarshal→executor→marshal。每工具 `var _ chat.Tool = (*Tool)(nil)` 编译期断言。两边 schema 都靠反射，区别在 lynx 反射的是**专门 DTO struct** 而非业务方法——解耦 LLM 契约和执行逻辑。

### 7.3 工具解析 / 内置工具 / 可插拔 backend

**解析**：Spring 有 `ToolCallbackResolver`（`StaticToolCallbackResolver` / `SpringBeanToolCallbackResolver` 从容器按 bean 名查）为 DI 设计；lynx 无 resolver 无 DI，`NewToolSupport().Register(...)` 构造即注册（`ToolRegistry` 是 thread-safe map）。CLAUDE.md 明确 ❌ 全局 tool registry（多 agent 各管各 toolset）。

**内置工具（lynx 核心差异化，Spring ship 0 个）**：
- **fs**（5 工具，两层 SPI：`Executor` 5 方法 + Tool 层 JSON↔Go）。`LocalExecutor` 干的脏活很实在：Read 做 binary 检测/CRLF→LF/BOM 剥离；Write/Edit 做 per-path `sync.Mutex` 串行化 + atomic rename + 保留 CRLF/BOM/mode + exact-string 唯一性校验；Glob shell out 到 `find`；Grep 优先 ripgrep fallback GNU grep（三种 output mode + context 行）。
- **bash**：`/bin/sh -c`，`boundedBuffer` 双流各 30KiB 截断（mirror Claude Code），非零 exit 不算 error，timeout/cancel → `Killed=true`。
- **httpreq**：allowlist 守卫（`AllowedHosts` 必填支持 `*.` 通配、写方法须显式 opt-in、256KiB cap），空 host `ErrMissingHosts`。

**可插拔 backend（ISP/strategy，Spring 无对应）**：`websearch`/`webfetch` 是教科书级窄接口 + 多实现：

```go
type Provider interface { Name() string; Search(ctx, *Request) (*Response, error) }
```

websearch 7 backend（brave/exa/firecrawl/jina/perplexity/serper/tavily），webfetch 4 backend。Tool 层完全不知道背后是谁，每 backend 把各家 API 映射到统一 `Response`，LLM 看到的形状恒定。Spring 连搜索工具都没有。

### 7.4 错误处理 / ToolContext

**错误**：Spring `DefaultToolExecutionExceptionProcessor` 内置成熟策略（allowlist rethrow / `alwaysThrow` / **失败转 message 字符串喂回 LLM**）+ `ToolCallResultConverter`。lynx 工具层只 typed sentinel error + `%w` + OTel span（标 `lynx.tool.is_error`），**降级策略下放 agent 层**。Spring 这块更"成品"，lynx 更"分层不替你决定"。

**ToolContext**：Spring 有显式 `ToolContext`（per-call 注入 user id/tenant，`JsonSchemaGenerator` 跳过该参数）。lynx 无独立 ToolContext，状态/配置在**构造工具时**通过 executor 注入（`fs.NewLocalExecutor(root)` 把 jail root 焊进），运行期数据走 `ctx`。两种哲学。

### 本节小结

- **lynx 反超**：**内置工具是降维优势**（bash/fs/websearch/webfetch/httpreq 开箱即用且实现质量高，对标 Claude Code）；websearch/webfetch Provider strategy（窄接口 + 7/4 backend）；两层 SPI 分离；每工具 OTel span。
- **spring-ai 反超**：`@Tool` 注解零样板；内建错误降级策略（失败转文本喂回 LLM）；`ToolContext` per-call 注入；resolver + DI 集成；`MethodToolCallback`/`FunctionToolCallback` 把任意 lambda 转工具通用性更强。
- **gap**：两边都靠反射生成 schema（无编译期代码生成）；lynx fs `LocalExecutor.Root` path-jail 仍是 TODO；内置工具空缺/注解空缺各是双方的**设计选择**（"batteries-included 工具库" vs "纯 SPI"），不是同维度优劣。

---

## 8. rag

> 两者都是 spring-ai 模块化 RAG 论文（arXiv:2407.21059）的实现，stage 拆分/prompt/空响应处理几乎逐字对应。lynx 是**忠实移植 + 工程加固**。

### 8.1 管线形态：扁平 middleware vs 四层 advisor

**Spring**：`RetrievalAugmentationAdvisor implements BaseAdvisor`，整套流程写死在 `before()` 一个方法（7 步），包目录本身就是四层语义层级（preretrieval/retrieval/postretrieval/generation）。

**lynx**：拆成 `Pipeline`（纯编排，`Execute` 返 `(*Query, []*document.Document, error)`，**与 chat 无关、可独立测试**）+ `pipelineMiddleware`（接到 `chat.CallMiddleware`/`StreamMiddleware`，薄 adapter）。**架构干净度 lynx 略胜**（Spring 把编排和接入 ChatClient 焊死）；**语义可发现性 Spring 略胜**（四层目录对照论文）。

### 8.2 Stage 逐项对应

三个 transformer（rewrite/translation/compression）默认 prompt **与 spring-ai 一字不差**，augmenter 两段模板（context block + empty-context 拒答）也逐字一致——明确的移植关系。

**Retriever 差异**：默认 filter，Spring 用 `Supplier<Filter.Expression>`（无参）；lynx 用 `FilterFunc func(ctx, params)(ast.Expr, error)`——**lynx 更强**，拿到 ctx + query 的整个 `Extra` map 能动态算 filter。MinScore 校验 lynx 强制区间，Spring 只 `>= 0`。

### 8.3 Query 模型

```go
type Query struct { Text string; Extra map[string]any }       // lynx
```
```java
public record Query(String text, List<Message> history, Map<String, Object> context) {}  // Spring
```

**Spring 把 conversation history 提升为一等强类型字段**（`List<Message>`）；lynx 把 message list 塞进 `Extra` 的 `ChatHistoryKey`，compression transformer type-assert 取出。**Spring 在类型安全上领先一档**（history 这种几乎必用的字段在 lynx 退化成 `any` + runtime assert）；lynx 在简洁/可扩展上领先。

### 8.4 Refiner 链 vs Joiner + PostProcessor（结构分歧最大）

Spring 拆两阶段：`DocumentJoiner`（retrieval 层，吃 `Map<Query, List<List<Document>>>` 保留 per-query 维度，dedup + score 降序，**有实现**）+ `DocumentPostProcessor`（postretrieval 层，rerank/去冗余，**spring-ai-rag 模块里无任何实现，只有空接口**）。

lynx 取消 Joiner，retrieve 阶段 `parallelCollect` 直接 union 成扁平 slice，然后用可串联 `DocumentRefiner` 链，**自带两个实现**：`DeduplicationRefiner`（by ID keep first 保序）+ `RankRefiner`（score 降序 + top-K）。

**开箱即用 lynx 领先**（dedup/rank 有实现 vs Spring 空接口）。**Spring 唯一结构性优势**：保留 `Map<Query, List<List<Document>>>` 是 **RRF 的前提**（需知道每个 doc 在每个 query 的 rank）；lynx 在 `parallelCollect` 即 union 成扁平 slice，**结构上排除了 RRF**——所以"不做 RRF"不只是 YAGNI，也是数据结构选择的必然。

### 8.5 RRF / 并行 / 可观测性

- **RRF**：**两边都没实现**。Spring `DocumentJoiner` 注释承诺、唯一实现（Concatenation）只做 dedup + sort；lynx 判定 YAGNI 且数据结构已排除。
- **并行**：Spring `CompletableFuture` 只对 expanded queries 并行（单层），任一异常炸整个 `before()`。lynx 泛型 `parallelCollect[Item,Out]` **两层并行**（queries × 每 query 内 retrievers）+ `errgroup` + **部分失败容错**（全失败才返 error）——**lynx 工程质量最突出处**。
- **multi-query 解析**：lynx LLM 多吐/少吐一行不废整次（鲁棒）；Spring 严格行数相等否则整个放弃（脆）。
- **可观测性**：lynx 每阶段独立 OTel span + doc/query count；**Spring RAG 组件只有 slf4j 日志、不产生 per-stage span**（靠外层 ChatClient observation 兜底）。**stage 级可观测性 lynx 明确领先**。

### 本节小结

- **lynx 反超**：编排/接入解耦；后处理 dedup+rank **有实现**（Spring 空接口）；两层并行 + 部分失败容错；multi-query 解析鲁棒；stage 级 OTel span；Retriever `FilterFunc(ctx, params)` 更灵活；config-struct + 编译期断言。
- **spring-ai 反超**：Query 强类型 history；保留 per-query 维度（为 RRF 留路）；advisor 有 `order`（多扩展点协作）。
- **共同 gap**：RRF / 查询路由 / per-doc 内容压缩 两边都没做。**整体 lynx 是忠实移植 + 工程加固**，Spring 仅在两处结构设计上更前瞻（恰是 lynx 用 Extra map + 扁平化换简洁的清醒取舍）。

---

## 9. documentreaders

> lynx `documentreaders/`（markdown/html/pdf 三子模块 ~700 LOC）vs Spring `document-readers/`（markdown/jsoup/pdf×2/tika）。

### 9.1 Reader SPI 契约

lynx `Reader.Read(ctx) ([]*Document, error)`——接 `io.Reader`、返 `error`（全 `%w` 包装）、带 `ctx`（虽三个 reader 当前都忽略 ctx，是小 gap）。Spring `DocumentReader extends Supplier<List<Document>>`——无 ctx、`throw RuntimeException` 吞异常、绑 `Resource` 体系（但 `PathMatchingResourcePatternResolver` 支持 glob 批量读多文件）。**SPI 干净度 lynx 领先**。

### 9.2 Markdown：goldmark vs commonmark

切分粒度是最大分歧：lynx 默认整篇一个 Document，`WithHeadingSplit(maxLevel)` 按 heading 切且**维护 heading path 栈**（`"Intro > Architecture"` 写进 `markdown.heading.path`），body 从 AST byte segment 还原原始 markdown。Spring 用 `AbstractVisitor` 每遇 Heading 就 flush（粒度固定不可调），但对 code block/blockquote/inline code 有**语义分类元数据**（`category`/`lang`）。**lynx 反超** heading path 栈（层级上下文，Spring 完全没有）+ 切分级别可调；**Spring 反超** code 语义分类更细。

### 9.3 HTML：goquery vs jsoup

lynx 默认抓整篇 body，`WithSelector` per-element；亮点是 `extractText` **先 Clone 再删 `script,style,noscript,template,head`** 防 JS/CSS 混进 embedding + `collapseWhitespace`，抓 `html.title`/`description`/**canonical**。Spring 三种模式 + `metadataTags`（任意 meta 标签）+ `includeLinkUrls`。**lynx 反超** 主动剔除 script + canonical；**Spring 反超** 任意 metaTags + 抓全部链接 + selector join 中间模式。

### 9.4 PDF：ledongthuc/pdf vs PDFBox（差距最大）

lynx 单策略（纯文本提取，整篇拼接或 `WithPerPage` 切页，支持 `WithPassword`，单页 error 不致命），**无版面、无 TOC、无分栏感知**。Spring 双 reader：`PagePdfDocumentReader`（**版面感知** `PDFLayoutTextStripperByArea` + 页边裁切 + 多页分组）+ `ParagraphPdfDocumentReader`（用 **PDF 目录大纲 TOC bookmark** 切分，输出**带层级**的 Document）。**Spring 在 PDF 上全面反超**（多栏不乱序、TOC 结构化切分）；代价是体量极大（`layout/` 6 个手写几何类 + PDFBox 重依赖）。lynx 遇多栏复杂版面会乱序。

### 9.5 Tika：Spring 有，lynx 故意不做

Spring `TikaDocumentReader` 用 `AutoDetectParser` 支持 PDF/DOC/DOCX/PPT/PPTX/HTML/RTF/ODT 几十种格式——**万能兜底**。lynx 刻意不做：Apache Tika 是重型 JVM 库（POI/PDFBox 一大串传递依赖），Go 侧无等价物（cgo 绑 native 或起子进程都违背纯 Go/in-process 定位）；DOCX/PPTX 真要支持可纯 Go 单独写 `documentreaders/docx/`。**有意 gap 而非缺陷**（符合反向不变量"不引重依赖"），代价是 Word/PPT/Excel 当前完全无解。

### 本节小结

- **lynx 反超**：SPI 干净（io.Reader/error/ctx/不吞异常）；**markdown heading path 栈**；HTML **主动剔除 script/style** + canonical；依赖轻、纯 Go 跨平台；元数据 key 带命名空间前缀。
- **spring-ai 反超**：**PDF 碾压**（版面几何重建 + 页眉页脚裁切 + **TOC bookmark 结构化切分带层级**）；**Tika 万能 reader**（Word/PPT/Excel...）；markdown code 语义分类；HTML 任意 metaTags + 链接抓取；通用 `additionalMetadata` 注入点；glob 批量读。
- **gap 性质**：多数是 lynx 有意识权衡（不引 Tika、PDF 不做版面因 Go 生态无成熟 layout stripper）；**两项真 gap 已补齐（2026-05-29）**：(1) ✅ `ctx` 现在真正生效——三个 reader 都在入口 + 逐页/逐段/逐元素循环 check `ctx.Err()`；(2) ✅ 三个 reader 都加了 `WithMetadata(map)` 注入口（reader-namespaced 键优先，map 深拷贝防泄漏）。PDF 版面/TOC 与 Tika 属能力深度差距，需单独造轮子才能追平（仍未做）。

---

## 10. mcp

> 两者都是**官方 MCP SDK 的适配层**——lynx 包 Go SDK（`modelcontextprotocol/go-sdk` v1.6.0），Spring 包 Java SDK——都**不自己实现 JSON-RPC / 协议状态机**。

### 10.1 体量差 ~10x 溯源

lynx **1 个 package / ~1.3k LOC / 13 文件**覆盖 client+server 双向桥接；Spring **4 个 Maven 模块 / ~185 main Java 文件**（common 18 + annotations 167 + 两 transport 模块）。三个根因：

1. **Sync/Async 双轨**：`SyncMcpToolCallback` + `AsyncMcpToolCallback`、provider 两份、`toSyncToolSpecification`/`toAsyncToolSpecification`/`toStatelessSync`/`toStatelessAsync` 同逻辑写 4 份。lynx 只有 sync，异步交给上层 agent runtime。
2. **annotations 模块的 sync×async×stateless 矩阵**：75 个 method callback + 43 个 provider，覆盖 tool/prompt/resource/complete/sampling/elicitation/logging/progress 八种能力 ×4 变体。lynx **只做 tool 一种能力的双向桥**，prompt/sampling 是薄 helper，resource/complete 不做。
3. **Server transport 自实现**：Spring 把 SSE/Streamable/Stateless × WebMvc/WebFlux 作为独立模块自实现；lynx 直接复用 go-sdk 的 `NewStreamableHTTPHandler`，`transport.go` 只是 option 翻译。

### 10.2 Client 侧（消费远端 MCP tool）

lynx `Provider.Tools(ctx) ([]chat.Tool, error)` 跨多 `Source` fan-out（go-sdk `iter.Seq2[*Tool, error]` 流式迭代），缓存用 `atomic.Pointer[[]chat.Tool]` + `sync.Mutex` double-checked，`tools/list_changed` 接到 `OnToolListChanged` 触发 `Invalidate()`。错误分流：远端 `IsError` → `*ToolCallError`（给 LLM），transport 失败 → `%w`（`errors.As` 区分）。

Spring `SyncMcpToolCallbackProvider`/`AsyncMcpToolCallbackProvider` → `ToolCallback`，缓存用 `volatile boolean` + `ReentrantLock`，`list_changed` 走 **Spring `ApplicationListener<McpToolsChangedEvent>`**（耦合事件总线）。错误**远端 isError 和 transport 异常都包成 `ToolExecutionException`**（粒度更粗）。`AsyncMcpToolCallback.call` 异步底层但同步出口（最后 `.block()`）。

### 10.3 Server 侧 / Transports / 反向能力

**Server**：lynx `RegisterTools(server, tools...)` 用 go-sdk 低层 `server.AddTool`（**故意避开泛型 `AddTool[In,Out]`** 因反射会覆盖手写 schema），handler 把 `(string, error)` 翻成 `CallToolResult`（失败 → `IsError:true` + 文本，**绝不返回 Go error** 否则被提升为协议错误对 LLM 隐藏失败）。ISP：`tool.ToolSource` 单方法接口。Spring `toSyncToolSpecification` 等 4 变体，handler 行为一致（异常翻 `isError`），额外支持 image mimeType → `ImageContent`。

**Transports**：lynx 把"server transport"降维成 HTTP handler + option 翻译（Go `http.Handler` 通用契约挂任意 router）；Spring 因要分别适配 WebMvc（servlet/阻塞）和 WebFlux（reactor）两套 web 栈，被迫自实现 6 个 transport provider——**Go `net/http` 统一抽象 vs Java servlet/reactive 分裂的直接代价**。lynx 不提供 legacy SSE server（已弃用）。

**反向能力**：lynx = 包级 free function + context 注入（`WithServerSession(ctx, session)`，tool 作者调 `ReportProgress`/`ElicitFromClient`/`LogToClient`，无 session 时 `ErrNoServerSession` sentinel → benign no-op，tool 代码无需 MCP-aware 分支）。亮点 `SamplingViaChatClient`——**MCP server 借 client 的 LLM、自己不持凭证**。Spring = 注解 + 富 `McpSyncRequestContext`（25+ 方法接口，六种反向能力做成实例方法 + 类型化 elicit `StructuredElicitResult<T>`），覆盖面更全（含 roots/ping/complete）但接口臃肿 + DI 深绑。

### 10.4 注解 vs 构造器

Spring `@McpTool`（带 readOnly/destructive/idempotent/openWorld hints）+ `McpJsonSchemaGenerator` 反射生成 schema + `spring/scan/` 启动期扫描 bean。lynx 无注解无反射，手写 `chat.ToolDefinition{Name, Description, InputSchema}` + `RegisterTools` 一行。**"Spring magic / 启动期反射" vs "Go 显式装配 / 编译期确定"**。

### 本节小结

- **lynx 反超**：体量/认知负担（1.3k LOC vs ~185 文件，靠"只做 tool 桥 + sync-only + 复用 go-sdk transport"砍掉所有维度）；错误分流精确（`*ToolCallError` vs `%w`，Spring 一律 `ToolExecutionException`）；反向能力 Go idiom（context 注入 + sentinel，tool 无需 MCP-aware 分支 + 类型安全）；sampling 借用 client LLM；零反射零 magic。
- **spring-ai 反超**：能力覆盖面（resources/prompts/complete/roots/ping 全做）；async 一等公民（真 reactor 栈）；DX（`@McpTool` + 自动 schema + bean 扫描）；server transport 灵活度（WebMvc/WebFlux/Stateless + 富 hint）；富 `RequestContext` + 类型化 elicit。
- **gap（多为 YAGNI 取舍）**：非文本 content（受限于 chat.Message text-first schema）；resources/prompts(list)/completion 协议未覆盖；`_meta` 仅覆盖 outbound CallTool。lynx 精准只覆盖"agent 调远端 tool / 把 agent tool 暴露给外部"主干。

---

## 11. otel：OTel-native vs Micrometer Observation

### 11.1 哲学：直连 OTel vs vendor-neutral 抽象层

**lynx** 直接消费 OTel Go SDK，无中间抽象。每模块 `var xxxTracer = otel.Tracer("lynx/...")`，instrumentation 直接落在 `trace.Span`/`attribute.KeyValue`/`codes.Error`，未配 `TracerProvider` 时编译期 noop 零成本，attr key 直接用 GenAI semconv 字符串（collector/auto-instrumentation 零 wiring 识别）。

**Spring AI** 依赖 **Micrometer Observation API**——"记录一次，多处分发"（同时驱动 Metrics 和 Tracing → 桥到 OTel/Brave）。call site 只看到 `ObservationRegistry`：

```java
VectorStoreObservationDocumentation.AI_VECTOR_STORE
    .observation(customConvention, DEFAULT_CONVENTION, () -> ctx, observationRegistry)
    .observe(() -> this.doAdd(documents));
```

lynx 把 OTel 当事实标准直接绑定（再加 vendor-neutral 是 YAGNI）；Spring 把 OTel 当 backend 之一，用 Micrometer 解耦（代价是多一层 indirection）。

### 11.2 Instrumentation 机制

**lynx**：每模块一个 `tracing.go` 手写 span 生命周期，模式一致（`startXxxSpan` 塞 request attr → `finishXxxSpan` 塞 response attr + `End()`，错误三连 `RecordError` + `SetStatus(codes.Error)` + `End()`）。命令式、显式、贴调用点。

**Spring**：声明式三件套——`ObservationContext`（承载状态）+ `ObservationConvention`（翻译成 `KeyValues`，**显式分 low-cardinality**（进 metric tag）**vs high-cardinality**（只进 trace））+ `ObservationRegistry` + `ObservationHandler`（运行时 fan-out）。**关键优势：cardinality 显式分层**（强制区分哪些 attr 能进 metric tag 防爆炸）+ Convention 可继承覆盖单个 attr（OCP）。lynx 全部 attr 塞 span（无 metric 所以无此区分，既是简洁也是缺口）。

### 11.3 语义约定 / Logging / Metrics

- **semconv**：attr key 字符串几乎逐字相同（都来自 GenAI semconv）。lynx 散落各模块 `const`，Spring 集中枚举 `AiObservationAttributes`/`AiOperationType`/`AiTokenType`。lynx 有轻微命名漂移（`lynx.tool.is_error` vs `lynx.mcp.tool.is_error`）。Spring response 侧多 cache token + total_tokens。
- **Logging（两个相反方向）**：lynx **禁用 stdlib log/slog**，内部 logging 走 OTel span（"log 即 span"）；`otel/log` + `otel/slog` 是**唯一例外**——OTel → stdlib 的**导出桥（bridge OUT）**，把 finished span 渲染成单行 logfmt（保留 `gen_ai.*`/`lynx.*` 原 key，error span 升 `LevelError`，永远返回 nil 不污染业务）。Spring 反向：SLF4J 日志 + `TracingAwareLoggingObservationHandler` 把 trace context 推进 scope 做 log correlation。**lynx = trace 唯一真相，log 是 trace 的可选导出；Spring = log/trace 两套，handler 关联**。
- **Metrics（2026-05-29 已补齐）**：Spring `ChatModelMeterObservationHandler` 每次 chat 结束用 Micrometer `Counter` 累加 token 计数器（`gen_ai.client.token.usage`）。lynx 原先无 metrics（token usage 只作为 span attribute），现已在 `core/model/metrics.go` 落地 **GenAI semconv metrics**：`gen_ai.client.token.usage`（Int64Histogram，token.type=input/output）+ `gen_ai.client.operation.duration`（Float64Histogram，秒，失败带 error.type），由 chat / embedding 的 `record*Metrics` 在每次调用结束时上报，全部走 low-cardinality tag（operation/system/request.model/response.model）。注意 lynx 选 **Histogram**（OTel GenAI semconv 的规范 instrument 类型，sum 聚合即得总 token，count 即请求数），而非 Spring 的 Counter——更贴 semconv 且同样可聚合。与 tracer 一样，未配 `MeterProvider` 时 instrument 是 noop，零成本。

### 11.4 覆盖面

| 子系统 | lynx | Spring |
|---|---|---|
| chat / embedding / tool / vectorstore | ✅ | ✅ |
| RAG pipeline | ✅（5 阶段各 span） | ❌（advisor 自己埋） |
| MCP | ✅ | ❌ N/A |
| chatmemory | ✅（DB semconv） | ❌ |
| image | ❌ | ✅ |
| **metrics** | ✅ token usage + duration histogram（已补） | ✅ token counter |

lynx trace 在 RAG/MCP/chatmemory 上**比 Spring 更广**；metrics 现已对齐（chat/embedding 的 token usage + duration）；Spring 仅在 image tracing 上仍更全。

### 本节小结

- **lynx 反超**：架构纯度与轻量（直连 OTel 无 indirection，~270 LOC + 几个 tracing.go vs Spring 四件套，认知负担低一个数量级）；logging 模型更干净（log 即 span 渲染，`otel/slog` 桥不丢 attr）；trace 覆盖更广（RAG/MCP/chatmemory）；noop 默认零成本。
- **spring-ai 反超**：**cardinality 显式分层**（metric tag 不爆炸——lynx 现在靠 `OperationMetrics` 只放 low-cardinality dims 做到同等效果，但没有框架级强制）；Convention 可覆盖（OCP）；vendor-neutral（换 Brave/Zipkin 零改动）；semconv 集中治理；cache token + stream + tool_names attr lynx 未覆盖。
- **gap（按性价比）**：(1) ~~Metrics~~ ✅ 已补——`core/model/metrics.go` 落地 `gen_ai.client.token.usage`（Int64Histogram）+ `gen_ai.client.operation.duration`（Float64Histogram），chat/embedding 接入；(2) attr key 集中化消除命名漂移；(3) 补 cache token + stream + tool_names attr；(4) image tracing + metrics（当前仅 chat/embedding 上报 metrics）。Convention 不可覆盖、vendor lock-in OTel **是设计选择而非缺口**（按 YAGNI 信条，OTel 已是标准、当前无多 backend 需求）。

---

# 第四部分 · 综合定档

## 12. 跨模块综合判断

### 12.1 lynx 在哪些维度系统性领先

1. **接口纪律（贯穿所有模块）**：ISP 切碎（vectorstore/memory/tokenizer/mcp）+ 接口在消费方定义 + 全 SPI 带 `ctx`+`error`。这是 lynx 相对 spring-ai 函数式接口 + 单胖接口 + `throw RuntimeException` 的一致优势，直接来自 CLAUDE.md 的设计纪律。
2. **Reasoning 一等公民的复利**：`ReasoningPart{Text, Signature}` + `Usage.ReasoningTokens` 不是孤立特性，它让 chat accumulator（无魔法字符串）、structured parser（无 `<think>` 清理）、models 适配器（signature 续流）、chatmemory（完整 round-trip）四个模块同时受益。spring-ai 至今靠硬编码 vendor 字符串打补丁。
3. **组合 + 泛型分发**：`filterhelp.DispatchBinary[T]`、`ResponseAccumulator` type-agnostic、`parallelCollect[Item,Out]`——一致地用组合 + Go 泛型替代 Java 的继承模板方法，且换来更强能力（同骨架出 SQL 字符串或 SDK 对象）。
4. **扩张成本曲线**：models 的 OpenAI-compat 复用（5 行 vs 千行）+ 跨协议适配器 + 一个 sibling 目录覆盖多模态，远低于 spring-ai 的 artifact 三件套 + autoconfigure + AOT。
5. **序列化保真 + 可观测覆盖**：chatmemory 完整 round-trip 富 Parts（Spring 有损）；OTel trace 覆盖 RAG/MCP/chatmemory（Spring 缺）。

### 12.2 spring-ai 在哪些维度系统性领先

1. **产品化兜底**：Document 自动 id、chunk 溯源 metadata、dialect 自动探测——这些"用户不用操心"的细节 spring-ai 更全。
2. **能力深度的成熟轮子**：PDF 版面/TOC 解析、Tika 万能 reader——需要造大轮子才能追平。（metrics 已补齐，见 §11；cardinality 分层 lynx 靠 `OperationMetrics` 约束达成同等效果。）
3. **类型安全的 vendor 参数 / 100% vendor 能力覆盖**：typed `*ChatOptions` + 每 vendor hand-roll 让 Anthropic citations/skills、Bedrock cache 等专属能力有专门类；lynx 的 Extra map + compat 复用下沉到最小公分母。
4. **框架级开箱即用**：autoconfigure/starter/BOM/DI/`@Tool`/`@McpTool` 注解 + async 一等公民——这是 framework vs library 的定位差异，多数对 lynx 是"故意不做"。
5. **协议/DSL 完整度**：MCP resources/prompts/complete 全覆盖；filter DSL 的日期归一化（IS NULL / NOT IN 已对齐）。

### 12.3 lynx 真实 gap 优先级表

| 优先级 | gap | 模块 | 性质 | 工作量 |
|---|---|---|---|---|
| ✅ 已闭合 | OTel metrics（token usage + operation duration histogram）| core/model + otel | `core/model/metrics.go`：`gen_ai.client.token.usage` + `gen_ai.client.operation.duration`，chat/embedding 接入，low-cardinality tag（2026-05-29） | — |
| ⤳ 非缺口 | embedding token-aware batching | core/document | 已由 `document.TokenCountBatcher`（document 层 `Batcher`）覆盖——`Batch` 后逐批 embed；不在 embedding 侧重复造抽象 | — |
| ✅ 已闭合 | Document 自动 id + chunk 溯源 metadata（parent_id/chunk_index/chunk_total + score 透传）| core/document | `Document.EnsureID` + `IDAssigner` + splitter `IDGenerator` + lineage 元数据（2026-05-29） | — |
| ✅ 部分闭合 | filter `IS NULL`/`IS NOT NULL` | core/filter + vectorstores | 语言层 + inmemory + 10 backend 落地（复用 BinaryExpr+NOT）；vector-native/不可表达的 backend 优雅拒绝（2026-05-29） | — |
| ✅ 部分闭合 | vectorstore 按 id 删除 | core/vectorstore + 20 backend | 新增可选 `vectorstore.IDDeleter`（不入 `Store` union，ISP-pure 非破坏），core inmemory + pgvector(含 cockroachdb/supabase 别名) + 16 backend 落地；niche 实验后端优雅未实现（2026-05-29） | — |
| ✅ 部分闭合 | chatmemory 会话枚举（`findConversationIds` 对位） | core + chatmemory | 新增可选 `memory.Lister`（不入 `Store` union，ISP-pure 非破坏），core InMemory + 6 backend 全实现（2026-05-29）。关系型 dialect 广度仍未做 | — |
| ✅ 已闭合 | documentreaders `ctx` 真正生效 + `WithMetadata(map)` 注入口 | documentreaders | 三 reader 入口+循环 check `ctx.Err()` + `WithMetadata` 选项（2026-05-29） | — |
| **P2** | `Content`/`MediaContent` 统一抽象（Document/Message 共享）| core | DRY | 中 |
| **P2** | Usage 铺满 image/audio/moderation 模态 metadata | core | 计费完整性 | 小 |
| **P3** | streaming 结构化输出；image 多生成；attr key 集中化消除漂移 | 多 | 长尾 | 小-中 |

### 12.4 故意不做（"为什么不抄"）

| # | spring-ai 有但 lynx 不做 | 原因 |
|---|---|---|
| A | autoconfigure / starter / BOM / DI 容器 | lynx 是 library；Go 无 DI 自动装配传统 |
| B | Micrometer Observation 抽象层 | OTel 已是 Go 世界事实标准，再包 vendor-neutral 是过度设计 |
| C | `@Tool` / `@McpTool` 注解 | Go 无 runtime annotation；显式 struct/构造路线更可查 |
| D | Tika document reader | 重型 JVM 依赖；纯 Go 路线宁可单独写 `docx/` |
| E | sync/async MCP 双轨 | Go goroutine + `iter.Seq2` 单路径已覆盖，异步交上层 |
| F | StringTemplate(ST4) 完整模板引擎 | `text/template` 标准库 + `{{.key}}` 够用 |
| G | RRF DocumentJoiner + 保留 per-query 维度 | 当前 retriever 都向量同尺度；待非向量 retriever 落地再做 |
| H | retry `Transient`/`NonTransient` 分类 | SDK 内部已自带重试，再加一层重复 |
| I | GemFire / Coherence vector store | Java 强生态，Go SDK 缺位 |
| J | transformers / postgresml 本地推理 vendor | Go 调 Python ML runtime 是异常路径，用户该用 ollama |

## 13. 一句话定档

**逐文件看下来，结论比"vendor 数量盘点"更扎实：lynx 在*接口设计 / Go idiom / Reasoning 建模 / 组合优于继承 / 扩张成本曲线 / 序列化保真 / 可观测覆盖*这些"架构与协议层"维度系统性领先 spring-ai 主干；spring-ai 在*产品化兜底 / 成熟能力轮子（PDF/Tika/metrics）/ vendor 专属能力 100% 覆盖 / 框架级开箱即用*这些"成熟度与生态深度"维度系统性领先。两者的差距高度可预测——它就是 `thin-library + Go 泛型 + OTel-native` 路线对 `batteries-included framework + DI + Micrometer` 路线的必然投影。** 原本唯一影响生产可用性的硬缺口 **OTel metrics 已于 2026-05-29 补齐**（`gen_ai.client.token.usage` + `operation.duration` histogram）；连同 Document 自动 id / chunk 溯源 metadata，三项 P0/P1 gap 已闭合。其余 P1/P2 gap（filter `IS NULL`、按 id 删除、`findConversationIds`、SQL dialect 广度）都是低成本可补的产品化细节，不涉及架构返工。

---

*对比结束。lynx HEAD `e480bc7`（branch lyra），对照 spring-ai main HEAD `7969b4d97`，2026-05-29。本文由逐模块深度审计（11 个并行 code-level 审计 pass）综合而成；配套文档 `doc/SPRING_AI_COMPARISON.md`（上一版，偏盘点）、`doc/ABSTRACTIONS_COMPARISON.md`（agent 对 embabel）。*
