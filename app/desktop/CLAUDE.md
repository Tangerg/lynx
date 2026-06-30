# CLAUDE.md — project context for Claude Code

> **Lyra** — Wails 桌面应用（Go 壳 + React/TS 前端），由自研 **Lyra Runtime Protocol v2**（JSON-RPC，Session→Run→Item 流式）驱动的插件化 agent client。
> 结构看 `frontend/ARCHITECTURE.md`，视觉规范看 `frontend/DESIGN.md`，桌面质感防回归清单看 `frontend/DESKTOP_UI_POLISH.md`，协议看 `docs/protocol/`。
>
> 本文件只放**法则 —— 只宏观、不写具体**（具体文件名 / 符号 / 版本 / 行数 / 历史会随演化变动，活在代码 / git / ARCHITECTURE.md 里，不进本则）。读法：先「两条法则」（总透镜）→ §1 架构心智 → §2-§5 写代码的判断与硬约定 → §6 别走的方向 → §7 怎么干活。

---

## 第一法则 —— 绝不为一时方便留历史债务

> **最高优先级，凌驾于本文件其余所有约定之上。**

项目处于快速开发 / 试错阶段，没有历史债务、也没有外部兼容包袱 —— 协议 wire shape、暴露的类型 / 组件契约 / store shape、命名，全都可以调整。正因如此：

- ❌ **绝不为"少改几处 / 降低迁移 / 赶进度"留下任何历史债务** —— 迁就外部库或旧名字的命名、为兼容留的字段、推测性 shim、"以后再清"的 TODO，一律不留。
- ✅ **发现设计不对，就在源头改对**，不在错的设计上叠补丁。**现在改成本最低，往后只会更贵。**
- 命名 / shape 按**本质第一性**决定；参考业界**只取思想、不作命名锚** —— 名字恰好相同，只因它在独立评估下最优，不为兼容或省迁移。
- **唯一允许背的"债"是"设计还没想清楚"本身**；绝不允许"明知更好、却为省事不改"。

## 第二法则 —— 修理问题必须治本，绝不治标

> **与第一法则并列的最高优先级。** 第一法则管"别欠债"，第二法则管"修就修到根"。

修任何 bug / 问题，都在它的**根因和正确的层**上修，绝不在症状点打补丁、绝不 hacky。一个 workaround 让现象消失但根因还在 —— 那不是修复，是把债藏进"看起来修好了"的假象里，下一个相邻 bug 会从同一根因再冒出来。

- ❌ **绝不治标**：症状点加 if 绕过、只在消费侧 workaround 而留源头不动、"加行日志 / 别吞错误就算修"、reactive 地兜底去掩盖上游的错误状态、把不变量交给"调用方记得别犯错"。
- ✅ **治本**：找到根因落在哪一层（组件 / store / SDK 形状 / 协议契约），就在那一层改对。根因常在更底层，治本往往要动公开形状 —— 先按"破坏性改动先算爆炸半径 + 咨询"走，但默认倾向治本、而非在上层将就。
- **判据一句话**：问"根因消除了吗，还是只是这个现象不出现了？"—— 只让现象消失的，是治标，打回重修。**（注意区分真 bug 与 dev-only HMR 假象，见 §5）**

---

## 1 · 架构心智模型

- **一句话定位**：**Kernel 不长肉，所有功能都是插件。** 路由 / 布局 / 内容渲染 / 命令 / 快捷键 / 主题 / 运行时事件处理 / 设置面板，全部由内置插件贡献；Kernel 自己只是一组命名 Slot + 几个共享 store。
- **三大支柱**：
  1. **插件系统**：一个开放扩展点底座 —— 每个贡献面是一个 typed ExtensionPoint，所有贡献（内置与第三方）走同一条 `contribute` 写路径、selector 读路径。Host 上只留少量薄 facade + 命令式动作。
  2. **协议 fold 层**：reducer 是**纯派发器**，把 wire 的 StreamEvent 路由到注册的 handler chain；所有协议语义（Item→message/block 投影、HITL）都在 agent 插件里（handlers 派发 / projections 纯映射 / fold 有状态折叠）。wire 层独立于 fold 层。
  3. **状态分层**：几个小 Zustand store 各司其职（每会话 ephemeral / tab·draft / UI 偏好 / 握手能力 / 后台任务 / 编辑器），无 context 链。store schema 变了 bump `version` 丢旧数据，不写 migration。
- **结构细节**（目录树 / 各模块职责 / composition root）见 `frontend/ARCHITECTURE.md`。Go 侧只是 Wails 壳、不内嵌 runtime；前端经 HTTP 连外部 Lyra Runtime。

---

## 2 · 技术栈（选择已定，别轻易换 —— 反向不变量见 §6.1）

- **UI**：React + TypeScript。
- **样式**：Tailwind（utility-first）+ `cva` / `cn()`；**所有新代码用 utility class，不写新 .css 文件**，全局样式只剩一个 `globals.css`。
- **Headless 基件**：**Radix Primitives first**（带交互 / 焦点 / 键盘 / aria 的一律先用 Radix，没有的才自写）。借鉴 shadcn 的 className 字符串可以，但不引其 npm 包。
- **状态 / 数据 / 路由**：Zustand（多小 store）/ TanStack React Query / TanStack Router。
- **协议**：自研 Lyra Runtime Protocol v2（JSON-RPC 2.0，已弃用 AG-UI），见 `docs/protocol/`。
- **桌面壳**：Wails。**测试**：Vitest + Testing Library。**构建 / 质检**：VoidZero 栈（Vite + Rolldown / Vitest / OxLint）+ prettier + knip。
- 具体库（命令面板 / Toast / 图标 / 高亮 / i18n / 动画 等）见 `package.json` 与 §3「不重复造轮子」。

---

## 3 · 设计原则（怎么判断）

判断标准，不是机械规则 —— 拿不准时回到这里。

- **KISS / SOLID / YAGNI / DRY / 高内聚低耦合** 是总判据。不为不存在的需求做抽象；组件按职责拆（SRP）；**抽象只在 3+ 重复时引入**；模块间通过最小接口（plugin SDK / store selector）通信。
- **用户体验细节是一等公民，不是"以后再说"**。功能正确只是底线；拉开观感的是那些不影响功能、却天天硌用户的细节。做任何 UI 都按**打磨后的终态**交付，每次至少扫：尺寸稳定（不随内容长度跳动，长文本截断 + 兜底，绝不靠换行撑高度）、行高 / 间距节奏一致、字号 / 字重 / 颜色的层级对比、对齐与数字（同列对齐、数字 `tabular-nums`）、截断 / 空态 / hover 的可达性。**判据**：改完自己当用户走一遍——"长内容会炸吗？切窄会挤吗？层级一眼分得清吗？尺寸会跳吗？" 宁可多花一轮打磨，也不交"功能对、但硌手"的 UI。`DESIGN.md` 给规范，这条给**态度**。
- **桌面质感不是网页质感**。做视觉调整前先读 `frontend/DESKTOP_UI_POLISH.md`：surface 层级用 edge ring + contact + ambient 的阴影模型进主题 token；去网页味靠原生桌面行为约束（少 cursor-pointer、少泛 hover、少玻璃 blur、少硬 border），不是靠更大圆角或更重阴影。
- **零 legacy 兼容代码 / 迁移路径**（开发阶段）。store schema 变了 bump version 丢旧数据；注释不写 "Legacy …"。
- **不重复造轮子**：写工具函数前先查 npm，沿用已选定的栈（§2）。例外：极小封装、应用专属业务逻辑、对现有库的薄包装加 hook。一定要手写就在注释说明社区库为何不够用。
- **边界校验用 Zod，内部数据流不加**：信任边界（外部 spec / wire envelope / 用户输入）必须用 Zod 拒脏数据；内部流（store / props / 类型守护的调用）不加 —— TS 已够，再加是噪音 + bundle 浪费。
- **现代特性只在"可读性 / 正确性提升 _且_ 不引入分配 / 不退化热路径"时采用**。判据三连：真有等价老写法可替换、不引入额外分配 / 不退化热路径（复制整组的 `toReversed`/`toSorted` 在 reducer / 每 token 调的函数里就是回归）、类型与行为完全保留。**地道 ≠ 最新。**
- **重构是节奏，不是可选项**，分两档（完整镜头清单见 [`REFACTORING.md`](REFACTORING.md)）：
  - **小型（每几轮 feature）**：聚焦最近改动的文件，扫超长文件 / 局部 3+ 重复 / 死注释 / 命名漂移 / 新增 API 破坏既有抽象 —— 产出小、直接做完跑全绿 commit。
  - **大型（每十几~二十轮）**：跨整个 `frontend/src/` 扫（`knip` 找未引用 export/文件/依赖、超大文件拆 SRP、跨模块重复、架构层级）—— 产出 multi-batch 计划，用户确认后逐批 commit、每批全绿。
  - **目的**：小型防局部熵增，大型防架构熵增。信号：单文件过大、同一不变量手写 > 3 处未收敛、新建插件要改 kernel 多处、命名漂移、反复改同一段代码（抽象方向错了）。
- **注释纪律：轻易不要写注释** —— 优先让代码通过命名 / 结构 / 抽象自解释。注释是表达力用尽后的最后手段，只在以下情况写（写代码无法表达的）：① 公开 API / 扩展点 / hook 的契约（语义 / 参数 / 返回 / 异常 / 副作用 / 调用约束）；② 特殊约定（业务规则 / 历史原因 / 协议条款 / 后端行为依赖）；③ 特殊算法（思路 / 边界 / 复杂度 / 非显然优化）；④ 反直觉实现（为什么不能用更简单写法，防"好心"改回去）；⑤ 并发 / 安全 / 信任边界等违反不报编译错只在生产炸的约束。**判据**：注释只写 _why_ 与_约束_，不写 _what_ 与 _how_；改代码必同步改注释，宁可删不留过期；对下一个读者说话，不对 reviewer 说话。

---

## 4 · 硬约定（违反 = 回归）

机械规则，不需再判断，照做：

- **Tailwind first**：组件样式用 utility class；`style={{}}` 内联只在 token 值真动态时用。
- **不写新 .css 文件**：新样式进 className，`globals.css` 是唯一例外。
- **Radix first（硬规则）**：任何带交互 / 焦点 / 键盘 / aria 的组件一律先用 Radix，在 `common/` 下薄包一层套设计 token，**绝不手写 focus trap / roving tabindex / aria-\* / 键盘事件**。唯一豁免（须注释写明理由）：纯展示无交互、Radix 版有实测开销且无 a11y 收益、定制行为 Radix 模型套不进 —— 判据是"Radix 是否带来真实 a11y / 行为收益"，纯为统一不换。
- **No cheap lines**（DESIGN.md + DESKTOP_UI_POLISH.md）：区域分隔优先用 surface ladder / shadow hairline，不用灰色硬边线堆卡片；真正的结构边界、focus ring、语义状态线例外。
- **Cards-on-canvas + Grid/Flex first**：暗 canvas 上浮 surface 卡片；超过两个元素的排版用 Grid / Flex，不用 `position: absolute` 手算坐标 / `<table>` 排版 / 连串 margin 凑对齐（absolute 只用于浮层与锚点）；隐式 / 单列 grid 显式写 `minmax(0,1fr)` 防宽 child 撑爆。
- **Plugin 一定走 registry**：不直接 import 一个 builtin plugin，永远走 selector。
- **运行时事件单向**：render 路径不回写 agent store；要"做事"调 store 上的 send / stop / resume。
- **components 不直连后端**：`components/` / `pages/` 只经 store selector / query hook / SDK selector 触业务，**不得 import composition root 或协议客户端**（已有 layer 守卫强制）。
- **Disposable 由 Host 收集**：不手动调 `dispose()`。
- **加文档先问**：不主动建 `*.md`，除非用户明确要。

---

## 5 · React effect 纪律（写组件 / hook 时必看）

perf 排查沉淀的硬规则 —— 几个"看似没事其实在累积"的坑：

- **State 读：handler 里走 `getState()`，render top-level 不订阅**（除非那个值真要驱动渲染）。每条消息 top-level 订阅 = 列表 × token 流 = 上千次 selector 评估/秒。
- **模块级 `subscribe(...)` 必须配 HMR 清理**（`import.meta.hot.dispose`）—— 否则每次热重载叠一个 listener，dev 越用越卡。
- **热路径不 `console.log` 完整对象** —— 高频流会让 DevTools 留引用、长会话累积无法 GC。
- **写入数组前去重**：无脑 push 会让事件二次触发时同 id 共存 → React duplicate-key warning loop 拖死帧率。每个 push handler 先 findById（upsert）。
- **UI shell 1:1 镜像 store，绝不在 async query 上 join 后 filter** —— filter 会让 store 已确认的 id 在 DOM 消失，用户报"点了没反应"。query 没 ready 用 placeholder。
- **先判定是不是 dev-only HMR 假象，再决定要不要修 —— 很多根本不用修**。只在 dev 出现、整页刷新即消失、生产不可能发生的，几乎都是 HMR 把模块级副作用重跑 / 叠加的假象。**别用生产代码去"修"它**（给正确性敏感的热路径——plugin loader / registry / reducer——加绕过逻辑反而更糟）；真要消除 dev 摩擦用纯 dev 手段（`import.meta.hot` 清理 / accept-reload）。

---

## 6 · 强反向不变量（已知错的方向，别再提）

### 6.1 架构 / 技术栈

- ❌ 把 Zustand 换 Redux/Jotai/Effector、React Query 换 SWR/RTK Query、Wails 换 Tauri、Radix 换 Base UI、或切换前端框架（Vue/Solid/Svelte…）—— 都评估过，切框架 zero-feature 期 + 生态损失换不来收益。
- ❌ 给内部数据流加 Zod —— 只用在信任边界（§3）。
- ❌ 引入 CSS-in-JS / 退回手写 CSS、引入完整 UI Kit（shadcn-as-npm / HeroUI / DaisyUI …）—— 跟设计语言打架。
- ❌ 把贡献面退回 per-slot 的 `addX/removeX` map —— 已塌进单一 `extensions` 底座；加贡献面 = 定义一个 ExtensionPoint + 一个 selector，不动 registry。
- ❌ 把分层模块拆成 monorepo、把 VoidZero 栈（OxLint/Vite-Rolldown）退回 ESLint/Rollup —— 触发条件没命中 / 是退步。

### 6.2 协议 / 后端边界

- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**：Lyra Runtime 是无状态纯计算单元，协议层零 user 概念；鉴权由更外层（OS 信任 / 本地门禁 token / 未来 facade）解决。
- ❌ **给 LLM provider 加 OAuth / token refresh / 订阅检测**：用户填 API key、存 keychain、401 让 UI 提示重填。
- ❌ **把"远程后端 / 团队 server / 云端订阅"当部署形态**：那是未来 facade 层的事，Runtime 协议永不感知 facade（同一份代码跑桌面也跑服务器）。
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）：走 `context.Context` 或 HTTP header，永不进 message body。
- ❌ **协议 wire 用 REST + verb / 状态码**：是 JSON-RPC 2.0 envelope（参考 MCP），HTTP 只是其中一种 transport；method 名照搬 method 表、点保留（不斜杠化）；不加 RESTy read-only shadow（sidecar 只限 info / health）；业务 error 走 `error.code`，不映射 HTTP status。
- 协议细节见 `docs/protocol/API.md` + `docs/protocol/TRANSPORT.md`。

---

## 7 · 工作流

- **开发**：`wails dev`（自动起 vite + Go backend）。
- **质量门禁**（在 `frontend/` 跑）：`npm run check` —— typecheck + lint + format + test + knip + circular + layers + bundle，全绿才往下走（单项也可单跑 `typecheck` / `lint` / `test` / `knip`）。
- **会漂的量（测试数 / 插件数 / 文件数）直接跑命令查，不在本文件维护硬编码数字。**
- **沟通约定**：中文回复（用户偏好），代码 / 注释保持英文；破坏性或结构性改动前先算爆炸半径（grep 所有消费方）+ 给方案 + 权衡，等用户确认再动；改动后跑 `npm run check`，commit message 写清 _why_，commit 后默认推送；commit trailer 用 `Co-Authored-By: Claude <当前实际模型名> <noreply@anthropic.com>`（署名以实际生成该 commit 的模型为准，不硬编码型号）。
