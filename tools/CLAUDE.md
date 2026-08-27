# CLAUDE.md — tools module family

> `tools` module 提供可直接装配的 shell、文件系统、HTTP、网页抓取、网页搜索和 skill 工具。单工具协议、typed function、schema 与实例 Registry 属于更低层的 `core/tool`。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。工具名录 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **模块拥有具体工具，Core 拥有工具模型**：`core/tool.Registry` 管理实例集合，`tools/*` 只实现可执行能力，不再维护第二套集合抽象。
- **具体能力各自拥有实现**：`fs`、`shell` 等保持独立子包。两层 SPI：**Tool 层**对 LLM(JSON in/out + schema + 交互)，**Executor / Provider 层**做真正执行(本地 / 远程 / 沙箱后端可换)。演示工具属于 `examples`。
- **外部依赖按方向与生命周期成岛**：`web` 的中立 `Searcher` / `Fetcher` SPI、各 provider、`httpreq` 与 `skills` adapter 共享同一个 `tools` module；它们都是可装配工具，依赖方向与发布周期一致。package 负责隔离能力，module 不重复切发布单元；成熟三方库不因“预算”被拒绝。

## 架构心智

- **两层 SPI 是核心**:Tool 层只做 JSON ↔ Go + schema 校验 + LLM 交互;**所有业务逻辑**(行号、binary 检测、写锁、路径锚定 …)都在 Executor 层 —— 这样远程 backend 能独立优化,不必往返整个文件。
- **手动注册,无全局 registry**:调用方显式把工具注册进自己的 toolset,多 agent / 多进程各管各的。
- **只有一套可执行 Tool 身份**：`core/tool.Registry` 只管理普通 `tool.Tool`；Agent Framework `interaction.Dispatcher` 冻结同一批 Tool，不建立第二套 Tool 类型、可变 registry 或 bridge。
- **schema 归单工具层**：`core/tool.NewFunc` 从 Input struct 派生 schema；手写具体 Tool 使用 `tool.Schema`，不在本 family 复制 schema 内核。
- **typed helper 不承载 runtime policy**：`tool.NewFunc` 不处理并发、重试、HITL、直接返回或 Tool loop 终止；这些属于 `agent/interaction` 与 Host adapter。
- **Nil-safety 双标**:有本地实现的(shell / fs 等)`New(nil)` 默认本地、开箱即用;必须外部配置的(web / httpreq)`New(nil)` **返错** —— 没有本地 fallback。
- **输出超限截断而非报错**:带 truncated 标记,LLM 据此决定下一步。
- **bulk 查询下沉 Executor**:glob / grep 这类进 SPI 层,远程 backend 一次 RPC 完成,而非多轮 list + read。
- **Provider 统一响应形状**:各家 web provider 返回统一的 `SearchResponse` / `FetchResponse`,LLM 不用适配每家 API。

## 模块特有反向不变量

- ❌ **全局 tool registry** —— 显式注册是有意的,多 agent / 多进程各自管理 toolset。
- ❌ **在 `tools` 复制 Tool/Registry/schema 原语** —— 这些只由 `core/tool` 拥有。
- ❌ **为同依赖方向与生命周期的 web provider 拆 module** —— `web/<provider>` 是实现 package，不是独立发布单元；只有引入明显不同的重型 SDK 或生命周期时才重新评估边界。
- ❌ **在 Tool 层做业务逻辑** —— 业务全在 Executor,Tool 只是 JSON ↔ Go + schema。
- ❌ **给 shell 加 root 限制** —— 信任调用方,要 jail 在外层(进程上下文 / 容器)。
- ❌ **httpreq 带默认 allowlist** —— 必须显式配置;"忘配也能跑" 是 SSRF 敞口。
- ❌ **超限抛错而非截断** —— 截断 + 标记对 LLM 更友好。

## 改动前必看(波及面)

- **动 `chat.ToolDefinition`**:这是当前 `core/chat` 协议值；所有 Tool、Registry 和 provider request mapping 都受影响。
- **加新工具**:新起子包,定义 Input struct + Tool + 工厂;schema 自动生成。
- **加新 Executor 后端**(远程沙箱 / 容器):实现对应 SPI 接口,在调用处注入。
- **加新 Web Provider**:在 `web/<provider>` 实现 `Searcher`、`Fetcher` 或两者,不改 Tool 层；同一 provider 只拥有一个 package，并共享 `tools` module。
