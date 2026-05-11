# 持久化与状态可恢复 — 扩展接入指南

> lynx/agent 默认是 **进程内 in-memory** 的：blackboard、awaitable、process registry 都活在 `*runtime.Platform` 实例的内存里。这对单机短任务足够；要做长任务、跨重启 HITL、多副本横向扩展，需要把状态外置到持久存储。
>
> **lynx 的取舍**：框架不内置具体的持久化后端实现（Redis / Postgres / Mongo / DynamoDB 各家诉求差异大），而是把所有持久化关注点 **抽成扩展点**，让用户按部署形态接入自己想要的后端。本文列出每个接入位、需要实现什么接口、注册怎么做。
>
> 对照 embabel：embabel 提供 `InMemoryAgentProcessRepository` / `InMemoryContextRepository` 这类参考实现 + Spring Boot starters；lynx 等价物是注册一个 `core.Blackboard` extension（prototype 模式）+ 用户自己的 backend 实现。**抽象层等价，开箱实现差异**。

---

## 1. 三个状态层

| 层 | 默认实现 | 是否已有扩展点 |
|---|---|---|
| **Blackboard**（process 内的工件 / 条件 / 事件） | [`runtime.inMemoryBlackboard`](../runtime/in_memory_blackboard.go) | ✅ 注册 `core.Blackboard` extension 作为 prototype |
| **Process registry**（`Platform.GetProcess(id)` / `ActiveProcesses` 看到的 `*AgentProcess` 列表） | `runtime.processRegistry`（map） | ❌ 暂不抽象（见 §4） |
| **Awaitable**（`AwaitInput` 暂存 `core.Awaitable` 等待 ResumeProcess） | `runtime.processSignals.pendingAwaitable`（atomic.Pointer） | ❌ 走 Blackboard 间接持久化 |

只有 **Blackboard** 是显式扩展点；其余两层目前只能保持 in-memory。落地真持久化最重要的是 Blackboard——它承载所有 agent 工件 + 累积事件 + 计算条件，HITL 暂停期保留它就能 resume。Process registry 与 awaitable 的持久化路线见 §4。

---

## 2. 实现 `core.Blackboard` extension

### 2.1 接口契约

`core.Blackboard` 本身就是 `core.Extension`（嵌入了 `Name() string`），把你的实现注册到 `PlatformConfig.Extensions` 即可。lynx 用 **prototype 模式**——注册的实例本身不读不写，runtime 调它的 `Spawn()` 拿到隔离的 per-process 实例。

```go
type Blackboard interface {
    Extension                  // Name() string

    BlackboardReader
    BlackboardWriter

    // Spawn returns a fresh per-process blackboard that starts from a
    // copy of the receiver's state. runtime 在 createProcess 时调用一次。
    Spawn() Blackboard

    Clear()
}
```

`BlackboardReader` + `BlackboardWriter` 见 [`core/blackboard.go`](../core/blackboard.go)，覆盖 `Bind` / `Set` / `Get` / `GetValue` / `HasValue` / `Objects` / `SetCondition` / `GetCondition` / `BindProtected` / `Hide` / `InfoString` 全部读写方法。

写法上推荐 **复用 in-memory 作为 cache**：每次写操作同步 flush 到后端（或异步 flush + WAL），读操作命中本地缓存。这样既能保留 in-memory 的访问延迟，又能在崩溃后从后端重建。

### 2.2 注册

```go
platform := agent.NewPlatform(&runtime.PlatformConfig{
    Extensions: []core.Extension{
        myredis.NewBlackboardPrototype(redisClient, ...),
    },
})
```

注册后，**所有** 通过 `Platform.RunAgent` / `Platform.StartAgent` / `Platform.CreateChildProcess` 创建的 process 都会调用 `registered.Spawn()` 拿一份持久化 blackboard。`ProcessOptions.Blackboard` 仍可逐次覆盖。

### 2.3 子 process 的 Spawn 语义

`Spawn()` 在 lynx 里身兼二职：

1. **`Platform.CreateChildProcess`**：从父 blackboard 调 `Spawn()` 生成子 blackboard——子要与父隔离 + 共享父 snapshot。
2. **runtime 启动新 process**：从注册的 prototype 调 `Spawn()` 拿到第一份实例——prototype 本身保持空状态当模板。

后端实现的 `Spawn()` 必须返回一个**新的、与父/prototype 隔离、共享其 snapshot 的** blackboard——典型做法是复制父的工件 map 后绑定新前缀；或者引用父 + 私有覆写层。注册的 prototype 通常用一个 fresh-empty 实例（`NewXxxBlackboard()` 直接返回），让 `prototype.Spawn()` ≈ 新建一个干净实例。

### 2.4 注意事项

- **Bind 的 dual-binding**：`Bind(value any)` 同时写"it"槽位和类型名槽位（`pkg.Foo` 而非 `*pkg.Foo`），后端要同时处理两次 Set。
- **`Protect(name)` 的语义**：`Clear()` 不能擦掉 protected 条目，后端实现别忘了。
- **事件 / 条件分离**：除了工件，blackboard 还存"条件 → bool"的 map（`SetCondition` / `GetCondition`），是 GOAP 计算 world state 的依据，要一并持久化。
- **并发**：runtime 假设 Blackboard 实现是 goroutine-safe，因为 concurrent tick 会在多 goroutine 写。

---

## 3. ProcessOptions 与持久化的协作

ProcessOptions 上有几个字段需要和持久化 blackboard 协作：

| 字段 | 说明 |
|---|---|
| `ProcessOptions.Blackboard` | 直接传 backend 实现；优先级最高（甚至高于注册的 `Blackboard` extension prototype），适合 per-call 选择不同存储 |
| `ProcessOptions.OutputChannel` | 输出流；持久化后端可以同时 mirror 一份到外部消息总线，host 可订阅 |
| `ProcessOptions.Budget` | 与持久化无关，但跨重启要注意：恢复时累计的 cost/tokens/actions 应从 backend 读出而非清零 |

---

## 4. Process registry / Awaitable 暂未抽象

embabel 提供 `AgentProcessRepository` 作为 `process_id → AgentProcess` 的持久化仓。lynx 当前只有内存 `processRegistry`，**重启后所有 process 丢失**。两条出路：

### 4.1 短期：通过 Blackboard 做"软持久化"

如果用户注册的 `Blackboard` extension 是持久化的，HITL 暂停期间的 awaitable 也要落到 blackboard——用户在 awaitable 的 `OnResponse` 里写"approval"到 blackboard，host 重启后只需重建 process（按 agent 配置 + bindings 重新 RunAgent），新 process 读到 blackboard 已有的 approval 就跳过 HITL。

要点：用户的 awaitable 把"决策"写进 blackboard，而不是只暂存"等待"。

### 4.2 长期：等 P3 抽象出 `runtime.ProcessRepository`

未来可以补一个：

```go
type ProcessRepository interface {
    core.Extension
    Save(*AgentProcess) error
    Load(id string) (*AgentProcess, error)
    List() []string
}
```

并让 `Platform.RunAgent` / `ContinueProcess` 在 process 创建/状态切换时调用。**目前不做**——等真有跨重启 HITL 的明确用例再设计；先保证 Blackboard 接口够用。

---

## 5. 推荐的 backend 实现策略

### 5.1 Redis

- 以 `agent:{processId}:bb` 为前缀，每个 key 一个 hash field（artifacts / conditions / protected 各一个 hash）
- 工件用 JSON 序列化，类型字段单独存 `__type__` 便于反序列化
- `Bind` → 同时 HSET `it` + `<type>`
- `Spawn` → 拷贝父进程 hash 到子 prefix，或用 Redis 7+ COPY

### 5.2 SQL（Postgres / SQLite）

- 一张 `agent_artifacts(process_id, name, type, payload, created_at)` 表，PK 复合
- `Bind` 是两条 INSERT（`it` + 类型名）
- 用 trigger 或外部任务做 GC

### 5.3 内存 + WAL（轻量持久化）

- 复用 `runtime.inMemoryBlackboard` 全部读路径
- 每次写操作 append 一条 WAL 记录到磁盘
- 启动时 replay 重建状态
- 适合单机重启即可，不要求高可用

---

## 6. 测试持久化实现

参考 `lynx/agent/runtime` 包内 in-memory 实现的契约：

```go
import (
    "testing"
    "github.com/Tangerg/lynx/agent/core"
)

func RunBlackboardContractTests(t *testing.T, factory func() core.Blackboard) {
    // 验证 Bind / Set / Get 基础读写
    // 验证 Spawn 隔离
    // 验证 Protect 不被 Clear 擦除
    // 验证 dual-binding：Bind(myFoo{}) 后 Get("it") 和 Get("pkg.myFoo") 都能取到
    // ...
}
```

写自己的 backend 时，把 `factory` 传成 `myredis.NewBlackboard` 之类的构造器，跑一次确保契约一致。

---

## 7. 与 embabel 对照

| | embabel | lynx |
|---|---|---|
| Blackboard 抽象 | `Blackboard` 接口 + `BlackboardProvider` SPI | `core.Blackboard` 接口本身就是 `core.Extension`（prototype 模式） |
| 默认 in-memory 实现 | ✅ `InMemoryBlackboard` | ✅ `runtime.inMemoryBlackboard`（私有，框架默认 fallback）|
| 开箱持久化 backend | ❌（用户自己挂 Spring Data 等）| ❌（同 embabel 路线）|
| Process repository 抽象 | ✅ `AgentProcessRepository` | ❌（未抽象，可走 Blackboard 软持久化绕过）|
| Context / session repository | ✅ `ContextRepository`（多 session 跨 process）| ❌（不在路线图）|

**结论**：抽象层等价，开箱实现都让用户自补；lynx 在 Process repository 这一层暂时弱一点，但 Blackboard 路径足以撑住绝大多数生产诉求。

---

## 8. 路线图

- ✅ `Blackboard` extension（prototype 模式）— 已落地
- ⏳ P3：`runtime.ProcessRepository` 抽象 + 一个内存默认实现 — 等真用例
- ⏳ P3：`AwaitableRepository`（暂存 awaitable 跨重启）— 多数情况下走 Blackboard 软持久化即可，独立抽象优先级更低
