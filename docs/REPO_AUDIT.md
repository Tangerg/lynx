# 全仓反馈闭环记录

**日期**：2026-08-30

**范围**：当前工作树中的全部 workspace module

**定位**：Scope 是模块化 AI 基础库；应用会话、UI、业务持久化、产品权限与交付流程属于 Flame 或其他 Host。

本文不再把旧提交上的扫描结果当作当前事实。每个历史反馈只保留最终裁决：代码修复、机制化门禁，或基于框架边界的明确拒绝。

## 结论

旧报告中的可执行缺陷均已关闭。当前没有需要靠临时兼容层、第二套 API 或应用层抽象解决的遗留项。

| 条目 | 最终状态 | 当前证据 |
|---|---|---|
| N1 Agent 覆盖率 | 已修复 | `scripts/check-agent-coverage.sh` 与 CI 的 Agent 专属预算 |
| N2 Eval 模块合同 | 已修复 | `eval/CLAUDE.md` |
| N3 并发零值死锁 | 已修复 | `DefaultMaxConcurrency` 在构造边界一次解析；负值拒绝 |
| N4 Report 无界递归 | 已修复 | `eval.MaxReportDepth` 在 validate/clone/JSON 边界统一执行 |
| N5 错误链 | 已修复 | errorlint 与 `%w`/`errors.Is` 规则成为仓库门禁 |
| B3/B7 并发所有权 | 已修复 | `Engine` 两锁明确不嵌套；`treeRuntime` owner line 和字段分区写明 WHY |
| M 根入口与许可 | 已修复 | 根 `README.md` 与 Apache-2.0 `LICENSE` |
| O1 OTel 能力缺口 | 已修复 | 新增 embedding、image、moderation、speech、transcription 与 eval adapter；含内容不泄露测试 |
| V2 普通路径示例 | 已修复 | Agent、RAG、ETL、Tools、Skills、Eval、OTel、A2A、MCP 均有 checked Go Example |
| V3 文档逃逸 | 已修复 | `dev/repoarch` 动态发现上述能力模块的公开包，强制 Package comment 与模块级 checked Example |
| E4 Judge 自洽性 | 已修复 | `judge.Config.Samples` 多次采样，median 聚合进入结构化 Metric identity |
| E5/E6 组合评估 | 已修复 | 显式 Weight、Required 与 PassAll/PassAny/PassAtLeast；配置进入 Metric identity |
| Eval 观测 | 已修复 | `otel/eval.Middleware[T]` 包装通用 `Evaluator[T]`，不依赖 RAG |
| 多模块兼容 | 已修复 | 所有内部依赖提升到同一已发布基线；CI 以 `GOWORK=off` 独立编译每个 module |

## 经裁决不应修改的条目

### A1 · 协议指针不是贫血模型

Core 的 request/response/options 是 wire DTO。指针字段只用于表达“缺失”和“显式零值”的差异，或表达 tagged payload 的可选分支；领域实体仍优先采用受控构造和值对象。

机械消除这些指针会丢失 provider 协议信息，反而是抽象不足。`core/internal/arch` 现会扫描公开 JSON DTO：可选指针必须带 `omitempty` 或 `omitzero`；必填嵌套对象指针必须由 owner 的 `Validate` 显式拒绝 nil，防止普通数据借指针逃避 presence 设计。

### A2 / AG4 · `lo.IsNil` 是统一 typed-nil 语义

跨 interface 构造边界需要识别 typed nil。仓库统一使用 `lo.IsNil`，不再在几十个 package 复制 reflection helper，也不建立一个所有模块反向依赖的公共 utils module。这个依赖是经过明确取舍的实现策略，不是待清理债务。

### V1 · 性能反馈必须有可比较场景

Agent 已有三组架构边界 benchmark：

- `BenchmarkTreeSnapshotBoundary`：1/15/63/255 Process 的 build、parse、allocation 与 snapshot bytes。
- `BenchmarkExecutionReplayBoundary`：1 KiB / 64 KiB state 的 restore → step → snapshot → restore。
- `BenchmarkTreeRuntimeFastSiblingLatency`：被阻塞 sibling 存在时，owner line 对快速 sibling 的完成延迟。

“必须输出 p50/p95/p99、owner-line 占用率、连续 commit 数”没有 workload、阈值或回归基线，不能成为稳定 API 或 CI 判断。Go benchmark 和 profile 是当前权威入口；只有真实 Host trace 证明瓶颈后才增加对应指标，避免为了漂亮数字污染内核。

### V6 · `t.Parallel` 比例不是质量指标

测试是否并行取决于是否共享 global provider、端口、文件系统、时钟、进程状态或顺序合同。仓库用 race、`testing/synctest`、并发合同测试与显式 ownership 证明安全；不会为了提高文件比例机械添加 `t.Parallel`。

### E8 · 不回退到 RAG 专用 Eval

旧反馈把 context relevance 与 eval runtime 混为一体。当前 Eval 是通用协议内核，RAG-specific evaluator 不再进入其根包；文本 evaluator 留在 `eval/text`，RAG 组合由调用方完成。新增的是通用 `otel/eval`，不是另一套 RAG evaluator。

### “第二个真实消费者”

Scope 不会为了证明通用性在仓库内制造假应用。Flame 是现有真实 Host；`agenttest` conformance、checked examples、provider conformance 与 module isolation 负责证明可移植合同。第二个真实产品消费者只能来自生态采用，不是基础库应伪造的代码资产。

## 框架边界保持不变

- Durable runtime 只定义 snapshot、checkpoint、effect settlement、fencing 与 Host commit 端口，不拥有数据库、lease、outbox 或业务幂等表。
- 安全层只定义中性 capability、authorization 与 middleware，不拥有用户、租户、审批产品或沙箱服务。
- Eval 只拥有 dataset、experiment、comparison、aggregation 和 report，不拥有实验 UI、制品平台或项目目录。
- OTel adapter 只记录低基数身份、数量、时延和错误分类，不采集 prompt、document、media、transcript 或 evaluation subject。
- 普通调用由 Core client / middleware 直接完成；需要恢复和托管生命周期时才进入 Agent。

## 统一复验

```bash
scripts/check.sh build vet test race tidy
scripts/check.sh isolate
scripts/check-core-coverage.sh
scripts/check-agent-coverage.sh
golangci-lint run --config=.golangci.yml ./...
```
