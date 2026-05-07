# 测试体系

> 0.4.0-SNAPSHOT

## 1. 测试模块拓扑

```
embabel-agent-test-support/       (聚合 pom)
├── embabel-agent-test-common     依赖 embabel-agent-ai + MockK;提供最基础的工具
├── embabel-agent-test            依赖 embabel-agent-api;**用户可见的公共测试基类**
└── embabel-agent-test-internal   依赖 api/domain/test/test-common;框架内部使用
```

依赖关系:`test-internal → test → test-common`。用户应用应仅依赖 `embabel-agent-test`(`<scope>test</scope>`)。

## 2. 公共测试基类:`EmbabelMockitoIntegrationTest`

**文件**:`embabel-agent-test/src/main/java/com/embabel/agent/test/integration/EmbabelMockitoIntegrationTest.java`

提供"Spring Boot 集成 + Mockito LLM 打桩"的开箱即用基类:

```java
@SpringBootTest
@TestPropertySource(properties = {
    "embabel.models.default-llm=test-model",
    "embabel.agent.verbosity.debug=true",
    // ...
})
public class EmbabelMockitoIntegrationTest {
    @Autowired protected AgentPlatform agentPlatform;
    @MockitoBean protected LlmOperations llmOperations;    // 框架会用 mock 替换

    protected <T> OngoingStubbing<T> whenCreateObject(
        Predicate<String> promptMatcher, Class<T> outputClass) { ... }

    protected <T> void verifyCreateObject(
        Predicate<String> promptMatcher, Class<T> outputClass) { ... }
}
```

**模式**:用 `Predicate<String>` 匹配 prompt(而不是 exact match),因为 prompt 通常含随机值 / 时间戳。

## 3. 配置:`application-test.yml`

0.4.0 之前放在 `src/main/resources/`,#1613 迁到 `src/test/resources/`,避免主 jar 泄漏测试配置。

## 4. 事件捕获测试

框架提供 `AgenticEventListener` 的测试实现,可用来断言 Agent 执行中产生的事件序列(如"先 `ActionExecutionStartEvent(name=A)` 再 `ReplanRequestedEvent` 再 `ActionExecutionStartEvent(name=B)`")。

典型用法:
```kotlin
val events = mutableListOf<AgentProcessEvent>()
val listener = object : AgenticEventListener {
    override fun onProcessEvent(e: AgentProcessEvent) { events += e }
}
agentPlatform.runAgentFrom(agent, ProcessOptions(listeners = listOf(listener)), bindings)
assertThat(events.map { it::class.simpleName }).containsExactly(
    "ProcessStartedEvent", "ActionExecutionStartEvent", "ActionExecutionEndEvent", ...
)
```

## 5. 测试 Agent 的常见套路

### 5.1 单元测试 Action 方法
直接 new 对象、`@Mock` 依赖,测试业务逻辑本身。**不要**在这一层测规划——规划是框架的职责。

### 5.2 集成测试 Agent 组合
用 `EmbabelMockitoIntegrationTest`,Mock `LlmOperations`,打桩返回期望的结构化对象。断言:
- 执行路径(哪几个 action 被触发 / 顺序)
- 黑板最终状态
- 产生的事件序列
- `agentProcess.cost()` / `usage()` 的正确性(若涉及多进程)

### 5.3 规划器测试
直接构造 `Agent` + `ConditionWorldState`,手动调 `planner.planToGoal(...)` 断言返回的 plan。这是验证 GOAP 行为的最佳粒度。

### 5.4 Docker 依赖测试
如 `DockerSkillScriptExecutionEngineTest`——Windows 上禁用(#1605),其他平台真的起 Docker 容器(#1601 引入 TestContainers)。

## 6. 集成测试命名与隔离

- 单元测试:`*Test`(Surefire 运行)
- 集成测试:`*IT`(Failsafe 运行)
  - 例:`ParallelToolLoopGuardRailIT` · `LLMGeminiGuardRailsIntegrationIT`(#1589)
- `#1577`:为自动化准备 IT 测试,`#1580`:在 README 更新 IT 测试说明

## 7. 0.4.0 测试方面的显著变化

| PR | 改动 |
|----|------|
| #1589 | 增 Gemini 守护集成测试 + **并行工具循环**端到端验证(验证 `AssistantMessageGuardRail` 在并行场景仍被调用) |
| #1582 的测试补充 | 验证 `AssistantMessageGuardRail` 对结构化对象(而非仅 text)也触发 |
| #1573 | 为 5 个类补充了测试(Issue #29 推动) |
| #1605 | 在 Windows 禁用 `DockerSkillScriptExecutionEngineTest` |
| #1578 | 修复 onnx 测试 |
| #1575 | 修复 JaCoCo 覆盖率报告以适配 SonarCloud |

## 8. 静态检查

| 工具 | 作用 | 配置 |
|------|------|------|
| SpotBugs + sb-contrib | 静态分析 | `spotbugs-concurrency.xml`(专注并发 bug 模式) |
| ArchUnit | 模块边界 / 依赖方向规则 | 随单元测试运行 |
| JaCoCo | 覆盖率 | 排除实验性 / 测试包 |
| Kapt | Spring Boot 配置元数据生成 | 用于 `@ConfigurationProperties` |

## 9. Surefire 配置亮点

```xml
<configuration>
    <rerunFailingTestsCount>2</rerunFailingTestsCount>   <!-- 抑制偶发 flake -->
    <failIfNoSpecifiedTests>false</failIfNoSpecifiedTests>
</configuration>
```

框架 CLAUDE.md 规定"先写失败测试,再修 bug / 加功能",两次重试是容忍偶发抖动(如超时),不是掩盖真实失败。

## 10. 给开发者的建议

1. **规划层测试不要过多 Mock**:GOAP 的健壮性来自状态搜索,打桩太多反而掩盖问题。推荐直接构建小型 agent + 真实 `AStarGoapPlanner`。
2. **验证 `cost()` 聚合**:0.4.0 修复后,含 Subagent 的测试应验证父进程 cost > 0。
3. **验证 QoS 一致性**:DSL / workflow / annotation 三种构造路径对 retry 行为应该表现一致;新 feature 加测试覆盖三条路径。
4. **Guardrail 要同时测 text 与结构化响应**(#1582 教训)。
5. **IT 测试尽量贴近真实 provider**(#1589 补强)——Mock 再好也可能掩盖 API 合约差异(tool call/result 配对、parallel tool calls 支持)。
