# Lyra — 微内核架构(engine 作核 + 端口注入)

> **日期**:2026-06-12。**状态**:目标架构 + 边界定义(动手翻代码前先对齐)。
> **关系**:这是 [`LAYERING.md`](LAYERING.md) 的**演进**。LAYERING 把内部收敛成单向四层并消除了反向边(已完成)。本文件**修正其中一条**:`engine` 与 `service` **不是上下层关系,而是"核 + 能力"关系** —— engine 是微内核,service 是它通过端口消费的能力实现,`runtime` 是把两者绑起来的装配者。其余(infra 仍是最底技术设施、delivery 仍只经 runtime)不变。

---

## 0. 本质 —— engine 是微内核,不是"层"

我们在 LAYERING 阶段反复撞到同一种别扭:"chat 在 engine 之上还是之下?""tool 算 service 还是 engine?""toolset 抽出去会不会成环?"。这些纠结本身是信号:**engine 根本不是一个"层",它是一个核(kernel)**。

一个 agent 的核心是一个 loop(组装上下文 → 调 LLM → 跑工具 → 喂回 → 重复)。**engine = 驱动这个 loop 的微内核**:它提供 loop 机制本身,把它**需要的一切外部能力声明成端口(port 接口)**,自己不实现这些能力。能力由 service 实现,由 `runtime` 注入。

```
                         runtime  (composition root / 装配者)
                        /         \         \
            engine（微内核）   service（能力实现）   infra（技术设施）
            定义 port 接口  ◄────实现 port────┘         ▲
            驱动 loop                                   │ service 内部调 infra
            装配工具集 + 系统提示                         │
                  └─────── 注入(runtime 把 service 填进 engine 的 port)
```

**方向**:`runtime → {engine, service, infra}` 装配;运行期 engine 经 port 调 service,service 经具体调 infra。engine **不 import 任何具体 service**;它只定义 port,runtime 注入实现。

---

## 1. 与 LAYERING.md 的差异(就改这一条)

| | LAYERING(旧) | 微内核(新) |
|---|---|---|
| engine ↔ service | engine **在上**,向下 import 具体 service(codeintel/maintenance/…) | engine 是**核**,定义 port;service **实现** port;runtime **注入** |
| 依赖方向(engine 边界) | engine → service(具体) | engine 定义接口 ← service 实现(结构化满足,service 通常**无需 import engine**) |
| "chat 在哪" | 纠结(批次 5 搬到 engine/chat) | **非问题**:chat 是消费 engine 的 service,引用 engine 正常 |
| infra | service 之下 | 不变:service 内部用 infra 实现 port;infra 不直面 engine |
| delivery(rpc) | 经 runtime | 不变 |

**前 11 个批次不作废**:抽出的 codeintel / workspace / maintenance / conversation / skills / infra/* **正是要变成 port 实现的 adapter**。本次只翻转 engine 边界的依赖方向(加 port + 把构造挪到 runtime 注入)。

---

## 2. 核(kernel)= engine 保留什么

engine 只保留"**驱动 loop 本身**"的东西 —— 是机制,不是能力:

- **驱动 `agent/runtime` 的 `for{}`**:`StartChat` / `ResumeChat` / `RestoreChat` / cancel / steering(turn 生命周期状态机)。
- **装配工具集**:把各能力 port 包成 `chat.Tool`(schema + 参数解析 + 调 port + 格式化)并 dispatch;per-role(coding/subtask)、per-cwd 解析(`cwdToolResolver`,读 ctx)。
- **装配 system prompt 骨架**:basePrompt + 调 Knowledge/AgentDocs port 拼接。
- **HITL**:park-on-interrupt + resume 编排(`ChatProcess.Resume`、approval gate、ask_user 工具)。
- **turn 边界触发**:turn 结束时调 Compaction/Extraction port + workspace 快照。
- **契约类型**:`ChatProcess` / `RunChatRequest` / `RestoreChatRequest` / `ChatOutput` / `InterruptResolution` / `ToolApprovalVerdict` / `QuestionPrompt` / `TokenUsage` / `ModelUsage`(engine 的 I/O 形态 + HITL 契约)。
- **observer**:turn 事件流 + 工具审批 gate(`core.Extension`/`ToolDecorator`)。
- **port 接口定义**:见 §3。

> 判据:**"这是 loop 怎么跑"还是"loop 跑的时候要用的某个能力"?** 前者留 engine,后者抽 port。

---

## 3. 端口(ports)—— engine 定义、service 实现、runtime 注入

每个 port 是 **engine 在消费方定义的窄接口**(ISP);service **结构化满足**它(Go 隐式实现,service 通常无需 import engine)。runtime 构造 service 实现并注入 `engine.Config`。

| port(engine 定义) | 方法(窄) | 实现(现有 adapter) | engine 用它做什么 |
|---|---|---|---|
| `ChatClient` | `chat.Client`(已是注入) | provider 解析(runtime `clientResolver`) | 调 LLM(已是 port,经 `core.ChatClientProvider` per-turn 换) |
| `CodeIntelligence` | Definition/References/Hover/DocumentSymbols/WorkspaceSymbols/Diagnostics/DiagnoseEdit/Supported | `service/codeintel` | 建 `lsp_*` 工具 + 编辑后诊断 |
| `Background` | Launch/Get/Kill/KillAll(或 Shell 读写) | `infra/exec`(或薄 service 包) | 建后台命令工具 |
| `ToolSources`(MCP) | Tools/Statuses/Reconnect/Close/SetToolSink | `infra/mcp` | MCP 工具 + workspace.mcp.* 视图 |
| `RemoteAgents`(A2A) | dial 出的 `[]chat.Tool` + Close | `infra/a2a` | A2A 委派工具 |
| `Knowledge` | Get/Update/List(scope,dir) | `service/knowledge` | 系统提示注入 + 提取写回 |
| `AgentDocs` | Discover/Render | `service/agentdoc` | 系统提示 AGENTS.md 级联 |
| `Conversation` | Read/Seed/Count/Truncate/InjectUser | `service/conversation` | fork/rollback/steering/messages.list |
| `Compactor` | MaybeCompact | `service/maintenance` | turn 边界压缩 |
| `Extractor` | MaybeExtract | `service/maintenance` | turn 边界提取 |
| `Planner` | Plan | `service/maintenance` | plan 模式产出 |
| `Skills` | List/MergedSource | `service/skills` | skill 工具 + workspace.listSkills |

> 注:`workspace`(VCS+checkpoint)、`session`、`transcript`、`approval`、`interrupts`、`provider`、`tool`(registry) 当前由 **rpc/runtime 直接消费**,不经 engine,所以**不是 engine 的 port** —— 它们是 runtime 层装配/消费的 service,保持原样。engine 的 port 只列 engine 自己 loop 里要用的。

**port 定义放哪**:`internal/engine`(消费方)。可单开 `internal/engine/port` 子包放接口集中管理,或就近放在用它的 engine 文件里。倾向**集中一个 `port.go`**(便于一眼看清核需要什么)。

---

## 4. 装配 —— runtime 绑定(代码形态)

```go
// runtime.New：构造能力实现(adapter)→ 注入 engine 的 port
func New(ctx, cfg Config) (*Runtime, error) {
    // 1. 构造 infra / service 能力实现(adapter)
    codeIntel := codeintel.New(cfg.LSPServers)          // 实现 engine.CodeIntelligence
    bg        := exec.NewManager()                       // 实现 engine.Background
    mcpConns, _ := mcp.Dial(ctx, cfg.MCPServers)         // 实现 engine.ToolSources
    knowledge := cfg.Knowledge                           // 实现 engine.Knowledge
    convo     := conversation.New(memStore)              // 实现 engine.Conversation
    compactor := maintenance.NewCompactor(memStore, cc)  // 实现 engine.Compactor
    // …

    // 2. 注入 engine（engine 只认 port 接口，不认这些具体类型）
    eng, err := engine.New(ctx, engine.Config{
        ChatClient:      cc,
        CodeIntelligence: codeIntel,   // 结构化满足 port，无需 codeintel import engine
        Background:       bg,
        ToolSources:      mcpConns,
        Knowledge:        knowledge,
        Conversation:     convo,
        Compactor:        compactor,
        Extractor:        extractor,
        Planner:          planner,
        Skills:           skillsSvc,
        // …
    })

    // 3. runtime 持 engine + 其余直接消费的 service（session/transcript/...）
}
```

要点:**engine.New 不再 `codeintel.New(...)` / `exec.NewManager()`** —— 构造能力是 runtime 的事;engine 只接收 port。能力的 **lifecycle/Close** 也归 runtime(它构造的它关)。

---

## 5. 三条硬规则(否则退化成反模式)

1. **DI,不用 Service Locator**。runtime **注入**窄 port 给 engine;**禁止** service 内部 `import engine` 再 `engine.SomeGlobalCapability()` 到处伸手(那是 Service Locator / god-object,与低耦合相反)。port 是显式、最小、单向的。
2. **ambient 环境走 `context.Context`,不进 engine"能力仓库"**。cwd / session id 是**本次 turn 的环境**,不是稳定能力 —— 继续挂在 process blackboard、经 `turnCwd(ctx)` 读(现状,正确)。**不要**做成"问 engine 要当前目录"。
3. **port 窄、由 engine(消费方)定义、service 结构化满足**。engine 要代码智能的 6 个方法就定义那 6 个,不照搬 codeintel 全表;service 不必 import engine(Go 隐式实现)。判据:**这条依赖需要可替换 / 可 mock 吗?** 需要→port;不需要→别抽。

---

## 6. 明确不做(避免 god-object / 接口泛滥)

- ❌ engine 不做"公共能力仓库"让大家 `import engine` 取用 —— 那是 Service Locator,见 §5.1。
- ❌ 不给纯数据类型 / engine 内部结构套 port —— port 只给**跨 engine 边界、要 mock 测**的能力(§5.3)。
- ❌ 不把 rpc/runtime 直接消费的 service(session/transcript/workspace/…)硬塞成 engine 的 port —— engine 不用它们,就不是它的 port。
- ❌ cwd 这类 ambient 不做成 port / 不做成 engine getter —— 走 ctx。
- ✅ port 只在 engine **真正消费**的能力上设,数量 ≈ §3 那张表。

---

## 7. 迁移批次(增量,每批全绿可 revert;复用前 11 批抽出的 adapter)

> 模式:为一个能力定义 engine port → engine 改持 port(不再具体构造)→ 构造挪到 runtime 注入 → 全绿提交。先挑已经最像 port 的开刀验证模式。

- **M0**:写本文件 + engine 加 `port.go` 骨架(空接口集),不改行为。
- **M1(验证模式)**:`CodeIntelligence` + `Background` 两个最干净的(纯能力、无契约耦合)。engine.Config 加 port 字段,New 改接收注入,runtime 构造并注入。跑通模式。
- **M2**:`Knowledge` / `AgentDocs` / `Conversation`(系统提示 + 会话上下文 port 化)。
- **M3**:`Compactor` / `Extractor` / `Planner`(maintenance 三 port)。
- **M4**:`ToolSources`(MCP)/ `RemoteAgents`(A2A)/ `Skills`(工具源 port 化)。
- **M5**:engine.New 收尾 —— 确认 engine **零具体 service/infra import**,只剩 port + agent SDK + tools/* 装配模块 + 契约类型。chat 在此模型下作为消费 engine 的 service 自然归位(可从 engine/chat 移回 service/chat,因为"service 引用 engine"在微内核里正常)。

---

## 8. 完成态(self-check)

- `internal/engine` 生产代码 **不 import 任何 `internal/service/*` 或 `internal/infra/*` 具体包** —— 只 import 自己的 port 接口 + agent SDK + `tools/*` 装配模块 + `core/model/chat`。
- 每个 port 是 engine 定义的窄接口;service 结构化满足,**多数 service 不 import engine**。
- runtime 是唯一构造能力实现并注入 engine 的地方;能力 lifecycle 归 runtime。
- 没有 service `import engine` 取"公共能力"(无 Service Locator);ambient 环境走 ctx。
- engine 可用 stub port 孤立单测。

> 维护:每完成一个 M 批次,回来勾掉 §7 + 更新 §3 落地状态。本文件与 LAYERING.md §1「engine↔service 上下关系」冲突处,**以本文件为准**。
