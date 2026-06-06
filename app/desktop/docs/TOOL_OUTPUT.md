# 工具输出的持久化（Durable Tool Output）

> 适用范围：`commandExecution` 的 stdout/stderr，以及任何"输出在 run 期间流式产生"的工具。
> 关联：`docs/API.md` §4.4（ToolInvocation）/ §5.1（ItemDelta）/ §5.2（Durable/Ephemeral 不变量）/ §10（历史·重连·恢复）。

---

## 1. 一句话

**工具输出的权威终值在 `item.completed` 的结构化字段上（必发）；`item.delta{toolOutput}` 只是它的流式预览（可丢）。**
客户端渲染输出只认 completed 的字段（`commandExecution.output` / 通用 `tool.result`），**绝不**把 delta 累积当作终态来源。

---

## 2. 问题：旧设计违反了协议自己的不变量

§5.2 是协议**硬保证**：

> 丢弃每一个 `durable=false` 事件，客户端仍必然得到正确终态。
> 推论：每个 item 的最终态必然出现在后续 `item.completed`。

旧 §4.4 却让 `commandExecution` 的 stdout **只**走 `item.delta{toolOutput}`（`durable=false`），completed 上"无独立 output 字段"。两者直接打架——只要 toolOutput delta 不在场，completed 上没有任何输出，终态就是错的。

delta 不在场是**日常**，不是边角：

1. **历史回放**：`items.list` 只返回 durable 的 item 终态，**没有任何 delta**。重开会话 → 所有命令输出为空。
2. **重连**：断线期间的 `durable=false` 帧不重放（§10.1）。重连后那段输出永久丢失。
3. **opt-out**：客户端可 `optOutNotificationMethods` 关掉高频 delta（§9）仍要求正确——关掉就没输出。
4. **后端整发不流式**：有的 runtime（或快命令）直接在 completed 给完整结果，**根本不发 delta**。

症状：命令卡片展开是空的（`(no output)`），但模型明明拿到了输出、并据此作答——因为输出确实产生了，只是从未以**可持久化**的形式发给客户端。

> 对照：其它工具早就是对的——`search.results`、`fileChange.diff`、通用 `tool.result` 都在 completed 上权威落定。
> 唯独 `commandExecution` 的 stdout 是个例外，是它单独破坏了 §5.2。

---

## 3. 修复：`output` 是 completed 上的硬契约

```ts
// API.md §4.4
| { kind: "commandExecution"; command: string[]; cwd?: string;
    exitCode?: number; durationMs?: number; output?: string; outputTruncated?: boolean }
```

- `output` / `exitCode` / `durationMs` 是**结算字段**：
  - **`item.completed` 的 toolCall 上必发**（durable，权威终态）。`output` 是合并后的 stdout+stderr 全文，命令无输出则为 `""`。
  - **`item.started` 壳上不存在** —— 这是**生命周期**（结算前还没有值），不是"契约可选"。type 上写 `?` 仅因 `ToolInvocation`
    一个形状被 started/completed 共用（与 `exitCode?` 同理）。
- `item.delta{toolOutput, text}` 是 `output` 的**流式预览**（`durable=false`，可丢），逐片预览其最终值。
- `outputTruncated`：runtime 因体量上限截断 `output` 时置 `true`，并把 delta 流截到同一边界（预览与终态一致）。

> **不留兼容退路**（本项目无历史包袱）：不存在"老后端不发 output 时前端兜底用 delta"这种降级路径——那是把非合规后端当一等场景。
> completed 的 commandExecution **一定**带 `output`；不带就是后端 bug，前端如实显示空，不替它打补丁。

### 为什么不选其它方案

| 方案 | 否决理由 |
| --- | --- |
| 把 `toolOutput` delta 改成 `durable=true` | delta 是逐 token 高频流，标 durable 会让 `items.list` / 重放被 stdout 撑爆，且摧毁 durable/ephemeral 分层。 |
| 新增一个 durable 的 "toolOutput 快照" 事件 | 平白多一种事件类型；completed item 本就是「权威终态」的既有落点，直接放字段最小、最一致。 |
| stdout / stderr 拆成两字段 | 丢失两者的实时交错顺序（终端体验里恰恰要交错）。合并单字段与 toolOutput delta 的单一文本流也天然一致。 |

权威字段落在 completed，与 `toolArguments` delta → completed 已解析入参、`search.results` 等是**同一套收敛模式**：流式给预览，completed 给真相。

---

## 4. 客户端（前端）如何消费

权威值在 completed，预览值在 delta。fold 规则（`core-reducer/projections.ts` + `fold.ts`）：

```
inProgress（started 壳 / 流式中）：
  result ← 累积的 toolOutput delta 文本           // 实时预览，体验用
completed：
  result ← tool.output（权威），outputTruncated ← tool.outputTruncated
```

实现要点：`toolFields(commandExecution)` 在 `output` 在场（completed）时写 `result` + `outputTruncated`；started 壳无 `output`，**不写**这两个键——让 `fold.ts` 里 `result: prev?.result` 的预览基线保留到 completed 来 reconcile（与 `args` 仅在 completed 从结构化对象 reconcile 完全一致）。

收敛结果（三条路径渲染一致，这是 reducer 的核心不变量）：

- **实时流式**：delta 预览 → completed 的 `output` 接管。
- **历史回放**（`items.list`，仅 completed、无 delta）：直接拿 `output`。✅
- **后端整发不流式**（无 delta）：直接拿 `output`。✅

渲染只认 `tool.result`（`ToolCard` → `BashPreview` / `ToolInspector`），三条路径自动统一。`outputTruncated` 时 `BashPreview` 追加"output truncated by runtime · Open in Terminal"。

---

## 5. 对后端（runtime）的契约

- 命令执行结束时，**必须**在 `item.completed` 的 `tool.output` 给出合并后的完整 stdout+stderr（无输出给 `""`）。
- `item.delta{toolOutput}` **可选**（纯体验增强）：发就是逐片预览，不发也合规（completed 的 `output` 是唯一真相源）。
- `output` 超限时截断并置 `outputTruncated:true`；delta 与 `output` 截到**同一边界**，保证预览与终态一致。
- 其它工具早已合规，无需改动。

---

## 6. 影响面（本次改动）

- `docs/API.md`：§4.4 `commandExecution` 加 `output` / `outputTruncated`，明确「结算字段 = completed 必发」；§5.1 补「toolArguments/toolOutput 皆预览通道」。
- `frontend/src/rpc/shapes.ts`：wire 类型加 `output` / `outputTruncated`；`toolOutput` ItemDelta 注释标为预览。
- `frontend/src/protocol/run/viewState.ts`：视图 `ToolCall` 加 `outputTruncated`。
- `frontend/src/plugins/builtin/agent/core-reducer/projections.ts`：`toolFields` 在 completed 把 `output`→`result`、`outputTruncated` 投影。
- `frontend/src/plugins/builtin/chat/tools/previews/index.tsx`：`BashPreview` 读 `result`、显示截断提示。
- 回归测试：`reducer.tooloutput.test.ts`（回放 / 权威覆盖预览 / 截断）+ `reducer.hitl-resume.test.ts`（HITL resume 重发后仍保留 output）。
