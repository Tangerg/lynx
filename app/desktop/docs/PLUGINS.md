# 插件机制 — 设计提案 v2

> 状态：**提案 / RFC**，尚未实现。
> 负责人：待定 · 最后修订：2026-05-19
> 上一版：v1（含 `plugin.json` 的双文件方案），已废弃。

本文说明第三方（以及我们自己作为内置扩展）如何在不 fork 主仓库的前提下扩展 Lyra 的能力。**Lyra 是一个纯前端 UI 壳，真正的 agent 业务在 Go 服务端**，两者通过 RPC/HTTP 通信。这个架构事实贯穿整份 doc，决定了插件能管什么、不能管什么。

参考实现：[pi-mono](https://github.com/earendil-works/pi-mono) 的 Extension 系统 —— 它把"扩展不是特权层，是普通调用"做到了极致，本文借了它若干思路并标注了出处。

---

## 一、架构定位

```
┌──────────────────────────────────────────────────────────┐
│  Go 端 (lyra/internal/agui)                              │
│  ── AG-UI 协议服务端 + 真正的 agent 业务逻辑              │
│  ── 工具执行（bash / read_file / edit_file / ...）        │
│  ── MCP server 接入（扩展工具的入口）                     │
│  ── REST 端点（/sessions /diff /terminal /...）           │
└──────────────────────────────────────────────────────────┘
        ▲                       ▲
        │ HTTP/SSE              │ HTTP/JSON
        │ (AG-UI events)        │ (react-query)
        ▼                       ▼
┌──────────────────────────────────────────────────────────┐
│  前端 (frontend/)                                         │
│  ── 渲染 AG-UI 事件流                                     │
│  ── 渲染各种 artifact（diff / terminal / files / plan）   │
│  ── 输入框 + 命令 + 主题                                  │
│  ── ★ 插件在这里 ★                                        │
└──────────────────────────────────────────────────────────┘
```

**插件只活在前端**。这是核心约束，由它推导出后面所有的取舍。

具体含义：

| 想做的事 | 走哪条路 |
|---|---|
| 加一个新工具让 agent 调用（比如 `kubectl`） | **MCP server**（后端事），不是 Lyra 插件 |
| 改变 agent 怎么思考、怎么选择工具 | **Go 端 agent 配置**，不是 Lyra 插件 |
| 让 `kubectl` 工具输出渲染成资源表格 | **Lyra 插件**：注册一个 ToolPreview |
| 给 inspector 加一个"Profiler"标签 | **Lyra 插件**：注册一个 Inspector Tab |
| 接入自建的 LLM 网关 | **Go 端 transport 替换**，不是 Lyra 插件 |
| 把 AG-UI `CUSTOM` 事件渲染成图表 | **Lyra 插件**：注册一个 CUSTOM 事件 handler + 渲染 |
| 加一个 `/lint` slash 命令 | **Lyra 插件**：注册 command（command 体内可以通过 `host.rpc` 回调后端） |

---

## 二、目标与非目标

**目标**

- 一个人用一个下午写完一个**前端 UI 扩展**（仅 TypeScript）。
- 让消息渲染、inspector、composer、主题都可扩展，不需要 fork 前端。
- 宿主必须稳定：插件出 bug 不能炸主应用。
- 扩展看起来要像原生：复用 DESIGN.md tokens、复用 motion、复用通用组件。

**非目标（v1，明确不做）**

- **后端扩展**。后端能力扩展走 MCP 协议（标准化）或直接改 Go 代码。Lyra 插件 = 客户端体验。
- **JS 沙箱 / Worker 隔离**。trust-on-install。pi-mono 也是这个立场（`coding-agent.md:605`：「无显式权限模型」）。
- **插件市场 / 注册中心**。内置 + 本地 sideload + npm 包 dispatch 足够 v1 用，先看实际需求。
- **跨产品通用**。Lyra 插件就只是 Lyra 的，不假装是某种通用规范。

---

## 三、用例

整份 doc 围绕五个真实场景。如果你的想法不在这些套路里，那大概率"还不需要做成插件"，或者**应该做在后端**。

| 场景 | 现状 | 用插件之后 |
|---|---|---|
| 把 `kubectl` 工具的输出渲染成彩色资源列表 | `bash` preview 只能看到原始文本 | 注册 `host.tool.registerPreview("kubectl", ...)` |
| 给 inspector 加一个 "Profiler" 面板 | inspector 标签是硬编码的 | `host.inspector.registerTab({ id: "profiler", ... })` |
| `/lint` 命令把 diff 喂给后端的 eslint endpoint | 没法加新命令 | `host.composer.registerCommand("/lint", ...)`，内部 `host.rpc.post("/lint", { diff })` |
| 自定义事件 `monitoring.cpu` → 实时折线图卡片 | reducer 不认识 | `host.agui.on("monitoring.cpu", ...)` + 注册 `ContentBlock` 渲染 |
| 给企业部署做品牌主题 | 改 `tokens.css` | `host.theme.register({ id: "brand", tokens: {...} })` |

---

## 四、插件形态（吸收 pi-mono 的轻量方案）

一个插件就是**一个 TypeScript 文件**：

```
~/.lyra/plugins/my-plugin/
└── index.ts        # 默认 export 一个 definePlugin(setup) 调用
```

> 设计取舍：v1 草案要求 `plugin.json` + `index.js` 两份文件。pi-mono 的实践证明 manifest 不是必需的 —— 元数据通过 `register*` 的调用顺序声明，调试更简单，类型推导更顺畅。**我们因此砍掉 manifest**。如果以后想做 Pi Package 式的打包分发（见 §13），用 `package.json` 的 `lyra` 字段做声明即可。

### 4.1 最小插件

```ts
// ~/.lyra/plugins/kubectl/index.ts
import { definePlugin } from "@lyra/plugin-sdk";

export default definePlugin({
  name:    "kubectl-preview",
  version: "0.1.0",
  setup({ host }) {
    host.tool.registerPreview("kubectl", KubectlPreview);
    host.inspector.registerTab({
      id:    "kubectl-resources",
      icon:  "list",
      label: "Resources",
      component: KubectlTab,
    });
  },
});
```

`name` / `version` 写在 `definePlugin` 的参数里 —— 加载时就能拿到，不需要额外读文件。

### 4.2 入口约定

- 必须默认 export `definePlugin` 调用的结果。
- `setup` 函数同步或 async 都行；`async` 会阻塞插件激活直到 resolve。
- `setup` 只能调用 `host.*` 暴露的方法；**直接 `import`** 第三方库由 host 通过 virtual modules 提供（见 §6）。

---

## 五、扩展点（v1 暴露面）

扩展点**故意做窄**，每个都精确对应 UI 里一处现有的接缝。

### 5.1 工具预览 — `host.tool.registerPreview(fn, Component)`

替代 / 扩展 `components/tools/ToolPreview.tsx`。现在这个 router 把 `bash → BashPreview` 等映射写死，让插件能加自己的 `fn`。

```ts
host.tool.registerPreview("kubectl", ({ tool, onOpenInspector }) => (
  <ResourceTable args={tool.args} />
));
```

如果多个插件抢同一个 `fn`，**后注册的覆盖前者**，console 给一条 warning。冲突直接报错的策略后续再加。

### 5.2 Inspector 标签 — `host.inspector.registerTab(spec)`

插件给 inspector rail 推一个新 tab。rail 按钮、badge、激活样式由宿主渲染；内容由插件渲染。

```ts
host.inspector.registerTab({
  id: "profiler",
  icon: "lightning",            // 任意宿主暴露的 IconName
  label: "Profiler",
  badge: () => useFlameGraphCount(),   // 可选，组件 hook
  component: ProfilerTab,
});
```

### 5.3 自定义消息内容块 — `host.message.registerContentBlock(kind, Renderer)`

> **新增 v2** — 受 pi-mono 的 `registerMessageRenderer` 启发。

我们的 `ContentBlock` 联合（`viewState.ts`）现在是固定的 7 种：`text / reasoning / plan / tool / code / search / approval / checkpoint`。插件可以扩展这个联合，加自己的块类型。

```ts
// 1. 类型声明（declaration merging）
declare module "@lyra/plugin-sdk" {
  interface CustomContentBlocks {
    cpuChart: { kind: "cpuChart"; series: { t: number; v: number }[] };
  }
}

// 2. 注册渲染器
host.message.registerContentBlock("cpuChart", ({ block }) => (
  <LineChart data={block.series} />
));

// 3. 让 reducer 知道怎么从 CUSTOM 事件转成 ContentBlock（见 §5.6）
host.agui.on("monitoring.cpu", (value, { dispatch }) => {
  dispatch.appendBlock("cpuChart", { series: value.series });
});
```

### 5.4 Slash 命令 — `host.composer.registerCommand(cmd, spec)`

```ts
host.composer.registerCommand("/lint", {
  description: "Run eslint --fix on the staged diff",
  run: async ({ args, host }) => {
    // 命令体里调后端
    const result = await host.rpc.post("/plugins/eslint/lint", { args });
    host.notify(`Fixed ${result.fixed} issues`);
  },
});
```

### 5.5 主题 — `host.theme.register(spec)`

```ts
host.theme.register({
  id: "midnight",
  label: "Midnight",
  tokens: {
    "--color-bg":       "#0a0c14",
    "--color-surface":  "#101424",
    "--color-accent":   "#7c8ef7",
    /* … DESIGN.md 里任意 token 子集 … */
  },
});
```

主题选择器从 `host.theme.list()` 读列表。**内置主题（dark / light）也走同一个 API 注册** —— 一等公民 / 三等公民没有区别（这点直接抄 pi-mono）。

### 5.6 AG-UI CUSTOM 事件处理器 — `host.agui.on(name, handler)`

现在 `lyra.plan` / `lyra.code-proposal` / `lyra.approval` / `lyra.telemetry` 这些事件在 reducer 里直接处理。插件可以监听新的 CUSTOM name，不用 fork reducer。

```ts
host.agui.on("kubectl.resources", (value, { dispatch }) => {
  // dispatch 是 host 暴露的一组安全 mutation 方法
  dispatch.updateInspector({ kubectlResources: value });
});
```

handler 可以是 sync 或 async；返回的 `Promise` 被宿主 await，方便插件在事件链里做事。

### 5.7 Settings 面板 — `host.settings.registerPane(spec)`

```ts
host.settings.registerPane({
  id: "kubectl",
  label: "kubectl",
  icon: "tool",
  component: KubectlSettings,    // 自带表单 + persistence
});
```

由 host 管 schema 校验 + 持久化（用 Zustand store），插件只渲染表单 UI。

---

## 六、Host SDK（`@lyra/plugin-sdk`）

```ts
type Host = {
  // ---- 注册点 ----
  tool:     { registerPreview(fn: string, c: ToolPreviewComponent): Disposable };
  inspector:{ registerTab(spec: TabSpec): Disposable };
  message:  { registerContentBlock<K extends string>(kind: K, r: BlockRenderer<K>): Disposable };
  composer: { registerCommand(cmd: string, spec: CommandSpec): Disposable };
  theme:    { register(spec: ThemeSpec): Disposable; list(): ThemeSpec[] };
  agui:     { on<T>(customName: string, handler: CustomEventHandler<T>): Disposable };
  settings: { registerPane(spec: SettingsPaneSpec): Disposable };

  // ---- 后端桥（§7 详述）----
  rpc: {
    get<T>(path: string, params?: object): Promise<T>;
    post<T>(path: string, body?: object): Promise<T>;
  };

  // ---- 横向工具 ----
  notify(message: string, level?: "info" | "warn" | "error"): void;
  storage: KeyValueStore;       // 命名空间隔离的 localStorage
  ui:      UIPrimitives;        // Icon / PillButton / Panel / Chip / ...
  motion:  MotionPresets;       // 共享的 ease / spring / 预设动画
};
```

每个 register-* 返回 `Disposable`。宿主在停用时统一调 `.dispose()`，**插件作者不用自己跟踪句柄** —— 反向操作总是隐式可逆（这点也是抄 pi-mono）。

### 6.1 Virtual modules — 关键工程细节

> 这是 v1 草案的重大遗漏，pi-mono 用 jiti `virtualModules` 解决得很漂亮（`loader.ts:44-61`）。

插件**不应该自己安装 React / motion / react-query / @lyra/plugin-sdk**。它们是宿主已经加载过的实例，插件用 `import` 拿到的应该是同一份引用。否则：

- 多份 React → hook 状态不共享，组件 mount 就报错
- 多份 motion → 动画 context 错乱
- 多份 zustand store → 状态完全隔离，看不到主应用的 UI 状态

实现：宿主用 Vite 的 [`build.lib.external`](https://vitejs.dev/guide/build.html#library-mode) + 一个 ImportMap shim，把这些依赖代理成全局变量。插件代码里写 `import { motion } from "motion/react"`，运行时拿到的就是宿主那一份。

```ts
// 插件代码长这样
import { motion } from "motion/react";        // ✔ 复用宿主
import { Panel } from "@lyra/plugin-sdk/ui";  // ✔ 复用宿主
import { useQuery } from "@tanstack/react-query"; // ✔ 复用宿主
import lodash from "lodash";                  // ✔ 自己 vendor
```

**provided 列表（不可自带版本）**：

- `react`, `react-dom`
- `motion/react`
- `@tanstack/react-query`, `@tanstack/react-router`
- `zustand`
- `@ag-ui/core`, `@ag-ui/client`
- `@lyra/plugin-sdk`（含所有 host 类型 + UI primitives + motion presets）

其它依赖插件自己 bundle。

---

## 七、与 Go 后端通信

插件想跟后端说话有**三条路**，按推荐顺序排列：

### 7.1 走标准 react-query + 既有 REST endpoint（推荐）

如果你的插件只是渲染已经有的数据，直接用 `host.rpc.get` 拿就行：

```ts
function KubectlTab() {
  const { data } = useQuery({
    queryKey: ["kubectl-resources"],
    queryFn:  () => host.rpc.get("/kubectl/resources"),
  });
  return <ResourceTable data={data} />;
}
```

`host.rpc` 是 ky 实例的 thin wrapper，自动带上 baseUrl + 错误处理。

### 7.2 让 Go 后端发 CUSTOM 事件 → 插件 handler

如果是流式 / 推送场景，让 Go 端在 AG-UI 流里发 `CUSTOM` 事件，前端插件订阅。这避免拉/推混用，所有事件都走同一条 SSE。

**Go 端**（插件作者要么自己写 Go 包，要么改 `lyra/internal/agui/mock.go`）：
```go
sub.next(Custom("monitoring.cpu", map[string]any{ "series": [...] }))
```

**前端插件**：
```ts
host.agui.on("monitoring.cpu", (value, { dispatch }) => {
  dispatch.appendBlock("cpuChart", { series: value.series });
});
```

### 7.3 注册新 REST endpoint（最重）

如果插件需要新的后端能力（比如 `/lint` 跑 eslint），那**这其实是 Go 端的扩展**，不在 Lyra 插件的范畴。两个选择：

1. **MCP server**：把这个能力做成 MCP server，agent 可以调用，前端通过现有 `/run` SSE 看到工具调用。这是最干净的方式。
2. **改 Go 主仓**：在 `lyra/internal/agui/` 加一个新 endpoint，跟现有 `/diff` `/terminal` 同级。需要给主仓提 PR。

**前端插件不应该假装能凭空增加后端 endpoint**。`host.rpc.post("/lint", ...)` 在后端没有对应实现时直接 404，插件应该 graceful degrade。

### 7.4 数据流方向小结

```
                    ┌─────────────┐
                    │  Go 后端     │
                    └──┬───┬──┬───┘
              SSE 流  │   │   │  HTTP/JSON
       (AG-UI events) │   │   │  (REST)
                      ▼   │   ▼
        ┌──────────────┐  │  ┌──────────────┐
        │ reducer →    │  │  │ react-query  │
        │ viewState    │  │  │ + ky         │
        └──────┬───────┘  │  └──────┬───────┘
               │          │         │
               ▼          ▼         ▼
        ┌─────────────────────────────────┐
        │  插件 (`host.agui.on`,           │
        │       `host.rpc.get`,           │
        │       component rendering)      │
        └─────────────────────────────────┘
```

插件**永远不直接发 fetch**（被 host.fetch 拦截），它的网络出口都是 `host.rpc.*`，由宿主统一管 baseUrl、错误、超时、未来的认证。

---

## 八、类型扩展（declaration merging）

借 pi-mono 的 `CustomAgentMessages` 思路，让插件能把自己的类型加进宿主的联合，**保持类型安全**：

```ts
// 插件代码
declare module "@lyra/plugin-sdk" {
  interface CustomContentBlocks {
    cpuChart: { kind: "cpuChart"; series: ChartPoint[] };
  }
  interface CustomCommands {
    "/lint": { args: string };
  }
}
```

宿主的 `ContentBlock` 联合 = 内置的 7 种 + 所有插件 declare 的合并。reducer 的 `dispatch.appendBlock` 现在能正确推断 `kind: "cpuChart"` 对应的 payload 形状。

---

## 九、生命周期

```
发现  →  加载 (jiti 或 dynamic import)  →  setup()  →  ⟨运行中⟩
                                              ↓
                                          dispose 所有句柄  →  ⟨卸载⟩
```

| 阶段 | 行为 |
|---|---|
| **发现** | 宿主扫 `~/.lyra/plugins/*/index.ts` + 项目级 `.lyra/plugins/*/index.ts` + 内置集合 |
| **加载** | dynamic `import()`，配 virtualModules / importMap |
| **setup** | 调用 `definePlugin` 传入的 setup 函数；所有 `register*` 都在这一步发生 |
| **运行** | 插件被动响应：组件渲染、事件回调、命令执行 |
| **卸载** | 宿主依次 dispose 该插件注册的所有 Disposable，**插件不写自己的 cleanup** |

**热重载**：dev 模式下 chokidar 监听插件目录变化，触发卸载 + 重新加载。仿 pi-mono 的 `/reload` 命令。

---

## 十、错误处理与隔离

> 抄 pi-mono `runner.ts:680-712`：**每个 hook 调用独立 try/catch**，错误上报给 listener，不影响其他 hook。

```ts
// 宿主内部
for (const ext of extensions) {
  const handlers = ext.handlers.get(eventType) ?? [];
  for (const handler of handlers) {
    try {
      await handler(event, ctx);
    } catch (err) {
      host.notify(`Plugin ${ext.name} failed on ${eventType}: ${err.message}`, "error");
      host.log.plugin(ext.name, err);
      // 继续下一个 handler，不抛
    }
  }
}
```

UI 贡献的组件外层包一个 React Error Boundary：

```tsx
<PluginBoundary plugin={ext.name}>
  <PluginContributedComponent {...props} />
</PluginBoundary>
```

效果：

- 一个插件的渲染崩溃 = 那一处显示一个红色 fallback，主应用照常
- 一个插件的 hook 抛错 = console 报错 + 状态栏提示，下一个事件继续
- 一个插件加载失败 = 宿主继续启动，settings 里看到这个插件标"disabled (load error)"

---

## 十一、安全模型

Lyra 跑在 Wails webview 里，跟用户的对话挤在一个进程。v1 立场是 **trust-on-install + 极窄 SDK 暴露面**，明确**不做沙箱**：

- 没有权限声明、没有 ACL DSL。pi-mono 也是这个立场（`coding-agent.md:605`：「无显式权限模型 — 用扩展的 `tool_call` 阻断实现策略」）。
- `host.rpc` 是插件**唯一**的网络出口；加载时通过 Proxy 把 `globalThis.fetch` / `XMLHttpRequest` 拦截掉。
- `host.storage` 加 `lyra.plugin.<id>.*` 前缀，插件之间数据不可见。
- 不开放 `document` / `window` / React 树。UI 是通过返回组件来贡献。
- 渲染层逃逸（恶意 `dangerouslySetInnerHTML`）v1 接受这个风险。

**为什么不沙箱**：

- Worker + structured clone 让 React 渲染基本不可行
- iframe 隔离会破坏统一的设计语言（DESIGN.md tokens 不再共享）
- 对于桌面应用，trust-on-install 跟"装个 npm 包"是一个量级的信任决策

如果将来真有企业部署诉求，签名 + 注册中心 + Worker 沙箱可以一起加 —— 那是 v3 的事。

---

## 十二、分发

| 渠道 | 发现方式 | 备注 |
|---|---|---|
| **内置** | 编译时从 `frontend/src/plugins/builtin/*` 打进 bundle | 内置主题、内置工具预览（kubectl 等）最终都搬到这里。**宿主里永远不应该出现"这是内置 vs 这是第三方"的特判**。 |
| **本地 sideload** | `~/.lyra/plugins/<name>/index.ts` 或 `.lyra/plugins/<name>/index.ts` | v1 主要分发方式；dev 模式热重载 |
| **Pi Package 式 npm/git** | `package.json` 加 `"lyra": { "plugins": ["./index.ts"] }` | 仿 pi-mono 的 Pi Package；用户 `lyra-pkg install npm:@foo/lyra-kubectl`。**v1 不做**，留接口 |

---

## 十三、兼容与版本

- 宿主公布 `lyra.apiVersion`（如 `1.0.0`）。
- 插件可在 `definePlugin` 调用里声明 `apiVersion: "^1.0.0"`；不在范围内拒绝加载。
- SDK 破坏性变更 = 主版本号升级；新增 = 次版本号；bugfix = 修订号。
- Deprecation 在 SDK 里保留两个次版本，调用时 console warn。

---

## 十四、把现有功能改造成内置插件（自证可行）

为了证明 SDK 暴露面够用，v1 里程碑会把所有现有的工具预览改造成内置插件。

**改造前** — `components/tools/ToolPreview.tsx` 写死的 switch case：

```tsx
if (fn === "bash") return <BashPreview onOpenInspector={onOpenInspector} />;
```

**改造后** — `plugins/builtin/bash/index.ts`：

```ts
export default definePlugin({
  name: "builtin.bash",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerPreview("bash", BashPreview);
  },
});
```

`ToolPreview.tsx` 的 router 收缩成一次查表：

```tsx
const Preview = host.tool.lookupPreview(tool.fn);
return Preview ? <Preview tool={tool} onOpenInspector={onOpenInspector} /> : null;
```

如果改造过程中发现 SDK 没覆盖到的接缝，那 SDK 就加一个方法、doc 加一节。**这一关一过，就说明插件 API 够用了**。

---

## 十五、分阶段

第五节的所有扩展点不会一次性全做。

**阶段一 — 静态贡献 + 自证（~1 sprint）**

- `host.tool.registerPreview`
- `host.inspector.registerTab`
- `host.theme.register`
- 把现有 `BashPreview` / `DiffPreview` / `dark theme` / `light theme` 改造成内置插件（§14）
- Sideload 从 `~/.lyra/plugins/`，virtualModules 工作起来
- **目标：验证 SDK 形状选对了**

**阶段二 — 动态贡献（~1 sprint）**

- `host.message.registerContentBlock` + declaration merging
- `host.agui.on` + `dispatch` 安全 mutation API
- `host.composer.registerCommand`
- `host.rpc.*` 网络代理

**阶段三 — 配置与设置（~0.5 sprint）**

- `host.settings.registerPane`
- 持久化层（Zustand persist middleware）
- 错误 boundary + 状态栏错误聚合

**阶段四 — 加固（按需）**

- 热重载（chokidar）
- Pi Package 式 npm/git dispatch
- semver `apiVersion` gate
- 签名 / 沙箱（v3 再说）

---

## 十六、待解决问题

1. **后端能力到底归谁。** Lyra 插件管前端，后端走 MCP —— 但 MCP 写起来不轻量，社区不熟。是否要在 Go 端也搞一套"后端插件"？倾向**不做**，因为标准 MCP 才是该投资的方向；偏离 MCP 会分散精力。

2. **状态共享。** 插件组件需不需要读 Zustand UI store？倾向 **暴露白名单 selector**：`host.useUI.theme()`、`host.useUI.activeSession()` 等只读 hook，写操作只能通过 host.action 触发。

3. **多面板插件。** 插件想加一整个新顶级面板（跟 sidebar / chat / inspector 平级）—— 是大改动，留到有人真的提需求再考虑。

4. **dev 热重载。** Vite 已经 HMR 宿主，但插件 JS 在 `~/.lyra/plugins/` 下，超出 Vite watcher。v1 接受"sideload 插件改动后需要重启 app"；阶段四补 `fs.watch`。

5. **插件 → 插件通信。** 大概率永远不允许直接 import。如果 A 要 B 的数据，走宿主居中的 bus（CUSTOM 事件感觉就是合适的形状）。

6. **virtualModules 在 Wails 打包后还能用吗。** 生产 bundle 是静态的，没法在运行时 `import()` 外部 `.ts` 文件 —— 需要要么先用 esbuild 编译 sideload 的 `.ts` 成 `.js`，要么强制插件以已编译形式分发。倾向先用 esbuild 起一个 Go-side 转译器，跟着 SSE 服务一起跑。

---

## 十七、决策

**先 spec 再实现**。等手上真有一个想做成插件而不是内置硬编码的具体功能（kubectl preview 是第一个候选），再启动阶段一。在这之前，本 doc 就是 spec。

启动阶段一的判断标准：

- [ ] 至少有 1 个明确的、不适合直接内置的具体 UI 扩展需求
- [ ] Go 端 SSE + REST 已经稳定，不会频繁加 endpoint
- [ ] DESIGN.md token 系统稳定，主题扩展的 surface 不会乱变

---

## 参考

- pi-mono Extension 系统：`/Users/tangerg/Desktop/pi-mono/docs/07-extensibility.md`、`packages/coding-agent/src/core/extensions/`
- AG-UI 协议：[docs.ag-ui.com](https://docs.ag-ui.com)
- DESIGN.md：`frontend/DESIGN.md`
