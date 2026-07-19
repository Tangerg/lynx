# 并发 / 正确性深度审计 —— 2026-07 轮

> **目的**：可追溯地记录一轮针对 `app/runtime` 的深度并发/正确性审计——方法、每条发现的**根因**与**治本落点**、以及**刻意不改**项的理由。供后续会话核对"这条是否已修 / 为什么这么修 / 为什么那条没改"，避免重新论证。
>
> **基准**：本轮之前另有两批已完成工作——① 一批协议/幂等健壮性修复（前端幂等 key 稳定性、schedule 工具 revision、MCP 凭证 origin 绑定、MCP OAuth 锁范围、幂等 Complete-fail 收敛、schedule 分页次级排序、updateSession OCC 等 7 项），② 更早的 2026-06 并发审计轮（runs.start/resume TOCTOU、subagent progress clobber、scheduler MarkFired CAS、ForkSession orphan、Import/Rollback 原子性等）。本轮**不重复**那些，只收新发现。

---

## 0. 方法

六路对抗式审计，每路一个子系统、独立追踪 goroutine 拓扑 / 锁范围 / 事务边界，并对每条候选**构造具体触发交织**、判 CONFIRMED（追到端到端）/ PLAUSIBLE（latent，前提未在当前 caller 触发）：

1. **run 生命周期**（`application/runs`）· 2. **agentexec**（turn 状态机 / observer / offload）· 3. **sqlite**（tx seam / 事务原子性 / 迭代 / 迁移）· 4. **MCP / integrations / a2a**（连接 mutation 序列化 / goroutine 生命周期 / 凭证）· 5. **delivery / transport**（流 hub / 幂等分片 / SSE / lifecycle detach）· 6. **scheduler / compaction / bootstrap**（goroutine 归属 / shutdown 顺序 / 死锁）。

**判据**：每条问"根因消除了吗，还是只是现象不出现了？"——只让现象消失的打回。基线 `go test -race ./...` 全绿（80 包）。

---

## 1. 已修发现（13 条，两个 commit）

### 第一批 —— 4 HIGH + liveness + robustness + logic

| # | 子系统 | 缺陷（触发） | 治本落点 |
|---|---|---|---|
| MCP-1 | integrations | `ConfigureMCPServer`/`SetMCPServerEnabled` 持 `mcpMutationMu` **跨 live dial**（≤30s reconcile）→ 一个慢/挂 endpoint 冻结整个 MCP 控制面 | durable commit + policy 发布留锁内；(re)dial **detached 派发**（`dispatchMCPConnection`），复用已正确的异步 connect 模式；async 语义随之更新了 `TestMCP...OutlivesRequestCancellation` |
| MCP-2 | infra/mcp | `validateToolCatalog` 与 session commit 在**两次独立 `c.mu`** 持有中做 → 两个 sanitized 公名冲突的 server 并发 dial（各自 `session==nil` 互不可见）都通过、都 commit 重名 → 下一轮 `tools.NewRegistry` 构建失败 | collision 校验**并入 commit 临界区**（同一 `c.mu`），第二个到达者见首个已提交 session 而 fail-closed |
| RUN-1 | runs | Cancel 输给"在竞态中已提交的 park"被静默吞：`runs.cancel` 报成功、run 却 durably 停在 `Interrupted`（surface 为可 resume），需再 cancel 一次 | `Coordinator.Cancel` 的 live 分支在 `turns.Cancel` 后经 `cancelParkedRun` **reconcile 已提交 park**（走 durable cancel write-set） |
| AE-1 | agentexec + runs | Cancel 抢 parking turn：`emitInterrupt`（`parkIfLive` 后到 emit 之间不复查 park 归属）在 Cancel 已 `claimPark`+emit `TurnEnd` 后仍 emit `TurnInterrupted` → pump 处理终态后事件 → Suspend 已 terminalize 的 run | **pump 强制不变量**"终态提交后不再处理任何事件"（`finished` break）——pump 是 durable run-state 权威所有者，turn 层多发的事件因此永不到达 durable state/订阅者（emit 遇 `st.ctx.Done()` 返 false，不泄漏） |
| DEL-1 | transport/http | SSE 每帧写是阻塞 `net/http` Write，`ctx.Done` 打不断它 → connected-but-stalled client（满 TCP 窗）把请求 goroutine + 上游 mapper 停在 Write 里直到 TCP keepalive（~2h），并卡 graceful shutdown | 每帧写前 `ResponseController.SetWriteDeadline`（`recordingResponseWriter` 新增 `Unwrap()` 使 controller 触底层 conn）→ 单次阻塞写 fail-and-detach |
| DEL-2 | delivery/server | detached 流 goroutine（`mapRunEvents` 跑 `presentRunEvent`，含 exhaustive-switch `panic` + 指针操作）在请求作用域 recover 之外 → 一个 panic 崩**整进程** | goroutine body 加 recover：panic 收敛为 record on span + `close(out)` → **单流终止**，进程存活 |
| SCB-1 | maintenance | compaction token 触发阈值在构造时**冻结为默认模型窗口** → run 若 pin 更小窗模型，累积几条大 tool 结果即在消息数 trigger 之下、又远低于默认窗 → 永不压缩 → provider `context_length_exceeded` | `MaybeCompact` 加 `contextWindow` 参数，per-run 经**包级 `catalog.Lookup(provider, model)`** 解析窗口、`tokenTrigger` per-call 计算；turnState 加 `provider` 字段 |
| SQL-3 | sqlite | boot 恢复"不完整 interrupt 边界"守卫用 `time.IsZero()`，但列经 `time.Unix(0, ns)` 解码——schema 默认 0 是 1970 epoch（非 Go 零值 year 1）→ 守卫**恒不触发**（死代码） | 改判 decoded `UnixNano() == 0` |
| RUN-2 | runs | 事件 cursor `%011d` 是**最小**宽 11、未封顶 → 进程内 seq 过 10^11 后 12 位串 lexical 排在 11 位前，破坏 Journal 依赖的 lexical==numeric replay 序（latent，10^11 不可达） | `%020d` 覆盖满 uint64 宽 |

### 第二批 —— latent 硬化（今天安全、结构化治本）

| # | 子系统 | 缺陷 | 治本落点 |
|---|---|---|---|
| SCB-2 | scheduler | cron worker 在 run **启动失败**（非取消，如 session-busy）时仍 `MarkFired` 推进 cursor → 该 occurrence 静默跳过至下 slot | 失败不推进 cursor、下 tick 重试；**统一有界**（`maxFireRetries`，不分类瞬时/持久——守 lyra 反向不变量）后才放弃 occurrence + 推进；重试计数用 Run 循环单 goroutine 本地 map，无锁无 schema 改 |
| SQL-2 | sqlite | `idempotency.Claim` / `codebaseindex.{ReplaceFile,DeleteFile,Clear}` 裸 `s.db.BeginTx` 绕过 `conn(ctx)` seam → 若未来嵌进 caller 的跨 store 事务，单连接池（MaxOpenConns=1）**死锁** | 全部改走 `RunInTx(ctx, s.db, …)` + `conn(ctx)`，不变量**结构化**而非依赖当前 caller 从不嵌套 |
| SQL-1 | sqlite / conversation | session 删除 / 全历史重置用 `Truncate(0)`，经 `Read`(skip-malformed) 往返：`Read` 返空时 `keepN>=len` 短路 no-op → 全损坏会话**留孤儿行**（keyed to 已删 session，无 FK 清理） | 新 `conversation.Messages.Clear`（委托 `Store.Replace` 无消息=裸 `DELETE WHERE conversation_id=?`、不经 Read），两处全清（delete / restore）改用它 |
| SCB-3 | component/taskgroup | `Group.Close` 用无界 `wg.Wait()` join → 未来某 task 若忽略 ctx，`Host.Close` 顺序驱动的 shutdown **永久挂** | 有界 join（`closeGraceTimeout` 宽限窗，超时则 proceed 泄漏该 misbehaving task 而非挂整进程）——今天无 task 触及，安全网，镜像 turn dispatcher 的 bounded close |

---

## 2. 刻意不改（防未来会话重新论证）

**RUN-3 · `reducer` 在 commit 决策前 `drain` r.tools**（`application/runs/reducer_interrupt.go` / `reducer.go`）
`reduce(TurnInterrupted)` 与 `reduce(TurnEnd)` 无条件 `tools.drain()`（清空 map）在 pump 决定是否 commit **之前**；若随后 skip publish（cancel 赢 interrupt-commit 竞态）或 commit 失败，deferred `synthesizeTerminal` 的 `drainTools()` 拿到空 map，in-flight tool 调用在合成终态里没有 `ItemCompleted`/incomplete 记录。

**为何不改**：审计追证其**当前非缺陷、不损坏**——`ItemStarted` 是**非持久**事件（durable transcript 从不持有那些行），live stream 在终态解析它们；只是"reducer 在 commit 决策前 mutate 可消费状态"是个脆弱模式。**只有当未来把 `ItemStarted` 改成持久事件时，它才会变成真 bug。** 此刻重构 reducer 的 tool-draining 是为不存在的需求提前抽象（YAGNI）。**正解**：等 `ItemStarted` 真持久化时，与那次改动一并把 tool-drain 移到 commit 决策**之后**——而非现在治标性地动它。

---

## 3. 审计判为 SOUND 的区域（记录，避免重复审）

- **runs**：`Registry`（admission-claim 与 open-run handoff 原子）、`Journal`+subscribers（`j.mu→s.mu` 单向、Close/cancel/timer 幂等、closed-journal 返 durable backlog+closed channel）、`handle` interrupt/cancel 线性化（`interruptDone` join、无 I/O under lock）。
- **agentexec**：observer start-ordering（`publishing` flag + re-scan 补 lost-wakeup）、per-process `toolObservation`（dedup key 不跨 delegation 树碰撞）、eviction/offload 单一真源、`claimPark`/`terminalOnce`/steering `flushed` gate。
- **sqlite**：cross-store 写集全走单 `RunInTx`/`conn`、OCC/CAS +1 正确、每 `rows` loop 有 `rows.Err()`、时间单位表内一致、`MaxOpenConns(1)` 进程级串行、`discardSchema` 崩溃自愈。
- **MCP**：origin 绑定 roundtripper（scheme+host+port 严格等 + redirect 每跳复验）、corrupt-data fail-closed、policy 绑 tool identity、env 注入 denylist、OAuth loopback goroutine 清理、a2a dial-once + `sync.Once`。
- **delivery**：`Journal.Subscribe` 快照+注册原子（无 miss-last-frame/dup/lost-terminal）、幂等分片锁序 shard→pendingMu→store.mu、`workspaceHub` unregister-before-close、metadata stripping 只 reassign（不原地 mutate 共享 request）、`withServerLifecycle` AfterFunc detach。
- **scheduler/bootstrap**：Host 逆依赖序 shutdown（pump→dispatcher→tools→resources，`sync.Once` 幂等）、MarkFired CAS + `WithoutCancel` 持久化跨 shutdown、checkpoint tree-lock→repo-lock 一致序、compaction ladder copy-on-write + 位置保留（tool-pair 不拆）。
