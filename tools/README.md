# tools

`tools` provides ready-to-assemble executable tools: shell, filesystem, HTTP,
web fetch, web search, and Agent Skills. Each one is a plain
`core/tool.Tool`, so a chat client, an Agent, or an MCP server consumes them
through the same contract.

The tool protocol itself, typed functions, and the registry live in
`core/tool`; schemas are derived by `core/jsonschema`. This module only
implements capability.

## Install

```bash
go get github.com/Tangerg/scope/tools
```

## Packages

| Package | Capability |
|---|---|
| `shell` | Run a command and capture its output |
| `fs` | Read, write, edit, glob, and grep inside a fixed path authority |
| `textread` | Read text with line numbering and limits |
| `httpreq` | Issue an HTTP request against an explicit allowlist |
| `web` | Neutral `Searcher` and `Fetcher` SPIs plus the search and fetch tools |
| `web/brave`, `web/exa`, `web/firecrawl`, `web/jina`, `web/perplexity`, `web/serper`, `web/tavily` | Provider implementations of those SPIs |
| `skills` | Expose an Agent Skills repository as a callable tool |

## Assembling a toolset

There is no global registry. A caller registers exactly what it wants:

```go
registry, err := tool.NewRegistry(
    fs.NewReadTool(fs.NewLocalExecutor(root)),
    fs.NewGlobTool(fs.NewLocalExecutor(root)),
    shell.NewTool(nil),
)
if err != nil {
    return err
}
```

`New(nil)` means "use the local backend" for capabilities that have one
(`shell`, `fs`). For capabilities that must be configured (`web`, `httpreq`) it
returns an error instead — there is no safe local fallback for a network call.

## Two tiers

Every capability is split in two:

- The **Tool tier** faces the model: JSON in, JSON out, schema validation.
- The **Backend Port tier** does the work and holds all the domain logic — line
  numbering, binary detection, write locks, path authority.

That split is what lets a remote or sandboxed backend answer a glob or grep in
one round trip instead of listing and reading through the Tool tier.

## Concurrency

A tool may declare, per call, whether it can overlap others and what it
conflicts on. Reads declare no conflict and run in parallel; an edit or write
keys on its target path, so the loop parallelizes different files and serializes
the same file.

## Web search and fetch

Every provider returns the same `web.SearchResponse` and `web.FetchResponse`, so
the model never adapts to a vendor API. A provider needs a key:

```go
searcher, err := tavily.NewSearcher(tavily.Config{APIKey: key})
if err != nil {
    return err
}
searchTool, err := web.NewSearchTool(searcher)
```

The neutral `Recency` filter (`hour`, `day`, `week`, `month`, `year`) is
translated into each provider's native freshness syntax.

## Limits

Output over a limit is truncated and marked, never turned into an error, so the
model can decide what to do next.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the boundaries these rules rest on.
