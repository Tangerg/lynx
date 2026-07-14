# Core 演进验证与依赖基线

> 基线日期：2026-07-14
> 代码基线：`5a4e828d3`
> Go：`go1.26.4 darwin/arm64`
> 对应计划：[`CORE_ARCHITECTURE_EXECUTION_PLAN.md`](./CORE_ARCHITECTURE_EXECUTION_PLAN.md) P0-04

本文是 P1–P7 的回归比较基准。后续任务不能用“重构幅度大”作为降低测试、覆盖率或依赖门禁的理由；若基线本身较低，只要求被修改 package 不回退，并按计划为新协议纯逻辑补到至少 85%。

## 1. Workspace 基线

`go.work` 当前包含 17 个主 module：

```text
a2a
agent
app/runtime
chathistory
core
documentreaders
documentreaders/html
documentreaders/markdown
documentreaders/pdf
mcp
models
otel
pkg
rag
skills
tools
vectorstores
```

规范入口 `scripts/check.sh` 已改为通过以下 Go workspace 事实动态发现 module，不再维护容易过期的手工数组：

```bash
go list -m -f '{{if .Main}}{{.Dir}}{{end}}' all
```

脚本变更：

- 删除不存在的 `chatmemory`、`lyra`。
- 自动纳入此前漏掉的 `a2a`、`app/runtime`、`chathistory`、`skills`。
- `test` 运行普通 `go test ./...`；新增显式 `race` 检查，避免命令语义隐藏。
- `MODULE=<go.work 相对路径>` 可做单 module 检查，并拒绝未知路径。
- 默认仍运行 build、vet、test、lint、vuln；`FAST=1` 只跳过 vuln。

## 2. Build/Vet/Test/Lint

命令：

```bash
scripts/check.sh build vet test lint
```

结果：`68/68` checks 通过，exit code 0。

| 检查 | 范围 | 结果 |
|---|---|---|
| Build | 17 个 workspace 主 module | PASS |
| Vet | 17 个 workspace 主 module | PASS |
| Test | 17 个 workspace 主 module | PASS |
| Lint | 17 个 workspace 主 module | PASS |

工具版本：

```text
golangci-lint v1.64.8 (built with go1.26.3)
govulncheck v1.3.0, DB updated 2026-07-08
```

P0 没有运行在线漏洞扫描；`govulncheck` 按计划在 P7 发布准备执行，不属于日常阶段退出门禁。

## 3. Race 基线

命令：

```bash
for mod in core agent chathistory rag tools vectorstores; do
  MODULE="$mod" scripts/check.sh race
done
```

| Module | 结果 | 说明 |
|---|---|---|
| core | PASS | 包括 model middleware、history、filter、tokenizer |
| agent | PASS | 包括 runtime、toolloop、workflow、planning |
| chathistory | PASS | 6 个 backend package 均通过或无测试文件 |
| rag | PASS | 单 module 全包 |
| tools | PASS | fs/shell/webfetch/websearch 等全包 |
| vectorstores | PASS | 所有有测试的 backend；无测试 package 明确显示为 no test files |

当前 race 基线没有发现 data race。无测试文件的 provider/backend 只能证明可编译，不能视为并发行为已经验证；P4/P6 必须由统一 conformance suite 补齐。

## 4. Module 覆盖率

统计方式：逐 module 执行 `go test -coverprofile=<temp> ./...`，再由 `go tool cover -func` 取 statement total。

| Module | Statement coverage |
|---|---:|
| a2a | 66.8% |
| agent | 61.9% |
| app/runtime | 58.9% |
| chathistory | 14.3% |
| core | 63.7% |
| documentreaders | 0.0% |
| documentreaders/html | 86.0% |
| documentreaders/markdown | 87.0% |
| documentreaders/pdf | 17.6% |
| mcp | 67.9% |
| models | 56.2% |
| otel | 63.7% |
| pkg | 88.1% |
| rag | 73.7% |
| skills | 85.0% |
| tools | 59.4% |
| vectorstores | 24.6% |

Module total 用于趋势观察；任务级门禁以被修改 package 为准，避免低覆盖的大 provider module 掩盖核心协议回归。

## 5. Core package 覆盖率

命令：

```bash
(cd core && go test -cover ./...)
```

| Package | Statement coverage |
|---|---:|
| `document` | 29.2% |
| `document/id` | 100.0% |
| `evaluation` | 83.7% |
| `internal/arch` | no statements |
| `media` | 85.4% |
| `model` | 95.2% |
| `model/audio/transcription` | 42.1% |
| `model/audio/tts` | 47.8% |
| `model/chat` | 81.5% |
| `model/chat/conversation` | 0.0% |
| `model/chat/history` | 84.8% |
| `model/chat/middleware/history` | 52.9% |
| `model/chat/middleware/logger` | 48.6% |
| `model/chat/middleware/safeguard` | 65.6% |
| `model/embedding` | 61.0% |
| `model/image` | 42.3% |
| `model/moderation` | 46.5% |
| `tokenizer` | 87.1% |
| `vectorstore` | 78.4% |
| `vectorstore/filter` | 5.4% |
| `vectorstore/filter/ast` | 68.6% |
| `vectorstore/filter/lexer` | 84.7% |
| `vectorstore/filter/parser` | 69.1% |
| `vectorstore/filter/token` | 81.2% |
| `vectorstore/filter/visitors` | 70.3% |

P1 新建的 metadata/media/chat validation 与 serialization package、P4 重写的 filter 纯逻辑以 85% 为最低线，不能沿用旧 package 的低基线。

## 6. Core 生产依赖基线

统计不包含 `_test.go` 专用依赖；按 `go list` 的生产 package import graph 计算。

| 指标 | 当前值 | 目标 |
|---|---:|---:|
| Direct non-stdlib import path | 16 | 0；例外必须另立 ADR |
| Direct non-stdlib module root | 7 | 0；例外必须另立 ADR |
| Transitive non-stdlib package | 55 | 随 direct dependency 收敛 |
| Transitive non-stdlib module root | 19 | 随 direct dependency 收敛 |

当前 16 个直接 non-stdlib import path：

```text
github.com/Tangerg/lynx/pkg/assert
github.com/Tangerg/lynx/pkg/io
github.com/Tangerg/lynx/pkg/json
github.com/Tangerg/lynx/pkg/math
github.com/Tangerg/lynx/pkg/mime
github.com/Tangerg/lynx/pkg/ptr
github.com/Tangerg/lynx/pkg/slices
github.com/Tangerg/lynx/pkg/text
github.com/google/uuid
github.com/pkoukk/tiktoken-go
github.com/spf13/cast
go.opentelemetry.io/otel
go.opentelemetry.io/otel/attribute
go.opentelemetry.io/otel/codes
go.opentelemetry.io/otel/metric
go.opentelemetry.io/otel/trace
```

直接依赖归属：

| Core package | Non-stdlib dependency |
|---|---|
| `document` | pkg/io, pkg/slices, cast |
| `document/id` | google/uuid |
| `media` | pkg/mime |
| `model` | OTel trace/metric API |
| `model/audio/transcription` | pkg/ptr |
| `model/chat` | pkg/json, pkg/ptr, pkg/slices, pkg/text, cast, OTel trace API |
| `model/chat/history` | pkg/slices |
| `model/embedding` | pkg/assert, pkg/mime, pkg/ptr, OTel trace API |
| `model/image` | pkg/mime, pkg/ptr |
| `tokenizer` | tiktoken-go |
| `vectorstore/filter` | pkg/math |

`core/go.mod` 还直接声明了 OTel SDK 相关 requirement，但当前生产 package 没有直接 import SDK；它们属于 go.mod hygiene/P6 清理项。ADR-006 已决定 Core 不保留 OTel API/SDK，现有 import 是绑定 P3-06 删除期限的迁移例外。

## 7. 阶段比较规则

每阶段退出时至少重新执行：

```bash
scripts/check.sh build vet test lint
MODULE=<affected-module> scripts/check.sh race
```

并比较：

1. 被修改 package coverage 不低于本文基线。
2. 新协议 validation/serialization/filter 纯逻辑不低于 85%。
3. Core direct non-stdlib import 不增加。
4. 新增 direct dependency 必须有已采纳 ADR；只有“go.mod 已存在”不能作为理由。
5. 无测试 provider/backend 在迁移后必须接入 conformance suite，不能继续只依赖编译通过。
6. 任何失败都记录真实命令、package 和错误；禁止用排除列表改造成“绿”。
