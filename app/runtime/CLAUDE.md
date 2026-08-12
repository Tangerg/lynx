# CLAUDE.md — Lyra Runtime module

> Lyra Runtime 是供桌面、Web、CLI、TUI 和同进程消费者使用的 Go 应用后端，实现 Lyra Runtime Protocol。项目级法则继承自 [`../../CLAUDE.md`](../../CLAUDE.md)。

在设计、实现、评审或重构本模块前，必须按顺序完整阅读：

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — 已接受的目标架构和所有权边界；
2. [`doc/DECISIONS.md`](doc/DECISIONS.md) — 架构决策与理由；
3. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — 强制实施和质量标准；
4. [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md) — 授权范围、阶段、进度和下一步；
5. [`doc/CAPABILITY_LEDGER.md`](doc/CAPABILITY_LEDGER.md) — 当前能力事实、迁移 verdict 和验收证据；
6. [`doc/CONTRACT_BASELINE.md`](doc/CONTRACT_BASELINE.md) — Protocol、storage、Agent Framework consumer 和边界基线。

六份文档各有唯一 owner。不要把进度写进架构、把目标复制到能力台账、把当前内部 API 冒充已冻结基线，或在没有 superseding ADR 时静默改写既有裁决。

涉及 Domain 行为所有权、Run/Transcript 边界、Plan 或 Session 充血模型的专项设计与实施，还必须阅读 [`doc/DOMAIN_MODEL.md`](doc/DOMAIN_MODEL.md)。该文档不替代六份 owner；实施授权和进度仍只进入 Execution Plan。

协议改动还必须阅读：

- [`doc/API.md`](doc/API.md)；
- [`doc/TRANSPORT.md`](doc/TRANSPORT.md)；
- [`doc/AUX_API.md`](doc/AUX_API.md)；
- [`contract/`](contract/) 机器真相源。

## 当前状态

P1–P42 服务端重构、公共 binding、真实 consumer 联调与持续反证审计已经完成。Protocol `2026-08-12`、Artifact v18、SQLite epoch 69、唯一 `runtimeInstanceRootSegment` replay scope、全部生成物/严格 sample gate 与 Agent Framework Baseline 20 是当前基线；Runtime 只通过 canonical Agent module 和 `adapter/agentexec` 防腐层消费 Framework，不读取 private state，也不向 Agent 泄露产品抽象。消费者同步状态以 [`doc/CONSUMER_HANDOFF.md`](doc/CONSUMER_HANDOFF.md) 为准，Runtime 不为任何消费者恢复旧合同。

## 工作纪律

- 按 Execution Plan 当前授权阶段工作，不顺手扩大到前端、TUI、CLI 或无关模块；
- 每批完成完整语义纵切，不提交 stub、TODO、compat path 或已知债；已经接管的旧 owner 同批删除；
- 原框架实现和迁移前 Runtime 只作为行为证据，不作为新 API 兼容合同；
- 不修改 Agent Framework private state 或建立第二 execution lifecycle；发现真实 Framework 缺口先走 Agent Framework ADR/baseline；
- 改名和 breaking shape 同步代码、GoDoc、schema、fixtures、生成物和 owner 文档，不保留兼容层；
- 完成后运行该批规定的 build、vet、staticcheck、test、race、fuzz 和 contract/architecture gates；
- 更新 Execution Plan、Capability Ledger 和必要 baseline 后，形成可独立 revert 的提交并及时推送。
