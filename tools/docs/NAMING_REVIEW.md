# `tools/` 命名 review

扫完 `fs/` / `bash/` / `httpreq/` / `webfetch/` / `websearch/` /
`fakeweather/`。**整体相当干净**，主要问题是 `*Request` / `*Response`
+ `*Input` / `*Output` 两种命名方式共存。

---

## 1. `*Request` / `*Response` vs `*Input` / `*Output` 不统一 🟡

`tools/fs/`、`tools/bash/`、`tools/webfetch/`、`tools/websearch/` 等
内部多个工具有两套命名：

```go
// tools/fs/fs.go — 用 Input/Output
type ReadInput  struct { ... }
type ReadOutput struct { ... }
type WriteInput / WriteOutput / EditInput / EditOutput / GlobInput / GlobOutput / GrepInput / GrepOutput

// tools/fs/edit.go / glob.go / grep.go / read.go / write.go — 又用 Request/Response
type EditRequest  struct { ... }
type EditResponse struct { ... }
type GlobRequest / GlobResponse / GrepRequest / GrepResponse / ReadRequest / ReadResponse / WriteRequest / WriteResponse
```

**问题**：同一个包内，`EditRequest` 和 `EditInput` 并存 — 一个是
Tool 调用入参的"Tool definition"层用的（Request/Response），一个是
Executor 接口层用的（Input/Output）。**语义有别但命名差异不清楚**。

**建议**：统一到一种命名。两种选择：

- **方案 A**：Tool 层都用 `*Request/*Response`，Executor 层都用
  `*Input/*Output`。明示分层。当前其实就是这样，**只需 doc 注释明确**。
- **方案 B**：全部统一成 `*Input/*Output`（更 Go — `http.Request` /
  `http.Response` 是 HTTP 专有，工具调用更近"函数 in/out"语义）。

推荐方案 A + 加 doc 解释，因为 grep 看 EditRequest 一眼难分层。

**调用面**：内部多个文件，统一改不算贵。

---

## 2. 各 tools 包的 `Tool` 类型都同名 — 设计意图

`tools/fs/edit.go`、`glob.go`、`grep.go`、`read.go`、`write.go` 各有：

```go
type EditTool struct { inner *core.Tool }   // edit.go
type GlobTool struct { inner *core.Tool }   // glob.go
type GrepTool struct { ... }                // grep.go
type ReadTool struct { ... }                // read.go
type WriteTool struct { ... }               // write.go
```

`tools/bash/tool.go`、`tools/httpreq/tool.go`、`tools/webfetch/tool.go`
等：

```go
type Tool struct { ... }   // 单包单工具
```

**评价**：
- 单包单工具 → `type Tool` ✓ 用 `fs.ReadTool` / `bash.Tool` 都不口吃
- 多工具 → 区分名是必要的

唯一可改的：`fs.EditTool` / `fs.GlobTool` etc. 的 `Tool` 后缀，配合
包名 `fs` 不算口吃，但**调用方写 `fs.EditTool` 比 `fs.Edit` 啰嗦**。

**建议（边界）**：
- `fs.EditTool` → `fs.Edit` （单字短名）
- 同理 `fs.GlobTool` → `fs.Glob` 等

注意：会和 `fs.NewEditTool(executor)` 等 constructor 同名命名空间，
检查冲突。

**判定**：保留也行，命名一致就好。改不改取决于审美。**P3**。

---

## 3. `httpreq.Tool` 命名 ✅

`tools/httpreq/tool.go:17`

```go
type Tool struct { ... }
```

**评价**：包名 `httpreq` 已经说明是 HTTP 请求工具，类型用 `Tool`
简洁，外部 `httpreq.Tool` 无口吃。保留 ✓

---

## 4. `tools/webfetch/firecrawl/` 等子包内 `*ResponseFormat` 枚举 ✅

`tools/webfetch/firecrawl/firecrawl.go:7`

```go
type ResponseFormat string
const (
    ResponseFormatMarkdown ResponseFormat = "markdown"
    ResponseFormatHTML     ResponseFormat = "html"
)
```

**评价**：string typedef 枚举，前缀化常量符合 Go 习惯（与
`net/http.MethodGet` 同风格）✓

---

## 5. `*Provider` 接口命名 ✅

`tools/webfetch/webfetch.go:52` / `tools/websearch/websearch.go:106`

```go
type Provider interface { ... }
```

**评价**：`webfetch.Provider` / `websearch.Provider` 无口吃，命名清楚。

---

## 6. `fakeweather/` 数据载体全部 public 字段 ✅

```go
type Coordinates struct { ... }
type Temperature struct { ... }
type Wind / Precipitation / AirQuality / UVIndex / Astronomy /
     TimeRange / HourlyForecast / Alert struct { ... }
```

**评价**：所有数据载体都是 public 字段直接暴露，无 getter ✓

---

## 不动 / 已经 OK 的

- `Executor` 接口名 (`fs.Executor` / `bash.Executor`) ✅
- `LocalExecutor` 实现 ✅
- `Recency` / `GrepOutputMode` / `ResponseFormat` string 枚举 ✅
- 各 `*Config` 结构 ✅
- 所有数据载体都已 public 字段 ✅
- 零 Get/Set 前缀的 lynx 自定义代码 ✅
- 零 ToString/InfoString ✅
- 零 stutter ✅

---

## 优先级建议

**P1 — 中等代价**
1. 把 `*Request`/`*Response` vs `*Input`/`*Output` 两套命名在
   `tools/fs/doc.go` 加注释说明分层语义；或统一改成一种（推荐
   `*Input`/`*Output`）

**P3 — 审美讨论**
2. `fs.EditTool` / `GlobTool` 等 `*Tool` 后缀去掉，调用方写 `fs.Edit`
   等更简洁。需要看构造函数命名是否冲突。

---

## 体检命令

- `go test ./tools/...` — 应全绿
- `grep -rnE "^type \w+(Request|Input|Output|Response)\b" --include="*.go" tools/`
  - 看清两套命名分布
