# Lyra — Agent 能力横向对比

> **日期**：2026-06-11。**基线**：lyra HEAD（含 LSP / session export-import / edit 守卫 / 文件 checkpoint / 后台命令 五项落地后）。
> **对比对象**：桌面源码仓 `claude_code`（Claude Code CLI）、`codex`（OpenAI Codex CLI, Rust）、`opencode`、`cline`（VS Code 扩展）、`kimi-code`（Moonshot），外加 `aider` / `cursor` / `windsurf` / `amp` / `gemini-cli` 的公开认知。
> **方法**：对每个仓库实地清点能力面（工具名 / 特性 / 架构），再与 lyra 现状拉成矩阵。

---

## 0. 定位校正（公平比较的前提）

Lyra 是**运行时后端**（协议层，经 Lyra Runtime Protocol 服务独立前端），竞品多是**完整产品**（CLI / IDE 扩展）。因此：

- **属于前端、不计入 lyra 缺口**：slash 命令、TUI / Web UI、输出样式（output styles）、diff 渲染、自动补全（autocomplete）、IDE 深集成。
- **真正可比的是运行时能力**：工具集、上下文/记忆、安全、会话、扩展机制、多 agent、可观测。

本文只在**运行时层**判定缺口。

---

## 1. 能力矩阵

✅ 有 · 🟡 部分 · ❌ 无

| 维度 | lyra | claude_code | codex | opencode | cline | kimi-code |
|---|---|---|---|---|---|---|
| read / write / edit | ✅（edit 有 read-before + stale 守卫） | ✅ | 🟡（仅 apply_patch） | ✅ | ✅ | ✅ |
| 多文件 / apply_patch | ❌（刻意不做，见 §4） | ❌ | ✅ V4A | ✅ | ✅ | ❌ |
| notebook 编辑 | ❌ | ✅ | 🟡 | ❌ | ❌ | ❌ |
| grep / glob | ✅ | ✅ | 🟡（走 shell） | ✅ | ✅ | ✅ |
| **LSP 代码智能** | ✅ 6 op | ✅ 9 op | ❌ | ✅ 9 op | 🟡（插件示例） | ❌ |
| bash 同步 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **后台命令** | ✅（无 PTY） | ✅ | ✅（PTY） | ✅ | 🟡 | ✅ |
| web fetch / search | ✅（+httpreq） | ✅ | 🟡（仅 search） | ✅ | 🟡（仅 fetch） | ✅ |
| **todo / plan 工具** | 🟡（有 plan 模式，无 model-facing todo） | ✅ | ✅ | ✅ | ✅（plan 模式） | ✅（todo + goal） |
| 持久记忆（*.md） | ✅ LYRA.md + AGENTS.md | ✅ CLAUDE.md | ✅ AGENTS.md | ✅ AGENTS.md | ✅ .clinerules | ✅ AGENTS.md |
| 自动压缩 | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ |
| 子 agent 委派 | ✅（单个） | ✅ | ✅ | ✅ | ✅ | ✅ |
| **多 agent swarm / team** | ❌ | ✅ teams | ✅ agent_jobs | 🟡 | ✅ 16 team tools | ✅ swarm |
| MCP client | ✅（5 态生命周期） | ✅ | ✅ | ✅ | ✅ | ✅ |
| **MCP OAuth** | ❌（seam 已备） | ✅ | ✅ | ✅ | ✅ | ✅ |
| MCP server（自身作为） | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **A2A（agent 互联）** | ✅ | ❌ | 🟡（进程内 multi-agent） | ❌ | ❌ | ❌ |
| **多模态（图片输入）** | ❌ | ✅ | ✅ | ✅ | ✅ | ✅（+视频） |
| 图片输出 | ❌ | ✅ | ✅（生成） | ❌ | ❌ | ❌ |
| **Hooks（pre/post tool）** | ❌ | ✅ 8+ 事件 | ✅ 10 事件 | 🟡（插件 hook） | ✅ | ✅ |
| **OS Sandbox** | ❌（有审批兜底） | 🟡 可选 | ✅ 三平台 | ❌ | ❌ | 🟡 路径限制 |
| 审批 / 权限 | ✅ R 模型 4 档 | ✅ | ✅ | ✅ | ✅ | ✅ |
| session export / import | ✅ | 🟡（仅 export） | ❌ | ✅ | 🟡（仅 export） | 🟡（仅 export） |
| fork | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **文件 checkpoint / restore** | ✅ 影子 git | 🟡（file history） | 🟡（compaction 边界） | ❌ | ✅ 影子 git | ❌ |
| skills | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ |
| 多 provider × model | ✅ 显式配对 | ❌（Anthropic） | ❌（OpenAI） | ✅ | ✅ | 🟡 |
| cron / 调度 | ❌ | ✅ | 🟡（cloud-tasks） | ❌ | ❌ | ✅ |
| **OTel 可观测（traces+metrics+logs）** | ✅ 三件套 | 🟡 | 🟡（trace） | 🟡 | ❌ | 🟡 |

---

## 2. Lyra 的真正缺口（运行时层，按价值/成本排序）

### 第一梯队 —— 平台无关、价值高、竞品几乎全有

1. **多模态图片输入** —— **5/5 全有，唯独 lyra 没有**。最显眼的单点缺口。截图 / 设计稿 / 报错图驱动是主流用法。平台无关：牵协议 content block + 附件存储 + model adapter 的 image content。
2. **Hooks（pre/post tool 用户脚本）** —— 4/5 有（claude_code 8 事件、codex 10 事件、cline 子进程 + 文件 IPC、kimi session hooks）。扩展性核心：lint / 格式化 / 拦截 / 审计。平台无关（exec）。lyra 当前零扩展钩子。
3. **model-facing todo 工具** —— 5/5 有。lyra 有 plan 模式，但没有"模型自维护的 live todo list"。**成本极低**（一个会话态状态工具），对长任务连贯性立竿见影。
4. **MCP OAuth** —— 5/5 有；lyra 的 5 态生命周期已含 `needsAuth`，seam 已备好（`internal/engine/mcp.go`），接入是"填空"非"重构"：ServerConfig 加 auth 字段 + dial 识别 401 → needsAuth + OAuth 回调端点 + token 存储。

### 第二梯队 —— 价值中、偏进阶

5. **多 agent swarm / 并行 team** —— 4/5 有（cline 16 个 team 工具、kimi `AgentSwarm`、codex `agent_jobs`、claude_code teams）。lyra 只有单个 `task` 委派。lyra 的 agent runtime + A2A 是好地基。
6. **LSP 操作补全** —— lyra 6 op；claude_code / opencode 9 op（缺 `goToImplementation` + call hierarchy in/out）。纯加法、低成本。
7. **语义代码检索 / repo-map** —— lyra 有 `rag` 模块但**未接进 agent**；aider 的 tree-sitter repo-map、cursor/windsurf 的 embeddings 全仓索引印证此维度有价值。

### 第三梯队 —— 平台相关或小众

8. **OS Sandbox** —— 仅 codex 全做、claude_code 可选；**平台强相关**（每 OS 一套），已论证暂缓（审批模型 = claude_code 的主防御，lyra 已有）。
9. **notebook 编辑 / 图片输出 / cron 调度** —— 小众，按需。
10. **apply_patch** —— **刻意不做**（见 §4），非缺口。

---

## 3. Lyra 反而领先 / 独有的（不是全面落后）

- **A2A（agent-to-agent）**：这 5 家基本都没有（codex 的 multi-agent 是进程内编排，非跨运行时协议）。
- **fork + 文件 checkpoint + export/import 三件齐全**：多数竞品只占其中一两项（codex 无 fork/export；opencode 无 checkpoint；cline/claude_code/kimi 多为单向 export）。
- **多 provider × model 显式配对** + **per-round 成本核算** + **OTel 三件套**：运行时治理比多数 CLI 完整。
- **read-before-edit + stale 守卫**：与最 Claude 优化的 claude_code 的 `readFileState` 对齐（无 PTY、无 V4A，靠守卫式单 edit 保可靠）。

---

## 4. 已记录的刻意决策（非缺口）

- **不做 codex V4A apply_patch**：lyra 默认跑 Claude，而最 Claude 优化的 claude_code **故意不用** apply_patch、专门强化守卫式单 edit。lyra 已对齐其核心（unique-or-fail + replace_all + read-before + stale）。若未来要多文件原子，再单议。
- **后台命令无 PTY**：仅 codex 用 PTY（平台重）；claude_code / opencode 都不用。lyra 选无 PTY + 纯进程 kill（不引入 setpgid / taskkill 等平台特性），代价是深层孙进程可能残留——与现有同步 bash 同档。
- **Sandbox 暂缓**：无可移植抽象（每 OS 一套原语）；审批模型兜底。

---

## 5. 其他几家补充（桌面无源码）

- **aider**：`repo-map`（tree-sitter 抽全仓符号图喂模型）+ git-native commit → 印证缺口 §2.7。
- **cursor / windsurf**：embeddings 全仓索引 + 大型 "apply model"（快速套改）+ 深 IDE → 语义检索 + IDE 集成（后者属前端）。
- **amp（Sourcegraph）**：子 agent + "oracle"（强模型做架构判断）+ 代码搜索 → 印证缺口 §2.5。
- **gemini-cli**：超长上下文 + MCP + 内置 web → 无新维度。
- **continue**：IDE 自动补全（非 agent 范畴）。

---

## 6. 结论与建议顺序

跨家最一致、lyra 又缺的运行时能力：**多模态、hooks、todo 工具、MCP OAuth、语义检索（repo-map）**。

按"平台无关 + 价值/成本最优"建议落地顺序：

1. **多模态图片输入**（唯一 5/5 全有而 lyra 没有；缺口最显眼）
2. **todo 工具**（成本极低、顺手）
3. **Hooks**（扩展性核心）
4. **MCP OAuth**（seam 已备）
5. **多 agent swarm** / **repo-map 接 rag**（进阶）

> 维护提示：本文是**时点快照**。竞品演进快，落地新能力后请回来勾掉对应缺口、更新矩阵。
