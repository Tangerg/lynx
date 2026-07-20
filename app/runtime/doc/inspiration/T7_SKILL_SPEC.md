# T7 · 自进化 Skill —— 落地 Spec（Hermes 挖掘脑 + B4 强制-HITL 身）

> **状态**：设计钉在现有代码上，待**新满血会话逐批执行**（每批绿灯独立 commit+push）。来源 [`HERMES.md`](HERMES.md) Focus 2 / [`README.md`](README.md) T7（H-Skill-1~4）。分支 `codex/runtime-architecture-refactor`。
>
> **一句话**：lyra B4 已有**完整的写侧**——agent 主动 `propose_skill` → 校验/扫描 → `_drafts/` 暂存 → **强制 HITL 晋升** → active↔archived curator（治理比 Hermes 强）。T7 = 补 Hermes learning-loop 的三样 B4 缺的能力：**① 轨迹自动挖掘（agent 不必主动提）② 反馈驱动精修现有 skill ③ 自动闲置生命周期（provenance-gated）**。**核心纪律**：取 Hermes 的**自动挖掘脑**、接到 B4 的**强制-HITL 身**——每个自动提议落 `_drafts/` 走人审门，只有 auto-authored 才 auto-curate，绝不引 Hermes 的自由写默认。**与 C8 记忆是同一个 HITL-propose 模式**（后台挖掘→pending/draft→人审），可镜像。

---

## 0. 现状（B4 写侧已建成，勿重造）

- **`propose_skill` 工具**（`internal/adapter/toolset/skillpropose/skillpropose.go`）：agent 提议 → `Draft.Validate/Scan` → `SaveDraft` → **in-band `QuestionInterrupt` HITL** → Approve→`Promote` / Reject→`DiscardDraft`。**agent 永不自发布**。`Authoring` 口（:31）：Enabled/SaveDraft/Promote/DiscardDraft。build.go:185 装配。
- **`skillauthoring.Store`**（`internal/infra/skillauthoring/store.go`）：`SaveDraft`（content-addressed `_drafts/<sha256>/`，atomic rename、no-clobber、`os.OpenRoot` path-confined）· `Promote`（`_drafts/<rev>`→`<name>/`，re-validate、archived-conflict 拒）· `DiscardDraft` · `Archive`/`Restore`（`_archive/`，never delete）· `List`（active+archived）。**关键缺口**：**无 `ListDrafts`**（不能枚举 `_drafts/`）；**无 provenance / usage 追踪**。
- **`domain/skills`**：`authoring.go` `Draft{Name,Description,Body}` + `DraftHandle`(sha256) + `Scan`(保守正则、非安全边界) + `Render`(YAML frontmatter)。`skills.go` `DraftsSubdir="_drafts"` / `ArchivedSubdir="_archive"`（下划线前缀=非法 skill 名 → 只读 loader 自动跳过，草稿对模型不可见）· `Lifecycle{Active,Archived}` · `Entry{Name,Description,Lifecycle}`。
- **`skills` 模块**（`/Users/tangerg/Desktop/lynx/skills`）：`Source`(List/Load/OpenResource 渐进披露) + `Frontmatter{Name,Description,License,Compatibility,Metadata map[string]string,AllowedTools}` —— **`Metadata` 是现成 provenance 槽**（无需改 schema）。`skill` 工具（`tools/skills/tool.go`，op: list/load/load_resource）= 渐进披露机制。
- **RPC**：`skills.discovered.list` · `skills.library.list/archive/restore`（`delivery/server/skills.go` → `application/workspace` `SkillCurator` 口 List/Archive/Restore）。**缺口**：**无 drafts RPC**——晋升只在 `propose_skill` 工具调用内 in-band。
- **maintenance seam（挖掘器同构点）**：`internal/adapter/agentexec/turn/turn.go:211 postTurnMaintenance` → 压缩触发后调 `extractor.MaybeExtract`（seam `Extractor.MaybeExtract(ctx,sessionID,cwd)` in `turn/engine.go`，`memoryDispatcher` 持有，`Deps` 注入）。**= skill 自动挖掘的直接同构点**。蓝本实现 `adapter/maintenance/extraction.go`（LLM 直接调用）+ 装配 `bootstrap/maintenance_wiring.go`。
- **grep 确认**：无任何 `miner/candidate/trajectory/self-evolv*` 代码 —— S1 在上述原语之上是 greenfield。

## 1. 缺口（vs Hermes learning-loop）

| # | 缺口 | 落到哪批 |
|---|---|---|
| G1 | 轨迹自动挖掘（后台 review 按复杂度/cadence 触发挖轨迹为 skill 候选） | S1 |
| G2 | 草稿**离线**审阅面（自动草稿无 live turn 的 in-band interrupt 看着 → 需离线 list/promote/reject） | S2 |
| G3 | 反馈驱动精修**现有** skill（从用户纠正 patch）+ read-before-write guard | S3 |
| G4 | 自动闲置生命周期（usage + active→stale→archived by inactivity + grace floor + pinned/referenced 豁免） | S4 |
| G5 | provenance（created_by/source_session）+ provenance-gated（只 auto-authored auto-curate） | S1 stamp + S4 gate |
| G6 | prompt 智慧（反模式清单 + umbrella 塑形，可移植纯智慧） | S1 用 + S5 |

## 2. 目标架构

三段，全走 B4 的 `_drafts/` + 强制 HITL 门：
- **挖掘器**（S1）：postTurnMaintenance 里按复杂度触发的 `SkillMiner`，**直接 LLM 调用**（取 Hermes 挖掘脑、但用 lyra extraction 式直接调用，**不 fork agent**——更简、贴反向不变量）读轨迹 → 产出 SKILL.md 草稿 → `SaveDraft`，frontmatter stamp `created_by=agent` / `source_session`。
- **离线审阅**（S2）：`ListDrafts` + `skills.drafts.list/promote/reject` RPC（镜像 C8 `agentMemory.*`）。in-band propose_skill 路径保留（前台主动提）。
- **自动生命周期**（S4）：usage sidecar + 确定性 stale/archive by inactivity，**provenance-gated**（只 created_by=agent 自动降级），never delete。

## 3. 批次（各批绿灯独立 commit，前端 defer）

**S1 · 轨迹自动挖掘 → 草稿 + provenance（核心 gap H-Skill-1）**
- `SkillMiner interface { MaybeMine(ctx, sessionID, cwd string) error }`（`turn/engine.go`），`memoryDispatcher` 持有，`postTurnMaintenance` 调用。**触发 = 复杂度**（本 turn tool 迭代数 ≥ 阈值，类似 Hermes `_skill_nudge_interval`）**+ cadence**（每 N 复杂 turn 至多一次，bound LLM 成本）；**独立于压缩**（复杂 turn 值得挖，不必等 compaction）。
- Miner impl `adapter/maintenance/skillmine.go`（镜像 `extraction.go`）：读 history → `askDirect` 工程化蒸馏 prompt → 产出草稿 → `SaveDraft`。**draft content-addressed → 重提幂等**。
- **provenance**：扩 `domain/skills` `Draft` 带 `Metadata map[string]string`（或 Origin/SourceSession 字段）→ `Render` 写进 frontmatter（复用 `Frontmatter.Metadata` 槽）。stamp `created_by=agent`、`source_session=<id>`。
- **蒸馏 prompt（H-Skill-4 智慧）**：偏好序（patch-loaded > patch-umbrella > add 支撑文件 > new-umbrella）+ **反模式清单**（别捕环境依赖失败 / 负面工具断言 / 已解决瞬时错误 / 一次性任务叙事）+ umbrella 塑形（class-level 非一 session 一 skill）。

**S2 · 草稿离线审阅 RPC（H-Skill-1 交付面）**
- `Store.ListDrafts(ctx) ([]DraftInfo, error)`：枚举 `_drafts/`，读每个的 frontmatter + provenance + handle。
- `skills.drafts.list / promote / reject` RPC，**全链路照 goal.go / C8 agentMemory.\***（method_names + method_table + handlers_skilldrafts → server/skills.go → protocol → runtime；disabled→capability_not_negotiated）。application port 扩（`SkillCurator` 加 draft 方法或新 `SkillDrafts` 口）。promote/reject 复用 store 的 `Promote`/`DiscardDraft`（按 handle）。
- **为什么离线**：自动草稿没有 live turn 的 in-band `QuestionInterrupt` → 必须离线枚举+审。in-band propose_skill（前台主动提）保留不动。

**S3 · 反馈驱动精修现有 skill（H-Skill-2）**
- 同一 miner pass 增 **patch 路径**：检测用户纠正（"别做 X"/"太啰嗦"/加载的 skill 被证过时）→ 产出**替换现有 `<name>/` 的新版本草稿**。
- **read-before-write guard**：patch 前必须先 `Source.Load` 目标 skill（prompt 喂全 body），防改写只从 transcript 推断的内容。
- ⚠️ **store 改动**：现 `Promote` 遇 active `<name>/` 已存在会 rename 失败。S3 需 `Promote` 支持"替换现有 active skill"（archive 旧 → rename 新，或原子 swap），且 archived-conflict 逻辑相应调整。

**S4 · 自动闲置生命周期 + provenance-gated curation（H-Skill-3）**
- usage 追踪：`.usage.json` sidecar（use/view/patch counts、last_used、state、pinned、created_by）——skill 被 `skill load` 时记 use（触点 `toolset/skill` 或 store）。
- 确定性生命周期：active→**stale@Nd**→**archived@Md** 无活动；re-use 复活；never-used/新建 **grace floor**；**pinned/referenced/user-created 豁免**。
- **provenance-gated**：只 `created_by=agent` auto-curate（auto-archive）；用户/前台创作绝不自动降级。
- curator：boot sweep + cadence（镜像 `application/schedules` worker 或 `goals.Driver`；或 postTurnMaintenance 顺带低频扫）。**never delete**（只 `_archive/`）。

**S5 · prompt 智慧 + 可观测 + 测试收尾**
- 反模式清单 + umbrella 塑形也折进 `propose_skill` 指引（前台提议同样受益，零机制纯 prompt）。
- OTel：`skill.mine` / `skill.promote` span + 计数（照 `application/goals/observe.go`）。
- 单测（miner 挖掘、ListDrafts、RPC 映射、provenance stamp、lifecycle 降级 provenance-gate）+ race + 12 模块 build 全绿。

**前端 skill drafts 审阅 UI** = 独立前端批，**defer**（照 C8；API 先暴露）。

## 4. 复用地图（勿重造）

| 要用的 | 在哪 |
|---|---|
| 草稿暂存 + 晋升 + 归档（content-addressed/idempotent/no-clobber/path-confined） | `infra/skillauthoring/store.go` SaveDraft/Promote/DiscardDraft/Archive/Restore |
| Draft 值 + handle + scan + render | `domain/skills/authoring.go` |
| `_drafts/`/`_archive/` 约定（loader 自动跳过） | `domain/skills/skills.go` |
| provenance 槽（零 schema 改动） | `skills.Frontmatter.Metadata map[string]string` |
| Authoring 消费口 | `toolset/skillpropose` `Authoring` |
| 渐进披露工具 | `tools/skills` + `toolset/skill` |
| **挖掘器 seam 蓝本** | `turn/turn.go:211 postTurnMaintenance` + `turn/engine.go Extractor` seam + `adapter/maintenance/extraction.go`（直接 LLM 调用）+ `bootstrap/maintenance_wiring.go` 装配 |
| HITL 门原语 | `interrupts.Func` + `runs.QuestionInterruptKind`（in-band）；离线走 RPC |
| RPC 全链路 | goal.go / C8 `agentMemory.*` 那条 |
| curator worker 蓝本 | `application/schedules`（cron worker）/ `goals.Driver`（cadence driver） |
| 可观测 | `application/goals/observe.go` |
| skill 管理面前端先例 | B4 Skill Library 视图（`app/desktop`） |

## 5. 刻意不吸（Hermes，防重新论证）

- **自由写默认 / opt-in-only 扫描** —— B4 强制 `_drafts/`+HITL+扫描更强，保。
- **background review fork 新 AIAgent + 工具白名单隔离** —— 重机器；lyra 用更简的**直接 LLM 调用**（extraction 模式），把要 patch 的 skill body 喂进 prompt 即可，无需 fork agent。YAGNI。
- **weekly LLM umbrella-consolidation** —— aux 成本，Hermes 自己默认 off，单用户 YAGNI。
- **semver `version:` 字段** —— theater，`_archive/` 是唯一历史。
- **两文件记忆切分 / 8 外部 provider** —— N/A（那是 Focus 1 记忆的，已在 C8 处理）。

## 6. 反向不变量校验

- **写生命周期所有 + HITL 一致**：自动草稿走强制人审门，只有 auto-authored auto-curate；agent 永不自发布 skill。
- **无隐藏后台自由写** —— 挖掘器只产 `_drafts/` 草稿，不落 active。
- **无 fork-agent 重机器** —— 直接 LLM 调用（extraction 蓝本）。
- **存储**：skill 是文件（SKILL.md repo，唯一文件例外之一）；`.usage.json` sidecar 与 skill 同源（合理，非新存储后端）。
- **破坏性改动**：扩 `Draft`（加 Metadata）+ 新 `skills.drafts.*` RPC + `Store.ListDrafts` + `Promote` 替换语义 —— dev 阶段直接改；新会话执行时按"破坏性公开 API 改动先咨询"走（本 spec 即 scope）。
