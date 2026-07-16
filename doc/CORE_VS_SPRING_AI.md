# Core vs Spring AI —— 移植对照与设计取舍

> lynx `core` 的协议层最初移植自 **Spring AI** 的 `spring-ai-model` / `spring-ai-commons` / `spring-ai-client-chat` / `spring-ai-vector-store`。本文逐块对照两者,并说明 lynx 在哪里**按 Go 第一性重新决策**、以及**为什么**。
>
> 对照基准:Spring AI `main` @ `0df8a450`(2026-07-15,`org.springframework.ai`,`@since 2.0.0` 迁移中);lynx `core` 当前分支。
>
> 结论先行:core **不是逐类翻译**,而是把 Spring AI "泛型三层契约 + builder + marker interface + Reactor + Micrometer 内嵌 SPI + ANTLR + retry 分类"这套 Java/Spring 形态,**换成** "tagged-value + 值语义 + `iter.Seq2` + 一组能力对称的函数式 middleware + 手写 scanner + 边界 `Validate` + 依赖预算 fail-fast" 的 Go 形态。名字偶有相同,只因独立评估下它最优,不为兼容或省迁移。

---

## 0. 模块对应关系

| Spring AI 模块 | 移植到 lynx | 说明 |
|---|---|---|
| `spring-ai-model`(Model SPI + chat/embedding/image/audio/moderation) | `core/chat`、`core/embedding`、`core/image`、`core/speech`、`core/transcription`、`core/moderation` | 扁平领域包,不用 `core/model/<modality>` 的 Java 式层次 |
| `spring-ai-commons`(Content/Media/Document/Metadata) | `core/media`、`core/document`、`core/metadata` | |
| `spring-ai-client-chat`(ChatClient + Advisors) | **`chatclient` 模块**(不在 core) | core 只留 `chat.CallMiddleware` / `chat.StreamMiddleware` + 纯组合;高层便利层拆出去 |
| `spring-ai-vector-store`(VectorStore + Filter DSL) | `core/vectorstore`、`core/vectorstore/filter` | |
| `spring-ai-retry`(Transient/NonTransient + RetryTemplate) | **刻意不移植** | 见 §9 |
| Micrometer `Observation`(内嵌进 SPI) | **`otel` 模块**(装饰在协议边界,core 不 import) | 见 §10 |

**依赖方向的根本差异**:Spring AI 的 `spring-ai-model` 直接 import `org.springframework.util.*`(`Assert`/`MimeType`/`Resource`)、Reactor、Micrometer、Jackson、Apache Commons Logging;可观测性直接焊进 SPI(`AbstractObservationVectorStore`、advisor chain 内嵌 `ObservationRegistry`)。lynx `core` **生产代码只依赖 Go 标准库和 core 自身**,`internal/arch` 对该依赖预算 fail-fast —— provider SDK、tokenizer、OTel、vector DB driver 全在外圈。这是最底层的分歧,决定了后面所有取舍。

---

## 1. Model SPI —— 泛型三层契约 vs 最小能力接口

**Spring AI**:一套泛型骨架贯穿所有 modality。

```java
interface Model<TReq extends ModelRequest<?>, TRes extends ModelResponse<?>> {
    TRes call(TReq request);
}
interface StreamingModel<TReq, TResChunk> { Flux<TResChunk> stream(TReq request); }
interface ModelRequest<T>  { T getInstructions(); ModelOptions getOptions(); }
interface ModelResponse<T extends ModelResult<?>> { T getResult(); List<T> getResults(); ResponseMetadata getMetadata(); }
interface ModelOptions {}     // 空 marker
interface ResultMetadata {}   // 空 marker
```

每个 modality 是它的特化:`ChatModel extends Model<Prompt,ChatResponse>, StreamingChatModel`;`EmbeddingModel extends Model<EmbeddingRequest,EmbeddingResponse>`(call-only)。`ChatModel`/`TextToSpeechModel` 有 `default getOptions()`,其他没有;`ChatModel.stream()` 有 default 实现 `throw UnsupportedOperationException`。

**lynx core**:不要泛型骨架、不要空 marker、不用继承表达 modality。每个 modality 是**扁平的具体包**,`Model` 只有一个方法:

```go
// core/chat
type Model interface {
    Call(ctx context.Context, req *Request) (*Response, error)
}
type Streamer interface {
    Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}
```

**取舍与理由**:

- **能力靠组合,不靠泛型继承**。Spring 用 `Model<TReq,TRes>` 泛型 + `StreamingModel` 子接口表达"这个 modality 支持流"。lynx 用**两个独立接口**:`Model`(只 `Call`)与 `Streamer`(只 `Stream`)。一个 provider "只支持同步"就只实现 `Model`,不被迫 `throw UnsupportedOperationException`;"只支持流"也不用伪造一个同步 `Call`。**接口隔离(ISP)到方法级**,而不是到 modality 级。
- **没有空 marker interface**。`ModelOptions {}` / `ResultMetadata {}` 在 Go 里是纯噪音 —— Go 的 `any` 和结构化类型让它们毫无价值。lynx 直接用具体 `Options` 值类型。
- **`context.Context` 是一等参数**,不是 Reactor 的 `Context`。取消、超时、trace 传播全走 stdlib `ctx`,`errors.Is` 保留 `context.Canceled`/`DeadlineExceeded` 身份。
- **不强制 default options**。Spring 的 `getDefaultOptions()`→`getOptions()` 正在 `@Deprecated(forRemoval)` 迁移中 —— 这种"接口上挂一个可选能力"的摇摆,lynx 从设计上回避:default option 是 provider 实现的私事,不是 SPI 方法。
- **流式用 `iter.Seq2[*Response, error]`,不用 Reactor `Flux`**。这是 core 反向不变量之一(❌ 用 channel 或第三方 reactive 取代 `iter.Seq2`)。`Flux` 是一个重依赖 + 一套独立的调度/背压心智;`iter.Seq2` 是语言原语,调用方 early-stop / ctx-cancel / 首错终止都有测试保证,provider 在 caller 停止迭代时必须同步释放资源、不留 detached goroutine。

> Spring 的泛型三层在 Java 里是为了"一套 metadata/response 代码复用到 5 个 modality";lynx 用**每个 modality 各自的具体值类型 + 少量 `internal/ptr`、`internal/extension` 共享 helper** 达到同样的 DRY,而不引入贯穿所有类型的泛型参数。

---

## 2. Message / Part —— 单 text + 平行列表 vs tagged-value parts

**Spring AI**:Message 是**非 sealed 的抽象类层级**,内容表达为"一个 text blob + 平行的 media 列表 + 平行的 tool-call 列表"。

```java
abstract class AbstractMessage implements Message {   // Message extends Content
    MessageType messageType;              // enum USER/ASSISTANT/SYSTEM/TOOL
    @Nullable String textContent;         // 单个文本
    Map<String,Object> metadata;          // free-form
}
class UserMessage      extends AbstractMessage implements MediaContent { List<Media> media; }
class AssistantMessage extends AbstractMessage implements MediaContent {
    List<ToolCall> toolCalls;             // record ToolCall(id,type,name,arguments)
    List<Media> media;
}
class ToolResponseMessage extends AbstractMessage { List<ToolResponse> responses; }
```

分派靠 `instanceof` 模式匹配(`Prompt.instructionsCopy()` 遇未知类型 `throw IllegalArgumentException`)。一条 message = (一段文本) +(media 列表)+(tool-call 列表),**不是有序异构 parts 数组**。

**lynx core**:Message 是**tagged-value + 有序异构 `[]Part`**。

```go
// core/chat
type Message struct {
    Role     Role         `json:"role"`
    Parts    []Part       `json:"parts"`     // 有序,可交错
    Metadata metadata.Map `json:"metadata,omitempty"`
}
// Part 用公开 discriminator PartKind(PartText/PartMedia/PartReasoning/PartToolCall/PartToolResult)
```

**取舍与理由**:

- **有序 parts 让交错内容可无损 round-trip**。现代 provider(Anthropic/OpenAI)一条 assistant 消息里 text、reasoning、tool-call 是**交错**的。Spring 的"单 text + 平行 toolCalls 列表"丢掉了顺序,拼回去要靠约定。lynx 的 `[]Part` 保序,`Part.Clone()` 深拷贝。
- **tagged-value 而非 sealed hierarchy**。Go 没有 sealed class;lynx 用**公开 `PartKind` discriminator + 普通值**,未知类型返回**可诊断错误**,而不是靠未导出方法封口或 `instanceof` 抛异常。这与 core 心智"Tagged value,而非 sealed hierarchy"一致。
- **Role/Part 兼容矩阵是显式方法**。`Role.allowsPart(kind)` 把"system 只能 text、tool 只能 tool-result、assistant 可 text/media/reasoning/tool-call"写成一张可测的表,`Validate()` 逐 part 检查。Spring 把它散在各 `AbstractMessage` 子类的构造断言里(`Assert.notNull(textContent)` for SYSTEM/USER)。
- **`MarshalJSON` 在 wire 边界校验**。`Message.MarshalJSON` 先 `Validate()` 再序列化,`UnmarshalJSON` 解码后立即 `Validate()`。非法 message 无法悄悄过 wire。Spring 靠构造期 `Assert` + Jackson,wire 边界无统一校验闸。
- **`metadata` 是 JSON-safe 的 `metadata.Map`,不是 `Map<String,Object>`**。见 §4。

---

## 3. Options —— portable 接口 + provider 子类 + builder vs 值类型 + Extensions + Validate

**Spring AI**:两级设计 + self-typed builder + `mutate()`。

```java
interface ChatOptions extends ModelOptions {           // portable 跨 provider 旋钮,全 @Nullable getter
    String getModel(); Double getTemperature(); Integer getMaxTokens(); ...
    Builder<?> mutate();
    interface Builder<B extends Builder<B>> {          // self-typed 泛型,可被 provider builder 继承
        B temperature(Double); ... B combineWith(ChatOptions.Builder<?> other);   // runtime-over-default 合并
    }
}
class DefaultChatOptions implements ChatOptions { ... }  // 不可变
class DefaultToolCallingChatOptions extends DefaultChatOptions implements ToolCallingChatOptions { ... }  // provider 子类加原生字段
```

Provider 特定 option 靠**继承 `DefaultChatOptions` 并 self-typed builder 链**;portable+runtime 合并靠 `combineWith` / `ModelOptionsUtils.mergeOption(runtime, default)`。

**lynx core**:Options 是**值类型 struct**,provider extras 走 `Extensions metadata.Map`,合并/校验是 receiver 方法。

```go
// core/embedding(其余 modality 同构)
type Options struct {
    Model      string       `json:"model"`
    Dimensions *int64       `json:"dimensions,omitempty"`
    Extensions metadata.Map `json:"extensions,omitzero"`   // provider 特定项,JSON-safe
}
func NewOptions(model string) (Options, error)               // 返回值,不返指针
func (o *Options) SetExtension(key string, value any) error
func (o Options) Clone() Options
func (o Options) Merged(overrides ...Options) (Options, error)  // 标量非零覆盖 + Extensions last-write-wins
func (o Options) Validate() error                            // Options{} 合法
```

**取舍与理由**:

- **值语义替 builder 链**。Spring 的 `Builder<B extends Builder<B>>` self-typed 泛型 + `mutate()` 是最典型的、要在 Go 里**压平**的 Java-ism(项目"没有 Java 味"硬约定:禁 builder 链)。lynx 直接用 struct literal + `Merged(...)`。`DefaultOptions`/`NewOptions` **返回值不返指针**是刻意的不可变性保证([[feedback_default_options_signature]])。
- **provider extras 进 `Extensions` 一张 map,不进继承层级**。Spring 每个 provider 一个 `DefaultXxxChatOptions extends DefaultChatOptions` 子类;lynx 不为 provider 造子类型 —— 只有 core 语义稳定、跨 provider 共享的旋钮才是 `Options` 固定字段(core 反向不变量:❌ 把单 provider 或跨 provider 语义不同的 option 提升为固定字段),其余一律 `SetExtension`。
- **`Validate()` 是一等公开方法 + 边界调用**。Spring 的校验散在 `Assert.isTrue(...)` 里、随构造抛 `IllegalArgumentException`;lynx 把 `Validate()` 导出,在 I/O 边界显式调用,`Options{}` 零值合法(useful zero value)。
- **合并规则显式且可测**:标量非零覆盖、`Extensions` last-write-wins(`Merge`),而不是 `combineWith` 的 "take other's non-null"。语义等价,但写成普通 Go 方法而非泛型 builder。

---

## 4. Usage / Metadata / Media —— 无类型逃生口 vs 类型化 + JSON-safe

**Spring AI** 到处是**无类型逃生口**:

- `Usage`:interface,`getPromptTokens()`/`getCompletionTokens()`/`getTotalTokens()` + **`Object getNativeUsage()`**(原始 provider 对象逃生口)+ `EmptyUsage` sentinel。
- `RateLimit`:interface(requests/tokens limit/remaining/reset)+ `EmptyRateLimit`。
- `ResponseMetadata`:string-keyed map,impl 背后是 **`ConcurrentHashMap`**;`ChatResponseMetadata` 有 typed 字段(id/model/rateLimit/usage)+ 继承的 free-form map。
- `Media`:`@Nullable String id`、`MimeType`(**`org.springframework.util.MimeType`**)、**`Object data`**(存 String URL / `byte[]` / `java.net.URL`,`getDataAsByteArray()` 不匹配就抛)。
- `Document`:`metadata: Map<String,Object>`;`@JsonIgnore` 的可变 `ContentFormatter`(stray setter)。

**lynx core**:

- **删掉了 shared `Usage` / `RateLimit` 包**。曾有个 `core/model` 放共享 `Usage`/`RateLimit`,审计发现只有 embedding 消费 `Usage` 且只填 `PromptTokens`,`RateLimit` 零生产者零消费者 —— 于是**整包删除**,`embedding` 自带 `Usage{PromptTokens int64}`,`RateLimit` 不复存在(第一法则:发现设计不对在源头改对,不留 dead type)。chat 侧 `Usage` 是自己的具体值类型,`Total()` 是 receiver 方法。
- **`metadata.Map` 替 `Map<String,Object>`**:一个 JSON-safe 的类型化 map,`Validate()` 在 I/O 边界拒绝非 JSON-safe 值,`Set`/`Merge`/`Clone` 是方法。没有 `ConcurrentHashMap` 藏在 metadata 背后(并发安全是上层的事)。
- **`media.Source` 是类型化的,没有 `Object data`**:`SourceKind` discriminator + 明确字段,不用 `Object` 存三种可能。
- **没有 `getNativeUsage()` 这类逃生口**:DTO 不携带任意运行时对象(core 反向不变量:❌ 把 `any`/闭包/SDK client/`io.Reader` 塞进 wire DTO)。要原始 provider 数据,provider adapter 自己在外圈保留,不污染协议值。

**取舍与理由**:无类型逃生口(`Object data`、`Object nativeUsage`、free-form `Map<String,Object>`)在 Java 里是"先跑起来"的务实,但它们让协议值不再可静态验证、不再保证 JSON-safe、给了 prompt-injection 面(Spring 自己的 doc 注释都标了 `Media.name`/`id` 是注入面)。lynx 的立场:**协议值必须可序列化、JSON-safe、边界可 `Validate`**;逃生口一旦开在协议层,就是第一法则要偿还的债。

---

## 5. Tool calling —— core 只留协议,执行/循环下沉到 agent

**Spring AI**:tool 执行**在 model 层**就有完整机制。

```java
interface ToolDefinition { String name(); String description(); String inputSchema(); }   // schema 是裸 JSON string
interface ToolCallback   { ToolDefinition getToolDefinition(); String call(String toolInput); }  // 入参/返回都是 JSON string
interface ToolCallingManager { List<ToolDefinition> resolveToolDefinitions(...); ToolExecutionResult executeToolCalls(Prompt, ChatResponse); }
// AssistantMessage.ToolCall(id,type,name,arguments) / ToolResponseMessage.ToolResponse(id,name,responseData)
```

工具循环由 `ToolCallingAdvisor` 在 advisor chain 里跑(见 §6),或由 `ToolCallingManager` 编排。

**lynx**:**core 只保留协议值**(`ToolDefinition`、tool-call part、tool-result part),**执行与循环全在 `agent/toolloop`**。

- core `chat` 只有 `ToolDefinition`、`ToolCall`、`ToolResult` 等协议值,**没有可执行 `Tool` 接口**,也没有 `ToolCallingManager` 这类编排服务。可执行工具的最小接口位于外圈 `tools` 模块。
- 工具循环是 `agent/toolloop.Runner`:消费 `chat.Model` + `Request` + `ToolResolver`,emit `iter.Seq2` 的 `Event`;工具默认互斥,实现 `ConcurrentTool` 后可按 resource key 做有界 conflict-aware 并发,但结果与 continuation 始终按原 tool-call 顺序提交;无自动 retry,可 `checkpoint`/`resume`(pause 在 pending call 处续跑,不重放已完成工具或模型轮)。协议值与运行时状态**分离**:model request/response 是协议值,可执行工具在运行时的 `ToolResolver` 里,pause/resume 通过 `Event` 表达,不往 provider `Response` 塞运行时状态。

**取舍与理由**:core 是"协议,不是总框架"。`ToolCallingManager` / 工具循环是**运行时语义**,不是 provider 之间稳定共享的协议 —— 放 core 会让 core 反向不变量(❌ 在 core 放 tool executor/registry / agent control flow)破功。lynx 把它下沉到 agent framework,core 只欠"一个 tool-call 长什么样"的协议定义。此外 lynx 不把 structured output 当 tool-options 的一个开关(Spring 的 `StructuredOutputChatOptions`),而是 `chat.JSONParser[T]` / `ListParser` / `MapParser` 一族 parser,Reasoning 是 first-class([[feedback_structured_output_closed]])。

---

## 6. 扩展机制 —— Advisor 环绕链 vs 一组函数式 middleware

这是**最大的架构分歧**,也是 lynx 最坚决的重决策。

**Spring AI**:`Advisor` 环绕链是核心扩展点,且**模型调用本身也是一个 advisor**。

```java
interface CallAdvisor   extends Advisor { ChatClientResponse adviseCall(ChatClientRequest, CallAdvisorChain); }
interface StreamAdvisor extends Advisor { Flux<ChatClientResponse> adviseStream(...); }
interface BaseAdvisor   extends CallAdvisor, StreamAdvisor {   // around-advice: after(chain.nextCall(before(req)))
    ChatClientRequest before(req, chain); ChatClientResponse after(resp, chain);
}
```

- `ChatModelCallAdvisor` / `ChatModelStreamAdvisor`(`LOWEST_PRECEDENCE`)—— **模型调用被建模成链尾 advisor**。
- `ToolCallingAdvisor`(`HIGHEST_PRECEDENCE+300`)—— **工具循环也是一个 advisor**(`do { chain.copy(this).nextCall } while hasToolCalls`)。
- `MessageChatMemoryAdvisor`、`SafeGuardAdvisor`、`SimpleLoggerAdvisor`、`StructuredOutputValidationAdvisor`、`QuestionAnswerAdvisor`(RAG)、`RetrievalAugmentationAdvisor` —— 一堆内置 advisor。
- 链是**破坏性遍历**的 `ConcurrentLinkedDeque`,`OrderComparator` 排序,`ConcurrentHashMap` 的 `context` 当 blackboard 贯穿 request→chain→response,Micrometer observation 包在每个 advisor 上。

**lynx**:**一个扩展机制 —— 函数式 middleware/decorator**;core 为同步与流式两个独立能力分别保留 `chat.CallMiddleware` / `chat.StreamMiddleware`,以及 `Wrap` / `WrapStream` 纯组合,不保留任何具体 advisor 实现。

- core `chat` 的两个 middleware 类型分别装饰 `Model` 与 `Streamer`;它们是同一种组合机制的能力对称版本。**没有内置 memory/RAG/safeguard/logger advisor**。
- 高层便利(ChatClient 的 fluent 面、entity 映射)在**独立的 `chatclient` 模块**,不在 core。
- memory 是 `chathistory` 模块,RAG 是 `rag` 模块,safeguard 是 agent 的 guardrails,logger/observation 是 `otel` 装饰器 —— 各自一域,通过 middleware 组合,而**不是**都塞进一条 advisor 链。

**取舍与理由**:

- **core 反向不变量明令 ❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链**。Spring 的 advisor 链把"模型调用、工具循环、记忆、RAG、日志、安全"全焊进**一条**链、一个 `context` blackboard、一套 Micrometer。这在 Spring 里优雅,但它让"协议库"背上了"总框架"的职责。lynx 的立场:core 只提供**类型 + 纯组合**(middleware),具体行为(memory/RAG/observe)是外圈模块各自用 middleware 接入,不预置一条包罗万象的链。
- **"模型调用是 advisor / 工具循环是 advisor"这个统一很聪明,但代价是把控制流藏进链**。lynx 反而把工具循环**显式**成 `toolloop.Runner`(一个可 checkpoint/resume 的状态机),把它当一等运行时对象,而不是链里一个隐式 link —— 这对 HITL pause/resume 是必须的(要精确定位 pending call)。
- **observability 不焊进 SPI**。Spring 的 `ObservationRegistry` 在 advisor chain、`AbstractObservationVectorStore` 里都是构造参数。lynx core **不 import OTel**,由 `otel` 模块在协议边界 wrap/decorate(见 §10)。

---

## 7. VectorStore —— 两边都 ISP 拆分(收敛),lynx 拆得更细

**难得的收敛点**:Spring AI 近期也把 `VectorStore` ISP 拆开了。

```java
interface VectorStoreRetriever { List<Document> similaritySearch(SearchRequest); }   // 只读
interface DocumentWriter        { void accept(List<Document>); }                     // 写
interface VectorStore extends DocumentWriter, VectorStoreRetriever {
    void add(...); void delete(List<String> ids); void delete(Filter.Expression); ...
}
```

**lynx** 拆得**更细**,把删除的两条路径也分开:

```go
// core/vectorstore
type Indexer       interface { Add(ctx, docs) error }
type Searcher      interface { Search(ctx, SearchRequest) ([]Match, error) }
type IDDeleter     interface { DeleteIDs(ctx, ids) error }        // 按 id 删
type FilterDeleter interface { DeleteWhere(ctx, filter.Predicate) error }  // 按 filter 删
```

**取舍与理由**:很多 provider "能搜不能改索引",或"能按 id 删但不支持 filter 删"。Spring 把 `delete(ids)` 和 `delete(Expression)` 都塞在 `VectorStore` 一个胖接口里(`doDelete(Expression)` default `throw UnsupportedOperationException`)。lynx 按**消费者真实能力**拆成四个窄接口,装配处 union、消费者各依赖自己那片 —— 这正是 core 心智"按 Indexer/Searcher/IDDeleter/FilterDeleter 拆小能力"。另外 lynx 的 `SearchRequest` 同时拥有**输入校验 `Validate()` 和 Match 输出校验 `ValidateMatches()`**(检查 score 范围、阈值、降序、TopK 上限);Spring 的 `SearchRequest` 是 plain class + builder,阈值过滤文档为"客户端后处理",无输出契约校验。

---

## 8. Filter DSL —— ANTLR 生成 vs 手写 scanner + 私有 optimizer

**Spring AI**:**ANTLR4 形式文法 + 生成的 lexer/parser/visitor**,每个 store 一个 converter。

- 文法 `Filters.g4`,生成 `FiltersLexer`/`FiltersParser`/`FiltersBaseVisitor`(check-in 源码)。
- AST 是 `Filter` 命名空间下的 `record`:`Expression(type,left,right)`、`Key`、`Value`、`Group`,`enum ExpressionType{AND,OR,EQ,NE,GT,...,IN,NIN,NOT,ISNULL,ISNOTNULL}`。
- `FilterExpressionTextParser` 用 ANTLR 解析 + `ConcurrentHashMap` 缓存;`FilterExpressionBuilder` 是 Hibernate-Criteria 风格的编程构造。
- **每个 store 一个 `FilterExpressionConverter`**(17 个:Pinecone/PgVector/Milvus/Redis/Elasticsearch/...),继承 `AbstractFilterExpressionConverter` template-method;`FilterHelper.negate` 用 De Morgan 下推 NOT,`expandIn` 把 `IN` 展开成 OR-of-EQ。

**lynx**:**手写 scanner + parser + 单一公开 AST + 私有 optimizer + Visitor SPI**,零 codegen/第三方依赖。

```go
// core/vectorstore/filter —— 公开面
type Expr interface { Start() Position; End() Position; Equal(Expr) bool; expr() }   // sealed via 未导出 expr()
type Selector interface { Expr; selector() }
type Predicate interface { Expr; predicate() }
// 节点:Ident / Literal / ListLiteral / UnaryExpr / BinaryExpr / IndexExpr
func Parse(input string) (Predicate, error)   // scanner→parser→Validate→optimize
func Validate(expr Predicate) error
// + Visitor/Visit(backend 遍历)、Formatter
// 私有:scanner.go / parser.go / token.go / optimizer.go / validate.go
```

**取舍与理由**:

- **不引入 ANTLR/codegen 依赖**。core 生产代码只依赖 stdlib —— ANTLR runtime 是外部依赖 + 一个构建期代码生成步骤,与 core 的依赖预算冲突。手写 scanner/parser 几百行,可读、可测、零依赖,且是 Go 生态惯例(`go/scanner`、`go/parser` 也是手写)。
- **单一公开 AST + tagged-value**。曾经存在过"公开语义 AST + 内部带 token 的 CST"两套 `Expr`,已收敛成**一个**扁平包里的公开 AST;`expr()`/`selector()`/`predicate()` 未导出方法把接口 sealed 住(遍历穷尽),节点字段对 backend adapter 可见。`scanner`/`parser`/`token`/`optimizer`/`analyzer` 全部包私有(小写)—— 稳定的只有 AST/Visitor/Formatter/Parse/Validate。
- **私有 `optimizer` 做的比 `FilterHelper` 多**。Spring 的 `FilterHelper` 只在 per-store converter 里做 De Morgan 下推 + `IN` 展开。lynx 的 `optimizer`(`optimize(Predicate) Predicate`)在 parse 后统一做**布尔代数规范化**:双重否定消除、flatten 同类逻辑项、去重(`uniquePredicates`)、吸收律(`removeAbsorbed`)、分配律公因子提取(`factorCommon`/`partitionCommon`)。backend visitor 拿到的是已规范化的树,不必各自重做。
- **`Validate` 有环检测**。`analyzer` 用 `active map[Expr]struct{}` + `defer delete` 做指针身份的 cycle detection(DAG-safe),`Literal.isIntegerIndex()` 用 `big.Rat` 精确判定非负整数下标。程序化构造的树(不经 parser)也能被 `Validate` 挡住非法算子/操作数/异构列表。
- **位置信息内建**。节点带 `Position{Line,Column}`,parse 出来的表达式带位置(错误可定位),程序化构造的用零值 —— 对齐 `go/token.Pos` 的做法。

> 两边都是"store-agnostic AST + per-backend 转换"的大结构(这点收敛);差异在**怎么得到 AST**(ANTLR codegen vs 手写)、**AST 是几套**(record 族 vs 单一 sealed-by-unexported 接口)、**规范化在哪**(per-store converter vs 统一 optimizer)。

---

## 9. Retry —— Transient/NonTransient 分类 vs 完全不做

**Spring AI**:`spring-ai-retry` 有完整 retry 层。

```java
class TransientAiException    extends RuntimeException {}   // 429/5xx/网络 → 重试
class NonTransientAiException extends RuntimeException {}    // 4xx → 不重试
// RetryUtils.DEFAULT_RETRY_TEMPLATE: maxRetries(10) includes(TransientAiException) delay(2s) multiplier(5) maxDelay(3min)
// DEFAULT_RESPONSE_ERROR_HANDLER: 4xx→NonTransient, else→Transient
```

**lynx**:**刻意完全不做 retry 层,也不做 Transient/NonTransient 分类**。

**取舍与理由**:这是明确的项目法则([[feedback_no_retry_layer]] + core 反向不变量 ❌ 加 retry layer / Transient·NonTransient 分类)。provider SDK 内部已有 retry(Anthropic/OpenAI 官方 SDK 都带指数退避),再叠一层是重复。**401 让 UI 提示重填 key,不做 OAuth/token refresh**(core 反向不变量)。唯一的"重试"语义在 agent 层的 `RetryPolicy` —— 但那是**action 级、由 action 作者声明 `RetrySafety`(idempotent/compensated)才允许 `MaxAttempts>1`**,框架绝不从 error 猜测副作用是否可重放(治本:不变量不交给"调用方记得别犯错")。这与 Spring 的"从 HTTP status 分类、在 HTTP 边界自动重试"是**根本不同的层**:lynx 把重试决策留给最懂副作用语义的人(action 作者 / SDK),不在协议层做一刀切分类。

---

## 10. 可观测性 —— Micrometer 内嵌 SPI vs OTel 装饰在边界

**Spring AI**:`ObservationRegistry` / `ObservationConvention` **直接是 SPI 的构造参数**。`AbstractObservationVectorStore` 把 `add/delete/similaritySearch` 包在 Micrometer observation 里;advisor chain 每个 link 包一个 `AI_ADVISOR` observation;`ChatClient`/`VectorStore` builder 都收 `ObservationRegistry`。可观测性**焊在协议类型内部**。

**lynx**:**core 不 import OTel**;`otel` 模块在协议调用边界 wrap/decorate。

**取舍与理由**:项目可观测性规约 —— core 只传播 `context.Context`,不拥有观测策略;OTel 三驾马车(trace/metric/log)由 `otel` wrapper 在边界添加,组合根启动时一次性绑定 exporter。这样"生产换 OTLP exporter 即把 span/metric/log 全导云端、业务零改",且 core 的依赖预算保持干净。Spring 把 observation 焊进 SPI 是"方便",但违反 lynx 的依赖边界(❌ 改变 import 方向)。这是一次刻意的**依赖倒置**:不是 core 主动观测自己,而是外圈装饰 core。

---

## 11. 通用 Java-ism → Go 的系统性压平

| Spring AI 形态 | lynx core 形态 | 理由 |
|---|---|---|
| self-typed `Builder<B extends Builder<B>>` + `mutate()` | struct literal + `Clone()`/`Merged()` 值方法 | 禁 builder 链 |
| 空 marker interface(`ModelOptions`/`ResultMetadata`) | 直接具体值类型 | Go 无需 marker |
| `getX()`/`setX()` getter/setter | 直接导出字段;必要时 receiver 方法 | 禁 `GetX/SetX` |
| 抽象基类(`AbstractMessage`/`AbstractResponseMetadata`) | 组合 + 少量 `internal` helper | composition over inheritance |
| `instanceof` 模式分派 | `PartKind`/`LiteralKind` 等公开 discriminator + 类型断言 | tagged-value |
| `Object data` / `Object nativeUsage` 逃生口 | 类型化字段 + 明确 discriminator | 协议值可校验、JSON-safe |
| `Map<String,Object>` free-form metadata | `metadata.Map`(JSON-safe + `Validate`) | 边界可验证 |
| Reactor `Flux`/`Mono` | `iter.Seq2[T, error]` + `context.Context` | 语言原语,无重依赖 |
| `org.springframework.util.Assert.*` 满地 | 边界 `Validate()` + sentinel error + `%w` 包装 | `errors.Is/As` 可用 |
| `record` 值元组 | 普通 struct + 值语义 | |
| Micrometer 内嵌 SPI | `otel` 边界装饰 | core 不 import OTel |
| ANTLR 生成 parser | 手写 scanner/parser | 零 codegen 依赖 |
| Transient/NonTransient + RetryTemplate | 无 retry 层 | SDK 内建足够 |

---

## 12. Spring AI 有、而 core 刻意没有

- **retry 分类与 RetryTemplate**(§9)—— 交给 SDK / agent action 级。
- **advisor 库**(memory/RAG/safeguard/logger/结构化校验)(§6)—— 拆到 `chathistory`/`rag`/agent guardrails,用 middleware 接入。
- **ANTLR 文法与生成 parser**(§8)—— 手写。
- **Micrometer 内嵌 SPI**(§10)—— `otel` 边界装饰。
- **StringTemplate(ST)模板引擎 / `BeanOutputConverter`**(ChatClient 的 `entity()` + 模板渲染)—— 高层便利在 `chatclient`;结构化输出是 `chat.JSONParser[T]` 一族。
- **`getNativeUsage()` / `getNativeClient()` 原始对象逃生口** —— 协议层不开逃生口。
- **RAG advisor(`QuestionAnswerAdvisor`/`RetrievalAugmentationAdvisor`)** —— 独立 `rag` 模块。

## 13. core 有、而 Spring AI 没有(或更弱)

- **wire 契约冻结 + fixture 门禁**:`internal/arch` 的 `TestExportedAPIMatchesBaseline`(公共面冻结)+ `TestWire*`(JSON DTO 聚合 fixture)+ 依赖预算 fail-fast。任何 exported API / JSON tag 变更必须过闸、评审后才 `-update-*` 重生基线。Spring AI 无等价的自动化冻结门禁。
- **边界 `Validate()` 贯穿**:Message/Request/Options/Document/SearchRequest 都有显式 `Validate()`,`MarshalJSON` 强制校验,输出侧还有 `ValidateMatches`。
- **有序异构 `[]Part`**:交错 text/reasoning/tool-call 无损 round-trip(§2)。
- **`iter.Seq2` 流式契约**:early-stop / ctx-cancel / 首错终止 / 同步释放资源,全部有测试保证,无 detached goroutine。
- **值语义不可变构造器**:`DefaultOptions`/`New*` 返回值不返指针;pointer-receiver 方法自带 nil 守卫。
- **零第三方依赖的协议核**:整个 core 生产代码只依赖 stdlib,依赖方向由 arch test 强制单向。

---

## 一句话总结

> Spring AI 把"跨 provider 协议"实现成一套 **Java 泛型骨架 + builder + marker + Reactor + Micrometer 内嵌 + ANTLR + retry 分类**的**总框架雏形**;lynx `core` 把同一层需求重新决策成 **tagged-value + 值语义 + `iter.Seq2` + 能力对称的函数式 middleware + 手写 scanner + 边界 `Validate` + 依赖预算 fail-fast** 的**纯协议窄腰** —— 凡是"运行时语义"(工具循环、retry、memory、RAG、observe)一律下沉/外挂,core 只欠 provider 之间稳定共享的那点协议。收敛处(VectorStore ISP 拆分、store-agnostic filter AST)是因为独立评估下它们本就最优;分歧处几乎都能追溯到一条:**core 是协议,不是框架**。
