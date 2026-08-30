# Eval：通用评估运行时，不是实验应用平台

Eval 是 Scope 的独立支持层。它可以评估任意类型的 subject，但不参与 Agent 的执行、恢复或调度，也不依赖 RAG、Chat 或具体模型。

## 公共窄腰

根包只要求一个行为：

```go
type Evaluator[T any] interface {
    Evaluate(context.Context, T) (Report, error)
}
```

围绕这个接口，`eval` 提供：

- 不可变、顺序稳定且 ID 唯一的 `Dataset[T]`；
- 有界并发及 collect/fail-fast 语义明确的 `Experiment[T]`；
- 保留异构结果的 `SuiteEvaluator`；
- 显式 weight、required component 与 pass policy 的 `CompositeEvaluator`；
- 按完整 Metric identity 聚合的 summary 和分位数分布；
- 对同一 ordered Dataset 与同一 Metric identity 做精确 delta 的 baseline/candidate comparison；
- 把聚合 subject 映射为窄输入的 `ProjectionEvaluator`。

`Report` 不强迫所有 evaluator 产生同一种结论。Verdict、归一化且 higher-is-better 的 `Score`、有限 raw `Measurement`、Feedback、Metadata 与 Details 相互独立；原始 measurement 的单位和优化方向属于结构化 `Metric` identity。Details 在所有公开信任边界受 `MaxReportDepth` 限制。

## 领域词汇的归属

- `eval/judge` 把结构化模型判断适配为通用 Report，并支持多次采样和 median 聚合。
- `eval/text` 拥有回答相关性、正确性和 groundedness 等文本 subject。
- `eval/ranking` 拥有 precision、recall、MRR 和 NDCG 等排序 subject。

这些叶子包只实现根协议，不能把自己的 sample、prompt 或指标词汇提升为根模型。RAG 若需要评估检索结果，应在调用方构造相应 typed subject，而不是让 `eval` 重新依赖 RAG。

## 明确边界

Eval 拥有内存中的 dataset、experiment、aggregation、comparison 和 report；它不拥有：

- dataset 或结果的持久化服务；
- trace、运行制品、项目目录或远程调度；
- 仪表盘、实验 UI、权限、租户或发布工作流；
- 没有统计模型支撑的显著性声明；
- 把不同单位或不同 Metric identity 合并成单一总分的策略。

这些能力需要 Host 生命周期和产品数据模型，应由 Flame 或独立实验 harness 组合。`otel/eval` 只在 evaluator 边界记录低基数身份、结果和时延，不观察 subject 内容。

## 结论

`eval` 是通用 AI 基础库中的评估运行时，而不是 RAG evaluator 集合，也不是完整实验产品。它的扩展方向应继续围绕可复用协议和诚实的结果语义；存储、制品、服务端和 UI 不进入本仓库。
