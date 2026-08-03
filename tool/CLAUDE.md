# CLAUDE.md — tool module

> Lynx 的 provider-neutral 可执行工具窄腰：只拥有最小执行契约、decorator 能力发现和实例 Registry。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。

## 定位

- **契约与通用适配，不是实现集合**：`Tool`、`WrappingTool`、`Capability`、`Registry` 和显式定义的 `NewFunc` 属于本模块；schema 自动推导与 shell、文件系统、HTTP、搜索、skill 等具体工具属于上层 `tools` 模块。
- **生产依赖只到 Core**：除 `core/chat.ToolDefinition` 与标准库外，不依赖 Agent、MCP、A2A、具体工具或应用。
- **零全局状态**：每个 runtime/process 显式持有自己的 `Registry`；零值可用，注册批次保持原子性。

## 架构心智

- `Tool` 只表达 `Definition` 与 `Call`，不吸收 retry、并发、HITL、checkpoint 或 provider 协议。
- `NewFunc` 只拥有 typed function 的严格 JSON 编解码；调用方必须传入完整 `chat.ToolDefinition`，本模块不依赖反射 schema 库。
- 可选能力由消费方定义，并通过 `Capability` 沿 `WrappingTool` 链查找；外层实现优先。
- Registry 只做验证、快照、稳定排序和按名解析，不负责发现、生命周期或配置。

## 模块特有反向不变量

- ❌ import `tools`、`agent`、`mcp`、`a2a`、provider SDK 或 app。
- ❌ 增加具体工具工厂、schema 反射生成、全局注册或自动发现。
- ❌ 把 runtime policy 塞进 `Tool` 接口；新增接口方法会破坏全部外部实现。
