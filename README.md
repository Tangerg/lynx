# Scope

Scope is a modular Go foundation for building AI systems. It provides
provider-neutral protocols, composable model clients, Agent execution,
retrieval, evaluation, tools, and observability without owning an application
runtime or product platform.

Scope is designed for libraries and Hosts that need explicit contracts:

- Core protocols for chat, embeddings, images, speech, transcription,
  moderation, tokenization, tools, history, and vector stores.
- Direct chat and embedding clients with immutable defaults and middleware.
- Agent interaction, planning, workflows, lifecycle management, and durable
  tree execution contracts.
- ETL, document processing, RAG, evaluation experiments, MCP, A2A, and Skills.
- OpenTelemetry adapters and independent provider modules for models, vector
  stores, and history stores.

## Boundaries

Scope owns framework semantics and portable execution contracts. It does not
own product sessions, dashboards, user identity, deployment marketplaces,
application persistence, or desktop workflows. A Host composes those concerns
around Scope; an application layer such as Flame can provide the product
experience.

Each capability has one owning package and one public entry point. Scope avoids
root façades and re-exports: import the semantic package directly, and combine
capabilities through its documented interfaces or middleware.

## Direct model calls

Create a provider model, then wrap it with `core/chatclient`:

```go
client, err := chatclient.New(model, chatclient.Config{
    Defaults: chat.Options{Model: "provider-model"},
})
if err != nil {
    return err
}

response, err := client.Call(ctx, &chat.Request{
    Messages: []chat.Message{
        chat.NewUserMessage(chat.NewTextPart("Hello")),
    },
})
```

For the small direct-use case, `chatclient.NewToolMiddleware` freezes an
executable Tool manifest, publishes its schemas, executes complete calls
serially, and continues the model conversation. Retries, approvals,
concurrency, planning, and durable execution remain Agent or Host concerns.

## Repository layout

The repository is a Go workspace of independently versioned modules:

- `core`: stable protocols and direct clients.
- `agent`: managed execution kernel and strategies.
- `eval`: datasets, suites, experiments, comparisons, and reports;
  `eval/trajectory` adds Agent regressions as a leaf integration module.
- `etl`, `rag`, `tools`, `skills`, `mcp`, `a2a`: focused framework modules.
- `models`, `vectorstores`, `historystores`: optional provider modules.
- `otel`: observability adapters that remain outside Core.

Run the full repository gates with:

```bash
scripts/check.sh build vet test race tidy
scripts/check.sh isolate
scripts/check-core-coverage.sh
scripts/check-agent-coverage.sh
```

## Releasing every module

From a clean `main` branch, one command validates the repository, derives the
module dependency graph from `go.work` and `go.mod`, commits dependency pins,
creates and verifies annotated tags, pushes `main`, and publishes every module
tag in dependency order:

```bash
scripts/release.sh v0.11.0
```

The argument is one stable semantic version shared by all modules. The command
is safe to resume after a network failure: existing tags must match exactly and
are never deleted, moved, or force-pushed.

## License

Scope is licensed under the [Apache License 2.0](LICENSE).
