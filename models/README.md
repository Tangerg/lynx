# models

`models` is the namespace holding one independent module per AI provider. Each
one adapts a vendor SDK to the `Model` contracts defined by `core` — `chat`,
`embedding`, `image`, `moderation`, `speech`, `transcription` — so a consumer
depends on the protocol, never on a vendor's shape.

There is no aggregate `models` module. Take only the providers you use:

```bash
go get github.com/Tangerg/scope/models/openai
go get github.com/Tangerg/scope/models/anthropic
```

## Constructing a model

Every provider follows the same three parts: a validated `Config`, a `Model`
implementing the Core contract, and the endpoint and dialect wiring.

```go
model, err := openai.NewChat(openai.ChatConfig{APIKey: key})
if err != nil {
    return err
}

response, err := model.Call(ctx, request)
```

Defaults live in the construction config. A per-request override is an ordinary
`chat.Options` value resolved once per call; provider-specific parameters travel
as typed extensions rather than a manual type assertion.

## Providers

| Family | Modules |
|---|---|
| Native chat and multimodal | `anthropic`, `google`, `openai`, `mistral`, `cohere`, `minimax`, `zhipu`, `xiaomi` |
| OpenAI-compatible endpoints | `alibaba`, `azureopenai`, `deepseek`, `fireworks`, `groq`, `moonshot`, `openrouter`, `perplexity`, `together`, `xai` |
| Managed platforms | `bedrock`, `huggingface`, `replicate` |
| Local | `ollama` |
| Embeddings | `jina`, `nomic`, `voyage` |
| Image | `blackforestlabs`, `luma`, `prodia`, `stability` |
| Speech and transcription | `assemblyai`, `deepgram`, `elevenlabs`, `gladia`, `hume`, `lmnt`, `revai` |
| Shared wire protocols | `protocol/openai`, `protocol/anthropic` |
| Model catalog | `catalog` |

A provider that speaks an official wire verbatim reuses `protocol/openai` or
`protocol/anthropic` and promotes the shared `Model` by type alias. It never
wraps it in a shell that only forwards `Call` and `Stream`.

## Streaming

Real streaming is a separate `Streamer`, so a call-only provider never pretends
to stream. Streams are `iter.Seq2`; SSE deltas are accumulated into chunks by the
native provider or the shared protocol owner.

## Testing

These modules integrate third-party services and need credentials, so their
tests focus on what can be checked without a live account: option and request
mapping, wire round-trips, and error classification. The shared behavior suite
is `core/modeltest`; cross-provider construction and API consistency is checked
by `dev/providerconformance`. A provider module copies neither.

Integration probes skip unless the matching key is present:

```bash
SCOPE_TEST_OPENAI_KEY=... go test ./models/openai/...
```

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the contract every provider module
obeys.
