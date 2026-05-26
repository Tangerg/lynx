# `core/` — Review 阅读顺序

`core/` 是 lynx 的领域基石：定义 chat / embedding / vector / document
等抽象，但**不绑定**任何具体 LLM 提供商或 vector store。所有上层模块
(`models/` / `vectorstores/` / `chatmemory/` / `agent/` / `lyra/`) 都依赖
这一层。

阅读顺序：跨域共享 → chat 抽象 → embedding / image / audio / moderation →
document → vectorstore → tokenizer → evaluation。

---

## 1. 跨域共享

1. `media/media.go` **[精读]** — `*media.Media` 通用富媒体载体
   (二进制 + MIME)。chat / image / audio 都引用，签名不可破坏。
2. `model/model.go` — 所有 model 客户端的"模型元数据"基类
   (`ModelMetadata` / `Options` 接口的祖先)。
3. `model/api_key.go` — `ApiKey` 类型 + 安全打印。
4. `model/usage.go` **[精读]** — `Usage` (PromptTokens /
   CompletionTokens / ReasoningTokens *int64 / CacheRead /
   CacheWrite)。多个 provider 的 token roll-up 都要往这上面映射；
   字段语义注释看仔细。
5. `model/middleware.go` — 通用 middleware 抽象（chat-specific 的在
   `chat/`，这里是通用钩子骨架）。
6. `model/handler.go` — handler / handlerFunc 适配器范式。

## 2. Chat 抽象（**整库的承重墙**，精读全部）

7. `model/chat/doc.go` — 包说明，先看。
8. `model/chat/part.go` **[精读]** — Part 体系（TextPart / ReasoningPart /
   ToolCallPart / MediaPart）。整套 message-parts 设计的核心，配合
   `/doc/MESSAGE_PARTS_DESIGN.md` 一起看。
9. `model/chat/message.go` **[精读]** — SystemMessage / UserMessage /
   AssistantMessage / ToolMessage + `JoinedText` / `JoinedReasoning` /
   `ToolCalls()`。
10. `model/chat/request.go` — `Request` 结构：Messages / Options / Tools /
    Params。
11. `model/chat/response.go` — `Response` + `ResponseMetadata`
    (Usage / RateLimit / Created / Extra)。
12. `model/chat/part_accumulator.go` **[精读]** — Part 级流式累加
    （type-agnostic：`appendDelta`）。
13. `model/chat/response_accumulator.go` **[精读]** — Response 级累加器；
    Lyra 的 token 求和 + AG-UI 翻译都要靠它的合并规则。
14. `model/chat/model.go` — `Model` 接口（Call / Stream）。所有
    `models/<provider>/chat.go` 实现这个接口。
15. `model/chat/client.go` **[精读]** — `*Client` + Fluent
    `ClientRequest`（WithSystemPrompt / WithUserPrompt / Stream() /
    Call().Response()）。
16. `model/chat/tool.go` **[精读]** — `Tool` / `ToolDefinition` /
    `ToolMetadata` / 注册中心 / 调用包装。`agent/runtime` 的 AgentTool 装饰
    全靠这套。
17. `model/chat/tool_middleware.go` **[精读]** — 工具循环递归实现：每轮
    `next.Stream` → 累加 → 检查是否需要再 invoke → 重入。Lyra
    `agent.go` 检测 round-boundary 就靠这个层间合约。
18. `model/chat/parser.go` — 输出解析器 (JSON / List / Map)，泛型版。
19. `model/chat/prompt_template.go` — 简单模板。
20. `model/chat/tracing.go` — OTel chat span 名 / 属性常量。
21. `model/chat/middleware/` — 通用 chat middleware：
    - `logger.go` — 调用记录
    - `safeguard.go` — 敏感词 / 注入防护
22. `model/chat/memory/` **[精读]**
    - `memory.go` — `Store` 接口（Read / Write / Clear）。
    - `in_memory.go` — 默认实现。
    - `message_window.go` — 截断窗口策略。
    - `middleware.go` — 自动注入历史的 middleware（lyra 的
      `lyra/internal/engine` 直接复用）；注意 `prepareRequest`
      复制 `req.Tools` 的修复（避免下游 tool middleware 看不到工具）。

## 3. 其他模型形态（与 chat 同构）

23. `model/embedding/` — Embedding 客户端 / Request / Response /
    Dimensions / tracing。结构跟 chat 对称，扫读即可。
24. `model/image/` — 图像生成。
25. `model/audio/transcription/` 与 `model/audio/tts/` — STT / TTS。
26. `model/moderation/` — 内容审核。

## 4. 文档处理

27. `document/interface.go` — `Document` / `Reader` / `Transformer` /
    `Writer` / `Splitter` 等接口。
28. `document/document.go` — 默认结构。
29. `document/id/` — id 生成（UUID / SHA256）。
30. `document/reader_text.go` / `reader_json.go` — 内置读取器。
31. `document/transformer_*.go` — 文本 / token 分块器。
32. `document/batcher_token_count.go` — 按 token 数批切。
33. `document/writer_file.go` — 落盘。
34. `document/formatter_simple.go` / `nop.go` — 格式化助手。

## 5. 向量存储抽象（实现在 `vectorstores/`）

35. `vectorstore/` — `Store` 接口、`Query` / `Result` / 距离度量等。

## 6. Tokenizer

36. `tokenizer/interface.go` — `Tokenizer` 接口。各 provider 实现在
    `models/<provider>/tokenizer.go` 中。

## 7. 评估

37. `evaluation/evaluator.go` — `Evaluator` 接口（0-1 连续分 +
    可配阈值，参见最近 commit `4c762f8`）。
38. `evaluation/llm_evaluator.go` — LLM 作判官的通用模板。
39. `evaluation/fact_checking.go` / `relevancy.go` — 内置两种。
40. `evaluation/composite.go` — 多评估器加权组合。

## 跨模块提醒

- **核心**：`Part` / `Message` / `Response` 一改，半个仓库要跟。这层签名
  改动需要在 `/doc/MESSAGE_PARTS_DESIGN.md` 同步。
- chat-memory middleware 的 `req.Tools` 必须 clone — 之前出过 bug
  (lyra branch 修复了)，复查时验证还在。
- `tool_middleware` 的 round-boundary 合约（`Result.ToolMessage != nil
  && Result.AssistantMessage == nil`）是 Lyra token 求和的依据。
- Structured output 已"闭合设计"：`JSONParser[T]` / `ListParser` /
  `MapParser` 覆盖了 spring-ai 的 converter 家族。不要试图加 cleaner
  抽象 — Reasoning 是 first-class。
- `DefaultOptions()` 返回值（非指针），是有意为之的不可变保证。

## 体检命令

- `go test ./core/...` — 单测应全绿；约 30~50s。
- `grep -r "TODO\|FIXME" core/` — 应几乎没有。
- 看 `/doc/MESSAGE_PARTS_DESIGN.md` 的 §3.9 跨维度对比（最新 commit）。
