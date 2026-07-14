# CLAUDE.md — mcp module

> lynx 对 Model Context Protocol 的**薄适配**:client / server / session / transport 直接用官方 MCP go-sdk,本模块不做第二套 SDK。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体符号 / SDK 版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **根包放 lynx 自己需要的 MCP 周边**:context metadata、server-to-client 反向能力、`tools.Tool` 与 MCP tool 的双向适配、sampling / prompt 转换。
- **应用层的事不在这里**:MCP 服务器配置、OAuth 登录、热重连、状态展示属于 app/runtime 的 infra 层 —— 本模块只做协议适配。

## 架构心智

- **薄适配,不包官方 SDK**:transport / session 直接用官方类型,不再套一层。
- **单包优先,少暴露**:同一 MCP 适配域先放根包;远端工具只通过 `Tools` 暴露为 `[]tools.Tool`,不公开具体 wrapper / config。
- **无 Provider / cache 层**:工具列表刷新策略由应用层决定(收到 list-changed 后重新拉)。
- **协议错误与 tool 错误分开**:远端工具报错投影成工具级错误、传输 / 协议问题保持 wrapped Go error;server 侧 `tools.Tool` 的错误转成"结果里标错",不升格成 JSON-RPC error。

## 模块特有反向不变量

- ❌ **加配置注册中心**(服务器清单 / OAuth handler / headers / reconnect)—— 都在 app/runtime 的 infra。
- ❌ **恢复 Provider / cache** —— 除非有多个真实调用方证明应用层刷新不够。
- ❌ **把 MCP primitive 包成框架** —— 优先直接暴露 SDK 类型或写一个小函数。

## 改动前必看(波及面)

- **先看官方 go-sdk 的接口形状**:本模块是 thin adapter,不维护自己的协议状态。
- **加新 MCP primitive**:优先直接暴露 SDK 类型或小函数,不要包成框架。
