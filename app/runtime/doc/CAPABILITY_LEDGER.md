# Lyra Runtime 能力迁移台账

> 状态：P5 当前事实，随每个实施批次更新
>
> 基线日期：2026-08-09

本文记录当前能力事实、目标 owner、迁移 verdict、实施阶段和验收证据。它不重复目标架构和 ADR。代码变化后必须在同一批更新对应条目；不能用“计划保留”冒充“已经迁移”。

## 1. Verdict

| Verdict | 含义 |
|---|---|
| `Retain` | 所有权和抽象正确，保留实现，只做必要接线/命名同步 |
| `Refactor` | 产品语义保留，但 API、package、职责或依赖需要治本调整 |
| `Rewrite` | 能力保留，但当前实现建立在旧执行模型上，按新合同从零实现 |
| `Remove` | 能力重复、owner 错误或已经由 Agent2 原生拥有，完成阶段必须删除 |
| `Defer` | 不是当前服务端重构前置条件，有真实需求后单独设计 |

## 2. 当前基线事实

### 2.1 规模与依赖

- `app/runtime/internal` 当前有 974 个 Go 文件（含 architecture tests 与反例 fixture）；
- `adapter/agentexec` 有 110 个 Go 文件，`adapter/toolset` 有 68 个；
- 55 个 Runtime Go 文件存在旧 `github.com/Tangerg/lynx/agent` import declaration，共 90 条；逐文件和逐数量事实由 architecture AST guard 精确持有，文本扫描只用于辅助定位；
- 其中 39 个位于 `adapter/agentexec`，9 个位于 `adapter/toolset`；
- 剩余 direct imports 位于 runsegment tests 与 bootstrap tests；
- `app/runtime/go.mod` 在 parallel harness 期间同时依赖旧 `agent` 与 `agent2`；Agent2 生产 import 精确收敛在 `adapter/agentexec` 四个 integration 文件，共七条，通用 Toolset 为零；
- Domain、Application 和 Delivery 生产代码目前对旧 Agent Framework 零 import；
- P2 已删除 `domain/execution` 及全部 forwarding/alias path；Domain 生产代码与测试对 Application/Adapter/Infra/Delivery/Bootstrap 零 import，context-based I/O port 为零；
- Run、Accounting、Conversation、Transcript、Interrupt、ToolResult 已成为准确顶层 bounded-context package；executor checkpoint/ref、pending continuation 与 workspace mutation 由 Application consumer 拥有；
- P3 已删除 Application 的 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖接口；root start/observe/release、Session reads/termination 与 Run projection write-sets 均由真实 consumer-owned ports 表达；
- P3 已把 Application executor tree identity 统一为 `ExecutorMember`/`MemberID`；旧 `ProcessID` 只存在于待替换的 old Agent adapter 内部，SQLite technical shape 已一次性改为 epoch 59 的 `root_member_id`/`memberId`；
- P5 已在真实 Agent2 Engine + native Interaction root 上接通 authoritative model/tool projection、Runtime Toolset、usage/pricing、自动 approval、hooks、presentation/offload、并发 Tool batch 与 live unknown reconciliation；新路径尚未接管 Bootstrap；
- SQLite 当前唯一 shape 为 epoch 61，model/tool invocation operational journal 与 Transcript semantic final 分离；Run pump 是唯一 reducer/persistence writer；
- 当前 Agent dependency 已经主要集中在执行防腐层，这是原位重构而非完整 runtime2 的关键依据。

### 2.2 当前架构基础

| 能力 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run lifecycle | `application/runs` 拥有 start、pump、waiting、cancel、terminal ordering；P5 已验证 authoritative model/tool commit 与 live unknown terminal | Retain；P6–P8 扩展 | P3–P5 已完成；P6–P8 纵切 |
| Session lifecycle | 独立 application/domain，拥有 workspace/admission 产品语义 | Retain | P2/P8 |
| Domain framework isolation | P2 已删除十个 context-based Domain I/O port，生产与测试均由机器守卫禁止向外依赖 | Retain + strengthen | P2 已完成；例外为零 |
| Agent anti-corruption | Agent2 native root 与普通 model/tool Effect 已在 `adapter/agentexec` 建立；旧 Framework lifecycle 仍服务生产到 P8 | Rewrite | P4/P5 已完成；P6–P8 继续 |
| Delivery separation | P1 起 target DAG 禁止 Delivery import 任意 concrete Adapter；protocol/dispatch/server/transport 继续按现有职责迁移 | Retain + naming audit | P1 guard；P9–P10 收口 |
| Adapter/Infra direction | 当前主要为 Adapter 使用 Infra，Infra 不反向 import Adapter | Retain + package audit | P9 |
| Component primitives | 十个 direct package 已由 P1 exact ledger 锁定，不能新增；umbrella 名仍过宽 | Refactor ownership | P9 |
| Contract generation | `contract/` 已生成 manifest/OpenRPC/schema/TypeScript | Retain | P10 |
| SQLite exact epoch | epoch 61 单一 shape，无生产 migration chain | Retain | P5 已推进；P8/P10 继续 |

## 3. 产品领域能力

### 3.1 Run、Segment 与 execution package

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Run identity/state/outcome | `domain/run` | 保持 | Retain | P2 旧 path 归零，状态行为测试全绿 |
| Segment identity/lifecycle | `domain/run` + `application/runs` | 保持；P3 重推 root port | Retain + Refactor port | resume 保持 RunID、打开新 Segment |
| Run limits/capabilities | `domain/run` | 保持 | Retain | admission/restore 同值，不能重新谈判 |
| Terminal outcome taxonomy | Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost | P8 结合 Agent2 Termination 冻结完整映射 | Retain + complete mapping | P2 产品与 wire taxonomy 已精确；P8 完成新 Framework matrix |
| ExecutorRef/checkpoint | `application/runs` opaque executor binding/checkpoint | P3/P6 按真实 consumer 演进 | Refactor port | Run entity 不保存执行端口细节，无 Agent2 concrete type/payload parsing |
| Executor member identity | Application `ExecutorMember`、continuation/child binding | 保持不透明 member identity | Retain | P3 Application `ProcessID` 归零；旧 adapter 显式映射 |
| Step count | Run usage | 保留产品需要的计数，区分 Agent2 Step | Refactor | 不把两种 Step 当同一类型 |

### 3.2 Conversation、Transcript、Knowledge

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Conversation message log | `domain/conversation` + `application/conversations` I/O | 保持 | Retain | Count/Truncate/Seed 不依赖 Run executor |
| Transcript Items/Runs | `domain/transcript` | 保持 | Retain | rollback/fork/item timing 保持权威 |
| Offloaded transcript content | `domain/toolresult` | 保持准确独立 capability | Retain | 无泛化 blob service |
| Knowledge/LYRA.md | `domain/knowledge` | 保持独立 | Retain | 用户编辑与 Agent state 无关 |
| WorkingContext | 旧 agent interaction/history bridge | Agent2 Interaction private state | Remove Runtime duplicate | restore 不从 Conversation 重算 |

### 3.3 Interrupt 与 approval

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Interrupt semantics | `domain/interrupt` | 保持 | Retain | Kind/Key/Resolution 纯领域值，无 I/O/executor |
| Pending continuation | `application/runs.Pending` + `adapter/persistence` mapping | P6 重写 bridge，保持 owner | Refactor port | 一个 root tree 一个 pending hand-off；Infra 只见 technical record |
| Approval domain | `domain/approval` | 保持产品策略 | Retain + remove I/O ports | 不进入 Agent2 |
| Ask-user/approval tool input | toolset + old HITL codec | agentexec public Interaction helper → product Interrupt | Rewrite bridge | 不解析 private suspension JSON |
| Answer/resolution | runs + suspension adapter | semantic Application command → Signal | Rewrite bridge | 无任意 Signal API |

### 3.4 Accounting

| 当前能力 | 当前 owner | 目标 | Verdict | 验收 |
|---|---|---|---|---|
| Token/model-call accounting | `domain/accounting` + native authoritative model decorator | final/usage/pricing/Run progress 同一事务 | Refactor bridge | P5 已完成；Delta drop 不丢 final 或 usage |
| Pricing/USD | adapter/pricing + observer | Runtime adapter/domain value | Retain | Agent2 Usage 无价格字段 |
| Framework resource usage | old Agent aggregate | Agent2 Usage | Replace | 只翻译需要的中性事实 |
| Goal budget attribution | goals/runs | 保持 Application | Retain | child/root、resume、lost 归属准确 |

### 3.5 其他产品上下文

| 能力 | 当前 owner | Verdict | 迁移影响 |
|---|---|---|---|
| Goal | domain/application/toolset | Retain | executor port/name 更新；不下沉 Agent2 |
| Plan | domain/application/toolset | Retain | 保持 Plan 唯一术语；不与 Goal/Todo 合并 |
| Schedule | domain/application/toolset | Retain | 通过 Run use case 启动，不直接调用 Agent2 |
| Skill/Proposal | domain/application/adapter/toolset | Retain | deferred manifest 接线更新 |
| Agent memory | domain/application/toolset | Retain | 与 Conversation/Knowledge 分开 |
| Model/provider catalog | domain/application/adapters | Retain | 每 Run exact model binding 进入 deployment assembly |
| MCP/A2A/LSP | domain/application/infra/toolset | Retain | 保持技术能力，不进入 Agent2 Kernel |
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
| Product Cancel | durable terminal decision → executor release | Retain | product outcome 先提交，resource lifecycle 后执行 |
| Resume/rehydrate | 隔离的 provisional `ContinuationExecutor` | Rewrite port shape | P6 由 Agent2 snapshot/semantic answer consumer 重推 |
| Steer | provisional `ExecutionSteerer` | Retain semantics, Rewrite bridge | P6 由 Agent2 safe-boundary consumer 重推 |
| Child subtree cancel | `MemberID` + provisional subtree port | Refactor identity | P7 由 Agent2 prepared capability consumer 重推 |
| Waiting subtree mutation | prepared data + live Mutation lease | Rewrite as precise one-shot prepared tree change | Application 不见 Agent2 plan；Apply/Discard 前 source frozen；transaction/crash tests |
| Run pump | executor event reducer | Retain | 不推进 Agent2 internal state |
| Run journal | committed RunEvent | Retain | persist-before-publish |

P4 real Agent2 consumer 已验证 root candidate 的 stage/observe/begin/release 语义；P5 在同一 candidate 上验证了 authoritative commit receipt、Tool manifest/scope、unknown reconciliation 和 release ordering。它仍不是兼容 API，完整 port 到 P8 才冻结。Continuation/steer/subtree ports 只隔离旧生产路径，其最终方法、参数和 error 由 P6/P7 真实纵切决定。

### 4.2 Cross-aggregate writes

| 写集合 | 当前事实 | Verdict | 阶段 |
|---|---|---|---|
| Run admission | Session + Run/Transcript | Retain/refactor types | P3 |
| Waiting tree barrier | Run + Pending + checkpoint + Items | Retain semantics | P6 |
| Waiting child cancellation | Run tree + Pending + checkpoint + Items | Rewrite Agent boundary, retain App transaction | P7 |
| Terminal tree | Runs + Pending + checkpoint + transcript + Goal | Retain and converge | P8 |
| Boot recovery | stored facts + opaque checkpoint probe | Retain and rewrite probe | P8 |
| Rollback/fork cleanup | Conversation + Transcript + active/parked Run | Refactor explicit invariant | P8 |

## 5. Agent execution 能力

### 5.1 当前 `adapter/agentexec`

| 当前实现 | 作用 | Verdict | Agent2 replacement |
|---|---|---|---|
| `Engine` wrapping old runtime.Engine | Framework facade | Rewrite | per-Run Agent2 Engine assembly |
| `InteractionExecutor` native root harness | 独立真实 root consumer，当前不进 Bootstrap | Retain and extend | P4 root + P5 model/tool/unknown 已完成；P6/P7 扩展，P8 接管生产 |
| root GOAP Agent/Action | 包裹聊天 Interaction | Remove | native `interaction.Definition` |
| `TurnProcess` | 二次包装 Process wait/resume/cancel/capture | Remove | Agent2 Process/Engine public lifecycle |
| `turn` controller/pump | live turn registry、goroutine、terminal | Remove Framework duplicates | Application Run pump + Agent2 Engine |
| process tree codec | 解释旧 snapshot tree | Remove | opaque Agent2 TreeSnapshot codec at boundary |
| suspension codec | 解释 old HITL payload | Remove | Interaction public pending-input helpers |
| child execution/configuration | 传播 model/hooks/budget | Rewrite | exact child Deployments + ProcessAdmitter |
| observer | model/tool/usage projection | Rewrite, retain product semantics | P5 native decorators 已完成；旧 observer P8 删除 |
| tool decorator | approval/hooks/presentation | Retain semantics, Rewrite attribution | P5 ToolInvocation context 已完成；HITL 留 P6 |
| usage aggregator | subtree token/cost | Refactor | P5 root cumulative accounting/pricing 已完成；P7 tree roll-up继续 |
| deferred manifest glue | old toolloop promotion | Rewrite | P5 唯一 `toolset.Manifest` + Interaction DeferredTools/AdvertiseTools 已完成 |
| maintenance/restore checks | live registry + checkpoint probe | Refactor | exact deployment restore/probe |

### 5.2 Agent2 已提供的合同

| Runtime 需要 | Agent2 Baseline 9 | Runtime 责任 |
|---|---|---|
| root execution | Engine/Deployment/Interaction | 组装产品配置并翻译结果 |
| tree identity | ProcessID/Relation/root/parent/depth | 映射不透明 executor member/child Run |
| child admission | ProcessAdmitter + prospective identity，但 admit 后 start/publish 失败无 conclusive child outcome | 先补 Agent2 neutral started/aborted contract，再做 durable product Run admission |
| waiting | WaitID/Signal + Interaction pending helper | 产品 Interrupt 与事务 |
| restore | Snapshot/TreeSnapshot + exact resolver | Store、BuildID、Host metadata |
| steer | Interaction steer signal | 产品命令与内容 projection |
| subtree cancel | pure Plan/Apply 可精确变换 snapshot，但 transaction 期间不冻结 source | 先补 one-shot prepared change；Application write-set 只见 projection + opaque checkpoint |
| model/tool attribution | ModelInvocation/ToolInvocation | Transcript/accounting/hooks/pricing |
| deferred tools | Tools/DeferredTools/AdvertiseTools | Tool catalog、发现语义和权限策略 |
| observation | Event/Delta | lifecycle projection/临时流；权威记录另行同步 |
| resource facts | Budget/Limits/Usage | 产品 limits、USD accounting 和 policy |
| unknown Effect | UnknownEffectIDs/ResolveEffect | P5 live root 由 Dispatcher wake + bounded public query 收口为 durable RunLost；不开放任意 resolution；recovery/tree-wide 留 P8 |
| prepared-step durability | PreparedStepAcknowledger 只有单 Process Snapshot | 初版不启用；只恢复 committed quiescent TreeSnapshot |

当前 root Interaction、ordinary Tool/model、waiting capture/restore 和 steer 没有已知 Agent2 blocker。P7 有两个已确认的 Framework blocker：durable child admission 缺 conclusive post-admission outcome，waiting subtree 缺跨 Application transaction 保持 safe cut 的 prepared capability。它们必须先以真实 Runtime consumer 证明并更新 Agent2 ADR/baseline；不得在 Runtime 建补偿内核。单 Process prepared-step acknowledgment 本轮明确不启用，不算待实现 Runtime 能力。

## 6. Tool 能力

| 当前能力 | Verdict | Agent2 接线变化 |
|---|---|---|
| built-in tool schemas/names/descriptions | Retain | 作为 frozen Tools/DeferredTools 输入 |
| strict typed decode | Retain | P5 Dispatcher 调用同一 Tool contract，无第二 schema/decode |
| approval/safety | Retain Runtime ownership | P5 自动 allow/deny/rewrite 使用 ToolInvocation 精确归因；HITL 留 P6 |
| activity/presentation | Retain | P5 已接线，不进入 Agent2 Event/Delta |
| `search_tools` | Retain | P5 使用注入的 precise advertiser 调用 `AdvertiseTools`；Toolset 对 Agent2 零 import |
| delegation wrapper | Rewrite | Interaction `Delegate`，不再 old AgentTool |
| ask_user | Rewrite bridge | pending tool input + product Interrupt |
| Plan/Goal/Schedule/Skill tools | Retain | Application ports 更新 |
| MCP/A2A dynamic tools | Retain | 本 Run deployment assembly 冻结 authority |
| old agent/core/toolloop imports | Remove | Toolset 最终 Framework-neutral |

完整模型工具面、参数和历史删除裁决仍以 [`TOOL_SYSTEM_VNEXT.md`](TOOL_SYSTEM_VNEXT.md) 为准。

## 7. Persistence 与 recovery

| 能力 | 当前事实 | Verdict | 验收 |
|---|---|---|---|
| SQLite schema | epoch 61；保留 `root_member_id`/`memberId`，新增 model/tool invocation journals | Retain pattern | 旧 epoch/列/codec 被拒绝，无 migration |
| executor checkpoint | Host metadata + old Agent payload | Rewrite payload owner | opaque TreeSnapshot + exact expectation |
| checkpoint transaction | runsegment/persistence 组合 | Retain semantics | waiting facts 同事务 |
| BuildID | Host-owned | Retain | 不进入 Agent2 deployment/snapshot |
| boot recovery | Application policy + agentexec probe | Refactor | Agent2 exact restore validation |
| unknown Effect live/recovery | P5 native live root 已实现，旧生产路径尚未切换 | Rewrite | live wake 丢失可由 public polling 收敛；RunLost 写失败重试且 release 不提前；recovery/tree-wide 留 P8 |
| isolated recovery | fail closed | Retain | 不猜测重建 scratch world |
| terminal deletion | root aggregate transaction | Retain/converge | terminal 与 delete 原子 |
| waiting subtree persistence | Application plan + old live mutation | Rewrite boundary + Agent2 prerequisite | one-shot prepared change + opaque resulting TreeSnapshot |
| active-step crash durability | 旧路径能力不作为新合同 | Defer | 初版无 claim；无 quiescent tree checkpoint 时 RunLost |

## 8. Delivery 与协议

| 能力 | 当前 owner | Verdict | 说明 |
|---|---|---|---|
| Protocol types/errors | `delivery/protocol` + `contract` | Retain | 机器真相在 contract |
| Runtime method implementation | `delivery/server` | Retain | Protocol server side 与 projection，不持有 transport listener |
| JSON-RPC dispatch/registry | `delivery/dispatch` | Retain | method dispatch/router，不与 server 合并 |
| HTTP/SSE | transport/http | Retain | envelope I/O、stream/backpressure |
| In-process | transport/inprocess | Retain | 与 HTTP 共享 Application entrypoint |
| Server direct domain projections | 当前较多 | Refactor | 优先消费 Application read models |
| Delivery adapter imports | target 禁止 | Refactor | Delivery 只驱动 Application |
| frontend/TUI/CLI generated consumers | 当前旧 baseline | Defer | 服务端 contract 后专项接线 |

## 9. 结构清理台账

### 9.1 `component`

| 当前 package | 初步 owner verdict | 最终裁决阶段 |
|---|---|---|
| completion | 移到拥有等待/完成语义的 Application owner，或证明多 consumer 后准确提升 | P9 |
| httporigin | HTTP/MCP 真实消费者裁决，不能留泛 component | P9 |
| idempotency | RPC/transport 或 persistence owner，按身份语义拆分，不能虚假共享 | P9 |
| keyset | 查询/pagination 真实 owner | P9 |
| pathidentity | workspace/execution adapter owner 或准确共享 package | P9 |
| replaycursor | Run journal/RPC replay owner | P9 |
| secretmask | model/MCP config projection owner 或准确共享 package | P9 |
| shutdown | bootstrap/host lifecycle owner | P9 |
| signal | 真实使用点裁决，避免与 Agent2 Signal 同名 | P9 |
| taskgroup | bootstrap/application task ownership；若多 owner 证明则准确共享 | P9 |

这些是待使用图验证的 owner verdict，不是提前指定机械移动路径。最终要求是 umbrella `component` 清零且依赖不反转。

### 9.2 Adapter/Infra

- codebaseindex/modelclient/persistence/workspace 等 Adapter 是否只透传 Infra：逐个审计；
- SQLite、Git、exec、sandbox、LSP、MCP/A2A client 等可复用 technical mechanism：保留 Infra；
- Infra 若已经决定 Application use-case 顺序：职责上移 Adapter/Application；
- Adapter 若没有翻译/组合：与 Infra 合并；
- 不整体合并两个环，也不为保持现状拒绝逐包收敛。

## 10. 删除清单

以下条目完成对应阶段后必须为零：

- 旧 `github.com/Tangerg/lynx/agent` Runtime imports；
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
- `component` umbrella；
- temporary architecture exceptions；
- alias、dual wire、compat decoder、空目录和历史 TODO。

## 11. 验收覆盖矩阵

| 场景 | Domain | Application | Agent2 real vertical | Persistence | Delivery |
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
- P5 已完成 ordinary root Agent2 vertical：Application commit receipt、model/tool attribution、Toolset、accounting、并发 canonical batch、Delta/final 分层和 live unknown 已由真实 consumer 验证；P6–P8 继续演进未冻结能力；
- Agent2 已提供 P4–P6 所需的公共合同；P7 必须先补齐本台账已确认的两项中性 Framework 合同；
- 当前生产执行仍由旧 Agent implementation 承担；P4/P5 Agent2 路径只进入独立真实 harness，P8 能力齐备后才原子切换。
