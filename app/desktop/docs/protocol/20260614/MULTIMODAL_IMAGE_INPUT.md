# Lyra Runtime Protocol · 变更说明 · 多模态图片输入（`2026-06-14`）

> **状态：变更说明（migration note），非独立契约。** 本文记录一次后端→前端的协议变更：**图片输入改为内联 base64，删除整套 attachment 上传子系统**。
> 正式契约仍以同目录上一层的 [`../API.md`](../API.md) / [`../AUX_API.md`](../AUX_API.md) / [`../TRANSPORT.md`](../TRANSPORT.md) 为准——它们已就地更新到本变更后的形态；本文只做「改了什么 + 前端怎么迁」的集中说明。
>
> `protocolVersion` 不变，仍 **`2026-06-07`**（wire 仍向后兼容老 client 的「只实现子集」语义——image block 是 `ContentBlock` 的既有变体，只是数据形态变了；attachment 域本就是 capability-gated 且后端从未真正实现，移除不影响任何在用路径）。

---

## 1. 一句话

图片随 `runs.start.input` 的 **image `ContentBlock` 内联传入**（`mime` + base64 `data`），不再经「先上传拿 `attachmentId` 再引用」那套。对照业界 coding agent（opencode / Claude Code / codex 等）一致做法——全是内联 base64、无独立上传子系统。

---

## 2. `ContentBlock` 形态变更（核心）

`API.md §4.3`。image 变体从「引用 attachmentId」改为「内联 mime + base64」：

```diff
 type ContentBlock =
   | { type: "text";  text: string }
-  | { type: "image"; attachmentId: string };
+  | { type: "image"; mime: string; data: string };   // mime=媒体类型；data=base64（无 "data:…;base64," 前缀）
```

- **`mime`**：图片媒体类型，如 `"image/png"` / `"image/jpeg"` / `"image/gif"` / `"image/webp"`。
- **`data`**：图片字节的**纯 base64**，**不带** `data:` data-URL 前缀(后端按 `mime` + `data` 直接组装成模型所需形态)。
- 一条 `input` 里可混合若干 text block 与若干 image block；后端把 text 合并为用户消息正文、image 作为该消息的媒体附件一并发给模型。

发送示例：

```jsonc
{
  "method": "runs.start",
  "params": {
    "sessionId": "ses_…",
    "input": [
      { "type": "text",  "text": "这张图里有什么 bug?" },
      { "type": "image", "mime": "image/png", "data": "iVBORw0KGgoAAAANS…" }
    ],
    "provider": "anthropic",
    "model": "claude-sonnet-4-6"
  }
}
```

> 回显与回放：该 run 的开场 `userMessage` Item 的 `content` **原样携带** image block（同 `input`），所以 `items.list` / `sessions.rollback` 的 `userInput` 会带上图片，composer 可零转换重填。

---

## 3. 删除的 attachment 子系统（前端需移除对应代码）

整套 attachment 上传/引用域已从契约移除——它一直是 capability-gated 且后端从未落地（`enabled:false`），无任何在用路径：

| 类别 | 移除项 |
| --- | --- |
| 方法（`API.md §7.7`） | `attachments.createUploadUrl` / `attachments.get` / `attachments.delete` |
| 类型 | `Attachment`、`CreateUploadUrl` 的请求/响应 |
| 字段 | `StartRunRequest.attachments`（`string[]`）；`ContextItem` 的 `image` 变体及其 `attachmentId` |
| 能力位（`API.md §9`） | `features.attachments`（`{ enabled; maxSizeBytes?; mimeTypes? }` 对象形态） |
| id 前缀（`API.md §2.2`） | `att_` |
| 错误码（`API.md §10`） | `-32010 attachment_too_large`（已废，留空号） |

**前端迁移**：删除任何「createUploadUrl → PUT 二进制 → 拿 attachmentId → 放进 input/context」的上传流程；改为读取图片字节 → base64 → 直接作为 image `ContentBlock` 放进 `runs.start.input`。

---

## 4. 能力位与门控

- **server 级**：`features.multimodal` 现为 **`true`**（runtime 支持图片输入）。
- **模型级**：`Model.capabilities.multimodal`（`models.list`）= 该模型是否接受图片输入，源自 catalog modalities（精确判断「输入含 image」，非粗略「输入类型数 > 1」）。**前端应据此 gate 上传 UI**——模型不支持图片时禁用图片输入入口。
- **后端门控**：当 `runs.start` 显式指定了 `provider`+`model` 且该模型不接受图片时，带图请求直接返 `invalid_params`（不会白跑一轮再被 provider 拒）。用默认模型（不传 provider/model）时后端不前置拦截（由运维选定的默认模型负责），仍建议前端用模型级能力位 gate。
- **mime 校验**：image block 的 `mime` 非图片类型或不可解析 → `unsupported_mime`（`-32011`，复用此码）；`data` 为空 → `invalid_params`。

---

## 5. 注意事项

- **data 是纯 base64**，不要传 data-URL（`data:image/png;base64,…`）——后端按 `mime` 自行组装。
- **体积**：图片 base64 内联在请求体里，也会随开场 `userMessage` Item 持久化进会话记录（用于回放）。大图请前端先做合理压缩/降采样（业界做法：超过约数 MB 即转 JPEG / 缩边长），契约层当前不设硬上限。
- **HITL / resume**：带图的 turn 在工具审批中断后 resume 无特殊处理——图片已随用户消息进入会话记忆并在 resume 时回放。

---

> 配套正式契约：[`../API.md`](../API.md)（§4.3 ContentBlock / §7.3 runs.start / §9 capabilities / §10 错误码）、[`../TRANSPORT.md`](../TRANSPORT.md)（§12 旁路通道）。后端实现 SSOT：`lyra/internal/delivery/protocol`。
