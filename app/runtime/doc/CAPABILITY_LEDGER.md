# Lyra Runtime 能力迁移台账

> 状态：当前能力事实，随每个实施批次更新
>
> 基线日期：2026-08-12

本文记录当前能力事实、目标 owner、迁移 verdict、实施阶段和验收证据。它不重复目标架构和 ADR。代码变化后必须在同一批更新对应条目；不能用“计划保留”冒充“已经迁移”。

## 1. Verdict

| Verdict | 含义 |
|---|---|
| `Retain` | 所有权和抽象正确，保留实现，只做必要接线/命名同步 |
| `Refactor` | 产品语义保留，但 API、package、职责或依赖需要治本调整 |
| `Rewrite` | 能力保留，但当前实现建立在旧执行模型上，按新合同从零实现 |
| `Remove` | 能力重复、owner 错误或已经由 Agent Framework 拥有，完成阶段必须删除 |
| `Defer` | 不是当前服务端重构前置条件，有真实需求后单独设计 |

## 2. 当前基线事实

### 2.1 规模与依赖

- Runtime 源码、测试、`go.mod` 与 `go.sum` 对旧 `github.com/Tangerg/lynx/agent` 的依赖均为零；architecture guard 将其作为永久禁止边，而非迁移数量台账；
- `go.mod` 直接绑定已发布 Agent Framework Baseline 20 pseudo-version `v0.0.0-20260811152247-8e667d716b22`；workspace 与 `GOWORK=off` 消费同一 canonical source，standalone tidy/build/vet/test/race/staticcheck/lint 全绿且没有 `replace` 或 Runtime sanitizer；
- Agent Framework production import 只允许位于 `adapter/agentexec`；Domain、Application、Infra、Delivery、Bootstrap 与通用 Toolset 对 Agent Framework concrete types 为零；
- P2 已删除 `domain/execution` 及全部 forwarding/alias path；Domain 生产代码与测试对 Application/Adapter/Infra/Delivery/Bootstrap 零 import，context-based I/O port 为零；
- Run、Accounting、Conversation、Transcript、Interrupt、Tool 与 ToolResult 已成为准确顶层 bounded-context package；P16-01 已由 `run.Run` 接管完整 Run aggregate，Transcript 不再承载第二 Run lifecycle，Run/Tool failure taxonomy 按 owner 分离；executor checkpoint/ref、pending continuation 与 workspace mutation 由 Application consumer 拥有；
- P3 已删除 Application 的 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖接口；root start/observe/release、Session reads/termination 与 Run projection write-sets 均由真实 consumer-owned ports 表达；
- Application executor tree identity 已统一为 `ExecutorMember`/`MemberID`；Framework `ProcessID` 只存在于 `adapter/agentexec` 内部映射，SQLite technical shape 使用 `root_member_id`/`memberId`；
- 同一 Interaction tree 已在生产 Bootstrap 接通 conclusive child start、durable Delegate child Run attribution、nested/sibling child reconciliation，以及 one-shot prepared waiting-subtree cancellation；
- SQLite 当前唯一 shape 为 epoch 69：model/tool invocation operational journal 与 Transcript semantic final 分离，interrupt row 具有 `open`/`resuming` answer-claim 状态，accepted Question answer 与 claim/checkpoint 更新同事务提交，child-start reservation payload 由 `runsegment` 显式编码；Goal/Run provenance 使用目标 incarnation，Goal 同时冻结 canonical Run capabilities；Transcript/committed-tool failure payload 使用准确的 `failure` vocabulary，并区分 owning Run 取消、执行失败、审批拒绝与父 Run 上的 child Run 取消；Run pump 是唯一 reducer/persistence writer；
- Agent Framework production dependency 只存在于执行防腐层；Runtime 其他 ring 对 Framework concrete type 为零。

### 2.2 当前架构基础

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run lifecycle | `application/runs` 拥有 start、pump、waiting、resume、cancel、terminal ordering；child opening reservation、conclusive start 与 waiting-subtree transaction ordering 已在生产冻结 | Retain | P3–P8 已完成 |
| Session lifecycle | `domain/session.Session` 独占 construct/edit/fork/restore replacement、revision/time 与 generated-title first-writer；Application 独占 workspace/admission、Run opening 和跨聚合 lifecycle write-set；SQLite 只做 exact Insert/Save CAS | Retain | P2/P8；P16-04 authoritative aggregate |
| Domain framework isolation | P2 已删除十个 context-based Domain I/O port，生产与测试均由机器守卫禁止向外依赖 | Retain + strengthen | P2 已完成；例外为零 |
| Agent anti-corruption | Agent Framework tree、model/tool Effect、waiting/restore/steer、Delegate child 与 prepared subtree change 均由 `adapter/agentexec` 生产 owner 独占 | Retain | P8 已接管生产；旧 Framework lifecycle 已删除 |
| Delivery separation | target DAG 禁止 Delivery import 任意 concrete Adapter；protocol/dispatch/server/transport 已按准确职责收口 | Retain | P1/P9/P10 已完成 |
| Adapter/Infra direction | Adapter 单向使用 Infra；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零 | Retain | P9 已完成 |
| Shared mechanisms | `component` umbrella 已删除；根级只保留三个真正跨环 capability，Application 共享机制归还 Application | Retain exact owners | P18 所有权纠偏，永久 purity/owner guard |
| Contract generation | `contract/` 的 manifest/OpenRPC/schema/TypeScript/Go validator 来自唯一 registry；public Go API baseline 来自真实 type information；canonical samples 同时经 round-trip 与 strict validator | Retain | P10/P19-06 已完成 |
| SQLite exact epoch | epoch 69 单一 shape，无生产 migration chain | Retain | P6/P8 完成；P15-03 收回 child-start durable wire owner；P16-02 收回 Tool failure vocabulary；P21 区分 owning Run 取消；P32 收敛 Goal incarnation provenance；P34 冻结 Goal capabilities 与 accepted Question answer |

### 2.3 P19 公共 Go binding 事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| 公共 protocol | 唯一 binding-neutral DTO/validation/version owner；旧 internal path 已物理删除 | Retain exact owner | P19-02 完成 |
| Typed operation | 唯一私有 catalog/invocation/policy owner；HTTP 与 embedded 已直接复用 | Retain exact owner | P19-03/P19-05 完成 |
| Embedded Runtime | 公共 concrete binding 完整覆盖唯一 operation catalog；直接复用严格验证、能力、幂等、problem 与 replay，不建立 transport round-trip | Retain exact owner | P19-05 完成 |
| Runtime instance lifecycle | `bootstrap.Instance` 唯一拥有装配、恢复、operation、worker、retryable Close 与 canonical data-directory lease；HTTP 只消费同一 Endpoint | Retain exact owner | P19-04 完成 |
| Consumer wiring | CLI/前端/TUI 均不在本阶段修改；Runtime 不提供旧接口适配 | Defer | P19 完成后专项 |

### 2.4 P20 真实 consumer 回归事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| TUI 核心交互 | 隔离 mock PTY 已证明 Shift+Enter 多行、运行中 PageUp 和 Ctrl+O Tool 详情均可工作；这些行为不解释真实 Run failed | Retain + regression | P20-01 已验证 |
| embedded 可观测性 | `embedded` 作为库不安装进程级 OTel provider；HTTP dev 的 `lyra.log` 只是 Makefile 对独立服务进程 stderr/stdout 的重定向。当前 CLI 宿主未安装 provider，`LYRA_LOG_LEVEL=debug` 的真实 embedded 查询仍保持 stderr 为空、stdout 为单行合法 JSON，隔离数据目录也只产生 Runtime 数据库与锁文件 | Host composition gap；禁止 library 擅改 globals | P20-01 已审计 |
| model stream/final 对账 | reasoning/text Delta 各自保持同一开放 Item，直到 `ModelCallCompleted` 以最终完整消息完成原 identity；类型切换不再提前落库或复制 reasoning | Retain authoritative reducer state | P20-02 完成 |
| provider metadata schema | Agent `SchemaFor` 已在唯一 Framework owner 按 `encoding/json` 修正：`json.RawMessage` 接受任意合法 JSON，`[]byte` 接受 null/base64 string；真实 DeepSeek object/string metadata 与 signature 通过，Runtime 未删除 metadata、识别 provider 或绕过校验 | Framework-owned correction；Runtime 无 workaround | P20-03 完成 |
| Tool invocation segment | 审批/恢复后，canonical Tool Item 可保留原 Segment identity，外部调用尝试 journal 归恢复后的 Segment；Run/Item 关联和 SQLite foreign-key/integrity 均正常 | Retain exact semantics | P20-01 已反证非串写 |

### 2.5 P21 运行链路与交互控制事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Effect / Item identity | canonical model/tool Item、provider SourceCallID 与外部 invocation attempt 各有唯一 owner；重复/乱序/replay 不再产生第二次 started | Retain exact identities | P21-01 完成 |
| waiting / recovery | checkpoint schema v1 冻结 product capabilities；Pending、open Item/context、answer claim、continuation opening 与 checkpoint 删除保持原子顺序，boot recovery 在 Goal 注入后探测真实 Agent tree | Retain exact transaction | P21-02 完成 |
| approval / HITL | allow once、deny、session/project/global remember 及作用域边界已由真实 DeepSeek 验证；问题 choice/text、取消、同进程与 crash/restart resume 共用一个 typed continuation contract，restore 不重跑 policy/hook/admission | Retain Runtime ownership | P21-03 完成 |
| cooperative cancellation | Runtime adapter 将 product root/subtree intent 绑定到对应 Agent Effect context；root 覆盖全树，child 只覆盖目标及后代，Framework 继续拥有 safe settlement 后的 lifecycle | Retain anti-corruption owner | P21-04 完成 |
| Segment active duration | 每个 product Segment 在首次激活/continuation 时独立计时，不从长寿命 Agent Process started time 重算，授权/HITL 等人工等待不计入 active | Retain exact accounting | P21-05 完成 |
| Tool cancellation | Tool 所属 Run 的取消造成的终止使用 `tool_canceled` / `toolCanceled`；父 Run 的 Delegate Tool 使用 `child_run_canceled`，内部 sentinel 不进入产品 detail，CLI 显示 canceled | Retain first-class failure | P21-06 完成 |

### 2.6 P24 Runtime/Desktop 全链路硬化事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Session activity / cold hydration | Session activity 由 durable non-terminal Run 派生；Desktop Session projection 使用语义订阅，reload 从 durable running Item 恢复 fold 状态 | Retain durable read model | P24-01 完成 |
| Workspace invalidation | Runtime Git watcher 比较 HEAD/index 语义指纹且禁用 optional lock；Desktop workspace event 只失效对应 query scope，不再把 scoped resync 扩张为 global | Retain exact observation boundary | P24-02 完成 |
| Segment activation arbitration | root-owned arbiter 串行化 opening 后 activation 与 cancel；cancel 先赢时不跨 executor boundary，activation 先赢时 cancel 等待其结果后分类 | Retain single lifecycle owner | P24-03 完成 |
| Boot invocation settlement | Application recovery snapshot 包含全部 open model/tool invocation 并形成 exact write-set；Adapter 与 Run tree、Goal、cleanup 在同一 transaction 应用，Infra 只存 technical journal | Retain cross-aggregate transaction | P24-04 完成 |
| Desktop E2E acceptance | HITL、Plan、Goal、并发 cancel/resume、reload 与真实 `kill -9` 均已由隔离 Runtime/Desktop/SQLite 闭环；空闲页面无自激 RPC | Retain regression matrix | P24-05 完成 |

### 2.7 P25 第二轮反证事实

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Session/workspace projection serialization | live Run stream 活动期拥有前端 fold；durable snapshot 延迟到 stream tail/idle 后应用；snapshot→subscribe 期间 Run 进入 terminal/waiting/stale 时立即重读 durable projection，ack 后跟随失败不回流 start-error 通道；Session adapter 将权威 `session_not_found` 投影为 application port 的 absent snapshot，运行故障继续失败；每次成功 Session 集合读都对账 active/open 导航，本地 delete 不在 Runtime commit 前伪造身份缺失，rename/favorite 失败回滚后必须重读并发权威变更；URL/history 直接挂载的 active Session 在 material view 建立前进入 held-open/last-session memory，权威对账保留 active 并清理 stale ref，relocate 条件写失败重读当前 cwd/revision；workspace resolution 区分 default/authoritative unavailable/transient failure，瞬时失败由 event application 有界退避恢复，retarget/dispose 可取消旧 generation 且旧订阅不发布；Run/Item/Plan duplicate/late fold 单调并保留 HITL continuation；提交中的 HITL batch 若被 standing projection 淘汰则立即释放本地 staged 状态，延迟 rejection 不再覆盖远端已胜出的 continuation | Retain single projection/subscription owner | P25-01/P35/P36/P38/P39/P40/P41 完成 |
| JSON-RPC envelope ambiguity | `delivery/transport.DecodeMessage` 在 SDK decode 前递归拒绝 duplicate/unknown member、空 method、非字符串 id、request/response 混合及 result/error 双载荷；HTTP 只接受 request/notification 并投影 transport problem | Retain exact transport owner | P25-02 完成 |
| Adversarial recovery matrix | HITL 双提交与真实 ask_user resume、Plan revision 2、Goal completed/blocked/budget、cancel/resume、idempotency drift、cursor、transaction failure、断线与 active-Run `kill -9` 已覆盖；崩溃 started invocation 收口 unknown，真实终态无开放 lifecycle | Retain regression matrix | P25-03 完成 |
| Standalone Framework dependency | Agent Baseline 20 已发布为 commit `8e667d716b22`；Runtime 直接绑定远端 pseudo-version `v0.0.0-20260811152247-8e667d716b22`，没有 `replace`、Schema 复制或 metadata 降形旁路 | Retain canonical dependency | P25-04 完成 |
| Goal terminal settlement projection | Domain `complete` 保持 objective terminal fact，Application drive 独占最终 Run accounting 与条件清除；Delivery 将可观察窗口投影为公共 `completing`，Desktop Goal context 保留占位且禁止 lifecycle command，随后由 `goals.changed` 收敛到 `null` | Retain exact cross-layer owners | P37 完成 |
| Desktop command settlement replay | 生成 method policy 是 40 个 command replay 类别的唯一输入；Desktop RPC SDK 为每个 logical mutation 持有稳定 key，对 settlement-unknown transport failure 有界重放一次，并按 typed `idempotency_in_progress.retryAfterSeconds` 有界等待一次；definitive business/4xx refusal 与 caller cancellation 不重放。Run opening 每次 attempt 独占新 event stream，失败流先释放；Session create 的 attempt deadline 留在 Runtime Adapter，Application 只见 Session 结果 | Retain generated policy + SDK settlement owner | P42 完成 |
| Desktop Run opening / cancel settlement | RPC Agent Adapter 将 opening handshake deadline 与 accepted stream lifetime 分离：单次 30 秒，首个 ambiguous timeout 只换 delivery signal 并复用原 mutation identity，第二次 timeout 有限返回；ack 后 deadline 释放而 session signal 继续拥有长流。Cancel response 只在发起 revision 仍成立时提交；失败后由中性 Agent projection 重读 terminal 事实，远端胜出不产生陈旧错误 | Retain Adapter deadline + Application neutral revalidation | P43 完成 |
| Desktop Session mutation settlement | destructive rollback 经 mounted Session 唯一同步 owner 等待权威 commit 并按 Session single-flight；Adapter 消费 `droppedRuns.userInput` 后只向 Application 投影中性 AgentInput。无 checkpoint 的 files/both 不降级为 history。rename/favorite/relocate 条件写按 Session 串行并链式携带成功 revision，失败只条件回滚本字段 | Retain Application command/projection owners + Adapter wire translation | P44 完成 |
| Desktop Goal/Plan lifecycle boundary | Goal lifecycle unary command 共用 RPC 有界同-key settlement；Goal Application 按 Session 串行命令、以 typed response 单调提交并重读，Adapter 独占 wire→Goal read model 与 provider 注册。预算耗尽的 blocked Goal 不暴露无效 Resume，但可由 launcher 原位替换；launcher 状态按 Session identity 隔离。Plan wire snapshot/event 在 Agent Adapter 转成中性 Plan domain 后才进入 fold/view；`goals.changed` 只失效点名 Session | Retain Goal-owned adapter + neutral Agent Plan boundary | P45 完成 |
| Desktop Agent Runtime fact boundary / Composer model restore | Agent SDK 用中性 facts 表达 live event、durable Run/Item/Interrupt 与 cancel projection，Runtime wire 的校验和映射只存在于 Agent Adapter；pending work provider 由 Agent bootstrap 装配，defaults/public surface 不拥有 Agent 内部能力。Composer 在无有效显式偏好时等待 active durable Session model，只有无 Session 才退回 catalog 首项 | Retain Agent-owned anti-corruption boundary + durable model precedence | P46 完成 |

## 3. 产品领域能力

### 3.1 Run、Segment 与 execution package

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Run aggregate | `domain/run.Run` | 保持唯一 aggregate owner | Retain + Refactor | P16-01 已用私有字段统一 identity、lineage、frozen admission facts、lifecycle、active Segment、metrics 与 terminal facts；所有 mutation 经 admission/restore/advance/suspend/resume/terminate/cancel/lost 行为，Application/Persistence/Delivery 只消费完整值 |
| Segment identity/lifecycle | `domain/run` + `application/runs` | 保持；P3 重推 root port | Retain + Refactor port | resume 保持 RunID、打开新 Segment |
| Run limits/capabilities | `domain/run` | 保持 | Retain | admission/restore 同值，不能重新谈判 |
| Terminal outcome taxonomy | Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost | Agent Framework Termination + Application intent 唯一映射 | Retain | P8 完整 matrix 已冻结并覆盖 |
| ExecutorRef/checkpoint | `application/runs` opaque executor binding/checkpoint | P3/P6 按真实 consumer 演进 | Refactor port | Run entity 不保存执行端口细节，无 Agent Framework concrete type/payload parsing |
| Executor member identity | Application `ExecutorMember`、continuation/child binding | 保持不透明 member identity | Retain | P3 Application `ProcessID` 归零；旧 adapter 显式映射 |
| Step count | Run usage | 保留产品需要的计数，区分 Agent Framework Step | Refactor | 不把两种 Step 当同一类型 |

### 3.2 Conversation、Transcript、Knowledge

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Conversation message log | `domain/conversation` + `application/conversations` I/O | 保持 | Retain | Count/Truncate/Seed 不依赖 Run executor |
| Transcript Items | `domain/transcript` | 保持唯一 Item aggregate owner | Retain + Refactor complete | P16-02 已关闭公开 tagged-union mutation：非 Tool variants 构造即 complete，ToolCall 只经 start/complete/fail/abandon/classify 行为迁移；Application 仅拥有 provisional stream `ItemStart`，Persistence/Artifact codec 只在机器守卫允许的技术边界使用严格 snapshot |
| Offloaded transcript content | `domain/toolresult` | 保持准确独立 capability | Retain | 无泛化 blob service |
| Knowledge/LYRA.md | `domain/knowledge` + `application/workspace` + `infra/knowledgefile` | 保持独立 | Retain + CAS hardening | P50 以 opaque content revision、权威返回与 committed-only `knowledge.changed` 消除单进程丢更新；P51 以唯一 physical identity containment、跨进程 document lease、权限继承、fsync+rename 与 cold staging recovery 关闭 symlink 越界和 crash/CAS 缺口；P52 以精确路径指纹观察外部 create/write/rename/remove，API write 则在发布前按 exact identity 接受新基线，避免漏失效和重复 refetch；用户编辑与 Agent state 无关，wire 只在 Delivery/Desktop Adapter 映射 |
| WorkingContext | Application composer + Agent Framework Interaction private state | 保持 Host composition 与 executor state 分离 | Retain | fresh root 读取产品事实；restore 只用 opaque checkpoint，不从 Conversation 重算 |

### 3.3 Interrupt 与 approval

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Interrupt semantics | `domain/interrupt` | 保持 | Retain | Kind/Key/Resolution 纯领域值，无 I/O/executor |
| Pending continuation | `application/runs.Pending` + `adapter/persistence` mapping | 已保持 owner 并接入 Agent Framework public pending input | Retain | 一个 root tree 一个 pending hand-off；Infra 只见 technical record；claim 后普通读取不可见 |
| Approval domain | `domain/approval` | 保持产品策略 | Retain + remove I/O ports | 不进入 Agent Framework |
| Ask-user/approval tool input | Toolset product Interrupt + `agentexec/interactioninput` | public Interaction helper → product Interrupt | Retain | 不解析 private Framework payload；旧 adapter 已删除 |
| Answer/resolution | runs + Interaction input adapter | semantic Application command → WaitID-addressed Signal | P6 Agent Framework bridge 已完成；P14 内部职责精修 | 无任意 Signal API；text/choice answer 各自验证其语义，answer claim/segment opening/Signal 顺序受测试 |

### 3.4 Accounting

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Token/model-call accounting | `domain/accounting` + authoritative model decorator | final/usage/pricing/Run progress 同一事务 | Refactor bridge | P5 已完成；Delta drop 不丢 final 或 usage |
| Pricing/USD | adapter/modelcatalog + observer | Runtime adapter/domain value | Retain | Agent Framework Usage 无价格字段 |
| Framework resource usage | old Agent aggregate | Agent Framework Usage | Replace | 只翻译需要的中性事实 |
| Goal budget attribution | goals/runs | 保持 Application | Retain | child/root、resume、lost 归属准确 |

### 3.5 其他产品上下文

| 能力 | 当前 owner | Verdict | 迁移影响 |
|---|---|---|---|
| Goal | domain/application/toolset | Retain | autonomous Run admission retry、等待边界后的 incarnation ownership refresh 与 terminal resolution 分属准确私有行为；消费端口名为 `AutonomousRuns`，不下沉 Agent Framework |
| Plan | `domain/plan` + `application/plans` | Retain + Refactor complete | P16-03 已由私有 `plan.State` 独占 replacement/invariant/revision/time；Application 形成 CAS replacement 并在成功保存后发布 invalidation，Tool 只消费用例，SQLite 不决定 transition；保持 Plan 唯一术语且不与 Goal/Todo 合并 |
| Schedule | domain/application/toolset | Retain | 通过 `RunStarter` 启动并返回 `StartedRun` 事实；有界 `occurrenceBatch` 分别处理 pending dispatch 与 due claim，不直接调用 Agent Framework。更新 workspace 的 wire 三态由 Protocol/Delivery 拥有，Domain/Application 继续只消费空值=Runtime 默认、非空=已准入显式路径的 `CWD` |
| Skill/Proposal | domain/application/adapter/toolset | Retain | deferred manifest 接线更新 |
| Agent memory | domain/application/toolset | Retain | 与 Conversation/Knowledge 分开 |
| Model/provider catalog | domain/application/adapters | Retain | 每 Run exact model binding 进入 deployment assembly |
| MCP/A2A/LSP | domain/application/infra/toolset | Retain | 保持技术能力，不进入 Agent Framework Kernel |
| Workspace/change/isolation | application/adapters/infra | Retain + recovery audit | 外部事实失效由 Host policy 处理 |
| Hooks | domain/application/adapter | Retain | P5 已在普通 Tool 边界触发；post-hook 是 observation，不覆写 settlement；P50 trust commit 后发布专用 `hooks.changed`，Desktop 按 project 串行且 UI pending 锁定；P52 让 global/project/cwd `.lyra/hooks.json` 的外部新增、替换与删除进入同一专用失效流，文件布局仍只属于 Workspace Adapter |
| Feedback/codebase index | domain/application | Retain | 与 execution migration 无直接耦合 |

## 4. Application 能力

### 4.1 Runs use cases

| 当前能力 | 当前形态 | Verdict | 目标证据 |
|---|---|---|---|
| Start admission | `ValidateRootStart` → `StageRoot` → durable opening → `BeginRoot` | P4 real consumer 已验证；P14 内部命名/依赖边界精修 | Start 与 Resume 各自验证 staging dependencies，共享准确的 segment-supervision dependency boundary；stage 不外呼 model/tool，commit 前失败只 Release |
| Event observation | `ExecutionObserver.Observe` | P4 real consumer 已验证 | 只流 Application-owned executor facts；final 来自 Result |
| Executor release | `ExecutionReleaser.Release` | Retain | 与产品 Cancel 分离；非 Waiting 终止恰好一次 |
| Product Cancel | durable control intent → `RunningRootCancellationRequester` → continued observation → release | Retain；P14 内部职责精修 | durable cancellation Run tree 独占 topology/lifecycle 校验，process-local member binding 保持独立；request cancel 不提前切断 pump，确定终态后才释放 |
| Resume | `WaitingExecutionContinuer.StageContinuation/BeginContinuation` | Retain | exact BuildID/deployment/scope/capabilities restore；waiting continuation 的 envelope/topology/order 与 resume binding 各有准确内部 owner；per-Run reducer、Segment identity 和 deterministic preorder 由私有 resumed-route builder 原子重建；opening commit 后才 Signal |
| Steer | `RunningExecutionSteerer.SubmitSteer` | Retain | semantic steer 只在下一 model safe boundary 投影 |
| Child subtree cancel | `MemberID` + `WaitingSubtreeCancellationPreparer` | P7 real consumer 已重推 | Application 只看 member projection、resulting checkpoint 与一次性 Apply/Discard capability |
| Waiting subtree mutation | `WaitingSubtreeChange` 持有 execution ACL capability | P7 Agent Framework bridge 已完成；P14 精修 commit invariant owner | Application 不见 Agent Framework plan；source 冻结跨过 transaction；contextless Apply 只安装状态，final-boundary Continue 独立激活，失败时 exact restore/terminal 收口；`WaitingSubtreeCancellationCommit` 内部按 boundary、disposition envelope、pending-tree topology 和 surviving continuation 分阶段证明同一原子 write-set，不产生第二 validator owner |
| Run pump | executor event reducer | Retain；P14 内部职责精修 | `segmentStartup` 只拥有 durable opening 前可回滚资源，commit 后转交 pump；ItemStarted projection 与 park-boundary write-set 各有准确 owner；不推进 Agent Framework internal state |
| Run journal | committed RunEvent | Retain | persist-before-publish |

P8 production cutover 已用真实 Bootstrap consumer 冻结 root stage/observe/begin、cancel request/release、waiting restore/answer/steer、child reservation/start outcome 与 waiting-subtree prepare/commit/apply-or-restore 的准确端口集合。

### 4.2 Cross-aggregate writes

| 写集合 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run admission | Session + Run/Transcript | Retain/refactor types | P3 |
| Waiting tree barrier | Run + Pending + checkpoint + Items | Retain semantics；P6 Interaction 已验证 | P6 已完成 |
| Waiting child cancellation | Run tree + Pending + checkpoint + Items | P7 Agent Framework boundary 已完成，保留 Application transaction | P7 已完成 |
| Terminal tree | Runs + Pending + checkpoint + transcript + Goal | Retain | P8 已统一 write-set/first-wins/release ordering |
| Boot recovery | stored facts + opaque checkpoint probe | Retain | P8 已由 Agent Framework production probe 接管 |
| Rollback/fork cleanup | Conversation + Transcript + active/waiting Run | Retain | P8 已保持 cleanup intent 与 live release 分相 |

## 5. Agent execution 能力

### 5.1 当前 `adapter/agentexec`

| 当前实现 | 作用 | Verdict | Agent Framework replacement |
|---|---|---|---|
| `InteractionExecutor` | 唯一生产 tree owner | Retain | per-Run Agent Framework Engine + Interaction；P4–P8 完整纵切 |
| old Engine/GOAP/TurnProcess/turn controller | 已删除 | Remove complete | Agent Framework Process/Engine public lifecycle + Application Run pump |
| private process-tree/suspension codec | 已删除 | Remove complete | opaque public TreeSnapshot + Interaction pending-input ACL |
| child execution/configuration | 传播 model/hooks/budget | Rewrite | exact child Deployments + ProcessAdmitter |
| authoritative projection | model/tool/usage/final 同步 receipt | Retain | P5 authoritative decorators；旧 observer 已删除 |
| tool decorator | approval/hooks/presentation | Retain semantics, Rewrite attribution | P5 ToolInvocation context + P6 interactive HITL 已完成 |
| usage accounting | subtree token/cost | Retain | cumulative accounting/pricing、child attribution 与 terminal tree 已收口 |
| deferred manifest glue | old toolloop promotion | Rewrite | P5 唯一 `toolset.Manifest` + Interaction DeferredTools/AdvertiseTools 已完成 |
| maintenance/restore checks | per-Run Interaction session + checkpoint probe | Retain | exact Deployment/BuildID/scope/capabilities 与 tree-wide boot owner 已收口 |

### 5.2 Agent Framework 已提供的合同

| Runtime 需要 | Agent Framework Baseline 20 | Runtime 责任 |
|---|---|---|
| root execution | Engine/Deployment/Interaction | 组装产品配置并翻译结果 |
| tree identity | ProcessID/Relation/root/parent/depth | 映射不透明 executor member/child Run |
| child admission | `ProcessAdmitter` + prospective identity + `ProcessStartOutcomeAcknowledger` conclusive started/aborted | durable Opening reservation、public Running 与 aborted cleanup |
| waiting | WaitID/Signal + Interaction pending helper | 产品 Interrupt 与事务 |
| restore | Snapshot/TreeSnapshot + exact resolver | Store、BuildID、Host metadata |
| steer | Interaction steer signal + `ModelInvocation.AppliedSteerSignalIDs` exact attribution | Signal ID 到产品内容的 adapter-owned opaque checkpoint mapping；首次可见消息整批原子 projection |
| subtree cancel | `PreparedWaitingSubtreeCancellation` 冻结 source，并提供 prospective result 与 contextless one-shot Apply/Discard | Application write-set 只见 projection + opaque checkpoint；commit 后 apply-or-exact-restore |
| model/tool attribution | ModelInvocation/ToolInvocation | Transcript/accounting/hooks/pricing |
| deferred tools | Tools/DeferredTools/AdvertiseTools | Tool catalog、发现语义和权限策略 |
| observation | Event/Delta | lifecycle projection/临时流；权威记录另行同步 |
| resource facts | Budget/Limits/Usage | 产品 limits、USD accounting 和 policy |
| unknown Effect | UnknownEffectIDs | live/recovery tree 由 wake + bounded public query 收口为 durable RunLost；不开放任意 resolution |
| prepared-step durability | PreparedStepAcknowledger 只有单 Process Snapshot | 初版不启用；只恢复 committed quiescent TreeSnapshot |

当前 root Interaction、ordinary Tool/model、waiting capture/restore、steer、durable Delegate child admission 与 waiting-subtree change 均没有已知 Agent Framework blocker。P7 真实 Runtime consumer 已推动并验证两个中性 Framework 合同：conclusive `ProcessStartOutcome` 与 one-shot `PreparedWaitingSubtreeCancellation`；Agent Framework 仍不感知 Run、Store、transaction 或产品恢复策略。单 Process prepared-step acknowledgment 本轮明确不启用，不算待实现 Runtime 能力。

### 5.3 WorkingContext provenance

| 来源 | Runtime owner | checkpoint 投影 | purpose |
|---|---|---|---|
| base prompt | `adapter/agentexec` | `base_prompt` + builtin reference | instruction |
| home/projectRoot/cwd LYRA.md 级联 | Knowledge reader + composer | `user_knowledge` / `project_knowledge` | instruction |
| pinned/recalled Memory | Agent Memory + composer | 只记录实际注入的 item ID | data |
| AGENTS.md cascade | prompt-source Adapter 保留 canonical path + home/projectRoot/cwd provenance，Application 验证闭合集合，composer 只消费内容 | 只记录预算后实际渲染的 canonical path | instruction |
| Session Plan | Plan reader + composer | `session_plan` | instruction |
| lifecycle hook context | Hook Application + composer | 精确 SessionStart/UserPromptSubmit Part source | instruction |

这些 kind 是 `agentexec` 私有诊断合同，不进入 Runtime Protocol、Application port、SQLite schema 或 Agent Framework。文本顺序、header、预算与 best-effort 读取语义未改变；metadata 只伴随自包含 WorkingContext checkpoint。

## 6. Tool 能力

| 当前能力 | Verdict | Agent Framework 接线变化 |
|---|---|---|
| built-in tool schemas/names/descriptions | Retain | 作为 frozen Tools/DeferredTools 输入 |
| strict typed decode | Retain | P5 Dispatcher 调用同一 Tool contract，无第二 schema/decode |
| approval/safety | Retain Runtime ownership | P5 自动 allow/deny/rewrite + P6 interactive approval/remember resolution；restore 不重跑 plan/hook |
| activity/presentation | Retain | P5 已接线，不进入 Agent Framework Event/Delta |
| `search_tools` | Retain | P5 使用注入的 precise advertiser 调用 `AdvertiseTools`；Toolset 对 Agent Framework 零 import |
| delegation wrapper | Rewrite | Interaction `Delegate`，不再 old AgentTool |
| ask_user | Retain | 真实 Runtime Tool 注入 Interaction pending input；旧生产构造已删除 |
| Plan/Goal/Schedule/Skill tools | Retain | Application ports 更新 |
| MCP/A2A dynamic tools | Retain | 本 Run deployment assembly 冻结 authority |
| old agent/core/toolloop imports | Remove | Toolset 最终 Framework-neutral |

完整模型工具面、参数和历史删除裁决仍以 [`TOOL_SYSTEM.md`](TOOL_SYSTEM.md) 为准。

## 7. Persistence 与 recovery

| 能力 | 当前事实 | Verdict | 验收 |
|---|---|---|---|
| SQLite schema | epoch 69；保留 `root_member_id`/`memberId`，open/resuming answer audit；accepted Question answer 与 resume claim 同事务；Goal/Run 使用 `incarnation_id`/`goal_incarnation_id`，Goal 冻结 canonical Run capabilities；child-start payload 为 adapter-owned canonical JSON；Transcript/committed-tool 使用 closed `failure` taxonomy；tool invocation identity 按 Segment 隔离 | Retain pattern | 旧 epoch/列/codec 被拒绝，无 migration |
| executor checkpoint | Host metadata + Agent Framework public complete-tree snapshot | Retain opaque payload owner | Application/Store 完全 opaque；exact capabilities 纳入 expectation |
| checkpoint transaction | runsegment/persistence 组合 | Retain semantics | waiting facts 同事务 |
| BuildID | Host-owned | Retain | 不进入 Agent Framework deployment/snapshot |
| boot recovery | Application policy + agentexec framework probe | Retain | P8 production recovery wiring 已收口 |
| unknown Effect live/recovery | tree-wide reconciliation | Retain | wake 丢失由 public polling 收敛；RunLost 写失败重试且 release 不提前 |
| isolated recovery | fail closed | Retain | 不猜测重建 scratch world |
| terminal deletion | root aggregate transaction | Retain/converge | terminal 与 delete 原子 |
| waiting subtree persistence | Application transaction + execution-owned prepared change | P7 Agent Framework boundary 已完成 | one-shot prepared change + opaque resulting TreeSnapshot + exact restore fallback |
| active-step crash durability | 旧路径能力不作为新合同 | Defer | 不启用 PreparedStepAcknowledger；无 committed quiescent TreeSnapshot 时 RunLost |

## 8. Delivery 与协议

| 能力 | 当前 owner | Verdict | 说明 |
|---|---|---|---|
| Protocol types/errors | `protocol` + `contract` | Retain | Protocol `2026-08-12`、Artifact v18；Question `answers` 只投影 Runtime 已接受响应，未回答/取消不伪造；ToolCall lifecycle 与 optional exact execution duration 分离，后者排除审批等待且 unknown 时不伪造；Tool cancellation 是独立 `tool_canceled` / `toolCanceled` variant；Schedule workspace patch 以省略/合法 ref/`workspaceMode:"default"` 精确表达保持/设置/清空且生成互斥约束；机器真相仍在 contract |
| Runtime method implementation | `delivery/server` | Retain | Protocol server side 与 projection；构造按 required use-case validation、defaults、contract facts、instance、notification observation 分阶段，不持有 transport listener |
| JSON-RPC dispatch/registry | `delivery/dispatch` | Retain | method registry/router/idempotency；typed params decode 与 response projection 分属 `params.go`/`response.go`，不与 server 合并 |
| HTTP/SSE | transport/http | Retain | envelope I/O、stream/backpressure |
| Embedded Go | 旧 internal channel prototype 已删除；P19 新建类型化公共 binding | Rewrite, do not revive transport | CLI 已成为真实消费者；复用 operation/Application，不导出 envelope/Router |
| Server product-value projections | Application read/write use cases + 必要 immutable Domain values | Retain | 只做 wire validation/error mapping/projection，不读取 repository、不持有 lifecycle owner |
| Delivery adapter imports | architecture guard 禁止 | Retain | Delivery 只驱动 Application；对 concrete Adapter/Infra/Bootstrap/Agent Framework import 为零 |
| frontend/TUI/CLI generated consumers | Desktop generated Runtime bindings、strict validators、samples 与 handwritten SDK 已同步 P25/P26/P33；Schedule SDK update 直接消费生成 request，不以 create shape 猜测 update surface；其他消费者按各自阶段消费公共 `protocol` / `embedded` | Retain boundary | 精确 cutover 见 `CONSUMER_HANDOFF.md`；Runtime 不为消费者建立兼容 shape |

## 9. 结构清理台账

### 9.1 Shared mechanism ownership

| 能力 | 最终 owner | 证据 |
|---|---|---|
| completion | `internal/completion` | Application run/goal、Application taskgroup 与 Infra teardown 共享同一 completion-first join rule |
| HTTP origin | `internal/httporigin` | Application MCP policy 与 Infra MCP redirect security 共用纯 normalization |
| idempotency | `internal/idempotency` | Delivery consumer port 与 SQLite implementation 共享 opaque record/errors |
| opaque token framing | `application/opaquetoken` | pagination 与 Run replay 两个 Application continuation owner 共用 strict URL-safe framing；payload 语义、版本与校验仍归各自 owner |
| pagination cursor | `application/pagination` | 多个 Application read 共用 keyset pagination 语义；Delivery 只消费 Page/error contract，不拥有 cursor policy |
| replay cursor | `application/runs` 私有实现 | 位置、scope、retention、版本与校验均由 Run journal 独占；只复用 ring 外的 opaque framing |
| teardown step | `infra/teardown` | non-cooperative close serialization 是 Bootstrap 消费的纯技术机制；组合根只装配与关闭 |
| task group | `application/taskgroup` | Application coordinator 启动 request-detached work；Bootstrap 只构造、注入和关闭该 Application lifecycle owner，Delivery 不消费 |
| path identity | `infra/pathidentity` | filesystem/symlink identity 是技术机制，Adapter 单向消费 Infra |
| secret masking | `application/secrets` | model/MCP 两个 Application consumer 共享 presentation-boundary policy |
| notification relay | `adapter/notification` | Bootstrap 只组装，producer/Delivery 各见其最小 method set；避免与 Agent Framework Signal 同名 |

`internal/component` 已物理删除；永久架构守卫把根级跨环 capability 与 Application-owned shared mechanism 分开约束。前者不得依赖任何产品 ring，后者不得吸收 Domain 语言或 Adapter/Infra/Delivery/Bootstrap 实现职责。

### 9.2 Adapter/Infra

- 原混合 `adapter/maintenance` 已按真实 owner 物理拆分：`runmaintenance` 只拥有 clean-Run 边界的 history compaction、memory consolidation、Skill proposal mining 与 idle-Skill archival；`sessiontitle` 只生成 Session title；`utilitymodel` 只提供辅助能力共享的 middleware-free 单次模型调用。旧目录、`llm.go`、`extraction.go`、`skillmine.go`、`skillcurate.go` 与 `title.go` 均由架构守卫禁止回流；
- `codebaseindex`、`codeintel`、`executionctx`、`hooks`、`isolation`、`mcpconnection`、`modelcatalog`、`modelclient`、`persistence`、`providerregistry`、`runrecovery`、`runsegment`、`skillproposal`、`toolname`、`toolset`、`workspace` 与 `workspacepath` 已逐个按调用图审计：均拥有应用值映射、外部错误翻译、跨机制组合、安全策略、稳定模型词汇或 transaction write-set，不是同名 Infra 方法的包装；
- 原单消费者 `adapter/pricing` 只读取与 `modelcatalog` 相同的静态模型目录且无独立变化轴，已收回 `modelcatalog.Pricing`；其余小 Adapter 均有明确 Application port、外部 SDK 防腐、安全策略或多个消费者证据，不按文件数机械合并；
- SQLite、knowledge-file、Git、exec、sandbox、LSP、MCP/A2A client、checkpoint、telemetry、path identity 与 advisory lock 只提供 technical mechanism，保留 Infra；Bootstrap data-directory lease 与 Knowledge document CAS 共用中性 advisory-lock 原语，各自在自己的 owner 翻译生命周期/错误；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零；
- 原 `infra/storage` 无行为 umbrella 已删除：SQLite 直接归 `infra/sqlite`，LYRA.md 文件布局与原子替换归 `infra/knowledgefile`；原误置于 Adapter 的进程级 OTel global 配置归 `infra/telemetry`。三个 package 分别只因数据库、知识文件和进程遥测而变化，不再共享泛化 storage/observability 目录；
- MCP live registry 中的已配置服务器状态统一称为 `configuredServer`，从 live projection 移除且等待关闭的连接统一称为 `detachedSession`，按名称过滤的 API 参数称为 `serverName`；进程内连接生命周期不再依赖 `ms`/`old` 等上下文猜测；
- `workspacepath` 原有第二份 symlink/containment 判定已删除，统一消费 `infra/pathidentity` 的 physical identity；Adapter 只保留 Application path-policy 错误与相对路径投影；
- `agentexec` 已按 root execution、effect attribution、waiting/restore、Delegate admission/projection、tree mutation 分离变化原因；Delegate 的 model input/description 是 Interaction executor 独占的策略合同，已从单消费者 `toolset/delegation` 回归该 ACL；没有第二 controller、scheduler、mailbox、private wire owner 或为了文件拆分制造的子 package；
- 原先每个 Runtime built-in tool 一个目录的 `toolset/{agentmemorysearch,askuser,conversationsearch,goal,lsp,offload,plan,schedule,shell,skill}` 已按共同变化轴收敛为 `toolset/builtin`，文件继续按能力家族组织；deferred discovery 回归 Resolver owner，稳定 model-facing identity 从误称 catalog 的 cycle-workaround package 收敛为多消费者 `adapter/toolname`。Run Application 的 process-local lifecycle owner 仍统一为 `runTreeOwner`；Toolset 的 manifest assembly、Call decorator、schedule/shell command group 与 deferred search ranking 分别由 `manifestBuilder`、`callDecorator`、`scheduleManagementTools`、`commandTools`、`discoverableTool`/`rankedTool` 表意；这些全是 Runtime 私有 owner，不进入 Agent Framework；
- Chat provider catalog 以 `ProviderOpenAICompatible`/`ProviderAnthropicCompatible` 准确表达 caller-defined compatible endpoint，以 `buildOpenAIModel`/`buildAnthropicModel` 表达模型 adapter 构造；Ollama 的 chat/embedding 同样只在 Infra composition 内消费 provider-scoped OpenAI-compatible protocol 与 `/v1` endpoint，不再为客户端能力引入完整 Ollama 服务端 module；`Compat` 不再伪装成兼容层，`Native` 不再混称厂商 wire、宿主平台或 Framework 执行能力；
- Hook wire root、codebase in-memory corpus、MCP current dial/mutation scope/tool-list snapshot、usage fold bucket 与 teardown attempt backing state 分别使用 `hooksFile`、`cachedCorpus`、`activeDial`、`mutationScope`、`toolListTarget`、`usageAccumulator`、`attemptState`，不再依赖 `config`/`loaded`/`dial`/`mutation`/`target`/`accumulator`/`attempt` 的上下文猜测；具体错误结构统一使用 `Error` 后缀，注释与测试同步，不保留旧别名；
- `persistence.SessionStores` 仍是 Session aggregate 原子 write-set 的单一 Adapter，但 rollback boundary/drop projection、workspace restore intent/cleanup、restore projection rebuild、parked terminal cleanup/terminalize/Goal record 已各归准确私有行为；portable snapshot 的 Run/parent index、cycle detection 与 resolved-root lineage 由单一 `snapshotRunTree` 拥有，不存在只转发 `Transactor` 的伪抽象；
- Application consumer-owned ports 均按用例拆分；`Coordinator` 只用于确实协调多个 use-case collaborators 的 package aggregate，不存在 package-name + exported-type 口吃或跨用例胖 executor interface；
- Application 的 post-commit read-again 信号统一归 `application/invalidation`：`Notice` 只携 resource/identity，不复制查询值，Goals/Plans/Runs/Sessions、Bootstrap Relay 与 Delivery projection 全链路只使用 `Invalidations` 这一术语。process-local Session/working-tree gate 归准确的 `application/sessionadmission`；单消费者 `approvals/approvaltest` 已回收到其黑盒测试，不再伪装成共享生产 package；
- Delivery `server` 只做 wire validation/projection，`dispatch` 只做 JSON-RPC registry/routing/idempotency，二者对 concrete Adapter/Infra/Bootstrap/Agent Framework import 为零，现名准确且不合包；
- `internal/component`、temporary exception、空目录、历史 fixture 与一层纯转发 wrapper 均为零；最终 DAG、命名和 shared-capability purity 由永久 architecture tests 守卫。

### 9.3 Delivery/Bootstrap

- Delivery 的 `protocol`、`dispatch`、`server`、`transport` 四层分别拥有 wire vocabulary、method routing、Application projection 和 envelope I/O；StreamFrame 使用完整的 `Notification` 语义，HTTP transport 只消费预编码 frame；无消费者且不可能成为公共客户端 API 的 internal in-process prototype 已删除；
- `server` 的 Session import/export、Run/Plan event presentation、workspace subscription lifecycle 分别归 `session_transfer.go`、`presenter_run_event.go`、`workspace_stream.go`；workspace hub 的 notification coalescing、subscription admission 与 queue drain 仍在同一并发 owner 内，没有拆出第二状态机；
- Bootstrap 仍是唯一 composition root；conversation、model、MCP、tool 的 composition-time environment 和 post-Run maintenance 各由 focused builder 组装，`Stack` 只暴露 Application entrypoints/notification sources，`Host` 单独拥有 shutdown graph；
- `hostLifetime` 以 `goalDriver`、`mcpCoordinator`、`codebaseCoordinator`、`runCoordinator`、`executor`、`runEffectTasks`、`toolResources`、`hostResources` 表达真实关闭职责；旧 `goals`/`mcp`/`execution`/`effectsTasks`/`resources` 一类含混字段已删除；
- `engine_wiring.go`、`embedding_env.go`、`utility_role.go`、`execution_support.go`、`toolenv.go`、`mcp_env.go`、`notification.go`、`reply.go`、`presenter.go`、`sessionio.go`、`builders.go` 等旧职责失真路径由 architecture guard 永久禁止回流。
- Tool `Call` 装饰只由 `call_decorator.go` 的 `decorateCall`/`callDecorator` 表达；HTTP transport 的请求级 tracing、response metrics 与 panic containment 只由 `request_instrumentation.go` 的 `instrumentRequests` 表达。JSON-RPC 路由消费面统一使用 `messageDispatcher.Dispatch`/`dispatch.Result`，注册项的完整解码—调用—编码行为统一称为 `pipeline`；这些名称分别对应 Tool capability、HTTP instrumentation 与 RPC dispatch，不再共享含混的 `decorate`、`observability`、`messageHandler` 或 `HandleResult`。
- 六份权威 Runtime 文档统一使用 `Interaction` 表达 Framework Strategy，只有存在真实对照维度时才使用额外限定词；精确文档守卫禁止把 Interaction 与含混的原生/native 限定组合，不误伤 provider wire、OS path 等确有区分对象的 native 语义。

## 10. 删除清单

以下条目已在对应阶段清零，并由永久门禁防止回流：

- 临时 Agent module path imports；
- root chat GOAP single-action wrapper；
- `TurnProcess` 和旧 Turn lifecycle vocabulary；
- `adapter/agentexec/turn` 中第二 controller/pump/registry；
- old suspension private JSON decoder；
- old process-tree private codec；
- old `toolloop` promotion/deferred glue；
- old AgentTool delegation wrapper；
- Application/Domain 中 `execution` package path；
- Product-facing `ProcessID`/`TurnID` terminology；
- Delivery 对 concrete agentexec/persistence/infra 的直接控制；
- `component` umbrella（P9 已删除并由永久 guard 禁止）；
- temporary architecture exceptions；
- alias、dual wire、compat decoder、空目录和历史 TODO。

## 11. 验收覆盖矩阵

| 场景 | Domain | Application | Agent Framework real vertical | Persistence | Delivery |
|---|---:|---:|---:|---:|---:|
| start/terminal | 必须 | 必须 | 必须 | 必须 | 必须 |
| stream + Delta drop | — | 必须 | 必须 | Transcript 必须 | 必须 |
| ToolCall/result/usage | accounting/tool | 必须 | 必须 | 必须 | 必须 |
| waiting/checkpoint | run/interrupt | 必须 | 必须 | 必须 | 必须 |
| restore/resume | run/interrupt | 必须 | 必须 | 必须 | 必须 |
| approval allow/deny/remember | approval/tool | 必须 | Tool input | 必须 | 必须 |
| question choice/text/cancel | interrupt/transcript | 必须 | Tool input | 必须 | 必须 |
| steer | run command | 必须 | 必须 | 按产品事实 | 必须 |
| Delegate child | run/lineage | 必须 | 必须 | 必须 | 必须 |
| subtree cancel | run/interrupt | 必须 | scoped Effect settlement | 必须 | 必须 |
| cancel/deadline/lost | run outcome/tool failure | 必须 | tree-wide Effect settlement | 必须 | 必须 |
| rollback/fork | conversation/transcript | 必须 | parked cleanup | 必须 | 必须 |
| recovery after restart | run/session | 必须 | restore probe | 必须 | projection |

## 12. 当前结论

- Runtime 产品领域、协议、持久化和工具能力大部分保留；
- 执行 Framework integration 是主要 Rewrite 区；
- P8 已将 Agent Framework vertical 原子切为唯一生产 owner；root、managed Delegate child、waiting subtree、termination、unknown 与 recovery 均由真实 Bootstrap consumer 验证；
- Agent Framework Baseline 20 已提供 P4–P7 公共合同与通用 RawMessage/byte JSON Schema 修订，并以 canonical module pseudo-version 被 Runtime standalone 直接消费。Framework 没有引入任何 Runtime 产品、持久化或 transaction 抽象；
- Runtime 对原框架 source/test/module dependency 与临时 module path 已归零；唯一 `agent` Framework 仍只拥有中性合同，产品 Run、Store、transaction、WorkingContext composition 与 recovery policy 均留在 Runtime；
- P11 删除迁移期 execution/port 快照文档；P12 继续删除已完成的架构清洗台账。当前架构、端口与工具接线分别只有 `ARCHITECTURE.md`、真实 consumer code/GoDoc 和 `TOOL_SYSTEM.md` 一个 owner，历史实施事实归 Git，不保留第二套错误现状；
- P12 全量静态审计捕获的格式漂移与嵌入字段冗余已在各自源码 owner 治本清除；Runtime 与 Agent Framework 的 tracked production TODO/FIXME/HACK、旧 Framework 路径、旧 replay scope、空文件、空目录和内部死代码均为零。
- P12 最终行为矩阵证明 Runtime root/child Interaction、authoritative model/tool、waiting、restore、resume、steer、unknown、recovery、rollback 与全部外环能力仍自洽；SQLite 当前 epoch 69 由 baseline consistency test 永久守卫。`interactioninput` 是 pending continuation/prompt/resolution 的唯一 ACL 与 codec owner，原单消费者子包和第二 decoder 已删除；三个 Runtime strict-codec fuzz owner与 Agent Framework 十三个 wire/state fuzz owner均全绿。
- P14 完成 Runtime 内部职责与全层级命名精修：package、目录、文件、类型、方法、函数、常量、字段和局部变量按 owner/行为逐层反证；淘汰词汇与失真文件路径由精确 package guard 防回流。最终 standalone、race、生成物、文档链接、死代码、空残留、复杂度与完整 lint 门禁全绿，未改变 Runtime/Agent Framework 边界、协议或持久化合同。
- P15-02 再次反证 Domain/Application 后，将 system-invariant 的说明性 catalog 从生产 Application graph 移至 `cmd/contractgen`，而真实跨聚合不变量继续由对应 write-set 和 integration fixture 独占；无消费者导出链已删除，结果值、水位与 child/executor 语言按真实语义统一。Domain 仍只拥有聚合、值与纯策略合同，Application 仍独占 I/O ports；本批没有新增 Framework concrete type、协议 wire、SQLite shape 或消费者兼容路径。
- P15-03 反证 Adapter/Infra/Delivery/Bootstrap 后，executor checkpoint 在 `agentexec` 之外重新成为纯 opaque bytes：Bootstrap 与 runsegment 不再复制或解释 TreeSnapshot shape，持久化测试只证明 checkpoint envelope 的原子保存、替换和删除。非 Framework 层的 member/request/child-Run 词汇、LLM catalog/profile 命名、execution scope 文件职责与 SQLite epoch 事实已同步；新增 architecture guard 禁止外环重新拼装 Framework tree wire。本批没有改变协议、SQLite shape 或 Agent Framework 合同。
- P16-01 已完成完整 Run aggregate 纵切：`domain/run.Run` 是 lifecycle、frozen admission facts、cumulative metrics 和 terminal facts 的唯一 mutation owner；`domain/transcript` 不再定义 Run 或跨 Run/Tool 的通用 Problem。SQLite 只重放并验证 aggregate transition 后执行 CAS，不再以裸 State 形成第二状态机。Application 的 Run tree/Pending/checkpoint/Goal/Conversation/Transcript 原子 write-set 仍留在原 owner，Agent Framework concrete import island、Protocol wire、SQLite epoch 和消费者实现均未改变。
- P16-03 已完成 Plan aggregate 纵切：`domain/plan.State` 是 Steps、replacement、revision 与 updated time 的唯一 mutation owner；`application/plans` 读取当前状态并形成 immutable CAS replacement，Tool Adapter 只调用该用例，SQLite 保存上层已决定的精确状态。Session fork/rollback/restore 继续由 Application 组织跨聚合原子 write-set，restore 不再删除 Plan row 后重置 revision；Protocol、Artifact、SQLite shape 与 Agent Framework 边界均未改变。
- P16-04 已完成 Session aggregate 纵切：`domain/session.Session` 以私有字段独占 fresh/scheduled construction、edit、generated-title arbitration、fork、restore workspace installation、revision 与 time；Application 继续拥有 workspace admission、identity/clock、Run opening、mutation claim 和跨 aggregate write-set。SQLite SessionStore 只保存 Domain/Application 已决定的 exact initial/replacement，旧 Create/Ensure/Patch/Fork/Restore/field-setter owner 已删除；portable restore 保持目标 revision 单调，Protocol、Artifact、SQLite shape/epoch、Agent Framework 和消费者均未改变。
- P16-05 已逐一冻结 21 个 Domain package 的独立词汇、变化原因、消费者和 import DAG 理由：没有按体量机械合包，也没有保留无 owner 的微包。Live workspace projection 与 CWD admission sentinel 收回 Workspace Application，Schedule availability 收回 Schedules Application；Domain 文件名/GoDoc、Interrupt 零值和 codebase ranking 边界同步精修。新增永久 guard 要求每个 Domain package 的目录名、package 名、边界说明和直接测试共同存在；Protocol、Artifact、SQLite、Agent Framework 和消费者合同均未改变。
- P16 已完成：Run、Transcript Item、Plan、Session 的单 aggregate 行为均只有一个 Domain mutation owner，Application 继续独占跨 aggregate 和外部 lifecycle。最终 standalone/build/vet/test/race/lint/staticcheck/deadcode/generator、三个 strict codec fuzz、architecture、文档与 hygiene 门禁全绿；没有兼容路径、协议/Artifact/SQLite/Agent Framework 变化或消费者接线项。
- P17 已完成：package 总数从 115 收敛到 100；shared capability、Toolset、Application、Adapter、Infra 与 Delivery 的伪边界和错层完成当期收敛；in-process transport 因零生产消费者且不可越过 Go `internal` 边界而删除。全 internal package GoDoc/目录名/零行为 umbrella 永久门禁、全量 race/lint/staticcheck/deadcode/generator 与空残留扫描共同通过。
- P18 进一步以行为 owner 而非 import 数量反证根级 package：pagination 的全部策略调用属于 Application read，taskgroup 的启动者属于 Application coordinator，Bootstrap/Delivery 的引用不产生共同所有权；opaque-token 的两个 payload owner同属 Application continuation。三者已移动到 `application`，旧根路径永久禁止；真正跨环的根级 pure capability 只剩 completion、HTTP origin 与 idempotency。
- P19-02 已建立唯一公共 `protocol` owner：外部 Go consumer 与 HTTP 共用相同 values、strict validation、version 和 client-visible problem identity；服务端 method interface/context/enrichment、JSON-RPC numeric code、reflection shape walker、enum/sample artifact catalog 分别收回私有 `operation`、`dispatch`、`contractshape` 与 `contractcatalog`。旧 internal protocol path 已物理删除，无 alias、shim、双份 DTO 或 generator internals 暴露。
- P19-03 已建立唯一私有 typed operation pipeline：method catalog、strict request/result/event validation、capability gate、safe problem projection、durable idempotency 与 run-stream reattach 由 `delivery/operation` 独占；HTTP dispatch 只适配 JSON-RPC envelope/numeric code/frame。幂等 replay payload 具有显式版本且未知形状 fail closed；旧 dispatch registry、capability、idempotency owner 与 catalog forwarding 均已删除。
- P19-04 已建立唯一私有 Runtime instance owner：canonical data-directory advisory lease 早于 SQLite/recovery，`bootstrap.Instance` 统一拥有 Host、operation endpoint、Server projection、scheduler 与 reverse-order retryable Close。HTTP cmd 不再组装 Server/recovery/worker，HTTP transport 只消费 instance-owned Endpoint；同路径、symlink alias 和第二进程争锁均被拒绝，未完成 Close 不释放 lease。
- P19-05 已建立公共 concrete `embedded.Runtime`：全量 operation 以类型化 Go 方法直接消费唯一 operation pipeline，公开面只含 `protocol` values、精确 options、`iter.Seq2` streams 与完整 Open/Close lifecycle。外部 module 无需且不能依赖 Runtime internal；AST+reflection 门禁保证 operation 覆盖和签名不漂移，真实集成保证 protocol、idempotency、notification、stream close、directory lease 与 reopen 行为。
- P19-06 已冻结完整公共 Go 合同：生成的 `contract/go-api.json` 来自真实 Go type information，架构门禁同时守住唯一 public package 集合、visible import、operation/method parity、external-module compilation 与 artifact digest。稳定失败由 protocol sentinel + `ProblemError` 提供准确的 `errors.Is`/`errors.As` 语义；README/consumer handoff、准确 capability 文件和最终质量矩阵均已收口，P19 不留下 consumer shim 或 transport duplicate。
- P21 已以真实 DeepSeek、SQLite、mock 与 PTY 四类证据闭环普通/连续/并发 Tool、授权五种选择、问题 HITL、resume、取消、重连、重复/乱序和 crash/restart。Runtime adapter 只把产品取消作用域翻译为对应 Agent Effect context，未把 Run、授权、Store 或 transaction 职责泄露进 Framework；终态没有 checkpoint、open interrupt、unknown Effect 或未完成 invocation。
- P22 已在 WorkingContext 唯一组合 owner 内为 base/Knowledge/Memory/Agent document/Plan/hook 建立私有 typed provenance；预算后的实际来源与 instruction/data purpose 随 Message/Part metadata 进入 opaque checkpoint，文本、Protocol、Artifact、SQLite 与 Agent Framework 合同均未变化。
- P23 将 WorkingContext provenance 从“调用点同步填充的数据”收敛为行为所有权：source kind 唯一派生并验证 purpose，预算后的 Memory/Agent document fragment 原子拥有 text+sources，hook result 自己应用拒绝/注入，`WorkingContextComposer` 统一拥有 system/Plan/recall/hook。公开 DTO、Protocol、Artifact、SQLite 与 Agent Framework 合同均未变化。
- P25 已关闭 P23 暴露的发布顺序缺口：Agent Baseline 20 canonical 发布后 Runtime 一次性前移真实 module version，原先两条冷恢复 consumer 回归转绿；Runtime 没有复制 Schema、清洗 provider metadata、增加 `replace` 或跳过回归。
- P24 以真实 Desktop、Runtime 与隔离 SQLite 将 read model、query invalidation、Git observation、Segment activation/cancel 和 boot recovery 重新对账：终态无 started invocation、非终态 Run、open interrupt 或 checkpoint，崩溃恢复的 Run/Goal/invocation 使用同一终结时间；Agent production graph 仍不 import Runtime，Framework concrete type 仍只存在于 `adapter/agentexec`。
- P25 第二轮反证进一步把前端 live/durable projection、workspace retarget/reconnect、Run/Item/Plan late-event 单调性与 JSON-RPC envelope 歧义收回唯一 owner：真实 Runtime 对合法请求返回 200、notification 返回 204，对 duplicate/unknown、空 method、显式 null/numeric id、client response 与 mixed envelope 返回 400；真实 Desktop 在 active Run `SIGKILL` 后把 Run/model invocation 收口为 lost/unknown，无刷新重连后继续完成新 Run。真实 ask_user、两次 Plan 替换、Goal completed/blocked 与 blocked 后普通 Run 均完成，终态无 open interrupt/checkpoint/active Run。Agent Baseline 20 已发布，Runtime 绑定远端真实 pseudo-version 后 standalone tidy-diff/build/vet/test/race/staticcheck/lint 全绿；原 Baseline 18 下失败的两条冷恢复 consumer 回归转绿，且没有引入 `replace`、Runtime sanitizer 或 Framework 抽象泄露。
- P26 将 Tool 可见 lifecycle 与真实 executor interval 分离：Reducer 是 exact execution duration 的唯一计算 owner，Transcript terminal fact 持有 optional duration，SQLite 只编码、Delivery 只投影，恢复无法证明时保持 unknown。Protocol `2026-08-12`、Artifact v17 与 Desktop generated consumer 已同步；真实 HITL lifecycle 31.160s、execution 2.016s、UI `2s`，Goal/Plan/普通 Run 后续矩阵与全部质量门禁全绿，Agent Framework 边界未变化。
- P27 以真实依赖图收缩发布信任面：Frontend clean install 解析到修复后的 Mermaid/DOMPurify/NanoID；Runtime 的 Ollama chat/embedding 在 Infra 内复用既有 OpenAI-compatible protocol，移除完整 Ollama 服务端 module。`npm audit` 与 Runtime 可达 `govulncheck` 均为零，真实 HITL/Tool/Goal/Plan 崩溃恢复、双 Session 隔离、约 92.6 万次 codec fuzz 和终态数据库不变量全绿；Agent/Application/Domain/Delivery 公共合同未变化。
- P28 将同一依赖边界修复推进到独立 `models/ollama` owner：provider module 以私有窄 wire 保留原生 `/api/chat`、`/api/embed`、NDJSON stream、Core extensions 与 HTTP failure 语义，同时移除完整 daemon repository 及其 server/auth/ordered-map 依赖。该模块 `govulncheck` 从 8 条可达路径降为零，公开构造器与 Runtime/Agent 合同不变化。
- P29 关闭 CLI standalone consumer 的发布图缺口：CLI 直接绑定已推送 commit `420f627f131a` 对应 Runtime pseudo-version，并随其消费 Agent Baseline 20；旧 Runtime→`models/ollama`→daemon 依赖链从 standalone graph 删除。CLI 全量 normal/race/static/lint/build/vet 与 `govulncheck` 全绿，未使用 workspace `replace` 或夹带 CLI 功能改动。
- P33 关闭 Schedule 默认工作区更新的协议表达缺口：Protocol/Delivery 精确拥有保持、设置、回到默认三态及互斥校验，Desktop handwritten SDK 直接消费生成 `UpdateScheduleRequest`。Domain/Application 仍只拥有 Schedule/CWD 行为，Agent Framework 未接收任何 Runtime workspace、wire 或 UI 抽象；真实 UI/SQLite 证明失败路径已持久化收口。
- P34 将 HITL 的最终显示事实收回 Runtime：Question Transcript 在 accepted resume claim 的同一事务内不可变补充答案，Artifact v18/Protocol/SQLite codec/Delivery/Desktop 只消费该事实；取消保持未回答。Goal 在 fresh Start 冻结协商能力，Resume 验证调用方覆盖，自治 Run 与 Goal 内 `create_goal` 继承它；执行上下文 carrier 留在 Runtime adapter，不修改 Agent Framework。Desktop 的未知 raw HTML 在 Markdown AST 边界按字面量展示，不把 sanitizer 或 UI 语义下沉 Runtime。
- P34 的真实 HTTP 反证进一步关闭 Goal resume 与 parked Run 的 durable admission 缺口：`application/runs` 的 startable observation 同时等待本地 gate 和权威 non-terminal Run，committed lifecycle change 只负责唤醒，状态仍由 durable projection 重读；Goal 不再把同一 waiting Run 误判为新 Run start failure。
- P35 将这个 wake-only 原则补齐到完整生命周期：Runtime Session change generation 只由活跃 Application waiter 持有，多观察者共享当代通知并以幂等 disposer 释放，无 waiter 时不保存 Session 条目；它不是 durable state/cache，也不承担 reservation。
- P35 同时关闭 Desktop 的 snapshot→subscribe TOCTOU 与 accepted boundary 混淆：initial recovery/replay reattach 遇到 terminal/waiting/stale 时经 Agent application port 重读 durable projection；只有 Run ack 前拒绝进入 command error/HITL rollback，ack 后 stream failure 不否定已提交命令。Runtime DTO、Store、transaction 与 Agent Framework concrete type 均未进入 Agent context。
- P36 关闭 Desktop 旁路 file watch 的静默失联：Session workspace adapter 只把 `session_not_found` 解释为权威 unavailable，Workspace event application 对其他瞬时失败执行同 identity 可取消退避；retarget/dispose 使旧 generation 失效，global topics 与 file watch 继续由唯一 `runtime.subscribe` consumer 拥有。该恢复策略没有进入 Agent context，Runtime/Protocol/Artifact/SQLite shape 均未变化。
- P37 纠正 Goal 完成态不可观察的错误协议假设：Domain/Application 继续分别拥有 terminal fact 与最终 settlement，Delivery 只将该窗口投影为 `completing`；生成合同与 Desktop 自有 read model 同步，banner 保留目标但不提供 stop/resume，最终清除仍经 invalidation 收敛。真实 HTTP 回归在 `report_goal_outcome(completed)` 后卡住 owning Run，证明 `goals.changed → goals.get(completing) → null` 全链闭合；Agent Framework、Artifact 与 SQLite 均未变化。
- P38 关闭 Desktop 失效 Session 深链接的假故障：Runtime Gateway adapter 识别协议 `session_not_found` 并向 Agent application port 返回 absent snapshot，projection refresh/history action 只消费中性缺失语义；transport/protocol 等运行失败仍然 reject 并进入原 recovery error owner。前端不 import Runtime DTO，Agent Framework 和 Runtime 合同未变化。
- P39 关闭 Desktop 多客户端 Session 删除后的幽灵导航：启动记忆恢复仍为一次性动作，但每次权威 Session 列表读都对账 active/open 集合；delete 在 commit 前不乐观改写身份集合，Adapter 把已缺失视为目标达成；rename/favorite 失败的快照回滚后会重读 Runtime，不会复活并发删除的行。Runtime/Agent Framework 零变更。
- P40 关闭 Desktop 深链/history active Session 未进入 held-open lifecycle 的断裂：mounted driver 在 material view 前建立 open/last-session memory，权威 selection model 保证存活 active 不被 stale open cleanup 清除；close/reconcile 同步纠正 cold-start seed，relocate 的条件写失败重读 Session truth。改动止于 Desktop Adapter/Application，Runtime 与 Agent Framework 零变更。
- P41 关闭 Desktop 双客户端 HITL resume 的延迟拒绝反转：Application 的 staged batch 随 standing projection 淘汰而幂等退休，并将“opening 已被更新视图取代”的中性事实交给 Adapter 抑制过时 command error；Adapter 不识别 HITL wire error，延迟本地 ack 也不能覆盖已物化的远端结果。Goal lifecycle 命令无论成功或结算不明都回读自身 query，回读失败不替换原命令错误。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P42 关闭 Desktop 未消费 Runtime command replay 合同的全局缺口：RPC SDK 对全部生成 command policy 统一完成有界同-key settlement recovery，流式 opening 的每次 attempt 使用并清理自己的 event stream；Agent Application 不接触 `MutationPromise`、transport 或 idempotency。Session create 的 attempt deadline 归 Adapter，provisional draft owner 跨冷启动持久化，而“刚在本进程创建、可跳过首次 durable read”的 freshness 独立保持 ephemeral；冷启动从权威 snapshot 判定毕业，导航只让 fresh create 补充尚未出现的列表身份。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P43 关闭 Desktop Run opening 永久 pending 与 cancel 迟到响应反转：RPC Agent Adapter 对 replayable opening 建立两次有界 delivery attempt，同 key 但各自 fresh signal，ack 后只解除握手 deadline、不解除长流的 session ownership；传输失败投影为稳定产品问题而不进入 Application。Cancel controller 用发起 revision 条件提交快照，失败后只询问 Application 的权威 terminal 事实，另一客户端先结束时不识别 wire error 也能收敛。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P44 关闭 Desktop Session 历史重写与条件写收敛缺口：rollback 不再绕过 mounted Session 的 stream/snapshot 串行 owner，重复动作不会双重截断或执行两次 follow-up；Runtime 返回的 dropped user input 在 Adapter 转成 AgentInput 后驱动 edit/regenerate。第一轮没有 checkpoint 时 files/both 不再被省略字段误投影成 history-only 全量删除。rename/favorite/relocate 共用 Session 级 revision settlement，失败只回滚自己仍拥有的乐观字段。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P45 关闭 Desktop Goal/Plan 的能力不可达、命令竞态与抽象泄露：blocked/paused Goal 可从 composer 原位替换，预算耗尽不再显示 Runtime 必然拒绝的 Resume；Goal command 按 Session 串行、有界同-key settlement，并以 typed response 单调提交后重读，provider 与 wire mapping 均回归 Goal Adapter。Plan shared snapshot/event 在 Agent Adapter 转为中性 Plan domain，bootstrap fold composition 不再伪装成公共产品 surface；旁路 `goals.changed` 只触发点名 Session。真实 Runtime 证明 Goal replacement、budget block、Plan live/cold projection 与 SQLite 权威状态一致。Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P46 关闭 Desktop Agent 四条剩余 Runtime DTO 泄露路径：SDK 中性 facts 承载 live event、durable Run/Item/Interrupt 和 cancel projection，wire 不变量只在 Agent Adapter 校验，pending work provider 也回归 Agent bootstrap；静态发布边界门禁防止 Application/Domain/public 与 SDK event contract 再次导入 RPC。真实冷启动进一步发现 Composer catalog 默认模型先于 active Session summary 落地并形成错误 override，现按显式偏好、durable Session 模型、catalog fallback 的所有权顺序解析并等待未决 summary。真实两步 Plan、冷重载与第二次 Run 均保持 `deepseek-v4-pro`；Runtime、Protocol、SQLite 与 Agent Framework 零变更。
- P47 关闭 Workspace subscription opening 的取消传播缺口：retarget/dispose 的 lifecycle signal 从 Workspace Application 经 Runtime Adapter 与 SDK 到达 `workspaces.resolve`，旧 watch-root resolution 不再阻塞唯一重连循环。Runtime Delivery 同时将 `watchIds` 与非 `files.changed` topics 的跨字段矛盾视为不可猜测输入并扩大到完整 subscription resync；watch、wire 与 HTTP 取消语义没有进入 Agent 或 Runtime Domain/Application。真实 HITL、Plan、Goal、旁路 Session、Git 文件事件与 Runtime 重启恢复均收敛。
- P48 关闭 Desktop “丢弃旧结果但不取消底层身份读取”与 Schedule mutation 返回事实未消费的缺口：Workspace Application 的每代 signal 经 Adapter/SDK 到达 `sessions.get`，retarget/dispose 不再积累孤儿请求；`schedules.runNow` 的 Session/Run identity 由 Schedule 用例保留，并经 Agent 公开会话动作进入 Agent 自有 durable recovery，Schedule 条件更新则先提交后端返回的新 revision 再重验。Runtime DTO/transport 没有进入 Agent 内层，Runtime、Protocol、SQLite shape 与 Agent Framework 均未变化。
- P49 关闭 Desktop 非 void mutation 返回事实未消费及连续写乱序覆盖的系统性缺口：Provider、Approval、MCP、Agent Memory、Codebase 各自的 Adapter 将 Runtime response 投影为中性产品资源，所属 Application 按真实冲突域串行并先提交权威 fact 再失效重验；通用 serial queue 对失败后续跑与 keyed independence 有直接回归。MCP data provider/wire projection 回到 MCP context Adapter，defaults 和 Agent 内层均不接触其 wire；Provider `requiresBaseUrl` 驱动真实必填校验，UI 用 saved resource 重建规范化草稿。Runtime、Protocol、SQLite shape 与 Agent Framework 均未变化。
- P50 关闭 Knowledge 丢更新、空文件不可创建及 Knowledge/Hook 多窗口静默陈旧：Knowledge Domain 只持有 scope/revision conflict，Application 拥有条件更新与 committed invalidation，Infra 独占内容 revision 和同目录原子替换，Delivery 独占 Protocol problem/topic 投影；Desktop Workspace/Hook Adapter 将 wire 转为中性模型，干净编辑器跟随事件、脏草稿保留且冲突后显式 rebase，Hook intent 按 project 串行并有 UI latch。Agent/Agent Framework 未导入 Runtime、Knowledge revision、Hook topic 或前端状态，SQLite/Artifact/Framework shape 不变。
- P51 将 Knowledge containment、read/revision/atomic replacement 收敛到唯一 physical identity，并以中性 advisory lease 保证跨进程只有一个 stale writer 提交；权限继承、file/parent sync 与 crash staging recovery 均留在 Infra。Application/Delivery 只投影既有 path policy/problem，Agent/Agent Framework、SQLite、Artifact shape 不变。
- P52 让外部进程对 Knowledge/Hooks 的 create、write、atomic rename、remove 与 symlink target 更新进入既有 committed invalidation：Infra 只观察精确路径指纹，Workspace Adapter 独占文件布局，Application 只接收语义资源，Delivery 只投影既有 topic。API write 在发布前接受 exact path 基线以去除 filesystem 回声；Agent/Framework、Protocol、SQLite、Artifact shape 不变。
- P53 关闭 Desktop Goal lifecycle mutation snapshot 的第二事实源：Application 不再用 `updatedAt` 猜测延迟响应的新旧，也不直接写 standing cache；command port 只返回中性 Session receipt，完整 wire 响应由 Goal Adapter 消费，长期状态唯一来自 `goals.get` data provider。同 Session 串行与成功/失败后的权威回读仍由 Goal Application 拥有，Agent/Agent Framework、Runtime/Protocol、SQLite/Artifact shape 不变。
- P54 关闭 Checkpoint source index 与 shadow backstop ignore 的 ownership 冲突：`infra/checkpoint` 先以 `ls-files --exclude-standard` 唯一选择 source-tracked / untracked-unignored 路径，再应用类型与大小策略，最后只对精确候选 force-add；合法跟踪的 build 输入不会被 shadow exclude 二次否决，同目录 ignored generated sibling 仍不归 checkpoint。Run/Goal maintenance 继续显式报告失败，Agent/Agent Framework、Protocol、SQLite、Artifact shape 不变。
- P55 关闭 Git subprocess 继承宿主仓库控制面的缺口：中立 `infra/gitprocess` 是 Runtime 内唯一 Git OS-process owner，剥离父进程全部 `GIT_*` 后再安装命令显式参数；checkpoint source/shadow index、workspace VCS read 与 watcher 不再被 foreign repo/index/object/config/pathspec 重定向。Watcher 把 Git metadata directory 收敛为 physical identity，architecture guard 禁止 owner 外直接启动 Git。Application/Delivery/Agent/Framework、Protocol、SQLite、Artifact shape 不变。
- P56 关闭 HTTP sidecar 未纳入唯一合同和 Desktop 产品消费的缺口：Runtime HTTP Delivery endpoint registry 同源驱动 handler/auth/info/contractgen，生成 manifest/TS/validator/reference 精确覆盖 info/liveness/readiness；Desktop Runtime context 以中性 service inspection port 拥有 timeout/coalescing/dispose/retry，Settings 不接触 HTTP DTO。Workspace event Adapter 同时把客户端可折叠 topic 与 discovery 声明取交集，旧 Runtime 不再因新增 topic 拒绝全部旁路失效流。Agent/Agent Framework 未导入 sidecar、HTTP、service status、topic negotiation 或 UI 状态，Runtime Application/Domain、SQLite、Artifact shape 不变。
