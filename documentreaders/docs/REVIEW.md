# `documentreaders/` — Review 阅读顺序

把外部文档（HTML / Markdown / PDF）转成 `core/document.Document`
切片。每个子目录是一个格式 reader，实现的都是
`core/document.Reader` 接口。

## 阅读顺序

1. **先看** `core/document/interface.go` 的 `Reader` 接口。
2. `doc.go` — 包说明。
3. `markdown/` — 最简单，Heading / 段落 / 代码块切分逻辑先看。
4. `html/` — 走 goquery，关注脚本 / 样式剥离。
5. `pdf/` — 最复杂，关注：
   - 文本提取保真度
   - 跨页段落合并
   - 嵌入图像处理（如果有的话）

## 每个 reader review 重点

- **元数据**：是否往 `Document.Metadata` 写入 source path / page no /
  section title 等检索友好的字段？
- **空白处理**：连续空行、换行规范化（CR/LF / NEL）。
- **错误**：错误是否带文件路径上下文（方便排查批量任务）？
- **资源管理**：reader 是否 `defer Close()`，PDF 这种巨结构尤其要看。

## 跨模块提醒

- 上游接 `rag/` — RAG 链路的入口。reader 的 chunk 形状会影响检索效果。
- 上游接 `core/document/transformer_*.go` 做后续分块。

## 体检命令

- `go test ./documentreaders/...` — 应该有 fixtures (`testdata/`)。
- `grep -rn "io.Closer" documentreaders/` — 大文件类型应该 deferred close。
