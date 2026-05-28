# CLAUDE.md — pkg module

> Shared utility library — generics 集合、并发原语、流式处理、JSON Schema 生成。
> 项目级约定见 `../lyra/CLAUDE.md`。

---

## 一句话定位

整个 monorepo 的"工具层基础设施" —— `core` / `agent` / `models` / `vectorstores` 都依赖 pkg，**pkg 不依赖任何业务模块**（zero-cycle 关键护栏）。

## 技术栈

- Go 1.26.3（generics + Go 1.23 range-over-func iterator 全用上）
- 5 个核心外部依赖：
  - `gammazero/workerpool` / `panjf2000/ants/v2` —— goroutine pool 两套（按场景选）
  - `sourcegraph/conc` —— structured concurrency
  - `invopop/jsonschema` —— JSON Schema 生成
  - `gabriel-vasile/mimetype` —— MIME 检测
- ~9.3k LOC，22 个子包，每个一个独立 niche

## 核心架构

按职能分 6 组：

- **Collections** —— `maps`（HashMap/LinkedMap/SyncMap 统一接口）/ `sets` / `slices`（边界安全 + 分块）
- **Data types** —— `result`（Result[T]）/ `dataunit`（typed bytes B/KB/MB...）/ `text`（fluent renderer）
- **Concurrency** —— `sync`（Future / Pool / Limiter）/ `safe`（goroutine panic recovery）/ `retry`（policy 化重试）
- **Streaming** —— `stream`（typed pipe + Map/Filter/FlatMap）/ `xml`（LLM-safe element scanner）/ `json`（streaming parser + JSON Schema 生成）
- **Encoding** —— `strings`（quote / case 转换）/ `mime` / `bufio`（多格式 line scanner）
- **Primitives** —— `ptr` / `math` / `io` / `assert` / `random` / `system`

## 关键接口/类型（按调用方数量排）

| 类型 | 调用方数 | 说明 |
|---|---|---|
| `mime.MIME` | 27 | MIME 解析 / 检测 / 通配匹配 |
| `math.NumericType` 约束 | 24 | 泛型算术约束 |
| `json.StreamParser` + Schema | 13 | 流式 JSON + schema 推断 |
| `slices.*` | 12 | 边界安全 / 分块 / 迭代 |
| `ptr.To / ptr.From` | 8 | 指针字面量 / 安全 deref |

其余（`maps.Map` / `result.Result` / `stream.Stream` / `sets.Set` / `sync.Future` / `retry.Retrier` / `safe.Go`）< 5 调用方 / 个。

## 强约定

- **零业务依赖**：pkg 不 import 任何 `core/` / `agent/` / `models/` / `vectorstores/`。CI 应该锁这条（否则形成循环）
- **导出即长存**：一旦 export 的类型 / 函数永远不破坏。breaking 改动 = 新包 / 新主版本
- **Generics 强制**：所有集合用类型参数，公开 API 禁 `interface{}` / `any`
- **Iterator-first**：Go 1.23 `range-over-func` 优先于 `ForEach(func(T))` —— 调用方拿到 `break` / 提前退出能力
- **流式不缓冲**：`xml` / `json` / `stream` 优先增量处理；处理不可信输入时强制 buffer cap（防 OOM）
- **小到不必抽**：`ptr.To` 这种 < 5 行的 helper 直接放，不二次封装 stdlib
- **`-race` 必跑**：`SyncMap` / `SyncSet` / `sync.Pool` / `safe.Go` 都在 race detector 下测过

## 关键目录

```
pkg/
├── maps/         Map interface + Hash/Linked/Sync 实现
├── sets/         Set interface + Hash/Linked/Sync
├── slices/       边界安全 / 分块 / 迭代
├── result/       Result[T] for 错误流水线
├── dataunit/     typed DataSize (B/KB/MB/GB/TB, IEC 1024)
├── text/         line align / Renderer
├── stream/       typed pipe + Map/Filter/FlatMap
├── xml/          LLM-safe streaming element scanner
├── json/         StreamParser + Schema 生成
├── strings/      quote / camelCase ↔ snake_case
├── mime/         MIME 解析 / 检测 / 通配
├── bufio/        多格式 line scanner（LF/CR/CRLF）
├── ptr/          To/From/Clone helpers
├── math/         NumericType 约束 + overflow-checked 算术
├── io/           buffered read iterator
├── assert/       Must/Ensure（只用于不变量）
├── random/       math/rand/v2 wrappers（非 crypto）
├── sync/         Future / Pool / Limiter
├── safe/         goroutine panic recovery
├── retry/        Do / DoWithResult / Retrier
└── system/       平台常量
```

## 常用命令

```bash
go build ./...
go test ./...
go test -race ./...   # 并发结构必跑
```

## 修改任何东西之前

- **加新子包**：先问"为什么 stdlib 不够"——只在 stdlib 真不够 OR 跨业务模块要复用时才加
- **改 export API**：考虑兼容性，宁可加新函数也别改老的
- **加业务概念**：**不行** —— `pkg/retry/Transient` 这种分类已经被否过（见 lyra/CLAUDE.md `❌ retry layer` 反向不变量），这里只放纯工具
- **XML / JSON parser 改 buffer 上限**：跑 fuzz，处理 LLM 输出的恶意 case
