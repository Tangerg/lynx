# Lyra Runtime 执行计划

> 状态：P0–P114 已完成并形成里程碑；P115 已授权，Frontend 与 Bootstrap 收敛批次已完成，当前执行 `application/runs` 行为对象重塑。
>
> 最近基线：2026-08-18，commit `babec316e`。

本文只拥有四类信息：当前授权、长期约束、里程碑索引、下一阶段准入。能力现状由
[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有；稳定合同由
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有；架构与实施规则分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md) 和
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 拥有。

P0–P114 的逐批红例、文件清单和门禁原始记录已冻结在 Git 快照
`babec316e:app/runtime/doc/EXECUTION_PLAN.md`。Git 是历史审计源，本文不再复制提交日志。

## 1. 当前授权

- P115 已于 2026-08-18 授权，设计与验收 owner 为
  [`inspiration/MAINTAINABILITY_CONVERGENCE.md`](inspiration/MAINTAINABILITY_CONVERGENCE.md)。
- Batch 0 已固化 R1–R5 的真实反例、唯一 owner、linearization point、breaking surface 与参考机制；Batch 1 已重建 Mutation Journal；Batch 2 已收敛 Frontend lifecycle publication/retirement；Batch 3 已合并没有独立不变量的 facade 与 contribution 分层；Batch 4 已重塑 Runtime Bootstrap composition 与资源生命周期。当前执行 Batch 5，按锁、生命周期和事务不变量重塑 `application/runs` 内部行为对象。
- breaking change 被允许，但只用于建立更准确的唯一合同；禁止以 breaking change 为名制造并行实现或迁移半成品。
- `app/cli` 不在本计划授权范围内，不得修改或暂存。
- 保留所有无关工作区改动；每个独立批次精确暂存、提交并推送。

## 2. 长期产品与架构约束

1. 真实产品严格为一个 client 对一个 server。优先验证同一客户端内的真实交错、失败窗口、恢复流程和 SQLite 不变量，不堆砌无业务意义的多客户端/高并发 race。
2. Runtime 只能通过 `adapter/agentexec` 接入 Agent Framework。Agent inner ring 不得依赖 Runtime、RPC、Desktop、SQLite、持久化或产品终态词汇。
3. Domain、Application、Adapter、Infra、Delivery、Bootstrap 各自拥有自己的事实和机制；跨层依赖必须沿既定方向，不能用 locator、全局可变状态或 DTO 反向穿透。
4. mutation、query、event、optimistic state 和 material snapshot 不能同时写同一个 read model。每份可见状态必须有唯一 writer、generation 和提交边界。
5. renderer replacement、Runtime replacement、final close、重复 dispose、迟到 response/callback 和成功回执丢失必须服从 exact owner identity。旧代只能结算自己，不能写入、清理或取消 successor。
6. 修复必须落在所有权、生命周期、事务边界或领域不变量上。禁止兼容补丁、双路径、刷新绕过、基于延时的竞态掩盖和“先留 TODO”。
7. 优先 OOP 与充血模型。对象应持有行为所需的状态、不变量和生命周期；不把同一业务动作摊成跨文件过程脚本。

## 3. 参考基线

参考用于提取机制和反证，不是兼容合同，也不是目录/类型形状模板。

- 服务端主参考：[`/Users/tangerg/Desktop/study/codex-server/codex-rs`](/Users/tangerg/Desktop/study/codex-server/codex-rs)，重点研究 Rust 实现中的进程 incarnation、事件/请求 identity、断线恢复、持久状态重建、取消和迟到 settlement。
- 前端主参考：[`/Users/tangerg/Desktop/study/codex`](/Users/tangerg/Desktop/study/codex) 解包 UI，重点研究 Run Summary、Terminal、Diff、Tool/审批卡、Goal/Plan、Session navigation、Dock、loading/empty/error feedback 和长对话心智模型。
- 前端补充参考：[`/Users/tangerg/Desktop/study/zcode`](/Users/tangerg/Desktop/study/zcode) 与 [`/Users/tangerg/Desktop/study/minimax`](/Users/tangerg/Desktop/study/minimax)。
- 每个采纳点必须说明 Lyra 中真正的 owner；不采纳时记录产品约束或架构理由。不得复制 Codex 的多 connection 设计、私有状态、包结构或产品词汇。

## 4. 历史里程碑索引

| 阶段 | 完成主题 | 稳定结论 |
|---|---|---|
| P0–P12 | 事实基线、目标依赖 DAG、Run/Interaction/Tool/HITL/child/recovery 纵切、协议与消费者移交 | 建立 Clean Architecture 与 Agent Framework 防腐边界，删除原执行双环和迁移兼容面 |
| P13–P23 | 领域与 Application 精修、package/命名、公共 `protocol`/`embedded`、真实 consumer、WorkingContext provenance | 公共 Go surface 和内部职责分离，领域行为回到聚合，消费端只依赖明确合同 |
| P24–P38 | Runtime/Desktop 时序、HITL、Goal、Run 订阅、事件断号、失效 Session 与恢复 | 冷启动、断线、终态和旁路事件开始服从持久事实与单一恢复入口 |
| P39–P58 | Session lifecycle、command replay、mutation/read model、Knowledge CAS、Git/HTTP sidecar、Runtime 连接和外部失效 | 条件写、文件身份、事件协商、连接 owner 和配置失效形成可证明边界 |
| P59–P83 | 单客户端真实交错、SQLite owner/lease、事件与物料一致性、幂等和恢复反证 | 中间批次持续消除跨代拼接、双 writer、恢复猜测和非原子结算；详细证据见冻结快照 |
| P84–P97 | 多 Runtime 共享库业务所有权、survivor recovery、compaction、EventCommit 与 composite command 回执丢失 | SQLite durable winner、exact command receipt 和 request-detached proof 成为唯一事务结算依据 |
| P98–P112 | Desktop Plugin Host、sideload、Context Dock、Tool/Terminal、Run Summary、审批事实、material snapshot | 插件与 renderer 生命周期收敛；挂载 Session 的 HITL/Plan/Goal/Run/Tool 以一次 material transaction 恢复 |
| P113 | Runtime 内部坏味道治本清理 | 合法构造、required dependency、共享并发 owner 与 package 粒度收敛；只为单一调用者存在的微模块被吸回真正 owner |
| P114 | 单 client/server generation、恢复、真实 Desktop 接线和 UI 打磨 | renderer/Runtime/command/query/material 服从 exact generation；真实断线、重启、SIGKILL、迟到响应和长对话产品路径完成反证闭环 |

## 5. 当前里程碑结论

P113/P114 共同建立了以下不可回退的心智模型：

- renderer、Plugin Host、Runtime process、connection、command、query writer 和 mounted material 都有显式 generation；generation 是提交能力，不只是日志字段。
- Runtime 每次进程实例发布新的 opaque `instanceId`；同 endpoint 重启也是 replacement。SQLite durable identity 与进程 incarnation 分离。
- successor 先获得当前所有权，predecessor 再同步退休；任何 await 后的提交和 cleanup 都必须重新证明 exact owner。
- `sessions.snapshot` 是挂载 Session 的原子 material owner；HITL、Plan、Goal、Run、Tool 不能再由独立 query/event/material 多路拼接。
- durable mutation 以事务 marker/identity 判断“已提交但成功回执丢失”；不得靠重试猜测或本地 optimistic 状态冒充服务端事实。
- Run Summary、Terminal、Diff、Tool selection、Goal、Plan、审批、Session/Dock navigation 只消费所属 Session 与 generation 的权威投影。
- Desktop 冷启动依赖在 composition root 显式声明；Composer、Recipes 和 Workspace Events 的 session ports 不再依赖偶然安装顺序。
- 流式输出期间，消息底部反馈/操作区服从可见 turn 的稳定 material 边界，不能跟随每个 delta 反复挂载造成闪烁。

最近一次完整验收基线：Frontend 331 files / 1976 tests 与异步泄露门禁全绿；Runtime 全量 test、SQLite/architecture invariant、vet、build 全绿；Desktop Wails v3 test/vet/build 和生产冷启动通过。精确命令、环境和逐批结果保留在冻结快照及对应提交中。

## 6. 新阶段准入

新 Goal 必须先完成以下内容，才可开始生产代码：

1. 用当前产品构建提出一个可复现的红色反例；“代码看起来不舒服”只能触发审计，不能直接触发抽象。
2. 指明唯一状态 owner、生命周期、事务边界和允许的 breaking surface。
3. 对照服务端/前端参考，分别记录采纳机制与拒绝理由。
4. 定义单客户端真实恢复矩阵、SQLite 不变量、Frontend 全门禁、异步泄露和生产 Wails 验收。
5. 证明没有引入第二 writer、第二执行循环、兼容双读、刷新旁路、timer 掩盖或对 `app/cli` 的改动。

候选方向保留在 [`inspiration/`](inspiration/)；它们不是实施授权。开始下一阶段时新建简短阶段条目，完成后只更新里程碑结论与能力事实，不恢复逐提交流水账。

P115 已完成上述准入审计：R1–R5 均由审计时的生产代码和既有交错测试证明，定向基线为 6 个 Frontend test files / 86 tests 全绿。审计时 Mutation Journal 同时拥有三代 persisted codec、renderer/process ownership、heartbeat/leader election 与 command settlement；15 个业务对象重复 static publication/retirement；静态 contribution factory 存在无独立消费者和不变量的 application 层；Bootstrap 的宽 Stack/foundation 与 Runs 的跨纵切 Coordinator 均有真实传播和认知热点。参考实现只提供 identity、actor/capsule 与 settlement 机制证据，不改变 Lyra 的单 client/server、单窗口和既有领域合同。

Batch 1 已删除 Mutation Journal 的 legacy codec、migration、persisted owner/lease、heartbeat、leader election、claimable 与 settled 状态。当前唯一 storage shape 只持久化未决命令的 salted fingerprint、idempotency key、Runtime endpoint/namespace 和 retention boundary；请求参数不落盘。renderer 提交权由进程内 exact object identity 拥有，replacement 先发布 successor 再同步退休 predecessor；前任迟到 response、error 和 dispose 均不能删除或结算后继身份。相同 Runtime store 可从未决记录恢复 exact key，endpoint/namespace replacement 则删除旧 scope 并生成新身份。定向验收覆盖 3 个 test files / 44 tests。

Batch 2 已删除 15 个业务 Owner 各自复制的 `static #active` publication/retirement 模板，并把 Runtime connection 与 HITL response coordinator 的同类 module-global 交接纳入同一机制。`publicationSlot.ts` 只拥有 process-local exact object identity、successor-first publication 和 exact withdrawal，不持有业务 task、cache、event、error 或 material；各 concrete owner 继续拥有 serialization、abort、projection repair 和 typed retirement。Singleton application port 复用该 identity primitive。Recipes plugin final cleanup 现在 fencing 晚到 fetch，并删除自己留下的 inactive query，修复了可稳定复现的 async timer leak。定向 owner/lifecycle 验收覆盖 30 个 test files / 159 tests。

Batch 3 已把 30 个只投影静态 extension spec 的 application contribution module 吸回各插件 composition entry，并删除只复述对象字面量的测试。Composer contribution 混合文件被拆除，只保留有真实键位语义的 `composerKeyBindings`；tool family、default command policy、Session search behavior 与 Work Index published facade 因独立不变量继续保留。`check:published-boundaries` 现在按 AST 拒绝“所有 exported function 都只返回对象字面量”的 application contribution module，不依赖具体历史路径白名单。Chat search 新增 `messages` / `transcript` 可发现关键词，用单文件真实功能修改证明 registration 触达面已收敛到 owner entry。

Batch 4 已删除向 Instance、Delivery 和测试传播 concrete coordinator 的宽 `Stack`，并删除把 18 个裸依赖聚合后立即逐项解包的 `assemblyFoundation`。Assembly 现在顺序构造 policy、workspace、execution 三个 package-private capsule；tool builder 从 12 参数函数 seam 收敛为单一 feature dependency value，且所有 tool closer 与 executor 在失败返回前先转交 `hostLifetime`。Host 私有 application capsule 直接持有 `server.Config` consumer surface、Session startup recovery、scheduler/recovery workers 与窄 idempotency port；Instance 只调用 capsule 行为。operation service/endpoint 形成 `operationDelivery` lifecycle capsule，external-change observer 启动失败时同步停止 admissions、取消 endpoint 并等待退出。架构门禁要求 Host 零 exported field、Delivery consumer config 与窄可靠性 port 不得退化为 Stack locator，并继续以闭集限制 Bootstrap receiver 只能拥有 construction/lifecycle 行为。
