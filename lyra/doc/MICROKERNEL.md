# Lyra — 微内核架构(engine 作核 + 端口注入)

> **⚠️ 目录已重命名（2026-06-14，见 [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md) §9）**：`internal/engine→internal/kernel`（"engine 作核" 即此 kernel）/ `internal/service→internal/domain` / `rpc→internal/delivery` / `engine/chat→kernel/turn`。本文行文中的旧路径名指代重命名后的同一目录，未逐处回改。

> **日期**:2026-06-12。**状态**:**已采纳,执行中**(M 批次见 §7)。
> **关系**:这是 [`LAYERING.md`](LAYERING.md) 的**演进**。LAYERING 把内部收敛成单向四层并消除了反向边(已完成)。本文件**修正其中一条**:`engine` 与 `service` **不是上下层关系,而是"核 + 能力"关系** —— engine 是微内核,定义它直接消费的窄 port,能力实现由 `runtime` 注入。其余(infra 仍是最底技术设施、delivery 仍只经 runtime)不变。

## 业界实证(2026-06-12,读了五家源码)

采纳依据不是偏好,是 **convergent design** —— 主流 agent runtime **无一例外**走"核心 loop 依赖接口/端口 + 注入",没有一家是单体直连:

| 仓库 | 核心 loop | 能力获取 | 关键证据 |
|---|---|---|---|
| codex (Rust) | `run_turn()` | 微内核/Ports | 工具 `HashMap<Name,Arc<dyn CoreToolRuntime>>`;model `Arc<dyn ModelProvider>`;`ToolRouterParams` 注入 |
| claude_code (TS) | `queryLoop()` | Context 注入 + DI | `ToolUseContext` 串 tools/abort/fileState/权限;`QueryDeps{callModel,autocompact}` 注入 |
| cline (TS) | `AgentRuntime.execute()` | 微内核/DI | 工具 `Map<string,AgentTool>` 注册表;`AgentModel` 接口多 provider;`ToolExecutors` 注入 |
| opencode (TS) | `SessionRunner.run` | Effect Context/Layer | 一切 `yield* XxxService`;Layer 组装 |
| kimi-code (TS) | `runTurn(input)` | 微内核/Ports | 核心定义 `LLM`/`ExecutableTool`/`LoopHooks` 接口,Agent 注入 adapter |

**实证还修正了 port 边界(关键)**:五家核心 loop 的真正端口**很窄** —— 基本是 **model + 工具集 + 几个 hook**。而且**工具是在核心之外构造好、再注入进核心的**(claude `ToolUseContext.options.tools`、cline `addTools`、kimi `ToolManager.loopTools`、codex per-turn build `ToolRouter` 后传入)。深层能力(代码智能 / exec / 文件 / 记忆)**是"工具的依赖",不是核心的独立 port** —— 它们被工具包住,工具在装配层组好注入。

> 对 lyra 的含义:**不要给每个能力都设核心 port**(那是我初稿的 12-port 表,过细)。核心 port 收窄到 §3;codeintel/exec/mcp/a2a/skills 降为**工具装配层的注入依赖**;**工具构造移出核心 loop 文件**(= 把 engine 变薄的直接手段)。lyra 已经走在路上:`core.ChatClientProvider`(model port)、`cwdToolResolver` 是注入进 platform 的 `core.ToolGroupResolver`(工具集 port)、middleware 是 hook —— 都已具备。

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

## 3. 端口(ports)—— 按实证收窄

分两类(实证的关键区分):**(甲) 核心 loop 的真 port**(engine 直接调,窄,挣得起接口);**(乙) 工具装配层的注入依赖**(被工具包住,不是核心 port)。

### (甲) 核心 port —— engine 定义、runtime 注入

| port(engine 定义) | 方法 | 实现 | engine 用它做什么 | 现状 |
|---|---|---|---|---|
| **model** | `core.ChatClientProvider` + `*chat.Client` | runtime `clientResolver` | 调 LLM、per-turn 换 model | ✅ **已是 port** |
| **工具集** | `core.ToolGroupResolver`(per-role/per-cwd 解析) | `toolset.Resolver`(M1 抽出) | 提供该 turn 的 `[]Tool` | ✅ 已是注入进 platform 的 extension;M1 把构造移出核心 |
| **maintenance hooks** | `Compactor.MaybeCompact` / `Extractor.MaybeExtract` / `Planner.Plan`(三个独立窄接口) | `service/maintenance` | turn 边界压缩/提取 + plan 模式 | M2 端口化(可换策略 = 扩展点) |
| **prompt-context** | `Knowledge.Get` + `AgentDocs.Discover/Render` | `service/knowledge` / `service/agentdoc` | 拼 system prompt | M2 端口化(窄,只 engine 直接调的方法) |
| **conversation** | `Read/Seed/Count/Truncate/InjectUser` | `service/conversation` | fork/rollback/steering/messages.list facade | M2 端口化 |

### (乙) 工具装配层的注入依赖 —— **不是核心 port**

`codeintel` / `exec` / `mcp` / `a2a` / `skills` + `tools/{fs,bash,web}` —— 这些被**工具**包住(`lsp_*` / `bash`/后台命令 / MCP / A2A / skill 工具)。**工具构造在 `toolset` 装配层完成**(M1 移出核心),装配层直接持这些能力的具体类型;`runtime` 构造能力 → 喂给装配层 → 得到 resolver → 注入核心。**核心只见"工具集 resolver"这一个 port,见不到这些能力。**

> 为什么不给它们各设核心 port:它们多是单实现,且**核心根本不直接调它们**(经工具间接调)。给它们设核心 port = 我初稿的过细 12-port,违 ISP/YAGNI。装配层用具体类型即可(单实现直接依赖)。

### 不经 engine 的 service(保持原样)

`workspace` / `session` / `transcript` / `approval` / `interrupts` / `provider` / `tool`(registry) 由 **rpc/runtime 直接消费**,engine loop 里不用 → **不是 engine 的 port**。

**port 定义放哪**:`internal/engine`(消费方),集中一个 `port.go`,一眼看清核需要什么。能力实现**结构化满足**(Go 隐式),多数无需 import engine。

### 扩展性(你要求的"考虑拓展性")

- **加一个工具**:在 `toolset` 加一个 builder + 注册进 resolver 的某个源 —— 不动核心。
- **加一类工具源**(如新的 MCP-like 协议):给装配层加一个源切片 —— 不动核心。resolver 已是"多源聚合"形态,扩展只加源。
- **换 maintenance 策略 / model provider**:实现对应 port 注入 —— 不动核心(这就是 port 的价值兑现处)。
- **加一个 hook**(如新的 turn 边界自治操作):走 maintenance hook 模式或 middleware,不动 loop。

---

## 4. 装配 —— runtime 绑定(代码形态)

```go
// runtime.New：构造能力 → 装配工具集 → 注入核心的窄 port
func New(ctx, cfg Config) (*Runtime, error) {
    // 1. 构造能力(infra/service)—— 它们是工具装配层的依赖,不是核心 port
    codeIntel := codeintel.New(cfg.LSPServers)
    bg        := exec.NewManager()
    mcpConns, mcpTools, _ := mcp.Dial(ctx, cfg.MCPServers)
    a2aConns, a2aTools, _ := a2a.Dial(ctx, cfg.A2AAgents)

    // 2. 工具装配层在核心之外组好工具集(= 五家"tools assembled outside the core")
    resolver := toolset.NewResolver(toolset.Deps{
        CodeIntel: codeIntel, Background: bg, Skills: skillsSvc,
        Online: onlineTools, MCP: mcpTools, A2A: a2aTools, /* fs/bash 内建 */
    })

    // 3. 注入核心的窄 port:工具集 resolver + maintenance hooks + prompt-context + conversation
    eng, err := engine.New(ctx, engine.Config{
        ChatClient:   cc,            // model port（已有）
        ToolResolver: resolver,      // 工具集 port（核心只见这个，不见 codeintel/exec/...）
        Compactor:    compactor,     // maintenance hook（接口，runtime 注入 maintenance 实现）
        Extractor:    extractor,
        Planner:      planner,
        Knowledge:    knowledge,     // prompt-context port
        AgentDocs:    agentDocs,
        Conversation: convo,         // conversation port
    })
    // 4. runtime 持 engine + 其余直接消费的 service（session/transcript/workspace/...）
}
```

要点:
- **核心只见窄 port**(工具集 / maintenance / prompt-context / conversation),**见不到 codeintel/exec/mcp/a2a/skills** —— 后者是装配层的依赖。
- **engine.New 不再 `codeintel.New()` / `exec.NewManager()` / 建工具** —— 构造能力 + 装配工具是 runtime 的事;能力 **lifecycle/Close** 归 runtime。
- port 接口由 engine 定义,实现**结构化满足**(多数无需 import engine)。

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

## 7. 迁移批次(已落地)

> 两条主线:**(I) 把工具构造移出核心 → 装配层**;**(II) 把核心消费的能力翻成 engine 定义的窄 port + 注入**。

- ✅ **M1 — 工具装配层 `internal/engine/toolset`**:builders(workdir/online/lsp/bgshell/skill + edit-guards)+ `Resolver` + seam 移出 engine 核心到 `toolset`(import 能力 + agent/core + tools/*,**不 import engine**)。ask_user/task 由 engine 建好经 `SetTask`/`SetAskUser` 注入。engine 核心瘦身 ~800 LOC。
- ✅ **M2 — conversation + maintenance port 化**:engine 定义 `Conversation`/`Compactor`/`Extractor`/`Planner` 窄接口 + 契约结果类型(port.go);engine 持接口、不再构造;runtime 注入实现。maintenance 反过来 import engine(adapter→kernel,微内核允许)。每个 port 用处 nil-guard,故 bare `engine.New` 仍可跑 loop(测试零 churn)。
- ⏸ **M3 — knowledge/agentdoc — 评估后保留(net-negative ceremony)**:`knowledge.Service` 已是注入的接口(rpc 也用),再定义 engine port 会**重复其 `Scope` 枚举**(违 CLAUDE.md「enum 双份」);`agentdoc` 是**无状态包函数**,套接口是仪式。`skills` 同理(engine.ListSkills 薄委派)。这三者作为**共享接口/枚举/无状态 helper** 留在 engine 的 import 里 —— 不是具体 service 耦合,符合 §8 精神。
- ✅ **M4 — 能力+工具构造上移装配层 + runtime 注入**:新增 `toolset.Build(ctx, BuildConfig)` 作唯一构造点(codeintel/exec/mcp/a2a + resolver + 工具表 + MCPControl + closers);engine.Config 去掉 Online/MCPServers/A2AAgents/LSPServers(→ runtime.Config),换成注入的 `ToolResolver`/`Tools`/`MCP`/`Closers`。engine.New 不再构造任何能力,只注册 resolver + 建 task/ask_user + Close 时跑 closers。MCP facade 经 `toolset.MCPControl` port(engine 零 import infra/mcp)。runtime.New 是 composition root,调 toolset.Build + 注入。
- ✅ **M5 — 验证**:实测 `internal/engine/*.go`(核心,非 toolset 子包)**零 import `internal/infra/*`、零 codeintel/maintenance/conversation**。chat 留在 `engine/chat`(微内核下"消费 engine 的子包"正常,移回 service/chat 是可选的命名偏好,非必需,未做)。

---

## 8. 完成态(self-check,2026-06-12 已达成)

- ✅ engine **核心**(`internal/engine/*.go`,不含 `toolset` 子包)**零 import `internal/infra/*`、零 codeintel/maintenance/conversation 具体包**。剩余 lyra-internal import 仅:`engine/toolset`(自家装配子包)+ `service/{knowledge,agentdoc,skills}`(共享接口/枚举/无状态 helper,见 §7 M3,非具体耦合)。
- ✅ model(`ChatClientProvider`)+ 工具集(`ToolResolver`)+ maintenance(Compactor/Extractor/Planner)+ conversation 都是**注入的 port**;engine 核心不构造它们。
- ✅ 能力构造集中在 `toolset.Build`(装配层),runtime 是唯一 composition root,调它并注入;能力 lifecycle(Close)经注入的 closers 由 engine 在 Close 跑(runtime 拥有装配)。
- ✅ 无 Service Locator(无 service `import engine` 取公共能力);ambient cwd/session 走 `context.Context`(blackboard + `toolset.TurnCwd`)。
- ✅ bare `engine.New`(空 port)仍驱动 loop —— 每个 port 用处 nil-guard,核心可孤立测。

> 维护:本文件与 LAYERING.md §1「engine↔service 上下关系」冲突处,**以本文件为准**。微内核迁移 M1/M2/M4/M5 已落地,M3 评估后保留(ceremony)。
