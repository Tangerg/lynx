# Agent 内核反馈闭环记录

**日期**：2026-08-30

**范围**：当前 `agent` module、`agenttest` 合同套件与 `otel/agent` 集成。

## 结论

AG1–AG5 均已关闭；其中 AG4 经最新设计裁决保留 `lo.IsNil`，不是遗漏。

| 条目 | 状态 | 当前实现 |
|---|---|---|
| AG1 `agenttest` 未登记 | 已修复 | `ARCHITECTURE.md` 的精确 package 集合和 package DAG 均登记 `agenttest`，生产代码禁止依赖它 |
| AG2 悬空 ADR | 已修复 | 规则只引用当前 `ARCHITECTURE.md` / `ENGINEERING_STANDARDS.md`，不依赖已删除 ledger |
| AG3 两锁与 owner line | 已修复 | `treeOperationsMu` 与 registry `mu` 明确永不嵌套；`treeRuntime` 声明单写 owner line、原子旁路和 commit 状态所有权 |
| AG4 `samber/lo` | 已裁决保留 | typed nil 统一使用 `lo.IsNil`；不复制 reflect helper，不新建跨模块 utils owner |
| AG5 durable wire 无版本 | 已修复 | `CurrentTreeSnapshotVersion`、`TreeSnapshot.Version()` 与 `ErrUnsupportedTreeSnapshotVersion` 区分升级和损坏；只读当前版本 |

## Durable 能力闭环

- `TreeDurability` 只暴露 activation、effect boundary、checkpoint 与 start outcome commit；存储读取和事务实现归 Host。
- pending 在外部 dispatch 前提交，settled 在内存 apply 前提交，Parked/Terminal 只发布 canonical safe cut。
- `TreeIncarnationID` 与 previous head digest 组成恢复 fencing；旧 writer 无法覆盖新 incarnation。
- crash-prefix matrix 覆盖 commit 前后、dispatch 前后、publication 前后、activation 与 admin apply。
- `RunTreeDurabilityConformance` 和 `MemoryTreeDurability` 让任意 Host adapter 验证幂等、fencing 与安全切点，不把参考内存实现伪装成生产存储。

## 可观测性与跨进程恢复

`otel/agent` 只消费 typed Event facts。恢复测试会从 snapshot 建立新 Engine，验证 restored activation、TreeIncarnationID、父子 span 与 metrics；raw payload、Input、Output 和 Host 产品身份不会进入 telemetry。

## 性能证据

`performance_benchmark_test.go` 固定观察三条可能影响架构判断的边界：snapshot 尺寸/编解码、Execution replay、blocked sibling 下的 owner-line latency。没有 workload 与阈值的 percentile 或占用率不会被固化成内核运维 API。

## 边界裁决

- Agent 不拥有 repository、database schema、lease、业务 exactly-once 或产品 session。
- 外部 Effect 的 replay 由 dispatcher 的显式 `ReplayPolicy` 决定；稳定 EffectID 不等于可安全重试。
- TreeSnapshot wire 变化允许 breaking，但 Host 必须在升级前排空、作废或显式迁移；Framework 不做隐式 dual-read。
- Flame 继续拥有桌面和应用层工作流；Agent 只提供可组合、可恢复的执行合同。
