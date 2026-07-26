# 协议设计指南 —— Lyra Runtime Protocol 的取舍基准与优化路线

> **本篇是什么**：`API.md` / `AUX_API.md` / `TRANSPORT.md` 是 wire 契约的**真相源**（有什么、什么形状）；本篇是**为什么这样定、接下来怎么调**——四方协议的一手对照、我们赢在哪必须守、别家哪一点是精华该取、哪些是糟粕明确拒，以及新增协议表面时的自检标尺。
>
> **对比对象（协议/API 设计本身，不谈引擎能力）**：
> **Codex app-server**（OpenAI，JSON-RPC over stdio/WS/unix）· **opencode v2**（sst，Effect HttpApi + OpenAPI 3.1 + 事件源）· **Claude Code / Agent SDK**（Anthropic，NDJSON over stdio + in-band control 协议）。
>
> **方法**：一手核对。opencode 读其 `packages/protocol` + `packages/schema` 源码（v2 重写后的形态，与旧 REST/Zod 形态不同）；Codex 读 app-server 规范全量方法/通知表；Claude Code 读 headless + Agent SDK 文档，并以本地 `claude_code/src/bridge` 的 `control_request` 实现交叉验证；我方读 `docs/protocol/*` + `internal/delivery/protocol`。基线 **2026-07-26**。
>
> **裁决立场**：对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md)（薄核 / 窄腰 / 一个扩展机制）与 [`../../CLAUDE.md`](../../CLAUDE.md)（第一法则不留债、第二法则治本、YAGNI）判"该不该学"，而非见特性就抄。引擎能力对比见 [`RUNTIME_COMPARISON.md`](RUNTIME_COMPARISON.md)，GUI 形态见 [`DESKTOP_COMPARISON.md`](DESKTOP_COMPARISON.md)。

---

## 0. TL;DR

**守住 6 条**（我们比三家更有原则，动它就是回归）：领域中立工具信封 · durable 事件是投影而非存储 · HITL R 模型 · 一个判别字段 `type` · 无状态 per-request 能力协商 · 核心/旁路分离 + 三扩展缝。

**该取 2 条**：① 从 SSOT 导出机器可读制品（OpenRPC + JSON Schema）——三家都有，我们只有 golden-sample 闸；② 控制面推送落点——后台 run / goal / 会话状态目前只能轮询，是四家里唯一要轮询的。

**明确拒 5 条**：Codex 的 per-variant 强类型 item + 分裂审批方法、Codex 的 server→client request、opencode 的 per-event 版本化 + V1/V2 并存、opencode 的业务错误映 HTTP status、Claude Code 的 in-band control 协议（绑单客户端）。

**当下 4 处债**（§6 有逐条改法）：6 个 `mcp.*` 方法实现了但规范里没有 · `API.md` §13 与 §7.3 自相矛盾（`runs.steer`）· §14 承诺的制品导出未落地 · goal 状态靠前端轮询。

---

## 1. 四方形态对照

| | 核心模型 | wire | 流拓扑 | schema SSOT | 人机交互（HITL） |
|---|---|---|---|---|---|
| **lyra** | Session → **Run → Segment** → Item | JSON-RPC 2.0（严格 envelope） | **per-call streamable HTTP**：事件走本次调用的响应流，无常开总线、无连接身份 | Go `delivery/protocol` 手写 + golden-sample 双侧闸 | **R 模型**：interrupt 收尾当前 segment、durable OpenInterrupt、任意客户端 `runs.resume` |
| **Codex app-server** | Thread → Turn → Item（**强类型变体**） | JSON-RPC 2.0，**双向**（server 可发 request） | stdio / WS / unix，通知独立异步通道 | Rust → ts-rs 生成 TS + JSON Schema | **server→client request** + `serverRequest/resolved` 收孤儿 |
| **opencode v2** | Session → Message → Part（**事件源投影**） | REST + OpenAPI 3.1（Effect HttpApi） | **单条全局 SSE** `GET /api/event`；PTY 走 WS | Effect Schema → OpenAPI → codegen SDK | **三套并存**：`permission.*` / `question.*` / `form.*` 各有 reply/cancel |
| **Claude Code** | 无服务端资源（JSONL + `session_id`） | NDJSON over stdio，`control_request` / `control_response` **复用同一管道** | 单进程、单客户端 | TS/Python 类型即 SSOT | 进程内回调 `canUseTool` |

一句话差异：四家都是"引擎与前端分离 + 流式 turn"，但**只有我们把"一次逻辑运行"与"一段流"分成两级身份**（`runId` 稳定 / `segmentId` 每段新铸），这是"停车-续跑"能在多客户端、断线、进程重启下都自洽的根。

---

## 2. 已经赢的地方 —— 守住（附对方代价的一手证据）

### 2.1 领域中立工具信封（★ 差距在扩大）

我们核心只认 `ToolInvocation{name, arguments, result}` 一个形状，富渲染走客户端展示注册表。

- **Codex 的代价现在完整暴露**：item 是 `commandExecution` / `fileChange` / `mcpToolCall` / `webSearch` 等强类型变体，**审批方法跟着分裂**成 `item/commandExecution/requestApproval`、`item/fileChange/requestApproval`、`item/permissions/requestApproval`。加一类工具要动 wire 类型 + 审批方法 + 客户端三处。
- **opencode 的代价**：一次工具入参流式要三个事件（`session.next.tool.input.started` / `.delta` / `.ended`），事件名深点号嵌套；我们是一个 `item.delta{type:"toolArguments"}`。
- **Claude Code 是另一极**：直接透传 Anthropic 的 `tool_use` / `tool_result` 块，零抽象——客户端必须自己认识每个工具，且换 provider 就换形状。

**守的判据**：任何"给某类工具加 wire 一等类型"的提案一律拒；新工具只在客户端展示注册表登记一行。**前提**是富 `result` 形状的前后端一致性必须由 golden-sample 闸机器保证（§6.2 是它的延伸）。

### 2.2 durable 事件是投影，不是存储（opencode v2 反向印证）

opencode v2 现在把 durability 做成 per-definition 声明 `durable: {aggregate, version}`，帧上带 `{aggregateID, seq, version}`——**事件流就是存储格式**。代价在其 schema 包里一览无余：`Event.latest()` / `versionedType()` 一整套 per-event 版本迁移机制，加上 session / permission / question 的 **V1 与 V2 定义并存** + 专门的 legacy 事件模块。

我们的 durable 落点是 Item + SQLite，wire 事件只是投影，于是：durability 能是 `event.type` 的**纯函数**（不必每帧冗余携带）、协议能按日期整体 bump、**永不需要 per-event 版本号**。这正是第一法则不肯背的那类债，现在有了反面样本。

**守的判据**：不把 wire 事件当持久格式。任何要给单个事件加 `version` 的提案 = 事件源化的第一步，拒。

### 2.3 HITL R 模型（Codex 自己的补丁是最好的证据）

Codex 走 server→client request，因此必须发明 `serverRequest/resolved`：**"没有订阅者，或 turn 在批准到达前就结束了"**的孤儿态得靠一条清理通知收尾；等审批期间 turn 仍在飞（占资源，且要求某个客户端在线应答）。

我们：interrupt 收尾当前 segment、资源全释放、待解项落成 durable `OpenInterrupt`（跨重启可 `runs.listOpenInterrupts` 发现）、**任意**客户端 `runs.resume` 在同一 `runId` 上开新段。那类孤儿态在我们的模型里**不可表达**。

opencode 更糟：`permission` / `question` / `form` 三套并存的人机交互资源，客户端得先判断"这次该用哪套"。Claude Code 的 `canUseTool` 是进程内回调，结构上只能单客户端。

顺带一处我们更细：Codex 的记忆粒度是 `acceptForSession` + scope `session|turn`；我们是 `remember{session|project|global}` × `(tool, subject)` 规则表——记住的是「`npm run *` 在本 project」而非「整个 shell」。

### 2.4 一个判别字段 `type`（`kind` 禁上 wire）

三家都用 `type`，但都没有"无例外"这条硬规则。它消除的是"这个看 type、那个看 kind"的认知税与拼错判别字段的无声 bug——**这是给客户端作者和 codegen 同时省事的规则，不是洁癖。**

### 2.5 无状态 per-request 能力协商

我们把 `protocolVersion` / `clientInfo` / `clientCapabilities` 放进每个请求的 `params._meta`，不写成进程全局状态。Codex 是 per-connection `initialize` 握手；opencode **完全无协商**（靠调 list 路由探能力）；Claude Code 在 `system/init` 上给 `capabilities: string[]` 行为令牌。

我们的形态在**多客户端**下唯一自洽：两个客户端能力不同，同一 runtime 同时服务，互不影响——而握手式协商必须回答"这条连接的能力是谁的能力"。配套两条硬约束（server 不得发客户端未声明的事件类型 / 不得产出未声明 `interruptTypes` 的 open interrupt）把"永远 resume 不了的持久 interrupt"这种最坏状态从协议层排除。

### 2.6 核心 / 旁路分离 + 三扩展缝

核心只 sessions / runs / items；工作树视图、skills、recipes、mcp、hooks、codebase 各自顶层根，归 AUX；info / health 走 sidecar 且不进 envelope。两家都是"全在一个 app-server / 全在 REST"。

**已收口的一条规则（曾经漂移过，别再犯）**：新增旁路能力必须先选它**真实的**协议根——`workspace.*` 只放工作树视图（VCS / 文件读 / 搜索 / 项目 / 事件流），skills、recipes、agentDocs、mcp、hooks、codebase 都是独立顶层根，delivery 内部的 typed Runtime 按同一边界切分，不以 `Workspace` 聚合无关领域。

配套的**三扩展缝选择指南**（Item = durable 历史产物 / state = run 期有终值的共享可变态 / custom = 一次性信号）+ `plugin:<name>/` 命名空间（统一用于 custom 名、state key、error type、开放枚举）是两家都没有的成文扩展契约。**新增任何"额外的东西"先过这张表**，选错就是漂移。

---

## 3. 值得取的精华

### 3.1 【取 · P1】schema SSOT → 机器可读制品

- **opencode v2 是标杆**：Effect Schema 即 SSOT → OpenAPI 3.1 注解 → 独立 codegen 包 → 多语言 SDK。校验、文档、SDK 一处定义。
- **Codex**：Rust SSOT → ts-rs 生成 TS union + JSON Schema。
- **我们**：Go 是 SSOT，TS 手写，靠 golden-sample 双侧 pin 抓字段名/形状漂移（这层已生效，覆盖 §4 数据目录 + §5 流的每个联合变体）。但 `API.md §14` 自己把"导出 OpenRPC（方法表）+ JSON Schema（数据类型）"写成**要求**，至今是空的。

**为什么该做而不是 YAGNI**：它是 §2.1 薄核选择的**安全前提**——富 `result` 形状是"非规范展示约定"，不被 wire 联合机器保证；样本闸只覆盖有样本的那些形状，导出的 schema 才是非 Go / 非 TS 客户端的单一对接物。**取"schema 即对接物"的思想，不取 Effect/Zod 那套框架**（我们不换 SSOT 语言方向：Go 结构体仍是源，制品是导出物）。

### 3.2 【取思想 · P2】控制面要有推送落点

三家都有"与某条具体 run 无关"的状态推送：Codex `thread/status/changed`、`thread/goal/updated`、`thread/tokenUsage/updated`；opencode `session.status` / `session.idle` / `todo.updated`；Claude Code 因单客户端不需要。

我们有 `workspace.subscribe` 这条半控制面（`files.changed` / `mcp.serverChanged` / `skills.changed` / `schedules.fired` / `resync`），但**没有"我没订阅的那条 run、那个会话的状态变了"的落点**。直接后果：goal 模式在前端定时轮询 `goals.get`（见 `chat/goal/application/goalData.ts`），而 schedules 触发的 headless run 只能靠 `schedules.fired` 提示"去重拉列表"。

**取的是"控制面事件"这个思想，不取 opencode 的常开全局总线**（TRANSPORT 反不变量：不退回常开 server→client 通道 + 连接路由 + 广播 fan-out）。落点见 §6.4 的两个备选。

### 3.3 【不取】行为令牌式能力探测

Claude Code 在 `system/init` 给 `capabilities: ["interrupt_receipt_v1", …]`，文档明确说"用它做特性探测，别比版本号"——思想和我们开放 `features` map 同源，而我们的 map 还带 `stability`。**已被覆盖，不加第二种探测机制。**

同类还有 **Codex 的字段/方法级 `experimentalApi` 门控**（客户端在 `initialize` 声明才解锁，未声明就调返回"requires experimentalApi capability"）：我们的 `features` map 已覆盖方法级门控，`FeatureCapability.stability` 已表达"未定稿"，加上"客户端忽略未知字段"，**字段级门控没有真实消费者 → 不加**。真需要时的落点是 `features` 的子能力，不是新机制。

### 3.4 【部分取】错误 payload 的类型化

opencode v2 用 tagged error class，每类错误带**类型化字段**（会话不存在带 `sessionID`、消息不存在带 `sessionID` + `messageID`、审批不存在带 `requestID`），客户端不用从 message 里抠。这一点比我们的 `errors[].field`（面向表单校验）更适合"资源不存在"类错误。

**取法克制**：我们的 `ProblemData` 已是 RFC 9457 裁剪 + `channel` 自描述 + 稳定符号 `type`；**若**将来发现客户端在拿 `detail` 做字符串解析（那是硬禁的），再在 `ProblemData` 上加一个开放的 `subject?: Record<string,string>`。现在没有该消费者 → 不加。**坚决不取的是他们把业务错误绑 `httpApiStatus`**（我们的反不变量：HTTP status 只反映传输层）。

### 3.5 【已有落点】背压与重试的可观测性

Codex 在 WS 过载时回 `-32001 "Server overloaded; retry later."`；Claude Code 把 provider 重试做成一等事件 `system/api_retry{attempt, max_retries, retry_delay_ms, error}`。

我们不加 retry layer（跨模块反不变量：SDK 内 retry 已够），但"正在第 2/5 次重试"对 UI 有价值——**落点已经存在**：`segment.progress.activity`（人读的当前动作，ephemeral，终值在 `segment.finished`）。**不新增事件类型**；要做就是让 progress 的 activity 覆盖这类阶段。

### 3.6 【记录为非目标】location / workspace 二轴寻址

opencode v2 把工作目录从带外 header 改成 typed query（`location[directory]` / `location[workspace]` 的 deepObject）——**方向和我们"业务参数进 params、不进带外"一致**，是他们向我们这边靠。他们多出的 `workspace` 轴是为 project copy / worktree 服务的。

我们 `Session.cwd` 单根 + isolated run 的副本路径**有意不入库**（副本即世界）。**触发条件写在这里**：只有当"同一 session 同时对多个根可见"成为真实需求时才动——那是 §13 列明的破坏性改动，须先咨询。

---

## 4. 明确不学

| 别家做法 | 判据（一句话） |
|---|---|
| Codex 强类型 item 变体 + per-variant 审批方法 | 加一类工具动三处协议，违背薄核；我们的中立信封 + 展示注册表更优（§2.1） |
| Codex server→client JSON-RPC request | 制造"server 在等哪个客户端"的孤儿态，得靠 `serverRequest/resolved` 补；R 模型不可表达该状态（§2.3） |
| opencode per-event 版本化 + V1/V2 并存 + legacy 模块 | wire 事件当存储格式的必然后果；我们按日期 bump 整协议、dev 阶段不留兼容（§2.2） |
| opencode 业务错误映 HTTP status | HTTP status 只反映传输层；业务判错一律 `error.data.type` 符号名 |
| opencode 深点号嵌套事件名（`session.next.tool.input.started`） | 扁平 `item.delta{type}` 一个事件覆盖，判别在 `type` 不在名字层级（§2.4） |
| opencode 三套并存的 permission/question/form | "该用哪套"就是设计没收敛；统一到 HITL interrupt（§2.3） |
| Claude Code in-band `control_request`（复用同一管道、`request.subtype` 分发） | 控制通道绑在单条进程管道上 = 单客户端假设；我们是无连接身份的 per-call 流 |
| Claude Code 无服务端会话资源（JSONL + 客户端读文件） | 历史/恢复/多客户端一致性全推给客户端；我们 `items.list` 即历史、单一 SQLite |

---

## 5. 判准：什么叫"人体工程学 + 符合 agent 心智模型"

这两条是新表面的**验收标准**，不是形容词。

### 5.1 对人（客户端作者）

1. **一个读法**：所有 list 是 `Page<T>`（读 `data`，`nextCursor` 存在即 has-more）；所有联合看 `type`；所有 id 自带类型前缀。学一次，用在全协议。
2. **最小可用路径要短**：`sessions.create` + `runs.start` + 消费 `item.*` / `segment.finished` + `items.list` 就是一个能用的客户端。其余全部 capability 门控、缺省关闭。
3. **渲染不需要 join**：interrupt payload 自包含（不必回查工具入参）、`items.list` 顺带返回 `runs`、`RunRef` 自带 model/provider。要求客户端"先查 A 再查 B 才能画一帧"= 设计失败。
4. **错误可分支**：按 `type`（+ `retryable`）分支，`channel` 自述来自 rpc / run / tool 哪条通道；**永不** substring-match `detail`。
5. **丢帧不致命**：丢掉每个 ephemeral 事件，客户端仍必然得到正确终态。新增无 durable 落点的 ephemeral = 协议违规。
6. **无隐式全局状态**：没有"连接级 active project"、没有握手后才能用的方法、没有必须按序调用的仪式。任何客户端、任意顺序进来都对。

### 5.2 对 agent（模型与工具侧的心智模型）

1. **Item 是 agent 工作的自然单位**，不是 UI 单位：消息 / 推理 / 计划 / 提问 / 工具调用。所以同一套原语既能流式播、又能回放重建，不需要第二套"历史模型"。
2. **工具形状与模型看到的 tool schema 同构**：`{name, arguments, result}`，`arguments` 永远是已解析对象、绝不双重编码。协议层不做翻译，也就不会翻错。
3. **"等人"是常态而不是失败**：interrupt 是可恢复的 outcome 变体，不是 error。把 HITL 建模成错误的协议会逼客户端把正常流程写进 catch 分支。
4. **预算与步数上限是终态，不是异常**：`maxSteps` / `maxBudget` 是 outcome 变体并带 `detail`（花了多少 / 上限多少），因为对 agent 来说"钱花完了"和"做完了"一样是正常收尾。
5. **共享可变态用整份快照**：`state.snapshot{todos}` 全量替换，与模型 `todo_write` 的语义一致——不要求模型维护增量，也就不会有增量对不上的状态。
6. **工具失败不终止 run**：`toolCall.error` + `status:"incomplete"`，agent 据此换方案继续。把工具失败上升成 run 失败会毁掉 agent 的自纠错能力。

### 5.3 新增协议表面的十问自检

> 任何新方法 / 新事件 / 新字段落地前，逐条过。任一条答不上就先别写。

1. 它属于三扩展缝的哪一缝（Item / state / custom），还是真的需要新方法？
2. 它的判别字段是 `type` 吗？枚举是开放还是闭合（会被插件扩展吗）？
3. 如果它是事件：durable 还是 ephemeral？**ephemeral 的终值落在哪个 durable 字段上**（能指名）？
4. 它是核心（session/run/item）还是旁路（AUX）?放错根就是下一次收窄重构。
5. 客户端能只靠这一帧渲染吗，还是要额外 join？
6. 最小客户端不实现它，会不会坏？（不会 → 必须 capability 门控且缺省关闭）
7. 它会不会让 server 需要主动向某个客户端发 request？（会 → 重新设计成 R 模型）
8. 它把领域知识焊进 wire 了吗？（工具形状、渲染结构、UI 概念 → 拒）
9. 命名过关吗：`<domain>.<verb>` / camelCase / 缩写在白名单内 / 单位后缀已登记？
10. 有 golden sample 吗？（新联合变体 / 新方法 req·resp 必须补样本，否则闸不覆盖它）

---

## 6. 现存债与调整路线

### 6.1 【P0 · 纯文档治本，无 wire 改动】规范与实现对齐

- **6 个方法实现了但两份规范都没有**：`mcp.configs.list` / `.configure` / `.remove` / `.setEnabled` / `.test` 与 `mcp.servers.authorize`（含 OAuth 2.1 DCR+PKCE 那条路径）。`API.md` 自称唯一 wire 真相源，现在缺 6 个方法——前端与未来的第三方客户端只能读代码。
- **`API.md` 自相矛盾**：§7.3 已完整定义 `runs.steer`（实现也在），§13「v2 明确不做」还写着"经 `runs.send` 的 mid-run steering（留 v2.x）"。stale 条目按第一法则直接删，不留"历史上曾经"。
- **§14 的语气要诚实**：制品导出仍未落地却写成"本协议要求"——要么做（§6.2），要么改成显式待办。规范里的空头承诺与代码里的 TODO 同性质。

### 6.2 【P1 · additive，非破坏】从 Go SSOT 导出 OpenRPC + JSON Schema

- **现状**：golden-sample 闸已覆盖 §4 数据目录 + §5 流的每个联合变体（Go round-trip + TS 侧 pin），字段名/形状漂移有机器抓。缺的是**给非 Go / 非 TS 客户端的对接物**，以及"方法表本身"的机器可读形态。
- **改法**：`delivery/protocol` + dispatch 方法表为源，生成 OpenRPC（方法 + 入/出 schema）与 JSON Schema（数据类型），CI 卡 drift。
- **不做的事**：不引入 schema-first 框架（不换 SSOT 方向）；不做判别联合感知的 TS 生成器（Go flat-struct 不映射 TS union 这条结论不变，且有样本闸后它已非漂移风险关键路径）。
- **影响面**：新增构建产物 + 一个 CI gate，零 wire 改动。

### 6.3 【P1 · 文档】把本篇的 §5 判准接到 review 流程

`API.md` 附录 A 是"设计不变量摘要"（是什么），本篇 §5.3 是"新表面自检"（怎么用）。协议改动的 PR 描述里过一遍十问，比事后 review 便宜。

### 6.4 【P2 · additive 协议改动，须先确认】控制面推送落点

- **根因**：非当前流的 run / 会话 / goal 状态没有推送落点，客户端只能轮询（goal 是现成的症状）。
- **备选 A（推荐）**：在 `workspace.subscribe` 的 `WorkspaceEvent` 上增开控制面事件（`session.status` 变更 / `goal.updated`），与既有 `schedules.fired`、`mcp.serverChanged` 同性质——复用现有流、不新增连接、不回退到常开总线。落地后删掉 goal 的定时轮询。
- **备选 B**：新开 `sessions.subscribe`。语义更正（会话域的事件归会话根），代价是多一条流 + 多一套重连/回放规则。
- **判据**：`workspace.subscribe` 事实上已是"runtime 级控制面"而非"工作树专属"——若选 A，需同时在 AUX_API 里把它的职责说清，否则下一个人会以为它只管文件。若认为这个命名歧义不可接受，就选 B。**两者都是 additive，但都动公开事件集，按约定先出 scope 再动。**

### 6.5 优先级

1. **P0 §6.1**：规范补 6 个方法 + 删 stale 条目 + §14 语气诚实化（纯文档，可立即做）。
2. **P1 §6.2**：OpenRPC + JSON Schema 导出（薄核选择的安全前提）。
3. **P1 §6.3**：十问自检接入协议改动流程。
4. **P2 §6.4**：控制面事件（需确认 A / B）。

---

## 7. 演进纪律

- **同版本号可加**：新方法、新可选响应字段、新事件类型、新 `features` key、新 `custom` name / `state` key。客户端必须忽略未知字段、容忍未知方法与事件。
- **必须 bump 日期版本**：改语义、删字段、改字段类型、改判别集合、**新增必填请求字段**（旧 server 严格解码会拒）。
- **永不做**：per-event 版本号、legacy 兼容层、migration、"以后再清"的 wire 字段。dev 阶段 shape 变了就 bump + 丢旧 store。
- **破坏性公开 API 改动（含改一个签名 / 删一个类型 / 改一个字段）先咨询**：列 scope + 影响面 + 备选，确认再动。本篇的 §6.4 就属于此类。
