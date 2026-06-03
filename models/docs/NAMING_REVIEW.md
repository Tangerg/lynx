# `models/` 命名 review

40+ provider 适配器，扫完后**主要发现一个跨包问题**：`Api*` 类型名
违反 Go 初始词大小写约定，散布在 ~30+ 个 provider 子包里。

---

## 1. `Api` / `ApiConfig` 命名违反初始词约定 ✅ DONE

**实际定义**（10+ provider 子包里都有）：

```go
// models/openai/api.go:17,32
type ApiConfig struct { ... }
type Api struct { ... }

// models/lmnt/api.go:32
type Api struct { ... }
// models/cohere/api.go:38, models/prodia/api.go:31,
// models/luma/api.go:31, models/blackforestlabs/api.go:31,
// models/gladia/api.go:32, models/google/api.go:50,
// models/deepgram/api.go:33, models/nomic/api.go:31,
// models/midjourney/api.go:52 等等
```

**问题**：
- `API` 是初始词 (Application Programming Interface)，按 Go effective-
  go 必须全大写或全小写，不能 `Api`。
- 整个 lynx 仓库 `Api` 出现 267 处，跨 68 个 .go 文件。
- 同时仓库其他地方用 `APIKey` / `APIToken` / `APIError` 等正确大写。
  models/ 自成一派违反惯例。

**建议**：
- `Api` → `API`
- `ApiConfig` → `APIConfig`
- `NewApi(...)` → `NewAPI(...)`

**调用面**：267 处，但全部在 models/ 子包内（不跨模块）。机械替换：

```bash
find models -name "*.go" -exec perl -i -pe \
  's/\bApi(Config|Client)?\b/API$1/g' {} \;
go test ./models/...
```

⚠️ 跑前先 grep 验证替换面，避免误伤 `EndpointAPIxxx` 这种已经
正确的命名。

---

## 2. 各 provider 的 `<Modality>Model` / `<Modality>ModelConfig` ✅

`models/openai/`:

```go
type ChatModelConfig struct { ... }
type ChatModel       struct { ... }
type AudioTranscriptionModelConfig struct { ... }
type AudioTranscriptionModel       struct { ... }
type AudioTranslationModelConfig   struct { ... }
type AudioTranslationModel         struct { ... }
type AudioTTSModelConfig struct { ... }   // TTS 大写正确 ✓
type AudioTTSModel       struct { ... }
type EmbeddingModelConfig struct { ... }
type EmbeddingModel       struct { ... }
type ImageModelConfig struct { ... }
type ImageModel       struct { ... }
type ResponsesModelConfig struct { ... }  // 新 Responses API
type ResponsesModel       struct { ... }
type ModerationModelConfig struct { ... }
type ModerationModel       struct { ... }
```

**评价**：
- `<Modality>Model<Config>` 横向命名一致 ✓
- TTS / OpenAI 等初始词正确大写 ✓
- 命名清楚分模态 ✓

外部读：`openai.NewChatModel(...)` / `openai.NewImageModel(...)` —
清楚 ✓

---

## 3. 各 provider 的 `Metadata` ✅

`models/openai/metadata.go` / `models/anthropic/metadata.go` /
`models/google/metadata.go` 等：

```go
type Metadata struct { ... }
```

**评价**：单字 + public 字段，符合数据载体规则 ✓

---

## 4. `models/internal/options/` 共享 helper ✅

跨 provider 共享的 options merge 工具。命名简洁。

---

## 5. `models/internal/testutil/contract.go` 契约测试矩阵 ✅

`Contract` / `TestContract` 命名，跨 provider 共享 ✓

---

## 6. 各 provider 命名前缀**没**stutter ✅

- `openai.ChatModel` / `anthropic.ChatModel` / `google.ChatModel` ✓
- `openai.NewChatModel(...)` ✓

包名 = provider 名，类型 `ChatModel` / `EmbeddingModel` 不口吃 ✓

---

## 7. `openaicompat/` 共享父类 — 命名 OK ✅

`models/openaicompat/` 提供给 deepseek / moonshot / fireworks / groq /
mistral 等 OpenAI-compatible provider 用的共享代码。

---

## 不动 / 已经 OK 的

- `<Modality>Model<Config>` 模板跨 40+ provider 统一
- 零 Get/Set / ToString / stutter / Java suffix
- 零自定义中间层 `*Manager` / `*Helper`
- TTS / TTM / VTT 等其他缩写大写正确
- 测试 stub 也跟着同套命名

---

## 优先级建议

**P0 — 高 ROI 大批量改**
1. **`Api` → `API` 全局重命名** (267 处, 68 文件)
   - models/ 内部统一改
   - lyra / agent / tests 不会被波及（核查后再决定）

**其他**：**无 P1~P3**。本包**单一最大命名债务就是 Api 初始词**，
搞定后整个 models/ 子模块就清白了。

---

## 体检命令

```bash
# 改前面板
grep -rnE "\bApi\b" --include="*.go" models/ | grep -v _test | wc -l
# 应得 ~150

# 改后验证
grep -rnE "\bApi\b" --include="*.go" models/ | grep -v _test | wc -l
# 应得 0（或只剩外部 SDK 的字段引用）
```

跑 `go test ./models/...`（部分需要 integration tag + 真 key，
能跑通的子集应全绿）
