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
| R4 | Tool/Approval/Question/delegated Run/cancel/steer/checkpoint/recovery | in progress（Runtime 与 Desktop production 已实现，待最终统一门禁） |
| R5 | Plan + Goal 完整生命周期与 UI | in progress（Runtime 与 Desktop production 已实现，待最终统一门禁） |
| R6 | Files/Diff/Git/Search/Index/Context Dock/Terminal/Timeline | in progress（Files/Git/Review/Codebase production 已实现，待其余纵切与最终统一门禁） |
| R7 | Skills/Recipes/AgentDocs/Knowledge/Memory/Hooks/Tool catalog | implemented（真实 PreCompact producer 已由 R10 compaction 纵切补齐，整体待最终统一门禁） |
| R8 | Provider/Model/MCP/Approval policy/Schedule/Settings/Runtime topics | implemented（production 纵切与全 topic/resync 已闭环，最终统一门禁留到 R11） |
| R9 | Session fork/rollback/import/export、history search、usage、feedback | implemented（production 纵切已闭环，最终统一门禁留到 R11） |
| R10 | Remote HTTP(S) 加固、全量内容渲染、主题/i18n、视觉/性能/无障碍收口 | in progress（R10a remote/tool/lifecycle、R10b content/media、R10c appearance/remote control production 已实现，locale 与 product 收口继续） |
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

- **Workspace/Session truth**：Session aggregate 只拥有 identity/title/workspace/provider+model/favorite/revision；活动状态不落到
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

### R3 production implementation 进度（2026-08-23）

- **Durable/live handoff**：`runs.subscribe` 先挂 live subscriber、再读取 durable replay，并以 `eventId` 去重合并；
  replay 与 live 之间不再有通知窗口，iterator 退出会释放订阅。
- **Root lifecycle publication**：Run admission、finish、cancel、wait、resume、recovery 和 steer 的 committed transition
  已发布 `runs.changed`/`sessions.changed`；Desktop Work Index 与 mounted snapshot 可按 source owner 回拉。
- **Interrupt invalidation**：waiting set 的 open、resume consumption 与 waiting cancel 只在各自事务提交后发布
  `interrupts.changed`，并携带 exact Session/Run identity；runtime event 仍只是 query invalidation，不复制答案状态。
- **Long history reads**：`items.list` 已按 Session 或 Run subtree 做 ASC/DESC keyset pagination，cursor 绑定完整 query
  identity，并返回本页引用 Run 的祖先闭包；Run cursor 同样拒绝跨 query 复用。
- **Model streaming projection**：Agent Framework delta 只在 `agentexec` 内解析为 provider-neutral append；Run projection
  以 Framework Effect identity 派生稳定 Lyra Item key，先原子提交可回放 `item.started` anchor，再发布非权威、
  非回放 `item.delta`。最终 `item.completed` 使用同一 key 并写入 durable Transcript，delta 永不成为内容真相。
- **Progress/context footprint**：每次 settled model call 产生 bounded `segment.progress`，其 usage/step 从 Run 的 durable
  baseline 累加；最后一次已知 input token footprint 写入 `RunRef.contextTokens`，waiting/terminal/reload 不退回
  ephemeral preview。unknown usage 保持缺席，不伪造为零。
- **AgentSessionView owner**：Desktop 用 `runsById/itemsById` normalized fold 合并 coherent snapshot、完整 replay 与
  live event；重叠 stream 以 `eventId` bounded dedupe，connection/session generation fencing 会终止旧 lease，
  authoritative completion 替换 provisional，terminal 清理未完成的纯渲染 anchor。
- **Composer SendIntent**：每 Session 独立保存 draft/attachment/history；支持 image/text file picker、paste、IME-safe
  Enter、成功后清空与失败保留。start/steer retry 复用同一 idempotency key，running Run 可 steer/stop，mutation 与
  stream 均 single-flight 且切换 Session 会 abort 旧 owner。
- **Agent Narrative**：root user/commentary/reasoning/final/tool/question/compaction 已有 source-aware presentation；
  commentary work wave 与 final answer card 分离，stream/cold 共用同一 Item renderer，follow lock 尊重用户向上阅读。
- **Context gauge**：Desktop 从 AgentSessionView 的 live/cold context footprint 与 provider-scoped `models.list`
  合成占用比例；model window 未知时只显示 token count，绝不把未知容量画成 0%。`models.changed` 只失效 catalog query。
- **Bounds/resource ownership**：Framework、agentexec 和 Run projector 均为 bounded queue；provider 不等待 SQLite 或
  renderer，projector 在 Executor 返回后先 drain/close，再进入 terminal commit。本批未启动 Runtime、Wails、Vite、
  browser/agent-browser 或 watcher，无待释放的外部资源。
- **仍未关闭的门**：reload/restart、长历史接续与故障矩阵仍待 production implementation；
  delegated disclosure 属于 R4。按集中验证约定，本阶段不运行分批测试，
  不把 R3 提前标为 verified。

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

### 当前实现记录（尚未统一验证）

- Runtime 的 waiting open、resume consumption 与 waiting cancel 均在 durable transaction 提交后发布
  `interrupts.changed`；事件只携带 exact identity，PendingInterruptSet 仍由 snapshot/query owner 提供；
- Desktop 只有在 approval/question 的完整答复面可用后才声明这两种 `interruptTypes`，并订阅
  `interrupts.changed` 做 Session snapshot 精确失效；
- AgentSessionView 以 `rootRunId` 归一化 cold/live PendingInterruptSet；`segment.finished(interrupt)` 可立即建立
  committed live projection，`segment.started` 或 authoritative snapshot 清除已消费集合；
- `runs.resume` 与 start 共用 generation-fenced stream lease、replay cursor 和 receipt-loss retry；同一
  idempotency key 固定第一次提交的 exact full response set，不能被后续 UI draft 改写；
- Narrative 内只有一个整组 HITL action surface：approval 支持 approve/deny、one-shot edited JSON args、reason 与
  rememberable scope；question 按字段顺序支持 text、single/multi choice 和 custom answer，IME composition 不触发误提交；
- 当前 Lyra Protocol 没有 skip response，因此 Desktop 不发明无法被 Runtime 原子校验或持久化的 Skip 语义；
- **Tool live lifecycle**：agentexec 的 bounded live observation boundary 同时承载 model 与 Tool semantic facts；
  普通 ToolCall 的 running Item 与 `item.started` 原子提交，settlement 把 terminal Item、可选 offloaded result、
  `item.completed` 和 committed Plan projection 归入同一 transaction。Segment settled Output 仍是 drop/failure fallback；
- **Stable Item union**：`ask_user` / `exit_plan_mode` 等 intrinsic-input capability 不发布 provisional ToolCall，等 typed
  input request 存在后直接建立 Question，因此同一 Item identity 永不从 ToolCall 变型为 Question；approval gate 则复用
  已 durable 的 running ToolCall，并在 wait transaction 中只更新内容、不重复 `item.started`；
- **Tool disclosure adapter**：Desktop 继续消费唯一的通用 `ToolInvocation`，仅在 presentation 层为 file/search/patch/
  shell/network/Plan/Goal/MCP 建立可读标题与 subject；status、duration、safety、approval、stdout/stderr/exit、line range、
  error 和 raw arguments 都由 canonical Item material 渲染，未知工具安全回退而不扩展 Lyra wire；
- **Child admission foundation**：delegated child 采用独立 `delegation.Admission` 聚合承接 Framework 的 prospective
  Process identity 与 Lyra Run identity；SQLite 只在 private pending reservation 中保存映射，重复 admission 必须完全一致，
  且 pending 永远不是 wire Run。父级 running `delegate_task` Item 与 reservation 同事务；只有 started outcome 会在同一
  transaction 里建立 child Run + `segment.started`，aborted outcome 则原子结算父级 Item 而不伪造 child。root profile
  继续治理整棵树，child RunRef 不复制 `protocolProfile`；
- **Managed Delegate family**：agentexec 已建立固定深度、固定 fan-out/active/tree/process budget 的
  bottom-up Deployment family；`delegate_task` 是 Framework managed Delegate，不是一个同步 Tool。ProcessAdmitter 与
  start-outcome acknowledger 只把 opaque Framework identity 交给上述 application port，Run/Segment identity 仍由 Lyra
  分配。child Tool 通过当前 invocation 动态解析真正的 Run scope，placeholder manifest 只冻结 definition/capability，
  绝不执行；root 未协商 `subagents` 时仍走原本单 Process deployment；
- **Source-owned child settlement**：model/tool/delta/progress observation 在 agentexec 边界即绑定实际 child
  Run/Segment，不再用 root identity 二次猜测。Framework tree terminal material 按 depth-descending 收集，runflow 验证
  每份 material 的 source 与 lineage；child transcript/metrics/context/terminal event 和父级 `delegate_task` terminal Item
  在同一 SQLite transaction 结算，后代失败时 fail-fast，祖先不能提前完成。child provider conversation 从投影策略入口
  排除，不会污染 root Session Conversation；
- **Root tree stream**：durable journal 同时保存 source Run/Segment 与 carrying root Run/root Segment；SQLite commit
  sequence 给整棵树一个 replay 顺序，attach-first handoff 以 eventId 去重。Hub 按 root scope fan-out，child
  `segment.finished` 不再关闭 Runtime iterator 或 Desktop lease，只有 root Segment terminal 才结束流；wire shape 与 7 种
  Lyra event 均未改变。该内部修正提升 app2 SQLite schema epoch 到 6，不承担旧 app2 开发库兼容；
- **Atomic tree wait**：Framework tree 在 external input 出现后先把所有 running member 停到 safe boundary，再捕获
  only-waiting/paused/terminal 的完整 opaque checkpoint；terminal descendants 先按既有 parent/child transaction 结算，
  其余 source Run 按 depth-descending 投影各自 transcript/metrics/context，并以 `interrupt | suspended` 结束当前 Segment。
  SQLite 用一笔 transaction CAS 整棵 open tree、写入 child-before-root event order，并只在 root 保存完整
  `PendingInterruptSet` 与 checkpoint；任一遗漏 member 都拒绝提交；
- **Exact tree resume**：`runs.resume` 仍是唯一外部 continuation 命令；application 从 durable open tree 与 private
  delegate admission 重建 postorder member binding，为每个 waiting/suspended Run 原子开启各自的新 Segment，整组消费
  InterruptSet/checkpoint，并把 answer 按 source Run 精确交回 restored Framework Process。paused sibling 原位恢复，
  terminal snapshot 不再生成第二份 child settlement；root stream 继续承载整棵 resumed tree。可选 resume input 在只有一个
  interrupted branch 时与该 response 同一 Framework safe boundary 接受，多 branch 时 fail closed 而不猜投递目标；
- **Exact cancel**：running root/child cancel 不再由 API 直接写伪造 terminal。runflow 将意图送入 active root owner，
  agentexec 以 Run→Process binding 对 root 或 exact child subtree 调用 Framework cancellation；child subtree terminal material
  按 depth-descending 回交既有 parent/child transaction，root cancellation 则由正常 tree settlement 收口。RPC 等待 durable
  terminal 后返回；child response 同时携带同一 command boundary 读取的 root Run。single-process waiting cancel 现在显式
  打开并关闭一个新 Segment。delegated waiting child cancel 由 Framework 对完整 opaque checkpoint 计算 exact subtree replacement，
  Lyra 再按 descendant-first/root-last 原子开启新代际并终结目标子树，survivor 以 replacement checkpoint 自动续跑；整棵 waiting
  tree 的 root cancel 同样在一个事务内终结全部开放成员并消费 interrupt/checkpoint。Framework 当前不能安全变换的 paused child
  继续 fail closed，不以产品层猜测状态；
- **Provider Conversation closure**：Lyra Transcript 继续保存产品语义，provider Conversation 则由独立投影器按模型调用顺序
  闭合 assistant ToolCall/ToolResult。普通 Tool、managed Delegate、恢复后的旧 waiting call 走同一条投影路径；direct child
  settlement 在对应 assistant call 已持久化时与 parent Item/result 同事务追加，segment terminal/tree wait 则把本代 observation
  与 durable Delegate Item 合并后统一收口。single/delegated waiting cancel 同时把开放 Item 标记 incomplete 并持久化 error result，
  fresh Run 不再继承悬空 provider ToolCall；该修正没有扩展或复制 Lyra wire；
- **Atomic tree recovery**：Runtime 启动恢复与 launch-failure settlement 都先按 root 聚合完整 running tree，验证每个 parent
  仍在同一开放 lineage，再以 descendant-first/root-last 的一笔 transaction 收敛为 `lost`。每个 source Segment 保留自己的
  ordinal/metrics，事件统一挂到 root Segment stream；开放 Tool/Question/流式占位按各自 Item union 规则变为 incomplete，child
  对应的 parent Delegate Item、provider error result、interrupt/checkpoint 清理和 root Plan boundary 同步提交。waiting tree 不进入
  此路径，仍保留完整 checkpoint 可恢复；
- **Desktop nested disclosure**：Session snapshot 在明确协商 `subagents` 后读取完整 descendants；Agent Narrative 只把 root
  material 放在主时间线，child/grandchild 由 `spawnedByItemId` 锚定父 `delegate_task` 并递归披露。sibling 保留 authoritative
  顺序，status/model/exact Run identity/terminal detail 均来自 RunRef；缺失父锚点时显示完整性告警，不猜 lineage 或静默丢弃；
- **Tree control ownership**：Desktop 只为 active root Segment 建立 stream lease，因为 root stream 已承载整棵树事件；running
  与 waiting 的 child 均以 exact Run ID 发起 cancel，操作错误回到对应 disclosure。root waiting 也可从 Composer 明确停止；
- **Capability gate opened**：Runtime discovery 与 Desktop client 现在共同开放并请求 Lyra `subagents` feature；profile 仍在 root
  Run admission 时冻结，未 opt-in 的 client 仍不能创建或直接寻址 child。该能力只复用 Lyra 现有 Run/Item/Segment/feature
  语义，没有照搬 Codex 线程或协议；
- 本批未启动 Runtime、Wails、Vite、browser/agent-browser 或 watcher，无待释放的外部资源。

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

### 当前实现记录（尚未统一验证）

- **Workspace file read model**：`workspaceflow` 继续独占 path jail 与 plain-text material；file read 改为流式扫描完整文件，
  因而 `totalLines` 不再等于预览字节内的偶然行数。UTF-8/NUL、最大单行/返回字节与 line window 都显式有界，served
  `startLine/endLine` 不再回显未实际读取的请求值；
- **Session-owned Context Dock**：Workspace/Session 是同一 Dock 的两个明确 pane。每个 Session 只持久化有界的 open file、
  selected target、expanded directory 与 search draft/query；workspace identity 改变即丢弃旧 presentation state，不把 path
  暗中带到新 workspace；
- **Files consumer**：Desktop 用 lazy per-directory cursor query 构造树，text search 只消费 canonical `GrepMatch`，点击命中
  打开包含目标行的 1000-line window。最多保留 8 个 tab、100 个展开目录和 32 个 Session 状态；大文件不会一次生成无界 DOM；
- **Repository scope**：Git owner 显式区分 repository root、Session workspace root 与 repo-relative prefix。status/numstat/diff
  只读取 exact workspace pathspec，再把 path/previousPath 投影回 workspace-relative identity；子目录 Session 不会泄漏同仓库其他目录；
- **Honest working-tree material**：worktree diff 以 `HEAD` 为基线，同时覆盖 staged、unstaged、untracked；未跟踪文本以有界
  流式扫描计算行数，binary 不伪造 `0/0`。尚无首个 commit 时直接以空树语义呈现现存文件，base mode 则明确返回 VCS
  unavailable；structured diff 只在文件边界截断，raw patch 与 rows 来自同一 material。Git stdout/stderr 都有显式硬边界，
  context cancel、超限与失败都会 wait/回收子进程，不允许 configured external diff/textconv 扩张执行面；
- **Review Workspace consumer**：Workspace pane 内的 Files/Review 是同一 Session-owned context 的两个 view。Review 使用独立
  VCS status vocabulary 展示 rename/untracked/binary 与真实增删统计；一次 bounded query 保留整组 change 的空间位置，changed-file
  navigator 可 exact scroll，每个文件可折叠并真正 unmount rows，同时保留 worktree/branch 与 unified/split。binary、empty、loading、
  error、truncated 都有明确状态，不把 `DiffRow.type` cast 成 file status；
- **Reading floor**：选中文件后 Context Dock 覆盖式扩展到 Work Index 右侧的可用宽度，最窄窗口仍保留约 640px reader，
  而不是永久挤压 Agent Narrative 或扩大应用最小宽度；Review 主动请求同一扩展面，关闭最后 tab 或切回 Session pane 即释放；
- **File watch ownership**：全局 Runtime invalidation lease 随 selected Session 注册一个 exact workspace watch；Session/Runtime
  generation 切换会 abort 旧 subscription，`files.changed`/resync 只失效对应 workspace query prefix；
- **Codebase index owner**：索引 identity 是 exact Session workspace，不再暗中扩大到 repository root。每个 workspace 只有一个
  admission owner；新 operation durable admission 后才取消前任，终态通过 operation ID CAS 提交，因此旧 generation 无法覆盖新索引。
  Runtime restart 或异常结算遗留的 `indexing` 会由 status 回读识别并收敛为可重建 error，terminal state 不保留伪 active operation；
- **Bounded semantic material**：source discovery 优先使用 Git tracked/untracked + ignore 语义，非 Git workspace 才使用有界 walk；
  path jail、symlink、UTF-8/NUL、extension、file/path/byte/chunk/line 限制都在 embedding 前完成。embedding response 的数量、finite
  vector 与跨 batch dimension 被显式验证；只有同一 embedding role identity 能搜索既有索引，documents 与 ready metadata 原子替换；
- **Codebase Desktop consumer**：Context Dock 的 Workspace view 增加 Session-owned Codebase，而不是复制另一套产品协议。UI 直接消费
  Lyra `codebase.status/reindex/search` 与 `codebase.changed`，覆盖 none/indexing/ready/error、provider/reindex/search failure、empty、
  truncated metadata 与 bounded results；点击 passage 回到 Files 并定位 exact start line，draft/submitted query 有界持久化；
- **Session activity projections**：Context Dock 的 Session pane 以 per-Session `overview/timeline/terminal/summary` 视图取代旧式并列
  小组件。Timeline 按 canonical root/delegated Run tree 投影 start/tool/approval-or-question interrupt/settlement，保留 lineage integrity
  提示与 exact active Run cancel；Terminal 只聚合 `shell` ToolCall 的 authoritative result，并在运行期消费 64K-character 有界
  `toolOutput` 展示缓存，读者离开尾部即解除 follow；Summary 只汇总最新 root tree 的 files/read/commands/approvals/errors、steps、
  active duration 与 known usage，可复制有界纯文本。三者都在 render 时从同一个 `AgentSessionView` 派生，不建立第二个 timeline store；
- **Tool detail sharing**：tool identity/subject/kind/status/duration/value formatting 收敛为一份 presentation helper，Narrative disclosure、
  Timeline 与 Summary 复用同一语义。非权威 live output 只在 Tool terminal 之前显示，item completed、segment finished、snapshot
  recovery 或 Session identity 切换都会立即清除；terminal result 始终来自 canonical Item，不用 preview 反写 durable result；
- **Protocol boundary**：本纵切不引入 Codex wire、progress shape 或兼容 adapter；Codex 仍只作为 lifecycle/recovery 机制研究样本，
  Lyra Protocol 的既有 operation、state、event 与 generated client 继续是唯一合同真源；
- 当前实现批未启动 Runtime、Wails、Vite、browser/agent-browser 或 watcher 进程；仅声明式 Runtime file-watch subscription 会在
  Desktop 运行时存在，并由 React effect cleanup 明确释放。

## 11. R7：能力资源

按真实 UI/agent consumer 顺序完成：Skills → Recipes/AgentDocs → Knowledge → AgentMemory → Hooks → Tool catalog。

每个资源必须同批拥有：domain language、file/store owner、query/mutation、watch/invalidation、settings/dock UI、
empty/error/unavailable states、Run injection boundary 和资源清理。

完成 O09/O10/O12/O19/O21/O22，skills/knowledge/hooks/agentMemory topics，U20 与 Skill/回忆 tools。

### R7 Skills 纵切（已实现，待最终统一门禁）

- **单一内容事实源**：project `<workspace>/.lyra/skills` 与 user `~/.lyra/skills` 是唯一 Skill bundle
  来源；SQLite 只记录 user library 的 active/archived 生命周期。每次 Desktop/Agent 读取都会让物理有效 bundle 与生命周期记录
  收敛，外部新增默认 active，外部删除或失效会清理 ghost metadata；project 同名 Skill 始终优先，但 user archive 不会错误隐藏
  project Skill；
- **同一 Agent source**：`skills.discovered.list`、Desktop Available 与 Run 内 `list_skills`/`load_skill`/
  `read_skill_resource` 共用同一个 project-first、archive-aware `ResourceSource`，不再扫描 `.agents`、`.claude` 或 `.codex`
  fallback。`propose_skill` 仅 root Run 可见，只在用户明确要求复用工作流时提交 pending review，不直接发布或执行；
- **精确审核与安全发布**：proposal 保存完整 instructions、trusted origin/source Session、scope、revises 和由 canonical
  `SKILL.md` 内容生成的 revision。approve/reject 必须携带 exact workspace/name/scope/revision；approve 会重新校验 Agent Skills
  frontmatter、大小与已知破坏性指令，以 `os.Root` confinement + 同目录原子 rename 发布，发布后 metadata/delete 失败可按同一内容安全重试；
- **按身份串行**：user library 与 exact workspace/name proposal 使用可取消的 identity lane 串行，不用全局 Skill 大锁；不同 project
  Skill 可并行，user lifecycle reconciliation 不与 archive/restore/approve 相互覆盖；
- **外部变化与资源所有权**：Runtime subscription 为 `skills.changed` subscriber 独立拥有并释放 user Skill watcher；selected
  Workspace watcher 会把 `.lyra/skills` 外部变化同时投影为 files/skills invalidation。subscription cancel、Runtime close 都会取消并
  join watcher；本实现批未启动 Runtime、Wails、Vite、browser/agent-browser 或独立 watcher 进程；
- **Desktop 工作台**：Context Dock Workspace 增加 per-Session 持久化 Skills 视图，含 Available/Proposals/Library 三个子视图，覆盖
  capability unavailable、loading、error/retry、empty、action pending/error；proposal 展示 complete instructions 与精确 provenance，
  approve/reject 使用 exact ref；user library 明确分组 active/archived，archive 永不删除物理 bundle；
- **协议边界**：继续使用既有 Lyra dotted operations、Skill/Proposal types、`skills.changed` 与 generated TypeScript client；没有引入
  Codex wire、Skill 路径或兼容 adapter。Codex 只作为 progressive disclosure / lifecycle 机制研究样本。

### R7 Recipes / AgentDocs 纵切（已实现，待最终统一门禁）

- **Recipes 是现有 Run 的客户端入口**：Runtime 只从 Workspace `.lyra/recipes` 与 user `~/.lyra/recipes` 发现
  有界 Markdown 模板，project 同名项优先；文件名必须能直接作为 slash identity，frontmatter 只拥有 description 与 argument hint，
  损坏或未闭合 frontmatter 会退化为完整正文而不吞掉用户内容。Desktop 负责 `$ARGUMENTS`、`$1..$9` 展开，再以现有
  `ContentBlock[]` 和 Run operation 发送，不建立 command/plugin 协议；幂等 key 绑定展开后的真实 payload，历史保留原始 slash invocation；
- **AgentDocs 是 capability owner 与 executor consumer 的窄接口**：Capability 以 home → project root → cwd 顺序发现
  `~/.lyra/AGENTS.md`、通用 `~/.agents/AGENTS.md` 与项目层级的 `.lyra/AGENTS.md`/`AGENTS.md`，所有读取均由 `os.Root`
  confinement、regular-file 检查、物理文件去重和大小门禁保护。`agentDocs.list` 只暴露 path/title/scope；完整 content 只通过
  consumer-owned `AgentDocumentSource` 进入 Agent executor，不扩张 wire；
- **新 Run 冻结、恢复不漂移**：只有 fresh root Run 会把适用文档组装为一条 Lyra system message；单文档绝不截断，聚合超预算时保留
  更靠近 cwd 的完整文档。消息随 Framework checkpoint 冻结，waiting/resume 不重新读取外部文档，也不把外部文件 digest 混入
  deployment identity；delegated Run 依赖 root 已接收的约束，不建立 Codex prompt/wire 兼容层；
- **统一 Resources 工作台**：Context Dock 顶层用 Resources 聚合 Skills、Recipes、Agent docs，避免顶层导航随资源种类横向膨胀；
  Recipes/Agent docs 覆盖 loading、error/retry、empty、scope/source，并与 selected Workspace query scope 及 `files.changed`
  invalidation 共用同一所有权。Composer 提供有界 slash listbox、键盘/鼠标选择和 ARIA combobox 语义；
- **资源生命周期**：本纵切没有启动 Runtime、Wails、Vite、browser/agent-browser 或 watcher 进程；project 外部改动沿既有
  Workspace file watcher 失效，user 资源在 query mount/focus/reconnect 时重读，不为缺少语义身份的全局文件伪造 `files.changed`。
- **协议边界**：`recipes.list`、`agentDocs.list`、Recipe/AgentDoc 与现有 Run/ContentBlock 继续是唯一合同；Codex 仅提供
  “资源发现与渐进消费”研究样本，未复制其协议、prompt 或产品路径。

### R7 Knowledge 纵切（已实现，待最终统一门禁）

- **人类知识的唯一事实源**：`~/.lyra/LYRA.md`、`<project-root>/LYRA.md`、`<cwd>/LYRA.md` 是 broad-to-specific
  cascade；SQLite 不保存正文。projectRoot 与 cwd 指向同一物理文件时，`knowledge.list` 只返回一个 cwd 编辑入口，避免一份文件出现
  两个可写表示；`knowledge.get` 仍可按 exact scope 读取；
- **物理 identity CAS**：每次读写先解析已有 symlink 并证明最终目标仍在 scope root 内；in-root file symlink 更新物理目标而不替换 alias，
  out-of-root target 明确返回 `path_outside_root`。写入按物理 path 使用共享的可取消 identity lane，只串行同一文档；compare revision、
  同目录 exclusive stage、file sync、atomic rename、directory sync 构成一个提交边界。rename 后的响应由已写入 bytes 构造，避免成功提交被
  post-commit 读失败伪装成失败；home 强制 `0600`，project/cwd 强制 `0644`；
- **fresh Run 上下文**：Capability 通过 consumer-owned `KnowledgeDocumentSource` 向 executor 提供非空 cascade；Knowledge 与
  AgentDocs 各自执行完整文档/most-specific-tail 预算，再合成一条 Lyra system message。fresh root Run 读取一次并由 checkpoint 冻结，
  resume 不重读，也不把外部文件 revision 混入 deployment digest；
- **外部变化 owner**：`knowledge.changed` 继续是现有全局失效语义。Runtime subscription 根据 home 与 active Workspace refs 解析 exact
  home/projectRoot/cwd 文件，单一 watcher goroutine 只观察这些文件的 parent directory；subscription cancel 与 Bus close 都会 close/join。
  `knowledge.update` 的 committed publish 与外部编辑最终收敛到同一 query family；
- **Desktop CAS editor**：Resources 增加 Knowledge 子视图，覆盖 capability unavailable、loading、error/retry、distinct scopes、
  never-created、dirty/saving/saved、revert 和 write error。clean draft 接受 event refetch；dirty draft 保留 exact baseline。冲突后刷新
  authoritative revision 并保留用户正文，要求显式再次保存；若写响应丢失但冷读已与 submitted content 收敛，则直接认定已保存；
- **协议与资源边界**：只使用既有 `knowledge.list/get/update`、content revision 与 `knowledge.changed`，没有复制旧 app 的 Plugin Host、
  publication slot 或 Codex prompt。实现批未启动 Runtime、Wails、Vite、browser/agent-browser 或独立 watcher 进程。

### R7 AgentMemory 管理 / recall / search / 自动维护纵切（已实现，待最终统一门禁）

- **独立领域与关系事实源**：AgentMemory 从 `capabilityflow.Service` 和 protocol-shaped JSON envelope 中移出，由 `domain/agentmemory` +
  `memoryflow` 独立拥有。SQLite epoch 8 使用 closed scope/origin/status、project partition、digest uniqueness、pin/status、revision/time、
  source-session/day 的关系约束，并增加 extraction receipt、immutable ledger、curation CAS state；已无 consumer 的
  `knowledge_entries` 表同时删除；
- **review lifecycle**：user add 立即 active；auto proposal 只能 pending，approve 转 active，reject 转不可见 tombstone，避免相同事实被反复
  提案。相同 target/content 的 add 幂等返回或显式复活 pending/rejected；每个 target 保持 complete-list 上限，不能无界撑爆非分页合同；
- **原子管理命令**：review/update 在一个 SQLite transaction 内读取、应用领域 transition、检查 digest collision 并按 internal revision
  提交；delete/add 同样只在 committed change 后发布 `agentMemory.changed`。现有 Lyra `agentMemory.*` request/result 不增加第二套 revision
  surface；
- **fresh Run recall**：consumer-owned `MemorySource` 合并 active user + canonical project memory，pinned-first/newest-first，以完整 item 边界
  填充 16 KiB recall budget。Memory 明确作为 fact data 放在 system message 前部，后续人类 Knowledge/AgentDocs 始终优先；resume 使用
  checkpoint 中已冻结的上下文，不重读 memory；
- **渐进检索**：Run 内 `search_memory` 读取同一 effective corpus，完整词法候选始终可用；embedding role 健康时只为有界 recent/pinned
  candidate 做即时 semantic signal，并与词法结果融合，provider/role 故障自动降级而不让安全读工具失效；Tool 输出总量与条数均有界；
- **Desktop owner**：Resources 增加 Memory 子视图，使用 Runtime-generation query family 管理 project/user scopes，覆盖 unavailable、
  loading/error/retry/empty、add、pending approve/reject、active edit/pin、two-step delete 和 row-level cancellation。命令应答丢失时通过冷读
  判断 approve/reject/update/delete 是否已经收敛，不建立 Plugin Host、publication slot 或全局 mutable mirror；
- **durable extraction**：只扫描 committed completed root Run，以 per-Run receipt 表达完成；空提炼同样提交 receipt。Conversation 只取 source
  Run 的 first + recent 有界窗口，省略 system/private reasoning/tool result body；模型输出同时受 schema 字符上限与领域 UTF-8 bytes/count/
  aggregate 上限约束，source Session/day 与真实 extractor provider/model provenance 在 receipt 归一化，immutable project ledger 只存有序
  fact。durable attempt marker 让失败项在一次 sweep 中让位给后续 backlog，扫完即停，直到下一外部唤醒再试；
- **watermark curation**：首次 ledger 立即策展，后续按事实阈值或下一次维护唤醒时的最大间隔折叠。模型只产出待人工 review 的新 proposal；
  active/user memory 不被自动改写或删除，pending/rejected/digest duplicate 都抑制重复。watermark CAS 与 proposal commit 原子化，竞争 loser
  无副作用；只有新增 visible pending 才发布 `agentMemory.changed`；
- **lifecycle isolation**：terminal Run 先 durable commit 并发布既有 Run 事件，再发非阻塞维护信号；重启扫描补偿 commit/signal 间 crash。
  utility role 可用时用于维护，否则回退 source Run 的 provider/model。单一有界/coalescing worker 归 Runtime lifetime 所有，Close cancel/join；
  失败仅保留 backlog，不改变 Run outcome，不写 LYRA.md，也不扩张现有 Lyra wire；
- **后续纵切**：Hooks 与 Tool catalog 已按同一资源边界完成；真实 PreCompact producer 已在 R10 闭合，现只待最终统一门禁。

### R7 Hooks 管理 / execution / observation 纵切（已实现，待最终统一门禁）

- **独立领域与 owner**：移除 `capabilityflow` 内嵌的一行 JSON parser 与 generic `hook_id='project'` persistence；由 closed
  `domain/lifecyclehook`、`hookfs`、`hookflow` 分别拥有规则、受限文件 I/O 与 use case。当前 SQLite epoch 9 只以
  `trusted_hook_projects(project_path, trusted_at)` 表达正向信任，false 是记录缺席；
- **confined cascade**：global 与 projectRoot→cwd 文件通过 `os.Root` 读取，in-root symlink 可用、escape 被拒绝；同一物理文件只出现一次。
  JSON 拒绝 unknown/trailing shape，event/scope/matcher/exactly-one action/timeout、单文件 bytes/count、整条 cascade count 均有界；
- **trust boundary**：`hooks.list` 解析完整 cascade 供用户在执行前审计；`hooks.setTrust` 只接受 list 返回的 canonical resolved project root，
  changed-only commit 后才发布 `hooks.changed`。effective execution source 在未信任时根本不打开 project hooks，坏配置不能阻断 global hooks；
- **external convergence**：Runtime event Bus 以 exact logical + confined physical targets 观察 global/project/cwd hooks.json；即使 `.lyra`
  尚不存在，也从最近 existing ancestor 跟进后续目录/文件创建。watch cancel 与 Bus close 仍由既有 owner join；没有新增常驻 owner；
- **bounded command adapter**：Unix `/bin/sh`、Windows `cmd.exe` 只消费 Lyra-owned typed JSON；stdin/stdout/stderr、timeout、单 action 与 aggregate
  context 均有硬上限。Unix process group 在完成、取消或超时后清理；timeout/spawn/malformed output/非 0、2 exit 只记 warning，不冻结 Run；
- **prompt / Tool policy**：fresh root 精确触发 first-Session `SessionStart` 与每次用户发起 Run 的 `UserPromptSubmit`（自治 Goal 指令与 live `runs.steer` 均不伪装成 prompt admission），deny 形成明确 failed Run，inject 进入 trusted
  system context。PreToolUse rewrite 后才走 Plan/path safety，Hook ask 与既有 approval 合并一次；用户 edited args 会重新过 Hook，若再次 rewrite
  就以最终参数重新审批。PostToolUse 只能补充模型 context，不能把已发生 effect 改写为另一个事实；durable Tool Item 记录 effective args；
- **committed lifecycle**：SubagentStart 只在 child Run durable admission 后排队；SubagentStop、Stop 覆盖 completion/cancel/lost recovery 等所有
  terminal commit；Notification 只覆盖真实 interrupt source。observe-only cascade 在 commit 时冻结并进入单一 bounded Runtime worker，Close
  cancel/join。命令私有契约见 `HOOKS.md`；现有 Lyra management/event wire 未改变；
- **R10 闭环**：真实 post-Run compaction worker 从 immutable Conversation journal 建 candidate；在 summary model call 与 projection commit 前执行
  `PreCompact`，deny 保留原上下文，allow/inject 进入唯一 summary boundary。summary projection 与 Compaction Item 原子提交，Runtime Close cancel/join；
  旧 journal 不删除，因此 export/fork/rollback 仍有完整事实。Hooks 与 Tool catalog production 纵切现已闭合，只待最终统一门禁。

### R7 Tool catalog / 渐进暴露纵切（已实现，待最终统一门禁）

- **两个目录、两个边界**：既有 `tools.list/tools.invoke` 只描述无需 Session、模型循环、approval 或进程生命周期即可安全执行的
  Run 外诊断能力，目前闭合为 `read/glob/grep`；Agent 的完整可执行集合继续由每个 Run 私有装配，绝不把 MCP、Skill、Memory 或写入工具
  暴露成 direct API，也不新增 Codex tool wire；
- **单一文件安全适配器**：`workspacefs.ConfinedExecutor` 统一拥有 admitted physical root、absolute/parent 拒绝、existing target 与 nearest
  existing ancestor 的 symlink escape 检查。Agent 文件工具、mutation path guard 与 direct diagnostics 共用这一实现；`LocalExecutor` 不再被任何
  app2 consumer 误当作 security jail，旧 `toolflow` 的手写 strict/normalize/confine 副本已删除；
- **冻结权限、渐进可见性**：Run admission 先建立完整 executable manifest，再把 Skill、Memory 与 MCP bindings 标为 deferred；初始模型只看到
  核心文件/命令/HITL/Plan/Goal/result 工具和 `search_tools`。`search_tools` 对冻结 definitions 做有界确定性关键词检索，只有再次提交 exact
  `select` names 才从下一次 model call 起可见；有序 definition/safety/deferred/intrinsic metadata 进入 deployment configuration digest，Framework
  checkpoint 持久化 advertised names，因此同清单 waiting/resume 不丢失已加载工具，外部清单漂移则按 deployment identity fail closed，绝不动态换权；
- **策略链不旁路**：deferred Tool 在进入索引前已绑定 exact safety class、Plan gate、Lyra approval 与 lifecycle Hook；`search_tools` 自身也经过同一
  Hook observation。加载只改变模型 manifest，不执行 capability、不修改安全等级，也不绕过 ToolCall/Item/Transcript 的 Lyra 语义；
- **委派清单稳定性**：delegated deployment 保留每个 binding 的 visible/deferred metadata。child 第一次真实调用以 per-Run 可取消 resolution lane
  在锁外装配能力，并对数量、definition、safety、deferred 与 intrinsic-input 做整份 frozen manifest 比对；MCP/Skill 配置漂移不能借由
  `search_tools` 换入未冻结定义，不同 child 仍可并行解析；
- **Desktop 诊断工作台**：Resources 增加可持久化 Tools 子视图，展示 Runtime 返回的真实 JSON Schema、安全级别与当前 Workspace，提供有界 JSON
  object 编辑、取消、loading/error/empty/result 状态。页面明确标注 direct diagnostics 不属于 Agent Run，不伪造 approval、Transcript 或 Run 结果；
- **资源边界**：本纵切未启动 Runtime、Wails、Vite、browser/agent-browser 或 watcher；React unmount/新调用会取消 exact diagnostic request，
  Runtime 工具清单无额外 goroutine。参考实现只提供 progressive visibility 的机制研究，Lyra Protocol、operation registry 与 generated client 仍是
  唯一合同真源。

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

### Provider/Model Runtime 实现记录（尚未统一验证）

- A2-028 以同名 model 跨 provider 的明确反例修复 Lyra 合同内部矛盾；协议只增加 Session 模型身份的 provider
  半边并提升 exact version，不复制参考产品的 wire、transport 或设置结构；
- Session、fork、rollback、JSON import/export 与 Run admission 共享一个私有 paired selection value object；explicit
  Run pair 优先，其次 Session durable pair，最后才用 Runtime default；
- Provider 配置是 revisioned aggregate，SQLite CAS 冲突会重读并重放 exact patch；base URL 与 secret 的并发更新不再
  whole-record 覆盖，no-op 不消耗 revision；
- utility/embedding role 使用 typed private store record，不再把 protocol DTO 作为 JSON persistence，也不接受 `any`；
  chat role 校验 static/live catalog，embedding role校验 provider adapter；
- dynamic model discovery 的 unreachable/malformed/empty 被区分，错误不再静默降级为空 catalog；provider test 返回
  有界脱敏 detail；只有 durable provider/role fact 真的变化才发布 `models.changed`；
- protocol `2026-08-23`、Artifact `app2/2`、SQLite epoch 10 与 generated TypeScript client 已同批刷新；
- Desktop Composer 以 explicit Session pair 保存模型并在每次 fresh `runs.start` 继续显式发送，active Run 禁止换模；
  模型能力同时治理 image attachment，已有图片不会在切到 text-only model 后被静默发送；
- Settings 是明确的 Provider/Model section，不是 generic plugin host。Provider card 的 draft/save/test/secret provenance
  独立，环境 key 只读；utility/embedding role 各自冷读与保存，dynamic catalog error/empty 分开呈现；
- `models.changed` 同时失效 models、providers 与 role query，Session update 走既有 `sessions.changed`；所有 UI query
  仍以 Runtime generation 定界。最终测试/打包门禁仍留到 R11 统一执行。

### MCP Runtime/Desktop 实现记录（尚未统一验证）

- 保留 Lyra 既有 9 个 MCP dotted methods、closed transport/secret/status/auth-attempt shape 与 protocol
  `2026-08-23`；没有引入参考产品 handshake、transport、Thread/Turn 或第二套 tool wire；
- MCP 配置重写为私有 revisioned aggregate。safe body、secret column、revision 与 `updatedAt` 分离，SQLite CAS
  使配置更新、OAuth token refresh 与 endpoint/transport 切换共享同一代际；旧 authorization result 不能覆盖新配置；
- 同 server mutation 使用 cancellable FIFO identity lane，不同 server 并行，registry lock 不跨 I/O；该 coordinator
  同时替代 capabilityflow 的重复 serializer。connect、reconnect、tool-list notification 与 interactive authorization
  都由 live generation 拒绝旧 session/result，并由 service `Close` cancel、close、join；
- `timeoutSeconds` 现在真实约束 connect/test，默认 15 秒；HTTP credential 只发往同 origin，跨 origin redirect fail
  closed；stdio working directory 必须是 host absolute path，transport 切换原子清除另一 transport 的 secret；
- authorization attempt 是独立 domain lifecycle。前序进程遗留 pending 在启动时转 canceled，terminal outcome 按
  discover retention 用 SQLite time comparison 清理；callback listener、browser launch command 与 MCP session 均有界；
- remote tool 的 lossless `(server, raw name)` 只在 model definition 边界折叠为唯一 64-char name，执行不反解析；
  tool schema 深拷贝，完整 MCP result envelope 保留 structured content/resource/annotation/error bit；
- Desktop Settings 通过明确页面组合 Provider/Model 与 MCP section，不建设 generic plugin host。新 server 可对完整
  candidate 做无副作用 Test；既有 write-only secret 不伪造 candidate test，只提供真实 reconnect。HTTP/stdio draft、
  masked secret replacement/clear、disabled/auto-approve disjoint policy、OAuth terminal polling、confirmed delete 与所有
  loading/error/empty state 均回读 Runtime authority；unmount 会取消 test/auth polling；
- durable mutation和每个 live/auth status transition 都由 MCP owner 发布 `mcp.changed`，Desktop 失效 MCP query root。
  SQLite 当前 exact epoch 提升到 11；Lyra wire 与 generated client 无 shape 变化。最终测试、恢复与 package 门禁仍留到
  R11 统一执行。

### Approval Policy Runtime/Desktop 实现记录（尚未统一验证）

- 保留 Lyra 既有 `approval.getMode/setMode/listRules/forgetRule` 四方法与现有 Interrupt remember shape；没有增加
  handshake、别名 method 或第二套协议，protocol 仍为 `2026-08-23`，generated contract 无 shape 变化；
- Approval 从 schedule-oriented `settingsflow` 分离成私有 `approvalpolicy` domain 与 `approvalflow` use case owner。
  SQLite 不再保存 protocol DTO JSON，而以 mode revision、scope key、match kind、decision、timestamps 和 Session FK
  表达可约束状态；exact schema epoch 提升到 12，开发期直接重建旧 app2 data home；
- mode 不在 Run 建立时冻结。每个 Tool effect 在 Hook rewrite 后读取当前 stance，并以 Session > Project > Global、
  exact > glob > whole-tool 的 specificity 选择 visible rule；同优先级冲突 fail closed 为 deny；
- 用户从既有 Approval Interrupt 选择 remember 后，Runtime 用原始 subject 保存 exact rule；edited args 保持 one-shot。
  Session rule 随 Session 删除级联，project/global rule 独立；forget missing 是幂等 no-op，事件只在事实真正变化时发布；
- MCP auto-approve 只豁免默认 mode prompt，不绕过 Hook `ask` 或灾难命令确认。高置信 root/home/device wipe 即使
  Yolo 或 remembered allow 仍强制确认；它是 confirmation override，不冒充 sandbox；
- Desktop Approval section 冷读 authoritative mode，不用本地假默认；按 selected Session 展示 visible rules、scope、
  tool/subject/project 与 confirmed forget，并通过 `approvals.changed` 失效同 Runtime generation query family。
  最终 domain/property/SQLite/HITL/recovery/UI/package 门禁仍留到 R11 统一执行。

### Schedule Runtime/Desktop 实现记录（尚未统一验证）

- 保留 Lyra 既有 `schedules.list/create/update/delete/runNow` 五方法、Schedule DTO 与
  `schedules.changed`；没有引入参考产品 Thread/Turn、task wire、stdio handshake 或别名 method，protocol 仍为
  `2026-08-23`，generated contract 无 shape 变化；
- Schedule 改为私有 aggregate：cron、enabled/next-run、last admitted time、paired provider/model、显式或 default
  workspace、revision/timestamps 由同一 invariant boundary 管理。SQLite 使用 normalized columns 与 CAS，不再保存
  protocol DTO JSON；exact schema epoch 提升到 13；
- `scheduleflow.Service` 只拥有管理 use case，`scheduleflow.Dispatcher` 只拥有 runNow、timer、pending recovery 与
  cancel/join。此拆分解除 Agent tool → Schedule management → Run engine 的依赖环，不使用 late-bound service locator；
- cron worker 先在一个 transaction 中 CAS 推进 cursor 并保存 immutable pending occurrence。Occurrence 固定
  Session/Run identity 与 Schedule snapshot；Runtime 重启先重放 pending，Schedule 后续编辑或删除不改变已经到期的
  intent；同 Schedule 同时至多一个 pending occurrence；
- Session、opening Run/Item/Conversation/events、occurrence acceptance 与 Schedule lastRun 在一个 transaction
  admission。多 Runtime 竞争只允许一个 caller 获得 launch ownership；runNow 复用同一 admission，但不推进 cron
  cursor。已 admission 后的进程丢失继续由 Run recovery 收敛为 visible lost，不盲目重放可能已发生的 effect；
- root Run 获得 deferred `list_schedules/create_schedule/delete_schedule` 单动作工具，并继续经过 Plan、Hook 与
  Approval 链；工具管理依赖不触碰 Dispatcher。Desktop Schedules section 支持 create、revisioned edit、pause/enable、
  runNow 后导航、confirmed delete，明确显示 Runtime default workspace/model 与 authoritative next/last/revision；
- CRUD、occurrence claim/accept 与 runNow 由 owner 发布 exact `schedules.changed`。Desktop subscription 与 resync
  router 失效同 Runtime generation 的 Schedule query。最终 domain/transaction/race/restart/UI/package 门禁仍留到
  R11 统一执行。

### Runtime Subscription / Hooks Desktop 实现记录（尚未统一验证）

- 保留 Lyra 现有 `runtime.subscribe`、15 个 subscribable topics、`resync` event、WatchSpec 与 HTTP/SSE binding；没有增加
  event snapshot、Last-Event-Id 资源 journal、connection initialize 或参考产品通知协议，generated contract 无 shape 变化；
- subscriber 在 ack 前先进入 Bus registry，使事务 producer 从该点起不会漏发；files/skills/knowledge/hooks external watcher
  各有 startup barrier，只有成功 watch 或明确发出 scoped resync 后才标记 ready。所有 watcher ready 后，Bus 发一次包含 exact
  requested topics/watch IDs 的 initial resync，关闭 query-before-subscribe 与 reconnect acknowledgement-loss 窗口；
- 每个 subscriber 拥有独立 sequence 与 128-frame bounded queue。overflow 不挑一条静默丢弃，而是清空不可信 backlog 并替换为
  全订阅 resync；递归 file watch 同时受 context 与每 watcher 目录硬上限约束，越界后发 scoped resync 并停止不可信 watcher；
  Close/cancel 删除 subscriber、取消 watcher 并 join Runtime-owned WaitGroup，Subscribe/Close 的 Add/Wait 不竞争；
- Desktop generated client继续验证每个 frame；invalidation consumer 额外检查 sequence 必须逐一递增，gap 会合成同订阅范围
  resync。新 connection generation 使用隔离 query key；每次重连都从服务端 initial resync 冷读，不把 renderer cache 当事件真相；
- Desktop 现在订阅全部 15 topic，并为 `hooks.changed` 失效 Runtime-generation Hooks query。Lifecycle Hooks Settings 按 selected
  Session workspace 冷读 global/project cascade，逐条展示 event/matcher/verbatim command 或 inject/source/timeout/active；project
  command 必须经过 review-confirm 才写 exact project trust，revoke 与 external hooks.json edit 同样回读 authority；
- 至此 Runtime resource topic 的 15 change + resync production producer/consumer 路径均为 implemented。最终 overflow/gap、
  watcher create/rename、disconnect/reconnect、generation replacement、UI 与 resource-leak 门禁仍集中到 R11。

## 13. R9：高级 Session 与运营能力

- fork boundary、rollback history/files/both；
- app2 JSON/Markdown export、strict atomic import；
- conversation/session search、long history pagination；
- usage session/summary 与 cost unknown；
- feedback create；
- portable archive/favorite 等最终 Session lifecycle。

必须用大历史、corrupt artifact、identity conflict、partial file failure、restart 和 dropped-run cleanup 验证。
完成 O02 剩余、O20/O23、U19。

### R9a Session material 纵切（已实现，待最终统一门禁）

- **既有 Lyra Protocol 是唯一边界**：继续使用 `sessions.snapshot/fork/rollback/export/import` 与
  `items.list(order=desc)`，不复制 Codex Thread/Turn、artifact 或 transport，也不新增平行方法族；Codex 只提供
  lifecycle、bounded hydration 与恢复机制研究样本；
- **有界 mount、完整冷读**：Session snapshot 在同一只读事务返回最新 200 个 Item、其 Run ancestor closure、open Run、
  Interrupt、Plan 与 Goal；更老 transcript 仍由既有 DESC keyset cursor 读取，不再要求 Desktop mount 前下载无限历史；
- **结构化 fork/rollback**：fork 只接受 terminal root boundary，为 Session/Run/Item remap 新 identity，并复制 exact
  Conversation、known Plan boundary 与大 Tool result binding；history rollback 原子删除 boundary 后的 root/child tree、
  Interrupt/Goal/Plan mode 等 FK material，以 known boundary 的新 revision 恢复 Plan，并返回 dropped root user input 给
  Composer。files/both 继续走 Session shadow checkpoint，提交后清理 dropped Run refs；
- **portable terminal artifact**：JSON artifact 保留 Session metadata、paired provider/model、terminal Run tree、root-only
  protocol profile、Run metrics/limits/outcome、Item、opaque validated chat messages、message marks、Plan 与 canonical offloaded
  Tool result；Markdown 是人类可读导出，不冒充可导入格式。既有 identity 冲突返回 `revision_conflict`，绝不覆盖；
- **strict all-or-nothing import**：Runtime 在任何 write 前递归校验版本、workspace/model pair、时间、record bound、Run DAG、
  child ToolCall owner、root profile、Item union/nested content/question/tool、base64 image、tool-result binding、message mark 与
  ToolCall/ToolResult journal closure；全部 material 只在一个 SQLite transaction 中以新 Session identity 创建；
- **Desktop native ownership**：Wails host 只负责 bounded JSON open 与 JSON/Markdown save dialog；文件名来自受控 Session ID，
  不从 title 或导入内容构造 path。Frontend 只消费 generated Lyra client，Work Index 提供 import/export/whole fork，历史 turn
  提供 boundary fork 与 history/files/both rollback，dropped input 原样恢复到 Composer；并发 history action 在 client 侧串行；
- **Session lifecycle 收敛**：旧产品没有独立 Session archive operation，89-method Lyra catalog 也没有第二 lifecycle；本阶段的
  archive 指 portable artifact，收藏继续通过既有 revisioned `favorite` 更新表达，不为参考产品概念扩张协议；
- 当前只标记 O02/U03 为 `implemented`；conversation search/long-history UI、usage、feedback 仍分别留在 R9c/R9b，所有
  compile/test/fault/parity/package 与 resource-leak 证据按既定节奏集中到 R11。

### R9b Usage + Feedback 纵切（已实现，待最终统一门禁）

- **成熟 Lyra wire 保持唯一**：继续使用 `usage.session`、`usage.summary`、`feedback.create` 与 generated Desktop client；
  不复制 Codex analytics、Thread/Turn attribution、通知 taxonomy，也不新增运营 topic 或第二套 transport；
- **typed accounting boundary**：SQLite adapter 是唯一 Run durable JSON decoder，operations owner 只消费私有 typed usage
  record。只读取 terminal Run，按 exact Session/provider/served-model/UTC-day 汇总；Session 的 `byModel` key 使用
  `provider/model`，避免跨 provider 的 model id 冲突；
- **unknown cost 端到端闭合**：模型 observation、live/durable Run projection 与跨 Run accumulator 都遵循同一规则——只要
  任一真实 contribution 没有 cost，对应 total/bucket 就省略 cost；绝不把已知部分或 0 当作完整价格。provider/day 使用
  whole-Run total，model bucket 优先使用 durable `byModel` slice；
- **private feedback aggregate**：Item/Run/Session 以最具体身份回查 canonical owner，请求同时给出的上层身份必须一致；
  general feedback 必须有文本。文本限制为 4000 UTF-8 bytes，并在领域边界确定性 redaction credential-like material；SQLite
  只持久化 normalized private record，exact schema epoch 为 14；
- **产品边界**：Settings 新增 7/30/all-time Usage section，展示 authoritative token/run/session/provider/model/day 与明确的
  unknown-cost 语义。只有 terminal root Run 的 completed final answer 显示 Helpful/Needs work；失败只显示该 answer 的局部
  error，不修改 Run outcome、snapshot 或 Session 状态，delegated final material 没有反馈入口；
- O20、O23、U21 当前只标记为 `implemented`。production code 已完成；compile/test/restart/corruption/transport/UI/package 与
  resource-leak 证据按集中验证约定留到 R11 一次执行。

### R9c Transcript Search + Long History 纵切（已实现，待最终统一门禁）

- **Item 是唯一 durable source**：private `SearchableText` 只投影 user/agent text 与 Question prompt，明确排除 reasoning、Tool
  arguments/result 与 Conversation model journal；SQLite external-content FTS5 与 Item insert/update/delete/cascade 同事务维护，
  不建立异步搜索 owner、第二 revision 或 parallel conversation store；
- **Agent search 不扩张协议**：`search_conversations` 作为 deferred safe Tool 进入 Run-frozen catalog，经既有 `search_tools`
  渐进暴露；scope 只能是 mounted Session 或 exact workspace，query/term/hit/snippet/output 全部有界，结果明确为 untrusted
  historical excerpts。89 个 dotted operations、HTTP/SSE 与 generated contract shape 均不变；
- **既有 cursor 承担长历史**：Desktop 使用 `items.list(order=desc, limit=100)` infinite query，自动跨过与 bounded snapshot
  重叠的 page，只在用户要求时 materialize older window；每页同时合并 Runtime 返回的 Run ancestor summary，不伪造 metrics；
- **本地查找与阅读控制**：已 materialize 的 Item 支持 Cmd/Ctrl+F、Enter/Shift+Enter、previous/next、match count、message
  highlight 与 exact Item scroll；无 match 时可逐 window “Search older”，加载旧材料会退出 follow-tail，避免跳回最新回答；
- SQLite exact epoch 为 15。U19 与回忆/发现 Tool family 当前标记 `implemented`；large-history/FTS corruption/rebuild、
  rollback/import/delete、keyboard/a11y/visual/package、统一 compile/test 与 resource-leak 证据仍留到 R11。

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

### R10a Remote / Tool / lifecycle 实现记录（尚未统一验证）

- remote Runtime 继续使用 Lyra `/v2/rpc`、SSE replay/cancel 与既有 sidecars；explicit remote mode 才允许非 loopback，且必须同时提供 TLS certificate/key、path-owned bearer token 与 exact non-wildcard Origin；
- bearer source 每个 RPC 从 0600 token file 读取，原子替换即可轮换，失效 credential 返回 bounded transport problem；本地 descriptor 与 remote mode 互斥，Desktop 绝不终止不属于自己的 remote process；
- Desktop remote profile 只持久化 origin 与 server name，token 进入 OS keyring；首次配置、重新启用与后续 bootstrap 都验证系统 TLS、`info/live/ready/discover` 的 protocol/instance 同一性，instance replacement 形成新 renderer generation；本地/已保存远端切换不要求把 secret 重新带回 renderer；
- 通用 connection credential breaking rename 为 `bearerToken`，endpoint contract authentication 收敛为 `bearer`，不保留 `localToken` alias；RPC method/event/error 仍完全沿用 Lyra Protocol；
- generated client 只放行 loopback HTTP 或 origin-only HTTPS，不修改 Runtime method、params/result、event 或 error shape；这是 transport deployment 能力，不是复制 Codex handshake；
- Shell 改为 Runtime-owned background job aggregate，`shell/read_shell_output/stop_shell` 共享 process/output/cancel owner，并在 owner boundary 强制 session identity、终态有界回收；下一 root Run 动态注入仍存活 job ID 与当前 in-progress Plan，不把命令文本或过期状态固化进 summary。LSP 为 workspace-confined lazy server manager，支持 config replacement table 与单文件 mutation 前后新诊断差分；三个 network tools 只有凭据/allowlist 明确配置时才进入 deferred manifest；
- Conversation 完整 journal 继续 immutable；`conversation_compactions` 只保存可回退的 effective-context projection。真实 candidate 调 `PreCompact` 后由 utility/fallback model 总结，并与 user-visible Compaction Item 原子提交；启动时在 Run recovery 后幂等重排关机窗口遗漏的 candidate；SQLite epoch 提升到 16，Lyra wire shape 不变。

### R10b Content / media 实现记录（尚未统一验证）

- Agent Narrative 不再用 delimiter split 冒充 Markdown parser；Lyra-owned renderer boundary 使用 semantic Markdown + GFM、CJK emphasis/strikethrough、basic raw HTML allowlist 与 single URL policy。raw HTML 在 KaTeX materialization 前清洗，link 只允许 absolute HTTP(S)/mailto 或 fragment，Markdown image 不静默发起不可信 remote fetch；
- fenced code 拥有 language、copy、wrap 与 plain fallback；Shiki 仅在带语言 code 首次出现时动态加载，未知 language 或加载失败不损失 source。table 使用可聚焦 semantic overflow container；KaTeX 保留横向阅读而不挤坏 Narrative；Mermaid 进入视口前不加载，使用 strict renderer、unique identity 与二次 SVG sanitation，失败回退原始 source；
- Runtime `ContentBlock` 与 Lyra Protocol 完全不变。Renderer 只接受 allowlisted image MIME、strict base64 与 32 MiB 对应的 encoded bound；相邻图片组成 gallery。lightbox 支持 zoom、前后键、Escape、focus containment、40px+ controls 与 reduced-motion；保存只调用既有 `NativeHost.SaveImage`，由 Go host 再次校验并打开 native save dialog，不建立浏览器 download 或任意路径写入；
- `react-markdown`/unified、KaTeX、Shiki、Mermaid 是 presentation implementation detail，不形成新的 wire、conversation identity 或参考产品兼容层。U17/U18 到 `implemented`；streaming/large material/a11y/visual/WebKit/package 与 dependency audit 留到 R11 统一门禁。

### R10c Appearance / remote control 实现记录（尚未统一验证）

- shell appearance 由单一 React context owner 持有，只持久化 strict `{theme, accent}` value；system/light/dark resolution、OS scheme listener、root token painting、native `color-scheme`/theme-color 同步在一个 boundary 完成。内建 Linen/Graphite 与四个 functional accents 不建立 plugin registry、custom theme engine 或 component-local storage；
- dark token ladder、status foreground 与之前散落的 light-only translucent surface 已收敛到 semantic tokens；内容 Mermaid 在 serialized render owner 内按当前 scheme 重建，避免切换主题后保留不可读 SVG；
- Settings 增加 Appearance 与 Runtime connection 两个真实页面。remote profile 显示 active identity，initial/replacement secret 仍只作为 write-only Wails call 进入 OS keyring；saved remote/local/forget 都经过既有 DesktopHost owner，mutation single-flight，active target change 以 Runtime instance/generation key 重新 mount Workspace，避免 Query 与 local Session state 跨实例串线；
- active remote 在启动时不可达时，boot failure 提供显式 `Use local Runtime` recovery，不要求先进入已不可达 Runtime 的 Settings。remote 仍复用 Lyra probes/RPC；本批没有新增 wire、descriptor 或 secret persistence；
- U22 中 theme/accent/light-dark production 已实现，locale/RTL 仍保持 `in_progress`，不得因只设置 `lang` 冒充完成。P01–P03 的用户切换/离线逃生路径已接通；最终 network/package matrix 留到 R11。

### R10d1 Typed localization boundary 实现记录（尚未统一验证）

- Desktop 新增 app2-owned typed semantic message set、safe named interpolation、locale-aware number/time formatting 与 isolated-component English default；不引入旧 app 的 i18next/plugin graph，也不复制 Codex 的文案 key、协议或产品命名；
- 启动/recovery、Agent Narrative、history search、HITL approval/question、Composer attachment、recipe、Markdown code/diagram/image/lightbox 的可见 copy、error fallback 与 accessibility label 已进入同一 localization boundary；用户/模型内容、Tool identity、Lyra enum 与发送给模型的 attachment material 保持原始领域值；
- provider 当前只激活完整 English dictionary，并据此设置 `html.lang/dir`；locale preference、其余 Desktop surface、八个旧能力 locale 的精选 app2 dictionary、Arabic RTL 与 logical CSS 尚未闭合前不暴露 locale selector，因此 U22 继续保持 `in_progress`；
- 本批不改变 Lyra wire、generated contract、Runtime method/event/error、SQLite epoch 或 Desktop bridge。其余 production copy 继续按 bounded surface 迁移，最终 compile/a11y/visual/package 仍只在 R11 统一门禁执行。

### R10d2 Session / activity presentation language 实现记录（尚未统一验证）

- Session index/new-session、per-Session model picker、Overview/Timeline/Terminal/Summary、Tool disclosure 与 clipboard summary 已迁入 typed localization boundary；empty/loading/error/action/a11y copy 与 locale-aware date/number presentation 使用同一 context；
- known Tool 与 Run/Session 状态经显式 display resolver 映射，`presentTool`/activity projection 仍是 pure presentation function，并以 complete English translator 作为 isolated test/default seam；wire status、approval decision、tool name、provider/model identity、command/path/output 继续保留 canonical 原值；
- summary projection 的本地化只影响当前 renderer/copy material，不写回 Session、Item、Run 或 artifact，也不新增 protocol operation。Settings、Workspace、Goal/Plan 与最终 locale dictionaries/selector/RTL 仍继续迁移，U22 保持 `in_progress`。

### R10d3 Goal / Plan / shell context language 实现记录（尚未统一验证）

- Goal composer/tray/budget/status/reason、Plan compact/progress/a11y、Work Index/Agent Narrative/Context Dock shell、Runtime identity/facts 与 file Context Dock 的 tree/search/tabs/reader states 已迁入 typed localization boundary；数字、日期、USD budget presentation 从 active locale formatter 派生；
- Goal reason/Plan step/connection/Run-facing shell 状态只在 display resolver 变成人类可读 copy；objective、step description、workspace/path/file/command、Runtime instance/protocol 与 artifact material 不翻译、不写回。恢复图片名称是 renderer-local attachment label，不进入 wire；
- Session artifact/import/export/history/feedback 的 renderer fallback error 使用 semantic key，但 RPC error detail 继续原样展示。Resources 子页面与 Settings 尚待后续 bounded 批；U22 仍保持 `in_progress`，locale selector 继续隐藏。

### R10d4a Resource reading / diagnostics language 实现记录（尚未统一验证）

- Resources 导航、Recipe/Agent docs catalog、semantic Codebase、Git Review、Knowledge editor 与 direct diagnostic Tools 已迁入 typed localization boundary，覆盖完整 loading/empty/error/mutation/a11y copy，并统一使用 locale-aware counts/date；
- Recipe body、Agent doc/Knowledge 文档、code snippet、diff row、path、tool schema/arguments/result、model/index identity 与 Runtime error detail 仍是原始内容；本地化不会改变 search query、diagnostic invocation 或 Knowledge CAS revision；
- direct diagnostic 继续是 workspace-scoped read-only Runtime capability，不因 presentation 迁移伪装成 Agent Run。Skills/Memory 与 Settings 仍待后续批，U22 保持 `in_progress`。

### R10d4b Skills / Agent Memory language 实现记录（尚未统一验证）

- Skills discovery/proposals/library 与 Agent Memory scope/add/review/edit/pin/delete surfaces 已迁入 typed localization boundary，覆盖 unavailable/loading/empty/error、exact-revision approve/reject、archive/restore、byte bound 与 CAS conflict copy；
- Skill name/scope/revision/instructions、Memory content/origin identity、proposal decision 与 query keys 保持 Runtime canonical value；翻译不改变 proposal approval、managed lifecycle、Memory convergence refresh 或 abort owner；
- Resources production copy 已闭合，后续只剩 Settings surface、dictionary/locale owner/selector 与 RTL/CSS 收口；U22 仍保持 `in_progress`。

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
