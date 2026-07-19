# opencode 启发的能力吸纳 Backlog

> **来源**：对 **opencode**（sst/opencode，开源 TS coding agent，Bun/Effect 生态，~2500 .ts + 590 .tsx；桌面克隆 `~/Desktop/opencode`）的源码级对比分析。核心形态 = **headless server + 多客户端**（`opencode serve` loopback HTTP server，TUI/web/Wails/IDE/slack/github 皆客户端）——与 lyra 的 runtime+desktop 分离**同构**。正处 **v1（promise+Zod）→ v2（Effect+Drizzle 事件溯源）迁移中途**，很多子系统两代并存，判断真 delta 时认准 v2。方法与 [`GROK.md`](GROK.md) 一致；跨应用总索引见 [`README.md`](README.md)。
>
> **状态**：全部 proposed。已跳过 parity 项（结构化输出=lyra JSONParser、skills 自著、LSP、rules、agent 配置、tool-output spill=lyra A2、approval 粒度、`external_directory`≈B5）。

---

## 0. 五个 distinctive 子系统

CodeMode（自研 confined JS 解释器，工具即函数）· client/server 协议（OpenAPI 3.1 REST + SSE + 多语言 SDK codegen + ACP stdio 适配）· models.dev provider 抽象（远程拉取目录 + OAuth 栈）· 事件溯源 session core（durable event log + projector + system-context epoch + 逐文件可回滚 revert）· in-process JS 插件 + share 云。

**收敛信号**：① **工具搜索/渐进披露** —— opencode(§OC4) 是**第三家**（[Grok G4](GROK.md) + [Claude Code CC1](CLAUDE_CODE.md)）独立做，P1 级强信号；② **tools-as-code** —— CodeMode(§OC1) 与 [codex code-mode](CODEX.md) 是**第二次**出现。

## 0.1 筛选准则

沿用 lynx 四道筛子 + 反向不变量，详见 [`GROK.md` §0.1](GROK.md)。

---

## 1. 吸纳清单

### OC1 · CodeMode —— 面向 MCP 的「工具即代码」编排（P2~P3，先写 spec）

- **来源/为什么**：`packages/codemode/`（`interpreter/runtime.ts` ~3465 行、`stdlib/*`、`tool-runtime.ts`）+ `tool/code-mode.ts`。给模型**一个** `execute({code})` 工具：TS `transpileModule` 去类型 → acorn 解析 → **自研 tree-walking 解释器**执行（无 eval/vm/import/环境 global/原型改写），整套 JS stdlib 手工重实现并 sandbox 化；工具按对象树暴露成 `tools.server.tool(args)`，模型可 sequence/branch/loop/`Promise.all`(上限 8)/过滤聚合，**只回传需要的数据**。收益：(a) 大工具目录不灌 prompt (b) 工具间不再每步一 LLM round-trip (c) 大中间结果留程序里不进 context (d) 权限只限宿主给的工具树。**只把 grouped/deferred（即 MCP）工具**转 CodeMode namespace，direct 工具仍走原生 tool-call。
- **目标**：把海量 MCP 工具收敛成一个 `execute` 命名空间 + 程序内编排，砍 round-trip 和 context。
- **落点**：tools/MCP runtime 层。
- **风险/边界**：**形绝不照搬**——~4500 行 confined 解释器 + 手写 stdlib 是巨大表面积，违 KISS/thin-core；Go 侧要么 goja 锁 globals + 时间/指令预算，要么定义极小编排 DSL。天然是**第二套工具分发机制（双机制债风险）**，必须限定"MCP 编排"这个明确痛点、作**可选模式**，非替换原生 tool-call。**与 [codex CX6](CODEX.md) 是同一方向**（tools-as-code）——两家出现说明是业界收敛方向（Anthropic code-execution-with-MCP / Cloudflare Code Mode），但两家实现成本都极高、codex 自己还 dev-gated。**建议：与工具搜索（OC4）二选一先上更便宜的 OC4，CodeMode 仅在 MCP 编排成真实瓶颈时写 spec 认真评估。**
- **优先级**：**P2~P3，需先写 spec**（最高智力价值、最高实现成本）。

---

### OC2 · 可刷新 system-context 源 + 中途 durable system-delta（P1~P2）

- **来源/为什么**：`core/system-context/{index,builtins}.ts`、`core/session/context-epoch.ts`、`session/history.ts`。把 system prompt 建模成一组**独立可刷新的类型化 source**（env / date / AGENTS.md 指令 / references），每个 `= {key, codec, load, baseline, update, removed?}`。`reconcile(current, snapshot)` diff：未变→不动；变了→注入一条**中途 system-delta 消息**（"The environment is now…" / "These instructions replace all previously loaded…"）并推进 snapshot；不兼容/源被删/compaction 后→整段 baseline 替换；某 source `unavailable` 则**阻塞替换**而非静默丢上下文。`context_epoch` 持久化 `baseline+snapshot+baseline_seq`，`history.load` 用 `baseline_seq` 只保留**当前** baseline、排除被取代的旧 system 消息、从最近 compaction 切片——**compaction 和 resume 都尊重这个 epoch**。
- **目标**：system context 不是会话开始冻结的静态串，而是"类型化源 → 增量、可重放、完整性校验的 system-delta，锚在一个 compaction 会尊重的 baseline epoch 上"。
- **落点**：runtime session/context 层。
- **风险/边界**：这是 opencode sub-agent 评"最该偷的单点"，认同——**原则性强、与 lyra 已有 epoch/compaction 世界观最契合的纯加法**（无双机制冲突）。env/date/指令在会话中途变化时以 durable delta 注入、且穿越 compaction/resume 保持单一连贯 baseline——lyra 现在没有这套。
- **优先级**：**P1~P2**。

---

### OC3 · models.dev 外部模型目录（build 时 vendor）（P1）

- **来源/为什么**：`core/models-dev.ts` + `core/catalog.ts`。从 `https://models.dev/api.json` 拉模型 `id/family/reasoning/tool_call/modalities[]/cost{tiers,cache_read/write}/limit{context,input,output}/status/provider{npm,api}`，原子写盘缓存、离线降级到编译期快照；`catalog.small()` 用 `cost*0.8+age*0.2` 自动选便宜快模型（生成标题用），`default()` 选最新可用，`experimental.modes` 把一个模型展开成多变体（如 `-thinking`）。
- **目标**：把"模型 id/能力/定价/上限"从二进制解耦成外部维护的数据集——手维护的 Go 21-provider 表每次新模型上线就漂移。
- **落点**：provider/catalog 层 + build 脚本。
- **风险/边界**：**只取数据源、不取机器**——对单机工具，**build 时 vendor `models.dev/api.json`**（每次 release 重生成）+ 可选刷新即拿 ~90% 价值，Effect/Flock/TTL/原子 rename 整机器全丢（filter②④）。`small()`/newest-default/tiered-pricing/mode 展开是纯数据 + 几行逻辑，顺带薅。
- **优先级**：**P1**（小、独立、收益明确）。

---

### OC4 · 大工具集渐进披露 + 工具搜索（P2；与 [Grok G4](GROK.md)+[Claude Code CC1](CLAUDE_CODE.md) 三方收敛）

- **来源/为什么**：`codemode/tool-runtime.ts`。不把全部工具灌进 prompt，而给一个 **token 预算目录**（chars/4，默认 2000），**跨 namespace round-robin 公平**选完整签名（防一个大 namespace 饿死别人），再永远挂一个 `search({query,namespace,limit,offset})` 做确定性字段加权搜索 + 分页。
- **目标**：解决"MCP 工具一多就淹没 prompt"——与 CodeMode 解耦，可单独落地、不需要解释器。
- **落点**：MCP/tool registry 层。
- **风险/边界**：**三家独立收敛（Grok G4 + Claude Code CC1 + opencode）= 最强信号**，实现时合并为一条。opencode 的 **round-robin 跨 namespace 公平**是 Grok/CC 没强调的好细节（防单个大 MCP server 饿死别人）——一并吸。自足小设计。
- **优先级**：**P2**（成本低、快赢；总排序里因三方收敛应视为 **P1** 级）。

---

### OC5 · 可预览、可撤销的 revert 生命周期（stage→preview→clear→commit）（P2）

- **来源/为什么**：`core/snapshot.ts`、`core/session/revert.ts`。revert 三段式：`stage`（存 `original` tree、逐文件恢复、算 unified diff、持久化 `Revert.State`，**仍可逆**）→ `clear`（撤回）→ `commit`（永久截断 boundary 后消息日志 + 删排队输入 + 重置 context epoch）。staged diff 在提交前作为 UI **预览**。
- **目标**：revert 是**可预览、可撤销**的暂存态——用户看 diff 预览再决定是否 commit，而非一步不可逆回滚。
- **落点**：checkpoint/UI 层。
- **风险/边界**：**只吸可逆预览生命周期**；opencode 的**逐文件 content-store** 对 lyra 是**已决不吸**（lyra checkpoint = gated 整仓 shadow-git 是定型的刻意选择，别重开 content-store rewrite / edited-files-only 辩论，filter③）。"stage→preview-diff→clear→commit"是**正交** UX 思想，可在不动整仓模型前提下叠加。与 [Grok G5 hunk-tracker](GROK.md) 有部分重叠——都涉及"改动的可视化 curation"，实现时统筹（hunk 级 accept/reject 是更细的 curation，revert-preview 是更粗的整批可逆预览）。
- **优先级**：**P2**。

---

### OC6 · doom_loop 守卫 —— 同工具+同入参连续 3 次 → ask（P1~P2）

- **来源/为什么**：`session/processor.ts`（`recentParts.slice(-3)` 全为同 tool 且 `JSON.stringify(input)` 相等 → `permission.ask({permission:"doom_loop"})`）。极廉价的死循环刹车，作为一类 first-class 可 ask 的权限。
- **目标**：模型陷入"重复相同调用"死循环时刹车、问用户，别把 context 烧在死循环里。
- **落点**：runtime tool-exec 前置检查。
- **风险/边界**：**这是 [Grok doom-loop](GROK.md)（provider 信号、当时判"观察项"）的可落地客户端版**——纯计数、几十行、无 provider 依赖。吸。
- **优先级**：**P1~P2**（小而实）。

---

## 2. 协议 / client-server 专项

opencode 协议 = OpenAPI 3.1 REST + SSE（Effect HttpApi），按 group 组织、`/doc` 发 spec 生成多语言 SDK。对照 lyra 自研 JSON-RPC：

- **REST/OpenAPI/codegen** —— 纯**形差**，lyra 已定案（拒 codegen、改 golden-sample drift gate）。**不吸**。
- **可断线重放的事件流**（事件带 `durable{aggregateID,seq,version}`；`SessionsCursor` + `after=seq` 重放）—— **部分吸/待确认**：若 lyra desktop event 通道重连时无 seq 重放则干净吸（P2，wire 层）；若已有 items.list seq 重放 + per-run last-event-id 则 **parity**。先核对 lyra 现状。
- **多项目 `location` 作用域**（一个 server 并发服务多项目目录、各自隔离）—— 真架构 delta 但改造大、非多租户诉求，**记 note 暂不吸**（除非 desktop 确需多项目并发）。
- **TUI 控制通道**（server 反向驱动客户端长轮询）—— lyra desktop 用 events+command 即 parity。**不吸**。
- **ACP 适配器**（`opencode acp`，stdio 接 Zed/JetBrains/Neovim）—— 额外**入站**适配器、不违反 lyra "runtime 协议不用 stdio"。**与 [Grok G17](GROK.md)+[Kimi K4](KIMI_CODE.md) 三方收敛**：一个 ACP 适配层让 lyra agent 可被第三方编辑器嵌入。**部分吸/defer**（生态期权，非核心，P3）。

---

## 3. 其余小候选

- **plugin hook seam 审计**（`session.compacting` 等 seam）—— in-process JS loader **不吸**（lyra subprocess-contract 更安全），但**审计这些 seam 点 lyra BeforeRound 是否都覆盖**，尤其 `session.compacting`（自定义 compaction 上下文/prompt）与 A1/A2/A3 天然配对。**部分吸**（只取 seam 清单），P2~P3。
- **background-job wait-timeout→promote**（前台跑、`wait(timeout)` 超阈值自动转后台，agent 继续）—— lyra 有 background commands + bash_output block，这是"跑着自动转后台"的 nuance。**部分吸**，P3。
- **PTY**（native pty + ring-buffer 绝对 cursor 重放 + ticket-websocket）—— lyra 明确"背景命令、无 PTY"。真能力缺口但价值窄（纯人向 UX：vim/htop/彩色 dev-server）；对 agent 自己非-PTY 更好。**部分吸/有条件**：仅当 lyra 想要"desktop 内嵌人操作终端面板"才做。P3，别高估。

---

## 4. 刻意不吸清单

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **share / 云同步公开链接** | `core/share`、`opncd.ai/s/<id>` | filter② 多租户云，违单机本地用户 |
| **Zen 网关** | `zen.mdx` | opencode 商业化托管 gateway；lyra 用户自带 key |
| **OAuth/credential/refresh 状态机** | `core/integration.ts`、`oauth/` | 反向不变量：不给 provider 加 OAuth/token refresh，用户填 key、401 UI 重填 |
| **retry / Transient 分类** | `llm/route/executor.ts` | 反向不变量：SDK 内建 retry 已够 |
| **in-process JS 插件 loader** | `packages/plugin` | lyra 刻意选 subprocess-contract（更安全、语言无关）；只薅 hook seam 清单 |
| **OpenAPI-codegen SDK / REST transport** | `packages/protocol` | lyra 已定案 JSON-RPC + golden-sample gate |
| **多项目 `location` 作用域** | `core/location-services.ts` | 真架构 delta 但大改、非多租户诉求，记 note |
| **provider policies** | `policies.mdx` | 单机用户对自配 provider 做 deny-list 属 YAGNI |
| **CodeMode 的解释器机器本体** | `codemode/interpreter` | ~4500 行 confined 解释器 + 手写 stdlib，违 KISS；只取"工具即代码"思想（OC1）|

---

## 5. 建议节奏

- **纯加法、原则强**：OC2（可刷新 system-context）。
- **小独立快赢**：OC3（models.dev vendor）、OC4（工具搜索，三方收敛，并入 [G4](GROK.md)/[CC1](CLAUDE_CODE.md) 实现）、OC6（doom_loop 守卫，Grok 观察项的可落地版）。
- **中等**：OC5（可撤销 revert 预览，与 [G5 hunk](GROK.md) 统筹）。
- **高智力/高成本，先写 spec**：OC1（CodeMode，与 [CX6](CODEX.md) 同向，MCP 编排成真瓶颈再评估）。
- **defer**：ACP 适配（与 [G17](GROK.md)/[K4](KIMI_CODE.md) 合并）。
