# app2

`app2` 是 `app` 的绿地替代实现。它不是兼容层、渐进重构目录或第二套长期实现。

当前阶段：**R1 已完成，进入 R2**。Runtime/Desktop 的可执行骨架、合同生成、进程监督、
`runtime.discover` 直连纵切和生产打包链已经闭环；后续严格按能力账本推进 Workspace + Session。

## 目标

- 保留当前 Runtime 与 Desktop 的全部已证明产品能力；
- 不兼容旧 Go API、SQLite、Artifact、前端 published surface 或目录写法；当前 Lyra wire contract 因其
  设计质量被主动选作 app2 基线，不通过兼容 adapter 维持；
- 用领域边界组织代码，以依赖方向实现整洁架构，不复制“分层目录仪式”；
- Runtime 保持独立可监督进程，并以当前 Lyra Runtime Protocol 作为合同基线；
- 以静态限界上下文组合替代没有生产动态安装入口的通用插件内核；
- 完成等价验收后一次切换，随后删除旧 Runtime/Desktop 实现；
- Runtime/Desktop 完成后，继续迁移 `app` 的其余消费者，直到 `app2` 成为唯一实现。

## 文档地图

| 文档 | 唯一职责 |
| --- | --- |
| [`AGENTS.md`](AGENTS.md) | app2 的规范性工程规则 |
| [`doc/REFERENCE_STUDY.md`](doc/REFERENCE_STUDY.md) | 当前系统、Codex、Dougong、Oolong 的证据与取舍 |
| [`doc/DOMAIN_MODEL.md`](doc/DOMAIN_MODEL.md) | 统一语言、上下文、聚合与业务不变量 |
| [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) | 目标拓扑、依赖、所有权和目录语法 |
| [`doc/CONTRACT.md`](doc/CONTRACT.md) | app2 协议、传输、生成与存储合同 |
| [`doc/DECISIONS.md`](doc/DECISIONS.md) | 已接受且可追溯的架构决策 |
| [`doc/CAPABILITY_LEDGER.md`](doc/CAPABILITY_LEDGER.md) | 全功能迁移与验收状态的唯一账本 |
| [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md) | 阶段、顺序、门禁、切换与资源回收 |

文档之间不得复制所有权。架构事实只在 `ARCHITECTURE.md` 定义，业务语义只在
`DOMAIN_MODEL.md` 定义，合同形状只在生成源和 `CONTRACT.md` 定义，迁移状态只在
`CAPABILITY_LEDGER.md` 定义。

## 当前阶段入口

R2 只从以下已验证基础继续，不重新发明协议或进程拓扑：

1. `runtime.discover` 是 app2 registry 中唯一已迁入的 operation，另外 88 项仍不得用 placeholder 冒充；
2. `/v2/info`、live、ready、one-shot descriptor、token 与 generation identity 已验证；
3. Desktop 只通过生成的 Lyra client 访问 Runtime，Wails 只保留 native/bootstrap surface；
4. SQLite epoch 1 是 app2 durable truth，R2 在其上增加 Workspace/Session schema 与事务；
5. 迁移状态与 R1 证据以 `CAPABILITY_LEDGER.md` 和 `EXECUTION_PLAN.md` 为准。
