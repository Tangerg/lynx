# Lyra AI Agent 客户端体验缺口 Review

日期：2026-05-26

范围：仅分析 `frontend/` 作为 AI agent 客户端的产品体验、交互闭环和前端能力缺口。服务端运行时当前是 mock，但真实 runtime 在另一个仓库开发，因此本文不把“后端能力未接入”本身算作问题；只评估前端是否已经准备好承载一个真实 agent runtime。

## 1. 总体判断

Lyra 前端已经不是普通 chat demo，而是一个接近 IDE 形态的 agent client：

- 有多会话 tab、消息流、composer、command palette。
- 有工具调用卡片、inline preview、workspace view。
- 有 HITL approval card、run status、stop、token/cost/status bar。
- 有 plugin registry、settings pane、plugins pane、notifications、diagnostics。
- 有 plan、reasoning、code、search、checkpoint 等内容块。

当前缺口主要不在“能不能把 agent 消息显示出来”，而在“用户能不能长期、放心、高效地和一个真实 agent 协作”。真实 agent 客户端的体验核心是：上下文可控、执行可追踪、改动可审计、失败可恢复、权限可理解、长任务可管理。

Lyra 现在的前端骨架已经对，但以下能力还不够完整：

- 会话和项目上下文还是偏占位，缺少真实工作区心智。
- 工具调用可见，但执行链路、风险、输入/输出 diff、重试和回滚还不够强。
- approval 只有单次 approve/decline，缺少策略化授权和风险解释。
- plan 能展示，但不能成为可操作的任务控制面。
- 长对话缺少上下文管理、压缩、检索和引用溯源视图。
- 错误只展示 banner，缺少 retry、resume、fork、report、diagnose 的恢复路径。
- 诊断面向开发者，用户级“agent 正在做什么/为什么卡住”还不够。

## 2. 用户旅程视角

### 2.1 开始一次任务

现状：

- Welcome screen 有建议 prompt。
- Composer 支持 mode、attachment source、toolbar slot、slash suggestions。
- Sidebar 有 sessions/projects 的信息结构。

缺口：

1. 缺少“工作区选择/确认”。

   对 coding agent 来说，用户发第一句话前最关心的是：当前 repo 是哪个、branch 是什么、是否有未提交改动、运行目录在哪里、agent 能访问哪些文件。当前 status bar 的 branch 还是 placeholder，composer footer 可扩展但没有形成清晰的 workspace summary。

2. 缺少任务模板和意图确认。

   Welcome prompt 是静态建议，但没有把常见任务流程产品化，例如：

   - review 当前 diff
   - 修一个 failing test
   - 实现 feature
   - 解释某个模块
   - 生成迁移计划

   这些任务的差异不只是 prompt 文案，还包括默认权限、上下文收集、是否允许改文件、是否需要先出 plan。

3. Composer mode 的语义不够可见。

   现在 mode 是插件注册出来的 label/icon，但用户很难知道 Agent / Ask / Plan 之间具体差异：会不会写文件、会不会跑命令、会不会需要审批、会不会自动执行。

建议：

- P0：在 composer/status 附近增加 workspace capsule：repo、branch、dirty state、cwd、runtime connection。
- P0：给 mode 增加说明 tooltip 和能力差异，例如 read-only / plan-only / execute。
- P1：把 welcome suggestions 升级成 task templates，每个 template 可设置 mode、默认附件、权限策略和起始 prompt。

### 2.2 观察 agent 执行

现状：

- Run status 显示 step/activity/token/cost/run id。
- Message stream 支持 reasoning、plan、tool card、checkpoint。
- Tool card 有 running/ok/err、args、duration、preview、open view。
- Tasks pill 可以展示插件任务。

缺口：

1. 工具调用缺少统一 timeline。

   Tool card 分散在消息流里，workspace view 有 Tools，但用户需要一个“本次 run 到底做了什么”的连续时间线：读了哪些文件、跑了哪些命令、改了哪些文件、失败在哪里、重试过几次。

2. tool args/result 的可读性和风险分级不够。

   当前 args 是一行 function signature，适合密集展示，但真实使用中需要：

   - 命令类工具突出 cwd、env、timeout、exit code。
   - 文件写入突出 path、diff、是否已落盘。
   - 网络/MCP 工具突出 endpoint/server、输入摘要、输出摘要。
   - 高风险工具有风险 badge。

3. running 状态只有“正在跑”，缺少卡住判断。

   真实 agent 经常会卡在命令、网络、长推理或等待权限。前端应能区分：

   - streaming 正常
   - long-running tool
   - waiting approval
   - runtime disconnected
   - no event heartbeat

建议：

- P0：增加 run timeline workspace view，按 run 聚合 reasoning、tool、approval、error、checkpoint。
- P0：定义 tool risk/status 展示规范，至少覆盖 command/file/network/MCP 四类。
- P1：增加 heartbeat/staleness UI：超过阈值无事件时显示“可能卡住”，并给 stop/retry/report 入口。

### 2.3 审批与安全感

现状：

- ApprovalCard 能展示 what/cmd/reason。
- 用户可 approve/decline。
- useApprovalSubmit 通过 gateway 提交，前端已经和 transport 解耦。

缺口：

1. 审批信息不够结构化。

   当前 approval 主要是 command 文本和 reason。真实 agent 需要更强的信任提示：

   - 影响范围：read/write/delete/network/shell。
   - 目标路径或命令。
   - 是否会修改工作区。
   - 是否可撤销。
   - 为什么需要这个动作。

2. 缺少授权策略。

   用户不应该每次都在相同低风险动作上被打断，也不应该对高风险动作只看到一个普通按钮。需要支持：

   - 仅本次允许。
   - 本会话允许同类动作。
   - 对某路径/命令前缀允许。
   - 始终询问。
   - 永不允许。

3. 缺少审批历史。

   用户需要回看自己批准过什么，尤其在任务出错或生成 diff 时。

建议：

- P0：扩展 approval block 的前端展示模型，支持 risk、scope、target、reversible、policy hint。
- P1：增加 Approval History workspace view，按 run/session 聚合。
- P1：支持 approval policy UI。前端可先设计策略选择面，不依赖后端实现细节。

### 2.4 查看和接受结果

现状：

- 有 DiffView、FilesChanged、Terminal、Plan、Tools workspace view。
- Tool preview 可以 open full view。
- Message actions 支持 copy/edit/regenerate。

缺口：

1. 缺少“最终结果审查”界面。

   对 coding agent，任务结束后用户要回答：

   - 改了哪些文件？
   - 为什么改？
   - 测试跑了什么？
   - 还有哪些风险？
   - 我要不要接受这些改动？

   当前这些信息分散在消息、tool card、workspace views 里，没有一个 final review surface。

2. diff 还不是操作中心。

   真实体验里 diff view 应支持 file tree、per-file summary、open in editor、copy path、stage/discard/revert hunk、关联工具调用和测试结果。当前更像展示面。

3. regenerate 语义偏弱。

   当前 regenerate 是找到前一个 user prompt 并重发。真实 agent 客户端更需要：

   - retry failed tool
   - resume from checkpoint
   - fork from message
   - edit-and-rerun from a specific point

建议：

- P0：增加 Run Summary / Final Review 视图，聚合 changed files、tests、approvals、errors、remaining risks。
- P1：DiffView 升级为 review surface：文件树、每文件摘要、open/copy path、关联 tool/run。
- P1：把 regenerate 拆成明确动作：Retry response、Fork from here、Edit prompt and rerun。

### 2.5 长对话和上下文管理

现状：

- status bar 有 token used/total/ctxPct/cost。
- session store 保留 tabIds，agentStore 按 session 保存 view state。
- Message stream 有 sticky bottom、jump-to-bottom、reasoning 折叠。

缺口：

1. 缺少上下文解释。

   用户看到 token 百分比，但不知道上下文里装了什么：哪些文件、哪些消息、哪些 tool result、哪些 attachments、哪些 memory。

2. 缺少上下文预算控制。

   当 ctxPct 变高时，用户需要可执行动作：

   - summarize conversation
   - drop old tool results
   - pin important files
   - remove attachment
   - fork clean session

3. 长消息流可能性能退化。

   当前有 rAF batching，但没有消息虚拟化。真实长任务会产生大量 markdown、tool results、diff、reasoning。前端需要在体验层准备折叠、虚拟化、分页或 archive。

建议：

- P0：增加 Context Inspector：展示当前上下文组成、token 占比、可移除项。
- P1：上下文高水位提示：超过 70/85/95% 给不同级别建议。
- P1：长会话消息虚拟化或分段卸载，至少对 tool result / reasoning 默认折叠并限制首屏渲染成本。

## 3. 横向能力缺口

### 3.1 会话管理还不够像生产 agent client

当前 tabIds 默认是 `s1/s2/s3`，会话 metadata 来自 data provider。真实产品需要：

- 新建、重命名、删除、归档会话。
- 会话搜索。
- 会话和项目绑定。
- 会话状态：running/waiting/error/done。
- 从历史会话恢复 runtime thread。
- 多会话并发时的资源和通知管理。

优先级：

- P0：新建/重命名/删除会话的前端交互。
- P1：会话搜索和项目过滤。
- P1：恢复/继续历史 thread 的 UI 状态。

### 3.2 文件和项目心智还不够强

现有 sidebar projects、files changed、diff、terminal 都有形状，但还缺一个清晰的 project model：

- 当前 repo/cwd/branch/dirty state。
- 文件树或最近打开文件。
- agent 已读/已改文件记录。
- 用户 pin 的上下文文件。
- workspace trust 状态。

建议把 Project/Workspace 作为前端一等概念，而不是几个 data provider 的松散集合。

### 3.3 错误恢复路径不足

当前 RunErrorBanner 可 dismiss，但真实用户需要“下一步怎么做”：

- Retry run。
- Resume from last checkpoint。
- Copy diagnostics。
- Open related tool call。
- Report plugin/runtime error。
- Switch runtime endpoint。

P0 建议：RunErrorBanner 增加 primary action，根据错误类型显示 Retry / Open diagnostics / Check connection。

### 3.4 通知系统偏内部

Notifications 现在能收 plugin feed，但用户需要按严重程度和任务关系理解通知：

- runtime disconnected
- approval waiting
- task finished
- plugin failed
- run failed
- file changed

建议通知按 run/session 归属，并和 status bar、timeline、workspace view 联动。否则通知会变成孤立日志。

### 3.5 插件体验还缺用户级治理

Plugins pane 能显示 loaded/error/reload，但真实 sideload 插件需要：

- capabilities 可视化。
- enabled/disabled。
- trust level / origin。
- version compatibility。
- error detail，不只是 “see browser console”。
- 插件贡献了哪些 surface：commands/views/tools/agui handlers。

这会直接影响用户信任。尤其 Lyra 的架构是“所有功能都是插件”，插件面板就是产品控制台的一部分，不只是开发调试页。

### 3.6 可访问性和键盘闭环需要补测试

已有 Radix/cmdk 是好基础，但需要从 agent client 高频操作验证：

- 全键盘完成：切会话、打开命令、发送、停止、展开工具、审批、打开 diff。
- 焦点恢复：关闭 palette/dialog/view 后回到原位置。
- screen reader：tool status、run status、approval risk、error banner。
- 高对比主题下的状态色可辨认。

这类问题会强烈影响重度用户体验。

## 4. 前端应优先补的 P0

### P0.1 Run Timeline

目标：让用户一眼知道本次 run 做过什么、正在做什么、卡在哪里。

最小范围：

- 事件类型：run start/end/error、reasoning、tool start/end、approval、checkpoint。
- 每条显示时间、状态、摘要、关联 message/tool id。
- 可从 status bar、tool card、error banner 跳转。

### P0.2 Workspace Context Capsule

目标：用户发消息前知道 agent 当前工作空间。

最小范围：

- repo/cwd
- branch
- dirty state
- runtime connection
- current mode 权限

可以先放在 composer footer/status bar，不需要大页面。

### P0.3 Run Summary / Final Review

目标：任务结束后给一个收口页面。

最小范围：

- changed files
- commands/tests run
- approvals
- errors/warnings
- remaining risks
- copy summary

### P0.4 Error Recovery Actions

目标：错误不是死胡同。

最小范围：

- Retry
- Stop
- Open timeline
- Open diagnostics
- Check connection

### P0.5 Approval Risk Model

目标：审批不只是 approve/decline，而是可理解的安全决策。

最小范围：

- risk level
- action category
- target
- scope
- reversible
- reason

## 5. P1：体验成熟度提升

1. Context Inspector：展示上下文组成和 token 占比。
2. Approval History：可回看本会话批准/拒绝过什么。
3. Diff Review Surface：文件树、per-file summary、关联 tool/test。
4. Plugin Control Panel：capabilities、enable/disable、error detail。
5. Long Session Performance：消息虚拟化、tool result 折叠策略、heavy markdown 延迟渲染。
6. Task Templates：把 welcome prompt 升级成可配置任务入口。
7. Session Search/Archive：让历史会话可管理。
8. Connection Health：runtime endpoint、latency、last event、reconnect 状态。

## 6. P2：高级 agent client 能力

1. Fork from message / branch conversation。
2. Checkpoint resume / rollback。
3. Multi-agent 或 subtask lane。
4. User memory / project memory 管理。
5. MCP/tool marketplace 级别的工具发现和权限管理。
6. Timeline export / shareable run report。
7. Cost budget 和 per-run spending guardrail。

## 7. 不建议优先做

- 不建议先做更复杂的视觉改版。当前设计语言够用，体验缺口主要是信息架构和操作闭环。
- 不建议把所有 workspace view 一次性做重。先补 run timeline 和 final review，比扩每个 view 更有效。
- 不建议先做多 agent。单 agent 的上下文、审批、错误恢复、结果审查还没有闭环。
- 不建议把 diagnostics 当用户级体验。Diagnostics 是开发者视图；用户需要的是 timeline、connection health 和 actionable recovery。
- 不建议用更多静态 welcome 文案替代 task templates。真实价值在模板背后的 mode、权限、上下文和后续视图。

## 8. 建议路线

第一阶段：把一次 run 讲清楚。

1. Run Timeline。
2. Error Recovery Actions。
3. Approval Risk Model。
4. Run Summary / Final Review。

第二阶段：把工作区和上下文讲清楚。

1. Workspace Context Capsule。
2. Context Inspector。
3. Connection Health。
4. Diff Review Surface。

第三阶段：把长期使用做顺。

1. Session Search/Archive。
2. Plugin Control Panel。
3. Long Session Performance。
4. Task Templates。

这条路线比继续堆插件 surface 更关键。真实 runtime 接上后，用户体验的瓶颈会从“有没有事件”迅速变成“用户是否理解 agent 在做什么、是否敢让它继续做、出了错是否知道怎么恢复”。
