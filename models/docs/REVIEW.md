# `models/` — Review 阅读顺序

`models/` 是 LLM / Embedding / Image / Audio / Moderation 等模型 provider
适配器：每个目录一个 provider，实现 `core/model/*` 的对应 `Model` 接口。

provider 数量很多 (40+)。review 不必逐个看完，**按模板挑代表性 provider
深读**，其余 spot-check。

## 共享基础（**先读**）

1. `internal/options/` — Options 助手 + 默认值合并模板。
2. `internal/testutil/contract.go` **[精读]** — 跨 provider 共享的
   契约测试矩阵。看完这个就懂 lynx 对一个 Model 实现的最低期望。

## 主流 chat provider（深读 2~3 个）

3. `openai/`
   - `chat.go` **[精读]** — Chat Completions 适配。
   - `responses.go` **[精读]** — 新的 Responses API（含 reasoning +
     tool call，最近 commit `d879c0d` 加入）。
   - `embedding.go` / `image.go` / `audio_*.go` / `moderation.go` —
     多模态全家桶，结构同 chat。
   - `api.go` — HTTP client wrapper。
   - `constant.go` — 模型 id / endpoint。
4. `anthropic/`
   - `chat.go` — Claude 适配（注意 extended thinking 字段映射到
     `ReasoningPart`）。
   - `chat_openai.go` — 兼容 OpenAI 格式的回退（罕用）。
   - `tokenizer.go` — claude tokenizer。
5. `google/chat.go` — Gemini。
6. `bedrock/chat.go` — AWS Bedrock 多 provider 中转。

## OpenAI-compatible provider（同协议不同 endpoint）

按需挑读一个就够，主要看 endpoint / api key / default options 是否对：

- `openaicompat/` — 通用 OpenAI-compatible 模板。
- `deepseek/` / `moonshot/` / `mistral/` / `fireworks/` / `groq/` /
  `together/` / `xai/` / `zhipuai/` / `qwen/` / `alibaba/` / `minimax/`
  等。

## 本地推理

7. `ollama/chat.go` — Ollama 适配，本地常用。
8. `vllm/` / `lmstudio/` 等同套路。

## 多模态专家

- `assemblyai/` / `deepgram/` / `gladia/` / `elevenlabs/` / `lmnt/` —
  音频转写 / TTS。
- `midjourney/` / `blackforestlabs/` / `luma/` / `runway/` /
  `replicate/` / `recraft/` — 图像 / 视频生成。
- `nomic/` / `voyage/` / `cohere/` / `jina/` — embedding 专用。
- `huggingface/` — 通用入口。

## 每个 provider 的 review 重点

- **请求构造**：lynx `Request` → provider 原生 payload，字段是否齐？
- **流式响应**：chunk → `core.Response` 累加，Usage 是否在最后一帧填？
- **Tool call**：tool_calls 在响应里的位置 / id / arguments 字段是否
  正确映射到 `ToolCallPart`？
- **Reasoning**：thinking / reasoning 字段是否走 `ReasoningPart`？
- **错误归一**：HTTP 4xx / 5xx + provider 错误码 → lynx 统一错误。
- **OTel**：span 是否带 `lynx.chat.*` 属性？
- **集成测试**：`*_integration_test.go` 是 build tag 控的，CI 矩阵关心。

## 跨模块提醒

- 增加新 provider 模板：copy `openaicompat/` 加 endpoint + key。
- `DefaultOptions()` 返回值（非指针）— 是有意为之的不可变保证，新增
  provider 必须遵守。
- `chat.Usage.ReasoningTokens` 是 `*int64`，nil 表示 provider 没报，
  不要写成 0。

## 体检命令

- `go test ./models/internal/...` — 共享 helper 应稳定。
- `go test -tags=integration ./models/openai/...` — integration test
  通常需要真 key。
- `grep -l "RecordLLMInvocation" models/` — 看哪些 provider 已把 token
  数报到 process budget（目前 lynx 中间件层不主动调用，集成代码可以）。
