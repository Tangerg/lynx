# 协议对齐审视（Protocol Alignment Review）

> 本文记录对 lyra 后端 ↔ 前端 wire 契约（`/Users/tangerg/Desktop/lyra/docs/protocol/` 的
> API.md / TRANSPORT.md / TOOL_OUTPUT.md）的严格核对结论与改进清单。契约是真理来源；本文
> 列出实现偏离契约处、以及契约本身可优化处。
>
> 状态记号：✅ 已修 / 🔲 待办 / ⏸ 有意保留（附理由）。

---

## 第一轮：实现 vs 冻结契约的对不齐（已修）

逐节比对 `rpc/protocol` / `rpc/dispatch` / `rpc/server/translator` / `rpc/transport/http` 与
前端 `frontend/src/rpc/shapes.ts`。**大部分已对齐**（transport 状态码、streamable HTTP 首帧无
SSE id、415/413/400-not-409/204、error 走 `data.type`、方法路由表、id 前缀、HITL R 模型均正确）。
发现并修正 3 处：

### ✅ A — `commandExecution.output` / `outputTruncated` 完全缺失（最严重）
- **症状**：命令卡片在历史回放 / 重连时显示 `(no output)`，但模型明明拿到了输出。
- **根因**：`ToolInvocation` 协议类型没有 `output`/`outputTruncated` 字段；translator 把 stdout
  **只**作为 `toolOutput` delta（`durable=false`）发，completed item 上不带 output —— 违反
  API.md §4.4 + TOOL_OUTPUT.md §3/§5（output 是 completed 上的**硬契约**）。前端早已按契约实现
  `output?`/`outputTruncated?` 并把 delta 当预览。
- **修正**：`ToolInvocation` 加 `Output *string`（指针使 started 壳省略、completed 即使空也发
  `"output":""`）+ `OutputTruncated bool`；`fillCommandResult` 在 completed 时填合并的
  stdout+stderr；preview delta 改用同一合并文本。补回归测试
  `TestTranslator_CommandOutputOnCompleted`。

### ✅ B — `items.list` 字段名 `items` 应为 `data`
- **症状**：历史列表恒空（前端 `ListItemsResponse extends Page<Item>` 读 `resp.data`，后端发
  `resp.items`）。
- **修正**：`ListItemsResponse.Items json:"items"` → `Data json:"data"`，与 §4.11 `Page<T>` 统一
  （API.md §7.4 明确"无 `data`/`items` 漂移"）。

### ✅ C — `run_not_running` / `-32007` 死代码且违反规范
- 规范 §8.2 故意删除该码（run 二态，无单独 "not running" 码，-32006 直跳 -32008），且代码里
  从无 Runtime 返回它。
- **修正**：删 `CodeRunNotRunning`/`ErrRunNotRunning` + dispatch 映射。

---

## 第二轮：设计层改进清单（混合 —— wire shape + 后端工程）

> ⚠️ 本轮把 wire-shape 与后端工程混在一起列了。后续把**纯 wire shape** 的部分抽出来单独深挖
> （见姊妹文档 / 下一轮）。这里保留原始清单备查。

### P0 — 正确性 / 数据丢失

**1. 🔲 `items.list` 静默截断在 200 条，且永不回 cursor → 长会话丢历史**
`rpc/server/items.go` 把 limit 钳到 200 后 `items[:limit]`，但 `NextCursor` 恒为 `""`。会话超 200
item 时客户端只拿前 200 条、且无信号知道还有更多。`items.list` 的 cursor 当前直接 `notImpl`。
→ 真分页，或至少截断时回填 `nextCursor`（CLAUDE.md "no silent caps"）。

### P1 — 设计根因：漂移防护缺失

**2. 🔲 "Go 为机械 SSOT + codegen 派生 TS + CI 卡 drift" 是空头支票**
API.md 开篇宣称 codegen 派生 TS + CI 卡 drift，实际靠人工 review 同步。刚修的两个 bug
（`items`→`data`、缺 `output`）正是 codegen / 契约测试会当场抓到的漂移。"每次交互七七八八小问题"
的结构性原因就在这里。
→ (a) 真 codegen（根除手写两份，但 flat-struct↔discriminated-union 映射难）；或
  (b) **黄金样本契约测试**（一组 canonical JSON wire 样本，前后端各自 CI 往返校验）—— 便宜、
  立刻能上，覆盖 90% 漂移。建议先做 (b)。

**3. 🔲 判别联合用"胖扁平 struct"表达 → 可表达非法态 + 手搓 map**
`ToolInvocation` 一个 struct 塞所有 kind 字段；`Interrupt.Payload map[string]any` 完全无类型，
translator 手搓 `map[string]any{...}`，key 拼错即无声 wire bug。
→ Go 做不出真 sum type，但能收紧：`Interrupt` 按 kind 配类型化字段；`ToolInvocation` 补往返测试。

### P2 — 设计异味 / 泄漏 / 死代码

**4. 🔲 后端内部数据 `_resume` 跑到 wire 上（还被持久化）**
approval interrupt payload 带 `_resume:{name,args}`，纯后端 resume-binding 数据，因强类型
`commandExecution` 把 name 从 wire 丢了、resume 时拿不回。`OpenInterrupt` 持久化时一起落库。
→ resume key 留服务端（已 keyed by itemId），wire 上不出现 `_resume`。

**5. 🔲 `ProblemData.Extra` 是死字段，注释还在说谎**
`Extra map[string]any json:"-"`，注释称由 dispatch flat 序列化，但全仓无 `MarshalJSON` / 无引用，
放进去的值被静默丢弃。契约 §4.6 `[key:string]:unknown` 确实要支持扩展成员。
→ 写 `MarshalJSON` 兑现契约，或删字段 + 改注释。

**6. 🔲 translator 与工具 arg-schema 名字硬耦合**
`toolKind()` 按 `strings.ToLower(name)` 硬匹配工具名；`toolInvocation()` 硬取
`args["command"]`/`args["pattern"]`/`args["query"]`/`args["path"]`。工具改名或 MCP 工具撞名即误投影。
→ 工具自声明 wire kind + 投影；或按工具身份索引的注册表。

**7. 🔲 `commandExecution.command: string[]`（argv）与 shell-string 执行语义不符**
契约说 argv 数组（规避引号歧义），lyra 走 `/bin/sh -c "<整条串>"`，translator 只能 `[]string{cmd}`
单元素。形状对、语义假。
→ 承认 shell-string 模型：契约改 `command: string`，或文档化"argv[0] 即整行"。

### P3 — 清理 / YAGNI / 次要

**8. 🔲** `RuntimeLimits.MaxItemsPerSession` 不在契约、零用 → 删；`MaxConcurrentRuns:8` advertise
但未强制 → 实现或别承诺。
**9. 🔲** `notifications.canceled` 是 no-op；HTTP 靠断连兜，显式取消路径名存实亡 → 接线或文档化。
**10. ⏸/🔲** `question` interrupt 带 payload（契约说"无 payload"）—— 务实兜底（inProgress question
中断时可能未进 items.list）。→ 保证 question 落 items.list 后去 payload，或把它写进契约。
**11. 🔲** `ListItemsResponse` 没复用 `Page[Item]`（前端是 `extends Page<Item>`）→ embed 镜像。
**12. 🔲** 契约定义了一堆"定义但永不 emit/advertise"的表面（`run.progress`/`state.*`/`custom` Go
类型根本没建；`background.subscribe` 没进 method table、返 `method_not_found` 而非
`capability_not_negotiated`）→ 决定去留，至少 `background.subscribe` 进表保持一致。

---

## 第三轮：纯 wire-shape 优化

> **只看"前后端交换的 JSON 形状本身好不好"**，不碰后端如何生产它、不碰 lyra 工程。
> 这些是对**契约（API.md 的 TS 类型）本身**的设计建议；动它们要前端同步。
> 注：API.md 声明 Go `rpc/protocol` 是机械 SSOT，故"改 shape" = 改 Go 类型 + 改契约文档 + 改前端。

记号：⭐ 强（真实形状缺陷，建议改）/ ◐ 中（值得决断）/ ○ 弱（note，多半可保留）。

---

### S1 ⭐ Interrupt 形状不统一：approval/toolResult 自包含，question 却要 join
契约 §4.8：`approval` 带 `payload.tool`、`toolResult` 带 `payload.tool`，**自包含**——客户端读
payload 即可渲染。唯独 `question` **无 payload**，要靠 `itemId` 回 `items.list` join 出 question 内容。

- **形状问题**：3 个 interrupt kind，2 个自包含、1 个要二次请求。`listOpenInterrupts` 的语义是
  "什么在等我处理"，强制对其中一类再发一次 `items.list` 才能渲染，是形状层的不一致 + 额外往返。
  而且 inProgress 的 question 在中断时可能**还没进** `items.list`（durable 历史只收 completed），
  join 直接落空——这正是 lyra 不得不给 question 塞 payload 的根因（见第二轮 P3.10）。
- **建议**：让 `question` interrupt 也**自包含**——`Interrupt` 的 question 变体直接带 `Question`：
  ```ts
  type Interrupt =
    | { kind: "approval";   itemId; payload: ApprovalPayload }
    | { kind: "question";   itemId; payload: { question: Question } }   // ← 自包含
    | { kind: "toolResult"; itemId; payload: ToolResultPayload };
  ```
  三类一致"payload 足以渲染"，去掉 join、去掉 lyra 的兜底 hack，去掉"重启后渲染不出待答问题"的坑。
- **权衡**：question 内容在 interrupt 与（completed 后的）item 上各存一份。但 interrupt 是**待办快照**，
  本就该自包含；completed item 是历史，二者生命周期不同，不算虚假 DRY。

### S2 ⭐ 强类型 ToolInvocation 变体是"有损"的：丢掉了 name 和原始 arguments
契约 §4.4 有意规定强类型变体（commandExecution/fileChange/search/webSearch）**不带 name、不带
raw arguments**（"kind 即身份，消除 kind↔name 双重身份漂移"）；只有通用 `tool` 变体带 `name` +
`arguments`。

- **形状问题**：强变体把"工具身份（name）+ 模型给的原始入参"**从 wire 上抹掉了**。后果：
  1. 无法对**所有** toolCall 做统一处理（如"按 name 分组的工具调用日志 / 审计视图"）——强变体没 name。
  2. resume / 审批跨边界要 (name,args) 作关联键时拿不回来——lyra 被迫往 payload 塞后端内部的
     `_resume:{name,args}`（第二轮 P2.4 的泄漏，根因就在这）。
- **建议**：让**所有** ToolInvocation 变体共享一个 `{ name, arguments }` 基底（稳定身份），`kind`
  退化为**渲染提示** + 结构投影层叠加在基底上：
  ```ts
  interface ToolBase { name: string; arguments: Record<string, unknown> }
  type ToolInvocation =
    | (ToolBase & { kind:"commandExecution"; command:string[]; output?; exitCode?; … })
    | (ToolBase & { kind:"fileChange"; changes: FileChangeEntry[] })
    | (ToolBase & { kind:"tool"; result?: unknown })   // 通用：无额外投影
    | …
  ```
  name 永远在 = 身份稳定；kind 决定如何富渲染。`kind↔name 漂移`担忧由"name 是 SSOT 身份、kind 只是
  渲染分类"解决，而非靠删字段。
- **权衡**：通用 `tool` 的 `name` 与强变体的 `kind` 不再"二选一"，需在文档讲清"name=身份、kind=渲染"。
  收益是消除整类关联难题 + 去掉 `_resume` 泄漏的形状动因。

### S3 ⭐ SearchResult 过度合并：契约本身是两种类型，被压成一个松形状
契约 §4.5 定义了**两个**类型：`SearchHit { path; lineNumber?; snippet? }`（本地）与
`WebSearchResult { title?; url; snippet?; faviconUrl? }`（网络）；`search` 用前者、`webSearch` 用后者。
但 lyra 的 Go `SearchResult` 把两者**合并成一个 struct**（path/line/snippet + title/url/favicon 全塞一起）。

- **形状问题**：因 Go 是 SSOT，codegen 会从这个合并 struct **生成一个比手写契约更松的 TS 类型**——
  wire 上一个 `SearchResult` 可以同时带 `path` 和 `url`（非法但可表达）。契约的精度（两个互斥类型）
  在实现里丢了。这是 SSOT 与契约文档**互相矛盾**。
- **建议**：拆回两个类型（`SearchHit` / `WebSearchResult`），`ToolInvocation` 的 search 变体用
  `SearchHit[]`、webSearch 用 `WebSearchResult[]`，与契约一致。Go 侧用两个 struct（即便字段有重叠）。
- **权衡**：Go 多一个 struct；但消除"一个结果对象同时是本地命中又是网页命中"这种非法可表达态。

### S4 ◐ RunEvent.durable 是冗余字段：first-party 事件可由 event.type 推出
契约 §5 给了**固定表**：run.started/run.finished/item.started/item.completed/state.snapshot =
durable true；run.progress/item.delta/state.delta = false。即对所有 first-party 事件，`durable`
是 `event.type` 的**纯函数**，在每帧上再发一个 bool 是去规范化（denormalization）。

- **形状问题**：可表达非法态——一帧能声称 `item.completed` 且 `durable:false`（自相矛盾）。冗余字段
  能漂移。唯一**不能**推导的是 `custom` 事件（其 durable 由产出方标注）。
- **建议**：二选一——(a) 去掉 `durable`，客户端按 type 表推导（custom 默认 false 或要求 custom 自带）；
  或 (b) 把 `durable` 收窄为**只**出现在 `custom` 事件上。两者都消除 first-party 帧上的冗余 bool。
- **权衡**：(a) 客户端要内置 type→durable 表（但本就要懂事件语义）；(b) 形状不再"每帧统一含 durable"。
  当前的全帧 durable 是"自描述"换"可漂移"，是值得决断的取舍。

### S5 ◐ 分页形状不统一：只有两个 list 用 Page<T>，其余裸数组
`sessions.list` / `items.list` 返回 `Page<T>`（`{data, nextCursor}`）；但 `runs.list` /
`runs.listOpenInterrupts` / 所有 `workspace.*` / `providers.list` / `models.list` / `tools.list`
都返回**裸数组**。

- **形状问题**：客户端得逐方法记"这个分页那个不分页"。两套 list 约定并存是认知负担，且某个今天
  "有界"的 list 将来要分页时是破坏性改动（裸数组 → `{data}`）。
- **建议**：决断其一——(a) **所有** list 统一 `Page<T>`（即便 nextCursor 永远空，形状面向未来一致）；
  或 (b) 明确写进契约"仅 sessions/items 分页，其余有界裸数组"，并接受未来若要分页需 bump。
- **权衡**：(a) 给本地有界 list 套 `{data}` 壳略显仪式（YAGNI 张力），但换来"所有 list 一个读法"+
  无破坏性扩展路径。倾向 (a)：分页是已发生过的需求（sessions/items 已分），不算投机。

### S6 ◐ 强终态缺 detail：maxSteps/maxBudget/canceled 只有判别式，无人读说明
`RunOutcome` 的 `error` 变体带 `result.error: ProblemData`（含 detail）；但 `maxSteps` /
`maxBudget` / `canceled` **只有 type**，无任何 detail/reason。

- **形状问题**：客户端无法区分"被用户取消" vs "被超时取消" vs "被上层策略取消"，也无法给
  maxBudget 显示"花了 $X / 上限 $Y"。`runs.cancel` 入参有 `reason?`，但它**不回流**到 outcome。
- **建议**：给非 error 终态也开一个可选 `detail?: string`（或让这几类也能挂轻量 reason），让 cancel
  的 `reason` 能在 outcome 里被看到。
- **权衡**：多一个可选字段；多数情况 type 已够，故列为 ◐ 而非 ⭐。

### S7 ○ Usage.byModel 条目缺 cache 字段（与顶层 Usage 不对称）
顶层 `Usage` 有 `cacheReadTokens` / `cacheWriteTokens`，但 `byModel` 的 per-model 条目只有
`input/output/reasoning/costUsd`。形状不对称。若 per-model 不打算追踪 cache，文档说明即可；否则补齐。

### S8 ○ AnswerResponse.answers 值类型是 `string | string[]`
单选给 string、多选给 string[]。值类型联合让客户端每次都要判类型。若统一为 `string[]`（单选也是
单元素数组），消费端形状更平。属微调。

### S9 ○ 每帧 JSON-RPC 信封 + eventId 双载，是 streamable HTTP 下的仪式开销
每个 SSE 事件帧是 `{jsonrpc, method:"notifications.run.event", params:RunEvent}`——`jsonrpc` 与
恒定的 `method` 每帧重复；`eventId` 同时出现在 SSE `id:` 行与 `params.eventId`。

- **现状判断**：**保留**。`method`/`jsonrpc` 给所有 transport 一个统一解析路径；`params.eventId` 是
  InProcess/IPC（无 SSE id 行）的唯一 eventId 载体，SSE `id:` 是 HTTP 重放机制。冗余是 transport
  无关性换来的，可接受。仅记录为 note，不建议改。

### S10 ○ 三套"进行中"状态词：running / inProgress / running
`SessionStatus: running|waiting|idle`、`ItemStatus: inProgress|completed|incomplete`、
`RunStatus: running|finished`、`PlanStep.status: pending|inProgress|completed|failed`。"进行中"在
Run/Session 叫 `running`，在 Item/PlanStep 叫 `inProgress`。语义不同故分开合理，但同义词不同拼写是
轻微不一致。统一"进行中"用词（全 `running` 或全 `inProgress`）可读性更好。属命名微调。

---

## 形状优化优先级

| 级 | 项 | 一句话 |
| --- | --- | --- |
| ⭐ | S1 | question interrupt 自包含，消除 join 不一致 + lyra 兜底 hack |
| ⭐ | S2 | 所有 ToolInvocation 共享 {name,arguments} 基底，name=身份/kind=渲染 |
| ⭐ | S3 | SearchResult 拆回 SearchHit / WebSearchResult 两类型 |
| ◐ | S4 | durable 去冗余（去掉或只留给 custom） |
| ◐ | S5 | 分页形状统一（全 Page<T> 或文档化裸数组） |
| ◐ | S6 | 非 error 终态可带 detail（cancel reason 回流） |
| ○ | S7-S10 | Usage 对称 / answers 统一数组 / 信封仪式(保留) / 状态词统一 |

S1+S2 是**最有价值**的两条：它们同时是形状缺陷、又分别是 lyra 两处 hack（question payload 兜底、
`_resume` 泄漏）的形状根因——改了形状，两处 hack 自然消失。S3 是 SSOT 与契约文档的直接矛盾，必修。

---

## 第四轮：协议作为长期契约的元质量（通用 / 可拓展 / 可维护 / 对接成本 / 漂移 / 门槛）

> 不再看单个形状对错，而看协议**整体好不好用、扛不扛得住演化**。每条标注它主要服务哪个质量。
> 记号同前（⭐/◐/○）。这些多数触及契约本身的组织方式，是方向性建议。

### G1 ⭐ 通用性：闭集"强类型工具变体"把"编码 agent"这个领域焊进了核心 wire
`ToolInvocation` 的强变体是 `commandExecution` / `fileChange` / `search` / `webSearch`——**全是编码
agent 的工具**。一个通用 agent runtime（客服 / 数据分析 / 运营）根本没有这些，却要继承这套核心类型。

- **问题**：核心协议本应**领域中立**，"工具长什么样"是领域知识。把 4 个编码工具做成一等 wire 变体，
  等于把一个特定领域焊死在协议核心里。换个领域，这 4 个变体既无用又误导；加一个新领域的富工具
  （如"SQL 查询结果表"），又得改 wire + codegen + 前端三处——这是协议级的 OCP 违反。
- **建议**：核心只保留**通用** `tool { name, arguments, result }`（领域中立、人人都有）。"如何富渲染某个
  工具"下沉为**数据驱动的展示层**或**可选 profile**：客户端默认按 JSON 树渲染任何工具（开箱即用、零
  对接成本），编码工具的卡片渲染是**加法增强**（按 name 命中展示注册表），永不阻塞新工具。
- **联动**：这与 S2（所有工具共享 `{name,arguments}` 基底）是同一方向——name 是通用身份，富渲染是
  可选叠加。落地后：**新工具无需动协议**，前端可零工具代码起步、渐进增强。
- **权衡**：失去强变体的编译期形状/定制渲染（要靠展示注册表补）。但换来真正的通用性 + 可拓展性 +
  最低对接成本。当前的"闭富集 + 开通用"混合，问题在闭集**画在了领域里**而非画在"渲染机制"里。

### G2 ⭐ 可拓展：ServerCapabilities.features 是**闭合 struct**，加一个能力就得 bump 契约
`ClientCapabilities.features` 是 `Record<string, unknown>`（**开放**），但 `ServerCapabilities.features`
是写死字段的 struct（reasoning/mcp/multimodal/…）。**不对称**。

- **问题**：runtime 想 advertise 一个新能力（如 `voiceInput`），现在必须改契约 struct + 所有客户端
  类型——即便老客户端只会忽略它。这违反 §11 的"前向兼容硬约定"（加可选字段=同版本号）：能力本应是
  协议里**最该可加**的东西，却被建模成最封闭的形状。
- **建议**：`ServerCapabilities.features` 也用开放 map（`Record<string, FeatureFlag>`，FeatureFlag =
  `boolean | { enabled: boolean; … }`），保留几个文档化的已知 key。新能力 = 加一个 key，老客户端按
  §9"忽略未知"自动容忍。`events: string[]` / `providers: string[]` 已是开放数组，features 应一致。
- **权衡**：失去 server features 的编译期字段名。但能力协商**就是**协议的扩展点，开放才符合它的职责。

### G3 ⭐ 减少漂移 + 门槛：两套判别字段名 `type` vs `kind` 混用
协议的判别联合，有的用 `type`、有的用 `kind`：

| 用 `type` | 用 `kind` |
| --- | --- |
| ContentBlock / Item / StreamEvent / ItemDelta / RunOutcome / DiffRow / QuestionField | ContextItem / ToolInvocation / FileChangeEntry / Interrupt / InterruptResponse |

- **问题**：同一个概念（"这个联合用哪个字段判别"）有**两个答案**，且无规律可循。新人必须逐类型记
  "这个看 type、那个看 kind"；写 reducer / 序列化时拼错判别字段是无声 bug；codegen / 校验也要处理两
  套。**纯认知税 + 漂移面**，没有任何收益。
- **建议**：**统一一个判别字段名**（建议全用 `type`，它已是多数派且语义最直白）。一次性 rename，之后
  "所有判别联合都看 `type`"成为一条无例外的规则——门槛、漂移、维护三项同时受益。
- **权衡**：一次破坏性 rename（前后端同步）。但这是**一次性**成本换**永久**的一致性，性价比极高。

### G4 ◐ 对接成本 + 门槛：错误有三个落点，新人预期只有一个
§8.1：错误可能出现在（a）同步 JSON-RPC `error`、（b）`run.finished{outcome:error}`、（c）
`toolCall.error`。三处。新对接者几乎都会先假设"错误在响应里"，然后被流内错误和工具级错误绊倒。

- **缓解（已部分到位）**：三处都用**同一个 `ProblemData` 形状**（很好，别破坏）。
- **建议**：(1) 文档给一张"错误落点决策表"放在显眼处（不是埋在 §8）；(2) 考虑给 ProblemData 加一个
  可选 `channel?: "rpc"|"run"|"tool"` 自描述字段，让客户端无需根据"它从哪来"反推语义——错误自带它
  属于哪条通道。(3) 明确"工具失败通常不终止 run"这条反直觉规则的位置（onboarding 必读）。
- **权衡**：channel 字段是冗余信息（落点已隐含），但它把"靠上下文推断"变成"显式自描述"，降低对接
  成本。列 ◐ 因 ProblemData 统一已解决大半。

### G5 ◐ 对接成本：哪些方法是流式的，靠"硬编码记住"而非协议自描述
同一个 `POST /v2/rpc/{method}` 既可能回 `application/json` 也可能回 `text/event-stream`，客户端要按
响应 `Content-Type` 分支。这本身标准（对标 MCP/OpenAI），但"哪些方法会流式"目前是**人记**的（4 个）。

- **建议**：让"流式方法集"**机器可读**——放进 `ServerCapabilities`（如 `streamingMethods: string[]`）
  或方法元数据。客户端据此预知，而非硬编码 4 个名字（硬编码=漂移源：将来加流式方法，老客户端不知道）。
- **权衡**：能力快照多一个字段。收益是客户端不再硬编码方法分类。

### G6 ◐ 门槛：缺一条"最小集成路径"，新人直面全部概念
协议一上来就是三级资源（Session/Run/Item）+ run 树（spawnedByItemId/parentRunId）+ HITL R 模型
（interrupt→resume→延续 run）+ durable/ephemeral 不变量 + eventId 重放。**全都要懂才能动手**。

- **问题**：只想做"发消息→流式显示回复"的新人，被迫先消化整套模型。§3.1 的端到端走查很好，但没有
  "**最小可用客户端**"的明确边界。
- **建议**：在契约里定义一个**文档化的 Minimal Profile**——做一个能聊天的客户端**最少**需要哪几个
  方法 + 哪几个事件（`initialize` / `sessions.create` / `runs.start` / `notifications.run.event` 里的
  `item.*`+`run.finished` / `items.list`）。HITL / subagents / state / attachments / background 全部
  作为**分层可选能力**，各自由 capability 门控。配合：客户端能声明 `interruptKinds:[]` / 精简 `events`，
  server 必须尊重（§6.2 已要求 server 不产客户端声明外的 interrupt——把这个保证扩展成"最小客户端也能跑"）。
- **权衡**：主要是文档 + 契约组织，但也反向要求能力门控足够细，让"只实现一个子集"是协议合法状态。

### G7 ◐ 可维护 + 门槛：三个扩展缝（items / state / custom）职责边界没讲清
协议有三处可放"运行时想表达的额外东西"：durable 的 **Item**（历史单元）、**state**（snapshot + JSON
Patch 的共享可变视图态）、**custom** 事件（一次性信号）。但**何时用哪个**没有指引。

- **问题**：新人（甚至 first-party 自己）面对"我要表达 X"时，不知道该开一个 Item 类型、还是塞进
  state、还是发 custom 事件。选错就漂移（如把本该 durable 的东西放进 ephemeral 的 custom）。state 还
  引入 RFC 6902 JSON Patch 这个**进阶依赖**，却没说它**为何存在**。
- **建议**：契约里给一张"扩展缝选择指南"：**Item** = 要进 durable 历史、用户回看的工作单元；**state** =
  run 期间的共享可变视图态（如 todo board / 进度面板），有终值快照；**custom** = 不进历史、不改状态的
  一次性提示信号。配 §2.5 命名空间，三缝各自的 plugin: 用法说清。
- **权衡**：纯文档 + 少量形状约束。但这是"协议清晰度"的关键——三个扩展机制不讲边界，等于没有机制。

### G8 ○ 通用性：闭合枚举里哪些该开放，无统一原则
`safetyClass: string`（开放，好）、`mode: "agent"|"chat"|"plan"`（闭合）、`FileChangeEntry.kind`（闭合）。
没有一条"什么枚举该开放给插件、什么该闭合"的原则。建议明确：**面向插件/未来扩展的分类用开放 string +
§2.5 命名空间**（如 mode 可能想加 plugin 模式），**纯内部有限态用闭合枚举**（如 RunStatus）。

### G9 ○ 可维护：协议无"能力/形状的机器可读 schema 制品"
契约是 Markdown + Go 类型。没有一份**机器可读的 schema 制品**（JSON Schema / OpenRPC）作为对接物。
新客户端（非 TS / 非 Go）只能读文档手抄。建议从 Go SSOT 导出 OpenRPC / JSON Schema，作为跨语言对接
的单一制品——同时是 P1.2 漂移闸的基础（黄金样本可由 schema 生成）。

---

## 元质量优化优先级

| 级 | 项 | 服务的质量 | 一句话 |
| --- | --- | --- | --- |
| ⭐ | G1 | 通用 / 可拓展 / 对接成本 | 核心去领域化：通用 tool 信封 + 数据驱动渲染，新工具不动协议 |
| ⭐ | G2 | 可拓展 | ServerCapabilities.features 改开放 map，新能力不 bump 契约 |
| ⭐ | G3 | 漂移 / 门槛 / 维护 | 统一判别字段 `type`（消灭 type/kind 双轨） |
| ◐ | G4 | 对接成本 / 门槛 | 错误三落点：决策表 + 可选 `channel` 自描述 |
| ◐ | G5 | 对接成本 | 流式方法集机器可读（进 capabilities） |
| ◐ | G6 | 门槛 | 定义 Minimal Profile + 全可选能力分层门控 |
| ◐ | G7 | 清晰 / 维护 | items vs state vs custom 三扩展缝的选择指南 |
| ○ | G8/G9 | 通用 / 维护 | 开放枚举原则 / 导出 OpenRPC·JSON Schema 制品 |

**三条 ⭐ 是结构性的**：G1 决定协议能不能跨领域复用、新工具对接成本能不能压到零；G2 决定能力能不能
无痛演化；G3 是一次性 rename 换永久一致（漂移 + 门槛 + 维护三杀）。G3 最该先做——它纯机械、风险低、
收益立竿见影。G1 最有战略价值但改动最大（牵动 S2）。

---

## 第五轮：迁移到 `2026-06-07`（前端已发布新契约，后端对齐）

> 2026-06-07，前端把第三/四轮的建议**几乎全部**落地成正式契约 `2026-06-07`
> （`/Users/tangerg/Desktop/lyra/docs/protocol/2026-06-07/`）。后端 lyra 需从 `2026-06-03` 迁移对齐。
> 这是**协议版本级大迁移**：§12 "dev 无 legacy 兼容，shape 变了 bump version、丢旧 store、不写 migration"
> —— 旧 `lyra.db` 的 Item/Run 历史 blob 与新 shape 不兼容，迁移后须丢弃旧 store（dev 阶段可接受）。
> TOOL_OUTPUT.md 已并入新 API.md §4.4（命令输出进 `tool.result.{exitCode,output,outputTruncated}`）。

### 完整 delta（current → 2026-06-07）

| 区 | 变更 | 落点 |
| --- | --- | --- |
| 版本 | `ProtocolVersion 2026-06-03 → 2026-06-07` | `protocol/runtime.go` |
| G3 判别字段 | 所有 wire `kind`→`type`：`Interrupt`/`InterruptResponseValue`/`ContextItem`/`FileChangeEntry` 的字段 + json tag | `protocol/runs.go`/`items.go` |
| G3 分类字段 | `BackgroundTask.Kind json:"kind"` → `Category json:"category"` | `protocol/background.go` |
| S10 状态词 | `ItemStatusInProgress "inProgress"` → `"running"`；`PlanStep.status` 同 | `protocol/items.go` + translator |
| G1/S2 工具 | `ToolInvocation` 联合 → 单一 `{name, arguments, result}`；删 `ToolInvocationKind`/`toolKind()`；强变体字段（command/exitCode/output/changes/query/results）全移入 `result` JSON（按 §4.4.2 约定） | `protocol/items.go` + translator 大改 |
| A→新 | 命令输出从"顶层 Output 字段 + delta"改为 `result.{exitCode,output,outputTruncated}` + toolOutput delta 预览 | translator |
| S2 泄漏 | 删 approval interrupt 的 `_resume` hack（name 现恒在 `tool.name`） | translator |
| S3 搜索 | `SearchResult` 合并 struct → `SearchHit`/`WebSearchResult` 两类型 | protocol + translator |
| S1 question | question interrupt 自包含 `payload:{question}`（已实现，现转正） | translator（保留） |
| S5 分页 | **所有 list → `Page[T]`**：runs.list/listOpenInterrupts/workspace.*(5)/providers/models/tools/memory/background；请求加 cursor/limit | protocol 全接口 + server + dispatch |
| P0.1 | items.list 截断回填 `nextCursor`（不静默丢） | `server/items.go` |
| S5 | `ListItemsResponse` embed `Page[Item]`（已改 Data，改成 embed） | protocol + server |
| S4 durable | `RunEvent` 删 `Durable` 字段；hub 改由 `event.type` 推导；`custom` 帧自带 `durable?` | events.go + hub.go + runs.go |
| 事件类型 | 补 `run.progress`(StreamRunProgress)+`RunProgress` 类型（SSOT 完整；不强制 emit） | events.go |
| G2 能力 | `ServerFeatures` 固定 struct → 开放 `map[string]FeatureFlag`；加 `StreamingMethods` | capabilities.go + server.go |
| G3 | `ClientCapabilities.InterruptKinds`→`InterruptTypes`；下游 `SetInterruptKinds`/`interruptKinds` 跟改（或保留内部名，仅改 wire tag） | capabilities + lifecycle + chat service |
| S6 | `RunOutcome` 加 `Detail`；`runs.cancel` reason 回流 outcome.detail | protocol + translator + runs |
| S7 | `Usage` embed `ModelUsage`；`ModelUsage` 补 cacheRead/cacheWrite | protocol items.go + translator |
| S8 | `AnswerResponse.answers` `map[string]any`→`map[string][]string`；engine `answerText` 跟改 | protocol + engine/askuser |
| G4 | `ProblemData` 加 `Channel`/`DocUrl`；各错误落点填 channel | protocol + dispatch + translator |
| P3.8 | 删 `RuntimeLimits.MaxItemsPerSession`（不在契约、零用） | capabilities.go |
| §14 | （follow-up）从 Go SSOT 导出 OpenRPC/JSON Schema + 黄金样本契约测试 | 新增 infra |

### 批次方案（每批 build+vet+test 全绿、可独立 revert）

> 契约已据后端反馈 + 前端回复（见 `2026-06-07/REVIEW_FEEDBACK.md` + `REVIEW_RESPONSE.md`）**定稿**。
> 关键定稿点影响批次：N3 = **id 生成不改**（契约撤回顺序断言，排序锚 `createdAt`/`eventId`）；N7 = **§14 漂移闸
> 升为硬前置（B0）**；N1 = 删 `RunResult.costUsd`/`RunProgress.costUsd`；N2 = `WorkspaceFileChange`/`FileEdit`；
> N5 = `getDiff` 返回 `Diff{rows,truncated?}`。

- **B0 — 漂移闸（§14，硬前置，N7）**：从 Go `rpc/protocol` SSOT 导出 JSON Schema / OpenRPC + 一组黄金样本
  JSON wire 样本（每方法 req/resp、每类事件帧、tool `result` 各形状），后端 CI 往返校验。**先立闸再迁类型**——
  否则 G1 去领域化后 §4.4.2 富 result 约定会无声漂移。
- **B1 — 判别字段 + 版本 + 状态词（G3/S10）**：纯 rename（wire `kind`→`type`：Interrupt/InterruptResponse/
  ContextItem/FileChangeEntry 字段；`BackgroundTask.kind`→`category`；`ItemStatus inProgress`→`running` + PlanStep；
  版本 `2026-06-07`）。机械、低风险、面广。
- **B2 — ToolInvocation 领域中立化（G1/S2/S3 + 命令输出进 result）**：删联合改单一 `{name,arguments,result}`，
  删 `ToolInvocationKind`/`toolKind`；translator 工具投影重写为按 name 产 `result` JSON（bash→`{exitCode,output,
  outputTruncated?}`、grep/glob→`{hits:SearchHit[]}`、webSearch→`{results:WebSearchResult[]}`、edit/write→
  `{changes:FileEdit[]}`）；`SearchResult` 拆 `SearchHit`/`WebSearchResult`；`FileChangeEntry`→`FileEdit`、
  `FileChange`→`WorkspaceFileChange`；删 `_resume` hack（name 恒在 `tool.name`）。**最大批**。
- **B3 — 全 list 统一 Page[T]（S5/P0.1）**：13 个 list 方法签名 + server + dispatch；items.list 回填 `nextCursor`
  （不静默截断）；`getDiff`→`Diff{rows,truncated?}` + `limit?`（N5）；`ListItemsResponse` embed `Page[Item]`。
- **B4 — 能力开放化 + durable 去字段（G2/S4）**：`ServerFeatures` struct → 开放 `map[string]FeatureFlag` +
  `StreamingMethods`；删 `RunEvent.Durable`、hub 改由 `event.type` 推导（+ custom 帧 `durable?`）；
  `InterruptKinds`→`InterruptTypes`（wire；内部 `interruptKinds` 命名可留）。
- **B5 — 小字段对齐（S6/S7/S8/G4/P3.8 + N1）**：`RunOutcome.detail` + cancel reason 回流；**删 `RunResult.costUsd`/
  `RunProgress.costUsd`**（N1，总成本读 `usage.costUsd`）；`Usage` embed `ModelUsage` + cacheRead/cacheWrite 对称；
  `answers` `map[string]any`→`map[string][]string`（engine `answerText` 跟改）；`ProblemData` 加 `channel`/`docUrl`
  + 各落点填 channel；删 `RuntimeLimits.MaxItemsPerSession`。

> 顺序：**B0**→B1→B2→B3→B4→B5。每批一个 commit、message 记 why + 对应契约节。
> **id 不改**（N3 定稿）；**旧 store**：迁移落地后旧 `lyra.db` 历史 blob 不兼容，按 §12 直接丢（删库重建），不写 migration。

### ⚠️ 先决：新契约自身的问题（迁移前应先和前端敲定，免得对齐到一个带 bug 的契约）

新 `2026-06-07` 整体是大幅改进、几乎全采纳了第三/四轮建议。但审查发现**契约自身**几处新引入的问题，
**应先修契约再迁后端**（否则后端会忠实复制这些问题）：

- **N1 ⭐ 总成本字段重复（S7 修过头）**：`Usage extends ModelUsage`，而 `ModelUsage` 带 `costUsd` → 于是
  `RunResult.costUsd` 与 `RunResult.usage.costUsd`（继承来的）**都表示总成本**，二义。`RunProgress.costUsd`
  vs `RunProgress.usage.costUsd` 同样重复。旧契约 Usage 无 costUsd、总成本只在 `RunResult.costUsd`，本是干净的。
  → 建议：总成本**只留一处**。最干净是删 `RunResult.costUsd` / `RunProgress.costUsd`，总成本读 `usage.costUsd`；
  `costUsd` 在 ModelUsage 上保留是为 `byModel[*]` 的 per-model 成本。
- **N2 ◐ `FileChange` 与 `FileChangeEntry` 两个近义类型、枚举还分叉**：`FileChange{path, status:
  added|modified|deleted|renamed|untracked}`（workspace VCS 视图）vs `FileChangeEntry{path, type:
  add|modify|delete|rename, diff?}`（工具编辑结果）。命名撞车（FileChange/FileChangeEntry）+ 过去式 vs
  祈使式 + `untracked` 只在一个里。新人极易混。→ 建议：二者重命名区分（如 `WorkspaceFileChange` /
  `EditedFile`）或统一枚举词汇，并在契约写清"为何两个"。
- **N3 ⭐ §2.4 断言"id 字典序≈创建时间序、cursor 可直接用资源 id"——当前后端 SSOT 不满足**：实测
  `session=ses_+UUIDv4`（纯随机，**不可排序**）、`item=item_<runId>_<seq>`、`event=evt_<seq>`（进程内单调、
  重启重置），**无一内嵌时间戳**。契约把这条写成普适属性，但 SSOT 违反它。§4.11 靠"cursor 不透明、server
  可用任意编码"兜住了分页正确性，但 §2.4 的普适断言仍是假的——若前端据此"按 id 排序=时间序"会错。
  → 建议二选一：(a) 后端换**时间可排序 id**（ULID/KSUID 式，前缀保留）真正兑现 §2.4；或 (b) 把 §2.4 弱化为
  "id **可选**时间可排序；client 一律视为不透明、不据 id 推时间序"，排序靠 `createdAt`。

minor / note：
- **N4** §5.2 落点表写 `item.delta{toolOutput}` → `tool.result`，更精确是 `tool.result.output`（result 现在是
  对象）。措辞收紧即可。
- **N5** `workspace.getDiff` 返回裸 `DiffRow[]` 无上限——全仓 diff 可能巨大，绕过了 Page 的"no silent caps"
  纪律。可加 limit / 截断信号（如 grep 的 `total`）。
- **N6** 删 `RunEvent.durable` 后，SSE 层要靠 `event.type`（+ custom.durable）判该不该带 `id:`/可重放——
  transport 因此要读业务事件语义。这是 S4 的既定代价，可接受，记录在案。
- **N7** G1（通用 tool + 非规范展示约定 §4.4.2）落地后，富渲染形状不再机器保证——**§14 黄金样本契约测试
  从"建议"变成"必需"**（否则 §4.4.2 约定会无声漂移，正是它要防的）。迁移时 B6 不应再当可选。
