# Agent Framework 绿色重构执行计划

> 状态：持续实施
> 建立日期：2026-08-06
> 最后更新：2026-08-06
> 当前阶段：P7 组合模式与能力覆盖，5/7 完成
> 当前实施范围：仅 `agent2`
> 临时模块路径：`github.com/Tangerg/lynx/agent2`
> 最终模块路径：`github.com/Tangerg/lynx/agent`

本文只记录实施范围、阶段任务、当前进度、风险、验证结果和执行日志。目标架构见 [`ARCHITECTURE.md`](ARCHITECTURE.md)，长期决策见 [`DECISIONS.md`](DECISIONS.md)，能力裁决与消费者证据见 [`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md)，强制工程标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。

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
| P0 模块边界与设计合同 | 完成 | 6/6 | 建立独立 module、分层文档、能力台账和候选合同 |
| P1 候选窄腰与消费审计 | 完成 | 9/9 | 用只读审计和多策略 spike 验证 erased wire、协议与状态机，不冻结 API |
| P2 Engine 最小执行闭环 | 完成 | 8/8 | 单 Process、Signal、Effect、状态提交、snapshot、event/delta、limit |
| P3 真实 Interaction 验证 | 完成 | 9/9 | 真实模型/工具 dispatcher、流、HITL、steer，并接入 disposable consumer |
| P4 子 Process 组合与合同冻结 | 完成 | 9/9 | start/wait、递归、组合、预算、取消、恢复；多消费方验证后冻结窄腰 |
| P5 Planning 与 GOAP | 完成 | 8/8 | Planning 状态、Planner SPI、GOAP 搜索与 replan |
| P6 managed Workflow | 完成 | 11/11 | 以有序 Stage 和真实 child Process 实现可恢复确定性编排，`flow` 保持独立 in-process 边界 |
| P7 组合模式与能力覆盖 | 进行中 | 5/7 | 动态 worker 组合、typed artifacts、evaluator/optimizer、示例 |
| P8 Platform 与治理 | 未开始 | 0/7 | resolver、deployment catalog、版本、路由、递归治理和观测 |
| P9 独立完整性验收 | 未开始 | 0/6 | API/wire/arch/race/fuzz/examples/standalone 全绿 |
| P10 消费迁移 | 未开始 | 0/5 | 单独迁移 `app/runtime` 及批准的直接消费者 |
| P11 原模块替换 | 未开始 | 0/5 | 删除旧模块、改回 `agent`、清零兼容和残留 |

---

## 5. 分阶段任务

### P0：模块边界与文档分层

- [x] P0-01 创建独立 `agent2` Go module，并加入 workspace。
- [x] P0-02 建立不含进度噪声的目标架构文档。
- [x] P0-03 将 ADR 和实施事实拆成独立文档，并明确旧模块参考规则。
- [x] P0-04 建立宏观架构、微观代码/API、测试和批次完成定义的强制工程标准。
- [x] P0-05 建立不复制正文的模块级指引，确保后续任务必须读取五份职责文档。
- [x] P0-06 吸收复审裁决，建立能力台账和 Signal/Transition/Effect/Event/Delta、erased wire、终态、外部事实边界的候选合同。

### P1：候选窄腰与消费审计

- [x] P1-01 依据能力台账审查旧 `agent`，记录保留思想、重新实现和移除裁决。
- [x] P1-02 只读审计 `app/runtime` 与其他真实消费者；只提取中性需求证据，不复制应用术语、存储协议或交付模型。
- [x] P1-03 用真实形态的 Interaction spike 验证模型、Tool、HITL、stream 和 steer 所需合同，spike 完成后可整体丢弃。
- [x] P1-04 用两个 Action 的 Planning spike 验证 Goal/Condition/Planner 专属状态与共同窄腰的边界。
- [x] P1-05 验证非泛型 `json.RawMessage` 窄腰、Descriptor schema/version/digest 和边缘 `Typed[I, O]` adapter。
- [x] P1-06 验证 Signal/WaitID、Transition、Effect/settlement、Event/Delta 的候选协议和 payload 所有权。
- [x] P1-07 验证 Definition、Deployment、ExecutionState、Process 状态机、last-stable/prepared capture、pre-dispatch durability gate 和终态映射矩阵。
- [x] P1-08 建立类型、codec、状态机、终态和依赖架构 contract tests。
- [x] P1-09 删除未被多个真实实现或真实消费者共同证明的推测性抽象；P1 只形成候选合同，不冻结公开 API/wire。

### P2：Engine 最小执行闭环

- [x] P2-01 实现 Definition 候选合同、Descriptor 校验和精确 Deployment 绑定。
- [x] P2-02 实现单 Process start/step/run/control 生命周期，并将 Process 构造权封装在 Engine。
- [x] P2-03 实现 Signal mailbox、Engine 铸造 WaitID、投递去重和安全边界消费。
- [x] P2-04 实现 Strategy-owned Effect dispatcher、稳定 EffectID、settlement Signal 和禁止隐式重投的默认策略。
- [x] P2-05 实现合法 Transition gate、last-stable ExecutionState、prepared/finalize 原子边界、失败实例丢弃和终态映射。
- [x] P2-06 实现 last-stable/prepared snapshot capture/validate/restore，并证明 durable recovery 启用时 Host 可在 dispatch 前同步确认 prepared boundary；不引入 Store、transaction、Host 水位或应用 revision。
- [x] P2-07 实现 attempt/committed Event、有界 best-effort Delta 和最小 usage/limit。
- [x] P2-08 完成 standalone build/vet/test/race、codec/terminal/effect contract 和公开 API 审查。

### P3：真实 Interaction 验证

- [x] P3-01 审查旧 `toolloop`、interaction 和 runtime interaction 实现并记录裁决。
- [x] P3-02 使用 `chatclient` 和 `tool` 实现原生 Interaction Definition 与 Effect dispatcher。
- [x] P3-03 支持普通与流式模型调用；listener 失败隔离、Delta 有界丢弃可观测、恢复不补播。
- [x] P3-04 支持模型/工具循环、清晰停止条件和可独立于 Delta 导出的最终 Output。
- [x] P3-05 支持工具 checkpoint、挂起和精确恢复，验证 settlement 去重与不可重试副作用。
- [x] P3-06 支持 HITL；WaitID 由 Engine 铸造，业务 payload 只由 Interaction 解释。
- [x] P3-07 支持 steer，并为 Interaction 明确和测试 Signal 安全消费边界与最坏生效延迟。
- [x] P3-08 支持显式安全且有界的并行工具调用。
- [x] P3-09 用一个可随时删除的真实 consumer 接入完整路径，并通过 autonomous agent 和 direct-vs-managed 示例验收。

### P4：子 Process 组合

- [x] P4-01 实现 StartChild 和 WaitForChildren 的候选 Effect/Signal 语义。
- [x] P4-02 实现 parent/root/depth 和 Process tree 不变量。
- [x] P4-03 实现 all/any/quorum 等待与确定结果顺序。
- [x] P4-04 实现同 Definition 递归调用和不同 Strategy 嵌套。
- [x] P4-05 实现预算划拨、能力衰减、深度/fan-out/并行限制。
- [x] P4-06 实现取消传播、失败传播、幂等恢复和祖先等待拒绝。
- [x] P4-07 验证父子 Process 的 schema 校验、Effect settlement、snapshot 和终态传播。
- [x] P4-08 用第二个 disposable consumer 验证嵌入式 Engine 与组合式 Agent 应用共用同一窄腰，且没有应用抽象进入 Framework。
- [x] P4-09 完成恢复、race、指数扩张防护和递归 contract tests；只有 P3/P4 多实现、多消费者证据通过后才冻结首个公共 API/wire baseline。

### P5：Planning 与 GOAP

- [x] P5-01 审查旧 Planning/GOAP/HTN/Utility 算法与测试并记录裁决。
- [x] P5-02 建立 Planning 独占的 Goal/Condition/Truth/WorldState。
- [x] P5-03 建立基础 Action 与 PlannedAction 的清晰分层。
- [x] P5-04 用真实消费点定义最小 Planner SPI。
- [x] P5-05 实现 GOAP 搜索、成本、前置条件和效果。
- [x] P5-06 实现 observe/plan/act/reobserve 和环境变化 replan。
- [x] P5-07 将 no-plan/stuck 留在 Planning policy，不污染 Process 状态。
- [x] P5-08 用多路线、不可达目标、动态环境和子 Agent Action 验收。

### P6：managed Workflow

- [x] P6-01 审查旧 workflow builders、并行与 supervisor 代码并记录裁决；只保留组合能力，不复制独立 Supervisor Strategy。
- [x] P6-02 只读审查独立 `flow` 仓库，验证直接使用、整个 Node 的 dispatcher adapter、Graph/Spec managed 编排三种边界，并撤回未经实现证明的原生 Workflow 设计。
- [x] P6-03 用户恢复设计后，以只依赖 Baseline 2 公开 API 的 disposable consumer 验证串行 Call、窗口化 Fork、tree capture/restore、逆序完成稳定聚合和无重复 child；随后整体删除 spike。
- [x] P6-04 追加 ADR-A2-038，裁决唯一边界为 `flow` 负责 in-process 组合、原生 Workflow 负责 managed child Process；词汇收敛为 Transform/Call/Switch/Fork/Map/Loop，不恢复任意 DAG 或第二 runtime。
- [x] P6-05 实现 sealed Stage value、Definition 构造校验、精确 schema 衔接和只拒绝协议违约的 Dispatcher。
- [x] P6-06 实现 Transform/Call 的有序纵切面、child resource allocation、失败归因和每个合法边界的恢复。
- [x] P6-07 实现 Switch/Fork、显式固定 `WindowSize`、声明顺序聚合和最低索引稳定失败。
- [x] P6-08 实现 Map、显式 item limit、窗口调度和稳定 item 结果。
- [x] P6-09 实现至少一次的 Loop、正数 iteration limit 和 `Satisfied`/exhausted 精确结果。
- [x] P6-10 完成 strict state codec、malformed protocol、snapshot/tree restore、cancel、budget/capability、race、fuzz 和 architecture gates。
- [x] P6-11 用独立 command consumer 和最小可运行示例验证公开人体工程学，删除无收益抽象并冻结 Workflow API baseline。

### P7：组合模式与能力覆盖

- [x] P7-01 审查 Embabel supervisor、utility、hybrid 及旧实现并记录裁决。
- [x] P7-02 实现 exact child Deployment 的 managed Delegate，不虚构通用 Action-to-Tool，Tool/Dispatcher 不直接启动 Process。
- [x] P7-03 实现 typed artifact state 和 completion validator。
- [x] P7-04 使用 P6 最终裁决的唯一确定性编排边界、Interaction、Planning 和 child Process 组合模型动态拆分、worker 调度和结果综合，不新增 Supervisor package/kind/lifecycle。
- [x] P7-05 实现 evaluator-optimizer 组合。
- [ ] P7-06 为 Anthropic 所列每种模式建立行为测试和示例。
- [ ] P7-07 比较简单实现与复杂编排，删除无实际收益的抽象。

### P8：Platform 与治理

- [ ] P8-01 用真实 Engine 消费点冻结最小 DeploymentResolver 合同。
- [ ] P8-02 实现不可变 Deployment catalog 和精确 DeploymentRef。
- [ ] P8-03 实现显式 Deploy/Replace/Undeploy 和版本冲突语义。
- [ ] P8-04 实现 Definition 路由与选择，不建立全局注册表或让 Engine 依赖 Platform concrete type。
- [ ] P8-05 实现跨 Process 的统一 budget、policy 和 capability guard。
- [ ] P8-06 完成 Framework Event 和 OTel decorator 边界。
- [ ] P8-07 验证内嵌 Engine 与完整 Platform 使用同一执行语义。

### P9：独立完整性验收

- [ ] P9-01 审计所有公开名称、参数、描述和 error 语义。
- [ ] P9-02 建立 exported API 和 snapshot wire baseline。
- [ ] P9-03 建立完整 package DAG 和边界守卫。
- [ ] P9-04 完成全量 build/vet/test/static analysis/race/fuzz。
- [ ] P9-05 完成独立 module、examples 和文档验证。
- [ ] P9-06 清除空目录、漂移注释、dead code、TODO 债务和兼容残留。

### P10：消费迁移

- [ ] P10-01 复核 P1 只读消费审计结果与 P4 后冻结的合同，确认迁移范围没有新增应用抽象需求。
- [ ] P10-02 更新迁移决策并确认 breaking change 实施批次。
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

- Interaction、Planning 与 Workflow 都是原生 Execution，并通过共同 contract tests；`flow` 与 Workflow 按 in-process/managed-Process 生命周期各自保持一个准确边界。
- Signal、Transition、Effect、Event、Delta 的方向、所有权和可靠性等级唯一且通过合同测试。
- Engine 以非泛型 erased contract 同构持有异构 Definition，typed adapter 只存在于边缘。
- GOAP 的真实搜索、观察、执行和 replan 行为完整。
- Anthropic 编排模式有行为测试和示例。
- 同 Definition 递归子 Process 可暂停、恢复、取消和预算限制。
- Framework snapshot 与 Host persistence 无交叉所有权。
- 等待中的 Process 遇到 Host 外部事实失效时，由 Host 通过中性 lifecycle 能力清理；共同 snapshot 不含产品水位、删除集合或存储协议。
- `app/runtime` 只依赖中性 Agent 生命周期，不解析策略 payload。
- API、wire、race、fuzz、architecture 和 standalone module gate 全绿。
- 旧 `agent` 和所有兼容路径被删除。
- 临时 `agent2` 路径改回唯一的 `agent`。

---

## 7. 风险与控制

| 风险 | 后果 | 控制 |
|---|---|---|
| 为统一而造 god `ExecutionContext` | 所有策略依赖无关能力 | 接口由消费方定义；P1 用多个 concrete prototype 反证 |
| 把当前应用概念提升成 Framework 合同 | `agent2` 无法独立复用且再次泄露 | P1 只读消费审计只记录需求证据；公共类型脱离产品独立说明；architecture tests 禁止产品依赖 |
| 把所有模式做成枚举 | 添加模式持续修改中心 switch | Definition/Execution 多态，显式装配 |
| 复制旧目录和类型 | 历史耦合进入新模块 | 从行为合同和测试开始，按 owner 新建 package |
| 忽略旧实现经验 | 重复已解决的并发、恢复和边界错误 | 每阶段先直接审查旧代码和测试并记录裁决 |
| 新旧实现长期并存 | 两套术语和维护成本 | P11 是完成定义，最终只保留 `agent` |
| 过早设计 Platform | Engine 未稳定就形成 god object | Engine/Strategy 先行，Platform 在 P8 实现 |
| 递归指数扩张 | 成本、延迟和错误快速放大 | 深度、fan-out、总量、预算划拨和并行硬限制 |
| 子 Process 权限提升 | 安全边界失效 | capability attenuation，只允许父能力子集 |
| 把稳定 EffectID 误当外部 exactly-once | 恢复时重复不可逆副作用 | dispatcher 明确 retry 能力；默认不重投不可证明可去重的 Effect；Host/adapter 拥有事务和业务幂等 |
| snapshot 假持久化 | 恢复重复副作用或定义漂移 | 精确 DeploymentRef、Effect settlement、严格 codec 和恢复测试 |
| Delta 被当作权威历史 | 丢包或重连后语义不完整 | 有界 best-effort、显式 drop Event、恢复不补播、final Output 独立完整 |
| P1 过早冻结公开合同 | spike 假设变成长期债务 | P1 只产候选合同；P3 真实 Interaction 与 P4 第二消费者共同通过后冻结 |
| Host 解析策略 payload | `app/runtime` 再次抽象泄漏 | opaque envelope + architecture tests |
| 多 Agent 复杂度被滥用 | 成本更高、效果更差 | direct-first，要求 eval 证明复杂编排收益 |

---

## 8. 执行日志

| 日期 | 阶段 | 实际事实 | 验证与结果 |
|---|---|---|---|
| 2026-08-06 | P7 | 完成 evaluator-optimizer 组合纵切面。只读审计旧 Go/Embabel `RepeatUntilAcceptable` 与独立 `flow` Loop：保留 original input、ordered attempt/feedback、latest feedback、best-so-far、显式 acceptance criteria 和 hard limit；移除 GOAP/Action 包装、mutable Blackboard、runtime type/timestamp、默认 threshold/limit、第二 Journal，以及把 limit stop 混成 acceptable。新增独立 command consumer，只用 Workflow Loop + exact body/optimizer/evaluator child；typed objective/history/current/best/accepted 全属 consumer。optimizer 消费最新 feedback，evaluator 追加 immutable attempt，仅严格更高分替换 best；同分保留最早。threshold、score schedule、max iterations 与 exact child refs 均进入相关 Deployment identity | command 三轮在 0.95 达到显式 0.9 阈值，稳定输出 best revision、score、attempts、accepted、iterations 和十 Process tree。行为测试覆盖首轮提前接受并按 exact Deployment 名验证 root/body/optimizer/evaluator 四 Process；三轮耗尽以 accepted=false 正常完成并返回第二轮 0.9 best 而非最后 0.3；同分保留首轮；第二候选包含首轮 feedback；zero iteration、score 数量错配、zero/NaN threshold、infinite score 均构造期拒绝。未新增生产 package/API/state/wire，五个 package digest、Snapshot v3、TreeSnapshot v1 不变。standalone mod tidy/diff/build/vet/staticcheck/test/race 与六个 examples 全绿；P7-05 完成，P7 更新为 5/7 |
| 2026-08-06 | P7 | 完成 orchestrator-worker 组合纵切面，不新增 Supervisor/Worker/Task package、Strategy、kind、dispatcher、registry 或 facade。新增独立 command consumer：decomposer Interaction 按输入产生 consumer-owned typed task list，Workflow Map 以显式 item limit/window 创建 exact worker child 并稳定聚合，synthesizer Interaction 形成 typed report；最终 tree 为 root + 两个 Interaction child + 三个 worker child。新增公开 API 行为测试证明另一条正交组合：Interaction 同轮选择两个 exact Planning Delegates，两个 GOAP child 独立达到可观察 Goal，模型消费有序 ToolResult，completion validator 再从两个 immutable typed `planning.Output` artifacts 确认 Achieved；父级不读取 Plan/Action 内部状态 | command 稳定输出 objective、三个声明顺序 worker、综合摘要和 `processes: 6`；Planning Delegate 测试验证两个真实 Action execution、两个 exact Planning child 和三 Process tree。反向审计确认 task/result/prompt/综合规则均留在 consumer，Planning 只用于 machine-verifiable goal，Workflow 只负责确定调度，Tool/Dispatcher 不获得 Engine。五个 package API digest、Interaction State v3、Snapshot v3 与 TreeSnapshot v1 均无变化。standalone mod tidy/diff/build/vet/staticcheck/test/race 与五个 examples 全绿；P7-04 完成，P7 更新为 4/7 |
| 2026-08-06 | P7 | 完成 Interaction-owned typed artifact 与 completion validator 纵切面。成功 Delegate child 只有在 Completed、Output 存在且再次通过 exact frozen schema 后，才按模型轮次/原 ToolCall 位置追加 immutable portable record；普通 Tool、错误结果、start failure 与非 Completed child 均不进入。公开 `Artifact` 只暴露 exact Delegate name、immutable erased Output 与 `DecodeArtifact[T]`，WorkingContext/candidate Output/`Artifacts.All` 均防御性复制；不暴露 Process/child/Host identity、runtime type、Blackboard 或产品 store。可选 `CompletionValidator` 冻结于 Definition，以当前 Interaction WorkingContext + candidate Output + artifacts 返回显式 Accepted/Feedback；拒绝模型或 direct-Tool 候选都会保留准确上下文、追加 user feedback 并进入下一模型轮次，仍受 MaxModelCalls 硬限制。私有 Interaction ExecutionState 从 v2 直接升级 v3，strict restore 复验 artifact 顺序、identity 与 schema，v2 不双读；Kernel 与共同 snapshot/tree wire 不变 | 真实 Workflow Delegate consumer 证明 typed output 可严格解码、过早模型答案被拒绝、feedback 后第三轮完成；篡改 WorkingContext/`Artifacts.All` 返回值不能影响状态。direct-result consumer 证明 assistant ToolCall→ToolResult→feedback 顺序；非法 decision 进入稳定 contract failure，拒绝后耗尽得到稳定 `interaction.limit.model_calls`。strict tests 拒绝未知 Delegate、错误 schema、重复 identity、逆序 record；state fuzz 3 秒执行 334388 次无失败。Interaction API digest 显式修订为 `9678f94265b227e7d085cc18a264ad3be4cac98709d94638f47c9ee7960e3fee`，其余 API/wire digest 不变。standalone mod tidy/diff/build/vet/staticcheck/test/race 与四个 examples 全绿；P7-03 完成，P7 更新为 3/7 |
| 2026-08-06 | P7 | 完成 Interaction-owned managed Delegate 纵切面。`NewDelegate` 冻结模型专用 name/description、exact child DeploymentRef、目标 Descriptor Input/Output schema、每次调用 Budget 与 attenuated Capabilities；非 object Input、重复 Delegate 名、普通 Tool/Delegate 重名均在启动前拒绝。`NewDispatcher` breaking 改为接收 exact Definition，只负责发出普通 Tool + Delegate 的统一模型 manifest 和执行普通 Tool；AST guard 禁止 Dispatcher 引用 Engine/Process/StartChild/WaitForChildren。Interaction 将一个模型 ToolCall batch 按原顺序切为普通 Tool/Delegate 连续区段：普通区段沿用并发/HITL，Delegate 区段批量声明 Framework StartChild、wait-all，并在完整结算后合成一个原顺序 ToolResult message。非法参数、确定 start failure、child 非 Completed 终态是模型可见 `IsError`；身份/wait/output schema 违约是父 Interaction 合同失败。完整 Delegate cursor/child/wait/result 进入 strict ExecutionState v2，v1 不双读，根 snapshot/tree wire 不变 | 真实 Workflow child consumer 验证普通 Tool + 两个 Delegate + 普通 Tool 混合 batch、三个 Process、原序结果、exact child Budget/Capability；非法参数与缺 resolver 不创建 child；parent Waiting/child Paused 的 tree 在销毁后 exact restore，复用原 ChildID 并完成，最终无重复 child。新增 awaiting-start/waiting-child strict restore fuzz；3 秒 147362 次无失败。Baseline 3 的 Interaction API digest 显式修订为 `086f0c7c0897ddb8a851c9ec45266ee6b97eddfb9c29f495c132f074109e4ff8`，其余 API 与 snapshot/tree digest 不变。standalone mod tidy/build/vet/staticcheck/test/race、四个 examples 全绿；P7-02 完成，P7 更新为 2/7 |
| 2026-08-06 | P7 | 只读审计 clean Embabel main `e2d7b987c` 的 HybridUtilityPlanner、UtilityPlanner、SupervisorInvocation、SupervisorAction、CurriedActionTool、typed supervisor/artifact tests，以及旧 Go supervisor、utility、AgentTool、RepeatUntilAcceptable。确认 Embabel Hybrid 是 satisfaction-first 的单步 greedy planner，不是新执行生命周期；Supervisor 本质是一个 synthetic Action 内的模型 Tool loop，依靠 mutable Blackboard、runtime type currying、thread-local Process 和直接 Action execution，且 max iterations 硬编码为 10、耗尽只记录日志。保留 schema-informed worker manifest、typed goal、每轮重看结果、显式 limit、attempt/feedback/best-of-N；拒绝 Blackboard/type-name completion/Action 内 while-loop/Engine-owning Tool。审计同时发现目标架构虚构了尚不存在的通用 Framework Action：当前 `planning.Action` 明确只有预测语义，无法诚实推出 Tool schema 或执行行为。追加 ADR-A2-040，将 P7-02 治本改为 Interaction-owned managed Delegate：模型友好 Tool 合同 + exact child Deployment，Execution 只经 Framework child Effects 推进 | Embabel 仓库保持 clean；targeted upstream tests 源码已核对。本机没有 Maven wrapper 或 `mvn`，因此没有虚报上游测试执行。`agent2` 仅修改事实/架构文档，Baseline 3 代码和 wire 零变化；diff check 与 standalone build/vet/test 保持全绿。P7-01 完成，P7 更新为 1/7 |
| 2026-08-06 | P6 | 完成独立 `examples/workflow` command consumer：只使用公开 API 装配一个 Call→Fork root Workflow，Call 的 normalizer 与两个 Fork reviewer 都是独立 Workflow Deployment；Engine resolver 只按 exact DeploymentRef 提供三个 child，最终 tree 恰好包含 root + 3 child。消费者审计发现公开 `Concurrency` 实际表达“整窗结算后再开下一窗”，不是持续补位的并发池，因此在冻结前治本改名为 `WindowSize`，生产字段、内部状态、测试、GoDoc 和稳定文档统一术语。反向审计进一步删除没有真实消费者的 `StageKind`、Stage ID/kind/schema getters 与 `Definition.Stages`；这些半套反射 API 既不能重建配置，也不应提前冒充 P8 编辑器/catalog 合同。保留的 Stage 仍是 sealed immutable value，只经六个 typed constructor 产生。追加 ADR-A2-039，并将 workflow exported API/GoDoc 纳入 Baseline 3；根公开 API、Snapshot v3 与 TreeSnapshot v1 wire digest 均未变化 | command 输出稳定为规范化请求、声明顺序 reviews 和 `processes: 4`；四个独立 examples 全部真实运行。workflow API digest 冻结为 `0493f8f7ae6e4cc5a3190735c5d02952ec0e0fdb230794bbb01735b8ecfae055`，既有四个 package digest 与 snapshot/tree wire digest 继续通过。standalone mod tidy/build/vet/staticcheck/test/race 全绿；P6-11 完成，P6 更新为 11/11 |
| 2026-08-06 | P6 | 完成 Workflow 恢复与边界合同。ExecutionState strict codec 继续拒绝未知字段、错误 kind/version 及与 Stage/phase 矛盾的 child/fan-out/loop 游标；所有缺失或错配的 child start、wait-open、completion Signal 以及任何 dispatcher Effect 都统一归类为 `ErrInvalidProtocol`。真实 Fork tree 在 root Waiting、两个 child Paused 时 capture，销毁原 Engine 后由 exact resolver 恢复；逆序恢复第一窗口只创建一次第三 child，最终输出与四 Process tree 都稳定。Host cancel 正确映射 root/child 的 host/parent cancellation；child Budget 与 Capability 继续由 Engine 拒绝升级。等待恢复测试发现共同 Kernel 在 parent 于 child wait 期间终止时只注销 Engine 登记、却未关闭 terminal Snapshot 中 mailbox wait 的不变量缺口；现由终止提交统一关闭全部 wait，并以稳定顺序注销内部 child wait，没有增加 Workflow 字段或改变 snapshot wire | 新增终态 tree 回归、恢复无重复 child、取消传播、预算/能力衰减、零协议 Dispatcher、sealed Stage AST 与 strict-state fuzz 合同；fuzz 2 秒执行 260682 次无失败，Workflow statement coverage 78.5%。standalone build/vet/staticcheck/test/race 全绿，Workflow race 连续 50 次、Kernel 终态 wait 回归 race 连续 50 次全绿；P6-10 完成，P6 更新为 10/11 |
| 2026-08-06 | P6 | 新增 typed `Loop[T]` Stage，唯一合同是输入 T、每轮 exact Body child 执行 T→T、最终输出 `LoopResult[T]{Value,Iterations,Satisfied}`。Body、每轮 Budget、attenuated Capabilities、正数 MaxIterations 与 pure Predicate 全部在构造期冻结；Body 输入/输出 schema 必须与 T 精确相等。Execution 总是先启动 iteration=1 的真实 child，只在 child 成功并验证 Output 后执行 predicate；满足时以 Satisfied=true 完成，未满足且达到上限时以 Satisfied=false 正常完成，不把 exhausted 伪装成 predicate 成功或 Process failure。每轮 ChildKey/WaitKey 纳入一基 iteration，下一轮只有上一轮 child 完整结算后才开始；LoopIteration 进入 strict ExecutionState，最终离开 Stage 时清零。Loop 沿用单 child start/wait 状态机，没有内嵌第二 Execution、Go for-loop 执行 body 或独立 scheduler | 三个真实 Engine 场景分别验证 0→3 在第 3 轮 satisfied、0→2 在第 2 轮 exhausted、初值 3 仍先执行一次得到 4/iterations=1/satisfied。每个终态 TreeSnapshot 恰好包含 root + Iterations 个 Process，直接证明每轮独立身份与至少一次语义；body execution failure 映射为 `workflow.loop.child_failed`。zero MaxIterations、nil Predicate 和非 T→T Body 在构造期拒绝，零值 LoopResult 非法。standalone race、vet、staticcheck 全绿；P6-09 完成，P6 更新为 9/11 |
| 2026-08-06 | P6 | 新增 typed `Map[I,O]` Stage，合同唯一且准确：Stage 输入/输出分别为 `[]I`/`[]O`，一个 exact child Deployment 对每项执行 I→O，Budget 是每个已启动 item 的独立划拨，Capabilities 是每个 child 的衰减权限；`WindowSize` 与 `ItemLimit` 都必须显式为正且 window size 不得超过 item limit。输入长度在任何 child Effect 前检查，超限以 `workflow.map.item_limit_exceeded` 合同失败；空输入不创建 Process 并返回非 nil 空切片。实现没有复制 Fork 状态机，而是把两者真实共有的 indexed child start→wait-open→completion→ordered slots→next window 生命周期收敛为包内 sealed fan-out；Fork 只提供静态 branch binding/reducer，Map 只提供动态 item binding/collector，用户不能实现或绕过 fan-out SPI。Map 每个窗口只解码一次原列表并只编码本窗口 item，不按 item 重复解码整个集合；ExecutionState 统一使用 fanout index/next/window/nullable outputs，避免两套恢复 wire 漂移 | 真实 Engine consumer 以输入 [3,1,2]、window=2 启动三个 exact child Process，输出稳定为 [6,2,4]；空输入在没有 DeploymentResolver 的 Engine 上直接完成，证明没有偷跑 child；三项输入命中 ItemLimit=2 时在没有 resolver 的 Engine 上得到稳定 limit failure，证明拒绝发生在 child start 前。zero/oversized window size、zero item limit、child schema mismatch 均在构造期拒绝。Fork 逆序窗口与最低失败索引测试在共享 fan-out 重构后继续通过；专项 race 连续 20 次、vet、staticcheck 全绿；P6-08 完成，P6 更新为 8/11 |
| 2026-08-06 | P6 | 在同一 sealed Stage 代数内新增 Switch 与 Fork，没有引入 Router、Parallel、Join 或第二 Execution。Switch 用 typed pure selector 返回显式 case ID；构造期要求 case ID 唯一、每个 exact child Deployment 输入 schema 与 Switch 输入精确相等、所有 case 输出 schema 精确相等，运行时只启动被选 case 的真实 child。Fork 是 homogeneous fan-out + typed pure reducer：每个 branch 接受同一输入并产生同一 branch type，异构工作通过 child Workflow 先收敛合同；branch ID 唯一、正数 `WindowSize` 不得大于 branch 数。Execution 按固定窗口批量声明 StartChild Effects，当前窗口所有 start 结果与已启动 child 全部结算后才开下一窗口；start failure 与 child terminal/output failure 都保存于声明位置，并按最低 branch index 归因。结果使用显式 nullable slots 表示未结算，避免 `json.RawMessage(nil)` 经 JSON round-trip 变成字面 `null` 后伪装成已完成值。ChildKey/WaitKey 改为带 namespace 的长度前缀 SHA-256 稳定派生，既保留恢复身份又不会因合法 Stage/case/branch ID 组合超过 Framework identity 上限 | 真实 Engine 测试用三个拥有 dispatcher Effect 的独立 child Deployment 验证 window=2：first/second 启动后 third 在 50ms 观察窗内确定未启动；second→first 逆序释放并全部结算后 third 才启动；reducer 最终仍收到 first/second/third 声明顺序。两个同窗失败 child 以完成时序无关的最低索引 `first` 归因。Switch 的左右 case、未声明 case 合同失败、重复/mismatched case，以及 Fork 的空 branch、zero/oversized window size、nil reducer 均有测试。专项 race 连续 20 次、vet、staticcheck 全绿；P6-07 完成，P6 更新为 7/11 |
| 2026-08-06 | P6 | 新增独立 `workflow` 生产 package，完成 sealed Stage、Definition、Execution 与 zero-state Dispatcher 的首个纵切面。Stage 只能由 typed `Transform` 或配置化 `Call` 构造，不存在用户节点 SPI；Definition 要求非空、唯一稳定 Stage ID，并在构造期逐边精确比较规范化 schema。Transform 只接受文档合同明确的有界确定纯函数；Call 从 concrete Deployment 提取 exact DeploymentRef 与权威输入/输出 schema，冻结正数 child budget 和合法 capability set，不保存 Deployment、resolver 或 Engine。Execution 以 ordered Stage/value/phase/child/wait 的 strict state 推进；Call 只经 StartChild/WaitForChildren Framework Effects 创建和等待真实 child，按 Stage ID 形成稳定 ChildKey/WaitKey，能够正确处理 wait-open 与已完成 child Signal 同时排队。child start、child terminal failure、缺失输出和 schema 违约都有 Workflow-owned 稳定归因；Dispatcher 明确拒绝全部 dispatcher Effect，不形成第二执行边界 | 外部测试 package 只用公开 API 装配 Transform→Call→Transform，真实 Engine 启动 exact child Deployment 并完成结果；失败 child 被 parent 映射为 `workflow.call.child_failed`。Engine 在每个 Step 提交前 capture 并用 Definition.Restore 重建候选，实际遍历 ready、awaiting child start、awaiting wait open、waiting child 与 completed 的合法恢复边界。strict state 测试拒绝未知字段、完成游标伪装为 ready 和 Transform 上伪造 child phase；architecture gate 禁止 `flow`、旧 `agent`、`app`、OTel/slog、Store/Journal/Registry/Graph/Scheduler 与 Host 术语。standalone build/vet/staticcheck/test/race 全绿；P6-05、P6-06 完成，P6 更新为 6/11 |
| 2026-08-06 | P6 | 用户明确恢复 Workflow 设计，只要求吸收独立 `flow` 的部分思想、不强求复用。完成一个仅消费 Baseline 2 公开 API 的 disposable managed Workflow consumer：串行 prepare child 后，以 window=2 分批启动三个 fork child；前两路 Paused 后 capture 完整 tree，销毁原树，在新 Engine exact restore，再以第二路、第一路的逆序完成。第三路在第一窗口收口前不存在，收口后只创建一次；最终结果仍按 first/second/third 声明顺序聚合。由此接受 ADR-A2-038：`flow` 继续独立负责 in-process 组合；原生 Workflow 只负责 managed child Process；定义使用有序 Stage，最小词汇为 Transform/Call/Switch/Fork/Map/Loop，其他模式从中组合，不恢复任意 DAG、Registry、Store/Journal、第二 scheduler 或强制 adapter | 定向行为测试 `-count=20` 与 race `-count=20` 全绿；完整 tree 在恢复前只含 prepare + fork[0:2]，恢复完成后恰好含 prepare + fork[0:3]，证明窗口、稳定身份、声明顺序与无重复创建。spike 随后整体删除，`flow` 仓库和 `agent2` 生产代码零修改；P6-03、P6-04 完成，P6 重写为 11 项并更新为 4/11 |
| 2026-08-06 | P6 | 按用户要求立即暂停 Workflow 模式设计与实施，删除尚未提交的 `agent2/workflow` 空骨架；只读审查独立 `github.com/Tangerg/flow`。确认 `flow` 已以 typed Node、派生组合子、runtime Graph/Spec、copy-on-write Store、Journal、suspension/resumption、streaming 和 deterministic merge 覆盖普通 in-process control flow，且准确排除 distributed scheduler、durable timer 与 exactly-once。裁决 Host 可直接使用 `flow`；未来可以 disposable spike 验证把整个 Node 作为单个 dispatcher Effect 的 coarse-grained adapter；禁止在纯 `Execution.Step` 内调用 `Node.Run`/`workflow.Step.Run`，也不把共享 Store/Journal/goroutine scheduler 误称为独立 child Process。ADR-A2-037 已整体取代预先冻结原生 Workflow 的 ADR-A2-036，目标架构不再声明节点词汇或 package | 审查基线为 clean `flow` main `6280bfc4` 且与 `origin/main` 一致；Go 1.26 module 的 vet、staticcheck、test、race 全绿，所有生产 package statement coverage 100%。`flow` 仓库零修改，`agent2` 零 Workflow 生产代码；P6-02 完成，P6 更新为 2/3 并保持暂停实施 |
| 2026-08-06 | P6 | 在生产代码前接受 ADR-A2-036 并将稳定边界写入目标架构：Workflow 是 schema 化 DAG 的原生 Execution，普通图禁止环，Loop 独占有界重复；唯一节点词汇为 Transform/Gate/Switch/Call/Fork/Map/Vote/Loop。所有 I/O 与独立 branch/iteration 都是真实 child Process，纯回调只在 Step 内归约；Fork/Map 以显式固定执行窗口分批启动，Map 另受 item limit，聚合按声明顺序。Loop 输出显式区分 predicate satisfied 与 iteration limit exhausted。Workflow 不建立任意节点 plugin SPI、第二 runtime 或 Strategy dispatcher 能力 | 文档职责保持分离：长期取舍进入 DECISIONS，稳定结构进入 ARCHITECTURE，实施事实只记本行；未把设计批次误报为 P6-02 完成，P6 保持 1/8 |
| 2026-08-06 | P6 | 完成旧 `workflow`、`routing` 与 supervisor example 的只读审计。保留 Sequence 显式值流、真实 child Process、Fork/Map 有界并发与声明顺序聚合、Gate/Switch 确定选择、Vote 稳定同票、Loop 显式终止和完整恢复。确定唯一术语：Workflow 内用 Switch 而非 Router、Fork 而非 Parallel/ScatterGather、Vote 而非 Consensus、Loop 而非 Repeat/RepeatUntil、Call 而非 SubAgent。治本式移除 Workflow 编译 GOAP、builder 持有 Engine/构造期 Deploy、Action 内 RunChild、Blackboard latest-object/history/computed condition、裸 goroutine Generator、Team definition 合并、独立 Supervisor Strategy；RepeatUntilAcceptable 留给 P7 evaluator-optimizer 组合，catalog Ranker/Confidence 留给 P8 Platform routing | 审计覆盖旧 Workflow 14 个生产文件及对应 Sequence/Parallel/ScatterGather/Consensus/Loop/Repeat/Supervisor/Team 行为测试，并核对 routing 的 candidate identity、稳定 tie、filter/panic 语义。裁决已写入能力台账，只修改 `agent2` 事实文档；P6-01 完成，P6 更新为 1/8 |
| 2026-08-06 | P5 | 完成原生 Planning Definition/Execution/Dispatcher 纵切面。Definition 冻结 Goal、最小 Planner、ActionBinding 与 `MaxActionAttempts`；predictive Action 与 dispatcher `ActionExecutor`/精确 child Deployment capability 完全分离。Execution 严格执行 observe → plan → 单个 Action → reobserve：每轮只采用 Plan 首 Action，reobservation 才确认预测 Effects，definite failure/unconfirmed Action 在本 Process 排除后重新规划；child Action 使用真实 child Process、预算、能力衰减与 all-child wait，完成后仍以 Observer 为现实权威。首次 no-plan 输出 `unreachable`，尝试后无路线或达到上限输出 `stuck`，二者均为 Completed 的 Planning Output；观察失败、Planner error/非法 Plan、协议违约保持稳定 failure 分类。Observation Effect 因无副作用允许 same-identity replay；Action Effect 默认不可重投，unknown outcome 通过公开 definite settlement 显式裁决。真实 child consumer 还证明根 `ChildWaitOpened` 必须暴露 defensive-copy 的原始 Spec 供 Strategy 完整核对；已追加 ADR-A2-035 并冻结包含 Planning/GOAP 的 Baseline 2。P1 disposable Planning spike 已删除 | 多路线两步执行、每 Action 后 replan、现实未确认 fallback、definite failure fallback、unreachable/stuck、观察/Planner/能力失败、exact snapshot restore、unknown Action resolution、真实跨 Strategy child Process 全部通过。Planning state/protocol/Output strict codec 增加 fuzz seeds，architecture gate 保持 Kernel/Host/OTel 泄漏禁入；standalone build/vet/staticcheck/test/race 与定向 fuzz 全绿后提交。P5-06～P5-08 完成，P5 更新为 8/8 |
| 2026-08-06 | P5 | 新增独立 `planning` 与 `planning/goap` 生产 package，完成纯规划纵切面。`Truth` 用 Unknown/False/True 区分未知与已知假，`Condition` 只表达已知命名事实，`WorldState` 是 zero-valid、规范排序、不可变且可严格恢复的观察值；Goal、WorldState、Plan 均不进入共同 Process。`Action` 只拥有预测性的 Name/Description/Preconditions/Effects/Cost，不执行 I/O；`PlannedAction` 只是稳定 Action 引用，彻底切断 planner metadata 与执行 capability。最小 Planner SPI 只有 `Plan(context.Context, Problem) (Plan, found, error)`，没有 Name、Extension、registry、default、Agent、Domain service 或 telemetry。GOAP 使用非负有限成本的 deterministic uniform-cost search：dynamic cost 在边源状态求值、相同成本保持声明顺序、同 state 只接受严格更低 cost predecessor、no-op successor 跳过、direct producer 仅保守短路、context cancellation 与显式 expansion limit 生效；已满足返回 found+empty Plan，完整 frontier 耗尽返回 no-plan，limit 命中返回 `ErrExpansionLimit` 而不谎报不可达。所有 Planner 输出再由 Problem 复算适用性、预测状态、总成本和 Goal satisfaction | standalone build/vet/staticcheck/test 全绿；Planning/GOAP 覆盖率分别 75.9%/87.5%，GOAP 确定性测试连续 50 次通过，Planning 全 race 连续 10 次通过。WorldState strict JSON fuzz 723930 次、Plan strict JSON fuzz 685151 次无失败；architecture gate 递归扫描 Planning packages，禁止旧 `agent`、`app`、OTel/slog、Blackboard、ProcessContext、ConditionEnv/Resolver 和 Host 持久化/产品身份声明。P5-02～P5-05 完成，P5 更新为 5/8 |
| 2026-08-06 | P5 | 完成旧 Planning/GOAP/HTN/Utility/Reactive 的专项只读审计。保留三值 Truth、不可变 WorldState、规范 state key、非负有限成本的确定性 uniform-cost search、源状态动态成本、stable tie order、strictly-cheaper predecessor、no-op edge 跳过、expansion limit/context cancellation，以及 Action 后 reobserve/replan 的领域规则。Planning 将独占 Goal/Condition/WorldState/Plan；Action 的规划描述、PlannedAction 引用和真实执行 binding 分层。治本式移除 `Action.Execute(ProcessContext)`、可执行 Condition/ConditionEnv/ConditionResolver、Blackboard/binding/dependency scope、Agent identity、Domain registry、Planner Extension/name/default 和 Planning 内 OTel。HTN、Utility 与 Reactive 均无当前真实 consumer：P5 不预建对应 package，只保留已审计算法证据，未来必须通过同一最小 Planner SPI 和真实消费重新申请；Stuck/no-plan 保持 Planning Output，不进入共同 Process Status | 裁决已写入能力台账；只修改 `agent2` 事实文档，未改旧 `agent`、`app/runtime` 或生产实现。文档 diff check 通过；P5-01 完成，P5 更新为 1/8 |
| 2026-08-06 | P4 | 完成 P4 最终合同审计并冻结 Baseline 1。公开面最后移除未被任何真实 consumer 使用、只暴露 Kernel 内部状态机规则的 `Status.CanTransitionTo`，以及重复回显调用方已知 wait contract 的 `ChildWaitOpened.Spec`；状态迁移检查保留为私有不变量，wait-opened 只返回 Engine-minted WaitID。根 package doc 从临时 greenfield/candidate 说明改为稳定 kernel owner 说明。新增 `API_BASELINE.md`，将 root kernel、`interaction`、Process Snapshot v3 与 TreeSnapshot v1 的精确范围、变更纪律和明确未冻结能力分离记录。自动守卫对完整 `go doc -all`（包含 exported names、参数名、字段、签名和 GoDoc）及 snapshot/tree schema version、JSON tag、字段类型、嵌套 wire shape 做 SHA-256 校验；breaking change 仍允许，但必须有真实证据、ADR、同提交 baseline 更新和全门禁，绝不以兼容 shim 迁就。新增二叉递归扩张合同：输入 depth 8 理论会产生 511 个 Process，Engine 在并发/调度无关的注册临界区将实际 tree 硬限制为 15，且所有 parent 仍通过真实 child wait 收口，不靠泄漏 goroutine 或软取消截断 | standalone build/vet/staticcheck/test/race 全绿；递归、depth/fan-out/active/tree limit、二叉指数扩张、Waiting tree restore、in-flight settlement capture、prepared child-start recovery 专项 race 连续 50 次通过。Input fuzz 164525、ExecutionState 227431、Transition 346225、DeploymentRef 288304、Process Snapshot 26、TreeSnapshot 20 次执行无失败；三个 examples 的 `go run` 输出符合合同，依赖扫描无旧 `agent`/`app`，空目录和非预期 TODO/FIXME/HACK 扫描无残留。P4-09 完成，P4 9/9 完成，Baseline 1 冻结 |
| 2026-08-06 | P4 | 新增第二个可整体删除的 public-API consumer `examples/composition`，以同一程序中的两个真实路径反证是否需要第二套 runtime 抽象。embedded 路径把一个纯本地 uppercase Definition 直接交给 Engine.Run；composed 路径把同一个 exact Deployment 与一个真实 Interaction Deployment 作为异构 child Processes，由第三个 composition Definition 仅通过公开 `StartChild`、`ParseChildStartResult`、`WaitForChildren`、`ParseChildrenCompleted`、erased Input/Output 和 `DeploymentResolver` 组合。parent 不持有 `*Engine`，不解析 Framework 私有 Effect/Signal/state wire；resolver 只做 exact binding；本地与模型 child 各有独立身份、schema、ExecutionState、budget 和 Result。示例没有 Session、Conversation、Workspace、Store、transaction、UI 或应用 revision 概念，也没有为了 consumer 新增 Framework API | `GOWORK=off go test`、race 连续 50 次、vet、staticcheck、`go run` 全绿；依赖扫描确认不 import 旧 `agent`、`app`、TUI 或 CLI。可执行输出同时证明 embedded 结果 `EMBEDDED` 与 composed 结果 `COMPOSITION | model: composition`。P4-08 完成，P4 更新为 8/9 |
| 2026-08-06 | P4 | 实现完整 Process tree 的一致 capture/restore，并收口 P4-06/P4-07。新增严格、版本化、规范排序的 `TreeSnapshot`：只包含每个 portable Process `Snapshot` 和 Engine-owned active direct-child wait；不含 Store、transaction、revision、lease、resolver 实例或应用 identity。`CaptureTree` 使用 Engine 私有 quiescence barrier：按 root→descendant 阻止新 Step/child expansion，prepared/in-flight Effect 必须先按原 settlement 合同收口；barrier 内仍同步吸收 child completion 与 parent termination，外部控制命令延后到释放后按到达顺序执行，因此不会产生“child 已终态、parent mailbox 尚未记账”的丢信 cut。终态 Process 另有 tree-settled join point，确保跨 Process 通知完成后才进入整树快照。`RestoreTree` 对 root/parent/depth/ChildKey、TreeLimits、能力子集、父级 reserved budget 与直接 child budget 总和、active child wait/mailbox、每个 exact DeploymentRef 和终态 Output schema 做全量校验；所有 Definition/Deployment 解析成功后才在一个 Engine 临界区注册整棵树，失败不留下半棵树。普通 `Restore` 只接受从未形成 child relation、child budget 或 child wait 的独立 root，残缺树明确返回 `ErrTreeSnapshotRequired`。child wait completion 改为同步确认后标记 delivered，已消费或 parent 终态时立即注销，恢复时由 terminal facts + stable SignalID 重建投递缓存而不持久化派生状态。prepared child-start 恢复沿原 EffectID 派生同一 ProcessID/request digest，只创建一次；child failure 仍由父 Strategy 显式裁决 | 独立 build/vet/staticcheck/test/race 全绿；tree capture/restore 专项 race 重复 20 次通过。测试覆盖三 child Waiting/Paused tree 继续执行、in-flight child Effect 未结算时 capture 必须等待且恢复可继续、prepared child-start crash cut 的稳定身份与 exactly-one Framework entity、跨 Strategy exact resolver、单 Process 残缺恢复拒绝、终态 Output 对 exact Descriptor schema 的恢复校验、strict unknown-field 拒绝和 TreeSnapshot JSON round-trip fuzz seed。P4-06、P4-07 完成，P4 更新为 7/9 |
| 2026-08-06 | P4 | 收口 structured tree lifecycle 的取消、deadline、失败与等待边界。任意父级终态都会向仍活动的直接 child 投递 Engine 内部控制命令，再由 child 终态逐层传播，不建立第二状态写入口；父级 deadline 在后代保留 `ParentDeadline`，父级 completion/failure/kill/cancellation 则准确记为 `ParentCancellation`，不会把后代伪装成被 Engine 直接 kill。传播只记录控制意图，不抢占正在结算的 Effect；child 先完成 prepared settlement，再于安全边界提交终态。child failure 仍作为 `ChildOutcome` 进入 parent Execution，由 Strategy 显式选择 Fail、fallback 或继续；Engine 不替编排策略猜测。直接 child 检查成为硬约束，parent 无法越层注册 descendant wait。为满足“终态父不留孤儿”，递归测试 Definition 改为显式等待 child 后再完成，不保留 fire-and-forget 假设 | 全量与 race 门禁通过；专项重复测试证明父正常完成和 kill 都只在阻塞 child Effect 结算后产生 `Cancelled/ParentCancellation`，Host deadline 父产生 `TimedOut/HostDeadline`、后代产生 `TimedOut/ParentDeadline`，child execution failure 由 parent 显式映射为自己的稳定 Failure，跨层 descendant wait 被确定拒绝。P4-06 的结构化终止、失败输入与祖先等待部分完成；幂等 tree recovery 留在下一轮与 P4-07 一并验证，P4 总进度保持 5/9 |
| 2026-08-06 | P4 | 完成递归/跨 Strategy 组合和 Engine-owned 结构、预算、能力约束。same-Definition recursion 每层都是新 ProcessID、独立 ExecutionState/结果/预算的真 child，不是 Go 调用栈重入或同 Process 内嵌套 Execution。跨 Strategy 只经最小 `DeploymentResolver.Resolve(context, DeploymentRef)`；Engine 再校验返回 Deployment 的 exact reference，错 binding 产生 definite child-start Failure 且不构造 Process，resolver 不获得生命周期所有权。新增与本地 `Limits` 正交的 `TreeLimits`：`MaxDepth`、`MaxChildren` 终生 fan-out、`MaxActiveChildren`、`MaxTreeProcesses` 均在 child 注册的 Engine 临界区内硬校验。`Budget{Steps,Effects,Signals}` 是非可再生的 Framework work-unit 划拨；child 必须显式请求正值 budget，一旦成功则从父级可用余额中永久保留，不复制全额、不静默回收；父级自身后续 Step/Effect/Signal 同样受已划拨额度约束。`Capability` 只是 Framework 不解释业务含义的 qualified authority；root 从 EngineConfig 获得上限，child 只能请求父级 `CapabilitySet` 子集。`NewDispatcherEffect(payload, required...)` 将真实能力需求冻结进 Effect wire，Engine 在 prepare 前校验；无权 Effect 零 dispatch，因此 capability 不是无行为的 metadata。budget/capability/tree limits 和 child request digest 全部进入严格 snapshot，不引入用户、订阅、工作区、锁或事务概念 | 独立 build/vet/staticcheck/test/race 全绿；递归、跨 Strategy、限制与衰减专项连续 50 次通过。测试证明三层递归共享 root 但 depth 严格递增，exact resolver 能启动不同 Strategy 且错 binding 被拒绝；depth、fan-out、active-child 和 tree-process 四种上限各自独立命中；合法 capability 子集与 budget 准确进入 child，capability/budget 提权产生稳定 Failure；根 Process 缺失 Effect 所需 capability 时 dispatcher 调用数为 0。P4-04–P4-05 完成，P4 更新为 5/9 |
| 2026-08-06 | P4 | 实现第一组 child Process 组合窄腰。`StartChild(ChildSpec)` 产生 Framework-owned Effect，Execution 只声明 parent-scoped `ChildKey`、exact `DeploymentRef` 和经 schema 验证的 erased Input；ProcessID 由 Engine 基于稳定 EffectID 派生，Process 仍只能由 Engine 构造。`ChildStartResult` 明确区分已创建子 Process 与 definite Failure，resolver 只能返回完全匹配请求 reference 的不可变 Deployment；同 Deployment 递归不需要 resolver。`ProcessRelation` 将 process/parent/root/depth/ChildKey 收敛为不可变值对象，root 与 child 字段集严格互斥，同父 `ChildKey` 只能映射一个 child，relation 进入严格 snapshot wire 并通过 `Process.Relation`/`Snapshot.Relation` 只读暴露。`WaitForChildren(ChildWaitSpec)` 只允许等待直接子级，Effect 立即结算并由 Engine 铸造 WaitID，长时间 child 执行不阻塞 Step 也不持有 prepared boundary。child completion 是 Engine 内部定址 Signal，对外伪造回答被 mailbox 确定拒绝。`AllChildren`/`AnyChild`/`ChildQuorum` 只定义终态数量条件，不暗中取消 loser、不改写 child 终态；达成条件时已终态的 `ChildOutcome` 始终按原 ChildWaitSpec 顺序返回，不按 goroutine 完成顺序提交 | 独立 build/vet/staticcheck/test/race 全绿；child 专项稳定性测试连续 50 次通过。行为测试证明 same-Deployment child 具有唯一身份和正确 parent/root/depth/ChildKey，relation 经 snapshot 不漂移；同父重复 ChildKey 一个成功、一个 definite failure；any 返回首个已终态 child，quorum 返回达标集，all 在逆序完成下仍返回声明顺序；外部 Signal 不能伪造 child completion。P4-01–P4-03 完成，P4 更新为 3/9 |
| 2026-08-06 | P3 | 新增两个可整体删除的独立 command consumer，只依赖公开 `agent2`/`interaction` API 和基础 `chatclient`/`chat`/`tool` 合同，不共享测试 helper，不 import 私有 effect/signal/state protocol。`direct_vs_managed` 在同一个程序中对比最小的 `chatclient.Call` 与 Definition→Dispatcher→Deployment→Engine→Process→typed Output 托管路径，证明框架没有强迫基础 AI 嵌入使用 Process。`autonomous` 以真实可执行 Tool 走通 model→ToolCall→ToolResult→model final，模型决定工具与停止点，Definition 只提供硬性 `MaxModelCalls`。两个示例都用无密钥、无网络的确定性模型，因此它们同时是可运行文档和消费者合同测试。P3 末尾复核确认 Interaction 仍是单一公共概念，没有恢复第二 Runner，没有 Host history/store/UI/approval 抽象，也没有冻结尚未经 P4 验证的 API/wire baseline | `GOWORK=off` 独立 build/vet/staticcheck/test/race 全绿；两个 command 的行为测试和 `go run` 都通过，分别稳定输出 direct/managed 结果与 `20 + 22 = 42`。公开 API GoDoc、依赖列表、空目录、漂移注释、TODO/FIXME/HACK、兼容残留扫描无异常。P3-09 完成，P3 9/9 完成 |
| 2026-08-06 | P3 | 实现默认串行、显式声明才并行的 Tool batch scheduler。`ConcurrentTool.ConcurrencyKey(arguments)` 是 Interaction-owned 可选 capability；未声明或返回 `concurrent=false` 的调用独占当前 batch，空 key 表示本 batch 内无已知冲突，相同非空 key 始终串行。`DispatcherConfig.MaxConcurrentToolCalls` 语义固定为 zero=串行、positive=最大活跃调用数、negative=非法；实现只创建有界 worker，不按 ToolCall 数无界生成 goroutine。调度计划在任何 Tool 执行前完整解析，capability panic 不被当成并行许可；完成顺序不影响最终 ToolResult 的模型 ToolCall 顺序。并行声明同时承诺该调用不会请求外部输入；若违约且 sibling 已可能产生副作用，整个 Effect 按 unknown 处理，不伪造可恢复 checkpoint、不重执行 sibling。zero/单调用路径仍完整保留 P3-05 的 HITL checkpoint 语义。concurrency key 只管理单个模型 batch，不伪装成跨 Process 的业务锁或事务抽象 | 独立 build/vet/staticcheck/test/race 全绿；并发专项稳定性测试连续 50 次通过。行为测试证明最大活跃数严格不越界、逆序完成仍按 ToolCall 顺序提交、同 key 不重叠、未声明 Tool 形成独占屏障、zero 配置保持串行，以及并行 Tool 违规等待时暴露 unknown Effect 且 sibling 只执行一次。P3-08 完成，P3 更新为 8/9 |
| 2026-08-06 | P3 | 实现 Interaction-owned steer Signal，没有新增第二推进入口、middleware 侧路或 Engine 对消息语义的解析。`NewSteerSignal` 仅接受经验证的 user-role 消息，使用调用方稳定 SignalID 参与 Engine 去重，payload 仍为严格版本化且对 Kernel 不透明。Interaction 只在当前 model Effect 或整个 Tool batch 确定结算后消费 steer：模型已产生的最终 assistant 消息先进入 WorkingContext，ToolCall/ToolResult 批次也必须按稳定顺序完整结算，随后才追加 steer 并发起下一次 model Effect。正在结算的操作不中断，不丢弃 in-flight Tool 副作用；公开 GoDoc 明确最坏生效延迟为当前不可中断操作的剩余时长加 Engine Step 调度时间。暂存 steer 纳入 Strategy-owned ExecutionState，可跨 Tool/HITL checkpoint 恢复，但不进入 Framework 共同快照字段 | 独立 build/vet/staticcheck/test/race 全绿；steer 专项稳定性测试连续 20 次通过。可控阻塞模型证明 in-flight request 不可见 steer，仅下一模型轮可见；可控阻塞 Tool 证明 steer 不能越过 Tool batch 结算边界；非 user 消息在投递前即被拒绝。P3-07 完成，P3 更新为 7/9 |
| 2026-08-06 | P3 | 收口 Interaction HITL 的 typed edge，并保持 Kernel 对 payload 完全不透明。Framework `Snapshot` 只新增 `CommittedExecutionState` 和 `WaitID` 两个中性、只读、防御性访问器；它仍不 import Interaction，不解析 prompt/schema/continuation，prepared candidate 也不暴露。`PendingToolInputFromProcess`/`PendingToolInputFromSnapshot` 只在 Interaction package 解码自己的已提交 state，并交叉校验 Strategy WaitID 与 Engine Snapshot WaitID；对 consumer 只暴露 `PendingToolInput.WaitID/Prompt/ResponseSchema`，Tool 私有 continuation state 不越过边界。`PendingToolInput.ResponseSignal` 先本地按权威 schema 验证回答，再用调用方提供的 SignalID 和 Engine-minted WaitID 构造去重 Signal。统一命名为 `ToolInputRequest`、`RequireToolInput`、`ToolInputContinuation`、`PendingToolInput`，避免与 Interaction 初始 `Input` 混淆；没有 approval/user/actor/form 等应用术语进入 Framework | 独立 build/vet/staticcheck/test/race 全绿；Snapshot strict codec fuzz 2 秒执行 20 次无失败。端到端恢复测试增加 live Process 和 stored Snapshot 两条 pending-input 查询路径；非法 schema response 在投递前就被 `ErrInvalidToolInputRequest` 拒绝，错 WaitID 被 Engine `ErrSignalRejected` 拒绝，同 SignalID 在续跑 Tool 执行中再投返回 `accepted=false` 且不重入，终态后的过期回答返回 `ErrProcessFinished`。P3-06 完成，P3 更新为 6/9 |
| 2026-08-06 | P3 | 实现 Tool batch 的 Strategy-owned 精确 checkpoint 与可恢复挂起。Tool 通过语义明确的 `RequireToolInput` 返回经验证的 `ToolInputRequest`；其 prompt 是 consumer-facing JSON，response schema 是权威 JSON Schema，continuation state 只返回原 Tool，不含 Process/WaitID 或应用审批类型。Dispatcher 在确定 settlement 中保存按 ToolCall 顺序的已结算前缀、当前 call 位置、暂停计数和 Tool-owned continuation state，待执行后缀由原模型响应唯一导出。Execution 把 checkpoint 纳入版本化 ExecutionState，通过 Framework wait Effect 进入 Waiting；回答经 schema 验证后，Dispatcher 只向挂起 Tool 的 context 附加 `ToolInputContinuation`，不重调模型、不重执行已结算前缀，随后继续未执行后缀。WaitKey 由 Execution 对 model-call/call-ID/pause-count 稳定派生，WaitID 仍只由 Engine 铸造。Tool Effect 保持 `ReplayPolicyNever`；恢复 prepared 但未能证明结果的副作用只暴露 unknown EffectID，不擅自重投 | 独立 build/vet/staticcheck/test/race 全绿。第一个端到端测试执行“前缀 Tool 完成→后续 Tool 挂起→Waiting capture→杀死原 Process→恢复→回答→续跑 Tool→下一模型轮→完成”，确认前缀 Tool、首次模型和挂起前逻辑均只执行一次，结果顺序不变；第二个测试在阻塞 Tool Effect 执行中 capture，新 Engine 恢复后得到一个 unknown Effect 且 Tool 调用数仍为 1，证明 settlement 不重入和不可证明副作用不重试。P3-05 完成，P3 更新为 5/9 |
| 2026-08-06 | P3 | 收口 Interaction 的三种唯一停止语义。普通完成要求模型返回含完整 assistant message 且有明确 FinishReason 的非 ToolCall 响应，Output 以 `CompletionSourceModelResponse` + `ModelResponse` 表达；工具只有显式实现 `DirectResultTool.ReturnsDirectResult` 的可选 capability，且当前 batch 每个 ToolCall 都如此声明时，才以 `CompletionSourceDirectToolResults` + 原 ToolCall 顺序的 `DirectToolResults` 完成；达到显式 `MaxModelCalls` 时以 `interaction.limit.model_calls` 的 execution Failure 终止，不伪造模型或工具输出。Output 使用严格互斥字段集并验证重复 ToolResult ID；ExecutionState 改为显式保存最终 Output，不再将“待工具结算的模型响应”复用为“已完成结果”。Direct capability 通过 `tool.Capability` 穿透 decorator 链且在 Dispatcher 构造时冻结，panic/非法链直接拒绝 Deployment 组件 | 独立 build/vet/staticcheck/test/race 全绿。端到端测试证明普通模型响应和两轮 model→tool→model 都产生 `CompletionSourceModelResponse`；显式 Direct Tool 只调用一次模型并返回独立 ToolResult Output；非 Direct Tool 在 `MaxModelCalls=1` 时不发起第二次模型调用，而是以稳定 code 失败。P3-04 完成，P3 更新为 4/9 |
| 2026-08-06 | P3 | 在 Interaction Dispatcher 中实现明确的普通/流式模型调用选择；`StreamModelResponses=false` 只调用 `chatclient.Call`，true 只调用 `chatclient.Stream`。流式路径将每个 provider-neutral response chunk 先通过 `chat.ResponseAccumulator.Add` 验证并累积，再编码为严格版本化的 `ModelResponseDelta`；对外只提供 `ParseModelResponseDelta` 和防御性 `Response` 快照。流结束时 settlement 携带 Accumulator 的完整 response，Execution 仍仅由 settlement Signal 产生 final Output，不读取 Delta。空流、nil chunk、非法 chunk 或中途 stream error 都不伪造 final；产品展示语义没有进入 Framework | 独立 build/vet/staticcheck/test/race 全绿。真实 Engine 测试证明 Event/Delta listener 返回 error 或 panic 不影响 Interaction 完成；容量为 1 的慢 listener 使 `DroppedDeltas` 单调增长并发布 `agent.delta.dropped`，同时 final 仍保留 66 个流式片段的完整内容；已完成 Process 的 snapshot/restore 不补播任何历史 Delta 且 Output 不变。P3-03 完成，P3 更新为 3/9 |
| 2026-08-06 | P3 | 新增生产级 `agent2/interaction` package，使用原生 Definition/Execution 和 Deployment-bound Dispatcher 实现第一条完整托管路径。Definition 仅持有 Descriptor、显式 `MaxModelCalls` 和 Strategy-owned state；state 使用 strict versioned wire 自足保存 WorkingContext、模型调用计数、推进阶段与待结算模型响应。Dispatcher 直接复用根模块 `chatclient.Client`、`tool.Tool` 和 `core/chat`，构造时冻结并校验唯一 Tool manifest；model call 和整个 Tool batch 都通过严格可判别的 dispatcher Effect/settlement Signal 完成，Engine 不解析 payload。模型请求的 assistant ToolCall 与 ToolResult 按原顺序归并进下一轮 WorkingContext；重复 ToolCall ID、多 choice Tool 分支、错位 result 和非法恢复状态均确定拒绝。Dispatcher 默认 `ReplayPolicyNever`，不对可计费模型调用或可能有副作用的 Tool 做隐式重投。新增子 package 架构守卫，禁止旧 `agent` 和 `app` 依赖 | 使用已推送的根模块 pseudo-version 建立真实独立 module 依赖，不依赖 `go.work` 偶然解析；`GOWORK=off go build ./...`、`GOWORK=off go vet ./...`、`staticcheck ./...`、`GOWORK=off go test ./...`、`GOWORK=off go test -race ./...` 全绿；真实消费测试覆盖纯模型完成、模型→Tool→模型循环、冻结工具清单、稳定结果顺序和 WorkingContext snapshot/restore；P3-02 完成，P3 更新为 2/9 |
| 2026-08-06 | P3 | 完成旧 `agent/interaction`、`agent/toolloop` 与真实 `app/runtime` 交互路径的 P3 专项只读审计。裁决保留 WorkingContext 自足恢复、ToolCall/ToolResult 稳定顺序、显式并行声明、暂停 checkpoint 和 `chat.ResponseAccumulator` 终值聚合思想；治本式移除 `interaction`/`toolloop` 双公共概念、Strategy/Tool 自铸等待 ID、Runner 多推进入口、middleware 侧路 steer、同步 observer 反向控制执行，以及 conversation/history、价格、run/segment、存储 checkpoint 和业务审批抽象。确定 Model/Tool batch 均通过 dispatcher-owned Effect，HITL 通过 Strategy payload + Engine-minted WaitID，无可证明 settlement 不隐式重试 | 只读证据和裁决已写入能力台账；本轮未改动旧 `agent`、`app/runtime` 或生产代码；P3-01 完成，P3 更新为 1/9 |
| 2026-08-06 | P2 | 完成 P2 最终结构、命名、公开 API 与 wire 审查。将单文件中混杂的 Process loop/control、Step prepare/finalize、Effect dispatch、capture/restore、Event publication 与 panic boundary 治本式拆为同 package 内六个职责文件；没有为了拆文件新增 package、service object、接口或依赖方向。公开命名按返回值和参数语义收敛为 `DeliverSignal`、`UnknownEffectIDs`、`DeploymentRef`、`DeltaBufferCapacity`，补齐 Engine/Deployment/Descriptor/Limits/Usage 配置字段的并发、所有权和 zero-value GoDoc，统一 error 文本风格，更新 package doc 明确候选 API 要到 P3/P4 后才冻结。architecture guard 从误报任意 selector 的词法扫描修正为只审计实际声明，并新增 `Process` 无公开可变字段、所有返回 `*Process` 的入口只能属于 Engine 的构造权守卫。新增真正位于 `agent2_test` 外部 package 的 direct Definition consumer，证明仅靠公开 API 可完成 Descriptor→Deployment→Engine→Result；新增部分 Effect batch 合同，证明已确定 settlement 不丢失、unknown 项不重投、显式 resolution 后按声明顺序产生 Signal。未建立 exported API/wire baseline，遵守 P3/P4 多消费者验证后才冻结的 ADR | `GOWORK=off go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿，证明 module 可脱离 workspace 独立使用；DeploymentRef、ExecutionState、Process Snapshot、Transition、Input 五组 strict codec fuzz 分别执行 226841、206113、15、370722、186165 次且无失败；无空目录、legacy/Host import、应用声明、compatibility shim、TODO/FIXME 或未处理静态检查。P2-08 完成，P2 更新为 8/8；下一阶段从 P3-01 旧 Interaction/toolloop 只读裁决开始 |
| 2026-08-06 | P2 | 完成单 Process Engine 执行闭环。`Process` 只能由 Engine 颁发，公开面只提供 identity/read/control/capture/await，不暴露构造或第二推进入口；Start/Run/Restore 共用一个单写者循环，Step、Snapshot、Restore、dispatcher 和 durability acknowledgment 的 panic 均被分类隔离。实现 running/waiting/paused/terminal 控制、安全边界 pause/resume/cancel/kill、Host context 终态来源和 first-terminal-wins；in-flight Effect 不因控制请求被静默遗弃。将 mailbox 接入真实执行：WaitID/EffectID/内部 settlement SignalID 均由 Engine 稳定派生，wait-opened Signal 与外部 answer 分开记账，只有回答 Signal 与候选状态共同提交时才关闭 wait；重复 SignalID 幂等，错 WaitID/状态确定拒绝。实现完整 prepare → optional durable ack → dispatch/settle → finalize：candidate state、消费范围、冻结 Effect、逐项 settlement 与 pending control 可 capture；失败 Step 丢弃实例并从 last-stable 重建且不吞 Signal；exact Deployment restore；Framework Effect 可稳定恢复，dispatcher 仅在 `same_identity` 合同下重放，unknown 保持 prepared 并通过 `UnknownEffectIDs` + `ResolveEffect` 显式裁决。snapshot 使用严格、版本化、opaque wire，保存 lifecycle、mailbox、last-stable、prepared、limits 与 usage，不含产品水位、Store 或 transaction。Event 区分 attempt/committed 并保持 Process-local 顺序；observer error/panic 不改变结果；Delta 使用显式容量的 Engine-owned bounded queue，慢 listener 触发可观察 drop，final Output 独立；最小 Limits/Usage 只记录 Framework step/effect/signal/delta 事实，不引入价格、token、模型或 Tool 术语 | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；新增真实 Engine 行为覆盖正常 effect、durable ack 前零 dispatch、unknown 显式裁决、never/same-identity 恢复策略、wait identity、waiting/paused capture→restore→continue、safe-boundary kill、Host cancellation、失败实例丢弃、limit/usage、listener 隔离和 Delta drop；Process Snapshot strict codec fuzz 3 秒执行 33 次且无失败。P2-02～P2-07 完成，P2 更新为 7/8；下一轮只做 P2-08 standalone/API/wire/architecture 清洗与阶段验收 |
| 2026-08-06 | P2 | 为 Signal mailbox 补齐确定性 snapshot/restore：wire 明确记录逐项到达序、完整 Signal、消费游标及按 WaitID 稳定排序的 WaitKey/answered/closed 事实；seen 去重集合只从完整 Signal 记录重建，不建立第二真相源。恢复严格拒绝越界 cursor、非连续 sequence、重复 SignalID/WaitID、重复 open WaitKey、未知 wait 的 addressed Signal，以及没有对应 Signal 的 answered wait；恢复后已消费前缀、pending 顺序、重复投递与提前回答语义保持一致 | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；mailbox 已具备接入 Process snapshot 的完整状态合同，P2 阶段进度保持 1/8 |
| 2026-08-06 | P2 | 完成 Engine 将消费的 Signal mailbox 领域对象，但不提前暴露第二执行入口。新增 immutable SignalRequest，由 Engine 负责补充 received-at 并转换为 Signal；mailbox 原子拥有到达序、SignalID 去重、pending prefix 和消费游标，失败的 cursor commit 不改变权威位置。WaitKey→WaitID 映射支持 Engine 注册后的提前回答：回答到达后 enter-wait 会确定跳过 Waiting；同一 SignalID 重复幂等成功，不同回答、未知/closed WaitID、Waiting 下无地址输入及 Paused 下有地址输入均确定拒绝；Paused 可排队普通输入但不消费。mailbox 不读取 Strategy payload，也未加入公开 ProcessView/god context | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；这是 P2-03 的完整内部构件，Engine delivery 与 snapshot 尚未接入，因此阶段进度保持 1/8 |
| 2026-08-06 | P2 | 将 P1 的 exact binding 证据提升为正式 Deployment 聚合：DeploymentConfig 必须显式提供 Definition、Strategy Dispatcher、implementation digest 与 configuration digest；Deployment 冻结 Descriptor 和 DeploymentRef，并持续拒绝 Definition contract 漂移。新增由 Engine 构造的 immutable EffectRequest，准确携带 ProcessID、Step sequence、batch index、EffectID 与防御性复制的 Effect；新增 Dispatcher SPI、无错误回传的 DeltaEmitter 和只允许 never/same-identity 的最小 ReplayPolicy。复用 typed-nil 检查，未引入 registry、resolver、Store 或通用 DI；Typed adapter 仍不拥有启动能力 | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；P2-01 完成，P2 更新为 1/8 |
| 2026-08-06 | P1 | 完成 P1 contract gate 与候选 API 清洗。新增生产源码 AST 架构守卫，禁止根 Framework 依赖旧 `agent`、Host `app` 或声明 Session/Conversation/Workspace/WriteSet/Store/Repository/Transaction/Lease 抽象；终态表覆盖 Engine kill、Process/parent/Host deadline、parent/Host cancellation、execution/contract/external/panic failure 和 completion 的完整优先级；补齐 Status、Framework Effect 和 DeploymentRef strict codec 证明。公开 API 审计删除 `Typed.Start`，避免 typed adapter 成为绕过 Engine 的第二生命周期入口；Typed 只保留 I↔Input、Output↔O 与显式类型擦除。新增 `ComputeDigest` 作为 Deployment assembly 的唯一直接哈希入口，删除无意义 helper，确认剩余候选类型均由架构合同、两个 Strategy spike、Prepared Step 或观察协议直接证明；无兼容层、应用抽象、TODO 或占位实现。P1 仅验证候选合同，没有冻结 API/wire，也没有提前实现 Engine | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；Input/ExecutionState/Transition/DeploymentRef codec fuzz 分别执行 250757、309026、402347、112284 次且无失败；P1-08/P1-09 完成，P1 更新为 9/9 |
| 2026-08-06 | P1 | 完成 exact Deployment 与 Prepared Step disposable harness。将字符串 digest 治本式收敛为不可伪造的 SHA-256 `Digest` 值对象；DeploymentRef 明确绑定 Descriptor contract、实现代码和冻结 dispatcher/configuration 三份 digest，并校验派生总 digest。候选 Process snapshot 同时保存 last-stable state、Signal 到达序/游标和只读 prepared envelope；prepared envelope 固定 last-stable digest、candidate state、拟消费范围、Transition、按 ProcessID/Step/index 生成的 EffectID、冻结 payload 与逐项 settlement。验证 durable acknowledgment 前绝不 dispatch；ack 后可从 prepared snapshot 恢复并 finalize；Step 先污染实例再失败时丢弃实例且不推进游标；unknown settlement 恢复后不隐式重投；不同 exact Deployment 无法恢复同一 snapshot。未引入 Store、transaction、CAS、lease 或 Host 水位 SPI | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；P1-07 完成，P1 更新为 7/9；Prepared Process/Deployment 仍是测试 harness，未提前冻结 P2 Engine API |
| 2026-08-06 | P1 | 用两个只存在于测试的 disposable spike 反向验证候选窄腰。Interaction 完整走通 model Effect、stream Delta、Tool Effect、Effect 进行期间到达但只在安全 Step 消费的 steer、Framework wait request、Engine-minted WaitID 写回、Waiting snapshot/restore、HITL answer、下一次 model Effect 和不依赖 Delta 拼接的最终 Output；Planning 完整走通 observe、两个具有前置条件/效果的 Action、act、reobserve 和完成，并证明相同 ExecutionState + Signal 产生规范化等价的 Transition/候选 state。由真实消费形态新增 `RequestWait(WaitKey, signalPayload)`：Engine 只解释 wait key 并原样回送 Strategy-owned payload；同时将终态矩阵输入全部降为 Engine 私有，公共面只暴露最终 Termination，避免第二终态写入口。Descriptor 补充 schema 参与 digest 的合同测试 | `go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；P1-03～P1-06 完成，P1 更新为 6/9；spike 未进入生产 package，仍可整体删除 |
| 2026-08-06 | P1 | 建立第二组候选合同：强类型 Process/Signal/Wait/Effect identity；opaque Signal、Dispatcher/Framework Effect target 与 definite/unknown settlement；严格可判别的 ExecutionState 和 Transition；Definition/Execution 非泛型窄腰及 `Typed[I, O]` 边缘；完整 Process Status 合法迁移表；基于 Engine 控制事实优先级而非 error 猜测的终态裁决；区分 attempt/committed 的 Event 与 Effect-local、非权威 Delta。API 反向审计移除未被当前行为使用的 WaitKey、通用 Framework Effect 构造器和公开 StepOutcome 枚举，并将复用语法从 Definition 专属命名收敛为私有 qualified-name 规则。候选合同仍待 Interaction/Planning spike 验证，因此 P1 保持 2/9，不提前冻结或标记完成 | `go mod tidy`、`go build ./...`、`go vet ./...`、`staticcheck ./...`、`go test ./...`、`go test -race ./...` 全绿；ExecutionState codec fuzz 3 秒执行 326958 次、Transition codec fuzz 3 秒执行 424157 次，均无失败 |
| 2026-08-06 | P1 | 完成旧 `agent` 与真实 Host 消费面的只读审计并将中性需求证据写入能力台账；确认旧 ProcessView/ProcessContext、共同 snapshot 的策略类型、同步事件监听和根聊天 GOAP 包装均不能复制。建立第一组候选基础值对象：不可变且限长的 erased JSON Input/Output、编译并禁止隐式外部加载的权威 Schema、只拥有 Definition 静态合同的不可变 Descriptor 与合同 digest；实现过程中主动移除错误归属到 Descriptor 的 Deployment revision。P1-05 尚未完成，typed Definition adapter 及多 Strategy 证据仍待实现 | Schema 实现实测发现并修复元 Schema 未完整校验和数字类型误判；`go mod tidy`、`go build ./...`、`go vet ./...`、`go test ./...`、`go test -race ./...` 全绿；Input codec fuzz 3 秒执行 420342 次无失败；P1 更新为 2/9 |
| 2026-08-06 | P0 | 完整吸收二轮设计复审：新增能力裁决台账；补齐 opaque Signal/Engine-minted WaitID、Transition/Effect/Event/Delta、erased JSON + typed edge、终态矩阵、Prepared Step 两阶段提交、durability boundary、真实 child Process Fork/Map、stream/steer 和 Host 外部事实清理合同；Supervisor 降为 orchestrator-worker 组合；公共设计文档移除具体应用依赖，P1 改为候选验证、P3/P4 后才冻结 | 独立复审确认无剩余 blocker；稳定架构/ADR/工程标准无具体 Host module 引用；`git diff --check`、`go build ./...`、`go vet ./...`、`go test ./...`、`go test -race ./...` 全绿；P0 更新为 6/6 |
| 2026-08-06 | P0 | 新增独立工程实施标准；明确治本式实施、恰当抽象、DAG、Framework/Host 边界、下游事务/幂等扩展、Go 风格充血模型、组合式 OOP、API 人体工程学和批次完成定义；新增三项 ADR；接入模块级指引且不复制四份职责文档正文 | 文档一致性和 diff check 通过；`go build ./...`、`go vet ./...`、`go test ./...` 全绿；P0 更新为 5/5 |
| 2026-08-06 | P0 | 创建独立 `agent2` module 并加入 workspace；仅用根 `doc.go` 建立可验证的 package，未引入接口或占位实现；将稳定架构、ADR、实施进度拆为三份文档；明确旧 `agent` 是并存期直接参考实现但不是兼容规范；未修改旧 `agent` 或 `app/runtime` | `go build ./...`、`go vet ./...`、`go test ./...`、独立 module 解析和 diff check 全部通过；P0 3/3 完成 |

---

## 9. 当前下一步

P1–P6 与 P7-01～P7-05 已完成。Baseline 3 继续由 Interaction、Planning、GOAP、Workflow、六个独立 command consumer、managed Delegate、typed artifact、completion validator、递归 child Process、tree recovery、race 和 fuzz 保护。下一轮 P7-06 逐项核对 Anthropic 模式表与现有行为证据，只补真正缺失的测试/最小示例；不为模式名称新增近义 API，不把已有 composition 重写成专用 Strategy。

在 P1–P9 完成前，不迁移 `app/runtime`，不删除旧 `agent`，不发布 `agent2` 稳定版本。
