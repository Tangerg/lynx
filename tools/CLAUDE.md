# CLAUDE.md — tools module

> 给 LLM 调用的工具协议、实例 Registry、typed function adapter 和具体工具集(shell / 文件系统 / HTTP / 网页抓取 / 网页搜索 / skill 等)。所有实现统一满足根包 `tools.Tool`；模型可见定义只使用 `core/chat` 协议值。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。工具名录 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **实现 `tools.Tool` 的工具集合**,两层 SPI:**Tool 层**对 LLM(JSON in/out + schema + 交互),**Executor / Provider 层**做真正执行(本地 / 远程 / 沙箱后端可换)。

## 架构心智

- **两层 SPI 是核心**:Tool 层只做 JSON ↔ Go + schema 校验 + LLM 交互;**所有业务逻辑**(行号、binary 检测、写锁、路径锚定 …)都在 Executor 层 —— 这样远程 backend 能独立优化,不必往返整个文件。
- **手动注册,无全局 registry**:调用方显式把工具注册进自己的 toolset,多 agent / 多进程各管各的。
- **只有一套 Registry**:根包 `tools.Registry` 是唯一实例 registry，同时满足 `agent/toolloop.ToolResolver`；工具定义直接投影成 `core/chat.ToolDefinition`，不建立第二套 registry 或 bridge。
- **schema 从 Input struct 自动推导**:根包 `tools.New` 只接受 struct/`*struct` 输入，schema 与 strict JSON decoder 同源；手写具体 Tool 时才显式提供 schema。
- **typed helper 不承载 runtime policy**:`tools.New` 只做 typed function ↔ JSON Tool 适配；并发、重试、HITL、直接返回和 tool-loop 终止属于 agent/runtime decorator。
- **Nil-safety 双标**:有本地实现的(shell / fs 等)`New(nil)` 默认本地、开箱即用;必须外部配置的(websearch / webfetch / httpreq)`New(nil)` **返错** —— 没有本地 fallback。
- **输出超限截断而非报错**:带 truncated 标记,LLM 据此决定下一步。
- **bulk 查询下沉 Executor**:glob / grep 这类进 SPI 层,远程 backend 一次 RPC 完成,而非多轮 list + read。
- **Provider 统一 Response 形状**:各家 websearch / webfetch 返回一致结构,LLM 不用适配每家 API。

## 模块特有反向不变量

- ❌ **全局 tool registry** —— 显式注册是有意的,多 agent / 多进程各自管理 toolset。
- ❌ **在 Tool 层做业务逻辑** —— 业务全在 Executor,Tool 只是 JSON ↔ Go + schema。
- ❌ **给 shell 加 root 限制** —— 信任调用方,要 jail 在外层(进程上下文 / 容器)。
- ❌ **httpreq 带默认 allowlist** —— 必须显式配置;"忘配也能跑" 是 SSRF 敞口。
- ❌ **超限抛错而非截断** —— 截断 + 标记对 LLM 更友好。

## 改动前必看(波及面)

- **动 `chat.ToolDefinition`**:这是当前 `core/chat` 协议值；所有 Tool、Registry 和 provider request mapping 都受影响。
- **加新工具**:新起子包,定义 Input struct + Tool + 工厂;schema 自动生成。
- **加新 Executor 后端**(远程沙箱 / 容器):实现对应 SPI 接口,在调用处注入。
- **加新 Provider**(websearch / webfetch):实现 Provider 接口,不改 Tool 层。
