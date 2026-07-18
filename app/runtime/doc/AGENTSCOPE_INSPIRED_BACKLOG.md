# AgentScope 启发的能力吸纳 Backlog

> **来源**：对 `agentscope-ai/agentscope-java` v2.0(2026-07,Java/Reactor,~46.8 万行企业级 agent 平台;桌面克隆 `~/Desktop/agentscope-java`,其根 `ANALYSIS.md` 是一份可信的源码级自析)的对比分析。AgentScope 与 lyra 是**高度同构的两个生产 agent 运行时**;本文档只收录"它工程化更细、而 lyra 值得搬"的点子,过 lynx 哲学筛子后落地。
>
> **本文档职责**:给"满血上下文"实现会话提供每条吸纳项的 _为什么 / 目标 / 落点 / 计划 / 验收 / 风险 / 进度_,以及**刻意不吸**清单(防止未来会话重新论证)。架构基准以 [`EXECUTION_CENTERED_ARCHITECTURE.md`](EXECUTION_CENTERED_ARCHITECTURE.md) 为准;本文只是它之上的一批增量能力提案。
>
> **状态**:全部 **未开始(提案)**。开工时先更新 §5 进度看板的执行指针,再动代码。

---

## 0. 筛选准则(为什么大多数 AgentScope 能力不进这份清单)

AgentScope 覆盖的能力(事件流 / HITL / 权限 / 子代理 / skills / 记忆 / compaction / MCP·A2A / 调度)lyra **大都已有、局部更强**(`iter.Seq2` 单内核、pending-call 精确重入、显式装配无双机制债)。因此吸纳只收"点子",且必须满足:

1. **取思想、不取形态** —— 不引 Project Reactor(lyra 用 `iter.Seq2` + `context.Context`);不抄 Spring Boot starter / 注解装配。
2. **不为多租户云** —— lyra 是单进程 in-house 后端(协议层零 user、单 SQLite),分布式/水平扩展/跨副本相关一律不吸。
3. **不引双机制债** —— AgentScope 自身背着 Hook↔Middleware、旧↔新 subagent bus 两套并存的迁移债;lyra 不复制。
4. **薄核优先** —— 能力若属运行时编排,落 `app/runtime`;若是稳定 SPI/协议,才下沉 core/agent/tools;不硬塞。

---

## 1. 吸纳清单

每项标注:**来源/为什么**、**目标**、**落点**(层/模块/SPI,不写易腐的 file:line)、**计划**、**验收**、**风险/边界**、**优先级**、**进度**。

---

### A1 · 压缩绝不拆散 tool-call / tool-result 对(可能是治本)

- **来源/为什么**:AgentScope `ConversationCompactor.findSafeCutoffPoint` —— 二分定位 cutoff 后**回退**,保证一个 assistant 的 tool-call 永不与它的 tool-result 被切到压缩边界两侧。这是**正确性不变量**:一旦拆散,重建的对话里会出现"孤儿 tool-result / 悬空 tool-call",多数 provider 会直接拒绝该请求或行为异常。
- **目标**:lyra 的对话压缩在任何 cutoff / 截断路径下都保持 tool-call↔tool-result 配对完整。
- **落点**:`app/runtime` 的压缩逻辑(定位 compaction 所在的 application/adapter 组件);tool-call/result 的配对语义来自 `core/chat`(Part `PartToolCall`/`PartToolResult` + tool_call id)。
- **计划**:
  1. **先核查**:定位 lyra 现有 compaction,构造"cutoff 恰好落在 tool-call 与其 result 之间"的用例,看是否会拆散。
  2. 若会拆 → 加"安全 cutoff"回退:选定 cutoff 后,若该点切断了某 call/result 对,则回退到包含发起该 call 的 assistant 消息之前(或整体保留该对)。
  3. 加回归测试:交错 text/tool-call/tool-result 的长对话,压缩后断言无孤儿 call/result。
- **验收**:新测试覆盖"cutoff 落在配对中间"的场景并通过;`app/runtime` 全绿。
- **风险/边界**:纯内部正确性修复,无协议/wire 变更。若核查发现 lyra **已守**该不变量,则本项降级为"补一个显式回归测试锁住它"。
- **优先级**:**P0(最高)** —— 若中招是真 bug,最该先验先修。
- **进度**:**已完成(核查:未中招)**。lyra 现有 `MaybeCompact` 把 cutoff 前进到下一个 user 边界,而 user 消息永不落在 tool-call 与其 result 之间 —— 不变量已守,与 AgentScope `findSafeCutoffPoint` 殊途同归(前进 vs 回退,前者还额外让 `recent` 从完整 turn 起始)。已把原 `TestCompactor_CutBoundary` 点测升级为属性测试 `TestCompactor_PreservesToolPairsAcrossCutoffs`(遍历所有 keepRecent × 交错历史,`assertNoOrphanToolParts` 断言无孤儿/悬空 tool part)锁死之。

---

### A2 · 宽度(tool-result eviction)与深度(compaction)正交

- **来源/为什么**:AgentScope `ToolResultEvictionMiddleware` —— **单个**工具结果超阈值(它用 80k 字符)即落盘到 `.../large_tool_results/<agent>/<toolCallId>`,原位换成 head+tail 占位、指向一个 read 工具重新取。它与"对话压缩"正交:eviction 管**单条过宽**,compaction 管**历史过深**。
- **目标**:单条超大工具结果不撑爆上下文,也不触发整轮压缩;LLM 可按需 readFile 取回全文。
- **落点**:`app/runtime` —— 一个作用在 tool-result 边界的组件(agentexec turn 循环 / toolset 结果处理处);落盘走现有存储/workspace;占位符指向已有的文件读取工具。
- **计划**:
  1. 定阈值(可配置,默认取一个保守字符/ token 数)。
  2. 在工具结果进入上下文前拦截:超阈值 → 落盘 + 替换为 `head + "…N chars offloaded…" + tail` + 全文路径。
  3. 排除 fs/memory 类读工具自身(避免"读→又超→再 evict"的回环);shell 类不排除。
- **验收**:超大结果被落盘 + 占位,LLM 能用现有 read 工具取回全文;正常大小结果不受影响;全绿。
- **风险/边界**:与 A1/A3 正交,不要混进压缩逻辑。落盘要走 workspace 的多租户/会话隔离路径(沿用现有 session-scoped 存储约定)。
- **优先级**:P1。
- **进度**:**已完成**。落地形态(经与用户对齐后**改为 SQLite 持久化**,而非落文件树):
  - **存储**:`sqlite` 新增 `tool_result_blobs` 表；schema v7 把 blob 与 canonical item 显式绑定，并将 v5/v6 数据无损迁移到强类型关系。`ToolResultStore` 负责 Stage/Bind/Discard/Fetch/List/Restore/PurgeUnbound/DropSession，所有读取保持 session 隔离。
  - **写(eviction)**:offload 收进 `agentexec` 的 observation chokepoint(`observedTool.Call`)—— 有界预览同时流向 transcript(`OnToolCallEnd`)与 model(返回值),session 从 `turnctx.TurnSession(ctx)` 每调用读取;offload 失败降级为全文(best-effort)。展示文本仅由共享 `component/toolresultpreview.Render` 生成；内部身份使用 `domain/execution/offload.ID/Ref` 贯穿，不再解析展示字符串。
  - **读回**:新工具 `read_tool_result`(名字常量在 `toolport`,eviction 据此排除自身防回环),session 作用域、offset/limit 分页、rune 安全、未找到/无 session 均为可恢复消息。**复用不成**(SQLite blob 非文件,fs read 读不到)→ 故起专用读回工具。
  - **单一真源(C1 精修,已 ship)**:原 A2 全文存两份(blob + transcript 全文)= 为省管线的捷径;现 **blob 为唯一全文源**,其余只存有界预览。offload 引用作为强类型事实沿 observer→reducer→原子提交传递，`history_items.offload_id` 与 blob 的 `item_id` 双向约束；读取通过结构化关联回补，不再解析展示字符串。items.list 见全文、LLM 见预览、artifact item 见预览且 toolResults 携带唯一全文。
  - **可移植性与清理**:session artifact v4 显式携带 toolResults，跨数据库导入后 read_tool_result 仍可用；restore 先清旧 blob 再在同一事务恢复。事件提交失败只补偿未绑定 staging，进程崩溃遗留 staging 在启动时回收；session 删除按 session 清理，rollback/DeleteRun 按 item 绑定精确清理，不再保留已回滚结果。
  - **配置**:`toolResultOffload.threshold`(viper,默认 50000 字节=开启;显式 0/负=关闭),贯穿 config→bootstrap→agentexec。
  - **测试**:sqlite 往返/session 隔离/绑定冲突/DropSession/DeleteRun/**v5 与 v6 迁移不丢数据**；跨数据库 artifact 往返；中间件全路径(超阈 offload、小结果不动、错误透传、无 session 保全、offload 失败降级、读回排除、能力转发)；读回工具(分页/未知、空、非法 id/无 session/window rune 安全)。

---

### A3 · 分级压缩阶梯(先非 LLM,再 LLM)

- **来源/为什么**:AgentScope compaction 是 6 步阶梯:① 非 LLM 截断超长 tool-args → ② 非 LLM 修剪旧 tool-result(保留最近 N token,旧的换 head+tail)→ ③ 安全 cutoff(=A1)→ ④ flush 记忆 → ⑤ 落盘全量 → ⑥ **仅在仍超时**才一次 LLM summary,且 summary 里**嵌入落盘文件路径**供回溯。先便宜后昂贵,省 token、更稳。
- **目标**:lyra 压缩优先用零成本的确定性裁剪,LLM summary 作为最后一步,并让 summary 可回指全量历史。
- **落点**:`app/runtime` 压缩逻辑(与 A1 同一组件)。
- **计划**:
  1. 在现有压缩前置两步非 LLM 裁剪(截断超长 args、修剪旧 result)。
  2. summary 步仅在裁剪后仍超预算时触发;summary 文本内嵌 A2 落盘内容的引用 —— **注意 A2 已落 SQLite**,故引用是 `read_tool_result` 的 blob id(不是文件路径),与 A2 对齐。
  3. summary 消息打稳定标记(如 `__compaction_summary__`),避免后续轮次把它当普通历史再归档。
- **验收**:能构造"裁剪即达标、不触发 LLM"的用例;summary 路径可被 read 工具取回;全绿。
- **风险/边界**:依赖 A1(安全 cutoff)、A2(落盘)先到位;三者是一组,建议同批实现。
- **优先级**:P1(与 A1/A2 同批)。
- **进度**:**已完成(完整版)**。`MaybeCompact` 重构为阶梯:
  - **非 LLM rung**(`compaction_ladder.go` 的 `trimForBudget`):对 keep-recent 窗口**之前**的旧消息,把超长 tool-call args 换成合法 JSON 占位(`{"_trimmed":…}`,strict provider 重放仍接受)、超长旧 tool-result body 换 head+tail 预览。copy-on-write 不改输入。
  - **跳过 LLM**:trim 后若已 `!shouldCompact` 则直接 replace、`Compacted=false` 返回 —— 无 boundary 事件、不触发 extraction(那是要省的 LLM 调用)。**理由**:trim 只改 messages store(LLM 上下文),不动 UI transcript(A2 已确立 UI 全文/LLM 精简),不掉消息、对用户不可见,故不报 boundary 是诚实的。
  - **有损但明标**:trim 标记为 `…trimmed on compaction; not retrievable…`,**刻意不引用 read_tool_result**,与 A2 的可读回 offload 区分(A2=fresh/宽度→可读回;A3=old/深度→有损)。
  - **LLM rung 不变**:仍走原 safe-cutoff(不拆 tool 对)+ summary,且 summarise 用**原始** older(summariser 自带 4000 cap,比存储 trim 看得更全)。
  - `capText` 抽出共享 `headTail(s,limit,marker)` 原语(包内 DRY,summary cap 与 ladder trim 各给 marker)。
  - **测试**:deterministic 路径(token 触发→trim 达标→跳过 LLM、0 次 Call、不掉消息、旧 body 预览、recent 全文、pairing 保全);trim 不够→仍走 LLM;`trimForBudget` 单测(args 合法 JSON、旧 result 预览、recent 不动、输入不被 mutate)。全绿。

---

### B4 · 受治理的技能自著述(draft → 扫描 → HITL 晋升门 → 审计 → curator)

- **来源/为什么**:AgentScope"技能自演化"**经代理打假**:并非自动挖掘成功轨迹,而是 **LLM 主动调工具写 markdown**(`propose_skill`/`skill_manage`),其自动合并回路(`runUmbrellaDryRunReport`)是 **DRY_RUN 空壳**。真正值钱的是外面那圈**治理管线**:草稿 `skills/_drafts/` → 静态安全扫描(regex 规则,自认"非安全边界,只挡低级错误")→ 晋升门(默认 `RejectAllGate` = 必须人审)→ 审计日志 → curator 生命周期(ACTIVE→STALE→ARCHIVED,只归档不删)+ **渐进披露加载**(prompt 只放 name+desc 目录,正文按需 load)。
- **目标**:让 agent 能**提议**新 skill,经**人审门 + 审计 + 可回滚**后才进只读 skill 仓 —— agent 能力可沉淀,但绝不自我污染。
- **落点**(**关键:全落在 lynx 已有积木上,能比 AgentScope 更干净**):
  - `skills` 模块:目前是**只读** SKILL.md 仓(`Source: List/Load/LoadResource`)。新增"草稿区 + 晋升"是它的自然扩展。
  - `app/runtime` approval domain:**晋升门 = 现有 approval/HITL**(默认拒绝、人工确认),不新造 gate 机制。
  - `app/runtime` 文件 checkpoint(shadow-git):**审计/回滚** = 现有 checkpoint。
  - `tools`:一个 `propose_skill` 工具(typed struct → schema),写草稿。
- **计划**:
  1. skills 加草稿区(`_drafts/`)与"晋升"操作(move draft → 正式仓 + 记审计)。
  2. `propose_skill` 工具:agent 提交 name+desc+body(+可选 scripts)→ 落草稿。
  3. 晋升复用 approval:草稿晋升是一个需人工确认的动作(默认拒绝);确认后才可见、可加载。
  4. 加载走渐进披露(若 skills 目前已是"目录进 prompt、正文按需 load"则确认即可,否则补齐)。
  5. 可选静态扫描:一个保守 regex 扫描器,DANGEROUS 直接回滚草稿(**明确不是安全边界**,只挡低级错误)。
- **验收**:agent 能提议 skill;未经确认不进正式仓、不进 prompt;确认后可加载;有审计记录、可回滚;全绿。**不实现**自动合并/自主晋升(AgentScope 自己都是 stub)。
- **风险/边界**:必须 **opt-in + HITL-gated**;绝不做"agent 自主写入即生效"。安全扫描只当便利,不当信任边界(真隔离靠 C7 沙箱或外层容器)。这是**新特性**,需你确认方向再动。
- **优先级**:P2(最有新意;体量中等;需产品方向确认)。
- **进度**:**Batch 1(核心自著述)已完成**(用户定"完整版")。落地:
  - **domain/skills 著述模型**(纯):`Draft{Name,Description,Body}` + `Validate`(复用 skillspec frontmatter 规则,晋升物必过只读 loader)+ `Render`(yaml-safe frontmatter+body)+ `Scan`(保守静态、**明确非安全边界**,拦 rm-rf 根/家、--no-preserve-root、fork bomb、curl|sh、设备擦除)。
  - **infra/skillauthoring**(skills 树的唯一 writer):草稿落 `<root>/_drafts/<name>/SKILL.md`(`_drafts` 非法名 → 只读 source 天然跳过、agent 不可见);`Promote` 移入正式仓,`DiscardDraft` 删。skills 模块保持只读。
  - **toolset/skillpropose** `propose_skill` 工具:Validate → Scan → SaveDraft → **HITL Approve/Reject**(复用 exit_plan_mode 的 QuestionInterrupt)→ Promote/Discard。分类 Safe(真门是人审);coding/root role only(子任务不著述);disabled store → 省略工具。
  - 接线 toolset.Build/resolver/bootstrap over 全局 skills 目录。测试:store 草稿→晋升→可发现/晋升缺失报错/丢弃/非法名拒;工具 approve→晋升、reject→丢弃、非法/危险 gate 前拒。全绿、已推送。
  - **渐进披露**:已是 tool-driven(`skill` op=list 取目录、op=load 取正文),无需改。
  - **Batch 2(管理面)已完成**:curator 生命周期 **ACTIVE↔ARCHIVED**(目录式 `_archive/`,只移不删,可 restore;infra archive/restore/list + workspace coordinator + `workspace.skills.list/archive/restore` RPC + wire golden Go/TS + 前端 "Skill Library" 视图,skills.changed 失效刷新)。全栈全绿。**有据不做**:STALE 中间态(目录式与 ARCHIVED 等价 / "加载但打标"需给只读加载路径引入 state 耦合);专用审计日志(企业治理面,单用户桌面过度范围,HITL 已 durable 记录人审)。
- **风险/边界**:必须 **opt-in + HITL-gated**;绝不做"agent 自主写入即生效"(Batch 1 已守)。安全扫描只当便利,不当信任边界(真隔离靠 C7 沙箱或外层容器)。

---

### B5 · bypass-immune 工具自否决(安全精修)

- **来源/为什么**:AgentScope 权限引擎 6 步优先级里,**工具自检(step 3)排在 BYPASS 全放行(step 5)之前** —— 即使处于"全自动批准"模式,工具对某类操作仍能**硬否决**,BYPASS 绕不过。
- **目标**:lyra 即便在"跳过审批"模式下,某些工具操作(如破坏性/敏感)仍能被工具自身或策略强制要求确认/拒绝,不可被全局 bypass 绕过。
- **落点**:`tools`(工具声明一个"不可自动批准"位)+ `app/runtime` approval domain(评估时:工具自否决优先于任何 allow/bypass 规则)。
- **计划**:
  1. 给 tool/approval 加"un-bypassable"语义(工具级标志或策略钩子)。
  2. approval 评估顺序:工具硬否决/强制确认 **先于** 全局 allow/bypass。
  3. 测试:bypass 模式下,标记为不可绕过的工具调用仍需确认/被拒。
- **验收**:bypass 下不可绕过工具的行为符合预期;普通工具在 bypass 下照常放行;全绿。
- **风险/边界**:lyra approval 是 **GLOBAL** 且刻意无 per-run 模式(见 [[project_entry_points_backlog]] 的既有决定);本项只加"工具级不可绕过",**不**引入 per-run/per-user 审批粒度。
- **优先级**:P2(小、安全相关)。
- **进度**:**已完成**。核查确认前提成立且有具体消费者:**工作区外的文件改动**(write/edit/apply_patch/download 到 cwd 之外)当前完全无守卫,Yolo 无提示、Balanced 甚至自动放行(fs 不 jail)。落地:
  - `tool.MutatesOutsideWorkspace(name, args, cwd)` 纯函数(域内):按工具名取 path 参数、解析相对/绝对/`~`、判目标是否逃出 cwd;保守取向(`~`/不可解析/无法相对化都算"外部",fail toward asking)。shell 不纳入(命令行非单一目标,留给未来命令分类器)。
  - `approval.ToolCallInput.Cwd` + `Plan()` 升级:`GatePass` 且是工作区外改动 → 升为 `GatePrompt`(高风险 + "targets a path outside the workspace directory")。复用与 PreToolUse hook `Ask` 同一条"even-in-Yolo 强制提示"缝,但改为**内建、工具/参数驱动**。记住(remember)后续同调用仍可放行(不破坏审批规则)。
  - `turnObserver.ApproveToolCall` 传入 `Cwd: t.st.cwd`(已握有)。
  - **边界**:仅在审批已配置时升级(无审批系统=无 HITL 可问,保持放行);**只加工具级不可绕过,不引入 per-run/per-user 粒度**(遵循既有 GLOBAL approval 决定 [[project_entry_points_backlog]])。
  - **测试**:`MutatesOutsideWorkspace` 12 例(内/外/`~`/相对/`..`逃逸/非改动工具/shell/空 cwd/坏 JSON);Plan 升级(Yolo+外部→prompt 高风险;Yolo+内部→pass;无 cwd→pass)。全绿。
  - **⚠ 行为变更(供你复核)**:这让 Yolo/Balanced 对"工作区外文件改动"新增一次确认(可 remember 后免)。若你希望 Yolo 保持"绝对零提示",告诉我,我改为可配置或收窄。

---

### B6 · 超时不丢活(timeout adoption)

- **来源/为什么**:AgentScope `execWithTimeoutPromotion` —— 同步 spawn 子代理若超时,**不取消**在飞的运行,而是登记为后台任务(`AdoptedTaskRunSpec`),把 `task_id` 交回父,后续 `task_output` 轮询。工作永不因超时被丢。
- **目标**:lyra 的同步子任务/工具调用超时后,把在飞运行**收养为后台任务**并交回 handle,而非取消丢弃。
- **落点**:`agent` 子进程模型(`RunChild` 同步 + `child_async`)+ `app/runtime` 后台任务(已有 background commands 模型)。
- **计划**:
  1. 同步子任务加"超时收养"路径:超时 → 不 cancel,转登记为后台任务、返回 handle。
  2. 复用现有后台任务/轮询协议暴露被收养任务的输出。
- **验收**:超时的同步子任务转为可查询的后台任务且结果不丢;正常完成路径不受影响;全绿。
- **风险/边界**:注意与现有取消/预算/生命周期语义的交互(收养的任务仍受父预算树 / 关闭级联约束);别让"收养"泄漏成永不回收的孤儿 goroutine。
- **优先级**:P2。
- **进度**:**不做(前提不成立)**。核查(2026-07-18)结论:lyra 的 `task` 委派虽是同步阻塞,但**根本没有子任务超时** —— 没有任何 deadline 会取消在飞子任务(`toolloop` 无 per-tool 超时;唯一的 `llmIdleTimeout` 是模型流空闲超时,非墙钟;`WithTimeout` 只用于 teardown,基于 `WithoutCancel`)。所以"同步 spawn 超时→取消丢活"这个 bug **在 lyra 不存在**,没有可修的东西。要做就是**新增**一套子任务超时+后台化(SDK 已有 `StartChild` 异步原语但 lyra 的 task 没用;shell 已有"超时转后台从不丢活"的范式可仿),属新增能力而非修复。**未观察到"长子任务被误杀"的实际问题 → YAGNI,暂缓**,等真出现再按 shell auto-background 范式做。

### C7 · 可快照恢复的沙箱 Executor

- **来源/为什么**:AgentScope `Sandbox` SPI 的可搬设计:**`stop()`=只存快照 / `shutdown()`=销毁后端**的清晰拆分;快照打 **tar** 传对象存储(快照只持久化 id,client 在 resume 时重注入);可插拔文件系统 overlay(local/sandbox/remote)。per-vendor 驱动(Docker CLI/K8s/E2B)是 commodity,不必抄。
- **目标**:当 lyra 需要安全执行不可信代码(编码 agent)时,有一个干净的沙箱 Executor 后端形状可依。
- **落点**:`tools` 的 `Executor` SPI(远程/沙箱后端本就是计划内的 Executor 实现)+ `app/runtime/infra`(进程执行/存储)。
- **实现**:
  - Executor 沙箱后端契约:`stop`(快照)/`shutdown`(销毁)拆分;文件系统 overlay 是独立工作副本,快照以 tar 进入现有持久化层。
  - `infra/sandbox.Workspace` 实现 `tools/shell.Executor`:创建独立工作副本;macOS 以 Seatbelt 禁网络、禁工作区外写、隐藏宿主 HOME、清洗环境变量;不支持的平台明确 `ErrUnavailable`,绝不静默退化成本地执行。
  - `Stop` 等待在飞命令、生成确定性 tar、以 sha256 内容寻址写 SQLite;重复 stop 返回同一 id。`Resume` 重新注入 store/runner,校验摘要后恢复到新后端;`Shutdown` 幂等销毁临时工作副本但保留快照。
  - tar 信任边界拒绝绝对路径/`..`/越界 symlink/设备与特殊文件/重复路径,并限制单文件、总大小和条目数;恢复使用 `os.Root` 防目录穿越。
- **验收**:快照确定性 + stop/resume/shutdown 生命周期 + SQLite 不可变去重 + 摘要损坏/缺失 + tar traversal/symlink/device + Seatbelt 工作区内写/工作区外写/HOME 读取/env 清洗测试全绿。
- **风险/边界**:当前交付一个真实 macOS 后端,不是多租户安全承诺;Linux/Windows 必须新增各自 fail-closed jail 后端后才能启用。分布式租约仍不吸(单进程)。
- **优先级**:C。
- **进度**:**已完成**。

---

### C8 · 每日账本 → watermark 策展 MEMORY.md 的跨会话持久记忆(可选)

- **来源/为什么**:AgentScope 四层记忆里的两级策展:append-only 每日账本 `memory/YYYY-MM-DD.md`(LLM 抽取事实/决策/偏好,去重)→ watermark 门控地合并进策展 `MEMORY.md`(token 上限,整文件覆盖,推进 watermark)。给 agent 干净的跨会话长期记忆。
- **目标**:若 lyra 要给 agent 长期记忆,提供"每日账本 → 策展 MEMORY.md"的两级持久记忆(区别于原始会话日志)。
- **落点**:`app/runtime` + `chatmemory`(注意:lyra 已有用户可编辑的 `LYRA.md` memory —— 本项是 agent 自策展的记忆,需想清与 LYRA.md 的关系,别重叠)。
- **计划**(**暂不实现,记录设计**):账本 append + 定期 LLM 策展入 MEMORY.md + watermark。
- **验收**:N/A(设计记录)。
- **风险/边界**:与现有 LYRA.md(用户可编辑)边界要划清;属产品特性,需方向确认。
- **优先级**:C(可选 / 延后)。
- **进度**:未开始。

---

## 2. 刻意不吸(附因 —— 未来会话勿重新论证)

| 项 | 为什么不吸 |
|---|---|
| Project Reactor 全响应式 | lyra 用 `iter.Seq2` + `context.Context`;不引响应式(AgentScope 自身 SKILL.md 大篇幅纠正 `.block()` 误用 = 成本反证) |
| "一个内核跑 stream+blocking" | **已有**:`agent/toolloop.Runner.Run` 返回 `iter.Seq2[Event,error]`,阻塞=drain 到终态,天然不分叉 |
| per-(user,session) 权限隔离 | lyra 刻意选 **GLOBAL approval**、无 per-run(见 [[project_entry_points_backlog]]);单用户桌面不需要,转多用户才重估 |
| 无状态水平扩展 + 跨副本子代理注册表 + reload-per-call | lyra 单进程 in-house、协议层零 user;超范围。其 `cur_iter` 重入计数器 lyra 反而更强(toolloop checkpoint 在 pending call 处精确重入 > 一个计数器) |
| Spring Boot starters / 新旧 Hook+Middleware 并存 | lyra 显式装配、不留双机制迁移债 |
| IM 渠道(钉钉/飞书/企微)/ AG-UI 若无需求 | app 集成,非 runtime SDK 职责 |

---

## 3. 已有、无需吸(核对基线)

lyra 已具备且不弱于 AgentScope,列此避免重复实现:HITL 在 pending call 精确重入([[project_hitl_resume_at_pending_call]])、持久审批规则([[project_t1_t2_design_decisions]])、file checkpoints(gated whole-repo shadow-git)、read-only skills 仓、plan mode、scheduler(cron)、事件流(stable runId/segmentId,[[project_lyra_run_segment_identity]])、middleware + tool.Config 钩子、MCP/A2A、durable 快照恢复([[project_durable_resume_design]])。

---

## 4. 进度看板

### 4.1 项目状态

| 项 | 目标一句话 | 优先级 | 落点 | 状态 | 阻塞 |
|---|---|---|---|---:|---|
| A1 | 压缩不拆 tool-call/result 对 | **P0** | app/runtime compaction | ✅ 已完成(核查未中招 + 属性测试锁死) | 无 |
| A2 | 单条超大 tool-result 落盘 eviction | P1 | app/runtime agentexec/toolset + sqlite | ✅ 已完成(SQLite 持久化 + read_tool_result) | 无 |
| A3 | 分级压缩阶梯(先非 LLM) | P1 | app/runtime compaction | ✅ 已完成(完整版:trim 阶梯 + 跳过 LLM) | 无 |
| B4 | 受治理技能自著述(HITL-gated) | P2 | skills + approval + tools + delivery + 前端 | ✅ 完成(propose_skill 人审晋升 + Skill Library 管理视图,端到端) | 无 |
| B5 | bypass-immune 工具自否决 | P2 | tool 域 + approval | ✅ 已完成(工作区外改动 bypass-immune) | 无 |
| B6 | 超时收养为后台任务 | P2 | agent child + app/runtime bg | ⛔ 不做(前提不成立:无子任务超时) | — |
| C7 | 可快照恢复的沙箱 Executor | C | tools.Executor + infra/sqlite | ✅ 已完成(macOS Seatbelt + tar/SQLite resume) | 无 |
| C8 | 跨会话策展记忆(设计参考) | C | app/runtime + chatmemory | 延后 | 与 LYRA.md 边界待定 |

### 4.2 当前执行指针

```text
Current item: C8 — 每日账本 → watermark 策展跨会话记忆。
Last completed code checkpoint: C7 可快照恢复的沙箱 Executor(macOS Seatbelt + deterministic tar + SQLite snapshots)
```

每次开始一项 / 提交后,更新此指针与 §4.1 状态,勿只改历史。

## 5. 验证门禁

每项提交前,在受影响模块执行(参照根 `CLAUDE.md` 与本模块约定):

```bash
gofmt -w <changed-go-files>
go build ./... && go vet ./... && go test ./...     # 在每个受影响 module 内
# 触及 core 公开面/wire → core/internal/arch 的 TestExportedAPIMatchesBaseline / TestWire*
# 触及跨包移动 → 按 go.work 构建全部受影响 module
```

破坏性公开 API / 协议 / wire / schema 变更:先按第一法则给 scope+影响面+备选咨询,评审后才 `-update-*` 重生基线。B4/B5/B6/C7/C8 属新特性/新公开面,开工前先与用户对齐方向。
