# Tool System vNext

本文是 `app/runtime` 新工具体系的活文档，也是分批实施台账。代码、协议、提示词、持久化和桌面 UI 必须使用同一套领域词汇；不保留旧名称、别名字段或双路径兼容。

本轮只实施服务端。允许服务端 breaking wire change 暂时使前端接线失效，但不修改 `app/desktop`、TypeScript 或桌面协议文档；前端适配在服务端契约稳定后单独进行。

## 1. 目标

- 让每个模型可见工具都具备准确、单一且可预测的语义；
- 用状态和模型能力决定工具是否出现，避免同时暴露重叠能力；
- 把工具可见性、执行授权和运行生命周期分开；
- 允许模型通过自然语言请求直接进入 Plan mode 或创建 Goal；
- 在开发期直接修正公开 API、wire shape 和 store schema，不积累兼容债务。

## 2. 规范词汇

| 概念 | 唯一术语 | 不再使用 |
|---|---|---|
| 多步骤工作的有序执行状态 | Plan | Todo、Checklist、Task List |
| Plan 中的一项 | Step | Todo、Task、Item |
| 跨多个 Run 自主追求的目标 | Goal | Mission、Long Task |
| 委派给子 Agent 的工作 | Delegated Task | Process、Plan Step |
| 后台命令实例 | Process | Task、Job、Shell Task |
| 定时执行定义 | Schedule | Cron Task、Scheduled Job |

`Plan mode` 是编写和审批同一个 Plan 的 session-scoped 只读状态，不是第二种 Plan。Plan 在退出 Plan mode 后继续承担执行进度记录。

## 3. 命名规则

- 模型工具名统一使用 `lower_snake_case`；
- 动作工具使用 `verb_noun`，已形成稳定模型先验的 `read`、`write`、`edit`、`glob`、`grep`、`shell` 除外；
- 完整替换使用 `set_*`，局部修改才使用 `update_*`；
- 创建、读取、删除分别使用 `create_*`、`get_*`、`delete_*`；
- 模型报告判断使用 `report_*`，不得伪装成任意状态修改；
- 同类参数使用同一名称：文件路径统一 `path`，超时统一 `timeout_ms`，对象标识使用 `process_id` 等限定名；
- 参数名必须表达值本身，不表达 UI 控件或实现细节；
- 内建工具名称匹配 `^[A-Za-z][A-Za-z0-9_-]{0,63}$`；通用 `core/chat` 只执行 provider 共同要求的 `^[A-Za-z0-9_-]{1,64}$`，不得把内建命名风格强加给第三方 MCP 工具。

## 4. Plan 契约

### `enter_plan_mode`

参数为空。它只把当前 session 切换到只读 Plan mode，不创建或修改 Plan。进入不需要审批，因为它只收窄能力。

### `set_plan`

```json
{
  "steps": [
    {
      "description": "Implement session-scoped plan mode",
      "status": "in_progress"
    }
  ]
}
```

- `steps` 是必填的完整有序 Plan；每次调用替换原值；
- 空数组清空 Plan；
- `description` 是非空步骤描述；
- `status` 只允许 `pending | in_progress | completed`；
- 最多一个 Step 为 `in_progress`；
- 工具只修改 session Plan 状态，不修改 workspace，也不切换 Plan mode；
- Plan 状态逐轮注入上下文，因此不提供 `get_plan`。

### `exit_plan_mode`

参数为空。Runtime 读取持久化 Plan 并请求用户审批；空 Plan 时返回可恢复的模型错误。批准后恢复进入 Plan mode 前的 session permission mode，拒绝后保留 Plan mode。

## 5. Goal 契约

| 工具 | 语义 |
|---|---|
| `create_goal` | 仅在用户明确要求自主执行时创建持久 Goal |
| `get_goal` | 读取当前 Goal、预算和使用量 |
| `report_goal_outcome` | 模型只报告 `completed` 或 `blocked` 终态 |

`create_goal` 在当前 Run 中只持久化状态并记录启动意图。当前 Run 结束且 session admission 释放后，idle lifecycle 才启动第一个 Goal Run；工具调用不得同步嵌套启动 Run，也不得自行创建无所有者 goroutine。

暂停、恢复、停止和预算调整属于用户或系统控制，不进入 `report_goal_outcome`。

## 6. 工具可见性

模型工具清单按每个 Run 的 session、角色、模式、模型能力和资源状态生成：

- **Direct**：当前 Run 直接可见；
- **Deferred**：可由 `search_tools` 查找并加载；
- **Hidden**：Runtime 可解析但模型不可见；
- **Unavailable**：当前 Run 不注册。

可见性过滤不是授权。执行时仍按 permission action、session mode 和安全策略重新判定。

`enter_plan_mode` 与 `exit_plan_mode` 是一个例外：两者始终同时对根 coding Agent 可见，执行时分别校验当前 session 状态。Agent 的一次 Prompt 在开始时解析工具 registry，后续 tool rounds 复用该 registry；若按 Plan mode 动态隐藏其中一个，模型进入后同一轮无法退出。Runtime 不把应用状态泄露进 `agent` 核心来制造动态 registry 刷新。

初始 Direct 工具保持精简：文件读取与搜索、一个模型适配的文件修改族、shell、用户提问、Plan、Goal、委派和工具发现。网络、记忆、LSP、Skill、MCP、A2A、Schedule 等默认 Deferred 或按状态出现。

同一模型不得同时看到 `apply_patch` 与 `edit`/`write`：支持结构化 patch 的模型只获得 `apply_patch`，其他模型获得 `edit` 和 `write`。

## 7. 删除与收敛

### 完全移除

- `download` 及其 allowlist 配置；
- `sourcegraph_search` 及 Sourcegraph endpoint/token 配置；
- 内建 `propose_skill`，由 Skill authoring 工作流取代；
- Todo 领域、工具、store、wire 和 UI 术语。

### 从模型工具面移除

- `codebase_search`；代码索引仅在仍有前端消费方时保留；
- 共享 session Plan store 的子 Agent 写入口；
- 普通子 Agent 的递归委派入口。

### 合并或拆分

- `lsp_diagnostics` 合并进 `lsp(operation="diagnostics")`；
- 多操作 `schedule` 拆成 `create_schedule`、`list_schedules`、`delete_schedule`；
- `task` 改为 `delegate_task`；
- `update_goal` 改为 `report_goal_outcome`；
- `skill(op=...)` 收敛为语义明确的 Skill 加载操作；
- 不新增与 Plan 重叠的 Todo 或 `update_plan` 工具。

## 8. Schema 与结果契约

- 模型工具定义只承载 provider 共同消费的 object input schema；不把 provider 不消费的 output schema 塞进 `core/chat.ToolDefinition`；
- object schema 默认拒绝未知字段，非 `omitempty` / `omitzero` 字段默认 required；
- schema 支持 enum、字符串长度、数值范围、正则和数组范围，并由 typed function tool 在调用边界执行同一份约束；
- 跨字段业务不变量继续由具体工具验证，不为尚不存在的消费者扩张通用 schema DSL；
- 工具输出保持各模型协议共同支持的文本结果；结构化结果可以编码为 JSON 文本，但在出现至少两个真实 output-schema 消费者前不提升为通用协议；
- 普通工具错误由 `agent/toolloop` 结算为 `ToolResult.IsError`，不再增加第二套失败类型；approval、question 和 lifecycle interrupt 仍是控制流；
- 输出裁剪与 offload 由统一 settlement 边界负责，不进入 `tool.Tool`。

## 9. `agent` 与 `app/runtime` 边界

抽象以真实消费者为依据，不以“统一工具体系”为理由机械上提或下沉：

- `agent` 只拥有所有 Agent Runtime 消费者都必须理解的工具调用、并发和结果原语；
- Plan、Goal、session、approval、store、模型选择和工具 Exposure 是产品语义，只能留在 `app/runtime`；
- `app/runtime` 不复制 `agent` 已有的调用循环、并发键或结果传播机制；
- 跨边界接口由消费方定义，并且只包含该消费方实际调用的方法；
- 禁止跨边界传递 `*Engine`、store、registry、授权闭包、delivery DTO 或持久化结构；
- 通用 `tool.Tool` 不承载某个应用才使用的 manifest、permission 或 lifecycle 字段；
- 只有所有 Tool 消费者都必须遵守的 input/output 定义与调用结果契约，才允许沉入通用工具核；
- 每批用 import、公开 API、字段和构造参数扫描检查泄露，不以新增 facade 或胖 SPI 掩盖依赖。

判断一个抽象是否合理时同时回答三问：是否已有两个真实消费者、是否消除了一个实际变化轴、是否迫使无关消费者理解产品概念。前两项都否定则是过度抽象；最后一项肯定则是边界泄露。

## 10. 实施批次

| 批次 | 内容 | 状态 |
|---|---|---|
| 0 | 固化词汇、契约、删除范围和服务端实施台账 | 完成 |
| 1 | 工具 Definition、schema、result 和错误基础协议 | 完成 |
| 2a | Plan 领域、持久化、wire、归档和工具替换 | 完成 |
| 2b | session-scoped Plan mode | 完成 |
| 3 | `create_goal` 与 idle continuation | 待开始 |
| 4 | Manifest、Exposure、模型/状态驱动工具清单 | 待开始 |
| 5 | 工具名、参数和描述的全量收敛 | 待开始 |
| 6 | 删除冗余能力和配置 | 待开始 |
| 7 | 全仓验证、文档收敛和最终审计 | 待开始 |

每批必须独立验证、独立提交并推送。实现发现契约需要调整时，先更新本文，再在同一批修改代码和测试。

## 11. 变更记录

### 批次 0

- 选择 Plan 作为多步骤工作唯一术语；
- 确定 `enter_plan_mode` / `set_plan` / `exit_plan_mode` 三工具边界；
- 确定 Goal 自然语言入口与 idle continuation；
- 确定工具命名、参数、描述、可见性和删除规则。
- 将本轮实施范围限定为服务端，前端接线延后为独立工作。
- 将 `agent` / `app/runtime` 抽象充分性与泄露审计纳入每批验收。

### 批次 1

- `core/chat.ToolDefinition` 统一验证 provider 共同接受的名称字符集、64 字符上限和 object input schema；
- typed function schema 从 Go JSON 语义推导 required / optional，并新增字符串、数值、正则和数组约束；
- typed function 调用在解码前执行同一份派生 schema，缺字段、未知字段和越界参数成为可恢复的工具错误；
- 保持 `tool.Tool` 两方法最小接口，复用 `agent/toolloop` 已有的 `ToolResult.IsError` 结算；
- 否决通用 output schema 和新 error-code SPI：当前 provider 没有共同消费方，引入会造成过度抽象；
- 参数校验只依赖标准库，根模块继续零外部依赖，`tool` 继续只依赖 Core。

### 批次 2a

- 以 `Plan -> Step{description,status}` 作为唯一多步骤状态模型，完整删除当前服务端中的 Todo 包、store、工具、query、feature 和 wire 名称；
- `set_plan` 使用必填完整 `steps` 数组做 whole replacement，空数组清空，领域层保证描述非空、状态闭集且最多一个 `in_progress`；
- `set_plan` 只对根 coding Agent 可见；子 Agent 可读取注入提示中的当前 Plan，但不能替换共享 Plan；
- Plan 随 session 的 fork、rollback、export/import 和 delete 生命周期移动，独立 revision 保证 state snapshot 单调；artifact 提升到 v10，旧 artifact 直接拒绝，不增加迁移或别名；
- 删除未被执行路径产生的 transcript plan Item、plan delta 及其第二套 `running/failed` Step 状态，只保留 session Plan 一套语义；
- `app/runtime` 的根模块依赖提升到已推送的工具 schema 版本，保证 workspace 与 `GOWORK=off` 使用同一份 input contract；
- 服务端 contract 和 Go wire validator 已重新生成；按本轮范围未修改桌面端 TypeScript、canonical sample 或协议文档，其 drift 留给服务端工具契约稳定后的前端专项；
- 边界审计确认 Plan 领域留在 `app/runtime`，`agent` 模块没有导入 runtime；tool adapter 仅依赖消费方定义的 `List/Replace` 或只读 `State` 窄端口，没有把 SQLite、delivery DTO、session aggregate 或 runtime Engine 泄露进 Agent 核心。

### 批次 2b

- 将 `safe | balanced | yolo` 收敛为 runtime default permission mode；`plan` 不再是 `approval.getMode/setMode` 可读写的全局值，而是持久化的 session overlay；
- 新增无参数 `enter_plan_mode`：只收窄当前 session 权限并捕获进入时的精确 mode；重复进入幂等，不覆盖最初恢复值；
- 将 `exit_plan_mode` 改为无参数：只读取并展示 canonical session Plan，不再接受第二份 `plan` 文本或 `options`，避免“Agent 状态”和“待审批文本”分叉；拒绝保持 Plan mode，批准恢复进入时捕获的 mode；
- Plan mode 通过 `session_permission_modes` 持久化并由 session foreign key 管理生命周期；默认 mode 在进出期间改变，也不会改变该 session 的精确恢复结果；
- 三个 Plan 工具只对根 coding Agent 可见，子 Agent 无进入、退出或共享 Plan 写权限；`enter_plan_mode`、`set_plan`、`exit_plan_mode` 均归入无需外层 approval 的 safe control operation，其中退出工具自己拥有一次明确审批；
- 删除最初尝试的 mode-driven resolver gate：一次 Agent Prompt 不会在 tool rounds 之间重建 registry，保留该 gate 会导致进入后无法退出。两个 mode 工具改为同时可见并在执行边界校验状态，没有为产品状态修改 `agent` 核心；
- 边界审计确认 permission policy、Plan mode store、Plan tool adapters 和 delivery mapping 全部留在 `app/runtime`；turn 只依赖 `Mode(ctx, sessionID)`，工具只依赖 `EnterPlanMode`、`ExitPlanMode`、`List` 窄端口，SQLite 实现未穿透到 resolver 或 Agent；
- 服务端 schema 与 Go validator 已重新生成；仍按本轮范围不修改桌面 TypeScript 和 canonical samples，因此完整 drift 测试继续只报告已记录的前端接线差异。
