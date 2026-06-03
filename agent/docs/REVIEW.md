# `agent/` — Review 阅读顺序

`agent/` 是 lynx 的 agent 框架：Platform / Action / Goal / Planner /
Workflow / HITL 等抽象，构建在 `core/` 之上，Lyra 是它的产品级集成。

阅读顺序：**顶层入口 → core 抽象 → runtime 实现 → 调度域 → 计划域 →
工作流 → HITL → 示例**。

---

## 0. 阅读前先看

1. `docs/README.md` — 模块导航。
2. `docs/GUIDE.md` **[精读]** — 上手指南。
3. `docs/EXTENSION_DESIGN.md` — Extension 设计哲学。
4. `docs/EMBABEL_COMPARISON.md` — 与 embabel 的对比，理解设计偏好。
5. 顶层 `doc.go` + `agent.go` + `builder.go` — 用户入口（`agent.New`
   + Fluent builder）。

---

## 1. core/ — 框架抽象（**整库的语义骨架**）

> 这一层是后续 runtime / plan / workflow 的基础。**全文必读**。

6. `core/doc.go` — 包概览。
7. `core/agent.go` — `Agent` 类型。
8. `core/action.go` + `action_typed.go` + `action_config.go` —
   `Action` / `TypedAction` / 配置。
9. `core/goal.go` — `Goal` 抽象，含 typed `GoalProducing[T]`。
10. `core/enum.go` — `ActionStatus` / `ProcessStatus` 等枚举。
11. `core/io_binding.go` — typed 输入输出绑定（`DefaultBindingName`）。
12. `core/blackboard.go` + `inmemory_kv.go` — 黑板抽象 + 默认 KV。
13. `core/process.go` + `process_options.go` + `process_context.go`
    **[精读]** — `Process` 接口 (含 `RecordLLMInvocation` / `Budget` /
    `LLMInvocations`)；`ProcessContext` 是 action body 收到的运行时
    句柄。
14. `core/process_store.go` + `process_store_inmemory.go` —
    process 持久化。
15. `core/session.go` + `session_inmemory.go` — `Session` 抽象。
16. `core/invocation.go` — `LLMInvocation` / `EmbeddingInvocation` /
    `TokenTotals` 记录。
17. `core/output_channel.go` — typed result 输出。
18. `core/condition.go` + `prompt_condition.go` — 条件 / 决策。
19. `core/determination.go` — 多候选 Action 的判定。
20. `core/awaitable.go` — 异步等待原语（HITL 用）。
21. `core/early_termination.go` — 早停。
22. `core/extension.go` — `Extension` 抽象（platform / process scope）。
23. `core/hooks.go` — 生命周期钩子。
24. `core/guardrails.go` — `Guardrails`（call / stream middleware
    集合）。
25. `core/id_gen.go` — id 生成策略。
26. `core/planning.go` — Planning 抽象（与 `agent/plan/` 配合）。
27. `core/prompt_runner.go` — 直 LLM 调用（不走 action 调度）。
28. `core/service_provider.go` — 服务定位。
29. `core/tool_group.go` — `ToolGroup` / `ToolGroupRequirement` /
    `ToolRolesFor` / `StaticToolGroupResolver`（lyra 注册 tool 用）。

---

## 2. runtime/ — 实际执行

30. `runtime/doc.go` — 包说明。
31. `runtime/platform.go` **[精读]** — `Platform` / `PlatformConfig`。
32. `runtime/platform_deploy.go` / `platform_process.go` /
    `platform_run.go` / `platform_scope.go` — Deploy + Run 流程。
33. `runtime/registries.go` — 已部署 agent / extension 注册中心。
34. `runtime/extension.go` — Extension 装饰挂载。
35. `runtime/run.go` + `dispatch.go` + `execute_action.go` —
    `Tick` / `runWithRetry` / `runActionInterceptors`。
36. `runtime/agent_process.go` **[精读]** — `AgentProcess` 实现
    `core.Process`。
37. `runtime/agent_tool.go` — `AgentTool` decorator 装饰链。
38. `runtime/process_state.go` + `process_signals.go` — 状态机 +
    信号。
39. `runtime/process_budget.go` — 预算累加（subtree aggregation）。
40. `runtime/process_snapshot.go` — 快照（持久化用）。
41. `runtime/in_memory_blackboard.go` + `blackboard_determiner.go` —
    黑板 backend + 决策器。
42. `runtime/concurrent.go` + `child.go` + `subagent.go` —
    子进程 / 子代理调度。
43. `runtime/publish.go` — 事件 publish。
44. `runtime/mcp.go` — platform-level MCP 接入。
45. `runtime/autonomy/` — autonomy / LLM 排序的 helper（用于多候选
    action 自主选择）。

---

## 3. plan/ — 计划域

46. `plan/doc.go`
47. `plan/plan.go` — `Plan` 数据结构。
48. `plan/planning_system.go` — 计划系统入口。
49. `plan/condition_world_state.go` — 条件 → 世界状态映射。
50. `plan/planner.go` — `Planner` 接口。
51. `plan/planner/`
    - `reactive/` — 反应式（最简单，先读）
    - `utility/` — 效用函数
    - `htn/` — Hierarchical Task Network
    - `goap/` — Goal-Oriented Action Planning（最复杂）

---

## 4. workflow/ — 编排原语

52. `workflow/doc.go` — 包说明。
53. `workflow/workflow.go` + `types.go` — 基础类型。
54. `workflow/sequence.go` — 顺序。
55. `workflow/parallel.go` — 并行。
56. `workflow/loop.go` — 循环。
57. `workflow/repeat_until.go` / `repeat_until_acceptable.go` —
    重试 / 达标重复。
58. `workflow/scatter_gather.go` — 散播-汇集。
59. `workflow/consensus.go` — 共识。

---

## 5. event/ — 事件总线

60. `event/event.go` — 事件类型枚举。
61. `event/listener.go` + `multicast.go` — listener + 多播。
62. `event/process.go` / `execution.go` / `planning.go` /
    `platform.go` / `summaries.go` — 各域事件。

---

## 6. 其他

63. `toolpolicy/toolpolicy.go` — 工具调用策略（per-tool 限流 / 选择）。
64. `hitl/awaitable.go` + `tool.go` — Human-in-the-loop。
65. `examples/` — 端到端示例（blogllm / mcpagent 等），review 完抽象
    再回这里看用法。

---

## 跨模块提醒

- `Action` 的 typed 输入/输出绑定是该框架的"招牌" — 别破坏。
- `Process.RecordLLMInvocation` 当前**没人主动调** (chat middleware 没
  调用)，所以 `LLMInvocations()` 在大多数场景下是空 — Lyra 直接从 chat
  stream 累加 Usage 绕开了它。这是个待补的口子，review 时确认是否要
  保留。
- 子进程 / 子代理 (`subagent.go`) 是 embabel-style "agent 调 agent"
  的实现，注意预算/资源在父子之间的传递。
- `core.Guardrails` 与 `core.ToolGroupRequirement` 是 lyra 注册工具
  的入口。

## 体检命令

- `go test ./agent/...` — 全部应绿。
- `grep -l "RecordLLMInvocation" agent/` — 验证记录函数的调用面。
- `go vet ./agent/...`
