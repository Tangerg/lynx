# providerconformance

`providerconformance` checks the cross-provider consistency that no single
provider module can check for itself: that every provider constructs the same
way, exposes the same shape, and classifies failures with the same vocabulary.

It is development tooling — no product module depends on it.

## Running the checks

```bash
cd dev/providerconformance && go test ./...
```

## Why it lives outside the providers

A provider module runs the behavior suite from `core/modeltest` against its own
implementation. That proves one provider obeys the protocol; it cannot prove
that thirty-nine providers agree with each other on construction, naming, and
error classification. Checking that requires seeing them all at once, which is
exactly what a provider module must not do — providers never import siblings.

## Boundaries

This module reads provider surfaces to compare them. Nothing here ships, and no
provider may import it or copy its helpers.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for what this module owns.
