# app2 Runtime 正向经验复盘

> 状态：不可变复盘，非规范性文档
>
> 证据快照：2026-08-24，分支 `codex/app2-greenfield`
>
> 适用范围：只评价 `app2/runtime`，并为原版 `app/runtime` 的后续治本重构提供证据和候选方向

本文不拥有 Runtime 的目标架构、当前能力、实施授权、版本或合同事实。稳定结论仍分别由
[`ARCHITECTURE.md`](ARCHITECTURE.md)、[`DECISIONS.md`](DECISIONS.md)、
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)、
[`EXECUTION_PLAN.md`](EXECUTION_PLAN.md)、[`CAPABILITY_LEDGER.md`](CAPABILITY_LEDGER.md) 和
[`CONTRACT_BASELINE.md`](CONTRACT_BASELINE.md) 拥有。本文是一份有日期的重写实验复盘；后续代码变化不回写本次
快照，真正采用的结论必须进入新的授权阶段，必要时追加 ADR。

## 1. 结论

app2 作为原版 Runtime 的替代实现并不成功，但作为一次受控的反事实删减实验是有价值的。

失败的核心不是代码完全没有优点，而是它没有同时满足“能力完整、领域类型安全、封装边界和验证证据”四个条件。
它用更少的代码展示了若干更直接的表达方式，也通过缺失能力和新生 God object 证明了：绿色重写很容易把历史复杂度
连同必要复杂度一起删除，再以更少但更胖的对象重新长回来。

因此，正确的反哺方式不是把 app2 package 或代码整体搬回原版，而是采用下面的组合：

```text
原版的完整能力、类型化领域、恢复语义、公共边界和测试证据
+ app2 已证明更直接的身份、纵切、资源所有权和验收方法
- app2 的能力缺口、公开实现包、opaque JSON、胖 facade 和薄测试
= 原版 app/runtime 的渐进式治本重构
```

决策上应把 app2 降级为 architecture spike / reference lab，停止把它当作候选替代 Runtime。后续只在原版
`app/runtime` 内按真实消费者和失败反例逐个纵切改造，不再进行第二次 big-bang rewrite。

## 2. 证据基线

下表只描述本次快照，不是持续更新的规模目标。生产行数包含非测试 Go 文件；接口数只统计显式命名的 Go
`interface` 声明；“公共 package”表示不在 `internal` 和 `cmd` 下、可被外部模块导入的 package。

| 维度 | 原版 `app/runtime` | `app2/runtime` | 解释 |
|---|---:|---:|---|
| 生产 Go 文件 | 654 | 276 | app2 明显更小，但不能证明能力等价 |
| 生产 Go 行数 | 106,242 | 53,750 | app2 约为原版一半，是删减线索，不是成功指标 |
| 测试 Go 文件 | 423 | 27 | app2 的验证密度远低于原版 |
| 测试 Go 行数 | 86,728 | 4,697 | 大量恢复、并发、故障和边界证据没有迁移 |
| 生产 Go package | 106 | 69 | 纵切更平，但仍有较多 package |
| 显式 interface | 263 | 108 | app2 证明原版接口面存在继续审计空间 |
| 直接 module 依赖 | 36 | 23 | 部分依赖可被更直接的机制取代；也包含能力未实现造成的下降 |
| 公共非命令 package | 2 | 65 | app2 严重扩大实现暴露面，不能复制 |

协议对比同样具有两面性：app2 的 37 个 `protocol` Go 文件中，27 个与原版逐字节相同，9 个修改，1 个新增。
这说明当前 Lyra Protocol 的窄腰已经成熟，正确做法是保留它并只修复有反例的缺陷；它也说明 app2 并非真正从零
重建全部语义，较小体积不能直接归因于更好的架构。

## 3. app2 真正做得更好的地方

### 3.1 没有把 Codex 的 wire 搬进 Lyra

这是 app2 最重要、也最应该保留的判断。app2 明确让当前 Lyra dotted operations、typed registry、
streamable HTTP、SSE replay、token、CORS、sidecar、capability、pagination 和 idempotency 继续构成协议窄腰；Codex
只影响进程监督、generation、背压和生命周期机制。

证据包括：

- [`app2/doc/REFERENCE_STUDY.md`](../../../app2/doc/REFERENCE_STUDY.md) 明确把现有协议判断为成熟资产；
- [`app2/runtime/operation/catalog.go`](../../../app2/runtime/operation/catalog.go) 继续以一个类型化注册表拥有 operation；
- [`app2/runtime/contractgen/generator.go`](../../../app2/runtime/contractgen/generator.go) 从注册表生成 TypeScript 合同；
- [`app2/runtime/capability_inventory_test.go`](../../../app2/runtime/capability_inventory_test.go) 锁定 89 个 operation、
  3 个 sidecar、16 个 Runtime event variant 和 7 个 Run event variant。

反哺原版时不应重写协议层，也不应照搬 Codex 的 stdio、slash method、rollout JSONL 或多 connection 产品模型。
原版现有的 `protocol + embedded + operation + generator` 继续作为不可轻易扰动的窄腰；只有真实 wire defect 才允许
通过新 ADR 和单一新 shape 修改。

### 3.2 Session 的 provider/model 成为不可拆分身份

这是 app2 相对原版最明确的领域改进。原版 Run、Goal、Schedule 已经使用
[`modelref.Selection`](../internal/domain/modelref/selection.go)，但原版
[`session.Session`](../internal/domain/session/session.go) 仍只持有 `model`。当两个 provider 发布同名模型时，Session
默认选择、fork、reload 和 Artifact import 没有唯一含义。

app2 用 [`modelselection.Selection`](../../../app2/runtime/domain/modelselection/selection.go) 把 provider/model 建模为
“同时为空或同时存在”的私有值，并让 Session、Run、Goal、Schedule 和 Artifact 传递完整配对。这个改变来自可复现的
身份歧义，不是为了模仿参考项目。

可迁经验：

- 原版 Session 应最终持有 `modelref.Selection`，而不是新增第二个选择类型；
- create/update/fork/import/default admission 必须共用同一配对规则；
- 这是高爆炸半径的语义修复，会同时触及 Protocol、Artifact、SQLite、Desktop consumer 和生成物；必须作为独立
  授权纵切完成，不能混入普通目录清理。

### 3.3 Workspace 被表达为精确值，而不是含糊的 Project 影子

app2 的 [`session.Workspace`](../../../app2/runtime/domain/session/session.go) 是经过构造验证的绝对路径值，Session
直接拥有它；Work Index 只是从 Session 投影出来的读模型。这使“执行发生在哪里”和“左栏如何分组显示”不再反向制造
一个没有独立生命周期的 Project 聚合。

这比在多个用例中传递裸 `cwd string` 更容易保持统一语言和构造合法性。反哺时应复用原版现有的 path identity、
workspace admission 和 isolation 语义，建立一个真正的领域值；不能直接复制 app2 对 filesystem root 的规则，也不能借机
删除原版 isolated workspace、Git checkpoint 或 rollback/restore 能力。

### 3.4 领域纵切缩短了能力追踪路径

app2 以 `sessionflow`、`runflow`、`planflow`、`goalflow`、`providerflow` 等纵切组织用例。一个维护者通常可以从
operation registration 进入一个 flow，再到该 flow 消费的端口和 SQLite write-set，而不需要先理解所有通用层。

正确经验不是采用 `*flow` 后缀，而是：

- 先确定一个能力的唯一用例 owner，再决定 package；
- 接口放在消费用例旁边，composition root 只知道 concrete adapter；
- 一个能力的 command、事务顺序、投影和错误语义尽量在同一纵切内可追踪；
- 只为外部防腐、独立技术机制或独立并发不变量保留额外 package。

app2 的 [`runflow.Store`](../../../app2/runtime/runflow/service.go) 已经胖到失去“窄端口”价值，说明这个方向只完成了一半。
原版反哺时应迁移“纵切所有权和 consumer-owned port”原则，而不是迁移 package 名和大接口。

### 3.5 资源获取与所有权转移更直观

[`runtimehost.openGuard`](../../../app2/runtime/runtimehost/ownership.go) 把构造期资源所有权表达得很清楚：资源获取后立即
登记回滚动作，失败时逆序释放，成功后一次性 `Disarm` 把所有权转给 Runtime。这种 acquisition ledger 比依赖每个错误
分支记住 cleanup 更容易审查。

[`localruntime.Descriptor`](../../../app2/runtime/localruntime/descriptor.go) 还把本地进程就绪交接建模为一次性、严格校验的
descriptor：nonce、PID、instance、protocol 和 token path 必须共同匹配，消费后删除。这是对“进程 generation 不是逻辑
Runtime identity”的清晰落地。

可迁经验：

- 原版 Bootstrap 每次 acquisition 都应立刻进入唯一 owner 的逆序 ledger；
- 构造成功必须发生一次明确的 ownership transfer；
- 本地 Runtime 进程交接使用 exact generation proof，不用固定端口、轮询猜测或旧连接回调；
- descriptor 属于部署/监督边界，不进入 Runtime Protocol 或 Domain。

app2 的 shutdown 实现仍有无界 `WaitGroup.Wait`、内部 `context.Background()` 和不完整 join 语义，因此只能迁移所有权
结构，不能复制其 `Close` 实现。

### 3.6 单一 durable truth 与命令重放入口更容易看见

app2 坚持 SQLite 是唯一 durable truth，没有再建立 JSONL history。Run admission、wait、resume、terminal 和事件写入由
命名明确的 SQLite transaction method 执行；operation endpoint 通过 store namespace、fingerprint、claim、complete 和 replay
统一处理幂等命令。

值得保留的经验是“一个命令只有一个 durable winner 和一个重放入口”。原版已有更成熟的 command marker、回执丢失
证明和恢复语义，不能用 app2 较弱的实现替换它。反哺重点应是减少这些机制周围的 forwarding 层和重复表达，让现有
语义更容易从 operation 追踪到 Application write-set，而不是重写幂等算法。

### 3.7 可读的公共验收场景补足了局部测试的盲区

app2 的 [`runtimehost/acceptance_test.go`](../../../app2/runtime/runtimehost/acceptance_test.go) 直接从公共 HTTP surface
验证 normal Run、Question、Approval、Delegate、Plan、provider failure、Artifact round-trip 和 Session lifecycle；资源类
验收又覆盖 Skills、Recipes、Agent documents、Knowledge 和 Memory。

这层测试的价值不在数量，而在于它用产品语言回答“一个公开 Runtime 是否真的完成一条能力”。原版拥有远强于 app2
的领域、事务、故障、race、恢复和 architecture 测试，但顶层场景分散。反哺时可以新增少量、稳定、面向公共
`embedded`/HTTP 的 capability acceptance suite，作为原版细粒度测试的上盖；绝不能用它替换原版 86k+ 行的深层证据。

### 3.8 小而语义化的架构守卫更易长期维护

app2 的 [`architecture_test.go`](../../../app2/runtime/architecture_test.go) 主要守住四类长期边界：不导入旧 app、Agent
Framework 只能经 `agentexec`、Domain/Protocol 不向外泄漏、SQLite 只能由 composition/adapter 访问。这类规则与文件名、
局部变量或历史语法无关，维护成本低。

原版已经在 ADR-RT-063 和 P116 开始删除一次性语法守卫。app2 进一步证明，架构测试应保护 import DAG、唯一 owner、
公共 shape 和生命周期语义，而不是保护某次重构的源代码形状。后续只能在行为/边界测试已经覆盖同一风险时删除旧
AST guard，不能为减少测试文件而弱化真实门禁。

## 4. 明确不能从 app2 搬回原版的内容

| app2 现状 | 为什么不能迁移 |
|---|---|
| 没有 public `embedded` package | 文档承诺与实现漂移，并且丢失原版真实同进程 consumer 能力 |
| 生产树未发现 A2A、isolated workspace/sandbox、完整 OTel trace/metric/log composition | 体积下降的一部分来自能力未实现，不能计为熵回收 |
| Transcript 与 Conversation 主要保存 `Body []byte` | 把领域事实退化为 opaque JSON，弱于原版 typed aggregate 和跨投影闭包 |
| `application.Runtime` 有 90 个方法 | 用一个 facade 汇总全部 operation，重新形成 God object |
| `sqlite.Database` 有 122 个方法 | concrete composition 方便，但 persistence owner 过宽 |
| `runflow.Service` 有 59 个方法，`Store` 同时承担大量上下文 | 纵切内部再次失去用例边界和窄端口 |
| 21 个通用 `Service` struct | 命名没有表达真实职责，违背 app2 自己的标准 |
| 65 个公共非命令 package | implementation 被外部模块可见，远差于原版只公开 `protocol`/`embedded` |
| 27 个测试文件、4,697 行测试 | 无法承接原版的并发、故障、恢复、strict codec、race 和 fuzz 证据 |
| `runflow.locks` 只增长不删除 | 每个历史 Run 留下 mutex，生命周期 owner 不闭合 |
| 多处 `Close` 直接无界 `WaitGroup.Wait` | 外部依赖不协作时 shutdown 可永久挂起 |
| Runtime lifetime/收尾多处新建 `context.Background()` | 请求 trace values、owner lifetime 和 cancel/join 关系不够精确 |
| 目标架构承诺整体进入 `internal/`，实现却大面积留在 module root | 文档不能证明实现边界已经成立 |

这些问题共同说明：app2 的扁平化降低了目录深度，却没有稳定控制对象宽度、公共面和验证债。原版优化必须同时观察
“层数”和“单个 owner 的职责宽度”，不能只追求 package/LOC 下降。

## 5. 反哺原版的决策矩阵

| app2 经验 | 决策 | 迁回原版的形态 |
|---|---|---|
| 保留 Lyra Protocol 窄腰 | 保持 | 不改 wire；继续由 canonical registry 和 generator 驱动 |
| provider/model 配对 | 采用 | 原版复用 `modelref.Selection`，完整贯通 Session 与持久/公共 shape |
| 精确 Workspace value | 调整后采用 | 复用原版 path/isolation owner，不复制 app2 的简化规则 |
| capability vertical slice | 调整后采用 | 以真实 use case 重画边界，不引入 `*flow.Service` 模板 |
| consumer-owned interface | 采用 | 接口留在调用点；单实现且无边界价值时用 concrete type |
| construction acquisition ledger | 调整后采用 | 保留原版更强 shutdown 语义，只简化 acquisition/transfer 表达 |
| one-shot process descriptor | 调整后采用 | 只用于 Runtime/Desktop 监督边界，跨模块实施需单独授权 |
| public acceptance suite | 采用 | 作为原版深层测试的上盖，不替换原测试 |
| 小型语义 architecture guards | 采用 | 删除旧 guard 前必须证明同一风险已有稳定替代门禁 |
| app2 opaque JSON domain | 拒绝 | 原版继续使用 typed Transcript/Conversation/Run facts |
| app2 root-level public packages | 拒绝 | 公共 package 继续精确限制为 `protocol` 和 `embedded` |
| app2 Runtime/Store/Service God object | 拒绝 | 按用例、锁和生命周期拆解，不建立总 facade |
| app2 功能/测试删减 | 拒绝 | 能力和验证证据零损失是先决条件 |

## 6. 原版重构的证据化候选

以下是审计队列，不是已经授权的删除清单。每项在进入生产代码前仍要证明真实 consumer、动态入口、持久/公共义务和
失败窗口。

### C1：Session 模型身份闭合

- 信心：高。
- 风险：高；涉及 Domain、Protocol、Artifact、SQLite、Desktop 和生成物。
- 证据：原版 Session 只有 `model`，app2 已用完整 provider/model 反例闭合该歧义。
- 改造：让 Session 使用既有 `modelref.Selection`；create/update/fork/import/default admission 一次切换，不保留单 model
  alias 或兼容 reader。
- 代价：必须提升唯一 contract/storage shape，并同步所有消费者。
- 验收：两个 provider 发布同名 model 时，create、reload、fork、Run admission、export/import 都保持 exact pair；旧
  shape 确定性拒绝。

### C2：Session Workspace 值化

- 信心：中。
- 风险：高；原版还有 isolation、rollback、Git checkpoint 和 path ownership。
- 证据：app2 的 Workspace value 明显优于跨层裸字符串，但其能力集合不完整。
- 改造：先绘制 `cwd` 在 Domain、Application、Protocol、SQLite、workspace lease 和 artifact 中的 owner map，再决定值
  应归 `session` 还是现有稳定 path owner；不新建 shared `workspace` facade。
- 代价：需要处理 canonical spelling 与用户可见 spelling、symlink、fork/import 和 isolated workspace。
- 验收：同一 physical path alias、root/outside-root、fork/import、rollback/restore 和共享目录 lease 的现有行为全部保留。

### C3：按能力追踪并回收 forwarding 边界

- 信心：中。
- 风险：中到高；原版没有静态 dead code，剩余候选多为被真实调用的结构复杂度。
- 证据：原版有 24 个 Application、25 个 Adapter、21 个 Domain、17 个 Infra package 和 263 个接口；app2 证明部分能力
  可以用更短路径表达，但不能证明任一具体 package 已可删除。
- 改造：为 `runs.start/resume`、`sessions.snapshot`、provider/model、`runtime.subscribe`、`embedded.Open` 五条代表性纵切
  记录 operation → use case → consumer port → adapter → mechanism 的真实边，逐一识别只转发参数、只改名或只为测试存在的
  边界。一次只重构一条纵切，并在同批删除旧 owner。
- 代价：部分一实现接口仍承担依赖反转、故障注入或外部防腐，不能机械删除。
- 验收：行为和恢复矩阵不变；package/interface/依赖边只在有 owner 证明时减少；无新 facade、alias 或跨环 import。

### C4：Composition 与构造失败所有权收敛

- 信心：中高。
- 风险：高；启动恢复、后台任务和重复 Close 都在爆炸半径内。
- 证据：app2 `openGuard` 让 acquisition/transfer 一眼可见；原版已经拥有更强的 `hostLifetime`、逆序关闭和可重试 Close
  语义，但 composition capsule 仍值得以同一视角复核。
- 改造：建立一份只在代码中表达的 acquisition ledger；每个资源取得后立即登记，合法 capsule 完成后一次 transfer；下游
  只拿行为或窄 capability，不拿完整 Assembly。
- 代价：不能为了代码短而删除 deadline、join、失败保留和跨进程 setup/ownership 语义。
- 验收：对每个构造失败点注入错误，证明已取得资源逆序释放；Close timeout、重复 Close、部分失败重试和 goroutine leak
  测试继续成立。

### C5：接口面按真实消费者收窄

- 信心：中。
- 风险：中。
- 证据：app2 把接口数量从 263 降到 108，说明原版存在审计空间；app2 的胖 `Store` 又证明只减少接口数量并不等于边界
  变好。
- 改造：逐个记录 interface 的生产消费者、实现、测试替身和依赖反转价值。只有一个实现、一个消费点，且不隔离外部
  SDK/环边界/故障注入的接口改为 concrete；横跨多个 use case 的接口按消费点拆小。
- 代价：测试 seam 不能单独证明生产接口，但真实故障注入能力可能证明它。
- 验收：调用点更清晰、实现者集合准确、无 typed nil 或 mock-only production abstraction；既有测试不降级为内部字段窥探。

### C6：建立小型公共 capability acceptance 层

- 信心：高。
- 风险：低。
- 证据：app2 的公共场景测试可读性好；原版的深层测试证据更强但分散。
- 改造：在原版公共 `embedded` 与 HTTP conformance 上建立少量数据驱动场景，覆盖 normal、Question、Approval、Delegate、
  provider failure、Artifact round-trip、restart/replay。共享 scenario 定义，但不建立巨型万能 harness。
- 代价：会增加少量运行时间；必须避免重复已有低层断言。
- 验收：同一场景对两个 binding 得到等价产品事实，且失败能定位到具体能力而非整套黑盒。

### C7：Architecture guard 熵回收

- 信心：中。
- 风险：中；删错会失去高代价回归保护。
- 证据：app2 的小型 import guard 表达长期语义；原版 ADR-RT-063/P116 已认可删除历史语法守卫。
- 改造：把每条 guard 分类为 import boundary、唯一 owner、public/persistent shape、lifecycle invariant、历史 AST 形状。只有最后
  一类且无真实复发风险、或已被更强语义门禁覆盖时才删除。
- 代价：静态 guard 偶尔比行为测试便宜，不能只按行数删。
- 验收：人为加入同类回归 fixture 时，新门禁仍失败；删除 guard 后不需要把源代码固定在某个文件或变量名。

## 7. 建议实施顺序

本节不是 [`EXECUTION_PLAN.md`](EXECUTION_PLAN.md) 的新授权，只定义获得授权后的节奏。

### 阶段 A：冻结零损失基线

1. 复用当前 89 operation、3 sidecar、16 Runtime event、7 Run event 和 30 built-in Tool 的机器守卫；
2. 补齐 operation 计数之外的能力清单：public embedded、A2A、isolated workspace/sandbox、OTel trace/metric/log、MCP、LSP、
   recovery、Artifact、shared data-directory ownership；
3. 记录公共 package、package DAG、接口、直接依赖、goroutine owner 和五条代表性 capability trace；
4. 把 app2 acceptance scenario 转写为原版 characterization tests，但不修改生产行为。

### 阶段 B：先做低语义风险的结构减法

1. 审计并删除有证据的 forwarding wrapper、mock-only convenience API 和失去 owner 的 syntax guard；
2. 收敛 composition acquisition/transfer 表达；
3. 每批只改一条纵切，旧 owner 同批物理删除；
4. 不修改 Protocol、Artifact 或 SQLite，先证明结构方法可靠。

### 阶段 C：闭合 Session 身份

1. 独立处理 provider/model pair；
2. 另一个独立纵切处理 Workspace value；
3. 每个纵切一次性前移唯一 contract/storage shape，不建立 compatibility reader；
4. 同批更新 Desktop/CLI 等真实 consumer，或者在授权中明确拆分 consumer handoff，不能伪称整体完成。

### 阶段 D：按领域纵切继续收敛

用阶段 A 的 capability trace 选择收益最高、证据最完整的下一条路径。优先减少行为定位需要跨越的无语义边，避免按
目录层级或文件长度批量改名。任何新 package、interface、registry 或 owner 必须说明它切断的依赖或保护的不变量。

### 阶段 E：封板

重新运行完整 Runtime/Desktop build、vet、staticcheck、test、race、fuzz、contract/generator、public binding、restart/SIGKILL
和资源泄漏门禁。对比前后结构指标，但只有能力和证据零损失时，package/interface/LOC 减少才算成功。

## 8. 成功标准

原版重构成功必须同时满足：

- 用户能力零损失；operation 清单之外的 embedded、A2A、sandbox/isolation、OTel 和恢复能力也在账本内；
- 公共 Go package 继续只有 `protocol` 与 `embedded`；
- Agent Framework concrete import 继续只存在于 `adapter/agentexec`；
- Run、Session、Conversation、Transcript、Interrupt、Plan、Goal 继续是 typed domain/application facts，不退化为 opaque JSON bag；
- SQLite 继续是唯一 durable truth，跨聚合 write-set、命令回执和恢复语义不弱化；
- 每个 goroutine 和外部资源仍有 owner、cancel、deadline 与 join；
- 原版深层测试不因“app2 测试更少”而删除，并新增小型公共 acceptance 上盖；
- package、interface、依赖、代码行和能力追踪 hop 下降只作为结果，不作为驱动设计的配额；
- 每个删除项都有 consumer/dynamic/public/persistent/lifecycle 证据，每个保留项都能说明义务；
- 不产生 `app3`、长期双 Runtime、兼容转发层或新的大一统 `Service/Manager/Runtime` facade。

## 9. 最终判断

app2 留下的最好经验，不是“重写比重构简单”，而是相反：只有把成熟协议、完整能力、领域真相和恢复证据当作不可丢失
资产，删减实验才有意义。

app2 已经帮原版找到三个高价值方向：闭合 Session 的 provider/model 身份、把 Workspace 提升为精确值、用更短的领域
纵切和更清楚的 acquisition ledger 表达现有语义；它还提供了一个可读的公共 capability acceptance 模板。其余代码只应
作为反例或局部机制参考。

下一轮应在原版 `app/runtime` 内进行证据化、可回滚、逐纵切的治本重构：保留原版已经赢得的正确性，只删除无法证明
存在价值的结构复杂度。这样才能把 app2 的试验成本转化为原版的长期收益。
