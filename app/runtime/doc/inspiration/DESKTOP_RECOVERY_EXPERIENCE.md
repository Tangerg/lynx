# Desktop 恢复体验与 UI 精修里程碑建议

> 状态：P117 完成定义；授权、实施与验收状态由 Execution Plan 拥有。
>
> 适用范围：`app/desktop`、`app/desktop/frontend`；只有真实产品反例证明权威事实缺失时，才进入 `app/runtime`。`app/cli` 明确不在范围内。
>
> 本文只拥有下一阶段的候选目标、范围、批次和 Goal 专项编码规范。当前授权与进度由
> [`../EXECUTION_PLAN.md`](../EXECUTION_PLAN.md) 拥有，强制工程标准以
> [`../ENGINEERING_STANDARDS.md`](../ENGINEERING_STANDARDS.md) 为准。

## 1. 为什么旧提案不能继续沿用

旧提案把 Renderer replacement、Runtime 重启和恢复一致性作为待建能力，并试图建立统一的 Runtime generation owner。这个前提已经过时：P114 已完成真实恢复纵切，P116 又证明进程、connection 和 endpoint 都不是逻辑 Runtime 或 durable store identity。

下一阶段不得重新发明全局 Generation Owner，也不得把同一逻辑 Runtime 的进程重启描述为“旧服务端向后继服务端交接”。Renderer、Plugin Host、Runtime process、connection、query writer 和 material 可以在各自真实可替换的资源边界拥有局部 generation；SQLite durable fact、Session material 与 mutation identity 继续由现有 owner 决定。

旧分支名、提交号、测试数量、PID、端口和数据库路径是历史证据，不再复制到新 Goal。开始时直接读取当前 Git、Execution Plan 和 Capability Ledger。

## 2. 建议的新 Goal

**P117：Desktop 恢复反馈与 UI 精修闭环**

目标描述：

> 在一个 Desktop actor 对一个逻辑 Runtime 的产品形态下，从真实可见的红色反例出发，统一长对话、流式输出、审批 continuation、Session 切换、Dock 折叠与恢复、Renderer replacement 和 Runtime 进程恢复时的用户反馈。Run Summary、Terminal、Diff、Tool selection、Goal、Plan、HITL 与 Session navigation 必须消费现有权威 material 和 durable fact，不出现永久 loading、旧内容复活、跨 Session 拼接、稳定控件反复挂载或无法解释的状态跳变。在保持职责边界清晰的前提下完成 Codex 心智模型导向的交互和视觉精修。

这个 Goal 的默认工作层是 Desktop。只有红例能够证明“前端没有可消费的权威事实”，才允许修改 Runtime；不能因 UI 难写就增加 query、event、刷新命令或第二 read model writer。

## 3. 产品前提与非目标

必须接受以下前提：

- 产品只有一个 Desktop actor 和一个逻辑 Runtime；Runtime 可以经 loopback HTTP、socket 或同进程 binding 接入；
- binding 和进程 incarnation 只隔离它们实际拥有的进程内资源，不产生逻辑服务端世代；
- Desktop 与 CLI 打开同一目录是既有 SQLite、文件和 working-tree owner 的存储并发场景，不是多客户端产品模型；
- `sessions.snapshot`、durable mutation receipt 和既有局部 replacement owner 是当前基线，不在本 Goal 重做；
- Composer、Recipes 与 Workspace Events 的 Session 启动依赖继续由 composition root 显式声明，不退回 singleton、global accessor 或安装顺序依赖。

本 Goal 不包含：

- 全局 generation framework、server registry、transport factory 或多窗口选主；
- 为未来 socket、embedded 或远程部署预建 binding 矩阵；
- Mutation Journal identity、claimed Resume 补偿或历史静态守卫的二次重构；
- 兼容层、双写、刷新绕过、timer/debounce 竞态掩盖；
- `app/cli` 的任何修改或暂存。

## 4. 建议批次

### Batch 1：可见红例与参考证据

- 在真实 production Desktop 上记录 loading、审批 continuation、Run Summary、Terminal、Diff、Goal、Plan、Tool selection、Session/Dock 和流式消息操作区的当前行为；
- 每个问题必须包含用户动作、当前可见错误、权威事实、预期反馈和唯一 presentation owner；
- 优先选择单 Desktop 内可稳定复现的失败窗口，不堆砌无业务意义的并发、race 或 fuzz；
- 对照 Codex 主参考和 Zcode/Minimax 补充参考，逐项记录采纳机制与不采纳理由。

本批只建立证据和测试。未证明真实缺陷前，不修改生产 owner。

### Batch 2：恢复反馈的唯一 presentation owner

- 收敛 loading、recovering、waiting approval、failed、ready 和 terminal 的合法可见阶段；
- 稳定 footer、反馈按钮、Dock、toolbar 和 Run 操作区不得随 stream delta 反复卸载、重建或跳位；
- Session 切换、Renderer replacement 和 Runtime reconnect 后，只从当前挂载 Session 的权威 material 恢复；
- 若局部 UI state 需要跨 `await` 保存，绑定 exact entity 与局部 owner identity，并在切换、失败或 retirement 时确定释放。

只在多个状态确实共同保护一条生命周期不变量时建立对象或判别联合；不能把每个组件包装成 Owner、Coordinator 或状态机。

### Batch 3：页面心智模型与像素精修

- 统一 Run Summary、Terminal、Diff、Tool/审批卡、Goal、Plan、Session navigation 和 Dock 的信息层级、间距、反馈、空态、错误态与恢复态；
- 覆盖多轮长对话、compaction、HITL continuation、Session 切换、Dock 折叠/恢复、Renderer replacement 与 Runtime restart；
- Codex 作为主参考，优先复用其页面心智模型和交互反馈；视觉数值必须落入 Lynx 现有 token、组件和可访问性体系；
- 完成真实 production Wails 验收、必要的 fresh database/restart/SIGKILL 恢复验收和异步泄露检查。

## 5. 参考实现要求

### 5.1 前端主参考

- [`/Users/tangerg/Desktop/study/codex`](/Users/tangerg/Desktop/study/codex)：主参考，研究 loading、审批、恢复反馈、Run/Terminal/Diff、Session navigation、Dock 和长对话的页面心智模型；
- [`/Users/tangerg/Desktop/study/zcode`](/Users/tangerg/Desktop/study/zcode) 与 [`/Users/tangerg/Desktop/study/minimax`](/Users/tangerg/Desktop/study/minimax)：仅用于补充比较，不拼接多套设计语言。

像素级复刻不是复制源码。每个采纳点必须说明它在 Lynx 中由哪个组件、token 或 presentation owner 承担。

### 5.2 后端条件参考

- [`/Users/tangerg/Desktop/study/codex-server/codex-rs`](/Users/tangerg/Desktop/study/codex-server/codex-rs)：只有 UI 红例证明 Runtime 缺少权威事实或恢复语义时才研究；重点关注事件事实、恢复 cutpoint、事务和响应丢失处理；
- 不机械照搬 Codex 的多 connection、server history、内部状态或 Rust 包结构；
- 参考结果必须先翻译为 Lynx 的 Domain、Application、Adapter、Infra、Delivery 或 Desktop owner，再决定是否采纳。

## 6. Goal 专项编码规范

以下规范是 [`../ENGINEERING_STANDARDS.md`](../ENGINEERING_STANDARDS.md) 在本 Goal 的执行摘要。冲突时以后者为准。

### 6.1 所有权与对象模型

- 一个可见事实只有一个 authoritative writer；query、event、optimistic cache、material snapshot 和 component state 必须明确主从关系；
- concrete object 只有在同时拥有状态、行为、不变量或生命周期时才成立。优先把合法转换封装为对象行为，不让调用方跨文件拼接 `validate → mutate → cleanup`；
- `Owner`、`Generation`、`Coordinator`、`Manager`、`Service` 和状态机都需要真实反例证明。一个函数或直接对象能完整表达不变量时，不再包一层；
- 只有一个调用者且没有独立变化原因、资源生命周期、替换价值或防腐边界的 helper、file、package 或 interface，应吸回真实 owner；不按行数或“一概念一文件”拆分；
- 充血模型不等于把跨 aggregate 事务塞入 entity。Domain 保护自身不变量，Application 拥有跨 owner 用例与提交顺序。

### 6.2 React 与 TypeScript

- React component 只负责渲染和局部 presentation intent，不拥有 Runtime transport、跨 Session material、全局 mutation queue 或 renderer lifecycle；
- 使用判别联合表达真实可见阶段，禁止 boolean 组合制造非法状态；禁止用 `any`、`as never`、非空断言或缺失 dependency 绕过合同；
- 不在 render 中写状态，不用 effect 模拟 command，不从深层 UI import QueryClient singleton、container 或其他 context 的 private store；
- selector 返回稳定的最小投影。stream delta 只能更新流内容，不应重建稳定 footer、Dock、toolbar 或整页对象；
- 动效必须表达状态变化并服从现有 motion token、reduced-motion 和可访问性约束；不得用动画掩盖错误 owner 或迟到写入。

### 6.3 Go 与 Runtime

- 只有前端红例证明缺少权威事实时才修改 Go；先指出事实属于哪一层，不能从 Delivery 直读 Store 或把 transport DTO 泄露给 Application；
- constructor 返回 concrete type，required dependency 在构造边界一次验证；消费方只在真实边界定义 1–3 方法的窄接口；
- 接受接口、返回 struct；错误用 `%w` 保留 cause；I/O 和长任务首参数为 `context.Context`；
- 每个 goroutine 必须有明确启动者、cancel owner 和 join owner；channel 只有一个关闭者；测试不以 `time.Sleep` 等待并发结果；
- 进程实例、endpoint 和 binding 不是 durable identity。SQLite transaction、durable namespace 与已定义的 domain identity 决定恢复 winner；不得新增 endpoint alias、兼容双读或第二 recovery path。

### 6.4 异步与 replacement

- 只有异步间隙中 owner 可能真实替换，且后续会写入“当前实例”的共享状态时，才捕获并重新证明 exact owner；局部 workflow 自己拥有的状态不重复加 generation guard；
- cancellation 只负责尽快停止工作，不证明迟到 callback 无权提交。需要拒绝迟到写入时，由实际 commit owner 校验 exact identity；
- replacement 先发布 successor，再同步退休 predecessor。旧 disposer 只能清理自己拥有的资源；final close 不可逆，重复 dispose 共享 settlement；
- 禁止 sleep、debounce、延迟 invalidate、额外 refresh 或固定轮询次数掩盖排序问题。

### 6.5 持久化、测试与提交

- schema、wire 或 persisted shape 若必须 breaking，直接更新唯一 owner、调用方、fixture、生成物和文档；不保留 alias、migration 双读或 shadow state；
- 每个生产修复先提交能够失败的真实产品反例，再完成根因纵切；按风险选择最小充分测试，里程碑封板才运行 Frontend 全门禁、Wails production build、必要的 Runtime/SQLite/restart/SIGKILL 和异步泄露全集；
- 静态门禁只保护稳定依赖方向、public surface 或发生过的高代价回归，不冻结文件名、对象字面量或某次重构的语法形状；
- 每个独立可验证批次精确暂存、提交并推送。始终保留无关工作区改动，不修改或暂存 `app/cli`；
- 使用 agent-browser 后关闭本批创建的全部会话和 daemon，并确认没有遗留测试或 Runtime 进程；不得关闭用户已有的 Wails、Vite 或 Chrome 进程。

### 6.6 文档与注释

- 注释只解释当前代码无法直接表达的原因、不变量和失败语义，不记录迁移故事、旧实现考古或临时阶段；
- 阶段授权和里程碑进入 Execution Plan，当前能力和验收证据进入 Capability Ledger，编码标尺进入 Engineering Standards；
- 不在候选 Goal 中固化分支、提交号、测试数量、PID、端口或本机数据库路径。Git 和 owner 文档保存这些可变事实；
- 一个页面或文档只做一件事。标题描述内容，段落简短，使用主动语态，避免宽泛口号和重复结论。

## 7. 完成定义

只有以下条件全部成立，P117 才能封板：

1. 每个已纳入范围的 UI 红例都有唯一 presentation owner、失败测试和生产修复；
2. 流式输出、Session 切换、Dock 恢复、审批 continuation、Renderer replacement 和 Runtime restart 后没有永久 loading、旧内容复活、跨 Session 拼接或稳定控件闪烁；
3. Run Summary、Terminal、Diff、Tool selection、Goal、Plan 和审批反馈共享一致的信息层级与恢复心智模型；
4. 没有新增第二 read model writer、全局 generation、transport 矩阵、刷新旁路、兼容层或 timer 竞态掩盖；
5. 参考采纳点和拒绝理由可追溯，最终实现符合 Lynx 的领域边界和单 Desktop 产品约束；
6. 定向测试、Frontend 全门禁、Wails production build、必要的真实恢复验收和异步泄露检查全部通过；
7. `app/cli` 与用户无关工作区改动保持原样，自动化创建的进程和会话已清理；
8. Execution Plan、Capability Ledger 和必要 owner 文档已压缩更新，批次提交均已推送。

## 8. 新 Session 的启动顺序

用户正式创建 Goal 后，按以下顺序开始：

1. 阅读 [`../EXECUTION_PLAN.md`](../EXECUTION_PLAN.md)、[`../CAPABILITY_LEDGER.md`](../CAPABILITY_LEDGER.md)、[`../ENGINEERING_STANDARDS.md`](../ENGINEERING_STANDARDS.md)、本文和当前分支最近提交；
2. 在 Execution Plan 写入简短 P117 授权，不复制本提案全文；
3. 先复现一个真实可见红例，并记录 Codex/Zcode/Minimax 的对应证据；
4. 红例成立后再修改 production owner；未成立则记录 verdict 并停止该候选项；
5. 每批更新 owner 文档、精确提交并推送。除非用户叫停，持续到 P117 完成定义全部满足。
