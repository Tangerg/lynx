# 旁路 API 草案 v0.1 · 前端终审 · `2026-06-10`

> 终审 [`AUX_API_DRAFT_SPEC_v0.1.md`](./AUX_API_DRAFT_SPEC_v0.1.md)。两部分:**① 吸收核对**——上轮 REVIEW 的
> 14 项 ⛔/⚠/✎ 逐条对照,**全部正确吸收,无走样**(尤其 E2 选了"v1 只 session"这个最诚实的选项,好);
> **② 对修订引入的新结构再做一轮对抗**——发现 **2 处形状级(N1/N2)+ 2 处行为级(N3/N4)+ 5 处措辞级**,
> 都很小,但 N1/N2 动字段,趁没写代码改掉。
>
> **结论先行:放行。** B1 / B4 / B5 可立即开写(N2 是 B4 的一行参数改动,随写随改);B2/B3 把 N1/N3/N4/N5
> 吸收进措辞后开写。不需要再出 v0.2 评审轮——以下条目后端确认即终稿。

---

## ① 吸收核对(全数通过)

A(watch 并入 subscribe)✓ · B(单方法+联合)✓ · C1(DroppedRun 包装)✓ · C2(inclusive-keep + root-only +
interrupt 清理)✓ · C3(删 residualDiff)✓ · C4(session_busy)✓ · C5(砍 items.edit + checkpoints 语义改写)✓ ·
D1-D5(git 五边界)✓ · E1(key=工具名 + editedArgs 一次性)✓ · E2(v1 只 session,additive 后补)✓ ·
E3(deny+remember)✓ · F(serverChanged 改名+任何字段变即发)✓ · G1-G4 ✓

## ② 新一轮发现

### N1 ⚠ [B2] `subscribe` 的 watch 缺 cwd 锚 —— 多项目下一条流装不下

§2.1 说「path 相对 cwd」,但 `workspace.subscribe` **没有 cwd 参数**;同时 §2.1 又规定「整个 app 共享一条
workspace 流」。app 可以同时开着多个不同 cwd 的会话(多项目),一条流 + 单一隐式 cwd 无法监听两个项目。
**修法(一行)**:cwd 下放到每个 watch 条目:

```jsonc
workspace.subscribe { watches?: [ { watchId: string, cwd?: string, path: string } ] }
// cwd 缺省 = serve 目录(与 §7.5 其余方法一致);jail 按各自 cwd 判
```

单流多项目自然成立,`files.changed` 的 `paths` 相对**该 watch 的 cwd**(措辞同步改)。

### N2 ⚠ [B4] `rollback` 缺「丢弃全部」形态 —— 编辑第 1 轮无解

`toRunId` = inclusive-keep,「编辑第 K 轮」传第 K-1 轮 id——**K=1 时没有第 0 轮可传**,而"编辑第一条消息重跑"
是最常见的编辑场景之一。**修法(一字)**:`toRunId` 改可选:

```jsonc
sessions.rollback { sessionId, toRunId? }   // 省略 = 丢弃全部 root run,回到空会话
```

### N3 ⚠ [B3] `mcp.serverChanged` 没覆盖「条目增删」

「条目任何字段变化即发」管不到 server 被加进 / 移出配置(config reload)。事件形状里 `status` 是必填,
而"已移除"不是一种 status。**修法**:`status` 改可选 + 一句话语义:

```jsonc
{ type: "mcp.serverChanged", server: string, status?: McpStatus, ... }
// 条目增、删、任何字段变化均发;status 缺省 = 条目已不存在(前端重拉自知)
```

前端消费本来就是"该 server 变了 → 失效 mcp-servers + mcp-tools 两个 key",字段只是 loading 态的提示
(`connecting` 绑按钮),改可选无副作用。

### N4 ⚠ [B2] `watches` 给了但 `features.fileWatch: false` 的行为未定义

subscribe 服务三族事件,`fileWatch` 只门控 watches 参数。feature 关着还带 watches:**接受但不监听 = 债**
(E2 同款论证)。钉死:`fileWatch:false` 时带 `watches` → `capability_not_negotiated`;不带 watches 的
subscribe(只要 skills/mcp 事件)**始终可用**、不受该位门控。

### N5 ✎ [B2] `resync.domains` 词表未定义 —— v1 建议删掉

`domains?: string[]` 的合法取值从未列出("skills"?"mcp"?"files"?),后端发不出有意义的局部值,前端也无从映射。
**v1 直接去掉 `domains`**(`{type:"resync"}` 裸 = 全量失效,一行 invalidate),additive 后补——比留一个无词表的
开放字段干净。

### N6-N9 ✎(各一句话,落规范时带上)

- **N6** 订阅竞态:规范建议「**先开订阅、再拉列表**」(订阅前变更不补发,先订后拉则无丢失窗口)。
- **N7** 类型命名不变量:optOut 按 type 名跨 run/workspace 两个联合匹配,**type 名必须跨联合唯一**
  (现状成立:`item.delta` vs `files.changed`;写成一句约束防将来撞名)。
- **N8** fork 运行中的「快照语义」钉死:**只复制已完结 run**(in-flight run 不进副本;codex「如同先 interrupt」
  实际含义)。
- **N9** v0 的「InProcess/IPC 上即 notification 回调」这句在 v0.1 被删了——保留它(传输无关措辞,§5 同款)。

---

## 放行判定(终)

| 批次 | 判定 |
|---|---|
| **B1 git** | **开写**(无新发现) |
| **B2 通道** | N1(watch 带 cwd)、N4、N5、N6/N7/N9 措辞吸收后**开写** |
| **B3 推送/MCP** | N3(status 可选+增删语义)吸收后随 B2 |
| **B4 rollback/fork** | **开写**,N2(toRunId 可选)+ N8 随写随改 |
| **B5 审批** | **开写**(无新发现) |
| **去债 background** | **开写**;前端清单(v0.1 §9)就绪,等后端批次落地按 `BACKEND_CAPABILITIES.md` 节奏同轮动 |

> 本轮 9 条全部是修订引入面上的小修,无结构异议。后端确认 N1-N5 后**无需再发评审件**,直接落 API.md 正文,
> 前端按 §9 清单跟进。四轮调研 + 三份评审到此收口。
