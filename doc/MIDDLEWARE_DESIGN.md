# 中间件设计：如何让 Call/Stream 不必写两份

> 针对 Lynx 当前中间件需要**同时实现 Call 和 Stream 两份逻辑**这一痛点的设计分析与落地方案。
> 关联文档：`SPRING_AI_COMPARISON.md` §2、§3；`ARCHITECTURE.md` §7。

---

## 1. 问题陈述

当前 Lynx 中间件体系：

```go
// core/model/handler.go
type CallHandler[Req, Res any] interface {
    Call(ctx context.Context, req Req) (Res, error)
}
type StreamHandler[Req, Res any] interface {
    Stream(ctx context.Context, req Req) iter.Seq2[Res, error]
}

type CallMiddleware[Req, Res any]   func(next CallHandler[Req, Res]) CallHandler[Req, Res]
type StreamMiddleware[Req, Res any] func(next StreamHandler[Req, Res]) StreamHandler[Req, Res]
```

典型中间件（如 `ToolMiddleware`）必须写两套包装器：

```go
func NewToolMiddleware() (CallMiddleware, StreamMiddleware) {
    mw := &ToolMiddleware{}
    return mw.wrapCallHandler, mw.wrapStreamHandler   // 两个入口
}
```

**痛点**：绝大多数跨切面中间件（logging、metrics、auth、rate-limit、retry、cache）在 Call 和 Stream 里逻辑 95% 雷同，却必须 copy-paste 一遍。

---

## 2. 根因分析

**类型不对称是不可调和的**：

| 方法 | 返回类型 |
|-----|---------|
| `Call` | `Res` |
| `Stream` | `iter.Seq2[Res, error]` |

没法设计一个函数签名让两者共用同一个实现体——**这是 Go 类型系统决定的硬约束**，而非抽象能力不够。

## 3. 中间件分两类

### A 类：模式无关中间件（可合并）

关心「请求→响应」的生命周期事件，不关心响应是一次性的还是流式的：

- Logging：开始/结束记日志
- Metrics：统计耗时、错误率
- Auth / Rate-limit：请求前校验
- Tracing：一对开启/关闭 span
- Retry（call only）：失败重试
- Cache（call only）：命中直返

**这类在 Call 和 Stream 里 95% 雷同**。用 copy-paste 是体力劳动。

### B 类：模式相关中间件（必须分写）

Call 和 Stream 的**执行语义本质不同**：

- **ToolMiddleware**：
  - Call：等 `next.Call` 返回完整 response，检查 tool calls，执行，递归
  - Stream：**边收 chunk 边 yield 给上游**，同时累积成完整 response 判断 tool calls，最后**重启流**递归
- **MemoryMiddleware**：对 stream 要等完整累积才能写入 memory

这类**无法合并**。强行抽象反而引入错误的中间层。

---

## 4. 设计空间

### 方案 1：Around 风格的高层抽象（仿 Spring AI `BaseAdvisor`）

```go
// core/model/around.go
type AroundMiddleware[Req, Res any] struct {
    Name  string
    Order int

    // 都可 nil；nil 即 no-op
    Before      func(ctx context.Context, req Req) (context.Context, Req, error)
    AfterCall   func(ctx context.Context, req Req, res Res, err error) (Res, error)
    AfterStream func(ctx context.Context, req Req, in iter.Seq2[Res, error]) iter.Seq2[Res, error]
}

// 展开到两种中间件
func (a *AroundMiddleware[Req, Res]) Call() CallMiddleware[Req, Res]   { ... }
func (a *AroundMiddleware[Req, Res]) Stream() StreamMiddleware[Req, Res] { ... }
```

`MiddlewareManager.UseMiddlewares` 中识别 `*AroundMiddleware` 自动展开成 Call + Stream。

**✅ 优点**：
- 分层清晰，A 类中间件只写 `Before` + 需要的 After
- Spring AI 路线忠实，架构一致性强
- 保留 `Order`/`Name` 字段，为后续 ordering / observability 预留空间

**⚠️ 代价**：
- `AfterCall` 和 `AfterStream` 仍是两个字段——诚实的妥协
- 引入一个新类型

### 方案 2：Observer 模式（纯观察）

```go
type Observer[Req, Res any] interface {
    OnRequest(ctx context.Context, req Req)
    OnChunk(ctx context.Context, req Req, res Res, err error)  // Call: 1 次; Stream: 每 chunk
    OnComplete(ctx context.Context, req Req, err error)
}

func LiftObserver[Req, Res any](o Observer[Req, Res]) (CallMiddleware, StreamMiddleware) { ... }
```

**✅ 优点**：真正写一遍，API 极简
**❌ 局限**：不能修改 request/response，只适合日志/指标/追踪；logging 之外的通用需求都兜不住

### 方案 3：Stream-first，Call 退化为聚合

删除 `CallHandler`，只保留 `StreamHandler`；`Call = Stream + 聚合到单条`。

**✅ 优点**：理论上最优雅，中间件真正只写一份
**❌ 代价**（拒绝理由）：
- Model provider 都要在内部实现 Stream→Call 聚合
- OpenAI/Anthropic 原生 Call 和 Stream 是两套 API，统一后反而绕路
- 脱离 Spring AI 架构基线，模仿关系断裂
- Chat 的 `ResponseAccumulator` 语义并非所有模态通用（音频是拼 bytes、embedding 无 stream）

---

## 5. 推荐方案

**方案 1 + 方案 2（作为方案 1 的派生）**，**不动现有** `CallMiddleware` / `StreamMiddleware`：

```
┌─────────────────────────────────────────────────────────────┐
│  高层：AroundMiddleware[Req, Res]                            │
│    • Before / AfterCall / AfterStream                        │
│    • 覆盖 80% 的 A 类中间件（logging/auth/retry/cache/...）   │
│    • Observer 可作为其上的 convenience 包装                   │
├─────────────────────────────────────────────────────────────┤
│  底层：CallMiddleware / StreamMiddleware（不变）              │
│    • 保留给 B 类（ToolMiddleware、MemoryMiddleware）         │
│    • 任何极端场景的 escape hatch                             │
└─────────────────────────────────────────────────────────────┘
```

### 这样的好处

1. **B 类零改动**：ToolMiddleware、MemoryMiddleware 不用动，继续用底层 API
2. **A 类大瘦身**：logging / auth / retry 从 2×wrapper 降到 1 个 struct
3. **向后兼容**：老代码不受影响，AroundMiddleware 是**新增选项**
4. **为 §SPRING_AI_COMPARISON.md §2.4 Ordering 铺路**：`Order` 字段天然对应 Spring AI 的 `Advisor.getOrder()`
5. **与 Spring AI `BaseAdvisor` 心智模型对齐**：便于用户类比理解

### 使用示例对比

**改造前（双份）**：

```go
type LoggingMiddleware struct { logger *log.Logger }

func (l *LoggingMiddleware) WrapCall(next CallHandler) CallHandler {
    return CallHandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
        l.logger.Printf("request: %v", req)
        res, err := next.Call(ctx, req)
        l.logger.Printf("response: %v, err=%v", res, err)
        return res, err
    })
}

func (l *LoggingMiddleware) WrapStream(next StreamHandler) StreamHandler {
    return StreamHandlerFunc(func(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
        l.logger.Printf("stream request: %v", req)
        return func(yield func(*Response, error) bool) {
            for chunk, err := range next.Stream(ctx, req) {
                l.logger.Printf("chunk: %v, err=%v", chunk, err)
                if !yield(chunk, err) { return }
            }
        }
    })
}
```

**改造后（单份）**：

```go
var LoggingMiddleware = &AroundMiddleware[*Request, *Response]{
    Name: "logging",
    Before: func(ctx context.Context, req *Request) (context.Context, *Request, error) {
        log.Printf("request: %v", req)
        return ctx, req, nil
    },
    AfterCall: func(ctx context.Context, req *Request, res *Response, err error) (*Response, error) {
        log.Printf("response: %v, err=%v", res, err)
        return res, err
    },
    AfterStream: func(ctx context.Context, req *Request, in iter.Seq2[*Response, error]) iter.Seq2[*Response, error] {
        return func(yield func(*Response, error) bool) {
            for chunk, err := range in {
                log.Printf("chunk: %v, err=%v", chunk, err)
                if !yield(chunk, err) { return }
            }
        }
    },
}
```

---

## 6. 落地步骤

### Step 1：新增 `core/model/around.go`（~80 行）
- 定义 `AroundMiddleware[Req, Res]` struct
- 实现 `Call() CallMiddleware[Req, Res]`
- 实现 `Stream() StreamMiddleware[Req, Res]`
- 处理 nil-hook 的 passthrough

### Step 2：`MiddlewareManager.UseMiddlewares` 识别展开
让 `UseMiddlewares(middlewares ...any)` 的类型 switch 多识别一种：

```go
case *AroundMiddleware[Req, Res]:
    m.callMiddlewares   = append(m.callMiddlewares, v.Call())
    m.streamMiddlewares = append(m.streamMiddlewares, v.Stream())
```

### Step 3：补充示例 + 文档
- `core/model/around_example_test.go` 展示 logging/metrics 用法
- 在 `ARCHITECTURE.md` §7「Middleware：三种实例，没有共同祖先」段落更新：现在有了高层统一抽象

### Step 4：**不做**的事
- **不动** `ToolMiddleware` / `MemoryMiddleware`（它们本就该用底层 API）
- **不删** `CallMiddleware` / `StreamMiddleware`
- **不引入** Observer 新接口（先把 AroundMiddleware 跑通再说）

---

## 7. 与 Spring AI 的对照

| 维度 | Spring AI | Lynx（改造后） |
|-----|-----------|----------------|
| 底层接口 | `CallAdvisor` / `StreamAdvisor` | `CallMiddleware` / `StreamMiddleware`（保留） |
| 高层抽象 | `BaseAdvisor` 默认 `adviseCall/adviseStream` 委托给 `before/after` | `AroundMiddleware` 用 struct 字段 + 展开方法 |
| Ordering | `Ordered.getOrder()` | `AroundMiddleware.Order`（待 `MiddlewareManager` 消费） |
| Name | `Advisor.getName()` | `AroundMiddleware.Name` |
| 共享 context | `ChatClientRequest.context` + `ChatClientResponse.context` | `Request.Params`（单向） |

改造后，Lynx 对 Spring AI 的 `BaseAdvisor` 层做了 Go 风格实现：用 struct + 可选函数字段代替 Java 的抽象类 + default methods，语义等价。

---

## 8. 核心判断

- **「Call/Stream 合并到一个函数」在 Go 类型系统下不可行**——接受这个前提
- **但 90% 的中间件根本不需要真合并，只需要「少写一份」**
- **`AroundMiddleware` 用 struct 字段让「少写一份」从仪式变成自然写法**
- **底层 `CallMiddleware`/`StreamMiddleware` 保留**作为 escape hatch，支撑 ToolMiddleware 这类 B 类场景

这是**承认类型不对称、然后用便利层消解痛点**的工程解法，不是架构革命。

---

## 9. 决策状态

- [ ] 方案已确认
- [ ] `core/model/around.go` 实现
- [ ] `MiddlewareManager.UseMiddlewares` 接入
- [ ] 示例补齐
- [ ] `ARCHITECTURE.md` §7 更新

— 文档版本 v1（2026-04-17）
