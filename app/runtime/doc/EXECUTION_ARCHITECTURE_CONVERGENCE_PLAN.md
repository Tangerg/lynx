# Runtime Execution-Centered Architecture 收敛执行计划

> 状态：完成
> 建立日期：2026-07-13  
> 完成 checkpoint：`a94e63b91`（`codex/runtime-architecture-refactor`）
> 最近审查修复 checkpoint：`a03c123e3`
> 目标架构基准：[EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md)

## 0. 文档职责

本文档是 Runtime 后续架构重构的**执行控制面**，用于回答：

- 最终要达到什么状态；
- 当前已经完成了什么；
- 剩余差异是什么；
- 必须按什么顺序实施；
- 每一批允许和禁止修改什么；
- 如何证明一批工作真正完成；
- 长任务如何更新进度、记录决策并防止跑偏。

它不重复解释 DDD 或 Clean Architecture，也不替代目标架构文档。三类事实的优先级如下：

1. **目标与原则**：以 `EXECUTION_CENTERED_ARCHITECTURE.md` 为准；
2. **执行顺序、当前批次和进度**：以本文档为准；
3. **当前真实行为**：以代码、测试和数据库约束为准。

若三者不一致，不得通过修改文档掩盖差异。必须先判断是实现偏离、目标需要修订，还是本文档状态未更新，然后在决策日志中记录裁决。

## 1. 唯一目标

将当前“结构上分层正确，但部分完整用例和持久化投影语义仍由 Delivery 组织”的实现，收敛为：

```text
Domain Execution Model
        ↑
Application Run Use Cases + Run Lifecycle
        ↑                         ↑
Delivery Adapters           Driven Adapters
        \                         /
              Bootstrap Host
```

完成后的关键数据流必须是：

```text
wire request
  → Delivery decode / validate / map
  → one Application use case
  → Executor port
  → application-owned EngineEvent reduction
  → application-owned EventCommit + RunEvent
  → durable commit
  → application Journal<RunEvent>
  → Delivery protocol projection
```

核心判断不是目录名称，而是**决策所有权**：

- Domain 决定允许的状态和纯业务规则；
- Application 决定完整用例、副作用顺序、事务语义和生命周期；
- Adapter 实现 Application/Domain 消费的外部能力；
- Delivery 只处理 wire，不组织跨组件业务流程；
- Bootstrap 只装配、启动和关闭。

## 2. 执行原则与硬边界

### 2.1 执行原则

1. 批次用于控制决策和验收边界；当分批会制造短命兼容层时，允许通过决策日志批准原子合并执行。
2. 每个提交只解决一个可描述的语义问题，并能独立回退。
3. 先移动所有权，再清理命名和目录；不得以重命名代替职责迁移。
4. 当前开发期不为历史 wire artifact、数据库 schema 或错误 exported API 保留兼容；改变现行用户协议时仍须单独列出影响。
5. 不保留明知错误的兼容层、双写、双事件族或“以后删除”的永久 shim。
6. 中间提交可以为同一批次服务，但一批只有在其临时桥接全部删除后才能标记完成。
7. 每批结束必须全量验证并更新本文档；测试通过不是可选收尾步骤。

### 2.2 明确不做

- 不引入 DI container、EventBus、Mediator 或 CQRS/Saga 框架；
- 不建立 `AggregateRoot` marker 或统一 Repository 基类；
- 不为目录对称合并/拆分 package；
- 不把所有 domain 字符串机械包装成 value object；
- 不把 agent loop 插件化；
- 不为了减少本批改动而保留错误的数据流方向；
- 不在 Delivery 外再增加一层新的业务 facade；
- 不把 `runs.Coordinator` 推倒重写，优先复用已经验证的生命周期机制。

### 2.3 必须暂停并重新确认的条件

出现以下任一情况，当前任务不得自行扩大范围：

- 需要改变 D-008 已批准范围之外的现行公开协议 wire shape；
- 需要改变 D-008 已批准范围之外的外部可见 exported API；
- 需要保留旧数据库、artifact、API 或双模型兼容；
- 当前批次无法完成，必须依赖后续批次的临时设计才能合并；
- 发现目标架构与已证实的业务不变量冲突；
- 需要新增兼容层、双写或长期 feature flag；
- 工作区存在无法安全绕开的用户修改；
- 同一阻塞条件重复出现且无法通过当前权限解决。

## 3. 当前基线

### 3.1 已完成并应保留的成果

以下工作视为基线，不应在后续批次中被重新发明：

- Domain / Application / Adapter / Infra / Delivery / Bootstrap 的源码依赖方向已建立；
- `internal/runtime` 业务 facade 已删除；
- `bootstrap.Host` 拥有资源关闭顺序，`Stack` 只负责发现和交付；
- live Run registry、Journal、pump、cancel owner 和 component lifetime 已进入 `application/runs`；
- RunID 在 resume 前后稳定，SegmentID 表达每次流式执行段；
- application 拥有唯一 `EngineEvent` 事件族，turn adapter 通过 alias 直接发出；
- durable event 已实现 commit-before-publish；
- fresh start 的 admission 与 opening projections 已通过 `CommitOpening` 原子提交；
- resume 已实现 prepare → attach/commit opening → activate 的所有权顺序；
- interrupt consume、Run resume state 和 opening projections 在同一事务提交；
- SQLite partial unique index 保证一个 Session 至多一个 non-terminal Run；
- Session fork/rollback/restore/delete/cancel 已收敛为原子 write-set；
- 文件 rollback 已建立可恢复的 workspace mutation intent；
- architecture fitness tests 已覆盖主要跨环依赖和生命周期字段所有权；
- 关键 runs/sessions/runsegment 测试及 race 检查已建立。

这些成果已经解决了旧架构最危险的问题：生命周期分散、状态多源、interrupt/resume 非原子、publish 早于 commit，以及 composition root 兼任业务 facade。

### 3.2 执行前核心差异（现已收敛）

以下内容保留为本计划的历史问题定义，不再描述当前实现。对应收敛结果见 §17 执行记录。

#### 差异 A：完整 Run 用例仍由 Delivery 编排

当前 `Server.StartRun` 依次调用 turn control、sessions admission、working-tree admission、ID/Clock、projector 和 `runs.Coordinator.Start`。Resume、Cancel、Steer 也存在同类跨组件编排。

因此当前 `runs.Coordinator.Start` 实质上是“打开并监督一个已准备好的 segment”，不是用户可见的完整 `runs.start` 用例。

#### 差异 B：事件和持久化投影方向仍然反向

当前路径为：

```text
EngineEvent
  → Delivery Projector
  → protocol.StreamEvent
  → Delivery 从 wire event 生成 EventCommit
  → Application commit + Journal<protocol.StreamEvent>
```

虽然 application 没有 import protocol，但 `runs.Projection` 的具体实现仍是 `protocol.StreamEvent`。这是结构依赖合规、语义所有权仍在外层的状态。

#### 差异 C：Transcript 是 wire snapshot，不是 canonical execution projection

`transcript.Item/Run` 通过 `json.RawMessage` 保存 `protocol.Item/RunRef`。这导致 rollback/fork/recovery 必须由 Delivery 解码 wire blob，再把 boundary 回调给 Application。

#### 差异 D：Run identity 与状态模型仍不完整

- RunID/SegmentID 由 Delivery 使用 protocol prefix 生成；
- Clock 分散在 Delivery、Application 和 Adapter；
- `EventCommit` 的状态转换没有显式 RunID；
- Suspend/Terminalize 主要按 SessionID 定位 active Run；
- `Created → Running` 存在于 domain 状态机，但生产 admission 直接持久化为 running；
- TurnID、ProcessID 和 opaque handle 在部分边界仍有具体 adapter 类型泄漏。

#### 差异 E：Delivery 仍是内部调用入口

schedule worker 通过 Delivery `Server.StartRun` 构造 protocol request 启动内部 Run。内部 Application 组件不应反向借用 wire handler。

#### 差异 F：Domain 仍有少量 I/O

`domain/worktree` 直接读取文件系统并解析 symlink，`domain/editguard` 直接读取文件和计算 hash。它们不是当前最高优先级，但仍偏离纯规则模型。

### 3.3 进度口径

进度必须区分两件事：

| 口径 | 当前值 | 说明 |
|---|---:|---|
| 原始重写目标总体完成度 | 100% | 四批目标及最终验收全部完成 |
| 本文档剩余收敛计划 | 4 / 4 批 | 已封账，无剩余批次 |

不得用代码行数、文件数或提交数计算进度。只有批次完成判据全部满足，才能增加进度。

## 4. 最终完成判据

以下条件必须同时成立：

### 4.1 Application 与完整用例

- `application/runs` 暴露 Start、Resume、Cancel、Steer、Subscribe 等用户可见用例；
- 阅读 `application/runs` 可以理解 Run 从请求接纳到 terminal/interrupt 的完整流程；
- Delivery 的每个 Run command handler 只调用一个 Application 用例入口；
- scheduler 直接调用 Application Run 用例，不经过 Server/protocol；
- executor handle 对 Delivery 完全 opaque，Delivery 不再 type assert 为 `turn.TurnHandle`。

### 4.2 Event Pipeline

- Application Journal 只保存 application/domain 定义的封闭事件族；
- `protocol.StreamEvent` 只在 Delivery 中创建和消费；
- Application 根据 EngineEvent 生成 canonical RunEvent 与 EventCommit；
- durable fact 在 protocol projection 之前形成；
- Delivery 不实现决定 terminal/interrupt/commit 语义的 Projector；
- commit-before-publish、terminal owner 和 interrupt linearization 保持不变。

### 4.3 Persistence 与领域模型

- Transcript 持久化 canonical execution/read-model 数据，不保存 protocol DTO 作为领域事实；
- rollback/fork/recovery boundary 可在 Application/Domain 中独立解析；
- Pending Interrupt 保存 typed canonical payload，wire JSON 只属于 Delivery/read model；
- Run 状态转换显式携带 RunID + SessionID；
- SQLite transition 使用 RunID + SessionID + expected state 做严格 CAS；
- Run 状态机与实际持久化状态完全一致，不保留未使用的 transition。

### 4.4 分层与生命周期

- Bootstrap 继续只装配和关闭；
- Delivery 不持有 Run 生命周期状态，也不组织跨 coordinator 完整用例；
- Domain 不执行文件、网络、数据库、进程或 telemetry I/O；
- 每个 goroutine 能明确回答 owner、cancel、join；
- application 不 import agent SDK、SQLite、Git、MCP、LSP 或 protocol；
- architecture fitness tests 能阻止上述关键边界回退。

### 4.5 验证

- protocol golden 全部保持预期；
- SQLite contract tests 证明 admission、resume、interrupt、terminal 和 rollback 原子性；
- application tests 证明 request cancel、commit-before-publish、resume ordering、cancel race 和 close/join；
- `go build ./...`、`go vet ./...`、`go test ./...` 全绿；
- 关键并发包 `go test -race` 全绿；
- 文档、注释和实现不存在互相矛盾的所有权声明。

## 5. 执行路线总览

```text
Batch 1  完整 Run 用例进入 Application
   ↓
Batch 2  Application-owned Event + Projection Pipeline
   ↓
Batch 3  Canonical Transcript / Interrupt / Run Identity
   ↓
Batch 4  Domain 纯化、Fitness Tests 与最终清理
```

批次顺序是依赖关系，不是建议顺序：

- 没有 Batch 1，scheduler 和 Delivery 无法共享同一 Run 入口；
- 没有 Batch 2，Transcript 仍只能从 wire event 反推；
- 没有 Batch 3，rollback/fork/recovery 仍需 Delivery 解析历史；
- Batch 4 必须在稳定形态上收紧规则，否则测试会固化中间设计。

## 6. Batch 1：完整 Run 用例进入 Application

> 状态：已完成（checkpoint `5cdd31c53`）
> 目标：Application 拥有 Start/Resume/Cancel/Steer 的完整副作用顺序；现有 segment lifecycle 机制继续复用。

### 6.1 设计结果

`application/runs` 对外提供 protocol-neutral command/result：

```go
Start(ctx context.Context, cmd StartCommand) (StartResult, error)
Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error)
Cancel(ctx context.Context, cmd CancelCommand) error
Steer(ctx context.Context, cmd SteerCommand) error
Subscribe(ctx context.Context, runID RunID, after Cursor) (...)
```

命名以最终实现评审为准，不为复制模板而强制新增 `Service`、`UseCase` 或 interface。现有 `Coordinator` 可保留为公开用例入口，也可以把当前 segment 逻辑收为包内 supervisor，但不得形成两个平行 Run owner。

### 6.2 子步骤

#### 1.1 建立 command、result 与消费方 port

- 定义 protocol-neutral Start/Resume/Cancel/Steer 输入；
- 定义 Application 需要的 Session lookup/admission、working-tree gate、Executor control；
- 将 ID generator 与 Clock 放到 Application 消费面；
- 保留 executor handle 为真正 opaque token；
- 明确 fresh 与 continuation 的合法状态，不再由 Delivery 通过 callback 是否为 nil 表达业务命令。

#### 1.2 迁移 Start

- 移入 Session 解析与默认值应用；
- 移入 session/working-tree admission；
- 移入 RunID、SegmentID、CreatedAt 创建；
- 通过 Application-owned executor port plan/start turn；
- 调用现有 opening commit + segment pump；
- 失败时由同一用例负责 turn cleanup 和 admission release。

#### 1.3 迁移 Resume

- 移入 response 的 canonical resolution；
- 移入 interrupt claim、prepare/rehydrate；
- 移入 continuation SegmentID 创建；
- 保持 attach + opening commit 后才 activate；
- opening 失败时 interrupt 必须仍开放；
- activation 失败必须在已接纳 segment 内 terminalize。

#### 1.4 迁移 Cancel 与 Steer

- Application 统一处理 live/parked cancel 分支；
- 保持 cancel/interrupt linearization；
- Delivery 不再读取 live record 并手工重建 `turn.TurnHandle`；
- Steer 通过 Application executor-control port 定位并注入。

#### 1.5 移除内部对 Delivery 的依赖

- schedule runner 直接调用 `application/runs.Start`；
- Server 不再实现 Application 的 scheduled-run 执行策略；
- internal/background caller 与 HTTP/inprocess 共享同一 Application 用例。

#### 1.6 收窄 Delivery 消费面

- Run handlers 只负责 wire decode、command map、error map、result/event present；
- 如确有测试/多实现价值，由 Delivery 定义本地窄接口；
- 不为单一具体 coordinator 制造无意义 interface。

### 6.3 本批禁止事项

- 不重做 Journal/pump/registry；
- 不改变 protocol event shape；
- 不迁移 Transcript schema；
- 不同时清理所有 domain I/O；
- 不引入第二个 Run supervisor；
- 不通过新的 facade 把现有 Server 编排原样包起来。

### 6.4 完成判据

- [x] Server Start/Resume/Cancel/Steer 每个只调用一个 Application Runs 用例；
- [x] scheduler 不 import/use protocol request 启动 Run；
- [x] Delivery 不 import `adapter/agentexec/turn` 处理 Run command；
- [x] Delivery 不 type assert opaque executor handle；
- [x] fresh/resume/cancel 的原子性和竞态测试全部保留；
- [x] wire golden 无非预期变化；
- [x] 全量 build/vet/test 与关键 race 通过。

## 7. Batch 2：Application-owned Event + Projection Pipeline

> 状态：已完成（checkpoint `a94e63b91`）
> 依赖：Batch 1 完成  
> 目标：durable fact 和 canonical RunEvent 在 Application 中形成，Delivery 只做最终 protocol projection。

### 7.1 目标数据流

```text
Executor EngineEvent
  → application reducer
      ├─ canonical RunEvent
      ├─ runs.EventCommit
      └─ optional application nudge
  → commit when required
  → Journal<RunEvent>
  → delivery translator
  → protocol.StreamEvent
```

这里的 reducer/projector 是 Application 内部状态化投影器，不是 Delivery 实现的 callback port。它可以维护 text/reasoning/tool item 的打开与完成状态，但不能生成 protocol DTO。

### 7.2 子步骤

#### 2.1 定义 canonical RunEvent

- 区分 executor 输入事件 `EngineEvent` 与订阅输出事件 `RunEvent`；
- RunEvent 为封闭、强类型、transport-neutral 的 Application 事件族；
- 明确 durable/live、terminal/interrupt、cursor/timestamp 语义；
- 不把 wire item status、JSON-RPC code 或 protocol ID prefix 放进事件。

#### 2.2 将状态化翻译迁入 Application

- 迁移 message/reasoning/tool/interrupt/terminal 的状态累计；
- Application 根据 EngineEvent 决定 completed item、interrupt、terminal；
- application projector 生成 canonical persistence fact；
- terminal synthesis 和 cancel reason 归 Application；
- 删除由 Delivery 决定 lifecycle/commit 的行为。

#### 2.3 Journal 改为 canonical payload

- `runs.Event.Payload` 改为 application-owned RunEvent；
- 删除 `runs.Projection` marker；
- 删除对 `protocol.StreamEvent` 的 runtime type assertion；
- HTTP 和 inprocess 从同一 canonical Journal 独立映射 wire。

#### 2.4 Delivery 只保留 presentation translator

- translator 输入 canonical RunEvent，输出 protocol.StreamEvent；
- translator 不生成 EventCommit；
- translator 不决定 suspend/terminal outcome；
- translator 不参与 opening transaction；
- protocol-specific optimistic item ID 等只在 presentation result 中处理，必要的 canonical identity 由 Application 提供。

#### 2.5 删除旧链路

- 删除 Delivery 实现的 `runs.Projector`；
- 删除 `sideEffectEvent(protocol.StreamEvent)`；
- 删除 wire → EventCommit 的任何路径；
- 删除仅为 opaque wire payload 存在的 adapter callback。

### 7.3 本批完成判据

- [x] Application Journal 中不存在 Delivery/protocol concrete payload；
- [x] `protocol.StreamEvent` 只在 Delivery 包内出现；
- [x] EventCommit 在 protocol projection 前形成；
- [x] Delivery presenter 不 import/use domain EventCommit；
- [x] opening、interrupt、terminal 仍严格 commit-before-publish；
- [x] 同一 canonical RunEvent 经统一无状态 presenter 得到一致 wire；
- [x] reducer、presenter 与 resume/cancel race 测试通过；
- [x] 本批临时桥接全部删除。

## 8. Batch 3：Canonical Transcript、Interrupt 与 Run Identity

> 状态：已完成（与 Batch 2 原子切换，checkpoint `a94e63b91`）
> 依赖：Batch 2 完成  
> 目标：Execution 的 durable record 不再以 protocol DTO 为领域事实，Run 状态转换严格定位逻辑 Run。

### 8.1 子步骤

#### 3.1 重塑 Transcript

- 定义 canonical TranscriptItem、RunRecord 或等价结构；
- 明确 UI transcript 与 model conversation 是两个 projection；
- 持久化字段覆盖 rollback/fork/query 所需的结构化数据；
- protocol DTO 只由 Delivery 从 canonical record 投影；
- exact wire snapshot 如仍有真实需求，必须作为外层 read model 明确命名和归属。

#### 3.2 移回 boundary resolution

- Application/Domain 直接从 canonical RunRecord 构造 Timeline；
- rollback/fork/recovery 不再接受 Delivery 提供的 BoundaryResolver；
- recovery 可以在 Server 尚未构造时由 Application/Bootstrap 启动；
- dropped run response 由 Delivery 从 canonical result 映射。

#### 3.3 Typed Interrupt persistence

- Pending Interrupt 保存 canonical typed payload；
- approval/question/tool-result 的 wire 编码只在 Delivery；
- rehydrate 所需 ProcessID/TurnID 与用户可见 interrupt content 分离；
- Resume command 从 wire response 映射为 canonical resolution 后进入 Application。

#### 3.4 收紧 Run identity

- `EventCommit` 显式携带 RunID + SessionID；
- Admit/Resume/Suspend/Terminalize 全部针对同一个逻辑 Run；
- SQLite 使用 RunID + SessionID + expected state CAS；
- 新 Run admission 仍由 partial unique index 保证 Session 唯一；
- ProcessID 只属于恢复 adapter，不成为领域身份。

#### 3.5 对齐状态机

- 裁决 `Created` 是否是真实持久化状态；
- 若 admission 即开始运行，则删除未使用的 Created/Begin；
- 若需要 Created，则 opening transaction 必须显式执行 Created → Running；
- 删除状态机、注释、schema 三者之间的虚假状态。

### 8.2 Schema 与数据策略（已执行）

- SQLite current schema 升为 version 2；打开旧版本数据库时直接删除旧结构并安装当前 schema，不迁移历史数据；
- Transcript 与 Pending Interrupt 只序列化 canonical typed record，不保留 protocol JSON blob；
- session export/import artifact 升为 version 3，旧 version 2 明确拒绝；
- 删除旧 artifact 外层存储键和旧投影路径，不提供双读、双写或转换 shim；
- 当前 command/event wire contract 保持不变，只有持久化与导入导出 artifact 按当前正确模型切断历史。

### 8.3 本批完成判据

- [x] domain/application 不再把 protocol Item/RunRef blob 当作 authoritative record；
- [x] rollback/fork/recovery 不依赖 Delivery callback；
- [x] Pending Interrupt 不保存 wire interrupt JSON；
- [x] 每个 Run state transition 都严格携带并校验 RunID；
- [x] 状态机没有生产代码永不使用的 transition；
- [x] import/export、items/runs/query wire tests 通过；
- [x] SQLite 原子性与 CAS 竞态测试通过。

## 9. Batch 4：Domain 纯化、Fitness Tests 与最终清理

> 状态：已完成（checkpoint `a94e63b91`）
> 依赖：Batch 3 完成  
> 目标：删除中间残留，用机器规则固化最终架构，并完成全量验收。

### 9.1 Domain 纯化

- 将 worktree `os.Stat`、symlink/canonical path I/O 移到 workspace adapter；
- 将 editguard 文件读取/hash 变为 adapter 提供的 stamp，domain 只判断 read-before-edit；
- 审核 skills/recipes/agentdoc，只保留 parse、precedence、validation 等纯规则；
- 审核 domain 中的 Store/Registry interface，保留真实 domain consumer port，迁走 application use-case port；
- 不为纯 DTO 强造 aggregate。

### 9.2 收紧 architecture fitness tests

至少增加或等价覆盖：

- Delivery Run handlers 不得直接依赖 agentexec turn adapter；
- Application Journal payload 必须来自 application/domain；
- Delivery 不得定义 wire event → EventCommit 的生产路径；
- scheduler runner 不得通过 Delivery Server 启动 Run；
- domain 全环禁止 `os`、`path/filepath` 中涉及 I/O 的能力；
- Run state writer 的 mutation contract 必须携带 RunID；
- Bootstrap exported 类型仍只允许生命周期方法；
- protocol 包仍不 import domain/application。

AST 无法可靠判断的语义规则，应通过编译期类型封闭、package API 形状和行为测试共同保证，不编写脆弱的字符串匹配假测试。

### 9.3 清理与同步

- 删除旧 Projector/Projection/BoundaryResolver/opaque handle 桥接；
- 删除未使用类型、状态、接口和注释；
- 统一 Run/Segment/Turn/Process 术语；
- 更新目标架构文档中的“落地状态”；
- 更新本文档进度和最终决策；
- 检查 README/doc 索引与代码 godoc。

### 9.4 本批完成判据

- [x] Domain 生产代码无文件系统、数据库、网络、进程、telemetry I/O；
- [x] architecture tests 能阻止关键语义边界回退；
- [x] 不存在中间兼容层、双事件族、死接口或过期注释；
- [x] 全量 build/vet/test/race 通过；
- [x] 目标架构文档完成判据逐项验收通过；
- [x] 本文档状态更新为“完成”。

## 10. 进度看板

### 10.1 批次状态

| 批次 | 目标 | 状态 | 完成度 | 当前阻塞 | 验证证据 |
|---|---|---|---:|---|---|
| Baseline | 分层、生命周期、原子 opening、stable RunID | 已完成 | 100% | 无 | 基线 `581005b50`；现有 arch/runs/sessions/runsegment tests |
| Batch 1 | 完整 Run 用例进入 Application | 已完成 | 100% | 无 | `5cdd31c53`；full build/vet/test + key race |
| Batch 2 | Application-owned event pipeline | 已完成 | 100% | 无 | canonical reducer/journal；旧 Projector/translator 链已删除 |
| Batch 3 | Canonical durable execution record | 已完成 | 100% | 无 | typed transcript/interrupt；strict RunID CAS；schema v2/artifact v3 |
| Batch 4 | 纯化、fitness tests、最终清理 | 已完成 | 100% | 无 | `a94e63b91`；domain I/O 迁移、architecture tests、full/race/frontend 验收 |
| Post-audit | Go 审查发现的不变量、并发与资源边界修复 | 已完成 | 100% | 无 | `7ff31775f`；strict artifact/identity、lossless Journal、bounded health probe、full race |
| Post-audit II | 便携快照、parked cancel、interrupt 与 rollback 子树边界 | 已完成 | 100% | 无 | `75adcec22`；atomic snapshot/cancel/subtask rollback、detached cleanup、full race |
| Post-audit III | durable recovery、park batch、ProcessSnapshot 与 Session 子树 admission | 已完成 | 100% | 无 | `a03c123e3`；atomic park batch、durable `run_lost`、snapshot lifecycle、full race |

### 10.2 当前执行指针

```text
Current batch: complete (including three post-convergence audit remediations)
Current sub-step: none
Last completed code checkpoint: a03c123e3
Next required gate: none
```

每次开始新的子步骤或提交后，必须更新这里。不得只修改下方历史记录而保留过期指针。

## 11. 验证门禁

### 11.1 每个提交前

在 `app/runtime` 模块执行：

```bash
gofmt -w <changed-go-files>
go test <affected-packages>
```

并检查：

- 是否只包含当前子步骤；
- 是否意外改变 protocol/schema/exported API；
- 是否新增 `any`、callback 或 interface 来掩盖层间具体类型；
- 是否产生临时桥接，且桥接是否仍在本批删除清单内；
- 是否同步修改受影响的注释和测试。

### 11.2 每个批次完成前

在 `app/runtime` 执行：

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

同时执行：

- protocol golden；
- SQLite 真实临时库 contract tests；
- 当前批新增的高重复并发测试；
- `git diff --check`；
- architecture fitness tests；
- 从失败路径反向检查资源 cleanup 和 admission release。

### 11.3 不允许的验收方式

- 只跑新增测试；
- 用 mock 证明 SQLite 原子性；
- 用随机 sleep 证明并发顺序；
- 因为 arch test 通过就宣称策略所有权正确；
- 因为 wire 未变化就忽略 durable record 语义变化；
- 将已知失败标记为“与本次无关”后继续提交，除非有明确基线证据且未触碰相关代码。

## 12. 防跑偏检查表

每次准备修改代码前，回答以下问题：

1. 当前修改属于哪个批次和子步骤？
2. 它改变的是所有权，还是只改变了名字/目录？
3. 这个业务决策是否仍由 Delivery callback 提供给 Application？
4. Delivery handler 是否只调用一个完整用例？
5. durable fact 是否在 protocol projection 之前形成？
6. Application Journal 是否仍可能携带 outer-layer concrete value？
7. 状态转换是否明确针对 RunID，而不是“当前 Session 的某个 Run”？
8. failure path 谁释放 admission、取消 executor、关闭 stream？
9. goroutine 的 owner、cancel、join 分别是谁？
10. 是否为了省改动引入了兼容层、双写或空接口？
11. 当前测试证明的是业务不变量，还是只证明代码能运行？
12. 本次改动是否要求先确认 protocol/schema/exported API？

有任一问题无法明确回答，不应继续扩大代码修改。

## 13. 提交与批次策略

建议提交序列如下，实际可按依赖细分。D-008 之后 Batch 2/3 是一次原子切换：

```text
Batch 1
  1. application runs commands + ports
  2. move fresh start orchestration
  3. move resume orchestration
  4. move cancel + steer orchestration
  5. route scheduler directly to runs
  6. thin delivery + remove old seams

Batch 2 + 3 atomic cutover
  1. one canonical execution/transcript/interrupt model + application reducer
  2. typed SQLite persistence (discard the old schema; no migration)
  3. canonical journal + stateless delivery projection
  4. move rollback/fork/recovery boundary resolution inward
  5. strict RunID state transitions + align the state machine
  6. remove Projector/Projection/blob/sideEffectEvent and every transition shim

Batch 4
  1. remove residual domain I/O
  2. tighten architecture fitness tests
  3. delete dead seams and synchronize docs
  4. full acceptance verification
```

每批结束时应有一个明确的 checkpoint commit。不得把多个批次压成一个无法独立评审和回退的大提交。

## 14. 风险登记

| 风险 | 触发批次 | 影响 | 控制方式 |
|---|---|---|---|
| Start/Resume 所有权迁移破坏 admission release | Batch 1 | session 永久 busy 或双 Run | failure-path tests + barrier race tests |
| Resume 在 attach 前 activate | Batch 1 | continuation 事件丢失、ghost resume | 保留 opening transaction 与 activation ordering tests |
| 两个 Run owner 并存 | Batch 1 | cancel/terminal 重复、registry 状态分叉 | 复用现有 coordinator；禁止第二 supervisor |
| canonical event 与 wire event 漂移 | Batch 2 | 客户端事件行为变化 | protocol golden + HTTP/inprocess projection contract |
| terminal synthesis 行为变化 | Batch 2 | 错误被误报 canceled 或反之 | application reducer terminal matrix tests |
| transcript 重塑破坏 rollback/fork | Batch 3 | 历史截断错误 | Timeline table tests + SQLite integration tests |
| interrupt typed 化破坏重启恢复 | Batch 3 | parked Run 无法 resume | real SQLite rehydrate/resume tests |
| RunID CAS 过严暴露旧竞态 | Batch 3 | 旧代码测试失败 | 不放宽 CAS；修复错误 owner/ordering 根因 |
| domain I/O 迁移造成过度抽象 | Batch 4 | 接口膨胀 | 只为真实 I/O 边界定义 consumer interface |
| AST 规则过度脆弱 | Batch 4 | 正确重构被字符串规则阻塞 | 优先类型封闭和编译依赖，AST 只查稳定结构 |
| park batch 被拆分提交或发布 | Post-audit III | durable timeline 缺少 interrupt item，cancel 可观察到半成品 | 在 cancel linearization 内原子提交并发布完整 park batch；真实 SQLite contract test |
| 启动时保留不可恢复的 Interrupted Run | Post-audit III | admission 永久占用或 resume 指向无效进程 | 校验完整 parked boundary 与 ProcessSnapshot；外部丢失持久化为 `run_lost` |
| 删除/rollback 子树时子 Session 启动 Run | Post-audit III | 活跃执行被删除或工作区部分恢复 | 文件 mutation 前 claim 完整 KindSubtask 子树；删除与 snapshot cleanup 同事务 |

## 15. 决策日志

只记录会影响后续执行方向的裁决，不记录普通代码细节。

| ID | 日期 | 决策 | 原因 | 状态 |
|---|---|---|---|---|
| D-001 | 2026-07-13 | 保留现有 runs registry/journal/pump 机制，在其上收拢完整用例 | 该部分已通过原子性、生命周期和 race 验证，问题是入口所有权而非机制本身 | 已接受 |
| D-002 | 2026-07-13 | Batch 1 默认不改变 wire/schema | 先将完整用例内收，缩小变量数量 | 已接受 |
| D-003 | 2026-07-13 | Application 必须在 protocol projection 前生成 RunEvent 与 EventCommit | 防止 wire 模型反向决定持久化和生命周期 | 已接受 |
| D-004 | 2026-07-13 | protocol snapshot 不再作为 Execution authoritative record | rollback/fork/recovery 必须独立于 Delivery | 已接受 |
| D-005 | 2026-07-13 | Run state transition 必须显式定位 RunID + SessionID | 用 aggregate identity 防止错误 segment/late event 修改其他 Run | 已接受 |
| D-006 | 2026-07-13 | 独立 infra 环、Session/Todo 的物理分包不回滚 | 这些是合理的 Go/工程化偏离，不影响 bounded context 与依赖规则 | 已接受 |
| D-007 | 2026-07-13 | Batch 1 只保留既有 ProjectorFactory 作为 Batch 2 的明确迁移边界，不允许其携带 executor handle 或表达 fresh/resume 模式 | Batch 1 禁止同时重写 event pipeline；先移除命令编排与 handle 泄漏，再在 Batch 2 一次删除 outer projection seam | 已完成，seam 已删除 |
| D-008 | 2026-07-13 | Batch 2 与 Batch 3 合并为一次原子切换；旧 transcript/interrupt schema 直接丢弃，不写 migration，不用 canonical→旧 wire JSON blob 过渡 | 用户明确要求按第一、第二法则实施且不做任何历史兼容；分批保留 blob 会在错误的持久化边界上制造短命 shim，并让 Delivery 语义继续反向渗透 | 已完成 |
| D-009 | 2026-07-13 | Session Cwd 在进入 Application 时经 `CwdResolver` 一次性解析为 canonical path；Domain 与后续用例信任该不变量，不重复访问文件系统或防御性 canonicalize | 路径身份是外部世界解析结果，重复解析会把 I/O 泄漏回 Domain 并制造多套 workspace identity | 已接受并实施 |
| D-010 | 2026-07-13 | Run ID 永久归属一个 Session，Item ID 永久归属一个 Session+Run；artifact 在任何写入前完成严格 union、引用关系和 run-tree 校验 | identity re-parent 会跨 aggregate 覆盖 durable fact；宽松 decode 会把损坏状态带入事务，二者都必须在持久化边界被拒绝 | 已接受并实施 |
| D-011 | 2026-07-13 | Journal 为每个 subscriber 建立独立投递泵；durable/terminal 进入有序队列且不受 live-only 丢弃预算约束，只有过量 live-only event 可丢弃，关闭后的遗弃订阅有明确退出上限 | 原先 channel 满时可能漏掉 durable event，却仍送达 terminal，使客户端无法通过重连察觉历史缺口 | 已接受并实施 |
| D-012 | 2026-07-13 | 每个 HealthProbe 最多一个 in-flight invocation；并发请求共享该调用，各自受请求 budget 约束 | 单纯给请求加 timeout 无法停止忽略 context 的 probe，重复请求会持续泄漏 goroutine | 已接受并实施 |
| D-013 | 2026-07-13 | Session artifact 只表达 idle Session 的 terminal transcript；导出先占 admission 并在一个事务中读取完整快照，导入拒绝 running/interrupt Run | artifact 不携带 admission、parked executor 或 ProcessSnapshot；接受非终态只会生成无法恢复的孤儿状态，分次读取也不能保证 round-trip 自洽 | 已接受并实施 |
| D-014 | 2026-07-13 | Parked cancel 必须原子提交 canceled Run、incomplete interrupt items、terminal watermark/result 与 interrupt/admission 关闭；未知或 malformed executor interrupt fail closed 为 error terminal | 只删除 interrupt/admission 会留下幽灵 Interrupted transcript；把未知 kind 强制当 question 会把 adapter 错误持久化为合法业务状态 | 已接受并实施 |
| D-015 | 2026-07-13 | Rollback 只删除归属于 dropped window 的 KindSubtask 子树，并把子树删除并入父 rollback write-set；用户 fork 永远保留 | ParentID 同时承载 fork/subtask lineage，按 ParentID best-effort 递归会误删用户 fork，吞错还会留下半删子树 | 已接受并实施 |
| D-016 | 2026-07-13 | Park 是一个完整 aggregate boundary：running interrupt items、Interrupted Run、Pending Interrupt、admission suspend 与 ProcessSnapshot reference 必须作为一个 write-set 提交，并在 cancel linearization 内整批发布 | 只提交最终 interrupt projection 会丢失此前 `ItemStarted` 事实；分项发布会让并发 cancel 观察到不可解释的半个 park | 已接受并实施 |
| D-017 | 2026-07-13 | 启动恢复必须修复 durable truth：只保留 transcript、interrupt、admission 与可用 ProcessSnapshot 完全自洽的 parked boundary；外部进程丢失持久化为 `run_lost`，不可能由原子写集产生的内部矛盾则启动失败 | read path 临时伪造 `run_lost` 会让不同客户端看到不同事实，也会掩盖数据库不变量破坏 | 已接受并实施 |
| D-018 | 2026-07-13 | ProcessSnapshot 属于 parked Run 的资源生命周期；cancel/delete/rollback/recovery 必须在同一 SQLite write-set 清理引用快照，parked cancel 先提交 durable terminal 再做进程侧 best-effort teardown | 先删进程快照再提交 cancel 会在事务失败时留下仍开放但不可恢复的 interrupt | 已接受并实施 |
| D-019 | 2026-07-13 | Session 用例自己拥有 admission：restore 在 Application 内 claim；fork 读取一致快照；delete/rollback 在任何文件 mutation 前 claim 全部待删除 KindSubtask 后代 | 把 claim 留给 Delivery 或分次读取/逐层删除，会允许内部调用绕过互斥并形成 TOCTOU 与部分工作区恢复 | 已接受并实施 |

新增决策使用递增 ID，并同步修改受影响批次的范围、完成判据和风险。

## 16. 执行记录模板

每个子步骤完成后追加一条，不覆盖历史：

```text
### YYYY-MM-DD — Batch X.Y — <summary>

- Commit: <hash>
- Changed ownership:
  - <从哪一层移动到哪一层>
- Invariants preserved/added:
  - <invariant>
- Removed seams/debt:
  - <removed type/callback/path>
- Validation:
  - <commands and results>
- Remaining within current batch:
  - <next concrete item>
- Decision log updates:
  - <none or D-xxx>
```

## 17. 执行记录

### 2026-07-13 — Baseline established

- Commit: `581005b50`
- 已确认当前代码具备分层骨架、application-owned segment lifecycle、atomic opening/resume、stable RunID/SegmentID、durable admission 与 rollback recovery；
- 已确认剩余差异集中在完整用例所有权、event projection 方向、wire-shaped transcript、Run identity/state targeting 和 Delivery 内部入口；
- focused verification：architecture、application/runs、application/sessions、adapter/runsegment、delivery/server tests 通过；
- race verification：application/runs、application/sessions、adapter/runsegment 通过；
- 下一步：Batch 1.1，先评审 Application Runs command/port 形状，再修改生产代码。

### 2026-07-13 — Batch 1 — complete Run use cases moved into Application

- Commit: `5cdd31c53`
- Changed ownership:
  - Start/Resume/Cancel/Steer 的 Session 解析、admission、working-tree gate、ID/Clock、turn prepare/rehydrate/activate 与 cleanup 从 Delivery/`sessions`/`turn.Control` 移入 `application/runs`；
  - scheduled execution strategy 移入 `application/schedules.RunLauncher`，HTTP 与 background worker 共享 `application/runs.Start`；
- Invariants preserved/added:
  - fresh opening 仍 admission + opening projections 原子提交；
  - resume 仍先 attach + commit opening，再 activate decision；
  - live/parked cancel 共用 session admission，cancel/interrupt linearization 保留；
  - request cancellation 只退订，不终止 coordinator-owned pump；
- Removed seams/debt:
  - 删除 adapter `turn.Control`；
  - 删除 `sessions` 中重复的 resume prepare/rehydrate/activate orchestration；
  - Delivery 不再 import agent turn adapter，也不再 type assert opaque handle；
  - raw prepared-segment `StartSpec` 收为 package-private `segmentSpec/openSegment`；
- Validation:
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `go test ./...`：通过；
  - `go test -race ./internal/application/runs ./internal/application/sessions ./internal/application/schedules ./internal/adapter/runsegment ./internal/delivery/server`：通过；
- Remaining within current batch: none；
- Decision log updates: D-007；
- 下一步：Batch 2.1，建立 Application-owned canonical RunEvent/reducer，并保持现有 wire golden 不变。

### 2026-07-13 — Batch 2 + 3 — canonical execution pipeline atomic cutover

- Commit: `a94e63b91`
- Changed ownership:
  - EngineEvent 的状态归约、item identity、terminal/interrupt synthesis、EventCommit 与 canonical RunEvent 从 Delivery 移入 `application/runs`；
  - Transcript、Interrupt、Run/Item durable record 从 protocol JSON blob 改为 `domain/execution/transcript` typed model；
  - rollback/fork/recovery timeline resolution 从 Delivery callback 移入 Application/Domain；
- Invariants preserved/added:
  - opening、interrupt、terminal 继续严格 commit-before-publish；
  - Suspend/Resume/Terminalize 使用 RunID + SessionID + expected state 严格 CAS；
  - cancel 只有一个 terminal owner，不再执行 live cancel 后的重复 durable terminalize；
- Removed seams/debt:
  - 删除 Delivery Projector、stateful translator、wire → EventCommit、BoundaryResolver 与 protocol payload marker；
  - 删除 `Created/Begin` 假状态，admission 直接建立 Running；
  - SQLite schema v2 遇到旧版本直接丢弃重建，artifact v3 拒绝旧版本，不保留 migration/双读/双写；
- Validation:
  - Runtime full build/vet/test 与关键 race：通过；
  - protocol、SQLite、import/export、resume/cancel/reducer tests：通过；
- Remaining within current batch: none；
- Decision log updates: D-008。

### 2026-07-13 — Batch 4 — domain purity, fitness tests, and closeout

- Commit: `a94e63b91`
- Changed ownership:
  - workspace canonical path 的文件系统解析移入 `adapter/workspacepath`，Application 通过 `CwdResolver` 在入口建立一次性不变量；
  - editguard 文件读取/hash 移入 tool adapter，Domain 只比较 fingerprint；
- Invariants preserved/added:
  - Domain 全环禁止文件系统、数据库、网络、进程和 framework I/O；
  - architecture guards 禁止 opaque execution record、outer projection seam、Delivery concrete turn control 回归；
  - 前后端 artifact v3 shape 同步删除冗余外层存储 identity；
- Removed seams/debt:
  - 删除 `domain/worktree` I/O package、死 Durability 类型、过期 migration/strict-transition 注释；
- Validation:
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `go test ./...`：通过；
  - `go test -race ./internal/application/runs ./internal/application/sessions ./internal/application/schedules ./internal/adapter/runsegment ./internal/delivery/server`：通过；
  - frontend Prettier、RPC sample test、TypeScript typecheck：通过；
  - `git diff --check`：通过；
- Remaining within current batch: none；
- Decision log updates: D-009。

### 2026-07-13 — Post-convergence Go review remediation

- Commit: `7ff31775f`
- Invariants preserved/added:
  - Transcript store 拒绝跨 Session/Run 重新绑定既有 Run/Item identity，真实 SQLite restore write-set 在冲突时整体回滚；
  - artifact v3 在 admission 和存储 mutation 前严格校验枚举、tagged union、message mark、引用归属、interrupt item、run tree 与 terminal item 状态；
  - Session restore 与 create/update 共用 canonical Cwd 不变量，不可解析目录在 mutation 前失败；
  - Journal 的 durable/terminal event 按 subscriber 顺序可靠投递，live-only backpressure 独立受限；
  - resume item identity 消费一次即从 exact/fallback 两个索引移除，drained tool 使用显式启动顺序；
  - 不遵守 context 的 HealthProbe 每个配置项最多占用一个 goroutine，未知 health status 归一为 unhealthy；
- Removed seams/debt:
  - 删除未使用的 workspace tool-event helper 和 test stub；
  - 清理剩余 goimports、现代 `errors.AsType` 与可直接类型转换的 lint 问题；
- Validation:
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `golangci-lint run ./...`：通过；
  - `go test ./...`：通过；
  - `go test -race ./...`：通过；
  - Journal、resume、artifact、restore、health 关键用例在 `-race -shuffle=on` 下重复 30–50 次：通过；
  - `git diff --check` 与 Go 格式检查：通过；
- Remaining work: none；
- Decision log updates: D-010、D-011、D-012。

### 2026-07-13 — Post-convergence lifecycle and portability hardening

- Commit: `75adcec22`
- Invariants preserved/added:
  - terminal Run 的 conversation watermark 解析失败会使整个 event transaction 失败，不再持久化 `-1` 假边界；
  - Session export 先 claim single-writer slot，再从一个 SQLite transaction 读取 metadata、conversation 和 transcript；active/open-interrupt Session 返回 `session_busy`；
  - artifact 只接受 terminal Run，且 terminal outcome 必须携带 Result 和已解析 watermark；running/interrupt artifact 在 mutation 前拒绝；
  - parked cancel 原子写入 canceled Run、incomplete interrupt items、取消原因、完成时间与 message watermark，同时关闭 interrupt/admission；
  - executor 发出 empty/unknown/mismatched interrupt union 时中止 turn，并通过 canonical error terminal 收口，不再默认强制转换成 question；
  - rollback 只删除 dropped run window 对应的 KindSubtask subtree，用户 fork 保留；父 timeline mutation 与 subtask subtree delete 在同一 write-set 回滚或提交；
  - post-commit turn/mutation cleanup 使用 `WithoutCancel` + bounded timeout，客户端取消不会跳过进程或 intent 清理。
- Removed seams/debt:
  - 删除 artifact 对 running/interrupt payload 的无效 decode/restore 路径及其 dead payload decoder；
  - 删除吞错的 best-effort `PurgeSubtree`，subtask 删除进入明确的 RollbackPlan；
  - 修正已失效的 workspace file-change 注释。
- Validation:
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `golangci-lint run ./...`：通过；
  - `go test ./...`：通过；
  - `go test -race ./...`：通过；
  - interrupt abort、parked cancel/export、detached cleanup、fork preservation、atomic subtask rollback 与 terminal watermark 用例在 `-race -shuffle=on` 下重复 20–30 次：通过；
  - `git diff --check` 与 Go 格式检查：通过。
- Remaining work: none；
- Decision log updates: D-013、D-014、D-015。

### 2026-07-13 — Post-convergence durable lifecycle truth audit

- Commit: `a03c123e3`
- Invariants preserved/added:
  - park 将本段所有 running interrupt items、Interrupted Run、Pending Interrupt、admission suspend 与 ProcessSnapshot reference 作为一个 EventCommit write-set；commit 与整批 Journal publish 均位于 cancel linearization 内；
  - terminal shutdown 使用脱离 owner cancellation 且有界的 context 合成 durable terminal；只有 terminal/park commit 成功后才执行 Finish maintenance，runsegment snapshot 固定使用 Run 启动时捕获的 Cwd；
  - 启动恢复在一个事务中审计 non-terminal Run：只保留完整可恢复的 parked boundary；进程快照缺失/不可用等外部损失持久化为 canonical `run_lost` terminal，原子事实内部矛盾则阻止启动；
  - ProcessSnapshot 随 cancel、session delete、subtask delete、rollback 与 orphan recovery 在同一 SQLite write-set 删除；parked cancel 先建立 durable terminal truth，再 best-effort teardown 进程侧 turn；
  - Snapshot read/restore 严格校验 Session/Run/Item ownership、terminal coupling、watermark、accounting、spawning reference 与 run-tree；restore admission 由 Application 用例自己持有；
  - fork 使用一个 coherent snapshot；Cwd mutation 进入 Session admission；delete 递归处理 KindSubtask 后代而保留用户 fork；rollback 在 workspace restore 前 claim 全部待删 subtask descendants。
- Removed seams/debt:
  - 删除 Delivery read path 中临时伪造 `run_lost` 的 live-run reconciliation；客户端读取只呈现已经提交的 durable fact；
  - 删除 Delivery 对 restore admission 的外置编排，内部调用无法绕过 Session single-writer 规则；
  - 删除分次读取 session history/transcript 的 fork/export 路径，以及脱离 ambient transaction 的 ProcessSnapshot cleanup。
- Validation:
  - `go build ./...`：通过；
  - `go vet ./...`：通过；
  - `golangci-lint run ./...`：通过；
  - `go test ./...`：通过；
  - `go test -race ./...`：通过；
  - park write-set、boot recovery、ProcessSnapshot rollback、session subtree admission 与 coherent snapshot 使用真实 SQLite/并发测试覆盖；
  - `git diff --check` 与 Go 格式检查：通过。
- Remaining work: none；
- Decision log updates: D-016、D-017、D-018、D-019。

## 18. 最终验收清单

架构收敛完成时逐项勾选：

- [x] `application/runs` 展示完整 Start → Resume/Cancel → Terminal 流程；
- [x] Delivery Run command handler 只 decode/call/present；
- [x] scheduler 与 transports 共享同一 Application Runs 入口；
- [x] Delivery 不理解 executor handle；
- [x] Application Journal 不携带 protocol concrete payload；
- [x] EventCommit 不由 wire event 反推；
- [x] Transcript/Interrupt 是 canonical durable record；
- [x] rollback/fork/recovery 不依赖 Delivery callback；
- [x] 所有 Run transition 严格按 RunID + SessionID；
- [x] Domain 无 I/O/framework；
- [x] Bootstrap 只装配、启动和关闭；
- [x] architecture fitness tests 覆盖最终边界；
- [x] protocol golden、SQLite contract、全量 build/vet/test/race 通过；
- [x] 不存在中间 shim、双写、死类型和过期注释；
- [x] artifact 导入在 mutation 前拒绝非法 union、悬空引用、identity 冲突和循环 run tree；
- [x] artifact 导出来自 idle Session 的单事务快照，且只包含可往返的 terminal Run；
- [x] parked cancel 不留下 Interrupted transcript 或 running interrupt item；
- [x] park 原子提交 running interrupt items、Run/Interrupt/admission/ProcessSnapshot，并在 cancel linearization 内整批发布；
- [x] boot recovery 只保留可恢复的 parked boundary，外部丢失以 durable `run_lost` 收口；
- [x] ProcessSnapshot 在 cancel/delete/rollback/recovery 中与引用状态原子清理；
- [x] rollback 原子删除 dropped KindSubtask subtree，且不删除用户 fork；
- [x] delete/rollback 在任何 durable 或 workspace mutation 前 claim 完整待删 KindSubtask 子树；
- [x] restore admission 由 Application use case 持有，fork/export 读取 coherent snapshot；
- [x] durable Journal event 不因 subscriber backpressure 静默丢失；
- [x] context-unaware health probe 不会随请求数无限增殖 goroutine；
- [x] `EXECUTION_CENTERED_ARCHITECTURE.md` 与真实实现一致；
- [x] 本文档状态改为“完成”，进度看板和执行记录已封账。
