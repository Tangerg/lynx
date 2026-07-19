# Codex 启发的能力吸纳 Backlog

> **来源**：对 **Codex**（OpenAI 的 Rust 编码 agent CLI，`codex-rs` ~130 crate workspace + TS SDK；桌面克隆 `~/Desktop/codex`）的源码级对比分析。形态：本地 `Codex` 核心引擎（SQ/EQ 提交/事件双队列）+ 传输无关 app-server 协议 + TUI/GUI 前端。**把"安全执行"当一等公民**——这正是它相对 lyra 最有价值处。方法与 [`GROK.md`](GROK.md) 一致；跨应用总索引见 [`README.md`](README.md)。
>
> **状态**：全部 proposed。已跳过 parity 项。

---

## 0. 五个 distinctive 子系统（按对 lyra 价值排序）

`network-proxy`（托管 MITM 代理 + 凭证经纪人）· `sandboxing`（双轴 PermissionProfile → 三平台原生沙箱）· `execpolicy`（Starlark argv 策略 DSL + approval/sandbox 耦合 + 审批自动回写）· `code-mode`（模型写 JS 在 V8 里编排工具）· `apply-patch`（无行号、内容寻址模糊匹配多文件补丁）。

**关键定位**：codex 的沙箱在 [Grok G1](GROK.md) 的 profile 底座上多了实质细节（见 §CX2 与末尾对比小节），而 **凭证经纪人是别家都没有、对 lyra 杠杆最高的新能力**。

## 0.1 筛选准则

沿用 lynx 四道筛子 + 反向不变量，详见 [`GROK.md` §0.1](GROK.md)。

---

## 1. 吸纳清单

### CX1 · 凭证经纪人 + loopback 网络策略（P1，最高价值、别家皆无）

- **来源/为什么**：`network-proxy/src/{credential_broker,config,connect_policy,mitm_hook}.rs`。网络访问被强制走一个**只监听 loopback 的托管代理**（fs 沙箱只放行 `localhost:proxy-port` 出站，其余 fail-closed）。代理叠四层策略：(a) `NetworkMode::Limited`＝只允许 `GET/HEAD/OPTIONS`、`Full`＝全放行（HTTP 方法级）；(b) 按域 `allow/deny`（`None<Allow<Deny`，deny 胜）；(c) MITM 拆 TLS 对 HTTPS 内层执行方法/域策略；(d) **credential broker**：spawn 子进程前把 env 里真实密钥（`GITHUB_TOKEN` 等）换成 dummy 并登记 `(dummy→real, host_binding)`；出站请求到达该 host 且 header 含 dummy 时，代理才注入 real 值。**agent 进程自始至终看不到真实密钥**。
- **目标**：切断"密钥 → agent env → shell 工具可读 → prompt-injection 外泄"这条链——密钥只存在于 runtime 代理里、只朝正确 host 注入。即便 agent 完全被攻陷，也只能"用"密钥打绑定 host，读不到、传不走。
- **落点**：runtime 执行边界（lyra 的 shell/exec 出口 + 一个新的 session 级代理进程）。
- **计划**：① session 级 loopback 代理进程 ② spawn 子进程时 env 密钥 → dummy + 登记绑定 ③ 出站请求到绑定 host + header 含 dummy → 注入 real ④（次要）域 allowlist + 方法级策略 ⑤ fs 沙箱只放行 `localhost:proxy-port`（与 C7 联动）。
- **验收**：agent 的 shell 读不到真实密钥值（env 里是 dummy）；`git push` 到绑定 host 仍成功（代理注入）；朝非绑定 host 发 dummy 不注入；配了代理但推不出 loopback 端点 → 网络置空（零出站）。
- **风险/边界**：**治本（第二法则）**——在正确的层（出站边界）消除外泄根因，而非工具侧打补丁。credential-broker 内核薄、值得全吸；完整 rama-based MITM+SOCKS 栈成本高，可先做"loopback 代理 + 域 allowlist + 密钥注入"最小版。这是**真实威胁面**（编码 agent + 不可信仓库/网页内容 = 经典 exfiltration 场景），lyra 当前完全缺失。
- **优先级**：**P1（credential broker 最小版）/ P2（域·方法完整策略）**。

---

### CX2 · OS 沙箱：双轴 PermissionProfile + 平台翻译（P1，= 落地 C7；[Grok G1](GROK.md) 的增量）

- **来源/为什么**：`sandboxing/src/{manager,seatbelt,denial}.rs` + `seatbelt_base_policy.sbpl` + `linux-sandbox/src/landlock.rs`。一个 `PermissionProfile` → `(FileSystemSandboxPolicy, NetworkSandboxPolicy)` **两条正交策略**；`SandboxManager::transform` 在执行边界把每条命令包成平台原生沙箱：macOS `sandbox-exec`（动态 SBPL，`(deny default)` 起步，**路径以 `-D key=path` 参数传入**而非字符串插值）、Linux `codex-linux-sandbox`（Landlock+seccomp+bwrap）、Windows 受限令牌。超出"profile-Seatbelt"的细节：可写根内仍用正则 `require-not` 保护 `.git`/`.codex`；unreadable glob 同时 deny `file-read*` 和 `file-write-unlink`（防用删除探测密钥）；base SBPL 精细放行 sysctl/PTY/cfprefs/POSIX-sem 让 Python multiprocessing、PyTorch/libomp、openpty 真能跑。
- **目标**：直接落地延期的 C7，把 lyra 从"defense-in-depth confirm"升级到真正 OS confinement。
- **落点**：exec 执行边界（lyra 的 shell/exec runtime）；macOS Seatbelt 先行（覆盖用户主平台）。
- **计划**：① 双轴 `PermissionProfile`（FS ⊥ Network）从一个 profile 派生 ② **逐命令** fs 沙箱包裹（sandbox-exec/landlock）③ 工作区内挖保护子路径（`.git`/`.lyra`）④ 敏感 glob deny-read + deny-unlink ⑤ 路径作 `-D` 参数 ⑥ 直接搬 base SBPL allowlist 文本让工具链不崩 ⑦ fail-closed。
- **验收**：可写工作区内改不了 `.git` 历史/lyra 配置；删不动也读不了敏感 glob；Python/PyTorch 类工具链在沙箱内正常跑；策略不可强制即拒绝。
- **风险/边界**：take idea not form（Go 用 `sandbox-exec`/`landlock-go`/bwrap，不搬 tokio/rama）。与 [Grok G1](GROK.md) 是**同一 C7**——实现时以 codex 的双轴 + 保护子路径 + deny-read+unlink + 路径作参数为**更细的落地细节**，Grok 的 fail-closed + 项目只能加为治理规则。**架构分歧同 G1**：lyra 长驻多 session → 逐命令/逐子进程 confine，不锁 runtime 进程。
- **优先级**：**P1**（与 [Grok G1](GROK.md) 合并为 C7 一条）。

---

### CX3 · execpolicy：argv 策略层 + approval×sandbox 联合决策 + 审批自动泛化（P2）

- **来源/为什么**：`execpolicy/src/{policy,rule,decision,parser}.rs` + `core/src/exec_policy.rs` + `core/src/tools/orchestrator.rs`。Starlark 规则文件 `prefix_rule([...], decision, match, not_match, justification)` 按 program 首 token 索引、对后续 token 前缀匹配（支持 `Alts`）；`Decision` 定序 `Allow<Prompt<Forbidden`、多规则命中取 `.max()`（最严胜）；每条带 justification + 正/负示例，**parser 编译期验证示例必须命中/必不命中**（防规则悄悄失效）；`host_executable` 把 `git` 规则绑到真实路径（防 PATH 劫持）。关键耦合：最终决策 = **策略规则 × approval 模式 × 沙箱可用性**（如 OnRequest + Restricted 沙箱下，非危险的未命中命令 → 直接 Allow，交沙箱兜底、不打扰用户）；用户批准时**自动派生一条最小泛化 allow-prefix 规则回写** `~/.codex/rules/default.rules`（self-authoring policy）并校验其覆盖所有 segment；沙箱拒绝（`is_likely_sandbox_denied`：关键词 + Linux `SIGSYS` 退出码）→ escalation "沙箱外重跑？" → 回写 bypass-amendment。
- **目标**：把 lyra approval 从"每工具 safety class"升级为**命令级、可解释、带示例校验的策略层**，且 approval 决策**联合考虑沙箱是否在场**（有沙箱兜底就少打扰），审批**自动泛化成最小前缀规则**。
- **落点**：approval Service + exec runtime；落在 lyra 现有 approval Service + `Rule` schema 上演进。
- **风险/边界**：**不引 Starlark**（那是 form）；吸的是"argv 前缀规则 + 最严胜 + justification + 示例校验 + approval×sandbox 联合 + 审批自动泛化"这套**思想**。KISS：DSL 用 Go 结构体/TOML，别引脚本引擎。approval×sandbox 联合门控于 CX2/C7 落地。
- **优先级**：**P2**。

---

### CX4 · apply_patch 编辑格式（P3，edit 漂移是真痛则 P2）

- **来源/为什么**：`apply-patch/src/{lib,parser,seek_sequence}.rs`。`*** Begin Patch / Add|Update|Delete File / Move to / @@ 上下文 / +|-|space 行 / End Patch`。**无行号**——forward-only 游标 + 可选 `@@ 锚`定位，`old_lines` 连续块匹配，逐级模糊回退（精确→去尾空白→去首尾空白→Unicode 归一，en-dash/花引号/nbsp 折 ASCII），EOF pattern 从尾锚定；**单信封批量** add/update/delete/rename **多文件**（顺序 apply，`AppliedPatchDelta` 记录已提交变更以便部分失败可诊断）。
- **目标**：内容寻址模糊匹配 + `@@` 人类可读锚，让模型凭记忆写编辑不数行号；一个信封原子批量跨文件增删改移。
- **落点**：tools 的 edit 工具族。
- **风险/边界**：freeform Lark grammar 是 OpenAI Responses 专属（**不吸** grammar，lyra 跨 provider）；但**补丁格式 + seek_sequence 模糊匹配器**是 provider 无关、可作普通 typed 工具移植。lyra 现在是 read-before + stale-guard 单串替换（对空白/Unicode 漂移脆）；是否值得取决 edit 漂移痛点强度。
- **优先级**：**P3（edit 漂移是真实痛点则升 P2）**。

---

### CX5 · guardian：自动审批复核（forked LLM reviewer）（P3；与 [Claude Code CC3](CLAUDE_CODE.md) 收敛）

- **来源/为什么**：`ext/guardian/src/lib.rs` + orchestrator 的 `ApprovalReviewer::Guardian`/`strict_auto_review`。`strict_auto_review` 开启时，风险命令的 approval 不弹给人，而路由给一个从当前线程 fork 的 **guardian 子 agent（LLM）**复核批/否；沙箱拒绝后"沙箱外重跑"也要 guardian 重审。
- **目标**：在 yolo 与"每次问人"之间加一档——独立 LLM reviewer 做自动第二意见，无人值守时也有判断而非全放行。
- **落点**：approval Service 的一个**可插 reviewer**（非新子系统）。
- **风险/边界**：**与 [Claude Code CC3](CLAUDE_CODE.md)（yolo LLM 分类器）殊途同归**——两家独立都做"yolo 下 LLM 自动复核"→ 强信号。实现时合并为 approval Service 内一条 LLM-reviewer decision path，复用 lyra per-role utility model + 现有 spawn 模式；连同 fail-closed + 只喂 tool 投影一起吸。
- **优先级**：**P3**（与 CC3 合并考量）。

---

### CX6 · code-mode（tools-as-code / V8）—— 不吸（暂缓）

- **来源**：`code-mode`/`code-mode-host`/`code-mode-protocol`。模型写一段 **JavaScript** 在 V8 isolate 里跑，工具暴露为 `await tools.<name>(...)`，用真 loop/条件/变量在调用间整形数据；长脚本返 `cell_id`、`wait` 增量拉输出。
- **Verdict**：**不吸（暂缓）**。为此要在 Go 里嵌 JS 引擎（goja/V8）+ 沙箱 + 一整套 host 协议，成本极高；连 codex 自己都还 dev-gated 默认关；lyra 已有 ConcurrencyKey 并行工具，拿到大部分"减往返"红利。**明确记为设计决策：先不做**——若未来出现真实"多步工具编排"瓶颈再评估（届时优先轻量 batch/multi-tool 调用而非嵌 JS）。

---

## 2. 刻意不吸清单

| 项 | 来源 | 不吸理由 |
|---|---|---|
| **rollout：JSONL 真源 + SQLite 可弃索引** | `core` rollout | lyra 刻意选"单 SQLite 为唯一真源"（既有决策）；codex 的 read-repair/fs-fallback 是为其双存储服务，lyra 单源不需要 |
| **app-server 的 ts_rs/JsonSchema 协议 codegen** | app-server | lyra 已否决 protocol→TS codegen（Go flat-struct 不映射 TS 判别联合），改用 golden-sample 漂移门；不回头 |
| **`response_id` 续跑** | Responses API | 依赖 OpenAI 服务端状态（provider 锁定），违反 lyra "不给 provider 加私有续跑"；lyra 已有自己的 durable cross-restart resume |
| **config `requirements`/`constraint`（MDM/企业托管）** | config | 多租户/企业治理，lyra 单本地用户（出 filter #2）|
| **`unified_exec` 的 PTY 交互会话** | core exec | lyra 已刻意 park PTY（background commands, no PTY），不重开 |
| **collaboration-mode-templates（plan/execute/pair）** | core | 等价 lyra 已有 plan-mode + personalities，parity |
| **`rollout-trace`（事件→离线 reducer→语义图）** | rollout-trace | 好的深度调试观测思想，但 lyra OTel 三驾马车已覆盖此意图 |
| **code-mode / V8 tools-as-code** | code-mode | 嵌 JS 引擎成本极高、codex 自己 dev-gated、lyra 并行工具已拿红利（见 CX6）|

---

## 3. 专项：codex 沙箱 vs Grok 式 profile，codex 多了什么

Grok 式 profile ≈ 每次 exec 一个参数化 Seatbelt/Landlock profile（可写根列表 + 网络开关）。codex 在同底座上多了这些**实质差异**（C7 落地时应直接吸）：

1. **两条正交轴**：FS 策略与 Network 策略各自独立、从一个 `PermissionProfile` 派生，非一个"sandbox level"标量。
2. **工作区内再切保护子路径**：可写根内正则 `require-not` 挖掉 `.git`/`.codex`——"可写工作区"≠"能改写 git 历史/篡改 agent 配置"。
3. **敏感 glob 连删除一起 deny**：unreadable glob 同禁 `file-read*` 和 `file-write-unlink`，堵"用 unlink 探测密钥"。
4. **网络是策略不是布尔**：HTTP 方法级 + 按域 allow/deny + 按协议 + unix-socket allowlist，**统统经只监听 loopback 的托管代理执行**（fs 沙箱 deny 一切出站、只放行 `localhost:proxy-port`）。
5. **凭证经纪人**：真实密钥永不进 agent env（见 CX1）——无任何 profile 沙箱有这层。
6. **per-command fs 沙箱 + session 级共享代理**：agent 本体不沙箱，**每条命令**各自包 fs 沙箱，**网络**是 session 级一个代理进程。
7. **策略层决定"要不要沙箱"**：execpolicy 明确 allow 的命令 `bypass_sandbox` 原生跑，未命中进沙箱，沙箱拒绝 → 带审批的 escalation + 自动 amendment。
8. **路径以 `-D key=path` 参数传入** `sandbox-exec`，防"构造路径名注入策略"；**fail-closed**。

一句话：Grok-profile 是"参数化 profile + 网络开关"；codex 是"**双轴 permission profile → 每命令 fs 沙箱 + 一个执行 host/方法/凭证策略的托管网络代理**，并与可解释、可自我生长的 exec-policy 层耦合"。对 lyra，**凭证经纪人 + loopback 代理（CX1）**杠杆最高，**双轴 profile + 工作区内保护子路径 + deny-read+unlink（CX2）**是 C7 落地时应直接吸收的细节。

---

## 4. 建议节奏

- **头号**：CX1（凭证经纪人最小版）—— 别家皆无、消灭一整类外泄漏洞。
- **与 [Grok G1](GROK.md) 合并为 C7**：CX2（双轴沙箱，Seatbelt 先行 + codex 细节）。
- **中等**：CX3（execpolicy 思想，门控于 C7）、CX5（guardian，与 [CC3](CLAUDE_CODE.md) 合并）。
- **随缘**：CX4（apply_patch，edit 漂移是真痛则升）。
- **不做**：CX6（code-mode）。
