# Grok Build 启发的能力吸纳 Backlog

> **来源**：对 **Grok Build**（`grok`，xAI/SpaceXAI 的终端 AI coding agent，Rust，~70 个 `xai-*` crate 的 Cargo workspace；桌面克隆 `~/Desktop/grok-build`，其 `crates/codegen/xai-grok-pager/docs/user-guide/` 24 篇是可信的能力索引）的源码级对比分析。Grok Build 与 lyra 是**高度同构的两个生产 coding agent 运行时**（TUI/headless/ACP 三形态 vs lyra 的 desktop/headless）；本文档只收录"它工程化更细、而 lyra 值得搬"的点子，过 lynx 哲学筛子后落地。
>
> **本文档职责**：给"满血上下文"实现会话提供每条吸纳项的 _为什么 / 目标 / 落点 / 计划 / 验收 / 风险 / 优先级 / 进度_，以及**刻意不吸**清单（防止未来会话重新论证）。方法与 [`../AGENTSCOPE_INSPIRED_BACKLOG.md`](../AGENTSCOPE_INSPIRED_BACKLOG.md) 一致；架构基准以 [`../EXECUTION_CENTERED_ARCHITECTURE.md`](../EXECUTION_CENTERED_ARCHITECTURE.md) 为准。跨应用合并排序与总索引见 [`README.md`](README.md)。
>
> **状态**：**全部为 proposed**（尚未实现）。分析基于 5 个能力簇的源码级并行深挖（执行/隔离、代码智能/git、run-loop/压缩、工具/协议、扩展/治理/记忆）。

---

## 0. 两条定调（先纠两个直觉误区）

1. **"Computer-Hub" 不是 computer-use**。`xai-computer-hub-*` 是一套**多租户云端工具路由 fabric**（JSON-RPC/WebSocket + OIDC + 连接池 + replay + 遥测 donation），命名假朋友。grok 全仓**没有** GUI/浏览器自动化。lyra 想要 computer-use，这里一点设计都借不到——必须另寻来源（Anthropic 式 computer-use 工具面挂在既有工具系统之后）。
2. **压缩算法 lyra 不落后、反而领先**。grok 的 `xai-grok-compaction` 每次压缩必走 **full-replace LLM 摘要**；lyra A3 的"先确定性裁剪、必要时才 LLM"更省。A1 tool-pair 安全双方同构。**不要被 grok 带偏去把压缩重构成 full-replace**。

## 0.1 筛选准则

沿用 lynx 四道筛子：① **取思想、不取形态**（lyra 用 Go + `iter.Seq2` + `context.Context`，不引 Rust/tokio/`nono`/gitoxide，不抄 TUI）；② **不为多租户云**（单进程 in-house 后端、单本地用户、单 SQLite；分布式/水平扩展/多端 leader/云分发一律不吸，但单用户**安全 confinement 是要的**）；③ **不引双机制债**（lyra 刻意保持 skills/hooks/recipes/MCP/rules 分立，已删 queueStore 换 BeforeRound steer——重引须过高门槛）；④ **薄核优先**（运行时编排落 `app/runtime`，稳定 SPI 才下沉）。另守反向不变量：无 retry-layer/Transient 分类；用户填 provider key（不做 LLM-provider OAuth）；runtime 协议无 stdio transport。

---

## 1. 吸纳清单（按价值排序）

每项标注：**来源/为什么** · **目标** · **落点**（层/模块，不写易腐 file:line）· **计划** · **验收** · **风险/边界** · **优先级** · **进度**。

---

### G1 · OS 级沙箱 confinement —— 落地 deferred 的 C7（P1，头号）

- **来源/为什么**：`xai-grok-sandbox`。声明式 allow/deny profile 编译成 **OS 内核 confinement**（macOS **Seatbelt** / Linux **Landlock + bwrap + seccomp**）：`SandboxProfile { read_only, read_write, deny, default_read, restrict_network }`，内置 `off/workspace/devbox/read-only/strict`，自定义 profile 从 `sandbox.toml` `extends` 继承。**deny 列表内核强制**（读+写+rename 全禁），堵死 `mv secret x && cat x` 绕过；缺 bubblewrap / glob 非法即 `exit(1)` **fail-closed**（拒绝在"看起来沙箱、实则没有"下运行）；项目 `.grok/sandbox.toml` **只能加新 profile 名、不能重定义**已有的（防恶意 workspace 掏空可信 profile）。
- **目标**：给 lyra 真实的、内核强制的执行边界，退役掉现在脆弱的"分类危险命令再自我否决"拐杖（B5/C3）。
- **落点**：domain 定窄 SPI（`Confiner.Apply(profile)` + `RestrictChild(cmd)`）；infra 放 OS 后端（macOS `sandbox-exec`/seatbelt profile、Linux go-landlock/bwrap）；application 放 profile 解析 + "项目只能加" merge 规则 + fail-closed 拒启动编排。
- **计划**：
  1. 定 `SandboxProfile`（allow/deny 路径集 + network flag）+ 内置 profile；`.lyra/sandbox.*` 解析 + 项目只能加规则。
  2. **逐子进程 confine**（见风险）：每个 shell 子进程用平台 wrapper 起（seatbelt/landlock），而非对 runtime 进程一次性 apply。
  3. fail-closed：策略无法强制（缺 bubblewrap、glob 非法）就拒绝执行该子进程，不降级为 fail-open。
  4. C7 落地后翻转 B5/C3（见 G1.1）。
- **验收**：sandbox 开启时，越界写/网络被内核拒绝；`mv secret x && cat x` 取不到 deny 路径；策略不可强制时拒绝执行而非静默放行；正常 in-workspace 命令不受影响。
- **风险/边界**：**架构分歧（必须尊重）**——grok 是一次性 CLI、启动时对**自身进程**一次性、不可逆 apply；lyra 是**长驻多 session、跨 workspace** 后端，不能锁死 runtime 进程 → 必须**逐子进程 confine**。取"声明式 profile → OS confinement + fail-closed + 项目只能加"的思想，**不搬 `nono`**。bwrap re-exec 与 Seatbelt last-match 怪癖是"看起来沙箱实则没有" bug 的高发区，预算要留足。
- **优先级**：**P1（头号）**。

### G1.1 · confinement 换来"降摩擦"的资格（P1，随 G1）

- **来源/为什么**：`xai-grok-workspace/permission/manager.rs`。沙箱开启时 bash 自动放行，**除了**一个仍要人审的 floor（越界写真实文件 / 设 unsafe env 且无精确授权）。**真 confinement 才配少问**：confinement 约束爆炸半径，审批只确认残余动作的 intent。
- **目标**：C7 落地后把 B5/C3 从"yolo 也拦截危险命令"翻转为"沙箱兜底、yolo 真安全、审批只留 out-of-confinement 写 + unsafe-env floor"。
- **落点**：application 审批引擎——决策读"该子进程是否将被 confine"，是则降到 floor 规则。
- **风险/边界**：**严格作为 G1 的自然终态、不单独建**。没有真边界就自动放行=第二法则最忌的"治标"。floor 要保留（grok 沙箱下仍拦真实文件写）。
- **优先级**：**P1，门控于 G1**。

---

### G2 · 跨会话记忆 —— 落地 deferred 的 C8（P1，头号）

- **来源/为什么**：`xai-grok-memory`。两层：**curated evergreen**（`~/.grok/memory/MEMORY.md` 全局 + `{workspace-slug-hash}/MEMORY.md` 项目，人可编辑）+ **raw decaying**（`sessions/*.md` 会话日志）；全部 chunk 进 per-workspace SQLite（FTS5 + sqlite-vec）。**写路径由生命周期驱动、非 model 自由写**（无 `memory_write` 工具，只有 `memory_search`/`memory_get`）：① session-end 零 LLM 结构化摘要（≥3 真实 user prompt 且 ≥50 字节才写）② flush（LLM 摘要 + 语义去重 cosine≥0.92 才写）③ **"dream" 周期性反思式蒸馏**——把 session log 合并进 curated `MEMORY.md`，解矛盾、相对→绝对日期、弃 ephemera、留决策/理据/架构，无可留则 `NO_REPLY`，带 gate（enabled→min_hours→min_sessions）+ DreamLock + **只删已读且在 cap 内的 session log**。读=混合 FTS5+向量 + 时间衰减（evergreen 豁免）+ staleness 标注 + 首轮注入 + compaction 后再注入。
- **目标**：让 lyra 跨会话记住项目约定/决策/调试经验，agent 自动召回，写路径受生命周期治理 + HITL 一致（对齐 B4）。
- **落点**：新 `memory` 子模块（SPI 形，feature-gated 默认关），**复用 lyra 现成的 @codebase embedding/cosine**；`LYRA.md` 直接当 curated 层；dream/consolidation 由 session lifecycle（session-end + `/memory consolidate`）驱动。
- **计划**：
  1. 加 session-log 层（每会话一份 markdown）。
  2. dream consolidation：gate + lock + `NO_REPLY` 自限 + 只删已读；蒸馏进 `LYRA.md`。
  3. `memory_search` 工具 + 首轮注入，复用现有 embedding/cosine。
  4. 只取 20%——MMR / query-expansion / 时间衰减 knob / 文件 watcher / GC 先 defer（YAGNI）。
- **验收**：多会话后新会话首轮能召回上次的项目约定/决策；dream 不写垃圾（`NO_REPLY` 生效）、不重复（语义去重）、不删未读日志；写全部经生命周期、无 model 自由写。
- **风险/边界**：grok 全栈（dream+hybrid+MMR+query-expansion+decay 调参+watcher+GC）对单用户是过量精调，只取"session-log + dream + search + 注入"这条主干。写保持生命周期所有，与 B4"无 model 自由写"一致。
- **优先级**：**P1（头号）**。

---

### G3 · 压缩后把"活的执行态"作为确定性 system-reminder 带过去（P1）

- **来源/为什么**：`xai-grok-compaction/reminder.rs`。LLM 摘要**天然会丢活的执行副作用**。grok 压缩重建时除 prose 摘要外，确定性拼一段 system-reminder：Running Background Tasks（task_id + 命令 + 状态 + poll 工具名）、TODO List（逐条 status）、Running Subagents（id + 已运行秒数 + poll/cancel 工具名），工具名从当前 toolset 动态解析（解析不到就整段省略而非指错名）。
- **目标**：压缩后模型仍确定性地知道"你有后台 shell 在跑（id X）、3 个 in-progress todo、子 agent Y 已跑 42s"并知道用哪个工具去 poll/cancel。
- **落点**：app/runtime 压缩重建路径末尾——用运行时快照（running shells / todos / running subagents）生成独立 reminder item 注入；工具名从当前 toolset 解析。
- **计划**：① 快照运行态 ② 确定性拼 reminder 段 ③ 作为独立 item 注入重建历史末尾。
- **验收**：构造"压缩前启动后台 shell + 若干 todo + 子 agent"，压缩后断言 reminder 段含其 id/状态/poll 工具名；无 LLM 调用参与该段。
- **风险/边界**：这是压缩**正确性的独立维度**，与摘要质量无关；零 LLM、single-user 完全成立。
- **优先级**：**P1**（本次压缩类最高价值借鉴）。

---

### G4 · 渐进式工具披露 / `search_tools`（P1）

- **来源/为什么**：`xai-tool-runtime/search.rs`。MCP 工具**不全塞进 manifest**——建 `ToolSearchIndex`，给模型一个 `search_tools`（Claude 别名 `ToolSearch`）meta-tool 返回 top-k（qualified name + description + **完整 input_schema**，模型无需二次取 schema），带 `total_hidden_tools` / `is_ready`。第一方工具照常常驻，**只有 MCP 工具进搜索**。
- **目标**：接多个富 MCP server（Linear/Slack/GitHub…）时不让 manifest 与 prompt token 爆炸。
- **落点**：新 builtin `search_tools` 工具 + 内存索引（over MCP `QualifiedToolName` 描述，BM25 或线性扫即可）；阈值以上把 MCP 工具移出常驻 manifest。
- **计划**：① 内存索引 over 已知 MCP 工具描述 ② `search_tools` 工具返回 name+schema+server+score ③ 阈值门控 MCP 工具是否常驻 ④ "N of M tools" system-reminder 用 `total_hidden`。
- **验收**：接 3+ 富 MCP server 时 manifest token 显著下降，模型能经 `search_tools` 找到并直接构造调用；第一方工具始终可见。
- **风险/边界**：薄核、无协议改动、无新依赖；与现有 `QualifiedToolName`/`disabledTools` 互补。
- **优先级**：**P1**。

---

### G5 · Hunk 级、按 agent-turn 归属的改动追踪 + accept/reject（P1~P2）

- **来源/为什么**：`xai-hunk-tracker`（+ enabler `xai-fsnotify`）。per-session actor 维护每文件 `{baseline=git HEAD, current, Vec<Hunk>}`，每个 Hunk 带 `HunkSource` 归属（`AgentEdit{prompt_index}` / `ExternalEditOnAgentFile` / `External`）；逐 hunk / 按文件 / 按 turn / 全部 **accept**（前推 baseline）或 **reject**（改回 baseline）；emit 按 turn 分组的 pending hunks summary。
- **目标**：给用户一个"Review Changes"面——按 hunk 与 agent 轮次精挑保留/驳回 agent 的改动，区别于整仓 undo。
- **落点**：新 app/runtime 子系统（per-session hunk tracker）；`workspace.hunks.*` RPC + 桌面"Review Changes"面板。归属键复用 lyra 现有 `runId/segmentId`（≈ grok 的 `prompt_index`）。
- **计划**：① edit 工具每次写标 `runId/segmentId` ② diff（go-diff/unified-hunk）vs git HEAD baseline ③ per-hunk / per-turn accept·reject ④ 桌面面板 ⑤ enabler G5.1。
- **验收**：agent 改多文件后，UI 按轮次列 hunk，逐 hunk accept/reject 生效且不误伤其他改动；外部编辑标记为 External。
- **风险/边界**：**accept/reject 会改盘 = 第三条文件 mutation 路径**，必须走同一 per-session lock + edit-tool stale-guard，否则与编辑工具/回滚回滚打架。与整仓 checkpoint **不重叠**（精挑 curation vs 时间旅行 undo），非双机制债。LFS/symlink/`baseline_accepted`/JSONL-LOC-sink 是 grok 特有精调，先只落核心。
- **优先级**：**P1~P2**。

### G5.1 · 语义化文件事件流（P2，作 G5 enabler）

- **来源/为什么**：`xai-fsnotify`。把 raw notify 事件变成语义 `FsEvent`（`FilesChanged` / `GitMetaChanged{Head/Index/Refs}` / `GitOperationStarted·Completed`），关键是用 **lock-file 状态机（Idle→Locked→Settling→Cooldown）合并 rebase 风暴**成一对 Started/Completed 并报 HEAD 是否真移动。
- **目标**：把 **git HEAD 变更**当一等事件 → 触发 hunk baseline 刷新（否则 diff 全是幻影）。
- **落点**：app/runtime watcher（Go fsnotify + debounce + 小 HEAD/index/refs 状态机）。
- **风险/边界**：**只作"workspace 变更"的唯一真源替换现有显式 re-index，不叠第二条路**（否则双机制债）。价值门控于 G5——单独作"增量 re-index"nicety 时优先级低（lyra 刻意选显式）。
- **优先级**：**P2（G5 采纳则升 P1）**。

---

### G6 · 工具输出三分 + 一等 media 句柄 —— 落地 parked 的 Media（P2）

- **来源/为什么**：`xai-tool-runtime/{tool.rs,render.rs}`。一个工具 typed 输出**一次性**投影三份：`value`（原始 JSON，agent/client 逻辑）/ `model_output(): Vec<ContentBlock>`（LLM 看的，保证非空）/ `chat_completion_output`（可选前端 render 卡片）。`ContentBlock::Image` 带 `data`(base64) + **`media_id`**（后续工具调用可引用"我刚生成的那张图"）。
- **目标**：把 lyra **parked 的 `ToolReturn.Media`** 落地，且干净区分 model-view / UI-render / raw-data 三个消费者。
- **落点**：core 工具 shape——`ToolReturn.Media = []ContentBlock`（Text/Image/Resource）+ `MediaID` 句柄；独立 UI-render 投影。
- **计划**：① `ToolReturn` 三投影 ② media block + MediaID ③ per-adapter M1/M2 落 anthropic/openai（对齐已有 media 计划）。
- **验收**：工具返回 typed media block，LLM 侧见 content block、UI 侧见卡片；后续工具能按 MediaID 引用。
- **风险/边界**：取**显式**形态（工具作者返回 typed block），**弃 grok 的 JSON 魔法自动提升**（一个叫 `screenshot` 的字段被静默变媒体 = KISS 反面）。
- **优先级**：**P2**。

---

### G7 · subagent 的 typed IO 契约（P2）

- **来源/为什么**：`xai-grok-subagent-resolution`。persona 声明式 `inputs`/`outputs`（`PersonaIOField{name, io_type, required, description}`），父 agent 读其声明"知道该给什么文件/上下文、该收什么产物传给下一 agent"。
- **目标**：把 lyra 的一次性 subagent 委派升级为**可组合的流水线阶段**（explore→review→fix 无需硬编码自动串联）。
- **落点**：给 lyra 的 subagent/agent 定义加可选 `inputs`/`outputs`（name/type/required/description）；task 工具描述里 surface 出来供编排 agent introspect。
- **风险/边界**：保持"父可读的声明式元数据"，**别做强制 dataflow 引擎**（KISS——grok 的也只是父读的字段）。
- **优先级**：**P2**。

---

### G8 · `tool_scope=Write→仅 leader` + `is_read_only→loop 检测`（P2）

- **来源/为什么**：`xai-tool-protocol/capabilities.rs`。`is_read_only`（喂 doom-loop 检测）+ `tool_scope: Read|Write`（写工具路由到 **leader agent only** 做多 agent 写协调）。
- **目标**：多 agent run 里"只有 leader 能跑 mutating 工具、子 agent 只读/探索"的声明式基础。
- **落点**：从 lyra 现有 safety class 派生 `ToolScope`；在 subagent coordinator 强制"Write ⇒ leader session only"。
- **风险/边界**：廉价声明式 flag、真安全；契合第二法则（在正确层协调 mutation，而非靠约定）。
- **优先级**：**P2**。

---

### G9 · 压缩健壮性三件套（P2）

- **来源/为什么**：`xai-grok-compaction` + host `session/compaction.rs`。① **退化摘要门**：清洗后 <500 字符视为 degenerate、重试 ② **preflight overflow**：发请求前估 token，注定超窗就**主动**触发压缩（而非等 400 回来）③ 触发阈值**预留 summary/response headroom**（`exceeds_threshold_with_headroom`）。
- **目标**：压缩更稳——摘要质量有门、在注定失败的请求之前压、留输出余量。
- **落点**：app/runtime 压缩触发与摘要路径。
- **验收**：短摘要触发重试；超窗前主动压；触发点留 headroom。
- **风险/边界**：**切勿连 grok 的 `Deterministic/Transient` 重试分类一起抄**——那正是 lyra 反向不变量禁的 retry-layer；只要"质量门/降级输入/提前压"这几个决策，不要那台分类机。input-ladder（超窗历史降级喂给 summarizer）仅在确认 lyra 存在"历史大到压不动"死局时再补（P3）。
- **优先级**：**P2**。

---

### G10 · verbatim 骨架：用户原话 + AGENTS.md 永不进摘要（P2）

- **来源/为什么**：`xai-grok-compaction/history/filter.rs`。**所有** user 消息抽成块跨多次压缩逐字保留、**永不被再摘要**（再压缩前从上一版摘要里剥出单独携带，防 snowball 退化）；AGENTS.md 压缩后 verbatim 重注入（带 tag 供 resume 幂等守卫识别）；超长 user 消息 head+tail 截断而非丢弃。
- **目标**：摘要可漂移，但用户字面诉求 + 项目指令是"骨架"，走 verbatim 通道而非 LLM 通道。
- **落点**：app/runtime 压缩重建——确保项目指令/AGENTS.md verbatim 重注入（带 resume 幂等 tag）+ 前次摘要里的用户原话骨架在再压缩前剥离单独携带。
- **风险/边界**：取思想，不取 `<grok_user_queries>` 的 XML 具体形态。
- **优先级**：**P2**。

---

### G11 · steer 永不静默丢失（P2，正确性收尾）

- **来源/为什么**：`xai-interjection-core` + host `interjection.rs`。interjection 在 turn 安全点 drain、framing 成独立 synthetic user message 注入、**永不取消 turn**；关键：若在 idle 时或最后一次 drain 之后到达（错过 turn），转成一个 queued prompt turn（`flush_stranded_interjections`）而**不静默丢失**。framing **不加 defer 指令**、把权衡交给模型（与 lyra 哲学一致）。
- **目标**：lyra BeforeRound steer 若在"turn 已无后续 round"时到达，应降级为下一 prompt turn 而非丢弃；并考虑 interject/send-now 的显式二分。
- **落点**：核对并加固 lyra BeforeRound steer 的 stranded 兜底。
- **风险/边界**：核心能力已 **parity**（BeforeRound steer == interjection），只吸这个健壮性收尾。
- **优先级**：**P2**。

---

### G12 · MCP SSE 重连 backoff（P2，直接做）

- **来源/为什么**：`xai-grok-mcp/mcp_http_client`。给 rmcp 的 HTTP client 包一层 backoff，修其零 backoff 的 SSE 重连洪泛（带 `repro_sse_flood.rs` 回归测试）。
- **目标**：MCP HTTP/SSE 断连不洪泛重连。
- **落点**：lyra MCP HTTP client。
- **风险/边界**：廉价健壮性，与 lyra 现有 MCP gating 平级、无冲突。
- **优先级**：**P2**（直接做）。

---

### G13 · 秘密脱敏 chokepoint（P2，条件 P1）

- **来源/为什么**：`xai-grok-secrets`。两个纯函数（`redact_secrets` regex 快路径 + `redact_user_paths`），**只挂遥测导出 chokepoint**（内/外 OTLP pipeline），**不撒进业务代码、不脱敏 model 上下文**。
- **目标**：不把用户自己的凭证/家目录名泄进 log/telemetry/导出的 session。
- **落点**：leaf pkg 纯 `redact` helper（regexp + 合并 alternation 快路径），挂 OTel export decorator（adapter/infra）+ session-export；**绝不挂 live model turn**。
- **风险/边界**：**绝不脱敏 model 输入**（grok 也故意不脱——agent 常需要看 `.env` 调试，静默脱敏=治标 correctness bug）。一个 redact 函数两个调用点（trace export + session export），别 fork。
- **优先级**：**P2**（若 lyra 导出遥测到任何非本地 sink 则升 P1）。

---

### G14 · 非交互 shell 注入 pager/color env（P2）

- **来源/为什么**：`xai-grok-shell` terminal（`color_env`/`no_color_env`/`pager_env`）。非 TTY 子进程执行时注入 env：禁 pager（`GIT_PAGER=cat` 类，防 `git log` 挂起）+ 强制/抑制 ANSI color（`FORCE_COLOR=1`/`CLICOLOR_FORCE`/`CI=true`/`NO_COLOR`）。
- **目标**：堵"为什么我的命令挂住了"这一 bug-class + 捕获输出更好看。
- **落点**：application 层 shell 子进程 env 注入。
- **风险/边界**：廉价、自包含、与 PTY 无关（独立于 lyra 的 no-PTY 决策）。
- **优先级**：**P2**。

---

### G15 · config / approval 微加固（P3）

- **来源/为什么**：`xai-grok-config`。① 配置报错只用 span、**不回显含密源码行**（`toml_error_detail`）② 拒绝把不可信项目 `.grok` 经 cwd-relative fallback **提升到 user 层** ③ permission rule 默认 **Deny**（CWE-1188 fail-closed omission）。
- **目标**：对 lyra 现有 config/approval loader 做一次安全核查。
- **落点**：core config + approval loader。
- **优先级**：**P3**（核查项，若已满足则关闭）。

---

### G16 · hooks seam 覆盖审计（P3）

- **来源/为什么**：`xai-grok-hooks`（14 个 seam）。对照 lyra 现有 seam，补 **PreCompact / PermissionDenied / SubagentStart·Stop**（若 lyra 跑 subagent/compaction 而缺）；确认**单一 folder-trust 权威**统一门控 hooks+MCP+LSP。
- **目标**：hook seam 覆盖不漏关键生命周期点。
- **落点**：现有 hooks 模块（无新机制）。
- **风险/边界**：lyra hook 设计已等价，**别重构**；HTTP hook = skip（单用户 subprocess 足够）；plugin-bundle 统一 = 不吸（见刻意不吸）。
- **优先级**：**P3**。

---

### G17 · ACP 设计先借、线协议 defer

- **来源/为什么**：`xai-acp-lib`。base ACP + `x.ai/*` 扩展让**一套协议**同时服务编辑器嵌入（Zed/Neovim/Emacs）与 grok 自己的前端；typed `session/update` 事件分类（`agent_message_chunk`/`agent_thought_chunk`/`tool_call`/`tool_call_update`/`plan`）+ "client 托管面"倒置（permission、**buffer-aware** fs read/write、terminal 生命周期委派给 host）。
- **目标**：(i) 编辑器触达（产品决策）；(ii) 协议设计点（现在就能借，与是否上 ACP 无关）。
- **落点/计划**：**现在只借设计**——给 lyra 自有 runtime→desktop 事件流一套 typed `session/update` 分类；把 file read 建模成能返回 **未保存 buffer** 内容的 host capability。**线协议本身 defer**——只有编辑器触达成为产品目标时，才加 additive `lyra agent stdio`（Go ACP SDK `coder/acp-go-sdk`），**绝不重写桌面协议**。
- **风险/边界**：**不是**双机制债（方向=外部编辑器驱动我的 agent，异于 lyra 桌面协议与 a2a，反向不变量"runtime 协议无 stdio"也不违反——ACP-over-stdio 是对编辑器说的另一套协议）。但 grok 自己证明"上 ACP 不消除自有协议、只是 rebase 成 `x.ai/*` 扩展"——win 是产品/分发决策、非工程免费午餐。
- **优先级**：**defer（设计点可随手借，P3）**。

---

### G18 · prompt 队列（type-ahead 待跑）—— 先证伪再动

- **来源/为什么**：`xai-prompt-queue` + host `prompt_queue.rs`。SessionActor 持权威 FIFO `pending_inputs`：用户可**排队/编辑/重排/清空/send-now** 未来 turn 的 prompt；Queue（当前 turn 后 FIFO 跑）/ Send-now（插到运行 turn 后立刻跑，可选取消当前）/ Interject 三语义分明。
- **目标**：agent 干活时用户先把后续几个任务排上、看着跑、随时改序/撤——与 mid-run 转向**正交**的能力。
- **落点**：runtime per-session pending-prompt 队列 + 桌面可取消/重排队列视图；send-now = 插队并可选 interrupt。
- **风险/边界**：**关键 filter #3**——它与被删的 queueStore（mid-run **转向**）正交不重叠，过得了高门槛；但 grok 实现重度绑定**多端 leader 模式**（broadcast + owner + last_editor + version-LWW），那是 filter #2 要砍的复杂度，**只留单用户本质**。**务必先证伪"lyra 当前 run 进行中提交 prompt 已有等价 type-ahead"再动**，避免重造被删的机制。
- **优先级**：**P3，且先证伪**。

---

### G19 · MCP-server OAuth（待决 gap，非 reject）

- **来源/为什么**：`xai-grok-mcp`。对 MCP **server**（Linear/Slack 式）做完整浏览器 OAuth（跨进程+进程内 dedup、`mcp_credentials.json` 凭证库、BYO OAuth config）。
- **目标**：接入 OAuth-gated 的远程 MCP server。
- **落点**：lyra MCP client（device/authorization-code + 凭证库）。
- **风险/边界**：与"不给 **LLM provider** 做 OAuth"的反向不变量**是两回事**——MCP-server OAuth 是 resource-server auth，远程 MCP 生态（2025+ spec）越来越**要求**它，bearer-only 是真天花板。诚实挂 backlog、**用到再做**（YAGNI until then，但命名它）。
- **优先级**：**待决**（真有 OAuth-gated 远程 MCP server 需求时决策）。

---

## 2. 刻意不吸清单（防未来会话重新论证）

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **computer-hub fabric** | `xai-computer-hub-*` | 多租户云工具网（OIDC/连接池/replay/donation），**非** computer-use；违 filter #2/#4。lyra 的"远程工具"=MCP 已覆盖。若真需 builtin-vs-MCP 重名规则，用 local-shadows-remote 一行策略 |
| **plugin marketplace** | `xai-grok-plugin-marketplace` | 本质多作者云分发、骑 Anthropic plugin 生态；且逼你把 hooks/skills/recipes/MCP 揉成 "bundle"=双机制债。用户要 skill 包可自行 `git clone` 进 `.lyra/skills/` |
| **circuit breaker + Retryable/Terminal 分类** | `xai-circuit-breaker` | 捆绑的分类器正是 lyra 反向不变量禁的 Transient/NonTransient retry-layer；breaker 本体需并发/规模（grok 只用于云 blob 上传队列），单用户无 consumer。真出现"死 MCP 端点被紧循环猛敲"再考虑窄 per-endpoint breaker（future bug fix，非现建） |
| **PTY** | `ptyctl`/`ptyctl-cli` | 重申 lyra 已权衡后选 background commands；screen-grid 仿真 + HTTP 控制面重子系统、收益 niche（KISS/YAGNI） |
| **system-power** | `xai-system-power` | 动机是保护 OAuth refresh token 跨 suspend——lyra 无 OAuth、run 本就 durable 可跨重启恢复 |
| **结构化 codebase-graph 作 agent 上下文** | `xai-codebase-graph` | name-based（非类型解析）比 lyra LSP 更不准；**grok 自己都只给编辑器人工导航、agent 走单独 LSP**——正是 lyra 已有（LSP + 语义嵌入）。缺全仓符号表就给 LSP 加 `workspaceSymbol` op（治本在正确层），不另造图。仅纯前端 serverless 导航时才 defer 级"部分吸"（LOW） |
| **fast-worktree / gix-status** | `xai-fast-worktree`/`xai-gix-status` | YAGNI（lyra 无并行 worktree 隔离需求）/ gitoxide+panic=abort 特有 mitigation |
| **two-pass 投机压缩** | `session/two_pass.rs` | 纯延迟优化、烧额外 token；除非压缩卡顿成真实投诉 |
| **doom-loop 检测** | `xai-grok-sampler` | 依赖 provider 侧非标 SSE 信号（Anthropic/OpenAI 不发）；客户端自检 tail-repetition 投机（观察项：真出现复读死循环再考虑） |
| **agent-lifecycle registry / token-estimation / sampling 结构** | `xai-agent-lifecycle` 等 | lyra 已覆盖且更强（durable resume + 真 tokenizer + per-run model 配对） |
| **voice** | `xai-grok-voice` | dictation-only + 单厂商 STT + provider auth，TUI 形态、非 agent 能力；桌面有 OS 原生听写 |
| **announcements / self-update** | `xai-grok-announcements`/`-update` | 云推 channel（无云）/ Wails 打包层的事，非后端 runtime 能力 |
| **full-replace 压缩策略** | `xai-grok-compaction` | lyra A3"确定性优先、必要才 LLM"更省，别倒退成"每次必 LLM" |

---

## 3. 建议节奏

- **两块大的、独立的**：G1（C7 沙箱）、G2（C8 记忆）——各自成批，用户确认后逐批。
- **中等高杠杆**：G3（活状态 reminder）、G4（工具搜索）、G6（media 落地）、G5（hunk 追踪，含 G5.1 enabler）。
- **廉价收尾**（可随邻近 feature 带）：G8~G16。
- **待决/defer**：G17（ACP 设计先借）、G18（prompt 队列，先证伪）、G19（MCP-server OAuth）。
