# Lyra 后端对 INTEGRATION_VERIFICATION 的回应

> **日期**：2026-06-04
> **回应对象**：[`INTEGRATION_VERIFICATION.md`](./INTEGRATION_VERIFICATION.md)（§4 待办 · §4.1 后端 bug · §7.3 spec 缺口）
> **后端构建**：`d01c5ad`（已推 origin/main）
> 本文逐项回应前端反馈。契约仍以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准。

---

## TL;DR

| 项 | 来源 | 状态 |
| --- | --- | --- |
| **B1** `Session.model` 恒空 | §4.1 | ✅ 已修 |
| **B2** HITL 审批后工具新 item id、原 item 永不终态 | §4.1 | ✅ 已修（按你们 §7.3 建议「沿用原 item id」实现） |
| 🆕 中断时开着的 tool item 不终态 | 后端自查（§5.2 全扫） | ✅ 顺带修 |
| 🆕 plan-review `question` item 永不终态 | 后端自查（§5.2 全扫） | ✅ 顺带修 |
| 已批准工具执行如何回链审批 item | §7.3 | ✅ 已钉死「沿用原 item id」（见下，建议写进契约） |
| `workspace.getDiff` / `grep` / `getFileHead` | §4 #1 | ⏳ 仍延后；但 **per-session cwd 地基已铺好**（见下） |
| `sessions.update` / `sessions.fork` | §4 #2 | ⏳ 待定（破坏性 API / checkpoint 模型） |
| `features.skills` / `listSkills` | §4 #3 | ⏳ 待 engine |
| API.md §7.3 正文残留 `providerId` | §7.3 | 📝 那是**前端仓文档**笔误，建议你们改成 `provider` |

---

## ✅ B1 —— `Session.model` 恒空（§4.1）

**已修。** `Session.model` 现在带真实模型名：

- session 落库时记录其 model（domain 加 `Model` 字段，sqlite 加 `model` 列）。
- `runs.start{provider, model}` **显式选模型**时把它写回该 session。
- 从未显式选过模型的 session（用服务端默认 model 跑），`sessions.list` / `sessions.get` 在 wire 层回退到运行时默认 model —— **所以 `Session.model` 永远非空**，前端解析 displayName 即点亮。

> 前端无需改动：你们 §5.2 说「后端一填即点亮」，现已填。

---

## ✅ B2 —— HITL item-id 不连续（§4.1）+ §7.3 的「回链」钉死

**已修，且正是按你们 §7.3 建议的「沿用原 item id」实现。**

冻结契约里这是 **§5.2 铁律**违例（"每个 item 的最终态必然出现在**后续** `item.completed`"——R 模型下「后续」跨 interrupt 到延续 run），不是渲染问题。修法：

- 续延 run 复用被中断 run 那个 toolCall 提案 item 的**原 id + 原 runId**（不再 mint 新 id）。
- 一个 item 走完 **提案 → 批准 → 执行 → 完成**；`item.completed` 落在原 id 上。
- `items.list` 天然干净（只有 `item.completed` 入库），最终一条 completed item、归属原 run，无重复卡、无永久 LIVE。
- **拒绝**路径同样闭合（deny 也会重发 start+end、复用原 id，output=拒绝理由）。
- **edited-args** 审批（改了参数）不匹配 → 干净回退到新 item，不比现状差。

> **§7.3 的争议点（"§6/§4.4 未规定 同 id 还是新 id"）**：我们采纳你们的建议**钉死「沿用原 item id」**。建议把这条写进 API.md §6：*已批准/拒绝的工具在延续 run 沿用原 toolCall item 的 id 续跑（item.delta/completed 打在原 id 上）*。

---

## 🆕 后端自查：§5.2 全路径扫描，又修两处同类孤儿

修 B2 后我们把**每条 `item.started → item.completed` 路径**对照所有终止/中断/拒绝路径全扫了一遍，又发现并修了两个同源 §5.2 缺口：

1. **中断时开着的 tool item**：`interrupt` 原来只关 text/reasoning、不 drain 开着的 toolCall（gated 调用本身在 started 前已暂停，但并行的兄弟 tool 可能在飞）→ 会留永久 inProgress。已对齐 `turnEnd`/`finish` 补 drain（incomplete 收尾）。
2. **plan-review 的 `question` item**：它由 resume 的答复闭合、没有 re-fire 事件,所以批准/拒绝**都不补终态** → 提案卡永久 LIVE。续延起点现在显式补 `item.completed`（同 id/runId、保留 Question 内容,`items.list` 也正确）。

**已审计确认全部配平**：userMessage（原子）/ agentMessage / reasoning / toolCall（执行·批准·拒绝·出错·中断时 drain）/ question / run.started↔finished。

---

## ⏳ §4 待办

### §4 #1 `workspace.getDiff` / `grep` / `getFileHead`
仍延后接 JSON-RPC。**但地基已就位**：这轮后端做了 **per-session 工具工作目录** —— fs/bash 工具现在跑在该 session 的 `Session.cwd`（不再是 serve 启动目录），多 project 在工具层成立。getDiff/grep/getFileHead 落地时直接复用这套 cwd（grep/getFileHead 走 fs、getDiff 走 git）。届时你们可删掉遗留 REST 影子。

### §4 #2 `sessions.update` / `sessions.fork`
仍待定。`update` 需在 `session.Service` 加动词（破坏性公开 API，需先确认）；`fork` 需对齐 checkpoint / item-id 模型。

### §4 #3 `features.skills` / `listSkills`
仍待 engine 实现 skill 发现，故 `features.skills:false`。

---

## 📝 §7.3 文档勘误（前端仓）

- **API.md §7.3 正文残留 `providerId`**：与命名变更表 + 参数表（`provider`）矛盾。这是**你们仓里的 API.md**，建议统一改成 `provider`（运行中后端 + 我们这边都按 `provider`）。

---

## 已知边界（非孤儿、是「未接」）

- **子 agent（`task`）的内部 item 当前不上 run 流**：observer 是 process-scope、不传子进程,所以子 run 的 message/tool 事件没被 translate。这不是 start/complete 失衡（没发就没孤儿），属 §5.4「root 流含整棵 run 树」+ `features.subagents` 门控的范畴，列入后续。

---

> 重测建议：B1（assistant 名字点亮）、B2（审批/拒绝一个工具,原卡走到完成、无重复）、plan-review（批准/拒绝后 plan 卡收尾）对**最新构建 `d01c5ad`** 复核。
