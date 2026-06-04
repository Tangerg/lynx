# Lyra 前端 → 后端反馈（本轮）

> **日期**：2026-06-05
> **被测后端**：Lyra Runtime（`d01c5ad`，运行中），`http://127.0.0.1:17171`，streamable HTTP，`protocolVersion 2026-06-03`。
> **前端**：本仓库 frontend（HEAD）。
> 上一轮的 B1（`Session.model` 恒空）、B2（HITL 沿用原 item id）已修并活体验证通过；providers 配置/探活、多 provider×model、userItemId 对账均已打通。本文只记**本轮新发现**的后端问题 + 一个**工具参数约定提案**（解决反复对不齐）。
> 契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准；累积版历史见 git `3d90a10`。

---

## 1. 本轮后端问题

### B3 — 多步 HITL 后「实时流」消息乱序（后端实时排序）

**现象**：连续审批/执行多个工具后，气泡顺序看着前后颠倒（"已删除"出现在"我先删除"之上）。

**探针实测（多步 approve 流）**：

1. **resume / continuation run 不发 `run.started`**。续延 run 直接从 `item.started` 开始 → 前端没有 run 边界可用来分隔 turn / 设 running 态。
2. **被批准的工具在续延流里「迟发」**：approve 了 item A，续延流却先跑了别的 item，再回头补发 A 的 `item.started/completed` → **到达顺序非时间序**。
3. **但 `items.list`（durable，按 `createdAt`）顺序是对的** → 纯实时流问题，**重开会话顺序正常**。

**归属**：后端实时流排序 + 缺 `run.started`。前端按到达顺序如实 append（块层面配对已自查正确）。

**建议修法（择一）**：
- 续延 run 也发 `run.started`（带 `parentRunId`），且**实时流按时间序投递**（被批准工具的 start/complete 紧接其逻辑位置，不迟发）；或
- 给每个 `RunEvent` 加**单调 `seq`**（root run 内单调），前端据此重排 + 去重 + 断线重放（前端这边的 seq 重排是已排期项）。

### W1 — 命令工具 wire 形状与契约不符（详见 §2，这里只点名）

`API.md §4.4` 写命令工具是 `{kind:"command"; command; cwd?; output?; exitCode?}`，但实际发的是 `{kind:"command"; name:"bash"; output:"<json串>"}`，命令在流式 `arguments` 里。前端已容错（`name` 回退 + 解析 args），但这是反复对不齐的典型——见 §2 提案。

### 轻微 — deny 显示绿 ✓

deny 一个工具后，终态是 `status="completed"` + `tool.output="tool call denied by user"` → 前端工具卡显示绿 ✓（成功色）。审批卡本身已正确标"declined"。**建议**：被拒工具用一个可区分的终态（如 `incomplete`/`canceled`）或在结果里带 `denied:true`，让 UI 能显示"拒绝"色。非阻塞。

---

## 2. 工具参数约定提案（消除反复对不齐）

**问题根因**：工具的「身份 / 参数 / 结果」在 wire 上没有统一契约，同一份数据在不同地方形状不同，前端只能逐处猜+容错。本轮实测到的漂移：

| 位置 | 实际形状 | 问题 |
| --- | --- | --- |
| `ToolInvocation`（command） | `{kind:"command", name:"bash", output:"<json串>"}` | 契约写的是 `command`/`exitCode` 等，实际是 `name` + 无 `command`；命令藏在 args 里 |
| 流式参数 `item.delta{toolArguments}` | `argumentsTextDelta`（JSON **文本**增量） | 参数是「文本流」 |
| 审批 interrupt `payload` | `{tool, arguments:"<json串>"}`（JSON **字符串**） | 又变成字符串；且 payload 形状未进契约 |
| 完成项结果 | command `output:"<json串>"`；mcp `result:unknown`；subagent `result:string` | 结构化结果（stdout/exit_code…）被塞进**字符串**，前端要二次 parse；各 kind 不一致 |

→ 同一个「参数对象」在三处分别是 **文本增量 / JSON 字符串 / 对象**；结果在四处各不相同。这就是"老对不齐"的来源。

**提案（建议写进 `API.md §4.4` + §6，作为唯一真相）**：

- **A. 工具身份统一为 `{ kind, name }`**。`name` = 工具/函数名（`bash` / `read` / `grep` / mcp `server.tool`…）。命令工具的 shell 命令是**参数** `arguments.command`，不再设顶层 `command`。前端工具卡标题 = `name`，参数展开显示 `arguments`。

- **B. 参数权威表示 = 对象 `arguments: Record<string, unknown>`**。
  - 流式阶段：保留 `item.delta{toolArguments, argumentsTextDelta}`（本就是 JSON 文本流，前端累积+`item.completed` 时解析）。
  - **完成项 / 审批 payload：一律给已解析的对象**，**绝不回传 JSON 字符串**（消除 `"{\"command\":\"...\"}"` 这种双重转义）。

- **C. 结果结构化（不塞字符串）**。每个 kind 的 result 形状在 §4.4 显式定义，例如：
  - command → `result: { stdout: string; stderr: string; exitCode: number; durationMs: number }`（而非 `output: "<json串>"`）。
  - mcp / search / subagent 各自定义具体 result 形状，不用 `unknown`。

- **D. 审批 interrupt `payload` 进契约（§6）**：
  ```
  Interrupt.payload (kind="approval") = {
    tool: string;                       // 工具名
    arguments: Record<string, unknown>; // 已解析的对象（同 B）
    command?: string;                   // 便利字段：命令工具的 shell 命令
    risk?: "low"|"medium"|"high";
    scope?: string[]; target?: string; reversible?: boolean; reason?: string;
  }
  ```
  这样审批卡不必猜 command 在哪、不必处理字符串转义。

- **E. 单一真相 + 流程**：`ToolInvocation` 的 per-kind shape（身份 / args schema / result schema）是前后端**唯一契约**。新增工具：先在 `API.md §4.4` 登记 `kind` 定义，再实现；前端按它解析，后端按它发。任何"前端要二次 parse / 容错猜形状"的地方，都说明契约缺了一条，应回到 §4.4 补齐而不是各自打补丁。

> 收益：消除 §1 W1 这类反复出现的"形状漂移"，前端可去掉一圈防御性解析/容错，双方对着 §4.4 的 kind 表即可。

---

> 后端处理 §1（尤以 B3）后请告知，前端复测实时顺序；§2 的约定确定后我同步更新前端的 `ToolInvocation` 类型 + 解析逻辑，并删除现有的容错猜测。
