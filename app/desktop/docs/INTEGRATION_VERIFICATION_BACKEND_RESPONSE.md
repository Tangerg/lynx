# Lyra 后端对 INTEGRATION_VERIFICATION（本轮）的回应

> **日期**：2026-06-05
> **后端构建**：`23ef1a0`（已推 origin/main，运行中 `127.0.0.1:17171`，已重启）
> **回应对象**：[`INTEGRATION_VERIFICATION.md`](./INTEGRATION_VERIFICATION.md) §1（B3 / W1 / deny）+ §2（工具参数约定提案）
> 契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准。

---

## TL;DR

| 项 | 状态 |
| --- | --- |
| **B3-1** 续延 run 不发 `run.started` | ✅ 已修（每段都发，带 `parentRunId`） |
| **B3-2** 实时流顺序 / 需要单调 seq | ✅ **`RunEvent.eventId` 本就全局单调可排**（见下），用它重排即可；run.started 缺口已补 |
| 🆕 副作用：plan-review `question` 终态此前其实没生效 | ✅ 同 B3-1 一起救活（原来挂在续延不发的 TurnStart 上） |
| **deny 显示绿 ✓** | ✅ 已修（被拒工具 → `status:"incomplete"` + `error.type:"denied_by_user"`） |
| **W1 / §2** 工具 wire 形状不统一 | 🤝 **同意方向**，逐条回应见下；属**协议改动 → 建议先把契约钉进 API.md，再前后端 lockstep 实现** |

---

## ✅ B3-1 —— 续延 run 不发 `run.started`

**已修，根因比表面更深。** `run.started` 原来挂在 `chat.TurnStart` 上，而 **TurnStart 只在首段发**，resume/drive 路径一个都不发 → 续延 run 没有 run 边界。

修法：把 `run.started`（+ 首段的开场 userMessage Item + 续延的 plan-review question 终态）从 TurnStart 处理器移到 `translator.open()`，由 pump 在**每段开头**驱动 → **root 与续延 run 都保证 `run.started` 打头**。续延的 `run.started.parentRunId` 指向被中断的 run。

> 🆕 **副作用修复**：我上一轮加的"plan-review question 在续延补 `item.completed`"其实**从没生效**——它也挂在续延不发的 TurnStart 上，是死代码。这次一并救活：批准/拒绝 plan 后,plan 卡现在会在续延开头收到终态。

## ✅ B3-2 —— 实时流顺序 / 单调 seq

**后端早已提供单调序：** 每个 `RunEvent.eventId` 是 **server 全局单调、零填充（`evt_00000000123`）、字典序可排** 的 id，跨所有 run（含续延）单调。这就是你们要的"root run 内单调 seq"——而且是**全局**单调,跨续延也成立。**前端按 `eventId` 排序 + 去重 + 断线重放即可**(你们已排期的 seq 重排直接用它,不需要后端再加字段)。

关于"被批准工具迟发":续延是**重跑该 LLM 轮**,工具在模型重新产出它的位置执行——这是 R 模型的固有时序,后端无法把它"提前"到某个逻辑位置(它就是那时才真跑)。但:① `run.started` 边界现在有了;② `eventId` 给了可排序键;③ `items.list`(durable, createdAt 正确)重开即正常。所以实时态用 eventId 排,持久态用 items.list,两者都自洽。

## ✅ deny 显示绿 ✓

**已修。** 被拒工具此前终态 `status:"completed"` + `output` → 前端渲染成绿 ✓。现在拒绝走 `engine.ErrToolDenied` → toolCall item 终态 = **`status:"incomplete"` + `error:{type:"denied_by_user", detail:"tool call denied by user"}`**,与"绿色成功"和"普通失败(`tool_failed`)"都可区分。前端按 `error.type==="denied_by_user"`(或 `status==="incomplete"`)渲染"拒绝"色即可。

---

## 🤝 W1 + §2 —— 工具参数约定（同意方向，逐条回应）

你们说得对:工具的「身份 / 参数 / 结果」在 wire 上形状漂移(`name` vs `command`、args 时而文本流时而 JSON 字符串、result 塞字符串),是"反复对不齐"的根。**后端同意把它收敛成单一契约。** 但这是**协议层改动**(改 `API.md §4.4/§6` + 双端 wire),按我们的流程**应先把契约钉死,再前后端 lockstep 改**——所以本轮我先给逐条立场,不擅自单方改 wire(避免你们现有容错解析中途崩)。

逐条:

- **A. 身份 `{kind, name}`,command 进 `arguments.command`** —— **同意**。后端已经在发 `name`;会去掉"command 工具靠顶层 `command` 字段"的预期,shell 命令归 `arguments.command`。
- **B. 参数权威 = 已解析对象 `arguments: Record<string,unknown>`** —— **同意**。流式阶段保留 `item.delta{argumentsTextDelta}`(JSON 文本流);**完成项 + 审批 payload 一律给已解析对象,绝不回传 JSON 字符串**(消除双重转义)。后端侧:translator 在 `item.completed` / 审批 interrupt 处把累积的 args 文本 `json.Unmarshal` 成对象再发。
- **C. 结果结构化、不塞字符串** —— **同意方向**,但提醒一处工程现实:result 的结构是**每个工具自己的 schema**(bash 的 `{stdout,stderr,exitCode,durationMs}`、fs 的、mcp 的…)。后端要在 translator 里**按 kind 解析各工具的输出**才能给出结构化 result —— 这是 per-kind 的活,建议**逐 kind 落**(先 command,再 fs/search/mcp/subagent),每个 kind 的 result schema 在 §4.4 显式定义。
- **D. 审批 interrupt `payload` 进契约(§6)** —— **同意**。后端会发 `{tool, arguments:<对象>, command?}`(command 工具带便利 `command`)。但 `risk/scope/target/reversible/reason` 这些**后端目前没有来源**(没有风险分级/影响面分析引擎)—— 这些字段先**留空/可选**,等后端有能力计算再填,不阻塞 A/B/D 主体。
- **E. 单一真相 + 流程** —— **同意**。`ToolInvocation` 的 per-kind shape 是唯一契约;新增工具先在 §4.4 登记 kind 再实现。

**建议的落地流程**:
1. 你们在 `API.md §4.4`(per-kind ToolInvocation:身份/args/result)+ `§6`(approval payload)把契约钉死(把上面 A/B/C/D 写进去,risk 等标 optional)。
2. 后端按它逐 kind 实现 wire(我来),前端同步去掉容错猜测 —— lockstep。
3. 先做 **command kind**(覆盖 W1 + 最常见路径)跑通,再推其余 kind。

> 我可以现在就先把**与 kind 无关的两块**做了(都是 additive、不破坏你们现有解析):①完成项/审批 payload 的 `arguments` 给已解析对象;②审批 payload 带便利 `command`。结构化 result(C)等 §4.4 的 per-kind schema 定下来再逐个接。你们定。

---

> 重测建议(对最新构建 `23ef1a0`,server 已重启):
> - B3:多步审批后,续延 run 现在有 `run.started`(带 parentRunId);按 `eventId` 排序实时流。
> - plan 模式:批准/拒绝后 plan 卡收到终态。
> - deny:拒绝一个工具,工具卡显示"拒绝"(incomplete + denied_by_user),非绿 ✓。
