# 旁路 API 调研 R2 · 前端回应(§3 四开放项)· `2026-06-10`

> 回应 [`AUX_API_DESIGN_STUDY_R2.md`](./AUX_API_DESIGN_STUDY_R2.md) §3 的四个开放项。回答前对涉及的三家做了源码核对
> (Claude Code `src/types/permissions.ts`、cline `apps/vscode/src/integrations/checkpoints/`、kimi-code
> `packages/protocol/src/ws-control.ts`),有两处事实补充影响答案,先记 delta 再逐项答。
> **总体:R2 定论全部赞成,四个开放项都有明确答案,后端可以开写 §4 两份规范。**

---

## 0. 源码核对的两处 delta

1. **Claude Code 的 destination 是五层,不是三层**(`permissions.ts:88`):
   `userSettings | projectSettings | localSettings | session | cliArg`。R2 §2.3 的 `session|project|user` 丢了
   **`localSettings`**——"项目内、但不进版本库"(对应 `.claude/settings.local.json`)。团队场景里"我个人在这个项目里
   总是允许 X,但别替队友决定"是真实需求。不必 v1 就做,但**枚举请留 additive 余地**(见 §2 答案)。
   另:它的 `PermissionUpdate` 是完整规则引擎(addRules/replaceRules/removeRules/setMode/addDirectories)——
   那是 config 域的事,**审批响应里不要搬**(下文 §2)。
2. **kimi 的 `resync_required` 是 per-session 的**(`ws-control.ts:178`):
   `{ session_id, reason: buffer_overflow | session_recreated, current_seq }`;重连 `client_hello` 带
   `last_seq_by_session`,`hello_ack` 回 `resync_required: string[]`(哪些 session 需要重拉)。
   它需要这套精度,是因为把 run 流和旁路混在同一条 WS 上;**我们分层之后旁路不需要 seq**(见 §3 答案)。

---

## 1. checkpoints v2 的 `restoreType`(开放项 1)

**三态够**,与 cline 完全同构(`task`=history / `workspace`=files / `taskAndWorkspace`=both)。逐点:

- **"编辑某条消息重跑" → `history`**,确认。文件去留是用户的事,UI 在确认对话框里并列勾选"同时还原文件改动"
  (勾上 = `both`),不做第四态。
- **`both` 的失败语义请在规范里钉死,前端偏好原子**:cline 的实现是 files 还原可以局部失败
  (`didWorkspaceRestoreFail`)而 history 截断照常——部分成功的中间态("对话回去了,文件一半回去了")在 UI 上
  几乎无法诚实表达。建议:**files 先行,files 失败则整体失败、history 不动**,返回明确错误;
  绝不静默降级成 `history`。
- **files / both 还原前自动 `track()` 一次当前态**(opencode 模式)——成本就是一次 `write-tree`,
  换来 unrevert 免费可逆。建议直接进 v2 规范,不要留"还原不可撤销"的尖角。
- `history`-only 路径上 v1 的 `residualDiff` 语义保留不变(RESPONSE §2)。

## 2. 审批 scope(开放项 2)

- **不要 `once`**:"仅本次"就是普通 `approve`、不带任何记住语义——它是缺省,不是 scope 枚举的一员。
  加进枚举反而让"没勾记住"变得有歧义。
- **三层够,但命名建议 `session | project | global`**(而非 `session|project|user`):runtime 协议零 user 概念
  (API.md §0/§13),"user"这个词会泄漏租户语义;`global` 落在 runtime 的 home 层,与现有 `MemoryScope`
  (`cwd | projectRoot | home`)的词汇能对上,不引入第三套层级词。将来要区分"项目内个人不入库"
  (Claude Code 的 `localSettings`)时 additive 加 `projectLocal`,不破坏既有值。
- **形状建议比 R2 §2.3 再收一档(KISS)**:`updatedPermissions { behavior: allow|deny|ask, scope }` 里的
  `ask` 在审批响应语境没有意义(响应本身就是那次 ask 的回答),`behavior` 与 `decision` 也重复。建议:
  ```jsonc
  // ApprovalResponse(§6.1)增量
  { "type": "approval", "decision": "approve" | "deny",
    "remember"?: { "scope": "session" | "project" | "global" },   // 缺省 = 仅本次
    "editedArgs"?: { ... } }                                       // 已有字段,即 Claude Code 的 updatedInput
  ```
  完整规则引擎(addRules/setMode/目录授权)是 config 域将来的事,别在审批响应里开口子。
- **`updatedInput` 无需新字段**:wire 上 `ApprovalResponse.editedArgs` 已存在且同义,后端实现即可。
  前端审批卡片本来就有参数编辑入口(`useApprovalSubmit` 链路),`remember` 只是加一个下拉:
  本次 / 本会话 / 本项目 / 所有项目。

## 3. 旁路通道 `resync_required`(开放项 3)

**两种重拉并存,不冲突,各管各**:

- **常态 = 按事件类型局部重拉**:收到 `skills.changed` 只失效 skills,`mcp.statusChanged` 只失效 mcp 条目。
  前端所有旁路数据都已走 react-query(query key 即域),`invalidateQueries({ queryKey })` 一行的事。
- **`resync_required` = 兜底信号,语义就是全量**:它表达的是"我丢过事件、不知道丢了什么"(有损档溢出/重连),
  此时按类型局部重拉在逻辑上不成立。前端拿到后失效**全部旁路 key 集合**——同样一行,代价是几个面板
  各自懒重拉(不可见的面板等到展示才拉),完全可接受。
- **所以 v1 不需要 seq**:kimi 的 per-session seq + `last_seq_by_session` 是把 durable 流和旁路混在一条 WS 上的
  代价;我们 run 流已有自己的续传(`Last-Event-Id`),旁路分层后天然是"变了→重拉",
  `resync_required` 空载荷(或带可选 `domains?: string[]` 提示,有就局部、没有就全量)即可。
  **请不要把 seq 机制引进旁路通道**——那是为不需要的精度付复杂度。

## 4. MCP 状态枚举(开放项 4)

`connecting | connected | disconnected | failed | needsAuth` **够用**,五态 UI 映射清晰:
connecting=spinner、connected=绿、disconnected=中性灰(主动/配置性断开)、failed=红、needsAuth=黄+「登录」按钮
(衔接将来的 oauth 流)。两个请求:

1. **`failed` 条目请带 `error?: ProblemData`**——tooltip / 详情面板要展示原因,只有一个 "failed" 字面量做不了交互。
2. **`reconnect` 期间的推送顺序请保证 `connecting → (connected | failed | needsAuth)`**——前端不自造过渡态,
   按钮的 loading 态直接绑 `connecting` 事件,终态解除。这也回答了为什么 statusChanged 推送和 reconnect 必须同批落地。

---

## 5. 对 §4 两份规范的态度

顺序赞成(通知通道规范先行 gate 三项;git 最自包含先落)。两点期待:

- **通知通道规范**:包含 §3 的 `resync_required` 兜底语义 + 复用 `optOutNotificationMethods` 按名抑制 +
  连接级边界(断连不补发)的明确措辞;`request_id` 关联那套(R2 §2.2)只在将来真有 server→client 请求时再启用,
  v1 纯 notification 不需要。
- **git 规范**:形状按 RESPONSE §4.1(per-file + `DiffRow`,raw 后置);`features.git` 启动探测 + `vcs_unavailable`
  与"干净仓"三态分离的退化策略,赞——这正是比八家都诚实的地方,值得写进 API.md 正文而不只是补丁文档。

> 四个开放项至此全部闭环。规范出来后前端按 `BACKEND_CAPABILITIES.md` 同款节奏对接;审批 `remember` 下拉和
> rollback 确认框的 UI 可以先按本文形状画原型,字段名等规范定稿对齐。
