# Lyra Runtime 能力迁移台账

> 状态：P10 当前事实，随每个实施批次更新
>
> 基线日期：2026-08-09

本文记录当前能力事实、目标 owner、迁移 verdict、实施阶段和验收证据。它不重复目标架构和 ADR。代码变化后必须在同一批更新对应条目；不能用“计划保留”冒充“已经迁移”。

## 1. Verdict

| Verdict | 含义 |
|---|---|
| `Retain` | 所有权和抽象正确，保留实现，只做必要接线/命名同步 |
| `Refactor` | 产品语义保留，但 API、package、职责或依赖需要治本调整 |
| `Rewrite` | 能力保留，但当前实现建立在旧执行模型上，按新合同从零实现 |
| `Remove` | 能力重复、owner 错误或已经由 Agent Framework 原生拥有，完成阶段必须删除 |
| `Defer` | 不是当前服务端重构前置条件，有真实需求后单独设计 |

## 2. 当前基线事实

### 2.1 规模与依赖

- Runtime 源码、测试、`go.mod` 与 `go.sum` 对旧 `github.com/Tangerg/lynx/agent` 的依赖均为零；architecture guard 将其作为永久禁止边，而非迁移数量台账；
- `go.mod` 精确声明包含 Baseline 14 的 Agent Framework commit；`GOWORK=off` 的 tidy/build/vet/test 已证明 Runtime 不依赖 workspace 隐式替换；
- Agent Framework production import 只允许位于 `adapter/agentexec`；Domain、Application、Infra、Delivery、Bootstrap 与通用 Toolset 对 Agent Framework concrete types 为零；
- P2 已删除 `domain/execution` 及全部 forwarding/alias path；Domain 生产代码与测试对 Application/Adapter/Infra/Delivery/Bootstrap 零 import，context-based I/O port 为零；
- Run、Accounting、Conversation、Transcript、Interrupt、ToolResult 已成为准确顶层 bounded-context package；executor checkpoint/ref、pending continuation 与 workspace mutation 由 Application consumer 拥有；
- P3 已删除 Application 的 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖接口；root start/observe/release、Session reads/termination 与 Run projection write-sets 均由真实 consumer-owned ports 表达；
- Application executor tree identity 已统一为 `ExecutorMember`/`MemberID`；Framework `ProcessID` 只存在于 `adapter/agentexec` 内部映射，SQLite technical shape 使用 `root_member_id`/`memberId`；
- 同一 native Interaction tree 已在生产 Bootstrap 接通 conclusive child start、durable Delegate child Run attribution、nested/sibling child reconciliation，以及 one-shot prepared waiting-subtree cancellation；
- SQLite 当前唯一 shape 为 epoch 62：model/tool invocation operational journal 与 Transcript semantic final 分离，interrupt row 具有 `open`/`resuming` answer-claim 状态；Run pump 是唯一 reducer/persistence writer；
- Agent Framework production dependency 只存在于执行防腐层；Runtime 其他 ring 对 Framework concrete type 为零。

### 2.2 当前架构基础

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run lifecycle | `application/runs` 拥有 start、pump、waiting、resume、cancel、terminal ordering；child opening reservation、conclusive start 与 waiting-subtree transaction ordering 已在生产冻结 | Retain | P3–P8 已完成 |
| Session lifecycle | 独立 application/domain，拥有 workspace/admission 产品语义 | Retain | P2/P8 |
| Domain framework isolation | P2 已删除十个 context-based Domain I/O port，生产与测试均由机器守卫禁止向外依赖 | Retain + strengthen | P2 已完成；例外为零 |
| Agent anti-corruption | Agent Framework native tree、model/tool Effect、waiting/restore/steer、Delegate child 与 prepared subtree change 均由 `adapter/agentexec` 生产 owner 独占 | Retain | P8 已接管生产；旧 Framework lifecycle 已删除 |
| Delivery separation | target DAG 禁止 Delivery import 任意 concrete Adapter；protocol/dispatch/server/transport 已按准确职责收口 | Retain | P1/P9/P10 已完成 |
| Adapter/Infra direction | Adapter 单向使用 Infra；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零 | Retain | P9 已完成 |
| Shared capabilities | `component` umbrella 已删除；仅保留七个经多消费者或 codec boundary 证明的准确顶层 capability | Retain exact packages | P9 第一批已完成，永久 purity guard |
| Contract generation | `contract/` 的 manifest/OpenRPC/schema/TypeScript/Go validator 来自唯一 registry；canonical samples 同时经 round-trip 与 strict validator | Retain | P10 已完成 |
| SQLite exact epoch | epoch 62 单一 shape，无生产 migration chain | Retain | P6/P8 已完成 |

## 3. 产品领域能力

### 3.1 Run、Segment 与 execution package

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Run identity/state/outcome | `domain/run` | 保持 | Retain | P2 旧 path 归零，状态行为测试全绿 |
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
| Transcript Items/Runs | `domain/transcript` | 保持 | Retain | rollback/fork/item timing 保持权威 |
| Offloaded transcript content | `domain/toolresult` | 保持准确独立 capability | Retain | 无泛化 blob service |
| Knowledge/LYRA.md | `domain/knowledge` | 保持独立 | Retain | 用户编辑与 Agent state 无关 |
| WorkingContext | Application composer + Agent Framework Interaction private state | 保持 Host composition 与 executor state 分离 | Retain | fresh root 读取产品事实；restore 只用 opaque checkpoint，不从 Conversation 重算 |

### 3.3 Interrupt 与 approval

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Interrupt semantics | `domain/interrupt` | 保持 | Retain | Kind/Key/Resolution 纯领域值，无 I/O/executor |
| Pending continuation | `application/runs.Pending` + `adapter/persistence` mapping | 已保持 owner 并接入 Agent Framework public pending input | Retain | 一个 root tree 一个 pending hand-off；Infra 只见 technical record；claim 后普通读取不可见 |
| Approval domain | `domain/approval` | 保持产品策略 | Retain + remove I/O ports | 不进入 Agent Framework |
| Ask-user/approval tool input | Toolset product Interrupt + `agentexec/interactioninput` | public Interaction helper → product Interrupt | Retain | 不解析 private Framework payload；旧 adapter 已删除 |
| Answer/resolution | runs + native interaction input adapter | semantic Application command → WaitID-addressed Signal | P6 native bridge 已完成 | 无任意 Signal API；answer claim/segment opening/Signal 顺序受测试 |

### 3.4 Accounting

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Token/model-call accounting | `domain/accounting` + native authoritative model decorator | final/usage/pricing/Run progress 同一事务 | Refactor bridge | P5 已完成；Delta drop 不丢 final 或 usage |
| Pricing/USD | adapter/pricing + observer | Runtime adapter/domain value | Retain | Agent Framework Usage 无价格字段 |
| Framework resource usage | old Agent aggregate | Agent Framework Usage | Replace | 只翻译需要的中性事实 |
| Goal budget attribution | goals/runs | 保持 Application | Retain | child/root、resume、lost 归属准确 |

### 3.5 其他产品上下文

| 能力 | 当前 owner | Verdict | 迁移影响 |
|---|---|---|---|
| Goal | domain/application/toolset | Retain | executor port/name 更新；不下沉 Agent Framework |
| Plan | domain/application/toolset | Retain | 保持 Plan 唯一术语；不与 Goal/Todo 合并 |
| Schedule | domain/application/toolset | Retain | 通过 Run use case 启动，不直接调用 Agent Framework |
| Skill/Proposal | domain/application/adapter/toolset | Retain | deferred manifest 接线更新 |
| Agent memory | domain/application/toolset | Retain | 与 Conversation/Knowledge 分开 |
| Model/provider catalog | domain/application/adapters | Retain | 每 Run exact model binding 进入 deployment assembly |
| MCP/A2A/LSP | domain/application/infra/toolset | Retain | 保持技术能力，不进入 Agent Framework Kernel |
| Workspace/change/isolation | application/adapters/infra | Retain + recovery audit | 外部事实失效由 Host policy 处理 |
| Hooks | domain/application/adapter | Retain | P5 已在普通 Tool 边界触发；post-hook 是 observation，不覆写 settlement；child 边界留 P7 |
| Feedback/codebase index | domain/application | Retain | 与 execution migration 无直接耦合 |

## 4. Application 能力

### 4.1 Runs use cases

| 当前能力 | 当前形态 | Verdict | 目标证据 |
|---|---|---|---|
| Start admission | `ValidateRootStart` → `StageRoot` → durable opening → `BeginRoot` | P4 real consumer 已验证 | stage 不外呼 model/tool；commit 前失败只 Release |
| Event observation | `ExecutionObserver.Observe` | P4 real consumer 已验证 | 只流 Application-owned executor facts；final 来自 Result |
| Executor release | `ExecutionReleaser.Release` | Retain | 与产品 Cancel 分离；非 Waiting 终止恰好一次 |
| Product Cancel | durable control intent → `RunningRootCancellationRequester` → continued observation → release | Retain | request cancel 不提前切断 pump；确定终态后才释放 |
| Resume | `WaitingExecutionContinuer.StageContinuation/BeginContinuation` | Retain | exact BuildID/deployment/scope/capabilities restore；opening commit 后才 Signal |
| Steer | `RunningExecutionSteerer.SubmitSteer` | Retain | semantic steer 只在下一 model safe boundary 投影 |
| Child subtree cancel | `MemberID` + `WaitingSubtreeCancellationPreparer` | P7 real consumer 已重推 | Application 只看 member projection、resulting checkpoint 与一次性 Apply/Discard capability |
| Waiting subtree mutation | `WaitingSubtreeChange` 持有 execution ACL capability | P7 native bridge 已完成 | Application 不见 Agent Framework plan；source 冻结跨过 transaction；contextless Apply 只安装状态，final-boundary Continue 独立激活，失败时 exact restore/terminal 收口 |
| Run pump | executor event reducer | Retain | 不推进 Agent Framework internal state |
| Run journal | committed RunEvent | Retain | persist-before-publish |

P8 production cutover 已用真实 Bootstrap consumer 冻结 root stage/observe/begin、cancel request/release、waiting restore/answer/steer、child reservation/start outcome 与 waiting-subtree prepare/commit/apply-or-restore 的准确端口集合。

### 4.2 Cross-aggregate writes

| 写集合 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run admission | Session + Run/Transcript | Retain/refactor types | P3 |
| Waiting tree barrier | Run + Pending + checkpoint + Items | Retain semantics；P6 native 已验证 | P6 已完成 |
| Waiting child cancellation | Run tree + Pending + checkpoint + Items | P7 native Agent boundary 已完成，保留 Application transaction | P7 已完成 |
| Terminal tree | Runs + Pending + checkpoint + transcript + Goal | Retain | P8 已统一 write-set/first-wins/release ordering |
| Boot recovery | stored facts + opaque checkpoint probe | Retain | P8 已由 native Agent Framework production probe 接管 |
| Rollback/fork cleanup | Conversation + Transcript + active/waiting Run | Retain | P8 已保持 cleanup intent 与 live release 分相 |

## 5. Agent execution 能力

### 5.1 当前 `adapter/agentexec`

| 当前实现 | 作用 | Verdict | Agent Framework replacement |
|---|---|---|---|
| `InteractionExecutor` | 唯一生产 tree owner | Retain | per-Run Agent Framework Engine + native Interaction；P4–P8 完整纵切 |
| old Engine/GOAP/TurnProcess/turn controller | 已删除 | Remove complete | Agent Framework Process/Engine public lifecycle + Application Run pump |
| private process-tree/suspension codec | 已删除 | Remove complete | opaque public TreeSnapshot + Interaction pending-input ACL |
| child execution/configuration | 传播 model/hooks/budget | Rewrite | exact child Deployments + ProcessAdmitter |
| authoritative projection | model/tool/usage/final 同步 receipt | Retain | P5 native decorators；旧 observer 已删除 |
| tool decorator | approval/hooks/presentation | Retain semantics, Rewrite attribution | P5 ToolInvocation context + P6 interactive HITL 已完成 |
| usage accounting | subtree token/cost | Retain | cumulative accounting/pricing、child attribution 与 terminal tree 已收口 |
| deferred manifest glue | old toolloop promotion | Rewrite | P5 唯一 `toolset.Manifest` + Interaction DeferredTools/AdvertiseTools 已完成 |
| maintenance/restore checks | per-run native session + checkpoint probe | Retain | exact Deployment/BuildID/scope/capabilities 与 tree-wide boot owner 已收口 |

### 5.2 Agent Framework 已提供的合同

| Runtime 需要 | Agent Framework Baseline 14 | Runtime 责任 |
|---|---|---|
| root execution | Engine/Deployment/Interaction | 组装产品配置并翻译结果 |
| tree identity | ProcessID/Relation/root/parent/depth | 映射不透明 executor member/child Run |
| child admission | `ProcessAdmitter` + prospective identity + `ProcessStartOutcomeAcknowledger` conclusive started/aborted | durable Opening reservation、public Running 与 aborted cleanup |
| waiting | WaitID/Signal + Interaction pending helper | 产品 Interrupt 与事务 |
| restore | Snapshot/TreeSnapshot + exact resolver | Store、BuildID、Host metadata |
| steer | Interaction steer signal | 产品命令与内容 projection |
| subtree cancel | `PreparedWaitingSubtreeCancellation` 冻结 source，并提供 prospective result 与 contextless one-shot Apply/Discard | Application write-set 只见 projection + opaque checkpoint；commit 后 apply-or-exact-restore |
| model/tool attribution | ModelInvocation/ToolInvocation | Transcript/accounting/hooks/pricing |
| deferred tools | Tools/DeferredTools/AdvertiseTools | Tool catalog、发现语义和权限策略 |
| observation | Event/Delta | lifecycle projection/临时流；权威记录另行同步 |
| resource facts | Budget/Limits/Usage | 产品 limits、USD accounting 和 policy |
| unknown Effect | UnknownEffectIDs | live/recovery tree 由 wake + bounded public query 收口为 durable RunLost；不开放任意 resolution |
| prepared-step durability | PreparedStepAcknowledger 只有单 Process Snapshot | 初版不启用；只恢复 committed quiescent TreeSnapshot |

当前 root Interaction、ordinary Tool/model、waiting capture/restore、steer、durable Delegate child admission 与 waiting-subtree change 均没有已知 Agent Framework blocker。P7 真实 Runtime consumer 已推动并验证两个中性 Framework 合同：conclusive `ProcessStartOutcome` 与 one-shot `PreparedWaitingSubtreeCancellation`；Agent Framework 仍不感知 Run、Store、transaction 或产品恢复策略。单 Process prepared-step acknowledgment 本轮明确不启用，不算待实现 Runtime 能力。

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
| SQLite schema | epoch 62；保留 `root_member_id`/`memberId`，新增 open/resuming answer audit；tool invocation identity 按 Segment 隔离 | Retain pattern | 旧 epoch/列/codec 被拒绝，无 migration |
| executor checkpoint | Host metadata + Agent Framework public complete-tree snapshot | Retain opaque payload owner | Application/Store 完全 opaque；exact capabilities 纳入 expectation |
| checkpoint transaction | runsegment/persistence 组合 | Retain semantics | waiting facts 同事务 |
| BuildID | Host-owned | Retain | 不进入 Agent Framework deployment/snapshot |
| boot recovery | Application policy + agentexec native probe | Retain | P8 production recovery wiring 已收口 |
| unknown Effect live/recovery | native tree-wide reconciliation | Retain | wake 丢失由 public polling 收敛；RunLost 写失败重试且 release 不提前 |
| isolated recovery | fail closed | Retain | 不猜测重建 scratch world |
| terminal deletion | root aggregate transaction | Retain/converge | terminal 与 delete 原子 |
| waiting subtree persistence | Application transaction + execution-owned prepared change | P7 native boundary 已完成 | one-shot prepared change + opaque resulting TreeSnapshot + exact restore fallback |
| active-step crash durability | 旧路径能力不作为新合同 | Defer | 不启用 PreparedStepAcknowledger；无 committed quiescent TreeSnapshot 时 RunLost |

## 8. Delivery 与协议

| 能力 | 当前 owner | Verdict | 说明 |
|---|---|---|---|
| Protocol types/errors | `delivery/protocol` + `contract` | Retain | Protocol `2026-08-09`、Artifact v14；机器真相在 contract |
| Runtime method implementation | `delivery/server` | Retain | Protocol server side 与 projection，不持有 transport listener |
| JSON-RPC dispatch/registry | `delivery/dispatch` | Retain | method dispatch/router，不与 server 合并 |
| HTTP/SSE | transport/http | Retain | envelope I/O、stream/backpressure |
| In-process | transport/inprocess | Retain | 与 HTTP 共享 Application entrypoint |
| Server direct domain projections | 当前较多 | Refactor | 优先消费 Application read models |
| Delivery adapter imports | target 禁止 | Refactor | Delivery 只驱动 Application |
| frontend/TUI/CLI generated consumers | Desktop 仍含旧 `processRootSegment` vendored binding/fixtures；CLI/TUI 当前扫描无该 token | Defer | 精确 backlog 见 `CONSUMER_HANDOFF.md`；本 goal 不改消费者 |

## 9. 结构清理台账

### 9.1 Shared capabilities

| 能力 | 最终 owner | 证据 |
|---|---|---|
| completion | `internal/completion` | Application、taskgroup、shutdown 共享同一 completion-first join rule |
| HTTP origin | `internal/httporigin` | Application MCP policy 与 Infra MCP redirect security 共用纯 normalization |
| idempotency | `internal/idempotency` | Delivery consumer port 与 SQLite implementation 共享 opaque record/errors |
| pagination cursor | `internal/pagination` | 多 Application reads 与 Delivery error mapping 共用；base64/JSON codec 不进入 Application ring |
| replay cursor | `internal/replaycursor` | Run journal 拥有位置语义；opaque base64/JSON codec 保持在 ring 外的准确 capability |
| shutdown | `internal/shutdown` | Bootstrap 只装配/调用；deadline-aware teardown mechanism 不藏进 composition root |
| task group | `internal/taskgroup` | Application、Delivery transport 与 Bootstrap 共享 request-detached task ownership |
| path identity | `infra/pathidentity` | filesystem/symlink identity 是技术机制，Adapter 单向消费 Infra |
| secret masking | `application/secrets` | model/MCP 两个 Application consumer 共享 presentation-boundary policy |
| notification relay | `adapter/notification` | Bootstrap 只组装，producer/Delivery 各见其最小 method set；避免与 Agent Framework Signal 同名 |

`internal/component` 已物理删除；永久架构守卫只允许上表经证明的准确共享 capability，且禁止其反向依赖任何产品 ring。

### 9.2 Adapter/Infra

- `codebaseindex`、`codeintel`、`executionctx`、`hooks`、`isolation`、`mcpconnection`、`modelcatalog`、`modelclient`、`persistence`、`providerregistry`、`runrecovery`、`runsegment`、`skillproposal`、`toolset`、`workspace` 与 `workspacepath` 已逐个按调用图审计：均拥有应用值映射、外部错误翻译、跨机制组合、安全策略或 transaction write-set，不是同名 Infra 方法的包装；
- SQLite、Git、exec、sandbox、LSP、MCP/A2A client、checkpoint、storage 与 path identity 只提供可复用 technical mechanism，保留 Infra；Infra 对 Application/Adapter/Delivery/Bootstrap 反向 import 为零；
- `workspacepath` 原有第二份 symlink/containment 判定已删除，统一消费 `infra/pathidentity` 的 physical identity；Adapter 只保留 Application path-policy 错误与相对路径投影；
- `agentexec` 已按 root execution、effect attribution、waiting/restore、Delegate admission/projection、tree mutation 分离变化原因；没有第二 controller、scheduler、mailbox、private wire owner 或为了文件拆分制造的子 package；
- Application consumer-owned ports 均按用例拆分；`Coordinator` 只用于确实协调多个 use-case collaborators 的 package aggregate，不存在 package-name + exported-type 口吃或跨用例胖 executor interface；
- Delivery `server` 只做 wire validation/projection，`dispatch` 只做 JSON-RPC registry/routing/idempotency，二者对 concrete Adapter/Infra/Bootstrap/Agent Framework import 为零，现名准确且不合包；
- `internal/component`、temporary exception、空目录、历史 fixture 与一层纯转发 wrapper 均为零；最终 DAG、命名和 shared-capability purity 由永久 architecture tests 守卫。

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
| steer | run command | 必须 | 必须 | 按产品事实 | 必须 |
| Delegate child | run/lineage | 必须 | 必须 | 必须 | 必须 |
| subtree cancel | run/interrupt | 必须 | 必须 | 必须 | 必须 |
| cancel/deadline/lost | run outcome | 必须 | 必须 | 必须 | 必须 |
| rollback/fork | conversation/transcript | 必须 | parked cleanup | 必须 | 必须 |
| recovery after restart | run/session | 必须 | restore probe | 必须 | projection |

## 12. 当前结论

- Runtime 产品领域、协议、持久化和工具能力大部分保留；
- 执行 Framework integration 是主要 Rewrite 区；
- P8 已将 Agent Framework vertical 原子切为唯一生产 owner；root、managed Delegate child、waiting subtree、termination、unknown 与 recovery 均由真实 Bootstrap consumer 验证；
- Agent Framework Baseline 15 已提供 P4–P7 所需的全部公共合同并完成 canonical module 身份替换；Runtime standalone 精确绑定 `v0.0.0-20260809043847-2590dbc81a1f`，且 Framework 没有引入任何 Runtime 产品、持久化或 transaction 抽象；
- Runtime 对原框架 source/test/module dependency 与临时 module path 已归零；唯一 `agent` Framework 仍只拥有中性合同，产品 Run、Store、transaction、WorkingContext composition 与 recovery policy 均留在 Runtime；
- P11 删除迁移期 execution/port 快照文档；P12 继续删除已完成的架构清洗台账。当前架构、端口与工具接线分别只有 `ARCHITECTURE.md`、真实 consumer code/GoDoc 和 `TOOL_SYSTEM.md` 一个 owner，历史实施事实归 Git，不保留第二套错误现状；
- P12 全量静态审计捕获的格式漂移与嵌入字段冗余已在各自源码 owner 治本清除；Runtime 与 Agent Framework 的 tracked production TODO/FIXME/HACK、旧 Framework 路径、旧 replay scope、空文件、空目录和内部死代码均为零。
- P12 最终行为矩阵证明 Runtime root/child Interaction、authoritative model/tool、waiting、restore、resume、steer、unknown、recovery、rollback 与全部外环能力仍自洽；SQLite epoch 64 现由 baseline consistency test 永久守卫。`interactioninput` 是 pending continuation/prompt/resolution 的唯一 ACL 与 codec owner，原单消费者子包和第二 decoder 已删除；三个 Runtime strict-codec fuzz owner与 Agent Framework 十三个 wire/state fuzz owner均全绿。
