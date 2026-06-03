# `mcp/` 命名 review

扫完结论：**mcp/ 包大体干净**。没有 Get/Set 前缀、没有 InfoString
之类 Java toString、没有口吃。下列是少数边界 / 可优化点。

---

## 1. `mcp.Tool` / `mcp.ToolConfig` — 与 SDK 同名

`mcp/tool.go:24`

```go
type Tool struct { ... }       // lynx 自己的 wrapper
type ToolConfig struct { ... }
```

**问题**：`sdkmcp.Tool` 是 SDK 类型，`mcp.Tool` 是 lynx 自己包装出来
满足 `chat.Tool` 接口的桥接器。外部读者看 `mcp.Tool` 容易和 SDK 的
`mcp.Tool` 混淆（虽然导入路径不同，但快速一眼看代码会迷惑）。

**建议（边界）**：
- 保留：lynx 的 `mcp` 包就是包装 SDK，类型同名是自然映射，go.mod
  路径已经把两个 `mcp` 分开，**实战很少出错**。
- 或者改名 `mcp.ToolAdapter` / `mcp.LynxTool`（太冗长，不推荐）。

**判定**：⚪ 保留，但建议在包顶 `doc.go` 加一句注释说明"`mcp.Tool`
是 SDK Tool 的 lynx 包装"。

---

## 2. `mcp.Provider` / `mcp.Source` — 命名清晰，但有微妙

`mcp/provider.go:79` / `provider.go:32`

```go
type Provider struct { ... }
type Source struct { Name string; Session *sdkmcp.ClientSession }
```

**问题**：`Provider` 在 lynx 全局也有 `embedding.Provider` 等命名。
单独看不混淆，组合使用时（一个文件同时 import 多个）可能。

**判定**：保留。Go 里 `Provider` / `Source` 都是常见词，包前缀消歧
足够。

---

## 3. `NamingFunc` / `MetaFunc` ✅

`mcp/provider.go:19` + `mcp/meta.go:16`

```go
type NamingFunc func(sourceName string, tool *sdkmcp.Tool) string
type MetaFunc func(ctx context.Context) sdkmcp.Meta
```

**评价**：函数类型用 `*Func` 后缀符合 Go 习惯。保留 ✅

---

## 4. `mcp.SamplingHandler` — 类型别名 ✅

`mcp/sampling.go:15`

```go
type SamplingHandler = func(...) (...)
```

**评价**：类型别名，OK。直接用函数签名等价于一阶引用，Go 风格 ✅

---

## 5. `mcp.HTTPServerOptions` / `HTTPClientOptions` / `CommandClientOptions`

`mcp/transport.go:16,75,136`

**评价**：HTTP 大写初始词、`*Options` 后缀。两个都符合 Go 风格 ✅

唯一可斟酌的：包内同时存在 `HTTPClientOptions` 和
`CommandClientOptions`，对称命名 OK。也可以考虑统一前缀分类，但当前
方式已经清楚。**保留**。

---

## 6. `mcp.ToolCallError` 数据载体 ✅

`mcp/errors.go:37`

```go
type ToolCallError struct { ... }
```

**评价**：public 字段（error 实例），符合"数据载体不要 getter"规则 ✅

---

## 7. `mcp.ElicitOptions` — 已经 ✅

`mcp/sampling.go:40`

```go
type ElicitOptions struct { ... }
```

**评价**：`*Options` 后缀、public 字段。Go 风格 ✅

---

## 不动 / 已经 OK 的

- 所有公开类型都没有 stutter（`mcp.MCPFoo` 这种）
- 所有公开类型都没有 `Get*` / `Set*` getter
- 所有数据载体（Config / Options / Source）都是 public 字段直接暴露
- `Provider` 的 `Tools(ctx)` / `Invalidate()` / `OnToolListChanged`
  方法名简洁有意义
- `RegisterTools` / `NewStreamableHTTPHandler` / `DialStreamableHTTP`
  / `DialCommand` 函数名清楚 ✅

---

## 优先级建议

**P3 — 可选讨论**
1. 在 `mcp/doc.go` 顶部加一行注释，说明 `mcp.Tool` 是 SDK Tool 的
   lynx wrapper，避免后续维护者跟 `sdkmcp.Tool` 混淆。

**其他**：**无需调整**。本包是当前 lynx 命名最干净的子包之一。

---

## 体检命令

- `go test ./mcp/...` — 应全绿
- `grep -rnE "(Get|Set)[A-Z]" --include="*.go" mcp/` — 应只匹配 SDK
  调用 (SetAttributes / GetProgressToken / GetPrompt 等)，零 lynx
  自定义
