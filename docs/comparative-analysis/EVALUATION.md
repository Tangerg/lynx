# Evaluation：通用质量内核，不是运行时排名项

Evaluation 属于框架的支持层，但不应与 Agent 执行内核混成一个总分。一个项目可以拥有成熟的评估服务，却没有 durable execution；也可以拥有严格恢复语义，却只提供很小的评估内核。

## Scope 当前定位

Scope 的 `evaluation` 已经不再以 RAG sample 作为根抽象。当前公共窄腰是：

```go
type Evaluator[T any] interface {
    Evaluate(context.Context, T) (Report, error)
}
```

围绕这一个泛型 subject，内核提供：

- 带稳定 ID 和 metadata 的 `Case[T]`；
- 有界并发、collect/fail-fast 策略和分位数汇总的 `Runner[T]`；
- 结构化 `Metric` 身份；
- `Passed`、归一化 `Score`、Feedback、Metadata 和嵌套 Details；
- 权重、required component 和 pass policy 明确的并发 Composite；
- 将聚合数据投影成窄 subject 的 ProjectionEvaluator。

文本 judge、回答相关性、正确性、groundedness 位于 `evaluation/text` 和 `evaluation/judge`；precision、recall、MRR、NDCG 位于 `evaluation/retrieval`。RAG 已经是一个领域包，不是内核的数据模型。

因此，若问题是“它是否仍然专为 RAG 打造”，答案是 **否**。它现在是一个通用的、以归一化质量判断为中心的 evaluation kernel。

但“通用”有明确边界：

- 所有结果最终都要求 `[0,1]` 且越高越好的 `Score`。
- `Report` 以 pass/fail + scalar score 为主，适合质量指标，不是任意实验事件模型。
- 内核没有实验版本、baseline/candidate 配对、统计显著性、数据集存储、运行制品和远程服务。
- 它不负责捕获 Agent trace，也不直接绑定 Agent Runner。

这不是残留 RAG 味道，而是一个合理的产品边界：**通用质量计算库，不是完整实验平台。** 若未来确有非标量、越低越好或多目标 Pareto 需求，应通过新的结果协议重新设计，不能把方向、单位等含义塞进 metadata 打补丁。

## 与同类项目的真实差异

| 项目 | 已实现的 evaluation 形状 | 通用性判断 |
| --- | --- | --- |
| Scope | 泛型 subject、统一 Report/Metric、Runner、Composite；文本和检索分包 | 对质量评分领域通用，不绑定 Agent 或 RAG |
| tRPC-Agent-Go | AgentEvaluator、Runner、EvalSet、trace、metric registry、结果存储、服务端、用户模拟和多轮运行 | 产品面最完整，但核心直接绑定 Agent Invocation/Runner |
| Spring AI | `Evaluator` 接口；`EvaluationRequest` 固定 user text、`Document` 列表和 response content | 表面简单，但数据模型明显偏 Chat/RAG |
| Pi | 私有 `pi-evals`，基于 `vitest-evals` 驱动真实 Coding Agent session 并保存制品 | 是高价值的应用行为评估，不是公开通用框架模块 |
| Microsoft AF Go | loop middleware 中的 `Evaluator` 决定是否继续运行并返回反馈 | 本质是循环终止/反思策略，不是离线质量评估内核 |
| Eino | 本次主仓库未发现同等级公共 evaluation 子系统 | 不据此评价运行时设计 |
| ADK Go | 本次 Go 主仓库只发现尚未实现的 Eval API 路由 | 未形成与 Scope/tRPC 同形的公共独立内核 |
| Embabel | 文档中有 eval 指引，核心 API 未发现同形通用评估库 | 未形成主框架窄腰 |

## 两个最有价值的对照

### 对比 tRPC-Agent-Go

tRPC-Agent-Go 的 evaluation 更接近完整 Agent 评估产品：它能驱动 Runner，管理 eval set/result/metric，聚合多次运行，记录 trace，并提供服务端。它适合“评估一个运行中的 Agent 应用”。

Scope 的 evaluation 更小：任何 `T` 都能被评估，既不需要 Agent，也不需要模型。它适合被测试、离线任务、CI 或 Flame 自己的实验层复用。

Scope 不应把 tRPC 的 AgentEvaluator 整体吸入通用内核。若 Flame 需要数据集存储、实验编排或 trace 采集，应在应用/实验层组合 `evaluation.Runner` 与 Scope Agent 事件，而不是让 `evaluation` 反向依赖 `agent`。

### 对比 Pi

Pi 的私有 evals 做了 Scope 当前没有做的实验实践：真实 session、临时目录隔离、运行制品、baseline/candidate、重复运行，以及将 token、latency、cost 作为独立差值保留。它没有强行把所有信号压成一个“总质量分”。

Scope 应借鉴这种实验层方法，但位置应保持在外部 harness 或 Flame：

- `evaluation` 负责单项指标、组合和运行汇总；
- Agent/Flame harness 负责执行、trace 和制品；
- 实验层负责 baseline/candidate、重复、随机化和统计解释。

三层合并会让通用内核重新变成某个应用的评估系统。

## 修正后的结论

Scope evaluation 当前已经足够支撑多个领域实现同一套质量评估协议，不能再称为“RAG 专用”。它尚未、也不应自称完整实验平台。

目前最重要的边界不是继续增加内置指标，而是保持：

1. 根包不依赖 `agent`、RAG、Chat 或具体模型。
2. 领域 subject 和指标词汇只存在于叶子包。
3. trace、制品、数据集持久化和实验对照由上层拥有。
4. 一旦出现标量分数无法诚实表达的领域，升级结果协议，而不是滥用 metadata。
