# Lynx Core v1.0.0 Release Notes

> 状态：Release Candidate；最终架构审查、baseline 更新与全量门禁通过后冻结
> Module：`github.com/Tangerg/lynx/core`
> 计划 tag：`core/v1.0.0`
> 最低 Go 版本：1.26.5

Lynx Core v1 将最初从 Spring AI 移植而来的“大 Core”收缩为 Go 风格的窄腰：跨 provider 稳定共享的协议值、每种 modality 的最小调用接口，以及少量无状态组合算法。Client framework、工具执行、Agent 状态机、历史存储、观测、provider reference data、tokenizer 和具体 integration 均位于独立 module。

这是一次有意的破坏性发布。旧 API 和旧 wire 已删除，不提供兼容包、alias、deprecated wrapper 或双读窗口。升级步骤见 [`CORE_V1_MIGRATION.md`](./CORE_V1_MIGRATION.md)。

## Highlights

- Core 公共面从重构前 24 个 package、1,205 个导出标识符收敛为 12 个公共 package、351 条自动冻结的 exported declaration/method signature。
- Core 生产代码只依赖 Go 标准库和 Core 自身 package；没有 provider SDK、OTel、vector DB driver、tokenizer 或 sibling helper 依赖。
- Chat 使用可序列化 tagged Message/Part、纯 Request、多 Choice Response，以及独立 `Model`/`Streamer` 能力。
- Metadata 在写入时编码为 JSON，Media 只允许 bytes/URI/provider reference 三种显式来源。
- Document 是纯数据；VectorStore 按 Index/Search/Delete 能力拆分，Filter 只公开稳定语义 AST。
- Embedding/Image/Transcription/Speech/Moderation 采用一致的单方法最小 SPI；Speech streaming 为可选独立能力。
- Provider、VectorStore、wire、API、docs/examples、coverage 和 dependency budgets 均有 blocking 自动门禁。

## Public packages

| Package | v1 职责 |
|---|---|
| `core/chat` | provider-neutral Chat 协议、`Model`、可选 `Streamer`、middleware 函数形态与 response accumulator |
| `core/document` | 可序列化 Document 值与最小 Reader/Writer 词汇 |
| `core/embedding` | text-to-vector 协议、单方法 Model、可选 Dimensioner 与显式维度探测 |
| `core/image` | image generation 协议与单方法 Model |
| `core/media` | bytes/URI/reference tagged media |
| `core/metadata` | JSON-safe metadata map 与 typed encode/decode helper |
| `core/model` | 跨模态共享的 Usage/RateLimit 值；不含泛型模型框架 |
| `core/moderation` | moderation 协议、分类结果与单方法 Model |
| `core/speech` | text-to-speech 协议、独立 Model/Streamer |
| `core/transcription` | audio-to-text 协议与单方法 Model |
| `core/vectorstore` | Indexer/Searcher/IDDeleter/FilterDeleter 与 SearchRequest/Match |
| `core/vectorstore/filter` | 稳定 metadata filter Expr、构造器、校验与解析入口 |

每个 package 都有唯一 package documentation 和带 checked output 的 runnable Example。

## Major changes

### Chat protocol and SPI

- 删除 message interface/具体 message 类型层次，改用带 Role/Kind discriminator 的普通值。
- Request 不再持有 executable Tool、registry、闭包或任意 Params；provider 扩展使用 namespaced JSON extension。
- Response 保留全部 Choices，并提供 nil/empty-safe `First`/`Text`。
- `Model` 只要求 `Call`；`Streamer` 不嵌入 Model，Model 也不嵌入 Streamer。
- provider defaults 和 identity 不再是 Model 方法；分别由 provider config、ChatClient defaults 和观测 wrapper 显式拥有。
- Core 只保留 `Wrap`/`WrapStream` 的函数装饰形态，不再提供 Clone/With/Build middleware builder。

### Runtime responsibilities moved out

| 能力 | 新归属 |
|---|---|
| 默认参数、Chat middleware、template、structured output | `chatclient` |
| 历史接口、backend 与 history middleware | `chathistory` |
| typed tool、schema reflection、executor、registry | `tools` |
| tool loop、Event、HITL、checkpoint/resume | `agent/toolloop` |
| safeguard | `chatclient/middleware/safeguard` |
| Chat/Embedding tracing 与 metrics | `otel` |
| factuality/relevancy evaluation | `rag/evaluation` |
| formatter/splitter/batcher/ID generator | `documentpipeline` |
| tokenizer SPI 与 tiktoken | `tokenizer` |
| model catalog/capability/pricing | `models/catalog` |
| provider credential | 各 `models/<provider>` config 与应用 secret layer |

通用 request/response Logger middleware 被删除；普通调用遥测由 OTel span/metric 承担，审计日志由应用按业务语义单独建模。

### Metadata and media

- `metadata.Map` 保存 `json.RawMessage`，通过 `Set`/`FromValues` 在写入时编码和校验。
- 所有持有 metadata 的 Core DTO 在 Marshal/Validate 时递归检查 JSON 有效性。
- Embedding/Image/Moderation/Speech/Transcription 的 Options 与响应 metadata 扩展统一使用 `metadata.Map`；其 `Set` 方法返回并要求调用方传播编码错误。
- 删除所有 modality Request-level `Params` 参数袋；typed Options 之外的 provider JSON 仅能进入 `Options.Extra`。
- 删除 `model.Usage.OriginalUsage`；Core Usage 只保存跨 provider 规范化计数，原始 SDK payload 留在 adapter 边界。
- 五个 modality Request 统一提供递归 `Validate`；官方 provider 的 Call/Stream 在读取字段前执行验证，并由 AST 门禁防止绕过。
- provider 原生 options key 统一为 `<provider>/options`，删除 Spring 移植期 `lynx:ai:model:*` key；Google transcription 使用共享 typed Prompt。
- 删除 `Media.Data any`、`NewMedia` 和运行时对象承载能力。
- `media.NewBytes`、`NewURI`、`NewReference` 分别构造唯一有效 source；MIME 使用边界校验后的普通字符串。

### Document and VectorStore

- `Document.Score` 删除，相关度只存在于 `vectorstore.Match.Score`。
- Document formatter/transformer/batcher/ID 实现迁出 Core。
- 删除胖 `Store`、Creator/Retriever/Deleter 聚合、fluent RetrievalRequest 和 `NativeClient any`。
- 新接口为 `Indexer`、`Searcher`、`IDDeleter`、`FilterDeleter`；调用方可以在本地组合所需能力。
- `SearchRequest` 是普通 struct + `Validate`。
- Filter lexer/parser/token/analyzer/optimizer 全部 internal 化；provider 只依赖稳定 Expr。

### Other modalities

- `core/model/<modality>` 扁平为 `core/<modality>`。
- 删除各 modality 的 ClientCaller/Handler/Middleware/Chain/ModelMetadata framework。
- Embedding dimensions 不再由所有实现强制提供，也没有全局 probe cache。
- Speech 的同步/流式能力分开；其余 modality 只定义单方法 Model。
- Request 不再有任意 `Params`；provider 扩展在 adapter 中通过 JSON-safe `Options.Extra` 编解码。

## Removed API and paths

以下路径或等价职责在 v1 中不存在：

- `core/model/chat` 及其 conversation/history/middleware 子包
- `core/model/embedding`
- `core/model/image`
- `core/model/audio/transcription`
- `core/model/audio/tts`
- `core/model/moderation`
- `core/tokenizer`
- `core/evaluation`
- `core/document/id`
- `core/vectorstore/filter/{ast,lexer,parser,token,visitors}` 公共路径
- Core generic Model/StreamingModel/Handler/MiddlewareChain
- Core APIKey、provider catalog/pricing、tool executor、control-flow error、tracing/metrics 实现

旧路径不会在 v1.0.x 中恢复。确有新职责时，应在正确 module 以新设计提出；不是以兼容名义回填 Core。

## Wire compatibility

v1 wire 基线冻结 50 个带 JSON tag 的导出 struct，并用 19 个代表性 root 生成 514 行聚合 golden，覆盖 Metadata、Media、Chat、Document、Embedding、Image、Moderation、Speech、Transcription、Usage/RateLimit 与 VectorStore Search/Match。

该基线只承诺 v1 当前 wire 之后的 SemVer 管理，不承诺读取重构前 type-tagged Chat/History wire。旧持久化数据必须在升级前一次性迁移；详见迁移指南第 8 节。

## Quality and compatibility gates

- 351 条 exported API baseline 自动检测声明和方法签名增删。
- 50 项 wire DTO inventory 防止新 JSON struct 绕过 fixture review。
- AST 协议安全门禁拒绝任何可序列化公共 DTO 重新引入 `any`/`interface{}` 字段或 Request `Params` 参数袋。
- 12/12 公共 package docs 与 runnable examples 自动守卫。
- 17 个 Core package 逐包 coverage budget 全部不低于 P0 baseline；新增协议面采用更高目标。
- Core、ChatClient、Agent、ChatHistory、RAG、Tools 和全部 27 个 VectorStore backend 已通过 race。
- 7 个 serialization/parser fuzz target 各独立运行 5 分钟，累计约 1.45 亿次执行且无失败语料。
- 30 个公开 Chat provider/facade 构造入口和五类参考协议实现纳入 Models blocking conformance。
- Models 架构门禁自动发现非 Chat modality 的 Call/Stream，要求请求验证或显式委托到已验证 Call，并冻结 `<provider>/options` key 规则。
- 27/27 VectorStore backend 纳入自动发现、注册结构和 race conformance 门禁。
- 20 个 workspace module 的 build/vet/test/lint 在 Go 1.26.5 下 80/80 通过；20/20 `go mod tidy -diff` 为空。

## Security and known risks

- Core v1 自身没有第三方生产依赖，`govulncheck` 无可达漏洞。
- Go 从 1.26.4 升到 1.26.5，已清除标准库 `crypto/tls` 可达漏洞。
- `models` 与 `app/runtime` 使用当时最新 Ollama v0.32.0 时仍命中 8 个上游公告，且扫描结果全部为 `Fixed in: N/A`。这不影响 Core module 本身，但会影响协调发布的 Models/App 风险裁决。详细规则见 [`CORE_V1_RELEASE_RUNBOOK.md`](./CORE_V1_RELEASE_RUNBOOK.md)。

## Upgrade and release order

- 外部消费者：先阅读 [`CORE_V1_MIGRATION.md`](./CORE_V1_MIGRATION.md)，在单个 breaking branch 中完成源码与数据迁移。
- Lynx 多 module 发布：先发布 Core 和无内部依赖基础 module，再按 DAG 发布 adapters/组合模块、协议桥、Agent，最后更新 App。精确波次见 [`CORE_V1_RELEASE_RUNBOOK.md`](./CORE_V1_RELEASE_RUNBOOK.md)。
- 当前 API 的最小调用方式见 [`CORE_GETTING_STARTED.md`](./CORE_GETTING_STARTED.md)。

## SemVer policy after v1

- v1.0.x：只能做兼容 bug/security/documentation 修复，不能删除或改变 exported API/wire 语义。
- v1.x：可以增加向后兼容的 API；新增接口方法通常是 breaking change，不得伪装成 minor。
- 任何 API/wire baseline 差异都必须先有 ADR、爆炸半径评估、迁移说明和版本裁决。
- 破坏性变化进入新的 major；不通过 deprecated shim 在 v1 内长期维持两套设计。
- 已推送 tag 不可移动；修复使用新 patch/minor/major 版本。
