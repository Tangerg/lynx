# `documentreaders/` 命名 review

扫完结论：**documentreaders 全部子包（html/markdown/pdf）命名干净**。
零问题。

---

## 1. 各 reader 的 `Reader` / `Option` ✅

每个子包：

```go
// documentreaders/html/html.go
type Reader struct { ... }
type Option func(*Reader)

// documentreaders/markdown/markdown.go — 同上
// documentreaders/pdf/pdf.go — 同上
```

**评价**：
- 包名 = 格式（`html` / `markdown` / `pdf`），类型 `Reader` 无口吃 ✓
- functional options 模式（`Option func(*Reader)`）符合 Go 习惯 ✓
- 三个 reader 横向命名完全对称 ✓

外部读：`html.NewReader(...)` / `markdown.NewReader(...)` /
`pdf.NewReader(...)` — 一致体验。

---

## 不动 / 已经 OK 的

- 命名跨子包完全统一
- 零 Get/Set / ToString / stutter / Java suffix

---

## 优先级建议

**无任何调整项**。

---

## 体检命令

- `go test ./documentreaders/...`
- `grep -rn "^type [A-Z]" documentreaders/` — 看每子包都只有 Reader +
  Option
