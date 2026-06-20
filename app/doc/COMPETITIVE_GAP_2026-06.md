# 竞品差距清单 / 落地 backlog — 2026-06-20

> 基于对 10 个桌面 AI Agent 应用 + 我们 `app/`(runtime + desktop)的一轮横向分析(claude_code / codex / opencode / kimi-code / crush / cline / continue / goose / plandex + tier-3 sweep)。
> 本文件是**可执行清单**,按难度分 T1<T2<T3,从低到高逐项做,每项一个独立 commit、做完全绿。
> 设计哲学/反不变量见根 `CLAUDE.md`;**明确不抄**的方向见文末。

## 执行状态 — 2026-06-20 自主一轮(全部 commit+push,逐项全绿)
**T1 全部完成(4/4):**
- T1.1 ✅ composer token+成本 chip(`RunState.usage`,删死字段 `cost`)
- T1.2 ✅ steer 乐观气泡(`useChatSend` mint `local-steer-*` + `dropMessage` 回退)
- T1.3 ✅ 纯三选 Restore(`restoreCheckpoint` + MessageContextMenu 子菜单)
- T1.4 ✅ 修 per-run model 重启丢失(interrupt 持久化 provider+model → Rehydrate 重解析 client)

**T2:全部 9 项收口(7 项做完、2 项设计性非差距):**
- T2.0 ✅ context 占用条(后端 `OnUsage` +contextTokens → run.progress;chip 显示 `N%` 染色)
- T2.1 ✅ 模糊编辑回退(`fs/editmatch.go`,whitespace-tolerant + 歧义拒绝)
- T2.6 ✅ diff 按文件语言高亮(`langFromPath`;DiffView 原本已 Shiki,只是写死 TS)
- T2.7 ✅ loop 渐进提醒(SDK `tool` 中间件:第3轮注入 `<system-reminder>`,第6轮硬停)
- T2.8 ✅ history 富化,分三块结账:① fuzzy 搜索 ✅(侧栏会话筛选)② **收藏/置顶** ✅(first-class `session.favorite`:sqlite 列 + wire + `sessions.update` toggle;侧栏右键 Pin/Unpin + accent 星标 + 组内置顶排序,乐观更新)③ 每会话成本展示 + 按成本排序 ⊘ **不做(代价不成比例 + 撞设计)**:per-run 成本已由 composer chip 实时显示;每会话**累计**成本 `Session.Usage` 从未被填充,要新建用量聚合子系统(run-finalize denormalize + session usage 列 + rollback/fork 递减语义),却只为往刻意精简的会话行塞个数字;排序还要侧栏没有的排序 UI,价值边际。同 T2.4/T2.5 的 HistoryView 式分歧。
- T2.4 ⊘ **设计性非差距**:我们 `@file` 是引用式(插路径,agentic 模型用工具按需读),cline 的 @problems/@diff/@terminal 是内容内联——对 tool-using 模型会胀 prompt 且重复工具能力,且内联易过期。判定为有意分歧(同 provider-OAuth 不抄)。
- T2.5 ⊘ **已覆盖**:`MessageStream` 已用 `use-stick-to-bottom`(跟随+让位+resize+jump 按钮)。唯一缺的"滚过用户消息吸顶 header"是边际 polish,不值得加复杂度。
- T2.2 ✅ **重框为真省钱杠杆**:不是另起微压缩(我们已有主动 compaction),真差是 **Anthropic 历史零 prompt 缓存**。改为在 anthropic adapter 自动放 `cache_control` 断点(tools+system 静态前缀 + 对话尾巴滚动断点);**仅对带 tools 的请求**(一次性 utility 调用不缓存,免 +25% 写费)。缓存读 ~10% 价。
- T2.3 ✅ **拆解后:fast=已有 maintenance、planner/auto-continue=YAGNI、fallback=撞 no-retry**;真差只在 UI。落地=① 把 maintenance model 角色重命名为 `utility`(破坏性 config 改动,已咨询);② runtime-mutable + 持久化(单行 `utility_role` 表 + atomic cell + `models.get/setUtilityRole` RPC + Providers 面板 picker)。维护服务改为 per-call `ClientFunc` 解析,设置改动下个 turn 边界生效、无需重启。


## 基线纠正(读代码核实,记忆曾漂移)
- MCP **已有完整 OAuth 2.1**(DCR+PKCE+loopback),非"bearer no-OAuth"。
- **Planner 不存在**(配置注释是死的);maintenance 双模型(compaction/title/extract)在。
- 桌面**状态栏已删**(intentional)。
- **checkpoint 后端完整**(shadow-git 整库 + `sessions.rollback`),**前端无 UI**。
- **token usage 已在 wire**(`RunResult.Usage` / `UsageReported` 事件 / provider `maxInputTokens`)。
- **重启 resume 丢 per-run model**(已知 bug)。

---

## T1 — 低难度(后端已就绪,纯前端 surface / contained 修复)

> 共同点:"发动机造好了没接方向盘"。风险低,逐项可独立 ship。

- [ ] **T1.1 context 预算条 + token/成本**(frontend)
  - 数据已在:`protocol/items.go` `Usage`/`ModelUsage`、`turn/observer.go` `OnUsage`→`UsageReported`、`RunResult.Usage`、`providers` `maxInputTokens`。
  - 做:任务头/footer 一条 context 占用进度条(`used/max`,>80% 变色)+ in/out/cache token + 成本;hover 明细。
  - 参考:cline `task-header/ContextWindow.tsx`、kimi footer `context: %`、crush sidebar context bar。

- [ ] **T1.2 steer 队列 UI**(frontend)
  - 后端在:`runs.steer`(`BeforeRound` hook 注入,真 mid-run)。基线明确"无 client steer-while-running buffer UI"。
  - 做:运行中输入→可见队列 pill(`▶▶▶`/计数)+ 两段式 ESC(第一下清队列、第二下取消 run);经 `runs.steer` flush。
  - 参考:crush queue pill + 2-press ESC、claude_code 队列即 steering、codex Tab 入队。

- [ ] **T1.3 checkpoint / rollback UI**(frontend;后端最完整,T1 里最大)
  - 后端在:shadow-git 整库 checkpoint(`chk/<runID>` tag)、`sessions.rollback`(restoreType history/files/both)、`workspace.listFileChanges`/`getDiff`。前端只有死 block。
  - 做:时间线内联 checkpoint 标记 + hover Restore/Compare;Restore 弹窗三选(Files / Conversation / Both);Compare 走 `getDiff`。
  - 参考:cline `CheckmarkControl.tsx` 三选恢复 + Compare。

- [ ] **T1.4 修 per-run model 重启丢失**(backend;contained,但缠 durable snapshot/core 层 —— 若需动 core/schema 则标记、留给有人时再动)
  - 现状:中断快照不带 run 的 model;resume 续跑用 session/default model。
  - 治本:快照(或 park/interrupt 记录)带 run model,resume 重解析同一 per-run client。
  - 注意:`core.ln` 快照类型在更低层;先评估爆炸半径。

---

## T2 — 中难度(收敛验证的后端能力;"有但做得弱")

- [ ] **T2.1 模糊编辑回退**(backend)—— 我们 `edit` 仅精确匹配,一击不中即失败(agent 体感"老失败"根因)。多级回退:exact→trim-line→block-anchor→whitespace/缩进归一,歧义则拒(强制模型消歧)。参考:opencode 9级、cline 3级、continue layered。
- [ ] **T2.2 微压缩 / tool_result 增量剪枝**(backend)—— 每轮便宜剪旧工具输出、不触发全量摘要(claude_code 还能 cache-editing 不毁 prompt cache)。最高 ROI 省钱杠杆。参考:claude_code `apiMicrocompact`、kimi micro、opencode prune、goose middle-out。
- [ ] **T2.3 多模型角色扩展**(backend)—— 现有 main+maintenance;扩到 fast(命名/摘要/auto-continue 走小模型)+ 可选 planner;加大上下文/大输出/错误 fallback。参考:plandex 9角色、goose fast_model+planner、crush large/small。
- [ ] **T2.4 @-mentions 广度**(frontend)—— 现只 @file;加 @folder/@url/@problems/@diff/@terminal。参考:cline/continue context providers。
- [ ] **T2.5 流式自动滚动控制器**(frontend)—— 跟随但用户上滚即让位 + partial→complete 高度补偿 + "已滚过用户消息"吸顶。参考:cline `useScrollBehavior`、continue `useAutoScroll`。
- [ ] **T2.6 语法高亮叠在 diff 红绿底上**(frontend)—— Shiki token 色叠 +/- 背景,宽屏 split / 窄屏 unified。参考:crush `xchroma`、cline。
- [ ] **T2.7 loop 检测渐进提醒**(backend)—— 现"6 轮相同硬停";改升级提醒(提醒/报告/死路/强停)。参考:kimi tool-dedup、crush 签名计数。
- [ ] **T2.8 cost/token per-run 展示 + history 富化**(frontend)—— $ pill、↑↓cache;history fuzzy 搜索 + 按成本/token 排序 + 收藏。参考:cline `HistoryView`。

---

## T3 — 大注(平台化 / 架构级;涉及 schema/公开 API —— 先出方案,落地前与用户对齐)

- [ ] **T3.1 生命周期 hooks**(用户可脚本化 PreToolUse/PostToolUse/SessionStart/PreCompact…)—— 最大扩展性缺口,把 runtime 变平台。参考:claude_code(28事件×4类型)、goose、kimi(16)、continue、crush、codex。
- [ ] **T3.2 代码库语义检索 @codebase**—— 内容寻址+分支感知索引(embeddings + sqlite FTS5/BM25 + repo-map 让模型选文件 + 最近编辑加权 + reranker)。参考:continue `core/indexing`。
- [ ] **T3.3 Recipes(参数化工作流)+ Scheduler/cron**—— typed 参数 + 模板 + 结构化 response + 定时;skills 之上的产品面。参考:goose `recipe/`+`scheduler.rs`、AionUi 会话绑定 cron、kimi in-conv cron。
- [ ] **T3.4 OS 级 sandbox**(seatbelt/landlock/bwrap)+ 沙箱内自动放行 —— 同时更安全+更少打扰。参考:codex `sandboxing/`、claude_code。
- [ ] **T3.5 新颖安全/自治**(择优):prompt-injection/外泄扫描器(goose `security/`)、judge-model 停止条件(MiMo `/goal`、kimi)、tentative-changes 沙箱(plandex 改动先进 sandbox、逐文件 review 后 apply)、self-distill(挖 trace→打包 skill;我们已有 Extractor→LYRA.md 的半成品)。

---

## 我们已持平/领先(别误判成差距)
LSP 深度(且 edit 结果已折入诊断,editguard decorator)、MCP OAuth(比 crush/continue 广)、shadow-git 整库 checkpoint(比 claude_code 内容快照/goose fork 强,只差前端)、持久化细粒度审批规则(多数竞品仅 session-only)、工具组折叠、@file、记忆抽取→LYRA.md、durable cross-restart resume、multimodal image。

## 明确不抄(撞已定法则)
- Sleep 工具(选了 `bash_output` block)。
- retry layer(SDK 内置够)。
- provider OAuth / token refresh(用户填 key;撞反不变量)。
- 结构化输出 converter 链(已 closed,Reasoning first-class)。
- shadow-git 退回 edited-files-only。
- > 旁注:codex/opencode/goose 都做**协议→TS codegen + drift 测试**,撞我们"评审保持同步、不 codegen"的决策(Go 扁平 struct 映射不到 TS 判别联合)。被多家验证,**值得重评**,但属已知取舍,非疏漏。
