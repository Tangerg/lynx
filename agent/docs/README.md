# Lynx Agent 文档

`agent` 是 planner-driven 的 Go agent runtime：agent 定义由 Goal、Action、
Condition 和 Blackboard 组成，runtime 在每个 tick 重新观察状态并规划下一步。
它不是把所有能力塞进一个 ReAct client 的框架，也不复制 provider 协议。

当前文档只有三份：

- [`GUIDE.md`](./GUIDE.md)：从 agent 定义、运行到 LLM/tool-loop、HITL 和持久化的使用指南。
- [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)：公开 SPI、扩展分发规则和所有权边界。
- [`../CLAUDE.md`](../CLAUDE.md)：模块维护规则和反向不变量。

跨模块的 Core 架构、最终依赖方向和完整执行证据见
[`../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md)。
provider-neutral Chat/Tool 上手入口见
[`../../doc/CORE_GETTING_STARTED.md`](../../doc/CORE_GETTING_STARTED.md)。

早期的 Spring/Embabel 移植对照、greenfield 草案和阶段性架构体检已经删除。
这些文件引用了已移除的 `core/model/chat`、旧 Chat client 和旧 tool-loop，不能再作为
当前设计依据；需要追溯决策时，以执行计划中的 ADR、变更日志和提交证据为准。

## 当前结构

```text
agent/
├── core/                 Agent、Action、Goal、Condition、Blackboard、SPI
├── planning/             Planner 契约、PlanningSystem 与算法实现
│   └── planner/          goap、htn、reactive、utility
├── runtime/              Platform、AgentProcess、生命周期与持久化
├── event/                生命周期事件和 listener
├── hitl/                 typed await/interrupt helper
├── toolloop/             唯一 Event Runner、Checkpoint、Pause/Resume
├── toolpolicy/           tools.Tool decorator
├── workflow/             编译为普通 Agent 的高阶组合器
└── examples/             可运行示例
```

运行基础示例：

```bash
go run ./examples/hello
go run ./examples/blog
go run ./examples/toolloop
```
