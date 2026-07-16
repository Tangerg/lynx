# Runtime Agent/Core 对齐与执行边界收敛计划

> 状态：**进行中，下一批 B3 Process 启动错误显式化**
>
> 建立日期：2026-07-16
>
> 审查基线：`92b4147a5afd7294a144196043d54858bb80b89d`
>
> 实施分支：`codex/runtime-architecture-refactor`
>
> 架构基准：[`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md)
>
> 历史计划：[`EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md`](EXECUTION_ARCHITECTURE_CONVERGENCE_PLAN.md) 已完成，只作历史证据，不再追加本轮任务。

---

## 0. 文档职责

本文档是 `app/runtime` 在 Agent/Core 大幅演进后的新一轮执行计划，用于固定：

- 当前代码事实与已确认缺口；
- 本轮重构的目标、边界、非目标和不可破坏的不变量；
- 各批次的先后依赖、完成条件和验证门禁；
- 需要暂停咨询的公开 API 决策；
- 每批实施结果、偏差、测试证据和提交记录。

后续实施必须以本文档为控制面。发现新事实时，先更新“事实 / 决策 / 风险 / 进度”，再改变执行路线，禁止只在对话中临时改方向。

### 0.1 事实优先级

发生冲突时按以下顺序判断：

1. 当前代码、测试和可复现命令；
2. 根目录与模块级 `CLAUDE.md`；
3. [`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md)；
4. 本计划中的目标设计；
5. 已完成的历史计划和旧讨论。

若架构文档与当前已确认目标不一致，不迁就旧文字；在对应批次完成后同步修正文档。

### 0.2 最高实施法则

本轮明确遵循项目第一、第二法则：

- 不做历史兼容、不保留 legacy 字段、别名、shim、双写、迁移分支或“以后再删”的临时代码；
- 问题必须在根因所在层修复，禁止在 Runtime 症状点包一层 workaround；
- store schema、内部结构和未发布 API 可直接改成最终形态；
- 若治本需要破坏性修改其他模块的 exported API，先记录影响面并暂停咨询，确认后直接改正，不为规避咨询而在 Runtime 留补丁。

---

## 1. 当前状态

### 1.1 基线验证

在审查基线上的已确认结果：

| 验证项 | 状态 | 说明 |
|---|---|---|
| Runtime workspace 测试 | 通过 | `go test ./...` |
| Runtime workspace vet | 通过 | `go vet ./...` |
| 关键并发路径 race | 通过 | `agentexec`、`turn`、`runs`、`sessions`、`server`、SQLite 等关键包 |
| Runtime 独立模块测试 | **失败** | `GOWORK=off go test ./...` 暴露 `go.mod` 内部模块版本滞后 |
| 本轮代码实施 | 进行中 | `2 / 10` 批次完成 |

独立模块失败不是外部环境问题，而是依赖声明与实际源码不一致。workspace 的本地替换掩盖了该问题，本轮第一批必须先恢复依赖真相。

当前可复现的三组失配：

- Runtime 使用了新版 Core 的 `ProcessViewFrom`，独立模块解析到的旧 Core 尚无该符号；
- Runtime SQLite ProcessStore 已采用带 revision / CAS 的新契约，旧 Agent Core 仍要求旧 `Save` 签名和旧 snapshot shape；
- Runtime embedding adapter 已采用 Options 值语义，独立模块解析到的旧 Core 仍要求指针形态。

B1 已在 `09f32465afd4` 解决上述失配；当前 workspace 与 standalone 的 build、vet、test、race 均通过。

### 1.2 最近底层调整的影响

Agent/Core 最近的调整已经改善：

- `chat.Options.Clone` 提供完整请求快照能力；
- media / modality 更接近不可变值语义；
- 部分空壳 helper 和重复抽象已移除；
- Core 的 options 校验覆盖比 Runtime 手写校验更完整。

这些变化没有推翻 Runtime 现有的 Run 中心架构，但使以下 Runtime 自有逻辑变成了重复、滞后或不安全实现，需要重新对齐。

### 1.3 总体判断

当前实现不是推倒重写状态。以下核心资产应保留：

- `application/runs.Coordinator` 对 Run 生命周期的所有权；
- event reducer、journal、hub 和重连语义；
- Session / Transcript 领域模型；
- Runtime 的 Run、Segment、Turn 与 Agent Process 的防腐边界；
- `adapter/agentexec/turn` 作为“一次 segment 执行”的应用专用适配器；
- `bootstrap.Host` 对进程级资源生命周期的最终所有权。

本轮的本质是：

> 保留 Run 中心应用架构，吸收 Agent/Core 已成熟的值语义、校验、进程控制和持久化原语；同时清除 Runtime 对底层能力的重复实现、错误所有权和过宽依赖。

---

## 2. 唯一目标

完成本计划后，Runtime 应满足：

1. `app/runtime/go.mod` 精确声明实际使用的 Agent/Core 等内部模块版本，脱离 workspace 也能独立构建和测试。
2. 所有跨 goroutine 的 turn 请求在启动前形成完整、不可变快照，不共享调用方可变数据。
3. Options 校验由 Core 负责通用协议约束，Runtime 只保留应用特有约束。
4. Agent Process 的创建失败、运行失败、暂停、取消和终止都以显式契约进入 Run 状态机，不以 nil dereference 或隐式 panic 表达。
5. 一次 Run 选择的 provider / model、审批策略、hooks 和取消语义覆盖完整委派树。
6. 子 Agent 内部的人工介入可被提升为同一个应用 Run 的 interrupt，并能精确恢复原子进程树，不伪装成普通工具 JSON 结果。
7. 模型流空闲超时、工具执行超时和整轮无活动策略各自有明确所有者，互不误杀。
8. 最终响应只有一个权威来源；停止原因只有一个合法值，不能构造互斥状态同时为真。
9. Build identity 和 snapshot failure policy 显式装配；不兼容快照按当前产品策略确定性转为 `run_lost`，不做历史迁移。
10. `adapter/agentexec.Engine` 只拥有 Agent execution/deploy/process 相关职责；工具目录、MCP facade、维护能力和资源关闭回归各自所有者。
11. 架构测试能够阻止上述所有权、依赖方向和异步值语义重新漂移。

---

## 3. 非目标

本轮不做：

- 用 Agent Process 替代应用 Run；
- 把 `application/runs` 生命周期下沉到 Agent SDK；
- 改 Runtime Protocol 的 wire shape，除非实现中发现无法避免的协议根因；届时必须单独暂停确认；
- 为旧 snapshot、旧 schema 或旧 exported API 保留兼容分支；
- 全局改变 Agent 默认的 child capability inheritance；
- 为“未来可能用到”增加新的通用框架层、胖接口或抽象工厂；
- 在没有测量证据的情况下做性能优化；
- 顺手重构与本计划无关的模块。

---

## 4. 目标所有权

| 关注点 | 唯一所有者 | Runtime 允许做什么 | Runtime 禁止做什么 |
|---|---|---|---|
| Run 生命周期 | `application/runs` | admission、journal、pump、park、resume、cancel、terminal | 交给 Agent Process 替代 |
| Segment 执行 | `adapter/agentexec/turn` | 翻译 app 输入、驱动 Agent 交互、返回 app 输出 | 拥有整个 Run |
| Chat 协议值与通用校验 | Core `chat` | 调用 `Clone` / `Validate`，包装应用上下文 | 复制一套落后的字段校验 |
| Agent Process 生命周期 | Agent | create、interact、suspend、resume、snapshot | 用 nil/panic 猜测创建失败 |
| 委派树应用策略 | Runtime host + Agent 显式扩展点 | 为 child 显式注入本 Run 策略 | 修改 Agent 全局默认继承 |
| 审批与 hooks | Runtime application policy | 覆盖 root 和 child 的副作用工具 | 只保护根进程 |
| 模型流空闲超时 | model stream 边界 | 配置每次模型流的 idle policy | 用整轮 timer 误杀长工具 |
| 工具执行超时 | 工具 / tool runner | 各工具按自身契约控制 | 与模型流 timeout 混用 |
| 最终响应 | Agent tagged `Final` event | 按 ModelResponse / ToolResult 分支直接消费 | 再维护一份并行 builder |
| Build identity | bootstrap / binary | 注入 deploy/build 标识 | 在 adapter 内硬编码版本 |
| 资源关闭 | `bootstrap.Host` | 保存并逆序关闭 closers | 把 closers 转交给 Engine |
| MCP / tools / maintenance | 各自 adapter 或 registry | 依赖所需窄端口 | 通过胖 Engine 中转 |

---

## 5. 不可破坏的不变量

### 5.1 Run 与 Process

- Run 是应用事实，Agent Process 是执行机制；两者不可合并。
- 一个 Run 可以跨多个 segment、interrupt、重连和进程恢复。
- Process 创建失败必须成为正常的 terminal error 路径。
- Run terminal 后不得留下活跃 child、goroutine、observer、snapshot 或 tool call。

### 5.2 异步值所有权

- goroutine 只能读取启动前已经快照化的请求值。
- `chat.Options`、media、interrupt kinds 和嵌套 slice/map/pointer 不得与调用方共享可变存储。
- “当前调用方碰巧不修改”不构成安全契约。

### 5.3 委派树策略

- 本 Run 选择的 provider / model 默认覆盖整个委派树。
- root 和 child 的每次副作用工具调用都经过同一套审批与 hooks。
- child suspension 不得编码为普通成功工具结果后让 parent 继续生成。
- 恢复必须回到原 child continuation，再完成 parent tool call，不能新建重复 child。
- 取消 root Run 必须取消所有后代。
- Agent 默认仍保持最小 child capability inheritance；Runtime 策略通过显式 option / hook 注入。

### 5.4 持久化

- 只有持久化成功的事件才能对订阅者可见。
- parked Run 的 parent / child 关系必须可恢复，不能只存在于内存闭包。
- Build 不兼容时确定性失败并清理，不静默尝试旧形态。
- 本轮不写 snapshot migration。

### 5.5 完成语义

- 正常完成以 Agent 返回的 tagged final event 为权威来源。
- budget / step 等同类提前停止原因必须是单值枚举。
- cancel、error、interrupt 继续由各自 terminal 类型表达，不与 budget / step 混成一个枚举。
- observer / hook / event 每个逻辑动作最多提交一次。

---

## 6. 已确认差异与归属批次

| 编号 | 严重度 | 当前差异 / 风险 | 根因层 | 目标批次 |
|---|---|---|---|---|
| G-01 | P0 | **已解决**：workspace 通过但 `GOWORK=off` 失败 | 模块依赖声明 | B1 |
| G-02 | P1 | **已解决**：pure-media turn 只构造 media part，不再生成空 text part | turn 输入翻译 | B2 |
| G-03 | P1 | **已解决**：dispatcher 与 Agent engine 的异步入口完整快照 options、media 和 interrupt kinds | 请求所有权 | B2 |
| G-04 | P1 | **已解决**：通用 Options 字段规则委托 Core `Validate`，Runtime 只包装上下文和限制 model 选择位置 | 校验边界 | B2 |
| G-05 | P1 | Process create error 可返回 nil process，随后访问 `ID()` 可能 panic | Agent adapter 契约 | B3 |
| G-06 | P0 | child 默认不继承 per-run provider、tool middleware、guardrail / dependency 策略 | 委派树装配 | B6、B8 |
| G-07 | P0 | child suspension 被编码成 `waiting` 工具结果，parent 可继续而应用 Run 未真正 parked | Agent 子进程暂停契约 | B7-B8 |
| G-08 | P1 | `llmIdleTimeout` 包围整个 `Interact`，但仅模型 chunk 重置，长工具可能被误杀 | timeout 所有权 | B4 |
| G-09 | P2 | adapter 同时维护文本 builder 与 Agent ResponseAccumulator | 输出所有权 | B4 |
| G-10 | P2 | `StoppedOnBudget` / `StoppedOnSteps` 两个 bool 可构造非法组合 | 输出状态模型 | B4 |
| G-11 | P1 | steering guardrail 构造错误被静默丢弃 | 错误传播 | B4 |
| G-12 | P1 | HITL 依赖 JSON 字段形状猜 approval / question，缺少应用级 discriminator | interrupt 防腐契约 | B8 |
| G-13 | P1 | BuildID / SnapshotFailurePolicy 未显式装配，Agent definition 版本硬编码 | deploy / persistence | B5 |
| G-14 | P2 | `agentexec.Engine` 同时中转 process、maintenance、tools、MCP、closers 等职责 | adapter 所有权 | B9 |
| G-15 | P2 | 若干单实现 process 接口只增加间接层；真实消费方接口反而被胖 Engine 遮蔽 | 包设计 | B9 |
| G-16 | P2 | 架构文档仍有旧 toolloop / Engine 所有权描述 | 文档漂移 | B10 |

严重度定义：

- P0：安全性、持久化或核心行为错误，必须在本轮解决；
- P1：明确 correctness 风险或错误契约；
- P2：结构性漂移，若不修会继续扩大维护成本。

---

## 7. 执行路线

### 7.1 批次顺序

| 批次 | 主题 | 前置 | 当前状态 |
|---|---|---|---|
| B1 | 依赖真相与独立模块闭环 | 无 | 已完成 |
| B2 | Turn 输入值语义与校验统一 | B1 | 已完成 |
| B3 | Process 启动错误显式化 | B2 | 待执行 |
| B4 | Managed interaction、timeout 与输出语义 | B3 | 待执行 |
| B5 | Build identity 与 snapshot failure policy | B4 | 待执行 |
| B6 | Child 执行上下文与成本归属 | B5 | 待执行 |
| B7 | Agent generic nested suspension / checkpoint 原语 | B6 | 待执行 |
| B8 | Runtime HITL envelope、恢复与取消集成 | B7 | 待执行 |
| B9 | `agentexec` 职责与资源所有权收敛 | B8 | 待执行 |
| B10 | 架构适配测试、文档与最终清理 | B9 | 待执行 |

不得为了“先做容易的”跳过前置。尤其 B9 的结构拆分必须等 B2-B8 的行为契约稳定后再做，避免同时移动行为和所有权导致回归难以定位。

B4 提前于委派树实施：先消除整轮 idle timer、重复输出状态和错误吞没，避免它们干扰后续长工具、child 和 resume 时序测试。B5 再固定持久化身份与失败策略，为 B7-B8 的 parent/child checkpoint 提供稳定地基。

### 7.2 每批固定流程

1. 重新核对该批涉及的当前代码与测试，不机械照搬本文的旧符号名。
2. 在本文更新该批“开始时间 / 实际 scope / 新发现”。
3. 先补能暴露根因的失败测试。
4. 在根因层实施，不加兼容路径。
5. 跑该批局部测试与 race。
6. 跑 Runtime 全量 workspace 与 standalone 门禁。
7. 检查 diff、死代码、错误包装、goroutine / channel 所有权。
8. 更新本文证据。
9. 一个批次形成一个可独立 revert 的 commit，并推送。

如果某批必须跨多个模块，可在同一批内形成多个按依赖顺序的原子提交，但不能把半完成、无法独立全绿的中间状态推为“该批完成”。

涉及 Agent/Core 与 Runtime 的跨模块批次固定采用：

1. 先完成底层模块修改、全量验证、commit 并 push；
2. 再让 Runtime `go.mod` pin 到已存在的底层提交；
3. 完成 Runtime 集成、standalone 验证、commit 并 push；
4. 只有消费者 pin 已落到可获取提交且最终门禁全绿，整个批次才算完成。

禁止让 Runtime pseudo-version 指向尚未提交 / 尚未推送的工作区状态。

---

## 8. B1 — 依赖真相与独立模块闭环

### 8.1 目标

让 `app/runtime/go.mod` 成为真实、可复现的模块声明，而不是依赖 `go.work` 才能编译的近似记录。

### 8.2 实施内容

- 盘点 Runtime 实际使用的所有仓内模块版本。
- 更新 Agent、Core、embedding 及其传递相关的 pseudo-version。
- 执行 `go mod tidy`，确认没有 workspace 才可见的隐式依赖。
- 检查 `go.sum` 变化是否全部由真实依赖闭包产生。
- 增加或调整 CI / 本地验证入口，使 standalone 测试成为固定门禁。

### 8.3 禁止

- 用 `replace ../...` 写进 Runtime `go.mod`；
- 降级 Runtime 代码以适配旧 pseudo-version；
- 在构建脚本里强制开启 workspace 掩盖问题；
- 只更新直接报错的一个模块而不验证完整依赖图。

### 8.4 完成条件

- `go test ./...` 通过；
- `go vet ./...` 通过；
- `GOWORK=off go test ./...` 通过；
- `GOWORK=off go vet ./...` 通过；
- `go mod tidy` 后工作区无二次差异；
- 记录最终内部模块版本与提交。

### 8.5 执行记录

- 状态：已完成
- 开始时间：2026-07-16
- 完成时间：2026-07-16
- 实际 scope：
  - Runtime 全部仓内直接依赖与 tokenizer 间接依赖统一到 `v0.0.0-20260716134603-cc8be60da95e`；
  - `go.sum` 只替换对应仓内模块的旧 / 新 checksum；
  - 新增 `make check-standalone`，固定执行 `GOWORK=off go mod tidy -diff`、build、vet、test；
  - CI 的 `app/runtime` matrix 增加 standalone module graph 阻塞门禁。
- 新发现：旧 pin 后不只 Agent/Core 发生变化，a2a、chatclient、chathistory、mcp、models、tools 等也已调整；只升级最先报错的模块会留下混合依赖图。
- 与计划偏差：无。
- 公开 API 影响：无。
- 底层模块 Commit / Push：`cc8be60da95e` 已推送并可由 Go proxy 解析。
- Runtime pin Commit / Push：`09f32465afd4` 已推送。
- 测试命令与结果：
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `go test -count=1 ./...`：通过；
  - `make check-standalone`：通过；
  - `GOWORK=off go mod tidy -diff`：无输出；
  - CI YAML 解析与 `git diff --check`：通过。
- race 结果：
  - `go test -race -count=1 ./...`：通过；
  - `GOWORK=off go test -race -count=1 ./...`：通过。
- standalone 结果：build、vet、test、race 全部通过。
- Commit：`09f32465afd4`
- Push：`origin/codex/runtime-architecture-refactor`
- 剩余风险：无；G-01 关闭。

---

## 9. B2 — Turn 输入值语义与校验统一

### 9.1 目标

在 turn goroutine 启动前形成完整请求快照，并让 Core 成为通用 chat options 约束的唯一权威。

### 9.2 实施内容

- 使用 `chat.Options.Clone()` 替代 Runtime 的字段级手工复制。
- 对 Runtime 自有请求字段建立同等级快照：
  - media；
  - interrupt kinds；
  - 其他 slice / map / pointer 字段；
  - observer / callback 仅按其明确并发契约保存，不伪装成值复制。
- pure-media 输入仅在 text 非空时构造 text part。
- Runtime application 边界：
  - 先调用 Core `Options.Validate()`；
  - 再校验 Runtime 特有的 model/provider 选择规则；
  - 用 Runtime 自有 sentinel 包装应用上下文，保留 `errors.Is/As`。
- turn adapter 边界可独立做防御性校验，但不得复制字段规则；直接委托 Core，再补 adapter 特有前置条件。

### 9.3 设计约束

- 不为了消除两处薄包装，建立跨 application / adapter 的共享 validation helper；两处边界因不同原因独立演化。
- snapshot 后的 goroutine 不读取调用方 request。
- Clone 的 nil / zero value 行为以 Core 契约为准，不另造 Runtime 语义。

### 9.4 必测场景

- text-only、media-only、text + media；
- invalid temperature / penalties / TopK；
- NaN / ±Inf；
- 启动 turn 后调用方修改原 options、media slice、interrupt slice，不影响执行快照；
- `go test -race` 下并发修改测试无竞态；
- nil observer 与合法 observer。

### 9.5 完成条件

- Runtime 不再维护通用 Options 字段规则；
- 不存在无条件 `NewTextPart("")`；
- turn 异步边界的输入所有权可由测试证明；
- 全量与 standalone 门禁通过。

### 9.6 执行记录

- 完成日期：2026-07-16。
- 实施范围：
  - application 与 turn adapter 的通用 generation options 校验均委托 `chat.Options.Validate()`，并用各自 `ErrInvalidTurnOptions` 保留应用错误上下文和 Core sentinel；
  - `StartTurnRequest` 在启动 goroutine 前完整快照 options、media 和 interrupt kinds；
  - `agentexec.TurnRequest` 在创建异步 Agent Process 前再次拥有 options 与 media，保留 observer、callback、client 等运行期协作者原有共享契约；
  - 移除 turn goroutine 内不完整的手工 options 浅复制；
  - pure-media 不再构造非法空 text part，text-only、media-only、text + media 均按真实 part 形状进入 Core request。
- 回归证据：
  - 覆盖 FrequencyPenalty、PresencePenalty、TopK、NaN、Inf 和 Runtime `Options.Model` 约束；
  - 覆盖 caller 在 `StartTurn` 返回后修改 options 指针字段、stop slice、media bytes / slice、interrupt kinds，不影响执行快照；
  - 覆盖 dispatcher 和 Agent engine 两个异步入口的值所有权；
  - 覆盖 nil observer、合法 observer、text-only、media-only 和 text + media。
- 定向验证：
  - `go test -count=1 ./internal/application/runs ./internal/adapter/agentexec/turn ./internal/adapter/agentexec`：通过；
  - `go test -race -count=1 ./internal/application/runs ./internal/adapter/agentexec/turn ./internal/adapter/agentexec`：通过。
- 全量验证：
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `go test -count=1 ./...`：通过；
  - `go test -race -count=1 ./...`：通过；
  - `make check-standalone`：通过；
  - `GOWORK=off go test -race -count=1 ./...`：通过；
  - `GOWORK=off go mod tidy -diff` 与 `git diff --check`：无输出。
- Commit：`3d9a6f33c444`
- Push：`origin/codex/runtime-architecture-refactor`
- 剩余风险：无；G-02、G-03、G-04 关闭。

---

## 10. B3 — Process 启动错误显式化

### 10.1 目标

把 Process 创建失败从“nil process + 异步错误通道”的隐式组合翻译为 Runtime 可处理的同步启动错误。

### 10.2 实施内容

- 将 Runtime 内部 `StartTurn` 契约改为返回 `(TurnProcess, error)`。
- Agent adapter 在返回前确认 process 非 nil、ID 可用。
- 创建失败转换为 Runtime 的标准 EngineError / terminal failure，不进入普通 interact 路径。
- 保持对 application 的 dispatcher API 异步语义不变：
  - dispatcher 可以立即返回 handle；
  - 启动错误由该 handle 的结果流正常结束；
  - 不 panic、不产生半初始化 active turn。
- 审计 restore 路径是否存在同类 nil + error 时序问题，一并在根因层处理。

### 10.3 禁止

- 在 `process.ID()` 前加 `if process == nil` 后伪造未知错误；
- recover panic；
- 返回 dummy process；
- 把 create error 记日志后继续。

### 10.4 必测场景

- 重复的 process-scope extension name；
- process dependencies 不是 engine dependencies 的直接 child；
- Agent 请求了未注册 planner；
- definition / deploy 解析失败；
- create 返回错误时没有 goroutine / observer 泄漏；
- handle 只收到一次 terminal；
- Run journal 中错误状态与事件顺序正确；
- cancel 与 create failure 并发时仍只终止一次。

### 10.5 完成条件

- Runtime 内部不存在“成功返回 nil process”的合法状态；
- 创建失败路径不 panic；
- error、cancel、terminal 竞争通过 race 测试；
- 全量与 standalone 门禁通过。

---

## 11. B4 — Managed Interaction、Timeout 与输出语义

### 11.1 目标

使交互超时、最终响应和停止原因各自只有一个明确语义与所有者。

### 11.2 模型流 idle timeout

- 将 `llmIdleTimeout` 约束到每次 provider model stream。
- 仅模型流的有效进度重置该 timer。
- tool execution 不受 model stream idle timer 影响。
- 工具若需要 timeout，由工具自身或 tool runner 的 context 契约负责。
- 如果未来需要“整轮无任何活动”策略，必须以独立命名、独立配置和统一 activity 事件实现，不复用 LLM idle timeout。

### 11.3 最终响应

- 权威来源是 Agent 返回的 `result.Final` tagged event，而不是 Runtime 自建 builder。
- `Final.Kind == ModelResponse` 时读取 `Final.Response.Text()`。
- `Final.Kind == ToolResult` 时读取 `Final.ToolResult.Result`，保留 direct-return tool 的合法完成语义。
- 移除与 ResponseAccumulator 并行的整轮 text builder。
- 仅 budget / step 等人为提前停止、尚无 final response 的路径允许维护局部 partial 输出。
- partial 数据不得在正常 final 路径覆盖权威 response。

### 11.4 停止原因

用单值 `StopReason` 替换互斥 bool，至少能表达：

- none / normal；
- budget；
- steps。

cancel、error、interrupt 若已通过独立 terminal 类型表达，不强塞进同一枚举。具体枚举只覆盖当前真实需要，不预留推测性值。

### 11.5 错误传播

- `steeringGuardrails` 构造错误直接返回并进入正常 terminal error。
- 审计同路径所有被 `_` 丢弃的构造 / 注册错误。
- 仅真正不可达的不变量允许 panic，且 panic 必须包含断言上下文。

### 11.6 必测场景

- 模型长时间无 chunk 超时；
- 长工具执行超过 LLM idle timeout 但正常完成；
- tool timeout 不被误报为 model idle；
- ModelResponse final；
- ToolResult direct-return final；
- budget / step partial response；
- StopReason 不可同时为两个值；
- guardrail construction failure；
- stream completion、timeout、cancel 三方竞争。

### 11.7 完成条件

- 整轮 `Interact` 不再被只由模型 chunk 驱动的 timer 包围；
- normal final 以 tagged `result.Final` 为唯一权威；
- TurnOutput 不含互斥 bool 组合；
- 构造错误不被静默丢弃；
- 全量与 race 门禁通过。

---

## 12. B5 — Build Identity 与 Snapshot Failure Policy

### 12.1 目标

使可恢复执行的代码身份和失败策略成为 bootstrap 显式配置，而不是 adapter 硬编码或 Agent 默认值。

### 12.2 已定策略

- BuildID 是**当前运行二进制内容的 SHA-256 digest**，格式固定为 `sha256:<hex>`。
- release version、协议版本只用于展示，不作为 snapshot 兼容身份。
- `cmd/lyra` 在 bootstrap 前计算一次 BuildID 并显式注入；测试和嵌入式 host 使用固定可注入值。
- ProcessStore 开启时 BuildID 必须非空；无法读取 / 计算当前 executable 时启动失败，不退回 `"dev"`。
- SnapshotFailurePolicy 固定采用 Agent 的 `SnapshotFailureFailProcess`：
  - 自动 snapshot 写失败立即使 Process / Run 失败；
  - 不 pause-and-retry；
  - 不 report-only 后继续非持久化执行。
- restore 时 build 不兼容或单 Process snapshot 损坏 / 缺失，应用 Run 确定性转 `run_lost`。

选择二进制 digest 是为了避免 tagged version、相同 Git revision 下的 dirty build 或不同构建参数错误共享兼容身份。允许“本可兼容但被判不兼容”的保守失败，不允许不同代码错误恢复同一 snapshot。

### 12.3 实施内容

- 增加单一 build identity provider，计算逻辑不散落到业务包。
- bootstrap 将 BuildID 注入 Agent runtime config。
- 移除 Agent definition 中无语义的硬编码 `1.0.0`；只有 Agent 自身存在独立、真实的语义版本时才设置 Version。
- 显式装配 `SnapshotFailureFailProcess`，不依赖零值碰巧等价。
- 对自动写失败、不兼容 build、损坏 / 缺失 snapshot 分别定义：
  - 错误分类；
  - Run terminal 映射；
  - snapshot / interrupt 清理；
  - 对订阅者的事件顺序。
- 按本项目 dev 策略，不迁移旧 snapshot；不兼容状态确定性转 `run_lost`。

### 12.4 设计约束

- BuildID 属于部署 / 组合根，不属于 domain。
- Agent snapshot 错误在 adapter 边界翻译为应用语义。
- 不把 executable digest 计算散落到业务代码。
- 测试可注入固定 BuildID。
- active run 的自动 snapshot 写失败是 execution error，不误标为 restore-only 的 `run_lost`。

### 12.5 必测场景

- 同 BuildID 正常恢复；
- 不同 BuildID 确定性 `run_lost`；
- 相同展示版本、不同二进制 digest 不兼容；
- BuildID 计算失败时 durable host 拒绝启动；
- 自动 snapshot 写失败使 Process / Run 失败且不继续调用模型 / 工具；
- 单 Process snapshot 损坏 / 缺失；
- failure 后 interrupt / snapshot 清理；
- 重复恢复请求幂等。

### 12.6 完成条件

- BuildID 与 SnapshotFailurePolicy 在组合根可见；
- ProcessStore 开启时不可能以空 / `"dev"` BuildID 启动；
- adapter 无硬编码版本；
- 不兼容状态没有 migration / fallback；
- 自动 snapshot 写失败不会继续非持久化执行；
- 恢复失败事件与清理行为有测试；
- 全量与 standalone 门禁通过。

---

## 13. B6 — Child 执行上下文与成本归属

### 13.1 目标

先建立 host 显式提供 child execution context 的通路，使 provider / model、运行身份、预算与取消覆盖委派树；本批不处理 nested HITL。

### 13.2 目标行为

- Run 选择的 provider / model 默认传递给所有 child。
- child 不回退到全局默认 provider，也不把成本记到默认 provider。
- child model calls 保留本 Run 的 provider identity、model identity 和定价归属。
- root 的总 token / cost / step budget 聚合完整委派树。
- root cancel 传播到同步运行的 child，不留下 orphan process。
- child 继续保持独立对话上下文，不继承 parent conversation history。
- working directory 等已经声明为 protected ambient 的值继续按 Agent 既有 blackboard 契约继承。
- 将来若引入 child model role，必须是新的显式产品能力，本轮不预埋。

### 13.3 实施边界

- 先确认 Agent 当前 child API 是否能接收 host 明确给出的 `ProcessOptions` / execution metadata。
- 若只缺 additive option/provider hook，可在 Agent 增加最小显式扩展点。
- 不改变 Agent 默认的“child 只继承最小 capability”安全策略。
- 本批不把 approval observer 注入 child，不尝试在 `waiting` JSON 上补救 suspension。
- `task` 工具在 B8 完成前继续保持保守审批等级，作为 child side effect 的整体验证门。

若本批需要破坏性 exported API 修改，按 §0.2 暂停咨询；不得为避开公开 API 改动而让 Runtime 通过全局变量或 context 私有 key 偷渡 child policy。

### 13.4 必测场景

- root 显式 provider/model，child 实际调用同一 client；
- child model attribution 不落到默认 provider；
- `UsageByModel`、CostUSD 和 root 总预算包含 child 消耗；
- child 超预算能正确终止并反馈 parent；
- cancel root 时同步 child 结束；
- child 保持 clean conversation，不读 parent 历史；
- Agent 未配置 host child options 时保持原默认继承行为。

### 13.5 完成条件

- provider/model、identity、accounting、budget、cancel 覆盖委派树；
- 没有全局放宽 Agent inheritance；
- 没有引入半成品 child HITL 路径；
- Agent 与 Runtime 相关测试、race、standalone 全绿。

---

## 14. B7 — Agent Generic Nested Suspension / Checkpoint 原语

### 14.1 目标

在 Agent 根因层表达“作为 parent tool 同步运行的 child 可以暂停 parent，并在恢复 child 后继续完成同一个 parent tool call”，不包含 Runtime 的 approval / question 业务类型。

### 14.2 行为契约

- child 进入 waiting 时，agent-as-tool 不再只能返回普通成功 `{"status":"waiting"}`。
- nested suspension 是显式控制流，不伪装成 tool result 或普通 error。
- parent 暂停时，snapshot 中可确定：
  - parent process；
  - child process；
  - parent 当前 interaction / tool call checkpoint；
  - 正在等待的 child suspension；
  - resume 后应返回的 continuation。
- resume 顺序固定为：
  1. 响应原 child suspension；
  2. 继续原 child；
  3. child 再次 waiting 时更新同一 nested relation；
  4. child terminal 后提取结果；
  5. 用该结果完成原 parent tool call；
  6. parent 从原 interaction checkpoint 继续。
- 已完成的 parent / child 工具副作用不得因 restore / resume 重放。
- child terminal 后 relation 和可删除 snapshot 有确定清理语义。

### 14.3 API 与 schema 门禁

先写 Agent 模块规格测试，再确定最小 API。Agent 只允许表达：

- generic child options；
- generic nested suspension；
- durable parent/child/tool-call relation；
- resume / continue / cleanup。

禁止把 Runtime 的：

- approval；
- question；
- wire interrupt；
- RunID / Session item；

引入 Agent Core。

若需要修改 exported type、函数签名或 `ProcessSnapshot` shape，必须先在本文记录：

- 拟改 API / schema；
- Agent 模块所有消费者影响面；
- Runtime 集成影响；
- additive 方案为何不足；
- 不做历史兼容的清理范围；

然后暂停咨询用户。确认后直接采用最终 shape，不保留旧 schema reader。

### 14.4 必测场景

- child 首次 suspension 暂停 parent；
- child 连续两次 suspension；
- child resume 后 terminal，再继续 parent；
- parent / child snapshot round-trip；
- parent snapshot 引用的 child snapshot 缺失时明确失败；
- engine restart 后恢复 nested relation；
- resume 不重复 parent tool call；
- child terminal / failed / killed 的清理；
- cancel parent 时 child 一并终止；
- standalone agent tool 仍可保留其面向外部 host 的 waiting result 语义，不被 parent nested 语义误改。

### 14.5 完成条件

- Agent 层具备 generic、持久化、可恢复的 nested suspension；
- parent/child/tool checkpoint 行为由 Agent 测试证明；
- Runtime 业务类型未泄漏进 Agent/Core；
- 不存在旧 snapshot 兼容路径；
- Agent 全量、race 通过，底层提交已推送。

---

## 15. B8 — Runtime HITL Envelope、恢复与取消集成

### 15.1 目标

把 B7 的 generic nested suspension 翻译成同一个应用 Run 的 typed interrupt，并让 per-tool approval / hooks 覆盖 root 与 child。

### 15.2 Runtime HITL 防腐契约

建立应用自有、带 discriminator 的 typed envelope：

- `approval`：tool、arguments、risk / policy context；
- `question`：prompt、answer schema / choices。

具体类型名以实施时领域语言为准，但必须满足：

- 只在 Agent adapter 边界解码一次；
- application / domain 不再通过 `toolName`、`questions` 等 JSON key 猜类型；
- 未知 kind 明确失败，不静默回退；
- resume payload 按对应 schema 验证。

### 15.3 委派树工具策略

- root 与 child 的每次副作用工具调用都经过同一 approval policy 和 pre/post hooks。
- hooks 对每个逻辑 tool call 恰好执行一次。
- child safe tool 不误触发审批。
- child approval / question 通过 B7 nested suspension 提升为同一个应用 Run 的 interrupt。
- B8 的 per-tool gating 全部通过后，将 `task` 从副作用审批对象降为纯编排级别，消除 root + child 双重审批。
- 若 B8 未完成，`task` 保持保守审批等级，禁止提前降级。

### 15.4 恢复与取消

- Runtime 持久化足够的 Run ↔ root process ↔ child relation 索引。
- restart 后恢复 parked child 时：
  1. 定位原 Run 和 root；
  2. 恢复 Agent nested relation；
  3. 响应原 child；
  4. 继续 child 和 parent；
  5. 使用同一个 Run journal / event stream。
- root cancel 终止全部后代并关闭 pending interrupt。
- approve、deny、cancel 并发时只能有一个状态转换获胜。
- 无法恢复完整 process tree 时按 B5 策略转 `run_lost`，不新建 child 猜测继续。

### 15.5 必测场景

- child safe tool；
- child side-effect tool 的 approve / deny / edited input；
- root 与 child hooks 的调用次数和顺序；
- child question interrupt；
- child 二次 suspension；
- restart 后恢复 parked child；
- parent 可恢复但 child 缺失 / 损坏时转 `run_lost`；
- cancel parked child；
- cancel 与 approve 同时发生；
- child terminal 后无 orphan process / snapshot / interrupt；
- tool 副作用不因 resume 重复执行；
- unknown suspension kind / invalid resume payload；
- `task` 在 per-tool gating 完成前后风险等级的唯一切换点。

### 15.6 完成条件

- `waiting` 不再作为 Runtime parent-child suspension 的普通工具成功结果；
- approval、hooks、interrupt、resume、cancel 覆盖完整委派树；
- child HITL 可持久化恢复；
- typed envelope 取代 JSON key 猜测；
- Runtime 与 Agent 相关测试、race、standalone 全绿。

---

## 16. B9 — `agentexec` 职责与资源所有权收敛

### 16.1 目标

在行为契约稳定后，缩小 `adapter/agentexec.Engine`，恢复 consumer-side 窄依赖和 Host 资源所有权。

### 16.2 目标形态

`agentexec` 顶层运行对象只保留高度内聚的：

- Agent engine / deploy；
- process create / restore / control；
- 若确实属于 execution 防腐层的 prompt / action 能力。

以下能力不再经胖 Engine 中转：

- `turn.Dispatcher` 直接接收其真实使用的 steering / compactor / extractor 等窄依赖；
- integrations 直接接收 MCP live ports；
- tool registry 直接接收 tool catalog / source；
- maintenance 使用自己的 adapter / port；
- `bootstrap.Host` 直接保存并关闭 `built.Closers`。

### 16.3 接口策略

- 删除仅包装一个具体实现、没有替换价值的 `processStarter` / `processRestorer` / `processControl` 等内部接口。
- Agent adapter 内部可直接持有一个内聚具体运行时。
- 保留真实 consumer-side interface，例如 `turn.engineDep`，但只包含实际使用方法，并随 B3 新签名更新。
- 不建立新的 `Manager` / `Service` / `Facade` 作为换名后的胖对象。

### 16.4 资源关闭

- closers 从创建时起归 Host。
- Host 负责唯一、逆序、幂等关闭。
- Engine close 仅关闭它真正创建且独占的 Agent 资源；若无此类资源则不提供空壳 Close。
- shutdown 与 active Run / child 的顺序必须明确并有测试。

### 16.5 必测场景

- bootstrap 装配完整；
- Host shutdown 关闭顺序；
- 部分 bootstrap 失败时已创建资源回收；
- Engine 不再暴露 MCP / catalog / maintenance / closers 中转方法；
- turn dispatcher 的 mock / stub 只需实现窄接口；
- arch tests 阻止 delivery / application 依赖具体 Agent Engine。

### 16.6 完成条件

- Engine 的字段和方法都服务同一 execution 目的；
- Host 是 closers 的唯一最终所有者；
- 单实现空壳接口清理完成；
- 消费方依赖不因拆分变胖；
- 全量、race、standalone 门禁通过。

---

## 17. B10 — 架构适配测试、文档与最终清理

### 17.1 目标

把本轮新不变量写进机器可执行门禁，并清理重构残留。

### 17.2 架构与行为门禁

至少覆盖：

- Runtime standalone module 可构建；
- application 不直接依赖 Agent/Core 具体 engine；
- `bootstrap.Host` 持有 closers；
- turn 异步入口使用完整 Clone / snapshot；
- 通用 Options 校验不在 Runtime 重复字段实现；
- TurnOutput 使用单值停止原因；
- child suspension 不走普通 waiting result；
- provider / approval policy 的委派树测试；
- normal final 只读一个权威 tagged event；
- adapter 不硬编码 build version。

测试应优先验证行为和 import 边界。禁止写依赖源码字符串、易随格式变化而失效的脆弱测试，除非该规则确实只能通过静态源码检查表达。

### 17.3 清理

- 删除被替代 helper、字段、接口、测试 fixture；
- 删除 stale TODO、兼容注释和旧名字；
- 检查 exported / unexported 边界；
- 检查 error sentinel 与 `%w`；
- 检查 goroutine、timer、channel 关闭方；
- 检查 race、重复事件和重复 tool side effect；
- 运行 `go mod tidy`，确认无死依赖。

### 17.4 文档同步

更新：

- [`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md)：
  - managed Agent interaction 的真实边界；
  - 委派树与 child suspension；
  - Engine / Host 资源所有权；
  - Build identity 与恢复策略。
- [`EXTENSIBILITY.md`](EXTENSIBILITY.md)：
  - 新的真实可替换端口；
  - 删除已经不存在的胖 Engine 扩展描述。
- 本文：
  - 所有批次证据；
  - 最终偏差；
  - 决策日志；
  - 完成结论。

### 17.5 最终门禁

在 `app/runtime`：

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./...
```

对受影响的 Agent/Core 等模块分别执行：

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

若全量 race 因已确认的外部成本不可接受，必须在本文记录原因，并至少覆盖所有新增 / 修改的并发路径；不得无记录跳过。

### 17.6 完成条件

- 所有目标与不变量有代码或测试证据；
- 文档与实际实现一致；
- 无兼容层、无遗留 TODO、无临时双路径；
- 所有门禁通过；
- 每批提交可独立追溯，最终分支已推送。

---

## 18. 进度看板

实现进度只按“批次完成且全量门禁通过”计数，不按代码量估算。

| 批次 | 状态 | 开始 | 完成 | Commit | 验证摘要 |
|---|---|---|---|---|---|
| 基线审查 | 已完成 | 2026-07-16 | 2026-07-16 | `92b4147a5afd` 基线 | workspace 绿；standalone 失败已定位 |
| B1 依赖真相 | 已完成 | 2026-07-16 | 2026-07-16 | `09f32465afd4` | workspace / standalone build、vet、test、race 全绿；CI 固定门禁 |
| B2 输入值语义 | 已完成 | 2026-07-16 | 2026-07-16 | `3d9a6f33c444` | Core 校验委托；双异步入口完整快照；pure-media；workspace / standalone 全绿 |
| B3 启动错误 | 待执行 | — | — | — | — |
| B4 Interaction / 输出 | 待执行 | — | — | — | — |
| B5 Build / Snapshot | 待执行 | — | — | — | — |
| B6 Child Context / Accounting | 待执行 | — | — | — | — |
| B7 Agent Nested Suspension | 待执行 | — | — | — | — |
| B8 Runtime HITL | 待执行 | — | — | — | — |
| B9 Engine / Ownership | 待执行 | — | — | — | — |
| B10 Fitness / Docs | 待执行 | — | — | — | — |

当前实现进度：**2 / 10**。

状态只允许：

- 待执行；
- 进行中；
- 已完成；
- 阻塞（必须写明阻塞事实和解除条件）。

---

## 19. 风险登记

| 风险 | 影响 | 控制措施 | 状态 |
|---|---|---|---|
| child suspension 需要 Agent 新原语 | 可能跨模块并涉及公开 API | B7 先写行为测试与影响面；破坏性 API 先咨询 | 开放 |
| workspace 掩盖依赖漂移 | 本地绿、独立发布失败 | B1 起固定 `GOWORK=off` 门禁 | 已关闭 |
| resume 重复执行工具副作用 | 数据损坏 / 重复外部动作 | checkpoint + parent/child relation + 恰好一次测试 | 开放 |
| timeout 所有权混淆 | 长工具被误杀 | model stream 与 tool context 分离 | 已确认 |
| 大范围结构拆分掩盖行为回归 | 难审查、难回滚 | 行为批次先行，B9 最后拆所有权 | 已控制 |
| 二进制 BuildID 改变导致旧 snapshot 丢失 | dev 数据不可恢复 | 明确接受，不迁移；确定性 `run_lost` | 已接受 |
| executable digest 无法读取 | durable Runtime 无法启动 | 启动失败；测试 / 嵌入 host 显式注入固定 BuildID | 开放 |
| docs 基准与当前实现漂移 | 后续决策被旧描述误导 | B10 同步架构文档；事实优先级固定 | 已确认 |

---

## 20. 决策日志

| 日期 | 决策 | 原因 | 影响 |
|---|---|---|---|
| 2026-07-16 | 保留 Run 中心架构，不把 Run 折叠为 Agent Process | 两者生命周期、持久化和协议职责不同 | 本轮是对齐与收敛，不推倒重写 |
| 2026-07-16 | 新建独立执行计划，不续写历史收敛计划 | 避免已完成状态与新任务混杂 | 本文成为当前唯一实施控制面 |
| 2026-07-16 | 不做任何历史兼容 | 项目第一法则与用户明确要求 | schema/API/snapshot 直接采用最终形态 |
| 2026-07-16 | child 采用显式 host policy，不改变 Agent 默认继承 | 保持 SDK 安全默认，同时满足应用 Run 策略 | B6-B8 通过最小 Agent 扩展点实施 |
| 2026-07-16 | child suspension 必须提升为应用 Run interrupt | waiting JSON 会破坏暂停、恢复和副作用语义 | B7-B8 解决 parent/child continuation |
| 2026-07-16 | BuildID 使用运行二进制 SHA-256，不复用展示版本 | 避免 dirty build / 构建参数不同却错误兼容 | 每次二进制变化使旧 snapshot 确定性失效 |
| 2026-07-16 | SnapshotFailurePolicy 固定为 fail-process | durable Run 不允许 snapshot 失败后继续非持久化执行 | active write failure 进入 execution error |
| 2026-07-16 | standalone module 是固定完成门禁 | `go.mod` 必须描述真实依赖 | 从 B1 起每批执行 `GOWORK=off` |
| 2026-07-16 | 委派 / HITL 拆为三个批次 | child context、框架原语、应用恢复的风险和回滚边界不同 | B6、B7、B8 各自独立全绿 |
| 2026-07-16 | Runtime 仓内依赖按同一已推送提交对齐 | 避免 MVS 拼出跨契约时期的混合模块图 | B1 统一 pin `cc8be60da95e`，CI 强制 standalone |

后续新增决策必须记录“为什么”，不能只记最终结论。

---

## 21. 批次执行记录模板

每批开始时复制以下内容到对应批次末尾或本节：

```markdown
### Bx 执行记录

- 状态：
- 开始时间：
- 完成时间：
- 实际 scope：
- 新发现：
- 与计划偏差：
- 公开 API 影响：
- 底层模块 Commit / Push：
- Runtime pin Commit / Push：
- 测试命令与结果：
- race 结果：
- standalone 结果：
- Commit：
- Push：
- 剩余风险：
```

---

## 22. 最终完成定义

只有同时满足以下条件，才能声明本计划完成：

- B1-B10 全部为“已完成”；
- workspace 与 standalone 的 build / vet / test 全绿；
- 修改的并发路径通过 race；
- pure-media、process create failure、ModelResponse / ToolResult final、child approval / question / resume / cancel、长工具 timeout、snapshot write failure / build mismatch 均有回归测试；
- provider/model identity、CostUSD / budget、approval、hooks 和 cancel 覆盖完整委派树；
- parent/child suspension 可跨重启恢复，且副作用不重复；
- `agentexec.Engine` 不再是 MCP、tools、maintenance、closers 的中转站；
- Host 恢复资源最终所有权；
- 架构适配测试和文档同步完成；
- 没有历史兼容、临时 shim、双路径或计划内 TODO；
- 所有提交已推送；
- 本文记录最终 commit、验证证据和已接受风险。

在此之前，任何“主要功能已经能跑”都不等于重构完成。
