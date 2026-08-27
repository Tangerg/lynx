# CLAUDE.md — core module

> Lynx 生态的窄腰：只定义跨 provider 稳定共享的协议、最小调用 SPI 和纯组合算法。

---

## 定位

- **模块是发布边界，package 是职责边界**：Core 定义 metadata/media/document、各 modality 的 Request/Response、最小 Model 能力和高层 VectorStore 语义；同一依赖预算下的 `chatclient`、`embeddingclient`、`jsonschema`、`tool`、`tokenizer`、`history` 与内存参考实现也在本 module，但仍保持独立 package，不揉成一个总框架。
- **生产依赖遵守架构边界**：标准库优先；标准库缺失时可引入经过评估、语义精确且维护成熟的通用库。Core 不 import sibling module、provider SDK、具体 tokenizer 词表或 OTel；架构门禁检查依赖方向与公共 API 泄露，不以精确第三方 import 白名单阻止正常复用，也不能用临时 wrapper 绕过边界。跨 provider 的公共契约测试由 `core/modeltest` 与 `core/vectorstore/storetest` 持有。
- **依赖方向单向**：provider、能力模块、agent 与 OTel adapter 可以 import Core；Core 不反向 import 任何 sibling module。

## 架构心智

- **扁平且语义分区**：协议包使用 `core/chat`、`core/embedding`、`core/image` 等领域名；紧邻协议的使用体验位于 `core/chatclient`、`core/embeddingclient`；跨协议基础能力位于 `core/jsonschema`、`core/tool`、`core/tokenizer`、`core/history`、`core/vectorstore`。目录层次表达语义或所有权，不用于装饰。
- **最小能力接口**：每个 modality 的 `Model` 默认只有 `Call`；真实流能力以独立 `Streamer` 表达，需要发请求的维度探测归消费工作流，不伪装为 Core SPI。
- **协议值可序列化**：DTO 不携带闭包、reader、logger、tracer、registry、native client 或任意运行时对象；Metadata/Extensions 必须 JSON-safe，并在 I/O 边界显式 `Validate`。
- **Tagged value，而非 sealed hierarchy**：Message/Part 使用公开 discriminator 与普通值；未知类型返回可诊断错误，不依赖未导出方法封口。
- **流式使用 `iter.Seq2`**：不自定义 iterator，不用 channel 冒充拉模型；调用方提前停止、context cancel 和首错终止必须有测试。
- **扩展机制归 owner**：协议扩展统一走类型化 metadata；模型调用行为由 client/middleware 组合；history、safeguard 与内存参考实现留在其语义 package，OTel 和 provider policy 仍在外部 module。
- **VectorStore 保留应用语义**：公共面仍处理 Document/查询文本，但按 Indexer/Searcher/IDDeleter/FilterDeleter 拆小能力；`IndexRequest` 自持索引输入与分批规则，`SearchRequest` / `SearchOptions` / `SearchResult` / `SearchResponse` 分别持有自己的不变量，`Score` 统一 provider 边界。filter 只公开不可变 AST/Visitor 与节点语义方法，scanner、analyzer、formatter 与 optimizer 保持私有操作对象。
- **契约测试归契约 owner**：`modeltest` 与 `vectorstore/storetest` 只提供可复用的测试 suite/fixture，不包含 provider 实现，也不成为生产适配器的依赖。

## 演进纪律

- 已删除的旧 package、alias、bridge 和 generic framework 不得重新引入。
- v1 前的 breaking change 必须在同一批次迁完全部 workspace 消费方；不保留 deprecated wrapper、双读写或旧 wire 解码。
- 任何 exported API 变更先运行 `go test ./internal/arch -run TestExportedAPIMatchesBaseline`，评估 provider/backend 爆炸半径，并同步 package docs、examples、serialization fixtures 和 release notes。只有完成评审与版本裁决后才用 `-update-api` 更新基线。
- 任何 JSON DTO/tag/省略规则变更先运行 `go test ./internal/arch -run '^TestWire'`；只有完成兼容性评审与版本裁决后才用 `-update-wire-fixtures` 更新聚合 wire baseline。新增 JSON struct 未登记 fixture coverage 会直接失败。

## 模块特有反向不变量

- ❌ 在 Core 放 provider SDK、外部存储 backend、具体 tokenizer 词表、业务 Tool executor、agent control flow、evaluation 或 OTel 埋点。
- ❌ 用泛型 Model/StreamingModel 模拟继承，或让 Model 强制 DefaultOptions/Metadata/Stream。
- ❌ 把 `any`、闭包、SDK client、`io.Reader` 等运行时对象塞进 wire DTO。
- ❌ 新增全局 registry/cache/state，或让探测错误以 0/空值静默返回。
- ❌ 把只由单一 provider 支持、或跨 provider 语义不同的 option/taxonomy 提升为 Core 固定字段。
- ❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链。
- ❌ 用 channel 取代 `iter.Seq2` 做流式。

## 改动前必看

- Message/Request/Response 变更会影响全部 chat provider 和多个 agent/RAG/tool 消费模块。
- Document/VectorStore 同路径变更必须覆盖 ETL、RAG 和全部 vectorstores backend。
- Filter 公共面变更必须同步所有 backend visitor；lexer/parser/token/visitor 不能继续成为新外部依赖。
