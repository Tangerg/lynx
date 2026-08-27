# ScopeApp 内容区渲染规格

> **这份文档是自包含的。** 读它不需要打开仓库、不需要事先了解 ScopeApp。
> 它回答三个问题：**后端会给什么数据**（TypeScript 写全）、**每个字段大致要表达什么**、**大概落在界面的哪个区域**。
>
> 面向两类人：给内容区**重做视觉**的设计/前端；以及要给 ScopeApp Runtime 写另一个客户端的人。

> ### ⚠️ 怎么用这份文档
>
> 内容分三层，**只有两层是硬的**：
>
> | 层 | 硬度 | 内容 |
> | --- | --- | --- |
> | **① 数据契约** | **硬** | 有哪些字段、什么类型、什么时候缺席、哪个是权威的。§2 / §3 / §7 / §9 全属此层，可直接当类型定义用 |
> | **② 正确性约束** | **硬** | 违反了会出错或误导人的规则（加密的思考不能渲染、用户拒绝不能画成失败、`undefined` 不等于 `0`、路径要保住文件名…）。全文用**加粗**标出 |
> | **③ 表达意图 + 现状速写** | **软 —— 参考，不是要求** | 「这个字段大概想让读者知道什么」「大概在哪块区域」。ASCII 线框、分几档卡形、chip 顺序、折叠策略，**都是当前实现的一次答案，不是唯一答案**。重做视觉时可以整体推翻 |
>
> 换句话说：**数据怎么来、什么时候没有、不能怎么画** —— 照着；**长什么样** —— 你说了算。

| 想找 | 去 |
| --- | --- |
| 屏幕分区叫什么名字 | [§1 屏幕地图](#1--屏幕地图位置词汇表) |
| 后端发什么（协议原始类型） | [§2 协议类型](#2--协议类型全量-ts) |
| 组件真正吃什么（视图模型） | [§3 视图模型](#3--视图模型组件真正吃的东西) |
| 某个卡片怎么渲染 | [§4 内容区渲染物](#4--内容区渲染物d-区) |
| 顶部那几条横幅 | [§5 常驻条](#5--常驻条c-区) |
| 某个工具的参数和卡片形态 | [§7 三十个工具](#7--三十个内置工具) |
| 数字/时间/截断怎么写 | [§8 呈现约定](#8--呈现约定打磨基线) |
| 设置面板、文件树、诊断这些面的字段 | [§9 全量参数字典](#9--全量参数字典其余-surface) |

<details>
<summary><b>真值源</b>（本文与它们冲突时以它们为准）</summary>

| 事实 | 文件 |
| --- | --- |
| wire 形状 / 枚举 | `frontend/src/rpc/wire.generated.ts`（codegen，含 `PROTOCOL_VERSION`） |
| state key / 事件可靠性 / feature 门控 / 工具 result 归一化登记 | `app/runtime/contract/manifest.json` |
| 字段级 presence 规则（跨字段约束） | `app/runtime/contract/schema.json` |
| 工具身份 + safetyClass + 进行中文案 | `app/runtime/internal/adapter/toolset/catalog/` |
| 被归一化的工具 result | `app/runtime/internal/adapter/toolset/presentation.go` |
| 未归一化工具的入参/出参 | `tools/**` 与 `app/runtime/internal/adapter/toolset/<tool>/` |
| 协议为什么长这样 | `app/desktop/docs/protocol/API.md` |

本文档**只写渲染**：不写视觉 token（见 `DESIGN.md`）、不写插件装配（见 `ARCHITECTURE.md`）。

</details>

---

## 0 · 三分钟背景

### 0.1 三个概念

```
Session ──┬── Run ──┬── Item   (userMessage / agentMessage / reasoning / question / toolCall / compaction)
（一次会话）│（一次任务）├── Item
          │          └── Run   （子 agent：delegate_task 生出的 child run）
          └── Run …
```

- **Session** = 一条会话。有标题、绑定一个工作目录、有默认模型。
- **Run** = 用户发一句话到 agent 停下来的整个过程。一个 Run 可以分成多个 **Segment**（段）—— 每次需要人介入（审批、提问），当前段就结束、资源释放、Run 转入 `waiting`，人回答后在**同一个 Run** 上开新的一段继续。
- **Item** = 会话历史里的一条不可变事实。六种。**内容区渲染的主体就是 Item。**

### 0.2 一次对话的事件时序

```
用户发消息
   │
   ├─ runs.start ─────────────────────────► { runId, segmentId, userItemId }
   │
   │  ◄── segment.started      { run: RunRef }
   │  ◄── item.started         { item: userMessage }     ← 壳
   │  ◄── item.completed       { item: userMessage }
   │  ◄── segment.progress     { step, activity, usage, contextTokens }   ← 瞬时，可丢
   │  ◄── item.started         { item: reasoning }
   │  ◄── item.delta           { reasoning: "..." } × N  ← 预览，可丢
   │  ◄── item.completed       { item: reasoning }
   │  ◄── item.started         { item: toolCall }
   │  ◄── item.delta           { toolArguments: "..." } × N
   │  ◄── item.delta           { toolOutput: "..." } × N
   │  ◄── item.completed       { item: toolCall }        ← 权威：arguments + result 在这里落定
   │  ◄── state.snapshot       { plan: [...] }           ← 整份共享状态
   │  ◄── item.started/delta/completed { agentMessage }
   │  ◄── segment.finished     { outcome, metrics }
   ▼
```

**需要人介入时**，`segment.finished` 的 outcome 是 `{ type: "interrupt", interrupts: [...] }`，Run 不结束；客户端调 `runs.resume` 带上答复，开新的一段。

### 0.3 四层数据通路

一个字段从后端走到像素要过四层，**打磨时要知道自己站在哪一层**：

```
① wire            RunEvent / Item / ToolInvocation           ← §2，后端发的原始形状
      ↓  fold（纯映射 + 有状态折叠）
② 视图模型         ContentBlock / ToolCall / Message         ← §3，组件真正吃的东西
      ↓  render unit 规划（把 block 编排成 block / toolGroup / wave）
③ 渲染单元         MessageRenderUnit                         ← §4.7
      ↓  组件分发
④ 像素            ToolCard / ReasoningBlock / ApprovalCard…  ← §4
```

### 0.4 三条铁律

1. **`item.delta` 是预览，`item.completed` 是权威。** 流式攒的值在 completed 到达时必须被**覆盖**（不是累加）；而 completed 上**缺席**的键要**省略**（不是写 `undefined`），否则会把攒到的预览清空。
2. **丢掉每一个非权威事件，UI 仍必须正确。** `segment.progress` / `item.delta` / `custom` 可丢、不重放、重连后可能永远见不到。任何"只有流式才出现"的视觉，在**历史加载**路径（只有 completed、零 delta）里必须优雅缺席。
3. **fold 层不产出人类语言。** 所有文案是 i18n key，locale 在组件层解析。fold 里出现一句英文 = 缺陷。

---

## 1 · 屏幕地图（位置词汇表）

> **这是现状分区，目的只是给后文的「位置」一个能指代的名字。** 新设计完全可以重排、合并、删掉其中任何一块 —— 那时把代号重新绑到新分区上即可。
> 真正有约束力的只有一条：**C 区（常驻）与 D 区（滚动）必须分离** —— 一个随消息滚走的 Plan / Goal / 错误提示，等于没有。

### 1.1 全局分区（现状）

```
┌──────────────┬─────────────────────────────────────────────────┬────────────────┐
│              │  B  顶栏：会话标题 · 状态点 · 模型 · 用量 chip   │                │
│              ├─────────────────────────────────────────────────┤                │
│  A  侧栏     │  ┌── C  常驻条区（阅读列宽，钉住不随 D 滚动）──┐ │  G  Context    │
│              │  │  C1 工作区失联   C2 Run 错误                │ │     Dock       │
│  Work Index  │  │  C3 Plan 进度    C4 Goal 预算               │ │                │
│  · 会话列表  │  └─────────────────────────────────────────────┘ │  工具 inspector│
│  · 收藏      │  ┌─E─┬──── D  阅读列（滚动区，居中定宽）──────┐ │  diff / 文件树 │
│  · 工作区    │  │转 │  D1 日期分隔                            │ │  timeline      │
│              │  │场 │  D2 turn caption（角色 · 时钟）         │ │  run summary   │
│              │  │轨 │  D3 内容块  ← 本文主体                  │ │  …             │
│              │  │   │  D4 turn 操作条（复制 / 编辑 / 反馈）   │ │                │
│              │  │   │  D5 Run 收尾行                          │ │                │
│              │  └───┴─────────────────────────────────────────┘ │                │
│              │  ┌──── F  Composer（浮层，压在 D 之上）───────┐ │                │
│              │  └─────────────────────────────────────────────┘ │                │
└──────────────┴─────────────────────────────────────────────────┴────────────────┘
                 H  设置面板（独立路由，覆盖 B/C/D/E/F）
```

- **D 阅读列**居中定宽，左右各留一条同宽 gutter。**E 转场轨绝对定位在中线上、不参与流** —— 轨可以出现/消失/变宽而正文**不横移**。
- **C 常驻条**在 D 的滚动容器**之外**，用户滚消息时它们不动。
- **F Composer** 是浮层，D 的尾部要留出等高的 clearance。
- **G Context Dock** 可折叠，它和左侧 drawer 都会改变 D 的宽度 —— 所以响应式断点必须是**容器查询**不是视口查询。

### 1.2 阅读列内部（D 区）

```
D1  ──────────── 8 月 6 日 ────────────          ← 仅在日期变化处
D2  你 · 14:32
D3  ┌─────────────────────────────────┐
    │ 用户消息（贴右，最宽 77%）      │
    └─────────────────────────────────┘
D2  Agent · 14:32
D3  ▸ 思考了 4 秒                                 ← 折叠的 reasoning
D3  ┌ 工具卡 ────────────────────────────────────┐
    └────────────────────────────────────────────┘
D3  助手正文（Markdown，占满阅读列宽）
D4  [复制] [重新生成] [👍] [👎]                   ← hover / 末条常驻
D5  ✓ 已完成 · 42s · 8 步 · ↑12k ↓3.4k · $0.08
```

### 1.3 卡片内部槽位（工具卡为例）

```
┌─ T0 卡框 ───────────────────────────────────────────────┐
│ T1  [icon]  标题                        meta · meta · ✓ │  ← 一行，永不换行
│ T2          detail（mono，次级色）                      │  ← 可缺席
│ T3  ┌────────────────────────────────────────────────┐  │
│     │ 展开体 / preview                               │  │  ← 默认收起
│     └────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**只读工具没有 T0 卡框**，退化成一行（见 §4.5「卡形」）。

---

## 2 · 协议类型（全量 TS）

以下类型即 wire 上的形状，可直接复制使用。

### 2.1 流事件

```ts
/** 服务端推给客户端的唯一 run 事件方法：notifications.run.event */
interface RunEvent {
  runId: string;
  segmentId: string;
  eventId: string;
  timestamp: string; // ISO-8601
  event: StreamEvent;
}

type StreamEvent =
  | { type: "segment.started"; run: RunRef }
  | { type: "segment.progress"; progress: RunProgress }
  | { type: "segment.finished"; outcome: SegmentOutcome; metrics: RunMetrics }
  | { type: "item.started"; item: Item }
  | { type: "item.delta"; itemId: string; delta: ItemDelta }
  | { type: "item.completed"; item: Item }
  | { type: "state.snapshot"; state: StateSnapshot }
  | { type: "custom"; name: string; payload?: unknown };
```

| 事件 | 权威 | 可重放 | 渲染含义 |
| --- | :-: | :-: | --- |
| `segment.started` | ✅ | ✅ | 可改变持久 UI |
| `segment.finished` | ✅ | ✅ | 同上 |
| `item.started` | ✅ | ✅ | 同上 |
| `item.completed` | ✅ | ✅ | 同上 |
| `state.snapshot` | ✅ | ✅ | 同上 |
| `segment.progress` | ⬜ | ⬜ | **只能改善实时观感** |
| `item.delta` | ⬜ | ⬜ | 同上 |
| `custom` | ⬜ | ⬜ | 同上；**禁止承载正确性依赖的事实** |

> `segment.finished` **不带 `run`** —— 段结束时 Run 的完整状态要从 `runs.get` 读。

### 2.2 Item（六变体）

```ts
type ItemStatus = "running" | "completed" | "incomplete";

type ContentBlock =
  | { type: "text"; text: string }
  | { type: "image"; mime: string; data: string }; // data = 纯 base64，无 "data:" 前缀

type Item =
  | { type: "userMessage";  id: string; runId: string; status: ItemStatus; createdAt: string;
      content?: ContentBlock[] }
  | { type: "agentMessage"; id: string; runId: string; status: ItemStatus; createdAt: string;
      content?: ContentBlock[] }
  | { type: "reasoning";    id: string; runId: string; status: ItemStatus; createdAt: string;
      text?: string; redacted?: boolean }
  | { type: "question";     id: string; runId: string; status: ItemStatus; createdAt: string;
      question?: Question }
  | { type: "toolCall";     id: string; runId: string; status: ItemStatus;
      startedAt: string;              // ← 唯一用 startedAt 而非 createdAt 的 Item
      finishedAt?: string;
      durationMs?: number;            // runtime 测的，不是客户端秒表
      safetyClass?: SafetyClass;
      tool?: ToolInvocation;
      error?: ProblemData }
  | { type: "compaction";   id: string; runId: string; status: ItemStatus; createdAt: string;
      summary?: string; droppedMessages?: number };
```

**`item.started` 的壳是空的**：`content` / `text` / `question` / `tool` 都可能缺席，随后由 delta 补、由 completed 落定。所有读取必须容忍缺席 —— 抛错会被 reducer 吞掉，那个块就永远不渲染。

### 2.3 增量

```ts
type ItemDelta =
  | { type: "content";        text: string; index?: number } // agentMessage 正文
  | { type: "reasoning";      text: string }
  | { type: "toolArguments";  argumentsTextDelta: string }   // JSON 文本片段，中途不是合法 JSON
  | { type: "toolOutput";     text: string };                // stdout 实时预览
```

每个预览通道都有**权威终值**：`content`→`agentMessage.content`；`reasoning`→`reasoning.text`；`toolArguments`→`tool.arguments`；`toolOutput`→`tool.result.output`。

### 2.4 工具调用

```ts
type SafetyClass = "safe" | "write" | "exec" | "network";

/** 核心只有一个工具形状，不是联合。"某个工具怎么富渲染"是客户端知识，不进协议。 */
interface ToolInvocation {
  name: string;                          // 身份 = 路由键（icon / preview / 分类全按它）
  arguments: Record<string, unknown>;    // 已解析的 JSON 对象，绝不是 JSON 字符串
  result?: unknown;                      // best-effort JSON，绝不双重编码；仅 completed 时有
}
```

**MCP 工具的两个身份不可互换**：`ToolInvocation.name` 是 `sanitize("<server>_<tool>")` 的**模型可见名**（非 `[A-Za-z0-9_-]` 换 `_`，超 64 截断），**有损** —— `("a_b","c")` 与 `("a","b_c")` 都塌成 `a_b_c`。不能反解它去关联工具目录。

### 2.5 Run

```ts
type RunStatus = "running" | "waiting" | "finished";

interface RunSummary {          // 列一行、连一棵树够用
  id: string;
  sessionId: string;
  status?: RunStatus;
  outcome?: RunOutcome;         // 仅 finished
  createdAt?: string;
  finishedAt?: string;          // 仅 finished
  model?: string;
  provider?: string;
  parentRunId?: string;         // 血缘三字段全有或全无
  rootRunId?: string;
  spawnedByItemId?: string;
}

interface RunRef extends RunSummary {
  metrics: RunMetrics;
  limits?: RunLimits;
  activeSegmentId?: string;     // 仅 running
  protocolProfile: RunProtocolProfile;
}

interface RunMetrics {
  steps: number;
  activeDurationMs: number;     // 不含等人的时间
  usage?: Usage;
}

interface RunLimits {           // 冻结的 run-tree policy，跨 child 与 resume 累计
  maxSteps?: number;
  maxBudgetUsd?: number;
  maxTotalTokens?: number;      // prompt + completion，≠ params.maxTokens（后者只管一次输出）
}

interface RunProtocolProfile {
  interruptTypes: InterruptType[];
  requiredFeatures: ("subagents")[];
}

interface RunProgress {         // 瞬时读数，无权威落点
  step?: number;
  activity?: string;            // runtime 给的进行中文案
  usage?: Usage;
  contextTokens?: number;       // 此刻窗口占用（压缩后回落），≠ usage.inputTokens（累计只增）
}

type RunOutcome =
  | { type: "completed" }
  | { type: "error";     error: ProblemData }
  | { type: "maxSteps";  detail?: string }
  | { type: "maxBudget"; detail?: string }
  | { type: "canceled";  detail?: string };

/** 段级终态比 Run 级多两个：段结束不等于 Run 结束 */
type SegmentOutcome =
  | { type: "interrupt"; interrupts: Interrupt[] }   // Run 转 waiting
  | { type: "suspended" }                            // Run 未结束
  | RunOutcome;

interface Usage {               // 非重叠细分：同一批 token 不会被计两次
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;             // 缺席 = 该模型不在定价表（≠ 免费）
  byModel?: Record<string, Omit<Usage, "byModel">>;
}
```

**三条 presence 规则是 schema 强制的**：`outcome` 只在 `finished`、`activeSegmentId` 只在 `running`、`finishedAt` 只在 `finished`。`status` 与 `outcome` **正交**，别用 outcome 反推 status。

### 2.6 人机协同（HITL）

```ts
type InterruptType = "approval" | "question" | "toolResult";

type Interrupt =
  | { type: "approval"; itemId: string; runId: string;
      payload: { tool: ToolInvocation; reason?: string;
                 risk?: "low" | "medium" | "high"; rememberable?: boolean } }
  | { type: "question"; itemId: string; runId: string;
      payload: { question: Question } }
  | { type: "toolResult"; itemId: string; runId: string;
      payload: { tool: ToolInvocation } };   // 客户端侧工具，由客户端执行后回填

interface Question { fields: QuestionField[] }   // 有序、非空、全部必答

type QuestionField =
  | { type: "text";   prompt: string; header?: string }
  | { type: "choice"; prompt: string; header?: string;
      options: QuestionOption[];       // ≥2，label 唯一
      multiple?: boolean;
      allowCustom?: boolean };

interface QuestionOption {
  label: string;
  description?: string;
  preview?: string;                    // 用于对比的预览体（mockup / 代码片段）
}

/** runs.resume 的回执 */
interface InterruptResponse { itemId: string; response: InterruptResponseValue }

type InterruptResponseValue =
  | { type: "approval"; decision: "approve" | "deny";
      editedArgs?: Record<string, unknown>;
      reason?: string;
      remember?: { scope: "session" | "project" | "global" } }
  | { type: "answer"; answers: string[][] }          // answers[i] ↔ fields[i]，同序
  | { type: "toolResult"; result?: unknown; error?: ProblemData };

/** interrupts.list 的分页单位：一个 waiting Run 的完整 interrupt 集 */
interface PendingInterruptSet {
  sessionId: string;
  rootRunId: string;
  createdAt: string;
  interrupts: Interrupt[];             // 必填非空
}
```

- **关联键 = `itemId`**，没有单独的 requestId。
- **`Interrupt.runId` 是"谁提出的"不是"谁在等"** —— 一棵树里 interrupt 集挂在 root 上，但每条记提出它的那个 Run。
- **拒绝 ≠ 取消**：拒绝走 `runs.resume{decision:"deny"}`，Run 继续（agent 换方案）；取消走 `runs.cancel`，硬终止整棵树。
- **半个集合无法应答** —— 所以分页单位是整集，不按条分页。

### 2.7 共享状态

```ts
type StateSnapshot =
  | { type: "plan"; plan: PlanSnapshot[]; revision: number;
      sessionId: string; updatedAt?: string };

interface PlanSnapshot {
  id: string;
  description: string;
  status: "pending" | "in_progress" | "completed";   // 至多一个 in_progress
}
```

传播规则（**三条都会影响 UI 正确性**）：

1. **只有整份快照，没有增量事件。**
2. **`segment.finished` 之前必发**该段改过的每个 key —— 收到终态的人就已经收到了终值。**这一段没改过就不发**（一份 revision 0 的空快照会被按 revision 折叠的客户端读作"清单被清空了"）。
3. **`revision` 单调只增**。重新发布一个更早的值（回退、导入归档）也是一次**新写入**，拿更大的 revision。

**冷读是一等公民**：`plan.get` 返回与事件**同形同 revision**的快照。重载 / 回退 / replay 窗口过期后靠它接回来。

### 2.8 错误

```ts
/** 唯一判别键是 type。三个落点共用：RPC error / RunOutcome.error / toolCall.error。
 *  没有 retryable 字段（那个布尔量除了 type 之外不带信息，且 false 与缺席不可分）。
 *  没有 channel 字段（一个 problem 属于哪条通道，由它落在哪儿决定）。 */
type ProblemData =
  // ── 裸 type：既无 detail 也无 docUrl，文案 100% 由客户端本地供 ──
  | { type: "mcp_authorization_failed" }
  | { type: "mcp_authorization_required" }
  | { type: "mcp_dial_failed" }
  | { type: "provider_not_configured" }
  | { type: "provider_test_failed" }
  // ── 带可选退避秒数 ──
  | { type: "provider_unavailable" | "rate_limited" | "timeout";
      detail?: string; docUrl?: string; retryAfterSeconds?: number }
  // ── 带必填退避秒数 ──
  | { type: "idempotency_in_progress"; detail?: string; docUrl?: string; retryAfterSeconds: number }
  // ── 带结构化成员（客户端因此不必从 detail 里 substring-match）──
  | { type: "invalid_params"; detail?: string; docUrl?: string;
      errors?: { field: string; detail: string }[] }
  | { type: "capability_not_negotiated"; detail?: string; docUrl?: string;
      requiredCapabilities: { type: "feature" | "interruptType" | "runtimeTopic" | "stateSnapshot";
                              name: string }[] }   // 必填非空，按 {type,name} 唯一
  | { type: "session_has_active_run"; detail?: string; docUrl?: string;
      activeRun: { runId: string; status: RunStatus } }
  // ── 其余 34 个，一律 { detail?, docUrl? } ──
  | { type: "agent_stuck" | "checkpoint_unavailable" | "child_run_canceled" | "denied_by_user"
          | "idempotency_conflict" | "internal_error" | "interrupt_not_open" | "invalid_api_key"
          | "invalid_protocol_version" | "invalid_request" | "item_not_found"
          | "mcp_authorization_attempt_not_found" | "mcp_server_already_exists"
          | "mcp_server_disabled" | "mcp_server_not_found" | "method_not_found"
          | "path_outside_root" | "provider_error" | "provider_rejected"
          | "replay_cursor_invalid" | "replay_unavailable" | "revision_conflict"
          | "run_finished" | "run_lost" | "run_not_found" | "run_not_root" | "run_waiting"
          | "session_busy" | "session_not_found" | "stale_segment" | "tool_failed"
          | "unsupported_mime" | "vcs_unavailable" | "workspace_unavailable";
      detail?: string; docUrl?: string }
  // ── 第三方命名空间 ──
  | { type: `plugin:${string}/${string}`;
      detail?: string; docUrl?: string; retryAfterSeconds?: number };
```

**`detail` 只是 runtime 真的说了的那句话。** 缺席时由客户端本地 locale 从 `type` 供词 —— runtime 刻意不供那句话，它会是一句没有翻译者能看见的英文。

---

## 3 · 视图模型（组件真正吃的东西）

wire 经过 fold 后变成下面这些。**组件只接触这一层**，不 import 协议类型。

```ts
type BlockStatus = "running" | "complete" | "incomplete" | "requires-action";

/** 内容区的原子。renderBlock 就是对这个联合做分发。 */
type ViewContentBlock =
  | { kind: "text";      text: string; status: BlockStatus; itemId?: string }
  | { kind: "image";     mime: string; data: string }
  | { kind: "reasoning"; reasoningId: string; text: string; status: BlockStatus }
  | { kind: "tool";      toolCallId: string }               // 只带 id，实体在 TurnFacts.toolCalls
  | { kind: "approval";  status: BlockStatus; itemId?: string; runId?: string;
                         toolName?: string; command: string; reason: string;
                         args?: Record<string, unknown>;
                         risk?: "low" | "medium" | "high";
                         rememberable?: boolean;
                         decision?: "approved" | "declined" }
  | { kind: "question";  status: BlockStatus; itemId?: string; runId?: string;
                         questions: QuestionItem[]; answered?: boolean; answers?: string[][] }
  | { kind: "compaction"; summary?: string; droppedMessages?: number };

type QuestionItem =
  | { type: "text";   prompt: string; header: string }
  | { type: "choice"; prompt: string; header: string;
      options: { label: string; description: string; preview?: string }[];
      multiple: boolean; allowCustom: boolean };

type MessageRole = "user" | "assistant" | "system";

interface Message {
  id: string;
  role: MessageRole;
  runId: string | null;        // 乐观气泡未落地时为 null
  createdAt?: string;          // 裸 ISO；合成的 assistant turn 可能没有
  blocks: ViewContentBlock[];
}

type ToolCallStatus = "running" | "ok" | "err" | "denied" | "requires-action";

/** 工具卡的完整输入。所有可选字段"缺席"都意味着"这一项不画"。 */
interface ToolCall {
  id: string;
  runId: string;
  name: string;                // wire 工具名 → icon / preview 路由
  fn: string;                  // 显示标题
  fnKind?: "path";             // fn 是路径 → 从左截断
  args: string;                // 累积的参数文本（未解析）
  status: ToolCallStatus;
  safetyClass?: SafetyClass;   // 决定卡形
  durationMs?: number;
  error?: string;              // status="err" 时的人读原因

  // ── 命令族 ──
  command?: string;            // 实际执行的命令行
  exitCode?: number;

  // ── 文件变更族 ──
  added?: number;
  removed?: number;
  diff?: ToolDiffRow[];        // 本次调用自己的补丁
  files?: number;              // 本次调用碰了几个文件

  // ── 读文件族 ──
  lines?: number;              // 文件总行数
  range?: { start: number; end: number };  // 实际返回的行窗口（非整文件时才有）

  // ── 检索族 ──
  hits?: number;

  // ── Plan 族 ──
  step?: string;                            // 当前进行中的步骤文字
  progress?: { done: number; total: number };

  // ── 操作分派型工具（lsp）──
  operation?: string;

  result?: string;             // 权威结果的文本形态
}

type ToolDiffRow =
  | { type: "hunk";    text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added";   rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

interface AgentRunView {
  id: string;
  sessionId: string;
  parentRunId: string | null;
  rootRunId: string;
  spawnedByItemId: string | null;
  status: "running" | "waiting" | "finished";
  activeSegmentId: string | null;
  outcome: AgentRunOutcome | null;
  metrics: { steps: number; activeDurationMs: number; usage: RunUsage };
  progress: { step?: number; activity?: string; usage?: RunUsage; contextTokens?: number } | null;
  createdAt: string;
  finishedAt: string | null;
}

interface RunUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  costUsd?: number;            // 缺席 ≠ 0
}

interface AgentProblem {
  code?: string;               // 一切分支只读这个
  message?: string;            // runtime 的 detail，缺席时本地供词
  retryAfterSeconds?: number;
}

/** 一条 turn 渲染时能看到的会话事实，已按 turn 收窄 */
interface TurnFacts {
  toolCalls: Record<string, ToolCall>;
  delegatedRuns: Record<string /* 父 toolCallId */, DelegatedRunNarrative[]>;
}

/** transcript 的共享控制，刻意不含任何会话数据 */
interface BlockCtx {
  onSelectTool: (id: string) => void;
  onToggleExpand: (id: string) => void;
  expandedIds: Set<string>;
  instant?: boolean;           // 用户自己刚打的字，不重放揭示动画
  typewriter?: boolean;        // 全局流式揭示偏好
}
```

---

## 4 · 内容区渲染物（D 区）

> **本节的所有 ASCII 线框都是现状速写**，画出来只是为了让「这块数据大概是个什么东西」有个形。
> 分几档、什么顺序、折不折、什么图标 —— **重做视觉时全部可推翻**。
> 每节里真正要遵守的是 **字段表**（数据）和 **加粗的句子**（正确性约束）。

### 4.0 总表

| # | 渲染物 | 位置 | 来自 | 状态 |
| --- | --- | --- | --- | --- |
| 4.1 | 用户消息 | D3，贴右，≤77% 宽 | `Item{userMessage}` | 原子 |
| 4.2 | 助手消息 | D3，占满阅读列 | `Item{agentMessage}` + `delta{content}` | running → complete |
| 4.3 | 图片 | D3，跟在文本后 | `content[].type="image"` | 原子 |
| 4.4 | 思考 | D3 | `Item{reasoning}` + `delta{reasoning}` | running → complete → 折叠 |
| 4.5 | 工具卡 | D3 | `Item{toolCall}` + 两种 delta | 5 态 |
| 4.6 | 工具组 | D3 | 相邻只读调用 | 派生 |
| 4.7 | 折叠波 | D3 | 已被答案覆盖的 process 段 | 派生 |
| 4.8 | 审批卡 | D3 | `SegmentOutcome{interrupt}` | 待决 → 已决 |
| 4.9 | 提问卡 | D3 | `Item{question}` / interrupt | 待答 → 已答 |
| 4.10 | 压缩条 | D3，全宽无 chrome | `Item{compaction}` | 原子 |
| 4.11 | 子 agent 叙事 | D3，挂在父工具卡下 | 子 run 的全部 Item | 跟随子 run |
| 4.12 | Run 收尾行 | D5 | `segment.finished` | 仅非 error 终态 |
| 4.13 | 日期分隔 / caption / 操作条 / 等待指示 | D1 / D2 / D4 / D3 尾 | 派生 | — |

---

### 4.1 用户消息

**位置** D3，贴阅读列右缘，宽度只取所需、上限 77%。
**何时** 用户发送后立刻出现（乐观气泡），`item.completed` 到达后被真实 Item 接管。

| 字段 | UI | 位置 |
| --- | --- | --- |
| `content[].text`（合并后） | Markdown 正文 | D3 气泡内 |
| `content[].image` | 缩略图，点击开灯箱 | D3 气泡内，文本之后 |
| `createdAt` | 时钟 | D2 caption 右侧 |
| `id` | React key + 定位锚 | — |
| `runId` | 编辑重跑 / 从此分叉的锚 | D4 操作条 |

**渲染要求**

- **`instant: true`** —— 不走打字机揭示。用户自己刚打的字重放一遍是荒谬的。
- 用户 turn **只取所需宽度**；助手 turn 占满。两者都全宽会让 transcript 读成"两种文档交替"而不是"一份文档带旁注"。
- 图片 `data` 是**纯 base64**，前端负责拼 `data:${mime};base64,${data}`。

---

### 4.2 助手消息

**位置** D3，占满阅读列宽。

| 字段 | UI | 位置 |
| --- | --- | --- |
| `content[].text` / `delta{content}.text` | Markdown（含代码高亮、表格、公式、mermaid） | D3 |
| `status === "running"` | 尾部流式光标 | 正文末尾 |
| `createdAt` | 时钟 | D2 |

**渲染要求**

- **只有最后一个 text 块允许显示光标**：一条 turn 里非末尾的 running 文本要强制降为 complete —— 已完成 turn 中间闪光标是谎言。
- **壳阶段 `content` 缺席**要折成一个空 text 块供 delta 打补丁，不是跳过。
- Markdown 包裹层用 `<div>` 不是 `<p>`（渲染器自己会产出 `<p>`，嵌套是非法 HTML，浏览器会静默拆开外层）。

---

### 4.3 图片

**位置** D3，紧跟同条消息的文本块。
**字段** `mime` `data`。
**要求** 需**预留盒子**（宽高比）避免加载完成后跳版；点击进灯箱。

---

### 4.4 思考（Reasoning）

**位置** D3。
**何时** 模型支持推理且 `features.reasoning` 开启。与助手正文可能交错到达。

```
折叠态：  ▸ 思考                                   ← 一行，mono header，不大写
展开态：  ▾ 思考
          思考正文（mono，次级色，不作 Markdown 渲染）
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `text` / `delta{reasoning}.text` | 思考正文 | 展开体 |
| `status === "running"` | 进行中脉冲 + 自动展开 | 折叠头 |
| `redacted === true` | **只画"已思考"的壳，不画正文** | 折叠头 |
| 派生 `superseded` | 后面已有带内容的答案 → 自动折叠 | — |

**陷阱**

- `redacted=true` 时即使 `text` 非空也**不得渲染** —— 那是 provider 的加密载荷。
- `superseded` 的判据必须是「后面存在 `text.trim() !== ""` 的文本块」。用"块存在"当判据会在模型刚开口、一个 token 都还没到时就把思考折掉。

---

### 4.5 工具卡 —— 内容区最大的一块

**位置** D3。

#### 分量：一次调用该占多大地方

**数据依据（硬）**：`safetyClass` 与 `status`。
**现状答案（软）**：分三档 —— 出问题的最重、只读的最轻、有副作用的居中。分几档、怎么区分，重做时随意。

```
① flagged（err / denied / requires-action）—— 状态优先，失败的读不再是一瞥
   ┌──────────────────────────────────────────────┐
   │ ⚠ [icon] 标题                        · 状态  │
   │          错误原因（取代 detail）             │
   └──────────────────────────────────────────────┘

② line（safetyClass === "safe"）—— 一行，无卡框
     [icon] 标题                       meta · meta ✓

③ card（write / exec / network / 未知）
   ┌──────────────────────────────────────────────┐
   │ [icon] 标题                      meta · meta │
   │        detail（mono）                        │
   │ ┌──────────────────────────────────────────┐ │
   │ │ preview                                  │ │
   │ └──────────────────────────────────────────┘ │
   └──────────────────────────────────────────────┘
```

**为什么要分**：一条 turn 可以有一打「读」和一条「命令」，全给一样的分量，transcript 就是一整片灰。
**硬约束**：`safetyClass` **缺席时按"非只读"处理**（与审批门同一张表 —— 卡的分量和门的判断不能互相矛盾）。

#### 状态：五种，各自要传达什么

| status | 何时 | **要传达的意思** | 硬约束 |
| --- | --- | --- | --- |
| `running` | `status === "running"` | 还在跑 | — |
| `ok` | 正常结束 | 做完了，没什么可说 | 「没什么可说」的记号只在**真的没什么可说**时画（无任何 meta、无 diffstat）；有数字可报时那个数字就是结论 |
| `err` | 有 `error` 或 `incomplete` | 失败了，这是原因 | 错误原因**取代**副行 —— 哪里错了压过它本来要做什么 |
| `denied` | `error.type === "denied_by_user"` | **用户拒绝了** | **绝不能画成失败** —— 用户的决定不是错误。它需要一个和 `err` 明显不同的表达 |
| `requires-action` | 挂在一个未决审批上 | 在等你 | 要能一眼与 `running` 区分 |

#### 字段 → 要表达什么 → 大概在哪

一张工具卡在回答三个问题：**它做了什么**（主）· **对什么做的**（次）· **结果如何**（数字）。字段按这三组分：

**① 它做了什么 —— 主槽**

| 字段 | 表达 | 硬约束 |
| --- | --- | --- |
| `name` | 工具身份 → 图标 | **一工具一字形**（三十种形状，不复用）。**颜色不表达身份** —— 颜色留给状态，不然一个失败的读和一个成功的读会一样刺眼 |
| `fn` | 标题：这次调用叫什么 | — |
| `fnKind === "path"` | 标题是一条路径 | **从左截断**，保住文件名 |

**② 对什么做的 —— 次槽**

| 字段 | 表达 | 硬约束 |
| --- | --- | --- |
| `error` | 失败原因 | `err` 时**取代**次槽内容 |
| `command` | 实际执行的命令行 | 这是读者真正要核对的一行，**不截断** |
| `step` | Plan 当前进行的步骤 | — |
| 参数里首个命中的 `path` / `query` / `pattern` / `url` | 兜底的作用对象 | 路径仍要保住文件名 |

**③ 结果如何 —— 数字组**（全部可缺席，**缺席 = 不画**）

| 字段 | 表达 | 硬约束 |
| --- | --- | --- |
| `added` / `removed` | 改了多少行 | **一个事实两个数**，不要拆成两个独立标签。**双缺席时不画**（不是 `+0 −0`，也不是短横 —— 短横是一个要读者停下来解读的记号） |
| `progress` | Plan 走到哪 | 用**裸记号**（`3/7`），各语言同形，不进翻译目录 |
| `files` | 碰了几个文件 | — |
| `hits` | 命中多少 | — |
| `range` | 实际返回的行窗口 | **仅当不是整个文件时才有意义**，否则它只是把 `lines` 又写一遍 |
| `lines` | 文件多长 | — |
| `exitCode` | 退出码 | **`≠0` 不代表失败**（grep 无匹配就是 1）。可以标成需要注意，但**不能让整张卡变成失败态** —— 真失败会设 `error` |
| `durationMs` | 耗时 | 只在**足够长**时才值得占位（现状阈值 1s）。这是**服务端测的**，客户端秒表测的是自己的渲染循环 |
| `status === "running"` | 正在跑 | 此时其余数字大多还没有 |
| `result` / `diff` | 完整结果 | 展开体，见 §7 逐工具 |

**软的部分**：这些数字现状是排成一行右对齐的小标签、顺序固定。放哪、怎么排、要不要全部同时出现，重做时自定 —— 只要守住上面的「缺席不画」和几条不能误导的规则。

#### 交互

- 最新的工具会被**自动选中**（喂给 G 区 inspector）但**不自动展开** —— 展开是用户的点击。
- 展开状态由 `BlockCtx.expandedIds` 持有，跨会话切换不保留。

#### 两个工具的行会被丢弃

`ask_user` 与 `exit_plan_mode` 在自己的执行里发起 HITL，于是同时产生一个 toolCall Item（被抽干成 `incomplete`、读作红 ✗）**和**一个 question Item。**提问卡才是真身，工具行是它的冗余影子** —— 渲染器在问题块存在时丢掉这一行。

`set_plan` / `create_goal` / `get_goal` / `report_goal_outcome` 的行也被丢弃：它们的结论由 C3 / C4 常驻条回答（见 §5.3 / §5.4）。

---

### 4.6 工具组

**位置** D3，取代连续多张工具卡。
**何时** **相邻**的多个只读调用（`safetyClass === "safe"`）。

```
▸ 读了 3 个文件 · 搜了 2 次                        ← 折叠，一行
▾ 读了 3 个文件 · 搜了 2 次
    [icon] a.ts                              120 行
    [icon] b.ts                               48 行
    …
```

摘要按 `read` / `lsp` / 其余 三分。组内任一条 `running` 或 `err` → 组需要注意，不自动折叠。

---

### 4.7 折叠波（Wave）

一条 turn 在「做」和「答」之间交替，而读者是为**答案**来的。所以：**每一段后面已经有答案的 process 块，折成一行可展开的"它是怎么得出这个答案的"。**

```
▸ 4 步工作                                        ← wave 折叠态
▾ 4 步工作
    ▸ 思考
    [icon] 读了 3 个文件
    ┌ shell ─────────────────┐
    └────────────────────────┘
```

规则：

| 规则 | 说明 |
| --- | --- |
| process = `reasoning` + `tool` **而已** | 审批 / 提问**不是** process（它们在向读者要东西，折走一个待决请求 = 一条 turn 在沉默里结束）；图片、Plan 也不是（那是读的不是做的） |
| 正在进行的那一段**永不折** | 那正是读者在看的 |
| wave 至少要两个单元 | 一个已经规划成一行的（孤立思考、自成一组的三个读）自己会折，再包一层只是多一级要打开 |
| 单元之间的距离归包装层 | 距离是两个单元之间的**关系**，没有卡片能知道自己离一个从没听说过的邻居有多远 |

---

### 4.8 审批卡

**位置** D3。
**何时** `segment.finished{outcome:{type:"interrupt"}}` 里带 `approval`。此时 Run 已转 `waiting`，资源全释放。

```
┌─ 需要你批准 ─────────────────────────── [risk] ─┐
│ [icon] 执行命令                                  │
│                                                  │
│   $ rm -rf ./dist                                │
│                                                  │
│ ⚠ 递归删除                        ← 客户端启发式 │
│ 原因：清理构建产物                ← payload.reason│
│                                                  │
│ ▸ 参数（可编辑）                  ← 仅部分工具   │
│                                                  │
│ [拒绝]  [批准]        □ 以后都这样 ▾（会话/项目/全局）│
└──────────────────────────────────────────────────┘
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `payload.tool.name` | 标题（"执行命令" / 工具名） | 卡头 |
| `payload.tool.arguments.command` | `$ cmd` 行（mono，**不截断**） | 卡身 |
| `payload.reason` | 原因句 | 命令下方 |
| `payload.risk` | 风险标（low/medium/high） | 卡头右 |
| `payload.rememberable` | 是否显示"以后都这样" | 卡脚 |
| 派生 `args` | 可编辑参数区（**只对自由形态工具开放**） | 卡身可折叠区 |
| 派生 危险提示 | 客户端正则启发式，与后端 `risk` **相互独立** | 命令下方 |
| `itemId` | React key + resume 关联键 | — |

**客户端危险启发式**（漏判只是少个横幅，误判只是多个横幅）：递归删除 `rm -rf` · 提权 `sudo`/`doas` · 管道进 shell `curl … | sh` · 覆写设备 `dd of=` · 格式化 `mkfs` · 全局可写 `chmod 777` · fork 炸弹 · 裸盘写 · 强制推送 `git push -f`。

**渲染要求**

- 三态：**待决**（阻塞视线）→ **已批准** / **已拒绝**（收成一行记录留在 transcript 里）。
- **React key 必须是 `itemId`**，不能是数组下标 —— 卡持有本地草稿（记忆勾选、编辑过的参数），下标 key 会把上一个 interrupt 的草稿泄漏给下一个。
- 重连 / 重放再看到同一个 interrupt 要 **upsert 重申**同一张卡，不能追加第二张。
- 「以后都这样」的记忆键是**模型可见名** —— 塌名的两个 MCP 工具会共享一条规则，UI 不要假装能区分。

---

### 4.9 提问卡

**位置** D3。
**何时** `Item{question}` 出现，或 interrupt 里带 `question`。

```
┌─ 需要你确认 ─────────────────────────────────────┐
│ [Auth 方式]  用哪种鉴权？                        │
│  ○ OAuth 2.1（推荐）   ┌ preview ──────────────┐ │
│    DCR + PKCE          │  code / mockup 对比   │ │
│  ○ Bearer token        └───────────────────────┘ │
│  ○ 其他… ______________          ← allowCustom   │
│                                                  │
│ [提交]                                           │
└──────────────────────────────────────────────────┘
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `fields[].header` | 短标签 chip（≤12 字符） | 每个 field 上方 |
| `fields[].prompt` | 问题文字 | header 右 |
| `fields[].type="text"` | 单行/多行输入 | field 体 |
| `fields[].options[].label` | 选项 | field 体 |
| `options[].description` | 选项下的一行解释 | 选项内 |
| `options[].preview` | **对比预览体** → 布局切成「左选项列 + 右预览」 | field 右侧 |
| `fields[].multiple` | 单选 → 多选 | field 体 |
| `fields[].allowCustom` | 追加一个「其他」输入 | 选项末 |

**渲染要求**

- **全部必答**：协议里没有 optional field 这回事 —— **不要画"跳过"**。
- **不是表单子系统**：只有 `text` 和 `choice` 两种，没有 number / date / 条件显示 / 外部数据源。别往这个方向加。
- `preview` 只在**单选**时有意义（多选没法左右对比）。
- 回执 `answers[i]` 与 `fields[i]` **按同序对应** —— 没有可从顺序推导的 field id，别发明一个。
- React key 同 §4.8。

---

### 4.10 压缩分隔条

**位置** D3，但作为 **system 角色**：无头像、无名字、无时钟、无轮廓、无右键菜单，**全宽**，自己画一条分隔线。

```
──────────── 已压缩 42 条更早的消息 ────────────
```

| 字段 | UI |
| --- | --- |
| `droppedMessages` | "已压缩 n 条更早的消息" |
| `summary` | 摘要（**通常缺席** —— 摘要文本已折进重写后的历史，Item 只表达边界） |

它是时间线上的一道坎，不是一个人在说话。

---

### 4.11 子 agent 叙事

**位置** D3，**缩进挂在父工具卡（`delegate_task`）正下方**。

```
┌ delegate_task ─ 修复登录态竞态 ──────────────┐
└──────────────────────────────────────────────┘
  ▸ ● 委派任务 (1/2) · 运行中 · 12 步 · 正在读取文件
  ▾ ● 委派任务 (2/2) · 等待中 · 4 步
      ┌ 子 agent 的完整 transcript（递归同一套渲染）┐
      │  思考 / 工具卡 / 审批卡 / 助手消息          │
      └────────────────────────────────────────────┘
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `spawnedByItemId` | **挂载点** —— 决定挂在哪张父卡下 | — |
| `status` + `outcome` | 状态点 + 状态词 | 折叠头 |
| `progress.activity` | 未终态时的 detail | 折叠头 |
| `metrics.steps` / `progress.step` | 步数 | 折叠头 |
| 兄弟序号 / 总数 | "委派任务 (i/n)"；只有一个时不带序号 | 折叠头 |
| `status !== "finished"` | 取消按钮 | 折叠头右 |
| 子 run 的全部 Item | 递归渲染 | 展开体 |

**`waiting` 状态自动展开，其余全部自动折叠。** 一个 waiting 的子 run 有人得去处理的 interrupt；因为父开始说话就把它折走 = 一条 turn 在沉默里死锁。

**陷阱**

- 子 run 的 Item 与父的 Item 在**同一条流上交错到达**。每个 run 必须有自己的 turn 游标，用一个全局游标会把子 Item 加进父的 turn。
- 血缘三字段（`spawnedByItemId` / `parentRunId` / `rootRunId`）**全有或全无**。`features.subagents` 关闭时三者恒空 —— **形状在、行为关**，不要据此报错。

---

### 4.12 Run 收尾行

**位置** D5，transcript 末尾，一条带图标的分隔线。
**何时** 最新的根 Run 结束，**且终态不是 error**（error 归 §5.2 的可操作恢复条 —— 那是读者能做点什么的，这里只放读者无能为力的）。

```
──── ✓ 已完成 · 42s · 8 步 · ↑12.4k ↓3.4k · $0.08 ────
```

| `outcome.type` | 图标 | 意图色 | 附加 |
| --- | --- | --- | --- |
| `completed` | ✓ | accent | — |
| `canceled` | ■ | neutral | `detail`（区分"被用户取消" vs "被超时取消"，`runs.cancel` 的 reason 经此回流） |
| `maxSteps` | ⚠ | neutral | `detail` |
| `maxBudget` | ⚠ | neutral | `detail`（给出花费/上限） |

数字来自 `metrics`：`activeDurationMs`（**不含等人的时间** —— 停在审批上过夜的 run 不因此变贵）、`steps`、`usage.inputTokens`（`↑`）、`outputTokens`（`↓`）、`costUsd`。
**窄面板下数字隐藏** —— 词是事实，数字是脚注。

---

### 4.13 turn 周边

| 渲染物 | 位置 | 来自 | 要点 |
| --- | --- | --- | --- |
| **日期分隔** | D1 | `Message.createdAt` | **硬**：只在日期**变化处**出现一次；**无时间戳的 turn 既不开新日也不断链**（缺席是没信息，不是另一天） |
| **turn 标识** | D2 | `role` + `createdAt` | 谁在说 + 什么时候。**软**：现状是一行压在全宽正文之上的 caption 而非头像侧槽 —— 侧槽会从下面每个代码块 / diff / 表格里扣掉阅读宽度，重做时值得先算这笔账 |
| **turn 操作** | D4 | 派生 | 复制 / 编辑重跑 / 重新生成 / 反馈。**硬**：流式期间不要出现（还没写完的东西没什么可复制）。**软**：现状是末条常驻、其余 hover |
| **等待指示** | D3 尾 | `running` 且尾行是 user | 助手消息一到就交班隐藏 |
| **turn 间距** | D 区 | 派生 | **软，但值得保留这个判断**：turn 内块之间的距离要明显小于 turn 之间。扁平的间距会让一条 turn 自己的块隔得跟两条 turn 一样远，页面上就没有任何东西在说"这些是一体的" |

---

## 5 · 常驻条（C 区）

这四样东西的共同点：**它们回答的是"现在是什么状态"，不是"刚才发生了什么"** —— 所以必须**不随消息滚走**。这一条是硬的；长什么样、放哪、是不是横条，都是软的（现状是四条与阅读列同宽的横幅，压在滚动区之上）。

> 本节线框同样是现状速写。

### 5.1 工作区失联（C1）

**何时** `Session.workspace.availability === "missing"`。

```
┌ ⚠ 工作目录不可用 ───────────────────────────────┐
│ /Users/me/project 已失联。可以继续纯对话，       │
│ 或重新指定目录。            [重新指定…]          │
└──────────────────────────────────────────────────┘
```

**这不是错误态，是必须显式渲染的事实**：降级成纯聊天 + 提供重定位（受 `relocate` feature 门控）。
条要**按 session 重置**，否则半打的路径会跨会话带过去。

### 5.2 Run 错误条（C2）

**何时** 最新 Run 的 `outcome.type === "error"`，或命令级错误。

```
┌ ⚠ 运行失败 · rate_limited ───────────────── [×] ─┐
│ 上游限流，请稍后重试。                            │
│ [↻ 12 秒后重试]  [打开时间线]  [打开诊断]         │
└───────────────────────────────────────────────────┘
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `code`（`ProblemData.type`） | 标题后缀 + **一切行为分支的唯一依据** | 标题右 |
| `message`（`detail`） | 正文；**缺席时由本地 locale 从 `code` 供词** | 条身 |
| `retryAfterSeconds` | 重试按钮上的倒计时（期间按钮可见但**不可点** —— 别去锤一个刚请你等待的服务） | 动作行 |

**重试门禁按 `type` 判**，当前不可重试集：`invalid_api_key` / `invalid_params` / `provider_rejected`（重发同样的文本改变不了凭证、请求形状或对方的判决）。

**错误不能是死路**：负向色只落在图标 / 标题 / 一条 1px 边，**不冲刷整个阅读宽度**；必须给具体下一步。

### 5.3 Plan 进度条（C3）

**何时** 当前 Plan 存在 `in_progress` 步，且未被本 Run dismiss。

```
┌ 计划 ─────────────────────────────── 3/7 ─ [×] ─┐
│ ▸ 正在：把 fold 层的 plan 投影接到常驻条        │
│ ████████████░░░░░░░░░░░░░░░░  43%               │
└──────────────────────────────────────────────────┘
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `plan[].status === "in_progress"` 的那条 `description` | 当前步文字 | 条身 |
| `done` / `total`（派生） | `3/7` + 进度条 | 条头右 / 条底 |
| `plan[]` 全量 | 展开的清单（勾选态三档） | 展开体 / G 区 Plan 视图 |
| `revision` | 折叠依据，**不上屏** | — |

**冷读**：重载 / 回退 / replay 过期后靠 `plan.get` 接回来。

### 5.4 Goal 预算条（C4）

**何时** 当前会话有一个自治 Goal。数据来自 `goals.get` + `goals.changed` 失效（**不轮询**）。

```
┌ 目标 ── 运行中 ──────────────────── 花费 62% ─┐   ← 折叠：只显示最紧的那条轴
│ 把 desktop 的内容区渲染文档补完                │
└────────────────────────────────────────────────┘
展开：
│  运行数  ████████░░░░░░  8 / 20                │
│  花费    ███████████░░░  $6.20 / $10.00        │
│  步数    ─────────────── 无限制                │   ← 未设限的轴不画轨道
```

| 字段 | UI | 位置 |
| --- | --- | --- |
| `objective` | 目标文字（一行，从右截断） | 条身 |
| `status` | 状态标：`active`=中性 / `paused`=warning / `blocked`=negative | 条头 |
| `budget.maxRuns` / `maxCostUsd` / `maxSteps` | 三条预算轴。**0 或缺席 = 该轴不设限** | 展开体 |
| `used.runs` / `costUsd` / `steps` | 已用值 | 展开体 |
| 最紧的轴（派生） | 折叠态唯一显示的数字 | 条头右 |
| `reason.code` | 停止原因（**闭合 10 值，查表出文案**） | 条身 |
| `reason.detail` | **不直接显示** —— 会把英文塞进每个 locale | — |

**未设限的轴不画进度条** —— "无限制"下面一条满宽轨道读作"快用完了"，正好是反义。
折叠态只有一个数字的位置，三个会是噪音；有用的是**最先耗尽的那条**（未设限的轴不是候选，它永远不会是终止原因）。

`reason.code` 全集：`stoppedByUser` `runtimeRestarted` `runStartFailed` `awaitingInput` `terminalOutcomeMissing` `runNotCompleted` `runBudgetReached` `costBudgetReached` `stepBudgetReached` `blockedByModel`。

### 5.5 会话用量 chip

**位置** B 顶栏右 / F Composer 工具条。
**来自** `usage.session{sessionId}`。

```
↑ 128k  ↓ 34.2k  $1.24
```

三项全为 0 且无花费 → **不画**（不是画三个 0）。`↑`/`↓` 的记号与 §4.12 收尾行**同一套写法**。

---

## 6 · 长对话的导航（E 区）

**要解决的问题**：一条几十轮的会话，读者需要知道**自己在哪**、以及**怎么跳到某一轮**。数据只有 `messages[]` 的顺序与每条的 DOM 高度 —— 协议不提供任何"章节"概念。

现状答案是阅读列左侧一条按高度比例分布的刻度轨。**形式随便换**，但下面四条踩过的坑值得带走：

- **刻度按每条 turn 的实际高度比例分布**，不是等距圆点 —— 等距会让一条 20 行的回答和一条 3 行的回答在导航上一样大，那就不是"我在哪"了。
- **导航元素的出现 / 消失 / 变宽不能让正文横移。** 正文的位置绝不能依赖它 —— 否则一条内容为空的 turn 会在滚动中途把整段文字拽向一侧。
- **透明区域不能吞指针。** 一整片什么都没画、却接收滚轮的空白，会让面板在光标停留处莫名其妙停止滚动。
- **别让可点击的东西跑到浮层底下。** 压在输入框下面的刻度是点不到的刻度。

---

## 7 · 三十个内置工具

### 7.0 读法

每个工具给：**class**（`safetyClass` → 卡形 + 审批门）· **卡形** · **图标** · **进行中文案** · **标题/副行取自哪个参数** · **TS 入参/出参** · **展开体**。

三条通则：

1. **只有 5 个工具的结果被 runtime 归一化并登记了 schema**：`glob` `grep` `apply_patch` `shell` `web_search`。它们的 result 是 camelCase 且有契约守着。**其余 25 个的结果形状是约定不是契约** —— 改了不会有任何门禁报警，所有读取必须防御式。
2. 归一化**幂等**：结果里已有目标键就原样返回。
3. 多数只读工具直接返回**模型读的散文**。给它们做展开体 = 解析散文，parser 只能锚在后端真的会发的**那一处结构**上，找不到就退化成纯文本 —— 后端改文案的代价是"展开体变朴素"，不是"展开体说错话"。

> **图标是一工具一字形，有测试守着。** 曾经 16 个字形铺 32 个工具，滚动的 transcript 就是四种形状在重复 —— 等于没有形状。
> **颜色不参与工具身份**：tone 只表达状态（running / failed / refused）。三十个形状承担变化，调色板保住它自己的职责。

---

### 7.1 文件与代码（5）

#### `read` — 读文件

`safe` · **一行** · 图标 `eye` · 进行中「Reading file」
**标题** = `path`（路径，从左截断）｜**chips** = `range` `lines`｜**展开体** 带行号的文件头

```ts
interface ReadArguments {
  path: string;         // 绝对或相对工作区根
  start_line?: number;  // 1-based，省略 = 从第 1 行
  max_lines?: number;   // 省略 = 读到文件尾
}
interface ReadResult {  // snake_case：未归一化，原样透传模型侧形状
  content: string;
  start_line: number;   // 1-based 闭区间
  end_line: number;
  total_lines: number;
  truncated?: boolean;
}
```

- `content` **直出为文本**，不要 JSON.stringify（转义后的信封是噪音）。
- `range` chip **仅当不是整个文件**时画（`start > 1 || end < total_lines`）—— 否则它只是把 `lines` 又写了一遍。

#### `glob` — 按文件名找

`safe` · **一行** · 图标 `folder-search` · 「Finding files」
**标题** = `pattern`｜**chips** = `hits`｜**展开体** 路径列表

```ts
interface GlobArguments {
  pattern: string;       // doublestar，如 **/*.go
  path?: string;         // 起始目录，默认工作区根
  ignore_case?: boolean;
  max_results?: number;  // 1–1000，默认 100
}
// 原始：{ paths: string[]; truncated?: boolean }
interface GlobResult {   // ← runtime 归一化后
  hits: { path: string; snippet?: string; lineNumber?: number }[];  // glob 只填 path
}
```

#### `grep` — 按内容找

`safe` · **一行** · 图标 `text-search` · 「Searching」
**标题** = `pattern`｜**chips** = `hits`｜**展开体** 命中行（重查工作区检索接口）

```ts
interface GrepArguments {
  pattern: string;                 // ripgrep 语法（interface\{\} 要转义）
  path?: string;                   // 文件或目录，默认工作区根
  file_glob?: string;
  file_type?: string;              // ripgrep file type：go / ts / rust
  ignore_case?: boolean;
  multiline?: boolean;             // 需要 ripgrep
  before_context_lines?: number;   // 0–20
  after_context_lines?: number;    // 0–20
  output_mode?: "content" | "files_with_matches" | "count";  // 默认 content
  max_results?: number;            // 1–1000，默认 250
}
// 原始按 output_mode 三选一：{ matches[] } | { files[] } | { counts[] }（+ truncated?）
interface GrepResult {             // ← 归一化把三种全折进一个信封
  hits: { path: string; snippet?: string; lineNumber?: number }[];
}
```

归一化映射：`matches` → `{path, snippet: text, lineNumber: line}`；`files` → `{path}`；`counts` → `{path, snippet: "N matches"}`。

#### `lsp` — 语言服务查询

`safe` · **一行** · 图标 `code` · 进行中文案**按 operation 分**（「Finding a symbol definition」/「Inspecting a symbol」/…）
**标题** 按 operation：`workspace_symbols` → `query`；`document_symbols`/`diagnostics` → `path`；其余 → `path:line:character`
**展开体** 按 `operation` 换脸：`hover` → 单段文本；其余 → 结果行列表

```ts
interface LspArguments {
  operation:
    | "definition" | "references" | "implementation" | "hover"
    | "incoming_calls" | "outgoing_calls"
    | "document_symbols" | "workspace_symbols" | "diagnostics";
  path?: string;       // 除 workspace_symbols 外必填
  line?: number;       // 1-based，位置类操作必填
  character?: number;  // 1-based 列，位置类操作必填
  query?: string;      // workspace_symbols 必填
}
type LspResult = string;   // 纯文本，未归一化
```

**这是一个 operation 分派的工具**，诊断是它的一个 operation —— 后端断言不存在独立的诊断工具。这也是**唯一允许展开体按字段分支**的工具（那是同一个工具的不同面孔，不是两个工具）。

#### `apply_patch` — 应用补丁

**`write`** · **卡片** · 图标 `replace` · 「Applying a patch」
**标题** = 单文件时 `changes[0].path`（路径）；多文件时工具名 + `files` chip
**chips** = `files`、`+n −m`｜**展开体** diff

```ts
interface ApplyPatchArguments {
  patch: string;   // 标准 unified diff 全文，支持 create / modify / delete / move
}
// 原始：{ files: [{ path, hunks, created?, deleted?, moved_from? }], hunks }
interface ApplyPatchResult {   // ← 归一化
  changes: {
    path: string;
    status: "added" | "deleted" | "modified" | "moved";  // 注意：这是文件系统变更状态
    from?: string;                                        // 仅 moved
  }[];
}
```

> `status` 用的是**文件系统变更**词汇（有 `moved`），**不是** VCS 扫描状态（那个有 `untracked`）。两者不可互相 cast，见 §9.5。

**这是唯一的文件变更工具** —— 没有 `edit`、没有 `write`（见 §10 D4）。

---

### 7.2 命令执行（3）

#### `shell` — 跑命令

**`exec`** · **卡片** · 图标 `terminal` · 进行中文案 = `description`（合规时）否则「Running command」
**标题** = **`description`**（人写的动作短语）｜**副行** = **`command`**（mono，不截断）
**chips** = `exit`（≠0）、`duration`｜**展开体** 终端输出

```ts
interface ShellArguments {
  command: string;      // 由 /bin/sh -c 跑。每次调用是全新 shell：cd / 变量 / shell option 都不持久
  description: string;  // ≤120，动作短语（"Run backend tests"）。非空、无首尾空白，后端校验
  timeout_millis?: number;               // 硬超时，省略 = 无
  run_in_background?: boolean;           // 立刻返回 shell_id，命令继续跑
  auto_background_after_seconds?: number;// 前台跑够这么久转后台，默认 60。与 run_in_background 互斥
}
interface ShellResult {   // ← 归一化
  output: string;         // stdout + stderr 已按到达顺序合并
  exitCode?: number;      // 后台启动时缺席 —— 没退出就没有退出码
}
```

原始形状三种：完成 `{stdout（含合并输出）, stderr（恒空）, exit_code, killed?, duration}`；转后台 `{stdout: "Command running in background as shell <id>. …"}`；缓冲溢出时 output 前置 `[earlier output dropped — buffer overflowed]`。

- **标题不用命令行**：那是把数据塞进"意图"槽，且和下面 mono 的副行逐字重复。
- 流式期间展开体由 `delta{toolOutput}` 顶着，completed 时被 `result.output` 覆盖。

#### `read_shell_output` — 读后台输出

`safe` · **一行** · 图标 `scroll` · 「Reading command output」
**标题** = `shell_id`｜**展开体** 终端输出

```ts
interface ReadShellOutputArguments {
  shell_id: string;
  wait?: boolean;           // 等它退出再返回。取代 sleep 轮询；别对 server / watcher 无限等
  timeout_millis?: number;  // 仅 wait=true 时有意义；wait=false 时传了会报错
}
type ReadShellOutputResult = string;
// "Shell <id> still running.\n<output>" | "Shell <id> finished (<info>).\n<output>" | "No background shell <id>."
```

#### `stop_shell` — 停后台命令

**`exec`** · **卡片** · 图标 `stop` · 「Stopping command」
**标题** = `shell_id`｜**展开体** 终端输出

```ts
interface StopShellArguments { shell_id: string }
type StopShellResult = string;
// "Killed background shell <id>." | "Background shell <id> had already exited." | "No background shell <id>."
```

---

### 7.3 网络（3）

#### `web_search` — 搜网页

**`network`** · **卡片** · 图标 `globe` · 「Searching the web」
**标题** = `query`｜**chips** = `hits`｜**展开体** 结果卡（标题 / 域名 / 摘要）

```ts
interface WebSearchArguments {
  query: string;
  max_results?: number;         // 1–20，省略用 provider 默认（通常 5–10）
  allowed_domains?: string[];   // ≤20 个裸域名，与 blocked_domains 互斥
  blocked_domains?: string[];   // ≤20
  recency?: "hour" | "day" | "week" | "month" | "year";
}
interface WebSearchResult {     // ← 归一化
  results: { url: string; title?: string; snippet?: string; faviconUrl?: string }[];
}
```

归一化会同时吃下 provider 的 `favicon_url` / `faviconUrl` 两种拼写；原始 hit 上的 `published_time` / `source` **归一化时丢弃**。
域名由前端从 URL 派生（去 `www.`）。

#### `web_fetch` — 抓一个页面

**`network`** · **卡片** · 图标 `download` · 「Fetching a page」
**标题** = `url`（从右截断）｜**展开体** 页面正文

```ts
interface WebFetchArguments {
  url: string;
  format?: "markdown" | "html" | "text";   // 默认 markdown（可读结构最好）
}
interface WebFetchResult { content: string; format: string }
```

#### `http_request` — 调接口

**`network`** · **卡片** · 图标 `webhook` · 「Sending \<METHOD\> request」
**标题** = `url`｜**展开体** 状态码 + header 表 + body

```ts
interface HttpRequestArguments {
  url: string;                              // 绝对 http(s)，host 须在配置的 allowlist
  method?: "GET" | "HEAD" | "POST" | "PUT" | "PATCH" | "DELETE";  // 默认 GET，须在 allowlist
  headers?: Record<string, string>;         // 覆盖配置的默认 header
  query?: Record<string, string>;
  body?: string;                            // JSON 要自己编码成字符串并用 headers 设 Content-Type
  timeout_ms?: number;                      // 1–120000
}
interface HttpRequestResult {
  status: number;
  headers?: Record<string, string>;
  body: string;
  truncated?: boolean;                      // 为 true 时必须显式提示
  duration: string;
}
```

---

### 7.4 Skill（4）

#### `list_skills` — 列出可用 Skill

`safe` · **一行** · 图标 `library` · 「Listing Skills」· **无参数**
**标题** = 本地化的「列出 Skills」（无可用参数当标题时的兜底，否则行上会是生的 snake_case）
**展开体** name + description 列表

```ts
type ListSkillsArguments = Record<string, never>;
type ListSkillsResult = string;
// 散文，每条形如 <skill><name>…</name><description>…</description>
```

#### `load_skill` — 加载一个 Skill

`safe` · **一行** · 图标 `book-open` · 「Loading Skill: \<name\>」
**标题** = `name`｜**展开体** 同上

```ts
interface LoadSkillArguments { name: string }
type LoadSkillResult = string;   // 该 Skill 的完整指令
```

#### `read_skill_resource` — 读 Skill 的附件

`safe` · **一行** · 图标 `paperclip` · 「Reading a Skill resource」
**标题** = `name/path`

```ts
interface ReadSkillResourceArguments {
  name: string;   // 指令引用了该资源的那个 Skill
  path: string;   // 相对该 Skill 目录，如 references/REFERENCE.md
}
type ReadSkillResourceResult = string;   // 文件内容。只读内容，永不执行脚本
```

#### `propose_skill` — 提交一个新 Skill

`safe` · **一行** · 图标 `sparkle` · 「Proposing Skill: \<name\>」
**标题** = `name`

```ts
interface ProposeSkillArguments {
  name: string;          // ≤64，^[a-z0-9]+(-[a-z0-9]+)*$。描述工作流，不描述当前任务
  description: string;   // ≤1024，一两句：提供什么可复用流程、何时用
  instructions: string;  // 完整自足指令。排除一次性进度、临时上下文、密钥
  scope: "project" | "user";
}
interface ProposeSkillResult { status: string; name: string; revision: string; scope: string }
```

提案进的是审核队列，工具行只记"提过了" —— **不要在这里画审批 UI**。

---

### 7.5 Plan 模式（3）

三个都 `safe`、三个都**无展开体**（Plan 是常驻状态，见 §5.3）。

#### `enter_plan_mode` — 进入只读计划模式

图标 `map` · 「Entering Plan mode」· **无参数** · result 纯文本
只降权限（封 write / command / network），**不需要审批**；它不创建也不修改 Plan。

#### `set_plan` — 写计划

图标 `list-checks` · 「Updating the Plan」
**标题** = 本地化的「更新计划」｜**副行** = 当前进行中的那一步｜**chips** = `3/7`
**这一行会被丢弃**（结论由 C3 常驻条回答）。

```ts
interface SetPlanArguments {
  steps: {                                            // 每次传完整有序清单，整份替换
    description: string;
    status: "pending" | "in_progress" | "completed";  // 至多一个 in_progress
  }[];                                                // 空数组 = 清空
}
type SetPlanResult = string;
```

真正的 plan 走 `state.snapshot{plan}`，**不靠这个 result**。

#### `exit_plan_mode` — 请求批准计划并退出

图标 `flag` · 「Requesting Plan approval」· **无参数**
它读已存的 session Plan，**不接受 plan 文本或备选方案** —— 所以被批准的值不可能和存的不一致。

**这是一个提问工具**：同时产生 toolCall Item（被抽干）和 question Item（选项 `Approve` / `Reject`，**闭合、不给 allowCustom**）。**工具行被丢弃，提问卡才是真身。**

---

### 7.6 Goal（3）

三个都 `safe`、都**无展开体**、行都被丢弃（结论由 C4 常驻条回答）。

#### `create_goal` — 启动自治目标

图标 `target` · 「Starting an autonomous Goal」

```ts
interface CreateGoalArguments {
  objective: string;
  budget?: {
    max_runs?: number;      // 0 或省略 = 该轴不设限
    max_cost_usd?: number;  // 同上
    max_steps?: number;     // 同上
  };
}
```

#### `get_goal` — 查看目标

图标 `crosshair` · 「Inspecting the autonomous Goal」· **无参数**

`create_goal` 与 `get_goal` 共用出参：

```ts
interface GoalToolResult {   // snake_case，未归一化
  goal: {
    session_id: string;
    objective: string;
    status: string;
    reason?: string;
    provider?: string;
    model?: string;
    budget: { max_runs?: number; max_cost_usd?: number; max_steps?: number };
    usage:  { runs: number; cost_usd: number; steps: number };
    created_at: string;
    updated_at: string;
  } | null;
  message?: string;
}
```

⚠️ 这是**那一刻**的 goal。常驻条**不读它**，读 `goals.get` 的读模型 + `goals.changed` 失效 —— 自治循环会在两次工具调用之间移动 goal。

#### `report_goal_outcome` — 汇报目标结果

图标 `clipboard-check` · 「Reporting a Goal outcome」

```ts
interface ReportGoalOutcomeArguments {
  outcome: "completed" | "blocked";  // completed = 整个目标已达成并验证过
  reason?: string;                   // blocked 必填：具体阻塞点 + 需要什么改变
}
type ReportGoalOutcomeResult = string;
```

---

### 7.7 定时任务（3）

#### `list_schedules` — 列出定时任务

`safe` · **一行** · 图标 `clock` · 「Listing schedules」· **无参数**
**标题** = 本地化的「列出定时任务」｜**展开体** 任务行表

#### `create_schedule` — 新建定时任务

**`write`** · **卡片** · 图标 `calendar-plus` · 「Creating schedule: \<title\>」
**标题** = 本地化词条｜**展开体** 同上（一行 vs 多行只差在拿到几行）

```ts
interface CreateScheduleArguments {
  instructions: string;     // 每次定时运行的完整自足指令
  cron: string;             // 五段：分 时 日 月 周
  title?: string;
  workspace_path?: string;  // 省略用默认工作区
  provider?: string;        // 与 model 成对给或都不给
  model?: string;
}
interface ScheduleView {    // snake_case
  schedule_id: string;
  title?: string;
  instructions: string;
  workspace_path?: string;
  provider?: string;
  model?: string;
  cron: string;
  enabled: boolean;
  last_run_at?: string;
  next_run_at?: string;
  created_at?: string;
}
type ListSchedulesResult   = { schedules: ScheduleView[] };
type CreateScheduleResult  = { schedule: ScheduleView };
```

#### `delete_schedule` — 删除定时任务

**`write`** · **卡片** · 图标 `calendar-x` · 「Deleting a schedule」

```ts
interface DeleteScheduleArguments { schedule_id: string }
type DeleteScheduleResult = { schedule_id: string };
```

**无展开体** —— 这是一张回执，不是一个视图。

---

### 7.8 回忆与发现（4）

四个都 `safe`，结果都是**散文**。

#### `search_memory` — 搜项目记忆

图标 `brain` · 「Searching project memory」· **标题** = `query`
**展开体** 逐条记忆

```ts
interface SearchMemoryArguments { query: string; limit?: number }  // limit 1–20，默认 8
type SearchMemoryResult = string;   // 每条 "N. content"，content 可换行续接
```

#### `search_conversations` — 搜历史会话

图标 `history` · 「Searching earlier conversations」· **标题** = `query`
**展开体** 三列（说话人 / 日期 / 摘录）

```ts
interface SearchConversationsArguments { query: string; limit?: number }  // 1–20，默认 8
type SearchConversationsResult = string;   // 每条 "N. [speaker · YYYY-MM-DD] snippet"
```

#### `search_tools` — 按需加载更多工具

图标 `package-search` · 「Loading additional tools」· **标题** = `query`
**展开体** 按来源分组的工具名

```ts
interface SearchToolsArguments {
  query: string;   // 描述需要的能力；或 select:name1,name2 精确加载；+keyword 表示必含
  limit?: number;  // 1–20，默认 5。select: 查询忽略它
}
type SearchToolsResult = string;   // 散文 + "Not loaded:" 段，每源一行 "  [source] a, b, c"
```

#### `read_tool_result` — 读被省略的工具输出

图标 `archive` · 「Reading omitted tool output」· **标题** = 本地化词条 · **无展开体**

```ts
interface ReadToolResultArguments {
  result_id: string;      // 2–64，^[A-Z2-7]+$。从内联标记里原样抄
  offset_bytes?: number;  // 默认 0
  limit_bytes?: number;   // 1–20000，默认且上限 20000
}
type ReadToolResultResult = string;
```

**这个工具的存在本身就是"上一条工具结果被卸载了"的证据**：超大结果只在 Item 上留有界预览，完整正文单独持久化。**客户端读到的 `result` 就是预览，不必也不该拼回全文。**

---

### 7.9 委派与提问（2）

#### `delegate_task` — 委派给子 agent

`safe` · **一行** · 图标 `users` · 「Delegating: \<summary\>」
**标题** = **`summary`**（3-5 词动作标签，正是为了让父行不必引用整份 brief）
**展开体** 子 agent 的最终答复｜**卡下方** 递归的子 agent 叙事（§4.11）

```ts
interface DelegateTaskArguments {
  summary: string;       // ≤80，3-5 词动作标签，无首尾空白
  instructions: string;  // 完整自足工作指令 —— 子 Agent 看不到父对话
}
type DelegateTaskResult = string;   // 子 agent 的最终答复
```

**唯一会长出第二层内容的工具。** 审批时允许"改参数后批准"。

#### `ask_user` — 向用户提问

`safe` · **一行** · 图标 `question` · 「Waiting for your answer」
**标题** = `questions[0].question` 的第一行｜**展开体** 答案（未答时显示等待）
**这一行会被丢弃**，提问卡（§4.9）才是真身。

```ts
interface AskUserArguments {
  questions: {                       // 1–4 条
    question: string;
    header?: string;                 // ≤12 字符
    options?: { label: string; description?: string }[];   // 2–4；省略 = 自由文本题
    multi_select?: boolean;          // 仅在有 options 时有意义
  }[];
}
type AskUserResult = string;         // {answer} / {response} 或其他字符串字段
```

⚠️ **工具参数是 `multi_select`（模型侧形状），协议上的 `QuestionField` 是 `multiple`（协议形状）** —— 两者由后端翻译，别混用。
它的选项**允许 `allowCustom`**（推荐选项不该封死用户表达），而计划 / 安全决策保持闭合。

---

### 7.10 MCP 与未知工具

```ts
// name = sanitize("<server>_<tool>")：非 [A-Za-z0-9_-] 换 _，超 64 截断
interface UnknownToolCall {
  name: string;
  arguments: Record<string, unknown>;  // 展开体渲染成 JSON 树
  result?: unknown;                    // 同上
}
```

- **卡形**由 `safetyClass` 决定（后端给；缺席按"非只读"处理）· 图标兜底 `tool` · 展开体 JSON 树。
- 审批时允许"改参数后批准"。
- **塌名是有损的**：不能反解 `name` 去关联工具目录；启用/禁用/自动批准的名单用**远端原名**；审批记忆的键是**模型可见名**，塌名的两个工具会共享一条规则。

**接入一个新工具（含第三方 / MCP）要做的全部事**：登记一个图标、（可选）登记一个展开体组件。**协议不动、代码生成不动、契约不升版。**

---

## 8 · 呈现约定

**同一种协议类型在全 app 只能有一种读法。** 具体读成什么样可以重新定，但**必须只有一种** —— 两个面把同一个 token 数写成两种样子，是任何视觉方案都救不回来的廉价感。

下面每条分「**规则**（硬，跟数据语义绑死）」和「**现状**（软，可重定）」。

### 8.1 数字

| 类型 | **规则（硬）** | 现状（软） |
| --- | --- | --- |
| token 计数 | 数字本身随 locale；**单位不翻译**（它要在等宽列里对齐，翻成「万」会改变量级和宽度） | `<1000` 精确；`1.2k` / `1.2M`；整千丢 `.0` |
| 美元 | **不能把非零金额显示成 `$0.00`**（会读作免费） | 常规 2 位；`0<x<0.01` 用 4 位 |
| 时长 | **进位后再选单位**（59.6s 不能显示成 `60s`） | `<10s` 一位小数；`<60s` 整秒；否则 `4m 06s` |
| 步数 / 条数 / 命中数 | 走 i18n 复数规则，**不手拼** | — |
| 比率（`3/7`、预算轴） | 用**裸记号**：各语言同形，**不进翻译目录**（一条没有词的词条只会让格式更难找） | `3/7` |
| 行号 / 行跨度 | **协议全线 1-based**，别减一 | `L40-80` |
| 相关度评分（0–1） | **不要印裸浮点** | 画条或不显示 |

**三条不能破的**

1. **会变化的数字用等宽字形。** 流式里逐 token 跳动的数字宽度，是最容易被一眼看出来的抖动源。
2. **`undefined` ≠ `0`。** 花费缺席 = 该模型不在定价表 → **显示 token 但不许编一个价格**；`added`/`removed` 双缺席 → 什么都不画；`exitCode` 缺席（命令转后台）→ 不画。
3. **预算语境里 `0` = 不设限**（协议在为零时省略该字段）→ **不设限的轴不能画成一条满的进度条**，那读起来正好是反义。

### 8.2 时间

协议全部是 **ISO-8601 字符串**。

| 位置 | 读法 |
| --- | --- |
| D2 turn caption | 时钟 |
| D1 日期变化处 | 日期分隔条，**只在变化处画一次** |
| 列表行（会话 / 定时任务 / 记忆） | 相对时间 |
| 导出 / 诊断 | 完整戳 |

- **fold 层永不预格式化时间**。曾经存过第二份格式化副本，结果是语言切换到不了已经在屏的消息，而且那份副本是硬编码英文。
- **`createdAt` 可以缺席**（合成的助手 turn 没有自己的 Item）。**不许去摸墙钟补它** —— 客户端盖的戳坐在服务端盖戳的流里，机器时间偏移时它上面的日期分隔条会和旁边的消息互相矛盾。
- 缺席的时间戳**既不开新日也不断链**。

### 8.3 字符串与截断

| 类型 | 截断方向 | 理由 |
| --- | --- | --- |
| 路径 | **从左**（保住文件名） | `ToolCall.fnKind === "path"` 是判据 |
| URL | 从右 | 域名在前面 |
| 散文 / 描述 / 副行 | 从右 | |
| 命令行、代码、diff 行 | **不截断**，横向滚动或换行 | 截掉的那半正是要核对的东西 |
| 掩码过的密钥 | 原样 | **永不解掩码、永不回显、永不据此推原文** |

1. **一个面只用一种截断策略。**
2. **尺寸不随内容长度跳动** —— 长文本靠截断 + 兜底，**绝不靠换行撑高度**。

### 8.4 枚举

**所有闭合枚举一律查表，不用 `` `prefix.${value}` `` 模板。**

判据：这个枚举背后有没有运行时数组？

- **没有**（`GoalReasonCode` / `RunOutcome.type` / `FileStatus` / `PlanStatus` / MCP 连接状态 …）→ **必须查表**。模板会在协议长出第 N+1 个值的那天，在每一种语言里静默渲染一个生 key；查表编译不过。
- **有**（由后端播报的数组）→ 可以遍历，但落到文字仍走词条。

### 8.4.1 一个取舍：颜色留给状态还是身份

**现状的答案是「只表达状态」** —— 中性 / 注意 / 失败 留给 running / failed / refused，工具身份、文件状态、作用域一律用**字形**和**位置**区分。

理由值得先看一眼再决定要不要改：三十种工具身份如果去抢调色板，一个**失败的读**和一个**成功的读**就会显得一样刺眼，而"这次跑挂了"恰恰是读者扫 transcript 时唯一真正要找的东西。

新设计如果想让颜色也承担身份，就得先回答：**状态还剩什么手段可用。**

### 8.5 空态与缺席

每个吃可选字段的面必须回答三个问题：

1. **还没有**（running / pending / indexing）→ 骨架或进行中指示，**不是空盒**。
2. **有但是空**（`hits: []` / `changes: []` / `steps: []`）→ 一句"没有结果"，**不是留白**。
3. **不可用**（feature 关闭 / 工作区失联 / 该 runtime 没实现这个方法）→ **平静的"此运行时不提供"**，不是错误红。

### 8.6 可靠性对渲染的三条约束

- **重连后 `contextTokens` 是"没有值"不是"旧值"** → 占用条要能画「未知」。
- **历史加载只有 completed、零 delta** → 所有流式视觉（打字机、live chip、增量高亮）必须优雅缺席，而不是留一个半成品。
- **客户端可以主动排除高频事件并仍然正确** → 低配模式下 UI 不能崩。

---

## 9 · 全量参数字典（其余 surface）

内容区之外、但同一份协议数据喂养的面。

### 9.1 会话与工作区（A 侧栏 / B 顶栏）

```ts
interface Session {
  id: string;
  title: string;                    // A 侧栏行 + 窗口标题；一行，从右截断
  model: string;                    // B 顶栏模型槽；显示时优先用 Model.displayName
  status: "running" | "waiting" | "idle";  // 派生字段 → B 状态点
  revision: number;                 // 乐观并发凭证，不上屏；冲突要有可见反馈
  favorite?: boolean;               // A 侧栏星标
  createdAt: string;
  updatedAt: string;                // A 侧栏相对时间
  workspace: WorkspaceInfo;
}

interface WorkspaceInfo {
  ref: { path: string };            // B 顶栏面包屑，从左截断
  projectRoot?: string;
  availability: "available" | "missing";   // missing → C1 常驻条
}

interface WorkspaceSummary {        // 派生读模型，不是可写聚合
  name: string;
  workspace: WorkspaceInfo;
  sessionCount: number;
  lastActiveAt?: string;
}
```

- **`status` 是派生的**（有 running run → running；有 waiting run → waiting；否则 idle），不是一个可以单独被写坏的字段。
- **`revision` 是条件写的唯一凭证**：更新必带 `expectedRevision`，过期返回 `revision_conflict`（重拉聚合再合并）。**没有"最后写赢"的路径。**

### 9.2 用量面板（H 设置）

```ts
interface UsageSummary {
  total: ModelUsage;
  runs?: number;
  sessions?: number;
  byDay?: UsageBucket[];       // 折线 / 柱状
  byModel?: UsageBucket[];     // 分组条
  byProvider?: UsageBucket[];
}
interface UsageBucket extends ModelUsage { key: string; runs?: number }
// 请求：{ sinceDays?: number }
```

### 9.3 模型与 Provider（F 模型选择器 / H 设置）

```ts
interface Model {
  id: string;
  provider: string;              // per-run 必须与 model 显式配对，provider 不从 model 名推断
  displayName?: string;          // 优先显示；缺席用 id
  contextWindow?: number;        // 配 RunProgress.contextTokens 画占用条；缺席则不画
  maxInputTokens?: number;
  maxOutputTokens?: number;
  knowledgeCutoff?: string;
  deprecated?: boolean;          // 弃用标记，不隐藏（用户可能还在用）
  pricing?: {
    inputUsdPerMillionTokens?: number;
    outputUsdPerMillionTokens?: number;
    cacheReadUsdPerMillionTokens?: number;
    cacheWriteUsdPerMillionTokens?: number;
  };                             // 全缺 = 无定价 → costUsd 也会缺
  capabilities?: {
    toolUse?: boolean; reasoning?: boolean; structuredOutput?: boolean; multimodal?: boolean;
    inputModalities?: Modality[]; outputModalities?: Modality[];
    reasoningLevels?: string[]; reasoningDefaultLevel?: string;
  };                             // → 能力 chip
}
type Modality = "text" | "image" | "audio" | "video" | "pdf";

interface Provider {
  id: string;
  apiKeyMasked: string;          // 永不可逆推
  baseUrl?: string;
  requiresBaseUrl?: boolean;
  keySource?: "stored" | "env";  // env 来源的不该给"清除"按钮
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}
interface ProviderTestResult { ok: boolean; error?: ProblemData }   // inline 判决，不是失败的调用
```

### 9.4 文件与 diff（G Context Dock）

```ts
/** sum-type：按 format 二选一，不是同时带两者的松对象 */
type Diff =
  | { files: FileDiff[]; truncated?: boolean }   // format="rows"
  | { patch: string;     truncated?: boolean };  // format="raw"
// 请求：{ workspace; mode?: "worktree" | "base"; path?; format?: "rows" | "raw"; limit? }

interface FileDiff {
  path: string;
  status: FileStatus;
  rows: DiffRow[];
  added?: number;          // binary 时省略，不伪造 0
  removed?: number;
  binary?: boolean;
  previousPath?: string;   // 仅 renamed
}

type DiffRow =
  | { type: "hunk";    text: string }                                  // 分隔行，无行号
  | { type: "context"; code: string; leftLine: number; rightLine: number }
  | { type: "added";   code: string; rightLine: number }               // "+"
  | { type: "deleted"; code: string; leftLine: number };               // "−" U+2212，与 + 等宽

type FileStatus = "added" | "modified" | "deleted" | "renamed" | "untracked";

interface WorkspaceFileChange {   // VCS 工作区扫描态
  path: string; status: FileStatus;
  added?: number; removed?: number; binary?: boolean; previousPath?: string;
}

interface FileContent {
  path: string; content: string; encoding: string;
  totalLines: number; startLine?: number; endLine?: number; truncated?: boolean;
}
interface FileHead { path: string; lines: { lineNumber: number; text: string }[] }
interface FileEntry {
  name: string; path: string;
  type: "file" | "dir" | "symlink";
  sizeBytes?: number; modifiedAt: string;
}
interface GrepResult {
  matches: { path: string; lineNumber: number; text: string }[];
  total: number;    // 可以大于 matches.length → 要显示"还有 N 条"
}
```

> **三个文件类型共用 `FileStatus` 词汇但意图不同、故各自独立**：`WorkspaceFileChange`（扫描态，有 `untracked`）/ `FileDiff`（逐文件结构化 diff，带 `rows`）/ `apply_patch` 的 `changes`（用 `ChangeStatus`，有 `moved` 无 `untracked`）。**别合并、别互相 cast。**
>
> `FileLine.text` / `DiffRow.code` / `GrepMatch.text` 是**纯文本，不含服务端 HTML** —— **高亮全部由客户端做。**

### 9.5 Skills（G 区 Skill 视图）

```ts
interface Skill        { name: string; description?: string; scope: "project" | "user" }
interface ManagedSkill { name: string; description?: string; lifecycle: "active" | "archived" }
interface SkillProposal {
  name: string;
  description: string;
  instructions: string;              // 可能很长 → 需要滚动区，不是截断
  scope: "project" | "user";
  revision: string;
  revises?: boolean;                 // 是否修订已有 Skill
  origin?: "requested" | "mined";    // 来源决定这条要不要更谨慎地审
  sourceSession?: string;            // → 可跳回来源会话
}
```

### 9.6 Agent 记忆（G 区记忆视图）

```ts
interface AgentMemoryItem {
  id: string;
  content: string;                       // 正文，可多行
  scope: "project" | "user";
  status: "active" | "pending";          // pending 的记忆在审前不会被注入 —— 设计如此，UI 要说清
  origin: "auto" | "user";               // auto = 挖掘出来的 → 需要审
  pinned: boolean;                       // 置顶核心记忆
  sessionId?: string;                    // 来源会话
  day?: string;
  createdAt: string;
  updatedAt: string;
}
```

### 9.7 知识文件（G 区）

```ts
interface MemoryEntry {                        // 用户可编辑的 SCOPEAPP.md
  scope: "cwd" | "projectRoot" | "home";
  path: string; content: string; updatedAt?: string;
}
interface AgentDoc { path: string; scope: "cwd" | "projectRoot" | "home"; title?: string }
```

### 9.8 MCP（H 设置）

```ts
interface McpServer {
  name: string;
  description?: string;
  connection: McpConnection;
  status: McpServerState;
  timeoutSeconds?: number;
  disabledTools?: string[];      // 用远端原名，不受塌名影响
  autoApproveTools?: string[];   // 同上
}

/** 闭合的安全读联合：另一 transport 的字段不可出现，secret 原文永不通过读 API 返回 */
type McpConnection =
  | { type: "stdio"; command: string; args?: string[]; dir?: string;
      envMasked?: Record<string, string> }
  | { type: "streamableHttp"; url: string; authorizationMasked?: string;
      headersMasked?: Record<string, string> };

/** 六态，各画各的 —— disabled 与 disconnected 不再靠字段缺席或客户端猜测区分 */
type McpServerState =
  | { type: "disabled" }                    // 持久化开关关闭
  | { type: "disconnected" }                // 已启用但当前没有连接
  | { type: "connecting" }
  | { type: "connected"; toolCount: number }
  | { type: "failed";    error: ProblemData }
  | { type: "needsAuth"; error: ProblemData };

interface McpTool { server: string; name: string; description?: string;
                    inputSchema?: Record<string, unknown> }  // name = 远端原名

interface McpAuthorizationAttempt {
  id: string; server: string; createdAt: string; finishedAt?: string;
  status: { type: "pending" } | { type: "succeeded" }
        | { type: "failed"; error: ProblemData } | { type: "canceled" };
}

/** 写入：省略 = 保留（仅同 secret scope 内）；scope 变了必须显式 set 或 clear */
type SecretChange = { type: "set"; value: string } | { type: "clear" };
```

- **终态保留窗口由服务端能力公布**（pending 不按该窗口清理）→ 过期后的查询要引导「重新发起登录」，而不是「重试这个 id」。
- UI 换 URL origin / 换进程目标时**必须**逼用户对凭证表态 —— 运行时绝不把凭证静默带到新 origin。

### 9.9 定时任务（H 设置）

```ts
interface Schedule {          // 协议形状，与 §7.7 工具出参的 snake_case ScheduleView 不是一回事
  id: string; title: string; cron: string; prompt: string;
  enabled: boolean;
  revision: number;           // 条件写凭证
  workspace?: { path: string };
  provider?: string; model?: string;
  createdAt: string; lastRunAt?: string; nextRunAt?: string;
}
// schedules.runNow → { runId, sessionId }（→ 直接跳过去）
```

### 9.10 Recipes 与 Hooks（H 设置）

```ts
interface Recipe {
  name: string; description?: string; body: string;
  argumentHint?: string;
  scope: "project" | "global";
  source: string;             // 文件路径，从左截断
}

interface HooksListResult {
  hooks: HookInfo[];
  projectRoot?: string;
  projectTrusted: boolean;    // 未信任的项目 hook 不执行 → 要有显式信任开关
}
interface HookInfo {
  event: "PreToolUse" | "PostToolUse" | "UserPromptSubmit" | "SessionStart"
       | "SubagentStart" | "SubagentStop" | "PreCompact" | "Stop" | "Notification";
  scope: "global" | "project";
  source: string; active: boolean;
  command?: string; inject?: string; matcher?: string; timeoutMs?: number;
}
```

### 9.11 语义索引（G 区）

```ts
interface CodebaseStatus {
  state: "none" | "indexing" | "ready" | "error";
  fileCount: number; chunkCount: number;
  indexedAt?: string; modelId?: string; operationId?: string; truncated?: boolean;
}
interface CodebaseHit {
  path: string; startLine: number; endLine: number; snippet: string;
  score: number;   // 0–1，不要印裸浮点
}
```

### 9.12 审批策略（H 设置）

```ts
type ApprovalMode = "safe" | "balanced" | "yolo";   // 全局姿态，不是 per-run
interface ApprovalRule {
  id: string;
  tool: string;                                     // 模型可见名
  decision: "allow" | "deny";
  scope: "session" | "project" | "global";
  subject?: string;
  dir?: string;
}
```

### 9.13 工具目录（G 区）

```ts
interface ToolSpec {
  name: string; description?: string;
  parameters?: Record<string, unknown>;   // JSON Schema
  safetyClass?: SafetyClass;
}
```

⚠️ 这是**直接诊断**目录 —— **不是运行入口的覆盖点，也不是 Agent 的完整工具集**（一个 Run 的工具集由运行时按会话 / 审批策略 / Skills / MCP 统一装配）。面板文案不要暗示它可配置。

### 9.14 能力发现（决定什么该出现在界面上）

```ts
interface DiscoverResponse {
  protocol: { current: string; minSupported: string };
  serverInfo: { name: string; version: string; home: string; defaultWorkspace: { path: string } };
  capabilities: ServerCapabilities;
}

interface ServerCapabilities {
  features: Record<string, {
    enabled: boolean;
    clientOptIn: boolean;              // 需要客户端在请求里声明
    requiredByRunProtocol: boolean;
    stability: "stable" | "experimental";   // experimental → 可以打标
  }>;
  runEvents: string[];
  runtimeTopics: RuntimeTopic[];
  stateSnapshots: { key: string; scope: "session" | "run";
                    writer: "rootRun" | "anyRun"; recoveryMethod: string }[];
  streamingMethods: string[];
  limits: {
    maxConcurrentRuns?: number;
    runReplay: { maxEvents: number; maxBytes: number; scope: "processRootSegment" };
    runtimeSubscription: { maxTopics: number; maxWatches: number };
    idempotency: { retentionSeconds: number };
    mcpAuthorizationAttempts: { retentionSeconds: number };
  };
}
```

**19 个 feature**：`reasoning` `multimodal` `compaction` `plan` `goals` `agentMemory` `knowledge` `skills` `mcp` `schedules` `codebase` `git` `checkpoints` `fileWatch` `lsp` `sessionExport` `relocate` `subagents` `clientTools`。
**关闭的域整块不渲染** —— 不是渲染出来再报错。

### 9.15 后台变更通知：哪个话题让哪块 UI 失效

```ts
type RuntimeEvent =
  | { type: "files.changed";      sequence: number; paths: string[];
      watchId?: string; workspace?: { path: string } }
  | { type: "skills.changed";     sequence: number; names?: string[] }
  | { type: "mcp.changed";        sequence: number; serverIds?: string[] }
  | { type: "schedules.changed";  sequence: number; scheduleIds?: string[] }
  | { type: "sessions.changed";   sequence: number; sessionIds?: string[] }
  | { type: "runs.changed";       sequence: number; runIds?: string[]; sessionIds?: string[] }
  | { type: "state.changed";      sequence: number; key: "plan";
      runIds?: string[]; sessionIds?: string[] }
  | { type: "goals.changed";      sequence: number; sessionIds?: string[] }
  | { type: "interrupts.changed"; sequence: number; runIds?: string[]; sessionIds?: string[] }
  | { type: "resync";             sequence: number; topics: RuntimeTopic[]; watchIds?: string[] };
```

| 话题 | 失效 |
| --- | --- |
| `files.changed` | G 区文件树 / diff / 评审工作台 |
| `skills.changed` | G 区 Skill 列表与草稿 |
| `mcp.changed` | H 设置 MCP 面板 |
| `schedules.changed` | H 设置定时任务面板 |
| `sessions.changed` | A 侧栏 |
| `runs.changed` | A 侧栏状态点 / G 区 run 列表 |
| `state.changed{plan}` | **C3 Plan 常驻条** |
| `goals.changed` | **C4 Goal 常驻条** |
| `interrupts.changed` | G 区待办 |
| `resync` | **全量重拉列出的话题** |

### 9.16 回滚与导出

```ts
interface RollbackSessionResponse {
  session: Session;
  droppedRuns: { run: RunSummary; userInput?: ContentBlock[] }[];
}
type RestoreType = "history" | "files" | "both";   // 必须让用户明确知道会不会动磁盘

interface ExportSessionResponse {
  format: "md" | "json";
  markdown?: string;
  artifact?: SessionArtifact;
}
interface SessionArtifact {
  version: number;
  session: unknown; runs: unknown[]; items: unknown[]; states?: unknown[]; messages: unknown[];
  toolResults: {
    id: string; itemId: string; toolName: string;
    preview: string;
    body: string;      // 归档里才有完整正文 —— 在线时 Item 上只有有界预览
    createdAt: string;
  }[];
}
```

**`droppedRuns[].userInput` 是给"把这条重新填回输入框"用的** —— 回滚不能让用户丢掉自己打的字。

⚠️ 归档里的错误类型是一套**更小的 11 值 camelCase 枚举**（`internalError` `runLost` `agentStuck` `rateLimited` `invalidApiKey` `timeout` `providerUnavailable` `providerRejected` `deniedByUser` `toolFailed` `childRunCanceled`），**与在线的 snake_case `ProblemData.type` 不是同一张表** —— 别复用同一个查表函数。

### 9.17 分页

```ts
interface Page<T> { data: T[]; nextCursor?: string }
```

两种**诚实**能力：

- **有界集合**：不接受 `cursor/limit`，一次给完，**不产生 `nextCursor`**。
- **游标集合**（会话列表 / run 列表 / Item 列表 / 待办列表 / 文件列表 / 定时任务列表）：接受 `cursor/limit`，**`nextCursor` 的存在性就是"还有更多"**。

打磨含义：

- **服务端不得静默截断**。有界集合超界会**失败并要求缩小查询** → 要有对应的 UI（不是空列表）。
- **游标不透明**：不解析、不据其推断顺序，只回传。
- **游标绑定完整查询**：换了筛选 / 排序就是新游标 —— 切换筛选器必须重置分页状态。
- Item 列表的响应**额外带 `runs: RunSummary[]`**（页级附加数据），逐页取时不要丢。

---

## 10 · 已知漂移

对照契约登记与运行时工具表核出来的**文档/代码与真值源不一致**处，**均未修**。

| # | 位置 | 现状 | 真值 |
| --- | --- | --- | --- |
| D1 | `docs/protocol/API.md` §4.3 | 称 Item 有**七个**变体，含 `plan` | 只有**六个**（§2.2）。plan 是**共享状态**不是 Item |
| D2 | `docs/protocol/API.md` §5.1 / §5.2 | 称增量有**五种**，含 `plan` | 只有**四种**（§2.3） |
| D3 | `docs/protocol/API.md` §5.3 / 附录 C.4 | 共享状态 key 是 `todos`，冷读方法 `todos.get` | 契约登记：key = **`plan`**，冷读 = **`plan.get`** |
| D4 | `docs/protocol/API.md` §4.4.2 | 列了 `edit` / `write` 两个工具，参数写 `file_path` | 运行时**没有** `edit`/`write`，文件变更只有 **`apply_patch`**（参数 `patch`）。所有文件工具的路径参数是 **`path`** |
| D5 | 前端 diff 展开体注册 | 注册了 `["edit","write","apply_patch"]` | 前两个是**永不命中的死键** |
| D6 | 前端工具分类表 + 图标表 | 分类表含 `edit`/`write`；图标表 32 条（多出 `write`/`edit`），注释也写作 "32 tools" | 内置工具只有 **30** 个。删掉后图标表恰好 30 条，"一工具一字形"的守恒才是真的 |
| D7 | 前端 `writtenHead()` | 从 `arguments.content` 取写入内容的头几行 | **没有工具接受 `content` 参数** → `ToolCall.written` 恒空，diff 展开体的第三级回落是死路 |
| D8 | 前端 `editLineCounts()` | 期待 `result.changes[].diff` | `apply_patch` 的 `changes[]` **只有** `{path, status, from?}`，**没有 diff 行** → `ToolCall.diff`/`added`/`removed` 实践中恒空，diff 展开体总是走全工作区回落。（`FileEdit` 类型还在生成的 wire 里，但 schema 已经没有它了） |
| D9 | 视图模型 approval 块 | 声明了 `scope` / `target` / `reversible` | fold 从不设置它们（协议的审批载荷只有 `tool`/`reason`/`risk`/`rememberable`）→ 推测性占位 |
| D10 | 前端错误文案表 + 八份 locale | 为 `no_language_server` / `is_a_directory` / `file_too_large` 备了文案 | 这三个**不在协议的 `ProblemData` 联合里**（历史遗留）→ 24 条（3 × 8 语言）不可达文案 |

**处理原则**：D1–D3 是文档滞后于契约，改文档；D4 同时是文档滞后 + 生成的 wire 里有类型残留，需要一起核；D5–D10 是前端死码 / 死路，属于"易补回来"的推测性冗余，应当删而不是留着。**都涉及公开形状，动手前先算爆炸半径再改。**

---

## 11 · 打磨自查

### 11.1 每个面走一遍

**这十条与视觉方案无关** —— 换一套设计语言之后，它们还是同样的十条，因为它们问的全是"数据的真实形态被如实表达了吗"。

1. **会变的数字用等宽字形了吗？** 流式里跳动的数字宽度是第一眼就能看出来的廉价感。
2. **`undefined` 和 `0` 分开画了吗？** 尤其花费 / 退出码 / diffstat / 预算三轴。
3. **三种空态都有吗？**（还没有 / 有但为空 / 不可用）
4. **路径是从左截断的吗？** 丢了文件名的路径等于没有路径。
5. **枚举是查表不是模板吗？** 协议长一个值就在每种语言里露生 key 的地方，编译期挡不住。
6. **尺寸随内容跳吗？** 长标题、长错误、长目标、长指令 —— 每个都试一遍最长值。
7. **窄面板下呢？** 阅读列会被左右两侧挤窄，断点必须是容器查询不是视口查询。
8. **历史加载态对吗？** 只有 completed、零 delta 的那条路径。
9. **重连态对吗？** `contextTokens` 没有值、`progress` 为 null、`activeSegmentId` 换了。
10. **feature 关闭时整块消失了吗？** 不是渲染出来再报错。

### 11.2 加一个新渲染物之前

1. **它是协议的哪个缝？** 只能选一个 —— **Item**（要进历史、要能回放的事实）/ **共享状态**（"当前是什么"的整份值，要登记作用域 + 写者 + 冷读方法）/ **custom**（只改善实时体验，**丢了不影响正确性**）。选错的代价：拿 `custom` 承载事实 = 重连后永久丢失。
2. **参数从哪一层取？** 能在 fold 里一次算出来的，不许留到 render 期算（render 期 parse = 每个 token 一次）。
3. **有没有权威落点？** 每个预览通道**必须**在权威投影上有命名终值。没有 = 设计还没想清楚。
4. **文案是词条 key 吗？** fold 层出现英文句子 = 缺陷。
5. **它是「做」还是「答」？** 「做」（思考 / 工具）会被折叠波收走；**要读者做决定的东西永远不折**。
6. **组件里出现 `name === "xxx"` 了吗？** 是就改成登记一个图标 / 展开体。
7. **React key 是身份吗？** 持有本地草稿的卡（审批 / 提问）用下标 key 会把上一个的草稿泄漏给下一个。
