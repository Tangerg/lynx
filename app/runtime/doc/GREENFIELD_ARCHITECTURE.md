# Greenfield 架构设计 —— 如果从零重写 Lyra

> **日期**：2026-06-14。**视角**：架构师命题作文 —— "假设从零开始写 Lyra，目录结构与架构怎么设计？"
> **状态**：**现行架构基准（canonical）**。§6 的目录/命名重构已于 2026-06-14 落地（见文末 §9）—— 代码按本文树形组织（kernel/domain/delivery/turn）。**同日清理**把多份演进切片（`LAYERING`/`MICROKERNEL`/`STRUCTURE_REVIEW`/`ARCHITECTURE`/`ROADMAP`）折叠进本文并删除,历史见 git。配套 [`EXTENSIBILITY.md`](EXTENSIBILITY.md)（SPI vs 焊死政策）+ [`../CLAUDE.md`](../CLAUDE.md)。
> **方法**：第一手通读 lyra 现状结构（rings / ports / 组合根 / arch_test）+ 对照那几份演进文档，把"用 11 个重构批次、3 次文档互相 supersede 才逼出来的终态"重写成**第一天就刻意设计的起点**。
>
> **结论先行**：
> 1. **从零，我会写的就是现状这套** —— Clean Arch 依赖规则锁方向 ⊕ 微内核（engine 作核、定义 port）⊕ DDD 限界上下文承载领域 ⊕ SPI/焊死二分 ⊕ `arch_test` 防腐。代码组织约 **90% 收敛**，这个收敛本身就是现状架构健康的证明。
> 2. **从零的真正红利不在推翻，在"无包袱"**：① 从第一天只用**一个**心智模型（依赖向内），不留 LAYERING 那套"infra 在最底"后来要被推翻的措辞；② **目录名 = 环名**（`engine→kernel`/`service→domain`/`rpc→delivery`），读目录树即懂架构；③ adapter / use-case 边界**一开始就画硬**。
> 3. **这三处差异在存量上"从零值得、现在改不值得"** —— 纯机械 ripple、零行为变化、违反"不为省事之外的理由制造 churn"的纪律（`rpc→api` 已 PARKED 同理）。本文把它们记为**设计取向**，不是**重构待办**。

---

## 0. 怎么读这份文档

- **作者评审视角**：本文是"如果重来"的应然设计，用来和现状对照、检验现状的每一处决策是不是经得起第一性推敲。**它不引入新债，也不要求改名**（§6 解释为什么存量不动）。
- **与既有文档的关系**：这是 LAYERING → MICROKERNEL → STRUCTURE_REVIEW 三轮演进的**收束态快照**。那三份是**演进的考古层**（各自描述一个时间切片，且互相"本文 §X 以那份为准"）；本文把终态合成一处。若哪天要给新人一个**单一入口**，本文的 §1–§5 即可充当（见 §6③）。

---

## 1. 先钉死一个心智模型（从零最大的红利）

现状最大的"历史包袱"不是代码，是**文档里并存着两套互相矛盾的分层叙述**：

| | 传统分层（LAYERING 的措辞） | Clean Arch / 六边形（代码实际走的） |
|---|---|---|
| 形状 | 自上而下 `delivery→engine→service→infra` | 同心环，依赖**向内** |
| infra 位置 | **最底**，被依赖 | **最外**，依赖向内（DB 是细节） |
| `infra→service` | ❌ 违规（底层依赖上层） | ✅ 正确（外层适配器→内层端口/实体） |

代码一直走右列，文档却用左列描述，于是 STRUCTURE_REVIEW 初版列的"反向依赖违规"全是幻觉。**从零，只用一句话定义整个架构，不留第二套叙述：**

> **依赖只能指向内层（领域）。外层（交付 / infra 适配器）可以认识内层；内层永远不认识外层；同环的适配器不能横跨内核互相伸手。**

这正是 `internal/arch/arch_test.go` 现在机器强制的规则。从第一天就写它 —— LAYERING 那张"自上而下 infra 在底"的图根本不会出现。

---

## 2. 架构本质，三句话

1. **agent 的本质是一个 loop**（组上下文 → 调 LLM → 跑工具 → 喂回 → 重复），而这个 loop **不在 Lyra 里**，在 `agent/runtime` SDK 的 `for{}`。Lyra 没有手写循环，复用框架原语。
2. **engine = 驱动这个 loop 的微内核**：提供"怎么跑一个 turn"的机制，把**自己需要的外部能力声明成窄 port**，自己不实现这些能力。
3. **其余一切只有两种身份**：要么是"loop 往下调的能力"（domain + infra），要么是"把 loop 包成协议给前端的适配器"（delivery）。

---

## 3. 同心环图

```
                         runtime  (组合根 / composition root)
                        /         |          \
        ┌──────────────┘          |           └──────────────┐
        ▼                         ▼                           ▼
   ┌─────────┐  注入 port   ┌───────────┐  实现 port   ┌──────────┐
   │ delivery │ ──────────► │  kernel    │ ◄────────── │  domain   │
   │ (rpc/线) │   驱动 turn   │ (engine)   │  结构化满足   │ 限界上下文 │
   └─────────┘             │ 定义窄 port │             └────┬─────┘
        │                  │ 驱动 loop   │                  │ 内部调
        │                  │ 装工具集    │                  ▼
        └──────────────────┴────────────┴──────────►  ┌──────────┐
                  依赖一律向内（指向 domain）            │  infra    │
                                                       │ 驱动适配器 │
                                                       └──────────┘
```

要点：engine 既"在上"（被 delivery 驱动），又"定义 port 让 domain 实现"，所以它**不是一层**，是同心环的**核**（kernel）。这是微内核设计的核心洞察 —— 从零应该让目录结构直接长成这个样子，而不是先摆成四层再纠结"chat 在 engine 之上还是之下"。

---

## 4. 从零的目录结构

```
lyra/
├── cmd/lyra/                       # CLI 入口（组合根的最外壳）
│
├── internal/
│   ├── kernel/                     # ◀ 现 engine：微内核 —— "怎么跑一个 turn"
│   │   ├── port.go                 #   核定义的窄 port（model / toolset / maintenance / prompt-ctx / conversation）
│   │   ├── loop.go                 #   驱动 agent SDK 的 for{}；start/resume/cancel/steering
│   │   ├── prompt.go               #   system prompt 骨架装配（调 prompt-ctx port）
│   │   ├── hitl.go                 #   park-on-interrupt + resume 编排
│   │   ├── turn/                   # ◀ 现 engine/chat："跑一个 turn" 这个用例（状态机/lifecycle/observer/policy）
│   │   └── toolset/                #   工具装配层：builders + resolver（在 loop 之外组好工具再注入）
│   │       ├── resolver.go         #   多源聚合 + per-role/per-cwd 解析
│   │       ├── build.go            #   唯一构造点（codeintel/exec/mcp/a2a → 工具表 → resolver → closers）
│   │       └── {shell,lsptools,askuser,skill,todotool,turnctx,editguard}/
│   │
│   ├── domain/                     # ◀ 现 service：限界上下文（实体 + 仓储 port + 领域服务）
│   │   │                           #   ⚠ 不是全局实体环 —— 每个子目录是自包含上下文
│   │   ├── session/                # 会话聚合根（Fork/NewSubtask/rollback 不变量）
│   │   ├── transcript/             # items+runs 时间线（rollback 边界算法 = 聚合不变量）
│   │   ├── conversation/           # 喂 LLM 的 chat.Message[] 上下文
│   │   ├── knowledge/              # LYRA.md 长期知识（用户可编辑）
│   │   ├── maintenance/            # 压缩 / 提取 / 规划（turn 边界自治操作）
│   │   ├── codeintel/  workspace/  # 代码智能（包 lsp）/ VCS 视图+checkpoint（包 git）
│   │   ├── approval/  interrupts/  # 审批 stance / HITL 中断登记
│   │   ├── provider/  tool/        # provider 注册表 / 工具注册表
│   │   └── agentdoc/ skills/ todo/ # AGENTS.md 发现 / skill 取用 / 任务清单
│   │
│   ├── infra/                      # 驱动适配器（实现 domain 与 kernel 的 port，零领域知识）
│   │   ├── storage/sqlite/         #   单一 SQLite 后端 + file（LYRA.md）
│   │   └── {git,lsp,checkpoint,exec,mcp,a2a}/
│   │
│   ├── delivery/                   # ◀ 现 rpc：接口适配器（wire 契约 + 传输）
│   │   ├── protocol/               #   wire 类型 + Runtime port（冻结契约）
│   │   ├── server/                 #   Runtime 实现：decode → 调用例 → present（薄！）
│   │   ├── dispatch/               #   JSON-RPC 方法路由（表驱动）
│   │   └── transport/{http,inprocess}/
│   │
│   ├── runtime/                    # 组合根：把各环拼起来，nil-default 注入 SPI
│   └── config/                     # env / 配置解析（纯数据）
│
└── doc/                            # GREENFIELD_ARCHITECTURE.md（本文 = 架构基准）+ EXTENSIBILITY.md（SPI 政策）
```

> 跟现状的差异**只有三个名字**：`engine→kernel`、`service→domain`、`rpc→delivery`（§6 详述）。代码组织几乎一致。

### 限界上下文怎么判断"该不该新设一包"

contexts 概念上按"在 loop 的哪一步被调"分五组（沿 LAYERING §3）：

| 组 | 上下文 | 在 loop 里的角色 |
|---|---|---|
| ① prompt-context | knowledge / agentdoc / skills | 拼 system prompt 的素材源 |
| ② 会话与历史 | session / transcript / conversation | 会话实体 + UI 时间线 + LLM 消息上下文 |
| ③ 工具能力 | codeintel / workspace / tool | loop 执行步要调的能力 |
| ④ 策略 / 横切 | approval / interrupts / provider | 审批 stance / HITL / provider 注册表 |
| ⑤ 维护 | maintenance | turn 边界自治操作（压缩/提取/规划） |

**判据**：新能力归哪组？组内已有上下文能容纳吗？—— **内聚但不原子化**：一个领域一包，别为"看起来分层"把单一职责再切碎；但薄到只是无 I/O 的 helper（如 fs/shell/web 工具）就别单列 service（YAGNI），直接 infra + kernel 装配。

---

## 5. 关键设计决策（逐条带 why）

### 5.1 微内核 + 窄 port + nil-default 注入（脊梁）

engine 只定义它**直接消费**的 port，集中在 `port.go`：

| port | 方法面 | 实现 | 核拿它干嘛 |
|---|---|---|---|
| model | `core.ChatClientProvider` | runtime `clientResolver` | 调 LLM、per-turn 换 model |
| 工具集 | `core.ToolGroupResolver`（per-role/cwd） | `kernel/toolset.Resolver` | 提供该 turn 的 `[]Tool` |
| maintenance | `Compactor`/`Extractor`/`Planner` | `domain/maintenance` | turn 边界压缩/提取/规划 |
| prompt-context | `Knowledge`/`AgentDocs` | `domain/knowledge`、`agentdoc` | 拼 system prompt |
| conversation | `SteeringSink`（1 方法） | `domain/conversation` | turn 内转向注入 |

**关键收窄**（读 codex / claude_code / cline / opencode / kimi-code 五家源码后的实证）：核心 loop 的真端口很窄 —— 基本是 **model + 工具集 + 几个 hook**。`codeintel / exec / mcp / a2a / skills` **不是核心 port** —— 它们被**工具**包住，工具在 `kernel/toolset` 装配层（loop 之外）组好再注入，核心只见"工具集 resolver"这一个 port。给每个能力都设核心 port 是过细的 12-port 反模式（违 ISP/YAGNI）。

**SPI 接缝**：port 在 `kernel.Config` 暴露为接口字段；`runtime.New` **仅当字段为 nil 时**才建内部默认实现，否则**尊重注入值**。这是 SPI 能被第三方（如 mem0/HTTP 桥接）替换的接缝。

### 5.2 不抽全局实体环

Clean Arch 教科书会引诱你建 `internal/domain/entities`。**不做。** session 的"实体 + 仓储 port + 领域服务"高内聚地待在 `domain/session/` 里 —— 这正是 DDD 限界上下文。抽成全局实体环会得到**贫血实体 + 逻辑分家**，反 DDD、降内聚。`infra→domain` 在 Clean Arch 下本就正确（适配器→内层端口）。

### 5.3 不设独立 application / use-case 环

单交付运行时下，一个 `internal/app` 层就是 `delivery/server` 的 1:1 影子。用例归属**按性质分派**，不塞进一个层：

| 用例性质 | 落在哪 |
|---|---|
| "跑一个 turn" | `kernel/turn`（就是这个用例的状态机） |
| 聚合不变量（rollback 边界、fork 续链） | 对应 `domain` 服务（现状 F1 已把它从 `rpc/server/rollback.go` 请回 `transcript`） |
| 纯 CRUD 直读 | `delivery/server` 直接调 domain，不绕 |

**触发条件**（满足才建 app 环）：出现第二种交付（CLI 直连 / remote runtime），或用例需脱离 wire 单测。现在建是 YAGNI。

### 5.4 SPI vs 焊死，判据只有一句："外部会不会真的来实现它？"

- **会** → 抽接口（SPI）：knowledge（mem0）、compaction、extraction、planning、LLM provider、chat 历史存储、工具/MCP/A2A、会话/时间线/中断/provider 存储（后端可换）。
- **不会** → 焊死成具体类型：agent loop、turn 状态机、transport/dispatch/protocol、事件 hub、run/session 生命周期容器、`conversation.Service`（真正的接缝在底下的 `memory.Store`）、`clientResolver`/toolset 装配。
- **两头都是错**：过度抽象（给焊死核心套没人实现的接口）和抽象不足（把可替换服务焊死）一样糟。

### 5.5 Ubiquitous language 从第一天就对齐

现状踩过的坑："memory" 和 "history" 各指两样东西。从零直接用三个不同的名字，因为三者生命周期、消费方、存储都不同：

| 概念 | 是什么 | 名字 |
|---|---|---|
| 用户可编辑的长期知识 | LYRA.md（跨会话） | `knowledge` |
| 喂给 LLM 的消息上下文 | `chat.Message[]` | `conversation` |
| 给 UI 渲染的时间线 | items + runs | `transcript` |

### 5.6 ambient 走 `context.Context`，不进 port

cwd / session id 是"本次 turn 的环境"，不是稳定能力 —— 挂 process blackboard、经 `turnCwd(ctx)` 读。**不要**做成"问 engine 要当前目录"（那是 Service Locator / god-object，与低耦合相反）。

### 5.7 观测走 OTel 三驾马车 sink 到 slog，不在业务码撒 `slog`

一个事件该被观测就开 span（带 attr）/ 记 metric，**不是**加一行 `slog.InfoContext`。attr key 去品牌（semconv 优先 `gen_ai.*`/`db.*`，否则裸 domain `run.*`/`agent.*`）。生产把 exporter 换 OTLP 即把 span/metric/log 全导云、业务零改。

### 5.8 `arch_test.go` 是防腐底座，第一天就写

用 `go/parser` 解析 import、机器断言依赖规则（`infra ↛ delivery/kernel`，`domain/kernel ↛ delivery`）。Go 没有强制分层，这个测试就是"如何让整洁架构保持整洁"的答案 —— 它通过本身，就证明代码早已向内依赖（初版怀疑的"反向依赖"是传统分层视角的错觉）。

---

## 6. 与现状的三处差异（诚实的增量）

### ① 目录名 = 环名（`engine→kernel`、`service→domain`、`rpc→delivery`）

现状 `arch_test.go` 注释里已经把 `internal/engine` 叫 `orchestration`、`internal/service` 叫 `domain`、`rpc` 叫 `delivery` —— **目录名和环名已经漂移**。从零会让它们一致，读目录树即懂架构。
> **存量改的代价**：纯机械 ripple（~170 处 import）、零行为变化。最初评估为"不值这串 churn"（`rpc→api` 曾 PARKED，同理）。
> **2026-06-14 更新**：在确认"差异极小、纯改名、零架构收益"后，用户仍选择执行以换取"目录名 = 环名"的可读性收益。已分批落地（§9）—— 全程 build/vet/test 全绿、每批可独立 revert。**故此条从"设计取向"转为"已执行"。**

### ② `delivery/server` 从第一天守住"decode → 调用例 → present"三段

现状 `Server` 80 方法 / 19 文件，大多 1:1 协议绑定（健康），但 F1 暴露过用例编排（rollback 时间线）漏进适配器。从零会把这条边界画得更早更硬：adapter 里**只准**有 wire↔domain 翻译 + 编排调用，任何"领域算法"（把 run 边界映射到消息 watermark 这种）一出现就立刻下沉 domain。**不是加层，是纪律。**

### ③ 一份架构文档，不是多份演进切片（**2026-06-14 已落地**）

原本 LAYERING / MICROKERNEL / STRUCTURE_REVIEW / ARCHITECTURE / ROADMAP 多份在描述同一架构的不同演进切片，互相"本文 §X 以那份为准"——演进考古记录,有价值但不该是入口。**已折叠**:本文（GREENFIELD_ARCHITECTURE.md）成为**唯一架构入口**（依赖规则 + 微内核 + port 表 + SPI/焊死判据 + 不做清单 + §9 执行记录）,那些演进切片已删、历史进 git log;`EXTENSIBILITY.md` 作 SPI 政策 companion 保留。

---

## 7. 明确不做（尊重 YAGNI 红线 + 触发条件）

| 不做 | 为什么 | 触发条件（满足才做） |
|---|---|---|
| 全局 `domain/entities` 环 | 反 DDD，降内聚 | 永不 |
| 独立 application-service 环 | 单交付下是影子层 | 第二种交付 / 用例需脱 wire 单测 |
| DI 容器（uber/dig） | 组合根手写注入足够清晰 | 永不（PortAI 教训） |
| event-bus / Mediator / CQRS / Saga | 已有 per-run hub；单团队单后端 | 永不 |
| 给焊死核心（loop/状态机/transport/conversation）套 SPI | 无外部实现者 | 出现真实第三方实现者 |
| 压缩 `Strategy` 接口 | 当前单实现 | 第二个压缩策略（microcompaction）真要落地 |
| 给 LLM provider 加 OAuth/token refresh | 用户填 key、401 让 UI 重填 | 永不 |
| 加 retry layer | SDK 内部 retry 够 | 永不 |
| 把实体抽全局环 / 因方法多就拆 `Server` | 反 DDD / 适配器方法多是 1:1 协议绑定，正常 | 永不 |

---

## 8. 一句话

从零，我会写的就是现状这套 —— **Clean Arch 依赖规则锁方向、微内核把 engine 收成"定义 port 的核"、限界上下文承载领域、SPI/焊死按"外部会不会来实现"二分、`arch_test` 防腐**。唯一区别是：让**目录名 = 环名**、从第一天用**单一心智模型**写文档、把 adapter/use-case 边界**一开始就画硬**。代码层面 90% 收敛，这个收敛本身就说明现状已经到位；剩下 10% 是把"演进逼出来的终态"变成"刻意设计的起点"。

> **后记（2026-06-14）**：那 10% 里的"目录名 = 环名"（§6①）已应用户要求执行落地（§9）；§6② adapter 边界、§6③ 文档单一入口仍是取向，未单独施工。

---

## 9. 执行记录（2026-06-14）

§6① 的目录/命名重构在与用户确认"差异极小、纯改名、零行为变化、零架构收益、可整体 revert"后执行。分批落地，每批 `go build && vet && test ./...` 全绿、单独 commit、可独立 revert：

| 批次 | 内容 | 手段 |
|---|---|---|
| 1 | `internal/service/* → internal/domain/*`（15 个限界上下文） | `git mv` + 72 处 import 路径改写 + arch_test 分环 |
| 2 | `rpc/* → internal/delivery/*`（移入 internal/，无 lyra 外消费者） | `git mv` + 57 处 import + arch_test |
| 3a | `internal/engine → internal/kernel` | `git mv` + import + `package` 子句 + **AST 安全的 `gofmt -r 'engine.a -> kernel.a'`**（56 文件，不碰 `r.engine` 字段） |
| 3b | `kernel/chat → kernel/turn` | `git mv` + import + `package` + 按文件 scoped `gofmt -r`（避开 SDK `core/model/chat`）+ 去 `chatsvc` 别名 |
| 4 | 文档对齐 | 本文 + `../CLAUDE.md` 当前态段 + arch_test 注释 + 根 `CLAUDE.md`；历史 changelog 保留当时路径名 |

**顺手精修**（不改业务逻辑）：修过期路径注释、`doc.go` godoc、`kernel/agent.go` 去可推断的显式类型参数、turn 包的 `"chat:"` 错误前缀 → `"turn:"`（已核 wire 错误分类不依赖该前缀）。

**未做**（仍是取向，等触发）：§6② adapter/use-case 边界硬化、§6③ 把四份演进文档合一份单一入口（仅在各文加了重命名提示横幅）；§7"明确不做"清单整体未动。

**Engine 类型名保留**：包改名 `kernel`，但主类型仍是 `kernel.Engine`（避免 `kernel.Kernel` stutter）；故 prose / 错误前缀里指代 Engine 组件的 "engine" 字样**有意保留**，非遗漏。
