# 前端 → 后端建议 · wire 类型从 Go SSOT codegen · `2026-06-14`

> 来源：一次对照 **Codex（codex-rs）** 与 **OpenCode** 等老牌 agent 客户端的"前后端通信 / API 参数设计"研究。结论是 Lyra 协议在多数轴上已追平或领先，但有**一条结构性差距**值得提给后端——也正好赶在 613（B7–B13）扩面、手写镜像成本上升的当口。
>
> 本文只提**方向 + 选项 + 约束**，实现归后端（你们持有 Go protocol SSOT）。

---

## 1. 问题：TS wire 类型是**手写镜像**，靠人防漂移

- 前端 `frontend/src/rpc/shapes.ts` 顶部自承：*"Type naming follows the backend Go `lyra/rpc/protocol` interface as the mechanical SSOT; this file is the zero-mapping TS projection."*
- 但这个 "zero-mapping projection" **是人手敲的**。后端改一个字段、加一个方法，前端要手动跟一遍。
- **613 把这个成本放大了**：B7–B12 的 wire 契约（~16 方法 + 一批 shapes + feature 位）是前端**逐行手写**接进 `shapes.ts` / `methods.ts` 的。每加一批 = 更多手写镜像面 = 更多"两边悄悄不一致"的漂移点。漂移在 TS 编译期**抓不到**（前端类型自洽、但可能跟后端真实 wire 不符），只在运行时炸。

---

## 2. 两家老牌儿怎么做的（取其精华）

**都不手写 wire 类型——都 codegen。**

- **Codex（codex-rs）**：协议结构体用 `#[derive(TS)]`（[ts-rs](https://github.com/Aleph-Alpha/ts-rs)）+ `#[derive(JsonSchema)]` 从 Rust **直接生成 TS 类型 + JSON Schema**。enum 的 `#[serde(tag="type", rename_all="snake_case")]` 同时驱动 wire 形状与生成的 TS 判别联合。证据：`codex-rs/protocol/src/protocol.rs`（`EventMsg` / `Op` 全部 `derive(TS)`）。
- **OpenCode**：用 Effect `Schema` 作单一 SSOT → `OpenApi.fromApi()` 生成 OpenAPI 3.1 → `@hey-api/openapi-ts` 生成 fetch SDK + TS 类型。一份 schema 同时是**校验 + 文档 + 客户端类型**。证据：`packages/core/src/v1/session.ts`（Part/ToolState 判别联合）+ `packages/sdk/js/script/build.ts`（codegen）。

→ 两条路线不同（Rust-derive vs Schema-first），但**共识一致：wire 类型由 SSOT 机械产出，人不手写**。这正是 Lyra 与它们唯一的结构性差距。

---

## 3. 提案：让"机械镜像"变成真·机械

后端从 `lyra/rpc/protocol` 的 Go 结构体 codegen 出前端消费的 TS 类型（对应当前的 `shapes.ts`），前端删掉手写镜像、改 import 生成产物。两条可选路线：

- **A · Go → TS 直生成**（对标 Codex 的 ts-rs）：用 [`tygo`](https://github.com/gzuidhof/tygo) 这类 Go→TS 工具，从 protocol package 生成 `.ts`。Go 的 struct tag（`json:"..."`）已是 wire 形状 SSOT，tygo 直接读。判别联合需要约定（见 §4）。
- **B · Go → JSON Schema → TS**（对标 OpenCode 的 Schema-first）：后端导出协议的 JSON Schema（method 表 + 类型），前端用 `json-schema-to-typescript` 生成。额外收益：JSON Schema 还能喂 §5 信任边界的运行时校验（前端现在用 Zod 手写校验 RunEvent 信封，可由 schema 生成）。

**选哪条由后端定**——A 更轻（一步到位 TS），B 更通用（schema 还能复用到校验 / 文档 / 其他语言客户端）。

---

## 4. 约束（结合 Lyra 协议原则 + 避开 OpenCode 踩的坑）

codegen **不能**破坏我们已经做对的东西：

1. **判别联合要保真**：Go 的 tagged union（如 `Item` 按 `type`、`ContentBlock` 按 `type`、`StreamEvent` 按 `type`）必须生成 TS 的 discriminated union（`{ type: "x", ... } | { type: "y", ... }`），而不是把所有字段塞进一个 optional 满天飞的大对象。**别学 cline 的 `field?` 假多态。**
2. **typed-prefix ID 保留**：Go 的 id newtype（`ses_`/`run_`/`item_`）应生成 TS branded string（`type SessionId = string & {…}`），保住编译期防混淆。**别学 Codex 的裸 `String` ID。**
3. **transport metadata 不进生成的 body 类型**：trace / auth / idempotency 走 header / `context.Context`（TRANSPORT §2），codegen 只覆盖 business params。**别学 Codex 把 `trace` 塞进 Submission body。**
4. **shape 在源头定清楚，别后处理**：OpenCode 因为 `Schema.optional → anyOf:[T,null]` 不合 SDK 口味，生成后写了 100+ 行补丁去修（`public.ts:82-175`）。我们要在 Go 侧把 `omitempty` / 指针语义定对，让生成产物**开箱即用**，不要生成后再补丁。
5. **单一 error 模型保留**：`ProblemData{type, channel, detail}` 单一来源、按符号 `type` 分支——这是我们**强于** Codex（三层割裂 error）的地方，codegen 不要引入第二套 error 形状。

---

## 5. 附带两条小建议（顺带可做，非必须）

- **update 类方法的 keep/set/unset 三态**（对标 Codex 的 `Option<Option<T>>`）：`sessions.update` 这类 PATCH，区分"不改 / 设值 / 清空"。Go 侧用指针 + 明确语义（`nil`=不改、显式空=清）；前端 `field?: T | null`。避免"没提到的字段被静默改"。
- **turn 计时后端盖戳**：`run.finished`（或 RunResult）带 `timeToFirstTokenMs` / `durationMs` / `modelContextWindow`。Codex 在 `TurnComplete` 上盖了这些；Lyra 现在 TTFT 是**前端自测**（状态栏 throughput），后端权威值更准、更省前端。

---

## 6. 去其糟粕（给后端设计 613 + 未来方法时的护栏）

对照下来，这些是老牌儿背着、**我们已经躲过**的坑——继续躲：

| 坑 | 谁 |
|---|---|
| 三层割裂 error（transport err / event err / 分类 enum 各一套） | Codex |
| 裸 string ID 无类型前缀 | Codex |
| v1/v2 双 API + legacy alias 长期并存 | Codex |
| optional 字段假装多态（靠运行时 narrow） | cline |
| 无版本判别的双格式信封（靠 context 猜 V0/V1） | OpenHands |
| 生成后 100+ 行补丁修 schema 形状 | OpenCode |

**613 本身做得对**（additive feature 位、不引双形状、判别联合、显式 `cwd`）——保持。

---

## 7. 收益小结

- 消除 `shapes.ts` 手写漂移：后端改结构体 → 前端类型自动跟，编译期就锁一致。
- 613+ 新方法**零额外手写镜像成本**：加方法 = 后端定义 + 重跑 codegen，前端 import 即用。
- 前后端类型由**同一 SSOT** 保证，不再"两边各维护一份、靠人对齐"。

> 实现细节（工具选型、生成产物落哪、CI 接线）归后端。前端这边：codegen 一旦有产物，删 `shapes.ts` 手写镜像、改 import 生成物即可，迁移成本低（类型同构）。
