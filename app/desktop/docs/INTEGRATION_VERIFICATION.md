# Lyra 前后端对接验证报告（UI 层）

> **日期**：2026-06-04
> **被测后端**：Lyra Runtime（`serverInfo.name = "runtime"`），监听 `http://127.0.0.1:17171`，**streamable HTTP** transport，`protocolVersion 2026-06-03`，模型 `deepseek-v4-flash`。
> **被测前端**：本仓库 frontend（Vite dev，HTTP transport 直连 :17171）。
> **范围**：把 UI 各功能接到真实后端（会话生命周期、历史、流式、模型、providers…），并记录对接中暴露的前端 bug + 后端缺口。
>
> 本文是**某次后端构建**的对接快照，不是契约。契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准；后端补齐缺口后请重测并更新。

---

## 0. 结论

**核心链路已端到端打通**：握手 → 建会话 → 流式对话（真实模型）→ 历史加载 → 删除会话，全部用真实前端 transport 对真实后端验证通过。对接中发现并修复了 **6 个前端 bug**；**5 类后端方法仍未实现**（`capability_not_negotiated`），对应 UI 先行容错/占位，待后端补齐再接。

> **关键现象**：本会话期间后端覆盖面**变动过**（providers/models 从空 → 有真实数据；skills/agent-docs 从 `[]` → gated）。结论:**前端必须对 `capability_not_negotiated` 优雅降级**，不能假定某方法恒可用。

---

## 1. 已验证通过（真实前端代码 + 真实后端 E2E）

| 流程 | 方法 | 结果 |
| --- | --- | --- |
| 握手 | `runtime.initialize` | ✅ `runtime` / `2026-06-03` |
| 建会话 | `sessions.create` | ✅ `ses_…`、`cwd=/Users/tangerg` |
| 列会话 | `sessions.list` | ✅ |
| 删会话 | `sessions.delete` | ✅ 删后列表减少、侧栏右键可删 |
| 流式对话 | `runs.start` + `notifications.run.event` | ✅ `run.started → item.started → item.delta×N → item.completed → run.finished` |
| 选模型 | `models.list` → `runs.start{model}` | ✅ run 的 `usage.byModel` 反映所选 `deepseek-v4-flash` |
| 历史 | `items.list` → reduce 重建 | ✅ 重建出 `user[text] | assistant[text]` |
| providers | `providers.list` | ✅ `deepseek`（`apiKeyMasked` 掩码） |

---

## 2. 后端覆盖矩阵

### ✅ 已实现可用

`runtime.initialize` · `sessions.create/list/delete` · `runs.start/resume/cancel` · `items.list` ·
`workspace.listProjects/listFileChanges/mcp.listServers` · `providers.list` · `models.list`

### ❌ `capability_not_negotiated`（未实现 / 能力关闭）—— 阻塞对应 UI

| 方法 | 阻塞的 UI | 备注 |
| --- | --- | --- |
| `sessions.update` | 会话重命名 | UI 已留，未接 |
| `sessions.fork` | 会话复制 | UI 已留，未接 |
| `workspace.getDiff` | Diff 视图 | 现仍挂在**坏的 REST 影子**端点上（真后端无此 REST 路由），待迁 JSON-RPC |
| `workspace.grep` | 搜索 | 同上 |
| `workspace.getFileHead` | 文件预览 | 同上 |
| `workspace.listSkills` / `listAgentDocs` | Skills / Agent docs 视图 | 受 `features.skills:false` 门控；前端已容错（返空） |
| `providers.configure` / `providers.test` | providers 面板的配置/测试 | 面板现为只读 |

### 未接（按 `features` 关闭，低优先）

`attachments`（`enabled:false`）· `background`（`false`）· `items.edit`（`checkpoints:false`，重试）· `memory`（`true`，可接但暂无 UI）· `feedback`（未门控）

---

## 3. 对接中发现并修复的前端 bug

| # | 现象 | 根因 | 修复 |
| --- | --- | --- | --- |
| 1 | 内容区不流式，只在最后一次性出现 | `agentMessage`/`reasoning` 的 `item.started` 壳**不带 content/text**(靠 delta 流式)，`contentText(undefined).filter` 崩溃,跳过建块 | fold 容忍缺失 content/text → seed 空串 |
| 2 | 主区全空白、无从开聊 | 0 会话时 `ChatPanel` `sessions.length===0` 直接 `return null`，欢迎页没机会渲染（后端重启清空会话踩中） | 改成仅"首次加载中"返 null；加载完即使 0 会话也渲染欢迎页 |
| 3 | 删最后一个会话后状态悬挂 | `closeTab` 在无 tab 剩余时不清 `activeSessionId`，悬指已删会话 | 关活跃 tab 回退到邻居或 `""`（欢迎页） |
| 4 | Skills/Agent docs 视图报错 | 后端把 `listSkills/listAgentDocs` 改成 gated，前端直接抛错 | 数据 provider 捕获 `capability_not_negotiated` → 返 `[]`（空态） |
| 5 | 发消息后**自己的气泡不显示**，重载历史才出现 | **后端 run 流不回传 userMessage item**，live 视图无事件来源 | 发送时**乐观渲染**用户气泡（local id；重开按 `items.list` 替换，无重复） |
| 6 | 中文输入法回车重复/误发("你好啊"→"你你好啊") | `onKeyDown` 未拦 IME 合成期 Enter | `e.nativeEvent.isComposing` 时直接 return |

---

## 4. 给后端的待办（做了 UI 立刻能用）

按价值排序：

1. **在 run 流里发 userMessage item**（`item.started`/`item.completed`）。这是 bug #5 的"正确"解：用户消息是 run 的首个 Item，应随流投递,前端就能从事件渲染 + 拿到真实 item id（而非靠乐观渲染兜）。**后端实现后请告知**,我把前端改成"乐观 + 按 id 去重"。
2. **`workspace.getDiff` / `grep` / `getFileHead`** —— 解锁 Diff / 搜索 / 文件预览（且能删掉前端遗留的 REST 影子）。
3. **`providers.configure` / `providers.test`** —— providers 面板的配置 + 连接测试。
4. **`sessions.update` / `sessions.fork`** —— 会话重命名 / 复制。
5. **开 `features.skills`**（或实现 `listSkills/listAgentDocs`）—— Skills / Agent docs 视图填数据。

---

## 5. 其它观察（非阻塞）

- **assistant 名字硬编码 "Sonnet 4.5"**（`defaultRoles` 的 `MESSAGE_ROLE` displayName），与真实模型 `deepseek-v4-flash` 不符。待决定:跟随真实模型名 / 改中性 "Assistant"。
- **terminal 视图是纯 mock**（协议无 terminal 方法），按约定保留作设计占位。
- **diff/grep/file-head 仍走 REST 影子**（`defaults` 的 `HTTP_KEYS`），真后端无此路由 → 这几个视图当前不可用,等第 4 节第 2 项后端方法就绪一并迁到 JSON-RPC 并删影子。

---

> 后端补齐第 4 节后请重跑 `runs.start` 流式（含 userMessage item）+ `items.list` + 对应新方法复核，并更新本文。
