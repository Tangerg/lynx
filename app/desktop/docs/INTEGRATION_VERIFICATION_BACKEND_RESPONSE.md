# Lyra 后端对 INTEGRATION_VERIFICATION（本轮）的回应

> **日期**：2026-06-05
> **后端构建**：`91ec6f9`（已推 origin/main，运行中 `127.0.0.1:17171`，已重启）
> **回应对象**：[`INTEGRATION_VERIFICATION.md`](./INTEGRATION_VERIFICATION.md) §1（B3 / W1 / deny）+ §2（工具参数约定提案）
> 契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准。

---

## TL;DR

| 项 | 状态 |
| --- | --- |
| **B3-1** 续延 run 不发 `run.started` | ✅ 已修（每段都发，带 `parentRunId`） |
| **B3-2** 实时流顺序 / 需要单调 seq | ⚠️ **上轮指导需更正**：`eventId` 只是**单条流内**的实时游标，**不是跨 run/跨重启的全局时钟**。跨 run 时间线请用 `items.list` 的 seq 顺序（见下） |
| 🆕 重启坑 A | `eventId` 进程重启后归零（内存计数器）→ 不能当全局排序键；正确排序见 B3-2 |
| 🆕 重启坑 B | 崩溃/Ctrl-C 打断的非中断 run 残留 `status:"running"` 幽灵；✅ 已修（`items.list` 惰性对账成 `error(run_lost)`） |
| 🆕 副作用：plan-review `question` 终态此前其实没生效 | ✅ 同 B3-1 一起救活（原来挂在续延不发的 TurnStart 上） |
| **deny 显示绿 ✓** | ✅ 已修（被拒工具 → `status:"incomplete"` + `error.type:"denied_by_user"`） |
| **W1 / §2** 工具 wire 形状不统一 | 🤝 **同意方向**，逐条回应见下；属**协议改动 → 建议先把契约钉进 API.md，再前后端 lockstep 实现** |

---

## ✅ B3-1 —— 续延 run 不发 `run.started`

**已修，根因比表面更深。** `run.started` 原来挂在 `chat.TurnStart` 上，而 **TurnStart 只在首段发**，resume/drive 路径一个都不发 → 续延 run 没有 run 边界。

修法：把 `run.started`（+ 首段的开场 userMessage Item + 续延的 plan-review question 终态）从 TurnStart 处理器移到 `translator.open()`，由 pump 在**每段开头**驱动 → **root 与续延 run 都保证 `run.started` 打头**。续延的 `run.started.parentRunId` 指向被中断的 run。

> 🆕 **副作用修复**：我上一轮加的"plan-review question 在续延补 `item.completed`"其实**从没生效**——它也挂在续延不发的 TurnStart 上，是死代码。这次一并救活：批准/拒绝 plan 后,plan 卡现在会在续延开头收到终态。

## ⚠️ B3-2 —— 实时流顺序（上轮指导更正）

**先纠正我上轮的说法。** 我上轮写"`eventId` 全局单调、跨续延也成立，前端按它排序"——这只在**单进程生命周期内**成立，**跨重启就是错的**（`eventId` 由内存计数器生成，重启归零，见下方"重启坑 A"）。`eventId` 的正确定位是**单条流内的实时游标**，不是跨 run 的全局时钟。

**正确的排序模型（三个场景各用各的键）：**

1. **持久态（刷新/重连后用 `items.list` 重建历史）**：后端 `history_items` 已按插入 `seq` 返回 → **前端直接按返回数组顺序渲染，不要再排**。run 树用 `parentRunId` 拼父子；`createdAt` **只用于显示时间，不做主排序键**（wall-clock，重启/时钟回拨会乱）。这个 seq 顺序**跨 run、跨续延、跨重启都对**——它是权威时间线。
2. **实时态（单个 run 的 SSE 流）**：在**这一条流内**按 `eventId` 排序 + 去重（`Last-Event-Id` 重连也是它）。单 run 内 `eventSeq` 只增不退，流内永远正确；hub 是内存的、一条 live 流不可能跨重启，所以"重启归零"对流内排序**无影响**。
3. **实时 ⊕ 历史 合并**：按 **item id** 关联（同一 item 的 `started`/`delta`/`completed` 都带同一 id）。历史 item 保持 `items.list` 位置，新来的 live item 追加到尾部。**全程不需要跨 run 的全局数字时钟。**

> 所以你们排期的"seq 重排"：**单 run 内**用 `eventId`，**跨 run/历史**用 `items.list` 顺序——后端不需要再加全局序字段。

关于"被批准工具迟发"：续延是**重跑该 LLM 轮**，工具在模型重新产出它的位置执行——这是 R 模型固有时序，后端无法把它"提前"。但：① `run.started` 边界现在每段都有了；② 单 run 内 `eventId` 给了流内排序键；③ `items.list`（durable, seq 序）重开即正常。实时态用流内 eventId，持久态用 items.list seq，两者自洽。

## ✅ deny 显示绿 ✓

**已修。** 被拒工具此前终态 `status:"completed"` + `output` → 前端渲染成绿 ✓。现在拒绝走 `engine.ErrToolDenied` → toolCall item 终态 = **`status:"incomplete"` + `error:{type:"denied_by_user", detail:"tool call denied by user"}`**,与"绿色成功"和"普通失败(`tool_failed`)"都可区分。前端按 `error.type==="denied_by_user"`(或 `status==="incomplete"`)渲染"拒绝"色即可。

---

## 🔁 进程重启 / 时序专项排查（本轮新增）

按"还有没有时序坑 + 考虑重启"做了一轮专项审计。**单进程内时序是干净的**（单 pump 顺序消费、`recordInterrupt`/落库都在事件上 wire 之前、`items.list` 按插入 seq 稳定排序）。重启相关挖到两个，结论如下：

### 坑 A —— `eventId` 重启归零（已更正指导，不改代码）

`eventId` 由 server 内存计数器生成，**进程重启从 0 重新开始**。所以重启后续延 run 的 `eventId` 会比被中断 run 留下的更小 → 撞号/逆序。**这只影响"把 eventId 当跨重启全局时钟"的用法**——即我上轮的错误指导。修正后（见 B3-2）：跨 run/历史一律靠 `items.list` 的 seq，`eventId` 只在单流内用。`Last-Event-Id` 重放是 per-hub、hub 内存、不跨重启，所以重放/去重不受影响。**故不硬修 eventId 格式**（它本就是 ephemeral 游标，强行兜底全局时钟属过度设计）。

### 坑 B —— 崩溃后残留 `running` 幽灵 run（✅ 已修）

非中断 run 在 `run.started` 与 `run.finished` 之间进程死掉（崩溃，或 dev 里 Ctrl-C 打断一个正在跑的 turn），终态永不落库。重启后该 run 不在活表、无 interrupt 记录（不可 resume），但 `items.list` 仍把它当 `running` 返回 → 前端永远转圈。

**修复**：`items.list` 惰性对账——`status:"running"` 但不在活跃运行表里的 RunRef，呈现为终态 `outcome:{type:"error", result:{error:{type:"run_lost", detail:"run lost on restart"}}}`。纯读路径对账（每次 list 重判 liveness，不写回），所以真正在跑的 run 绝不会被误判终结。

> **前端动作**：遇到 `RunRef.outcome.type==="error"` 且 `error.type==="run_lost"`，渲染为"运行已丢失（重启）"的终态卡，而不是继续 spinner。
>
> 注：**park 后重启再 resume 是干净的**——被中断 run 的 `run.finished{interrupt}` 已是落库终态，续延另起 run，不受此坑影响。

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
