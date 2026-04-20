# 执行流程总览

> 0.4.0-SNAPSHOT

本文给出端到端的执行链路。深入的状态转移、并发、终止、重规划细节见 `execution-flow-deep.md`。

## 1. 启动期:Spring Boot 冷启动

```
SpringApplication.run()
  ↓
AgentPlatformAutoConfiguration 生效
  ↓
ScanConfiguration 扫描 @Agent / @EmbabelComponent 类
  ↓
对每个 bean:
    AgentMetadataReader.createAgentMetadata(bean)
      ├─ 校验:@Agent 类构造器不能注入 OperationContext (#1618)
      ├─ findActionMethods() / findConditionMethods() / findCostMethods()
      ├─ 从 goal getter / @AchievesGoal 方法收集 Goal
      ├─ 递归 unrollStateType() 展开 @State 类
      └─ AgentValidationManager 校验(对 GOAP plan 做路径可达性校验)
  ↓
AgentPublisher 把每个 Agent 注册到 DefaultAgentPlatform(ConcurrentHashMap)
  ↓
其他 autoconfigure 生效:
    · 各 LLM provider(Anthropic / OpenAI / ...)注册 LlmService bean
    · Rag / MCP / A2A / Observability 各自注入 bean
```

扫描期间,**任何一个 `@Agent` bean 通过构造器注入 `OperationContext` 都会抛 `IllegalStateException`**(#1618),消息中会给出推荐用法("把 `OperationContext` 作为 `@Action` 方法参数")。

## 2. 执行入口:三种模式

```
                                Autonomy(api.common.autonomy.Autonomy)
                                        │
              ┌─────────────────────────┼───────────────────────────┐
              ▼                         ▼                           ▼
       runAgent(input, opts, agent)   classifyAndRun(userIntent)   openMode(userIntent)
       ─────────────────────────      ─────────────────────────   ─────────────────────
       聚焦模式:调用方明确指定          封闭模式:平台在已部署 Agent   开放模式:跨 Agent
       要运行的 Agent                  池中分类选择                  从全部 Goal 动态组装
```

三种模式**最终都**进入 `runAgent(...)`,差异仅在"Agent 从哪里来"。

## 3. 运行期:一次 `runAgent(input, opts, agent)` 的完整链

```
Autonomy.runAgent(inputObject, processOptions, agent)
  1. 自治绑定(0.4.0 更宽松):
     bindings = {
       "it":             inputObject,
       "<typeDerivedName>": inputObject      // 例如 UserInput → "userInput"
     }
  2. AgentPlatform.createAgentProcess(agent, processOptions, bindings)
       ├─ BlackboardProvider.createBlackboard()
       ├─ 为每个 binding 调用 blackboard.bind(...)
       ├─ AgentProcessIdGenerator.createProcessId(agent, opts)
       ├─ PlannerFactory.createPlanner(opts, worldStateDeterminer)
       ├─ AgentProcessRepository.save(process)
       └─ 返回 SimpleAgentProcess 或 ConcurrentAgentProcess
  3. agentProcess.run()                      // 主循环,见 §4
  4. 完成后:
       ├─ agentProcess.cost() / usage() / modelsUsed()  // 含子进程聚合(#1615)
       └─ 返回 AgentProcessExecution(result, process)
```

## 4. OODA 主循环

```
AbstractAgentProcess.run() {
    makeRunning()
    while (status == RUNNING) {
        formulateAndExecutePlan()               // 即 tick()
        identifyEarlyTermination()               // 检查信号 / 策略
    }
    emit <终态> event
}

formulateAndExecutePlan() {
    /* Observe */
    worldState = worldStateDeterminer.determineWorldState()
                   for each knownCondition:
                     · SpEL 表达式        → 解析 + 求值(缺失变量 → FALSE)
                     · "var:Type" 绑定    → blackboard.getValue() + 类型检查
                     · "hasRun:..."      → blackboard.getCondition(...)
                     · 命名 Condition     → Condition.evaluate(OperationContext)
                     · 默认                → blackboard.getCondition()

    /* Orient */
    plan = planner.bestValuePlanToAnyGoal(agent.planningSystem, replanBlacklist)
                   → AStarGoapPlanner.planToGoalFrom(worldState, actions, goal)
                     · openList: PriorityQueue<SearchNode> (按 f = g + h)
                     · 扩展:对每个 achievable action,计算 nextState,更新 gScore
                     · 找到目标后做 backwardOptimize + forwardOptimize

    if (plan == null) {
        if (replanBlacklist.isNotEmpty()) {
            replanBlacklist.clear()              // 再给一次机会
            return                                // 下轮重试
        }
        status = STUCK; stuckHandler?.handle(process)
        return
    }

    /* Decide */
    SimpleAgentProcess:  actionToRun = [plan.actions.first()]
    ConcurrentAgentProcess: actionsToRun = plan.actions.filter { isAchievable(ws) }

    /* Act */
    for (action in actionsToRun) [并发/串行]:
        status = executeAction(action)        // 见 §5
        updateStatusFromActionStatus(status)
}
```

## 5. 单个 Action 的执行

```
executeAction(action) {
    emit ActionExecutionStartEvent
    schedule = OperationScheduler.scheduleAction(startEvent)
    when (schedule) {
        Pronto           → 立即执行
        Delayed(dt)      → Thread.sleep(dt) 后执行
        Scheduled(ts)    → return PAUSED(调度器以后唤醒)
    }
    blackboardBefore = blackboard.objects.toList()
    try {
        effective = action.withEffectiveQos(platformProps)
        actionStatus = effective.qos.retryTemplate("Action-${action.name}").execute {
            effective.execute(processContext)          // 真正的执行
        }
    } catch (AwaitableResponseException)  → blackboard.addObject(awaitable); WAITING
    catch (ReplanRequestedException)      → 透传
    catch (TerminateActionException)      → TERMINATED
    catch (TerminateAgentException)       → AGENT_TERMINATED

    _history += ActionInvocation(name, timestamp, runningTime)
    if (blackboardObjectsBefore 未被清空)
        blackboard.setCondition("hasRun:${name}", true)
    emit ActionExecutionEndEvent(actionStatus)
    return actionStatus
}
```

## 6. LLM 调用子流程(在 Action 内)

典型的 `@Action` 内部会通过 `OperationContext.promptRunner()` / `ai()` 发起 LLM 调用:

```
operationContext.promptRunner(llm = LlmOptions(...))
  .withToolObjects(...)
  .withToolGroups(ToolGroupRequirement("web", terminationScope = ACTION))
  .createObject(prompt, MyType::class.java)
                  ↓
    生成 Prompt(含 PromptContributor / LlmReference 贡献)
                  ↓
    UserInputGuardRail.validate(userMessages, blackboard)     [前置守护]
                  ↓
    LlmMessageSender.call(messages, tools)                     [非流]
       或 LlmMessageStreamer.stream(messages, tools)            [流式,#1581]
                  ↓
    ToolLoop 驱动工具调用:
       · ParallelToolLoop 并行执行独立工具
       · ToolInjectionStrategy 按进度增删工具
       · ToolNotFoundPolicy 处理幻觉工具名
       · SlidingWindowTransformer 在每次 LLM 调用前裁剪历史(保留 tool call/result 配对)
                  ↓
    AssistantMessageGuardRail.validate(message/ThinkingResponse, blackboard)   [后置守护]
                  ↓
    记录 LlmInvocation(usage + cost);挂到 agentProcess.llmInvocations
                  ↓
    Jackson 反序列化为 MyType(结构化输出)
```

## 7. 重规划

```
Action.execute() 抛 ReplanRequestedException(blackboardUpdater) {
                  ↓
    进程捕获:
        blackboardUpdater(blackboard)       // 更新黑板
        replanBlacklist.add(action.name)    // 阻止立即再选中此 action
        status = RUNNING                    // 让主循环再来一轮
                  ↓
    下一轮 formulateAndExecutePlan() 重新 Observe / Orient,
    planner 看到更新后的 WorldState,可能产生完全不同的 plan
}
```

`ConcurrentAgentProcess`(#1599 起)同样支持:并发 Action 中任一个请求重规划,**只**采用第一个的黑板更新(其他因为即将 replan 而丢弃)。

## 8. 终止与优雅停止

| 触发 | 效果 |
|------|------|
| `kill()` | 立即 KILLED,级联子进程 |
| `terminateAgent(reason)` | 在 RUNNING/NOT_STARTED 时留信号,下一个 checkpoint 转 TERMINATED;其他状态立刻转 TERMINATED;级联子进程 |
| `terminateAction(reason)` | 在当前 action 设置 ACTION scope 信号,当前 action 结束后检查 |
| `ToolGroupRequirement(terminationScope=AGENT)` + 工具缺失 | 抛 `TerminateAgentException` → AGENT_TERMINATED |
| `ToolGroupRequirement(terminationScope=ACTION)` + 工具缺失 | 抛 `TerminateActionException` → 当前 action TERMINATED,再次 replan |

## 9. 入口形态总览

```
  HTTP 请求(webmvc)          CLI(Spring Shell)           MCP Server
        │                           │                        │
        ▼                           ▼                        ▼
  @Controller                 Shell 命令                   PromptFactory
        │                           │                        │
        └───────────────┬───────────┴────────────────────────┘
                        ▼
                   Autonomy.runAgent(...)
                        ▼
                  AgentProcess.run()
                        ▼
                   OODA 循环(§4)
                        ▼
              AgentProcessExecution 结果
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
    序列化响应     Shell 打印        A2A/MCP 回传
```

A2A 服务端侧:

```
远端 Agent POST JSON-RPC 请求
    ↓
A2ARequestHandler.handleJsonRpc(req)   (支持 SSE 流)
    ↓
AutonomyA2ARequestHandler → Autonomy.runAgent(...)
    ↓
响应打包为 JSONRPCResponse<T>
```
