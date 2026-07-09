# Second Batch Runtime Workflow

> 日期: 2026-07-09  
> 状态: done  
> 目标: 把第一批新增的工具原语收束成更可靠的 agent 工作流闭环。  
> 范围: `app/runtime` 应用侧为主, 必要时触及 `tools/fs` 的库原语边界。

## 目标

第二批不是继续堆新工具, 而是提升 runtime 的工作流可靠性:

1. 编辑链路闭环: `write/edit/multiedit/apply_patch -> autoformat -> diagnostics -> editguard refresh` 记录最终落盘状态。
2. 任务/计划能力增强: 让 todo/schedule 更像 runtime 的工作台能力, 状态和阻塞原因由 domain 表达。
3. Subagent 能力闭环: 明确子任务输入、输出、失败原因、父子关系和可观测事件。
4. 上下文检索层收敛: 在 runtime 内部提供轻量搜索策略, 组合本地搜索、语义索引、Sourcegraph 和精读文件。

## 非目标

- 不引入大而全的 agent 平台层。
- 不把 runtime 工具包装成框架式 registry 或 builder DSL。
- 不把库模块反向依赖 runtime 应用。
- 不为了未来可能性增加空 hook、空 interface 或配置爆炸。

## 完成判据

- 编辑后 editguard 记录的是自动格式化之后的最终文件内容。
- todo/schedule 的状态、阻塞原因和下一步动作有清晰 domain 表达, 工具只做薄适配。
- subagent 的 start/stop 事件包含可用的任务信息和终态摘要, 父进程生命周期不被子进程污染。
- 检索策略是小型内部编排, 不成为新的公共框架 API。
- `go test ./...`, `go vet ./...`, `golangci-lint run`, 重点包 `go test -race` 通过。

## 执行计划

1. 建立本文件并保持进度更新。
2. 审阅现状: 编辑链路、todo/schedule、subagent、检索工具。
3. 实现编辑链路最终状态刷新。
4. 增强任务/计划 domain 表达和工具输出。
5. 增强 subagent 事件 payload 与测试。
6. 增加轻量检索策略工具或内部组件。
7. 全量验证, 更新进度, 提交推送。

## 进度

| 项 | 状态 | 记录 |
|---|---|---|
| 文档 | done | 建立目标、非目标、计划、完成判据和执行记录 |
| 现状审阅 | done | 审阅 editguard、todo、schedule、subagent lifecycle、codebase/sourcegraph/grep 工具 |
| 编辑链路 | done | 增加 autoformat 后 editguard 刷新回归测试, 固化最终落盘内容被记录 |
| 任务/计划 | done | todo 增加 blocked_reason / next_action；schedule 将 Patch 和 NextRunAt 计算收进 domain entity |
| Subagent | done | 子进程创建事件携带输入绑定, 完成事件携带结果；runtime hooks 输出父子关系、任务摘要、终态和错误 |
| 检索策略 | done | 增加内部 `code_search` 工具, 组合语义索引、本地 literal grep、可选 Sourcegraph, 并给出 suggested_reads |
| 验证 | done | `go test ./...`, `go vet ./...`, `golangci-lint run`, 重点 `go test -race` 通过 |

## 执行记录

- 编辑链路: 增加 `TestEditGuard_RefreshesAfterAutoFormat`, 防止 JSON autoformat 后 read tracker 仍记录旧内容。
- todo: domain item 承载阻塞原因和下一步动作, completed item 禁止携带这些未完成态字段, protocol snapshot 同步暴露。
- schedule: 新增 `schedule.Patch` 与 `Schedule.ScheduledAfter`, 工具层只做请求到 domain 的翻译。
- subagent: child spawn 在创建时绑定输入, process completed 事件暴露最后结果, turn lifecycle hook 缓存 start 信息并在 stop 输出完整摘要。
- 检索: `code_search` 不取代底层 `grep` / `codebase_search` / `sourcegraph_search`, 只作为小型策略入口返回候选片段和建议精读文件。
- 验证命令:
  - `go test ./...` (`app/runtime`, `agent`)
  - `go vet ./...` (`app/runtime`, `agent`)
  - `golangci-lint run` (`app/runtime`, `agent`)
  - `go test -race ./runtime` (`agent`)
  - `go test -race ./internal/adapter/toolset ./internal/kernel/turn` (`app/runtime`)

## 设计纪律

- 应用侧遵循 Clean Architecture + DDD: domain 表达不变量, adapter/toolset 做薄翻译。
- 工具能力以组合为主, 不引入框架式统一平台。
- 接口只在真实消费方定义, 不为单实现制造占位接口。
- 错误用 `%w` 包装, 可观察事件走既有 hook/event/OTel 机制。
