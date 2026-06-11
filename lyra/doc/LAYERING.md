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

## 3. 目标包映射

| 目标层 | 包 | 说明 |
|---|---|---|
| delivery | `rpc/protocol` `rpc/server` `rpc/dispatch` `rpc/transport/*` | 不变；改为只经 service 触达 infra |
| engine | `internal/engine` | 编排 + 工具集装配；**吸收** chat/tool 的编排（§5 批次 5）；领域算法下沉 |
| service | `internal/service/session` `memory` `approval` `history` `interrupts` `provider` `agentdoc` `tool` | 既有领域/端口 |
| service（新） | `internal/service/maintenance`（compaction+extraction+planning）`codeintel`（LSP 能力）`workspace`（git diff + checkpoint） | 由 engine / rpc 下沉而来 |
| infra | `internal/infra/storage`（原 storage）`infra/git` `infra/lsp` `infra/checkpoint` | 物理归集到 `internal/infra/*`,层次可见 |
| composition | `internal/runtime` `cmd/lyra` | 装配处,可 import 所有层 |

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
`engine` 的私有 `compactor`/`extractor`/`planner`（compaction.go 211 + extractor.go 143 + planner.go 72）→ `internal/service/maintenance`（或三个 service）。engine 经接口编排。
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
