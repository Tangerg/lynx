# Lyra Runtime 执行计划

> 状态：P0–P115 已完成并形成里程碑；P116 正在实施。
>
> 最近基线：2026-08-18，P115 完整验收；P116 C1 红例已建立。

本文只拥有四类信息：当前授权、长期约束、里程碑索引、下一阶段准入。能力现状由
[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 拥有；稳定合同由
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有；架构与实施规则分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md) 和
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) 拥有。

P0–P114 的逐批红例、文件清单和门禁原始记录已冻结在 Git 快照
`babec316e:app/runtime/doc/EXECUTION_PLAN.md`。Git 是历史审计源，本文不再复制提交日志。

## 1. 当前授权

- P116 已于 2026-08-18 获得实施授权，唯一设计与验收范围为
  [`inspiration/MAINTAINABILITY_CONVERGENCE.md`](inspiration/MAINTAINABILITY_CONVERGENCE.md) 第 12 节 C1–C3；P115 的其他 breaking surface 不延续。
- C1 的行为红例已证明：`ClaimResume` 把 durable interrupt 置为 `resuming` 后，continuation opening 失败仍走 open-only `ApplyRunLost`，因而无法提交补偿，也不能释放已 staged 的 exact executor。修复只能由 claimed Resume owner 使用其已持有的 claim 提交匹配 `resuming` 的唯一 terminal write-set，durable commit 后才释放 executor；普通 `Get`、fallback、retry 和第二 recovery path 不得扩张。
- C2 必须先证明同一 Desktop、同一 durable namespace 仅更换 transport binding 时，未决 mutation 是否被错误退休；若红例成立，直接从当前 journal identity/storage shape 删除 endpoint，不保留 alias、registry、migration 或兼容双读。
- C3 只审计 `check:published-boundaries` 的 exported object-literal 语法规则是否阻止真实跨 context/public surface 泄露；无独立行为价值即删除该语法规则，继续保留 import DAG、published surface 和 consumer ownership 守卫。
- 每项先红测再修改、独立提交推送；最终运行 Frontend 全门禁与 async leak、Runtime standalone/race、Desktop Wails production build、fresh database 冷启动及真实 Runtime restart/SIGKILL。P116 不修改或暂存 `app/cli`，并保留所有无关工作区改动。

## 2. 长期产品与架构约束

1. 真实产品严格为一个 Desktop actor 对一个逻辑 Runtime。Runtime 可以经 HTTP、socket 或同进程 binding 接入；进程重启、连接重建或 binding 变化不产生“旧服务端/后继服务端”关系。Desktop 与 CLI 共享目录只属于已有存储并发合同，不扩张为多客户端产品架构。
2. Runtime 只能通过 `adapter/agentexec` 接入 Agent Framework。Agent inner ring 不得依赖 Runtime、RPC、Desktop、SQLite、持久化或产品终态词汇。
3. Domain、Application、Adapter、Infra、Delivery、Bootstrap 各自拥有自己的事实和机制；跨层依赖必须沿既定方向，不能用 locator、全局可变状态或 DTO 反向穿透。
4. mutation、query、event、optimistic state 和 material snapshot 不能同时写同一个 read model。每份可见状态必须有唯一 writer、generation 和提交边界。
5. renderer、Plugin Host、Runtime 进程和 connection 只在各自真实可替换的进程内资源上使用 generation。迟到 response/callback、final close 和重复 dispose 必须服从 exact owner identity；进程代际不能冒充逻辑 Runtime 或 durable store identity。
6. 修复必须落在所有权、生命周期、事务边界或领域不变量上。禁止兼容补丁、双路径、刷新绕过、基于延时的竞态掩盖和“先留 TODO”。
7. 优先 OOP 与充血模型。对象应持有行为所需的状态、不变量和生命周期；不把同一业务动作摊成跨文件过程脚本。
8. 不过度设计和过度防御。抽象、generation、guard、retry、锁、状态机和测试层级都必须对应当前产品中的真实反例；边界已经验证且事实不可变时，不重复包装或校验。

## 3. 参考基线

参考用于提取机制和反证，不是兼容合同，也不是目录/类型形状模板。

- 服务端主参考：[`/Users/tangerg/Desktop/study/codex-server/codex-rs`](/Users/tangerg/Desktop/study/codex-server/codex-rs)，重点研究 Rust 实现中的进程 incarnation、事件/请求 identity、断线恢复、持久状态重建、取消和迟到 settlement。
- 前端主参考：[`/Users/tangerg/Desktop/study/codex`](/Users/tangerg/Desktop/study/codex) 解包 UI，重点研究 Run Summary、Terminal、Diff、Tool/审批卡、Goal/Plan、Session navigation、Dock、loading/empty/error feedback 和长对话心智模型。
- 前端补充参考：[`/Users/tangerg/Desktop/study/zcode`](/Users/tangerg/Desktop/study/zcode) 与 [`/Users/tangerg/Desktop/study/minimax`](/Users/tangerg/Desktop/study/minimax)。
- 每个采纳点必须说明 Lyra 中真正的 owner；不采纳时记录产品约束或架构理由。不得复制 Codex 的多 connection 设计、私有状态、包结构或产品词汇。

## 4. 历史里程碑索引

| 阶段     | 完成主题                                                                                                        | 稳定结论                                                                                                                                                                                  |
| -------- | --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0–P12   | 事实基线、目标依赖 DAG、Run/Interaction/Tool/HITL/child/recovery 纵切、协议与消费者移交                         | 建立 Clean Architecture 与 Agent Framework 防腐边界，删除原执行双环和迁移兼容面                                                                                                           |
| P13–P23  | 领域与 Application 精修、package/命名、公共 `protocol`/`embedded`、真实 consumer、WorkingContext provenance     | 公共 Go surface 和内部职责分离，领域行为回到聚合，消费端只依赖明确合同                                                                                                                    |
| P24–P38  | Runtime/Desktop 时序、HITL、Goal、Run 订阅、事件断号、失效 Session 与恢复                                       | 冷启动、断线、终态和旁路事件开始服从持久事实与单一恢复入口                                                                                                                                |
| P39–P58  | Session lifecycle、command replay、mutation/read model、Knowledge CAS、Git/HTTP sidecar、Runtime 连接和外部失效 | 条件写、文件身份、事件协商、连接 owner 和配置失效形成可证明边界                                                                                                                           |
| P59–P83  | 单客户端真实交错、SQLite owner/lease、事件与物料一致性、幂等和恢复反证                                          | 中间批次持续消除跨代拼接、双 writer、恢复猜测和非原子结算；详细证据见冻结快照                                                                                                             |
| P84–P97  | 多 Runtime 共享库业务所有权、survivor recovery、compaction、EventCommit 与 composite command 回执丢失           | SQLite durable winner、exact command receipt 和 request-detached proof 成为唯一事务结算依据                                                                                               |
| P98–P112 | Desktop Plugin Host、sideload、Context Dock、Tool/Terminal、Run Summary、审批事实、material snapshot            | 插件与 renderer 生命周期收敛；挂载 Session 的 HITL/Plan/Goal/Run/Tool 以一次 material transaction 恢复                                                                                    |
| P113     | Runtime 内部坏味道治本清理                                                                                      | 合法构造、required dependency、共享并发 owner 与 package 粒度收敛；只为单一调用者存在的微模块被吸回真正 owner                                                                             |
| P114     | 单 Desktop/逻辑 Runtime 的进程恢复、真实接线和 UI 打磨                                                          | renderer、Runtime process、command、query 与 material 在真实替换点服从 exact owner；断线、重启、SIGKILL、迟到响应和长对话产品路径完成反证闭环                                             |
| P115     | 前后端可维护性治本收敛                                                                                          | durable unresolved-command、lifecycle publication、Frontend extension、Bootstrap resource 与 Runs execution handoff 各自回到唯一 owner；真实 SIGKILL 验收补齐 path-owned local credential |

## 5. 当前里程碑结论

P113–P115 共同建立了以下不可回退的心智模型：

- 产品始终只有一个 Desktop actor 和一个逻辑 Runtime。renderer、Plugin Host、Runtime process、connection、command、query writer 和 mounted material 仅在真实可替换边界拥有局部 generation。
- Runtime 每次进程实例发布新的 opaque `instanceId`；同 endpoint 重启只替换进程内资源，不替换逻辑 Runtime、SQLite durable identity 或 mutation store identity。
- 进程内 owner replacement 先发布新实例，再同步退休旧实例。只有异步间隙可能发生 replacement，且后续会修改当前共享状态时，提交和 cleanup 才需要 exact owner proof。
- `sessions.snapshot` 是挂载 Session 的原子 material owner；HITL、Plan、Goal、Run、Tool 不能再由独立 query/event/material 多路拼接。
- durable mutation 以事务 marker/identity 判断“已提交但成功回执丢失”；不得靠重试猜测或本地 optimistic 状态冒充服务端事实。
- Run Summary、Terminal、Diff、Tool selection、Goal、Plan、审批、Session/Dock navigation 只消费所属 Session 与 generation 的权威投影。
- Desktop 冷启动依赖在 composition root 显式声明；Composer、Recipes 和 Workspace Events 的 session ports 不再依赖偶然安装顺序。
- local transport token 由 durable data path 拥有，不属于 Runtime process generation；`instanceId` 换代不撤销仍存活 Desktop 的认证能力。
- 流式输出期间，消息底部反馈/操作区服从可见 turn 的稳定 material 边界，不能跟随每个 delta 反复挂载造成闪烁。

最近一次完整验收基线：Frontend 308 files / 1925 tests 与严格异步泄露门禁全绿，98 条 published context edge 无环，87/87 Runtime operation fact families、3/3 sidecars、16/16 events 有产品消费者；Runtime standalone 全量 test/vet/build 和 Runs/Bootstrap/SQLite/HTTP/architecture race 全绿；Desktop Wails v3 Go test/vet/build、生产 `.app` 打包和 fresh database 冷启动通过。真实 SIGKILL 让 Runtime 发布新 `instanceId` 后，path-owned token 保持同一值，存活 Desktop 自动恢复 discovery/RPC 且 successor 请求全部为 200。

## 6. 新阶段准入

新 Goal 必须先完成以下内容，才可开始生产代码：

1. 用当前产品构建提出一个可复现的红色反例；“代码看起来不舒服”只能触发审计，不能直接触发抽象。
2. 指明唯一状态 owner、生命周期、事务边界和允许的 breaking surface。
3. 对照服务端/前端参考，分别记录采纳机制与拒绝理由。
4. 按改动风险定义一个 Desktop/逻辑 Runtime 的真实恢复矩阵和必要门禁；不要求每个局部批次机械运行 SQLite、Frontend、race、fuzz 与生产 Wails 的全集。
5. 证明没有引入第二 writer、第二执行循环、兼容双读、刷新旁路、timer 掩盖或对 `app/cli` 的改动。
6. 证明没有为多窗口、多服务端、假想 transport 组合或不可达状态引入抽象与防御分支。

候选方向保留在 [`inspiration/`](inspiration/)；它们不是实施授权。开始下一阶段时新建简短阶段条目，完成后只更新里程碑结论与能力事实，不恢复逐提交流水账。

P115 已完成上述准入审计：R1–R5 均由审计时的生产代码和既有交错测试证明，定向基线为 6 个 Frontend test files / 86 tests 全绿。审计时 Mutation Journal 同时拥有三代 persisted codec、renderer/process ownership、heartbeat/leader election 与 command settlement；15 个业务对象重复 static publication/retirement；静态 contribution factory 存在无独立消费者和不变量的 application 层；Bootstrap 的宽 Stack/foundation 与 Runs 的跨纵切 Coordinator 均有真实传播和认知热点。参考实现只提供 identity、actor/capsule 与 settlement 机制证据，不改变 Lyra 的单 Desktop、单逻辑 Runtime、单窗口和既有领域合同。

P115 后复核校正了后续工作的产品模型：进程/connection incarnation 不是逻辑 server generation，transport endpoint 也不是 durable store identity。复核只保留能在一个 Desktop 与一个逻辑 Runtime 中证明的候选问题，包括 claimed Resume 进入 `resuming` 后的失败补偿是否还能读取并终结该 claim，以及当前 Mutation Journal 把 endpoint 纳入 durable scope 是否错误耦合了 binding 与 store identity。它们必须先形成红色行为测试，不能通过 open/resuming 双读、endpoint alias、fallback、retry 或新增通用 generation layer 修补。详细范围见 [`inspiration/MAINTAINABILITY_CONVERGENCE.md`](inspiration/MAINTAINABILITY_CONVERGENCE.md) 第 12 节；该记录不构成新的生产实施授权。

P116 C1 已形成真实状态语义红例：`ClaimResume` 成功后，普通 `Get` 只投影 `open`，原 `claimedResumeAttempt.fail → ApplyRunLost` 因而无法读取自己的 `resuming` claim；RunLost 未提交时 staged executor 也按既有次序保持占有。修复没有扩张普通读取或增加 recovery path：claimed owner 直接携带 claim 返回的 immutable `Pending`，Sessions 由该事实生成现有 terminal plan，并标明只能消费 claimed Resume；SQLite write-set 只删除同 Session/root Run/root member 且 `state='resuming'` 的行，随后在同一事务 terminalize Run tree，commit 成功后原 owner 才释放 exact executor。正反测试同时证明该写集不能删除普通 `open` barrier 或不同 root member 的 claim。

Batch 1 已删除 Mutation Journal 的 legacy codec、migration、persisted owner/lease、heartbeat、leader election、claimable 与 settled 状态。当前唯一 storage shape 只持久化未决命令的 salted fingerprint、idempotency key、Runtime endpoint/namespace 和 retention boundary；请求参数不落盘。renderer 提交权由进程内 exact object identity 拥有，replacement 先发布 successor 再同步退休 predecessor；前任迟到 response、error 和 dispose 均不能删除或结算后继身份。相同 Runtime store 可从未决记录恢复 exact key，endpoint/namespace replacement 则删除旧 scope 并生成新身份。定向验收覆盖 3 个 test files / 44 tests。

Batch 2 已删除 15 个业务 Owner 各自复制的 `static #active` publication/retirement 模板，并把 Runtime connection 与 HITL response coordinator 的同类 module-global 交接纳入同一机制。`publicationSlot.ts` 只拥有 process-local exact object identity、successor-first publication 和 exact withdrawal，不持有业务 task、cache、event、error 或 material；各 concrete owner 继续拥有 serialization、abort、projection repair 和 typed retirement。Singleton application port 复用该 identity primitive。Recipes plugin final cleanup 现在 fencing 晚到 fetch，并删除自己留下的 inactive query，修复了可稳定复现的 async timer leak。定向 owner/lifecycle 验收覆盖 30 个 test files / 159 tests。

Batch 3 已把 30 个只投影静态 extension spec 的 application contribution module 吸回各插件 composition entry，并删除只复述对象字面量的测试。Composer contribution 混合文件被拆除，只保留有真实键位语义的 `composerKeyBindings`；tool family、default command policy、Session search behavior 与 Work Index published facade 因独立不变量继续保留。`check:published-boundaries` 现在按 AST 拒绝“所有 exported function 都只返回对象字面量”的 application contribution module，不依赖具体历史路径白名单。Chat search 新增 `messages` / `transcript` 可发现关键词，用单文件真实功能修改证明 registration 触达面已收敛到 owner entry。

Batch 4 已删除向 Instance、Delivery 和测试传播 concrete coordinator 的宽 `Stack`，并删除把 18 个裸依赖聚合后立即逐项解包的 `assemblyFoundation`。Assembly 现在顺序构造 policy、workspace、execution 三个 package-private capsule；tool builder 从 12 参数函数 seam 收敛为单一 feature dependency value，且所有 tool closer 与 executor 在失败返回前先转交 `hostLifetime`。Host 私有 application capsule 直接持有 `server.Config` consumer surface、Session startup recovery、scheduler/recovery workers 与窄 idempotency port；Instance 只调用 capsule 行为。operation service/endpoint 形成 `operationDelivery` lifecycle capsule，external-change observer 启动失败时同步停止 admissions、取消 endpoint 并等待退出。架构门禁要求 Host 零 exported field、Delivery consumer config 与窄可靠性 port 不得退化为 Stack locator，并继续以闭集限制 Bootstrap receiver 只能拥有 construction/lifecycle 行为。

Batch 5 从五条产品纵切复核 Runs owner。fresh Start 原先在 `StageRoot` 成功后、Session model replacement 准备失败时没有 owner 释放 executor；现在 `stagedExecutionHandoff` 唯一拥有 stage→opening 窗口，并在 transfer 前任一失败精确释放。HITL Resume 原先由多处分支手写 `RunLost → Release`；现在 `claimedResumeAttempt` 独占已消费 durable claim 和 staged continuation，严格先提交 `RunLost`，成功后才把 executor 交给 Segment lifecycle。boot recovery 的 active slice、claimed map 和 reverse-release closure 已收回 exact-once `recoverySessionClaims`，write-set 仍只由 `recoveryPlanner` 生成。running root/child cancel 继续由带锁 `runTreeOwner`/child-cancellation arbiter 拥有；waiting-child cancel 继续由 immutable `waitingCancellationTransformation` 与 one-shot `WaitingSubtreeChange` 拥有，因为它们已经对应真实事务和 executor apply/discard 边界。`SessionPorts`/`ProjectionPorts` 经消费者审计后保留为 composition-only grouping；Coordinator 仍按窄消费能力存储，不把它们升级为 locator 或 mirror config。

Batch 6 把生产注释收敛为当前约束、owner 和机制说明，并删除迁移故事与已经失效的实现考古。最终真实 Wails + fresh SQLite 验收额外发现：Runtime 进程被 SIGKILL 后，新进程为同一路径生成新 local token，存活 Desktop 只持有启动时凭据，自动重连因此稳定返回 401。红测证明同一路径两次打开得到不同 token；根因不是 Desktop 缺少刷新，而是 data-path credential 被错误绑定到 process generation。现在 `OpenLocalToken` 读取并校验现有 0600/32-byte credential，只在路径缺失时通过完整临时候选与原子 hard-link 发布新值；Runtime `instanceId` 继续正常换代，Desktop 不增加 401 特判、token hot reload 或第二 credential lifecycle。真实复跑中 `instanceId` 已变化、token 哈希保持一致，恢复后的 discovery 与 RPC 全部为 200。
