# CLAUDE.md — core module

> Lynx 生态的窄腰：只定义跨 provider 稳定共享的协议、最小调用 SPI 和纯组合算法。已完成的重构、ADR 与发布准备见 [`../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md`](../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md)；重构前公共面只作为历史基线保存在 [`../doc/CORE_API_INVENTORY.md`](../doc/CORE_API_INVENTORY.md)。

---

## 定位

- **协议，不是总框架**：Core 定义 metadata/media/document、各 modality 的 Request/Response、最小 Model 能力和高层 VectorStore 语义。ChatClient、history backend、tool runtime、agent loop、evaluation、document pipeline、tokenizer 实现和可观测性都在外圈。
- **生产代码只依赖标准库和 Core 自身包**：Core 不 import sibling module、provider SDK、tokenizer、UUID、cast 或 OTel；`internal/arch` 对该依赖预算 fail-fast，不接受临时白名单。
- **依赖方向单向**：models/vectorstores/chatclient/tools/agent/otel 可以 import Core；Core 不反向 import 它们。

## 架构心智

- **扁平领域包**：当前路径是 `core/chat`、`core/embedding`、`core/image`、`core/transcription`、`core/speech`、`core/moderation`，不使用 `core/model/<modality>` 表达 Java 式层次。
- **最小能力接口**：每个 modality 的 `Model` 默认只有 `Call`；`Streamer`、`Dimensioner` 等能力独立，provider 只实现真实支持的能力。
- **协议值可序列化**：DTO 不携带闭包、reader、logger、tracer、registry、native client 或任意运行时对象；Metadata/Extensions 必须 JSON-safe，并在 I/O 边界显式 `Validate`。
- **Tagged value，而非 sealed hierarchy**：Message/Part 使用公开 discriminator 与普通值；未知类型返回可诊断错误，不依赖未导出方法封口。
- **流式使用 `iter.Seq2`**：不自定义 iterator，不用 channel 冒充拉模型；调用方提前停止、context cancel 和首错终止必须有测试。
- **一个扩展机制**：跨调用行为只用函数式 middleware/decorator；Core 只保留类型和纯组合，不保留具体 history/logger/safeguard/OTel 实现。
- **VectorStore 保留应用语义**：公共面仍处理 Document/查询文本，但按 Indexer/Searcher/IDDeleter/FilterDeleter 拆小能力；filter 只公开稳定 Expr 门面，实现细节进 internal。

## 演进纪律

- 已删除的旧 package、alias、bridge 和 generic framework 不得重新引入。
- v1 前的 breaking change 必须在同一批次迁完全部 workspace 消费方；不保留 deprecated wrapper、双读写或旧 wire 解码。
- 任何 exported API 变更先运行 `go test ./internal/arch -run TestExportedAPIMatchesBaseline`，评估 provider/backend 爆炸半径，并同步 package docs、examples、serialization fixtures 和 release notes。只有完成评审与版本裁决后才用 `-update-api` 更新基线。

## 模块特有反向不变量

- ❌ 在 Core 放 Client builder、history 实现、tool executor/registry、agent control flow、evaluation、tokenizer 实现或 OTel 埋点。
- ❌ 用泛型 Model/StreamingModel 模拟继承，或让 Model 强制 DefaultOptions/Metadata/Stream。
- ❌ 把 `any`、闭包、SDK client、`io.Reader` 等运行时对象塞进 wire DTO。
- ❌ 新增全局 registry/cache/state，或让探测错误以 0/空值静默返回。
- ❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链。
- ❌ 用 channel 取代 `iter.Seq2` 做流式。

## 改动前必看

- Message/Request/Response 变更会影响全部 chat provider 和多个 agent/RAG/tool 消费模块。
- Document/VectorStore 同路径变更必须覆盖 documentreaders、RAG 和 27 个 backend。
- Filter 公共面变更必须同步所有 backend visitor；lexer/parser/token/visitor 不能继续成为新外部依赖。
