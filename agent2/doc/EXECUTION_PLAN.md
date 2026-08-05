# Agent Framework 绿色重构执行计划

> 状态：持续实施
> 建立日期：2026-08-06
> 最后更新：2026-08-06
> 当前阶段：P0 模块边界与文档分层，3/3 完成
> 当前实施范围：仅 `agent2`
> 临时模块路径：`github.com/Tangerg/lynx/agent2`
> 最终模块路径：`github.com/Tangerg/lynx/agent`

本文只记录实施范围、阶段任务、当前进度、风险、验证结果和执行日志。目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)，长期决策见 [`DECISIONS.md`](DECISIONS.md)。

---

## 1. 事实记录规则

1. 状态只允许 `未开始`、`进行中`、`完成`、`阻塞`。
2. 阶段任务只有在实现、测试、文档和对应架构守卫全部完成后才能标记完成。
3. 每轮实现完成后更新阶段表和执行日志；日志只记录真实发生的事情。
4. 改变包边界、领域术语、Process 状态机、snapshot wire 或跨模块职责前，先更新 [`DECISIONS.md`](DECISIONS.md)。
5. 不在本计划复制稳定架构说明，避免进度更新污染设计基线。
6. 不记录易漂移的逐文件清单；精确符号和依赖事实以代码、GoDoc、测试与 Git 为准。
7. 不保留兼容层、别名、双写、双读或“以后删除”的临时抽象。

---

## 2. 实施隔离与旧模块参考

P1–P9 默认只修改 `agent2` 以及为其独立编译所必需的 workspace/module 元数据：

- 不修改 `app/runtime`、前端、TUI 或 CLI 来迁就未完成设计。
- 不修改旧 `agent` 来为 `agent2` 提供兼容入口。
- `agent2` 不 import 旧 `agent`，旧 `agent` 不 import `agent2`。
- 消费迁移集中在 P10，旧模块删除和目录归一集中在 P11。

旧 `agent` 在并存期保留为可直接阅读和运行的参考实现。每个能力阶段开始时必须检查对应旧代码及测试，形成以下裁决之一：

| 裁决 | 含义 |
|---|---|
| 保留思想 | 不复制 API/目录，只继承已证明的领域规则或算法性质 |
| 重新实现 | 能力仍需要，但按新 Definition/Execution/Process 合同实现并重新测试 |
| 移除 | 能力重复、语义错误、属于 Host，或新架构不再需要 |

旧代码是高价值实现证据，但不是兼容规范。新实现不得仅因旧代码已有某类型或 package 就复制它。

---

## 3. 每批提交与验证纪律

每轮形成一个可独立回滚的提交，只 stage 本轮明确范围内的文件；工作区其他模块的并行改动视为用户改动，不触碰、不暂存。

基础门禁：

```bash
cd agent2
go build ./...
go vet ./...
go test ./...
```

涉及并发、snapshot 或恢复时追加对应 race/fuzz/contract gate。提交前运行 `go mod tidy` 并确认没有非预期 diff；提交成功后默认推送当前分支。

---

## 4. 阶段总览

| 阶段 | 状态 | 进度 | 目标 |
|---|---|---:|---|
| P0 模块边界与文档分层 | 完成 | 3/3 | 建立独立 module，拆分架构、决策和进度事实 |
| P1 公共窄腰与状态机 | 未开始 | 0/7 | 用多策略原型验证 Definition/Execution/Transition/Process |
| P2 Engine 最小执行闭环 | 未开始 | 0/6 | 单 Process 调度、生命周期、snapshot、event、limit |
| P3 Interaction | 未开始 | 0/8 | 原生模型/工具循环、流、checkpoint、HITL、结构化输出 |
| P4 子 Process 组合 | 未开始 | 0/7 | start/wait、递归、并行、预算、取消、恢复 |
| P5 Planning 与 GOAP | 未开始 | 0/8 | Planning 状态、Planner SPI、GOAP 搜索与 replan |
| P6 Workflow | 未开始 | 0/8 | 原生 sequence/gate/router/fork/join/loop/agent call |
| P7 Supervisor 与模式覆盖 | 未开始 | 0/7 | 动态 worker、typed artifacts、evaluator/optimizer、示例 |
| P8 Platform 与治理 | 未开始 | 0/6 | deployment catalog、版本、路由、递归治理和观测 |
| P9 独立完整性验收 | 未开始 | 0/6 | API/wire/arch/race/fuzz/examples/standalone 全绿 |
| P10 消费迁移 | 未开始 | 0/5 | 单独迁移 `app/runtime` 及批准的直接消费者 |
| P11 原模块替换 | 未开始 | 0/5 | 删除旧模块、改回 `agent`、清零兼容和残留 |

---

## 5. 分阶段任务

### P0：模块边界与文档分层

- [x] P0-01 创建独立 `agent2` Go module，并加入 workspace。
- [x] P0-02 建立不含进度噪声的目标架构文档。
- [x] P0-03 将 ADR 和实施事实拆成独立文档，并明确旧模块参考规则。

### P1：公共窄腰与状态机

- [ ] P1-01 审查旧 `agent` 对应实现，记录保留思想、重新实现和移除裁决。
- [ ] P1-02 用最小 Interaction concrete prototype 证明自主交互需求。
- [ ] P1-03 用两个 Action 的最小 Planning concrete prototype 证明规划需求。
- [ ] P1-04 冻结 Definition、Descriptor、Deployment 和 DeploymentRef 语义。
- [ ] P1-05 冻结 Execution、Step、Transition、ExecutionState 和 Process 状态机。
- [ ] P1-06 建立类型、状态机和依赖架构 contract tests。
- [ ] P1-07 删除未被多个真实实现共同证明的推测性抽象。

### P2：Engine 最小执行闭环

- [ ] P2-01 实现 Definition 冻结与精确 Deployment 绑定。
- [ ] P2-02 实现单 Process start/step/run/control 生命周期。
- [ ] P2-03 实现合法 Transition gate 和终态不变量。
- [ ] P2-04 实现 snapshot capture/validate/restore，不引入 Store。
- [ ] P2-05 实现通用 event 与最小 usage/limit。
- [ ] P2-06 完成 standalone build/vet/test/race 和公开 API 审查。

### P3：Interaction

- [ ] P3-01 审查旧 `toolloop`、interaction 和 runtime interaction 实现并记录裁决。
- [ ] P3-02 使用 `chatclient` 和 `tool` 实现原生 Interaction Definition。
- [ ] P3-03 支持普通与流式模型调用，保持事件顺序确定。
- [ ] P3-04 支持模型/工具循环和清晰停止条件。
- [ ] P3-05 支持工具 checkpoint、挂起和精确恢复。
- [ ] P3-06 支持 HITL 和审批等待，不序列化调用栈。
- [ ] P3-07 支持显式安全且有界的并行工具调用。
- [ ] P3-08 通过 autonomous agent 和 direct-vs-managed 示例验收。

### P4：子 Process 组合

- [ ] P4-01 冻结 StartChild 和 WaitForChildren 语义。
- [ ] P4-02 实现 parent/root/depth 和 Process tree 不变量。
- [ ] P4-03 实现 all/any/quorum 等待与确定结果顺序。
- [ ] P4-04 实现同 Definition 递归调用和不同 Strategy 嵌套。
- [ ] P4-05 实现预算划拨、能力衰减、深度/fan-out/并行限制。
- [ ] P4-06 实现取消传播、失败传播、幂等恢复和祖先等待拒绝。
- [ ] P4-07 完成恢复、race、指数扩张防护和递归 contract tests。

### P5：Planning 与 GOAP

- [ ] P5-01 审查旧 Planning/GOAP/HTN/Utility 算法与测试并记录裁决。
- [ ] P5-02 建立 Planning 独占的 Goal/Condition/Truth/WorldState。
- [ ] P5-03 建立基础 Action 与 PlannedAction 的清晰分层。
- [ ] P5-04 用真实消费点定义最小 Planner SPI。
- [ ] P5-05 实现 GOAP 搜索、成本、前置条件和效果。
- [ ] P5-06 实现 observe/plan/act/reobserve 和环境变化 replan。
- [ ] P5-07 将 no-plan/stuck 留在 Planning policy，不污染 Process 状态。
- [ ] P5-08 用多路线、不可达目标、动态环境和子 Agent Action 验收。

### P6：Workflow

- [ ] P6-01 审查旧 workflow builders、并行与 supervisor 代码并记录裁决。
- [ ] P6-02 实现原生 Workflow Execution 和类型化节点合同。
- [ ] P6-03 实现 Sequence、Prompt Chaining 和 Gate。
- [ ] P6-04 实现 Router/Switch。
- [ ] P6-05 实现 Fork/Join、Map/Reduce 和稳定聚合。
- [ ] P6-06 实现 Vote/Consensus。
- [ ] P6-07 实现有明确终止条件的 Loop/Repeat Until。
- [ ] P6-08 实现 Agent call node，并验证任意 Strategy 嵌套。

### P7：Supervisor 与模式覆盖

- [ ] P7-01 审查 Embabel supervisor、utility、hybrid 及旧实现并记录裁决。
- [ ] P7-02 实现 Action-to-Tool adapter，不混淆 Action 与 Tool。
- [ ] P7-03 实现 typed artifact state 和 completion validator。
- [ ] P7-04 实现模型动态拆分、worker 调度和结果综合。
- [ ] P7-05 实现 evaluator-optimizer 组合。
- [ ] P7-06 为 Anthropic 所列每种模式建立行为测试和示例。
- [ ] P7-07 比较简单实现与复杂编排，删除无实际收益的抽象。

### P8：Platform 与治理

- [ ] P8-01 实现不可变 Deployment catalog 和精确 DeploymentRef。
- [ ] P8-02 实现显式 Deploy/Replace/Undeploy 和版本冲突语义。
- [ ] P8-03 实现 Definition 路由与选择，不建立全局注册表。
- [ ] P8-04 实现跨 Process 的统一 budget、policy 和 capability guard。
- [ ] P8-05 完成 Framework event 和 OTel decorator 边界。
- [ ] P8-06 验证内嵌 Engine 与完整 Platform 使用同一执行语义。

### P9：独立完整性验收

- [ ] P9-01 审计所有公开名称、参数、描述和 error 语义。
- [ ] P9-02 建立 exported API 和 snapshot wire baseline。
- [ ] P9-03 建立完整 package DAG 和边界守卫。
- [ ] P9-04 完成全量 build/vet/test/static analysis/race/fuzz。
- [ ] P9-05 完成独立 module、examples 和文档验证。
- [ ] P9-06 清除空目录、漂移注释、dead code、TODO 债务和兼容残留。

### P10：消费迁移

- [ ] P10-01 单独审计 `app/runtime` 的真实消费面和迁移影响。
- [ ] P10-02 更新迁移决策并取得 breaking change 实施确认。
- [ ] P10-03 将聊天路径从单 Action GOAP wrapper 迁移为 Interaction Definition。
- [ ] P10-04 迁移其他批准的直接消费者，不修改无关前端/TUI/CLI。
- [ ] P10-05 删除应用侧框架通用编排并完成应用门禁。

### P11：原模块替换

- [ ] P11-01 确认旧 `agent` 无剩余消费者或独有能力。
- [ ] P11-02 删除旧 `agent`，不保留兼容层。
- [ ] P11-03 将 `agent2` 目录和 module path 改为 `agent`。
- [ ] P11-04 清理 workspace、文档、依赖和旧术语。
- [ ] P11-05 执行全 workspace 最终门禁并记录替换提交。

---

## 6. 最终完成定义

- Interaction、Planning、Workflow 都是原生 Execution，并通过共同 contract tests。
- GOAP 的真实搜索、观察、执行和 replan 行为完整。
- Anthropic 编排模式有行为测试和示例。
- 同 Definition 递归子 Process 可暂停、恢复、取消和预算限制。
- Framework snapshot 与 Host persistence 无交叉所有权。
- `app/runtime` 只依赖中性 Agent 生命周期，不解析策略 payload。
- API、wire、race、fuzz、architecture 和 standalone module gate 全绿。
- 旧 `agent` 和所有兼容路径被删除。
- 临时 `agent2` 路径改回唯一的 `agent`。

---

## 7. 风险与控制

| 风险 | 后果 | 控制 |
|---|---|---|
| 为统一而造 god `ExecutionContext` | 所有策略依赖无关能力 | 接口由消费方定义；P1 用多个 concrete prototype 反证 |
| 把所有模式做成枚举 | 添加模式持续修改中心 switch | Definition/Execution 多态，显式装配 |
| 复制旧目录和类型 | 历史耦合进入新模块 | 从行为合同和测试开始，按 owner 新建 package |
| 忽略旧实现经验 | 重复已解决的并发、恢复和边界错误 | 每阶段先直接审查旧代码和测试并记录裁决 |
| 新旧实现长期并存 | 两套术语和维护成本 | P11 是完成定义，最终只保留 `agent` |
| 过早设计 Platform | Engine 未稳定就形成 god object | Engine/Strategy 先行，Platform 在 P8 实现 |
| 递归指数扩张 | 成本、延迟和错误快速放大 | 深度、fan-out、总量、预算划拨和并行硬限制 |
| 子 Process 权限提升 | 安全边界失效 | capability attenuation，只允许父能力子集 |
| snapshot 假持久化 | 恢复重复副作用或定义漂移 | 精确 DeploymentRef、幂等身份、严格 codec 和恢复测试 |
| Host 解析策略 payload | `app/runtime` 再次抽象泄漏 | opaque envelope + architecture tests |
| 多 Agent 复杂度被滥用 | 成本更高、效果更差 | direct-first，要求 eval 证明复杂编排收益 |

---

## 8. 执行日志

| 日期 | 阶段 | 实际事实 | 验证与结果 |
|---|---|---|---|
| 2026-08-06 | P0 | 创建独立 `agent2` module 并加入 workspace；仅用根 `doc.go` 建立可验证的 package，未引入接口或占位实现；将稳定架构、ADR、实施进度拆为三份文档；明确旧 `agent` 是并存期直接参考实现但不是兼容规范；未修改旧 `agent` 或 `app/runtime` | `go build ./...`、`go vet ./...`、`go test ./...`、独立 module 解析和 diff check 全部通过；P0 3/3 完成 |

---

## 9. 当前下一步

下一阶段是 P1。编码前先完成两个最小 spike：

1. 一个不含工具的最小 Interaction Definition。
2. 一个只有两个 Action 的最小 Planning Definition。

同时直接检查旧 `agent` 中对应的 Process、planning 和 interaction 实现及测试，提取已经被证明的不变量。用两个新 concrete implementation 共同需要的最小语义确定 Definition、Execution、Step、Transition、Process 和 ExecutionState；任何只被其中一个策略需要的字段都留在对应策略 package。

spike 通过后删除试验性多余结构，再冻结第一批公开合同。在 P1–P9 完成前，不迁移 `app/runtime`，不删除旧 `agent`，不发布 `agent2` 稳定版本。
