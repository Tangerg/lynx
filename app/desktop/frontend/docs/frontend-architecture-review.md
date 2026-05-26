# Lyra 前端宏观架构 Review

日期：2026-05-26

范围：仅 review `frontend/`。后端仍按 mock / 本地 AG-UI 服务看待，不评价 Go 后端架构。已先阅读根目录 `CLAUDE.md`，本 review 按当前约定执行：kernel 薄、功能插件化、Tailwind/Radix first、Zod 只放信任边界、VoidZero 工具链、当前不拆 monorepo。

## 1. 总体判断

这轮调整后，Lyra 前端的架构方向比上一轮更清晰：项目已经从“插件化 UI + AG-UI reducer”进一步补上了 `domain/infra/main` 的 Clean Architecture 边界，并用 `src/test/architecture.test.ts` 把关键依赖方向固化成测试。

当前整体判断：

- 插件化 kernel 仍是正确主轴。`pages/AgentClientPage.tsx` 足够薄，实际能力继续由 builtin plugins 贡献。
- AG-UI reducer 仍是纯派发器，协议语义由 `core-reducer` 等插件承载，和“Kernel 不长肉”一致。
- `domain/infra/main` 已经形成最小可用的依赖倒置样例：`PermissionGateway` 由 domain 定义，HTTP 实现在 infra，composition root 在 main。
- 工程化闭环明显增强：`typecheck/lint/test/knip/check` 已经落到 npm scripts，并且本轮全绿。
- 当前最大架构风险不是主方向错误，而是边界语义还不够稳定：`lib/http`、plugin SDK runtime、protocol dispatcher 这类跨层模块需要明确“它们是外层 adapter，不是内层 domain”。

结论：不建议换技术栈、不建议拆 monorepo、不建议把 Zod 扩散到内部流。下一步重点应该是把 Clean Architecture 的依赖方向扩展到更多业务用例，同时收紧 sideload 插件权限、route 能力边界和 bundle 治理。

## 2. 当前架构画像

### Kernel

`AgentClientPage` 继续只负责 Slot 级布局，sidebar、main、status、overlay、settings、workspace view、command palette 等能力都由插件注册。这个形态符合 `CLAUDE.md` 的核心原则：kernel 不承载具体功能，只承载命名 Slot 和共享 store。

### Plugin Runtime

`plugins/sdk` 是事实上的扩展内核：

- `Host` 暴露 tool / message / agui / layout / workspace / theme / router / composer / sidebar / commands / lifecycle / storage / rpc / plugins / tasks 等 namespace。
- registry 用 Map 保存所有 contribution，通过 selectors 暴露 `useXxx()` / `lookupXxx()`。
- `HostCapability` 已支持限制声明了 capabilities 的插件访问未授权 namespace。
- lazy activation 支持 command、declared view、declared settings pane 的占位注册。

这套机制适合当前“内置插件驱动产品能力”的目标。需要注意的是，`plugins/sdk/host.ts` 现在直接依赖 `lib/http`、`lib/i18n`、`sessionStore`、`tasksStore`、plugin registry。它是“宿主运行时 adapter”，不是一个可独立发布的纯 SDK。如果未来要开放第三方插件 SDK，建议拆成：

- `plugins/sdk/types/*`：纯公开 contract。
- `plugins/sdk/runtime/*`：Lyra 宿主实现，允许依赖 store、i18n、rpc、toast。

### Protocol

`protocol/agui/reducer.ts` 继续保持 dispatcher 角色：

- core event 通过 `host.agui.onCore(type, handler)` 注册的 handler chain 处理。
- CUSTOM event 通过 `host.agui.on(name, handler)` 路由。
- handler 抛错会被隔离，并通过 plugin error 机制记录。

从 Clean Architecture 角度看，当前 `protocol/agui` 不是内层 domain，而是 application/plugin runtime 的一部分，因为它依赖 plugin selectors 和 `reportPluginError`。这没有问题，但文档里应避免把它描述成纯业务内核。

### State

Zustand store 按职责拆分：

- `agentStore`：每会话 view state，承接 AG-UI 流式事件。
- `themeStore` / `layoutStore` / `sessionStore`：持久化 UI 状态。
- `composerStore`：输入状态。
- `tasksStore`：任务状态。
- plugin registry store：扩展贡献数据库。

当前 store 规模仍可控，没必要换 Redux / Effector / Jotai。

### Domain / Infra / Main

当前 Clean Architecture 样例集中在 HITL permission：

- `domain/models/Approval.ts`：业务模型。
- `domain/gateways/PermissionGateway.ts`：domain 定义的 outbound contract。
- `infra/http/HttpPermissionGateway.ts`：HTTP transport 实现。
- `main/container.ts`：composition root，把具体实现注入到 gateway slot。
- `lib/useApprovalSubmit.ts`：UI hook 通过 container 调 gateway，不直接 import `infra`。

这是正确方向。

## 3. Clean Architecture 专项判断

本轮用户明确要求增加规则：只准外层依赖内层，不能反向依赖；抽象应该定义在需要的地方。

### 当前做得好的地方

1. `domain/` 没有依赖 React、Zustand、plugin、protocol、infra、main、lib，也没有 `fetch/localStorage/sessionStorage`。
2. `infra/http/HttpPermissionGateway.ts` 只实现 domain contract，不被 UI 直接调用。
3. `main/container.ts` 是唯一 wiring 点，UI 通过 `getContainer().permission` 间接使用 gateway。
4. `architecture.test.ts` 已经把三条关键规则写成测试：
   - domain 不依赖外层。
   - infra 不依赖 UI/state/plugins/main/protocol。
   - presentation 不直接 import `@/infra/*`。

### 需要继续收紧的地方

1. `main/container.ts` 从 `@/lib/http` import `AGUI_BASE`。

   `lib/http.ts` 不是纯通用工具，它依赖 plugin config 和 rpc hooks，所以它本质上是 plugin-aware transport facade。composition root 依赖它拿常量会把 permission gateway 的默认配置和 plugin RPC facade 绑在一起。

   建议拆分：

   - `infra/http/baseUrl.ts` 或 `main/config.ts`：只放 mock backend 默认 base URL。
   - `lib/http.ts`：继续作为 plugin-aware RPC client，服务 `host.rpc`。

2. plugin SDK runtime 不应被误认为 inner layer。

   `plugins/sdk/host.ts` 依赖 store、i18n、toast、rpc、task，这是宿主 adapter 的合理职责。但如果未来要做第三方 SDK，抽象必须留在 `plugins/sdk/types`，宿主实现留在 runtime，避免外部插件 contract 反向依赖 Lyra 内部 store。

3. `protocol/agui/reducer.ts` 依赖 plugin selectors。

   这符合“所有 AG-UI 语义由插件贡献”的架构；但它意味着 protocol reducer 是外层应用编排，不是 domain。未来如果出现稳定业务用例，例如 session lifecycle、permission flow、agent run lifecycle，不应该把 domain contract 定义在 protocol reducer 里，而应定义在对应 use case/domain 侧。

4. `domain/` 目前仍偏窄。

   当前只有 permission flow 的 domain contract。只要后端仍是 mock，这可以接受。真实后端接入前，建议按实际用例逐步补：

   - `AgentRunGateway`
   - `SessionRepository`
   - `PermissionGateway` 的 query/status 能力

   不建议先做一套抽象大而全的 `ApiClient` interface。抽象应由调用方用例需要时定义。

### 建议写入长期架构规则

推荐在 `frontend/ARCHITECTURE.md` 或 `CLAUDE.md` 增加明确规则：

```text
依赖方向：
domain <- application/use case <- presentation/runtime/main <- infra

允许：
- domain 定义模型和它需要的 outbound interface。
- infra 实现 domain/application 定义的 interface。
- main/container 组装具体实现。
- UI/plugin/runtime 通过 container 或 registry 使用能力。

禁止：
- domain import React/Zustand/plugin/protocol/infra/lib/main。
- presentation 直接 import infra。
- infra 反向 import UI/store/plugin runtime。
- 为 infra 方便而在 infra 里定义业务接口，再让 domain/use case 依赖它。

抽象位置：
- 谁需要抽象，谁定义抽象。
- Permission flow 需要提交审批，所以 PermissionGateway 定义在 domain。
- HTTP 只是实现细节，所以 HttpPermissionGateway 放 infra。
- 不为尚不存在的后端能力提前创建 Repository/Gateway。
```

## 4. 主要架构风险

### P0：sideload 插件权限仍应默认 deny

`restrictHost` 只有在 plugin 显式声明 `capabilities` 时生效；省略时仍拿 full host。对 builtin plugin 可以 full trust，但对 sideload plugin 不够安全。

建议：

- built-in 插件保持 full trust。
- sideload 插件默认 deny，必须声明 capabilities。
- 插件管理页展示 origin、capabilities、load result、errors。
- 高风险 namespace 单独标记：`rpc`, `plugins`, `router`, `layout`, `agui`, `storage`, `window`, `tasks`。

### P1：动态路由仍不是完整热插拔 surface

`AppRouter` 的 route tree 构建方式决定了 sideload 后注册 route 的能力边界需要被明确。当前更稳妥的定位是：

- startup route 可以由插件注册。
- sideload 后的动态内容优先走 workspace view / settings pane / command。
- 如果要支持动态 route，增加稳定宿主 route，例如 `/plugin/$viewId`，内部再查 registry。

### P1：StrictMode 仍需要验证路径

React 19 下更应该验证副作用幂等性。插件 loader、agent subscription、Diagnostics provider、Zustand persist 都可能受重复 mount/unmount 影响。

建议先补 harness/test，不急着直接打开入口 StrictMode：

- `PluginProvider` 重复 mount 不重复注册。
- `useAgentSession` 重复 mount 不重复启动 run。
- Diagnostics `ensureProvider` 多次调用只安装一次，teardown 后可重建。

### P1：bundle budget 仍缺治理

本轮 `npm run build` 通过，构建约 1.82s，但仍有大 chunk：

- `index-*.js`：约 3.18 MB / gzip 830 KB。
- `dist-*.js`：约 1.53 MB / gzip 469 KB。
- `wasm-*.js`：约 622 KB / gzip 232 KB。

建议建立 bundle budget，不一定立即 fail build，但要记录 shell、markdown/katex、shiki、diagnostics 的目标体积和归因。

### P2：builtin plugin manifest 会继续膨胀

当前 `plugins/builtin` 一级目录为 41 个，`plugins/builtin/index.ts` 仍是集中 manifest。短期可接受，中期建议按能力分组 manifest，再由总入口汇总，避免中央清单变成维护瓶颈。

## 5. 优先路线

P0：

1. sideload plugin capability 默认 deny。
2. 拆出 `AGUI_BASE` 配置，避免 `main/container` 依赖 plugin-aware `lib/http`。
3. 把 Clean Architecture 依赖方向和“抽象定义在使用方”写进长期架构文档。

P1：

1. StrictMode harness。
2. 动态 route 能力定界。
3. bundle budget 和 chunk 归因。
4. 继续扩大 `architecture.test.ts`，覆盖 `main` 是唯一 infra wiring 点。

P2：

1. 真实后端接入前按用例补 domain gateway/repository。
2. builtin manifest 分组。
3. 如果 plugin SDK 要对外开放，拆 public contract 和 host runtime。

## 6. 不建议

- 不建议现在拆 monorepo。
- 不建议把 `domain` 扩成空泛的“大业务层”。
- 不建议创建通用 `ApiClient` interface 作为所有后端访问的抽象。
- 不建议让 UI 或 plugin 直接 import `infra`。
- 不建议把 Zod 放到 React props、Zustand store、内部 plugin contribution 这类已由 TS 守护的数据流。
