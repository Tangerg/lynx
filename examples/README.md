# examples

`examples` holds runnable programs that demonstrate Scope end to end. It is not
a library: nothing in the repository depends on it, and it is not published for
consumption.

## Running an example

Each subdirectory is a `main` package:

```bash
go run ./mcp/mcpagent
```

Examples that need a provider key read it from the environment and say so when
it is missing, rather than failing with a transport error.

## What belongs here

A program that shows how the pieces compose — an agent driving MCP tools, a
bridge between two protocols, a fake provider that makes a flow runnable
offline. A demonstration tool that a real caller might want belongs in `tools`
instead, as an assemblable capability.

## Boundaries

Example code may import any Scope module. No Scope module may import an example.
That direction is what keeps a demonstration from quietly becoming an API.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the rules this rests on.
