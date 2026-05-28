# CLAUDE.md — core module

> Document / embedding / chat / vectorstore / evaluation 接口层 —— RAG / agent pipeline 的语言基础。
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

整个 lynx 生态的"协议定义层"：`Document` / `Message` / `CallHandler` / `Store` / `Tokenizer` 全在这里。**接口在 core，具体实现在外圈模块**（models / chatmemory / vectorstores / documentreaders）。

## 技术栈

- Go 1.26.3
- 关键外部依赖：
  - `pkoukk/tiktoken-go` —— token 计数（tiktoken）
  - `go.opentelemetry.io/otel` —— span 注入
  - `google/uuid` / `spf13/cast`
- 依赖 `pkg/`（工具）；**不依赖**任何业务模块
- ~13k LOC / 103 文件 / 15 包

## 核心架构

6 个一级包：

1. **`document`** —— `Document{id, text, media, metadata, formatter}` + `Reader/Writer/Transformer/Batcher/Formatter` 流水线接口
2. **`model`** —— `CallHandler[Req, Resp]` / `StreamHandler[Req, Resp]` 泛型 + `MiddlewareManager`（call / stream 两条链）
3. **`model/{chat,embedding,image,audio,moderation}`** —— 各模态的 Request/Response + 客户端
4. **`tokenizer`** —— `Encoder` / `Decoder` / `Tokenizer` + `TextEstimator` / `MediaEstimator`
5. **`vectorstore`** —— `Creator` / `Retriever` / `Deleter` / `Store` + filter mini-language（parser → AST → analyzer）
6. **`evaluation`** —— `Evaluator` 接口 + FactChecking / Relevancy / Composite

## 关键接口/类型

1. **`document.Document`** —— ID / Text / Media / Metadata / Formatter
2. **`CallHandler[Req, Resp]` / `StreamHandler[Req, Resp]`** —— 泛型，stream 返回 `iter.Seq2[Response, error]`（Go 1.23+）
3. **`chat.Message`** —— **sealed interface**：`SystemMessage` / `UserMessage` / `AssistantMessage` / `ToolMessage`，靠未导出 `message()` 方法封口（外部不能加新子类型）
4. **`vectorstore.Store` = Creator + Retriever + Deleter + Metadata** —— 责任分离
5. **`Evaluator`** —— 给一个 request + response 打分（RAG feedback loop）
6. **`document.Reader / Writer / Transformer / Batcher`** —— 可组合的 pipeline stage
7. **`model.MiddlewareManager[Req, Resp]`** —— `CallMiddleware` 和 `StreamMiddleware` 各一条链

## 强约定

- **Sealed interfaces 用未导出方法**：`chat.Message.message()` 保证 type switch 穷尽 —— 外部不能加新 Message 子类型
- **Request 必带 `Validate()`**：`ErrNilRequest` / `ErrEmptyDocuments` 等 sentinel error，调用方可 `errors.Is`
- **函数形 adapter**：`CallHandlerFunc[Req, Resp]` 类比 `net/http.HandlerFunc`
- **泛型走模态多样性**：`CallHandler[Req, Resp]` 一个骨架；chat / embedding / image / audio 各自具化
- **Streaming 用 `iter.Seq2`**，不自定义 iterator
- **Metadata = `map[string]any`**：`Document` / `Message` 都用，lazy alloc via `.Meta()`
- **Options 字段 = pointer**：`chat.Options.Temperature *float32` —— nil 表示"用 provider 默认值"
- **`MiddlewareManager.UseMiddlewares(...any)`**：接 any，运行时 type-assert 到 `CallMiddleware` / `StreamMiddleware`，允许一个 fn 同时实现两条链

## 关键目录

```
core/
├── document/          Document + Reader/Writer/Transformer/Batcher/Formatter
│   └── id/            ID 生成（SHA256 / UUID）
├── model/             CallHandler/StreamHandler + MiddlewareManager
│   ├── chat/          Message（sealed） + Request / Response + Client
│   ├── embedding/     Embedding Req/Resp + Client
│   ├── image/         图像生成
│   ├── audio/         转录 / TTS
│   ├── moderation/    审核
│   └── middleware/    具体 middleware（logger / safeguard）
├── tokenizer/         Encoder/Decoder + Estimator + tiktoken 适配器
├── vectorstore/       Store + Creator/Retriever/Deleter + filter mini-language
│   └── filter/        过滤 DSL（lexer → parser → analyzer → AST）
├── evaluation/        Evaluator + 具体实现（FactChecking / Relevancy / Composite）
├── media/             Media 容器（image / audio / file）+ JSON round-trip
└── docs/              架构 + API 文档
```

## 常用命令

```bash
go build ./...
go test ./...
go test -race ./model/...
```

## 修改任何东西之前

- **改 `chat.Message`**：sealed —— 改了所有 models/* 适配器都受影响
- **改 `vectorstore` filter AST**：所有 vectorstores/* 后端有 visitor 转方言，AST 节点变了全部 visitor 都要跟
- **改 `CallHandler` / `StreamHandler` 签名**：整个生态都依赖；不要改
- **加新模态**：复用 `CallHandler[Req, Resp]` 泛型骨架，新建 `model/<modality>/`，写自己的 Request / Response / Options

## 强反向不变量

- ❌ **`chat.Message` 加新子类型**：sealed 接口的设计意图，会破坏所有 type switch（`message()` 方法封口故意的）
- ❌ **stream 用 channel 不用 `iter.Seq2`**：Go 1.23+ 标准
- ❌ **业务逻辑放 core**：core 是协议层，具体实现在外圈
