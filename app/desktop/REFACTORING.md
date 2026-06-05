# REFACTORING.md — Lyra 前端重构要求

> 重构清单:重构时**改什么、怎么改、按什么节奏**。
> 文档分工(尽量不重述):
> - [`CLAUDE.md`](CLAUDE.md):决策透镜 + 硬约定 + 反向不变量 —— "**能不能**这么写"。重构的**两档节奏**(§3)、**Fowler 清单**(§3)、**React effect 纪律**(§5)、**工作流**(§7)已在那里;本篇不复制,只补它没展开的镜头。
> - [`frontend/ARCHITECTURE.md`](frontend/ARCHITECTURE.md):系统长什么样(结构/运行)。
> - [`frontend/DESIGN.md`](frontend/DESIGN.md):视觉规范。
> - **本篇**:把上面的"判据"落成几面可执行的**重构镜头**(命名 / 派生态 / 边界吞空 / 作用域卫生 / 卫语句 / 就近组织 / 节奏)。
>
> 这套镜头来自一轮跨多个 feature 的系统性精修,**已全部用 TS/React 语境表达**——是工程通则的前端译法,不是搬运某种后端语言的写法。例子用 Lyra 自己的代码。

---

## 0. 总则

- **精修 ≠ 重写**:外科级、可逆、**在源头改对**,不在错的设计上叠补丁(呼应 CLAUDE.md 第一法则:现在改成本最低)。
- 参考业界(Claude Code / Codex / VS Code 扩展模型 …)**只取思想、不作命名锚**。
- 唯一允许背的"债"是"设计还没想清楚";**绝不允许"明知更好却为省事不改"**。

---

## 1. 命名 —— 名字必须和"它装什么 / 它做什么"一致

CLAUDE.md §3(d) 讲的是 `x`/`tmp`/`data` 不算名字;本面更进一步:**当一个名字在"撒谎"时就改它,而不是加注释解释**。

- **类型 / 接口名 == 它真正承载的东西**。一个类型名暗示 A、实际装 B(例:名叫 `XxxInfo` 实际是一组开关;名叫 `Result` 实际是某域的判定),直接**改名**,不靠注释打补丁。
- **字段名 == wire / 序列化键**。协议 `shapes.ts`、Zod schema、store 持久化形状里,字段名和 JSON key 应一致;不一致时**优先改字段名**,而非将就 key。
- **组件 / hook / store-action / 事件 handler 名 == 它的行为**。`onX` 必须真的在 X 时触发;`useXxx` 名要反映它读/做的事。
- 消除 **stutter**(`ChatChatPanel` / `useChatChatStore` / `chat/chat.ts`)。
- **文件名描述内容**。禁把 `utils.ts` / `helpers.ts` / `index.ts` 当杂物堆;按内容命名(仓库里 `toolRouting.ts` / `streamReveal.ts` / `runDigest.ts` 是好例子)。**同目录文件重命名不改公开面**,低风险。

---

## 2. 注释

- 只解释 **why**;删 what-注释、过期迁移注释、误导性的"为什么"(实际已不再这样)。CLAUDE.md §3(e) 已列。
- **改名 / 重构后同步清理所有引用**——doc-comment、`@/...` 链接、示例代码里指向旧名/旧文件的都要跟着改,不留陈旧指向。

---

## 3. 值语义 / 引用稳定 / 不存派生态

前端最容易踩的"状态该怎么放"三件事:

- **不存派生状态**。凡能由现有 store / props / query 算出来的,**不要再存一份**到 Zustand 或 `useState`——派生走 **selector / `useMemo` / 渲染期计算**(范式:`runDigest` 从 timeline+toolCalls 派生、各 `useXxx` selector)。存了就要"改一处连改 N 处",且天然会不同步。
- **引用稳定**。传给 `memo` 子组件、或进 effect/`useMemo` deps 的 object / array / function 要保持**稳定身份**;身份每帧新建 = memo 白做 + effect 反复触发(CLAUDE.md §5 的 selector 风暴正是此类)。能传 primitive 就别传新建对象。
- **可选 vs 必填要诚实**。"缺失"用 `field?: T` / `T | undefined` 表达,**别拿 sentinel**(`-1` / `""` / `{}` 假装"没有");必填的就标必填,别让调用方猜哪些可能是空。

---

## 4. 让边界自己吞掉"空 / 加载 / undefined",别把判空推给每个调用点

一个数据可能未就绪时,**应由提供它的组件/hook 自己**给出 loading / empty / 安全默认,消费者直接用——而不是每个调用点都先 `if (!data)`:

- 三态(loading / empty / error)**收口到一个组件**:你们的 `<DataView>` render-prop 就是范式,新增列表/卡片优先复用它,而非各自重写三态分支。
- 一个 hook / selector 在数据未到时返回**安全空值**(空数组、占位对象),让渲染端无脑 `.map`。
- **UI 1:1 镜像 store,绝不在 async query 上 join 后 filter**(CLAUDE.md §5 踩过:filter 会让 store 已确认的 id 在 DOM 消失,用户报"点了没反应")。query 没 ready 用 placeholder。
- 只在**真正会缺失**的边界加这层吞咽(数据未就绪 / 可选 prop / 外部输入);内部已确定非空的地方再加判空就是噪音。

---

## 5. 作用域卫生 —— helper 与它的所有者就近,别污染共享面

- 只被**一个**组件/模块用的 helper:**就近放在它旁边**(模块私有,**不从 barrel 导出、不丢进全局 `lib/utils.ts`**)。
- 与某个类型/数据强相关的纯函数,**和它的类型定义 / owner 放一起**,而不是散落到通用 util。
- **共享面越小越好**:barrel(`index.ts`)只导出**真正跨模块消费**的符号;模块私有的不上 barrel。你们 `components/chat/{message,panel,composer}` 各有 barrel 唯一公开面、`lib/` 是"明确跨插件共享"的语义——就是这条的体现。把"只在内部用"的东西暴露到共享面,等于无谓扩大表面积 + 心智负担。

---

## 6. 卫语句 / 降低分支复杂度

- 早返代替嵌套 `if`(CLAUDE.md §3b);查表 `Record<K,V>` 代替 3+ 条件链 / 嵌套三元(§3c)。
- **JSX 里**:loading / empty **早返**,别在一个 `return` 里堆三层三元;分支一多就抽子组件,让主体扁平。
- 但 `for await (ev of stream)` / reducer 派发这类**流式/事件驱动的嵌套**是结构性的,不是逻辑复杂度——别为"看起来浅"硬平铺。

---

## 7. 现代 JS / TS —— 判据照 CLAUDE.md §3

不重复。要点:现代特性(`.at(-1)` / `findLast` / `Object.groupBy` / `structuredClone` …)**只在"可读性/正确性↑ 且 不引入分配/不退化热路径"时采用**;`toReversed()`/`toSorted()` 会整组复制,在 reducer / 每 token 调的函数里就是性能回归。**地道 ≠ 最新**——CLAUDE.md §3 的三连判据 + 反例已经很完整,照它走。

---

## 8. 就近原则 / 组织 —— 相关放一起、公共下沉、按职责拆

- **相关代码放一起**(按 feature / 限界上下文分目录,你们 `builtin/<domain>/` 即是);**公共的下沉**到 `lib/` / `components/common/` / SDK,不让多个地方各抄一份。
- **god-file 按职责拆**成**同目录多文件**(纯移动,公开面不变):范式有 ChatPanel 196→51 + 拆 `PanelHeader`/`ChatStream`、core-reducer 拆 `handlers`/`projections`/`fold`、`sdk/types.ts` 1045→12 个 domain 文件。
- **大但内聚、单一职责的不拆**。拆的判据是"**能真正切断耦合且不破坏公开面**",不是单纯按行数;拆了反破坏内聚的就保留(同 ARCHITECTURE §3.2 不拆 monorepo、不强抽 `application/` 层的逻辑)。
- 拆完 **barrel 作为目录唯一公开面**,对外形状不变,消费方零改动。

---

## 9. 工程硬规则(SE 层,补 CLAUDE.md §4 的机械规则之外)

- **工厂 / 构造类函数返回"有效值或抛错"**,不返回半成品(别让调用方拿到一个字段缺失、需要它自己补全校验的对象)。
- **死代码立刻删**(`npx knip` 扫未引用的 export/文件/依赖),不留"将来可能用";禁推测性占位(`// TODO: 以后接` / 空 stub)。
- 重构途中发现的**真实 bug 顺手修,但单独成 commit**,与纯重构分开,便于独立 revert 与 review。
- **边界校验用 Zod、内部数据流不加**(CLAUDE.md §3):重构时别顺手给内部流加 Zod,也别把边界校验拆掉。

---

## 10. 节奏与纪律 —— 详见 CLAUDE.md §3「重构」+ §7「工作流」

那两节已给全(两档节奏、先给 3 项候选 + 权衡、等确认再动、每批一 commit、`npm run check` 全绿、commit 写 why)。本篇只补两条精修中反复验证、值得单列的:

- **破坏性改动先算"爆炸半径"**:改一个 SDK 公开形状 / store schema / 协议 `shapes` / 组件 props 之前,先 grep 出**所有消费方**(跨插件、跨层、跨 selector),把影响面列清再动手;碰 **Host 接口 / spec 形状**的改动按 ARCHITECTURE §10 **bump `apiVersion`**。store schema 变了 bump `version` 丢旧数据(无 migration)。
- **承认审计误报**:深入看后发现某条"坏味道"其实是**有意设计**(流式结构性嵌套、故意的非 Radix 豁免、内聚的大文件、`extensions` 单底座而非 per-slot map …),就 **skip 并在结论里写明理由**——这是正常的 false positive,不是失败,也别为了"清干净"去动正确性敏感的热路径(plugin loader / registry / reducer)。

---

## 一句话

设计前问"**该不该**"(CLAUDE.md 决策透镜);重构时对照本篇的镜头定"**改什么、怎么改**";每批一个可独立 revert 的 commit、`npm run check` 全绿再推。**镜头是工程通则,例子是 Lyra 的。**
