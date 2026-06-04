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

## 2. §2 ToolInvocation 新契约 — ✅ 已全部实现 + 活体验证（`commit 后续`）

原以为要分两阶段（observer 只透字符串），但发现**后端工具的 output 本就是结构化 JSON**（bash→`{stdout,exit_code,duration}`、grep→`{matches:[{path,line,text}]}`、glob→`{paths}`、websearch→`{results:[{title,url,snippet,favicon_url}]}`），translator 解析即得全部富字段，**无需动 engine/observer/tools**。所以**五变体一次做到位**。

**§4.4 ToolInvocation** — 已切判别联合，去 kind↔name 双身份，`arguments` 恒解析对象、`result` best-effort JSON、绝不双重编码。后端按工具名映射：

| 工具 | wire `kind` | 字段来源 |
| --- | --- | --- |
| bash | `commandExecution` | `command`=argv(单元素 shell 串)；`exitCode`/`durationMs` 解析自 output；**stdout 走 `item.delta{toolOutput}`，不在完成项上** |
| grep / glob | `search` | `query`=pattern；`results[]`={path,lineNumber?,snippet?} 解析自 output |
| websearch | `webSearch` | `query`；`results[]`={title,url,snippet,faviconUrl} |
| write / edit | `fileChange` | `changes[]`={path, kind:"modify"}（**diff 暂缺**，见下） |
| read / webfetch / httpreq / MCP `<server>.<tool>` / subagent / 其余 | `tool` | `name` + `arguments`(对象) + `result`(best-effort JSON) |

**§4.8 Interrupt** — approval payload 改为 `{tool: ToolInvocation}`（复用 §4.4，你方读 `payload.tool` 即可，不再猜 command/转义）。question 无 payload。
> 注：approval payload 里另有一个 `_resume` 字段（`{name,args}`）是**后端内部**用来把"恢复后重发的工具"重新绑定到原 proposal item（强类型变体 wire 上没 name，无法从 `tool` 反推）。**前端按 §11 忽略未知字段即可**，不要消费它。

**§6.1 ToolResultResponse** — `output:string` → `result?:unknown + error?:ProblemData`，与 `ToolInvocation.result` 同形。

**§8.4 工具级 error type** — `toolCall.error.type` 的 `tool_failed` / `denied_by_user` 早已正确落在 item 上（你方 E1/deny 验证已确认），无需改。

**活体验证**（bash `echo hello-lyra`，过审批）：
- 提案 + 审批 payload.tool = `{kind:"commandExecution", command:["echo hello-lyra"]}` ✓
- approve resume 后：`toolOutput` delta `text:"hello-lyra\n"` → 完成项 `{kind:"commandExecution", exitCode:0, durationMs:17}` ✓
- 完成项**复用原 proposal item id**（一个 item 走 提案→审批→执行→完成）✓

**中间态**：wire 已一次性切到新形态（不混用旧 kind）。请你方 lockstep 切 `switch(kind)` + 删 `parseArgs`/容错解析。

**仍有两处后续（非阻塞，下一批）**：
1. **`fileChange.diff`**：write/edit 工具当前只回 `{replacements}`/`{bytes_written}`，拿不到 diff 行；需让 fs 工具透出结构化 diff（动 tools 层）。当前 `fileChange` 只给 `{path, kind:"modify"}`（diff 可选，契约允许）。`kind` 暂统一 "modify"（无法可靠区分 add/modify，需 stat）。
2. **`provider_error`（接 E2 遗留）**：provider 类错误（401/限流）目前仍归 `internal_error`；应分类成 `provider_error`（§8.2，码 `-32001`）带干净 detail。

---

## 3. 本轮处置小结

| 项 | 状态 | 位置 |
| --- | --- | --- |
| E1 工具失败炸 run | ✅ 已修 `c5a03e8` | `core/model/chat`（`FeedbackOnToolError` + 控制流 carve-out）、`lyra engine` |
| E2 detail 泄漏栈 | ✅ 已修 `c5a03e8` | `lyra translator`（wire 干净文案，栈进 span） |
| E3 plan 工具策略 | ✅ 维持现状（E1 兜底） | — |
| E4 /v2/info providers | ✅ 已修 `c5a03e8` | `lyra server.Capabilities` |
| E5 id 必须 string | ✅ 已写文档 | `TRANSPORT.md §2` |
| §4.4 ToolInvocation 五变体 | ✅ 已实现 + 活体验证 | `rpc/protocol` + `rpc/server/translator.go` |
| §4.8 Interrupt（payload.tool） | ✅ 已实现 | `rpc/server/translator.go` |
| §6.1 ToolResultResponse（result+error） | ✅ 已实现 | `rpc/protocol/runs.go` |
| §8.4 tool error type | ✅ 早已正确 | `rpc/server/translator.go` |
| fileChange diff / add-vs-modify | ⏳ 下一批 | 需 fs 工具透出结构化 diff |
| provider_error 分类（E2 遗留） | ⏳ 下一批 | §8.2 错误分类 |

> 全部已 push + 重启验证。**请你方 lockstep 切 `switch(kind)` + 删 `parseArgs`/容错**——wire 已一次性切到新形态。剩 fileChange diff + provider_error 两处后续（非阻塞）。
