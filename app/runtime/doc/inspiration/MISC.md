# 其余桌面 Agent 的轻量扫描（低细粒度）

> **这是什么**：对桌面上**其余 ~23 个** agent 类应用的一次**轻量快速扫描**（只读 README/docs/顶层结构，不深挖源码），补在 6 份细致对照（[`README.md`](README.md)）之外。目的：确认没有遗漏的真 gap。**排除** `portai*`（用户自有 PortAI）与已细看的 7 家。
>
> **总结论**：**绝大多数无新增可吸纳项**——要么多租户云平台、要么 chat-UI 形态、要么框架抽象、要么与 lyra parity 或已在 backlog 覆盖。净新增只有 **6 个低优先（P3 / design-note 级）的 部分吸** 点，且多为已有条目的**细化或 UX 加分**，无一改变主 backlog（[`README.md`](README.md)）的优先级格局。
>
> **状态**：proposed（轻量结论，未细验）。

---

## 1. 净新增可吸纳项（全部 P3 / design-note 级）

| # | 来源 | 点子 | 判据 | 归属 |
|---|---|---|---|---|
| M1 | **plandex** (Go) | **命名自主档位拨盘**（None/Basic/Plus/Semi/Full）把 ~10 个运行时开关（auto-continue / auto-load-context / smart-context / can-exec / auto-exec / auto-debug / auto-apply / auto-commit）打包到一个"自主度"档位后。 | **部分吸**（纯 UX，零新机制）——lyra 这些开关多已有（approval/yolo/hooks），只是散落的 per-toggle；一个"自主度预设"是廉价整合。 | 独立小 UX 项，P3 |
| M2 | **plandex** (Go) | **stage-to-sandbox → review → 逐文件 reject → apply**：改动 + shell 命令先累积在版本化沙箱里、apply 前绝不碰真实文件。 | **design-note（多半不吸）**——是 lyra "先 apply 再 checkpoint 回滚" 的真替代安全模型（aider/opencode 系），但整套引入会与 checkpoints+approval **双机制**（filter③）。值得写一条"为何选 apply-then-rollback 而非 stage-then-apply"的设计说明，未必采纳。 | 关联 [T11 hunk/revert](README.md) |
| M3 | **craft-agents-oss** | **对话式"add a source"自助接入**：说"把 Linear 加为 source"，agent 自主发现其 API/MCP server、读文档、自接凭证（无配置向导）。 | **部分吸**——本质是 lyra 现有 MCP + [凭证经纪人 T2](README.md) + [工具搜索 T3](README.md) 之上的一个 prompt-loop，但作为一等 UX 流程是真新（lyra 现为手动 MCP 配置）。 | 关联 T2/T3，P3 |
| M4 | **trpc-agent-go** (Go) | **信号驱动的 skill 自进化触发**：不是"agent 决定调 propose_skill"，而是从 transcript **自动检测**用户纠正 / tool-error 恢复 / ≥N tool calls 触发 review；多门 promote（Spec→Safety→**EffectivenessGate** 结果启发式→HumanGate）。 | **部分吸**（细化 [T7 自进化 skill](README.md)）——与 Hermes 的 cadence/复杂度触发**互补**：信号驱动 vs 节奏驱动。EffectivenessGate 是个验证角度。仍走 B4 强制 HITL 身。 | 并入 T7，P2~P3 |
| M5 | **skill-creator** (Anthropic) | 授权时的 **SKILL.md validator/linter + packager**（`quick_validate.py`/`package_skill.py`）。 | **部分吸**——B4 自著有静态安全扫描，但缺 SKILL.md **结构校验/打包门**。 | 并入 B4，P3 |
| M6 | **DeepSeek-Code-Whale** (Go) | **`skills-lock.json` content-hash lockfile**：每个装的 skill 钉 GitHub 源 + `computedHash`（skill 版 package-lock，可复现/供应链校验）。 | **部分吸（次要）**——B4 缺 content-hash pinning 角度；仅当做 skill 从远程装（[Kimi K3 bundle](KIMI_CODE.md)）时才相关。 | 关联 K3，P3 |

---

## 2. 印证 / 收敛（非新增，但强化已有条目）

- **MiMo-Code**（小米，Go 终端 agent）：plan/build/compose agents + SQLite-FTS5 记忆 + goal/stop judge + **Dream(`/dream`) & Distill(`/distill`)** + voice。**每一项都已覆盖**——judge-loop ≈ [T5 Goal mode](README.md)、FTS5 记忆 ≈ [T4 C8](README.md)（且再证 FTS5 关键词层）、Distill ≈ [T7 skill 蒸馏](README.md)、compose ≈ plan-mode、voice=form。**独立印证 Goal-mode + 自进化 skill + FTS5 记忆的收敛**。
- **trpc-agent-go / Hermes / MiMo-Code** 三家都做"轨迹→技能/记忆"的自动学习回路 → 再次确认 [T7](README.md) 是真实趋势。

---

## 3. 全部不吸（分类，防重新论证）

| 类别 | 应用 | 理由 |
|---|---|---|
| **多租户云平台** | dify、coze-studio、OpenHands 的 multi-backend/cloud | filter② 单本地用户 |
| **chat-UI 形态（lyra 有自己的桌面）** | cherry-studio、lobehub、AionUi、Proma | form/parity，非 lyra 缺的能力 |
| **框架 / 编排抽象（welded core 拒绝）** | koog、eino、langchain4j、adk-go、embabel(GOAP 符号规划器) | filter③④：DAG/图/FSM/符号规划器是第二套编排机制；lyra LLM loop 已做规划/重规划 |
| **coding agent 但 parity/已覆盖** | cline、goose、crush、continue(EOL)、harness9 | plan-act/checkpoints/MCP/LSP/recipes/subagents/yolo 全 parity；goose subrecipes=recipes+subagents、lead-worker=per-role models、toolshim=反向不变量；crush `herdr`=tmux 形态 |
| **skill 内容仓（非能力）** | skills、skills 2、strategy-builder-skill | content，不是机制 |
| **adk-go 的 Go 工程细节** | ctx-injected time/uuid provider | lyra 刻意让 eventId ephemeral（durable order=items.list seq），不吸 |

---

## 4. 一句话

这轮轻扫**没有翻出改变格局的新能力**——主 backlog（[`README.md`](README.md) 的 T1~T25）已经把 6 家细看 + AgentScope 的精华收全。其余桌面 agent 要么是云平台/聊天 UI/框架抽象（结构性不吸），要么与 lyra parity。净新增只有 6 个 P3/design-note 级的 UX/治理小点（M1~M6），可在相邻 feature 里顺带，不单独立项。
