# mcp

`mcp` is Scope's thin adapter for the
[Model Context Protocol](https://modelcontextprotocol.io/). It is not a second
MCP SDK: protocol clients, servers, sessions, and transports all come from the
official `github.com/modelcontextprotocol/go-sdk/mcp`.

This module owns only what Scope needs and the SDK should not know about:
mapping remote MCP tools into `core/tool.Tool`, exposing a Scope tool to an MCP
server, converting MCP prompts into `core/chat` messages, and the context
plumbing for `_meta` and reverse capabilities.

## Install

```bash
go get github.com/Tangerg/scope/mcp
```

## Naming

The package shares its name with the official SDK, so both are normally
imported under an alias:

```go
import (
    scopemcp "github.com/Tangerg/scope/mcp"
    sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)
```

## Using remote tools

`DiscoverTools` lists the tools a connected session offers and wraps each one as
a `core/tool.Tool`, so an Agent or a chat client consumes them exactly like a
local tool:

```go
tools, err := scopemcp.DiscoverTools(ctx, scopemcp.ToolDiscoveryConfig{
    Session: session,
})
if err != nil {
    return err
}
```

The wrapper is deliberately not exported. Two capabilities are reachable through
interface assertion instead:

- `MCPToolIdentity()` returns the unsanitized `(source, remote)` name pair for
  policy decisions. `Definition.Name` is a provider-constrained presentation
  label, not an injective identity.
- `ConcurrencyKey()` lets a conflict-aware scheduler run compatible remote calls
  in parallel. Unknown tools stay exclusive unless a `ConcurrencyPolicy` says
  otherwise.

## Serving a Scope tool

The reverse direction registers a `core/tool.Tool` on an MCP server. A tool
error becomes a result marked `IsError`, never a JSON-RPC protocol error —
those two failures mean different things to a client.

## Prompts

`PromptMessages` converts an MCP prompt result into `[]chat.Message`. Text,
image, audio, resource links, and embedded resources all map to the matching
`chat.Part`; a shape the protocol adds later is preserved as encoded text rather
than dropped.

## What this module does not own

Server inventories, OAuth login, reconnect policy, tool-list caching, and status
UI belong to a Host runtime. This module has no registry, no cache, and no
background refresh: when a session reports `list-changed`, the caller decides
when to list again.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the boundaries this rests on and
[`DESIGN.md`](DESIGN.md) for the package-organization rationale.
