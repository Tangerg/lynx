# Lyra 前端实现细节 Review

日期：2026-05-26

范围：`frontend/src` 实现细节，包括命名、注释、局部代码组织、样式约定、测试与可维护性。已先阅读根目录 `CLAUDE.md`，以下建议按该文件约定执行。

## 1. 总体评价

实现质量总体较高：代码普遍使用清晰的函数边界，核心模块有测试，注释大多解释 why，插件 registry 和 AG-UI reducer 的复杂度被控制在可理解范围内。最近一轮调整也体现了较好的工程判断：没有引入大型新框架，而是在现有插件内核里补可观测性、批处理、索引和边界校验。

但实现层仍有几类细节债务：

- 新旧样式约定混在一起：大量 inline style、border utility、旧 CSS hook 与 `CLAUDE.md` 的 Tailwind/no-lines 约定冲突。
- 注释数量偏多，部分注释已经从“解释原因”滑向“复述代码”。
- Diagnostics 的命名和数据语义有几处不精确。
- 个别文件保留 TODO 和旧式实现，容易成为后续复制模板。
- 测试覆盖了核心 registry/loader，但新增 metrics/diagnostics 的行为测试不足。

## 2. 命名 Review

### 2.1 好的命名

- `applyEvents`：准确表达批量 fold 多个 AG-UI events。
- `createIndex`：比直接写多个 `Map` 缓存更泛化，但没有过度抽象。
- `withSchema`：清楚表达“用 schema 包 handler”的边界语义。
- `restrictHost` / `createDenyProxy`：行为明确，错误信息也能指导插件作者补 capabilities。
- `layoutBySlot`, `customByName`, `coreByType`：索引意图非常直接。

### 2.2 建议改的命名

1. `MetricRow.last`

   在 histogram 场景里，`last` 实际写的是 `sum / count`，DiagnosticsView 里也显示为 `avg`。这不是 last seen value。建议改名为 `avg` 或 `mean`，counter 场景如果需要当前累计值则用 `value`。

2. `measureMarkdownParse`

   当前测的是 `remend(display)`，不是完整 Markdown parse/render。名字容易误导。建议改为 `measureMarkdownRepair`、`measureRemend` 或 `measureMarkdownPreprocess`。

3. `DiagnosticsExporter`

   名字本身可以，但如果未来还会有 remote exporter，建议更明确为 `InMemoryDiagnosticsExporter`。

4. `EXPORT_INTERVAL_MS`

   当前常量位置在 diagnostics plugin 内，含义是 OpenTelemetry reader export interval。建议命名为 `METRIC_EXPORT_INTERVAL_MS`，避免和通用导出动作混淆。

5. `HTML_DENY`

   实际是 tag deny list。建议 `DENIED_HTML_TAGS`，语义更准确。

## 3. 注释 Review

### 3.1 做得好的注释

以下注释是有价值的：

- `main.tsx` 解释为什么暂时关闭 StrictMode。
- `selectors.ts` 解释 selector 返回 raw Map + useMemo 的原因。
- `useAgentSession.ts` 解释 rAF batch 对 streaming delta 的影响。
- `schemas.ts` 解释 Zod 只用于信任边界。
- `host.ts` 解释 runtime seam 避免 circular import。

这些注释都在解释架构约束或踩坑原因，值得保留。

### 3.2 应减少的注释类型

1. 复述代码行为的注释偏多。

   例如一些 `register*`、`dispose`、`return null` 附近的注释，在类型和函数名已经足够清晰时可以删减。当前文件长度不算失控，但 `host.ts`、`selectors.ts`、`definePlugin.ts` 的注释密度偏高，后续修改时应主动删除过期解释。

2. TODO 不符合当前“开发阶段不留未来兼容包袱”的风格。

   当前仍有：

   - `plugins/builtin/workspace-views/plan.tsx`
   - `plugins/builtin/workspace-views/terminal.tsx`

   如果短期不做，建议改成 issue/文档任务；如果保留在代码里，应写清楚触发条件，不要作为常驻注释。

3. Diagnostics 注释和实际行为有轻微偏差。

   `diagnostics/index.tsx` 强调 SDK chunk “only loads when this plugin's setup() runs”，但该插件现在作为 builtin eager setup，启动后就会运行。建议改注释，避免读者误以为打开 Diagnostics view 才会加载 SDK。

## 4. 样式与 UI 约定 Review

### 4.1 Tailwind first 尚未完全落地

`CLAUDE.md` 明确写了“所有新代码必须用 Tailwind utility class，不再写新的 .css 文件”。当前仍有不少 inline style，部分是合理动态 token，部分可以改掉。

优先处理这些旧式实现：

- `plugins/builtin/workspace-views/notifications.tsx`：`NotificationRow` 几乎全是 inline style，可改为 Tailwind className。
- `plugins/builtin/workspace-views/terminal.tsx`：多个 `style={{ color, margin }}` 可改 Tailwind arbitrary values。
- `plugins/builtin/workspace-views/diff.tsx`：颜色和间距 inline style 可替换。
- `components/chat/SearchResults.tsx`：`-webkit-line-clamp` 可用 Tailwind line-clamp utility 或局部 class 统一处理。

动态 swatch 这类 `style={{ background: value }}` 是合理例外，不需要强行改。

### 4.2 No-lines 原则需要重新定界

仓库里大量 `border border-line`、`border-b`、`border-t`，与 `CLAUDE.md` 的 “No lines anywhere” 原则不完全一致。考虑到这些不是本轮新增，建议不要一次性大改，但要明确分层：

- 允许：focus ring、active accent line、输入控件状态、真正的 data table 行列辅助。
- 收敛：普通卡片、panel、message block、tool card 的硬边线，逐步改为 surface ladder + shadow。
- 禁止复制：新组件不要继续以 `border border-line` 作为默认容器风格。

### 4.3 Radix first 仍有空间

已有 Popover、Tooltip、ContextMenu 等 Radix 用法。需要注意：

- `MermaidBlock` 的 zoom lightbox 当前手写 dialog 行为，只处理 Escape 和点击关闭。按 Radix first，后续应考虑用 Radix Dialog，获得 focus trap、aria、scroll lock。
- Diagnostics view 是开发工具，简单实现可以接受；如果后续加 filter/select/tabs，优先 Radix。

## 5. 代码结构 Review

### 5.1 `host.ts`

优点：

- namespace 边界清晰，register 返回 Disposable 的模式稳定。
- `restrictHost` 简洁，错误信息具体。
- `CONSOLE_METHOD` 查表替代条件链，符合约定。

建议：

- 文件 518 行，仍可接受，但 Host namespace 继续增长时应拆成 `createHostNamespaces/*`，否则每加一个 surface 都会碰同一个大文件。
- `capabilities` 当前按 namespace 粒度控制，后续如果 sideload 要做真实权限，可能需要 method-level 或 high-risk action 细分，例如 `plugins.load` 与 `plugins.list` 风险不同。

### 5.2 `selectors.ts`

优点：

- `createIndex` 与 WeakMap 设计贴合 registry mutation 模型。
- selector 返回 Map、组件侧 useMemo 派生，避免了 useSyncExternalStore 快照不稳定问题。

建议：

- `createIndex` 返回数组是原数组引用，`useLayoutSlot` 再复制排序是正确的。建议在注释里强调 callers 不应 mutate index 返回的数组，或在 helper 内统一返回 readonly。
- `useDeclaredMerged` 的 `declaredToReal` 是函数参数，会被作为 useMemo dep。当前传入的是模块级函数，没问题；后续不要传 inline lambda。

### 5.3 `lib/metrics.ts` 与 diagnostics

优点：

- call site 很薄，避免让业务代码直接接触 OpenTelemetry API。
- 属性 cardinality 做了 length bucket，方向正确。

建议：

- 给 metrics/diagnostics 加测试，至少覆盖 store ingest 的 histogram/counter 行为、`clear()`、percentile fallback。
- `measurePluginLoad` 只在 setup 成功后记录。失败 plugin 的 setup duration 当前没有记录；建议用 `finally` 记录并带 `result=loaded|failed|skipped` 属性，方便排查坏插件。
- `pluginName` 作为 metric attribute cardinality 可能随 sideload 插件增多而扩大。dev-only 可以接受；若生产开启，应限制或 bucket。

### 5.4 `useAgentSession.ts`

优点：

- rAF batch 实现简单，避免了引入复杂 stream scheduler。
- cleanup 会 cancelAnimationFrame、unsubscribe、abortRun。

建议：

- 现在所有事件都进 rAF queue，包括 `RUN_ERROR` / `RUN_FINISHED`。一帧延迟通常可接受，但如果后续有 permission/approval 这类交互事件，建议保留 immediate path。
- `send` 逻辑在 `setSend` 和 return object 中重复。可以抽一个 `sendText(agent, text)` 小 helper，降低未来行为漂移。

### 5.5 `MarkdownMessage` / artifact 渲染

优点：

- `rehypeRaw` 后有 tag deny list，HTML artifact iframe sandbox 也做了 opaque origin。
- Shiki/Mermaid 都有 debounce 和 fallback。

建议：

- `HTML_DENY` 是 deny list，安全边界不够强。建议后续统一 `contentSecurity` 模块，至少处理 URL scheme，例如 `javascript:` href。
- `ShadowStyleBlock` 用 `shadow.innerHTML = <style>${css}</style>`，虽然隔离了 host stylesheet，但仍应纳入统一 sanitizer 策略。
- `dangerouslySetInnerHTML` 的来源应集中列清：Shiki、Mermaid、HTML artifact、Shadow style，给每处写明是否 sanitize、是否可信。

## 6. 测试 Review

当前测试基线健康：220 个用例通过，覆盖了 registry、plugin loading、lazy activation、architecture rules、settings、storage、smoothText、Markdown 等。

建议补的测试：

1. Diagnostics store/exporter：
   - histogram ingest 后 p50/p95/sum/count 正确。
   - counter ingest 后 value/avg 命名调整后语义正确。
   - `clear()` 不会在下一次 cumulative export 前产生假数据。

2. Metrics wrapper：
   - `measureReduce` 在 fn throw 时仍记录并 rethrow。
   - `measurePluginLoad` 失败场景是否记录，取决于后续实现。

3. rAF batching：
   - 多个 onEvent 在同一 frame 内只调用一次 `applyEvents`。
   - cleanup 后 queue 不再 flush。

4. capability：
   - sideload schema + capabilities 联动。
   - omitted capabilities 对 builtin/full trust 与 sideload/default deny 的差异。

## 7. 具体可执行清单

P0：

1. 修正 Diagnostics eager/按需语义：要么加开关，要么改成真正打开 view 后启用。
2. 把 `MetricRow.last` 改为 `avg`/`value`，同步 DiagnosticsView 表头和注释。
3. 清理 `notifications.tsx` 的 inline style，改为 Tailwind className。

P1：

1. 给 diagnostics store/exporter 增加单元测试。
2. 抽 `sendText(agent, text)`，消除 `useAgentSession` 重复。
3. 给 `contentSecurity` 建最小 URL sanitizer，先覆盖 Markdown link href。
4. 同步 `CLAUDE.md` / `ARCHITECTURE.md` 中的文件数量、插件列表和测试数。

P2：

1. 逐步收敛 `border border-line` 默认容器风格。
2. Mermaid lightbox 迁到 Radix Dialog。
3. Host 文件继续增长时拆 namespace factory。

## 8. 不建议改的细节

- 不建议把 `createIndex` 提前做成复杂 registry database。当前 WeakMap 索引足够。
- 不建议把所有 inline style 一刀切删除，动态 token swatch 和测量尺寸仍可保留。
- 不建议为了“少注释”删除关键 why 注释，尤其是 StrictMode、selector snapshot、rAF batching、Zod boundary 这些踩坑说明。
