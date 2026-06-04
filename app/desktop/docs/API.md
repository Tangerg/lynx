# Lyra Runtime Protocol（正式定稿 v2）

> **状态：正式契约（canonical, frozen baseline）。** 本文是 Lyra 客户端 ↔ Lyra Runtime 的唯一 wire 契约真相源。
> 物理传输见 [`TRANSPORT.md`](./TRANSPORT.md)。
>
> 类型命名以后端 Go interface（`lyra/rpc/protocol`）为机械 SSOT，经 codegen 派生 TS / 其他语言类型；
> 本文与生成产物**同名零映射层**。
>
> `protocolVersion`: **`2026-06-03`**（本定稿基线版本）。

---

## 目录

- §0 模型与概念
- §1 Wire 格式（JSON-RPC 2.0）
- §2 命名规范
- §3 Lifecycle（握手 / 关闭 / 取消）
- §4 数据类型目录（schemas）
- §5 流式（事件信封 / Item·Run·State 事件 / 不变量）
- §6 Human-in-the-Loop（R 模型）
- §7 方法参考（逐方法：输入 / 输出 / 错误 / 示例）
- §8 错误（两条投递通道 + 错误码表）
- §9 Capabilities 与协商
- §10 历史 / 重连 / 恢复
- §11 版本规则
- §12 v2 明确不做
- §13 安全不变量汇总

---

## 0. 模型与概念

Lyra Runtime 是一个**本地 agent runtime**。客户端可以是 web UI、桌面外壳（Wails/Tauri/Electron）、TUI 或另一个
本地进程。协议是 **JSON-RPC 2.0**，跑在多种 transport（InProcess / IPC / HTTP）上，语义在所有 transport 上一致。

### 0.1 三级资源模型

```text
Session            会话；绑定一个工作目录 cwd          id: ses_…
  └─ Run           一次 agent 执行，从用户输入到一个 outcome    id: run_…
       └─ Item     run 内一个 durable 工作单元                  id: item_…
```

- **Session**：一次对话，绑定 `cwd`。
- **Run**：一次 agent 执行；以一个 `RunOutcome` 收尾（completed / error / maxSteps / maxBudget / canceled / interrupt）。
- **Item**：run 内一个 durable 工作单元 —— `userMessage` / `agentMessage` / `reasoning` / `plan` / `question` / `toolCall`。

**`Item` 是历史与流式的唯一原语**：流式推 Item 生命周期事件（`item.started → item.delta* → item.completed`），
历史 `items.list` 返回 completed Item。**没有独立的 `Message` 资源类型**；"消息"就是 `userMessage`/`agentMessage`
两种 Item。

### 0.2 工程目录模型（cwd）

- `cwd` 是**项目身份 + 文件系统工具根**。`Session.cwd` 建会话时设定，可经 relocate（`sessions.update`）更新。
- runtime **不持有连接级的 active project**；workspace 读方法显式收 `cwd?`，缺省 = runtime serve 目录（`ServerInfo.cwd`）。
- `Project` 是按 `Session.cwd` 分组的**派生视图**：无不透明 id、无 active 标记。
- `projectRoot` 只是**配置发现根**（向上找到最近含 `.git` 的祖先，找不到回落 cwd），**不取代** cwd 作为身份/工具根。

### 0.3 HITL 采用 R 模型（收尾 + 延续 run）

agent 需要人介入（审批 / 提问 / client 侧工具结果）时，**当前 Run 结束**，以 `RunOutcome.type="interrupt"` 收尾、
资源全释放；待解项作 `OpenInterrupt` 落持久状态。客户端用 `runs.resume` 起一个**延续 Run**（`parentRunId` 串接）来
应答。无"活跃挂起 run"、无被占用的 goroutine。详见 §6。

### 0.4 收敛点

- **一个下行事件方法**：`notifications.run.event` 承载 run/item/state 全部事件（HTTP 上随对应流式调用的响应流投递，见 TRANSPORT §6.4）。
- **一个终态信号**：`run.finished{ outcome }`，判别式 outcome。
- **一个 HITL 恢复口**：`runs.resume`。

---

## 1. Wire 格式（JSON-RPC 2.0）

```ts
type Message = Request | Response | Notification;

interface Request  { jsonrpc: "2.0"; id: string; method: string; params?: unknown; }
interface Response { jsonrpc: "2.0"; id: string; result?: unknown; error?: RPCError; }
interface Notification { jsonrpc: "2.0"; method: string; params?: unknown; }   // 无 id

interface RPCError { code: number; message: string; data?: ProblemData; }
```

规则：

- envelope `id` 一律 **string**；Response 的 `id` 匹配 Request。
- Response 必须恰好含 `result` 与 `error` 之一。
- **不支持** JSON-RPC batch。
- **不支持** server→client request（HITL 用"通知 + 后续 client request"，§6）。
- 元数据（trace / 协议版本 / 幂等键 / 门禁 token / 流游标）走**带外通道**，不进 `params`；
  业务数据（`cwd` / `sessionId` / `runId` 等）走 `params`。判定见 [`TRANSPORT.md`](./TRANSPORT.md) §2。

---

## 2. 命名规范

### 2.1 字段与枚举

- 字段名、枚举值一律 **camelCase**。
- 缩写**白名单**（写死，其余全词）：`id` / `url` / `mime` / `cwd`。
- 单位用显式后缀全词：`maxBudgetUsd`、`expiresAt`（ISO-8601 串）、`sizeBytes`。

### 2.2 资源 id（server 生成 + 类型前缀）

业务资源 id **一律 server 生成**：

| 资源 | 前缀 | 说明 |
| --- | --- | --- |
| Session | `ses_` | |
| Run | `run_` | 幂等见 §7；client 不传 |
| Item | `item_` | HITL 关联键（§6） |
| Attachment | `att_` | |
| Background task | `tsk_` | |
| Event | `evt_` | **单个 root run stream 内单调**，流续传/去重锚 |

- 客户端只生成两种非业务 id：JSON-RPC envelope `id`、幂等键。
- **`eventId` 作用域**：在单个 **root run stream** 内单调（不是 per-Run）；一条 root 流复用根 Run + 所有子孙 Run
  的事件。`runs.resume` 起的延续 Run = **新 root stream**（新 runId + eventId 从头）；subagent Run = **非 root**，
  事件并入父 stream、共享父序列。

### 2.3 事件名

小写 `domain.action`：`run.started` / `run.finished` / `item.started` / `item.delta` / `item.completed` /
`state.snapshot` / `state.delta` / `custom`。运行时自有行为**必须**用一等事件/Item 类型；`custom` 只留第三方扩展，
其 `name` 遵 §2.5 命名空间。

### 2.4 方法名

`<domain>.<verb>`，HTTP URL 保留点（不斜杠化）。例：`runs.start` / `items.list` / `workspace.getDiff` /
`workspace.mcp.listServers`。

### 2.5 第三方扩展命名空间（防撞名）

**唯一**的可枚举标识命名约定，统一适用于所有"first-party 与第三方共用一个 keyspace"的扩展缝：

- **first-party**（runtime 自身 / 内置）用**裸符号**（`session_not_found` / `progress`）。
- **第三方插件**产出的同类标识一律加前缀 `plugin:<pluginName>/<symbol>`（如 `plugin:acme/progress`），
  避免与 first-party 及彼此撞名。

适用面（**全部沿用此一条**，不各自再定）：

| 缝 | 标识 | 见 |
| --- | --- | --- |
| `custom` 事件 | `name` | §5 / §9 |
| 共享 `state` | 顶层 key | §5 |
| error | `type` | §8.4 |

client 路由：裸符号按 first-party 集匹配；`plugin:` 前缀按 `<pluginName>` 分发。

---

## 3. Lifecycle

```text
connect → runtime.initialize → operate → runtime.shutdown → disconnect
```

握手完成前 runtime 拒绝业务方法。版本协商：client 报想用版本 → server 支持则回同版本，否则回自己支持的最新版本
→ client 无法 fall back 必须断开。`runtime.initialize` 是**唯一**因版本不匹配硬断开的方法。

取消有两个不同对象：

| 对象 | 信号 | 效果 |
| --- | --- | --- |
| 在飞的 JSON-RPC request | `notifications.canceled`（client→server 通知，带被取消请求的 envelope `id`） | 取消一个慢请求 |
| agent run | `runs.cancel`（request，带 `runId`） | 硬终止在跑的 run（`outcome:canceled`） |

网络断开**不**取消 run。

---

## 4. 数据类型目录（schemas）

> 本节是所有 wire 类型的权威定义。§7 方法参考引用这里的类型，不重复定义。

### 4.1 Session / Project

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface Session {
  id: string;                          // ses_…
  title: string;
  status: SessionStatus;
  model: string;                       // 默认模型 id
  cwd: string;                         // 绝对路径，server 已解符号链接
  projectRoot?: string;                // 派生：最近 .git 祖先，无则 = cwd
  cwdMissing?: boolean;                // cwd 在磁盘失联 → 降级纯聊天 + 可 relocate
  createdAt: string;                   // ISO-8601
  updatedAt: string;
  usage?: Usage;                       // 本 session 累计
  metadata: Record<string, unknown>;
}

interface Project {                    // 派生视图：distinct Session.cwd；无 id、无 active
  cwd: string;                         // 唯一身份
  name: string;                        // basename(cwd)
  projectRoot?: string;
  branch?: string;                     // git 分支，best-effort 装饰
  sessionCount: number;
  lastActiveAt?: string;
  cwdMissing?: boolean;
}
```

### 4.2 Run

```ts
interface RunRef {
  id: string;                          // run_…
  sessionId: string;
  spawnedByItemId?: string;            // child-of：本 Run 是子 agent，由该 toolCall Item 派生
  parentRunId?: string;                // continuation-of：本 Run 是 resume/edit 的延续
  status?: "running" | "finished";
  outcome?: RunOutcome;                // status=finished 时给
  createdAt?: string;
  finishedAt?: string;
}

type RunOutcome =
  | { type: "completed"; result: RunResult }
  | { type: "error";     result: RunResult }
  | { type: "maxSteps";  result: RunResult }   // agent 步数上限（Run 即一次 turn，故不叫 maxTurns）
  | { type: "maxBudget"; result: RunResult }   // 成本上限（含子 agent 子树）
  | { type: "canceled";  result: RunResult }
  | { type: "interrupt"; interrupts: Interrupt[] };   // ★可恢复；Run 已结束、资源释放

interface RunResult {
  usage?: Usage;
  costUsd?: number;                    // 模型不在定价表时省略，不臆造 0
  steps?: number;
  error?: ProblemData;                 // outcome.type=error 时给
}
```

> `spawnedByItemId`（子）与 `parentRunId`（延续）是**两种不同关系，永不互相复用**。

### 4.3 Item

```ts
type ItemStatus = "inProgress" | "completed" | "incomplete";   // incomplete = 被中断/取消而未完成

interface ItemBase {
  id: string;                          // item_…
  runId: string;                       // 所属 Run（子 agent 的 item.runId = 子 Run）
  status: ItemStatus;
  createdAt: string;
}

type Item =
  | (ItemBase & { type: "userMessage";  content: ContentBlock[] })
  | (ItemBase & { type: "agentMessage"; content: ContentBlock[] })
  | (ItemBase & { type: "reasoning";    text: string; redacted?: boolean })
  | (ItemBase & { type: "plan";         steps: PlanStep[] })
  | (ItemBase & { type: "question";     question: Question })
  | (ItemBase & { type: "toolCall";     tool: ToolInvocation; safetyClass?: string; error?: ProblemData });
```

- `question` 是一等 Item：一个提问可能成为 durable open interrupt，之后由 `runs.resume` 应答。
- `toolCall.error`（+ `status:"incomplete"`）是**工具级失败的统一结构化落点**（不限 kind）。工具失败**通常不终止
  整个 run** —— agent 可据此换方案、继续（§8 通道 b）。

```ts
type ContentBlock =
  | { type: "text";  text: string }
  | { type: "image"; attachmentId: string };

interface PlanStep {
  id: string;
  title: string;
  status: "pending" | "inProgress" | "completed" | "failed";
}

interface Question {
  prompt: string;
  fields: QuestionField[];
}
interface QuestionFieldBase {
  name: string;                        // answers 按此 key 索引
  label: string;
  header?: string;                     // ≤12 字短标签（UI chip）
  required?: boolean;
}
type QuestionField =
  | (QuestionFieldBase & { type: "text" })
  | (QuestionFieldBase & { type: "choice"; options: QuestionOption[]; multiple?: boolean });
interface QuestionOption {
  label: string;
  description?: string;                // 选项释义
  preview?: string;                    // 侧边预览（方案对比）
}
```

### 4.4 ToolInvocation

> **通用 + 特殊（混合契约）**：工具调用按"变体集合是闭还是开"分两类——
> - **闭集 + 结构丰富 + 集中渲染**（命令执行、文件改动）→ **强类型变体**：`kind` 即身份，字段直接给（`exitCode` / `diff` 等），
>   客户端拿到编译期形状、专门渲染。
> - **开集 / 不可枚举**（MCP、动态、子 agent、搜索、第三方自定义）→ **一个通用信封** `{ kind:"tool", name, arguments, result }`：
>   `name` 是身份，入参是已解析 JSON 对象，输出是 best-effort JSON，客户端按 `name` 自取值 + 展开渲染。
>
> 关键：**没有任何工具同时带 kind 隐含字段 + 冗余 `name`**（命令用 `command` argv、通用用 `name`）——从根上消除 `kind`↔`name`
> 双重身份的漂移（上轮 W1 一类的对不齐）。

```ts
type ToolInvocation =
  // 特殊：闭集 + 结构丰富 → 强类型（kind 即身份，无 name）
  | { kind: "commandExecution"; command: string[]; cwd?: string; exitCode?: number; durationMs?: number }
  | { kind: "fileChange";       changes: FileChangeEntry[] }
  // 通用：开集 / 不可枚举 → 一个信封兜底
  | { kind: "tool";             name: string; arguments: Record<string, unknown>; result?: unknown };

interface FileChangeEntry {
  path: string;
  kind: "add" | "modify" | "delete" | "rename";
  diff?: DiffRow[];                      // §4.5
}
```

- **`commandExecution`**：`command` 是 **argv 数组**（无 shell 引号歧义）；stdout/stderr 走 `item.delta{ type:"toolOutput", text }`
  流式累积预览，`exitCode` / `durationMs` 在 `item.completed` 落定。无独立 `output` 字段。
- **`fileChange`**：多文件改动列表，每项带 `path` / `kind`（增改删移）/ `diff`。
- **`tool`（通用）**：
  - **入参 `arguments` 永远是 JSON 对象，绝不回传 JSON 字符串。** 流式部分入参走 `item.delta{ type:"toolArguments", argumentsTextDelta }`
    （JSON 文本增量，§5.1）累积；server 在 `item.completed`（及审批 payload，§4.8）处 `unmarshal` 成对象再发——消除"双重转义"。
  - **输出 `result` 是 best-effort JSON**：首选对象（可按下表取键），也允许任意 JSON 值（数组/标量/字符串——有些工具输出奇形怪状）。
    硬约束：**绝不双重编码**（`{x:1}` 不是 `"{\"x\":1}"`）；纯文本用 JSON string 值即可（`result:"ok"`），能结构化就尽量给对象。
  - **客户端按 JSON 展开渲染入/出**：`arguments` 必为对象 → JSON 树；`result` 是 JSON → 美化展开，非 JSON → 兜底原始文本。
- **工具级失败不进 `result` / 不改 wire 形状**：错误一律走 `toolCall.error`（ProblemData）+ `status:"incomplete"`（§4.3 / §8），三种 kind 通用。
- 子 agent 的子 Run 经 `子 RunRef.spawnedByItemId == 本 toolCall.id` 关联（通用 `tool` 也可在 `result.childRunId` 给便利值）。

**通用 `tool` 的 `arguments` / `result` 键按 `name` 约定**（共识约定表，非 wire 强制；客户端按 `name` 取值，未知 `name` 走 JSON 兜底）：

| name | arguments（取值键） | result（取值键） |
| --- | --- | --- |
| `read` | `path`, `range?` | `content` |
| `glob` | `pattern`, `cwd?` | `matches: string[]`, `total` |
| `grep` | `pattern`, `path?` | `matches: GrepMatch[]`（§4.5）, `total` |
| `web_search` | `query` | `results: SearchResult[]`（§4.5） |
| `<server>.<tool>`（MCP） | 工具自定义 | 工具自定义 |
| `subagent` | `prompt` / `task` | `summary`, `childRunId?` |

> 新增**通用**工具只需在此表登记一行、按 `name` 对齐，不动 wire；新增**强类型**工具（极少）才加一个 `ToolInvocation` 变体。
> 客户端的"按 name 取值 + 渲染"集中在一处工具展示注册表（可插件扩展）。

### 4.5 Diff / Search / 文件

```ts
type DiffRow =
  | { type: "hunk";    text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added";   rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

interface SearchResult { title?: string; url?: string; path?: string; snippet?: string }

interface FileChange { path: string; status: "added" | "modified" | "deleted" | "renamed" | "untracked" }
interface FileHead   { path: string; lines: FileLine[] }
interface FileLine   { lineNumber: number; text: string }
interface GrepResult { matches: GrepMatch[]; total: number }       // total ≥ matches.length（matches 可能被 limit 截断）
interface GrepMatch  { path: string; lineNumber: number; text: string }
```

> `FileLine.text` / `DiffRow.code` / `GrepMatch.text` 是**纯文本**（不含 server 端 HTML）；高亮由客户端做。

### 4.6 Usage / Error

```ts
interface Usage {
  inputTokens?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  byModel?: Record<string, {           // 子 agent 子树可跨多模型；per-model 拆分（不递归）
    inputTokens?: number; outputTokens?: number; reasoningTokens?: number; costUsd?: number;
  }>;
}

interface ProblemData {                // 用于 RPCError.data、RunResult.error、toolCall.error
  type: string;                        // 稳定符号名（判错用，§8.3）；插件用命名空间（§8.4）
  detail?: string;                     // 本次发生的人读说明（per-occurrence，非每类不变）
  retryable?: boolean;                 // 是否可重试
  retryAfterSeconds?: number;          // 可重试时的最早重试时机（如 provider 限流回传的退避）
  errors?: FieldError[];               // 字段级错误（invalid_params / 表单校验，按 field 寻址）
  [key: string]: unknown;              // 仍可附加 type-specific 扩展成员
}
interface FieldError { field: string; detail: string }   // field = 出错字段名（params 里的 key）
```

### 4.7 上下文 / 工具规格

```ts
type ContextItem =
  | { kind: "file";      path: string }                          // 相对 Session.cwd
  | { kind: "selection"; path: string; range: [number, number] } // 1-based 闭区间
  | { kind: "url";       url: string }                           // Runtime 主动 fetch
  | { kind: "image";     attachmentId: string };

interface ToolSpec {
  name: string;
  description?: string;
  parameters?: Record<string, unknown>;   // JSON Schema
  safetyClass?: string;                    // "safe" | "write" | "exec" | "network" …
}
interface GenerationParams { temperature?: number; maxTokens?: number; topP?: number; stop?: string[] }
```

**`ContextItem` 安全不变量（定死，不许蒸发）**：

- `file.path` / `selection.path` **相对 `Session.cwd`**；逃出 cwd（`../` 越界）必须拒绝并返 `path_outside_root`。
- `selection.range` = `[startLine, endLine]`，1-based 闭区间。
- `url`：Runtime 会主动 fetch，**有 SSRF 面** —— 必须拦 loopback / 私网 / 元数据地址（`169.254.*` 等），除非宿主策略显式放行。

### 4.8 HITL 类型

```ts
// 通用（itemId）+ 特殊（按 kind 的 payload）。approval / toolResult 复用 ToolInvocation（§4.4）——
// 客户端读 payload.tool 即可，不再猜 command 在哪 / 处理字符串转义。question 的内容在 question Item 上（item.question），
// 故不带冗余 payload。
type Interrupt =
  | { kind: "approval";   itemId: string; payload: ApprovalPayload }
  | { kind: "question";   itemId: string }                          // 内容见 question Item，无 payload
  | { kind: "toolResult"; itemId: string; payload: ToolResultPayload };

interface ApprovalPayload {
  tool: ToolInvocation;                // 待批工具（此时 result 尚无）
  risk?: "low" | "medium" | "high";    // 可选；后端暂无风险引擎时留空
  reason?: string;                     // 可选；为何需要批准
}
interface ToolResultPayload {
  tool: ToolInvocation;                // 要客户端执行的工具（client-side tools）；结果经 runs.resume 回传
}
interface OpenInterrupt {
  parentRunId: string;                 // 待 resume 的 Run（其 outcome.type=interrupt）
  sessionId: string;
  interrupts: Interrupt[];
  createdAt: string;
}
```

### 4.9 Provider / Model

```ts
interface Provider {
  id: string;
  type: string;                        // "openai" | "anthropic" | …
  baseUrl?: string;
  apiKeyMasked: string;                // "" = 未配置；如 "sk-…fc78"；永不可逆推（短 key 全打码）
}
interface Model {
  id: string;
  provider: string;                    // Provider.id
  displayName?: string;
  contextWindow?: number;
  maxOutputTokens?: number;
  capabilities?: { reasoning?: boolean; multimodal?: boolean; toolUse?: boolean };
  pricing?: { inputUsdPerMillionTokens?: number; outputUsdPerMillionTokens?: number };
}
```

### 4.10 Workspace 周边 / 可选域类型

```ts
interface Skill    { name: string; description?: string; source?: string }
interface AgentDoc { path: string; title?: string; scope: "cwd" | "projectRoot" | "home" }
interface McpServer{ name: string; status: "connected" | "disconnected" | "error"; description?: string }
interface McpTool  { server: string; name: string; description?: string; inputSchema?: Record<string, unknown> }

interface MemoryEntry { scope: "cwd" | "projectRoot" | "home"; path: string; content: string; updatedAt?: string }
interface Attachment  { id: string; name: string; mime: string; sizeBytes: number; createdAt: string }
interface BackgroundTask {
  id: string; kind: string; status: "running" | "completed" | "failed" | "canceled";
  createdAt: string; updatedAt?: string; result?: unknown; error?: ProblemData;
}
```

### 4.11 分页

```ts
interface Page<T> { data: T[]; nextCursor?: string }   // cursor 不透明
```

---

## 5. 流式

所有 run 事件走**一个**通知方法 `notifications.run.event`，params 为 `RunEvent`：

```ts
interface RunEvent {
  runId: string;
  eventId: string;        // evt_…；单 root run stream 内单调（§2.2）
  timestamp: string;      // ISO-8601
  durable: boolean;       // true=权威可 list；false=高频易失 delta
  event: StreamEvent;
}

type StreamEvent =
  | { type: "run.started";    run: RunRef }
  | { type: "run.finished";   outcome: RunOutcome }
  | { type: "item.started";   item: Item }                       // item 壳（status=inProgress）
  | { type: "item.delta";     itemId: string; delta: ItemDelta }
  | { type: "item.completed"; item: Item }                       // 权威终态，durable
  | { type: "state.snapshot"; state: Record<string, unknown> }  // 第三方顶层 key 遵 §2.5
  | { type: "state.delta";    patch: JsonPatch }
  | { type: "custom";         name: string; payload: unknown }; // 第三方 name 遵 §2.5

type JsonPatch = Array<{ op: "add"|"remove"|"replace"|"move"|"copy"|"test"; path: string; value?: unknown; from?: string }>;
```

事件 `durable` 取值：`run.*` / `item.started` / `item.completed` / `state.snapshot` = **true**；
`item.delta` / `state.delta` = **false**；`custom` 由产出方标注。

### 5.1 ItemDelta

```ts
type ItemDelta =
  | { type: "content";       index?: number; text: string }    // agentMessage 文本增量
  | { type: "reasoning";     text: string }                    // 推理文本增量
  | { type: "toolArguments"; argumentsTextDelta: string }      // 工具入参的部分 JSON 文本增量
  | { type: "toolOutput";    text: string }                    // 工具输出（如命令 stdout）增量
  | { type: "plan";          steps: PlanStep[] };              // 计划当前全量（非高频字符流）
```

> **`toolArguments` 是文本增量**：流式工具入参是逐片到达的不完整 JSON 文本，无法在中途构成合法对象；客户端累积
> `argumentsTextDelta`、用 `untruncate-json` 修复成可解析片段做预览；**已解析结构化 `arguments` 只在 completed
> `toolCall` item**（§4.4）。

### 5.2 Durable / Ephemeral 不变量（协议级保证）

> **丢弃每一个 `durable=false` 事件，客户端仍必然得到正确终态。**

推论：

- 每个 item 的最终态必然出现在后续 `item.completed`；
- 被 `state.delta` 影响的每个共享状态最终值必然出现在后续 `state.snapshot`；
- 客户端可 opt out 高频 delta 而仍保持正确（§9 `optOutNotificationMethods`）。

### 5.3 state.snapshot 必发边界

当 run 期间创建或改变了共享状态时，server **必须**在以下边界发 `state.snapshot`：

- `run.finished` 之前；
- 重连后，若客户端可能漏过 `state.delta`；
- 在尊重某个 `state.delta` opt-out 之后。

无共享状态时 `state.snapshot` 可省略。

### 5.4 Run 树

一条 root run 流包含**整棵 run 树**的事件（根 + 所有子孙 subagent run）：

- 每条事件带它所属的 `runId`；
- 子 run 以 `run.started` 携带 `run.spawnedByItemId` 开始；
- 客户端用 `runId` + `spawnedByItemId` join 还原树；
- 对 root run `runs.subscribe` = 订阅整棵树；
- `features.subagents=false` 时不产出任何子 run 事件。

---

## 6. Human-in-the-Loop（R 模型）

流程：

1. agent 需要人介入 → 产出对应 Item（`toolCall` 待批 / `question` 待答 / client 工具调用）。
2. Run **结束**：`run.finished{ outcome:{ type:"interrupt", interrupts:[…] } }`，资源全释放。每个 `Interrupt.itemId`
   指向那个待解 Item。
3. 待解项作 `OpenInterrupt` **持久化**（跨重启可经 `runs.listOpenInterrupts` 发现）。
4. 客户端调 `runs.resume{ parentRunId, responses:[…] }` 起一个**延续 Run**（新 runId，`parentRunId` 串接）应答。
5. 延续 Run 像普通 run 一样流式，直到下一个 outcome。

**关联键 = `itemId`**（无单独 requestId）。**拒绝 ≠ 取消**：拒绝是 `runs.resume` 带 `decision:"deny"`，run 继续
（agent 据理由换方案）；取消是 `runs.cancel`，硬终止整个 run。

### 6.1 InterruptResponse

```ts
interface InterruptResponse {
  itemId: string;                      // 对应 Interrupt.itemId
  response: ApprovalResponse | AnswerResponse | ToolResultResponse;
}
interface ApprovalResponse  { kind: "approval"; decision: "approve" | "deny"; editedArgs?: Record<string, unknown>; reason?: string }
interface AnswerResponse    { kind: "answer"; answers: Record<string, string | string[]> }   // key = QuestionField.name
interface ToolResultResponse{ kind: "toolResult"; result?: unknown; error?: ProblemData }      // client 工具结果，与 ToolInvocation.result 同形（best-effort JSON）；失败给 error
```

### 6.2 防挂死（协议级硬约束）

client 在 `ClientCapabilities.interruptKinds` 声明能处理的 `Interrupt.kind`。server **必须不产出 client 未声明 kind
的 open interrupt** —— 否则会留下一个**永远 `runs.resume` 不了的持久 open interrupt**（比挂起 run 更糟）。不支持时
server 走非阻塞默认策略（auto-deny / 不进该模式）。`toolResult` interrupt 额外受 `features.clientTools` 门控。

---

## 7. 方法参考

> 约定：**Stream<R>** = 返回 `R`（含 `runId`）+ 随后流式 `notifications.run.event` 的 `RunEvent`。这是
> 传输无关语义；HTTP 上 `R` 作首帧、与后续 `RunEvent` 同走该 POST 的 `text/event-stream` 响应（streamable
> HTTP，见 TRANSPORT §6.4），InProcess/IPC 则返回值 + 通知回调。
> 每个方法的「错误」列的是**可能返回的 `ProblemData.type`**（按 name 判，§8）；通用错误（`invalid_params` /
> `internal_error` / `capability_not_negotiated`）不逐一重列。

### 7.1 runtime.*

#### `runtime.initialize`
握手。握手前 runtime 拒绝业务方法。

- 入参 `InitializeRequest`：

  | 字段 | 类型 | 必填 | 说明 |
  | --- | --- | --- | --- |
  | `protocolVersion` | string | 是 | client 想用的版本（日期串） |
  | `clientInfo` | `{ name: string; version: string }` | 是 | |
  | `capabilities` | `ClientCapabilities`（§9） | 是 | client 能渲染的事件 / 能处理的 interruptKinds / opt-out |

- 返回 `InitializeResponse`：

  | 字段 | 类型 | 说明 |
  | --- | --- | --- |
  | `protocolVersion` | string | server 实际采用版本 |
  | `serverInfo` | `{ name; version; cwd; home }` | serve 进程目录上下文（冷启动默认 cwd 兜底） |
  | `capabilities` | `ServerCapabilities`（§9） | |

- 错误：`invalid_protocol_version`（版本无法协商，硬断开）。

#### `runtime.shutdown`（通知，无响应）
- 入参 `{ reason?: string }`。runtime 停接新工作、按宿主策略结束/取消进行中的 run、关闭 transport。

#### `runtime.ping`
- 入参：无；返回：无。**仅 InProcess / IPC**；HTTP 探活走 sidecar `GET /v2/health`（见 TRANSPORT）。

#### `notifications.canceled`（client→server 通知）
- 入参 `{ id: string; reason?: string }` —— `id` 是被取消的在飞 Request 的 envelope id。取消一个慢请求，不取消 run。

---

### 7.2 sessions.*

#### `sessions.list`
- 入参 `{ cursor?: string; limit?: number }`；返回 `Page<Session>`。

#### `sessions.get`
- 入参 `{ sessionId: string }`；返回 `Session`；错误 `session_not_found`。

#### `sessions.create`
- 入参 `CreateSessionRequest`：

  | 字段 | 类型 | 必填 | 说明 |
  | --- | --- | --- | --- |
  | `cwd` | string | 否 | 缺省 = `ServerInfo.cwd`（冷启动零摩擦，不弹强制选择框） |
  | `title` | string | 否 | |
  | `model` | string | 否 | 默认模型 |
  | `metadata` | object | 否 | |

- 返回 `Session`（`cwd` 为 server 规范化后的绝对路径）。错误 `cwd_unavailable`。

#### `sessions.update`
- 入参 `UpdateSessionRequest`：

  | 字段 | 类型 | 必填 | 说明 |
  | --- | --- | --- | --- |
  | `sessionId` | string | 是 | |
  | `title` | string | 否 | |
  | `cwd` | string | 否 | **改 cwd = relocate**；受 `features.relocate` 门控 |
  | `model` | string | 否 | |
  | `metadata` | object | 否 | 全替换 |

- 返回 `Session`。错误 `session_not_found` / `capability_not_negotiated`（relocate 关闭时改 cwd）/ `cwd_unavailable`。

#### `sessions.delete`
- 入参 `{ sessionId: string }`；返回无。

#### `sessions.fork`
- 入参 `{ sessionId: string; fromItemId?: string; title?: string }`（在某 item 边界分叉，继承源 cwd）；返回新 `Session`。

#### `sessions.export`
- 入参 `{ sessionId: string; format?: "md" | "json" }`；返回 `{ url: string; expiresAt: string }`。
- **导出走 transport 文件通道**（不内联会话内容，避免长会话巨型 payload）：HTTP 上 `url` 是带本地门禁的短期下载路径，
  InProcess/IPC 上是 `file://` 或 native binding 句柄。受 `features.sessionExport` 门控。

---

### 7.3 runs.*

#### `runs.start` — Stream
起一个新 Run 并打开事件流。

- 入参 `StartRunRequest`：

  | 字段 | 类型 | 必填 | 说明 |
  | --- | --- | --- | --- |
  | `sessionId` | string | 是 | cwd 从 session 解析；本请求**不带 cwd**、**不带 runId** |
  | `input` | `ContentBlock[]` | 是 | user 消息正文 |
  | `context` | `ContextItem[]` | 否 | 附带上下文；`file.path` 相对 session.cwd（§4.7 安全规则） |
  | `tools` | `ToolSpec[]` | 否 | 覆盖该 run 的默认工具集 |
  | `state` | object | 否 | 初始共享状态 |
  | `attachments` | `string[]` | 否 | attachmentId（来自 `attachments.createUploadUrl`） |
  | `provider` | string | 否¹ | 选用的 provider（`Provider.id`，来自 `providers.list`）。与 `model` **配对** |
  | `model` | string | 否¹ | 选用的 model（`Model.id`，来自 `models.list`）。与 `provider` **配对** |
  | `mode` | `"agent"\|"chat"\|"plan"` | 否 | |
  | `maxSteps` | number | 否 | 触顶 → `outcome.maxSteps` |
  | `maxBudgetUsd` | number | 否 | 含子 agent 子树；触顶 → `outcome.maxBudget` |
  | `params` | `GenerationParams` | 否 | |

  ¹ **`provider` + `model` 配对、显式**：要么**都给**（选定 provider+model），要么**都不给**（用服务端默认 provider+model）。**只给其一 → `invalid_params`**。provider **不从 model 反推**（显式 > 隐式：同名 model 可能跨多个 provider；自定义 model 可能不在 catalog 里）。所选 provider 未配置 key（`apiKeyMasked == ""`）时，run 以 `run.finished{outcome:error}`（"set its API key first"）收尾。两个字段都是有意义的 slug（无 `Id` 后缀，与 `model`、`Model.provider` 一致），直接取自 `models.list` 的 `Model.{provider, id}`。

- 返回 `{ runId: string; userItemId?: string }`；随后流式 `RunEvent`（§5），以 `run.finished{outcome}` 收尾。（HTTP 上响应
  作首帧、与事件同走一条流，见 TRANSPORT §6.4。）
  - `userItemId`：本 run 开场的 `userMessage` Item 的 id —— **与流上 `item.started/completed` 及 `items.list` 里的同一个 id**。客户端用它把**乐观气泡按精确 id 对账**（不必再按内容文本启发式匹配）。是 result 里的业务字段、非 transport 元数据。`runs.resume` 无开场 user 回合，故为空。
- 幂等：经 `X-Idempotency-Key` 头（见 TRANSPORT §10），重复键重新接上既有 run、不新建。
- 错误（通道 a，§8.1）：`session_not_found` / `cwd_unavailable` / `invalid_params`（含 `provider`/`model` 只给其一）。
  执行期错误（通道 b）：`run.finished{outcome:error}` / 工具级 `toolCall.error`。

- **示例**：

  请求
  ```json
  { "jsonrpc": "2.0", "id": "1", "method": "runs.start",
    "params": { "sessionId": "ses_01", "input": [{ "type": "text", "text": "列出当前目录" }], "mode": "agent" } }
  ```
  响应（HTTP 上是流的首帧）
  ```json
  { "jsonrpc": "2.0", "id": "1", "result": { "runId": "run_01" } }
  ```
  随后事件（节选）
  ```json
  { "jsonrpc":"2.0", "method":"notifications.run.event",
    "params": { "runId":"run_01", "eventId":"evt_0001", "timestamp":"2026-06-03T10:00:00Z", "durable":true,
                "event": { "type":"run.started", "run": { "id":"run_01", "sessionId":"ses_01" } } } }
  { "jsonrpc":"2.0", "method":"notifications.run.event",
    "params": { "runId":"run_01", "eventId":"evt_0008", "timestamp":"2026-06-03T10:00:02Z", "durable":true,
                "event": { "type":"run.finished",
                           "outcome": { "type":"completed", "result": { "usage": { "inputTokens": 1200, "outputTokens": 80 }, "steps": 2 } } } } }
  ```

#### `runs.resume` — Stream
应答 open interrupt，起一个延续 Run。

- 入参 `ResumeRunRequest`：

  | 字段 | 类型 | 必填 | 说明 |
  | --- | --- | --- | --- |
  | `parentRunId` | string | 是 | 被中断的 Run（其 outcome.type=interrupt） |
  | `responses` | `InterruptResponse[]`（§6.1） | 是 | 对每个 open interrupt 的应答（按 itemId 寻址） |

- 返回 `{ runId: string }`（新 runId，其 `RunRef.parentRunId` = `parentRunId`）+ 流式 `RunEvent`。
- 错误：`run_not_found` / `interrupt_not_open`（interrupt 缺失或已 resolve）/ `invalid_params`。

- **示例**（批准一个 toolCall interrupt）：
  ```json
  { "jsonrpc":"2.0", "id":"7", "method":"runs.resume",
    "params": { "parentRunId":"run_01",
                "responses": [ { "itemId":"item_09", "response": { "kind":"approval", "decision":"approve" } } ] } }
  ```

#### `runs.subscribe` — Stream
为一个已存在的 run（root）打开一条新的事件流（重连/崩溃恢复）。

- 入参 `{ runId: string }`；返回 `{ runId }` + 流式 `RunEvent`（订阅整棵 run 树，§5.4）。带 `Last-Event-Id` 从断点续传 durable 事件（HTTP 见 TRANSPORT §9.2）。
- 错误：`run_not_found`。

#### `runs.cancel`
硬终止一个在跑的 run。

- 入参 `{ runId: string; reason?: string }`；返回无。run 以 `outcome:canceled` 收尾。错误：`run_not_found` / `run_not_running`。

#### `runs.list`
- 入参 `{ sessionId?: string }`；返回 `RunRef[]` —— **仅在跑的 run**。已结束/被中断的 run 经 `listOpenInterrupts` 或
  item 历史发现。

#### `runs.listOpenInterrupts`
- 入参 `{ sessionId?: string }`；返回 `OpenInterrupt[]`（§4.8）。durable HITL 发现：重启后据此 `runs.resume`。

---

### 7.4 items.*

#### `items.list`
会话历史 = completed Item 序列 + 还原 run 树所需的 RunRef。

- 入参 `{ sessionId: string; cursor?: string; limit?: number }`。
- 返回 `ListItemsResponse`：

  | 字段 | 类型 | 说明 |
  | --- | --- | --- |
  | `items` | `Item[]` | 平铺（避免 `resp.items.data`） |
  | `nextCursor` | string? | 分页游标 |
  | `runs` | `RunRef[]` | 这些 item 所属 Run（含已完成/在跑），带 `parentRunId`/`spawnedByItemId`，供 join 还原 Run 树（§10.3） |

- 错误：`session_not_found`。

#### `items.edit`
编辑某 item → 起一个延续 Run（语义同 resume）。

- 入参 `{ itemId: string; replacement: ContentBlock[] }`。
- 返回 `{ runId: string; parentRunId: string }`（新延续 Run）。受 `features.checkpoints` 门控。
- 错误：`item_not_found` / `checkpoint_unavailable`。

---

### 7.5 workspace.*

所有读方法显式收 `cwd?`（缺省 = serve 目录）。入参基类 `WorkspaceQuery { cwd?: string }`。

| 方法 | 入参 | 返回 | 说明 |
| --- | --- | --- | --- |
| `workspace.listFileChanges` | `WorkspaceQuery` | `FileChange[]` | 工作区改动 |
| `workspace.getDiff` | `WorkspaceQuery & { path?: string }` | `DiffRow[]` | 结构化 diff |
| `workspace.getFileHead` | `WorkspaceQuery & { path: string; lines?: number }` | `FileHead` | 文件头若干行 |
| `workspace.grep` | `WorkspaceQuery & { query: string; path?: string; limit?: number }` | `GrepResult` | `{ matches, total }` |
| `workspace.listProjects` | 无 | `Project[]` | distinct-cwd 派生视图 |
| `workspace.listSkills` | `WorkspaceQuery` | `Skill[]` | |
| `workspace.listAgentDocs` | `WorkspaceQuery` | `AgentDoc[]` | 从 cwd 向上发现的 AGENTS.md |
| `workspace.mcp.listServers` | 无 | `McpServer[]` | MCP 全局，不收 cwd |
| `workspace.mcp.listTools` | `{ server?: string }` | `McpTool[]` | |
| `workspace.mcp.reconnect` | `{ server: string }` | 无 | |

错误：读方法可返 `cwd_unavailable` / `path_outside_root`。

---

### 7.6 providers.* / models.* / tools.*

> **多 provider × 多 model**。装配流程：`providers.list`（看支持哪些）→ 用户填 key（`providers.configure`）→ `providers.test`（验）→ `models.list`（解锁该 provider 的 model）→ `runs.start{ provider, model }`（选用，§7.3）。
>
> 命名：引用 provider / model 的参数一律用裸名 **`provider`** / **`model`**（有意义的 slug，无 `Id` 后缀），与 `Model.{provider, id}` 字段一致。对象自身的身份字段仍是 `Provider.id` / `Model.id`。

#### `providers.list`
- 入参无；返回 `Provider[]`（§4.9）—— **后端支持的全部 provider**（有 adapter 的那些），无论是否已配置。每条按注册表标注：`apiKeyMasked == ""` 即**未启用**（启用 ⇔ 已配 key），并带已配的 `baseUrl`。

#### `providers.configure`
- 入参 `{ provider: string; type?: string; baseUrl?: string; apiKey?: string }`；upsert 进**运行态注册表**（持久化），返回 masked `Provider`。`provider` 必须是支持的 provider，否则 `invalid_params`。`baseUrl` 可覆盖默认端点（代理 / 网关 / 自建 OpenAI 兼容端点）。

#### `providers.test`
- 入参 `{ provider: string }`；返回 `{ ok: boolean; error?: ProblemData }`。**真实探活**：用该 provider 默认 model 发一次极小（`max_tokens=1`）请求验证 key + 端点。失败回 `{ ok:false, error }`（**不**报 RPC 错），UI 可内联显示原因（如 401）。

#### `models.list`
- 入参 `{ provider?: string }`；返回该 provider 的 `Model[]`（§4.9，带 `displayName` / `contextWindow` / `maxOutputTokens` / `capabilities` / `pricing`）。直读后端内置 model catalog —— **不需要 key、不受启用门控**（这解决了"没填 key 就拿不到 model 列表"的死结）。`provider` 省略时返回空数组（model 按 provider 组织）。

#### `tools.list`
- 入参无；返回 `ToolSpec[]`。

#### `tools.invoke`
不经 LLM 直接调一个工具（诊断 / client 驱动）。

- 入参 `{ name: string; arguments: Record<string, unknown>; cwd?: string }`；返回 `unknown`（工具原始输出）。
- 错误：`tool_denied` / `path_outside_root`。

---

### 7.7 可选域（capability-gated）

关闭时返 `capability_not_negotiated`。门控位见 §9。

| 方法 | 入参 | 返回 | 门控 |
| --- | --- | --- | --- |
| `memory.list` | `WorkspaceQuery` | `MemoryEntry[]` | `memory` |
| `memory.get` | `{ scope: "cwd"\|"projectRoot"\|"home"; cwd?: string }` | `MemoryEntry` | `memory` |
| `memory.update` | `{ scope; cwd?; content: string }` | 无 | `memory` |
| `attachments.createUploadUrl` | `{ name: string; mime: string; sizeBytes: number }` | `{ attachmentId; uploadUrl; expiresAt }` | `attachments` |
| `attachments.get` | `{ attachmentId: string }` | `Attachment` | `attachments` |
| `attachments.delete` | `{ attachmentId: string }` | 无 | `attachments` |
| `background.list` | 无 | `BackgroundTask[]` | `background` |
| `background.subscribe` | `{ taskId: string }` | Stream（经 `notifications.background.update`，params `BackgroundTask`） | `background` |
| `background.cancel` | `{ taskId: string }` | 无 | `background` |
| `feedback.create` | `{ sessionId?; runId?; itemId?; rating?: "positive"\|"negative"; text? }` | 无 | —— |

错误：`attachment_too_large` / `unsupported_mime`（attachments）。

---

### 7.8 服务端发出的 Notification 汇总

| Notification | params | 何时 |
| --- | --- | --- |
| `notifications.run.event` | `RunEvent` | run 期间每个事件（run/item/state） |
| `notifications.background.update` | `BackgroundTask` | background 任务状态变化（gated） |

> `notifications.canceled` 是 **client→server**（§7.1），不在此表。

---

## 8. 错误

### 8.1 两条投递通道（流式场景必读）

`runs.start` / `runs.resume` **立即返成功**（`{ runId }`，流已开）。run **执行期**失败走另一条路：

| 通道 | 何时 | 怎么投递 |
| --- | --- | --- |
| (a) **method 调用失败** | 调用本身就错：`session_not_found` / `invalid_params` / `cwd_unavailable` / `capability_not_negotiated` / 未握手 | 该 method 的 **JSON-RPC error response**（同步） |
| (b) **run 执行期失败** | run 已起、执行中出错 | **`run.finished{ outcome:{type:"error", result.error} }`**；**工具级**失败落在对应 `toolCall` item 的 `error` + `status:"incomplete"`，run 多半继续 |

实现方**不要**期望 run 执行期错误能在 `runs.start` 的 response 里拿到。

### 8.2 错误码表

业务错误用 JSON-RPC `error.code`，**不映射 HTTP status**（HTTP status 仅反映传输层，见 TRANSPORT §6.3）。

> **码值是 v2 全新分配、不保证与任何旧基线一致。client / server 一律按 `error.data.type`（符号名）判错，
> 不按数字码。** 数字码仅作粗分类。

| Code | type（name） | 含义 |
| --- | --- | --- |
| `-32600` | `invalid_request` | JSON-RPC envelope 非法 |
| `-32601` | `method_not_found` | 未知方法 |
| `-32602` | `invalid_params` | params 校验失败 |
| `-32603` | `internal_error` | 运行时意外失败 |
| `-32001` | `provider_error` | provider 请求失败（含限流 / 超时） |
| `-32002` | `session_not_found` | session 不存在 |
| `-32003` | `run_not_found` | run 不存在 |
| `-32004` | `item_not_found` | item 不存在 |
| `-32005` | `cwd_unavailable` | 工作目录缺失或不可读 |
| `-32006` | `capability_not_negotiated` | 方法或能力被关闭 |
| `-32007` | `run_not_running` | 操作需要一个在跑的 run |
| `-32008` | `run_already_finished` | 操作不能作用于已结束 run |
| `-32009` | `checkpoint_unavailable` | 编辑 / checkpoint 不可用 |
| `-32010` | `attachment_too_large` | 附件超限 |
| `-32011` | `unsupported_mime` | 附件 MIME 不支持 |
| `-32012` | `tool_denied` | 工具执行被策略拒绝 |
| `-32013` | `path_outside_root` | 路径逃出 cwd 根 |
| `-32014` | `interrupt_not_open` | interrupt 缺失或已 resolve |
| `-32015` | `idempotency_conflict` | 幂等键与不同 params 冲突 |
| `-32016` | `invalid_protocol_version` | 版本协商失败（`initialize` 硬断开） |

`error.data` 为 `ProblemData`，**必须含 `type`（= 上表 name）**。

> 业务码 `-32001..-32016` 落在 JSON-RPC 2.0 的 `-32000..-32099`「implementation-defined server-error」保留段；
> `-326xx` / `-32700` 为 spec 预定义码。数字码合规即可，**判别一律走 `type`**。

### 8.3 错误细节（ProblemData，对标 RFC 9457）

`error.data` 是 `ProblemData`（§4.6）—— **RFC 9457 *Problem Details* 数据模型的传输无关裁剪**：去掉 HTTP 专属的
`status`（与 JSON-RPC `code` 冗余）和 `instance`；`type` 是稳定符号名、**不要求是可解析 URI**（命名空间见 §8.4）。
单个 `type` 即机器判别键，不引入第二套分类。两个**约定好的扩展成员**（替代各产出方自创 shape）：

- **`retryAfterSeconds`** —— 可重试错误（典型 `provider_error` 限流）回传的最早重试时机；client 退避以此为准，缺省回落自有退避。
- **`errors: FieldError[]`** —— 字段级校验错误（典型 `invalid_params`、provider 配置 / `question` 答案表单）；
  `field` = 出错的 params key，UI 可逐字段标红。

### 8.4 `type` 命名空间（防撞名）

error `type` 是 §2.5 命名空间的一个实例：first-party（§8.2 上表）用裸 `snake_case`；**第三方插件**产出的错误
（工具执行失败落在 `toolCall.error`，§4.3）用 `plugin:<pluginName>/<symbol>`（如 `plugin:acme/quota_exceeded`）。

---

## 9. Capabilities 与协商

```ts
interface ServerCapabilities {
  protocolVersion: string;
  events: string[];                    // 发出的事件 type（run.* / item.* / state.* / custom 名；第三方 custom 名遵 §2.5）
  features: {                          // 未声明默认 false
    reasoning: boolean; mcp: boolean; multimodal: boolean; checkpoints: boolean;
    background: boolean; subagents: boolean; skills: boolean; sessionExport: boolean;
    memory: boolean; relocate: boolean; clientTools: boolean;
    attachments: { enabled: boolean; maxSizeBytes?: number; mimeTypes?: string[] };
  };
  providers: string[];
  limits: { maxConcurrentRuns?: number; maxItemsPerSession?: number };
}

interface ClientCapabilities {
  events: string[];                    // 渲染得了的事件 type
  features: Record<string, unknown>;
  interruptKinds?: ("approval"|"question"|"toolResult")[];   // 能处理的 HITL 类型（防挂死，§6.2）
  optOutNotificationMethods?: string[]; // 握手时声明抑制某些高频通知，如 ["item.delta"]
}
```

规则：

- 缺省 feature flag 默认 `false`。
- server 不得发协商集合外的事件类型（已知 payload 上的未知未来字段除外）；client 必须忽略未知字段。
- server **必须不**产出 client 未在 `interruptKinds` 声明的 open interrupt（§6.2）。
- `features.subagents=false` 时不产出子 Run；`features.clientTools=false` 时不产出 `toolResult` interrupt。

---

## 10. 历史 / 重连 / 恢复

### 10.1 正常断线重连

1. 对每个仍显示的 root run 调 `runs.subscribe { runId }` 续流（每个 run 一条流，传输细节见 TRANSPORT §9.2）；
2. 支持时带 last-seen event id（`Last-Event-Id`）让 server 重放 durable 缺口；
3. 调 `items.list` 补 durable 缺口；
4. 按 `itemId` 与 `eventId` 去重。**ephemeral delta 不重放**（§5.2 保证正确）。

### 10.2 进程/客户端重启恢复

1. `sessions.list` / `sessions.get`；
2. `items.list` 拉 durable 历史；
3. `runs.list` 拿在跑的 run；
4. `runs.listOpenInterrupts` 拿可恢复的被中断 run；
5. `runs.resume` 应答 open interrupt。

### 10.3 还原 Run 树（一个会话跨多个 Run）

`items.list` 同时返回 `runs: RunRef[]`。客户端按 `runId` 把 item 归到 Run，再用：

- `RunRef.parentRunId` 串**延续链**（resume / edit）—— 显式，不靠 `createdAt` 猜；
- `RunRef.spawnedByItemId` 把子 Run 嵌到父 toolCall Item 下（**子树，`features.subagents` 门控**）。

三个 run 视图职责不重叠：`runs.list`（在跑）/ `listOpenInterrupts`（待解）/ `items.list.runs`（历史结构）。

---

## 11. 版本规则

- `protocolVersion` 是日期串（本定稿 `2026-06-03`）。
- **前向兼容是硬约定**：client 必须忽略未知字段、对未知 method/事件容忍。加 method / 加可选字段 / 加事件 →
  **同版本号**。
- 改语义 / 删字段 / 改字段类型 → 新日期版本 + 协商。
- `initialize` 是唯一因版本不匹配硬断开的方法。
- dev 阶段无 legacy 兼容：shape 变了就 bump version、丢旧 store、不写 migration。
- HTTP URL 里的 `/v2/`（wire major epoch）与日期 `protocolVersion`（epoch 内协商版本）是两个层级，不重复
  （见 TRANSPORT §6.1）。

---

## 12. v2 明确不做

- 经 `runs.send` 的 mid-run steering（留 v2.x，additive）。
- server→client JSON-RPC request。
- 远程多用户鉴权（协议层零 user 概念；鉴权由更外层 / 未来 facade 解决）。
- JSON-RPC batch。
- stdio transport。
- 客户端自选的业务资源 id。
- 多根 workspace（`Session.cwd` 单根；多根是未来破坏性改动）。

---

## 13. 安全不变量汇总

- **路径 containment**：`ContextItem` / fs 工具路径相对 `cwd`，越界 → `path_outside_root`（§4.7）。
- **URL fetch SSRF**：拦 loopback / 私网 / 元数据地址，除非宿主放行（§4.7）。
- **provider secret**：只回 `apiKeyMasked`，永不可逆推（§4.9）。
- **协议层零鉴权**：无 user / account 概念；本地进程门禁由 transport 层处理（TRANSPORT §11）。
- **cwd 是会话身份不是传输上下文**：走 body 不走带外 directory header（TRANSPORT §2）。
- **防挂死**：server 不产出 client 解不了的 open interrupt（§6.2）。

---

> 正式契约。配套 [`TRANSPORT.md`](./TRANSPORT.md)。后端 `lyra/rpc/protocol` Go interface 为机械 SSOT，经 codegen
> 派生 TS / schema；CI 卡 drift。
