# Core v1 最终架构审查

> 审查日期：2026-07-15
> 审查结论：通过；Filter tag 前精修后重新冻结 Core v1 公开契约
> Module：`github.com/Tangerg/lynx/core`
> 目标 tag：`core/v1.0.0`（尚未创建）

## 1. 结论

Core 已从 Spring AI 移植期的“大 Core/框架内核”收敛为 Go 风格的协议窄腰：只保留跨 provider 稳定的可序列化值、每个 modality 的最小能力接口和无状态组合算法。运行时编排、默认参数、工具执行、Agent 状态、持久化、观测、provider 数据、tokenizer 与具体 integration 均由外层 module 拥有。

最终审查批准当前契约进入 v1 稳定期。冻结意味着从 `core/v1.0.0` 开始按 SemVer 管理当前 API 与 wire，而不是恢复任何重构前 API 或历史 wire。仓库中没有为旧设计保留 alias、bridge、shim、兼容字段、dual-read/dual-write 或旧 decoder。

`core/v1.0.0` 尚未创建，因此维护者在 tag 前重新打开 Filter 契约审查：删除双 AST/转换层、builder 与 precedence 表面，公开 `Predicate`/`Selector`/`Visitor`，并把递归下降前端收敛为同包私有实现。维护者随后确认 `Visitor` 是外部 adapter 的扩展逃生舱，补充 `Visit(predicate, visitors...)` 作为验证一次、顺序分派的公共消费入口；最终保留公开 `Formatter`，把 analyzer/optimizer 作为同包私有 visitor，并收紧 provider 数字编译边界。最终 334 行 API baseline 取代先前 341 行草案；wire inventory 与 golden 不变。

## 2. 冻结范围

| 项目 | 冻结结果 |
|---|---:|
| 公共 package | 11 |
| exported API baseline | 334 行冻结快照 |
| 带 JSON tag 的导出 DTO | 49 |
| 代表性 wire root | 17 |
| 聚合 wire golden | 487 行 |
| Core 第三方生产依赖 | 0 |
| Core sibling module 生产依赖 | 0 |

公共 package 为 `chat`、`document`、`embedding`、`image`、`media`、`metadata`、`moderation`、`speech`、`transcription`、`vectorstore` 与 `vectorstore/filter`。每个 package 都有唯一职责文档和带 checked output 的可执行示例。

## 3. 架构原则审查

| 原则 | 证据 | 结论 |
|---|---|---|
| Core 是协议库，不是运行时框架 | Client、History、Tool runtime、Agent、OTel、catalog 与 tokenizer 已外移 | 通过 |
| 公共值可独立序列化 | 49 项 DTO inventory 与 487 行 wire golden 为 blocking gate | 通过 |
| 公共 wire 不承载任意运行时值 | AST 门禁拒绝序列化 DTO 中的 `any`/`interface{}`、Request `Params` 和 `Usage.OriginalUsage` | 通过 |
| 扩展数据写入时即验证 | `metadata.Map` 保存 `json.RawMessage`；`Set`/`FromValues` 返回编码错误，`Merge` 校验后深拷贝且失败不修改 receiver | 通过 |
| 无效请求不进入 provider SDK | Chat 与五个非 Chat modality 递归 `Validate`；Models AST 门禁覆盖 Call/Stream 边界 | 通过 |
| 接口由消费能力塑造 | Model/Streamer、Indexer/Searcher/Deleter 等接口保持 1–3 个方法且能力可分离 | 通过 |
| 依赖方向单向 | Core 生产 import 只允许标准库或 Core 自身，外层 module 依赖 Core | 通过 |
| 便利层不反向塑造协议 | Embedding 向量便利方法位于 `embeddingclient`；Core 不公开 Client、默认值或 middleware | 通过 |
| provider 差异不扩张 Core | provider JSON 进入 typed Options 或 namespaced `metadata.Map`；options key 冻结为 `<provider>/options` | 通过 |
| 值对象行为归属明确 | 五个 modality 的不可变 Options 合并由 `Merged` receiver 承担；Embedding/Moderation 首项统一为 `First`；Speech 使用 `OutputFormat`/`Audio` | 通过 |
| Filter 遍历职责单一 | 公开 Formatter 服务外部文本适配；私有 analyzer/optimizer 分别拥有校验与 Parse 规范化，provider 数字转换按 SDK 表达能力显式失败 | 通过 |
| 不保留迁移债务 | 旧 package、旧 wire decoder、alias/bridge/shim 与双轨读写均不存在 | 通过 |

## 4. 可扩展性结论

Provider 通过实现各 modality 的最小接口并在 adapter 内完成 typed SDK 映射；VectorStore backend 通过实现调用方需要的小能力接口并翻译稳定 Filter Expr。`filter.Visit` 消费公开 `Visitor`，让外部 adapter 在一次校验后按顺序组合多个 compiler/interpreter；首错原样返回且后续 visitor 不执行。`Formatter` 提供稳定文本出口，分析与优化策略不成为外部协议。新增 integration 不需要修改 Core 接口，也不需要 Core 反向 import SDK、driver 或上层 module。

Chat provider/facade 的构造与共享协议行为由 Models conformance 覆盖；五类参考实现覆盖 Anthropic、Bedrock、Google、Ollama 与 OpenAI。VectorStores 自动发现实现集合，并要求 27/27 backend 注册和执行共享 conformance，新增实现无法静默绕过发布门禁。

## 5. 依赖图与独立 module 验证

最终远端依赖图由以下提交建立：

- `783df3ee9`：完成 tag 前协议词汇收口、API/wire 基线更新和发布门禁修复。
- `229e06c8e`：将 standalone module 对齐到最终 Core 源码基线。
- `04a37a9fe`：使远端 pseudo-version DAG 闭合，避免 `go.work` 或本地 cache 掩盖缺失依赖。
- `3f7af1a3a`：将 Embedding Client 从 Core 外移并迁移真实消费者。
- `3938d179f`：闭合 `embeddingclient`、VectorStores 与 App 的远端依赖图，并推进 App 的 Models 基线。

21/21 workspace module 在 `GOWORK=off` 下完成独立 `go test -count=1 ./...`、`go vet ./...` 与 `go mod tidy -diff`；`go list -m all` 不再解析旧的 `v0.0.0-20260714110600-0abc7c70a85d` 基线。当前 pseudo-version 只是创建正式 tag 前的可复现协调图，发布时仍须按运行手册自底向上替换为精确 tag。

## 6. 质量门禁证据

- `FAST=1 scripts/check.sh build vet test lint race`：21 个 module、105/105 项通过。
- `scripts/check.sh vuln`：21/21 module 通过精确漏洞策略；新增 `embeddingclient` 无可达漏洞。
- `scripts/check-core-release.sh` 的确定性部分：Core test/race/vet/lint/tidy、当前 12 个生产 package 的逐包 coverage budget、Models provider gate 和 27 个 VectorStore backend gate 全部通过。
- Core API、wire inventory/golden、协议字段安全、公共 docs/examples、标准库-only 依赖均为 blocking 架构测试。
- P7-05 已记录 7 个 fuzz target 各独立运行 5 分钟：Metadata Map 98,292,275 次、Media 105,198,336 次、Filter Parse 75,835,393 次、Chat Part 83,961,412 次、Chat Message 79,948,335 次、Chat Request 82,260,984 次、Chat Response 84,349,479 次；累计 609,846,214 次且无失败语料。tag 前收口后又完成 Metadata Map 5 分钟、98,942,554 次执行；按维护者要求停止重复整组长时间 fuzz，未把主动停止的 Media 运行记作通过。

确定性门禁在最终 dependency DAG 上执行；不是只验证本地替换后的中间图。完整 fuzz 数字是 P7-05 的历史发布证据，不冒充本轮重复执行结果。

## 7. 安全裁决

Core 没有第三方生产依赖，`govulncheck` 没有可达漏洞。Models 与 App Runtime 通过 Ollama v0.32.0 仍可达 8 个无修复版本的上游公告：`GO-2025-3557`、`GO-2025-3558`、`GO-2025-3559`、`GO-2025-3582`、`GO-2025-3689`、`GO-2025-3695`、`GO-2025-3824`、`GO-2025-4251`。精确 allowlist 只允许这两个 module、这个版本、这些公告且 `Fixed in: N/A`；版本、可修复状态或命中集合变化都会使门禁失败。

`GO-2026-5932` 仅作为非可达 module finding 出现在 Models、App Runtime、ChatHistory 与 VectorStores，不构成当前可达调用链；扫描仍保留该信息，若未来变为可达会立即阻断发布。

这两类外层风险不改变 Core v1 的审查结论，但必须按协调发布手册在 Models/App 发布记录中披露并持续复核。

## 8. 剩余风险与发布边界

- 本审查冻结契约，但没有创建或推送 `core/v1.0.0` tag；tag 属于单独的不可逆发布动作。
- 正式发布必须按六波次 DAG 进行，不能一次创建全部 tag，也不能以 `replace`、移动 tag 或本地 cache 补图。
- 旧持久化数据必须由升级前二进制一次性导出转换；新库不会增加旧 decoder 或双读窗口作为回滚手段。
- Ollama 的 8 个上游公告由 Models/App 发布负责人按部署暴露面接受、隔离或延期；Core 不为它们引入依赖或抽象。

## 9. v1 冻结规则

1. API baseline 或 wire golden 的任何差异都必须先有 ADR、兼容性判断、迁移说明和版本裁决。
2. v1.0.x 只接受兼容 bug、安全与文档修复；接口增方法通常是 breaking change。
3. 破坏性变化进入新的 major，不通过 deprecated shim 在 v1 内维持两套设计。
4. Provider 或 backend 的新能力优先在 adapter/上层 module 组合；只有四个以上真实实现与消费方共享的稳定语义才进入 Core 候选审查。
5. 已推送 tag 不得移动；发布操作与版本集合按 [`CORE_V1_RELEASE_RUNBOOK.md`](./CORE_V1_RELEASE_RUNBOOK.md) 留档。

P8-05 确定性门禁通过后，Core 架构演进计划的 65 项任务已经全部完成；后续工作从“重构计划”切换为“稳定契约维护与正式发布”。
