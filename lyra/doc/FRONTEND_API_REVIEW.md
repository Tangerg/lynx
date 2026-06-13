# 前端 API 体验评审 —— 交互参数设计 / 同类对照 / 能力缺口

> **日期**：2026-06-14。**视角**：架构师 · 前端开发体验(DX)。**范围**：Lyra Runtime Protocol —— 后端经 JSON-RPC 2.0 + HTTP/SSE 给 Wails/Web 前端的**交互 API**,重点是**请求/响应参数形状**(wire DTO、字段命名、可选性、ID/分页/版本约定),而非纯传输架构。
> **方法**：通读现行协议代码(`internal/delivery/protocol/*.go`,实测 `protocolVersion = 2026-06-07`)+ 前端契约(前端仓 `docs/protocol/{API,TRANSPORT,AUX_API}.md`)+ 前端 UX 研究(`docs/ui-research/`,6/14)+ 我们已有的 `AGENT_CAPABILITY_COMPARISON.md` / `PROTOCOL_ALIGNMENT_REVIEW.md`,对照 9 个同类(opencode / OpenHands / plandex / cline / claude_code / codex / continue / assistant-ui / crush)。
> **优雅范本**:以 **Stripe** 为参数设计标杆(日期版本、前缀 ID、游标分页、幂等键、可展开对象、统一错误信封)。
>
> **结论先行**：
> 1. **参数设计已是 Stripe 级**:日期版本、前缀 ID、游标分页、幂等键走 header、单一 `type` 判别、偏改用指针语义、对称可往返——Stripe 那套约定我们几乎全中(§2 记分卡)。唯一缺的是**字段展开 / 稀疏字段集**(`expand[]`),但对本地 loopback、浅对象的 runtime 是 YAGNI。
> 2. **传输/HITL/可观测性是同类里最好的之一**(streamable HTTP 单流、R 模型、durable/ephemeral 不变量、OTel 三驾马车)——领先 OpenHands(SSE+WS 双通道)、cline(同步阻塞 gate)等(§4)。
> 3. **2026-06-07 迁移已把旧 wire-shape 债清干净**(通用工具信封 / `kind`→`type` / 开放 features map / 统一 `Page[T]` / durable 按 type 推断)——`PROTOCOL_ALIGNMENT_REVIEW`(6/8)列的 S1-S6/G1-G3 基本都已落地(§2.3 实测核对)。
> 4. **真正的体验天花板不在参数设计,在两个缺口**:**(A)** 协议层零契约测试,wire↔前端 API.md 全靠人肉 review 防漂移;**(B)** 一批前端急需的能力(codeintel / 文件浏览 / 审批模式 / 压缩 / 任务清单 / MCP 鉴权)后端内部已有、但还没接成 RPC(§5)。

---

## 0. 范围与读法

你关心的"API 优雅度"= **前端开发者每天要拼的请求体、要解析的响应体长什么样**。所以本文不重复传输机制评审(那部分见 `MICROKERNEL`/`GREENFIELD_ARCHITECTURE`),而是逐字段审视**参数 DTO 的设计品味**,再把它放到同类与 Stripe 的坐标里。

---

## 1. 一句话结论

**参数设计本身已接近最佳(Stripe 级),不需要"重做得更优雅";体验上限被"能力还没接出线"和"没有防漂移测试"卡住,而不是被参数设计卡住。** 想再上一层,做 §5 的两个缺口,而不是翻修 §2/§3 已经很好的参数。

---

## 2. 优雅 API 的判据(Stripe 范式)× 我们的记分卡

### 2.1 记分卡

| Stripe 式优雅判据 | 我们 | 证据(现行代码) |
|---|---|---|
| **日期版本号**(`2023-10-16` 式,非 `v1/v2` 语义) | ✅ | `const ProtocolVersion = "2026-06-07"`;`runtime.initialize` 协商,版本不符是唯一硬断点 |
| **前缀化、不透明、自描述 ID** | ✅ | `run_…` / `evt_…`(SSE id = eventId);`RunRef.id`/`sessionId` 一致 |
| **游标分页**(统一 `limit` + cursor + has-more) | ✅ | `Page[T]{Data, NextCursor}`,覆盖 sessions/items/runs/interrupts/providers/models/tools/workspace/memory;局部有界列表 `NextCursor` 留空 |
| **幂等键走 header,不进 body** | ✅ | `StartRunRequest` 注释明确"no client-supplied runId;幂等是 `X-Idempotency-Key` header" |
| **统一错误信封** | ✅ | `ProblemData{type, channel, detail, retryable, retryAfterSeconds, errors[]}`,按 symbolic `type` 分支(非数字码) |
| **单一判别字段**(一个 `type`,不混 `kind`) | ✅ | 协议层 `grep json:"kind"` **零命中**;Item/Event/Outcome/Interrupt/ContextItem 全 `type` |
| **偏改语义清晰**(partial update) | ✅✨ | `UpdateSessionRequest` 用 `*string`/`*map` 指针:nil = "不动该字段"——比 Stripe 的"凭出现与否"更显式 |
| **对称 / 可往返** | ✅✨ | `DroppedRun.UserInput` 与 `StartRunRequest.Input` **同形**(rollback 后零转换回填 composer);`SessionArtifact` export↔import 忠实往返 |
| **传输元数据不进 body** | ✅ | trace(W3C `traceparent`)/ auth(`Authorization`)/ 续连(`Last-Event-Id`)/ 幂等键 全走 header |
| **能力可加不破契约** | ✅ | `features map[string]FeatureFlag`(开放 map);`streamingMethods`/`events`/`interruptTypes` 双向声明 |
| **可展开对象 / 稀疏字段集**(`expand[]` / `fields=`) | ⚠️ 无 | 但对象浅、本地 loopback、`items.list` 已按 runId 收窄——**YAGNI**,不建议为优雅而加 |

**11 条判据命中 10 条**,其中 2 条(偏改指针语义、对称回填)**比 Stripe 更显式**。结论:**参数设计已是 Stripe 级,且在两处更优。**

### 2.2 已经做对的参数细节(值得保持的品味)

- **资源以自然键为身份,不滥发不透明 id**:`Project` 直接以 `cwd` 为身份(无 id、无 active 标志);`Session.cwd` 是身份而非属性——relocate 是属性更新,不是新建资源。这是 REST/资源导向的正解。
- **显式优于推断**:`runs.start{provider, model}` 必须**成对**(缺一即 `invalid_params`),provider 永不从 model 反推——消除"看 model 猜 provider"的歧义。
- **乐观渲染对账走业务字段**:`StartRunResponse.UserItemID` 让前端用 id 精确对账乐观气泡,而非按内容匹配;明确标注"是业务字段,不是传输元数据"。
- **诚实标注未完成的语义**:`RememberScope` 注释直言"v1 只认 `session`;`project`/`global` 没有持久化家,发了也只给 one-shot,不做假承诺"——不留"看起来支持其实不支持"的陷阱。
- **能力 gating 显式**:`features.relocate`/`checkpoints`/`sessionExport` 等,前端按 bit 决定 UI 是否出现。
- **非错误终态也带 detail**:`RunOutcome.Detail` 让前端区分"用户取消"vs"超时"vs"超预算 $X/$Y",而错误终态的 note 留在 `Result.Error.Detail` 不重复(单一来源)。

### 2.3 旧债已清(实测核对,非 6/8 review 的过期结论)

| `PROTOCOL_ALIGNMENT_REVIEW`(6/8)列的债 | 现状(2026-06-07) |
|---|---|
| S2/G1 ToolInvocation 强变体丢 `name`+raw args | ✅ 通用信封 `{Name, Arguments map, Result}`("not a union",`internal/delivery/protocol/items.go`) |
| G3 `kind` vs `type` 双判别 | ✅ 协议层零 `json:"kind"` |
| G2 features 闭合 struct | ✅ 开放 `map[string]FeatureFlag` |
| S4 `RunEvent.Durable` 冗余 | ✅ 仅 `custom` 自声明,first-party 由 `IsDurable()` 按 type 推断 |
| S5/P0.1 分页不统一 / 静默截断 | ✅ `Page[T]` + `NextCursor` 全覆盖 |
| S1 question interrupt 需 JOIN | ✅ interrupt payload 自包含(`Interrupt.Payload`) |
| S3 SearchResult 过度合并 | ✅ 已拆 |

> 即:那份 review 的修复清单基本执行完了。**不要据 6/8 的 review 再去"修"这些——它们已经是对的。**

### 2.4 比 Stripe 更优雅?——"最优雅"是形状相对的

Stripe 是 **CRUD-over-REST 开发体验**的天花板,但我们的 API **只有少部分是 CRUD**(sessions / providers / memory)。重心其实是**有状态 + 流式 + 长运行操作(LRO)+ 能力协商**的协议——这是另一种形状,它的优雅天花板由 **LSP / MCP / Google AIP** 定义,**不由 Stripe 定义**。而我们恰好**血缘上就源自 MCP/LSP**,已经坐在那个天花板上。所以"有没有更优雅的"要按坐标分别看:

| 参考设计 | 它在某一轴上比 Stripe 更优雅的点 | 对我们 |
|---|---|---|
| **LSP / MCP**(我们的血缘) | 能力协商握手 + request/notification 二元 + progress / partial-result token | ✅ 已具备(`runtime.initialize` 协商、durable/ephemeral、per-run 事件流);**可借鉴**:统一的 **progress-token 约定**让任意长任务报进度,而非只在 run 事件里 |
| **Google AIP — LRO**(AIP-151) | 长运行操作建模为 Operation 资源(`done` / `metadata` / `response` ∣ `error`) | ✅ 我们的 `run` + `RunOutcome` **就是领域特化的 LRO**——AIP 反过来印证了设计 |
| **Google AIP — field mask**(AIP-134/157) | **一套机制**同时表达**偏改**(`update_mask`)与**稀疏读**(`read_mask`),比 Stripe `expand[]` 更一般 | ⚠️ 比我们"指针偏改 + 无 expand"理论上更一般;但对象浅、本地 loopback → **YAGNI**,指针偏改已足够优雅 |
| **Temporal**(start / signal / query / result) | `signal`(不中断地注入)、`query`(无副作用读运行态)是极干净的原语 | ◐ `runs.resume`≈signal;B13a `runs.steer`=signal;**缺一等的 "query 运行态"**(现以 `items.list` / `state.snapshot` 代偿,够用) |
| **类型态 / 让非法状态不可表达**(Rust/Haskell ethos) | 非法参数组合在类型上**根本无法构造**(不是靠注释约束) | ◐ **wire 契约(TS)已是干净判别联合**;Go 侧扁平 struct(`InterruptResponseValue` 等可表达 `type:approval`+`answers` 这种非法组合)是**有意的 no-codegen 取舍**——wire 优雅、Go 务实 |
| **GraphQL / JSON:API** | 客户端精确选字段 / 严格统一资源信封 | ✗ 更重、更啰嗦;对单客户端 loopback agent 过度,**不取** |

**结论**:沿"我们这种形状"的坐标,**没有把优雅留在桌上**——能力协商、LRO、durable/ephemeral、判别联合,该有的都在,且血缘正统。理论上"更优雅"的只剩两条:**field mask**(YAGNI,浅对象不值)与**真正的 sum type**(Go 限制,但 wire 上已是干净联合)。Stripe 之外**唯一值得长期内化的一条原则是"让非法状态不可表达"**——它已落在 wire 的 TS 契约上,Go 侧的扁平是不做 codegen 的自觉代价(本仓既定决策)。换句话说:**该向 Stripe 学的我们学到了,该超越 Stripe 的地方(流式/LRO/能力协商)我们走的是更对口的 LSP/MCP/AIP 血脉。**

### 2.5 旁路 API 已对齐核心(2026-06-14 落地)

把旁路 API 提到核心同一条 typed-enum + 复用 PageQuery 的纪律线上,**全非破坏**(`type X string` 与 struct embed 的 wire JSON 一字不变),三批落地:

- **输入闭合枚举 typed 化** + `Valid()`:`DiffMode`(worktree|base)、`DiffFormat`(rows|raw)、`FeedbackRating`(positive|negative)——对齐 `RunMode`/`RestoreType`/`ExportFormat`/`MemoryScope`。
- **响应 status 枚举 typed 化**(对齐核心 `SessionStatus`/`RunStatus`,无 Valid):`FileStatus`(WorkspaceFileChange/FileDiff/FileEdit 共用一套词汇)、`McpStatus`、`McpAuthStatus`、`WorkspaceEventType`。
- **复用 `PageQuery`**:`ListModelsRequest`/`MCPListToolsRequest`/`ListRunsRequest`/`ListOpenInterruptsRequest` 从手搓 `cursor`/`limit` 改为 embed `PageQuery`(对齐 `WorkspaceListQuery` 范式,消除分页字段漂移)。
- **有意未做**:`ProviderType`(适配器集开放,不宜闭合 Valid)、`AgentDoc.Scope`(需 `MemoryScope→Scope` 共享改名)、`Skill.Source`(取值模糊)——低价值,留作触发条件。

至此旁路 API 与核心 API 在参数纪律上**一致**:枚举皆 typed、列表皆 `Page[T]` + `PageQuery`、判别皆单 `type`、能力皆经 `features` map gating。

---

## 3. 仍可打磨的参数小项(低优先,非缺陷)

这些不影响"优雅"判定,是锦上添花,**不急、可不做**:

1. **`InterruptResponseValue` 是宽联合**:一个 struct 塞了 approval/answer/toolResult 三态的全部字段(`Decision`/`Answers`/`Result`/...),靠 `Type` 判别 + `omitempty`。Stripe 风格会更窄;但 Go 没有 TS 判别联合,扁平 struct + 注释标注每态用哪些字段是**务实的本仓 idiom**(已在注释里写清)。**保持。**
2. **`ContextItem` 同理**(file/selection/url/image 四态一个 struct)——同上,保持。
3. **`Range []int`(`[startLine, endLine]`)用数组而非 `{start,end}` 对象**:紧凑但弱自描述。Stripe 会用对象。**可改可不改**,数组在注释标了 1-based inclusive,够用。
4. **`Metadata map[string]any` 无 schema**:开放逃生舱,正确;但缺"前端可写哪些 key"的约定文档——建议在 API.md 补一句命名约定(如 `ui.*` 前缀归前端)。

> 判据:以上都是"窄联合 vs 扁平 struct"的取舍,**本仓 Go idiom 选扁平 + 注释**,与 CLAUDE.md 一致,不构成债。

---

## 4. 同类对照(你问的"他们怎么给前端供 API")

### 4.1 前后端是否分离 + 各自的 API 形态

| 项目 | 分离? | 给 UI 的 API 形态 | 会话/参数模型 | 对我们的启示 |
|---|---|---|---|---|
| **opencode** | ✅ server+多客户端 | HTTP REST + 自定义流 | Session 对象;暴露 9 个 LSP 操作 | LSP 接成 RPC 是值得抄的能力面(→ §5 B7) |
| **OpenHands** | ✅ Python 后端 + React | REST + **SSE + 另一条 WebSocket** | 事件溯源(event 判别联合,全 durable + 重放) | 双通道更复杂;但**condensation-as-event**(压缩=产生不可变事件)优雅(→ §6) |
| **plandex** | ✅ Go server + CLI | HTTP REST / SSE | plan + 阶段流水线(非 tool-loop) | 刚性流水线**不抄**——我们的 tool-loop 更适合探索 |
| **cline** | ❌ 编辑器内 | VS Code webview↔host IPC | 扩展实例;**同步阻塞审批 gate** | 同步 gate 无审计——我们的 R 模型更强 |
| **claude_code** | ❌ 编辑器内/SDK | stdio 流式 JSON | 进程态 | 参数稳定性、microcompaction 思想 |
| **codex** | ❌ TUI(可选 UI) | HTTP/WS 通道 | thread-fork | 拒绝即停 turn(防"拒绝驱动循环") |
| **continue** | ❌ 编辑器内 | webview | @-context | **`isItemTooBig` 注入前预校验**(防 context 爆) |
| **assistant-ui** | n/a(组件库) | — | name-keyed tool-UI 注册表 | **正是我们通用工具信封的渲染端范式** |
| **crush** | ❌ TUI | 进程内 channel | agent 态 | 密集状态行、loop 检测 |

**真正可对照"协议参数"的是 opencode / OpenHands / plandex(三家分离)**。结论:**他们没有一家在"参数形状的优雅度"上超过我们**;OpenHands 的传输更重(双通道),plandex 模型更刚性。我们要吸纳的是**能力面**(opencode 的 LSP 暴露)与**个别思想**(OpenHands 的 condensation-as-event),不是参数风格。

### 4.2 我们独有(无单一同类同时具备)

streamable HTTP 单流(无 WebSocket、无 connId 路由)· HITL R 模型(可持久化/审计/跨重启)· durable/ephemeral 硬不变量 · 通用工具信封(渲染按 `name` 查注册表,天然对齐 assistant-ui) · 多 provider×多 model 显式配对 · OTel 三驾马车 → slog · fork + 影子 git checkpoint + export/import 三件套。

---

## 5. 真正的体验缺口(非参数瑕疵)

### 缺口 A — 协议层零契约测试(防漂移)

`internal/delivery/protocol/` **无任何测试文件**;wire ↔ 前端 `docs/protocol/API.md` 全靠**人肉 review** 同步(`PROTOCOL_ALIGNMENT_REVIEW` 的 P1.2 / B0,列为迁移前置却未落地)。
- 有意**不做 TS codegen**(Go 扁平 struct ↔ TS 判别联合不对应,本仓既定决策,合理)。
- 但 **golden-sample 契约测试**(把典型 wire JSON 固化进 `testdata/`,两端 CI 各自校验)是更便宜替代,挡住 ~90% 漂移。
- **纯后端、additive、不需前端协调** —— 可立即做。

### 缺口 B — 能力已有、未接成 RPC(`docs/613/NEW_API_SPEC.md` 提案)

前端 6/14 的 `ui-research/` 把急需能力列清,且**后端内部大多已实现,只差暴露**:

| 提案 | 解锁的前端体验 | 后端现状 | 优先级 | 破坏性? |
|---|---|---|---|---|
| **B7 `workspace.code.*`**(definition/references/hover/documentSymbols/workspaceSymbols/diagnostics) | `@symbol` 跳转、悬停签名、诊断面板 | `domain/codeintel` 完整 LSP(gopls+tsserver)已就绪 | **P0** | 否(additive,`features.codeIntel` gated) |
| **B8 `workspace.listFiles` / `readFile`** | 文件树浏览器、`@file` 补全 | fs 工具已有 glob/read,只暴露了 getFileHead/grep | **P0** | 否(additive) |
| **B9 `approval.{getMode,setMode,listRemembered,forget}`** | 审批模式分段器、always-allow 管理 | `domain/approval` 有 mode+remember,仅启动时可设 | P1 | 否 |
| **B10 `sessions.compact` + `Item{type:compaction}`** | "上下文>80% 一键压缩" + 时间线压缩分隔 | `MaybeCompact` 已有,只在 turn 边界自触发 | P1 | 否(additive,`features.compaction`) |
| **B11 `todos.list` + `state.snapshot{todos}`** | 持久任务清单 + 实时更新 | `domain/todo` SQLite 已持久化,仅进 system prompt | P1 | 否(复用 state 通道) |
| **B12 `workspace.mcp.authenticate{server,token}`** | MCP `needsAuth` 时前端回传 token | 鉴权基座已落(`a4f7a4d`),缺回传方法 | P2 | 否(`features.mcp`) |

**B7+B8 是当前体验的最大解锁点**(@-context + 文件浏览是编码 agent 的核心交互;前端称从"对话层"升级到"工作台层")。这些都是**加方法、不破坏旧契约**,且前端已写好 spec。

> 参数设计层面这些提案**也遵循我们既有的优雅约定**(dot 命名、`type` 判别、`Page[T]`、cwd jailing、0-based LSP 坐标),无需另立风格。

---

## 6. 值得吸纳的思想 / 明确不抄

**吸纳(FE-API 层)**:
- **OpenHands「condensation-as-event」**:压缩不是替换历史,而是产生**不可变压缩事件 + projection 重放丢弃旧消息**。比"整段 LLM 摘要进一条 system message"更可重放/审计。**B10 的 `Item{type:compaction}` 已朝此走一半**——把它做成时间线一等事件而非旁路即可。
- **`state.snapshot` 收敛为单一可变视图通道**(todos / context% / 未来运行态),别为每类状态开新流(B11 已这么设计,对)。
- **continue `isItemTooBig` 注入前预校验**:`@file` 注入前验大小、超限拒绝。我们 `readFile` 已返回 `truncated+totalLines` 支撑它(前端职责,wire 已就位)。
- **「最小客户端 profile」文档化**:明确"朴素 chat 客户端只需 initialize / sessions.create / runs.start / item 事件 / items.list",HITL/subagent/state 全可选——降低接入门槛(我们协议唯一偏重处:三层资源树 + run 分叉 + durable/ephemeral 一次性全要理解)。

**不抄**:plandex 刚性多阶段流水线 · WebSocket 双通道(我们 streamable HTTP 更干净)· cline 同步阻塞 gate(无审计) · 为优雅而加 `expand[]`(YAGNI)。

---

## 7. 建议与优先级

| 序 | 动作 | 性质 | 是否需前端协调 |
|---|---|---|---|
| **1** | **缺口 A:协议 golden-sample 契约测试** | 纯后端、additive、防腐 | 否 — 可立即做 |
| **2** | **缺口 B-P0:`workspace.code.*` + `workspace.listFiles/readFile`** | additive RPC,后端能力已具备 | **是** — 先与前端 `API.md` 对齐(spec 已在 `613/`)+ 列 scope 待确认 |
| **3** | B9/B10/B11 接线(审批模式 / 压缩事件 / 任务清单) | 多为暴露已有内部能力 | 是 |
| **4** | (可选,锦上添花)§3 参数小项 + 「最小客户端 profile」文档 | 低优先 | 文档侧 |

**不建议做**:翻修 §2/§3 已经 Stripe 级的参数(无收益,纯 churn)。

---

## 8. 附:方法 × 参数速查(现行 `2026-06-07`)

| 方法 | 关键请求参数 | 响应 | 流式? |
|---|---|---|---|
| `runtime.initialize` | `protocolVersion, clientInfo, capabilities` | `serverInfo, capabilities` | 否 |
| `sessions.create` | `cwd?, title?, model?, metadata?` | `Session` | 否 |
| `sessions.update` | `sessionId, title?*, cwd?*, model?*, metadata?*`(指针=偏改) | `Session` | 否 |
| `sessions.fork` | `sessionId, fromRunId?, title?` | `Session` | 否 |
| `sessions.rollback` | `sessionId, toRunId?, restoreType?` | `{session, droppedRuns[]}` | 否 |
| `sessions.export/import` | `sessionId, format?` / `artifact` | `SessionArtifact`(往返) | 否 |
| `runs.start` | `sessionId, input[], context?, tools?, provider?+model?(成对), mode?, maxSteps?, maxBudgetUsd?, params?` | `{runId, userItemId}` + 事件流 | **是** |
| `runs.resume` | `parentRunId, responses[]{itemId, response{type,...}}` | `{runId}` + 事件流 | **是** |
| `runs.subscribe` | `runId`(+ `Last-Event-Id` header) | 事件流(durable 重放) | **是** |
| `runs.cancel` | `runId, reason?` | — | 否 |
| `items.list` | `runId?, cursor?, limit?` | `Page[Item]` | 否 |
| `workspace.getDiff` | `mode, format, path?` | diff(rows/raw) | 否 |
| `providers.{list,configure,test}` / `models.list` | provider key/baseURL | `Page[Provider]`/`Page[Model]` | 否 |

**判别联合速查**:`Item.type`(userMessage/agentMessage/reasoning/plan/question/toolCall) · `RunOutcome.type`(completed/error/maxSteps/maxBudget/canceled/interrupt) · `Interrupt.type`(approval/question/toolResult) · `ContextItem.type`(file/selection/url/image) · 错误按 `ProblemData.type` symbolic 名分支。

---

> **维护**:能力面(§5 B7-B12)落地后回来勾掉;参数若再迁版本,§2.3 + §8 同步更新。本文与 `PROTOCOL_ALIGNMENT_REVIEW.md` 的关系:那份是 wire-shape 债的修复账(已基本完成),本文是**站在前端 DX 视角的现状评审 + 能力缺口**。
