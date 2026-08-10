# Lyra

**Lyra Runtime — 产品级通用 agent 运行时后端（Go）。** 实现 Lyra Runtime Protocol（JSON-RPC 2.0，MCP-inspired），通过 streamable HTTP 服务桌面/Web 客户端，并为 Go CLI/TUI/宿主程序提供公共 embedded binding。

> 模块级上下文见 [`CLAUDE.md`](./CLAUDE.md)；重构目标架构见 [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md)；阶段、当前事实和全部文档入口见 [`doc/README.md`](./doc/README.md)。

---

## 这是什么

以 **Run 生命周期**（而非 agent loop）为中心的 Agent 应用后端。**协议层薄、业务层厚、binding 同源**：公共 `protocol` 是唯一合同，HTTP 与公共 `embedded.Runtime` 共用 binding-neutral operation；`internal/application/*` 驱动 Run/Session/能力生命周期，`internal/adapter/agentexec` 隔离 Agent Framework，`internal/domain/*` 按限界上下文表达产品规则，`internal/infra/*` 提供技术机制。

当前生产执行只消费唯一的 [`agent`](../../agent) Framework Baseline 18，并通过 `internal/adapter/agentexec` 完成防腐翻译。Runtime 不解析 Framework private state，也不复制 Process loop、tree scheduler 或 Tool loop。

## 架构（Clean Arch 同心环，依赖向内，`internal/arch` 机器强制）

```
composition (internal/{bootstrap,config}, cmd)  唯一装配与 Host 生命周期 owner
embedded    (embedded)              公共同进程 binding；只持有 concrete Runtime lifecycle
protocol    (protocol)              公共 binding-neutral values / validation / errors
delivery    (internal/delivery)     operation / server / dispatch / HTTP transport
adapter     (internal/adapter/*)     应用能力与外部 SDK 的防腐/翻译
application (internal/application/*) Run / Session / capability use cases 与 consumer ports
infra       (internal/infra/*)       sqlite / git / lsp / mcp / a2a / exec 等技术 mechanism
domain      (internal/domain/*)      entity / value / aggregate behavior / pure domain policy
```

依赖一律向内（Domain 是核心）；Application 依赖 Domain 和消费方端口，Adapter/Infra 实现外部能力，Delivery 只驱动 Application，Bootstrap 是唯一组合根。详见 [`doc/ARCHITECTURE.md`](./doc/ARCHITECTURE.md)。

## 能力（现状）

Framework-managed Interaction · nested child checkpoint 精确
pause/resume · HITL 审批/提问 · plan 模式 · LSP 代码智能 · read-before/stale 编辑保护 ·
worktree 与 Git checkpoint · MCP client/server bridge · A2A 远端 agent · Agent Skills ·
LYRA.md 长期知识与提取 · model-facing plan · per-run provider+model 显式选择 ·
token 触发上下文压缩 · OTel trace/metric/log → slog。

## 跑起来

```bash
cd app/runtime                                         # 从仓库根进入 runtime 模块
go build ./... && go vet ./... && go test ./...        # 全绿
ANTHROPIC_API_KEY=xxx ./lyra                           # 默认 127.0.0.1:17171（匹配前端默认 base），SQLite at $LYRA_HOME/lyra.db
```

## 嵌入 Go 程序

外部宿主只导入公共 [`protocol`](./protocol) 与 [`embedded`](./embedded)，不经过 HTTP、JSON-RPC 或 SSE：

```go
rt, err := embedded.Open(ctx, embedded.Config{
    DataDirectory:        dataDirectory,
    DefaultWorkspacePath: workspace,
})
if err != nil {
    return err
}
defer rt.Close()

session, err := rt.CreateSession(ctx, protocol.CreateSessionRequest{
    Workspace: &protocol.WorkspaceRef{Path: workspace},
}, embedded.CommandOptions{IdempotencyKey: requestID + ":session"})
if err != nil {
    return err
}

started, events, err := rt.StartRun(ctx, protocol.StartRunRequest{
    SessionID: session.ID,
    Input: []protocol.ContentBlock{{
        Type: protocol.ContentBlockText,
        Text: prompt,
    }},
}, embedded.RunCommandOptions{IdempotencyKey: requestID + ":run"})
if err != nil {
    return err
}
for event, streamErr := range events {
    // Fold protocol.RunEvent into the host's own presentation model.
    _ = event
    if streamErr != nil {
        return streamErr
    }
}
_ = started
```

`Runtime` 是数据目录、恢复器与后台任务的唯一 owner；同一 canonical 数据目录同一时刻只能由一个进程打开，宿主必须完成 `Close`。调用错误支持 `errors.Is(err, protocol.Err…)`；需要结构化恢复信息时，可用 `errors.As` 取得 `protocol.ProblemError`。CLI/TUI 应在消费侧定义自己真正需要的窄接口，不复制协议 DTO，也不要求 Runtime 导出胖接口。

## 不做（刻意）

不复制 HTTP 与 embedded 业务入口 · 不复活 JSON-RPC channel 式伪 in-process transport · 不公开 Host/Store/Engine/Application concrete · 不做 stdio/gRPC binding · 不做用户鉴权/多租户（协议层零 user 概念）· 不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。
