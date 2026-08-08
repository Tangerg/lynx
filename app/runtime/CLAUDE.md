# CLAUDE.md — Lyra Runtime module

> Lyra Runtime 是供桌面、Web、CLI、TUI 和同进程消费者使用的 Go 应用后端，实现 Lyra Runtime Protocol。项目级法则继承自 [`../../CLAUDE.md`](../../CLAUDE.md)。

在设计、实现、评审或重构本模块前，必须按顺序完整阅读：

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — 已接受的目标架构和所有权边界；
2. [`doc/DECISIONS.md`](doc/DECISIONS.md) — 架构决策与理由；
3. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — 强制实施和质量标准；
4. [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md) — 授权范围、阶段、进度和下一步；
5. [`doc/CAPABILITY_LEDGER.md`](doc/CAPABILITY_LEDGER.md) — 当前能力事实、迁移 verdict 和验收证据；
6. [`doc/CONTRACT_BASELINE.md`](doc/CONTRACT_BASELINE.md) — Protocol、storage、Agent2 consumer 和边界基线。

六份文档各有唯一 owner。不要把进度写进架构、把目标复制到能力台账、把当前内部 API 冒充已冻结基线，或在没有 superseding ADR 时静默改写既有裁决。

协议改动还必须阅读：

- [`doc/API.md`](doc/API.md)；
- [`doc/TRANSPORT.md`](doc/TRANSPORT.md)；
- [`doc/AUX_API.md`](doc/AUX_API.md)；
- [`contract/`](contract/) 机器真相源。

## 当前状态

目标设计已经建立，生产迁移尚未开始。当前代码仍消费旧 `agent`，迁移前实现细节可参考 [`doc/EXECUTION_CENTERED_ARCHITECTURE.md`](doc/EXECUTION_CENTERED_ARCHITECTURE.md)，但它不是新实现兼容规范。

当前轮次若只授权文档，不得提前修改生产代码。实施必须由后续 goal 按 Execution Plan 逐批启动。

## 工作纪律

- 按 Execution Plan 当前授权阶段工作，不顺手扩大到前端、TUI、CLI 或无关模块；
- 每批完成完整语义纵切，不提交 stub、TODO、compat path 或已知债；已经接管的旧 owner 同批删除，P4–P7 harness 仍被生产使用的旧执行 owner 按 ADR-RT-035 统一在 P8 原子切换时删除；
- 旧 `agent` 和迁移前 Runtime 只作为行为证据，不作为新 API 兼容合同；
- 不修改 Agent2 private state 或建立第二 execution lifecycle；发现真实 Framework 缺口先走 Agent2 ADR/baseline；
- 改名和 breaking shape 同步代码、GoDoc、schema、fixtures、生成物和 owner 文档，不保留兼容层；
- 完成后运行该批规定的 build、vet、staticcheck、test、race、fuzz 和 contract/architecture gates；
- 更新 Execution Plan、Capability Ledger 和必要 baseline 后，形成可独立 revert 的提交并及时推送。
