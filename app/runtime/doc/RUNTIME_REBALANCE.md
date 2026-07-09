# Runtime Rebalance — 组合根减重与领域重心校准

> 日期：2026-07-09。状态：执行中。目标不是重写 `app/runtime`，而是在现有 Clean Arch + DDD 骨架上做小批次结构精修，让代码阅读重心更符合“协议薄、应用编排清楚、领域规则归位”。

## 目标

1. **降低头部清单感**：`cmd/lyra` 与 `internal/runtime` 负责启动和装配，允许重，但不应把大段配置字面量、端口投影和能力接线混在一个函数里。
2. **保持行为不变**：不改协议、不改 store schema、不改 exported API 形状，不做兼容 shim，也不引入 migration。
3. **继续把规则往正确层收**：领域不变量进 `domain/*`；跨 bounded context 的写集进 `kernel/*`；delivery 只做 wire 翻译和调用 use case。
4. **不硬造“厚 domain”**：CRUD/registry 型上下文保持轻；只有真实规则和不变量才进入充血模型。

## 非目标

- 不新建独立 `application/` 环；`kernel` 已是应用层 / 微内核。
- 不把 `Store` / `Registry` 改名成 `Repository`。
- 不引入 Domain Event bus、DI container、CQRS/Saga。
- 不为视觉平衡拆碎内聚良好的 domain 包。

## 判断标准

- 一个改动如果只让目录树看起来“更 DDD”，但没有降低认知负担或收回规则，不做。
- 一个规则如果需要同时读 delivery/runtime/kernel 才能理解，优先寻找能否下沉到 domain 或 kernel use case。
- 一个装配函数如果主要是“把已有依赖摆到 Config 里”，优先抽成命名 helper，而不是塞进 CLI 入口。

## 批次计划

- [x] **Batch 1：cmd/lyra runtime 配置构建下沉**
  - 把 `ensureRuntime` 里的 `lyraruntime.Config` 大字面量抽成专门构建函数。
  - CLI 保留：加载配置、建默认 client、建 stores、seed、创建 hook resolver、调用 runtime。
  - 预期收益：`runtime_bootstrap.go` 从“启动 + 装配细节”变成“启动流程”。

- [x] **Batch 2：internal/runtime.New 命名化装配步骤**
  - 把 history/conversation、utility/embedding、tool/MCP/approval 等局部装配变成命名步骤。
  - 保留 `Runtime` 作为 transport-facing facade，不拆公开结构。
  - 预期收益：组合根仍集中，但每段能力边界更容易复查。

- [x] **Batch 3：寻找真实领域下沉点**
  - 只在发现具体规则外泄时做；当前候选是 `transcript` 的 wire blob 味道和 session/run 边界语义。
  - 需要单独评估 blast radius；本批不强行执行。

## 执行记录

### 2026-07-09

- [x] 建立本跟踪文档。
- [x] Batch 1：新增 `cmd/lyra/runtime_config.go`，把 `ensureRuntime` 中的 `lyraruntime.Config` 大字面量抽成 `buildRuntimeConfig`。`runtime_bootstrap.go` 现在保留启动顺序：load config → build client → build stores → seed registries → build hooks → `runtime.New`。
- [x] Batch 2：新增 `internal/runtime/engine_wiring.go` 与 `facade_wiring.go`，把 engine config/message history/tool env 注入、Runtime facade 端口投影从 `New` 的主流程中拿出。`New` 现在只保留装配流程和错误处理。
- [x] Batch 3：新增 `transcript.Timeline` 值对象，把 rollback/fork 边界算法提升为 `Timeline.BoundaryAt`；保留原 `transcript.BoundaryAt` 函数作为兼容入口。`kernel/lifecycle.ResolveRollbackBoundary` 改为调用领域对象方法。
- [x] 局部验证：`go test ./internal/domain/transcript ./internal/kernel/lifecycle ./internal/runtime/... ./cmd/lyra` 通过。
- [x] 全量验证：`go build ./... && go vet ./... && go test ./...` 通过；`golangci-lint run` 通过。
