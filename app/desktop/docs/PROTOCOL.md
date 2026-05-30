# PROTOCOL.md — 协议速查（一页）

> 简表。完整语义看 `API.md`,对接细节看 `INTEGRATION.md`。
> 状态:✅ 真后端已联通 · 🟡 前端已接待验 · ◻ 已定义未接 · ⚠️ 后端缺/不符

## HTTP 路由（transport）

| 路由 | 作用 |
| --- | --- |
| `POST /v1/rpc/{method}` | 所有 JSON-RPC 调用,body 是 envelope,method 在 URL 末段 |
| `GET /v1/rpc/stream` | SSE:服务端→客户端的通知流(fetch 流式读取,带 `Lyra-Connection-Id` 头) |
| `GET /v1/info` | 元数据(serverInfo + capabilities),握手前探活 |
| `GET /v1/health` | 健康检查 |

## JSON-RPC 方法

**runtime**
| method | 作用 | 状态 |
| --- | --- | --- |
| `runtime.initialize` | 握手:协商版本 + 能力 | ✅ |
| `runtime.shutdown` | 通知:关闭 | ◻ |
| `runtime.ping` | 保活 | ◻ |

**sessions**
| method | 作用 | 状态 |
| --- | --- | --- |
| `sessions.list` | 会话列表(分页) | ✅ |
| `sessions.create` | 新建会话 | ✅ |
| `sessions.get` | 单个会话 | ◻ |
| `sessions.update` | 改标题/pin/归档 | ◻ |
| `sessions.delete` | 删除 | ◻ |
| `sessions.fork` | 从某消息分叉 | ◻ |
| `sessions.export` | 导出 md/json | ◻ |

**messages**
| method | 作用 | 状态 |
| --- | --- | --- |
| `messages.list` | 历史消息(分页) | ◻ |
| `messages.edit` | 编辑消息 → 新 run | ◻ |

**runs**（核心）
| method | 作用 | 状态 |
| --- | --- | --- |
| `runs.start` | 发起一轮 run,流式返事件 | ✅ |
| `runs.cancel` | 停止进行中的 run | 🟡 |
| `runs.list` | 活跃/等待态 run(崩溃恢复) | ◻ |
| `runs.subscribe` | 重连认领已有 run 的流 | ◻ |
| `runs.approval.submit` | 提交 HITL 审批决策 | 🟡 |
| `runs.question.answer` | 提交澄清提问答案 | 🟡 |

**workspace**
| method | 作用 | 状态 |
| --- | --- | --- |
| `workspace.projects` | project 列表 | ✅ |
| `workspace.filesChanged` | 改动文件列表 | ✅ |
| `workspace.mcp.list` | MCP server 列表 | ✅ |
| `workspace.diff` | 单文件 diff | ◻ |
| `workspace.fileHead` | 文件头部预览 | ◻ |
| `workspace.grep` | 搜索 | ◻ |
| `workspace.skills` | skill 列表 | ◻ |
| `workspace.agentDocs` | AGENTS.md 正文 | ◻ |
| `workspace.mcp.tools` | 某 MCP server 的工具 | ◻ |
| `workspace.mcp.reconnect` | 重连 MCP server | ◻ |
| `workspace.selectProject` | 切换 active project | ◻ |

**providers / models / tools**
| method | 作用 | 状态 |
| --- | --- | --- |
| `providers.list` | provider 列表 | ⚠️ 返空 |
| `providers.test` | 测试 provider 连通 | ◻ |
| `providers.configure` | 配置 key/endpoint | ⚠️ -32601 未实现 |
| `models.list` | 模型列表 | ⚠️ 返空 |
| `tools.list` | 工具列表 | ◻ |
| `tools.invoke` | 不经 LLM 直接调工具 | ◻ |

**memory / attachments / background / feedback**
| method | 作用 | 状态 |
| --- | --- | --- |
| `memory.list` / `get` / `update` | LYRA.md 读写 | ◻ |
| `attachments.createUploadUrl` / `delete` | 上传附件 | ◻ |
| `background.list` / `stop` / `subscribe` | 后台任务 | ◻ |
| `feedback.submit` | 反馈(赞/踩/书签) | ◻ |

## 服务端→客户端通知（走 SSE）

| 通知 / 事件 | 作用 | 状态 |
| --- | --- | --- |
| `notifications/run/event` | RunEvent 信封,内含 AG-UI 事件 | ✅ |
| `notifications/run/closed` | RunResult:终态 + 用量 | ✅ |
| `notifications/terminal/output` | 终端输出流 | ◻ |
| `notifications/background/update` | 后台任务进度 | ◻ |
| `notifications/canceled` | 客户端→服务端:取消在飞 Request | 🟡 |

**AG-UI 标准事件**(在 run/event 里):`RUN_STARTED/_FINISHED/_ERROR` · `STEP_*` · `TEXT_MESSAGE_*` · `TOOL_CALL_*` · `REASONING_*` · `STATE_*` · `MESSAGES_SNAPSHOT` ✅

**CUSTOM 事件**(`event.name`):`lyra.approval` / `lyra.approval-result` / `lyra.question` / `lyra.question-result` / `lyra.plan` / `lyra.code-proposal` / `lyra.search-results` / `lyra.telemetry` — 🟡 前端就绪,⚠️ 后端命名不符(发的是 `plan_generated`/`compact_boundary`/`memory_updated`,见 INTEGRATION §8 BE-4)
