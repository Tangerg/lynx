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
| 后台命令执行句柄 | Shell | Process、Task、Job |
| 定时执行定义 | Schedule | Cron Task、Scheduled Job |

`Plan mode` 是编写和审批同一个 Plan 的 session-scoped 只读状态，不是第二种 Plan。Plan 在退出 Plan mode 后继续承担执行进度记录。

## 3. 命名规则

- 模型工具名统一使用 `lower_snake_case`；
- 动作工具使用 `verb_noun`，已形成稳定模型先验的 `read`、`write`、`edit`、`glob`、`grep`、`shell` 除外；
- 完整替换使用 `set_*`，局部修改才使用 `update_*`；
- 创建、读取、删除分别使用 `create_*`、`get_*`、`delete_*`；
- 模型报告判断使用 `report_*`，不得伪装成任意状态修改；
- 同类参数使用同一名称：文件路径统一 `path`，超时使用带单位的 `timeout_ms`，对象标识使用 `shell_id`、`schedule_id`、`result_id` 等领域限定名；
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

### `create_goal`

```json
{
  "objective": "Finish and verify the server-side tool migration",
  "budget": {
    "max_turns": 12,
    "max_cost_usd": 8,
    "max_steps": 80
  }
}
```

- `objective` 是必填的自然语言最终状态，不是 Plan、下一步或 UI 标题；
- `budget` 整体可选，只有用户明确给出限制时才传；三个限制均独立可选，零值/缺省表示该维度不设上限；
- 工具不接受 provider/model：自然语言入口使用 Runtime 周围的默认模型选择，避免把当前执行 Checkpoint 的模型字段复制进工具 context；
- 工具只在根 coding Agent 中出现，且描述明确禁止从普通 coding 请求推断自主执行授权；
- 调用时 Driver 持久化 Goal，并把唯一 loop 加入受 Host 管理的 task group。loop 通过 admission 的可取消事件等待当前 Run 连同 terminal maintenance 完全释放 session，并等待同 working tree 的 destructive mutation 结束，再竞争下一次 Run admission；不在工具调用栈内同步嵌套 Run，也不轮询或创建无所有者 goroutine；
- `WaitSessionStartable` 只是观察而非 reservation；若观察后被其他 Run 或 working-tree mutation 抢先，进程内 Gate 返回可识别的 `ErrRunAdmissionBusy`，Goal Driver 才重新等待。普通 `ErrSessionBusy` 或 durable conflict 不会被无限重试，而是按真实启动失败处理。

### `get_goal`

参数为空。返回 `goal` JSON；没有 Goal 时为 `null`。结果包括 `session_id`、`objective`、`status`、安全停止原因、模型、预算、使用量和时间戳，明确排除 lease 与 persistence revision：后两者是内部所有权机制，模型不能据此采取行动。

### `report_goal_outcome`

```json
{
  "outcome": "blocked",
  "reason": "Repository credentials must be supplied by the user"
}
```

- `outcome` 必填，只允许 `completed | blocked`，不再复用可任意修改状态的 `status` 参数；
- `reason` 仅在 `blocked` 时必填且必须是具体阻塞条件，`completed` 时省略；
- `completed` 只表示完整 objective 已实现并验证，不表示单个 Run、Plan 或部分步骤结束；
- 工具只在 active Goal 的根 coding Run 开始构造工具清单时出现；`create_goal` 和 `get_goal` 在启用 Goal 能力的普通根 Run 中可见；三个工具均不提供给子 Agent。

暂停、恢复、停止和预算调整属于用户或系统控制，不进入 `report_goal_outcome`。

## 6. 工具可见性

模型工具清单按每个 Run 的 session、角色、模式、模型能力和资源状态生成：

- **Direct**：当前 Run 直接可见；
- **Deferred**：可由 `search_tools` 查找并加载；
- **Hidden**：Runtime 可解析但模型不可见；
- **Unavailable**：当前 Run 不注册。

这四个词是行为分类，不强制对应一个通用 enum。当前实现只有 Direct 与 Deferred 构成真实的组装变化轴：Resolver 把两组工具注册到同一 Run，`agent/toolloop` 只读取 `search_tools` 已有的通用 `DeferredTool` 能力生成初始 manifest。Unavailable 继续用“不加入 Run registry”表达；当前没有内建 Hidden 工具，因此不为它创建无消费者的状态、接口或 registry 层。

可见性过滤不是授权。执行时仍按 permission action、session mode 和安全策略重新判定。

`enter_plan_mode` 与 `exit_plan_mode` 是一个例外：两者始终同时对根 coding Agent 可见，执行时分别校验当前 session 状态。Agent 的一次 Prompt 在开始时解析工具 registry，后续 tool rounds 复用该 registry；若按 Plan mode 动态隐藏其中一个，模型进入后同一轮无法退出。Runtime 不把应用状态泄露进 `agent` 核心来制造动态 registry 刷新。

初始 Direct 工具保持精简：文件读取与搜索、一个模型适配的文件修改族、shell、用户提问、Plan、Goal、委派和工具发现。网络、记忆、LSP、Skill、MCP、A2A、Schedule 等默认 Deferred 或按状态出现。

同一 Run 不得同时注册 `apply_patch` 与 `edit`/`write`，避免模型即使通过工具搜索也加载出两套重叠修改语言。模型 id 属于现代 GPT（排除 GPT-4 与 OSS）或 Grok 时使用 `apply_patch`，不依赖它经由原厂、OpenRouter 或兼容端点接入；Anthropic/Claude、Moonshot/Kimi、GPT-4、OSS 和未知模型保守使用 `edit` + `write`。

模型选择不进入持久化 `TurnScope`。fresh Run 在执行入口把显式选择写入临时 `executionctx`，恢复 Run 从已有 checkpoint 的独立 `ModelSelection` 字段重建同一临时值；Resolver 在没有显式值时使用 Runtime default。这避免了 checkpoint 字段重复，也没有把模型或 Exposure 注入 `agent` 模块。

`search_tools` 是 Deferred 能力唯一入口，不再是 MCP 专用工具：它同时索引 runtime 内建能力和连接的 integration。`query` 必填且非空，可用自然语言能力描述或 `select:name1,name2` 精确加载；`limit` 可选、范围 `1..20`、默认 `5`，精确选择时忽略。参数使用 typed function 的同一份严格 schema，拒绝未知字段；结果只提升匹配 definition，不改变执行权限。

### 委派、LSP 与 Schedule

- `delegate_task(summary,instructions)` 是唯一 Agent 委派入口。`summary` 是服务端生命周期真正消费的简短身份，`instructions` 是隔离 child Agent 的完整输入；不再使用含糊的名词工具 `task`，也不再暴露只为 UI 存在的 `description` 或模型实现词 `prompt`；
- `lsp(operation,...)` 是唯一语言服务器入口。`diagnostics` 与 definition/references/implementation/hover/call hierarchy/symbol operations 使用同一个闭集；文件参数统一为 `path`，位置操作同时要求 1-based `line` 与 `character`；
- Schedule 不再使用 `schedule(op=...)` 的互斥字段总集，而是 `list_schedules({})`、`create_schedule(instructions,cron,...)`、`delete_schedule(schedule_id)` 三个单动作 schema。模型侧删除 update：修改 schedule 必须显式删除并新建，前端/协议自己的 revisioned update use case 不受此工具面收敛影响。

### Shell、记忆与会话检索

- `shell(command,timeout_ms?,run_in_background?,auto_background_after_seconds?)` 是唯一命令启动入口；工具参数不再承载只供 UI 展示的 description。后台句柄统一称 Shell，并只通过 `shell_id` 标识；
- `read_shell_output(shell_id,wait?,timeout_ms?)` 增量读取一个后台 Shell 的输出；`wait=true` 使用完成事件等待，`timeout_ms` 只限制本次等待。`stop_shell(shell_id)` 只终止该 Shell；旧 `shell_output`、`shell_kill`、`block`、无单位 `timeout` 不保留；
- `search_memory(query,limit?)` 检索当前项目经过蒸馏的长期记忆；`search_conversations(query,limit?)` 检索过往会话原始 transcript。两者名称和描述显式区分 corpus，`limit` 均为 `1..20`，不再用 `memory_search` / `session_search` 这一组名词-动作倒置名称。

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

- `read_tool_result(result_id,offset_bytes?,limit_bytes?)` 是 offloaded result 唯一回读入口；标识与分页参数都携带领域或单位，单次最多读取 20000 bytes，preview 和续页提示直接给出同一 JSON 参数形状；旧 `id/offset/limit` 不保留；
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
| 3 | `create_goal` 与 idle continuation | 完成 |
| 4 | Manifest、Exposure、模型/状态驱动工具清单 | 完成 |
| 5 | 工具名、参数和描述的全量收敛 | 进行中（5a、5b.1、5b.2 完成） |
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

### 批次 3

- 将旧 `update_goal(status="complete|blocked")` 完整替换为 `report_goal_outcome(outcome="completed|blocked", reason?)`；工具名称、参数、enum、描述、Goal prompt、注释、安全分类和测试使用同一套报告语义，不保留别名或兼容字段；
- 新增 `create_goal(objective,budget?)` 和无参数 `get_goal`。`create_goal` 只响应用户明确的跨 Run 自主执行请求；`get_goal` 返回模型可操作的 Goal 投影，并剔除 lease/revision 内部机制；
- `create_goal` 通过 Goal Driver 的 `Start` 窄端口持久化并启动受 task group 所有的 loop；Goal Run 在 runs `WaitSessionStartable` 观察到当前 Run、pending opening、terminal maintenance 和 working-tree mutation 全部释放后才开始，且只对 Gate 标记的可恢复 admission 竞争重试；
- `get_goal` 始终对启用 Goal 的根 coding Agent 可见；`report_goal_outcome` 只在该 session 存在 active Goal 时可见；`create_goal` 在 Driver 构造完成后晚绑定到根 resolver。子 Agent 不获得任何 Goal 生命周期或状态报告能力；
- 晚绑定 seam 只传递通用 `tool.Tool`。Resolver 不持有 Driver，`agentexec.ToolResolver` 不新增 Goal 方法，`agent` 模块不知道 Goal、session admission 或 Runtime lifecycle；`create/get/report/active gate` 分别消费 `Starter/Reader/Reporter/ActiveReader` 单方法接口，只有 tool family 的 BuildConfig 组合为 `State`；没有引入 Bootstrap proxy、store facade、unowned goroutine 或复制的 Run 状态；
- admission/runs 只新增通用的 Run-startable 事件等待与可恢复 Gate 竞争分类，不知道 Goal。Goal application 只依赖 `WaitSessionStartable/Start/Cancel` Run 用例接口，不读取 Gate、registry 或 delivery DTO；边界扫描继续确认 `agent` 对 `app/runtime` 零导入，domain/application 对 adapter/infra/delivery/bootstrap 零反向依赖；
- 聚焦测试覆盖当前 Run 释放后启动、terminal maintenance 等待、context 取消、admission 丢失竞争重试、终态 CAS、Schema 词汇、根/子角色可见性和安全表可达性。

### 批次 4

- 将 resolver 的 Run 工具组装收敛为两个真实集合：Direct 进入初始 manifest，Deferred 保持可执行并由 `search_tools` 加载；资源或状态不可用的工具仍直接不注册。没有引入四态 enum、Hidden 空实现、第二个 registry 或 app-specific `tool.Tool` 字段；
- 初始清单保留文件读/搜、单一文件修改族、shell、提问、Plan/Goal、委派、结果回读和工具发现；将 network、LSP、Skill、MCP、A2A、memory/session search、Schedule 与 Skill authoring 统一转为 Deferred；
- 按每个 Run 的实际模型选择 mutation vocabulary：现代 OpenAI GPT/Codex 与 xAI/Grok 只注册 `apply_patch`，Claude/Kimi/GPT-4/OSS/未知模型只注册 `edit` + `write`。测试同时约束映射策略和“一次 Run 永不共存两族”；
- 模型选择以临时 `executionctx` 值从 fresh/restore 两个执行入口传播，Resolver 才读取；默认选择由 Bootstrap 组装传入。持久 `TurnScope`、Agent blackboard、通用 Tool 和 Agent Runtime API 均未扩张；
- 将 `search_tools` 从 MCP catalog 改为完整 Deferred catalog，按 runtime 或 MCP source 分组并公平排序；`query` 和 `limit` 改用 typed function 严格 schema，拒绝缺字段、未知字段与越界值；
- 继续复用 `agent/toolloop.DeferredTool` 与 `PromoteTools` 的通用 manifest 投影。`agent` 不知道 Plan、Goal、session、model provider、Exposure 策略或 runtime registry；边界变化仅发生在 app adapter/composition root。

### 批次 5a

- 将模型工具 `task(description,prompt)` 彻底替换为 `delegate_task(summary,instructions)`；同批修改 Agent deployment name、安全分类、approval/hook 特例、活动展示、测试模型和 catalog guard，不保留旧名或旧字段；
- `summary` 与 `instructions` 都由服务端消费并进入 typed `SubagentProjection`，分别驱动 child lifecycle identity 与隔离输入。删除“参数只给前端展示”的旧解释，避免 delivery concern 倒灌 Agent tool contract；
- 将 `lsp_diagnostics(path)` 合并为 `lsp(operation="diagnostics",path=...)`，完整删除第二个 tool definition 和安全表项；位置操作新增 line/character 同时必填且大于零的业务约束，schema 同步声明 minimum；
- 将四操作 `schedule(op=...)` 拆为 list/create/delete 三工具，各 schema 只包含真实消费字段；参数统一为 `instructions`、`workdir`、`schedule_id`，返回视图使用同一词汇；删除模型 update port 后，`ScheduleManagement` 同步缩窄为三个真实方法；
- Resolver 只接收 `[]ScheduleTools` 并按统一 Deferred 策略组装，不知道 Coordinator、revision 或 firing；Agent executor 的 resolver seam 从 `UseTaskTool` 精确更名为 `UseDelegationTool`，仍只传通用 `tool.Tool`。

### 批次 5b.1

- 将后台命令领域词汇统一为 Shell：`shell_output` / `shell_kill` 改为 `read_shell_output` / `stop_shell`，参数统一为 `shell_id`，删除与 Agent process tree 冲突的 Process 叫法；
- `shell` 删除仅供前端展示、服务端不消费的 `description`，将无单位 `timeout` / `auto_background_after` 改为 `timeout_ms` / `auto_background_after_seconds`；输出读取将 UI 控件式 `block` 改为行为语义 `wait`，等待超时也统一为 `timeout_ms`；
- 将 curated project memory 与 raw transcript 两个 corpus 分别命名为 `search_memory`、`search_conversations`，重写 definition 与参数描述，统一 `query` 和 `limit=1..20` 的严格 schema；旧名称和越界截断路径不保留；
- 改动只触及具体 toolset adapter、其 runtime 组装与基础 exec 注释；没有向通用 `tool.Tool`、Agent executor seam 或 delivery DTO 增加 Shell、Memory、Conversation 概念。

### 批次 5b.2

- 保留语义准确的 `read_tool_result` 工具名，将通用 `id/offset/limit` 收敛为 `result_id/offset_bytes/limit_bytes`，schema 直接约束 result identity 格式、非负 offset 与 `1..20000` 的单次读取上限；
- offload preview、工具 description、续页提示和测试都使用同一 JSON 参数形状；续页结果直接给出下一次 `result_id + offset_bytes`，不再让模型从自然语言单位中猜字段；
- 删除模型入口对负 offset、超大 limit 和旧字段的静默容忍；内部 `offload.ID`、SQLite store 与 artifact identity 保持领域内聚，不为参数重命名增加 DTO、兼容字段或跨模块 wrapper。
