# Agent 框架层对比

本文档集比较的是 **Agent 框架**，不是用框架构建出来的应用。

这一区分会直接改变结论：内置终端、代码编辑器、搜索工具、Web UI、CLI 和现成 Agent 数量，能说明应用或发行包的完成度，却不能直接证明框架内核更通用。相反，协议抽象、执行契约、状态所有权、外部副作用、恢复语义和依赖方向，才是本轮的主比较对象。

## 比较对象

### 主比较组

| 项目 | 本轮识别出的框架中心 |
| --- | --- |
| Scope | 可恢复的受管执行、显式副作用、宿主与框架分离 |
| Pi | 紧凑的模型—工具循环、统一多提供商接口、可嵌入 Agent 运行时 |
| Eino | 类型化 Runnable 与图编排 |
| tRPC-Agent-Go | Agent 运行时、图执行和产品化组件的组合框架 |
| Google ADK Go | 会话驱动的 Agent、Workflow 图调度与回调体系 |
| Microsoft Agent Framework Go | 流式 Agent 调用、工作流执行器和人工请求端口 |
| Spring AI | Spring 风格的模型、ChatClient、Advisor、工具调用和 RAG 组合 |
| Embabel Agent | Blackboard、Action 和 GOAP 动态规划 |

### 不进入主评分的对象

- **GitNexus** 是代码知识图谱和检索系统，不是 Agent 框架。它只作为“工具结果如何表达事实、证据和不确定性”的相邻系统证据。
- **Flame** 是 Scope 的真实应用层消费者，用来验证依赖方向和框架边界，不与框架横向评分。
- `pi-coding-agent`、tRPC OpenClaw、ADK CLI/Web、Embabel Shell 等同样属于应用或产品层，不折算成框架内核优势。

## 统一比较口径

本轮使用八个框架层维度：

1. **协议与提供商边界**：领域协议是否被特定 SDK、传输协议或产品概念污染。
2. **最小执行契约**：框架要求实现的核心接口有多宽，是否同时承担过多职责。
3. **状态所有权**：消息、会话、执行状态、编排状态分别由谁持有和持久化。
4. **副作用边界**：模型调用、工具调用和 I/O 是直接发生，还是先被描述、再由运行时执行。
5. **编排与子执行生命周期**：图、工作流、动态规划是否拥有独立标识、预算、取消和恢复语义。
6. **持久化与恢复**：区分“会话记录”“图 checkpoint”“完整执行快照”和“声明了但尚未实现”。
7. **扩展与可观测性**：中间件、回调、事件和遥测是否形成单一、可组合的扩展面。
8. **依赖与应用边界**：框架是否可被独立应用消费，应用能力是否反向渗入内核。

不再给出总分。不同框架有不同设计中心，把所有取舍压成单一排名会重新制造偏差。

## 证据规则

- 以本地仓库源码和模块清单为准，不以首页功能列表代替实现。
- 明确区分 **已实现**、**接口已声明** 和 **应用层提供**。
- 对其他框架使用其自身设计目标解释取舍，不把 Scope 的架构当成唯一正确答案。
- 结论必须能落到具体接口、类型、依赖或执行路径。
- 规模只用于解释边界成本，不用于判断设计优劣。

## 证据基线

分析日期：2026-08-30。

| 仓库 | 本地提交 |
| --- | --- |
| Scope | `3d0b2ba51c3ba09915b56aa1ebcacd0c7eb749fc` |
| Pi | `853a80d26c90a14c1886f0ebb8ffaae133ca2185` |
| Eino | `0e01b2a4e3050c4027bd61f2c2e2a519aa1e237c` |
| tRPC-Agent-Go | `91bde85eb243333b2b33fe89061f2218ede00c99` |
| Google ADK Go | `0da17d5183cc7affd4bdb7b4075f9e264bb598be` |
| Microsoft Agent Framework Go | `6aabdc7ea2d2af7ac5f673dd693a01d5e61bed35` |
| Spring AI | `e5e277fd08a017c5a3efc57aef377d2a067e91dd` |
| Embabel Agent | `6988f286544bb792bed35d8ae45812c446be082d` |
| GitNexus | `b059ab3541ea68c2ce292955fc367a5de04b39ea` |
| Flame | `a2e937181e111ca1c4e29d492605ee3838929002` |

Flame 当前工作区有大量未提交改动，因此这里只采用稳定的模块依赖方向作为证据，不引用其易变实现细节。

## 文档索引

- [综合结论](SYNTHESIS.md)
- [Evaluation 支持层对比](EVALUATION.md)
- [源码证据索引](EVIDENCE.md)
- [Pi](pi.md)
- [Eino](eino.md)
- [tRPC-Agent-Go](trpc-agent-go.md)
- [Google ADK Go](adk-go.md)
- [Microsoft Agent Framework Go](agent-framework-go.md)
- [Spring AI](spring-ai.md)
- [Embabel Agent](embabel-agent.md)
- [GitNexus：相邻系统证据](gitnexus.md)

## 阅读提示

主文档先回答“这些框架实际上分别在优化什么”，再讨论 Scope 的真实优势与结构成本。各项目文档保留可复核的源码落点，并列出适合借鉴和不宜照搬的部分。
