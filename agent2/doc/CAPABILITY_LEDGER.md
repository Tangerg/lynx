# Agent Framework 能力裁决台账

> 状态：持续维护的实施事实
> 建立日期：2026-08-06
> 最后更新：2026-08-06

本文只追踪旧能力是否被新 Framework 认领、归谁拥有、如何裁决和在哪一阶段验收。它不定义目标架构、不复制 ADR、不记录逐提交进度。

- 目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。
- 长期决策见 [`DECISIONS.md`](DECISIONS.md)。
- 工程标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。
- 阶段与完成事实见 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)。

## 1. 记录规则

1. 每项旧能力必须裁决为保留思想、重新实现或移除，不能因未被某阶段提及而静默消失。
2. `真实消费者` 只记录需求证据，不决定新 API、包名或 owner。
3. `新 owner` 必须是 Framework/Strategy/基础库/Host 中准确的一层；应用领域名称不得进入 Framework 公共合同。
4. `验收` 必须是可执行行为或 architecture gate，不能只写“已迁移”。
5. 阶段开始时补充代码证据，阶段完成后才把裁决状态改为已验证。

## 2. Kernel 与执行生命周期

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| Agent definition、descriptor、deployment digest | 旧 Engine、长期恢复 | Definition/Deployment | 重新实现 | P1–P2 | 不可变、schema 进入 digest、精确恢复 |
| Process 状态机 | 所有托管执行 | Engine/Process | 保留思想并重新实现 | P1–P2 | 全状态矩阵、first-terminal-wins |
| ProcessView | policy、listener、Host adapter | 按消费者拆分的只读小接口 | 重新裁决 | P1 | 不保留共同胖视图；每个接口有真实消费者 |
| ProcessContext | Action、interaction、tool、控制 | Signal/Transition/Effect 与消费侧小接口 | 移除旧形态 | P1–P5 | 不出现 god context；Engine 保持唯一 owner |
| Process snapshot/tree snapshot | pause、restart、child tree | Framework capture/restore | 重新实现 | P2–P4 | last-stable/prepared 边界、pre-dispatch durability gate、严格 codec、tree restore |
| Respond/Resume/Continue | HITL、parked tool、child completion | Signal/WaitID 控制面 | 重新实现 | P1–P4 | 排序、去重、过期、Running/Waiting 投递 |
| Engine deployment catalog | deploy、restore | Engine resolver consumer + Platform catalog | 拆分重写 | P2、P8 | 单一真相源、精确 DeploymentRef |
| child Process 与递归 | delegation、workflow、supervisor composition | Engine Process tree | 保留思想并重新实现 | P4 | 恢复、取消、预算、深度、fan-out |
| usage、budget、limit | 所有托管执行 | Engine 中性资源事实 | 重新实现 | P2、P4 | 单调累计、父子划拨、终止原因准确 |
| StuckPolicy/ProcessPolicy | GOAP 无计划、运行限制 | Planning policy / Engine 中性 policy | 拆分重写 | P2、P5 | Stuck 不进入共同状态 |

## 3. Interaction、Tool 与人机协作

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| `interaction` + `toolloop` 两层 | 聊天、编码、工具自主调用 | Interaction | 合并重写 | P3 | 单一公共概念，多轮/工具/停止完整 |
| Tool checkpoint/PendingCall | Tool pause、精确 resume | Interaction ExecutionState | 保留思想并重写 | P3 | 每个合法挂起点 capture→restore→continue |
| Suspension/HITL | approval、question、child wait | WaitID + Signal；typed helper 按需 | 重新实现 | P3–P4 | schema、批量回答、重复/过期拒绝 |
| StreamCall/OnDelta | desktop/CLI 实时输出 | 有界非权威 Delta | 重新实现 | P3 | 顺序、背压、丢弃可观测、final 独立 |
| steering middleware | 运行中追加输入 | Interaction Signal | 移除 middleware 绕行并重写 | P3 | 下一安全边界生效，不中断不安全 Effect |
| ChatMiddleware | history、prompt/context 注入 | `chatclient` 或 Interaction 的具体 pre-model seam | 按能力拆分 | P3 | Framework 不拥有产品历史；顺序可测试 |
| ToolMiddleware | 权限、审批、观测、业务策略 | `tool`/Host adapter/decorator | 保留基础库能力，不进入 Kernel | P3 | Tool 合同不依赖 Agent/Host 类型 |
| conversation/history compaction | 跨执行产品上下文维护 | Host/独立 history 能力 | 从 Framework 移除 | P3 consumer spike | Agent 公共 API 无产品 history/compaction |
| WorkingContext | Tool loop、resume、模型请求 | Interaction ExecutionState | 重新实现 | P3 | 与产品历史术语分离；恢复自足有界 |
| InteractionCostProjector | 产品价格与成本投影 | Host observer/accounting | 从 Framework Kernel 移除 | P3、P10 | Framework 只发中性 usage，不认识价格表 |

## 4. Planning、Workflow 与组合

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| Goal/Condition/Truth/WorldState/Blackboard | GOAP/HTN/Utility | Planning | 保留思想并重写 | P5 | 不进入共同 Process/snapshot |
| GOAP planner | 多路线、动态重规划 | `planning/goap` | 保留算法思想并重写 | P5 | 搜索、成本、reobserve/replan |
| HTN planner | 层次任务规划 | `planning/htn` | 待真实需求复核 | P5 | 无真实消费者则不预建 package |
| Utility planner | utility selection | `planning/utility` | 待真实需求复核 | P5 | 与 Planning 生命周期共用 Planner SPI |
| Reactive planner | 旧 planning/reactive | Planning 内 Planner 或移除 | 待审查 | P5 | 明确区别 ReAct；不得静默遗漏 |
| routing Ranker/Candidate/Choice | Definition/worker selection | Workflow Router 或 Platform routing | 拆分裁决 | P6、P8 | 不建立重复 router abstraction |
| Workflow sequence/gate/router/loop | 代码确定路径 | Workflow | 重新实现 | P6 | 原生 Execution，不编译 GOAP |
| Workflow fork/map/join | 并行 section/vote/reduce | child Process + Workflow join state | 重新实现 | P4、P6 | 分支独立 snapshot/预算、fan-out 有界、确定聚合 |
| Supervisor | 动态 worker、结果综合 | Interaction/Workflow composition | 移除独立 Strategy 预设 | P7 | 无独立 kind/package；组合行为测试 |
| evaluator-optimizer | Anthropic pattern | Workflow + Definitions | 重新实现为组合 | P7 | 明确终止条件和质量门槛 |
| Action-to-Tool adapter | 模型选择 framework Action | Interaction/Workflow 组合 | 重新实现 | P7 | 名称/schema/结果准确，不混淆 Action/Tool |

## 5. 扩展、依赖与观察

| 旧能力 | 真实消费者/证据 | 新 owner | 裁决 | 阶段 | 验收 |
|---|---|---|---|---|---|
| `core.Dependencies` typed scope chain | 动态 Action、应用装配 | 构造/闭包优先；真实动态需求留 Strategy | 待审查 | P1、对应 Strategy | 无全局 DI、无 service locator、无共同 god scope |
| Extension marker/capability dispatch | policy、middleware、listener、planner | 单一横切扩展机制；主 Strategy 移出 | 拆分重写 | P2、P8 | 忽略扩展不破坏 Kernel 正确性 |
| Event multicast | runtime observer、Host projection | Framework Event | 重新实现 | P2 | attempt/committed 区分、顺序、listener 隔离 |
| model/tool lifecycle event | UI、usage、observability | Strategy dispatcher → Framework Event | 重新实现 | P2–P3 | 时间语义、Process/Step/Effect identity 完整 |
| OTel instrumentation | tracing/metrics/logs | 外部 `otel` decorator | 保留边界 | P8 | Kernel 不 import OTel，不重造 tracer SPI |
| Action/Agent validator | deploy/admission | Definition/Deployment 构造；横切 guardrail 按需 | 拆分重写 | P1–P2 | 非法定义不能进入 Engine |
| typed dependency/environment access | Action/Condition 执行 | Strategy-owned dispatcher/constructor | 重新实现或移除 | P3–P5 | Kernel 不认识产品依赖 |

## 6. Host 专属能力

以下能力是迁移验收证据，不是 Agent Framework 待抽象的领域模型：

| 能力 | 新 owner | Framework 只提供 | 验收阶段 |
|---|---|---|---|
| 产品身份、历史、记录和展示投影 | Host | Process/Event/Output 中性事实 | P3 spike、P10 |
| Store、transaction、CAS、lease、retention | Host | Snapshot capture/restore value | P2、P10 |
| 销毁、回滚、替换、导入导致的执行清理 | Host | Kill/Capture 和稳定 Process identity；持久记录删除仍归 Host | P3 spike、P10 |
| 业务权限、订阅、计费和价格表 | Host/adapters | capability/usage 中性合同 | P4、P8、P10 |
| 外部副作用事务、补偿和业务幂等 | Action/Tool adapter 或 Host | EffectID 与 settlement 事实 | 对应 Strategy、P10 |

这些名称不得成为 Kernel 类型、字段、package 或 snapshot 字段。Host 的真实消费审计可以补充本表证据，但不能以迁移方便反向决定 Framework API。
