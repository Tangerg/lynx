# Lyra — Agent 能力横向对比（全量重写 · 2026-06-14）

> **视角**：能力面（运行时 capability），不比 UI 样式。**基线**：lyra = Go agent-runtime backend（前端在 `/Users/tangerg/Desktop/lyra/`）。
> **对比对象**：桌面上**所有 AI Agent 应用**。**排除**：库 / 框架（a2a-go / adk-go / eino / koog / langchain4j / spring-ai / trpc-* / go-sdk / ai / sse / embabel / langgraphgo …）、**Port AI 系列**、纯模型目录服务（catwalk）、纯前端壳（agent-chat-ui）。
> **公平前提**：lyra 是**后端运行时**;桌面 app 是 frontend+backend 打包。本文比"后端运行时能力";**前端/桌面 UX**（语音 ASR、全局热键、图片生成的显示）归 lyra 前端,**不算 runtime 缺口**（见 §6）。

---

## 0. 一句话结论

> **lyra 在「单次 agent turn 的执行质量」上已是第一梯队**（LSP / HITL / fork+checkpoint / 多 provider / A2A / OTel / loop-detect / token 压缩 / 编辑安全 全都有）。**真正落后的是「agent 的自主性与触达」**——调度/自动化、远程 IM 桥接、多 agent 团队、hooks——这正是同品类的 **Proma / AionUi** 领先之处。

---

## 1. 对比对象（两类）

### 1.1 同品类 —— 本地优先 AI 桌面 agent（lyra 的真正对标）

| 应用 | 形态 | 一句话 |
|---|---|---|
| **Proma** | Electron + Claude Agent SDK | 多模型 Chat + 通用 Agent + 工作区 + Skills + MCP + **远程机器人** + 记忆 + **自动化调度** |
| **AionUi** | Electron + 自带引擎 | 统一 15+ CLI agent + **Team mode（ACP）** + **cron 24/7** + 6 个 IM 桥接 + 21 助手 |
| **cherry-studio** | Electron 多 LLM 客户端 | 强 UX、typed per-tool 渲染、分支导航;agent 能力偏轻 |
| **lobe-chat（lobehub）** | Web/桌面 | chat UX 标杆;agent/工具偏轻 |

### 1.2 编码 agent / runtime

`claude_code` · `codex` · `cline` · `continue` · `crush` · `opencode` · `OpenHands` · `kimi-code` · `plandex` · `MiMoCode` · `harness9` · `pi`

> assistant-ui 是 React 组件库（UX 范式参照,非 runtime,不进矩阵）。

---

## 2. lyra 现状能力基线（2026-06-14 实测,防止误报缺口）

**单 turn 执行**：agent loop（复用 lynx `agent/runtime` 的 `for{}`）· 工具循环 + **并行工具** · **HITL R 模型**（park-on-interrupt + resume,可持久化/审计/跨重启）· plan 模式 · steering 注入 · **MaxBudget/MaxCostUSD/MaxSteps 上限**。
**防失控**：**loop detection 已接**（`kernel/engine.go: LoopDetection`,经 agent/tool SDK）· budget/step backstop。
**上下文**：压缩 **同时按消息数(24) 和 token 估算(100k) 触发** · 整段 wholesale 摘要 + 保留最近 · LYRA.md 长期记忆 + extractor 提取事实。
**代码能力**：**LSP 6 操作**（definition/references/hover/symbols/diagnostics）· 编辑安全（read-before + stale 守卫）· fs/bash/web（fetch+search）· **model-facing todo（`todo_write`,SQLite 持久化）**。
**会话/状态**：Session→Run→Item · **fork + 影子 git 文件 checkpoint + export/import** 三件套 · per-session cwd。
**集成**：MCP client（5 态生命周期 + **auth 基座已铺**）· **A2A**(agent-to-agent 跨 runtime) · Skills（project+global） · **多 provider×多 model（38 provider,显式配对）**。
**委派**：subagent 3 种 spawn 模式（protected-only 作委派默认）。
**可观测**：OTel 三驾马车 → slog（vendor-neutral）。

> **旧版列为缺口、现已落地的**：model-facing todo（`78fc84e`）、loop detection、token-aware 压缩触发、MCP auth 基座（`a4f7a4d`）、budget/step caps。**别再当缺口。**

---

## 3. 能力矩阵（capability → lyra → 谁有）

### ✅ lyra 已具备（与第一梯队齐平或领先）

| 能力 | lyra | 备注 |
|---|---|---|
| agent loop + 工具循环 | ✅ | 复用 SDK `for{}`,不手写 |
| 并行工具执行 | ✅ | `ParallelToolLoop` + per-call 取消;**缺 per-path 锁**（§4.10） |
| HITL 审批（R 模型） | ✅ | 可持久化/审计/跨重启,优于 cline 同步 gate |
| loop detection | ✅ | SDK `LoopDetectionConfig` |
| budget/step 上限 | ✅ | token + cost + steps |
| 压缩（token 触发） | ✅🟡 | 有触发,**策略待精修**（§4.6） |
| LSP 代码智能 | ✅ | 6 操作;harness9/pi/codex/OpenHands 零 LSP |
| 编辑安全（read-before+stale） | ✅ | claude_code/crush 同款,方向被验证正确 |
| model-facing todo | ✅ | `todo_write` SQLite |
| fork + 文件 checkpoint + export/import | ✅ | 三者同时具备,组合罕见 |
| 多 provider×model | ✅ | 38 provider 显式配对,广度+显式性领先 |
| A2A 跨 runtime | ✅ | 全部对比里独有 |
| Skills | ✅ | project+global 合并 |
| OTel 三驾马车 | ✅ | vendor-neutral,独有形态 |
| web fetch + search | ✅ | |

### ❌ / 🟡 lyra 缺或弱（详见 §4）

| 能力 | lyra | 谁有 |
|---|---|---|
| **调度 / 自动化运行时** | ❌ | **Proma**（scheduler 30s tick）· **AionUi**（cron 24/7）· claude_code/kimi 部分 |
| **远程 IM 桥接** | ❌(仅蓝图) | **Proma**（飞书/钉钉/微信）· **AionUi**（6 平台） |
| **Hooks（pre/post-tool）** | ❌ | claude_code(27)·codex(10)·pi(~40)·harness9 |
| **多模态图片输入** | 🟡(wire 有,模型路径未接) | 几乎全部 peer |
| **多 agent 团队/并行编排** | 🟡(单委派) | AionUi(ACP Team)·cline(16 team tools)·claude_code·kimi |
| **microcompaction / 压缩精修** | ❌ | claude_code·mimocode·OpenHands·pi |
| **语义代码搜索 / repo-map** | ❌(rag 未接) | plandex(tree-sitter)·cursor/windsurf(向量) |
| **per-role 模型分配** | ❌ | plandex·crush·OpenHands·pi |
| **OS sandbox** | ❌(缓做) | codex·OpenHands·harness9 |
| **evals 回归框架** | ❌ | harness9 |
| **stall 检测 / anti-cheat-todo / denial-stops-turn** | ❌ | crush·harness9·codex |
| **MCP-as-server / per-workspace MCP** | ❌ | OpenHands·crush(server)·Proma(per-workspace) |
| **proactive 推荐 / monitors** | ❌ | Proma |
| image 生成 · 语音输入 | ❌(前端层,§6) | claude_code/codex/AionUi(生成)·Proma(语音) |

---

## 4. lyra 的真正缺口（按价值/普遍性,三梯队）

### 第一梯队 —— 同品类（Proma/AionUi）领先、定义"自主 agent"的三样

**4.1 调度 / 自动化运行时** ❌ —— **最大品类差距**
Proma `automation-scheduler.ts`（30s tick、断电恢复、daily/weekly/interval、run 历史、失败连续阈值自动暂停、permission profile per task）;AionUi cron 24/7 无人值守 + 会话绑定。lyra 是**纯 turn 驱动,零调度器**。
> 价值:把 agent 从"一问一答"变成"会自己定时跑";本地优先桌面 agent 的核心卖点。成本:中（新 domain:调度表 + tick + run 记账,可复用 session/run 基建）。

**4.2 远程 IM 桥接** ❌ —— 只有 `IM_GATEWAY.md` 蓝图
Proma 飞书 OAuth(68KB)+钉钉+微信(presence/通知/触发);AionUi 飞书/钉钉/微信/Telegram/Slack/Discord。lyra 未建。
> 价值:从手机/群里触发任务、收结果。成本:中-高（每平台一座桥 + 鉴权;可先做一个 webhook 入口）。

**4.3 Hooks / 扩展点（pre/post-tool）** ❌ —— **最大扩展性缺口**
claude_code 27 事件（PreToolUse = 改入参+权限+注上下文 三合一）;codex 10;pi ~40（自扩展 + LLM 写自己的工具 + 热重载）;harness9 装饰器注册表 + **权限从磁盘热重载**（改"always allow"即时生效,便宜招）。lyra 零 hooks。
> 价值:lint/format/拦截/审计的地基。成本:中（事件分发 + SPI 接口）。

**4.4 多模态图片输入** 🟡 —— wire 支持、模型路径待接
wire 的 `ContentBlock{type:image, attachmentId}` + `StartRunRequest.attachments` 已就位,`features.multimodal` 能力位也在,但**模型适配器是否真把图片发给模型待核实**。除 lyra/harness9 外几乎所有 peer 都有。
> 价值:最显眼的单点能力差。成本:低-中（接通 model adapter 的 image content）。

### 第二梯队 —— 进阶、数家有

**4.5 多 agent 团队 / 并行编排** 🟡 —— lyra 只单委派
AionUi Leader+Teammate 经 **ACP** 并行 + 共享任务板;cline 16 team tools;claude_code teams+SendMessage。lyra 有 `SpawnChildProtectedOnly` 单委派,**无并行团队编排**。
> 成本:高（新 domain:团队生命周期 + 消息板 + 成本 roll-up）。

**4.6 压缩精修** ❌ —— 现在是整段 wholesale 摘要 + 固定阈值
缺:① **window-相对 token 触发**（remaining ≤ window*80%,用 CatalogPricing 真 token,而非固定 100k 字符估算,5/5 收敛）;② **microcompaction**（清旧 tool_result 体、留 tool_use + 最近 N;claude_code 还有 API-native `clear_tool_uses`,lyra 默认 Claude 可先评估这条）;③ **结构化摘要模板**（claude_code 9 段 / pi Goal-Constraints-Progress + 文件清单）;④ **condensation-as-event**（OpenHands:压缩=产生不可变事件 + projection 重放,可审计)。
> 成本:① 触发改写 trivial;②③④ 各中等。

**4.7 语义代码搜索 / repo-map** ❌ —— `rag/` 模块未接进 agent
plandex tree-sitter 符号图;cursor/windsurf 全库向量。lyra 有 RAG pipeline 但不喂给 agent 选文件。
> 成本:中（接 rag + tree-sitter 变体）。

**4.8 per-role 模型分配** ❌ —— Compactor/Extractor/Planner 各用合适模型
plandex per-role model pack;crush Large/Small。lyra 可复用 per-run-model seam。
> 成本:中（角色边界 + 复用现有 seam）。

**4.9 OS sandbox** ❌ —— 缓做（审计模型兜底）
codex（policy→argv 纯函数,3 平台）· OpenHands（Workspace 接口 + LocalWorkspace 无 Docker）· harness9（per-agent Docker）。可移植抽象**确实存在**（三家收敛）。
> **立即可做、零依赖**:`.git` 子路径强制只读（防 agent 改 `.git/hooks` 提权,codex 范式）。macOS path 便宜（sandbox-exec 字符串,无 cgo);Linux 重（bwrap/seccomp）。

**4.10 per-path 工具锁** 🟡 —— 并行已有,缺 fs-写互斥
claude_code `isConcurrencySafe`·OpenHands `ResourceLockManager`·pi per-realpath 队列·harness9 path_locker（5/5 收敛）。
> 成本:低（PathLocker 装饰器,串行化同路径写,读/LSP/MCP 不受影响）。

### 第三梯队 —— 防御性 / niche

**4.11 evals 回归框架** ❌ —— harness9 ScriptedProvider（确定性 mock）+ Hard/Soft 断言 + hermetic + CI 闸（golden 用例,失败阻 merge）。lyra 零。成本:中。
**4.12 防失控补全** ❌ —— lyra 有 loop-detect,缺:stall 检测（harness9:3 turn + todo 完成数不变,~10 行）、anti-cheat-todo（harness9:1 完成/调用硬限,~20 行）、denial-stops-turn（codex/crush:拒绝即停 turn,trivial）。成本:低,~120 行全做。
**4.13 MCP-as-server / per-workspace MCP** ❌ —— OpenHands/crush 把自己暴露成 MCP server;Proma 每 workspace 独立 MCP/Skills。lyra MCP 是 runtime 全局。成本:中。
**4.14 proactive 推荐 / monitors** ❌ —— Proma 规则信号→建议定时任务/监控 + 文件/会话/外部事件触发 run。lyra 有 `workspace.subscribe`(文件事件) 但不"事件→触发 run"。niche。

---

## 5. lyra 领先 / 独有（勿误报为缺口）

- **A2A（跨 runtime agent-to-agent 协议）** —— 全部对比里**独有**。
- **LSP 代码智能（6 操作,内建非外部服务）** —— 第一梯队;harness9/pi/codex/OpenHands 零 LSP。
- **多 provider×model 显式配对（38 provider）** —— 广度 + `(provider,model)` 必须成对的显式性,独有。
- **OTel 三驾马车 → slog（vendor-neutral semconv 去品牌）** —— 独有形态（codex 仅 OTLP / OpenHands 仅 trace / crush PostHog / pi 无）。
- **fork + 影子 git 文件 checkpoint + inline export/import 三者同时** —— 组合罕见（cline 缺 fork/export 之一）。
- **HITL R 模型可持久化跨重启** —— 比 cline 同步阻塞 gate 强（有审计、可恢复）。
- **协议参数纪律**（32 typed 枚举 / 单 `type` 判别 / `Page[T]` / 开放 features map,对齐 Stripe/AIP）—— 见 `FRONTEND_API_REVIEW.md`。

---

## 6. 刻意不做 / 非 runtime 缺口

- **apply_patch（V4A 多文件 patch）** —— 刻意不做;claude_code（最 Claude-优化的 agent）也避开,lyra 守"guarded 单编辑"被验证正确。可选未来:codex 4 级模糊匹配作 fallback。
- **PTY 后台命令** —— 协议层刻意不做;trade-off:深层子进程孤儿不追踪。
- **图片生成(output) / 语音输入(ASR) / 桌面 UX（全局热键、语音窗口）** —— **前端/产品层**,归 lyra 前端;runtime 最多提供数据通道,不背为 runtime 缺口。
- **多租户 / 用户鉴权 / 订阅** —— 协议层零 user 概念,由更外层解决。

---

## 7. 落地优先级（平台无关 + 价值/成本最优）

| 序 | 项 | 梯队 | 成本 | 为什么 |
|---|---|---|---|---|
| 1 | **调度/自动化运行时** | 1 | 中 | 同品类核心差距,把 agent 变"自主" |
| 2 | **防失控补全**（stall/anti-cheat/denial-stops）+ **per-path 锁** | 2/3 | 低（~120+ 行） | 极便宜、即时稳健性 |
| 3 | **`.git` 只读不变量** | 2 | 极低 | 零依赖、防提权,现在就能做 |
| 4 | **window-相对 token 压缩触发** | 2 | trivial | CatalogPricing 已有,改触发即可 |
| 5 | **多模态图片输入** | 1 | 低-中 | 接通 model adapter,补最显眼缺口 |
| 6 | **Hooks（PreToolUse 三合一为核心）** | 1 | 中 | 扩展性地基 |
| 7 | **远程 IM 桥接（先一个 webhook 入口）** | 1 | 中 | 触达;先飞书/钉钉其一 |
| 8 | **microcompaction**（先评估 API-native `clear_tool_uses`） | 2 | 低-中 | 长会话质量 |
| 9 | **per-role 模型分配** | 2 | 中 | 复用 per-run-model seam |
| 10 | **语义检索 / repo-map** | 2 | 中 | 接 rag |
| 11 | **多 agent 团队编排** | 2 | 高 | 进阶,需求驱动再做 |
| 12 | **OS sandbox（macOS first）/ evals / MCP-as-server** | 3 | 中-高 | 防御性 / 触发条件驱动 |

---

> **维护**:本文是 2026-06-14 全量重写的现状快照。能力落地后回来勾掉对应 §4 项 + 更新 §2 基线;新对比对象出现时增列 §1。机制级实现细节（压缩 4 不变量、loop-detect 算法、sandbox 各平台成本）在落地该项时展开,不在本对比文档堆砌。
