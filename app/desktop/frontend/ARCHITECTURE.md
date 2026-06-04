# Lyra 前端架构

> 本文档描述 `frontend/` 这个 React + TypeScript 应用是怎么组织、怎么运行的。
> 设计系统 / 视觉规范看 `DESIGN.md`；决策透镜 / 工程约定看仓库根的 `CLAUDE.md`；
> 协议权威定义看 `docs/API.md` + `docs/TRANSPORT.md`；扩展点底座看 `docs/EXTENSION_POINTS.md`。
>
> **分工**：`CLAUDE.md` 讲"怎么判断"（决策与硬约定），本文讲"系统长什么样"（结构与运行）。两者尽量不重述。

---

## 1. 一句话概括

**Lyra 前端 = 自研 Lyra Runtime Protocol v2 流式协议 + 插件化 React 外壳。**

外壳几乎不长肉——路由、布局、内容渲染、命令、快捷键、主题、协议事件处理（StreamEvent fold）、设置面板，全部由内置插件贡献。Kernel 自己只是一组命名 Slot 加几个共享 Zustand store；插件通过统一的 `Host` API 往开放扩展点底座里贡献：往 Slot 塞组件、往 reducer 挂事件 handler、往 registry 写自己的数据。

---

## 2. 技术栈

| 层       | 选型                                                                  |
| -------- | --------------------------------------------------------------------- |
| UI       | React 19 + TypeScript                                                 |
| 样式     | Tailwind 4 + `cva` + `clsx` + `tailwind-merge`（`cn()`）              |
| Headless | Radix Primitives first（Dialog / Popover / Select / Tooltip / …）     |
| 特定件   | `cmdk`（命令面板）/ `sonner`（Toast）/ `lucide-react`（图标）         |
| 状态     | Zustand（多 store，无 context 链）                                    |
| 路由     | TanStack Router（route tree 动态构建）                                |
| 数据     | TanStack React Query                                                  |
| 协议     | 自研 Lyra Runtime Protocol v2（JSON-RPC 2.0，`src/rpc/`）             |
| 动画     | motion/react                                                          |
| 桌面壳   | Wails v2（Go 后端 + WebView 前端）                                    |
| 测试     | Vitest 4 + Testing Library + happy-dom                                |
| 构建     | Vite 8（内置 Rolldown bundler）                                       |
| Lint     | OxLint 1.x（Rust-based）；`prettier` 格式化                           |

> 已弃用 AG-UI——协议、类型、reducer 全部自研原生模型（见 `CLAUDE.md` 第一法则）。

---

## 3. 目录速览

```
src/
├── main.tsx              入口 — createRoot(<App/>)
├── App.tsx               顶层 Provider 链：QueryClient → PluginProvider → AppRouter
├── router.tsx            动态 TanStack 路由（从 listRoutes() 构建）
│
├── pages/
│   └── AgentClientPage.tsx   kernel：app.sidebar / app.main / app.overlay 三个 Slot
│
├── plugins/              插件系统
│   ├── host/                 插件宿主运行时
│   │   ├── PluginProvider.tsx    启动编排：bridge → loadPlugins → tag → ready → sideload
│   │   ├── Slot.tsx              <Slot name="…"/> 渲染注册到该 slot 的插件组件
│   │   ├── PluginBoundary.tsx    每个插件组件的 React Error Boundary
│   │   ├── PluginContentBlock.tsx 包装消息内容块的边界
│   │   ├── PluginToaster.tsx     全局 toast 层（sonner）
│   │   ├── ShortcutsProvider.tsx 全局键盘快捷键派发
│   │   ├── hostBridge.ts         挂 window.__LYRA__，让 sideload 包共用 React/SDK
│   │   └── sideload.ts           从 Go 后端拉取并 dynamic-import 用户插件
│   │
│   ├── sdk/                  插件平台
│   │   ├── types/                12 个 domain 文件 + barrel（按贡献面拆）
│   │   ├── kernelPoints.ts       ~35 个内置 ExtensionPoint（THEME / COMMAND / LAYOUT_SLOT / …）
│   │   ├── defineExtensionPoint.ts  defineExtensionPoint<T>(def) — typed point handle
│   │   ├── host.ts               createHost(pluginName) — extensions.contribute 写路径
│   │   ├── registry.ts           usePluginStore — 单一 extensions map + bookkeeping
│   │   ├── registryState.ts / registryHelpers.ts  store 内部结构 + 纯 helper
│   │   ├── selectors/            按面分组的 useXxx / lookupXxx + extensions.ts（读侧底座 + O(1) 索引）
│   │   ├── definePlugin.ts / definePluginPack.ts   loadPlugin(s) / pack
│   │   ├── capabilities.ts / pluginOrigin.ts       最小权限 + sideload 默认 deny
│   │   ├── evalWhen.ts           when 子句求值器（VS Code-when 子集）
│   │   ├── lazyActivator.ts      activationEvents + contributes 占位激活
│   │   ├── state.ts / stateSlice.ts / sharedState.ts  插件共享 state
│   │   ├── config.ts / storage.ts / notifications.ts / errors.ts
│   │   └── apiVersion.ts         HOST_API_VERSION 常量
│   │
│   └── builtin/              内置插件，按领域（限界上下文）分组
│       ├── index.ts          manifest（topo sort 由 spec.requires 驱动）
│       ├── agent/            bootstrap · core-reducer（StreamEvent→view state）· rpc-agent（默认 driver）
│       ├── chat/             composer · message-actions · plan-progress · slash-hints ·
│       │                     chat-search · preview-blocks · tools/(meta + previews)
│       ├── command/          command-palette · global-keymap · shortcuts
│       ├── defaults/         默认 commands / config / data / accents / roles / title
│       ├── i18n/             locales pack（8 语言）
│       ├── settings/         appearance · personalization · connection-settings ·
│       │                     plugins-pane · providers · icon-gallery
│       ├── shell/            纯框架：kernel · main-route · status · toaster ·
│       │                     topbar-new-tab · welcome-screen
│       ├── sidebar/          brand / search / projects / sessions / footer / 三个 rail-*
│       ├── theme/            kit（defineThemePlugin helper）+ themes（10+ 主题）
│       └── workspace/        workspace-views · tasks · diagnostics · conversation-export
│
├── protocol/run/         协议 fold 边界
│   ├── reducer.ts        纯派发器：StreamEvent → host.events.onStream/onCustom 注册的 handler
│   ├── viewState.ts      AgentViewState 形状（Item→message/block 投影）+ INITIAL_VIEW_STATE
│   └── runDigest.ts      从 timeline + toolCalls 派生的 run 摘要
│
├── state/                跨切面 Zustand store（kernel 自身的，非插件）
│   ├── agentStore.ts     每会话 AgentViewState + applyEvents + send/stop/resume binding（ephemeral）
│   ├── sessionStore.ts   tab / draft / 选择（部分持久化）
│   ├── uiStore.ts        主题 / accent / 字体 / motion / sidebarRail（持久化）
│   ├── runtimeStore.ts   握手能力（全局 ephemeral）
│   ├── tasksStore.ts     后台任务
│   ├── composerStore.ts  撰写区文本 / 模式 / provider+model（ephemeral）
│   ├── useAgentSession.ts  一个会话的 driver 生命周期编排（send/stop/resume + rAF 批处理）
│   ├── useDefaultChatSession.ts  从 AGENT_SOURCE registry 挑 driver（priority 最高）
│   ├── useWhenContext.ts  build context for `when` clauses
│   ├── toolRouting.ts    openViewForTool(toolId) — tool card 点击的路由
│   └── deeplinks.ts      deeplink 解析
│
├── components/           纯展示 + 薄 store 接线（经 selector/hook 触业务，不直连 rpc/main）
│   ├── chat/             message/(消息渲染 + markdown/ + cards/) · panel/(ChatPanel 编排) · composer/
│   ├── common/           设计系统原子（Icon · Panel · DataView 三态 render-prop · …）
│   ├── tools/            ToolCard · ToolInspector · ToolPreview · previews/
│   └── sidebar/          导航组件
│
├── lib/                  共享 hook + 纯函数（跨插件共享，不属于上述任一层）
│   ├── agent/            会话用例 hook（useChatSend / useApprovalSubmit / useQuestionAnswer /
│   │                     useCreateSession / …）+ HITL 决策词表 + streamReveal + messageContent
│   ├── data/             ky 包装（http）+ React Query（queries / queryClient）
│   ├── i18n/             i18next 接线 + 分词 + 相对时间
│   ├── markdown/         rehype 插件 + shiki + KaTeX（纯 infra）
│   └── utils.ts / motion.ts / metrics.ts / hmr.ts / systemFonts.ts
│
├── rpc/                  Runtime Protocol boundary —— 唯一 outbound 副作用层
│   ├── sdk.ts            createLyraClient(transport) — JSON-RPC client + typed methods
│   ├── methods.ts        typed method 包装（runs.start / runs.resume / runs.cancel / items.list / …）
│   ├── shapes.ts         wire schema（Zod，信任边界校验）
│   ├── stream.ts         RunEvent 信封校验 + 去重（iterableOf / bindLifecycle）
│   ├── transports/       http / memory（测试）
│   └── client.ts / channel.ts / ids.ts / errors.ts
│
├── main/                 composition root（DI）
│   ├── container.ts      getContainer() 单例：client()=createLyraClient + sidecar；测试 setContainer 注入
│   └── config.ts         RUNTIME_BASE / PROTOCOL_VERSION 等常量
│
├── styles/               globals.css（Tailwind base + @theme token + keyframes，唯一主样式）
│                         + tool/markdown/overlays/layout.css（只承载无法用 utility 表达的 chrome）
└── test/                 测试 setup
```

### 3.1 单向依赖与 outbound 边界

Lyra 大部分 UI ↔ 数据流已经通过插件系统解耦，真正需要"内外分层"的只有一处：**UI / 状态 / 插件不应直接发起 outbound 副作用**（HTTP、SSE、IPC）。这一层由 **`rpc/`（Runtime Protocol boundary）+ `main/container.ts`（composition root）** 承担。

> 早期规划过 `domain/gateways` + `infra/HttpXxx` 的清洁架构分层，**那套从未落地**：所有 outbound 都收敛成一个 JSON-RPC 协议，gateway-per-capability 的抽象没必要了。现在的边界就是"一个协议客户端"。

```
        ┌────────────────────────────────────────────┐
        │  rpc/                                        │
        │   createLyraClient(transport)                │
        │   JSON-RPC client + typed methods + shapes   │
        │   transports（http / memory）+ stream 校验   │
        │   独立层：只依赖外部库 + 自己（check-layers 强制）│
        └────────────────────────────────────────────┘
                          ▲ wires
        ┌────────────────────────────────────────────┐
        │  main/container.ts                           │
        │   getContainer() 单例：client() / sidecar    │
        │   可依赖任何东西                              │
        └────────────────────────────────────────────┘
                          ▲ via getContainer()
        ┌────────────────────────────────────────────┐
        │  components/ state/ plugins/ lib/ pages/     │
        │   拿 client 一律走 getContainer()，           │
        │   不自己 createLyraClient / 不绕过 container   │
        └────────────────────────────────────────────┘
```

**层依赖规则**（`scripts/check-layers.mjs` + `check-circular.mjs` 强制，alias-aware，`npm run check` 跑）：

- `rpc/` 是独立层：只依赖外部库 + 自己，**禁** import `state` / `sdk` / `components` / `protocol` / `main` 任何 app 层。
- `protocol/run/`（fold/viewState）可达 `rpc`（wire 类型）+ `sdk`（dispatcher seam）+ `lib`，**禁** UI / state / main。
- `plugins/sdk` / `state` / `lib` **禁** import UI（`components` / `pages` / `builtin`）——锁住"平台/工具层不依赖它被消费的 UI"。
- `components/` / `pages/` **禁** import `@/main`（composition root）或 `@/rpc`（协议客户端）——只经 store selector / `lib/data` query / SDK selector 触业务。
- 业务调用一律走 `getContainer().client().xxx(...)`；测试用 `setContainer({ client })` / `resetContainer()` 注入假实现。

**增加新协议方法的步骤**：

1. `rpc/shapes.ts` 加 wire schema（Zod，信任边界校验）。
2. `rpc/methods.ts` 加 typed method 包装。
3. UI / state / plugin 通过 `getContainer().client().foo(...)` 调用。
4. 测试用 `setContainer({ client: () => fakeClient })` 注入。

> 协议 method 表 / envelope / transport 形状的权威定义在 `docs/API.md` + `docs/TRANSPORT.md`，勿在本文重述。

### 3.2 关于 monorepo（暂不拆）

`rpc/` `main/` `plugins/` 等完全可以拆成独立 workspace packages。**暂不拆**——package 边界的真正回报是当有第二个消费方时。**触发条件**任一命中才启动：

1. 出现第二个 app（CLI / mobile / 嵌入式 web）。
2. `sample-plugins/` 里有 ≥ 2 个非空 demo，且至少一个需要外部 publish。
3. 团队扩到 3+ 人，需要按包做 CODEOWNERS。
4. 任一 `packages/` 候选超过 ~200 文件且有 5+ 外部依赖。

在那之前，TypeScript path alias + `check-layers` / `check-circular` 已给到等价的边界约束。

---

## 4. 启动流程

```
main.tsx
  └─ createRoot(<App/>)
       │
       ▼
App.tsx
  <QueryClientProvider client={queryClient}>   ◄── 最宽：plugins + queries 都需要
    <PluginProvider>                            ◄── 在 QueryClient 内，插件组件可用 query
      <AppRouter />                             ◄── 在 Plugins 内，路由能渲染插件贡献
    </PluginProvider>
  </QueryClientProvider>
```

### 4.1 PluginProvider 启动步骤

`src/plugins/host/PluginProvider.tsx`：

1. **`installHostBridge()`** — 把 React / motion / SDK 单例挂到 `window.__LYRA__`，sideload 插件不必自带这些重依赖。先于一切，让模块求值期就 touch `window.__LYRA__` 的 sideload 包能看见它。
2. **`loadPlugins(builtinPlugins)`** — 对 `spec.requires` 做拓扑排序后顺序加载内置插件（同步 setup，几个微任务搞定）。
3. **`tagAllAsBuiltin()`** — 给已加载插件打 origin 标记，Plugins 面板显示用。
4. **`markAppReady()`** — 触发所有 `host.lifecycle.onReady(...)` 回调（注册顺序）。
5. **`setBuiltinsReady(true)`** — 解除 children 渲染门；同时 fire-and-forget `loadSideloadedPlugins()`（不阻塞首屏，附加式注册）。

外层再包一个 `RadixTooltip.Provider`（`delayDuration=250`），让 kernel + 任意插件的 `<Tooltip>` 不必各自带 provider。

> **为什么门控？** AppRouter 挂载时一次性构建路由树（`buildRouter()` 读 `listRoutes()`）。内置插件注册路由前就 mount 会"no routes match"白屏。门控保证 route registry 就绪。内置 setup 无 I/O，门在下一个微任务就解开——只是首帧短暂空白。

### 4.2 AgentClientPage —— 整个 Kernel

`src/pages/AgentClientPage.tsx` 约 30 行，VS Code 式三区：

```tsx
<div className={`app ${sidebarRail ? "rail" : ""}`}>
  <div className="app-main">
    <aside aria-label="Sidebar" className="contents"><Slot name="app.sidebar" /></aside>
    <main  aria-label="Main"    className="contents"><Slot name="app.main"    /></main>
  </div>
  <Slot name="app.overlay" />
</div>
```

三个 Slot 是 kernel 的全部肉（没有底部状态栏——run telemetry 在 composer footer，全局指示/通知在 sidebar footer）：

| Slot          | 典型贡献者                                  |
| ------------- | ------------------------------------------- |
| `app.sidebar` | `kernel-sidebar`                            |
| `app.main`    | `kernel-chat`（ChatPanel）                  |
| `app.overlay` | `command-palette` / `toaster` / `shortcuts` |

`aside` / `main` 用 `display: contents` 对外层 grid 透明（landmark role 给读屏用），让每个 Slot 的 Panel 直接做 `.app-main` grid cell 的子元素，正确撑满。

---

## 5. 三大支柱

### 5.1 插件系统 —— Plugin SDK + 开放扩展点底座

#### 数据流：贡献 → 存储 → 订阅 → 渲染

```
PluginSpec.setup({ host })
       │  host.extensions.contribute(POINT, spec, opts?)   // POINT 来自 kernelPoints.ts
       ▼
   host.ts ── 通过 store() 调 registry actions
       │
       ▼
   usePluginStore (Zustand) —— 单一 extensions: Map<"${point.id}#${dedupe}", Owned<Entry>>
       │
       ▼
   selectors（sdk/selectors/）：
     useLayoutSlot("app.sidebar") → 排序后的 specs[]
     useWorkspaceViews() / useCommands() / useToolPreview(fn) / …
     lookupStreamHandlers(type) / lookupCustomHandlers(name)  ← reducer 用，非 React
       │
       ▼
   React 组件按 selector 订阅 — registry 变更触发重渲染
```

#### 一个插件长这样

```ts
// frontend/src/plugins/builtin/agent/rpc-agent/index.ts（简化）
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(AGENT_SOURCE, {
      id: "rpc",
      label: "Runtime Protocol (JSON-RPC)",
      priority: 1,
      factory: () => makeDriver(/* sessionId */),
    });
  },
});
```

`setup` 返回时所有贡献已进 registry；对应 selector 在 React 渲染时就能看见。

#### Host 接口（写路径 + 命令式动作）

`Host`（`src/plugins/sdk/types/host.ts`）是插件能调的"动词"集合。**贡献统一走开放扩展点底座**——权威文档 `docs/EXTENSION_POINTS.md`。

**贡献写路径（绝大多数"注册一个 spec"）**：

```ts
host.extensions.contribute(POINT, spec, opts?)
```

内置点（`kernelPoints.ts`，~35 个）涵盖主题 / 强调色 / 路由 / 命令 / 设置面板 / 侧栏分区 + rail / 工具预览 + 操作 + 图标 / 内容块 / 消息角色 / slash 命令 / agent source / 数据 provider / composer 状态 + 模式 + 占位符 / 快捷键 + 键绑定 / locale / 工作区 view / 错误回退 / rpc + log + 生命周期 hook 等。第三方插件用 `defineExtensionPoint` 开自己的点，机制完全相同。

**保留的薄 facade**（仍只是调 `contribute`，但各带逻辑/泛型/防错）：

| 面                                          | 为什么不退化成裸 contribute              |
| ------------------------------------------- | ---------------------------------------- |
| `host.events.onStream(type, handler)`       | StreamEvent（run.*/item.*/state.*）订阅  |
| `host.events.onCustom(name, handler)`       | custom StreamEvent 订阅                  |
| `host.layout.register(slot, spec)`          | 内部算去重 id `${slot}#${spec.id}`       |
| `host.message.registerContentBlock<K>`      | per-kind 泛型类型安全                    |
| `host.lifecycle.onReady / onBeforeUnload`   | onReady 带"已 ready 则立即触发"逻辑      |
| `host.rpc.beforeRequest / afterResponse`    | HTTP 拦截 hook 订阅                      |
| `host.log.subscribe`                        | 日志订阅 hook                            |

**命令式动作（非贡献，本就该是方法）**：`host.workspace.openView/closeView` · `host.config` · `host.storage` · `host.state` · `host.notify` · `host.window` · `host.plugins.{list,load,unload,reload}` · `host.i18n.addBundle` · `host.tasks` · `host.rpc.get/post` · `host.log.{debug,info,warn,error}` · `host.commands.execute`。

`contribute` 与每个 facade 都返回 `Disposable`，`createHost` 收集到 `setup` 期的 sink；plugin setup 抛错时自动 dispose 已注册部分，避免半成品挂在 registry。

#### Registry 内部结构

`usePluginStore`（`registry.ts` + `registryState.ts`）是一个 Zustand store。所有贡献坐落在**单一** `extensions: Map<"${point.id}#${dedupe}", Owned<ContributionEntry>>`。两种 keying（点定义里声明）：

- **single**：`dedupe` = 归一后的单键（主题 by id、工具预览 by fn、slash by trigger…）。同 key 后来者覆盖 + console 警告。
- **multi**：`dedupe` = `${plugin}|${id}`（id 默认 mint），每条共存——事件 handler / layout slot / rpc+log hook 等链式执行。

`pluginName` 嵌在外层 `Owned`，dispose 只删本插件那条；selector 侧（`selectors/extensions.ts`）用缓存在 map 引用上的二级索引保 O(1) 读。

#### 加载 / 卸载 / 懒激活

- `loadPlugin(spec)`：校验 `apiVersion` → 建 disposable sink + 绑 Host → `await setup({ host })` → 成功 `registerLoaded`，失败 dispose 已注册部分 + `reportPluginError`（**不抛出**，其它插件继续）。
- `host.plugins.{unload,reload}`：dispose 全部 + 清 loaded 表 + fire `onUnload`；`reload` = unload + 重 load。Settings → Plugins 每行有 Reload 按钮。
- **懒激活**：`activationEvents: ["onCommand:foo"]` + `contributes: { commands/views/settingsPanes }` → setup 不跑，只注册占位；用户首次访问占位时再 `loadPlugin`，selector 自动用真组件替换占位。
- **能力切片**：`spec.capabilities` 列出用到的 host namespace；未声明的在 `createHost` 阶段换成 throwing Proxy（最小权限 + 未来 marketplace 审核底）。
- **when 子句**：`CommandSpec.when?` 控制命令何时出现，语法是 VS Code-when 子集，求值器在 `evalWhen.ts`（纯模块）。

#### 内置 vs 外置（sideload）

|             | 内置                  | 外置（sideload）                                |
| ----------- | --------------------- | ----------------------------------------------- |
| 来源        | 同 bundle 静态 import | Go 后端 manifest + dynamic `import(url)`        |
| 加载时机    | 启动前同步串行        | 启动后异步（不阻塞首屏）                        |
| origin      | `builtin`             | `sideload`（默认 deny，需用户授权——`pluginOrigin.ts`）|
| 共享 React  | 同 bundle 自然共享    | 通过 `window.__LYRA__` 桥接                     |

#### 内置插件 manifest（`builtin/index.ts`）

按"先依赖、后消费"分组（仅人类可读；真实约束在各 `spec.requires`）：

```
protocol        → core-reducer
infrastructure  → defaultConfig / bootstrap / defaultData / rpcAgent / defaultTitle /
                  defaultAccents / themesPack / localesPack / mainRoute
messageRendering→ defaultRoles / messageCopy+Edit+Regenerate / previewBlocks
toolRendering   → bash / diff / file / grep / toolActions / toolIcons
composer        → slashHints / chips / modes / toolbar / placeholders / keymap / send
panes           → appearance / personalization / connection / pluginsPane / providers /
                  workspace views（diff/terminal/files/plan/timeline/runSummary/tools/skills/agentDocs/notifications）/ diagnostics
kernel          → kernelSidebar / kernelChat / kernelSettings
sidebar         → search / projects / sessions / footer / 三个 rail-*
overlays        → toaster / commandPalette / chatSearch / defaultCommands / statusPill /
                  tasksPill / statusNotifications / welcomeScreen / topbarNewTab /
                  shortcuts / globalKeymap / iconGallery / planProgress / conversationExport
```

---

### 5.2 协议 fold 层（数据流入口）

#### 形状：单 store + 一个纯派发器

```
LyraClient（rpc/）—— runs.start / runs.resume 流式返回 RunEvent
   │   useAgentSession.pump：for await (ev of stream.events)
   ▼
useAgentStore.applyEvents(sessionId, batch)   ◄── rAF 批处理，~1 commit/帧
   │   reduce(state, event)
   ▼
src/protocol/run/reducer.ts —— 纯派发器
   │
   ├─ type === "custom"   → lookupCustomHandlers(ev.name) 链式 → StateUpdate → next
   └─ 其它 StreamEvent     → lookupStreamHandlers(type) 链式 fold
   │   （每个 handler try/catch 隔离；抛错 reportPluginError + 保留入态）
   ▼
新的 AgentViewState
   │   Zustand 通知订阅者
   ▼
React 组件按 selector 重渲染
```

#### 为什么 reducer 是"纯派发器"

reducer 自己不处理任何事件语义——全部搬到 `lyra.builtin.core-reducer` 插件（`handlers` 派发 / `projections` 纯 wire→view 映射 / `fold` 有状态 upsert）。这样：

1. **统一扩展点**：第三方插件想拦截某个 StreamEvent？`host.events.onStream(type, …)` 即可，跟内置一视同仁。
2. **可测**：测试只加载需要的 core-reducer 子集。
3. **错误隔离**：一个 handler 抛错，其余继续——dispatcher 包了 try/catch。

`AgentViewState`（`viewState.ts`）把 v2 wire 模型（Session→Run→Item）投影成 UI 形状：`messages: Message[]`（每条含 `blocks: ContentBlock[]`）+ `toolCalls: Record<id, ToolCall>` + `plan` + `run`（含 `sessionId` / `runId` / step / tokens）+ `timeline` + `openInterrupts` + `shared`。其中"连续助手 Item 折进一个气泡"叫一个 **turn**（`turnMessageId`），是纯 UI 概念，与协议 Run 干净分离。

#### useAgentSession 编排会话生命周期

`src/state/useAgentSession.ts` 为**一个会话**拥有 driver 生命周期：

```
useEffect([sessionId])
  → driver = makeDriver()                         // 来自 priority 最高的 AGENT_SOURCE
  → store.resetSession(sessionId)
  → 非 draft：client.items.list → 以 item.completed 回放历史（走同一个 fold）
  → setSend / setStop / setResume（让任意插件可发起/中止/恢复）
  → 若有 pending（welcome 屏排队的首条消息）→ send 之

send(text):
  → 乐观渲染本地 local-* userMessage 气泡
  → driver.start(text, signal) = client.runs.start({ sessionId, input, mode, provider?, model? })
  → runs.start resolve 时把占位 relabel 成返回的 userItemId（流回来的 Item 按 id 去重）
  → pump 流事件（rAF 批处理）

resume(parentRunId, responses):  → driver.resume = client.runs.resume(...)   // HITL R-model
stop():                          → abort + client.runs.cancel(runId)

unmount → cancel + 解绑 send/stop/resume（该会话 view state 留在 store，切回来还在）
```

默认 driver 由 `rpc-agent` 插件贡献（`AGENT_SOURCE`，走 JSON-RPC）；插件可替换成 mock / IPC / 本地模型等。

---

### 5.3 状态分层（除 agent 外的 UI 状态）

| Store               | 内容                                                          | 持久化              |
| ------------------- | ------------------------------------------------------------- | ------------------- |
| `agentStore`        | 每会话 AgentViewState + send/stop/resume 引用 + applyEvents   | ❌ ephemeral        |
| `sessionStore`      | activeSessionId / tabIds / draft / 选择                       | ✅（部分字段）      |
| `uiStore`           | theme / accent / 字体 / motion / messageStyle / sidebarRail   | ✅                  |
| `runtimeStore`      | 握手协商的 runtime 能力                                       | ❌ 全局 ephemeral   |
| `tasksStore`        | host.tasks 的后台任务                                         | ❌                  |
| `composerStore`     | 撰写区文本 / 模式 / 附件 / provider+model                     | ❌ ephemeral        |
| `usePluginStore`    | 整个插件 registry                                            | ❌                  |
| `useConfigStore`    | 插件可读写的全局 config（如 `api.baseUrl`）                   | ✅                  |

每个 store 各自用 Zustand `persist` + 自己的 `version`；**schema 变了就 bump version 丢旧数据，不写 migration**（开发期无历史包袱）。

---

### 5.4 主题系统（IDE 风格的"主题即插件"）

每个主题就是一个完整的 CSS 变量调色板，用 `defineThemePlugin()` helper（`theme/kit/`）声明独有部分：

```ts
defineThemePlugin({
  id, label, scheme: "dark" | "light", order,
  palette: { "color-bg": "#…", "color-surface": "#…", "color-accent": "#…", … },
});
```

helper 自动补 shadow ladder + CTA defaults + 注册仪式（`host.extensions.contribute(THEME, …)`）。切主题时 `uiStore` 副作用：替换 `<html>` 的 `theme-{scheme}` class + 把 `palette` 全部 inline 写到 `:root.style`（内联永远胜过 stylesheet，插件完全拥有调色板）+ 最后写一次用户选的 `--color-accent`。

加新主题 = 新文件（调 `defineThemePlugin`）+ `theme/themes/index.ts` 加一行；Settings → Appearance 的 picker 从 registry 自动读列表。首屏防闪烁靠 `index.html` 内嵌一段同步 JS 在 CSS 解析前贴 `theme-{scheme}` class。

---

## 6. 渲染端：Slot 与各种 useXxx Hook

### 6.1 `<Slot name="…"/>`

`src/plugins/host/Slot.tsx` 是 kernel ↔ 插件的"插槽桥"：

```ts
const specs = useLayoutSlot("app.sidebar");   // 订阅 registry
return specs.map(spec => (
  <PluginBoundary key={spec.id} plugin={spec.pluginName}>
    <spec.component />
  </PluginBoundary>
));
```

- 按 `order ?? 100` 升序渲染。
- 每个 spec 包一层 `PluginBoundary`（React Error Boundary）——单个插件 render 抛错只是它自己空白，kernel 不挂。
- 默认透明（Fragment），不引入额外 DOM。

### 6.2 其它"消费端"选择器（`sdk/selectors/`）

| Hook / 函数                                          | 用途                                |
| ---------------------------------------------------- | ----------------------------------- |
| `useToolPreview(fn)` / `useToolActions()`            | 工具卡片预览 / 头部按钮             |
| `useWorkspaceViews()` / `useSettingsPanes()`         | 主区 workspace view / 设置左栏      |
| `useSidebarSections()` / `useSidebarRailItems()`     | 侧栏内部                            |
| `useCommands()` / `useSlashCommands()`               | 命令面板 / composer slash 提示      |
| `useComposerModes()` / `useComposerStatus()` / …     | composer 工具栏                     |
| `useThemes()` / `useAccents()`                       | Appearance 面板                     |
| `useMessageRole(id)`                                 | 消息头像 / 名字                     |
| `lookupStreamHandlers(type)` / `lookupCustomHandlers(name)` | reducer 内部用，非 React 选择器 |

---

## 7. 端到端的几个典型流程

### 7.1 用户输入消息发送

```
Composer onKeyDown (Enter) → submitComposer → useChatSend(text)
   → 有 active session → agentStore.send；无 → useCreateSession 起草稿 + 排队首条
   → useAgentSession.send → 乐观渲染 local 气泡 + driver.start
   → client.runs.start → 流出 run.started / item.* / state.* …
   → pump（rAF 批）→ agentStore.applyEvents → reduce → core-reducer handlers → 新 state
   → React 订阅者重渲染（ChatStream 等）
```

### 7.2 工具调用展开 / 打开完整视图

```
ChatPanel → ChatStream → MessageBlock → PartRenderer
   ─ kind="tool" 分支 → <ToolCard onOpenView={() => openViewForTool(toolId)} />

用户点 "Open in …"
   → state/toolRouting.ts.openViewForTool(toolId)
   → 按 tool.kind 决定 view id（commandExecution→terminal, fileChange→diff …）
   → uiStore.openMainView({ id, title, icon }) → mainViewTabs 追加 + active
   → ChatPanel 解析 useWorkspaceViews().find(id).component → 主区换成那个 view
```

### 7.3 HITL（人审）—— R-model

```
后端的 run 以 outcome.type="interrupt" 结束（释放资源），落一条 durable OpenInterrupt
   → core-reducer 物化一个 approval / question 块（status="requires-action"）
   → 绑定 { parentRunId, itemId }
用户点 Approve / Decline（或回答 question）
   → useApprovalSubmit / useQuestionAnswer → useAgentSession.resume(parentRunId, responses)
   → client.runs.resume 起一个续跑 Run（parentRunId 链接），新 RunEvent 流接着 fold
   → 卡片乐观 settle（resolveInterrupt）
```

### 7.4 一个 custom 协议事件落地

```
后端发 custom StreamEvent { name: "lyra.plan", payload: { items } }
   → reduce → type==="custom" 分支 → lookupCustomHandlers("lyra.plan")
   → 插件 handler(payload) 返回 StateUpdate（(state) => ({ ...state, plan: items })）
   → reducer 套到 state → agentStore 更新 → Plan workspace view 读到新 plan
```

---

## 8. 错误隔离策略

| 失败点                          | 行为                                                       |
| ------------------------------- | ---------------------------------------------------------- |
| 插件 `setup` 抛错               | dispose 已注册部分；其它插件继续；写错误到 Plugins 面板    |
| 插件组件 render 抛错            | PluginBoundary 接住画 fallback；其余 kernel 正常           |
| stream / custom handler 抛错    | 该 handler 跳过，state 保持入态；其余 handler 继续         |
| 插件 tool action / command 抛错 | console.error + `reportPluginError`，UI 不挂               |
| `runs.start` 调用 reject        | channel-a 失败：无流、无 run.finished，自行 setError 上 banner |
| run 流中断 / `run.finished{error}` | banner 显示，下次 run 启动时清除                        |
| sideload 模块 import / manifest 失败 | 跳过，其它继续；console.warn                          |

Plugins 面板（Settings → Plugins）汇总所有 `reportPluginError` 的红 badge。

---

## 9. 怎么写一个插件

最小三件套：

```ts
import { definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.example.hello",
  version: "1.0.0",
  apiVersion: "^1.0.0",                          // 可选；不写接受任意 host
  requires: ["lyra.builtin.default-themes"],     // 可选；依赖（拓扑排序）
  capabilities: ["commands", "events", "message", "notify", "logger"], // 可选；最小权限
  setup({ host }) {
    // 1. 加一个 Cmd+K 命令
    host.extensions.contribute(COMMAND, {
      id: "hello.world", label: "Hello, world!", group: "Examples",
      run: () => host.notify("hi", "info"),
    });
    // 2. 监听一个 custom StreamEvent
    host.events.onCustom<{ text: string }>("example.banner", (payload) =>
      (state) => /* 返回 StateUpdate */ state,
    );
    // 3. 提供该 kind 的内容块渲染器
    host.message.registerContentBlock("exampleBanner", ({ block }) => <div>{block.text}</div>);
    // 4. 副作用（subscribe 等）通过 setup 返回 cleanup
    const unsub = someStore.subscribe(/* … */);
    return () => unsub();
  },
});
```

> 内置：放 `plugins/builtin/<domain>/<name>/index.ts(x)`，在 `builtin/index.ts` 合适分组 import + 加进数组。
> 外置：构建成 ESM，把 React/motion/SDK 标 external 去引用 `window.__LYRA__`，放后端 sideload 目录。

**自定义内容块的类型注册**（让 TS 满意）：

```ts
declare module "@/protocol/run/viewState" {
  interface CustomContentBlockMap {
    exampleBanner: { kind: "exampleBanner"; text: string };
  }
}
```

> 内置块（text/reasoning/plan/tool/approval/question）在 `components/chat/message/` 内部直渲（`PartRenderer` switch）；扩展块（第三方 / `preview-blocks`）才走 `registerContentBlock` registry。

---

## 10. 不变量速查

- **Kernel 不知道任何具体功能**——所有看得见的元素都来自插件。改一处功能 = 改一个插件目录。
- **registry 是唯一真相**——不直接 import 内置插件去用，永远走 `useXxx` / `lookupXxx`。
- **store 是单 Zustand instance**——多 selector 订阅，不要把 store 包进 context。
- **运行时事件单向流入 view state**——render 路径不回写 agent store；想"做事"就调 store 上的 `send` / `stop` / `resume`。
- **components 不直连后端**——只经 store selector / `lib/data` query / SDK selector，**禁** import `@/main` / `@/rpc`（`check:layers` 强制）。
- **Disposable 一律由 Host 收集**——别手动 `dispose()`。
- **协议是唯一 outbound 边界**——不在 UI/store 里直接 `fetch` / 开 SSE / 调 IPC，都走 `rpc/`。
- **API breaking 改动碰 `apiVersion.ts`**——破坏 Host 接口/spec 形状的改动 bump major。

---

## 11. 进一步的阅读路径

| 想了解                          | 先看                                                              |
| ------------------------------- | ----------------------------------------------------------------- |
| 决策透镜 / 工程约定 / 反向不变量 | 仓库根 `CLAUDE.md`                                                |
| 视觉规范 / 颜色 / 排版          | `frontend/DESIGN.md`                                              |
| 协议 method 表 / envelope / 语义 | `docs/API.md`                                                     |
| transport / handshake / 错误码  | `docs/TRANSPORT.md`                                               |
| 扩展点底座 + Plugin Pack        | `docs/EXTENSION_POINTS.md`                                        |
| 插件实现细节 / 能力上限         | `docs/PLUGINS_IMPL.md` / `docs/PLUGINS_CEILING.md`               |
| Host 全部接口                   | `src/plugins/sdk/types/host.ts`                                  |
| 协议 fold                       | `src/protocol/run/reducer.ts` + `builtin/agent/core-reducer/`    |
| 一个完整内置插件                | `src/plugins/builtin/agent/rpc-agent/index.ts`                   |
| 会话生命周期                    | `src/state/useAgentSession.ts`                                   |
| 主题如何注册                    | `src/plugins/builtin/theme/kit/` + 任意 `theme/themes/*`         |

---

## 12. 改进方向（forward-looking analysis）

这份清单**有依据**而非 wishlist——每条标"做/不做的理由 + 触发条件"，避免 backlog 变成永不收敛的"理想架构"幻象。

### 12.1 值得做（收益明确、风险可控）

#### A. 补全 core-reducer 各 handler 的语义测试

**现状**：`builtin/agent/core-reducer/` 已拆成 `handlers`（派发）/ `projections`（纯映射）/ `fold`（有状态折叠），`reducer.*.test.ts` 覆盖 dispatcher + 聚合 + custom + 主要事件路径。缺口是**逐 handler 的语义快照**（每个 item.* / state.* 的 input→state delta）。
**触发条件**：加新的内置事件类型 / Item 类型时一并补上对应 handler 的语义测试。

#### B. search / webSearch 富结果渲染

**现状**：后端已发 grep（`search` kind，含 path/line/snippet）与 `webSearch`（含 title/url/snippet/faviconUrl）富结果，但 view 层 ToolCall 目前主要投影计数，preview 还从 workspace query 取数而非 tool 自带 `results`。
**怎么做**：view ToolCall 增 `results`，投影 `tool.results`，直接渲染（webSearch 带 favicon）。
**触发条件**：已就绪，属下一波可做的 quick win。

#### C. fileChange diff 直渲

**现状**：`fileChange` 当前主要带 `{path, kind}`，DiffPreview 仍用 `useDiff()` workspace query。后端补 `changes[].diff` 后，DiffPreview 应改用 `tool.changes[].diff`。
**触发条件**：后端开始下发 diff 行。

### 12.2 想做但当前 KISS / YAGNI 不允许

- **`<ToolPrimitive>` headless 组件**：目前只有 `approval` 一个真正 actionable 的块（tool 是只读指针，code/search 是被动展示）。给单一消费者抽 primitive 违反"3+ 重复才抽象"。**触发条件**：第二个 actionable block 出现（如 code-proposal 升级为 accept/reject）。
- **把 `lib/agent` 提成独立 `application/` 层**：`lib/` 已是"跨插件共享"的明确语义（`messageContent` 就是被刻意从 plugin 内部移来的），6 个用例 hook 不足以撑起一个独立层 + 一条新 layer-guard。**触发条件**：用例 hook 显著增多、或 UI 开始绕过它们直接编排 rpc。
- **MessageStream 虚拟化**：长会话（1000+ 消息）目前无人抱怨。**触发条件**：实测 > 500 消息卡顿时引入 `@tanstack/react-virtual`。
- **monorepo packages**：见 §3.2 的 4 个触发条件，目前一个都没命中。

### 12.3 反向不变量（已知错的方向，别再提）

与 `CLAUDE.md §6` 一致，不重述。要点：不换 Zustand / React Query / Wails / OxLint / Vite；不给内部数据流加 Zod（只在信任边界）；不把贡献面退回 per-slot add/remove map（已塌进 `extensions` 底座）；协议保持 JSON-RPC，不 RESTy 化、不在 envelope 装 transport 元数据、不做后端鉴权/订阅（Runtime 无状态纯计算单元）。详见 `CLAUDE.md §6` + `docs/API.md §0`。

---

> 当前架构通过所有审计原则（KISS / SOLID / YAGNI / DRY），无 AG-UI 残留，文件 LOC 在合理范围，热路径有测试覆盖。日常维护维持现状，**继续等触发条件出现**，不做投机式重构。
