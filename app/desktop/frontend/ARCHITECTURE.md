# Lyra 前端架构

> 本文档描述 `frontend/` 这个 React + TypeScript 应用是怎么组织、怎么运行的。
> 主 UI 心智模型看 [`../docs/FRONTEND_AGENT_WORKSPACE_MODEL.md`](../docs/FRONTEND_AGENT_WORKSPACE_MODEL.md)；
> 设计系统 / 视觉规范看 `DESIGN.md`；决策透镜 / 工程约定看仓库根的 `CLAUDE.md`；
> 协议权威定义看 `app/runtime/doc/API.md` + `app/runtime/doc/AUX_API.md` + `app/runtime/doc/TRANSPORT.md`。
>
> **分工**：`CLAUDE.md` 讲"怎么判断"（决策与硬约定），本文讲"系统长什么样"（结构与运行）。两者尽量不重述。

---

## 1. 一句话概括

**Lyra 前端 = 自研 Lyra Runtime Protocol v2 流式协议 + 插件化 React 外壳。**

外壳几乎不长肉——路由、布局、内容渲染、命令、快捷键、主题、协议事件处理（StreamEvent fold）、设置面板，全部由内置插件贡献。Kernel 由 dougong `Host` 的契约图与生命周期、Lyra 的开放扩展点策略和少量共享 Zustand store组成；插件经 `PluginContext` 写贡献，经 typed selector 消费稳定 read model。

---

## 2. 技术栈

| 层       | 选型                                                              |
| -------- | ----------------------------------------------------------------- |
| UI       | React 19 + TypeScript                                             |
| 样式     | Tailwind 4 + `cva` + `clsx` + `tailwind-merge`（`cn()`）          |
| Headless | Base UI primitives first（Dialog / Popover / Menu / Tooltip / …） |
| 特定件   | `cmdk`（命令面板）/ `sonner`（Toast）/ `lucide-react`（图标）     |
| 状态     | Zustand（多 store，无 context 链）                                |
| 路由     | TanStack Router（route tree 动态构建）                            |
| 数据     | TanStack React Query                                              |
| 协议     | 自研 Lyra Runtime Protocol v2（JSON-RPC 2.0，`src/rpc/`）         |
| 动画     | motion/react                                                      |
| 桌面壳   | Wails v3 beta（Go 后端 + WebView 前端，版本钉死）                 |
| 测试     | Vitest 4 + Testing Library + happy-dom                            |
| 构建     | Vite 8（内置 Rolldown bundler）                                   |
| Lint     | OxLint 1.x（Rust-based）；`prettier` 格式化                       |

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
│   │   ├── PluginProvider.tsx    启动编排与代际 owner：Host.start → ready → stop
│   │   ├── Slot.tsx              <Slot name="…"/> 渲染注册到该 slot 的插件组件
│   │   ├── PluginBoundary.tsx    每个插件组件的 React Error Boundary
│   │   ├── PluginContentBlock.tsx 包装消息内容块的边界
│   │   ├── PluginToaster.tsx     全局 toast 层（sonner）
│   │   └── ShortcutsProvider.tsx 全局键盘快捷键派发
│   │
│   ├── sdk/                  插件平台
│   │   ├── types/                17 个 domain 文件 + barrel（按贡献面拆）
│   │   ├── kernelPoints.ts       ~35 个内置 ExtensionPoint（THEME / COMMAND / LAYOUT_SLOT / …）
│   │   ├── contracts.ts          dougong token + Lyra key/read policy
│   │   ├── definePlugin.ts       绑定 dougong context 与 Lyra contribute policy
│   │   ├── bootstrap.ts          Host create/start/stop + installation transaction
│   │   ├── kernel.ts             当前 Host 代际、ContributionView cache 与安装 read model
│   │   ├── services.ts / shellServices.ts  typed shell capability contracts
│   │   ├── selectors/            按面分组的 useXxx / lookupXxx + extensions.ts（读侧底座 + O(1) 索引）
│   │   └── config.ts / storage.ts / notifications.ts / errors.ts
│   │
│   └── builtin/              内置插件，按领域（限界上下文）分组
│       ├── index.ts          manifest（依赖由 requires / provides contract graph 驱动）
│       ├── agent/            agent ports bootstrap · fold（StreamEvent→view state）· rpc-agent
│       ├── chat/             composer · message/(渲染 ui/ + public) · message-actions · plan-progress ·
│       │                     slash-hints · chat-search · preview-blocks · file-references ·
│       │                     tools/(meta + previews + ui/)
│       ├── command/          command-palette · global-keymap · shortcuts
│       ├── defaults/         默认 commands / data / accents / roles / title
│       ├── i18n/             locales pack（8 语言）
│       ├── navigation/       Work Index read model（projects/sessions/attention）
│       ├── observability/    OTel 生命周期插件
│       ├── runtime/          连接配置 · capability discovery/read model
│       ├── settings/         appearance · personalization · connection-settings ·
│       │                     plugins-pane · providers · icon-gallery
│       ├── shell/            纯框架：kernel · main-route · status · toaster ·
│       │                     topbar-new-tab · welcome-screen
│       ├── sidebar/          Work Index renderer / footer / rail surfaces
│       ├── theme/            kit（defineThemePlugin helper）+ themes（10+ 主题）
│       └── workspace/        workspace-views · tasks · diagnostics · conversation-export
│
├── plugins/builtin/agent/                          Agent 限界上下文
│   ├── domain/             AgentInput 等业务输入语言
│   ├── application/        fold / input / session / run / HITL 用例
│   ├── adapters/           driver lifecycle / Zustand read model / wire input bridge
│   ├── presentation/       message/tool/HITL/run digest view model
│   └── public/             对其他上下文发布 input / session / run / conversation ports
│
├── plugins/builtin/chat/composer/                  Composer 限界上下文
│   ├── domain/             Draft / Attachment / SendIntent / history archive
│   ├── application/        submit / draft mutation / file mention use case
│   ├── adapters/           composer Zustand adapter + draft port implementation
│   └── public/             draft / submit / history / attachment public facade
│
├── plugins/builtin/navigation/                     Navigation 限界上下文
│   ├── domain/             Work Index / Work Group / Work Session read model
│   ├── application/        projects + sessions + active context projection
│   └── public/             Work Index renderer consumption facade
│
├── plugins/builtin/runtime/                        Runtime 限界上下文
│   ├── domain/             capability published language
│   ├── application/        endpoint policy / discovery / consumer-owned ports
│   ├── adapters/           Host config、typed SDK discovery、capability read model
│   └── public/             endpoint / capability facade
│
├── plugins/builtin/workspace/                      Workspace 限界上下文
│   ├── application/        navigation / tool routing / activity projection
│   ├── adapters/           workspace navigation port adapters
│   ├── events/             runtime workspace event loop + invalidation rules
│   └── public/             navigation / deeplink / sidebar rail facade
│
├── state/                Kernel 共享 store（不承载业务规则）
│   ├── uiStore.ts        主题 / accent / 字体 / motion / sidebarRail（持久化）
│   ├── tasksStore.ts     后台任务
│   ├── paletteStore.ts   命令面板 UI 状态
│   ├── workspaceSurfaceStore.ts  app-global workspace tabs / settings target
│   ├── contextDockStore.ts       session-scoped split / file / tool material
│   └── useWhenContext.ts  build context for `when` clauses
│
├── ui/                   本地 UI kit：primitives(Base UI 防腐层) / atoms / agent 业务原子
│                         页面只消费 atoms 或 agent 原子，不直连 headless 外部库
│
├── lib/                  共享 hook + 纯函数（跨插件共享，不属于上述任一层）
│   ├── agent/            会话用例 hook（useChatSend / useApprovalSubmit / useQuestionAnswer /
│   │                     useCreateSession / …）+ HITL 决策词表 + streamReveal + messageContent
│   ├── data/             React Query 基础设施（dataQuery / queryClient；不放业务模型）
│   ├── i18n/             i18next 接线 + 分词 + 相对时间
│   ├── markdown/         rehype 插件 + shiki + KaTeX（纯 infra）
│   ├── observability/    OTel 三信号（setup/sink/stores/tracing/logBridge）—— 见 §5.5
│   └── classNames.ts / motion.ts / metrics.ts / hmr.ts / systemFonts.ts
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
│   ├── container.ts      按 active endpoint/token 缓存 LyraClient；测试 setContainer 注入
│   └── config.ts         local desktop shell URL / desktop client identity
│
├── styles/               globals.css（Tailwind base + @theme token + keyframes，唯一主样式）
│                         + tool/markdown/overlays/layout.css（只承载无法用 utility 表达的 chrome）
└── test/                 测试 setup
```

### 3.1 单向依赖与 outbound 边界

Lyra 大部分 UI ↔ 数据流已经通过插件系统解耦，真正需要"内外分层"的只有一处：**UI / 状态 / 插件不应直接发起 outbound 副作用**（HTTP、SSE、IPC）。这一层由 **`rpc/`（Runtime Protocol boundary）+ `main/container.ts`（composition root）** 承担。

所有 outbound 都收敛成一个 JSON-RPC 协议客户端，但业务上下文不会直接依赖它：application 定义窄 port，adapter 在组合边界把 port 接到 `getContainer().client()`。协议 DTO 只在 adapter 或明确的 fold 反腐层出现。

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
        │   getContainer() 单例：client() / shell      │
        │   可依赖任何东西                              │
        └────────────────────────────────────────────┘
                          ▲ via getContainer()
        ┌────────────────────────────────────────────┐
        │  builtin context adapters                     │
        │   实现 application ports，经 getContainer()    │
        │   接到 JSON-RPC client                         │
        └────────────────────────────────────────────┘
```

**层依赖规则**（`scripts/check-layers.mjs` + `check-circular.mjs` 强制，alias-aware，`npm run check` 跑）：

- `rpc/` 是独立层：只依赖外部库 + 自己，**禁** import `state` / `sdk` / `components` / `protocol` / `main` 任何 app 层。
- `plugins/builtin/agent/application/fold/`（fold/viewState）可达 `rpc`（wire 类型）+ `sdk`（dispatcher seam）+ `lib`，**禁** UI / state / main。
- `plugins/sdk` / `state` / `lib` **禁** import UI（`components` / `pages` / `builtin`）——锁住"平台/工具层不依赖它被消费的 UI"。
- `components/` / `pages/` **禁** import `@/main`（composition root）或 `@/rpc`（协议客户端）——只经 context public facade / store selector / SDK selector 触业务。
- **跨限界上下文只能走 `public/`**：`plugins/builtin/<ctx>/` 一旦有 `application/domain/adapters/presentation/ui/public` 任一目录即视为限界上下文；别的上下文只准 import 它的 `public/` facade，连它根目录下的松散文件也不行（`builtin/index.ts` manifest 作为插件组合根豁免）。`check-builtin-contexts.mjs` 再在这些合法的 public→public 边上查环。
- **`settings/` 各面板的上下文形态**：统一 `ui/`（React 组件）+ `application/`（用例、读模型与 `ports/`）+ `adapters/`（gateway 实现），plugin 入口 `index.{ts,tsx}` 注册面板。只有被其他上下文消费的稳定查询才建立 `public/queries.ts`；其余面板保持叶子。
- 业务用例依赖 application port；只有 adapter / composition root 调用 `getContainer().client().xxx(...)`。测试优先替换 port，协议 adapter 测试再用 `setContainer({ client })` / `resetContainer()`。

React Query 的 cache 与 provider lookup 是共享技术机制，留在 `lib/data/dataQuery.ts` 与 `queryClient.ts`。Session、Workspace、Approval、Provider、MCP、Hooks、Schedules、Recipes、Usage 的 query key、read model 与 hook 均由所属上下文拥有；跨上下文消费必须经过该上下文的 `public/queries.ts`（或既有 public facade）。`lib/data` 不再充当全局业务模型仓库。

Application port 使用 `lib/ports/singletonPort.ts` 管理进程内绑定。每个 adapter installer 必须返回 disposer，plugin `setup` 必须把它交给 `ctx.cleanup`；Installation remove、Host stop 或 HMR owner 退休会断开旧 adapter。disposer 按实例比较，旧插件的迟到 cleanup 不会误清除后来安装的新 adapter。`public/` 不暴露 adapter installer，组合入口在同一上下文内直接装配。

需要全局命令入口的 replaceable application owner 使用 `lib/publicationSlot.ts`：slot 只拥有 process-local exact object identity，先发布 successor、再同步退休 predecessor，并只允许 exact owner withdraw。它不能持有 task、cache、event、error 或任意业务状态；serialization、abort、projection repair、material generation 和 typed retired error 必须继续留在 concrete owner。Singleton port 复用同一 identity primitive，业务类不再各自复制 `static #active` lifecycle protocol。

RPC mutation journal 只持久化 unresolved command identity，不持久化请求参数、renderer owner、generation、lease、heartbeat 或 settlement 状态。`durableMutationJournal.ts` 拥有当前唯一 storage codec、Runtime endpoint/namespace fencing 与 exact idempotency recovery；`mutationJournal.ts` 只用 renderer 进程内 exact object identity 决定 response/error/cleanup 是否仍有提交权。client replacement 必须先发布 successor 再同步退休 predecessor，旧结果和旧 disposer 不能清理 successor 复用的 identity。

Runtime endpoint 是 Runtime context 的应用配置：application 拥有默认值、HTTP(S)
校验与 `applied | rejected` 结果语义，adapter 才把 consumer-owned port 接到 Host
config/storage。`lyra.builtin.runtime` 在 capability discovery 之前同步恢复 endpoint，
`main/container.ts` 只通过 Runtime `public/endpoint` 读取 active endpoint，并按
endpoint 与 Wails bootstrap 返回的 local token 缓存客户端。Connection 面板把稳定 rejection code 翻译为当前 locale
文案；应用变更后重载前端，让 streams、queries、capabilities 与 Session read models
在同一个 Runtime 边界上重新装配，不做半热切换。

本地 token 属于 Wails DesktopHost；它不进入 Runtime Protocol，也不借 Runtime HTTP
endpoint 建立第二套旁路 API。Desktop 不扫描或执行用户目录中的 JavaScript。
Capability discovery application 只依赖 `RuntimeDiscovery.discoverCapabilities()`；
adapter 调用 typed `client.runtime.discover()` 并移除 `DiscoverResponse` envelope。
插件 unload 后迟到的 discovery result 不得重新发布 capability。

Workspace event context 是 `runtime.subscribe` 的唯一产品 consumer：global topics 在 active
Session 工作区暂不可解析时仍保持在线，file watch 则 fail closed 为 `none`。Session adapter
只把权威 `session_not_found` 投影为 unavailable；network/transport/protocol 瞬时故障保留为
failure，由 event application 以可取消、有上限退避在同一 identity 上恢复。Session retarget
和 plugin dispose 必须使旧解析 generation 失效，旧 cwd 不能重新安装 watch。wire 错误识别、
订阅 lifecycle 和 Agent Session 投影因此仍分别留在 adapter、Workspace application 与 Agent
public facade，不互相泄露抽象。

**增加新协议方法的步骤**：

1. 在 Runtime Contract Registry 声明 method、shape、error、capability 与 wire
   constraint，并从同一来源生成 schema、OpenRPC、API Reference 与 Desktop wire。
2. `rpc/methods.ts` 只补不能由生成 metadata 表达的 typed transport 编排；禁止手写第二份
   wire union，也禁止编辑 `wire.*.generated.ts`。
3. 所属 bounded context 在 application 定义最小 consumer-owned port / use case，
   adapter 才通过 `getContainer().client().foo(...)` 实现它。
4. 业务 UI 只消费该 context 的 `public/` facade；application 测试替换 port，adapter /
   protocol boundary 测试才注入 fake client。

> 协议 method 表 / envelope / transport 形状的权威定义在
> [`app/runtime/doc/API.md`](../../runtime/doc/API.md)、
> [`app/runtime/doc/AUX_API.md`](../../runtime/doc/AUX_API.md) 与
> [`app/runtime/doc/TRANSPORT.md`](../../runtime/doc/TRANSPORT.md)，勿在本文重述。

### 3.2 Design System 反腐边界

交互实现沿一个方向组合：

```text
Base UI / browser-native semantics
  → ui/primitives
  → ui/atoms
  → ui/agent
  → feature / plugin UI
```

每层只回答自己的问题：

- `ui/primitives` 是唯一可直接 import `@base-ui/react`、渲染原生交互标签或声明底层
  ARIA widget role 的防腐层。它只做第三方/浏览器语义归一，不知道产品 tone、密度和业务。
- `ui/atoms` 定义产品交互词汇与横切策略：可见动作使用 `Button`，纯图标动作使用
  `IconButton`，文字动作使用 `TextButton`；整行、卡片、图片、色块等由内容自身拥有
  外观的复合控件使用 `Pressable`。后者只提供 button 语义与 accessibility baseline，
  不能被当作普通按钮的无样式逃生口。
- 浏览器专属行为由语义 atom 收口：`HiddenFileInput`、`ColorPickerInput`、
  `ExternalLink`、`ResizeHandle` 分别拥有固定 input type、隐藏/覆盖策略、外链安全策略
  和 separator/focus 语义；feature 不重复这些实现知识。
- `ui/agent` 只用 atoms 组合 Agent presentation primitive；feature/plugin 只消费
  atoms 或 agent primitive，并继续拥有自己的业务状态、文案、布局和领域视觉。

`npm run check:design-system` 使用 TypeScript AST 强制上述边界：禁止 primitives 外部
直接 import Base UI、禁止 design-system 外部 import primitives、禁止 primitives 外部
渲染原生交互标签或手写交互 role。测试文件可以直接渲染 DOM 以断言行为，但 production
代码没有白名单和兼容出口。

### 3.3 关于 monorepo（暂不拆）

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

1. **`startKernel(builtinPlugins, signal)`** — 创建 dougong Host，将 shell Services 与全部 built-ins 作为一个启动事务安装；Host 根据 `requires` / `provides` 解析契约图。启动失败整笔 rollback，不暴露半启动 read model。
2. **`publishKernel(host)` + ready handlers** — `host.start()` 成功后才发布当前 Host 代际与安装 read model，并触发 ready contributions。
3. **`setReady(true)`** — 解除 children 渲染门。
4. **effect cleanup** — `stopKernel(ownedHost)` 同步撤销 exact Host publication，再等待 Host 结构化资源回收。旧 renderer 的迟到 cleanup 不能撤销 successor Host；启动在 owner 退休后才结算时也必须 rollback。`main.tsx` 的 `beforeunload` 直接关闭它拥有的 renderer。

外层再包一个 `TooltipProvider`（Base UI provider，250ms delay），让 kernel + 任意插件的 `<Tooltip>` 不必各自带 provider。

> **为什么门控？** AppRouter 挂载时要读取已经提交的 route contributions。门控保证消费者只观察完整 Host transaction，不会看到"一部分插件已启动、另一部分仍缺失"的中间态。

### 4.2 AgentClientPage —— 整个 Kernel

`src/pages/AgentClientPage.tsx` 只把 plugin slots 填进 agent shell：

```tsx
<AgentAppShell
  sidebarLabel={t("shell.region.workIndex")}
  sidebarResizeLabel={t("sidebar.action.resize")}
  sidebarOpen={!drawer.collapsed}
  sidebarWidth={width}
  onResize={setWidth}
  sidebar={activeViewId === "settings" ? undefined : <Slot name="app.sidebar" />}
  main={<Slot name="app.main" />}
  overlay={<Slot name="app.overlay" />}
/>
```

三个 Slot 是 kernel 的全部肉（没有底部状态栏——run telemetry 在 composer footer，全局指示/通知在 sidebar footer）：

| Slot          | 典型贡献者                                  |
| ------------- | ------------------------------------------- |
| `app.sidebar` | `kernel-sidebar`                            |
| `app.main`    | `kernel-chat`（ChatPanel）                  |
| `app.overlay` | `command-palette` / `toaster` / `shortcuts` |

`AgentAppShell` 拥有窗口外壳、Work Index 区域和 settings 的 single-surface
组合；实时 resize 只写 shell custom property，release/keyboard step 才通过
`onResize` 持久化。插件只贡献 slot 内容，不直接组织顶层 layout。

---

## 5. 三大支柱

### 5.1 插件系统 —— Plugin SDK + 开放扩展点底座

#### 数据流：贡献 → Host read model → 订阅 → 渲染

```
PluginSpec.setup(ctx)
       │  ctx.contribute(POINT, item, opts?)
       ▼
dougong Host —— owner-qualified contribution storage + transaction + cleanup
       │  host.contributions(POINT.token)
       ▼
kernel.ts —— current Host generation + cached ContributionView + Lyra single/multi policy
       │
       ▼
selectors（sdk/selectors/）
       │
       ▼
React hooks / imperative lookup consumers
```

`contracts.ts` 把 dougong `ExtensionPoint` token 与 Lyra 的领域 key、排序、capability 和 `single | multi` read policy绑定。Core 保留所有 owner-qualified contribution；Lyra selector 才按领域 key 解析 shadow。因而覆盖插件卸载后，被遮蔽的原贡献会重新出现，而不是在写入时被破坏。

#### 一个插件长这样

```ts
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  setup(ctx) {
    ctx.contribute(AGENT_SOURCE, rpcAgentSource(t, getActiveSessionId, runtimeRunsGateway));
  },
});
```

`requires` / `provides` 是 typed service contract，不是插件名排序提示。`ctx.cleanup`、contributions、spawned tasks 与 abort signal 全部归当前 Installation lifetime；Host stop 或 Installation remove 会按结构化生命周期回收。

#### Host、Services 与 read policy

- dougong `Host` 只由 composition root 持有：安装、启动、事务变更、贡献 read model 与 stop；产品插件不通过全局 Host 绕过契约。
- 插件间命令式能力通过 `services.ts` token 与 `requires` / `provides` 注入。Lyra shell 只提供 config、i18n、window、workspace、commands、plugins 六类明确 Service。
- Runtime 网络访问不属于通用 shell Service；内置业务仍经 context adapter → `main/container` → typed JSON-RPC client，Runtime DTO 停在 Adapter。
- `kernel.ts` 只发布一个 Host generation。views 与 installed-plugin read model 都绑定该 Host identity；stale stop / subscription callback 不能清理或写入 successor generation。

#### 启动

- **built-ins**：`createKernel` 先安装 shell Services 和 manifest，`host.start()` 作为一个完整 transaction；任一 setup 失败就 rollback 全部，不发布半成品。

Desktop 只安装同 bundle 静态 import 的内置插件。不存在外部插件目录、动态 module
import、Host API 版本协商或 permission manifest；需要新的内置能力时直接改当前
manifest 和调用方，不留兼容 loader。

`builtin/index.ts` 的分组只供人阅读；依赖真相在各 spec 的 `requires` / `provides`，贡献覆盖的稳定 tie-break 才使用 manifest 顺序。

---

### 5.2 协议 fold 层（数据流入口）

#### 形状：Session projection + normalized Run tree

```
LyraClient（rpc/）—— runs.start / runs.resume 流式返回 RunEvent
   │   useAgentSession → AgentRunPump：for await (event of stream.events)
   ▼
useAgentStore.applyRunEvents(sessionId, batch)  ◄── rAF 批处理，~1 commit/帧
   │   reduceRunEvent(view, completeEnvelope)
   ▼
agent/application/fold/reducer.ts
   │
   └─ protocol event      → lookupStreamHandlers(type)
       └─ source Run / Segment / eventId / timestamp 全部来自 RunEvent envelope
   ▼
新的 AgentSessionView
   │   Zustand 通知订阅者
   ▼
Agent public read model → Chat / Workspace / Shell consumers

durable items + runs + pending interrupts + optional Plan
   │   projectAgentSessionSnapshot（Store 外完整构建）
   ▼
refreshSequence + viewRevision CAS → 整份 AgentSessionView 原子替换
```

#### 唯一 projection 与来源规则

`AgentSessionView` 是一个 Session 的唯一 Agent projection：

- `runsById` 保存 root / child / sibling / nested Run 的独立 lifecycle；
- `plansByRunId` 与 `assistantTurnByRunId` 按 source Run 隔离；
- `Message.runId`、`ToolCall.runId`、`TimelineEntry.runId` 保留 durable ownership；
- `pendingInterrupts`、`shared` 和 `commandError` 保留 Session 级事实；
- children、roots、depth 与 narrative placement 由 selector 从 lineage 派生，不保存第二份索引。

live fold 只接受完整 `RunEvent`；durable Item、Run snapshot、PendingInterruptSet 与 local
optimistic message 各有独立入口，不能伪装成 stream event。若 Item owner 与 envelope
owner 冲突、`item.completed` 仍是 running 等不变量失败，fold fail closed，不猜 root、
不改写状态。每个注册 handler 独立隔离，错误进入 plugin diagnostics。

`fold` 是 wire → published view language 的反腐层：协议 DTO 只在 fold / adapter
边界出现，`AgentSessionView` 本身不持有 `@/rpc` 类型。连续 assistant-side Item
折成一个 UI turn，但 cursor 按 RunID 保存，因此 child 与 root 事件即使交错也不会拼进同一气泡。

#### 对外 Run API 按真实 scope 命名

`agent/public/run.ts` 不发布内部 Store，也不把整个 Session 伪称成 Active Run：

- 当前 root：`useCurrentRootRunId`、`useCurrentRootPlan`、
  `useIsCurrentRootRunning`、`stopCurrentRootRun`；
- 活动 Session：`useActiveSessionRunTree`、`useActiveSessionTimeline`、
  `useActiveSessionToolCalls`、`useActiveSessionProblem`；
- 精确 Run 命令：`cancelActiveSessionRun(runId)`；
- window-level attention：`subscribeAnySessionRunning`、
  `subscribeRootRunSettlements`。

内部同样按职责分成 `runReadModel.ts`、`runCommands.ts` 与 `rootAttention.ts`，不保留旧
`activeRun` alias。

#### useAgentSession 编排会话生命周期

`plugins/builtin/agent/adapters/useAgentSession.ts` 为**一个 Session**拥有 driver 生命周期：

```
useEffect([sessionId])
  → driver = makeDriver()                         // 来自 priority 最高的 AGENT_SOURCE
  → store.ensureSession(sessionId)                // 保留已 materialized projection
  → 非 draft：读取完整 durable Session snapshot
  → off-store projection + CAS commit；并按 active root Segment reattach
  → 绑定 send / stop / resume / synchronize / cancelRun capability
  → 若有 pending（welcome 屏排队的首条消息）→ send 之

send(input):
  → 乐观渲染本地 userMessage
  → driver.start(input, signal) = client.runs.start(...)
  → StartRunResponse 必须返回 userItemId
  → 占位按 exact ItemID relabel；durable Item 按 id 去重，不做内容匹配
  → pump root Segment 的 tree-wide stream（rAF 批处理 + cursor reattach）

resume(runId, responses):
  → driver.resume = client.runs.resume(...)
  → ResumeRunResponse 只在请求同时提交新 input 时返回 userItemId

stop():
  → Session-owned command 只取消当前 running root，并返回是否接受

cancelRun(runId):
  → 精确取消 active Session 内的 root 或 descendant
  → 只合并 committed CancelRunResponse，再触发 authoritative synchronize

unmount → abort follower + 解绑 actions；projection 留到 Session 不再 open 时统一 prune
```

`runs.start/resume` 的 ack 是 accepted boundary：只有 ack 前拒绝进入 command error / HITL
`onStartError`；ack 后 stream/recovery failure 不能否定已经提交的命令。cold recovery 或 replay
reattach 在 snapshot 与 subscribe 之间遇到 terminal/waiting/stale Run 时，必须经 Agent
application port 重读完整 durable projection；不能把旧 Running 留给 UI 等待偶然 invalidation。

默认 driver 由 `rpc-agent` 插件贡献（`AGENT_SOURCE`，走 JSON-RPC）；插件可替换成 mock / IPC / 本地模型等。

---

### 5.3 状态分层（除 agent 外的 UI 状态）

| Store                    | 内容                                                                | 持久化         |
| ------------------------ | ------------------------------------------------------------------- | -------------- |
| `agentStore`             | 每 Session 的 `AgentSessionView`、refresh revision 与已绑定 actions | ❌ ephemeral   |
| `agentSessionStore`      | active/open/draft Session、selection epoch 与 welcome pending input | ✅（部分字段） |
| `uiStore`                | theme / accent / 字体 / motion / messageStyle / sidebarRail         | ✅             |
| Runtime capability store | 握手协商能力（由 runtime context 私有持有）                         | ❌ ephemeral   |
| `tasksStore`             | host.tasks 的后台任务                                               | ❌             |
| `composerStore`          | 撰写区文本 / 模式 / 附件 / provider+model                           | ❌ ephemeral   |
| `contextDockStore`       | 按 Session 隔离的 file/tool/dock material                           | ❌ ephemeral   |
| `workspaceSurfaceStore`  | app-global main/settings surface                                    | ❌ ephemeral   |
| `useConfigStore`         | 插件可读写的全局 config（如 `runtime.endpoint`）                    | ✅             |

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

helper 自动补 shadow ladder + CTA defaults + `ctx.contribute(THEME, …)` 注册仪式。切主题时 `uiStore` 副作用：替换 `<html>` 的 `theme-{scheme}` class + 把 `palette` 全部 inline 写到 `:root.style`（内联永远胜过 stylesheet，插件完全拥有调色板）+ 最后写一次用户选的 `--color-accent`。

加新主题 = 新文件（调 `defineThemePlugin`）+ `theme/themes/index.ts` 加一行；Settings → Appearance 的 picker 从主题 extension view 自动读列表。首屏防闪烁靠 `index.html` 内嵌一段同步 JS 在 CSS 解析前贴 `theme-{scheme}` class。

---

### 5.5 可观测性（OpenTelemetry 三信号）

`lib/observability/` 是后端 `setupObservability` 的前端镜像：**一处**装好三个全局 OTel provider（Tracer / Meter / Logger）+ 共享 Resource（`service.name=lyra-frontend`）+ W3C TraceContext+Baggage propagator，其余代码只用 `trace.getTracer` / `metrics.getMeter` / `logs.getLogger` 这些静态访问器（无注入）。

- **安装时机**：独立 `observability` 插件**动态导入** `setup.ts` 并 always-on 安装——重 SDK 进懒 chunk、不碰首屏；trace context 传播又始终在线。
- **可切换 exporter**（同后端）：本地有界内存 sink（dev 可见，`stores.ts`）始终在；配了 `otel.endpoint` config 才追加 OTLP（prod 切换，懒导入 + 批处理）。
- **三信号**：①Traces——`tracing.ts` 给每个 run 开 span（`useAgentSession`），`rpc/transports/http.ts` 给每个 RPC 开 CLIENT span 并把 `traceparent` 注入 header（接上后端已有 trace，§6.2：trace 元数据走 header 不进 body）。**粗粒度**——绝不按 StreamEvent/token 开 span。②Metrics——`lib/metrics.ts` 的 histogram/counter。③Logs——`logBridge.ts` 把 `host.log.*` 也发成 OTel LogRecord（按 active span 关联）。
- **性能/存储**：本地 sink 是**内存有界环形缓冲**（最新 N，非 localStorage/IndexedDB——高频遥测不该落前端，持久化交给 OTLP→collector），sink 批量刷新（一波一次 store commit）；Diagnostics view 三页（traces/metrics/logs）的 traces/logs 用 `@tanstack/react-virtual` 虚拟滚动。

---

## 6. 渲染端：Slot 与各种 useXxx Hook

### 6.1 `<Slot name="…"/>`

`src/plugins/host/Slot.tsx` 是 kernel ↔ 插件的"插槽桥"：

```ts
const specs = useLayoutSlot("app.sidebar");   // 订阅当前 Host contribution view
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

| Hook / 函数                                      | 用途                            |
| ------------------------------------------------ | ------------------------------- |
| `useToolPreview(fn)` / `useToolActions()`        | 工具卡片预览 / 头部按钮         |
| `useWorkspaceViews()` / `useSettingsPanes()`     | 主区 workspace view / 设置左栏  |
| `useSidebarSections()` / `useSidebarRailItems()` | 侧栏内部                        |
| `useCommands()` / `useSlashCommands()`           | 命令面板 / composer slash 提示  |
| `useComposerModes()` / `useComposerStatus()` / … | composer 工具栏                 |
| `useThemes()` / `useAccents()`                   | Appearance 面板                 |
| `useMessageRole(id)`                             | 消息头像 / 名字                 |
| `lookupStreamHandlers(type)`                     | reducer 内部用，非 React 选择器 |

---

## 7. 端到端的几个典型流程

### 7.1 用户输入消息发送

```
Composer onKeyDown (Enter) → submitComposer → useChatSend(text)
   → 有 active session → agentStore.send；无 → useCreateSession 起草稿 + 排队首条
   → useAgentSession.send → 乐观渲染 local 气泡 + driver.start
   → StartRunResponse.userItemId 精确 relabel optimistic Item
   → client.runs.start → 流出 segment.* / item.* / state.* …
   → pump（rAF 批）→ agentStore.applyRunEvents → reduceRunEvent → 新 projection
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
   → agent fold 物化一个 approval / question 块（status="requires-action"）
   → 绑定 { parentRunId, itemId }
用户点 Approve / Decline（或回答 question）
   → useApprovalSubmit / useQuestionAnswer → useAgentSession.resume(parentRunId, responses)
   → client.runs.resume 起一个续跑 Run（parentRunId 链接），新 RunEvent 流接着 fold
   → 卡片乐观 settle（resolveInterrupt）
```

## 8. 错误隔离策略

| 失败点                                  | 行为                                                      |
| --------------------------------------- | --------------------------------------------------------- |
| 插件 `setup` 抛错                       | dispose 已注册部分；其它插件继续；写错误到 Plugins 面板   |
| 插件组件 render 抛错                    | PluginBoundary 接住画 fallback；其余 kernel 正常          |
| stream handler 抛错                     | 该 handler 跳过，state 保持入态；其余 handler 继续        |
| 插件 tool action / command 抛错         | console.error + `reportPluginError`，UI 不挂              |
| `runs.start/resume` 在 ack 前 reject    | channel-a 失败：无流；保存 Session command problem        |
| 已 accepted Run 的 stream/recovery 失败 | 不回滚命令；durable projection 负责权威收敛               |
| `segment.finished{error}`               | terminal Run outcome 投影为可 dismiss problem             |
| stream 断线且 replay 可用               | 从最后 folded eventId reattach                            |
| `replay_unavailable` / runtime resync   | 读取完整 durable Session snapshot，再做 CAS 原子替换      |
| fold 来源或 lifecycle 不变量失败        | 当前 handler fail closed；保留入态并写 plugin diagnostics |

Plugins 面板（Settings → Plugins）汇总所有 `reportPluginError` 的红 badge。

---

## 9. 怎么写一个插件

最小三件套：

```ts
import { definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.example.hello",
  capabilities: ["commands"],
  setup(ctx) {
    // 1. 加一个 Cmd+K 命令
    ctx.contribute(COMMAND, {
      id: "hello.world",
      label: "Hello, world!",
      group: "Examples",
      run: () => ctx.notify("hi", "info"),
    });
    // 2. 副作用归当前 Installation lifetime
    const unsub = someStore.subscribe(/* … */);
    ctx.cleanup(unsub);
  },
});
```

> 放到 `plugins/builtin/<domain>/<name>/index.ts(x)`，在 `builtin/index.ts` 合适分组 import 并加入数组。

静态 registration 是 plugin composition，不是 application use case。只返回 `{ id, order, component }` 或同类 extension spec 的 factory 必须直接写在插件入口；不得为它单建 `application/*Contributions.ts` 和只复述字面量的测试。只有 contribution module 自己拥有稳定策略或行为时才保留，例如 Composer key-binding 语义、默认命令集合、tool family 映射，或跨 context 的 SDK published-language facade。`check:published-boundaries` 会拒绝只投影对象字面量的 application contribution module。

**自定义内容块的类型注册**（让 TS 满意）：

```ts
declare module "@/plugins/sdk/types/contentBlock" {
  interface CustomContentBlockMap {
    exampleBanner: { kind: "exampleBanner"; text: string };
  }
}
```

> 内置协议块（text/reasoning/plan/tool/approval/question）在 `plugins/builtin/chat/message/ui/` 内部直渲（`renderBlock` switch）；可拆卸功能块（如 `preview-blocks`）才走 typed content-block extension point。

---

## 10. 不变量速查

- **Kernel 不知道任何具体功能**——所有看得见的元素都来自插件。改一处功能 = 改一个插件目录。
- **当前 Host contribution view 是插件真相**——不直接 import 内置插件取贡献，永远走 `useXxx` / `lookupXxx`。
- **store 是单 Zustand instance**——多 selector 订阅，不要把 store 包进 context。
- **Agent projection 只有一个作者**——live fold 与 durable snapshot 投影共享规则；
  render 不回写 store，跨 context 只调用 Agent `public/` command/read model。
- **components 不直连后端**——只经 context public facade / store selector / SDK selector，**禁** import `@/main` / `@/rpc`（`check:layers` 强制）。
- **插件资源归结构化 lifetime**——setup 中用 `ctx.cleanup`，composition root 只停止自己持有的 exact Host generation。
- **协议是唯一 outbound 边界**——不在 UI/store 里直接 `fetch` / 开 SSE / 调 IPC，都走 `rpc/`。
- **交互语义只向外组合**——Base UI / 原生标签只在 primitives；业务 UI 只用 atoms /
  agent primitives，复合内容用 `Pressable` 而不是反向撤销 `Button` 样式。

---

## 11. 进一步的阅读路径

| 想了解                                                      | 先看                                                                                        |
| ----------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 决策透镜 / 工程约定 / 反向不变量                            | 仓库根 `CLAUDE.md`                                                                          |
| 视觉规范 / 颜色 / 排版                                      | `frontend/DESIGN.md`                                                                        |
| 后端给什么数据 / 每个字段要表达什么（自包含，可给外部人看） | `frontend/CONTENT_RENDERING.md`                                                             |
| 协议 method 表 / envelope / 语义                            | `app/runtime/doc/API.md` + `app/runtime/doc/AUX_API.md`                                     |
| transport / handshake / 错误码                              | `app/runtime/doc/TRANSPORT.md`                                                              |
| 插件 context / shell contracts                              | `src/plugins/sdk/definePlugin.ts` + `src/plugins/sdk/services.ts`                           |
| 协议 fold                                                   | `src/plugins/builtin/agent/application/fold/reducer.ts` + `builtin/agent/application/fold/` |
| 一个完整内置插件                                            | `src/plugins/builtin/agent/rpc-agent/index.ts`                                              |
| Agent Session driver / recovery                             | `src/plugins/builtin/agent/adapters/useAgentSession.ts`                                     |
| Run tree read model / commands                              | `src/plugins/builtin/agent/application/run/` + `src/plugins/builtin/agent/public/run.ts`    |
| 主题如何注册                                                | `src/plugins/builtin/theme/kit/` + 任意 `theme/themes/*`                                    |

---

## 12. 改进方向（forward-looking analysis）

这份清单**有依据**而非 wishlist——每条标"做/不做的理由 + 触发条件"，避免 backlog 变成永不收敛的"理想架构"幻象。

### 12.1 已落地的改进（现在只随 wire 新形态维护）

> 这些曾是 backlog；截至当前 HEAD 都已落地。保留在此是记录「完成态 + 维护触发点」，不是待办 —— 下一轮别再当新活做。

#### A. agent fold 各 handler 的语义测试（已落地）

**现状**：`builtin/agent/application/fold/` 拆成 `handlers`（派发）/
`projections`（纯 wire → view）/ `fold`（source-owned upsert）；`reducer.*.test.ts`
覆盖 dispatcher、聚合、root/child/sibling/nested lifecycle 与主要 Item 路径。
`reducer.handlers.test.ts` 为每个 handler 钉住“完整 RunEvent → 隔离 projection delta”
契约，包括 Plan replacement、未知 ItemID delta no-op、`segment.started` lifecycle 初始化与
`plan.updated` 只更新显式 `plan` projection。
**维护触发**：加新的内置事件类型 / Item 类型时，一并补对应 handler 的语义测试（input→state delta）。

#### B. search / webSearch 富结果渲染（已落地）

**现状**：view 层已直接从 tool 自带结果渲染，不再「只投影计数 + 从 workspace 取数」——`webSearch.tsx` 解析 `tool.result` 的 title/url/snippet/favicon；grep preview 优先用 call-scoped `tool.result`（`inlineGrepRows`），workspace.grep query 降为 fallback。
**维护触发**：wire 出现新的富结果形态（新字段 / 新 tool family）时，扩展
`application/specialisedPreviewProjections` 的解析并补 preview 测试。

#### C. fileChange diff 直渲（已落地）

**现状**：`DiffPreview` 优先用 call-scoped `tool.diff`（`useDiffToolPreview`：`tool.diff ? tool.diff : 整树 diff`），仅在没有 call-scoped diff 时回退 worktree query。
**维护触发**：后端下发更细的 diff（多文件 `changes[].diff` / 更大 diff 行）时按需扩展投影。

#### D. Work Index read model（首批落地）

**现状**：`plugins/builtin/navigation/` 已承接左侧工作索引投影，`sidebar/` 不再现场 join `projects + sessions + active session`，expanded sidebar 与 rail 都从 `navigation/public/workIndex` 消费分组 / 最近会话 read model。会话运行状态在 navigation application 投影为 `WorkSession.attention`，sidebar 只显示 Work Index attention，不泄漏底层 `AgentSessionSummary.status`。
**维护触发**：继续推进 `FRONTEND_AGENT_WORKSPACE_MODEL.md` 的后续阶段时，新的 workspace/cwd 面板不要塞回 `sidebar/`。

#### E. Right workspace open intent（已落地）

**现状**：右侧已从“单一 dock view + context launcher”改为用户拥有的 workspace tab set。`openWorkspaceViewInDock(id)` 负责新增或聚焦 singleton tab；header 的 add-panel menu 直接读取 `CONTEXT_DOCK_DESTINATION`，按 `workspace / run / session` scope 分组，不再用中间 launcher view 替换当前材料。tab 可逐个关闭，关闭 active tab 后选相邻项；折叠只隐藏 workspace，不销毁 tab set 或已挂载 view。Settings 等 global surface 仍使用 full workspace view。
**维护触发**：新 workspace/cwd-scoped 入口贡献 `CONTEXT_DOCK_DESTINATION` 并默认走 `openWorkspaceViewInDock`。不要重建 launcher view、固定 pinned tabs 或 dock→full 的隐式迁移路径。

#### F. Context Dock session scope（已落地）

**现状**：`contextDockStore` 已把 `dockOpen + dockViewIds + activeDockViewId` 及 material state 按 active session scope 保存/恢复；`workspaceSurfaceStore` 只承载 app-global surface state（main tabs / settings target）。`workspace.session-navigation` 监听 agent session selection/lifecycle，切换 session 时保存离开的右侧 workspace、恢复进入的 workspace，关闭 session 后清理不再打开的 scope。右栏宽度是稳定的单一用户偏好，切换 tab 不改变列宽。
**维护触发**：后续如果引入 cwd 级共享，不要把 app-global surface state 与 session-scoped dock state 重新揉回一个 store；在 workspace application 层显式定义 `sessionId -> cwd` 的归属规则。

### 12.2 想做但当前 KISS / YAGNI 不允许

- **`<ToolPrimitive>` headless 组件**：目前只有 `approval` 一个真正 actionable 的块（tool 是只读指针，code/search 是被动展示）。给单一消费者抽 primitive 违反"3+ 重复才抽象"。**触发条件**：第二个 actionable block 出现（如 code-proposal 升级为 accept/reject）。
- **把 `lib/agent` 提成独立 `application/` 层**：`lib/` 已是"跨插件共享"的明确语义（`messageContent` 就是被刻意从 plugin 内部移来的），6 个用例 hook 不足以撑起一个独立层 + 一条新 layer-guard。**触发条件**：用例 hook 显著增多、或 UI 开始绕过它们直接编排 rpc。
- **MessageStream 虚拟化**：长会话（1000+ 消息）目前无人抱怨。**触发条件**：实测 > 500 消息卡顿时引入 `@tanstack/react-virtual`。
- **monorepo packages**：见 §3.2 的 4 个触发条件，目前一个都没命中。

### 12.3 反向不变量（已知错的方向，别再提）

与 `CLAUDE.md §6` 一致，不重述。要点：不换 Zustand / React Query / Wails /
OxLint / Vite；不给内部数据流加 Zod（只在信任边界）；不把贡献面退回 per-slot
add/remove map（已塌进 `extensions` 底座）；协议保持 JSON-RPC，不 RESTy 化、不在
envelope 装 transport 元数据。详见 `CLAUDE.md §6` +
`app/runtime/doc/API.md §0`。

---

> 当前架构通过所有审计原则（KISS / SOLID / YAGNI / DRY），无 AG-UI 残留，文件 LOC 在合理范围，热路径有测试覆盖。日常维护维持现状，**继续等触发条件出现**，不做投机式重构。
