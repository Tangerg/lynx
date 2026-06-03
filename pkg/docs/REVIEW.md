# `pkg/` — Review 阅读顺序

`pkg/` 是 lynx 最底层的工具集，**没有任何内部依赖**，所有上层模块都引用它。
阅读顺序按 "原子工具 → 容器 → IO → 控制流" 推进，标记 **[精读]** 的文件
要逐行看。

## 1. 原子 / 单元工具（依赖为零，关注分配与签名稳定性）

1. `assert/` — 测试内断言。先扫一遍；下面几乎所有 pkg 子包都在测试里用它。
2. `ptr/` **[精读]** — `Ptr(v)` / `Deref(p, def)` 等指针助手。重点看是否
   有意外的堆逃逸，`core/model/*` 的热路径会反复用到。
3. `result/` **[精读]** — `Result[T]` 求和类型，是 `pkg/stream` 的契约
   基础。检查 `Ok` / `Err` 是否穷尽覆盖所有路径。
4. `safe/` — `recover` 包装。看一眼，确认没有静默吞掉 panic。

## 2. 容器 / 集合工具

5. `slices/` — slice 助手 (chunk / unique / partition)。索引算术看一下；
   这里一个 off-by-one 会传到 `core/document` 与 `vectorstores`。
6. `maps/` — map 助手，审视角度同上。
7. `sets/` — `map[T]struct{}` 包成 Set 语义。验证零值安全。

## 3. IO + 解析

8. `io/` — `Reader` / `Writer` 扩展、行计数、前缀读取。`documentreaders`
   用得最多。
9. `bufio/` — 大行容忍的 buffered scanner。`lyra` 的 JSONL message store
   用到。
10. `json/` **[精读]**
    - `schema.go` — JSON-Schema 助手 (Tool InputSchema 用)。对照
      OpenAI / Anthropic 的 tool definition 格式核对。
    - `stream_parser.go` — 增量 JSON 解析器，`models/` 流式适配器用到。
11. `text/` / `strings/` / `xml/` — 小工具集，扫读。
12. `mime/` — Content-Type 嗅探。

## 4. 数值 / 格式

13. `math/` — 数值助手。
14. `dataunit/` — 字节大小解析 ("4MB" → 4*1024*1024)。
15. `random/` — 随机数原语 (crypto vs math)。

## 5. 并发 + 控制流（重点）

16. `sync/` **[精读]**
    - `future.go` — 可等待的"值或错误"。
    - `limiter.go` — 并发上限。
    - `pool.go` — 对象池。
    重点验证 ctx 取消语义，`agent/runtime` 依赖它。
17. `stream/` **[精读]**
    - `stream.go` / `ops.go` — 类型化迭代器 (`iter.Seq[T]` 友好)。
      流式聊天响应全靠它。看清反压语义。
18. `retry/` **[精读]** — 重试策略 (固定 / 指数 / 抖动)。**Lyra 不再加
    重试层** — SDK 自带的够用了。看 `agent/runtime` 用了哪些 policy。

## 6. 进程 / 系统

19. `system/` — 进程 + 环境变量助手。

## 跨模块提醒

- `pkg/` 是最稳定的地基 — 签名一旦改，下游全炸。当作 v1 冻结对待。
- `pkg/stream` 的语义是流式聊天响应的"承重墙"
  (`core/model/chat/response_accumulator.go`)。
- `pkg/retry` 是**唯一的重试层** — 如果在下游看到第二个重试实现，标记
  为重复抽象需要清理。

## 体检命令

- `find pkg -name "*_test.go" | wc -l` — 应 ≥ 30，每个助手都有测试。
- `go test ./pkg/...` — 跑得快 (< 10s)；失败即回归。
