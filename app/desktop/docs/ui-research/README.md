# UI 深研档案 · 对照同类 AI Agent 前端的差距清单

> 本目录是一份**研究档案**，不是实现承诺。它把桌面上 8 个同类 AI Agent 仓库的前端 UI
> 拆开深究后，整理成"**他们有、Lyra 没有（或做得薄）**"的清单，供后续按 CLAUDE.md §7.3
> 的节奏（先扫现状 → A/B/C 方案 + 权衡 → 用户确认 → 逐批落地）取用。
>
> **落地前不动代码。** 这里只负责"看清差距 + 指明去哪里抄思想"。

---

## 怎么读这组文档

| 文件 | 作用 |
|---|---|
| `README.md`（本文件） | 索引 · 核心判断 · 优先级矩阵 · 研究方法 |
| [`GAP-CATALOG.md`](GAP-CATALOG.md) | **核心**：逐条差距清单（P0/P1/P2），每条带"谁在做+file:line / Lyra 现状+file:line / 为何重要 / 落地方向 / 工作量·风险"。附 spec 漂移表、死占位符表、别抄的反模式、Lyra 已赢项。 |
| [`PEER-NOTES.md`](PEER-NOTES.md) | 8 个对照仓库的速查卡：技术栈 + 最值得偷的 3–5 点 + 关键文件路径，方便落地时直接去读源码。 |

---

## 核心判断（thesis）

**Lyra 的"对话"已是第一梯队，但"工作台"还没建完。**

- **对话层**（流式渲染、RPC-log 工具卡、可编辑参数的审批卡、消息操作矩阵、主题系统）
  对标甚至**超过** OpenHands / cline / opencode / cherry-studio / lobe-chat。
- **外壳层**（持久状态行、实时终端、文件树、并排多面板、@-context、附件、上下文预算）
  在 Lyra 里是**假数据、占位符或缺失**。真正像"agent 工作站"的产品赢在外壳，不在对话。

落到一句：**补外壳、收自伤，别动已经赢的对话核心；落地时别把对照仓库的历史债一起抄进来。**

---

## 优先级矩阵

> 影响 = 对"dense / keyboard-driven agent 工作站"定位的拉动；工作量/风险为粗估。
> "spec 已要求" = DESIGN.md 自己已经写了、但实现没跟上（自伤，优先级天然更高）。

| ID | 差距 | 影响 | 工作量 | 风险 | spec 已要求 |
|---|---|---|---|---|---|
| **G1** | 持久状态行（dense data row） | 高 | 中 | 低 | ✅ §8 |
| **G2** | 只读工具折叠分组 | 高 | 中 | 低 | — |
| **G3** | 多面板 / 并排工作区 | 高 | 大 | 中 | 部分 §4 |
| **G4** | @-context / @-mention 注入 | 高 | 大 | 中 | — |
| **G5** | 真实终端（替换 fixture） | 高 | 中 | 中（依赖后端） | — |
| **G6** | HITL 键盘化 + 分级 always-allow + mode segmented | 中高 | 中 | 低 | 部分 §components |
| **G7** | 上下文预算可观测 + 一键压缩 | 中高 | 中 | 中 | — |
| **G8** | 清死占位符（搜索/附件/用户卡/终端假数据/plan 假 header/Stage all） | 中 | 小 | 低 | — |
| **G9** | 收 spec 漂移（ALL-CAPS / dark shadow / reasoning 左边框 / 暗调语义色 / rail 默认 / 会话 tab） | 中 | 小–中 | 低 | ✅ 多处 |
| **G10** | 流式打磨（stable-prefix+mutable-tail / 状态 crossfade） | 中 | 中 | 中（热路径） | — |
| **G11** | 消息分支导航 `< i/N >` | 低中 | 小 | 低 | — |
| **G12** | 文件树 / 工作区浏览 | 中 | 中 | 低 | — |

**建议起手**：G1 + G2（最高 ROI、贴合 dense 定位、复用现有注册表、不动架构）；其次 G8 + G9（低风险快速止血、先恢复信任）；G3 是最大结构升级，单独立项。

---

## 研究方法与范围

- **对照集（8 个 GUI + 1 组 TUI，按用户要求排除 `portai*`）**：
  - GUI agent 客户端：**OpenHands**（最近邻）、**cline**、**opencode**、**continue**
  - 设计基准 chat UI：**cherry-studio**、**lobe-chat (lobehub)**、**assistant-ui**（组件范式）
  - TUI 密度范本：**codex**（Rust/ratatui）、**crush**（Go/bubbletea）、**plandex**
- **方法**：每个仓库一个独立 agent，按统一 10 维 rubric（app shell / 消息流 / 工具渲染 / 代码与 diff /
  HITL / composer / 可观测 / 导航历史 / 视觉语言 / 可偷模式）深读源码并产出带 file:line 的报告；
  另一个 agent 对 Lyra 实际渲染组件做诚实盘点（区分"spec 意图"与"实现现实"）。
- **未纳入**：`MiMo-Code`（疑似 opencode 衍生）、`AionUi`/`Proma`/`agent-chat-ui`（同类信号已被上面覆盖）。
  如需补充可再起 agent。
- **时效**：file:line 为研究当时各仓库 HEAD 的读取结果，落地前应复核。

---

## 与现有文档的关系

- 本档案是 [`frontend/DESIGN.md`](../../frontend/DESIGN.md) 的**输入/对照**，不替代它。
  G9（spec 漂移）直接指回 DESIGN.md 的条款。
- 任何落地仍受 [`CLAUDE.md`](../../CLAUDE.md) 第一法则约束：在源头改对、不留历史债、不抄对照仓库的债。
