# CLAUDE.md — project context for Claude Code

> **Lyra** — Wails 桌面应用（Go 后端 + React/TS 前端）。由自研 **Lyra Runtime Protocol v2**（JSON-RPC，Session→Run→Item 流式）驱动的插件化 agent client。
> 详细架构看 `frontend/ARCHITECTURE.md`，设计系统看 `frontend/DESIGN.md`，协议看 `docs/protocol/API.md` + `docs/protocol/TRANSPORT.md`。

> **本文件的读法**：先读「第一法则」（决策的总透镜）→ 再读 §1 架构心智（系统长什么样）→ §2-§5 是写代码的判断与硬约定 → §6 是别走的方向 → §7 是怎么干活。附录是参考。

---

## 第一法则 —— 绝不为一时方便留历史债务

> **最高优先级，凌驾于本文件其余所有约定之上。**

**项目处于快速开发 / 试错阶段，没有历史债务、也没有外部兼容包袱** —— 协议 wire shape、暴露的类型 / 组件契约 / store shape、命名，**全都可以调整**。正因如此：

- ❌ **绝不为"少改几处 / 降低迁移 / 赶进度"留下任何历史债务** —— 迁就外部库或旧名字的命名、为兼容留的字段、推测性 shim、"以后再清"的 TODO，一律不留。
- ✅ **发现设计不对，就在源头改对**，不在错的设计上叠补丁。**现在改成本最低，往后只会更贵。**
- 命名 / shape 按**本质第一性**决定；参考业界（Claude Code / Codex / AG-UI 等）**只取思想、不作命名锚** —— 名字恰好相同，只因它在独立评估下最优，不为兼容或省迁移。**例**：弃用 AG-UI 作契约、改自研原生模型 —— 宁可现在付重写成本，也不背一个 `0.0.x` 单厂商标准的长期耦合。
- **唯一允许背的"债"是"设计还没想清楚"本身**；绝不允许"明知更好、却为省事不改"。

---

## 1 · 架构心智模型

### 1.1 一句话定位

**Kernel 不长肉，所有功能都是插件。** 路由、布局、内容渲染、命令、快捷键、主题、运行时事件处理（StreamEvent fold）、设置面板 —— 全部由 `frontend/src/plugins/builtin/` 下的内置插件贡献。Kernel 自己只是一组命名 Slot + 几个共享 Zustand store。

### 1.2 三大支柱

1. **插件系统** (`frontend/src/plugins/sdk/` + `builtin/`) — `definePlugin({ name, version, setup({ host }) { host.extensions.contribute(POINT, spec) } })`。**开放扩展点底座**：每个贡献面是一个 typed `ExtensionPoint`（`defineExtensionPoint`，见 `kernelPoints.ts` 的 ~35 个内置点），所有贡献——内置与第三方——走同一条 `host.extensions.contribute(point, item, opts?)` 写路径，读侧走 `useExtensionPoint` / `lookupExtensionByKey` 等 selector。Host 上只保留少量**薄 facade**（`events.onStream/onCustom`、`layout.register`、`message.registerContentBlock`、`lifecycle.onReady` 等——各带去重 id / 泛型 / 行为逻辑，本身也只是调 `contribute`）+ 命令式动作（`workspace.openView`、`config`、`storage`、`rpc.get/post`、`notify`、`window`、`plugins.load` …）。
2. **协议 fold 层** (`frontend/src/protocol/run/`) — `reducer` 是**纯派发器**，把 v2 `StreamEvent`（run.* / item.* / state.*）路由到 `host.events.onStream(...)` 注册的 handler chain，`custom` events 路由到 `host.events.onCustom(...)`。所有协议语义（Item→message/block 投影、HITL）都在 `builtin/agent/core-reducer` 插件里（`handlers` 派发、`projections` 纯映射、`fold` 有状态折叠）。wire 层是 `frontend/src/rpc/`（Item / RunEvent / 方法表）。
3. **状态分层** (`frontend/src/state/`) — 几个小 Zustand store，各司其职：`agentStore`（每会话 view state，ephemeral）/ `sessionStore`（tab / draft / 选择，部分持久化）/ `uiStore`（主题 / accent / 字体 / motion，持久化）/ `runtimeStore`（握手能力，全局 ephemeral）/ `tasksStore`（后台任务）/ `composerStore`（编辑器，ephemeral）。store schema 变了就 bump `version` 丢旧数据，不写 migration。

### 1.3 关键目录

```
frontend/src/
├── pages/AgentClientPage.tsx       kernel — 几个命名 Slot 而已
├── plugins/
│   ├── host/                       插件宿主运行时（Slot · PluginProvider · PluginBoundary · PluginContentBlock · PluginToaster · sideload · hostBridge）
│   ├── sdk/                         插件平台：types/(12 domain 文件) · kernelPoints.ts(~35 点) · defineExtensionPoint · host.ts(contribute 写路径) · registry.ts · selectors/(useXxx/lookupXxx + extensions.ts 底座读侧)
│   └── builtin/                     内置插件，按域分组：agent · chat(+tools/) · command · defaults · i18n · settings · sidebar · theme · workspace · shell(纯框架)
├── protocol/run/                    协议 fold 边界：reducer(StreamEvent 派发器) · viewState(Item→message/block 投影 shape) · runDigest(派生)
├── state/                           agentStore · sessionStore · uiStore · runtimeStore · tasksStore · composerStore + 会话编排 hook(useAgentSession …)
├── components/                      纯展示 + 薄 store 接线（经 selector/hook 触业务，不直连 rpc/container）
│   ├── chat/message/               消息内容渲染整体：消息壳 + markdown/(渲染基底) + cards/(块卡片)。barrel 是唯一公开面
│   ├── chat/panel/                 面板编排 + 流 chrome（公开 ChatPanel）
│   ├── chat/composer/              输入
│   ├── common/                     设计系统原子（Icon · Panel · DataView 三态 render-prop …）
│   ├── tools/                      工具调用渲染（ToolCard · ToolInspector · previews/）
│   └── sidebar/                    导航组件
├── lib/                            agent/(HITL + 会话 hook + streamReveal) · data/(ky + react-query) · i18n/ · markdown/(rehype + shiki，纯 infra) + util
├── rpc/                            Runtime Protocol 边界（JSON-RPC client + methods + shapes + transports）— 唯一 outbound 副作用层
├── main/container.ts               composition root（DI 单例：createLyraClient(transport) + sidecar）
└── styles/globals.css              Tailwind base + @theme token + 全局 keyframes（唯一 CSS；其余历史 .css 只承载无法用 utility 表达的 markdown/code/panel chrome）
```

> Go 侧只剩 Wails 桌面壳（`app.go` / `main.go`），**不内嵌 runtime**；前端经 HTTP 连外部 Lyra Runtime（`api.baseUrl`，默认 `:17171`）。协议契约见 `docs/protocol/`。

---

## 2 · 技术栈

- **UI**: React 19 + TypeScript
- **样式**: **Tailwind 4** + `cva`（class-variance-authority）+ `clsx` + `tailwind-merge`（`cn()`）。**所有新代码用 Tailwind utility class，不再写新的 .css 文件。** 全局样式只剩 `src/styles/globals.css`。
- **Headless 基件**: **Radix Primitives first**（Dialog / Popover / Select / Tooltip / DropdownMenu / ContextMenu / Tabs / 等）。**Radix 没有的才考虑别的库或自己写。** 不用 shadcn/ui npm 包，但可借鉴它的 className 字符串作起点。
- **特定组件库**: `cmdk`（Cmd+K 命令面板）/ `sonner`（Toast）/ `lucide-react`（图标）
- **状态**: Zustand 多 store（无 context 链）
- **路由**: TanStack Router（动态构建）
- **数据**: TanStack React Query
- **协议**: 自研 Lyra Runtime Protocol v2（JSON-RPC 2.0，`frontend/src/rpc/`，见 `docs/protocol/API.md` + `docs/protocol/TRANSPORT.md`）。已弃用 AG-UI。
- **桌面壳**: Wails v2
- **测试**: Vitest + Testing Library + happy-dom
- **构建/打包**: **VoidZero 栈** —— Vite 8（内置 Rolldown bundler）/ Vitest 4 / OxLint 1.x（Rust-based）。`prettier` 做格式化。CI / pre-commit 跑 `tsc + oxlint + vitest + knip`。

---

## 3 · 设计原则（怎么判断）

这些是**判断标准**，不是机械规则——拿不准时回到这里。

- **KISS / SOLID / YAGNI / DRY / 高内聚低耦合** 是总判据。不为未来不存在的需求做抽象。组件按职责拆分（SRP），**抽象只在 3+ 重复时引入**，模块之间通过最小接口（plugin SDK / store selectors）通信。
- **开发阶段，零 legacy 兼容代码 / 迁移路径**。store schema 变了 bump version 丢旧数据。注释里也不写 "Legacy …" 之类的措辞。
- **不要重复造轮子** —— 写工具函数前先查 npm。现有约定：HTTP 用 `ky`，状态用 `zustand`，缓存用 `@tanstack/react-query`，class 合并用 `clsx + tailwind-merge`（`cn()`），代码高亮用 `shiki`，国际化用 `i18next + react-i18next`，动画用 `motion/react`，命令面板用 `cmdk`，Toast 用 `sonner`，图标用 `lucide-react`，分词用 `Intl.Segmenter`（Wails 2 WebView 自带，无需 polyfill）。**例外**：(a) 极小封装 < 10 行；(b) 应用专属业务逻辑（如 `useApprovalSubmit`）；(c) 对现有库的薄包装加插件 hook（如 `lib/data/http.ts` 包 ky 加 RPC hooks）。一定要手写就在注释里说明社区库为何不够用。
- **边界校验用 Zod，内部数据流不加** —— 信任边界（sideload plugin spec / v2 RunEvent envelope / 用户输入的 URL 等）必须用 Zod schema 拒掉脏数据（RunEvent 信封校验在 `src/rpc/stream.ts`，其余就近）。内部流（Zustand store / React props / codegen 类型守护的 StreamEvent payload / SDK 类型守护的调用）**不加 Zod** —— TS 已经够了，再加是噪音 + bundle 浪费。
- **现代特性只在"可读性/正确性提升 _且_ 不引入分配/退化"时采用** —— target 是 ESNext，Wails 2 WebView 带全套现代 API（`Array.at` / `findLast` / `findLastIndex` / `Object.groupBy` / `structuredClone` / `Intl.Segmenter` / 异步迭代器 等），但"追新"本身不是理由。判据三连：(a) **真有等价老写法可替换**（`arr[arr.length-1]` → `.at(-1)`、原地反向 find 循环 → `findLast(Index)`、`indexOf(x) !== -1` → `includes`），不为没有的场景硬套（无 `JSON.parse(JSON.stringify)` 就别提 `structuredClone`）；(b) **不引入额外分配 / 不退化热路径**——`toReversed()` / `toSorted()` 会复制整个数组，在 reducer / 每 token delta 调的函数里用就是性能回归，保留 allocation-free 的原地反向循环；(c) **类型与行为完全保留**——`noUncheckedIndexedAccess` 已开，`.at(-1)` 与索引形式同为 `T | undefined`，互换安全。**反例（别动）**：反向 `splice` mutation 循环必须倒序索引，换 `findLast` 会损坏；`.then()` 链 / `.reduce` 数值累加已是地道写法，换 async-await / `groupBy` 是硬套。**地道 ≠ 最新。**
- **重构是节奏，不是可选项** —— 分两档：
  - **重构镜头的完整清单见 [`REFACTORING.md`](REFACTORING.md)**（命名名实相符 / 不存派生态 / 边界自吞空态 / 作用域卫生 / 卫语句降复杂度 / 就近组织 等）；本节只给下面两档**节奏**与 Fowler 清单，落手时对照那份。
  - **小型重构（每 3-5 轮 feature）**：聚焦最近改动的那几个文件。扫一遍：单文件超 250 行没？局部 3+ 重复 pattern？最近加的注释里有 what-说明可删？rename 漂移没？新增 API 破坏既有抽象没？产出通常是抽 1-2 个 helper / 删几条死注释 / 精修 1-2 个名字 —— **净变化 < 100 LOC、touch < 5 文件**。
  - **大型重构（每 15-20 轮 feature）**：跨整个 `frontend/src/` 扫。跑 `npx knip` 找未引用的 export / 文件 / 依赖；找 > 500 行的文件拆 SRP；找跨模块的 3+ 重复；考虑架构层级。产出通常是 multi-batch 重构计划（A/B/C 三批），用户确认后逐批 commit，每批之间跑全套 `tsc + vitest + oxlint + knip` 全绿。
  - **共同做法**：上来先扫一遍现状给 3 项候选方案 + 权衡，**等用户确认再动手**。每轮收尾跑相应检查全绿再 commit。
  - **目的**：小型重构防局部熵增，大型重构防架构熵增。
  - **信号**（任一命中就该开始）：单文件 > 500 行、同一结构不变量（复合键格式、归一逻辑）手写 > 3 处未收敛、新建插件需改 kernel 多处、命名漂移导致两个名字指同一个东西、最近 commit 反复改同一段代码（抽象方向错了）。
- **重构清单**（Fowler《重构》的实践，每轮要扫的不只是拆分）：(a) **死代码清理**——`npx knip` 找未引用的 export / 文件 / 依赖，**删干净**而非留着"将来可能用"；(b) **卫语句替代嵌套 if**——`if (!x) return` 比层层缩进可读 10 倍；(c) **查表法替代条件链**——三个以上 `if/else if` 或嵌套三元换成 `Record<K, V>` 查表；(d) **精准命名**——`idCounter` → `nextCompositeKeyId`，`x` / `tmp` / `data` 不算名字；(e) **注释清理**——解释 what 的大段注释删掉、过期迁移注释删掉、误导性的"为什么这样写"（实际不再这样）删掉，留下来的只解释 _why_；(f) **性能扫描**——`useMemo` deps 是否引用稳定、`Map`/`Set` 还是数组、循环里有没有 N² 的 `Array.find`。

---

## 4 · 硬约定（违反 = 回归）

这些是**机械规则**，不需要再判断，照做：

- **Tailwind first**：组件样式用 utility class。`style={{ ... }}` 内联只在 token 值真的动态时用（如主题预览 swatch 的 `style={{ background: spec.tokens["color-bg"] }}`）。
- **不写新的 .css 文件**：所有新样式进 JSX className。`src/styles/globals.css` 是唯一例外。
- **Radix first（硬规则，不是"考虑"）**：任何带交互行为 / 焦点管理 / 键盘导航 / aria 语义的组件，**一律先用 Radix Primitives，不手写**。已在用：dialog / popover / select / dropdown / tooltip / context menu / tabs / slider / toggle-group(segmented) / checkbox / progress。需要新的这类组件时第一反应查 Radix 有没有，有就在 `common/` 下薄包一层（复刻设计 token 的 className，见 `Checkbox` / `Slider` / `Segmented`），**绝不手写 focus trap / roving tabindex / aria-\* / 键盘事件**。
  - **唯一豁免**（换 Radix 零收益甚至有害，且必须在代码注释写明理由）：(a) 纯展示无交互（如 `Avatar` 只渲染 initials）；(b) Radix 版有实测开销且无 a11y 收益（如 `ScrollArea` 故意用原生滚动）；(c) 定制行为 Radix 模型套不进去（如 `ReasoningBlock` 流式自动展开）。判据：**Radix 是否带来真实 a11y / 行为收益**——有就换，纯为统一不换。
  - **测试注意**：Radix 的 `Tabs.Trigger` 等在 `mousedown`(+focus) 激活而非 `click`，测试用 `fireEvent.mouseDown`。
- **No lines anywhere**（DESIGN.md 原则）—— 区域分隔用 surface ladder（color-mix 派生的 `--color-surface` / `surface-2` / `surface-3`），不用 1px 边线。状态指示（active tab 底部 accent 横线、focus ring）允许。
- **Cards-on-canvas 布局** —— 外层 `--color-bg` 是暗 canvas，`.panel` 浮在上面用 `--color-surface`，8px gap + radius + shadow。
- **Grid / Flex first** —— 超过两个元素的排版首选 CSS Grid 或 Flexbox。**不用** `position: absolute` 手算坐标、`<table>` 排版、连串 `margin/padding` 凑对齐。Grid 适合二维 / Flex 适合一维，嵌套是常态。`position: absolute` 只用于浮层（dropdown / tooltip / 右 gutter outline）和 SVG 锚点。**给隐式 / 单列 grid 显式写 `grid-cols-[minmax(0,1fr)]`** 防止宽 child 撑爆 track（反复踩过的坑）。
- **Tab hover === active** —— `.chat-tab` 用同一背景（`color-mix(var(--color-text) 4%, transparent)`），只用底部 2px accent line 区分激活态。
- **Plugin 一定走 registry** —— 不直接 import 一个 builtin plugin 去用，永远走 `useXxx()` / `lookupXxx()` selector。
- **运行时事件单向** —— render 路径不回写 agent store；要"做事"就调 store 上的 `send` / `stop` / `resume`（HITL）。
- **components 不直连后端** —— `components/` / `pages/` 只经 store selector / `lib/data` query hook / SDK selector 触业务，**不得 import `@/main`（composition root）或 `@/rpc`（协议客户端）**。`check:layers` 已强制（alias-aware）。
- **Disposable 由 Host 收集** —— 不手动调 `dispose()`。
- **加文档？先问** —— 不主动创建 `*.md`，除非用户明确要。CLAUDE.md / ARCHITECTURE.md / DESIGN.md 已存在，其他默认不写。

---

## 5 · React effect 纪律（写组件 / hook 时必看）

perf 排查沉淀的硬规则，几个"看似没事其实在累积"的坑：

- **State 读：handler 里走 `useXxxStore.getState()`，render top-level 不订阅**（除非那个值真要驱动渲染）。`MessageContextMenu` 之前 top-level 订阅 `useAgentAction("send")` + `useComposerStore((s) => s.setValue)`，每条消息 mount 一个 → 50 条消息 × 30 token/秒 = 上千次 selector 评估/秒。改成 `onSelect` 里 `getState()` 后零订阅。
- **模块级 `subscribe(...)` 必须配 `import.meta.hot.dispose` 清理**。HMR 每次重载该模块都重新注册 listener，旧的不清，N 轮后 N+1 个 listener 跑同一更新——dev 越用越卡的隐形元凶。例：`state/uiStore.ts` 的 subscribe 都捕获 unsubscribe 句柄 + 注册 `import.meta.hot.dispose`。
- **热路径不要 `console.debug` 完整对象**。run streams ~30 events/sec，DevTools console 留每个 event 引用，长会话累积几万个对象无法 GC。要看去 Diagnostics view。
- **Reducer / state 更新写入数组前去重**：无脑 push 会让事件二次触发时同 id 共存——React 警告 `Encountered two children with the same key`，warning loop 拖死帧率。每个会 push 的 handler 先 `findById`（如 fold 里的 upsert）。
- **`oxlint --react-plugin + react-hooks/exhaustive-deps + rules-of-hooks` 已在 `.oxlintrc.json` 启用**，`npm run check` 会卡掉缺 deps 的 useEffect。故意忽略用 `// eslint-disable-next-line react/exhaustive-deps` + 注释写明为啥。
- **UI shell 不在 async query 上 join 后 filter**。tab strip / sidebar 列表 / 任何"反映 store 状态"的 UI 必须 **1:1 镜像 store 形状**。query 没 ready 用 placeholder，**不要 filter 掉**——filter 会让 store 已确认的 id 在 DOM 消失，用户报"点击没反应"。例：`PanelHeader` 用 `tabIds.map(id => found ?? {id, title: id, status: "idle"})` 保持 1:1（commit `5dd7789`）。
- **先判定是不是 dev-only HMR 假象，再决定要不要修——很多根本不用修**。症状只在 `wails dev` / `vite dev` 出现、**一次整页刷新（Cmd+R）就消失、生产不可能发生**的，几乎都是 HMR 把模块级副作用重跑/叠加的假象。判据：(a) 触发路径只在 dev（`import.meta.hot` 存在）才重入吗？(b) 生产里该入口只执行一次吗（如 `PluginProvider` 只 mount 一次 → `loadPlugins` 只跑一次）？(c) reload 后立刻干净吗？三个都 yes → **别动正确性敏感的热路径（plugin loader / registry / reducer）去"修"它**，刷新即可；真要消除 dev 摩擦用纯 dev 手段（`import.meta.hot.dispose` 清理、或 `import.meta.hot.accept(() => location.reload())`），不进生产代码。**反例（踩过）**：流式文本逐字重复是 HMR 重跑 `loadPlugins` 让 handler 叠加（每 delta apply N 次）——给 loader 加"幂等卸载"反而把刚加的 layout slot/主题/命令按同 key 删掉，HMR 后工具栏消失，比原症状更糟，已回退（`1c56be2` revert `06fdf94`）。教训：dev-only 假象别用生产代码绕。

---

## 6 · 强反向不变量（已知错的方向，别再提）

### 6.1 架构 / 技术栈

- ❌ 把 Zustand 换成 Redux Toolkit / Effector / Jotai —— 现有几个小 store，订阅模型够用
- ❌ 给内部数据流（builtin plugin spec / React props / Zustand store / TS 守护的调用）加 Zod —— Zod 只用在信任边界（见 §3「边界校验」）。内部用是 bundle 浪费 + 维护负担
- ❌ 把 React Query 换 SWR / RTK Query —— 切框架无收益
- ❌ 把 Wails 换 Tauri —— 没实际 bug
- ❌ 引入 CSS-in-JS 或退回手写 CSS —— 已是 Tailwind 4 + Radix（见 §2）
- ❌ 引入完整 UI Kit（shadcn-as-npm / HeroUI / DaisyUI / Catalyst / ReUI）—— 都跟设计语言打架。`shadcn/ui` 可借鉴 className 字符串，但**不引 npm 包**
- ❌ 换 Base UI —— 评估过，Radix 在 AI 工具协作 + 社区资料量上明显领先
- ❌ 把贡献面退回 `registry.ts` 的 per-slot `addX/removeX` map —— **已反向做完**：40 个命名 map 塌进单一 `extensions` 底座，贡献统一走 `host.extensions.contribute(POINT, …)`。加新贡献面 = `kernelPoints.ts` 加一个 `defineExtensionPoint` + 一个 selector，**不动 registry/registryState**。别再提"抽 factory"或"加一对 addX/removeX"。见 `docs/EXTENSION_POINTS.md`。
- ❌ 把 `rpc/main/plugins/` 等分层模块拆成 monorepo —— 4 个触发条件都没命中（见 ARCHITECTURE.md §3.2）
- ❌ 把 OxLint 换回 ESLint / 把 Vite 退回 Rollup —— **已在 VoidZero 栈**，回退就是退步
- ❌ 装 `rolldown-vite` 单独包 —— Vite 8 已把 Rolldown 合入主线
- ❌ 切换前端框架（Vue / Solid / Svelte / Lit 等）—— 评估过，6-10 周 zero feature 期，~155 个 React 文件 + 多个 React-only 库（cmdk / sonner / streamdown / use-stick-to-bottom / Radix）要重写。"useEffect deps 心智坑"用 oxlint react-hooks 规则 + §5 两条规则封堵 80% 就够。生态损失换不了那点 signal 模型上的舒适。

### 6.2 协议 / 后端边界

- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**。Lyra Runtime 是无状态纯计算单元，**协议层零 user 概念**。需要时由更外层（OS 信任、本地进程门禁 token、未来 facade）解决。详见 `docs/protocol/API.md` §0 + §13。
- ❌ **给 LLM provider 加 OAuth / token refresh / subscription 检测**。第一刀只走 kimi-code 模式：用户填 API key、存 keychain、provider 401 就让 UI 提示重填。OAuth 是 Claude Code 路线的复杂度，暂不要。
- ❌ **把"远程后端 / 团队 server / 云端订阅"当部署形态**。这些是**未来 facade 层**的事，Runtime 协议永不感知 facade。Runtime 同一份代码跑桌面也跑服务器；facade 在外面包一层做 billing / 用户管理 / 授权。
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）。走 Go `context.Context` 或 HTTP header，**永不进 JSON-RPC message body**。详见 `docs/protocol/TRANSPORT.md` §2。
- ❌ **协议 wire 用 REST + 各种 verb / 状态码**。Lyra Runtime Protocol 是 **JSON-RPC 2.0** envelope（参考 MCP）。HTTP 只是其中一种 transport；InProcess / Wails IPC 用同样 envelope 形状。
- ❌ **把 method 名斜杠化成 `/v2/rpc/runs/start`**。HTTP transport 上 method 名照搬 method 表字符串，点保留（`/v2/rpc/runs.start`）。斜杠化会跟 REST shadow 混淆。详见 `docs/protocol/TRANSPORT.md` §6.1 + `docs/protocol/API.md` §2.5。
- ❌ **加业务方法的 RESTy read-only shadow**（如 `GET /v2/sessions/{id}`）。业务调用一律走 JSON-RPC `POST /v2/rpc/{method}`。Sidecar 只限 `/v2/info` + `/v2/health` 两个 metadata 端点，永不扩展。详见 `docs/protocol/TRANSPORT.md` §12。
- ❌ **把业务 error 映射到 HTTP status code**（如 `session_not_found` 返 404）。HTTP status 仅反映 transport 层；业务 error 一律走 JSON-RPC `error.code`。详见 `docs/protocol/API.md` §8.2 + `docs/protocol/TRANSPORT.md` §6.3。

---

## 7 · 工作流

### 7.1 常用命令

```bash
# 开发
wails dev                # 在 /Users/tangerg/Desktop/lyra/ 跑，自动启 vite + Go backend

# 质量门禁（在 frontend/ 跑）
cd frontend && npm run typecheck    # tsc --noEmit
cd frontend && npm run lint         # oxlint src
cd frontend && npm run test         # vitest run
cd frontend && npm run knip         # 死代码扫描
cd frontend && npm run check        # 全套：typecheck + lint + format + test + knip + circular + layers + bundle，全绿才往下走
```

> 测试数 / 插件数 / CSS 文件数等会漂的量请直接跑命令查，不在 CLAUDE.md 维护硬编码数字（review 反复指出过此处会过期）。

### 7.2 修改前定位

1. **`plugins/builtin/<name>/`**：动一个 builtin 插件，不影响其他面。
2. **`plugins/sdk/`**：动了 SDK 公开形状 → 跑 `vitest run` 验证所有插件还能注册。
3. **`state/<store>.ts`**：动了 store schema → bump `version`（zustand persist 自动丢旧数据）。
4. **`protocol/run/` 或 `rpc/`**：动了协议 fold 层或 wire 层 → 重点跑 `reducer.*.test.ts` + `rpc/*.test.ts`。
5. **加一个内容块**：内置块（`BuiltinContentBlockMap`）在 `components/chat/message/` 内部直渲（`PartRenderer` switch）；扩展块（`CustomContentBlockMap`，第三方 / `preview-blocks`）才走 `registerContentBlock` registry。
6. **加一个主题**：见附录 A。新文件 + `plugins/builtin/theme/themes/index.ts` 加一行。
7. **加一个插件**：`definePlugin({ ... })` → `plugins/builtin/index.ts` 合适分组加 import + 数组项。

### 7.3 沟通约定

- **中文回复**（用户偏好）；代码 / 注释保持英文。
- 大重构前先给三步方案 + 权衡，等用户确认再动。
- 改动后跑 `npm run check`，commit message 写清"why"而不仅"what"。
- commit trailer 用 `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`。

---

## 附录 A · 主题系统（IDE 风格）

内置主题（lyra-dark/light + atom-one-dark/light + tokyo-night-storm/light + solarized-dark/light + catppuccin-mocha/macchiato/latte），每个是一个独立 plugin 文件，用 `defineThemePlugin()` helper：

```ts
defineThemePlugin({
  id, label, scheme: "dark" | "light", order,
  palette: { "color-bg": "#...", "color-surface": "#...", ... },
});
```

helper 自动补 shadow ladder + CTA defaults + 注册仪式。加新主题 = 新文件 + 一行 manifest。Settings → Appearance 的 theme picker 从 registry 自动读列表。

## 附录 B · 已做的大重构（避免重复讨论）

- ✅ uiStore 拆分 + `plugins/sdk/types.ts` 1045 行拆成 12 个 domain 文件（Phase 1）
- ✅ ChatPanel 196 → 51 行 + 拆出 PanelHeader / ChatStream / WorkspaceViewBody；`<DataView>` 三态 render-prop（Phase 2）
- ✅ **开放扩展点底座（L2+L3）**：40 个命名贡献 map → 单一 `extensions` 底座 + `defineExtensionPoint`/`contribute`；删 ~24 个纯 spec-register host facade；capability-on-point + 风险分级（`capabilities.ts`）+ sideload 默认 deny（`pluginOrigin.ts`）+ `host.commands.execute` + `definePluginPack`。见 `docs/EXTENSION_POINTS.md`。
- ✅ **协议切到 Lyra Runtime Protocol v2**：`rpc/` 抽成注入式 SDK（`createLyraClient(transport)`）；stream 去重（`iterableOf`/`bindLifecycle`）；item-fold 收敛（started=append / completed=upsert 共用）；补 `approval-result` timeline emit。
- ✅ **未来内容块隔离**：search / code / checkpoint 移到可整体删除的 `chat/preview-blocks` 插件（`CustomContentBlockMap` augment + `MESSAGE_CITATION_SOURCE` 扩展点），核心不再硬引用；清掉孤儿 view 字段。
- ✅ **`components/chat` 拆三模块**：`message/`（消息内容渲染，内分 `markdown/` + `cards/`）· `panel/`（面板 + 流 chrome）· `composer/`（输入），各带 barrel 公开面。内置内容块改 message 模块内部直渲，registry 退为 Custom 扩展缝。
- ✅ **分层守卫修复**：`check-layers` / `check-circular` 改 alias-aware（之前漏看所有 `@/` 跨层边，是假绿）；加 `components`/`pages` 禁连 `main`/`rpc` 守卫。
- ✅ **域布局对齐**：`shell/sidebar`→`sidebar/`、`shell/defaults`→`defaults/`、`chat/tool-*`→`chat/tools/`；`plugins/` 根 glue → `plugins/host/`；core-reducer `helpers.ts` 拆 `projections.ts` + `fold.ts`；全仓清除残留 AG-UI 措辞。

> 下一波"值得做但要先决条件"的清单在 `frontend/ARCHITECTURE.md §12`。
