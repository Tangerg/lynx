# Embabel Agent：Blackboard 与 GOAP 动态规划

证据基线：`embabel-agent` 提交 `6988f286544bb792bed35d8ae45812c446be082d`。

## 框架层判断

Embabel Agent 的核心不是固定 DAG，也不是单一模型—工具循环，而是让 Action 根据 Blackboard 当前状态、前置条件和目标动态参与规划。它代表了 Scope 当前没有选择的一条路线：运行时根据目标搜索可行行动序列。

Embabel Shell、示例应用和 Spring Boot 发行体验属于应用或生态维度，不进入框架评分。

## 可复核证据

- Blackboard 保存运行中对象和条件判断所需事实。
- Action 描述输入、输出、前置条件、成本和执行逻辑。
- AgentProcess 表达一次 Agent 运行及其状态。
- AgentPlatform 负责 Agent、Action 与运行设施的装配。
- GOAP 规划器根据当前状态和目标选择行动路径，而非要求作者预先固定完整流程。
- Spring 注解、事件和 Bean 是主要扩展与装配手段。

## 八维对照

| 维度 | Embabel 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 领域对象和 Spring 生态优先 | Scope 更强调独立协议与 provider 隔离 |
| 最小契约 | Action/condition/goal/process 形成规划模型 | Scope Definition/Execution 更小，但不自动规划 |
| 状态所有权 | Blackboard 与 AgentProcess 持有共享运行事实 | Scope Execution 快照更封闭，Host 持有产品事实 |
| 副作用 | Action 执行时直接发生 | Scope Step 产出 Effect，不直接 I/O |
| 编排 | GOAP 根据目标动态选 Action | Scope Workflow 使用闭合 Stage 与明确子 Process |
| 恢复 | Process/Blackboard 有持久化方向 | Action 外部工作不自动拥有 Effect 重放身份 |
| 扩展 | 注解、Spring Bean、事件 | Scope middleware/listener 更宿主中立 |
| 依赖 | Spring/JVM 生态集成强 | Scope 组装显式，应用容器耦合更低 |

## Scope 应该借鉴什么

1. **目标与当前事实分离。** 即使 Scope 不引入 GOAP，执行为何完成、还缺什么条件也应可观测。
2. **Action 元数据。** 前置条件、产出和成本是规划、解释和测试的共同语言。
3. **执行路径解释。** 动态选择时记录“为什么选这个行动”，比只记录最终调用更适合审计。
4. **Blackboard 的消费者视角。** 多 Action 协作时，共享事实的查询体验值得借鉴，但所有权仍需由 Host 明确。

## Scope 不应照搬什么

- 不应把 GOAP 加进通用内核；它是高成本、强意见的规划策略。
- 不应用无约束共享 Blackboard 替代类型化 ExecutionState。
- 不应让 Action 内任意副作用绕过长期执行的 Effect 语义。
- 不应引入注解扫描或容器隐式装配来替代显式 Go 依赖。

## 最终定位

当任务路径需要根据运行中事实和目标动态规划时，Embabel 的模型比 Scope 闭合 Workflow 更自然。当重点是确定性恢复、可识别副作用和子执行生命周期时，Scope 的语义更强。动态规划可以成为 Scope 上层策略，但不应成为其内核默认。
