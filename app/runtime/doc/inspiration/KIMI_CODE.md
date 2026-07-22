# Kimi Code 启发的能力吸纳 Backlog

> **来源**：对 **Kimi Code CLI**（`@moonshot-ai/monorepo`，Moonshot 的终端 coding agent，pnpm/TS monorepo；桌面克隆 `~/Desktop/kimi-code`）的源码级对比分析。**非 gemini-cli fork**——表面像（README/GEMINI 式 memory、`/`-命令、`@`-文件、subagents、ACP），实为自研引擎栈：`agent-core`(v1)/`agent-core-v2`(DI×Scope)、自研 LLM 层 `kosong`、自研存储 `minidb`、自研执行环境 `kaos`。方法与 [`GROK.md`](GROK.md) 一致；跨应用总索引见 [`README.md`](README.md)。
>
> **状态**：全部 proposed。已跳过 parity 项（subagents/swarm、hooks、`/`-命令、`@`-文件、plan、compaction、per-session cwd、background commands、MCP gating、telemetry）——lyra 均 match 或 lead。

---

## 0. 五个 distinctive 子系统（先定位）

Goal mode（自主多轮驱动）· KAOS（fs/process/path OS 抽象，local+SSH）· plugin bundle + marketplace + trust tier · ACP adapter（编辑器嵌入）· AI-native config skill。其中**只有 Goal mode 是对 lyra 的大 gap**，其余部分吸/低优先。

## 0.1 筛选准则

沿用 lynx 四道筛子（取思想不取形态 / 不为多租户云 / 不引双机制债 / 薄核优先）+ 反向不变量（无 retry-layer；用户填 provider key、不做 LLM-provider OAuth；runtime 协议无 stdio）。详见 [`GROK.md` §0.1](GROK.md)。

---

## 1. 吸纳清单

### K1 · Goal mode —— 受监督的自主多轮执行循环（P1，头号）

- **来源/为什么**：`GOAL.md`；`packages/agent-core/src/agent/goal/`（状态机）；`agent/turn/index.ts` `driveGoal()`（续跑循环）；`tools/builtin/goal/{create,update,get,set-goal-budget}.ts`；`agent/permission/policies/goal-start-review-ask.ts`。goal 是**运行时拥有的 typed 结构化状态**（非 chat 文本），每 agent 一个、从 record log 重建，刻意最小 4 态机：`active`（唯一推进态）/ `paused`（用户或可重试运行时停）/ `blocked`（模型/预算/hook 僵局，可恢复）/ `complete`（瞬态：announce 后即清、绝不落盘长驻）。`driveGoal()` 跑一个普通 turn → 读模型经 `UpdateGoal` 设的状态 → 若仍 `active` 就**重注入 continuation prompt**（"朝目标继续、挑一个 bounded slice、别在 plan/partial 上标 complete、同一 blocker 连续 3 turn 才算 blocked"）并跑下一 turn——**自主替代用户反复敲"continue"**。turn/token/wall-clock **预算是 opt-in 硬顶**（每 turn/每模型 step 前后检查 → 破顶即 `blocked`）。失败分类有原则：interrupt/rate-limit/provider/runtime error → `paused`；模型声明僵局/预算/hook → `blocked`。跨重启：replay 时 `active` goal **降级为 paused**（旧 turn 不可能还在跑，绝不静默 resume-and-burn）；fork 不继承 goal；模型发起的 `CreateGoal` **HITL 门控**（菜单选自主运行的 permission mode）。
- **目标**：给 lyra 一个"干到完成"的受监督自主循环——用户不必每轮敲 continue，但入口有审批门、有预算硬顶、重启安全。
- **落点**：`app/runtime` 执行核心（turn 循环外包一层 `GoalDriver`）+ `execution.*` 状态 + goal 工具（create/update/get/set-budget）+ 桌面 "goal box"（状态/预算/统计）。复用 lyra 现有 approval Service（goal-start 门）、durable-resume（active→paused 降级 ≈ lyra "protected re-binds on re-run"）、usage（预算记账）。
- **计划**：① 定 4 态机 + goal 结构化状态（从现有 snapshot/record 重建）② `GoalDriver` 包 turn 循环、读模型 stop-signal、active 则重注入 continuation prompt ③ opt-in 预算（turn/token/wall-clock）硬顶 → blocked ④ 入口 HITL 门 + 选 permission mode ⑤ 重启降级 active→paused ⑥ 桌面 goal box。
- **验收**：给一个多步目标，agent 自主连跑至 complete/blocked；标 complete 必须靠机器信号（模型 prose 说"done"不算）；破预算即 blocked；重启后 active goal 变 paused 不自动烧；入口有审批。
- **风险/边界**：**保持为独立机制、别折进 steer/plan/scheduler**（filter #3）——它与三者正交（steer=mid-run 转向、plan=只读规划、scheduler=cron headless）。设计纪律是价值本身：无 `cancelled`/`error`/`impossible` 态（塌进 paused+reason / blocked+reason / clear）；completion 需机器信号；goal 注入只在 turn 边界（prompt-cache 友好）。预算硬顶超出 lyra 现有 `usage`（只观测不强制）。
- **优先级**：**P1（本对照头号，也是本轮所有对照里少见的"大而干净"能力 gap）**。

---

### K2 · KAOS 执行环境 seam（P2，门控于 C7）

- **来源/为什么**：`packages/kaos/src/kaos.ts`（`Kaos` 接口——"让 agent 经统一 API 与不同执行环境 local/SSH/容器 交互"）；`local.ts`/`ssh.ts` 后端；`current.ts`（AsyncLocalStorage 环境态绑定）。一个接口统一**所有** host 交互——path 风格、cwd/home、`stat`/`iterdir`/`glob`、字节/行/文本读写、进程 spawn，`withCwd()`/`withEnv()` 返回 scoped 派生；`LocalKaos` 跑本机、`SSHKaos` 对远程跑**同一** surface；按 async context 环境态绑定，每个 fs/shell 工具从 context 解析"在哪跑"。
- **目标**：把**执行环境抽成一个可换接口、置于每个 fs+shell 工具之后**（不只 cwd），让沙箱/远程后端成为 drop-in。
- **落点**：`app/runtime` 工具 infra——把现有 per-session-cwd blackboard resolver **泛化**成 `ExecutionEnvironment` 接口（从 context 解析），与 C7 沙箱工程耦合。
- **风险/边界**：**只在 C7 沙箱真正启动时才建这个抽象**（C7 是已 backlog 的可预见扩展、非投机，届时抽象才 justified）；**不建 SSH 后端**（无需求，纯 YAGNI）；Go 用 `io/fs`+`os.FileInfo` 惯用法，不镜像 Python `os.stat_result`。lyra 已有此 insight 的缩影（per-session-cwd resolver），这是把单点 seam 泛化到 fs+process 全面。
- **优先级**：**P2（C7 启动那天升 P1）**。

---

### K3 · capability bundle + 本地 git 安装（P3）

- **来源/为什么**：`packages/agent-core/src/plugin/{manifest,manager,github-resolver,source,...}.ts`。一个 **plugin** = 单 manifest（`kimi.plugin.json`）打包 skills + MCP + hooks + `/`-命令 + `sessionStart` skill，可从 `local-path`/`zip-url`/`github` 安装（github-resolver 走 codeload/redirect 避 API 配额）；`marketplace.json` 列可装 plugin + 信任 `tier`，README 装前显示信任级别。是**引用既有机制的分发信封**、非竞争性运行时机制。
- **目标**：把一组 skill/MCP/hook/recipe 作为**一个单元从 git 分享/安装**，装前显示信任。
- **落点**：`app/runtime`（bundle installer + manifest 展开进现有各注册表）+ 桌面 install/trust UI。
- **风险/边界**：bundle-manifest + 本地 git 安装 = 吸；**中心托管 `marketplace.json` 索引 = 不吸**（filter #2：那是多租户/云目录，单用户桌面不该依赖托管服务）。bundle 是**打包信封**、非双机制（不违 filter #3），但**信任门必须复用 lyra 现有 approval/trust 轨**（如 hooks-trust），不新造。价值低于 K1/K2——lyra 自著述已覆盖"创建"那半，缺的只是"从别处装一个包"。
- **优先级**：**P3**。

---

### K4 · ACP adapter（部分吸，低；与 [`GROK.md` G17](GROK.md) 合并考量）

- **来源/为什么**：`packages/acp-adapter/`（`@agentclientprotocol/sdk`，`kimi acp` 子命令）。薄 adapter 把现有 harness 暴露成 **ACP** server（stdio ndjson），翻译 ACP `initialize`/`session.new`/`prompt`/tool-confirmation ↔ harness，让 Zed/JetBrains 驱动真实 session。复用 harness、是 additive 外部面、非第二引擎。
- **目标 / verdict**：与 Grok 的 ACP 结论一致——**不是**双机制债（方向=外部编辑器驱动我的 agent，异于 lyra 桌面协议；"runtime 协议无 stdio"不违反，ACP-over-stdio 是对编辑器说的另一套面）。但服务的 use case 与 lyra"自有 Wails 桌面"产品论点正交、面大。**现在只借架构note（任何外部协议 = harness 之上的薄 adapter、绝不第二引擎），线协议 defer 到有真实 IDE 嵌入需求**。
- **优先级**：**defer（P3）**。详见 [`GROK.md` G17](GROK.md)。

---

## 2. 刻意不吸清单

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **Kimi OAuth / managed login** | `packages/oauth/`、ACP `TERMINAL_AUTH_METHOD` | 正是 lyra 拒绝的 provider-OAuth；用户填 key、401→reprompt（反向不变量）|
| **AI-native MCP config via skill** | `skill/builtin/mcp-config.md` | 让 agent 手改 `mcp.json`；lyra **更强**——typed `workspace.mcp.*` RPC + 设置面板比 agent 编辑 JSON 更干净安全（parity-plus，无借鉴）|
| **video input / multimodal** | README | lyra 图像-only、Media 尚 parked；video 重且对 coding agent niche |
| **`minidb` + event-sourced agent-records** | `packages/minidb/` | lyra 单 SQLite + type-tagged snapshot durable-resume 是刻意且 working 的选择，别换存储模型（借 goal 的**语义**、非其持久化）|
| **`agent-core-v2` DI×Scope 引擎** | `agent-core-v2` | 架构 pattern 非能力，与 lyra welded-core 立场相悖 |
| **telemetry + crash upload** | `packages/telemetry/` | lyra 的 OTel triad → slog 已 vendor-neutral 覆盖 |
| **`write-goal` 独立 skill** | `skill/builtin/write-goal.md` | 把它的 completion-contract framing（end-state/proof/boundaries/loop/stop-rule）折进 K1 goal 的 continuation prompt，不单独成 skill |

---

## 3. 建议节奏

- **头号独立批**：K1（Goal mode）——大而干净、blast radius 受控，layer 在现有 turn 循环/approval/resume 之上。
- **门控于 C7**：K2（KAOS 环境 seam）——C7 沙箱启动时一并做。
- **随缘**：K3（bundle 本地安装，P3）、K4（ACP 设计 note，defer，与 G17 合并）。
