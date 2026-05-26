# `core/` 命名 review

整套扫完 core/ 全部子包（`model/` / `model/chat/` / `model/embedding/`
/ `model/audio/` / `model/image/` / `model/moderation/` / `document/`
/ `media/` / `tokenizer/` / `vectorstore/` / `evaluation/`）后整理。
按"问题严重度"排。

---

## 1. `model.ApiKey` 接口名违反初始词大小写约定 ✅ DONE

`core/model/api_key.go:19`

```go
type ApiKey interface { Get() string }
func NewApiKey(value string) ApiKey { ... }
```

**问题**：`Api` 不是英文单词而是初始词 (API = Application Programming
Interface)。Go effective-go 明确规定初始词全大写或全小写，从不写成
`Api` / `Url` / `Id`。

仓库内已经有 62 处正确使用 `APIKey`（例如 `JinaAPIKey` /
`TavilyAPIKey` / `cfg.APIKey`）；而 `model.ApiKey` 类型本身却是 380
处 `ApiKey` — 类型定义跟惯例反着写。

**建议**：rename
- `ApiKey` → `APIKey`
- `NewApiKey` → `NewAPIKey`
- `staticApiKey` → `staticAPIKey`
- `maskApiKey` → `maskAPIKey`

**调用面**：380 处，遍布 `models/` 全部 provider + lyra + agent 测试。
机械替换 (perl one-shot)。

---

## 2. `embedding.GetDimensions(...)` 顶层 Get 前缀 🟡

`core/model/embedding/dimensions.go:25`

```go
func GetDimensions(ctx context.Context, model Model) int64 { ... }
```

**问题**：Go effective-go 反对 `Get` 前缀的访问器。这是顶层函数不是
方法，但语义还是"探测并返回 dimension 数"。

**建议**：
- `embedding.Dimensions(ctx, model) int64` — 简洁
- 或 `embedding.ProbeDimensions(...)` — 强调它会真的发请求
- 或保留但删 `Get` 前缀 → `embedding.Of(...)` 太短

推荐 `Dimensions`：调用方写 `embedding.Dimensions(ctx, m)` 自然。

**调用面**：单点函数，搜下就知道。

---

## 3. `chat.MessageToString` / `chat.MessagesToStrings` — Java-ish

`core/model/chat/message.go:668`

```go
func MessageToString(message Message) string { ... }
func MessagesToStrings(messages []Message) []string { ... }
```

**问题**：`ToString` 是 Java 写法。Go 用 `fmt.Stringer.String()`，或
独立函数命名为动词或目标名词。

**建议**：
- `MessageToString` → `FormatMessage` / `RenderMessage`
- `MessagesToStrings` → `FormatMessages` / `RenderMessages`

或者让 `Message` 接口约束实现 `String() string`（如果还没的话），调用
方直接 `fmt.Sprint(msg)`。

---

## 4. `vectorstore/filter/ast.Literal.AsString` — Java-ish

`core/vectorstore/filter/ast/ast.go:74`

```go
func (l *Literal) AsString() (string, error) { ... }
func (l *Literal) AsInt()    (int64, error)  { ... }   // 类似的
```

**问题**：`AsX` 是 Java/Kotlin / Scala 的强转命名（`.asInstanceOf[T]`
风）。Go 习惯用 `XValue` 或不加前缀。

**建议**：
- `AsString` → `StringValue` （Go std `reflect.Value.String()` 风）
- 或保留但接受这是 AST 节点的惯例（PostgreSQL pgquery / GraphQL AST
  在 Go 实现里也常用 `AsX`），属于边界场景。

**判定**：保留 `AsX` 也行（AST 包外部用户少；类型断言式命名 reads
意图清楚）。可降为 P3。

---

## 5. `media.Media.DataAsBytes` / `DataAsString` — Java-ish

`core/media/media.go:120-148`

```go
func (m *Media) DataAsBytes()  ([]byte, error) { ... }
func (m *Media) DataAsString() (string, error) { ... }
```

**问题**：与 §4 同根。

**建议**：
- `DataAsBytes` → `Bytes` （`Media.Bytes()`）
- `DataAsString` → `Text` 或 `String` 但与 `fmt.Stringer` 冲突

推荐：`Bytes()` + `Text()`（明确意图）。

---

## 6. 包名口吃 — 几处边界情况

### 必然口吃但是行业惯例，**保留**

| 类型 | 外部读 | 评级 | 行业先例 |
|---|---|---|---|
| `media.Media` | `media.Media` | ⚪ keep | 类似 `bytes.Buffer` 这种"包就是干这事的"模式 |
| `document.Document` | `document.Document` | ⚪ keep | 同上 |
| `model.Model` | `model.Model` | ⚪ keep | 同上 (`time.Time`) |
| `tokenizer.Tokenizer` | `tokenizer.Tokenizer` | ⚪ keep | 同上 |
| `evaluation.Evaluator` | `evaluation.Evaluator` | ⚪ keep | 同上 |

这类"包名 = 主类型名"在 stdlib 里也很多 (`time.Time` / `errors.Error`
等)，是 Go 公认的"轻度口吃可接受"。**全部保留**。

### `model.ModelMetadata` 🟡 真口吃

`core/model/chat/model.go:39` + `core/model/embedding/model.go:40`

```go
type ModelMetadata struct { ... }
```

**问题**：`chat.ModelMetadata` / `embedding.ModelMetadata` 都用此命名。
但从 `core/model` 包导入视角，实际写 `chat.ModelMetadata` 时 `Model`
前缀已经在子包名上层（模板路径是 `core/model/chat`），所以从 `chat`
包内的 `ModelMetadata` 角度其实**不口吃**。

**判定**：保留。

### `model.MiddlewareManager` 🟡

`core/model/middleware.go:28`

```go
type MiddlewareManager[Req, Resp any] struct { ... }
```

**问题**：`Manager` 后缀是 Java 味（Spring / J2EE 一抓一大把）。Go 里
更常见的命名是 `Chain` / `Stack` / 直接复数。

**建议**：
- `MiddlewareManager` → `MiddlewareChain` （直观，AOP 风）
- 或 `Middlewares` （复数名词，简洁）

**调用面**：需要确认实际用法。

---

## 7. `model.CallHandler` / `StreamHandler` / `*Func` 命名 ✅

```go
type CallHandler[Req, Resp any] interface {
    Call(ctx context.Context, req Req) (Resp, error)
}
type CallHandlerFunc[Req, Resp any] func(...)
```

**评价**：标准 Go 命名（参考 `http.Handler` + `http.HandlerFunc`）。保留。

---

## 不动 / 已经 OK 的

- `chat.ClientRequest` / `ClientStreamer` / `ClientCaller` / `Client` —
  builder + 三个角色，命名清楚 ✅
- `chat.JSONParser` / `ListParser` / `MapParser` / `AnyParser` —
  结构化输出 parser 家族，命名一致 ✅
- `chat.PromptTemplate` / `MessageParams` / `ResponseMetadata` —
  数据载体直接 public 字段 ✅
- `document.Reader` / `Writer` / `Transformer` / `Formatter` /
  `Batcher` / `Splitter` — 单方法接口 + `-er` 后缀，Go 习惯 ✅
- `vectorstore.Retriever` / `Creator` / `Deleter` — 同上 ✅
- `model.CallMiddleware` / `StreamMiddleware` — `Middleware` 后缀
  Go 生态早已接受 ✅
- 所有 `*Config` 类型 — 命名一致 ✅
- `chat.FinishReason` / `MessageType` / `PartKind` 等 string typedef
  枚举 — 简洁 ✅
- `chat.Usage` / `RateLimit` / `ApiKey` 类的接口/类型 — 除 ApiKey
  初始词外，其他都 OK

---

## 优先级建议

**P0 — 低成本高价值**
1. `model.ApiKey` → `model.APIKey` (全局机械替换 380 处)

**P1 — 中等代价**
2. `chat.MessageToString` / `MessagesToStrings` → `FormatMessage` /
   `FormatMessages` (调用面 ~10 处)
3. `embedding.GetDimensions` → `embedding.Dimensions` (单点函数)
4. `media.Media.DataAsBytes/DataAsString` → `Bytes()/Text()`
   (调用面 ~5 处)

**P2 — 哲学讨论**
5. `model.MiddlewareManager` → `MiddlewareChain` / `Middlewares`
   (跨 chat / embedding / image / audio 调用面，需要先 design 讨论)
6. `vectorstore/filter/ast.Literal.AsString/AsInt` → `StringValue` 等
   (AST 包内部，保留或改皆可)

---

## 体检命令

- `go test ./core/...` — 应全绿
- `grep -rn "\bApiKey\b" --include="*.go" core/` — 验证替换面 (P0 后应为 0)
- `grep -rnE "^func [A-Z]\w*ToString" --include="*.go" core/` — 验证 ToString 清理
