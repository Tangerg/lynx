# Fat Services, Thin Engine — lyra 领域模型改革方案

> **核心命题**：lyra 当前 `engine(6269L) > 全部 service 之和(2172L)` 的体重分布是反的。
> 对标 Claude Code / OpenCode / Cline / Continue 四家的架构，结论是：**让 service 自己长肉，
> engine 只做装配和编排。每个领域自包含（类型 + 实现 + 构造），不拆接口/实现/工具构造三层。**

## 1. 问题诊断

### 1.1 体重失衡

```
engine (4522L) + engine/chat (1747L) = 6269L     ← 编排层
─────────────────────────────────────────
全部 14 个 service 合计                 = 2172L     ← 领域层
```

engine 是全部 service 之和的 **3 倍**。且 14 个 service 中：

| 类别 | 例子 | LOC | 本质 |
|---|---|---|---|
| **接口壳** | knowledge, transcript, interrupts, provider | 57–74 | 只定义 `type Service interface` + 类型，实现在 `infra/storage` |
| **薄 wrapper** | session, workspace, conversation, skills | 104–191 | 少量逻辑，主体是调 infra 的 pass-through |
| **真领域服务** | maintenance, codeintel, agentdoc, tool, approval | 128–479 | 有实质性领域逻辑 |

### 1.2 领域逻辑散落三处

以 LSP 代码智能为例，同一个领域被切成 4 个包：

```
service/codeintel/service.go      ← 服务接口 (357L)
infra/lsp/{client,manager,...}.go ← LSP 客户端实现
engine/toolset/lsptools.go        ← 工具构造 (171L)
engine/toolset/editguard.go       ← 编辑守卫 (218L)
```

以内存/知识为例：

```
service/knowledge/knowledge.go    ← 纯接口定义 (57L)
infra/storage/knowledge_store.go  ← 文件读写实现
engine/prompt.go + engine.go      ← 调用编排
```

### 1.3 根因

当前架构把"分层纯粹性"（service 只能定义接口、infra 只能做存储、engine 只能做编排）
当作不可侵犯的原则，结果就是：

- 本应属于一个领域的代码被强行拆到三个层级
- 每个层级单独看都太薄，加起来却分布在不该在的地方
- engine 被迫持有大量本应属于 service 的领域知识

## 2. 对标分析

### 2.1 五个仓库的架构模式

逐一阅读了 Claude Code、OpenCode、Cline、Continue 四个 Agent 工具的源码。
共同规律：**全部不用 lyra 当前的"接口壳 service + 分离 infra"模式**。

#### Claude Code（TypeScript, ~3000L 引擎）

```
src/
├── services/           ← 自包含领域模块（含有实现、不是接口）
│   ├── compact/        ← 10 文件，含算法+prompt
│   ├── lsp/            ← 7 文件，客户端+管理+诊断全部在一起
│   ├── mcp/            ← 22 文件，完整 MCP 连接生命周期
│   ├── extractMemories/← 提取算法+prompt
│   └── tools/          ← 工具执行器+编排
├── tools/              ← 43 个工具目录，每个工具自包含（实现+prompt+UI）
│   ├── BashTool/       ← 16 文件，含安全/权限/校验/UI
│   └── FileWriteTool/  ← 实现+prompt+UI
├── query.ts (1729L)    ← 引擎 loop（import 服务，不包含领域逻辑）
└── QueryEngine.ts (1295L)
```

**关键**：服务就是实现。`services/mcp/` 不是接口定义，是 22 个直接实现文件。

#### OpenCode（TypeScript, ~300L 引擎）

```
packages/core/src/      ← 扁平文件-per-领域，无 engine/service 分层
├── agent.ts (142L)
├── session.ts (436L)
├── tool.ts
├── project.ts / workspace.ts / filesystem.ts / git.ts
├── permission.ts / plugin.ts / skill.ts
└── session/            ← 复杂领域有子目录
    ├── runner/
    ├── message/
    └── sql/
packages/cli/           ← 交付层
packages/server/        ← 交付层
```

**关键**：根本没 engine 层。`core/` 就是全部领域逻辑，CLI/TUI/Server 只是 delivery。

#### Cline（TypeScript, ~400L 引擎）

```
@cline/core             ← SDK（npm 包），含全部领域+工具+provider
apps/cli/src/runtime/
├── run-agent.ts (405L) ← 薄 CLI 运行时
└── tools.ts (17L)
apps/vscode/src/        ← 扩展集成
```

**关键**：核心 SDK 和交付层分离，SDK 自带全部实现。

#### Continue（TypeScript, ~1460L 引擎）

```
core/
├── core.ts (1460L)     ← 单一入口，import 各领域模块
├── tools/              ← 工具实现
├── llm/                ← 模型适配
├── config/             ← 配置
├── edit/               ← 编辑
└── diff/               ← 差异
```

**关键**：`core.ts` 是大文件但有边界——它只做编排，领域逻辑各在 `tools/`, `llm/`, `edit/`。

### 2.2 对比表

| | 四家仓库 | lyra |
|---|---|---|
| **服务有肉吗** | 有。`BashTool` 16 文件、`LSP` 7 文件 | 无。knowledge 57L 纯接口 |
| **接口/实现分离吗** | 不分。服务即实现 | **分**。`service/X` 定义接口 → `infra/X` 实现 |
| **领域逻辑在哪** | 自包含在各自的目录/文件里 | **散落三处**（service + infra + engine/toolset） |
| **引擎多胖** | 薄。import 服务、按序调用 | **胖**。`engine/toolset/resolver.go` 368L 的领域规则 |
| **工具构造在哪** | 工具目录内自构造 | `engine/toolset/*`，和领域分离 |

### 2.3 核心教训

> **"接口壳 service" 不是服务 —— 它只是把 import 路径拉长了一截。**
>
> 真正的服务 = 类型 + 不变量 + 行为 + 持久化 + 构造，
> 全部在一个包内。不为了"分层纯粹"拆成三块。

## 3. 改进原则

### 3.1 一个领域 = 一个包 = 自包含

```
现在：service/knowledge (接口) → infra/storage (实现) → engine (编排)
改为：service/knowledge (类型 + 实现 + 构造)
```

不再有"接口项目"——每个 service 包既是接口也是实现。
删掉 `Service` 和 `Store` 两个 interface 之间的一个（谁只有单实现就删谁）。

### 3.2 领域逻辑从 engine 回流到 service

| 当前在 engine | 应归属的 service | 原因 |
|---|---|---|
| `toolset/resolver.go` (368L) | `service/tool` | 工具按 cwd 解析是领域规则 |
| `toolset/editguard.go` (218L) | `service/tool` 或 `service/codeintel` | read-before-edit 是领域不变量 |
| `toolset/lsptools.go` (171L) | `service/codeintel` | LSP 工具构造是 codeintel 的职责 |
| `toolset/bgshell.go` (115L) | `infra/exec` 或 `service/tool` | 后台 shell 执行机制 |
| `observer.go` 审批判定 | `service/approval` | 审批规则是 approval 的领域逻辑 |

### 3.3 Engine 只保留编排

engine 的目标体型：~2000L（从 6269L 瘦下来），只包含：

- `engine.go` — 装配 system prompt + 工具集 + model client
- `chatturn.go` — 驱动一个 turn 的 Start → Loop → End 状态机
- `chat/turn.go` — turn 内事件循环（已接近目标形态）
- `chat/service.go` — StartTurn / Resume / Cancel 入口

不应该留在 engine 的：
- 工具解析规则 → service/tool
- LSP 工具构造 → service/codeintel
- 编辑守卫逻辑 → service/tool 或 service/codeintel
- 审批判定 → service/approval

## 4. 执行路径

分三批，每批可独立 revert，`go build && go vet && go test ./...` 全绿。

### 批次 A — 接口壳消融（低风险）

把 4 个"纯接口定义"包变成有肉的服务。

```
service/knowledge/  ← 吞并 infra/storage/knowledge_store.go
service/transcript/ ← 吞并 infra/storage/sqlite/transcript.go
service/interrupts/ ← 吞并 infra/storage/sqlite/interrupt.go
service/provider/   ← 吞并 infra/storage/sqlite/provider.go
```

每个包：
- 删除中间的 `Service`/`Store` 接口（单实现，YAGNI）
- 把 infra 实现移入 service 包
- 删除 sqlite 子包中对应的文件
- 调用方（engine、rpc/server）从 `import service/knowledge` 拿到直接可用的 `*Service`

### 批次 B — 领域逻辑回流（中风险）

把 engine 中散落的领域逻辑移到对应的 service。

```
service/codeintel/  ← engine/toolset/lsptools.go (171L)
                    ← engine/toolset/editguard.go (218L) 中 LSP 相关部分

service/tool/       ← engine/toolset/resolver.go (368L)
                    ← engine/toolset/build.go (132L)
                    ← engine/toolset/skill.go (30L)
                    ← engine/toolset/bgshell.go (115L)  → infra/exec/

service/approval/   ← engine/observer.go 中的审批判定逻辑
```

### 批次 C — 聚合合并（需要设计）

把 Session-Transcript-Conversation 三个存在强耦合的服务合并为一个聚合。

```
service/session/    ← session + transcript + conversation 合并 (~400L)
  aggregate.go      ← Session 聚合根，拥有 Item/Run/Message
```

## 5. 对标总结

| Claude Code | OpenCode | Cline | Continue | lyra 改后 |
|---|---|---|---|---|
| services/ 含实现 | core/src/ 扁平文件 | @cline/core SDK | core/tools/ 含实现 | service/ 含实现 |
| tools/ 各含实现 | tool.ts 单文件 | SDK 内 | core/tools/ | service/tool/ 含构造 |
| query.ts 薄编排 | 无 engine | run-agent.ts 405L | core.ts 编排入口 | engine/ ~2000L 纯编排 |

## 6. 明确不做

- ❌ 不引入 Repository 层 —— 现有直调 sqlite 够用
- ❌ 不引入 Application Service 层 —— engine 就是编排层
- ❌ 不引入 Domain Event Bus —— 单进程、engine 编排够用
- ❌ 不为单实现保留接口 —— YAGNI，删掉 knowledge.Service/provider.Service 等接口
- ❌ 不把 infra/storage 全部拆到 service —— storage/sqlite 的 DB 初始化、迁移、连接管理等基础设施保留在 infra 层

> 修改历史：
> 2026-06-12 — 初稿。基于对 Claude Code、OpenCode、Cline、Continue 四家仓库的源码审计。
