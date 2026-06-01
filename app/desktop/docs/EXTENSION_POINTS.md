# 统一扩展点底座 + Plugin Pack —— 方向设计 v2（待 review，未实现）

> **状态**：设计提案，等 review 后再动代码。**一行代码都还没写。**
>
> **本版野心升级**：不再是"加一个增量原语 + dogfood 两个插件"，而是
> **建一套足够通用的扩展点底座，让所有内置插件都跑在它上面** —— kernel 自己的每个"槽"也只是底座上一个保留的扩展点。
> 这把插件系统从"VSCode 式（kernel 拥有所有点）"推到 **JetBrains 式（万物皆扩展点，kernel 与第三方对等）**。
>
> 对应 `PLUGINS_CEILING.md` 的 T1-a，但范围从"增量"扩到"**底座统一**"。

---

## 0. 这一版要解决的两个明确诉求

1. **能力足够通用**：一套机制能承载当前所有不同形状的贡献（单槽覆盖 / 多槽共存 / 自定义 key / 组合键归一化 / 按事件名或 slot 名分发），第三方插件和 kernel 用**完全相同**的机制。
2. **所有内置插件都跑在这套底座上**：不是挑两个 dogfood，而是 40 个 owned-map + 31 个 `register*` 全部坐落到**一个**通用底座之上。

---

## 1. 必须先讲清楚的张力（请 review 时确认你接受）

> ⚠️ **这个方向直接推翻 CLAUDE.md 里的一条 ❌ 强反向不变量**：
> *"把 `registry.ts` 30+ 对 `addX/removeX` 抽 factory —— 剩下的 per-slot wrapper 是 type-safety 成本"*，
> 以及 `ARCHITECTURE.md §12.2-E`("强行抽 factory 必须牺牲类型推断，clarity loss > LOC saving")。

当初那条不变量的**唯一论据是"会丢类型推断"**。本设计的核心结论是：

> **类型安全 ≠ 必须用 31 个写死的 typed 方法。只要每个点暴露成一个 `ExtensionPoint<T>` 类型句柄，
> 通用底座 + 完整推断可以同时成立。** 那条不变量的前提（"通用化必然擦类型"）是错的——前提推翻，结论就该更新。

但**代价是真实的**，必须接受：

- **这是 registry 核心重写**，不是加 feature。registry 是全应用最热的代码（每个 AG-UI 事件 30/s、每次渲染都读它）。
- **爆炸半径 = 整个 app**。缓解：先做**行为保持的重构**（底座替换在现有 typed API 背后，321+ 测试不改照样绿），再谈是否迁移调用点。
- 需要**同步更新** CLAUDE.md 那条不变量 + `ARCHITECTURE.md §12.2-E` + §12.5 反向不变量（把"别抽 factory"改成"已统一到扩展点底座"）。

**如果你接受以上，再往下看具体设计。**

---

## 2. 关键洞察：现有 40 个 map 其实只有「两种 keying + 一个排序」

扫完 `registryState.ts` 的全部 owned-map，它们只有两类写入策略（已经被 `registryHelpers.ts` 抽成两个 helper）：

| 策略 | helper | 语义 | 现有用户（举例） |
|---|---|---|---|
| **single** | `addOwned` | 同 key 覆盖 + 跨插件冲突告警 | themes / commands / contentBlocks / toolPreviews / dataProviders / shortcuts … |
| **multi** | `addOwnedMulti` | 复合键 `${plugin}|${id}` 共存 | layoutSlots / coreEventHandlers / customEventHandlers / rpcHooks / logSubscribers … |

差异只在三处旋钮：
- **key 怎么取**：多数 `item.id`；少数特殊（toolPreview/toolIcon=fn 名、dataProvider=`spec.key`、contentBlock=kind、shortcut/composerKeyBinding=`normalizeCombo(key)`）。
- **single 还是 multi**。
- **消费时按什么分发**：多数"全部列出按 order 排"；layout/events 是"按 slot 名 / 事件名分组"。

**这三个旋钮就是一个点定义需要的全部参数。** 所以通用底座是可行的、而且很小 —— 它就是把现在**静态实例化**的
`ownedKeySlot / ownedSpecSlot / multiSlot` 三个工厂，改成**按 point 动态实例化**。

> 一个额外简化：layout/events 的"按 slot 名/事件名分发"**不需要单独的 indexed 概念** ——
> 让 **slot 名/事件名本身就是 point id** 即可：`contribute("layout:app.sidebar", c)` / `lookup("agui:core:RUN_STARTED")`。
> 于是 `<Slot name>` = `useExtensionPoint("layout:"+name)`，reducer = `lookup("agui:core:"+type)`。机制收敛为「单 map + single|multi + 排序」。

---

## 3. 通用底座设计

### 3.1 点定义（typed 句柄）

```ts
type Keying = "single" | "multi";

interface ExtensionPoint<T> {
  readonly id: string;
  readonly keying: Keying;
  readonly keyOf?: (item: T) => string;        // 默认 item.id；fn 名 / spec.key / kind / combo 用它
  readonly normalizeKey?: (k: string) => string; // 仅 shortcuts / composerKeyBinding 用（combo 归一化）
  // 仅类型用途的 phantom T，不持有状态
}

function defineExtensionPoint<T>(def: {
  id: string; keying: Keying; keyOf?: (i: T) => string; normalizeKey?: (k: string) => string;
}): ExtensionPoint<T>;
```

### 3.2 读写 API

```ts
// 贡献端（host 绑定 → 自动 disposable 追踪 + 归属 + single 的冲突告警）
host.extensions.contribute<T>(point: ExtensionPoint<T>, item: T, opts?: { order?: number }): Disposable;

// 消费端（不需要 host）
useExtensionPoint<T>(point: ExtensionPoint<T>): T[];     // React hook，按 order 排序
lookupExtensionPoint<T>(point: ExtensionPoint<T>): T[];  // 命令/setup/reducer 里程序化读取
```

`ExtensionPoint<T>` 句柄带着 `id`+`T`，所以 `contribute` 的 `item` 被约束成 `T`、`use/lookup` 直接推出 `T[]`。
**通用 + 全类型推断同时成立。**

### 3.3 存储

`registryState` 的 ~40 个 owned-map **塌缩成 1 个**：
```ts
extensions: Map<string, Owned<{ point: string; order?: number; item: unknown }>>;
// single: entryKey = `${point}#${normalizeKey?(keyOf(item)) ?? keyOf(item)}`  → 覆盖 + 跨插件告警
// multi:  entryKey = `${point}#${pluginName}|${mintedId}`                      → 共存
```
消费用现成 `createIndex`（WeakMap 缓存、随 Map 引用失效）按 point 聚合 + 按 order 排。**热路径性能与现状同级**。

> **不迁移的东西**（它们不是"贡献"，是 kernel 簿记）：`loaded` / `pendingActivations` / `appReady` /
> `windowTitle` / `windowBadge`。这些留在 registryState，不进扩展点。

### 3.4 kernel 点目录（typed 句柄集中定义）

新增 `sdk/kernelPoints.ts`，把现在 40 个 map 对应的点全部声明成 typed 句柄，例如：
```ts
export const THEME        = defineExtensionPoint<ThemeSpec>({ id: "lyra.theme", keying: "single" });
export const COMMAND      = defineExtensionPoint<CommandSpec>({ id: "lyra.command", keying: "single" });
export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({ id: "lyra.workspace.view", keying: "single" });
export const TOOL_PREVIEW = defineExtensionPoint<...>({ id: "lyra.tool.preview", keying: "single", keyOf: x => x.fn });
export const SHORTCUT     = defineExtensionPoint<ShortcutSpec>({ id: "lyra.shortcut", keying: "single", keyOf: s => s.key, normalizeKey: normalizeCombo });
export const CORE_EVENT   = (type: string) => defineExtensionPoint<...>({ id: `agui:core:${type}`, keying: "multi" });
export const LAYOUT_SLOT  = (slot: string) => defineExtensionPoint<LayoutSlotSpec>({ id: `layout:${slot}`, keying: "multi" });
// …其余 ~35 个
```

---

## 4. "所有内置插件都跑在底座上" —— 渐进式 L2 → L3（已定）

> **✅ 已定方向：渐进式演进 —— 先 L2（行为保持地塌底座、保留 facade、测试全绿），稳定后按 domain 一批批推进 L3。**
>
> **为什么 L2→L3 安全**：L2 做完后 `host.theme.registerTheme(spec)` 与 `host.extensions.contribute(THEME, spec)`
> 底层是同一行 `contribute`。于是 L3 可以**按 domain 一批批迁移**、**两套 API 全程共存**（迁某 domain 时 facade 仍工作）、
> **facade 只在最后一个调用者迁走后才删**（knip 抓孤儿）、**随时可停**。风险单调下降，没有 big-bang。

"跑在底座上"有三个侵入级别，下面 L2 / L3 是我们的两个阶段，L1 仅作对照排除：

### L1 —— 仅增量（不满足你的诉求）
底座只给第三方/新点用，现有 40 map + 31 方法原样不动。
→ ❌ **不满足"所有内置都跑在上面"**，排除。

### L2 —— 底座塌缩 + 保留 typed facade（**推荐**）
- registryState 的 40 map → 1 个 `extensions` map（**底座成为唯一实现**）。
- 现有 `host.theme.registerTheme(spec)` 等 31 个方法**保留**，但实现塌成一行 facade：`contribute(THEME, spec)`。
- 现有 `useThemes()` 等 selector 塌成一行：`useExtensionPoint(THEME)`。
- **内置插件源码一个字不用改**（call site 不变）；改的是 registry 内部。
- ✅ "所有贡献都流经底座" 在**实现层**成立；✅ 全类型安全；✅ 321 测试不改照样绿（行为保持）；✅ churn 最小。
- **代价**：SDK 表面仍有 31 个方法（但都是 1 行 facade）。有人会问"那不还是 31 个方法吗" —— 区别在于它们不再各自背一个 map/工厂，新增点不再是 4 文件改动，且第三方与 kernel 同底座。

### L3 —— 全量源码迁移 + 删除 typed facade（最纯，churn 最大）
- 在 L2 基础上，**删掉 31 个 typed 方法**，把 ~69 个内置插件源码全改成 `host.extensions.contribute(KERNEL.theme, spec)`。
- ✅ 真正"一种 API"，SDK 表面最小；kernel 与第三方零差别。
- **代价**：69 文件机械改动；丢掉 `host.theme.registerTheme` 的**自解释性/autocomplete 可发现性**；
  shortcut 的 `normalizeCombo` 等"就近语义"从 facade 散到调用点（或挪进点定义）。
- **我的诚实评估**：L3 的纯粹性收益**有限**，churn 和可发现性损失**真实**。除非你明确要"源码层只有一种写法"，否则 L2 已经实现了"所有内置跑在通用底座上"的实质。

> **我的建议：先做 L2（底座统一，行为保持，测试全绿），稳定后再评估要不要对部分/全部源码做 L3 迁移。**
> L2→L3 是纯机械、可增量、可随时停的。一上来就 L3 是把"核心重写"和"69 文件改动"两个风险叠在一次。

---

## 5. Plugin Pack（插件的插件）—— 与底座正交

无论 L2/L3，pack 都一样：

```ts
definePluginPack({
  name, version, requires?, capabilities?,  // 若声明 capabilities 须含 "plugins"
  children: PluginSpec[],                    // 真正独立的子 PluginSpec
  setup?,                                    // 在子插件全部加载后跑 → 可消费它们填的点
});
```
语义：父 setup 里先按序 `await host.plugins.load(child)`，再跑 `setup`，cleanup 级联逆序 `unload`。
kernel manifest 只见 1 条；子插件各自 host/capabilities/disposable/错误隔离。`host.plugins.load` 已存在，helper 只封装"加载 + 级联卸载"。

**已知约束**（review 关注）：子插件 `requires` 不走 topoSort（靠 children 数组顺序，倾向 YAGNI 不补）；
capabilities 须含 `"plugins"`；Plugins pane v1 平铺显示（父子树是 polish，v1 不做）。

---

## 6. 落地面积（L2 路线）

### 底座（新增/改）
| 文件 | 改动 |
|---|---|
| `sdk/types/extensions.ts` | **新增** `ExtensionPoint<T>` |
| `sdk/defineExtensionPoint.ts` | **新增** `defineExtensionPoint` |
| `sdk/kernelPoints.ts` | **新增** ~40 个 kernel 点的 typed 句柄 |
| `sdk/registryState.ts` | 40 map → 1 `extensions` map（保留 4 个簿记字段） |
| `sdk/registry.ts` | 用单一 `contribute/remove` 实现取代 ~40 对 add/remove；31 个 `addX` 变成调 contribute 的 facade |
| `sdk/host.ts` | 加 `extensions.contribute`；31 个 `register*` 改为调 contribute（签名/行为不变） |
| `sdk/selectors/*` | 各 `useXxx/lookupXxx` 改为 `useExtensionPoint(POINT)` 的薄封装（**对外签名不变**） |
| `sdk/types/host.ts` / `plugin.ts` | Host 加 `extensions`；HostCapability 加 `"extensions"` |
| `sdk/index.ts` | 导出底座 API（现有 selector 导出保持） |
| `sdk/extensions.test.ts` | **新增** 底座单测（single 覆盖+告警 / multi 共存 / order / keyOf / normalizeKey / 跨 point 隔离） |

> **关键安全网**：L2 下**所有现有 selector/host 方法对外签名与行为不变** → `registry.test.ts` /
> `reducer.test.ts` / 各 selector 测试 / 插件注册测试**全部不改、全部应保持绿**。这是"行为保持重构"的验证标准。

### Pack（新增）
`sdk/definePluginPack.ts` + 导出 + `definePluginPack.test.ts`。

### Dogfood / 迁移（取决于 L2 vs L3）
- **L2**：内置插件源码不改。可选做 1-2 个"示范性"pack（export / message-actions）证明 pack + 自定义点能力。
- **L3**：69 文件迁移到 `contribute(KERNEL.x, ...)`，分批（每批一类：themes / composer / message / tool / sidebar / workspace / overlays …），每批跑全套 check。

---

## 7. 分阶段实施（review 通过后）

- **P1**：底座 + `defineExtensionPoint` + 单测。（此时还没人用，纯新增）
- **P2**：`kernelPoints.ts` 声明 ~40 个点；把 registry 的 add/remove **改为 contribute 实现**，selector 改为 `useExtensionPoint` 封装。**31 个 host 方法签名不变**。跑全套 `check` —— **现有测试必须全绿**（行为保持）。
- **P3**：清掉 registryState 里塌缩后多余的 40 个 map 字段 + 工厂；确认 knip 无死代码。
- **P4**：`definePluginPack` + 单测。
- **P5**：示范 pack（export 用 lookup 模式、message-actions 用 useExtensionPoint hook 模式）—— 证明"插件定义自己的点 + 打包子插件"。
- **P6（L3，增量）**：按 domain 分批迁移内置源码到 `contribute(KERNEL.x, ...)`，每批 check 全绿；某 facade 的最后一个调用者迁走后再删它（knip 抓孤儿）。可随时停。
- **PP（权限，与 L2/L3 正交，可独立排期）**：见 §9。便宜的两件事（风险分级表 + sideload 强制 deny + Plugins pane 展示）可在 L2 之后单独成一两个 commit；install-time 同意 UI 等 T3 触发。
- **P7**：更新文档 —— CLAUDE.md（推翻旧不变量、记录新底座 + 权限模型）、`ARCHITECTURE.md §5.1 + §12`、`PLUGINS_IMPL.md` SDK 参考、`PLUGINS_CEILING.md`（T1-a ✅、T3 框架从"全推迟"改为"设计已定、分阶段建"）。
- 每个 P 独立 commit（why-focused），可单独 review/回退。

---

## 8. 决策点（全部已定）

1. **侵入级别** → ✅ **渐进式 L2 → L3**（先行为保持地塌底座，再按 domain 增量迁移源码、删 facade）。
2. **推翻旧不变量** → ✅ **接受**。P7 同步更新 CLAUDE.md 那条 ❌（"别抽 factory"）+ `ARCHITECTURE.md §12.2-E / §12.5`。
3. **`keyOf` / `normalizeKey` 放哪** → ✅ **放点定义里**（集中；shortcut 的 combo 归一化随点走）。
4. **命名** → ✅ **extensions 词汇**：`host.extensions.contribute` / `defineExtensionPoint` / `useExtensionPoint` / `lookupExtensionPoint` / `definePluginPack` / `sdk/kernelPoints.ts`。
5. **子插件 topoSort** → ✅ **不补**（靠 children 数组顺序，YAGNI）。
6. **示范 pack 范围** → ✅ **export（lookup 模式）+ message-actions（useExtensionPoint hook 模式）** 两个，覆盖两条消费路径。
7. **强制权限模型** → ✅ **纳入设计（见 §9）**。便宜部分（风险分级 + sideload 强制 deny + Plugins pane 展示）随底座一起做；同意 UI 等 T3 触发。

---

## 9. 权限模型（吸收 AionUi 的分级强权限）

> 调研 AionUi 后吸收的唯一一处设计：它的 `permissions` 是**一等公民、按 `safe/moderate/dangerous` 分级、后端强制、UI 展示 `granted`**。
> 这正是 Lyra `PLUGINS_CEILING.md` 标的 **T3 缺口**（Lyra 的 `capabilities` 目前**自愿、不强制**）。现在把它**设计进来**，分阶段建。

### 9.1 原则：capability 即权限，按信任来源分档强制

Lyra 的 `capabilities` 本就是权限单元（gate 24 个 Host namespace）。借鉴 AionUi 补三件事：**风险分级 + 来源分档强制 + UI 展示**。

- **built-in（first-party，可信）**：默认全权，**零摩擦**。声明 `capabilities` 仅作自律文档（即现状）。
- **sideload（第三方，不可信）**：**默认 deny** —— **必须**声明 `capabilities`，未声明的 namespace 一律 throwing proxy。
  即把现有 `restrictHost` 对 sideload **从「可选」变「强制」**：省略 capabilities ≠ 全权，而是 **deny-all**。

> ⚠️ 这是 Lyra **渲染端的 in-process 能力闸**，**不是后端 auth** —— 不违反"Runtime 零 user 概念"（那是后端协议层的事，见 `API.md §0`）。两件事不同层。

### 9.2 风险分级（映射 AionUi 的 safe / moderate / dangerous）

| 等级 | Lyra namespace | 理由 |
|---|---|---|
| **safe** | notify / log / i18n / state / config / storage / theme | 自身数据 / 纯展示，无外溢 |
| **moderate** | commands / layout / composer / sidebar / workspace / settings / message / tool / agui / window / tasks / **extensions** | 注册贡献 / 改 UI，影响面限本 app |
| **dangerous** | **rpc**（打后端 / 网络） / **plugins**（加载卸载任意插件 = 提权） / 未来 **fs**（文件读写） | 越权 / 外溢 / 可被武器化 |

- `extensions.contribute` 本身是 **moderate**（贡献数据）；危险性在于**消费方**拿数据去干什么（由 rpc / plugins / fs 这些 dangerous 权限把关）。**定义一个点不需要特殊权限。**
- ⚠️ **`definePluginPack` 的子插件加载靠 `plugins`（dangerous）** —— sideload 的 pack 想加载子插件必须显式声明 `plugins`，会被标红/需同意。built-in pack 不受影响。

### 9.3 落地（分阶段，T3 触发才全做）

| 子项 | 成本 | 何时做 |
|---|---|---|
| 给每个 capability 标 risk（一张表 + 类型 `Record<HostCapability, Risk>`） | 便宜 | **可随底座做** |
| sideload 入口对 `origin=sideload` **强制** `restrictHost`（无 capabilities → deny-all；built-in 不变） | 便宜 | **可随底座做**（关掉一个真实风险口子） |
| Plugins pane 展示每个插件的 capabilities + 聚合风险（safe/moderate/dangerous），仿 AionUi UI | 中 | 可随底座做 |
| **install-time 同意**：加载请求 dangerous 权限的第三方插件时弹窗 + 记 `granted` | 重 | **等第一个真实第三方 sideload（T3），现在 YAGNI** |
| 运行时按 `granted` 二次 gate（声明了 dangerous 但未授予 → 仍 deny） | 中 | 同上，等触发 |

### 9.4 与扩展点底座的关系

**正交**。底座统一的是「注册存储」；权限闸 gate 的是「Host 访问」。唯一接触点：新增的 `extensions` namespace 进 capability 表、标 **moderate**。
**权限模型不阻塞 L2/L3**，可独立成 `PP` 阶段推进。

---

## 10. 一句话总结

把"40 个 owned-map + 31 个 typed register"塌缩成**一个通用扩展点底座**（single|multi + keyOf + normalizeKey 三旋钮 +
typed 句柄保住推断），让 **kernel 自己的每个槽也只是底座上一个保留点**，第三方与 kernel 完全对等 —— 这才是"向 JetBrains 演进"的实质。
推荐 **L2（行为保持地塌底座、保留 typed facade、测试全绿）** 先落地，L3 全量源码迁移按需增量推进。
代价是 registry 核心重写 + 推翻一条旧不变量，但前提（"通用化必丢类型"）已被 typed 句柄证伪。
**权限模型**（吸收 AionUi 的分级强权限，§9）与底座正交：便宜的两件事（风险分级 + sideload 强制 deny）随底座一起做，install-time 同意 UI 等 T3 触发 —— 这把 ceiling 文档的 T3 从"全推迟"升级为"设计已定、分阶段建"。
