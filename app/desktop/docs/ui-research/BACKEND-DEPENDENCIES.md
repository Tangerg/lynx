# 后端依赖项（前端先延后，待后端推进）

> 来源：[`GAP-CATALOG.md`](GAP-CATALOG.md) 里需要后端/协议配合才能落地的能力。
> **原则**：前端先不做空壳。每项记录"要后端给什么 + 给了之后前端补什么 UI"，
> 待后端 runtime / 协议（`~/Desktop/lynx` + `docs/protocol/`）支持后再回来接线。
>
> 不在此列的差距（G1/G2/G3/G6/G8/G9/G10/G11）是**纯前端可做**，正在按优先级落地。

---

## B1（=G5）· 真实终端流

- **现状**：`workspace-views/terminal.tsx` 是 fixture——标题/cwd/错误数硬编码，`Terminal.tsx:36` 硬编码 "tsc watching…"，数据走 `lib/data/data.ts:134` 的 `HTTP_KEYS=["terminal"]` HTTP-GET stub。
- **要后端给什么**：
  - 可订阅的**进程/命令输出流**（长驻进程的 live tail，不只是单次 bash tool 的 toolOutput delta）。
  - 进程元数据：title / cwd / status(running|exited) / exitCode。
  - 形态上仍走 JSON-RPC（参考 `runs.subscribe` 的流式语义），**不要**新增 RESTy 端点（违反协议边界，见 `TRANSPORT.md §12`）。
- **给了之后前端补什么**：复用已有的 bash/shell stdout preview 渲染（xterm 或现有 ANSI 渲染），把假 metadata 换成 run/process 派生数据；与 G3 并排面板搭配常驻侧栏。
- **已处置（G8）**：终端视图的 fixture 已撤，改为诚实空态（"Terminal isn't wired up yet"）；
  连带删掉 `useTerminal` / `TermLine` / `views/Terminal.tsx` 渲染器 / `defaults/data.ts` 的
  `HTTP_KEYS` escape hatch（terminal 是其唯一用户）。后端就绪后：在 `terminal.tsx` 接订阅流
  + 复用 bash preview 的 ANSI 渲染。

---

## B2（=G4 / G12 数据源）· 工作区文件枚举 + 搜索

- **现状**：composer 无 @-mention（仅 slash）；Files 视图只有"changed files"扁平列表，无工作区浏览。
- **要后端给什么**：一个**工作区文件枚举 / 模糊搜索**协议方法（在 session cwd 下），理想含：
  - 列目录 / 递归列文件（给文件树 G12 + @file 候选）。
  - 路径模糊搜索（给 @-mention 的 ripgrep-like 体验）。
  - 可选：符号搜索（给 @symbol）。
  - 方法名走点号风格（如 `workspace.listFiles` / `workspace.search`，参考 `API.md` 方法表约定），**不斜杠化、不加 REST shadow**。
- **给了之后前端补什么**：
  - G4：composer 的 `@` 触发器（与现有 `SlashSuggestions` 同构）+ mention chip + **插入前体积预校验**（抄 continue `isItemTooBig`，agent 上下文的关键守卫）。
  - G12：一个 file-tree workspace view（与 G3 并排面板搭配）。
- **注意**：continue/cline 在编辑器进程内用客户端 ripgrep；Lyra 的 runtime 是**远程**的，所以必须走协议方法，不能假设本地文件系统访问。

---

## B3（=G7 压缩动作）· 对话压缩 / 摘要

- **现状**：上下文占用**可见性**是纯前端（数据已有，G1 状态行会常驻显示 context 预算 + 阈值色）；但"压缩对话"的**动作**没有。
- **要后端给什么**：一个**压缩 / 摘要当前会话上下文**的协议能力（参考 cline 的 Compact、continue 的 compaction）。可能形态：`sessions.compact` 或 run 级 summarize，产出一个 compaction 边界（后端已有 `CompactBoundary` 内部事件，见 lynx `translator.go:104`——需暴露为可调用动作 + 可观测边界）。
- **给了之后前端补什么**：context 预算过阈值（如 >80%）时在状态行/composer 给一键"compact"动作 + 内联确认（抄 cline `ContextWindow` 的 Compact + confirm）。
- **前端先做的部分（不依赖后端）**：G1 里把 context 预算做成**常驻、过阈值变色**的可观测，已能让用户提前知道要换话题/手动开新会话。

---

## 不阻塞的提醒（后端无需介入，前端自理）

| 能力 | 为何不阻塞 |
|---|---|
| G1 持久状态行 | 数据（branch/cwd/tokens/cost/context/throughput）前端已有，仅重排呈现 |
| G2 只读工具折叠分组 | reducer 投影 + 组件，纯前端 |
| G6 分级 always-allow | 协议 `InterruptResponse.remember{scope:"session"}` 已支持（`API.md §6.1`），仅前端接线 |
| G3 / G8 / G9 / G10 / G11 | 布局 / 清理 / 样式 / 流式 / 分支，纯前端 |

---

## 落地后回填

后端能力就绪后，回到 [`GAP-CATALOG.md`](GAP-CATALOG.md) 对应条目，按"给了之后前端补什么"接线，并把本文件对应项标记为已解除。
