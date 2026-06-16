# BUGS — lyra + agent 深度排查记录

> 2026-06-16 手动逐文件审计。只记录，不修。

---

## 🔴 严重

### 1. `Cancel` 导致 TurnEnd 事件被静默丢弃

**文件**: `lyra/internal/kernel/turn/turn.go:369-379` + `inmemory.go:212-229`

**根因**: `emit()` 使用 `select { case st.events <- ...; case <-st.ctx.Done(): }` 做背压逃生阀。`Cancel` 先调 `state.cancel()` 取消 `st.ctx`，之后所有 `emit` 调用全部命中 `st.ctx.Done()` 分支，事件被静默丢弃。

**关键路径**:

```
Cancel → state.cancel()          // st.ctx.Done() 立即可读
       → claimed=true
       → finishTurn(state, ...)  // 内部 s.emit(st, TurnEnd{...})
                                  //   → select 命中 ctx.Done() → TurnEnd 被 drop
                                  // endTurn() 仍执行 → channel 关闭
```

非 parked 路径同理：`drive()` → `emitTurnEnd()` → `emit()` 也会丢弃 TurnEnd。

**症状**: consumer 收到 channel close 但无 TurnEnd 事件。in-process consumer（CLI、测试）不可靠收到 cancelled turn 的终态。

**修复方向**: `emit` 的 escape-hatch 不应在发送 terminal 事件时生效。可考虑：
- emit 分两态：`emitLossy`（可丢，用于流式中继）和 `emitTerminal`（不可丢，用于 TurnEnd/ErrorEvent）
- 或者 `finishTurn` / `emitTurnEnd` 在 emit 前做一次 `select { case <-st.ctx.Done(): default: }` 打桩
- 或者 terminal 事件绕过 select 直接写入（`st.events <- stamped`），接受阻塞风险

---

### 2. `Cancel` 和 `drive()` 之间的终结竞态

**文件**: `lyra/internal/kernel/turn/inmemory.go:212-229` + `turn.go:198-217`

**场景**: agent process 自然结束 → Done channel 关闭 → `Cancel` 此时到达：

```
drive goroutine 阻塞在 <-doneCh   ← 已就绪但未调度
Cancel goroutine:
  state.cancel()                  // ctx 取消
  proc = state.process()          // 拿到已结束的 process
  claimed = state.claimPark()     // false（未 parked）
  proc.Cancel()                   // 对已终止 process 调用 KillProcess
  claimed==false → 不调 finishTurn ← 不终结

调度器切回 drive goroutine:
  <-doneCh → runErr 就绪
  emitTurnEnd → s.emit(st, TurnEnd) → 被 ctx.Done() 分支丢弃！(Bug 1)
  endTurn()                        // 关闭 channel
```

当前**碰巧安全**：`proc.Cancel()` 对已终止 process 的 KillProcess 被 agent runtime 忽略。但如果未来 `proc.Cancel()` 对已终止 process 返回 error 并被处理，可能引入重复终结或 panic。

**修复方向**: `Cancel` 中 `proc.Cancel()` 和后续终结之间，加 process 状态检查；或者统一通过 `drive` 的 doneCh 路径终结。

---

## 🟡 中等

### 3. 残留的重构碎片注释

**文件**: `agent/runtime/execute_action.go:130-133`

```go
// On a retry (any attempt after the first), clear this action's
// declared effect conditions so a half-applied effect from the
// failed attempt doesn't poison the next one. On retry, clear the
// false) } when retryCount > 0). The hasRun key is only promoted on
// success after the loop, so clearing it here is a harmless no-op.
```

第 132 行 `// false) } when retryCount > 0).` 是某次重构的源代码片段混入了注释。`"false) }"` 和 `"retryCount > 0"` 是 Go 语法残留，不在当前代码中。

**症状**: 注释语法不通、信息失真，对读者无意义。

**修复方向**: 改回通顺的注释，或直接删除（上下文已自解释）。

---

### 4. `handleWaiting` 中 nil awaitable 引发空中断

**文件**: `lyra/internal/kernel/turn/turn.go:226-253`

```go
func (s *inMemory) handleWaiting(st *turnState, proc kernel.ChatProcess) {
    aw := proc.PendingAwaitable()
    if aw == nil || s.canSurface(interruptKind(aw)) {
        s.emitInterrupt(st, proc)   // ← aw==nil 走这里
        return
    }
    ...
}
```

`emitInterrupt` 中 `aw == nil` 分支：
```go
st.park()                                  // parked=true
s.emit(st, TurnInterrupted{})              // Interrupts 为空!
```

**症状**: 
- turn 标记为 parked，surface 空 `TurnInterrupted`（`Interrupts: []`）
- 客户端收到无 interrupt 的中断事件，不知道如何 resume
- 如果没有外部 intervention，turn 永远 parked

代码中已有防御注释 `// Defensive: Waiting without a parked awaitable shouldn't happen`，但处理方式不完整。

**修复方向**: `aw == nil` 时不应 park——走 deny / fail 路径结束 turn，而不是 surface 一个无法回答的空中断。

---

### 5. `checkEarlyTermination` 不记录 Failure

**文件**: `agent/runtime/run.go:119-136`

```go
func (p *AgentProcess) checkEarlyTermination() bool {
    ...
    p.state.setStatus(core.StatusTerminated)   // 只设 status
    p.publishEvent(event.ProcessTerminated{
        Reason: reason,                         // reason 在 event 中有
    })
    return true
}
```

对比其他终止路径：

| 路径 | 设置 status | 记录 Failure | publish Event |
|------|------------|-------------|---------------|
| `markCancelled` | StatusKilled | ✅ setFailure | ProcessKilled |
| `failProcess` | StatusFailed | ✅ setFailure | ❌ (由 publishTerminalEvent) |
| `completeForGoal` | StatusCompleted | ❌ | GoalAchieved |
| `checkEarlyTermination` | StatusTerminated | ❌ | ProcessTerminated |
| `handleStuck` | StatusStuck | ❌ | ProcessStuck |

`StatusTerminated` 和 `StatusStuck` 都不设 Failure。如果调用方通过 `Failure() != nil` 判断出错，会漏掉这两个状态。`StatusCompleted` 不设 Failure 是正确的——完成不是失败。

**风险**: lyra 的 `turnLifecycle` 通过 `event.ProcessTerminated` 映射 `TurnEndCanceled`（见 `planTurnEnd`），不依赖 `Failure()`。但如果有其他 consumer 走 `Failure()` 路径，会行为不一致。

**修复方向**: 取决于 `StatusTerminated` 的语义——如果它是"正常终止"（类似 Completed），则当前行为正确；如果是"异常终止"，应设 Failure。需要确认设计意图。

---

## 🟢 确认无问题（原怀疑但验证安全）

| 怀疑 | 验证结论 |
|------|---------|
| `Cancel` 和 `Resume` 对 `claimPark` 的竞态 | `claimPark` 是 mutex-guarded CAS，互斥正确 |
| `pumpRun` defer 中 `go snapshotCheckpoint` 的 ctx 泄漏 | `context.WithoutCancel` 正确解耦 |
| `emit` 的 seq 并发安全 | `atomic.Uint64` |
| `turnLifecycle.listener` 的 event 竞态 | mutex-guarded earliest-wins |
| `runActionsInParallel` 的 slice 并发写入 | 每个 goroutine 写独立 index，WaitGroup 同步 |
| `cancelReasonFor` 的 runMu 竞态 | 读写均在 runMu 下，正确 |
| `recordActionFailure` 的 double setFailure | 有 nil guard，不会覆盖首次 failure |
| `stamp` 方法的值拷贝并发安全性 | 值类型，每个 goroutine 独立副本 |

---

## 🟡 第二轮排查新增 (2026-06-16)

### 6. `runHub.Append` 持久事件无界增长

**文件**: `lyra/internal/delivery/server/hub.go:48-63`

```go
func (h *runHub) Append(ev protocol.RunEvent) {
    ...
    if ev.Event.IsDurable() {
        h.durable = append(h.durable, ev)  // ← 无上限
    }
    ...
}
```

**问题**: `h.durable` 保存所有 `item.started` 和 `item.completed` 事件用于崩溃恢复重放。一个长 coding session（几十个 turn、每个 turn 数十个 tool call）会累积数千个事件。内存随运行时长线性增长，无 GC 路径。

**修复方向**: 设耐久事件上限（如最近 10,000 个），超出时截断最旧的事件，同时记录截断点使 `runs.subscribe` 知道无法完全重放。

---

### 7. `runHub.Subscribe` 重放期间持锁阻塞其他操作

**文件**: `lyra/internal/delivery/server/hub.go:86-118`

```go
func (h *runHub) Subscribe(fromEventID string) (<-chan protocol.RunEvent, func()) {
    h.mu.Lock()
    defer h.mu.Unlock()
    ...
    ch := make(chan protocol.RunEvent, len(replay)+liveHeadroom)
    for _, ev := range replay {   // ← 持锁同步写入 channel
        ch <- ev
    }
    ...
}
```

**问题**: 重放事件逐个写入 buffered channel 时持有 `h.mu`。如果重放列表很长（见 bug 6），此期间 `Append`（由 pump goroutine 调用）和新的 `Subscribe` 全部被阻塞。pump 被阻塞会导致 run 暂停——短暂但可感知的延迟。

**修复方向**: 先 copy replay，释放锁，再写入 channel；或者用 `Append` 之外的队列。

---

### 8. `appendText` / `appendReasoning` 的 delta index 始终为 0

**文件**: `lyra/internal/delivery/server/translator_items.go:26,30`

```go
func (t *translator) appendText(text string) []protocol.StreamEvent {
    ...
    t.text.buf.WriteString(text)
    idx := 0                                     // ← 始终为 0，不递增
    return append(out, protocol.StreamEvent{
        ...
        Delta: &protocol.ItemDelta{
            Type: protocol.DeltaContent,
            Index: &idx,                         // ← 每次都是 &0
            Text:  text,
        },
    })
}
```

**问题**: 所有 `item.delta` 的 `Index` 字段始终为 0。如果客户端依赖 `Index` 做片段排序——例如在丢包重传场景——所有 delta 将被视为同一位置，覆盖而非拼接。

**症状**: 当前客户端按到达顺序拼接（不依赖 Index），暂无实际问题。但如果未来客户端做 Index-based 重组，会出现内容被截断。

---

### 9. `approvalInterrupt` / `askUserInterrupt` 未检查类型断言

**文件**: `lyra/internal/delivery/server/translator_interrupt.go:68,107`

```go
func (t *translator) approvalInterrupt(in turn.Interrupt) (...) {
    p, _ := in.Payload.(turn.ApprovalPrompt)  // ← ok 丢弃
    ...
}
func (t *translator) askUserInterrupt(in turn.Interrupt) (...) {
    q, _ := in.Payload.(interrupts.QuestionPrompt)  // ← ok 丢弃
    ...
}
```

**问题**: 如果 `in.Kind == "approval"` 但 `in.Payload` 不是 `ApprovalPrompt`——程序性错误——类型断言静默失败，返回零值 struct。客户端收到 ID 为空、无 tool name/args 的中断项。

**修复方向**: 检查 ok，失败时 emit error 事件 + fallback（如返回 `InternalErrorProblem` 的错误 run.finished）。

---

### 10. `ApproveToolCall` 中 `Remember` 错误被静默吞掉

**文件**: `lyra/internal/kernel/turn/observer.go:87-88`

```go
if res.Remember {
    _ = t.svc.approval.Remember(ctx, sessionID, toolName, res.Approved)  // ← 吞错
}
```

**问题**: 用户勾选 "remember this decision"，但 `approval.Remember` 失败（如存储后端不可用），错误被丢弃。用户的 "记住" 意图**静默丢失**——下次同一工具调用依然会弹出审批提示，违背用户体验预期。

---

### 11. `Engine.Close` 非 goroutine 安全

**文件**: `lyra/internal/kernel/engine.go:212-221`

```go
func (e *Engine) Close() error {
    var errs []error
    for _, closeFn := range e.closers { ... }
    e.closers = nil  // ← 竞态：两个 goroutine 可同时读 e.closers
    return errors.Join(errs...)
}
```

**问题**: 两个 goroutine 同时调用 `Close()`：它们都读到 `e.closers`（未置 nil），**每个 closer 被执行两次**。如果某个 closer 不是幂等的（如关闭一个已关闭的 MCP session），可能 panic。

**修复方向**: 用 `sync.Once` 或 `atomic.CompareAndSwap` 守卫。

---

### 12. `emit` 在 ctx 取消后写入已关闭 channel 的可能

**文件**: `lyra/internal/kernel/turn/turn.go:354-379`

这是 Bug 1（TurnEnd 丢弃）的对偶问题：如果 consumer 已离开（ctx not cancelled），但 `endTurn` 已调用 `close(st.events)`，后续 `emit` 的 `st.events <- stamped` 会 **panic（send on closed channel）**。

当前保护：`endTurn` 在 `close` 之前 delete 了 turns map，且 `emit` 只在持有 turn 引用的 goroutine（`runTurn`/`drive`/`finishTurn`）中调用——这些 goroutine 在 `endTurn` 之后不再 emit。所以**当前安全**。但任何未来在 `endTurn` 之后 emit 的代码都会 panic。这是一个脆性不变量。

### 13. `pumpRun` defer 中 `snapshotCheckpoint` 无超时

**文件**: `lyra/internal/delivery/server/pump.go:151-153`

```go
if !parked {
    go s.snapshotCheckpoint(context.WithoutCancel(ctx), handle.SessionID, runID)
}
```

**问题**: `snapshotCheckpoint` 使用 `context.WithoutCancel(ctx)`，不受 runCtx 取消影响。内部是 git 操作，可能在慢文件系统上长时间阻塞。goroutine 最终会完成（git 操作总会超时或结束），但中间无法取消。极端情况（NFS hang）下 goroutine 永久泄漏。

**修复方向**: 传带 timeout 的 context 或使用 `context.Background()` + deadline。

---

## 🔴 第三轮排查——时序专项 (2026-06-16)

### 14. Cancel 在 Resume 赢得 claimPark 后仍 KillProcess —— 用户审批被静默丢弃

**文件**: `lyra/internal/kernel/turn/inmemory.go:212-229` + `agent/runtime/platform_run.go:201-212`

**时序**:

```
Resume goroutine                     Cancel goroutine
─────────────────                    ─────────────────
findTurn → found
claimPark → wins, parked=false
proc.Resume()
  deliverResponse → blackboard 已更新
  ContinueProcessAsync:
    makeRunning → Waiting→Running
    run loop starts                  
                                     findTurn → found
                                     state.cancel() → ctx done
                                     claimPark → FALSE (lost)
                                     proc.Cancel() → KillProcess!
                                       setStatus(Killed)     ← 覆盖 Running!
                                       publish(ProcessKilled)
run loop: ctx.Err() → markCancelled
         setStatus(Killed)           ← 重复
         publish(ProcessKilled)      ← 重复事件!
         exit
```

**问题**: `Cancel` 不管 `claimPark` 结果如何都调用 `proc.Cancel()`。当 Resume 赢得竞态后，`KillProcess` 把 status 从 `Running` 翻成 `Killed`，导致 `ContinueProcessAsync` 中的 run loop 在第一次迭代就退出——**已批准的 tool 永远不会执行**。用户批准被静默丢弃，turn 以 Canceled 结束。

**同时触发双重 ProcessKilled 事件**：`KillProcess` 和 `markCancelled` 各自发布一次 ProcessKilled。lifecycle listener 只取第一个（earliest-wins），所以不影响 TurnEnd 判定，但事件总线收到两个重复事件。

**修复方向**: `Cancel` 中 `proc.Cancel()` 仅在 `claimed == true` 时调用；或者进程无 parked 状态时才 kill。

---

### 15. `KillProcess` 的 ProcessKilled 事件在进程真正终止前发布

**文件**: `agent/runtime/platform_run.go:201-212`

```go
func (p *Platform) KillProcess(id string) error {
    proc.state.setStatus(core.StatusKilled)    // ← 立即生效
    p.publish(event.ProcessKilled{...})        // ← 立即发布
    return nil
}
```

**问题**: `KillProcess` 不等待进程停止——它只设置 status 然后立即返回。此时进程可能**正在执行一个 action**（tool call、LLM 请求），尚未退出。事件消费者（如 lyra 的 lifecycle listener）收到 ProcessKilled 后判定 turn 结束，但进程可能还要执行数秒才真正终止。

**症状**:
- ProcessKilled 事件和后续 turn 清理之间有时间窗口
- 进程的 Done channel 在 ProcessKilled 之后才关闭
- 如果消费者在 ProcessKilled 之后、进程实际停止之前发起新操作，会与未终止的进程冲突

**对比**: `markCancelled` 路径（ctx 取消 → run loop 退出）是同步的——ctx 取消先触发，run loop 在下一次迭代检查 `ctx.Err()` 后退出，**然后** `markCancelled` 发布事件。所以只有 `KillProcess` 路径有此问题。

---

### 16. `Cancel` 双路径导致重复 ProcessKilled 事件

**文件**: `lyra/internal/kernel/turn/inmemory.go:217-223` + `agent/runtime/run.go:104-112`

```go
// Cancel
state.cancel()           // → ctx cancel → markCancelled → publish ProcessKilled (路径 A)
proc.Cancel()            // → KillProcess → publish ProcessKilled (路径 B)
```

**问题**: `Cancel` 通过两个独立机制终止进程：(A) ctx cancel 触发 run loop 的 `markCancelled`；(B) `KillProcess` 直接设置 status。两条路径都发布 `ProcessKilled` 事件。无去重保护。

**症状**: 事件总线在同一个 turn 收到两份 ProcessKilled。lifecycle listener 取 earliest-wins 所以不影响 TurnEnd 判定，但其他 listener（metrics、audit、logging）可能计数翻倍。

---

### 17. `makeRunning` CAS 不防御 StatusKilled 重入

**文件**: `agent/runtime/process_state.go:133-145`

```go
func (s *processState) makeRunning() bool {
    s.mu.Lock()
    defer s.mu.Unlock()
    switch s.status {
    case core.StatusRunning,
        core.StatusCompleted, core.StatusFailed, core.StatusStuck,
        core.StatusKilled, core.StatusTerminated:
        return false           // ← Killed 拒绝 makeRunning
    }
    s.status = core.StatusRunning
    return true
}
```

**与 Bug 14 的交互**: 在 Bug 14 的时序中，`ContinueProcessAsync` 调用 `makeRunning` 时 status 仍是 `Waiting`（ResumeProcess 不改变 status），所以 `makeRunning` 成功，status → `Running`。然后 `KillProcess` 覆盖为 `Killed`。所以 `makeRunning` 的 CAS 本身是正确的——问题在于 `KillProcess` 破坏了它的不变式。

**独立问题**: 如果 status 被外部设为 `Killed`（如 `KillProcess`），然后 `ContinueProcess` 被调用，`makeRunning` 返回 false，run loop 不启动。这是预期行为（死进程不应重启）。但如果调用方期望 `ContinueProcess` 一定能恢复进程，会吃惊。

---

### 18. `state.cancel()` 和 `proc.Cancel()` 的调用顺序导致 ctx 无效化

**文件**: `lyra/internal/kernel/turn/inmemory.go:217`

```go
state.cancel()    // ← 先取消 ctx
proc := state.process()
claimed := state.claimPark()
if proc != nil {
    _ = proc.Cancel()    // ← 后 kill 进程
}
```

**问题**: `state.cancel()` 取消 turn ctx → run loop 中任何检查 `ctx.Done()` 的代码都立即可读。但 `proc.Cancel()` 在之后才调用。在这之间，run loop 可能因 `ctx.Err()` 退出并调用 `markCancelled` → `setStatus(Killed)`。然后 `proc.Cancel()` → `KillProcess` 再设置一次 `Killed`（已是 Killed，但事件重复发布）。

**修复方向**: 先 `proc.Cancel()`，再 `state.cancel()`，确保进程先收到 kill 信号，ctx 取消作为兜底。

---

## 📊 总汇 (三轮排查)

| 级别 | 数量 | 类别 |
|------|------|------|
| 🔴 严重 | 3 | TurnEnd 丢弃 (#1)、Cancel/Resume 竞态 (#14)、KillProcess 提前发布 (#15) |
| 🟡 中等 | 14 | 重构碎片 (#3)、空中断 (#4)、Terminated 无 Failure (#5)、hub 无界 (#6)、Subscribe 持锁 (#7)、delta=0 (#8)、断言未检 (#9)、Remember 吞错 (#10)、Close 竞态 (#11)、脆性 send-on-closed (#12)、snapshot 无超时 (#13)、双路径重复事件 (#16)、makeRunning 边界 (#17)、cancel/kill 顺序 (#18) |
| 🟢 安全 | 8 | 竞态/并发/ctx/channel/race 已验证 |

---

## 🔴 第四轮排查——恢复/持久层深度 (2026-06-16)

### 19. `Interrupts.Delete` 失败可致工具重复执行

**文件**: `lyra/internal/delivery/server/runs.go:127-148` + `turn/inmemory.go:310-352`

**时序**:

```
第一次 resumeRun                       第二次 resumeRun (同 parentRunID)
──────────────────                     ──────────────────
Interrupts.Get → 成功                  Interrupts.Get → 成功 (Delete 失败，记录仍在)
Chat.Resume(handle, res) → 成功        Chat.Resume(handle, res) → ErrTurnNotFound (turn 已结束)
  进程 unpark → tool 执行 → 完成        rehydrate(pending, approved):
Interrupts.Delete → 失败(存储错误)!      RestoreChat(snapshot) → 恢复 parked 状态
                                         resumeAndDrive → tool 再次执行!
```

**问题**: `ResumeRun` 中 `Interrupts().Delete()` 的错误被静默吞掉（`_ =`）。如果 Delete 失败（存储后端瞬断），interrupt 记录残留。下次 `resumeRun` 找到记录，调用 `Resume` → 返回 `ErrTurnNotFound`（turn 已结束）→ 尝试 rehydrate。

rehydrate 从 ProcessStore 恢复 snapshot。如果 auto-snapshot 尚未覆盖 parked snapshot（时间窗口：第一次 Resume 到 tick 完成 auto-snapshot 之间），恢复的是 parked 状态→ `resumeAndDrive` → **同一个 tool 被执行第二次**。

**影响**: 对非幂等 tool（`bash rm -rf`、`write`、`edit`）产生数据损坏或副作用重复。

**修复方向**: 
- Delete 失败不吞错误，至少记录诊断日志
- rehydrate 前检查 snapshot 状态是否为 Waiting（只有 Waiting 的 snapshot 才能 resume）
- 或者 Delete 失败时 mark interrupt 为 "resolved"（不再被 Get 返回）

---

### 20. `restorePark` 的 `Clear` 失败导致 ParkStore 永久污染

**文件**: `core/model/chat/middleware/tool/middleware.go:410-423`

```go
func (m *middleware) restorePark(ctx context.Context, req *chat.Request) (*chat.Request, error) {
    ...
    if state, _ := m.parkStore.Read(ctx, parkID); state != nil {
        req = injectParkTail(ctx, req, state)
        _ = m.parkStore.Clear(ctx, parkID)   // ← Clear 失败被吞掉!
    }
    return req, nil
}
```

**问题**: `Clear` 失败时，park 状态永久残留在 ParkStore 中。后续每个 turn 的 `restorePark` 都会读到相同的 stale park state，重复执行 `injectParkTail` → `parseResumePoint` 流程。如果 tail 注入成功且 resume point 被识别，turn 会错误地进入 resume 路径（跳过模型调用，执行旧的 pending tool calls），即使这是全新的用户消息。

**症状**: 每次新 turn 都试图恢复一个早已结束的 tool 调用。结果取决于 snapshot 是否仍可用——最终会收到错误并 fallback 到正常路径，但每个 turn 多一次无效的 restore 尝试。

**修复方向**: Clear 失败至少记录错误，或改用 write-through 语义（写入 "已消费" 标记而非删除）。

---

### 21. `KillProcess` 绕过 `processSignals.terminate` 通道，直接修改 status

**文件**: `agent/runtime/platform_run.go:201-212`

```go
func (p *Platform) KillProcess(id string) error {
    proc.state.setStatus(core.StatusKilled)   // ← 绕过 signal 通道
    p.publish(event.ProcessKilled{...})
    return nil
}
```

**问题**: 进程有两种终止机制：(A) 通过 `processSignals.queueTermination` → `drainTerminate` → `handleTerminationSignal`（优雅终止），和 (B) 通过 `KillProcess` 直接设置 status（硬杀）。

机制 B 绕过信号通道——如果 `queueTermination` 中已有信号排队，`KillProcess` **不同步消费**它。下次 tick 时 `drainTerminate` 会取出排队信号并处理，而此时 status 已是 Killed。`handleTerminationSignal` 会再次 `setStatus(Terminated)`（覆盖 Killed！）并发布 `ProcessTerminated` 事件。结果是 process 收到 ProcessKilled + ProcessTerminated 两个事件。

机制 A 和 B 未被设计为互斥——它们可以同时激活。优雅终止排队的信号能与硬杀在同一 tick 中竞争。

**修复方向**: KillProcess 消费/清空排队的 terminate 信号；或 KillProcess 通过 queueTermination 而非直接 setStatus。

---

### 22. `invokeToolCalls` 中断轮次时未设 `request`/`response`

**文件**: `core/model/chat/middleware/tool/invoker.go:311-313,394-401`

```go
// invokeToolCalls 中
return &invocationResult{
    interrupt: &roundInterrupt{done: returns, cause: err},
}, nil   // ← request/response 未设

// invoke 中
result.request = req    // ← 统一设置
result.response = resp
if result.interrupt != nil {
    return result, nil  // ← validate() 跳过
}
```

当前代码安全——`invoke` 在 `invokeToolCalls` 返回后统一设置 `request`/`response`，中断分支也在之后才检查。但 `invokeToolCalls` 内部返回的 `invocationResult` 缺少 `request`/`response`，如果有任何代码路径在 `invoke` 之外直接调用 `invokeToolCalls` 并访问这些字段，会拿到零值。当前无此路径，但这是一个脆性不变量。

---

### 23. `pumpRun` 中 `emit` 闭包的 `runID` 字段在闭包外修改

**文件**: `lyra/internal/delivery/server/pump.go:78-114`

```go
emit := func(events []protocol.StreamEvent) {
    for _, se := range events {
        re := protocol.RunEvent{
            RunID: runID,    // ← 闭包捕获外部变量 runID
            ...
        }
```

`runID` 是 `pumpRun` 的参数，不会在闭包生命周期内被修改。但 `se` 可能是共享的 `*protocol.StreamEvent`（如果 translator 返回指向同一内存的指针），`emit` 中将其字段读取并复制到 `RunEvent` 中。translator 返回的是值类型 `[]protocol.StreamEvent`（切片按值），每个元素是独立副本。所以安全。

但 `se.Event` 是一个 `protocol.StreamEvent` 值。如果 `StreamEvent` 包含指针字段（如 `Run *protocol.RunRef`, `Item *protocol.Item`, `Outcome *protocol.RunOutcome`），这些指针指向 translator 内部状态。`emit` 闭包捕获了这些指针，在闭包内访问它们。translator 在 `emit` 调用返回后可能修改这些指针指向的数据（比如下一个 `translate` 调用复用 `*protocol.Item`）。但这取决于 translator 的实现——如果 translator 每次都创建新的 `Item` 对象，就没有问题。

查看 translator 代码：`nextItemID()`, `open()`, `appendText()` 等都创建新的 `Item` 值并返回。所以没有复用问题。安全。

---

## 📊 总汇 (四轮排查)

| 级别 | 数量 | 类别 | 
|------|------|------|
| 🔴 严重 | 4 | TurnEnd 丢弃 (#1)、Cancel/Resume 竞态 (#14)、KillProcess 提前发布 (#15)、Interrupts.Delete 失败致重复执行 (#19) |
| 🟡 中等 | 18 | 重构碎片 (#3)、空中断 (#4)、Terminated 无 Failure (#5)、hub 无界 (#6)、Subscribe 持锁 (#7)、delta=0 (#8)、断言未检 (#9)、Remember 吞错 (#10)、Close 竞态 (#11)、脆性 send-on-closed (#12)、snapshot 无超时 (#13)、双路径重复事件 (#16)、makeRunning 边界 (#17)、cancel/kill 顺序 (#18)、ParkStore 污染 (#20)、KillProcess 绕过信号 (#21)、invokeToolCalls 脆性 (#22)、emit 闭包审计 (#23) |
| 🟢 安全 | 8 | 已验证 |

---

## 🔴 agent 模块专项排查 (2026-06-16)

### 19. GOAP A* `backwardOptimize` 可能错误删除必要 action

**文件**: `agent/planning/planner/goap/astar.go:377-432`

**根因**: `backwardOptimize` 从后往前遍历 plan，维护 `needed` 条件集。处理 action 时检查它是否满足 `needed`——不满足则丢弃。但 `needed` 是**惰性增长**的：action 被保留时才将其 preconditions 加入 `needed`。

**错误场景**: Plan `[A, B]`, Goal `{X}`, A 有 precondition `{Y}`, B 有 effect `{X}` 和 precondition `{Y}`：
1. 后向遍历 B: `needed={X}`, B 满足 → 保留 B, `needed={Y}`
2. 后向遍历 A: `needed={Y}`, A 无 effects → **丢弃 A**
3. 结果 plan `[B]`——B 的 precondition Y 无人满足

**影响**: 特定依赖链场景下返回不可执行 plan。

### 20. `StatusPaused` 被 lyra 误判为 Completed

**文件**: `agent/runtime/run.go:141-154` + `lyra/internal/kernel/turn/turn.go:354-398`

`publishTerminalEvent` 只处理 Completed/Failed。Paused 退出时不发事件。lyra 的 `planTurnEnd` 在无 terminal 事件时走 `fallbackPlan`→`completedPlan`→`TurnEndCompleted`。

**结果**: Action 返回 `ActionPaused`→process `StatusPaused`→无 Paused 事件→lyra 判为 Completed。Paused 语义完全丢失。

### 21. `prepareChild` 中 `linkChildSession` 失败致子进程泄漏

**文件**: `agent/runtime/child.go:255-261`

`CreateChildProcess` 注册子进程后，`linkChildSession` 失败时 `prepareChild` 返回 error **未清理已注册子进程**。子进程永留 registry（`StatusNotStarted` 非 terminal，不会被 Prune）。

### 22. `SpawnChildAsync` 子进程无人清理

**文件**: `agent/runtime/child.go:153-164` + `lyra/internal/kernel/chatprocess.go:97-103`

lyra 的 `Discard()` 只 `RemoveProcess` 自身，不调 `KillChildren`。async spawn 的子进程在父进程终止后继续运行。

### 23. `inMemoryBlackboard.Spawn` 浅拷贝致父子共享可变对象

**文件**: `agent/runtime/in_memory_blackboard.go:210-220`

`maps.Copy` 浅拷贝 named map——指针值被父子 blackboard 共享。子进程修改共享对象时父进程可见。当前 lyra 的 protected entries 为 string（不可变），暂安全。未来放入可变 protected 对象则会数据竞争。

---

## 📊 总汇（全五轮）

| 级别 | 总数量 | 代表项 |
|------|--------|--------|
| 🔴 严重 | 6 | #1 TurnEnd 丢弃, #14 Cancel/Resume 竞态, #15 KillProcess 提前, #19 Interrupts.Delete 重复执行, #24 backwardOptimize, #25 Paused→Completed |
| 🟡 中等 | 21 | 其余 |
| 🟢 安全 | 9 | 已验证 |

---

## 🔴 第六轮排查——内存/边界/深层时序 (2026-06-16)

### 29. `blackboard.objects` / `hidden` 无界增长 — O(n) 扫描每 tick 一次

**文件**: `agent/runtime/in_memory_blackboard.go:57,107,128,189`

每次 `Set()` / `BindProtected()` 向 `objects` append，每次 `Hide()` 向 `hidden` append。两者永不修剪。每 tick 的 `Lookup("it", type)` 调用 `findLatestByType` 从后往前 O(n) 扫描 `objects`。每次扫描还对每个元素调用 `isHidden`——`reflect.DeepEqual` 比较 `hidden` 中每一项（O(n×m)）。

**影响**: 长运行 agent（数百 ticks）→ `objects` 数百项 → 每次 action 输入解析变慢。`hidden` 也累积使扫描更慢。进程重启后重置（in-memory），不持久。

---

### 30. `processState.history` 无界增长 — 每次 tick 追加一条

**文件**: `agent/runtime/process_state.go:104-108`

```go
func (s *processState) recordInvocation(inv ActionInvocation) {
    s.history = append(s.history, inv)
}
```

每次 tick 向 `history` append 一个 `ActionInvocation`（含 name、timestamp、duration、status）。永不修剪。`getHistory()` 每次全量 `slices.Clone`。

**影响**: 长运行 agent（数千 ticks）→ history 数千条 → 每次 `Usage()` 调用 clone 整个 slice。内存线性增长直到进程终止。

---

### 31. `emit` 中 `persistStreamEvent` 和 `recordInterrupt` 阻塞 pump goroutine

**文件**: `lyra/internal/delivery/server/pump.go:108,92`

```go
emit := func(events []protocol.StreamEvent) {
    for _, se := range events {
        ...
        s.persistStreamEvent(...)  // ← sqlite write，阻塞!
        s.emitToolFileChange(...)
        hub.Append(re)
    }
}
```

每个 stream event（包括每个 `item.delta`——流式 LLM 响应中每秒数十个）都**顺序**写入 sqlite 和推送到 hub。如果 sqlite 慢（写锁竞争、WAL 检查点），pump goroutine 被阻塞，继而：
- hub 收不到新事件 → 所有 SSE 订阅者暂停
- 下一个 `translate()` 调用也暂停（for-range 在 `inner` 迭代器中等待）

**影响**: LLM 流的实时性被持久化延迟绑定。一次慢 sqlite 写入延迟所有订阅者。

---

### 32. `context.AfterFunc(reqCtx, unsubscribe)` 过早关闭 subscriber channel

**文件**: `lyra/internal/delivery/server/pump.go:44-52`

```go
events, unsubscribe := hub.Subscribe("")
context.AfterFunc(reqCtx, unsubscribe)  // ← 如果 reqCtx 已 done，立即执行!
go s.pumpRun(runCtx, ...)
return &protocol.StartRunResponse{RunID: runID}, events, nil
```

`AfterFunc` 文档："If the context is already done, AfterFunc calls f immediately in a new goroutine." `unsubscribe` 会 `close(c)`（关闭 subscriber channel）。

如果 HTTP 客户端在 `openSegment` 和 `startRunResponse` 返回之间断开连接：
1. `AfterFunc` → `unsubscribe` → **close(events)** → 调用方收到已关闭 channel
2. `pumpRun` 启动，开始 appending 到 hub，但原 subscriber 已不存在

**影响**: 极端时序下，调用方收到已关闭的 channel，`for ev := range events` 立即退出，以为 run 已结束。

---

### 33. `uuid.NewString()` 失败时返回确定性碰撞 ID

**文件**: `lyra/internal/kernel/turn/inmemory.go:22`

```go
func newTurnID() string { return turnIDPrefix + uuid.NewString() }
```

`uuid.NewString()` 在 `crypto/rand` 失败时返回 `"00000000-0000-0000-0000-000000000000"`。此时 `newTurnID()` 返回 `"run_00000000-0000-0000-0000-000000000000"`——所有失败调用返回**相同 ID**。

`StartTurn` 不做重复 ID 检查：
```go
s.turns[handle.TurnID] = state  // ← 覆盖旧 turn!
```

**影响**: 概率极低（需 `crypto/rand` 失败），但一旦发生：旧 turn 从 map 中消失（无法 Cancel/Resume），但其 goroutine 继续运行（泄漏）。新 turn 使用同一 ID。

---

### 34. `inMemoryBlackboard.Spawn` 后 `Clear` 的 `objects` 清空但非 protected 条目残留

**文件**: `agent/runtime/in_memory_blackboard.go:224-240`

```go
func (b *inMemoryBlackboard) Clear() {
    preserved := make(map[string]any, len(b.protected))
    for key := range b.protected {
        if value, ok := b.named[key]; ok {
            preserved[key] = value
        }
    }
    clear(b.named)
    maps.Copy(b.named, preserved)
    b.objects = b.objects[:0]   // ← 清空
    b.hidden = b.hidden[:0]
    clear(b.conditions)
}
```

`b.objects = b.objects[:0]` 清空 slice 但**底层数组未释放**——保留原容量。如果 `objects` 曾在 Spawn 前增长到很大，清空后的 slice 仍占用大量内存（cap 保持原值）。真正的内存释放需 `b.objects = nil`。

**影响**: 子进程创建后，父进程的 `objects` 底层数组未 GC，造成隐性内存占用。（`b.hidden` 同样问题）

---

## 📊 总汇 (六轮审计)

| 级别 | 总数量 | 代表项 |
|------|--------|--------|
| 🔴 严重 | 6 | #1, #14, #15, #19, #24, #25 |
| 🟡 中等 | 26 | #3-#13, #16-#18, #20-#23, #26-#34 |
| 🟢 安全 | 9 | 已验证 |

---

## 📊 总汇 (两轮排查)

| 级别 | 数量 | 类别 |
|------|------|------|
| 🔴 严重 | 2 | TurnEnd 事件丢弃 (#1)、drive/Cancel 终结竞态 (#2) |
| 🟡 中等 | 11 | 重构碎片 (#3)、空中断 (#4)、Terminated 无 Failure (#5)、hub 无界增长 (#6)、hub Subscribe 持锁 (#7)、delta index=0 (#8)、类型断言未检查 (#9)、Remember 吞错 (#10)、Close 非安全 (#11)、脆性 send-on-closed (#12)、snapshot 无超时 (#13) |
| 🟢 安全 | 8 | 竞态/并发/ctx 泄漏/channel/race 已验证安全 |

---

## 🔴 lyra ↔ agent 整合排查 (2026-06-16)

### 35. `task` 工具的子 agent 遇 HITL 时被 agent runtime 判为 Failed

**文件**: `lyra/internal/kernel/agent.go:174-186` + `agent/core/action_typed.go:49-52`

主 action body 调用了 `hitl.HandleInterrupt(pc, err)` 正确消费 InterruptError。但 `task` 子 agent 的 body 没有此调用，InterruptError 直接 return 给 agent runtime → `typedAction.Execute` 将其映射为 `ActionFailed` → 子 process → `StatusFailed`。父 LLM 收到 "ended in failed" 错误。

### 36. 子 agent InterruptError 被 `shouldRetryAction` 误判可重试

**文件**: `agent/runtime/execute_action.go:188-196`

`shouldRetryAction` 只排除 `ReplanRequest`/`haltSignal`，InterruptError → 返回 `true`。`MaxAttempts:1` 避免无限重试，但语义错误。

### 37. 子 agent `turnBudget{}` 无预算约束

**文件**: `lyra/internal/kernel/agent.go:182`

子 agent 在单次 `runChatTurn` 内无预算上限。仅在子 agent 完成后父 agent 下一 tick 才检查。大量 LLM round 成本在检查前已全额发生。

### 38. 🔴 `inMemoryBlackboard.Restore` 清除 protected 条目 → 跨重启 resume 后子 agent 丢失上下文

**文件**: `agent/runtime/in_memory_blackboard.go:264-275`

```go
func (b *inMemoryBlackboard) Restore(named map[string]any, conditions map[string]bool, objects []any) {
    ...
    clear(b.protected)   // ← 清除所有 protected 标记!
}
```

`BlackboardSnapshotter` 接口只捕获 `named`/`conditions`/`objects`，**不捕获 `protected`**。Restore 后 protected 标记全失。

**影响**: 跨重启 resume（`rehydrate` → `RestoreProcess` → `RestoreFromSnapshot`）后，`SpawnChildProtectedOnly` 产生的子 agent **无 session ID 和 cwd**。`task` 工具的子 agent 无法解析工具路径、无法关联对话历史。

### 39. `observedTool.Call` 中 `uuid.NewString()` 生成 callID 可能碰撞

**文件**: `lyra/internal/kernel/observer.go:132-133`

每次 tool call 生成新 `callID = uuid.NewString()`。两个并发的 tool call 理论上可产生相同 ID（概率极低但可能），导致 turn observer 的 `OnToolCallStart`/`OnToolCallEnd` 配对混乱。

### 40. `budget.exceeded` 检查子 process 时使用父 budget 快照

**文件**: `lyra/internal/kernel/usage.go:57-61`

```go
func (b turnBudget) exceeded(pc *core.ProcessContext) bool {
    cost, tokens, _ := pc.Process.Usage()
    ...
}
```

`pc.Process.Usage()` 递归汇总所有子 process。子 agent 同步运行在父 action 内，每次 `runChatTurn` 的 `recordRound` 后调用 `budget.exceeded(pc)`。此时 `Usage()` 走锁下的 `budget.usage()` 遍历 `children`。子 process 的 budget 在并发修改（子 agent 在运行），但 budget 共享 `processState.mu`，父在 `usage()` 中拿 `RLock`，子在 `recordLLMInvocation` 中拿 `Lock`。不会死锁但子 process 的 LLM 写入会阻塞父的预算检查——可能造成微暂停。

---

## 🟡 第九轮排查——记忆/持久/调度层 (2026-06-16)

### 41. 记忆中间件 `persist` 失败导致对话历史永久丢失

**文件**: `core/model/chat/middleware/memory/middleware.go:104-115,137-139`

```go
func (m *middleware) executeCall(...) (*chat.Response, error) {
    resp, err := next.Call(ctx, spliced)
    if err != nil { return nil, err }
    if err := m.persist(ctx, id, toPersist, resp); err != nil {
        return nil, err   // ← 回复已产生但 persist 失败 → 返回 error!
    }
    return resp, nil
}
```

persist 失败时返回 error——模型回复已成功产生（用户看到），但**未存入对话历史**。下一 turn splice 时历史缺失，模型不记得自己说过什么。

### 42. 记忆中间件 `splice` 失败直接中断整个 turn

**文件**: `core/model/chat/middleware/memory/middleware.go:66-69,127-130`

`splice` 调用 `store.Read`，任何错误都导致整条 call/stream 链失败。瞬态存储错误（DB 繁忙、网络抖动）→ 整个 turn 被中止，而非退化为"无历史"模式。

**对比**: stream 路径对消费者取消友好（不 persist），但对存储 Read 错误不宽容。

### 43. `runChatTurn` 在 No-ParkStore 路径中 persist 错误被吞

**文件**: `lyra/internal/kernel/turn.go:82-85`

```go
if isInterruptResult(chunk) {
    recordRound()
    inflightTail.Save(chunk.Result)  // ← Save 用 bb.Set，错误被吞!
    continue
}
```

`inflightTail.Save` 调用 `bb.Set(inflightTailKey, data)`——Set 无返回值。如果 blackboard 是只读或已满，inflight tail 静默丢失→ resume 时 Load 找不到 tail→ 退化为全新 turn。

### 44. `handleNotification` 吞掉 `Shutdown` 错误

**文件**: `lyra/internal/delivery/dispatch/dispatch.go:188-191`

```go
case MethodShutdown:
    var in protocol.ShutdownRequest
    _ = unmarshal(msg.Params, &in)
    _ = d.api.Shutdown(ctx, in)   // ← 错误被吞
```

如果 `Shutdown` 失败（资源无法释放），错误完全不可见——通知无响应通道。

### 45. 记忆中间件 `persist` 对 tool_calls 消息的原子性假设脆弱

**文件**: `core/model/chat/middleware/memory/middleware.go:106-109`

```go
if am := resp.Result.AssistantMessage; !am.IsBlank() && !am.HasToolCalls() {
    msgs = append(msgs, am)
}
```

设计假设 tool-calling middleware 总是将 `(assistant, tool_result)` 成对回传。如果外部 caller 绕过 tool middleware 直接调用记忆中间件层的模型（非 lyra 场景），带 tool_calls 的 assistant message 被静默丢弃→历史断层。

### 46. `adaptStream` goroutine 在 ctx 永不 cancel 时泄漏

**文件**: `lyra/internal/delivery/dispatch/dispatch.go:224-249`

```go
func adaptStream[T any](ctx context.Context, in <-chan T, ...) <-chan StreamFrame {
    out := make(chan StreamFrame)
    go func() {
        for {
            select {
            case <-ctx.Done(): return
            case ev, ok := <-in:
                ...
                select {
                case out <- frame:
                case <-ctx.Done(): return
                }
            }
        }
    }()
    return out
}
```

正常路径 `ctx.Done()` 是逃生阀。但若 caller 拿到 `out` channel 后永不 drain（例如忘了 `for range out`），goroutine 阻塞在 `out <- frame`。ctx 未被 cancel 时**永久泄漏**——goroutine + 未关闭 channel。

---

## 🟡 第十轮排查——MCP/文件监控/会话层 (2026-06-16)

### 47. `Reconnect` 无法区分 dial 失败和 tools/list 失败

**文件**: `lyra/internal/infra/mcp/mcp.go:248-269`

```go
session, err := lynxmcp.Dial(ctx, c.client, cfg)
if err == nil {
    if _, terr := sourceTools(ctx, ...); terr != nil {
        _ = session.Close()
        err = terr   // ← dial 成功但 tools/list 失败 → 同 err 变量
    }
}
// ms.status = dialStatus(err) — 两类失败无区分
```

`dialStatus(err)` 对两类错误无法区分。UI 不知道是"server unreachable" 还是 "server reachable but tools/list failed"——两者都显示为 `statusFailed`。

### 48. `Truncate` 非事务性读-改-写：并发写入消息丢失

**文件**: `lyra/internal/domain/conversation/conversation.go:78-96`

```go
msgs, err := s.store.Read(ctx, sessionID)       // 读
...
if err := s.store.Clear(ctx, sessionID); err != nil { ... }  // 清
...
return s.store.Write(ctx, sessionID, msgs[:keepN]...)       // 写
```

当前 lyra turn 串行执行，暂安全。但 `InjectUser`/`Seed` 与 `Truncate` 之间的并发窗口未受保护——任何并发写入在 Read 和 Clear 之间到达的消息都会被静默丢弃。

### 49. `Seed` 无 freshness 检查——重复 Seed 导致历史拼接

**文件**: `lyra/internal/domain/conversation/conversation.go:49`

注释说 "seed a fresh session only" 但代码无校验。对非空 session 调用 Seed → 新旧消息拼接 → 历史膨胀且语义错误。当前调用方（`sessions.fork`）保证了 freshness，但这是脆性调用约定。

### 50. `gitWatcher.run` 吞掉 `fsw.Errors` 导致静默失效

**文件**: `lyra/internal/delivery/server/filewatch.go:76-77`

```go
case <-w.fsw.Errors:
    // Non-fatal (transient overflow / removed ref dir) — keep watching.
```

错误被丢弃且无 resync 发出。如果监听的目录被永久删除（如 `rm -rf .git`），watcher 不再收到事件但继续运行→客户端永不知道 git 状态已改变。

### 51. `emitToolFileChange` 的工具名精确匹配遗漏 MCP 工具

**文件**: `lyra/internal/delivery/server/workspace_stream.go:170,185`

`fileMutatingTools = {"write": {}, "edit": {}}`——仅匹配精确名称（不区分大小写）。MCP 工具前缀化后为 `"server_write"`——不匹配→文件变更事件不发出→客户端不知道远程 agent 编辑了文件。

### 52. MCP `Reconnect` 中 `refreshTools` 失败静默吞错

**文件**: `lyra/internal/infra/mcp/mcp.go:300-304`

```go
srcTools, err := sourceTools(ctx, lynxmcp.Source{...})
if err != nil {
    continue   // ← 错误被吞——该 server 所有 tools 静默消失
}
```

Reconnect 后 `refreshTools` 重建全量 tool set。某一 server 的 tools/list 失败时该 server 贡献的 tools 从集合中消失——而非保留旧工具列表。UI 端无法区分 "server 无 tool" 和 "tools/list 失败"。

---

## 🟡 第十一轮排查—跨模块深度 (2026-06-16)

### 53. `TokenCountBatcher.ReservePercentage=0` 被默认值覆盖

**文件**: `core/document/batcher_token_count.go:75-77`

```go
func (c *TokenCountBatcherConfig) ApplyDefaults() {
    if c.ReservePercentage == 0 {
        c.ReservePercentage = defaultBatcherReservePercentage  // 0.1
    }
}
```

用户显式设置 `ReservePercentage=0` 时被默认值覆盖为 `0.1`。零值无法区分 "未设置" 和 "明确设为 0"——这是 classic Go API bug。`MaxInputTokenCount=0` 同理（虽然 0 本身不合理）。

### 54. `TokenCountBatcher.Batch` 在单文档超限时丢弃全量

**文件**: `core/document/batcher_token_count.go:132-134`

一个文档超限 → 整个 `Batch()` 调用返回 error。前置文档的 token 估算工作中断且结果丢弃。无 partial-success 机制。

### 55. `ResponseAccumulator.accumulateMetadata` 全部标量字段被后继 chunk 覆盖

**文件**: `core/model/chat/response_accumulator.go:52-57`

```go
r.Metadata.ID = other.ID         // ← 全量覆盖
r.Metadata.Model = other.Model
r.Metadata.Usage = other.Usage
```

后继 chunk 的空值/零值会覆盖首个 chunk 的有效数据。依赖于 provider 适配器仅在首/尾 chunk 发送 metadata——无代码级保护。

### 56. Anthropic adapter `json.Marshal(block.Input)` 吞错误

**文件**: `models/anthropic/chat.go:266`

```go
rawInput, _ := json.Marshal(block.Input)
```

Marshal 错误被吞→ arguments 为空字符串。SDK 应返回合法数据，但异常场景下静默丢失参数。

### 57. `Accumulator.add` 可能 append nil Part

**文件**: `core/model/chat/response_accumulator.go:121-129`

```go
func (a *partAccumulator) add(delta OutputPart) {
    if delta == nil { return }
    if n := len(a.parts); n > 0 && a.parts[n-1].appendDelta(delta) { return }
    a.parts = append(a.parts, delta.clone())  // ← clone()可能返回 nil?
}
```

`clone()` 为 interface 方法——若实现返回 nil（编程错误），parts 含 nil 元素 → 后续 `parts[n-1].appendDelta()` 对 nil 解引用 **panic**。

### 58. `gitWatcher.run` 的 timer 在 loop 前未 drain

**文件**: `lyra/internal/delivery/server/filewatch.go:61-62`

```go
timer := time.NewTimer(gitWatchDebounce)
timer.Stop()
```

`NewTimer`→`Stop()` 之间 timer 已启动。若在纳秒级 race 中 timer 在 Stop 之前触发，`timer.C` 中遗留一个值。进入 loop 后第一个 select 可能立即命中 `<-timer.C`→ fires resync 后 `armed=false`。此时若无事件触发，则直到下一事件才重新 arm。这是极其 rare 的虚假 resync。

---

## 🟡 第十二轮排查—bug hunter 深度 (2026-06-16)

### 59. `hasActiveRun` 漏检 parked run → rollback 导致 parked turn 上下文损坏

**文件**: `lyra/internal/delivery/server/rollback.go:62-71`

```go
func (s *Server) hasActiveRun(sessionID string) bool {
    for _, e := range s.runs {        // ← 只检查 pumping runs
        if e.sessionID == sessionID { return true }
    }
    return false                       // ← parked run 的 pump 已退出→返回 false!
}
```

`s.runs` 仅包含当前 pumping 的 run。Parked run 的 pump goroutine 已退出→不在 `s.runs`→`hasActiveRun` 返回 `false`。但 parked run 的 **agent process 仍在平台注册表中**，可被 resume。此时 rollback 被允许执行→截断 chat-memory log、删除 interrupt 记录→resume 时历史不完整。

**修复**: `hasActiveRun` 同时检查 `s.rt.Interrupts()` 中是否有该 session 的 open interrupt。

### 60. `hasActiveRun` 不检查平台 process registry → 遗漏 running process

**文件**: `lyra/internal/delivery/server/rollback.go:62-71`

`s.runs` 由 `openSegment` 注册、`pumpRun` defer 清理。如果 run 的 pump goroutine 在 deferred cleanup 之前因 panic 退出（虽然当前无 panic 路径），run 条目残留但 pump 已停止。此时 `hasActiveRun` 返回 `true`（残留条目）→ rollback 被阻止——碰巧保护。但如果未来添加了显式清理路径（如 timeout 清理），此保护会消失。当前碰巧安全，但脆性。

### 61. Rollback 的 `KeepMark == -1` 路径导致不一致状态

**文件**: `lyra/internal/delivery/server/rollback.go:136-147`

```go
if b.KeepMark >= 0 {
    s.rt.TruncateMessages(...)  // ← 截断消息
}
// 删除 items + runs + interrupts (无论 KeepMark 值)
for _, rec := range b.Dropped {
    _ = s.rt.Transcript().DeleteRun(...)
    _ = s.rt.Interrupts().Delete(...)
}
```

`KeepMark == -1`（watermark 未记录）→ 消息未截断，但 runs/items/interrupts 被删除。结果：**run 的持久化 items 消失，但其对话消息仍保留在 chat-memory 中**。下个 turn 时 memory middleware 加载完整历史→模型看到已删除 run 的对话。

### 62. `accumulateToolMessage` 的 `EnsureIndex` 扩容参数错误

**文件**: `core/model/chat/response_accumulator.go:150`

```go
msg.ToolReturns = pkgSlices.EnsureIndex(msg.ToolReturns, len(other.ToolReturns)-1)
```

`EnsureIndex(slice, maxIndex)` 确保 slice 至少 `maxIndex+1` 个元素。调用 `EnsureIndex(s, len(other.ToolReturns)-1)` → 确保 `s` 至少有 `len(other.ToolReturns)` 个元素。但后续 loop `for index, ret := range other.ToolReturns` —— `index` 从 0 到 `len(other.ToolReturns)-1`，所以 slice 大小够了。正确。

但如果 `other.ToolReturns` 为空（`len = 0`），`EnsureIndex(s, -1)` 被调用——这可能导致 **panic** 或未定义行为（取决于 `EnsureIndex` 的实现对负数的处理）。

### 63. `emitToolFileChange` 的 `it.Tool.Name` 大小写不敏感匹配但 `fileMutatingTools` 仅为小写

**文件**: `lyra/internal/delivery/server/workspace_stream.go:185`

```go
if _, ok := fileMutatingTools[strings.ToLower(it.Tool.Name)]; !ok {
```

`fileMutatingTools = {"write": {}, "edit": {}}` 仅为精确的小写名。但 MCP tools 的前缀形式为 `"ServerName_write"` → `strings.ToLower` 产生 `"servername_write"` → 不在 map 中 → **MCP 工具的文件变更通知被遗漏**。

### 64. `normalizeContext` 不防御 nil 类型指针的 interface 值

**文件**: `agent/runtime/platform.go:237-241`

```go
func normalizeContext(ctx context.Context) context.Context {
    if ctx == nil { return context.Background() }
    return ctx
}
```

Go interface 陷阱：`var c context.Context = (*myCtx)(nil)` → `c != nil`（interface 非 nil）但底层指针为 nil。`normalizeContext(c)` 返回 `c`，后续 `c.Err()`→nil pointer deref **panic**。当前 lyra 传递的 ctx 均为具体值（非 nil pointer wrapped in interface），安全。但函数签名允许传入此类值——缺少防御。

---

## 📊 总汇 (十二轮审计)

| 级别 | 数量 |
|------|------|
| 🔴 严重 | 8 |
| 🟡 中等 | 60 |
| 🟢 安全 | 9 |
| **合计** | **77** |
