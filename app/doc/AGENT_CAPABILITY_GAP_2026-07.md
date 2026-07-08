# Agent 能力差距 — 2026-07（vs codex / opencode / kimi-code / crush）

> **目的**：以「**agent 能力**」为镜头（不是 UX）盘点 lyra 相对四个桌面 CLI coding agent 还差什么、哪些是刻意分歧、哪些地方我们已领先。为后续「取其精华」提供一张可执行的差距地图。
>
> **方法**：对每个竞品的源码做第一手枚举（工具面 / loop / 上下文 / 自治 / 子 agent / 会话 / 模型 / 扩展 / 独特能力 9 维），再对 lyra 现状（后端 `kernel/toolset` 工具装配 + `internal/delivery/protocol` 方法面 + `agent/` SDK）逐项对照。竞品源码克隆在 `~/Desktop/{codex,opencode,kimi-code,crush}`。基线 **2026-07-08**。
>
> **与 `COMPETITIVE_GAP_2026-06.md` 的关系**：那份偏 **UX / 前端 surface**（composer chip、steer 气泡、diff、history），且 T1/T2 多已落地。**本文件是它的 agent-能力侧补集**——只谈"agent 能做什么"这一层。哲学/反不变量以根 `CLAUDE.md` + `app/runtime/CLAUDE.md` 为准。

---

## 0. 一句话结论

lyra 在**会话与状态**（检查点回滚 / fork / 导出导入 / durable resume）、**语义 RAG 索引**、**plan 模式**、**持久审批规则**、**per-role 模型**、**MCP OAuth**、**LSP 代码智能**上，**领先或打平全部四家**。真正的能力短板集中在四处：**① 编辑/执行原语的"力度"**、**② 工具经济学**、**③ 几个便利工具**、**④ 并行编排**；外加一个值得重估的哲学点——**agent 自我沙箱**。

---

## 1. lyra 已经领先（先立信心，避免误判为差距）

| 能力 | 竞品状况 |
|---|---|
| 文件检查点 + 回滚（shadow-git 整库，gated） | crush 只有文件版本链（仅供 diff）；kimi 只有对话 undo；codex/opencode 只有 turn-count / git-snapshot per step |
| 语义 RAG 代码索引（`codebase_search`，sqlite-blob 向量 + Go cosine） | **四家全缺**（都只有 grep/glob/sourcegraph） |
| plan 模式（只读调研 → 提交计划 → 批准转执行） | codex/opencode/kimi 有对等物；crush **无** |
| 持久审批规则（`Rule{scope,tool,subject,decision}` + sqlite，重启存活） | crush 授权仅进程内、会话级；opencode "always" 主要在内存 ask 路径 |
| per-role 模型（utility / embedding 角色，运行态可变 + 持久化） | crush 有 large/small；kimi **无**（子 agent/压缩都继承主模型）；codex 有 review/memory 角色 |
| LSP 代码智能（`lsp` 8 ops + `lsp_diagnostics` 改后回灌） | crush 部分（references/diagnostics/restart）；**kimi 无、codex 无**（纯 shell+grep） |
| MCP OAuth 2.1（DCR + PKCE + loopback callback） | crush **无**（仅 header/bearer）；opencode/kimi/codex 有 |
| A2A 远程 agent 委派（远程 agent 折成一个工具） | 四家均无对等物 |
| 真·mid-run steer（core `BeforeRound` 续跑钩子注入用户轮） | codex/kimi 有队列；crush 折入当前轮；opencode V1 仅"加入运行" |

---

## 2. 真差距矩阵（按 价值/成本 排序；标注哪些竞品有 + 分类）

### 2.A Tier A —— 低成本、明显该补（编辑/工具原语层）

| 差距 | 谁有 | 说明 / 为何值得 |
|---|---|---|
| **`apply_patch` 多文件补丁**（增/删/改/**移动**一次调用 + 免行号模糊匹配） | codex（`apply-patch/` Lark 语法 + `seek_sequence` 模糊）、opencode（`tool/apply_patch.ts` OpenAI 格式，gpt- 模型自动切换） | lyra 只有单文件 `edit`+`write`；跨文件重构要 N 次调用、无文件移动。codex 的 `*** Begin Patch` 免行号格式对模型很鲁棒 |
| **`multiedit`**（单文件多处编辑一次调用，各自 `replace_all`） | crush（`tools/multiedit.go`）；opencode 由 apply_patch 覆盖 | lyra 单 `edit` 一次一处；一个文件多处改要多轮 |
| **写入即 auto-format**（按语言 formatter，诊断前跑） | opencode（`format/formatter.ts`，~25 formatter，写/改/patch 前都跑） | lyra 已有改后 LSP 诊断守卫栈，**但不自动格式化**；接进现有 editguard 守卫栈即可，产出即整洁、少一轮"格式化"往返 |
| **`sourcegraph` / 公共仓库代码搜** | crush（`tools/sourcegraph.go`，GraphQL regex/lang/repo/symbol） | 查"别的项目怎么用这个 API/库"；零凭据、低成本 |
| **`download`**（URL → 文件，二进制安全，流式） | crush（`tools/download.go`，max 600s） | lyra `web_fetch` 只出 markdown；下载依赖/二进制资产做不到 |
| **模型自调 cron 工具**（`CronCreate/List/Delete`） | kimi（`tools/cron/`，5 段 cron，thundering-herd jitter，仅 idle 触发） | lyra **已有 scheduler 子系统**，只是用户侧（RPC/UI）；暴露成模型工具即可，让 agent 自排未来任务 |
| **hook 事件补全**（PostToolUse / SubagentStart·Stop / PreCompact / PermissionRequest） | kimi（16 事件）、codex（10 事件） | lyra hook 系统已在（PreToolUse/UserPromptSubmit/SessionStart/Stop/Notification/…），缺后置 + 子 agent + 压缩前几个 seam；纯扩表 |

### 2.B Tier B —— 中成本、真能力增量

| 差距 | 谁有 | 说明 |
|---|---|---|
| **持久交互 PTY / write-stdin**（活的伪终端跨调用 + 写 stdin + 信号 + resize） | codex（`unified_exec/` `exec_command`+`write_stdin`，≤64 进程） | lyra shell 每次全新、只能后台 + 读输出，**不能向活进程写 stdin**；REPL / 交互式安装器 / `git rebase -i` / 需回答提示的命令做不了。是唯一的"交互式执行"硬缺口 |
| **渐进工具披露**（按需加载工具 schema） | codex（`tool_search` BM25 over deferred tools）、kimi（`select_tools` 走 Moonshot message-level tools） | lyra 一次性挂全部 MCP 工具；server 多时白吃 context。lyra `skill` 的 list/load 已是渐进披露思路，可推广到工具面 |
| **并行 swarm 编排**（fan-out N 子任务 + 429 背压调度） | kimi（`AgentSwarm`，128 并行 + 斜坡启动 + 自适应退避）、codex（`spawn_agents_on_csv`，默认 16 / max 64） | lyra `agent/workflow` **已有** Parallel/ScatterGather builder + 预算树，但**模型侧 `task` 是单发、不可递归**；缺一个 fan-out 工具 + 背压 |
| **`code_mode` / `execute`**（模型写 JS 在受限运行时里编排嵌套工具调用） | codex（`code-mode/` V8 isolate，`exec`+`wait`）、opencode（`tool/code-mode.ts`，MCP 工具变可调函数） | 把多轮工具调用压成一轮，省 round-trip + context。**权衡**：引入 JS 解释器，与 lyra 干净工具模型有张力，KISS 上要掂量；建议观望或仅在 MCP 重场景评估 |
| **agent 自省工具**（查自身运行态 + 读自身日志排障） | crush（`crush_info` 模型/LSP/MCP/hook 状态 + `crush_logs` 读自身日志、脱敏） | 让模型自查"我挂了哪个 provider/LSP/MCP"，减少人肉排查；lyra 有 OTel 但没暴露给模型 |
| **本地模型自动发现**（`/v1/models` + ollama/lmstudio/litellm 探测） | crush（`internal/discover/`） | lyra provider 注册表要手填；局域模型自动上架是便利 |

### 2.C Tier C —— 战略 / 较大，视产品方向定

| 差距 | 谁有 | 说明 |
|---|---|---|
| **ACP server**（Agent Client Protocol，给 Zed/JetBrains 驱动） | kimi（`packages/acp-adapter`）、opencode（`cli/cmd/acp.ts`） | lyra 有自有协议 + transport（HTTP+SSE / inprocess），但没实现 ACP；是否嵌入第三方编辑器取决于产品方向 |
| **插件 / 市场**（一个包捆 skills+MCP+hooks+commands + 信任层级 official/curated） | kimi（`plugin/` + marketplace）、codex（`plugin/manifest.rs`） | lyra 这些能力**都有、但分散**（skills/recipes/hooks/MCP 各一套）；缺统一分发单元 + 信任模型 |
| **结构化 `/review` 子 agent**（锁死工具 + rubric → file:line findings） | codex（`tasks/review.rs`，review_model，web/collab 关、approval Never） | lyra 可用 recipe 逼近，但没内建的结构化审查产物 |
| **视频输入**（`ReadMediaFile` 读视频，非仅图片） | kimi（`read-media.ts` + Kimi `uploadVideo`→`ms://`） | lyra 图片输入已有，`read_media` 已 parked；视频更远，Kimi/Gemini/Vertex 才声明 `video_in` |

---

## 3. 刻意分歧 / 明确不抄（符合 lyra 哲学）

- **cloud tasks / best-of-N / share links / 多客户端共享 workspace**（codex/opencode/crush）——lyra 是本地单用户桌面，协议层零 user 概念，这些是"团队/云"形态，**刻意不做**（根 CLAUDE.md + app/runtime 反不变量）。
- **Guardian LLM 自动审批**（codex：第二个模型裁决审批，fail-closed + 熔断）——本地场景人就在场，自动裁决边际收益低，略过度。
- **provider OAuth / token refresh**（多家）——lyra 反不变量：填 API key，401 让 UI 提示重填。
- **catwalk ~35 provider 广度**（crush）——lyra 21 个数据驱动 provider 表，按需加，不追广度。
- **换 Chat Completions-only 或 Responses-only**——codex 只认 Responses API（连不上通用 OpenAI-compatible 端点）是它的**收窄**，lyra 多 provider × 兼容端点是有意保留的更宽面。

---

## 4. ⚠️ 值得重估的哲学点：**agent 自我沙箱**

codex 的 OS 沙箱是四家里最硬的能力（macOS Seatbelt SBPL / Linux bubblewrap+seccomp / Windows WFP+restricted-token，**fail-closed** + 升级流 + 网络代理 MITM + 凭据 broker），lyra **完全没有**。lyra CLAUDE.md 把它归入"本地单用户 / OS 信任 → 沙箱是 ceremony"。

**要点区分**：codex 沙箱防的是**"agent 伤害用户机器"**（幻觉/注入出的 `rm -rf`、越权写、意外外联），这跟"多租户鉴权"是**两码事**。lyra 现在以**审批模式 + 路径守卫**为主，一条被批准的 shell 命令内部仍是完全放开的。

**判断**：这不是要求做多租户/鉴权（那确实不做），而是——**"限制 agent 自己那条 shell 的爆炸半径"是否值得从 OS 层补一道**（哪怕只做 macOS Seatbelt 只读默认 + 可写根 + 网络门）。**值得作为独立议题重估**，不宜无脑归入"不做"。涉及新的 infra 子系统，落手前先咨询。

---

## 5. 建议落地短名单（若要动手）

1. **Tier A 全打包**：`apply_patch`(多文件) / `multiedit` + auto-format + `sourcegraph` + `download` + cron 工具 + hook 补全——都低成本、强对齐、即时提升 agent 干活力度，各自独立 commit。
2. **持久 PTY / write-stdin**（Tier B 之首）——补上唯一的"交互式执行"硬缺口。
3. **渐进工具披露 + swarm 工具**——工具经济学 + 并行（lyra 底层 workflow builder 已就绪，主要是模型侧暴露 + 背压）。
4. **agent 自沙箱**单独立项讨论（先咨询，涉及 OS 层新子系统）。

`code_mode` / ACP / 插件市场 / 视频 / `/review` 归为**观望**：要么与 KISS 有张力，要么取决于产品方向，不在第一波。

---

## 6. 附录：各竞品最独特能力（evidence，供后续深挖）

**codex（Rust，`codex-rs/`）**
- `code_mode`：模型写 JS 在 V8 isolate 编排嵌套工具调用（`code-mode/`, `core/src/tools/code_mode/`）。
- 多 OS in-process 沙箱 + 升级流（`sandboxing/`, `linux-sandbox/`, `windows-sandbox-rs/`, `shell-escalation/`）。
- Guardian LLM 自动审批 fail-closed + 熔断（`core/src/guardian/`）。
- 网络代理 MITM + 凭据 broker（模型永不见真实密钥）（`network-proxy/`）。
- cloud tasks best-of-N + preflight apply（`cloud-tasks/`）。
- `apply_patch` 免行号模糊补丁、`tool_search` BM25 延迟加载、`unified_exec` 持久 PTY、`/review`、跨会话 `memories` 管线、外部 agent(Claude Code) 迁移、staged feature gating。
- **只有** shell/apply_patch/view_image，**无**通用 read/write/grep/glob/LSP 工具（全走 shell）；**无**语义 RAG；**Responses-API only**。

**opencode（TS，`packages/`）**
- 写入即 auto-format（~25 formatter）+ 每次 edit/write/patch 回灌 LSP 诊断（~38 language server 自动下载）（`format/formatter.ts`, `lsp/server.ts`）。
- tree-sitter 命令权限：AST 解析出 arity（批准 `git status` → 学 `git *`）（`tool/shell.ts`, `permission/arity.ts`）。
- `code-mode`（MCP 工具变可调 JS 函数）、9 策略模糊 edit、gpt- 模型 apply_patch 自动切换。
- server-first + 生成 SDK + ACP + GitHub/PR 命令 + share links；MCP OAuth + DCR。
- agent/命令/skill 全是 markdown+frontmatter；per-model system prompt 变体。
- **无**语义 RAG（grep/glob/LSP）。

**kimi-code（TS，MoonshotAI，非 fork）**
- 预算化 **Goal 系统**：objective + completionCriterion + turn/token/wallclock 预算 → 自主续跑（`agent/goal/`, `tools/builtin/goal/`）。
- 视频输入（`ReadMediaFile` + `uploadVideo`→`ms://`）。
- 模型自调 cron（`tools/cron/`）。
- **Swarm**：128 并行子 agent + 429 背压调度（`session/subagent-batch.ts`）。
- 渐进工具披露 `select_tools`（Moonshot message-level tools）。
- ACP server、插件市场（信任层级）、16 生命周期 hook、TOML 配置、background subagent + notification + "by the way" 侧信道 Q&A。
- **无**检查点/回滚（仅对话 undo）、**无** LSP、**无** RAG、**无** per-role 模型（**这些 lyra 全有**）。

**crush（Go，Charmbracelet）**
- 多客户端共享 workspace server（多个 client 按 cwd attach 一个 backend）。
- catwalk 外部 provider 注册表（~35 provider 为数据非代码）+ 本地模型自动发现。
- agent 自省工具 `crush_info`/`crush_logs`。
- 两个专用子 agent：只读 `task` 搜索 + 小模型 `agentic_fetch` web 研究（各自可并行、成本上卷父会话）。
- `sourcegraph` 公共仓库搜、`download`、`ls` 树列、`multiedit`、跨平台 shell（mvdan/sh）、PreToolUse hook 可改写工具输入 / 停轮、Docker MCP gateway。
- **无** fork / 导出导入 / 检查点回滚 / plan 模式 / 持久审批规则 / RAG / MCP OAuth / 自定义 agent（**这些 lyra 全有**）。
