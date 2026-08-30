# 源码证据索引

本页列出综合结论的主要源码落点。提交基线见 [README](README.md#证据基线)。路径均相对于对应桌面仓库。

## Scope 与 Flame

| 结论 | 证据 |
| --- | --- |
| 五方法拆分窄腰 | `scope/agent/definition.go`：`Definition`、`Execution` |
| 九个有效运行状态 | `scope/agent/status.go`：`Status.Valid` |
| Step 不做 I/O，外部工作由 Effect 表达 | `scope/agent/definition.go`、`scope/agent/effect.go` |
| 直接调用、普通 flow、受管 workflow 是不同层级 | `scope/agent/doc/ARCHITECTURE.md` |
| 通用 evaluation 根内核 | `scope/evaluation/evaluator.go`、`run.go`、`report.go`、`composite.go` |
| RAG/文本指标已离开根内核 | `scope/evaluation/text/`、`scope/evaluation/retrieval/` |
| Flame 单向消费 Scope | `flame/runtime/go.mod` |

## Pi

| 结论 | 证据 |
| --- | --- |
| 统一多提供商消息与流协议 | `pi/packages/ai/src/types.ts` |
| provider registry 与动态模型目录 | `pi/packages/ai/src/models.ts` |
| 注入式 StreamFn、工具和生命周期配置 | `pi/packages/agent/src/types.ts` |
| 模型—工具循环直接执行 | `pi/packages/agent/src/agent-loop.ts` |
| 状态型 Agent 与事件 | `pi/packages/agent/src/agent.ts` |
| Harness 类型、未实现主路径 | `pi/packages/agent/src/harness/agent-harness.ts` |
| operation log、replay 与 session 存储协议 | `pi/packages/agent/src/harness/session/types.ts` |
| evals 是私有 Coding Agent harness | `pi/packages/evals/package.json`、`README.md` |

## Eino

| 结论 | 证据 |
| --- | --- |
| Runnable 四种调用形态 | `eino/compose/runnable.go` |
| Graph/Workflow 编排 | `eino/compose/graph.go`、`compose/workflow.go` |
| 图 checkpoint | `eino/compose/checkpoint.go` |
| 共同消息协议 | `eino/schema/message.go` |
| Agent callback | `eino/adk/callback.go`、`flow/agent/react/callback.go` |

## tRPC-Agent-Go

| 结论 | 证据 |
| --- | --- |
| Invocation 与 Agent 契约 | `trpc-agent-go/agent/invocation.go`、`agent/agent.go` |
| Runner | `trpc-agent-go/runner/runner.go` |
| Graph state 与 checkpoint | `trpc-agent-go/graph/state.go`、`graph/checkpoint.go` |
| 多条 callback 表面 | `agent/callbacks.go`、`model/callbacks.go`、`tool/callbacks.go`、`graph/callbacks.go` |
| Agent 绑定的完整 evaluation 服务 | `trpc-agent-go/evaluation/evaluation.go`、`evaluation/service/` |

## Google ADK Go

| 结论 | 证据 |
| --- | --- |
| Agent 契约 | `adk-go/agent/agent.go` |
| InvocationContext | `adk-go/internal/context/invocation_context.go` |
| Session 与事件 | `adk-go/session/session.go` |
| Runner | `adk-go/runner/runner.go` |
| 工作流 Agent | `adk-go/agent/workflowagents/sequentialagent/agent.go`、`parallelagent/agent.go`、`loopagent/agent.go` |
| Workflow Node/Edge 与 scheduler | `adk-go/workflow/workflow.go`、`workflow/scheduler.go` |
| RunState、持久化与恢复 | `adk-go/workflow/state.go`、`workflow/persistence.go`、`workflow/resume.go` |
| HITL 请求协议 | `adk-go/workflow/request_input.go` |
| Plugin 生命周期 | `adk-go/plugin/plugin.go` |
| Eval API 尚未实现 | `adk-go/server/adkrest/internal/routers/eval.go` |

## Microsoft Agent Framework Go

| 结论 | 证据 |
| --- | --- |
| RunFunc 与 Agent | `agent-framework-go/agent/agent.go` |
| middleware/context provider | `agent-framework-go/agent/middleware.go`、`agent/context.go` |
| Workflow Executor | `agent-framework-go/workflow/executor.go` |
| RequestPort | `agent-framework-go/workflow/requestport.go` |
| PortableValue 与 checkpoint | `agent-framework-go/workflow/portable.go`、`workflow/checkpoint/store.go`、`workflow/checkpoint/manager.go` |
| loop evaluator 是重入策略 | `agent-framework-go/agent/harness/loop/loop.go` |

## Spring AI

| 结论 | 证据 |
| --- | --- |
| Model/StreamingModel/ChatModel | `spring-ai/spring-ai-model/src/main/java/org/springframework/ai/model/Model.java`、`StreamingModel.java`、`chat/model/ChatModel.java` |
| ChatClient | `spring-ai/spring-ai-client-chat/src/main/java/org/springframework/ai/chat/client/ChatClient.java` |
| Advisor 与工具循环 | `spring-ai/spring-ai-client-chat/src/main/java/org/springframework/ai/chat/client/advisor/api/Advisor.java`、`advisor/ToolCallingAdvisor.java` |
| VectorStore | `spring-ai/spring-ai-vector-store/src/main/java/org/springframework/ai/vectorstore/VectorStore.java` |
| Chat/RAG 形状的 EvaluationRequest | `spring-ai/spring-ai-commons/src/main/java/org/springframework/ai/evaluation/EvaluationRequest.java` |

## Embabel Agent

| 结论 | 证据 |
| --- | --- |
| Action | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/Action.kt` |
| Blackboard | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/Blackboard.kt` |
| AgentProcess/AgentPlatform | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/AgentProcess.kt`、`AgentPlatform.kt` |
| GOAP | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/plan/goap/OptimizingGoapPlanner.kt`、`goap/astar/AStarGoapPlanner.kt` |

## GitNexus

| 结论 | 证据 |
| --- | --- |
| 代码图协议 | `GitNexus/gitnexus-shared/src/graph/types.ts` |
| 索引与查询实现 | `GitNexus/gitnexus/src/core/`、`gitnexus/src/storage/` |
| MCP 工具边界 | `GitNexus/gitnexus/src/mcp/tools.ts`、`mcp/server.ts` |
| 输出预算与陈旧性 | `GitNexus/gitnexus/src/mcp/output-budget.ts`、`mcp/staleness.ts` |

## 解释规则

源码中出现一个类型，只能证明该协议被声明。只有主执行路径调用它、测试覆盖其语义且实现不返回占位错误时，才按“已实现能力”计入综合结论。Pi Harness 是本轮最重要的反例：类型设计丰富，但主运行方法仍明确返回未实现错误。
