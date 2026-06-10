# 后端新增能力 · 前端对接说明 · `2026-06-10`

> 后端（lyra）本轮把一批此前 `capability_not_negotiated` 的协议方法**真正接通**了。本文告诉前端：
> **哪些方法现在可以调、怎么调、有哪些边界**，以及**哪些仍未实现**（别提前对接，会拿到 `capability_not_negotiated`）。
> §x.y 指 [`../API.md`](../API.md)。
>
> **第一原则不变**：`runtime.initialize` 返回的 `ServerCapabilities`（§9）始终如实反映后端真实接线 —— features 标 `true`
> 的才有，标 `false` / 缺席的调了会被诚实拒绝。**前端一律按这份快照协商，不要硬编码方法可用性。**

---

## 1. 本轮新增（7 个方法，已可对接）

| 方法 | §节 | 能力 | 关键边界 |
| --- | --- | --- | --- |
| `workspace.getFileHead` | §7.5 | 读 cwd 内文件前 N 行 | 见 §3.1 |
| `workspace.grep` | §7.5 | 正则搜索工作区（ripgrep 后端） | 见 §3.2 |
| `workspace.listSkills` | §7.5 | 列出 cwd 可见的 Agent Skills | 见 §3.3 |
| `workspace.mcp.listTools` | §7.5 | 按 server 列 MCP 工具 | 见 §3.4 |
| `sessions.update`（`cwd` / `metadata`） | §7.2 | relocate 改 cwd + metadata 全替换 | 见 §3.5 |
| `runs.start` `mode:"chat"` | §7.3 | tool-less 单轮对话 | 见 §3.6 |
| `sessions.fork` | §7.2 | **整段**复制会话分叉 | 见 §3.7 |

## 2. Capability 快照的变化（§9）

`initialize` 返回的 `features` 里，这两位从 `false` 翻成 **`true`**：

```jsonc
{
  "features": {
    "skills": true,      // ← 新：workspace.listSkills 可用
    "relocate": true,    // ← 新：sessions.update 改 cwd 可用
    "memory": true,      // （取决于部署是否配 memory service）
    "mcp": true,
    "reasoning": true
    // 仍为 false 的见 §4
  }
}
```

> `workspace.getFileHead` / `grep` / `mcp.listTools` / `sessions.fork` / `runs.start mode=chat` 都是**核心 §7.5/§7.2/§7.3
> 方法，不受 feature 门控** —— 协商通过后直接可调，快照里没有对应开关位。

---

## 3. 逐方法对接细节

### 3.1 `workspace.getFileHead`

- 入参 `{ cwd?, path, lines? }`。`path` **相对 cwd**（缺省 cwd = serve 目录）。`lines` 缺省 **200**。
- 返回 `FileHead { path, lines: FileLine[] }`，`FileLine = { lineNumber, text }`，**lineNumber 从 1 起**，纯文本（前端自己高亮）。
- 错误：
  - `path_outside_root` —— `path` 用 `../` 或绝对路径逃出 cwd。**已做词法越狱守卫**，前端不用自己校验，但要能展示这个错。
  - `cwd_unavailable` —— cwd 不是个存在的目录。
  - 二进制文件 → 内部错误（暂不细分；后续可加专用码）。

### 3.2 `workspace.grep`

- 入参 `{ cwd?, query, path?, limit? }`。`query` 是**正则**；`path` 可选地把搜索范围缩到 cwd 下的子路径（同样 jail）；`limit` 缺省 **100**。
- 返回 `GrepResult { matches: GrepMatch[], total }`，`GrepMatch = { path, lineNumber, text }`。
- **截断语义（重要，§7.5 no-silent-caps）**：`matches` 最多 `limit` 条；**`total` 可能 > `matches.length`**，表示"还有更多，请缩小查询"。前端判断截断就看 `total > matches.length`，**不要假设 `total == matches.length`**。
- 依赖宿主有 `rg`（或 `grep` 兜底）；都没有时返回内部错误。
- 错误：`query` 为空 → `invalid_params`；`path` 逃逸 → `path_outside_root`；cwd 无效 → `cwd_unavailable`。

### 3.3 `workspace.listSkills`

- 入参 `WorkspaceQuery { cwd? }`（缺省 serve 目录）。返回 `Page<Skill>`，`Skill = { name, description?, source? }`。
- **`source` 字段语义**：标注该 skill 来自哪一层 —— `"project"`（`<cwd>/.lyra/skills`）或 `"global"`（用户级目录）。同名时 **project 覆盖 global**（与后端喂给模型的优先级一致）。
- 没有任何 skill 时返回空页（不是错误）。

### 3.4 `workspace.mcp.listTools`

- 入参 `{ server?, cursor?, limit? }`。**MCP 是 runtime 全局，不收 cwd。**
- 返回 `Page<McpTool>`，`McpTool = { server, name, description?, inputSchema? }`。
  - **`name` 是裸工具名**（不带前缀）；`server` 单独给。模型侧看到的是 `"<server>_<name>"`，但这里两个字段分开，前端按需自己拼。
  - `inputSchema` 是该工具的 JSON Schema（object）。
- `server` 省略 = 列所有已连接 server 的工具；给了未知名字 = 空列表（运行中的 runtime 只认启动时连上的 server，先用 `workspace.mcp.listServers` 拿名字）。

### 3.5 `sessions.update` —— `cwd`（relocate）+ `metadata`

- **relocate**：`{ sessionId, cwd }`。把会话的工作目录改到新路径，**之后的 run 在新 cwd 解析工具/记忆，已有历史不动**。
  - 后端**校验新 cwd 是存在的目录**，否则 `cwd_unavailable`（防止改到坏路径后续 run 静默崩）。
  - 受 `features.relocate` 门控（现已 `true`）。
- **metadata**：`{ sessionId, metadata }`，**全替换**（不是 merge）。
  - **`metadata` 是任意 JSON object**（值可以是 string / number / bool / 嵌套对象）—— 后端已按契约 §4.1 忠实存取，往返不丢类型。`null`/省略则不动该字段。
- `title` / `model` 仍如常（早已支持）。返回更新后的 `Session`。

### 3.6 `runs.start` `mode: "chat"`

- 之前 `mode:"chat"` 会被拒；**现在是 tool-less 单轮对话** —— 模型拿不到任何工具（无 fs/bash、无 `task` 委派、无 skill），就是一次纯 LLM 应答。
- 其余入参/流式事件与 `mode:"agent"` 完全一致（同一条 `runs.start` Stream，§7.3）。
- `mode:"plan"` / `mode:"agent"` 不变；未知 mode → `invalid_params`。

### 3.7 `sessions.fork` —— ⚠️ 只支持整段 fork

- **支持**：`{ sessionId, title? }`（**不带 `fromItemId`**）。后端创建一个子会话，**继承源 cwd + 复制源的完整聊天历史**，然后各自演化。`title` 可覆盖默认的 `"<原标题> (fork)"`。返回新 `Session`。
- **暂不支持**：`fromItemId`（"从某 item 边界分叉"）。给了会返回 **`checkpoint_unavailable`**。
  - 原因：持久 Item 日志（`item_<run>_<seq>`）和聊天消息日志是两条平行表示、无位置关联，"截到某 item 之前"目前无法可靠求解（需要 checkpoints 那块数据模型，见 §4）。
  - **前端对接建议**：fork 入口先只做"复制整个会话";"从这条消息分叉"的 UI 等 checkpoints 落地再开。

---

## 4. 仍未实现（调了会拿到 `capability_not_negotiated` / 对应 feature 仍 `false`）

**别提前对接这些**，按 capability 快照走即可。按缺口性质分组：

| 方法 | 状态 | 缺口性质 |
| --- | --- | --- |
| `workspace.getDiff` | ❌ | 需引入 git（仓内尚无 git 能力） |
| `workspace.listFileChanges` | ❌ | 同上，git 工作树状态 |
| `attachments.createUploadUrl` / `get` / `delete` | ❌ `attachments:false` | 需 blob 存储 + 上传 URL + transport 文件通道 + multimodal |
| `sessions.export` | ❌ `sessionExport:false` | 需 transport 文件通道（与 attachments 共用） |
| `items.edit` | ❌ `checkpoints:false` | 需 message↔item 关联（数据模型补丁） |
| `sessions.fork` 的 `fromItemId` | ❌ → `checkpoint_unavailable` | 同 `items.edit`（整段 fork 已可用） |
| `background.list` / `subscribe` / `cancel` | ❌ `background:false` | 需 client 可见的后台任务注册表（服务层） |
| `workspace.mcp.reconnect` | ❌ | 需 MCP 会话可重连句柄 + 状态机 |
| `feedback.create` | ◐ 接受但不持久化 | 无 readback、无消费者（设计决策，非缺陷） |

> **优先级（后端视角，供前端排期参考）**：git 的 `getDiff`/`listFileChanges`（代码视图面板）> checkpoints（`items.edit` + 按点 fork）> attachments（多模态）> background > mcp.reconnect。前端如有不同优先诉求，回我们调整。

---

## 5. 对接 checklist（给前端）

1. **以 `initialize` 的 `ServerCapabilities` 为准**判断方法可用性，别硬编码。
2. 本轮可立即接：**文件预览（getFileHead）、代码搜索（grep）、skill 列表（listSkills）、按 server 列 MCP 工具（mcp.listTools）、改 cwd/metadata（sessions.update）、tool-less 聊天（mode=chat）、整段复制会话（fork）**。
3. 注意三个边界：**grep 的 `total` 截断信号**、**fork 的 `fromItemId` 暂不支持**、**relocate 改坏路径返 `cwd_unavailable`**。
4. §4 的清单**不要对接**；想推进哪块（尤其 git diff / checkpoints）跟后端约时间。

有问题或要补字段/补错误码，直接在本目录提 issue 文档或回后端。
