# lynx-agent vs. Kimi Code — 架构对比

> 对照对象：moonshotai/kimi-code @ `/Users/tangerg/Desktop/kimi-code`。
> 对照范围：agent 框架 + agent 运行时 + transport / SDK surface。

Kimi Code 是 Moonshot 出的终端 AI 编码 agent，TypeScript monorepo。
lynx-agent 是 Go 框架，Lyra 是基于它的产品。两者形态差异巨大（framework
vs. product / Go vs. TS / 类型抽象 vs. 用例驱动），但目标域重合 — 都在做
"在终端跑一个能用工具的 AI"。本文按"对象 → 接口 → 调度 → 工具 → Plan /
Permission / Compaction / MCP / Transport / Subagent / 数据持久"维度对比，
拎出对方的设计亮点和我们这边的差距。

---

## 0. 模块拓扑速览

```
kimi-code (TS, pnpm monorepo)              lynx (Go, go.work multi-mod)
─────────────────────────────────          ─────────────────────────────
apps/kimi-code              TUI binary     lyra/cmd/lyra            CLI / TUI
apps/vis                    debug viewer   (无对应 — vis 没有)
packages/node-sdk           外部 SDK       (无对应 — Go 用 in-process API)
packages/agent-core         核心引擎       agent/                   框架
packages/kosong             LLM 抽象       core/model/chat          基础
packages/kaos               文件 / 进程    pkg + tools/{fs,bash}    基础
packages/oauth              OAuth          (无对应 — 各 provider 自己处理)
packages/telemetry          遥测           otel/                    OTel 桥
```

**关键观察**：
- kimi-code 的 `agent-core` 是个**重内核**：Agent / Session /
  Compaction / Permission / Plan / Records / Replay / Skill /
  Background / Injection / MCP 全收在一个包里，往 TUI 暴露 RPC。
- lynx 这边横向更宽（Embedding / RAG / VectorStore / Document /
  Models 40+），但 agent 框架本身 (`agent/`) 是"通用 Platform +
  Action + Goal + Planner"抽象，并不直接是一个能跑的 agent — Lyra
  才是。

---

## 1. 顶层对象与生命周期

| 维度 | Kimi `Agent` | lynx `core.Agent` + `lyra Runtime` |
|---|---|---|
| 顶层对象 | `Agent` 类，可独立 new（不需要 Session）| `core.Agent` 是声明定义（Actions / Goals），不持有状态；运行时实例是 `runtime.AgentProcess` |
| Session | 单独的 `Session` 对象，可 fork / export / resume | `core.Session` + `lyra session.Service`；fork 已在 Repo 层实装 |
| Sub-agent | `SessionSubagentHost` + `collaboration/agent.ts` tool | `agent/runtime/subagent.go` 支持 |
| 角色类型 | `AgentType = 'main' \| 'sub' \| 'independent'` | 没有显式枚举；隐含在调度上下文里 |
| 生命周期 | prompt → turn → step → tool → step.end → turn.end | RunAgent → tick → Action → ProcessContext → result |

**亮点（Kimi）**：Agent 必须能脱离 Session 单独使用（明文写在
`AGENTS.md`）— "构造函数不能强迫调用方先建 Session"。lynx 这边
`core.Agent` 是定义不是实例，无此问题；但 Lyra 的 `runtime.Runtime`
当前**捆绑了所有 service**，要看 Lyra 是否也需要这种"裸 agent"用法。

**差距**：lynx 没有 `AgentType` 这种"我是 main / sub / independent"
的自我认知字段。subagent 派发场景下可能需要补。

---

## 2. Loop 内核（Step / Turn / Tool 三层结构）

Kimi 的 `loop/` 单独抽出了"无状态 agent loop"：

```
run-turn.ts         turn 级收敛 / 中止 / 自动续跑
  └ turn-step.ts    一次 provider step：消息构造、原子 envelope、流式回调
      └ tool-call.ts        tool 批生命周期（分类纯函数、按 provider 顺序）
          └ tool-scheduler.ts  按资源访问图调度，不冲突可并行
```

并自定 `dispatchEvent` 单一事件出口，区分 `LoopRecordedEvent`（落到
transcript）和 `LoopLiveOnlyEvent`（只推 live 监听者），失败容错。

**lynx 对应**：
- `agent/runtime/run.go` `Tick` + `execute_action.go` + `dispatch.go` —
  逻辑相近但**没有同等层次切分**。"turn / step / tool-batch / scheduler"
  四层在 lynx 这边塌缩成 `executeAction` + 内部循环。
- chat 工具循环在 `core/model/chat/tool_middleware.go` 里递归实现 —
  和上层的 action 调度分两个世界，桥接靠 lynx 的 `ChatWithActionTools`。

**亮点（Kimi）**：
1. **资源访问图调度并行** — `tool-scheduler.ts` 让"读不冲突的工具
   同时跑、写冲突的串行"。lynx 工具循环是**严格顺序**的（chat tool
   middleware 一轮回应只对应一组 tool_calls，但每个 tool 仍是顺序
   invoke 的，参见 `core/model/chat/tool.go:invokeToolCalls`）。
2. **step envelope 原子性** — 一个 step 要么完整带 `step.begin` +
   `step.end`，要么允许"被中断只有 begin"作为故意的状态。lynx 这边
   没有显式 envelope。
3. **provider usage 立即记录**（chat 返回后就记，不等工具跑完）—
   即使工具被中止也保留 LLM 花费。lynx 的 `core.RecordLLMInvocation`
   存在但 chat middleware **没有主动调用**，所以 token 求和靠 Lyra
   action body 自己累加。

**对 lynx 的启示**：
- 考虑把 `agent/runtime/execute_action.go` 拆出 "turn-step / tool-batch /
  scheduler" 三层。
- 工具并行调度是个有意义的性能 + UX 升级，但要先有"资源访问声明"
  机制（read / write / search / spawn）。

---

## 3. 事件模型 / RPC surface

Kimi 在 `agent-core/src/rpc/events.ts` 里定义了 **34 个事件类型**
（一个 TS interface 一个）：

```
TurnStartedEvent / TurnEndedEvent / TurnStepStartedEvent /
TurnStepCompletedEvent / TurnStepRetryingEvent / TurnStepInterruptedEvent /
AssistantDeltaEvent / ThinkingDeltaEvent /
ToolCallDeltaEvent / ToolCallStartedEvent / ToolProgressEvent / ToolResultEvent /
SubagentSpawnedEvent / SubagentCompletedEvent / SubagentFailedEvent /
CompactionStartedEvent / CompactionBlockedEvent / ... /
SkillActivatedEvent / HookResultEvent / ...
```

并把 RPC surface 显式分三层：

- `AgentAPI` — 单 Agent 的方法 (28 个：prompt / steer / cancel /
  setThinking / setPermission / setModel / enterPlan / cancelPlan /
  beginCompaction / cancelCompaction / registerTool / unregisterTool /
  setActiveTools / stopBackground / clearContext / activateSkill /
  getContext / getConfig / getPermission / getPlan / getUsage / getTools /
  getBackground / ...)
- `SessionAPI extends AgentAPIWithId` — 多 Session 上下文 (rename /
  metadata / skills / mcpServers / reconnectMcpServer / generateAgentsMd)
- `CoreAPI extends SessionAPIWithId` — Core 全局 (getCoreInfo /
  getKimiConfig / setKimiConfig / createSession / closeSession /
  resumeSession / forkSession / listSessions / exportSession)

**lynx Lyra 对应**：
- `chat.Service`: StartTurn / Events / InjectSteering / ContinuePlan /
  Cancel（5 个方法 + 8 个事件类型）。
- `session.Service`: List / Get / Create / Fork / Delete。
- `approval.Service`: ListPending / Decide / SetMode / GetMode /
  Register。
- 全部加起来约 15 个方法 + 8 个事件类型。

**差距**：
- Lyra 没有 `setModel` / `setThinking` 这种**运行时切换 LLM** 的能力 —
  当前 chat client 是 engine.New 时固定的。
- Lyra 没有 `registerTool` / `unregisterTool` / `setActiveTools` —
  工具集是固定的离线 6 + 在线 3。
- Lyra 没有 `getBackground` / `stopBackground` — 后台任务（长跑工具）
  的概念不存在。
- Lyra 没有 `getContext` / `clearContext` — 客户端拿不到当前
  context memory 的快照，也不能主动清。
- Lyra 没有 `generateAgentsMd` — 自动生成项目级 `LYRA.md` 的工具
  接口（虽然有 auto extract）。

**亮点（Kimi）**：把 API 显式分 Agent / Session / Core 三层、每层都
有 `WithAgentId<>` / `WithSessionId<>` 包装 — 一个 Session 可以挂多个
Agent 是它的核心模型。lynx Lyra 当前是"一会话一 Agent" 一对一。

---

## 4. 工具体系

### Kimi 的内建工具集

```
file/        Read / Write / Edit / Grep / Glob / ReadMediaFile
shell/       Bash
web/         WebSearch / FetchUrl
planning/    EnterPlanMode / ExitPlanMode      ← 工具化的 plan 模式切换
collaboration/  Agent (subagent spawn) / AskUser / SkillTool
state/       (state 工具，外部状态管理)
```

每个工具有配套的 `.md` 文件 — **工具描述用 markdown 单独存**，TS 文件
只放代码 + 引用 `.md` 字符串。

**亮点**：
1. `EnterPlanMode` / `ExitPlanMode` **作为工具暴露给模型**（不是
   client 的开关）。模型自己决定何时切。Lyra 的 plan mode 是 client
   起 `--plan` 时静态开启的，模型并不"知道自己在 plan 模式"。
2. `AskUser` 工具 — 模型主动停下来问用户，得到结构化答案
   (`QuestionItem` / `QuestionAnswers` 类型化)。lynx 这边只有 HITL
   `awaitable`，没有"问用户的标准工具"。
3. `Agent` 工具是"spawn subagent" — 模型可以委派子任务给独立 context
   的子 agent。lynx `runtime/subagent.go` 有底层，但没暴露成 LLM 可见
   的工具。
4. `SkillTool` — 触发某个 skill（见 §6）。
5. 工具 description 单文件 `.md` —  描述是"内容"不是代码，迭代起来比
   写多行 string literal 顺手。lynx 这边描述硬编码在 `Definition.
   Description`，没有 docs/markdown 分离。

### lynx 的内建工具集

```
fs/      Read / Write / Edit / Glob / Grep
bash/    Bash
webfetch/   WebFetch (jina / firecrawl / exa / tavily 等 provider)
websearch/  WebSearch (同上)
httpreq/    任意 HTTP（白名单守门）
fakeweather/  示例
```

**差距**：
- 没有 plan / exit-plan 工具。
- 没有 ask-user 工具。
- 没有 spawn-subagent 工具（底层有，没暴露）。
- 没有 skill 工具。
- 没有 task / todo 管理工具（claude code 那种 `TodoWrite`）。
- 没有 read-media（图片 / PDF）工具。

---

## 5. Permission / Approval 模型

Kimi 的 `agent/permission/`：

- **模式**：`'manual' | 'yolo' | 'auto'`
- **规则系统**：`PermissionRule` + `PermissionRuleScope =
  'turn-override' | 'session-runtime' | 'project' | 'user'`
- **匹配**：`parse-pattern.ts` + `path-glob-match.ts` + `matches-rule.ts` —
  可写 glob 风格的规则
- **策略链**：`policies/`:
  - `default-git-cwd-write.ts` — 默认 yolo 模式只允许 git cwd 内写
  - `yolo-workspace-access.ts` — 工作区外 read / write / grep 仍需问
  - `ask-user-question.ts` — askUser 工具的特殊策略
  - `plan.ts` — plan 模式下的工具白名单
- **决策**：`'allow' | 'deny' | 'ask'`，由 `check-rules.ts` 按规则 +
  policy 链综合判定

lynx Lyra 的 `service/approval/`：

- **模式**：`Safe / Balanced / Yolo`
- **规则系统**：**无** — 写死的 (tool name → SafetyClass) +
  (Mode → 是否 gate) 矩阵 (`service/chat/policy.go`)
- **匹配**：无 glob，只看 tool name
- **策略链**：无
- **决策**：`AllowOnce / AllowAlways / Deny`（**注意 AllowAlways 还
  没真做缓存**）

**差距很大**：
- Kimi 的 yolo 在 git cwd 外仍会问，**这是真实安全的 yolo**；Lyra 的
  yolo 是"全放"，等价于"不防守"。
- Kimi 的 turn-override scope 让 "这次同意 + 下次仍要问" 成为一类
  正交 scope；lynx 没有 scope 概念。
- Kimi 的 glob 规则可以写 `Write[<workspace>/!docs/**]` 这种 — 
  Lyra 的工具白名单只能粒度到工具名。
- Kimi 的 `permission/policies/plan.ts` 在 plan 模式下自动收紧 —
  模型进入 plan 时只能用 read / search 工具。Lyra plan 模式只是
  "出 plan 让用户审，审了再跑"，没有"plan 模式下工具集自动变化"
  这层。

**对 lynx 的启示**：
- `AllowAlways` 必须配合 (tool, normalized-args) 缓存 — Kimi 的
  rule 系统等价于"模式 + glob 规则 + scope" 三维。
- 加 PermissionRule 抽象 + glob 匹配 + scope。
- Plan 模式下的工具白名单是个有意义的"安全护栏"。

---

## 6. Skill（lynx 没有此抽象）

Kimi 的 `agent-core/src/skill/`：

- **定义**：`SkillDefinition` = name + description + path + dir +
  content + metadata + 源 (`SkillSource = 'project' | 'user' |
  'extra' | 'builtin'`)
- **注册**：`scanner.ts` 扫描目录，`parser.ts` 解析 metadata header，
  `registry.ts` 索引
- **激活**：`activateSkill(name)` RPC method
- **内建**：`mcp-config` skill — 用对话式配置 MCP server
- **触发**：模型用 `SkillTool` 工具激活某个 skill；激活后 skill 内容
  注入 system prompt 或 tool description
- **测试用**：metadata `disableModelInvocation` / `safe` / `whenToUse`
  等字段控制何时激活

**lynx 当前**：没有 Skill 抽象。LYRA.md 是"持久化记忆"层面，不是"按需
激活的能力包"。

**差距**：Skill 是 Claude Code / Kimi Code 都很重视的"扩展点"，远比
"硬编码工具"更轻量。如果 lynx 也想做生态扩展，这是个关键缺口。一个
最小可用的 Skill 设计：
- 文件系统约定：`<workspace>/.lyra/skills/<name>/SKILL.md`
- metadata frontmatter (name / description / when_to_use / safe)
- 模型通过 SkillTool 激活 — 激活后内容注入下一轮 system prompt
- skill 可以注册自己的工具（带 schema 的命名子例程）

---

## 7. Compaction

| 维度 | Kimi | lynx Lyra |
|---|---|---|
| 触发 | `beginCompaction` RPC + auto | Auto 阈值（24 条消息） |
| 策略 | `FullCompaction` 等可换 | 单一策略 |
| 暂停 | 有 `CompactionBlockedEvent` | 无 |
| 取消 | `cancelCompaction` | 无（同步执行） |
| 模板 | `compaction-instruction.md` 单文件 | 硬编码 prompt |
| 结果 | `{summary, compactedCount, tokensBefore, tokensAfter}` | bool + err |
| 客户端反馈 | `CompactionStarted/Finished/Blocked` 事件 | 无事件，写完就过 |

**亮点（Kimi）**：把 compaction 当成**一等公民**：模型可以告诉客户端
"我在 compact 了"，客户端 UI 能显示，能取消，能看 before/after token
节省。lynx 的 compaction 是"无声后台清理"。

---

## 8. Plan 模式

| 维度 | Kimi | lynx Lyra |
|---|---|---|
| 触发 | 模型用 `EnterPlanMode` 工具切入 | client `--plan` flag 强制 |
| 退出 | 模型用 `ExitPlanMode` 工具退出 | 用户 Approve/Reject |
| 工具白名单 | plan 模式下 policy 自动限制为只读 | 无限制 |
| 状态 | `getPlan()` / `clearPlan()` RPC | 无服务端 plan 状态 |
| 显式中止 | `cancelPlan` | 走通用 `Cancel` |

**亮点（Kimi）**：plan 是**模型驱动的状态切换**，不是 client 开关。
模型决定 "现在该停下来想清楚"。这意味着 Lyra 的 `--plan` 标志是
"我们要不要让模型想"，而 Kimi 是"模型自己说我要想了"。

**对 lynx 的启示**：
- Plan 工具化 — 让模型自主进入 plan 模式。
- Plan 模式 + 工具白名单联动。
- Plan 状态服务端持有，多客户端可 query。

---

## 9. MCP

Kimi 的 `agent-core/src/mcp/`：

- 客户端三传输：`client-stdio.ts` / `client-http.ts` / `client-shared.ts`
- 连接管理：`connection-manager.ts` — 多 server 的连接生命周期
- OAuth：`oauth/` 完整 OAuth 流程
- 工具命名去冲突：`tool-naming.ts`
- 配置：`config-loader.ts` + `session-config.ts`
- 一个**对话式配置工具**：`mcp-config` skill + `auth-tool.ts` —
  用户说"接个 github mcp"，AI 自己改配置 + 触发 OAuth

**lynx Lyra**：
- 只有 HTTP Streamable transport（`lyra/internal/engine/mcp.go`）
- 没有 stdio 子进程支持
- 没有 OAuth 流程
- 命名前缀写死 `<sourceName>_<toolName>`，没有冲突解决策略
- 没有对话式配置

**差距非常大**。MCP 生态主流是 stdio 子进程（npx -y @modelcontextprotocol/
server-github 之类）。Lyra 当前只能接已经跑起来的 HTTP MCP server，
覆盖面窄。

**对 lynx 的启示**：
1. 立刻补 stdio MCP transport — lynx SDK 已有 `sdkmcp.CommandTransport`，
   一层包装的事。
2. OAuth 流程可以晚做（多数本地用法不需要），但应该有口子。
3. 对话式配置 MCP 是个产品级 UX 加分项。

---

## 10. Transport / 进程边界

| 维度 | Kimi | lynx Lyra |
|---|---|---|
| 核心-客户端边界 | RPC over IPC | HTTP+SSE / stdio JSON-RPC |
| RPC 库 | 自有（`rpc/client.ts` / `core-impl.ts`）| 自有 JSON-RPC 行协议 + REST |
| 客户端实现 | `node-sdk` (TS) | 无独立 SDK；HTTP 走 AG-UI 协议 |
| 协议形态 | `RPCMethods<API>` 类型派生 | 手写路由表 |
| 多 Agent 同 Session | 是（API 显式三层） | 否（一会话一 Agent） |
| 协议文档 | 类型即文档 | 待补 proto IDL |

**亮点（Kimi）**：
- `RPCMethods<API>` 把 TS 接口自动派生 RPC 方法名 — 类型即协议契约。
  Lyra 走 cobra + handler 手写，多一层维护。
- `node-sdk` 把 RPC 封成 `KimiHarness` + `Session` 两个友好类，外部
  应用直接 `import` 就用。lynx 没有"给外部 Go 应用集成的 SDK"。

**对 lynx 的启示**：
- 写 IDL（proto）— ARCHITECTURE.md 也提了这是 v0.2 要做的事。
- 多 Agent 共享 Session 是个值得评估的模型 — 一个 session 内多
  agent 协作（main + sub）共享 history，但调度独立。

---

## 11. 其他维度

### 11.1 Records / Replay（事件溯源）

Kimi 的 `agent/records/` + `agent/replay/`：每个 agent 行为都是
`AgentRecord` 事件（含 type + timestamp + payload），持久化到
`FileSystemAgentRecordPersistence`。`ReplayBuilder` 可以从 records
重放出整个 agent 状态。

**lynx Lyra**：session metadata + JSONL message store + LYRA.md。
**没有"事件级溯源"** — chat events 当成事件流播给客户端，但不持久。
要回看历史只能看 message store。

**差距**：审计 / 回放对 enterprise 场景很关键。

### 11.2 Background tasks

Kimi 的 `agent/background/` + `tools/background/`：长跑工具（持续运行
的 server / watch 进程等）作为后台任务，有独立 ID，可以 stop / 查
output / 查 output path / 列出当前所有 background tasks。

**lynx Lyra**：无。`bash` 工具是同步阻塞执行。

**对 lynx 的启示**：bash 工具加 background 模式（spawn → 返回 id，
后续 `bash-status` / `bash-stop` / `bash-output` 工具操作）。这对
"启动开发服务器 / 跑测试 watch" 类任务很有用。

### 11.3 Hooks（生命周期钩子）

Kimi 的 `agent/hooks/`：
- `engine.ts` + `runner.ts` — hook 引擎
- `user-prompt.ts` — 用户 prompt 提交时的钩子
- 用例：审计 / 拦截 / 桌面通知 / 集成 CI

**lynx 当前**：`agent/core/hooks.go` 有平台级 hook 抽象，但 Lyra 没
配置成用户可挂的扩展点。

### 11.4 Context Memory

Kimi 的 `agent/context/`：
- `projector.ts` — 把当前状态投射成"模型可见的上下文"
- `complete-slice.ts` — context 切片
- `notification-xml.ts` — 给模型的"系统通知" XML 格式
- `clearContext` RPC — 显式清空

**lynx Lyra**：仅有 system prompt 拼接（LYRA.md 级联 + base
prompt），没有"context projector"这种独立抽象，更没有 clearContext。

### 11.5 Telemetry

Kimi 的 `telemetry/`：自有的客户端 telemetry，独立包，可由
`KimiHarness` 注入。每个组件都收 `TelemetryClient`，无侵入。

**lynx**：OTel 桥在 `otel/`，但 agent 内部 telemetry 多数靠 OTel
span。

---

## 12. 总结对照表

| 能力 | Kimi | lynx (agent + Lyra) | 差距评级 |
|---|---|---|---|
| 顶层 Agent 抽象 | ✅ 类 + 三种角色 | ✅ Action/Goal 但无角色枚举 | 小 |
| Loop 三层分离 (turn/step/tool-batch) | ✅ | 部分（一层） | 中 |
| 工具并行调度 | ✅ 资源访问图 | ❌ 严格顺序 | 中 |
| Step envelope | ✅ | ❌ | 小 |
| 显式 Usage 立即记录 | ✅ | ❌（chat middleware 没调用 Record） | 中 |
| Plan 工具化 (EnterPlanMode) | ✅ | ❌ (client flag) | 中 |
| AskUser 工具 | ✅ 结构化 | ❌ (有 HITL 但无 tool) | 中 |
| Subagent 工具暴露 | ✅ | ⚠️ 底层有，工具没暴露 | 中 |
| Skill 系统 | ✅ 完整 | ❌ | 大 |
| Background tasks | ✅ | ❌ | 中 |
| Permission rule + glob | ✅ | ❌ 写死矩阵 | 大 |
| Permission scope | ✅ 四级 | ❌ | 中 |
| Plan 模式 + 工具白名单 | ✅ | ❌ | 中 |
| Compaction 状态化 + 客户端反馈 | ✅ | ⚠️ 静默 | 小 |
| Compaction 可取消 | ✅ | ❌ | 小 |
| Records / Replay | ✅ 事件溯源 | ❌ | 大 |
| Hook 用户可挂 | ✅ | ⚠️ 底层有，未暴露 | 中 |
| Context projector | ✅ | ❌ | 中 |
| ClearContext | ✅ | ❌ | 小 |
| MCP stdio | ✅ | ❌ 只 HTTP | 大 |
| MCP OAuth | ✅ | ❌ | 中 |
| 对话式 MCP 配置 | ✅ skill | ❌ | 小 |
| 运行时切 model / thinking | ✅ | ❌ | 中 |
| 运行时注册 / 卸载工具 | ✅ | ❌ | 中 |
| 多 transport (HTTP + IPC + ...) | ✅ RPC | ✅ HTTP + stdio | 持平 |
| 外部 SDK | ✅ node-sdk | ❌ 无独立 SDK | 中 |
| IDL / proto | ✅ TS 类型派生 | ❌ 待补 | 中 |
| AG-UI 兼容 | ❌ (自有协议) | ✅ | 持平（互补） |

---

## 13. 给 lynx + Lyra 的优先级建议

按 ROI 排序，可以独立做、对产品力提升最大的几项：

**P0（产品级硬伤）**
1. **MCP stdio transport** — 不补这个，Lyra 接不上主流 MCP 生态。
2. **Permission rule + scope + plan-mode 白名单** — 现在的 mode 矩阵
   太粗，AllowAlways 还没真做缓存。

**P1（产品深度，1-2 周量级）**
3. **Plan 工具化**（EnterPlanMode / ExitPlanMode）— 让模型自主进入
   plan 模式。
4. **AskUser 工具** — 模型问用户的标准入口。
5. **Skill 系统最小版** — 文件约定 + scanner + activate RPC。
6. **Subagent 工具暴露** — 底层已有，包成 `agent` 工具。
7. **Bash background mode** — long-running 任务支持。

**P2（生态 / 工程化，月级别）**
8. **Records / Replay** — 事件溯源 + 重放。审计场景关键。
9. **Tool 并行调度 + 资源访问图** — 性能 + UX 升级。
10. **运行时 setModel / setThinking / registerTool** — 让客户端能在
    会话中切换 LLM、增删工具。
11. **proto IDL** — 把 service 接口生成 proto，自动派生 HTTP / gRPC /
    SDK 客户端。

**P3（差距小，按需做）**
12. **Step envelope** — 调度 trace 更结构化。
13. **Context projector + clearContext** — UI 能看上下文也能清。
14. **Compaction 事件化 + 可取消** — UX 加分。

---

## 14. lynx 这边 Kimi 没有的优势

- **横向更宽**：embedding / RAG / vectorstore / document / 40+ model
  provider — Kimi 全 in-house，lynx 是通用基础。
- **agent 框架抽象更彻底**：Platform / Action / Goal / Planner / Workflow
  是真正的"框架级"抽象，Kimi 的 agent-core 更像"一个完整 agent 的
  实现"。
- **AG-UI 兼容**：Lyra HTTP+SSE 是 AG-UI 协议，可以直接被 AG-UI 生态
  客户端消费。Kimi 是自有 RPC 协议，互通成本高。
- **多 LLM provider 一等公民**：lynx 一开始就支持 40+ provider，
  Kimi 主要给 Moonshot/Kimi 模型用，其他 provider 是兼容补充。
- **Go 二进制部署**：单文件 binary 比 Node.js 部署轻得多。

---

## 15. 实现风格 / 工程化对比

| 维度 | Kimi | lynx |
|---|---|---|
| 语言 | TypeScript | Go |
| Monorepo 工具 | pnpm workspaces | go.work |
| 测试 | vitest | 标准 testing |
| 构建 | tsdown | go build |
| Telemetry | 独立 package | OTel 桥 |
| 文档形态 | TS 类型即文档 + 工具 .md 单文件 | doc/*.md + godoc |
| Skill 文档 | metadata frontmatter | 无 |
| 规范文档 | AGENTS.md 多层级 | CLAUDE.md / 单层 doc/ |

**值得借鉴**：
- **工具 description 用 .md 单文件存** — `bash.md` / `edit.md` 等。
  迭代描述不动代码，PR diff 干净。
- **AGENTS.md 分层** — 根级硬规则 + 子目录局部规则。lynx 这边 CLAUDE.md
  只有一份。

---

*版本：v1，对照 kimi-code @ 2026-05-23。下次对照建议关注：
Background tasks 实际产品形态、Skill 生态成熟度、permission rule 是否
完成 AllowAlways 缓存。*
