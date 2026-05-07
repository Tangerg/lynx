# 执行流程深度分析(源码级)

> 0.4.0-SNAPSHOT
> 本文给出逐行级别的控制流、状态转移、并发、成本聚合、终止、重规划细节。所有路径为 `embabel-agent-api/src/main/kotlin/` 下的相对路径。

---

## 1. 关键类与文件索引

| 作用 | 文件 |
|------|------|
| 进程抽象基类 | `com/embabel/agent/core/support/AbstractAgentProcess.kt` |
| 串行实现 | `com/embabel/agent/core/support/SimpleAgentProcess.kt` |
| 并发实现 | `com/embabel/agent/core/support/ConcurrentAgentProcess.kt` |
| A\* GOAP 规划器 | `com/embabel/plan/goap/astar/AStarGoapPlanner.kt` |
| 三值逻辑 | `com/embabel/plan/common/condition/ConditionDetermination.kt` |
| 世界状态 | `com/embabel/plan/common/condition/ConditionWorldState.kt` |
| 黑板 | `com/embabel/agent/core/support/InMemoryBlackboard.kt` |
| 黑板→WorldState 桥 | `com/embabel/agent/core/support/BlackboardWorldStateDeterminer.kt` |
| Action 执行工具 | `com/embabel/agent/core/ActionRunner.kt` |
| 子进程工具 | `com/embabel/agent/api/tool/Subagent.kt` · `com/embabel/agent/api/annotation/RunSubagent.kt` |
| 工具循环 | `com/embabel/agent/spi/loop/ToolLoop.kt` 及 `support/DefaultToolLoop.kt` / `support/ParallelToolLoop.kt` |
| 滑窗转换 | `com/embabel/agent/api/tool/callback/SimpleToolLoopCallbacks.kt`(`SlidingWindowTransformer`) |

---

## 2. 进程启动:`run()` 主循环

```kotlin
// AbstractAgentProcess.run()  (:211)
override fun run(): AgentProcess {
    makeRunning()
    while (status == AgentProcessStatusCode.RUNNING) {
        tick()
    }
    return this
}

// AbstractAgentProcess.tick()
override fun tick(): AgentProcess {
    val worldState = worldStateDeterminer.determineWorldState()
    lastWorldState = worldState
    formulateAndExecutePlan(worldState)          // 子类实现
    identifyEarlyTermination()                   // 检查信号、更新状态
    return this
}
```

**makeRunning()**(:258 附近)完成:
1. `_status.compareAndSet(NOT_STARTED, RUNNING)`
2. 发射 `ProcessStartedEvent`
3. `processContext.agentProcess = this`

**为什么是 while 循环**:每一次 `tick` 都会**重新**调用 `determineWorldState()` 和 `bestValuePlanToAnyGoal()`,即使上一轮已经得到 plan,也只取其"下一步"并扔掉剩余——这正是 OODA 的精神:环境可能在 Action 执行过程中被改变,计划必须持续适应。

---

## 3. SimpleAgentProcess.formulateAndExecutePlan

```kotlin
// SimpleAgentProcess.kt:40-226 核心逻辑伪代码化:
override fun formulateAndExecutePlan(worldState: WorldState) {
    val planToUse = planner.bestValuePlanToAnyGoal(
        planningSystem = agent.planningSystem.prune(worldState),
        excludedActionNames = replanBlacklist
    ) ?: run {
        if (replanBlacklist.isNotEmpty()) {
            replanBlacklist.clear()              // 黑名单导致无解 → 清空重试
            return
        }
        _status.set(STUCK)
        stuckHandler?.handle(this)
        emit(ProcessStuckEvent(...))
        return
    }

    replanBlacklist.clear()

    // 目标已满足?
    if (planToUse.isGoalAchieved) {
        _status.set(COMPLETED)
        emit(ProcessCompletedEvent(...))
        return
    }

    val action = planToUse.actions.first()       // Decide
    try {
        val status = executeAction(action)        // Act
        when (status.status) {
            SUCCEEDED         -> { /* keep RUNNING */ }
            FAILED            -> _status.set(FAILED)
            WAITING           -> _status.set(WAITING)
            PAUSED            -> _status.set(PAUSED)
            TERMINATED        -> { /* keep RUNNING,让下一 tick 继续 */ }
            AGENT_TERMINATED  -> _status.set(TERMINATED)
        }
    } catch (e: ReplanRequestedException) {
        e.blackboardUpdater(blackboard)
        replanBlacklist.add(action.name)
        emit(ReplanRequestedEvent(...))
        // status 保持 RUNNING,下一 tick 重新规划
    }
}
```

**关键设计点**:

1. **`replanBlacklist` 不是"永久禁用"**:只作用于**下一次**规划调用。如果黑名单导致无解,它会被清空重试——确保不会因为一次重规划请求永久破坏规划能力。

2. **`SUCCEEDED` 后**不是直接跳到"已达目标",而是**回到主循环的条件判断**:下一轮 `tick` 重新计算 WorldState 并规划。这让外部变化(如 Subagent 执行期间修改黑板)能被捕获。

3. **`TERMINATED`(action 级)保持 RUNNING**:和 `AGENT_TERMINATED`(进程级)是不同语义——前者只是跳过当前 action,后者整体停止。

---

## 4. ConcurrentAgentProcess 的并发扩展

```kotlin
// ConcurrentAgentProcess.kt 核心片段:
override fun formulateAndExecutePlan(worldState: WorldState) {
    val plan = planner.bestValuePlanToAnyGoal(...)
    if (plan == null) { /* 同 Simple */ }
    if (plan.isGoalAchieved) { /* 同 Simple */ }

    val achievable = plan.actions.filter { action ->
        action.isAchievable(worldState as ConditionWorldState)
    }

    val statuses = CopyOnWriteArrayList<ActionStatus>()
    val replanRequests = CopyOnWriteArrayList<Pair<Action, ReplanRequestedException>>()

    val futures = achievable.map { action ->
        platformServices.asyncer.async {
            callbacks.forEach { it.onActionLaunch(action) }
            val elapsed = kotlin.time.measureTime {
                try {
                    statuses += executeAction(action)
                } catch (e: ReplanRequestedException) {
                    replanRequests += action to e
                }
            }
            callbacks.forEach { it.onActionComplete(action, elapsed) }
        }
    }
    futures.forEach { it.join() }

    // 聚合状态(优先级)
    val aggregated = statuses.maxByOrNull { it.status.priority }
        ?: ActionStatus(Duration.ZERO, SUCCEEDED)
    updateProcessStatus(aggregated)

    // 重规划:只取第一个请求
    if (replanRequests.isNotEmpty()) {
        val (firstAction, firstRequest) = replanRequests.first()
        firstRequest.blackboardUpdater(blackboard)
        replanBlacklist.add(firstAction.name)
        emit(ReplanRequestedEvent(...))
    }
}
```

**状态优先级**(高→低):
```
AGENT_TERMINATED  >  FAILED  >  PAUSED  >  SUCCEEDED  >  TERMINATED  >  WAITING
```

**为什么丢弃其他 replan 请求的黑板更新**:如果几个并发 action 都要 replan,我们已经准备 replan 了——再合并它们的更新只会让 WorldState 更难预测。只取第一个,其他的交给下一轮 replan 自己再试(它们的 action 已经加入黑名单则会被排除)。

---

## 5. A\* 搜索的实现细节

### 5.1 `SearchNode`

```kotlin
private class SearchNode(
    val state: ConditionWorldState,
    val gScore: Double,   // 从起点到该状态的累计 action.cost()
    val hScore: Double,   // 启发函数:到目标的剩余代价估计
) : Comparable<SearchNode> {
    private val fScore: Double = gScore + hScore
    override fun compareTo(other: SearchNode) = fScore.compareTo(other.fScore)
}
```

PriorityQueue 是小顶堆,所以每次 `poll()` 取的是 `f` 最小的节点——A\* 的核心。

### 5.2 主循环(简化)

```kotlin
while (openList.isNotEmpty() && iterationCount++ < 10_000) {
    val current = openList.poll()

    // 剪枝:已有更好的目标解
    if (bestGoalNode != null && current.gScore >= bestGoalScore) continue
    if (current.state in closedSet) continue
    closedSet.add(current.state)

    // 终点?
    if (goal.isAchievable(current.state)) {
        if (current.gScore < bestGoalScore) {
            bestGoalNode = current
            bestGoalScore = current.gScore
        }
        continue     // 不从目标继续扩展
    }

    // 扩展
    for (action in actions.sortedByDescending { it.preconditions.size }) {
        if (!action.isAchievable(current.state)) continue
        val nextState = current.state + action
        if (nextState == current.state) continue            // 空转
        val tentativeG = gScores[current.state]!! + action.cost(startState)
        if (bestGoalNode != null && tentativeG >= bestGoalScore) continue

        if (tentativeG < gScores.getValue(nextState)) {
            cameFrom[nextState] = current.state to action
            gScores[nextState] = tentativeG
            openList.add(SearchNode(nextState, tentativeG, heuristic(nextState, goal)))
            // 允许重开 closed 节点(当找到更低 g 的新路径)
            closedSet.remove(nextState)
        }
    }
}
```

**几个工程细节**:

- **`iterationCount < 10_000`** 上限:避免退化场景(状态空间爆炸)导致无限搜索
- **`sortedByDescending { preconditions.size }`**:先展开"前置条件最多"的 action,等价于"更具体 / 受限的 action 优先",启发式地减少分支
- **重开已关闭节点**:A\* 的经典条件。因为 heuristic 是 admissible 的但不一定 consistent(monotonic),允许后发现的更优路径更新 gScore

### 5.3 计划优化

路径重建后并不是最终方案,`backwardPlanningOptimization` 和 `forwardPlanningOptimization` 依次剪掉:
- 不建立任何后续 action 所需 precondition 的 action(后向剪)
- 不推进任何 goal precondition 的 action(前向剪)

如果剪后的方案无法从 startState 达到目标,回退原方案。

---

## 6. `executeAction` 的控制流与埋点

```kotlin
// AbstractAgentProcess.executeAction(action)  (:491)
protected fun executeAction(action: Action): ActionStatus {
    val outputTypes = action.outputs.associateBy({ it.name }, { agent.resolveType(it.type) })
    val startEvent = ActionExecutionStartEvent(
        agentProcess = this,
        action = action,
        outputTypes = outputTypes,
    )
    platformServices.eventListener.onProcessEvent(startEvent)

    val schedule = platformServices.operationScheduler.scheduleAction(startEvent)
    when (schedule) {
        is ProntoActionExecutionSchedule   -> { /* no-op */ }
        is DelayedActionExecutionSchedule  -> Thread.sleep(schedule.delay.toMillis())
        is ScheduledActionExecutionSchedule-> return ActionStatus(Duration.ZERO, PAUSED)
    }

    val blackboardObjectsBefore = blackboard.objects.toList()
    val timestamp = Instant.now()

    val actionStatus = try {
        withCurrent {
            val effective = action.withEffectiveQos(platformServices.actionQosProperties())
            effective.qos.retryTemplate("Action-${action.name}").execute<ActionStatus, Throwable> {
                effective.execute(processContext)
            }
        }
    } catch (e: TerminateActionException) {
        ActionStatus(Duration.between(timestamp, Instant.now()), ActionStatusCode.TERMINATED)
    } catch (e: TerminateAgentException) {
        ActionStatus(Duration.between(timestamp, Instant.now()), ActionStatusCode.AGENT_TERMINATED)
    }
    // 注意:ReplanRequestedException 不在这里捕获,透传给 SimpleAgentProcess.formulateAndExecutePlan

    val runningTime = Duration.between(timestamp, Instant.now())
    _history += ActionInvocation(action.name, timestamp, runningTime)

    // hasRun 条件 —— 如果黑板被 @Action(clearBlackboard=true) 清空则跳过
    val blackboardWasCleared = blackboard.objects.none { it in blackboardObjectsBefore }
    if (!blackboardWasCleared) {
        blackboard.setCondition(Rerun.hasRunCondition(action), true)
    }

    platformServices.eventListener.onProcessEvent(
        startEvent.resultEvent(actionStatus = actionStatus)
    )
    return actionStatus
}
```

**`withCurrent { ... }`**:把当前 `AgentProcess` 作为 ThreadLocal 绑定,`AgentProcess.get()` 能在嵌套调用栈中拿到;对 Kotlin 协程兼容(coroutine context element)。

**`retryTemplate`**:Spring Retry,按 `ActionQos` 的 `maxAttempts / backoffMillis / backoffMultiplier` 配置。`ActionQos.idempotent = false` 的情况只重试"可重试的异常"(由 `RetryPolicy` 决定)。

---

## 7. Action 内部:LLM 调用

`Action.execute()` 的大多数实现最终通过 `operationContext.promptRunner()` 进入 LLM:

```
PromptRunner.createObject(T::class.java, prompt)
    ↓
ChatClientLlmOperations(或自研 ToolLoop 路径)
    ↓
前置:UserInputGuardRail.validate(userMessages, blackboard)
    ↓
LlmMessageSender.call(messages, tools)
    │
    ├─ 进入 ToolLoop 驱动多轮(如果 LLM 要求调用工具):
    │     while (!terminated) {
    │         transformers.forEach { before(...) }
    │         response = llmMessageSender.call(history, tools)
    │         if (response 没有 toolCalls) break
    │         for toolCall in response.toolCalls (并行或串行):
    │             tool = findTool(toolCall.name) ?: toolNotFoundPolicy.handle(...)
    │             history += toolResult
    │         injectionStrategy.evaluate(ctx).apply(tools)
    │         transformers.forEach { after(...) }   // e.g. SlidingWindow 裁剪
    │     }
    │
    └─ 流式路径(#1581):LlmMessageStreamer.stream(...)
       返回 Flux<String>,工具处理交给底层框架
    ↓
后置:AssistantMessageGuardRail.validate(message | ThinkingResponse<*>, blackboard)
    ↓
ToolLoopResult(rawOutput, usage, history)
    ↓
记录 LlmInvocation 并挂到 agentProcess.llmInvocations
    ↓
Jackson 反序列化为 T
```

### SlidingWindowTransformer(#1569)的内部算法

```kotlin
fun applyWindow(history: List<Message>): List<Message> {
    val truncated = if (history.size > maxMessages) {
        if (preserveSystemMessages) {
            val system = history.filterIsInstance<SystemMessage>()
            val nonSystem = history.filterNot { it is SystemMessage }
            system + nonSystem.takeLast(maxMessages - system.size)
        } else history.takeLast(maxMessages)
    } else history

    // 修复 tool call/result 成对断裂:
    // LLM API 要求每个 ToolResultMessage 前必须有包含其 id 的 AssistantMessageWithToolCalls
    val systemMessages = truncated.filterIsInstance<SystemMessage>()
    val nonSystem = truncated.filterNot { it is SystemMessage }
    var firstValid = 0
    for (msg in nonSystem) {
        if (msg !is ToolResultMessage) break
        firstValid++
    }
    return systemMessages + nonSystem.drop(firstValid)
}
```

**为什么要丢弃孤立的 tool result**:如果窗口裁剪把 `AssistantMessageWithToolCalls(id=x)` 切掉但留下了 `ToolResultMessage(id=x)`,OpenAI / Anthropic 的 API 会 400。#1569 修复前,裁剪只按长度做,导致长对话随机 500。

---

## 8. Subagent / 子进程

### 8.1 两条路径

| 方式 | 同步性 | 特点 |
|------|--------|------|
| `Subagent` 工具(`api.tool.Subagent`) | 工具调用时同步执行 | 推荐;可在 LLM 工具定义中自然使用 |
| `RunSubagent.instance(agent, T::class.java)`(`api.annotation.RunSubagent`) | 抛 `SubagentExecutionRequest` 异常(#1390) | 总是跳出调用点;调用后代码不可达 |

### 8.2 子进程成本聚合(#1615/#368 修复)

```kotlin
// AbstractAgentProcess 伪代码(:151-225 附近):
override fun cost(): Double {
    val own = llmInvocations.sumOf { it.cost() }
    val children = platformServices.agentProcessRepository.findByParentId(id)
    return own + children.sumOf { it.cost() }   // 递归
}

override fun usage(): Usage {
    val own = llmInvocations.map { it.usage }.reduceOrNull { a, b -> a + b } ?: Usage.ZERO
    val children = platformServices.agentProcessRepository.findByParentId(id)
    return own + children.map { it.usage() }.reduceOrNull { a, b -> a + b } ?: Usage.ZERO
}

override fun modelsUsed(): List<LlmMetadata> {
    val own = llmInvocations.map { it.llmMetadata }
    val children = platformServices.agentProcessRepository.findByParentId(id).flatMap { it.modelsUsed() }
    return (own + children).distinct().sortedBy { it.name }
}
```

**注意**:这依赖 `AgentProcessRepository` 能查到子进程。默认 `InMemoryAgentProcessRepository` 保存到 `ConcurrentHashMap`,按 `parentId` 索引;替换为 JDBC / Redis 实现时必须保证 `findByParentId` 可用。

---

## 9. 终止完整状态机

```
                            ┌─────────────┐
                            │ NOT_STARTED │
                            └──────┬──────┘
                                   │ run()
                                   ▼
                            ┌─────────────┐
                         ┌──│   RUNNING   │──┐
                         │  └──────┬──────┘  │
                         │         │         │
                         │   无解/stuck       │ tick()
                         │         │         │
                         │         ▼         │
                         │   ┌────────┐      │
                         │   │ STUCK  │      │
                         │   └────────┘      │
                         │                   │
     Action FAILED ←─────┤                   ├─→ Action SUCCEEDED → 继续 RUNNING
                         │                   │
     WAITING       ←─────┤                   ├─→ goal achieved  → COMPLETED
                         │                   │
     PAUSED        ←─────┤                   ├─→ terminateAgent / AGENT_TERMINATED
                         │                   │         → TERMINATED
                         │                   │
                         │                   ├─→ kill() → KILLED
                         │                   │
                         │                   └─→ tool not found (scope=AGENT)
                         │                         → TERMINATED
                         │
                         └─ action-level TERMINATED(tool not found scope=ACTION)
                            → 继续 RUNNING(此 action 在下一轮 replan 中被排除)
```

`identifyEarlyTermination()`(`AbstractAgentProcess.kt:372-411`)在每个 tick 末尾运行:
1. 检查 `terminationRequest: AtomicReference<TerminationSignal>`;若非 null 且进程可应用 → 转 `TERMINATED`
2. 应用 `earlyTerminationPolicy`(如"超过 N 次迭代 / N 分钟 → 自动终止")
3. 清掉过时的 ACTION 级信号(没被消费的)
4. 发射 `ProcessTerminatedEvent` / `ProcessKilledEvent`

---

## 10. @Agent 的元数据读取期校验

```kotlin
// AgentMetadataReader.createAgentMetadata(instance)
private fun rejectOperationContextConstructorInjection(agentClass: Class<*>) {
    val illegal = agentClass.constructors
        .flatMap { it.parameters.toList() }
        .filter { OperationContext::class.java.isAssignableFrom(it.type) }
    if (illegal.isNotEmpty()) {
        throw IllegalStateException(
            "@Agent class '${agentClass.simpleName}' injects ... via its constructor. " +
            "OperationContext is action-scoped and cannot be constructor-injected: it would be " +
            "permanently bound to a placeholder process created at Spring wiring time, not the " +
            "process actually executing the action. ..."
        )
    }
}
```

**为什么这是致命的**:Spring 在 bean 创建时,框架提供一个**占位** `AgentProcess`(只为了满足构造器注入)。如果 `OperationContext` 被绑定到占位进程:
- Action 里 `operationContext.agentProcess` 永远指向占位,不是执行中的真实进程
- LLM 调用产生的 `LlmInvocation` 挂在占位上 → 真实进程 `cost()` 为 0
- 任何向 blackboard 写入都写到了占位的黑板

修复方式是**在 `AgentMetadataReader` 的第一步做静态校验**,启动期直接失败(fail-fast),而不是让应用默默工作到生产环境才发现成本统计为零。

---

## 11. 小结:执行流程的五大不变量

1. **"计划是每次 tick 重算的",不是首次规划就定型** —— 让 Agent 对环境变化敏感
2. **Action 与 Action 间只通过黑板通信** —— Action 自身是"输入 → 输出"的纯变换,便于测试和替换
3. **所有副作用(LLM 调用、外部 API)都通过工具 / 守护 / 调度 SPI 中介** —— 使得观测、重放、成本核算可行
4. **UNKNOWN 是一等公民** —— 规划器在信息不全时仍能选出"能减少不确定性"的 Action
5. **父进程对子进程的 LLM 成本负责**(#1615) —— 层级组合时账本仍然正确
