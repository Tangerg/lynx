# Plandex 架构剖析 —— 对 lyra 的启发

> **日期**：2026-06-12。**对象**：`plandex`（`/Users/tangerg/Desktop/plandex`，Go，plan-first 终端 AI 编码 agent，v2.2.1）。
> **目的**：第一手读 plandex server 端编排核心，判断它的架构哪些是债、哪些优雅，给 lyra 提炼可落地的架构启发。
> **结论先行**：**lyra 在大多数架构维度上已经比 plandex 干净**（分层纪律 / 无 god object / tool-calling vs 脆弱文本解析 / OTel vs `log.Printf`）。plandex 真正值得偷师的只有 **「Role 即配置」** 一个设计形状；外加一条 lyra 已在贯彻、值得显式固化的原则「派生态不二存」。
> **配套**：能力面对比见 [`AGENT_CAPABILITY_COMPARISON.md`](AGENT_CAPABILITY_COMPARISON.md)（已含 plandex 列 + §2 复盘追加）。本文只谈**架构**。

---

## 1. plandex 架构怎么搭的

**3 个 Go module**：`cli` / `server` / `shared`。`shared` 用 `tygo`（`shared/tygo.yaml`）把 Go struct 生成 TS 类型给前端——与 lyra **刻意不做 protocol→TS codegen**（Go flat-struct 不映射 TS 判别联合，靠 review 同步）相反。

**server 端分层**：

```
handlers/        HTTP 入口（plans_*.go / stream_helper.go / context_helper.go …）
   │
   ▼
model/plan/      编排核心 —— 34 个文件的扁平包（tell_* / build_* / exec_status …）
   │                · tell_*  = planning + implementation 的流式驱动
   │                · build_* = builder 模型把描述变成结构化编辑（沙箱）
   ▼
db/              Postgres + 每个 plan 一个 git repo（ExecRepoOperation 加锁）
model/           LLM client（client.go / litellm.go / summarize.go）

旁路：
types/           内存运行态（ActivePlan / SafeMap）
model/prompts/   所有 prompt 按 role/stage 集中
syntax/          tree-sitter（codebase map + 结构化编辑应用）
```

### 三个支柱机制

**① 状态机 = 从对话历史反推的纯函数**（`model/plan/tell_stage.go:21` `resolveCurrentStage`）

plandex **没有独立状态机表**。"当前在哪个阶段" 是读**最后一条成功 convo message 的 `Flags`** 算出来的：

```go
// 伪化自 tell_stage.go
lastMsg := state.lastSuccessfulConvoMessage()      // 跳过 Stopped / HasError
if isUserPrompt { tellStage = Planning } else if lastMsg.Flags.DidMakePlan { tellStage = Implementation } …
// Planning 内再分 Context / Tasks 两 phase
```

对话日志本身就是唯一真相，阶段是 `pure_fn(history)`。阶段流：`Planning(Context→Tasks) → Implementation(Subtask₁…Subtaskₙ)`。

**② ActivePlan：全局 SafeMap + 监督 goroutine**（`model/plan/state.go:17`、`types/active_plan.go`）

内存运行态对象，key=`planId|branch`，放全局 `SafeMap`。每个 ActivePlan 起一个 goroutine `select` 监 `Ctx.Done()` / `StreamDoneCh`，负责落 DB 状态（`SetPlanStatus`）+ 清理 + 错误广播。流式是**订阅广播**（多 CLI 客户端可订同一 plan）+ buffer + 70ms 限速 + 5s 心跳。文件操作走 `db.ExecRepoOperation` 的 **repo 写锁**（git-backed）。

**③ 自定义流式文本块解析器**（`model/plan/tell_stream_main.go:46` `chunkProcessor` + `types.ReplyParser`）

plandex **不用 tool calls**（tool call 只用于命名 / 完成校验）。它从 token 流里**增量解析 `<PlandexBlock>` 标签**，靠一堆状态 flag 维持：`fileOpen` / `awaitingBlockOpeningTag` / `awaitingBlockClosingTag` / `awaitingBackticks` / `maybeRedundantOpeningTagContent`…

---

## 2. 哪些是 plandex 的债，哪些是它的优雅

### ❌ 三个架构债 —— lyra 应庆幸没学，别学

| 债 | plandex 现状 | lyra 的对照（更优） |
|---|---|---|
| **God state struct** | `activeTellStreamState`（`tell_state.go:14`）**55 字段**：req / auth / modelConfig / convo / subtasks / parser / retry 计数 / 计时 全糊一起，教科书级 SRP 违反 | `ProcessContext` 靠**字段分区**治理；engine 把 compactor/extractor/planner **拆独立 service**。方向相反，lyra 对 |
| **自定义流文本解析** | 一堆 `awaiting*` flag 的标签状态机，脆、边界条件多 | **tool-calling loop**：结构化、provider 原生、不靠脆弱标签状态机 |
| **`log.Printf` 撒满** | `resolveCurrentStage` 一个函数十几行 debug log，零结构化可观测 | **OTel 三件套** + "观测优先 span 而非 slog 行" 纪律 |

> 一句话：**plandex 的"编排核心"是个低内聚大泥球**。lyra 的 `delivery → engine → service → infra` 单向分层 + ISP 窄接口在工程纪律上明显更成熟。**lyra 不需要从 plandex 学分层。**

### ✅ 两个真正优雅、值得偷师的点

**A.「Role 即配置对象」—— plandex 最干净的设计**（`shared/ai_models_roles.go` + `ModelRoleConfig`）

plandex 把每个内部角色（planner / builder / summarizer / auto-continue / commit-messages / names / whole-file-builder）建模成**一等配置数据**：

```go
// 形态（plandex shared）
type ModelRoleConfig struct {
    Role                 ModelRole   // planner / builder / summarizer …
    Model                ModelId
    Temperature, TopP    float32
    ReservedOutputTokens int
    Fallback             *ModelRoleConfig   // 可选降级链
}
```

流水线**读 role config 选模型+参数**，不是 `if role==X` 的代码分支。**角色是数据，不是代码路径** —— 加新角色 = 加一条配置，不改 dispatch（OCP）。

这正是 lyra per-role model 的**优雅落地形态**（见 §3）。是 plandex 唯一一个 lyra 该照搬"形状"的设计。

**B.「状态用算的，不用存的」—— lyra 已在做、值得显式命名的原则**

plandex 的 `resolveCurrentStage` = `pure_fn(history)`，单一真相源是日志。**lyra 其实更彻底地贯彻了这条**：

- Item store 是 history + streaming 的唯一 primitive；
- HITL resume **AT pending call**（从 blackboard + interrupt record 重建，不重跑 LLM）就是"状态算出来"的范例。

**但 plandex 自己破坏了这条** —— 它算出 stage 后**又缓存进 god struct 的 `currentStage` 字段**（`tell_state.go:49`），同一状态双存、可能漂移。

> **教训给 lyra**：派生态永远别二次存储。lyra 目前对齐得很好；这条是"保持 + 写进设计原则防回归"，不是"新增能力"。

---

## 3. 对 lyra「更优雅架构」的建议

排序后，真正能让 lyra 架构更优雅的动作只有一个半：

### 【该做】把 per-role model 设计成「Role 即配置」

偷 plandex 的**形状**，不偷它的 god struct。lyra 的 `maintenance` 域已把 **Compactor / Extractor / Planner** 拆成独立 service —— 它们正好是天然 role 边界。

落地骨架（复用现有 `core.ChatClientProvider` + `clientResolver` seam，不引入新机制）：

```go
// internal/service/maintenance（或更上层）—— 角色即数据
type RoleModel struct {
    Provider string   // 空 = 回退 session 的 per-run model
    Model    string
    // 可选: Temperature / TopP（缺省继承默认）
}

// 每个 maintenance service 构造时注入各自 binding；
// turn 内经现成 ChatClientProvider seam 按 (provider, model) 解析/缓存 client。
// 全空配置 ⇒ 行为与今天完全一致（全部走 session model）。
```

要点：
- **默认零配置不变行为**：binding 为空时全部回退 session 的 per-run model（YAGNI 友好，不强迫用户配）。
- **不新增 retry/fallback 链**：plandex 的 `Fallback *ModelRoleConfig` **不抄** —— lyra 已有「无 retry layer」决议（SDK 内部 retry 足够）。只取"role→model 绑定"这一层。
- **provider 不推断**：沿用 lyra 现有「显式 (provider, model) 配对，缺一即 invalid」纪律。
- 一步同时拿下「per-role model 能力」+「更优雅的角色抽象」。

### 【半个，确认即可】把「派生态不二存」写进 `../CLAUDE.md` 设计原则

lyra 已在做（Item 唯一真相、HITL resume-from-pending），plandex 反例正好佐证。低成本固化纪律，防未来回归。可挂在 §设计原则 的 "Go-specific" 或 "Make zero values useful" 附近，措辞如：

> **派生态不二存（derived state is computed, not stored）**：能从单一真相源（Item 日志 / blackboard / interrupt record）算出来的状态，永不二次缓存进可变字段。plandex `resolveCurrentStage` 算出 stage 又存进 god struct，导致双存漂移 —— 反例。lyra 的 HITL resume-at-pending 是正例。

### 【不要做】明确排除项

- plandex 的 **god orchestration struct**（55 字段）—— 违 SRP，lyra 字段分区 + service 拆分已更优。
- **自定义流文本解析** —— lyra tool-calling 更干净。
- **role 硬编码流水线**（planner→architect→coder→builder 写死阶段）—— lyra 的 planner 驱动 loop + `collectExtensions[T]` 扩展机制更 OCP。
- **provider fallback retry 链** —— 违 lyra「无 retry layer」决议。

---

## 4. 一句话总览

plandex 体量大、产品完整，但**架构上是另一类东西**（plan-first 刚性流水线产品 + 大泥球编排核心）。它给 lyra 的架构启发收敛到：

1. **偷一个形状**：Role 即配置（per-role model 的优雅落地）。
2. **固化一条原则**：派生态不二存（lyra 已在做，plandex 反例佐证）。

其余 plandex 的"特性"要么属前端/产品层，要么是 lyra 已刻意决策或已更优的方向。**lyra 的分层纪律、ISP、可观测、tool-calling loop 不需要向 plandex 看齐 —— 反过来才对。**
