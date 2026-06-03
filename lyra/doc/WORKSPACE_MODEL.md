# WORKSPACE_MODEL.md — 工程目录 / 会话 的心智模型与设计基准

> 定位:lyra 怎么建模"项目文件夹 ↔ 会话"的关系。这份 doc 是**前后端协议重设计前的对齐基准**——
> 协议字段、UX 行为、边界处理都从这里推导。结论来自横向调研 6 个 agent 项目(2026-06)。
>
> 项目级约定见 [`../../CLAUDE.md`](../../CLAUDE.md);lyra 模块约定见 [`../CLAUDE.md`](../CLAUDE.md)。

---

## 0. 一句话心智模型(TL;DR)

> **文件夹就是项目。** 一个 `Session` 带一个 `cwd` 字段指向文件夹。**不存在 Workspace 实体**。
> "项目列表"是对所有 `session.cwd` 去重的**派生视图**,不是一张表。项目的**身份与配置住在文件夹里**
> (`AGENTS.md` / `LYRA.md`),不住在 DB 里。runtime 不持有"当前 workspace",每个 session 自带 cwd。

两级结构,到此为止(单用户本地 runtime,**没有第三层** org/account/team):

```
  cwd(文件夹绝对路径,best-effort 解符号链接)── 唯一承重原语
     │
     ├── Session A (cwd = /proj-foo) ┐
     ├── Session B (cwd = /proj-foo) ├── "项目 /proj-foo" = GROUP BY cwd 的派生视图(不落库)
     ├── Session C (cwd = /proj-bar) ┘
     │
  每个文件夹里的 AGENTS.md / LYRA.md = 该项目的身份与配置(已实现:agentdoc / memory_store)
```

---

## 1. 调研基线(6 个项目,2026-06)

| 项目 | 形态 | 会话单元 | 文件夹绑定 | Workspace 实体? |
|---|---|---|---|---|
| **Codex** (OpenAI) | CLI/daemon | thread | cwd 是 thread 的 setting | ❌ |
| **Claude Code** | CLI | SessionId | `sessionProjectDir` per-session + `switchSession(id,dir)` | ❌ |
| **kimi-code** | CLI | Session | `workDir` 是 session 字段,按 workDir 分桶 | ❌ |
| **opencode** (SST) | **server+多 client** | Session(pin projectID) | `directory` 列 + per-request `x-opencode-directory` | ✅ 但内容派生 id |
| **AionUi** | 独立桌面 | Conversation | `workspace?: string` 字段 | ❌ |
| **cherry-studio** | 独立桌面 | AgentSession/Topic | `accessible_paths: string[]` | ❌ |
| **Crush** (Charm) | Go TUI | Session(sqlite) | per-Workspace ConfigStore | ✅ 重(Workspace 拥有 App+DB) |
| **Proma** | 独立桌面 | Conversation | workspaceId→cwd | ✅ 轻书签 |

**计票:文件夹是 session 字段 / ambient = 多数;有 Workspace 实体 = 少数(Crush 重、Proma 轻、opencode 因内容派生 id 才需要)。**

**lyra 自己 follow 的是 kimi-code 模式**(见 `../CLAUDE.md`),而 kimi-code/Claude Code/Codex 三个 CLI 全是
"folder 当 session 字段、不立实体"。架构最像 lyra 的 opencode(server+多 client)也**确认**了核心:
cwd 是 session 字段、server 目录无状态、一个 server 服务多目录。

---

## 2. 核心决策(D1–D7)

### D1 · 不立 Workspace 实体;cwd 是 Session 的字段
- 内部 `session.Session` 加 `Cwd string`(绝对路径)。
- "项目"是 `GROUP BY session.Cwd` 的派生视图,不是表/服务/实体。
- **最硬的理由**:文件夹本身已是项目身份(`AGENTS.md`/`LYRA.md` 已 key 在 cwd 上)。再立 DB 实体 = 给项目身份造**第二真相源**,和文件夹打架(违反 DRY)。

### D2 · 路径派生身份,不用内容派生(对比 opencode)
opencode 用 git remote/commit 哈希当项目 id(改名自动幸存),但代价:仅 git 仓有效、要往 `.git/` 写文件、同 repo 两 checkout 合并历史、且**因此才需要 Project 实体**。
- lyra 选**路径派生**:简单、对非 git 仓一样有效、两 checkout 各算各的(对桌面用户更直觉)、**不需要实体**。
- 改名这个低频事件用 relocate 处理(见 §5),对 git/非 git 一律有效。

### D3 · runtime 目录无状态,服务多文件夹,cwd per-session
- runtime **不持有**"当前 active workspace"。每个 session 自带 cwd。
- 未指定 cwd 时默认 = serve 进程 cwd(对齐 opencode 的 `process.cwd()` 回落、保证单文件夹 MVP 零改动可跑)。

### D4 · Session flat 按 id 存,**不按 cwd 路径分桶**
- lyra 现状已是 flat:`FileSessionService` 存进单个 `sessions.json`,key 是 uuid。
- **刻意不学** Codex/Claude/kimi 的"按 cwd 路径 hash 分桶存储"——那会继承它们的**改名即 orphan** 病(见 §5)。

### D5 · cwd 身份做 best-effort 符号链接解析
- 建 session 规范化 cwd 时:`resolved, err := filepath.EvalSymlinks(abs); if err != nil { resolved = abs }`。
- 对齐 Claude Code + Codex(两个最权威参照都 resolve)。`/link` 与 `/real` → 同一项目身份。
- **必须 best-effort**:iCloud/Dropbox 等云挂载会 `EPERM`(Claude Code 踩过),失败回落 clean 后的绝对路径,不报错。
- ⚠️ **与 path-jail 分清**:resolve 是为身份一致;fs 工具的安全 confinement 是另一回事(用词法规范化,kimi-code 的做法),且 lyra path-jail 现仍是 TODO。两者不混。

### D6 · "项目"视图 = 对 session.Cwd 的派生分组
- `workspace.projects` 退化为:distinct `session.Cwd`(可附 basename 作 name、git 分支作注解)。
- `workspace.selectProject` = 纯 UI(决定新 session 默认 cwd),后端可 trivial 甚至砍。

### D7 · git 可选;`.git` 检测走文件系统,git 元数据 best-effort
- 项目根检测 = `os.Stat(<dir>/.git)` 文件系统向上走(lyra `findProjectRoot` 已如此),**不需要 git 二进制**;找不到回落 cwd。
- 没装 git / 不是 git 仓 → 项目根 = cwd,发现退化单层。零崩溃。
- git 元数据(branch/sha/diff)= 唯一需要 git 二进制处,**纯 UI 装饰**,以后加时必须 `exec.LookPath` + 失败静默忽略,绝不设为必需。lyra 现在 exec git 处为零。

---

## 3. 两个根的分离(防返工的关键原则)

所有 CLI 参照(Codex/Claude/kimi)一致:**cwd 用 verbatim/resolved,git-root 只用于"向上发现配置"**。存两个值、职责分清:

| 值 | 来源 | 用途 |
|---|---|---|
| **`cwd`** | 用户打开的目录(best-effort 解符号链接) | 会话身份 + fs 工具根 + 分组键。**永不 snap 到 git root** |
| **`projectRoot`** | 从 cwd 上溯到 `.git`(派生,可不存或只缓存) | **仅**给 `AGENTS.md` / `LYRA.md` / 配置的发现范围 |

**只要守住"cwd 逐字、发现向上走",子目录/父目录/嵌套全部自动正确,不需要任何 path-containment 合并逻辑。**

---

## 4. 前端 UX 四边界行为

### ① 冷启动(没选任何目录)
- **不弹强制选择框**。默认到:**上次用的目录 → 否则 serve 进程 cwd → 否则 home**。
- header 显示当前目录 + "打开文件夹" + 最近列表。"空状态"其实是"默认到 X,点击可换"。工具永远能跑,零摩擦。
- (参照:AionUi 非阻塞、Claude Code/Codex 永远有 cwd;反例 cherry-studio 是纯聊天无文件夹概念,不适用。)

### ② 打开一个路径
- 原生目录选择器(Wails `runtime.OpenDirectoryDialog`)。
- 路径 **best-effort 解符号链接后逐字用**,**不 snap git root**。
- 加进**最近目录列表**(按精确路径去重,cap ~10,存 `~/.lyra/recent.json` 或 config)。
- "打开" = 设定**新建 session 的默认 cwd**;已存在 session 各自保留自己的 cwd。

### ③ 打开已开目录的**子目录**
- 当作**不同 cwd → 不同 session 组 / 不同项目条目,不合并、不去重**(三个 CLI 一致)。
- **但** `AGENTS.md`/`LYRA.md` 发现**向上走到 git root** → 子目录**自动继承** `/repo` 的配置。
- "同项目"的连贯感来自**向上发现**,不来自合并条目。

### ④ 打开**父目录**
- 同理:不同 cwd、独立条目、无特殊处理。

---

## 5. 边界处理矩阵(参照共识 + lyra 现状 + 决策)

| 边界 | 参照共识 | lyra 现状 | 决策 / 动作 |
|---|---|---|---|
| **没装 git / 非 git 仓** | 一致:文件系统 stat `.git`、回落 cwd、不需 git 二进制 | ✓ `findProjectRoot` 已如此;exec git 处为零 | **已对齐**。守住"未来 git 元数据 best-effort" |
| **外部改名文件夹** | 三家**按路径分桶 → 必 orphan**,且无 relocate;opencode 内容派生可幸存(git 时) | flat 按 id 存,cwd 是字段 | **lyra 结构性更优**:session 幸存,cwd 字段悬空 → `os.Stat` 检测 → `sessions.update(cwd)` 重指即恢复。**别学按路径分桶。** 缺失时优雅降级(标记"文件夹丢失",允许纯聊天/重定位,不崩) |
| **符号链接 cwd** | **分裂**:Codex/Claude resolve,kimi verbatim | verbatim(无 EvalSymlinks) | **改为 best-effort resolve**(D5):`/link` 与 `/real` 同身份,EPERM 回落 |
| **同文件夹多 session 并发** | 一致:无锁、cwd 当参数、**永不 chdir** | ✓ 全程无 Chdir;fs 收 workdir;消息 per-conversation mutex;LYRA.md 单 mutex | **已安全**。同文件改同文件 = last-write-wins(三家同,且 lyra 原子写不会写坏) |
| **网络盘 / 跨盘** | Claude 有 EXDEV 回退;kimi 无 | `atomicWriteFile` 用**同目录 sibling temp** + rename | **已安全**:sibling temp 保证同卷,EXDEV 结构上不可能。网络盘只是慢,不特判 |
| **权限不足** | 一致:优雅降级 | ✓ agentdoc best-effort 跳过;fs 工具返回 error 给 model | **已优雅**。小补丁:`~/.lyra` 不可写时给清晰错误(学 Codex 把 EACCES 当 actionable) |

**头条:6 个边界里 5 个 lyra 现状已正确**(no-git / 并发 / 跨盘 / 权限,外加改名结构性更优);**唯一动作项是符号链接改 best-effort resolve**。

---

## 6. 对协议重设计的影响(下一步的输入)

> 以下是 wire 协议变更点,前后端协同。**改 `protocol.Session` / `ServerInfo` / `sessions.*` 是破坏性公开 API 改动,
> 按约定动手前先列 scope 给用户确认。**

| 变更 | 字段 | 作用 |
|---|---|---|
| `protocol.Session` += `cwd` | `cwd string` | **主通道**:每个会话属于哪个文件夹;list/get/create 都带;前端按它分组、header 显示 |
| `ServerInfo` += `cwd` | serve 进程 cwd | 还没建 session 时显示"当前文件夹" + 新建默认;`runtime.initialize` + `/v1/info` 都返 |
| `sessions.create` += `cwd` | 可选,空则默认 serve cwd | 前端文件夹选择器传绝对路径 |
| `sessions.update` += `cwd` | 可选 | **relocate**:改名后重指文件夹(语义参照 Codex `ThreadMetadataPatch.cwd`) |
| `workspace.projects` | 改为 distinct-`session.Cwd` 派生视图(+ 可选 name/git) | **不立 Workspace 实体** |

**不做**:Workspace 实体 / 内容派生 id / 按 cwd 路径分桶存储 / per-request 强制带 directory(lyra 走 create-time 传一次 + 存 session,比 opencode 的 header 更简)。

**引擎侧(非 wire)**:cwd 经 `chat.StartTurnRequest` → `engine.RunChatRequest.Cwd` → per-turn 建 fs 工具 + 拼 system prompt(替换构造期单 `workdir`)。因 `fs.NewLocalExecutor(root)` 和 `agentdoc.Discover(cwd, home)` 本就收参数,是"常量改 per-turn 入参",不动结构、**不引入 os.Chdir**。

---

## 7. 已知待办 / 未决

- **LYRA.md 发现没向上走**:现在只读 `<cwd>/LYRA.md` + `~/.lyra/LYRA.md`,而 `AGENTS.md` 走到了 git root。
  为子目录场景连贯,建议把 LYRA.md 发现也改成"从 cwd 上溯 projectRoot",与 AGENTS.md 对齐。
- **fs path-jail confinement**(`tools/fs/local.go` 的 `TODO(security)`):独立于本 doc 的身份解析,用词法规范化,以后单独做。
- **`~/.lyra` 不可写**:给 actionable 错误而非 cryptic panic。
- **同文件夹并发的 LYRA.md 提取**:可能追加重复事实(语义去重,非损坏,低优先)。
- **多根(multi-root)**:Codex `runtime_workspace_roots` / cherry `accessible_paths[]` 有;lyra 先单 cwd,YAGNI。

---

## 落地批次(待用户确认 scope 后逐批 commit)

1. **wire + 透传**:`session.Session.Cwd` + `protocol.Session.cwd` + `ServerInfo.cwd` + `sessions.create` 收 cwd(默认 serve cwd)+ best-effort EvalSymlinks 规范化 → 前端可显示 cwd。
2. **引擎 per-session 解析**:`RunChatRequest.Cwd` 透传 → per-turn fs 工具 + AGENTS.md/LYRA.md 按 session cwd;LYRA.md 发现对齐向上走。
3. **relocate + 派生视图**:`sessions.update(cwd)` + 缺失降级标记 + `workspace.projects` 改 distinct-cwd 派生。
