# CLAUDE.md — tools module

> 给 LLM 调用的具体工具集 —— bash / 文件系统 / HTTP / 网页抓取 / 网页搜索 / 假天气.
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

实现 `core/model/chat.Tool` 接口的工具集合。**两层 SPI**：Tool 层做 JSON in/out + schema + LLM 交互；Executor / Provider 层做真正执行（本地 / 远程 / 沙箱后端可换）。

## 技术栈

- Go 1.26.3
- `github.com/go-resty/resty/v2`（HTTP client）
- 依赖 `core` / `pkg`，内部 schema 走 `pkg/json.StringDefSchemaOf` 自动生成
- ~7k LOC / 60 文件 / 21 子目录
- 无框架，纯接口 + 函数式组装

## 核心架构（两层 SPI）

**Tool 层**（实现 `chat.Tool`，暴露给 LLM）：
- `Definition() ToolDefinition` —— Name + Description + InputSchema(JSON)
- `Metadata() ToolMetadata`
- `Call(ctx, arguments string) (string, error)` —— JSON in / out

**Executor 层**（后端 SPI，可换实现）：
- `bash.Executor` —— `Run(ctx, RunInput) (RunOutput, error)`
- `fs.Executor` —— `Read / Write / Edit / Glob / Grep` 五方法
- `httpreq.Client` —— 配置 AllowedHosts + AllowedMethods，守卫执行
- `websearch.Provider` / `webfetch.Provider` —— `Name() string` + 动作方法（Search / Fetch）

**Registration**：调用方手动 `ToolSupport.Register(tool1, tool2, ...)`，**没有全局 registry**。

## 关键接口/类型

1. **`chat.Tool`** —— Definition / Metadata / Call
2. **`chat.ToolDefinition`** —— Name（snake_case 唯一）/ Description / InputSchema（JSON Schema 字符串）
3. **`bash.Executor`** / **`fs.Executor`** —— 后端 SPI
4. **`httpreq.Client`** —— allowlist-guarded HTTP
5. **`websearch.Provider` / `webfetch.Provider`** —— 一致 Provider 接口

## 强约定

- **工具名 snake_case 一目了然**：`bash` / `read` / `write` / `edit` / `glob` / `grep` / `http_request` / `web_search` / `web_fetch` / `weather_query`
- **错误处理用包级 sentinel**：`ErrEmptyCommand` / `ErrHostNotAllowed`，调用方 `errors.Is()` 匹配
- **非零退出码不算错**：`bash` 返回 `RunOutput` 里带 `ExitCode`，调用方决定如何处理
- **输出 JSON 序列化**：Call 返 JSON string，框架反序列化喂给 LLM
- **SPI 职责分工**：Tool 只 JSON 序列化 / schema 校验 / LLM 交互；所有业务逻辑（行号 / binary 检测 / 写锁 / path 锚定）都在 Executor → remote backend 可独立优化（不用往返整文件）
- **Nil-safety 双标**：
  - `bash` / `fs` / `fakeweather` 的 `NewXxxTool(nil)` 默认 LocalExecutor（开箱即用）
  - `websearch` / `webfetch` / `httpreq` 的 `NewTool(nil)` **返错**（无本地 fallback，必须显式配置）
- **输出上限**：bash 默认 30 KiB/stream，httpreq 默认 256 KiB response，**超限截断不报错**（`RunOutput.Killed` / `Response.Truncated` 标记）

## 强反向不变量

- ❌ **全局 tool registry**：当前显式注册有意为之，多 agent / 多 process 各自管自己的 toolset
- ❌ **Tool 层做业务逻辑**：所有业务在 Executor，Tool 只是 JSON ↔ Go 转换 + schema
- ❌ **`bash` 加 root 限制**：信任调用方，要 jail 在外层（ProcessContext / 容器）
- ❌ **httpreq 默认 allowlist**：必须显式 —— LLM 调用任意 URL 是安全敞口，"忘记配 allowlist 也能跑"是反 pattern
- ❌ **超限抛错而不是截断**：截断 + 标记的设计更友好；LLM 可以根据 truncated 决定下一步

## 特殊点

- **沙箱 / 路径隔离**：
  - `fs.LocalExecutor.Root` —— 相对路径**锚点**（非安全 jail：绝对路径与 `../` 仍可穿透，见 `local.go` 的 `TODO(security)`）；真正隔离靠外层（容器 / ProcessContext）
  - `httpreq.Client.AllowedHosts` —— 强制 allowlist（无默认值）
  - `bash.LocalExecutor` —— **无 root 限制**（信任调用方，lyra 通过 `ProcessContext.Workdir` 在外层管）
- **Glob/Grep 在 SPI 层**：远程 backend 不能每次都往返整个文件系统，所以这两个 bulk 查询直接进 Executor，一次 RPC 而不是多轮 list+read
- **Provider 多租户**：websearch / webfetch 各家 Provider 实现同接口，Tool 不关心是 Tavily 还是 Brave —— `NewTool(provider)` 就行
- **Response shape 一致**：所有 Provider 返回统一 Response（URL + title + snippet + ...），LLM 不用适配各家 API
- **JSON Schema 自动生成**：通过 `pkg/json.StringDefSchemaOf` 从 Input struct 推 schema，写工具不用手动维护 schema 字符串

## 关键目录

```
tools/
├── bash/                单 bash 工具 + Executor SPI（local.go = LocalExecutor）
├── fs/                  5 个文件工具（read/write/edit/glob/grep） + Executor SPI + 本地实现
│                        - Executor.Root 锚定相对路径（非 jail，见 local.go TODO(security)）
├── httpreq/             HTTP 请求 + Client + 主机/方法 allowlist（无默认，必显式）
├── websearch/
│   ├── tavily / brave / exa / ...    各 Provider 实现
│   └── internal/                     共享 utility（query 参数转换）
├── webfetch/
│   ├── jina / firecrawl / ...        各 Provider 实现
│   └── internal/
├── fakeweather/         演示用虚拟天气（确定性输出，无网络依赖）
└── docs/                架构文档（待补）
```

## 常用命令

```bash
go build ./...
go test ./...
go test ./bash/... -v        # 单工具调试
```

## 修改任何东西之前

- **加新工具**：放新子包 `tools/<name>/`，定义 `Input` struct + `Tool` + `NewTool(executor)`；schema 走 `pkg/json` 自动生成
- **加新 Executor 后端**（远程沙箱 / 容器化）：实现 SPI 接口（如 `bash.Executor`），在 caller 处 `NewBashTool(yourExecutor)` 注入
- **加新 websearch / webfetch Provider**：放 `websearch/<provider>/`，实现 `Provider` 接口；不要改 Tool 层
- **改 `chat.ToolDefinition`**：是 `core/model/chat` 的契约 —— 改了所有 tools/* + models/* 工具调用都受影响
