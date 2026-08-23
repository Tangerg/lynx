# app2 合同策略

> 状态：R0 accepted design。app2 是实现层面的绿地重写，但当前 Lyra Runtime Protocol 是经过验证的
> 领域合同基线，不因为内部重写而被替换。Codex 只提供生命周期、并发、恢复和工程化机制的参考。

## 1. 合同身份与兼容边界

app2 区分“值得继承的产品合同”和“可以彻底重写的实现”：“允许 breaking change”不是“必须制造 breaking
change”。第一阶段的合同身份如下：

| 合同 | app2 初始 identity | 决策 |
| --- | --- | --- |
| Runtime protocol | `2026-08-23` | Lyra 合同；client/server 必须精确相等，A2-028 修复 Session 模型身份歧义 |
| Session artifact | `app2/2` | 新合同；不承诺读取旧 v22 或 app2/1 |
| SQLite schema | epoch `13` | 新存储；A2-031 的 normalized Schedule 与 durable occurrence；开发期 exact epoch，不承诺迁移旧 epoch |
| Agent snapshot | adapter 显式声明 | 仅由对应 adapter 解码，不泄漏到领域层 |

协议字段或语义只有在存在明确产品缺陷、不可消除的歧义或更强反例时才能改变；改变必须有 ADR、合同版本提升、
生成产物和全部消费者的同批证据。Go package、数据库、前端状态管理和进程编排不属于兼容面，可以彻底重写。

## 2. 唯一真相源与生成

Runtime `protocol` package 和 typed operation catalog 共同构成 canonical source：

- public protocol types 定义 wire shape、约束和闭合 union；
- 每个 operation 只注册一次，同时声明 method、query/command/subscription、unary/stream、幂等、分页、
  capability、错误集和 materialized facts；
- 同一注册直接驱动 typed invocation，type erasure 只允许存在于 operation 的私有窄腰；
- 生成 TypeScript wire types、client、strict validators、JSON Schema、OpenRPC、method manifest、examples
  和 Desktop consumer coverage facts。

生成物禁止手工编辑。CI 生成到临时目录并逐字节比较 worktree。新增 operation、event 或 problem 必须同批拥有
handler、测试和 consumer；若确实是 headless 能力，必须在 ledger 中解释。

## 3. Method 合同

app2 第一阶段精确保留当前 89 个 dotted methods，例如：

```text
runtime.discover
sessions.snapshot
runs.start
runs.subscribe
runtime.subscribe
workspace.files.read
goals.update
```

复数领域名和 dotted method 是 Lyra 的现有语言，不改写为 Codex 的 slash method。每个 method 恰好拥有：

- params/result/event 的 concrete Go type；
- `query | command | subscription` 与 `unary | stream` 分类；
- idempotency class 和 namespace；
- bounded/cursor pagination class；
- required capability；
- closed problem set；
- side effects、cancellation、replay 与 materialization 语义。

不能为同一业务动作添加 alias、generic escape hatch 或第二条直达 handler 的路径。

## 4. 协商与发现

协议不引入 Codex 风格的 connection `initialize/initialized` 握手。协商由已有两部分完成：

1. request 的 `_meta` 可携带 `protocolVersion`、client identity 和 client capabilities；一旦提供 version，必须
   与当前 exact version 相等；
2. `runtime.discover` 返回 exact protocol version、`ServerInfo`（instance/name/version/defaultWorkspace/home）、
   capabilities、event/topic catalog 和公开 limits。

version mismatch 在进入 application use case 前失败；省略保持当前合同语义。`runtime.discover` 是普通 typed operation，因此 HTTP 与
embedded binding 的结果必须一致。重连形成新的 transport generation，但不创造第二套会话协议。

## 5. JSON-RPC envelope

Runtime 使用完整 JSON-RPC 2.0 envelope：

```json
{"jsonrpc":"2.0","id":"01J...","method":"runtime.discover","params":{"_meta":{"protocolVersion":"2026-08-23"}}}
{"jsonrpc":"2.0","id":"01J...","result":{}}
{"jsonrpc":"2.0","method":"notifications.run.event","params":{}}
```

约束：

- call 的 request id 是不透明 JSON string；notification 省略 `id`；
- response 恰好包含 `result` 或 `error`；
- 拒绝重复 JSON member、未知 envelope field、错误 JSON-RPC version 和非 string id；
- params/result/event 经生成 validator 和 Go admission 双重验证；
- sum type 使用单一 discriminant，collection、字符串、path 和 URL 有显式上限；
- transport close 终止该 connection 的 pending request；durable operation outcome 仍按幂等协议恢复；
- trace、client、protocol 元数据由现有 RequestMeta/header 映射承载，不混成业务字段。

## 6. Streamable HTTP binding

当前 transport 是 app2 的默认首选 binding，而不是待删除的历史包袱：

- `POST /v2/rpc` 接受 JSON-RPC request；
- unary operation 返回 JSON response；
- stream operation 返回 SSE：首帧是 request response/ack，后续帧是 protocol notification；
- `GET /v2/info`、`GET /v2/health/live`、`GET /v2/health/ready` 提供 typed operational sidecars；
- local Desktop 使用 loopback HTTP、短期 bearer token 和严格 CORS/origin policy；
- remote mode 使用相同 HTTP(S) contract，并按部署边界加强 TLS、auth 和 origin policy；
- sidecar、HTTP 和 embedded binding 都调用同一个 operation endpoint，不能各自实现 validation 或 dispatch。

`Content-Type`、body size、HTTP header/read/idle limits、SSE per-write deadline，以及 protocol 已发布的
subscriber/replay/watch bounds 都必须显式强制。R1 不增加通用 ingress queue 或新的 overload 合同。应用错误始终是
JSON-RPC error；HTTP status 只表达 HTTP admission/transport 是否成立。

未来增加其他 binding 必须先证明真实部署需求，并复用同一个 operation pipeline。不会仅因为 Codex 使用 stdio 就
把 stdio 设成 Lyra 默认协议。

## 7. Run stream 语义

保留当前闭合的 7 个 `RunEvent` variants 及其 authority/replay 分类。每个 frame 具备可验证的
`instanceId/sessionId/rootRunId/runId/segmentId/sequence/event` 上下文。

- committed/replayable frame 才获得 SSE `id`；
- `Last-Event-Id` 只恢复 manifest 声明的 `runtimeInstanceRootSegment` scope 内可回放事实；Runtime restart 或
  新 root Segment 必须 cold recover；
- gap、retention eviction 或 identity 变化通过现有 resync/closure 语义恢复；
- client disconnect 只 detach subscriber，不隐式 cancel durable Run；
- cancel 必须是显式 command；
- 慢 consumer 受到有界队列和写 deadline 保护，不能阻塞其他连接或 Runtime shutdown；
- terminal state 必须能从 durable read 重建，不能只存在于瞬时 stream。

若后续引入优先级调度，必须先有测量数据证明当前 bounds 不足；Codex 的 priority/backpressure 做法是候选机制，
不是复制目标。

## 8. Runtime resource events

保留当前 16 个 runtime event topics 的功能：

```text
files.changed, skills.changed, mcp.changed, schedules.changed,
sessions.changed, runs.changed, plan.changed, goals.changed,
interrupts.changed, knowledge.changed, hooks.changed, models.changed,
approvals.changed, agentMemory.changed, codebase.changed, resync
```

resource event 是 invalidation/fact hint，不是第二份 authoritative state。consumer 看到 sequence gap、buffer
eviction、新 instance 或 `resync` 时，按 topic 重新调用 typed query。事件 payload 不携带未界定的任意 JSON 快照。

`runtime.subscribe` 的 ack 之前，Runtime 已注册 subscriber 并使该请求的 external watchers ready；流的首批 frame 包含覆盖
exact requested topics/watch IDs 的 `resync`，要求 consumer 冷读。sequence 对每个 subscription 从 1 单调递增；非连续、重连或
新 Runtime generation 均按同一 resync 规则收敛。这个约束不把 resource event 变成 durable replay stream，也不改变现有 wire shape。

## 9. Command identity 与幂等

保留当前 header/operation metadata 驱动的幂等模型：

- durable mutation 的 identity 由 `Idempotency-Key` 与 `Idempotency-Namespace` 承载；
- identity 绑定 method、normalized params fingerprint 和 discover 发布的幂等 namespace；
- 同 identity、同 fingerprint 返回同一 committed result；
- 同 identity、不同 fingerprint 返回闭合 conflict problem；
- retention 与 method 支持度由 `runtime.discover`/manifest 公布；
- query 不接收幂等 key；conditional write 使用独立 revision/etag 语义；
- response 丢失后重放 exact identity，而不是生成新 identity；
- 无法保证 exactly-once 的 provider effect 使用 attempt/receipt/unknown 领域状态显式表达。

renderer journal 只能保存恢复所需的 identity/fingerprint/namespace/deadline，不能保存 token、完整 params、prompt
或 optimistic UI state。

## 10. 分页

只允许 catalog 声明的两类集合：

- bounded：没有 cursor/limit，超过公开上限明确失败；
- cursor：request 接受 opaque cursor/limit，response 返回 items/data 与可选 `nextCursor`（以 concrete type 为准）。

cursor 绑定完整 query/sort 的服务端快照语义；切换筛选或 Runtime instance 必须丢弃 cursor。client 不解析 cursor。
列表 closure 应由一次 query 提供，禁止 renderer 对每一行发 N+1 补查。

## 11. 错误合同

错误 code 和 problem shape 以当前 generated error manifest 为准，不从 Codex 抄写数字区间。特别是
`-32001` 在 Lyra 当前合同中表示 `provider_error`，不得改造成 `overloaded`。

UI 只对稳定 symbolic problem type/code 分支，不直接依赖 JSON-RPC numeric code。未知 problem 使用安全 fallback，
并记录 contract violation。任何错误都不得泄漏 Go type、SQL、stack、provider secret、完整 prompt、token 或任意
服务端 HTML。

transport admission、protocol validation、application problem 和 provider failure 必须在日志与 metrics 中可区分；
它们不能都坍缩为 `internal_error`。

## 12. Embedded binding

public embedded consumer 和测试可以直接使用 typed endpoint，但必须经过同一 operation pipeline：

```text
request metadata
  -> protocol/version/capability validation
  -> operation lookup
  -> idempotency/pagination policy
  -> application port
  -> typed response/event
```

禁止 embedded client 直接 import handler、repository 或 domain implementation。embedded binding 不需要模拟 HTTP，
但必须与 HTTP 对同一输入产生等价 application outcome。

## 13. Desktop consumer contract

Wails 只拥有 native shell 和 Runtime bootstrap/supervision，不再叠加第二套业务 RPC：

- Go DesktopHost 选择 binary/config/data home，启动或连接 Runtime，校验 `/v2/info`、liveness、readiness 和
  `instanceId`，并负责 bounded shutdown；
- 本地 spawn 的实际 loopback endpoint/token path 通过 ADR-A2-023 的私有 one-shot descriptor 交接；descriptor
  不是 Runtime operation，不包含 token 原文，并在交接后删除；
- renderer 使用生成的 Lyra TypeScript client 直接访问 Runtime `/v2/rpc`；
- Runtime URL/token 通过只读 bootstrap contract 注入，不允许 renderer 自由执行进程或任意改写 endpoint；
- Wails event 只服务原生窗口/文件选择等 native concern，不能复制 Run/resource event；
- reconnection 先核对 protocol/instance identity 与幂等 namespace；Run replay 仅用于同 instance/root Segment，
  Runtime restart 通过 typed query/snapshot 冷恢复，再建立 `runs.subscribe` 和 `runtime.subscribe`。

NativeHost 是独立小合同：窗口/几何、选择绝对工作目录、保存经校验的 inline image。用户取消使用显式
`canceled` result，不以空字符串或 `false` 冒充成功。图片保存只接受 allowlisted MIME 的 bounded base64 data URL；renderer 不传目标路径，
NativeHost authoritative decode 后才打开系统 save dialog。该 native concern 不扩张 Lyra Runtime Protocol。

## 14. Operational probes

三条 sidecar 在本地与远程保持同一语义：

| Endpoint | 语义 |
| --- | --- |
| `/v2/info` | protocol、server/instance、transport endpoints 与公开 build facts |
| `/v2/health/live` | process/event loop 存活，不代表依赖已就绪 |
| `/v2/health/ready` | dispatcher 正在接收请求且必需依赖已就绪 |

同一次 bootstrap 观察到的 info、ready 和 `runtime.discover` 必须指向同一 `instanceId`。probe 不进入 operation
catalog，不返回 secret，也不能把 provider 暂时不可用错误等同于 Runtime process 不存活。

## 15. Artifact 合同

Artifact 是 portable snapshot，不是数据库备份：

- exact `app2/2` version；
- Session 的 provider/model 完整配对，不能由 model id 猜 provider；
- Session、Runs、Segments、Items、Interrupt settlements、Goal、Plan 的完整 closure；
- bounded/complete tool results；
- workspace checkpoint metadata，是否包含文件恢复材料必须显式；
- checksums、creation metadata 和 provenance。

`export md | json` 由一个 application use case 驱动；JSON 是 app2 artifact。import 先 strict validate 全量
closure，再在单一 transaction 提交，不允许部分导入。Artifact error enum 独立版本化，不与 live problem 盲目复用。

## 16. 存储合同

- app2 data home 与旧 `~/.lyra` 隔离；R1 已冻结默认路径 `~/.lyra-app2`，测试和 supervisor spawn 使用显式
  私有路径，不通过改写系统 `HOME` 偷换环境；
- metadata 表记录 schema epoch、storeId 和 createdByVersion；
- schema 打开先检查 exact epoch，再启动 worker/listener；
- reader 遇到 impossible state fail closed 并报告 corruption problem，不用默认值“修好”；
- backup/restore 是显式运维动作；corruption 处置先保留可恢复副本，不静默删除；
- timestamp 统一 UTC，wire 使用 RFC3339，数据库只用一种 canonical representation；
- ID 在 application admission 创建，数据库不偷偷替换；
- foreign key、transaction 与 ownership 明确处理删除，不依赖无主后台 sweep。

## 17. 合同变更检查表

任何合同变化必须回答：

1. 这是已证明的业务缺陷，还是仅仅偏好另一种写法？
2. 是否可以保留现有 Lyra wire contract，只重写内部实现？
3. canonical Go owner 和 operation registration 在哪里？
4. 是否引入第二种同义 method、binding、state 或 consumer？
5. method classification、stream、幂等、分页、capability、problem 和 materialized facts 是否完整？
6. generator、schema、OpenRPC、TS client、validators、examples 是否 diff-free？
7. HTTP 与 embedded conformance 是否同时通过？
8. Runtime handler、Desktop consumer、artifact/storage 与恢复是否同批更新？
9. capability ledger 的哪一行获得新证据？
10. 若确实 breaking，是否提升 protocol identity 并直接删除旧 shape，而不是添加 alias/fallback？
