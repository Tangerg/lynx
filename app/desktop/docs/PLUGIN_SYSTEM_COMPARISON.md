# 插件系统对比：Lyra vs BetterScroll

> 2024-06-16 · 架构审计文档

---

## 总体定位差异

| 维度 | BetterScroll | Lyra |
|------|-------------|------|
| **领域** | 滚动行为库 | AI Agent 桌面客户端 |
| **插件目的** | 扩展滚动交互能力（下拉刷新、缩放、滚动条等） | 让第三方扩展整个应用的任意层面 |
| **复杂度** | 简单，单领域 | 复杂，全栈（UI + 数据 + 后端通信 + 主题 + 国际化） |
| **受众** | 前端开发者配置滚动行为 | 插件开发者构建独立功能模块 |
| **安全需求** | 无（插件与核心代码同级运行） | 有（sideload 插件需能力声明，默认拒绝） |

两者不在一个量级上——BetterScroll 的插件系统是"库的配置扩展"，Lyra 的插件系统是"应用的插件平台"。但核心设计模式可以做有意义的对比。

---

## 1. 架构模式

### BetterScroll：Class-based Mixin 模式

```
BScroll.use(PluginClass)           // 全局注册
→ BScrollConstructor.plugins[]     // 静态数组
→ new PluginClass(scroll)          // 构造时注入 BScroll 实例
→ plugin 通过 hooks 监听生命周期
```

```typescript
// PullDown 插件示例
export default class PullDown {
  static pluginName = 'pullDownRefresh'  // 约定式命名
  constructor(public scroll: BScroll) {  // 接收宿主实例
    this.init()
  }
  private init() {
    this.scroll.registerType(['pullingDown', ...])  // 扩展事件类型
    this.scroll.hooks.on('contentChanged', () => { ... })
    this.scroller.hooks.on('computeBoundary', (boundary) => { ... })
  }
  finishPullDown() { ... }       // 公共 API 自动挂载到 BScroll 实例
  autoPullDownRefresh() { ... }
}
```

**特点：**
- 插件是类，实例化后直接持有 BScroll 引用
- 通过 TypeScript module augmentation 扩展类型：`declare module '@better-scroll/core' { interface CustomOptions { pullDownRefresh?: ... } }`
- 插件 API 通过 `propertiesProxy` 自动代理到 BScroll 实例上
- 插件之间通过共享的 hooks/events 通信
- `ApplyOrder.Pre / Post` 控制初始化顺序

### Lyra：Function-based 注册模式

```
definePlugin({ name, setup(ctx) })
→ loadPlugin(spec)
  → validate apiVersion
  → createHost(pluginName, disposables, capabilities)
  → spec.setup({ host })
  → registerLoaded(spec, disposables)
```

```typescript
// 主题插件示例
export default defineThemePlugin({
  id: "dark",
  scheme: "dark",
  brand: { accent: "#1ed760", textOnAccent: "#000000" },
  surfaces: { bg: "#010102", surface: "#181a1d" },
  ink: { text: "#f7f8f8", textSoft: "#d0d6e0", ... },
  // 无需构造函数，无需引用宿主实例
})
```

**特点：**
- 插件是纯数据描述符 + setup 函数，无类继承
- 通过 `host` 对象访问宿主能力，而非直接持有宿主引用
- 所有注册返回 `Disposable`，由框架管理生命周期
- 独立的 Zustand store 管理插件状态，React 组件订阅响应式更新
- TypeScript 类型安全通过 `defineExtensionPoint<T>()` 实现而非 module augmentation

---

## 2. 扩展点机制

### BetterScroll

扩展点本质上是 **hooks/events**：

| 扩展点 | 类型 | 说明 |
|--------|------|------|
| `hooks.on(eventName, handler)` | 生命周期钩子 | refresh, enable, disable, destroy, beforeInitialScrollTo, contentChanged |
| `on(eventName, handler)` | 用户事件 | beforeScrollStart, scroll, scrollEnd, flick, touchEnd 等 |
| `registerType(names)` | 自定义事件 | 插件可注册新事件类型 |
| `options[pluginName]` | 配置开关 | 通过 options 启用/禁用插件 |
| `propertiesProxy` | API 代理 | 插件方法通过代理暴露到 BScroll 实例 |

**局限：** 扩展点完全限定在 BScroll 的滚动生命周期内。没有 UI 插槽、没有命令注册、没有主题系统。

### Lyra

扩展点系统分为三个层次：

**层次一：Kernel Extension Points（第一方扩展点）**

共 30+ 个类型化的扩展点，覆盖所有子系统：

| 系统 | 扩展点 | 说明 |
|------|--------|------|
| 主题 | `THEME`, `ACCENT` | 注册主题 / 强调色 |
| 路由 | `ROUTE` | 注册新页面路由 |
| 命令 | `COMMAND` | 注册命令面板条目 |
| 工作区 | `WORKSPACE_VIEW`, `SETTINGS_PANE` | 注册视图 / 设置面板 |
| 侧边栏 | `SIDEBAR_SECTION`, `SIDEBAR_RAIL_ITEM` | 注册侧边栏区块 |
| 编辑器 | `COMPOSER_PLACEHOLDER`, `COMPOSER_STATUS`, `SLASH_COMMAND`, `COMPOSER_KEY_BINDING`, `COMPOSER_ATTACHMENT_SOURCE` | 编辑器的每个可扩展点 |
| 消息 | `MESSAGE_ROLE`, `CONTENT_BLOCK`, `MESSAGE_CITATION_SOURCE` | 消息渲染 |
| 工具 | `TOOL_ACTION`, `TOOL_PREVIEW`, `TOOL_ICON` | 工具卡片 |
| 事件 | `STREAM_EVENT_HANDLER`, `CUSTOM_EVENT_HANDLER` | 运行时事件流 |
| 布局 | `LAYOUT_SLOT` | UI 插槽（类似 Vue slot） |
| 数据 | `DATA_PROVIDER`, `AGENT_SOURCE` | 数据层 |
| 基础设施 | `RPC_BEFORE_REQUEST`, `RPC_AFTER_RESPONSE`, `LOG_SUBSCRIBER` | RPC/日志钩子 |
| 生命周期 | `READY_HANDLER`, `BEFORE_UNLOAD_HANDLER`, `PLUGIN_LOAD_LISTENER`, `PLUGIN_UNLOAD_LISTENER` | 插件间生命周期 |

每个扩展点通过 `defineExtensionPoint<T>({ id, capability, keying })` 定义，支持：
- `keying: "single"` — 每个 key 只能有一个贡献者（后注册覆盖前者）
- `keying: "multi"` — 多个贡献者共存，按 order 排序
- `capability` — 关联到权限模型
- `normalizeKey` — 自动规范化 key（如 "Cmd+K" → "mod+k"）

**层次二：Plugin-defined Extension Points（第二方/第三方扩展点）**

任何插件都可以用 `defineExtensionPoint<T>()` 创建自己的扩展点，其他插件通过 `host.extensions.contribute(point, item)` 填充。这是 JetBrains 风格的"插件打开点供其他插件填充"机制。

```typescript
// 插件 A 定义扩展点
export const MY_FORMATTER = defineExtensionPoint<Formatter>({
  id: "my-plugin.formatter",
  capability: "extensions",
});

// 插件 B 贡献格式化器
host.extensions.contribute(MY_FORMATTER, { priority: 1, format: (text) => ... });
```

**层次三：Layout Slots（UI 插槽）**

通过 `<Slot name="app.sidebar" />` 等命名的 DOM 节点，插件可以将 React 组件注入到任意 UI 位置。这是 VSCode 式的 WebView 面板 + JetBrains 式扩展点的混合。

---

## 3. 安全模型

### BetterScroll：无安全模型

- 所有插件在同一个 JS 上下文中运行
- 没有能力声明、没有权限检查
- 插件可以访问 BScroll 实例的全部内部状态
- 没有 sideload 概念（都是 npm 安装）

### Lyra：三级安全模型

**第一级：能力声明（Capability Declaration）**

```typescript
definePlugin({
  name: "my-plugin",
  capabilities: ["storage", "notify", "theme"],  // 白名单
  setup({ host }) { ... }
})
```

共 26 个能力，分为三个风险等级：

| 风险等级 | 能力 | 说明 |
|---------|------|------|
| `safe` | notify, log, i18n, state, config, storage, theme | 纯数据/展示，无外部影响 |
| `moderate` | tool, message, events, layout, workspace, router, composer, sidebar, shortcuts, agent, data, commands, extensions, settings, window, tasks, lifecycle | 注册贡献/改变 UI，范围限本应用 |
| `dangerous` | rpc, plugins | 可访问后端/网络，或加载代码 |

**第二级：Sideload 默认拒绝**

```typescript
const origin = pluginOrigin(spec.name);
const declared = origin === "sideload" 
  ? (spec.capabilities ?? [])  // 无声明 = 零权限
  : spec.capabilities;          // 内置插件 = 全权限
```

**第三级：Proxy 门控（Capability Gate）**

```typescript
function restrictHost(host, pluginName, allowed) {
  // 用 Proxy 拦截未授权的 namespace 访问
  // host.rpc.get() → throws "not in declared capabilities"
}
```

未声明的 namespace 通过 Proxy 在 **访问时** 抛出清晰错误，而非在 setup 时静默忽略。每个扩展点的 `capability` 字段也参与门控——sideload 插件只能向已声明能力的扩展点贡献。

**额外安全措施：**
- Sideload 插件必须命名空间化标识符：`plugin:<name>/<symbol>`
- 插件加载时记录 origin，防止名称伪造
- 插件 pack 的子插件继承父级的 origin 和能力声明

---

## 4. 生命周期管理

### BetterScroll

| 阶段 | 机制 |
|------|------|
| 注册 | `BScroll.use(PluginClass)` — 添加到全局静态数组 |
| 初始化 | 每个 BScroll 实例构造时，遍历 plugins 数组，`new PluginClass(scroll)` |
| 启用/禁用 | `hooks.on('enable/disable')` |
| 销毁 | `hooks.on('destroy')` + 手动清理 hooks 和 DOM 事件 |
| 卸载 | 无正式卸载机制 |

插件实例与 BScroll 实例 1:1 绑定。同一个 BScroll 实例上的两个插件通过共享的 hooks/events 通信。

### Lyra

| 阶段 | 机制 |
|------|------|
| 注册 | `definePlugin(spec)` → `loadPlugin(spec)` |
| 验证 | `apiVersion` 范围检查（semver satisfies） |
| 拓扑排序 | `requires` 依赖解析 + Kahn 算法 + 循环检测 |
| 初始化 | `spec.setup({ host })` → 返回 cleanup 函数或 void |
| 激活 | 即时（onStartup）或延迟（onCommand:xxx，首次使用才 setup） |
| 运行中 | 插件可通过 `host.plugins.load/unload/reload` 动态加载/卸载其他插件 |
| 卸载 | `unloadPlugin(name)` → 递归 dispose 所有注册返回值 |
| 错误恢复 | setup 失败时 dispose 已注册的内容，不传播异常 |

**延迟激活（Lazy Activation）** 是 Lyra 独有的：

```typescript
definePlugin({
  name: "heavy-plugin",
  activationEvents: ["onCommand:heavy-plugin.search"],  // 声明延迟激活
  contributes: {
    commands: [{ id: "heavy-plugin.search", title: "Search", ... }]
    // 命令在命令面板中可见，但 setup 尚未执行
  },
  async setup({ host }) {
    // 只在用户首次运行搜索命令时才加载
  }
})
```

占位符系统：延迟插件的 `contributes.commands/views/settingsPanes` 在激活前就已经可见和可交互，首次交互触发 `activateLazy() → loadPlugin()` → 占位符替换为真实实现。

---

## 5. 通信机制

### BetterScroll

| 模式 | 实现 |
|------|------|
| 宿主 → 插件 | 构造函数注入 BScroll 实例，插件直接访问所有属性 |
| 插件 → 宿主 | 通过 `scroll.trigger(eventName)` 发送事件 |
| 插件 → 插件 | 共享 BScroll 实例，通过 hooks/events 间接通信 |
| 类型安全 | 通过 TypeScript module augmentation（`declare module`）扩展接口 |

### Lyra

| 模式 | 实现 |
|------|------|
| 宿主 → 插件 | `host` 对象（能力门控后的受限 API） |
| 插件 → 宿主 | 通过扩展点贡献（命令、视图、事件处理器等） |
| 插件 → 插件 | 三种方式：`host.commands.execute(id)`（命令调用）、`host.state.slice(name)`（共享状态）、`host.config.get/set/onChange`（配置广播） |
| 插件 → 后端 | `host.rpc.get/post`（通过 Go 后端的 HTTP API） |
| 类型安全 | `defineExtensionPoint<T>()` 的泛型约束 + `ExtensionPoint<T>` 的 phantom type |

**共享状态（State Slice）：**

```typescript
// 插件 A（生产者）
const counter = host.state.slice("counter", { value: 0 });
counter.set({ value: 10 });

// 插件 B（消费者）
const counter = host.state.slice("counter", { value: 0 });
counter.subscribe((v) => console.log(v.value));  // → 10
```

**跨插件命令调用（VSCode 风格 executeCommand）：**

```typescript
// 插件 B 调用插件 A 注册的命令
await host.commands.execute("plugin-a.format", selectedText);
```

---

## 6. 数据持久化

| 维度 | BetterScroll | Lyra |
|------|-------------|------|
| 插件存储 | 无内置方案 | `host.storage.get/set/remove/keys/clear` — 自动命名空间隔离（`lyra.plugin.<name>.<key>`） |
| 应用配置 | 无 | `host.config.get/set/has/onChange` — 应用级键值，变更时广播给所有订阅者 |
| 共享状态 | 无 | `host.state.slice(name, initial)` — 带类型的 Zustand slice |

---

## 7. UI 贡献方式

### BetterScroll

无 UI 贡献机制。插件可以通过 DOM 操作修改滚动容器的子元素，但这是副作用而非声明式贡献。

### Lyra

| 方式 | API | 说明 |
|------|-----|------|
| 布局插槽（Layout Slot） | `host.layout.register(slotName, { id, component, order })` | 将 React 组件注入到 `<Slot name="...">` 位置 |
| 命令面板 | `host.commands.register({ id, title, run })` | 注册 ⌘K 可调用的命令 |
| 工作区视图 | 通过 `WORKSPACE_VIEW` 扩展点 | 注册 Tab 页视图（如 Diff、Files、Terminal） |
| 设置面板 | 通过 `SETTINGS_PANE` 扩展点 | 注册设置页 |
| 侧边栏区域 | 通过 `SIDEBAR_SECTION` 扩展点 | 注册侧边栏自定义区块 |
| 消息渲染器 | `host.message.registerContentBlock(kind, renderer)` | 注册自定义消息块类型的渲染器 |
| 主题/强调色 | 通过 `THEME` / `ACCENT` 扩展点 | 注册颜色主题 |
| 国际化 | `host.i18n.addBundle(locale, dict)` | 注册翻译词条 |

每个贡献都返回 `Disposable`，卸载时自动清理。PluginProvider 用 React Error Boundary 包裹每个插件 UI 贡献，单个插件崩了不影响全局。

---

## 8. 核心设计哲学对比

| 维度 | BetterScroll | Lyra |
|------|-------------|------|
| **设计范式** | Class-based Mixin（面向对象） | Function-based Registration（函数式） |
| **宿主耦合** | 紧耦合——插件持有宿主实例引用 | 松耦合——插件通过 host 门面交互 |
| **类型扩展** | TypeScript module augmentation | 泛型 + phantom type 的类型安全扩展点 |
| **权限模型** | 无 | 三级（safe / moderate / dangerous） |
| **插件隔离** | 无——同实例共享所有状态 | 错误隔离 + 能力门控 + 命名空间强制 |
| **延迟加载** | 不支持 | 支持（activationEvents） |
| **动态加载/卸载** | 不支持 | 支持（host.plugins.load/unload/reload） |
| **依赖管理** | 无 | 拓扑排序 + 传递性跳过 + 循环检测 |
| **UI 贡献** | 无 | 完整的 Slot + Extension Point 系统 |
| **插件间通信** | 隐式（共享 BScroll 实例） | 显式（commands.execute + state.slice + config） |
| **打包分发** | 无 | PluginPack（一个 manifest = N 个子插件） |

---

## 9. 哪些 Lyra 做得好（BetterScroll 没有的）

1. **能力声明 + 权限模型** — 第三方插件安全的基础
2. **延迟激活** — 启动快，插件按需加载
3. **扩展点类型系统** — `defineExtensionPoint<T>()` 在整个注册→查询链路保持类型安全
4. **Disposable 模式** — 所有注册返回值可追踪，卸载时级联清理
5. **插件 pack** — 一组相关插件作为一个单元分发和管理
6. **拓扑排序加载** — 依赖关系明确，循环自动检测
7. **跨插件通信基础设施** — 命令调用 + 共享状态 + 配置广播
8. **插件管理 API** — `host.plugins.list/onLoad/onUnload/load/unload/reload` 让插件可以管理其他插件
9. **错误隔离** — 单个插件崩溃不影响主机和其他插件

## 10. BetterScroll 做得简洁（值得借鉴的）

1. **零配置可用** — `BScroll.use(Plugin)` 一行代码注册
2. **约定优于配置** — `static pluginName` 和 `options[pluginName]` 自然对应
3. **API 自动代理** — 插件公共方法通过 `propertiesProxy` 自动暴露到宿主实例
4. **自重很轻** — 整个插件系统核心不到 200 行
5. **TypeScript augmentation 巧妙地扩展类型** — `declare module` 让插件选项自然融入宿主 Options 类型

## 11. Lyra 可以改进的

1. **插件发现/市场** — 目前只有 builtin + sideload，缺少插件浏览/安装机制
2. **插件配置 Schema** — `host.config` 是扁平的键值对，缺少结构化配置描述（JSON Schema 或类似）
3. **插件间版本兼容声明** — 只有 `requires`（依赖），没有 `conflicts` / `breaks`
4. **插件热重载的开发体验** — 目前有 reload API，但缺少文件监听 + 自动重载的开发模式
5. **插件性能分析** — `measurePluginLoad` 已记录耗时，但没有暴露给插件作者的可视化面板

---

## 总结

| | BetterScroll | Lyra |
|---|:---:|:---:|
| 复杂度 | ★☆☆☆☆ | ★★★★★ |
| 安全模型 | ☆☆☆☆☆ | ★★★★★ |
| 扩展性 | ★★☆☆☆ | ★★★★★ |
| 易用性 | ★★★★★ | ★★★☆☆ |
| 适用场景 | 库的配置扩展 | 应用的插件平台 |

BetterScroll 的插件系统是一个**精心设计的库扩展机制**，适合单一领域的轻量级扩展。Lyra 的插件系统是一个**完整的应用插件平台**，具备权限模型、延迟加载、类型安全扩展点、插件间通信和完整生命周期管理——更像 VSCode Extension API 的轻量版。

两者的差距本质上是"框架内部机制"和"对外开放的平台 API"之间的差距，并非同一个赛道的比较。
