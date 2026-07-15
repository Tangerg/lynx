# Core v1 协调发布顺序与执行手册

> 目标：发布 `github.com/Tangerg/lynx/core` v1.0.0，并让所有 dependent module 在关闭 `go.work` 时也能独立解析和测试
> 当前状态：P7-07 最终架构审查已通过，Core v1 契约已冻结；tag 尚未创建，发布人按本手册执行
> 原则：按真实 module DAG 自底向上发布；不使用 `replace`、兼容 module、路径桥接或未推送提交

仓库采用多 module 布局。位于子目录的 Go module 必须使用带目录前缀的 Git tag，例如 Core v1.0.0 的 tag 是 `core/v1.0.0`，而不是仓库根 `v1.0.0`。

## 1. 发布边界

- 本次冻结为 v1 的只有 `core` module。
- `chatclient`、`models`、`tools`、`agent` 等 dependent module 仍可保持 v0；它们需要发布一个明确依赖 Core v1 的协调版本。
- `app/runtime` 是最终集成和部署目标，不构成 Core v1 公共契约。
- `app/desktop` 不依赖 Go workspace 内部 module，不进入本次发布链。
- 当前仓库没有历史 Git tag。首个 dependent module 标签建议从各自的 `v0.1.0` 开始；如果发布负责人另定版本，只能改变版本号，不能改变拓扑顺序。

## 2. 当前直接依赖 DAG

以下关系来自各 module 当前 `go.mod`，不是概念图：

```text
core ------> chatclient/chathistory/otel/documentreaders/*/vectorstores/
             tools/documentpipeline/models/rag/a2a/mcp/agent/app-runtime
pkg -------> a2a/agent/documentreaders/mcp/models/rag/tools/vectorstores/app-runtime
skills ----> tools/app-runtime
tokenizer -> documentpipeline/models/app-runtime

chatclient -> rag/mcp/agent/app-runtime
chathistory -----------------> agent/app-runtime
tools ------> a2a/mcp/agent/app-runtime
mcp -------------------------> agent/app-runtime
a2a -------------------------------> app-runtime
models/otel -----------------------> app-runtime
agent -----------------------------> app-runtime
```

箭头表示“左侧必须先有可解析版本，右侧才能更新依赖并发布”。

## 3. 发布波次

同一波次内没有互相依赖的 module 可以并行准备，但每个 tag 仍必须指向已通过门禁的同一协调基线。

### Wave 0：无内部依赖的基础 module

| Module | 建议首版 tag | 说明 |
|---|---|---|
| `core` | `core/v1.0.0` | 本次唯一 v1 冻结契约 |
| `pkg` | `pkg/v0.1.0` | dependent modules 的通用 helper；Core 不依赖它 |
| `skills` | `skills/v0.1.0` | `tools` 的输入依赖 |
| `tokenizer` | `tokenizer/v0.1.0` | `models`/`documentpipeline` 的可选实现边界 |

Core 的 P7-07 审查已经完成；发布人核对最终提交和记录后才创建 `core/v1.0.0`。其余三个 module 不属于 Core v1 契约，但为摆脱 pseudo-version、形成可复现依赖图，应在后续波次前发布。

### Wave 1：直接依赖 Core 的叶子/适配模块

| Module | 必须先更新的内部依赖 |
|---|---|
| `chatclient` | `core v1.0.0` |
| `chathistory` | `core v1.0.0` |
| `otel` | `core v1.0.0` |
| `documentreaders` | `core v1.0.0`、已发布的 `pkg` |
| `documentreaders/html` | `core v1.0.0` |
| `documentreaders/markdown` | `core v1.0.0` |
| `documentreaders/pdf` | `core v1.0.0` |
| `vectorstores` | `core v1.0.0`、已发布的 `pkg` |

Wave 1 完成后，先关闭 `go.work` 分别执行独立 module 测试，再创建对应 v0 tag。

### Wave 2：组合模块

| Module | 必须先更新的内部依赖 |
|---|---|
| `tools` | `core v1.0.0`、已发布的 `pkg`/`skills` |
| `documentpipeline` | `core v1.0.0`、已发布的 `tokenizer` |
| `models` | `core v1.0.0`、已发布的 `pkg`/`tokenizer` |
| `rag` | `core v1.0.0`、已发布的 `chatclient`/`pkg` |

`models` 发布前必须跑 provider constructor/protocol conformance；`vectorstores` 虽在 Wave 1，发布前必须跑 27 backend conformance。

### Wave 3：协议桥接模块

| Module | 必须先更新的内部依赖 |
|---|---|
| `a2a` | 已发布的 `core`/`pkg`/`tools` |
| `mcp` | 已发布的 `chatclient`/`core`/`pkg`/`tools` |

### Wave 4：Agent 运行时库

| Module | 必须先更新的内部依赖 |
|---|---|
| `agent` | 已发布的 `chatclient`/`chathistory`/`core`/`mcp`/`pkg`/`tools` |

### Wave 5：应用集成

`app/runtime` 最后更新到已发布的 `a2a`、`agent`、`chatclient`、`chathistory`、`core`、`mcp`、`models`、`otel`、`pkg`、`skills`、`tokenizer` 和 `tools` 版本，关闭 workspace 后完成构建、测试与部署验证。它不应成为任何库 module 的依赖。

## 4. 每个 module 的发布循环

对每个波次重复以下过程：

1. 将 `go.mod` 中已发布的 Lynx 内部依赖从 pseudo-version 更新为精确 tag。
2. 不添加 `replace`；运行 `go mod tidy` 并检查 `go mod tidy -diff` 为空。
3. 临时关闭 workspace 影响：

   ```bash
   GOWORK=off go test ./...
   GOWORK=off go vet ./...
   ```

4. 运行该 module 的 lint/race/conformance 门禁。
5. 提交并推送依赖更新，确认远端 commit 与本地 HEAD 一致。
6. 用正确的目录前缀创建 annotated tag，例如：

   ```bash
   git tag -a core/v1.0.0 <commit> -m "core v1.0.0"
   git push origin core/v1.0.0
   ```

7. 在干净目录中使用 Go proxy 解析该 tag，确认不是由本地 `go.work` 或 module cache 偶然兜底。
8. 只有当前波次全部可从远端解析后，才能更新下一波次。

不要一次创建所有 tag 再补 `go.mod`。那会让 tag 永久指向 pseudo-version 图或未发布依赖。

## 5. Core v1 发布门禁

Core tag 前必须在最终提交上运行统一门禁：

```bash
scripts/check-core-release.sh
```

该脚本串行执行下列 Core、Models、VectorStores 和 fuzz 检查；以下命令用于定位失败或人工复核：

```bash
cd core
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
golangci-lint run ./...
go mod tidy -diff
go test -count=1 ./internal/arch \
  -run '^(TestExportedAPIMatchesBaseline|TestWireContractsMatchGolden|TestWireTypeCoverage|TestWireDTOFieldsExcludeArbitraryRuntimeValues|TestPublicPackagesHaveDocsAndRunnableExamples|TestTargetPackagesHaveNoExternalDependencies|TestCoreProductionImportsAreStandardLibraryOnly)$'
```

另外从仓库根执行：

```bash
scripts/check-core-coverage.sh
FAST=1 scripts/check.sh build vet test lint race
```

Fuzz release gate 必须让以下 7 个 target 各独立运行至少 5 分钟：Metadata Map JSON、Media JSON、Filter Parse、Chat Part/Message/Request/Response JSON。不得用一次 5 分钟的 package fuzz 在多个 target 间共享预算。

## 6. Dependent module 门禁

除各 module 普通 build/vet/test/lint/race 外，至少执行：

- Models：30 个公开 Chat provider/facade 构造入口、共享 protocol/mapping suite、非 Chat modality 请求边界验证与 `<provider>/options` key 架构门禁，以及 Anthropic、Bedrock、Google、Ollama、OpenAI 五类参考实现。
- Vectorstores：自动发现的 backend 集合必须与 27 项发布清单一致，并在 race 下逐个执行共享 conformance。
- ChatClient：Call/Stream、同步资源释放、middleware 顺序、template 与 structured output。
- Agent：唯一 Event Runner、tool error/abort、pause/checkpoint/resume、HITL 与 race。
- ChatHistory：当前 wire codec、defensive snapshot、完整 tool-call round 持久化及 backend contract。
- App Runtime：关闭 `go.work` 后独立 build/test，并验证 provider 装配、history、HITL 和 transport。

## 7. 漏洞与供应链裁决

发布基线使用 Go 1.26.5，已清除 Go 1.26.4 中可达的 `crypto/tls` 公告。

`models` 和 `app/runtime` 当前通过 Ollama v0.32.0 间接命中 8 个上游 `govulncheck` 公告：`GO-2025-3557`、`GO-2025-3558`、`GO-2025-3559`、`GO-2025-3582`、`GO-2025-3689`、`GO-2025-3695`、`GO-2025-3824`、`GO-2025-4251`；扫描结果均为 `Fixed in: N/A`。处理规则：

1. Core v1 本身不依赖 Ollama 或任何第三方 module，因此该风险不阻塞 Core tag。
2. `models`/`app/runtime` release notes 必须列出准确公告、可达调用链和部署暴露面。
3. Ollama adapter 不应被默认装配到不需要它的部署；实际使用方按风险接受、隔离或延期发布该 adapter。
4. 一旦上游发布修复版本，先升级并重跑 provider conformance/govulncheck，再发布新的 Models/App 版本。
5. 禁止用本地 fork、虚假版本、空实现或忽略整个漏洞扫描来制造“全绿”。

`GO-2026-5932` 当前只作为非可达 module finding 出现在 Models、App Runtime、ChatHistory 与 VectorStores。它不阻断当前发布，但必须保留在扫描输出和发布记录中；若未来出现可达调用链，精确漏洞门禁会按“非 allowlist 可达漏洞”立即失败。

## 8. 回滚策略

- Core tag 一经推送不可移动或覆盖。如果发现契约错误，按 SemVer 发布 v1.0.1；破坏性修正只能进入新的 major。
- dependent module 某一波失败时，停止后续波次；已发布版本保持不变，以新 patch/minor 修复。
- 应用回滚使用上一套完整 module 版本集合和旧数据存储，不让一个运行实例混用新旧 wire。
- 数据迁移回滚依赖升级前的只读快照；库中不增加双读或旧 decoder。

## 9. 发布记录模板

每个 tag 记录：

```text
Module:
Version/tag:
Commit:
Required internal versions:
GOWORK=off test/vet result:
Race/conformance result:
govulncheck result and accepted risks:
Release notes URL:
Publisher/date:
```

最终版本集合应写入同一个 release manifest，供 `app/runtime` 和外部消费者复现。
