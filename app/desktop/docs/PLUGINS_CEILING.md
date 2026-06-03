# 插件系统能力上限分析（横向对比 + 演进方向）

> 本文是一次**深度复盘**：Lyra 当前插件系统能力到哪、对比 VSCode / JetBrains / Obsidian / Raycast 还差什么、
> 以及"用插件加一个文件编辑能力"这个经典场景会撞到哪些墙。
>
> 形态对齐 `ARCHITECTURE.md §12`：每条改进都标 **做的理由 / 不做的理由 / 触发条件**，不堆 wishlist。
> 已有的写法看 `EXTENSION_POINTS.md`（扩展点底座 + 落地状态）+ `PLUGINS_IMPL.md`（实现指南），本文不重复 API 细节。

---

## 0. 结论先行（TL;DR）

1. **Lyra 的插件内核已经做到 VSCode 同级的"贡献点 + 懒激活 + 能力切片 + sideload"骨架**，
   并且在两点上**强于 Obsidian**：disposable 由 Host 自动收集（Obsidian 风格的 `this.register*` 自动清理，
   但我们连 `host.*.register*` 的返回值都自动 track）、capability proxy 的安全缝隙已经存在。
2. **真正的能力上限不在"还差几个 `register*`"，而在三处结构性缺口**：
   - (A) ~~**扩展点是封闭的**~~ **【已解决：L2+L3】** —— 现在任何插件都能 `defineExtensionPoint` 开 typed 点、别人 `contribute` 填 / `useExtensionPoint` 读（JetBrains 的核心能力已落地，见 §4 瓶颈 A）。
   - (B) **外置插件的世界 = Host 表面** —— 凡是没挂在 `Host` 上的东西（如 `activeFile`、文档模型、fs），
     sideload 插件**根本看不见**，只有同 bundle 的 builtin 能 `import @/state/*` 绕过。
   - (C) **能力是自愿声明，不是强制边界** —— 没声明 `capabilities` 就是全权限；写文件这类危险能力没有 install-time 授权。
3. **"文件编辑"场景同时踩中 A/B/C + 后端缺 `workspace.fs.*` 方法**，是检验上限最好的标尺（见 §5）。
4. 这些缺口**大多不该现在补**（YAGNI）—— 触发条件是"第一个第三方 sideload 插件 / 第一个 editor 类功能落地"。
   本文的价值是**把缺口和触发条件钉死**，避免到时候临时拍脑袋。

---

## 1. 当前能力地图（一页速览）

| 维度        | Lyra 现状                                                                                                                         | 文件                       |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| 贡献模型    | `definePlugin({ setup({host}) })`，贡献统一走 `host.extensions.contribute(POINT, …)`（POINT 见 `kernelPoints.ts`）+ 少量薄 facade | `sdk/host.ts`              |
| 注册表      | Zustand 单一 `extensions` map，`single`(覆盖+告警) / `multi`(复合键共存)                                                          | `sdk/registry.ts`          |
| 激活        | `onStartup`(默认 eager) / `onCommand:<id>`(lazy)，`contributes.{commands,views,settingsPanes}` 占位                               | `sdk/definePlugin.ts`      |
| 依赖        | `requires[]` → Kahn 拓扑排序，缺依赖/成环则 skip                                                                                  | `definePlugin.ts:topoSort` |
| 版本闸      | `apiVersion` semver `satisfies` 检查                                                                                              | `sdk/apiVersion.ts`        |
| 能力切片    | `capabilities[]` → 未声明的 namespace 变 throwing proxy                                                                           | `host.ts:restrictHost`     |
| sideload    | Go 后端列清单 → URL 动态 import → Zod 校验 default export → `loadPlugin`                                                          | `plugins/sideload.ts`      |
| 卸载/热重载 | disposable sink 由 Host 收集，`host.plugins.{unload,reload}`                                                                      | `definePlugin.ts`          |
| 错误隔离    | `PluginBoundary`(React) + `safeCall` + 按插件归因 + 可注册 errorFallback                                                          | `PluginBoundary.tsx`       |
| 跨插件通信  | `host.state.slice(name, initial)`（约定名共享 ephemeral state）                                                                   | `sdk/stateSlice.ts`        |
| 后端通信    | `host.rpc.{get,post}`(JSON-RPC) / `host.agui.on`(CUSTOM 事件) / `host.extensions.contribute(DATA_PROVIDER, …)`                    | `host.ts`, `docs/API.md`   |
| `when` 子句 | 上下文表达式求值（命令/菜单可见性）                                                                                               | `sdk/evalWhen.ts`          |

**一句话**：内核不长肉，所有面都是插件贡献；Host 是插件能看到的**唯一**世界。

---

## 2. 横向对比：四大插件生态的设计哲学

每个生态有一个"招牌设计"，Lyra 对位如下。

### 2.1 VSCode —— 声明式 manifest + 进程隔离 + 懒激活

- **`package.json contributes` 是纯数据**：commands / menus / keybindings / languages / themes / views 全部
  **不运行插件代码就能解析**。所以 marketplace 能列、能搜、能预览，激活才真正 lazy。
- **Extension Host 独立进程**：扩展跑在单独的 Node 进程，崩溃/死循环/内存泄漏不波及 renderer，API 表面稳定。
- **`commands.executeCommand(id, ...args)`**：扩展之间通过命令互相调用，命令即轻量 RPC。
- **FileSystemProvider / LSP / DAP**：文件系统、语言能力、调试都是**带外协议**的扩展点。
- **Lyra 对位**：贡献点 ✓、懒激活 ✓（但 `contributes` 只覆盖 commands/views/settingsPanes 三类）、
  进程隔离 ✗（同 renderer）、`executeCommand` ✗、fs/LSP 扩展点 ✗。

### 2.2 JetBrains —— 插件可以**定义自己的扩展点**

- `plugin.xml` 里插件不仅能 `<extensions>`（填别人的点），还能 `<extensionPoints>`（**开自己的点让别人填**）。
  扩展性是**递归开放**的：editor 插件开一个 `formatter` 点，formatter 插件去填，内核全程不参与。
- 每插件独立 classloader 强隔离；后期补了 dynamic plugin（不重启加载/卸载）。
- **Lyra 对位（已追平，L2+L3）**：`defineExtensionPoint<T>` + `host.extensions.contribute` + `useExtensionPoint` —— 插件开 typed 点、别人填/读，kernel 自己的 ~35 个点也跑在同一底座上。曾经"最大的上限差距"现在已抹平。

### 2.3 Obsidian —— 极简 + 自动资源回收

- `Plugin` 类 `onload/onunload`；`this.registerEvent / addCommand / registerView` 等注册的东西**自动在卸载时清理**。
- 全程同 renderer，**无能力沙箱**，信任模型 = "代码开源你自己看"。
- **Lyra 对位**：自动清理我们**做得更彻底**（`createHost` 的 `track()` 把每个 `register*` 返回的 disposable
  压进 sink，`unload` 一次性 dispose，插件作者不用手动 `push`）；而且我们**多了** capability proxy + 依赖排序 +
  apiVersion 闸 —— 这三样 Obsidian 都没有。**这一档 Lyra 全面领先。**

### 2.4 Raycast —— 受限组件集 = 一致 UX + 可沙箱

- 插件不拿原始 DOM，只能用官方 `List / Detail / Form / ActionPanel / Grid` 组合 → UX 强一致、易沙箱。
- 命令是入口单元，preferences 在 manifest 里类型化声明。
- **Lyra 对位**：我们走的是**相反路线** —— 插件拿到的是 React `ComponentType`，自由渲染（`common/` 提供
  Radix 薄包件作为"推荐基件"但不强制）。对桌面 IDE 形态这是对的（power user 要自由度），
  代价是**做不到 Raycast 那种"插件 UI 天然沙箱"**。

### 2.5 一句话总结对位

| 招牌设计               | VSCode            | JetBrains | Obsidian | Raycast | **Lyra**                           |
| ---------------------- | ----------------- | --------- | -------- | ------- | ---------------------------------- |
| 声明式 manifest 预解析 | ⭐⭐⭐            | ⭐⭐⭐    | ⭐       | ⭐⭐⭐  | ⭐（仅 3 类 contributes）          |
| 插件自定义扩展点       | ✗                 | ⭐⭐⭐    | ✗        | ✗       | **⭐⭐⭐（defineExtensionPoint）** |
| 进程/沙箱隔离          | ⭐⭐⭐            | ⭐⭐⭐    | ✗        | ⭐⭐    | ✗                                  |
| 自动资源回收           | ⭐⭐（手动 push） | ⭐⭐      | ⭐⭐⭐   | ⭐⭐    | **⭐⭐⭐（全自动）**               |
| 能力/权限模型          | ⭐⭐              | ⭐⭐      | ✗        | ⭐⭐    | ⭐（自愿声明）                     |
| 依赖管理               | ⭐⭐              | ⭐⭐⭐    | ⭐       | ⭐      | ⭐⭐（topo-sort）                  |
| 命令互调               | ⭐⭐⭐            | ⭐⭐⭐    | ⭐⭐     | ⭐      | ✗                                  |

---

## 3. Lyra 已经做对的（别动）

写在前面，避免"补短板"时把长板也一起重构掉：

- ✅ **Host 自动收集 disposable** —— 比 VSCode `context.subscriptions.push` / Obsidian `this.register*` 更省心，
  插件作者零心智负担。**不要**改成手动管理。
- ✅ **disposable 收集 + React 错误边界 + safeCall + 按插件归因** —— 错误隔离这套已经很完整（见 `PLUGINS_IMPL.md §8`）。
- ✅ **`state.slice` 作为轻量跨插件总线** —— producer/consumer 约定名 + 类型即可通信，无硬 import。
- ✅ **theme = plugin** —— `defineThemePlugin` 的 manifest 一行加主题，VSCode 同级且更 DRY。
- ✅ **sideload 边界 Zod 校验** —— 信任边界拒脏数据，符合 CLAUDE.md "边界校验用 Zod"。
- ✅ **开放扩展点底座（L2+L3 已落地）** —— 40 个 owned-map 塌成单一 `extensions`，贡献统一走 `host.extensions.contribute(POINT, …)`，typed `ExtensionPoint<T>` 保住类型推断。见 `EXTENSION_POINTS.md`。（注：本文早期版本曾建议"保持 per-slot register\*、不抽 factory" —— 已被底座方案取代。）

---

## 4. 能力上限的四个真实瓶颈

### 瓶颈 A：扩展点封闭（kernel-only）—— ✅ 已由扩展点底座解决（L2+L3）

> **更新（2026-06）**：此瓶颈已基本消除。`defineExtensionPoint<T>({ id, keying })` 让**任何插件**开自己的 typed 点，别的插件 `host.extensions.contribute(point, …)` 填、`useExtensionPoint(point)` 读 —— JetBrains 的"插件开点 → 别人填点"现在在 Lyra 成立，且 kernel 自己的 ~35 个点也只是底座上的保留点（与第三方对等）。新增 kernel 点 = `kernelPoints.ts` 一个 `defineExtensionPoint` + 一个 selector，不再改 4 处。详见 `docs/EXTENSION_POINTS.md`。下面保留原始分析作背景。

新增任何扩展点 = 改 `host.ts` + `registry.ts` + `selectors/*` + `types/*` 四处。对 **builtin** 这没问题
（kernel 本来就该拥有这些点）。但对**生态**，这意味着：第三方插件无法为另一个第三方插件开放贡献点。
JetBrains 的"插件开点 → 别人填点"在 Lyra 不成立 —— 唯一逃逸口是 `state.slice` 的约定名共享，无类型化、无发现机制。

### 瓶颈 B：外置插件只能看见 Host 表面

sideload 插件是**独立 bundle**，`@/` alias 不解析，它的整个宇宙就是 setup 收到的 `host`。
凡是没挂在 `Host` 上的东西它都摸不到：

- `activeFile` 在 `useSessionStore`，**不在 Host 上** → 外置插件不知道用户选了哪个文件。
- 文档/缓冲区模型不存在 → 外置插件无法参与"打开的文档"。
- builtin 插件能 `import @/state/sessionStore`（`files.tsx` 就这么干）**绕过**这个限制，
  造成 **builtin 与 sideload 能力不对等** —— 文档里看不出来，写第三方插件时才撞墙。

### 瓶颈 C：能力是自愿，不是强制边界

`capabilities` 省略 = 全权限；声明了才变 throwing proxy。这是"自律契约 + 未来 hook"，
**不是**安全边界。对写文件 / 执行命令这类危险能力，缺 install-time 授权提示 + 路径作用域。

### 瓶颈 D：无 fs / 文档 / 编辑器扩展点（场景级缺口）

`workspace` namespace 只有 `registerView / openView / closeView`。**没有** `fs.read/write/watch`、
没有文档模型、没有"editor 增强点"（formatter/linter/lens）。后端 JSON-RPC 方法表里也**没有** `workspace.fs.*`。
这正是 §5 场景要撞的墙。

---

## 5. 经典场景走查：「用插件加一个文件编辑能力」

目标：装一个插件，能在工作区打开一个**可编辑**的文件 tab，改完 `Mod+S` 存回磁盘。
下面是**用当前系统**的真实路径，每步标注摩擦点 → 映射瓶颈。

### Step 1 — 注册一个 editor workspace view（✅ 现成）

```ts
definePlugin({
  name: "acme.file-editor",
  version: "1.0.0",
  capabilities: [
    "workspace",
    "rpc",
    "state",
    "commands",
    "shortcuts",
    "notify",
  ],
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "editor",
      title: "Editor",
      icon: "filetext",
      component: EditorView,
    });
    host.extensions.contribute(COMMAND, {
      id: "editor.save",
      title: "Save File",
      run: () => save(),
    });
    host.extensions.contribute(SHORTCUT, { key: "Mod+S", run: () => save() });
  },
});
```

这一段**完全可行**，和 `workspace-views/files.tsx` 同款。

### Step 2 — EditorView 需要知道"当前文件" → 撞 **瓶颈 B**

builtin 可以 `const path = useSessionStore(s => s.activeFile)`。
但**外置插件没有 `useSessionStore`**，Host 上也没有 `host.workspace.activeFile` / `onActiveFileChange`。
→ 外置 editor 插件**根本拿不到当前文件路径**。
（临时绕法：约定一个 `host.state.slice("workspace.activeFile", null)`，但要求另一个插件去写它，脆弱。）

### Step 3 — 读文件内容 → 撞 **瓶颈 D + 后端缺方法**

```ts
const content = await host.rpc.get("v1/rpc/workspace.fs.read", { path }); // ← 方法不存在
```

后端 JSON-RPC 方法表（`docs/API.md`）里**没有** `workspace.fs.read/write/list`。
需要在 Go Runtime 新增这组方法（dot-named，watch 走 streaming subscribe，对齐 `workspace.terminal.subscribe`）。
且按"Runtime 无状态、无 user 概念"的不变量，**文件作用域/授权是 facade 层的事**，不进协议 body。

### Step 4 — 缓冲区 / dirty 状态 → 撞 **瓶颈 A**

没有文档模型，插件只能自己用 `host.state.slice("editor.buffers", {})` 或私有 store 管理 buffer/dirty。
如果希望**别的插件**（formatter、字数统计、Git gutter）也能挂到这个文档上 —— 当前**做不到**：
没有"editor 文档"这个可被贡献的扩展点（JetBrains 会用扩展点，VSCode 会用 `TextDocument` + `languages.register*`）。

### Step 5 — 保存 → 撞 **瓶颈 C**

```ts
await host.rpc.post("workspace.fs.write", { path, content }); // 任何声明了 rpc 的插件都能写任意路径
host.notify("Saved");
```

一个只声明了 `["rpc"]` 的插件就能写任意后端允许的路径。**没有** install-time"此插件请求文件写入权限"提示，
**没有**路径作用域。对 builtin 无所谓，对第三方 sideload 这是真实风险。

### Step 6 — 选文件时打开 editor 而非 diff → 撞 **瓶颈 B（再次）**

`files.tsx` 里选中文件**硬编码** `openMainView({id:"diff"})`。要改成打开 editor，要么 fork files.tsx（坏），
要么监听 `activeFile` 变化再 `host.workspace.openView("editor")` —— 而监听 activeFile 又回到 Step 2 的墙。

### 场景结论

| 步骤                  | 现状                          | 瓶颈 |
| --------------------- | ----------------------------- | ---- |
| 注册 editor view      | ✅ 现成                       | —    |
| 拿当前文件            | ❌ 外置插件看不到             | B    |
| 读文件                | ❌ 后端无 fs 方法             | D    |
| 文档模型 / 第三方增强 | ❌ 无扩展点                   | A    |
| 写文件                | ⚠️ 无授权/作用域              | C    |
| 选中→打开 editor      | ❌ 硬编码 + 看不到 activeFile | B    |

**一个看似普通的需求，精准踩中全部四个瓶颈。** 这说明"文件编辑"不是一个 view，
而是一组**新的 Host 能力面 + 后端协议面 + 安全面**。

---

## 6. 改进方向 catalog（分档 + 做/不做/触发条件）

### Tier 1 — 抬升上限、低风险、清晰 ROI

#### T1-a. 通用开放扩展点（解瓶颈 A）

**做的理由**：给 Host 加一组**泛型集合 API**，让插件能开自己的贡献点，不必改 kernel：

```ts
host.registry.define<T>(id); // 开点
host.registry.contribute(id, item); // 填点（返回 disposable，复用现有 track）
useRegistryCollection<T>(id); // 渲染端订阅
```

本质是把 `state.slice` 从"单值共享"升级为"有序集合 + 订阅 + disposable"，复用 `addOwnedMulti` 那套。
**这是从 VSCode 风格走向 JetBrains 风格的关键一步。**

**不做的理由**：泛型集合**擦类型**（callers cast），而现有 per-slot `register*` 有完整推断 ——
CLAUDE.md 明确反对把 30 个 typed slot factory 化。所以这必须是**增量并存**：kernel 自己的点保持强类型，
**只新增**一个泛型集合给"插件 ↔ 插件"用，不动既有任何 `register*`。

**触发条件**：第一次出现"两个 sideload 插件要互相扩展"（如 editor 插件想让 formatter 插件挂进来）。

#### T1-b. `host.commands.execute(id, ...args)`（VSCode parity）

**做的理由**：命令目前只能注册不能**程序化调用**。加一个 `execute` 让命令成为轻量服务总线，
插件间组合零耦合。实现极小（registry 已有 commands map，查表调 `run`）。

**不做的理由**：无 —— 这条几乎是纯收益，唯一要想清楚的是返回值类型（`unknown` + caller cast）。

**触发条件**：第一个想触发别的插件动作的插件出现（如 "editor.save" 被 conversation-export 调用）。

### Tier 2 — "文件编辑"这类功能的前置（解瓶颈 B + D）

#### T2-a. `host.workspace` 暴露 active-file / 文档读访问

**做的理由**：把 `activeFile` + `onActiveFileChange` 挂上 Host，**抹平 builtin 与 sideload 的能力差**（解瓶颈 B）。
再加 fs facade（`host.fs.read/write/watch`）包住 JSON-RPC 方法字符串，插件不硬编码 method 名。

**不做的理由**：在没有任何 editor 类插件时，这是给不存在的消费者修路（YAGNI）。

**触发条件**：第一个需要读/写工作区文件的插件（即 §5 场景真正落地时）。

#### T2-b. 后端 `workspace.fs.*` JSON-RPC 方法

**做的理由**：`read / write / list / watch(subscribe)`，dot-named，watch 走 streaming（对齐 `workspace.terminal.subscribe`）。
是 §5 场景的硬前置。

**不做的理由**：协议面扩张要谨慎；且按不变量，**作用域/授权属于 facade 层**，方法本身只做无状态计算。

**触发条件**：同 T2-a，二者必须一起做。

### Tier 3 — 安全上限（解瓶颈 C，仅在有 marketplace 时）

#### T3-a. 强制权限模型

**做的理由**：`capabilities` 从自愿升级为 **default-deny + install-time 授权提示 + fs 路径作用域**。
现有 capability proxy + apiVersion 闸已经是天然的 hook 点。

**不做的理由**：**没有 marketplace**（对齐 `ARCHITECTURE.md §12.2-H`，纯 YAGNI）。当前全是 first-party builtin，强制权限是纯负担。

**触发条件**：第一个第三方 sideload 插件请求 `rpc` / `fs` / 写权限。

#### T3-b. Worker / iframe 沙箱隔离

**做的理由**：把不可信 sideload 跑进 Web Worker / iframe + postMessage RPC 桥，
拿到 VSCode Extension Host 级的崩溃/死循环/内存隔离。

**不做的理由**：工程量大（要给整个 Host 表面做 postMessage 序列化代理），当前 React 错误边界 + safeCall 对
**可信 builtin** 已够用。

**触发条件**：marketplace 上线、加载**未经审查**的第三方插件。

### Tier 4 — 声明式 manifest 拓宽（VSCode parity，最低优先）

#### T4-a. 拓宽 `contributes`

**做的理由**：让 themes / keybindings / menus 也能在激活前被列出/搜索（不跑 setup）。

**不做的理由**：Lyra 是**单 bundle 桌面应用**，绝大多数 builtin 都 eager，预解析的边际收益很小。

**触发条件**：出现"插件很多、需要在不激活的前提下搜索/预览贡献"的产品需求（基本等同于有了 marketplace）。

---

## 7. 优先级 & 触发条件总表

| #    | 改进                                     | 解决瓶颈 | 投入 | 触发条件                           | 现状判断              |
| ---- | ---------------------------------------- | -------- | ---- | ---------------------------------- | --------------------- |
| T1-a | 通用开放扩展点 `host.registry.*`         | A        | 中   | 两个 sideload 互相扩展             | 抬上限关键，等触发    |
| T1-b | `host.commands.execute`                  | —        | 小   | 第一个跨插件命令调用               | 近纯收益，可随时做    |
| T2-a | `host.workspace` active-file + fs facade | B,D      | 中   | 第一个 fs 读写插件                 | 场景前置              |
| T2-b | 后端 `workspace.fs.*` 方法               | D        | 中   | 同 T2-a                            | 与 T2-a 同批          |
| T3-a | 强制权限模型                             | C        | 中   | 第一个第三方 sideload 请求危险能力 | YAGNI，等 marketplace |
| T3-b | Worker/iframe 沙箱                       | C        | 大   | 加载未审查第三方插件               | YAGNI，等 marketplace |
| T4-a | 拓宽声明式 `contributes`                 | —        | 中   | 需要激活前搜索/预览贡献            | 桌面形态收益低        |

**下一轮如果真要动**：T1-b（小、纯收益）可以随手做；其余全部**等触发条件**。
"文件编辑"功能一旦立项，就是 **T2-a + T2-b 同批**，且大概率顺带需要 T1-a（让别的插件增强 editor）。

---

## 8. 反向不变量（已知会被提、但当前是错方向）

- ❌ **把 31 个 typed `register*` 抽成泛型 factory** —— CLAUDE.md 已论证：类型推断收益 > LOC，T1-a 是**新增**不是替换。
- ❌ **现在就上 Worker 沙箱 / 强制权限** —— 全是 first-party builtin，无 marketplace，纯负担（YAGNI）。
- ❌ **学 Raycast 限制成受限组件集** —— 桌面 IDE 形态要 power-user 自由度，`common/` 的 Radix 薄包件做"推荐"不做"强制"。
- ❌ **给 Runtime 协议加 user / 文件作用域 / 授权** —— 不变量：Runtime 无状态、零 user 概念，作用域是 facade 层的事（见 `API.md §0`）。
- ❌ **把 `state.slice` 删掉改纯扩展点** —— 单值共享和有序集合是两种需求，T1-a 之后二者并存。
- ❌ **照搬 VSCode 独立 Extension Host 进程** —— Wails 单 WebView 形态，跨进程序列化代价大；隔离需求用 Worker（T3-b）按需上，不整体进程化。

---

## 9. 延伸阅读

- `ARCHITECTURE.md §5.1`（插件系统数据流）+ `§12`（改进方向总账）
- `EXTENSION_POINTS.md`（扩展点底座 + 落地状态）/ `PLUGINS_IMPL.md`（实现指南 + SDK 参考 + 内置插件清单）
- `API.md §0-1`（Runtime 无状态 / 协议 envelope 不装 transport 元数据）/ `§5.1`（method 命名）
- `sdk/host.ts`（Host 全表面）/ `sdk/registry.ts`（owned-map 机制）/ `sdk/definePlugin.ts`（加载生命周期）
