# Lyra 前端实现细节 Review

日期：2026-05-26

范围：`frontend/src` 的命名、注释、局部实现、样式习惯、测试覆盖。本轮重点仍以宏观架构和工程化为主，但同步记录实现层会影响架构演进的细节问题。

## 1. 总体评价

实现层整体质量继续提升。相比上一轮：

- CSS 文件已从更分散的历史状态收敛到 5 个。
- `package.json` 已补齐 `typecheck/knip/check`。
- `domain/infra/main` 的最小 Clean Architecture 样例已经落地。
- `architecture.test.ts` 开始用测试保护依赖方向。
- Diagnostics store 测试仍保持在基线内，当前 Vitest 21 files / 228 tests 通过。

剩余问题主要是局部一致性：少数注释已经过期，inline style 还有一些静态样式，`host.ts` / `core-reducer/handlers.ts` 文件偏大，Clean Architecture 的命名和归属还需要在跨层模块上更精确。

## 2. 命名

### 做得好的命名

- `PermissionGateway` / `HttpPermissionGateway`：抽象和实现命名清楚，表达了 domain contract 与 transport implementation 的关系。
- `ApprovalSubmission` / `ApprovalDecision`：业务语义明确。
- `getContainer` / `setContainer` / `resetContainer`：测试替换点清楚。
- `addOwned` / `addOwnedMulti` / `clearByPlugin`：registry helper 命名贴合 ownership 语义。
- `nextCompositeKeyId`：比泛泛的 counter 更准确。

### 建议优化的命名

1. `lib/http.ts`

   这个文件不是普通 HTTP 工具，而是 plugin-aware RPC client：它读取 plugin config，也执行 plugin rpc hooks。建议中期改名或迁移为：

   - `plugins/sdk/runtime/rpcClient.ts`
   - 或 `infra/http/pluginRpcClient.ts`

   同时把 `AGUI_BASE` 这种默认配置从这里拆出去，避免 composition root 依赖 plugin runtime。

2. `domain/gateways`

   `gateways` 可以接受，但建议在长期文档中明确：这里放“domain/use case 需要的 outbound contracts”，不是 infra 抽象池。后续不要出现 `HttpGateway`、`ApiGateway` 这类按实现命名的抽象。

3. build chunk 名称

   `index-*.js`、`dist-*.js` 对工程排查不友好。建议用 chunk split 配置让产物名表达来源，例如 `vendor-react`、`markdown-runtime`、`shiki-runtime`、`diagnostics-runtime`。

## 3. 注释

### 应保留的注释

以下注释解释的是架构 why，值得保留：

- `domain/gateways/PermissionGateway.ts` 对 gateway contract 的解释。
- `infra/http/HttpPermissionGateway.ts` 对 transport 可替换性的解释。
- `main/container.ts` 对 composition root 和测试替换的解释。
- `protocol/agui/reducer.ts` 对 pure dispatcher 的解释。
- `plugins/sdk/host.ts` 对 runtime seam 避免循环 import 的解释。
- `plugins/sdk/registry.ts` 对 owned slot helper 的解释。

### 建议清理或改写的注释

1. `src/test/architecture.test.ts`

   文件头仍写 “we don't have ESLint configured yet” 和未来迁移 `eslint-plugin-boundaries`。当前项目已经使用 OxLint，这条注释不再准确。建议改成：

   - 架构边界是项目自定义规则，所以用 Vitest 扫 import。
   - 如果规则增长，再考虑专门的 boundary 工具。

2. `workspace-views/plan.tsx` / `workspace-views/terminal.tsx`

   仍有 TODO 表达“未来接真实数据”。如果短期不会做，建议转到 issue 或 review 文档，代码里只保留当前行为需要的注释。

3. `eslint-disable-next-line`

   当前仍有少数 `eslint-disable-next-line` 注释，但项目实际 lint 是 OxLint。建议统一改成工具无关的解释，或者确认 OxLint 是否识别这些规则；不要让注释长期停留在旧工具名上。

## 4. 样式实现

### 已改善

`styles/` 现在只有：

- `globals.css`
- `layout.css`
- `markdown.css`
- `overlays.css`
- `tool.css`

这更符合 Tailwind first 的方向。

### 仍需收敛

`style={{ ... }}` 仍有一些静态样式：

- `SidebarExpanded.tsx` 的 padding。
- `SearchResults.tsx` 的 line clamp。
- `workspace-views/terminal.tsx` 的 color / margin。
- `workspace-views/diff.tsx` 的 color / margin。
- `workspace-views/tools.tsx` 的 padding。

动态 swatch、动态尺寸、测量值可以保留 inline style；静态布局和颜色应迁到 Tailwind className。

## 5. Clean Architecture 实现细节

当前 permission flow 是一个好的模板：

```text
useApprovalSubmit -> getContainer().permission -> PermissionGateway
main/container -> HttpPermissionGateway
HttpPermissionGateway -> fetch /permission
```

这条链路里 UI 没有直接 import `infra`，domain 也没有反向依赖外层。

建议下一步补两类测试或规则：

1. `main` 是唯一允许 import `@/infra/*` 的层。

   当前 `architecture.test.ts` 已禁止 presentation import infra，但可以更直接地扫描全 `src`，确保除 `main` 和 `infra` 自身外没有其他 importer。

2. gateway contract 只能定义在使用方。

   这个规则不适合完全自动化，但可以靠 review checklist 落地：新增接口时必须回答“哪个 use case 需要它”。如果答案是“infra 实现方便”，就不该建这个接口。

## 6. 行为实现

### Plugin Host

`plugins/sdk/host.ts` 已到 518 行。它承担了所有 host namespace 的注册、runtime seam、capability restriction、toast/log/tasks/rpc/i18n。现在还没到必须重写的程度，但已达到拆分信号。

建议按 namespace 拆低风险文件：

- `hostRegistrations.ts`
- `hostRuntime.ts`
- `hostRpc.ts`
- `hostNotifications.ts`
- `hostCapabilities.ts`

拆分时保持 public `createHost()` 不变，避免影响插件调用面。

### Core Reducer Handlers

`plugins/builtin/core-reducer/handlers.ts` 已到 544 行。它是 builtin plugin 的实现细节，但继续增长会影响 review 和测试定位。

建议按 AG-UI event family 拆：

- run lifecycle
- text/message
- tool call
- reasoning
- custom helper

### Protocol Error Isolation

`protocol/agui/reducer.ts` 的 handler error isolation 是正确的。建议补测试覆盖：

- 某个 core handler throw，不阻断后续 handler。
- custom handler throw，会 report plugin error。
- throwing handler 不污染已累积的 state。

## 7. 测试

当前测试基线健康，但下一步应补这些高价值测试：

1. `architecture.test.ts` 增加 “only main may import infra”。
2. sideload plugin 未声明 capabilities 时默认 deny。
3. `ensureProvider()` 多次调用幂等，teardown 后可重建。
4. `useAgentSession` rAF batching 对 error / finish / permission event 的策略测试。
5. `PermissionGateway` fake 注入下 `useApprovalSubmit` 的成功/失败状态测试。

## 8. 具体清单

P0：

1. 改写 `architecture.test.ts` 过期 ESLint 注释。
2. 拆出 `AGUI_BASE`，让 `main/container` 不依赖 plugin-aware `lib/http`。
3. sideload capabilities 默认 deny 并补测试。

P1：

1. 静态 inline style 迁 Tailwind。
2. `host.ts` 按 namespace 拆分。
3. `core-reducer/handlers.ts` 按 event family 拆分。
4. build chunk 命名。

P2：

1. 清理长期 TODO。
2. 继续减少普通容器的硬边线用法。
3. 如果 SDK 要开放给外部插件，拆 public types 和 runtime host adapter。
