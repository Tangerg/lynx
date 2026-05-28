# CLAUDE.md — documentreaders module

> Format-agnostic readers that emit `core/document.Document` streams for RAG pipelines.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

不同格式（markdown / HTML / PDF）解析成统一的 `document.Document` 流，给 RAG 索引用。每个 reader 是独立子包 + 独立 go.mod，依赖只在用到时拉。

## 技术栈

- Go 1.26.3
- 解析器各管各：`yuin/goldmark`（markdown AST）/ `PuerkitoBio/goquery`（HTML CSS 选择器）/ `ledongthuc/pdf`（PDF 文本提取）
- 依赖 `core/document` 提供 `Reader` 接口和 `Document` 类型
- ~700 LOC，3 个 reader 各占一个子包

## 核心架构

每个 reader 实现 `document.Reader.Read(ctx) ([]*Document, error)`：

- **`markdown/`** — goldmark AST 遍历 + 可选按 heading 切分（`WithHeadingSplit` / `WithSourceName` options）。heading 层级构成 path 栈
- **`html/`** — goquery 抓正文 + 元数据标记
- **`pdf/`** — 按页迭代 + page metadata

## 关键接口/类型

- `document.Reader`（来自 `core/document`）—— `Read(ctx) ([]*Document, error)`
- Functional options 模式：`markdown.WithHeadingSplit()` / `WithSourceName(str)`
- 元数据 key 都按 reader 命名空间：`markdown.heading` / `markdown.heading.level` / `markdown.source` / `html.title` / `pdf.page`

## 强约定

- **全量先读到内存**，不流式（小文档场景；大文档由调用方自己分块）
- **元数据 key 必须带 reader 前缀**（`markdown.*` / `html.*` / `pdf.*`），跨 reader 不冲突
- **每个 reader 独立 go.mod**（解析器依赖大，不让消费方拉不用的）
- markdown heading split 构建路径栈（`"# Intro > ## Architecture"`），给 LLM 看上下文用

## 关键目录

```
documentreaders/
├── markdown/    goldmark AST 走 + heading 切分
├── html/        goquery CSS 选择器
└── pdf/         ledongthuc/pdf 按页提取
```

## 常用命令

```bash
go build ./...
go test ./...
go test ./markdown/... -v   # 单 reader
```

## 修改任何东西之前

- **加新 reader（docx / epub / xlsx）**：新建 `documentreaders/<format>/`，独立 `go.mod`，实现 `document.Reader`。元数据 key 用 `<format>.*` 前缀
- **改元数据 key 命名**：跨 reader 协调；下游 RAG pipeline 可能直接读这些 key
