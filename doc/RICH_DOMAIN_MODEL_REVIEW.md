# Core / Agent / Lyra 充血模型审查

> 范围：`core/`、`agent/`、`lyra/`
>
> 关注点：领域行为是否跟随领域对象，私有顶层函数是否在稀释模型内聚。

## 结论

整体上，当前代码已经不是典型贫血模型。`core` 和 `agent/core` 的核心对象行为比较充分：`Agent.Validate`、`ProcessContext.Chat/ChatWithActionTools/ResolveTools`、`typedAction.Execute`、`ActionMetadata.computePreconditionsAndEffects`、`Request/Response/Message`、`MiddlewareManager` 等都把规则放在拥有状态或表达领域概念的类型上。

但也还没有完全达到“充血模型足够稳定”的程度。风险主要集中在运行时编排和协议适配层：`agent/runtime`、`lyra/internal/engine`、`lyra/rpc/server`、`core/model/chat/middleware/tool` 仍有不少顶层私有函数在承载同一条业务流程的分支规则。它们不是全都应该改成方法；其中很多是合理的纯函数、构造函数、wire mapping。但有几处已经接近“状态散落在函数参数里”的味道，后续应优先收敛。

## 判定标准

本审查不把“顶层私有函数”本身视为坏味道。Go 里包内纯函数是正常手段，强行方法化会制造伪对象。

建议方法化或对象化的信号：

- 函数反复传入同一组状态参数，并按同一条生命周期推进。
- 函数名描述的是领域行为，而不是通用转换，例如 `resumeAndDrive`、`expandNeighbors`。
- 函数需要维护不变量，例如“run.finished 必须最后发出”、“工具调用 resume 不能重新调用模型”。
- 函数之间存在强顺序耦合，单独调用容易破坏状态机。
- 注释需要解释大量上下文，说明行为已经不是简单辅助。

可以继续保留为顶层函数的信号：

- 纯转换、wire mapping、JSON best-effort 解析。
- 小型谓词或格式化函数。
- 构造函数、默认值补全。
- 不拥有领域状态，也不改变生命周期。
- 放成方法不会减少参数或增强不变量。

## 函数分布概览

排除 `_test.go` 后，顶层私有函数最密集的包如下：

| 包 | 顶层私有函数 | 方法 | 判断 |
|----|-------------:|-----:|------|
| `agent/runtime` | 44 | 173 | 方法很多，但运行时规则仍有明显顶层编排函数 |
| `lyra/rpc/server` | 36 | 82 | `translator` 已较充血，其他 run/resume/wire mapping 较分散 |
| `lyra/internal/engine` | 26 | 43 | `Engine` 是入口，但 compaction/HITL/tool/prompt 规则分在 worker 和函数间 |
| `core/model/chat/middleware/tool` | 17 | 36 | 中间件对象完整，但 resume/tool-round 规则还有函数式状态机痕迹 |
| `lyra/internal/service/chat` | 16 | 49 | `turnState` 较好，终止计划和分类函数可进一步对象化 |
| `core/model/chat` | 14 | 130 | 基本健康，多为消息列表、tracing、JSON 辅助 |
| `agent/core` | 6 | 106 | 健康，核心实体行为足够集中 |

这个分布说明：问题不在核心领域类型缺方法，而在“跨对象流程”尚未全部沉淀成更明确的运行时对象。

## Core

`core` 的模型总体比较充血，尤其是 chat、document、vectorstore/filter 这几块。

健康点：

- `core/model/chat` 把消息、请求、响应、元数据、工具定义放在对象方法上，例如 `Request.AppendToLastUserMessage`、`Options.Clone`、`AssistantMessage.CollectToolCalls`、`Response.TextDelta`。
- `core/model/chat/middleware/tool` 已经有 `middleware`、`support`、`registry`、`callInvoker`、`invocationResult` 等内部对象，工具循环不是完全靠散函数推进。
- `core/vectorstore/filter` 的 AST、token、lexer、parser、visitor 有清晰对象边界，builder/visitor 方法化充分。
- `core/document` 的 reader/writer/transformer/batcher 都是接口 + 实现对象，顶层私有函数很少。

需要注意：

- `core/model/chat/middleware/tool/resume.go` 里的 `trailingPendingToolCalls`、`mergeRoundReturns`、`priorModelRounds` 是同一套“tool round resume”规则。它们目前是纯函数，可以接受；但它们共同维护一个重要不变量：HITL resume 只能执行 pending tool calls，不能重放模型。如果后续 resume 逻辑继续增长，应抽成 `toolRound` / `resumePoint` 之类的小对象。
- `core/model/chat/middleware/tool/middleware.go` 的 `executeCallRecursively` 和 `executeStreamRecursively` 共享大量状态机规则，只是同步/流式输出不同。现在靠注释和测试维持一致，后续可以考虑抽出 `loopRunner`，让 call/stream 只负责输出策略。
- `core/model/chat/message_list.go` 的消息列表操作是合理纯函数，不建议强行挂到 `[]Message` 包装类型上，除非未来形成稳定的 `MessageList` 领域对象。

判断：`core` 基本达标。不要为了减少私有函数而方法化。优先关注工具循环的 resume 状态对象，而不是 chat 基础模型。

## Agent

`agent/core` 是最接近充血模型的部分。`Agent`、`ProcessContext`、`ActionMetadata`、`typedAction`、`Condition`、`Goal`、`Session`、`ToolGroup` 都有明确行为。

健康点：

- `Agent.Validate` 把结构性规则留在 `Agent` 上。
- `ProcessContext` 是 action 的能力边界，聊天、工具解析、事件发布、session params、tool-call cancel 都通过方法暴露。
- `typedAction.Execute` 拥有 typed input loading、执行、await、blackboard output 写入的完整行为。
- `ActionMetadata.computePreconditionsAndEffects` 把 action 的 planner 语义留在 metadata 上。
- `Condition` 的组合对象 `andCondition/orCondition/notCondition` 行为清晰，`Evaluate` 负责短路。

轻微问题：

- `validateUniqueNamed` 是 `Agent.Validate` 的抽取函数。它现在只是去重循环，保留可以；如果未来 Agent 校验继续增加，可以抽成私有 `agentValidator`，但当前没必要。
- `conditionName/conditionCost/evaluateCondition` 是 nil-safe 组合辅助。它们服务于组合条件，不需要挂到 `Condition` 接口上。

真正的热点在 `agent/runtime`：

- `Platform.createProcess` 已经较好，把建进程流程挂在 `Platform` 上。
- 但 `validateProcessExtensions`、`bindBlackboardSeed`、`inheritEventListeners`、`findPlannerByName` 都在维护 process construction 的规则。它们目前参数简单，可读性还行；如果 process 创建继续复杂化，建议抽 `processFactory` 或 `processBuilder`，把这些作为方法集中起来。
- `runActionMiddleware`、`runToolDecorators`、`runAgentValidators`、`runGoalApprovers`、`runToolGroupResolvers` 都围绕 extension dispatch。现在它们是纯函数，但领域上属于 `extensionRegistry` 或一个 `extensionDispatcher`。这里比 `agent/core` 更像贫血边缘，因为 `extensionRegistry` 只有注册和收集，执行规则散在顶层函数里。
- GOAP planner 的 `candidateActions`、`expandNeighbors`、`heuristic`、`reconstructPath`、`goalReachable`、`backwardOptimize`、`forwardOptimize` 都服务于一次 A* search。`Planner.searchForGoal` 已经是方法，但搜索过程的内部状态仍通过参数传递。这里适合引入私有 `search` 对象，持有 `start/actions/goal/gScores/cameFrom/open/closed`，让 `expandNeighbors`、`reconstructPath`、`goalReachable` 成为方法。

判断：`agent/core` 达标；`agent/runtime` 局部偏过程式；`agent/planning/planner/goap` 可进一步充血化。

## Lyra

`lyra` 是应用和协议层，本来就会比 `core/agent` 更偏编排。这里不能简单要求所有函数都挂到领域实体上，因为很多代码是 RPC 适配、wire DTO 转换、CLI glue。

健康点：

- `lyra/internal/service/chat` 的 `turnState` 很好：它拥有 parked/proc/steering 的并发不变量，`park/claimPark/appendSteering/drainSteering/process` 都是方法。
- `inMemory` service 拥有 `StartTurn/Events/Cancel/Resume/resumeAndDrive/runTurn/drive/endTurn`，turn 生命周期没有完全散掉。
- `lyra/rpc/server/translator` 是明显充血的协议状态机：`open/translate/interrupt/appendText/closeText/toolStart/toolEnd/turnEnd/outcome/drainTools` 都在 `translator` 上，状态和行为匹配。
- `lyra/internal/engine/compactor` 和 `extractor` 已经是 worker 对象，`maybeCompact/summarize` 不是裸函数。

需要注意：

- `lyra/rpc/server/translator.go` 后半段的 `toolKind/toolInvocation/fillCommandResult/parseLocalSearchHits/parseWebSearchHits/classifyRunError` 是 wire mapping。它们可以保留为顶层函数；如果继续增长，建议抽成 `toolInvocationMapper` 和 `runErrorClassifier`，而不是塞进 `translator`。这些函数不读写 translator 状态，挂到 `translator` 上会降低语义清晰度。
- `lyra/rpc/server/runs.go` 的 `resumeBindingFrom/questionFromPayload/resolveResolution/lastUserText/durableFor` 目前是协议转换函数。`resumeBindingFrom` 和 `questionFromPayload` 强绑定 interrupt persistence + continuation translator，可考虑抽 `resumeBinder`；`resolveResolution` 可考虑成为 `protocol.InterruptResponseSet` 的解析器，但 protocol DTO 当前没有行为，保留在 server 层也合理。
- `lyra/internal/service/chat/turn.go` 的 `terminalPlan/completedPlan/fallbackPlan/interruptKind` 是 turn terminal decision 的规则。它们现在是顶层函数，已经比普通 helper 更像领域策略。可以抽成 `turnEndPlanner` 或让 `turnEndPlan` 增加构造方法，例如 `planTurnEnd(terminal, out, runErr, ctxErr, status)`。
- `lyra/internal/engine/hitl.go` 的 `saveInflightTail/loadInflightTail/clearInflightTail/marshalMessages/unmarshalMessages` 操作同一个黑板 key 和同一套序列化格式。建议抽成 `inflightTailStore`，持有 blackboard 或 key，减少“约定字符串 + 序列化规则”散落。
- `lyra/internal/engine/tools.go`、`prompt.go`、`config.go` 里有若干构建函数，它们偏 composition root，保留顶层函数通常比对象化更清楚。

判断：`lyra` 不是贫血，但协议/应用层的流程对象还可以更明确。优先改 HITL tail、turn end planning、resume binding，而不是 translator 的纯 mapping。

## 优先级建议

### P0：不建议改

这些私有函数当前是合理的 Go 写法，强行方法化收益低：

- `core/model/chat` 的消息列表、JSON、tracing 辅助。
- `agent/core` 的 `conditionName/conditionCost/evaluateCondition/resolveBindingName`。
- `lyra/rpc/server/translator.go` 中不读写 `translator` 状态的 wire mapping。
- `lyra/rpc/transport/http` 的 auth/cors/status/error/SSE 小函数。
- `lyra/cmd/lyra` 的 CLI parsing 和 printing helper。

### P1：建议近期收敛

1. `agent/planning/planner/goap` 引入私有 `search` 对象。

   目标：把 `expandNeighbors`、`reconstructPath`、`goalReachable`、`heuristic`、优化 pass 中依赖同一 search 上下文的函数收进对象。这样 A* 的不变量不再靠长参数列表维持。

2. `core/model/chat/middleware/tool` 引入 `resumePoint` / `toolRound`。

   目标：把 `trailingPendingToolCalls`、`mergeRoundReturns`、`priorModelRounds`、`buildInterruptResponse` 合成一个表达“当前 round 是否可恢复、哪些 done、哪些 pending、如何继续”的小对象。

3. `lyra/internal/engine/hitl.go` 引入 `inflightTailStore`。

   目标：把 `inflightTailKey`、marshal/unmarshal、save/load/clear 聚合，避免黑板持久化格式成为散落约定。

4. `lyra/internal/service/chat/turn.go` 收敛 turn terminal decision。

   目标：把 `terminalPlan/completedPlan/fallbackPlan` 聚成一个策略对象或 `turnEndPlan` 构造函数，明确“根据 terminal event + engine output + errors 生成 TurnEnd/Event/metrics”的领域规则。

### P2：随复杂度增长再改

- `agent/runtime` 的 extension dispatch 可以抽 `extensionDispatcher`，但当前函数短、规则明确，除非新增更多 extension 类型。
- `agent/runtime` 的 process construction 可以抽 `processFactory`，但目前 `Platform.createProcess` 已经承担聚合根职责。
- `lyra/rpc/server` 的 resume binding 可以抽 `resumeBinder`，前提是 interrupt 类型继续增加。
- `translator` 的 tool mapping 可抽 mapper，前提是工具种类和 wire variant 继续扩展。

## 推荐重构方向

### GOAP search

当前形态：

```go
bestGoalNode, cameFrom, iterations, err := p.searchForGoal(ctx, start, candidates, goal, cap)
path := reconstructPath(cameFrom, bestGoalNode.state.HashKey(), start.HashKey())
path = backwardOptimize(path, start, goal)
path = forwardOptimize(path, start)
```

建议形态：

```go
search := newSearch(start, candidates, goal, cap)
result, err := search.run(ctx)
path := result.optimizedPath()
```

收益：`open/cameFrom/gScores/closed/start/goal/actions` 不再在函数间裸传，搜索规则更像一个领域对象。

### Tool resume

当前形态：

```go
assistant, done, pending := trailingPendingToolCalls(req.Messages)
full := mergeRoundReturns(assistant.CollectToolCalls(), done, fresh)
rounds := priorModelRounds(req.Messages)
```

建议形态：

```go
point, ok := parseResumePoint(req.Messages)
full := point.merge(fresh)
state := loopState{iteration: point.priorModelRounds()}
```

收益：HITL resume 的关键不变量有名字，call/stream 两条路径更容易保持一致。

### HITL inflight tail

当前形态：

```go
saveInflightTail(bb, result)
msgs, ok := loadInflightTail(bb)
clearInflightTail(bb)
```

建议形态：

```go
tail := inflightTailStore{bb: bb}
tail.Save(result)
msgs, ok := tail.Load()
tail.Clear()
```

收益：持久化 key、序列化格式、缺失处理聚合，后续支持版本化或更多 resume metadata 更自然。

## 最终判断

当前项目的核心领域不是贫血模型，尤其 `core` 与 `agent/core` 已经有足够的方法和领域行为。你观察到的“零散私有函数”确实存在，但主要集中在运行时流程、协议转换、工具循环、HITL resume 这些跨边界编排区域。

后续不建议做全局“私有函数改方法”的机械重构。更好的策略是：

1. 保留纯函数和 wire mapping。
2. 对维护生命周期/状态机不变量的函数引入小对象。
3. 让对象化先发生在 GOAP search、tool resume、HITL tail、turn terminal planning 这四个高收益点。
4. 每次重构都以减少参数组、集中不变量、降低 call/stream 双路径分叉为验收标准。

