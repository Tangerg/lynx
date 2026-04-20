# 核心运行时架构

> 对象:`embabel-agent-api` 中的 `core` / `plan` / `spi` 三大包
> 版本:0.4.0-SNAPSHOT

## 1. 核心抽象五件套

| 概念 | Kotlin 类型 | 一句话 |
|------|-------------|--------|
| **Agent** | `com.embabel.agent.core.Agent`(`data class`) | 一组 Action + Goal + Condition 的**静态定义**;不持有运行时状态 |
| **AgentProcess** | `com.embabel.agent.core.AgentProcess`(interface) | Agent 的**一次执行**,持有状态、黑板、历史、规划器 |
| **Blackboard** | `com.embabel.agent.core.Blackboard`(interface) | Action 之间传递数据的**类型化共享内存**;线程安全、只追加(可隐藏) |
| **Planner** | `com.embabel.plan.Planner<S, W, P>`(interface) | 给定当前 WorldState,找到到达 Goal 的 **Action 序列** |
| **Condition** | `com.embabel.agent.core.Condition`(interface) | 带名谓词,返回**三值** `TRUE / FALSE / UNKNOWN` |

---

## 2. OODA 执行循环

`AgentProcess.run()` 本质是 **Observe → Orient → Decide → Act** 的循环:

```
run() {                                             // AbstractAgentProcess:211
  makeRunning()
  while (status == RUNNING) {
    tick()                                           // 一次 OODA 迭代
  }
}

tick() = formulateAndExecutePlan(worldState) {
  1. OBSERVE  : worldStateDeterminer.determineWorldState()
                  └─ BlackboardWorldStateDeterminer 遍历所有 condition,
                     求值后得到 ConditionWorldState(Map<String,ConditionDetermination>)
  2. ORIENT   : planner.bestValuePlanToAnyGoal(system, replanBlacklist)
                  └─ A* 搜索,返回最优 ConditionPlan 或 null
  3. DECIDE   : plan.actions.first() 或 plan.achievableActions
  4. ACT      : executeAction(action) → ActionStatus
                  └─ 成功 → 继续;失败 → FAILED;WAITING/PAUSED/TERMINATED 各有处理
}
```

**关键状态码**(`AgentProcessStatusCode`):

`NOT_STARTED` · `RUNNING` · `COMPLETED` · `FAILED` · `STUCK`(规划器找不到方案) · `WAITING`(等待外部输入,如 HITL) · `PAUSED` · `KILLED` · `TERMINATED`(提前优雅终止)

两种实现:

- **SimpleAgentProcess**(`core.support.SimpleAgentProcess`):串行,每轮只执行 `plan.actions[0]`
- **ConcurrentAgentProcess**(`core.support.ConcurrentAgentProcess`):并行,每轮执行 `plan` 中所有**当前可达**的 Action,通过 `platformServices.asyncer.async { ... }` 提交;状态聚合优先级 `AGENT_TERMINATED > FAILED > PAUSED > SUCCEEDED > TERMINATED > WAITING`

---

## 3. GOAP 规划器:A\* 搜索

**文件**:`embabel-agent-api/src/main/kotlin/com/embabel/plan/goap/astar/AStarGoapPlanner.kt`

### 3.1 搜索空间建模

```
节点 = ConditionWorldState (Map<String, ConditionDetermination>)
边   = ConditionAction:应用 action.effects 后产生新的 ConditionWorldState
目标 = goal.isAchievable(state) 即 state 满足 goal.preconditions
```

### 3.2 A\* 主循环

```kotlin
while (openList.isNotEmpty() && iterationCount < 10_000) {
    val current = openList.poll()                   // 按 f-score 取最小
    if (current.state in closedSet) continue
    closedSet.add(current.state)

    if (goal.isAchievable(current.state)) {         // 找到目标
        if (current.gScore < bestGoalScore) {
            bestGoalNode = current; bestGoalScore = current.gScore
        }
        continue                                    // 不从目标扩展
    }

    for (action in actions.sortedByDescending { it.preconditions.size }) {
        if (!action.isAchievable(current.state)) continue
        val nextState = current.state + action     // 应用 effects
        val tentativeG = gScores[current.state]!! + action.cost(startState)
        if (tentativeG < gScores[nextState]) {
            cameFrom[nextState] = current.state to action
            gScores[nextState] = tentativeG
            openList.add(SearchNode(nextState, tentativeG, heuristic(nextState, goal)))
        }
    }
}
```

### 3.3 启发函数

```kotlin
heuristic(state, goal) = goal.preconditions.count { (k, v) -> state.state[k] != v }.toDouble()
```

**admissible**:从未高估实际代价(每缺一个条件至少需要一个 Action 来满足),保证 A\* 返回**最优**方案。

### 3.4 计划优化(关键)

找到目标状态后**不**直接返回路径,而是做两次优化:

1. **Backward optimization**:从目标回溯,只保留"建立了后续 Action 所需条件"的 Action
2. **Forward optimization**:前向模拟执行,剔除不推进目标的 Action

如果优化过于激进(导致 plan 无效),回退原始路径。

### 3.5 Replan Blacklist

当某个 Action 执行时抛出 `ReplanRequestedException`,下一轮规划会:
```
planner.bestValuePlanToAnyGoal(system, excludedActionNames = replanBlacklist)
```
阻止"刚刚请求 replan 的 action"立即再被选中,避免死循环。未找到方案时再清空 blacklist 重试。

---

## 4. 黑板(Blackboard)

**接口**:`com.embabel.agent.core.Blackboard`
**默认实现**:`com.embabel.agent.core.support.InMemoryBlackboard`

### 4.1 语义

- **只追加**:`addObject(v)` / `bind(name, v)`;不能删除,只能 `hide(v)` 或 `clear()`
- **受保护绑定**:`bindProtected(key, v)` 在 `clear()` 时保留,用于跨状态的稳定上下文
- **类型解析**:`getValue(variable, type, dd)` 支持**父类匹配**——声明参数为 `Person`,但黑板里存的是 `Employee`,也能找到
- **条件存储**:`setCondition(key, bool)` / `getCondition(key)` ;`null` 表示未设置,由调用者决定映射到 TRUE/FALSE/UNKNOWN 中的哪个
- **线程安全**:`ConcurrentHashMap` + `Collections.synchronizedList`

### 4.2 "it" 与命名绑定

- `bind("it", v)` 是默认绑定,代表"上一个输出"
- `bind("userInput", v)` 是命名绑定,可被 `@RequireNameMatch("userInput")` 参数精准匹配
- **自治绑定(Autonomy binding)**(#306385b1 修复):`Autonomy.runAgent(input)` 同时按 `"it"` 和 **类型衍生名(`inputObject::class.simpleName` 首字母小写)** 绑定,这样 YAML 定义的 Action 也能通过类型名匹配到,而不必依赖 `it`

### 4.3 BlackboardWorldStateDeterminer

将黑板翻译为规划器所需的 `ConditionWorldState`(`core.support.BlackboardWorldStateDeterminer`):

```
for condition in knownConditions:
    条件格式          求值方式
    ---------         -----------
    SpEL 表达式       SpelLogicalExpression.evaluate(blackboard)
    "var:Type"       blackboard.getValue(var, Type) 后检查类型匹配
    "hasRun:Xxx"     blackboard.getCondition(...) , null → FALSE
    已注册的 Condition 对象   cond.evaluate(OperationContext)
    其他              blackboard.getCondition(key) , null → FALSE
```

**SpEL 语义变更(#950b32ce)**:表达式引用的变量不在黑板上时,返回 **FALSE**(而非 UNKNOWN)。语义理由:"对象不存在,条件当然不满足;返回 UNKNOWN 会让规划器误把无关 Action 拉进搜索空间"。

---

## 5. 三值逻辑

**文件**:`com/embabel/plan/common/condition/ConditionDetermination.kt`

```kotlin
enum class ConditionDetermination { TRUE, FALSE, UNKNOWN }
```

### 5.1 构造

```kotlin
ConditionDetermination(true)   // TRUE
ConditionDetermination(false)  // FALSE
ConditionDetermination(null)   // UNKNOWN
```

### 5.2 组合(带短路)

| 运算 | 规则 |
|------|------|
| `NOT` | TRUE→FALSE · FALSE→TRUE · **UNKNOWN→UNKNOWN** |
| `OR` | 任一为 TRUE → TRUE;均 FALSE → FALSE;其余 UNKNOWN |
| `AND` | 任一为 FALSE → FALSE;均 TRUE → TRUE;其余 UNKNOWN |
| `INV`(一元 `~` / `inv()`) | UNKNOWN→UNKNOWN 其他保持 |

### 5.3 为什么三值?

标准布尔只能表达"我知道它成立 / 不成立"。GOAP 规划在信息不全时需要第三种语义——"我不知道"——以便:
- 不把不确定条件当成已满足 → 避免过早终止规划
- 不把不确定条件当成不满足 → 避免把可能成立的分支剪掉
- 推动"先执行能确定该条件的 Action"

实际上,`UNKNOWN` 让某些 Action 的后置效果可以**主动"将不确定变为确定"**,这是 Embabel 区别于大多数确定性工作流引擎的深层设计。

---

## 6. Action 的执行模型

**文件**:`com/embabel/agent/core/ActionRunner.kt` + `AbstractAgentProcess.executeAction()`(:491)

### 6.1 生命周期

```
emit ActionExecutionStartEvent
  → OperationScheduler.scheduleAction(event)
      Pronto        / 立即执行
      Delayed(dt)   / Thread.sleep(dt) 后执行
      Scheduled(ts) / 返回 PAUSED;以后由调度器恢复
  → 保存 blackboardObjectsBefore(用于检测 clear)
  → action.withEffectiveQos(platformProps).qos.retryTemplate("Action-${name}")
       .execute { action.execute(processContext) }    // 带重试
  → catch
       AwaitableResponseException → WAITING,把 awaitable 加入黑板
       ReplanRequestedException   → 透传给进程,进程加入 blacklist 并重规划
       TerminateActionException   → TERMINATED(当前 action)
       TerminateAgentException    → AGENT_TERMINATED(整个进程)
       其他 Throwable             → 透传为失败
  → 记录 ActionInvocation(name, timestamp, runningTime)
  → 如果黑板没被清,设置 blackboard.setCondition("hasRun:${action.name}", true)
emit ActionExecutionEndEvent
```

### 6.2 QoS(重试策略)

**文件**:`core.support.ActionQosExtensions.withEffectiveQos`

```
if (action.qos != ActionQos())   // action 已显式配置 → 用自己的
    return action
val platformQos = properties.default.toActionQos()
if (platformQos == ActionQos())  // 平台也没配 → 无操作
    return action
return QosOverridingAction(action, platformQos)   // 否则用平台默认
```

**覆盖点对称性(#1572)**:注解 `@Action` 的 QoS 在 `DefaultActionQosProvider` 阶段已有值;DSL / workflow builder 构建的 Action 默认是 `ActionQos()`,在 `executeAction` 时注入平台默认。确保三种构造路径表现一致。

### 6.3 成本聚合(#368/#1615 修复)

```
AgentProcess.cost()   = ownCost() + childProcesses.sumOf { it.cost() }
AgentProcess.usage()  = ownUsage + childProcesses.aggregate()
AgentProcess.modelsUsed() = distinct, sorted
childProcesses = agentProcessRepository.findByParentId(id)
```

**场景**:一个父 Agent 调用子 Agent(`Subagent` 工具或 `RunSubagent`)时,子进程的 LLM 调用成本之前没有被计入父进程。0.4.0 通过递归遍历子进程树完整修复。

---

## 7. 终止与控制流

### 7.1 信号类型

- **`TerminateActionException`**:只终止当前 Action,Agent 继续
- **`TerminateAgentException`**:级联终止整个进程
- **`AwaitableResponseException`**:挂起等待外部响应(HITL)
- **`ReplanRequestedException`**:用新信息触发重规划

### 7.2 TerminationScope(#1593 新机制)

`ToolGroupRequirement.terminationScope` 决定"缺失工具时"抛什么异常:

| scope | 缺失工具 → 抛出 |
|-------|----------------|
| `null`(默认) | `RequiredToolGroupException` |
| `AGENT` | `TerminateAgentException`(整个进程停) |
| `ACTION` | `TerminateActionException`(当前动作停,继续 replan) |

给了上层应用**按"容忍度"定制失败处理**的能力。

### 7.3 主动终止 API

```kotlin
agentProcess.terminateAgent(reason)    // 延迟信号;在下一个检查点应用
agentProcess.terminateAction(reason)   // 设置 ACTION scope 的信号
agentProcess.kill()                    // 立即 KILLED,级联子进程
```

`AbstractAgentProcess.identifyEarlyTermination()`(:372)检查所有累积的信号与策略,每轮 tick 都会调用。

---

## 8. 执行入口:Autonomy

**文件**:`api.common.autonomy.Autonomy`

三种运行模式:

| 方法 | 何时使用 |
|------|----------|
| `runAgent(input, opts, agent)` | **聚焦模式**:明确知道要调哪个 Agent |
| `classifyAndRun(userIntent)` | **封闭模式**:平台从已部署 Agent 中选最合适的 |
| `openMode(userIntent)` | **开放模式**:从全部 Goal 中动态组装新的"临时 Agent" |

`runAgent` 是最底层的 API,其他两种模式内部也最终调用它。

---

## 9. 三个核心流:读懂就算入门

**输入流**:`User → ProcessOptions → AgentProcess → Blackboard`
**控制流**:`AgentProcess.tick() ←→ Planner ←→ WorldStateDeterminer ←→ Blackboard`
**数据流**:`Action 输入 ← Blackboard ← 上一 Action 输出 / Autonomy 初始绑定`

这三条流互相正交——输入流通过 `ProcessOptions` 注入一次;控制流在每个 tick 闭合循环;数据流在每次 Action 成功完成时前进一格。
