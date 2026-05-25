# CLAUDE.md — project context for Claude Code

> **Lyra** — Wails 桌面应用（Go 后端 + React/TS 前端）。AG-UI 流式协议驱动的插件化 agent client。
> 详细架构看 `frontend/ARCHITECTURE.md`（967 行），设计系统看 `frontend/DESIGN.md`。

---

## 一句话定位

**Kernel 不长肉，所有功能都是插件。** 路由、布局、内容渲染、命令、快捷键、主题、AG-UI 事件处理、设置面板 —— 全部由 `frontend/src/plugins/builtin/` 下的 37 个插件贡献。Kernel 自己只是一组命名 Slot + 几个共享 Zustand store。

## 技术栈

- **UI**: React 18 + TypeScript
- **样式**: **Tailwind 4** + `cva`（class-variance-authority）+ `clsx` + `tailwind-merge`。**所有新代码必须用 Tailwind utility class，不再写新的 .css 文件。** 全局样式只剩 `src/styles/globals.css`（Tailwind base + `@theme` token + 全局 keyframes）。
- **Headless 基件**: **Radix Primitives first**（Dialog / Popover / Select / Tooltip / DropdownMenu / ContextMenu / Tabs / 等）。**Radix 没有的才考虑别的库或自己写。** 不用 shadcn/ui npm 包，但可以借鉴它的 className 字符串作为起点。
- **特定组件库**:
  - `cmdk` — Cmd+K 命令面板（Linear / Vercel / Cursor 同款）
  - `sonner` — Toast 通知
  - `lucide-react` — 图标（已在用，继续）
- **状态**: Zustand 多 store（无 context 链）
- **路由**: TanStack Router（动态构建）
- **数据**: TanStack React Query
- **协议**: `@ag-ui/core` / `@ag-ui/client`
- **桌面壳**: Wails v2
- **测试**: Vitest + Testing Library

## 三大支柱

1. **插件系统** (`frontend/src/plugins/sdk/` + `builtin/`) — `definePlugin({ name, version, setup({ host }) { host.X.register*(...) } })`。Host 有 31 个 `register*` 方法分布在 16 个 namespace（tool / message / agui / layout / workspace / theme / composer / sidebar / commands / …）。
2. **AG-UI 协议层** (`frontend/src/protocol/agui/`) — reducer 是纯派发器，把 core events 路由到 `host.agui.onCore(...)` 注册的 handler chain，CUSTOM events 路由到 `host.agui.on(...)`。所有协议语义都在 `core-reducer` 插件里。
3. **状态分层** (`frontend/src/state/`) — `agentStore`（每会话 view state，ephemeral）/ `themeStore`（持久化 `lyra.theme`）/ `layoutStore`（持久化 `lyra.layout`）/ `sessionStore`（部分持久化 `lyra.session`）/ `composerStore`（ephemeral）。

## 主题系统（IDE 风格）

10 个内置主题（lyra-dark/light + atom-one-dark/light + tokyo-night-storm/light + solarized-dark/light + catppuccin-mocha/latte），每个是一个独立 plugin 文件，用 `defineThemePlugin()` helper：

```ts
defineThemePlugin({
  id, label, scheme: "dark" | "light", order,
  palette: { "color-bg": "#...", "color-surface": "#...", ... },
});
```

helper 自动补 shadow ladder + CTA defaults + 注册仪式。加新主题 = 新文件 + 一行 manifest。Settings → Appearance 的 theme picker 从 registry 自动读列表。

## 强约定（违反 = 回归）

- **Tailwind first**：所有组件样式用 utility class。`style={{ ... }}` 内联样式只在 token 值真的动态时用（比如主题预览 swatch 的 `style={{ background: spec.tokens["color-bg"] }}`）。
- **Radix first**：需要 dialog / popover / select / dropdown / tooltip / context menu / tabs 这类带行为的组件时，**优先用 Radix Primitives**。不要手写 focus trap / aria 标签 / 键盘导航 —— Radix 已经做好了。
- **不写新的 .css 文件**：所有新样式进 JSX 的 className。`src/styles/globals.css` 是唯一例外（持有 Tailwind base + @theme token + 几个全局 keyframes）。
- **KISS / SOLID / YAGNI / DRY / 高内聚低耦合** 是判断标准。不为未来不存在的需求做抽象。组件按职责拆分（SRP），抽象只在 3+ 重复时引入，模块之间通过最小接口（plugin SDK / store selectors）通信。
- **目前在开发阶段，不需要任何 legacy 兼容代码 / 迁移路径**。store schema 变了就 bump version 丢弃旧数据，不写 migration。注释里也不写 "Legacy …" 之类的措辞。
- **No lines anywhere**（DESIGN.md "no lines" 原则）—— 区域分隔用 surface ladder（color-mix 派生的 `--color-surface` / `surface-2` / `surface-3`），不用 1px 边线。状态指示（active tab 底部 accent 横线、focus ring）允许。
- **Cards-on-canvas 布局** —— 外层 `--color-bg` 是暗 canvas，`.panel` 浮在上面用 `--color-surface`，8px gap + radius + shadow。
- **Tab hover === active** —— `.chat-tab` 用同一个背景（`color-mix(var(--color-text) 4%, transparent)`），只用底部 2px accent line 区分激活态。
- **Plugin 一定要走 registry** —— 不要直接 import 一个 builtin plugin 去用，永远走 `useXxx()` / `lookupXxx()` 选择器。
- **AG-UI 事件单向**：render 路径不回写 agent store；要"做事"就调 store 上的 `send` / `stop`。
- **Disposable 由 Host 收集** —— 不要手动调 `dispose()`。
- **加文档？先问** —— 不要主动创建 `*.md`，除非用户明确要。CLAUDE.md / ARCHITECTURE.md / DESIGN.md 已经存在；其他文档默认不写。
- **不要重复造轮子（DRY）** —— 写工具函数前先查 npm。现有约定：HTTP 用 `ky`，状态用 `zustand`，缓存用 `@tanstack/react-query`，class 合并用 `clsx + tailwind-merge`（`cn()`），代码高亮用 `shiki`，国际化用 `i18next + react-i18next`，动画用 `motion/react`，命令面板用 `cmdk`，Toast 用 `sonner`，图标用 `lucide-react`，分词用 `Intl.Segmenter`（不需要 polyfill — Wails 2 WebView 都带）。**例外**：(a) 极小封装 < 10 行；(b) 应用专属业务逻辑（如 `useApprovalSubmit`）；(c) 对现有库的薄包装加上插件 hook（如 `lib/http.ts` 包 ky 加 RPC hooks）。如果一定要手写，注释里说明为什么社区库不够用。
- **每 3-5 个 feature 之后必须重构一次** —— 不是可选项，是节奏。每加 3-5 个新东西就停下来跑一遍：找最大的文件、找 3+ 重复 pattern、找误导性的注释、找累积的死代码、找新增 API 是否破坏了既有抽象。**目的**：避免熵增到尾大不掉。**信号**：单文件 > 500 行、`addX/removeX` 模式 > 3 处未抽象、新建插件需要改 kernel 多处。**做法**：上来先扫一遍代码体积 + 重复，给 3 项重构方案 + 权衡，用户确认后动手，每轮收尾要跑 `tsc + vitest + eslint` 全绿再 commit。
- **重构清单**（Fowler《重构》的实践）：每轮要扫的不只是拆分，还要包含 (a) **死代码清理**——`npx knip` 跑一遍找未引用的 export / 文件 / 依赖，**删干净**而不是留着"将来可能用"；(b) **卫语句替代嵌套 if**——`if (!x) return` 比层层缩进的 if/else 链可读 10 倍；(c) **查表法替代条件链**——三个以上 `if/else if` 或嵌套三元运算符都应该换成 `Record<K, V>` 的查表；(d) **精准命名**——`idCounter` → `nextCompositeKeyId`，`x` / `tmp` / `data` 不算名字；(e) **注释清理**——大段解释 what 的注释删掉、过期的迁移注释删掉、误导性的"为什么这样写"（实际不再这样了）删掉，留下来的只解释 _why_ 而不是 _what_；(f) **性能扫描**——`useMemo` deps 是否引用稳定、`Map`/`Set` 还是数组，循环里有没有 N² 的 `Array.find`。
- **边界校验用 Zod，内部数据流不加** —— 信任边界（sideload plugin spec / AG-UI custom event payload / 用户输入的 URL 等）必须用 Zod schema 拒掉脏数据，schema 写在 `src/protocol/agui/schemas.ts` 或就近文件，统一用 `withSchema(name, schema, handler)` 包装。内部流（Zustand store / React props / 已被 `@ag-ui/core` 类型校验的 core events / 已被 SDK 类型守护的 plugin SDK 调用）**不加 Zod** —— TS 已经够了，再加是噪音 + bundle 浪费。

## 强反向不变量（已知错的方向，别再提）

- ❌ 把 Zustand 换成 Redux Toolkit / Effector / Jotai —— 现有 5 个 store 都很小，订阅模型够用
- ❌ 给内部数据流（builtin plugin spec / React props / Zustand store / 已被 TS 守护的调用）加 Zod —— Zod 只用在信任边界（见上面"边界校验"规则）。内部用 Zod 是 bundle 浪费 + 维护负担
- ❌ 把 React Query 换 SWR / RTK Query —— 切框架本身无收益
- ❌ 把 Wails 换 Tauri —— 没实际 bug
- ❌ ~~把 CSS 换 Tailwind / CSS-in-JS~~ —— **已经换了，方向是 Tailwind 4 + Radix。见上面"技术栈"段**
- ❌ 引入完整 UI Kit（shadcn-as-npm / HeroUI / DaisyUI / Catalyst / ReUI）—— 都会跟我们的设计语言打架。`shadcn/ui` 是 copy-paste 模式可以借鉴 className 字符串，但**不引 npm 包**
- ❌ 换 Base UI —— 评估过，Radix 在 AI 工具协作 + 社区资料量上明显领先
- ❌ 把 `registry.ts` 30+ 对 `addX/removeX` 抽 factory —— 现有 `addOwned` / `addOwnedMulti` 已经抽完真重复；剩下的 per-slot wrapper 是 type-safety 成本
- ❌ 把 `domain/infra/main/` 拆成 monorepo —— 4 个触发条件都没命中（见 ARCHITECTURE.md §3.2）

## 关键目录

```
frontend/src/
├── pages/AgentClientPage.tsx          kernel — 4 个 Slot 而已
├── plugins/
│   ├── sdk/
│   │   ├── types/                     12 个 domain 文件 + barrel
│   │   ├── host.ts                    createHost(pluginName)
│   │   ├── registry.ts                usePluginStore + addOwned / addOwnedMulti / clearByPlugin
│   │   ├── selectors.ts               所有 useXxx / lookupXxx
│   │   └── definePlugin.ts            loadPlugin
│   └── builtin/                       37 个插件，按 domain 分组
├── protocol/agui/                     reducer + viewState + customEvents
├── state/                             agentStore / themeStore / layoutStore / sessionStore / composerStore
├── components/
│   ├── chat/                          ChatPanel(51 行 orchestrator) + ChatHeader + ChatStream + WorkspaceViewBody
│   └── common/                        Icon / Panel / DataView(三态 render-prop) / …
├── domain/                            清洁架构：types-only contracts
├── infra/                             domain gateway 的 HTTP 实现
├── main/container.ts                  composition root（DI 单例）
└── styles/                            14 个 CSS 文件，theme tokens 在 tokens.css

internal/agui/                         Go AG-UI mock server，监听 :17171
```

## 常用命令

```bash
# 开发
wails dev                # 在 /Users/tangerg/Desktop/lyra/ 跑，自动启 vite + Go backend

# 测试 / 类型检查（在 frontend/ 跑）
cd frontend && npx tsc --noEmit
cd frontend && npx vitest run

# 当前测试数：175 cases / 18 test files
```

## 修改任何东西之前

1. **路径在 `frontend/src/plugins/builtin/<name>/index.ts(x)`**：动一个 builtin 插件，不会影响其他面
2. **路径在 `frontend/src/plugins/sdk/`**：动了 SDK 公开形状 → 跑 `vitest run` 验证所有插件还能注册
3. **路径在 `frontend/src/state/<store>.ts`**：动了 store schema → bump `version` 数字（zustand persist 自动丢旧数据）
4. **路径在 `frontend/src/protocol/agui/`**：动了协议层 → 重点跑 `reducer.test.ts`
5. **加一个主题**：见上文"主题系统"。一个新文件 + 在 `plugins/builtin/themes/index.ts` 加一行
6. **加一个插件**：`definePlugin({ ... })` → 在 `plugins/builtin/index.ts` 合适的分组加 import + 数组项

## 已经做过的大重构（避免重复讨论）

- ✅ uiStore 拆成 themeStore / layoutStore / sessionStore（Phase 1）
- ✅ plugins/sdk/types.ts 1045 行拆成 12 个 domain 文件（Phase 1）
- ✅ ChatPanel 196 → 51 行 + 拆出 ChatHeader / ChatStream / WorkspaceViewBody（Phase 2）
- ✅ `<DataView>` 三态 render-prop 替换 6 处 `loading | empty | content` 重复（Phase 2）
- ✅ smoothText pickRate 加 8 个单元测试（Phase 3）
- ✅ registry.ts 抽 `clearByPlugin` helper（Phase 3）
- ✅ 主题系统：10 个主题 + `defineThemePlugin` helper + Settings picker UI
- ✅ 所有 legacy 代码 / 注释 / 文档清除（dev phase 阶段）

下一波"值得做但要先决条件"的清单在 `frontend/ARCHITECTURE.md §12`。

## 沟通约定

- **中文回复**（用户偏好）
- 代码 / 注释保持英文
- 大重构前先给三步方案 + 权衡，等用户确认再动
- 改动后跑 `tsc + vitest`，commit message 写清"why"而不仅是"what"
- commit trailer 用 `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`
