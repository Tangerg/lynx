# Lyra — 架构设计

> **基线**：lynx HEAD `c68784d`，2026-05-21。Lyra 不是 lynx 的一部分 —— 它是**基于 lynx framework 构建的产品**。

---

## 1. 产品定档

**Lyra is a general-purpose AI coding agent — opinionated, product-grade, built on the lynx-agent framework.**

中文一句话：**用 lynx 这套 framework 搭起来的通用编码 agent 产品，对标 Claude Code / Codex / pi-coding-agent**。

### 1.1 Lyra 是什么

| 维度 | 说明 |
|---|---|
| 形态 | CLI 应用 + TUI 交互（后续可加 Web / IDE 桥接） |
| 目标用户 | 工程师在终端里用的"AI pair programmer" |
| 核心能力 | 读写编辑代码、执行命令、搜索文件、调用工具、长对话记忆、子任务委派 |
| 模型 | 不绑定 —— 走 lynx-core 的 chat.Client，44+ provider 全部可用 |
| 沙箱 | macOS Seatbelt / Linux bwrap / Windows Sandbox 三后端，按平台选 |

### 1.2 Lyra 不是什么

- ❌ 不是 framework — framework 已经是 lynx-agent。Lyra 是**用** framework 而不是再造 framework
- ❌ 不是 SDK — 用户不 `import "github.com/Tangerg/lynx/lyra"`；用户直接跑 `lyra` 二进制
- ❌ 不是 fork — 不复制 lynx-agent 的能力，直接依赖
- ❌ 不是 LLM provider wrapper — 那是 lynx-core 的职责
- ❌ 不做 plugin marketplace、不做云端 server、不做团队协作

---

## 2. 产品级 vs 框架级 的具体区别

这个区分是 Lyra 设计的核心。lynx-agent 是框架（用户写 5 行配 Action），Lyra 是产品（用户什么都不写，下载就用）。

| 维度 | lynx-agent（framework） | Lyra（product） |
|---|---|---|
| **System prompt** | 用户在 Action 里自己写 | 烤进二进制的 system prompt，按场景切换（coding / debugging / refactor / explain） |
| **Tool 选择** | 用户在 Action 里指定 ToolGroups | 默认开箱即用一整套 coding tool 链 |
| **Agent definition** | 用户用 `agent.New(...).Build()` 写 | Lyra 启动时自动构造 internal agent definition |
| **Planner** | 用户选 `PlannerName` | 默认 `reactive`，长任务自动切 `goap`（按 step count heuristic） |
| **Permission** | 用户配 GoalApprover extension | 三档预设（safe / balanced / yolo）+ tool-level cache + LLM classifier |
| **Context 管理** | 用户自己管 blackboard | 自动 compact（token-ratio threshold）+ memory 自动提取写回 `LYRA.md` |
| **UX** | 没有 — 调用者自己处理 IO | TUI（bubbletea）+ streaming + interrupt + steering 队列 |
| **配置** | `core.PlatformConfig` struct | `~/.lyra/config.toml` + `LYRA.md` 级联加载 |
| **错误处理** | 返回 error，调用者处理 | 友好提示、retry 建议、recovery 操作面板 |
| **观测** | OTel span（用户自接 exporter） | 自带本地 trace viewer (`lyra trace`) |
| **测试** | unit / integration / fixture | + e2e（真跑 LLM）+ snapshot（agent loop 行为对比）+ 性能基准（first-token latency） |

> **底线**：lynx-agent 给"能做" 的能力，Lyra 给"开箱即用"的体验。

---

## 3. 模块拆分

```
lyra/                         # 独立 Go module
├── cmd/lyra/                 # CLI 入口
│   └── main.go               # cobra root command
├── internal/
│   ├── agent/                # Agent definition factory
│   │   ├── system_prompt.go  # 内置 system prompt（按 mode）
│   │   ├── definition.go     # 构造 *core.Agent 的工厂
│   │   └── mode.go           # coding / debug / refactor / explain
│   ├── tools/                # 产品自带的 tool 包
│   │   ├── bash/             # Bash + 沙箱
│   │   ├── fs/               # Read / Write / Edit
│   │   ├── grep/             # Grep（基于 ripgrep）
│   │   ├── glob/             # Glob
│   │   ├── webfetch/         # WebFetch
│   │   └── task/             # Task delegation（复用 lynx AsChatTool）
│   ├── sandbox/              # Seatbelt / bwrap / win sandbox 三后端
│   ├── instructions/         # LYRA.md / AGENT.md 级联加载
│   ├── compaction/           # 上下文自动压缩
│   ├── approval/             # tool-level approval 缓存 + LLM classifier
│   ├── memory/               # 长期记忆（用户级 + 项目级）
│   ├── session/              # Session 树 + branching + JSONL 持久化
│   ├── tui/                  # bubbletea UI（可选启动）
│   ├── stream/               # Event-stream agent loop
│   ├── config/               # ~/.lyra/config.toml 加载
│   └── trace/                # 本地 OTel collector + viewer
└── doc/
    ├── ARCHITECTURE.md       # 本文档
    ├── ROADMAP.md            # Milestone 路线图
    └── ...
```

### 3.1 跟 lynx 的依赖关系

```
┌─────────────────────────────────────────────┐
│  Lyra (this module)                          │
│  - 不引入任何新的 framework 抽象              │
│  - 完全消费 lynx-agent + lynx-core            │
└──────────────────┬───────────────────────────┘
                   ↓ depends on
┌─────────────────────────────────────────────┐
│  lynx-agent                                  │
│  Platform / Action / Goal / Planner /        │
│  Workflow / Extension / HITL / Snapshot      │
└──────────────────┬───────────────────────────┘
                   ↓ depends on
┌─────────────────────────────────────────────┐
│  lynx-core                                   │
│  Chat / Tool / Vector / RAG / MCP / OTel     │
└─────────────────────────────────────────────┘
```

**强约束**：Lyra 不允许向 lynx 反向贡献抽象。如果 Lyra 发现需要某个能力而 lynx 没有 —— 要么 lynx 提交 PR（走 lynx 的发布流程），要么 Lyra 自己内部实现（在 `internal/` 下，不暴露）。

### 3.2 internal/ vs 顶层包

**全部业务代码进 `internal/`**。Lyra 不对外暴露 Go API —— 它是一个**应用**，不是一个 library。这点跟 lynx-agent 完全相反。

唯一对外的"接口"是：
- CLI 命令（`lyra chat`, `lyra exec`, `lyra trace` 等）
- 配置文件格式（`~/.lyra/config.toml` + `LYRA.md`）
- Tool 协议（MCP，让 Lyra 既能连外部 MCP server，也能被当成 MCP server 被调用）

---

## 4. 核心架构决策

### 4.1 Agent loop 形态：event-stream（参考 pi-mono）

```go
// 推荐形态（伪代码）
events, err := lyra.RunInteractive(ctx, userMessage)
for event := range events {
    switch e := event.(type) {
    case TurnStart:     ui.RenderTurnHeader()
    case MessageDelta:  ui.AppendText(e.Text)
    case ToolCallStart: ui.ShowSpinner(e.Tool)
    case ToolCallEnd:   ui.HideSpinner()
    case TurnEnd:       ui.RenderTurnFooter()
    }
}
```

底层走 `lynx-agent.Platform.RunAgent`，但在外面包一层 event channel。

### 4.2 Tool 系统：lynx-agent + 沙箱

每个 Lyra 内置 tool 实现 `core.AgentTool`（lynx 已有接口），但额外携带：
- **safety metadata**：`Safe` / `RequiresApproval` / `Dangerous`
- **sandbox profile**：跑这个 tool 时需要的文件系统 / 网络权限
- **idempotency hint**：是否可重试 / 是否可缓存

Lyra 启动时把这些 tool 注册成一个 `ToolGroup`（lynx 概念），agent definition 自动 require 这个 group。

### 4.3 Planner 选择：默认 reactive，长任务切 goap

- **短任务**（≤5 步）：reactive planner（lynx 已有）
- **长任务**（>5 步 / 用户显式 plan mode）：goap A*
- **明确分解的任务**（用户 `/plan` 命令）：HTN

Lyra 启动时把 5 个 planner 全部注册成 extension，按 mode 切。

### 4.4 Permission：三档预设 + cache + classifier

```
模式           Bash         FS-Write     Network       MCP-tool
─────          ────         ────────     ───────       ─────────
safe           ASK          ASK          DENY          ASK
balanced       ASK*         ALLOW        ASK           ASK*
yolo           ALLOW        ALLOW        ALLOW         ALLOW

* = 带 LLM classifier 智能识别（rm -rf 永远 ASK）
```

ApprovalCache 按 `(tool, pattern)` 记忆决策，下次同模式直接走（参考 Codex）。

### 4.5 上下文压缩：token-ratio auto-compact + 写回 memory

- 当 prompt token 超过 model context 的 **60%**：触发 auto-compact
- 压缩策略：保留最近 N 条 + 历史摘要 + LYRA.md（永远保留）
- **关键**：压缩时同步把"学到的东西"提取到 `LYRA.md`（参考 Claude Code 的 memory 提取），下次启动自动加载

### 4.6 Session：树 + branching + JSONL（参考 pi-mono）

- 不复用 lynx 的线性 ProcessStore —— Session 是**树**
- 每条 message 有 id + parentId，可以 `/fork` 任意分支
- 持久化：JSONL 单文件（按 session id 一个文件）
- 位置：`~/.lyra/sessions/<id>.jsonl`

### 4.7 UI：TUI 默认开，可降级到 plain text

- 默认走 bubbletea TUI（差分渲染）
- `--no-tui` / 管道环境 → 纯文本流式输出
- `lyra trace --session <id>` → 离线查看 trace timeline

---

## 5. 关键的"不做"

| 不做 | 理由 |
|---|---|
| ❌ 自建 LLM client | lynx-core 已有 44+ provider |
| ❌ 自建 vector store | lynx-core 已有 27 个 |
| ❌ 自建 MCP 协议 | lynx-mcp 已有 client + server |
| ❌ 自建 planner | lynx-agent 已有 5 个 |
| ❌ 自建 workflow primitive | lynx-agent 已有 7 个 pattern |
| ❌ Plugin marketplace | YAGNI — extension 直接用 Go import |
| ❌ Web UI | YAGNI — 先把 TUI 做透 |
| ❌ IDE extension（VSCode / JetBrains） | YAGNI — MVP 后看用户需求 |
| ❌ 团队协作 / 云同步 | 不是 MVP 范围 |
| ❌ 自己做 LSP | 走 MCP，让用户接 lsp-mcp-server |
| ❌ Skill marketplace | YAGNI |
| ❌ 多语言 i18n | MVP 只英文（中文可后续加） |

---

## 6. 风险与权衡

### 6.1 lynx 边界变化的风险

Lyra 强依赖 lynx-agent / lynx-core API。lynx 在 PR 阶段还会演化 → 短期内 Lyra 的 import 可能跟 lynx HEAD 同步漂。

**应对**：
- Lyra 用 `replace github.com/Tangerg/lynx/agent => ../agent`（workspace 模式开发）
- 发布 v0.1 时 pin lynx version

### 6.2 沙箱跨平台一致性

macOS Seatbelt vs Linux bwrap 接口差异大。

**应对**：
- 抽象 `sandbox.Profile` 接口
- 平台特定代码用 `//go:build darwin/linux/windows` tag 分离
- Windows 第一版可以走"无沙箱"（用户自己负责）

### 6.3 LLM 行为不确定性

Agent loop 在不同模型上行为差异大（Claude 跟 GPT-4 不一样），系统 prompt 要适配。

**应对**：
- system prompt 按 model family 分文件
- e2e 测试 matrix 跑主流模型
- 用户可在 config 里 override

### 6.4 跟 lynx-agent 的"产品对框架"耦合

Lyra 用 lynx-agent 时可能发现"框架太抽象"或"框架缺东西"。

**应对**：
- 缺的能力先在 `lyra/internal/` 自己实现
- 等沉淀到 3+ 用例再提议进 lynx-agent
- 不轻易给 lynx 加 lyra-specific API（污染框架中立性）

---

## 7. 成功标准

Lyra v0.1（MVP）的成功定义：

1. 在一台干净的 Mac 上 `go install ./cmd/lyra` 后，`lyra chat` 能直接跑
2. 不写一行配置就能完成：读文件、写文件、跑测试、git 操作、Web 搜索
3. 长对话（>50 轮）不爆 context
4. 中断恢复：Ctrl+C 后下次启动可恢复
5. 跑 `lyra trace` 能看到完整 agent loop 时间线
6. 比 Claude Code 启动快 5x（Go binary 优势 vs Node.js）

---

## 8. 跟参考项目的关系

| 项目 | Lyra 借鉴 | Lyra 不借鉴 |
|---|---|---|
| **pi-mono** | event-driven loop / session 树 / steering 队列 / AGENTS.md 级联 | extension-first（Lyra 是 product，opinionated） |
| **Claude Code** | tool 集（Bash/Read/Write/Edit/Grep/Glob/WebFetch/Task）/ 自动 compact / memory 提取 / permission 三档 | Coordinator Mode（耦合太深）/ Snip 多层压缩（过度设计） |
| **Codex** | 沙箱多后端 / approval cache / two-stage compaction / Responses API 一等公民 | Rust 重写（无意义，Go 够用） |

**Lyra 的独门**（lynx 给的）：
- 5 个 planner 可切（reactive / goap / htn / utility / hybridUtility）
- HTN 任务分解（pi-mono / Claude Code / Codex 都没有）
- 类型安全 Action（Go 泛型，编译期检查）
- OTel 原生（其他三家都不是 native）
- Workflow primitive（scatter-gather / consensus / repeat-until-acceptable 等）

---

*文档版本：v0.1（初稿）。任何决策修改在此追加 changelog。*
