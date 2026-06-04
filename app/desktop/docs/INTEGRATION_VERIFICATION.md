# Lyra 前端 → 后端反馈（本轮 · e2e）

> **日期**：2026-06-05
> **被测后端**：Lyra Runtime（`23ef1a0`，运行中），`http://127.0.0.1:17171`，streamable HTTP，`protocolVersion 2026-06-03`。
> **前端**：本仓库 frontend（HEAD）。
> 本文记：① 上轮问题的活体闭环；② 本轮 e2e 探针**新发现**的后端坑；③ **新工具调用契约已定稿**（API.md）+ lockstep 实施清单。
> 契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准。

---

## 0. 上轮问题闭环（活体验证 PASS）

针对 `23ef1a0` 实跑 HITL deny 探针（start → 审批 → `deny` resume）：

- **B3-1** 续延 run 发 `run.started`：✅ 续延流**第一帧**即 `run.started`，`parentRunId` 指向被中断 run。
- **deny 终态**：✅ 被拒 toolCall = `status:"incomplete"` + `error.type:"denied_by_user"`。前端已据此渲染中性「拒绝」态（非绿 ✓、非失败红）。
- **eventId 流内单调**：✅ 单条流内 `evt_…` 零填充、字典序 == 到达序。

对后端「进程重启专项」两坑的前端处理：

- **坑 A（eventId 重启归零）**：前端**零改动**。历史重建本就按 `items.list` 返回序渲染、`createdAt` 只做显示不当排序键（符合更正后指导）；原排期的「eventId 实时重排」**重新定范围为：只在单流内去重、且仅当实现 Last-Event-Id 重连时才需要**，不做跨 run 全局重排。
- **坑 B（幽灵 running run）**：后端在 **RunRef 层**对账（`outcome.error.type:"run_lost"`），但**前端当前不读 `runs.list`/`RunRef`**，该对账到不了 UI。前端真正的幽灵在 **item 层**——`items.list` 把崩溃 run 的末个 item 仍以 `status:"inProgress"` 返回，hydration 折成永久转圈块。已在 fold 加不变量修掉（`item.completed` ⟹ 已 settle ⟹ `inProgress→incomplete`）。**无需后端动作**，记录以同步认知。

---

## 1. 本轮 e2e 新发现

探针：plan 模式跑一个会触发文件工具的 prompt（`mode:"plan"`，`deepseek` provider）。

### E1 — 工具级失败被升级成致命 run error（与 API.md §8.1 矛盾）⭐

**现象**：模型调用 `glob` 工具失败，**整个 run** 以 `run.finished{ type:"error" }` 收场：

```
item.completed(toolCall) status=incomplete
  tool  = {"kind":"search","name":"glob"}
  error = {"type":"tool_failed","detail":"fs.glob: fs.LocalExecutor.Glob: exit status 1"}
run.finished
  outcome = {"type":"error","result":{"error":{
    "type":"internal_error",
    "detail":"chat.toolCallInvoker.invokeToolCalls: tool \"glob\" failed: fs.glob: fs.LocalExecutor.Glob: exit status 1"
  }}}
```

**契约**：`API.md §8.1`（投递通道表，行 b）+ §4（`toolCall.error` 是"工具级失败的统一结构化落点……工具失败通常不终止 run"）明确——**工具级失败落在对应 `toolCall` item 的 `error` + `status:"incomplete"`，run 多半继续**（让模型看到错误、自行调整）。这里 toolCall item 的 `error` 已经正确落了（`tool_failed`），但 invoker **又把同一个工具失败包成 `internal_error` 把整条 run 打死**了。

**归属**：后端 `toolCallInvoker.invokeToolCalls` 把单个工具的 `tool_failed` 当致命错误向上抛，而非"记在 item 上、让 run 继续"。

**建议**：工具调用失败只体现在 toolCall item（`status:incomplete` + `error.type:tool_failed`），把错误回灌给模型继续下一步；**不要**升级成 `run.finished{error}`。只有真正的引擎/基础设施故障才配 `internal_error` + 终止 run。

### E2 — `internal_error.detail` 泄漏内部调用栈

E1 里的 `detail` 是 `chat.toolCallInvoker.invokeToolCalls: tool "glob" failed: fs.glob: fs.LocalExecutor.Glob: exit status 1`——把 Go 包/方法路径直接抛给了客户端。

**契约**：`API.md §8.2` 业务 error 走结构化 `type` + `detail`，`detail` 应是面向用户/agent 的可读说明，不该带实现内部的调用路径。**建议**：`detail` 收敛成干净文案（如 `glob failed: exit status 1`），内部栈走服务端日志，别上 wire。

### E3 —（观察，需后端确认）plan 模式执行了工具

`mode:"plan"` 下，agent 没有"只产出计划 + 等批准"，而是直接发起了 `glob` 工具调用（随后撞上 E1）。

**问题**：plan 模式的语义是什么？是否应禁止/门控副作用工具、只产出计划？若 plan 模式允许只读工具（glob/read），那也不该因一次只读工具失败而 E1 式炸 run。需要后端明确 plan 模式的工具策略。**非阻塞，先记为问题。**

### E4 —（轻微）`/v2/info` 的 `capabilities.providers` 为空，与 `providers.list` 不一致

`GET /v2/info` 返回 `capabilities.providers: []`，但 `providers.list` 返回 4 个（anthropic/deepseek/moonshot/openai，deepseek 已配 key）。

**问题**：`capabilities.providers` 语义是什么？若意指"可用/已配置 provider"则为空是错的；若是别的含义则需在 `API.md` 写清，避免客户端误判"无 provider"。**非阻塞。**

### E5 —（nit）JSON-RPC `id` 必须是字符串，但未在 TRANSPORT.md 写明

整数 `id` 被拒：`{"code":-32600,"message":"invalid_request","detail":"id must be a JSON string, got int64"}`。JSON-RPC 2.0 本身允许 number id；`TRANSPORT.md` 所有示例用字符串但未声明这是**硬约束**。前端 client 一直发字符串、不受影响，但其他客户端会踩。**建议**：`TRANSPORT.md §2` 显式写明"`id` 必须为 string"，或放宽接受 number。

---

## 2. 工具调用契约：已钉进 API.md（lockstep 实施清单）

约定不再 park——**新的工具调用契约已定稿写进 `API.md`**（§4.4 / §4.5 / §4.8 / §6.1），请后端按它改 wire，前端同步切换并删容错解析。设计原则是**通用 + 特殊**（按"变体集合开 / 闭"取舍，参考 OpenAI Codex app-server），不是一刀切统一。

### 2.1 `ToolInvocation`（§4.4）——五个变体

```ts
type ToolInvocation =
  // 特殊：闭集 + 结构丰富 → 强类型（kind 即身份，无冗余 name）
  | { kind: "commandExecution"; command: string[]; cwd?; exitCode?; durationMs? }
  | { kind: "fileChange";       changes: FileChangeEntry[] }            // {path, kind:add|modify|delete|rename, diff?}
  | { kind: "search";           query: string; results?: SearchHit[] }        // 本地 grep/glob：{path, lineNumber?, snippet?}
  | { kind: "webSearch";        query: string; results?: WebSearchResult[] }  // 网络：{title?, url, snippet?, faviconUrl?}
  // 通用：开集 / 不可枚举 → 一个信封兜底（MCP / 动态 / 子 agent / 自定义）
  | { kind: "tool";             name: string; arguments: Record<string, unknown>; result?: unknown };
```

后端实施要点：
- **不再有 `kind`↔`name` 双重身份**：命令用 `command` argv、通用用 `name`，没有工具同时带两者（这是上轮 W1 漂移的根）。
- **`arguments` 永远是已解析 JSON 对象**，完成项 / 审批 payload **绝不回传 JSON 字符串**（消除双重转义）；流式仍走 `toolArguments` 文本增量。
- **`result` 是 best-effort JSON**：首选对象，允许任意 JSON 值，**绝不双重编码**；命令 stdout 走 `toolOutput` 流式，`exitCode`/`durationMs` 完成时落定。
- **bash → `commandExecution`、grep/glob → `search`、web → `webSearch`、MCP/子agent → `tool`**（不再发 `{kind:"command",name:"bash"}` / `{kind:"search",name:"glob"}` 这类）。

### 2.2 HITL payload（§4.8 / §6.1）

- `Interrupt` 改为按 kind 的判别联合；**approval / toolResult 的 payload 复用 `ToolInvocation`**（`payload.tool`）——审批不再猜 command 在哪 / 处理字符串转义。question 无 payload（内容在 question Item 上）。
- `ToolResultResponse` 由 `output: string` 改为 `result?: unknown + error?`，与 `ToolInvocation.result` 同形。

### 2.3 工具级 error type（§8.4，澄清）

`toolCall.error.type` 的 first-party 取值（通道 b、无数字码）已在 §8.4 明确：`tool_failed` / `denied_by_user` / `timeout` 等；
注意与 §8.2 的 `tool_denied`（**策略**拒绝 `tools.invoke`，RPC 级 `-32012`）区分——`denied_by_user` 是**用户** HITL 拒绝（item 级，无码）。

> 落地节奏建议：**后端先按 §4.4 发新 wire**（先 `commandExecution` + `tool` 覆盖最常见路径，再 fileChange/search/webSearch），前端随后 lockstep 切 `switch(kind)` + 删 `parseArgs` 容错。中间态别混用新旧。

---

> 本轮 e2e 坑优先级：**E1 最高**（违反 §8.1、任何工具失败都会终结对话），E2 次之（同处一改），E3 需语义确认，E4/E5 轻微。
> 工具契约见上 §2 + `API.md`（§3.1 新增「一次 run 端到端走查」可作总览入口）。
