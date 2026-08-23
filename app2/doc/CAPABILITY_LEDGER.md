# app2 能力迁移账本

> 基线：2026-08-22；最近更新：2026-08-23。本文是迁移状态的唯一 owner。当前 app2 已完成 R1，并在继续
> R2–R11 production implementation；R2 的 Workspace 与 Session/Work Index 部分纵切、R5 的 Plan/Goal Runtime
> 与 Desktop consumer 已到 implemented，最终统一门禁未完成。
> 旧 app 的全绿证据不能冒充 app2 已完成。

## 1. 状态定义

| 状态 | 含义 |
| --- | --- |
| `specified` | 已登记目标 owner、阶段与验收，尚无 app2 生产实现 |
| `implemented` | app2 production 纵切与 owner 文档存在；按当前集中验证节奏，允许尚未运行最终测试/parity/recovery/product 门禁 |
| `verified` | 场景、合同、恢复、Desktop consumer 和相应视觉/包体验收均通过 |
| `cutover` | 产品入口已切换且旧实现已删除 |
| `blocked` | 有具名外部阻塞；必须记录 owner/解除条件，不能用作普通 pending |

能力只有达到 `verified` 才计入 Wave A parity。文件已复制、编译通过、mock UI、unit test 或旧 app 已通过均不算。

## 2. 总量基线

| 集合 | 旧基线 | app2 R0 | 完成门 |
| --- | ---: | ---: | --- |
| Runtime operations | 89 | 1 verified，9 implemented，79 specified | 89 verified |
| Operational probes | 3 | 3 implemented；local evidence 通过，remote R10 pending | 本地/远程机制均 verified |
| Runtime resource topics | 16 | 3 implemented，13 specified | 16 producer/consumer/resync verified |
| Run event variants | 7 | 1 implemented，6 specified | live + replay + recovery verified |
| Desktop product surfaces | 24 groups | 1 verified，3 implemented，20 specified | 全部 verified |
| 内置 tool presentation | 30 + MCP/unknown | 6 implemented，24 + MCP/unknown specified | 全部真实 material verified |

## 3. Runtime operation 全量映射（89）

目标 method 名精确保留当前 89 个名称；R1 generator 冻结其 shape 和 manifest。下表的 `target family` 只表达
app2 内部 owner，不代表改名或新建第二套 surface。

| ID | 旧 operation（完整集合） | 数量 | app2 owner / target family | 阶段 | 状态 | 核心验收 |
| --- | --- | ---: | --- | --- | --- | --- |
| O01a | `runtime.discover` | 1 | runtime protocol/discovery | R1 | verified | exact RequestMeta/capability、generated client、sidecar/instance/namespace identity、strict failure |
| O01b | `runtime.subscribe` | 1 | runtime subscription | R8 | specified | topic watch 有界；gap/new generation/buffer eviction resync |
| O02 | `sessions.list`, `sessions.get`, `sessions.snapshot`, `sessions.create`, `sessions.update`, `sessions.delete`, `sessions.fork`, `sessions.rollback`, `sessions.export`, `sessions.import` | 10 | session + sessionflow + artifact | R2/R9 | specified | CRUD/CAS、snapshot closure、fork boundary、rollback files/history、exact app2 artifact、cascade cleanup |
| O03 | `runs.start`, `runs.resume`, `runs.subscribe`, `runs.cancel`, `runs.steer`, `runs.get`, `runs.list` | 7 | run + runflow + session hydration | R3/R4 | implemented | single-open-tree、root tree Segment stream、tree-scoped replay/live handoff、running/waiting exact cancel、tree-atomic recovery、exact steer、cursor list 与 Desktop resume lease 已实现；HITL 统一门禁后 verified |
| O04 | `interrupts.list` | 1 | interrupt query in snapshot/audit | R4 | implemented | pending exact identity、cursor、settled 不复活、Session/Run filtering 与 snapshot consumer 已实现；待统一门禁 |
| O05 | `plan.get` | 1 | plan + planflow | R5 | implemented | Session current Plan、CAS revision、root terminal boundary、fork/rollback/import lifecycle、snapshot/event 同源、无 generic state registry；待 Desktop 与最终统一门禁 |
| O06 | `items.list` | 1 | transcript | R3 | implemented | Session/Run subtree + ASC/DESC keyset cursor、query-bound cursor、page Run ancestor closure；待统一门禁 |
| O07 | `workspaces.resolve`, `workspaces.list` | 2 | workspaceflow | R2/R6 | implemented | absolute identity、projectRoot 派生、missing 显式、无 active Project；Session create/native picker 已消费 exact ref，待统一门禁与 R6 consumer |
| O08 | `workspace.changes.list`, `workspace.diff.get`, `workspace.files.head`, `workspace.files.search`, `workspace.files.list`, `workspace.files.read` | 6 | workspaceflow + filesystem/git | R6 | implemented | jailed file list/read/head/search + Desktop lazy tree/exact grep/1000-line window 已接通；Git exact repository/workspace scope、staged+unstaged+untracked、unborn HEAD、honest numstat/binary、bounded subprocess output、raw/rows file-boundary truncation 与 Review consumer 已实现；待最终统一门禁 |
| O09 | `skills.discovered.list`, `skills.library.list`, `skills.library.archive`, `skills.library.restore`, `skills.proposals.list`, `skills.proposals.approve`, `skills.proposals.reject` | 7 | capability/skills | R7 | implemented | `.lyra/skills` 单一内容源、project-first/user lifecycle reconcile、exact immutable proposal review、confined atomic publish、Desktop 三面 consumer 与 external watch 已实现；待最终统一门禁 |
| O10 | `recipes.list`, `agentDocs.list` | 2 | capability/recipes/docs | R7 | implemented | confined project-first/global Recipe discovery、Desktop slash expansion + actual-payload idempotency、home→root→cwd AgentDoc discovery、fresh-root bounded injection/checkpoint freeze、Resources consumer 与 files invalidation 已实现；待最终统一门禁 |
| O11 | `mcp.servers.list`, `mcp.servers.create`, `mcp.servers.update`, `mcp.servers.delete`, `mcp.servers.test`, `mcp.tools.list`, `mcp.servers.reconnect`, `mcp.authorizationAttempts.create`, `mcp.authorizationAttempts.get` | 9 | integration/mcp | R8 | specified | closed transport union、secret set/clear/keep、six states、generation-safe reconnect、auth retention |
| O12 | `hooks.list`, `hooks.setTrust` | 2 | capability/hooks | R7 | specified | global/project sources、project trust、event/matcher/timeout、untrusted hook 不执行 |
| O13 | `approval.getMode`, `approval.setMode`, `approval.listRules`, `approval.forgetRule` | 4 | interaction policy | R4/R8 | specified | safe/balanced/yolo、rule scope/identity、explicit forget、Run decision transcript 独立 |
| O14 | `schedules.list`, `schedules.create`, `schedules.update`, `schedules.delete`, `schedules.runNow` | 5 | operations/schedule | R8 | specified | cron validation、revision、workspace/model、next run、runNow 返回可导航 Run/Session |
| O15 | `goals.start`, `goals.update`, `goals.clear`, `goals.get`, `goals.stop`, `goals.resume` | 6 | goal + goalflow | R5 | implemented | one incarnation、quiesce/CAS、paused/active/blocked/completing、Session cascade、autonomous control 不进 Transcript；待最终统一门禁 |
| O16 | `codebase.search`, `codebase.status`, `codebase.reindex` | 3 | workspace index | R6 | implemented | exact Session workspace、none/indexing/ready/error、durable operation CAS、replacement cancellation/crash recovery、Git-aware bounded source corpus、embedding role identity、atomic document replacement、bounded ranked hits 与 Desktop exact-line consumer；待最终统一门禁 |
| O17 | `providers.list`, `providers.update`, `providers.test` | 3 | integration/provider | R8 | specified | masked key/source、explicit secret change、base URL validation、test 业务判决、不泄漏原文 |
| O18 | `models.list`, `models.getUtilityRole`, `models.setUtilityRole`, `models.getEmbeddingRole`, `models.setEmbeddingRole` | 5 | integration/model | R8 | specified | provider/model 显式配对、capability/pricing/context、role validation、catalog invalidation |
| O19 | `tools.list`, `tools.invoke` | 2 | capability/tool diagnostics | R7 | specified | catalog 是诊断非 Run 配置；manual invoke 完整 approval/safety/result/error 语义 |
| O20 | `usage.session`, `usage.summary` | 2 | operations/usage | R9 | specified | exact attribution、day/model/provider buckets、unknown cost 缺席、terminal/restart 稳定 |
| O21 | `knowledge.list`, `knowledge.get`, `knowledge.update` | 3 | capability/knowledge | R7 | implemented | distinct home→projectRoot→cwd file cascade、confined physical-target CAS/atomic publish、fresh-root bounded injection/checkpoint freeze、exact external watcher、Desktop conflict-preserving editor 已实现；待最终统一门禁 |
| O22 | `agentMemory.list`, `agentMemory.review`, `agentMemory.update`, `agentMemory.delete`, `agentMemory.add` | 5 | capability/memory | R7 | implemented | project/user closed target、active/pending/rejected tombstone、user/auto provenance、transactional internal revision、dedupe/cap、review-only recall、Desktop management 已实现；待自动提炼与最终统一门禁 |
| O23 | `feedback.create` | 1 | operations/feedback | R9 | specified | bounded/redacted payload、run/session attribution、transport failure 不改 Run outcome |

总数：`1+1+10+7+1+1+1+2+6+7+2+9+2+4+5+6+3+3+5+2+2+3+5+1 = 89`。

## 4. Operational probe 映射（3）

| ID | 旧 endpoint | app2 本地 | app2 remote | 阶段 | 状态 | 验收 |
| --- | --- | --- | --- | --- | --- | --- |
| P01 | `/v2/info` | `/v2/info` + supervisor identity check | `/v2/info` | R1/R10 | implemented | local protocol/server/instance 与 `runtime.discover` 同 generation 已验证；remote 待 R10 |
| P02 | `/v2/health/live` | `/v2/health/live` | `/v2/health/live` | R1/R10 | implemented | local process/event loop 语义已验证；remote 待 R10 |
| P03 | `/v2/health/ready` | `/v2/health/ready` | `/v2/health/ready` | R1/R10 | implemented | local schema/store/dispatcher 与 draining 语义已验证；remote 待 R10 |

## 5. Runtime topics 映射（16）

| Topic | Producer owner | Desktop consumer | 阶段 | 状态 | 验收 |
| --- | --- | --- | --- | --- | --- |
| `files.changed` | filesystem/watch | Context Dock files/diff | R6 | implemented | selected Session 注册 exact workspace watch；path event 或 resync 失效 workspace query scope，Session/generation switch abort 旧订阅；待最终资源门禁 |
| `skills.changed` | skill store/watch | Skills view/query | R7 | implemented | archive/restore/approve/reject/propose committed mutation 与 project/user external edit 均失效同一 Skills query scope；每订阅 watcher 可取消并由 Bus close join，待最终统一门禁 |
| `mcp.changed` | MCP lifecycle | MCP settings/tool catalog | R8 | specified | status/auth/reconnect invalidate exact server |
| `schedules.changed` | schedule transaction | Schedules settings | R8 | specified | CRUD/runNow 后 exact IDs invalidated |
| `sessions.changed` | session transaction | Work Index | R2 | implemented | create/update/delete/fork/rollback/import committed mutation 发布，Desktop cursor catalog 与 exact snapshot 收敛；待统一门禁 |
| `runs.changed` | run transaction/recovery | Work Index + Agent/Dock audit | R3/R4 | specified | Run IDs/Session IDs，terminal/restart 不丢 |
| `plan.changed` | plan transaction | Plan pill/tooltip | R5 | implemented | committed replacement 与 Session lifecycle 精确失效，Desktop SSE consumer 回读 coherent snapshot；待最终统一门禁 |
| `goals.changed` | goal transaction | Goal tray | R5 | implemented | API、tool、driver、recovery 的 committed mutation 统一发布，Desktop SSE consumer 精确失效；待最终统一门禁 |
| `interrupts.changed` | interrupt transaction | Composer/Narrative attention | R4 | implemented | open/resume-consume/waiting-cancel transaction 后发布 exact Session/Run ids；Desktop 精确失效 snapshot，待统一门禁 |
| `knowledge.changed` | knowledge file owner | Knowledge view | R7 | specified | external edit 与 CAS update 收敛 |
| `hooks.changed` | hook/trust owner | Hooks settings | R7 | specified | trust/source change 收敛 |
| `models.changed` | provider/catalog owner | model picker/settings | R8 | specified | catalog/role/provider update 收敛 |
| `approvals.changed` | policy owner | Approval settings | R8 | specified | mode/rules exact invalidation |
| `agentMemory.changed` | memory owner | Memory view | R7 | implemented | changed-only review/add/update/delete publish；Desktop Runtime-generation query family、acknowledgement-loss cold-read 收敛；待自动提炼 publish 与最终统一门禁 |
| `codebase.changed` | index worker | Search/index view | R6 | implemented | committed admission/terminal settlement 后通知；Desktop 按 Runtime generation + workspace query scope 回读 canonical status/search，resync 同样收敛；待最终统一门禁 |
| `resync` | runtime subscription | topic router | R1 | specified | gap/new generation/buffer eviction 后只重拉列出 topics |

## 6. Run event 映射（7）

| Event | Authority/replay | app2 projection | 阶段 | 状态 | 验收 |
| --- | --- | --- | --- | --- | --- |
| `segment.started` | authoritative/replayable | Run/Segment lifecycle | R3 | implemented | committed admission/resume/replay 使用同一 Run/Segment identity；待统一门禁 |
| `segment.progress` | ephemeral/non-replay | usage/context/live progress | R3 | implemented | settled model call 发布 cumulative usage/step/context preview；terminal Run facts 收敛，drop 不改结果 |
| `segment.finished` | authoritative/replayable | outcome/segment terminal | R3 | implemented | terminal outcome/metrics durable commit 后发布；待故障矩阵统一门禁 |
| `item.started` | authoritative/replayable | placeholder/source owner | R3/R4 | implemented | model anchor 以 Effect identity 派生稳定 key；ToolCall running Item 与 event 同事务；delegated observation 先绑定 exact child Run/Segment，intrinsic-input 直接投影 Question；待统一门禁 |
| `item.delta` | ephemeral/non-replay | message/reasoning/tool streaming | R3 | implemented | message/reasoning append bounded live-only，SSE 无 id；Tool streaming 随 R4 完成 |
| `item.completed` | authoritative/replayable | durable content/material | R3/R4 | implemented | model completion 替换同 key provisional；Tool terminal Item、offloaded result 与 event 原子提交；child terminal material 与 parent Delegate Item 同事务结算，settled Output 补漏；待统一门禁 |
| `plan.updated` | authoritative/replayable | Plan resource projection | R5 | implemented | 成功 `set_plan` 的 committed result 投影，revision/order/status 与 `plan.get` 同源；待最终统一门禁 |

app2 精确保留这些 wire discriminants 与 authority/replay 分类；变化必须满足 ADR-A2-022 的协议门槛。

## 7. Desktop 产品表面（24 groups）

| ID | 产品表面 | 目标 owner | 阶段 | 状态 | 可观察验收 |
| --- | --- | --- | --- | --- | --- |
| U01 | Wails window/titlebar/chrome/min geometry | NativeHost + shell | R1/R10 | implemented | Wails v3 beta.12、精确 host surface、min 1120×720、signed package；R10 完成全量视觉/原生验收 |
| U02 | Runtime start/reconnect/error/version | supervisor + runtime context | R1 | verified | one-shot descriptor、sidecars + `runtime.discover`、SIGKILL successor、stale callback ignored、bounded backoff、quit join |
| U03 | Work Index/new Session/cwd grouping/search | sessions context | R2 | implemented | source-owned paged projection、exact cwd、status、native create、rename/favorite/delete/search/keyboard；fork 随 R9 |
| U04 | Agent Session hydrate/cold restore | agent context | R2/R3 | implemented | coherent snapshot + attach-first full replay/live fold、bounded dedupe、generation fencing；待 reload/restart 统一门禁 |
| U05 | Composer draft/attachments/paste/@file/history/IME | composer context | R3 | implemented | per-Session draft、image/text attachment+paste、history、IME-safe send、start/steer/stop、success clear/failure exact retry |
| U06 | Root narrative commentary/final hierarchy | agent presentation | R3 | implemented | user/work/reasoning/final/tool/question/compaction 同 renderer，work/final 分层与 reader-owned follow lock；待视觉统一门禁 |
| U07 | Delegated Run tree/disclosures | agent presentation | R4 | implemented | managed Delegate family、bounded tree budget、source-owned child projection、tree wait/resume/cancel/recovery 已建立；Desktop 以 `spawnedByItemId` 递归披露 sibling/depth/status/material，root-only stream lease 与 exact child cancel 已接通，`subagents` 已开放协商；待最终统一门禁 |
| U08 | Approval interaction | interrupt context | R4 | implemented | 单一整组 request surface、allow once/scope split/deny、edited JSON args、reason、settled exact identity；待统一门禁 |
| U09 | Question interaction | interrupt + composer | R4 | implemented | one atomic surface、多题顺序、text/single/multi/custom、IME-safe；当前 wire 无 Skip，待统一门禁 |
| U10 | Plan compact progress | plan context | R5 | implemented | canonical snapshot 的 ring + N/M、当前步骤、完整 checklist hover/focus；无复制 Plan state；待最终统一门禁 |
| U11 | Goal submit/tray/editor | goal + composer | R5 | implemented | `/goal` mode、budget、start/update/pause/resume/two-step clear、外部变化不覆盖 draft、SSE recovery；待最终统一门禁 |
| U12 | Context token gauge | agent projection + model catalog | R3/R8 | implemented | live preview + durable Run footprint 与 provider model contextWindow 合成；unknown 容量只显示 token，待 reload/import 统一门禁 |
| U13 | Context Dock tabs/split/persistence | workspace context | R6/R7 | implemented | Workspace/Session owner 分离；per-Session Files/Review/Codebase/Resources view、open tabs/selected target/expanded tree/search/resource subview 有界持久化；阅读与复合 workspace view 覆盖式扩展并保留窄窗 Work Index；待最终统一门禁 |
| U14 | Files/tree/read/head/search | workspace context | R6 | in_progress | lazy directory pagination、plain-text grep hit、exact line navigation、1000-line reader window、loading/error/empty/truncated/external change 已实现；语法 material 随 R10 renderer 收口 |
| U15 | Diff/Review Workspace | workspace context | R6 | implemented | Session-owned Files/Review switch、whole-change cards、exact-scroll navigator、true-unmount collapse、worktree/branch、unified/split、rename/untracked/binary/empty/error/truncated states 与 exact Open file 已实现；待最终统一门禁 |
| U16 | Terminal/tool detail/timeline/run summary | workspace + agent public | R4/R6 | implemented | shared tool presentation + known/unknown material disclosure；Session-owned overview/timeline/terminal/summary 从 canonical Run tree/Item/HITL 即时派生，支持 active child cancel、lineage integrity、bounded live tool output/follow lock、latest-tree summary/copy 与 complete empty/error states；无第二 timeline store，待最终统一门禁 |
| U17 | Markdown/text/code/table/math/diagram | UI renderer | R10 | specified | semantic Markdown、Shiki、copy/wrap、table preview、KaTeX/Mermaid lazy、safe HTML |
| U18 | Images/lightbox/native save | renderer + NativeHost | R10 | specified | grouping、zoom/keys/close、data validation、native save cancel/error、40px targets |
| U19 | Chat/session search and long-history navigation | sessions/agent | R9/R10 | specified | cursor/pagination、highlight/range、no full-history eager load、keyboard |
| U20 | Skills/recipes/docs/memory/knowledge/tool/index views | capability/workspace | R6/R7 | in_progress | Codebase 与 Resources 已覆盖 Skills、Recipes、AgentDocs、Knowledge；Memory 覆盖 project/user、pending review、active add/edit/pin/two-step delete、loading/error/empty/unavailable 与 lost-ack convergence；tool catalog 随 R7 |
| U21 | Provider/model/MCP/hooks/schedules/approval/usage settings | settings bounded sections | R8 | specified | form draft 与 wire 分离、validation/test/save、secret masked、query invalidation |
| U22 | Theme/accent/light-dark/i18n | shell/settings/UI tokens | R10 | specified | built-in static themes、8 locale parity 或显式新范围、reload、no raw key、RTL |
| U23 | Commands/shortcuts/toasts/menus/tooltips | shell + concrete registries | R10 | specified | keyboard scopes、collision、a11y、error isolation、no plugin host |
| U24 | Streaming scroll/virtualization/layout/visual quality | agent/shell/workspace | R10 | specified | raw follow lock、reader escape、resize/materialization、WCAG/IME/CJK/Retina/WebKit |

## 8. 内置 Tool presentation 全量

所有普通 ToolCall 使用透明 activity row；只有 delegated Run 等复合生命周期边界可使用 card hierarchy。

| Family | Tool names | 阶段 | 状态 | 验收重点 |
| --- | --- | --- | --- | --- |
| 文件与代码（5） | `read`, `glob`, `grep`, `lsp`, `apply_patch` | R4/R6 | specified | path/range/hits/symbol/diff；真实 material；不把 VCS status 与 patch status 合并 |
| 命令（3） | `shell`, `read_shell_output`, `stop_shell` | R4 | specified | command/description、stream/output、exit/duration、background exact ID、cancel |
| 网络（3） | `web_search`, `web_fetch`, `http_request` | R4 | specified | URL/status/header/body、安全 link、result normalization、bounded preview |
| Skill（4） | `list_skills`, `load_skill`, `read_skill_resource`, `propose_skill` | R7 | implemented | progressive tools 与 Desktop discovery 共用 archive-aware Lyra source；resource confined；root-only proposal 只进 review queue，正文与管理 UI owner 分离；待最终统一门禁 |
| Plan（3） | `enter_plan_mode`, `set_plan`, `exit_plan_mode` | R5 | implemented | root-only、durable Session read-only policy、dynamic effect gate、committed Plan fact、revision-bound approve/reject question；待 Desktop 与最终统一门禁 |
| Goal（3） | `create_goal`, `get_goal`, `report_goal_outcome` | R5 | implemented | 前两者常驻；outcome tool 仅 exact owned Run 可见；待最终统一门禁 |
| Schedule（3） | `list_schedules`, `create_schedule`, `delete_schedule` | R8 | specified | cron/title/identity，write safety，settings invalidation |
| 回忆与发现（4） | `search_memory`, `search_conversations`, `search_tools`, `read_tool_result` | R7/R9 | in_progress | `search_memory` 已使用 effective reviewed user+project corpus、bounded prose、lexical-complete + optional semantic degradation；其余随 R7/R9 |
| 委派与提问（2） | `delegate_task`, `ask_user` | R4 | implemented | child Run 以父 Delegate Item 为唯一 disclosure anchor；Question 为唯一交互真身；model/protocol field mapping 保持 Lyra 自有语义；待最终统一门禁 |
| MCP/unknown | dynamically discovered names | R8 | specified | remote original name 与 model-visible collapsed name 分离、safety class、JSON fallback |

当前仅 `glob/grep/apply_patch/shell/web_search` 有旧 Runtime 归一化 result schema；app2 R1–R8 要么为被 UI
读取的结果建立 canonical typed presentation，要么明确只用 safe generic preview，不能继续依赖无门禁约定。

## 9. 跨能力不变量

| ID | 不变量 | 必须覆盖的阶段 | 证据要求 |
| --- | --- | --- | --- |
| I01 | Session 至多一个 open root Run tree | R3/R4 | concurrent admission + DB unique/CAS + multi-process |
| I02 | terminal Run 说明如何结束 | R3/R4 | every terminal variant + recovery/import corruption |
| I03 | admitted Run capabilities immutable | R3/R5 | resume/Goal/delegated attempts to alter rejected |
| I04 | waiting tree 恰好一个完整 open Interrupt set | R4 | multi-question/approval/child barrier/crash |
| I05 | continuation 与 Run facts/occurrence/checkpoint 匹配 | R4 | stale/foreign/replayed responses rejected |
| I06 | dropped Run 不留任何资源 | R4/R9 | rollback/delete/import failure + task/listener ledger zero |
| I07 | imported Session identity closure 一致 | R9 | conflict/partial/corrupt/checksum/all-or-nothing |
| I08 | Goal 不晚于 Session | R5/R9 | delete/rollback/fork/archive lifecycle |
| I09 | Transcript/Conversation/WorkingContext 不互相冒充 | R3/R5 | autonomous control visibility + compaction/recovery |
| I10 | only committed facts reach UI/events | all | fault injection before/during/after commit |
| I11 | old generation cannot commit to successor | R1/R3/R4 | renderer/connection/Runtime replacement races |
| I12 | every acquired resource has bounded cleanup/join | all | leak detector + process/port/temp-home audit |

## 10. Intentional breaking / 非目标

以下不计为功能丢失：

- 不读取旧 SQLite epoch 77、Artifact v22；
- 不保留旧 Go package layout、handler/repository concrete API、frontend facade/store shape；
- Runtime Protocol `2026-08-21`、89 个 dotted methods、HTTP/SSE、sidecars 与部署所需 token/CORS 是主动继承的
  合同基线，不是兼容负担；只有 ADR-A2-022 规定的证据和版本门槛才能改变；
- 不保留外部 JavaScript plugin/sideload（当前生产已无此能力）；
- 不保留无生产者的 custom RunEvent、clientTools feature、toolResult Interrupt、generic state registry；
- 不保留旧 frontend public facade、plugin identity、extension point 或 store shape；
- 不保留已知不可达 locale/error 文案；
- 不迁移旧 bug。语义对照差异必须裁决并记录。

## 11. 每项能力的证据格式

一行从 `implemented` 升为 `verified` 时，在行下或对应阶段记录：

```text
Implementation: paths / canonical owners
Contract: generated method/types/events/errors
Tests: domain + transaction + adapter + Desktop scenario
Recovery: reload/reconnect/restart/SIGKILL as applicable
Product: package/visual/a11y/IME evidence as applicable
Resources: processes/ports/temp homes/browser sessions released
Decision: intentional differences from old app
```

没有上述闭环，不得仅凭“功能看起来能用”更新状态。

## 12. R1 证据索引

```text
Implementation: runtime/{operation,dispatch,rpcwire,httptransport,runtimehost,localruntime,sqlite,cli,contractgen}; desktop/{supervisor,DesktopHost,NativeHost,frontend,Taskfile}
Contract: protocol 2026-08-21; R1 verified runtime.discover; current 89 registered shapes + 4 fixed HTTP endpoints generated diff-free
Tests: Runtime/Desktop race+vet+Staticcheck; frontend type+lint+format+7 tests+build; signed package verification
Recovery: real child SIGKILL -> successor generation; graceful close; corrupt DB/no descriptor; predecessor UI result ignored
Product: arm64 lyra-app2.app contains sibling lyra-runtime; U01 remains implemented pending R10 visual/native matrix
Resources: controlled Runtime/Vite/agent-browser/Chrome/ports/temp roots released; unrelated user processes preserved
Decision: current Lyra contract preserved; no Codex protocol surface copied
```
