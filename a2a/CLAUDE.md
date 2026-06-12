# CLAUDE.md — a2a module

> Bridge between the Agent-to-Agent (A2A) protocol and lynx — both client (consume a remote A2A agent as a `chat.Tool`) and server (expose a lynx capability as an A2A endpoint).
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

`github.com/a2aproject/a2a-go/v2` 的 lynx 适配层（mcp 模块的姊妹）：把远端 A2A agent 包装成 `core/chat.Tool` 给 agent 委派，也把 lynx 能力暴露成 A2A endpoint。**协议层不重新发明**，靠官方 SDK；wire 形态（JSON-RPC envelope / SSE / AgentCard schema / 方法名）全是 SDK 的事。

## 技术栈

- Go 1.26.4
- `github.com/a2aproject/a2a-go/v2`，三个子包别名/原名：核心类型 `sdka2a`、server `a2asrv`、client `a2aclient`（`sdka2a` 别名避免和本包名 `a2a` 冲突，同 mcp 的 `sdkmcp`）
- `go.opentelemetry.io/otel` 1.43
- `core/model/chat` —— 复用 `chat.Tool` 接口
- ~0.5k LOC

## 核心架构

**Client 侧（消费远端 A2A agent）**
- **`transport.go`** —— `Dial(ctx, ClientConfig)`：resolve 远端 AgentCard + 开 `a2aclient.Client`（默认 JSON-RPC + REST 两种 HTTP transport）
- **`agent_tool.go`** —— `AgentTool` 包装器：一个远端 agent → 一个 `chat.Tool`（`Call` → `SendMessage` → 抽回复文本）。输入 schema 统一 `{"message": string}`（A2A 是发消息,不是 typed call）
- **`transport.go`** —— client transport：`Dial`(单个)+ `DialAll` 一次拨号多个远端 agent 直接产出 `[]chat.Tool`，`CloseClients` 收尾。**无 Provider 包装层** —— A2A agent 是静态的（不像 MCP 工具列表会变），没有 cache/refresh 要管，故不照搬 `mcp.Provider`（那是被真实差异证成的分叉）

**Server 侧（暴露 lynx 能力）**
- **`executor.go`** —— 窄接口 `Agent`(文本进/流式文本出)→ `a2asrv.AgentExecutor`,按规范 task 生命周期(submitted → working → artifact deltas → completed/failed)
- **`server.go`** —— `NewHTTPHandler`：挂 JSON-RPC 方法端点(默认 `/invoke`)+ well-known AgentCard

**共用**
- **`content.go`** —— codec：`sdka2a.Part`/`Message`/`Task` ↔ 文本(text-first)
- **`errors.go` / `tracing.go`** —— sentinel + `RemoteAgentError` / OTel span(`lynx/a2a`)

## 关键接口/类型

- `Agent` —— **server 侧窄接口**(`Run(ctx, input string) iter.Seq2[string, error]`),由消费方(lyra/agent)实现。**定义在本包内**(接口在消费方),所以模块只依赖 `core/model/chat` + SDK,**不依赖 `agent`/`lyra`**
- `AgentTool` —— 包装远端 agent,实现 `chat.Tool`
- `DialAll` —— client 一次拨号多个远端 agent → `([]chat.Tool, []*client, error)`,按名去重;无中间 Provider/Source 类型(去掉了空壳聚合层)
- `ClientConfig` / `AgentToolConfig` / `ServerConfig` —— 都带 `Validate()`/`ApplyDefaults()`(充血,同 `mcp.ToolConfig`)
- `RemoteAgentError` —— 远端 task 落 failed/rejected/canceled 终态(`errors.As` 区分于 transport 失败)

## 强约定

- **传输默认 JSON-RPC over HTTP** —— 与 lyra 既有传输一致;SDK 的 REST/gRPC 不回避但不主动接线
- **接口在消费方**：`Agent` 在本包定义,模块零 `agent`/`lyra` 依赖(同 mcp 纪律)
- **executor 首个事件必须是 Task 或 Message**(SDK `taskupdate.Manager.Process` 硬约束)→ 先 `NewSubmittedTask`,再 status/artifact;空 artifact 会被 SDK 拒(`chunk==""` 跳过);必须到终态
- **`SendMessage` 默认阻塞到终态**(不设 `Config.ReturnImmediately`),所以 client 一次拿到最终结果
- **包名 alias**：核心类型统一 `import sdka2a "github.com/a2aproject/a2a-go/v2/a2a"`
- **`content.go` 的自由函数不是贫血**：操作的是 SDK 拥有的类型(`sdka2a.Part` 等),Go 不许给外部类型加方法 → 只能自由函数(同 `mcp/content.go`)

## 关键目录

```
a2a/
├── transport.go    [client] Dial / DialAll / CloseClients (resolve card + 开 client + 产出 []chat.Tool)
├── agent_tool.go   [client] 远端 agent → chat.Tool
├── executor.go     [server] Agent → a2asrv.AgentExecutor
├── server.go       [server] NewHTTPHandler(JSON-RPC + well-known card)
├── content.go      codec: sdka2a 类型 ↔ 文本
├── errors.go       sentinel + RemoteAgentError
├── tracing.go      OTel(lynx/a2a)
└── doc.go          包说明
```

## 常用命令

```bash
go build ./...
go test ./...   # a2a_test.go 跑端到端 round-trip(stub Agent → server → client tool call)
```

## 修改任何东西之前

- **本包是 thin wrapper**：加新 A2A 能力前先在官方 a2a-go(`a2asrv`/`a2aclient`/`a2a`)看接口形状,不维护自己的协议状态;不要自己写 JSON-RPC envelope / SSE 帧 / AgentCard schema
- **改 executor 事件序列**：对照 SDK `internal/taskupdate/manager.go` 的 `Process` 校验(首事件 Task/Message、空 artifact、终态后不许更新)
- **lyra 消费**：client 侧已接(`lyra/internal/engine/a2a.go::dialA2AAgents`,`LYRA_A2A_AGENTS` env);server 侧(把 lyra 暴露成 A2A endpoint)接线另见 lyra
- **多轮 / input-required**：当前 `Agent` 是一次性(文本进/流式文本出),不支持 A2A 的 `input-required`/`auth-required` 多轮中断 —— 真要做会牵动 `Agent` 接口形状,单独设计
