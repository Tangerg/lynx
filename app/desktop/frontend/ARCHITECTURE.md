# Lyra 前端架构

> 本文档描述 `frontend/` 这个 React + TypeScript 应用是怎么组织、怎么运行的。
> 设计系统 / 视觉规范请看 `DESIGN.md`。

---

## 1. 一句话概括

**Lyra 前端 = AG-UI 流式协议 + 插件化 React 外壳**。

外壳几乎不长肉——所有路由、布局、内容渲染、命令、快捷键、主题、AG-UI 事件处理、设置面板都由"插件"贡献。Kernel 自己只是一组命名 Slot 加几个共享 store；插件通过一个统一的 `Host` API 往 Slot 里塞组件、往 reducer 里挂事件 handler、往 Zustand 注册的 registry 里写自己的数据。

---

## 2. 技术栈

| 层     | 选型                                                    |
| ------ | ------------------------------------------------------- |
| UI     | React 19 + TypeScript 6                                 |
| 状态   | Zustand（多 store，无 context 链）                      |
| 路由   | TanStack Router（route tree 动态构建）                  |
| 数据   | TanStack React Query                                    |
| 协议   | `@ag-ui/core` / `@ag-ui/client`（AbstractAgent 事件流） |
| 动画   | motion/react                                            |
| 桌面壳 | Wails v2（Go 后端 + WebKit/Chromium 前端）              |
| 测试   | Vitest 4 + Testing Library + happy-dom                  |
| 构建   | Vite 8（内置 Rolldown）                                 |
| Lint   | OxLint 1.x（Rust-based）                                |
| Node   | >= 22.12（CI / dev 推荐 24 LTS）                        |

---

## 3. 目录速览

```
src/
├── main.tsx              入口 — createRoot(<App/>)
├── App.tsx               顶层 Provider 链
├── router.tsx            动态 TanStack 路由
├── style.css             全局样式（基础变量 + 主题）
│
├── pages/
│   └── AgentClientPage.tsx   kernel：sidebar / main / overlay 三个 Slot
│
├── plugins/              插件系统
│   ├── PluginProvider.tsx    启动编排：加载 builtin → 加载 sideload
│   ├── Slot.tsx              <Slot name="..."/> 渲染所有注册到该 slot 的插件组件
│   ├── PluginBoundary.tsx    每个插件组件的 React Error Boundary
│   ├── PluginContentBlock.tsx 包装消息内容块的边界
│   ├── PluginToaster.tsx     全局 toast 层
│   ├── ShortcutsProvider.tsx 全局键盘快捷键派发
│   ├── hostBridge.ts         挂载 window.__LYRA__，让 sideload 包共用 React/SDK
│   ├── sideload.ts           从 Go 后端拉取并 dynamic-import 用户插件
│   ├── sdk/                  插件 SDK —— Host、Registry、各种 spec 类型
│   │   ├── types/            13 个 domain 文件 + barrel（按面拆，details 见 §5.1.7）
│   │   │   ├── index.ts      barrel 重导出，外部 import 仍走 `./types`
│   │   │   ├── common.ts     Disposable + lifecycle hooks
│   │   │   ├── tool.ts       ToolPreview / ToolAction
│   │   │   ├── message.ts    ContentBlock / MessageRole
│   │   │   ├── agui.ts       StateUpdate / Core / Custom EventHandler
│   │   │   ├── theme.ts      ThemeSpec / ThemeAccentSpec
│   │   │   ├── composer.ts   全部 composer-* spec
│   │   │   ├── sidebar.ts    SidebarSection / SidebarRailItem
│   │   │   ├── commands.ts   Command / Shortcut
│   │   │   ├── workspace.ts  WorkspaceView / Layout / Route / SettingsPane
│   │   │   ├── infra.ts      RPC / Data / Agent / Notification / Log / ErrorFallback
│   │   │   ├── i18n.ts       I18n resource / locale 贡献类型
│   │   │   ├── host.ts       Host 接口（extensions.contribute 写路径 + 少量薄 facade + 命令式动作）
│   │   │   └── plugin.ts     PluginSpec / Contributed / HostCapability / LoadedPlugin
│   │   ├── host.ts           createHost(pluginName) — extensions.contribute 写路径
│   │   ├── defineExtensionPoint.ts  defineExtensionPoint<T>(def) — typed point handle
│   │   ├── kernelPoints.ts   ~35 个内置 ExtensionPoint（THEME / COMMAND / LAYOUT_SLOT / …）
│   │   ├── pointIds.ts       registry 内部 firing 循环用的 point id 常量（破环）
│   │   ├── registry.ts       usePluginStore — 单一 extensions map + declared* + bookkeeping
│   │   ├── selectors/        按面分组的 useXxx / lookupXxx + extensions.ts（底座读侧 + 缓存索引）
│   │   ├── definePlugin.ts   loadPlugin / loadPlugins
│   │   ├── state.ts          AG-UI state 更新组合子（appendBlock…/patchRun…）
│   │   ├── storage.ts        每插件 namespaced localStorage
│   │   ├── stateSlice.ts     插件共享 state slice
│   │   ├── config.ts         全局 config store
│   │   ├── errors.ts         插件错误聚合（PluginsPane 显示）
│   │   ├── notifications.ts  持久化通知 feed
│   │   └── apiVersion.ts     HOST_API_VERSION 常量
│   │
│   └── builtin/              内置插件，按领域分组（数量动态增长，跑 `ls plugins/builtin | wc -l` 查实时）
│       ├── index.ts          manifest（topo sort 由 spec.requires 驱动）
│       ├── core-reducer/     AG-UI 内置事件 → view state
│       ├── kernel/           填 app.sidebar / app.main / Settings 三个核心 slot
│       ├── composer/         撰写区一组 plugin（modes / toolbar / send / …）
│       ├── sidebar/          侧栏一组 plugin（brand / sessions / rail / …）
│       ├── workspace-views/  workspace view（Diff/Files/Terminal/Plan/Tools/Notifications/Timeline/RunSummary 等）
│       ├── content-blocks/   一组消息内容块（plan/code/approval/…）
│       ├── agui-handlers/    一组 CUSTOM 事件 → state 的 handler
│       ├── tool-previews/    4 个工具 inline preview（bash/diff/file/grep）
│       ├── tool-meta/        tool actions + tool icons
│       ├── defaults/         默认 commands / config / data / accents / roles / title
│       ├── themes/           defineThemePlugin helper + builtinThemes 数组
│       ├── lyra-dark / lyra-light / atom-one-dark / atom-one-light /
│       │   tokyo-night-storm / tokyo-night-light / solarized-dark /
│       │   solarized-light / catppuccin-mocha / catppuccin-latte
│       │                     10 个独立主题插件（details 见 §5.4）
│       ├── status/           status bar 的 pill + notifications badge
│       ├── command-palette/  Cmd+K 浮层
│       └── … 其它独立小插件（toaster / shortcuts / icon-gallery / demo / …）
│
├── protocol/agui/        AG-UI 协议适配
│   ├── viewState.ts      AgentViewState 形状 + INITIAL_VIEW_STATE
│   ├── reducer.ts        纯派发器：core → onCore 链；CUSTOM → on 单 handler
│   ├── customEvents.ts   Lyra 自定义事件类型（lyra.plan、lyra.telemetry…）
│   └── mockScript.ts     mock 数据
│
├── state/                跨切面 Zustand store（非插件，kernel 自身的）
│   ├── agentStore.ts     AgentViewState + applyEvent + send/stop binding
│   ├── themeStore.ts     theme + accent + applyTheme 副作用（持久化）
│   ├── layoutStore.ts    sidebarRail + toggleSidebar（持久化）
│   ├── sessionStore.ts   activeSessionId / tabIds / mainViewTabs / activeFile /
│   │                     selectedToolId / expandedToolIds（部分持久化）
│   ├── composerStore.ts  撰写区文本 + 模式 + 附件
│   ├── useAgentSession.ts AG-UI Agent 生命周期 hook
│   ├── useDefaultChatSession.ts  从 agentSource registry 挑选 agent
│   ├── useWhenContext.ts  build context for `when` clauses（theme / scheme / sidebarRail / mainView）
│   └── toolRouting.ts    openViewForTool(toolId) — tool card 点击的路由
│
├── components/           共享 UI 组件（不是插件）
│   ├── common/           Icon / Panel / Chip / ScrollArea / EmptyState / Skeleton /
│   │                     DataView（loading|empty|content 三态 render-prop）
│   ├── chat/             ChatPanel（约 50 行 orchestrator） / PanelHeader / ChatStream /
│   │                     WorkspaceViewBody / Composer / MessageStream / PartRenderer
│   ├── tools/            ToolCard / ToolPreview / previews/
│   ├── views/           DiffView / Terminal / FilesChanged / McpRow / PlanList / ViewHeader
│   ├── settings/         SettingsPage（workspace view 主体）
│   ├── sidebar/          类型 + Avatar
│   └── icon-gallery/     IconGallery / IconShowcase
│
├── domain/               【整洁架构】业务接口 + 模型（零外部依赖）
│   ├── models/           值类型：Approval / 未来的 Session/Tool/…
│   ├── gateways/         outbound 副作用契约：PermissionGateway / …
│   └── index.ts          barrel，外部从 `@/domain` 引入
│
├── infra/                【整洁架构】框架适配（实现 domain gateway）
│   └── http/             HTTP 实现：HttpPermissionGateway
│
├── main/                 【整洁架构】composition root
│   └── container.ts      DI 单例：getContainer() / setContainer() 测试用
│
├── lib/                  共享工具 hook + 纯函数（不属于上述任何层）
│   ├── queries.ts / http.ts / queryClient.ts / motion.ts
│   ├── smoothText.ts / segmentWords.ts / rehypeFadeIn.ts / shiki.ts
│   ├── markdownPartials.ts / useDebouncedValue.ts
│   ├── useStickyBottomScroll.ts / useApprovalSubmit.ts
├── utils/                inline 渲染、TS 简易语法高亮
├── styles/app.css        承载主体样式
└── test/                 测试 setup
```

### 3.1 整洁架构 → Lyra 适配

参考 [clean-react-app](https://github.com/rmanguinho/clean-react) 的整洁架构分层，但**没有照搬全套六层**（`domain/data/infra/main/presentation/validation`）。Lyra 大部分 UI ↔ 数据流通过插件系统已经解耦了，所以只补真正缺的那一层 —— "UI 直接发起 outbound 副作用"——剩下的层 YAGNI 不做。

**层依赖规则**（由 `src/test/architecture.test.ts` 强制）：

```
                       ┌──────────────────────────┐
                       │  domain/                 │
                       │   - models (types only)  │
                       │   - gateways (contracts) │
                       │   零依赖                  │
                       └──────────────────────────┘
                              ▲
                              │ implements
                       ┌──────────────────────────┐
                       │  infra/                  │
                       │   - HttpXxxGateway       │
                       │   只依赖 domain + lib/http│
                       └──────────────────────────┘
                              ▲
                              │ wires
                       ┌──────────────────────────┐
                       │  main/container.ts       │
                       │   getContainer() 单例     │
                       │   可以依赖任何东西        │
                       └──────────────────────────┘
                              ▲
                              │ via getContainer()
                       ┌──────────────────────────┐
                       │  components/ state/      │
                       │  plugins/ lib/ pages/    │
                       │   不能直接 import infra/  │
                       └──────────────────────────┘
```

具体规则：

- `domain/` 只能 `import type`，**禁** React / fetch / zustand / `@/infra/*` / 其他 `@/...` 任何运行时模块
- `infra/` 实现 `domain/gateways/*`，可以用 fetch / localStorage / IPC，但**禁** 反向 import `@/components/*` / `@/state/*` / `@/plugins/*` / `@/main/*`
- `main/container.ts` 是**唯一**把 infra 实现塞给 domain 接口槽的地方
- UI / 状态 / 插件 / lib 全部通过 `getContainer().xxx.method(...)` 拿 gateway，**禁直接 import `@/infra/*`**

**增加新 gateway 的步骤**：

1. `domain/gateways/Foo.ts` 写接口 + 必要的 `domain/models/Foo.ts` 类型
2. `infra/.../HttpFoo.ts` (或别的 transport) 实现接口
3. `main/container.ts` 加字段 + `defaultContainer()` 里实例化
4. UI 调用 `getContainer().foo.method(...)`
5. 测试用 `setContainer({ foo: fakeFoo })` 注入假实现

合规检查跑 `npm run test` 里的 `architecture.test.ts` 即可——违反任何一条规则都会失败。

**抽象定义在使用方（关键纪律）**：

- 谁需要抽象，谁定义抽象。Permission flow 需要提交审批 → `PermissionGateway` 定义在 `domain/`，**不是因为 infra 要写 HTTP 实现才定义**。
- 反例：**禁**为了 infra 方便先建一个通用 `ApiClient` interface 然后让 domain / use case 依赖它。这样会把 transport 形状渗回业务层，违反"内层不知道外层"。
- 反例：**禁**新建空泛的 `Repository<T>` / `Gateway<T>` 这种"以防万一"的抽象。每个 gateway 是一个具体业务能力（提交审批 / 发起 agent run / 列会话）。
- 真实后端尚未接入前，**禁**先一次性把所有可能用到的 gateway 都建出来。等具体 use case 触发再开。
- `lib/`、`main/config.ts` 这类配置 / 工具属于 outer adapter，不要被误读成 inner domain。Composition root 可以读 `main/config` 拿默认 URL，但不应该读 `lib/http`（plugin-aware RPC facade）拿同一个常量——配置和 RPC facade 是两件事。

### 3.2 关于 monorepo（暂不拆）

`domain/` `infra/` `main/` 完全可以拆成独立的 `@lyra/domain` `@lyra/infra` 等 workspace packages（参考 clean-react-app 风格）。我们**暂时不拆**——package 边界的真正回报是当有第二个消费方时。**触发条件**任一命中就启动 monorepo 改造：

1. 出现第二个 app（CLI / mobile / 嵌入式 web）
2. `sample-plugins/` 里有 ≥ 2 个非空 demo，且其中至少一个需要外部 publish
3. 团队扩到 3+ 人，需要按包做 CODEOWNERS
4. 任何一个 `packages/` 候选超过 ~200 文件且有 5+ 外部依赖

在那之前，TypeScript path alias + `architecture.test.ts` 已经给到等价的边界约束。

---

## 4. 启动流程

```
main.tsx
  └─ createRoot(<App/>)
       │
       ▼
App.tsx
  <QueryClientProvider client={queryClient}>
    <PluginProvider>                ◄── 1. 安装 window.__LYRA__
      <AppRouter />                       2. loadPlugins(builtinPlugins)
    </PluginProvider>                     3. tagAllAsBuiltin()
  </QueryClientProvider>                  4. lifecycle.onReady 派发
                                          5. setState(builtinsReady = true)
                                          6. 后台 loadSideloadedPlugins()
```

### 4.1 PluginProvider 启动 5 步

`src/plugins/PluginProvider.tsx`:

1. **`installHostBridge()`** — 把 React / motion / SDK 单例挂到 `window.__LYRA__`，sideload 插件不必自带这些重依赖。
2. **`loadPlugins(builtinPlugins)`** — 顺序加载 manifest 里的内置插件（同步 setup，几个微任务搞定）。
3. **`tagAllAsBuiltin()`** — 给已加载的插件打 origin 标记，PluginsPane 显示用。
4. **`markAppReady()`** — 触发所有 `host.lifecycle.onReady(...)` 回调。
5. **`setBuiltinsReady(true)`** —— 解除 children 的渲染门。同时 fire-and-forget 启动 `loadSideloadedPlugins()`（不阻塞首屏）。

> **为什么要门控？** AppRouter 在挂载时一次性构建路由树（`buildRouter()` 读 `listRoutes()`）。如果在内置插件注册路由前就 mount，会出现"no routes match"白屏。门控保证 route registry 已就绪。

### 4.2 AppRouter 动态构建路由

`src/router.tsx`:

- `rootRoute` 是 TanStack 的固定根。
- `buildRouter()` 从 `listRoutes()` 读出每个插件贡献的 `RouteSpec`，调用 `createRoute({ parent, path, component })` 拼接。
- 内置 `lyra.builtin.main-route` 注册 `/` → `AgentClientPage`。
- 注：sideload 路由暂未在首次构建后重建，需要 reload。

### 4.3 AgentClientPage —— 整个 Kernel

`src/pages/AgentClientPage.tsx` 全文约 30 行：

```tsx
<div className={`app ${sidebarRail ? "rail" : ""}`}>
  <div className="app-main">
    <Slot name="app.sidebar" />
    <Slot name="app.main" />
  </div>
  <Slot name="app.overlay" />
</div>
```

三个 Slot 就是 kernel 的全部肉（没有底部状态栏 —— run telemetry 在 composer footer，全局指示/通知在 sidebar footer 的 avatar 区）：

| Slot          | 典型贡献者                                  |
| ------------- | ------------------------------------------- |
| `app.sidebar` | `kernel-sidebar`                            |
| `app.main`    | `kernel-chat`（ChatPanel）                  |
| `app.overlay` | `command-palette` / `toaster` / `shortcuts` |

---

## 5. 三大支柱

### 5.1 插件系统（最大）—— Plugin SDK + Registry

#### 数据流：注册 → 存储 → 订阅 → 渲染

```
PluginSpec.setup({ host })
       │
       │  host.extensions.contribute(POINT, spec)
       ▼
   host.ts ── 通过 store() 调 registry actions
       │
       ▼
   usePluginStore (Zustand) —— addX(pluginName, spec)
       │  state 是一堆 Map<key, { pluginName, value }>
       ▼
   selectors / hooks（registry.ts）：
     useLayoutSlot("app.sidebar") → 排序后的 specs[]
     useWorkspaceViews()
     useCommands()
     useToolPreview(fn)
     lookupCoreEventHandlers(eventType)
     …
       │
       ▼
   React 组件订阅 — registry 变更触发重新渲染
```

#### 一个插件长这样

```ts
// frontend/src/plugins/builtin/sidebar-footer/index.tsx
import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.sidebar-footer",
  version: "1.0.0",
  // 可选：apiVersion: "^1.0.0" 表明对 host 的最低期望
  setup({ host }) {
    host.layout.register("sidebar.footer", {
      id: "user-card",
      order: 0,
      component: SidebarFooter,
    });
  },
});
```

`setup` 返回时所有注册都进了 registry；`Slot name="sidebar.footer"` 在 React 渲染时就能看见这个 component。

#### Host 接口（写路径 + 命令式动作）

`Host` 接口 (`src/plugins/sdk/types/host.ts`) 是插件能调的"动词"集合。**贡献统一走开放扩展点底座**——权威文档 `docs/EXTENSION_POINTS.md`。

**贡献写路径（绝大多数"注册一个 spec"）**：

```ts
host.extensions.contribute(POINT, spec, opts?)   // POINT 来自 kernelPoints.ts
```

内置点（`src/plugins/sdk/kernelPoints.ts`，~35 个）涵盖主题/强调色/路由/命令/设置面板/侧栏分区+rail/工具预览+操作+图标/内容块/消息角色/slash 命令/agent source/数据 provider/composer 状态+模式+附件源+占位符/快捷键+键绑定/locale/工作区 view/错误回退/rpc+log+生命周期 hook 等。第三方插件用 `defineExtensionPoint` 开自己的点，机制完全相同。

**保留的薄 facade**（仍只是调 `contribute`，但各自带逻辑/泛型/防错，故保留具名）：

| 面                                        | 为什么不退化成裸 contribute         |
| ----------------------------------------- | ----------------------------------- |
| `host.agui.on / onCore`                   | AG-UI 事件订阅 API，2-arg 具名表意  |
| `host.layout.register(slot, spec)`        | 内部算去重 id `${slot}#${spec.id}`  |
| `host.message.registerContentBlock<K>`    | per-kind 泛型类型安全               |
| `host.lifecycle.onReady / onBeforeUnload` | onReady 带"已 ready 则立即触发"逻辑 |
| `host.rpc.beforeRequest / afterResponse`  | HTTP 拦截 hook 订阅                 |
| `host.log.subscribe`                      | 日志订阅 hook                       |

**命令式动作（非贡献，本就该是方法）**：`host.workspace.openView/closeView` · `host.config` · `host.storage` · `host.state` · `host.notify` · `host.window` · `host.plugins.{list,load,unload,reload,onLoad,onUnload}` · `host.i18n.addBundle` · `host.tasks` · `host.rpc.get/post` · `host.log.{debug,info,warn,error}`。

`contribute` 与每个 facade 都返回一个 `Disposable`。`createHost` 把这些 disposable 收集到 `setup` 期间的 sink 数组里——一旦 plugin setup 抛错，会自动 dispose 已注册的部分，避免半成品挂在 registry 里。

#### Registry 内部结构

`usePluginStore` (`src/plugins/sdk/registry.ts`) 是一个 Zustand store。所有贡献坐落在**单一** `extensions: Map<"${point.id}#${dedupe}", Owned<ContributionEntry>>`（key 格式收敛在 `composeExtensionKey`）。两种 keying（点定义里声明）：

- **single**：`dedupe` = 归一后的单键（主题 by id、工具预览 by fn、slash by trigger…）。同 key 后来者覆盖 + console 警告。
- **multi**：`dedupe` = `${plugin}|${id}`（id 默认 mint），每条共存——事件 handler / layout slot / rpc+log hook 等链式执行。

`pluginName` 嵌在外层 `Owned`，dispose 只删本插件那条；selector 侧（`selectors/extensions.ts`）用缓存在 `extensions` map 引用上的二级索引保 O(1) 读。`registry.ts` 自身只剩 `addContribution`/`removeContribution` + declared-placeholder 的 `addOwned` + `loaded`/`pendingActivations`/window 等 bookkeeping。

#### 加载与卸载

`loadPlugin(spec)` (`src/plugins/sdk/definePlugin.ts`):

1. 校验 `apiVersion`（`compare-versions.satisfies`）；不兼容跳过。
2. 创建一个 disposable sink + 绑定到 `pluginName` 的 Host。
3. `await spec.setup({ host })`。
4. **成功** → `registerLoaded({ spec, disposables })`，PluginsPane 显示一行。
5. **失败** → 把 sink 里已注册的 disposable 全部 dispose，调用 `reportPluginError(name, "setup", err)`，**不抛出**——其它插件继续加载。

`reportPluginError` 写到 `usePluginErrorStore`；Plugins 设置面板用红色 badge 展示，提供"Clear"。

#### 内置 vs 外置（sideload）

|             | 内置                  | 外置                                            |
| ----------- | --------------------- | ----------------------------------------------- |
| 来源        | 同 bundle 静态 import | Go 后端 `/agui/plugins` + dynamic `import(url)` |
| 加载时机    | 启动前同步串行        | 启动后异步                                      |
| 阻塞首屏    | 是（设计如此）        | 否                                              |
| origin 标记 | `builtin`             | `sideload`（PluginsPane 显示徽章）              |
| 共享 React  | 同 bundle 自然共享    | 通过 `window.__LYRA__` 桥接（不需要自带 React） |

`hostBridge.ts` 把这几个单例挂到 `window.__LYRA__`：

- `React`、`react/jsx-runtime`
- `motion/react`
- `@/plugins/sdk`（整个 SDK 命名空间）
- `apiVersion: HOST_API_VERSION`

第三方插件包应该把 React / motion / SDK 标记为 external，运行时从 `window.__LYRA__` 拿，避免重复 React 实例。

#### 内置插件加载顺序（`builtin/index.ts` manifest）

按"先依赖、后消费"的拓扑分组：

```
protocol         → core-reducer / 各 CUSTOM handler
infrastructure   → defaultConfig / defaultData / httpAgent / defaultTitle / defaultThemes / mainRoute
messageRendering → defaultRoles + 各内容块（plan/code/search/approval/checkpoint/reasoning）
toolRendering    → bash / diff / file / grep / toolActions / toolIcons
composer         → slashHints / demo / chips / modes / toolbar / placeholders / sampleAttachments / keymap / hint / send
panes            → appearance / pluginsPane / workspace views (Files / Diff / Terminal / Plan / Tools / Notifications)
kernel            → kernelSidebar / kernelChat / kernelSettings
sidebar          → brand / search / projects / sessions / footer / 三个 rail-*
overlays         → toaster / commandPalette / defaultCommands / statusPill / statusNotifications / welcomeScreen / topbarNewTab / shortcuts / iconGallery
```

所有"必须先于 X 加载"的约束通过 `spec.requires` 声明，kernel 加载时做拓扑排序、自动按依赖顺序加载、检测缺失依赖与循环。manifest 数组的顺序仅作 tie-breaker，不再承载语义约束。

#### 懒加载：`activationEvents` + `contributes`

每个 `PluginSpec` 可选字段：

```ts
activationEvents?: ("onStartup" | `onCommand:${string}`)[];
contributes?: {
  commands?:       ContributedCommand[];       // = CommandSpec minus run
  views?:          ContributedView[];           // = WorkspaceViewSpec minus component
  settingsPanes?:  ContributedSettingsPane[];   // = SettingsPaneSpec minus component
};
```

- 缺省或包含 `"onStartup"` —— 启动期立即跑 `setup()`（默认行为）
- 只有 `"onCommand:foo.bar"` —— `setup` **不跑**，只把 `contributes.*` 注册成占位；用户首次访问占位时再激活
- 激活流程：占位被触发（点击命令 / 打开视图 / 切到面板）→ activator → `loadPlugin(spec)` → setup 完成 → 真实 register 已就位 → selector 重新发出列表，自动替换占位

`useCommands` / `useWorkspaceViews` / `useSettingsPanes` 内部自动合并 declared + registered（registered 优先），对外完全透明。占位视图/面板渲染时显示一个 `.lazy-activator` 容器，setup 完成后自动被真组件替换。

#### 卸载 / 热重载：`host.plugins.{unload,reload}`

每个 `register*` 返回的 `Disposable` 被 kernel 收集；`setup` 也可以返回一个清理函数（subscribe 这类副作用用）。`unloadPlugin(name)` / `host.plugins.unload(name)` 一次性 dispose 全部、清出 loaded 表、fire `onUnload` 监听器。`reload` 是 unload + 重新 load 同一个 spec。

Settings → Plugins 面板每行有 Reload 按钮；插件开发期改了一个文件，可以在 UI 里直接刷新而不需要重启 Wails。

#### 能力切片：`spec.capabilities`

可选数组列出本插件用到的 host namespace（`"tool" | "commands" | "agui" | …`）。设置后 kernel 在 `createHost` 阶段把未声明的 namespace 替换为 throwing Proxy——访问 `host.workspace.openView` 时如果 capabilities 没列 `"workspace"`，直接抛错。这是给"最小权限"+"未来的 marketplace 审核"打的底；不写就是全开（backward compatible）。

#### `when` 子句

`CommandSpec.when?: string` 允许声明上下文表达式控制命令何时出现在面板。语法是 VS Code-when 的小子集：

| 形式                  | 含义                                      |
| --------------------- | ----------------------------------------- |
| `mainViewActive`      | 标识符；context map 里查值，真值视为 true |
| `!mainViewActive`     | 取反                                      |
| `mainView == "diff"`  | 字符串等值                                |
| `a && b` / `a \|\| b` | 逻辑组合，`&&` 优先级高于 `\|\|`          |
| `(a \|\| b) && c`     | 括号分组                                  |

上下文由 `useWhenContext()` 产生（state/useWhenContext.ts），默认携带 `mainViewActive` / `mainView` / `theme` / `sidebarRail`。比如 `default-commands` 给 "View: X" 加上 `when: 'mainView != "X"'`，这样当前在哪个 view 就不会再列出对应的"打开"命令。

求值器在 `sdk/evalWhen.ts`，纯模块、无依赖；解析失败的表达式静默隐藏对应命令（不抛错给 UI）。

---

### 5.2 AG-UI 协议层（数据流入口）

#### 形状：单 store + 一个事件 fold

```
AbstractAgent (远端 / mock)
   │   subscribe.onEvent({ event })
   ▼
useAgentStore.applyEvent(event)
   │   reduce(state, event)
   ▼
src/protocol/agui/reducer.ts —— 纯派发器
   │
   ├─ EventType.CUSTOM            → lookupCustomEventHandler(ev.name)?.(value) → StateUpdate → next state
   └─ 其它 built-in event type    → lookupCoreEventHandlers(type) 链式 fold
   │
   ▼
新的 AgentViewState
   │   Zustand 通知订阅者
   ▼
React 组件按 selector 重渲染
```

`AgentViewState` (`viewState.ts`):

```ts
{
  run: { id, status, model, ... },        // 一次跑的元数据
  messages: Message[],                     // 转录
  toolCalls: Record<string, ToolCall>,     // 按 id 索引
  plan: PlanItem[],                        // lyra.plan CUSTOM 事件维护
  // ...
}
```

#### 为什么 reducer 是"空壳"

历史上 reducer 自己处理 RUN_STARTED / TEXT_MESSAGE_CONTENT / TOOL_CALL_START 等。现在全部搬到 `lyra.builtin.core-reducer` 插件——这样：

1. **统一的扩展点**：用户插件想要拦截 RUN_FINISHED 加自己的副作用？`host.agui.onCore("RUN_FINISHED", ...)` 即可，跟内置一视同仁。
2. **可测**：测试可以只加载需要的 core-reducer 子集。
3. **错误隔离**：一个 handler 抛错，其余继续——`applyCoreHandlers` 用 try/catch 包了每次调用。

#### useAgentSession 编排会话生命周期

`src/state/useAgentSession.ts`:

```
useEffect → makeAgent() → 一个 AbstractAgent 实例
          → useAgentStore.reset()
          → store.setStop/setSend(...)             // 让任意插件可以 send/stop
          → agent.subscribe({ onEvent: applyEvent })
          → agent.runAgent()                       // 启动第一个 turn

unmount  → subscription.unsubscribe()
          → agent.abortRun()
          → setStop/setSend(null)
```

`makeAgent` 由 `useDefaultChatSession` 从 `host.extensions.contribute(AGENT_SOURCE, …)` 注册的 agent source 里挑（priority 最高的）。内置 `http-agent` 走 HTTP；插件可以替换成 mock、IPC、本地模型等。

---

### 5.3 状态管理（除 agent 之外的 UI 状态）

| Store                  | 内容                                                                                                     | 持久化 key                                                            |
| ---------------------- | -------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `useAgentStore`        | AgentViewState + 当前 agent 的 send/stop 引用                                                            | ❌ 每次会话重置                                                       |
| `useThemeStore`        | theme id + accent hex + applyTheme 副作用                                                                | ✅ `lyra.theme`                                                       |
| `useLayoutStore`       | sidebarRail boolean                                                                                      | ✅ `lyra.layout`                                                      |
| `useSessionStore`      | activeSessionId / tabIds / mainViewTabs / activeMainView / activeFile / selectedToolId / expandedToolIds | ✅ `lyra.session`（仅 activeSessionId + tabIds 持久；其余 ephemeral） |
| `useComposerStore`     | textarea 文本 + 模式 + 附件                                                                              | ❌                                                                    |
| `usePluginStore`       | 整个插件 registry                                                                                        | ❌                                                                    |
| `useConfigStore`       | 插件可读写的全局 config（如 `api.baseUrl`）                                                              | ✅                                                                    |
| `useNotificationStore` | host.notify 推过的持久 feed                                                                              | ❌                                                                    |
| `usePluginErrorStore`  | 插件错误聚合                                                                                             | ❌                                                                    |

**为什么 UI store 拆成三块**：原来的 `useUIStore` 把主题 / 布局 / 会话 tab / 工具检查器五种 concern 揉一起，违反单一职责。拆分后每个组件只订阅自己关心的那一块。

每个 store 各自用 Zustand `persist` 中间件 + 自己的 version 号；任意单一 store 的 schema 变更只重置该 store 的存档。

---

### 5.4 主题系统（IDE/VS Code 风格的"主题即插件"）

#### 形状：ThemeSpec.tokens = CSS 变量 map

每个主题就是一个**完整的 CSS 变量调色板**：

```ts
type ThemeSpec = {
  id: string; // 持久化到 useThemeStore.theme（"dark" / "atom-one-dark" / ...）
  label: string; // 显示名
  scheme: "dark" | "light"; // 决定 <html> 上的 theme-{scheme} class + shadow 策略
  icon?: string; // 选择器图标，默认按 scheme 给 moon / sun
  order?: number; // 排序提示
  tokens?: Record<string, string>; // CSS 变量名（无 -- 前缀）→ 值
};
```

当主题切换时，`themeStore` 的副作用：

1. 从 `usePluginStore.themes` 查找 spec
2. 替换 `<html>` 上的 `theme-dark` / `theme-light` class（驱动结构性 CSS override）
3. 把 `spec.tokens` 全部以 inline style 写到 `:root.style` —— 内联永远胜过 stylesheet 声明，于是插件完全拥有调色板
4. 最后写一次 `--color-accent`，让用户选的 accent 覆盖主题默认

#### `defineThemePlugin` helper：高内聚 DRY

每个主题只需要声明它独有的部分：

```ts
// frontend/src/plugins/builtin/atom-one-dark/index.ts
import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "atom-one-dark",
  label: "Atom One Dark",
  scheme: "dark",
  order: 10,
  palette: {
    "color-accent": "#528bff",
    "color-accent-border": "#4078e6",
    "color-bg": "#1c2026",
    "color-surface": "#282c34",
    "color-text": "#abb2bf",
    // ... 大概 25 行调色板
  },
});
```

helper 自动补：

- Shadow ladder（dark scheme = inner none / overlay shadow-lg；light scheme = 完整堆叠 ladder）
- CTA 默认指向 accent（除非传 `cta:` override，比如 Vercel 风格的 lyra-light 用 `#000`）
- 注册仪式：`name`、`version`、`setup({ host }) { host.extensions.contribute(THEME, …) }`

文件从原来的 ~95 行（含 shadow / CTA / setup 模板）缩到 ~30 行（**纯调色板**）。每个主题都靠它节省 ~60 行。

#### 内置主题

| order | id                   | series      | accent          |
| ----- | -------------------- | ----------- | --------------- |
| 0     | dark                 | Lyra        | `#1ed760` green |
| 1     | light                | Lyra        | `#15883e` green |
| 10    | atom-one-dark        | Atom        | `#528bff` blue  |
| 11    | atom-one-light       | Atom        | `#526fff` blue  |
| 20    | tokyo-night-storm    | Tokyo Night | `#7aa2f7` blue  |
| 21    | tokyo-night-light    | Tokyo Night | `#34548a` blue  |
| 30    | solarized-dark       | Solarized   | `#268bd2` blue  |
| 31    | solarized-light      | Solarized   | `#268bd2` blue  |
| 40    | catppuccin-mocha     | Catppuccin  | `#cba6f7` mauve |
| 41    | catppuccin-macchiato | Catppuccin  | `#c6a0f6` mauve |
| 42    | catppuccin-latte     | Catppuccin  | `#8839ef` mauve |

所有色值来自上游 canonical（`one-dark-syntax` / `enkia/tokyonight` / Ethan Schoonover Solarized / `catppuccin/catppuccin`），没有臆造。

#### 加新主题的步骤

1. `plugins/builtin/<theme-name>/index.ts`：调 `defineThemePlugin({ id, label, scheme, order, palette })`
2. `plugins/builtin/themes/index.ts`：在 `builtinThemes` 数组里加一行
3. Done —— Settings → Appearance 的 theme picker 从 registry 读列表，自动出现新选项

#### 首屏防闪烁

`index.html` 内嵌一段 8 行同步 JS：读 `localStorage["lyra.theme"]`、解析出 id、根据 id 推断 scheme，在 CSS 解析前把 `theme-{scheme}` class 加到 `<html>`。light 用户冷启动不再闪一下黑。tokens.css 里的 `:root` 默认值作为 dark 的 fallback；插件 setup 完成后 inline tokens 接管。

---

## 6. 渲染端：Slot 与各种 useXxx Hook

### 6.1 `<Slot name="..."/>`

`src/plugins/Slot.tsx` 是 kernel ↔ 插件的"插槽桥"。

```tsx
<Slot name="app.sidebar" />
```

内部：

```ts
const specs = useLayoutSlot("app.sidebar");   // hook 订阅 registry.layoutSlots
return specs.map(spec => (
  <PluginBoundary key={spec.id} plugin={...}>
    <spec.component />
  </PluginBoundary>
));
```

特性：

- 按 `order ?? 100` 升序渲染。
- 每个 spec 包一层 `PluginBoundary`（React Error Boundary）—— 单个插件的 render 抛错只是它自己空白，kernel 不挂。
- 默认透明（Fragment），不引入额外 DOM；传 `wrapper=true` 或 `className` 时才包 `<div data-slot=...>`。

### 6.2 其它"消费端" hook

`src/plugins/sdk/registry.ts` 里：

| Hook / 函数                                                        | 用途                                        |
| ------------------------------------------------------------------ | ------------------------------------------- |
| `useToolPreview(fn)`                                               | 工具卡片找展开预览组件                      |
| `useToolActions()`                                                 | 工具卡片头部按钮                            |
| `useWorkspaceViews()`                                              | ChatPanel 解析当前 main view tab 的渲染组件 |
| `useSettingsPanes()`                                               | SettingsPage 左栏                           |
| `useSidebarSections()` / `useSidebarRailItems()`                   | 侧栏内部                                    |
| `useCommands()`                                                    | 命令面板列表                                |
| `useSlashCommands()`                                               | composer slash 提示                         |
| `useComposerModes()` / `useComposerStatus()` / …                   | composer 工具栏                             |
| `useThemes()` / `useAccents()`                                     | Appearance 面板                             |
| `useMessageRole(id)`                                               | MessageBlock 头像 / 名字                    |
| `lookupCoreEventHandlers(type)` / `lookupCustomEventHandler(name)` | reducer 内部用，非 React 选择器             |

---

## 7. 端到端的几个典型流程

### 7.1 用户输入消息发送

```
Composer onKeyDown (Enter)
   → submitComposer(value, clear, sendText)
   → onSend(text) 来自 ChatPanel 的 prop
   → kernel-chat 的 useDefaultChatSession.send
   → useAgentSession.send → agent.addMessage + agent.runAgent
   → AbstractAgent 流出 RUN_STARTED / TEXT_MESSAGE_* / TOOL_CALL_* …
   → subscription.onEvent → useAgentStore.applyEvent
   → reduce → core-reducer 各 onCore handler → 新 state
   → React 订阅者重渲染（MessageStream 等）
```

### 7.2 工具调用展开 / 打开完整视图

```
ChatPanel 渲染 → MessageStream → MessageBlock → PartRenderer
   ─ kind="tool" 分支
      → <ToolCard onOpenView={() => openViewForTool(toolCallId)} />

用户点击 "Open in Terminal" PreviewFoot
   → onOpenView() → state/toolRouting.ts.openViewForTool(toolId)
   → 根据 tool.fn 决定 view id（bash→terminal, edit_file→diff …）
   → ui.setActiveFile(...) 若是文件类工具
   → ui.openMainView({ id, title, icon })
   → uiStore.mainViewTabs 追加 + activeMainView = id
   → ChatPanel.activeViewBody = useWorkspaceViews().find(id).component
   → 主区域换成那个 workspace view（Diff / Terminal / …）
```

### 7.3 Cmd+K 打开 Settings

```
command-palette 插件注册了 Mod+K shortcut + app.overlay 组件
ShortcutsProvider 监听 keydown → 转 normalized combo → 派发 handler
   → usePaletteStore.toggle() → open: true
   → CommandPalette overlay 渲染

用户选 "View: Settings"（由 default-commands 自动从 workspaceViews 生成）
   → cmd.run() = useUIStore.openMainView({ id: "settings", ... })
   → ChatPanel 解析到 settings 这个 workspace view（kernel-settings 注册）
   → 渲染 <SettingsPage />
   → SettingsPage 通过 useSettingsPanes() 拿到 panes 列表
   → 用户在左栏点 Appearance → 渲染 appearance plugin 的 pane component
```

### 7.4 一个 CUSTOM 协议事件落地

后端发 `Custom("lyra.plan", { items: [...] })` →

```
agent.subscribe.onEvent → applyEvent → reducer.reduce
   → EventType.CUSTOM 分支
   → lookupCustomEventHandler("lyra.plan")  →  plan-handler 插件注册的 handler
   → handler(value) 返回 setPlan(items)  —— 来自 sdk/state.ts 的组合子
   → reducer 把 StateUpdate 套到 state 上：state.plan = items
   → useAgentStore 更新
   → Plan workspace view (`workspace-views/plan.tsx`) 用 useAgentStore((s) => s.plan) 读到新 plan
```

---

## 8. 错误隔离策略

| 失败点                          | 行为                                                       |
| ------------------------------- | ---------------------------------------------------------- |
| 插件 `setup` 抛错               | dispose 已注册的部分；其它插件继续；写错误到 PluginsPane   |
| 插件组件 render 抛错            | PluginBoundary 接住，画 fallback；其余 kernel 正常         |
| `onCore` / CUSTOM handler 抛错  | 该 handler 跳过，state 保持进入时的版本；其余 handler 继续 |
| 插件 tool action / command 抛错 | console.error + `reportPluginError`，UI 不挂               |
| AG-UI runAgent 失败             | `onRunFailed` console.error；store 不动；其它会话仍可用    |
| sideload 模块 import 失败       | 跳过这个，其它继续；console.warn                           |
| sideload manifest 抓取失败      | 整批跳过，kernel 正常运行（只有内置）                      |
| beforeunload handler 抛错       | console.error 但不阻塞卸载                                 |

PluginsPane（Settings → Plugins）汇总所有 `reportPluginError` 的红 badge，方便定位是哪个插件的哪个面在出问题。

---

## 9. 怎么写一个插件

最小三件套：

```ts
// my-plugin/index.ts
import { definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.example.hello",
  version: "1.0.0",
  apiVersion: "^1.0.0",            // 可选；不写就接受任意 host
  requires: ["lyra.builtin.default-themes"],   // 可选；依赖（拓扑排序）
  capabilities: ["commands", "agui", "message", "notify", "logger"], // 可选；最小权限

  // 可选：懒加载 — 只有用户在 Cmd+K 选了占位命令才跑 setup
  activationEvents: ["onCommand:hello.world"],
  contributes: {
    commands: [{
      id: "hello.world",
      label: "Hello, world!",
      group: "Examples",
      when: '!mainViewActive',   // 可选；when 子句
    }],
  },

  setup({ host }) {
    // 1. 加一个 Cmd+K 命令
    host.extensions.contribute(COMMAND, {
      id: "hello.world",
      label: "Hello, world!",
      group: "Examples",
      run: () => host.notify("hi", "info"),
    });

    // 2. 监听一个 AG-UI CUSTOM 事件
    host.agui.on<{ text: string }>("example.banner", (value) =>
      appendBlockToLatestAssistant({
        kind: "exampleBanner",
        text: value.text,
      }),
    );

    // 3. 提供该 kind 的渲染器
    host.message.registerContentBlock("exampleBanner", ({ block }) => (
      <div className="banner">{block.text}</div>
    ));

    // 4. 启动时通知一下
    host.lifecycle.onReady(() => {
      host.log.info("hello plugin ready");
    });

    // 5. 可选：subscribe 等副作用通过 setup 返回 cleanup 函数处理
    //    在 host.plugins.unload(name) 时自动跑
    const unsubscribe = someStore.subscribe(/* ... */);
    return () => unsubscribe();
  },
});
```

> 内置插件下：放到 `frontend/src/plugins/builtin/<name>/index.ts(x)`，并在 `builtin/index.ts` 的合适分组里 import + 加进数组。
> 外置插件下：构建成 ESM `index.js`，把 React/motion/SDK 标 external，去引用 `window.__LYRA__.{React, Motion, SDK}`；放到后端 sideload 目录。

**自定义内容块的类型注册**（让 TS 满意）：

```ts
// 在自己的 index.ts 内
declare module "@/protocol/agui/viewState" {
  interface CustomContentBlockMap {
    exampleBanner: { kind: "exampleBanner"; text: string };
  }
}
```

---

## 10. 不变量速查

- **Kernel 不知道任何具体功能**——所有看得见的元素都来自插件。改一处功能 = 改一个插件目录。
- **registry 是唯一真相**——不要直接 import 一个内置插件去用，永远走 `useXxx` / `lookupXxx`。
- **store 是单 Zustand instance**——多个 selector 订阅，refresh 粒度由 React 自己处理；不要把 store 包到 context 里。
- **AG-UI 事件单向流入 view state**——render 路径上不要回写 agent store；想"做事"就调 store 上的 send/stop。
- **`setup` 同步注册，懒构建子组件**——`setup({ host })` 内不要做 `await fetch(...)`；要拉数据用 `host.extensions.contribute(DATA_PROVIDER, …)` + 让 React Query 在 render 时跑。
- **Disposable 一律由 Host 收集**——别手动调 `dispose()`，让 plugin 失败回滚机制管理。
- **API breaking 改动要碰 `apiVersion.ts`**——任何破坏 Host 接口或 spec 形状的改动，应该 bump major；插件用 `apiVersion: "^X"` 自我保护。

---

## 11. 进一步的阅读路径

| 想了解                             | 先看                                                                                                    |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------- |
| 视觉规范 / 颜色 / 排版             | `DESIGN.md`                                                                                             |
| Host 全部接口                      | `src/plugins/sdk/types/host.ts`                                                                         |
| 各类 Spec 类型                     | `src/plugins/sdk/types/<domain>.ts`（按 domain 拆 13 个）                                               |
| Registry 形状 + composite key      | `src/plugins/sdk/registry.ts`                                                                           |
| 插件能力上限 / 演进方向 / 横向对比 | `docs/PLUGINS_CEILING.md`                                                                               |
| 一个完整的内置插件                 | `src/plugins/builtin/demo/index.tsx`                                                                    |
| 主题如何注册                       | `src/plugins/builtin/themes/defineThemePlugin.ts` + 任意 `<theme>/index.ts`                             |
| AG-UI 数据 fold                    | `src/protocol/agui/reducer.ts` + `src/plugins/builtin/core-reducer/index.ts`                            |
| ChatPanel 怎么把一切串起来         | `src/components/chat/ChatPanel.tsx`（orchestrator）+ `PanelHeader` / `ChatStream` / `WorkspaceViewBody` |
| Store 拆分                         | `src/state/themeStore.ts` / `layoutStore.ts` / `sessionStore.ts`                                        |
| 路由动态构建                       | `src/router.tsx`                                                                                        |
| Sideload 入口                      | `src/plugins/sideload.ts` + `src/plugins/hostBridge.ts`                                                 |

---

## 12. 改进方向（forward-looking analysis）

下面这份清单是**有依据的**而不是 wishlist —— 每条都标了"做的理由 / 不做的理由 / 触发条件"，避免 backlog 变成永远不收敛的"理想架构"幻象。

### 12.1 值得做（有明确收益、风险可控）

#### A. 给 `core-reducer` 补全各 handler 的语义集成测试

**现状**：`plugins/builtin/core-reducer/index.ts` 已经是约 16 行薄壳，真正的逻辑拆进了 `core-reducer/handlers/` 子目录（activity / messages / reasoning / run / state / text / tool 等，合计约 620 行），按事件类型分文件，每个 handler 处理一类 AG-UI 内置事件（RUN*\*、TEXT_MESSAGE*_、TOOL*CALL*_、REASONING*\*、STEP*_、STATE\__、ACTIVITY\_\*）。文件级拆分已完成，剩下的缺口是**逐 handler 的语义测试覆盖**。

**为什么没做全**：协议语义层。改错了流式渲染会挂，目前 `reducer.test.ts` 只覆盖 dispatcher 层（"事件路由到哪个 handler"），handler 子目录里只有 activity / reasoning / tool 有专项测试，messages / run / state / text 等的具体语义还没补齐。

**怎么做**（先决条件）：

1. 给每个内置事件类型写一组 input event → expected state delta 的快照测试（参考 redux-toolkit 的 reducer 测试模式），补齐尚未覆盖的 handler
2. 有了全套语义测试后，再考虑后续重构（如查表派发）时可零回归验证

**触发条件**：要加新的内置事件类型时（比如 `THINKING_*` / `MEMORY_*`）一并补上对应 handler 的测试。

#### B. ChatPanel/MessageStream 的视觉回归测试

**现状**：ChatPanel 已经拆成 4 个子组件（约 50 行 orchestrator + PanelHeader/ChatStream/WorkspaceViewBody），单文件没法继续小拆。但视觉回归（"切 tab 时 tab strip 是否正确"、"打开 workspace view 时 composer 是否消失"）目前靠手测。

**为什么没做**：DOM 集成测试容易脆弱，Playwright/Storybook 是更大的引擎引入。

**触发条件**：tab 状态机改三次以上 / 引入第一次回归 bug。届时 ROI 翻正。

#### C. plugin sideload 的端到端测试

**现状**：sideload 路径（`sideload.ts` + `hostBridge.ts`）只有单元测试，没有"真的从 Go 后端下载 + dynamic import + 注册"的 e2e 覆盖。
**为什么没做**：mocking dynamic import 比较奇技淫巧，工程量大。
**触发条件**：sideload 路径出第一个真实 bug / 第一个外部 plugin 进入仓库。

#### D. 给 `useSmoothText` hook 加最小行为测试

**现状**：`pickRate` 纯函数有 8 个测试（`smoothText.test.ts`），但 hook 本身（rAF + 词分段 + 句末停顿 + drain mode）零测试。
**为什么没做**：rAF 在 happy-dom 下需要 stub + fake timers，hook 行为随性能特征调，测试容易跟 vsync jitter 打架。
**触发条件**：流式渲染速率出 bug / pickRate 重新设计时一起做。

### 12.2 想做但当前 KISS / YAGNI 不允许

#### E. ✅【已做，方向反了】把 `registry.ts` 的 per-slot `addX/removeX` 收敛

**结果**：不是抽 factory，而是**整体塌进开放扩展点底座**（L2+L3，2026-06）。40 个命名 owned-map → 单一 `extensions` map，贡献统一走 `host.extensions.contribute(POINT, …)`，~24 个纯 spec-register host facade 删除。`addOwnedMulti` 已删。当初"clarity loss > LOC saving"的判断在"加 factory"这条路上成立，但底座方案用 typed `ExtensionPoint<T>` handle 保住了类型推断、同时消除了 per-slot map/action/工厂三处样板。**权威文档见 `docs/EXTENSION_POINTS.md`。**

#### F. 把 `html.theme-light .foo` 结构性 override 全部 token 化

**为什么不做**：`styles/theme.css` 里剩下约 200 行 `html.theme-light .panel { border: none }` 这类规则 —— 改的是 CSS rule 而不是 token 值，没法用 inline style 表达。要全部 token 化需要：

- 给每条 rule 找到对应的 "新 token"（`--panel-border`、`--panel-shadow-policy`、…）
- 在 base CSS 里改成 `var(--token)`
- 主题 plugin 写出这些新 token

**触发条件**：第三方主题作者需要修改这些"结构性"差异。在那之前 `theme-{scheme}` class 已经够用。

#### G. 把 `domain/infra/main/` 拆成 monorepo packages

**为什么不做**：见 §3.2 的现有 4 个触发条件，目前一个都没命中。TS path alias + `architecture.test.ts` 已经强制了同样的边界约束。

#### H. plugin marketplace 元数据 / 签名

**为什么不做**：`apiVersion` 闸已经在；marketplace 概念目前只是设想。等出现真的第三方 plugin 提交时再设计。**纯 YAGNI**。

### 12.3 可能值得做的"隔壁优化"（非重构）

#### I. 国际化（i18n）

**现状**：所有 UI 文案 hardcode 中英文混合（command label / empty state / 注释）。
**判断**：单语言用户没有痛点；但 ChatPanel/Composer 这些核心 UI 的文案应该走 i18n 框架以便未来扩展。引入 `@lingui/macro` 或 `react-intl` 是几小时活，但**重构 200+ 字符串到字典是数天**。

**触发条件**：第一个非中英文用户 / 第一个 PR 想加日文。

#### J. 性能：MessageStream 虚拟化

**现状**：消息流用普通 React 列表 + `use-stick-to-bottom`。
**判断**：长会话（1000+ 消息）目前没人抱怨；流式刚开始的消息密度不高。

**触发条件**：实际遇到 > 500 消息会话卡顿时，引入 `@tanstack/react-virtual`。

#### K. 后端：把 mock SSE server 替换成可插拔的 agent provider

**现状**：`internal/agui/` 全是 fixture demo 数据；真要接 LLM 还得改一遍。
**判断**：当前定位是 UI 设计预览 + 协议验证；真接入 LLM 是一个**新阶段**，不属于"前端架构改进"。

**触发条件**：进入"真实 agent 集成"阶段（与本文档无关）。

### 12.4 优先级建议

**下一轮**：

- 暂时**无明确的下一步**。§12.6 八条已落实/盘点完毕：A 已做、B/C/G YAGNI、D 已存在、E/F/H 都需要产品路线决策才能启动。
- 当前架构通过所有审计原则，LOC 在合理范围，所有热路径有测试覆盖。**继续等触发条件出现**（详见 §12.6 各项的"触发条件"），不要做投机式重构。

**再下一轮**（如果有 1 周）：

1. §12.6 H（MetaEvent 反馈统一，~6h）—— RLHF 数据基础设施，看产品路线
2. §12.1 A（core-reducer table-driven 重构，需要先补测试）

如果只是日常维护：维持现状。当前架构通过了所有审计原则（KISS / SOLID / YAGNI / DRY），LOC 在合理范围，所有热路径有测试覆盖。**继续等触发条件出现**，不要做投机式重构。

### 12.5 反向不变量（强意见弱依据）

下面这些方向**已知会被人提出**，但当前判断是错的方向，记下来避免重复辩论：

- **❌ Redux Toolkit / Effector / Jotai 替换 Zustand**：现有 store 都很小，订阅模型够用；切换框架是纯切换，无收益。
- **❌ 给所有 plugin 加 schema 验证（Zod）**：plugin spec 由 TS 类型守护，runtime 验证只在 sideload 边界有意义 —— 加重 build size 没好处。要做也是只在 sideload 入口加。
- **❌ 把 React Query → SWR / RTK Query**：同上，切换框架本身没有收益。
- **❌ 把 Wails → Tauri**：桌面壳兼容性问题，没有实际 bug 也没好处。
- **❌ 自研响应式框架替代 Zustand**（assistant-ui TAP 的教训）：`useAui` 200+ 行 boilerplate 只为 scope-based 细粒度订阅，学习曲线陡，**保留 Zustand**，需要 scope 时加 selector 而不是换 store。
- **❌ Protocol multiplexing**（continue 的 `onWebview / onCore / onWebviewOrCore` 三套 handler）：Lyra 坚持 Go ↔ Frontend 单一 Wails RPC，IDE-specific 行为在 handler 内部条件判断，不要 fork protocol。
- **❌ Interrupt / Approval UI 硬编码进消息组件**（agent-chat-ui 的耦合）：通过 Slot 注入（`message-footer` / `inline-overlay`）让 plugin 独立贡献 interrupt UI，reducer 只管数据。
- **❌ Webview 初始化混杂 HTML/CSP/端口逻辑**（cline 的 WebviewProvider）：Wails 已经把模板 + dev server + 资源加载分离，**保持现状**，不要把任何运行时逻辑塞回 HTML 模板。

### 12.6 外部仓库对比 → 候选行动项（2026-05-26 调研）

横向对比 `assistant-ui` / `continue` / `cline` / `ag-ui-protocol` / `agent-chat-ui` 5 个仓库后，挖出 5 大共识主题。共识强 = 业界已收敛 = 值得做；单源 = 看场景。

**5 大共识主题**：

| #   | 主题                                                                                         | 共识源               | Lyra 现状                                                                          |
| --- | -------------------------------------------------------------------------------------------- | -------------------- | ---------------------------------------------------------------------------------- |
| 1   | **Block-level `status` 状态机**（`running \| complete \| incomplete \| requires-action`）    | assistant-ui / cline | `blocks[]` 已 block-based，但只有 `streaming: boolean`，无法表达 `requires-action` |
| 2   | **`<ToolPrimitive>` headless 组件**（props 方法注入：`onApprove` / `onReject` / `result`）   | assistant-ui / cline | approval 走 hook + reducer dispatch，每个 block 自己 wire                          |
| 3   | **Capability discovery + state compaction**（`/api/capabilities` + DELTA 周期合并 SNAPSHOT） | ag-ui 官方           | 前端硬假设 backend 支持所有 event；timeline 不 compact                             |
| 4   | **History 元数据与消息体分离**                                                               | cline                | sessionStore (meta) + agentStore (view) 已分层 ✓；messages 落盘待做                |
| 5   | **Markdown 完整能力**（LaTeX + GFM 表格）                                                    | agent-chat-ui        | shiki 高亮已有，缺 LaTeX 公式 + 表格 CSS                                           |

**8 个候选行动项**（按 ROI / 投入排序）：

| #   | 行动项                                                    | 共识强度 | 投入     | 收益                 | 状态                                            |
| --- | --------------------------------------------------------- | -------- | -------- | -------------------- | ----------------------------------------------- |
| A   | Block-level `status` 字段 + approval 状态机               | ⭐⭐⭐   | 中 (~4h) | 高                   | ✅ **已做**（2026-05-26）                       |
| B   | `<ToolPrimitive>` headless 组件 + button config map       | ⭐⭐⭐   | 中 (~3h) | 高                   | ⏸ **YAGNI 推迟** — 见下方"为什么 B 没做"        |
| C   | `/api/capabilities` 端点 + 前端 UI gating                 | ⭐⭐     | 小 (~2h) | 中                   | ⏸ **YAGNI 推迟** — 见下方"为什么 C 推迟"        |
| D   | LaTeX (`remark-math + rehype-katex`) + GFM 表格 CSS       | ⭐⭐     | 小 (~1h) | 中                   | ✅ **已在做**（pre-existing）— 见下方"D 的发现" |
| E   | State compaction（DELTA chain 周期性合并为 SNAPSHOT）     | ⭐⭐     | 中 (~3h) | 低（dev 期数据量小） | P2                                              |
| F   | Generative UI spec + allowlist（agent 返回 JSON 描述 UI） | ⭐       | 大 (~8h) | 中（看路线）         | P2                                              |
| G   | 配置树 + workspace overrides（continue `mergeJson`）      | ⭐       | 大 (~6h) | 低（单用户桌面）     | × 暂不                                          |
| H   | MetaEvent 统一用户反馈（thumbs/note/bookmark）            | ⭐⭐     | 大 (~6h) | 高（RLHF）           | P1（看产品路线）                                |

**为什么 B 没做**：实施时 grep 了 `content-blocks/` 才发现 Lyra **只有 `approval` 一个真正 actionable 的 block**（tool 块是只读指针，code / search 是被动展示）。给单一消费者抽 `<ToolPrimitive>` 违反 CLAUDE.md "3+ 重复才抽象" 原则——属于 premature abstraction。`ApprovalCard` + `useApprovalSubmit` 已经把 HTTP / UI 分得很干净，下一个 actionable block 出现时再抽 primitive 也来得及。**触发条件**：第二个 actionable block 出现时（如 code-proposal 升级为 accept/reject、或 interrupt-block 落地）。

**为什么 C 推迟**：Lyra 是单进程 Wails 应用（Go + React 一起 build），不存在"backend version drift"风险；UI 已经是**懒渲染**——只有事件流到了才会渲染对应 block（reasoning / activity 都是这样），capability discovery 没有真实消费者。只有出现以下场景才有价值：多 agent source 并存（OpenAI / Anthropic / local LLM）、外部 agent provider 接入、plugin sideload 加载第三方 agent。**触发条件**：`AgentSourceSpec` 超过一个 built-in 注册时，给它加 `capabilities?: AgentCapabilities` 字段 + `useAgentCapabilities()` selector。

**D 的发现**：原计划基于 agent-chat-ui 报告说"Lyra 缺 LaTeX / GFM 表格"。实际看 `components/chat/MarkdownMessage.tsx` 才发现 Lyra **早就有**：`remarkGfm`（表格 + 删除线 + 任务列表 + autolinks） / `remarkMath + rehypeKatex`（LaTeX 公式）/ `ensureKatexCss()` 懒加载 KaTeX 样式 / `markdownComponents.tsx` 自定义 `<table>` 包成 `md-table-wrap` 横向滚动容器 / `styles/markdown.css` 完整表格 CSS。**Lyra 的 Markdown pipeline 严格强于 agent-chat-ui 的**（多了 streamdown block 拆分、自定义 citation、shiki 高亮、fade-in 动画）。这条 audit 是外部 agent 没深挖到的伪缺口。

**触发条件**：A + B 同时命中两个共识源，且 P0 块状态信号在 approval / interrupt UX 升级时**必经**，应在下一波重构里执行（即 §12.4 优先级建议下一轮直接做 A + B）。C / D 是 quick win，可以搭车做。

**5 个仓库都为 Lyra 现状盖章的设计**（"做对了别动"）：

- ✅ **Plugin reducer 模式**：ag-ui 官方明确——协议层不规定 reducer 实现，应用自由组织
- ✅ **`internal/agui/events.go` 嵌入 + ToJSON override**：ag-ui agent 评价为"不侵入 SDK 的优雅扩展法"
- ✅ **Zustand 多 store 分层**：和 assistant-ui scope 树同思路，但更轻
- ✅ **`definePlugin({ host })` SDK**：跟 cline "10 个设计缝隙"思想一致
