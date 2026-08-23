# app2 重写执行计划

> 当前阶段：R1 已完成，正在一次性实现 R2–R11 的 app2 Runtime/Desktop。按用户要求，全部代码写完后只做
> 一轮统一测试、修复与 package；每个实现批次提交并推送，但未经过最终门禁前不冒充 `verified`。

## 1. 授权与边界

本计划基于用户的明确授权：

- 绿地重写，允许 breaking change，完全不用兼容旧写法；
- 保留原本全部功能并彻底清除坏味道；
- 使用整洁架构与 DDD 思想，代码必须优雅；
- 前端重点参考桌面 study/codex，服务端重点参考 Codex；
- 吸收 Dougong/Oolong 的 AGENTS/CLAUDE 标准并形成 app2 自身标准；
- 第一轮先产出文档，后续用文档控制节奏；
- 每轮释放 agent-browser 等外部资源；
- 当前目标止于 app2 Runtime/Desktop 全功能与独立交付；旧 app、CLI、入口切换与删除明确不在本轮授权内。

旧 app 在迁移期默认只读。除非发现会妨碍行为对照的严重环境问题，不对旧实现继续做功能修补。

## 2. 总体路线

| 阶段 | 结果 | 当前状态 |
| --- | --- | --- |
| R0 | 参考审计、标准、领域、架构、合同、ADR、能力账本、执行计划 | complete（本文档批） |
| R1 | app2 工程骨架、Lyra 合同生成、Runtime HTTP/SSE host、Wails supervisor/bootstrap、`runtime.discover` | complete |
| R2 | Workspace + Session 首个完整纵切、snapshot hydration、Work Index、基础 shell | in progress |
| R3 | root Run + Item stream + Composer + Agent Narrative + durable cold restore | pending |
| R4 | Tool/Approval/Question/delegated Run/cancel/steer/checkpoint/recovery | pending |
| R5 | Plan + Goal 完整生命周期与 UI | in progress（Runtime 与 Desktop production 已实现，待最终统一门禁） |
| R6 | Files/Diff/Git/Search/Index/Context Dock/Terminal/Timeline | pending |
| R7 | Skills/Recipes/AgentDocs/Knowledge/Memory/Hooks/Tool catalog | pending |
| R8 | Provider/Model/MCP/Approval policy/Schedule/Settings/Runtime topics | pending |
| R9 | Session fork/rollback/import/export、history search、usage、feedback | pending |
| R10 | Remote HTTP(S) 加固、全量内容渲染、主题/i18n、视觉/性能/无障碍收口 | pending |
| R11 | Runtime/Desktop 89/3/16 全量 parity、统一门禁与独立 package；旧 app 不动 | pending |
| R12 | 迁移 CLI/其余 consumer（deferred，本轮不实施） | deferred |
| R13 | 切换与删除旧 app（deferred，本轮不实施） | deferred |

阶段编号是能力依赖顺序，不是大批量分层开发。每个阶段内部仍按小纵切交付。

## 3. 每轮固定工作法

实现阶段按依赖选择 `CAPABILITY_LEDGER.md` 中边界清晰的场景批次：

1. **选场景**：写清用户意图、durable outcome、UI projection、失败和恢复路径；
2. **核旧证据**：在隔离临时 HOME 中运行/阅读旧 app，记录语义，不复制实现；
3. **记录最终例证**：明确最终统一门禁需要覆盖的 domain/SQLite/transport/Desktop scenario；实现期不运行批次测试；
4. **实现纵切**：domain → flow → store/adapter → protocol → Desktop context → UI；
5. **删脚手架**：清除临时 adapter、重复 DTO、fallback、TODO、未用 export；
6. **更新 owner 文档**：只更新语义/架构/合同/ledger/plan 中真正变化的 owner；
7. **释放资源**：关闭进程、浏览器、watcher、端口和临时数据，并验证为零；
8. **提交推送**：每个批次提交到当前 `codex/` 分支并推送；ledger 最多升到 `implemented`；
9. **最终统一门禁**：全部生产代码完成后一次执行全矩阵、集中修复，再升为 `verified`。

同一轮不得横向先建多个 context 的空目录，也不得以“后面再接 UI/恢复”宣称纵切完成。

## 4. R0：设计冻结

### 目标

让 R1 无需重新讨论基础拓扑、术语、owner、协议原则和完成门。

### 已完成产物

- `app2/AGENTS.md` 与 `CLAUDE.md`：吸收并发展 Dougong/Oolong 标准；
- `REFERENCE_STUDY.md`：当前系统/Codex/标准证据与采用/拒绝；
- `DOMAIN_MODEL.md`：统一语言、contexts、aggregates、invariants；
- `ARCHITECTURE.md`：Runtime/Desktop/frontend 拓扑、依赖、状态和资源 owner；
- `CONTRACT.md`：继承 Lyra protocol、重建 artifact/schema 的合同策略；
- `DECISIONS.md`：23 条 accepted ADR；
- `CAPABILITY_LEDGER.md`：89 operations、3 probes、16 topics、7 run events、24 UI groups、30 tools。

### 退出门

- 文档链接和数量机械校验；
- 源参考路径存在，Oolong `CLAUDE.md` 缺席被诚实记录；
- 所有旧 operations 精确出现一次，总数 89；
- architecture/domain/contract/ledger 没有互相复制 shape owner；
- `git diff` 只包含 app2 文档，不覆盖 `.opencode/` 等用户内容；
- 本轮未启动 agent-browser、浏览器、Wails、Vite、Runtime 或 watcher，无资源待回收。

## 5. R1：骨架与进程/合同纵切

### 首个可运行场景

```text
启动 Wails Desktop
  -> supervisor 启动 app2 Runtime
  -> Runtime 持有 OS-selected loopback listener，原子发布 one-shot descriptor
  -> 校验 /v2/info、live、ready 指向同一 instanceId
  -> renderer 获得只读 endpoint/token bootstrap
  -> generated Lyra client 调用 runtime.discover
  -> 关闭窗口
  -> request gate drain、HTTP/SSE listener 关闭、Runtime child exit、无残留进程/端口/task
```

### 产物

- `app2/runtime` 与 `app2/desktop` 独立 Go modules，同一 Go 版本；
- canonical registry/generator 基础设施与首个 `runtime.discover` operation；当前 89-operation manifest 是只读
  迁移 oracle，每个后续纵切把自己的 concrete operation 移入 app2 registry，不创建 placeholder handler；
- strict JSON-RPC/HTTP admission、unary JSON、SSE framing/`Last-Event-Id`/write deadline；R1 transport 按
  `Stream.Next` pull，每连接最多一个 frame 在途，真实 subscriber queue 随首个 stream producer 建立并强制其 published bound；
- exact `RequestMeta` validation、`runtime.discover`、幂等 header 与三个 typed sidecars；
- Runtime resource object、task group、SQLite epoch 1 metadata/open/close；
- Wails v3 `NativeHost` 与 `DesktopHost` 精确绑定面；
- Cobra factory + fresh Viper -> typed Runtime Config；business packages 不依赖 Cobra/Viper；
- one-shot bootstrap descriptor、supervisor spawn/probe/identity check/stderr/restart/shutdown；
- generated TS client/validator 的 `runtime.discover` 直连纵切；
- OTel trace propagation 和安全 structured logging 基础。

### R1 dependency budget

- 两个 Go module 均从当前 Go `1.26.5` 起步；Desktop 只使用 Wails v3，不混入 v2 API；
- `lyra-runtime serve` 使用 Cobra factory 和命令私有 fresh Viper，只负责参数/config/env 到 typed Config 的映射；
- 四个固定 HTTP endpoints 使用标准库 `net/http` 路由，不为 route parameter 引入 router；
- 继续使用已验证的 `modelcontextprotocol/go-sdk/jsonrpc` envelope 与 `Tangerg/sse` writer，保留当前严格
  duplicate/shape validation；不再引入另一套 JSON-RPC/framing library；
- CORS middleware、SQLite driver 与 OTel SDK 只有在对应 production path 和 teardown test 同批落地时进入
  module；每个新增依赖必须说明它删除了哪段更高风险的自研机制。

### 必须先证明的失败

- malformed/mismatched RequestMeta、duplicate JSON member、unknown envelope field、non-string id、oversized body；
- bad content type、body limit、SSE write timeout/disconnect、shutdown 时 late request；
- probe identity mismatch/timeout、unexpected child exit、quit 不重启、stale generation callback、Close 幂等；
- descriptor symlink/foreign path、nonce/PID mismatch、partial write、stale descriptor、token path escape；
- schema mismatch/corruption 不发布 ready。

### 退出门

- generator diff-free；Go test/race/vet/staticcheck/build；TS type/lint/test/build；Wails package smoke；
- graceful close 和 forced child exit 均无残留 PID/port/temp home；
- O01a、P01–P03、U01–U02 至少达到 implemented，R1 涉及部分有真实 smoke 才 verified。

### R1 完成证据（2026-08-23）

- **Implementation**：`app2/runtime` 拥有 protocol/discovery/operation/dispatch/rpcwire/httptransport/
  localruntime/sqlite/runtimehost/cli/contractgen；`app2/desktop` 拥有 supervisor、两个精确 Wails host、
  generated-client frontend 和最小 production package task。
- **Contract**：R1 门禁只验证当时已迁移的 `runtime.discover`；当前 canonical Go registry 的 89 个 method shape
  已在后续批次完整投影为 manifest、TS wire validator/client，并保持可重复生成。注册 shape 本身不冒充对应能力
  已迁移或 verified。
- **Tests**：两模块 `go test -race ./...`、`go vet ./...`、固定上游提交的 Staticcheck 全绿；前端
  type/lint/format/7 tests/build 全绿；W3C trace extraction 与 secret-free structured log 有边界测试。
- **Recovery**：真实 runtime binary 的 descriptor/token/probes/discover/close smoke 通过；supervisor 真实
  `SIGKILL` 后产生不同 PID/instance/generation，并删除 predecessor root；renderer 的第 1 代迟到 discovery
  不能覆盖第 2 代投影。
- **Product**：Wails v3 beta.12 host 编译；`wails3 package` 产出 ad-hoc signed arm64 `.app`，bundle 内主程序与
  `lyra-runtime` 并列，plist 与签名校验通过。U01 的全量 Retina/WebKit/原生交互视觉验收仍归 R10。
- **Resources**：Runtime、Vite、R1 agent-browser session、受控 Chrome、5174/54205 监听和两个 exact QA
  temp roots 均已关闭或移入废纸篓；未触碰用户已有的其他开发进程与 `.opencode/`。
- **Decision**：Lyra Protocol `2026-08-21` 未改名、未引入 Codex handshake/stdio/WebSocket；Codex 只影响
  one-shot bootstrap、bounded shutdown、generation identity 和 fail-closed supervision 机制。

## 6. R2：Workspace + Session + Work Index

### 纵切顺序

1. resolve Workspace + create Session；
2. list/read/update revision；
3. `sessions.snapshot` consistent empty closure，并建立无丢失的 `runs.subscribe`/`runtime.subscribe` handoff；
4. delete cascade；
5. Desktop Work Index/new Session/native directory picker；
6. renderer replacement 重建 active Session。

### 核心门

- no active Project；exact cwd、missing workspace、default workspace 不冒充 Session；
- concurrent create/update/delete、revision conflict、多 Runtime shared DB winner；
- Work Index 只消费一个 projection，不现场 join query；
- new Session、rename、favorite、delete、group/search 的键盘/a11y/窄窗状态；
- O02 中基础 CRUD/snapshot、O07、U03–U04 分批 verified。

### R2 production implementation 进度（2026-08-23）

- **Workspace/Session truth**：Session aggregate 只拥有 identity/title/workspace/model/favorite/revision；活动状态不落到
  Session，也不由 flow 猜测。SQLite 的 Work Index projection 在同一次读取中从唯一 open root Run 派生
  `idle | running | waiting`，`sessions.get/list/update` 返回该 committed projection。
- **Catalog ordering**：分页游标覆盖完整的 `favorite DESC, updated_at DESC, id DESC` 排序键，跨 favorite 分区不会
  跳过或重复 Session；Desktop 用生成 client 的 cursor page 增量读取，不一次 eager load 全历史。
- **Desktop Work Index**：已实现默认 workspace 与 Wails v3 原生目录选择两条 new Session 路径、exact cwd 分组、
  title/path 搜索、Session 选择、rename CAS（冲突保留 draft）、favorite、二次确认 delete、长列表继续加载、
  Arrow/Home/End 与 Cmd/Ctrl+N；行级选择和操作没有 nested button。
- **Client ownership**：mutation 先把 Runtime 返回的 committed Session 写入唯一 infinite-query cache，再精确失效；
  删除同步移除 snapshot cache 并选择可用 fallback。`sessions.changed`/`runs.changed` 回拉 catalog，Plan/Goal/Session
  topic 回拉 exact snapshot；连接 replacement 仍由 generation-scoped query key 隔离。
- **仍未关闭的 R2 门**：snapshot 与 Run stream 的无丢失 watermark handoff、reload/restart 证据随 R3 root Run 纵切
  完成；fork/history action 随 R9 完成。按集中验证约定，本批只写 production code，状态保持 `in progress`，不提前
  把 O02/U03/U04 升为 `verified`。
- **协议与资源**：继续使用 Lyra Protocol 的 Session/Workspace/Runtime topic；Codex 仅作为进程/状态所有权参考，
  未复制其协议或领域命名。本批未启动 Runtime、Wails、Vite、浏览器、agent-browser 或 watcher，无持有资源。

## 7. R3：Root Run + Narrative 首个真实 agent 纵切

### 首个场景

用户在 Composer 输入文本 → start root Run → agent stream commentary/tool-free final answer → terminal →
reload/restart 后从 SQLite 恢复完全相同的 Transcript、phase、usage/context footprint。

### 产物与门

- Run/Segment/Transcript domain 和 single-open-tree DB invariant；
- Agent Framework 仅 `agentexec` 接入；Conversation/Transcript/WorkingContext 分开；
- attempt/commit/begin、committed run events、snapshot/stream watermark handoff；
- AgentSessionView normalized fold、Composer SendIntent、Narrative work/final rows；
- exact cancel/steer 基础、items/runs pagination；
- provider failure、pre-opening failure、receipt loss、duplicate delta、notification gap、cold completed history；
- O03/O06、run events（Plan 除外）、U05/U06/U12 基础 verified。

## 8. R4：HITL、Tool、delegation 与恢复

### 纵切顺序

1. ToolCall lifecycle + typed material + shell/read/patch 基础；
2. Approval exact claim/decision/edited args/scope；
3. Question ordered fields/Skip/IME；
4. quiescent checkpoint + answer invalidation + resume Segment；
5. delegated child admission/tree/disclosure/waiting subtree；
6. cancel root/child/tool、steer、unknown effect；
7. graceful restart/SIGKILL/multi Runtime recovery。

### 核心门

- stale/foreign/duplicate Interrupt answer 永远不能提交；
- approval/question 同时是 durable Transcript facts 与唯一 UI request surface；
- active-step crash fail closed 为 lost/unknown；完整 waiting checkpoint 才 resume；
- child lineage 不猜，parent/child terminal 和 callback/reservation 原子清理；
- 所有 Tool presentation 的 identity/material/duration/result ownership；
- O03 控制面、O04、O13、U07–U09/U16 和相关 tool groups verified。

## 9. R5：Plan 与 Goal

### 纵切顺序

1. Plan resource/update/event/snapshot + compact UI；
2. Goal start/get + Composer goal mode；
3. update/pause/resume/clear + incarnation CAS；
4. autonomous continuation、completing settlement、restart/rollback/delete；
5. Plan/Goal tool presentations 去重复。

### 核心门

- Goal internal control 只进 Conversation；
- Goal 不晚于 Session，old incarnation command 被拒绝；
- Plan 一个 in-progress、无 generic state registry；
- O05/O15、plan/goals topics、plan event、U10/U11 和 Plan/Goal tools verified。

### 当前实现记录（尚未统一验证）

- Plan 已从共管 Plan/Interrupt 的 `stateflow` 拆为私有聚合、CAS store 与独立 `planflow.Service`；Interrupt
  冷读也回到独立 owner，不再保留 generic state package；
- `set_plan` 整体替换有序步骤并提交单调 revision；`plan.changed` 只通知冷读失效，成功工具调用把同一份
  committed Plan 投影为该 Segment 的 authoritative/replayable `plan.updated`；
- `enter_plan_mode` 建立 Session-scoped durable read-only policy，动态阻止 write/exec/network；
  `exit_plan_mode` 只在用户批准 exact stored Plan 后解除，不改变 Lyra 全局 approval policy；
- Plan/Goal tools 只进入 root Run catalog；root Run terminal commit 原子捕获 Plan boundary，fork 复制被选边界及
  已知历史边界，rollback 以新 revision 恢复 known boundary；imported Run 的缺席 boundary 保持 unknown，不猜空；
- Session snapshot/fork/rollback/import/export 都消费 Plan 聚合；delete/rollback/import 清理 Plan mode，fork 不继承；
- Goal 已从 `stateflow` DTO CRUD 拆为私有聚合、CAS store、`goalflow.Service` 与单 owner Driver；
- fresh autonomous control 仅进入 exact Conversation journal，不伪装成用户 Transcript Item；
- Goal Run 在执行启动前认领 exact `incarnationId + revision + runId`，旧 incarnation 无权提交 outcome；
- restart 先由 runflow 把 predecessor execution 收敛为 `lost`，再把仍 active 的 Goal 显式暂停，绝不暗中续跑；
- `create_goal`、`get_goal` 常驻，`report_goal_outcome` 只对 exact owned Run 可见；
- Plan/Goal 均保留现有 Lyra wire；Codex 只提供机制研究，不提供协议或产品语义；
- generated Lyra client 从同一 operation registry 投影 89 个 method 的 unary/stream、nullable、幂等与 SSE event
  边界；Desktop 不维护手写 method shape，也不把 stream 当 unary RPC；
- Desktop 通过 `sessions.list` 选择 exact Session、由 `sessions.snapshot` 同源读取 Plan/Goal，并消费
  `runtime.subscribe` 的 `sessions/plan/goals.changed` 做精确失效与有界重连；
- Plan compact projection 直接计算 canonical checklist 的 ring、N/M 与当前步骤，完整列表仅用 hover/focus
  disclosure；Goal composer/tray 覆盖 budget、start/update/pause/resume/two-step clear，且远端 objective 变化不覆盖
  本地 dirty draft；
- 本记录只升到 implemented；最终 R11 统一门禁通过后才可标记 verified。

## 10. R6：Workspace 与 Context Dock

### 纵切顺序

1. file list/read/head/search；
2. git changes/diff rows/raw；
3. Context Dock per-Session state与 narrow capability；
4. Review Workspace、file navigation、syntax material；
5. terminal/tool detail/timeline/run summary；
6. codebase status/reindex/search；
7. file watch/resync。

### 核心门

- path traversal/symlink/scope、binary/large/encoding/pagination；
- file status 三套词汇不互相 cast；
- dock close/reopen/switch/reload 不复活 stale Tool/scroll；
- 640px reading floor、light/review density、全文件 diff；
- O08/O16、files/codebase topics、U13–U16 verified。

## 11. R7：能力资源

按真实 UI/agent consumer 顺序完成：Skills → Recipes/AgentDocs → Knowledge → AgentMemory → Hooks → Tool catalog。

每个资源必须同批拥有：domain language、file/store owner、query/mutation、watch/invalidation、settings/dock UI、
empty/error/unavailable states、Run injection boundary 和资源清理。

完成 O09/O10/O12/O19/O21/O22，skills/knowledge/hooks/agentMemory topics，U20 与 Skill/回忆 tools。

## 12. R8：Integration 与 Settings

### 顺序

1. Provider secret/config/test；
2. Model catalog/roles/picker/context window；
3. MCP closed transport config/connect/reconnect/tool/auth attempt；
4. Approval global policy/rules；
5. Schedules + runNow；
6. Runtime resource topic subscription 全覆盖；
7. Settings navigation/forms/query invalidation。

### 核心门

- secret never read back/logged，scope change 必须显式选择；
- provider/model 显式配对；MCP original/model-visible name 不混；
- generation replacement 不接受旧 connect/test/auth result；
- form draft 不泄漏 wire，save/test/validation 可独立测试；
- O11/O13/O14/O17/O18，mcp/schedules/models/approvals topics，U21 verified。

## 13. R9：高级 Session 与运营能力

- fork boundary、rollback history/files/both；
- app2 JSON/Markdown export、strict atomic import；
- conversation/session search、long history pagination；
- usage session/summary 与 cost unknown；
- feedback create；
- archive/favorite 等最终 Session lifecycle。

必须用大历史、corrupt artifact、identity conflict、partial file failure、restart 和 dropped-run cleanup 验证。
完成 O02 剩余、O20/O23、U19。

## 14. R10：Remote、内容与产品打磨

### Remote HTTP(S)

- 复用 `/v2/rpc` JSON/SSE 与 `/v2/info`、live、ready，不建立第二套 method/stream 语义；
- TLS、auth、Origin、remote identity 与 secret rotation；
- slow-client queue、network offline/online、bounded backoff、manual stop；
- local/remote/embedded operation conformance suite。

### 内容与视觉

- Markdown basic HTML allowlist、code/copy/wrap/Shiki、tables、KaTeX、Mermaid、images/lightbox/native save；
- static themes/accent/light-dark、locale coverage、RTL；
- Work Index/Context Dock geometry、dialogs/scrim/surface ladder、40px hit targets；
- streaming follow/reader escape/virtualization/bundle/lazy loading；
- keyboard/IME/CJK/reduced motion/WCAG/Retina/WebKit/Wails packaged app。

完成 U17/U18/U22–U24、P01–P03 remote path，并对所有 Desktop surface 做 production-equivalent matrix。

## 15. R11：Wave A 全量 parity 与切换

### 准入

- 89 operations、3 probes、16 topics、7 run events、24 UI groups、30 tools 全部 `verified`；
- app2 没有旧 compatibility reader/alias/fallback；
- architecture/contract/consumer/leak gates 全绿；
- no unresolved known smell 或“切换后处理”TODO。

### 黑盒场景矩阵

旧 app 与 app2 使用隔离临时 HOME 运行同一用户场景，比较规范化结果：

- Session CRUD/fork/rollback/import/export；
- normal Run/HITL/delegation/tool/Goal/Plan；
- workspace/file/git/index/resources/settings；
- renderer reload、transport close、graceful restart、SIGKILL、多 Runtime；
- long history、large tool result、large diff、missing workspace、provider/MCP failure；
- Desktop light/dark/narrow/keyboard/IME/a11y/package。

差异不自动判失败：每项裁决为 app2 defect、旧 bug 或 intentional breaking，并链接 ADR/ledger。

### 本轮交付边界

1. fresh checkout 跑全部 gates 和 package/codesign；
2. 产出可独立运行的 app2 Runtime/Desktop 与验收证据；
3. 保留旧 `app/runtime`、`app/desktop`、`app/cli` 和当前产品入口，不做切换或删除。

## 16. R12–R13：明确延期

### R12（本轮不实施）

盘点并迁移 `app/cli` 与 `app/` 下其余真实入口：

- 直接调用 app2 public resource API 或 app2 protocol client；
- 若保留 Cobra/Viper：command 由 factory 构造，使用 `RunE` 与 `cmd.OutOrStdout/ErrOrStderr`，每次测试创建
  fresh Viper；business use case 只接收 typed Config，不 import Cobra/Viper，也不使用 package-level command/flag；
- 不建立旧到新 adapter；
- 每个 consumer 走同样的 capability/contract/recovery tests；
- 删除已无 consumer 的旧公共 module surface。

### R13（本轮不实施）

- `app/` 不再包含任何生产实现或引用；
- old modules、docs、contract artifacts、scripts、fixtures、dependencies 全部证据化删除；
- full repo test/vet/staticcheck/build/frontend/package/license/security/deadcode gates；
- clean checkout product smoke；
- 资源归零与 worktree 审计；
- capability ledger 全部 `cutover` 后，persistent goal 才能标记 complete。

## 17. 测试门禁矩阵

| 变化 | 最低门禁 |
| --- | --- |
| Domain only | unit + fuzz/property for invariants + architecture imports |
| Application transaction | unit + real SQLite + race + fault before/after commit |
| Protocol | generator diff + schema/OpenRPC/TS validator + malformed/unknown/limits |
| Transport/supervisor | concurrency/race + overload + close/reconnect + stale generation + leak audit |
| Agent execution/HITL | fake deterministic engine + real adapter contract + restart/SIGKILL |
| Workspace/file/git | temp repo/filesystem + traversal/symlink/encoding/large/binary |
| React context | unit + interaction + async leak + public boundary + render count where relevant |
| Visual/layout | production fixture + light/dark/narrow/Retina/WebKit + a11y/keyboard/IME |
| Wails/native | Go test/vet/build + Wails production package + launch/native dialog/chrome smoke |
| Cutover | all above + isolated old/app2 semantic matrix + consumer/deadcode scan |

实现批次不运行局部门禁；全部生产代码完成后，在 R11 统一执行完整矩阵并集中修复。

## 18. 数据与环境隔离

- app2 默认 home 与旧 app 不同；测试每次 `mktemp -d`，路径直接传 Config，不复用 `$HOME`；
- old/app2 semantic oracle 使用两个不同 temp home、database、socket/connection；
- test binary 不扫描用户真实 skill/plugin/config/credential，除非场景显式注入；
- fixture 不写桌面工作目录或 repo 外用户文件；
- destructive cleanup 只作用于本轮记录并验证过的 exact temp path/PID。

## 19. 每轮资源回收清单

每轮结束按实际使用项执行并记录：

- agent-browser/browser：关闭 session/context，确认没有遗留受控 Chrome/Electron；
- Playwright：关闭 browser/context/page，确认无 worker/webserver；
- Runtime/Wails/Vite：先 graceful stop，超时后只终止记录的 exact PID，等待退出；
- MCP/LSP/file watcher/background shell：调用 owner disposer 并 join；
- HTTP/SSE listener/remote tunnel：关闭 listener/stream，确认端口/socket 不再占用；
- temp HOME/SQLite/WAL/SHM/log/screenshot/build cache：仅删除本轮 exact temp root；
- 最终用进程表、端口、socket 和 temp root existence 检查证明归零。

本 R0 只使用只读 shell 与文件 patch，没有启动 agent-browser、浏览器、Runtime、Wails、Vite、Playwright、
MCP/LSP 或持久 watcher；因此没有外部运行资源需要释放。

## 20. 节奏防偏规则

- ledger 未登记的功能不能直接实施；先补语义、owner、phase、acceptance；
- 实现发现 ADR 错误时先追加 superseding ADR，再改代码；不在代码里悄悄偏航；
- 一个阶段连续出现三次同类 adapter/DTO/owner 时暂停，判断是否缺少正确 domain abstraction；
- 一个抽象只有一个实现和一个调用点时默认内联，除非它承担明确边界或测试 seam；
- 每阶段做一次 evidence-backed entropy audit，删除无 producer/consumer/发布义务的新增表面；
- 旧 app 新增的能力必须先登记到 ledger，再决定进入当前阶段或后续阶段；不能静默漏掉；
- 当前 goal 不因某一实现批次、预算或时间而结束；只有 R11 的 app2 Runtime/Desktop 全量统一门禁与独立
  package 完成才结束。R12/R13 是后续目标，不能在本轮提前实施。
