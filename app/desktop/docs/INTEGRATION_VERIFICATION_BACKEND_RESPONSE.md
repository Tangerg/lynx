# 后端 → 前端回复（本轮 e2e · 工具调用契约）

> **日期**：2026-06-05
> **后端**：Lyra Runtime `c5a03e8`（已 push + 重启，运行中 `http://127.0.0.1:17171`）。
> 对应前端 [`INTEGRATION_VERIFICATION.md`](./INTEGRATION_VERIFICATION.md)（本轮 e2e + §2 工具契约定稿）。
> 本文：① E1–E5 处置；② §2 ToolInvocation 新契约的实施计划（分阶段，附约束说明）。

---

## 1. 本轮 e2e 坑（E1–E5）

### E1 — 工具失败升级成致命 run error ✅ 已修

**根因**：`core/model/chat` 的工具循环（`toolCallInvoker.invokeToolCalls`）把**单个工具的执行失败当致命错误向上抛**，于是整条 run 以 `internal_error` 收场——违反 API.md §8.1。

**修法**：`core/model/chat` 新增 `FeedbackOnToolError`（`FeedbackOnUnknownTool` 的姊妹项）。开启后，工具 `Call` 报错时，把错误**作为该工具的结果回灌给模型**（"error: tool \"glob\" failed: …"），模型据此换方案、**run 继续**；不再升级成 `run.finished{error}`。lyra 的工具循环已开启它。

- toolCall item 仍按你看到的那样落 `status:"incomplete"` + `error.type:"tool_failed"`（经 tool observer，与回灌**并行发生**，二者都有）。
- **控制流错误仍照常传播**（不被回灌）：context 取消，以及实现 `ToolLoopAbort() bool` 的错误——HITL 的 `PauseError` 已实现它，所以审批挂起/取消仍能正确 park（这条被审批门测试守住，差点漏了）。
- 只有真正的引擎/基础设施故障才仍配 `internal_error` + 终止 run。

> 请你方按这个再跑一次 E1 探针（plan + 触发文件工具的 prompt）：预期 `glob` 失败 = toolCall item `incomplete`/`tool_failed`，run **继续**（模型看到错误后自行调整），不再 `run.finished{error}`。

### E2 — `internal_error.detail` 泄漏内部调用栈 ✅ 已修

`internal_error` 的 wire `detail` 现收敛为干净文案（`"the run failed due to an internal error"`）；**完整错误（含 Go wrap 上下文）只进服务端 trace span，不上 wire**（API.md §8.2）。E1 修好后，工具失败不再走这条，`internal_error` 现在只代表真·引擎故障。

> **遗留/建议**：目前 provider 类错误（如 401、限流）也被归到 `internal_error`，genericize 后会丢掉有用信息。**正确做法是把 provider 错误分类成 `provider_error` 单独的 type + 干净 detail**（API.md §8.2 已有该码 `-32001`）。这块我放到下一批一并做（与 §2 工具契约同期），现在先保证不泄漏栈。

### E3 — plan 模式执行工具（语义确认）→ 维持现状

经确认：**plan 模式不特殊门控工具**，只是先给计划预览；工具策略与普通 turn 一致。你方观察到的"plan 模式发 glob"是预期行为。原本的隐患（一次只读工具失败炸 run）**已由 E1 兜掉**——plan 模式下工具失败同样不再终结对话。若后续要做"plan 模式只读门控/写操作走审批"，另开 issue。

### E4 — `/v2/info` `capabilities.providers` 为空 ✅ 已修

`capabilities.providers` 现返回**本 build 支持的 provider id 集合**：`["anthropic","deepseek","moonshot","openai"]`（实测）。语义定为"runtime 支持哪些 provider 类型"（稳定、非空）；**每个 provider 是否已配 key 仍看 `providers.list` 的 `apiKeyMasked`**——两者职责不同，不再矛盾。

### E5 — JSON-RPC `id` 必须 string（nit）→ 已写文档

按约定**保持 string-only**（不放宽）。已在 `TRANSPORT.md §2` 显式写明硬约束：envelope `id` 必须为 string，整数 id 拒为 `invalid_request`，`null` 仅用于无法解析时的错误响应。你方 client 一直发字符串、不受影响；此条是给其他客户端的明示。

---

## 2. §2 ToolInvocation 新契约 — 已接受，分阶段实施

新的五变体判别联合（`commandExecution` / `fileChange` / `search` / `webSearch` / `tool`）+ 去 kind↔name 双身份 + `arguments` 恒对象 / `result` best-effort JSON / 绝不双重编码 + HITL `Interrupt` 判别联合（approval/toolResult 复用 `ToolInvocation`）+ `ToolResultResponse` `output`→`result+error` —— **全部接受**，按你建议的节奏分阶段落地。

**一个实现约束（影响分期）**：当前后端 engine 的工具观测层（`ToolObserver`）只透出**字符串**（`OnToolCallStart(…, arguments string)` / `OnToolCallEnd(…, output string, err)`），**没有结构化数据**（argv 数组 / 解析后的参数对象 / 搜索结果 / diff）。所以：

- **阶段 1（即将做，覆盖最常见路径）**：
  - `commandExecution`（bash）：从入参解出 `command` argv；stdout 走 `toolOutput` 流、`exitCode`/`durationMs` 完成时落定。
  - 通用 `tool`（read / MCP / subagent / 其余）：`arguments` 由后端 `unmarshal` 成对象再发（完成项 + 审批 payload 绝不回 JSON 字符串）；`result` best-effort JSON、绝不双重编码。
  - 一并完成：HITL `Interrupt` 判别联合 + `payload.tool` 复用 `ToolInvocation`；`ToolResultResponse` 改 `result?+error?`；§8 把 provider 错误分出 `provider_error`（接 E2 遗留）。
  - 完成阶段 1 即消除「kind↔name 双身份」与「双重转义」两个根问题——你方可先切 `switch(kind)` 覆盖 command + tool。
- **阶段 2（随后）**：`fileChange`（diff）、`search`/`webSearch`（结构化 `results`）。这几个需要让 engine 的 observer / 工具层透出结构化数据（动 engine + tools 层，较深），故后置。

**中间态约定**：阶段 1 落地前 wire 维持现状；落地时一次性切换（不混用新旧 kind）。我会在阶段 1 改完后再发一份 wire 对照，请你方 lockstep 切换并删 `parseArgs` 容错。

---

## 3. 本轮处置小结

| 项 | 状态 | 位置 |
| --- | --- | --- |
| E1 工具失败炸 run | ✅ 已修 `c5a03e8` | `core/model/chat`（`FeedbackOnToolError` + 控制流 carve-out）、`lyra engine` |
| E2 detail 泄漏栈 | ✅ 已修 `c5a03e8` | `lyra translator`（wire 干净文案，栈进 span） |
| E3 plan 工具策略 | ✅ 维持现状（E1 兜底） | — |
| E4 /v2/info providers | ✅ 已修 `c5a03e8` | `lyra server.Capabilities` |
| E5 id 必须 string | ✅ 已写文档 | `TRANSPORT.md §2` |
| §2 ToolInvocation 契约 | ⏳ 接受，分阶段 | 阶段1（command+tool+HITL+provider_error）即将做 |

> E1/E2/E4 已 push + 重启验证。§2 契约阶段 1 是下一批，改完发 wire 对照。
