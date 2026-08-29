# 可观测性 / 可评估性 专项审计

> 审计范围：`app` 之外全仓，重点 `otel`、`evaluation`、`agent`。
> 与 [`CODE_SMELLS.md`](CODE_SMELLS.md) 的关系：那份是通用坏味道，这份只看**这两项能力本身够不够**。
> 交叉引用：`CODE_SMELLS.md` 的 **A6**（rag / mcp / a2a 自埋点违反法则）、**A3**（delta 观测链 context 断裂）也属可观测性，不在此重复。
>
> 每条都给 `file:line` 证据。未修改任何代码。

---

## 结论先说

**可观测性**：`otel/chat` 和 `otel/agent` 的**实现质量很高**（GenAI semconv、typed instrument、流式 TTFT 事件、早停处理）。问题不在做得糙，在两处：

1. **覆盖面窄** —— 11 个 core package 覆盖 3 个，8 个能力模块覆盖 1 个。**`core/tool` 没有埋点**，而工具调用是 agent 系统里运维价值最高的 span。
2. **领域语义没进遥测** —— agent 有精心设计的 `Failure{Kind, Code}` 分类法和数十个稳定 failure code，**一个都没进 event 载荷**，因此 otel 想记也记不到。生产事故只能看到 `execution_failure`。

**可评估性**：确实偏弱，但最根本的问题在「缺功能」之上一层 —— **模块定位没兑现（E0）**。

它已从 `rag/evaluation` 搬到顶层并在 `doc.go` 声明「不依赖 RAG」，但**内容仍然 100% 是 RAG 评估**：6 个具体评估器全部属于 RAG（2 个 RAGAS 模型评判 + 4 个 IR 排序指标），`Evaluator[T]` 的泛型参数**从未被任何具体类型实例化**，模型评判器被未导出的 `promptVariables{Input, Output, Context}` 锁死在 RAG 形状 —— 想为 agent 轨迹或带参考答案的摘要写模型评判器，没有任何入口。仓库内零消费者（连 `examples` 都不依赖）。

反差最大的是：`agent` 是仓库最大模块，有 `TreeSnapshot` / `Event` / `Transition` / `Failure` / `Usage` / `Budget` 等十种现成的可评估结构，`evaluation` 一条都没覆盖。

在此之上还缺两个形状：**数据集抽象**（E1，只能评单样本，聚合/通过率/回归对比全要调用方自己写）和**参考答案**（E3，`TextSample` 无 `Expected`，做不了 golden answer 对比）。

注意：现有代码**质量没问题** —— 覆盖率 91.7%，检索指标（precision / recall / MRR / NDCG）数学实现经核对是正确的，除零路径都被 `Validate()` 挡住。问题是**能力面**，不是实现缺陷。

---

# 第一部分：可观测性

## O1 · otel 覆盖矩阵 —— core 3/11，能力模块 1/8

| core package | 有无 wrapper | 备注 |
|---|---|---|
| `core/chat` | ✅ `otel/chat` | 质量高 |
| `core/history` | ✅ `otel/history` | 仅 traces，见 O5 |
| `core/vectorstore` | ✅ `otel/vectorstore` | 仅 traces，见 O5 |
| **`core/tool`** | ❌ | **最高价值缺口** |
| **`core/embedding`** | ❌ | RAG 索引与查询的主要成本项 |
| `core/image` / `moderation` / `speech` / `transcription` | ❌ | 均为付费 provider 调用 |
| `core/jsonschema` / `tokenizer` / `document` / `media` / `metadata` | ❌ | 纯值/工具包，本就不需要 |

| 能力模块 | 有无 wrapper | 备注 |
|---|---|---|
| `agent` | ✅ `otel/agent` | 质量高，但见 O2 / O3 / O6 |
| `rag` / `mcp` / `a2a` | ❌ | **自埋点，违反法则**，见 `CODE_SMELLS.md` A6 |
| `etl` / `tools` / `skills` / `evaluation` | ❌ | 完全无观测 |

**`core/tool` 的缺失最值得优先补**：它和 `core/chat` 是同一层的付费/高延迟调用边界，`otel/chat` 的 middleware 形态可以直接照搬。当前工具执行的耗时、失败率、调用分布全部不可见（`agent` 侧的 effect span 也不带工具身份，见 O3）。

---

## O2 · agent 的失败分类法对遥测完全不可见 ★

### 事实

`agent` 有两层失败语义，**只有粗的那层进了 event 载荷**：

**细粒度（真正的诊断信息）—— 完全不可见**

```go
// agent/failure.go:22-31
FailureKindExecution FailureKind = "execution"   // 普通 Strategy 执行失败
FailureKindContract  FailureKind = "contract"    // Framework/Strategy 契约违反
FailureKindExternal  FailureKind = "external"    // 外部基础设施失败
FailureKindPanic     FailureKind = "panic"       // 执行边界恢复的 panic
```

配套的稳定 failure code（仅 `engine.*` 前缀就有数十个）：

```
engine.batch                          engine.child.budget_exhausted
engine.capability.denied              engine.child.budget_invalid
engine.child.admission.rejected       engine.child.capability_escalation
engine.child.completion.invalid       engine.child.deployment_unavailable
engine.child.tree_limit               engine.child.identity_conflict        ...
```

**粗粒度（进了载荷）**

```go
// agent/event_payload.go:36-39
type processFinishedEventPayload struct {
	ProcessStatus    Status           `json:"process_status"`
	TerminationCause TerminationCause `json:"termination_cause"`
}
```

`TerminationCause` 共 10 个值（`agent/termination.go:157-175`），失败相关的只有 `execution_failure` / `contract_failure` 两个。

**核实结论**：`Failure` 在 `agent/event_payload.go` 中**零出现**。

### 后果

生产环境一个子进程启动失败，遥测里能看到的全部信息是：

```
agent.process.status = failed
agent.process.cause  = execution_failure
```

**看不到**是 `engine.child.tree_limit`（树规模超限）、`engine.child.budget_exhausted`（预算耗尽）、还是 `engine.capability.denied`（能力被拒）—— 这三者的处置方式完全不同。

而按法则「不在业务代码里撒 slog」，**也没有日志可以兜底**。

### 根因与治本层

根因**不在 `otel/agent`，而在 `agent` 的 event 载荷协议** —— otel 拿不到就记不了。治本要在 `processFinishedEventPayload`（及 step / effect 对应载荷）中加入 `FailureKind` 与 `Code`，再由 `otel/agent` 落成 attribute（`agent.failure.kind` / `agent.failure.code`）与 metric 维度。

属于破坏性协议改动（event 载荷是 wire shape），需先确认。

---

## O3 · 每次工具调用产生完全相同的匿名 span ★

### 事实

```go
// agent/event_payload.go:25-29
type effectFinishedEventPayload struct {
	EffectTarget     EffectTarget     `json:"effect_target"`
	SettlementStatus SettlementStatus `json:"settlement_status"`
	DurationMS       int64            `json:"duration_ms"`
}
```

而 `EffectTarget` 只有**两个**有效值（`agent/effect.go:18-23`）：

```go
EffectTargetFramework  EffectTarget = "framework"
EffectTargetDispatcher EffectTarget = "dispatcher"
```

`otel/agent/observer.go:395,405` 把它作为 `agent.effect.target` 记录。

### 后果

agent 框架里**每一次工具调用**（全部走 `EffectTargetDispatcher`）产生的 span 形状完全一致：

```
name: agent.effect
agent.effect.target = dispatcher
agent.effect.status = succeeded | failed | unknown
agent.effect.duration = 340ms
```

无法回答任何一个最常见的 agent 运维问题：

- 哪个工具最慢？
- 哪个工具失败率最高？
- `web_search` 今天被调用了多少次？
- 某次会话到底调了哪些工具？

### 为什么不是「照做就行」

工具身份位于 `Effect.Payload`（`json.RawMessage`），对 Engine 是**不透明的** —— 这是对的，Effect 载荷由 Strategy 定义，Engine 不应解析。

所以这是**框架缺一个 seam**：Strategy 没有任何途径向 Engine 贡献一个可观测标签。治本方向是在 Effect 上增加一个显式的、低基数的 `label` / `operation` 字段（由 Strategy 填写，Engine 只透传不解释），而不是让 Engine 去解析 payload。

同样属于协议改动。

---

## O4 · `error.type` 维度实际上是常量

### 事实

```go
// otel/chat/chat.go:336-341
func errorType(err error) string {
	if err == nil { return "" }
	return fmt.Sprintf("%T", err)
}
```

用于 metric 维度（`:215`）与流式累积失败事件（`:152`）。

而项目法则强制**所有错误包装用 `%w`**。实测 `%T` 在各种包装形态下的结果：

| 错误形态 | `%T` 结果 |
|---|---|
| 裸 sentinel `errors.New(...)` | `*errors.errorString` |
| `fmt.Errorf("...: %w", err)` | `*fmt.wrapError` |
| `fmt.Errorf("%w: ...: %w", a, b)` | `*fmt.wrapErrors` |
| `errors.Join(a, b)` | `*errors.joinError` |

也就是说 `error.type` 维度的取值空间实际只有这 4 个，**且与失败原因无关**。无法区分限流、鉴权失败、超时、模型不存在。

### 更严重的一点

`core/chat/model.go` 的接口契约明确承诺：

> Cancellation errors must retain `context.Canceled` or `context.DeadlineExceeded` for `errors.Is`.

契约特地保住了取消语义的可识别性，但 `%T` 把它抹平成 `*fmt.wrapError` —— **指标里无法把用户主动取消和真实故障分开**，而这是计算错误率时必须排除的一类。

### 覆盖面问题

`error.type` **只有 `otel/chat` 记录**。`otel/history`、`otel/vectorstore`、`otel/agent` 完全不记（实测 `ErrorTypeKey` 出现次数均为 0），且整个 `otel` 模块**零处使用 `errors.Is` 做分类**。

### 治本方向

用 `errors.Is` 对已知 sentinel 分类，产出稳定的低基数标签（如 `context.canceled` / `deadline_exceeded` / `invalid_request` / `invalid_output` / `unknown`），而不是反射类型名。分类表应由拥有 sentinel 的层提供。

---

## O5 · `otel/history` 与 `otel/vectorstore` 只有 traces，没有 metrics

### 事实

两个包**零 metric 仪表**，且 `MiddlewareConfig` 连 `MeterProvider` 字段都没有：

```go
// otel/history/history.go:46-49
type MiddlewareConfig struct {
	System         string
	TracerProvider trace.TracerProvider
}

// otel/vectorstore/vectorstore.go:40-45
type MiddlewareConfig struct {
	System         string
	Collection     string
	Namespace      string
	TracerProvider trace.TracerProvider
}
```

对比 `otel/chat/chat.go:41-45` 是有 `MeterProvider` 的。所以这不是接线遗漏，是**结构性缺失**。

### 判据

法则原文：

> **可观测性 = OTel 三驾马车**：观测 = Traces（span）+ Metrics（instrument）+ Logs

四个 instrumentation 包中有两个只提供了三驾马车里的一驾。

向量检索的核心运维指标 —— 检索延迟直方图、返回结果数分布、按 collection 的 QPS —— 目前只能从 span 采样里捞，无法作为 metric 告警。

---

## O6 · `otel/agent` 零 `RecordError`

实测 `otel/agent/observer.go` 中 `RecordError` 出现 **0** 次，`SetStatus` 出现 4 次，且状态描述是手写字符串：

```go
:226  record.span.SetStatus(codes.Error, "OpenTelemetry observer closed before span completion")
:290  record.span.SetStatus(codes.Error, payload.TerminationCause.String())
:340  record.span.SetStatus(codes.Error, "Execution Step failed")
:399  record.span.SetStatus(codes.Error, "Effect attempt "+payload.SettlementStatus.String())
```

法则：

> 错误走 `span.RecordError` + `SetStatus`

对比 `otel/chat/chat.go:200-201` 是两者都做的。

与 O2 叠加后果更明显：既没有 `RecordError` 的异常记录，`Failure` 又不在载荷里，agent 的失败在 trace 里只剩一句手写字符串。

---

## O7 · TTFT 只是 span event，不是 metric

```go
// otel/chat/chat.go:146
span.AddEvent(firstTokenReceivedEvent)
```

首字延迟（TTFT）是流式对话最关键的用户体感指标，目前只作为 span 事件存在 —— 只能在单条 trace 里看到时间戳，**无法对 p99 TTFT 建图或告警**。

（说明：OTel GenAI semconv v1.41 尚未定义 client 侧 TTFT 指标，所以这不算违反 semconv；但作为能力缺口仍然成立，可用自有 histogram 补，命名遵循法则的「去品牌 + 裸 domain 名」。）

---

# 第二部分：可评估性

## E0 · 定位与内容不符：一个自称通用的模块，内容 100% 是 RAG 评估 ★★ 最根本

> 这条先于 E1–E8。它不是缺功能，是**模块定位没兑现** —— 其余各条的取舍都取决于它怎么定。

### 它确实是从 rag 里搬出来的

不是猜测，有三条硬证据：

1. **架构测试把 `rag/evaluation` 列为已退休布局**（`dev/repoarch/architecture_test.go:341`）—— 说明这个位置被明确否决过，不允许退回。
2. **git 历史**：最早是 `feat(rag): add model-backed evaluation`（`96c709324`），住在 `rag/evaluation`，后由 `refactor: align module ownership boundaries`（`64010f987`）搬到顶层。
3. **`doc.go` 的声明**：「The package does not depend on RAG, documents, or a storage implementation.」

所以项目已经做过裁决：**evaluation 应该是顶层通用能力**。问题是 —— **搬了位置，没搬内容**。

### 内容盘点：骨架通用，血肉全是 RAG

**通用骨架（真的通用）**

| 符号 | 位置 |
|---|---|
| `Evaluator[T any]` / `EvaluatorFunc[T]` | `evaluator.go:6,13` |
| `Report{Metric, Passed, Score, Feedback, Metadata, Details}` | `report.go:12` |
| `Score` / `DefaultThreshold` | `score.go:8,11` |
| `Metric` | `metric.go:9` |
| `CompositeEvaluator[T]` | `composite.go:13` |

**具体实现（全部是 RAG 评估，无一例外）**

| 符号 | 本质 |
|---|---|
| `TextSample{Input, Output, Context []string}` | `Context` 就是 RAG 检索上下文 |
| `GroundednessEvaluator` | RAGAS faithfulness |
| `AnswerRelevanceEvaluator` | RAGAS answer relevance |
| `MetricGroundedness` / `MetricAnswerRelevance` | 同上 |
| `RetrievalSample{Retrieved, Relevant}` | IR 排序评估 |
| `RetrievalMetric` + `RetrievalEvaluator`（precision / recall / MRR / NDCG） | IR 排序质量 |

**具体评估器数量：6 个（2 个模型评判 + 4 个检索指标）。属于 RAG 的：6 个。**

`doc.go` 那句声明在**依赖层面成立**（`evaluation/go.mod` 只依赖 `core` + `lo`，确实不依赖 rag），但在**语义层面不成立**。

### 泛型只停在接口层，穿不透到实现

这是最能说明问题的一点 —— 模块看起来泛型，实际泛型参数从未被使用：

**1. `Evaluator[T]` 从未被任何具体类型实例化**

全模块 `Evaluator[...]` 共出现 6 次，**全部是 `Evaluator[T]`**（即 `CompositeEvaluator[T]` 自身泛型声明内部的引用）。没有一处 `Evaluator[TextSample]` 或 `Evaluator[RetrievalSample]`。也**没有任何编译期断言**（零处 `var _ Evaluator[...] = ...`）来证明具体 evaluator 满足这个接口。

**2. 模型评判基础设施被写死成 RAG 形状**

```go
// evaluation/model.go:51-55 —— 未导出的封闭结构
type promptVariables struct {
	Input   string
	Output  string
	Context string
}
```

`modelEvaluator.evaluate`（`model.go:118-120`）永远以这个结构渲染模板：

```go
message, err := evaluator.prompt.UserMessage(promptVariables{
	Input: sample.Input, Output: sample.Output, Context: sample.ContextText(),
})
```

`newModelEvaluator` 看起来是可复用的通用设施（接收 metric、defaultPrompt、required 变量名），**但它只能接受 `TextSample`**。想为 agent 轨迹、分类结果、带参考答案的摘要写一个模型评判器 —— 没有任何入口，必须绕开整个 `modelEvaluator` 重写。

结论：`Evaluator[T any]` 的泛型是**推测性的**，它没有承载任何真实的多态。

**3. 仓库内零消费者**

除 `app` 外，全仓无人 import `evaluation` —— 连 `examples` 都不依赖它（`examples/go.mod` 无此项）。所以这个「通用」抽象从未被第二个用例压过。

### 反差最大的地方：agent 的可评估素材一条都没用上

`agent` 是仓库最大的模块（22.5k 行），且是**评估素材最丰富的模块**：

```
agent.TreeSnapshot   agent.Snapshot     agent.Event      agent.Transition
agent.Failure        agent.Termination  agent.Settlement agent.Delta
agent.Usage          agent.Budget
```

有完整的步骤序列、Effect 结算、失败分类、预算消耗、进程树结构 —— 天然可以评 trajectory 合理性、工具选择正确性、任务完成度、预算效率。

**`evaluation` 一条都没有覆盖。** 一个自称通用的评估模块，对同仓最大、最需要评估的能力模块零支持。

### 两条路，其中一条已经被否决

**路线 A：承认它是 RAG 评估，收回 `rag`**
—— 已被否决。`rag/evaluation` 在退休布局清单里，不允许退回。

**路线 B：真正做成通用评估器**
—— 这是既定方向，需要补两件事：

1. **分层**：当前是**单一扁平 package**（`find evaluation -type d` 只有一个目录），通用内核与 RAG 实现完全混在一起。至少要把 `Evaluator` / `Report` / `Score` / `Metric` / `Composite` 与 RAG 专用的 sample + evaluator 分开表达，否则「通用」永远只是文档里的一句话。
2. **让泛型穿透**：把 `promptVariables` 从封闭结构改为由 subject 类型提供模板变量（例如约束 `T` 提供 `TemplateVariables() any`，或让 `ModelEvaluatorConfig` 泛型化），使模型评判器能服务 `TextSample` 之外的 subject。**这是通用化的技术关键点** —— 不解决它，加再多指标也还是 RAG 评估。
3. **补至少一个非 RAG 评估域验证抽象**：agent 是最自然的选择，素材现成。

### 与 E1 / E3 的关系

E1（无数据集抽象）、E3（无参考答案）是**在通用方向上继续缺的东西**；E0 是**方向本身还没走到位**。

建议顺序：**先定 E0 的分层与泛型穿透方案，再一并设计 E1 + E3** —— 因为数据集抽象的形状（`Dataset[T]`?）和参考答案字段放哪（`TextSample` 还是通用 sample 契约）都取决于 E0 怎么分层。反过来做必然返工。

---

## E1 · 没有数据集 / 批量 / 聚合抽象 ★ 最根本的缺口

### 事实

全模块唯一的评估入口是**单样本**：

```go
// evaluation/evaluator.go:6-11
type Evaluator[T any] interface {
	Evaluate(ctx context.Context, subject T) (Report, error)
}
```

全部 5 个 `Evaluate` 实现都是单样本签名（`answer_relevance.go:39`、`groundedness.go:39`、`retrieval.go:210`、`composite.go:57`、`evaluator.go:15`）。

实测：全模块**不存在** `Dataset` / `Suite` / `Batch` / `Aggregate` / `Summary` / `[]TextSample` / `[]RetrievalSample` 任何形态的 API。

### 为什么这是根本问题

评估的本质是**在数据集上做聚合判断**，而不是对单个样本下结论。一个可用的评估框架至少要能回答：

- 这 500 条样本的 groundedness 平均分 / p50 / p10 是多少？
- 通过率是多少？
- 相比上一个版本，哪些样本从 pass 变成了 fail？（回归检测）
- 哪些样本得分最低？（错误分析）

当前这些**全部要调用方自己写**：自己循环、自己收集 `[]Report`、自己算均值和通过率、自己做版本对比。模块只提供了最里层的一次判断。

`Report` 有 `Details []Report` 用于组合评估的子报告，但那是**同一个样本的多指标**，不是**多个样本的同一指标** —— 两个维度，当前只支持前者。

### 与仓库其他模块的对比

`core/vectorstore` 有 `IndexRequest` 自持批量与分批规则；`rag` 有 `parallelResults` 泛型并行。`evaluation` 是唯一停在单点 API 的模块。

---

## E2 · `CompositeEvaluator` 串行执行

```go
// evaluation/composite.go:57-72
func (composite *CompositeEvaluator[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	for index, evaluator := range composite.evaluators {
		if err := ctx.Err(); err != nil { return Report{}, err }
		report, err := evaluator.Evaluate(ctx, subject)   // ← 逐个串行
		...
	}
	return reports.combine()
}
```

对 model-backed evaluator，每个子评估都是一次 LLM 往返。组合 4 个指标 = **4 次串行 LLM 调用**，延迟线性叠加，而它们之间没有任何数据依赖。

仓库自己已有现成范式 —— `rag/retriever.go:180` 的泛型并行助手（用 `wg.Go`，Go 1.25+ 写法）：

```go
func parallelResults[Item, Out any](
	ctx context.Context, op string, items []Item, itemLabel string,
	fn func(context.Context, int, Item) (Out, error),
) ([]Out, error)
```

叠加 E1 后果更重：真做数据集评估时是 `样本数 × 指标数` 次串行 LLM 调用。

---

## E3 · 没有参考答案（reference-based）能力

```go
// evaluation/text_sample.go:13-17
type TextSample struct {
	Input   string   `json:"input,omitzero"`
	Output  string   `json:"output,omitzero"`
	Context []string `json:"context,omitzero"`
}
```

**没有 `Expected` / `Reference` / `Golden` 字段。**

因此无法表达任何一类参考答案指标：

- correctness（输出 vs 标准答案）
- exact match / F1（抽取类任务的标准指标）
- semantic similarity（可用现成的 `core/embedding` 算）

这是评估框架最基础的一类能力 —— 有标注数据集时，「答案对不对」是第一个要问的问题。当前只能做无参考评估（groundedness / relevance）。

注意 `RetrievalSample` 是有 ground truth 的（`Relevant []string`），说明模块并非有意排斥标注数据 —— 只是文本侧漏了。

---

## E4 · LLM-as-judge 单次采样，无自洽性

`modelEvaluator.evaluate`（`evaluation/model.go:113-133`）对每个样本只做**一次**模型调用，直接采信返回的分数。

judge 输出方差是这项技术公认的主要弱点。常见缓解是 N 次采样取中位数/多数，或至少把方差暴露出来。当前 `ModelEvaluatorConfig`（`:20-24`）只有 `Model` / `PromptTemplate` / `Threshold` 三个字段，没有采样次数或聚合策略的位置。

---

## E5 · 组合语义过于简陋

```go
// evaluation/composite.go:24-38
combined := Report{Metric: MetricComposite, Passed: true, Details: reports}
for _, report := range reports {
	combined.Passed = combined.Passed && report.Passed   // 纯 AND
	combined.Score += report.Score
}
combined.Score /= Score(len(reports))                    // 等权平均
```

- **等权平均**：groundedness 0.9 + relevance 0.1 = 0.5。无法表达「groundedness 权重更高」。
- **纯 AND**：无法表达「3 个指标过 2 个即可」。
- **无策略选择**：无法配置 fail-fast（一挂就停，省 LLM 调用）还是全跑完（拿完整诊断）。当前固定是 fail-fast（`:63` 首个错误即返回），但这是隐含行为，且「首个错误」和「首个不通过」是两回事 —— 不通过不会中断。

---

## E6 · Metric 身份是 stringly-typed，且分成两套注册表

```go
// evaluation/metric.go:13-19 —— 注册表一：3 个常量
MetricGroundedness    Metric = "groundedness"
MetricAnswerRelevance Metric = "answer_relevance"
MetricComposite       Metric = "composite"

// evaluation/retrieval.go:31-37 —— 注册表二：字符串拼装
func (metric RetrievalMetric) reportMetric(cutoff int) (Metric, error) {
	reportMetric := Metric(fmt.Sprintf("retrieval/%s@%d", metric, cutoff))
	...
}
```

`Metric.Validate()`（`metric.go:20-27`）只校验「非空且无首尾空白」—— 实际是开放字符串。

于是检索指标的身份变成了 `"retrieval/precision@5"` 这样的**结构化信息编码进字符串**。消费者要按指标族分组、或提取 cutoff 做对比，只能反过来解析字符串。

这与法则的「禁止魔法 / 稳定词汇用具名 value object」方向相悖。若要保留可读的扁平 ID，至少应让 `Metric` 拥有解析与构造两个方向，而不是只有单向 `Sprintf`。

---

## E7 · evaluation 模块自身零可观测性

实测 `evaluation` 模块 OTel 源文件 import = 0、go.mod 声明 = 0，且不存在 `otel/evaluation`。

所以：

- LLM-judge 调用的**延迟、token 消耗、失败率全部不可见**（它走的是 `chatclient`，而 `otel/chat` 是 middleware —— 除非调用方自己在构造 `ModelEvaluatorConfig.Model` 时手工套上，否则无任何遥测）
- 评估运行本身没有 span，无法看到一次评估跑了多久、卡在哪个指标

对一个**以测量为唯一职责**的模块来说，自己不可测量是个结构性问题。叠加 E1（无数据集抽象），一次大规模评估既没有进度可见性，也没有成本可见性。

---

## E8 · 只有 2 个 model-backed 指标，RAG 评估三元组缺一角

现有 model-backed 指标：

- `MetricGroundedness`（`groundedness.go`）
- `MetricAnswerRelevance`（`answer_relevance.go`）

业界 RAG 评估通常看三元组：**faithfulness/groundedness + answer relevance + context relevance**。第三项（检索到的上下文与问题是否相关）缺失。

注意 `RetrievalEvaluator` 的 precision/recall/NDCG **不能替代** context relevance —— 前者需要人工标注的 `Relevant []string` 相关性判断，后者是模型判定的、无需标注。两者适用场景不同。

其他常见缺口：toxicity / safety、coherence、conciseness。

**这一条优先级低于 E1–E3** —— 加指标是叠加式工作，而 E1/E3 是形状问题，形状定了再加指标才不会返工。

---

# 已核实为「做得好」的部分

避免重复排查，以下经核对确认没问题：

| 项 | 结论 |
|---|---|
| `otel/chat` 语义规范 | 使用官方 GenAI semconv v1.41.0 + `genaiconv` typed instrument，非手搓 attribute |
| `otel/chat` 流式处理 | 惰性启动、首字事件、累积失败降级为事件不污染业务错误、消费者早停正确收尾（`chat.go:117-167`） |
| `otel/chat` token 指标 | input / output 分别记入 `ClientTokenUsage` 直方图（`:228-243`） |
| `otel/agent` 指标面 | 5 个仪表：process.starts / process.exits / step.duration / effect.duration / delta.dropped |
| `otel/agent` span 生命周期 | Close 时兜底结束未完成 span（`:224-228`），不泄漏 |
| `otel/slog` | traces / metrics / logs 三个 exporter 各实现 OTel SDK 标准接口，未自造抽象 |
| 检索指标数学正确性 | precision@k / recall@k / MRR / NDCG@k 实现经逐条核对无误；`discountedGain = 1/log2(rank+1)`、IDCG 取 `min(cutoff, |relevant|)` 均正确 |
| 检索指标除零安全 | `Validate()` 强制 `len(Relevant) ≥ 1`、`Cutoff > 0`，三个除法分母均不可能为 0 |
| `Score` 边界语义 | 拒绝 NaN / Inf / 越界；`Passes` 显式拒绝无效值而非让 NaN 比较语义泄漏（`score.go:31-36`） |
| `Report.Clone` | 深拷贝含递归 Details（`report.go:22-30`） |
| evaluation 测试覆盖率 | 91.7% |
| evaluation 依赖方向 | 不依赖 rag / document / 存储实现，符合 doc.go 声明 |

---

# 建议处理顺序

## 可观测性

| 顺序 | 项 | 理由 | 破坏性 |
|---|---|---|---|
| 1 | **O4** error.type 分类 | 改 `otel/chat` 一个函数，立刻让错误率指标可用 | 否 |
| 2 | **O6** 补 `RecordError` | 4 处，纯补齐 | 否 |
| 3 | **O5** 补 metrics | 两个包加 `MeterProvider` + 延迟直方图，照抄 `otel/chat` | 加字段，向后兼容 |
| 4 | **O7** TTFT histogram | 自有仪表，命名去品牌 | 否 |
| 5 | **O1** 补 `otel/tool` | 价值最高的缺口，形态照搬 `otel/chat` middleware | 新增包 |
| 6 | **O2 / O3** 失败分类 + effect 标签 | **需先确认**：改 event 载荷 wire shape | **是** |
| 7 | O1 其余（embedding 等） | 叠加式 | 新增包 |

## 可评估性

| 顺序 | 项 | 理由 |
|---|---|---|
| **1** | **E0** 定位：分层 + 泛型穿透 | **一切的前提**。不解决 `promptVariables` 写死，加再多指标仍是 RAG 评估 |
| 2 | **E1** 数据集 / 聚合抽象 | 形状问题，与 E0 同批设计（`Dataset[T]` 的形状取决于 E0 怎么分层） |
| 3 | **E3** 参考答案字段 | 同为形状问题；放 `TextSample` 还是通用 sample 契约，取决于 E0 |
| 4 | **E2** 并行 | 复用 `rag/retriever.go:180` 的 `parallelResults` 形态；E1 定型后做收益最大 |
| 5 | **E5** 加权 / k-of-n | 组合语义，依赖 E1 的聚合形状 |
| 6 | **E4** judge 自洽性采样 | `ModelEvaluatorConfig` 加字段 |
| 7 | **E7** `otel/evaluation` | 依赖 E1 —— 有了 run 的概念才有 span 可挂 |
| 8 | **E6** Metric 身份结构化 | 独立，可随时做 |
| 9 | **E8** 补指标（含 agent 评估域） | 形状定完再加，避免返工 |

**E0 / E1 / E3 应当作为一次设计一起定型**，不要逐个补 —— 三者都是形状问题，且互相约束：分层决定 sample 契约放哪，sample 契约决定参考答案字段放哪，两者共同决定数据集抽象长什么样。任意先做一个，另外两个都会推翻它。

**关于「evaluation 偏弱」的判断**：准确，且弱点比「缺功能」更靠上一层 —— 是**定位没兑现**（E0）。模块已经从 `rag/evaluation` 搬到顶层并声明通用，但内容仍是 6 个 RAG 评估器、泛型参数零实例化、模型评判器被 `promptVariables{Input,Output,Context}` 锁死在 RAG 形状、仓库内零消费者。已写的代码质量没问题（91.7% 覆盖率、检索指标数学正确），问题是它还没变成它声称的那个东西。
