# repoarch

`repoarch` owns the repository-wide architecture invariants as executable tests:
module boundaries, package layering, provider isolation, and documentation
coverage. It is development tooling — no product module depends on it.

## Running the gates

```bash
cd dev/repoarch && go test ./...
```

A failure names the rule and the file that broke it. The gates check, among
others:

- Namespace roots such as `core`, `models`, `otel`, and `tools` contain no Go
  package, so no root façade can appear.
- A provider module never imports a sibling provider, and its dependency island
  holds.
- Each capability module documents its public packages and keeps at least one
  checked Go example.
- Model modality boundaries and vector-store compiler shapes stay within their
  declared surface.

## Why tests rather than review

An architecture rule that lives only in a document erodes silently. Here a
violation fails CI with the specific file and rule, and adding a new package or
provider fails until it is registered — the inventory cannot go stale by
omission.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
