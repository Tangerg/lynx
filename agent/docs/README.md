# Lynx Agent 文档

`agent` 是 planner-driven 的 Go agent runtime：agent 定义由 Goal、Action、
Condition 和 Blackboard 组成，runtime 在每个 tick 重新观察状态并规划下一步。
它不是把所有能力塞进一个 ReAct client 的框架，也不复制 provider 协议。

当前维护文档：

- [`GUIDE.md`](./GUIDE.md)：从 agent 定义、运行到 LLM/tool-loop、HITL 和 portable snapshot 的使用指南。
- [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)：公开 SPI、扩展分发规则和所有权边界。
- [`MIGRATION.md`](./MIGRATION.md)：开发期 breaking API 与行为迁移。
- [`RELEASE_NOTES.md`](./RELEASE_NOTES.md)：尚未发布的框架变化。
- [`../CLAUDE.md`](../CLAUDE.md)：模块维护规则和反向不变量。
- [`../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)：框架目标架构、阶段任务、进度、风险和 ADR 的唯一执行基准。

跨模块的 Core 架构与最终依赖方向见
[`../../core/CLAUDE.md`](../../core/CLAUDE.md)。
provider-neutral Chat/Tool 上手入口见
[`../../doc/CORE_GETTING_STARTED.md`](../../doc/CORE_GETTING_STARTED.md)。

包的划分与职责见 [`../CLAUDE.md`](../CLAUDE.md) 的「架构心智」与各包 godoc —— 这里不复制目录树，它只会在重构后变成误导。

运行基础示例：

```bash
go run ./examples/hello
go run ./examples/blog
go run ./examples/toolloop
```
