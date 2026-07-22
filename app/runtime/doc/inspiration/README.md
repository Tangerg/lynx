# 同类 Agent 应用启发的能力吸纳 —— 总索引与合并 Backlog

> **这是什么**：对桌面上 6 个同类生产级 coding agent 的源码级对比分析，用统一方法（源码级自析 → 过 lynx 四道筛子 → 只收可搬点子），产出"哪些能力值得吸纳进 lyra"。每个应用一份独立文档；本文件是**跨应用合并排序 backlog + 总索引 + 刻意不吸总表**。
>
> **状态**：部分仍 **proposed**（分析产物，未实现）。**已落地（前后端）**：T1 OS 沙箱(C7 隔离运行) · T3 工具搜索 · T4 跨会话记忆(C8) · T5 Goal mode · T7 自进化 skill · T8 压缩后活状态 reminder · T9 models.dev 目录 · T13 doom_loop 守卫；T6 结构化执行 TODO **原已实现**。**仍 open**：T2 凭证经纪人（头号）· T10~T12 · T14~T25。其余落地时每条按"破坏性公开 API 改动须先咨询"走。

## 0. 六道对照 + 索引

| 应用 | 语言/形态 | 文档 | 一句话头号借鉴 |
|---|---|---|---|
| **Grok Build** | Rust，TUI/headless/ACP | [`GROK.md`](GROK.md) | OS 沙箱(C7) + 跨会话记忆(C8) + 压缩后活状态 reminder + 工具搜索 |
| **Codex** | Rust，引擎+app-server | [`CODEX.md`](CODEX.md) | **凭证经纪人**（别家皆无）+ 双轴沙箱(C7) + execpolicy |
| **Claude Code** | TS，Bun/Ink TUI | [`CLAUDE_CODE.md`](CLAUDE_CODE.md) | 工具搜索 + 结构化执行 TODO + auto-memory(C8) |
| **Kimi Code** | TS，自研引擎栈 | [`KIMI_CODE.md`](KIMI_CODE.md) | **Goal mode**（自主多轮循环）+ KAOS 环境 seam |
| **opencode** | TS，headless server+多客户端 | [`OPENCODE.md`](OPENCODE.md) | 可刷新 system-context + models.dev 目录 + 工具搜索 |
| **Hermes** | Python，learning loop | [`HERMES.md`](HERMES.md) | 记忆(C8 蓝本) + **自进化 skill**（B4 扩展） |
| _其余 ~23 家_ | 轻量扫描 | [`MISC.md`](MISC.md) | 无格局级新能力；6 个 P3/design-note 小点（plandex 自主档位拨盘 / craft "add a source" 自助接入 / trpc 信号驱动学习触发…） |

**方法论关键**：多家**独立殊途同归**到同一能力 = 最强采纳信号。下面每条主题标注"收敛度"。

---

## 1. 合并排序 Backlog（按主题聚合，跨应用）

### 🥇 第一梯队（P1，高价值 + 高置信）

**T1 · OS 级沙箱 confinement —— ✅ 已落地 C7（隔离运行模式）**　`收敛：Grok + Codex（2家）`
- 声明式 allow/deny profile → OS 内核 confinement（macOS **Seatbelt** 先行，覆盖用户主平台；Linux Landlock+bwrap+seccomp），逐子进程/逐命令 confine（lyra 长驻多 session、不锁 runtime 进程），fail-closed，项目配置只能加不能改。
- **Codex 的实质增量**（C7 落地直接吸）：FS ⊥ Network **双轴** profile；可写根内挖**保护子路径**（`.git`/`.lyra`）；敏感 glob **deny-read + deny-unlink**（堵"用删除探测密钥"）；路径作 `-D` 参数防注入。
- **连带**：C7 落地后翻转 B5/C3——"真 confinement 才配少问"，yolo 真安全，审批只留 out-of-confinement 写 + unsafe-env floor。
- 详见 [Grok G1](GROK.md)、[Codex CX2](CODEX.md)。

**T2 · 凭证经纪人 + loopback 网络策略**　`唯一：Codex`
- 真实密钥**永不进 agent env**——spawn 子进程时换 dummy，代理只朝绑定 host 注入；agent 即便被 prompt-injection 攻陷也读不到、传不走密钥。切断"密钥→env→shell 可读→外泄"整条链。**别家都没有、对编码 agent 威胁面高度吻合、lyra 完全缺失**。
- 先做最小版（loopback 代理 + 密钥注入 + 域 allowlist），完整 MITM 栈 P2。详见 [Codex CX1](CODEX.md)。

**T3 · 工具搜索 / 渐进式披露**　`✅ 已落地`　`收敛：Grok + Claude Code + opencode（3家，最强信号）`
- **落地形态**：MCP 工具**类别性**移出常驻 manifest（第一方常驻），`search_tools`（keyword + `select:` 双模式 + 跨 server round-robin 公平）搜到即经 **agent/toolloop 的 mid-loop 工具提升 seam**（`PromoteTools`，加法非破坏）进入后续轮 manifest，resume-safe（promotion 随 checkpoint 的 request 走）。deferred 名字列进 `search_tools` 描述（"N of M" reminder）。
- MCP 工具**不全塞 manifest**——内存索引 + `search_tools` meta-tool 返回 top-k（name+schema+server）。provider-agnostic（降级 `<functions>` 文本块）。opencode 的**跨 namespace round-robin 公平**（防单个大 MCP server 饿死别人）一并吸。
- 纯上下文优化、零机制债、薄核。**三家独立收敛 → 最先做的确定项之一**。详见 [Grok G4](GROK.md)、[Claude Code CC1](CLAUDE_CODE.md)、[opencode OC4](OPENCODE.md)。

**T4 · 跨会话记忆 —— ✅ 已落地 C8**　`收敛：Grok + Claude Code + Hermes（3家）`
- 三家给出三套设计，**关键设计岔路 lyra 须显式选**：
  - **有界 + 溢出强制当轮 consolidate（Hermes）** —— 硬 char cap，溢出返回教模型自 curate 的结构化错误；**更贴 lyra 薄核 + 反向不变量**（无隐藏后台 LLM）。
  - **无界 + 后台 "dream" LLM 蒸馏（Grok）** —— 周期性反思式合并 session log 进 curated 文件，gate+lock+`NO_REPLY`+只删已读。
  - **检索式注入（Claude Code）** —— frontmatter manifest + 便宜模型选 top-5 + 注入；沙箱化 extraction fork 只读+只写 memdir。
- **综合落地**：`LYRA.md` 当 curated 层（不切分两文件）；加 session-log 层 + **FTS5 关键词召回**（Hermes 证明关键词对"是否聊过 X"够用，比向量便宜）；轨迹自动挖掘按 cadence；写全部生命周期所有 + HITL 一致；**推荐 Hermes 的有界路线**、dream 作可选。复用 lyra 现成 @codebase embedding/cosine 做向量层。详见 [Hermes 记忆](HERMES.md)、[Grok G2](GROK.md)、[Claude Code CC4](CLAUDE_CODE.md)。

**T5 · Goal mode —— 受监督的自主多轮执行循环**　`✅ 已落地（前后端）`　`唯一：Kimi`
- typed runtime state + 最小 4 态机（active/paused/blocked/complete）+ continuation-prompt 驱动 + **opt-in 预算硬顶** + 重启降级(active→paused) + **入口 HITL 门**。lyra 有 plan-mode/steer/scheduler/durable-resume，但**无自主自续执行循环**——用户现在得每轮敲 continue。保持为独立机制、别折进 steer/plan。详见 [Kimi K1](KIMI_CODE.md)。
- **落地形态**：`domain/goal`（session-keyed 独立 durable store，与 per-run RunState 正交）+ `update_goal` 工具（**机器信号**才结束循环，prose"done"不算；resolver 只在 goal active 时对 coding 角色出现）+ `application/goals` GoalDriver（镜像 schedules，消费 run 的 `SegmentFinished` terminal 决策：complete→clear · blocked/预算→停 · error/cancel→paused · 否则注入 continuation 开下一 run；**不在 pump 内**，back-to-back launch 用 ErrSessionBusy 有界退避等 admission 释放）+ goals.start/get/stop/resume RPC + boot Reconcile(active→paused)。自主 run headless（现有全局 approval stance 治理）。**complete 是 machine-signal 纪律 + 预算硬顶**是设计价值本身。前端 goal box 随后已跟上。

**T6 · 结构化执行 TODO 追踪**　`✅ 原已实现`　`唯一：Claude Code`
- lyra 已有 `todo_write` 工具 + `todo` 域（`todo.Validate` 强制"恰好一个 in_progress / 完成即标 / 一次一个"、`todo.Render`、session-keyed、两个角色），字段比 CC V1 更全（blocked_reason / next_action）。无需再做。
- 会话内、模型自管的执行进度清单（"恰好一个 in_progress"/"完成即标记"/"≥3 步才用"），既作模型 working-memory（实测提升长任务完成率）又给 UI 实时进度条。与 plan-mode 保持"计划 vs 执行"分层。极薄（session-state + 一工具）。详见 [Claude Code CC2](CLAUDE_CODE.md)。

**T7 · 自进化 Skill —— B4 扩展**　`✅ 已落地（前后端）`　`唯一：Hermes`
- B4 的三个真实短板：**轨迹自动蒸馏**（post-turn 后台 review 挖轨迹）+ **反馈驱动精修现有 skill**（从用户纠正 patch）+ **自动闲置生命周期**（active→stale→archived、re-use 复活、never delete、provenance-gated）。**把 Hermes 的"自动挖掘脑"接到 B4 的"强制 HITL 身"**——自动提议落 `_drafts/` 走人审门。治理**不借**（Hermes 默认自由写弱于 B4）。详见 [Hermes skill](HERMES.md)。

**T8 · 压缩后活状态 system-reminder**　`✅ 已落地`　`唯一：Grok`
- LLM 摘要天然丢活的执行副作用——压缩重建末尾确定性拼一段 reminder（在跑后台 shell + TODO + 在跑 subagent + 各自 poll/cancel 工具名）。压缩正确性的独立维度、零 LLM。详见 [Grok G3](GROK.md)。
- **落地形态**：LLM-summary rung 在 summary 后注入 `<system-reminder>`，列 session 的在跑后台 shell（`exec.Shells` 加 session-scoping + `RunningForSession`）+ in-progress todos；无活状态则整段省略。**subagent 段刻意不做**（task 工具同步跑子进程，post-turn 压缩时无在跑 subagent）。

**T9 · models.dev 外部模型目录（build 时 vendor）**　`✅ 已落地`　`唯一：opencode`
- 把模型 id/能力/定价/上限从二进制解耦成外部维护数据集，**build 时 vendor `models.dev/api.json`**，消除手维护 21-provider Go 表漂移。只取数据源、不取 Effect/TTL 机器。小、独立、收益明确。详见 [opencode OC3](OPENCODE.md)。

### 🥈 第二梯队（P1~P2）

**T10 · 可刷新 system-context + 中途 durable system-delta**　`唯一：opencode` —— system prompt 建模成可刷新类型化源，env/date/指令中途变化以 durable delta 注入、锚在 compaction/resume 都尊重的 baseline epoch 上。与 lyra 已有 epoch/compaction 世界观最契合的纯加法。[opencode OC2](OPENCODE.md)。

**T11 · Hunk 级改动 curation + 可撤销 revert 预览**　`Grok + opencode` —— 按 hunk + agent-turn 归属追踪改动、逐 hunk/整轮 accept·reject（Grok G5，复用 runId/segmentId）+ stage→preview-diff→clear→commit 可逆预览（opencode OC5）。与整仓 checkpoint 正交（curation vs 时间旅行）。⚠️ accept/reject 改盘=第三条 mutation 路径，走同一 lock+stale-guard。**逐文件 content-store 不吸**（lyra 整仓 shadow-git 已定型）。enabler：[Grok G5.1 fsnotify](GROK.md)。

**T12 · yolo LLM reviewer（自动第二意见）**　`收敛：Claude Code + Codex（2家）` —— yolo/auto 模式下每动作过便宜 LLM judge（Claude Code 两段式 fast→thinking + prompt-injection 硬化 + fail-closed；Codex guardian forked reviewer）。是 B5/C3 自否决的**智能治本版**。做成 approval Service 内一条可插 reviewer decision path、只挂 yolo、复用 per-role utility model、连 fail-closed + 只喂 tool 投影一起吸。[Claude Code CC3](CLAUDE_CODE.md)、[Codex CX5](CODEX.md)。

**T13 · doom_loop 守卫**　`✅ 已落地`　`收敛：opencode + Codex + Grok` —— 同工具+同入参连续 3 次 → ask（opencode 的纯计数客户端版是 Grok provider-信号"观察项"的可落地形态）。几十行、小而实。[opencode OC6](OPENCODE.md)。
- **落地形态**：治本用 **no-progress 信号**（同工具+同入参**且同输出**）而非纯计数，避免 `shell_output` 轮询长命令的假阳；只在 GatePass（yolo/auto 会放行）分支升级为审批中断（复用现成审批 UI/resume/headless-auto-deny），不碰 would-deny / would-prompt。

**T14 · 一等 media 落地 + 工具输出三分**　`唯一：Grok` —— value/model_output/UI-render 三投影 + `media_id` 跨调用句柄，落地 lyra parked 的 Media。取显式形态、弃 JSON 魔法自动提升。[Grok G6](GROK.md)。

**T15 · execpolicy：argv 策略层 + approval×sandbox 联合决策**　`唯一：Codex` —— 命令级、可解释、带示例校验的策略层；approval 决策联合考虑沙箱在场（有沙箱兜底就少打扰）；审批自动泛化成最小前缀规则回写。取思想、不引 Starlark（Go 结构体/TOML）。门控于 C7。[Codex CX3](CODEX.md)。

### 🥉 第三梯队（P2~P3，廉价收尾 / 门控 / defer）

- **T16 · 压缩健壮性 + verbatim 骨架**（Grok G9/G10）：退化摘要门 + preflight-before-doomed-request + headroom；用户原话/AGENTS.md 永不进摘要。⚠️别抄 Deterministic/Transient 重试分类。
- **T17 · subagent typed IO 契约 / 异步 worker / 对抗 verifier**（Grok G7 + Claude Code CC5/CC6）：persona 声明 inputs/outputs 组流水线；异步 worker + 续跑语义；built-in 只读对抗 verifier + "不许自评 verdict" nudge。拒整套 teams/mailbox/coordinator 机器。
- **T18 · KAOS 执行环境 seam**（Kimi K2）：把 per-session-cwd resolver 泛化成 fs+process `ExecutionEnvironment` 接口——**门控于 C7 启动**，不建 SSH 后端。
- **T19 · 廉价快赢集**：MCP SSE 重连 backoff（Grok G12，直接做）· 非交互 shell 注入 `GIT_PAGER=cat`/`FORCE_COLOR`（Grok G14）· 秘密脱敏只挂 OTel/session-export chokepoint、绝不脱敏 model 输入（Grok G13）· config/approval 微加固（Grok G15：报错不回显含密行 / 拒项目配置提升 user 层 / permission 默认 Deny）· hooks seam 覆盖审计（Grok G16 + opencode `session.compacting`）· `tool_scope=Write→仅 leader`（Grok G8）。
- **T20 · apply_patch 模糊编辑格式**（Codex CX4）：内容寻址模糊匹配 + `@@` 锚 + 多文件原子信封。edit 漂移是真痛则 P2。
- **T21 · capability bundle 本地 git 安装**（Kimi K3）：一个 manifest 打包 skill/MCP/hook/recipe 从 git 装、装前显示信任；**中心托管 marketplace 索引不吸**。
- **T22 · ACP 编辑器嵌入**　`收敛：Grok + Kimi + opencode（3家）`：**现在只借设计**（typed `session/update` 事件分类 + host 托管 buffer-aware fs/terminal/permission seam）；**线协议 defer** 到"要不要吃 Zed/Neovim 用户"成为产品决策——届时加 additive `lyra agent stdio`（Go ACP SDK），绝不重写桌面协议。不是双机制债（方向异于桌面协议+a2a）。
- **T23 · 事件流 seq 重放核对**（opencode）：核对 lyra desktop event 通道重连是否已有 after-seq 重放；若无则补（wire 层，P2）；若有 items.list seq + per-run last-event-id 则 parity。
- **T24 · MCP-server OAuth（待决 gap）**（Grok G19）：与"不给 LLM provider 做 OAuth"是两回事；远程 MCP 生态越来越要求，bearer-only 是真天花板。用到再做、但命名它。

### ⏸ 高智力 / 高成本，先写 spec

- **T25 · tools-as-code / CodeMode**　`收敛：Codex + opencode（2家）`：模型写一段程序把工具当函数编排，砍 round-trip + context（尤其 MCP fan-out）。**但**：嵌 JS 引擎（goja/V8）+ 沙箱 + host 协议成本极高，codex/opencode 自己都 dev-gated 或迁移中；lyra 已有 ConcurrencyKey 并行工具 + T3 工具搜索拿到大部分红利。**结论：先上更便宜的 T3，CodeMode 仅在 MCP 编排成真实瓶颈时写 spec 评估**，届时优先轻量 batch/multi-tool 而非嵌 JS。[Codex CX6](CODEX.md)、[opencode OC1](OPENCODE.md)。

---

## 2. 统一优先级速览

| 优先级 | 条目 |
|---|---|
| **✅ 已落地** | **T1 沙箱(C7)**、**T3 工具搜索**、**T4 记忆(C8)**、**T5 Goal mode**、**T7 自进化 skill**、**T8 活状态 reminder**、**T9 models.dev**、**T13 doom_loop**（+ **T6 执行 TODO** 原已实现） |
| **P1（先做）** | T2 凭证经纪人（头号 open） |
| **P1~P2** | T10 system-context、T11 hunk curation、T12 yolo LLM reviewer、T14 media 落地 |
| **P2** | T15 execpolicy、T16 压缩健壮性、T19 廉价快赢集、T23 事件 seq 核对 |
| **P3 / 门控 / defer** | T17 subagent 组合、T18 KAOS(门控 C7)、T20 apply_patch、T21 bundle 安装、T22 ACP(defer)、T24 MCP OAuth(待决)、T25 CodeMode(先写 spec) |

**C7 沙箱（T1）与 C8 记忆（T4）均已落地**；C7 的配套 T2 凭证经纪人 / T15 execpolicy / T18 KAOS seam 仍 open。其余多为薄核加法或廉价收尾。

---

## 3. 刻意不吸总表（跨应用去重，防未来会话重新论证）

| 类别 | 具体项（出处） | 不吸理由 |
|---|---|---|
| **多租户 / 云** | Grok computer-hub fabric、plugin marketplace；opencode share 云 + Zen 网关；Hermes Skills Hub + 8 外部记忆 provider（Honcho 云）；Kimi/CC marketplace | filter② 单本地用户，无云、无多作者分发、无多机 |
| **provider OAuth / token refresh** | Kimi OAuth、opencode integration/oauth 栈、CC OAuth、Grok system-power(为保护 OAuth refresh) | 反向不变量：用户填 key、401 UI 重填 |
| **retry / Transient 分类** | opencode executor Transient、Grok circuit-breaker 的 RetryPolicy | 反向不变量：SDK 内建 retry 已够 |
| **PTY** | Grok ptyctl、opencode pty、Codex unified_exec | lyra 已刻意 park（background commands, no PTY）|
| **协议 codegen / REST transport** | opencode OpenAPI-codegen、Codex app-server ts_rs | lyra 已定案 JSON-RPC + golden-sample drift gate |
| **双存储 / 换存储模型** | Codex rollout(JSONL 真源+SQLite 索引)、Kimi minidb 事件溯源 | lyra 刻意单 SQLite 唯一真源 |
| **企业 / 多机治理** | Codex config requirements/MDM、opencode 多项目 location、provider policies | 单本地用户，非 fleet |
| **结构化图/图谱作 agent 上下文** | Grok codebase-graph | name-based 不如 lyra LSP；grok 自己都只给编辑器；缺全仓符号表就给 LSP 加 `workspaceSymbol` op |
| **in-process JS 插件 loader** | opencode plugin、CodeMode 解释器机器本体 | lyra subprocess-contract 更安全；只薅 hook seam 清单 / tools-as-code 思想 |
| **过度工程 / theater** | Hermes weekly LLM consolidation（自己都默认 off）+ semver version 字段（无历史）；Grok two-pass 投机压缩 | YAGNI / 单用户不值 |
| **provider 私有续跑** | Codex `response_id` resume | provider 锁定；lyra 已有 durable cross-restart resume |
| **重叠机制** | CC output-styles（persona）；Hermes 两文件记忆切分；两文件/自由写默认 | 违 filter③；persona 并入 rules、记忆保持单 LYRA.md + 强制门 |
| **voice / announcements / self-update** | Grok voice/announcements/update | TUI 形态 / 云推 / Wails 打包层，非后端能力 |
| **full-replace 压缩** | Grok compaction 策略 | lyra A3 阶梯更省，别倒退 |

---

## 4. 几条方法论观察

1. **lyra 不落后**：6 家在 compaction / plan-mode / rules / MCP / subagents / sessions / structured-output / LSP / approval 粒度上与 lyra 大体同构或 lyra 更强。真 gap 集中在少数几处。
2. **两个 deferred 项被反复印证**：**C7 沙箱**（Grok+Codex，Codex 更细）与 **C8 记忆**（Grok+CC+Hermes，Hermes 更贴 lyra 哲学）——都不是要不要做的问题，而是**照哪家的设计落**。
3. **收敛 = 信号**：工具搜索（3家）、tools-as-code（2家）、yolo LLM reviewer（2家）、ACP（3家）——多家独立到达同一处，值得优先/认真。
4. **唯一亮点**：Codex 凭证经纪人、Kimi Goal mode、Claude Code 执行 TODO、Hermes 自进化 skill——单家独有但对 lyra 是真 gap。
5. **警惕线**：tools-as-code（T25）智力最诱人但成本最高，且被 T3 工具搜索抢走大部分红利——**先做便宜的、别为诱惑上大的**（YAGNI/KISS）。
