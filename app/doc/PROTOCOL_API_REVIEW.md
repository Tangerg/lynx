# 协议 / API shape 评审 —— Lyra Runtime Protocol vs Codex / opencode

> **目的**：审视 `app/runtime ↔ app/desktop` 的前后端交互协议(核心 API + 旁路 API)的 **shape 是否足够合理**,并给出"更合理 / 优雅 / 易扩展 / 易维护"的方向。配套 [`RUNTIME_COMPARISON.md`](./RUNTIME_COMPARISON.md)(引擎能力)/ [`DESKTOP_COMPARISON.md`](./DESKTOP_COMPARISON.md)(GUI 形态)——**本篇只谈协议设计本身**(命名 / 核心模型 / 事件流 / 错误 / 版本协商 / 扩展机制 / sidecar / transport / codegen)。
>
> **方法**：三方协议**源码/规范级**第一手核对——lynx 看 `docs/protocol/{API,AUX_API,TRANSPORT}.md` + `internal/delivery/protocol` + `rpc/methods.ts`;**Codex** 看 `~/Desktop/codex/codex-rs/app-server-protocol`(Rust app-server);**opencode** 看 `~/Desktop/opencode/packages/{server,sdk}`(Effect/OpenAPI)。基线 **2026-06-23**。
>
> **一句话元结论**：lynx 协议已相当成熟,且在 **type-唯一判别 / durable-ephemeral 不变量 / 领域中立核心+三扩展缝 / HITL R 模型 / 开放 features 协商 / plugin 命名空间 / 核心-旁路分离** 这几条上**比两家更有原则**,不要动。**唯一显著的可维护性缺口**:`API.md §14` 自己定的 **codegen + 黄金样本漂移闸只立了规范、没落地**(现 Go↔TS 靠人工 review 同步),而 Codex(Rust→ts-rs 生成)、opencode(Zod→OpenAPI→生成 SDK)都是 schema-first 自动生成——**而这层闸恰恰是 lynx"领域中立核心"安全的前提**,是头号该补的。

---

## 0. 三方定位(协议形态对照)

| | 核心模型 | wire | 流式 | schema 来源 |
|---|---|---|---|---|
| **lynx** | **Session → Run → Item**(Run 一等) | **JSON-RPC 2.0**(MCP-inspired,严格带 `jsonrpc:"2.0"`) | streamable HTTP:事件走调用自身的 POST 响应流,per-run hub;单方法 `notifications.run.event` | Go `internal/delivery/protocol` 手写,**TS 手写同步(review)** |
| **Codex** | **Thread → Turn → ThreadItem**(Turn≈Run,ThreadItem 强类型变体) | **JSON-RPC-lite**(自定义信封,**不带** `jsonrpc` 字段)over stdio/unix/ws | 通知独立异步通道;delta 通知(AgentMessageDelta…) | **Rust SSOT → ts-rs 宏生成** TS union + JSON-Schema |
| **opencode** | **Session → Message → (Part as events)**(Step≈Run,隐式) | **REST / OpenAPI 3.1**(Effect HttpApi)over HTTP | **SSE**(`GET /api/event` 事件总线),事件源化 | **Zod/Effect schema → OpenAPI → `@hey-api` 生成多语言 SDK** |

> 三方都走"引擎与前端分离 + 流式 turn",但 lynx 是**唯一 JSON-RPC + streamable-HTTP(事件随调用流回、无常开总线、无连接身份)**的;Codex 走类 MCP 的 stdio/ws + 独立通知通道;opencode 走 REST + 常开 SSE 总线 + 事件源。

---

## 1. 速查矩阵(协议设计维度)

> ✅ 有且成熟 · 🟡 部分/弱 · ❌ 无 · ★ 该项标杆

| 维度 | Codex | opencode | **lynx** |
|---|---|---|---|
| 判别字段一致性 | 🟡 `type` tag(多处) | 🟡 `type` 字段 | ★✅ **一律 `type`、`kind` 禁上 wire**(无例外硬规则) |
| 核心是否领域中立 | ❌ ThreadItem **强类型变体**(commandExecution/fileChange/mcpToolCall…,新工具动协议) | 🟡 message+part(part 偏内容型) | ★✅ **领域中立 `ToolInvocation` 信封 + 客户端展示注册表**(新工具不动协议) |
| 流式 durable/ephemeral 保证 | 🟡 delta+completed(无成文不变量) | 🟡 事件源(part 从事件装配) | ★✅ **"每个 ephemeral 必有 durable 落点"成文协议级不变量 + 推导表** |
| 能力协商 | ✅ initialize + capability flags + **experimental 字段/方法门控** | ❌ 无握手(调 list 路由探能力) | ✅ initialize + **开放 `features` map(additive 不 bump)** + interruptTypes + optOut |
| HITL 模型 | 🟡 **server→client request**(审批/工具,9 法;多 client 下耦合) | 🟡 REST 轮询 + reply 端点 | ★✅ **R 模型**(run park-on-interrupt,durable,任意 client 可 resume;无 server→client RPC) |
| 扩展机制清晰度 | 🟡 experimental 宏 | 🟡 加路由/加事件 | ★✅ **三扩展缝(Item/state/custom)成文选择指南 + `plugin:` 命名空间** |
| 错误模型 | JSON-RPC code + `CodexErrorInfo` 业务枚举 | HTTP status + 命名 error 类型 | JSON-RPC `code` + `error.data.type` 符号名(对标 RFC 9457 ProblemData),**不映射 HTTP status** |
| 版本与协商 | v1/v2 共存 + experimental 门控 | ❌ URL 无版本(仅 identifier tag) | ✅ HTTP `/v2/` epoch + 日期 `protocolVersion` 协商(两层不重复) |
| 核心 / 旁路分离 | ❌ 全在 app-server | ❌ 全在 REST | ✅ **core(sessions/runs/items)与旁路 AUX(workspace.* / sidecar)显式分文档分面** |
| **schema codegen / 漂移闸** | ★✅ Rust→ts-rs 生成 + 导出 | ★✅ Zod→OpenAPI→生成 SDK | ❌ **手写两份、review 同步(§14 立了规范未落地)** —— 真 gap |
| sidecar | ❌(initialize 即握手) | 🟡 `/api/health` + `/api/location` | ✅ `/v2/info` + `/v2/health`(flat、no-auth、不走 envelope) |

---

## 2. lynx 已经更优的(别动,继续守)

这些是 lynx 相对两家的**真实设计优势**,均与项目"薄核 / 三形态变体 / 窄腰 / 一个扩展机制 / 库优于框架"哲学一致:

1. **领域中立核心 `ToolInvocation` + 客户端展示注册表**(§4.4 / §13):Codex 把 `commandExecution`/`fileChange`/`mcpToolCall`/… 做成 wire 一等强类型变体——**每加一种工具就动协议**。lynx 核心只认中立信封,富渲染(shell 的 `{exitCode,output}`、grep 的 `{hits}`、diff)走前端展示注册表——**新工具零协议改动**。这是更纯的薄核,扩展性更好。**但有前提**:富 `result` 形状成了"非规范展示约定",其前后端一致性不再被 wire 机器保证——只有 §14 的黄金样本闸能防漂(见 §3.1)。
2. **durable/ephemeral 协议级不变量**(§5.2):"丢弃每个 ephemeral 事件,客户端仍必得正确终态"+ 一张 `event.type → durable → 权威落点` 推导表 + "新增无落点的 ephemeral = 协议违规"硬规则。两家都没成文化这层——这是 lynx 独有的、根除"回放/重连/opt-out 丢内容"那类 bug 的设计。
3. **`type` 唯一判别(kind 禁上 wire)**(§2.1):消除"这看 type、那看 kind"的认知税与拼错判别字段的无声 bug。两家用 `type` 但无此无例外硬规则。
4. **HITL R 模型**(§6,无 server→client RPC):Codex 用 server→client request 等客户端应答——多 client 下"server 在等哪个 client"是其自审都点名的耦合点。lynx 的 run park-on-interrupt + durable interrupt + 任意 client `runs.resume`,在多 client / 重连 / 重启下更干净。**这是设计优势,不是缺失。**
5. **开放 `features` map 协商**(§9):advertise 新能力=加 key,老 client 忽略未知 → 不 bump 契约。opencode **完全无协商**(靠调 list 路由探);Codex 有 capability flags。lynx 的对称开放 map + interruptTypes(防挂死)+ optOutNotificationMethods(抑高频)更完整。
6. **三扩展缝 + `plugin:` 命名空间**(§2.6 / §11):Item/state/custom 各有边界与选择指南;first-party 裸符号、第三方 `plugin:<name>/` 前缀防撞名——统一一条用于所有 keyspace(custom 名 / state key / error type / 开放枚举)。两家都没这层成文扩展契约。
7. **核心 / 旁路(AUX)显式分离**:core 只 sessions/runs/items;git/文件/搜索/mcp/hooks/事件流归 AUX_API + sidecar。两家不分(全 app-server / 全 REST)。这让核心协议保持薄,旁路独立演进——更易维护。
8. **Run 作为一等资源**(Session→Run→Item)+ `parentRunId`(延续链)/ `spawnedByItemId`(子 run 树):resume/fork/subagent 树建模显式,不靠时间猜。Codex 的 Turn、opencode 的隐式 Step 都不如这层显式。

---

## 3. 真正的 gap 与改进方向

### 3.1 【头号·可维护性】落地 §14 的 codegen + 黄金样本漂移闸

**现状**:`internal/delivery/protocol`(Go)是 SSOT,但前端 TS wire 类型**手写、靠 review 同步**(见 memory `project_lyra_no_protocol_ts_codegen`)。`API.md §14` 自己把"从 Go SSOT 导出 OpenRPC + JSON Schema + 黄金样本契约测试 + CI 卡 drift"定为**硬前置项**,但尚未落地。

**两家对照**:Codex `Rust → ts-rs 宏 → TS union + JSON-Schema`(SSOT 单一、生成);opencode `Zod/Effect schema → OpenAPI 3.1 → @hey-api 生成多语言 SDK`(schema 即 SSOT,校验+文档+SDK 一处定义)。**两家都不存在"手写两份 wire 类型"。**

**为何这是头号**:① 它是纯维护性收益(消除 `items` vs `data` 那类字段名漂移——§14 自己点名的历史 bug);② **它是 §2「领域中立核心」安全的前提**——富 `result` 形状不再被 wire 联合机器保证,唯一能防其前后端无声漂移的就是黄金样本 + 导出 schema。没有它,薄核选择反而是漂移负债。

**已知阻塞 + 务实路径**(memory 记:Go flat-struct 不直接映射到契约的 TS 判别联合):
- **第一步(低成本、绕开阻塞):黄金样本契约测试**——一组 canonical JSON wire 样本(每方法的请求/响应、每类事件帧),前后端 CI 各自往返校验。**它不需要解决 struct↔union 映射**(只比对 JSON),却能当场抓住 §14 想消除的两类 drift。**先立这层。**
- **第二步:从 Go SSOT 导出 OpenRPC(方法表)+ JSON Schema(数据类型)**作为非 Go/非 TS 客户端的单一对接物。
- **第三步(可选):判别联合感知的 TS 生成器**,或把 wire 形状改成生成器友好的表达——这是真正难的一步,但有了第一/二步后不再是漂移风险的关键路径。

> 这条与 RUNTIME_COMPARISON 的结论独立:那篇说"无 protocol→TS codegen 是 by-design(flat-struct 不映射 union)"——但 §14 明确这不是终点而是**待落地的硬前置**。务实解是**先上黄金样本闸**(绕开映射难题),而非继续纯靠 review。

### 3.2 【次·命名/SRP】留意 `workspace.*` 渐成 god-namespace

`workspace.*` 现已横跨:VCS(listFileChanges/getDiff/getFileHead)、文件读(listFiles/readFile)、搜索(grep)、技能(listSkills)、AgentDocs(listAgentDocs)、事件流(subscribe)、recipes.*、hooks.*、mcp.*、code.*。其中 **code-intel / mcp / hooks / recipes 本是独立 domain**(各有自己的 §)。这是"一个命名空间横跨多 domain"的 SRP 信号。

- **改法(轻、非紧急)**:把已成独立 domain 的子面**提为顶层**(如 `codeintel.*` / `mcp.*` / `hooks.*` / `recipes.*`),`workspace.*` 收敛回"工作树视图"(VCS+文件+搜索+事件)。**破坏性命名改动,按规矩先咨询**——dev 阶段无兼容包袱,值得一次性改对(第一法则)。触发条件:`workspace.*` 子面继续增多时做。

### 3.3 【次·演进粒度】借 Codex 的 field/method 级 experimental 门控

Codex 用 `#[experimental("path")]` 在**字段/方法级**门控,client `initialize` 声明 `experimental_api:true` 解锁——新表面可不 bump 版本、藏在 flag 后渐进上线。lynx 的 `features` map 已覆盖**方法级**门控 + "client 忽略未知字段"已覆盖大半,但无**字段级 experimental** 约定。

- **改法(可选)**:为"已在 wire 上、但语义未定稿"的字段加一条 `x-` / `experimental` 命名约定(或并入 `features` 的子能力 map `{enabled, ...}` 形态——协议已支持 `FeatureFlag = boolean | {enabled, ...}`)。低优先,按需。

### 3.4 sidecar / readiness(可选)

`/v2/info` + `/v2/health` 已够(opencode 也只 `healthy:true`)。若未来要无人值守/编排,可考虑 liveness vs readiness 拆分。**当前不必做。**

---

## 4. 明确不学(两家的做法不适配 lynx 哲学)

- **Codex 的强类型 ThreadItem 变体**(commandExecution/fileChange/mcpToolCall 作 wire 一等类型):违背薄核——新工具动协议。lynx 的中立信封 + 展示注册表是更优解(§2.1)。**前提是把 §3.1 的黄金样本闸补上。**
- **Codex 的 server→client JSON-RPC request**(审批/工具/elicitation):lynx 的 R 模型已更干净地解决 HITL(§13 明确不做 server→client request),多 client 友好。不学。
- **Codex 的"非 JSON-RPC-lite"自定义信封**(去掉 `jsonrpc` 字段):lynx 严格 JSON-RPC 2.0(借 MCP SDK envelope),工具生态兼容性更好。不学。
- **opencode 的无协商**(靠调 list 路由探能力):lynx 的 features map 协商更显式可控。不学。
- **opencode 的 REST-per-resource + 常开 SSE 总线 + `x-opencode-directory` 带外 header**:lynx 有意 cwd 走 body 不走带外 header(§15 安全不变量)、streamable-HTTP 不要常开总线(TRANSPORT 反不变量)。不学。
- **opencode 的事件源化(message 从事件日志投影)**:lynx 的 Item 即 durable 单元 + `items.list` 即历史,已满足;事件源是更重的存储模型,YAGNI。

---

## 5. 建议优先级

1. **【高·维护性】§3.1 第一步:立黄金样本契约测试**(前后端 CI 往返校验 canonical JSON 样本)——绕开 struct↔union 映射难题,当场防 wire drift,且是"领域中立核心"安全的前提。**这是唯一头号项。**
2. **【中】§3.1 第二步:从 Go SSOT 导出 OpenRPC + JSON Schema**——非 Go/非 TS 客户端的单一对接物 + 机器可读漂移闸。
3. **【中·按需】§3.2 `workspace.*` 拆 god-namespace**(codeintel/mcp/hooks/recipes 提顶层)——破坏性命名改动,先咨询;触发条件(子面继续增多)到了再做。
4. **【低·可选】§3.3 field 级 experimental 门控**(并入 `FeatureFlag` 对象形态);§3.4 readiness 拆分——按需。

> 总判:**协议 shape 已经合理且在多处领先**;把"更优雅/易扩展"继续守在薄核+三扩展缝+R 模型上即可,**真正要补的"易维护"是 §14 那层一直没立起来的漂移闸**——而它同时是薄核选择的安全前提,故应优先。落手前涉及破坏性命名/契约改动的(§3.2),按 `app/desktop/CLAUDE.md` 先出 scope + 影响面 + 备选,确认再动。
