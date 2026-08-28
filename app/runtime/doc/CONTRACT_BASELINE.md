# ScopeApp Runtime 合同基线

> 状态：Runtime Protocol Baseline 2
>
> 基线日期：2026-08-28
>
> 适用范围：Runtime Protocol 制品、持久化 shape、Agent Framework 消费边界和重构期间的内部防腐合同

本文只记录可被机器比较的边界事实和版本。它不是向旧消费者承诺兼容；仓库允许 breaking change，但任何变化必须显式、一次性、可验收。

## 1. 基线含义

Runtime 是应用后端，同时提供公共 Go binding。只有 `protocol`、`embedded` 与窄部署交接包 `localruntime` 的 exported surface 构成 Go API；`internal` exported identifiers 仍只服务模块内组合。因此本基线不冻结全部内部 `go doc`，而冻结四类真实合同：

1. 外部 Runtime Protocol、公共 Go protocol/embedded/localruntime surface 和生成制品；
2. SQLite/artifact/checkpoint 等持久 shape；
3. Application 与 Agent adapter 的防腐边界；
4. Clean Architecture import DAG 和外部 SDK isolation。

任何基线变化必须：

- 有对应 ADR 或已授权阶段；
- 同批更新 owner codec/schema/GoDoc/tests；
- 直接替换旧 shape，不保留 alias、双读或兼容 shim；
- 更新本文件和自动守卫；
- 运行该 owner 的 strict round-trip、malformed input、integration 和 consumer tests。

Digest 只用于发现未审计漂移，不能替代语义测试。

当前 Runtime 与 Desktop module 统一使用 Go `1.27.0`。重构代码使用该版本已经提供的标准库和测试能力，不引入为旧 Go 版本服务的兼容写法；两个 app module 的 `go` directive 与 Desktop 隔离 workspace 必须保持一致。

P186 为仓库内 85 个非 `app/**` Go module 建立首个 canonical `v0.0.1`，标签名称精确为 `<module-dir>/v0.0.1`；上层 module 只引用已发布的下层正式版本。4 个 app module 不属于该发布集合，`app/runtime` 只把非 app Scope 依赖切换为 `v0.0.1`，内部 `localruntime` 部署交接仍由 app 自身版本拥有。该发布事实不改变 Runtime Protocol、公共 Go surface、Artifact、SQLite、Agent Framework contract 或 Desktop generated binding。

P187 将产品身份一次性切换为 `scopeapp` / `ScopeApp`：协议元数据、OpenRPC 扩展键、生成 TypeScript package 与公共客户端名称不再发布旧品牌。Runtime 只接受 `SCOPEAPP_*` 环境变量，默认 durability 只位于 `~/.scopeapp`，知识文件只名为 `SCOPEAPP.md`；旧环境变量、目录、文件名、别名与兼容 reader 均不存在。共享 `localruntime.DataDirectory` 是 Runtime/Desktop 对 database 与 local-token 布局的唯一部署值对象。该 breaking cutover 将 Protocol 精确前移到 `2026-08-28`；Artifact v23 与 SQLite epoch 83 的数据 shape 不变。

P188 不改变 Runtime Protocol Baseline 2、Artifact v23、SQLite epoch 83 或 public Go/Desktop binding shape。它把内部模型上下文的预算检查从 terminal Run maintenance 扩展到每次主模型调用前，但只有 message/token footprint达到阈值才执行压缩与 durable rewrite；阈值以下不运行 hook、摘要或写事务，压缩后也不立即重复。durable root rewrite继续使用同一 SQLite transaction/CAS owner，transient Delegate只更新 Agent recovery state；compaction Item仍使用既有 Protocol shape。Runtime 独立 module消费 canonical `agent/v0.2.0`、Agent Baseline 33 与 Interaction state/protocol v8/v8，不双读 v7 settlement。Agent 的 `ToolInvocation.ModelResult` 是普通 Tool model-visible result 的唯一映射 owner；Runtime 不从 client presentation 反推 provider Conversation。

P189 不改变任何 public contract或持久化 shape。Runtime internal context provenance新增 isolated `session_goal` data source，并把 `session_plan`从 instruction重新归类为 data；两者只在每次主模型调用的 fixed Session-state context中出现，不进入 Deployment identity、Conversation store、summary或 public Protocol。configured Goal/Plan read failure与非法/foreign state现在 fail closed。Agent release、Interaction protocol/state version、Artifact v23、SQLite epoch 83、Desktop generated binding与 CLI均保持不变。

P190 不改变任何生产合同、wire或storage shape。真实retry-exhaustion与SIGKILL回归确认SQLite conversation+Run watermark事务是compaction唯一durable commit；运行中的未结算Strategy generation不构成可恢复状态，失败后按既有failed/lost语义退出。没有新增journal、checkpoint字段、两阶段提交、SQLite epoch、Agent protocol、Runtime operation/event或Desktop binding；HTTP E2E测试数增至45。

P191 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release或CLI。主模型上下文预算改为完整provider-neutral request的单一估算：全部Message Part、metadata、Tool manifest与Options同属一个owner，media transport payload不冒充文本token；provider成功响应的input usage校准同一Process下一次调用的阈值判断。低于message/token阈值仍不运行hook、summary或rewrite。executor-owned opaque Interaction checkpoint envelope因新增per-Process calibration从schema v3一次性升至v4；这是Runtime internal recovery shape，旧schema确定拒绝且不双读。

P192 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release或CLI。Runtime internal `CompactionConfig` 与调用签名一次性增加provider catalog已有的`MaxInputTokens`事实；有效token阈值不超过selected model硬输入上限，且不从default model向部分已知的selected model借用硬限。低于阈值的路径语义、provider usage校准、checkpoint schema v4与所有公开合同保持不变。

P193 不改变 Runtime Protocol、Artifact v23、SQLite epoch 83、Desktop binding、Agent Framework release、checkpoint schema v4或CLI。Runtime internal model-limit边界一次性收敛为immutable `modelref.TokenLimits`，把provider独立发布的context window、max input与max output事实映射到同一值对象；显式`runs.start.params.maxTokens`必须在durable Run admission前满足selected catalog model的output上限，并从总context中保留对应generation空间。catalog未知的私有兼容model仍允许通过，未显式设置output ceiling时不猜provider默认值。provider usage为零表示缺失，不产生虚构校准；负值、溢出与其他非法usage继续由既有`chat.Response`校验拒绝，正值仍是同一Process下一次调用的唯一校准事实。

P196 将模型选择从 exact provider/model pair 一次性扩展为 exact provider/model + 可选、model-owned `reasoningEffort`。Session 是可编辑默认的唯一 durable owner；Run 在 opening 时冻结自己的选择，Goal 与 Schedule 同样冻结完整选择，恢复、fork、occurrence、interrupt、checkpoint 与 Artifact 不得丢失该事实。仅修改 effort 保留 identity；显式切换 provider/model 而未同时给 effort 时清空旧 effort，禁止把上一模型的等级泄漏给新模型。已知 catalog model 在任何 durable write/staging 前校验 effort 是否属于其精确等级集合；catalog miss 的私有模型继续允许。`runs.resume` 的可选 `input` 与已接受 HITL responses 在同一 continuation opening 中提交：answer claim、用户 Item、checkpoint removal 与 next Segment 只有一个 durable winner；Agent 在同一 safe boundary 先结算 answer Tool result、再应用 input steer，Conversation 按同一合法 `tool → user` 顺序各追加一次，已由 opening 创建的 exact Item 不得重复投影。Artifact 前移到 v24、SQLite 前移到 epoch 84、executor-owned Interaction checkpoint envelope 前移到 private schema v5；旧 shape 全部确定性拒绝，不双读、不迁移。Desktop 直接消费 catalog 的 context/input/output limits、reasoning/default/levels、modalities、tool use 与 structured-output 事实；Composer 控件写回同一选择，active Run 的 Context gauge 使用该 Run 冻结的选择，不被 Session 后续默认修改污染。发布图已落为 `models/catalog/v0.0.2`、`app/runtime/localruntime/v0.0.1`、`app/runtime/v0.0.1` 与初始 `app/desktop/v0.0.1`；Wails beta.15 替代发布候选等待 `gorelease` 裁决。CLI/TUI 不变。

## 2. Runtime Protocol Baseline 2

机器真相源位于 [`../contract`](../contract)：

| 制品 | SHA-256 |
|---|---|
| `contract/manifest.json` | `30918e3265162772af307e56eff2ec3b83241d1fba83739272d6f396adbf073a` |
| `contract/openrpc.json` | `3af6c440fa61e406a715d17c49bd1c3fff5ed29540b13e2521569a22e14ad6fc` |
| `contract/schema.json` | `50edd5a8c7a5548387d0c0349a37a4af25f275dd8a7c78ba77568fb219db6737` |
| `contract/go-api.json` | `6d5034c2002cf6ea5b185c76fb28c02e543ece98a7e9a9c4e37003979d78a74b` |

TypeScript generated files 是派生制品，不单独定义语义。它们必须由同一个 contract generator 产生且 diff-free；当前前端/TUI/CLI 是否已经消费最新 shape，由 P10/P12 的 consumer handoff 记录，不通过兼容字段掩盖。

仓库身份迁移只让公共 Go surface 的 canonical package path 变为 `github.com/Tangerg/scope/app/runtime/...`，因此 `go-api.json` digest 显式重新冻结；声明集合、签名、Protocol、Artifact、SQLite 与运行时语义均未变化，也不保留旧 module path。

人读语义 owner：

- [`API.md`](API.md)：业务方法、Run/Item/Event 语义与跨方法不变量；
- [`TRANSPORT.md`](TRANSPORT.md)：HTTP/SSE binding、流、重放和安全；
- [`AUX_API.md`](AUX_API.md)：VCS、MCP、审批等旁路能力。

本文件不复制 method、field、error 或 example catalog。

当前协议版本为精确值 `2026-08-28`，Artifact 为 v24，不存在兼容范围或旧归档 reader。Session 在 Domain、SQLite、Protocol 与生成消费者上只发布 exact provider/model/reasoning selection；省略 Run selection 时读取该 durable selection，不按 model id 推断 provider，也不从上一模型继承 reasoning effort。Session workspace 在 Domain 中是 exact value，SQLite 只保存 `workspace_path`；Protocol/Artifact 既有 `WorkspaceRef` shape 不因内部 owner 收敛虚增版本。RunEvent 只有七个 Runtime 实际生产的一等变体，Interrupt/response 只有 approval 与 question；没有 custom 旁路、clientTools feature 或 toolResult interrupt。Plan 是一等 `plan.updated` / `plan.changed` / `plan.get` / `SessionSnapshot.plan` / `SessionArtifact.plan` 合同，不再经过通用 state registry、key、scope/writer metadata 或 `states[]` union。Feature 与 Method 合同只发布能改变协商或消费决策的事实，不携带恒为 `stable` 的 stability 标签；method policy 同时发布 idempotency 与 run replay cursor applicability，只有 `runs.start`、`runs.resume`、`runs.subscribe` 接受 run cursor。唯一 replay scope 是 `runtimeInstanceRootSegment`：它准确表达一个 Runtime instance 内的一条 root Segment replay buffer；旧 `processRootSegment` 已直接删除。消费者 breaking surface 与未接线事实由 [`CONSUMER_HANDOFF.md`](CONSUMER_HANDOFF.md) 唯一记录。

P153 直接删除 Codebase semantic-index contract：公共 Go surface、三项 operation、feature、runtime topic、DTO/enum/sample 及 Desktop/CLI direct consumer 同批消失；不存在旧同日 Protocol shape reader、disabled capability 或 compatibility binding。当前 manifest 精确发布 86 个 methods、17 个 features 与 15 个 runtime topics。Embedding role 仍是 Agent Memory 的可选配置，不是被删除能力的残留别名。

P154 不改变 Protocol、公共 Go API 或 Artifact shape。Agent Memory embedding role 仍是同一可选 provider/model pair；服务端内部 search cache 现在把 vector 与 exact embedding space、content digest 一起绑定，role/cache 变化不新增 operation、event、feature 或 consumer handoff。

P155 不改变 Protocol、公共 Go API、Artifact 或 SQLite shape。内部 Agent Memory recall 不再接受任意单 scope，而是对当前项目的 active project items 与全局 active user items 做一次联合 ranking/top-k；`agentMemory.list/add/update/review/delete` 的显式 scope 合同与 Desktop 管理面不变。

P156 将 Agent Memory 的 `add.content`、可选 `update.content` 与 `AgentMemoryItem.content` 精确约束为最多 4096 个 Unicode code point；Go validator、JSON Schema/OpenRPC、TypeScript validator 与 Desktop request/result boundary 均由同一 Contract Registry 生成。Protocol 精确值仍为当前开发日期 `2026-08-24`，不接受同日旧 shape；Artifact v23 不变。

P157 只收紧 Runtime 内部 auxiliary model request envelope：title、compaction、Agent Memory consolidation 与 Skill mining 必须显式携带 aggregate input-byte/output-token 上限，maintenance transcript 受 384KiB total / 24KiB per-message 公平预算约束。该批不改变 Protocol、manifest/OpenRPC/schema、公共 Go API、Artifact v23、SQLite epoch 82、Desktop binding、Agent Framework 或 CLI。

P158 只收紧 Agent Memory 内部有限集合与生命周期行为：每个 project/user target 最多 512 个 active + pending item、最近 2048 个 rejected tombstone，单次 extraction/curation 最多 32 条，pending ledger page 最多 128 条；显式 Add 可原地恢复同 digest 的 pending/rejected proposal。`agentMemory.*` request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23 与 SQLite epoch 82 均不改变。

P159 将 Runtime internal Skill Proposal storage 从 `_proposals/<revision>/SKILL.md` 一次性切换为 `_proposals/<name>/SKILL.md`，每个 project/user scope 最多 128 个 current proposal，完整 authored `SKILL.md` 最多 1 MiB；revision 仍是 scope/name/完整文档的 SHA-256 CAS，旧 handle 不兼容地失效。`skills.proposals.list/approve/reject` request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API 与 CLI 均不改变；`propose_skill.instructions` 的 internal Agent Tool schema 同步增加 1 MiB ceiling。

P160 只收紧 Runtime internal LSP document synchronization：进入 digest、`didOpen/didChange` 与 client open-state 的单文件最多 8 MiB，读取同时服从 caller cancellation 与 `limit+1` growth detection。LSP operation request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P161 只收紧 Runtime internal MCP remote catalog admission：每 connected server 最多 2048 个 tools，每个 description 最多 64 KiB 且为有效 UTF-8，每个 encoded input schema 最多 1 MiB；模型目录和 `mcp.tools.list` 管理目录都 fail closed，不返回截断前缀。MCP operation request/result shape、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P162 只收紧 Knowledge 完整文档准入：单份 home/projectRoot/cwd `SCOPEAPP.md` 最多 1 MiB，`knowledge.update` 在 persistence port 前拒绝超限内容并投影为 `invalid_params`，filesystem store 的 direct write 与外部文件 read 复用同一 Domain 上限；完整 cascade 不截断或跳过越界文档。Knowledge operation request/result shape、content-revision 格式、CAS/atomic-replace/recovery 语义、operation/feature/topic catalog、generated Desktop binding、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P163 只收紧 Lifecycle Hook 配置准入：单份 `hooks.json` 最多 256 KiB/128 条，global + project 完整级联最多 256 条；matcher 最多 256 bytes、command/inject 最多 8 KiB、command timeout 最多 5 分钟，配置文本必须是有效 UTF-8。`hooks.list` 与 fresh Run binding 对任一超限文件或级联整体失败，不截断、不跳过、不发布部分策略。Hook operation/request/result shape、trust key/active 语义、event/scope vocabulary、Protocol `2026-08-24`、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P164 收紧 Hook command 私有进程合同：stdout/stderr 各最多保留 64 KiB 且继续 drain；stdout 只能为空或一个 UTF-8 JSON object，只接受 `decision/reason/injectContext/rewriteArguments` 与 `allow/deny/ask`，unknown/trailing/malformed/overflow 输出作为可观察的 broken-hook failure，不贡献 decision。既有非阻断错误策略保持，exit code 2 即使 stdout 失效也继续 deny；Unix timeout/cancellation 终止整个 process group，返回时再次清理后代。该私有 shell contract 的严格化不改变 `hooks.*` Protocol shape、trust/event/scope 语义、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI。

P165 收紧同一私有 Hook command stdin 合同：Domain command projection 将 prompt/arguments/result/reason 类 material 分别限制在 256/256/128/8 KiB，prompt 与 result 只发布 marked UTF-8 prefix，arguments 必须 lossless；Shell 在进程创建前同时要求 raw material 与最终 JSON stdin 不超过 512 KiB。新增 `promptTruncated`、`tool.resultTruncated` 与 Subagent 对应 marker 只属于 private process JSON，不进入 ScopeApp Protocol。超界或非法 material 作为可观察的 broken-hook failure，不执行 command；declarative hook 仍独立生效。`hooks.*` Protocol shape、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P166 收紧现有 `agentDocs.list` / fresh Run AGENTS.md 与 `recipes.list` 的 authored-source 准入，不改变 wire shape：Application 统一规定每份完整文档最多 1 MiB/valid UTF-8，Agent document cascade 最多 64 份/4 MiB，Recipe 每 scope 128 份且完整级联最多 256 份/8 MiB；filesystem adapter 在 parse/materialize 前以 stat + cancellation-aware `limit+1` 复验，并以 1024-entry sentinel 限制 Recipe directory scan。现存 invalid/oversized source 整体失败；AGENTS.md 模型 projection 仍按 32 KiB 选择完整的 most-specific tail，但单份文档放不进预算时拒绝 Run，不静默省略。Protocol `2026-08-24`、operation/feature/topic catalog、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P167 收紧既有 Workspace VCS read semantics，不改变 wire shape。`workspace.changes.list` 的完整 catalog 最多 10,000 项；`workspace.diff.get` structured 结果最多 5,000 个完整文件、默认/最高 5,000 行与 64 MiB retained string material，`limit=0` 现在明确选择默认 5,000 行，超过预算只在完整文件边界返回 `truncated=true`，第一文件与 binary/zero-row 文件没有例外。Raw diff aggregate 最多 64 MiB，超界 changes/raw/process output 投影既有 `invalid_params` 并要求缩小 workspace/path；Git-backed workspace file listing 的既有 20,000-entry 合同现在在保留第 20,001 个 path 前失败。所有 Git stdout 限 64 MiB、stderr 只保留 64 KiB prefix 且继续 drain，watch fingerprint 有 10 秒 lifetime；external diff/textconv/pager 不参与事实生成，untracked symlink 不跟随 referent，binary 与 quoted path 保持无损。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P168 收紧既有 model-facing `read`/`apply_patch` 内部 Tool semantics，不改变 tool definition 或 wire shape。read-before-mutation stamp 现在对完整 regular file 做 cancellation-aware streaming SHA-256；删除撤销 stamp，创建/修改刷新 stamp，fingerprint 失败不再跳过 guard。Auto-format 只处理最多 8 MiB 的完整 input/output；Go/JSON 在进程内完成，Prettier 通过 stdin/stdout 运行且 stderr 只保留 64 KiB prefix 并继续 drain，只有验证成功后才 atomic replace。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework 与 CLI 均不改变。

P169 收紧既有 model/direct `read` 内部语义，不改变 Tool name、`path/start_line/max_lines` request 或 `content/start_line/end_line/total_lines/truncated` response shape。Runtime 最多准入 8 MiB regular file与 1 MiB line，默认一次结果最多 1 MiB 且只在完整行后停止；`EndLine` 是最后返回行，`Truncated` 对省略 prefix/suffix/result budget 均为 true。完整扫描验证 UTF-8/NUL、BOM/CRLF、读取增长与 caller cancellation；mutation stamp 只有在 Tool call 前后 8 MiB-capped streaming digest 一致时提交。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated Desktop binding、公共 Go API、Desktop source、Agent Framework、`tools/fs` 与 CLI 均不改变。

P170 收紧既有 `workspace.files.read/head` 语义，不改变 `ReadFileRequest`、`FileContent`、`GetFileHeadRequest`、`FileHead` 或 operation shape。`maxBytes=0` 现在选择 1 MiB 默认值，显式值最高 8 MiB；完整 `TotalLines` 扫描只接纳最多 64 MiB regular file，单行最多 8 MiB，并在 64 KiB buffer 上验证 caller cancellation、UTF-8/NUL、BOM/CRLF、trailing empty line 与读取期间增长。Application 对 port result 再验证 output bytes、text、window/content correspondence 与 truncation honesty。编辑器 read 可在有效 UTF-8 边界截断最后一行并设置 `truncated=true`；越界 range/资源返回 `invalid_params`，invalid text 返回 `unsupported_mime`。Head 默认 200、最高 400 行且不发布无标记 partial result。Desktop file view 现在对带行号导航请求目标前后各 200 行并消费响应 `startLine`；这是既有字段的真实消费，不是新 wire。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding shape、公共 Go API、Agent Framework、`tools/fs` 与 CLI 均不改变。

P171 收紧既有 `workspace.files.search` 行为，不改变 `GrepRequest` / `GrepResult` shape。`query` 是最多 64 KiB 且必须编译成功的 Go/RE2-compatible regex；`limit=0` 选择 100，显式值最高 1000。Searchable corpus 使用既有 ignore-aware 20,000-candidate file catalog，单个完整 UTF-8 regular file/line 最多 8/1 MiB，一次请求实际扫描最多 512 MiB；binary/invalid/oversized individual source 不属于 corpus，catalog/aggregate 超限映射 `invalid_params`。`Matches` 是最多 8 MiB 的稳定 whole-row prefix，`Total` 在同一次 complete admitted-corpus scan 上精确产生并可大于 prefix；Application 复验 direct port 的 count/material/path/line/order/text/query correspondence。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding shape、公共 Go API、Desktop source、Agent Framework、`tools/fs` 与 CLI 均不改变。

P172 breaking 收敛 model/direct filesystem search Tool contract；这不是 ScopeApp Protocol method。Tool 名 `glob`/`grep` 保持。两种 request 当前唯一 shape 都是 `pattern/path?/max_results?`，pattern/path 分别由 strict schema 限制，default/max result count 统一为 100/1000；glob pattern 是相对 selected path 的 case-sensitive segment-doublestar，grep pattern 是逐完整行匹配的 Go/RE2 regex。旧 glob `ignore_case` 与 grep `file_glob/file_type/ignore_case/multiline/before_context_lines/after_context_lines/output_mode` 完整删除，不接受 alias；case-insensitive regex 用 inline flag，文件集合与上下文分别组合 glob/read。Glob response 唯一 shape 为 `paths/total/truncated`，grep 为 `matches/total/truncated`，其中 total 来自完整 admitted corpus，truncated 精确表示 retained prefix。两者使用 canonical root confinement、ignore-aware 20,000-candidate catalog，并逐 row 计算 JSON representation，使最终 encoded Tool result 最多 1 MiB；grep 前置 scanner 仍服从 P171 的 8 MiB row result、8/1/512 MiB file/line/scan。Runtime 不再调用共享 `tools/fs` Glob/Grep 或宿主 `find/rg/grep`。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding、公共 Go API、Desktop source、Agent Framework、共享 `tools/fs` 与 CLI 均不改变。

P173 收紧 Runtime internal Skill source/lifecycle 行为，不改变 `skills.discovered.list`、`skills.library.list/archive/restore`、`skills.proposals.*`、`Skill`/`ManagedSkill`/`Page` wire shape 或 model Tool 名/schema。每个 project/user complete-list 当前最多 256 个 valid-name candidate、272 个 raw top-level entries；完整 `SKILL.md` 与 `read_skill_resource` 文件各最多 1 MiB。用户托管 active+archived 总量、approval、完整 list 与 idle sweep 共用 256-entry strict snapshot；`.usage.json` 最多 64 KiB/256 records。越界使用稳定 internal capacity/size error 整体失败，不分页、不截断、不回退共享 SDK 的 unbounded reader。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated binding、公共 Go API、Desktop source、Agent Framework、共享 `skills`/`tools/skills` 与 CLI 均不改变。

P174 breaking 扩展公共 Go surface，新增 `runtime/localruntime` deployment handoff package；`contract/go-api.json` 同批冻结 `ErrInvalidToken`、`Token.Value/Path`、`OpenToken` 与 `ReadToken`。Durable token 文件唯一合法内容为 43-byte canonical RawURL encoding of exactly 32 bytes，必须是 0600 regular file；reader 用 path/open `SameFile` identity 与固定 44-byte probe 拒绝 symlink、替换、增长、空白、padding 和 non-canonical encoding。Runtime executable 与 Desktop 共用该 package；internal HTTP `LocalToken/OpenLocalToken` 与 Desktop private parser 已删除，不保留兼容入口。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、generated TypeScript binding、Wails binding、Agent Framework 与 CLI 不变。

P175 只收紧 Runtime internal Knowledge crash-recovery 的资源语义：原子 stage sweep 从无界 `ReadDir(-1)` 改为 128-entry、caller-cancellable 的完整流式枚举，并由 architecture gate 禁止所有 Runtime production `os.ReadDir` 与 non-positive `File.ReadDir`。它不改变 `knowledge.list/get/update` wire、1 MiB document/CAS/revision、Protocol `2026-08-24`、公共 Go API、Artifact v23、SQLite epoch 82、generated binding、Desktop/Wails、Agent Framework 或 CLI。

P176 只收紧 Runtime internal Workspace Checkpoint：私有 Git command 统一进入既有 64 MiB stdout/64 KiB stderr `gitprocess.Run`，snapshot selection 固定为 20,000 paths/512 MiB current material，source alternates/index 分别为 64 KiB/64 MiB；raw `ls-files -z` path 不再 trim。内部新增稳定 `ErrSnapshotTooLarge` 供 owner 测试与错误链识别，但不进入公共 Go/Protocol。Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P177 只收紧 Runtime internal Sandbox command writer：stdout/stderr 各自继续完整 drain、最多保留 256 KiB 并使用既有 truncation marker；私有 storage 不再通过匿名嵌入暴露 `io.ReaderFrom`，因此 `os/exec`/`io.Copy` 不能绕过 bounded `Write`。Sandbox Tool output shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P178 只收紧 Runtime internal MCP stdio session teardown：`dial` 的 lifecycle release 现在是可报告错误的 cleanup；Unix command 在 Start 前独占 process group，context cancellation 与 session Close 后 cleanup 都终止整组后代，handshake failure、probe、replacement、detach 与 Host shutdown 共用该 owner。非 Unix 保持 direct-process termination。MCP config/operation/tool shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P179 只替换 Runtime internal Sandbox working-tree materialization：删除 in-memory tar pack/unpack，改为 source/destination `os.Root` 间的 64 KiB chunk copy；既有 100,000 entries、128 MiB/file、512 MiB aggregate 与 relative-symlink/mode 语义保留，并新增 opened identity/size、growth、cancellation 和 destination-inside-source fail-closed guard。Sandbox constructor/Tool output shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

P180 breaking 收紧 Runtime internal `infra/exec.Shell.Outcome`，增加 terminal process-tree cleanup error；唯一 Tool consumer 同批迁移，不保留旧三返回值 method。Unix Model Shell 在 Start 前独占 process group，timeout/stop/foreground cancel/natural leader exit/Host shutdown 都 group-stop 并 join；successful leader 的 descendant-held pipe 不改写其 exit code。Model-facing `shell/read_shell_output/stop_shell` name、request/result JSON shape、Protocol `2026-08-24`、86 methods/17 features/15 topics、Artifact v23、SQLite epoch 82、public Go/generated binding、Desktop/Wails、Agent Framework 与 CLI 不变。

`sessions.snapshot` 是挂载 Session material view 的命名用例，不是通用展开机制：Application 校验
Session/Item/Run/open Interrupt/Plan/Goal 的跨投影关系，并与启动恢复复用唯一 Pending projection closure；每个 waiting
Run 必须由 root Pending 拥有，每个 Interrupt 必须精确解析到同 Session/Run/Item/occurrence 与匹配的 Question/Approval
payload，running Item 必须由 active continuation 唯一认领，terminal Run 不得保留 running Item。Persistence 在一个 SQLite transaction 内读取全部事实，Delivery
按调用方 capability 原样投影或整体拒绝，不能裁剪 waiting set。Desktop 只走这一路恢复已挂载 Session 的 HITL、Plan、Goal、Run/Tool，
并且只有赢得当前 view generation 的响应可以提交整份 material；独立分页资源接口继续存在，未挂载 Goal 才继续由 `goals.get` 读取。该 additive method 不改变
`protocolVersion`、Artifact version 或 SQLite epoch，也不授权旧四读 fallback。
`manifest.methods[].materializes` 只声明复合 query 原子承载的独立事实族，供合同审计和 consumer gate 区分
服务端组合读取与孤儿能力；它不继承目标 query 的筛选/分页语义，也不建立 alias 或客户端 fallback。

Goal read model 的 `status:"completing"` 精确表示模型已声明 objective 成功、但 owning Run 的最终记账与条件清除尚未完成。它保持目标占位且不可 stop/resume/start；下一次 `goals.changed` 后读取 `null` 才表示 settlement owner 已释放。Domain `complete`、Application drive 与公共 `completing` 分属各层，不互相泄露类型。

Goal 管理面 additive 增加 `goals.update` 与 `goals.clear`，不改变 `protocolVersion`、Artifact version 或 SQLite epoch。
update 在 Application drive quiescence 与 Goal CAS 边界内替换 objective，并通过 fresh incarnation 隔离旧 Run provenance；
status/reason、model/capabilities、budget/usage 与 createdAt 不重置。clear 在相同 owner 边界内条件清除，目标已不存在时幂等成功。
两者都不建立 Frontend standing writer：挂载 Session 仍只用 `sessions.snapshot` 修复整份 material。

Knowledge 条目以内容摘要作为 opaque revision。`knowledge.list/get` 即使文件尚不存在也返回可用于首次条件创建的 revision；
`knowledge.update` 必须携带 `expectedRevision`，在 Infra 的同路径原子替换边界比较并返回 committed Entry，不匹配以
`revision_conflict` fail closed。Application 只在成功提交后发布 `knowledge.changed`；Hook trust 同理发布 `hooks.changed`。
三条 Knowledge operation 都将 physical document 越过 semantic scope root 投影为 `path_outside_root`。Infra 解析唯一 physical
identity 后才读写，域内 symlink 的 alias 本身保持不变；跨进程 directory lease 包围 revision compare、权限继承、临时文件 fsync、
原子 rename 和父目录 sync，cold read 回收严格命名的 pre-publish staging。进程崩溃后的可见内容只能是上一 committed revision 或完整
新 revision。
这些 topic 是失效事实，不携带配置值。Provider/model role、approval policy 与 agent-memory review 同样在所属 Application use case 提交后发布专用失效事实；Delivery 才将中性 notice 映射为 wire topic，Desktop Workspace events Adapter 再映射到各 context 公开 query identity，Agent Framework 零感知。

公共 Go surface 只有 `runtime/protocol`、`runtime/embedded` 与 `runtime/localruntime`，由生成的 `contract/go-api.json` 完整冻结。`protocol` 只公开 binding-neutral values、strict validation、版本、稳定错误 identity 与 `ProblemError`；`embedded` 只公开 concrete Runtime lifecycle、准确 options 和类型化 operation methods；`localruntime` 只公开 durable token 的 validated `Token`、`OpenToken`、`ReadToken` 与稳定 `ErrInvalidToken`，不公开 transport/server 或 host-directory discovery。同一 canonical data directory 可由另一个 embedded/HTTP Runtime 同时打开，因此旧的 `embedded.ErrDataDirectoryInUse` 已 breaking 删除；实际冲突在对应 Session operation 上投影既有 `session_busy`。服务端 method interface、request context plumbing、numeric JSON-RPC code、reflection shape walker、artifact catalogue、Host、Store、Engine 和 Router 均属于 `internal`，不构成公共 Go surface。P113 对 Assembly、operation、Interaction、Toolset、LSP、MCP 以及 Runs/Sessions/Runsegment constructor 的 breaking correction 只收紧 internal valid construction 与 lifetime ownership；P148/P149 先分离 terminal diagnostic、再按 SDK 合同纠正 MCP close，P150 删除失去生产消费者的 Retryable/settlement 双态并让 terminal Sequence 在失败 Assembly timeout 后继续完成逆序资源图，P151 让 Host 整体 shutdown generation 独立于 caller wait，P152 再让 Instance 以同一 owner 规则从 operation Endpoint 穿过 workers 加入 Host；公共/CLI Close timeout 不再遗弃下层图。P174 breaking 增加唯一 deployment handoff package 并删除 HTTP internal token owner；Protocol method/event、Artifact 与 SQLite shape 不变。

## 3. 持久化 Baseline 1

### 3.1 SQLite

- 当前 `schemaEpoch = 84`；Session、Run、Goal、Schedule 与 Schedule occurrence 各自持久化严格解码的 `reasoning_effort`，不允许 identity/effort 分裂或旧 epoch 迁移；
- P183 将 Runtime 稳定领域枚举直接持久化为其命名文本，并把 `session_permission_modes.mode/restore_mode` 从 INTEGER 改为 TEXT；旧 ordinal/数字字符串与新领域值不兼容，故一次性提升 epoch，不迁移、不双读、不保留 codec 映射表；
- P184 把 operation 注册/typed invocation 的 Go 1.27 泛型行为归还 `Registry` / `Endpoint`，让就近声明的 `operation.Name` 成为注册与 embedded binding 共用的唯一 method identity，并收紧 Hook verdict 与 Tool mutation scope 的 internal zero-value 边界；不改变 Protocol method text、Artifact 或 SQLite shape；
- P185 迁移到 Agent Baseline 31 的 `SchemaFor` owner并整体升级 Runtime/Desktop/Frontend 依赖图；不改变 Protocol method text、Artifact、SQLite、checkpoint 或公共 Go surface；
- `sessions.workspace_path` 是非空列；strict codec 先重建 Domain `Workspace`，相对、非 lexical-clean 或空路径均拒绝，旧 `sessions.cwd` 不读取；
- `sessions.provider` / `sessions.model` 是非空列；strict codec 只恢复 configured exact pair，Runtime 默认只在 Session admission 时安装，不在 reader/Run 层补写；
- `agent_memory_items.embedding_space` 与 `embedding` 是成对为空或成对有效的 search-derived cache；strict reader 拒绝空/半对、非 4-byte vector encoding 与非有限值。cache write 只在 exact item 仍 active 且 content digest 未变时提交；内容编辑同时清空 space/vector。epoch 80 的无空间裸 BLOB 不读取、不迁移；
- `agent_memory_items.content` 与 `agent_memory_ledger.fact` 各自最多 4096 个 Unicode code point；Domain constructor/normalizer、strict reader 与 fresh-schema CHECK 共用同一 owner constant。epoch 81 的无界 shape 不读取、不迁移；
- 数据目录为 `0700` 私有目录，可由少量同版本 Runtime 进程共享；schema/config setup 使用短期跨进程 lease，Runtime lifecycle 不拥有目录全局独占权；
- SQLite 事务与既有 uniqueness/CAS 继续拥有 durable winner。活跃 Session writer、physical working-tree shared/exclusive operation、Goal drive 与 ordered recovery sweep 使用 OS advisory lease；进程死亡由内核释放。单一 recovery winner 固定 Run-before-Goal 并只清理成功接管的 Session，不使用 TTL、heartbeat、全局 checkpoint/callback sweep 或兼容双路径；
- 其他 SQLite connection 的 commit 只触发全量 read-model resync，细粒度本地 invalidation 仍由提交用例发布；该同步机制不拥有 SQLite epoch、Artifact、checkpoint 或 protocol wire shape；
- `runtime_identity` 的单例 opaque namespace 与同一 durable idempotency replay store 共存亡；保留数据库重启不变，删除/重建同路径数据库必须变化，且不暴露数据库路径；
- Goal aggregate 与 Goal terminal ledger 使用 `incarnation_id`，Run/Interrupt provenance 使用 `goal_incarnation_id`；已退休的 `lease_id`/`goal_lease_id` 列不存在且不双读；
- Goal aggregate 还持久化 fresh Start 时协商并冻结的 canonical Run capabilities；Goal Resume 的调用方能力必须覆盖该集合，自治 Run 与 Goal 内 `create_goal` 都继承相同集合；
- executor checkpoint 与 pending interrupt 的技术身份列为 `root_member_id`；continuation/input-request binding JSON 使用 `memberId`/`requestId`，approval binding 额外持有 exact `toolCallId`，使 edited-arguments replay 不按 name/args 猜 ToolCall identity；
- `model_invocations` 与 `tool_invocations` 是 operational attempt journals，只保存 exact Run/Segment/call identity、state 与 started/finished time；semantic assistant final、Tool result、累计 usage 与最新权威 prompt footprint 仍只由 Transcript/Run owners 保存；`runs.context_tokens` 以 `0 = unknown` 保存该可回落的 footprint，不能从累计 input usage 推算；
- `runs.commit_segment_id` / `runs.commit_id` 保存当前 Run 最近一次完整 Application command write-set 的 opaque 技术回执，覆盖 fresh/resume opening、顶层 `EventCommit`、HITL answer claim、HITL tree barrier、waiting-child cancellation 与 terminal boundary；单 Run pump/command owner 在收到结算前不会发出下一笔 canonical command，因此 latest marker 足以核验 SQLite 已 COMMIT 但 success receipt 丢失的完整事务。Running marker 必须属于 exact active Segment，Waiting barrier 与 terminal 保留生产它们的 Segment；尚未打开 continuation Segment 的 answer claim，以及已经 Waiting 且不打开新 Segment 的 child cancellation，都以 empty Segment + unique command identity 表示，不能伪造 Segment。普通 Suspend、Resume、Restore 与 recovery 不沿用旧代 marker；
- `interrupts.state` 只有 `open`/`resuming`：`open` 不得携带 answer/claimedAt，`resuming` 必须携带两者；普通列表/读取只返回 `open`，continuation opening 必须在事务内证明 exact root 的 `resuming` claim；
- `child_run_start_reservations.payload` 是 `adapter/runsegment` 显式拥有的 canonical JSON，只保存没有独立列的 reservation facts；SQLite 不解释 payload，Application Go 结构体布局不是 durable wire。reservation/conclusion 只在 owning root tree 与当前进程仍可回调时保留；root terminal、parked terminal、rollback/restore/delete 在原 write-set 内按 Session 回收，boot recovery 在公共 Run 修复同一事务内清空上个进程的 callback ledger；
- 下一 quiescent barrier 只能由相同 Session/executor/root-member owner 替换 `resuming` row；terminal 与 recovery write-set 删除该 row。不存在 open row overwrite、answer rollback、dual state codec 或兼容列；
- Tool start 不占用 Transcript insertion order；同一 model Tool batch 的 completed Items 与 invocation terminals 按模型声明位置形成一个 canonical write-set；
- 一个 build 只接受一个精确 epoch；
- 没有运行时 migration chain、dual schema read 或 compatibility column；
- 重构产生 shape 变化时直接提升 epoch，并同步 fresh-schema tests、store codec、contract expectation 和本基线。

### 3.2 Executor checkpoint

当前 checkpoint 的产品语义是 Host envelope + opaque Agent Framework complete-tree payload。生产 Bootstrap 只保存 Agent Framework public TreeSnapshot v4 JSON，由 `adapter/agentexec` 唯一解释；Application/Store 不分支解析。

目标合同：

- Application owns checkpoint identity、BuildID、Session/Run identity、model selection、limits、capabilities、accounting 和 child Run binding；
- `adapter/agentexec` owns Agent Framework TreeSnapshot encode/decode/restore；
- payload 对 Application、Domain、Delivery、SQLite 和 Protocol 不透明；
- root Process tree 是 executor payload 的不可拆分恢复单位；
- checkpoint replacement 只能推进 frozen identity/limits 和 monotonic usage；
- terminalization 与 checkpoint deletion 由 Application write-set 原子决定。

P7 延续的 continuation payload baseline 是 Agent Framework TreeSnapshot v4 本身，不再包一层 Runtime 自创 payload version。Agent Framework public parser 校验 snapshot version/shape，exact DeploymentRef 校验策略实现与配置，Host BuildID 校验当前二进制/adapter expectation；任一不一致都 fail closed。Host envelope 的技术 codec 仍由 Runtime 当前唯一 SQLite epoch 拥有；当前 executor checkpoint policy schema 为 v2，并只接受 `goal_incarnation_id`。

### 3.3 Artifact 与 Transcript

Artifact、Transcript Item 和 ToolCall timing 的当前机器 shape 仍由 Runtime contract/store codec 拥有。Session Artifact 当前唯一版本为 24；其他版本在任何写入前确定性拒绝，不从旧 artifact 猜测缺失事实或改写版本号。Artifact 以显式 `plan` steps 携带 Plan 语义，并以必填 provider/model 与可选 `reasoningEffort` 保存 Session 的精确模型选择；每条 Artifact Run 也保存自己的 frozen exact selection。它不携带源 Runtime 的 revision/timestamp。Artifact Run 保留最新权威 prompt footprint，因此导入前后 Context gauge 的事实一致。AgentMessage 的 `phase` 是 Runtime 在模型调用边界写入的过程说明 / 最终回答语义，并随 Transcript、Artifact 与客户端恢复保持一致。Question Item 的 `answers` 是唯一已接受响应；未回答或取消保持字段缺失，claim 成功时与 pending/checkpoint 变更同事务写入 Transcript。ToolCall 的 `approvalDecision` 是该调用实际接受的人类决定，和 Pending consume/checkpoint invalidation/commit receipt 同事务写入，并随续跑终态与 artifact 保留；自动放行不伪造。ToolCall lifecycle 与可选 exact execution duration 是两个事实：后者排除审批等待，无法证明时保持 unknown。Tool failure taxonomy 将工具所属 Run 的取消导致的在途终止表示为 `toolCanceled`，不与执行失败、审批拒绝或父 Run 上的 `childRunCanceled` 合并。

## 4. Agent Framework 消费 Baseline

Runtime 使用 Agent Framework [`API_BASELINE.md`](../../../agent/doc/API_BASELINE.md) 的 Baseline 33 canonical `agent/v0.2.0` module。P8 已把 P4–P7 验证的 root start/result、authoritative model/tool、waiting/restore/answer/steer、managed Delegate child 和 prepared waiting-subtree合同切为生产 Bootstrap 唯一 owner，P11 完成 canonical module path 替换；Core 六个模型模态的产物词汇现统一为 `Output`，其中 Interaction 消费唯一 `chat.Response.Output`。P188 进一步消费调用前 `ModelContextReducer` 与普通 Tool 的 exact `ToolInvocation.ModelResult`；durable store、transaction、模型窗口、产品事件和 client presentation 仍由 Runtime owner持有：

- root Kernel、Agenttest、Interaction、Planning、Planning/GOAP、Workflow、Platform 七个 public package 已冻结；OpenTelemetry adapter 由集成层 `otel/agent` 拥有；
- Process Snapshot v6、TreeSnapshot v4；
- Interaction state/protocol v8/v8；
- context-aware ProcessAdmitter、conclusive ProcessStartOutcome、提交式 `RequestCancellation`、带 exact applied-steer Signal identity 的 ModelInvocation、ToolInvocation、DelegateChildKey、ActiveDelegateChild inspector、DeferredTools/AdvertiseTools 与 contextless PreparedWaitingSubtreeCancellation Apply 已存在；
- Agent Framework Event 是 Framework 已发生事实，Delta 是 best-effort 临时输出；
- Strategy payload 和 TreeSnapshot private state 对 Runtime 不透明。

Runtime 只把 Agent Framework public API 当合同。原框架实现、Agent Framework tests/private wire、当前 `agentexec` API 都不是兼容基线。

P7 的两个前置缺口已经由真实 Runtime consumer 在 Agent Framework 中以 Framework-neutral 合同关闭：accepted admission 通过 prospective identity 的 started/aborted outcome闭合；waiting subtree 通过 one-shot prepared capability 持有同一 safe cut，全部 fallible staging 位于 Prepare，durable commit 后只调用 contextless Apply。Run、Store、transaction、产品 ID 和 private tree wire均未进入 Agent Framework。

`PreparedStepAcknowledger` 仍只回调单 Process Snapshot，Runtime 初版不启用。durable recovery baseline 只有已提交 quiescent complete-tree checkpoint；active-step crash 不伪装为可恢复。

Runtime 的 executor-owned opaque checkpoint envelope 当前 schema 为 v5：除完整 TreeSnapshot、指令上下文、accounting、exact model/reasoning selection 与 Agent steer Signal identity 到产品消息内容的精确映射外，还保存每个已accounted Process最近一次完整主请求的provider-reported/Runtime-estimated token pair，并为 Resume input 保存已由 opening 创建的 exact projected Item identity；恢复后 Conversation 仍会在安全边界追加一次，但 Transcript 不会重复创建 Item。Application 仍只持有 opaque bytes；Agent Framework 不见 Transcript 内容、token估算策略或 Runtime persistence。旧 envelope 不双读，恢复时确定性 fail closed。

### 4.1 允许的 import 边界

目标 production allowlist：

```text
internal/adapter/agentexec/** -> agent, agent/interaction
```

只有真实接入 Planning/Workflow/OTel/Platform 时，才分别通过 ADR 增加精确 package edge。默认禁止：

```text
internal/domain/**      -> agent/**
internal/application/** -> agent/**
internal/delivery/**    -> agent/**
internal/infra/**       -> agent/**
internal/adapter/toolset/** -> agent/**
```

临时 Agent module import 已永久禁止；不存在迁移 allowlist。

## 5. Application/Agent 防腐基线

### 5.1 Application 可以表达

- Run execution start/observe/cancel；
- opaque executor root/member identity；
- opaque checkpoint payload 和 Host expectation；
- product Interrupt/answer、SteerRun；
- child Run admission facts；
- waiting subtree 的应用事务输入/结果；
- executor lifecycle facts 和稳定 product outcome。

### 5.2 Application 不得表达

- Agent Framework `Process`、`Execution`、`Deployment`、`Signal`、`Effect`、`WaitID` concrete types；
- `TreeSnapshot` field、ExecutionState payload、Interaction phase 或 mailbox；
- arbitrary Signal submission；
- model/tool lifecycle 从 Delta 推断；
- Agent Framework Engine/Dispatcher/Resolver handle；
- arbitrary EffectID/Settlement/ResolveEffect endpoint；
- Framework Store、transaction 或 product metadata extension。

Unknown Effect 的产品合同是 live/recovery 一致的 fail closed：Application/Delivery 不得到 Settlement payload 构造权；agentexec 只向 Application 投影 indeterminate executor fact/identity。RunLost write-set 提交前 Process 保持 unknown wait，提交后才 Kill/release。

P8 已冻结生产 executor port：`RootExecutionStarter` 负责 validate/stage/begin，`ExecutionObserver` 负责只读事实流，`RunningRootCancellationRequester` 只提交 Framework cancel request，`ExecutionReleaser` 只负责 resource lifecycle；`WorkingContextComposer` 在 Application 边界组装完整 fresh-root context。Application opening durable 后 Begin 才 Start Process；cancel intent durable 后请求停止，pump 继续观察到确定终态才 release。

P8 已冻结 authoritative model/tool 合同：executor producer 只能通过同一有序 observation stream 提交 Application-owned closed fact 并等待 receipt；它不取得 Store、transaction 或 reducer。Application Run pump 在 speculative reducer 上计算 write-set，只有 persistence 全部成功才替换 live reducer并完成 receipt。model/tool post-call receipt failure 必须返回 Agent Framework Dispatcher 形成 unknown；pre-call failure 禁止外呼。Toolset 的唯一 visibility value 是 framework-neutral `toolset.Manifest`，通用 Toolset 对 Agent Framework 零 import。

P8 已冻结 continuation 合同：`WaitingExecutionContinuer.StageContinuation` 只 stage 一棵 exact live waiting tree，或按 opaque TreeSnapshot + exact Deployment/BuildID/Host scope 恢复；它不读取 Conversation，也不重算 WorkingContext。Application 先原子记录 exact answers、隐藏 interrupt row 并删除旧 checkpoint，再 stage/restore；next-Segment opening transaction 必须证明 durable `resuming` claim，成功后 `BeginContinuation` 才投递 WaitID-addressed semantic Signal。claim 后到下一 quiescent checkpoint 前没有 fallback recovery point，crash/boot recovery 一律 `RunLost`。

Product Interrupt/prompt/answer 使用 framework-neutral strict codec；`interactioninput` ACL 是唯一把它映射到 Agent Framework pending-input/Signal 的 owner。旧 private suspension adapter 已删除。真实 Runtime `ask_user`、interactive approval、deferred advertisement restore 与 steer 均走唯一生产路径。

P8 已冻结 child/subtree 合同：Delegate ToolCall authoritative commit 先于不可见 child start reservation；Agent Framework conclusive started 后才公开 child Run，aborted 只闭合 reservation。多 child、嵌套 child与乱序 sibling completion 使用稳定 parent/model-call/tool-index 因果顺序；恢复归因只调用 Interaction owner 的 typed inspector。waiting child cancellation 执行 prepare → application transaction → contextless Apply/Discard；移除最后边界时，Apply 只安装 resulting state，独立 Continue 才激活已提交 Segment。Apply 异常先释放旧 owner并由 `WaitingExecutionRestorer` 从 committed resulting checkpoint 精确恢复，恢复失败才 RunLost。

P8 已冻结这些内部消费端口及防腐语义：Application 单写者、operational journal 与 semantic Transcript 分离、final 独立于 Delta、并发 Tool canonical prefix 原子提交、unknown 在 release 前 durable `RunLost` 收口、answer claim → stage/restore → durable opening → semantic Signal，以及 Delegate reservation → conclusive start → public child Run 的唯一顺序。

Fresh root input 的防腐合同是 Application `WorkingContextComposer` 读取 Host Conversation 并追加当前 user message，再组装 Knowledge、Plan、Memory 与 hooks，形成完整 `WorkingContext` seed；agentexec 不读取产品 Store。成功 assistant final 由 Agent Framework Result 投影 `AssistantMessageCompleted`，不从 Delta 拼接。

WorkingContext 的来源归因属于 `adapter/agentexec` 私有合同：base prompt、Knowledge、pinned/recalled Memory、AGENTS.md、Plan 与 lifecycle hook 只在实际注入后以 versioned JSON-safe Message/Part metadata 标记，并区分 instruction/data purpose。该 metadata 随 opaque Interaction checkpoint 自包含恢复，但不进入 Runtime Protocol、Artifact/SQLite schema、Application port 或 Agent Framework 公共类型；公共诊断若出现必须另行设计安全投影。

P23 进一步冻结该私有合同的行为所有权：context source kind 唯一决定 purpose 并在 metadata 写入前验证；预算后的 Memory/Agent document prompt fragment 同时持有可见文本与来源；hook result 负责 block/injection/provenance 应用；`WorkingContextComposer` 负责完整 system/Plan/recall/hook 编排。该内部重构不改变 metadata JSON shape、prompt 文本或任何公共/持久合同。

Application executor tree identity 统一为 `ExecutorMember`/`MemberID`。Framework `ProcessID` 只能由 execution adapter 在边界内映射，不能重新进入 Application field、port 参数、持久化 technical field 或 Runtime Protocol。

## 6. Clean Architecture 边界基线

目标允许边：

```text
domain      -> stdlib + approved stable values/pure domain strategy contracts
application -> domain + consumer-owned ports + approved observation API
infra       -> domain immutable values + technical SDKs/mechanisms
adapter     -> application + domain + infra + capability SDKs
delivery    -> application + domain projection values + protocol/transport
bootstrap   -> config + application + adapter + infra + delivery
cmd         -> bootstrap + config
```

目标禁止边：

```text
domain      -/> application|adapter|infra|delivery|bootstrap
application -/> adapter|infra|delivery|bootstrap|Agent Framework|driver SDKs
infra       -/> application|adapter|delivery|bootstrap
adapter     -/> delivery|bootstrap
delivery    -/> adapter|infra|bootstrap|Agent Framework
all rings   -/> bootstrap composition objects
```

同一 ring 内的 package 仍必须形成 DAG；不能用同层身份为循环或 god package 辩护。

## 7. 自动守卫

P1 已建立：

- production target package DAG，以及会被稳定拒绝的 Delivery → Adapter 反例 fixture；
- Agent Framework importing leaf 与 imported public package 的双重 exact allowlist；
- 临时 Agent module import 的永久 absence guard；
- Domain、Application 与 Delivery 既有 external SDK denylist；
- context-based Domain I/O port、旧 private snapshot decoder 和旧 lifecycle owner 的永久禁止守卫；
- compatibility/legacy/versioned source directory 禁令。

机器 owner 是 `internal/arch/target_architecture_test.go`、`internal/arch/framework_boundary_test.go` 与各专项 architecture fitness test；不存在 temporary exception 台账，本文件不复制易漂移的逐文件集合。

P2–P10 已建立：

- protocol artifact digest/drift test，以及 canonical sample 同类型 strict `ValidateWire` gate；
- SQLite schema epoch 和 prior-version rejection test；
- checkpoint envelope strict codec、size、copy、round-trip 和 prior-version rejection（P6 已覆盖 Agent Framework TreeSnapshot parser、copy、corrupt/wrong-build/deployment；P8 随 production owner 收口剩余 envelope guard）；
- Agent Framework type/name leakage AST guard；
- no `component/common/core/utils` package guard（P18 将根级 cross-ring purity allowlist 与 Application mechanism owner guard 分开冻结）；
- no alias/dual codec/legacy path guard；
- exported contract GoDoc/parameter/error wrapping guard where the contract is intentionally frozen。

## 8. 不在 Baseline 1 中

- Runtime 对 Agent Framework Platform 的接入；
- 前端/TUI/CLI 新 consumer API；
- Delivery `server`/`dispatch` 保持现名；未由真实职责变化证明时不做目录改名；
- 未来数据库 epoch、artifact version 或 Agent Framework TreeSnapshot version。

这些内容不能以 placeholder、预留字段或空接口提前进入代码；真实阶段完成后再冻结。
