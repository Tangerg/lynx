# app2 能力迁移账本

> 基线：2026-08-22；最近更新：2026-08-24。本文是迁移状态的唯一 owner。当前 app2 已完成 R1，并在继续
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
| Runtime operations | 89 | 1 verified，88 implemented，0 in_progress，0 specified | 89 verified |
| Operational probes | 3 | 3 implemented；local/remote production path 已闭合 | 本地/远程机制均 verified |
| Runtime resource topics | 16 | 16 implemented | 16 producer/consumer/resync verified |
| Run event variants | 7 | 7 implemented | live + replay + recovery verified |
| Desktop product surfaces | 24 groups | 1 verified，23 implemented，0 in_progress，0 specified | 全部 verified |
| 内置 tool presentation | 30 + MCP/unknown | 30 + MCP/unknown implemented，0 in_progress，0 specified | 全部真实 material verified |

## 3. Runtime operation 全量映射（89）

目标 method 名精确保留当前 89 个名称；R1 generator 冻结其 shape 和 manifest。下表的 `target family` 只表达
app2 内部 owner，不代表改名或新建第二套 surface。

| ID | 旧 operation（完整集合） | 数量 | app2 owner / target family | 阶段 | 状态 | 核心验收 |
| --- | --- | ---: | --- | --- | --- | --- |
| O01a | `runtime.discover` | 1 | runtime protocol/discovery | R1 | verified | exact RequestMeta/capability、generated client、sidecar/instance/namespace identity、strict failure |
| O01b | `runtime.subscribe` | 1 | runtimeevents + Desktop invalidation router | R8 | implemented | bounded topic/watch、watch-ready cold resync、per-subscription sequence、gap/reconnect/queue eviction 全范围回读、cancel/join 已实现；待最终门禁 |
| O02 | `sessions.list`, `sessions.get`, `sessions.snapshot`, `sessions.create`, `sessions.update`, `sessions.delete`, `sessions.fork`, `sessions.rollback`, `sessions.export`, `sessions.import` | 10 | session + sessionflow + artifact | R2/R9 | implemented | 既有十方法 Lyra wire 不变；CRUD/CAS、200-Item bounded snapshot、exact root fork、history/files/both rollback、dropped-input restore、terminal app2/2 JSON/Markdown、strict conflict-reject atomic import、message/tool/lineage closure 与 native file transfer 已实现；待 R11 fault/parity/package 门禁 |
| O03 | `runs.start`, `runs.resume`, `runs.subscribe`, `runs.cancel`, `runs.steer`, `runs.get`, `runs.list` | 7 | run + runflow + session hydration | R3/R4 | implemented | single-open-tree、root tree Segment stream、tree-scoped replay/live handoff、running/waiting exact cancel、tree-atomic recovery、exact steer、cursor list 与 Desktop resume lease 已实现；HITL 统一门禁后 verified |
| O04 | `interrupts.list` | 1 | interrupt query in snapshot/audit | R4 | implemented | pending exact identity、cursor、settled 不复活、Session/Run filtering 与 snapshot consumer 已实现；待统一门禁 |
| O05 | `plan.get` | 1 | plan + planflow | R5 | implemented | Session current Plan、CAS revision、root terminal boundary、fork/rollback/import lifecycle、snapshot/event 同源、无 generic state registry；待 Desktop 与最终统一门禁 |
| O06 | `items.list` | 1 | transcript | R3 | implemented | Session/Run subtree + ASC/DESC keyset cursor、query-bound cursor、page Run ancestor closure；待统一门禁 |
| O07 | `workspaces.resolve`, `workspaces.list` | 2 | workspaceflow | R2/R6 | implemented | absolute identity、projectRoot 派生、missing 显式、无 active Project；Session create/native picker 已消费 exact ref，待统一门禁与 R6 consumer |
| O08 | `workspace.changes.list`, `workspace.diff.get`, `workspace.files.head`, `workspace.files.search`, `workspace.files.list`, `workspace.files.read` | 6 | workspaceflow + filesystem/git | R6 | implemented | jailed file list/read/head/search + Desktop lazy tree/exact grep/1000-line window 已接通；Git exact repository/workspace scope、staged+unstaged+untracked、unborn HEAD、honest numstat/binary、bounded subprocess output、raw/rows file-boundary truncation 与 Review consumer 已实现；待最终统一门禁 |
| O09 | `skills.discovered.list`, `skills.library.list`, `skills.library.archive`, `skills.library.restore`, `skills.proposals.list`, `skills.proposals.approve`, `skills.proposals.reject` | 7 | capability/skills | R7 | implemented | `.lyra/skills` 单一内容源、project-first/user lifecycle reconcile、exact immutable proposal review、confined atomic publish、Desktop 三面 consumer 与 external watch 已实现；待最终统一门禁 |
| O10 | `recipes.list`, `agentDocs.list` | 2 | capability/recipes/docs | R7 | implemented | confined project-first/global Recipe discovery、Desktop slash expansion + actual-payload idempotency、home→root→cwd AgentDoc discovery、fresh-root bounded injection/checkpoint freeze、Resources consumer 与 files invalidation 已实现；待最终统一门禁 |
| O11 | `mcp.servers.list`, `mcp.servers.create`, `mcp.servers.update`, `mcp.servers.delete`, `mcp.servers.test`, `mcp.tools.list`, `mcp.servers.reconnect`, `mcp.authorizationAttempts.create`, `mcp.authorizationAttempts.get` | 9 | integration/mcp | R8 | implemented | 既有 9-method Lyra wire 不变；private revisioned aggregate、secret set/clear/keep、真实 timeout、generation-safe connect/tool refresh/OAuth CAS、terminal retention、Desktop candidate test/auth/tool trust 已接通；待最终门禁 |
| O12 | `hooks.list`, `hooks.setTrust` | 2 | capability/hooks | R7/R10 | implemented | 独立 lifecyclehook domain、confined discovery/trust/observation，以及 prompt、Tool、subagent、waiting/terminal execution 已实现；真实 post-Run compaction candidate 在 summary model call 与 durable projection commit 前执行 `PreCompact`，deny 不改 journal；待最终统一门禁 |
| O13 | `approval.getMode`, `approval.setMode`, `approval.listRules`, `approval.forgetRule` | 4 | interaction policy | R4/R8 | implemented | 既有四方法 Lyra wire 不变；动态 effect stance、Session/Project/Global specificity、exact remembered subject、deny fail-closed、catastrophic override、remember/forget 与 Desktop rule consumer 已接通；待最终门禁 |
| O14 | `schedules.list`, `schedules.create`, `schedules.update`, `schedules.delete`, `schedules.runNow` | 5 | schedule + scheduleflow | R8 | implemented | 私有聚合与 normalized store、5-field cron、revision/no-op、显式/default workspace、paired model、durable occurrence、atomic Session/Run admission、pending recovery、runNow 可导航；待最终门禁 |
| O15 | `goals.start`, `goals.update`, `goals.clear`, `goals.get`, `goals.stop`, `goals.resume` | 6 | goal + goalflow | R5 | implemented | one incarnation、quiesce/CAS、paused/active/blocked/completing、Session cascade、autonomous control 不进 Transcript；待最终统一门禁 |
| O16 | `codebase.search`, `codebase.status`, `codebase.reindex` | 3 | workspace index | R6 | implemented | exact Session workspace、none/indexing/ready/error、durable operation CAS、replacement cancellation/crash recovery、Git-aware bounded source corpus、embedding role identity、atomic document replacement、bounded ranked hits 与 Desktop exact-line consumer；待最终统一门禁 |
| O17 | `providers.list`, `providers.update`, `providers.test` | 3 | integration/provider | R8 | implemented | masked key/source、explicit secret change、base URL validation、revision CAS patch convergence、bounded redacted test verdict、changed-only invalidation 与 draft/save/test Desktop card 已接通；待最终门禁 |
| O18 | `models.list`, `models.getUtilityRole`, `models.setUtilityRole`, `models.getEmbeddingRole`, `models.setEmbeddingRole` | 5 | integration/model | R8 | implemented | provider/model paired value、Session durable picker、fresh Run explicit pair、static/live catalog honesty、typed role editors、capability/pricing/context/image gate、changed-only invalidation 已接通；待最终门禁 |
| O19 | `tools.list`, `tools.invoke` | 2 | capability/tool diagnostics | R7 | implemented | Run 外目录闭合为 safe `read/glob/grep`，schema 来自 executable definition；manual invoke 绑定 exact Workspace 并复用 physical/symlink confinement，不伪造 Run/approval；Desktop schema/JSON/result consumer 已实现，待最终统一门禁 |
| O20 | `usage.session`, `usage.summary` | 2 | operations/usage | R9 | implemented | terminal durable Run typed projection、exact provider/served-model/UTC-day buckets 与 `provider/model` Session key 已实现；任一 contribution cost unknown 时对应 total/bucket 省略 cost；待 R11 aggregate/restart/corruption/UI 门禁 |
| O21 | `knowledge.list`, `knowledge.get`, `knowledge.update` | 3 | capability/knowledge | R7 | implemented | distinct home→projectRoot→cwd file cascade、confined physical-target CAS/atomic publish、fresh-root bounded injection/checkpoint freeze、exact external watcher、Desktop conflict-preserving editor 已实现；待最终统一门禁 |
| O22 | `agentMemory.list`, `agentMemory.review`, `agentMemory.update`, `agentMemory.delete`, `agentMemory.add` | 5 | capability/memory | R7 | implemented | project/user closed target、review lifecycle、recall/search/Desktop management，以及 completed-root bounded extraction、durable ledger、watermark CAS curation、pending-only proposal 已实现；既有 Lyra wire 不变，待最终统一门禁 |
| O23 | `feedback.create` | 1 | operations/feedback | R9 | implemented | private bounded/redacted record、Item→Run→Session canonical ownership、normalized store 与 root final-answer consumer 已实现；transport/storage failure 只留局部 UI error，永不修改 Run outcome；待 R11 fault/package 门禁 |

总数：`1+1+10+7+1+1+1+2+6+7+2+9+2+4+5+6+3+3+5+2+2+3+5+1 = 89`。

## 4. Operational probe 映射（3）

| ID | 旧 endpoint | app2 本地 | app2 remote | 阶段 | 状态 | 验收 |
| --- | --- | --- | --- | --- | --- | --- |
| P01 | `/v2/info` | `/v2/info` + supervisor identity check | 同 `/v2/info` | R1/R10 | implemented | remote HTTPS attach 同时校验 TLS、origin-only endpoint、server name、instance/protocol 与 `runtime.discover`；Desktop 有 profile/active identity 与 local escape，待最终 conformance |
| P02 | `/v2/health/live` | `/v2/health/live` | 同 `/v2/health/live` | R1/R10 | implemented | remote attach 与每次 bootstrap 都要求 live identity 对齐；active remote boot failure 可显式回到 local，待 offline/restart 最终矩阵 |
| P03 | `/v2/health/ready` | `/v2/health/ready` | 同 `/v2/health/ready` | R1/R10 | implemented | remote attach 不把 liveness 冒充 readiness，ready/discover/instance 同代；target change remount Workspace，待最终 conformance |

## 5. Runtime topics 映射（16）

| Topic | Producer owner | Desktop consumer | 阶段 | 状态 | 验收 |
| --- | --- | --- | --- | --- | --- |
| `files.changed` | filesystem/watch | Context Dock files/diff | R6 | implemented | selected Session 注册 exact workspace watch；path event 或 resync 失效 workspace query scope，Session/generation switch abort 旧订阅；待最终资源门禁 |
| `skills.changed` | skill store/watch | Skills view/query | R7 | implemented | archive/restore/approve/reject/propose committed mutation 与 project/user external edit 均失效同一 Skills query scope；每订阅 watcher 可取消并由 Bus close join，待最终统一门禁 |
| `mcp.changed` | MCP lifecycle | MCP settings/tool catalog | R8 | implemented | durable mutation与 connecting/connected/failed/disconnected/needsAuth/auth/reconnect 均由 MCP owner 发布；Desktop 失效 Runtime-generation MCP query family；待最终门禁 |
| `schedules.changed` | schedule owner/dispatcher transaction | Schedules settings | R8 | implemented | changed-only CRUD、occurrence claim/accept 与 runNow 发布 exact ID；Desktop Runtime-generation query 失效并冷读 authoritative revision；待最终门禁 |
| `sessions.changed` | session transaction | Work Index | R2 | implemented | create/update/delete/fork/rollback/import committed mutation 发布，Desktop cursor catalog 与 exact snapshot 收敛；待统一门禁 |
| `runs.changed` | run transaction/recovery | Work Index + Agent/Dock audit | R3/R4 | implemented | committed admission/status/recovery 发布 exact Run/Session identity；Desktop catalog/snapshot 失效并冷读收敛；待最终门禁 |
| `plan.changed` | plan transaction | Plan pill/tooltip | R5 | implemented | committed replacement 与 Session lifecycle 精确失效，Desktop SSE consumer 回读 coherent snapshot；待最终统一门禁 |
| `goals.changed` | goal transaction | Goal tray | R5 | implemented | API、tool、driver、recovery 的 committed mutation 统一发布，Desktop SSE consumer 精确失效；待最终统一门禁 |
| `interrupts.changed` | interrupt transaction | Composer/Narrative attention | R4 | implemented | open/resume-consume/waiting-cancel transaction 后发布 exact Session/Run ids；Desktop 精确失效 snapshot，待统一门禁 |
| `knowledge.changed` | knowledge file owner | Knowledge view | R7 | implemented | confined CAS update 与 global/project/cwd external watch 收敛；Desktop 失效 Runtime-generation knowledge query family；待最终门禁 |
| `hooks.changed` | hook/trust owner | Hooks settings | R7 | implemented | changed-only trust mutation与 global/project/cwd hooks.json exact-file observation 收敛；缺失 parent directory 的后续创建同样可观察，待最终统一门禁 |
| `models.changed` | provider/catalog owner | model picker/settings | R8 | implemented | changed-only provider/role mutation 发布；Desktop 同时失效 provider/model/role query，Session model identity 仍由 sessions.changed 收敛；待最终门禁 |
| `approvals.changed` | policy owner | Approval settings | R8 | implemented | changed-only mode/remember/forget 由 Approval owner 发布；Desktop 失效 Runtime-generation approval query family，持续 Run 下次 effect 同样读取新策略；待最终门禁 |
| `agentMemory.changed` | memory owner | Memory view | R7 | implemented | changed-only review/add/update/delete/automatic pending publish；Desktop Runtime-generation query family、acknowledgement-loss cold-read 收敛；待最终统一门禁 |
| `codebase.changed` | index worker | Search/index view | R6 | implemented | committed admission/terminal settlement 后通知；Desktop 按 Runtime generation + workspace query scope 回读 canonical status/search，resync 同样收敛；待最终统一门禁 |
| `resync` | runtime subscription | topic router | R1/R8 | implemented | subscriber 注册且 external watcher ready 后先发 cold-read resync；queue eviction 替换为全订阅 resync，Desktop sequence gap/reconnect 同样只失效列出 topics/watch scope；待最终门禁 |

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
| U03 | Work Index/new Session/cwd grouping/search | sessions context | R2/R9 | implemented | source-owned paged projection、exact cwd、status、native create/import/export、rename/favorite/delete/search/keyboard、whole/boundary fork 与 history/file rollback actions 已接通；待最终门禁 |
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
| U14 | Files/tree/read/head/search | workspace context | R6 | implemented | lazy directory pagination、plain-text grep hit、exact line navigation、1000-line reader window、loading/error/empty/truncated/external change 与 typed code/source renderer 已实现；待 R11 large-file/visual/WebKit/package 统一门禁 |
| U15 | Diff/Review Workspace | workspace context | R6 | implemented | Session-owned Files/Review switch、whole-change cards、exact-scroll navigator、true-unmount collapse、worktree/branch、unified/split、rename/untracked/binary/empty/error/truncated states 与 exact Open file 已实现；待最终统一门禁 |
| U16 | Terminal/tool detail/timeline/run summary | workspace + agent public | R4/R6 | implemented | shared tool presentation + known/unknown material disclosure；Session-owned overview/timeline/terminal/summary 从 canonical Run tree/Item/HITL 即时派生，支持 active child cancel、lineage integrity、bounded live tool output/follow lock、latest-tree summary/copy 与 complete empty/error states；无第二 timeline store，待最终统一门禁 |
| U17 | Markdown/text/code/table/math/diagram | UI renderer | R10 | implemented | Lyra-owned semantic Markdown/GFM/CJK、basic HTML allowlist、single URL policy、code copy/wrap + lazy Shiki fallback、semantic overflow table、KaTeX 与 viewport-lazy strict/sanitized Mermaid 已实现；待 streaming/large/a11y/visual/WebKit/package 统一门禁 |
| U18 | Images/lightbox/native save | renderer + NativeHost | R10 | implemented | renderer MIME/base64/size gate、相邻 grouping、lazy thumbnail、zoom/arrow/Escape/focus-contained lightbox、40px+ controls 与 NativeHost 二次校验/save cancel/error 已接通；无 browser download/任意路径；待 packaged native/visual/a11y 门禁 |
| U19 | Chat/session search and long-history navigation | sessions/agent | R9/R10 | implemented | 既有 `items.list(desc)` 按需翻页、snapshot overlap skip、Run summary merge、Cmd/Ctrl+F、Enter/Shift+Enter、loaded highlight/exact scroll 与逐 window Search older 已实现；mount 不 eager 全历史，待 large-history/a11y/visual/package 门禁 |
| U20 | Skills/recipes/docs/memory/knowledge/tool/index views | capability/workspace | R6/R7 | implemented | Codebase 与 Resources 已覆盖 Skills、Recipes、AgentDocs、Knowledge、Memory 与 direct Tool catalog；Tools 展示真实 schema/safety/workspace，提供 cancellable JSON invoke 与 result/error/empty 状态，并明确不冒充 Run，待最终统一门禁 |
| U21 | Provider/model/MCP/hooks/schedules/approval/usage settings | settings bounded sections | R8/R9 | implemented | explicit Settings 已完成 Provider/Model、MCP、Approval、Schedules、Lifecycle Hooks 与 7/30/all-time Usage；用量只消费 terminal Runtime authority，明确 unknown-cost，selected Session 单独展示；待最终 UI/restart/package 门禁 |
| U22 | Theme/accent/light-dark/i18n | shell/settings/UI tokens | R10 | implemented | typed boundary/hidden error channel 与 TSX AST copy audit 已闭合；9 个 exact 1039-key static dictionaries（含 Arabic）、ShellPreferences v2 locale owner/native selector、active-only lang/dir/Intl、logical CSS、directional control 与 technical-material bidi isolation 已实现，0 missing/extra/placeholder mismatch 且无旧 app runtime dependency/raw fallback；待 R11 native-language/RTL visual/WebKit/package 统一门禁 |
| U23 | Commands/shortcuts/toasts/menus/tooltips | shell + concrete registries | R10 | implemented | finite typed command catalog、platform/non-US-layout dispatcher、Mod+N/K/F/, 与 Escape scopes、Settings discoverability、bounded toast owner、IME/input/dialog guard、async error isolation、shared action-menu outside/Escape/focus/arrow navigation 与 hover/focus shortcut tooltip 已实现；无 command palette/plugin host，待 R11 a11y/IME/visual/package 门禁 |
| U24 | Streaming scroll/virtualization/layout/visual quality | agent/shell/workspace | R10 | implemented | Narrative/Terminal 共用 reader-owned follow lock、reader escape、未读提示与 ResizeObserver materialization；长历史沿用显式分页并以 native render containment 跳过离屏 layout，不牺牲 DOM/search/a11y；fluid min-window geometry、40px actions、IME 229 guard 与 CJK line-break 已收口，待 R11 Retina/WebKit/a11y/large-history/package 统一门禁 |

## 8. 内置 Tool presentation 全量

所有普通 ToolCall 使用透明 activity row；只有 delegated Run 等复合生命周期边界可使用 card hierarchy。

| Family | Tool names | 阶段 | 状态 | 验收重点 |
| --- | --- | --- | --- | --- |
| 文件与代码（5） | `read`, `glob`, `grep`, `lsp`, `apply_patch` | R4/R6/R10 | implemented | Runtime-owned lazy LSP process、workspace confinement、replacement server table、9 query ops、单文件 mutation 新诊断差分、1-based presentation 与 shutdown join 已接通；文件/diff material 沿用既有 typed projection，待最终 server/query/UI 门禁 |
| 命令（3） | `shell`, `read_shell_output`, `stop_shell` | R4/R10 | implemented | 单一 Runtime-owned background job lifecycle 支持 session-bound identity、auto-background、incremental bounded output、event-first wait、stop、终态回收与 Close join；存活 ID 进入下一 root Run live context，三工具继续经过 Plan/Hook/approval，待最终门禁 |
| 网络（3） | `web_search`, `web_fetch`, `http_request` | R10 | implemented | Jina/Tavily 与 explicit host/method allowlist 只在配置存在时渐进暴露，统一 network safety/approval、bounded provider result；待最终 live/preview 门禁 |
| Skill（4） | `list_skills`, `load_skill`, `read_skill_resource`, `propose_skill` | R7 | implemented | progressive tools 与 Desktop discovery 共用 archive-aware Lyra source；resource confined；root-only proposal 只进 review queue，正文与管理 UI owner 分离；待最终统一门禁 |
| Plan（3） | `enter_plan_mode`, `set_plan`, `exit_plan_mode` | R5 | implemented | root-only、durable Session read-only policy、dynamic effect gate、committed Plan fact、revision-bound approve/reject question；待 Desktop 与最终统一门禁 |
| Goal（3） | `create_goal`, `get_goal`, `report_goal_outcome` | R5 | implemented | 前两者常驻；outcome tool 仅 exact owned Run 可见；待最终统一门禁 |
| Schedule（3） | `list_schedules`, `create_schedule`, `delete_schedule` | R8 | implemented | root-only deferred action schema、cursor list、paired model/default workspace、write safety、same Schedule owner/event invalidation；待最终门禁 |
| 回忆与发现（4） | `search_memory`, `search_conversations`, `search_tools`, `read_tool_result` | R7/R9 | implemented | `search_memory` 使用 reviewed corpus；`search_conversations` 使用 exact workspace/Session 的 bounded user-visible FTS projection并排除 reasoning/Tool material；`search_tools` 对 Run-frozen deferred manifest 做渐进暴露并跨 checkpoint 恢复；`read_tool_result` 有界读取 durable overflow；待最终统一门禁 |
| 委派与提问（2） | `delegate_task`, `ask_user` | R4 | implemented | child Run 以父 Delegate Item 为唯一 disclosure anchor；Question 为唯一交互真身；model/protocol field mapping 保持 Lyra 自有语义；待最终统一门禁 |
| MCP/unknown | dynamically discovered names | R8 | implemented | private execution始终保留 `(server, remote)`；domain 唯一生成 bounded model-visible name，完整 MCP result envelope 使用 safe JSON material；待最终门禁 |

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
- Runtime Protocol 以 `2026-08-21` 为成熟基线，89 个 dotted methods、HTTP/SSE、sidecars 与部署所需
  token/CORS 均主动继承；A2-028 仅为修复 Session 模型身份歧义提升到 `2026-08-23`
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
Contract: protocol 2026-08-23; R1 verified runtime.discover; current 89 registered shapes + 4 fixed HTTP endpoints generated diff-free
Tests: Runtime/Desktop race+vet+Staticcheck; frontend type+lint+format+7 tests+build; signed package verification
Recovery: real child SIGKILL -> successor generation; graceful close; corrupt DB/no descriptor; predecessor UI result ignored
Product: arm64 lyra-app2.app contains sibling lyra-runtime; U01 remains implemented pending R10 visual/native matrix
Resources: controlled Runtime/Vite/agent-browser/Chrome/ports/temp roots released; unrelated user processes preserved
Decision: current Lyra contract preserved; no Codex protocol surface copied
```
