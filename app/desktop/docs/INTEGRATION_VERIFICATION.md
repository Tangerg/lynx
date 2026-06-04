# Lyra 前后端对接验证报告（UI 层）

> **日期**：2026-06-04
> **被测后端**：Lyra Runtime（`serverInfo.name = "runtime"`），构建 `6d26db7`，监听 `http://127.0.0.1:17171`，**streamable HTTP** transport，`protocolVersion 2026-06-03`。
> **被测前端**：本仓库 frontend（Vite dev，HTTP transport 直连 :17171），对接批 `560b9f4` · `fb860b4` · `cb78953` · `25d3424`。
> **范围**：把 UI 各功能接到真实后端（会话生命周期、历史、流式、多 provider/model、providers 配置…），记录对接暴露的前端 bug + 仍存的后端缺口。
>
> 本文是**对当前后端构建**的对接快照，不是契约。契约以 [`API.md`](./API.md) / [`TRANSPORT.md`](./TRANSPORT.md) 为准；后端缺口补齐后请重测并更新。后端逐项回应见 [`INTEGRATION_VERIFICATION_BACKEND_RESPONSE.md`](./INTEGRATION_VERIFICATION_BACKEND_RESPONSE.md)。

---

## 0. 结论

**核心链路 + provider/model 装配链路均端到端打通**，全部用真实前端 transport 对真实后端验证：握手 → 多 provider/model 选择 → 流式对话（真实模型）→ userItemId 精确对账 → 历史 → 删会话；providers 配置/探活已实测。

对接共发现并修复 **14 个纯前端 bug**（§3，按层分组）。后端这轮补齐了上版反馈里 3 项最关键的（userItemId、providers.configure/test、多 provider×model），前端已对齐并验证。剩 **3 类后端方法**未实现（§4）。

> **命名定调**：引用 provider 的 wire 参数统一为裸名 **`provider`**（非 `providerId`），与 `model` / `Model.provider` 一致——这是 `API.md §7` 与**运行中后端**的实际口径（后端回应文档正文有两处仍写 `providerId`，是笔误；前端按 `provider` 实现并实测通过）。
>
> **贯穿性教训**：后端覆盖面随构建变动，前端必须对 `capability_not_negotiated` 优雅降级，UI shell 必须区分 loading / empty / **error** 三态。

---

## 1. 已验证通过（真实前端代码 + 真实后端 E2E）

| 流程                  | 方法                                  | 结果                                                                                                                                  |
| --------------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| 握手                  | `runtime.initialize`                  | ✅ `runtime` / `2026-06-03`                                                                                                           |
| 会话 建/列/删         | `sessions.create` / `list` / `delete` | ✅ `ses_…`、`cwd=/Users/tangerg`；删后列表减少、侧栏右键可删                                                                          |
| **provider 列表**     | `providers.list`                      | ✅ 4 个（anthropic / deepseek / moonshot / openai），`apiKeyMasked==""` 即未启用（deepseek 已启用）                                   |
| **per-provider 模型** | `models.list{provider}`               | ✅ deepseek 返 4 个模型（带 displayName）；**`models.list({})` → `[]`**（per-provider 契约）                                          |
| **provider 探活**     | `providers.test{provider}`            | ✅ `deepseek` → `ok=true`（真实 `max_tokens=1` 探活）                                                                                 |
| **流式 + 选模型**     | `runs.start{provider,model}`          | ✅ 配对生效，`run.started → item.started(userMessage) → item.completed(userMessage) → agentMessage delta×N → run.finished{completed}` |
| **userItemId 对账**   | `runs.start` 响应                     | ✅ 响应带 `userItemId`，且 **== 流上 `item.started(userMessage)` 的 id**（精确对账成立）                                              |
| 历史                  | `items.list` → reduce 重建            | ✅ 重建出 `user[text]                                                                                                                 | assistant[text]` |

---

## 2. 后端覆盖矩阵

### ✅ 已实现可用

`runtime.initialize` · `sessions.create/list/delete` · `runs.start`（**provider+model 配对**、返回 **userItemId**）/ `resume` / `cancel` · `items.list`（含 run 流回显 userMessage Item）· `workspace.listProjects/listFileChanges/listAgentDocs/mcp.listServers` · **`providers.list/configure/test`** · **`models.list{provider}`**

### ❌ `capability_not_negotiated`（未实现 / 能力关闭）—— 阻塞对应 UI

| 方法                                         | 阻塞的 UI              | 后端给出的原因                                                                                                    |
| -------------------------------------------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `workspace.getDiff` / `grep` / `getFileHead` | Diff / 搜索 / 文件预览 | 随 agent 工具（bash / file r·w·edit / grep / glob + git prompt）一并落地，届时接 JSON-RPC（前端同时删 REST 影子） |
| `sessions.update`（改名 / 换 cwd）           | 会话重命名 / relocate  | `session.Service` 无 `update` 动词；新增属破坏性公开 API，需先确认                                                |
| `sessions.fork`（复制）                      | 会话复制               | 已有 `Fork`，但"按 item 边界 fork"需先对齐 checkpoint / item-id 模型与 engine history                             |
| `workspace.listSkills`                       | Skills 视图            | engine 尚未实现 skill 发现，故 `features.skills:false`                                                            |

### 未接（按 `features` 关闭，低优先）

`attachments`（`enabled:false`）· `background`（`false`）· `items.edit`（`checkpoints:false`，重试）· `memory`（`true`，可接但暂无 UI）· `feedback`（未门控）

---

## 3. 对接中发现并修复的前端 bug（14 个，按层分组）

### 协议 fold 层（`protocol/run/` · `builtin/agent/core-reducer/`）

| 现象                              | 根因                                                                                                                                                                 | 修复                                                                                                                                 |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| 内容区不流式，只在最后一次性出现  | `item.started` 壳只带 `ItemBase`，body（content/text/steps/question/tool）走 delta；`contentText(undefined).filter` 等崩溃，被 reducer try/catch 吞掉 → 整块永不渲染 | 全部 6 类 item 容忍 body-less 壳（seed 空串 / `?? []` / 占位 label），started 折叠成空块由 delta 填                                  |
| 流偶发整条挂掉（malformed event） | `RunTree.admit` 在无 try/catch 的 subscribe 回调里裸解引用 `event.run` / `event.item`（Zod 只校验了 `type`）                                                         | 边界把 run/item 当可缺失：脏数据更新无效并丢弃，绝不抛                                                                               |
| 发消息后自己的气泡重复            | 后端流式真实 id 的 userMessage 与本地 `local-*` 乐观气泡共存；`appendUserMessage` 仅按 id 去重                                                                       | **乐观 + userItemId 精确对账**：runs.start 一解析就把占位气泡 id 升级为 `userItemId`，流上 Item 按 id 去重；并保留内容文本对账作兜底 |

### 状态层（`state/`）

| 现象                                                | 根因                                                                                               | 修复                                                   |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| 删最后一个会话后状态悬挂                            | `closeTab` 在无 tab 剩余时不清 `activeSessionId`                                                   | 回退邻居或 `""`（欢迎页）                              |
| 草稿引用无限增长 + 重开错跳历史                     | `draftSessionIds` / `pendingMessages` 关/删会话时从不清                                            | 一个 `tabIds`-diff 订阅兜住所有关闭路径，剔除已离场 id |
| A 会话的工具选择/展开/文件焦点串进 B                | `selectedToolId` / `expandedToolIds` / `activeFile` 注释称"session-scoped"实为全局单值，切会话不清 | `selectTab` 在会话真正切换时清掉三者                   |
| HITL：解决一个审批，同批未解决的兄弟 interrupt 消失 | `resolveInterrupt` 用 `some` 把整个 OpenInterrupt 信封删掉                                         | 粒度过滤：只删命中的 interrupt，信封空了才删           |

### UI 壳（`components/` · 各 view 插件）

| 现象                           | 根因                                                               | 修复                                                         |
| ------------------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------ |
| 主区全空白、无从开聊           | 0 会话时 `ChatPanel` 直接 `return null`（后端重启清空会话踩中）    | 仅"首次加载中"返 null；加载完即使 0 会话也渲染欢迎页         |
| 后端查询失败被伪装成"暂无数据" | `DataView` 只有 loading/empty 两态，rejected query 落入 empty 分支 | 加 `isError` 错误态 + `alert` 图标；9 个消费方透传 `isError` |
| 插件 view 卸载后留下空白死 tab | `WorkspaceViewBody` 对未注册 view id 返 `null`                     | 渲染 "View unavailable" fallback                             |
| 侧栏会话计数闪 `0`             | 计数 pill 在首帧 `data===undefined` 时显示 0                       | 首次 fetch 落定前隐藏 pill                                   |

### 输入 / 边界容错

| 现象                    | 根因                                         | 修复                                                             |
| ----------------------- | -------------------------------------------- | ---------------------------------------------------------------- |
| 中文输入法回车重复/误发 | `onKeyDown` 未拦 IME 合成期 Enter            | `e.nativeEvent.isComposing` 时直接 return                        |
| Skills 视图报错         | 后端把 `listSkills` 改成 gated，前端直接抛错 | 数据 provider 捕获 `capability_not_negotiated` → 返 `[]`（空态） |

> 另：`models.list` 改为 per-provider 后，旧的 `models.list()`（无 provider）会返空、清空模型选择器——已把 `models` data provider 改成跨**已启用** provider 聚合（`cb78953`）。

---

## 4. 给后端的待办（做了 UI 立刻能用）

> 上版的 #1 userItemId、#2 providers.configure/test、多 provider×model 已全部完成并验证（§1）。剩余：

1. **`workspace.getDiff` / `grep` / `getFileHead`** —— 解锁 Diff / 搜索 / 文件预览，并删掉前端遗留的 REST 影子。
2. **`sessions.update` / `sessions.fork`** —— 会话重命名 / 复制。
3. **开 `features.skills`**（或实现 `listSkills`）—— Skills 视图填数据。

---

## 5. 前端缺口全清单（按可做性分类）

状态记号：**E** 端到端可用 / **A** UI 在但未接后端 / **B** UI 在、调后端但方法未实现 / **C** mock / 简化占位 / **D** 未建 UI。

### 5.1 ✅ 已端到端可用（E）

握手 · 会话 建/列/删 · 流式对话（含 userMessage 回显 + **userItemId 精确对账**）· **多 provider/model 选择**（composer 选择器跨已启用 provider 聚合）· **providers 配置/探活**（Providers 面板 key/baseUrl 输入 + Save/Test）· 停止（`runs.cancel`）· HITL 审批/提问（`runs.resume`）· 消息复制 · `listProjects/listFileChanges/mcp.listServers/listSkills/listAgentDocs`。

### 5.2 🟢 现在就能做（后端已就绪 / 纯前端）

| 缺口                 | 现状                                                    | 要做的                                                            |
| -------------------- | ------------------------------------------------------- | ----------------------------------------------------------------- |
| assistant 名字真实化 | 硬编码 "Sonnet 4.5"                                     | 读 `models.list.displayName` / run `usage.byModel` 渲染真实模型名 |
| **memory 面板**      | `memory.*` 已实现 + `features.memory:true`，但无任何 UI | 建 memory 设置面板 / 视图                                         |
| **feedback 入口**    | `feedback.create` 已就绪、未门控，但无任何 UI           | 消息级 👍/👎 或反馈表单                                           |
| plan 头部 goal/ETA   | hardcoded（`plan.tsx` TODO）                            | 接 agentStore 真实 run 派生                                       |

### 5.3 🟡 等后端方法 / 能力（B / 受 feature 门控）

| 缺口                        | 阻塞于                                                                      | 前端现状                                                                              |
| --------------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| 会话 **重命名 / 复制**      | `sessions.update` / `sessions.fork`                                         | **UI 未建**（SessionRow 右键菜单只有 Delete）                                         |
| **Diff / 搜索 / 文件预览**  | `workspace.getDiff` / `grep` / `getFileHead`                                | 视图在，但走坏的 REST 影子 → 对真后端不可用                                           |
| **附件 选择/上传**          | `features.attachments`（现 `enabled:false`）+ `attachments.createUploadUrl` | composer 有 📎 按钮但无 onClick / 无文件选择器（A）                                   |
| **terminal 真实数据**       | 协议无 terminal 方法                                                        | 纯 mock（C），保留作设计占位                                                          |
| **background tasks 真实流** | `features.background`（现 `false`）                                         | tasksStore + pill 在，只服务本地插件任务，未接 `notifications.background.update`（C） |
| **Skills 数据**             | engine 未实现（`features.skills:false`）                                    | 视图在，gated 返空                                                                    |

### 5.4 🔵 功能在但是简化实现（C，非阻塞、可改进）

| 项                     | 当前实现                 | 说明                                                                                    |
| ---------------------- | ------------------------ | --------------------------------------------------------------------------------------- |
| 消息 **编辑 / 重生成** | 文本回填 composer + 重发 | 非协议级 `items.edit`（从 item 边界分支续跑）；`items.edit` 受 `checkpoints:false` 门控 |

---

## 6. 其它观察（非阻塞）

- **terminal / plan 视图头部是 mock**（各带 `// TODO`），body 走真实 provider，头部元数据无来源。
- **diff/grep/file-head 仍走 REST 影子**（`defaults` 的 `HTTP_KEYS`），真后端无此路由（API.md §9.3 禁业务 read shadow）→ 等 §4 #1 的 JSON-RPC 方法就绪后迁移并删影子。
- **后端回应文档笔误**：`INTEGRATION_VERIFICATION_BACKEND_RESPONSE.md` 新增段落正文有两处把参数写成 `providerId`，与其自身的命名变更表 + `API.md` 不一致；正确为 `provider`（前端已按此实现并实测）。

---

> 后端补齐第 4 节后请重跑对应方法复核，并更新本文。
