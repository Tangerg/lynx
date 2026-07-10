# Runtime Rebalance — 组合根减重与领域重心校准

> 日期：2026-07-09。状态：执行中。目标不是重写 `app/runtime`，而是在现有 Clean Arch + DDD 骨架上做小批次结构精修，让代码阅读重心更符合“协议薄、应用编排清楚、领域规则归位”。

## 目标

1. **降低头部清单感**：`cmd/lyra` 与 `internal/runtime` 负责启动和装配，允许重，但不应把大段配置字面量、端口投影和能力接线混在一个函数里。
2. **允许破坏式收敛旧入口**：协议 / store schema 仍谨慎，但内部 exported API 可按真实边界重排；不为旧调用点保留兼容 shim。
3. **继续把规则往正确层收**：领域不变量进 `domain/*`；跨 bounded context 的写集进 `kernel/*`；delivery 只做 wire 翻译和调用 use case。
4. **不硬造“厚 domain”**：CRUD/registry 型上下文保持轻；只有真实规则和不变量才进入充血模型。

## 非目标

- 不新建独立 `application/` 环；`kernel` 已是应用层 / 微内核。
- 不把 `Store` / `Registry` 改名成 `Repository`。
- 不引入 Domain Event bus、DI container、CQRS/Saga。
- 不为视觉平衡拆碎内聚良好的 domain 包。

## 判断标准

- 一个改动如果只让目录树看起来“更 DDD”，但没有降低认知负担或收回规则，不做。
- 一个规则如果需要同时读 delivery/runtime/kernel 才能理解，优先寻找能否下沉到 domain 或 kernel use case。
- 一个装配函数如果主要是“把已有依赖摆到 Config 里”，优先抽成命名 helper，而不是塞进 CLI 入口。

## 批次计划

- [x] **Batch 1：cmd/lyra runtime 配置构建下沉**
  - 把启动入口里的 `lyraruntime.Config` 大字面量抽成专门构建函数。
  - server 外壳保留：加载配置、建默认 client、建 stores、seed、创建 hook resolver、调用 runtime。
  - 预期收益：`runtime_bootstrap.go` 从“启动 + 装配细节”变成“启动流程”。

- [x] **Batch 2：internal/runtime.New 命名化装配步骤**
  - 把 history/conversation、utility/embedding、tool/MCP/approval 等局部装配变成命名步骤。
  - 保留 `Runtime` 作为 transport-facing facade，不拆公开结构。
  - 预期收益：组合根仍集中，但每段能力边界更容易复查。

- [x] **Batch 3：寻找真实领域下沉点**
  - 只在发现具体规则外泄时做；当前候选是 `transcript` 的 wire blob 味道和 session/run 边界语义。
  - 需要单独评估 blast radius；本批不强行执行。

- [x] **Batch 4：持久化后端装配移出 CLI**
  - 把 `cmd/lyra/stores.go` 搬到 `internal/adapter/persistence`，server 外壳只调用 `persistence.Open()`。
  - 预期收益：`cmd/lyra` 不再直接 import SQLite/file storage/各 domain store 类型，外壳更薄。

- [x] **Batch 5：三模块职责归位**
  - `cmd/lyra`：把 OTel/slog 进程 bootstrap 搬到 `internal/adapter/observability`。
  - `internal/runtime`：把 catalog-backed pricing port 搬到 `internal/adapter/pricing`。
  - `internal/kernel`：先把 token/cost accounting 从主包隔离；Batch 9 进一步确认其领域归属并迁入 `internal/domain/accounting`。
  - 预期收益：三个头部模块各少一块非核心职责，仍保留原有调用链和行为。

- [x] **Batch 6：破坏式删除旧入口 + 启动适配下沉**
  - 删除 `kernel` / `kernel/turn` 的 accounting re-export alias；调用方直接依赖 accounting 的真实 owner（Batch 9 后为 `internal/domain/accounting`）。
  - 删除旧的 `transcript.BoundaryAt` 函数入口；rollback/fork 统一走 `transcript.Timeline.BoundaryAt`。
  - 把 config/env 投影、默认 client 构建、provider/utility seed、hook resolver 构建从 `cmd/lyra` 搬到 `internal/runtime/startup`。
  - 预期收益：server 外壳只保留启动顺序；kernel/turn 不再假装拥有 accounting 类型；领域对象方法成为唯一入口。

- [x] **Batch 7：cmd/lyra 收敛为 server-only 壳**
  - 删除旧命令 root 与 agents/chat/repl/memory/session/hooks/version/a2a serve 等子命令。
  - `main.go` 不再接受任何参数，只启动 HTTP runtime server。
  - 删除额外 A2A serve listener 配置：对外服务端只暴露 Lyra Runtime Protocol；客户端/TUI/CLI 另行实现。
  - 预期收益：C/S 边界清楚，服务端只提供接口能力，客户端交互能力不再混进 runtime 二进制。

- [x] **Batch 8：入口依赖卫生**
  - 保留 inprocess transport 与 `Transport` interface，作为未来独立 CLI/TUI 的同进程接入点。
  - 保留 request-side `NewCall`/`StringID` convenience builders，供 inprocess/client-side 测试复用。
  - 删除 `Runtime.A2AAgent()` server adapter；保留远端 A2A agents 作为 toolset 能力。
  - 删除冗余 `turn.Event` marker 方法；`stamp` 已经封闭事件 sum type。

- [x] **Batch 9：领域重心校准**
  - 把 token/cost accounting 从 `kernel` 迁入 `domain`：它是跨 kernel、delivery、pricing adapter 共享的纯值对象，不属于 turn 编排机制。
  - 把 MCP 工具 disabled / auto-approve 的推导与判断收进 `domain/mcpserver.ToolPolicy`；runtime 只负责加载 registry 与原子替换策略快照，kernel/toolset 只消费判断函数。
  - 把 utility / embedding 共用的 `(provider, model)` 组合收成 model-role 领域值对象，让“清空 model 同时清空 provider”的不变量只有一个实现。
  - 不移动 run admission、turn event loop、HITL park/resume、跨 store 事务；这些是并发与用例编排机制，继续归 `kernel` / `runtime`。
  - 验收：协议 wire、配置文件、SQLite schema 不变；arch test、build、vet、全量 test、lint、race 全绿。

- [x] **Batch 10：深度卫生与生命周期所有权**
  - 删除 `kernel.SkillInfo`、`kernel.InterruptKey` 和 `lifecycle.RollbackBoundary` 等领域类型影子；消费方直接依赖真实领域 owner。
  - 把 session patch 的标题规范化、rollback dropped-run id 派生、skills 空 cwd 语义和 MCP ToolPolicy 值语义收回对应领域对象。
  - MCP 启动只读取一次 registry，再从同一快照派生连接配置和工具策略。
  - session 删除的 durable write-set 在同一事务中提交；提交成功后再取消 parked turn、释放进程态 gate。
  - kernel task group 统一承载组件后台任务：Runtime 的 codebase reindex / runsegment 终态维护，以及 Server 的 run pump / MCP connection action 都可在各自 `Close` 时取消并等待。
  - 完整关闭顺序固定为 Server delivery tasks → Dispatcher 全部 live/parked turns → Runtime maintenance → engine capabilities → persistence bundle；scheduler、inprocess stream pump、shell process、OAuth callback server 也都有明确 join。
  - 半构造失败路径保留原始错误并合并 cleanup 错误；OTel shutdown 错误回传，HTTP 与 telemetry 使用同一版本解析规则。
  - 明确保留：rollback 子树清理的 best-effort、schedule 失败后推进 cron、after-live 持久化错误只进入 OTel；这些是既定可恢复语义，不按 `_ =` 机械改写。
  - 验收：协议 wire、配置格式、SQLite schema 不变；arch test、build、vet、全量 test、lint、race 全绿。

- [x] **Batch 11：并发正确性与共享状态专项**
  - 为每个 goroutine 标出 owner、取消信号和 join 点；重点复查 Start/Attach 与 Close、WaitGroup.Add 与 Wait、channel send 与 close 的线性化顺序。
  - 逐类区分 request context、run/turn context 与 process/component context；后台任务不得意外继承短请求 deadline，入站调用也不得逃逸调用方取消，组件 Close 必须取消并等待其拥有的工作。
  - 审计所有跨 goroutine map/slice/pointer：锁内读取后不得把可变 backing storage 泄漏到锁外；配置、事件和 registry 查询优先返回不可变快照。
  - 审计 TOCTOU：run admission、turn register/cancel、MCP reconnect/configure/close、workspace subscription、inprocess Send/Close 必须在同一同步边界内决定状态转换。
  - 审计 durable commit 与 process-local cleanup 的先后关系，确保事务失败不提前取消 live state，提交成功后也不留下可恢复记录与内存状态互相矛盾。
  - 只修有具体失败时序的缺陷；不以换锁、改 RWMutex、加 channel 等形式做无证据“并发优化”。
  - 非目标：不改变 JSON-RPC wire、配置格式、SQLite schema，不重写既有 best-effort 恢复语义。
  - 验收：为每个修复补时序回归测试；高重复并发测试、arch test、build、vet、全量 test、lint、全量 race 全绿。

- [x] **Batch 12：关闭闸门、状态机与深层别名专项**
  - 复查所有 process/component Close：关闭开始后必须拒绝新工作，已经进入的同步调用要么被 owner context 取消并 join，要么在资源释放前自然完成。
  - 复查 channel 的唯一关闭方、订阅注销与 producer 退出顺序，重点覆盖 request 已取消、source 尚未退出、consumer 提前离开和 Close 并发调用。
  - 复查 durable write 后的 process-local 对账与错误补偿；禁止 request deadline 在 durable commit 之后留下只更新一半的 live/cache/policy 状态。
  - 继续扫描浅拷贝下的嵌套 map/slice/pointer、锁外返回的缓存对象、可复制 token/cancel handle，以及 race detector 不会报告的计数破坏。
  - 复查 timer、子进程、首次懒加载和半构造失败路径，确保每个启动动作都有终态、取消信号和 join 点。
  - 只修能给出具体交错顺序或不变量破坏的缺陷；不改变 JSON-RPC wire、配置格式、SQLite schema 和既有恢复语义。
  - 验收：新增确定性时序测试并高重复运行；arch test、build、vet、全量 test、lint、全量 race 与 diff check 全绿。

## 执行记录

### 2026-07-09

- [x] 建立本跟踪文档。
- [x] Batch 1：当时新增 `cmd/lyra/runtime_config.go`，把旧启动入口中的 `lyraruntime.Config` 大字面量抽成 `buildRuntimeConfig`；后续 Batch 6 将它继续下沉到 `internal/runtime/startup`。`runtime_bootstrap.go` 只保留 load config → build client → open persistence → seed registries → build hooks → `runtime.New` 的启动顺序。
- [x] Batch 2：新增 `internal/runtime/engine_wiring.go` 与 `facade_wiring.go`，把 engine config/message history/tool env 注入、Runtime facade 端口投影从 `New` 的主流程中拿出。`New` 现在只保留装配流程和错误处理。
- [x] Batch 3：新增 `transcript.Timeline` 值对象，把 rollback/fork 边界算法提升为 `Timeline.BoundaryAt`；删除旧的 `transcript.BoundaryAt` 函数入口，`kernel/lifecycle` 直接调用领域对象方法。
- [x] Batch 4：新增 `internal/adapter/persistence.Bundle` / `Open`，删除 `cmd/lyra/stores.go`。当时的启动与 hooks 调用点改为调用 persistence adapter，`buildRuntimeConfig` 接收 persistence bundle。
- [x] Batch 5：新增 `internal/adapter/observability.Setup`、`internal/adapter/pricing.Catalog`，并建立独立 accounting 包边界（Batch 9 后归于 `internal/domain/accounting`）。`serve` 调 observability adapter，runtime config 调 pricing adapter，不再保留 accounting re-export alias。
- [x] Batch 6：新增 `internal/runtime/startup`，承接 config/env → runtime/domain 的投影、默认 client 构建、provider/utility seed、hook resolver 构建。删除 `cmd/lyra/config_projection.go` / `runtime_config.go` / `seeds.go`，`bootstrapRuntime` 只保留启动剧本。同步删除 `transcript.BoundaryAt` 函数和 kernel/turn accounting alias。
- [x] Batch 7：删除 `cmd/lyra` 下所有旧子命令文件，`cmd/lyra` 只保留 `main.go` / `serve.go` / `runtime_bootstrap.go`；去掉额外 A2A serve listener 配置，runtime server 只提供 HTTP 接口能力。
- [x] Batch 8：保留 `delivery/transport/inprocess`、transport `Transport` interface、request-side builders；删除 `Runtime.A2AAgent()` server adapter，并补回 inprocess 测试覆盖。
- [x] 局部验证：`go test ./cmd/lyra ./internal/runtime/startup ./internal/runtime ./internal/kernel/... ./internal/domain/transcript ./internal/delivery/server ./internal/adapter/pricing` 通过。
- [x] 局部验证：`go test ./internal/domain/transcript ./internal/kernel/lifecycle ./internal/runtime/... ./cmd/lyra` 通过。
- [x] 全量验证：`go build ./... && go vet ./... && go test ./... && golangci-lint run` 通过；`go test -race ./...` 通过。
- [x] Batch 6 全量验证：`go test ./internal/arch && go build ./... && go vet ./... && go test ./... && golangci-lint run` 通过；`go test -race ./...` 通过。

### 2026-07-10

- [x] Batch 9：职责审计确认生产代码规模并非机械失衡；下沉 accounting 值对象与预算规则、MCP ToolPolicy、model role 不变量，并保留并发 / I/O 编排在原层。完成后生产代码约为 domain 4.38k、kernel 5.54k、runtime 2.50k 行。
- [x] Batch 9 全量验证：`go test ./internal/arch && go build ./... && go vet ./... && go test ./... && golangci-lint run ./... && go test -race ./...` 通过；`git diff --check` 通过。
- [x] Batch 10：删除 kernel/lifecycle 的领域类型影子；下沉 session/skills/transcript/MCP 规则；MCP 启动改为单 registry 快照；session durable 删除改为单事务。
- [x] Batch 10：新增 kernel `taskgroup.Group`，统一 Runtime/Server 后台任务生命周期；Dispatcher.Close 覆盖 running 与 parked turn；组合根关闭 Runtime 与 SQLite bundle，并 join scheduler。
- [x] Batch 10：补齐 inprocess stream pump、shell process、OAuth loopback server 的 join；构造失败 cleanup 与 OTel shutdown 错误不再吞掉；统一 HTTP/telemetry build version。
- [x] Batch 10 局部验证：领域、lifecycle、taskgroup、turn、runsegment、runtime、server、persistence、MCP、exec、inprocess、observability、cmd 与 arch 测试通过；相关并发包 race 通过。
- [x] Batch 10 全量验证：`go test ./internal/arch && go build ./... && go vet ./... && go test ./... && golangci-lint run ./... && go test -race ./...` 全部通过；`git diff --check` 通过。
- [x] Batch 11：修复 MCP configure/reconnect/authorize/remove/Close 的线性化边界；Close 不再持热锁做网络 I/O，关闭后写命令明确失败，配置切片/map 与 tool sink 均按快照发布。
- [x] Batch 11：HTTP server 在构造时取得完整生命周期，Shutdown-before-Start 不再漏启动；进程关停会在 graceful 超时后 force-close 并 join 监听 goroutine。健康探针拥有真正的共享硬预算，run terminal fan-out 也从逐订阅者超时改为单总预算。
- [x] Batch 11：taskgroup 增加 caller-linked scope；inprocess Transport 把普通调用和 streaming pump 都纳入 Close 的 cancel/join，stream context 从 Send 正确移交给 pump。LSP 启动、A2A cleanup、shell 进程、OAuth browser launcher 均补齐不可复活与 join 语义。
- [x] Batch 11：MCP registry → live connections → ToolPolicy 的多步写在 Runtime 用例边界串行化；提交前尊重 request context，提交后切换到有 30s 上限且仍受 Runtime.Close 管理的 owner context，避免请求断开留下 durable/live 半提交状态。
- [x] Batch 11：read tracker 与 write/edit 共用按路径锁，关闭同轮工具调用的 read→stamp TOCTOU；working-tree/session admission 的值副本共享幂等 release，避免重复释放吃掉其他真实占用。
- [x] Batch 11：构造输入、缓存查询和 context metadata 的可变 map/slice 全部在关键边界复制；覆盖 HTTP capabilities/CORS/probes、tool resolver、engine tool catalog、hooks、LSP diagnostics/specs、provider env、MCP config、request `_meta` 等泄漏点。
- [x] Batch 11：统一 toolset 半构造失败 cleanup，并给 terminal process snapshot 删除加独立短 deadline；Dispatcher.Close 并行发出 turn cancel，避免 parked turn cleanup 按数量串行放大关停时间。
- [x] Batch 11 验收：新增时序回归测试经 `-count=50` 高重复通过；`go test ./internal/arch && go build ./... && go vet ./... && go test ./... && golangci-lint run ./... && go test -race ./...` 全绿；`git diff --check` 通过。
- [x] Batch 12：Server 的 run pump 持久化改用 component owner context：继续脱离单次 `runs.cancel`，但不再逃逸 `Server.Close`；workspace subscription 也纳入 Server task group，Close 会取消、关闭 channel 并 join watcher。run admission 失败使用独立有界 cleanup context，避免请求已取消时遗留刚创建的 turn。
- [x] Batch 12：`runHandle` 串行化 interrupt durable commit + live publication 与 `runs.cancel`。取消先赢时不再发布不可恢复 interrupt；提交先赢时取消等待提交完成再删除，关闭 delete-before-put 复活窗口。取消清理使用 component owner + 有界 deadline，不因客户端断线留下 ghost interrupt。
- [x] Batch 12：interrupt persistence 从吞错改为显式失败；ProcessID 缺失、snapshot 查询失败或 Put 失败都不会向客户端发布假的 `run.finished{interrupt}`，并会终止已经 parked 的底层 turn，避免 dispatcher/process 泄漏。
- [x] Batch 12：跨重启 resume 的 `Consume` 增加提交语义：rehydrate 在 continuation 接受 decision 前失败会补偿写回 pending interrupt；decision 已提交且 turn 已 terminalize 的错误由 `turn.ErrRehydrateCommitted` 标记，不制造 ghost record。parked cancel 与 resume 共用 session admission，补偿 Put 不再和 cancel Delete 穿越。
- [x] Batch 12：Resolver 提升为 runtime 生命周期共享的 path locker，canonical tool catalog、并发 root turn 和 subtask 解析不再各持一把锁；锁等待改为 context-aware semaphore，取消中的 tool call 不会卡在同路径调用后面，ref-count 仍会归零。
- [x] Batch 12：文件身份从 lexical clean 提升为解析现有 symlink、允许缺失末端的 physical path；read stamp、edit check、跨 turn path lock 使用同一真实身份，`.git` 写保护拒绝目录 symlink、dangling symlink 和 symlink cycle，关闭别名竞态与保护绕过。
- [x] Batch 12 验收：新增 interrupt/cancel 线性化、Close/持久化 join、workspace owner、request-cancel cleanup、rehydrate 补偿、parked abort、path-lock cancel/ref leak、symlink alias/cycle 等确定性测试；高风险包 `go test -race -count=25` 通过；`go build ./...`、`go vet ./...`、`go test ./...`、`golangci-lint run ./...`、`go test -race ./...` 与 `git diff --check` 全绿。
