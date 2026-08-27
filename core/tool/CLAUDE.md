# CLAUDE.md — core/tool package

> Scope 的 provider-neutral 可执行工具窄腰：拥有最小执行契约、decorator 能力发现和 typed function 适配能力。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。

## 定位

- **契约、通用适配与实例集合**：`Tool`、`WrappingTool`、`Capability`、`Func`、`NewFunc` 与 `Registry` 属于本包；schema 的派生、解析与验证只由 `core/jsonschema` 拥有；shell、文件系统、HTTP、搜索、skill 等具体执行能力属于 `tools` module family。
- **生产依赖只到 Core**：除 `core/chat.ToolDefinition`、`core/jsonschema` 与标准库外，不依赖 Agent、MCP、A2A、具体工具或应用。
- **零全局状态**：每个 runtime 显式构造并持有自己的 `tool.Registry`。

## 架构心智

- `Tool` 只表达 `Definition` 与 `Call`，不吸收 retry、并发、HITL、checkpoint 或 provider 协议。
- `NewFunc` 从输入类型派生严格 schema，并拥有 typed function 的严格 JSON 编解码；schema 的反射、编译与 wire 规则由 `core/jsonschema` 单点拥有。
- 可选能力由消费方定义，并通过 `Capability` 沿 `WrappingTool` 链查找；外层实现优先。

## 模块特有反向不变量

- ❌ import `tools`、`agent`、`mcp`、`a2a`、provider SDK 或 app。
- ❌ 增加具体工具工厂、重新暴露 schema 派生转发 API、绕过 `core/jsonschema` 直连第三方 schema 库、全局注册或自动发现。
- ❌ 把 runtime policy 塞进 `Tool` 接口；新增接口方法会破坏全部外部实现。
