# 插件作者指南（Plugin Author Guide）

> 状态：**与代码对齐（Lyra Runtime Protocol v2）。**
> 这份文档讲**怎么写一个插件**——最小骨架、逐贡献面的配方、声明合并、常见陷阱、测试。
>
> 系统怎么组织、数据怎么流、registry 内部结构 → `frontend/ARCHITECTURE.md §5`。
> 扩展点底座的设计 + 落地状态 → `docs/EXTENSION_POINTS.md`。
> 能力分级 / 安全边界 / 能力上限 → `docs/PLUGINS_CEILING.md`。
> **API 的权威定义是 `frontend/src/plugins/sdk/types/host.ts`（typed Host 接口）**——本文示例是用法导引，签名以该接口为准。

---

## 1. 概览

一个插件就是一个 `PluginSpec`：`{ name, version, setup({ host }) }`。`setup` 在加载时同步跑一次，里面**贡献**东西给 kernel——往扩展点写 spec、往 Slot 塞组件、订阅协议事件。所有看得见的功能（连主题、命令、布局）都是插件，kernel 自己只有命名 Slot + 几个共享 store（见 `ARCHITECTURE.md §1`）。

**两种写路径**：

- **贡献一个 spec 到某个扩展点** → `host.extensions.contribute(POINT, item, opts?)`，`POINT` 来自 `sdk/kernelPoints.ts`（~35 个内置点）。这是绝大多数贡献的统一写路径。
- **少数薄 facade**（带去重 id / 泛型 / 行为逻辑，本身也只是调 contribute）：`host.events.onStream/onCustom` · `host.layout.register` · `host.message.registerContentBlock` · `host.commands.register` · `host.lifecycle.onReady/onBeforeUnload` · `host.rpc.beforeRequest/afterResponse` · `host.log.subscribe`。

**命令式动作**（不是贡献，本就该是方法）：`host.workspace.openView/closeView` · `host.config` · `host.storage` · `host.state.slice` · `host.rpc.get/post` · `host.i18n.addBundle` · `host.tasks.start` · `host.notify` · `host.window` · `host.plugins.*` · `host.log.*` · `host.commands.execute`。

每个 `contribute` / facade 都返回一个 `Disposable`，由 Host 在 `setup` 期收集——**别手动 `dispose()`**；plugin setup 抛错会自动回滚已注册部分。副作用（`subscribe` 这类）从 `setup` 返回一个 cleanup 函数即可，`unload` 时自动跑。

---

## 2. 写一个插件

### 2.1 最小骨架

```ts
import { definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";

export default definePlugin({
  name: "lyra.example.hello",
  version: "1.0.0",
  apiVersion: "^1.0.0",          // 可选；不写接受任意 host
  setup({ host }) {
    host.commands.register({
      id: "hello.world",
      label: "Hello, world!",
      group: "Examples",
      run: () => host.notify("hi", "info"),
    });
  },
});
```

### 2.2 内置插件的放置

- 文件放 `frontend/src/plugins/builtin/<domain>/<name>/index.ts(x)`（`<domain>` = agent / chat / command / defaults / i18n / settings / shell / sidebar / theme / workspace）。
- 在 `builtin/index.ts` 的合适分组里 import + 加进数组。
- 加载顺序由各 `spec.requires`（拓扑排序）决定；manifest 数组顺序只是 tie-breaker（外加 last-write-wins slot 的覆盖顺序）。

### 2.3 可选字段

```ts
definePlugin({
  name, version,
  requires: ["lyra.builtin.default-themes"],          // 依赖（拓扑排序，检测缺失/环）
  capabilities: ["commands", "events", "message"],    // 最小权限：未声明的 host namespace 变 throwing proxy
  activationEvents: ["onCommand:hello.world"],         // 懒激活：setup 不跑，只注册占位
  contributes: { commands: [{ id: "hello.world", label: "…" }] },  // 占位声明
  setup({ host }) { /* 首次访问占位时才跑 */ },
});
```

---

## 3. 贡献配方（按 API）

> 签名以 `sdk/types/host.ts` 为准；下面是常用形态。

### 3.1 协议事件 —— `host.events.onStream / onCustom`

```ts
// 订阅一个 first-class StreamEvent（run.* / item.* / state.*）。handler 链式 fold：
// (state, event) => nextState，按注册顺序串；抛错隔离到本插件并回退入态。
host.events.onStream("item.completed", (state, event) => /* 返回 nextState */ state);

// 订阅一个 custom StreamEvent（第三方约定的事件名）。handler 返回 StateUpdate：
host.events.onCustom<{ items: PlanItem[] }>("lyra.plan", (payload) =>
  (state) => ({ ...state, plan: payload.items }),
);
```

> 内置的 run.* / item.* / state.* 语义全部由 `lyra.builtin.core-reducer` 拥有（Item 是一等公民）；`custom` 事件留给第三方插件。状态组合子见 `sdk/state.ts`。

### 3.2 内容块渲染 —— `host.message.registerContentBlock`

```ts
host.message.registerContentBlock("exampleBanner", ({ block }) => <div>{block.text}</div>);
```

需要先用声明合并把 `exampleBanner` 加进 ContentBlock 联合（见 §4）。内置块（text / reasoning / plan / tool / approval / question）在 `components/chat/message/` 内部直渲，**不走**这里；这里是给扩展块（第三方 / `preview-blocks`）用的。

### 3.3 布局 Slot —— `host.layout.register`

```ts
host.layout.register("app.sidebar", { id: "my-panel", order: 10, component: MyPanel });
```

内部去重 id = `${slot}#${spec.id}`。kernel 区是 `app.sidebar` / `app.main` / `app.overlay`；侧栏内部、composer 工具栏等也是 slot（见 `kernelPoints.ts`）。

### 3.4 命令 / 工具预览 / slash / 设置面板 / 工作区 view

```ts
host.commands.register({ id, label, group, when, run });             // Cmd+K 命令
host.extensions.contribute(TOOL_PREVIEW, MyPreview, { key: "bash" }); // 工具卡展开预览，key=tool fn
host.extensions.contribute(TOOL_ICON, { key: "bash", icon: "terminal" });
host.extensions.contribute(SLASH_COMMAND, spec, { key: "/deploy" });
host.extensions.contribute(SETTINGS_PANE, { id, label, icon, component });
host.extensions.contribute(WORKSPACE_VIEW, { id, title, icon, component });
host.workspace.openView(id);   // 命令式打开/聚焦一个已注册 view
```

`when` 子句（命令可见性）是 VS Code-when 的小子集，求值器 `sdk/evalWhen.ts`，上下文来自 `state/useWhenContext.ts`。

### 3.5 后端通信 —— `host.rpc`

```ts
const data = await host.rpc.get<T>("/v2/info");
await host.rpc.post<T>("/v2/rpc/some.method", body);
host.rpc.beforeRequest((req) => { /* 注入 header / 日志 */ });
host.rpc.afterResponse((res) => { /* 归一错误信封 / token 刷新 */ });
```

> 业务协议调用（runs.* / items.* / sessions.*）走 typed client（`getContainer().client()`），不用 `host.rpc`。`host.rpc.get/post` 是给插件打 sidecar / 自定义后端端点用的逃生口。

### 3.6 状态 / 存储 / 配置 / i18n / 任务 / 通知

```ts
const slice = host.state.slice<T>("my.shared", initial);  // 跨插件共享 ephemeral state（约定名）
host.storage.set("key", value);                            // 每插件 namespaced localStorage
host.config.set("api.baseUrl", url); host.config.onChange("api.baseUrl", fn);
host.i18n.addBundle("zh", { "my.key": "你好" });           // 合并翻译字典
const task = host.tasks.start({ title: "Indexing…" });     // 后台任务 → status bar
host.notify("done", "success");                            // toast
host.window.setTitle("Lyra"); host.window.setBadge(3);
host.log.info("ready");                                    // 带 [plugin:<name>] 前缀 + 可订阅
```

---

## 4. 类型扩展（declaration merging）

让自定义内容块通过 TypeScript 检查——往 `CustomContentBlockMap` 合并：

```ts
declare module "@/protocol/run/viewState" {
  interface CustomContentBlockMap {
    exampleBanner: { kind: "exampleBanner"; text: string };
  }
}
```

之后 `registerContentBlock("exampleBanner", …)` 的 `block` 参数就被收窄成这个形状，`PartRenderer` 也能在联合里看到它。`Builtin*` / `Custom*` 两个 Map 的定义在 `protocol/run/viewState.ts`；插件只 augment `Custom*`。

---

## 5. 加载生命周期与门禁（要点）

- 启动编排（bridge → loadPlugins → tagAllAsBuiltin → markAppReady → sideload）见 `ARCHITECTURE.md §4.1`。
- `loadPlugin` 失败**不抛出**：dispose 已注册部分 + `reportPluginError`，其它插件继续。
- `apiVersion` 门禁：`compare-versions.satisfies(HOST_API_VERSION, spec.apiVersion)`，不兼容跳过。破坏 Host 接口 / spec 形状的改动要 bump `sdk/apiVersion.ts` 的 major。
- 懒激活（`activationEvents` + `contributes`）：占位先注册，首次访问时 `loadPlugin` 再用真组件替换；`useCommands` / `useWorkspaceViews` / `useSettingsPanes` 自动合并 declared + registered。
- 卸载 / 热重载：`host.plugins.{unload,reload}`，Settings → Plugins 每行有 Reload。

---

## 6. 错误隔离与调试

| 失败点                        | 隔离方式                          | 归因                          |
| ----------------------------- | --------------------------------- | ----------------------------- |
| `setup` 抛错                  | dispose 已注册部分；其它插件继续  | Plugins 面板红 badge          |
| 组件 render 抛错              | `PluginBoundary` 画 fallback      | 该 slot 空白，kernel 不挂     |
| stream / custom handler 抛错  | reducer try/catch，state 保持入态 | source `"events"` + 事件名    |
| tool action / command 抛错    | console.error + `reportPluginError` | UI 不挂                     |

**常见陷阱**：

- **per-message 组件别 top-level 订阅 store**——handler 里用 `useXxxStore.getState()`，否则流式时每条消息每秒重算 selector 上千次（见 `CLAUDE.md §5`）。
- **模块级 `subscribe(...)` 配 `import.meta.hot.dispose` 清理**，否则 HMR 叠加 listener。
- **`setup` 内不要 `await fetch`**——同步注册，拉数据用 `DATA_PROVIDER` + React Query 在 render 时跑。
- **push 进数组前先去重**（按 id），否则事件二次触发出 React duplicate-key 警告。

调试：Plugins 面板（Settings → Plugins）汇总 `reportPluginError`；运行时指标看 Diagnostics view（别在热路径 `console.debug` 完整对象，长会话累积无法 GC）。

---

## 7. sideload 与 `window.__LYRA__` 桥

外置插件从 Go 后端 manifest 动态 `import(url)` 加载（默认 deny，需用户授权，见 `sdk/pluginOrigin.ts`）。`hostBridge.ts` 把 `React` / `react/jsx-runtime` / `motion/react` / 整个 `@/plugins/sdk` / `HOST_API_VERSION` 挂到 `window.__LYRA__`——外置包应把这些标记为 external，运行时从桥上拿，避免重复 React 实例。最小外置插件：

```js
const { React, SDK } = window.__LYRA__;
export default SDK.definePlugin({
  name: "user.hello", version: "0.1.0", apiVersion: "^1.0.0",
  setup({ host }) {
    host.extensions.contribute(SDK.SLASH_COMMAND,
      { description: "Say hi", run: () => host.notify("Hello!") }, { key: "/hello" });
  },
});
```

---

## 8. 内置插件清单

不在此维护硬编码列表（会漂）。权威来源 = `frontend/src/plugins/builtin/index.ts` 的 manifest，按 domain 分组：protocol（core-reducer）/ infrastructure / messageRendering / toolRendering / composer / panes / kernel / sidebar / overlays（分组说明见 `ARCHITECTURE.md §5.1`）。

---

## 9. 测试

- 插件能注册：`sdk/*.test.ts`（registry / definePlugin / extensions / namespace…）。
- 协议 fold：`protocol/run/reducer.*.test.ts` + `builtin/agent/core-reducer/*` 的专项测试。
- 动了 SDK 公开形状 → 跑 `vitest run` 验证所有插件还能注册。
- 全套门禁：`cd frontend && npm run check`（typecheck + lint + format + test + knip + circular + layers + bundle）。

---

## 10. 文件索引

| 关注                 | 文件                                                            |
| -------------------- | --------------------------------------------------------------- |
| Host 接口（API 源） | `frontend/src/plugins/sdk/types/host.ts`                        |
| 扩展点定义          | `frontend/src/plugins/sdk/kernelPoints.ts`                      |
| `definePlugin`       | `frontend/src/plugins/sdk/definePlugin.ts`                      |
| registry            | `frontend/src/plugins/sdk/registry.ts` + `registryState.ts`     |
| 读侧 selector       | `frontend/src/plugins/sdk/selectors/`                           |
| 启动编排            | `frontend/src/plugins/host/PluginProvider.tsx`                  |
| Slot / 边界         | `frontend/src/plugins/host/Slot.tsx` + `PluginBoundary.tsx`     |
| sideload / 桥       | `frontend/src/plugins/host/sideload.ts` + `hostBridge.ts`       |
| 协议 fold           | `frontend/src/protocol/run/reducer.ts`                          |
| 内置 manifest       | `frontend/src/plugins/builtin/index.ts`                         |

---

## 11. 接下来可以做什么

前瞻清单（带触发条件）统一在 `frontend/ARCHITECTURE.md §12`，不在此重复。
