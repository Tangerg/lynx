# Core 冻结旧 API 删除台账

> 建立日期：2026-07-14
> 删除截止：P6-05 / P6-06
> 代码删除完成：2026-07-14（`f178f20ec`）

P6-05 已按本台账完成一次性物理删除：`core/model/chat`、冻结 provider adapter/旧契约测试、旧泛型 Model/Handler/Middleware 均已移除，workspace Go 源码不再 import 旧 Chat 路径，也不存在 alias、bridge 或 deprecated 转发。本文自此作为破坏性迁移说明和 P6-06 依赖清理的历史输入，不再表示仓库中仍有可调用的旧 API。

本文登记新路径迁移期间仍为 workspace 编译而保留、但禁止继续演进的旧 Core 表面。它不是兼容承诺；仓库当前处于协调式 v0 breaking 重构，台账中的项目必须在指定任务删除，不能成为永久 shim。

“冻结”表示：不新增 exported identifier、不为新需求扩充旧 DTO/SPI、不再接入新的 provider 或上层 consumer。安全修复、数据损坏修复及完成迁移所需的机械改动可以发生，但不能改变删除截止。

## LEGACY-CHAT-001：复合 Chat Model SPI

| 项目 | 内容 |
|---|---|
| 旧路径 | `core/model/chat.Model` |
| 旧职责 | 强制 Call + Stream + DefaultOptions + Metadata，并通过 `core/model` 泛型名义接口组合 |
| 目标替代 | `core/chat.Model`（Call）与独立 `core/chat.Streamer` |
| 冻结起点 | P2-03 |
| 迁移任务 | P2-06 四家 reference provider；P6-01 其余 provider；P6-02～P6-04 其余 consumer |
| 删除任务 | P6-05 |
| 验收 | workspace 中 `github.com/Tangerg/lynx/core/model/chat` import 为 0；旧 model.go 不存在 |

DefaultOptions 不再是 provider 能力：provider 构造参数持有 provider defaults，上层 client 持有调用策略。Metadata 不再是模型能力：观测 wrapper 在构造时显式接收 provider/model attributes，业务代码不依据 Model 身份分支。

## LEGACY-CHAT-002：混合运行时的 Request/Response

| 项目 | 内容 |
|---|---|
| 旧路径 | `core/model/chat.Request`、`Response`、`Result`、Message interface/具体消息类型 |
| 旧职责 | Request 持有 executable Tool/Params；Response 只表达单 Result 并被 tool-loop 合成控制流复用 |
| 目标替代 | `core/chat` tagged Message/Part、纯 Request、多 Choice Response；`agent/toolloop.Invocation/Event` |
| 冻结起点 | P1 完成 |
| 迁移任务 | P2-06/P2-07 reference provider；P3 ChatClient/Tool loop；P6 全 workspace |
| 删除任务 | P6-05 |
| 验收 | 旧 DTO、bridge 和旧序列化 fixture 全部删除；新协议 conformance 继续通过 |

## LEGACY-CHAT-003：Core 内 Chat framework 便利层

| 项目 | 内容 |
|---|---|
| 旧路径 | `core/model/chat` Client、conversation/history、剩余 history middleware、tool helper 与关联 `core/model` 名义层次；Logger/safeguard 已在 P3-05 删除，Chat/Embedding tracing 与 Core metrics 已在 P3-06 删除 |
| 旧职责 | Client builder、history/logger/safeguard middleware、tool schema/execution、通用调用/流式组合混在 Core |
| 目标替代 | `chatclient`、`chathistory`、`tools`、`agent/toolloop`、`otel`；Core 仅保留纯组合算法 |
| 冻结起点 | P2-03；P3 各迁移任务开始后按职责切换 |
| 迁移任务 | P2-04/P2-05、P3-01～P3-09 |
| 删除任务 | 通用 Logger 与 `core/evaluation` 已在 P3-05 删除；Core 观测代码与 OTel 依赖已在 P3-06 删除；剩余冻结 Chat 表面由 P6-05 删除，残余外部依赖由 P6-06 清理 |
| 验收 | 第 5 节 forbidden responsibilities 在 Core 中为 0；无 path bridge |

P3-07 已在目标 `tools` 根包建立 typed function Tool helper，并继续复用唯一
`tools.Registry`。旧 `core/model/chat.NewTool` 当前仍有真实旧运行时消费者，
因此只冻结、不转发；P3-08 与 P6 迁完最后消费者的同一批次必须删除它。

P3-08 已建立只依赖新 `core/chat` 与 `tools.Tool` 的 `agent/toolloop.Runner`，
并以 serializable `Checkpoint/Event` 承接 pause/resume；Core 中原
`Halt/ControlFlowError` 已删除，剩余 HITL 消费方直接依赖 agent 所有的旧
`toolloop.Halt`。旧 `NewMiddleware`、park/stream/concurrency 实现仍有真实旧 Chat
消费者，因此与新 Runner 分离冻结；它们不是 Runner adapter，最后消费者由 P6
迁移时连同旧 `chat.NewTool` 一起删除。

P6-01 已在 `d47445e52`、`14f80a8d4`、`ffc7736d2` 为全部真实 provider
形态建立目标 Chat 实现：兼容 facade 直接构造 reference adapter，Bedrock Converse
与 OpenAI Responses 直接映射 tagged protocol。旧 provider 实现不作为新实现的
内部依赖，只因 P6-02～P6-04 的旧 consumer 尚未迁完而冻结存在；P6-05 必须连同
旧 Core Chat 一次删除。

## P5 直接删除记录：Embedding framework 表面

P5-01 在 `7cd3865c3` 中直接删除了 embedding 的 `ClientRequest`、`ClientCaller`、
handler/middleware/chain、`ModelMetadata`、`GetDimensions` 与全局维度 cache，并同步
迁移所有 provider、vectorstore 和 runtime 消费点。该表面没有仍待迁移的消费者，
因此不建立冻结条目，也不保留 deprecated wrapper、alias 或 bridge。当前唯一 SPI
是单方法 `embedding.Model`；可选 `Dimensioner` 和无缓存探测 helper 均显式返回错误。

## P5 直接删除记录：其余模态 Client framework

P5-02 在 `c27886f59` 中直接删除 Image、Transcription、Speech、Moderation 的
Client/fluent request/caller、handler/middleware/chain 与 `ModelMetadata`。这些表面
没有 workspace 生产消费者，因此不建立冻结条目。25 个具体 provider 与 6 个
facade 已同步切换；Speech 的同步 `Model` 和流式 `Streamer` 是独立能力，其余模态
只保留单方法 `Model`。未保留兼容转发、alias 或 deprecated API。

## P5 直接删除记录：Catalog 与 APIKey

P5-03 在 `18d2e7a50` 中将模型卡、能力、限制、价格、计价和生成配置迁入公开
`models/catalog`，直接删除旧 Chat catalog 类型和 provider constructor 的隐式 catalog
lookup；在 `a68df8bd2` 中把全部 provider/facade credential 改为各自 config 的普通
字符串，直接删除 `core/model.APIKey` 与 `NewAPIKey`。Azure AD、Google Vertex/ADC、
Ollama keyless 由具体 adapter 显式处理，App 自己拥有 secret 脱敏。两类旧表面均无
待迁消费者，因此不建立冻结条目，也没有 alias、wrapper 或双轨配置。

## P5 直接删除记录：Tokenizer

P5-04 在 `687df9b60` 建立独立 `tokenizer` module 与 `tokenizer/tiktoken`
实现包，在 `6953b45da` 迁完 DocumentPipeline、Anthropic 和 Google 后直接删除
`core/tokenizer`、Core tiktoken 依赖以及无生产消费者的 `Estimator`/
`MediaEstimator`。新根协议只保留 TextEstimator、Encoder、Decoder 和两方法
Tokenizer；`c0b679029` 固化可独立解析的 module graph。未建立旧路径转发包、
type alias 或复合能力兼容接口。

## P5 直接删除记录：Core sibling helper 与 cast

P5-06 在 `fda80088d` 中删除 Core 对 `pkg/{json,mime,ptr,slices,text}` 和
`github.com/spf13/cast` 的全部生产 import；简单操作使用标准库或包内私有代码，
图片和 embedding 的公共 MIME 字段改为普通字符串并同步迁完 provider。没有保留
helper 类型 alias、转换 wrapper 或双字段 DTO。冻结旧 Chat 的 schema 推导改为直接
声明 `invopop/jsonschema`，它仅服务 LEGACY-CHAT-003，并在 P6-05 删除旧包、P6-06
整理 go.mod 时一并删除。

## 台账维护规则

1. P2～P5 每发现一个为迁移保留的旧表面，都必须在同一逻辑提交登记目标替代、消费方、迁移任务和删除任务。
2. 台账项目不能因“仍有 consumer”延后删除；必须推进对应 consumer 迁移。
3. P6-05 开始前以 `doc/CORE_API_INVENTORY.md` 重新生成实际 import/identifier 清单，与本台账逐项核对。
4. P6-05 完成后本文转为迁移说明的输入，不保留“已 deprecated 但仍可使用”的代码。
