# Lyra 前端宏观架构 Review

日期：2026-05-26

范围：仅 review `frontend/`。后端仍按 mock / 本地 AG-UI 服务看待，不评价 Go 后端架构。已先阅读根目录 `CLAUDE.md`，本 review 按其中的 Tailwind/Radix first、插件 registry、Zod 边界校验、KISS/SOLID/YAGNI、当前不拆 monorepo 等约定执行。

## 1. 总体判断

Lyra 前端的架构方向是成立的，而且这次仓库调整后比上一轮更接近“插件化 agent client 内核”的目标态。

当前核心形态可以概括为：

- `AgentClientPage` 是薄 kernel，只负责 Slot 布局。
- `plugins/sdk` 是扩展内核，Host + registry 管理命令、视图、主题、AG-UI handler、工具预览、设置面板等 surface。
- `protocol/agui` 保持纯派发器，语义处理由内置插件贡献。
- `state/` 负责运行期 Zustand store，agent view state 已按 session 隔离。
- `domain/infra/main` 已用测试守住 outbound gateway 边界。

这轮明显新增或强化了几项关键能力：

- `src/lib/metrics.ts` + `plugins/builtin/diagnostics` 引入了可观测性基础设施。
- `useAgentSession` 增加 rAF event batch，`agentStore` 增加 `applyEvents`。
- `selectors.ts` 对 AG-UI handlers 和 layout slots 增加二级索引，减少热路径扫描。
- `protocol/agui/schemas.ts` + `withSchema` 把 CUSTOM event payload 校验放到了信任边界。
- `HostCapability` + `restrictHost` 已能按 `PluginSpec.capabilities` 限制 host namespace。
- Mermaid 和 Shiki 已改成动态 import，构建产物开始拆出懒加载 chunk。

结论：现在优先级不再是“补齐第一层架构能力”，而是进入第二阶段：把这些能力做成可靠的平台机制，包括默认权限策略、按需激活、bundle budget、StrictMode 生命周期和文档一致性。

## 2. 验证结果

本轮验证：

- `npm run test`：20 个测试文件，220 个用例通过。
- `npm run lint`：通过。
- `npm run build`：通过。

构建仍有两类信号：

- CSS 优化阶段仍提示 `::highlight(chat-search)` / `::highlight(chat-search-active)` 不是被识别的有效 pseudo-element。
- 仍存在大 chunk：约 `1.61 MB` / gzip `492 KB`，以及约 `3.16 MB` / gzip `828 KB`。相比上一轮单个主包约 `4.91 MB` 已改善，但还没有形成明确 chunk budget。

## 3. 已改善的架构点

### 3.1 可观测性从“没有体系”变成“有入口”

`lib/metrics.ts` 把 reducer、Markdown remend、Shiki、Mermaid、plugin load 的测量统一封装起来。Diagnostics 插件通过 OpenTelemetry SDK exporter 写入本地 Zustand store，再用 Workspace View 展示。

这是正确方向：调用点不直接依赖具体 exporter，未来可以替换成更正式的 telemetry 后端，或者保留为 dev-only 插件。

### 3.2 流式事件处理开始具备背压

`useAgentSession` 使用 `requestAnimationFrame` 合并 AG-UI events，`agentStore.applyEvents` 在单次 `set()` 里顺序 reduce。这个改动对流式 token、tool args、reasoning delta 很关键，能把“每个 delta 一次 React commit”降为“每帧最多一次 commit”。

下一步不应盲目再优化，而应使用 Diagnostics 数据判断 reducer、render、Markdown、Shiki 哪个环节仍然占主导。

### 3.3 Registry 热路径已索引化

`selectors.ts` 的 `createIndex` 用 WeakMap 绑定 source Map 引用，解决了 `lookupCoreEventHandlers`、`lookupCustomEventHandlers`、`useLayoutSlot` 的全表扫描问题。这个实现符合当前 registry “每次 mutation 生成新 Map”的模型，缓存失效方式简单清晰。

### 3.4 边界校验位置正确

`schemas.ts` 明确写了“Zod 只用于 CUSTOM AG-UI event 这类信任边界”，内置 agui handlers 用 `withSchema` 包裹。这个符合 `CLAUDE.md` 约定，没有把 Zod 扩散到 React props / Zustand 内部数据流。

### 3.5 插件 capability 有了运行期约束

`HostCapability` 和 `restrictHost` 已经不是纯类型字段：声明了 capabilities 的插件访问未声明 namespace 会抛清晰错误，并有测试覆盖。这为后续 sideload 插件权限治理打下了基础。

## 4. 仍存在的宏观风险

### P0：Diagnostics 插件当前是 eager setup，不是真正按需

`plugins/builtin/index.ts` 把 `diagnostics` 放在 `panes` 数组里，内置插件启动时会 eager load。`diagnostics/index.tsx` 的 `setup()` 内部 fire-and-forget 动态 import `@opentelemetry/sdk-metrics`，所以它不阻塞 builtinsReady，但仍会在启动后主动拉 SDK chunk，并注册全局 MeterProvider。

这和注释里“only loads when this plugin's setup() runs” technically 一致，但从产品语义看，Diagnostics view `openByDefault: false` 并不等于 diagnostics runtime 按需加载。用户没打开 Diagnostics 时，它也已开始采集。

建议：

- 短期：明确这是“dev 默认开启”的诊断插件，并提供 config 开关。
- 中期：增加 `onView:diagnostics` activation event，或提供 workspace view lazy activation，让 Diagnostics 的 SDK import 和 MeterProvider 只在打开视图时发生。
- 长期：区分 always-on low-cost counters 与 opt-in heavy diagnostics backend。

### P0：sideload 插件权限仍是 opt-in，不是默认拒绝

`capabilities` 省略时仍返回 full host，这是为了保留现有行为，内置插件也大量依赖这个默认值。但对于 sideload 插件，这意味着权限边界仍然是自愿声明，不是安全策略。

建议：

- built-in 插件可继续 full trust。
- sideload 插件默认最小权限或默认无权限，必须声明 capabilities。
- 插件管理页展示 declared capabilities、origin、加载状态和错误。
- 高风险 namespace 单独标注：`rpc`, `plugins`, `agui`, `layout`, `router`, `storage`, `window`。

### P1：构建拆包已经开始，但还没有 budget 和 chunk 策略

Shiki、Mermaid、Diagnostics 已动态 import，构建产物比上一轮更健康。但 `vite.config.ts` 仍没有 `manualChunks` 或 chunk budget，build 仍提示大 chunk。

建议：

- 建立明确 budget，例如 shell 首屏 JS gzip 目标、lazy heavy chunk 单独目标。
- 用 `manualChunks` 把 React/vendor、markdown/katex、plugin-runtime、workspace-heavy、diagnostics 分开。
- 对 `@opentelemetry/sdk-metrics`、`react-markdown`/rehype、Shiki highlighter 做产物归因。
- CI 中至少记录 bundle size，后续再决定是否 fail build。

### P1：StrictMode 仍关闭，生命周期幂等性还没有被证明

`main.tsx` 仍明确关闭 StrictMode。考虑到插件 loader、agent subscribe、Zustand persist、Diagnostics MeterProvider 都依赖副作用顺序，StrictMode 关闭仍是一个真实架构风险。

建议：

- 先不强行打开入口 StrictMode。
- 建一个测试用 StrictMode harness，覆盖 `PluginProvider`、`useAgentSession`、Diagnostics plugin setup/unload。
- 目标是证明重复 mount/unmount 不重复注册 builtins、不重复启动 agent run、不遗留 MeterProvider 或 subscriptions。

### P1：动态路由仍是一阶段构建

`AppRouter` 首次 render 时构建 router，sideload 后到的 route 不会热注册。这个和“插件化外壳”的方向存在能力不一致：workspace view、command、layout slot 可动态，route 不可动态。

建议二选一：

- 如果当前路线是不支持 sideload route，写入 SDK 文档和 `RouteSpec` 注释。
- 如果要支持插件页面，优先考虑稳定宿主 route，如 `/plugin/$viewId`，动态内容仍从 workspace/view registry 取，避免动态重建 TanStack route tree。

### P1：宏观文档有漂移

`frontend/ARCHITECTURE.md` 和 `CLAUDE.md` 有几处与当前实现不完全一致：

- `CLAUDE.md` 常用命令里的测试数仍写 175 cases / 18 files，当前是 220 cases / 20 files。
- `ARCHITECTURE.md` 目录速览仍提到 `style.css`、`styles/app.css`、`utils/`、`mockScript.ts` 等当前文件结构不完全匹配的项。
- built-in 插件数量和新增 diagnostics 没同步。
- `CLAUDE.md` 说 styles 目录 14 个 CSS 文件，但当前实际是 8 个 CSS 文件。

建议把文档同步作为一次小型维护任务处理。这里不是吹毛求疵，架构文档是插件作者和后续 agent 的入口，漂移会直接诱导错误改动。

### P2：Clean Architecture 仍只覆盖 PermissionGateway

`domain/infra/main` 边界仍然主要服务 Approval/Permission。后端目前是 mock，所以这不是问题。但真实后端接入前，建议提前把稳定 contract 梳理出来：

- `AgentGateway`：run/stop/resume/subscribe。
- `SessionRepository`：list/create/rename/archive。
- `PluginRepository`：list/enable/disable/origin/capability。
- `PermissionGateway`：保留现有方向，补 query/status。

这不要求现在拆包，也不要求 monorepo。按 `CLAUDE.md` 约定，当前不应提前拆 monorepo。

## 5. 建议路线

第一优先级：

1. Diagnostics 改成真正按需或可配置启用。
2. sideload 插件 capability 默认拒绝，内置插件保留 full trust。
3. 建 bundle budget 和产物归因，先记录再收敛。

第二优先级：

1. StrictMode harness。
2. route surface 能力说明或稳定宿主 route。
3. 同步 `CLAUDE.md` / `ARCHITECTURE.md`。

第三优先级：

1. 真实后端接入前补 domain gateway contract。
2. 根据 Diagnostics 数据再决定是否做消息虚拟化、Markdown worker、Shiki 语言白名单缩减等更重优化。

## 6. 不建议做的事

- 不建议换 Zustand / React Query / Wails / UI Kit。当前问题不是框架选型。
- 不建议现在拆 monorepo，`CLAUDE.md` 的触发条件仍未命中。
- 不建议把插件系统回退成硬编码页面。插件系统是 Lyra 的核心架构资产。
- 不建议在内部 store/props 全面加 Zod。当前 `withSchema` 放在 wire boundary 是正确位置。
