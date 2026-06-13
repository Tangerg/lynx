# EXTENSIBILITY.md — Lyra 的可替换性边界（外部 SPI vs 内部焊死）

> **⚠️ 目录已重命名（2026-06-14，见 [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md) §9）**：`internal/engine→internal/kernel` / `internal/service→internal/domain` / `rpc→internal/delivery` / `engine/chat→kernel/turn`。本文行文中的旧路径名（`engine.Compactor` 等端口仍属 `kernel` 包）指代重命名后的同一目录，未逐处回改。

> **取代** 旧的 `EXTENSION_POINTS.md`（那份是"kernel 不长肉、**所有**能力都是插件、连 agent loop 都是插件"的 aspirational 蓝图，过度抽象，已删）。本文件是**已落地现状 + 判断准则**，不是蓝图。
> 配套：[`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md)（现行架构基准：微内核 + 限界上下文 + SPI）。

---

## 1. 准则：什么该抽接口，什么该焊死

**只为「外部/第三方能合理替换的服务」抽接口（SPI）；和系统核心强耦合的东西，焊死成具体类型，不抽接口。**

- **抽 SPI 的判据**：问一句「**一个外部 provider 会不会真的来实现它？**」——会（mem0 做 memory、某服务做压缩、某后端做向量检索），就抽接口，让官方实现只是其中一种、第三方经 HTTP 桥接自带一种。
- **焊死的判据**：它是不是**核心胶水**（驱动 run、状态机、传输、事件投递）——是，就没有外部实现者，给它套接口是 **YAGNI 仪式**，反而降内聚。
- **两头都是错**：过度抽象（给焊死的核心套没人实现的接口）和抽象不足（把可替换服务焊死）**一样糟**。

> 这条准则**收敛**了旧 `EXTENSION_POINTS.md` 的过度主张——agent loop 是核心，**焊死**，不是插件。

---

## 2. 外部 SPI（可替换 · 接口 · 可注入第三方实现）

这些是接口；官方在 `internal/service/*` / `internal/infra/*` 提供内部实现，组合根注入；第三方可经 HTTP 桥接自带实现替换。

| 能力 | 接口（端口） | 内部实现 | 外部替换示例 |
|---|---|---|---|
| **长期记忆** | `knowledge.Service` | `storage.FileKnowledgeService`（LYRA.md） | **mem0** 经 HTTP |
| **上下文压缩** | `engine.Compactor` | `maintenance.Compactor` | 外部压缩服务 |
| **事实提取** | `engine.Extractor` | `maintenance.Extractor` | 外部记忆抽取 |
| **规划** | `engine.Planner` | `maintenance.Planner` | 外部 planner |
| **转向注入** | `engine.SteeringSink` | `conversation.Service` | 少见，但可换 |
| **聊天历史存储** | `memory.Store`（SDK） | `sqlite.MessageStore` | 外部历史后端 |
| **LLM** | `core.ChatClientProvider` + `provider.Service` 注册表 | anthropic/openai/… | 任意 provider |
| **工具 / MCP / A2A** | `tool.ToolSource` / MCP / A2A 协议 | 内置工具集 | MCP server / A2A agent（天然外部） |
| **会话 / 时间线 / 中断 / provider 存储** | `session.Service` / `transcript.Store` / `interrupts.Store` / `provider.Service` | sqlite 实现 | 可换后端（语义仍 lyra 专有，主要为测试+后端替换） |

**怎么注入外部实现**（关键——SPI 必须能被组合根塞进去）：
engine 消费的端口在 `engine.Config` 暴露为接口字段；`runtime.New` **仅当字段为 nil 时**才建内部默认实现，否则**尊重注入值**。所以：

```go
// 组合根（cmd/lyra 或测试）：塞一个 mem0/HTTP 桥接的实现
eng := engine.Config{
    Knowledge: myMem0KnowledgeService,   // 实现 knowledge.Service
    Compactor: myHTTPCompactor,          // 实现 engine.Compactor
    // 其余留 nil → runtime 建内部默认
}
```

`knowledge` 已从 `cmd/lyra` 注入；`Compactor`/`Extractor`/`Planner`/`Steering` 经上述 nil-default 接缝注入（见 `internal/runtime/runtime.go`）。

---

## 3. 内部焊死（强耦合核心 · 具体类型 · 不抽接口）

这些**没有外部实现者**——它们就是 lyra 本身。给它们套 SPI 是过度抽象。

| 焊死的核心 | 为什么不抽 |
|---|---|
| **engine / agent-loop**（`internal/engine`） | "怎么跑一个 turn" 就是 lyra 的核心；ACL 包住 agent SDK，不对外开实现 |
| **turn 状态机**（`internal/engine/chat`） | 生命周期/HITL/steering 编排，纯核心胶水 |
| **transport / dispatch / protocol**（`rpc/*`） | 线本身 + 方法路由；契约是冻结的，不是可替换服务 |
| **事件投递 hub / pump**（`rpc/server/hub.go`） | per-run 事件扇出，核心机制 |
| **run / session 生命周期容器**（`rpc/server` 记账） | 哪些 run 在跑/能否取消，核心状态 |
| **`conversation.Service`** | 只是 `memory.Store` 之上的薄包装——**真正的替换接缝在底下的 `memory.Store`**（已是 SPI），所以这层焊死 |
| **`codeintel.Service`** | 包 LSP client；LSP 是标准协议，无"外部 code-intel provider"场景（要时再议） |
| **`clientResolver` / toolset 装配** | per-run model 解析 + 工具环境组装，组合根内部逻辑 |

---

## 4. 强反向不变量（别做）

- ❌ **给焊死的核心套接口**（engine/loop/transport/conversation 抽 SPI）——过度抽象，降内聚，回退。
- ❌ **把可替换服务焊死**（直接 `import` 具体 `maintenance.Compactor` 去消费，而非经 `engine.Compactor` 端口）。
- ❌ **SPI 接口的语义泄漏 transport/auth 关注点**——外部实现只拿领域输入，session id / 鉴权 / trace 走 `context` / header，不进端口签名。
- ❌ **runtime 无条件覆盖注入的端口**——必须 nil-default（尊重外部注入），否则 SPI 名存实亡。
- ❌ **重新引入"agent loop 也是插件"那套**——见 §1，agent loop 是焊死核心。

---

## 5. 一句话

可替换的服务（记忆 / 压缩 / 提取 / 规划 / LLM / 检索 / 工具）抽成 SPI 接口，第三方可经 HTTP 桥接替换；强耦合的核心（loop / 状态机 / 传输 / 事件 / 会话容器）焊死成具体类型。判据只有一个：**外部会不会真的来实现它**。
