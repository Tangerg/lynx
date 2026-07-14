# Core 冻结旧 API 删除台账

> 建立日期：2026-07-14
> 删除截止：P6-05 / P6-06

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

## 台账维护规则

1. P2～P5 每发现一个为迁移保留的旧表面，都必须在同一逻辑提交登记目标替代、消费方、迁移任务和删除任务。
2. 台账项目不能因“仍有 consumer”延后删除；必须推进对应 consumer 迁移。
3. P6-05 开始前以 `doc/CORE_API_INVENTORY.md` 重新生成实际 import/identifier 清单，与本台账逐项核对。
4. P6-05 完成后本文转为迁移说明的输入，不保留“已 deprecated 但仍可使用”的代码。
