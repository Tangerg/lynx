# Frontend Plugin Context Architecture

> 本文记录 Lyra desktop 前端下一阶段的架构演进方向：在保持插件式架构的前提下，向整洁架构与 DDD 演进。
>
> 结论一句话：**Kernel as plugin platform, Plugin as bounded context, Clean Architecture inside plugin.**

## 1. 背景与判断

Lyra desktop 的第一性不是单一业务产品，而是插件化 AI agent 桌面平台。路由、布局、内容渲染、命令、快捷键、主题、设置面板、运行时事件折叠都已经由插件贡献；Kernel 应继续保持薄平台。

因此，前端业务不适合收进一个全局 `src/domain/`。一个全局 domain 会很快变成所有业务名词的收容所：agent 的 `Run` / `ToolCall`、workspace 的 `Diff` / `Terminal`、settings 的 `MCPServer` / `Schedule`、plugin platform 的 `ExtensionPoint` / `Contribution` 都会被塞到一起。短期看似分层，长期会让 Kernel/domain 变厚，插件退化成 UI 壳。

更合适的方向是把 **插件升级为限界上下文容器**：一个业务插件不只是 UI contribution，它可以拥有自己的 domain、application、adapters、presentation 和 ui。不是每个插件都需要完整五层；只有拥有业务不变量、状态机、用例或外部适配的插件才需要完整分层。

对比 `planet_new`：

- `planet_new` 是稳定单领域音乐播放器，统一的 `Track / Album / Artist / Playlist / PlayQueue` 领域语言非常明确，所以集中式 `domain <- core <- providers <- ui` 合理。
- Lyra desktop 是多上下文插件平台，agent、composer、workspace、settings、plugin platform 的语言不同，所以更适合按插件上下文组织业务。

## 2. 目标形态

推荐的业务插件内部结构：

```text
plugins/builtin/<context>/
  domain/
    model and invariants
  application/
    use cases and ports
  adapters/
    rpc, store, plugin-host, persistence bridges
  presentation/
    domain/application model -> UI view model
  ui/
    React components and contributed views
  contributions.ts
```

依赖方向：

```text
ui -> presentation -> application -> domain
adapters -> application ports
contributions -> ui/application public facade
```

禁止方向：

```text
domain -> React / Zustand / RPC / components
application -> components
ui -> rpc / main/container
plugin A -> plugin B internal files
```

允许共享的层：

- `components/common`：设计系统原子、Base UI 薄封装、纯展示 primitive。
- `plugins/sdk`：插件平台公共契约与 extension point。
- `rpc`：Runtime Protocol wire boundary，只表达外部协议。
- `state`：可以作为跨插件 read model 或 store bridge，但不应成为业务规则中心。
- `lib`：只放真正跨上下文的纯函数或薄 use-case facade；当某段逻辑只属于一个上下文，应逐步靠近对应插件。

## 3. 业务上下文划分

### Agent

核心语言：

- Session
- Run
- Message / Turn
- ToolCall
- Interrupt
- Plan
- Timeline
- Usage

职责：

- 折叠 runtime stream events。
- 维护 agent conversation/read model。
- 处理 send / steer / stop / resume / recover 等 use case。
- 暴露 agent input、conversation read model、tool/timeline view model。

方向：

```text
plugins/builtin/agent/
  domain/
  application/
  adapters/
  presentation/
```

业务侧通过 `plugins/builtin/agent/public/viewState.ts` 消费 agent 视图语言；SDK 的泛内容块扩展契约独立在 `plugins/sdk/types/contentBlock.ts`，不再与 agent 会话视图模型混放。

### Composer

核心语言：

- Draft
- Attachment
- Paste
- Mention
- SendIntent

职责：

- 管理用户正在编辑的输入意图。
- 校验输入是否可发送。
- 把 draft 投影成 agent input，而不是直接了解 runtime wire shape。

Composer 不应直接依赖 agent 内部 store。它可以依赖 agent 暴露的公开 input port。

### Workspace

核心语言：

- WorkspaceView
- DiffArtifact
- TerminalView
- FileFocus
- TodoList
- Diagnostic
- RunTimeline

职责：

- 把 agent/tool/timeline 等事实投影成 workspace 视图。
- 管理 view placement、active file、tool detail 路由。
- 消费 agent public read model，不读取 agent 内部实现。

### Settings / Configuration

核心语言：

- ProviderConfig
- MCPServer
- Schedule
- Hook
- ApprovalPolicy
- ThemePreference

职责：

- 把配置编辑、验证、保存、测试连接等用例从设置页 UI 中抽出。
- RPC shape 不泄漏到表单组件；表单组件消费 draft/view model。

### Plugin Platform

核心语言：

- ExtensionPoint
- Contribution
- Command
- Capability
- Activation
- PluginOrigin

职责：

- 保持 Kernel 薄平台。
- 提供插件间交互的公开契约。
- 不承载具体业务上下文的规则。

## 4. 跨上下文协作

业务协作可以是双向的，但代码依赖不能形成循环。上下文之间不要互相 import 内部文件；跨上下文动作通过 public port、command、read model、event 或 extension point 完成。

推荐形态：

```text
agent public surface <- orchestration/use case -> composer public surface
```

例如“选择 agent 某段文字插入 composer”：

```text
chat/message-actions.quoteSelectionToDraft(selection)
  -> AgentConversationPort.readSelection(selection)
  -> ComposerDraftPort.insertQuote(quote)
  -> ComposerDraftPort.focus()
```

例如“编辑上一条用户消息并回填输入框”：

```text
chat/message-actions.editMessageInComposer(messageId)
  -> AgentConversationPort.getMessage(messageId)
  -> ComposerDraftPort.replaceDraft(draft)
  -> ComposerDraftPort.focus()
```

例如“发送 composer 输入给 agent”：

```text
composer.submitDraft()
  -> build SendIntent
  -> AgentInputPort.send(input)
```

这里的 `chat/message-actions` application 层是跨上下文用例编排层，不是新的超级聚合根。它只编排多个 public port，不拥有 agent/composer 的内部状态。

### Commands, Events, Selectors, Extension Points

按语义选择交互机制：

- **Command / Port**：有明确目标和时序的用户意图，例如 `composer.insertQuote`、`agent.sendInput`、`agent.forkFromRun`。
- **Read model / Selector**：读取稳定展示状态，例如 agent running、current model capability、selected tool。
- **Event**：广播已经发生的事实，例如 `run.started`、`draft.changed`、`message.selected`。
- **Extension point**：插件贡献 UI 或行为，例如 composer toolbar item、message action、workspace view。

不要用 event 代替明确命令；不要让 UI 直接改另一个上下文的 store。

## 5. Public Surface 规则

一个上下文对外只暴露 public surface。其他上下文只能依赖 public surface，不能依赖内部目录。

示例：

```text
plugins/builtin/agent/public/
  input.ts
  conversation.ts
  workspace.ts
```

可能的 agent public ports：

```ts
export interface AgentInputPort {
  send(input: AgentInput): void;
  steer(input: AgentInput): void;
  canAccept(input: AgentInput): AgentInputReadiness;
}

export interface AgentConversationPort {
  getMessage(id: MessageId): MessageSnapshot | null;
  getSelection(selection: MessageSelection): ConversationQuote | null;
}
```

可能的 composer public ports：

```ts
export interface ComposerDraftPort {
  replaceDraft(draft: DraftInput): void;
  insertQuote(quote: DraftQuote): void;
  focus(): void;
}
```

这些类型应表达业务语言，而不是直接暴露 RPC `ContentBlock[]` 或 Zustand store shape。Runtime wire conversion 属于 agent adapter。

## 6. Store 的定位

Zustand store 不是业务层。它可以承担：

- read model cache
- UI preference
- per-session ephemeral state
- adapter bridge pinned state

它不应承担：

- 业务不变量的唯一表达
- 跨上下文用例编排
- RPC shape 到 UI shape 的长期映射

当 store 内出现复杂规则时，优先判断它属于哪个上下文的 domain/application/presentation，然后把规则移到对应层，store 只保存结果或提供 mutation bridge。

## 7. 迁移路线

### Phase 1: Agent context 试点

目标：让最厚的 agent 业务先形成清晰边界。

优先动作：

1. 把 agent view model 与 protocol wire 命名切开。
2. 将 `agent fold` 继续收敛为 agent application/fold。
3. 将 `messageRenderUnits`、`toolPresentation`、HITL presentation 靠近 agent context。
4. 将 `agentStore` 定位为 adapter/read model bridge，而不是业务规则中心。
5. 为 agent public input/conversation ports 建立最小契约。

验收标准：

- React message/tool/HITL UI 不直接理解 runtime wire shape。
- agent fold 规则可独立测试。
- composer/workspace 通过 agent public surface 协作。

### Phase 2: Composer context

目标：让输入框成为独立业务上下文，而不是 agent store 的 UI 附属。

优先动作：

1. 建模 Draft / Attachment / SendIntent。
2. 将 draft -> agent input 的转换从 UI 中抽出。
3. 通过 `AgentInputPort` 发送，不直接依赖 agent internals。
4. 暴露 `ComposerDraftPort` 给 message actions / orchestration use cases 使用。

验收标准：

- Composer UI 只管理编辑体验。
- Agent 不知道 composer UI。
- 编辑上一条、引用选区、fork 回填都走 orchestration + public ports。

### Phase 3: Workspace context

目标：workspace views 不再自己拼 agent/session/runtime 状态。

优先动作：

1. 收口 tool -> terminal/diff/timeline 的路由规则。
2. 为 terminal/diff/timeline 建 view model。
3. workspace 通过 agent public read model 消费工具和 timeline。

验收标准：

- Workspace view UI 不直接理解 agent internal state。
- Tool routing 是 workspace application/presentation 规则。

### Phase 4: Settings / Configuration context

目标：配置业务从设置页组件中脱离。

优先动作：

1. MCP server、schedule、hooks、provider config 建 draft/view model。
2. 表单组件只渲染 draft，不直接操作 RPC shape。
3. 保存/测试连接/校验走 application use case。

验收标准：

- 设置页组件不直接拼接 RPC request。
- 配置规则可独立测试。

### Phase 5: Layer guards

当边界稳定后，再加强静态约束：

- 禁止 context 内 `domain` import React / Zustand / RPC。
- 禁止 context 内 `application` import components。
- 禁止其他插件 import 某插件内部目录。
- 允许 import public surface。

不要先写复杂脚本再迁移；先让结构自然长出来，再把已经稳定的规则固化成检查。

## 8. 何时需要完整分层

需要完整分层的信号：

- 有独立业务不变量。
- 有状态机或生命周期。
- 有多个 UI 入口共用同一规则。
- 有外部适配器，例如 RPC、storage、plugin host、runtime stream。
- 同一规则已经在组件/store/lib 中重复出现。

不需要完整分层的情况：

- 纯 UI contribution。
- 只贡献一个静态 command / shortcut。
- 只是设计系统组件。
- 只有一个调用点、没有业务不变量。

这条很重要：**Plugin as bounded context 不是要求每个插件都造五层目录。**

## 9. 命名与组织原则

- 用业务名词命名模型，不用技术壳命名。
- 新 public surface 表达 published language，不泄漏内部 store 或 RPC wire。
- 不为迁移写兼容层；发现边界错了，在源头改正。
- 不为了“看起来 DDD”引入空接口或空目录。
- 先按上下文收口，再考虑移动目录；目录是边界的结果，不是边界本身。

## 10. 下一步推荐切口

> 状态（2026-07-03）：切口 1–5 已落地——agent 的 HITL/approval/question presentation 已分层（`agent/presentation/*Presentation.ts`）、composer public ports 就位、workspace tool routing 收口、settings 面板全部目录化（`index + ui/ + application`）、agent view model 拆成 facade + SDK 类型。导航模型重建首批已收口：`plugins/builtin/navigation` 已提供 Work Index read model、attention 投影、Work Index contribution surface 与 action wiring；`sidebar/` 退成 renderer；左侧 workspace/run destination 已迁入 Context Dock，Context Dock launcher/destination 打开意图也已收进 workspace application。

按收益和风险排序：

1. ✅ **HITL / Approval / Question**（已落地）：agent context 的 presentation 分层完成；settings 侧的 approvals 面板也已收敛为 `index.tsx` 注册 + `ui/`。
2. ✅ **Composer Draft / SendIntent**（已落地）：composer 与 agent 的 public port 协作已建立。
3. ✅ **Workspace tool routing / view model**（已落地）：tool → terminal/diff/timeline 的规则已从 UI/store 收口。
4. ✅ **Settings configuration drafts**（已落地）：MCP/schedule/hooks/provider/approvals/connection/usage/plugins form 已从 RPC shape 解耦、每个面板 `index + ui/ + application`。
5. ✅ **Agent view model 深拆**（已落地）：`agent/public/viewState.ts` 已是 ~30 行 facade，SDK 的内容块扩展契约与会话视图模型已分文件（`plugins/sdk/types/contentBlock.ts` + `plugins/sdk/types/agentView.ts`）。
6. ✅ **导航模型重建（首批已落地）**：`plugins/builtin/navigation` bounded context 已承接 project/session grouping、recent-session read model、attention 投影、action wiring（create/select/rename/fork/delete/favorite）与 Work Index contribution surface；`sidebar/` 只消费 `navigation/public` 发布语言并渲染 Work Index。
