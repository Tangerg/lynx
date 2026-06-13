# Lyra 结构审视 —— 向 DDD / 整洁架构演进

> **⚠️ 目录已重命名（2026-06-14，见 [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md) §9）**：`internal/engine→internal/kernel` / `internal/service→internal/domain` / `rpc→internal/delivery` / `engine/chat→kernel/turn`，目录名 = 本文所述 Clean Arch 环名。本文行文中的旧路径名指代重命名后的同一目录，未逐处回改。

> **日期**：2026-06-13。**视角**：纯软件工程（结构 / 分层 / 耦合内聚 / 设计模式），不谈能力面（见 [`AGENT_CAPABILITY_COMPARISON.md`](AGENT_CAPABILITY_COMPARISON.md)）。
> **North Star**：DDD + 整洁架构（Clean Architecture / Hexagonal）。
> **方法**：第一手通读结构（文件/struct/方法普查 + 依赖边 + 关键文件 + 符号级 import 分析），对照桌面所有 agent 应用。
>
> **结论先行（经一轮深挖+自我修正）**：
> 1. **lyra 的代码已经在沿 Clean Arch 的依赖规则走**——依赖**向内指向领域**。`service/<context>` 是合格的 **DDD 限界上下文**（实体+值对象+仓储接口+领域服务同处），`infra` 实现仓储并 import 这些上下文 = **正确的"适配器→端口"向内依赖**。
> 2. 因此初版我列的"3 条反向依赖"**大多不是 bug**——是 **架构文档的描述**（`delivery→engine→service→infra，infra 在最底`）用了**传统分层模型**，方向恰与 Clean Arch（infra 在**最外**、向内依赖）**相反**。要落 Clean Arch，**要改的是文档措辞与心智模型**，不是把依赖"扳回"传统分层。
> 3. **唯一与模型无关、确实该改的结构问题是 F1**：用例编排（rollback/fork/run 生命周期的领域算法）漏在接口适配器 `rpc/server` 里。DDD 与 Clean Arch 都要求它向内（领域服务 / 应用服务）。**这是真正的活。**
> 4. 其余：F3（门面去重，小）、F4（压缩 Strategy，YAGNI 门控）、+ 架构守护测试。
>
> **自我修正记录**：初版本文提出"Batch A — DAG 闭合"（把 `infra→service` / `maintenance→engine` / `config→engine` 这些边"扳正"）。深挖符号级用途后判定**那是 audit 误判**——那些边是 Clean-Arch 正确的向内依赖，扳了反而退回传统分层、远离你要的 Clean Arch。已撤销 Batch A。

---

## 0. 前提：现状已是第一梯队

普查（21k 非测试 LOC）：最大文件 460 行；无 god-object（最大 struct 14 字段且分区）；`dispatch` 表驱动；`Server` 80 方法但仅 6 字段、按域分 19 文件、多为 1:1 协议绑定；turn 状态机是教科书级"算 plan 再执行"。

对照：plandex 55 字段 god-struct；PortAI（已排除）87/100 方法 god-class + 65% 贫血模型 + `pkg→internal` 反向 7 处。**lyra 避开了它们的全部债务。** 这是"A → A+"。

---

## 1. 两套分层模型的冲突（问题的根）

| | 传统分层（文档现在的描述） | 整洁架构 / 六边形（你要的方向） |
|---|---|---|
| 形状 | 自上而下：`delivery→engine→service→infra` | 同心环，依赖**向内** |
| infra 位置 | **最底**，被依赖 | **最外**，依赖向内（DB 是细节） |
| 实体在哪 | 模糊（落在 service） | **最内环**（被所有环依赖） |
| `infra→service` | ❌ 违规（底层依赖上层） | ✅ 正确（外层适配器→内层端口/实体） |

**lyra 代码实际走的是右列**（infra 向内依赖领域）。所以"违规"是**文档用左列描述了一个右列的代码**。

**第一步（零代码、纯认知）**：把 `ARCHITECTURE.md` / `LAYERING.md` 的分层叙述从"infra 在最底"改写成 Clean Arch 的**依赖规则**（依赖只向内指向领域；infra/delivery 是最外的适配器）。这一步让"违规"消失、且把团队心智对齐到 DDD。

### lyra 映射到整洁架构四环（现状）

| 环 | 应含 | lyra 现状 | 评 |
|---|---|---|---|
| **① 实体/领域** | 实体+值对象+仓储接口+领域服务 | `service/<context>`（session/transcript/knowledge/provider/interrupts/approval/maintenance…）—— **已是合格限界上下文** | ✅ 别拆散 |
| **② 用例** | 应用服务，编排领域完成一个用例 | turn 用例在 `engine/chat` ✅；**其余用例漏在 `rpc/server`** | 🟡 **F1** |
| **③ 接口适配器** | controller/presenter/gateway | `rpc/dispatch`+`rpc/server`+`rpc/transport` | 🟡 **兼任了用例环** |
| **④ 框架与驱动** | DB/web/外部 SDK | `infra/*`、HTTP/SSE transport；`engine` 是 agent SDK 的 ACL | ✅ |

> **关键 DDD 判断**：**不要**把实体从 `service/<context>` 抽到一个全局 `internal/domain` 环。那些上下文已经把"实体+仓储接口+领域服务"高内聚地放在一起——这正是 DDD 限界上下文的样子。抽成全局实体环反而**降内聚**（贫血实体 + 逻辑分家），是反 DDD 的。`infra→service` 在 Clean Arch 下本就正确。

---

## 2. 唯一与模型无关的真问题：F1 —— 用例编排漏进适配器

`rpc/server/rollback.go:22-75` 的 `runTimeline` / `rollbackBoundary` / `boundaryAt` 是**实打实的领域算法**（把 run 边界映射到消息 watermark、算 inclusive-keep 切分），`runs.go`(367) 是 run 生命周期编排。turn 用例正确地在 `engine/chat`，**其余用例却住在接口适配器里**。

无论传统分层还是 Clean Arch，**领域算法都不该在协议适配器**：
- **DDD 视角**：这是 **Session 聚合的不变量**（runs 成时间线；rollback 在 root-run 边界截断、保留续链）。属聚合 / 领域服务。
- **Clean Arch 视角**：适配器（ring③）混入了用例/领域规则（ring②/①）。

**一个 wrinkle（影响怎么搬）**：`boundaryAt` 现在直接对 **wire 类型 `protocol.RunRef`** 反序列化推理（run blob 存的是 wire JSON）。要把它搬进领域，得让领域**不依赖 wire**——即引入一个领域 Run 表示（或让 `transcript` 上下文持有 run 时间线的领域类型），适配器只负责 wire↔domain 翻译。这使 F1 不是"纯函数挪窝"，而是一次小的**领域建模**（Session 聚合 / transcript 时间线领域服务）。

### 其余（次要）

- **F3｜门面去重**：conversation 5 方法穿 `conversation.Service → Engine → RuntimeServices → Server` **两跳纯转发**；`RuntimeServices` 粒度不一（一半访问器、一半扁平 op）。可在 F1 之后顺手收。
- **F4｜压缩单策略**：`Compactor` 单实现。`Strategy` 接口是对的方向（OpenHands `Condenser` 5 策略印证），但**当前只有一个实现，属 YAGNI**——**等第二个策略（microcompaction / observationMask）真要落地时再抽接口**，不预抽。
- **config 装配**：`config` import `engine`/`lsp` 来构造 `OnlineConfig`/`Pricing`/`ServerSpec`。这是 **wiring 向内**（合法），唯一可选优化是把"装配"挪进组合根、让 `config` 退成纯解析数据——**纯内聚微调，非依赖问题，低优先**。
- **maintenance→engine**：maintenance 实现 engine 的 `Compactor`/`Extractor` **端口**并用其 DTO——**六边形里"驱动适配器→端口"正确**。可选：把 DTO 挪进 maintenance 让它端口无关，但非必须。

---

## 3. DDD / 设计模式：该补、已用、别用

### ✅ 该补

1. **Aggregate Root（DDD 核心，解 F1 根因）** —— 显式把 **Session 建模为聚合根**，Run/Item/消息是其内部，时间线 + rollback/fork 不变量由根（或 session/transcript 领域服务）守护，而非散在 `rpc/server`。
2. **Application Service / 用例环（解 F1）** —— 立薄应用服务层承载非-turn 用例（`RollbackSession`/`ForkSession`/`StartRun`/`ConfigureProvider`…），`rpc/server` 退成"decode→调用用例→present"的纯适配器。可落 `internal/app`，或（更轻）直接把编排下推进对应上下文的领域服务。
3. **架构守护测试（SWE 手段，强烈建议）** —— 一个 `arch_test.go` 用 `go list` 解析 import，断言 **Clean Arch 依赖规则**：`infra` 不得 import `rpc`(delivery)；领域上下文不得 import `engine`/`rpc`；`engine` 不得 import `rpc`。**机器强制依赖规则、防腐化**——这是"如何让整洁架构保持整洁"的答案。lyra 现在靠人肉 review。

### ✅ 已用、别过度形式化

State（turn 机）/ Observer（ToolObserver, per-run hub）/ Plugin·Strategy（`collectExtensions[T]`）/ Facade（Engine）/ Command 表（dispatch）/ ACL（engine 包 agent SDK）/ Repository（各上下文已是）/ Domain Service（maintenance 已是）—— 都在，别动。

### ⛔ 别用（过度工程）

DI 容器（uber/dig，PortAI 教训）/ 通用 event-bus·Mediator / CQRS 形式化 / Saga / Builder 链 / 把 turn 机 formalize 成 State 类层级 / **把实体抽成全局 domain 环**（反 DDD，降内聚）/ 因方法多就拆 `Server`。

---

## 4. 修订后的分批计划（依赖序，每批一 commit、全绿）

| 批 | 内容 | 风险 | 行为变化 | 破坏模块外 API |
|---|---|---|---|---|
| **0｜文档对齐（先做，零代码）** | `ARCHITECTURE.md`/`LAYERING.md` 把"infra 在最底"改写成 Clean Arch 依赖规则 | 极低 | 零 | 无 |
| **1｜F1：用例编排归位（核心）** | 把 rollback/fork/run 时间线领域算法从 `rpc/server` 迁入 Session 聚合 / 领域服务（含 wire↔domain 翻译）；`rpc/server` 退薄 | 中-高 | 零（搬迁） | 无（全 `internal/`，但 `rpc/server` 触面大） |
| **2｜F3：门面去重** | 砍 Engine 对 conversation 的 2 跳转发；`RuntimeServices` 粒度归一（全访问器） | 低-中 | 零 | 无 |
| **3｜架构守护测试** | `arch_test.go` 断言依赖规则 | 极低 | 零（纯加测试） | 无 |
| **—｜F4 压缩 Strategy** | **门控**：等 microcompaction/observationMask 真要做时，与之一起抽 `CompactionStrategy`。现在不做（YAGNI） | — | — | — |
| **—｜config 装配下沉、maintenance DTO** | 可选微调，低优先，可顺带 | 极低 | 零 | 无 |

**节奏**：0 立即（认知对齐）；1 是真正的活，因触及领域建模 + `rpc/server` 大改，**建议先定"应用服务层放哪（`internal/app` vs 下推领域服务）+ 领域 Run 表示怎么建"再动**；2、3 低风险随后；F4 门控待能力侧驱动。

---

## 5. 一句话

lyra 不需要"扳依赖方向"——它已经在沿 Clean Arch 向内依赖。需要的是：**(0) 把文档心智从传统分层改成 Clean Arch 依赖规则**；**(1) 把漏进协议适配器的用例/领域编排（rollback 时间线 = Session 聚合不变量）请回领域**；**(3) 用架构测试机器锁住依赖规则**。实体**别**抽全局环（限界上下文已高内聚）；压缩 Strategy **别**预抽（等第二实现）。

---

## 6. 执行进度（2026-06-13）

带"大刀阔斧、持续向 DDD/Clean Arch 发展"的授权落地：

**已做（全程 build/vet/test 全绿 + 架构守护测试持续通过）：**
- ✅ **架构守护测试** `internal/arch/arch_test.go` —— 机器强制 Clean Arch 依赖规则（依赖向内）。**它通过本身就证明：lyra 的代码早已是向内依赖的（Clean-Arch 正确），初版怀疑的"反向依赖"是 audit 误判**（传统分层视角的错觉）。这是本结构演进的"防腐底座"。
- ✅ **F1** —— rollback/fork 时间线边界（Session 聚合不变量）从 `rpc/server` 适配器请回 `transcript` 领域（wire-free `RunNode`/`BoundaryAt`/sentinels + 单测随迁）。
- ✅ **去 engine 的 conversation 代理** —— 5 方法 `Conversation` 端口 → 1 方法 `SteeringSink`；engine 只保留 turn-lifecycle 的 steering，4 个历史 CRUD 删除，runtime 直连 `conversation.Service`。微内核核心恢复单一职责 + 消除三跳代理。
- ✅ **ubiquitous-language 一致** —— 内部 `MemoryService`/`memory` → `knowledge`（对齐限界上下文）；wire 层 `Memory()`/`memory.*` 有意保留（前端契约术语，边界翻译）。

**已决策不做（避免过度工程，elegant ≠ ceremony）：**
- ❌ **独立 application/use-case 环** —— 单交付运行时下是 1:1 影子层，反降内聚。`rpc/server` 在单交付下兼任 适配器+应用层 是合法的 Clean-Arch 解读。等真出现第二交付（CLI 直连 / remote runtime）或需用例脱离 wire 单测再建。
- ❌ **实体抽全局 `domain` 环** —— `service/<context>` 已是高内聚限界上下文，抽散反而贫血。
- ⏸ **F4 压缩 Strategy** —— 门控到第二个压缩实现（microcompaction）真要落地时再抽。
- ⏸ **config 退叶子** —— lateral，低优先。

**结论**：lyra 结构已是健康的 DDD/Clean-Arch 形态。剩余的是"等触发条件"的项，不是当前的债。继续演进的抓手是 §6 的"已决策不做"里那些**触发条件**——满足了再动。
