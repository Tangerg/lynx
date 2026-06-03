# `tools/` — Review 阅读顺序

`tools/` 是 lynx 自带的工具集：FS / Bash / WebFetch / WebSearch /
HTTPReq / FakeWeather 示例。所有工具实现 `core/model/chat.Tool` 接口。
Lyra 默认装载离线工具 (fs + bash)，按 OnlineConfig 开启在线工具。

## 阅读顺序

1. **先确认接口契约**：回到 `core/model/chat/tool.go` 看 `Tool` /
   `ToolDefinition` 形状。
2. `fakeweather/` — 最简单的示例，**先读这个理解套路**：tool struct
   + Definition + Metadata + Call。

## 离线工具（Lyra 默认装载）

3. `fs/` **[精读]**
   - `fs.go` — `Executor` 抽象（按 workdir 限制）。
   - `local.go` — 本地实现（含 path resolve 安全检查）。
   - `format.go` — 输出格式化（路径相对化）。
   - `glob.go` / `grep.go` — 各自的工具（注意 grep 走 ripgrep）。
   - `edit.go` — 文件编辑（diff-style apply）。
   - `errors.go` — 错误集合。
4. `bash/` **[精读]**
   - `bash.go` — `Executor` 抽象。
   - `local.go` — 本地实现。**注意**：lyra 现在没沙箱，后续 M4 沙箱要
     在这一层挂钩。
   - `tool.go` — `chat.Tool` 包装。
   - `errors.go` — 错误集合。

## 在线工具（按需开启，credentials 必填才会注册）

5. `webfetch/`
   - `webfetch.go` + `tool.go` — 接口 + tool wrapper
   - 子目录 `jina/` / `firecrawl/` / `exa/` / `tavily/` — provider 实现
   - `internal/` — 共享 HTTP helper
6. `websearch/` — 同上结构，更多 provider：`brave/` / `exa/` /
   `firecrawl/` / `jina/` / `perplexity/` / `serper/` / `tavily/`。
7. `httpreq/`
   - `httpreq.go` + `tool.go` — 接口 + tool
   - 强制 `AllowedHosts` 白名单，否则不会注册（Lyra 的安全模型）

## 关注点

- **scoping**：fs/bash 的 `LocalExecutor` 是否真把所有路径夹紧在
  workdir 内？符号链接、绝对路径、`..` 等 bypass 是否堵住？
- **InputSchema**：每个 tool 的 schema 是否手写并准确？JSON-Schema
  字段约束应让模型一次写对。
- **Definition.Description**：是否够长 + 给好例子？模型对 description
  的解读直接影响调用质量。
- **Online providers 注册**：credentials 缺失要 silently skip，而不是
  panic — 工具静默禁用是 Lyra 的"显式 opt-in"安全模型。
- **截断**：tool output 应该按行 / 字符截断（避免 token 爆 + 上下文
  污染），Lyra runner 的 `truncateOutput` 是消费侧的兜底。

## 跨模块提醒

- 接口契约：所有 tool 实现 `chat.Tool`，可直接被 `chat.NewToolMiddleware`
  循环调度。
- Lyra 集成：`lyra/internal/engine/tools.go` 的 `BuildToolSet` 把这些工具
  注册到 engine。
- Agent 集成：`agent/runtime/agent_tool.go` 在 process scope 装饰这些 tool。

## 体检命令

- `go test ./tools/...`
- `grep -rn "panic(" tools/` — 应几乎没有；任何 panic 都该问"为什么不
  return error"。
