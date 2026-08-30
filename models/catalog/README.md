# catalog

`catalog` exposes the embedded model catalog: model identity, pricing,
capabilities, modalities, and token limits. It is provider reference data, kept
independent from the Core model invocation protocols.

## Install

```bash
go get github.com/Tangerg/scope/models/catalog
```

## Usage

```go
entry, found := catalog.Default.Lookup("openai", "gpt-4o")
if !found {
    return fmt.Errorf("unknown model")
}
```

The catalog answers questions about a model — what it costs, what modalities it
accepts, how many tokens it takes — without invoking anything. A caller that
needs to *call* a model uses the matching `models/<provider>` module.

## Testing

The catalog is embedded data, so its tests assert the shape and internal
consistency of that data rather than reaching a network.

## Boundaries

This module has no provider SDK dependency and implements no Core `Model`
contract. It never becomes a routing layer: choosing a model is the caller's
decision, and the catalog only supplies the facts.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
