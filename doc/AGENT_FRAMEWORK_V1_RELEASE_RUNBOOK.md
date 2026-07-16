# Agent Framework v1 发布执行手册

> 目标 module：`github.com/Tangerg/lynx/agent`
> 候选版本：`v1.0.0`
> 候选 tag：`agent/v1.0.0`
> 当前状态：开发期候选；未形成发布提交，未授权 tag、push 或 release

## 1. 版本裁决

当前架构具备 v1 所需的完整所有权：Engine、Deployment、Process、Interaction 和 Persistence 都有唯一 owner，真实 App 已迁移，API 与 wire 可机械审查。因此首个稳定候选仍为 `v1.0.0`，而不是用 v0 掩盖已经需要认真管理的公共合同。

这不表示当前工作树已经可发布。v1 承诺只从正式 tag 开始，也不承诺兼容任何旧开发版本。

Go module path 在 v1 保持 `github.com/Tangerg/lynx/agent`，不增加 `/v1`。多 module 仓库的 tag 必须使用 `agent/v1.0.0`，不能使用根 `v1.0.0` 代表 Agent module。

## 2. 当前发布阻塞项

1. 当前改动尚未形成并推送经过审查的发布 commit。
2. `agent/go.mod` 的 Lynx 内部依赖仍是 pseudo-version。
3. 上游 module 正式 tag 尚未从远端验证。
4. 当前 workspace 门禁已通过，但正式发布仍需在精确依赖版本下以 `GOWORK=off` 重跑。
5. 维护者尚未授权 commit、tag、push 或 release。

禁止用本地 `replace`、go.work 或未推送 commit 伪造最终 module DAG。

## 3. 当前公共基线

| 项目 | 候选值 |
|---|---:|
| public package | 16 |
| exported declaration | 626 |
| root façade | 46 / 50 |
| exported JSON struct | 14 |
| wire fixture | 456 行 |

- API SHA-256：`119e3e754688a922918e7fb257495e58aee9542a5d88382508f9f9ad8a6ef5af`
- wire SHA-256：`324d63d70cc6f7bb613028f7c470ba089ecbec4fcd8b1ff04fe91bfd7bb3a5ea`

公开 contract test package 只有 `agent/storetest`。不存在 `agent/providertest`。

## 4. 直接内部依赖

当前 `agent/go.mod` 直接依赖以下 Lynx module：

| 依赖 | v1 发布前候选版本 |
|---|---|
| `core` | `core/v1.0.0` |
| `pkg` | `pkg/v0.1.0` |
| `chatclient` | `chatclient/v0.1.0` |
| `chathistory` | `chathistory/v0.1.0` |
| `tools` | `tools/v0.1.0` |
| `mcp` | `mcp/v0.1.0` |

`mcp` 进入 module graph 是因为同 module examples；Agent Framework 生产包仍由架构测试禁止 import transport SDK。

上游发布拓扑以 [`CORE_V1_RELEASE_RUNBOOK.md`](./CORE_V1_RELEASE_RUNBOOK.md) 为准。Agent tag 必须晚于全部直接依赖 tag。

## 5. 发布波次

### Wave A：上游版本

1. 发布并验证 `core`、`pkg` 与其他基础 module。
2. 发布并验证 `chatclient`、`chathistory`、`tools`。
3. 发布并验证 `mcp`。
4. 确保每个版本在 `GOWORK=off` 的干净环境中可下载。

### Wave B：Agent 发布候选

1. 将 `agent/go.mod` 更新为已发布精确版本，无 `replace`。
2. 执行 `GOWORK=off go mod tidy` 并审查 `go.mod` / `go.sum`。
3. 运行完整门禁。
4. 重新核对 API / wire baseline、hash 与文档。
5. 运行 fuzz 目标的发布预算。
6. 审查工作树范围并形成发布 commit。
7. 推送 commit，核对远端 SHA。

最低门禁：

```bash
cd agent
GOWORK=off go mod tidy -diff
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -count=1 ./...
GOWORK=off go test -race -count=1 ./...
GOWORK=off golangci-lint run ./...
GOWORK=off go test -count=1 ./internal/arch
shasum -a 256 internal/arch/testdata/exported_api.txt
shasum -a 256 internal/arch/testdata/wire_contracts.golden.json
```

如果依赖更新导致源码、API 或 wire 改变，停止发布并重新做架构与 SemVer 审查，不能只刷新 baseline。

### Wave C：Agent tag

只有维护者单独授权后执行：

```bash
git tag -a agent/v1.0.0 <release-commit> -m "agent v1.0.0"
git push origin agent/v1.0.0
```

随后在干净环境验证：

```bash
GOWORK=off go mod download github.com/Tangerg/lynx/agent@v1.0.0
```

再创建临时 consumer module，至少 import 根 `agent`、`agent/runtime` 与 `agent/storetest`，执行 `go test ./...`。

### Wave D：App Runtime

1. 更新 `app/runtime/go.mod` 到正式 Agent 版本和同一 release manifest 的内部依赖。
2. SQLite ProcessStore adapter test 直接运行 `storetest.TestProcessStore`。
3. 在 `GOWORK=off` 下运行 App build、vet、test、lint、tidy 与高风险 race。
4. 执行 snapshot 数据迁移 dry-run 和部署验证。
5. 经单独授权后才操作真实数据库或发布应用。

## 6. 发布清单

- [ ] 上游内部依赖 tag 已发布且可从远端解析。
- [ ] Agent go.mod 只引用精确版本，无 `replace`。
- [ ] Agent build、vet、test、race、lint、tidy 全绿。
- [ ] API 626 条、root 46 / 50；wire 14 struct、456 行。
- [ ] API / wire hash 与候选值一致。
- [ ] architecture guard 与 fuzz 预算通过。
- [ ] migration、release notes、review、runbook 指向同一基线。
- [ ] 发布 commit 已推送且远端 SHA 已核对。
- [ ] 维护者明确授权创建和推送 tag。
- [ ] 干净 consumer 可编译 `agent`、`agent/runtime`、`agent/storetest`。
- [ ] App 依赖更新与数据迁移另有授权和验证。

## 7. 回滚

### Tag 前

停止当前波次、修复并重新跑门禁。未推送的本地 tag 可以删除；不要把失败候选推到目标 tag。

### Tag 已推送、App 未升级

不移动或删除正式 tag。兼容修复发布 patch；新增兼容能力发布 minor；破坏性修正进入新 major。

### App 已升级

应用回滚使用上一套完整二进制、module manifest 和数据备份。不要混用新 Agent runtime 与旧 Host，也不要临时加入旧/新 snapshot 双读：

- 恢复升级前 snapshot 数据，或明确丢弃新 snapshot 并终止相关非终态运行；
- Session、消息和 terminal history 按其独立兼容策略处理；
- 多节点先撤销新 worker 的 lease/fencing，再启动旧 worker。

## 8. v1 兼容政策

- `v1.0.x`：兼容 bug、安全、性能和文档修复。
- `v1.x`：向后兼容的新 API；给公开 interface 增方法通常是 breaking。
- `v2`：module path 使用 `github.com/Tangerg/lynx/agent/v2`，tag 使用 `agent/v2.x.y`。
- 已推送 tag 永不移动。
- API / wire baseline 差异必须先有兼容判断、迁移说明和版本裁决。

## 9. 发布记录模板

```text
Module: github.com/Tangerg/lynx/agent
Version/tag: agent/v1.0.0
Release commit:
Required internal versions:
GOWORK=off build/vet/test/race/lint/tidy:
API/wire hash:
Fuzz targets and duration:
Clean consumer result:
Known risks:
Migration/release notes:
Publisher/date:
App rollout manifest:
```
