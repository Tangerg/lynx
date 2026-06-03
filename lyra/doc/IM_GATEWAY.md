# IM_GATEWAY.md — 第三方 IM 接入(Slack/飞书/钉钉/微信/TG…)的心智模型与基准

> 定位:lyra 怎么接第三方 IM(让用户在 Slack/飞书/钉钉/微信里和 agent 对话)。
> 结论:**IM gateway 是"又一个协议 client",不进 runtime**。配套 [`WORKSPACE_MODEL.md`](./WORKSPACE_MODEL.md)。
> 结论来自横向调研 5 个项目(2026-06)。项目级约定见 [`../../CLAUDE.md`](../../CLAUDE.md)。

---

## 0. 一句话心智模型(TL;DR)

> **IM gateway = 一个独立进程,作为 Lyra Runtime Protocol 的普通 client**(和 Wails 桌面、Web、TUI 平权)。
> 它持有 IM 鉴权(bot token)+ `IM-chat ↔ session` 映射,调 `sessions.create(cwd)` / `runs.start`,
> 流式收 AG-UI 事件渲染回 IM。**runtime 零改动、零新概念、零 user 概念**——IM gateway 就是 lyra 一直说的"外层 app"。

```
  Slack / 飞书 / 钉钉 / 微信 ── bot/webhook/websocket
        │  (IM 鉴权、消息收发都在 gateway)
        ▼
  ┌─────────────────────┐      Lyra Runtime Protocol (HTTP loopback :17171)
  │   IM Gateway 进程     │ ───────────────────────────────────────────────►  ┌──────────────┐
  │  chatId ↔ sessionId  │      sessions.create(cwd) / runs.start             │ lyra runtime │
  │  渲染 AG-UI → IM 消息 │ ◄───────────────────────────────────────────────  │  (无状态)     │
  └─────────────────────┘      notifications/run/event (AG-UI 流)             └──────────────┘
        ▲                                                                       和 桌面/Web/TUI client 同一接口
  和桌面/Web client 完全平权
```

---

## 1. 调研基线(5 个项目,2026-06)

| 项目 | 平台 | 架构 | 角色 |
|---|---|---|---|
| **Proma** | 飞书 / 钉钉 / 微信 | **进程内 bridge**(Electron main):Lark SDK WebSocket / 钉钉 Stream API / 微信 iLink 长轮询 | 前端 + 工具 |
| **cherry-studio** | **Slack/TG/飞书/Discord/微信/QQ**(最全) | **进程内 adapter**,统一 `ChannelAdapter` 基类,懒加载,官方 SDK | 前端 |
| **opencode** | Slack | **独立 gateway 进程**(`packages/slack`),Socket Mode,**用 SDK 当普通 client 连 server** | 前端 |
| **AionUi** | 飞书/TG/钉钉/微信(仅配置 UI + 扩展示例) | 插件框架,真适配器在 extension | 配置 UI |
| **codex / claude_code** | ❌ 无(codex 仅 schema fixture;claude_code 仅跳转链接到官方 Slack app) | — | — |

---

## 2. 两种架构,为什么 lyra 选"独立 gateway client"

| | **A. 进程内适配器** | **B. 独立 gateway 当协议 client** |
|---|---|---|
| 代表 | Proma / cherry-studio | **opencode** |
| 形态 | IM bridge 长在 app 主进程,和 agent 紧耦合 | IM bot 是单独进程,经 SDK/协议连 server,与其它 client 平权 |
| 前提 | **没有** server/协议边界(Electron 内 agent 进程内跑) | **有** server/协议边界 |

**lyra 选 B**,因为:
- **lyra 已经有 opencode 那个边界**(Lyra Runtime Protocol + 无状态 server + 多 client),Proma/cherry-studio 没有,所以它们只能塞进程内。
- 对 lyra,IM gateway 就是**又一个协议 client**:`sessions.create` + `runs.start` + 收 AG-UI 流 → 渲染回 IM。**runtime 完全不用改。**
- **与 CLAUDE.md 反向不变量自洽**:"后端做用户鉴权/账号/多租户 ❌ —— 鉴权由更外层解决"。**IM gateway 就是那个"外层"**:它持 bot token、做 IM 鉴权、管 chat↔session 映射;runtime 仍零 user 概念。
- 同一套协议让 IM gateway / Web / TUI / 桌面 client **全部平权接入**——这正是协议层薄、业务层厚的回报。

**别学 A**:不要把 IM adapter 塞进 lyra-core / runtime。

---

## 3. IM-chat ↔ session 映射(所有家一致 + 套进 lyra 模型)

- **1:1**:一个 IM 会话(飞书 `chatId` / Slack thread `ts` / 钉钉 `conversationId`)= 一个 lyra **session**。
- **映射住在 gateway,不在 runtime**:Proma 用 `chatBindings: Map<chatId, {sessionId, workspaceId}>`(内存 map)。lyra 的 gateway 同样自己存 `chatId ↔ sessionId`(内存或小持久化)。
- **cwd 由 gateway 在建会话时定**(套用 [`WORKSPACE_MODEL.md`](./WORKSPACE_MODEL.md) 的 `sessions.create(cwd)`):Proma 是 per-bot `defaultWorkspaceId`;lyra gateway 可让"研发助手 bot 固定绑 `/repo-a`"。
- **流式渲染**:gateway 累积 AG-UI 的 `TEXT_MESSAGE_*` / `TOOL_CALL_*` / `REASONING_*` 事件,渲染成 IM 消息——飞书用交互卡片增量 `patch`,Slack/钉钉/微信用文本累积更新。lyra 的事件流**已经给够**。

---

## 4. 两个方向(Proma 都做了)

- **IM 当前端(主)**:用户在 IM 里和 agent 对话。→ gateway-as-client。**lyra 协议侧零缺口。**
- **IM 当工具(次)**:agent 能读/发 IM。Proma 给飞书群**注入 per-session MCP server**(`fetch_group_chat_history` 工具),走 `customMcpServers`。
  - lyra 对应物 = **per-session MCP/工具注入**。这是唯一"要支持 IM-as-tool 才需补"的能力。
  - lyra 现状:MCP 是**启动时全局配**(`LYRA_MCP_SERVERS`),**无 per-session 注入**。协议 `StartRunRequest.Tools` 字段已预留位置。低优先。

---

## 5. 决策与缺口

### 决策
- **G1**:IM 集成 = 独立 gateway 进程,作为 Lyra Runtime Protocol 的 client。**不进 runtime / 不加 runtime 概念。**
- **G2**:`IM-chat ↔ session` 1:1 映射,**映射 + IM 鉴权 + bot token 全在 gateway**,runtime 保持零 user。
- **G3**:每个 IM session 的 cwd 由 gateway 在 `sessions.create(cwd)` 时决定(per-bot 默认文件夹)。
- **G4**:gateway 消费 AG-UI 事件流渲染 IM 消息(飞书卡片 patch / 其它文本累积)。
- **G5**:gateway 是**后续独立项目**,等协议稳定再建;近期 lyra-core **不为 IM 做任何改动**。

### 缺口(仅 IM-as-tool 方向需要)
- **per-session MCP/工具注入**:让 gateway 给某个 session 临时塞 IM 专用工具(如拉群历史)。先例 Proma `customMcpServers`,协议 `StartRunRequest.Tools` 已留位。front-end 方向**不需要**,故低优先。

### 协议侧确认(front-end 方向所需,基本已具备)
- `sessions.create(cwd)`(见 WORKSPACE_MODEL §6)、`runs.start` + `notifications/run/event` 事件流、`runs.cancel` —— headless client 跑起来够用。
- gateway 连的就是现有 HTTP loopback transport(`127.0.0.1:17171`);若 bot 要常驻服务器,是部署问题,协议不变。

---

## 6. gateway 内部抽象(参考,非 runtime 关心)

cherry-studio 的统一 `ChannelAdapter` 基类(`sendMessage` / `sendTypingIndicator` / streaming hook + 懒加载 `adapterImportMap`)是个好的 **gateway 内部** 形状:每个平台一个 adapter,共享"收消息 → 调 runtime → 流式渲染回"。但这属于 gateway 工程,**不是 lyra runtime 的职责**。

---

## 一句话收尾

**因为 lyra 是"协议 + 无状态 server + cwd-per-session + 零 user 概念",IM 接入天然是"又一个协议 client"(opencode 已证明可行)。** runtime 不动,gateway 在外层持鉴权与映射、按 bot 选 cwd、渲染 AG-UI 流——这又一次验证了 [`WORKSPACE_MODEL.md`](./WORKSPACE_MODEL.md) 那套模型选对了。
