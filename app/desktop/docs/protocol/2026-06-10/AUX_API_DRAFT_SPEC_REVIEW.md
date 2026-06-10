# 旁路 API 草案 v0 · 前端评审(三轮查漏)· `2026-06-10`

> 评审 [`AUX_API_DRAFT_SPEC.md`](./AUX_API_DRAFT_SPEC.md)。做了三轮:**① 与 `../API.md` / `../TRANSPORT.md` /
> 现有 wire 层的契约一致性核对;② 语义完备性(每个字段在边界 case 下是否有唯一解释);③ 对抗性(并发 / 多客户端 /
> transport 落地)**。结论:**方向与四轮讨论完全一致,可以收口;但有 1 处阻断级、2 处设计一致性、若干语义洞**,
> 修完即可开写。按草案章节组织,标 ⛔(阻断)/ ⚠(须修)/ ✎(钉死措辞即可)。
>
> 点名要确认的三处字段名(getDiff / ApprovalResponse / rollback 响应):**形状方向都对**,具体修正见 §C/§E。

---

## A. ⛔ §2 通道:「连接级」在 streamable HTTP 上没有锚点

草案 §2.1/§2.3 反复使用「连接级」(watchId 连接级作用域、断连即止)。但 **TRANSPORT 自己已经废掉了连接身份**:

> TRANSPORT §2:「不再有连接 id」;§8:「不再有连接级路由——事件天然属于"开它的那条 POST 响应流",无
> `X-Conn-Id`、无 run→连接登记表」。

streamable HTTP 上每个 JSON-RPC 调用是独立请求:**`workspace.watch` 这个独立 POST,服务端无法知道它属于哪条
`workspace.subscribe` 流**。codex 能这么写是因为它跑 stdio/WS(连接=会话);我们不行。两条出路:

- ~~(b) `subscribe` 返回 `subscriptionId`,`watch/unwatch` 携带之~~ —— 等于重新引入 TRANSPORT §8 刚删掉的
  连接登记表,违背自家 transport 哲学。
- **(a) watch 集随 `subscribe` 参数走,流即作用域(建议采纳)**:

  ```jsonc
  workspace.subscribe { watches?: [ { watchId: string, path: string } ] }   // path 相对 cwd,jail 同 §7.5
    → 流式;流的生命周期 = 订阅 + 全部 watch 的生命周期
  // 改 watch 集 = 关流重订(文件监听集合本来就低频变化:面板开关时)
  ```

  `workspace.watch` / `unwatch` 两个方法**整个删掉**——少两个方法、零服务端登记表,与「事件属于开它的那条流」
  完全同构。重订后丢失窗口期事件?事件本就是「变了→重拉」语义,重订时前端无脑失效一次即可(等价收到 `resync`)。

**连带必须写进 TRANSPORT 的两条**:
1. **连接预算**(§6.5 已警告 HTTP/1.1 同 origin ~6 条):workspace 流是**第二条常驻连接**(run 流之外)。
   1 run + 1 workspace + 短连接 RPC,预算内,但要写明**整个 app 共享一条 workspace 流**(多面板复用,不许每面板开流)。
2. **重连语义**:无 `Last-Event-Id`、无补发;重订即隐式 `resync`(前端全量失效一次)。与 run 流的 durable 续传
   明确分写成对照表,杜绝实现时混淆。

## B. ⚠ §2.2 事件方法形态:与协议自己的既有模式矛盾

草案给每个事件单开 method(`notifications.workspace.files.changed` …),理由是「精确 optOut」。两个事实:

1. **API.md §5 的既有定论是「一个下行事件方法」**——run/item/state 全部事件走 `notifications.run.event`,
   类型在 payload 里(`event.type`)。
2. **`optOutNotificationMethods` 的粒度本来就是事件类型**:API.md §9 自己的示例是 `["item.delta"]`——
   那是 StreamEvent 的 type,不是 JSON-RPC method 名。「每事件一 method 才能精确 optOut」的前提不成立。

**建议**:对称复用 run 流的形态——**一个 `notifications.workspace.event`,payload 带类型联合**:

```jsonc
notifications.workspace.event  { event: WorkspaceEvent }
WorkspaceEvent =
  | { type: "files.changed",     watchId: string, paths: string[] }   // paths 相对 cwd(jail 词汇统一)
  | { type: "skills.changed" }
  | { type: "mcp.statusChanged", server: string, status: McpStatus, toolCount?: number, error?: ProblemData }
  | { type: "resync",            domains?: string[] }
```

收益:与 §5 一致(前端的 fold/派发基建直接复用,handler chain 按 type 路由);未来加事件 = 加联合成员,
不动方法表;optOut 继续按 type 名抑制。若后端坚持 per-method 也能用,但请先回答「为什么 workspace 域要偏离
run 域的形态」——目前草案没有给出理由。

## C. §5 checkpoints:四个语义洞 + 一处形状错误

1. **⚠ `droppedRuns: RunRef[]` 装不下 userMessage**——`RunRef`(§4.2)没有任何 input 字段,草案注释
   「每条带 userMessage 文本」与形状矛盾。修正:

   ```jsonc
   droppedRuns: [ { run: RunRef, userInput?: ContentBlock[] } ]
   // userInput = 该 run 开场 userMessage 的 content(resume/edit 类续 run 没有开场用户轮,缺省)
   ```

   用 `ContentBlock[]` 而非 string:与 `StartRunRequest.input` 同型,前端预填 composer 零转换。

2. **✎ `toRunId` 语义钉死**:建议 **inclusive-keep**(`toRunId` 是**保留的最后一个** root run,其后全部丢弃)。
   「编辑第 K 轮重跑」= 传第 K-1 轮的 runId(前端从 `items.list.runs` 可得)。同时写明:
   `toRunId` **必须是 root run**(子 agent run / continuation run → `invalid_params`);丢弃某 run 时其
   continuation 链与 subagent 子树一并丢弃;悬在被丢轮次上的 open interrupt 一并清理(`runs.listOpenInterrupts`
   不再返回)。`fork{fromRunId}` 同语义:**含 fromRunId 在内**截断复制。

3. **⚠ `residualDiff` 在 v1 做不出它名字承诺的东西**:没有快照就**无法归因**「这些脏文件是被丢轮次留下的」——
   worktree 的脏状态混着保留轮次的改动和用户手改。两个诚实选项:(a) **删掉该字段**,文档写明
   「v1 不动文件;UI 提示用户用 getDiff 查看当前未还原改动」(前端自调,反正 B1 先落地);
   (b) 改名 `worktreeDiff` 并明确「当前整个 worktree 的脏状态,不代表归因」。**前端偏好 (a)**——少一个
   耦合字段,rollback 响应保持纯粹。
4. **⚠ 运行中 rollback 的行为缺失**:session 有 run 在跑时 `sessions.rollback` 必须拒绝(截一条正在被
   append 的历史没有好语义)。需要**新 sentinel `session_busy`**(§8.2 码表新增,opencode 同款)。
   `fork` 则**随时可调**(快照语义,codex「如同先 interrupt」同款)——两者差异写明。
5. **⚠ `items.edit` 的命运草案没交代**:turn 粒度定论下,item 精确编辑已无存在理由——「编辑重跑」=
   `rollback` + `runs.start`。建议**砍掉 `items.edit`**(API.md §7.4 删节 + 前端同轮删 wire),
   并且 §9 的 `checkpoints` 能力位语义从「items.edit / fork-at-item」改写为「restoreType(v2 文件快照)」。
   连带:API.md §7.2 fork 的 `fromItemId` 行(L813)、§9 表(L1135)、BACKEND_CAPABILITIES §4 的两行都要改——
   给一个 API.md 改动清单进草案 §8,避免落规范时漏。

## D. §1 git:五个边界 case 需要进规范

1. **⚠ `renamed` 缺 `previousPath`**:`WorkspaceFileChange` / `FileDiff` 只有 `path`,rename 丢源路径
   (UI 显示「a → b」、按旧路径找 diff 都需要)。加 `previousPath?: string`(仅 renamed 时有)。
2. **⚠ 二进制文件**:`git diff --numstat` 对 binary 返回 `-`。`added/removed` 填什么?`rows` 给什么?
   建议:`binary?: true` 字段,added/removed 缺省(不要伪造 0),rows 为空数组。`listFileChanges` 同。
3. **✎ untracked 进不进 `getDiff`?** `listFileChanges` 有 untracked 状态,但 `git diff` 默认不含 untracked。
   前端 diff 面板需要新文件可见。建议:`mode:worktree` 包含 untracked(实现可用 `--intent-to-add` 或逐文件
   no-index),`FileDiff.status: "untracked"` 全文件 added rows;若 v1 不做,**文档必须写明不含**,
   否则「listFileChanges 有这文件、getDiff 没有」会被前端当 bug 报。
4. **✎ `limit` 截断粒度**:总行数上限,**在文件边界截断**(宁可整文件不出现 + `truncated:true`,
   不要半个文件的 rows——半截 diff 比没有更误导)。可选:被截掉的文件以 `rows: []` + 计数字段保留行内统计,
   让 UI 至少能列出「还有哪些文件改了」。
5. **✎ `base` 解析失败路径**:无 remote / 无 `origin/HEAD` / `main`/`master` 都不存在 / detached HEAD——
   返回什么?建议 `invalid_params`(带 detail「无法解析基线分支」),**不要**塌成空 diff 或 `vcs_unavailable`
   (那是「非仓」的语义,§1.1 刚立的三态分离别自己破坏)。

## E. §3 审批:remember 缺了「记的是什么」

1. **⚠ remember 的 KEY 未定义**——`{scope}` 只说了记到哪层,没说**记住的匹配规则是什么**:按工具名?
   按 `name+arguments` 精确匹配?Claude Code 是 `{toolName, ruleContent?}` 两段。v1 建议**就按工具名**
   (`ToolInvocation.name`),写进规范;按参数模式匹配是规则引擎的事,留给 config 域。
   连带写明:**`editedArgs` 是一次性的**,remember 记住的是「这个工具」而不是「这个工具+这次改过的参数」。
2. **⚠ v1 实际履行哪些 scope 要诚实**:`project`/`global` 需要持久化位置(`<cwd>/.lyra/…`?home?),
   若 v1 只实现 `session`(内存)——**接受但不持久化的 scope 比拒绝更糟**(BACKEND_CAPABILITIES §4 对
   feedback.create 的批评原话)。要么三层都落地并写明存储位置,要么 v1 枚举只放 `session`,
   `project|global` 作为 additive 值后补。
3. **✎ `decision:"deny"` + `remember` 合法且有效**(记住拒绝同样是 Claude Code 行为),规范加一句确认。

## F. §4 MCP:动态 tool 变更没有通路

MCP 协议本身有 `tools/listChanged` 通知(server 热更工具列表,**状态不变**)。草案里 `statusChanged` 只在
status 变化时发的话,toolCount 会陈旧、前端 listTools 缓存无从失效。两个解法选一:
- **文档定义宽**:`mcp.statusChanged` 在**条目任何字段变化**时发(含 status 不变、仅 toolCount 变)——
  事件名略名不副实但省一个类型;
- 或事件改名 `mcp.serverChanged`(语义即「这条 server 条目变了,重拉」)。**前端偏好后者**,名实相符,
  且与「变了→重拉」的失效语义一致(前端反正按 server 失效 mcp-servers + mcp-tools 两个 key)。

## G. 杂项(✎ 级,落笔时顺手)

1. **`skills.changed` 的 cwd 语义**:skills 是 per-cwd 的(project 层),事件空载荷时多项目并开的客户端
   只能全量重拉各自 cwd——可接受,但规范写一句「事件不区分 cwd,任何 skill 目录变化都触发」;
   或加 `cwd?: string` 提示。前端两者都能消化,倾向先不带(KISS)。
2. **`features.git` 与 `vcs_unavailable` 的分工写成表**:无 git 二进制 → feature false(前端隐藏面板,不调);
   有 git、cwd 非仓 → `vcs_unavailable`;有 git、是仓、无改动 → 空结果。三行表进 API.md,一目了然。
3. **前端同步动作清单**(草案落地时前端同轮做,列出来供排期):
   `shapes.ts` 补 `ServerCapabilities.streamingMethods` 镜像(API.md §9 已有、前端漏镜像,本次核对发现的
   存量 drift);`Diff` 重写为 `{files: FileDiff[]}`(DiffPreview/DiffView 改造);`McpServer` 状态枚举
   3→5 态 + `MCP_STATUS` 映射更新 + 退掉 listServers⨝listTools join;`WorkspaceFileChange` 加字段;
   `ApprovalResponse` 加 `remember`(审批卡加下拉);删 `background.*` 全家(`BackgroundTask`/`TaskId`/
   `streamBackgroundUpdates`/methods 三件套);fork 参数 `fromItemId`→`fromRunId`。
4. **§8 批次表加一列「API.md 落点」**:B1→§7.5+§8.2;B2→§5(新事件联合)+§9+TRANSPORT §6.4 补节;
   B4→§7.2(fork 改写、rollback 新增、items.edit 删除)+§7.4+§9;B5→§6.1。规范落地=改正文,
   补丁文档只做导读(§1.1 已自我要求,推广到全部批次)。

---

## 结论:各批次放行判定

| 批次 | 判定 |
|---|---|
| **B1 git** | **修 D1-D5 措辞后可开写**(全是规范措辞级,不动结构) |
| **B2 通道** | **等 A(阻断)+ B 定稿**——watch 并入 subscribe、单方法+类型联合,两处都改的是结构 |
| **B3 watch/MCP 推送** | 随 B2;F 的事件改名一并定 |
| **B4 rollback/fork** | **修 C1-C5 后可开写**(C1 形状、C4 sentinel、C5 连带删减是结构级) |
| **B5 审批** | **等 E1/E2 答案**(remember key + scope 持久化诚实性) |
| **去债 background** | 随时可做,前端就绪(G3 清单) |

修订一版(v0.1)后前端无保留放行;A/B 两处若后端有不同取舍,值得一次同步对齐而不是文档往返。
