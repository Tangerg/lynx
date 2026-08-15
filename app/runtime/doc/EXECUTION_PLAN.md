# Lyra Runtime 重构实施计划

> 状态：P1–P92 已完成；第二轮反证式全链路缺陷清零持续进行
>
> 工作方式：原模块内治本重构，按可验证纵切分批完成；不创建完整 `runtime2`

本文是唯一实施计划和进度台账。架构、ADR、工程标准和能力事实由各自文档拥有，这里只记录阶段目标、依赖、验收和完成事实。

## 1. 当前授权范围

当前实施 goal 已授权：

- 以 `app/runtime` 作为后端唯一业务入口，同时启动 Desktop 前端做真实端到端联调；允许修改全仓直接爆炸半径并执行 breaking correction；
- 在已完成并冻结的 P19 公共 `protocol + embedded.Runtime` 基线上，以真实 CLI consumer、终端行为、SQLite 与日志证据复核完整链路；
- 修改 Agent Framework、`app/runtime` 与 Desktop 直接爆炸半径，治本修复 Effect identity、waiting/resume/recovery、授权/HITL、取消和权威投影之间可复现的不一致；当前 goal 只读审计 `app/cli`，不修改也不暂存该模块；
- 每完成一个可独立验收批次，同步本计划和 Capability Ledger 并执行对应质量门禁；
- 允许 breaking change，不建立兼容路径；本轮新增的 Tool cancellation 产品语义以一次协议、Artifact 与 SQLite epoch 前移完整发布，不保留双份 vocabulary。

本 goal 同时审计真实 Desktop/CLI/TUI consumer。Agent Framework 仍只拥有中性进程与 Effect 语义；Runtime 的 Run、授权、HITL、Store、transaction、投影和 provider policy 均未泄露进入 Agent。

## 2. 全程约束

- 只修改 `app/runtime` 及该阶段不可避免的直接爆炸半径；P24/P25 已获得 Desktop 前端联调与全仓 breaking correction 授权，其他消费者仍按独立阶段处理；
- breaking change 允许，禁止 alias、shim、dual read/write 和两套长期路径；
- 每个阶段按依赖方向从内到外推进，但每个提交必须形成可运行纵切，不允许长期红仓；
- 旧实现可以作为事实证据，不能成为新 API 兼容规范；
- Runtime standalone 直接绑定已发布 Agent Framework Baseline 22 canonical module；workspace 与 `GOWORK=off` 消费同一 source，Runtime 不读取其 private state；
- 每批完成后更新本计划和 Capability Ledger，运行对应质量门禁，提交并推送；
- 如果发现 Agent Framework 缺口，先证明它是中性 Framework 能力且已有真实 Runtime consumer，再单独走 Agent Framework ADR/baseline；禁止在 Runtime 侧补第二套内核；
- 阶段完成不以文件数量或目录形状判断，只以验收合同和旧 owner 删除判断。

## 3. 阶段总览

| 阶段 | 目标 | 依赖 | 状态 |
|---|---|---|---|
| P0 | 文档、事实和边界基线 | 无 | 已完成 |
| P1 | 目标依赖 DAG 与迁移守卫 | P0 | 已完成 |
| P2 | Run 领域语言与 bounded contexts | P1 | 已完成 |
| P3 | Application root use cases 与候选消费端口 | P2 | 已完成 |
| P4 | Interaction 的 root 纵切 | P3 | 已完成 |
| P5 | 权威 model/tool observation 与 Tool 接线 | P4 | 已完成 |
| P6 | waiting、checkpoint、restore、resume、steer | P5 | 已完成 |
| P7 | Delegate child Run 与 waiting subtree | P6 + 两项 Agent Framework 中性合同 | 已完成 |
| P8 | terminal、recovery 与跨聚合一致性收口 | P7 | 已完成 |
| P9 | Adapter/Infra/共享原语/Delivery 结构收敛 | P8 | 已完成 |
| P10 | 协议、生成物与服务端 API 收口 | P9 | 已完成 |
| P11 | 原框架实现删除与唯一模块名替换 | P10 | 已完成 |
| P12 | 全量质量验收与消费者接线移交 | P11 | 已完成 |
| P13 | 重写后精修与双向边界复审 | P12 | 已完成 |
| P14 | Runtime 内部职责与全层级命名精修 | P13 | 已完成 |
| P15 | Agent/Runtime 合同同步与 Runtime 反证式精修 | P14 + Agent Baseline 18 | 已完成 |
| P16 | Domain aggregate 行为所有权纵切 | P15 + Domain Model | 已完成 |
| P17 | package 边界与目录结构精修 | P16 + 真实 import graph | 已完成 |
| P18 | Application mechanism 所有权纠偏 | P17 + 完整调用图 | 已完成 |
| P19 | 公共 protocol 与 embedded Runtime | P18 + 真实 CLI 嵌入消费者 | 已完成 |
| P20 | 真实 embedded consumer 回归与权威输出对账 | P19 + CLI 真实运行证据 | 已完成 |
| P21 | 运行链路、授权/HITL 与取消全矩阵闭环 | P20 + 真实 DeepSeek/SQLite/PTY 证据 | 已完成 |
| P22 | WorkingContext typed provenance | P21 + 现有 Knowledge/Memory/Plan/hooks consumer | 已完成 |
| P23 | WorkingContext 行为所有权精修 | P22 + typed provenance 回归 | 已完成 |
| P24 | Runtime/Desktop 全链路时序、事务与恢复硬化 | P23 + 真实 HITL/Plan/Goal/崩溃证据 | 已完成 |
| P25 | Runtime/Desktop 第二轮反证式缺陷清零 | P24 + 组合/乱序/失败注入/真实恢复证据 | 已完成 |
| P26 | Tool 可见 lifecycle 与真实 execution timing 分离 | P25 + 真实审批等待/Tool journal/UI 对账 | 已完成 |
| P27 | Runtime/Frontend 依赖信任边界收缩 | P26 + 真实依赖图/漏洞可达性/恢复压力证据 | 已完成 |
| P28 | Ollama standalone client/daemon 边界纠偏 | P27 + workspace module 可达性审计 | 已完成 |
| P29 | CLI standalone Runtime consumer 发布闭环 | P28 + 已推送 Runtime canonical commit | 已完成 |
| P30 | Runtime 失效流订阅隔离与断号恢复 | P29 + 多订阅/重连证据 | 已完成 |
| P31 | Goal root Run boundary / HITL 状态判定 | P30 + whole-tree stream 反例 | 已完成 |
| P32 | Goal objective incarnation / HITL Resume accounting | P31 + crash-resume accounting 证据 | 已完成 |
| P33 | Schedule 默认工作区更新合同与 Desktop 消费闭环 | P32 + 真实 UI/SQLite 失败证据 | 已完成 |
| P34 | Goal HITL capability、权威 Question answer 与 Desktop Markdown 收口 | P33 + 双客户端/真实 HTTP/事务反例 | 已完成 |
| P35 | Run 订阅终态收敛与观察生命周期 | P34 + snapshot/subscribe 竞态与多观察者反例 | 已完成 |
| P36 | Desktop 旁路事件目标解析恢复 | P35 + `runtime.subscribe` 瞬时失败反例 | 已完成 |
| P37 | Goal 完成窗口的事件/读取/产品状态闭环 | P36 + terminal outcome 与 owning Run settlement 反例 | 已完成 |
| P38 | Desktop 失效 Session 恢复的权威缺失语义 | P37 + stale deep-link/browser console 反例 | 已完成 |
| P39 | Desktop 多客户端 Session 删除与导航收敛 | P38 + remote delete / optimistic mutation 竞态 | 已完成 |
| P40 | Desktop 深链 Session lifecycle 与条件写收敛 | P39 + active/open memory / stale revision 竞态 | 已完成 |
| P41 | Desktop HITL 多客户端 resume 与 Goal 命令结算收敛 | P40 + delayed ack / remote winner / response-loss 竞态 | 已完成 |
| P42 | Desktop command replay 与 draft 冷启动所有权收敛 | P41 + response loss / in-progress / reload / remote delete 竞态 | 已完成 |
| P43 | Desktop Run opening / cancel 结算收敛 | P42 + hung response / remote winner / stale snapshot 竞态 | 已完成 |
| P44 | Desktop Session rollback/fork/metadata mutation 收敛 | P43 + rewrite/first-checkpoint/local-CAS 竞态 | 已完成 |
| P45 | Desktop Goal/Plan lifecycle、旁路失效与抽象边界收敛 | P44 + budget/replace/session-switch/late-response/Plan wire 反例 | 已完成 |
| P46 | Desktop Agent Runtime DTO 防腐与 Composer 冷启动模型收敛 | P45 + Run/Item/Interrupt/Event wire census 与 durable model race | 已完成 |
| P47 | Workspace 订阅 opening 取消与 Runtime resync fail-closed | P46 + retarget/dispose/malformed resync 反例 | 已完成 |
| P48 | Desktop API 返回事实与身份解析生命周期消费闭环 | P47 + hanging identity read / ignored mutation response 反例 | 已完成 |
| P49 | Desktop Mutation 权威事实、并发顺序与上下文归属收敛 | P48 + non-void result / rapid mutation 反例 | 已完成 |
| P50 | Knowledge CAS 与 Knowledge/Hook 失效闭环 | P49 + lost update / empty document / multi-window stale 反例 | 已完成 |
| P51 | Knowledge filesystem identity 与 crash-safe CAS | P50 + symlink / cross-process / kill recovery 反例 | 已完成 |
| P52 | 外部人工配置变更与 Runtime stream 收敛 | P51 + external create/write/rename/remove / duplicate event 反例 | 已完成 |
| P53 | Desktop Goal mutation/read-model 单一事实源收敛 | P52 + delayed response / equal timestamp / remote transition 反例 | 已完成 |
| P54 | Checkpoint source ownership 与 shadow ignore 收敛 | P53 + tracked build input / ignored generated sibling 反例 | 已完成 |
| P55 | Git 子进程仓库环境所有权收敛 | P54 + foreign `GIT_*` / physical metadata identity 反例 | 已完成 |
| P56 | HTTP sidecar 合同、Desktop 消费与事件协商收敛 | P55 + sidecar omission / outage recovery / older-topic negotiation 反例 | 已完成 |
| P57 | Desktop Runtime 冷启动、断线复检与事件流单 owner 收敛 | P56 + cold offline / crash / duplicate subscription 反例 | 已完成 |
| P84 | 多个内嵌 Runtime 共享数据库的业务所有权与崩溃接管 | P83 + CLI/Desktop 部署事实、双 Runtime/强杀证据 | 已完成 |
| P85 | Desktop 工作目录所有权与右侧 Context Dock 命令接线 | P84 + Wails v3 native dialog / Proma 右栏源码对照 | 已完成 |
| P86 | survivor Run recovery 与已挂载 read model 收敛 | P84 + local commit / data_version 盲区与 claimed-resume 强杀证据 | 已完成 |
| P87 | Run opening 命令结算与 executor activation 所有权 | P86 + HITL claim/opening 强杀窗口与阻塞 activation 反例 | 已完成 |
| P88 | Desktop final teardown 与 mutation owner 不可复活 | P87 + renderer replacement / window close / late settlement 交错 | 已完成 |
| P89 | 长对话 compaction 与 Run boundary 坐标统一收敛 | P88 + 连续多轮压缩 / fork / 事务失败与 SQLite 不变量 | 已完成 |
| P90 | EventCommit Segment 代际事务准入 | P89 + HITL resume 后旧 Segment 迟到写集反例 | 已完成 |
| P91 | Interaction Host 前置边界失败结算 | P90 + Agent Baseline 22 / 模型与 Tool 调用前事务失败注入 | 已完成 |
| P92 | 并行 Tool Effect 外部边界与失败仲裁 | P91 + 两 Tool 窗口 / pending canonical receipt / approval failure 反例 | 已完成 |

## 4. P0 — 文档、事实和边界基线

### 目标

在任何生产改动前，使两个独立实现者读完文档后会得到同一所有权、同一术语、同一阶段顺序和同一完成标准。

### 工作项

- P0-01：审计当前 Runtime package 面、旧 Agent imports、协议制品、schema epoch 和现有架构门禁；
- P0-02：建立目标架构和统一语言；
- P0-03：建立 ADR 台账；
- P0-04：建立工程实施标准；
- P0-05：建立本阶段计划；
- P0-06：建立能力迁移台账；
- P0-07：建立 contract/boundary baseline；
- P0-08：更新 `CLAUDE.md` 与 `doc/README.md` 路由，标记旧文档的当前/历史地位；
- P0-09：执行独立 Go spec review，清除 blocker、TBD、冲突链接和多重真相源。

### 验收

- 六份核心文档职责互斥且互相可导航；
- 明确不创建 `runtime2`；
- 明确 Agent Framework/Runtime、Run/Process、Conversation/Transcript/WorkingContext 边界；
- 每个生产能力都有 retain/refactor/rewrite/remove verdict；
- 实施阶段有依赖、输出、行为验收和删除条件；
- 本轮由本 goal 产生的变更只有 Runtime 文档；工作区中并存的用户改动保持未触碰、未纳入；
- 无 TODO、TBD、placeholder 或尚未裁决的 blocker。

## 5. P1 — 目标依赖 DAG 与迁移守卫

### 目标

先让架构测试表达目标边界和迁移期间允许的最小例外，防止实现一边迁移一边扩散旧依赖。

### 工作项

- 将当前 architecture tests 按 Target/Temporary 两类整理；
- 冻结 Domain、Application、Adapter、Infra、Delivery、Bootstrap 的允许边；
- 新增 Agent Framework import allowlist，默认只允许 `adapter/agentexec` integration leaf；
- 禁止 Delivery import concrete adapters，禁止 Infra import Application/Adapter；
- 建立旧 `agent` import census guard，迁移期间只允许数量单调不增，P8 切换后归零；
- 建立旧术语、compat path、private snapshot decoding 和第二 lifecycle owner 的静态守卫；
- 为最终移除 `component` 杂物层建立逐包 owner ledger，不先机械搬迁。

### 验收

- 新增一个错误 import 的 fixture 会稳定失败；
- allowlist 不使用过宽 prefix 掩盖未来扩散；
- 当前代码在明确 temporary exception 下通过；
- 每个 exception 绑定删除阶段和确切 owner。

## 6. P2 — Run 领域语言与 bounded contexts

### 目标

清除产品 `execution` 与 Framework `Execution` 冲突，建立可以独立于 Agent Framework 表达的产品领域模型。

### 工作项

- `domain/execution` 一次性改为 `domain/run`；
- 将 Conversation、Transcript、Interrupt、Accounting 按目标 ownership 提升为准确 package；
- 保持 Knowledge 独立；
- 收敛 Run、Segment、Outcome、Lineage、Limits 和 Capabilities；把 executor bridge value 留给 P3 Application boundary；
- 删除 Domain 内的 Store/Client/context-based I/O port，将真实消费接口移到 Application；
- 让 entity 自己保护状态迁移、terminal first-wins、usage monotonicity 和 checkpoint expectation；
- 将 deadline 建模为独立 TimedOut outcome，并区分 Completed、Canceled、Failed、MaxBudget、MaxSteps 与 recovery Lost；不把这些细因压成 error 字符串；
- 修正 Goal、Session、Plan、Schedule 等下游引用，不复制 ID/value type；
- 按新 owner 更新 SQLite mapping 和 protocol projection 编译面，但不改变外部 shape，除非新语义必须。

### 行为验收

- Run 跨 Segment 保持 identity；
- 非法状态迁移、usage regression、lineage/capability 矛盾在 Domain 边界失败；
- Conversation truncate/seed 与 Transcript rollback 是独立行为；
- Domain 对 Agent/Agent Framework、I/O framework 和 Delivery 零依赖；
- 旧 package path 和 terminology 全部删除。

## 7. P3 — Application root execution use cases 与候选消费端口

### 目标

从产品用例重新推导最小 root executor boundary，而不是把当前 `ExecutionControl` 换一个实现或提前设计尚无消费者的全部能力。

### 工作项

- 重新建模 Start、Observe、Cancel 和 Release 的 root 用例；Wait/AnswerInterrupt/Steer/Recover、child/subtree 只冻结应用语义与禁止泄露边界，精确 port 分别由 P6/P7 真实消费者发现；
- 将当前 `PrepareStart`、`Activate`、`Prepare` 等宽泛阶段拆成有准确事务语义的命令或删除；
- 定义 Application-owned executor reference、member identity、checkpoint envelope 和 executor fact；
- 明确 root admission、child admission、terminal、checkpoint 和 publish 的顺序；
- 为 waiting subtree 记录纯应用输入/输出和 transaction invariant，不在 P3 预造 capability shape；
- 将 Run pump 保持为产品 fact reducer，不让它推进 Framework；
- 收窄 Session/Run/Transcript/Checkpoint persistence ports，删除胖 transaction facade。
- 为保持每批可运行，旧 Agent adapter 可以直接实现新的 Application port，但不得增加中间 compatibility facade；该 concrete implementation 明确在 P8 随旧执行路径删除。

### 行为验收

- fake executor 可以证明全部应用状态机而不导入 Agent Framework；
- executor error、cancel source、deadline、lost checkpoint 映射为稳定产品结果；
- transaction failure 不会发布未提交 Run event；
- cancel/resume/terminal 竞争只有一个合法结果；
- Application 不读取 wire DTO、SQLite type 或 Framework state。
- P3 不冻结完整 executor port；root 候选必须允许 P4 真实 consumer 修订，完整 shape 到 P8 production cutover 才冻结。

### 完成事实

- root seam 已拆为 `RootExecutionStarter`、`ExecutionObserver` 与 `ExecutionReleaser`；启动顺序固定为 validate → side-effect-free stage → attach observation → durable opening → register → begin；任何 admission 前失败只释放 executor resource，不伪造产品取消；
- Run pump 保持唯一 application fact reducer；非 Waiting 的每个终止边界恰好 release 一次，Waiting tree 继续由 active owner 持有；
- `SessionLifecycle` 与 `Effects` 胖接口分别拆为真实 use case 消费的 reader/committer ports；组合 struct 只服务 Bootstrap，Coordinator 不保存 facade；
- Application 统一采用 `ExecutorMember`/`MemberID`，旧 Framework `ProcessID` 只留在待 P8 删除的 adapter 内部；SQLite technical record 同步采用 `root_member_id`/`memberId`，schema epoch 提升到 59，不保留旧列或 dual codec；
- P6/P7 尚未纵切的 continuation、steer、subtree 能力只作为隔离的旧生产路径消费 seam 存在，不属于 P3 root baseline，也不约束后续 Agent Framework consumer shape；
- architecture tests 已锁定当前 root candidate、Application Framework vocabulary 禁区、opaque checkpoint 字段和 storage exact shape。

## 8. P4 — Interaction root 纵切

### 目标

用 Agent Framework Interaction 跑通最小真实 root Run：start、模型输出、terminal、cancel。

### 工作项

- 在现有 `adapter/agentexec` 路径内建立新实现，不创建长期 `agentexec2`；
- 直接构造 Interaction Definition/Dispatcher/Deployment；
- 使用每 Run 独立 Engine、exact resolver、limits、capabilities、Event/Delta listeners；
- 从 Host Conversation 构造初始 Input；
- 把 Result/Termination 映射为 Application executor facts；
- 新路径从一开始就不实现 root GOAP 单 Action wrapper、TurnProcess 或第二 controller；
- 建立旧 owner 的精确删除清单，但生产旧路径仍被使用的文件留到 P8 原子切换同批删除；
- 先用独立真实 consumer harness 装配新路径；P4–P7 期间生产路径继续使用旧实现，直到 P8 能力齐备后一次切换，不能让半能力 adapter 接管真实 Run。

### 行为验收

- 真实 Agent Framework Engine + Interaction 完成一次 root Run；
- normal completion、model failure、deadline、cancel、panic isolation 有稳定映射；
- final Output 不依赖 Delta 拼接；
- 旧 Agent import 数量不增加，P8 原子切换后归零；
- 不存在两个同时控制同一 Run 的 execution loop。

### 完成事实

- 在现有 `adapter/agentexec` 内新增 `InteractionExecutor`，每个 staged root 独占一个 Agent Framework Engine、exact Deployment resolver、root admission guard、显式 Framework/Tree limits 和 bounded Delta listener；未创建 `agentexec2`、GOAP wrapper、`TurnProcess` 或第二执行 loop；
- P3 root candidate 经真实 consumer 验证继续成立：Stage 只解析完整 WorkingContext、选择 client 并组装 Definition/Dispatcher/Deployment/Engine/Input，零模型/Tool 调用；Observe 单消费者先 attach，durable opening 后 Begin 才调用 `Engine.Start`；
- Application 增加准确的 `ConversationReader`，在 admission gate 内读取 Host Conversation 并追加当前已校验 user message，形成 fresh `WorkingContext` seed；Agent adapter 不读取 Conversation store，Process 开始后也不回读可变 Host history；
- 成功终态从 `Process.Await`/`Result.Output` 投影 `AssistantMessageCompleted`，Reducer 用完整 final message 覆盖 partial/missing Delta；Delta 只投影临时 text/reasoning increments；
- Agent Framework Termination 集中映射为 Completed/Canceled/TimedOut/Failed/MaxSteps，模型 external failure、Host cancel/deadline、Framework panic cause 和 dispatcher panic isolation 均有确定测试；
- 初版 Engine 明确不配置 `PreparedStepAcknowledger`；P4 新路径仅由独立真实 harness 消费，Bootstrap 生产 wiring 仍使用旧 Agent owner，精确旧 import/delete ledger 未增长并保留到 P8 原子切换。

## 9. P5 — 权威 model/tool observation 与 Tool 接线

### 目标

恢复普通非挂起路径的完整产品 Transcript、usage、pricing、activity、自动 approval decision 和 hooks，同时不污染 Framework。需要用户输入的 approval/HITL 属于 P6。

### 工作项

- 使用 `ModelInvocation`/`ToolInvocation` 做 Process/Effect/ToolCall 精确归因；
- model stream wrapper best-effort 投影 chunk，并同步 durable 投影完整 final/usage；
- Tool decorator 同步投影 ToolCall、result、timing、presentation、approval 和 hooks；
- 为每个 EffectRequest 建立 context-scoped dispatch-attempt tracker：pre-call write 失败不外呼，post-call authoritative write 失败由 outer Dispatcher 返回 error 形成 unknown settlement；
- 将通用 Toolset 与 Agent Framework import 解耦；
- 使用 Interaction `Tools`/`DeferredTools` 冻结 manifest；
- 用 agentexec integration 调用 `AdvertiseTools`，保持 discovery Tool 通用；
- 对比现有 doom-loop/offload/history partition 行为，保留产品语义、删除旧 Framework 补偿。
- 接入 live unknown reconciliation：Dispatcher 直接唤醒 + 有界 public `UnknownEffectIDs` 对账，先提交 RunLost/incomplete/cleanup intent，再 Kill/release；P5 完成 ordinary root/tool 路径，P8 统一 tree-wide terminal transaction。

### 行为验收

- Delta drop 不丢 Transcript 或 usage；
- Tool 并发完成乱序不改变 canonical ToolCall 顺序和因果身份；
- deferred advertisement 在普通成功/失败路径正确提交；需要 HITL 的 rollback/restore 移到 P6；
- 自动 approval allow/deny、hook、pricing 保持 Runtime-owned；
- model started write failure、Tool started write failure、外部成功后 final/result/usage write failure、chunk drop 和并行 Tool batch partial write failure逐项有行为测试；
- 外部调用后 authoritative write 失败留下 incomplete/unknown Transcript fact，不伪造 Tool result，Effect 不自动重放；
- live unknown 不永久挂起：listener 丢失仍被 reconciliation 发现；RunLost transaction 失败可重试且不会提前 Kill；unknown 与 cancel/deadline 竞争有唯一 Lost-first 映射；
- Toolset production 对 Agent Framework 零 import。

### 完成事实

- outer Dispatcher 为每个 Agent Framework Effect 建立 context-scoped、按 EffectID 校验且并发安全的 dispatch-attempt tracker；model/Tool started 必须先完成 Application receipt 才跨过外部调用边界，post-call final/result/usage commit 失败统一使整个 Effect 进入 unknown，pre-call 失败保证零外呼；
- Application Run pump 仍是唯一 reducer/persistence writer。authoritative fact 使用同一 executor stream 的 commit/receipt handshake 和 speculative reducer，完整 Transcript、invocation journal、Run metrics 与 live publication 只有在一个 write-set 成功后才生效；
- SQLite epoch 61 新增 model/tool invocation operational journals。它们只保存 attempt started/completed/failed/unknown/incomplete 边界，不复制语义 final/result；完整 model final + cumulative usage/pricing + Run progress 同事务提交，Tool final Items 与 invocation terminal 同事务提交；
- 并发 Tool 的 start 不占用 Transcript 顺序；pump 暂存乱序完成结果，只在形成模型声明顺序的连续前缀时批量提交，并一起结算所有 receipt。批量失败会丢弃 speculative results，保留 started journal，随后以 incomplete Tool Items + `RunLost` 原子收口；
- Toolset 使用唯一 framework-neutral `toolset.Manifest`；现有 `toolset.Resolver` 直接满足 Interaction 的消费端口，Runtime execution scope 同时绑定到 manifest resolution 和实际 Tool context。`search_tools` 通过精确 callback 调用 `AdvertiseTools`，Toolset 对 Agent Framework 零 import；
- 真实 Tool decorator 保留 safety、自动 allow/deny、argument rewrite、activity/presentation、hooks、result offload、mutation paths 与 doom-loop policy。canonical Tool result 是 settlement truth；可重新读取的 live projection 和 post-hook 失败只进入观测，不把已经确定的 Effect 改成 unknown；
- model chunk 经过有界 best-effort 队列；Runtime 本地 drop 产生 OTel event，完整 final/usage 独立于 Delta 提交。慢消费反例证明 chunk 丢失不损坏 final 或 usage；
- live unknown 使用 Dispatcher 直接 wake 与有界 public `UnknownEffectIDs` polling 双通道。Run pump 在 release 前原子提交 started/incomplete diagnostic 与 `RunLost`，终态写失败持续重试；丢 wake、写失败重试和最终 release 顺序均有行为测试；
- P5 新路径仍只由真实 Agent Framework harness 消费，Bootstrap 生产 owner 保持旧 Agent 路径，等待 P8 原子切换；旧 owner 没有扩散，P5 没有引入第二执行 loop 或兼容路径。

## 10. P6 — Waiting、Checkpoint、Restore、Resume 与 Steer

### 目标

用 Agent Framework 公共 pending-input 和 snapshot 合同替换旧 suspension/continuation 私有解释。

### 工作项

- 通过 Interaction public helper 识别 pending tool input；
- 在 tree quiescent boundary 捕获 TreeSnapshot；
- 组装 Application checkpoint metadata + opaque payload；
- 按 exact Deployments 和 BuildID/Host expectation 恢复 tree；
- 将产品 interrupt answer 编码为 Interaction response signal；
- 将 SteerRun 编码为 steer signal，并明确下一安全 Step 的生效语义；
- 接入需要用户输入的 approval/ask-user HITL，并验证 deferred advertisement 在 wait/restore 前后保持 owner state；
- 新路径不读取旧 suspension private JSON、ProcessSnapshot codec 或 Continue/Resume 猜测路径；仍服务生产旧路径的对应文件加入 P8 原子删除清单；
- 初版明确不配置 Agent Framework `PreparedStepAcknowledger`；只持久化 quiescent complete-tree checkpoint，不声明 active-step crash recovery；
- 回答 Interrupt 时同一事务记录 exact answer claim、把 interrupt row 变为 `resuming`/普通读取不可见并删除旧 checkpoint；成功后才 stage live tree 或 RestoreTree，next-Segment opening transaction 证明 claim 后才 deliver semantic Signal；
- 恢复探测 unresolved unknown Effect 时拒绝自动恢复并返回准确不可恢复事实，不调用任意 `ResolveEffect`。

### 行为验收

- 未回答 waiting、已回答待继续、restore 后 resume/continue 均有真实测试；
- corrupt payload、wrong BuildID、wrong DeploymentRef、外部 workspace 失效 fail closed；
- unresolved unknown Effect 不重放、不伪造结算，进入 RunLost recovery；
- checkpoint 与 Pending/Run write-set 原子；
- conversation 在等待期间变化不会被静默用于恢复；
- steer 只在 Strategy 安全边界消费且延迟可观察。
- answer claim 前 crash 仍可恢复 waiting checkpoint；claim 后、RestoreTree 后、Signal accepted 后到下一 quiescent checkpoint 前 crash 均 RunLost，绝不重放旧 answer；
- active step crash 不从单 Process Snapshot 猜测 tree recovery。

### 完成事实

- Interaction 只通过 public pending-input helper、`TreeSnapshot`/`RestoreTree`、typed response/steer Signal 实现 waiting/restore；P12 将只有单一消费者的 codec 子包及重复 decoder 收回唯一 `interactioninput` ACL，新路径对旧 Agent 零 import；
- quiescent waiting reconciliation 从 public Process status + Strategy helper 捕获完整 tree，并将 opaque TreeSnapshot、Pending、Run/Items 作为 Application tree barrier 原子提交；Runtime 未配置 `PreparedStepAcknowledger`，unknown Effect 无法被捕获为 recovery point；
- `ClaimResume` 在 SQLite epoch 62 内原子记录答案、`open -> resuming` 并删除旧 checkpoint。Continuation opening 必须在事务内 `RequireResumeClaim`；下一 barrier 只有 exact Session/executor/root-member owner 可以替换，terminal/boot recovery 删除 hidden claim；
- continuation 使用 `StageContinuation`/`BeginContinuation`：live waiting tree 必须匹配已提交 checkpoint，cold restore 必须匹配 BuildID、Host scope、TreeSnapshot root/status、exact DeploymentRef 与 active WaitID；Application opening commit 早于 Signal；
- post-claim pre-opening failure 先提交 root `RunLost` 再 release staged tree。RunLost write 失败保持 tree/claim；release 失败与原 cause 一起报告。boot 对任何无 checkpoint 的 claimed-resume tree 都确定收口为 Lost；
- 真实 Runtime `ask_user`、interactive approval、approval argument/remember resolution、doom-loop HITL 与 deferred advertisement 均复用产品 Interrupt contract。Approval restore 不重跑 pre-hook 或 plan；
- steer 使用 `RunningExecutionSteerer.SubmitSteer`，Agent Framework 在当前 model call 之后的下一 safe boundary 消费；Runtime 依据 `ModelInvocation.AppliedSteerSignalIDs` 将精确产品消息批次在首个能看到它们的 `ModelCallStarted` 前原子提交；
- corrupt TreeSnapshot、wrong BuildID/DeploymentRef、missing/isolated workspace、capability mismatch、claim result drift、conversation change、unknown checkpoint prohibition 和 answer/release ordering 均有真实行为或 SQLite transaction test。

## 11. P7 — Delegate child Run 与 waiting subtree

### 目标

让 managed Delegate child Process 与产品 child Run 形成唯一、可恢复的因果映射。

### 工作项

- P7 的第一项是补齐并冻结两个中性 Agent Framework 前置合同：admitted child 的 conclusive started/aborted outcome，以及在 Apply/Discard 前冻结 source tree 的 one-shot prepared change；在此之前不得接 durable child/subtree production path；
- 实现 context-aware `ProcessAdmitter` adapter；
- root/child admission 使用 prospective Process identity 和 stable StartedAt；
- Delegate ToolCall durable commit 必须早于 child opening reservation；admission 只创建不可见 Opening，conclusive started fact 后才公开 Running；
- 使用 `DelegateChildKey` 关联父 ToolCall 和 child Run；
- 持久化 child Run binding，但不复制 Framework topology/snapshot；
- restore 已有 binding 时不重复 admission；
- agentexec concrete 持有 prepared tree change；Application 只消费 canceled/paused member、opaque resulting checkpoint 和 Apply/Discard capability，执行 prepare → transaction → Apply/Discard；
- 区分 Delegate product child 与 Workflow/Planning framework-internal child；
- 新路径不实现旧 ChildOpeningRequest、Blackboard、configure-child 或自建 subtree controller；仍服务生产旧路径的 owner 加入 P8 原子删除清单。

### 行为验收

- admission 拒绝前零 Process 发布；admission 成功后的 started/aborted 对同一 prospective identity 完整且无 ghost Opening；
- child start/terminal/cancel 与父 ToolCall 因果稳定；
- 多 child、嵌套 child、restore、并发 completion 不产生重复 Run；
- waiting child cancel 的 durable state 与 applied tree 完全一致；
- transaction failure Discard 且 live tree 不变；commit 后 Apply 精确一致，commit 后 crash 恢复 resulting checkpoint，无法证明的 apply failure 丢弃旧 tree并恢复该 checkpoint，失败才 RunLost；
- Framework 仍对 Run/transaction/store 零感知。

### 完成事实

- Agent Framework 以中性 `ProcessStartOutcomeAcknowledger` 闭合 accepted admission 后的 started/aborted 结论；Runtime 在 Delegate ToolCall durable commit 后创建不可见 child opening reservation，收到同一 prospective member 的 conclusive started fact 后才公开 Running，aborted 则只闭合 reservation；
- `InteractionExecutor` 从 Interaction-owned typed active-child inspector 投影稳定 model-call/tool-index/ChildKey/Process identity，不读取 Strategy private state；多 child、nested child、sibling 乱序完成与 restore 已验证不重复创建产品 child Run；
- child opening persistence 使用单一 transaction boundary，public `CommitStartedChildRun` 不在外层 transaction 内重入另一个 public transaction method；non-reentrant transaction 反例已锁定该所有权；
- Agent Framework Baseline 14 将 prepared waiting-subtree 变换收敛为 contextless one-shot `Apply()`：全部可失败、可取消的 staging 在 Prepare 内完成，Host durable decision 之后请求取消不能撤销提交；Agent Framework 仍不感知产品 Run、Store 或 transaction；
- agentexec concrete 独占 prepared Framework capability；Application 只消费 canceled/paused member projection、opaque resulting checkpoint 与 Apply/Discard/Continue。transaction failure Discard；commit 后 Apply 只安装 resulting state；移除最后边界时独立 Continue 才激活已提交 Segment。无法证明 Apply 成功时先 release obsolete owner，再通过 `WaitingExecutionRestorer` 精确恢复 committed checkpoint，恢复失败才 durable `RunLost`；
- P7 重复门禁暴露并治本修复旧 turn shutdown 的 stale-attempt 竞争：caller deadline 只结束当次等待，`turnState.done` 才是资源释放真相；旧 attempt 的 context error 不再污染已经完成的后续 join；
- P7 新增 child integration 没有堆进新的巨型文件：production 按 Delegate binding、child admission、child projection 分文件收敛，集成测试按核心执行、restore、waiting subtree 与 fixture 分离；全部保持同一 `agentexec` package，不为文件拆分制造新 package 或接口；
- P7 的 parallel harness 已在 P8 原子切为生产 owner；该阶段保留的旧 child controller、GOAP wrapper 和 suspension codec 已全部删除。

## 12. P8 — Terminal、Recovery 与跨聚合一致性收口

### 目标

统一正常终态、取消、run_lost、启动恢复、online checkpoint loss 和 rollback 清理路径。

### 工作项

- 汇总 P2 taxonomy、P4 root、P6 waiting/recovery、P7 child 逐步验证结果，用 Agent Framework Termination + Application control intent 完成并行为冻结完整 product outcome matrix；
- 收敛 terminal write-set、checkpoint deletion、Pending cleanup、Transcript repair、Goal reporting；
- 将 P5 live unknown 收口扩展到完整 root/child tree，并与 cancel/deadline/terminal first-wins matrix 合并；
- 重审 isolated workspace、BuildID、Session cwd/isolation 和 rollback 的恢复 policy；
- 启动 recovery 只读取 Application facts，通过 agentexec probe opaque checkpoint；
- 删除 root-only 与 tree-wide 重复 terminal transaction；
- 在全部执行能力纵切通过后一次切换 Bootstrap 到 Agent Framework 实现，并删除旧 root GOAP wrapper、Engine facade、TurnProcess、turn controller 和其余生产 execution path；
- 覆盖 crash point：quiescent capture 前后、waiting commit 前后、answer claim 前后、subtree commit 后 apply 前、terminal commit 前后；不包含未启用的 prepared-step ack。

### 行为验收

- 每种终态 cause 有唯一产品映射；
- terminal transaction 失败保留可重试的完整 aggregate；
- run_lost 清理不留下孤儿 Pending/checkpoint/child Run；
- rollback/delete/restore 会清理其作用域内 parked Process；
- boot recovery 与 online recovery 使用同一领域不变量。

### 完成事实

- Bootstrap、boot recovery、Run opening/continuation/cancel/release 已统一装配 `InteractionExecutor`；旧 Agent module dependency、root GOAP wrapper、Engine facade、TurnProcess、turn controller、private tree/suspension codec 及其旧测试已物理删除，Runtime 对旧 Agent import 为零；
- fresh root 由 Application `WorkingContextComposer` 读取产品 Conversation、Knowledge、Plan、Memory 与 hooks，输出完整模型上下文；Agent Framework 只接收中性 WorkingContext/Tool/Deployment，不读取产品 Store，也没有新增 Runtime 抽象；
- 产品取消先提交 control intent，再由 `RunningRootCancellationRequester` 请求 Framework 在安全边界停止；Run pump 持续观察到确定终态，`ExecutionReleaser` 只释放资源，不再把请求 context 取消混成产品终态；
- terminal matrix 已覆盖 completion、root/parent/host deadline、root/parent cancellation、model-call limit、strategy/external/contract/panic failure 与无意图 Engine kill；live/recovery unknown 均在 release 前 durable 收口为 `RunLost`；
- checkpoint 只接受带 exact build/deployment/workspace/model/limits/capabilities 的 Agent Framework public complete-tree snapshot；answer claim 后旧恢复点失效，active-step crash 不伪装为 effect-level durable recovery；
- Toolset 只暴露 framework-neutral `Manifest` 与精确 `ToolAdvertiser` capability；Agent Framework advertisement 由 agentexec 在调用边界注入，通用 Toolset 对两个 Framework 均零依赖；
- child opening 只保留 admission reservation → conclusive start outcome 的生产路径；旧 `ChildOpeningRequest`/rehydrate shadow path 删除，持久化测试改用 Runtime 自有 opaque executor-tree fixture；
- 删除旧执行路径后留下的空目录、漂移术语、只服务旧路径的 fixtures 与 temporary architecture exception 已清零。

## 13. P9 — 外环结构收敛

### 目标

在执行语义稳定后清除目录和包装坏味道，不把结构调整与核心生命周期调试混在一起。

### 工作项

- 逐包审计 Adapter/Infra：删除纯转发，保留真实 translation/mechanism；
- 逐包处置 `component`：归还 owner 或提升为准确共享 capability package；
- 收敛 `agentexec` 内部文件/package，只有证明独立变化原因才拆子包；
- 审计 application package 的 Coordinator/Service/Manager 口吃和胖接口；
- 复核 Delivery `server`/`dispatch` 的依赖与职责，保留准确现名并删除越界行为；
- 删除空目录、旧路径、历史 fixture 和 architecture temporary exceptions。

### 验收

- import graph 满足最终 DAG；
- 无 `component/common/core/utils` 杂物分类；
- 无一层只转发同名方法的 wrapper；
- package 名和 exported type 无口吃；
- 目录只反映真实 owner，不为对称存在。

## 14. P10 — 协议、生成物与服务端 API 收口

### 目标

让 Runtime Protocol 精确反映新的 Run/Segment/Interrupt/executor 语义，同时保持 transport 独立。

### 工作项

- 审计所有 execution/turn/process 相关 wire 名称；
- 更新 Go contract registry、OpenRPC、JSON Schema、manifest、examples 和 server projection；
- 一次性升级 artifact/schema version；
- 保持 protocol/server/dispatch/transport 职责分离；
- 记录尚未接线的前端/TUI/CLI breaking surface，不添加兼容字段；
- 更新 API/TRANSPORT/AUX_API 语义规范。

### 验收

- contract generator 零漂移；
- strict validator、canonical samples、artifact round-trip 全绿；
- HTTP/SSE binding 与 Application 语义一致；
- server 无旧字段/method/event；
- consumer backlog 精确列出，不伪装为整体完成。

## 15. P11 — 原框架实现删除与唯一模块名替换

### 目标

完成 Agent Framework 项目最后的消费者迁移和模块替换，只保留一个 canonical module。

### 工作项

- [x] workspace 搜索并确认原框架实现没有剩余 consumer 或独有能力；
- [x] 删除原框架实现；
- [x] 将重写实现原子安装为唯一 `agent` directory/module path；
- [x] 更新 Runtime import、workspace metadata、文档、baseline 和 architecture guards；
- [x] 完成 Agent Framework standalone、Runtime standalone 与 workspace 最终门禁；
- [x] 不保留 alias module、replace compatibility 或双 framework path。

### 验收

- workspace 只有一个 Agent Framework module/path；
- 原框架 symbols、imports、docs 和 module metadata 为零；
- Agent Framework standalone 全门禁和 Runtime 全门禁同时通过。

## 16. P12 — 全量质量验收与消费者移交

### 目标

证明重构后的 Runtime 自洽、无旧债，并把准确协议变化移交给消费者专项。

### 验收矩阵

- [x] Agent Framework 与 Runtime standalone `go mod tidy -diff` 为零，workspace 与 standalone 解析同一 canonical Framework source；
- [x] root/Agent Framework/Runtime 受影响后端 module 的 workspace build/vet/test；Agent Framework 与 Runtime standalone staticcheck、完整 lint；
- [x] Runtime 禁用缓存普通测试、完整 race 和 continuation/prompt/resolution 三个 strict-codec fuzz owner；
- [x] Agent Framework standalone 禁用缓存普通测试、完整 race、13 个 fuzz owner和 8 个真实 command examples；
- [x] SQLite fresh schema、HTTP/SSE、waiting/restore/recovery/rollback 高风险矩阵在 race 下重复验证；
- [x] architecture DAG、Agent API/wire baseline、Runtime protocol/contract/schema digest 和 generator 零漂移；
- [x] 旧名称、旧 import、compat codec、空目录、tracked 空文件、TODO/FIXME/HACK、死代码、漂移注释与失效本地文档链接扫描；
- [x] 前端/TUI/CLI breaking surface 由 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md) 精确移交，未修改消费者实现。

完成 P12 只表示服务端和 Agent Framework 自洽。消费者接线完成后才能宣称整个产品迁移完成。

## 17. P13 — 重写后精修与双向边界复审

### 目标

在不恢复迁移路径、不修改消费者的前提下，对已经自洽的 Runtime 做第二轮实现级反证审计；只处理真实职责混杂、重复 owner、并发所有权、命名和可变事实漂移，不以指标驱动制造细碎抽象。

### 工作项

- [x] P13-01 清除 Architecture/Capability Ledger/Contract Baseline 的可变阶段与版本漂移，并建立单一版本 owner 的永久门禁；
- [x] P13-02 精修 Application/Domain 的状态变换、聚合行为和 use-case orchestration；
- [x] P13-03 精修 Adapter/Infra/Delivery 的转换、技术机制与协议边界；
- [x] P13-04 执行 Runtime/Agent Framework 双向抽象泄露复审、全量 race/fuzz/contract/standalone 门禁并冻结事实。

## 18. P14 — Runtime 内部职责与全层级命名精修

### 目标

在 Agent Framework/Runtime 边界已经冻结的前提下，只反证 Runtime 内部实现：清除职责混杂、伪共享 package、重复判断、过程式聚合行为和含混命名，同时保持协议与消费者范围不变。

### 工作项

- [x] P14-01 复审 Domain/Application 聚合行为、use-case 依赖边界和内部命名；
- [x] P14-02 复审 Adapter/Infra 的 package owner、技术机制、转换职责和内部命名；
- [x] P14-03 复审 Delivery/Bootstrap 的组合与协议职责、文件布局和内部命名；
- [x] P14-04 执行全量 standalone、race、fuzz、生成物、跨环依赖、重复/死代码、目录/文件/标识符命名与空残留复扫。

### 验收

- 不以复杂度或文件大小本身驱动拆分，只处理可证明的多 owner、重复事实或语义失真；
- package、目录、文件、类型、方法、函数、常量和变量均准确表达其 owner 与行为；
- 不新增 Runtime → Agent Framework 抽象泄露，也不让产品策略进入 Framework；
- 不修改前端、TUI、CLI，不引入兼容层；
- 每个可独立验收批次更新事实、提交并推送。

## 19. P15 — Agent/Runtime 合同同步与 Runtime 反证式精修

### 目标

在上一轮零已知残留基线上重新从反例出发审计 Runtime：精确消费 Agent Framework 新合同，继续清除模块内部职责、并发、状态不变量和全层级命名坏味道，同时证明两模块没有互相泄露产品或内核抽象。

### 工作项

- [x] P15-01 同步提交式 cancellation 与 exact applied-steer attribution，消除本地时序猜测、持锁 Engine 调用和 steer 前缀投影；
- [x] P15-02 复审并精修 Domain/Application 内部 owner、行为与命名；
- [x] P15-03 复审并精修 Adapter/Infra/Delivery/Bootstrap 内部 owner、机制与命名；
- [x] P15-04 执行 standalone、race、fuzz、生成物、边界、死代码、复杂度、目录与全层级命名最终复扫。

### 验收

- Agent concrete types 仍只存在于 `adapter/agentexec`，Application 只消费产品 facts 与 opaque checkpoint；
- Runtime 产品消息、Run、Store、transaction 与 recovery policy 不进入 Agent Framework；
- 每个新发现以真实反例、唯一 owner、完整行为测试和无兼容纵切收口；
- 不修改前端、TUI、CLI。

## 20. P16 — Domain aggregate 行为所有权纵切

### 目标

按 Domain Model 的单一 owner 标尺，依次完成 Run、Transcript Item、Plan、Session 的完整纵切，再复审 Domain package；Domain 只拥有单 aggregate 行为与纯领域决策，Application 继续独占跨 aggregate write-set、事务、并发和外部生命周期。

### 工作项

- [x] P16-01 统一 `run.Run` aggregate、Run/Tool failure taxonomy 与 cumulative accounting，删除 Transcript 的第二 Run carrier，并让 Persistence 只验证、持久化聚合已经决定的迁移；
- [x] P16-02 关闭 Transcript Item tagged union 的公开拼装入口，把各 variant 构造与 ToolCall terminal first-wins 收回 `domain/transcript`；
- [x] P16-03 让 `plan.State` 独占 replacement/revision/invariant，建立 `application/plans` 用例并删除 Tool Adapter 直连 Store；
- [x] P16-04 让 `session.Session` 独占 construct/edit/fork/relocate 行为，Store 只保存已决定的下一 aggregate；
- [x] P16-05 按词汇、变化原因、消费者和 import DAG 复审 Domain package，并冻结行为所有权、命名与空残留永久门禁；
- [x] P16-06 完成 standalone、race、fuzz、contract generator、architecture、死代码、lint、文档和 hygiene 总验收。

### 验收

- Run、Item、Plan、Session 每种领域状态变化只有一个准确命名的 Domain 入口，外部无直接 mutation；
- Application 只组织跨 aggregate 行为，不复制单 entity invariant；
- Persistence 不决定领域迁移，Adapter 不越过 Application 成为用例 owner；
- Runtime/Agent Framework 边界、opaque checkpoint 和当前消费者隔离保持不变；
- 每个 batch 独立验证、记录、提交和推送，不修改前端、TUI、CLI。

## 21. P17 — package 边界与目录结构精修

### 目标

按真实 owner、独立变化轴、消费者集合与依赖方向重新裁决全部 Runtime package；删除单消费者伪共享、错误分层、仅为目录对称存在的子包与含混 package 名，同时保留具有独立安全边界、SDK 隔离、bounded-context 语言或多消费者纯机制的小包。

### 工作项

- [x] P17-01 裁决并收敛顶层共享 capability：业务语义归还真实 owner，纯 framing/mechanism 只在具有多个真实消费者时共享；
- [x] P17-02 收敛 Toolset 的单工具目录爆炸、cycle workaround 和只为转发存在的子包；
- [x] P17-03 复审 Domain/Application package 的 bounded-context 语言、用例 owner、I/O port 与共享策略边界；
- [x] P17-04 复审 Adapter/Infra/Delivery/Bootstrap/Testsupport package 的 SDK 隔离、技术机制、组合职责与命名；
- [x] P17-05 执行全量 import graph、standalone、race、fuzz、generator、lint、deadcode、文档与空残留验收，并冻结 package-boundary guard。

### 验收

- 不以文件数机械合并：每个保留 package 都能说明独立词汇、独立变化原因和禁止泄露的边界；
- 单消费者且没有 ring/SDK/security 隔离理由的伪共享 package 为零；
- package、目录、文件和公开/私有标识符使用同一准确语言，无 stutter、cycle-workaround package 或 umbrella；
- Domain/Application 不因收敛反向依赖 Adapter/Infra，Bootstrap 不接收业务/机制行为，Agent Framework ACL 不被打穿；
- 不修改前端、TUI、CLI，不建立兼容路径；每批记录、验证、提交并推送。

## 22. P18 — Application mechanism 所有权纠偏

### 目标

修正 P17 将“被外环引用”误判为“跨环共同拥有”的三个 package，让分页、Application 后台任务与 continuation token framing 回归真实行为 owner，同时保持 Delivery/Bootstrap 只向内消费且不制造 `util`/`common` 收纳层。

### 工作项

- [x] P18-01 将 `internal/pagination` 原子移动为 `internal/application/pagination`，同步 Application/Delivery consumer 与 cursor architecture gate；
- [x] P18-02 将 `internal/taskgroup` 原子移动为 `internal/application/taskgroup`，同步 Application/Bootstrap consumer 与 Delivery lifecycle 禁令；
- [x] P18-03 将 `internal/opaquetoken` 原子移动为 `internal/application/opaquetoken`，保持 pagination/Run replay payload 语义分离；
- [x] P18-04 收紧 shared capability allowlist、永久禁止旧根路径，并区分媒体内容 codec 与 Application continuation framing；
- [x] P18-05 完成 standalone、race、lint、staticcheck、deadcode、generator、架构、文档与空残留验收，更新事实后提交推送。

### 验收

- 三个旧根 package 路径物理删除，无 alias、shim 或转发；
- pagination/taskgroup/opaquetoken 只能保留 Application ownership，不反向依赖 Adapter/Infra/Delivery/Bootstrap；
- Delivery 只消费 Application pagination contract，Bootstrap 只装配 Application task ownership；
- 根级 pure capability 只保留有真实跨环行为消费者的准确 package；
- 不修改 Runtime Protocol、SQLite/Artifact shape、Agent Framework、前端、TUI 或 CLI。

## 23. P19 — 公共 protocol 与 embedded Runtime

### 目标

让外部 Go 程序在同一进程内直接拥有完整 Runtime，同时保证 HTTP 与 embedded 只存在一套协议类型、一套 operation 语义和一套 Runtime 生命周期；不把内部 Application、Host、Store、Agent Framework 或 transport envelope 公开。

### 工作项

- [x] P19-01 冻结 ADR、目标结构、公共/私有边界、生命周期、独占锁与批次验收；
- [x] P19-02 原子移动公共 protocol values/validation/version，收回服务端 method interfaces、context plumbing、numeric JSON-RPC code 与 generator internals，删除旧 internal protocol path；
- [x] P19-03 建立 binding-neutral typed operation catalog/pipeline，让 HTTP dispatch 与 embedded 共用 validation、capability、idempotency、safe problem projection 和 run-stream replay；
- [x] P19-04 建立单一 Runtime instance builder、data-directory advisory lock、recovery/workers ownership 与完整 retryable Close；
- [x] P19-05 实现公共 concrete `embedded.Runtime`、Config、command/subscription options 和外部 module import/stream/lifecycle 行为测试；
- [x] P19-06 更新 contract generator、架构门禁、公共 Go surface baseline、README 示例和 consumer handoff，执行全量质量与坏味道复扫。

### 强制顺序

1. `protocol` 先成为唯一公共值 owner，旧 path 同批删除；
2. operation 先让现有 HTTP 通过，再接 embedded，避免第二业务入口；
3. shared instance builder 先证明 HTTP 行为无回归，再由 `embedded.Open` 消费；
4. 每批只提交自洽、全绿的唯一路径，不保留 alias、shim、双 registry 或 JSON-RPC round-trip；
5. 本阶段不修改 `app/cli`、前端或 TUI，它们只作为后续 consumer backlog。

### 行为验收

- 外部测试 module 可以导入 `runtime/protocol` 与 `runtime/embedded`，不能导入任何 `internal`；
- HTTP 与 embedded 对同一请求得到等价的 validation、capability、idempotency、problem 和 replay 结果；
- embedded 不启动 listener、不要求 token、不做 JSON-RPC/SSE 编解码；
- `AfterEventID` 精确保持 tail/replay/cold-recovery 语义，`IdempotencyKey` 对 unary 与 run-stream command 均复用 durable record；
- 请求取消不取消已接受 Run；订阅取消只释放该订阅；Runtime Close 停止 admission、结束订阅、join workers 并逆序关闭资源；
- 同一路径、符号链接别名和第二进程均不能同时打开同一数据目录；Open 失败回滚锁，Close 未完成时不提前释放锁；
- 公共 surface 无胖 interface、context key、numeric RPC code、Router、Host、Store、Engine、Application concrete 或 transport type；
- generator 零漂移，standalone build/vet/test/race/staticcheck/lint/deadcode/fuzz 与架构门禁全绿。

## 24. P20 — 真实 embedded consumer 回归与权威输出对账

### 目标

以真实 CLI embedded consumer 的终端、数据库与结构化失败为反例，逐层区分 TUI 交互、consumer 适配、Runtime 投影和 Agent Framework 合同问题；Runtime 只修复自己拥有的权威输出对账，不吞掉上游有效数据，也不以展示层补丁掩盖持久化错误。

### 工作项

- [x] P20-01 隔离运行 mock TUI 与真实 embedded 数据库，复核多行输入、运行中滚动、Tool 详情、Run/Item/model/tool invocation、数据库完整性和宿主 telemetry 输出边界；
- [x] P20-02 让同一 model call 的 reasoning/text 流式观察持续到权威 `ModelCallCompleted` 边界，再以最终消息完成原 Item，消除 reasoning 重复投影；
- [x] P20-03 为真实 DeepSeek provider metadata 的 Agent Schema 失败建立最小回归并在唯一 Framework schema owner 修复；Runtime 未丢弃 metadata、识别 provider 或绕过校验；
- [x] P20-04 执行 Runtime targeted/full/race/static/generator/hygiene 与隔离 embedded 真实回归，确认 Transcript、Run outcome 和数据库完整性一致。

### 验收

- 同一 model call 的每个语义 reasoning/message 只形成一个 canonical Transcript Item；Delta 可以丢失，最终模型响应仍是权威内容；
- DeepSeek 的字符串、对象等合法 metadata 不再导致成功模型响应被标记为 failed，修复位于真实 schema owner；
- 不改变公共 Runtime Protocol、Go API、SQLite epoch 或 Session Artifact shape；
- TUI 交互验证与后端持久化验证分别有证据，不能用 mock UI 通过代替真实 embedded 闭环；
- 不建立 sanitizer、schema duplicate、失败吞噬、兼容 alias 或 provider 特判。

## 25. P21 — 运行链路、授权/HITL 与取消全矩阵闭环

### 目标

以真实 DeepSeek、embedded SQLite、mock Runtime 和 PTY 四类证据覆盖普通对话、连续/并发 Tool、授权、问题型 HITL、resume、取消、重连、重复/乱序事件与进程重启；任何异常都回到拥有 identity、lifecycle 或 transaction 不变量的源头修复。

### 工作项

- [x] P21-01 将 model/tool/approval/HITL 的 canonical Item identity 与外部 invocation attempt 分离，消除同一 block 重复 started，并冻结重复、乱序、Delta drop 和最终权威响应的 reducer 语义；
- [x] P21-02 让 waiting checkpoint 携带并验证完整 product capabilities，恢复时经真实 Agent tree probe；answer claim、opening commit、Framework continuation 与 checkpoint 删除保持准确顺序；
- [x] P21-03 覆盖 allow once、deny、session/project/global remember 及其作用域边界；覆盖问题 choice/text、取消、同进程 resume 和崩溃重启 resume，且 restore 不重复 policy、hook 或 Tool admission；
- [x] P21-04 将 product root/subtree cancellation 绑定到对应 Agent Effect dispatch context；协作式在途 model/Tool 立即形成确定 settlement，子树取消不影响 root 或 sibling，Framework 继续独占安全边界与最终 lifecycle；
- [x] P21-05 将 product Segment active duration 从长寿命 Process wall time 中分离，排除授权/HITL 等待且不跨 continuation 重复累计；
- [x] P21-06 以 `tool_canceled` / `toolCanceled` 表达 Tool 所属 Run 取消造成的终止，以 `child_run_canceled` 保留父 Delegate Tool 语义；同步 Protocol `2026-08-11`、Artifact v16、SQLite epoch 67、生成物与 CLI 展示，不暴露内部 cancellation sentinel；
- [x] P21-07 完成 Agent/Runtime/CLI targeted、full、race、static、generator、fuzz、SQLite invariants 与真实 DeepSeek 端到端验收。

### 验收

- 任一 canonical Item 最多 started 一次；连续/并发 Tool 即使乱序完成，Transcript 顺序、SourceCallID、model history 和 invocation journal 仍一致；
- waiting Run 的 open Tool/model context、Pending、checkpoint 和 capabilities 原子一致；resume/cancel/recovery 只有一个 owner，终态不遗留 checkpoint、open interrupt 或未知 Effect；
- 授权五种选择均映射到准确 decision/scope，session/project/global 规则只在合法边界内可见；问题回答保持字段顺序和精确类型，取消与重启恢复均可闭环；
- root 取消中止整棵树的协作式在途 dispatch；child 取消只中止目标及后代，root/sibling 继续运行；Tool 显示 canceled 而不是通用 error；
- Segment active duration 不包含人工等待，且恒不超过对应 wall duration；
- 真实数据库 `integrity_check=ok`、foreign-key 零违规，无未完成 invocation、pending interrupt 或 checkpoint；全量与竞态门禁无未解释失败。

## 26. P22 — WorkingContext typed provenance

### 目标

在不改变 prompt 文本、不公开 Adapter 类型、不扩张 Agent Framework 的前提下，使 fresh-root checkpoint 能解释模型实际看到的 Runtime context 来源。

### 工作项

- [x] P22-01 在 `adapter/agentexec` 建立 versioned context source/purpose 与 prompt composition，覆盖 base、Knowledge、pinned Memory、Agent document 和 Plan，并只记录预算后实际渲染的来源；
- [x] P22-02 为 recalled Memory 的 system message 与 SessionStart/UserPromptSubmit hook 注入 Part 标记同一 metadata，保持 user media/text ordering 与既有错误策略；
- [x] P22-03 以 source 顺序、reference、instruction/data、预算裁剪、Message validation 和完整 WorkingContext 行为回归冻结内部合同；确认 Protocol、Artifact、SQLite 与 Agent Framework public API/wire 均未变化。

### 验收

- 每个实际可见来源按 prompt 顺序产生一个 typed source；被预算整体排除的 Memory/Agent document 不产生虚假来源；
- recalled/pinned Memory 标记为 data，其余现有 instruction layer 标记为 instruction；hook provenance 留在注入 Part，不污染用户原始 part；
- 完整 Message 继续通过 Core validation，文本与既有 header/order 回归不变；metadata 随 opaque checkpoint 恢复但不进入公共协议或 Store schema；
- Runtime full/race/static/contract/hygiene 门禁无新增失败，Agent Framework package DAG 继续禁止 Runtime import。

## 27. P23 — WorkingContext 行为所有权精修

### 目标

在不改变 WorkingContext 文本或跨层合同的前提下，让 context source、prompt fragment、hook result 与 composer 各自拥有自己的构造、校验和应用行为，删除调用点手工同步 text/provenance 的过程式路径。

### 工作项

- [x] P23-01 由 source kind 唯一派生 instruction/data purpose，并由 source collection 在写入 metadata 前验证 kind/purpose 不变量；非法组合 fail closed。
- [x] P23-02 由 pinned Memory/Agent documents prompt fragment 同时拥有预算后文本与实际来源；由 `WorkingContextComposer` 统一拥有 system message、Plan、hook evaluation/application 与 recall，不保留测试专用生产 wrapper。
- [x] P23-03 完成 prompt 文本、来源顺序、hook Part ordering、recall、非法 purpose、全 Runtime standalone/race/static/lint/deadcode/generator 与重复代码门禁；Protocol、Artifact、SQLite、公共 Go API 和 Agent Framework 合同均不变。

### 验收

- provenance purpose 不再由多个调用点手填；来源种类和 purpose 的矛盾不能进入 checkpoint；
- 预算裁剪的 text/source 由同一 prompt fragment 原子产生，不存在“文本被裁掉但来源仍出现”或反向漂移；
- `ComposeWorkingContext` 只协调输入校验、hook 结果应用、system/recall/seed 顺序，具体层行为由各自 owner 完成；
- Runtime workspace build/vet/test/tidy/lint/race 全绿，`GOWORK=off` 的 `agentexec` 定向测试、全模块 staticcheck/deadcode、generator/dupl 无本轮问题；完整 standalone 冷恢复仍等待已完成但尚未发布的 Agent Baseline 19+ Schema 修订进入 `go.mod`。

## 28. P24 — Runtime/Desktop 全链路时序、事务与恢复硬化

### 目标

以真实后端、Desktop 前端和隔离 SQLite 为同一验收系统，覆盖 HITL、Plan、Goal、取消/恢复竞争、冷启动与进程崩溃；发现的问题回到拥有 lifecycle、transaction、read model 或 invalidation 语义的唯一 owner 修复，不以 UI 刷新、延时或兼容旁路掩盖。

### 工作项

- [x] P24-01 Session activity 从 durable non-terminal Run 投影；前端按语义 Session projection 订阅并从 durable running Item 冷 hydration，消除进程内 gate 与页面 reload 造成的状态漂移；
- [x] P24-02 Runtime workspace watcher 只比较 Git HEAD/index 语义指纹并禁用 optional lock；前端按事件精确映射 query scope，消除 `files.changed -> global invalidation -> git stat refresh` 自激 RPC 环；
- [x] P24-03 以 root-owned Segment activation arbiter 串行化 resume/start 与 cancel；终态归约关闭未重启的 drained Tool，避免同一 Run 同时激活/取消或遗留 running Item；
- [x] P24-04 Boot recovery 由 Application 规划完整 Run tree、Goal、cleanup 与全部 open model/tool invocation write-set；Adapter 在一个 SQLite transaction 内应用，Infra 只提供 technical journal iteration/update；
- [x] P24-05 用真实 HITL approve/reject/cancel、Plan set/exit、Goal complete/stop/resume、reload、并发请求和 `kill -9` 执行端到端回归，并完成 Runtime/Agent/Desktop/Frontend 全量质量门禁与边界审计。

### 验收

- cancel/resume/activation 每个边界只有一个线性化结果；Session activity、Run、Item 与页面冷加载读取同一 durable 事实；
- boot recovery 的 lost Run、Goal run、model/tool invocation 和 cleanup 同事务提交并使用同一终结时间；失败注入必须整批回滚；
- Desktop 空闲不产生 `/v2/rpc` 自激请求；Git stat cache 变化不伪装为 workspace change，真实 HEAD/index 变化仍会发布；
- 隔离数据库 integrity 为 `ok`、foreign-key 零违规，终态无 started invocation、非终态 Run、open/resuming interrupt 或 checkpoint；
- Agent production DAG 对 `app/runtime` import 为零，Runtime Framework concrete import 仍只在 `adapter/agentexec`；Protocol、Artifact 与 SQLite shape/epoch 不因本批变化。

## 29. P25 — Runtime/Desktop 第二轮反证式缺陷清零

### 目标

在新的隔离 Runtime、Desktop 与 SQLite 系统中扩大 HITL、Plan、Goal、Run/Tool、重复/乱序、失败注入、断线重连、冷启动和真实崩溃矩阵；所有反例回到拥有 transport、projection、lifecycle 或 Framework contract 的唯一 owner 修复，不以延时、刷新、sanitizer、`replace` 或测试跳过掩盖。

### 工作项

- [x] P25-01 前端 live Run stream 在活动期独占会话投影，durable `runs.changed` snapshot 串行合并并延迟到 stream tail 同步 flush 后应用；取消同时终止旧 stream。Workspace event target 以 `resolved/unavailable` 区分合法默认 workspace 与解析失败，draft cache miss 精确读取 Session，project→default、reconnect backoff 及 opening-retarget race 均立即切换且不发布旧订阅；Run/Item/Plan fold 对 duplicate、late started/delta 与同 revision replay 保持单调，同时保留 HITL Tool `requires-action -> running` 的合法 continuation；
- [x] P25-02 JSON-RPC transport 在 SDK decode 前递归拒绝任意深度 duplicate/unknown object member、空 request method、所有非字符串显式 id、request/response 混合与 result/error 双载荷；HTTP binding 只接受 request/notification 并统一投影 transport problem，SDK 不再折叠 null/number 或执行语义歧义 envelope；
- [x] P25-03 完成 HITL 双提交与真实 ask_user resume、Plan revision、Goal complete/blocked/budget、cancel/resume、幂等 replay/drift、cursor、事务失败、`kill -9`、child process、同 build/异 build恢复及真实 Desktop 无刷新重连回归；
- [x] P25-04 发布已完成的 Agent Baseline 20 canonical source，Runtime `go.mod` 从 Baseline 18 一次性绑定该真实 pseudo-version，并完成 `GOWORK=off` build/vet/staticcheck/lint/test/race 与冷恢复矩阵；禁止本地 `replace` 或 Runtime metadata 降形旁路。

### 验收

- live stream 与 durable snapshot 只有一个线性化投影顺序，terminal tail 不丢失、不重复、不触发状态机冲突；
- 相同 JSON bytes 不会因 first-wins/last-wins decoder 差异路由到不同 method；空 method、client response、混合 envelope 与显式 `null`/numeric id 不再伪装成 notification、被执行或错误关联到另一个 call；
- 隔离数据库 integrity 为 `ok`、foreign-key 零违规，终态无非 terminal Run、open/resuming interrupt、started invocation、Goal run 或 checkpoint；
- Agent production DAG 对 `app/runtime` import 为零，Runtime Framework concrete import 仍只在 `adapter/agentexec`；workspace 与 `GOWORK=off` 最终消费同一 canonical Agent source。

## 30. P26 — Tool execution timing 反证修复

### 目标

修复真实 HITL dogfood 发现的 Tool 卡片把审批等待计入执行时长的问题。可见 Item lifecycle 与真实 executor 活动区间必须由各自 owner 表达；unknown execution 不得用 wall time 猜测，Delivery 与前端不得跨层读取 invocation journal 纠偏。

### 工作项

- [x] P26-01 Run Reducer 以已拥有的 attempt start/finish 计算唯一 exact execution duration，Transcript terminal fact 持有可选值；恢复不能证明区间时保持 unknown；
- [x] P26-02 SQLite transcript codec 精确 round-trip execution duration，Delivery 只投影该领域事实；Artifact v17、Protocol registry、生成物、canonical samples 与 Desktop vendored contract 同步；
- [x] P26-03 完成 Domain/Reducer/SQLite/Artifact/Protocol/Frontend 全量门禁，以及带显式审批等待的真实 Runtime/Desktop 回归和数据库对账。

### 验收

- Tool 卡片时长等于真实 executor interval，显式审批等待只增长 lifecycle；
- terminal ToolCall 的 duration 可选、非负且不超过 lifecycle；unknown 不伪造成 `finishedAt - startedAt`；
- Delivery 不读取 Infra journal，Frontend 不二次计算；Agent Framework 不接收 Runtime timing、Store 或产品 DTO；
- Artifact v16 及更早版本在写入前拒绝，v17 exact/unknown duration 均能 round-trip。

## 31. P27 — Runtime/Frontend 依赖信任边界收缩

### 目标

清除发布后依赖审计发现的可复现漏洞版本和不准确依赖边界。Runtime 的 Ollama 能力只需要 OpenAI-compatible chat/embedding 客户端协议，不得为该能力引入 Ollama 服务端仓库；Frontend 锁文件必须解析到已修复版本，不能依赖开发机残留的另一套安装结果掩盖风险。

### 工作项

- [x] P27-01 Frontend 只通过 package lock 的正常依赖求解前移 Mermaid、DOMPurify 与 NanoID，完成 clean `npm ci`、完整质量门禁及真实 Mermaid SVG 渲染；
- [x] P27-02 在 Runtime Infra provider composition 内以已有 OpenAI-compatible protocol 构造 Ollama chat/embedding，保留 provider-scoped extension、默认本地 endpoint 与显式 API key，移除完整 Ollama 服务端 module 及其独占依赖；
- [x] P27-03 复跑真实 Desktop/Runtime 的审批 allow/deny、审批等待崩溃恢复、Tool 执行中崩溃、Goal/Plan 恢复、双 Session HITL 隔离、codec fuzz、数据库终态不变量及全部静态/race 门禁。

### 验收

- clean install 后 `npm audit` 为零，真实 Mermaid 内容渲染成功，Frontend test/build/bundle/架构门禁全绿；
- Runtime `govulncheck` 无可达漏洞，依赖图不含 `github.com/ollama/ollama`，Ollama chat/embedding wire 由 Infra 定向回归证明；
- 崩溃恢复不自动重放已开始 Tool，等待审批可继续，Goal 在剩余预算内 resume，两个 Session 的 HITL resolution 不串扰；
- Agent production graph 对 `app/runtime` import 仍为零，Runtime 的 provider、endpoint、Store、Run、HITL 与 transaction 抽象不进入 Agent Framework。

## 32. P28 — Ollama standalone client/daemon 边界纠偏

### 目标

清除 workspace 扩展依赖扫描在独立 `models/ollama` 模块发现的同根漏洞：Client adapter 不得为了两个 HTTP endpoint 依赖完整 daemon repository。修复必须留在 provider module 自己的 client/wire owner，保持 Core chat/embedding 与 Ollama 原生 endpoint 语义，不让 Runtime 或 Agent 承担 provider SDK 细节。

### 工作项

- [x] P28-01 在 `models/ollama` 内建立私有、窄化的 `/api/chat` NDJSON 与 `/api/embed` JSON wire/client，保留 request extension、Tool、多模态、thinking、stream/cancel、HTTPClient、原生响应 extension 与 HTTP 状态错误；
- [x] P28-02 移除 `github.com/ollama/ollama` 及其独占 ordered-map/auth/server 间接依赖，为 daemon module 禁止回流建立 architecture gate；
- [x] P28-03 完成 Core conformance/behavior、未知 provider 字段保留、HTTP contract、build/vet/test/race/staticcheck/golangci-lint 与 standalone `govulncheck`。

### 验收

- `models/ollama` 依赖图不含 daemon repository，standalone 可达漏洞为零；
- Native chat/embedding 仍命中原 `/api/chat`、`/api/embed`，公开构造器和 Core model interface 不变化；
- 非流响应和单帧有显式内存上限，慢流、取消、early stop、首个坏帧与 provider error 都确定收口；
- Runtime/Agent/Application/Domain/Delivery 合同不因 provider client 修复变化。

## 33. P29 — CLI standalone Runtime consumer 发布闭环

### 目标

关闭 workspace overlay 掩盖的 CLI 发布缺口：CLI standalone module 必须消费已推送、已移除 Ollama daemon 依赖的 Runtime canonical pseudo-version，而不是继续固定 P27 之前的旧 Runtime graph。依赖前移不得吞入 CLI 正在进行的功能改动，也不得用本地 `replace` 旁路发布事实。

### 工作项

- [x] P29-01 将 CLI 的 Runtime 直接依赖精确前移到 commit `420f627f131a` 对应 pseudo-version，并同步其 Agent Baseline 20 间接依赖；
- [x] P29-02 通过 standalone tidy 删除旧 `models/ollama`、daemon、easyjson 与 ordered-map 图，保留 CLI 自己新增的真实直接依赖；
- [x] P29-03 完成 `GOWORK=off` tidy-diff/build/vet/test/race/staticcheck/golangci-lint/govulncheck。

### 验收

- CLI standalone module graph 不含 `github.com/ollama/ollama`，可达漏洞为零；
- CLI 消费远端 canonical Runtime/Agent 版本，无 `replace`、workspace-only 假绿或 consumer compatibility shim；
- CLI normal/race 全包、staticcheck/lint 与 build/vet 全绿，Runtime public embedded contract 无漂移；
- 提交只包含依赖发布闭环与台账，不夹带 CLI 功能工作树。

## 34. P30 — Runtime 失效流订阅隔离与断号恢复

### 目标

修复 `runtime.subscribe` 在多连接、不同 topic/watch 集和队列拥塞下的作用域泄漏与过度收窄。每条流只消费自己声明的失效范围；丢帧恢复必须保守覆盖全部未投递事实；Frontend 必须从首帧开始校验连接内序号，不能把连接建立窗口中的丢失当成完整流。

### 工作项

- [x] P30-01 Delivery hub 将 topic 与 client-declared watch scope 同时归入 subscription owner；普通 `files.changed`、显式 `resync` 和无匹配 watch 的事件都在分配 sequence 前按本流声明过滤，非法内部 frame 仍 fail closed 为本订阅全量 resync；
- [x] P30-02 拥塞合并显式区分 targeted watch 与 broad file invalidation；任一 broad 事实支配已有/后续 watch 列表，避免最终 `resync.watchIds` 错误收窄；
- [x] P30-03 Frontend event loop 以 0 为每次新连接的 sequence 基线，首帧非 1、重复、倒退或后续断号都触发 authoritative 全量同步；retarget 后新流重新从 1 验证。

### 验收

- 不同订阅的 topic/watch invalidation 不串流，过滤事件不消耗 sequence；同一 resync 的 topic/watch 交集按协议/订阅声明顺序稳定输出；
- malformed resync 不被猜测性归一化，而是扩大为本订阅声明 topic 的合法全量 resync；队列内 broad+targeted 文件信号按任意顺序合并都保持 broad；
- Frontend 首帧和任意后继帧的断号均触发全量 query 与 mounted Session projection 同步，正常从 1 连续流不增加无关刷新；
- Runtime standalone build/vet/test/race/staticcheck/golangci-lint/tidy 全绿，Frontend 224 files/1381 tests、架构/格式/生产 bundle 全绿；Agent production graph 不获得 Runtime subscription、Delivery 或 watch 抽象。

## 35. P31 — Goal root Run boundary / HITL 状态判定

### 目标

清除 Goal driver 对 whole-tree Run stream 的边界猜测：child `SegmentFinished`、root waiting boundary 与缺失 root boundary 必须按各自权威身份和 Run state 区分。Goal 不得用“有没有任意 finished frame”推断 HITL，也不得把 stream contract failure 伪装成 awaiting input。

### 工作项

- [x] P31-01 Goal 只消费 `StartResult.RunID` 对应的 root `SegmentFinished`，child Run 的合法终结帧不参与 owning Goal lifecycle；
- [x] P31-02 root `run.Waiting` 明确落为 `awaitingInput` 且不计 completed Run budget；空流或仅 child boundary 明确落为 `terminalOutcomeMissing`；
- [x] P31-03 测试 fixture 补齐真实 `runs.Event.RunID` envelope，并覆盖 waiting root、missing root、foreign child waiting 与 malformed running boundary。

### 验收

- whole-tree stream 的 child boundary 不能暂停、完成或阻塞 root Goal；root waiting 由 durable Run state 而非 frame 缺席证明；
- 缺失 root boundary fail closed 为 contract failure，不产生虚假的 open interrupt/HITL 语义，不消耗 Goal Run usage；
- Runtime standalone build/vet/test/race/staticcheck/golangci-lint/tidy 全绿；Agent production graph 对 `app/runtime` import 仍为零；修复只位于 Goal Application owner 与其测试，不修改 Agent、Delivery、Infra、Protocol 或 Desktop。

## 36. P32 — Goal objective incarnation / HITL Resume accounting

### 目标

清除 Goal durable identity 与 process-local drive ownership 的概念混叠。暂停等待输入再恢复时，outstanding Run 仍属于同一个 objective incarnation；它的 terminal accounting/终态必须能够作用于原 Goal，等待中的新 drive 不得从陈旧快照额外启动 Run。

### 工作项

- [x] P32-01 将 Goal/Run/Pending/Interrupt/execution scope/checkpoint provenance 统一为 `IncarnationID`/`GoalIncarnationID`；fresh `Start` 才创建新 incarnation，Pause/Resume/Stop/Reconcile 均保留当前 objective identity；
- [x] P32-02 Goal driver 在每次 `WaitSessionStartable` 返回后、Run admission 前重读并结算权威 Goal；budget 已耗尽、模型已完成/阻塞、目标已暂停或被 fresh objective 取代时不启动额外 Run；
- [x] P32-03 SQLite 直接提升至 epoch 68，采用 `incarnation_id`/`goal_incarnation_id`；executor checkpoint policy 提升至 v2，旧 lease 列/字段与 v1 codec 均确定性拒绝；
- [x] P32-04 补齐 outstanding HITL Run 预算计费、模型完成报告、schema exact-shape 与 retired-shape 反例，并同步 Domain/Tool/Contract/Capability/ADR owners。

### 验收

- waiting Run 跨客户端 Resume 后仍能向同一 objective incarnation 提交 terminal accounting 与 completed/blocked outcome；fresh Start 仍隔离旧 Run；
- session startable 等待不是 reservation，等待返回后的权威 Goal 已 blocked/complete/paused/superseded 时 Run start 次数保持为零；
- Runtime standalone build/vet/test/race/staticcheck/lint/tidy 全绿，旧 Goal lease vocabulary 在 production/schema/checkpoint 当前 shape 中为零；Agent production graph 对 Runtime import 为零，Runtime 的 Framework import 仍只存在于 `adapter/agentexec`。

## 37. P33 — Schedule 默认工作区更新合同与 Desktop 消费闭环

### 目标

让 Schedule 的 workspace 部分更新完整表达保持、设置和回到 Runtime 默认三态，并保证 Desktop SDK 与产品入口直接消费 Runtime 生成合同，不再用非法空 `WorkspaceRef` 猜测清空语义。

### 工作项

- [x] P33-01 `UpdateScheduleRequest` 新增 closed `workspaceMode: "default"`，省略保持、合法 `workspace` 设置、default 清空，生成合同禁止同时出现后两者；
- [x] P33-02 Delivery 只把协议动作投影到既有 `schedule.Patch.CWD`，Domain/Application 的空 CWD 默认语义和 workspace admission owner 保持不变；
- [x] P33-03 Desktop handwritten SDK 直接消费生成的 `UpdateScheduleRequest`，Schedule gateway 对空工作目录发送 default mode，对显式目录发送合法 ref；
- [x] P33-04 补齐 wire、Delivery、SDK、gateway、真实 HTTP lifecycle 与浏览器/SQLite 回归，并同步生成制品和合同摘要基线。

### 验收

- 编辑默认工作区 Schedule 可持久化其他字段；显式绑定可回到 Runtime 默认，空路径在任何分支都不是清空语义；
- Runtime 生成器、Go/JSON Schema/TypeScript validator、Desktop SDK 与产品调用点使用同一 request shape；
- 变更不进入 Agent Framework、Domain 或 Application 协议层，不修改 SQLite shape，也不触碰并存的 CLI 工作。

## 38. P34 — Goal HITL capability、权威 Question answer 与 Desktop Markdown 收口

### 目标

消除多客户端 HITL 最终答案、Goal 内 Runtime 能力和 Markdown raw HTML 三处第二真相源，使 accepted response、自治 Run admission 与 UI 解析分别回到 Transcript、Goal/Application 和 Desktop Markdown owner。

### 工作项

- [x] P34-01 Question Transcript 不可变保存唯一 accepted answers；Application 从 Pending + resolution 形成 replacement，resume claim、Transcript replacement 与 checkpoint/Pending 更新在同一事务内提交；
- [x] P34-02 Protocol/Artifact v18/SQLite codec/Delivery/Desktop generated consumer 原子同步，前端 settled card 只把本地草稿作为短暂延迟桥，Runtime 投影到达后以 accepted answer 或未回答关闭态为准；
- [x] P34-03 Goal fresh Start 冻结协商后的 canonical Run capabilities，SQLite epoch 69 持久化，Resume 验证 caller 覆盖，自治 Run 和 Goal 内 `create_goal` 继承该集合；
- [x] P34-04 `WaitSessionStartable` 同时观察 process-local admission 与 durable non-terminal Run，以 committed lifecycle signal 唤醒重读，允许先恢复 Goal drive 再回答其 owned parked Run；
- [x] P34-05 Desktop 在 Markdown AST 边界把不支持/危险 raw HTML 保留为字面量，安全 allowlist 仍由前端 owner 维护；补齐 `<chosen>`、支持标签和 `<script>` 回归；
- [x] P34-06 双客户端 opposite answer、取消、reload、Goal ask_user、真实 HTTP resume、事务 rollback、生成合同和 API consumer 覆盖形成永久回归。

### 验收

- 并发 Question 只有一个 claim winner，所有客户端最终显示同一 Runtime accepted answer；取消不把本地草稿升级为回答，重放保持一致；
- Goal 只使用客户端真实承诺的 capabilities，HITL waiting/resume 不扩权、不降级、不启动额外 Run；
- `agent` module 不接收 Runtime Goal/Run/capability/Store/transaction/UI 抽象，Framework concrete import 仍只存在于 `adapter/agentexec`；
- Runtime standalone 与 race、contract digest、Desktop 完整 check、86/86 operations + 10/10 events、真实 HTTP/崩溃恢复和 SQLite 终态不变量全绿。

## 39. P35 — Run 订阅终态收敛与观察生命周期

### 目标

消除 durable snapshot 与 stream subscribe 之间的终态竞态、Run 已接受后跟随失败被误判为 start/resume 失败，以及 process-local Session 变更观察永久保留状态三处同源 lifecycle ownership 缺口。

### 工作项

- [x] P35-01 Desktop cold recovery 在 snapshot 后 subscribe 得到 terminal/waiting/stale 时立即重读 durable Session projection，不保留幽灵 Running；
- [x] P35-02 replay/cold reattach 统一把不可附着 Run 收敛到 durable projection，teardown 后的异步 refresh 通过 CAS commit guard 失效；
- [x] P35-03 `runs.start/resume` ack 成为明确 accepted boundary，ack 后 stream/recovery failure 不再进入 command error 或 HITL `onStartError` 回滚通道；
- [x] P35-04 Runtime Session Run change fan-out 只在存在活跃 waiter 时保留 generation，使用引用计数与幂等 disposer 支持多观察者、取消、通知和代际切换；
- [x] P35-05 补齐前端红测、Runtime 多观察者/取消/无观察者回归、重复 race、全量质量门禁与真实浏览器双客户端复核。

### 验收

- Run 在 snapshot 与 subscribe 之间进入 finished/waiting/stale 时，Desktop 最终只显示 Runtime durable truth，不打印伪 reattach failure；
- 已返回 Run ack 的 HITL answer/resume 不因后续 stream 故障被本地回滚，只有 ack 前拒绝才进入 start-error 通道；
- process-local wake signal 不保存产品状态、无 waiter 时零 Session 条目，多 waiter 同代唤醒且旧代不污染新代；
- 改动止于 Runtime Application wake mechanism 与 Desktop Agent adapter/application port 边界，不修改 Agent Framework、Protocol、Artifact 或 SQLite shape。

## 40. P36 — Desktop 旁路事件目标解析恢复

### 目标

消除 `runtime.subscribe` 全局 topic 流仍在线、但 active Session 工作区解析遇到瞬时 RPC 故障后文件 watch 永久停留在 `none` 的静默失联；保持 Session 语义、订阅 lifecycle 与 wire 错误各自位于原 owner。

### 工作项

- [x] P36-01 Session workspace adapter 只把权威 `session_not_found` 投影为 unavailable，不再吞掉网络、transport 或 protocol 故障；
- [x] P36-02 workspace event application owner 对瞬时解析失败做有上限的指数退避，并在同一 identity 上自主恢复，不等待偶然的 Session/query 变化；
- [x] P36-03 active Session retarget 与 plugin dispose 都取消旧解析和 backoff，旧 identity 不得重装 file watch；
- [x] P36-04 补齐失败语义、同 identity 恢复、identity 切换和 dispose 回归，并通过完整 Desktop 架构/API consumer/生产 bundle 门禁与真实 Git-state 文件事件联调。

### 验收

- 瞬时 `sessions.get` 故障不会被误写成业务 unavailable，恢复后同一 Session 自动重建精确 workspace watch；
- 权威 Session 缺失继续 fail closed 为无 file watch，同时 app-wide topics 保持在线；
- 重试、取消和日志属于 Workspace event application/composition，wire 错误识别止于 adapter；没有把 Runtime DTO、subscription 或重连策略泄露进 Agent context；
- Desktop 全量 test、type/lint/format/knip、限界上下文/层/循环/port/API consumer、设计系统、本地化、bootstrap 和 production bundle 全绿。

## 41. P37 — Goal 完成窗口的事件/读取/产品状态闭环

### 目标

消除模型已声明 Goal 完成、owning Run 尚在最终结算时 `goals.changed` 导向不可读取状态的合同断裂；由准确层分别拥有 Domain terminal fact、Application settlement、Delivery read projection 与 Desktop 产品行为。

### 工作项

- [x] P37-01 以 Delivery 红测证明 Domain `complete` 是合法可读取快照，删除“客户端观察前必然清除”的错误协议假设；
- [x] P37-02 公共 Goal read model 新增 `completing`，生成 Go/Schema/TypeScript 合同并同步 Desktop vendored binding，不修改 Domain 状态机或持久化 shape；
- [x] P37-03 Desktop Goal context 在自有 read model 中消费新状态，banner 保持目标占位、本地化显示收尾且不暴露 stop/resume；launcher 因权威 Goal 仍存在而保持关闭；
- [x] P37-04 真实 HTTP Runtime 在 `report_goal_outcome(completed)` 后卡住下一模型边界，稳定验证 `goals.changed → goals.get(completing) → final null`，并完成全量门禁、边界扫描与浏览器复核。

### 验收

- 任何已发布的 `goals.changed` 都只把消费者引向合法 `goals.get` 结果，不以 RPC error、active 伪装或 premature null 隐藏 settlement；
- `completing` 期间 UI 不开放 stop/resume/start，最终清除后由同一 query invalidation owner 收敛；
- Domain `complete`、Application drive、Delivery `completing` 与 Desktop context 各自持有本层词汇，Agent Framework 不认识 Goal/Run/Store/Protocol；
- Protocol `2026-08-12`、Artifact v18 与 SQLite epoch 69 以准确爆炸半径保持，Runtime/Desktop 全门禁、真实 HTTP 与浏览器场景全绿。

## 42. P38 — Desktop 失效 Session 恢复的权威缺失语义

### 目标

消除历史深链接指向已删除 Session 时，前端已正确回到新会话但 recovery 仍把权威缺失当作运行故障上报的语义错位；保持 wire error、Adapter 翻译和 Application projection 各自的抽象边界。

### 工作项

- [x] P38-01 Runtime Gateway adapter 只把 `session_not_found` 翻译为 `AgentSessionSnapshot | null`，其他 RPC/transport 错误继续原样失败；
- [x] P38-02 projection refresh 将 absent snapshot 视为合法权威结果，不解引用、不改写旧 projection；history action 以 `false` 表达目标不存在；
- [x] P38-03 以 adapter/application 红测覆盖缺失翻译与旧投影保留，并通过完整 Desktop 质量门禁和真实 stale URL 浏览器复验。

### 验收

- 不存在的 Session 深链接自动收敛到新会话且不打印 recovery failure；真实运行故障仍保持可观测；
- Application port 只表达可选会话快照，不识别 wire error；Adapter 不拥有导航或 projection lifecycle；
- Runtime Protocol/Artifact/SQLite 与 Agent Framework 零变更，前端 Agent bounded context 不 import Runtime Domain/Application 类型。

## 43. P39 — Desktop 多客户端 Session 删除与导航收敛

### 目标

消除启动后另一客户端删除当前 Session 时，列表已刷新但 active/open 导航因一次性对账门闩而永久停留幽灵会话的生命周期断裂；同时防止本地乐观 mutation 把未提交或已并发失效的快照冒充权威身份集合。

### 工作项

- [x] P39-01 将“恢复上次 Session”保持为一次性启动动作，而 active/open reconciliation 对每次成功 `sessions.list` 读执行；
- [x] P39-02 Session delete 在 Runtime commit 前不修改列表身份，Application Port 的完成语义为“权威不存在”，Adapter 只把 `session_not_found` 翻译为幂等成功；
- [x] P39-03 rename/favorite 乐观字段 mutation 失败时先恢复局部快照、再重读 Runtime，不让 cancelQueries 吞掉并发 delete/update 事实；
- [x] P39-04 补齐跨客户端删除、delete commit boundary、already-absent 和 summary mutation 竞态红测，并以隔离 Runtime + 真实浏览器 + 第二 JSON-RPC 客户端复验。

### 验收

- 另一客户端删除当前 idle 或 parked/HITL Session 后，Desktop 以 `sessions.changed → sessions.list` 收敛列表、open set、active URL、Agent view 和 composer lifecycle；
- delete 失败不提前导航，already absent 不变成假错误，rename/favorite 失败不复活已删除 Session；
- wire error 识别止于 Adapter，Application 只消费权威身份/快照语义；Runtime、Protocol、SQLite 和 Agent Framework 零变更。

## 44. P40 — Desktop 深链 Session lifecycle 与条件写收敛

### 目标

消除合法 URL 深链或浏览器历史直接挂载 Session 时，location 已 active 但 held-open lifecycle 尚未建立，随后 stale open-id 对账会删除当前 Agent/composer material state 的断裂；同时让 Session relocation 的条件写在 revision 冲突或响应丢失后回到权威 cwd/revision。

### 工作项

- [x] P40-01 mounted Session driver 在 material view 建立前持有 open/last-session memory，使直接 location 导航与显式 select 共享同一生命周期不变量；
- [x] P40-02 selection model 在权威 live/draft 集合中保留 active Session，close/reconcile 同步纠正 cold-start seed，不在下次启动重放已证明失效的 identity；
- [x] P40-03 relocate 条件写失败由 Session query owner 重新读取权威列表，Shell banner 不识别 wire error、不持有缓存恢复策略；
- [x] P40-04 以模型/Adapter/driver 红测和隔离 Runtime 真实深链浏览器场景验证 stale open-id 清理后历史、composer 与新 Run 均可用。

### 验收

- direct URL/history 的存活 active Session 始终 held-open，不会被无关 open-set cleanup 删除 material Agent/composer state；
- 关闭/权威移除 Session 后 URL、open set 与 last-session memory 指向同一存活邻居或共同为空；
- relocation 的 revision conflict、missing 或 ambiguous transport failure 都触发 Session truth 回读；Runtime/Protocol/SQLite 与 Agent Framework 零变更。

## 45. P41 — Desktop HITL 多客户端 resume 与 Goal 命令结算收敛

### 目标

消除本地 HITL resume 响应延迟时被另一客户端抢先消费后，standing projection 已收敛但本地 staged 状态与迟到 rejection 又把界面反转为失败的竞态；同时确保 Goal lifecycle 命令在响应丢失或并发 revision 拒绝后回到权威读模型。

### 工作项

- [x] P41-01 submitting HITL batch 也参与 standing projection 对账；其响应项消失时幂等释放全部 card-local staged 状态并记录 superseded opening；
- [x] P41-02 Run opening Adapter 只消费 Application 返回的中性 supersession 事实，不识别 HITL operation 或 wire error；迟到本地 ack 不覆盖已物化的远端结果；
- [x] P41-03 Goal start/stop/resume 统一在成功和结算不明后失效自身 query，回读失败不得替换原命令错误或产生未处理 rejection；
- [x] P41-04 以红测和隔离 Runtime、延迟代理、真实浏览器、第二 RPC 客户端复现 remote-winner 竞态。

### 验收

- 远端客户端先消费同一 HITL set 后，本地 approval/question 卡片和 staged 标记收敛，延迟 `runs.resume` 拒绝不产生 command banner、console error 或结果反转；
- 本地 opening 正常失败仍进入既有错误通道，非 HITL operation 不被协议特例吞掉；
- Goal lifecycle 成功、响应丢失、并发拒绝和回读失败均保持唯一命令错误与可重读 projection；Runtime、Protocol、SQLite 与 Agent Framework 零变更。

## 46. P42 — Desktop command replay 与 draft 冷启动所有权收敛

### 目标

消除生成客户端虽为全部 command 提供稳定幂等键、产品层却从不消费 replay 能力，导致 Runtime 已提交而响应丢失时仍显示失败、创建重复/不可见资源的全局断裂；同时让 Session draft 的 provisional ownership 在冷启动、跨客户端使用与远端删除后保持正确。

### 工作项

- [x] P42-01 RPC mutation settlement 统一从生成 method policy 进入：settlement-unknown transport failure 同 key 有界重放，typed in-progress 按服务端最早时机有界等待，definitive refusal/cancellation 立即返回；
- [x] P42-02 streaming command 每次重放创建独立 event stream，失败 attempt 在下一次 opening 前释放订阅、buffer 与 stream-owned signal；
- [x] P42-03 Session create 的 unary attempt deadline 从 Application 移到 Runtime Adapter，重试只换 delivery signal、不换 logical mutation identity；
- [x] P42-04 draft ownership 跨冷启动持久化，freshness 与 pending input 保持 ephemeral；冷启动读取 durable projection 后只以权威消息事实毕业，导航只让 same-process fresh create 暂时补充 Session membership；
- [x] P42-05 以 SDK/Adapter/Store/driver/selection 红测和隔离 Runtime、故障代理、真实浏览器、SQLite 对账验证 commit 后响应损坏与 reload。

### 验收

- 40 个生成 command policy 共享一个 SDK settlement owner；所有产品 adapter 无需理解 Idempotency-Key 或逐命令复制 retry；
- `sessions.create` 首响应在 commit 后损坏时，两次 attempt 使用同 key，Runtime/SQLite 只产生一个 Session；Run opening 重放不泄漏旧流；
- 空 draft 重载后仍隐藏且保持 URL/composer，权威历史使其毕业，权威 Session 缺失使其导航/owner 被清理；Runtime、Protocol、SQLite 与 Agent Framework 零变更。

## 47. P43 — Desktop Run opening / cancel 结算收敛

### 目标

消除 Run opening 响应永久悬挂、握手 deadline 误杀已接受长流，以及 cancel 迟到响应或失败反转较新权威终态；重试、传输、投影与产品状态仍各守原抽象层。

### 工作项

- [x] P43-01 RPC Agent Adapter 将 opening attempt deadline 与 accepted event stream lifetime 拆分；首个 timeout 对原 MutationPromise 使用 fresh signal 重试，第二个 timeout 有限返回；
- [x] P43-02 winning attempt 在 ack 后清除 deadline，但继续由外层 session signal 控制 stream；失败 attempt 在重放前完整释放；
- [x] P43-03 cancel controller 以发起时 material revision 条件提交 command snapshot，任何新 live/query projection 都能拒绝迟到回滚；
- [x] P43-04 cancel failure 通过 Application 权威 Session projection 重读 terminal 中性事实；远端胜出时收敛，重读失败时保留原命令错误；
- [x] P43-05 transport failure 由 Agent Adapter 投影为稳定产品 problem，Application 不接触 transport；以隔离 Runtime、故障代理、假 provider、第二客户端和真实浏览器验证。

### 验收

- opening 首响应悬挂后以同 key、fresh signal 有界重放，Runtime/SQLite 仍只有一个 Run 与一份消息；
- accepted stream 超过 30 秒继续运行且仍受 session owner 取消；cancel 迟到响应不能覆盖更新的终态；
- 86/86 Runtime operations、10/10 event types 和 103 个 typed call sites 继续完整消费；Runtime、Protocol、SQLite 与 Agent Framework 零变更。

## 48. P44 — Desktop Session 历史重写与条件写收敛

### 目标

完整消费 `sessions.rollback` 的提交结果，消除历史重写绕过 mounted projection owner、第一轮文件恢复误删对话、重复破坏动作与本地 Session 条件写互相制造 stale revision 的问题，同时保持 wire、Application、Store 与 Agent Framework 边界。

### 工作项

- [x] P44-01 rollback 通过 mounted Session 的唯一 stream/snapshot synchronization owner 等待权威 commit，失败/竞争有界重试，dispose 释放所有等待者；
- [x] P44-02 同一 Session 只允许一个 destructive history rewrite，重复 UI 动作不再双重截断或执行两次 resend/prefill；
- [x] P44-03 Runtime Adapter 消费 `RollbackSessionResponse.droppedRuns.userInput` 并投影中性 AgentInput，edit/regenerate 使用后端提交事实；
- [x] P44-04 第一轮缺少 checkpoint 时 files/both fail closed，不把省略 `restoreType` 的默认 history 误当文件恢复；
- [x] P44-05 rename/favorite/relocate 条件写按 Session 串行，成功 revision 传给后续本地命令，失败只条件回滚自身字段并回读权威列表。

### 验收

- files-only 第一轮无 rollback command/replay record，Run/Item/Message 原样保留；
- 两轮 edit-and-rerun 后只保留首轮，composer 使用 Runtime dropped input；reload 与 fork 后 Session/Run/Item/Message 精确；
- 86/86 Runtime operations、10/10 event types 和 103 typed call sites 继续完整消费；Application 无 wire/transport/idempotency，Agent Framework 无 Runtime import。

## 49. P45 — Desktop Goal/Plan lifecycle、旁路失效与抽象边界收敛

### 目标

让 Runtime 已提供的 Goal replacement 与预算语义在 Desktop 可达，消除 Goal lifecycle 命令跨 Session、乱序和永久等待；同时把 Plan wire projection 与 fold composition 收回 Agent Adapter/bootstrap，防止 Plan 的 app/runtime 协议抽象进入 Agent Application/domain/public surface。

### 工作项

- [x] P45-01 Goal launcher 按 Session identity 隔离 draft 与 async completion；active/completing 禁止替换，paused/blocked 可原位启动新 Goal；预算耗尽的 blocked Goal 不提供必然失败的 Resume；
- [x] P45-02 Goal start/stop/resume 返回中性 GoalReadModel，按 Session 串行并以 `updatedAt` 单调提交；旧命令响应不能覆盖较新自治状态，成功/失败后均由 Goal query owner 重读；
- [x] P45-03 通用 RPC unary mutation settlement 提供两次有界、同 mutation identity 的 delivery attempt；Session create 与三个 Goal command 复用，不向 Application 暴露 MutationPromise、signal 或 idempotency；
- [x] P45-04 Goal Adapter 独占 wire mapping 和 data provider 注册，defaults 不再拥有 Goal；`goals.changed` 只失效事件点名 Session 的 query key；
- [x] P45-05 Runtime Plan snapshot/event 在 Agent Adapter 映射为中性 Plan domain，Application fold/view 不消费 Runtime DTO；fold plugin 移入 bootstrap，public surface 不发布 composition mechanism。

### 验收

- 隔离 Runtime/真实浏览器证明 Plan live projection、预算 1 Run 后 `runBudgetReached`、blocked Goal replacement、冷重载和 SQLite 权威状态一致，无页面错误；
- 253 个 Frontend 测试文件/1565 个用例与 typecheck/lint/format/knip/循环/Context/发布边界/层级/port/bundle 全绿；
- 86/86 Runtime operations、10/10 event types、103 typed call sites 保持完整消费；Runtime、Protocol、SQLite 与 Agent Framework 零变更。

## 50. P46 — Desktop Agent Runtime DTO 防腐与 Composer 冷启动模型收敛

### 目标

关闭 Agent Application、Domain、public surface 与 SDK event contract 对 Runtime wire DTO 的剩余依赖，让 live event、durable snapshot、cancel response 与 install-wide pending work 全部经 Agent Adapter 映射为中性产品事实；同时消除 Composer 冷启动时 catalog 默认模型抢先覆盖 durable Session 模型的竞态。

### 工作项

- [x] P46-01 SDK 定义中性 Agent event/item/run/interrupt facts，Agent Adapter 统一校验并映射 Runtime live event、durable snapshot 与 cancel response；Application fold/view 只消费中性事实；
- [x] P46-02 install-wide pending work provider 从 defaults 移入 Agent Adapter/bootstrap，provider 在边界完成 wire→Agent read model 转换，公共 HITL surface 不再发布装配细节；
- [x] P46-03 发布边界门禁禁止 Agent Application/Domain/public 与 SDK Agent event contracts 在生产代码导入 `@/rpc`，防止 Runtime DTO 再次向内层扩散；
- [x] P46-04 Composer 模型选择按“有效显式偏好 → active durable Session model → catalog fallback”解析；active Session summary 未决时不物化 catalog 默认值，避免冷启动竞态制造错误 override；
- [x] P46-05 visual fixture 也从真实 wire fixture 经 Agent Adapter 进入中性 projection，不建立仅测试环境可见的旁路抽象。

### 验收

- 隔离 Runtime/真实浏览器创建两步 Plan 并完成两次 Run；冷重载前后 Composer 均恢复 durable `deepseek-v4-pro`，第二次 Run 的 `usage.byModel` 仍只有该模型；
- 256 个 Frontend 测试文件/1582 个用例与 typecheck/lint/format/knip/循环/Context/发布边界/层级/port/API consumer/style/design/token/chrome/locales/bootstrap/bundle 全绿；
- 281 个视觉、交互、WCAG、Retina 与 WebKit 场景全绿；active Goal 的 Stop 能力和当前工具目录 surface 已与明暗主题 golden 对齐；
- Runtime `go test ./...` 全绿；86/86 Runtime operations、10/10 event types、103 typed call sites 保持完整消费；Runtime、Protocol、SQLite 与 Agent Framework 零变更。

## 51. P47 — Workspace 订阅 opening 取消与 Runtime resync fail-closed

### 目标

消除 Workspace 切换或销毁时旧 watch-root resolution 阻塞唯一重连循环的问题，并让 Runtime Delivery 对跨字段矛盾的 resync 保持 fail-closed；取消、订阅 scope 与 Agent 投影继续由各自 owner 独占。

### 工作项

- [x] P47-01 `workspaces.resolve` SDK 方法接受调用方 lifecycle signal，Workspace Runtime Adapter 将 subscription signal 透传到 watch-root resolution；
- [x] P47-02 Workspace event loop 以反例证明 opening 中的旧订阅在 retarget 时先取消，随后立即建立新 target，不等待旧请求自然返回；
- [x] P47-03 Runtime Delivery 识别 `watchIds` 存在但 topics 不含 `files.changed` 的跨字段矛盾，不猜测生产者意图，扩大为完整 subscription resync；
- [x] P47-04 以真实 Runtime/浏览器覆盖 HITL approve/reject、两步 Plan、Goal 自主回合、旁路 Session 创建、Git 文件事件和 Runtime 重启恢复。

### 验收

- close/retarget 的取消从 Workspace Application lifecycle 经 Adapter/SDK 到 HTTP request 完整传播，旧 opening 不再阻塞新工作区订阅；
- malformed resync 在 Delivery subscription scope 内 fail closed，Domain、Application 与 Agent Framework 不接触 watch/protocol 校验；
- Frontend 全量门禁 256 个测试文件/1585 个用例全绿，API consumer 保持 86/86 operations、10/10 events、103 typed call sites；Runtime standalone tidy/build/vet/test 与目标 race 重复测试全绿。

## 52. P48 — Desktop API 返回事实与身份解析生命周期消费闭环

### 目标

消除 Workspace 订阅只丢弃旧会话身份解析结果、却没有取消底层读取的生命周期缺口；同时让 Schedule 产品真正消费 `runNow` 与条件更新返回的权威身份和 revision，而不是把已接入的后端 API 降格成仅等待成功的按钮。

### 工作项

- [x] P48-01 Workspace Application 将每代 identity resolution 的 lifecycle signal 交给 consumer-owned port，retarget/dispose 会终止仍在执行的读取且不把取消报告成产品错误；
- [x] P48-02 Workspace Adapter 只用 Agent 公开的中性 Session 身份/投影解析 cwd，并将 signal 交给 SDK `sessions.get`；SDK 独占 transport cancellation，Agent 不感知 Workspace/Runtime subscription；
- [x] P48-03 Schedule Adapter 保留 `schedules.runNow` 返回的 Session/Run identity，Schedule Application 经 Agent 公开会话动作打开目标 Session，Agent 仍通过自己的 durable recovery 接管具体 Run stream；
- [x] P48-04 Schedule enablement 消费后端返回的完整 Schedule 和新 revision，先提交权威 query fact 再失效重验，避免连续条件写继续使用旧 revision；
- [x] P48-05 类型级审计所有非测试 Methods consumer，区分只消费完成语义的 void command 与携带后续唯一身份/条件写版本的响应，不以 `_result` 一类伪消费满足门禁。

### 验收

- 三层取消回归分别证明 Application、Adapter、SDK 的 signal 传播，retarget/dispose 后旧 `sessions.get` 不再成为孤儿请求；
- 隔离 Runtime/真实浏览器从设置页创建 Schedule，`runNow` 后自动打开返回的 Session 并完成目标 Run；连续关闭/开启无 revision conflict，SQLite Schedule revision 单调且唯一 Run terminal completed；
- Frontend 全量门禁 257 个测试文件/1591 个用例全绿，86/86 Runtime operations、10/10 event types、103 typed call sites 保持完整消费；Runtime standalone tidy/build/vet/test 全绿；Protocol、SQLite shape 与 Agent Framework 零变更，Agent 内层没有新增 Runtime/Workspace 抽象。

## 53. P49 — Desktop Mutation 权威事实、并发顺序与上下文归属收敛

### 目标

系统性消除 Desktop 对非 void Runtime command 只等待完成、随后依赖异步重读的资源分叉；让 Provider、MCP、Approval、Agent Memory 与 Codebase 各自在所属 Application 上下文提交返回事实并串行化有冲突的写操作，同时把 Runtime wire 映射严格留在对应 Adapter。

### 工作项

- [x] P49-01 SDK API consumer 门禁识别直接丢弃及显式 `void` 丢弃非 void command 结果，自测固定 Promise<void> 合法反例；
- [x] P49-02 Provider configuration/role、Approval mode、MCP server、Agent Memory 与 Codebase reindex Adapter 返回中性产品事实，Application 先提交权威资源再失效重验；
- [x] P49-03 用可恢复 serial queue 按真实资源冲突域线性化写操作，覆盖连续成功、首个失败、同 key/跨 key、update→delete 与快速 toggle；
- [x] P49-04 MCP data provider 与 wire projection 回归 MCP context Adapter，defaults 不再拥有 MCP 读取或映射；Provider 的 `requiresBaseUrl` 成为产品模型并驱动必填校验；
- [x] P49-05 Provider UI 成功写入后从 Runtime 返回资源重建草稿，消除 endpoint 规范化后仍保持 dirty 的前后端事实分叉。

### 验收

- Frontend 完整门禁 269 个测试文件/1621 个用例全绿，95 条公开 Context edge 无环，发布边界、层级、port、API consumer、8 语言 981 keys 与 production bundle 全部通过；
- API consumer 保持 86/86 Runtime operations、10/10 events、103 typed call sites；Runtime standalone tidy/build/vet/test 全绿；
- 隔离 Runtime/真实浏览器验证 Approval 快速连点、MCP 创建/失败/三连切换/删除、Provider endpoint 必填与规范化回写、Agent Memory 新增/置顶/编辑/删除、Plan、HITL allow/deny 与 Goal 三轮完成。Runtime、Protocol、SQLite 与 Agent Framework 零变更，Agent 内层没有接触 MCP/Provider/Workspace Runtime DTO。

## 54. P50 — Knowledge CAS 与 Knowledge/Hook 失效闭环

### 目标

消除 Knowledge 无条件覆盖导致的真实丢更新、空 LYRA.md 不可从客户端首次创建，以及 Knowledge/Hook API 写入不经过文件 watch 时多窗口永久陈旧；所有修复留在各自 owner，尤其不把 Runtime/Workspace/Hook 抽象泄露到 Agent。

### 工作项

- [x] P50-01 Knowledge Domain 定义 opaque revision required/conflict 语义，Infra 从精确内容计算 revision，并在同一 Store 临界区内比较后使用同目录临时文件原子替换；
- [x] P50-02 `knowledge.list/get` 返回包括尚未创建空文档在内的可寻址 Entry，`knowledge.update` 强制 `expectedRevision`、返回 committed Entry 并声明 `revision_conflict`；
- [x] P50-03 Application 只在 Knowledge/Hook trust 成功提交后发布中性 invalidation，Delivery 穷尽投影为 `knowledge.changed` / `hooks.changed`，生成 Protocol/Go/TS 合同同步；
- [x] P50-04 Desktop Workspace Adapter 将 revision/冲突映射为中性 Knowledge 模型，编辑器保留飞行中与冲突时的用户草稿，干净 snapshot 跟随事件刷新；
- [x] P50-05 Hook trust 按 project 串行，UI 用同步 latch 禁止旧受控值重复提交；唯一 Workspace event consumer 订阅并消费两个新 topic；
- [x] P50-06 API consumer 门禁覆盖全部 86 个 operation 与 12 个 Runtime event type，真实 HTTP 验证首次创建、三 scope CAS、冲突拒绝、清空可寻址及两类 committed invalidation。

### 验收

- Runtime Domain/Application/Infra/Delivery/embedded 聚焦回归全绿；并发 stale writers 只有一个提交且无 torn content，失败写不发布 invalidation；
- Frontend Knowledge/Hook/Event 聚焦 6 文件/24 测试、typecheck/lint/format 与 API consumer 门禁全绿；真实 HTTP Knowledge 与 Hook trust 两条纵切均通过；
- Runtime standalone tidy/build/vet/test 与 Frontend 270 文件/1627 测试及全部架构、本地化、bundle 门禁全绿。真实浏览器证明首次创建、clean event refresh、dirty conflict/rebase/second-save 和 Hook 同步双击 latch；Agent 与 Agent Framework 零修改、零 Runtime/Knowledge/Hook 抽象新增，SQLite/Artifact shape 不变。

## 55. P51 — Knowledge filesystem identity 与 crash-safe CAS

### 目标

关闭 P50 条件更新在文件系统边界的反例：`LYRA.md` symlink 可越过 semantic scope 读取外部内容，域内 symlink 写入会替换 alias，原子替换会放宽已有权限，多个独立 Runtime 进程仍可同时赢得同一 revision；进程在 publish 前退出还会遗留 staging。修复必须留在 Infra/Application/Delivery 的既有 owner，Agent/Agent Framework 对 physical path、锁与 revision 零感知。

### 工作项

- [x] P51-01 Knowledge Infra 通过唯一 `pathidentity` 解析 scope root 与 document physical identity，越界 symlink fail closed；域内 symlink 的读、CAS、写与来源路径共同指向同一 physical target；
- [x] P51-02 新建中性 `infra/advisorylock` 原语，Unix 使用 physical directory handle、Windows 使用固定 owner thread 的 path-keyed mutex；Bootstrap 单实例 lease 与 Knowledge document CAS 分别消费并保留自己的错误/生命周期语义；
- [x] P51-03 revision compare、权限继承、私有 staging、file fsync、atomic rename 与 parent-directory sync 位于同一跨进程 lease；cold get/list/update 回收严格命名的 crash staging；
- [x] P51-04 Domain 只表达 scope containment failure，Application 翻译为既有 workspace path policy，Delivery 为 knowledge.list/get/update 声明 `path_outside_root`；生成合同与 Desktop wire 同步；
- [x] P51-05 单进程、跨进程、symlink alias、权限、强杀恢复、race、Windows 交叉编译与真实 HTTP case 覆盖完整纵切。

### 验收

- 12 轮、每轮 12 个独立进程持同一 revision 并发写均恰好一个 winner；12 轮真实子进程在 64 MiB staging 写入中被强杀，cold read 保持旧 committed content 并回收 orphan，后续 CAS 成功；
- focused Domain/Application/Infra/Delivery/Bootstrap、race 与 Windows advisorylock/knowledgefile/bootstrap 三包交叉编译全绿；真实 HTTP 验证三条 API 的外链拒绝，以及域内 symlink/0600 physical target 的 get/update/list；
- Runtime/Protocol 文档与机器合同同步；Agent/Agent Framework、SQLite、Artifact shape 均未变化，Desktop 只消费既有中性 `path_outside_root`。

## 56. P52 — 外部人工配置变更与 Runtime stream 收敛

### 目标

让用户或其他进程直接创建、修改、原子替换、删除 Knowledge 与 Hooks 配置时，复用既有 Runtime 专用失效流使全部客户端收敛；路径观察、语义资源和 wire topic 必须分别留在 Infra、Workspace Adapter、Application 与 Delivery，不得进入 Agent。

### 工作项

- [x] P52-01 中性 `infra/fileobservation` 以精确语义路径指纹观察 missing parent、write、rename、remove 与域内 symlink physical target，并提供可等待的幂等关闭；
- [x] P52-02 Workspace Adapter 独占 `LYRA.md` 与 global/project/cwd hooks cascade 的文件布局，向 Application 只暴露 workspace/projectRoot 与语义资源变更；
- [x] P52-03 Knowledge API commit 在发布事件前接受精确 returned path 的新基线，抑制自身 filesystem 回声但不吞同资源其他文档的并发外部编辑；
- [x] P52-04 Application 复用 committed invalidation，Delivery 继续只投影既有 `knowledge.changed` / `hooks.changed`，Protocol 和 Desktop consumer 无新增抽象；
- [x] P52-05 覆盖 create/write/atomic rename/remove、missing parent、symlink target、identity 去重、close join 与真实 HTTP SSE → TypeScript SDK 收敛。

### 验收

- Infra/Application/Adapter/Delivery/Bootstrap/architecture 聚焦、race、Runtime standalone tidy/build/vet/test 全绿；
- 真实 Go Runtime → HTTP SSE → TypeScript SDK 验证 home/projectRoot/cwd Knowledge 与 global/project/cwd Hooks 外部变更逐项发专用事件且冷读收敛；
- Desktop 唯一 consumer 继续覆盖 86/86 operations 与 12/12 events；Agent/Framework、Protocol、SQLite、Artifact shape 均未变化。

## 57. P53 — Desktop Goal mutation/read-model 单一事实源收敛

### 目标

关闭 Goal lifecycle mutation 返回的瞬时快照与 `goals.get` 长期 read model 争夺缓存所有权的缺口。延迟响应可能晚于自治循环或另一客户端的更新；`updatedAt` 在同时间戳、时钟校正和不同 Goal incarnation 下都不能证明事实新旧。修复必须停留在 Desktop Goal context，Runtime wire 只由 Adapter 消费，Agent context 不得接触 Goal wire、query cache 或 Runtime 抽象。

### 工作项

- [x] P53-01 Goal Application 删除 mutation 快照的 timestamp 比较和直接缓存写入，成功、失败与结算不明统一回读 `goals.get`；
- [x] P53-02 Goal command port 收窄为只含 Session correlation 的中性 receipt，完整 Runtime Goal 响应仅在 Adapter 映射，standing read model 只由 data provider 产生；
- [x] P53-03 保留同 Session intent 串行与权威回读 settlement boundary；回执 Session 不一致时 fail closed，并在二次读取失败时保留原 command error；
- [x] P53-04 回归覆盖 delayed/equal-timestamp response、mounted query 权威 refetch、跨 Session 回执、连续 stop/resume、失败恢复与 typed wire projection；
- [x] P53-05 完成 Frontend 全量门禁、Runtime 回归与真实客户端 Goal stop/resume/remote transition 联调。

### 验收

- mutation response 不再是 standing state 的第二事实源；mounted query 在命令结算后只能由 `goals.get` 写入；
- Frontend Goal/Application/Adapter 红测、全量架构/API consumer/bundle 门禁及 Runtime Goal/Run/Plan/HITL 回归全绿；
- 真实客户端验证 Goal stop/resume 与远端状态变化最终一致，Agent/Agent Framework、Runtime/Protocol、SQLite/Artifact shape 零变更。

## 58. P54 — Checkpoint source ownership 与 shadow ignore 收敛

### 目标

关闭 shadow checkpoint 的 backstop ignore 与复制 source index 自相矛盾的缺口：真实仓库合法跟踪的 `build/` 输入进入 shadow index 后，不能被 shadow 自己的 `build/` exclude 再次拒绝；同目录未跟踪生成物仍不得被 checkpoint 接管。修复必须留在 checkpoint Infra 的路径选择/暂存 owner，不在 Run/Goal maintenance 或 Agent 层吞错。

### 工作项

- [x] P54-01 用真实 source Git repo 复现 tracked nested build input 在首次 Snapshot 因 shadow ignore 被 `git add` 拒绝；
- [x] P54-02 明确 `git ls-files --exclude-standard` 是唯一 ownership selection：它保留 source index 已跟踪路径，同时排除 ignored untracked path；
- [x] P54-03 对这份经过类型/大小策略筛选的精确路径列表执行 force-add，避免后续 index update 二次解释 shadow ignore；
- [x] P54-04 回归覆盖 tracked build input 两轮 snapshot/restore、同目录 ignored generated sibling 保持不变、oversize drop、普通 gitignore 与 source-index baseline；
- [x] P54-05 完成 checkpoint race、Runtime standalone、真实 HTTP Plan/HITL/Approval/Goal 矩阵与真实 lynx 工作树 Goal checkpoint 复验。

### 验收

- checkpoint 首次/后续 Snapshot 都保留 source repo 已跟踪且命中 backstop exclude 的文件，ignored untracked 与 oversize 文件仍不进入 shadow tree；
- checkpoint focused 20×、完整包、race、Runtime tidy/build/vet/test 与真实 HTTP 7 个 Plan/HITL/Approval/Goal 场景全绿；
- 真实 lynx 工作树 checkpoint tree 包含已跟踪 Desktop build 配置、排除生成目录并通过 `git fsck`，maintenance 无 ERROR；Runtime/Protocol/SQLite/Artifact/Agent Framework shape 不变。

## 59. P55 — Git 子进程仓库环境所有权收敛

### 目标

关闭 Runtime 嵌入 Git tooling、hook、IDE 或测试进程时继承父进程 `GIT_*` 控制面的缺口。Checkpoint、workspace VCS read 与 Git watcher 明确指定的工作树必须是唯一事实源，不能被外部 index/object/common dir/config/pathspec/replace-ref 环境重定向。修复必须属于中立 Infra Git 进程机制；Application、Delivery 与 Agent 不得感知宿主进程环境。

### 工作项

- [x] P55-01 分别以 `GIT_INDEX_FILE` 与 `GIT_DIR`/`GIT_WORK_TREE` 污染稳定复现 checkpoint 失败、VCS read 误判和 watcher 订阅外部仓库；
- [x] P55-02 新建 `infra/gitprocess` 作为唯一 Git OS-process owner，剥离父进程全部 `GIT_*` 控制面后再安装命令自身显式 override；
- [x] P55-03 checkpoint shadow/source discovery、`infra/git` observation 与 workspace watcher 全部消费该机制，不在三个调用点维护会漂移的局部 denylist；
- [x] P55-04 watcher 将 Git 返回的 per-worktree/common metadata directory 解析为 physical identity，消除 `/var`/`/private/var` 与 symlink alias 的重复 watch；
- [x] P55-05 架构门禁禁止 Runtime production 在 owner 之外直接 `exec.Command("git", ...)`，并完成 focused、race、standalone 与污染环境真实进程复验。

### 验收

- foreign index sentinel 不被 checkpoint 读取或改写，请求 workspace 的 modified file 在 foreign repo routing 下仍被准确投影，watcher 只解析并订阅请求仓库的 physical `.git`；
- Git process、checkpoint、VCS read、workspace watcher、architecture 的完整包与 race 全绿，Runtime standalone tidy/build/vet/test 全绿；
- 生产代码直接 Git subprocess 只剩 `infra/gitprocess`，Application/Delivery/Agent/Agent Framework、Protocol、SQLite 与 Artifact shape 均未变化。

## 60. P56 — HTTP sidecar 合同、Desktop 消费与事件协商收敛

### 目标

关闭核心 RPC 之外的 info/liveness/readiness API 未进入生成合同、产品消费和静态覆盖门禁的缺口，并以真实停机/恢复反例验证其生命周期。所有 HTTP path/status/response/auth 事实必须归 Runtime HTTP Delivery；Desktop Runtime context 只向 Settings 暴露中性服务观察。同时确保事件消费者只请求 discovery 已声明的 topic，不能因新客户端连接旧 Runtime 而让全部文件、Session、Run、Goal、Plan/HITL 旁路失效流离线。

### 工作项

- [x] P56-01 Runtime HTTP Delivery 建立唯一 endpoint registry，同源驱动 handler 注册、auth policy、info 自描述与可生成 contract；
- [x] P56-02 contractgen 生成 HTTP endpoint manifest、sidecar response Schema/TS/validator/reference，兼容性比较覆盖 endpoint 删除、方法/路径/auth/响应与状态变化；
- [x] P56-03 Desktop sidecar SDK 只消费生成 method/path/status/type/validator，API consumer gate 将三条 sidecar 纳入非测试产品调用覆盖；
- [x] P56-04 Desktop Runtime Application 以中性 inspection/controller/port 管理 timeout、并发合并、dispose 与失败重试，HTTP Adapter 做防腐映射，Connection UI 只消费公开状态；
- [x] P56-05 Workspace event Adapter 以客户端 foldable topics 与 Runtime discovery topics 的交集建立订阅，旧 Runtime 不再因新增 topic 拒绝整个 stream；
- [x] P56-06 完成生成漂移、边界、全量前端/Runtime 门禁与真实浏览器停机、恢复、旧 Runtime negotiation 联调。

### 验收

- Runtime endpoint 注册、自描述、生成合同和 Desktop SDK 不存在第二套 path/status/schema；静态门禁覆盖全部 RPC、sidecar 和 Runtime event；
- 后端停机后 UI 有界进入 unavailable，同页重启可回到 ready；旧 Runtime 只接收自己声明的 9 topic，SSE 保持 200；
- HTTP/wire/UI/i18n 不进入 Runtime Domain/Application、Desktop Agent 或 Go Agent Framework。

## 61. P57 — Desktop Runtime 冷启动、断线复检与事件流单 owner 收敛

### 目标

关闭 Desktop 只做一次 `runtime.discover`、sidecar 恢复但能力永远为空的冷启动缺口，并让服务状态同时依赖 HTTP sidecar 与 RPC discovery 的同代事实。Runtime connection supervisor 是重试、deadline、健康巡检和能力撤销的唯一 owner；Workspace 事件消费者只上报流结束这一连接证据。Session 身份变化与同一 Session 投影追平必须分开，不能因缓存填充重复打开相同 `runtime.subscribe`。

### 工作项

- [x] P57-01 Runtime context controller 统一拥有 10 秒 inspection deadline、1/2/4/8/16/30 秒失败退避、30 秒健康巡检、并发合并、dispose 和 late-settlement 抑制；
- [x] P57-02 inspection 同代读取 info/liveness/readiness/discovery，并校验 server/version/protocol/endpoint identity 后原子发布 service 与 capabilities；
- [x] P57-03 Workspace capability 撤销会停止现有 stream，恢复时建立新 generation，旧 generation 不得清除新 iter/retarget ownership；
- [x] P57-04 active Session identity 变化先撤销旧 watch，同一 Session projection 变化保留当前 watch 直到解析出不同 target，且 revision 只观察 active Session cwd；
- [x] P57-05 event stream 异常或正常远端结束经 Runtime 公共 service port 请求静默复检，Workspace 不复制全局连接状态、sidecar 或 discovery 策略；
- [x] P57-06 完成前端全量门禁、Runtime standalone 回归与 fresh browser 冷启动、健康巡检、在线崩溃、自动恢复、单订阅联调。

### 验收

- 页面在 Runtime 离线时启动后无需 reload，后端上线可恢复 ready、产品读取和唯一 event stream；
- 每个 connection generation 恰好一条 `runtime.subscribe`，同 Session 列表投影追平与 30 秒健康巡检均不重启 stream；
- stream 断线立即请求 Runtime supervisor 复检并撤销能力，不等待下一轮健康巡检、不由 Workspace 自行猜测全局状态；
- HTTP/RPC DTO、重试/身份校验不进入 Workspace Application、Desktop Agent 或 Go Agent Framework。

## 62. P58 — 外部配置与后台任务失效闭环

### 目标

关闭第二客户端修改 provider/model role、approval policy、agent memory，或启动 codebase rebuild 后，已挂载 Desktop read model 静默陈旧的缺口。Application 只发布中性“资源已变”事实，Delivery 独占 wire topic，Desktop Workspace events Adapter 只依赖各 context 的公开 query identity；Runtime/transport 抽象不得进入 Agent Application/Domain 或 Go Agent Framework。

### 工作项

- [x] P58-01 Runtime 失效资源闭集新增 models / approvals / agent memory / codebase，并在各 use case 成功提交后发布；失败路径不伪造事件；
- [x] P58-02 codebase rebuild 在 worker 启动前同步发布可读 operation identity，并在 settle 后再次发布，固定 start-before-finish 时序；
- [x] P58-03 Delivery 映射四个专用 topic，Protocol 闭合 union 提升至 `2026-08-13`，生成合同与 Desktop vendored binding 原子同步；
- [x] P58-04 Desktop 订阅全部十六个 topic，并将每个 topic 精确映射到所属 context 的公开 query identity；
- [x] P58-05 完成 Runtime/Frontend 全门禁与真实双客户端 provider/role、approval、agent-memory、codebase 联调。

### 验收

- 成功提交事件、失败不发事件、codebase start/finish 顺序和 resync 闭集均有回归；
- Desktop 已挂载面板在第二客户端 mutation 后无需 reload 自动收敛，codebase 外部重建能进入 indexing 并退出；
- consumer gate 覆盖 86/86 operations、3/3 HTTP sidecars 和 16/16 Runtime events；Agent/Framework 反向 import 与 Runtime wire/DTO 泄露为零。

## 63. P84 — 多个内嵌 Runtime 共享数据库的业务所有权与崩溃接管

### 目标

允许 CLI 与 Desktop 各自内嵌一个 Runtime 并共享用户私有目录中的同一 SQLite 数据库，同时保持 Session/Run tree、Goal autonomous drive、恢复 cleanup 和 physical working-tree destructive mutation 各有唯一 owner。实现不能依赖全目录单实例锁、TTL/heartbeat、兼容宿主或进程内假设；另一个 Runtime 提交后，当前客户端的 HITL、Plan、Goal、Run/Tool read model 必须通过既有事件合同收敛。

### 工作项

- [x] P84-01 将 canonical data-directory lease 缩为 store/schema/config seeding setup boundary，目录强制 `0700`，删除公共 `ErrDataDirectoryInUse`；
- [x] P84-02 由 Application 定义 per-Session writer、Run working-tree shared 与 destructive mutation exclusive、Goal drive single-owner 语义，并由 Runtime ownership Adapter 映射到 OS advisory lease；
- [x] P84-03 Run/Goal recovery 先选举跨进程 sweep winner并固定 Run-before-Goal，再竞争各自业务 lease、重读权威 facts，只清理已接管 Session；Instance 存活期周期重验，进程强杀后由内核释放 lease并允许 survivor 接管；
- [x] P84-04 Persistence 用 connection-local `PRAGMA data_version` 观察其他 Runtime commit，向本 Runtime 发布 scoped full resync；本地提交继续使用精确 invalidation；
- [x] P84-05 以两个真实 embedded Runtime 共享目录/namespace、跨实例事件收敛、跨进程锁竞争/强杀交接、ownership-scoped recovery 与 SQLite rollback 不变量完成验证。

### 验收

- 两个 Runtime 可同时打开同一 canonical data directory；不同 Session 可独立写，同 Session/Goal 只有一个 owner，同 cwd active Run 可并存而 rollback/restore 被排除；
- live peer 的 active Run/Goal 不被另一 Runtime 恢复，owner 进程强杀后 survivor 无需重启即可接管；recovery 不删除未接管 Session 的 checkpoint 或 child reservation；
- peer commit 触发既有 `RuntimeResync` 并点名完整 topic 闭集，消费者重读 durable projection；Protocol wire、SQLite epoch、Artifact/checkpoint 和 Agent Framework 边界不变；
- `app/cli` 未修改或暂存；完成本批独立提交、推送后停止。

## 64. P85 — Desktop 工作目录所有权与右侧 Context Dock 命令接线

### 目标

关闭 Desktop 新建会话只能隐式落到 home、用户无法在创建前选择 CWD 的产品缺口；同时只读对照桌面 Proma 源码中的右侧文件/改动面板，把 Lynx 已有 Context Dock 的显隐接入统一命令与快捷键体系。目录选择属于 Desktop Host 能力，Session/CWD admission 仍由既有 Agent Application 流程拥有；右栏显隐仍由 Workspace Navigation port 独占，禁止在 Shell、RPC 或 Runtime 中建立第二状态。

### 工作项

- [x] P85-01 由 Wails v3 Desktop Host 提供严格的原生目录选择 binding，取消返回空结果，选择结果规范为存在的绝对目录；Host 未配置、dialog 失败、文件误选均 fail closed；
- [x] P85-02 Navigation Application 通过独立 picker port 驱动“选择目录 → 创建 Session `{cwd}` → 聚焦”的单一流程；取消零 mutation，重复点击复用同一在途用户意图，失败明确提示且不退回默认 CWD；
- [x] P85-03 在 Work Index 增加“打开文件夹”入口并覆盖 8 个 locale；Desktop Host generated-name guard、RPC strict validator 与 Browser fallback 同步接线；
- [x] P85-04 对照 Proma 源码确认“附加文件夹”只是既有 Session 的额外访问范围，不冒充 CWD；为 Lynx 现有右侧 Context Dock 增加 `view.toggle-dock` 命令与 `Mod+Shift+B` 快捷键，折叠/重开保留当前 Session 的原标签集合。

### 验收

- 原生 picker 选择、取消、Host 缺失、dialog error、非目录与 RPC malformed result 均有回归；创建请求精确携带所选绝对 CWD，取消/失败不会创建 home Session；
- 一客户端重复点击只产生一次 picker 与一次 Session mutation；右栏 toggle 的折叠/恢复不删除 per-Session tab identity；
- Frontend 完整门禁和异步泄露检测均为 288 files / 1784 tests，87/87 Runtime operation fact families、3/3 sidecars、16/16 events 保持产品消费；Desktop Go test/vet/build 全绿；
- Runtime/Protocol/SQLite/Agent Framework 合同不变，`app/cli` 未修改或暂存，无关工作区改动保持原样。

## 65. P86 — survivor Run recovery 与已挂载 read model 收敛

### 目标

关闭 owner Runtime 被强杀后，survivor 周期接管 Run recovery 虽已原子提交 `RunLost`、HITL 清理、Tool closure 与 Goal 记账，但其已挂载客户端可能永久停留在旧投影的缺口。恢复事务由 survivor 自己的 SQLite connection 提交，`PRAGMA data_version` 按定义只观察其他 connection，因此不能把本地 recovery commit 当作跨实例 observer 的输入；精确失效必须由拥有 recovery write-set 的 Run Application 在 commit 成功后发布。

### 工作项

- [x] P86-01 Run Recovery Application 接收唯一 invalidation publisher，并从已验证的 `RecoveryCommit` 按 Session 归并实际 write-set scope；
- [x] P86-02 commit 成功后以确定顺序发布 Runs、Interrupts、Sessions，并在 Goal-owned Run 记账时发布 Goals；Run/Tool/HITL/Plan 继续通过既有原子 `sessions.snapshot` 收敛，Goal 继续由独立权威 query 收敛；
- [x] P86-03 commit 失败零通知，仍由 live peer 持有的 Session 零通知；不以 Bootstrap 定时器或 SQLite observer 建立第二恢复策略；
- [x] P86-04 以一个 survivor 和真实 SQLite claimed-resume 强杀窗口验证：accepted Question answer 保留、隐藏 `resuming` row 删除、Run 变为 Lost、conversation Tool call 原位错误闭合，并发出精确 read-model 通知。

### 验收

- survivor 无需重启即可在接管后推动已挂载 HITL、Plan、Goal、Run/Tool 重读 durable truth；通知只在恢复事务 durable commit 后出现；
- Goal-owned Lost Run 同一恢复事务完成一次记账并使 Goal query 失效；foreign live-owner Session 不被恢复也不产生虚假事件；
- Runtime workspace/standalone 全量 test、build、vet，相关 owner race、staticcheck、`deadcode -test`、golangci-lint、tidy-diff、生成物与 diff-check 全绿；Protocol wire、SQLite epoch、Artifact、Desktop Agent inner ring 与 Agent Framework 合同不变；
- `app/cli` 未修改或暂存，无关工作区改动保持原样。

## 66. P87 — Run opening 命令结算与 executor activation 所有权

### 目标

关闭根 Run Start/HITL Resume 已完成 durable opening，Operation/idempotency 却仍同步等待 executor activation 的结算缺口。旧路径在 answer claim、Run `Waiting → Running` 与 opening projections 均已提交后才调用 `BeginContinuation`，若该外部 activation 阻塞，客户端会把已接受命令永久视为未结算，无法得到可重放的 Run/Segment identity；此时重试、断线或强杀恢复面对的业务事实与命令 receipt 不一致。

### 工作项

- [x] P87-01 把根 Start/Resume 的 durable opening 明确为唯一命令 acceptance point；注册 live owner、建立 opening subscription 并发布 committed opening 后，立即返回 Run/Segment identity；
- [x] P87-02 以 `DetachActivation` 明确标记只有根 Start/Resume 将纯 executor activation 交给既有 Run lifecycle task；activation 失败仍由同一 pump 形成权威 terminal，不新增 goroutine owner、重放路径或临时超时补丁；
- [x] P87-03 保持 waiting-child cancellation 的 activation 同步，因为该边界还拥有必须在命令返回前验证的 post-commit Application `Apply/Continue`；race 反例证明两种结算策略不能被一个通用异步分支混同；
- [x] P87-04 用一个客户端各执行一次阻塞 root Start 和 HITL Resume，证明 opening 已提交时 command ack 不等待 activation；以真实 SQLite 分别冻结强杀发生在 answer claim 后、以及 continuation opening 已提交但 activation 尚未完成的 durable shape。

### 验收

- Start/Resume 的 idempotency outcome 可在 opening commit 后立即结算；阻塞 activation 不占有已经接受的 command response，释放后仍沿唯一 lifecycle stream 自然完成；
- 强杀前若只完成 answer claim，或已完成 Run opening，survivor 均将该无 executor owner 的 Run 收敛为 Lost：accepted Question answer 保留、隐藏 resuming row 清理、Tool/conversation closure 与 P86 scoped invalidation 不变；
- waiting-child cancellation 的 Apply/Continue/Discard 后置条件、同步错误语义与 race 保持全绿；Protocol wire、SQLite schema、Artifact、Desktop Agent inner ring 与 Agent Framework 合同不变；
- `app/cli` 未修改或暂存；测试结束无 Runtime、Go test、agent-browser、Chrome/daemon 残留。

## 67. P88 — Desktop final teardown 与 mutation owner 不可复活

### 目标

关闭窗口 final teardown 已同步释放当前 Runtime client 的 mutation journal claim，旧 composition-root 引用却仍可重新构造 successor client 的生命周期缺口。旧实现先把 `shared` 清空并等待 retiring transport；若异步 bootstrap、事件回调或 renderer replacement 在等待窗口内再次调用旧 `container.client()`，新 client/journal 不在本次 `Promise.all(retiring)` 的所有权快照中，窗口关闭会遗留未 join transport 或新 owner lease，下一 renderer 可能被一个已经关闭的 owner 阻断。

### 工作项

- [x] P88-01 以单 renderer 确定性交错证明：`disposeContainer()` 已开始或完成后，旧 owner 仍能创建新的 Runtime client 与 sidecar；红测不依赖高并发或浏览器时序概率；
- [x] P88-02 让 composition root 显式拥有 `open → closed` 单向 lifecycle；final teardown 在任何资源退休前同步封闭 factory，后续 `client()`/`sidecar()` 一律 fail closed；
- [x] P88-03 将同一 owner 的重复 dispose 收敛到唯一 settlement，并继续等待 teardown 开始前已经进入 retiring 集合的全部 client；`resetContainer()` 仍先发布全新 owner，再 join 旧 owner，不建立兼容路径；
- [x] P88-04 冻结真实 identity 交接：旧 mutation 在途、replacement 接管同 key 后窗口再次关闭、旧 response 最后迟到；Host KV 无 owner lease 残留，下一次冷启动仍以更高 generation 接回原 key。

### 验收

- final teardown 一经开始，旧 composition-root 引用不能再创建 RPC transport、sidecar 或 mutation journal owner；teardown 前已经退休的 client 仍由同一 settlement 完整 join；
- replacement window 关闭不会让 retired late success 删除 successor、复活旧 generation 或遗留 owner KV；下一次冷启动复用原 idempotency key；
- Desktop 全门禁与完整异步泄露检测全绿；Runtime/Protocol/Host KV schema、Desktop Agent inner ring 与 Go Agent Framework 合同不变；
- `app/cli` 未修改或暂存；未使用 agent-browser，测试结束无本仓 Runtime、Vitest/Vite、Chrome/daemon 或 17171 监听残留。

## 68. P89 — 长对话 compaction 与 Run boundary 坐标统一收敛

### 目标

关闭多轮长对话压缩只替换 conversation、却保留旧 `runs.message_mark` 坐标的跨聚合缺口。旧 Run 的水位来自压缩前历史，连续压缩后可以大于当前消息数，使 Session export、fork、rollback 与冷启动 material read 在同一份 durable 数据上得到互相冲突的答案；若消息替换与水位修正分成两个事务，事务失败或进程强杀还会把该不变量永久写入 SQLite。

### 工作项

- [x] P89-01 以一个客户端的顺序长对话证明：历史从 8 条压成 3 条后，早期 terminal Run 仍保留 4/6/8 等旧坐标，`ResolveForkBoundary` 与 snapshot watermark invariant 因此失效；
- [x] P89-02 由 Conversation Domain 定义唯一坐标变换：零边界保持零，摘要覆盖区内的正水位统一折叠到 replacement prefix 末端，保留 suffix 按与 cutoff 的距离平移；纯 Tool 内容裁剪保持坐标不变；
- [x] P89-03 由 Conversations Application 读取完整 Run snapshot 并生成 exact aggregate replacement；非终态 Run 保持 unknown watermark，Adapter 不拥有 rebase 业务公式；
- [x] P89-04 由 Persistence Adapter 在同一 SQLite transaction 内重验消息数与完整 Run set，原子 Replace history 并以 expected-mark CAS 更新每个受影响 terminal Run；任何一步失败全部回滚；
- [x] P89-05 覆盖同一 Session 连续两次压缩、历史 Run 与最新 Run 分叉、Run-watermark 写失败注入及直接 SQLite `message_mark <= message count` 检查。

### 验收

- 每一代 compacted conversation 与所有 terminal Run 只使用同一坐标系；连续压缩不会重复使用第一代旧坐标，fork/export/rollback 的 watermark 前置条件保持成立；
- summary 调用仍在事务外完成，但最终 commit 对 summary 读取时的消息数与 Run snapshot 做 fail-closed 校验；消息变化、Run lifecycle 变化或 watermark CAS 失败均不产生半压缩状态；
- 领域公式位于 `domain/conversation`，跨聚合计划位于 `application/conversations`，事务执行位于 `adapter/persistence` / SQLite；`runmaintenance` 只决定压缩内容和 cutoff，Bootstrap 只装配；
- Protocol、Artifact、SQLite schema epoch、Desktop 与 Agent Framework 合同不变；`app/cli` 未修改或暂存，未使用 agent-browser，测试结束本仓无残留进程或 17171 监听。

## 69. P90 — EventCommit Segment 代际事务准入

### 目标

关闭 HITL Resume 替换活动 Segment 后，旧 Segment 的迟到事件仍可向同一 Run 提交完整写集的代际缺口。旧 `EventCommit` 只有 Run/Session envelope；Model/Tool/Progress 的局部 Segment fence 无法保护纯 Transcript/Conversation 投影，terminal Run 又会清空 `active_segment_id`，导致旧 Segment 的终态在时间与累计 metrics 恰好可重放时结束新恢复的 Segment。

### 工作项

- [x] P90-01 以一个客户端的真实 SQLite `Running(seg_old) → Waiting → Running(seg_new)` 顺序交错，稳定证明旧 Segment terminal 能错误结束新 Segment；不依赖高并发或概率 race；
- [x] P90-02 将 `SegmentID` 提升为 Application `EventCommit` 的完整 envelope，Reducer、authoritative batch、terminal combine、child opening 与 route invariant 均只产生同一 Segment 的写集；Model/Tool/Progress 必须与 envelope 精确一致；
- [x] P90-03 在 runsegment 已有 SQLite transaction 内、任何 Transcript/Conversation/invocation/progress/lifecycle/Goal 写入前执行 `RequireActiveSegment`，只有仍处于 Running 且 active Segment 精确匹配的 Run 才能提交；
- [x] P90-04 覆盖旧 Segment terminal、纯 Transcript + Conversation 迟到投影、combined terminal Segment 混代、Model/Tool/Progress 混代，以及 fresh/resumed/child opening 和 tree barrier 的 Segment 传递；
- [x] P90-05 保持 waiting subtree cancellation 的 Waiting→terminal 专用写集不伪造 active Segment；它继续从已收敛 Pending/Run aggregate 直接 terminalize，不被普通 live event 准入路径混同。

### 验收

- EventCommit 的全部 durable projection 共享唯一 Run/Session/Segment owner；局部 invocation/progress fence 不再掩盖 transcript、conversation 或 lifecycle 的无代际写入；
- 旧 Segment 在 Resume 后无论提交 terminal 还是 item/message-only write-set，都在事务首个持久化动作前失败；新 Segment 的 Run 状态、active identity 与所有 read model 保持不变；
- route/reducer 属于 Application，事务准入属于 runsegment consumer port，SQLite 只证明当前 durable active Segment；没有兼容双路径、读取时修补或 Agent Framework/Delivery 反向依赖；
- Protocol、Artifact 与 SQLite schema epoch 不变；`app/cli` 未修改或暂存，未使用 agent-browser，测试结束本仓无残留进程或 17171 监听。

## 70. P91 — Interaction Host 前置边界失败结算

### 目标

关闭模型或 Tool 外部调用尚未发生、Runtime 自有权威前置事实提交却失败时，Interaction 把它当作普通 provider/Tool error 的结算缺口。模型路径会把内部事务失败误报为 provider unavailable；Tool 路径更会把失败作为模型可见 Tool result 并继续下一轮，令长对话跨越一个从未持久化的 Tool boundary 继续生成。

### 工作项

- [x] P91-01 以单客户端确定性失败注入覆盖 ModelCallStarted、ToolCallStarted、自动审批拒绝结果与 SteerMessagesApplied 的 pre-call commit rejection，证明外部 provider/Tool 零调用、零 unknown effect，且 Tool/steer/denial 路径不再开始下一模型轮次；
- [x] P91-02 在 Agent Interaction 定义中性的 `ErrHostFailure`/`HostFailure` 合同，仅表达外部调用前 Host 自有前置边界被拒绝；不携带 Runtime、事务、数据库、RPC 或产品 failure kind；
- [x] P91-03 将 Interaction Dispatcher protocol 从 v4 直接升级到 v5，以互斥 `host_error` definite settlement 终止 Execution；旧格式不双读，普通 provider/Tool error 与调用后 unknown settlement 保持原语义；
- [x] P91-04 Runtime 只在 `adapter/agentexec` 标记权威 pre-call commit failure，并将稳定 `interaction.host.failed` 投影为产品 internal failure；Agent→Runtime 依赖保持为零；
- [x] P91-05 形成 Agent ADR-A2-076 / Baseline 22，先发布 canonical Agent source，再让 Runtime standalone 绑定唯一 pseudo-version，不建立 local replace 或兼容双路径。

### 验收

- 任一权威 pre-call commit failure 都在外部边界前确定终止；provider/Tool 不被调用，Tool failure 不成为 WorkingContext 业务结果，后续模型 turn 不发生；
- 调用后的 projection failure 仍走既有 attempt tracker / unknown-effect recovery，不误用 HostFailure；普通 provider error 仍映射 provider unavailable，普通 Tool error 仍对模型可见；
- Agent Interaction protocol v5 的互斥 mode、prior-version rejection、public/private digest 和 Runtime external-module consumer 全绿；workspace 与 `GOWORK=off` 解析同一已发布 Agent source；
- Protocol、Artifact、SQLite schema epoch 与 Desktop 均不变；`app/cli` 未修改或暂存，未使用 agent-browser，测试结束本仓无残留进程或 17171 监听。

## 71. P92 — 并行 Tool Effect 外部边界与失败仲裁

### 目标

关闭 P91 单 Tool definite Host failure 在两个显式并行 Tool 的同一 Effect 中被错误泛化的缺口。一个 sibling 的 Tool start 或自动审批拒绝投影失败时，另一个 sibling 可能已经跨越外部调用并在等待模型顺序的 canonical result prefix；若整批仍结算 definite HostFailure，会抹掉已经发生的外部副作用，真实 Run pump 还会形成“pending result receipt 等待缺失前缀、Dispatcher 等待全部 goroutine”的闭环。

### 工作项

- [x] P92-01 只用两个明确声明可并行的 Tool、窗口 2 构造确定性交错：index 1 外部写已发生且 result receipt 未结算，index 0 自动拒绝结果的权威提交失败；证明旧实现既不报告 unknown，又无法退休 pending sibling；
- [x] P92-02 让一个 Agent Effect 的 `dispatchAttempt` 统一仲裁外部边界与 Host projection failure：失败先发生时禁止后续 sibling 跨界，任一 sibling 已跨界时整批只能是 indeterminate；
- [x] P92-03 为 post-external canonical projection 建立 attempt-owned retirement context；它忽略普通 Effect cancellation以保护已发生调用的 durable result，却在同 Effect 已被证明 indeterminate 时立即释放等待，不再依赖 Run teardown打破环；
- [x] P92-04 保持单 Tool 的 P91 行为：外部调用前提交失败仍 definite internal；并行 sibling 已跨界时转入既有 unknown-effect/RunLost 路径，不能产生模型可见 Tool result 或 definite SegmentEnded；
- [x] P92-05 冻结 root cancellation 与 pre-model commit settlement 交错：已接受取消继续拥有产品 terminal，即使 start receipt 在 dispatch context 取消后才成功结算，provider 仍为零调用。

### 验收

- 两 Tool 场景中外部调用恰好一次；失败的 approval projection 不泄露普通 Tool output，未结算 canonical sibling receipt 被 attempt failure 主动退休，Run 收到唯一 unknown Effect且没有 definite terminal；
- Host failure 先于全部 external call 时后续 sibling 无法跨界，仍按 P91 唯一 internal terminal；root cancellation 不被 Host failure 抢占；
- 改动止于 Runtime→Agent 防腐层 `adapter/agentexec`，Agent Framework、Application、Domain、Persistence、Protocol、Artifact、SQLite 与 Desktop 合同均不变化；
- 核心两 Tool交错重复与 race、agentexec 全包 race、Runtime workspace/standalone 全门禁通过；`app/cli` 未修改或暂存，未使用 agent-browser，测试结束本仓无残留进程或 17171 监听。

## 72. P93 — 终态事务成功回执丢失收敛

### 目标

关闭 terminal `EventCommit` 已跨越 SQLite durable COMMIT、调用方却收到错误或在成功回执前被取消时的结算缺口。旧实现会把事务当作失败并盲重试，但 terminal Run 已不再属于 active Segment，精确栅栏会永久拒绝重放；RunLost pump 因而可能无限重试，同进程也不会发布 terminal Run/Goal read-model invalidation。

### 工作项

- [x] P93-01 以单客户端真实 SQLite transaction 包装器在 COMMIT 成功后丢失回执并立即取消请求上下文，冻结“durable terminal 已存在、调用方认为失败”的确定窗口；
- [x] P93-02 由 Application reducer 为每个不可变 terminal write-set 铸造稳定 identity，并由 combined terminal commit 原样保留；重试同一批次复用 identity，另一 terminal attempt 获得新 identity；
- [x] P93-03 让 runsegment/SQLite 在原子事务中把 terminal commit identity 与生产它的 Segment 写入 Run 行；Restore/Recovery/direct terminal 不伪造 marker，唯一索引禁止 identity 跨 Run 复用；
- [x] P93-04 让 terminal `CommitEvent` 在错误后用有界、request-detached context 核验 exact Session/Run/Segment/commit identity；精确成功与精确 replay 收敛为 nil，不再进入 blob compensation 或无限 RunLost 重试；
- [x] P93-05 冻结另一 Segment、另一 terminal attempt、无 marker terminal 继续失败，精确 replay 不重复追加 conversation，以及 caller 已取消时 reconciliation 仍能完成。

### 验收

- COMMIT 成功后注入错误并取消原请求，`CommitEvent` 仍结算成功；durable Run terminal、conversation watermark 与消息内容一致；
- 同一 commit identity 重放成功且 conversation 保持单份；不同 Segment、不同 terminal attempt 或 Restore/Recovery terminal 不能冒充原提交；
- 事务原子性继续证明 terminal marker、Run、Transcript/Conversation/invocation/Goal/checkpoint cleanup 属于同一 write-set；SQLite 唯一 shape 前移为 epoch 71，无 migration/兼容路径；Protocol、Artifact、Desktop 与 Agent Framework 合同不变；
- 核心场景重复与 race、Runtime workspace/standalone 全门禁通过；`app/cli` 未修改或暂存，未使用 agent-browser，测试结束本仓无残留进程或 17171 监听。

## 73. 进度记录

| 日期 | 阶段 | 完成事实 | 验证 |
|---|---|---|---|
| 2026-08-15 | P93（ambiguous terminal commit settlement） | 反证发现 terminal `EventCommit` 的 SQLite transaction 已 durable COMMIT、但 success receipt 丢失或调用方随即取消时，runsegment 仍返回失败；后续 RunLost/terminal replay 被 exact active-Segment fence 永久拒绝，同进程 terminal Run/Goal invalidation 也不会发生。进一步反证否决了仅比较 terminal Run 的初版方案：终态已清空 active Segment，Restore 或另一 Segment 可能产生相同 aggregate，无法证明 Transcript/Conversation/Goal/cleanup 就是本次 write-set。Application reducer 现在为每个不可变 terminal write-set 铸造稳定 identity，runsegment/SQLite 将 identity 与生产 Segment 在同一事务写入 Run 行；错误后只以 exact Session/Run/Segment/commit marker 结算。SQLite 唯一 shape 前移到 epoch 71，无兼容双路径 | 真实 SQLite 在 COMMIT 后注入回执丢失并立即取消 caller：terminal marker、Run 与一条 conversation 原子可读；同一已取消 context 上 exact identity replay 成功且消息不重复，另一 terminal identity 失败。核心场景 20×、race 5× 与受影响包定向测试全绿；Runtime workspace/standalone 全门禁与资源清理在本批提交前完成，受影响包无 fuzz owner，未使用 agent-browser，`app/cli` 未修改或暂存 |
| 2026-08-15 | P92（concurrent Tool Effect boundary arbitration） | P91 的单 Tool HostFailure 反例扩展到一个真实窗口中的两个 Tool 后，发现 index 1 外部写已经发生并等待 canonical result prefix，index 0 自动审批拒绝投影失败却把整批结算为 definite internal；真实 pump 还会让 sibling receipt 与 Dispatcher 互相等待。`dispatchAttempt` 现在统一拥有 Effect 级外部边界和失败 arbitration：Host projection failure 会阻止尚未跨界的 sibling，并通过独立 failure context 退休已跨界 sibling 的 post-external projection wait；只要任一外部调用发生，wrapper 就走既有 unknown-effect/RunLost，单 Tool 零外部调用仍保持 P91 definite internal。另补 accepted root cancellation 与迟到 model-start receipt 交错，取消 terminal 保持优先且 provider 零调用。无 Agent Framework、Application/Domain/Persistence、Protocol、SQLite 或 Desktop 变化 | 两 Tool / window 2 的 pending canonical receipt 红测修复前稳定得到零 unknown并超时，修复后外部 Tool 恰好一次、唯一 unknown、零 SegmentEnded；核心场景 20×、race 5× 与 agentexec 全包普通/race 全绿。Runtime workspace/standalone test/build/vet/staticcheck/deadcode/lint/tidy/generate/diff-check及资源清理在本批提交前完成；受影响包无 fuzz owner，未使用 agent-browser，`app/cli` 未修改或暂存 |
| 2026-08-15 | P91（Interaction Host pre-call failure settlement） | 反证发现 Runtime 在模型/Tool 外部调用前提交 ModelCallStarted、ToolCallStarted、自动审批拒绝结果或 applied steer 失败时，Agent Dispatcher 只能返回普通 error：模型路径被误报 provider unavailable，Tool/denial 路径更把 Host 持久化失败写进模型上下文并开始下一 turn。Agent Interaction 现以中性 `HostFailure`、protocol v5 互斥 `host_error` definite settlement 和稳定 `interaction.host.failed` 终止；Runtime 只在 `adapter/agentexec` 标记 pre-call 权威提交失败并投影为 internal failure。调用后结果不明仍走既有 attempt tracker，普通 provider/Tool error 不变。Agent ADR-A2-076 / Baseline 22 已先以 canonical commit `a9f35b30656a` 发布，Runtime standalone 绑定其唯一 pseudo-version，无 replace、shim 或双路径 | 单客户端 Model/Tool/approval-denial/steer 四类失败注入证明外部调用零发生、unknown effect 为零、后续模型 turn 为零，核心场景 10× 与 Agent/Runtime adapter race 全绿；Agent standalone build/vet/staticcheck/test/Interaction race/lint/tidy-diff 与 public/private baseline 全绿。Runtime workspace/standalone 全包 test、external-consumer、build、vet、受影响 adapter/architecture race、staticcheck、deadcode、golangci-lint（0 issues）、tidy、generate 与 diff-check 全绿；受影响包无 fuzz owner，未使用 agent-browser，本仓进程与 17171 监听为零，`app/cli` 未修改或暂存 |
| 2026-08-15 | P90（EventCommit Segment generation admission） | 反证发现 HITL Resume 用 `seg_new` 替换活动代际后，旧 `seg_old` 的迟到 `EventCommit` 仍只按 Run/Session 提交。Model/Tool invocation 与 Progress 虽各自携带 Segment，纯 Transcript/Conversation 没有 fence；terminal aggregate 又已清空 active Segment，因此旧终态在累计 metrics 与时间可重放时会从当前 `seg_new` aggregate 推导出完全相等的 terminal Run，错误结束恢复后的对话。Application 现在让每个 EventCommit 以 Run/Session/Segment 共同拥有完整写集，Reducer、authoritative combine、route 与 child opening 都保持同代；runsegment 在原事务的任何 projection 写入前要求 SQLite 中该 Run 仍为精确 Running Segment。waiting cancellation 继续走其 Waiting aggregate 专用事务，不伪造 live Segment。无 schema、wire、compat 或双路径 | 单客户端真实 SQLite `seg_old → Waiting → seg_new` 回归分别注入旧 terminal 与 item/message-only write-set：两者均 fail closed，Run 保持 running `seg_new`，Transcript/Conversation 零写入；Application 锁定 Model/Tool/Progress 与 combined terminal 混代拒绝。核心场景连续 10×、Runtime `go test ./... -count=1`、standalone tidy/build/vet/test、受影响 5 包 race、staticcheck、`deadcode -test`、golangci-lint（0 issues）、`go generate ./...` 与 diff-check 全绿；受影响包无 fuzz owner，未使用 agent-browser，本仓进程与 17171 监听为零，`app/cli` 未修改或暂存 |
| 2026-08-15 | P89（long-conversation compaction coordinate convergence） | 反证发现 `runmaintenance` 以原子 `MessageStore.Replace` 将长对话从旧历史改写为 `[summary, optional live reminder, recent…]`，但所有既有 terminal Run 的 `message_mark` 仍停留在压缩前坐标；连续多轮后旧水位可超过当前消息数，Session snapshot/export/fork/rollback 会拒绝同库数据。Conversation Domain 现在拥有唯一 compaction coordinate transform；Conversations Application 从完整 Run snapshot 决定 exact watermark replacements；Persistence 在一个 SQLite transaction 内重验 expected message count 与完整 Run set，再同时替换历史并以 expected-value CAS 重基准 terminal Runs。摘要区内已不可区分的历史边界显式折叠到 replacement prefix，suffix 保持相对距离，纯内容裁剪零坐标变化；不存在读取时 clamp、兼容双路径或 SQLite 自行发明业务公式 | 单客户端 4 个顺序 Run、两次连续长对话压缩后，所有旧/新 Run 水位均落在当前 3 条历史内，旧/最新 Run fork 继续成立；SQLite trigger 注入 Run-watermark 更新失败时，消息和全部水位一起回滚，原始 8 条历史完整保留；直接 SQL invariant 为零。领域/Application/Persistence/runmaintenance/Bootstrap 定向测试与 10× SQLite 场景、受影响包 race 全绿；Runtime `go test ./... -count=1` 全包、standalone build/vet、staticcheck、`deadcode -test`、golangci-lint、tidy-diff、`go generate ./...` 与 architecture gate 全绿；受影响包无 fuzz owner，未使用 agent-browser，本仓进程与 17171 监听为零，`app/cli` 未修改或暂存 |
| 2026-08-15 | P88（Desktop final teardown ownership） | 反证发现 final window teardown 虽同步释放当前 client 的 mutation journal claim，却把 `shared` 清空后继续暴露可调用的旧 factory；等待 retiring transport 时，迟到 bootstrap/事件回调可复活一个不在 teardown settlement 内的 successor client，重新领取 mutation identity 并遗留 transport/lease。composition root 现在先同步进入不可逆 closed 状态，再退休当前 client，并让重复 dispose 共享唯一 settlement；测试 reset 仍通过先换新 owner 保持正常 replacement。journal 回归进一步覆盖旧 mutation 在途、replacement 接管后窗口关闭、retired response 迟到与下一冷启动接回同 key，确认 generation fence 和 Host KV owner cleanup 共同成立 | 红测稳定证明 teardown 中与 teardown 后旧 owner 均可复活，修复后 container/SDK/journal/methods 4 files / 76 tests 及 `--detectAsyncLeaks` 全绿；Frontend `npm run check` 与完整泄露检测均为 288 files / 1786 tests、零泄露，consumer 为 87/87 operation fact families（84 direct + 3 server-materialized）+ 3/3 sidecars + 16/16 events / 103 typed call sites，994 locale keys 在 8 种语言完整；未使用 agent-browser，本仓进程与 17171 监听为零，`app/cli` 未修改或暂存 |
| 2026-08-15 | P87（Run opening command settlement） | 反证发现根 Start/HITL Resume 的 answer claim 与 Run opening 已 durable commit 后，`openSegment` 仍同步等待 executor activation；activation 卡住会让 Operation/idempotency 无法保存或返回已接受的 Run/Segment receipt。Application 现在显式区分结算边界：根 Start/Resume 在 commit、live-owner 注册与 opening publication 后立即结算，activation/pump 由原 Run lifecycle task继续拥有；waiting-child cancellation 保持同步，以保护其 post-commit Apply/Continue 验证。真实 SQLite 同时覆盖 claim-before-opening 与 opening-before-activation 两个强杀 shape，均由 survivor 恢复为 Lost并保留已接受 Question answer。无兼容路径、第二 activation owner 或协议/存储改动 | 单客户端 blocked Start/Resume 交错、detached activation failure、waiting-child cancellation 后置条件及 claimed/opened SQLite recovery 回归通过；runs/runrecovery/runsegment/server/operation 定向 race 全绿；Runtime workspace/standalone 全量 test、build、vet、staticcheck、`deadcode -test`、golangci-lint、tidy-diff、`go generate ./...` 与 diff-check 全绿；受影响 package 无 fuzz owner，未使用 agent-browser，进程检查无 Runtime/Go test/Chrome/daemon 残留；`app/cli` 未修改或暂存 |
| 2026-08-15 | P86（survivor Run recovery read-model 收敛） | 反证发现 survivor 的周期 Run recovery 通过本 Runtime SQLite connection 提交，connection-local `PRAGMA data_version` 不会观察自己的 commit；因此数据库虽已收敛为 RunLost，当前 Runtime 的已挂载 HITL/Plan/Goal/Run/Tool 仍可能无限停在被强杀 owner 的旧投影。Run Recovery Application 现在从原子 `RecoveryCommit` 按 Session 汇总精确 scope，仅在 commit 成功后按 Runs、Interrupts、Sessions、Goals 顺序发布既有 invalidation；Bootstrap 只注入 publisher，不拥有恢复策略。真实 claimed-resume SQLite 回归同时冻结 accepted Question answer、隐藏 resuming row、conversation Tool closure 与 Lost Run 的一致性；事务失败和 live peer owner 均零通知。Protocol、SQLite schema、Artifact、Desktop Agent inner ring 与 Agent Framework 合同未变化 | application/SQLite/bootstrap 定向测试与 runs/runrecovery/ownershiprecovery race 全绿；Runtime `go test ./... -count=1 -timeout=10m`、build、vet、`make check-standalone`、staticcheck、`deadcode -test`、golangci-lint、tidy-diff、`go generate ./...` 与 diff-check 全绿；受影响 package 无 fuzz owner，未新增无业务意义的并发压力；`app/cli` 未修改或暂存 |
| 2026-08-15 | P85（Desktop CWD 与右侧 Context Dock 接线） | Wails v3 Desktop Host 新增 native directory picker，严格返回已存在绝对目录；Navigation Application 以独立 port 组合 picker、Session create `{cwd}` 与 focus，取消零写入、失败不回退 home，同一在途用户意图只结算一次。Work Index 新增“打开文件夹”入口并覆盖 8 个 locale。只读对照桌面 Proma 源码后，明确区分额外目录附件与 Session CWD；Lynx 右栏继续使用现有 per-Session Context Dock owner，只新增统一 `view.toggle-dock` 命令及 `Mod+Shift+B`，折叠后不丢标签。Runtime、Protocol、SQLite、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化 | 定向 6 files / 37 tests 与 typecheck 通过；Frontend `npm run check` 及完整 `--detectAsyncLeaks` 均为 288 files / 1784 tests、零泄露，consumer 为 87/87 operation fact families（84 direct + 3 server-materialized）+ 3/3 sidecars + 16/16 events / 103 typed call sites，994 locale keys 在 8 种语言完整；Desktop `go test ./... -count=1`、`go vet ./...`、`go build ./...` 与 diff-check 全绿；`app/cli` 未修改或暂存 |
| 2026-08-14 | P84（shared embedded Runtime ownership） | 删除 data-directory 全生命周期单实例锁，以短期私有目录 setup lease、per-Session writer、physical working-tree shared/exclusive lease 与 per-Session Goal drive 建立真实业务所有权；跨进程 recovery sweep 只有一个 winner并固定 Run-before-Goal，各策略只接管成功取得 live-owner lease 的 identity并按 Session cleanup，Instance 周期重验让 survivor 在 peer 强杀后继续恢复。另一个 SQLite connection 的 commit 通过 `data_version` 发布 full resync，已挂载 HITL、Plan、Goal、Run/Tool 继续重读 durable truth。公共 `ErrDataDirectoryInUse` breaking 删除，不新增兼容路径、协议 wire、SQLite epoch或 Agent Framework 依赖 | 两个真实 embedded Runtime 同时打开相同目录并共享 idempotency namespace；跨实例 Session commit 触发 scoped `RuntimeResync`；独立子进程持有 Session lease 时 peer fail closed，强杀后 lease 转移连续 5/5；ordered sweep、Goal/Run recovery contention 与 scoped SQLite cleanup 回归通过。`go generate ./...` 零漂移，workspace/`GOWORK=off` 全量 test、build、vet，9 个高风险 package race，staticcheck、deadcode、golangci-lint、tidy-diff、contract/architecture 与 diff-check 全绿；未修改 `app/cli`，未使用 agent-browser |
| 2026-08-14 | P83-22（mounted Session atomic material snapshot） | Desktop 挂载/恢复 Agent Session 原先并行调用 `items.list`、`runs.list`、`interrupts.list` 与 `plan.get`；四个独立 SQLite transaction 即使最终会被事件修正，首次 fold 仍可能组合出数据库中从未共同存在过的 Run/Tool、HITL 与 Plan。现在 Application 定义并校验 Session/Item/Run/open Interrupt/Plan 的 material snapshot 不变量，Persistence 在一个 transaction 中读取全部事实，Delivery 以 `sessions.snapshot` 做 capability-preserving 投影，公开 Go binding、生成合同与 Desktop SDK/Adapter 同步接入并删除旧四读恢复路径。Goal 保持由独立 `goals.get` owner 恢复。Registry 的 `materializes` 元数据让 consumer gate 能区分服务端原子组合与孤儿能力，不要求前端发冗余请求，也不改变独立查询的分页/筛选语义 | 两个真实 SQLite connection 在 WAL 下制造读中途并发提交：旧 snapshot 精确保持 Session/Plan 1/1，下一次读取为 2/2，连续 20/20 稳定；定向 race、生成合同漂移/摘要、Server capability/existence、公开 Go binding 与真实 Runtime SIGKILL 恢复回归通过。Runtime `go test ./... -count=1 -timeout=10m` 全包通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 287 files / 1776 tests、零泄露，consumer 为 87/87 operation fact families（84 direct + 3 server-materialized）+ 3/3 sidecars + 16/16 events / 103 typed call sites；未使用 agent-browser |
| 2026-08-14 | P83-21（active-workspace FileTree pagination retirement） | FileTree 的根目录与每个展开目录都通过参数化 Query 调用 `workspace.files.list().autoPagingToArray()`；cwd/path cache identity 能隔离屏幕值，`files.changed`/global replacement 也能替换 writer，但默认 Data Provider 与 bound paged SDK 丢弃 Query generation signal。旧目录已进入 continuation 后切换 active Session/cwd 时，新树可以显示，旧首页/续页 correlation 与 HTTP fetch 却继续存活；目录多时会按每个展开节点累积。现在 provider 把同一 signal 交给 bound `files.list`，SDK 既有唯一 pagination policy 为首页和全部 cursor continuation 复用同一 call option；一次 generation abort 释放整代分页资源，不增加取消消息或额外 owner。Runtime wire 仍止于 defaults Adapter/SDK，Workspace/Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 一客户端真实挂载 FileTree read：旧 cwd 首页返回 `old-page-2` cursor、第二页悬挂后 active cwd 切到 successor；旧两页共享的 transport signal 立即 aborted，successor 使用独立 active signal并显示新树，随后注入旧第二页也不能覆盖。修复前精确红在旧两页 transport signal 都为 `undefined`；SDK 单测锁定 bound workspace identity、path 参数与 cancellation option。核心交错独立 Vitest 进程连续 20/20 通过，focused 2 files / 30 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 287 files / 1774 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-20（active-workspace change-summary transport retirement） | 始终挂载的 Header diff stat 与 Files view 都以 active Session cwd 查询 `workspace.changes.list`；参数化 Query key 已能隔离 cwd 的 cache identity，P83-15 也会在 `files.changed`/global replacement 时替换 writer，但默认 Data Provider 和 bound Workspace SDK 丢弃 Query generation signal。因此 active Session 从一个 cwd 切到另一个 cwd 时，UI 会显示 successor 事实，旧 cwd 的 RPC correlation/HTTP fetch 却继续存活到迟到响应或 transport teardown，形成被正确 cache 表象掩盖的资源泄露。现在 provider 将同一 signal 交给 bound `changes.list`，SDK 只把它作为 transport call option；cwd retarget、committed event、Runtime replacement 与 renderer teardown 都能释放旧 request，不引入第二 lifecycle。Runtime wire 仍止于 defaults Adapter/SDK，Workspace/Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 一客户端真实挂载 `useWorkspaceFileChanges`：旧 `/old` 请求悬挂时 active cwd 改为 `/successor`，旧 transport signal 立即 aborted，新请求使用独立 active signal并显示 successor diff；随后注入旧 cwd 响应不能改变当前 read model。修复前精确红在旧 transport signal 为 `undefined`；SDK 单测同时锁定 bound workspace identity 与 cancellation option。核心交错独立 Vitest 进程连续 20/20 通过，focused 2 files / 29 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 286 files / 1772 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-19（Session-derived Workspace project generation replacement） | Workspace project catalog 是已提交 Session identity/cwd/time 的派生 read model，但 projection subscriber 仍直接 invalidation：首次 `workspaces.list` 尚无缓存且旧 Promise 悬挂时，一次成功 Session list commit 会被旧 read 吞掉，不建立能观察新 project/session count 的 successor。即使手工替换 cache writer，默认 Data Provider 与 SDK `workspaces.list` 也丢弃 Query signal，旧 RPC correlation/HTTP fetch 继续占有 generation。现在 project refresh 复用 Workspace Events Adapter 唯一 cancel-before-invalidate policy；默认 provider 把同一 Query signal 交给 SDK/transport，successor query 成为唯一 cache commit owner，旧 request 精确退出。Session→Workspace 只经双方 public read-model facade，Runtime wire 仍止于 defaults Adapter/SDK；Agent/Workspace Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 第一条一客户端挂载场景让 project 首读永不结算，再向 Session query 提交会改变 project index 的权威 summary：旧 query signal 立即 aborted，第二次 read 在旧 Promise 释放前显示 successor project，随后返回旧 project 不能覆盖。第二条实际 defaults Provider + Lyra MemoryTransport 场景证明 replacement 会 abort 旧 `workspaces.list` transport signal，successor signal 独立存活。修复前两场景分别精确红在 fetcher 仍只有一次调用与 transport signal 为 `undefined`。核心双场景独立 Vitest 进程连续 20/20 通过，focused 4 files / 39 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 285 files / 1770 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-18（mounted Session-list paged transport generation retirement） | P83-15 已让 `sessions.changed` 与 global replacement 先退休 Session cache writer再建立 successor，但默认 Runtime Data Provider 丢弃通用 Query generation signal，SDK `sessions.list` 也没有 cancellation 参数。Session sidebar 的 `autoPagingToArray` 因而会在 cache owner 退休后继续持有旧 RPC correlation/HTTP fetch，尤其已进入的 cursor continuation 只能等待迟到响应或 transport teardown。现在 provider 把 Query signal 原样交给 `sessions.list`，SDK 的唯一 pagination policy 为首页和所有 continuation 复用同一 transport call option；一次 abort 同时释放整代分页资源，不引入额外取消消息或第二生命周期。Runtime wire 仍止于 defaults Adapter/SDK，Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 一客户端真实挂载 Session sidebar read：旧代第一页返回 cursor、第二页保持悬挂，标准 cancel-before-invalidate 随即建立 successor；旧代两页共享的 signal 被 abort，successor signal 保持 active 并提交唯一 Session 列表，随后注入旧第二页也不能覆盖。修复前场景精确红在 transport signal 为 `undefined`；SDK 单测同时锁定首请求 cancellation。核心交错独立 Vitest 进程连续 20/20 通过，focused 3 files / 36 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 284 files / 1767 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-17（mounted Usage transport generation retirement） | P83-15 已让 `runs.changed` 同时替换 Session usage 与 cross-session Usage summary 的 cache writer，但这两条 mounted query 绕过通用 Data Provider 且 queryFn 未消费 TanStack signal；各自 Application gateway 与 SDK `usage.session` / `usage.summary` 也没有 cancellation 参数。事件后 successor read 可以建立，旧 RPC correlation 与 HTTP fetch 却会继续占有 renderer/Runtime generation，直到迟到响应或 transport teardown。现在两条 hook 都把 Query signal 传入中性 Application port，Agent/Usage Adapter 再把它交给 SDK read，SDK 只将其作为 transport call option；取消精确释放旧 correlation/fetch，successor signal 保持 active。Runtime DTO 仍只在 Adapter/SDK，Agent 与 Usage Application 不依赖 RPC；Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 两个一客户端真实挂载场景分别让 Session usage chip 与 Usage pane 的首次 MemoryTransport 请求悬挂，再执行与 `runs.changed` 相同的 cancel-before-invalidate：旧 request signal 均立即 aborted，successor 请求返回新统计并成为当前 view；随后注入旧响应，两个 read model 都保持 successor 值。修复前两场景精确红在 transport signal 为 `undefined`。核心双场景独立 Vitest 进程连续 20/20 通过，focused 5 files / 38 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 283 files / 1766 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-16（pending-work paged transport generation retirement） | P83-14/P83-15 已让 Query cache writer 在 global replacement 与定向 committed event 时退休，但常驻 Workspace Inbox badge 的 pending-work provider 忽略了通用 Data Provider generation signal：install-wide `interrupts.list().autoPagingToArray()` 的首/续页继续由旧 renderer/Runtime generation 持有，cache 虽不会被迟到结果覆盖，旧 HTTP fetch、RPC correlation 与后续 cursor attempt 却只能等待 transport 自行结算。现在 Agent Adapter 将同一 Query signal 交给 `interrupts.list`；SDK paged method 的唯一 continuation policy 本就为每个 cursor attempt 复用 call options，因此首/续页共享一个 cancellation owner，RpcClient abort 直接释放 correlation，HTTP transport 使用同一 fetch signal，不引入第二取消协议。HITL read model 与 wire translation 仍停在 Agent Adapter，Agent Application/Domain、Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 一客户端对真实 MemoryTransport 发起 install-wide pending-work 读取：第一页返回 `cursor_2`，第二页保持悬挂后取消 query generation；完整 `autoPagingToArray` 立即以 `RpcTransportError` 结算，outbox 精确只有两次 `interrupts.list` 业务请求，证明续页继承 signal 且没有附加取消消息。核心交错独立 Vitest 进程连续 20/20 通过，focused 2 files / 7 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 281 files / 1764 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-15（committed-event query generation replacement） | P83-14 只在 global Runtime replacement 前取消旧 Query generation；`goals.changed`、`interrupts.changed` 等定向 committed event 仍直接 invalidation。首次 mounted query 尚无 cached data 时，TanStack 会复用事件发生前的 in-flight Promise，因此一次已经提交并发布的变化可能不建立 successor read：旧快照迟到后会成为看似成功的当前值，事件也不会再来。现在每个 query-backed event target 都在保持原 query key、Session exact scope 与 resync topic 闭集的前提下先 cancel 旧 writer、再 invalidate；global replacement 复用同一 helper，非 Query 的 Agent material projection 继续由既有同步 owner 独立处理。TanStack retryer 观察迟到 settlement 但禁止 cache commit，successor read 是事件后的唯一事实 owner。Goal/Agent public facade、RPC/Runtime、Protocol、SQLite 与 Agent Framework 合同均未变化 | 一客户端真实挂载无 cached Goal，让事件前 `goals.get` 永不结算，再送达只点名该 Session 的 committed `goals.changed`：旧 query signal 立即 aborted，第二次 read 在旧 Promise 释放前显示 committed Goal；随后返回 pre-event Goal，证明不能覆盖新代。所有事件 query target 的单元回归同时要求 cancel/invalidate scope 完全一致。核心交错独立 Vitest 进程连续 20/20 通过，focused 2 files / 11 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 281 files / 1763 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-14 | P83-14（mounted Goal query generation replacement） | P83-10–P83-13 已让 mounted HITL/Plan/Run/Tool material snapshot 在 Runtime replacement 时退休旧 owner，但同一 global resync 对 Goal read model 只调用 TanStack Query invalidation：首次 active query 尚无 cached data 时，refetch 会复用当前旧 Runtime Promise，而不是建立 successor generation；旧 `goals.get` 若不合作，Goal 会在其他 material view 已收敛后继续悬挂，甚至迟到提交 retired fact。现在 workspace global replacement 先取消全部 query generation、再 invalidation，并在同一 boundary 触发 Agent `replace-live`；通用 Data Provider query SPI 接受 TanStack ownership signal，Goal Adapter 将它贯穿 SDK `goals.get` 到 transport。即使旧 transport 忽略 abort，Query retryer 的取消状态也会阻止迟到结果进入 cache，successor query 成为唯一 commit owner。Goal public facade、Desktop Agent Application/Domain、Go Runtime、Protocol、SQLite 与 Agent Framework 合同均未扩张 | 一客户端真实挂载 Goal query，让旧 Runtime 的首次 `goals.get` 永不结算，再触发 global Runtime replacement：旧 signal 立即 aborted，第二次 read 在旧 Promise 释放前显示 successor Goal，Agent material snapshot 同时收到 `replace-live`；随后返回 retired Goal，证明不能覆盖新代。核心交错独立 Vitest 进程连续 20/20 通过，focused 5 files / 39 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 281 files / 1762 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-13 | P83-13（exact Run settlement-read generation retirement） | P83-12 让 opening/iterator 能在 Runtime replacement 时释放，但 stream terminal 或 reconnect 停止后的 pump `finally` 仍直接等待 exact `runs.get`：旧 Runtime 若忽略 abort，`currentRunId` 会持续表示 live ownership，successor durable snapshot 因而不能启动；该 read 迟到成功时原逻辑又只检查 pump sequence，不重验 generation signal，若尚无新 pump 还会把 retired RunRef 写回 successor projection。现在 exact read 同样与 pump signal 竞速，abort 立即跳过旧结果并进入 idle boundary；read 先结算、abort 后到达的窄窗在 apply 前再次检查 session cancellation 与 signal。迟到 fulfillment/rejection 始终被 Promise boundary 观察但不能写回，`currentRunId/currentSegmentId` 由原 pump sequence 精确释放，successor snapshot 接管 HITL/Plan/Run/Tool。修复止于 Agent Adapter pump；RPC/Runtime DTO 未进入 Desktop Agent Application/Domain，Go Runtime、Protocol、SQLite 与 Agent Framework 均未变化 | 一客户端从旧 Runtime running root Run 收到真实 `segment.finished`，随后卡住 exact `runs.get`，再触发 Runtime replacement：旧 pump 在 read 未释放时退出 live ownership，successor 完成无 active Run、Plan revision 2 的权威同步；随后返回旧 finished RunRef，证明它不能重新出现。pump 单元回归另证 abort 后 `isActive=false`、`onIdle` 精确一次且 late snapshot 永不 apply。核心产品交错独立 Vitest 进程连续 20/20 通过，相邻 2 files / 33 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1759 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-13 | P83-12（dropped-stream reconnect generation retirement） | P83-11 回收了 durable snapshot 初次 `runs.subscribe` 的迟到 opening，但 active Run stream 异常 EOS 后由 `runStreamReattach` 发起的 replay/tail opening 仍直接等待 SDK Promise：旧 Runtime 若忽略 abort，pump 会永久保持 live ownership，`replace-live` 的 successor durable snapshot 因 `runPump.isActive()` 只能排队；opening 迟到返回时同样会遗弃 event iterable。现在初次 recovery 与 dropped-stream reconnect 共用唯一 Adapter opening boundary：generation abort 立即结算为无 stream并释放 pump，迟到成功主动调用 iterator `return()` 且观察异步关闭，迟到失败始终被观察；opening 已返回后才观察到取消的窄窗也显式退休 stream。replay-lost/cold recovery 的 durable projection read 同样接收当前 generation signal，不再形成第二个不合作 owner。successor snapshot 仍是 HITL/Plan/Run/Tool 的唯一 material commit owner；RPC/Runtime DTO 未进入 Desktop Agent Application/Domain，Go Runtime、Protocol、SQLite 与 Agent Framework 均未变化 | 一客户端从旧 Runtime running root Run 建立首条 live stream，令其无 terminal 异常 EOS 后卡住第二次 replay subscribe，再触发 Runtime replacement：旧 generation 退出 active pump，successor 在未释放旧 opening 前完成无 active Run、Plan revision 2 的权威同步；随后释放旧 opening，证明迟到 iterator `return()` 精确一次且 successor view 不变。核心产品交错独立 Vitest 进程连续 20/20 通过，相邻 3 files / 36 tests 在 `--detectAsyncLeaks` 下通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1757 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1 -timeout=10m` 全包通过，未使用 agent-browser |
| 2026-08-13 | P83-11（late reattach-opening resource retirement） | P83-10 允许 successor durable snapshot 立即取代不合作旧代，但旧 snapshot 已发现 running root Run 并进入 `runs.subscribe` opening 时，底层依赖仍可能忽略 generation abort：successor 已完成 HITL/Plan/Run/Tool 收敛后，旧 opening 才返回 event iterable，原 recovery 只因 cancelled 丢弃结果而未调用 iterator `return()`，连续 Runtime replacement 会积累旧 transport stream。现在 reattach opening 与 generation signal 竞速，abort 立即释放 snapshot ownership；迟到成功会主动构造 iterator、调用可选 `return()` 并观察异步关闭，迟到失败也始终被观察。successor view token 仍是唯一提交 owner，旧 iterable 无论何时结算都不能接管 live pump 或改写 projection。修复止于 Agent Adapter 的 foreign-resource boundary；RPC/Runtime DTO 未进入 Desktop Agent Application/Domain，Go Runtime、Protocol、SQLite 与 Agent Framework 均未变化 | 一客户端从含 running root Run 的旧 Runtime snapshot 进入不合作 subscribe opening，再替换到无 active Run、Plan revision 2 的 successor Runtime：successor synchronization 先独立完成；随后释放旧 opening，证明旧 iterator `return()` 精确一次且 successor projection 不变。核心场景独立 Vitest 进程连续 20/20 通过，focused `useAgentSession` 18 tests 通过；Frontend `npm run check` 为 279 files / 1756 tests，完整 `--detectAsyncLeaks` 最终复验为 279 files / 1756 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1` 全包通过，未使用 agent-browser |
| 2026-08-13 | P83-10（mounted durable-snapshot generation replacement） | P83-02 已让 Runtime 重启时旧 `runs.start/resume/subscribe` opening 与 Run iterator 退出，但 mounted Session 的 durable recovery 自身没有 generation owner：`items.list`、`runs.list`、`interrupts.list`、`plan.get` 任一旧 Runtime 读取若不响应取消，唯一 snapshot coordinator 会永远停在旧代，successor Runtime 的 authoritative read 只能排队；同一 global resync 中 Goal query 已重验，HITL/Plan/Run/Tool material projection 却永久滞留。现在 Session projection coordinator 为每次 recovery 建立独立 AbortController，并提供明确 replacement 语义：global `replace-live` 同时退休 run opening/stream 与当前 durable snapshot；coordinator 自身与 Application snapshot boundary 都让 foreign Promise 和 AbortSignal 竞速，因此不合作依赖不能保留 ownership，迟到 resolve/reject 仍被观察但不能提交。generation signal 从 Agent Application port 经 Adapter 传到 items/runs/interrupts/plan 全部分页 attempt，后续 cursor 也复用同一 signal；reattach opening/pump 绑定同一 generation。successor snapshot 仍通过既有 view token 一次提交完整 material projection，普通增量 synchronization 保持 after-live 串行。Goal 继续由同一 Workspace global Query invalidation 重验；RPC/Runtime DTO 只在 Adapter，Desktop Agent Domain、Go Runtime/Protocol/SQLite 与 Agent Framework 均未变化 | 一客户端确定性交错让旧代 `items.list` 永不结算，再触发第二次 Runtime replacement：旧 signal 立即 aborted，successor 一次 commit 同时出现 waiting Run、HITL approval、Plan revision 2 与 requires-action Tool；随后释放旧 Item 证明迟到结果不写回。coordinator 与真实 mounted lifecycle 两层场景独立 Vitest 进程连续 20/20 通过，5 files / 64 tests 在 `--detectAsyncLeaks` 下通过。Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1755 tests、零泄露，consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 107 typed call sites；Runtime `go test ./app/runtime/... -count=1` 全包通过，未使用 agent-browser |
| 2026-08-13 | P83-09（server-side idempotency-store admission fence） | P83-08 已在 Desktop 每次 transport attempt 前重验 Runtime store ownership，但同一 HTTP endpoint 仍可能在客户端完成 discovery 校验后、请求被服务端准入前切换到另一数据目录：旧 key 会在 replacement store 上成为全新命令，客户端围栏无法关闭这个 TOCTOU。现在 Desktop 将 journal reservation 已证明的 opaque namespace 作为 `Idempotency-Namespace` 随每个 replayable mutation attempt 发送；HTTP Adapter 只把它投影为 binding-neutral operation option，Operation endpoint 在参数解码、idempotency claim 与任何业务执行之前比较调用方期望 namespace 和当前 durable store namespace，不一致返回专用 `idempotency_store_mismatch` problem，绝不创建 claim 或进入 handler。相同 namespace 保持既有 replay 语义；没有 transport race 的 embedded 调用通过新增显式 option 按需使用同一围栏，不引入 Runtime/Desktop 双路径。Protocol problem、生成合同、CORS 和 public Go API baseline 原子更新；这是 v0.11 明确合同修正，不保留兼容旁路。Runtime Domain/Application、SQLite shape、Desktop Agent inner ring 与 Go Agent Framework 均未接触 transport metadata | Operation 红测证明 mismatch 在 business admission 前 calls=0、matching namespace calls=1；HTTP 红测证明 bogus namespace 返回 JSON-RPC `-32033` 且 cancel handler 无副作用，两组准入场景独立进程 20/20 并通过 race。真实单客户端 Runtime HTTP 回归在当前 store 上发送伪造旧 namespace，得到 `idempotency_store_mismatch` 后以 typed read 验证 Session 未创建。`go generate ./...`、合同 digest/public binding/drift、Runtime 全包均通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1752 tests、零泄露，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；未使用 agent-browser |
| 2026-08-13 | P83-08（per-attempt Runtime-store ownership） | Mutation identity 虽在 reserve 与显式 retry 时校验 ownership，但 `createMutationPromise` 的首次 transport recovery replay 直接复用闭包：若首次响应丢失后 Runtime discovery 已发布不同 idempotency namespace，旧 key 会越过 transport 命中新 store，并把另一条业务执行误当成原命令结算；旧 client dispose/successor takeover 后也存在同类 attempt 窗口。现在 journal reservation 在每次真实 transport attempt（含自动 replay、`idempotency_in_progress` replay 与显式 retry）前重新证明同 endpoint/namespace、有效 retention、同 generation 及同 journal active claim。namespace 明确变化或 successor ownership 会 fail closed，旧 key 绝不发送；discovery 暂时撤销与 journal storage 暂不可用只表示结算仍未知，identity 保持 durable/claimable，待同 namespace 恢复后继续以原 key replay。校验仍由 Desktop RPC journal infrastructure 独占，通用 mutation 驱动、Runtime、Protocol、Host KV、Desktop Agent inner ring 与 Go Agent Framework 均不感知 store schema | 两个一客户端产品场景分别在首次 `RpcTransportError` 后切换 store A→B、以及暂时撤销 discovery 后恢复 store A：前者证明自动 replay 与显式 retry 均不打开第二次 transport，新业务 intent 在 B 使用新 key；后者证明 durable entry 未删除，恢复后第二次 transport 精确复用旧 key。两场景独立进程 20/20 通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1752 tests、零泄露，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime `go test ./app/runtime/... -count=1` 全包通过；未使用 agent-browser |
| 2026-08-13 | P83-07（same-renderer mutation handoff generation） | P83-03 已用 immutable generation 隔离不同 renderer/process 的迟到 settlement，但同一 renderer 内 client hot-swap 仍把相同 `PROCESS_OWNER` 误认为同一 mutation owner：旧 client 在读到 generation zero 后若同步经历 `dispose → replacement reserve`，replacement 会复用 generation zero；旧成功随后删除该物理记录和 process-local claim，使新 client 已开始的真实产品 mutation 失去 durable identity，后续确定结算也无法留下 fence。现在只有仍由同一 journal instance 持有 active claim 的显式 retry 才能续用当前 generation；dispose 后的同进程 replacement 与跨 renderer takeover 一样追加更高 generation。物理删除和 owner mismatch 清理改为 compare-owner release，旧 journal 不得释放 successor claim；另一个仍存活的同进程 journal也不能偷取 ambiguous identity，只会创建独立业务 intent。Host storage、Runtime namespace、Protocol、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化 | 确定性交错在旧 settlement 的 owner read 内同步执行 dispose 与 replacement reserve，证明 successor 为 generation 1、旧成功无法删除它、successor 成功后保留 settled fence；相邻回归继续证明同 journal retry 复用 identity、live twin 使用新 key、普通 client hot-swap 复用 key。核心交错独立进程 20/20 通过；Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1750 tests、零泄露，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime `go test ./app/runtime/... -count=1` 全包通过；未使用 agent-browser |
| 2026-08-13 | P83-06（Tool terminal write-set / rollback convergence） | P83-05 把外部调用前的 running Tool Item 与 `ToolInvocationStarted` journal 合并后，继续向终态反证发现两处同一所有权缺口：Application `EventCommit` 仍允许 journal transition 不携带对应 Tool Item，普通 `SegmentEnded` 与清理合成终态又会逐个提交 Item closure、最后才提交 Run terminal；终态事务失败因此可能留下“Item 已终结而 Run/journal 未结算”的半写状态，且 reducer 已消费 open Tool，重试无法重建同一写集。现在 Tool journal 的每个 started/completed/incomplete transition 都必须在同一实际 write-set 内拥有同 identity Tool Item；operational completed 明确允许成功的 completed Item 或带分类 failure 的 incomplete Item，避免把“有结论地失败”误当 unknown。所有正常、RunLost 与清理终态统一通过一个 combined terminal `EventCommit` 原子提交，终态 reducer 先在 clone 上推演，仅提交成功后接管；非原子 publisher 明确拒绝 terminal batch。数据库提交后仍按事件因果顺序登记 Item closure 再登记 terminal Run，保留 Delegate child cancellation 不变量。Protocol、SQLite shape、Desktop 与 Agent Framework 合同均未变化 | Application contract table 覆盖 journal 缺 Item、状态错配、成功、分类失败与 parked attempt；SQLite stale-segment 故障注入证明整个 terminal batch 回滚后两个 running Items 与 started journals 原样保留，正确 fence 再以同 identity 一次结算。终态 transaction failure 回归证明失败 terminal 不发布任何 closure/finish，清理只提交一个包含两个 incomplete Items+journals 的 internal-failure boundary；真实 child cancellation 重复 20 次保持 `child_run_canceled`。Runtime 全包、runs/runsegment/runrecovery/agentexec 定向 race 全绿；真实 SIGKILL 组合恢复与 active MCP Tool 2/2 通过。Frontend `npm run check` 与完整 `--detectAsyncLeaks` 均为 279 files / 1749 tests、零泄露，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；未使用 agent-browser |
| 2026-08-13 | P83-05（SIGKILL Tool journal / read-model atomicity） | 进程级反证先用同一数据目录强杀并重启真实 Runtime，证明 durable Plan、已完成 Tool、paused HITL/Goal、waiting Run、active Goal-owned Run 及 idempotency namespace 能统一恢复；随后把切点下沉到真实 stdio MCP 服务端，确认它已收到并开始外部 `tools/call` 时，Runtime 的 `items.list` 却仍没有对应 running Tool。根因是 reducer 将 `ToolInvocationStarted` 写入 operational journal，却把同一事实产生的 `ItemStarted` 一律当作 provisional stream projection 丢弃，使外部副作用已经越过进程边界，而公开 read model 及崩溃恢复所依赖的 ItemID 尚无 durable owner。现在只有 Tool start 会在同一个 authoritative transaction 内同时持久化 running transcript Item、invocation journal 与 progress，model/reasoning transient start 仍保持 stream-only；终态继续以同 identity 覆盖为 completed/incomplete。修复位于 Runtime Run Application 的事务计划，SQLite Adapter 只执行既有 write-set，Desktop 只消费公开合同；Runtime→Agent 仍仅 `adapter/agentexec`，Agent Framework、Protocol 与 Artifact shape 未变化 | 两个真实 SIGKILL 场景分别覆盖 active Goal-owned Run 与 active MCP Tool：前者重启后 Run=`lost`、Goal=`runNotCompleted` 且 `used.runs=1`，paused HITL/Plan/Tool/namespace 保持；后者以 MCP 进程 marker 证明外部调用已开始，强杀前 running Tool 可冷读，重启后同 Item=`incomplete`、Run=`lost`。定向 SIGKILL 2/2 与完整 Frontend 普通/`--detectAsyncLeaks` 均为 279 files / 1749 tests、零泄露；`npm run check` 全绿且 consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime 定向 race、runsegment/runrecovery SQLite 不变量及 `go test ./... -count=1` 全绿；未使用 agent-browser |
| 2026-08-13 | P83-04（monotonic Runtime-event recovery watermark） | 反证证明 Workspace event loop 虽会对 sequence gap 全量 resync，却在随后仍接受 duplicate/迟到低序帧并把 `lastSequence` 倒退：`1,3,2,4` 会连续触发三次 mounted Agent generation replacement，重复消费已被 authoritative snapshot 覆盖的旧失效，持续乱序时可让 HITL、Plan、Run 与 Tool projection 一直重建而无法稳定。loop 现在以每条 subscription generation 独占的单调 high-watermark 判定事件：仅 `sequence > watermark + 1` 的前向 gap 触发一次 global Query invalidation + mounted Agent `replace-live`；`sequence <= watermark` 的 duplicate/迟到帧直接丢弃，不再执行领域失效也不倒退水位；重连/retarget 仍在新 subscription 内从零开始，因此 Runtime 重启后的新首帧保持既有 resync 语义。组合回归以 Goal sequence 1、Run/Tool sequence 3、迟到 HITL sequence 2、Plan/state sequence 4 证明 gap replacement 已覆盖缺失事实，后续只按单调新帧做 targeted synchronization。修复完全位于 Workspace event Application；wire、transport、Query adapter、Desktop Agent inner ring、Runtime 与 Go Agent Framework 合同均未变化 | duplicate、`1,3,2,4` 和 Goal+HITL/Plan/Run/Tool 恢复边界三条场景在 `--detectAsyncLeaks` 下连续 20 轮全绿；Workspace subscription/Runtime adapter/query invalidation/真实 mounted Session 相邻 5 files / 58 tests 通过。Frontend 普通与完整泄露检测均为 279 files / 1747 tests 且零泄露；完整 `npm run check` 的 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；未使用 agent-browser |
| 2026-08-13 | P83-03（cross-renderer mutation generation fencing） | 反证证明 P79 的 owner/active-claim 检查仍把 Host KV 的分离 `get` 与 `set/remove` 误当成 CAS：旧 renderer 在 dispose 或迟到 settlement 读到自己仍是 owner 后，replacement renderer 若恰好接管同一 identity，旧代随后会把 successor 改回 claimable 或直接删掉。Mutation journal v3 现在按 idempotency identity 保存 generation-addressed records；跨 process/renderer takeover 追加更高 generation，旧 owner 的迟到关闭、成功、未知失败和 retry 只能改写自己的低代记录。接管代得到确定结算后保留到 retention expiry 的 settled fence，压住任何仍可能复活的低代；同形新业务 intent 仍生成新 key，显式重试旧 settled key 则 fail closed。retention 收缩和 Runtime namespace replacement 也只删除不高于已观察上界的代际，清理枚举期间新追加的 successor 不再被误删。已发布 v2 entry 以可重入逐记录迁移进入 generation zero，v1 snapshot 继续经过同一单向迁移。所有 schema、fencing 和迁移仍封装在 Desktop RPC infrastructure；Host 只提供 opaque KV，Runtime、Desktop Agent inner ring 与 Go Agent Framework 合同均未变化 | 三个确定性交错分别锁定 dispose 读后接管再写、迟到成功读后接管再删、retention cleanup 快照后追加 successor；另有 v2→v3 migration 与 successor-settled 后 retired ambiguous failure 回归。四条核心交错在 `--detectAsyncLeaks` 下连续 20 轮全绿；journal 聚焦 28 tests、RPC/Methods/SDK/storage/container 相邻 5 files / 69 tests 均通过。Frontend 普通与完整泄露检测均为 279 files / 1744 tests 且零泄露；完整 `npm run check` 的 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；未使用 agent-browser |
| 2026-08-13 | P83-02（mounted Agent generation replacement / Runtime restart convergence） | 反证证明 Runtime event 新代建立后的全量失效虽然会请求 mounted Agent durable snapshot，但旧 `runs.start/resume/subscribe` opening 或 Run AsyncIterator 若不响应取消，仍会永久占有 Session projection，导致 HITL、Plan、Run、Tool 同步一直排队；Goal/其他 Query read model 已重验，唯独 material projection 无法与同一全量 resync 边界共同收敛。Workspace global resync 现在向 Agent Application 传递中性的 `replace-live` ownership，普通 `runs.changed`/`interrupts.changed`/`state.changed` 仍保持 `after-live` 串行语义；mounted lifecycle 同步撤销旧 opening/stream generation，再复用唯一 snapshot coordinator 重建 durable projection。Run opening owner 同步释放 start latch 和 tracing span，以 generation fence 丢弃并回收迟到 stream；Run pump 的每次 foreign iterator `next()` 与 AbortSignal 竞速，取消获胜后有界调用 `return()`、flush 已接纳 tail、发布 idle boundary，因此不合作的旧代既不能阻塞新快照，也不能在新代后写回。RPC/Runtime DTO、QueryClient、transport 和持久化抽象没有进入 Agent inner ring，Go Runtime/Agent Framework 合同未变化 | 三条红测分别锁定迟到 opening 回收、忽略取消的 active iterator 释放和真实 mounted Session 的 Runtime-restart replacement；集成 fixture 一次 snapshot 同时证明 waiting Run、HITL approval、Plan revision 与 Tool card 收敛，Workspace adapter 证明同一 global boundary 继续失效 Goal/全部 Query read model。聚焦 4 files / 40 tests、Frontend 普通与完整 `--detectAsyncLeaks` 均为 279 files / 1739 tests 且零泄露；完整 `npm run check` 的 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 与 Desktop Agent inner ring→RPC/Runtime internals 均为零 |
| 2026-08-13 | P83-01（non-cooperative subscription cancellation） | 反证证明 Workspace event loop 把 AbortSignal 错当成底层一定合作的完成合同：旧 subscription opening 或 AsyncIterator `next()` 忽略取消时，retarget 会永久停在旧 workspace，新的 app-wide/HITL/Plan/Goal/Run/Tool 事件流无法建立。Application owner 现在让 opening 与每次 iteration 同当前迭代 signal 竞速，取消获胜立即推进 successor；迟到 opening 由自身 iterator `return()` 回收，协作型 pending `next()` 先有界 join，真正不合作的 iterator 不能阻塞新代。`start` 返回中性 lifecycle Promise 供可等待 owner 证明退出；生产同步 unload 仍只发起 abort。改动未引入 RPC/Runtime DTO，也未进入 Desktop Agent inner ring、Go Runtime 或 Agent Framework | 两条红测稳定复现 opening/iteration 各自卡死，修复后 Workspace loop/subscription 2 files / 25 tests 及 Workspace+RPC 相邻层 10 files / 100 tests 在 async detector 下零泄露。前端普通完整门禁 279 files / 1736 tests，全量 `--detectAsyncLeaks` 同为 279 files / 1736 tests 零泄露；type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 与 Desktop Agent inner ring→RPC/Runtime internals 均为零 |
| 2026-08-13 | P82（Workspace event generation ownership / resubscription convergence） | Desktop Workspace event Application 现在显式拥有唯一活跃 subscription generation：重复 `start` 会先撤销前代内部 cohort，即使前代调用方遗漏 abort 也不会留下第二条 stream；调用方 signal 仍由外部 owner 管理，前代迟到 cleanup 由 generation fence 隔离，retarget 继续只替换当前 iterator。订阅建立后的完整 resync 仍由 Workspace Application 统一失效 QueryClient 全部 read model，并同步 mounted Agent material projection；逐 context 复核确认 project index 有意派生自 Session query projection，没有为旁路事件引入 Workspace→Agent/Runtime 内环反向依赖。Runtime、Protocol、SQLite 与 Go Agent Framework 均未变化 | 新红测覆盖调用方未 abort 时连续 start，证明外部 signal 不被越权撤销、旧内部 subscription 被终止且新代保持活跃；Workspace event loop/subscription 聚焦 2 files / 23 tests 在 async detector 下通过。前端完整 `--detectAsyncLeaks` 279 files / 1734 tests 零泄露，普通 279 files / 1734 tests及 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿；consumer 为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 与 Desktop Agent inner ring→RPC/Runtime internals 均为零 |
| 2026-08-13 | P81（renderer graceful teardown / Runtime process restart convergence） | P80 建立的 Desktop container dispose owner 现已接到真实生产窗口生命周期：`main` composition root 向 Host bridge 注入唯一 core teardown，bridge 先按注册顺序执行 plugin before-unload handlers，再启动基础设施关闭；`LyraClient.close()` 在浏览器无法 await 的边界仍会同步释放 mutation journal heartbeat/claim，随后异步 join HTTP/receive loop。window-chrome resize watch 返回精确 disposer 并由同一 root 回收。最终 dispose 只关闭 composition root 创建的 client，测试/embedding 注入 client 仍由外部 owner 管理；Host bridge 只认识中性 teardown callback，不依赖 Runtime client，Runtime plugin、Desktop Agent inner ring 与 Go Agent Framework 均不感知 window/unload。真实 HTTP harness 抽出 Runtime process start/stop owner，SIGTERM 有界 join、超时才 SIGKILL；同一数据目录和监听地址完整重启后重新建立 client/event subscription | 回归证明 plugin unload handler 先于 host teardown、最终 dispose join owned client 且不关闭 injected client；真实 Go Runtime HTTP→TypeScript SDK 提升至 41/41，新增进程重启场景证明 idempotency namespace 稳定、重启前 Session 可 cold read、重订阅后新 `sessions.changed` 精确到达。前端 `--detectAsyncLeaks` 完整 279 files / 1733 tests 零泄露，普通 279 files / 1733 tests 及 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿；consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 与 Desktop Agent inner ring→RPC/Runtime internals 均为零 |
| 2026-08-13 | P80（Desktop asynchronous ownership / quiescent teardown） | Desktop composition root 显式拥有它创建的 RPC client：endpoint/token/storage identity 热切换同步发布 replacement 后等待 retiring owner 完整 dispose；测试注入 client 仍由注入方拥有。低层 `RpcClient.close()` 无论 transport close 成败都会 join 唯一 receive loop，保留 teardown 原始错误，不再在 generator `finally` 退出前宣告关闭。Mutation settlement、Run opening 与 Runtime inspection 的 deadline loser 在成功、失败、dispose 后均显式释放。真实 HTTP/E2E deadline 与进程 listener、Agent hook/pump、TanStack Query、Shiki、Motion/Base UI 等测试资源按创建 owner 收口；组件 browser-task drain 只在对应 primitive 的测试卸载后执行，不全局接管 Runtime E2E 的长生命周期 timer。改动仅位于 Desktop composition/RPC infrastructure、Runtime plugin Application 或测试夹具；Desktop Agent Application/Domain 与 Go Agent Framework 未接触 client、transport、deadline、journal 或 UI scheduler 抽象 | 红测证明旧 client close 早于 receive generator unwind、hot-swap 遗留 recv loop、race loser 永久悬挂与 UI/query fixture 残留；修复后前端 `--detectAsyncLeaks` 完整 278 files / 1729 tests 零泄露，普通 278 files / 1729 tests 及 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全绿。真实 Go Runtime HTTP→TypeScript SDK 40/40 覆盖 discovery、Session、Run、HITL、Plan、Goal、Schedule、provider/role、Knowledge/Memory、MCP、Skill、codebase 与 files.changed；consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。`go test -count=1 ./app/runtime/...` 全包通过；Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 与 Desktop Agent inner ring→RPC/Runtime internals 均为零 |
| 2026-08-13 | P79（multi-renderer lease generation / crash takeover fencing） | Desktop mutation journal 将 owner heartbeat 从“原地覆盖同一 lease key”升级为不可变 generation：每次续租先写入新的 generation，再回收本 renderer 的上一代；另一 renderer 只会删除已经不可再续租的过期 generation，因此不需要 Host/localStorage 不具备的 compare-and-delete，也不会把 crash owner 永久积累为 tombstone。已发布的可变 v1 lease 仍可读取并参与 active cohort，其过期记录仅作为有限的升级兼容 tombstone 保留。崩溃接管在 entry owner transition 前重新读取 owner cohort，若原 owner 已续租或当前 renderer 已不再是确定性 leader 就 fail closed；真正 lease 过期时 replacement renderer 仍继承原 idempotency key，retired renderer 的迟到 settlement 由 entry owner + client claim 双重 fencing，不能删除或改写 successor。该机制继续完全封装在 Desktop RPC infrastructure；Host 仍只实现 opaque KV，Runtime 只发布既有 opaque namespace/retention，Desktop Agent Application/Domain 与 Go Agent Framework 均不感知 owner、heartbeat、journal 或 transport replay 抽象。SDK teardown fault fixture 同时修正为先关闭真实 memory channel、再注入 close failure，错误聚合测试不再遗留 recv-loop Promise | 红测精确注入“读取过期 generation 时另一 renderer 已写入续租 generation”的交错，旧实现会按 owner stable key 删除刚续活的 lease 并错误接管；新实现只回收不可变旧代，第二次 cohort read 观察新代并阻断 contender。另以两次独立模块加载构造真实不同 PROCESS_OWNER，证明活跃窗口阻断、crash lease 到期后 exact-key 接管、retired late success 不覆盖 successor、successor replay 后收敛；heartbeat/dispose 回归证明多 journal generation 独立拥有、失败遗留旧代在 dispose 统一回收。两条竞态场景连续 20 轮全绿；Frontend journal/Methods/SDK 定向 3 files / 52 tests 在 `--detectAsyncLeaks` 下零泄露，完整 278 files / 1728 tests 以及 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全门禁通过，consumer 仍为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime 现有同库 reopen namespace、durable idempotency replay、HITL/Run crash recovery 与 cold-read 测试经盘点已覆盖进程重启边界，本轮 `go test -count=1 ./app/runtime/...` 全包通过，未复制一套第二 lifecycle owner |
| 2026-08-13 | P78（RpcClient teardown ownership / cross-context replay convergence） | Desktop 低层 `RpcClient` 将 request admission/correlation closure 与 transport teardown ownership 分离：recv clean/error EOS 仍会停止接纳并终止 pending/subscriber，但不再冒充底层资源已回收；显式 close 无论连接是否先断都恰好调用一次 transport close，并发调用共享同一 teardown promise 和结果。真实 Runtime fault oracle 扩展到 Goal start/stop/resume、Run HITL resume、Provider、utility/embedding Role、Knowledge、Agent Memory add/update/delete、MCP create/update/delete 与 managed Skill archive/restore：每个命令在业务已提交后丢失 HTTP body 或 streaming opening acknowledgement，再以同一 identity 重放，并通过独立 client 的 cold read、对应 Runtime event、Run/Item/interrupt 数量和文件 lifecycle 证明首次结果胜出、无重复 Run/continuation/item、无 revision 漂移、无 secret 泄露或半完成 move。关闭修复仍只属于 Desktop RPC infrastructure，故障注入只属于 E2E；Goal/HITL/配置/文件业务语义均留在 Runtime 公开合同与所属 context，Desktop Agent Application/Domain 和 Go Agent Framework 未接触 transport/idempotency 抽象 | 两条红测稳定证明旧 RpcClient 的并发 close 后到调用提前成功，以及 recv EOS 后显式 close 完全跳过 transport；修复后 client 15/15，通过两条场景连续 20 轮。RPC/HTTP/SDK/journal/Methods/真实 Runtime 组合 6 files / 118 tests 全绿；真实 Go Runtime HTTP→TypeScript SDK 提升至 40/40，完整场景在聚焦组合与全量各通过一次，另有各 fault 组独立通过。Frontend 278 files / 1725 tests 与 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全门禁全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；事件 supervisor 的 sequence gap/first-frame/retarget/stale-opening owner 复核无新泄露 |
| 2026-08-13 | P77（HTTP transport quiescent close / real replay cutpoints） | Desktop HTTP transport 的关闭语义从“只广播 abort 并 fire-and-forget reader cancel”收敛为真正 quiescent close：所有关闭前已接纳的 fetch/body read、SSE reader cancel 与 background drain 都由 transport 唯一 owner 追踪并 join；并发 close 共享同一 promise，fetch 在 abort 后延迟退出、response 与 abort 并发安装 reader 等切点都不能让 `client.close()` 提前宣告资源已释放。关闭后 admission 继续由既有 synchronous closed-channel guard 拒绝。真实 Runtime HTTP 黑盒故障注入在 transport 外模拟 commit 前断线、commit 后完整执行但 response body 丢失，以及 streaming response 首帧丢失：Session create/delete、Run start、Schedule create/runNow/update/delete 均以同一 idempotency identity 自动重放，只产生一个权威实体/Run，并由独立 cold read 与 `sessions.changed`/`schedules.changed` 证明最终收敛。生命周期机制只位于 Desktop RPC infrastructure，fault oracle 只位于 E2E；Runtime Application/Domain、Desktop Agent 与 Go Agent Framework 没有接触 fetch/reader、wire replay 或 client close 抽象 | 两条红测先稳定证明旧 close 在 ignored-abort fetch 与延迟 SSE cancel 尚未释放时已返回；修复后两条用例连续 50 轮（100 次释放断言）全绿。真实 Go Runtime HTTP→TypeScript SDK 提升至 35/35，并连续 5 轮覆盖 Session unary 前/后提交、Run streaming opening 丢失与 Schedule 全 mutation cold-read/event 收敛。Frontend 278 files / 1718 tests 与 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全门禁全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；Runtime `go test -count=1 ./app/runtime/...` 全包通过。Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 为零 |
| 2026-08-13 | P76（Desktop client hot-swap fencing / ambiguous storage confirmation recovery） | Desktop RPC journal 现在以每个 client instance 的 active claim 作为同进程 fencing token：endpoint/token/storage identity 热切换关闭旧 client 时，未决 identity 先持久化为可接管，replacement 同步复用原 key；旧 transport 的迟到成功、未知失败与内部 replay 均不能再删除或改写 successor entry，外部 renderer 已改 owner 时同样 fail closed。若 handoff 持久化失败，仅在当前进程保留跨 storage-adapter identity 的 opaque key fallback，成功交接后立即释放；共享同一 storage 的多个 client 由精确 claim registry 保持 owner lease，最后一个 owner 退出才回收。更深一层的 write-then-throw 切点也已闭合：Methods 在 transport 前持有候选 key 并传给 journal，写入已落盘但确认失败时只有同一 `MutationPromise.retry` 能按 exact key 接回，同形 fresh intent 仍使用独立 key；确定未发送而遗留的 entry 在 journal dispose 时删除，不占用容量。SDK close 即使 journal cleanup 失败也必定关闭 transport，双重失败以 `AggregateError` 完整保留；并发 close 共享唯一 cleanup settlement，后到调用不再因内部 closed 标志误报成功。所有 schema/key/owner/settlement 仍只存在于 Desktop RPC infrastructure，Host storage 仍由 Runtime plugin Adapter 提供 opaque KV，Agent Application/Domain 与 Go Agent Framework 零感知 | 红测覆盖 replacement 接管与 retired late-settlement fencing、外部 renderer supersede、shared-storage 最后 owner lease、跨 adapter handoff 写失败、write-then-throw exact retry/fresh twin/unsent dispose、Methods 级 token hot-swap same-key replay、journal/transport 双清理失败与并发 close 同一结果。55 项定向用例连续 20 轮全绿；Frontend 278 files / 1713 tests 与 type/lint/format/knip/circular/context/published-boundary/layers/port/API consumer/design/i18n/bootstrap/bundle 全门禁全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；真实 Go Runtime HTTP→TypeScript SDK 32/32 覆盖 Run/HITL/Plan/Goal/旁路事件，Runtime `go test -count=1 ./app/runtime/...` 全包通过。最终 agent-browser session/state/daemon/Chrome/监听、Runtime/Vitest/E2E 进程与临时日志均为零 |
| 2026-08-13 | P75（multi-window durable mutation ownership / Goal terminal cleanup convergence） | Desktop mutation journal 从单一 v1 snapshot 改为逐 entry / owner 的 v2 verified KV records：不同 renderer 不再以整表 read-modify-write 互相覆盖；Host storage 的 set/remove/keys 都以读回或唯一 probe 验证，quota、禁用、吞错、枚举残缺均在 transport 前 fail closed。每个 entry 自带 salt/fingerprint，v1 已发布 snapshot 以可中断、可重入的逐记录迁移升级，全部 v2 写成功后才删除 legacy；未决记录达到容量时拒绝新 mutation，不再淘汰仍可能提交的 identity。跨窗口以 owner lease + heartbeat 证明存活：另一活跃窗口持有同形 mutation 时拒绝重复发送，lease 过期后只由最老 live owner 接管原 key；同 renderer 的 fresh 同形 intent 仍保持独立 key，ambiguous settlement、写/删失败、retry claim 失败均保留并在恢复后复用同一 identity，retention 收缩与 Runtime namespace 替换会精确回收旧 scope。SDK close 释放 heartbeat，composition root 在 endpoint/token/storage identity 切换时关闭旧 client。Runtime context 只提供 opaque KV port，Host/localStorage 留在 Adapter，RPC 独占 journal schema、owner/key/fingerprint 与错误分类；Desktop Agent Application/Domain 和 Go Agent Framework 均零感知。全量高负载联调另复现 Goal budget 已 durable blocked、root boundary 尚在途时 Resume 取消 drive，Run cleanup 把正确的 `ErrRunFinished` 投成 `internal_error`；Application cleanup 现把 Run missing/finished 都视为已收敛终态，预算拒绝继续由 Goal aggregate 决定 | journal 红测覆盖独立 entry 无 lost update、敏感参数不落盘、同进程 twin、崩溃接管、活跃窗口阻断/lease 到期、确定/未知 settlement、claim/set/remove 故障恢复、v1 中断迁移、retention 收缩、容量满不淘汰、heartbeat dispose、Host 吞写/吞删/枚举残缺与唯一 probe；Methods 证明持久化恢复前 transport 调用为零，composition root 证明 token 热换先关闭旧 client。Goal cleanup 单测 100×、真实 budget HTTP 场景连续 20× 全绿。Frontend 278 files / 1703 tests 与 type/lint/format/knip/层级/发布边界/API consumer/design/i18n/bootstrap/bundle 全门禁全绿，consumer 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；Runtime 全包禁缓存、全量 race、generate、workspace/standalone tidy/build/vet、staticcheck 全绿。Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 为零；最终 agent-browser state/session/daemon/listener、Runtime/Vitest/E2E 进程及临时日志均清零 |
| 2026-08-13 | P74（durable idempotency namespace / Desktop cold-restart mutation journal） | Runtime SQLite epoch 70 新增与 durable replay store 共存亡的 opaque idempotency namespace：同库关闭/重开稳定，同路径删库重建必变；Persistence 只把它交给 Bootstrap，Delivery 在 discovery `limits.idempotency` 发布，Application/Domain/Agent Framework 均不可见。Desktop RPC 为 contract registry 中全部 replayable unary/stream mutation 建立统一 journal：transport 发送前持久化 endpoint + Runtime namespace + retention + 非明文 method/params 匹配指纹 + key，不保存 prompt、credential、文件内容或完整 params；确定结果立即删除，唯一 settlement-unknown 判定决定是否保留。process owner/claimable 状态阻止同进程 client 重建或 fresh 同形 intent 误领 active key，真正 Desktop 冷重启后才恢复未决 identity；endpoint、namespace、retention 任一不匹配均 fail closed。Runtime context 只拥有 opaque snapshot storage port，Host localStorage 机制留在 Adapter，RPC journal/Runtime wire/idempotency key 未进入 Agent Application/Domain；composition root cache 同时绑定 endpoint/token/storage adapter identity，提前构造的无 journal client 会在 Adapter 安装后重建 | 红测覆盖 SQLite namespace 重开/换库/损坏、真实 Bootstrap discovery 重开、journal 写前持久化/敏感参数不落盘/同进程不合并/跨进程恢复/确定与未知结果/HTTP 400/换 endpoint/换 namespace/过期/损坏 snapshot、全 Methods same-key replay 与 client cache Adapter late-install。Runtime 全包禁缓存、全量 race、generate、standalone tidy/build/vet、staticcheck、contract/architecture 全绿；Frontend 278 files / 1690 tests 及完整 type/lint/format/knip/层级/发布边界/design/i18n/bootstrap/bundle 门禁全绿，consumer 仍为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；真实 HTTP→TypeScript SDK 32/32 校验 namespace、Run/HITL/Plan/Goal 与全部旁路。Runtime→Agent 生产 import 仍仅 `adapter/agentexec`，Agent→Runtime 为零。额外清理旧 `lynx-p42-idempotency`/`lynx-p42-draft-cold` agent-browser 状态；最终 state/session/daemon/listener/E2E 进程均为零 |
| 2026-08-13 | P73（Desktop settlement-unknown logical mutation ownership） | Desktop RPC 新增 adapter-scoped unary mutation settlement owner：Session create 与 Goal start/stop/resume 在两次有界 delivery 后仍未知时保留原 `MutationPromise`，下一次显式产品重试只调用其 same-key `retry`，成功或确定性拒绝才释放。Run start/resume 由 streaming opening 专属 owner 实现同一身份规则，同时把 deadline 与成功后的 event-stream signal lifetime 分开；即使 transport 忽略 AbortSignal，两次预算也会独立结束。settlement-unknown 队列只复用已经返回未知的命令，不合并 fresh 同形调用，欢迎页首消息创建与工具栏空白创建等独立 Agent 意图仍由 Application owner 区分。未知判定集中在 RPC mutation 层，semantic identity 只在 Runtime adapters；Agent Application/Domain 与 Go Agent Framework 均未接触 idempotency key、wire error 或 Runtime lifecycle | 红测先证明 Session/Goal/Run/HITL 在第二次 timeout 后的产品重试重新 `open` 并换 key，也证明忽略 abort 的 Run opening 永久挂起；全量门禁又抓到错误合并两个 fresh `sessions.create` 的层级反例。修复后 core settlement/gateway 回归、Frontend 276 files / 1681 tests 与完整 type/lint/format/knip/层级/发布边界/API consumer/bundle 全绿，保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；真实 HTTP→TypeScript SDK 32/32 覆盖流式 Run、HITL、Plan、Goal。Runtime 全包禁缓存/全量 race、tidy/build/vet/staticcheck/generate 全绿；Runtime→Agent 实际生产 import 仍仅位于 `adapter/agentexec`（含 `interactioninput` ACL），Agent→Runtime 为零；本轮 E2E/Lyra/Wails 监听与 agent-browser session 回收为零 |
| 2026-08-13 | P72（crash-abandoned idempotency reservation safety） | opaque idempotency Store 将 24 小时 retention 精确限定为 completed result replay window；空 payload 表示业务 commit 是否发生仍未知的 reservation，不能因 wall clock 到期而释放给同 key 再次执行。SQLite prune 只回收已完成记录，aged pending 在关闭/重开后仍保持 fingerprint 绑定；其迟到 completion 可落地并从完成时重新开始 24 小时窗口。内存 Store 与 SQLite 共享同一状态语义，Protocol discovery/Transport 文档明确 completed 与 pending 的区别；Store 仍只认识 key/fingerprint/opaque payload，不知道 method、Run、Plan、Goal、invalidation 或 Agent 类型 | 红测稳定复现 aged empty claim 在 SQLite/内存中被当作全新 claim，意味着 crash 后同 key 可第二次运行 handler；真实 SQLite close/reopen 反例同时证明旧 reservation 会被首次后续 claim 清除。修复后两条 SQLite 场景 normal 100×/race 20×、内存 race 100×、operation+SQLite 全包 normal 20×/race 10× 全绿。Runtime 全包禁缓存/全量 race、tidy/build/vet/staticcheck/generate 与真实 HTTP→TypeScript SDK 32/32 全绿；Desktop 275 files / 1674 tests 与完整架构/consumer/bundle 门禁全绿，保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；双向依赖与测试资源回收为零 |
| 2026-08-13 | P71（graceful replay settlement drain / durable-first reconciliation） | `operation.Endpoint` 的 shutdown join 在所有已接受 unary/stream source 返回后、Store/Host/lease 关闭前，继续排空同一 Delivery replay owner 已知但尚未持久化的 outcome；排空失败使 Runtime Close 保持资源所有权且可重试，shutdown budget 会贯穿 Store 调用而不被 request-detached 写入语义吞掉。Settlement 每次成功/丢回执后都重新读取 durable record，以其 first result 为权威；claim 丢失只重新 claim 并写入已知 payload，任何路径都不重新执行业务 handler 或 invalidation。Application taskgroup、事务、Agent Framework 和公共 API 均未接触 Delivery pending/Store 类型 | 两条红测稳定复现旧 Close 成功后重开仍停在 `idempotency_in_progress`，以及 completion 再失败时旧 Close 错误放行；第三条红测复现 Store 已持久化竞争 first result 但回执丢失后，当前进程重放内存副本、重启后重放 durable 副本的不一致。新增 shutdown flush 成功/失败保留并重试/owner cancel、durable-first、lost-claim 并发回归，定向普通 50×、operation 全包 20×、race 20× 与 Bootstrap shutdown 10×、Bootstrap race 10× 全绿。Runtime 全包禁缓存/全量 race、tidy/build/vet/staticcheck/generate 与真实 HTTP→TypeScript SDK 32/32 全绿；Desktop 275 files / 1674 tests 及完整架构/consumer/bundle 门禁全绿，保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；双向依赖与测试资源回收为零 |
| 2026-08-13 | P70（idempotency lost-claim recovery / MCP lifetime contract oracle） | Delivery 的幂等重放 owner 在业务结果已知、receipt completion 失败后保留同一 fingerprint/outcome；重试若发现原 claim 已丢失，会先重新 claim 再只持久化已知结果，绝不重新执行命令或旁路 invalidation。若其他 owner 已经持久化结果，则 durable first result 胜出。MCP 测试同时把 request scope、handshake timeout 与 live session lifetime 三个不同合同拆开：真实 HTTP 只证明成功配置不继承请求取消，受控 transport 精确证明 handshake deadline 不进入 session lifetime 且确实取消 in-flight connect，不以扩大 timeout 掩盖负载抖动 | 红测稳定复现首次 completion 丢 claim 后第二次永久 `idempotency_in_progress`；修复后 16 个并发重试全部得到同一 replay，业务调用保持 1 次，定向普通 100×、race 50× 全绿。原 MCP 20ms 真实网络竞态在全量负载下复现后由确定性合同测试替代，定向普通 100×、race 20× 全绿。Runtime 全包禁缓存/全量 race、tidy/build/vet/staticcheck/generate 与真实 HTTP→TypeScript SDK 32/32 全绿；Desktop 275 files / 1674 tests 及完整门禁全绿，保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime→Agent 实际 import 仍仅位于 `adapter/agentexec`（含 `interactioninput` ACL），Agent→Runtime 为零；测试后 agent-browser session、Wails/Lyra/E2E 进程与相关监听端口均为零 |
| 2026-08-13 | P69（Endpoint admission / in-flight operation join） | binding-neutral `operation.Endpoint` 新增唯一 invocation ledger：每个已接受 unary call、已开始 range 的 stream source、以及返回后从未 range 的 stream 都从 admission 到真实 source return 持有同一 lease。Runtime Close 先同步关闭 Endpoint admission 并广播取消，再在同一 Instance shutdown budget 内等待 ledger 清零，之后才 join scheduler/Host、关闭 SQLite/工具资源并释放数据目录锁；未开始的 stream 在取消时由 owner 以 rejecting yield 主动进入 source，确保 source-side watcher/subscription teardown 已完成，而不是把 `context.AfterFunc` 已调度误当作已回收。该机制严格属于 Delivery operation 与 Bootstrap process owner，没有复用 Application taskgroup、没有改变已接受 Run 脱离请求取消的语义，也未向 Agent Framework、Protocol 或公共 embedded API 暴露内部 lifecycle 类型 | 红测稳定复现 blocking unary 已观察 Runtime 取消但仍占用依赖时，旧 Close 已关闭 Host resource 且提前返回；新增未 range stream 的 shutdown claim/join、已 range source 延迟退出时 Await 必须超时、post-shutdown stale iterator clean end 回归。定向普通 20×、operation race 20×、Bootstrap race 10× 与 Runtime 全包禁缓存/全量 race、tidy/build/vet/staticcheck/generate 全绿；真实 HTTP→TypeScript SDK 32/32 覆盖流式 Run、HITL、Plan、Goal，Desktop 275 files / 1674 tests 与完整门禁全绿，consumer gate 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。Runtime→Agent 实际 import 仍仅位于 `adapter/agentexec`（含 `interactioninput` ACL），Agent→Runtime 为零；测试后 agent-browser session、Wails/Lyra/E2E 进程与相关监听端口均为零 |
| 2026-08-13 | P68（Desktop stream source termination / exact listener teardown） | Desktop RPC stream 将自然终止的资源回收从“等待消费者下一次读取 `done`”收敛为 source-owned lifecycle：Run 根 Segment 终态、Run/Runtime transport stream-end、source error、caller abort、显式 dispose 与消费者 early return 共同进入一个幂等 teardown owner；即使调用成功后尚无人迭代、或 response head 已在 bind 前完整到达，也会立即注销 notification 与 stream-end listener 并中止该 stream 自有 request signal。Channel 仍负责保留已缓冲事件，晚到消费者可完整读到终态，transport teardown 不再篡改 caller-owned signal。修复仅位于 Desktop RPC Adapter，未把 Runtime wire/lifecycle 下沉到 Workspace/Agent fold 或 Go Agent Framework，也未改变后端公共流 API | 两条红测分别稳定复现 Run 根终态与 Runtime clean stream-end 在无消费者时各残留 2 个 RpcClient registration；新增 bind 前已终态 head 的 late-consumer 回归。修复后 channel/stream 29 项定向测试与原两组场景 50× 全绿；Desktop 275 files / 1674 tests、type/lint/format/knip/层级/发布边界/API consumer/bundle 全绿，保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；真实 HTTP→TypeScript SDK 32/32 覆盖流式 Run、HITL、Plan、Goal。Runtime 全包禁缓存、全量 race、tidy/build/vet/staticcheck/generate 全绿；实际 import graph 仍只有 `adapter/agentexec`（含其 `interactioninput` ACL）消费 Agent，Agent→Runtime 为零；测试后 agent-browser session、Wails/Lyra/E2E 进程与相关监听端口均为零 |
| 2026-08-13 | P67（Run journal overflow / subscriber registry lifetime） | Run journal 将 authoritative overflow 从“只中止 subscriber、等待 consumer 开始 range 后 deferred Cancel”收敛为完整的 registry ownership transition：同一 publish 在 journal lock 下判定订阅已断开并立即从 `subs` 摘除，未开始消费、stream ack 尚在写入或调用方不再 range 都不会让 aborted subscriber 被当前 Segment 永久保留并在后续每次 publication 重访；live-only headroom 溢出仍按既有 lossy 语义只丢事件而保持订阅。修复止于 `application/runs` 的既有 journal owner，没有向 Delivery、Desktop Agent fold 或 Go Agent Framework 增加第二 lifecycle owner，也未改变 Protocol、SQLite、Artifact 或公共 API shape | 红测稳定复现 retention=2 时第三个 authoritative event 中止尚未 range 的 subscriber、但 journal 仍保留 1 个注册项；修复后定向普通 100× 与 journal race 20× 全绿。Runtime 全包禁缓存与全量 race、tidy/build/vet/staticcheck/generate 全绿；真实 HTTP→TypeScript SDK 32/32 场景覆盖流式 Run、HITL、Plan、Goal；Desktop 275 files / 1671 tests 与完整门禁全绿，consumer gate 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。跨 Session/Segment 晚到帧审计确认 effect cleanup、rAF batch dispose、view epoch/revision、Run/Item/Plan 单调 fold 与 dropped-session no-op 已闭合；实际 import graph 仍只有 `adapter/agentexec`（含其 `interactioninput` ACL）消费 Agent，Agent→Runtime 为零 |
| 2026-08-13 | P66（child-start callback ledger lifetime / crash cleanup） | `child_run_start_reservations` 从永久 append-only 技术表收敛为准确的 active-tree/process callback ledger：正常 root terminal 在 Run/interrupt/checkpoint 同一 transaction 内按 Session 回收；parked terminal、rollback、restore、Session delete 复用各自既有 Session write-set 回收；boot recovery 即使没有公开 Run 需要修复，也在 recovery transaction 内清空上个进程全部 callback receipt。conclusion 仍在 live tree 内支持 exact replay，跨 Session 行不被 root terminal 误删；cleanup 失败让完整 terminal/recovery transaction 回滚。生命周期只由 Runtime Adapter/Infra 实现，Application 仍只拥有中性的 child-start handshake，Agent Framework、Protocol、Delivery 与 Desktop shape 均未变化 | 红测稳定复现正常 root 完结、模拟 reserve 后进程遗留/空 recovery 与 Session delete 后 reservation 永久残留；新增 owner-scoped/all-process cleanup、terminal cleanup failure rollback、boot cleanup failure 回滚先前 checkpoint deletion 的反例。受影响五包 race 5×、Runtime 全包禁缓存与全量 race、vet/staticcheck/build/tidy/generate 全绿；Desktop 275 files / 1671 tests 与完整门禁全绿，consumer gate 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；实际 import graph 双向反证 Runtime→Agent 只存在于 `adapter/agentexec` 且 Agent→Runtime 为零 |
| 2026-08-13 | P65（Run recovery monotonic time / exact live Segment ownership） | Boot recovery 不再假设重启后的墙钟一定前进：Application 从 active Run `updatedAt`、Pending barrier、Transcript occurrence/finish、未闭合 model/Tool attempt 的完整 durable snapshot 计算统一 high watermark，并在生成任何 lost Run、incomplete Item 或 unknown invocation 前固定 recovery observation time，机器校时回拨不再令 Runtime 永久启动失败。Run process registry 的删除权从 `runId` 收窄为 `(runId, segmentId)`；旧 pump 在 maintenance admission 释放后不能删除已 resume 的 replacement Segment。Subscribe/Steer 的共同寻址边界再次核对 live Segment identity，durable read 与 registry lookup 横跨 HITL park/resume 时返回 `ErrStaleSegment`，绝不把旧请求静默投递到新 Segment。全部策略和竞态仲裁留在 `application/runs`，SQLite、Delivery、Protocol、Desktop 与 Agent Framework shape 均未变化 | 红测分别稳定复现时钟回拨导致 Run/Transcript/model/Tool recovery 拒绝、旧 Segment 删除 replacement registry entry、旧 subscribe 误接 racing resume；修复后定向 race 20×、Runs 全包 race 10×、Bootstrap race 5×、Runtime 全包禁缓存、vet/staticcheck/build/tidy/generate 全绿。真实 HTTP→TypeScript SDK 32 场景继续覆盖流式 Run、HITL、Plan、Goal；Desktop 275 files / 1671 tests 与完整门禁全绿，consumer gate 保持 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites；Go 实际 import graph 双向反证 Runtime→Agent 只存在于 `adapter/agentexec` 且 Agent→Runtime 为零 |
| 2026-08-13 | P64（Goal boot recovery fail-closed CAS） | Goal startup reconcile 不再丢弃 active→paused 的 `Save applied=false`，也不再用无条件 Clear 删除 orphan/complete snapshot；三类恢复都以 `List` 得到的 incarnation+revision 执行 `Save`/`ClearIf`，任何 CAS 未落地都返回 `ErrGoalConflict` 并阻止 Runtime 进入“active Goal 无 drive”或误删更新 incarnation 的半恢复状态。Goal Application 的交互/恢复 `Store` 删除无条件 Clear 能力；只有 session 聚合原子 delete 所需的更宽 `DurableStore` 保留 Clear，避免 persistence 能力反向扩散到 Driver/Reader/Reporter。Protocol、Delivery、Desktop 与 Agent Framework shape 均未变化 | 新 faulting-store 回归分别证明 active pause 与 complete clear 的 CAS miss 必须 fail closed 且原 snapshot 不变；Goal 20× race、SQLite/Bootstrap 5× race、Runtime 全包禁缓存、vet/staticcheck/build/tidy 全绿。真实 HTTP→TypeScript SDK 32 场景继续覆盖流式 Run、HITL、Plan、Goal；本轮未改 Desktop，上一轮同一 HEAD 基线的 275 files / 1671 tests 与 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites 完整门禁保持全绿 |
| 2026-08-13 | P63（maintenance compaction convergence / loss prevention） | Run-boundary Compactor 修正 turn-safe cutoff 的单向搜索：优先使用目标位置后的 User 边界，最后一轮内无后继 User 时退回该轮起点，单轮超预算时允许压缩完整已完成历史；`KeepRecent` 明确只是保留偏好，不再令短历史或已经独立超限的 recent suffix 永久逃逸。确定性 Tool argument/result 裁剪先限制在最终可摘要前缀；若 recent suffix 单独超限则扩展到完整历史，仍不够才摘要完整历史，从而不再逐轮重复无效前缀摘要。空白模型摘要在原子 Replace 前被拒绝，保留原历史，避免无错误的数据丢失。全部变化留在 `adapter/runmaintenance` 的既有 history policy owner，Application/Domain/Delivery/Protocol/Desktop/Agent Framework shape 均未变化 | `runmaintenance` 新增最后一轮无后继 User、短历史多轮、短历史巨大 Tool result、recent suffix 独立超限、空摘要不替换等回归，20× race 全绿；Agent executor、Bootstrap、Runs/Goals 5× 受影响回归及 Runtime 全包禁缓存、vet/staticcheck/build/tidy/generate 全绿。真实 HTTP→TypeScript SDK 32 场景继续覆盖流式 Run、HITL、Plan 与 Goal；Desktop 275 files / 1671 tests 及全量 type/lint/format/架构/API consumer/bundle 门禁全绿，仍为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites |
| 2026-08-13 | P62（external Skill convergence / exact mutation observation） | authored-resource observation 补齐 user Skill root 与每个 project `.lyra/skills` 动态树：Infra 只观察精确 `SKILL.md` 内容与 physical identity，Workspace Adapter 独占路径布局，Application 只持有 `AuthoredSkills` 与 opaque identities，Delivery 复用既有 `skills.changed`，未向 Agent Framework、Protocol 或 Desktop Agent 内层泄露 Runtime/文件系统抽象。Skill authoring 的 submit/approve/reject/archive/restore/idle sweep 统一返回实际提交的文件 identities，包括“文件 move 已提交、后续 metadata 失败”的部分成功；Application 先精确接受这些 baseline 再发布一次失效，幂等 replay 与未提交错误不再误发，filesystem 回声不再重复发。动态 watcher 覆盖 missing root、目录增删、atomic replace、域内 symlink、域外 containment、非普通文件、无关 sidecar、close join 与同树并发外部变化 | Runtime 全包禁缓存测试、vet/staticcheck/build/tidy/generate 全绿；受影响包 3× 普通与两组 race（最高 10×）全绿，fileobservation 10× 通过。Desktop 275 files / 1671 tests 及 type/lint/format/knip/层级/发布边界/API consumer/bundle 全绿，仍为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。真实 HTTP→TypeScript SDK 32 场景验证 API archive/restore 各仅一条事件，外部 project/user Skill 创建均收到 `skills.changed` 且随后冷读收敛 |
| 2026-08-13 | P61（Run maintenance semantic ownership / exact invalidation） | Run-boundary Memory curation 与 idle Skill archival 从直接驱动 Infra Store 改为各自独立的 Application 用例对象；Review Coordinator、Workspace Skills 的交互面不再携带后台维护方法，Run-maintenance Adapter 只依赖 `PublishGeneration` / `ArchiveIdle` 语义端口，原始 SQLite/Skill Store 无法再靠同名方法误接。Memory 仅在 generation CAS winner 后发布 `agentMemory.changed`，ledger/embedding 隐藏写和 lost CAS 不误发；Skill sweep 对已经归档但后续 metadata 失败的部分提交仍精确发布。SQLite `agentMemory.add` 显式返回 created fact，重复/并发去重成功但不再制造虚假事件；Desktop 删除 `runs.changed → agentMemory` 跨聚合猜测，只消费专用事件 | Runtime 受影响包普通测试、架构门禁、全包禁缓存、20× race（32 路 generation 竞争、32 路 sweep admission、16 路 duplicate Add）及 vet/staticcheck 全绿；Desktop 275 files / 1671 tests、type/lint/format/layer/published-boundary/port/API consumer 全绿，仍为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。真实 HTTP 32 场景验证 project/user add/update/delete 均发布 `agentMemory.changed`，duplicate add 后订阅队首确定为随后提交的 `sessions.changed`，证明无时间猜测地排除虚假 memory event |
| 2026-08-13 | P59（invalidation single bridge / implicit codebase reconcile） | Skill archive/restore/proposal 的提交后通知从独立 `struct{}` Relay 收回 Application `invalidation.Skills`，Bootstrap 不再理解 Skill 事件，Delivery 只把统一 notice 投影为既有 `skills.changed`。`codebase.search` 的隐式 reconcile 由 Indexer 在状态进入 indexing 后精确通知 Coordinator，成功/失败 settle 再通知；显式 rebuild 的 public indexing 仍由 Coordinator operation identity 唯一拥有，避免等待底层锁或制造第二 operation。Protocol/Artifact/SQLite/Agent Framework shape 均未变化 | Runtime 定向普通/race、全包禁缓存测试、build/vet/generate/staticcheck/tidy/lint 全绿；Desktop 275 files / 1671 tests 及完整 type/lint/format/knip/架构/API consumer/design/i18n/bootstrap/bundle 门禁全绿，86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。真实第二客户端首次 search 时，已挂载代码库面板无 reload 精确经历 none→indexing→ready（4000 files / 15497 chunks），独立订阅收到连续两条 `codebase.changed`；真实 Run 经 `search_tools` 加载 `propose_skill` 后提交 proposal，已挂载面板 0→1，第二客户端 reject 后 1→0，独立订阅收到连续两条 `skills.changed` |
| 2026-08-13 | P58（external configuration / background invalidation closure） | Runtime Application 中性失效闭集扩展到 models、approvals、agent memory、codebase：provider/role、default/rules、review mutations 仅在成功提交后通知；codebase 在 operation 可读但 worker 尚未运行时发布 start，settle 后发布 finish。Delivery 独占四个新 wire topic，Protocol/生成合同/Desktop binding 原子提升到 `2026-08-13`；Desktop Workspace events Adapter 将 topic 映射到各 context 公开 query identity，Agent Application/Domain 与 Go Agent Framework 不感知 Runtime 事件 | Runtime 全包与生成内容稳定性全绿；Frontend 275 files / 1671 tests 及全部 type/lint/format/knip/架构/API/design/i18n/bootstrap/bundle 门禁全绿，consumer gate 为 86/86 operations + 3/3 sidecars + 16/16 events / 106 typed call sites。新 Runtime 原位重启后页面无 reload 恢复；真实第二客户端令 approval cache balanced→safe、models 四个挂载 read model 同步刷新、agent-memory active query 1→2；codebase 精确观察 none→indexing(operationId)→ready，最终缓存与权威 API 一致，page error/业务 console error 为零 |
| 2026-08-13 | P57（Desktop Runtime cold start / disconnect verification / single stream ownership） | Desktop Runtime context 将原一次性 discovery 与 sidecar 状态合并为唯一 connection supervisor：同代 inspection 并发读取 info/liveness/readiness/discovery，校验 endpoint/server/version/protocol identity 后原子发布 service/capabilities，失败执行有界退避，健康执行周期巡检。Workspace event stream 只通过 Runtime 公共 service port 上报连接丢失证据；capability withdrawal/restore 分别停止和新建 stream generation。Session target 输入区分 identity 与 projection，同 Session projection catch-up 不再先清空相同 watch，revision 只关注 active Session cwd。HTTP/RPC wire、连接策略未进入 Workspace Application、Desktop Agent 或 Go Agent Framework | Frontend 272 files / 1655 tests 与 typecheck/lint/format/knip/circular/context/published-boundary/layer/port/API consumer/design/i18n/bootstrap/bundle 全绿；consumer gate 为 86/86 operations + 3/3 sidecars + 12/12 events / 106 typed call sites。fresh browser 在后端离线启动后按 1/2/4/8/16 秒退避，无 reload 恢复；恢复、30 秒健康巡检、在线 crash、再次恢复各阶段 page error 为零。网络记录证明两个恢复 generation 各只有一条 `runtime.subscribe`，健康巡检只追加 discovery；stream 故障触发即时静默复检后 console/page error 为零 |
| 2026-08-13 | P56（HTTP sidecar contract / Desktop consumption / topic negotiation） | Runtime HTTP Delivery 以唯一 endpoint registry 同源生成 RPC、info、liveness、readiness 的 method/path/auth/status/response contract，并由该 registry 注册 handler、豁免 auth、生成 info 自描述；contractgen 将三条 sidecar API 与响应 Schema 投影到 manifest/TS/validator/reference，Desktop SDK 删除手写路径和响应 Schema。Runtime context 新增中性 service inspection Application/port，由 HTTP Adapter 消费三条 sidecar，Settings 只消费公开状态；10 秒 deadline、并发 coalescing、dispose late-settlement 与 retry 均由 lifecycle owner 管理。真实旧 Runtime 进一步暴露 workspace subscription 把客户端全量主题误当后端能力，现由 Runtime capability port 提供中性 topic membership，Workspace Adapter 只请求客户端可折叠集合与 discovery 声明集合的交集。HTTP/wire/UI/i18n 均未进入 Runtime Application、Desktop Agent 或 Go Agent Framework | Runtime standalone build/vet/test/tidy/lint/race 全绿；Frontend 定向 controller/inspector/store/UI/SDK/consumer/旧 Runtime topic negotiation 回归通过，静态 consumer gate 覆盖 86/86 operations + 3/3 HTTP sidecars + 12/12 events / 106 typed call sites。真实浏览器确认三条 sidecar 均 200、停机刷新进入 unavailable、同页重启恢复 ready；旧 Runtime discovery 仅声明 9 topic 时，重载后的 `runtime.subscribe` 精确发送这 9 个并保持 SSE 200，console/page error 为零 |
| 2026-08-13 | P55（Git process environment ownership） | 新增中立 `infra/gitprocess` 作为 Runtime 内唯一 Git OS-process boundary：清除父进程全部 `GIT_*` 控制面，再按 checkpoint shadow repo 等调用者安装显式 override；checkpoint source discovery、workspace VCS read 与 Git watcher 共同消费。Watcher 的 per-worktree/common metadata path 同时经 `pathidentity` 收敛为 physical identity。Application、Delivery、Agent/Framework、Protocol、SQLite/Artifact shape 均未变化 | 三条红测先证明 ambient foreign index 令 checkpoint 失败、foreign repository routing 令 VCS read 判错且 watcher 订阅外部 `.git`；修复后各自转绿，相关五包完整测试与四包 race、Runtime standalone tidy/build/vet/test 全绿。污染父进程中的真实 HTTP VCS read 返回目标仓库 modified +1/-1；真实 HTTP→TypeScript SDK 流式 Run 与手工保留 Run 均 completed，maintenance checkpoint 成功，shadow tag/tree/fsck 完整，foreign index SHA-256 前后不变；architecture guard 阻止 owner 外直接 Git subprocess |
| 2026-08-13 | P54（checkpoint source ownership / shadow ignore） | Checkpoint Infra 明确 `ls-files --exclude-standard` 是唯一路径 ownership decision：source index 已跟踪路径保留，ignored untracked 路径排除，普通文件再经类型/2 MiB 策略筛选；精确候选列表用 force-add 更新 shadow index，避免 `build/` backstop exclude 二次否决真实仓库合法跟踪的 build 输入。修复止于 `infra/checkpoint`，Run/Goal/Agent 不吞 maintenance 错误，Runtime/Protocol/SQLite/Artifact shape 不变 | 红测真实复现 tracked `app/build/config.yml` 因 shadow ignore 导致 `git add` 失败；修复后 tracked+ignored sibling 矩阵 20×、checkpoint 完整包/race、Runtime standalone tidy/build/vet/test 全绿，真实 HTTP Plan/HITL/Approval/Goal 7 场景通过。修复后的 Runtime 对真实 lynx 工作树完成 Goal checkpoint：shadow tree 精确含 7 个 tracked Desktop build 输入、排除 bin/ios/linux/windows generated paths，`git fsck` 通过且 maintenance 无 ERROR，Goal 到达 1/1 budget boundary |
| 2026-08-13 | P53（Goal mutation/read-model single authority） | Desktop Goal Application 删除 mutation snapshot 的 timestamp 排序与 standing cache 写入，成功、失败和结算不明统一回读；command port 收窄为中性 Session receipt，完整 Runtime Goal 只在 Adapter 消费，`goals.get` data provider 成为长期状态唯一作者。跨 Session receipt fail closed，Application 错误保持结构化且无产品文案。Agent/Framework、Runtime/Protocol、SQLite/Artifact shape 零变更 | 红测覆盖 equal timestamp、延迟未来时间戳、mounted query refetch、跨 Session receipt、失败恢复和同 Session 串行；Frontend 270 文件/1630 测试、95 public edges、86/86 operations + 12/12 events / 103 typed call sites、本地化与 bundle 全绿，Runtime standalone tidy/build/vet/test 全绿。隔离 Runtime/假 provider/延迟代理/真实浏览器中，本地 Stop response 被改为 `2099` 并延迟，远端 Resume 先提交；页面经 `goals.changed → goals.get` 保持 active 1/3，释放旧响应及随后权威回读均不倒退，page/业务 console error 为零 |
| 2026-08-13 | P52（external authored configuration convergence） | 中性 `infra/fileobservation` 以精确路径语义指纹观察 missing-parent create、write、atomic rename、remove 与域内 symlink physical target；Workspace Adapter 独占 `LYRA.md` / hooks cascade 布局，Application 只解析 workspace/projectRoot 与语义资源，Delivery 只投影既有 topic。Knowledge API commit 在发布前按 exact returned path 接受新基线，避免 filesystem 回声导致重复 refetch，同时不吞同资源另一文档的并发外部编辑。Agent/Framework、Protocol、SQLite、Artifact shape 均未变化 | 红测先证明外部配置 3 秒无事件；Infra/Application/Adapter/Delivery/Bootstrap/architecture 聚焦全绿，精确 identity 去重与 close-join 回归通过；真实 Go Runtime → HTTP SSE → TypeScript SDK 验证 home/projectRoot/cwd Knowledge 与 global/project/cwd Hooks 外部变更逐项发专用事件且冷读收敛；Desktop 唯一 consumer 仍消费 86/86 operations + 12/12 events |
| 2026-08-13 | P51（Knowledge physical identity / cross-process CAS / crash recovery） | Knowledge Infra 以唯一 physical identity 将 containment、read/revision/atomic replacement 对齐；域外 symlink fail closed，域内 alias 保持，目标 mode 继承。中性 advisory-lock 同时承载 Bootstrap 单实例 lease 与 Knowledge 跨进程 document lease，但错误与生命周期仍各归 owner；strict staging 在 cold read 回收。Application/Delivery 只投影既有 path policy/problem，Agent/Framework、SQLite、Artifact 不变 | 12×12 独立进程 CAS 恰好一个 winner；12 轮 64 MiB staging 强杀后旧 content 完整、orphan cold recovery、后续写成功；Runtime standalone、Frontend 270 文件/1628 测试及 86/86 operations + 12/12 events、focused/race/Windows 三包交叉编译全绿；真实 HTTP/浏览器验证三 API 外链拒绝及域内 symlink、0600 权限、physical target 写回，page/业务 console error 为零 |
| 2026-08-12 | P50（Knowledge CAS / dedicated invalidations / Desktop conflict recovery） | Runtime Knowledge 由无条件覆盖改为 opaque content revision 条件更新，空文档保持可寻址，成功写返回权威 Entry 并发 `knowledge.changed`；Hook trust 成功提交发 `hooks.changed`。Desktop Adapter 独占 wire/错误映射，Workspace editor 保留脏草稿并在冲突后显式 rebase，Hook mutation 按 project 串行且 UI latch 阻止重复意图。Agent/Framework、SQLite、Artifact shape 均未变化 | Runtime standalone 全绿；Frontend 270 文件/1627 测试、95 public edges、981×8 locales、bundle、86/86 operations + 12/12 events / 103 typed call sites全绿。真实 HTTP 验证首次空文档创建、三 scope CAS、stale revision 拒绝、清空可寻址和两类 committed invalidation；真实浏览器验证 clean refresh、dirty conflict/rebase/second-save 与 Hook 同步双击 latch，page error/业务 console error 为零 |
| 2026-08-12 | P49（mutation authoritative facts / concurrency / context ownership） | Desktop Provider、Approval、MCP、Agent Memory、Codebase mutation 均由所属 Adapter 将 Runtime response 投影为中性资源，所属 Application 在线性化队列中先提交权威 fact 再失效重验；MCP data provider/wire projection 从 defaults 收回 MCP context，Provider `requiresBaseUrl` 进入产品模型。真实联调发现 Provider 保存后草稿没有采用规范化返回值，现以 saved resource 重建草稿。Agent context 未导入 MCP/Provider/Workspace wire 或 Runtime DTO，Runtime/Protocol/SQLite/Framework 未变化 | 红测覆盖非 void 结果丢弃、队列失败恢复/冲突域、返回事实写回、Memory update→delete、MCP toggle/delete、Provider endpoint 必填/trim/UI authoritative reset；真实浏览器覆盖 Approval 快速连点、MCP 全生命周期、Provider、Memory、Plan、HITL approve/reject 与三轮 Goal。Frontend 完整门禁 269 文件/1621 测试、95 public edges、86/86 operations + 10/10 events / 103 typed call sites、981×8 locales 和 bundle 全绿；Runtime standalone tidy/build/vet/test 全绿 |
| 2026-08-12 | P48（API response / identity lifecycle consumption） | Desktop Workspace subscription 的 generation signal 从 Application consumer port 经 Workspace Adapter、SDK 到 `sessions.get`，retarget/dispose 真实取消旧身份读取。Schedule Adapter/Application 保留 `runNow` 的 Session/Run identity 并经 Agent 公开会话动作进入既有 durable recovery；enablement 先提交后端返回的新 revision 再失效重验。wire/transport 只停留在 Adapter/SDK，Agent 内层与 Runtime/Protocol/SQLite shape 均未变化 | Application/Adapter/SDK 三层红测和 Schedule 返回事实红测转绿；隔离 Runtime/真实浏览器证明 `runNow` 自动打开目标 Session、接管流并得到 `P48_SCHEDULE_DONE.`，连续关闭/开启后 SQLite revision 单调到 4、唯一 Run terminal completed，page error 为零。Frontend 完整门禁 257 文件/1591 测试及全部结构/消费/bundle 检查通过；Runtime standalone tidy/build/vet/test 全绿；API consumer 为 86/86 operations、10/10 events、103 typed call sites |
| 2026-08-12 | P47（Workspace opening cancellation / resync fail-closed） | Desktop Workspace subscription lifecycle 的 AbortSignal 现在贯穿 Adapter 与 SDK 的 `workspaces.resolve`，retarget/dispose 会取消仍在 watch-root resolution 的旧 opening 并立即建立新 target。Runtime Delivery 对 `watchIds` 与非 `files.changed` topics 的矛盾 resync 不再静默删除 scope，而是扩大为完整 subscription resync。watch/protocol 事实没有进入 Agent 或 Runtime Domain/Application | 三个定向 Frontend 文件 30 个测试、Workspace events 7 文件/36 测试通过；Runtime resync 目标测试 normal 50 次、race 20 次通过。隔离数据目录真实联调覆盖 HITL approve/reject、Plan、Goal、`sessions.changed`、Git staged 文件事件及 Runtime 重启后无刷新恢复。Frontend 完整门禁 256 文件/1585 测试及全部结构/消费/bundle 检查通过；Runtime standalone tidy/build/vet/test 全绿；API consumer 为 86/86 operations、10/10 events、103 typed call sites |
| 2026-08-12 | P46（Agent Runtime DTO boundary / cold model restore） | Desktop SDK 以中性 Agent facts 承载 live event、durable Run/Item/Interrupt 与 cancel projection，所有 wire 校验和映射集中在 Agent Adapter；pending work provider 回归 Agent bootstrap，defaults/public surface 不再拥有 Agent 装配。发布边界门禁阻止 Agent Application/Domain/public 与 SDK event contract 导入 RPC。Composer 以显式偏好、durable Session 模型、catalog fallback 的顺序解析，并在 active Session summary 未决时等待，消除冷启动默认模型竞态。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | Mapper/provider 13 个定向测试与 Composer model selection 4 个测试通过；隔离 Runtime/真实浏览器完成两步 Plan、冷重载及第二次 Run，两次 usage 均为 `deepseek-v4-pro`。完整 Frontend 门禁 256 个文件/1582 个测试、typecheck/lint/format/knip/循环/Context/发布边界/层级/port/API consumer/style/design/token/chrome/locales/bootstrap/bundle 全通过；281 个视觉/交互/WCAG/Retina/WebKit 场景通过；Runtime `go test ./...` 全绿；API consumer 为 86/86 operations、10/10 events、103 typed callsite |
| 2026-08-12 | P45（Goal/Plan lifecycle / boundary convergence） | Desktop Goal launcher 让 paused/blocked replacement 能力可达，预算耗尽不再暴露无效 Resume，状态和 async completion 按 Session identity 隔离。Goal Application 按 Session 串行、单调提交 typed response，Adapter 独占有界 unary settlement、wire mapping 与 provider。Plan wire snapshot/event 在 Agent Adapter 转为中性 domain，fold composition 移入 bootstrap；`goals.changed` 精确失效目标 Session。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | Goal/RPC 最终聚焦 3 文件/19 测试与 Plan 6 文件/41 测试通过。隔离 Runtime/假 provider/真实浏览器验证 Plan `1/2` live projection、Goal 1/1 `runBudgetReached`、同 Session blocked replacement 与冷重载；SQLite 精确为 replacement Goal revision 4、1/1 usage 和两步 Plan revision 1，console 仅开发提示、page error 为零。完整 Frontend 门禁 253 个文件/1565 个测试、typecheck/lint/format/knip/循环/Context/发布边界/层级/port/API consumer/style/design/token/chrome/locales/bootstrap/bundle 全通过；API consumer 为 86/86 operations、10/10 events、103 typed callsite |
| 2026-08-12 | P44（Session rewrite / mutation settlement） | Desktop rollback 进入 mounted Session 唯一同步 owner并可等待权威 commit，按 Session single-flight；Runtime Adapter 消费 dropped user input，Application/Chat 只见 AgentInput。无 checkpoint 的 files/both fail closed。rename/favorite/relocate 共享 Session 级 revision settlement，失败恢复缩小到条件字段。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | 红测覆盖同步等待/重试/dispose、重复 rollback、权威 dropped input、first-turn files/both、rename+favorite revision chain/局部回滚后转绿；7 个测试文件/40 个定向测试通过。隔离 Runtime/真实浏览器验证 files-only 不产生 rollback record且保留 1 Run/2 Items/2 Messages；两轮 edit-and-rerun 后精确回到 1 Run/2 Items/2 Messages并权威预填，reload 稳定；fork 后精确 2 Session/2 Run/4 Items/4 Messages，console/page error 为零。完整 Frontend 门禁 251 个文件/1543 个测试、typecheck/lint/format/knip/循环/Context/发布边界/层级/port/API consumer/style/design/token/chrome/locales/bootstrap/bundle 全通过；API consumer 仍为 86/86 operations、10/10 events、103 个 typed callsite |
| 2026-08-12 | P43（Run opening deadline / cancel remote-winner settlement） | RPC Agent Adapter 把 opening handshake deadline 与 accepted stream lifetime 分离：首 timeout 对原 MutationPromise 以 fresh signal 重试，第二 timeout 有限返回；winning signal 在 ack 后继续受 session owner 控制。Cancel controller 以 material revision 条件提交响应，失败后经 Application 权威投影判断 terminal，transport 只在 Adapter 投影。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | 红测覆盖 same-key/fresh-signal、parent abort、双 timeout、stale cancel snapshot、remote terminal、revalidation 双失败和 transport problem 后转绿；7 个测试文件/54 个测试、typecheck/lint 通过。隔离 Runtime 中代理在完整 6,514-byte Run stream 已生成后悬挂首响应，30 秒后第二 attempt 同 key，页面取得 completed tail；SQLite 1 Session/1 Run/2 Messages，reload 无重复。第二场代理延迟本地 cancel 30 秒，直连客户端先取消，页面先显示 canceled，迟到拒绝后无 banner/console/page error；Frontend 完整门禁通过（250 个测试文件/1533 个用例） |
| 2026-08-12 | P42（command replay / draft cold-start ownership） | Desktop RPC SDK 从生成 command policy 统一驱动稳定-key 有界 settlement recovery，分别处理 unknown transport、typed in-progress、definitive refusal 与 cancellation；Run opening attempt 各自拥有并清理 event stream。Session create deadline 留在 Adapter，Application 只消费结果；draft provisional owner 持久化，fresh-create proof 保持 ephemeral，durable recovery 是毕业 owner。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | 红测证明产品调用点从不消费既有 `retry()`、commit 后断帧直接失败、in-progress 不等待、opening stream 重放和 cold draft 身份错误后转绿；RPC 19 files/362 tests、40 command policy/API consumer gate、Agent targeted tests 通过；隔离 Runtime 中代理让首个 `sessions.create` commit 后返回坏 body，第二 attempt 使用同 key，SQLite 仅 1 Session/1 replay record；页面打开同一 URL，reload 后 Work Index 保持 0，首次消息后变 1 且 Session 总数仍为 1，console/page error 为零；Frontend 完整门禁在本批提交前收口 |
| 2026-08-12 | P41（HITL remote-resume / Goal settlement convergence） | Desktop Application 让 submitting HITL batch 随 standing projection 幂等退休并向 Adapter 返回中性 supersession 事实；Run opening Adapter 不识别 operation/wire error，只禁止迟到 rejection 反转权威结果；Goal start/stop/resume 在成功或结算不明后统一回读，回读失败保留原命令错误。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | 红测稳定复现 submitting batch 永久 staged、remote winner 后迟到 rejection 进入 command error、Goal 失败不回读及双失败错误替换后转绿；隔离 Runtime 经透明代理延迟浏览器 `runs.resume`，第二客户端先完成同一 approval，页面终态 `P41_HITL_REMOTE_DONE`、无卡片/command banner，console/page error 为零；Frontend 完整门禁在本批提交前收口 |
| 2026-08-12 | P40（deep-link Session lifecycle convergence） | Desktop mounted driver 在 material view 前建立 active Session 的 held-open/last-session memory；纯 selection model 以 Runtime live/draft identity 对账并永不清理存活 active，close/reconcile 同步修正 cold-start seed；relocate 失败回到 Session query owner 重读。Runtime、Protocol、SQLite 与 Agent Framework 零变更 | 红测分别复现 stale open cleanup 清空存活 active、close/reconcile 遗留失效 seed 和 relocate ambiguous failure 不回读后转绿；隔离 Runtime 中预置 stale open id 并直达真实 Session，权威对账后 localStorage 只保留 active identity，旧历史完整且页面成功提交第二个 completed Run，URL 稳定、console/page error 为零；Frontend 完整门禁在本批提交前收口 |
| 2026-08-12 | P39（multi-client Session deletion convergence） | Desktop 把启动记忆恢复与持续权威列表对账分离；Session delete 不再提交前乐观删除身份，already-absent 在 Adapter 层投影为目标达成；rename/favorite 失败回滚后重读权威列表。导航、query cache、wire error 各归 Agent State Port、Application query owner 和 Runtime Gateway Adapter | 红测稳定复现 one-shot reconciliation、pre-commit identity mutation 和并发删除复活后转绿；隔离 Runtime 下浏览器打开目标 Session，第二客户端删除后 URL 从 `?session=...` 收敛到 `/`、列表和 Agent view 同步清除，console/page error 为零；Frontend 完整门禁在本批提交前收口 |
| 2026-08-12 | P38（stale Session recovery semantics） | Desktop Runtime Gateway adapter 将权威 `session_not_found` 翻译为 application port 的 absent snapshot，projection refresh 与 history action 不再识别 wire error；其他 protocol/transport 失败继续进入原 recovery error owner。改动没有进入 Runtime 或 Agent Framework，也没有把 Runtime DTO 泄露到 Agent application | Adapter/application 红测先稳定复现 reject/null dereference 后转绿；Frontend 完整 check 和隔离 Runtime 下 stale URL 浏览器复验在本批提交前收口 |
| 2026-08-12 | P37（Goal terminal settlement projection） | Goal Domain `complete` 与 Application owning-drive settlement 保持原 owner；Delivery 将可观察窗口投影为公共 `completing`，生成合同与 Desktop 自有 Goal read model 原子同步。Banner 保留占位并禁止 lifecycle command，最终清除继续由 `goals.changed` 驱动回读。没有修改 Agent Framework、Artifact 或 SQLite shape，也没有把 Runtime 类型下沉到 Agent/Frontend context | Delivery 红测先因公共状态缺失失败后转绿；真实 HTTP Runtime 在 terminal outcome 后冻结下一模型边界，证明 `goals.changed → goals.get(completing) → null` 且唯一 Run completed；Frontend Goal/UI/wire 回归、Runtime contract/Delivery 回归、全量质量门禁和真实浏览器复核在本批提交前收口 |
| 2026-08-12 | P36（side-channel workspace watch recovery） | Desktop Workspace event application 对 active Session cwd 的瞬时解析失败执行可取消、有上限退避并在同一 identity 上自主恢复；adapter 只把权威 `session_not_found` 解释为 unavailable，其他 wire/transport failure 保持失败。Session 切换与 plugin dispose 均终止旧 generation，app-wide topics 与 workspace file watch 仍由一个 `runtime.subscribe` consumer 拥有。改动没有进入 Agent context，也没有修改 Runtime、Protocol、Artifact 或 SQLite shape | 红测覆盖 transient failure、同 identity 恢复、30 秒退避上限、旧 identity backoff 取消、dispose 和 typed missing-session；Frontend 239 files/1485 tests、86/86 Runtime operations + 10/10 events / 103 typed call sites，以及全部类型、lint、format、knip、架构、设计系统、本地化、bootstrap 和 production bundle 门禁全绿；隔离 Runtime/真实浏览器先证明普通外部 worktree 写入不伪造 Git-state event，再由 index semantic change 触发 `files.changed`，已打开 Explorer 无刷新出现新文件，console 仅 dev info、page error 为零 |
| 2026-08-12 | P35（Run subscription convergence / observation lifecycle） | Desktop 在 initial recovery 与 replay reattach 的 snapshot→subscribe 竞态中以 application port 重读 durable projection，accepted Run boundary 与 channel-A start/resume failure 分离；Runtime wake-only fan-out 改为 live-observer-owned generation/refcount/disposer，无 observer 时不保留 Session 状态。改动没有引入 Runtime DTO/Store/transaction/Framework 类型到 Agent context，也没有修改 Agent Framework、Protocol 或持久化 shape | 前端精确红测覆盖 terminal/waiting race 与 post-ack failure classification；Runtime 多 waiter、取消、代际、重复 disposer 与无 observer 回归在 race 下重复通过；Frontend/Runtime 全门禁、真实浏览器双客户端与 staged path/boundary 扫描在本批提交前收口 |
| 2026-08-12 | P34（Goal HITL / authoritative Question / Markdown） | Question accepted response 成为 Transcript 不可变事实并与 resume claim/checkpoint 同事务；Artifact v18、Protocol、SQLite epoch 69、Delivery 和 Desktop 只投影该事实。Goal 冻结/继承协商能力，Resume 验证 capability gap；Runtime execution context carrier 留在 Adapter，Agent Framework 未修改。真实 HTTP 反证出的 parked Run durable admission 缺口由 Application 同时观察本地 gate 与权威 Run 并用 lifecycle signal 唤醒解决。Desktop 未知 raw HTML 在 Markdown AST owner 按字面量显示 | Domain/Application/Adapter/Infra/Delivery/contract 定向回归与真实 HTTP Goal ask_user waiting→Goal resume→同 Run answer→terminal accounting 通过；双客户端、取消/reload、事务失败、冷重启、Frontend 全门禁、Runtime standalone/race/lint 与 SQLite 终态不变量在本批提交前收口 |
| 2026-08-12 | P33（Schedule workspace patch / Desktop consumer） | Schedule 更新合同补齐省略保持、合法 ref 设置、`workspaceMode:"default"` 清空三态，并生成互斥约束；Delivery 将 default 投影到既有空 CWD Domain 语义。Desktop 删除非法空 ref，handwritten SDK 改为直接消费生成 `UpdateScheduleRequest` 并公开该类型。没有把 Runtime workspace/transport 类型泄露进 Agent、Domain 或 Application，也没有修改 SQLite shape 或并存 CLI 工作 | 原失败 UI 路径真实复测后标题持久化、cwd 为空、revision 1→2；wire/Delivery/SDK/gateway/HTTP lifecycle 回归、Frontend 238 files/1471 tests、API consumer 86/86 operations + 10/10 events 和 Desktop Go tests 全绿；Runtime 全门禁与 race 在本批提交前收口 |
| 2026-08-12 | P32（Goal objective incarnation / HITL Resume accounting） | Goal provenance 从混合 process-local ownership 的 lease 治本纠正为 objective-lifetime incarnation：fresh Start 才换身份，Pause/Resume/Stop/Reconcile 保留身份；outstanding HITL Run 的 terminal accounting/outcome 因而仍归原目标。driver 在每次 session-startable 等待后重读并结算权威状态，消除预算耗尽或已完成后额外启动 Run 的窗口。SQLite epoch 68、checkpoint policy v2 采用唯一 incarnation shape，旧字段/版本 fail closed；没有修改公共 Protocol 或 Agent Framework | outstanding Run budget/completed-report、fresh-objective fencing、SQLite exact/retired shape 与 checkpoint strict codec 回归通过；Runtime standalone 全门禁与双向抽象边界扫描在本批提交前收口 |
| 2026-08-12 | P31（Goal root boundary / HITL classification） | Goal driver 从 whole-tree Run stream 只采纳 `StartResult.RunID` 的 root segment boundary；root `Waiting` 以权威 Run state 暂停为 `awaitingInput`，无 root boundary 或只有 child waiting frame 均 fail closed 为 `terminalOutcomeMissing`。测试 fake 同步真实 Event envelope。变更止于 Application/goals，没有引入 Delivery、Infra、SQLite、Frontend 或 Agent Framework 类型 | waiting/missing/foreign-child/malformed 定向普通与 race 各连续 20 轮通过，goals 全包普通与 race 各 20 轮通过；Runtime `GOWORK=off` build/vet/test/race/staticcheck/golangci-lint/tidy 全绿且 lint 0 issue；Agent production import 反向扫描零 `app/runtime` |
| 2026-08-12 | P30（subscription scope / loss recovery） | Runtime Delivery subscription 同时拥有声明的 topic/watch scope，按流过滤普通 watch invalidation 与 resync；拥塞合并新增 broad-file 支配语义，消除跨连接 watch 泄漏和 broad 事实被 watchIds 过度收窄。Frontend 从每条新连接的首帧开始验证 sequence，retarget 后按连接重置。修改止于 Delivery 与 Frontend event application，不把 Runtime/transport/watch 类型下沉到 Agent、Domain 或 Application | workspace hub 定向普通/竞态重复回归覆盖 topic/watch 交集、foreign drop、stable order、malformed recovery、broad↔targeted 两种顺序；Runtime `GOWORK=off` build/vet/test/race/staticcheck/golangci-lint/tidy 全绿且 lint 0 issue；Frontend 224 files/1381 tests及 type/lint/format/knip/全部架构/生产 bundle 门禁全绿 |
| 2026-08-12 | P29（CLI standalone consumer closure） | CLI 的 Runtime dependency 从旧 P24 pseudo-version 前移到已推送 commit `420f627f131a`，间接 Agent 同步 Baseline 20；standalone graph 删除旧 `models/ollama`、daemon、easyjson 与 ordered-map。没有 local replace、compat shim，也没有把 CLI 正在进行的功能文件纳入本批 | CLI `GOWORK=off` tidy-diff/build/vet/test/race/staticcheck/golangci-lint 全绿且 lint 0 issue；`govulncheck` 可达漏洞 0，`go mod why github.com/ollama/ollama` 确认为 main module 不需要该 module |
| 2026-08-12 | P28（Ollama client/daemon 边界） | 独立 `models/ollama` 不再把完整 daemon repository 当客户端 SDK；provider module 私有 wire 精确拥有原生 chat/embed request、response、NDJSON streaming、状态错误与内存上限，并以 raw response extension 保留未知 provider 字段。公开构造器、Core chat/embedding interface、Runtime 与 Agent 合同均未变化 | `models/ollama` standalone tidy-diff/build/vet/test/race/staticcheck/golangci-lint 全绿且 lint 0 issue，Core conformance/behavior、cancel/early-stop/首坏帧、原生 HTTP contract 与未知字段回归通过；`govulncheck` 从 8 条可达路径降为 0，architecture gate 禁止 daemon module 回流 |
| 2026-08-12 | P27（依赖信任边界收缩） | Frontend lock 前移到 Mermaid 11.16.1、DOMPurify 3.4.13、NanoID 6.0.1/3.3.18；Runtime Infra 将 Ollama chat/embedding 改由既有 OpenAI-compatible protocol 组装，移除只为客户端能力引入的完整 Ollama 服务端 module 及独占间接依赖。变更没有进入 Agent/Application/Domain/Delivery，也没有新增 shim、override 或双路径 | clean `npm ci` 后 audit 0，Frontend 224 files/1380 tests、生产 build/bundle 与真实 Mermaid SVG 全绿；Runtime `govulncheck` 可达漏洞 0，standalone tidy-diff/build/vet/test/race/staticcheck/golangci-lint 全绿。真实审批 reject、等待审批崩溃恢复、Tool 执行中崩溃、Goal/Plan crash-resume、双 Session HITL 隔离全部通过，四个 fuzz target 共约 92.6 万次执行通过；SQLite integrity `ok`、foreign-key/开放 lifecycle/active Goal 均为零 |
| 2026-08-12 | P26（Tool execution timing） | Reducer 以真实 attempt start/finish 产出 optional exact execution duration，Transcript 独占该终态事实，SQLite codec 精确 round-trip，Delivery 只投影；未重启/恢复不可证的 Tool 保持 unknown。Protocol 前移 `2026-08-12`、Artifact v17，Runtime contract 与 Desktop generated consumer 原子同步；Agent Framework 未获得任何 Runtime timing、Store、transaction 或产品 DTO | Runtime `GOWORK=off` tidy-diff/build/vet/test/race/staticcheck/golangci-lint 全绿且 lint 0 issue；Frontend 224 files/1380 tests与完整架构/格式/生产 bundle 门禁全绿。真实 HITL 明确等待后，Tool lifecycle 31.160s、journal/payload execution 2.016s、UI 显示 `2s`、最终唯一 `TOOL_DURATION_FIXED_OK`；两个自治 Goal 实时完成 Plan revision 2/3 与 completed audit，完成后普通 Run 返回 `AFTER_GOAL_ORDINARY_OK`。SQLite integrity `ok`、foreign-key/全部开放 lifecycle 为零 |
| 2026-08-11 | P25-04（canonical dependency closure） | Agent Baseline 20 以 commit `8e667d716b22` 发布，Runtime 直接绑定远端 pseudo-version `v0.0.0-20260811152247-8e667d716b22`；没有 `replace`、Runtime metadata sanitizer、Schema 复制或测试跳过。Framework 仍不认识 Runtime 的 Run、Store、transaction、provider 与 provenance policy | 原先 Baseline 18 下失败的两条 bootstrap 冷恢复/HITL consumer 测试转绿；Runtime `GOWORK=off` tidy-diff/build/vet/test/race/staticcheck/golangci-lint 全绿且 lint 0 issue，证明 P25 不依赖 workspace overlay |
| 2026-08-11 | P25-01～P25-03（second adversarial clear） | 前端建立 live-stream/durable-snapshot 单一串行投影，修正 default workspace、draft cache miss、resolution failure、reconnect backoff/opening retarget，并冻结 Run/Item/Plan late-event 单调性；Transport decode 边界递归拒绝 duplicate/unknown member、空 method、非字符串 id、client response 与 request/response 混合，消除 SDK 有损或歧义解释；HITL/Plan/Goal/取消恢复/幂等/cursor/失败注入/断线/崩溃 case 扩展完成。Agent Framework 没有接收 Runtime Run、Store、transaction 或 provenance policy，Runtime 也未建立 sanitizer/replace | Frontend 224 files/1380 tests及全部架构/格式/生产门禁全绿；Runtime workspace build/vet/test/tidy/lint/race/staticcheck、Agent standalone 同级门禁、transport fuzz 199,589 次及 contract generator 零漂移；真实 RPC valid=200、notification=204，九类 duplicate/unknown/null/numeric/client-response/mixed envelope=400。真实 `SIGKILL` 精确捕获 running Run + started model invocation，重启后收口为 lost + unknown，随后同页返回 `POST_CRASH_RECOVERY_OK`；真实 ask_user waiting/resume 返回 `HITL_RESUME_OK`，Plan 两次替换落为 revision 2，autonomous Goal 分别完成 completed/blocked（blocked reason 与 1-Run budget 持久化）且 blocked 后普通 Run 返回 `BLOCKED_GOAL_RELEASE_OK`。SQLite integrity `ok`、foreign-key/开放 lifecycle 为零；浏览器恢复后的 console/error 清洁。完整 Runtime `GOWORK=off` 唯一剩余失败精确归因为 `go.mod` 仍固定未含通用 RawMessage Schema 修订的 Baseline 18，等待 Baseline 20 canonical publication 后绑定 |
| 2026-08-11 | P24（Runtime/Desktop E2E hardening） | Session activity 与前端冷 hydration 统一消费 durable Run/Item；workspace event 精确失效且 Git watcher 只观察 HEAD/index 语义；Segment activation 与 cancel 由 root owner 仲裁；Boot recovery 将 Run tree、Goal、cleanup 和全部 open invocation journal 纳入同一 Application write-set/SQLite transaction，不再留下 terminal Run 对应的 `started` invocation | 真实 HITL、Plan、Goal、并发 cancel/resume、reload 与 `kill -9` 全链路通过；崩溃 Run/Goal/invocation 终结时间完全一致。Runtime 全量 race/static/lint/deadcode、Agent build/vet/race/static/lint、Desktop build/vet/test/static、Frontend 222 files/1361 tests及全部架构/格式/生产构建门禁通过；隔离 SQLite integrity `ok`、foreign-key/开放 lifecycle 均为零，前端空闲 5 秒 `/v2/rpc` 为零 |
| 2026-08-11 | P23（WorkingContext behavior ownership） | source kind 唯一派生并校验 purpose；Memory/Agent document fragment 原子拥有预算后 text+sources；hook result 自己拒绝、注入并附 provenance；`WorkingContextComposer` 统一拥有 system/Plan/recall/hook 流程。旧自由函数和测试专用生产 wrapper 已删除，没有 Java 式继承或 service 层级 | Runtime workspace build/vet/test/tidy/lint/race 全绿，lint 0 issue；`GOWORK=off agentexec`、全模块 staticcheck/deadcode、generator drift、目标重复代码与定向 provenance/WorkingContext race 均通过。完整 `GOWORK=off` 复验准确暴露 `go.mod` 仍固定 Baseline 18、尚未含已完成的通用 RawMessage/byte Schema 修订；这不是 P23 回归，不在 Runtime 建兼容旁路 |
| 2026-08-11 | P22（WorkingContext typed provenance） | `agentexec` 将 base/Knowledge/pinned+recalled Memory/AGENTS.md/Plan/hook 的实际可见来源投影为 versioned JSON-safe metadata，并区分 instruction/data；预算未选中的来源不会伪装成模型可见。归因伴随 opaque checkpoint，不进入 Application/Protocol/SQLite/Framework | prompt text 回归、来源顺序/reference/purpose、budget selection、hook Part ordering、recalled Memory 与 Message validation 定向测试通过；Runtime 全量质量门禁见本批最终验收 |
| 2026-08-11 | P21-07（全场景最终验收） | 真实 DeepSeek 分别完成普通对话、Tool 授权 allow/deny、session/project/global remember 作用域、问题 HITL 取消与 crash/restart resume、长运行 Tool 根取消；SQLite 对 Run/Segment/Item/model/tool invocation/rule/interrupt/checkpoint 逐项对账。mock/PTY 永久回归覆盖审批五种选择、问题 choice/text/cancel、Shift+Enter、滚动、Tool 详情、断线/重复 replay 与冷恢复。压力运行捕获到一次测试把 oolong 最近 diff frame 当完整屏幕的错误 oracle，现改为完成后请求以答案为条件的 full repaint，再验证唯一性，没有增加 sleep 或修改生产行为 | Runtime normal/race 全量、Agent/CLI normal 全量通过；授权 Domain/Application/SQLite、Runtime HITL/restart、CLI terminal/adapter 均 `-race -count=20`，断线/冷恢复唯一答案 `-race -count=50`；三个 strict codec fuzz 各 10 秒共约 59.6 万次通过；真实 SQLite integrity `ok`、foreign-key 零违规 |
| 2026-08-11 | P21-04～P21-06（取消、计时与产品语义） | Runtime adapter 为每个 Agent Effect 绑定 product cancellation context：root 取消覆盖全树，running child 取消按 managed relation 覆盖目标与后代且保留 root/sibling；取消后的 model/Tool 均形成确定 settlement，不进入 Unknown。长寿命 Process 的恢复 Segment 以 continuation 激活点单独计时。Tool 取消成为一等 failure kind，并完整前移 Protocol/Artifact/SQLite 合同 | root model/Tool 与 child model 协作取消定向 race 各连续通过；subtree scope/late descendant 回归证明无 sibling 误伤；真实 `sleep 30` 在 Ctrl+C 后及时退出，Run/Tool 均 canceled，审批等待不计入 active duration；真实 HITL wall 17.412s、active 2.414s |
| 2026-08-11 | P21-01～P21-03（identity、waiting、授权/HITL） | canonical Tool identity、provider SourceCallID 和 execution attempt 分责；Run reducer 在 waiting/terminal/recovery 原子关闭正确上下文，checkpoint 以 schema v1 冻结 capabilities，启动恢复在 Goal 注入后执行。真实 DeepSeek 授权矩阵与问题 HITL 验证 consumer→Protocol→Application→Agent tree 全链路 | 重复 started、连续/并发 Tool、乱序 completion、unknown/recovery、waiting cancel/resume/restore、approval hook-once 与 SQLite write-set 回归全绿；真实 allow once、deny、session/project/global remember 及作用域边界通过，question cancel 与 crash/restart resume 通过，终态 checkpoint/interrupt 均为零 |
| 2026-08-11 | P20-04（真实 embedded 与最终门禁） | 以当前 workspace 源码直接构建、无 overlay 的 CLI，分别完成 DeepSeek JSON one-shot 与交互 TUI；两次运行均得到指定文本并正常退出。窄终端 approval race 回归暴露的是测试依赖默认长对话首句可见的脆弱 oracle：拒绝已经生效但首句被正常滚出视口；测试现使用专用短交互脚本，以明确 deny 结果验证响应式审批，不延长 sleep 或修改正确生产行为 | Runtime 与 CLI 各自的 build/vet/test/tidy/lint/race 六项 CI 等价门禁全绿，lint 0 issues；approval 定向 race 连续 20 次通过。隔离 SQLite integrity `ok`、foreign-key 零违规；2 个 Run 与 2 个 model invocation 全部 completed，user/reasoning/agent 各 2 条且无重复 Item、problem、未完成调用、pending interrupt、open Tool invocation 或 checkpoint |
| 2026-08-11 | P20-03（Framework JSON wire schema correction） | 获得明确跨模块授权后，修复留在 Agent Framework 的唯一 `SchemaFor` owner：`json.RawMessage` 接受任意合法 JSON value，`[]byte` 接受 null/base64 string，均服从 `encoding/json`。生产实现不 import Runtime/Core consumer、不识别 DeepSeek、不删除 metadata，也未把 Run、Store、transaction、投影或 provider 策略带入 Agent | Agent 通用 Schema 回归覆盖 Raw JSON 六类值、byte slice 和无效输入；外部 Interaction 回归覆盖 object response extension、string reasoning metadata 与 signature。Agent standalone build/vet/tidy-diff/staticcheck/lint/test/race、public/private contract 和 package DAG 全绿；真实 DeepSeek 不再在成功模型响应后因 Schema 失败 |
| 2026-08-11 | P20-01（TUI、SQLite 与 embedded diagnostics attribution） | 隔离 mock PTY 逐项证明 Shift+Enter 多行输入、运行中 PageUp 暂停 bottom-follow、Ctrl+O 展开 Tool 详情和审批交互；真实 embedded SQLite 的 integrity/foreign-key、Run/Item/model/tool invocation/interrupt 关联反证 Tool continuation 并未串写。日志所有权同时厘清：HTTP dev 的 `lyra.log` 是 Makefile 重定向，`embedded` library 正确地不安装进程级 OTel globals，而当前 CLI host 尚未安装 provider，不能把旧服务日志当成当前 TUI 证据 | PTY 行为矩阵通过；真实数据库 integrity `ok`、foreign-key 零违规；`LYRA_LOG_LEVEL=debug` 的 embedded `sessions ls --json` exit 0、stdout 单行合法 JSON、stderr 0 字节且隔离目录只含数据库与锁文件 |
| 2026-08-11 | P20-02（stream/final authoritative reconciliation） | `application/runs` 不再把 reasoning/text 当作互斥流模式：两种 Delta 各自保持同一开放 Item，直到 `ModelCallCompleted` 以最终完整消息完成原 identity；最终内容可覆盖部分观察，`AssistantMessageCompleted` 仍只作相同结果确认。删除类型切换时的提前完成后，真实 DeepSeek 路径中的 reasoning 不再先落一次、再由最终响应重复写入 | 新回归同时固定“切换不产生 ItemCompleted”“最终沿用两个已开始 identity”“最终权威内容替换部分文本”，连续 100 次通过；`application/runs` 与 `agentexec` targeted 重复测试、Runtime 全量测试、两 owner race、CLI `-p 1` 全量 consumer 回归均通过 |
| 2026-08-11 | P19-06（public contract freeze and final audit） | contract generator 从 `protocol + embedded` 的真实 Go type information 派生完整 `go-api.json`，冻结 public package/import/constant/variable/function/type/field/method；架构门禁拒绝第三个公共 package、模块根 package、public signature 的 internal type、operation/method 漂移、generated artifact/digest 和 Session Artifact version 漂移。新增窄 `protocol.ProblemError`，保留 `errors.Is` sentinel 并为 `errors.As` 提供结构化恢复事实；README 与 consumer handoff 给出真实嵌入用法及唯一 owner 约束。embedded 的伞状 `automation`/`integrations` 文件和错位方法按准确 capability 文件彻底收敛，不增加子包；最终 race 发现的并发 Tool 测试缺少“全部跨过外部边界”线性化前提，以显式 barrier 治本，不更改正确的 production stop-unstarted 语义 | generator/drift/digest/public external-module/operation parity/structured-error tests 全绿；Runtime standalone build/vet/test/tidy/lint/race 六项全绿，lint 0 issues；GOWORK=off staticcheck 与 deadcode -test 零输出；3 个 continuation fuzz owner 约 52.9 万次执行全绿；并发 unknown-effect race 目标 50 次及全模块 race 复跑全绿；空目录/空文件、旧 public/internal protocol/inprocess、伞状文件、兼容残留和 public abstraction leak 复扫零新增违规 |
| 2026-08-11 | P19-05（public embedded Runtime） | 新增公共 concrete `embedded.Runtime`，以准确的 `CallOptions`、`CommandOptions`、`RunCommandOptions`、`RunSubscriptionOptions`、`SubscriptionOptions` 暴露全部 operation；方法直接进入 P19-03 的唯一类型化 pipeline，不启动 listener、不经过 JSON-RPC/SSE、也不公开 Host/Store/Engine/Router/context key。`Config` 冻结显式绝对 host path，默认 workspace 只取一次 user-home snapshot，默认 config search 只限 data directory；`Close` 停止新调用并由 instance owner 结束流、join worker、逆序关闭资源及释放独占 lease，失败保持可重试 | AST+reflection 门禁逐项证明每个 catalog operation 恰有一个公开方法且 request/options/result/event 类型精确一致；真实 embedded 集成覆盖 discovery/protocol reject、durable idempotency replay/conflict、Runtime notification、流随 Close 结束、closed rejection、同目录拒绝与 reopen；独立临时 Go module 仅导入 `protocol + embedded` 编译通过；embedded/operation/bootstrap/arch/cmd targeted test/vet 全绿 |
| 2026-08-11 | P19-04（single Runtime instance owner） | `bootstrap.Instance` 成为 HTTP/embedded 共用的唯一完整 Runtime owner：canonical data-directory lease 早于 SQLite/recovery，统一完成 config/client/store/seeding/Assembly/Host/recovery/server/operation 组装并启动 scheduler；operation endpoint 绑定 instance lifetime，Close 先停止 admission 与流、取消并 join worker，再关闭 Host/resources，最后释放 lease，失败可重试且不提前放锁。HTTP 改为接收 instance-owned Endpoint，不再自行构造第二 registry/idempotency pipeline；cmd 不再拥有 recovery、scheduler 或 Server 组装 | 同路径、symlink alias、真实 child process 争锁与 release/reopen tests；Close 未 join 保持 lease、retry 后释放；真实 Open→discover→第二 Open 拒绝→Close→reopen 集成通过；Windows lock path cross-compile、operation/HTTP/cmd/bootstrap/architecture targeted tests 与全 Runtime tests 全绿 |
| 2026-08-11 | P19-03（single typed operation pipeline） | method catalog、类型化 invocation、严格 request/metadata/result/event validation、capability gate、binding-neutral problem projection、durable idempotency claim/complete/replay 与 run-stream reattach 全部收敛到私有 `delivery/operation`；幂等 payload 具有显式 version，未知旧形状 fail closed。HTTP `dispatch` 只保留 JSON-RPC envelope、numeric code 与 frame encoding，Application server 从 operation context 读取 binding-neutral replay cursor；旧 dispatch registry/capability/idempotency owner 和 forwarding catalog 物理删除 | operation/dispatch/server/contractgen targeted tests、全 Runtime tests 与 architecture package-boundary guard 全绿；`go generate ./...` 成功且公开 contract 制品零语义 diff；完整静态、race 与 hygiene 门禁在本批提交前收口 |
| 2026-08-11 | P19-02（single public protocol owner） | binding-neutral DTO、枚举、请求/响应、事件、版本、严格递归验证与稳定 problem identity 原子移动到公共 `protocol`；旧 `internal/delivery/protocol` 物理删除且无 alias/forwarder。服务端能力接口、request-context 传播和 structured problem enrichment 收回私有 `operation`，numeric JSON-RPC code 留在 `dispatch`；reflection field walker、enum/sample catalog 收回私有 `contractshape`/`contractcatalog`，公共面不再暴露 generator machinery | `go generate ./...` 成功且 contract 制品无语义 diff；公共 protocol、私有 catalog/shape、dispatch、contract generator、server、HTTP 与 architecture targeted/full tests 在本批提交前统一收口 |
| 2026-08-11 | P19-01（public binding contract） | 真实 CLI 嵌入消费者推翻“无公共 Go binding 需求”的旧前提；ADR-RT-053 冻结唯一公共 `protocol`、concrete `embedded.Runtime`、binding-neutral `operation`、HTTP/embedded 共享 instance builder、data-directory 独占与完整 lifecycle。旧 internal in-process channel transport 不复活，不建立 JSON-RPC round-trip、胖 public interface 或消费者兼容层 | 六份核心文档与 API/Transport/README owner 同步；本批仅文档，`git diff --check` 与文档链接/术语门禁在提交前验证 |
| 2026-08-10 | P18（Application mechanism ownership） | 完整调用图推翻“外环引用即跨环共同 owner”的旧判断：pagination 的 cursor/query binding/page-width/page-cut 全由 Application read 决定，taskgroup 的任务启动者全是 Application coordinator，Bootstrap 只装配/关闭；opaque-token 的 pagination/Run replay payload owner 也同属 Application。三个 package 原子移动到 `application/{pagination,taskgroup,opaquetoken}`，旧根路径永久禁止；媒体内容 codec 门禁收窄为真实语义，只对精确 continuation framing 允许 URL-safe Base64，未建立宽泛 encoding 例外。package 总数保持 100，无 alias/shim、协议/schema/Framework/消费者变化 | `MODULE=app/runtime FAST=1 scripts/check.sh build vet test tidy lint race` 六项全绿，lint 0 issues；`GOWORK=off staticcheck ./...`、`deadcode -test ./...`、`go generate ./...` 零输出/零漂移；旧 import、旧目录、空目录与 `git diff --check` 扫描全绿 |
| 2026-08-10 | P17-05（final package-boundary acceptance） | 从 115 个 package 的 P17 起点收敛至 100 个真实 package；逐层完成 shared capability、Toolset、Domain/Application、Adapter/Infra/Delivery/Bootstrap/Testsupport 的消费者与变化轴反证。新增全 internal 生产 package 永久门禁：目录名必须等于 package 名、必须有准确 `Package <name>` GoDoc、禁止只含 `doc.go` 的零行为 umbrella；旧目录/文件、生产 import、空目录、单消费者伪共享、不可消费 transport、package-doc 缺失与生成漂移归零。强验收捕获并修复一个已提交 invalidation test 的 gofmt 对齐漂移，随后对整个 Runtime 格式化并重跑 lint | `MODULE=app/runtime FAST=1 scripts/check.sh build vet test tidy lint race` 中 build/vet/test/tidy/race 全绿，首次 lint 精确捕获 1 个旧 gofmt 漂移，修复后独立完整 lint 0 issues；`GOWORK=off staticcheck ./...`、`deadcode -test ./...`、`go generate ./...` 零输出/零漂移；Continuation/Prompt/Resolution 三个 strict codec fuzz 各 10 秒、合计 987,564 次执行通过；package-doc/import DAG/retired-path/TODO/空目录/diff hygiene 扫描全绿。P17 完成 |
| 2026-08-10 | P17-04c（Delivery/Bootstrap/Testsupport counterexample） | 按生产消费者和 Go import 可达性复审 Delivery/Bootstrap/Testsupport。删除 `delivery/transport/inprocess`：它只有自测、无任何生产消费者，并位于 Runtime `internal` 下，外部 CLI/TUI 根本不可导入，因此不是可嵌入能力而是伪公共实现；当前唯一 binding 回归真实的 streamable HTTP，transport/dispatch/Architecture/API/README/standards/decision/ledger 同步且旧路径永久禁止。Delivery 的 protocol/server/dispatch/transport、Bootstrap focused builders 与三个多测试消费者 fixture package 均有独立变化轴，保留；`httporigin` 和 `taskgroup` 经多 ring 消费者反证仍是纯 shared capability，不能下沉 Infra 迫使 Application 反向依赖。删除零消费者、零行为的 `domain/doc.go` 伪 umbrella package，Domain 父目录只作为 bounded-context namespace；旧 shutdown/replaycursor 空目录同步物理清除 | 全量消费者扫描证明 inprocess production imports=0；HTTP/dispatch/architecture/docs 与 Runtime test/vet/build 在本批和 P17-05 收口 |
| 2026-08-10 | P17-04b（Adapter boundary evidence） | 逐个复核 Adapter 的消费者、Application port/SDK boundary 与变化原因；保留 mcpconnection、providerregistry、runrecovery、notification、codebaseindex、sessiontitle、skillproposal、planpresentation、utilitymodel 等虽小但拥有独立防腐或策略边界的 package。删除单消费者且与静态 model catalog 完全同变化轴的 `adapter/pricing`，定价能力收回 `adapter/modelcatalog.Pricing`；为此前缺失 owner 文档的 codebaseindex/hooks/providerregistry/sessiontitle/skillproposal、Application hooks 与 Delivery dispatch 补齐准确 package GoDoc，使内部每个生产 package 的职责能由 `go list` 直接读取。旧 pricing 路径永久禁止，不制造 Adapter umbrella | modelcatalog/Bootstrap、全量 package-doc 扫描与 Runtime test/vet/build 在本批提交前收口 |
| 2026-08-10 | P17-04a（Infra package topology） | 删除没有行为 owner 的 `infra/storage` umbrella：SQLite technical mechanism 直接归 `infra/sqlite`，人类可编辑 LYRA.md 的路径布局、原子替换与 mtime 投影独立归 `infra/knowledgefile.Store`，不再以泛化 `storage.FileKnowledgeStore` 暴露实现。将完全不适配 Application port、只注册进程级 OpenTelemetry globals 的 `adapter/observability` 纠正到 `infra/telemetry`，入口从含混 `Setup` 改为 `Configure` 并以 `Shutdown` 表达返回能力。旧路径由 architecture guard 永久禁止，不保留目录 namespace、alias 或 shim | knowledgefile、sqlite、telemetry、Persistence、Bootstrap、Delivery 与 architecture targeted tests 在本批提交前收口；完整 Runtime test/vet/build 同批验证 |
| 2026-08-10 | P17-03（Domain/Application package boundary） | 复核 P16 已冻结的 21 个 Domain bounded context，确认其虽有小包但分别拥有独立词汇、行为/invariant、变化原因和直接测试，不按体量机械合并。Application 删除含混 `change`，以 `invalidation.Notice` 准确表达“提交后通知读者重新读取”的无值信号，并将 Bootstrap/Delivery/Goals/Plans/Runs/Sessions 的 `Changes`/`Changed`/`WithChangeNotices` 全链路统一为 `Invalidations`/`WithInvalidations`；原泛化 `admission` 按实际 process-local Session/working-tree gate 改为 `sessionadmission`；只有一个黑盒测试消费者的 `approvals/approvaltest` 回收到测试文件。旧三个路径由 architecture guard 永久禁止，不保留 alias/shim | Runtime `GOWORK=off go test ./...` 全绿；Application/Bootstrap/Delivery/agentexec targeted tests 全绿；完整 vet/build 与架构/命名/空残留门禁在本批提交前收口 |
| 2026-08-10 | P17-02（Toolset package convergence） | 将每个 model tool 一个目录的 `toolset/{agentmemorysearch,askuser,conversationsearch,goal,lsp,offload,plan,schedule,shell,skill}` 治本收敛为 `toolset/builtin`：一个 package 拥有 Runtime built-in tool 的共同变化轴，文件仍按能力族分责，所有原含混 `New`/`Build`/`Search`/`Reader`/`Store` API 改为 `NewAskUser`、`BuildPlan`、`AgentMemorySearch`、`GoalReader`、`ToolResultStore` 等准确词汇。deferred discovery 从子包归回 Resolver owner；单消费者 delegation contract 归回 `agentexec` Interaction ACL；稳定 model-facing identity 从误称 `toolset/catalog` 的 cycle-workaround package 改为 25 个真实消费者共享的 `adapter/toolname`。旧 12 个子目录物理删除并由架构 guard 禁止回流，package 总数从 115 降至 104，不保留 alias/shim | Runtime `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 全绿；toolname/toolset/builtin/agentexec/Bootstrap/architecture targeted tests 全绿；Tool descriptor parity、strict schemas、Role manifests、deferred advertisement、HITL restore、Delegate deployment 与全部原工具行为测试保持通过；`git diff --check` 通过 |
| 2026-08-10 | P17-01a（shared capability ownership） | 删除单消费者顶层 `internal/replaycursor`：Run replay 的 position、epoch、scope、format/version 与 validation 全部回归 `application/runs` 私有 journal 行为，不再暴露可拼装的 `Position`；抽取 pagination/replay 两个真实消费者共同使用的 `internal/opaquetoken`，只负责 strict URL-safe framing，unknown/trailing JSON 一律拒绝且不理解任何业务 payload。删除单消费者顶层 `internal/shutdown`，将 non-cooperative close serialization 下沉为 `infra/teardown` 技术机制，Bootstrap 继续只组装/关闭；`httporigin.Origin` 关闭字段，外部无法构造不合法 security origin。shared-capability 架构白名单与 Capability Ledger 同步，不保留 alias/shim | Runtime `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 全绿；runs/pagination/opaque-token/teardown/Bootstrap/HTTP-origin/architecture 直接行为与严格 codec tests 全绿；旧 package import 与旧目录生产引用为零，`git diff --check` 通过 |
| 2026-08-10 | P16-06（final Domain ownership acceptance） | 从已推送的 P16-05 干净 Runtime 基线完成 Domain ownership 专项最终反证；Run、Transcript Item、Plan、Session 四条纵切与 21 个 Domain package 的 package/文件/类型/方法/错误/注释/测试边界共同自洽。Domain 不含 context I/O port、Framework/Storage/Protocol/Workspace live projection 或 Runtime capability availability；Application 继续独占 workspace admission、跨 aggregate write-set、事务、并发、publish、cleanup 与 executor lifecycle。无 compatibility path、重复 mutation owner、透明 alias、forwarding layer、收纳 package、tracked 空文件/目录、生产 TODO/FIXME/HACK、生成漂移或消费者接线项；根 `LYRA.md` 仍是有意保留的空 knowledge 起点 | 仓库标准 `MODULE=app/runtime FAST=1 scripts/check.sh build vet test tidy lint race` 六项全绿；standalone make、staticcheck、`deadcode -test`、完整 golangci-lint 与 generator diff 全绿；Continuation/Prompt/Resolution 三个 strict codec fuzz 各 10 秒、合计 739,740 次执行无失败；architecture/contract/doc facts、文档本地链接、边界/空残留/hygiene 扫描全绿。P16 完成 |
| 2026-08-10 | P16-05（Domain package boundary freeze） | 对全部 21 个 Domain 子包逐一按独立词汇、变化原因、真实消费者与 import DAG 裁决，未按文件数机械合包：aggregate/policy 与精简 value package 均有一句准确 owner。删除 `session.WorkspaceIdentity` 这份 live filesystem projection，统一复用 `application/workspace.Resolved`；Session/Schedule 重复的 `ErrCWDUnavailable` 收敛为 Workspace Application 唯一 sentinel，Runtime scheduling availability 收回 `application/schedules`。`agentmemory/codebaseindex/hooks/mcpserver/provider/toolresult` 的失真或口吃文件名按实际内容改正，package owner 注释与实现同步；Interrupt kind 零值改为无效，codebase ranking 明确拒绝非正 limit，消除误判和 panic。所有 Domain package 现在均有 package boundary GoDoc 和直接测试，architecture guard 永久拒绝目录名/package 名漂移、nested context、缺失职责说明或无直接验证；无空目录、forwarding layer、透明 alias 或新收纳包。Protocol、Artifact、SQLite shape/epoch、Agent Framework 与消费者合同均未改变 | 逐包 package-doc/test guard，Domain 全包、Session/Schedule/Workspace、workspacepath、Tool schedule、Delivery 与 architecture targeted tests 全绿；完整 standalone/race/lint/deadcode/generator/hygiene 门禁在本批提交前收口 |
| 2026-08-10 | P16-04（authoritative Session behavior） | `domain/session.Session` 关闭全部字段并独占 fresh/scheduled construction、完整 edit、generated-title first-writer、fork inheritance、restore workspace installation、target revision advance、time/revision invariant 与 immutable identity/lineage；`Snapshot` 只承担严格 technical rehydrate。Application 解析 workspace、生成 identity/time、持有 mutation admission 和跨聚合 write-set，创建、更新、fork、restore 与 Run opening 均携带 Domain 已决定的完整 aggregate；scheduled opening 在 Run admission transaction 内精确 Insert，既有 Session 的 configured model 通过同一 Domain Apply 形成 CAS replacement。SQLite SessionStore 删除 Create/Ensure/Patch/Fork/Restore/逐字段 setter、clock 与 UUID，只保留 exact Insert/Save CAS/Delete；portable restore 不再 replace row 并重置 revision，异步标题通过 Application capability 重读/重试且并发用户标题必胜。Protocol、Artifact、SQLite column/epoch 与 Agent Framework 边界均未改变，不产生消费者接线项或兼容路径 | Domain construct/apply/no-op/conflict/fork/title/restore/overflow，Application workspace rejection/scheduled initial/reuse/generated-title race，Run opening initial/replacement，SQLite exact round-trip/stale/invalid shape，fork/restore write-set、Session import monotonic revision、Delivery/Bootstrap 与 architecture single-owner guard 全绿；完整 standalone/race/lint/deadcode/generator/hygiene 门禁在本批提交前收口 |
| 2026-08-10 | P16-03（authoritative Plan replacement） | `domain/plan.State` 关闭全部字段并独占 ordered Steps、whole replacement、clear、revision、updated time、step invariant、overflow/time-travel 拒绝与 defensive copy；原误称 Store 的领域文件按 aggregate 语义改为 `plan.go`。新增 `application/plans` consumer-owned Store port 与 `Coordinator`，由 Domain 计算下一 State、Application 形成不可变 `Replacement`、Persistence 只按 expected revision 保存精确状态；CAS 成功后由该用例发布 Plan invalidation，Run reducer 只投影自己的 stream snapshot，不再成为第二 publisher。`set_plan`/`exit_plan_mode` 退回参数翻译、execution scope、Application 调用与 presentation，不再直接验证或写 Store；成功 Tool outcome 从同一 Application state 回读。Session fork/rollback/restore 的跨聚合写集携带已裁决 replacement，SQLite 不再调用时钟或用 SQL 自增 revision；restore 保留 live Plan row 并 CAS 更新，治本修复先删后插把 revision 重置为 1 的错误。Plan Protocol/Artifact/SQLite column shape 均未改变，不产生消费者接线项或兼容路径 | Domain replacement/clear/invalid restore/time/revision overflow/defensive-copy，Application prepared write-set/CAS conflict/session identity/post-commit publication，SQLite exact save/stale conflict/round-trip/boundary，Tool adapter、Run projection、Session fork/rollback/restore revision integration 与 architecture ownership guard 全绿；完整 standalone/race/lint/deadcode/generator/hygiene 门禁在本批提交前收口 |
| 2026-08-10 | P16-02（closed Transcript Item union） | `transcript.Item` 改为私有字段的 closed tagged union，各 message/reasoning/question/compaction 通过语义构造器形成 complete fact；只有 ToolCall 拥有 running→completed/incomplete 行为，terminal settlement first-wins，child cancellation 只能对未分类的 abandoned ToolCall 补充准确因果且不能改写身份、时间或 invocation。Application 的流式 AgentMessage/Reasoning start 收敛为非持久 `ItemStart` 渲染锚点；user input、Question、compaction 不再制造 synthetic started/completed pair，Question 的待答状态只由 Pending interrupt 拥有。Reducer、recovery、waiting subtree cancellation、Session parked terminalization、SQLite strict codec、portable archive 与 Delivery presenter 全部改为构造/行为/accessor/严格 technical snapshot，不再直接修改 Item 字段。SQLite payload 将误称 `problem` 的 Tool failure 改为 `failure` 并一次提升 epoch 66；可观察事件序列与旧 running Question artifact 同时形成明确 breaking boundary，Protocol 提升为 `2026-08-10`、Artifact v15，生成合同与 consumer handoff 同步且不保留 dual codec | Item variant/ownership/ToolCall transition/effective-arguments/technical-boundary tests，Question park/resume/cancel/recovery，child opening/cancellation，offload round-trip，Artifact v15 rejection，contract/version/schema consistency 与全 Runtime ordinary/race tests 全绿；standalone tidy/build/vet/staticcheck、完整 lint、deadcode、generator diff check 和 hygiene 门禁通过 |
| 2026-08-10 | P16-01（authoritative Run aggregate） | `domain/run.Run` 以私有字段成为 Session/Run identity、lineage、model selection、Goal provenance、lifecycle、active Segment、metrics、limits/capabilities、terminal outcome/failure/detail/time/message watermark 的唯一 aggregate；新增显式 admission/restore、metrics advance、suspend/resume/terminate/waiting cancel/lost recovery、snapshot/equality 行为。Run cumulative usage 收敛到 `domain/accounting`，Run failure 与 Tool failure 按 owner 拆为两个 taxonomy；`domain/transcript` 删除第二 Run carrier 和通用 Problem。Application reducer、waiting/recovery/session write-set、Agent ACL、SQLite、portable artifact 与 Delivery projection 全部改为消费唯一 Run value，Run 不再复制 Pending open Interrupt。SQLite admission 和 lifecycle write 先调用/重放 Run aggregate 行为，再以 exact aggregate equality + CAS 持久化，不再由 Store 的裸 `State` 转换成为第二业务 owner；测试 fixture 也必须从合法 aggregate 转换产生，不再拼装半真终态 | 新增 Run lifecycle、terminal fact matrix、illegal transition、metrics regression/overflow、slice/map ownership 测试；application/runs、sessions、agentexec、runsegment、runrecovery、SQLite、Bootstrap、Delivery 与全量 `go test ./...` 全绿；Protocol、SQLite epoch、Agent Framework 合同和消费者代码均未改变，完整静态/race/generator/hygiene 门禁在本批提交前收口 |
| 2026-08-10 | P15-04（final counterexample freeze） | 从干净提交点完成 Agent/Runtime 双向抽象、六环 DAG、consumer-owned port、opaque checkpoint、Framework execution vocabulary、全层级命名、文件/目录、生成物和根仓库爆炸半径最终反证。Agent 仍不含 Runtime 产品/持久化抽象；Runtime Framework concrete import 仍唯一收敛在 `adapter/agentexec`；MCP lifecycle 末批改名未改变协议 wire、SQLite shape、Agent Baseline 或消费者。无新坏味道、兼容路径、重复 owner、死代码、复杂度热点、生成漂移、意外空文件/目录或生产 TODO/FIXME/HACK；空的根 `LYRA.md` 仍是有意保留的 workspace knowledge 起点 | Runtime standalone tidy-diff/build/vet/staticcheck、完整及精选 lint、`deadcode -test`、禁缓存全量 test/race 全绿；MCP 高风险 race 20 次稳定；三个 strict codec fuzz 共 613,917 次执行无失败；Agent standalone tidy-diff/build/vet/test 与根仓库 build/vet/test 全绿；gofmt、边界与旧词扫描零违规。P15 完成 |
| 2026-08-10 | P15-03d（MCP lifecycle naming） | 最终局部变量复扫发现 `infra/mcp` 仍用 `ms` 泛指 configured server state、用 `old` 泛指已从 live projection 脱离且待关闭的 session。按真实生命周期统一为 `configuredServer`、`detachedSession` 和 `serverName`，Reconnect/Configure/Authorize/Detach/Dial/Probe/Statuses/Tools/publish/shutdown 全路径使用同一语言，注释同步且不改变锁、generation、session ledger 或 Tool publication 行为 | MCP Infra、直接 Adapter/Application/Bootstrap 爆炸半径与 architecture 禁用缓存测试、vet 全绿；完整 standalone/race/lint/deadcode/generator/fuzz 和边界复扫由 P15-04 收口 |
| 2026-08-10 | P15-03c（lifecycle and adapter vocabulary） | Goal 的 process-local active-lease incarnation 从 `loopHandle`/`activeLoop`/`running` 多词漂移收敛为单一 `goalDrive`、`activeDrive` 与 `drives`，类型及完整 lifecycle 行为共同归 `loop.go`；JSON-RPC 调度返回值由口吃式 `dispatch.DispatchResult` 改为 `dispatch.Result`，方法泛型返回值同步命名为 `ResponseValue`，两类 Result 不再遮蔽；caller-defined provider 统一使用完整 `Compatible`，adapter builder 按实际构造行为命名，清除把 compatible endpoint 误称 legacy compat/pass-through、把 direct vendor wire 泛称 native 的注释。Framework Strategy 在权威文档只称 `Interaction`，新增 package-qualified retired-name 与精确文档词汇守卫；Go 1.22 后无效 loop-variable copies 同批删除。高重复 race 进一步证明 faulting-store 测试信号早于 drive completion publication，场景现明确容许 publication 前的 `ErrGoalActive`，并继续证明发布后的失败在 Stop 前可寻址 | 受影响 Goal/LLM/Delivery/architecture 禁用缓存测试与 vet 全绿；`revive`、`copyloopvar`、复杂度、重复、未使用参数及 Go 风格精选 lint 零问题；完整 standalone/race/generator/deadcode/fuzz 与边界复扫在 P15-04 统一收口 |
| 2026-08-10 | P15-03b（outer-ring ownership and vocabulary） | 删除 Bootstrap 与 `runsegment` 测试中自行拼装、解析 executor tree payload 的伪模型；Bootstrap write-set fixture 只持 opaque bytes，runsegment 只证明 checkpoint 旧 bytes 原子保留、新 bytes 原子替换，真实 TreeSnapshot 语义继续由 `agentexec` 独占。Adapter/Infra/Delivery/Bootstrap fixture 与错误文本统一使用 executor member/input request/child Run；已删除旧实现后失去区分对象的 `native` 限定词从代码与当前合同文档移除，provider SDK、OS path 等真实原生语义不受影响。LLM 内部表由泛化 `Info`/`Entry` 收口为 chat/embedding provider catalog/profile；execution context 文件按实际职责改为 `scope.go`；Runtime 指南的 SQLite epoch 同步为唯一事实 65。新增 checkpoint opaque boundary 与已淘汰 catalog 名称架构守卫 | Adapter/Infra/Delivery/Bootstrap、直接 Application 爆炸半径与 architecture 禁用缓存测试全绿；本批提交前执行 standalone tidy/build/vet/staticcheck/test/race、完整 lint、deadcode、生成物与 hygiene 门禁 |
| 2026-08-10 | P15-03a（child-start durable wire ownership） | 反证发现 `adapter/runsegment` 直接 marshal `application/runs.ChildRunStartReservation`，使 Application Go 字段布局隐式成为 SQLite durable wire。改为 runsegment 私有、显式 JSON tag、去除已有独立列重复身份的 canonical payload；SQLite 继续只比较 opaque bytes，不解释产品事实。持久 shape 按规则一次提升到 epoch 65，旧 epoch 确定性拒绝，不增加 migration、dual read 或兼容字段；SQLite 测试 fixture 的 `process-*` 技术身份同步改为准确的 `member-*` | 新增 exact canonical payload/UTC column ownership 回归测试；runsegment、SQLite、architecture 普通测试与 race、vet、staticcheck、baseline epoch consistency 和 diff check 全绿 |
| 2026-08-10 | P15-02（Domain/Application ownership and vocabulary） | 删除只向合同生成器提供说明性数据、却伪装成生产用例依赖的 `application/invariant` package，将 system-invariant catalog 收回 `cmd/contractgen`；Architecture 直接审计生成后的 authoritative manifest，事务行为仍由各 Application write-set 独占。删除 Conversation append、Hook event probing、Schedule latest-update、Pending lookup、child-binding tree validator、Transcript accounting addition 与 MCP public-name 等没有生产消费者的导出链，避免形成第二行为 owner。Domain/Application 结果值按真实用途由泛化 `Info` 收口为 `ProviderSummary`、`SkillSummary`、`ProposalReview`、`AdvertisedTool`；rollback/fork 水位统一为 `MessageMark`/`KeepMessageMark`，首次用户输入投影改为 `OpeningUserMessagesByRun`，残留 Framework `Process`/subagent 语言改为中性的 executor member/child Run。Domain package 说明明确只拥有聚合、值与纯策略，I/O port 仍由 Application consumer 独占 | 受影响 Domain/Application 及直接 Adapter/Infra/Delivery/Bootstrap 爆炸半径禁用缓存测试全绿；`staticcheck` 与 `deadcode -test` 零问题；旧标识符、空目录/空文件、生产 TODO/FIXME/HACK 与 Domain/Application subagent 术语复扫仅保留 Hook 公共 vocabulary；完整 standalone/race/lint 由本批提交前门禁验证 |
| 2026-08-10 | P15-01（exact control and steer bridge） | Runtime standalone 依赖提升到 Agent Framework Baseline 18；root/child cancellation 改用只承诺进入 Engine 单写者队列的 `RequestCancellation`。Steer bridge 删除按到达顺序与 caller 返回时机猜测生效的 `accepted` slice，以及持锁等待 Engine 的死锁窗口；改由 `ModelInvocation.AppliedSteerSignalIDs` 精确选择首次进入当前模型请求的产品消息，并以一个 authoritative fact 原子投影整批。Signal ID→产品内容映射只由 `adapter/agentexec` 持有，并进入 opaque executor checkpoint schema v3，Application 与 Agent Framework 均不见对方抽象。Prepared waiting-subtree expiry 同时改为在 capability 自身 mutex 下装卸 timer，清除发布后无锁写入竞态 | text/image pending steer canonical round-trip、旧 schema 拒绝、重复/乱序/混合 wire 拒绝、malformed Application fact 拒绝与同一 model boundary 双 steer 单批投影均有回归测试；Runtime standalone tidy-diff/build/vet/staticcheck、禁用缓存全量 test、全量 race、完整 golangci-lint、`deadcode -test`、contract drift 与 architecture gates 全绿 |
| 2026-08-10 | P14-04i（Final naming and hygiene freeze） | 最终命名反证将 Tool `Call` 替换能力统一为 `decorateCall`/`callDecorator` 并以 `call_decorator.go` 承载；HTTP 请求 tracing、response metrics 与 panic containment 统一为 `instrumentRequests` 和 `request_instrumentation.go`，清除名词式方法、含混局部缩写及误称 logging 的注释。JSON-RPC `Router` 的传输消费面统一为 `messageDispatcher.Dispatch`/`dispatch.Result`，注册项内部行为命名为 `pipeline`，不再与 `net/http.Handler` 混用 `Handle` 语言。架构守卫按精确 package 封锁 P14 淘汰的 owner/error/dispatch 名称和失真文件路径，不建立全局禁词或误伤 Go 惯用短名 | Runtime standalone tidy-diff、build、vet、staticcheck、完整 lint、精选 naming/duplication/complexity lint、`deadcode -test`、禁用缓存全量 test/race 全绿；contract generator 零漂移，三个 strict continuation codec fuzz owner 各运行 10 秒无失败；当前文档本地链接、跨环/Framework import、旧名称、tracked 空文件、空目录和生产 TODO/FIXME/HACK 复扫零违规。P14 完成，未修改协议、持久化 shape、Agent Framework 或消费者 |
| 2026-08-10 | P14-04h（Lifecycle, error, and identifier semantics） | Run Application 将含混的 process-local `handle` 治本改为唯一 `runTreeOwner`，文件、registry 字段、publisher/pump 依赖、cancellation arbiter、executor member binding 与测试名称同步使用同一所有权语言；Goal session mutation 的活动查询最终统一为 `activeDrive`，进程内 lease incarnation 统一为 `goalDrive`。所有具体错误结构统一以 `Error` 结尾，`shutdown.Attempt.Result` 按 Go 惯例返回 `completed, err`。Toolset/Adapter/Infra 的私有 owner 进一步收敛为 `manifestBuilder`、`callDecorator`、`managementTools`、`commandTools`、`searchableTool`/`rankedTool`、`hooksFile`、`cachedCorpus`、`activeDial`、`mutationScope`、`toolListTarget`、`usageAccumulator` 与 `attemptState`；测试 helper 删除从未变化的参数并以固定场景命名，不再伪造未验证的可配置能力 | Runtime `go test ./internal/...` 通过；完整 `golangci-lint`、精选 naming/style/duplication lint、生产/测试 `gocognit`/`gocyclo` 与 `deadcode -test` 均为零问题。未新增端口、wire、持久化 shape、消费者改动或 Agent Framework concrete type；最终 standalone/race/fuzz/generator/边界复扫仍由 P14-04 收口 |
| 2026-08-10 | P14-04g（Architecture guard ownership and naming） | 架构守卫把 source walk、AST declaration collection、receiver naming、mutable catalog detection、constraint emitter expectation、comment boundary、cursor namespace 和 retired vocabulary 扫描分别收敛到准确 helper；大型测试只陈述不变量与场景，不再同时承担遍历、解析、分类和断言。Value constraint 的 schema keyword/Go helper 映射成为显式 `compiledConstraintExpectation`，不再靠一个 89 分支过程维护三类生成物；共享生产 Go source walker 只提供扫描机制，不承载具体架构策略 | `internal/arch` 禁用缓存测试通过；Runtime 全模块生产与测试 `gocognit`/`gocyclo` 零问题；所有既有 boundary/contract/naming guard 仍通过，未改变任何生产代码、协议、持久化或消费者合同 |
| 2026-08-10 | P14-04f（Run-tree cancellation and segment publication） | Cancellation plan 将 durable `cancellationRunTree` 的 topology/lifecycle 与 process-local member binding 分离，变量与字段统一使用 `memberIDsByRunID`/`targetRunIDSet` 等真实映射语义；tree barrier 的 checkpoint、Pending continuation 和完整 Run write-set 由单一私有 validator 证明。Segment startup 以 `segmentStartup` 明确管理 durable opening 前可回滚资源，commit 后才转交 pump；tree-barrier reduction 将 interruption source indexing、per-Run suspension projection、atomic commit/publication 分阶段收敛。Reducer 把 ItemStarted 的 Session 归属放回单事件投影，并由 park-boundary owner 组装唯一 suspend write-set | `application/runs` 普通测试及完整 lint 全绿；该包生产 `gocognit`/`gocyclo` 热点归零；fresh/resumed opening、cancellation、tree barrier 与 reducer 既有行为矩阵通过；没有新增端口、协议、持久化 shape、消费者改动或 Agent Framework concrete type |
| 2026-08-10 | P14-04e（Contract and continuation invariant ownership） | Contract generator 将字段约束、枚举、union/object shape 各自收敛到准确的 validator check owner；Transcript `Item` 继续拥有完整聚合校验，但 envelope、kind payload、tool-call payload 与 disallowed payload 各归命名行为。Run continuation 将 text/choice answer、waiting tree envelope/topology/order、resume interrupt/tool binding 分别交给其真实 invariant owner；恢复 route reconstruction 以私有 `resumedRouteBuilder` 同时拥有新 Segment identity 唯一性、per-Run reducer 构造和 deterministic preorder，route commit/event 防串写校验不再藏在闭包或混合条件树中 | `cmd/contractgen`、`domain/transcript`、`application/runs` 普通测试和完整 lint 全绿；受影响生产函数 `gocognit`/`gocyclo` 热点归零；没有新增 Application port、协议 wire、持久化 shape、消费者改动或 Agent Framework concrete type |
| 2026-08-10 | P14-04d（Session rollback and snapshot invariants） | Session Rollback 保持一个 guarded use case，但把 Timeline boundary/dropped-Run projection、workspace restore failure cleanup 和最终历史提交从主编排中收敛到准确行为；`ses`/generic admission/`restoreLogged` 等局部语义分别改为 current Session、Session/working-tree mutation 与 recorded mutation intent。Portable Snapshot 的 Run tree 校验由内聚的 `snapshotRunTree` 拥有 Run/parent index、named visit state、cycle detection 与 resolved-root lineage，不再用匿名递归闭包共享四张无 owner map | `application/sessions` 普通测试、race、完整 lint 及 `gocognit`/`gocyclo` 全绿；rollback file/history/recovery 与 snapshot referential-integrity 既有矩阵通过；未改变 Session artifact、SQLite、协议或外部消费者合同 |
| 2026-08-10 | P14-04c（Goal/Schedule orchestration and naming） | Goal autonomous loop 将 Run admission retry、start failure、terminal observation、Goal lease re-read 与 terminal resolution 收敛到准确私有行为，删除重复 terminal span projection，并把残留 `turnCost`/`turnSteps` 治本统一为 Run 语言；测试 fake 将 start attempt、Goal 状态脚本、active Run 注册和 terminal stream 分别收敛到命名行为。Schedule worker 以有界 `occurrenceBatch` 明确 pending dispatch、due claim 与 batch budget，`RunHandle`/`Runner`/`RunUseCases` 分别改为表达返回事实和消费能力的 `StartedRun`/`ScheduledRunStarter`/`RunStarter`，局部 `sc`、`mark`、`handle` 等历史缩写同步消除 | `application/goals`、`application/schedules` 与受影响 Delivery server 测试、race、完整 lint 全绿；Goal/Schedule 两包 `gocognit`/`gocyclo` 零问题；未改变协议 wire、持久化 shape、消费者实现或 Agent Framework 边界 |
| 2026-08-10 | P14-04b（Application cancellation validation and scenarios） | `WaitingSubtreeCancellationCommit` 继续作为唯一聚合不变量 owner，但构造校验已按 root/Pending/checkpoint boundary、surviving disposition envelope、pending-tree topology 和 surviving continuation 分阶段收敛，返回 Run identity 的行为使用明确的 collect 命名，不再把多类验证藏进单个过程式条件树；child Segment publication、tree-barrier postorder 和 running-child cancellation 场景把生命周期事实、提交顺序、Pending binding、journal cursor 与终态断言收敛到准确的测试行为，主场景只保留用例时序 | `application/runs` 普通测试、race 与完整 lint 全绿；该包测试的 `gocognit`/`gocyclo` 热点归零，生产 `gocyclo` 归零；未改变 Application port、协议、持久化 shape、消费者或 Agent Framework 边界 |
| 2026-08-10 | P14-04a（execution/persistence test ownership） | Agent execution 的 waiting Delegate restore/cancel 测试共享准确的 `waitingDelegateFixture`，场景函数只保留行为顺序，边界、admission 与 event-order 断言各归命名 helper；Run-segment 的 tree resume、terminal checkpoint deletion 和 waiting-subtree restart 测试将数据库装配、故障注入、重启后的 Run topology/checkpoint/Item 验证收敛到各自 fixture；SQLite epoch refusal 测试将单 epoch 不变式独立为准确断言行为 | `agentexec`、`runsegment`、`infra/sqlite` 测试通过，三包 `gocognit`/`gocyclo` 零问题；只重构测试 owner，不改变生产合同、协议或消费者 |
| 2026-08-10 | P14-03（Delivery/Bootstrap ownership and naming） | `protocol` 将 package 说明、Runtime 方法面、版本窗口与资源 ID 前缀分别收敛到 `doc.go`、`runtime.go`、`version.go`、`identifiers.go`；`dispatch` 将请求解码与响应投影从含混的 `reply.go` 分离到 `params.go`/`response.go`，并把 StreamFrame、Router 注册闭包及 metadata 变量统一为准确语义；`server.New` 明确分为配置校验、默认值、合同事实推导、构造和通知源观察，Session artifact、Run event presentation 与 JSON-RPC message 文件按真实职责命名；Bootstrap 将 conversation/model/MCP/tool 组合图和 Run maintenance 从巨型构造函数提取为单一职责文件，Host lifetime 字段与组合根局部值全部按真实 owner 命名，删除无意义 executor alias；旧含混文件路径由架构守卫禁止回流 | Bootstrap/Delivery/architecture 禁用缓存测试通过；Bootstrap/Delivery 生产 `gocognit`/`gocyclo`/`revive` 零问题；空目录和生产 TODO/FIXME/HACK 扫描零残留；未改变 wire shape/value/tag，未修改消费者或 Agent Framework 边界 |
| 2026-08-10 | P14-02（Adapter/Infra ownership and naming） | 将混合历史压缩、记忆、Skill 与会话标题的 `adapter/maintenance` 治本拆为 `runmaintenance`、`sessiontitle` 与共享的 `utilitymodel`；类型与文件按真实行为统一为 `MemoryConsolidator`、`SkillProposalMiner`、`IdleSkillArchiver`、`LiveStateSnapshotter`、`Generator` 及 memory/skill/transcript 责任文件，删除 `Maybe*`、`Extractor`、`Titler` 等含混或过时词汇；`agentexec` 将 deployment assembly 收敛到私有 builder、restore delegate call 收敛到准确行为，interaction input codec 按 JSON object/field/array 验证分责；sandbox archive 将写入与提取分别交给有状态 owner，SQLite Transcript item append 将 offload identity、record write 与错误解释分责；Session write-set 删除无行为的 transaction 转发层，并将 rollback、restore、parked terminal 的各个原子子行为收敛为准确私有方法 | Adapter/Infra/Bootstrap/architecture 受影响包禁用缓存测试通过；Adapter/Infra 生产 `gocognit` 热点归零（Bootstrap composition root 留 P14-03）；`revive` 全绿，旧 package/文件/标识符由架构守卫禁止回流；未修改协议、消费者或 Agent Framework 边界 |
| 2026-08-10 | P14-01（Application/Domain invariants and naming） | Run 协调器将含混的通用依赖检查改为准确的 Start/Resume staging 检查与共享 segment-supervision 边界，删除 Resume 的重复 Run projection 检查；`run.Tree` 将成员索引校验与 canonical topology traversal 收敛到私有 builder 行为；`Pending` 与 `EventCommit` 仍由聚合拥有验证，但 envelope、continuation/interrupt/binding 及 item/invocation/lifecycle 不变量各归准确的内部行为；Domain Run API 清除 `run.RunState` 一类 package stutter，统一为 `run.State`、`run.Status`、`run.Draft`、`run.Limits`、`run.Capabilities`、`run.Lineage`、`run.Tree`/`run.NewTree`，文件按 lifecycle/limits/resume/admission 的真实内容分责；Run journal 删除没有消费者的导出面；Session mutation callback 改为使用真实传入的 transaction context | `domain/run`、`application/runs`、`application/sessions` 及直接爆炸半径禁用缓存测试全绿；revive 复扫中 Domain Run package stutter 与 unexported-return 归零，未新增端口、外环依赖、wire 变化或兼容面 |
| 2026-08-09 | P13-04（final boundary freeze） | 完成 Runtime/Agent Framework 双向抽象泄露反证：Framework 生产代码不含产品、持久化或 Runtime 类型；Runtime 的 Framework import 仍唯一收敛于 `adapter/agentexec`，Domain/Application/Infra/Delivery 不持有 Framework 状态，不解析 private strategy payload。剩余高分支生产行为逐项复核为聚合不变量、递归 wire 校验、原子写集、组合根或安全状态机，没有为复杂度分数制造第二 owner。前端、TUI、CLI 和用户并行修改保持未触碰 | Runtime standalone tidy-diff/build/vet/staticcheck/完整 lint/deadcode/test/race 全绿；3 个 strict ACL codec fuzz 本轮约 42.5 万次执行无失败；contract generator 重跑后 manifest/schema/OpenRPC/Go validator/TypeScript 零漂移；Agent Framework standalone 同级门禁、13 个 fuzz owner、8 个实跑 examples 与 root workspace build/vet/test 全绿；跨环 import、旧术语、tracked 空文件/空目录、生产 TODO/FIXME/HACK 扫描零违规，P13 完成 |
| 2026-08-09 | P13-03（Adapter/Infra/Delivery） | Agent Framework ACL 将 continuation answer 的全量翻译校验置于 Signal 投递前，并统一 streaming 失败的权威投影结算；Run persistence 将 opening admission、model/tool invocation、progress 与 waiting-subtree 原子写集收敛到准确的私有 owner，`Pending` 的持久值相等行为归还 Application，Adapter 不再理解 SQLite 表示差异；Delivery contract validation 按 identity/problem/operation/shape/capability 和 union validation state 分责，清除稳定注释中的阶段、具体存储与具体消费者词汇；archive extraction 以平台无关规则拒绝反斜杠和 Windows drive path；skill lifecycle move 保留同一 capability root 内的精确 `os.Root.Rename`，只分离 replay/outcome reconciliation，避免通用 move 的冲突改名语义破坏稳定 destination identity | 受影响 Application/Adapter/Infra/Delivery packages 的 test/race 全绿；Runtime 全量 test 除精确 owner 门禁随重构迁移后通过，vet/staticcheck/完整 lint/deadcode 零问题；架构 DAG、checkpoint transaction owner、空目录、坏味道标记和外环词汇扫描零违规 |
| 2026-08-09 | P13-02（Application/Domain final audit） | 将 MCP connection replacement 从一个混合 HTTP/stdio/authorization/headers/environment 的条件树收敛为 transport-specific resolver 和三种精确 secret-change 行为，没有引入不透明泛型；全 Application/Domain 复核确认剩余高分支点均是单一状态机、聚合不变量或顺序敏感 saga，继续拆分会产生第二 owner。Domain 生产代码保持 context/I/O/Framework import 为零，Application 端口继续由真实消费者拥有 | MCP 全量测试通过；Application/Domain hygiene 与依赖扫描零违规；Runtime 全量质量门禁通过后冻结 P13-02 |
| 2026-08-09 | P13-02（Application recovery / parked lifecycle） | 将 boot recovery 收敛为一次启动快照专属 planner：库存读取、Pending/checkpoint 唯一性、Session 级缓存、tree preservation/loss 和最终写集各有明确阶段；等待 child cancellation 的长 saga 拆为 admission 后重读、checkpoint continuation、stays-waiting commit/apply 与 resumes-running 四个行为；Session parked terminalization 从 Coordinator I/O 编排中抽出纯写集行为对象，并修正 `Snapshot` 过窄注释。恢复策略仍属于 Application，Run 聚合没有接收 I/O，executor checkpoint 仍完全 opaque | `application/runs`、`application/sessions` 全量 test/race、Runtime vet/staticcheck/lint 全绿；全量回归发现并修正 parked terminalization owner 的硬编码架构门禁后，arch 与全 Runtime tests 通过 |
| 2026-08-09 | P13-02（Application Run lifecycle） | 精修取消与 executor fact 归约链路：以 command-bound source 收敛树读取、live/pending owner 仲裁和计划构造；等待子树取消由纯 builder 分阶段生成 Run/Item/Continuation/Pending 原子写集，删除同一 prepared payload 的重复校验；Run reducer 的入口只编排 fact reduction、projection 与 durable observation，模型调用、工具调用和 Segment 终止分别拥有自身状态行为。所有新增 owner 均为 `application/runs` 私有实现，没有新增端口、协议或 Framework 类型依赖 | `application/runs` 全量 test/race、vet、staticcheck、lint 全绿；等待取消、并发仲裁、reducer 既有回归矩阵全部通过 |
| 2026-08-09 | P13-01 | 反证发现 Architecture 仍携带 P11 实施状态及历史 Agent Baseline，Capability Ledger 的“当前事实”同时保留 P10、Baseline 14 和 SQLite epoch 62，而唯一 Contract Baseline 已是 Baseline 15/epoch 64。治本删除稳定架构中的阶段/版本事实，把当前能力事实同步到唯一合同，并新增文档事实门禁：稳定文档不得出现 phase 状态，Architecture 不得拥有 Framework version，Capability Ledger 的全部当前 baseline/epoch 必须与 Contract Baseline 一致 | 新门禁先稳定复现三类漂移，修正后通过；`git diff --check` 通过。未修改协议、持久化、生产代码或消费者 |
| 2026-08-09 | P12（final acceptance） | 完成 Runtime/Agent Framework 最终质量矩阵；将已完成工具规范从迁移名收敛为唯一 `TOOL_SYSTEM.md`，清除源码/测试中的模糊 vNext 术语；发现并修正 Contract Baseline 的 SQLite epoch 62→64 漂移并建立永久测试守卫；把单一消费者 `interruptcodec`、转发函数和重复 decoder 收回唯一 `interactioninput` ACL。strict continuation codec 现在拒绝大小写 alias、duplicate/unknown field 和 trailing value，并规范化空集合；fuzz 找到的两个反例作为 regression corpus 保留 | Runtime standalone tidy-diff/build/vet/staticcheck、完整 lint、禁用缓存全量 test/race 全绿；3 个 Runtime fuzz owner 单轮执行约 84 万次。Agent Framework standalone 同等门禁全绿，13 个 fuzz owner单轮约 320 万次、8 个 examples 全部实跑。SQLite/HTTP/SSE/in-process/recovery/rollback 关键矩阵在 race 下重复 10 次；root module build/vet/test、contract generator/digest、architecture/API/wire baseline、文档本地链接和最终 hygiene scan 全部通过。P12 完成；消费者仍按 handoff 独立接线 |
| 2026-08-09 | P12（zero-debt audit） | 完整 lint 发现并清除两处嵌入字段冗余选择器和一处复合字面量格式漂移；删除 1935 行已完成架构清洗台账及其索引，把历史实施事实归还 Git，避免已删除类型以“历史文档”形式继续形成第二真相源。当前六份重构 owner、工具规范与带日期外部证据各守唯一职责 | Runtime/Agent Framework `gofmt` 与 `golangci fmt` 零漂移；Runtime `golangci-lint` 零问题、`deadcode -test` 零内部死代码；tracked production TODO/FIXME/HACK、旧 Framework path/类型、旧 replay scope、空文件和空目录扫描为零。P12 继续执行全量行为矩阵 |
| 2026-08-09 | P11（publication completion） | canonical source commit 发布后，Runtime standalone 依赖绑定真实 `github.com/Tangerg/lynx/agent v0.0.0-20260809043847-2590dbc81a1f`；关闭 workspace 与本地 workspace 均消费同一 Baseline 15 源码，未建立 `replace`、alias module、临时 path 或双 Framework 路径 | Runtime `GOWORK=off` tidy-diff/build/vet/staticcheck/test/race 全绿；Agent Framework standalone 与 Runtime workspace 门禁全绿，P11 完成 |
| 2026-08-09 | P11（canonical source publication） | 删除原框架 module，把绿色重写实现安装为唯一 `agent` module；Runtime imports、workspace metadata、architecture guards、Baseline 15 和直接受影响文档同步。删除已完成迁移后仍描述 `agent/runtime`、`agentexec/turn` 的 execution/port 快照，不保留第二套现状 | Agent Framework standalone tidy-diff/build/vet/staticcheck/test/race 全绿；Runtime workspace 全量 test 通过。Runtime standalone 依赖将在 canonical source commit 推送后立即绑定其真实 pseudo-version，故 P11 当前为 4/5 |
| 2026-08-08 | P0 | 只读盘点 Runtime 当前 package、旧 Agent import、Agent Framework Baseline 9、协议制品与 SQLite schema epoch；确认选择原模块内局部绿色重写，不创建 runtime2 | 生产代码未修改；事实写入 Capability Ledger 与 Contract Baseline |
| 2026-08-08 | P0 | 建立并交叉收口六份核心文档，冻结 DDD/Clean Architecture 边界、Agent Framework 防腐合同、P1–P12 依赖、parallel harness/P8 cutover、恢复与副作用失败语义；识别并裁决 P7 两项 Agent Framework 中性前置合同 | 独立 Go spec review 结论 Approved/Ready；本 goal 未修改生产代码；本地链接检查与 `git diff --check` 通过 |
| 2026-08-08 | P1 | 冻结目标六环 DAG；Delivery 开始禁止 concrete Adapter；Agent Framework 只允许从 `adapter/agentexec` 导入两个已批准 public package；旧 Agent import、Domain context I/O ports、`component` umbrella、旧 private snapshot decoder 和唯一旧 lifecycle owner 全部进入精确 Temporary 台账 | 错误 Delivery→Adapter fixture 被稳定拒绝；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P2 | 一次性删除 `domain/execution`：Run、Accounting、Conversation、Transcript、Interrupt 与 ToolResult 按 bounded context 提升；executor ref/checkpoint、pending continuation、workspace mutation 归还 Application；Approval、Agent Memory、Codebase、Hooks、Provider 的十个 context I/O port 全部移到真实 Application consumer，Domain 生产与测试均禁止向外依赖 | Domain context I/O port 从精确十项例外降为零；旧 path、alias、空目录为零；Run 状态/lineage/capabilities、Conversation seed/truncate、usage monotonicity 与 checkpoint expectation 行为测试通过 |
| 2026-08-08 | P2 | SQLite executor checkpoint、pending interrupt 和 workspace mutation 改为 technical records，由 `adapter/persistence` 显式映射 Application values，清除 Infra→Application 反向依赖；终态统一为 Completed/Canceled/TimedOut/Failed/MaxBudget/MaxSteps/Lost，并同步服务端 protocol/schema/generated artifacts | architecture target DAG、Domain test isolation、strict storage codecs、outcome round-trip 与 compatibility differ 通过；`go test ./...`、`go vet ./...`、`go build ./...` 通过 |
| 2026-08-08 | P3 | 删除 `ExecutionControl`、`SegmentExecutor`、`SessionLifecycle` 与 `Effects` 胖边界；建立 root stage/commit/begin、observe/release 的消费方端口；Run pump 在所有非 Waiting 边界统一释放；Application executor identity 统一为 Member；SQLite epoch 59 一次性采用 `root_member_id`/`memberId` | fake-backed ordering/race/release/waiting tests 与 architecture vocabulary/port-shape guards 通过；`go test ./...`、`go vet ./...`、`go build ./...` 及 runs/sessions/runsegment/SQLite targeted race 通过 |
| 2026-08-08 | P4 | 在原 `agentexec` 内完成 Agent Framework Interaction root harness；每 root 独立 Engine + exact Deployment；Application 组装完整 Conversation seed；Result final 与 Delta 分离；集中映射 completion/model failure/cancel/deadline/panic；生产旧 owner 保留到 P8 | real Engine/Interaction integration、stage-zero-side-effect、complete WorkingContext、final-without-Delta、termination/panic/release tests 通过；architecture exact old-import ledger 未增长；`go test ./...` 通过 |
| 2026-08-09 | P5 | 建立 per-Effect dispatch-attempt 与 authoritative commit receipt；model/tool invocation journal、Transcript final、usage/pricing 和 Run progress 原子提交；并发 Tool 乱序完成按模型顺序批量结算；接通唯一 Toolset Manifest、scope、deferred advertisement、approval/hooks/presentation/offload/doom-loop；live unknown 由 wake + polling 收口为 durable RunLost | pre/post-call failure、chunk drop、并发逆序与 batch rollback、lost wake、RunLost retry-before-release、scope propagation、best-effort projection/hook、SQLite transaction/order integration tests 通过；`go mod tidy` diff-free，`go test ./...`、`go vet ./...`、`go build ./...`、`staticcheck ./...` 与 agentexec/runs/runsegment/SQLite targeted race 全绿 |
| 2026-08-09 | P6 | 以 Agent Framework public pending input/TreeSnapshot/RestoreTree/typed Signal 完成 Interaction waiting、exact restore、answer claim、resume、ask-user/interactive approval、deferred advertisement 与 safe-boundary steer；SQLite epoch 62 引入 hidden `resuming` answer audit，opening 强制证明 claim；旧 suspension 仅保留为 P8 production delete owner | live/cold resume、real ask_user、approval hook-once、advertisement restore、corrupt/build/deployment/workspace/capability failure、unknown no-checkpoint、conversation isolation、steer ordering、claim rollback/audit/replacement/terminal/boot cleanup、post-claim RunLost-before-release tests通过；`go mod tidy -diff`、`go test ./...`、`go vet ./...`、`go build ./...`、`staticcheck ./app/runtime/...` 与 Agent Framework/runs/runsegment/runrecovery/SQLite targeted race 全绿 |
| 2026-08-09 | P7 | 以 Agent Framework conclusive start outcome 和 one-shot prepared waiting-subtree change 完成 durable Delegate child Run、nested/sibling causal binding、restore attribution、waiting child cancellation 与 resulting checkpoint recovery；Application 与 Agent Framework 之间只传中性 member projection 和 opaque checkpoint | accepted→started/aborted、admission reject、multi/nested/restore、non-reentrant child commit、prepare/transaction/Apply-or-Discard、Apply/Continue 分相、commit-after-Apply-failure exact restore、restore failure RunLost 与 canceled-request-after-commit tests 通过；旧 turn shutdown 竞争目标测试 100 次、整包 10 次稳定；Runtime/Agent Framework `go mod tidy -diff`、全量 test/vet/build/staticcheck、Agent Framework 全量 race 与 Runtime 高风险 race 全绿 |
| 2026-08-09 | P8 | 原子切换 Bootstrap/boot recovery 到 Agent Framework Interaction；Application-owned WorkingContext composition、request-cancel/observe/release、tree-wide termination/recovery 与 Tool advertisement 均完成最终纵切；旧 Agent module dependency、GOAP/TurnProcess/turn/suspension/private-tree/duplicate-child 路径及临时例外全部删除；standalone module dependency 提升到实际消费的 Agent Framework Baseline 14 commit | terminal cause matrix、Delegate/waiting/restore/cancellation/unknown、cold restart、opaque checkpoint capability、Toolset advertiser 与 protocol lifecycle tests 通过；`go mod tidy -diff`、全量 test/vet/build/staticcheck、`deadcode -test`、standalone GOWORK=off 全门禁、Agent Framework/runs/runsegment/runrecovery/bootstrap race、cold restart 与 Interaction lifecycle 各 10 次重复验证全绿 |
| 2026-08-09 | P9.1 | 依据真实 import graph 清零 `component` umbrella：path identity、secret masking、notification relay 分别归 Infra/Application/Adapter；pagination/replay cursor、completion/HTTP origin/idempotency/shutdown/taskgroup 以准确 capability 存在；Bootstrap 中长期同步行为移出 composition root；并发 Tool attribution test 改为按稳定 model-call/index 断言而非 goroutine 到达序 | component path/empty dir/temporary ledger 为零；shared capability purity、content-codec boundary、Bootstrap no-business-method、inner-ring comment gates 全绿；`go mod tidy -diff`、全量 test/vet/build/staticcheck、`deadcode -test`、相关 owner/ring race 通过，并发 attribution 100 次重复稳定 |
| 2026-08-09 | P9.2 | 完成 Adapter/Infra/Application/Delivery 逐包职责审计；workspace physical path identity 收敛到唯一 Infra mechanism；删除空的 temporary architecture 台账并把旧 Agent 禁止与 Domain no-context-I/O 变为永久 framework boundary guard；确认 agentexec 按真实变化原因组织且没有第二 lifecycle owner或虚构子包 | Adapter→Infra 单向图与目标六环 DAG 全绿；Delivery concrete Adapter/Infra import、Infra 反向 import、Application outward import、纯转发 wrapper、package/type 口吃、空目录和 temporary exception 均为零；workspacepath/pathidentity/arch targeted tests 与全量质量门禁通过 |
| 2026-08-09 | P10 | Runtime Protocol 一次性提升到 `2026-08-09`、Session Artifact 到 v14；删除 wire 中实现泄露的 `processRootSegment`，只保留准确的 `runtimeInstanceRootSegment`；Go registry、validator、manifest、OpenRPC、JSON Schema、TypeScript binding、canonical samples 与人读 API/Transport/Aux 文档同步；新增精确 consumer handoff | canonical artifact samples 中漏存的 v12 与旧 `outcome.error` 被 strict sample gate 暴露并治本修正；生成器零漂移、旧 wire token/版本归零、strict validator/round-trip、HTTP/in-process、全量质量门禁通过；Desktop backlog 已记录但未改消费者 |

## 73. 当前下一步

P92 已把 P91 的 definite Host failure 精确限制在整个 Effect 均未跨越外部调用的情形，并让两 Tool canonical receipt 在 sibling failure 下可退出。下一批继续以一个客户端反证同 Segment terminal commit 的响应不明、重复恢复和 Run/Tool/Plan/Goal 失效收敛；`app/cli` 继续只读且不暂存。
