# `lyra/` — Review 阅读顺序

`lyra/` 是基于 `agent/` 框架搭起来的通用 agent 运行时产品。本 review
顺序按 "**架构文档 → 配置 → 服务接口 → 引擎 → 运行时 façade →
传输 → CLI → 测试**" 推进。

---

## 0. 架构 + 路线（**先读，背景全在这**）

1. `doc/ARCHITECTURE.md` **[必读]** — CS 架构、transport-agnostic
   service 接口、模块拆分、关键决策。
2. `doc/ROADMAP.md` **[必读]** — M0~M7 milestone + 价值指标。Lyra
   当前进度对照这个看。
3. `README.md` — 简介。

---

## 1. 入口 + 配置

4. `cmd/lyra/main.go` — 单文件入口。
5. `cmd/lyra/app.go` **[精读]** — `App` 持有 IO 流 + lazy 加载的
   `runtime.Runtime`。`ensureRuntime` 是所有命令的 lazy bootstrap。
6. `cmd/lyra/root.go` — cobra root 注册所有子命令。
7. `cmd/lyra/version.go` — 版本子命令。
8. `internal/config/config.go` **[精读]** — env var 加载，含
   `LYRA_PROVIDER` / `LYRA_MODEL` / `*_API_KEY` /
   `LYRA_MCP_SERVERS` / `LYRA_HTTP_ALLOWED_HOSTS` 等。
9. `internal/config/chat_client.go` — Provider → `*chat.Client`
   构造 + `EngineOnline` / `EngineMCPServers` 映射函数。

---

## 2. Service 接口层（**transport-agnostic 契约**）

> 每个 service 都是 Go interface (`service.go`) + 实现
> (`impl.go`)。先看接口再看实现。

10. `internal/service/doc.go` — 包导航。
11. `internal/service/session/`
    - `service.go` — `Service` + `Session` + `ErrNotFound`。
    - `repo.go` **[精读]** — 共享 in-memory map + RWMutex 数据层；
      file backend 和 in-memory service 都基于这个 Repo 复用。
    - `inmemory.go` — `Service` 实现（ctx/error 契约）。
    - `errors.go`。
12. `internal/service/chat/` **[精读全部]**
    - `service.go` — `Service` 接口 + 全部 Event 类型 (TurnStart /
      MessageDelta / ReasoningDelta / ToolCallStart/End / ToolCallApproval /
      PlanGenerated / TurnEnd / ErrorEvent) + `stamp` 模式。
    - `impl.go` — `runTurn` 主循环、`turnObserver`、approval gate、
      steering flush、post-turn maintenance。
    - `policy.go` — tool name → SafetyClass / mode → needsApproval 矩阵。
    - `errors.go`。
13. `internal/service/approval/`
    - `service.go` — `Service` 接口（含 `Register` 分裂注册/等待避免
      竞态）+ Mode / Decision / Request 类型。
    - `impl.go` — atomic mode + pending map + 失败封闭的取消语义。
    - `errors.go`。
14. `internal/service/memory/service.go` — LYRA.md 级联读写接口。
15. `internal/service/tool/`
    - `service.go` — ToolService + SafetyClass。
    - `impl.go` — 列出 + 直接调用工具。
16. `internal/service/trace/service.go` — 仅接口（M7 未实现）。

---

## 3. Engine — 核心引擎

17. `internal/engine/doc.go`
18. `internal/engine/engine.go` **[精读]** —
    `Config` / `Engine` / `New` / `Close` / `RunChat` /
    `MaybeCompact` / `MaybeExtract` / `InjectUserMessage`。
19. `internal/engine/agent.go` **[精读]** —
    `buildChatAgent` 构造 `*core.Agent`，action body 是流式累加
    text + reasoning + per-round Usage 的核心。
20. `internal/engine/observer.go` **[精读]** —
    `ToolObserver` (含 OnToolCallApprove / OnToolCallStart/End /
    OnMessageDelta / OnReasoningDelta)、`toolObserverDecorator`、
    `observedTool` 在 Approval 拒绝时把 denial 转成 tool result。
21. `internal/engine/tools.go` — `BuildToolSet` (离线 + 在线工具) +
    `codingToolGroup` + resolver。
22. `internal/engine/mcp.go` — MCP server dial + Provider 包装。
23. `internal/engine/prompt.go` — `SystemPrompt` 由 base prompt +
    LYRA.md 用户 / 项目层级联组合。
24. `internal/engine/compaction.go` — auto-compaction 阈值 + LLM
    summary。
25. `internal/engine/extractor.go` — fact extraction 写回 LYRA.md。
26. `internal/engine/planner.go` — LLM-produced plan 生成（plan
    mode）。
27. `internal/engine/llm.go` — `askDirect` 直接 LLM 调用 helper（绕开
    chat-memory middleware）。

---

## 4. Storage — 持久化（File 后端）

28. `internal/storage/session_store.go` **[精读]** —
    `FileSessionService` 用 Repo + persist-on-mutate hook + rollback
    on failure。
29. `internal/storage/message_store.go` — JSONL 一会话一文件，pathFor
    做 id 安全检查。
30. `internal/storage/memory_store.go` — `<cwd>/LYRA.md` +
    `<home>/LYRA.md` 双 scope。
31. `internal/storage/home.go` / `dirs.go`（如果有）— XDG / `~/.lyra`
    解析。

---

## 5. Runtime — 解耦边界

32. `internal/runtime/runtime.go` **[精读]** — `Runtime` struct
    打包 engine + 5 services；transport 全部走 `*Runtime` 而非各
    service 指针，**这是未来"核心进程 + transport 进程"分离的接缝**。

---

## 6. AG-UI — 协议翻译

33. `internal/agui/translator.go` **[精读]** —
    `Translator` per-turn 状态机：text/reasoning 流的 lazy
    open + close-on-boundary；Step events 包裹工具调用 / plan /
    approval；TurnEnd 排干孤儿 Step。

---

## 7. Transport — 多 adapter 共用 Runtime

34. `internal/transport/http/`
    - `server.go` — `Server` + `Config{Runtime, Addr}` +
      Handler() 路由表 + `s.chat() / s.session() / s.approval()`
      accessor 包装。
    - `agent_run.go` — POST /v1/agent/run SSE 主路径 + POST
      /v1/turns/{id}/steer。
    - `sessions.go` — sessions CRUD。
    - `approvals.go` — approvals list / decide / mode。
35. `internal/transport/ipc/`
    - `server.go` — 行 JSON-RPC server 框架（一行一帧；streaming
      method 多帧 event + done 终止）。
    - `handlers.go` — agent.run / agent.steer / agent.cancel /
      sessions.* / approvals.* / healthz 全部 method 实现。

---

## 8. CLI 子命令

36. `cmd/lyra/chat.go` — 单轮 `lyra chat "..."`。
37. `cmd/lyra/repl.go` — 多轮 REPL，含 `/exit /help /new /plan /session`
    内嵌命令。
38. `cmd/lyra/runner.go` **[精读]** — `TurnRunner` 共用驱动器：
    StartTurn → drain events → 渲染 → SIGINT 取消单 turn 不杀进程；
    含 `renderTurnEnd` 显示 token usage、`renderToolEnd` 截断输出、
    `decidePlan` y/N。
39. `cmd/lyra/session.go` — session list / show / delete。
40. `cmd/lyra/memory.go` — LYRA.md show / edit / list。
41. `cmd/lyra/serve.go` **[精读]** — `lyra serve --listen :8080
    --stdio`，并发跑多 transport，SIGINT/SIGTERM 10s 优雅 drain。

---

## 9. 测试 + 文档

42. `*_test.go` — 重点看：
    - `internal/engine/engine_test.go` — token usage 累加 + 多轮
      memory + 流式 deltas。
    - `internal/service/chat/impl_test.go` — 全 service-level
      契约：事件序列、Seq monotone、plan mode 三路径、steering、
      approval gate 三路径。
    - `internal/transport/http/server_test.go` — HTTP 集成。
    - `internal/transport/ipc/server_test.go` — IPC 集成。
43. `doc/ARCHITECTURE.md` / `doc/ROADMAP.md` — 复习一遍架构 / 进度。

---

## 跨模块提醒

- **解耦边界**：`*Runtime` 是核心运行时与 transport 的接缝。未来 "core
  进程 + transport 进程" 只需写一个 `RemoteRuntime` 实现同样 accessor。
- **AG-UI 覆盖**：translator 现在覆盖了 Run / Text / Thinking / Tool /
  Step / Custom，approval / plan 走 Step + Custom 组合（AG-UI 没有
  first-class 的 plan / approval 事件）。
- **Approval 默认 Yolo**：保持向后兼容；切 safe/balanced 走 HTTP /v1/
  approvals/mode。
- **InjectSteering 是 next-turn 语义**：不是 true mid-stream 注入。
- **TokenUsage 求和靠 round-boundary** (`isToolRoundBoundary` 利用
  `Result.ToolMessage != nil && AssistantMessage == nil`)。
- **MCP**：当前只支持 Streamable HTTP transport，stdio 子进程没做。
- **没沙箱**：与 pi-mono 保持一致，M4 仍待补。
- **InjectSteering / Trace / Reasoning save-to-history** 三个仍是
  M-future。

## 体检命令

- `go test ./... -timeout 60s` — 应全绿。
- `go run ./cmd/lyra serve --help` — 看多 transport flag 是否正确。
- `git log --oneline -20` — 验证 milestone 实现序列与 ROADMAP 对得上。
