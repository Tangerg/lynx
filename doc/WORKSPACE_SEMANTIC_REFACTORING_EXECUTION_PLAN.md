# 全仓语义化与所有权重构执行计划

> 状态：实施中
> 建立日期：2026-07-16
> 最后更新：2026-07-16
> 当前基线：`02e9b5012`（`codex/runtime-architecture-refactor`）
> 维护原则：开发期直接改到最终形态，不保留 alias、shim、deprecated wrapper 或双路径

本文档是 Agent Framework P10 完成后的下一轮全仓精修执行基准。它不重新设计
Core 或 Agent，也不把所有模块套进同一套 DDD/Clean Architecture 模板；目标是基于
真实调用点继续收敛行为所有权、命名和 API，使代码读起来更像 Go 标准库或成熟
第三方 framework，而不是 Java/Embabel 模型的逐词移植。

上位约束：

- [`../CLAUDE.md`](../CLAUDE.md)
- [`../DESIGN_PHILOSOPHY.md`](../DESIGN_PHILOSOPHY.md)
- [`../REFACTORING.md`](../REFACTORING.md)
- [`AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](./AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)
- [`../app/runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md`](../app/runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md)

任何 exported identifier、公开签名、公开字段或现行 wire 行为的破坏性修改，都必须
先按本文列出爆炸半径并取得维护者确认。确认后直接迁移，不保留兼容面。

---

## 1. 范围

### 1.1 纳入

- `a2a`
- `agent`
- `app/runtime`
- `app/desktop` 的 Go/Wails v2 shell
- `chatclient`
- `chathistory` 及其全部 backend
- `core`
- `documentpipeline`
- `embeddingclient`
- `mcp`
- `otel`
- `rag`
- `skills`
- `tokenizer`
- `tools` 及其全部 executor/provider/tool adapter

`a2a`、`mcp`、`otel` 虽属于集成边界，但它们是 Lynx 明确需要维护的基础协议能力，
因此纳入。

### 1.2 排除

- `pkg`
- `models`
- `vectorstores`
- `documentreaders` 及其子模块

排除表示本轮不修改这些模块，不表示可以忽略它们作为消费方的影响。任何 Core
公开面调整如果要求同步修改排除模块，本轮默认暂缓。

### 1.3 不纳入进度统计

- tests/examples 中只为测试表达服务的 helper 数量；
- 第三方代码、生成代码、前端依赖；
- 单纯格式化、import 排序或无语义的文件搬动；
- 包级私有函数总数下降。

---

## 2. 总目标

1. 让行为从真实 owner 出发，减少“数据袋 + 外部 helper”。
2. 删除 Java 式、移植式、含糊或 package stutter 的命名。
3. 让公开 API 保持小、显式、可发现、难误用。
4. 让 Core 继续是窄协议腰，Agent 继续是生命周期 framework。
5. 让 Application、Delivery、Adapter、Infra 的知识边界保持单向。
6. 删除无真实消费者或只为旧开发版本存在的公开表面。
7. 保留解析器、codec、投影函数和切片算法的函数式形态，不为“充血”制造伪对象。

成功不以 receiver 数量、文件数量或代码行数衡量，而以以下结果衡量：

- 调用点是否更直接；
- 不变量是否只存在一处；
- 参数是否不再重复搬运同一上下文；
- 类型是否因此更完整；
- 包级命名空间是否更容易理解；
- 新 API 是否比旧 API 更难误用；
- 是否没有引入新的抽象层、Manager/Service/Builder 或兼容债。

---

## 3. Receiver 决策门槛

候选必须至少满足下列一项，才允许从自由函数迁入 receiver：

1. 函数维护或派生某个类型的不变量。
2. 多个调用点反复传入同一 owner 或同一上下文。
3. 函数操作 receiver 已经拥有的状态，或应由 receiver 拥有的状态。
4. 调用点改成从对象出发后，能明显减少参数、临时变量或类型跳转。
5. 行为迁入后能删除包级实现细节，缩小可见认知面。
6. 状态与行为放在一起后，测试可以直接围绕领域值或框架对象表达。

以下情况保留自由函数：

- 构造器、factory、wire decode；
- 跨多个类型的组装或 projection；
- slice/map 算法；
- parser/scanner/formatter 的无状态算法；
- SQLite row codec、时间/布尔存储编码；
- HTTP/MCP/A2A 内容转换；
- 多类型泛型算法；
- 无状态 Delivery presenter；
- reusable test conformance/helper。

反例：

```go
presentItem(item)
```

改成：

```go
server.presentItem(item)
```

如果 presenter 不依赖 `Server`，只会伪造所有权，必须保留自由函数。

正例：

```go
validatePatchFile(file)
applyHunks(lines, file.hunks, file.path())
```

`filePatch` 同时拥有路径、hunk 和 patch 语义，改成 `file.validate()` /
`file.apply(lines)` 能消除重复参数并集中不变量，属于真实 owner。

### 3.1 候选验收卡

每个 receiver 候选在实施前后都必须记录下表；不接受“更面向对象”“看起来更内聚”
作为理由。

| 观察项 | 实施前 | 实施后应满足 |
| --- | --- | --- |
| 重复上下文参数 | 同一 owner/state 被多少函数反复传递 | 数量明确下降 |
| 不变量入口 | 校验/默认化分散在多少处 | 收敛到一个 owner |
| 调用点跳转 | 理解行为需要跨多少类型/文件 | 不增加，且通常下降 |
| 临时组装 | 调用方是否先拆字段再调用 helper | owner 直接完成行为 |
| 可见符号 | 包级 helper 是否暴露实现细节 | helper 被删除或变成 owner 私有方法 |
| 测试表达 | 测试是否围绕散落函数和重复 setup | 能直接围绕完整值/state 验证 |
| receiver 状态使用 | 无 | 方法真实读取或维护 receiver 状态 |

至少满足以下规则，候选才通过：

1. 上表至少两项有明确改善，其中一项必须是“重复上下文参数”“不变量入口”或
   “receiver 状态使用”。
2. 新方法不能只是调用原自由函数的薄包装。
3. 新类型不能只为承载一个无状态函数而创建。
4. receiver 不能注入与其生命周期无关的依赖，只为让方法“有状态”。
5. 若方法仅有一个调用点，除非它维护不变量或显著补全类型语义，否则默认保留
   私有自由函数或直接内联。
6. 方法化后若总代码路径、类型跳转或测试 setup 上升，候选取消。

实施记录使用三种裁决：

- **迁入**：证据表明 owner 更完整且认知面下降；
- **保留**：自由函数已准确表达 parser/projection/algorithm/codec；
- **取消**：方法化只改变语法，没有减少上下文或集中不变量。

“取消”是正常审计结果，不计为进度失败。P8 会复核所有已迁入项；不能继续证明收益
的，恢复为自由函数，不保留空壳 receiver。

---

## 4. 审计基线

### 4.1 规模

纳入范围的生产 Go 代码约 8.1 万行。高密度区域：

| 区域 | 观察 |
| --- | --- |
| `app/runtime` | 约 417 个包级私有函数；大量为 wire projection、SQLite codec、组装和 parser |
| `tools` | 约 105 个包级私有函数；真实 owner 主要集中在 `fs`、provider DTO 和 fakeweather 生成上下文 |
| `agent` | P10 已完成；剩余自由函数大多是构造、wire decode、跨类型算法 |
| `core` | 大多是协议值、parser 和 validation；少量值对象行为仍散落 |
| `mcp` / `a2a` / `otel` | 主要是协议转换和 instrumentation projection，整体边界正确 |

这些数字只证明审计覆盖，不是重构 KPI。

### 4.2 大文件

| 文件 | 裁决方向 |
| --- | --- |
| `tools/fs/local.go` | 混合 path/read/write/edit/glob/grep，适合按 `LocalExecutor` 职责拆文件 |
| `tools/fs/patch.go` | 大但领域内聚；优先收回 `filePatch` / `patchHunk` 行为，再判断是否拆文件 |
| `app/runtime/internal/infra/storage/sqlite/runs.go` | admission state 与 restart reconciliation 两个关注点，适合同包拆文件 |
| `app/runtime/internal/delivery/server/canonical_from_wire.go` | artifact/run/item 解码可按职责拆分；不因大而强造 decoder receiver |
| `app/runtime/internal/delivery/server/presenter.go` | 无状态 projection 正确；可按 run/item/tool 拆文件但不挂 `Server` |
| `tools/fakeweather/generate_weather.go` | 共享 RNG/climate/date 上下文明显，候选为真实私有生成器 |

### 4.3 当前事实

- 当前 worktree 在审计开始时为 clean。
- Agent Framework P0–P10 已关闭，97/97。
- 当前 HEAD 为 `02e9b5012`。
- 本轮尚未修改生产代码。
- 实施开始前重新建立各受影响模块的 test/vet 基线。

---

## 5. 审计结论

### 5.1 明确应保留

#### Delivery projection

`app/runtime/internal/delivery/server` 的 `presentXxx`、`xxxToWire`、
`xxxFromWire` 大多是跨类型、无状态 projection。它们属于 Delivery 包，但不属于
`Server` 实例；应保留自由函数，只按内容拆文件。

#### SQLite codec

`scanSchedule`、`scanPending`、`rowToSession`、`encodeStrings`、
`decodeVec`、`toMillis` 等是存储边界 codec。它们没有领域 receiver，迁入 Store
只会让 SQL 编码看起来像领域行为，应保留。

#### Parser/algorithm

Core filter parser/optimizer、Git diff parser、LSP JSON 解析、RAG
`parallelCollect`、文本 diff、slice/map 聚合继续保持函数式。

#### Test support packages

- `agent/storetest`：公开 Store conformance suite，语义与标准库 `fstest` /
  `iotest` 同类，保留。
- `tools/websearch/internal/providertest`：内部 Provider 接入一致性测试，保留。
- `tools/webfetch/internal/providertest`：内部 Provider 接入一致性测试，保留。
- `app/runtime/internal/domain/approval/approvaltest`：内部领域测试支持，保留。

`test` 后缀准确表达“可复用测试契约/夹具”，不是临时目录，也不是生产实现，不应因
名字带 `test` 而删除或改成含糊的 `internal/helper`。

### 5.2 明确 owner 候选

#### Core

- `ToolDefinition.Clone`：`InputSchema` 是可变 slice，值的防御性快照应由值对象拥有。
- image/embedding/moderation/speech/transcription 的 `Result` 校验应成为
  `result.validate()`，而不是五个包各留 `validateResult(result)`。

#### Tools

- `EditOperation.apply(content, path)`
- `GrepInput.contextLines()`
- `filePatch.validate()`
- `filePatch.apply(lines)`
- `patchHunk.splitLines()`
- `unifiedPatch.paths()` / `unifiedPatch.duplicatePath()`
- provider `Request.params()`、`Response.toWebSearch(...)`、`Result.snippet()`
- fakeweather 私有 generator 持有 target/RNG/zone/coordinates/profile/season，
  让气象派生方法不再反复搬运 4–6 个相同参数

#### App Runtime

- `EventCommit.empty()` 或等价的 nil/commit 判定行为
- `Interrupt.validate()` / `TurnInterrupted.validate()`
- reducer 对 open tools 的排序、drain snapshot 和清理行为
- canonical transcript 中 FileChange 的路径派生行为
- artifact decode 若最终引入 receiver，必须真实持有 session ID、message count、
  run/item identity maps，而不是空 decoder

### 5.3 明确 API 候选

#### Document Pipeline

- `Sha256Generator` → `SHA256Generator`
- `NewSha256Generator` → `NewSHA256Generator`
- `TokenCountEstimator` → `Estimator`
- `MaxInputTokenCount` → `MaxTokens`
- `ReservePercentage` → `Reserve`
- `ExcludedInferenceMetadataKeys` → `ExcludeFromInference`
- `ExcludedEmbedMetadataKeys` → `ExcludeFromEmbedding`
- `WithDocumentMarkers` → `DocumentMarkers`
- `AppendMode` → `Append`
- `ApplyDefaults` 改为构造器内部私有默认化
- 删除含糊且一型三用的公开 `Nop/NewNop`；补齐标准函数适配器
  `TransformerFunc` / `BatcherFunc`，默认 formatter 使用包内实现

#### Chat History Backends

每个 backend package 只有一个主类型 `Store`，推荐：

- `StoreConfig` → `Config`
- `NewStore` → `New`
- `ApplyDefaults` / 当前依赖调用顺序的 `Validate` 收为构造器内部实现
- 删除全仓无生产消费者的 `Provider = "XxxChatHistory"` 常量

根包 `NewInMemoryStore`、`NewWindowStore` 不改；根包同时拥有多个 Store 类型，完整
名字有助于区分。

#### Web Providers

- `NewClient(cfg *Config)` → `NewClient(cfg Config)`
- Config 默认化/校验由构造器内部负责，避免无意义 nil config 状态
- Provider-native `Request.Validate` 保留：它对 `SearchNative` / `FetchNative`
  调用方仍有直接价值

#### MCP / RAG

- `SamplingViaChatClient` → `NewSamplingHandler`
- 修复 sampling 注释与实际 MaxTokens/Temperature/Stop 转发不一致
- `rag.Multi` → `rag.Parallel`
- `rag.NoopRetriever` → `rag.NopRetriever`

### 5.4 结构性问题

`app/runtime/internal/application/runs` 当前直接知道：

- `shell` / `grep` / `glob` / `web_search` / `edit` / `write` 工具名；
- 各工具 output JSON shape；
- UI 活动文案；
- 文件变更展示结构。

这比“某个 helper 是否挂 receiver”更重要。工具是可扩展能力，Application 不应随
每个工具的展示协议增长。

推荐方向：

1. canonical tool invocation 只保存通用 identity、arguments、raw result/error。
2. 已知工具的结构化展示由 Delivery 的 presenter registry 负责。
3. 文件变更路径作为执行 side-effect fact 显式进入 EngineEvent/EventCommit，不从
   UI transcript 反推。
4. `RunProgress` 在 Application 中携带 tool identity，而不是人类活动文案；Delivery
   最终生成 wire `activity`。
5. 未知工具自然回退 raw result，不修改 Application。

备选方向是由 Application 定义 `ToolResultDecoder` port、Adapter 实现。它能移除
具体工具知识，但仍让 canonical transcript 承担展示结构，扩展性弱于推荐方案。

### 5.5 暂缓项

#### Core Filter Operator 命名

`Operator.IsEqualityOperator` 等方法存在 stutter，理想名字是 `IsEquality` /
`IsOrdering` / `IsLogical` 等。但当前有：

- Core 内 18 个引用；
- 排除范围 `vectorstores` 内 49 个生产引用。

本轮若修改必须同步触碰排除模块，因此暂缓，不留兼容 alias。

#### Provider 错误分类

Runtime reducer 通过错误字符串识别 rate limit/auth/timeout/provider unavailable。
根治需要 Models/provider adapter 提供稳定 typed error，而 `models` 本轮排除。
因此本轮只记录，不在 Application 上继续堆更多字符串规则。

#### Core Options / extension 命名

Core Request/Options 的部分 Set/extension API 仍有统一空间，但 Models 是主要消费方。
在排除模块不迁移的前提下不打开该公开面。

---

## 6. 分批执行计划

### P0：审计与决策门

> 状态：完成

- [x] 固定纳入/排除范围。
- [x] 读取全仓治理与模块特有约束。
- [x] 盘点生产 Go 文件、私有函数与大文件。
- [x] 人工复核 Core/Agent/Runtime/Tools/History/MCP/A2A/OTel/RAG/
  Document Pipeline/Skills/Tokenizer/Desktop。
- [x] 区分真实 owner、合理自由函数、结构性边界问题和跨排除模块候选。
- [x] 维护者确认 P1–P8 的 breaking/structural scope。

### P1：Core 值对象所有权与 ToolDefinition 快照

> 类型：非破坏性，低风险
> 状态：进行中

- [ ] 为 `chat.ToolDefinition` 增加防御性 `Clone()`。
- [ ] `tools.Registry`、typed function tool、Agent tool、MCP tool、A2A tool 统一返回
  独立 definition snapshot。
- [ ] 删除 `tools.cloneDefinition` 重复实现。
- [ ] 五个 Core modality 的 Result 私有校验迁入 receiver。
- [ ] 增加 aliasing/validation tests。

退出标准：

- 修改调用方拿到的 `InputSchema` 不影响 Tool 内部定义；
- Core API/wire golden 无非预期变化；
- Core/Tools/Agent/MCP/A2A test + vet 全绿。

### P2：Tools 所有权与文件组织

> 类型：内部重构，低至中风险

- [ ] `tools/fs` 按真实 owner 收回 edit/grep/patch 行为。
- [ ] `LocalExecutor` 实现按 path/read-write/search 拆为同包文件。
- [ ] 保留 parser、atomic exact-overwrite、grep process helper 为自由函数。
- [ ] 不用 fileflow 替代 exact overwrite：其 no-overwrite/suffix 语义与编辑器写入合同不同。
- [ ] fakeweather 引入持有共享随机与气候上下文的私有 generator。
- [ ] provider native DTO 收回 params/shape/snippet/content 行为。

退出标准：

- 无空壳 receiver；
- 相同上下文参数显著减少；
- fs 原子写、BOM/CRLF、patch、grep、download 行为测试全绿；
- provider conformance test 全绿。

### P3：App Runtime 内部 owner 收敛

> 类型：内部重构，中风险

- [ ] reducer 的 commit 判定、interrupt 校验、open-tool drain 迁入真实 owner。
- [ ] transcript FileChange 路径派生迁入 canonical value。
- [ ] `runs.go` 按 admission/recovery 拆文件。
- [ ] artifact decode 与 presenter 按职责拆文件；无状态 projection 保持自由函数。
- [ ] sessions admission/slice projection helper 保持自由函数，不机械 receiver 化。
- [ ] SQLite scan/encode/decode helper 保持自由函数。

退出标准：

- Application/Domain 不新增 Delivery/Infra import；
- durable event 顺序、interrupt/resume/recovery tests 全绿；
- 无 wire/schema 变化。

### P4：App Runtime 工具结果边界纠正

> 类型：结构性内部改动，高风险，需确认

- [ ] 从 Application reducer 删除具体工具名与具体 JSON shape。
- [ ] canonical transcript 保存通用 raw tool fact。
- [ ] Delivery presenter registry 负责已知工具的结构化输出，未知工具 raw fallback。
- [ ] Agentexec/tool adapter 显式上报 mutated paths，Application 不从 UI result 反推。
- [ ] `RunProgress` 携带 tool identity，Delivery 生成活动文案。
- [ ] wire golden 保持不变；必要变化必须重新单独确认。

退出标准：

- `application/runs` 不包含 `shell` / `grep` / `glob` / `web_search` /
  `edit` / `write` 的展示分支；
- 新增工具不要求修改 Application；
- transcript/recovery/import-export/HTTP/inprocess golden 全绿。

### P5：Document Pipeline API 精修

> 类型：破坏性公开 API，需确认

- [ ] 修正 SHA-256 initialism。
- [ ] 缩短 config 字段并让布尔字段表达状态而非 builder 语气。
- [ ] 默认化/校验留在构造器内部。
- [ ] 删除公开 `Nop/NewNop` 多角色表面。
- [ ] 增加 `TransformerFunc` / `BatcherFunc`。
- [ ] 同步 tests、examples、GoDoc 和 API baseline。

已知仓内爆炸半径只在 `documentpipeline` 自身测试/文档；未发现其他生产模块消费。

### P6：Chat History Backend API 精修

> 类型：破坏性公开 API，需确认

- [ ] backend `StoreConfig` → `Config`。
- [ ] backend `NewStore` → `New`。
- [ ] 默认化/校验私有化。
- [ ] 删除无生产消费者的 backend `Provider` 常量。
- [ ] 同步 backend docs/tests。
- [ ] `storetest` 命名和位置不变。

已知仓内爆炸半径集中在各 backend 自身 docs/tests；未发现 App/Agent 生产消费者。

### P7：Provider、MCP、A2A、RAG 与 Desktop 精修

> 类型：混合；包含破坏性 API 和行为，需确认

- [ ] Web provider `NewClient` 接受 `Config` 值。
- [ ] MCP `SamplingViaChatClient` → `NewSamplingHandler`。
- [ ] 修复 MCP sampling 过期注释。
- [ ] A2A tool 参数严格遵守其 object schema；删除 bare-string fallback。
- [ ] RAG `Multi` → `Parallel`，`NoopRetriever` → `NopRetriever`。
- [ ] Desktop 删除无行为的 `App/NewApp/startup/shutdown/Bind`，使用标准日志输出。
- [ ] OTel、Skills、Tokenizer 若无新证据则不改公开面。

已知仓内消费：

- MCP sampling：`agent/examples/mcpagent` 1 处。
- Web provider pointer config：`app/runtime` 2 个生产调用点，provider 自身 tests/docs。
- RAG 两个名字：只在 RAG tests/docs。
- Desktop App：只在 desktop shell。

### P8：最终命名、文件与质量门禁

> 类型：收尾

- [ ] 扫描旧名字、alias、wrapper、dead exported surface。
- [ ] 检查文件名是否准确描述内容，删除 `util/helper/impl` 式残留。
- [ ] 检查所有新增 receiver 是否通过第 3 节门槛。
- [ ] 检查所有保留自由函数是否有明确理由。
- [ ] 更新本计划、模块 docs、API baseline 和 migration notes。
- [ ] 各批独立 commit，可单独 revert。
- [ ] 完整 workspace build/vet/test/lint/race 按受影响矩阵执行。

---

## 7. Breaking Change 决策表

| 决策 | 推荐 | 备选 | 代价 |
| --- | --- | --- | --- |
| Document Pipeline 字段/initialism | 直接改名 | 只修 initialism | 备选会保留冗长 Java 式配置 |
| 删除 `Nop/NewNop` | 删除，增加 Func adapters | 改名为 `Identity` | `Identity` 同时作为 formatter/transformer/batcher 仍含糊 |
| History backend API | `Config` + `New` | 只私有化 defaults | 备选继续保留 package stutter |
| Web provider config | 值传递 | 保留指针 | 指针制造无价值 nil config 状态 |
| MCP sampling | `NewSamplingHandler` | 保留现名 | 现名强调实现路径，不强调产出对象 |
| A2A arguments | 严格 object | 扩大 schema 为 union | 宽松 schema 会偏离统一 Tool object contract |
| RAG aggregator | `Parallel` | `Merge` | `Parallel` 更准确表达执行语义，`Merge` 更强调结果 |
| Runtime tool result | Delivery registry | Application decoder port | port 仍让 Application canonical model承担展示 shape |

确认后直接迁移全部仓内消费者，不保留旧名。

---

## 8. 风险

| 风险 | 级别 | 控制 |
| --- | --- | --- |
| 为 receiver 数量机械搬迁 | 高 | 每项必须通过第 3 节门槛；review 明确记录保留函数 |
| Runtime tool result 改动破坏 wire | 高 | 先锁 golden；canonical 与 wire 分步测试；默认 wire 不变 |
| Public API 大量改名造成遗漏 | 中 | 全仓旧符号扫描 + module standalone test |
| 修改 Core 牵连排除模块 | 高 | 任何有排除模块生产引用的 breaking change 暂缓 |
| Provider config 迁移不一致 | 中 | provider conformance + 构造器矩阵 |
| 文件拆分造成无意义 churn | 中 | 只拆混合关注点；大但内聚文件保留 |
| Fakeweather generator 变成空壳 | 中 | 必须持有共享 RNG/climate/date 状态并减少参数，否则取消 |
| 文件操作语义被通用库改变 | 高 | exact overwrite/atomic replace 合同优先，不机械引入 suffix/no-overwrite 语义 |

---

## 9. 验证矩阵

每批至少执行受影响 module：

```text
go test ./...
go vet ./...
```

高风险批次增加：

- Core API/wire golden；
- Agent architecture/wire tests；
- Runtime protocol golden；
- Runtime reducer/recovery/import-export tests；
- Tools fs race/atomic/path guard tests；
- Provider conformance tests；
- module standalone `GOWORK=off go test ./...`；
- 目标 race tests；
- root `scripts/check.sh` 全门禁。

测试通过不自动证明设计正确。每批 review 还必须回答：

1. 是否减少了概念或重复上下文？
2. 是否把行为挂到了真实 owner？
3. 是否有原本正确的自由函数被错误方法化？
4. 是否新增了只有一个实现却无真实替换需求的接口？
5. 是否留下旧名、兼容 wrapper 或双路径？
6. 第 3.1 节验收卡是否有至少两项可观察改善？
7. 删除 receiver 后是否几乎不损失语义？若是，该 receiver 很可能没有存在价值。

---

## 10. 进度看板

| 批次 | 状态 | 进度 | 阻塞 |
| --- | --- | --- | --- |
| P0 审计与决策门 | 完成 | 100% | — |
| P1 Core 值对象与 snapshot | 进行中 | 0% | — |
| P2 Tools owner 与组织 | 未开始 | 0% | P0 |
| P3 Runtime owner 收敛 | 未开始 | 0% | P0 |
| P4 Runtime 工具结果边界 | 未开始 | 0% | P0，结构性决策 |
| P5 Document Pipeline API | 未开始 | 0% | P0，breaking 决策 |
| P6 Chat History API | 未开始 | 0% | P0，breaking 决策 |
| P7 Provider/MCP/A2A/RAG/Desktop | 未开始 | 0% | P0，breaking/行为决策 |
| P8 最终门禁 | 未开始 | 0% | P1–P7 |

---

## 11. 当前推荐

建议批准 P1–P3、P5–P8 的方向，并把 P4 作为独立高风险批次实施：

1. 先完成低风险 owner/API 收敛，建立更清晰的值与边界。
2. 再单独处理 Runtime tool result，避免与大规模改名混在一起。
3. 每批一个独立 commit，全绿后再进入下一批。
4. 不 push、不 tag、不 release，除非维护者另行授权。
