# Lynx Agent 文档

`agent` 是 planner-driven 的 Go agent runtime：agent 定义由 Goal、Action、
Condition 和 Blackboard 组成，runtime 在每个 tick 重新观察状态并规划下一步。
它不是把所有能力塞进一个 ReAct client 的框架，也不复制 provider 协议。

当前维护文档：

- [`GUIDE.md`](./GUIDE.md)：从 agent 定义、运行到 LLM/tool-loop、HITL 和持久化的使用指南。
- [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)：公开 SPI、扩展分发规则和所有权边界。
- [`../CLAUDE.md`](../CLAUDE.md)：模块维护规则和反向不变量。
- [`../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)：框架目标架构、阶段任务、进度、风险和 ADR 的唯一执行基准。

跨模块的 Core 架构与最终依赖方向见
[`../../core/CLAUDE.md`](../../core/CLAUDE.md)。
provider-neutral Chat/Tool 上手入口见
[`../../doc/CORE_GETTING_STARTED.md`](../../doc/CORE_GETTING_STARTED.md)。

## 当前结构

```text
agent/
├── core/                 Agent、Action、Goal、Condition、Blackboard、SPI
├── planning/             Planner 契约、Domain、State 与 Plan
│   ├── goap/             deterministic uniform-cost GOAP
│   ├── htn/              hierarchical task network
│   ├── reactive/         reactive planner
│   └── utility/          utility-based planner
├── routing/              prompt 到已部署 Agent/Goal 的选择
├── runtime/              Engine、Deployment、Process 与生命周期协调
├── event/                生命周期事件和 listener
├── hitl/                 typed Interrupt helper
├── interaction/          Framework 托管交互的稳定事件与挂起协议
├── toolloop/             叶子 Event Runner、Checkpoint、Pause/Resume
├── toolpolicy/           tools.Tool decorator
├── workflow/             编译为普通 Agent 的高阶组合器
├── storetest/            ProcessStore 外部实现 conformance
└── examples/             可运行示例
```

运行基础示例：

```bash
go run ./examples/hello
go run ./examples/blog
go run ./examples/toolloop
```
