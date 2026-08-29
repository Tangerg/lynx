# Spring AI —— AI 集成层的工业标准

> 实测：Java，1,315 个 `src/main` 文件，**173,530 行**主代码，~60+ 个 Maven module（其中约半数是 `auto-configurations/*` 与 `starters/*`）。
> 结构：`spring-ai-commons` / `spring-ai-model` / `spring-ai-client-chat` / `spring-ai-rag` / `spring-ai-vector-store` / `spring-ai-retry` / `spring-ai-template-st` / `models/*` / `vector-stores/*` / `document-readers/*` / `memory-repositories/*` / `mcp/*` / `advisors/*`。

---

## 0. 一句话定位

**Spring AI 定义了"AI 集成层"这个层次本身。** 在它之前，"把 LLM 接进企业应用"没有公认的分层；在它之后，`ChatModel` / `EmbeddingModel` / `VectorStore` / `Document` / `ToolCallback` / `Advisor` 成了几乎所有后来者的默认词汇表 —— 包括 scope。

**scope 与 Spring AI 的关系不是竞争，是继承 + 去 Java 化。** scope 的 `core/chat`、`core/embedding`、`core/document`、`core/vectorstore`、`chatclient` 全部能在 Spring AI 里找到直系对应物。真正的分析价值在于：**哪些概念被完整继承、哪些被有意识地拒绝、为什么。**

---

## 1. 谱系对照总表

| Spring AI | scope | 关系 |
|---|---|---|
| `Model<TReq, TRes>` / `StreamingModel` | `chat.Model` / `chat.Streamer` | **拒绝泛型基接口**，改为每 modality 独立最小接口 |
| `ChatModel` / `EmbeddingModel` / `ImageModel` / `SpeechModel` / `TranscriptionModel` / `ModerationModel` | `core/chat` / `core/embedding` / `core/image` / `core/speech` / `core/transcription` / `core/moderation` | **一比一继承 modality 划分** |
| `ChatClient`（fluent builder） | `core/chatclient` | 继承定位，**拒绝 builder 链** |
| `Advisor` 有序链 | `chat.CallMiddleware` / `StreamMiddleware` | **概念继承，机制替换** |
| `Document` | `core/document.Document` | 直接继承 |
| `VectorStore`（胖接口） | `Indexer`/`Searcher`/`IDDeleter`/`FilterDeleter` | **ISP 拆碎** |
| `Filter.Expression` + `FilterExpressionConverter` | `filter.Predicate` + `filter.Visitor` | 继承思想，改为 Visitor |
| `ToolCallback` / `ToolCallbackProvider` | `core/tool.Tool` / `core/tool.Registry` | 继承，**去掉全局 Provider 发现** |
| `spring-ai-rag`（Modular RAG 四阶段） | `rag` 小接口 + 组合函数 | **拒绝固定 pipeline** |
| `ChatMemory` + `memory-repositories/*` | `core/history` + `historystores/*` | 直接继承 |
| Micrometer `Observation` | 官方 OTel + `otel/*` decorator | **拒绝自造观测抽象** |
| `spring-ai-retry` | ❌ 明确不做 | **反向不变量** |
| `auto-configurations/*` + `starters/*` | ❌ 无 | **拒绝隐式装配** |
| `document-readers/*` | `etl/{text,json,markdown,html,pdf}` | 继承 |

---

## 2. D1 协议层：`Model<TReq, TRes>` 与 scope 的分歧

```java
public interface Model<TReq extends ModelRequest<?>, TRes extends ModelResponse<?>> {
	TRes call(TReq request);
}
public interface StreamingModel<TReq extends ModelRequest<?>, TResChunk extends ModelResponse<?>> {
	Flux<TResChunk> stream(TReq request);
}
```

在 Java 里这是合理的：泛型基接口给了 `ChatModel`、`EmbeddingModel`、`ImageModel` 一个共同祖先，工具链（AOP、Observation、auto-config）可以对 `Model` 做统一处理。

scope 明确拒绝了它。`core/CLAUDE.md`：

> ❌ **用泛型 Model/StreamingModel 模拟继承**，或让 Model 强制 DefaultOptions/Metadata/Stream。

改成每个 modality 一个独立、最小的接口：

```go
// core/chat
type Model    interface { Call(ctx, *Request) (*Response, error) }
type Streamer interface { Stream(ctx, *Request) iter.Seq2[*Response, error] }
```

**理由不是"Go 没继承"，而是这个共同祖先不承载任何行为。** `Model<TReq, TRes>` 的唯一方法就是 `call`，没有任何逻辑能写在这一层。它存在的意义纯粹是让框架能说"这是个 Model"。在 Go 里，一个不承载行为的公共父类型只会制造类型体操 —— eino 的 `BaseModel[M messageType]` 就是这条路的终点（见 [`eino.md`](eino.md)）。

**scope 的做法有一处比 Spring AI 更进**：`Streamer` 完全独立于 `Model`。Spring AI 的 `ChatModel extends Model, StreamingModel` —— **所有 ChatModel 都必须实现 stream**，哪怕底层 API 没有流式（就得伪造一个单元素 Flux）。scope 的 `core/CLAUDE.md` 明写这个理由：

> 每个 modality 的 `Model` 默认只有 `Call`；真实流能力以独立 `Streamer` 表达，……**不伪装为 Core SPI**。

---

## 3. D5 扩展机制：`Advisor` vs `Middleware` —— 最重要的一处分歧

### 3.1 Spring AI 的 Advisor

```java
public interface Advisor extends Ordered {          // ← 继承 Spring 的 Ordered
	int DEFAULT_CHAT_MEMORY_PRECEDENCE_ORDER = Ordered.HIGHEST_PRECEDENCE + 200;
	String getName();
}
public interface CallAdvisor   extends Advisor { ChatClientResponse adviseCall(req, CallAdvisorChain); }
public interface StreamAdvisor extends Advisor { Flux<ChatClientResponse> adviseStream(req, StreamAdvisorChain); }
public interface BaseAdvisor   extends CallAdvisor, StreamAdvisor {
	default ChatClientResponse adviseCall(req, chain) {
		var processed = before(req, chain);
		var response  = chain.nextCall(processed);
		return after(response, chain);
	}
	...
}
```

内置实现（实测）：`ChatModelCallAdvisor`、`ChatModelStreamAdvisor`、`MessageChatMemoryAdvisor`、`ToolCallAdvisor`、`ToolCallingAdvisor`、`SafeGuardAdvisor`、`SimpleLoggerAdvisor`、`StructuredOutputValidationAdvisor`、`LastMaxTokenSizeContentPurger`、`UsageAccumulator`。

外加**七个接口**描述这一个机制：`Advisor` / `CallAdvisor` / `StreamAdvisor` / `BaseAdvisor` / `AdvisorChain` / `CallAdvisorChain` / `StreamAdvisorChain` / `BaseAdvisorChain` / `MemoryAdvisor` / `BaseChatMemoryAdvisor` / `ToolAdvisor`。

### 3.2 关键问题：`Ordered`

`Advisor extends Ordered` 意味着**顺序由每个 advisor 自己声明的整数决定**，不由装配处决定。于是文档里出现这种东西：

```java
/**
 * Default order for chat-memory advisors. Placed before (outside)
 * ToolCallingAdvisor#DEFAULT_ORDER so the memory advisor wraps the tool-call loop,
 * and the ToolCallingAdvisor manages its own intermediate conversation history.
 */
int DEFAULT_CHAT_MEMORY_PRECEDENCE_ORDER = Ordered.HIGHEST_PRECEDENCE + 200;
```

**"memory 必须包在 tool loop 外面"这个真实的语义约束，被编码成了两个魔法整数的相对大小。** 加一个新 advisor 时，你必须知道所有其他 advisor 的 order 值才能放对位置。这是 Spring 生态的经典模式，也是它的经典痛点。

### 3.3 scope 的做法

```go
type CallMiddleware   func(next Model) Model
type StreamMiddleware func(next Streamer) Streamer

func Wrap(model Model, middlewares ...CallMiddleware) Model
func WrapStream(streamer Streamer, middlewares ...StreamMiddleware) Streamer
```

- **顺序 = 参数顺序**，第一个是最外层。装配处一眼可读，无需知道任何全局常量。
- **两个函数类型 + 两个函数**，对比 Spring 的 11 个接口。
- 内部一个泛型 `compose[T any, M ~func(T) T]`，注释里明确说明为什么这里用泛型：

  > compose is the one generic in the target Chat SPI: it reuses an actual wrapping algorithm for two concrete capabilities rather than creating a nominal generic model hierarchy.

  —— **泛型用于复用算法，不用于制造类型层级。** 这一条直接对着 Spring AI 的 `Model<TReq,TRes>` 说。

### 3.4 但 Advisor 有一处 middleware 做不到的事

`BaseAdvisor` 的 `before(req, chain)` / `after(resp, chain)` 拿得到 **chain 本身**。这让 advisor 能做"重新进入链路"的事（例如 `ToolCallingAdvisor` 管理自己的中间对话历史、多轮回环）。

scope 的 `CallMiddleware func(next Model) Model` 拿到的是 `next`，同样可以多次调用 `next.Call(...)` 实现循环 —— **能力等价**，且不需要 chain 这个额外概念。scope 这边更干净。

真正的差异在于：Spring 用 Advisor 实现了 **tool calling loop 本身**（`ToolCallingAdvisor`）。scope 把 tool loop 放在 `agent/interaction`（一个有 snapshot、有 Step 边界、有 Effect dispatcher 的 Execution Strategy），**不放在 chatclient 的 middleware 链里**。

这是一个根本分歧：

| | Spring AI | scope |
|---|---|---|
| tool loop 在哪 | ChatClient 的一个 Advisor | `agent/interaction` Execution Strategy |
| 能否暂停/恢复 | ❌（同步 Java 调用栈） | ✅ Step 边界 + ExecutionState |
| 能否 steer | ❌ | ✅ 明确的安全消费边界 |
| 能否限预算 | 部分（`ChatOptions`） | ✅ Budget 从父划拨 |
| 起步成本 | 极低（加一个 advisor） | 需要 Engine + Definition |

**Spring AI 的选择让"加个工具循环"是一行配置；scope 的选择让"工具循环跑一半崩了能恢复"成为可能。** 这正是 scope `ARCHITECTURE.md §2.1` 的复杂度阶梯要处理的事 —— 只需要一次模型调用就用 `chatclient`，需要暂停恢复才升级到 Interaction Process。

⚠️ **但这里有一个 scope 应当承认的缺口**：scope 的 `chatclient` **没有**开箱即用的 tool loop。用户想要"模型自动调工具、我不关心恢复"这个最常见的场景，在 scope 里要么自己写循环，要么直接上 Agent Framework。Spring AI 的 `ToolCallingAdvisor` 是一行。

`ARCHITECTURE.md §7.1` 提到过这个问题但把它悬置了：

> 是否暴露更小的直接 Runner，必须由独立消费者证明，不能仅为了迁移旧代码保留。

**这个"独立消费者"现在有了**：Spring AI 的 `ToolCallingAdvisor` 和 MAF 的 `DisableFuncAutoCall`（默认自动加工具调用 middleware）两个独立项目都提供了它，说明这是真实需求而非迁移遗留。值得重新评估。

---

## 4. D3 RAG：Modular RAG 四阶段 vs 组合函数

Spring AI 的 `spring-ai-rag` 严格照 Modular RAG 论文分层：

```
preretrieval/   query/{expansion, transformation}
retrieval/      search/{DocumentRetriever}, join/{DocumentJoiner}
postretrieval/  document/{DocumentPostProcessor}
generation/     augmentation/{QueryAugmenter}
advisor/        RetrievalAugmentationAdvisor（把上面串起来）
```

scope 的 `rag` 有**完全相同的概念集合**：

```
query.go / query_transformer_{rewrite,translation,compression}.go
query_expander_multi.go
retriever.go / retriever_{vectorstore,fusion,tool}.go
candidate_refiner_{dedup,topk,model}.go
augmentation.go
chat_middleware.go
```

**继承是彻底的。分歧只有一条**：Spring AI 有 `RetrievalAugmentationAdvisor` 这个**中心装配对象**，scope 明确拒绝：

> ❌ **恢复 PipelineConfig / Pipeline** —— 组合用 Go 函数完成，不引框架式中心配置。
> ❌ **加 QueryRouter / DocumentJoiner 之类固定阶段** —— 路由写成自定义 Retriever，合并写成 Refiner。

注意 scope 点名拒绝了 `DocumentJoiner` —— 这**正是** Spring AI `retrieval/join/` 包的名字。这不是巧合，是有意识的裁决。

### 谁对

Spring AI 的固定阶段在 Java + Spring 语境下有真实价值：auto-configuration 能按阶段装配，`application.yml` 能按阶段配置。**中心配置对象是 Spring 编程模型的必需品。**

scope 没有 DI 容器，组合就是函数调用。此时中心配置对象只是多一层间接。

不过 scope 多了一条 Spring AI 没有的正确性不变量：

> **同一文档身份只占一个检索名额**：相同非空 Document ID 保留最高分候选，同分按首次身份出现稳定决胜；`TopK` 在截断前完成该唯一化，**不能让 refiner 顺序决定结果正确性**。

Spring AI 的 `DocumentJoiner` 有去重，但去重与 TopK 的**相对顺序由用户装配决定** —— 装反了结果就错，且不会报错。scope 把这个不变量焊死在 `Candidates.uniqueBest()` 里。

---

## 5. D6 模块边界：auto-configuration 的双刃

Spring AI 约 60+ module，其中一半是 `auto-configurations/*` 和 `starters/*`。

**收益**：`spring-boot-starter-ai-openai` 一个依赖 + `application.yml` 里三行配置 = 一个能跑的 ChatModel。这是 Spring 生态最强的护城河，无可争议。

**代价**：

1. **隐式装配** —— 哪个 `ChatModel` bean 生效、Advisor 的 order 怎么排、`ObservationRegistry` 从哪来，全都靠 classpath 扫描 + 条件注解推导。出问题时排查路径极长。
2. **module 数量爆炸** —— 每个 provider 需要 `models/spring-ai-{x}` + `auto-configurations/models/spring-ai-autoconfigure-model-{x}` + `starters/spring-ai-starter-model-{x}` **三个** module。
3. **配置即 API** —— `application.yml` 的属性名成为事实上的公开契约，改名 = breaking change。

scope 的对立立场（`ARCHITECTURE.md §3.11`）：

> **透明胜过魔法。** 不做扫描、注解、全局 DI、隐式策略注册或隐藏模型调用。

以及 `DESIGN_PHILOSOPHY §2.1`：

> **组合根集中装配具体实现**：执行核只依赖抽象，具体实现在组合根注入 —— 加/删一个具体实现对核零波及。

scope 的每 provider **一个** module（不是三个），装配是一段显式的 Go 代码。

**判断**：这条分歧的根因是语言生态，不是设计水平。Java 应用的组合根散在 Spring 容器里，必须靠 auto-config；Go 应用的组合根就是 `main()` 里那几十行。**scope 不应吸收 auto-configuration，因为它要解决的问题 Go 里不存在。**

---

## 6. D7 可观测性：Micrometer Observation vs 官方 OTel

Spring AI 用 Micrometer 的 `Observation` API，在 `chat/observation/`、`tool/observation/`、`vectorstore/observation/`、`embedding/observation/` 各有一套 `ObservationConvention` + `ObservationContext`，再由 `auto-configurations/models/*/observation/` 装配。

Micrometer Observation 是一个**厂商中立的观测抽象层**，可以后端桥接到 OTel、Brave、Prometheus。

scope 的立场（`otel/CLAUDE.md`）：

> ❌ **自造 tracer/meter/registry 抽象** —— **OTel API 就是 vendor-neutral 层。**
> **为什么走 OTel 而不是直接写 slog**：可替换性 —— dev 用 slog 看着方便，生产把每个 exporter 换成 OTLP，**业务代码零改**。

**这是同一个目标（vendor neutrality）的两条实现路径**。Micrometer 早于 OTel 稳定，且是 Spring 生态既有资产，所以 Spring AI 用它；scope 从零开始，直接用已经成为事实标准的 OTel API。

scope 这边多一条 Spring AI 没有的边界：**领域模块完全不 import 观测**，wrapper 在独立 `otel` module 里反向装饰，且 Kernel 有 architecture gate 禁 OTel import。Spring AI 的 observation 类型散布在各领域包内（`org.springframework.ai.chat.observation.*`）。

---

## 7. D8 一处 scope 明确反向的东西：`spring-ai-retry`

Spring AI 有一个独立 module `spring-ai-retry`，提供 `RetryTemplate`、`TransientAiException` / `NonTransientAiException` 分类。

scope 的根 `CLAUDE.md` **点名拒绝**：

> ❌ 加 retry layer / **Transient·NonTransient 分类** —— SDK 内部已有 retry 就够。

这条反向不变量的名字直接来自 Spring AI 的类名。理由是时代变了：Spring AI 早期各家 HTTP 客户端还是裸 `RestClient`，需要自己重试；现在 `openai-go`、`anthropic-sdk-go` 等官方 SDK 都内置了带 backoff 的重试。**在 SDK 已有重试的前提下再加一层，只会让重试次数变成乘法。**

⚠️ 但要与 [`trpc-agent-go.md`](trpc-agent-go.md) 里的发现放在一起看：**failover（换 provider）和 hedge（并发对冲）不是 retry**，SDK 内部的重试覆盖不到。scope 的这条反向不变量应该保持，但不应被误读为"不做任何可用性策略"。

---

## 8. 结论

### 同构（继承关系，非独立收敛）

scope 的以下概念直接源自 Spring AI，且判断是正确的、应当保留：

| 概念 | 说明 |
|---|---|
| modality 划分（chat/embedding/image/speech/transcription/moderation） | 一比一继承 |
| `Document` 作为 ETL/RAG/VectorStore 的统一中间形态 | 继承 |
| `ChatClient` 作为"直接模型调用"的一等路径 | 继承定位 |
| Filter 表达式 + 后端方言编译 | 继承思想，改 Visitor |
| `ChatMemory` + 多后端 repository | 继承（`core/history` + `historystores/*`） |
| provider 适配层（一家一个包，不抽公共基类） | 继承 |
| RAG 的概念集合（transform/expand/retrieve/refine/augment） | 继承 |
| 契约测试套件归契约 owner | 继承（`storetest` ← Spring 的 vector store TCK） |

### 有意识拒绝的（且理由成立）

| Spring AI | scope 的裁决 | 理由 |
|---|---|---|
| `Model<TReq,TRes>` 泛型基接口 | 每 modality 独立最小接口 | 共同祖先不承载行为，只制造类型体操 |
| `ChatModel extends StreamingModel` | `Streamer` 独立能力 | 不逼无流式 API 的 provider 伪造流 |
| `Advisor extends Ordered` | `func(next Model) Model` | 顺序归装配处，不归魔法整数 |
| 11 个 Advisor 相关接口 | 2 个函数类型 | 一个同质机制 |
| `VectorStore` 胖接口 | Indexer/Searcher/IDDeleter/FilterDeleter | ISP；不逼只读后端伪实现删除 |
| Modular RAG 固定阶段 + 中心 Advisor | 小接口 + 组合函数 | Go 没有 DI 容器，中心配置只是多一层 |
| auto-configuration + starter（每 provider 3 个 module） | 显式组合根 + 每 provider 1 个 module | 「透明胜过魔法」；Go 的组合根就是 `main()` |
| Micrometer Observation 抽象层 | 官方 OTel API + `otel/*` decorator | OTel API 本身就是 vendor-neutral 层 |
| `spring-ai-retry` + Transient 分类 | ❌ 反向不变量 | SDK 已内置重试，再加一层是乘法 |

### 真实缺口（scope 应认真对待）

| # | 能力 | 说明 | 建议 |
|---|---|---|---|
| 1 | **`chatclient` 层的开箱即用 tool loop** | Spring 的 `ToolCallingAdvisor` 一行搞定"模型自动调工具"。scope 目前要么手写循环，要么上整个 Agent Framework | **高优先级重估**。`ARCHITECTURE.md §7.1` 把"是否暴露更小的直接 Runner"悬置，理由是缺独立消费者证明 —— 现在 Spring AI 与 MAF 两个独立项目都提供了它，证据成立。落点：`chatclient` 的一个 `CallMiddleware`（组合形态，不新造机器），明确无 snapshot/steer/budget 语义，需要这些就升级到 `agent/interaction` |
| 2 | **`SafeGuardAdvisor` / 内容安全前置** | 敏感词/策略前置拦截 | scope 有 `core/moderation` 但没有 chatclient 层的 guard middleware。**低成本**，纯组合形态 |
| 3 | **`UsageAccumulator`（跨多次调用累计 usage）** | 一次 ChatClient 调用内含多轮 tool loop 时累计 token | 与缺口 1 绑定，一并处理 |
| 4 | **`StructuredOutputValidationAdvisor`** | 结构化输出失败时带校验反馈重试 | scope 的 `chatclient/output_format.go` 有结构化输出，但没有"校验失败→反馈→重试"。**注意**：这与 `agent/interaction` 的 `completion validator` 是同一机制的两个层级，设计时要保证不是第二套 |
| 5 | **`document-readers/*` 的格式覆盖面** | Spring 有 tika、jsoup、pdf、markdown、json 等 | scope `etl` 有 text/json/markdown/html/pdf。**覆盖面差距真实但非结构性**，按需增补 |

### 判词

> **Spring AI 是 scope 的词汇表来源，也是 scope 大部分反向不变量的具体所指。**
> 它证明了这个分层（Model / Client / Document / VectorStore / Tool / RAG）是对的 —— scope 一比一继承了它，且这些继承在四年后仍然站得住。
> 它同时证明了 Java + Spring 的三样东西不该被移植：**泛型基接口**（不承载行为的类型层级）、**Ordered 扩展链**（把语义约束编码成魔法整数）、**auto-configuration**（把装配藏进容器）。scope 的对应条款不是标新立异，是对同一批问题在无 DI 容器的语言里的重新作答。
> 唯一值得立刻补的缺口是最朴素的那个：**一个不需要 Engine 的、开箱即用的 tool loop**。Spring AI 用一个 Advisor 解决它，MAF 用一个默认 middleware 解决它 —— 两个独立证明，scope 该给它一个位置了。
