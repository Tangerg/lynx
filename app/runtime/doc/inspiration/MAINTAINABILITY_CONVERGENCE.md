# 前后端可维护性治本收敛提案

> 状态：P115 与 P116 已完成；产品模型校准结论见第 12 节。
>
> 适用范围：`app/runtime`、`app/desktop` 与 `app/desktop/frontend`；`app/cli` 明确不在范围内。
>
> 本文只拥有 P115 结构治理的缺陷假设、反例矩阵、目标边界和验收建议。当前授权与进度仍由
> [`../EXECUTION_PLAN.md`](../EXECUTION_PLAN.md) 拥有；当前能力事实、架构和合同分别以
> [`../CAPABILITY_LEDGER.md`](../CAPABILITY_LEDGER.md)、[`../ARCHITECTURE.md`](../ARCHITECTURE.md)
> 和 [`../CONTRACT_BASELINE.md`](../CONTRACT_BASELINE.md) 为准。

## 1. 结论

当前仓库不是整体架构失控，而是出现了两种不同的维护问题：

1. 后端的正确性边界总体可靠，但复杂度过度集中在 Bootstrap 和 `application/runs`，导致对象图、事务编排和执行生命周期难以局部理解；
2. 前端的代际正确性和插件边界已经建立，但局部把真实 generation 问题扩张成兼容迁移、多窗口选主、重复 Owner 模板和形式化分层，导航与修改成本偏高。

因此下一轮不能以“减少文件”“减少接口”或“把目录拍平”为目标。Goal 应当消除没有独立变化原因的边界，同时保留经过真实失败窗口证明的领域、事务和生命周期复杂度。

一句话目标：

> 让每一个 package、文件、Owner、port 和 generation 都能证明自己保护了一个真实不变量；证明不了的边界吸回唯一 owner，证明成立的复杂度通过对象行为和局部 composition 收敛，而不是继续拆层。

## 2. 审计基线

本提案基于 P114 封板后的当前代码完成只读审计。规模只用于定位认知热点，不作为机械删减指标：

- Runtime 约 106k 行生产 Go、87k 行测试、107 个生产 package；其中 51 个 package 只有一个生产文件；
- Runtime 约 263 个生产接口，主要位于 Application、Adapter、Delivery 和 Bootstrap，Domain 没有 I/O port 接口；
- `internal/application/runs` 约 16.5k 行生产代码、55 个生产文件；
- `internal/bootstrap/assemble.go` 约 879 行，持有宽 `Stack`、长 tool environment 构造签名和宽 assembly foundation；
- Frontend 约 84k 行生产 TS/TSX、906 个生产文件、214 个 builtin 子目录；约 100 个生产文件不超过 10 行；
- Frontend 至少 15 个功能对象重复实现 `static active → install/current → retire/dispose` 生命周期；
- `rpc/mutationJournal.ts` 约 1027 行，同时承担 durable mutation identity、renderer generation、进程 claim、owner lease、heartbeat 和旧版本迁移。

这些数字不直接证明坏味道。后续每项改动仍必须从真实产品反例开始，并证明删除或重塑的边界没有独立 owner。

## 3. 必须保留的复杂度

以下部分不能因为代码多、文件多或只有一个消费者而机械合并。

### 3.1 Runtime 领域与事务边界

- Run、Session、Transcript、Pending、Goal、Plan、Tool 和 accounting 是不同事实 owner；
- fresh start、waiting barrier、answer claim、resume、child admission、unknown settlement 和 terminalization 的 write-set 顺序属于 Application；
- Agent Framework Process tree 仍只能由 Framework 推进，Runtime 只经 `adapter/agentexec` 防腐层接入；
- authoritative model/tool commit、SQLite marker proof、回执丢失和 SIGKILL recovery 都是已验证的产品复杂度；
- consumer-owned 小接口、strict codec、外部 SDK adapter 和稳定领域 value package 可以只有一个文件或一个当前消费者。

### 3.2 服务端共享数据目录

产品运行时只有一个 Desktop actor 和一个逻辑 Runtime。Runtime 可经 HTTP、socket 或同进程 binding 接入，进程重启与连接重建不产生第二个逻辑服务端。Desktop 和 CLI 可以各自打开同一数据目录；这是 SQLite、文件和工作区 owner 的存储并发场景，不是 Desktop 多客户端或多服务端拓扑。

下一轮不得把 OS advisory lease、SQLite durable winner 或 ordered recovery sweep 与 renderer replacement 混为一谈。前者保护共享目录中的业务资源，后者只保护当前 Desktop 进程内的异步提交权。

### 3.3 Frontend generation 与插件平台

- renderer replacement、Plugin Host replacement、Runtime process restart 和迟到 async settlement 必须继续服从各自的 exact owner；process generation 不能替代逻辑 Runtime 或 durable store identity；
- durable idempotency identity 必须跨 renderer reload、Runtime response loss 和进程 crash 保留；
- mounted Session 的 HITL、Plan、Goal、Run、Tool material 仍由一次 authoritative snapshot 提交；
- Plugin Host、被真实内置消费者使用的 extension point 和 public context boundary 继续保留；P140 已证明 sideload 没有真实安装且样例不满足当前 Host API，因而整条外部动态加载 transaction 已删除；
- Wails v3 壳继续只提供 packaged-app capability，不内嵌第二 Runtime 业务 API。

## 4. 红色反例与根因假设

Goal 开始后，以下每项必须先用当前生产代码得到失败证明。若反例不能成立，该项不得仅凭风格偏好进入重构。

### R1：Mutation Journal 同时拥有不相干的生命周期

#### 当前信号

`rpc/mutationJournal.ts` 同时包含：

- v1/v2/v3 entry 和 owner record；
- legacy snapshot migration 与 compatibility tombstone；
- owner lease、heartbeat、process claim 和 claimable set；
- renderer generation、mutation reservation、durable idempotency 和 settlement tracking。

真实 Desktop 当前只创建一个窗口。允许发生的是同一窗口的 renderer replacement、Runtime 进程重启和 connection rebuild，不是多个窗口或多个逻辑服务端争夺 mutation writer。

#### 必须证明的红例

1. 删除 legacy 数据后，fresh database 和当前版本启动不再需要任何旧 shape reader；
2. 同一进程 renderer A 被 B 替换时，A 的 response、track settlement 和 dispose 不能提交或清理 B；
3. renderer 进程崩溃、Runtime 已提交但成功回执丢失后，新 renderer 仍能复用 exact idempotency identity并由服务端 marker proof 收敛；
4. durable namespace 改变时，旧未决命令不能投递到重建后的 store；transport endpoint 变化本身不能被当作 store replacement；
5. final close 和重复 dispose 不重新创建 journal、heartbeat 或 claim。

#### 根因假设

一个对象同时承担了两种身份：

- durable unresolved command identity；
- 当前 renderer/process 的提交权限。

兼容迁移和多窗口选主又叠在二者之上，形成难以证明的复合状态机。

#### 目标形态

- 一个小型 durable unresolved-command journal，只保存当前唯一 schema、durable store identity、fingerprint、idempotency identity 和 settlement；transport endpoint 不参与持久命令身份；
- 一个 renderer-generation owner，只决定当前异步 workflow 是否仍有提交权，不解释持久命令格式；
- replacement 通过 exact object identity 交接，不使用 heartbeat、TTL 或多窗口 leader election；
- 直接删除 legacy key、v1/v2 codec、migration 和 compatibility tombstone，不保留双读；
- 是否需要进程内 claim 必须由同一 renderer 中可并发出现的真实调用证明，不能由未来多窗口猜测证明。

### R2：Owner 成为重复的安装模板

#### 当前信号

多个功能对象重复实现：

```text
static #active
install(...)
current()
replaceRuntimeGeneration(...)
dispose()/retire()
retired error
```

它们真实拥有的业务行为并不相同，但 active publication、successor-first replacement、迟到结算和 exact disposer 规则高度重复。

#### 必须证明的红例

1. 任取三个 Owner，构造 successor 已安装、predecessor 迟到 dispose 的交错，确认每个实现是否完全相同；
2. Runtime process/connection owner replacement 后，旧 command/query continuation 是否都拒绝提交，同时只清理自己的 task/query；
3. HMR、Host stop 和 final close 是否通过同一 retirement 语义，还是各功能存在细微差异；
4. typed retired error 是否被 UI 当作普通失败展示，或被不一致地吞掉。

#### 根因假设

generation 是真实横切机制，但当前以每功能复制完整 singleton lifecycle 的方式落地。业务 Owner 与全局 publication/retirement 原语混在一起，导致“Owner”逐渐成为新的 `Service` 后缀。

#### 目标形态

优先选择以下两种之一，并由红例决定：

1. Plugin Host/Installation 直接持有 concrete application owner，调用方从已注入 capability 取得实例，不再用每类 static singleton；
2. 若全局调用入口确实不可避免，提炼一个只拥有 successor-first publication、exact disposer 和 retirement proof 的窄 lifecycle slot；业务 serialization、query handoff、material repair 仍留在各 concrete owner。

不得创建通用 `OwnerManager`、事件总线、DI container 或能承载任意业务状态的 generation framework。

### R3：无不变量的功能也套用 application/public 层

#### 当前信号

- 一行 public re-export 文件；
- 只返回 `{id, order, component}` 的 application contribution factory；
- 对静态字面量 wrapper 单独建立测试；
- 单一 UI contribution 也形成独立目录和层级。

这与现有 Frontend 架构规则“纯 UI、单调用点、无业务不变量不需要完整分层”不一致。

#### 必须证明的红例

从一个典型静态 UI contribution 出发，记录修改它的 id、order、组件和注册位置需要跨越多少文件。若 application/public 文件没有独立消费者、边界 guard 或变化原因，它们不能继续存在。

#### 目标形态

- 纯 contribution 在插件入口直接声明；
- 只服务同一相邻 owner 的一行 facade 合并到该 context 的真实 published surface，或由稳定的 context root export；
- public surface 只在跨 context 依赖、SDK 发布语言或防止内部泄露时存在；
- UI primitive 的逐组件出口可以保留，因为它承担第三方防腐和设计系统依赖方向；
- 不设“文件数必须下降多少”的指标，避免把独立不变量重新塞进 god file。

### R4：Bootstrap 对象图通过宽参数袋传播

#### 当前信号

- `Stack` 暴露大量 concrete coordinator；
- `toolEnvironmentBuilder` 以长函数类型作为 assembly test seam；
- `assemblyFoundation` 聚合大量依赖，随后在 `buildAssemblyCore` 开头逐项解包；
- 新增一个 capability 容易同时修改 Config、foundation、tool environment、Stack 和 Host wiring。

#### 必须证明的红例

选择最近一个真实能力，回放它进入 Bootstrap 所触及的参数、字段和构造调用。区分哪些修改是必要 wiring，哪些只是因为 foundation/Stack 是宽传播袋。

#### 根因假设

组合根保持静态可搜索是正确的，但目前缺少少量具有完整合法构造和生命周期的 feature composition object，导致局部能力以裸依赖穿越整个 assembly。

#### 目标形态

- 按共同生命周期和构造不变量形成少量 package-private composition capsule；
- capsule 构造完成即合法，拥有自己的 required dependency validation 和资源回滚；
- Delivery 只取得实际 use case capability，不取得完整 Stack locator；
- Assembly 继续是唯一组合根，wiring 继续静态可搜索；
- 不引入反射 DI、service locator、builder 链、generic module 或动态注册表。

### R5：`application/runs` 成为单一认知热点

#### 当前信号

`application/runs` 同时包含 reducer、pump、commit values、recovery、cancellation、executor routes、waiting continuation、child admission 和大量消费端口。它们属于同一 Run 应用上下文，但并不全部共享同一个锁、生命周期或变化原因。

#### 必须证明的红例

分别跟踪 fresh start、HITL resume、running cancel、waiting child cancel 和 boot recovery：

- 每条纵切需要阅读哪些对象和文件；
- 哪些状态由同一个 concrete owner 持有；
- 哪些 helper 只靠调用顺序形成隐式协议；
- 哪些接口只是 composition grouping，哪些是真实消费边界。

#### 目标形态

- 保持 `runs` 作为应用上下文，不为降低行数拆出无语义微 package；
- 按锁、生命周期和事务不变量建立 package-private 行为对象；
- 让 start、continuation、cancellation、recovery 各自拥有完整的 admission、commit、apply/cleanup 方法，而不是跨文件过程脚本；
- reducer 继续唯一拥有 executor fact fold；commit value 继续表达完整 write-set；
- 只有出现独立词汇、依赖切断或独立复用时才创建新 package；
- 不建立 Saga、UnitOfWork、generic command bus 或通用 state machine framework。

### R6：生产注释承担历史考古

#### 当前信号

部分 Wails、Runtime 和 Frontend 文件保留了长篇实验过程、旧版本行为和被删除实现的说明。并发、事务、平台反直觉约束需要注释，但试验过程容易漂移并遮蔽当前合同。

#### 目标形态

- 生产注释只保留不能由代码表达的当前 why、外部约束、事务顺序和生命周期不变量；
- 可以由测试名称证明的平台行为交给测试；
- 具有长期决策价值的历史进入已有 ADR/设计 owner，不在多个代码文件复制；
- 不为这项工作单独创建新的文档体系。

## 5. 明确不判为坏味道的部分

以下内容除非出现新的失败反例，否则下一 Goal 不应顺手重写：

- 一文件 Domain package；
- consumer-owned 1–3 方法接口；
- `delivery/server` 按协议能力定义的 use-case interfaces；
- `adapter/agentexec` 的 Agent Framework 防腐边界；
- Run reducer、exact commit value、Pending projection closure 和 SQLite strict codec；
- Plugin SDK 的内置 composition barrel 与有真实贡献者/消费者的 ExtensionPoint；
- `rpc/methods.ts` 的集中 typed transport 编排，仅因行数大不足以证明要拆；
- `createSingletonPort` 的 exact-instance disposer 语义；
- Wails v3 的单一 Desktop Host 边界；
- 服务端为 Desktop/CLI 共享数据库建立的 OS advisory ownership。

## 6. Goal 建议定义

可以直接用以下目标创建下一 Goal：

> 以当前 P114 基线为起点，从真实红色产品反例出发，治本收敛 Runtime 与 Desktop Frontend 中没有独立变化原因的 package、facade、Owner 和 generation 机制。优先把 mutation journal 重建为一个 Desktop actor、一个逻辑 Runtime 和单窗口 renderer replacement 下的唯一 durable unresolved-command owner；统一进程内 lifecycle publication，删除 legacy/multi-window/heartbeat 兼容状态；随后收敛无不变量的 Frontend 分层，并按锁、生命周期和事务边界重塑 Bootstrap 与 application/runs 的内部对象。保留 Agent Framework 防腐、SQLite durable winner、Run/Transcript/Pending 事务、Plugin Host 和 authoritative Session material。允许一次性 breaking change，禁止兼容补丁、双路径、刷新绕过、timer 掩盖、通用 DI/EventBus/Saga 框架和对 `app/cli` 的修改。后端主要参考 `/Users/tangerg/Desktop/study/codex-server/codex-rs`，前端主要参考 `/Users/tangerg/Desktop/study/codex`，zcode/minimax 仅作补充；只提取机制证据，不复制多 connection 产品设计或目录形状。

## 7. 建议批次

批次顺序必须服从依赖和可独立验收，不要求一次 Goal 全部并行展开。

### Batch 0：反证与爆炸半径

- 固化 R1–R5 的当前失败或维护性证据；
- 为每项写出唯一 owner、linearization point、successor/final-close 行为和允许的 breaking surface；
- 统计真实消费者和测试，不按文件名猜边界；
- 对照 Codex 服务端/前端参考，记录采纳机制与拒绝理由；
- 若某项不能形成红例，从 Goal 删除，不以“顺手清理”进入生产改动。

### Batch 1：Mutation Journal 单一职责重建

- 删除 legacy shape、迁移、compatibility tombstone、多窗口 heartbeat/lease；
- 分离 durable command identity 与 renderer commit authority；
- 保留 exact idempotency、durable namespace fencing、success-receipt-loss recovery；
- 覆盖 renderer replacement、同一 Runtime 的进程重启、final close、重复 dispose 和 storage failure；
- breaking 后只存在一个 storage shape 和一条生产路径。

### Batch 2：Frontend lifecycle owner 收敛

- 审计所有 static active Owner 的公共生命周期；
- 选择 Host-owned instance 或窄 lifecycle slot；
- 删除重复 publication/retirement 模板，保留各业务 Owner 的 serialization、material repair 和领域行为；
- 证明旧 disposer、迟到 response、query reset 和 task settlement 不能命中 successor；
- 不创建通用业务 owner framework。

### Batch 3：Frontend 无价值分层收敛

- 合并纯 contribution wrapper、无边界价值的一行 facade 和只验证字面量的测试；
- 保留跨 context published language、SDK surface 和 Design System 防腐出口；
- 更新 layer/public/circular gates，使规则表达长期语义而不是历史文件位置；
- 用一个真实小功能修改证明触达面下降。

### Batch 4：Bootstrap 合法 composition 收敛

- 以真实能力 wiring 红例重塑 assembly；
- 用少量 package-private capsule 收敛共同构造和生命周期，不扩散新 package；
- 删除仅为宽参数传播或测试注入存在的 facade；
- 保持唯一静态 composition root、资源逆序关闭和失败回滚；
- Runtime public Go API、Protocol 和 SQLite shape 默认不变。

### Batch 5：Runs 内部行为对象收敛

- 分别审计 start、continuation、cancellation、recovery 的锁和事务 owner；
- 把隐式 `validate → mutate → cleanup` 调用协议收回 concrete behavior object；
- 删除无独立消费价值的 config mirror、port bundle 和过程 helper；
- 保持 reducer、Domain aggregate、write-set 和 Agent Framework port 的唯一 owner；
- 只在真实 package boundary 成立时移动代码，不以行数拆包。

### Batch 6：注释与门禁收口

- 删除生产代码中的历史考古和重复 architecture prose；
- 保留反直觉平台、并发、事务和安全约束；
- architecture gates 只守依赖 DAG、唯一 owner、构造合法性、生命周期和无 legacy；
- 不建立文件位置白名单或代码行数 KPI。

## 8. Breaking surface

下一 Goal 可以修改或删除：

- Frontend mutation journal storage shape 和 local persisted data；
- internal Owner install/current/dispose API；
- builtin context 内部目录、facade 和 contribution factory；
- Runtime internal Bootstrap constructor、assembly type 和 package-private component；
- Runtime internal `application/runs` constructor、port grouping 和 helper shape。

默认不修改：

- Runtime Protocol、Artifact 和 SQLite schema；
- 公共 `runtime/protocol` 与 `runtime/embedded` Go API；
- Agent Framework public API/baseline；
- Frontend published plugin SDK；
- `app/cli`。

若红例证明必须突破默认边界，必须先列出爆炸半径和唯一新合同，再由用户确认。不得用兼容层降低 breaking 成本。

## 9. 验收矩阵

### 9.1 Frontend

- renderer A → B replacement：A 的 mutation response、query writer、stream callback、reset/remove 和 dispose 均不能影响 B；
- final close：重复 dispose 共享 settlement，不重建 root、journal、client 或 Host；
- Runtime same-endpoint restart：新 `instanceId` 只建立新的 process/event incarnation；旧 stream/query callback 不跨进程 owner，但 durable mutation identity 仍属于同一逻辑 Runtime/store；
- committed-success response loss：exact command identity 由 durable proof 收敛，不重复副作用；
- fresh storage：只有当前 journal shape；旧 key 存在时按当前 breaking policy 丢弃，不迁移；
- Session switch、Dock fold/restore、long conversation、compaction、HITL continuation 和 tool selection 不复活旧 material；
- `npm run check` 全门禁与 async leak detection 全绿；
- 生产 Wails 冷启动和 renderer replacement smoke 通过。

### 9.2 Runtime

- fresh start、waiting/resume、cancel、child cancellation、unknown settlement 和 boot recovery 语义零回归；
- SQLite transaction failure 与 success receipt loss 仍服从 exact marker proof；
- Runtime SIGKILL、survivor recovery 和共享数据库 ownership 零回归；
- Agent Framework import 仍只位于 `adapter/agentexec`；
- Domain/Application/Adapter/Infra/Delivery/Bootstrap DAG 全绿；
- Runtime build、vet、staticcheck、全量 test、相关 race/SQLite invariant/contract gates 全绿；
- Wails v3 Go test、vet、build 全绿。

### 9.3 结构验收

- 每个新 package、interface、facade 和 Owner 能说明保护的不变量或切断的依赖；
- 每个被删除边界的行为已由相邻唯一 owner 接管，没有转发 shim；
- 没有 legacy codec、dual read/write、alias、fallback 或第二 generation path；
- 没有通用 DI container、service locator、EventBus、Saga、Repository 或 Owner framework；
- 文件数、接口数和行数可以下降，但不作为验收条件；
- 新人跟踪一个 mutation 和一个 Run lifecycle 所需的 owner/文件集合应可明确列出，且不存在同一事实的多个 writer。

## 10. 执行纪律

- 新 Goal 建立前不修改生产代码；
- 每个批次先提交红色反例，再修改生产实现；
- 每批完成一个完整语义纵切，旧 owner 同批删除；
- 精确暂存本批文件，保留用户无关改动，绝不修改或暂存 `app/cli`；
- 每批更新 Execution Plan 的当前阶段和 Capability Ledger 的完成事实，但不恢复逐提交长流水账；
- 每批形成可独立 revert 的 commit 并推送；
- 浏览器自动化后关闭本批会话与 daemon，不关闭用户已有 Wails/Vite/Chrome 进程；
- 除非真实反例、外部合同或用户决定改变范围，不把结构治理扩张为功能开发。

## 11. 完成定义

该 Goal 只有在以下条件同时成立时完成：

1. Mutation Journal 只有当前唯一 storage shape，durable identity 与 renderer commit authority 各有唯一 owner；
2. Frontend replacement-safe lifecycle 不再由十余个功能重复实现基础 publication/retirement 模板；
3. 纯 UI contribution 不再被迫拥有无业务意义的 application/public 仪式层；
4. Bootstrap 的局部能力可以在不穿越宽 foundation/Stack 参数袋的情况下合法构造和回滚；
5. `application/runs` 的主要纵切分别由可命名的行为对象拥有，不依赖跨文件隐式调用协议；
6. 所有已验证的 generation、SQLite、Run、HITL、Goal、Plan、Tool、Session material 和 Agent Framework 边界零回归；
7. 没有以新增通用抽象替代旧过度抽象，也没有为了减少文件制造新的 god object；
8. 全部门禁、真实冷启动、renderer replacement、Runtime 进程重启、SIGKILL 和异步泄露验收通过。

最终衡量标准不是“代码更少”，而是：

> 一个事实只有一个 writer，一个异步流程只有一个 generation owner，一个对象只保护一组共同变化的不变量；任何额外边界都必须能用真实失败反例证明其存在价值。

## 12. P116 产品模型校准结论

P116 已完成原 C1–C3 的红例、根因修复和验收。具体授权、里程碑与当前能力分别由
[`../EXECUTION_PLAN.md`](../EXECUTION_PLAN.md) 和 [`../CAPABILITY_LEDGER.md`](../CAPABILITY_LEDGER.md) 拥有；本节只保留后续设计必须接受的稳定结论：

- claimed Resume 的失败补偿由持有 durable claim 的 owner 直接提交匹配 `resuming` 的 terminal write-set；普通读取不扩张为 open/resuming 双读，durable commit 前不释放 executor；
- Mutation Journal 的 durable identity 由 namespace、command fingerprint、idempotency key 和 retention boundary 决定；transport endpoint 不参与 store identity；
- architecture fitness test 保护依赖方向、published surface 和 consumer ownership，不冻结文件名、对象字面量或一次重构的语法形状；
- 一个 Desktop actor 始终对应一个逻辑 Runtime。进程、connection 与 binding 的换代只隔离其真正拥有的局部资源；
- Desktop 与 CLI 共享目录继续服从既有 SQLite、文件、Session writer 和 working-tree owner，不推广为通用多客户端协议。

### 12.1 明确排除的推导

以下场景不再作为缺陷依据：

- “旧逻辑服务端”的迟到响应写入“后继服务端”；产品没有这两个并存身份；
- 两个 renderer 或多个 Desktop 同时竞争 mutation leader；真实产品只有一个 Desktop actor；
- 仅因 mutation 参数形状相同就假设恢复歧义；没有单 Desktop 的可复现错误，不增加 disambiguation 状态；
- 为未来 socket、embedded 或远程部署预建 transport factory、server registry、连接矩阵或通用 generation framework；
- 为每个 `await`、每个函数入口或每个内部值重复验证 immutable identity。

### 12.2 后续实现尺度

后续每个候选项仍须先用最小红色反例锁定根因，再完成一个根因修复纵切。能在现有 owner 内修复就不新增 package、interface、facade、manager 或状态机；只有新对象确实拥有独立状态、行为和生命周期时才建立边界。定向测试覆盖根因与必要的相邻失败窗口，里程碑封板再运行完整门禁。

下一阶段候选已转向 [`Desktop 恢复体验与 UI 精修`](DESKTOP_RECOVERY_EXPERIENCE.md)，不再重做 C1–C3 或建立全局 generation framework。
