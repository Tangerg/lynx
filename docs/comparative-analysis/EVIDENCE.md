# Source evidence index

This page lists the primary source locations behind the synthesis. Commit
baselines are in the [README](README.md#evidence-baseline). Paths are relative
to each project's own repository.

## Scope and Flame

| Conclusion | Evidence |
| --- | --- |
| A five-method waist | `scope/agent/definition.go`: `Definition`, `Execution` |
| Nine valid run states | `scope/agent/status.go`: `Status.Valid` |
| A Step performs no I/O; external work is expressed as an Effect | `scope/agent/definition.go`, `scope/agent/effect.go` |
| Direct calls, ordinary flow, and managed workflow are different layers | `scope/agent/doc.go`, `scope/agent/definition.go` |
| A general eval root kernel | `scope/eval/evaluator.go`, `experiment.go`, `report.go`, `composite.go` |
| Domain metrics have left the root kernel | `scope/eval/text/`, `scope/eval/judge/`, `scope/eval/ranking/` |
| Flame consumes Scope one-way | `flame/runtime/go.mod` |

## Pi

| Conclusion | Evidence |
| --- | --- |
| A unified multi-provider message and stream protocol | `pi/packages/ai/src/types.ts` |
| A provider registry and dynamic model catalog | `pi/packages/ai/src/models.ts` |
| Injected StreamFn, tools, and lifecycle configuration | `pi/packages/agent/src/types.ts` |
| The model-tool loop executes directly | `pi/packages/agent/src/agent-loop.ts` |
| A stateful agent and its events | `pi/packages/agent/src/agent.ts` |
| Harness types with an unimplemented main path | `pi/packages/agent/src/harness/agent-harness.ts` |
| Operation log, replay, and session store protocol | `pi/packages/agent/src/harness/session/types.ts` |
| evals is a private coding-agent harness | `pi/packages/evals/package.json`, `README.md` |

## Eino

| Conclusion | Evidence |
| --- | --- |
| Runnable's four calling shapes | `eino/compose/runnable.go` |
| Graph and Workflow orchestration | `eino/compose/graph.go`, `compose/workflow.go` |
| Graph checkpoints | `eino/compose/checkpoint.go` |
| A shared message protocol | `eino/schema/message.go` |
| Agent callbacks | `eino/adk/callback.go`, `flow/agent/react/callback.go` |

## tRPC-Agent-Go

| Conclusion | Evidence |
| --- | --- |
| The Invocation and Agent contracts | `trpc-agent-go/agent/invocation.go`, `agent/agent.go` |
| Runner | `trpc-agent-go/runner/runner.go` |
| Graph state and checkpoints | `trpc-agent-go/graph/state.go`, `graph/checkpoint.go` |
| Several callback surfaces | `agent/callbacks.go`, `model/callbacks.go`, `tool/callbacks.go`, `graph/callbacks.go` |
| A complete agent-bound evaluation service | `trpc-agent-go/evaluation/evaluation.go`, `evaluation/service/` |

## Google ADK Go

| Conclusion | Evidence |
| --- | --- |
| The Agent contract | `adk-go/agent/agent.go` |
| InvocationContext | `adk-go/internal/context/invocation_context.go` |
| Sessions and events | `adk-go/session/session.go` |
| Runner | `adk-go/runner/runner.go` |
| Workflow agents | `adk-go/agent/workflowagents/sequentialagent/agent.go`, `parallelagent/agent.go`, `loopagent/agent.go` |
| Workflow Node, Edge, and scheduler | `adk-go/workflow/workflow.go`, `workflow/scheduler.go` |
| RunState, persistence, and recovery | `adk-go/workflow/state.go`, `workflow/persistence.go`, `workflow/resume.go` |
| The HITL request protocol | `adk-go/workflow/request_input.go` |
| Plugin lifecycle | `adk-go/plugin/plugin.go` |
| The eval API is not yet implemented | `adk-go/server/adkrest/internal/routers/eval.go` |

## Microsoft Agent Framework Go

| Conclusion | Evidence |
| --- | --- |
| RunFunc and Agent | `agent-framework-go/agent/agent.go` |
| Middleware and context provider | `agent-framework-go/agent/middleware.go`, `agent/context.go` |
| Workflow Executor | `agent-framework-go/workflow/executor.go` |
| RequestPort | `agent-framework-go/workflow/requestport.go` |
| PortableValue and checkpoints | `agent-framework-go/workflow/portable.go`, `workflow/checkpoint/store.go`, `workflow/checkpoint/manager.go` |
| The loop evaluator is a re-entry strategy | `agent-framework-go/agent/harness/loop/loop.go` |

## Spring AI

| Conclusion | Evidence |
| --- | --- |
| Model, StreamingModel, ChatModel | `spring-ai/spring-ai-model/src/main/java/org/springframework/ai/model/Model.java`, `StreamingModel.java`, `chat/model/ChatModel.java` |
| ChatClient | `spring-ai/spring-ai-client-chat/src/main/java/org/springframework/ai/chat/client/ChatClient.java` |
| Advisors and the tool loop | `spring-ai/spring-ai-client-chat/src/main/java/org/springframework/ai/chat/client/advisor/api/Advisor.java`, `advisor/ToolCallingAdvisor.java` |
| VectorStore | `spring-ai/spring-ai-vector-store/src/main/java/org/springframework/ai/vectorstore/VectorStore.java` |
| A chat- and RAG-shaped EvaluationRequest | `spring-ai/spring-ai-commons/src/main/java/org/springframework/ai/evaluation/EvaluationRequest.java` |

## Embabel Agent

| Conclusion | Evidence |
| --- | --- |
| Action | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/Action.kt` |
| Blackboard | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/Blackboard.kt` |
| AgentProcess and AgentPlatform | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/agent/core/AgentProcess.kt`, `AgentPlatform.kt` |
| GOAP | `embabel-agent/embabel-agent-api/src/main/kotlin/com/embabel/plan/goap/OptimizingGoapPlanner.kt`, `goap/astar/AStarGoapPlanner.kt` |

## GitNexus

| Conclusion | Evidence |
| --- | --- |
| The code graph protocol | `GitNexus/gitnexus-shared/src/graph/types.ts` |
| Index and query implementation | `GitNexus/gitnexus/src/core/`, `gitnexus/src/storage/` |
| The MCP tool boundary | `GitNexus/gitnexus/src/mcp/tools.ts`, `mcp/server.ts` |
| Output budget and staleness | `GitNexus/gitnexus/src/mcp/output-budget.ts`, `mcp/staleness.ts` |

## Interpretation rule

A type appearing in source proves only that a protocol was declared. It counts
as a delivered capability in the synthesis only when the main execution path
calls it, tests cover its semantics, and the implementation does not return a
placeholder error. Pi's harness is this round's most important counterexample:
a rich type design whose main run methods still return an explicit
not-implemented error.
