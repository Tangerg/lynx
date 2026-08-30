# a2a

`a2a` is Scope's thin adapter for the Agent-to-Agent protocol. It is not a
second A2A SDK: the JSON-RPC envelope, SSE framing, AgentCard schema, transport,
and task lifecycle all come from the official
`github.com/a2aproject/a2a-go/v2`.

It works in both directions — a remote A2A agent becomes a local callable tool,
and a Scope capability becomes an A2A endpoint.

## Install

```bash
go get github.com/Tangerg/scope/a2a
```

## Calling remote agents

`OpenToolSet` opens clients and resolves AgentCards in one batch, then hands
back a `ToolSet` that owns both an immutable `[]tool.Tool` view and an
idempotent close:

```go
toolset, err := a2a.OpenToolSet(ctx, a2a.ToolSetConfig{
    Endpoints: endpoints,
})
if err != nil {
    return err
}
defer toolset.Close()

registry, err := tool.NewRegistry(toolset.Tools()...)
```

The underlying SDK client is not exposed, and there is no provider, cache, or
registry here. Discovery, refresh, and caching policy belong to the caller.

An A2A tool takes a single `message` field. A2A is a message protocol, not a
typed function call, so the tool surface does not pretend otherwise.

## Serving a capability

Implement the narrow `Agent` interface — text in, streamed text out — and mount
it:

```go
handler, err := a2a.NewHTTPHandler(a2a.HandlerConfig{Agent: myAgent, Card: card})
if err != nil {
    return err
}
http.Handle("/", handler)
```

The handler mounts the JSON-RPC method endpoint plus the well-known AgentCard.
The executor emits a legal event sequence — submitted, working, incremental
output, then completed, failed, or canceled — and skips empty increments,
because the SDK rejects an empty artifact.

## Transport

JSON-RPC over HTTP is the default, matching the rest of the stack. The SDK's
REST and gRPC bindings are not precluded, just not wired here.

## Content projection

A2A content is projected text-first into Scope's semantics, so a caller reads
the same shape it gets from any other Scope capability.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the boundaries this rests on.
