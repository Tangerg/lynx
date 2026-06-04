# Lyra 后端对 INTEGRATION_VERIFICATION 的回应

> **日期**：2026-06-04
> **回应对象**：[`INTEGRATION_VERIFICATION.md`](./INTEGRATION_VERIFICATION.md) 第 4 节「给后端的待办」
> **后端构建**：commit `a27c1ab`（已推 origin/main）
> **被测**：Lyra Runtime，`127.0.0.1:17171`，streamable HTTP，`protocolVersion 2026-06-03`，`deepseek-v4-flash`。
>
> 本文记录后端针对前端对接反馈逐项的处理 + 未处理项的原因。契约仍以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准。

---

## 🆕 对最新一轮 §4 的回应（后端已推进到 `6d26db7`）

> ⚠️ 你们这轮测的是 `a27c1ab`。之后后端已推进多个版本 —— **§4 里有几项其实已经做了**，请对**最新构建**重测：

| §4 项 | 状态 | 说明 |
| --- | --- | --- |
| **#1 `StartRunResponse` 带 `userItemId`** | ✅ **已做** | `runs.start` 响应现在回 `userItemId`（= 流上 / `items.list` 的同一个 userMessage Item id，`item_<runId>_u`）。可按**精确 id** 对账,弃用内容文本启发式。已实测响应 id == 流上 id。 |
| **#3 `providers.configure` / `providers.test`** | ✅ **已做** | 见下方「多 provider × 多 model」段。`configure` 真写持久化注册表;`test` 真实探活(`max_tokens=1`,401 内联)。providers 面板可从只读升级为可配。 |
| 多 provider × 多 model + per-run 选 model | ✅ **已做** | `providers.list` 返回全部支持的 provider(`apiKeyMasked==""` 即未启用);`models.list{provider}` 直读 catalog(带 displayName/contextWindow/maxOutput/capabilities/pricing,**无需 key**);`runs.start{ provider, model }` per-run 选用。 |
| **命名变更** ⚠️ | 需前端跟进 | 引用 provider 的 wire 参数统一为裸名 **`provider`**(非 `providerId`),与 `model`、`Model.provider` 一致。涉及 `runs.start` / `providers.configure` / `providers.test` / `models.list`。**`models.list` 尤其注意**:旧实现解码 `providerId`、文档写 `provider`,现已统一为 `provider` —— 前端发 `provider` 即可。 |
| #2 getDiff/grep/getFileHead | ⏳ 仍延后 | 随 agent 工具一起落地。 |
| #4 `sessions.update` / `fork` | ⏳ 待定 | `update` 需在 `session.Service` 加动词(破坏性公开 API,需先确认);`fork` 需对齐 checkpoint/item-id 模型。 |
| #5 `listSkills` | ⏳ 待 engine | skill 发现未实现。 |

**§5 assistant 名字**:`models.list` 已带 `displayName`,前端可据此渲染真实模型名(你们 §6.2 也已列入"现在就能做")。

下面是首轮(`a27c1ab`)的逐项回应,保留作历史。

---

## ✅ 已修：§4 #1 —— run 流投递 userMessage Item

前端 bug #5 的**正确解**。此前后端 run 流不回传用户自己的消息，live 视图无事件来源，前端只能本地乐观渲染、无法与 `items.list` 对账。

**改动**：

- 用户输入现在作为 run 的**首个 Item**（`type: "userMessage"`）随流投递：
  ```
  run.started → item.started(userMessage) → item.completed(userMessage) → agent items… → run.finished
  ```
- 它走的是与其它 item **同一条持久化路径**，所以**流上的 item id == `items.list` 的 item id**。前端可从「纯乐观渲染」升级为「乐观 + 按 id 去重」：重开会话按 `items.list` 替换乐观气泡时，id 能对上，不会重复。
- 续连 run（`runs.resume`）**不产** userMessage —— 决策走带外，无新用户回合。
- userMessage 的 `item.started` 与 `item.completed` 都带完整 `content`（用户消息无 delta 阶段）。

**实测**（真实 deepseek 跑通）：

```
data: {…"result":{"runId":"run_…"}}              # ack 帧，无 SSE id
id: evt_…1  run.started
id: evt_…2  item.started   userMessage  id=item_run_…_1  content=[…]
id: evt_…3  item.completed userMessage  id=item_run_…_1  content=[…]
id: evt_…4  item.started   agentMessage id=item_run_…_2
id: evt_…5  item.delta     "ok"
id: evt_…6  item.completed agentMessage "ok"
id: evt_…7  run.finished   completed (usage/cost)
```

`items.list` 重放返回**同一个 `item_run_…_1`** + 相同 content ✅。

> **前端可做的事**：把发送路径从「纯乐观渲染」改成「乐观 + 按 id 去重」。乐观气泡用本地 id 占位；收到流上 `item.started(userMessage)` 或重开 `items.list` 时，按真实 item id 替换/去重即可。

---

## ✅ 新增：多 provider × 多 model（providers/models 面板可对接）

后端已支持**多 provider，每 provider 多 model，per-run 选 model**：

- **`providers.list`** → 返回**全部支持的 provider**（anthropic / openai / moonshot / deepseek），每个带 `apiKeyMasked`（空=未配置）+ `baseUrl`。前端据此渲染 provider 列表 + 启用态。
- **`providers.configure`** `{providerId, apiKey, baseUrl}` → 写入运行态注册表（持久化），返回 masked 结果。
- **`providers.test`** `{providerId}` → 真实探活（`max_tokens=1` 极小请求），返回 `{ok:true}` 或 `{ok:false, error:{detail}}`（401 等原因内联，不报 RPC 错）。
- **`models.list`** `{providerId}` → 该 provider 的**全部 model + 元数据**（`displayName` / `contextWindow` / `maxOutputTokens` / `capabilities` / `pricing`），直读静态 catalog，**不需要 key**（解决「不填 key 拿不到 model 列表」）。
- **`runs.start{ providerId, model }`** → 该 run 用所选 provider+model 跑。**显式配对**:`providerId` 与 `model` 要么都传(选模型)、要么都不传(用服务端默认);**只传一个 → `invalid_params`**。provider **不从 model 推断**(显式大于隐式,且避免同名 model 跨 provider 撞车)。所选 provider 未配 key 时,run 以 `outcome:error`("set its API key first")干净收尾。同一会话连续两 run 可切不同 provider/model(已实测 v4-flash↔v4-pro)。
- **baseURL** 现在是 provider 配置项（代理 / 网关 / 自建 OpenAI 兼容端点）。

> **流程**：`providers.list`（看支持哪些）→ 用户填 key（`providers.configure`）→ `providers.test` 验 → `models.list` 解锁该 provider 的 model（`{id, provider}`）→ `runs.start{ providerId, model }` 选用（两者一起回传）。

---

## 🔜 未处理项 + 原因

| # | 项 | 状态 / 原因 |
| --- | --- | --- |
| §4 #2 | `workspace.getDiff` / `grep` / `getFileHead` | **后续随 agent 工具一起做**。这些 workspace 文件/git 读取能力会与 bash / file write·edit·read / grep / glob 工具 + git 指导 prompt 一并落地，届时再接 JSON-RPC（前端可同时删掉遗留的 REST 影子端点）。 |
| §4 #4 | `sessions.update`（改名 / 换 cwd） | **待定**。`session.Service` 目前无 `update` 动词，新增需改 interface 并在 file + sqlite 两个后端各补实现 —— 属破坏性公开 API 变更，按项目约定需先确认再动。 |
| §4 #4 | `sessions.fork`（复制） | **需先对齐模型**。`session.Service` 已有 `Fork`，但「在 item 边界 fork」要先把 checkpoint / item-id 模型与 engine 的 history 对齐（非纯接线，属设计活）。 |
| §4 #5 | `workspace.listSkills` | **需 engine 支持**。skill 发现尚未在 engine 实现，故 `features.skills:false` 门控。 |

### 📝 文档勘误

- §4 #5 / §2 矩阵把 **`listAgentDocs`** 和 `listSkills` 并列标为 gated —— **`workspace.listAgentDocs` 实际已实现**（从 cwd→home 级联发现 AGENTS.md，与 engine 注入系统提示的同一套）。只有 `listSkills` 仍 gated。前端可对 `listAgentDocs` 重测。

---

## §5 其它观察的后端侧确认

- **assistant 名字硬编码**：纯前端项，后端通过 `providers.list` / `models.list` / run 的 `usage.byModel` 已暴露真实模型名（`deepseek-v4-flash`），前端可据此渲染。
- **diff/grep/file-head 走 REST 影子**：后端无此 REST 路由（设计上禁止业务 read shadow，见 API.md §9.3）；待 §4 #2 的 JSON-RPC 方法就绪后前端迁移并删影子。

---

> 后端补齐后会回到本文逐项更新状态。
