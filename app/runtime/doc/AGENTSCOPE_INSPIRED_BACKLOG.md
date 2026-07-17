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
- **进度**:未开始。

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
- **进度**:未开始。

---

### A3 · 分级压缩阶梯(先非 LLM,再 LLM)

- **来源/为什么**:AgentScope compaction 是 6 步阶梯:① 非 LLM 截断超长 tool-args → ② 非 LLM 修剪旧 tool-result(保留最近 N token,旧的换 head+tail)→ ③ 安全 cutoff(=A1)→ ④ flush 记忆 → ⑤ 落盘全量 → ⑥ **仅在仍超时**才一次 LLM summary,且 summary 里**嵌入落盘文件路径**供回溯。先便宜后昂贵,省 token、更稳。
- **目标**:lyra 压缩优先用零成本的确定性裁剪,LLM summary 作为最后一步,并让 summary 可回指全量历史。
- **落点**:`app/runtime` 压缩逻辑(与 A1 同一组件)。
- **计划**:
  1. 在现有压缩前置两步非 LLM 裁剪(截断超长 args、修剪旧 result)。
  2. summary 步仅在裁剪后仍超预算时触发;summary 文本内嵌 A2/⑤ 的落盘路径。
  3. summary 消息打稳定标记(如 `__compaction_summary__`),避免后续轮次把它当普通历史再归档。
- **验收**:能构造"裁剪即达标、不触发 LLM"的用例;summary 路径可被 read 工具取回;全绿。
- **风险/边界**:依赖 A1(安全 cutoff)、A2(落盘)先到位;三者是一组,建议同批实现。
- **优先级**:P1(与 A1/A2 同批)。
- **进度**:未开始。

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
- **进度**:未开始。

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
- **进度**:未开始。

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
- **进度**:未开始。

---

### C7 · 沙箱 Executor SPI 形状(记为参考,场景到了再落)

- **来源/为什么**:AgentScope `Sandbox` SPI 的可搬设计:**`stop()`=只存快照 / `shutdown()`=销毁后端**的清晰拆分;快照打 **tar** 传对象存储(快照只持久化 id,client 在 resume 时重注入);可插拔文件系统 overlay(local/sandbox/remote)。per-vendor 驱动(Docker CLI/K8s/E2B)是 commodity,不必抄。
- **目标**:当 lyra 需要安全执行不可信代码(编码 agent)时,有一个干净的沙箱 Executor 后端形状可依。
- **落点**:`tools` 的 `Executor` SPI(远程/沙箱后端本就是计划内的 Executor 实现)+ `app/runtime/infra`(进程执行/存储)。
- **计划**(**暂不实现,仅记录设计**):
  - Executor 沙箱后端契约:`stop`(快照)/`shutdown`(销毁)拆分;快照 = tar → 现有存储;文件系统 overlay(工作副本 + 远程镜像)。
  - 具体驱动(本地子进程 / 容器)按需再加。
- **验收**:N/A(设计记录)。真要做时:一个沙箱 Executor 后端 + 快照/resume 测试。
- **风险/边界**:agent-self-sandbox 在能力评估里被标 **RE-EVALUATE(安全≠多租户)**(见 [[project_agent_capability_gap_2026_07]]);lyra 现状是 shell 不 jail、信任调用方、靠外层容器隔离。**本项 DEFER**,等"真要跑不可信代码"的场景确立再启。分布式租约(Redis SET NX)那套跳过(单进程)。
- **优先级**:C(记录 / 延后)。
- **进度**:未开始(设计参考)。

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
| A1 | 压缩不拆 tool-call/result 对 | **P0** | app/runtime compaction | 未开始(先核查) | 无 |
| A2 | 单条超大 tool-result 落盘 eviction | P1 | app/runtime agentexec/toolset | 未开始 | 建议随 A1/A3 同批 |
| A3 | 分级压缩阶梯(先非 LLM) | P1 | app/runtime compaction | 未开始 | 依赖 A1/A2 |
| B4 | 受治理技能自著述(HITL-gated) | P2 | skills + approval + checkpoint + tools | 未开始 | 需产品方向确认 |
| B5 | bypass-immune 工具自否决 | P2 | tools + approval | 未开始 | 无 |
| B6 | 超时收养为后台任务 | P2 | agent child + app/runtime bg | 未开始 | 无 |
| C7 | 沙箱 Executor SPI(设计参考) | C | tools.Executor + infra | 延后 | 场景未确立 |
| C8 | 跨会话策展记忆(设计参考) | C | app/runtime + chatmemory | 延后 | 与 LYRA.md 边界待定 |

### 4.2 当前执行指针

```text
Current item: none (全部提案态,待满血上下文开工)
Recommended first: A1(核查 lyra compaction 是否会拆 tool 对 —— 若中招是真 bug)
Then: A1+A2+A3 同批(压缩健壮性一组)
Last completed code checkpoint: (开工后填)
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
