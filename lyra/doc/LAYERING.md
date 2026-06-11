# Lyra — 分层架构与单向依赖（重构计划）

> **日期**：2026-06-11。**状态**：目标架构 + 迁移计划（尚未执行）。
> **目标**：把 lyra 内部收敛成**严格单向依赖**的分层 —— 高层依赖低层，**禁止任何反向依赖**；同领域逻辑收敛进各自的 service；技术设施沉到 infra。
> **与既有原则的关系**：这是 CLAUDE.md「高内聚低耦合 / DIP」+ DESIGN_PHILOSOPHY §5.1「充血、但不叠 DDD 仪式层」的**落地形态**，不是引入 DDD tactical patterns（repository / aggregate / app-service / event-bus 一律不加 —— 见 §6）。

---

## 0. 本质 —— agent 就是一个 loop

一个 agent 的核心本质是**一个 for 循环**:`组装上下文 → 调 LLM → 执行返回的工具 → 把结果喂回 → 重复,直到没有更多工具调用（done）`。所有事情都发生在这个循环里。三家各有其名:

- **Claude Code** —— **query loop**（`src/query.ts` 的 `async function* query()`）。
- **codex** —— turn loop（core 里的 `loop {}`）。
- **opencode** —— `SessionRunCoordinator`（drain chain）。

**lyra 的不同（且更干净）**:循环**不在** lyra 自己,而在 lynx `agent` SDK —— **`agent/runtime/run.go` 的 `for {}`**。lyra **没有手写循环**,它复用框架原语。所以 **`engine` 的本质 = "给这个循环装配输入（system prompt / 工具集 / model client / 各 service）+ 驱动它跑一个 turn"** —— 一个薄驱动,不是循环本身。

这把分层**钉死**:**循环是脊梁,其余一切都是"循环往下调的东西"** ——

```
            ┌───────────────────────────────────────────┐
delivery →  │  engine = 装配 & 驱动 agent loop（跑一个 turn）│
            └───────────────────────────────────────────┘
                 │ 每次迭代 / turn 边界往下调
                 ▼
            service（工具能力 / 记忆 / 持久化 / 维护：压缩·提取·规划）
                 ▼
            infra（storage / git / lsp / checkpoint / exec）
                 ▼
            agent/runtime `for {}`（lynx SDK 提供的循环原语，engine 驱动它）
```

循环往一个方向调 → 依赖天然单向。下面 §1 是它的层次化表述。

## 1. 目标分层（自上而下，依赖只能向下）

```
delivery   rpc/*                     协议 / 传输 / dispatch（wire 形态）
   ↓
engine     internal/engine           编排:跑一个 turn = 组合 service + 驱动 agent SDK + 装配工具集
   ↓
service    internal/service/*        领域逻辑 + 状态:每个 service 收敛一个领域,只依赖 infra
   ↓
infra      internal/infra/*          技术设施:storage / git / lsp / checkpoint / exec —— 不依赖任何上层
```

- **composition root**：`internal/runtime` + `cmd/lyra` 在最外层把各层实现拼起来注入（它可以 import 所有层，是装配处，不算业务依赖）。
- **唯一方向**：delivery → engine → service → infra。**任意反向边 = 回归**。

### 各层职责

- **infra**：和外部世界打交道的纯技术适配器，**零领域知识**。sqlite / 文件、git 二进制、LSP client、影子 git（checkpoint 机制）、进程执行。它实现 service 定义的端口（或被 service 直接持有，见 §4）。
- **service**：一个领域一个 service，把该领域**所有相关逻辑收敛在一起**（实体不变量 / 派生 / 校验 / 编排该领域的 infra）。只向下依赖 infra。
- **engine**：**编排层**。"跑一个 chat turn"= 按序调用 service + 驱动 lynx `agent` 框架 + 把各 service 暴露的能力装配成工具集。**不放领域算法**（压缩 / 提取 / 规划等下沉到 service），engine 只负责"何时调用谁、怎么串起来"。
- **delivery**：`rpc/*`，wire 契约 + 传输。只依赖 engine（跑 turn）+ service（CRUD 类直读）。

---

## 2. 现状依赖图（实测，2026-06-11）

```
chat(service,1750L) ─→ engine ─→ memory(port)
tool(service)       ─→ engine ─→ agentdoc
                              └─→ lsp(infra)            ← engine 直碰 infra
rpc/server ─→ engine, 全部 service, checkpoint(infra), git(infra), sqlite(infra)   ← delivery 直碰 infra
内部各 port(session/memory/history/interrupts/provider) 无 internal 依赖 = 纯端口,实现在 storage
```

**问题（= 重构要消除的）**：

1. **engine 卡 DAG 中段**：`chat`/`tool`（编排）依赖 engine，engine 又依赖 `memory`/`agentdoc`（领域）。所以 `service/` 不是一层 —— 被 engine 劈成"编排（chat/tool，在 engine 上）"和"领域（其余，在 engine 下）"两半。**这是核心结点。**
2. **engine 直碰 infra**：`engine → internal/lsp`。应经 service（codeintel）。
3. **delivery 直碰 infra**：`rpc/server → checkpoint / git / sqlite`。应经 service。
4. **领域算法滞留 engine**：`compaction / extractor / planner`（~426L）是领域操作,却是 engine 的私有 struct。

> 注:不存在 import 循环（Go 禁止）；是 DAG 的中段纠缠。`chat→engine` 是为拿契约类型（`ChatProcess`/`RunChatRequest`/`ChatOutput`/`InterruptResolution`/`ToolApprovalVerdict`/`CompactionResult`…）+ 5 方法窄接口。

---

## 3. 模块目录（每个模块：能力 / 向下依赖 / 现状来源）

> 切分原则:**按"外部系统 / 领域"切,不按"函数"切**(infra 一个外部系统一包;service 一个领域一包;不把 git 拆成 diff/status)。内聚优先,不原子化。

### infra/ — 技术设施（零领域，不依赖任何上层）

| 模块 | 能力 | 现状来源 |
|---|---|---|
| `infra/storage` | 单一 SQLite（session / items+runs / interrupt / provider / message）+ file（LYRA.md）。**实现 service 层端口**。 | `internal/storage/*` |
| `infra/git` | git 二进制:`diff` / `status` / `available`。 | `internal/git` |
| `infra/lsp` | language-server client:spawn + JSON-RPC + diagnostics 缓存。 | `internal/lsp` |
| `infra/checkpoint` | 影子 git 工作区快照 / 还原。 | `internal/checkpoint` |
| `infra/exec` | 进程执行:前台（bash）+ 后台（ring buffer + kill）。 | 抽 `engine/bgshell.go` 机制（+ `tools/bash` 复用） |
| `infra/mcp` | MCP client:dial / 5 态生命周期 / 工具热插。 | 抽 `engine/mcp.go` 连接管理（SDK mcp 之上） |
| `infra/a2a` | A2A client:dial 远程 agent。 | 抽 `engine/a2a.go` 的 dial（SDK a2a 之上） |

### service/ — 领域（一域一包，只向下依赖 infra）

按 **loop 每一步要调的东西** 归类:

**① 上下文装配所需**
| 模块 | 能力 | 依赖↓ | 现状 |
|---|---|---|---|
| `service/knowledge`（现 `memory`,建议改名,见 §3.1） | LYRA.md 长期知识:get / update / list（project + user scope）。 | infra/storage(file) | `internal/service/memory` |
| `service/agentdoc` | AGENTS.md 级联发现 + render。 | — | `internal/service/agentdoc` |
| `service/skills` | skill 发现 / 取用（project + global 合并）。 | SDK `skills` | 散在 `engine/skills.go` |

**② 会话与历史**
| 模块 | 能力 | 依赖↓ | 现状 |
|---|---|---|---|
| `service/session` | 会话实体 + 生命周期:create / fork / subtask / rollback / restore / relocate（充血实体:Fork/NewSubtask 派生）。 | infra/storage | `internal/service/session` |
| `service/transcript`（现 `history`,建议改名,见 §3.1） | 持久 items + runs（UI 时间线）。 | infra/storage | `internal/service/history` |
| `service/conversation`（**新显形**,见 §3.1） | LLM 上下文消息历史 chat.Message[]:read / seed / truncate / count。 | infra/storage | SDK chat-memory store + engine 的 ReadHistory/SeedHistory 透传 |

**③ 工具能力（loop 执行步调用）**
| 模块 | 能力 | 依赖↓ | 现状 |
|---|---|---|---|
| `service/codeintel` | 代码智能:definition / references / hover / symbols / diagnostics。 | infra/lsp | `engine/lsptools.go` 能力侧 |
| `service/workspace` | VCS 视图（diff / status）+ 文件 checkpoint（snapshot / restore）。 | infra/git, infra/checkpoint | `rpc/server/{workspace_vcs,checkpoint,rollback}.go` |
| `service/tool` | 工具注册表 / 元数据 + 直调（tools.list / invoke）。 | — | `internal/service/tool` |

> fs / bash / web / 后台命令工具的"能力"够薄 —— 直接是 infra（storage / exec / http）+ engine 装配,**不单列 service**（YAGNI）。

**④ 策略 / 横切**
| 模块 | 能力 | 依赖↓ | 现状 |
|---|---|---|---|
| `service/approval` | 审批 stance（readonly/safe/balanced/yolo）+ HITL gate 判定。 | — | `internal/service/approval` |
| `service/interrupts` | open-interrupt 注册表（park/resume 查它）。 | infra/storage | `internal/service/interrupts` |
| `service/provider` | provider 注册表（key / baseURL）+ model→client 解析。 | infra/storage | `internal/service/provider` |

**⑤ 维护（turn 边界的自治操作）**
| 模块 | 能力 | 依赖↓ | 现状 |
|---|---|---|---|
| `service/compaction` | 历史摘要压缩（超阈值时折叠）。 | conversation, SDK chat client | `engine/compaction.go` |
| `service/extraction` | LYRA.md 事实提取（写回 knowledge）。 | knowledge, conversation, SDK chat client | `engine/extractor.go` |
| `service/planning` | plan 生成（plan 模式的策略产出）。 | SDK chat client | `engine/planner.go` |

> ⑤ 三者可保持三个单一职责小包,或合为 `service/maintenance` 一包三文件 —— 看后续是否共享状态决定;当前倾向**三个独立小包**(各自能脱离 engine 单测)。

### engine/ — 编排（装配 + 驱动 loop，§0）

不是模块清单而是**职责清单**(它 import 上述 service + agent SDK):

- 装配 **system prompt**（组合 knowledge + agentdoc + skills）。
- 装配 **工具集**（从 codeintel / workspace / tool / exec / mcp / a2a / skills + fs/bash/web）。
- 解析 **model client**（provider）。
- **驱动 `agent/runtime` 的 `for{}`** 跑一个 turn（`StartChat` / `ResumeChat`）。
- **turn 生命周期**:start / resume / cancel / steering / events（批次 5 吸收自 `chat.Service`）。
- **HITL** park / resume 编排（approval + interrupts）。
- **turn 边界**触发 compaction / extraction + workspace 快照。
- **会话操作**编排:fork / rollback / export / import（调 session + transcript + conversation + workspace）。

### delivery/ — rpc（只经 engine + service，不直碰 infra）

`rpc/protocol`（wire 契约）/ `rpc/server`（handler）/ `rpc/dispatch`（路由）/ `rpc/transport/*`（HTTP+SSE / inprocess）。

### composition root

`internal/runtime` + `cmd/lyra`:装配处,可 import 所有层,把各层实现注入。**不算业务依赖**。

---

## 3.1 命名消歧（架构清晰度的硬伤，建议一并修）

现在 **"memory" 和 "history" 各指两样东西**,是命名漂移 —— lyra 实则有**三种**不同的"历史/记忆",必须各有其名:

| 概念 | 是什么 | 现状叫法（混淆点） | 建议名 |
|---|---|---|---|
| **knowledge** | LYRA.md 长期知识（用户可编辑、跨会话） | `service/memory` | `service/knowledge` |
| **conversation** | 喂给 LLM 的消息上下文 `chat.Message[]` | SDK `memory.Store`（与上面撞名）+ 散在 engine | `service/conversation` |
| **transcript** | UI 渲染的 items + runs 时间线 | `service/history` | `service/transcript` |

> 三者职责、生命周期、消费方都不同(knowledge 用户编辑 / conversation 喂模型 / transcript 给 UI),却被 "memory×2 + history" 混指。消歧是低风险高收益的清晰度提升(纯改名 + import)。

---

## 3.2 演进纪律（避免过度切分 / 仪式）

- **内聚但不原子化**:一个外部系统 / 一个领域一包;别为"看起来分层"把单一职责再切碎。
- **单实现直接具体依赖**(你的原则,§4):端口只在"多实现 / 需测试桩 / 跨边界"处留(storage 端口、chat client 端口保留 —— 它们让 service 能 mock 测)。
- **不叠 DDD 仪式层**(§6):service 不是"application service 层",是"领域能力";infra 不是"repository 层",是"技术适配器"。
- **一次一刀**:模块目录是**目标**,不是一次到位;按 §5 批次逐步逼近,每刀全绿可独立 revert。

---

## 4. 依赖规则（怎么接）

1. **方向神圣**：只能向下。任何 `service → engine`、`infra → service`、`rpc → infra-直连` 都是回归。Go 无强制分层,靠 **review + 可选 import-lint（depguard）** 守。
2. **接口按需,不按仪式**（沿用 CLAUDE.md ISP「库 vs 应用」）：
   - **多实现 / 需测试桩 / 跨边界** → 在**消费方**定义窄接口（如现有 `memory.Store`、`chat.Engine`、storage 端口 —— 保留,它们让 service 能用 sqlite `:memory:` 测）。
   - **单实现 + 无测试桩需求** → **直接依赖具体类型即可**（你的原话；也是 §ISP「单实现→内联」）。不为单实现强抽窄接口。
   - 判据:这条依赖**需要可替换 / 可 mock 吗**?需要→接口;不需要→具体。
3. **端口归属消费方**：service 定义它要的 infra 端口,infra 实现;engine 定义它要的 service 端口（若需要),service 实现。**端口定义永远在上层（消费方）**,实现在下层。
4. **原子 SQL 留 infra**（§5.1）：单字段 UPDATE / 计数器自增不要上移到 service 实体,否则退化成 read-modify-write 竞态。

---

## 5. 迁移批次（安全→风险,每批独立可 revert,逐批确认）

> 节奏遵 REFACTORING.md §10:每批 `go build && vet && test ./...` 全绿、commit 写 why、推送。**批次 5 是大结构改动,动前单独签字。**

### 批次 1 — 领域算法服务化（最高价值/最低风险）
`engine` 的私有 `compactor`/`extractor`/`planner`（compaction.go 211 + extractor.go 143 + planner.go 72）→ `service/compaction` + `service/extraction` + `service/planning`（三个单一职责小包,§3⑤）。engine 经接口编排。
- **效果**:engine -426L;压缩/提取/规划可脱离 engine 单测;正中"重逻辑进 service"。
- **风险**:低。纯移动 + 注入;不动 wire、不动 infra。

### 批次 2 — infra 物理归集 + 确立层
`internal/{storage,git,lsp,checkpoint}` → `internal/infra/{storage,git,lsp,checkpoint}`（纯 import-path 改名）。同时确立"infra 不依赖任何上层"的红线。
- **效果**:层次在目录上可见;为批次 3/4 铺路。
- **风险**:低（机械改名 + 全模块构建）。

### 批次 3 — codeintel 服务（消除 engine→lsp-infra 直连）
`internal/service/codeintel` 包住 `infra/lsp`（LSP manager）。engine 的 `lsp_*` 工具改为**从 codeintel service 构造**;编辑后诊断也经 service。
- **效果**:engine 不再直碰 lsp infra;LSP 领域逻辑收敛。
- **风险**:中低。

### 批次 4 — workspace 服务（消除 rpc→infra 直连）+ 工具集装配收敛（A）
- `internal/service/workspace` 包住 `infra/git`（diff）+ `infra/checkpoint`（快照/还原）。`rpc/server` 改为调 workspace service,不再直 import checkpoint/git。
- 工具集装配（fs/bash/lsp/bgshell/web/skills/a2a,散在 lsptools/tools/bgshell/editguard/skills/askuser/a2a ~1400L）收敛成 engine 内聚的"装配"关注点,每个工具从其 service 取能力。
- **效果**:delivery 不再碰 infra;"agent 有哪些工具"单一归属。
- **风险**:中。

### 批次 5 — chat/tool 编排归位 engine 层（大结点,单独确认）（D）
消除 `chat→engine` / `tool→engine` 逆向边:把 `chat.Service` 的编排（turn 状态机 / lifecycle / dispatch / observer / policy）与 engine **并入同一编排层**;契约类型（`ChatProcess`/`RunChatRequest`/…）随之归位。结束后 engine 是唯一编排所有者,`service/` 只剩纯领域/数据,实现干净的 `engine → service → infra`。
- **效果**:彻底单向;消除中段纠缠。
- **风险**:高、面广（chat 1750L + 契约类型迁移 + rpc 接线）。**动前列 scope + 爆炸半径 + 备选,单独签字。**

### 批次 6（可独立、随时插入）— 命名消歧（§3.1）
`service/memory → service/knowledge`、`service/history → service/transcript`,并把 LLM 消息上下文显形为 `service/conversation`。纯改名 + import 调整,无行为变化。
- **效果**:消除 "memory×2 + history" 混指,三种历史/记忆各有其名。
- **风险**:低（机械,但 ripple 到 rpc/server + runtime + engine 的 import）。建议早做,清晰度收益立竿见影。

---

## 6. 明确不做（避免 DDD 仪式 / 过度抽象）

- ❌ 不引入 repository 层 —— 现有 `Store`/端口接口**就是** repository,改名 = 纯仪式。
- ❌ 不引入 application-service 层 —— `rpc/server` handler + engine 编排**就是**应用层,再加一层是重复。
- ❌ 不引入显式 aggregate root 框架 / domain-event bus —— 已有 run event hub;单后端单团队,叠这些是 YAGNI（DESIGN_PHILOSOPHY §5.1）。
- ❌ 不给纯端口 service（memory/history/interrupts/provider）"加肉" —— 它们薄是对的（端口）。
- ❌ infra 原子 SQL 不上移实体。
- ✅ 充血只做"把**无 I/O 的**领域规则收回实体/ service",不往上加层。

---

## 7. 完成态（self-check）

- delivery → engine → service → infra,`grep` 不出任何反向边。
- engine 不含领域算法（压缩/提取/规划在 maintenance service）。
- 无包直碰它**跨层**的 infra（rpc/engine 不直 import git/lsp/checkpoint/sqlite,经 service）。
- 每个 service 内聚一个领域,只向下依赖 infra。
- 端口只在"多实现 / 需测试桩 / 跨边界"处保留;单实现直接具体依赖。

> 维护提示:这是**目标 + 计划**文档。每完成一个批次,回来勾掉对应 §2 问题、更新 §3 映射的落地状态。

---

## 8. 执行状态（2026-06-12）

逐批落地记录。每批一个独立可 revert 的 commit,`go build && vet && test ./...` 全绿后推送。

- ✅ **批次 1 — 领域算法服务化**:`compactor`/`extractor`/`planner` → `internal/service/maintenance`(单包三文件 + 共享 `llm.go`,因三者共享 `askDirect`/`renderTranscript`,§3⑤ 允许的形态)。engine 经 `*maintenance.{Compactor,Extractor,Planner}` 编排。**消除 §2 问题 4**。
- ✅ **批次 2 — infra 物理归集**:`internal/{storage,git,lsp,checkpoint}` → `internal/infra/{...}`,层次在目录上可见。
- ✅ **批次 3 — codeintel 服务**:`internal/service/codeintel` 独占 `infra/lsp`,engine 的 `lsp_*` 工具 + 编辑后诊断经 service(`DiagnoseEdit` 闭包封住 baseline-diff)。**消除 §2 问题 2**(engine→lsp 直连)。
- ✅ **批次 4A — workspace 服务**:`internal/service/workspace` 独占 `infra/git` + `infra/checkpoint`(VCS 读为无状态包函数 + checkpoint 为有状态 `*Service`)。rpc/server + cmd/lyra 不再 import git/checkpoint。**消除 §2 问题 3**(delivery→infra 直连)。
- ✅ **批次 6 — 命名消歧(改名部分)**:`service/memory`→`service/knowledge`、`service/history`→`service/transcript`(含 sqlite `HistoryStore`→`TranscriptStore`、runtime SPI `History()`→`Transcript()`)。消除 "memory×2 + history" 混指;别名(lyramem/memsvc/...)全删。
- ⏳ **批次 4B — 工具集装配收敛**:评估后判定低价值、无 layering 收益(各工具构造已按类型内聚在各自文件,装配已集中在 engine.go + cwdToolResolver)——**暂缓**,避免投机性 churn(CLAUDE.md「承认 audit 过度 call 的项」)。
- ⏳ **批次 6 余下 — conversation 显形**:把 LLM 消息上下文(engine 的 `MemoryStore`/`ReadHistory`/`SeedHistory` 透传)显形为 `service/conversation`,并把 `engine.Config.MemoryStore`/`MemoryService` 字段改名。**暂缓**(纯改名 + 一个新 service 包,待 §2 问题 1 一并处理或单列)。
- 🚧 **批次 5 — chat/tool 编排归位 engine 层**:**§2 问题 1(核心结点)未消除**——`chat→engine` / `tool→engine` 逆向边仍在。这是计划里唯一**动前需单独签字**的大结构改动(chat 1750L + 契约类型迁移 + rpc 接线,爆炸半径大)。待用户确认 scope 后执行。

**当前依赖图**:§2 的问题 2/3/4 已消除,领域算法 + LSP + workspace 都已沉到 service/infra,delivery 不再碰 infra。**唯一剩余的反向边是 §2 问题 1**(chat/tool 编排在 engine 之上),由批次 5 处理。
