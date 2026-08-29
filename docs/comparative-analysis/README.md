# 同类项目对比分析（2026-08-29）

对桌面上 7 个参照项目与 `scope` 当前仓库的深度对比。每个参照项目**处在不同维度**，因此不做"谁更好"的排名，而是逐个定位它在哪一维度上代表了一种成熟答案，再回答两个问题：

1. **同构（convergent）**：它独立演化出的哪些取舍与 scope 一致 —— 一致越多，说明该取舍越可能是被问题域本身逼出来的，而不是个人趣味。
2. **分歧（divergent）**：它做了 scope 明确禁止的事，或 scope 缺了它已经证明有价值的能力 —— 前者是反向不变量的外部证据，后者是待补的真实缺口。

## 文档索引

| 文档 | 项目 | 语言 | 代表的维度 |
|---|---|---|---|
| [`eino.md`](eino.md) | cloudwego/eino | Go | **类型化编排图** —— 编译期连线的 Graph/Chain + 四流式范式 |
| [`trpc-agent-go.md`](trpc-agent-go.md) | trpc-group/trpc-agent-go | Go | **电池全包的一体化 runtime** —— 从 Agent 到成品应用 |
| [`adk-go.md`](adk-go.md) | google/adk-go | Go | **多语言一致的 Agent 语义** —— 跨 5 种语言实现同一概念模型 |
| [`agent-framework-go.md`](agent-framework-go.md) | microsoft/agent-framework-go | Go | **企业级工作流内核** —— 类型化 Executor 图 + checkpoint + HITL port |
| [`gitnexus.md`](gitnexus.md) | GitNexus (Akon Labs) | TypeScript | **Agent 上下文供给侧** —— 代码知识图谱 + MCP 工具设计 |
| [`spring-ai.md`](spring-ai.md) | spring-projects/spring-ai | Java | **AI 集成层的工业标准** —— 统一 Model 协议 + Advisor + Modular RAG |
| [`embabel-agent.md`](embabel-agent.md) | embabel/embabel-agent | Kotlin | **目标导向规划（GOAP）** —— scope agent 的直系祖先 |
| [`SYNTHESIS.md`](SYNTHESIS.md) | 全部 + scope | — | **总对比** —— 七维矩阵、同构证据、真实缺口、判词 |

## 分析维度框架

所有文档统一按这 8 个维度切：

| # | 维度 | 关键问题 |
|---|---|---|
| D1 | **协议层（Message/Model）** | 消息与模型契约是中立的，还是绑定某一家 wire？多模态与 Reasoning 怎么建模？ |
| D2 | **执行内核（Kernel）** | 谁拥有主循环？状态归谁？一个"步"是什么？纯不纯？ |
| D3 | **编排模型（Orchestration）** | 图 / 链 / 状态机 / 规划器 —— 拓扑是编译期还是运行期确定？ |
| D4 | **持久化与恢复（Snapshot）** | 快照边界在哪？序列化用什么？框架和宿主的持久化责任怎么划？ |
| D5 | **扩展机制（Extension）** | 一个同质机制，还是一堆具名插槽（callback / advisor / plugin / hook）？ |
| D6 | **模块与依赖边界** | module 是发布边界还是目录装饰？依赖方向可机器验证吗？ |
| D7 | **可观测性** | 自造抽象还是直接用 OTel？埋点归属哪一层？ |
| D8 | **工程治理** | 破坏性变更政策、门禁、文档所有权、历史债务处理方式 |

## 方法论声明

- 所有结论来自**读源码**，不引用项目宣传材料的自述能力。行数、文件数、依赖、Go 版本均为实测。
- 对参照项目的批评只针对**设计取舍在 scope 语境下的适配度**，不是对该项目本身的质量判断 —— 它们的取舍往往由各自的约束（多语言对齐、企业兼容承诺、生态锁定）决定，那些约束 scope 没有。
- scope 处于 pre-1.0、无外部兼容包袱，这是最大的不对称：几乎所有参照项目的"债"都是**兼容承诺的代价**，不是能力不足。
