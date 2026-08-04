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
| 委派给其他 Agent 的工作 | Delegated Task | Process、Plan Step、Subtask |
| 后台命令执行句柄 | Shell | Process、Task、Job |
| 定时执行定义 | Schedule | Cron Task、Scheduled Job |
| 可复用的 Agent 指令集合 | Skill | Prompt Template、Recipe |
| 等待人工评审的不可变 Skill 内容 | Skill Proposal | Skill Draft、Candidate |
| Proposal 评审通过/拒绝 | Approve / Reject | Promote、Publish、Discard |
| Skill 所属范围 | project / user | global、workspace-global |

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
- 工具只在根 Agent 中出现，且描述明确禁止从普通请求推断自主执行授权；
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
- 工具只在 active Goal 的根 Run 开始构造工具清单时出现；`create_goal` 和 `get_goal` 在启用 Goal 能力的普通根 Run 中可见；三个工具均不提供给委派 Agent。

暂停、恢复、停止和预算调整属于用户或系统控制，不进入 `report_goal_outcome`。

## 6. 工具可见性

模型工具清单按每个 Run 的 session、角色、模式、模型能力和资源状态生成：

- **Direct**：当前 Run 直接可见；
- **Deferred**：可由 `search_tools` 查找并加载；
- **Hidden**：Runtime 可解析但模型不可见；
- **Unavailable**：当前 Run 不注册。

这四个词是行为分类，不强制对应一个通用 enum。当前实现只有 Direct 与 Deferred 构成真实的组装变化轴：Resolver 把两组工具注册到同一 Run，`agent/toolloop` 只读取 `search_tools` 已有的通用 `DeferredTool` 能力生成初始 manifest。Unavailable 继续用“不加入 Run registry”表达；当前没有内建 Hidden 工具，因此不为它创建无消费者的状态、接口或 registry 层。

可见性过滤不是授权。执行时仍按 permission action、session mode 和安全策略重新判定。

`enter_plan_mode` 与 `exit_plan_mode` 是一个例外：两者始终同时对根 Agent 可见，执行时分别校验当前 session 状态。Agent 的一次 Prompt 在开始时解析工具 registry，后续 tool rounds 复用该 registry；若按 Plan mode 动态隐藏其中一个，模型进入后同一轮无法退出。Runtime 不把应用状态泄露进 `agent` 核心来制造动态 registry 刷新。

初始 Direct 工具保持精简：文件读取与搜索、一个模型适配的文件修改族、shell、用户提问、Plan、Goal、委派和工具发现。网络、记忆、LSP、Skill、MCP、A2A、Schedule 等默认 Deferred 或按状态出现。

同一 Run 不得同时注册 `apply_patch` 与 `edit`/`write`，避免模型即使通过工具搜索也加载出两套重叠修改语言。模型 id 属于现代 GPT（排除 GPT-4 与 OSS）或 Grok 时使用 `apply_patch`，不依赖它经由原厂、OpenRouter 或兼容端点接入；Anthropic/Claude、Moonshot/Kimi、GPT-4、OSS 和未知模型保守使用 `edit` + `write`。

模型选择不进入持久化 `TurnScope`。fresh Run 在执行入口把显式选择写入临时 `executionctx`，恢复 Run 从已有 checkpoint 的独立 `ModelSelection` 字段重建同一临时值；Resolver 在没有显式值时使用 Runtime default。这避免了 checkpoint 字段重复，也没有把模型或 Exposure 注入 `agent` 模块。

`search_tools` 是 Deferred 能力唯一入口，不再是 MCP 专用工具：它同时索引 runtime 内建能力和连接的 integration。`query` 必填且非空，可用自然语言能力描述或 `select:name1,name2` 精确加载；`limit` 可选、范围 `1..20`、默认 `5`，精确选择时忽略。参数使用 typed function 的同一份严格 schema，拒绝未知字段；结果只提升匹配 definition，不改变执行权限。

### 最终内建工具面

| 能力 | 保留的模型工具 | 可见性规则 |
|---|---|---|
| 文件读取与搜索 | `read`、`glob`、`grep` | Direct |
| 文件修改 | `apply_patch` **或** `edit` + `write` | 按模型 profile 互斥 Direct |
| Shell 生命周期 | `shell`、`read_shell_output`、`stop_shell` | Direct |
| Plan | `enter_plan_mode`、`set_plan`、`exit_plan_mode` | 根 Agent Direct |
| Goal | `create_goal`、`get_goal`、`report_goal_outcome` | 根 Agent；report 仅 active Goal |
| 编排与交互 | `delegate_task`、`ask_user` | 根/委派 Agent 按 resolver policy |
| 工具渐进披露 | `search_tools`、`read_tool_result` | 有 deferred/offloaded 资源时出现 |
| 代码智能 | `lsp` | Deferred |
| 网络 | `web_search`、`web_fetch`、`http_request` | 配置可用时 Deferred |
| 记忆与历史 | `search_memory`、`search_conversations` | 对应资源可用时 Deferred |
| Skill 读取 | `list_skills`、`load_skill`、`read_skill_resource` | 根/委派 Agent Deferred |
| Skill 提案 | `propose_skill` | 仅根 Agent、Deferred；只在用户明确要求保存工作流时调用 |
| Schedule | `list_schedules`、`create_schedule`、`delete_schedule` | Schedule subsystem 可用时 Deferred |
| 外部集成 | 动态 MCP / A2A 工具 | 连接可用时 Deferred |

这里的“保留”表示能力边界不同，不表示所有工具应同时进入初始 manifest：web search 查找来源，web fetch 阅读页面，HTTP request 调用 allowlisted API；Plan 记录当前请求步骤，Goal 管理跨 Run 自主目标，Schedule 表达重复执行。它们没有共享一个真实变化轴，不合并为含互斥字段的总工具。

### 委派、LSP 与 Schedule

- `delegate_task(summary,instructions)` 是唯一 Agent 委派入口。`summary` 是服务端生命周期真正消费的简短身份，`instructions` 是隔离 child Agent 的完整输入；不再使用含糊的名词工具 `task`，也不再暴露只为 UI 存在的 `description` 或模型实现词 `prompt`；
- `lsp(operation,...)` 是唯一语言服务器入口。`diagnostics` 与 definition/references/implementation/hover/call hierarchy/symbol operations 使用同一个闭集；文件参数统一为 `path`，位置操作同时要求 1-based `line` 与 `character`；
- Schedule 不再使用 `schedule(op=...)` 的互斥字段总集，而是 `list_schedules({})`、`create_schedule(instructions,cron,...)`、`delete_schedule(schedule_id)` 三个单动作 schema。模型侧删除 update：修改 schedule 必须显式删除并新建，前端/协议自己的 revisioned update use case 不受此工具面收敛影响。

### Shell、记忆与会话检索

- `shell(command,description,timeout_ms?,run_in_background?,auto_background_after_seconds?)` 是唯一命令启动入口；`description` 是服务端 activity 真正消费的、最多 120 字符的动作短语，不是无消费者的 UI 标签。后台句柄统一称 Shell，并只通过 `shell_id` 标识；
- `read_shell_output(shell_id,wait?,timeout_ms?)` 增量读取一个后台 Shell 的输出；`wait=true` 使用完成事件等待，`timeout_ms` 只限制本次等待。`stop_shell(shell_id)` 只终止该 Shell；旧 `shell_output`、`shell_kill`、`block`、无单位 `timeout` 不保留；
- `search_memory(query,limit?)` 检索当前项目经过蒸馏的长期记忆；`search_conversations(query,limit?)` 检索过往会话原始 transcript。两者名称和描述显式区分 corpus，`limit` 均为 `1..20`，不再用 `memory_search` / `session_search` 这一组名词-动作倒置名称。

### 文件与本地搜索

- `read`、`write`、`edit` 的文件参数统一为 `path`，与 `glob`、`grep`、`lsp` 一致；`apply_patch` 的每文件结果也返回 `path`。旧 `file_path` 不保留；
- `read(path,start_line?,max_lines?)` 使用 1-based `start_line` 和显式行数上限；删除把 0/default 与 1-based 行号混在同一字段里的 `offset`，也不再用无单位 `limit`；
- `write(path,content)` 只表达创建或完整替换。删除与 `edit` 重叠且会绕开完整读取保护的 `append` 入口；底层 Executor 仍可为非模型消费者保留 append 原语；
- `grep` 只保留 `before_context_lines` / `after_context_lines`，删除同时表达相同状态的 `context` shortcut；文件过滤使用 `file_glob` / `file_type`，结果上限使用与 `glob` 一致的 `max_results`，`output_mode` 是 `content | files_with_matches | count` 闭集；
- 六个 filesystem 工具的外层继续拥有 concurrency / mutation-path 等真实附加能力，但 Definition 与 Call 都委托给同一个 typed function contract。未知字段、缺失字段和越界值不再被手写 `json.Unmarshal` 静默接受。

### Skill 渐进披露

- `list_skills({})` 只列出当前 workspace 可见 Skill 的 name + description；`load_skill(name)` 只加载一个 Skill 的完整指令；`read_skill_resource(name,path)` 只读取已加载指令引用的 bundled resource，绝不执行脚本；
- 三个工具都保持 Deferred，并由同一个 working-directory-scoped source 提供数据，但各自拥有严格的单动作 schema。删除 `skill(op=list|load|load_resource,name?,path?)` 及其条件必填、忽略字段和 dispatch 分支；
- Skill usage 只在 `load_skill` 真正加载 user library Skill 时记录；list 与 resource read 不伪装成一次 instruction use。

### Skill Proposal 与 `propose_skill`

```json
{
  "name": "review-go-api",
  "description": "Review a Go API before implementation. Use when a design changes exported behavior.",
  "instructions": "Read the design, inspect consumers, and report compatibility risks.",
  "scope": "project"
}
```

- `propose_skill` 是 Skill 自然语言创作的唯一模型入口，只在用户明确要求保存、创建或沉淀一个可复用工作流时调用；“这个流程看起来有用”不构成授权；
- 参数只包含模型真正决定的 `name / description / instructions / scope`。`scope` 是闭集：`project` 仅属于当前 workspace，`user` 跨 workspace 属于当前用户；
- `name` 是稳定的 lowercase-hyphenated identifier；`description` 同时说明“做什么”和“何时使用”；`instructions` 是未来 Agent 可独立执行的完整指令，不携带本次进度、瞬时环境或 secret；
- session、cwd、origin 由执行上下文补齐，绝不作为模型参数。前台工具固定写入 `origin=requested`；后台 Miner 固定写入 `origin=mined`；
- 工具只提交不可变 Proposal，并返回 `pending_review + scope + name + revision`。它不 interrupt、不 approve、不 activate，也不复用前端交互作为调用前置条件；
- Proposal 按内容寻址并存入目标 library 的 `_proposals/<revision>/SKILL.md`。`revision` 同时覆盖 scope、name 和完整文件内容，评审操作不会误作用于后来变化的内容；
- 服务端评审面唯一使用 `skills.proposals.list / approve / reject`。list 必须带 workspace 并返回完整 description、instructions、scope、origin、source session 和 revises；approve/reject 必须携带 workspace、scope、name、revision；
- `Draft / promote / discard / global` 已从 Skill 领域、存储、应用和服务端协议移除，不提供别名或兼容转换；
- 显式用户创作与自动 Miner 共享应用层 `SubmitSkillProposal`，但触发策略不同：工具只响应明确用户意图；Miner 只在复杂轨迹达到 cadence 时自主蒸馏，并默认提交 user Proposal。两者都不能直接激活 Skill；
- Tool adapter 只依赖单方法 `ProposalSubmitter`，Miner 也只依赖同一单方法应用能力；project/user 文件布局由 adapter 路由，Application 不认识 `.lyra`，Agent 核心只接收通用 `tool.Tool`。

## 7. 删除与收敛

### 完全移除

- `download` 及其专属组装、路径写入与 allowlist 派生；通用 HTTP allowlist 继续只服务 `http_request`；
- `sourcegraph_search` 及 Sourcegraph endpoint/token 配置；
- 旧的组合式 Skill authoring 路径（stage + HITL + promote/discard）；
- `SkillDraft`、`_drafts`、`skills.drafts.*` 以及 promote/discard 术语；
- Todo 领域、工具、store、wire 和 UI 术语。

### 从模型工具面移除

- `codebase_search`；代码索引仅在仍有前端消费方时保留；
- 共享 session Plan store 的委派 Agent 写入口；
- 普通委派 Agent 的递归委派入口。

### 合并或拆分

- `lsp_diagnostics` 合并进 `lsp(operation="diagnostics")`；
- 多操作 `schedule` 拆成 `create_schedule`、`list_schedules`、`delete_schedule`；
- `task` 改为 `delegate_task`；
- `update_goal` 改为 `report_goal_outcome`；
- `skill(op=...)` 拆为 `list_skills`、`load_skill`、`read_skill_resource`；
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

### `toolset` 代码组织

- `toolset` 已经隐含模型工具语境，子包必须按能力命名为 `goal / plan / skill / lsp / offload / discovery` 等，不再使用 `goaltool / lsptools / toolresult / toolsearch` 这类重复后缀或前缀；
- 同一领域、同一状态源并共同构成生命周期的模型工具放在一个包中，但仍保持独立工具名和严格单动作 schema。Plan 的 enter/set/exit 属于一个 `plan` 包，Skill 的 read/propose 属于一个 `skill` 包，Schedule 的 list/create/delete 属于一个 `schedule` 包；
- 父包只保留组装、Resolver、Exposure 以及跨文件工具的 guard/decorator。只被父包消费的 read tracker 不建立子包，也不为制造包边界导出类型；
- 不能因为动词相似就机械合并。`search_memory` 与 `search_conversations` 使用不同 corpus、不同 application port 和不同结果语义，继续分包；`ask_user` 与 Plan approval 同为 interrupt，但业务授权与生命周期不同，也继续分包；
- 一个子包必须对应真实独立能力或生命周期，目录为空、只剩转发、或必须以 `Impl/Tool/Tools` 才能解释职责时，应删除、折回父包或重新命名。

## 10. 实施批次

| 批次 | 内容 | 状态 |
|---|---|---|
| 0 | 固化词汇、契约、删除范围和服务端实施台账 | 完成 |
| 1 | 工具 Definition、schema、result 和错误基础协议 | 完成 |
| 2a | Plan 领域、持久化、wire、归档和工具替换 | 完成 |
| 2b | session-scoped Plan mode | 完成 |
| 3 | `create_goal` 与 idle continuation | 完成 |
| 4 | Manifest、Exposure、模型/状态驱动工具清单 | 完成 |
| 5 | 工具名、参数和描述的全量收敛 | 完成 |
| 6 | 删除冗余能力和配置 | 完成（6a、6b、6c） |
| 7 | 全仓验证、文档收敛和最终审计 | 完成（7a—7g） |
| 8a | Skill Proposal 领域、双作用域存储、应用用例与服务端评审协议 | 完成 |
| 8b | 根 Agent 自然语言 `propose_skill` 与统一 Miner 提交链 | 完成 |
| 8c | 桌面前端 Proposal 接线与新 canonical samples | 延后到前端专项 |
| 9a | Toolset 包名、文件名、角色残留词与空目录清洗 | 完成 |
| 9b | Plan/Skill/Schedule 协作包收敛、私有 tracker 回收与测试就近组织 | 完成 |

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
- `set_plan` 只对根 Agent 可见；委派 Agent 可读取注入提示中的当前 Plan，但不能替换共享 Plan；
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
- 三个 Plan 工具只对根 Agent 可见，委派 Agent 无进入、退出或共享 Plan 写权限；`enter_plan_mode`、`set_plan`、`exit_plan_mode` 均归入无需外层 approval 的 safe control operation，其中退出工具自己拥有一次明确审批；
- 删除最初尝试的 mode-driven resolver gate：一次 Agent Prompt 不会在 tool rounds 之间重建 registry，保留该 gate 会导致进入后无法退出。两个 mode 工具改为同时可见并在执行边界校验状态，没有为产品状态修改 `agent` 核心；
- 边界审计确认 permission policy、Plan mode store、Plan tool adapters 和 delivery mapping 全部留在 `app/runtime`；turn 只依赖 `Mode(ctx, sessionID)`，工具只依赖 `EnterPlanMode`、`ExitPlanMode`、`List` 窄端口，SQLite 实现未穿透到 resolver 或 Agent；
- 服务端 schema 与 Go validator 已重新生成；仍按本轮范围不修改桌面 TypeScript 和 canonical samples，因此完整 drift 测试继续只报告已记录的前端接线差异。

### 批次 3

- 将旧 `update_goal(status="complete|blocked")` 完整替换为 `report_goal_outcome(outcome="completed|blocked", reason?)`；工具名称、参数、enum、描述、Goal prompt、注释、安全分类和测试使用同一套报告语义，不保留别名或兼容字段；
- 新增 `create_goal(objective,budget?)` 和无参数 `get_goal`。`create_goal` 只响应用户明确的跨 Run 自主执行请求；`get_goal` 返回模型可操作的 Goal 投影，并剔除 lease/revision 内部机制；
- `create_goal` 通过 Goal Driver 的 `Start` 窄端口持久化并启动受 task group 所有的 loop；Goal Run 在 runs `WaitSessionStartable` 观察到当前 Run、pending opening、terminal maintenance 和 working-tree mutation 全部释放后才开始，且只对 Gate 标记的可恢复 admission 竞争重试；
- `get_goal` 始终对启用 Goal 的根 Agent 可见；`report_goal_outcome` 只在该 session 存在 active Goal 时可见；`create_goal` 在 Driver 构造完成后晚绑定到根 resolver。委派 Agent 不获得任何 Goal 生命周期或状态报告能力；
- 晚绑定 seam 只传递通用 `tool.Tool`。Resolver 不持有 Driver，`agentexec.ToolResolver` 不新增 Goal 方法，`agent` 模块不知道 Goal、session admission 或 Runtime lifecycle；`create/get/report/active gate` 分别消费 `Starter/Reader/Reporter/ActiveReader` 单方法接口，只有 tool family 的 BuildConfig 组合为 `State`；没有引入 Bootstrap proxy、store facade、unowned goroutine 或复制的 Run 状态；
- admission/runs 只新增通用的 Run-startable 事件等待与可恢复 Gate 竞争分类，不知道 Goal。Goal application 只依赖 `WaitSessionStartable/Start/Cancel` Run 用例接口，不读取 Gate、registry 或 delivery DTO；边界扫描继续确认 `agent` 对 `app/runtime` 零导入，domain/application 对 adapter/infra/delivery/bootstrap 零反向依赖；
- 聚焦测试覆盖当前 Run 释放后启动、terminal maintenance 等待、context 取消、admission 丢失竞争重试、终态 CAS、Schema 词汇、根/委派角色可见性和安全表可达性。

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

### 批次 5b.3

- 将 `read/write/edit` 输入和 `apply_patch` 文件结果统一为 `path`，同步 mutation guard、并发键、approval subject、diagnostics、展示投影和 server 测试；待删除的 `download` 不做过渡性重命名；
- 从模型 `write` 删除 append，现有文件的完整替换一律执行 full-read guard；Executor 的 append 字段留在 filesystem SPI 内部，没有为了工具面收敛破坏非模型消费者；
- 收敛 `grep` 参数为 `file_glob/file_type/before_context_lines/after_context_lines/output_mode/max_results`，删除 `context/head_limit/glob/type` 多义或重叠字段，并为 context、result cap 和 output enum 增加严格边界；
- 文件工具外壳复用通用 `tool.Func` 的同一份 schema + decoder + result encoder，只额外实现 concurrency 与 mutation-path capability；没有把 runtime guard、working directory 或 permission 概念下沉到 `tool` 或 `tools/fs` Executor。

### 批次 5b.4

- 将三操作 `skill(op=...)` 完整拆为 `list_skills({})`、`load_skill(name)`、`read_skill_resource(name,path)`；删除 `op`、条件必填参数、operation constants、dispatch switch 和仅服务旧总集的 errors；
- `tools/skills` 继续只依赖只读 `skills.ResourceSource`，一个具体 `toolSet` 直接承载三个 typed function，没有新增 registry、operation framework 或 runtime-facing interface；
- Runtime 的 working-directory adapter 返回三工具切片并统一标为 Deferred，Resolver 只展开通用 `[]tool.Tool`；usage recorder 仍通过 Source decorator 只观察成功的 user `Load`，没有进入模型契约；
- 安全表以三个真实名称替换旧 `skill`，catalog completeness guard 同时约束可达性；通用 Agent、ToolDefinition 与 Skill repository 接口均未增加 app/runtime 概念。

### 批次 6a

- 从服务端源码完整删除 `download` definition、实现、测试、resolver 字段、BuildConfig 派生 allowlist、workdir tool family、审批 subject 和安全分类；不保留 disabled registration、旧名称或兼容参数；
- `download` 原本把任意 URL GET 与 workspace 写入重新组合，重复了 `http_request`、`write` / `shell` 已有能力，并产生第二套 SSRF gate、路径锁、overwrite 规则和写审批身份；删除组合工具后，模型用单一原语显式完成网络读取与受保护写入；
- 保留 `online.httpAllowedHosts` / `LYRA_HTTP_ALLOWED_HOSTS`，但其唯一模型消费者现在是 `http_request`。这不是 `download` 兼容配置，不再在 composition root 派生另一份 allowlist；
- 从服务端源码完整删除 `sourcegraph_search`、stream parser、条件注册、测试和 `online.sourcegraphEndpoint/sourcegraphToken` 及 `LYRA_SOURCEGRAPH_*` 配置；不保留 vendor-specific hidden tool；
- Sourcegraph JSON-RPC Go 依赖继续由 LSP transport 合法消费，没有因名称相同而机械删除。`agent` 与通用工具模块没有新增网络、workspace 或 provider 概念；runtime 的 OnlineConfig 反而收窄为三个真实能力字段。

### 批次 6b

- 从模型工具面完整删除 `codebase_search` package、definition、schema、resolver 动态 availability gate、BuildConfig/Deps 端口、安全分类和专属测试；不以 Hidden、disabled registration 或 MCP alias 保留第二条代码搜索入口；
- Agent 已有 `grep`、`glob`、`read` 与 `shell`，能在当前 checkout 上形成可观察、可组合的代码检索链路；删除语义索引工具避免模型在 exact/local search 与 opaque embedding ranking 间无谓选择，也让工具 manifest 少一个依赖用户 embedding 配置的变化轴；
- 保留 `domain/codebaseindex`、SQLite index store、application `codebase.Coordinator` 及 `codebase.search/status/reindex` delivery contract：它们仍被客户端 `@codebase` mention、状态和手动重建表面实际消费；
- Bootstrap 只把 semantic index 交给 application codebase use case，不再额外适配为 `toolset.CodebaseIndex` 并穿过 tool environment builder。移除这个双用途端口后，toolset 不认识 embedding role、index availability 或 client codebase lifecycle，边界更窄而非新增 facade。

### 批次 6c

- 当时从服务端模型工具面删除了旧 `propose_skill`：该实现把 stage、HITL interrupt、promote/discard 合在一次工具调用中，既重复后台 Miner，又让模型拥有不清晰的发布路径；
- 同批保留后台 SkillMiner、usage curator 与客户端评审面，Toolset 暂时只消费只读 Skill source 与 usage recorder；
- **本批结论已被批次 8 的新需求取代。** 当前实现保留同名但全新单一职责的 `propose_skill`：只响应用户明确创作意图并提交 Proposal，不再恢复旧 HITL 或发布行为；旧 Draft/promote/discard 领域仍然彻底删除。

### 批次 7a

- 最终反向审计发现 `web_search`、`web_fetch`、`http_request` 仍沿用“schema 由类型生成、Call 另行手写 `json.Unmarshal`”的双契约；将三个外层工具改为保留真实 concurrency capability、Definition/Call 委托同一个 typed function，与 filesystem wrapper 采用同一最小组合方式；
- 三工具现在在 provider/network 边界前统一拒绝未知字段、缺失字段、尾随 JSON、类型错误和 schema 约束违反，不再出现模型看见 enum/range 但执行路径静默接受的漂移；
- `web_search` 精确约束 `max_results=1..20`、recency 闭集和 domain filter 数量，并让空白 query 与描述一致地失败；`web_fetch` 约束 format 闭集并验证绝对 http(s) URL；`http_request` 约束大写 method 闭集与 `timeout_ms=1..120000`，同时修正“method allowlist 属于 host”的不准确描述；
- 三个独立网络工具 module 都提升到同一已发布严格工具核，并在 `GOWORK=off` 下分别通过 test/build/vet；Runtime 同时钉住新的根模块和三个子模块版本，避免 workspace 掩盖发布依赖漂移；
- 没有把 provider、allowlist、network safety 或 Runtime policy 下沉到通用 `tool.Func`：通用核只负责所有消费者共同需要的严格 schema/decode/encode，URL 与 provider 业务不变量仍留在各具体工具。

### 批次 7b

- 最终 catalog 反查发现固定内建名称的安全表只做了“表项必须可达”单向校验，遗漏项会静默走 third-party unknown 的 `Exec` fallback；这使 `read_shell_output`、`search_memory`、`search_conversations` 和 `search_tools` 被错误标成任意执行，也使 network 类虽然存在却没有真实内建消费者；
- 为全部固定内建工具显式分类：Shell 启动/停止为 `exec`，增量输出读取为 `safe`；记忆、会话和工具发现为 `safe`；`web_fetch`、`web_search`、`http_request` 为 `network`。MCP 与 A2A 动态名称继续保守走 unknown `exec`，没有通过字符串前缀猜安全性；
- catalog guard 现在用 every-optional-subsystem 的真实 Resolver 同时验证两个方向：每个 safety key 必须对应可构造工具，每个内建可构造工具必须有显式 key；测试联合 edit/write 与 apply_patch 两个互斥模型 profile，并覆盖 late-bound `delegate_task` / `create_goal`；
- approval 文档与 SafetyClass 注释同步真实矩阵：Balanced 自动允许已配置的已知 write/network，Safe 提示所有非 safe，Plan 拒绝所有非 safe；没有更改用户权限策略，只修复此前被错误归类工具走错策略的问题。

### 批次 7c

- 最终执行契约反查发现通用 `AgentTool` 仍是“Definition 用 typed schema、Call 自行 `json.Unmarshal`”的最后一处双路径；它会让 `delegate_task` 的 required、`minLength`、未知字段和尾随 JSON 只存在于模型定义而不被执行边界一致地强制；
- `AgentTool` 现在组合一个通用 typed function，由后者唯一拥有 Definition、schema、严格输入解码和结果文本约定；外层只保留子进程创建、并发声明、挂起恢复和结果提取这些真实 Agent Runtime 职责，没有新增 decoder、runtime facade 或第二套工具接口；
- 挂起关系仍需比对模型调用时的原始 JSON，外层只用私有 context key 把该次原始参数传给内部调用；typed 输入继续作为 child process 的领域输入。这个内部调用细节没有进入 `tool.Tool`、Agent 公开 API 或 `app/runtime`，也没有通过重编码参数改变恢复 identity；
- `agent` 模块在 workspace 与 `GOWORK=off` 下分别通过 test/build/vet；Runtime 钉住已发布的新 Agent module，并用真实 `delegate_task` 验证旧字段、空值、缺字段和尾随值都会在创建 child process 前失败；
- 边界扫描继续确认 `agent` 对 `app/runtime` 零导入，runtime domain/application 对 adapter/infra/delivery/bootstrap 零反向依赖。Plan、Goal、session、模型选择、安全策略和 Exposure 仍全部停留在产品 Runtime。

### 批次 7d

- 参数单位反查发现 `read(offset,limit)` 把“0 表示默认”和“1-based 行号”放进同一个字段，且两个名称都没有表达行单位；模型入口改为 `read(path,start_line?,max_lines?)`，显式 `start_line=1` 与省略含义一致，旧字段直接拒绝；
- edit guard 同步读取新参数，并按真实返回范围判定 partial：`start_line>1` 或设置 `max_lines` 才是 partial；从第 1 行读到文件末尾不再被错误标成 partial。这里继续只是 adapter presentation，没有把 session tracker 下沉到 filesystem tool；
- 根模块的通用同步 `shell` 工具把 `timeout` 改为 `timeout_ms=1..600000`，Definition 与 Call 改由同一个 typed function 拥有；它保留为可注入 `Executor` 的 SDK 工具，Runtime 自己的后台 Shell family 仍负责 session-scoped lifecycle，两者没有在同一模型 manifest 中重复注册；
- `fakeweather` 示例工具改名为动作优先且明确非真实数据的 `get_synthetic_weather`；`date/include_hourly/include_air_quality` 的 schema 可选性与实现一致，并拒绝未知字段、尾随 JSON 和无效日期格式。示例不再示范“schema 严格、执行宽松”的错误模式；
- root module 通过完整 test/build/vet；Runtime 钉住已发布的 root module，并用 edit-guard 集成测试覆盖 partial 与显式整文件读取。没有新增跨层接口、兼容字段或第二套 decoder。

### 批次 7e

- 条件字段反查不再只验证“需要的字段存在”，同时拒绝“该动作不会消费的字段”：`lsp` 的 position operation 只接收 path + line + character，document/diagnostics 只接收 path，workspace_symbols 只接收 query；仍保留一个语言服务器工具，不为每个 query 复制九个同构 wrapper；
- Shell family 删除 ignored-parameter 语义：`timeout_ms` 显式为正数或省略，`read_shell_output` 只有 `wait=true` 才接收它，`run_in_background=true` 不再同时接收不会生效的 `auto_background_after_seconds`；
- `report_goal_outcome` 用可选指针区分 reason 缺省与显式传值：blocked 必须提供非空 reason，completed 必须完全省略 reason；无效组合在 Goal state 改变前返回精确指导；
- `ask_user` schema 与真实 question invariant 对齐：question/label 非空，header 最长 12 字符，options 为 2..4；描述删除 `chip` 等展示控件词，并明确 options 与 multi_select 的使用条件；
- 网络、Schedule、Goal result 和 `search_tools` 的模型文本删除 `runtime`、`client`、`operator`、MCP/provider 实现措辞，分别改用 configured policy/default、service、built-in 和 authenticated integration 等行为词；三个网络子模块的发布版本同步钉入 Runtime；
- catalog fitness guard 在 every-optional-subsystem 的真实内建工具集合上永久验证 Definition、object schema、`additionalProperties=false` 和模型契约实现词；第三方 MCP/A2A definition 不受内建文案风格约束。该 guard 与 safety parity 各守一个维度，不合并成新的 registry abstraction。

### 批次 7f

- 现行架构基准把旧 `task` / Todo 改为 `delegate_task` / Plan，端口文档同步真实的 `toolset/plan.Store`、`agentexec.PlanReader` 与 SQLite plan store，不再让已经删除的类型看起来仍是当前设计；
- `ARCHITECTURE_HYGIENE_PLAN.md` 明确降级为历史实施台账，`doc/inspiration` 总索引明确降级为同类产品对比快照；其中 Todo/Task 等词只保留为历史代码或其他产品原生术语，不再声明 Lynx 当前实现状态；
- 当前规范只有本文与 `EXECUTION_CENTERED_ARCHITECTURE.md` / `EXTENSIBILITY.md`。历史材料通过醒目链接回到本文，而不是复制一份会再次漂移的“当前工具表”；
- 排除已标记历史材料、本文删除记录和本轮刻意未修改的前端 generated baseline 后，现行 runtime 文档不再出现 Todo、`task`、`update_plan` 或 `update_goal`。

### 批次 7g

- 最终验证矩阵全部通过：root module 完整 test/build/vet；Agent module workspace + standalone test/build/vet；`httpreq` / `webfetch` / `websearch` 三模块 workspace + standalone test/build/vet；Runtime 除明确延后的前端契约漂移包外，workspace + standalone 全包 test/build/vet；
- 完整 `app/runtime go test ./...` 只失败于 `internal/arch` 与 `internal/delivery/protocol`：桌面 TypeScript generated contract 未反映 Plan/GetPlan 与 artifact v10，canonical samples 仍是 Todo/artifact v9，另有旧 plan delta/item samples 已无 server binding。它们完全落在用户指定的下一轮前端专项范围，本轮没有越界生成或修改；
- 生产 Go 源码扫描确认旧工具名、旧参数名和 `weather_query` 均无模型入口；固定内建 catalog 的 safety/strict-schema/implementation-word 三组 fitness checks 全部通过；
- 最终边界扫描再次确认 `agent` 对 `app/runtime` 零导入，runtime domain/application 对 adapter/infra/delivery/bootstrap 零反向依赖；没有为收尾新增 facade、胖接口、通用 manifest 状态或兼容 decoder；
- 当时的删除结论包含 `propose_skill`；该单点结论已由批次 8 取代。`download`、`sourcegraph_search`、Agent `codebase_search` 与 Todo 的删除结论不变；client codebase index 与后台 Skill authoring 继续因非模型真实消费者而保留。

### 批次 8a

- 将 Skill authoring 的规范词汇统一为 `Proposal / Submit / Approve / Reject / project / user`，完整删除 `Draft / SaveDraft / Promote / Discard / global` 及其 store、application、wire 和 RPC 名称，不保留兼容别名；
- `skills.ProposalRef{scope,name,revision}` 绑定 scope、name 与完整渲染内容的 SHA-256，project/user store 都在 `_proposals` 保存不可变内容；评审 list 返回完整指令和 provenance，不再让用户只凭摘要审批；
- 新增 adapter-owned `skillproposal.Libraries`，以 workspace root 路由 project library、以配置 root 路由 user library。Application 只接收已解析 root 和领域 Proposal，不认识 `.lyra/skills` 文件布局；
- 服务端评审协议改为 `skills.proposals.list / approve / reject`，请求显式携带 workspace 和 scope，旧 `skills.drafts.*` 直接消失；本轮只生成 Go contract 和 validator，桌面 TypeScript 与 canonical samples 按约定留给前端专项；
- SkillMiner 改为消费应用层单方法 `SubmitSkillProposal`，固定提交 `scope=user, origin=mined`，不再直连 `skillauthoring.Store`。前台与后台共享持久化不变量，但不共享触发策略。

### 批次 8b

- 新建根 Agent Deferred 工具 `propose_skill(name,description,instructions,scope)`；四个字段均严格校验，scope 仅允许 `project | user`，未知字段由 typed function schema 拒绝；
- definition 明确规定“只有用户明确要求保存工作流才调用”，并明确 Proposal 仍待评审、不会 activate/publish/run；one-off fact、普通进度、瞬时修复和 secret 都是反例；
- session、cwd 与 `origin=requested` 只从 execution context 注入；模型 schema 不出现 host identity、provenance、review decision 或文件路径；返回值只含 `pending_review` 与不可变 ref；
- Resolver 将工具设为 root-only + Deferred，委派 Agent 不注册；safety 归类为 Safe，因为它只记录用户明确要求的待评审内容且无法激活能力，额外 approval 会重复同一意图门；
- 工具角色词汇同步收敛为 `root / delegated`；删除把根角色称作 `coding`、把委派角色称作 `subtask` 的内部常量与 resolver 名称，使通用场景与策略名称保持一致；
- Skill adapter 只定义单方法 `ProposalSubmitter`，Bootstrap 在 composition root 把 `workspace.Skills` 赋给它；Resolver 与 Agent 只看通用 `tool.Tool`，没有引入 application coordinator、store、delivery DTO 或 Skill filesystem layout；
- 隔离审计发现原有 `TurnScope.Cwd` 在 isolated Run 中指向短命 scratch copy，不能作为 project Proposal 的持久 workspace identity；执行上下文因此治本式拆为 execution Cwd 与 `WorkspaceCwd`，fresh/restore/checkpoint 全链路保持两者。文件与 Shell 继续使用 execution Cwd，Proposal 使用 persistent workspace，避免把 Skill 写进随后销毁的 sandbox；
- 参数、角色、Deferred manifest、provenance、双作用域路由、安全表和 catalog completeness 均有聚焦测试；服务端 build/vet 通过，完整 test 仅保留明确延后的桌面 generated contract/sample 漂移。

### 批次 9a

- 删除此前功能移除和目录迁移遗留的空目录；将 `goaltool / lsptools / toolresult / toolsearch` 分别改为能力名 `goal / lsp / offload / discovery`，不保留旧 import path；
- 根 `toolset` 文件按职责改为 `online / schedule / workdir / connections`，内部构造函数同步删除 `Tools` 重复词；测试 helper 中残留的 `coding` 统一为 `root`；
- `search_tools` 的模型名称保持不变；代码包名 `discovery` 表达 progressive disclosure 职责，避免出现 `toolset/toolsearch.Tool` 三重重复。

### 批次 9b

- 将 `enterplan / plantool / exitplan` 合并为 `plan` 包和一个 `Build` 入口；三工具继续拥有独立 schema，但共享同一 `Store` 和 `ModePolicy`，从结构上保证退出审批读取的就是 `set_plan` 维护的 canonical Plan；
- 将 `proposeskill` 合入 `skill` 包，读取工具与 Proposal 写入口按 `BuildReaders / NewProposal` 表达不同构造时机；同一 Skill 能力不再分散在两个相邻包；
- 将根包中的 Schedule schema 和 list/create/delete 实现整体迁入 `schedule` 能力包，以单一 `Build(Management)` 入口返回完整工具族；根 `toolset` 不再承载该领域的模型契约；
- 将仅由父包使用的 `editguardstate` 折回 `toolset`，`Tracker / Fingerprint / Result` 全部私有化为 read-tracker invariant，删除为绕包边界产生的导出面；
- 把混装 format、Schedule、path guard 的 `new_tools_test.go` 拆回对应职责测试文件；内部 `tool/createTool/pathLockedTool/decoratedTool` 等无信息量名称改成行为或职责名；
- 明确保留 `memorysearch / sessionsearch / askuser` 的独立边界：它们虽然表面共享 search 或 interrupt 动词，但没有共同状态源和变化轴，合并会造成虚假内聚。

### 批次 10a

- 为 transcript 的 ToolCall Item 增加唯一的终止事实 `FinishedAt`；执行开始仍由 Item 的 `CreatedAt` 定义，实时协议显式投影为 `startedAt`，终态再投影 `finishedAt` 与派生的 `durationMs`。没有把三个可变时间字段同时持久化，避免边界时间与持续时间互相漂移；
- reducer 在收到 `ToolCallEnd` 时立即盖章，而不是等并发调用按模型顺序 flush 时才计时，因此 transcript 顺序仍按模型调用顺序，耗时却保留真实完成边界；取消、挂起、重启恢复和不可恢复的 parked Run 也在各自终止事务中补齐同一事实；
- ToolCall 领域 invariant 明确为：running 只有开始时间，completed/incomplete 必须有不早于开始的结束时间，其他 Item 禁止携带 ToolCall 结束时间。实时 DTO 与 artifact DTO 的存在性规则、非负持续时间约束由同一 contract registry 生成；
- artifact 提升到 v11，导入同时核验 `startedAt == createdAt` 以及 `durationMs == finishedAt - startedAt`，不为旧 artifact 猜测执行边界，也不保留兼容字段；
- 时间事实停留在 transcript / runs / delivery：通用 Agent ToolCall 协议仍只表达模型请求，没有被 app/runtime 的持久化生命周期污染。本轮只更新服务端 Go 契约；桌面 TypeScript 与 canonical samples 继续留给约定的前端专项。

### 批次 10b

- `shell` 新增必填 `description`，语义严格限定为描述命令目的的简短动作短语，长度 `1..120`，拒绝空白和首尾空白；工具 definition 明确要求不复述原命令、不预言执行结果，避免模型把它写成第二份命令或状态字段；
- Turn 的 `ToolPresenter.Activity` 从只接收工具名改为同时接收 canonical arguments；Shell activity 直接消费 `description`，因此该字段既保留在 ToolCall 参数里供后续客户端投影，也在服务端执行生命周期中有真实消费者；
- `delegate_task` 不新增同义的 `description/label`，而是复用现有、已限定为 3–5 个词的 `summary` 生成 activity。其他工具也没有新增通用 `displayName`：path、query、Skill name 和 Schedule title 各有不同领域语义，强行抽成一个展示字段会制造重复事实；
- 参数解析仍停留在 Turn 的 consumer-side presenter seam，具体 Shell / delegation schema 停留在 toolset / Agent adapter；通用 `tool.Tool`、运行领域和 delivery DTO 没有获得 UI metadata 接口。本批继续只改服务端，前端如何优先渲染 `description` 留给后续接线专项。
