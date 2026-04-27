# Spring AI 的 Thinking 处理架构 —— `AssistantMessage` 只有 text 字段，是怎么做到的？

> **问题来源**：用户阅读 `SPRING_AI_CAPABILITY_GAPS.md` 后注意到，Spring AI 的 `AssistantMessage` 同样只有 `content`（text）+ `metadata`（Map）+ `toolCalls` + `media`，**并没有专门的 `thinking` 字段**。那它是怎么承载推理模型「思考过程」的？
>
> **本文目的**：拆解 Spring AI 实际的代码路径，作为 Lynx 实现的设计参考。
>
> **结论先说**：Spring AI **不在 `AssistantMessage` 上加字段**。它用三套不同模式联合解决：
> 1. **多 Generation 模式**（Anthropic）：`ChatResponse.results` 这个 list 里**同时放好几条 AssistantMessage**——thinking 是一条独立的 AssistantMessage，main answer 是另一条，用 `metadata` 里的 marker key 区分。
> 2. **Metadata 通道模式**（DeepSeek、OpenAI 兼容服务器）：单条 AssistantMessage，把 reasoning text 塞进 `metadata["reasoningContent"]`。
> 3. **子类扩展模式**（DeepSeek 强类型版）：`DeepSeekAssistantMessage extends AssistantMessage`，加一个 `reasoningContent` 字段——为强类型 API 准备。
>
> **对 Lynx 的启示**：Lynx 现有的 `Response.Results []*Result` + `Result.AssistantMessage.Metadata map[string]any` **结构已经足够**，不需要在 `AssistantMessage` 上加 `Thinking` / `ThinkingSignature` 字段。改造范围比 `SPRING_AI_CAPABILITY_GAPS.md §1.3 Step 1` 描述的小得多。

---

## 1. 关键事实：Spring AI 的 AssistantMessage 实际形状

`spring-ai-model/src/main/java/org/springframework/ai/chat/messages/AssistantMessage.java`：

```java
public class AssistantMessage extends AbstractMessage implements MediaContent {
    private final List<ToolCall> toolCalls;
    protected final List<Media> media;
    // 继承自 AbstractMessage：
    //   protected String textContent;            // 只有这一个 text 字段
    //   protected Map<String, Object> metadata;  // 所有「附加信息」都进这里
    //   protected MessageType messageType;
}
```

**只有 4 类字段**：text、metadata、toolCalls、media。

`spring-ai-model/src/main/java/org/springframework/ai/chat/model/Generation.java`：

```java
public class Generation implements ModelResult<AssistantMessage> {
    private final AssistantMessage assistantMessage;
    private ChatGenerationMetadata chatGenerationMetadata;  // 另一层独立 metadata
}
```

`ChatResponse`：

```java
public class ChatResponse {
    private final List<Generation> generations;   // ← 这是关键：是 LIST
    private final ChatResponseMetadata metadata;
}
```

**关键不在 `AssistantMessage` 上，而在 `ChatResponse.generations` 这个 `List` 上**。一个响应可以同时**装多条 AssistantMessage**。

---

## 2. 模式 A：多 Generation（Anthropic 的做法）

### 2.1 同步响应：多条 Generation 共存于一个 ChatResponse

`AnthropicChatModel.java:982-999` 是**最关键的几行代码**：

```java
for (ContentBlock block : message.content()) {
    if (block.isText()) {
        TextBlock textBlock = block.asText();
        textContent.append(textBlock.text());            // 主 text 累积
    }
    else if (block.isToolUse()) {
        ToolUseBlock toolUseBlock = block.asToolUse();
        toolCalls.add(new ToolCall(...));                // 工具调用累积
    }
    else if (block.isThinking()) {
        // ★★★ 关键：thinking block 单独成一条 Generation ★★★
        ThinkingBlock thinkingBlock = block.asThinking();
        Map<String, Object> thinkingProperties = new HashMap<>();
        thinkingProperties.put("signature", thinkingBlock.signature());
        generations.add(new Generation(AssistantMessage.builder()
            .content(thinkingBlock.thinking())            // thinking text 进 content
            .properties(thinkingProperties)               // signature 进 metadata
            .build(), generationMetadata));
    }
    else if (block.isRedactedThinking()) {
        // Redacted thinking 也是一条独立 Generation
        RedactedThinkingBlock redactedBlock = block.asRedactedThinking();
        Map<String, Object> redactedProperties = new HashMap<>();
        redactedProperties.put("data", redactedBlock.data());
        generations.add(new Generation(AssistantMessage.builder()
            .properties(redactedProperties).build(), generationMetadata));
    }
}

// 主 text + toolCalls 也是一条 Generation
AssistantMessage.Builder builder = AssistantMessage.builder().content(textContent.toString());
if (!toolCalls.isEmpty()) builder.toolCalls(toolCalls);
generations.add(new Generation(builder.build(), generationMetadata));
```

**响应结构示意**（thinking + tool use 混合的复杂例子）：

```
ChatResponse {
  generations: [
    Generation { AssistantMessage { content="思考过程...", metadata={signature: "abc"} } },
    Generation { AssistantMessage { content="",            metadata={data: "redacted"} } },
    Generation { AssistantMessage { content="最终答案...", toolCalls: [callA, callB] } }
  ],
  metadata: { id, model, usage, ... }
}
```

### 2.2 用户使用模式（来自 `anthropic-chat.adoc:436-451` 官方文档）

```java
ChatResponse response = chatModel.call(prompt);
for (Generation generation : response.getResults()) {
    AssistantMessage message = generation.getOutput();
    if (message.getMetadata().containsKey("signature")) {
        // 这是 thinking block
        System.out.println("Thinking: " + message.getText());
        System.out.println("Signature: " + message.getMetadata().get("signature"));
    }
    else if (message.getMetadata().containsKey("data")) {
        // 这是 redacted thinking block
        System.out.println("Redacted: " + message.getMetadata().get("data"));
    }
    else if (message.getText() != null && !message.getText().isBlank()) {
        // 这是最终文本响应
        System.out.println("Answer: " + message.getText());
    }
}
```

**判别 marker**：
- `metadata.containsKey("signature")` → thinking block
- `metadata.containsKey("data")` → redacted thinking block
- 都没有 → 普通 text

### 2.3 流式响应：每个 chunk 是一条 Generation

`AnthropicChatModel.java:391-457`（精简）：

```java
// content_block_start：检测到 redacted_thinking
if (contentBlock.isRedactedThinking()) {
    Map<String, Object> p = Map.of("data", redactedBlock.data());
    return new ChatResponse(List.of(new Generation(
        AssistantMessage.builder().properties(p).build())));
}

// content_block_delta 内部：
if (delta.text().isPresent()) {
    // 普通 text 增量
    return new ChatResponse(List.of(new Generation(
        AssistantMessage.builder().content(delta.asText().text()).build())));
}
if (delta.isThinking()) {
    // thinking 增量：用 marker key "thinking" = TRUE
    Map<String, Object> p = Map.of("thinking", Boolean.TRUE);
    return new ChatResponse(List.of(new Generation(
        AssistantMessage.builder()
            .content(delta.asThinking().thinking())
            .properties(p).build())));
}
if (delta.isSignature()) {
    // signature 在 thinking 块结束时才到达，单独一个 chunk
    Map<String, Object> p = Map.of("signature", delta.asSignature().signature());
    return new ChatResponse(List.of(new Generation(
        AssistantMessage.builder().properties(p).build())));
}
```

**流式 marker 体系**：

| chunk 类型 | metadata key | text 字段 |
|-----------|-------------|----------|
| 普通 text 增量 | （无 marker） | 增量文本 |
| thinking 增量 | `thinking = TRUE` | 思考增量 |
| signature 终态 | `signature = <值>` | 空 |
| redacted block | `data = <数据>` | 空 |
| tool call 增量 | `tool_call_id = <id>` 等 | 由 ToolCall 字段承载 |

### 2.4 Anthropic 模式的设计哲学

**核心观察**：Spring AI 把「thinking」和「response」当作**同一类东西的不同实例**——都是 `AssistantMessage`，都用 `content` 装文本。区别只在 **metadata 上的 marker key**。

这套设计的好处：
1. **不污染核心抽象**：`AssistantMessage` 的字段不增长，所有非 Anthropic 提供方不付成本。
2. **一种 type 表达 N 种角色**：thinking / redacted_thinking / text / tool_use 都是 AssistantMessage，下游遍历用 if-else on metadata key 即可。
3. **顺序保留**：list 的顺序就是 API 返回的 ContentBlock 顺序，多轮对话回放时直接按顺序 emit 即可。
4. **零迁移成本**：老代码遍历 `response.getResults()` 时，遇到 thinking 那条 Generation 也能拿到 `getText()`，最多就是看到「奇怪的多余文本」，不会崩。

---

## 3. 模式 B：Metadata 通道（DeepSeek、OpenAI 兼容服务器）

### 3.1 不能多 Generation 的场景

DeepSeek、vLLM、OpenAI 兼容 server 走的是**OpenAI Chat Completions JSON schema**——每个 choice 就是一条消息，没有「ContentBlock 列表」概念。响应里 `reasoning_content` 就是一个**与 `content` 平级的字符串字段**：

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "最终答案",
      "reasoning_content": "思考过程"
    }
  }]
}
```

### 3.2 Spring AI 的处理：直接进 metadata

`spring-ai-docs/.../openai-chat.adoc:884`：

> *Spring AI maps this field from the JSON response to the `reasoningContent` key in the AssistantMessage metadata.*

用户访问方式：

```java
ChatResponse response = chatModel.call(prompt);
AssistantMessage message = response.getResult().getOutput();
String reasoning = message.getMetadata().get("reasoningContent");  // ★
String answer = message.getText();
```

**这里只有一条 Generation**——因为 OpenAI schema 本身只允许一条 message。Reasoning 只能塞 metadata。

### 3.3 流式累积示例（来自 `openai-chat.adoc:954-971`）

```java
StringBuilder reasoning = new StringBuilder();
StringBuilder content = new StringBuilder();
chatModel.stream(prompt).subscribe(response -> {
    AssistantMessage message = response.getResult().getOutput();
    String text = message.getText();
    if (text != null) content.append(text);

    String reasoningChunk = message.getMetadata().get("reasoningContent");
    if (reasoningChunk != null) reasoning.append(reasoningChunk);
});
```

**每个 chunk 都是一条 AssistantMessage**，同时携带 text 增量和 reasoning 增量——两个独立 buffer 累积。

---

## 4. 模式 C：DeepSeek 子类扩展（`DeepSeekAssistantMessage`）

`models/spring-ai-deepseek/.../DeepSeekAssistantMessage.java:37-69`：

```java
public class DeepSeekAssistantMessage extends AssistantMessage {
    private @Nullable String reasoningContent;   // 强类型字段
    private @Nullable Boolean prefix;            // DeepSeek 的 prefix 模式开关
    
    public String getReasoningContent() { return this.reasoningContent; }
    // ...
}
```

**为什么要子类化**？
- `AssistantMessage.metadata` 是 `Map<String, Object>`，调用方需要 `getMetadata().get("reasoningContent")` + cast，类型不安全。
- DeepSeek 是 reasoning model 的代表性提供方，`reasoningContent` 在它的 API 里是**第一类字段**，子类化让用户拿到强类型 getter。
- 子类不破坏 `AssistantMessage` 基类的 ISP——其他 provider 不感知。

**用户访问方式**（来自 `deepseek-chat.adoc:253`）：

```java
ChatResponse response = chatModel.call(prompt);
AssistantMessage message = response.getResult().getOutput();
if (message instanceof DeepSeekAssistantMessage deepSeekMsg) {
    String reasoning = deepSeekMsg.getReasoningContent();   // 强类型
}
```

---

## 5. MessageAggregator：流式累积器对 thinking 的处理

`spring-ai-model/.../chat/model/MessageAggregator.java:55-208` 是 Spring AI **跨 provider 通用的流式聚合器**。它内部维护**三套独立的 StringBuilder**：

```java
AtomicReference<StringBuilder> messageTextContentRef = ...;     // 全部文本（thoughts + main）
AtomicReference<StringBuilder> thoughtsRef = ...;               // 仅思考
AtomicReference<StringBuilder> outputWithoutThoughtsRef = ...;  // 仅主答
```

每个流式 chunk 来时（lines 101-113）：

```java
if (chatResponse.getResult().getOutput().getText() != null) {
    messageTextContentRef.get().append(text);  // 全文累积
    var metadata = chatResponse.getResult().getOutput().getMetadata();
    if (metadata != null && metadata.containsKey("isThought")) {
        var isThought = Boolean.parseBoolean(metadata.get("isThought").toString());
        if (isThought) {
            thoughtsRef.get().append(text);                    // 进思考桶
        } else {
            outputWithoutThoughtsRef.get().append(text);       // 进主答桶
        }
    }
}
```

聚合完成后（lines 168-189）：

```java
AssistantMessage finalAssistantMessage;
var messageMetadata = messageMetadataMapRef.get();
if (!thoughtsRef.get().isEmpty()) {
    messageMetadata.put("thoughts", thoughtsRef.get().toString());                  // ★
    messageMetadata.put("outputWithoutThoughts", outputWithoutThoughtsRef.get().toString());  // ★
}
finalAssistantMessage = AssistantMessage.builder()
    .content(messageTextContentRef.get().toString())   // 全文（兼容）
    .properties(messageMetadata)                       // metadata 含 thoughts / outputWithoutThoughts
    .build();
```

**最终单条 AssistantMessage 的视图**：

| 想要 | 从哪里拿 |
|------|---------|
| 完整文本（兼容旧代码） | `getText()` |
| 仅思考过程 | `getMetadata().get("thoughts")` |
| 仅主答（剥离思考） | `getMetadata().get("outputWithoutThoughts")` |

**注意 marker 不一致**：
- Anthropic 流式 chunk 用 `metadata.thinking = TRUE`
- MessageAggregator 检测的是 `metadata.isThought`

这两处 key 不同。说明：
1. `MessageAggregator` 是**通用聚合器**，期望上游适配器把 marker 归一成 `isThought`，但 Anthropic 适配器目前发出的是 `thinking`——这处可能是上游历史包袱或正在迁移的版本不一致。
2. **对 Lynx 的启示**：自己设计时**应当一开始就统一 marker key**，避免类似不一致。建议用 `lynx:thinking` 或 `is_thought` 一种命名贯穿全栈。

---

## 6. 多轮对话回放：信号何时保留、何时丢弃

不同 provider 对「上一轮的 thinking 是否要回传给下一轮请求」要求不同：

| Provider | 多轮要求 | Spring AI 的做法 |
|----------|---------|----------------|
| **Anthropic** | thinking + tool_use 混合时**必须回传 thinking block 含 signature**，否则 API 拒绝 | 通过 Anthropic Java SDK 的 `Message.create()` 透传——SDK 接受用户传入的 ContentBlock 列表，由调用方负责把 history 中带 signature 的 AssistantMessage 重建成 ThinkingBlock |
| **DeepSeek** | 文档明确规定**不要把 `reasoning_content` 回传**，否则 API 返 400 | 构造 history 时**只取 `getText()` 和 `getToolCalls()`，丢弃 metadata 里的 reasoningContent**（见 `deepseek-chat.adoc:262`） |
| **OpenAI Chat Completions** | 不公开 reasoning text，无可回传 | 不处理 |
| **OpenAI Responses API** | 服务器侧保持 reasoning state（用 `previous_response_id`） | 客户端只回传 response id 引用，不重建 reasoning |
| **Google Gemini** | thinking + tool calling 多轮要求**回传 thoughtSignature**（每个 part 一个） | 在 `GoogleGenAiChatModel` 里抽取并在多轮中重建（已有完整 IT 测试 `GoogleGenAiThoughtSignatureLifecycleIT.java`） |

**关键洞察**：**这是适配器层的责任，不是核心抽象的责任**。`AssistantMessage` 不需要知道 thinking 怎么用——它只承担**「装载这些信息」**的职责，由各 provider 适配器决定**「装载到哪个 metadata key」**和**「构建下一轮请求时怎么读这些 key 重建 API 形状」**。

---

## 7. ThinkingTagCleaner：处理把 think 嵌在 text 里的「劣等」实现

某些**通过 OpenAI 兼容 API** 暴露的开源推理模型（Qwen3 Local、DeepSeek-R1 蒸馏版部分实现）会**把 `<think>...</think>` 直接拼在 content 字符串里**，不走结构化字段。这种情况下 Spring AI 提供 `ThinkingTagCleaner.java` 在结构化输出（`BeanOutputConverter`）解析前清洗：

```java
private static final List<Pattern> DEFAULT_PATTERNS = Arrays.asList(
    Pattern.compile("(?s)<thinking>.*?</thinking>\\s*", CASE_INSENSITIVE),  // Amazon Nova
    Pattern.compile("(?s)<think>.*?</think>\\s*", CASE_INSENSITIVE),         // Qwen
    Pattern.compile("(?s)<reasoning>.*?</reasoning>\\s*", CASE_INSENSITIVE),
    Pattern.compile("(?s)```thinking.*?```\\s*", CASE_INSENSITIVE),         // Markdown
    Pattern.compile("(?s)<!--\\s*thinking:.*?-->\\s*", CASE_INSENSITIVE)    // Comment
);

// fast path：text 不含 < 和 ` 直接返回（绝大多数响应零开销）
```

**这是「retroactive 清洗」**——理想做法是适配器在收响应时就把 `<think>...</think>` 提取出来塞进 metadata，但因为有些后端的 OpenAI 兼容服务器不规范，所以加这一层兜底。

---

## 8. 完整架构示意图

```
        ┌─────────────────────────────────────────────────────────┐
        │              Spring AI 核心抽象（不变）                   │
        │                                                         │
        │  AssistantMessage:                                      │
        │    - content: String              (text)                │
        │    - metadata: Map<String, Object>  (← 一切扩展点都在这)│
        │    - toolCalls: List<ToolCall>                          │
        │    - media: List<Media>                                 │
        │                                                         │
        │  Generation: { AssistantMessage, ChatGenerationMetadata }│
        │                                                         │
        │  ChatResponse: { List<Generation>, ChatResponseMetadata }│
        │                              ↑                          │
        │                              │                          │
        │                       这是 List！可装 N 条               │
        └─────────────────────────────────────────────────────────┘
                                       ▲
                                       │
                           ┌───────────┴───────────┬──────────────────┐
                           │                       │                  │
                ┌──────────▼─────────┐ ┌───────────▼──────────┐ ┌────▼─────────────┐
                │  Anthropic 适配器   │ │ DeepSeek/OpenAI 兼容  │ │ Google Gemini     │
                │                    │ │                       │ │                   │
                │ 多 Generation 模式：│ │ Metadata 通道模式：   │ │ Metadata 通道 +   │
                │ - 每个 ContentBlock │ │ - 单条 AssistantMsg   │ │ thoughtSignature │
                │   一条 Generation   │ │ - reasoning 进 meta   │ │ 多轮回传          │
                │ - signature 进 meta│ │   ["reasoningContent"] │ │                   │
                │ - data 进 meta     │ │                       │ │                   │
                │                    │ │ + DeepSeek 子类：     │ │                   │
                │                    │ │ DeepSeekAssistantMsg  │ │                   │
                │                    │ │ 强类型 getter         │ │                   │
                └────────────────────┘ └───────────────────────┘ └───────────────────┘
                           │                       │                  │
                           │                       │                  │
                           ▼                       ▼                  ▼
                ┌─────────────────────────────────────────────────────────────┐
                │ MessageAggregator (跨 provider 通用流式聚合器)                │
                │                                                              │
                │ 三套 StringBuilder：                                         │
                │   messageTextContentRef     ← 全文                            │
                │   thoughtsRef               ← 仅思考（按 metadata.isThought） │
                │   outputWithoutThoughtsRef  ← 仅主答                          │
                │                                                              │
                │ 输出单条 AssistantMessage，metadata 含：                      │
                │   "thoughts" / "outputWithoutThoughts" 两个干净视图          │
                └─────────────────────────────────────────────────────────────┘
                                                ▼
                ┌─────────────────────────────────────────────────────────────┐
                │ ChatClient / 用户代码                                         │
                │                                                              │
                │ 渲染只显示主答时：metadata.get("outputWithoutThoughts")     │
                │ 调试/审计看推理时：metadata.get("thoughts")                  │
                │ 老代码兼容：getText()                                        │
                └─────────────────────────────────────────────────────────────┘
                                                ▼
                                ┌────────────────────────────┐
                                │ BeanOutputConverter         │
                                │                            │
                                │ 解析前调用                  │
                                │ ThinkingTagCleaner          │
                                │ 兜底清洗 <think>...</think> │
                                └────────────────────────────┘
```

---

## 9. 对 Lynx 的设计映射（关键纠正）

> **本节已被 `REASONING_UNIFIED_DESIGN.md` 取代**。结论从「不需要在 AssistantMessage 上加字段」演化为「加一个 `Reasoning string` 单字段、provider-specific signature 走 Metadata map（key 在各 provider 包内）」。详见新文档。
>
> 历史归档保留如下：

> 本节修正 `SPRING_AI_CAPABILITY_GAPS.md §1.3 Step 1` 的过度激进——**不需要在 `AssistantMessage` 上加 `Thinking` 字段**。

### 9.1 现有 Lynx 结构已具备所有必需原语

`core/model/chat/response.go:70-79`：

```go
type Result struct {
    AssistantMessage *AssistantMessage   // 单条助手消息
    Metadata         *ResultMetadata     // 含 Extra map[string]any
    ToolMessage      *ToolMessage
}

type Response struct {
    Results  []*Result          // ★★★ 已是 list！可装多条 Result
    Metadata *ResponseMetadata
}
```

`core/model/chat/message.go:132-137`：

```go
type AssistantMessage struct {
    Text      string
    Media     []*media.Media
    ToolCalls []*ToolCall
    Metadata  map[string]any   // ★★★ 已有 metadata，可装 marker
}
```

**结论**：与 Spring AI 的 `ChatResponse / Generation / AssistantMessage` 三层完全同构。**无需破坏性结构变更**。

### 9.2 Lynx 需要的最小改造（替代 §1.3 Step 1）

#### 改造 #1：定义 marker key 常量（统一规范）

```go
// core/model/chat/marker.go (新文件，~20 行)
package chat

const (
    // 在 AssistantMessage.Metadata 上：
    MetaKeyIsThought   = "lynx:chat:is_thought"     // bool: true 表示该 Result 是思考块
    MetaKeySignature   = "lynx:chat:thinking_signature"  // string: Anthropic / Google 多轮必需
    MetaKeyRedactedData = "lynx:chat:redacted_thinking_data" // string: 安全脱敏标记

    // 在累积器输出的 AssistantMessage.Metadata 上：
    MetaKeyThoughts            = "lynx:chat:thoughts"               // string: 仅思考累积视图
    MetaKeyOutputWithoutThoughts = "lynx:chat:output_without_thoughts" // string: 仅主答累积视图

    // DeepSeek / OpenAI 兼容服务器：
    MetaKeyReasoningContent = "lynx:chat:reasoning_content"
)
```

#### 改造 #2：Anthropic 适配器走多 Result 模式

`models/anthropic/chat.go::buildAssistantMsg` 改为返回 `[]*chat.Result`：

```go
func buildResults(message *anthropic.Message) []*chat.Result {
    results := make([]*chat.Result, 0)
    var textBuf strings.Builder
    var toolCalls []*chat.ToolCall

    for _, block := range message.Content {
        switch b := block.(type) {
        case *anthropic.ThinkingBlock:
            am := chat.NewAssistantMessage(chat.MessageParams{
                Text: b.Thinking,
                Metadata: map[string]any{
                    chat.MetaKeyIsThought: true,
                    chat.MetaKeySignature: b.Signature,
                },
            })
            results = append(results, &chat.Result{AssistantMessage: am, ...})

        case *anthropic.RedactedThinkingBlock:
            am := chat.NewAssistantMessage(map[string]any{
                chat.MetaKeyIsThought:    true,
                chat.MetaKeyRedactedData: b.Data,
            })
            results = append(results, &chat.Result{AssistantMessage: am, ...})

        case *anthropic.TextBlock:
            textBuf.WriteString(b.Text)

        case *anthropic.ToolUseBlock:
            toolCalls = append(toolCalls, &chat.ToolCall{...})
        }
    }

    // 主 Result（text + toolCalls）
    am := chat.NewAssistantMessage(chat.MessageParams{
        Text: textBuf.String(),
        ToolCalls: toolCalls,
    })
    results = append(results, &chat.Result{AssistantMessage: am, ...})

    return results
}
```

#### 改造 #3：DeepSeek/OpenAI 兼容适配器走 metadata 通道

```go
// models/openai-compatible/chat.go (用于调 vLLM / DeepSeek-R1 等)
am := chat.NewAssistantMessage(chat.MessageParams{
    Text: choice.Message.Content,
    Metadata: map[string]any{
        chat.MetaKeyReasoningContent: choice.Message.ReasoningContent,
    },
})
```

#### 改造 #4：流式累积器加分流逻辑

`core/model/chat/response_accumulator.go` 加入：

```go
type accumulator struct {
    text                StringBuilder  // 全文
    thoughts            StringBuilder  // 仅思考
    outputWithoutThoughts StringBuilder // 仅主答
    // ...
}

func (a *accumulator) consume(chunk *chat.Response) {
    for _, result := range chunk.Results {
        msg := result.AssistantMessage
        if msg.Text != "" {
            a.text.WriteString(msg.Text)
            if isThought, _ := msg.Metadata[chat.MetaKeyIsThought].(bool); isThought {
                a.thoughts.WriteString(msg.Text)
            } else {
                a.outputWithoutThoughts.WriteString(msg.Text)
            }
        }
        // ... ToolCalls / Signature 等也累积
    }
}

func (a *accumulator) finalize() *chat.AssistantMessage {
    metadata := map[string]any{}
    if a.thoughts.Len() > 0 {
        metadata[chat.MetaKeyThoughts] = a.thoughts.String()
        metadata[chat.MetaKeyOutputWithoutThoughts] = a.outputWithoutThoughts.String()
    }
    return chat.NewAssistantMessage(chat.MessageParams{
        Text: a.text.String(),
        Metadata: metadata,
    })
}
```

#### 改造 #5：构建下一轮请求时的回放策略

`models/anthropic/chat.go::buildRequestMessages`（构建发给 API 的 messages）需要：

```go
for _, msg := range history {
    if am, ok := msg.(*chat.AssistantMessage); ok {
        if isThought, _ := am.Metadata[chat.MetaKeyIsThought].(bool); isThought {
            // 重建 ThinkingBlock
            sig, _ := am.Metadata[chat.MetaKeySignature].(string)
            blocks = append(blocks, &anthropic.ThinkingBlock{
                Thinking: am.Text,
                Signature: sig,
            })
        } else {
            blocks = append(blocks, &anthropic.TextBlock{Text: am.Text})
            // toolCalls 同上
        }
    }
}
```

`models/deepseek/chat.go::buildRequestMessages`：

```go
// DeepSeek 明确禁止回传 reasoning_content
// 所以构建 history 时只取 Text 和 ToolCalls，metadata 整个丢弃
```

#### 改造 #6：ThinkingTagCleaner 兜底

`pkg/text/thinking_cleaner.go`（~80 行 Go 实现，对应 Spring AI 的 `ThinkingTagCleaner`），让 `StructuredParser` 在解析前调用。

### 9.3 改造规模（对比 §1.3 旧估计）

| 项 | 旧估计（§1.3） | 新估计（本文） |
|-----|--------------|---------------|
| `AssistantMessage` 加字段 | ✅ 必须 | ❌ 不需要 |
| Marker 常量 | — | ~20 行 |
| Anthropic 适配器改 | 重写 buildAssistantMsg | 改成返回 []*Result，~80 行 |
| DeepSeek 适配器（新增） | 整个 module | 整个 module，但简单（~150 行） |
| 流式累积器 | 加 thinking 字段处理 | 加分流 buffer，~40 行 |
| 多轮回放 | 改基础结构 | 仅在适配器内部读 metadata key 重建，~30 行/适配器 |
| ThinkingTagCleaner | ~80 行 | 同 |
| **总代码量估计** | 1000+ 行 + 破坏性变更 | **400-500 行 + 完全向后兼容** |

### 9.4 与 Lynx 设计哲学的契合度

Lynx 在 `SPRING_AI_COMPARISON.md §7` 标识的优势之一是 **🏆 Interface Segregation 拆接口**——`memory.Store = Reader + Writer + Clearer`。沿用同样心法：

> **「让 thinking 这种 provider-specific 概念不污染核心抽象，靠 marker key + provider 适配器各自实现协议」**——这正是 Spring AI 的做法，也是 Lynx 应当借鉴的。

`AssistantMessage` 加 `Thinking` 字段会让所有不支持推理的 provider（gpt-4o、claude-haiku、gemini-flash）的代码也要处理这个字段，违背 ISP。

---

## 10. 一句话总结

> **Spring AI 不在 AssistantMessage 上加 `thinking` 字段。它把 thinking 视为「另一种文本」——同样装在 `content` 里，靠 metadata key（`signature` / `data` / `thinking` / `isThought` / `reasoningContent`）区分语义；多条同时返回时就放进 `ChatResponse.results` 这个 list 里。**
>
> **Lynx 的 `Response.Results []*Result` + `AssistantMessage.Metadata map[string]any` 已经具备完整原语。改造重心在适配器和累积器，而不是核心抽象。**

---

## 11. 配套阅读

- `SPRING_AI_COMPARISON.md` —— 整体架构对齐情况
- `SPRING_AI_CAPABILITY_GAPS.md` —— 能力清单（**§1.3 Step 1 应以本文 §9 为准**）
- Spring AI 源码索引：
  - `AssistantMessage.java`（核心抽象）
  - `Generation.java` + `ChatResponse`（多 Generation 容器）
  - `MessageAggregator.java:55-208`（流式聚合器）
  - `AnthropicChatModel.java:982-999`（多 Generation 模式实例）
  - `AnthropicChatModel.java:391-457`（流式 marker 实例）
  - `DeepSeekAssistantMessage.java`（子类扩展模式）
  - `ThinkingTagCleaner.java`（兜底清洗）
  - `anthropic-chat.adoc:312-499`（官方文档对外契约）
  - `openai-chat.adoc:881-1018`（reasoningContent 文档）
  - `deepseek-chat.adoc:239-285`（DeepSeek reasoning_content 文档 + 多轮规则）
