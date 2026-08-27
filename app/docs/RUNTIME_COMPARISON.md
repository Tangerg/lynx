# Runtime 能力对比 —— app/runtime vs 桌面 AI coding agent

> **对比对象**:`app/runtime`(ScopeApp Runtime,本仓后端)对 6 个主流编码 agent 的**后端/引擎能力**:
> **codex**(OpenAI,Rust)· **Claude Code**(Anthropic,TS)· **opencode**(sst,TS)· **Kimi Code**(Moonshot,TS)· **crush**(charmbracelet,**Go**)· **plandex**(**Go**)。
>
> **方法**:源码级核实(非文档/记忆)。peer 基线来自 2026-06 的桌面源码复核；ScopeApp 一侧已在 **2026-08-28 / P188** 重新核对当前生产代码。peer 未在本轮整体重扫，因此比较结论区分“当前 ScopeApp 事实”和“历史 peer 事实”，不把旧矩阵冒充同步时点。
> **桌面前端形态**(GUI/插件/原生体验)的对比见 [`DESKTOP_COMPARISON.md`](DESKTOP_COMPARISON.md);本篇只谈 runtime/引擎。
> **方法论**:对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) 与 [`../../CLAUDE.md`](../../CLAUDE.md) 的"库优于框架 / 薄核 / YAGNI / 不抄框架味"立场裁决"该不该学",而非见特性就抄。

---

## 0. TL;DR

**格局**:这 6 个 peer **全部以 ReAct/loop 或 staged-state-machine 为主**。ScopeApp 当前默认执行路径也是 managed Interaction 模型/Tool 状态机；独立 Agent Framework 另外提供 Planning/GOAP 与 Workflow Strategy，但它们不是默认 Runtime loop。ScopeApp 在 agent loop 健壮性、上下文/持久化、多 provider、可观测性和代码智能几条主线上处于第一梯队。

**真正的能力差(2026-06-23 刷新后)只剩 ~1.5 处,且都不是框架味:**
1. **跨平台 OS 沙箱广度** —— ScopeApp 已有 macOS Seatbelt、隔离 working copy、read-only/hidden roots 与 fail-closed assembly；当前真实差距是 codex/Claude Code 还覆盖 Linux/Windows，而 ScopeApp 非 Darwin 暂无 backend。
2. **权限/hooks 广度** —— 核心规则、审批、子进程 hooks 与 cron 已具备；P188 已把 PreCompact 接到真正的模型调用前 compaction boundary。残留主要是企业策略源、更多 hook event 与跨平台 sandbox。

其余 peer 独有项(codex 的 Guardian/code_mode/Realtime、plandex 的 per-plan git、多前端全家桶、V4A apply_patch)要么是值得借鉴的"思想"、要么是框架味/已被 scopeapp 等价能力覆盖的 by-design skip。详见 §0.5 + §10。

---

## 0.5 2026-06-23 历史刷新

> 本节保留 2026-06 的 peer 核对证据。ScopeApp 的当前结论以 §0、§1 和 2026-08-28/P188 就地修订为准。

**① 自上次基线后我方已补(原 gap #2 大半关闭):**
- **细粒度持久审批规则**:`Rule{scope(session/project/global), tool, subject-glob, decision(allow/deny)}` + sqlite,`approval.listRules/forgetRule`。从"单一 Mode stance"升级为 **4 模式(plan/safe/balanced/yolo)+ 声明式规则表**。
- **用户级 hooks**:子进程契约(exit-code / stdin-JSON,**无内嵌 VM**),配置级联(`~/.scopeapp` + 受信项目)；P188 后 PreCompact 已接到真实模型调用前压缩边界。
- **定时调度**:cron 触发的无人值守 headless 运行(`schedules.*` + worker)——原 §10"cron 归外层"的 by-design skip 已被真实需求推翻并落地。

**② codex 当前重核 —— 领先项仍集中在沙箱,其余多为实验 / 非编码核心:**
- **OS 沙箱**(硬实力):Seatbelt(mac)/ Landlock+seccomp+bwrap(Linux)/ Restricted-Token(Win);`FileSystemSandboxPolicy`(glob deny、`:workspace_roots` 等特殊路径、`.git/.codex` 元数据保护)、网络策略、拒绝可经审批升级。
- hooks 与我方同档(SessionStart/UserPromptSubmit/Pre·PostToolUse/PermissionRequest/Stop)。
- 新增/独有多为实验或非核心:**Guardian/AutoReview**(LLM 自动审批子 agent)、**code_mode**(模型写代码调工具,实验)、**Realtime WebRTC**(语音/实时)、**Profiles V2**、**app-server V2 协议**、**agent-graph-store**(SQLite 子 agent 拓扑 + thread-spawn 谱系)。
- 仍**无 LSP**、provider **OpenAI-centric** —— 我方此二项领先依旧。

**③ Claude Code 当前重核 —— 权限 / hooks / 可扩展最广:**
- **沙箱**:`@anthropic-ai/sandbox-runtime`(seatbelt / bubblewrap+landlock / WSL2),settings 驱动 fs/network allow·deny + git worktree 处理 + settings.json/.claude 写保护。
- **权限最细**:5 模式(default/plan/acceptEdits/bypassPermissions/dontAsk)+ 规则 DSL(`Bash(npm:*)`/`Edit(/src/**)`,含**显式 `ask`**)+ **多源**(policy-MDM/user/project/local/cli/session)。比我方多"显式 ask 规则 + 企业 MDM 源"。
- **hooks 最广**:18+ 事件(含 **PreCompact/PostCompact、SubagentStart、Elicitation、FileChanged、Worktree、Task 事件**)——比我方 6/7 更宽。
- **子 agent**:前台/后台 + **git worktree 隔离** + 自定义 `.claude/agents` + teams/coordinator + 跨 agent SendMessage。我方有 planner+并行+A2A+workflow+Supervisor,但**无 worktree 隔离**(靠 ConcurrencyKey + checkpoint 缓解)。
- **ToolSearch 工具延迟加载**(大工具池按需加载) —— 我方全量加载,MCP 工具多时可借鉴。
- LSP 现也是 config-driven(`lsp.servers`),与我方同档(非旧说的 plugin-only);仍无 scopeapp 的多 provider 广度(Anthropic-only,Bedrock/Vertex 仅网关)。

**④ NET 当前差距(对 codex / Claude Code):**
1. **OS 命令沙箱的跨平台广度**：ScopeApp 现有 macOS Seatbelt；codex / Claude Code 还覆盖 Linux/Windows。
2. **权限/hooks 的广度**：PreCompact 已补；剩余差异是企业策略源和更多事件类型。
3. **几个 situational/minor**:子 agent **worktree 隔离**(并行写隔离)、**ToolSearch 工具延迟加载**(MCP 池大时)、**暴露为 MCP server**(`mcp serve`,A2A 已覆盖大半)、**Guardian 式 LLM 自动审批**(可选,需谨慎防过度信任)。

> 当前一句话：macOS 沙箱、规则与调用前 PreCompact 已具备；下一安全差距是 Linux/Windows backend，而非“完全没有沙箱”。

---

## 1. 速查矩阵

> ✅ 有且成熟 · 🟡 部分/弱 · ❌ 无 · ★ 该项的标杆

| 维度 | codex | Claude Code | opencode | Kimi | crush | plandex | **app/runtime** |
|---|---|---|---|---|---|---|---|
| **架构形态** | CLI/TUI + app-server(JSON-RPC) | CLI/TUI + SDK + MCP-server | ★daemon + 多 client(HTTP/SSE) | TUI + ACP + RPC | TUI + 可选 client/server(socket REST) | daemon(REST/SSE)+PG | client-server,ScopeApp Protocol(JSON-RPC,HTTP+SSE/inproc)+独立 GUI |
| **agent loop** | streaming 状态机 | streaming | streaming ReAct(max25) | stateless(loop-detect) | streaming(loop-detect) | staged 状态机(auto-continue) | managed Interaction 状态机；可组合 Planning/GOAP、Workflow Strategy |
| **并行工具** | ✅RwLock 读写 | ✅cap10 | ✅fiber | ✅资源调度 | ✅opt-in | ❌ | ★✅**ConcurrencyKey 资源键**(避假冲突) |
| **工具错误恢复** | ✅fold 回模型 | ✅is_error | ✅catch 回模型 | ✅in-band | ✅文本错误 | 🟡validate-fix | ✅framework default(默认开) |
| **编辑安全** | V4A apply_patch | ✅read-before+stale(mtime) | ✅read-before+byte-stale | 🟡prompt-only | ✅read-before+stale | lazy-edit+builder | ✅read-before+stale(editguard) |
| **OS 沙箱** | ★✅Seatbelt/Landlock/Win | ★✅sandbox-runtime | ❌ | ❌ | ❌ | 🟡cgroup best-effort | 🟡macOS Seatbelt + isolated copy；Linux/Windows backend 暂缺 |
| **代码智能 LSP** | ❌ | 🟡LSP(config/plugin,9op) | ❌(V2 未移植) | ❌ | ✅LSP(powernap) | 🟡tree-sitter map(无 LSP) | ★✅**LSP(单 lsp 工具/8 操作 + lsp_diagnostics,config-driven server 表)** |
| **HITL/权限** | 多级 + ★Guardian(LLM 审批) | ★5 模式+allow/deny/ask(含 MDM) | rule DSL + question | policy chain + rule DSL + hooks | allowlist+safe-bypass+hooks | 5 级 autonomy(batch) | ✅R 模型 park/resume + 4 模式 + 规则表(scope/tool/subject,06-20) |
| **hooks 系统** | ✅(pre/post/compact/stop) | ★✅18+ 事件(可 block/rewrite) | ✅plugin hooks | ✅lifecycle hooks | ✅PreToolUse | ❌ | ✅子进程契约；P188 已接模型调用前 PreCompact |
| **多 agent/subagent** | ✅成熟(2 代协议,CSV fan-out) | ★✅深(subagent+teams/swarm) | 🟡弱(mention,无并行) | ✅swarm(128 并行,resumable) | 🟡并行但单类型(TODO) | 🟡model roles(无自主) | ✅**planner+并行(4 档 spawn)+workflow+Supervisor+A2A** |
| **上下文压缩+记忆** | ✅turn 内 compact+memory pipeline+AGENTS.md | ✅93%+CLAUDE.md+session memory | ✅compact+★Context Epoch+AGENTS.md | ✅compact+memory file | ✅summarize+多 memory 文件 | ★smart sliding window | ✅P188 调用前压缩+SQLite CAS rewrite+Strategy state同步+SCOPEAPP.md/提取 |
| **持久化/resume** | ✅rollout(SQLite FTS,fork) | ✅JSONL+resume+file rewind | ✅sqlite+resume(checkpoint TODO) | ✅jsonl replay+fork+export | ✅sqlite+resume(无 checkpoint) | ★PG+per-plan git(branch/rewind) | ✅SQLite+resume+影子 git checkpoint+fork+export |
| **MCP** | client(OAuth)+server | ★client(多 transport,OAuth/XAA)+server | client(OAuth) | client(OAuth) | client(header auth) | ❌ | client(+auth);**无 server 暴露**;+ ★**A2A 跨 runtime** |
| **多 provider** | 🟡4(OpenAI-centric) | 🟡Anthropic-only(Bedrock/Vertex 网关) | ✅10(models.dev) | ✅6 | ✅catwalk(数十) | ✅12+(LiteLLM) | ★✅**~40(显式配对)** |
| **plan 模式** | update_plan tool | ✅read-only+approval | ✅plan agent | ✅constrained writes | ❌ | ★全产品 plan-centric | ✅ |
| **可观测性** | ✅OTel+OTLP+metrics | ★✅OTel 三驾马车 | ✅OTel/OTLP | 🟡自研(无 OTel) | 🟡slog+PostHog(无 OTel) | ❌log.Printf | ★✅**OTel 三驾马车→slog** |
| **机器防腐(arch test)** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅(2026-06,见 [`../runtime/CLAUDE.md`](../runtime/CLAUDE.md)) |

---

## 2. 架构形态 —— `app/runtime` 与"协议驱动 client-server"一派同列,但走得最彻底

几乎所有现代 peer 都在走向**引擎与前端分离**,但程度不一:
- **codex** `app-server`:JSON-RPC over stdio/WebSocket,版本化协议,同一 Rust 核同时是 TUI / headless exec / daemon。
- **opencode** V2:后台 daemon 独占 sqlite 会话,OpenAPI HTTP + SSE,**多 client**(终端 TUI / Electron desktop / web / VSCode / Slack)并发驱动。**最接近 scopeapp 的形态。**
- **crush**:默认 in-process,`CRUSH_CLIENT_SERVER=1` 切换成 Unix socket 上的 Swagger REST,多 client。
- **plandex**:CLI ↔ Go daemon(REST+SSE)+ Postgres,有状态多租户(orgs/users)。
- **Claude Code**:CLI/TUI + 可嵌入 `query()` SDK + MCP-server 模式。
- **Kimi**:TUI + ACP(编辑器集成)+ 定义了 REST+WS 协议但当前仍 in-process RPC。

**`app/runtime` 的取舍**:从第一天就是**纯后端 runtime + 独立前端**,协议(ScopeApp Runtime Protocol,JSON-RPC 2.0,MCP-inspired)是一等公民,HTTP+SSE 与 inprocess 双 transport,streamable HTTP(per-run hub)。它不像 codex/crush 那样"默认 in-process、可选 server",而是**协议优先**——这让独立 Wails 桌面(`app/desktop`)、未来 Web/TUI 都是平权 client。**裁决:scopeapp 这条路线与 opencode 殊途同归且更纯(薄协议层 + arch_test 强制分层);不是 gap,是同一品味的更彻底版本。**

> 唯一形态差异:plandex/opencode 是**常驻多租户 daemon**(orgs/cloud),scopeapp 协议层**零 user 概念**(鉴权交给更外层)——这是 scopeapp 有意的 by-design,不追。

---

## 3. Agent loop & 工具 —— 默认 Interaction，Planning/Workflow 是可组合 Strategy

**loop 形态**:6 个 peer 里 5 个是 streaming ReAct/loop(codex 状态机、Claude Code/opencode/kimi/crush 循环),plandex 是 staged 状态机。ScopeApp 的生产默认同样是 managed Interaction 模型/Tool 状态机；Agent Framework 的 Planning/GOAP 与 Workflow 是可选择、可组合的第二和第三种 Strategy，不应被写成默认 planner-driven Runtime。这个边界避免把“库具备的策略”误报成“产品每个 Run 都采用的循环”。

**并行工具**:这是分水岭。
- ❌ plandex 根本不并行(staged 单流)。
- 🟡 codex(RwLock 读/写守卫)、Claude Code(concurrency-safe 分批,cap 10)是**粗粒度**——靠"只读/读写"二分。
- ✅ **Kimi(资源访问冲突图)和 `app/runtime`(ConcurrencyKey 资源键 + segmentEnd 冲突分组 + maxConcurrentToolCalls=8)是最精细的**——用**资源键**避免"两个读不同文件的 edit 被误判冲突"的假冲突。scopeapp 这条与 kimi 并列最优。

**loop 健壮性**:`app/runtime` 三件套齐全且默认开——max-iter(50)+ **LoopDetection**(SHA256 round-signature 固定点检测,先于 cap 触发)+ tool-error 默认 fold 回模型 + ToolNotFound 反馈自纠 + 空回复 nudge。crush/kimi 有 loop-detection;codex 靠 auto-compact + Guardian 断路器;**plandex 无并行也无 loop-detection**。scopeapp 在此与最强者持平、且 LoopDetection 的固定点检测是少数 peer 才有的。

---

## 4. 文件编辑 & 代码智能 —— `app/runtime` 在 LSP 阵营(少数派)

**编辑安全**:主流共识是 `app/runtime` 已落地的"**read-before-edit + stale 检测 + exact-string 替换**":
- Claude Code / crush / `app/runtime` 用 **mtime/时间戳 stale 检测**;opencode 用**字节级**比对;kimi 只靠 **prompt 约定**(无代码强制,是其弱项)。
- codex/opencode 额外支持 **V4A `apply_patch`**(`*** Begin Patch`),plandex 用 **lazy-edit + builder 合并 + tree-sitter 校验**(对大文件正确性投入最重)。
- **scopeapp 无 V4A** —— 但这是 by-design:editguard 的 read-before+stale 已保证安全,V4A 是另一种编辑表达而非安全增益。**不是 gap。**

**代码智能(LSP)是真正的分水岭**,且 `app/runtime` 站在少数派的正确一侧:
- ❌ **codex / kimi / plandex / opencode(V2) 都没有真 LSP** —— 全靠 ripgrep + glob(plandex 有 tree-sitter 符号 map 但无语言服务器)。
- ✅ **只有 crush(powernap)、Claude Code、`app/runtime` 有真 LSP**。其中 **Claude Code 的 LSP 服务器只能由 plugin 提供**(无 config 表),crush 自动按文件类型起;**`app/runtime` 是 config-driven server 表 + 6 个 lsp_* 操作 + edit 后 baseline-diff 诊断** —— 比 Claude Code 的 plugin-only 更可控。
- **裁决:代码智能是 scopeapp 的领先项**,不是要学的方向。

---

## 5. 命令执行 & 沙箱 —— macOS 已闭环，跨平台 backend 待补

ScopeApp 已不再裸跑所有 shell；当前差异是平台覆盖范围：

| | 沙箱 | 后台命令 |
|---|---|---|
| **codex** | ★macOS Seatbelt / Linux Landlock+seccomp+bwrap / Windows sandbox,`SandboxPolicy`(ReadOnly/WorkspaceWrite/...),拒绝时可经审批升级 | ✅ |
| **Claude Code** | ★`@anthropic-ai/sandbox-runtime`(seatbelt / bubblewrap+landlock / WSL2),settings 驱动 allow/deny | ✅run_in_background + 看门狗 |
| opencode | ❌host 直跑 | 🟡detached 但无 job 跟踪 |
| Kimi | ❌(经 Kaos 抽象,支持 SSH 远程但无 OS 隔离) | ✅BackgroundManager |
| crush | ❌in-process shell 直跑工作区 | ✅auto-background(60s)+job_output/kill |
| plandex | 🟡best-effort Linux cgroup/进程组(其余 OS no-op) | ✅daemon 后台任务 |
| **app/runtime** | 🟡opt-in macOS Seatbelt + hidden/read-only roots + isolated copy；其他平台 fail closed/无 backend | ✅process-group owned foreground/background + timeout/stop/join |

**裁决：当前 owner 与 fail-closed 方向正确。** 后续不是重建抽象，而是在现有 platform file边界补 Linux Landlock/seccomp 与 Windows restricted-token backend，并复用同一 writable/read-only/hidden contract。

---

## 6. HITL / 权限 / hooks —— ✅ 核心已补齐(06-20/06-21),残留只是广度

> **更新**:本节旧结论"scopeapp 偏粗 / 无 hooks"已过时——见 §0.5 ①。scopeapp 现有 **4 模式 + `Rule{scope,tool,subject,decision}` 规则表 + 用户级子进程 hooks(6/7 seam)**。下文保留 peer 细节作对照,裁决已改写。

`app/runtime` 的 HITL **模型**很强(R 模型:park-on-interrupt + 跨重启 resume + 在 pending tool call 处续 + atomic Consume 幂等),`Interrupt[R]` 单泛型也比 peer 的 sealed 子型层级干净。**权限/审批已从单一 `Mode` stance 升级为"4 模式 + 声明式规则表"**;peer 细节对照:

- **Claude Code**:5 种模式(default/acceptEdits/plan/bypass/dontAsk)+ `allow/deny/ask` 规则(`Bash(npm:*)` 形态,policy/user/project/local 多源)+ **26 种 hooks**(PreToolUse 可 **block 或 rewrite** 工具调用)+ MDM 企业策略。
- **Kimi**:`manual/auto/yolo` 3 模式 + 首匹配 policy 管线 + 规则 DSL(`Read(/etc/**)`)+ 会话级"本次会话批准"缓存 + lifecycle hooks。
- **crush**:静态 allowlist + per-session 持久授权 + **安全只读命令 bypass**(`ls`/`git status` 免提示)+ PreToolUse hooks + yolo。
- **opencode**:`Rule{action,resource,effect:allow|deny|ask}` 有序规则 + reject-with-feedback 转成模型 steering。
- **codex**:`AskForApproval` 多级 + **Guardian**(独立 LLM 会话自动裁决审批,新颖)。

**裁决:原"两条值得学"已落地(06-20/06-21),只剩广度残留——**
1. ~~细粒度权限规则~~ **已做**:`Rule{scope(session/project/global), tool, subject-glob, decision}` + sqlite,且保持 approval **单一 Service**(未拆 Console/Gate)。残留:claude code 有**显式 `ask` 规则 + 企业 MDM 源**,我方暂只 allow/deny(ask 由 Mode 兜底)。
2. ~~用户可配 hooks~~ **已做**:用户级子进程 hooks(`~/.scopeapp` + 受信项目,exit-code/stdin-JSON,可 block/rewrite/注入)；P188 已补真正调用前 PreCompact。残留是事件与策略源广度。
- crush 的"**安全只读命令免审批**"(`safeCommands` 白名单)仍是低成本小改进,可作为规则表的内置默认借鉴。

---

## 7. 多 agent / 上下文 / 持久化 —— `app/runtime` 全面领先

**多 agent**:`app/runtime`(planner + 并行 subagent + 4 档 spawn 梯度 + workflow over sub-agent + Supervisor + **A2A 跨 runtime**)与 **Kimi(128 并行 swarm)、codex(2 代协议 + CSV fan-out)同属最强档**,远超 opencode(mention 无并行)、crush(单类型 TODO)、plandex(model roles 非自主)。**A2A 跨 runtime 协议是 scopeapp 独有**(无 peer 有跨进程 agent 协议)。

**上下文管理**:都做 auto-compaction + 项目记忆文件。P188 后 ScopeApp 不再只在 Run 结束后压缩：每次主模型调用前按本次 exact model window 预算，保留 instructions、Tool/options 固定开销和未提交输入，summary/trim 完整成功后再以 SQLite CAS transaction重写 durable history与 Run watermarks，并把 effective messages写回 Agent Strategy state。Tool 的 exact model result与客户端展示分别投影，fresh opening持久化 hook composer实际交给模型的完整 User message；恢复只规范化语义等价的相邻 Tool grouping而不放松 payload核对。这样单个长 Run、Tool loop、Goal/HITL、下一 fresh Run和恢复都从同一前缀继续；provider-only request patch、presentation反推或 history/WorkingContext双真相源均不存在。

**持久化/resume**:`app/runtime`(SQLite + durable cross-restart resume + **影子 git 文件 checkpoint/rollback** + fork + export/import)是头部水平。checkpoint 维度:plandex 的 **per-plan git repo(branch + rewind + 版本控制整个 plan)**最强、Claude Code 的 **/rewind file-history** 与 scopeapp 的 gated whole-repo 影子 git 同思路;crush/kimi/opencode 在 checkpoint 上更弱(crush 无、opencode TODO)。**scopeapp 在此领先多数 peer。**

---

## 8. 多 provider / MCP / 可观测性

- **多 provider**:`app/runtime` ~40 provider(显式 `runs.start{provider,model}` 配对、provider 不从 model 推断)是**最广之一**,与 crush(catwalk)、opencode(models.dev)、plandex(LiteLLM 12+)同档,**远超 codex(4,OpenAI-centric)、Claude Code(仅 Anthropic,Bedrock/Vertex 只是网关)**。领先项。
- **MCP**:`app/runtime` 是 client(+auth);**但不把自己暴露成 MCP server**(codex/Claude Code 有 `mcp serve`)。补一个"scopeapp-as-MCP-server"能让 scopeapp 被其他 agent 当工具用——**可考虑,低优先级**。scopeapp 的 **A2A(client+server)** 覆盖了"跨 agent 协作"的更大场景。
- **可观测性**:`app/runtime` 的 **OTel 三驾马车→slog** 与 codex/Claude Code/opencode 同属第一梯队,**远超 kimi(自研无 OTel)、crush(slog+PostHog 无 OTel)、plandex(只有 log.Printf)**。领先项。

---

## 9. 双向独有清单

**`app/runtime` 独有 / 罕见(peer 全无或仅 1-2 个有):**
- **同一 Kernel 下可替换的 Interaction、Planning/GOAP 与 Workflow Strategy** —— 默认产品路径不冒充 planner-driven。
- **A2A 跨 runtime 协议**(client+server) —— 无 peer 有。
- **ConcurrencyKey 资源键并行** —— 仅 Kimi 有近似的资源调度。
- **config-driven LSP server 表 + 6 操作** —— crush/Claude Code 有 LSP 但形态更受限。
- **arch_test 机器防腐** —— 全部 6 个 peer 都没有。
- **协议优先 + 独立富 GUI** —— opencode 形态最近,但 scopeapp 协议层更薄更纯。

**peer 独有 / 领先(`app/runtime` 没有,2026-06-23 口径):**
- **Linux/Windows 沙箱 backend**(codex/Claude Code)—— ScopeApp 当前只有 macOS Seatbelt。
- **权限/hooks 的广度**:claude code 规则 DSL 含**显式 `ask` + 企业 MDM**和更多事件；ScopeApp 已有规则、子进程 hooks 与 PreCompact，差在广度。
- **子 agent worktree 隔离 / ToolSearch 工具延迟加载**(Claude Code)、**Guardian LLM 自动审批**(codex)—— situational / 新颖思想。
- **code_mode / Realtime WebRTC**(codex 实验)、**CSV fan-out / 128-swarm**(codex/kimi)、**per-plan git**(plandex)、**Context Epoch**(opencode)、**session 全文搜索**(codex SQLite FTS)、**暴露为 MCP server**(codex/Claude Code `mcp serve`)。

---

## 10. 该学什么 —— 批判性裁决(2026-06-23 刷新)

原 3 项里**中优先级的"细粒度权限规则 + 用户 hooks"已落地**(见 §0.5 ①),当前只剩:

| 优先级 | 学什么 | 来源 | 怎么落地(scopeapp 方式) | 为什么值得 |
|---|---|---|---|---|
| **高** | **补齐 Linux/Windows OS 沙箱 backend** | codex / Claude Code | 保留当前 macOS Seatbelt/isolated-copy owner，在 Linux/Windows 以平台文件实现同一 fail-closed contract | macOS 已不是 gap；跨平台 shipping 前必须让同一配置不降级裸跑 |
| **低** | **补 hooks/策略源广度** | Claude Code | PreCompact 已完成；后续只按真实消费者补企业策略源或新 event | 核心已齐，不为追数量机械扩表 |
| **低** | **子 agent worktree 隔离** + **ToolSearch 工具延迟加载** | Claude Code | 并行子 agent 各自 git worktree(避免并行写冲突,现靠 ConcurrencyKey+checkpoint 缓解);MCP 工具池大时按需加载工具 | situational:前者利于并行写安全,后者在 MCP 工具多时省 prompt |
| **低** | **Guardian 式 LLM 自动审批** + **scopeapp-as-MCP-server** | codex | 可选 LLM reviewer 在 on-request 审批自动裁决(fail-closed+断路器);把 scopeapp 暴露成 MCP 工具 | Guardian 减少无人值守打断(谨慎防过度信任);MCP-server 让 scopeapp 被别 agent 复用(A2A 已覆盖大半) |

**明确不学(框架味 / 已有等价 / by-design / 低需求):**
- **code_mode**(codex):实验性、复杂(沙箱 JS 解释器),YAGNI。
- **Realtime WebRTC / 语音**(codex):非编码 agent 核心,YAGNI。
- **多前端全家桶**(opencode 的 Slack/web/VSCode + codex 的 cloud):scopeapp 协议已支持多 client,真出需求再加前端即可,不为对齐而做。
- **V4A apply_patch**(codex/opencode):editguard 已保证编辑安全,V4A 是另一种表达非安全增益。
- **per-plan git / staged 状态机**(plandex):scopeapp 的 planner + 影子 git checkpoint 已覆盖其"可回溯、可分支"的价值,且 scopeapp 的 ReAct-free planner 更通用。
- **Context Epoch**(opencode):是 token 成本优化,可作为未来 compaction 的参考思路,但非能力 gap。
- 注:**cron 自调度**原列"不学",但真实需求推翻——scopeapp 已落地 `schedules.*`(见 §0.5 ①)。

---

## 一句话定档

**截至 2026-08-28 / P188，ScopeApp 的生产默认是 managed Interaction，Planning/GOAP 与 Workflow 是同一 Kernel 上的可组合 Strategy；不再误报为“唯一 planner-driven Runtime”。macOS Seatbelt、权限规则、hooks（含真正调用前 PreCompact）和 durable mid-Run compaction均已落地。当前明确差距主要是 Linux/Windows sandbox backend与少数企业级策略源；继续巩固协议优先、薄核、exact durable identity和长运行稳定性，不追 framework 全家桶。**

---

*对比基线 2026-06-19;**codex + Claude Code 于 2026-06-23 以当前桌面源码(`~/Desktop/{codex,claude_code}`)重核刷新**(见 §0.5),其余 peer 仍为 06-19 口径。各 peer 能力经其桌面源码第一手核实,Claude Code 经反编译 TS 快照核实(部分区域中等置信)。本篇对应桌面前端形态对比 [`DESKTOP_COMPARISON.md`](DESKTOP_COMPARISON.md)。*
