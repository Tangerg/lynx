# CLAUDE.md — lyra module

> **Lyra Runtime** — Go agent runtime backend,实现 Lyra Runtime Protocol(JSON-RPC 2.0,MCP-inspired)给前端用(前端是同仓独立模块 [`../desktop`](../desktop))。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md);架构基准见 [`doc/EXECUTION_CENTERED_ARCHITECTURE.md`](doc/EXECUTION_CENTERED_ARCHITECTURE.md);协议规范(方法表 / 错误码 / header / 状态码等一切 wire 细节)见 [`../desktop/docs/protocol/`](../desktop/docs/protocol/)。本文件只放 lyra 模块特有的宏观内容 —— 目录 / 符号 / 数值以代码与上述规范为准。

---

## 定位

**协议层薄、业务层厚、传输层可换,以 Run 生命周期为中心。** delivery 是 wire 形态契约(JSON-RPC + 自有事件 / Item 模型),application 是驱动 Run/Session/能力生命周期的用例协调层,`adapter/agentexec` 是"跑一个 segment(chat turn + 工具循环)"的 agent-SDK 防腐层,domain 把业务能力按限界上下文切片。Transport 只是 envelope I/O,对业务零感知。架构基准见 [`doc/EXECUTION_CENTERED_ARCHITECTURE.md`](doc/EXECUTION_CENTERED_ARCHITECTURE.md)。

## 架构心智

- **Clean Arch 同心环,依赖一律向内**(domain 是核心;application 只依赖 domain;adapter 实现 application/domain port 并包 infra;delivery 驱动 application/adapter):**目录名 = 环名**,由 `internal/arch` 的测试机器强制 —— 任何向外的依赖边都是回归。
  - **delivery**:协议契约 —— wire 类型、JSON-RPC 方法路由、transport envelope I/O。
  - **adapter**:能力适配器,实现 application/domain 的 port、包装外部能力;`adapter/agentexec` 装 system prompt + 工具集 + model client、驱动 agent SDK 跑一个 segment(防腐层),`adapter/{maintenance,runsegment,toolset,modelclient …}` 各司一域。
  - **application**:用例协调层 —— Run/Session/能力/workspace/schedules 生命周期的编排;`application/runs` 拥有 Run 从 Start 到 Terminal 的完整流程(admission / journal / pump / cancel),engine 与 wire 中立,consumer-side port 定义在此。
  - **domain**:限界上下文,一域一包(会话 / 知识 / transcript / 审批 / 工具 / provider / execution …),零外向依赖,纯 entities + 领域服务 + port。
  - **infra**:技术设施(driven adapter),零领域、只实现 domain port(存储 / git / LSP / 影子 git / 进程执行 / MCP / A2A)。
  - 组合根 `internal/{runtime,bootstrap,config}` + cmd:config·env → 装配 + host 生命周期 + SPI nil-default 注入;wires 每一环,arch_test 禁任何环 import 它。
- **transport 可换、对业务零感知**:HTTP + SSE 与 inprocess 都只是 envelope I/O,不把传输细节带进 application/domain。
- **流式走 streamable HTTP,不是常开通道**:每个流式调用的事件走它**自己那条 POST 响应流**,事件源是 server 侧的 per-run hub,无连接身份簿记;重连是 per-run(带 last-event-id),不是重连一条共享流。
- **持久化 dev 阶段单一 SQLite 后端**(纯 Go 无 CGO):session / snapshot / interrupt / history / provider / message 都在一个 DB;没有存储开关、没有 in-memory 并行实现。**唯一文件例外**是用户可编辑的 LYRA.md memory —— "可编辑" 正是它存在的意义。
- **per-run model 显式配对**:一次 run 指定 provider + model(缺一即报错、都缺用默认),provider **不从 model 推断**;凭证取自运行态可变的 provider 注册表,经 agent 的 client-provider 扩展点让该 turn 用它。

## Lyra-specific 强反向不变量

跨模块共用反向不变量见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的(数值 / header / 端点等细节以协议规范为准):

- ❌ **Stdio transport**(给 LLM 用那种):协议层有意不实现。Web / 桌面走 HTTP loopback；inprocess 保留给未来独立 CLI/TUI。
- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**:Runtime 协议层零 user 概念,鉴权由更外层(OS 信任 / 本地门禁 token / 未来 facade)解决。
- ❌ **业务方法的 RESTy read-only shadow**:业务调用一律走单一 RPC 形态,不加按资源的 GET 影子端点;sidecar 只留 info / health 两个 no-auth 端点。
- ❌ **HTTP transport 换 gin / echo / fiber**:它们的自家 ctx / ResponseWriter 把 SSE 的 buffer / flush 搞砸过。**"换 router" ≠ "换掉现用的 chi"** —— chi 就是标准 `net/http` handler,flush 与 stdlib 一致,是例外、已采用。
- ❌ **SSE 自写 frame 编码**:用合规的 SSE 库(auto-flush)—— 手写 `data:` 拼接在 body 含换行时会破坏帧。
- ❌ **协议 envelope 装 transport 元数据**(session id / auth token / trace id / 幂等键):走 `context.Context` 或 HTTP header,永不进 message body;trace 关联用 W3C 标准 propagator,不自造 header。
- ❌ **业务 error 映射 HTTP status**:业务错误走 JSON-RPC `error.code` + `error.data` 的 symbolic name 分支;HTTP status 只反映 transport 层。
- ❌ **退回常开的 server→client 通知通道**(独立 GET 流 + 连接路由 + 广播 fan-out):已被 streamable HTTP 取代。真要 server 主动推送,按 TRANSPORT 规范的退路**增量**加一条可选流,别把旧模型整套搬回来。

## 改动前必看(波及面)

- **动 delivery 的协议契约**:前后端都要同步 —— 先在协议规范里对一遍,再动代码。
- **动 transport**:跑该 transport 的整套测试(HTTP + SSE 的 auth / cors / sidecar)。
- **动 application 编排 / agentexec turn 循环**:跑 application(runs/sessions)与 adapter(agentexec/maintenance)的测试。
- **动某个 domain 的 service 接口**:跑该包测试;改接口形状先搜下游 consumer。
- **动 infra 持久化**:跑存储测试(sqlite + file knowledge)。
- **改 transport**:保持 transport 只做 envelope I/O;业务入口仍走 `delivery/server` 的协议方法。新增同进程客户端优先复用 inprocess。
