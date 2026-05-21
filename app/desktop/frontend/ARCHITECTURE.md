# Lyra 前端架构

> 本文档描述 `frontend/` 这个 React + TypeScript 应用是怎么组织、怎么运行的。
> 设计系统 / 视觉规范请看 `DESIGN.md`。

---

## 1. 一句话概括

**Lyra 前端 = AG-UI 流式协议 + 插件化 React 外壳**。

外壳几乎不长肉——所有路由、布局、内容渲染、命令、快捷键、主题、AG-UI 事件处理、设置面板都由"插件"贡献。Kernel 自己只是一组命名 Slot 加几个共享 store；插件通过一个统一的 `Host` API 往 Slot 里塞组件、往 reducer 里挂事件 handler、往 Zustand 注册的 registry 里写自己的数据。

---

## 2. 技术栈

| 层 | 选型 |
| --- | --- |
| UI | React 18 + TypeScript |
| 状态 | Zustand（多 store，无 context 链） |
| 路由 | TanStack Router（route tree 动态构建） |
| 数据 | TanStack React Query |
| 协议 | `@ag-ui/core` / `@ag-ui/client`（AbstractAgent 事件流） |
| 动画 | motion/react |
| 桌面壳 | Wails v2（Go 后端 + WebKit/Chromium 前端） |
| 测试 | Vitest + Testing Library |

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
│   └── AgentClientPage.tsx   kernel：sidebar / main / statusbar / overlay 四个 Slot
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
│   │   ├── types.ts          所有 *Spec 类型 + Host 接口
│   │   ├── host.ts           createHost(pluginName) 返回绑定的 Host 实例
│   │   ├── registry.ts       usePluginStore — 集中存放所有插件贡献的 Map
│   │   ├── definePlugin.ts   loadPlugin / loadPlugins
│   │   ├── state.ts          AG-UI state 更新组合子（appendBlock…/patchRun…）
│   │   ├── storage.ts        每插件 namespaced localStorage
│   │   ├── stateSlice.ts     插件共享 state slice
│   │   ├── config.ts         全局 config store
│   │   ├── errors.ts         插件错误聚合（PluginsPane 显示）
│   │   ├── notifications.ts  持久化通知 feed
│   │   └── apiVersion.ts     HOST_API_VERSION 常量
│   │
│   └── builtin/              内置插件，按领域分组到 25 个目录
│       ├── index.ts          manifest（topo sort 由 spec.requires 驱动）
│       ├── core-reducer/     AG-UI 内置事件 → view state
│       ├── kernel/           填 app.sidebar / app.main / Settings 三个核心 slot
│       ├── composer/         撰写区 7 个 plugin（modes / toolbar / send / …）
│       ├── sidebar/          侧栏 8 个 plugin（brand / sessions / rail / …）
│       ├── workspace-views/  6 个 workspace view（Diff/Files/Terminal/Plan/Tools/Notifications）
│       ├── content-blocks/   6 个消息内容块（plan/code/approval/…）
│       ├── agui-handlers/    5 个 CUSTOM 事件 → state 的 handler
│       ├── tool-previews/    4 个工具 inline preview（bash/diff/file/grep）
│       ├── tool-meta/        tool actions + tool icons
│       ├── defaults/         默认 commands / config / data / themes / roles / title
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
│   ├── uiStore.ts        theme / sidebarRail / mainViewTabs / activeFile …
│   ├── composerStore.ts  撰写区文本 + 模式 + 附件
│   ├── useAgentSession.ts AG-UI Agent 生命周期 hook
│   ├── useDefaultChatSession.ts  从 agentSource registry 挑选 agent
│   └── toolRouting.ts    openViewForTool(toolId) — tool card 点击的路由
│
├── components/           共享 UI 组件（不是插件）
│   ├── common/           Icon / Panel / Chip / ScrollArea / EmptyState / Skeleton
│   ├── chat/             ChatPanel / Composer / MessageStream / PartRenderer
│   ├── tools/            ToolCard / ToolPreview / previews/
│   ├── views/           DiffView / Terminal / FilesChanged / McpRow / PlanList / ViewHeader
│   ├── settings/         SettingsPage（workspace view 主体）
│   ├── sidebar/          类型 + Avatar
│   └── icon-gallery/     IconGallery / IconShowcase
│
├── lib/                  共享工具：queries.ts / http.ts / queryClient.ts / motion.ts
├── utils/                inline 渲染、TS 简易语法高亮
├── styles/app.css        承载主体样式
└── test/                 测试 setup
```

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

`src/pages/AgentClientPage.tsx` 全文不到 30 行：

```tsx
<div className={`app ${sidebarRail ? "rail" : ""}`}>
  <div className="app-main">
    <Slot name="app.sidebar" />
    <Slot name="app.main" />
  </div>
  <div className="app-statusbar">
    <Slot name="app.statusbar" />
  </div>
  <Slot name="app.overlay" />
</div>
```

四个 Slot 就是 kernel 的全部肉：

| Slot | 典型贡献者 |
| --- | --- |
| `app.sidebar` | `kernel-sidebar` |
| `app.main` | `kernel-chat`（ChatPanel） |
| `app.statusbar` | `status-pill` |
| `app.overlay` | `command-palette` / `toaster` / `shortcuts` |

---

## 5. 三大支柱

### 5.1 插件系统（最大）—— Plugin SDK + Registry

#### 数据流：注册 → 存储 → 订阅 → 渲染

```
PluginSpec.setup({ host })
       │
       │  host.<面>.register*(spec)
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

#### Host 接口（命名分组）

`Host` 接口 (`src/plugins/sdk/types.ts`) 是插件能调的"动词"集合。按面分组：

| Host 面 | 干什么 |
| --- | --- |
| `host.tool` | 注册工具预览组件 / 操作按钮 / 图标 |
| `host.message` | 注册内容块渲染器、消息角色 |
| `host.agui` | 订阅 AG-UI 协议事件（onCore / on） |
| `host.layout` | 把 component 塞进命名 Slot |
| `host.workspace` | 注册可作为主区域 tab 打开的 view |
| `host.theme` | 注册主题 / 强调色 |
| `host.router` | 贡献路由 |
| `host.composer` | 撰写区扩展：slash 命令 / 模式 / 占位符 / 状态 chip / 附件源 / 键绑定 |
| `host.sidebar` | 侧栏分区 / rail 项 |
| `host.shortcuts` | 全局键盘快捷键 |
| `host.agent` | 注册 AG-UI agent source（按优先级选） |
| `host.data` | 注册 React Query fetcher |
| `host.commands` | 注册 Cmd+K 命令 |
| `host.settings` | 注册 Settings 内的左栏面板 |
| `host.lifecycle` | onReady / onBeforeUnload |
| `host.logger` | 结构化日志 |
| `host.rpc` | 拦截 HTTP 调用（before / after） |
| `host.storage` | 每插件 namespaced kv store |
| `host.config` | 全局 config store 读写 |
| `host.state` | 跨插件共享 state slice |
| `host.notify` | 推一条通知到 feed + transient toast |

每个 `register*` 都返回一个 `Disposable`。`createHost` 把这些 disposable 收集到 `setup` 期间的 sink 数组里——一旦 plugin setup 抛错，会自动 dispose 已注册的部分，避免半成品挂在 registry 里。

#### Registry 内部结构

`usePluginStore` (`src/plugins/sdk/registry.ts`) 是一堆 `Map<key, { pluginName, value }>` 的 Zustand store。两种 key 形式：

- **单 owner**：`toolPreviews: Map<fn, Owned<Component>>` — 同一个 fn 只能有一个 preview，重复注册会有 console 警告，**后来者覆盖**。
- **复合键 / 多 owner**：`coreEventHandlers: Map<"${eventType}|${plugin}|${id}", Owned<Handler>>` — 多 handler 链式执行。

通过把 `pluginName` 嵌进 value，dispose 的时候只删本插件那条，不会误伤其他插件用同样 key 注册的条目（兜底，实际很少冲突）。

#### 加载与卸载

`loadPlugin(spec)` (`src/plugins/sdk/definePlugin.ts`):

1. 校验 `apiVersion`（`compare-versions.satisfies`）；不兼容跳过。
2. 创建一个 disposable sink + 绑定到 `pluginName` 的 Host。
3. `await spec.setup({ host })`。
4. **成功** → `registerLoaded({ spec, disposables })`，PluginsPane 显示一行。
5. **失败** → 把 sink 里已注册的 disposable 全部 dispose，调用 `reportPluginError(name, "setup", err)`，**不抛出**——其它插件继续加载。

`reportPluginError` 写到 `usePluginErrorStore`；Plugins 设置面板用红色 badge 展示，提供"Clear"。

#### 内置 vs 外置（sideload）

| | 内置 | 外置 |
| --- | --- | --- |
| 来源 | 同 bundle 静态 import | Go 后端 `/agui/plugins` + dynamic `import(url)` |
| 加载时机 | 启动前同步串行 | 启动后异步 |
| 阻塞首屏 | 是（设计如此） | 否 |
| origin 标记 | `builtin` | `sideload`（PluginsPane 显示徽章） |
| 共享 React | 同 bundle 自然共享 | 通过 `window.__LYRA__` 桥接（不需要自带 React） |

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

| 形式 | 含义 |
| --- | --- |
| `mainViewActive` | 标识符；context map 里查值，真值视为 true |
| `!mainViewActive` | 取反 |
| `mainView == "diff"` | 字符串等值 |
| `a && b` / `a \|\| b` | 逻辑组合，`&&` 优先级高于 `\|\|` |
| `(a \|\| b) && c` | 括号分组 |

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

`makeAgent` 由 `useDefaultChatSession` 从 `host.agent.registerSource(...)` 注册的 agent source 里挑（priority 最高的）。内置 `http-agent` 走 HTTP；插件可以替换成 mock、IPC、本地模型等。

---

### 5.3 状态管理（除 agent 之外的 UI 状态）

| Store | 内容 | 持久化 |
| --- | --- | --- |
| `useAgentStore` | AgentViewState + 当前 agent 的 send/stop 引用 | ❌ 每次会话重置 |
| `useUIStore` | theme / accent / sidebarRail / 当前 session / 打开的 chat tab / 主区 workspace view tab / activeFile / 工具选中态 | ✅ 部分字段 |
| `useComposerStore` | textarea 文本 + 模式 + 附件 | ❌ |
| `usePluginStore` | 整个插件 registry | ❌ |
| `useConfigStore` | 插件可读写的全局 config（如 `api.baseUrl`） | ✅ |
| `useNotificationStore` | host.notify 推过的持久 feed | ❌ |
| `usePluginErrorStore` | 插件错误聚合 | ❌ |

uiStore 用 Zustand `persist` 中间件序列化白名单字段到 `localStorage`，版本号变化时直接丢弃旧数据避免形状错配。

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

| Hook / 函数 | 用途 |
| --- | --- |
| `useToolPreview(fn)` | 工具卡片找展开预览组件 |
| `useToolActions()` | 工具卡片头部按钮 |
| `useWorkspaceViews()` | ChatPanel 解析当前 main view tab 的渲染组件 |
| `useSettingsPanes()` | SettingsPage 左栏 |
| `useSidebarSections()` / `useSidebarRailItems()` | 侧栏内部 |
| `useCommands()` | 命令面板列表 |
| `useSlashCommands()` | composer slash 提示 |
| `useComposerModes()` / `useComposerStatus()` / … | composer 工具栏 |
| `useThemes()` / `useAccents()` | Appearance 面板 |
| `useMessageRole(id)` | MessageBlock 头像 / 名字 |
| `lookupCoreEventHandlers(type)` / `lookupCustomEventHandler(name)` | reducer 内部用，非 React 选择器 |

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

| 失败点 | 行为 |
| --- | --- |
| 插件 `setup` 抛错 | dispose 已注册的部分；其它插件继续；写错误到 PluginsPane |
| 插件组件 render 抛错 | PluginBoundary 接住，画 fallback；其余 kernel 正常 |
| `onCore` / CUSTOM handler 抛错 | 该 handler 跳过，state 保持进入时的版本；其余 handler 继续 |
| 插件 tool action / command 抛错 | console.error + `reportPluginError`，UI 不挂 |
| AG-UI runAgent 失败 | `onRunFailed` console.error；store 不动；其它会话仍可用 |
| sideload 模块 import 失败 | 跳过这个，其它继续；console.warn |
| sideload manifest 抓取失败 | 整批跳过，kernel 正常运行（只有内置） |
| beforeunload handler 抛错 | console.error 但不阻塞卸载 |

PluginsPane（Settings → Plugins）汇总所有 `reportPluginError` 的红 badge，方便定位是哪个插件的哪个面在出问题。

---

## 9. 怎么写一个插件

最小三件套：

```ts
// my-plugin/index.ts
import { definePlugin } from "@/plugins/sdk";

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
    host.commands.register({
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
      host.logger.info("hello plugin ready");
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
- **`setup` 同步注册，懒构建子组件**——`setup({ host })` 内不要做 `await fetch(...)`；要拉数据用 `host.data.registerProvider` + 让 React Query 在 render 时跑。
- **Disposable 一律由 Host 收集**——别手动调 `dispose()`，让 plugin 失败回滚机制管理。
- **API breaking 改动要碰 `apiVersion.ts`**——任何破坏 Host 接口或 spec 形状的改动，应该 bump major；插件用 `apiVersion: "^X"` 自我保护。

---

## 11. 进一步的阅读路径

| 想了解 | 先看 |
| --- | --- |
| 视觉规范 / 颜色 / 排版 | `DESIGN.md` |
| Host 全部接口 | `src/plugins/sdk/types.ts` |
| Registry 形状 + composite key | `src/plugins/sdk/registry.ts` |
| 一个完整的内置插件 | `src/plugins/builtin/demo/index.tsx` |
| AG-UI 数据 fold | `src/protocol/agui/reducer.ts` + `src/plugins/builtin/core-reducer/index.ts` |
| ChatPanel 怎么把一切串起来 | `src/components/chat/ChatPanel.tsx` |
| 路由动态构建 | `src/router.tsx` |
| Sideload 入口 | `src/plugins/sideload.ts` + `src/plugins/hostBridge.ts` |
