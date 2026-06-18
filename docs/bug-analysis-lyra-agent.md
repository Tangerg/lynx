# lyra + agent 模块 Bug 深度排查报告

> 排查范围：`lyra/` + `agent/` 两模块全部代码（排除 pkg/）
> 排查时间：2026-06-17（五轮覆盖）
> 方法：go vet + go test (含 -race) + 五轮人工深度审查，覆盖 goroutine 生命周期/channel/panic/并发数据/资源泄漏/错误吞没/snapshot/工具安全/预算/HITL/Blackboard/中间件/路径保护/错误链/死代码/内存模型/Config 校验/整数溢出/测试质量/timer 竞态/domain service/SQLite

---

## 0. 执行摘要

| 严重度 | R1 | R2 | R3 | R4 | R5 | 合计 |
|---|---|---|---|---|---|---|
| 🔴 Critical | 1 | 0 | 0 | 0 | 0 | **1** |
| 🟠 High | 3 | 0 | 0 | 0 | 0 | **3** |
| 🟡 Medium | 4 | 4 | 4 | 3 | 2 | **17** |
| 🟢 Low | 2 | 2 | 3 | 2 | 3 | **12** |

**R1**：goroutine 生命周期 + 并发竞态  
**R2**：数据安全 + snapshot 细节  
**R3**：工具安全/预算/HITL/Blackboard 语义 — 确认 12+ 处防御正确  
**R4**：错误链/内存模型/Config 校验 — 全部通过  
**R5**：整数溢出/timer 竞态/测试质量/domain service/SQLite — 窄窗口竞态 2 处

**好消息**：全部测试通过（含 `-race`），`go vet` 零告警。所有并发数据结构（map/slice/atomic）的锁机制审查通过。代码整体质量很高。

---

## 1. 🔴 Critical — Race Condition：Cancel 与 handleWaiting 双关 events channel

**文件**：`lyra/internal/kernel/turn/inmemory.go:215` (Cancel) + `lyra/internal/kernel/turn/turn.go:226` (handleWaiting)

**根因**：`Cancel` 和 `handleWaiting`（ctx-canceled 路径）之间没有互斥机制，两者都可能调用 `finishTurn` → `endTurn` → `close(st.events)`。

**触发场景**：

```
Goroutine A (Cancel):                           Goroutine B (handleWaiting, on runTurn goroutine):
─────────────────────────────────               ────────────────────────────────────────────
1. findTurn → state                              1. drive: proc.Done() unblocks (ctx canceled)
2. state.cancel()    ← ctx canceled              2. proc.Status() == Waiting → enter handleWaiting
3. proc := state.process()
4.                                                3. st.ctx.Err() != nil → true
5.                                                4. proc.Cancel()
6.                                                5. s.finishTurn(st, TurnEndCanceled)
7.                                                6.   → emit(TurnEnd)    (ok, channel open)
8.                                                7.   → endTurn → close(st.events)
9.                                                8.   → delete(s.turns, id)
10. claimed := state.claimPark() → true!
   (parked flag was never cleared by handleWaiting)
11. s.finishTurn(state, TurnEndCanceled)
12.   → emit(TurnEnd) → st.events <- stamped
13.   → ⚡ PANIC: send on closed channel
```

**为什么测试没抓到**：需要精确的 goroutine 调度时序（B 在 `state.cancel()` 之后、`claimPark()` 之前执行 finishTurn），常规 -race 测试难以覆盖。

**修复建议**：在 `handleWaiting` 的 ctx-canceled 分支中调用 `claimPark()` 防止 Cancel 二次 finish：

```go
// lyra/internal/kernel/turn/turn.go:231
if st.ctx.Err() != nil {
    st.claimPark()  // prevent racing Cancel from double-finishing
    _ = proc.Cancel()
    s.finishTurn(st, TurnEndCanceled)
    return
}
```

或者让 `finishTurn` / `endTurn` 具有幂等性（用 `sync.Once` 或在 `endTurn` 中检查 channel 是否已关闭）。

---

## 2. 🟠 High — 3 处 goroutine 无 panic recovery

以下 goroutine 中任何一次 panic 都会导致 goroutine 静默死亡，上层消费者**永久阻塞**或**状态泄漏**。

### 2.1 `runTurn` goroutine — 事件通道永不关闭

**文件**：`lyra/internal/kernel/turn/inmemory.go:108`

```go
go s.runTurn(req, state)
```

`runTurn` → `drive` → `endTurn` → `close(st.events)` 是事件通道的唯一关闭点。如果 `runTurn` 在执行过程中 panic，`close(st.events)` 永远不会被调用，`Events()` 的 `for { select { case ev, ok := <-state.events } }` 会永远阻塞在接收上。

**影响**：消费方（SSE 连接）永久 hang，无法通过 `runs.cancel` 恢复（Cancel 需要 findTurn，但 turns map 中的条目未被删除）。

### 2.2 `drive` goroutine（resume 后继段）— 同 2.1

**文件**：`lyra/internal/kernel/turn/turn.go:268` (via `resumeAndDrive`)

```go
go s.drive(state, resumed)
```

HITL resume 后启动的 continuation driver goroutine，无 panic recovery。若 panic，终端事件永不发出，turn 变成 zombies。

### 2.3 `pumpRun` goroutine — run entry 泄漏

**文件**：`lyra/internal/delivery/server/pump.go:51`

```go
go s.pumpRun(runCtx, runID, parentRunID, handle, inner, hub, userInput, resume, model)
```

`pumpRun` 的 defer 负责：
- 合成 `run.finished` 事件
- `hub.Close()`
- 删除 `s.runs[runID]`
- checkpoint 异步快照

若 `pumpRun` panic，以上全部跳过去。`hub` 永不被 close → hub 订阅者永久阻塞。`s.runs[runID]` 永不删除 → 内存泄漏。

**修复建议**（适用于 2.1-2.3）：在每个 `go` 语句处添加统一的 panic recovery：

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("goroutine panic", "panic", r, "goroutine", "runTurn")
            // 如果是 runTurn：确保 close(st.events) + 从 turns map 删除
            // 如果是 pumpRun：确保 hub.Close() + delete(s.runs, runID)
        }
    }()
    s.runTurn(req, state)
}()
```

---

## 3. 🟡 Medium

### 3.1 `StartAgent` / `ContinueProcessAsync` goroutines 无 panic recovery

**文件**：`agent/runtime/platform_run.go:131,168`

```go
go func() {
    done <- proc.run(normalizeContext(ctx))
    close(done)
}()
```

若 `proc.run` panic，`done` channel 永远收不到值也永不关闭。消费者（`drive`: `runErr := <-doneCh`）永久阻塞。

### 3.2 `snapshotCheckpoint` goroutine 无 panic recovery

**文件**：`lyra/internal/delivery/server/pump.go:152`

```go
go s.snapshotCheckpoint(context.WithoutCancel(ctx), handle.SessionID, runID)
```

fire-and-forget goroutine。若 panic，goroutine 静默死亡，checkpoint 遗漏但不会导致系统级故障。

### 3.3 `emit` 函数存在理论竞态

**文件**：`lyra/internal/kernel/turn/turn.go:390-400`

```go
select {         // fast path: non-blocking
case st.events <- stamped:
    return
default:
}
select {         // slow path: blocking
case st.events <- stamped:
case <-st.ctx.Done():
}
```

如果 events channel 在第一个和第二个 `select` 之间被关闭（由另一个 goroutine 的 `endTurn` 执行），第二个 `select` 的 `st.events <- stamped` 会 panic。

**触发条件**：需要 `endTurn` 精确在两个 select 之间执行，概率极低。但一旦发生就是 panic。

### 3.4 `emitTurnEnd` 忽略 `proc.Output()` 错误

**文件**：`lyra/internal/kernel/turn/turn.go:327`

```go
out, _ := proc.Output()
```

当 `Output()` 返回错误时，`out` 为零值。后续 `planTurnEnd` 使用 `out.StoppedOnBudget`（false）、`out.Usage`（零值）来构建 TurnEnd。结果：一个非正常退出的 turn 可能被报告为 `TurnEndCompleted`（零值 Usage），误导上层 observability。

**修复**：检查 error，若 Output 失败则在 TurnEnd 中反映：

```go
out, outErr := proc.Output()
if outErr != nil && runErr != nil {
    // Both failed → prefer the run error, don't misreport as completed
}
```

---

## 4. 🟢 Low

### 4.1 `SpawnChildAsync` 子进程依赖显式 `KillChildren`

**文件**：`agent/runtime/child.go:144-164`

```go
done := platform.ContinueProcessAsync(context.WithoutCancel(ctx), child.ID())
```

后台子进程的 ctx 通过 `context.WithoutCancel` 脱离了父进程的 ctx。父进程结束时，子进程不会被自动取消。清理完全依赖 host 代码调用 `platform.KillChildren(parentID)`。

**当前状态**：lyra 的 `chatProcess.Discard` 或 turn 终态清理中应有此调用。但文档化的行为要求在**所有** parent 退出路径上都调用 `KillChildren`，否则子进程泄漏。这是一个"契约漏洞" — 接口未强制但依赖调用方纪律。

### 4.2 `StartChat` 可能返回含 nil proc 的 chatProcess

**文件**：`lyra/internal/kernel/chatturn.go:86-93`

```go
func (e *Engine) StartChat(ctx context.Context, req RunChatRequest) ChatProcess {
    proc, done := e.platform.StartAgent(ctx, e.agent, ...)
    return &chatProcess{proc: proc, done: done, platform: e.platform}
}
```

`StartAgent` 在 `createProcess` 失败时返回 `(nil, done)`。虽然当前 `e.agent` 在 `New()` 时已通过 `Deploy` 检查，但未来若引入动态 agent 注册，`createProcess` 可能失败。此时 `cp.ID()` → nil pointer deref。

---

## 5. 🟡 Round 2 发现（Medium）

### 5.1 `persistItem` / `persistRun` 静默吞没 SQLite 写入错误

**文件**：`lyra/internal/delivery/server/history.go:67,87`

```go
_ = store.AppendItem(ctx, transcript.Item{...})  // line 67
_ = store.PutRun(ctx, transcript.Run{...})         // line 87
```

`persistStreamEvent` 函数被标记为 "best-effort"（注释："persistence errors never fail the live stream"），SQLite 写入失败（磁盘满、DB 损坏、表锁）时 item/run 记录**静默丢失**——没有日志、没有 metric、没有 span error。

**影响**：运行态 transcript 数据缺失，但不影响实时代理对话。故障后 `items.list` 返回不完整记录，`runs.subscribe` 无法重放丢失的事件。

**修复**：至少打一条 OTel span event 或 metric counter（`history.persist.errors`），让运维可观测而非纯沉默。

### 5.2 `editguard` 工具包装器吞没 JSON 解析错误

**文件**：`lyra/internal/kernel/toolset/editguard.go:44,62,91,128,156,236`

所有 6 处 `_ = json.Unmarshal([]byte(arguments), &a)` 在解析失败时 `a.Path` 为空字符串，守卫逻辑被旁路：

```go
_ = json.Unmarshal([]byte(arguments), &a)
if a.Path != "" {  // 解析失败 → Path=="" → 守卫跳过
    tr.Record(...)
}
```

**影响**：LLM 生成合法 JSON 时此路径正常。若 LLM 生成非法 JSON，被包装的 inner tool 也会解析失败并返回 error，守卫不会误放过。所以此处无安全漏洞，但**诊断信息丢失**：read tracking 在 JSON 解析失败时静默跳过，没有记录实际被 inner tool 读取的文件。

### 5.3 `exec.Manager.Launch` 和 `lsp client` 使用 `context.Background()`

**文件**：`lyra/internal/infra/exec/exec.go:72,74`、`lyra/internal/infra/lsp/client.go:75,88`

```go
runCtx, cancel := context.WithCancel(context.Background())  // exec.go:72
connCtx, cancel := context.WithCancel(context.Background()) // client.go:75
```

后台 shell 和 LSP server 连接完全脱离父 trace context。OTel 链路上这些后台操作的 span 不会附着在发起的 HTTP 请求上，trace 视图断裂。

**影响**：observability 缺失（无 trace 链接），但操作不受父请求取消影响（这是设计意图 —— 后台进程应独立于请求生命周期）。

**建议**：用 `context.WithoutCancel(parentCtx)` 替代 `context.Background()`，保留 trace 传播而非取消传播。

### 5.4 `Snapshot.Restore` 丢失 `protected` 标记

**文件**：`agent/runtime/in_memory_blackboard.go:274`

```go
func (b *inMemoryBlackboard) Restore(...) {
    // ...
    clear(b.protected)   // line 274 — 保护标记被清空
}
```

`Restore` 清空 protected 标记（注释："Protected markers are reset because they have no portable wire form"）。恢复后 protected 条目仍然存在于 `named` map 中（通过 `Restore` 传入），但 `Clear()` 调用时不会保留它们。

**影响**：文档化行为。lyra 的 chat action 在每个 turn 开始时重新 BindProtected（Cwd + SessionID），所以此缺失不影响正常运作。但若外部代码在 restore 后、re-tick 之前调用 `Clear()`，protected 条目会丢失。

**风险**：很低。当前唯一 restore 的消费方（lyra `RestoreChat`）立即执行 re-tick，action body 重新 BindProtected。外部 consumer 应了解此限制。

---

## 6. 🟢 Round 2 发现（Low）

### 6.1 `editguard.Tracker.get` 使用 `Lock()` 而非 `RLock()`

**文件**：`lyra/internal/domain/editguard/editguard.go:97`

```go
func (t *Tracker) get(session, abs string) (stamp, bool) {
    t.mu.Lock()    // 应该是 RLock()
    defer t.mu.Unlock()
    st, ok := t.seen[session][abs]
    return st, ok
}
```

纯读操作用了排他锁。不是正确性 bug（Mutex 对读取完全安全），但 `Check`（频繁调用 path）在每个文件修改前都要调用 `get`，在高并发多 session 场景下读写会串行。

**修复**：`get` 改为 `RLock()`，`put` 保持 `Lock()`。

### 6.2 `translator` 吞没类型断言失败

**文件**：`lyra/internal/delivery/server/translator.go:50-52`

```go
tool, _ := in.Payload["tool"].(map[string]any)
name, _ := tool["name"].(string)
args, _ := tool["arguments"].(map[string]any)
```

三连 comma-ok 全部吞没。若 Payload 不包含 `tool` key 或类型不匹配，`name` 为 `""`，`addItem` 在 line 53 被 `if name != ""` 正确门控跳过。所以逻辑安全，但若上游 Payload 格式漂移，问题会静默出现。

---

## 7. 🟡 Round 3 发现（Medium）

### 7.1 `BudgetPolicy` float64 零值歧义

**文件**：`agent/core/early_termination.go:45-51`

```go
case p.Budget.CostLimit > 0 && cost >= p.Budget.CostLimit:
```

`CostLimit` 是 `float64`，注释说 "A non-zero limit means 'applies'; zero leaves that dimension unbounded"。但 `float64(0.0)` 无法区分"未设置"和"设为 0（立即终止）"。Go float64 的零值语义在此处是经典歧义。

**影响**：如果有人显式将 `CostLimit` 设为 `0.0` 期望"无预算即停止"，将被解释为 `0 > 0 == false` → 无限制。当前所有消费方均使用正数或零（默认），所以无实际触发场景。

**建议**：换成 `*float64` 或引入 sentinel（如 `-1` = unbounded），长期更好但当前不紧急。

### 7.2 `wrapTool` 吞没 `chat.NewTool` 构造错误

**文件**：`lyra/internal/kernel/toolset/resolver.go:45`

```go
func wrapTool(inner chat.Tool, call func(...) (string, error)) chat.Tool {
    t, _ := chat.NewTool(inner.Definition(), inner.Metadata(), call)
    return t
}
```

注释声称 "A valid inner yields a valid definition, so the error is impossible"。但如果 `inner` 为 nil，`inner.Definition()` 会 panic。如果 `inner.Definition()` 返回非法 JSON Schema，`chat.NewTool` 可能返回 error + nil tool。六处调用 `wrapTool` 的位置都没有 nil 检查。

**影响**：正常 flow 下不会触发（所有 inner 都是 `chat.NewTool` 构造的有效 tool）。但若 tool 构造链中某环返回 nil tool（如 `shell.Build` 返回 nil），后续 `wrapTool` 可能得到 nil tool 并静默传播。

**建议**：至少加 `if inner == nil { return nil }` 守卫。

### 7.3 HITL `Interrupt` 类型不匹配时静默创建新 awaitable

**文件**：`agent/hitl/interrupt.go:76-81`

```go
if v, ok := bb.Get(resumeSlotKey(key)); ok {
    if typed, ok := v.(R); ok {
        return typed, true, nil
    }
}
// Type assertion failed → falls through, creates NEW awaitable
awaitable := NewTypedRequest(value, func(r R) core.ResponseImpact {
    bb.Set(resumeSlotKey(key), r)  // overwrites the old (incompatible) value
    ...
})
```

如果同一个 `key` 被两个不同类型的 `Interrupt` 调用（代码重构后 `R` 变了），第二次 resume 时第一次的响应因类型不匹配被丢弃，`bb.Set(...)` 覆盖旧值。旧响应永久丢失。

**影响**：极低 —— 需要 crash-restart 期间 `key` 不变但 `R` 类型改变。当前所有 HITL 调用使用稳定类型（`interrupts.Resolution`、`string`）。

**建议**：类型不匹配时 log warning 而非静默覆盖。

### 7.4 `ClearBlackboard` + HITL 交互：partial state 残留

**文件**：`agent/core/action_typed.go:60-74`

```go
if pc.InputAwaited() {
    return ActionWaiting  // ClearBlackboard NOT run here
}
if a.metadata.ClearBlackboard {
    pc.Blackboard.Clear()  // runs only on non-awaited success
}
```

当 `ClearBlackboard=true` 的 action 调用了 `AwaitInput`（HITL interrupt），`ClearBlackboard` 不会执行。Action body 在 `AwaitInput` 之前可能已写入部分数据到 blackboard（如绑定中间结果），这些数据在 resume 后仍然存在。Action body 依赖**自身是幂等的**来处理此情况（resume 时检测 saved response，不走 AwaitInput 路径，正常完成并执行 Clear）。

**影响**：非 bug —— 这是 `ClearBlackboard` 语义的设计意图："On success, clear"。HITL suspend 不是 success。但此设计对 action body 编写者有隐含契约要求。

---

## 8. 🟢 Round 3 发现（Low）

### 8.1 `runChatTurn` 预算检查在工具轮次之后

**文件**：`lyra/internal/kernel/turn.go:109-113`

预算超过阈值时，检查发生在 `recordRound()` 之后（而非下一轮 LLM 调用之前）。这意味着超预算时可能多付一次 LLM 调用。

**文档化行为**：`chatInput.MaxBudget` 注释明确 "stops cleanly after the current round"。非 bug。

### 8.2 `engine.Close()` 不取消运行中 turn

**文件**：`lyra/internal/kernel/engine.go:222-234`

`Close()` 只关闭 MCP/A2A/LSP/exec 资源连接，不取消正在运行的 turns。In-flight turns 在引擎关闭后可能尝试调用已关闭的 LLM client 或 MCP 连接。

**影响**：低 —— 当前 shutdown flow 由 lyra server 编排，server 应先取消所有 turns 再调用 `engine.Close()`。但 `Close()` 自身无此守卫，若调用顺序错误则产生 spurious error。

### 8.3 BudgetPolicy `>=` 浮点比较精度

**文件**：`agent/core/early_termination.go:45`

`cost >= p.Budget.CostLimit` 使用 `>=` 浮点比较。在成本恰好等于预算边界时（概率极低但可能），会因浮点表示误差产假阳性/假阴性。

**影响**：可忽略 —— 成本值通常来自整数美分计算，浮点误差在 sub-cent 级别。

---

## 9. Round 3 确认正确的防御模式 ✅

以下模式经 Round 3 深度审查确认正确：

| 位置 | 模式 | 验证结论 |
|---|---|---|
| `editguard.withPathGuard` | `.git` 目录保护 + 路径穿越 (`../`) 检测 | ✅ 通过 `resolveAbs` + `protectedDirHit` 逐级上溯 |
| `editguard.pathLocker` | per-path 串行化并发写操作 | ✅ ref-count + per-path lock 正确，无死锁 |
| `hitl.Interrupt[T]` | 泛型 typed resume + blackboard slot | ✅ `resumeSlotKey` 隔离 + `ImpactUpdated` 正确 |
| `typedAction.Execute` | ClearBlackboard + HITL 暂停交互 | ✅ 文档化契约：suspend ≠ success，不执行 Clear |
| `processBudget.usage` | 子进程子树递归聚合 | ✅ RLock 递归安全（parent→children DAG） |
| `runChatTurn` | inflightTail 中断保存→resume 恢复 | ✅ 通过 `chat.UnmarshalMessage` round-trip，marshaling 失败静默丢弃 |
| `computePreconditionsAndEffects` | canRerun=false 的 hasRun 门控 | ✅ 前置 False + 后置 True → 执行后自动排除 |
| `workspace_stream` | hub 注册/取消 + gitWatcher 生命周期 | ✅ 先 unregister 再 close channel，防 late send |
| `wrapTool` at call sites | 所有调用方保证 inner 非 nil | ⚠️ 正确但不防御；见 #7.2 |

---

## 11. 🟡 Round 4 发现（Medium）

### 11.1 失败的 child spawn 在 parent budget 中留下孤儿引用

**文件**：`agent/runtime/platform_process.go:87` + `agent/runtime/child.go:259-264`

```go
// CreateChildProcess (platform_process.go:82-88)
child, err := p.createProcess(agentDef, nil, options)  // registers child
...
child.parentID = parent.id
parent.budget.addChild(child)  // always called, even if link fails later

// prepareChild (child.go:259-264)
if err := linkChildSession(ctx, platform, child, parentProc, agentDef); err != nil {
    _ = platform.RemoveProcess(child.ID())  // removes from registry...
    // ...but parent.budget.children still references this child
    return nil, fmt.Errorf("spawn child %q: link session: %w", agentDef.Name, err)
}
```

`CreateChildProcess` 在第 87 行调用 `parent.budget.addChild(child)` **之后**，`prepareChild` 在第 259-264 行若 `linkChildSession` 失败则调用 `RemoveProcess` 从注册表中移除 child，但 `processBudget` 没有 `removeChild` 方法。stale 引用残留在 budget 的 `children` slice 中。

**影响**：极低 — `linkChildSession` 的失败路径极窄（仅 `sessionStore.Save` 返回 error），且残留的 child 处于 `StatusNotStarted`，`Usage()` 返回零值，不影响预算计算。但 `children` slice 存在微小的内存泄漏。

**修复**：在 `processBudget` 中新增 `removeChild` 方法，或让 `RemoveProcess` 回调清理 budget。

### 11.2 `maybeAutoSnapshot` 同步阻塞 run loop

**文件**：`agent/runtime/run.go:52,69-80`

```go
// run loop
p.maybeAutoSnapshot(ctx)  // blocks on DB I/O inside the tick loop

func (p *AgentProcess) maybeAutoSnapshot(ctx context.Context) {
    ...
    if err := p.platform.processStore.Save(ctx, p.Snapshot()); err != nil {
        ...
    }
}
```

`maybeAutoSnapshot` 在每次 tick 后**同步调用** `processStore.Save`。若 SQLite 写入变慢（大事务、锁争用），整个 run loop 被阻塞，终端事件交付延迟。

**现状**：pump.go 的 checkpoint 已改为异步（注释明确 "It used to run synchronously... It now runs asynchronously off the critical path"），但 agent 层的 auto-snapshot 仍是同步的。

**影响**：生产环境下 SQLite 通常很快，影响有限。但极端场景可能造成 ~100ms 的 tick 延迟。

### 11.3 `AgentProcess.startedAt` 在 Snapshot 中无锁读取

**文件**：`agent/runtime/agent_process.go:42` + `agent/runtime/process_snapshot.go:30`

```go
// AgentProcess
startedAt time.Time  // written once, but no happens-before documented

// Snapshot
snap.StartedAt = p.StartedAt()  // reads startedAt without lock
```

`startedAt` 在构造函数中写入，之后永不变更。happens-before 链为：构造函数写入 → `procs.register(Lock/Unlock)` → 读取方 `ProcessByID(RLock/RUnlock)` → 可见。**当前正确**，但依赖隐式 happens-before（通过 processRegistry 的 mutex），而非显式文档化。

**影响**：零。当前所有读取路径均正确同步。标记此处为"隐式正确但脆弱" — 若将来有人移除 processRegistry 的 mutex 或改变注册顺序，此处可能断裂。

**建议**：加注释说明 happens-before 链或改用 `atomic.Pointer[time.Time]`。

---

## 12. 🟢 Round 4 发现（Low）

### 12.1 `parseMCPServerValue` 不做 name 合法性校验

**文件**：`lyra/internal/config/config.go:281-306`

MCP server name 直接来自用户输入 (`LYRA_MCP_SERVERS` 环境变量或 yaml)，无任何格式校验（长度、字符集、空字符串已在 line 230 检查）。非法 name 只在后续 `mcp.Dial` 时才会报错。

**影响**：低 — `mcp.Dial` 会完成校验并返回可读错误，非静默失败。

### 12.2 `BudgetPolicy.CostLimit` 浮点比较使用 `>=` 而非 tolerance

**文件**：`agent/core/early_termination.go:45`

```go
case p.Budget.CostLimit > 0 && cost >= p.Budget.CostLimit:
```

浮点直接比较。成本值来自 token × 定价（整数乘法），通常精确。但若 future pricing 引入分数定价，比较可能不精确。

**影响**：当前可忽略。成本计算路径均为整数 token × 固定定价率，无浮点累积误差。

---

## 13. Round 4 确认正确的领域 ✅

### 内存模型全面审查

| 结构 | 字段 | 写入方 | 读取方 | happens-before 链 | 结论 |
|---|---|---|---|---|---|
| `turnState.ctx` | context | StartTurn (before go) | Cancel/Resume (after findTurn mutex) | `go` statement + `s.mu.Lock()` in findTurn | ✅ |
| `turnState.model` | string | StartTurn (before go) | emitTurnEnd (drive goroutine) | `go` statement | ✅ |
| `turnState.lifecycle` | *turnLifecycle | runTurn (before setProc) | emitTurnEnd (after process() mutex) | `st.mu.Lock()` in process() | ✅ |
| `turnState.cwd` | string | StartTurn (before go) | postTurnMaintenance (drive goroutine) | `go` statement | ✅ |
| `AgentProcess.startedAt` | time.Time | newAgentProcess | Snapshot() via tick goroutine | processRegistry mutex via makeRunning | ✅ |
| `AgentProcess.parentID` | string | CreateChildProcess | Snapshot()/KillChildren | processRegistry mutex | ✅ |

### 错误包装链审查

- 生产代码中仅 **1 处** `%v` 误用（`dispatch.go:158`，`%v` 用于格式化权限列表，非错误包装）
- 所有 sentinel error (`ErrTurnNotFound`, `ErrSnapshotNotFound`, `core.ErrSessionNotFound` 等) 均被 `%w` 正确包装，`errors.Is`/`errors.As` 链路完整

### 死代码检查

- `go vet` 零告警，确认无不可达代码、无未使用变量/导入

### Config 校验审查

- `config.Load()` 完整校验必填字段（provider、apiKey），`parseMCPServers`/`parseA2AAgents` 处理格式错误、空值、重复名（在 `mcp.Dial` 中检查）
- `engine.New()` 的 `Config` 通过 nil guard + `Validate()` 双重检查

---

## 15. 🟡 Round 5 发现（Medium）

### 15.1 `bash` 工具 select 竞态导致已完成命令的结果丢失

**文件**：`lyra/internal/kernel/toolset/shell/shell.go:97-110`

```go
select {
case <-sh.Done():        // shell completed
    // return result ✅
case <-timer.C:          // auto-background timeout
    // return backgrounded ✅
case <-ctx.Done():       // turn canceled
    mgr.Kill(id)
    return "", ctx.Err() // ⚠️ loses result if shell ALSO finished
}
```

三条 case 同时就绪时（shell 恰好在 turn cancel 的同时完成），Go `select` 随机选择。若选中 `ctx.Done()`，已成功完成的 shell 结果被丢弃 —— 用户看不到命令输出。

**根因**：shell 用 `context.Background()` 启动（`exec.go:72`），不随 turn cancel 取消。cmd 可能正常退出，但其 `Done` channel 与 `ctx.Done()` 同时就绪。

**影响**：极窄窗口（shell 完成 ↔ turn cancel 之间的微秒级窗口），但一旦触发意味着有效命令结果静默丢失。

**修复**：`ctx.Done()` case 中先检查 shell 是否已完成：
```go
case <-ctx.Done():
    select {
    case <-sh.Done():
        // shell finished — return result instead of losing it
        out, _ := sh.Read()
        code, killed, dur := sh.Outcome()
        mgr.Remove(id)
        return completedJSON(out, false, code, killed, dur), nil
    default:
        mgr.Kill(id)
        return "", ctx.Err()
    }
```

### 15.2 `conversation.Count` 全量读取消息历史

**文件**：`lyra/internal/domain/conversation/conversation.go:66-71`

```go
func (s *Service) Count(ctx context.Context, sessionID string) (int, error) {
    msgs, err := s.store.Read(ctx, sessionID)
    if err != nil { return 0, err }
    return len(msgs), nil
}
```

`Count()` 调用 `store.Read()` 从 SQLite 读取**所有**消息（包括 JSON blob），仅为了返回 `len(msgs)`。当会话有数千条消息时，这会加载大量数据到内存。

**影响**：`Count` 被 `persistStreamEvent`（`history.go:35`）在每次 `run.finished` 时调用，用于记录 watermark。长会话下会增加不必要的 DB I/O 和内存开销。

**建议**：新增 `Store.Count()` 方法，SQLite 实现用 `SELECT COUNT(*)`。

---

## 16. 🟢 Round 5 发现（Low）

### 16.1 `int64`→`int` 隐式截断在 32-bit 平台

**文件**：`agent/runtime/process_budget.go:42,53`

```go
b.ownTokens += int(inv.PromptTokens + inv.CompletionTokens)  // line 42
b.ownTokens += int(inv.InputTokens)                           // line 53
```

`int64` token 计数转为 `int`。在 64-bit 平台（lynx 唯一目标）`int`=64-bit，安全。在 32-bit 平台上单次 LLM 调用的 token 数不可能超过 2^31，但累积预算可能溢出。

**影响**：可忽略。lynx 在 macOS/Linux amd64 上运行。

### 16.2 `turnBudget.exceeded` 将 `int` tokens 转回 `int64` 比较

**文件**：`lyra/internal/kernel/usage.go:59`

```go
return (b.MaxTokens > 0 && int64(tokens) >= b.MaxTokens)
```

`tokens` 来自 `pc.Process.Usage()` 返回 `int`，再转回 `int64`。多一次类型转换，无精度损失。低优先级清理项。

### 16.3 `stallContext` 的 `keepAlive` 使用 `Reset` 而非 `Stop`+`Reset`

**文件**：`lyra/internal/kernel/turn.go:37-38`

```go
t := time.AfterFunc(idle, cancel)
return ctx, func() { t.Reset(idle) }, func() { t.Stop(); cancel() }
```

Go 文档建议 `Reset` 前先 `Stop`（避免竞态）。但 `time.AfterFunc` 的 `Reset` 文档明确 "Reset changes the timer to expire after duration d... If the timer had been stopped, it is restarted." — `Reset` 对 `AfterFunc` timer 是幂等的。此用法正确。

---

## 17. Round 5 确认正确的领域 ✅

### Domain Service 审查（首批通读）

| Service | 文件 | 关键发现 |
|---|---|---|
| `conversation.Service` | `conversation.go` | `Truncate` 用 `Replace` 原子替换（非 DELETE+INSERT 分步），幂等正确 |
| `transcript.BoundaryAt` | `transcript.go` | rollback/fork 边界计算：`SortStableFunc` + `IsRoot()` 门控，`CreatedAt` 排序幂等 |
| `interrupts.Store` | `service.go` | `Consume` 定义 read-and-remove 原子语义，防止重复 resume |
| `approval.Service` | `service.go` | Mode 枚举 4 个常量，零值 `ModeSafe`（预设安全），`Remember` 支持 per-session 决策缓存 |

### SQLite 实现审查

| Store | 文件 | 关键发现 |
|---|---|---|
| `ProcessStore.Save` | `process.go:41-46` | `ON CONFLICT ... DO UPDATE` upsert 语义，参数化查询，无 SQL 注入 |
| `MessageStore.Replace` | `message.go:111-145` | 单事务 `DELETE`+`INSERT`，回滚时保留旧数据 |
| `InterruptStore.Consume` | `interrupt.go:109-122` | `DELETE ... RETURNING` 原子 read-and-remove，防止并发 resume 双消费 |
| `MessageStore.Read` | `message.go:57-61` | 损坏行跳过（`continue`）而非整个读失败，文档化取舍 |

### 整数安全性

| 转换点 | 类型 | 评估 |
|---|---|---|
| `process_budget:42` | `int64`→`int` | 64-bit 安全 |
| `turnBudget.exceeded:59` | `int`→`int64` | 无损失 |
| `process_budget:42` | `PromptTokens+CompletionTokens` | `int64` 加法，上限 ~1e18，LLM 单次调用 < 1e7 |
| `turnState.seq` | `atomic.Uint64` | 2^64 事件 ≈ 永不溢出 |

### 测试质量

- 测试全部用 `t.Fatal` 而非 `t.Error`+fallthrough（正确的 fail-fast 策略）
- stub 实现（`stubChatProcess`、`stubEngine`）完整覆盖 `ChatProcess` 接口，包括 `Discard` atomic 标记验证终端清理
- 所有 `errors.Is`/`errors.As` 断言直接对 sentinel，无字符串匹配脆弱性

---

## 18. 并发数据结构完整审查结果 ✅

第二轮对所有生产代码中的 `map[string]` struct 字段进行了锁保护一致性审查：

| 结构 | 文件 | 锁机制 | 结果 |
|---|---|---|---|
| `inMemoryBlackboard.named/protected/conditions/objects` | `agent/runtime/in_memory_blackboard.go` | `sync.RWMutex`，读 RLock / 写 Lock | ✅ |
| `agentRegistry.agents` | `agent/runtime/registries.go:21` | `sync.RWMutex` | ✅ |
| `processRegistry.procs` | `agent/runtime/registries.go:74` | `sync.RWMutex` | ✅ |
| `processState.excludedActions` | `agent/runtime/process_state.go:27` | 共用 `processState.mu` | ✅ |
| `inMemory.turns` | `lyra/internal/kernel/turn/inmemory.go:67` | `sync.Mutex` | ✅ |
| `turnState` cross-goroutine fields | `lyra/internal/kernel/turn/turn.go:68-87` | 文档化 `mu` 守卫 | ✅ |
| `exec.Manager.shells` | `lyra/internal/infra/exec/exec.go:39` | `sync.Mutex` | ✅ |
| `Manager.clients` | `lyra/internal/infra/lsp/manager.go:40` | `sync.Mutex` + Close 闭锁模式 | ✅ |
| `Tracker.seen` | `lyra/internal/domain/editguard/editguard.go:31` | `sync.Mutex` | ✅ |
| `pathLocker.locks` | `lyra/internal/kernel/toolset/editguard.go:191` | `sync.Mutex` + ref-counted per-path lock | ✅ |
| `Server.runs` | `lyra/internal/delivery/server/pump.go` | `runMu` (sync.Mutex) | ✅ |
| `runHub.subs` | `lyra/internal/delivery/server/hub.go:35` | `sync.Mutex` | ✅ |

**结论**：所有并发 map 访问均正确加锁，无 data race 风险。

---

## 8. 良好实践确认

以下模式被确认是正确的防御性设计：

| 位置 | 实践 |
|---|---|
| `agent/runtime/process_state.go` | `makeRunning()` CAS 防止并发 run loop |
| `agent/runtime/process_state.go` | `markKilled()` CAS 防止杀死已终态进程 |
| `agent/core/process_context.go:ExecuteSafely` | panic recovery 包裹 action 执行 |
| `agent/runtime/blackboard_determiner.go:safeEvaluateCondition` | panic recovery 包裹用户 Condition |
| `lyra/internal/kernel/turn/lifecycle.go` | `setRoot` + 门控防止子进程终端事件覆盖根进程 |
| `lyra/internal/kernel/engine.go:Close` | `sync.Once` 保证 closer 幂等 |
| `lyra/internal/kernel/turn/inmemory.go:emit` | 双 select 设计处理 ctx cancel 与终端事件的交付竞态 |
| `lyra/internal/delivery/server/hub.go` | `cancel` func 在 hub 已关闭时不重复 close channel |
| `lyra/internal/delivery/server/filewatch.go` | `sync.Once` + done/exited channel 确保 watcher 安全关闭 |
| `lyra/internal/kernel/toolset/build.go:117-119` | A2A dial 失败时关闭已打开的 MCP 连接，防止泄漏 |

---

## 20. 修复优先级建议（全五轮汇总）

| 优先级 | Round | Bug | 修复复杂度 |
|---|---|---|---|
| 🔴 P0 | R1 | #1 Cancel/handleWaiting race → panic | 1 行 (`st.claimPark()`) |
| 🟠 P1 | R1 | #2.1-#2.3 goroutine panic recovery（runTurn/drive/pumpRun） | 每处 ~10 行 |
| 🟡 P2 | R1 | #3.1-#3.2 goroutine panic recovery | 每处 ~10 行 |
| 🟡 P2 | R1 | #3.3 emit 理论竞态 | `sync.Once` |
| 🟡 P2 | R1 | #3.4 emitTurnEnd 忽略 Output error | 检查 error |
| 🟡 P2 | R2 | #5.1 persistItem/persistRun 静默数据丢失 | metric counter |
| 🟡 P2 | R2 | #5.3 ctx.Background() 断 trace | `WithoutCancel` |
| 🟡 P2 | R3 | #7.1 BudgetPolicy float64 歧义 | `*float64` |
| 🟡 P2 | R3 | #7.2 wrapTool 吞没错误 | nil guard |
| 🟡 P2 | R3 | #7.3 Interrupt 类型覆盖 | warning log |
| 🟡 P2 | R4 | #11.1 spawn orphan ref | `removeChild` |
| 🟡 P2 | R4 | #11.2 maybeAutoSnapshot 同步阻塞 | goroutine 异步 |
| 🟡 P2 | R5 | #15.1 bash select 竞态丢结果 | 先查 `sh.Done()` |
| 🟡 P2 | R5 | #15.2 Count 全量读取 | `SELECT COUNT(*)` |
| 🟢 P3 | R1 | #4.1-#4.2 文档化限制 | 文档 / nil guard |
| 🟢 P3 | R2 | #6.1-#6.2 性能/诊断 | RLock / log |
| 🟢 P3 | R2 | #5.2/#5.4 文档化 | 文档更新 |
| 🟢 P3 | R3 | #8.1-#8.3 文档化 | 无需修复 |
| 🟢 P3 | R4 | #11.3/#12.1/#12.2 低影响 | 注释/文档 |
| 🟢 P3 | R5 | #16.1-#16.3 平台假设/清理 | 无需修复 |
