# 代码坏味道审计报告

> 审计范围：`app` 之外的全部 85 个 workspace module。
> 审计基线：全部 85 模块 `go vet` + `go test` 实测全绿；仓库零 TODO / FIXME / HACK。
> 优先级取向：**agent 与功能性模块（core / rag / etl / tools / mcp / skills / evaluation / a2a / otel）为主**；
> 集成性模块（models / vectorstores / historystores）降级，集中放在附录 D。
>
> 本报告只记录**已核实**的问题：每条都给出 `file:line` 证据、根因判断和具体失败场景。
> 扫描中产生的误报已剔除（见附录 F）。未修改任何代码。
>
> **姊妹报告**：[`OBSERVABILITY_AND_EVALUATION.md`](OBSERVABILITY_AND_EVALUATION.md) 专项审计可观测性与可评估性**能力面**
> （本报告只看通用坏味道）。其中 A6、A3 两条同属可观测性，在两份文档间交叉引用。

---

## 目录

- **A. 宏观架构问题**
  - A1 Core 协议 DTO 指针语义 vs agent 值语义
  - A2 `samber/lo` 进 Core
  - A3 delta 观测链 context 断裂
  - A4 `tools/fs.Executor` 胖接口
  - A5 Core 覆盖率闸门失效
  - A6 rag / mcp / a2a 自埋点违反可观测性法则
- **B. 微观实现问题**
  - B1 `quiesceTree` channel 所有权手工维护 ★
  - B2 脱钩 goroutine 用 `context.Background()`
  - B3 并发不变量零文档
  - B4 同子系统两种幂等机制
  - B5 `newEvent` 10 个位置参数
  - B6 5 处静默 panic 吞噬
  - B7 god struct 缺字段分区注释
  - B8 `a2a` 用 `%v` 断因果链
  - B9 两处 `fmt.Errorf` 包常量
  - B10 nil-ctx 反应式兜底
- **C. 反 Go 范式**
  - C1 不可变构造器返指针
  - C2 185 处防御式 nil 守卫
  - C3 值接收者内取址调用
  - C4 接收者命名不一致
- **附录 D**：集成性模块（低优先级）
- **附录 E**：已核查为干净的维度
- **附录 F**：扫描误报记录

---

## 优先级总览

> 编号（A1/B1/C1…）只用于定位，**不代表优先级**；优先级见下表与文末处理顺序。

| # | 问题 | 模块 | 性质 | 破坏性 API 改动 |
|---|---|---|---|---|
| **B1** | `quiesceTree` channel 所有权手工维护 | agent | 可导致不可恢复死锁 | 否 |
| **A6** | rag / mcp / a2a 自埋点，违反可观测性架构法则 | rag, mcp, a2a | **明文法则违反** | 是 |
| **A1** | Core 协议 DTO 指针语义 vs agent 值语义 | core | 架构约定自相矛盾 | **是（面很大）** |
| **A3** | delta 观测链 context 断裂 | agent | 死参数 + trace 断裂 | 是（内部签名 + 一个公开接口） |
| **A2** | `samber/lo` 进 Core | core + 20 模块 | 依赖纪律 | 否 |
| **B2** | 脱钩 goroutine 用 `context.Background()` | agent | trace 断裂 | 否 |
| **B3** | 并发不变量零文档 | 全仓 | 可维护性 | 否 |
| **A4** | `tools/fs.Executor` 胖接口 | tools | ISP | 是 |
| **A5** | Core 覆盖率闸门失效 | core | 门禁失效 | 否 |
| C / B 其余 | 见正文 | — | 局部 | 否 |

---

# A. 宏观架构问题

## A1 · Core 协议 DTO 用指针语义，与 agent 的值语义构成同仓两套矛盾约定

**性质**：架构层面的根因问题。它是 C1、C2、C3 的共同源头。

### 事实

同一个仓库对「不可变协议值」存在两套相反的约定：

**agent 模块 —— 值语义（正确的那套）**

```go
// agent/delta.go:64
func (d Delta) Valid() bool {
	return d.processID.Valid() && d.effectID.Valid() && d.effectSequence > 0 &&
		!d.emittedAt.IsZero() && len(d.payload) > 0
}

// agent/event.go:169
func (e Event) Valid() bool { ... }
```

构造器一律返回值：

```
newDelta(...)        (Delta, error)
newEvent(...)        (Event, error)
newSnapshot(...)     (Snapshot, error)
newSignal(...)       (Signal, error)
newEffect(...)       (Effect, error)
newTreeSnapshot(...) (TreeSnapshot, error)
newProcessID()       (ProcessID, error)
```

值不可能为 nil，`Valid()` 是纯逻辑，没有一行防御代码。

**core 模块 —— 指针语义（法则明令禁止的那套）**

```go
// core/media/media.go —— 6 个方法，每个都以 nil 守卫开头
78:  if m == nil {
133: if m == nil {
150: if m == nil {
163: if m == nil {
176: if m == nil {
197: if m == nil {
```

```go
// core/vectorstore/filter/literal.go:69-72
// 文档称 filter 只公开「不可变 AST」，但每个访问器都要防 nil
func (l *Literal) IsString() bool { return l != nil && l.kind == LiteralString }
func (l *Literal) IsNumber() bool { return l != nil && l.kind == LiteralNumber }
func (l *Literal) IsBool() bool   { return l != nil && l.kind == LiteralBool }
func (l *Literal) IsNull() bool   { return l != nil && l.kind == LiteralNull }
```

**Core 全模块共 185 处防御式 nil 守卫**，密度最高的文件：

| 文件 | 守卫数 |
|---|---|
| `core/vectorstore/filter/literal.go` | 14 |
| `core/moderation/response.go` | 12 |
| `core/vectorstore/filter/binary_expr.go` | 11 |
| `core/embedding/response.go` | 11 |
| `core/transcription/response.go` | 10 |
| `core/speech/response.go` | 10 |
| `core/image/response.go` | 10 |

### 为什么这是宏观问题而非风格问题

Core 是窄腰，它的选择**强制传播给每一个下游**：

```go
// core/chat/model.go:20
type Model interface {
	Call(ctx context.Context, request *Request) (*Response, error)
}
```

约 30 个 model provider、24 个 vectorstore 后端、全部 agent / RAG / tool 消费方，都必须处理可为 nil 的协议值。而这些类型的文档本身声明它们是不可变值。

项目法则原文点名了这条：

> ❌ 让本应返值的不可变构造器返指针。

Go 惯例同样反对：`make zero values useful`。指针 DTO 让零值不可用，把「值是否存在」的判断推给每一个调用方。

### 影响面（必须先评估再动）

97 个 `New*` 返回指针，其中属于本问题的协议 DTO 构造器：

```
core/chat:          NewOutput NewRequest NewResponse
core/embedding:     NewRequest NewOutput NewResponse
core/image:         NewRequest NewOutput NewResponse
core/moderation:    NewRequest NewOutput NewResponse
core/speech:        NewRequest NewOutput NewResponse
core/transcription: NewRequest NewOutput NewResponse
core/document:      NewDocument
core/media:         NewBytes NewURI NewReference
core/vectorstore:   NewIndexRequest NewSearchRequest NewSearchResult NewSearchResponse
core/vectorstore/filter: NewLiteral NewListLiteral NewIdent
```

其余 `New*` 返回指针的是**有状态对象**（`NewEngine`、`NewDispatcher`、`NewRegistry`、`NewStore`、`NewPlatform`…），返回指针是正确的，不在本条范围内。

### 处理建议

这是本报告里唯一需要**分批迁移**的改动，且必然是破坏性公开 API 变更。建议按 modality 逐个 package 推进，每批一个可独立 revert 的 commit。`agent` 模块可以直接作为参照实现。

若判断迁移成本不可接受，**次优方案**是保留指针但删掉全部 receiver nil 守卫，把「不传 nil」写进接口契约 —— 但这与第二法则「不变量不能交给调用方记得别犯错」冲突，只能算权衡，不算治本。

---

## A2 · `samber/lo` 成为 Core 依赖，21 个模块只为一个 12 行 reflect 助手

### 事实

`samber/lo` 是 **21 / 85 个模块的直接依赖，包含 Core 本身**（`core/go.mod`）。

全仓 117 处 `lo.*` 调用中，**104 处是 `lo.IsNil`**；Core 内部 24 处调用**全部**是 `lo.IsNil`。

被依赖的实现（`type_manipulation.go`）：

```go
func IsNil(x any) bool {
	if x == nil { return true }
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer,
	     reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
```

剩余 13 处也都是几行标准库可替代的：

| 函数 | 处数 | 位置 |
|---|---|---|
| `lo.MapErr` | 6 | `historystores/*/storage.go`（6 家各一处，内容完全相同） |
| `lo.CoalesceMapOrEmpty` | 4 | `vectorstores/{typesense,azurecosmos,vectara,couchbase}/store.go` |
| `lo.Substring` + `lo.RuneLength` | 2 | `tools/web/jina/jina.go:189-190` |
| `lo.SliceToMap` | 1 | `etl/formatter_simple.go:44` |

### 判据

需求本身是**正当的**：`lo.IsNil` 解决的是「接口里装了 typed nil」这个 Go 真实陷阱，`x == nil` 检测不出来。agent 模块用它检查 SPI 接口（`config.Dispatcher`、`DeploymentResolver`、listener）也是恰当用法。

问题在解法。项目法则：

> 采用条件是它能净删除解析、边界条件与维护面，而不是**仅把一个函数包进新依赖**。

这正是被排除的形态：为一个 12 行 helper，把 lodash 风格的通用库拉进包括窄腰在内的 21 个发布单元。

### 处理建议

在 Core 内建立唯一 owner（例如 `core/internal/ptr` 已存在且 100% 覆盖，是天然落点），其余模块从 Core 取用；`core/internal` 不可跨 module 导出，因此需要评估是放 `core` 某个公开 package 还是各模块各留一份 3 行实现（后者符合「相似但独立演化不 DRY」）。

注意：本条与 A1 是**两个独立问题**，不要混淆。`lo.IsNil` 检查的是**接口**里的 typed nil；A1 的 185 处守卫检查的是**具体指针**。修好 A1 不会消除对 `lo.IsNil` 的需求。

---

## A3 · delta 观测链 context 断裂 —— `OnDelta` 的 ctx 参数结构上恒为 `Background`

### 事实

`agent/effect_dispatch.go` 同一个函数体内，相隔 10 行，两种处理方式：

```go
// effect_dispatch.go:142-151 —— delta 生产者，ctx 完全丢失
emit := func(payload json.RawMessage) {
	if !acceptingDeltas.Load() { return }
	sequence := deltaSequence.Add(1)
	delta, err := newDelta(p.controller.processID, record.ID, sequence, time.Now(), payload)
	if err != nil || !p.engine.observation.offerDelta(delta) {   // ← 无 ctx
		dropped.Add(1)
	}
}

// effect_dispatch.go:153 —— effect 本身，写法正确
dispatchCtx := context.WithoutCancel(ctx)
```

链路末端只能兜底：

```go
// agent/observation.go:143-146
func callDeltaListener(listener DeltaListener, delta Delta) {
	defer func() { _ = recover() }()
	listener.OnDelta(context.Background(), delta)   // ← 恒为 Background
}
```

### 这是漂移，不是设计

同模块的 **event 链写法是对的**：

```go
// agent/process_events.go:30
p.engine.observation.publishEvent(context.WithoutCancel(ctx), event)

// agent/process_start_outcome.go:128
acknowledger.AcknowledgeProcessStartOutcome(
	context.WithoutCancel(contextOrBackground(ctx)), outcome,
)
```

`publishEvent(ctx, ...)` 收 ctx 并透传；`offerDelta(delta)` 不收。两条观测链在同一个 bus 上，一条守规矩一条不守。

### 后果

公开接口 `DeltaListener.OnDelta(ctx context.Context, delta Delta)`（`agent/observation.go:34`）的 ctx 参数**不可能是 Background 以外的任何值** —— 是死参数。

`otel/agent/observer.go` 因此无法把 delta 观测挂回产生它的 trace。项目法则：

> **全链路**：trace_id 在入口生成，脱钩的后台 goroutine 用 `context.WithoutCancel` 保住 span。

### 根因与治本方向

根因在 `observationBus.offerDelta(delta Delta) bool`（`agent/observation.go:85`）签名不收 ctx，因此 `deltaObservation`（同文件 :56）无法携带 ctx，drainer（:102 `deliverDeltas`）无从传递。

治本需要：`offerDelta(ctx, delta)` → `deltaObservation` 增加 ctx 字段（存 `context.WithoutCancel(ctx)`）→ `callDeltaListener` 透传。调用点 `effect_dispatch.go:148` 处 `dispatchCtx` 已经算好，直接可用。

**替代方案**：若判定 delta 不需要 trace 归属，则应把 ctx 参数从 `DeltaListener` 接口删掉 —— 保留一个恒为 Background 的参数是最坏选项。二选一都涉及公开 API。

---

## A4 · `tools/fs.Executor` 胖接口 —— 仓库自己有正确范式却没照做

### 事实

`tools/fs/fs.go:12` 定义 6 方法接口。6 个 tool 各自**只用其中 1 个**，却都持有完整接口：

```
applypatch.go   → executor.ApplyPatch
edit.go         → executor.Edit
glob.go         → executor.Glob
grep.go         → executor.Grep
read.go         → executor.Read
write.go        → executor.Write
```

### 为什么不适用「库内部单实现可用具体类型」豁免

`tools/CLAUDE.md` 明确声明 Executor 是可替换后端的公开 SPI：

> 两层 SPI：**Tool 层**对 LLM，**Executor / Provider 层**做真正执行（本地 / 远程 / 沙箱后端可换）

法则原文：

> 窄接口优先留给**公开 SPI**、应用消费边界和**真实替换点**。
> ❌ 胖接口塞所有方法 —— 按消费者拆 ISP。

### 仓库内已有的正确范式

同一个仓库，Core 里就是标准答案：

- `core/history`：Reader / Writer / ReadWriter / Clearer / Store / Lister / Replacer / Counter（8 个）
- `core/vectorstore`：Indexer / Searcher / IDDeleter / FilterDeleter / Batcher（5 个）

`tools/fs` 是唯一没跟上的。

### 成本已经在了

`Executor` 目前 **1 个实现（`LocalExecutor`）、0 个测试替身**。`tools/fs/*_test.go` 全部传 `nil`，落到 `NewLocalExecutor("")` 打真实文件系统：

```go
// tools/fs/tools_test.go:27-29
{"read",  NewReadTool(nil).Definition().Name},
{"write", NewWriteTool(nil).Definition().Name},
{"edit",  NewEditTool(nil).Definition().Name},
```

要给 write 写一个测试替身，必须实现 Read / Edit / ApplyPatch / Glob / Grep 五个用不到的方法。这既是 ISP 问题的直接代价，也说明「后端可换」这个 SPI 承诺从未被实际行使过。

### 处理建议

拆成 6 个单方法接口，`Executor` 保留为它们的 union（`core/history.Store` 就是这个形状）。各 tool 的字段类型收窄到自己那一片。这是破坏性公开 API 改动。

---

## A5 · Core 覆盖率闸门已失效

### 事实

`scripts/check-core-coverage.sh` 的阈值远低于实测值，ratchet 从未随覆盖率提升而上调：

| 包 | 实测 | 闸门 | 松弛 |
|---|---|---|---|
| `core/document` | 91.7% | 29.2% | **62.5 pt** |
| `core/image` | 65.4% | 42.3% | 23.1 pt |
| `core/embedding` | 82.6% | 61.0% | 21.6 pt |
| `core/transcription` | 58.9% | 42.1% | 16.8 pt |
| `core/speech` | 60.5% | 47.8% | 12.7 pt |
| `core/moderation` | 57.3% | 46.5% | 10.8 pt |
| `core/vectorstore` | 86.5% | 78.4% | 8.1 pt |

`core/document` 可以从 91.7% 掉到 29.3% 而 CI 依然通过。

### 更严重的：多个公开 package 根本没有下限

闸门列表（脚本内 heredoc）只含 12 个包。**不在列表中**的公开 package：

| 包 | 实测覆盖率 | 闸门 |
|---|---|---|
| `core/tool` | 93.1% | 无 |
| `core/embeddingclient` | 93.9% | 无 |
| `core/history` | 90.9% | 无 |
| `core/chatclient` | 90.8% | 无 |
| `core/chatclient/safeguard` | 90.5% | 无 |
| `core/jsonschema` | **75.0%** | 无 |
| `core/vectorstore/inmemory` | 70.0% | 无 |
| `core/tokenizer` | 纯接口（no statements） | 无 |

`core/jsonschema` 是所有 tool schema 派生的唯一 owner，75% 且无下限。

### 处理建议

把阈值重设到「实测值 − 2pt」，并补齐缺失的包。脚本注释当前写的是「Thresholds come from the immutable P0 baseline」—— 若 baseline 有意不可变，则应另设一道「不得低于上次实测」的动态闸门，否则这道门禁事实上不拦任何现实回归。

---

## A6 · `rag` / `mcp` / `a2a` 自持全局 tracer —— 违反可观测性架构法则

**性质**：本报告中唯一有**明文条款直接对应**的架构违反，其余多为约定漂移或判断题。

### 法则原文

`otel/CLAUDE.md:10`：

> 按领域拆分的观测外挂：`otel` 是无根包的集成层命名空间；`otel/agent`、`otel/chat`、`otel/history`、`otel/vectorstore` 分别包装自己领域的能力……**领域模块不 import 本模块或官方 OTel。**

根 `CLAUDE.md` 可观测性段：

> **依赖边界**：……`otel` 是可依赖被观测能力模块的集成层，**被观测模块不能反向 import `otel` 或官方 OTel**。

根 `CLAUDE.md` 拓扑段把它们明确归为同一类：

> `agent`、`a2a`、`mcp`、`rag`、`etl`、`evaluation`、`tools`、`skills`、`otel` 是**能力模块**

### 事实

同一架构层（layer 1 能力模块）内存在两套相反的可观测性策略：

| 模块 | 源文件 import 官方 OTel | go.mod 声明 | 策略 |
|---|---|---|---|
| `agent` | 0 | 0 | 外部埋点（`otel/agent` 装饰器）✅ |
| `etl` | 0 | 0 | 无埋点 ✅ |
| `evaluation` | 0 | 0 | 无埋点 ✅ |
| `tools` | 0 | 0 | 无埋点 ✅ |
| `skills` | 0 | 0 | 无埋点 ✅ |
| **`rag`** | **3** | **3** | **自埋点** ❌ |
| **`mcp`** | **3** | **3** | **自埋点** ❌ |
| **`a2a`** | **3** | **3** | **自埋点** ❌ |

违规实现（三处形态一致）：

```go
// rag/tracing.go:14
var ragTracer = otel.Tracer("github.com/Tangerg/scope/rag")

// mcp/tracing.go:9
var mcpTracer = otel.Tracer("github.com/Tangerg/scope/mcp")

// a2a/tracing.go:8
var a2aTracer = otel.Tracer("github.com/Tangerg/scope/a2a")
```

调用点：`rag`（多处）、`mcp/server.go:75`、`mcp/tool.go:86`、`a2a/executor.go:68`、`a2a/tool.go:87`。

`otel` module 实际只覆盖 `agent` + `core`（`otel/go.mod` 仓内依赖仅这两个；子包为 `otel/{agent,chat,history,vectorstore,slog}`）—— 没有 `otel/rag`、`otel/mcp`、`otel/a2a`。

### 后果

1. **消费者体验分裂**：用 `agent` 必须显式装配 `otel/agent` 的 Observer 才有 span；用 `rag` / `mcp` / `a2a` 则 span 自动产生，无法选择不要。
2. **埋点策略不可替换**：`otel.Tracer(...)` 读的是**全局** TracerProvider，是隐藏的全局依赖。这三个模块的用户无法注入、替换或在测试中隔离 tracer；而 `agent` 的用户可以选择不装 Observer。这与「不新增全局 registry / state」的方向也相悖。
3. **`otel` module 定位被架空**：它本应是唯一集成层，实际只服务 2 个模块。
4. **门禁盲区**：`dev/repoarch/architecture_test.go:188` 只对 **core** 强制 `go.opentelemetry.io/otel` 禁令，`assertProviderBoundary`（:595）只对 **provider** 强制。layer-1 能力模块完全不在检查范围内 —— 这正是门禁本该拦住却没拦的漂移。

### 处理方向

按 `otel/agent` 的既有形态新建 `otel/rag`、`otel/mcp`、`otel/a2a` decorator 子包，把三个 `tracing.go` 及其调用点迁出领域模块，并从三个 `go.mod` 移除 OTel 依赖。

**同时应补门禁**：把 `architecture_test.go` 中的 OTel 禁令从「仅 core」扩展到全部 layer-0 / layer-1 能力模块，否则修完还会漂回来。

需要先确认：`a2a` / `mcp` 是否被有意归类为根 CLAUDE.md 中「应用、**integration** 和独立 otel module 直接使用官方 OTel API」里的 integration。若是，则应在 `otel/CLAUDE.md` 与根 CLAUDE.md 中显式写明豁免（并解释为何 `rag` 也在内），消除条款冲突；否则按上述迁移。**当前两处条款互相矛盾，这本身就需要裁决。**

---

# B. 微观实现问题

## B1 · `quiesceTree` 的 channel 所有权靠 6 处手工 `close` 维护 ★ 最高风险

**位置**：`agent/tree_snapshot.go:339-393`
**函数复杂度**：嵌套深度 6，循环内 2 个 `continue`，模块内最难推理

### 事实

```go
release := make(chan struct{})
for {
    ...
    close(release); return nil, err                 // 348
        close(release); return nil, err             // 355
            close(release); return nil, waitErr     // 370
        close(release); return nil, err             // 375
        close(release); return nil, ErrEngine...    // 379
    close(release); return nil, err                 // 390
}
return &treeQuiescence{controllers: ..., releaseGate: release}, nil  // 393 —— 故意不 close
```

**6 条错误出口各自手工 `close`；1 条成功出口故意不 close**（所有权转移给 `treeQuiescence.releaseGate`，稍后在 `:333` 关闭）。

没有 `defer`。没有任何注释说明这个所有权规则。

### 为什么这是高风险而不是风格问题

消费端 `agent/process_loop.go:284-331` 的 `holdQuiescence`，是被冻结进程停驻的循环：

```go
for {
    select {
    case <-applyGate:                          // 回到循环
        ...
    case <-quiescence.command.release:         // ← 唯一的 return 出口
        ...
        return
    case command := <-p.controller.commands:   // 全部 case 分支回到循环
        ...
    case <-*hostDone:                          // 一次性，之后置 nil channel
        ...
    }
}
```

**没有 `ctx.Done()` 逃生口，没有超时。** 而 `quiesceTree` 会冻结整棵树的**每一个** controller。

**失败场景**：将来任何人在这个 depth-6 嵌套循环里新增一条 `return` 而漏掉 `close(release)` → 该树全部进程 goroutine 永久阻塞 → 整棵 agent 进程树不可恢复地挂死，context 取消也救不回来。编译器不报错，现有测试不覆盖。

### 治本方向

用 `defer` + 成功路径显式转移所有权，把不变量从「6 处人工配对」变成结构性保证。例如：

```go
release := make(chan struct{})
transferred := false
defer func() { if !transferred { close(release) } }()
...
transferred = true
return &treeQuiescence{controllers: controllers, releaseGate: release}, nil
```

改动局限在单个函数内，**不涉及任何公开 API**，可立即执行。建议作为第一项处理。

---

## B2 · 脱钩 goroutine 用 `context.Background()` 而非 `context.WithoutCancel`

### 事实

```go
// agent/child_start.go:99 —— ctx 在 70 / 78 / 91 行都还在使用
go childLoop.run(context.Background())
```

```go
// agent/tree_snapshot.go:600-606 —— 恢复树时，只有 root 拿到真 ctx
rootContext := contextOrBackground(ctx)
...
runContext := context.Background()
if entry.controller.processID == restoration.wire.RootID {
    runContext = rootContext
}
go entry.loop.run(runContext)
```

子进程需要脱钩**生命周期**（正确 —— 子 agent 必须活过创建它的请求），但不该同时丢掉 trace 与请求值。法则明确要求 `context.WithoutCancel`。

`tree_snapshot.go:602` 的后果尤其明显：树恢复后，root 进程的 span 挂在原 trace 上，全部非-root 子进程变成孤立 root span —— trace 树从中间断成两截。

### 完整站点清单

| 位置 | 场景 | 是否有 ctx 在作用域 |
|---|---|---|
| `agent/child_start.go:99` | 启动子进程 loop | **是** |
| `agent/tree_snapshot.go:602` | 恢复非-root 进程 | **是** |
| `agent/waiting_subtree_cancellation.go:105` | 等待树 settle | 是 |
| `agent/child_wait_engine.go:123` | 投递父终止 | 否（回调无 ctx 参数） |
| `agent/child_wait_engine.go:155` | 投递子完成 | 否（同上） |

前三处可直接改；后两处的根因是回调签名不带 ctx，需一并评估。

---

## B3 · 并发不变量零文档（系统性）

### 事实

全仓 **23 个带锁结构体**，`guarded by` / 锁序 / goroutine 所有权注释 —— **0 处**。

项目法则把这条列为**必须写注释的第 5 类**：

> **并发 / 事务 / 安全约束**：goroutine 所有权与生命周期、锁持有顺序、channel 关闭方、ctx 取消语义、信任边界 —— 违反不报编译错、只在生产炸。

代码本身实测是对的，但不变量全部只存在于作者脑中：

| 位置 | 我逆向出的实际不变量 | 是否写下 |
|---|---|---|
| `agent/engine.go:99,102` | 两把锁；`treeOperationsMu` 是叶子锁，其临界区内绝不获取 `e.mu`（已核对 `tree_operation.go` 两处临界区，确无嵌套） | ❌ |
| `agent/tree_snapshot.go:326` | `released bool` 的裸读写是安全的，因为四个调用点都在外层 `resolution.mu` 保护下 | ❌ |
| `agent/process_loop.go:284` | `holdQuiescence` 唯一退出路径是 `release` channel，无 ctx 逃生 | ❌ |
| `core/tool/registry.go:32`、`core/vectorstore/inmemory/store.go:72` 等 | — | ❌ |

`agent/engine.go` 的 17 个字段恰好按两把锁分成两组（空行分隔），补两行注释即可同时解决 B7。

---

## B4 · 同一子系统内两种幂等机制

```go
// agent/tree_operation.go:41-53 —— sync.Once
func (t *treeOperation) release() {
	if t == nil || t.engine == nil { return }
	t.once.Do(func() { ... })
}

// agent/tree_snapshot.go:329-335 —— 裸 bool
func (t *treeQuiescence) release() {
	if t == nil || t.released { return }
	close(t.releaseGate)
	t.released = true
}
```

两个紧邻的、同属树操作子系统的 release 路径，用了两种不同的幂等保证。

`treeQuiescence.release()` 有 4 个调用点（`tree_snapshot.go:319`、`waiting_subtree_cancellation.go:101` / `:148` / `:188`）。**实测无竞态** —— 全部处于 `PreparedWaitingSubtreeCancellation.resolution.mu`（`waiting_subtree_cancellation.go:45`）的串行化之下。但读者必须一路追到那把锁才能确认裸 bool 是安全的，而 `treeQuiescence` 类型上没有任何说明。

要么统一为 `sync.Once`，要么写清「由 resolution.mu 保护」。

---

## B5 · `newEvent` 10 个位置参数

`agent/event.go:84`：

```go
func newEvent(
	processSequence uint64,
	processID ProcessID,
	deploymentRef DeploymentRef,
	relation ProcessRelation,
	stepSequence uint64,      // ← 与 processSequence 同型
	effectID EffectID,
	name string,
	phase EventPhase,
	occurredAt time.Time,
	payload json.RawMessage,
) (Event, error)
```

全模块最长参数表。两个 `uint64` 语义完全不同（进程序号 vs 步骤序号），传反了编译器不报错，`Valid()` 也检查不出来（两者都只要求 > 0）。

上游 `processLoop.publishEvent`（`agent/process_events.go:10`）也有 6 个参数，同样问题较轻。

考虑改为参数结构体，或让它成为 `processLoop` 的方法（大部分参数本就来自 loop 自身状态）。

---

## B6 · 5 处完全静默的 panic 吞噬

```
agent/observation.go:81            defer func() { _ = recover() }()   // callEventListener
agent/observation.go:144           defer func() { _ = recover() }()   // callDeltaListener
agent/interaction/observer.go:49   defer func() { _ = recover() }()
agent/interaction/observer.go:57   defer func() { _ = recover() }()
agent/interaction/observer.go:68   defer func() { _ = recover() }()
```

接口文档确实声明了「Panics are isolated from Process execution」（`agent/observation.go:11-13`），隔离本身是对的。问题是**完全静默** —— listener 里的 bug 永久不可见，既不计数也不上报。

对比同模块的处理：delta 被丢弃时有 `EventDeltaDropped` 事件（`agent/effect_dispatch.go:176`）并计入 `usage.DroppedDeltas`。listener panic 什么都没有。

按可观测性法则，这类事实应当被记录（metric 计数或事件），而不是消失。

---

## B7 · god struct 缺字段分区注释

| 类型 | 字段数 | 位置 |
|---|---|---|
| `processLoop` | 25 | `agent/process_loop.go:11` |
| `processSnapshotWire` | 24 | `agent/process_snapshot.go:173` |
| `processController` | 18 | `agent/process.go:271` |
| `Engine` | 17 | `agent/engine.go:90` |
| `executionState` | 15 | `agent/interaction/state.go:39` |
| `executionState` | 12 | `agent/planning/state.go:36` |

法则对固有复杂度的处理有明确规定：

> 先判断是不是固有复杂度（是则**用字段分区注释表达**，而非硬拆）。

这些确实是状态机，属于固有复杂度，**不建议硬拆**。但目前只有空行分组、没有分区注释。`processLoop` 的 25 个字段实际分为「依赖」与「运行时状态」两区；`Engine` 的两区恰好对应两把锁（与 B3 合并处理）。

---

## B8 · `a2a` 用 `%v` 包装导致因果链断裂

`a2a/origin_policy.go` 共 5 处：

```go
35:  return fmt.Errorf("%w: %v", ErrOriginNotAllowed, err)
74:  return endpointOriginPolicy{}, fmt.Errorf("%w %q: %v", ErrInvalidCardURL, cardURL, err)
83:  return endpointOriginPolicy{}, fmt.Errorf("%w %q: %v", ErrInvalidRPCOrigin, rawOrigin, err)
102: return fmt.Errorf("%w: supported interface %d URL %q: %v", ErrInvalidCard, i, iface.URL, err)
```

sentinel 用 `%w` 包了，但 cause 用 `%v` —— 底层错误（`url.Error` 等）无法通过 `errors.Is/As` 取到。

法则：「包装错误一律 `%w`（才能 `errors.Is/As`）」。

**仓库内已有正确范式**，Core 用的是多 `%w`：

```go
// core/jsonschema/schema.go:51
return Schema{}, fmt.Errorf("%w: derive %v: %w", ErrInvalid, typeOf, err)
```

---

## B9 · 两处 `fmt.Errorf` 包裹常量字符串

```go
// core/vectorstore/filter/visitor.go:26
return fmt.Errorf("filter: accept visitor: visitor is nil")

// agent/process_start_outcome.go:120
return fmt.Errorf("invalid Process start outcome")
```

前者正是法则点名的反例：

> ❌ 手写 `fmt.Errorf("xxx is nil")` —— 用 `errors.New`。

改用 `errors.New` 即可。注意 `agent/process_start_outcome.go:128` 同一函数内用的是正确写法（`context.WithoutCancel(contextOrBackground(ctx))`），说明只是漏改。

---

## B10 · nil-ctx 反应式兜底

全仓 10 处，其中 4 处是静默 coerce：

```go
agent/observation.go:120        ctx = context.Background()
agent/process.go:432            return context.Background()   // contextOrBackground
agent/platform/selection.go:101 ctx = context.Background()
otel/agent/observer.go:176      ctx = context.Background()
```

另 6 处是 context 值查找返回 `zero, false`（`core/history/conversation.go:63`、`agent/interaction/*.go` 等），性质较轻。

按第二法则，静默 coerce 属于「reactive 地 coerce 掩盖上游的错误状态」。Go 惯例是永不传 nil Context —— 传了就是调用方 bug，兜底会让它永久隐形。

---

# C. 反 Go 范式

> C1 / C2 / C3 都是 [A1](#a1--core-协议-dto-用指针语义与-agent-的值语义构成同仓两套矛盾约定) 的具体表现，修 A1 会一并消除。此处单列以便定位。

## C1 · 不可变构造器返回指针

97 个 `New*` 返回指针。其中**有状态对象返回指针是正确的**（`NewEngine`、`NewRegistry`、`NewDispatcher`、`NewStore`、`NewPlatform`…）；属于反范式的是协议 DTO 构造器，清单见 [A1 影响面](#影响面必须先评估再动)。

## C2 · 185 处防御式 receiver nil 守卫

指针语义的直接代价。分布见 [A1](#事实)。

典型形态：一个被文档描述为「不可变 AST」的类型，每个访问器都要 `l != nil &&`（`core/vectorstore/filter/literal.go:69-72`）。

## C3 · 值接收者内取址调用指针方法

```go
// core/chat/response.go:75
if err := (&r).validate(); err != nil {
```

`(&r).validate()` 说明该类型的指针/值边界本身是混乱的 —— 方法集被迫在两侧摇摆。这是 A1 的症状之一。

## C4 · 接收者命名不一致

```
tools/web/brave.searchRequest: [request, s]
tools/web/jina.searchRequest:  [request, s]
```

同一类型的方法用了两个不同的接收者名。Go 惯例要求一致（`go vet` 不检查，`golangci-lint` 当前配置也不检查）。

全仓仅此 2 处，其余类型全部一致。

---

# 附录 D：集成性模块（低优先级）

## D1 · 21 家 vectorstore 的 `Search` 前 17 行逐字相同

克隆检测出的最大簇。这 17 行**全部是 core 契约调用**，无一行后端逻辑：

```go
var docs []*vectorstore.SearchResult
if err = req.Validate(); err != nil {
	return nil, fmt.Errorf("<provider>.Store.Search: %w", err)
}
defer func() {
	if err == nil { err = response.ValidateFor(req) }
}()
var vector []float64
vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
if err != nil {
	return nil, fmt.Errorf("<provider>: embed query: %w", err)
}
```

**需要与 models 家族的重复区分开**：models 的重复是法则明确保护的（「跨不同 wire protocol 共享 mapper = 虚假 DRY」，因为各家 shape 真的不同）。这一簇相反 —— 21 家 shape 可证明完全一致，差异只有错误串里的 provider 名。且抽取不违反 sibling-import 禁令：落点在 `core/vectorstore`，21 家全都已 import。

**已逐家核对，目前无漂移**（21 家均 `req.Validate` = 1、`defer ValidateFor` = 1）。但没有任何机制保证它继续整齐 —— `storetest.Run` 按设计只测 pre-I/O 拒绝，不覆盖这段。`ValidateFor` 语义一旦变更，21 个文件必须齐步改，漏一个静默失效。

抽取时需参数化的真实例外：`bedrockkb`、`vectara` 是服务端 embedding，没有 `EmbedText` 那三行。

## D2 · 集成模块验证深度

- **vectorstores**：24 provider、20.7k 行源码 / 3.1k 行测试。`core/vectorstore/storetest/doc.go:21` 明确声明「输出等价性 NOT 覆盖」。全家族**无任何集成测试**（无 testcontainers、无 env 门控）。多数 provider 的输出断言只有 2-3 个手挑用例（如 elasticsearch：1114 行源码 / 98 行测试）。算子映射错误、或 AND 里嵌 OR 少括号，能通过全部现有测试。
- **覆盖率**：`models/bedrock` 39.7%、`models/google/vertexai` 40.0%、`historystores/cosmosdb` 22.0%、`historystores/mongodb` 29.3%。共享 wire 层 `models/protocol/openai` 63.1% —— 这层爆炸半径最大。

---

# 附录 E：已核查为干净的维度

以下维度已实测，**无需重复排查**：

| 维度 | 结果 |
|---|---|
| `go vet` + `go test` | 85 / 85 模块全绿 |
| TODO / FIXME / HACK / XXX | **0** |
| 死公共 API | **0**（agent 237 + core 208 + 其余 6 模块共 459 个导出符号，全部有模块外消费者）※ |
| 魔法数字 | 基本为 0（仅权限位 `0o600` 等） |
| 吞掉的错误 `_ = ` | 仅 8 处，多数有注释说明理由 |
| `any` / `interface{}` 结构体字段 | 仅 2 处（`agent/execution_boundary.go:33` panic 载荷、`mcp/reverse.go:14` 协议边界） |
| Java 味命名（Impl/Manager/Helper/Service/Factory） | 0 |
| `GetX()` getter | 0 |
| `context.Context` 存进 struct 字段 | 0 |
| `init()` 函数 | 0 |
| 可变包级状态 | 0 个可变全局。317 个包级 var 中 248 个是 sentinel error，其余为编译期断言 `_`、正则、escaper、context key、reflect type 等 init 后只读值 —— **例外是 A6 的 3 个全局 tracer**，它们读取全局 TracerProvider，属隐藏全局依赖 |
| 错误串首字母大写或带结尾标点 | 0 |
| 混用指针/值接收者 | 0 个真实问题（全部是 `UnmarshalJSON` 指针 + 其余值，标准 Go 形态） |
| 返回 channel 的流式函数 | 0（全部用 `iter.Seq2`，符合法则） |
| 导出函数返回未导出类型 | 0 |
| `for` 循环内 `defer` | 0 个真实问题（见附录 F） |
| 长函数裸 return | 0 个真实问题（见附录 F） |
| 架构门禁 | `dev/repoarch` 1989 行可执行测试，覆盖模块分层 / 依赖无环 / core 不 import OTel / provider 依赖岛 / 27 条退休布局 |

※ 该检查基于符号名全仓 grep，存在同名符号造成的假阴性可能，结论为「无明显死公共面」。

---

# 附录 F：扫描误报记录

以下曾被自动扫描标记，**经人工核实为正确代码**，不应作为改动依据：

1. **`tools/fs/patch.go:187` 循环内 `defer unlock()`**
   有意为之。所有路径锁必须持有到函数返回，代码有注释说明「Both endpoints of a move are locked」。正确。

2. **`agent/planning/action.go:109` 裸 return**
   该 `return` 位于 `defer func(){ recover() }()` 闭包内部，是闭包的返回，不是函数裸返回。分析器误判。

3. **IDE 报告的 `method must have no type parameters` 等约 30 条编译错误**
   由**泛型方法**（Go 1.27 新特性，如 `metadata.Map.Decode[T]`、`agent.Input.Decode[T]`）触发的旧版本解析器假阳性。实测 85 模块 `go vet` 全过。
   **但值得留意**：golangci-lint / gopls 生态对泛型方法的支持尚未跟上，会影响本地开发体验与 IDE 可用性。

4. **models / historystores 家族的大量重复块**
   `models/*/api.go`（16 家相同）、`historystores/*/storage.go`（6 家相同 `lo.MapErr`）等，属于法则明确保护的 provider 依赖岛设计，**不是坏味道**。

---

## 建议处理顺序

**第一批 —— 无公开 API 影响，可直接做**

1. **B1**（`quiesceTree` 所有权）—— 唯一有不可恢复后果的问题，改动局限单函数
2. **B2 / B8 / B9**（`context.WithoutCancel`、`%w`、`errors.New`）—— 局部修正
3. **B3 / B4 / B7**（并发不变量注释、幂等机制统一、字段分区注释）—— 纯注释与局部一致性，可与 2 合批
4. **A5**（覆盖率闸门重设）—— 改脚本即可

**第二批 —— 需先裁决，再动**

5. **A6**（rag / mcp / a2a 自埋点）—— **先裁决条款冲突**：这三个模块算「领域模块」还是「integration」。裁决后要么迁 decorator、要么改法则文本；无论哪种，都应同步把 OTel 禁令的门禁范围从「仅 core」扩到 layer-1
6. **A3**（delta context 链）—— 先决定「透传 ctx」还是「删掉死参数」
7. **A2**（`samber/lo`）—— 先定 owner 落点（`core/internal` 不可跨 module 导出，需评估）

**第三批 —— 破坏性公开 API，需方案 + 分批迁移**

8. **A4**（`Executor` 拆接口）—— 参照 `core/history` 形状
9. **A1**（协议值语义）—— 影响面最大，参照 `agent` 模块，按 modality 逐包迁移

**最后**

10. **附录 D** —— 集成性模块（models / vectorstores / historystores）

---

## 附：本次审计用到的一次性工具

结构度量与 Go 范式扫描用的 AST 分析器在 `/tmp/scopesmell/`（未入库）。若需复现：它按目录扫描并报告函数长度 / 参数数 / 嵌套深度 / 分支数、struct 字段数、接口方法数、`any` 字段、包级 var，以及接收者一致性、裸 return、循环内 defer、channel 返回、`New*` 返回指针、导出函数返回未导出类型。

克隆检测用的是「归一化 8 行窗口哈希」的 shell 管道，阈值为「同一窗口出现 ≥ 3 次」。
