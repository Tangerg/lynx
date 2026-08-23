# app2 工程标准

本文是 `app2/` 下的规范性标准。仓库根规则继续生效；发生冲突时，更具体且不降低质量的
app2 规则优先。本文吸收 `~/Desktop/dougong/AGENTS.md`、
`~/Desktop/dougong/CLAUDE.md` 与 `~/Desktop/oolong/AGENTS.md` 的有效约束，并按 app2
的 Runtime/Desktop 领域发展为自己的标准。Oolong 没有可搬运的 `CLAUDE.md`，不得伪造来源。

## 1. 产品与迁移边界

- `app2` 是唯一目标实现；旧 `app` 只是只读行为证据和验收 oracle。
- 当前授权范围只重写 `app2/runtime` 与 `app2/desktop`。不得修改或删除旧 `app/runtime`、
  `app/desktop`、`app/cli`；CLI、产品入口切换和旧树删除属于明确延期的后续目标。
- 不提供旧 API alias、deprecated wrapper、双读、双写、旧 wire reader、旧数据库迁移、
  旧 Artifact reader、目录 fallback 或长期 shadow implementation。
- breaking change 一次完成。不能用“以后删除”批准已知债；协议 breaking 还必须满足 ADR-A2-022 的反例、
  版本和消费者闭环门槛。
- 保留的是用户能力、已证明语义，以及主动选定的当前 Lyra Runtime Protocol；旧类名、package、DTO、
  handler/repository API、数据库、Artifact、frontend surface 和 UI 组件树都不是设计约束。
- 每个切片必须端到端完成；不得提交只有 schema、空 interface、TODO adapter、假数据 UI 或
  永远关闭的 feature flag 的半成品。

## 2. 判断顺序

做设计或改代码时按以下顺序判断：

1. 真实产品不变量是什么？
2. 哪个领域对象或应用用例唯一拥有它？
3. 最小完整纵切是什么？
4. 哪些机制可以删除，而不是转发或重新包装？
5. 如何用失败先行的测试证明新行为？
6. 如何在结构上阻止旧问题复发？

不要从目录、框架、数据库表或 wire DTO 反推领域模型。

## 3. 统一语言

代码、测试、日志、协议和文档必须使用同一组精确词汇：

- `Workspace`：由绝对路径标识的执行上下文值，不是可独立维护的 Project 聚合；
- `Session`：用户持续工作的会话；
- `Run`：一次可观察、可取消、可等待、可恢复的 agent 工作；
- `Segment`：同一 Run 在 HITL 或恢复边界之间的一段执行；
- `Item`：进入持久历史并可回放的用户或 agent 事实；
- `Conversation`：发给模型的历史；
- `Transcript`：向用户展示和审计的语义历史；
- `WorkingContext`：某次模型调用实际读取的有界上下文；
- `Interrupt`：阻止 Run 前进、需要精确回答的 approval 或 question；
- `Goal`：Session 拥有的持续目标；
- `Plan`：当前 root Run 的权威步骤事实；
- `ToolCall`：归属于 exact Run/Item 的外部作用生命周期。

相近词不能互换。特别禁止用 `activeRun` 同时表示 current root、任意 child 和整个 Session
审计材料，也禁止把 Conversation、Transcript 与 WorkingContext 合成一个“messages”。

## 4. 唯一作者与正交原子

- 同一能力在同一层恰好一个 canonical API、一个状态 owner、一个生命周期 owner。
- 一个状态字段只表达一个事实；不要用可空字段组合模拟闭合状态机。
- 使用 sum type 表达互斥状态，使用 revision/CAS 表达条件写，使用精确 identity 表达归属。
- 事件表达已经发生的事实；命令表达有目标的意图；查询返回稳定读模型；不要互相冒充。
- 缓存、索引、tree、summary 和 UI projection 都是派生物，不能成为第二真相源。
- 本仓库暂时没有消费者，不足以证明一个 public surface 可以删除；必须检查生成、动态入口、
  外部模块、文档承诺和黑盒消费。反过来，没有真实生产者/消费者也不足以保留 speculative API。

## 5. DDD 与整洁架构

- 先划分限界上下文，再决定 package；目录是边界的结果，不是目标。
- Domain 只包含业务身份、值、状态迁移和不变量，不依赖 React、Wails、Agent Framework、
  JSON-RPC、SQLite、OTel SDK 或操作系统。
- Application 用例拥有授权、事务 write-set、跨聚合协调、幂等命令和提交后事件。
- 外部 adapter 实现由消费用例定义的窄 port；interface 放在消费者一侧。
- Composition root 是唯一知道 concrete dependencies 的地方；不引入 service locator、DI framework、
  generic repository、generic event bus、base service 或万能 `Manager`。
- 不为了“看起来像 Clean Architecture”给每个 context 建五个空目录。只有存在真实变化轴时才拆
  `domain/application/adapter/presentation`。
- 跨上下文只依赖对方的 published language；禁止 import 对方内部 store、row、wire DTO 或组件。

## 6. Go 标准

- 使用模块声明的当前 Go 版本和现代标准库能力；所有 app2 Go module 保持同一版本。
- package 按领域或具体机制命名，例如 `run`、`runflow`、`sqlite`、`agentexec`；禁止
  `common`、`utils`、`helpers`、`base`、`service`、`impl`、`component` 垃圾分类。
- 优先 concrete type。只有真实替换、测试 seam 或所有权反转需要 interface。
- 配置使用显式 `Config` struct，并为每个零值写清含义；不使用 functional options 隐藏 required dependency。
- constructor 完成合法性证明；不要创建需要后续 `Init` 才合法的对象。
- error 提供动作和对象上下文并 `%w` 包装原因；领域拒绝使用可判定的稳定类型/code。
- `context.Context` 只作为调用链第一个参数，不存进 struct；取消必须传播到外部 I/O。
- goroutine 必须有 owner、取消路径和 join；channel 默认有界。不得 fire-and-forget。
- 锁保护数据，不保护漫长 I/O；需要串行化时按业务 identity 建队列，不建全局大锁。
- 资源获取与释放在结构上配对；`Close` 幂等、可观察，并等待 owner 的子任务退出。
- `panic` 只用于不可恢复的 programmer invariant；进程/IPC 边界必须转换为失败并保留诊断。

## 7. Runtime 与 Agent 执行

- Agent Framework 只经 `agentexec` 适配边界进入 app2；其他 package 不依赖 framework concrete types。
- Framework Engine 拥有模型/工具执行生命周期；app2 Run 用例拥有产品状态、事务、HITL、恢复和审计。
- framework snapshot 对 app2 内环保持 opaque；只有 adapter 可编码、解码和恢复。
- fresh start、resume、steer、cancel、child admission、waiting barrier 和 terminalization 都必须有精确命令身份。
- 外部副作用与权威投影采用 prepare/commit/apply 或 dispatch-attempt/receipt 语义；回执丢失不能靠
  “再试一次大概没事”处理。
- 事务只发布 committed facts。unknown effect 必须 fail closed，直到有确切 settlement。
- Session 同时最多一个 open root Run tree；child Run 是该 tree 的组成部分，不占第二个 Session writer。

## 8. Protocol 与持久化

- Go 中的 canonical contract registry/type 是 wire 的唯一源；TypeScript、JSON Schema、OpenRPC、
  method catalog、错误表和示例全部生成并要求 diff-free。
- 协议边界 strict decode：拒绝未知字段、缺失 discriminant、非法 enum、错 identity 和越界 payload。
- 传输只负责 framing、连接、背压和路由，不持有业务状态、不创造业务终态。
- 当前 Lyra Runtime Protocol 是 app2 的首选合同基线：dotted method、operation metadata、
  streamable HTTP、SSE replay、sidecar、capability/idempotency/pagination 语义默认保留。只有真实反例与
  新 ADR 能改变它；Codex 只提供进程、背压和生命周期机制参考，不能成为协议命名或 wire 的来源。
- 请求、通知和 server request 均携带连接代际；迟到结果不得提交到 successor。
- SQLite 是 app2 durable truth。一个 build 只接受一个精确 schema epoch；开发期直接重建，不保留迁移链。
- 不建立 SQLite 与 JSONL 两套权威历史。需要导出/诊断时从 canonical store 生成 artifact。
- secret 原文永不通过读 API 返回；日志、trace、错误和快照都执行相同脱敏策略。

## 9. React、TypeScript 与 Wails

- 前端按 `shell/sessions/agent/composer/workspace/settings/runtime` 限界上下文静态组合；不恢复通用插件宿主。
- 只在存在真实多生产者时保留显式 registry，例如 tool presentation、workspace destination、command；
  registry 由具体领域命名，不得演化成万能 extension point。
- Server state 由 TanStack Query 或 Agent projection owner 持有；URL 持有导航；Zustand 只持有 UI preference、
  session-scoped ephemeral state 或 adapter projection。一个事实不能复制到多个 store。
- React component 不直接发 RPC、不 import Wails generated raw binding、不理解 SQLite row 或 wire union。
- Context 内 application/presentation 把 wire 转成 UI language；跨 context 只走 public facade。
- 避免用 Effect 作为命令总线。Effect 只同步外部系统，并完整 cleanup listener/timer/subscription。
- 不用 barrel 隐藏依赖；大依赖、编辑器、Shiki、KaTeX、Mermaid 等按真实使用路径 lazy load。
- 列表 key 必须是领域身份；流式更新不能因 index key 泄漏草稿、展开态或 HITL 选择。
- Wails v3 API 不与 v2 混用。每个 exported Go method 都是 IPC surface，必须由绑定面测试锁定。
- Renderer 输入始终不可信；路径、URL、data URL、保存内容和协议 payload 在 host/runtime 边界重新校验。
- 视觉遵循 Work Index / Agent Narrative / Context Dock 心智模型、token 化 surface ladder、桌面命中区、
  键盘/IME/RTL/窄窗/Retina 和 reduced-motion 约束。

## 10. 测试与门禁

- 本轮迁移采用一次性实现、最终统一验证：先完整写完 R2–R11 的 Runtime/Desktop 生产代码，随后只执行一轮
  总体生成、测试、race、vet、static、frontend、视觉与 package 门禁，并在该轮集中修复。实现批次之间不得
  启动局部测试循环；已有测试只作为设计证据读取，新增最终测试在统一验证阶段补齐。
- Domain 测试状态迁移和不变量；Application 测试完整 use case 与事务；adapter 测试真实边界；
  transport 测试 framing/背压/断线；Desktop 测试用户可观察行为。
- 每个纵切至少包含一个从 Desktop intent 到 durable Runtime fact 再回到 UI projection 的真实集成测试。
- 恢复能力必须用 renderer reload、transport close、Runtime graceful restart 和精确 SIGKILL 验证，不能用
  手工构造内存状态替代。
- 架构规则必须机器执行：依赖方向、跨 context public 边界、无环、生成 diff、绑定面、资源泄漏、
  contract consumer 覆盖和 capability ledger 状态。
- 性能判断先测量；没有 profile/bundle/interaction trace 不引入缓存、池化或复杂调度。
- 测试代码也服从所有权和清晰度；不要用巨型 fixture helper 掩盖生产 API 缺陷。

## 11. 文档纪律

- `README.md` 只维护阶段入口；`ARCHITECTURE.md` 只维护结构；`DOMAIN_MODEL.md` 只维护业务语义；
  `CONTRACT.md` 只维护外部合同；`CAPABILITY_LEDGER.md` 只维护迁移事实；`EXECUTION_PLAN.md` 只维护顺序。
- 一个变更若改变语义、合同、目录边界、owner 或验收，代码、测试、生成物和对应 owner 文档必须同批更新。
- ADR 记录决策与替代方案，不写实施流水账；已接受 ADR 不覆写历史，只追加 supersede 关系。
- 注释解释约束和“为什么”，不复述代码；公共 API 文档说明行为、所有权、取消和错误。

## 12. 每批完成与资源回收

实现阶段的每批是防丢失 checkpoint，不代表能力已验证。每批必须：

1. 保持边界清晰，不引入兼容分支、重复 owner 或假实现；
2. 不把未运行门禁的能力标为 `verified`；
3. 提交并推送到当前 `codex/` 分支，防止进度意外丢失；
4. 没有修改或覆盖用户的无关 worktree 内容，尤其不得纳入 `.opencode/`；
5. 关闭本批打开的浏览器会话、agent-browser、Playwright、Vite、Wails、Runtime、MCP、LSP、
   临时 watcher、端口转发和后台命令；
6. 删除本批临时 HOME、数据库、socket、日志、截图和构建目录；保留的诊断资产必须有明确路径与用途；
7. 用进程、端口和临时目录检查证明资源已经释放，而不是只发送 stop 信号。

只有最终统一门禁通过，R2–R11 才能从 `implemented` 升为 `verified`。
