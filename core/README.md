# core

`core` is the narrow waist of Scope: the serializable, provider-neutral
protocols that every provider implements and every capability module consumes.

It owns protocol values, the minimal calling SPI per modality, and the pure
composition around them. It owns no provider SDK, no network backend, no
tokenizer vocabulary, no agent control flow, and no telemetry.

## Install

```bash
go get github.com/Tangerg/scope/core
```

## Packages

| Package | Owns |
|---|---|
| `chat` | Chat protocol: `Message`, `Part`, `Request`, `Response`, `Options`, tool calls, usage, output format |
| `chatclient` | Direct chat conveniences: immutable defaults, middleware chain, templates, structured output, tool middleware |
| `chatclient/safeguard` | Fail-closed input/output screening as chat middleware |
| `embedding` | Text-to-vector protocol and its `Model` SPI |
| `embeddingclient` | Direct embedding conveniences and dimension resolution |
| `image` | Image-generation protocol |
| `moderation` | Content-moderation protocol |
| `rerank` | Query-to-document relevance-ranking protocol |
| `speech` | Text-to-speech protocol and its independent `Streamer` |
| `transcription` | Audio-to-text protocol |
| `document` | The canonical `Document` content value |
| `media` | The `Media` container shared by every modality |
| `metadata` | JSON-safe typed extension values |
| `jsonschema` | Schema derivation, parsing, and validation |
| `tool` | Executable tool contract, binding, authorization, registry, typed functions |
| `tokenizer` | Token counting and encoding capabilities (no vocabulary) |
| `history` | Conversation history contracts, window projection, middleware |
| `history/inmemory` | Zero-value-ready in-process history store |
| `vectorstore` | Semantic indexing and search over `Document` |
| `vectorstore/filter` | Metadata-filter expression vocabulary, parser, and visitor |
| `vectorstore/inmemory` | In-process vector store |
| `modeltest`, `history/storetest`, `vectorstore/storetest` | Reusable contract suites for implementors |

## Calling a model

Every modality exposes a minimal `Model` SPI with a single `Call`. Real
streaming is a separate `Streamer`, so a provider that cannot stream never has
to pretend it can.

```go
response, err := model.Call(ctx, &chat.Request{
    Messages: []chat.Message{
        chat.NewUserMessage(chat.NewTextPart("Hello")),
    },
    Options: &chat.Options{Model: "provider-model"},
})
```

`chatclient` adds the ordinary-path conveniences — frozen defaults, a
middleware chain, prompt templates, and structured output — without changing
the SPI. `Client` is an immutable value, so a configured client is safe to
share:

```go
client, err := chatclient.New(model, chatclient.Config{
    Defaults: chat.Options{Model: "provider-model"},
})
if err != nil {
    return err
}

response, err := client.Call(ctx, request)
```

`Client.Output` binds one typed output contract, and the same decoder serves
both the synchronous and the streaming path:

```go
type Answer struct {
    Summary string `json:"summary"`
}

answer, err := client.Output(chatclient.JSON[Answer]()).Call(ctx, request)
```

## Streaming

Streams are `iter.Seq2[*chat.Response, error]`. Early `break`, context
cancellation, and first-error termination are part of the contract:

```go
for response, err := range client.Stream(ctx, request) {
    if err != nil {
        return err
    }
    fmt.Print(response.Text())
}
```

## Errors

Every provider-facing modality package exports the same three sentinels, so
providers and integrations classify failures identically:

- `ErrInvalidOptions` — the caller's options are not usable.
- `ErrInvalidRequest` — the request violates the protocol.
- `ErrInvalidResponse` — the provider returned something the protocol rejects.

`chat` extends that triple with its own protocol errors because its wire
additionally models tool calls, parts, and usage. Classify with `errors.Is`;
never match on message text.

## Implementing a provider

Implement the modality `Model`, then run the shared contract suite so your
provider is held to the same protocol as every other:

```go
func TestChatContract(t *testing.T) {
    modeltest.ChatSuite{
        New:     func(t *testing.T) (chat.Model, chat.Streamer) { return newProvider(t) },
        Request: func(t *testing.T) *chat.Request { return helloRequest() },
    }.Run(t)
}
```

`vectorstore/storetest` and `history/storetest` do the same for backends.

## Boundaries

`core` never imports a sibling module, a provider SDK, a concrete tokenizer
vocabulary, or OpenTelemetry. Instrumentation lives in the `otel` module and
decorates Core from the outside. Retries, approvals, planning, and durable
execution are Agent or Host concerns.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants these boundaries
rest on.
