# Lyra 前后端对接验证报告（UI 层）

> **日期**：2026-06-04
> **被测后端**：Lyra Runtime（`serverInfo.name = "runtime"`），构建 `a27c1ab`，监听 `http://127.0.0.1:17171`，**streamable HTTP** transport，`protocolVersion 2026-06-03`，模型 `deepseek-v4-flash`。
> **被测前端**：本仓库 frontend（Vite dev，HTTP transport 直连 :17171），修复批 `560b9f4` + `fb860b4`。
> **范围**：把 UI 各功能接到真实后端（会话生命周期、历史、流式、模型、providers…），记录对接暴露的前端 bug + 后端缺口。
>
> 本文是**某次后端构建**的对接快照，不是契约。契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准；后端缺口补齐后请重测并更新。后端对上一版反馈的逐项回应见 [`INTEGRATION_VERIFICATION_BACKEND_RESPONSE.md`](./INTEGRATION_VERIFICATION_BACKEND_RESPONSE.md)。

---

## 0. 结论

**核心链路端到端打通**：握手 → 建会话 → 流式对话（真实模型）→ 历史加载 → 删除会话，全部用真实前端 transport 对真实后端验证通过。

对接两轮共发现并修复 **14 个纯前端 bug**（见 §3，按层分组）。后端已补齐反馈里最关键的一项（run 流投递 userMessage Item），前端配套从「纯乐观渲染」升级为「乐观 + 按 id 对账」。剩 **5 类后端方法**未实现（`capability_not_negotiated`），各有原因（§2），对应 UI 先行容错/占位。

> **贯穿性教训**：后端覆盖面会随构建**变动**（本会话期间 providers/models 由空→有数据；`listAgentDocs` 由"以为 gated"→已实现）。前端**必须对 `capability_not_negotiated` 优雅降级**，UI shell 必须区分 loading / empty / **error** 三态（不能把失败当空态）。

---

## 1. 已验证通过（真实前端代码 + 真实后端 E2E）

| 流程 | 方法 | 结果 |
| --- | --- | --- |
| 握手 | `runtime.initialize` | ✅ `runtime` / `2026-06-03` |
| 列 / 删会话 | `sessions.create` / `list` / `delete` | ✅ `ses_…`、`cwd=/Users/tangerg`；删后列表减少、侧栏右键可删 |
| 流式对话 | `runs.start` + `notifications.run.event` | ✅ `run.started → item.started(userMessage) → item.completed(userMessage) → item.started(agentMessage) → item.delta×N → item.completed → run.finished` |
| **用户消息回显** | run 流首个 Item（`userMessage`） | ✅ 流上 item id **==** `items.list` id；前端乐观气泡按 id 对账，不重复 |
| 选模型 | `models.list` → `runs.start{model}` | ✅ run 的 `usage.byModel` 反映所选 `deepseek-v4-flash` |
| 历史 | `items.list` → reduce 重建 | ✅ 重建出 `user[text] | assistant[text]` |
| providers | `providers.list` | ✅ `deepseek`（`apiKeyMasked` 掩码） |

---

## 2. 后端覆盖矩阵

### ✅ 已实现可用

`runtime.initialize` · `sessions.create/list/delete` · `runs.start/resume/cancel` · `items.list`（含 run 流回显 `userMessage` Item）· `workspace.listProjects/listFileChanges/listAgentDocs/mcp.listServers` · `providers.list` · `models.list`

### ❌ `capability_not_negotiated`（未实现 / 能力关闭）—— 阻塞对应 UI

| 方法 | 阻塞的 UI | 后端给出的原因 |
| --- | --- | --- |
| `workspace.getDiff` / `grep` / `getFileHead` | Diff / 搜索 / 文件预览 | 随 agent 工具（bash / file r·w·edit / grep / glob + git prompt）一并落地，届时接 JSON-RPC（前端同时删 REST 影子） |
| `providers.configure` / `providers.test` | providers 面板配置 / 测试 | 当前只对接 `config.yaml` 单 provider，无可写 registry；需新增配置可变层（设计决策）。面板暂只读 |
| `sessions.update`（改名 / 换 cwd） | 会话重命名 / relocate | `session.Service` 无 `update` 动词；新增属破坏性公开 API，需先确认 |
| `sessions.fork`（复制） | 会话复制 | 已有 `Fork`，但"按 item 边界 fork"需先对齐 checkpoint / item-id 模型与 engine history |
| `workspace.listSkills` | Skills 视图 | engine 尚未实现 skill 发现，故 `features.skills:false` |

### 未接（按 `features` 关闭，低优先）

`attachments`（`enabled:false`）· `background`（`false`）· `items.edit`（`checkpoints:false`，重试）· `memory`（`true`，可接但暂无 UI）· `feedback`（未门控）

---

## 3. 对接中发现并修复的前端 bug（14 个，按层分组）

### 协议 fold 层（`protocol/run/` · `builtin/agent/core-reducer/`）

| 现象 | 根因 | 修复 |
| --- | --- | --- |
| 内容区不流式，只在最后一次性出现 | `item.started` 壳只带 `ItemBase`，body（content/text/steps/question/tool）走 delta；`contentText(undefined).filter` 等崩溃，被 reducer try/catch 吞掉 → 整块永不渲染 | 全部 6 类 item 容忍 body-less 壳（seed 空串 / `?? []` / 占位 label），started 折叠成空块由 delta 填 |
| 流偶发整条挂掉（malformed event） | `RunTree.admit` 在无 try/catch 的 subscribe 回调里裸解引用 `event.run` / `event.item`（Zod 只校验了 `type`） | 边界把 run/item 当可缺失：脏数据更新无效并丢弃，绝不抛 |
| 发消息后**自己的气泡重复**（后端上线 userMessage 流后） | 后端流式真实 id 的 userMessage 与本地 `local-*` 乐观气泡共存；`appendUserMessage` 仅按 id 去重 → 追加成第二个 | 乐观 + 对账：按 `role + 内容文本`（最旧优先=发送顺序）就地把占位气泡 id 升级为真实 server id；started/completed/重开 `items.list` 三处收敛成一个 |

### 状态层（`state/`）

| 现象 | 根因 | 修复 |
| --- | --- | --- |
| 删最后一个会话后状态悬挂 | `closeTab` 在无 tab 剩余时不清 `activeSessionId` | 回退邻居或 `""`（欢迎页） |
| 草稿引用无限增长 + 重开错跳历史 | `draftSessionIds` / `pendingMessages` 关/删会话时从不清 | 一个 `tabIds`-diff 订阅兜住所有关闭路径，剔除已离场 id |
| A 会话的工具选择/展开/文件焦点串进 B | `selectedToolId` / `expandedToolIds` / `activeFile` 注释称"session-scoped"实为全局单值，切会话不清 | `selectTab` 在会话真正切换时清掉三者 |
| HITL：解决一个审批，同批未解决的兄弟 interrupt 消失 | `resolveInterrupt` 用 `some` 把整个 OpenInterrupt 信封删掉 | 粒度过滤：只删命中的 interrupt，信封空了才删 |

### UI 壳（`components/` · 各 view 插件）

| 现象 | 根因 | 修复 |
| --- | --- | --- |
| 主区全空白、无从开聊 | 0 会话时 `ChatPanel` 直接 `return null`，欢迎页没机会渲染（后端重启清空会话踩中） | 仅"首次加载中"返 null；加载完即使 0 会话也渲染欢迎页 |
| 后端查询失败被伪装成"暂无数据" | `DataView` 只有 loading/empty 两态，rejected query（宕机 / 401 / `capability_not_negotiated` / 无 provider）落入 empty 分支 | 加 `isError` 错误态 + `alert` 图标；9 个消费方透传 `isError` |
| 插件 view 卸载后留下空白死 tab | `WorkspaceViewBody` 对未注册 view id 返 `null` | 渲染 "View unavailable" fallback |
| 侧栏会话计数闪 `0` | 计数 pill 在首帧 `data===undefined` 时显示 0 | 首次 fetch 落定前隐藏 pill |

### 输入 / 边界容错

| 现象 | 根因 | 修复 |
| --- | --- | --- |
| 中文输入法回车重复/误发（"你好啊"→"你你好啊"） | `onKeyDown` 未拦 IME 合成期 Enter | `e.nativeEvent.isComposing` 时直接 return |
| Skills 视图报错 | 后端把 `listSkills` 改成 gated，前端直接抛错 | 数据 provider 捕获 `capability_not_negotiated` → 返 `[]`（空态） |

---

## 4. 给后端的待办（做了 UI 立刻能用）

按价值排序：

1. **`StartRunResponse` 带上 `userItemId`** —— 当前前端只能按**内容文本**对账乐观气泡与回显的 userMessage Item（"极快连发两条相同文本"理论上有歧义）。在 `runs.start` 响应里回 userMessage 的 item id，前端即可按**精确 id** 对账，彻底消除启发式。item id 是业务字段而非 transport 元数据，不违反 §6.2。
2. **`workspace.getDiff` / `grep` / `getFileHead`** —— 解锁 Diff / 搜索 / 文件预览，并删掉前端遗留的 REST 影子。
3. **`providers.configure` / `providers.test`** —— providers 面板配置 + 连接测试（需先定可写配置层）。
4. **`sessions.update` / `sessions.fork`** —— 会话重命名 / 复制。
5. **开 `features.skills`**（或实现 `listSkills`）—— Skills 视图填数据。

---

## 5. 其它观察（非阻塞）

- **assistant 名字硬编码 "Sonnet 4.5"**（`defaultRoles` 的 `MESSAGE_ROLE` displayName）。后端确认真实模型名可经 `models.list`（含 `displayName`）/ run 的 `usage.byModel` 拿到 → 前端可据此渲染，待定（跟随真实模型名 / 改中性 "Assistant"）。
- **terminal 视图是纯 mock**（协议无 terminal 方法），按约定保留作设计占位。
- **diff/grep/file-head 仍走 REST 影子**（`defaults` 的 `HTTP_KEYS`），真后端无此路由（API.md §9.3 禁业务 read shadow）→ 等 §4 #2 的 JSON-RPC 方法就绪后迁移并删影子。

---

> 后端补齐第 4 节后请重跑 `runs.start` 流式（含 userMessage Item）+ `items.list` + 对应新方法复核，并更新本文。
