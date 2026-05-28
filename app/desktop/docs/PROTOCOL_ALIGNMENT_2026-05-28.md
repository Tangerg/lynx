# Protocol alignment — 前后端 schema 对齐决议 (2026-05-28)

> 后端实现 (lynx/lyra/pkg/) 跟前端 Lyra Runtime Protocol (docs/API.md +
> docs/TRANSPORT.md) 第一轮对齐结果。
>
> 背景：后端工程师做完骨架后，对自己实现 vs 前端协议定义做了一轮
> 14:2:4 的逐项比对（前端更优 14 处 / 后端更优 2 处 / 各有道理 4 处），
> 提议 95% 跟前端走，2 处反向 push back。本文档记录对两条 pushback 的
> 最终决议 + 完整动作清单。

---

## 决议

### #1 `sessions.fork` 用 `parentId` 而不是 `id` —— **接受后端的提议**

**理由**：

- 所有其他 `sessions.*` method（`get` / `update` / `delete`）的 `id`
  都是"被直接操作的对象"。`fork` 不一样 —— `id` 不是"被 fork 的
  目标"，而是**新 session 的来源**。同一个参数名表达不同的语义关系
  = 类型签名说谎
- callsite 上 `fork(currentSessionId, msgId)` 读不出"这个 id 是新
  session 的还是源 session 的"，`fork(parentId, atMessageId)` 一眼明白
- 跟 git fork / GraphQL Connection 的 `parent` / linked-data 的
  `derivedFrom` 同类命名习惯
- 改动成本几乎为零（前端 `methods.ts` 一行字段重命名 + spec 同步）

**动作**：spec / 前端 wire / 后端 wire 三处同步改成
`{ parentId: string, atMessageId: string }`。

---

### #2 `workspace.mcp.reconnect` 用 `name` —— **接受，但要顺便清理 `MCPServer` shape**

**论据**：后端折中说"倾向跟前端走（用 id），代价小"。我反而觉得这一条
**问题不在 method 字段名，问题在 `MCPServer` 形状本身**。

现状：

```ts
interface MCPServer {
  id: string;        // ← 这是什么？
  name: string;      // ← MCP 协议本身的 server 标识
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}
```

`id` 和 `name` 都是 string。两种可能：

- (a) `id === name`（slug-style alias）—— 冗余
- (b) `id` 是 Lyra 自己分配的 UUID、`name` 是 MCP 协议标识 —— 但 MCP
  server 在 Lyra 实例内 name 已经唯一（不然 lookup 必然冲突），没理由
  再分配 Lyra-side ID

**两种情况下 `id` 字段都没存在的理由**。它是 REST mock 时代继承下来的
字段，没经过协议层审视。

**推荐做法**：

1. **`MCPServer` shape 去掉 `id`**，保留 `name` 作为唯一标识符
2. 真要 pretty label 加一个可选 `displayName?: string`
3. `workspace.mcp.reconnect` wire key 用 `name`

收益：

- ✅ 跟 MCP 原生协议对齐
- ✅ 后端不用做 `id → name` 映射，零适配成本
- ✅ 形状本身更干净
- ✅ 不破坏后端"按 name 查"的实现

**最终 shape**：

```ts
interface MCPServer {
  name: string;              // MCP server identifier (== MCP 协议 name)
  displayName?: string;      // optional human-readable label
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
}
```

**动作**：spec §6 修 `MCPServer` 形状 + method §5.2 改 `workspace.mcp.reconnect` 字段名。

---

## 其他对齐项（后端 95% 同意的部分，列在这里做对账）

| 项 | 决议 | 动作方 |
| --- | --- | --- |
| 非分页 list 返裸数组 | ✓ 按前端 | 后端去掉 `{items}` 包装；分页方法（`sessions.list` / `messages.list`）保留 `Page<T>` |
| `ApprovalDecision` = `approve` \| `deny` | ✓ 按前端 | 后端简化 enum（去掉 "remember" 类语义） |
| `ContextItem` discriminated union | ✓ 按前端 | 后端用 Go struct + 可选字段表达，不要 `map[string]any` |
| `Session.Metadata` = `Record<string, unknown>` | ✓ 按前端 | 后端 wire 不约束 value 必须 string |
| `workspace.diff` 返结构化 `DiffRow[]` | ✓ 按前端 | 后端解析 unified diff，前端拿到结构化数据 |
| `workspace.fileHead` 返 `FileLine[]` | ✓ 按前端 | 同上 |
| `messages.edit` 用 `content`（非 `newContent`） | ✓ 按前端 | 后端改字段 |
| `background.stop` 用 `taskId` | ✓ 按前端 | 后端改字段 |
| P2 形状（`FileChange` / `TermLine` / `MCPServer` / `Provider` / `Model` / `BackgroundTask` / `Project` / `Skill`） | ✓ 按前端 | 后端按前端 shape 调整字段 + enum |
| `workspace.grep` 返 `{matches, total}` | ✓ 保持现状 | 不动（已经实用，将来加 `nextCursor?` 不破坏） |
| `sessions.export` 返 `{url}` | ✓ 按前端 | 后端需要一个 file-serving endpoint |
| `tools.list` 含 `SafetyClass` 多字段 | ✓ 保留 | 后端多给字段，前端忽略未知字段，向前兼容 |
| JSON-RPC `id` 类型 string \| number | ✓ 无冲突 | 后端 `json.RawMessage` 接两种，前端 number 递增 |

---

## P1（spec 要求但后端骨架暂缺）—— 上 prod 前必补

按 docs/API.md 协议规范，下列项后端需要补齐（dev 阶段不阻塞）：

1. **版本协商**：`runtime.initialize` 需读取 `in.ProtocolVersion`，不支持时
   按 spec §2.2 规则返不同版本或 `ErrInvalidProtocolVersion`。当前
   `coreimpl/lifecycle.go` 是 no-op pass-through。
2. **Capability 协商**：`dispatch.go` 需存储 client capabilities，后续
   emit 时按 client 声明过滤，未声明的 feature 不发对应事件。
3. **HTTP transport 层错误**：
   - `401 Unauthorized` 扁平 JSON（本地进程门禁 token 校验失败时）
   - `500 Internal Server Error` 扁平 JSON（非 panic 路径的 internal_error）
   - `503 Service Unavailable` 扁平 JSON（`/v1/health` 真实 probe 失败时）
   当前 spec §7.3 三条都缺，骨架只对 panic recovery 走扁平 JSON
4. **Observability §10**：
   - `X-Lyra-Request-Id` server 缺失时自动生成（当前仅 echo client header）
   - 结构化日志强制字段 `method` / `id` / `duration_ms` / `error_code` /
     `bytes_in` / `bytes_out` / `trace_id`（当前仅 `>=500` 才输出且非 JSON）
   - Prometheus metric `lyra_rpc_request_total{method,error_code}` /
     `lyra_rpc_duration_seconds{method}`（当前零实现）
5. **`/v1/health` 真实 probe**：当前永远返 200 ok，缺真实组件健康检查
6. **Cursor 分页**：`coreimpl/sessions.go` 收到 `q.Cursor != ""` 直接返
   `-32601`。spec §6.4 要求支持游标续翻

---

## P2（可在跑通后清理）

1. `MethodRunsCancel` 常量声明但未使用（dead code）
2. `pkg/transport/http/clients.go` SSE 广播是全 fan-out，没按
   `streamHandle` 路由 —— 功能正确（前端按 handle 过滤兜底了）但带宽浪费
3. `pkg/transport/http/replay.go` 的 `compareEventID` 实现 hacky，建议
   换成 `strconv.Atoi` 比较
4. `genID()` 用 UUID v4 而非 v7（spec §6.3 期望 v7，但允许任何 UUID，不阻塞）

---

## P0（前端调用必失败的 dispatch 字段漂移，最高优先级）

按上面 #1 / #2 决议 + "非分页 list 返裸数组" 一并落地后，下列 4 项 P0
自动消失：

1. `dispatch.go` 给非分页 list 方法包 `{"items": ...}` → 改成 return 裸数组
   （分页方法 `sessions.list` / `messages.list` 保留 `Page<T>`）
2. `ForkSessionIn.ParentID` 已是后端正确字段，前端 `methods.ts` 跟着改
3. `EditMessageIn.NewContent` → `Content`
4. `WorkspaceMCPReconnect{Name}` 已是后端正确字段，前端 `methods.ts` +
   `MCPServer` shape 跟着改

---

## 下一步谁动什么

### 后端

- [ ] dispatch 层去掉非分页 list 方法的 `{items}` 包装（约 8 处）
- [ ] `EditMessageIn` 字段 `NewContent` → `Content`
- [ ] 按前端 shape 调整 P2 类型（FileChange / TermLine / MCPServer / Provider / Model / BackgroundTask / Project / Skill 的字段名 + enum）
- [ ] `MCPServer` 形状去掉 `id`，加可选 `displayName`
- [ ] `Session.Metadata` 改成 `map[string]any`（或 `json.RawMessage`）
- [ ] `ContextItem` 用 Go struct + tagged 字段
- [ ] `workspace.diff` / `workspace.fileHead` 解析成结构化数据
- [ ] `ApprovalDecision` 简化为 approve/deny enum
- [ ] `sessions.export` 实现 file-serving endpoint 返 `{url}`
- [ ] **P1 6 项**（版本协商 / capability / 401/500/503 / observability / health probe / cursor 分页）—— 上 prod 前

### 前端

- [ ] `methods.ts` `sessions.fork` 第一参数命名 `parentId`，wire 同步
- [ ] `methods.ts` `workspace.mcp.reconnect(name: string)`，wire 同步
- [ ] `shapes.ts` `MCPServer` 形状去 `id`，加可选 `displayName`
- [ ] 等后端真发 `POST /v1/rpc/{method}` 后做 cutover PR（迁 queries /
      HttpPermissionGateway / http-agent，调 `runtime.initialize`）

### Spec / docs

- [ ] `docs/API.md` §5.2 `sessions.fork` 参数表标 `parentId`
- [ ] `docs/API.md` §5.2 `workspace.mcp.reconnect` 参数表标 `name`
- [ ] `docs/API.md` §6 `MCPServer` shape 修正
- [ ] `docs/API.md` §5.2 加 "Returns" 列明确每个 method 返裸值 / `Page<T>` /
      `void`，让"哪些 list 分页"零歧义
- [ ] 本文档（PROTOCOL_ALIGNMENT_2026-05-28.md）作为决议归档，spec 更新
      完成后可标 `[CLOSED]`

---

## 备注

后端这份 14:2:4 自评是个值得复用的 review 模板：

> 收到对方设计 → 不是问"能不能改成 X"，而是逐项比对 → 给量化判断 →
> 自己先承认弱势项 → 只对真的觉得自己更对的点反向 push back → 提议
> 简短确认 message

比"被动接受"或"对线吵架"都好。后续两边互 review 设计时可以照这个模式。
