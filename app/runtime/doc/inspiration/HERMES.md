# Hermes 启发的能力吸纳 Backlog —— 聚焦「记忆」与「自进化 Skill」

> **来源**：对 **Hermes Agent**（Nous Research，MIT，Python；单用户自托管 agent runtime，TUI + messaging gateway + React 桌面；桌面克隆 `~/Desktop/hermes-agent`）的源码级对比分析。**按用户指定聚焦两块：① 记忆怎么做的、② 自进化 skill 怎么做的。** Hermes 自称"唯一带内置 learning loop 的 agent"，且**非多租户云**（单本地用户、一个 `~/.hermes/` profile、SQLite `state.db`）——治理默认**比 lyra B4 弱**（默认自由写），但**自动挖掘/自动进化机器比 lyra 现有的都丰富**。两块由一个机制串起：**post-turn 后台 review fork**（`agent/background_review.py`）+ **weekly curator**（`agent/curator.py`）。方法与 [`GROK.md`](GROK.md) 一致；跨应用总索引见 [`README.md`](README.md)。
>
> **状态**：全部 proposed。

---

## FOCUS 1 —— 记忆（→ 落地 deferred 的 C8）

### Hermes 怎么做（源码级）

**三层，只有第一层是 lyra C8 意义上的"记忆"：**

- **Tier 1 · 有界 curated in-prompt 记忆**（`tools/memory_tool.py`, `agent/memory_manager.py`）：`~/.hermes/memories/` 两文件——`MEMORY.md`（agent 笔记，**硬顶 2200 字符/~800 tok**）+ `USER.md`（用户画像，1375 字符/~500 tok），`\n§\n` 分隔的 prose。**session start 冻结注入快照**（`_system_prompt_snapshot`），mid-session 绝不变（保 prefix cache 稳定）；写立即落盘、但下次 session 才进 prompt。写=agent 经 `memory` 工具 `add`/`replace`/`remove`/`apply_batch`（无 `read`——已在 prompt 里），`replace`/`remove` 用**唯一子串匹配**。**无 auto-compaction**：溢出时工具返回一个**教模型当轮 consolidate 的结构化错误**（"Consolidate now: use 'replace'… then retry this add — all in this turn"），并**限每轮 3 次失败**防脆弱 add 把整轮烧到预算耗尽；`apply_batch` 是原子逃生舱（remove+add all-or-nothing）。**安全**：每次写用共享威胁库 `strict` 扫描，**且 load 时对每条盘上条目重扫**——被投毒条目在快照里换成 `[BLOCKED: …]` 占位、raw text 留在 live state 供用户看/删（因为条目冻结进 system prompt，投毒会存活整个 session）。**external-drift guard**：file-locked read-modify-write，若盘上文件不能 round-trip（shell 追加/手改/兄弟 session 写过）就拒绝 clobber 并存 `.bak`。**读路径无检索无排序**——整个有界库逐字注入（char cap 的全部意义：小到永远能带，故无需 relevance/recency 逻辑）。治理：`memory.write_approval` 默认 **false=自由写**（true=stage→`/memory pending|approve|reject`，跨重启）。scope：`memory`(agent) vs `user`(profile)，均 **global per-profile**，无 per-project/session scope；**无 decay**（有界替代老化）。
- **Tier 2 · session 搜索（FTS5 关键词）**（`tools/session_search_tool.py`）：所有 CLI+messaging session 存 SQLite + **FTS5 全文搜索**，按需 `session_search` 工具（discovery/scroll/browse 三形态）。**关键词非语义**，"无 LLM 摘要、无截断"；无界、~20ms、未查询前零 token 成本。这是"上周是否聊过 X"层。
- **Tier 3 · 外部 memory provider（8 插件）**：builtin + **至多一个**外部 provider（Honcho/Mem0/…），外部**并行于** builtin 而非替代；`MemoryManager` 把 `prefetch`/`sync_turn`/`on_memory_write` fan-out 到**单后台 worker**（慢 provider 不阻塞 turn，源于一次"298s Hindsight block"事故）。Honcho=辩证式用户建模（语义/图，自动抽取事实）——**这层委派给插件、非 in-house**。

**自动挖掘**：同一个**后台 review fork**（见 Focus 2）按**turn cadence** 触发——`_memory_nudge_interval=10`，每 ~10 个 user turn 置 `should_review_memory`，fork replay transcript 问"用户是否透露 persona/preference/expectation？用 memory 工具存"。

### 可借鉴点

1. **有界 + 溢出强制当轮 consolidate，替代后台 "dream" compactor**：硬 char cap，溢出返回结构化错误让模型当轮自 curate（有界重试）。比 Grok 的 locked LLM dream consolidator 更简单便宜，且库保持 agent 拥有。
2. **frozen-at-session-start 快照**作为显式不变量（立即落盘、下次 session 才进 prompt，prefix-cache 稳定）。
3. **load 时重扫 → 占位**：任何进 system prompt 的文件都在 load 时重扫（持久面上的注入防御）。
4. **FTS5 关键词 session-log 层**作廉价召回——"是否聊过 X"不需要嵌入。
5. **轨迹自动挖掘**：一个隔离、工具白名单的 review pass 按 cadence 触发。
6. **写带 provenance 元数据**（write_origin foreground vs background_review、session_id、platform）。

### Lyra gap（vs C8-deferred）

lyra 今天**只有**单个可编辑 `LYRA.md` + `memory.*` RPC；C8 design-only——无 session-log 层、无自动挖掘、无检索、无 consolidation。Hermes 正好填这个洞，且**单用户形态**。关键：Hermes **原则上拒绝 auto-compaction**（"Memory does not auto-compact"）——与 lyra 对 LYRA.md 已持的有界/curated 哲学一致，**与 Grok "dream consolidator" 相反**。这是 lyra 要**显式选**的设计岔路：**有界+当轮强制（Hermes）vs 无界+后台 dream（Grok）**——Hermes 那条更贴 lyra 薄核 + 反向不变量（无隐藏 retry 层、写保持生命周期触发）。

### Verdict：部分吸（filter：薄核优先 + 不引双机制债）

- **吸**：自动挖掘 review pass、FTS5 session-log 层、溢出强制当轮 consolidate、load 时重扫、frozen-snapshot 不变量。
- **不吸**：**两文件切分**（agent-notes vs user-profile）——若 lyra 保持单 `LYRA.md`，引入它就是对 C8 的双机制债；**8 个外部 provider**（多厂商插件蔓延，Honcho 是云服务，off-strategy）。

### 落点 + priority

- **P2 · `app/runtime`**：C8 builtin 层 = 有界 curated 库 + `add/replace/remove` + 溢出强制当轮 consolidate 错误；**复用现有 LYRA.md、不切分**。
- **P2 · `app/runtime`**：FTS5 关键词 session-log 召回层（lyra 计划复用 @codebase 嵌入做向量——**先/并加更便宜的关键词层**，Hermes 证明关键词对常见场景够用）。
- **P3**：memory 块 load 时注入重扫 → 占位。

---

## FOCUS 2 —— 自进化 Skill（vs B4）

### Hermes 怎么做（源码级）

**skill 是什么**：markdown `SKILL.md`（agentskills.io 兼容 frontmatter + body）+ 可选 `references/`/`templates/`/`scripts/`/`assets/`，在 `~/.hermes/skills/<category>/<skill>/`。是**prose 形态的程序性记忆**（scripts 经 `terminal` 可跑，但 skill 本身是文档）。**渐进披露**：L0 `skills_list()`（name/desc/category 索引，常驻 system prompt ~3k tok，desc 截 60 字符）→ L1 `skill_view(name)`（全 body）→ L2 `skill_view(name, path)`（引用文件）。

**新 skill 三条创建路径**：① agent 前台经 `skill_manage`（create/patch/edit/delete/…）；② **`/learn <anything>`**（`learn_prompt.py`：**live agent** 用现有工具收集 sources——dir via `read_file`、URL via `web_extract`、"刚才做的" via history、粘贴的笔记——写一份 SKILL.md，**无独立蒸馏引擎**，因为它就是普通一轮，任何 backend 都行）；③ **后台 review fork 从轨迹自动蒸馏**（`background_review.py`）——**亮点**。

**自动蒸馏机制**：一个**复杂 turn** 后（`_skill_nudge_interval=10` 个 tool 迭代；docs 的"5+ tool calls"启发），daemon 线程 **fork 一个新 `AIAgent`**：replay 对话快照、**工具白名单只有 `memory` + `skill_manage`**（其余运行时 deny）、auto-deny 危险命令、隔离跑（`_persist_disabled`、`skip_memory=True`、字节一致的 `tools[]`/system-prompt 保 prefix-cache、`compression_enabled=False`）；默认跑**主模型**（warm cache 便宜）或路由到**便宜 aux 模型**+ compact digest replay。被一个重度工程化 prompt 驱动（`_SKILL_REVIEW_PROMPT`）："**要 ACTIVE——多数 session 产出 ≥1 个 skill 更新，no-op pass 是错失学习**"；目标**CLASS-LEVEL umbrella skill** 而非一 session 一 skill；**偏好序**：(1) patch 当前加载的 skill →(2) patch 已有 umbrella →(3) 加 `references/`/`templates/`/`scripts/` 支撑文件 →(4) 才创建新 umbrella；显式**反模式清单**——**别**捕获环境依赖的失败、负面工具断言（"browser 工具没用"）、已解决的瞬时错误、一次性任务叙事，因为那些会"硬化成 agent 日后拿来拒绝自己的理由"。

**进化循环（skill 怎么改进）**：**反馈驱动 patch**——用户纠正 style/workflow（"别做 X""太啰嗦""记住这个"当一等 skill 信号）或本 session 加载的 skill 被证明过时时，patch **现有** skill。即从**对话纠正与涌现技巧**改进，**非 success/failure 指标**。**usage tracking**（`.usage.json` sidecar：use/view/patch_count、last_*、state∈{active,stale,archived}、pinned、created_by）。**Curator**（`curator.py`，weekly 168h，inactivity 触发，跑 aux client、绝不碰主 prompt cache）：**(a) 确定性生命周期** active→**stale@30d**→**archived@90d** 无活动、**re-use 复活**、never-used/新建有 **grace floor**、pinned/cron-referenced/protected/hub-installed/external/bundled **豁免**；**(b) opt-in LLM umbrella 合并**（默认 **off**，aux 成本）；**绝不硬删**——只归档到 `.archive/`、完全可恢复、支持 dry-run。

**验证/晋升/provenance 门**：**无功能/测试门**（Hermes 从不*跑* skill 验证——verification 是 agent 写的 prose 段）；agent 创建的 skill **静态安全扫描默认 off**（in-code 理由："agent 本就能经 terminal() 无门执行同样代码，扫描只增摩擦无实质安全"），hub-installed 恒扫；**write-approval 默认 off=自由写**（true 才 stage）；**provenance-gated curation**：ContextVar 标记 `background_review` vs `foreground`——**只有 review fork 创建的 skill 标 `created_by=agent` 并受 curator 管**（auto-archive/consolidate 资格），**用户前台让 agent 写的绝不 auto-curate**；**read-before-write guard**：review fork patch 前必须 `skill_view` 目标（防它改写只从 transcript 推断的内容）；**versioning**：有 `version:` semver 字段但**无自动 bump、无历史**（archive 是唯一"历史"）。

### 可借鉴点

1. **从刚结束的轨迹自动蒸馏**——隔离、工具白名单、cadence/复杂度触发的 review pass，agent 不必记得主动提 skill。
2. **反馈驱动改进*现有* skill**（用户纠正/加载 skill 被证错时 patch）+ **umbrella 塑形**（class-level 非一 session 一 skill）+ 显式**反模式捕获清单**（可移植的 prompt 智慧）。
3. **usage 驱动的自动闲置生命周期**（active→stale→archived by inactivity、re-use 复活、grace floor、**never delete**）+ pinned/referenced 豁免。
4. **provenance-gated 自治**：只 auto-curate 自动创建的 skill，放过人/用户创作的。
5. **read-before-write guard**（任何自治编辑器）。

### Lyra gap（vs B4）

lyra B4 = `propose_skill`→静态安全扫描→`_drafts/`→**强制 HITL 晋升**→curator ACTIVE↔ARCHIVED（never delete），写生命周期所有 + 人审门。对比 Hermes：

- **治理：lyra B4 默认更强**。Hermes 自由写 skill（approval off、agent-created 扫描 off、无强制 draft staging）。lyra 的强制扫描 + `_drafts/` + HITL 门正是 Hermes 只作 opt-in 的。
- **能力：Hermes 有 B4 缺的三样**：① **自动挖掘**——B4 要前台 agent 主动 `propose_skill`；Hermes 自动 post-turn review 在旁挖轨迹。② **进化/精修循环**——B4 只创建 skill，无"从反馈改进现有 skill"路径；Hermes 从纠正 patch 现有 skill。③ **自动闲置生命周期**——B4 curator 是 ACTIVE↔ARCHIVED 但（据项目记忆）生命周期所有/手动；Hermes 加自动 staleness 降级 + grace floor + provenance gating。

### Verdict：部分吸（filter：反向不变量"写生命周期所有 + HITL 一致"）

- **吸机制、但走 B4 现有的门、不走 Hermes 的自由写默认**：自动挖掘 + 精修现有 应产出**草稿**、走 lyra **强制 HITL 晋升**——取 Hermes 的**轨迹挖掘脑**、保 B4 的**治理身**。这正是"取思想不取形态"的线。
- **吸**自动闲置生命周期（never-delete、pinned/referenced 豁免、provenance-gated）作 B4 curator 的扩展。
- **吸**工程化蒸馏 prompt（反模式清单 + umbrella 塑形）直接进 B4 `propose_skill` 指引——零机制、纯智慧。
- **不吸**：自由写默认、opt-in-only 扫描、weekly LLM consolidation（aux 成本，Hermes 自己都默认 off——单用户过度工程）、version-field theater。

### 落点 + priority

- **H-Skill-1 · P1 · `app/runtime`**：post-turn/post-run **后台 review pass**（便宜/aux 模型、工具白名单、隔离）挖完成的轨迹为 skill 候选 → **产出 B4 草稿** → 现有 HITL 晋升门。按 run 复杂度（tool 迭代数）+ cadence 触发。**这是对标 peers 的最大单项能力 gap**。
- **H-Skill-2 · P1/P2 · `app/runtime`**：同一 review pass 里的 **精修现有 skill** 路径（从用户纠正 patch 现有 skill 的草稿）、同样门控；加 **read-before-write guard**（reviewer 提 patch 前必须先 load skill）。
- **H-Skill-3 · P2 · B4 curator**：**自动闲置生命周期**（active→stale→archived、re-use 复活、grace floor、pinned/referenced 豁免、provenance-gated 使只有 auto-authored 才自动降级）。
- **H-Skill-4 · P3 · prompt-only**：把反模式捕获清单 + class-level umbrella 塑形折进 `propose_skill` 指引。

---

## Bottom line

**记忆（C8）**：Hermes 递给 lyra 一份**比 Grok 参照更贴 lyra 哲学**的完整、单用户形态 C8 蓝本——**有界 + curated + agent 拥有，溢出时强制当轮 consolidate 替代后台 dream daemon**、一个**廉价 FTS5 关键词 session-log 层**做"是否聊过 X"、一个**cadence 触发的轨迹自动挖掘器**经 memory 工具写。Hermes 逼 lyra 显式做的关键决定：**不 auto-compact**——有界化 + 溢出让模型自 curate（Hermes 刻意拒绝 dream-consolidator 路线）。跳过两文件切分（保单 LYRA.md）与 8 个外部 provider。

**Skill（B4）**：Hermes 的 skill **治理弱于 B4**（默认自由写），治理别借。但其 skill **进化机器在三个 lyra 真缺的轴上领先**：**轨迹自动蒸馏、反馈驱动精修现有 skill、自动闲置生命周期**。正解是**把 Hermes 的自动挖掘/自动精修脑接到 B4 的强制-HITL 身**——每个自动提议落 `_drafts/` 走人审门，只有 auto-authored 才 auto-curate（provenance gating）。既补 gap，又不引入 Hermes 的自由写债、不违 lyra 反向不变量。

---

## 刻意不吸清单

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **8-provider 外部记忆插件 + Honcho 云用户建模** | `plugins/memory/*` | 多厂商蔓延、Honcho 是云服务，off-strategy for 单本地用户 + 薄核 |
| **Skills Hub / marketplace（9 源）** | `tools/skills_guard.py` 等 | 多作者分发，违 filter #2（同 [Grok 刻意不吸](GROK.md) 的 marketplace）|
| **两文件记忆切分（MEMORY.md + USER.md）** | `tools/memory_tool.py` | 对单 LYRA.md 是双机制债；用户画像可作 LYRA.md 内一节 |
| **memory & skill 的自由写默认** | `*.write_approval=false` | 违 lyra B4/C8 的"生命周期所有 + HITL"不变量；lyra 保持强制门 |
| **weekly LLM umbrella-consolidation pass** | `curator.py` `DEFAULT_CONSOLIDATE=False` | aux 模型成本，Hermes 自己都默认 off——单用户 YAGNI |
| **semver `version:` 字段** | skill frontmatter | 无自动 bump、无历史——theater；archive 已是唯一历史 |
