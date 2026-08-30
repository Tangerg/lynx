# Agent 框架层综合结论

## 结论先行

Scope 不是“在所有维度都领先”的通用 Agent 框架。它在一个更窄、也更难的目标上做得突出：**让多步、可中断、带子执行的 Agent 工作在进程重启后仍能按明确语义恢复**。为此，它把决策与外部工作拆开，并让宿主持有产品会话、历史和基础设施。

Pi 则代表另一端的优秀答案：**用很小的嵌入面构建高生产力的模型—工具循环**。它的低层 Agent 运行时直接、灵活、易改造，但低层循环本身不提供 Scope 式的副作用身份与完整恢复。Pi 新增的 Harness 类型已经描述了 operation log、lane、replay 和 snapshot 等能力，不过本次基线中主要运行方法仍返回 `HarnessNotImplemented`，不能按已交付能力计入比较。

其余框架并不是两者之间的简单刻度：Eino 以类型化图组合为中心，ADK 以会话、Agent 树和 Workflow 图为中心，Microsoft Agent Framework 以流式 Agent 和工作流执行器为中心，Spring AI 以 Spring 生态中的模型调用组合为中心，Embabel 以目标驱动动态规划为中心，tRPC-Agent-Go 则覆盖 Agent、图和产品化组件的更宽表面。

因此，本轮不产生总排名。正确的问题是：**哪种执行语义适合目标系统，以及为此承担了什么复杂度。**

## 边界先于能力列表

### 应用层不参与框架评分

Flame 的正确角色是 Scope 的外部消费者：

```text
Flame CLI / Desktop
        ↓
Flame runtime
        ↓
Scope framework modules
```

`flame/runtime/go.mod` 直接依赖 Scope 的 `agent`、`core`、`mcp`、`otel`、`skills`、`tools`、`a2a` 和模型叶子模块；Scope 不依赖 Flame。这个方向说明应用与框架已经形成真实分层，比把演示应用放进框架仓库更有说服力。

但 Flame 只能证明 Scope 的复杂执行模型有真实消费者，不能证明每个 Agent 应用都需要同样的执行模型。类似地，Pi Coding Agent 的终端体验、tRPC OpenClaw 的工具数量、ADK 的 Web/CLI 或 Embabel Shell 都不应转化为内核分数。

### GitNexus 不再伪装成同类竞品

GitNexus 的核心是代码索引、知识图谱和检索接口。它没有与其他项目同构的 Agent 执行契约、工具循环或恢复模型。将它放入主矩阵会让“检索产品能力”与“Agent 框架语义”混为一谈。

它仍然有一项相邻价值：提醒 Agent 框架的工具协议不要把“查到的上下文”冒充“已验证事实”。这属于证据表达问题，而不是框架排名。

## 框架形状对比

| 项目 | 核心抽象 | 主要状态所有者 | 外部工作发生位置 | 设计中心 |
| --- | --- | --- | --- | --- |
| Scope | `Definition` + `Execution` | Execution 快照；产品数据由 Host 持有 | `Step` 描述 `Effect`，运行时执行 | 可恢复的受管执行 |
| Pi | `StreamFn`、`agentLoop`、`Agent` | 可变 `AgentState` 和消息历史 | 循环内直接调用模型和工具 | 紧凑的嵌入式工具循环 |
| Eino | `Runnable[I,O]`、Graph/Workflow | 图状态、上下文和节点数据 | 节点或组件直接执行 | 类型化组件与图组合 |
| tRPC-Agent-Go | `Agent`、`Invocation`、Graph | Invocation、Session、Graph State | Agent、Runner 或节点直接执行 | 宽表面的 Agent 运行时 |
| ADK Go | 密封 `Agent`；Workflow `Node`/`Edge` | Session、InvocationContext 与 Workflow RunState | Agent、节点、工具和回调直接执行 | 会话驱动的 Agent 树与图调度 |
| Microsoft AF Go | `RunFunc`、Agent、Executor | 会话、工作流状态和 checkpoint store | Agent 或 Executor 直接执行 | 流式 Agent 与工作流 |
| Spring AI | Model、ChatClient、Advisor | 应用会话、Memory、Spring Bean | Client、Advisor 和 ToolCallback 直接执行 | Spring 生态模型组合 |
| Embabel | Blackboard、Action、AgentProcess | Blackboard 与 AgentProcess | Action 直接执行 | GOAP 动态规划 |

这个表揭示了一个重要修正：Scope 的差异不在于“也有工作流”，而在于 **决策步骤不直接执行外部工作**。大多数对手选择更直接的调用模型，这在简单路径上更轻，在精确恢复上则需要额外约束。

## 八个框架层维度

### 1. 协议与提供商边界

Scope 的 `core.Message`、`Part`、工具和模型契约不依赖特定 SDK，模型实现被拆到叶子模块。这种边界最干净，也把转换成本明确留给适配器。

Pi 的 `pi-ai` 提供统一的多提供商接口和一致的流事件，实际使用非常顺手；但公共类型中保留了 Google thought signature、OpenAI namespace 等提供商字段，包本身也直接依赖多家 SDK。它追求的是兼容效率，不是最纯的领域内核。

Spring AI、ADK、Microsoft Agent Framework 和 tRPC-Agent-Go 都提供统一模型表面，但会不同程度地让框架类型、生态容器或具体 SDK 进入同一依赖闭包。Eino 的接口和提供商扩展仓库分离较好。Embabel 更关注规划模型，协议纯度不是其首要设计中心。

结论不是“越纯越好”：纯内核降低传递依赖和供应商耦合，但会增加适配工作，并面临最低公分母风险。Scope 需要持续证明其公共协议既稳定又能表达提供商特性，而不是只证明它没有 SDK 依赖。

### 2. 最小执行契约

Scope 当前窄腰由五个方法组成：

- `Definition`：`Descriptor`、`Start`、`Restore`
- `Execution`：`Step`、`Snapshot`

旧稿称其为“四方法窄腰”，是事实错误。这个拆分把定义、实例和恢复边界表达得很清楚，但每个实现从第一天就必须面对恢复协议。

Pi 的最低嵌入面更小：注入 `StreamFn` 和工具即可运行 `agentLoop`；状态型 `Agent` 再在上层提供事件和控制。Microsoft Agent Framework 的 `RunFunc`、Eino 的 `Runnable`、Spring 的 `ChatClient` 也都拥有比 Scope 更轻的普通调用路径。

因此，Scope 的窄腰是“语义窄”，不是“上手成本最低”。若调用方只需要一次模型调用或短暂工具循环，Scope 的受管执行不应成为强制入口；当前架构文档允许直接使用 `chatclient`，这是必要的双层路径，不应被重新合并。

### 3. 状态所有权

Scope 对状态的划分最明确：执行器快照属于框架执行语义；产品会话、聊天历史、凭证和存储属于 Host。它降低了框架变成应用容器的风险。

Pi 的低层 `AgentState` 同时持有消息、工具和 `isStreaming`、`streamingMessage`、pending tool calls 等运行时/UI 邻近状态。这让状态订阅和交互式应用非常直接，但持久化边界没有像 Scope 那样天然分层。

ADK 和 tRPC 以 Invocation/Session 上下文集中传递状态，使用方便但容易形成宽上下文。Eino 的图状态和 Embabel 的 Blackboard 对复杂组合很自然，也更依赖运行时约定或动态数据约束。Spring AI 通常把状态所有权交给应用和 Spring 组件。

### 4. 副作用边界

Scope 的 `Step` 只做确定性决策并产出 `Effect`，外部 I/O 由运行时执行；结果通过 Signal 回到下一步。这使副作用拥有可记录身份，也让重试、幂等、取消和恢复能共享一套语义。

Pi、ADK、Eino、Spring AI、Embabel 和多数 tRPC/MAF 路径都在循环、Action、节点、Advisor 或执行器中直接调用模型和工具。直接调用并非坏味道：它减少中间表示和模板代码，调试普通调用也更符合语言直觉。代价是要达到 Scope 同等级别的恢复精度，必须另行记录调用身份、参数、完成结果和重放策略。

Scope 在这里有最清楚的结构优势，也承担最高的作者成本。任何不能带来中断恢复、审计或严格生命周期收益的流程，都不应为了“统一”而强行 Effect 化。

### 5. 编排与子执行生命周期

各框架所谓的“工作流”不是同一种能力：

- Scope 的受管 Workflow 只用于拥有独立 Process 的子执行，子项有标识、预算、取消、快照和树恢复；普通同步数据流可走独立的 `flow` 库。
- Eino 和 tRPC 的 Graph 更接近节点数据流与执行图，表达任意拓扑更自然。
- ADK 除了 Sequential/Parallel/Loop Agent，还实现了独立 Workflow 图：Node/Edge、静态与动态调度、并发上限、schema 校验、HITL 和 resume 通过 Session/RunState 协作；两套工作流表面当前并存。
- Microsoft Agent Framework 的 Executor/Workflow 支持边、事件、checkpoint 和 RequestPort，适合工作流驱动的人机交互。
- Embabel 的 Action 与目标由 GOAP 动态选择，不要求预先固定整张图。
- Spring AI 与 Pi 低层运行时不把通用工作流作为核心；编排通常由应用或上层设施承担。

Scope 的闭合 Stage 词汇换来了稳定的 schema 和恢复语义，也牺牲了任意图组合的自由度。这个取舍适合受管子进程，不应宣传为对所有编排形式的替代。

### 6. 持久化与恢复

| 项目 | 本轮可确认的能力 | 不能混同的部分 |
| --- | --- | --- |
| Scope | Execution/TreeSnapshot、Effect 结果回灌、子执行恢复 | Host 的产品数据不由框架快照代管 |
| Pi | 低层 Agent 保留内存状态；Harness 类型声明 operation log、lane、snapshot、replay | Harness 的 `prompt`、`resume`、`compact`、树导航等主要路径当前仍未实现 |
| Eino | Graph checkpoint 与 state 序列化 | 节点内部任意外部副作用不自动获得 Effect 身份 |
| tRPC | Session/Graph checkpoint、父 checkpoint 等机制 | 宽运行时中的所有 Agent 副作用并非统一可重放 |
| ADK | Workflow RunState 可写入 Session，暂停状态也可由事件历史重建并 resume | 节点内外部调用没有统一 Effect 身份；Workflow 构造处仍明确缺少 graph fingerprint 校验 |
| Microsoft AF | Checkpoint Store、工作流状态、RequestPort | Agent 内直接外部调用仍需自己的幂等边界 |
| Spring AI | Chat memory、外部存储集成 | 没有统一执行快照语义 |
| Embabel | AgentProcess/Blackboard 持久化抽象 | Action 外部工作不自动成为可重放 Effect |

旧稿最大的证据问题之一，是把“有 checkpoint/session 类型”直接等同于“完整可恢复执行”。新口径要求同时回答：保存了什么、外部调用是否有身份、恢复从何处继续、结果是否会被重复提交。

### 7. 扩展与可观测性

Scope 用 middleware、listener 和独立 OTel 模块维持内核中立；这个边界清楚，但需要维护跨执行、Effect 和恢复的关联规则。

Pi 的 Agent 事件和具名生命周期回调很容易嵌入应用，`pi-telemetry` 还定义了类型化、供应商中立的遥测 schema。它避免让 Agent 直接依赖 OTel，却形成了一套需要适配到外部遥测系统的自有协议。

Eino 的 callback、ADK 的 callback/plugin、Spring 的 Advisor、Microsoft AF 的 middleware/context provider 都有各自生态优势。tRPC 同时存在多个回调和扩展面，覆盖面大，但统一心智模型的成本更高。Embabel 借助 Spring 事件与注解，生态一致性高，框架独立性较低。

扩展点数量不是质量指标。更关键的是：同一事件是否只有一个权威生命周期，扩展失败是否改变主执行语义，以及恢复后关联信息能否延续。

### 8. 依赖与应用边界

Scope 的多模块叶子结构给了消费者精细依赖控制，也带来真实成本：版本协调、发现性、跨模块测试和发布管理都更难。旧稿只把模块多当成优势，结论不完整。

Pi 在 monorepo 内明确拆开 `pi-ai`、`pi-agent-core` 和 `pi-coding-agent`，应用层分离是成立的；但 `pi-agent-core` 的导出面又包含 Harness、session、搜索和代码工具等偏 coding-agent 的能力，框架边界正在变宽。这里应评价的是依赖和导出面，而不是否认 Pi 的应用分层。

Eino 的核心与扩展仓库、Spring AI 的 starter/module 体系、MAF/ADK/tRPC 的模块组织各有生态背景。不能用 Go 多模块数量直接跨语言评分，应看最小消费者是否被迫引入不需要的协议、SDK 或产品能力。

## 各框架最适合解决的问题

| 目标问题 | 更自然的候选 | 原因 |
| --- | --- | --- |
| 可中断、可恢复、带子执行的长期任务 | Scope | 副作用、快照和子执行生命周期是同一内核语义 |
| 快速嵌入高质量模型—工具循环 | Pi | 小型 StreamFn/Agent 表面，消息和工具事件直接 |
| 类型化组件与复杂图组合 | Eino | Runnable 和 Graph 是一等抽象 |
| Go 中需要较宽的一站式 Agent 表面 | tRPC-Agent-Go | Agent、Runner、Graph、Session 等覆盖较全 |
| 会话驱动的 Agent 层级、图工作流与 Google 生态 | ADK Go | Invocation/Session、Agent 树和 Workflow scheduler 协作 |
| 流式 Agent 与显式工作流执行器结合 | Microsoft Agent Framework Go | RunFunc、Executor、Checkpoint、RequestPort |
| Spring 应用中的模型调用、Advisor、工具和 RAG | Spring AI | 与 Spring 容器和生态集成自然 |
| 运行时目标驱动的动态任务规划 | Embabel Agent | Blackboard + GOAP Action 模型 |

这不是排他选择。同一产品可以在直接模型调用、短工具循环和受管长期执行之间使用不同层级；关键是不要把所有路径压进最重的抽象。

## Evaluation 是支持层，不是内核加分项

Scope 当前 `evaluation` 根包已经是 `Evaluator[T]` 驱动的通用质量评估内核，RAG/文本词汇位于 `evaluation/retrieval`、`evaluation/text` 等叶子包。它不再是 RAG 专用，但仍明确限定为 `[0,1]`、越高越好的标量质量判断，不是完整实验平台。

tRPC-Agent-Go 的 evaluation 产品面更完整，但直接绑定 Agent Runner、Invocation、EvalSet、trace 和服务；Pi 的 evals 是私有 Coding Agent 行为 harness；Spring AI 的 `EvaluationRequest` 固定 user text、Document 和 response content，反而更偏 Chat/RAG。详细判断见 [Evaluation 支持层对比](EVALUATION.md)。

这个维度不能反向决定 Agent 运行时排名。Scope 应保持 `evaluation` 不依赖 `agent`，由 Flame 或独立实验 harness 组合执行 trace、数据集、制品和 baseline/candidate。

## 对 Scope 的修正后判断

### 已被源码和真实消费者支持的优势

1. **可恢复执行是内核语义，不是存储插件。** `Step`、Effect、Signal、Snapshot 和 Restore 共同决定恢复方式。
2. **宿主边界真实存在。** Flame 通过模块依赖消费 Scope，Scope 没有反向持有应用会话或 UI。
3. **受管编排与普通数据流已经概念分离。** 这避免 Workflow 退化成第二套通用 DAG。
4. **提供商依赖隔离得较彻底。** 消费者能按需选择叶子实现。

### 不能再回避的结构成本

1. **普通路径不够显眼。** Scope 允许直接 `chatclient`，但整体叙事容易让使用者误以为每次调用都应进入受管执行。
2. **Effect 模型提高作者门槛。** 它只有在恢复、审计、取消或幂等收益明确时才值得。
3. **模块数量增加治理成本。** 边界纯度需要与版本、文档、测试矩阵和发现性一起评估。
4. **闭合工作流词汇限制表达自由。** 这是稳定协议的代价，不是无条件优势。
5. **目前主要由 Flame 验证。** 一个强消费者能证明设计并非纸上谈兵，尚不足以证明外部场景的普遍性。

### 下一步最值得验证的不是“再加能力”

- 新调用方能否只依赖少量模块完成一次模型调用或短工具循环。
- 一个自定义 Execution 能否在不理解内部运行时的情况下正确实现快照与恢复。
- 跨进程恢复后，trace、Effect 身份和子执行因果关系是否仍连续。
- 模型提供商的特有能力能否通过扩展保留，而不污染核心协议。
- 多模块版本和兼容性是否有自动化约束，而不是依赖人工同步。

这些验证比继续扩张内置 Agent、工具或应用功能更能证明“框架通用性”。

## 本轮纠正的具体偏差

- 剔除 GitNexus 后，旧集合实际只有 7 个框架（含 Scope）；加入 Pi 后，主比较明确为 8 个框架。
- GitNexus 移出同类矩阵，改为相邻系统证据。
- Flame、Pi Coding Agent、OpenClaw 等应用能力全部从框架评分剥离。
- Scope 核心执行契约从错误的“四方法”修正为五方法拆分。
- Scope 有效状态按当前实现理解为 9 个，不再沿用旧稿互相矛盾的数量。
- 不再把模块数量、内置工具数量或仓库规模直接当作成熟度。
- 不再把接口声明、checkpoint 类型或 session 记录自动视为完整恢复能力。
- 删除总分和“全面领先”式结论，改为设计中心、适用场景和结构成本。
