# 插件系统实现指南

> 状态：**已实现 v1**，跟代码对齐。
> 最后修订：2026-05-19
> 配套：[`EXTENSION_POINTS.md`](./EXTENSION_POINTS.md) 是扩展点底座设计 + 落地状态；本文档讲实际实现细节。

> **写法已统一**：贡献走 `host.extensions.contribute(POINT, spec)`（POINT 见 `frontend/src/plugins/sdk/kernelPoints.ts`）；少数薄 facade（agui / layout / contentBlock / lifecycle / rpc hooks / log.subscribe）保留。详见 `docs/EXTENSION_POINTS.md`。

阅读顺序建议：

1. 先看 [§1 概览](#1-概览) 了解整体形状
2. [§2 写一个插件](#2-写一个插件) 抄一个 hello world
3. 按需翻 [§3 SDK 参考](#3-sdk-参考)
4. 想改 SDK 看 [§6 内部结构](#6-内部结构)
5. 调试问题看 [§8 错误隔离与调试](#8-错误隔离与调试)

---

## 1. 概览

### 1.1 当前覆盖度

| 接缝                  | 内置插件数 | API                                                             |
| --------------------- | ---------- | --------------------------------------------------------------- |
| AG-UI CUSTOM 事件处理 | 5          | `host.agui.on`                                                  |
| 消息内容块渲染        | 6          | `host.message.registerContentBlock`                             |
| 工具调用预览          | 4          | `host.extensions.contribute(TOOL_PREVIEW, c, { key: fn })`      |
| Composer slash 命令   | 2          | `host.extensions.contribute(SLASH_COMMAND, spec, { key: cmd })` |
| 设置面板 pane         | 2          | `host.extensions.contribute(SETTINGS_PANE, spec)`               |
| Workspace 视图        | 8          | `host.extensions.contribute(WORKSPACE_VIEW, spec)`              |
| HTTP / 通知 / 存储    | —          | `host.rpc` / `host.notify` / `host.storage`                     |

**所有内置插件**全部走标准 SDK 路径，跟未来第三方插件无差别。宿主代码（`reducer.ts` / `PartRenderer.tsx` / `WorkspaceViewBody.tsx`）里没有任何 "这是内置" 的特判。

### 1.2 数据流

```
┌─ Go 后端 ──────────────────────────────────────┐
│  /run         SSE — AG-UI BaseEvent 流          │
│  /sessions    /diff /terminal /...  (REST)      │
│  /plugins     列 ~/.lyra/plugins/               │
│  /plugins/<id>/index.js  静态 ESM 模块          │
└─────────────────────────────────────────────────┘
                   ▲
        ┌──────────┼─────────┐
        │          │         │
  AG-UI SSE   REST/JSON   import(url)
        │          │         │
        ▼          ▼         ▼
   ┌──────────────────────────────────┐
   │  Plugin SDK Host                  │
   │  绑定 plugin name，返回 Disposable │
   └──────────────────────────────────┘
        ▲                  ▲
        │                  │
   PluginProvider     builtin/* 静态 import
        ▼                  ▼
   ┌──────────────────────────────────┐
   │  Plugin Registry (Zustand store)  │
   │  toolPreviews / contentBlocks /   │
   │  customEventHandlers / ...        │
   └──────────────────────────────────┘
        ▲
        │ useToolPreview / useWorkspaceViews / ...
        ▼
   ┌────────────────────────────────────┐
   │  React 组件树                       │
   │  PartRenderer / WorkspaceViewBody /  │
   │  SlashSuggestions / SettingsModal    │
   └────────────────────────────────────┘
```

### 1.3 关键设计取舍（实现层面）

| 选择                                               | 理由                                                                                        |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Registry 用 **Zustand** 不用 Context               | reducer、Composer.submit 等非 React 代码也要查表，`getState()` 同步访问                     |
| Agent state 也 **Zustand** 化                      | 插件组件（如 PlanTab）直接 `useAgentStore((s) => s.plan)`，不用从 props 透传 5 层           |
| 选择器派生数组用 **`useMemo`** 不用 `useShallow`   | `useShallow` 用 `Object.is` 比较元素；`.map(() => ({...}))` 产生新对象，永远不等。坑见 §8.3 |
| 类型扩展用 **TS declaration merging**              | 跨插件类型安全，零运行时开销                                                                |
| 错误聚合到 **`usePluginErrorStore`**               | "插件挂了"得有 UI 入口，console.error 找不到                                                |
| Sideload 用 **`window.__LYRA__` 桥**而非 importmap | dev 模式下 importmap + Vite 时序脆弱，window 桥简单可靠                                     |

---

## 2. 写一个插件

### 2.1 最小 hello

`~/.lyra/plugins/hello/index.js`（注意是 `.js`，pre-bundled ESM）：

```js
const { React, SDK } = window.__LYRA__;
const { definePlugin, SLASH_COMMAND } = SDK; // POINT consts re-exported from SDK
const h = React.createElement;

export default definePlugin({
  name: "user.hello",
  version: "0.1.0",
  apiVersion: "^1.0.0",
  setup({ host }) {
    host.extensions.contribute(
      SLASH_COMMAND,
      {
        description: "Say hi via toast",
        run: ({ args }) => host.notify(`Hello${args ? `, ${args}` : ""}!`),
      },
      { key: "/hello" },
    );
  },
});
```

装：`cp -r hello ~/.lyra/plugins/`，重启 wails 窗口。Plugins pane 应该出现 `user.hello v0.1.0`（蓝色 Sideload 徽章）。

### 2.2 带 JSX/TS 的插件

宿主里有 esbuild 的话：

```bash
esbuild src/index.tsx --bundle --format=esm --outfile=index.js \
  --external:window
```

或者继续用 `React.createElement` 手写，不需要构建。

### 2.3 内置插件的写法

跟 sideload 完全一样，只是写在 `frontend/src/plugins/builtin/<name>/index.tsx` 里、可以 `import` 任意宿主模块。例：

```tsx
// frontend/src/plugins/builtin/workspace-views/files.tsx
import { useFilesChanged } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

function FilesView() {
  /* ... */
}

export const filesView = definePlugin({
  name: "lyra.builtin.view-files",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "files",
      title: "Files",
      icon: "filetext",
      openByDefault: false,
      order: 20,
      component: FilesView,
    });
  },
});
```

然后加进 `frontend/src/plugins/builtin/index.ts` 的 `builtinPlugins` 数组即可。

---

## 3. SDK 参考

完整类型见 `frontend/src/plugins/sdk/types.ts`；本节给最简调用示例。

### 3.1 `host.extensions.contribute(TOOL_PREVIEW, Component, { key: fn })`

工具调用展开时渲染什么。`key` 是 AG-UI `ToolCall.fn`（如 `"bash"`、`"kubectl"`）。组件签名 `({ tool, onOpenInspector }) => JSX`。

```ts
import { TOOL_PREVIEW } from "@/plugins/sdk/kernelPoints";

host.extensions.contribute(TOOL_PREVIEW, ({ tool }) => (
  <div>kubectl args: {tool.args}</div>
), { key: "kubectl" });
```

冲突策略：**后注册覆盖**，console.warn 一条。

### 3.2 `host.message.registerContentBlock(kind, Component)`

给消息流加新的内容块类型。**配合 declaration merging 使用**：

```ts
// 1. 扩展类型联合
declare module "@/protocol/agui/viewState" {
  interface CustomContentBlockMap {
    cpuChart: { kind: "cpuChart"; series: { t: number; v: number }[] };
  }
}

// 2. 注册渲染器（block 已经被推断成正确的子类型）
host.message.registerContentBlock("cpuChart", ({ block }) => (
  <LineChart data={block.series} />
));

// 3. 让块出现在某个消息里 —— 通常通过 agui.on
host.agui.on<{ series: any[] }>("monitoring.cpu", (value) =>
  appendBlockToLatestAssistant({ kind: "cpuChart", series: value.series }),
);
```

内置块（`text` / `tool` / `plan` / `code` / `search` / `approval` / `checkpoint` / `reasoning`）已经在 `BuiltinContentBlockMap` 里，第三方扩展只往 `CustomContentBlockMap` 加。

### 3.3 `host.agui.on(name, handler)`

监听 AG-UI `CUSTOM` 事件。handler 返回一个 `StateUpdate` 函数 `(state) => state`，由 reducer 应用。

```ts
host.agui.on<MyPayload>("my.event", (value) => (state) => ({
  ...state,
  custom: { ...state.custom, my: value },
}));
```

通常用 SDK 提供的辅助函数组合：

```ts
import {
  appendBlockToMessage,
  appendBlockToLatestAssistant,
  setPlan,
  patchRun,
  compose,
} from "@/plugins/sdk";

host.agui.on("lyra.plan", (v) => setPlan(v.items));
host.agui.on("lyra.telemetry", (v) => patchRun({ activity: v.activity, ... }));
host.agui.on("multi", (v) => compose(setPlan(v.items), patchRun({ step: v.step })));
```

返回 `void` 表示纯副作用（如埋点），不改 state。

### 3.4 `host.extensions.contribute(SLASH_COMMAND, spec, { key: cmd })`

```ts
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";

host.extensions.contribute(
  SLASH_COMMAND,
  {
    description: "Run eslint --fix",
    run: async ({ args, send }) => {
      const result = await host.rpc.post("/lint", { args });
      host.notify(`Fixed ${result.fixed} issues`);
      // 可选：把消息也发给 agent
      // send("Please review the eslint output");
    },
  },
  { key: "/lint" },
);
```

`run` 缺省 → "hint only" 模式，dropdown 里显示描述，回车把整行原样作为用户消息发给 agent。

### 3.5 `host.extensions.contribute(SETTINGS_PANE, spec)`

```ts
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

host.extensions.contribute(SETTINGS_PANE, {
  id: "k8s",
  label: "Kubernetes",
  icon: "tool",
  order: 100, // builtin 用 0-99，third-party ≥100
  component: K8sSettings,
});
```

pane 组件可以用 `host.storage` 持久化自己的配置：

```tsx
function K8sSettings() {
  const [url, setUrl] = useState(
    host.storage.get<string>("apiServerUrl") ?? "",
  );
  return (
    <input
      value={url}
      onChange={(e) => {
        setUrl(e.target.value);
        host.storage.set("apiServerUrl", e.target.value);
      }}
    />
  );
}
```

### 3.6 `host.extensions.contribute(WORKSPACE_VIEW, spec)`

```ts
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

host.extensions.contribute(WORKSPACE_VIEW, {
  id: "k8s-resources",
  title: "K8s",
  icon: "list",
  defaultLocation: "right", // "left" | "right" | "main" | "bottom"，可选
  openByDefault: false, // 是否启动即打开，可选
  order: 100,
  component: K8sView,
});
```

view 是用户可开关的 workspace tab —— kernel 只要 `id` + `component`，其余交给用户从 UI 里开关。打开/聚焦用 `host.workspace.openView(id)`，关闭用 `host.workspace.closeView(id)`。

view 组件**无 props**，自己用 `useAgentStore` / `useSessionStore` / react-query 取数据。共享状态（如"当前激活文件"）直接读写对应 store，不走 context：

```tsx
import { useSessionStore } from "@/state/sessionStore";

function K8sView() {
  const activeFile = useSessionStore((s) => s.activeFile);
  // ...
}
```

### 3.7 `host.rpc.{get, post}`

`ky` 实例的薄封装，自动带宿主 baseUrl（`http://127.0.0.1:17171`）：

```ts
const sessions = await host.rpc.get<Session[]>("/sessions");
const result = await host.rpc.post<{ ok: boolean }>("/lint", { files });
```

如果后端没有对应端点，会 404 —— 插件应该 graceful degrade。

### 3.8 `host.storage`

`lyra.plugin.<plugin-name>.<key>` 前缀的 localStorage，跨插件隔离：

```ts
host.storage.set("config", { x: 1 });
host.storage.get<{ x: number }>("config"); // { x: 1 }
host.storage.remove("config");
host.storage.keys(); // 列当前插件的所有 key
```

JSON 自动序列化；非 JSON 字符串透传。

### 3.9 `host.notify(message, level?)`

```ts
host.notify("Backend responded: ok");
host.notify("Quota almost full", "warn");
host.notify("Connection lost", "error");
```

实际是 `window.dispatchEvent(new CustomEvent("lyra:plugin-toast", { detail }))`，由 `PluginToaster` 组件渲染到右下角。

---

## 4. 类型扩展（declaration merging）

唯一不引入运行时分发的跨插件类型安全方案。

### 4.1 扩展 ContentBlock 联合

```ts
declare module "@/protocol/agui/viewState" {
  interface CustomContentBlockMap {
    cpuChart: { kind: "cpuChart"; series: Point[] };
  }
}
```

之后：

- `ContentBlock` union 自动包含 `{ kind: "cpuChart"; series: Point[] }`
- `host.message.registerContentBlock("cpuChart", ...)` 推断出正确的 `block` 类型
- `appendBlockToMessage(msgId, { kind: "cpuChart", series: [...] })` 类型校验通过
- `PartRenderer` 的 default case 会把 `cpuChart` 块路由到注册的渲染器

### 4.2 实现细节（关键文件位置）

`frontend/src/protocol/agui/viewState.ts`：

```ts
export interface BuiltinContentBlockMap {
  /* 内置块 */
}
export interface CustomContentBlockMap {} // ← 插件 augment 这个
export type ContentBlockMap = BuiltinContentBlockMap & CustomContentBlockMap;
export type ContentBlock = ContentBlockMap[keyof ContentBlockMap];
```

`Custom*Map` 故意做成空接口；插件用 `declare module` 加字段。

---

## 5. 加载生命周期

源码：`frontend/src/plugins/PluginProvider.tsx` + `plugins/sdk/definePlugin.ts` + `plugins/sideload.ts`。

```
App mount
  ↓
QueryClientProvider mount
  ↓
PluginProvider useEffect (一次性，empty deps):
  ↓
  installHostBridge()      ← 把 React/Motion/SDK 挂到 window.__LYRA__
  ↓
  await loadPlugins(builtinPlugins):
    顺序对每个 builtinPlugin 调 loadPlugin(spec):
      1. apiVersion semver 检查
      2. 创建 host (绑定 spec.name)
      3. try await spec.setup({ host })
      4. 成功 → registerLoaded
         失败 → 已注册 disposables.forEach(dispose) + reportPluginError
  ↓
  tagAllAsBuiltin()        ← 记录来源给 Plugins pane
  ↓
  await loadSideloadedPlugins():
    fetch /plugins → [{ id, url }, ...]
    对每个 info:
      import(url) → 校验 default export
      调 loadPlugin(spec) 走同一路径
  ↓
  setReady(true)           ← 触发 PluginProvider 重渲染（实际无影响）
```

子组件（AgentClientPage 等）**不等** loadPlugins 完成就开始渲染 —— 插件是增量的：没加载完时 ToolCard 不展开（因为 `useToolPreview(fn)` 返回 undefined），加载完了 Zustand 触发重渲染补上。

### 5.1 阶段四 sideload 关键文件

| 文件                                   | 作用                                                      |
| -------------------------------------- | --------------------------------------------------------- |
| `internal/agui/plugins.go`             | Go: 扫 `~/.lyra/plugins/`，serve `/plugins/<id>/index.js` |
| `internal/agui/server.go:registerREST` | 把 `/plugins` 路由挂到 mux                                |
| `frontend/src/plugins/sideload.ts`     | fetch + dynamic import + 走 loadPlugin                    |
| `frontend/src/plugins/hostBridge.ts`   | 静态 import 宿主依赖，挂到 `window.__LYRA__`              |

### 5.2 apiVersion 门禁

`HOST_API_VERSION` 当前是 `"1.0.0"`（`frontend/src/plugins/sdk/apiVersion.ts`，独立 leaf 模块避免循环依赖）。

```ts
if (spec.apiVersion && !satisfies(HOST_API_VERSION, spec.apiVersion)) {
  return { kind: "skipped", reason: "..." };
}
```

用 `compare-versions` 库的 `satisfies(version, range)`。

---

## 6. 内部结构

### 6.1 Registry（`plugins/sdk/registry.ts`）

Zustand store，每个接缝一个 Map：

```ts
type PluginStoreState = {
  loaded: Map<pluginName, LoadedPlugin>;
  toolPreviews: Map<fn, Owned<Component>>;
  contentBlocks: Map<kind, Owned<Renderer>>;
  customEventHandlers: Map<eventName, Owned<Handler>>;
  slashCommands: Map<cmd, Owned<Spec>>;
  settingsPanes: Map<id, Owned<Spec>>;
  workspaceViews: Map<string, Owned<WorkspaceViewSpec>>;
};
```

`Owned<T> = { pluginName, value }` —— 每个条目记住注册者：

- 冲突时给出准确 warning（"plugin B overrides X registered by plugin A"）
- `dispose` 只清自己的（stale dispose 不影响新 owner）
- Plugins pane 显示来源

通用 add/remove helper：

```ts
function addOwned<T>(map, pluginName, key, value, label) {
  // 冲突 warning + 返回新 Map
}
function removeOwned<T>(map, pluginName, key) {
  // 检查 owner 匹配才删
}
```

### 6.2 Host（`plugins/sdk/host.ts`）

`createHost(pluginName, sink)` 返回的 `Host` 对象：

- `host.extensions.contribute(POINT, ...)`（及保留的 facade）调用 `usePluginStore.getState().addXxx(pluginName, ...)`
- 返回 `Disposable` 同时塞进 `sink`（数组），让 `loadPlugin` 能在 setup 失败时统一回滚
- `host.storage` 是 `createStorage(pluginName)`，自动加 `lyra.plugin.<name>.` 前缀
- `host.rpc` 用 `lib/http.ts` 的 ky 实例
- `host.notify` `window.dispatchEvent` 出 toast 事件

### 6.3 Subscribers（React hooks）

**单值选择器**（直接订阅，引用稳定）：

```ts
export function useToolPreview(fn) {
  return usePluginStore((s) => s.toolPreviews.get(fn)?.value);
}
```

**列表派生选择器**（关键模式，避免死循环）：

```ts
export function useWorkspaceViews() {
  const map = usePluginStore((s) => s.workspaceViews);  // 只取 Map
  return useMemo(
    () => Array.from(map.values()).map((o) => o.value).sort(...),
    [map],                                              // 派生放组件侧
  );
}
```

> ⚠️ **不要**用 `useShallow` 包派生数组里含新对象的选择器 —— `useShallow` 用 `Object.is` 比元素，`.map(() => ({...}))` 永远不等，触发 React `getSnapshot should be cached` 警告 + 死循环。

### 6.4 PluginContentBlock 适配器

`frontend/src/plugins/PluginContentBlock.tsx`：

```tsx
export function PluginContentBlock({ block }) {
  const Renderer = useContentBlockRenderer(block.kind);
  if (!Renderer) return null;
  return (
    <PluginBoundary plugin={`content-block:${block.kind}`}>
      <Renderer block={block as any} /> {/* 类型在 SDK 层已校验 */}
    </PluginBoundary>
  );
}
```

PartRenderer 的 `default:` case 用它。

### 6.5 Workspace view 无 context

workspace view 之间没有 context 共享层。view 直接 `host.extensions.contribute(WORKSPACE_VIEW, ...)` 贡献，组件无 props；需要共享状态（如"当前激活文件"）就直接读写对应 store（`useSessionStore` 的 `activeFile` / `setActiveFile` / `openMainView`）。这样避免强制每个 view 接受统一 props 接口，也不需要把 view 包进 provider。

---

## 7. window.**LYRA** 桥

`frontend/src/plugins/hostBridge.ts`：

```ts
import * as React from "react";
import * as ReactJSXRuntime from "react/jsx-runtime";
import * as Motion from "motion/react";
import * as SDK from "@/plugins/sdk";

export function installHostBridge() {
  window.__LYRA__ = {
    apiVersion: HOST_API_VERSION,
    React,
    ReactJSXRuntime,
    Motion,
    SDK,
  };
}
```

**关键点：**

- **静态 import**，不能动态 `import()` —— dev 模式时序会出问题，React 可能被切到独立 chunk，出现两份实例（"dispatcher.useRef is null" 错误）
- 在 builtin plugin 加载之前调用 —— 即使 builtin 不依赖 window 桥，sideload 模块的顶层代码 `const { React } = window.__LYRA__` 立即执行
- SDK 也挂上去（不只是 React） —— sideload 插件需要 `definePlugin`、`appendBlockToLatestAssistant` 等 SDK 导出

sideload 插件不能 `import "react"` —— 浏览器没有 npm，dynamic import 也找不到。**只能从 `window.__LYRA__` 拿**。

---

## 8. 错误隔离与调试

四层防御 + 一处聚合（`usePluginErrorStore`）。

### 8.1 错误源 × 隔离方式

| 源                          | 位置                  | 隔离                                        | 上报                                |
| --------------------------- | --------------------- | ------------------------------------------- | ----------------------------------- |
| `setup` 抛错                | `loadPlugin`          | 已注册 disposables 全 dispose；其他插件继续 | source: `"setup"`                   |
| 渲染抛错                    | `PluginBoundary`      | 显示红色 fallback；主应用继续               | source: `"render"` + componentStack |
| `host.agui.on` handler 抛错 | reducer 里 try/catch  | state 不变                                  | source: `"agui"` + event name       |
| slash `run` handler 抛错    | Composer.submit catch | 命令调用结束                                | source: `"command"` + cmd           |

### 8.2 Plugins pane（设置 → Plugins）

- 列出所有 loaded 插件 + 来源（Built-in / Sideload 徽章）
- 有错误的行红边 + "N errors — see browser console"
- "Clear" 按钮调 `usePluginErrorStore.clearFor(name)`

### 8.3 已知陷阱

**陷阱 1：useShallow + map(new object)**

```ts
// ❌ 死循环
return usePluginStore(
  useShallow((s) =>
    Array.from(s.x.entries()).map(([k, v]) => ({ key: k, val: v })),
  ),
);
```

`useShallow` 用 `Object.is` 比较数组元素；`.map(() => ({...}))` 每次返回新对象引用 → 永远不等 → React `getSnapshot` 不稳定 → 死循环。

**正确：**

```ts
const map = usePluginStore((s) => s.x);          // 选 Map（稳定）
return useMemo(() => Array.from(map.entries()).map(...), [map]);
```

**陷阱 2：循环依赖**

`hostBridge.ts` 一度静态 import `@/plugins/sdk`，而 `definePlugin.ts` 又 import hostBridge 拿 `HOST_API_VERSION`。模块初始化时一边没就绪，常量是 undefined。

**修法：把跨边界常量放到 leaf module。** `apiVersion.ts` 独立文件，不 import 任何东西。

**陷阱 3：StrictMode 双 mount**

React 18 StrictMode 在 dev 下双 mount。插件 setup 跑两次。**正常** —— `addOwned` 同 plugin name 同 key 会覆盖自己，不会触发冲突 warning。但如果插件作者在 setup 里写副作用（不要这么做），会出问题。

当前 `main.tsx` 临时关掉了 StrictMode（注释里说明可以重开）。

### 8.4 调试技巧

**1. Console 看插件加载日志**

DevTools console 有 `[agui]` 调试日志（每个事件）+ `[plugin] X setup failed: ...` 报错。

**2. curl 验证后端**

```bash
curl http://127.0.0.1:17171/health             # → ok
curl http://127.0.0.1:17171/plugins            # → [...] sideload 列表
curl http://127.0.0.1:17171/plugins/foo/index.js  # → ESM 源码
```

**3. 看注册表状态**

DevTools console：

```js
window.__LYRA__.SDK.usePluginStore.getState();
// .toolPreviews / .contentBlocks / .workspaceViews / ...
```

**4. 看 agent state 实时变化**

```js
window.__LYRA__.SDK.usePluginStore.subscribe(console.log);
```

不过 SDK 不直接 export agentStore；如果要看 agent state 实时变化，临时在 useAgentSession.ts 加 `useAgentStore.subscribe(console.log)`。

---

## 9. 内置插件清单（按 domain 分组）

按加载顺序（见 `frontend/src/plugins/builtin/index.ts`）：

| #   | 名字                     | 类型           | 干什么                                                           |
| --- | ------------------------ | -------------- | ---------------------------------------------------------------- |
| 1   | `plan-handler`           | agui.on        | `lyra.plan` → `setPlan(items)`                                   |
| 2   | `code-proposal-handler`  | agui.on        | `lyra.code-proposal` → append `code` block                       |
| 3   | `search-results-handler` | agui.on        | `lyra.search-results` → append `search` block                    |
| 4   | `approval-handler`       | agui.on        | `lyra.approval` → append `approval` block                        |
| 5   | `telemetry-handler`      | agui.on        | `lyra.telemetry` → `patchRun(...)`                               |
| 6   | `plan-block`             | content block  | 渲染 `kind: "plan"`                                              |
| 7   | `code-block`             | content block  | 渲染 `kind: "code"`                                              |
| 8   | `search-block`           | content block  | 渲染 `kind: "search"`                                            |
| 9   | `approval-block`         | content block  | 渲染 `kind: "approval"`                                          |
| 10  | `checkpoint-block`       | content block  | 渲染 `kind: "checkpoint"`                                        |
| 11  | `reasoning-block`        | content block  | 渲染 `kind: "reasoning"`（折叠思考）                             |
| 12  | `bash`                   | tool preview   | `fn: "bash"` 终端输出                                            |
| 13  | `diff`                   | tool preview   | `fn: "edit_file" / "write_file"` mini-diff                       |
| 14  | `file`                   | tool preview   | `fn: "read_file"` 文件头预览                                     |
| 15  | `grep`                   | tool preview   | `fn: "grep"` 匹配列表                                            |
| 16  | `slash-hints`            | slash          | 8 条 hint-only 命令                                              |
| 17  | `appearance`             | settings pane  | 主题 / accent 选择                                               |
| 18  | `plugins-pane`           | settings pane  | 列出所有已加载插件                                               |
| 19  | `diffView`               | workspace view | Diff 视图                                                        |
| 20  | `filesView`              | workspace view | Files 视图（带未提交数）                                         |
| 21  | `notificationsView`      | workspace view | Notifications 视图                                               |
| 22  | `planView`               | workspace view | Plan 视图（带未完成数）                                          |
| 23  | `runSummaryView`         | workspace view | Run summary 视图                                                 |
| 24  | `terminalView`           | workspace view | Terminal 视图                                                    |
| 25  | `timelineView`           | workspace view | Timeline 视图                                                    |
| 26  | `toolsView`              | workspace view | Tools 视图（带 active MCP 数）                                   |

（清单只列代表性接缝；完整插件集见 `frontend/src/plugins/builtin/index.ts`，会随主题 / 视图增减而变。）

---

## 10. 测试

`frontend/src/plugins/sdk/*.test.ts` + `protocol/agui/reducer.test.ts`，49 个用例。运行：

```bash
cd frontend
npm test           # 一次性
npm run test:watch # 监听模式
```

覆盖：

| 文件                   | 测试什么                                                                           |
| ---------------------- | ---------------------------------------------------------------------------------- |
| `storage.test.ts`      | namespace 隔离 / clear 只清自己 / 非 JSON fallback                                 |
| `errors.test.ts`       | id 单调 / clearFor / 非 Error 值序列化                                             |
| `registry.test.ts`     | 注册 / 冲突 warning / Disposable 回收 / stale dispose 不污染 / unload 异常隔离     |
| `definePlugin.test.ts` | apiVersion gate / setup 抛错回滚 / loadPlugins 失败隔离                            |
| `state.test.ts`        | 5 个 state helpers                                                                 |
| `reducer.test.ts`      | 基本事件流 + 插件 handler 路由 + 抛错隔离 + 内置 CUSTOM 通过加载对应 plugin 后生效 |

测试间隔离用全局 `beforeEach` 重置 registry + error store（`src/test/setup.ts`）。

---

## 11. 文件索引

```
docs/
├── EXTENSION_POINTS.md # 扩展点底座设计 + 落地状态
└── PLUGINS_IMPL.md     # 本文档

frontend/src/plugins/
├── PluginProvider.tsx       # 启动 orchestrator
├── PluginBoundary.tsx       # render error boundary
├── PluginContentBlock.tsx   # PartRenderer default 走这里
├── PluginToaster.tsx        # host.notify 渲染层
├── hostBridge.ts            # window.__LYRA__
├── sideload.ts              # discover + dynamic import
├── sdk/
│   ├── apiVersion.ts        # HOST_API_VERSION = "1.0.0"（leaf）
│   ├── types.ts             # Host / Spec / Disposable 等
│   ├── registry.ts          # Zustand store + selectors
│   ├── host.ts              # createHost(pluginName, sink)
│   ├── definePlugin.ts      # definePlugin / loadPlugin / loadPlugins
│   ├── state.ts             # appendBlockToMessage / setPlan / patchRun / ...
│   ├── storage.ts           # createStorage(pluginName)
│   ├── errors.ts            # usePluginErrorStore + reportPluginError
│   ├── index.ts             # 公开 SDK barrel
│   └── *.test.ts            # 单测
└── builtin/
    ├── index.ts                  # builtinPlugins 数组
    ├── plan-handler/             # 5 个 CUSTOM handler
    ├── code-proposal-handler/
    ├── search-results-handler/
    ├── approval-handler/
    ├── telemetry-handler/
    ├── plan-block/               # 6 个 content block 渲染器
    ├── code-block/
    ├── search-block/
    ├── approval-block/
    ├── checkpoint-block/
    ├── reasoning-block/
    ├── bash/                     # 4 个工具预览
    ├── diff/
    ├── file/
    ├── grep/
    ├── slash-hints/              # composer 命令
    ├── appearance/               # 2 个 settings pane
    ├── plugins-pane/
    └── workspace-views/{diff,files,notifications,plan,run-summary,terminal,timeline,tools}.tsx  # 8 个 workspace view

frontend/sample-plugins/
└── hello-sideload/
    ├── index.js     # 纯 ES module，从 window.__LYRA__ 取依赖
    └── (parent README 在 sample-plugins/ 根下)

internal/agui/
├── plugins.go       # GET /plugins + 静态 serving
├── server.go        # mux + CORS + SSE 路由
└── rest.go          # REST 端点注册（含 /plugins）
```

---

## 12. 接下来可以做什么

按优先级排：

1. **热重载** — chokidar 监听 `~/.lyra/plugins/`，文件改动后 dispose + 重新 import。dev 体验大幅提升。
2. **`host.extensions.contribute(THEME, ...)`** — 主题也走插件 API（当前 dark/light 是 CSS 切类名）。
3. **Pi Package 风格 npm/git 分发** — `lyra-pkg install npm:@foo/bar`，独立 CLI。
4. **`host.session.send` / `host.useAgentState`** — 让 sideload 插件能调 agent 接口 / 读 agent state。
5. **沙箱化** — 把 sideload JS 放到 web worker 跑，structured-clone 划边界。trust-on-install → 签名 + ACL。
6. **重启 StrictMode** — 把所有有副作用的 useEffect 改成幂等的，让 dev 双 mount 安全。

按 doc 里 §15 阶段四的"加固"目标，1-3 是该做的；4-5 是企业部署诉求出现后再做；6 是健康度问题。
