# app2 目标架构

> 状态：R0 accepted。本文描述系统“长什么样”和依赖方向，不重复业务模型与 wire 字段。

## 1. 一句话架构

app2 是一个 **独立 Go Runtime + 当前 Lyra Runtime Protocol + Wails v3/React 领域工作台**。
Runtime 的业务内核与 transport 解耦；Desktop 是协议客户端和可选进程宿主，不是第二后端。Codex
只用于校准进程监督、连接代际和资源生命周期，不定义 Lyra 的 method、event、transport 或领域词汇。

## 2. 运行拓扑

```text
┌──────────────────────── Wails Desktop process ────────────────────────┐
│                                                                       │
│  React renderer                                                       │
│    feature UI -> context use cases -> generated Lyra client            │
│                                      │ streamable HTTP                  │
│  DesktopHost                         │ JSON-RPC POST / SSE response      │
│    picker / save / chrome            │ info / live / ready              │
│    bootstrap / optional supervisor   │                                 │
└──────────────────────────────────┬────────────────────────────────────┘
                                   │ current Lyra Runtime Protocol
┌──────────────────────────────────▼────────────────────────────────────┐
│  app2 Runtime process                                                 │
│    transport -> rpc dispatcher -> application flows -> domain          │
│                                      │                                 │
│                       sqlite / agentexec / fs / git / mcp / models      │
└───────────────────────────────────────────────────────────────────────┘
```

app2 首先完整重建当前 streamable-HTTP binding：`POST /v2/rpc`、流式 POST 的 SSE response、
`Last-Event-Id` replay、idempotency headers、local token、CORS 与 info/live/ready sidecars。它们是经过
真实 Runtime/Desktop 消费和恢复验证的设计，不因 Codex 使用 stdio 就被替换。

公共 typed embedded binding 继续与 HTTP 复用同一个 operation endpoint。Desktop 是否监督本地 Runtime
属于 host lifecycle 决策，不改变协议；远程仍使用同一 HTTP(S) binding。只有新的产品反例和对照证据
才能增加或替换 transport。

## 3. 为什么保留独立 Runtime 进程

- Runtime SIGKILL、崩溃或升级不必带走窗口与 renderer；
- Desktop 可以展示重连状态，并从 SQLite 权威事实恢复同一 Session；
- standalone、未来 CLI 和远程消费者共享一个 operation pipeline；
- Desktop host 可以监督本地进程，但 generated client 仍消费 Lyra Protocol，不经过第二套 Wails RPC；
- Codex 的 stale-generation、bounded shutdown 和 restart owner 只作为 lifecycle 参考。

代价是需要 supervisor、stream lifecycle 和 generation fencing；这些机制由 Desktop host、Frontend
connection owner 与 Runtime transport 各自明确拥有，不泄漏进业务 context。

## 4. Runtime 代码语法

目标目录是边界语法，不是要求一次创建所有空 package：

```text
app2/runtime/
  go.mod
  runtime.go                 optional in-process resource API
  protocol/                  public canonical request/response/event values
  contract/                  generated TS/schema/OpenRPC/catalog artifacts
  cmd/
    lyra-runtime/            streamable-HTTP executable composition root
  internal/
    session/                 Session aggregate/value/invariants
    run/                     Run tree, Segment, outcome
    transcript/              Item and visible-history rules
    interrupt/               Approval/Question semantics
    goal/                    Goal aggregate
    plan/                    Plan resource

    sessionflow/             Session use cases and consumer-owned ports
    runflow/                 start/resume/steer/cancel/recovery transactions
    snapshotflow/            consistent Session snapshot assembly
    goalflow/                Goal lifecycle orchestration
    workspaceflow/           file/git/index use cases

    agentexec/               only Agent Framework adapter
    sqlite/                  explicit stores and transaction write-sets
    filesystem/              path/file/watch adapter
    git/                     git process adapter
    mcp/                     MCP connection/tool adapter
    modelclient/             provider/model adapter

    operation/               canonical typed registry and invocation narrow waist
    dispatch/                JSON-RPC request/error/event mapping
    httptransport/           current streamable HTTP/SSE/sidecars/auth/CORS
    runtimehost/             process-scoped owner, workers, shutdown
    telemetry/               OTel SDK/export composition（只在真实 exporter 落地时建立）
```

规则：

- `session/run/transcript/...` 是纯领域 package；
- `*flow` 只在跨对象事务、外部 port 或完整用例确实存在时建立；小能力留在其领域 package；
- interface 定义在使用它的 flow 中；`sqlite/agentexec/...` 向内实现，不向内发布 concrete type；
- 技术 adapter 可互相通过 composition 注入，但不能形成 import cycle；
- `runtimehost` 和 `cmd` 是唯一 concrete assembly；
- 不建立顶级 `domain/application/adapter/infra/delivery/bootstrap` 套娃目录；依赖方向由 imports 与测试强制。

### 4.1 Runtime 依赖图

```text
cmd/runtimehost
  ├─ httptransport -> dispatch -> operation
  ├─ sqlite/filesystem/git/mcp/modelclient/agentexec
  └─ concrete flow assembly

operation/dispatch -> flow published APIs -> domain
adapters  -> flow-owned ports     -> domain values
domain    -> standard library only
```

允许 application flow 依赖 OTel API 记录 span/metric/log；只有 `telemetry` composition 依赖 SDK/exporter。
R1 仅在 `httptransport` 入口用 OTel API 的 W3C `TraceContext` 做提取和日志关联，不引入无 exporter 的空 SDK
composition，也不传播 baggage。日志不读取 query、Authorization 或 body。

## 5. Runtime 应用内核

### 5.1 Operation dispatcher

合同注册表为每个 operation 声明：

- method；
- request/response/event type；
- query/command/subscription 与 unary/stream；
- idempotency policy；
- pagination；
- capability gate；
- error set；
- materialized fact families。

保留当前注册表的优点：generic factory 从类型参数推导 unary/stream shape、query/command/subscription、
idempotency、pagination、capability rules、errors 和 materialized fact families；同一 registration 同时拥有
typed invocation 与机器合同。注册表生成 wire 产物与 dispatcher glue，但 handler 是具名 use case，
不使用 service locator。dotted method 与现有 89 项默认原样保留。

### 5.2 并发仲裁

数据库 revision/unique/CAS 和领域不变量是 correctness owner。app2 不在 R1 复制 Codex 的通用 request
serialization scheduler，也不把 serialization scope 塞进现有 operation metadata。

只有具体 flow 已证明同一 identity 的外部准备步骤不能安全并发时，才在该 flow 内建立最小 keyed admission；key
必须使用 Session/Run/MCP server 等领域 identity，任务必须可取消、可 join、容量可测。若容量耗尽需要新增
client-visible outcome，必须先按 ADR-A2-022 扩展 problem manifest，不能挪用现有 numeric code 或返回模糊
`internal_error`。数据库仍是跨进程与 crash 后的最终仲裁。

### 5.3 背压

- 保留当前 request body、replay window、subscription/watch、SSE write deadline 和 idempotency retention 上限；
- streamable POST 自然把每个调用的 ack/event 绑在同一 response，不引入 connection-routing protocol；
- workspace event queue 满载时折叠为 `resync`，不无限缓存或静默丢失事实；
- run replay 只保留 authoritative/replayable frame，ephemeral delta 不进入 replay buffer；
- Desktop 继续使用当前 query/mutation/stream owner；只有测量证明 starvation/queue pressure 后，才引入
  Codex 式 priority/coalescing scheduler，不能先复制其复杂度；
- HTTP client disconnect 只 detach subscription，不取消已被 Runtime 接受的 Run。

### 5.4 事务与 committed facts

每个 mutation flow 形成一个显式 write-set。推荐骨架：

```text
load + validate -> prepare fallible external capability -> database transaction
  -> commit marker -> post-commit apply -> publish facts
```

事务 closure 不执行网络、模型、进程或无法有界的文件 I/O。必须在 commit 之后发生的 effect 由 durable
intent/attempt 标识；apply 失败进入可恢复状态，而不是回滚已经发生的事实。

### 5.5 进程生命周期

`runtimehost.Runtime` 是进程资源对象：

- `Open(Config)` 完成数据库、adapter、worker 和 listener 的合法装配；
- `Serve(ctx, transport)` 持有连接代际；
- `Close(ctx)` 先停止 admission，再取消/排空连接和 worker，flush telemetry/store，最后释放文件资源；
- 每个 goroutine 都在 task group 中可 join；
- shutdown 有总 deadline，超时返回包含未退出 owner 的诊断；
- repeated Close 返回同一 settlement，不启动第二轮 teardown。

## 6. 持久化架构

SQLite 是唯一 durable truth：

- relational current state：sessions、runs、segments、interrupts、goals、plans、config resources；
- append-oriented semantic facts：items、tool lifecycle、usage、command markers；
- opaque framework checkpoints 单独存储并以 exact Run/Segment/occurrence 关联；
- transaction outbox 只在需要跨 transaction 发布/恢复时使用，不为每个 event 建仪式；
- derived indexes/read models 可重建，不参与真相仲裁。

app2 从 schema epoch 1 开始。开发期间修改 epoch 即重建 app2 data home；切换时不读取旧 epoch 77。
SQLite 设置由一个 owner 管理：WAL、foreign keys、busy timeout、synchronous policy、connection limit 和
checkpoint policy 均有测试。多 Runtime 打开同一 store 时，数据库 uniqueness/CAS 决定 winner；文件锁只
用于减少重复活跃 owner，不定义业务存活。

不复制 Codex 的 rollout JSONL 作为第二 store。app2 Artifact 是从 SQLite canonical facts 生成的 portable
projection；诊断日志也不是恢复输入。

## 7. Desktop Go Host

```text
app2/desktop/
  go.mod
  main.go                      Wails v3 composition root
  host/
    native.go                  narrow native capabilities
    runtime_supervisor.go      child process lifecycle/restart
    window_chrome_*.go
    picker.go
    image_saver.go
  frontend/
```

Wails service 只拥有 packaged-app capability：window chrome、选择工作目录、原生保存图片，以及本地
Runtime 的 bootstrap/supervision state。Renderer 使用生成的 Lyra Client 直接调用 Runtime Protocol；不再
为同一 operation 建 Wails `RuntimeBridge` 或 89 个 forwarding methods。导出的 Wails method 集由测试精确锁定。

若 Desktop 采用内建 supervisor，它拥有 child executable、ready inspection、restart policy 和 termination：

- 每次 spawn 创建独立私有临时根；Runtime 先持有 `127.0.0.1:0` listener，再原子发布 ADR-A2-023 定义的
  one-shot descriptor，Desktop 不猜端口、不解析日志、不在命令行传 token；
- spawn 后必须以 `/v2/info`、`/v2/health/live`、`/v2/health/ready` 检查同一个 `instanceId`，再发布连接；
- 非预期退出清空旧 connection 的 pending requests/listeners，发布 disconnected/recovering；
- backoff 指数增长并带 jitter，有上限；用户退出、版本/合同错误、明确 auth/config 拒绝不自动重试；
- successor 先成为当前 generation，旧 callback/cleanup 才退休；
- graceful app quit 先停止 renderer request，再关闭 Runtime，并等待子进程；最终才退出 Wails。

remote attach 由用户配置的 endpoint 与 secret-store token 建立，不读取本地 descriptor，也不让 Desktop 终止
不属于自己的 Runtime process。

## 8. Frontend 代码语法

```text
app2/desktop/frontend/src/
  app/                         providers, router, composition, startup
  shell/                       window layout, Work Index geometry, commands
  sessions/                    navigation read model and Session use cases
  agent/                       fold, AgentSessionView, narrative, HITL
  composer/                    Draft, Attachment, SendIntent, IME
  workspace/                   Context Dock, files, diff, terminal, timeline
  settings/                    providers/models/MCP/hooks/schedules/approval/usage
  runtime/                     generated client adapter, connection/capabilities
  ui/                          primitives -> atoms -> agent/workspace components
  lib/                         proven cross-context pure/technical utilities only
  styles/                      tokens and unavoidable global chrome
```

一个 context 可按真实复杂度拥有 `domain/application/adapters/presentation/ui/public`，但不强制对称。
例如 Agent 有状态机和外部 stream，需要完整边界；静态 About panel 不需要。

### 8.1 静态组合，不建万能插件平台

所有生产功能与同一 bundle 一起发布，composition root 直接静态装配：

- route table；
- Work Index section；
- Context Dock destination；
- settings section；
- command/shortcut；
- tool/content presentation。

只有真实多生产者且 consumer 需要按 identity 查找时才建立具体 registry。registry 返回稳定 read model，
但不拥有业务生命周期、异步 task、全局 Host generation 或 installation transaction。

主题是数据（palette/tokens/preferences），不是插件。内置主题可静态 lazy import；新增主题不需要一个
通用 plugin service graph。

### 8.2 UI 信息架构

```text
Work Index        Agent Narrative                 Context Dock
找工作/切工作      共同时间线与阻塞动作              做当前工作的材料

global/session    session/run                      workspace/session/run
```

- Work Index：New Session、cwd 分组、Session rows/status/attention、global utility；
- Agent Narrative：root transcript、delegated disclosure、commentary/reasoning/tools、final answer、
  approval/question、Goal/Plan/composer；
- Context Dock：overview/files/diff/review/search/tool detail/timeline/terminal/run summary/skills/recipes/memory/knowledge/index；
- settings 是 global surface，不与 session-scoped dock state 混存；
- blocking HITL 必须在 Narrative 可完成，Work Index 只投影 attention。

### 8.3 状态所有权

| Fact | Owner | 禁止副本 |
| --- | --- | --- |
| active route/session/destination | URL/router | Zustand `active*` mirror |
| server resources | Query cache owned by context | global data store |
| active Session run/item/HITL | one `AgentSessionView` fold owner | per-component stream state |
| Work Index groups/attention | sessions application projection | sidebar joins |
| Draft/attachments/IME | composer context | agent store |
| per-session dock view/tabs/file target | bounded workspace presentation keyed by Session | one global active file/activity view |
| theme/sidebar/dock widths | shell preference store | component localStorage writes |
| transient hover/filter/disclosure | local component | persisted global patch |

Runtime event 是 Query invalidation 或 Agent fold 输入，不直接修改多个 stores。mutation owner 管理 single-flight、
optimistic compensation 和迟到 settlement；裸 gateway 不能绕过它。

### 8.4 渲染性能与正确性

- streaming fold 在协议入口做一次解析/归一化，render 不按 token 重复 parse；
- `runsById/itemsById` normalized，tree/timeline/narrative 是 memoized selector；
- 订阅粒度以可见 read model 为单位，避免整份 Session 每个 delta 重渲染；
- 长历史和大列表使用 virtualization，但保持语义 DOM、键盘焦点和 screen reader 顺序；
- Shiki/KaTeX/Mermaid/image lightbox 按内容 lazy load；
- follow-to-bottom 有唯一 raw lock；用户 wheel-up 后任何 delta/resize/materialization 都不能抢回 scrollTop；
- remote text/HTML 一律视为不可信，Markdown allowlist 与 link/image policy 在单一 renderer boundary 实现。

## 9. 可观测性

- request/command 从 Desktop 到 Runtime 传播 W3C trace context；
- span 使用领域动作名和稳定低基数字段：session/run/method/transport/generation；
- error/log 不包含 prompt、secret、完整参数、文件内容或未脱敏路径；
- metrics 覆盖 queue depth、overload、request latency、run outcome、recovery、reconnect、SQLite wait、
  stream lag 与 renderer fold delay；
- telemetry 失败不改变业务结果，但 exporter lifetime 必须被结构化关闭。

## 10. 安全边界

- renderer 是不可信客户端；Runtime 对所有 payload strict decode 并执行业务授权；
- NativeHost 只接受最小数据，不接受任意文件写路径或 shell command；
- 本地 loopback HTTP 只绑定 loopback，并由 path-owned local token、严格 CORS 与 exact Runtime identity 保护；
  远程 HTTP(S) 必须 TLS、auth、Origin policy 和明确 server identity；
- workspace path 必须在 operation 边界解析并验证 scope；不靠 `filepath.Clean` 单独阻止 traversal；
- MCP/provider secret 只由 secret store owner 读写；read model 永不返回原文；
- tool execution、hooks 和 user shell 的 sandbox/approval policy 由 Runtime 决定，Desktop 不推断。

## 11. Architecture fitness tests

必须机器证明：

- domain packages 只依赖允许的标准库/领域 packages；
- adapter 不被 domain import；Agent Framework import 只存在于 `agentexec`；
- contexts 只能通过 public surface 互相依赖且无环；
- React UI 不 import raw protocol transport/Wails binding；
- 生产树不存在 generic plugin host、service locator、generic repository/state registry；
- contract generator 重跑无 diff，89 个迁移 operation 均有 handler 与 Desktop consumer/明确 headless reason；
- Wails exported surface 精确；
- goroutine/listener/timer/process/browser resource 在 test teardown 后为零；
- bundle budget、accessibility、IME、keyboard、RTL、narrow window、light/dark 与 recovery 场景通过。

## 12. 独立交付拓扑

迁移期 `app` 与 `app2` 使用完全独立的 binary、module、data home、port/socket 和 artifact。不得让 app2
读取或写入旧 `~/.lyra` 数据。旧实现只运行在隔离临时 HOME 的行为对照测试中。

当 Runtime/Desktop 能力账本全部 `verified`：

1. 运行完整语义对照、恢复、package 和视觉矩阵；
2. 产出独立 app2 Runtime/Desktop binary 与 bundle；
3. 保留旧 Runtime/Desktop、CLI 与产品入口，等待后续明确授权的切换目标。

app2 不读取旧数据、不依赖旧实现，也不建立同步或 fallback；旧 app 的存在只是本轮范围边界。
