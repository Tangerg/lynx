# app2 参考研究与现状审计

> 基线日期：2026-08-22。本文记录证据、采用与拒绝；不是旧实现的兼容规格。

## 1. 研究范围

| 来源 | 本地证据 | 用途 |
| --- | --- | --- |
| Lynx 当前 Runtime | `app/runtime/` 及 `app/runtime/doc/` | 能力、不变量、恢复与合同基线 |
| Lynx 当前 Desktop | `app/desktop/`、frontend 架构/设计/渲染文档 | 产品表面、状态 owner、视觉与回归门 |
| Codex Desktop | `~/Desktop/study/codex/deob`，v26.810.50856（README 记录 2026-08-15） | 富客户端进程、请求调度、连接 owner、UI 机制 |
| Codex Server | `~/Desktop/study/codex-server/codex-rs` | app-server 协议、执行/状态/rollout/recovery 机制 |
| Dougong | `~/Desktop/dougong/AGENTS.md`、`CLAUDE.md` | lifecycle、canonical API、依赖与事务标准 |
| Oolong | `~/Desktop/oolong/AGENTS.md` | KISS、breaking、纵切、Config/API 标准 |

Oolong 在桌面目录下没有 `CLAUDE.md`（已搜索四层目录）。app2 记录缺席，不伪造或把其他文件冒充来源。
Dougong 的 `CLAUDE.md` 只是 `@./AGENTS.md` 指针，因此 app2 同样以 `AGENTS.md` 为唯一规范正文。

## 2. 当前系统的必须保留基线

当前已验收事实：

- Protocol `2026-08-21`、Artifact v22、SQLite epoch 77、Agent Framework baseline 20；
- 89/89 Runtime operations、3/3 operational sidecars、16/16 runtime events 均有产品消费者；
- Frontend 最近完整门禁为 315 个 test files / 1969 tests，97 条 published context edge 无环；
- Runtime/Desktop test、vet、build、staticcheck、contract generation diff、standalone 和 Wails package 已有全绿基线；
- renderer reload、Runtime restart/SIGKILL、fresh database、HITL、Goal/Plan、tool material 与 context footprint
  已有真实产品 smoke 证据。

这些数字来自 `app/runtime/doc/EXECUTION_PLAN.md` 与 `CAPABILITY_LEDGER.md`，用于防止漏迁；它们不是
app2 必须复制的测试结构或文件数量目标。

关键语义资产：

- Workspace > Session > Run > Item，Run 可跨多个 Segment；
- Session 至多一个 open root Run tree，delegated Run 保持 source ownership；
- Conversation、Transcript、WorkingContext 分离；
- commentary 与 final answer 分相，只有 final answer 有 message actions；
- exact HITL identity、Question Skip 顺序、approval scope、checkpoint invalidation；
- durable command marker、unknown effect、SIGKILL recovery、多 Runtime winner；
- Goal incarnation、Plan 一等资源、contextTokens 持久恢复；
- Work Index / Agent Narrative / Context Dock；
- Composer draft/attachment/IME、流式 follow lock、Markdown/code/table/image/Mermaid/KaTeX；
- files/diff/git/search/index、Terminal/tool detail/timeline；
- provider/model/MCP/hook/skill/recipe/knowledge/memory/schedule/usage/feedback/settings/theme/i18n。

完整映射只在 `CAPABILITY_LEDGER.md` 维护。

## 3. 当前结构的证据化坏味道

### 3.1 架构由长期补丁历史主导

- Runtime 执行计划已到 P142，决策文档包含 ADR-RT-001 至 ADR-RT-063；
- `app/runtime/internal` 顶层按通用层分为 24 个 application、23 个 adapter、21 个 domain、17 个 infra
  子包，外加 delivery/bootstrap/arch；
- Runtime Go/contract/sql 合计约 223k 行，最大测试 3k 行，核心 reducer/use case 单文件接近或超过 1k 行；
- Desktop frontend TS/TSX/CSS 合计约 124k 行，HTTP E2E 单文件约 4.5k 行，手写 RPC methods 约 891 行，
  `globals.css` 约 2.1k 行。

体量本身不是缺陷；问题是定位一个能力通常要同时穿过通用层、operation wrapper、generated contract、
HTTP/SSE、plugin SDK、context adapter 和 store。app2 以领域纵切缩短这条路径。

### 3.2 协议实现成熟，但人类文档曾发生漂移

`CONTRACT_BASELINE.md` 明确 Interrupt/response 只有 approval 与 question，且生产已删除 toolResult interrupt；
`API.md` 第 462 行仍写“Interrupt 三变体 approval/question/toolResult”，第 650 行又写“两种 response”。

这不是协议设计本身的缺陷。相反，当前 `protocol` types、typed operation registry、generator、strict
JSON-RPC、HTTP/embedded 共用 endpoint，已经形成很强的机器真相源。需要清除的是手写文档对 generated shape
的重复描述，而不是替换 dotted methods、`runtime.discover` 或 wire contract。

### 3.3 本地 transport 是有价值的统一 binding，host ownership 需要重写

当前 Desktop `Bootstrap()` 读取 `~/.lyra/local-token` 并返回固定 loopback endpoint。Renderer 使用 strict HTTP
JSON-RPC、SSE、info/live/ready sidecar、CORS 和 token；HTTP 与 public embedded binding 共用同一个 typed
operation endpoint。

这些机制统一了本地、standalone、remote 和 embedded 语义，并提供成熟的回放、断连 detach、写 deadline 与
operational probes，不应因为 Codex 使用 stdio 而被删除。app2 要重写的是固定地址、bootstrap/supervision、
runtime generation ownership 和恢复边界：Wails host 选择并校验 Runtime，renderer 只消费只读 bootstrap contract。
若未来添加其他 binding，必须有真实部署反例并继续复用 operation pipeline。

### 3.4 通用插件内核没有生产动态变化轴

当前前端以 Dougong Host、PluginProvider、Plugin SDK、requires/provides graph、约 35 个 extension points、
Slot 和 selector 组织全部 UI；但 production 只安装同 bundle 静态 built-ins。外部 sideload、动态安装/移除和
无生产者 extension seam 已在 P140/P142 删除。

结论：限界上下文与模块化应保留；通用 installation/plugin runtime 不应保留。真正多生产者的 tool renderer、
commands、workspace destinations 使用更小的具名 registry。

### 3.5 已有实现中值得保留的质量资产

重写不等于否认现有成果。当前系统已经证明：

- exact identity/generation/disposer 可以阻止迟到提交；
- context public surface 和 architecture gates 能阻止横向泄漏；
- committed-only projection、CAS、marker 和 strict reader 能支持恢复；
- 视觉、IME、a11y、窄窗、WebKit/Retina 与真实 package smoke 必须成为完成门；
- 没有 producer/consumer 的 speculative surface 可以证据化删除。

app2 把这些原则放进更小的结构，而不是复制现有实现。

### 3.6 熵回收裁决

| 置信度/风险 | 候选与证据 | app2 cut | 放弃的可观察能力 | 决定性验证 |
| --- | --- | --- | --- | --- |
| 高/中 | 通用 frontend plugin host；production 只静态安装同 bundle built-ins，动态 sideload 已无产品入口 | 删除 installation lifecycle、service graph、generic extension points；只保留具名 registry | 无；不删除现有 built-in surface | 24 UI groups + production bundle consumer scan |
| 高/中 | 手写 Runtime TS methods、人类字段表和 operation registry 同时描述 wire | Go typed registry 单向生成 client/validator/schema/OpenRPC/manifest/examples | 无；89 methods 与 errors/events 保持精确 | generator diff + 89/89 manifest/consumer coverage |
| 高/高 | 通用层深树让一个能力跨越大量 forwarding packages | 新纵切按领域 package 落地，旧实现只作隔离 oracle，切换后整树删除 | 不保留旧 Go public/internal API；用户语义不变 | old/app2 normalized scenario + import/dead-code scan |
| 拒绝删除/高 | 当前 Lyra Protocol、strict validators、token/CORS、SSE replay 和 sidecars 均有真实消费者且保护 trust/recovery boundary | 主动继承，不用 stdio/bridge 包装 | 不适用 | HTTP/embedded conformance + reload/SIGKILL/remote smoke |
| 拒绝新增/中 | Codex scheduler/initialize/mount 在 Lyra 没有现存协议义务或测量反例 | 不建立 speculative scheduler、handshake 或第二订阅模型 | 无 | architecture search + current manifest exact comparison |

这份裁决只证明删除/拒绝的概念，不授权机械删除旧树；旧生产代码必须等对应能力 `verified` 并完成切换后才删除。

## 4. Codex Server 研究

### 4.1 已确认的机制证据

从 `codex-rs/app-server/README.md` 与源码确认：

- rich client 通过 bidirectional JSON-RPC 驱动；默认 stdio JSONL，也支持 WebSocket/Unix socket；
- 每条连接先 `initialize`，再 `initialized`，pre-handshake request 被拒绝；
- Thread/Turn/Item 是协议顶级 primitive，进度以 item/turn notifications 推送；
- schema/TypeScript 可从 server 版本生成；
- ingress、processor、outbound 使用 bounded queue，过载返回 `-32001`；
- `request_serialization.rs` 按 Thread、path、process、watch、MCP OAuth 等具体 key FIFO，shared read 可并发；
- `connection_rpc_gate.rs` 在 shutdown 时停止新 handler、等待已开始 handler；
- thread listener 用 generation 和 exact `Arc` identity 判断 successor，旧 listener 被结构化取消；
- recorder/background writer 持有 join handle 和 terminal failure，支持 flush/shutdown；
- model-context reverse scan 用明确 cutoff 规则重建有界上下文，不把所有 rollout 当同一语义。

Codex Desktop 的 deobfuscated main process 还显示：

- renderer 不直接拥有 app-server process；main process 负责 handshake、request/notification 中介与连接状态；
- request scheduler 有 critical/interactive/background 优先级、容量、过期、query coalescing 和跨 conversation 公平性；
- stale connection message/close/error 根据 exact connection identity 被忽略；
- reconnect 使用上限退避和 jitter，版本/auth/用户退出等状态会停止自动重试；
- stop 会清 pending request、listener、watch、timer、auth cache 和 port-forward 等资源。

这些事实用于提出和验证 Lyra 的生命周期机制；它们不自动成为 app2 的协议决策。

### 4.2 调整后采用

| Codex 做法 | app2 取舍 |
| --- | --- |
| Thread/Turn/Item | 保留 primitive 思想；映射为 Session/Run/Item，并显式保留 Segment/Run tree |
| stdio default | 不采用；保留 Lyra streamable HTTP/SSE 与 sidecars |
| WebSocket experimental | 不采用为目标；remote 继续复用 HTTP(S)，新 binding 需真实需求 |
| initialize/initialized | 不采用；保留 `RequestMeta + runtime.discover` |
| request priorities/coalescing | 只在测量证明需要后引入；完全相同的只读请求才可 coalesce |
| per-resource serialization | 采用，key 使用 app2 领域 identity，DB CAS 仍是最终仲裁 |
| rollout JSONL | 只学习 append/replay/cutoff 语义；不建立 SQLite 之外第二真相源 |
| large request processor switch | app2 用生成 catalog + context handlers，避免一个超级 dispatcher 文件 |
| unbounded per-thread command channel | 不照搬；保留 Lyra 已有 bounds、deadline、replay/resync 语义 |

### 4.3 明确拒绝

- 不把 Rust/Electron 当架构要求；app2 继续 Go/Wails/React；
- 不复制 deprecated/experimental API、ChatGPT/App/Marketplace/enterprise compatibility；
- 不因 Codex 有某能力就扩张 Lyra 产品范围；
- 不把 main-process 反编译代码当可复制源码，只用作机制证据；
- 不把 slash methods、connection handshake、stdio/WebSocket 或 Codex error code 移植成 Lyra 协议；
- 不把 app-server protocol 直接暴露给 React component。

## 5. Codex Desktop/UI 研究

当前 Lyra 的 P113–P142 已重点对照 `~/Desktop/study/codex` 并建立三面信息架构。app2 继续采用：

- 左侧低心率 Work Index，不做功能菜单；
- 中央以 narrative 呈现 work/final hierarchy、delegated disclosure 和 HITL；
- 右侧 Context Dock 按当前 Session/cwd 承载 files/diff/tool/timeline；
- composer 是主要输入设备，Goal/Plan/HITL 以紧凑 rung/tray 进入同一输入栈；
- tool call 是透明 activity row，复合 delegated boundary 才拥有 card hierarchy；
- 精确的流式 follow ownership，用户离开尾部后不抢滚动；
- native desktop surface、克制边缘/阴影、稳定几何、键盘和 IME。

不照搬：

- ChatGPT cloud project、library、voice、dictation、marketplace 等非 Lyra 能力；
- Electron-only preload/window/security API；
- 反编译 bundle 的组件拆分和命名；
- 仅为当前 Codex 多产品矩阵存在的调度/兼容复杂度。

## 6. Dougong 标准吸收

从 `~/Desktop/dougong/AGENTS.md` 采用：

- 生命周期词汇精确且一致；
- 同一能力同一层只有一个 canonical API；
- 依赖、所有权、取消和清理显式；
- 正交状态原子，不用模糊字段组合；
- 资源 lifetime 结构化；
- 事务只暴露 committed state；
- 单向依赖，纵向切片；
- docs/types/runtime/guards 同批更新；
- 本地 consumer 搜索不足以证明 public API 可删。

不直接复制 Dougong plugin Host 到 app2 frontend。其 lifecycle 原则成立，但当前 app2 没有 production dynamic
plugin 产品需求。

## 7. Oolong 标准吸收

从 `~/Desktop/oolong/AGENTS.md` 采用：

- 不兼容旧写法；
- 选择最简单的完整实现；
- 按纵切演进，避免 big-bang 半成品；
- 模块边界清晰，优先维护良好的依赖；
- 为长期正确性决策，不为短期迁移方便制造债；
- Config struct 显式、零值有用；不使用 functional options 隐藏依赖；
- public API 删除必须有完整 consumer/发布证据。

## 8. 参考优先级

发生冲突时：

1. 用户本次 greenfield/breaking/全能力要求；
2. 当前 Lyra Runtime Protocol 与 app2 领域不变量；
3. app2 已接受 ADR；
4. 当前产品已证明的用户行为与恢复事实；
5. Codex 的机制证据；
6. Dougong/Oolong 工程原则；
7. 旧目录和具体实现。

“Codex 这样做”不是充分理由。每次采用必须说明它解决的 app2 真实问题，并用本地测试证明。

## 9. R0 结论

- 重写是合理的，但不能丢弃现有语义和测试资产；
- 当前 Lyra Runtime Protocol 是合同基线，Codex 仅作机制参考；
- 默认拓扑确定为 Wails-supervised independent Runtime over streamable HTTP JSON-RPC/SSE；
- renderer 使用 generated Lyra client 直连 Runtime，Wails 不创建第二业务 bridge；
- 领域语言继续 Session/Run/Segment/Item；
- Runtime 使用领域 package + consumer-owned ports；
- Frontend 使用静态 bounded contexts，不恢复通用 plugin runtime；
- SQLite 是唯一 durable truth；
- 合同单向生成；
- 能力迁移以场景账本和真实恢复/视觉证据完成，而非以文件搬完或编译通过完成。
