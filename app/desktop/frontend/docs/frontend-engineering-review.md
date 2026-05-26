# Lyra 前端工程化与项目结构 Review

日期：2026-05-26

范围：`frontend/` 工程化、目录结构、构建/测试/lint/依赖/文档治理。已按当前 `CLAUDE.md` 的 VoidZero 栈和工程约定 review。

## 1. 总体判断

这轮工程化调整是实质性进步。`package.json` 已经明确提供 `typecheck`、`lint`、`test`、`knip`、`check`，并且 `knip` 已固定为 devDependency，不再依赖临时 `npx` 下载。当前工具链和 `CLAUDE.md` 基本对齐：

- React 19
- Vite 8 / Rolldown
- Vitest 4
- OxLint 1.x
- TypeScript 6
- Tailwind 4
- Knip 6

本轮验证结果：

- `npm run check`：通过。
- `npm run build`：通过。
- Vitest：21 个 test files / 228 tests 通过。
- 构建仍有 CSS Highlight API warning 和大 chunk warning。

当前工程化主要问题已经从“缺脚本”变成“缺治理实体”：仓库没有发现 CI workflow / pre-commit 配置；bundle budget 没有落地；`styles/` 虽然已收敛到 5 个 CSS 文件，但 Tailwind first 的迁移边界还需要持续守住。

## 2. 工具链

### 已对齐

`frontend/package.json` 当前脚本完整：

```json
"typecheck": "tsc --noEmit",
"lint": "oxlint src",
"test": "vitest run",
"knip": "knip",
"check": "npm run typecheck && npm run lint && npm run test && npm run knip"
```

这比上一轮更适合 agent 协作，因为任何修改后都有单一入口验证。

其他已对齐项：

- `engines.node >=22.12.0` 明确。
- `vitest.config.ts` 单独维护，不复用 Vite config，避免测试环境拉入 Wails/browser-only 逻辑。
- `tsconfig.json` 使用 `strict`、`isolatedModules`、`moduleResolution: Bundler`。
- `knip.json` 已忽略 `wailsjs/**` 和 `sample-plugins/**`。
- Vite config 保持轻量，只挂 React、Tailwind 和 `@` alias。

### 仍缺失

1. 没有 CI workflow / pre-commit 实体。

   `CLAUDE.md` 写了 CI / pre-commit 跑 `tsc + oxlint + vitest + knip`，但仓库根目录未发现 `.github/workflows`、lefthook、husky、lint-staged 等配置。建议最小先加 GitHub Actions：

   - `npm ci`
   - `npm run check`
   - 可选 `npm run build`

2. `.npmrc` 仍使用 `legacy-peer-deps=true`。

   这在 React 19 升级阶段可以理解，但应该写清原因和退出条件。否则它会长期掩盖 peer dependency 冲突。

3. build warning 没有收敛策略。

   当前 `::highlight(chat-search)` 和 `::highlight(chat-search-active)` 被 Lightning CSS 反复 warning。即使这是 CSS Highlight API 的兼容问题，构建日志长期带 warning 会降低信号质量。

## 3. 目录结构

当前 `frontend/src` 的顶层结构合理：

- `pages/`：入口页面和 kernel shell。
- `plugins/`：插件 SDK、runtime、builtin plugins。
- `protocol/agui/`：AG-UI reducer、custom event schema、view state。
- `state/`：Zustand stores。
- `components/`：共享 UI 和纯展示组件。
- `domain/`：types-only business models / outbound contracts。
- `infra/`：gateway implementation。
- `main/`：composition root。
- `lib/`：横切工具和 adapter。
- `styles/`：少量历史/全局 CSS。

整体不需要拆 monorepo。当前触发不了拆分条件：没有多包发布、没有多运行时产物、没有独立版本节奏，也没有明显的包级边界治理收益。

### 项目结构风险

1. `plugins/builtin` 数量持续增加。

   当前一级 builtin plugin 目录为 41 个。中期建议把 manifest 按领域拆分：

   - protocol / reducer
   - shell / layout
   - composer / commands
   - workspace views
   - settings / appearance
   - diagnostics / infrastructure

   总入口仍可保留 `plugins/builtin/index.ts`，但它只做分组汇总。

2. `lib/` 的层级语义偏混合。

   `lib/http.ts` 是 plugin-aware RPC facade；`lib/useApprovalSubmit.ts` 是 UI hook；`lib/smoothText.ts` 是纯工具；`lib/queryClient.ts` 是 runtime singleton。建议后续不要把 `lib/` 当成“内层共享库”，而要按性质迁移：

   - 纯函数工具可以留 `lib/`。
   - UI hook 可移到 feature/use-case 附近。
   - transport facade 可移到 `infra` 或 `plugins/sdk/runtime`。
   - app singleton 可移到 `main` 或 runtime。

3. `styles/` 已收敛但仍需守边界。

   当前 CSS 文件只有 5 个：

   - `globals.css`
   - `layout.css`
   - `markdown.css`
   - `overlays.css`
   - `tool.css`

   这比上一轮健康很多。建议把规则表述为：历史/全局/Markdown/Shiki/overlay chrome 可保留 CSS；新组件样式全部进 Tailwind className，不新增 CSS 文件。

4. 大文件正在接近拆分阈值。

   当前非测试 TS/TSX 最大文件：

   - `plugins/builtin/core-reducer/handlers.ts`：544 行。
   - `plugins/sdk/host.ts`：518 行。
   - `plugins/sdk/selectors.ts`：473 行。
   - `plugins/sdk/registry.ts`：430 行。

   `handlers.ts` 和 `host.ts` 已超过 500 行，建议进入下一轮小型重构候选。不要为拆而拆，优先按 namespace / event family 做低风险拆分。

## 4. 构建与产物

`npm run build` 通过，Vite 8/Rolldown 构建约 1.82s。构建速度很好，主要问题是产物治理。

### 当前构建信号

- CSS：`index-*.css` 约 110 KB / gzip 22 KB。
- 主入口：`index-*.js` 约 3.18 MB / gzip 830 KB。
- 大依赖 chunk：`dist-*.js` 约 1.53 MB / gzip 469 KB。
- WASM chunk：约 622 KB / gzip 232 KB。
- Shiki language/theme chunks 数量很多，但多数是按需小 chunk。

### 建议

1. 增加 bundle budget。

   最小目标：

   - shell entry gzip。
   - markdown/katex gzip。
   - shiki core + top languages gzip。
   - diagnostics runtime gzip。

2. 命名关键 chunk。

   默认 `index` / `dist` 不利于归因。建议使用 Vite 8 / Rolldown 对应的 chunk splitting 配置，把 React、markdown/katex、shiki、otel/diagnostics 拆出可读名称。

3. 处理 `::highlight` warning。

   如果该语法必须保留，建议集中放在一个 CSS 位置并加注释说明 Lightning CSS warning 的接受原因；如果可替换，优先改写，保持 build 日志干净。

## 5. 测试与质量门禁

当前验证闭环健康：

- TypeScript：通过。
- OxLint：通过。
- Vitest：21 files / 228 tests 通过。
- Knip：通过。
- Build：通过但有 warning。

建议补强：

1. CI 跑 `npm run check`。
2. CI 增加 `npm run build`，至少在主分支和 PR 上跑。
3. 对 `architecture.test.ts` 继续补规则：`main` 是唯一 importer of `infra`，`domain` 禁止 import `@/lib` 已有覆盖但应保持。
4. 对 bundle budget 建一个非阻塞报告，稳定后再考虑 fail build。

## 6. 依赖治理

当前依赖选择基本符合项目约定：

- HTTP：`ky`
- 状态：`zustand`
- server cache：`@tanstack/react-query`
- command palette：`cmdk`
- toast：`sonner`
- icon：`lucide-react`
- highlight：`shiki`
- i18n：`i18next + react-i18next`
- schema：`zod`

风险点：

1. `legacy-peer-deps=true` 需要文档化。
2. `@opentelemetry/sdk-metrics` 是运行时依赖，放 dependencies 合理，但要继续确保 diagnostics 不进首屏静态路径。
3. Shiki 语言 chunk 数量多，需要靠 lazy load 和 budget 观察，不建议盲目手写高亮器。

## 7. 推荐工程化改动顺序

P0：

1. 增加 CI workflow，跑 `npm ci && npm run check && npm run build`。
2. 处理或显式接受 `::highlight` build warning。
3. 在长期文档里解释 `.npmrc legacy-peer-deps=true` 的原因和退出条件。

P1：

1. 建 bundle budget 报告。
2. 给关键 chunks 命名并做归因。
3. 拆分 `host.ts` / `core-reducer/handlers.ts` 的自然边界。

P2：

1. builtin manifest 分组。
2. 继续收敛 `lib/` 的层级语义。
3. 给 `frontend/docs` 加 README，区分快照 review 和长期规范文档。
