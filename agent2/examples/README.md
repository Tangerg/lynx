# Interaction examples

These commands are disposable consumers of the public `agent2` API. They do
not share test-only helpers or import internal protocol types, so build and test
failures expose real consumer-facing contract problems.

- `direct_vs_managed` contrasts a direct `chatclient` call with an Engine-owned
  Interaction Process. Direct calls remain the smallest embedding level;
  managed calls add lifecycle, signals, Effects, snapshots, limits, and events.
- `autonomous` runs a model-directed `model -> Tool -> model` loop. The model
  selects the Tool and stop point while the Definition supplies a hard local
  model-call limit.
- `composition` runs one local Definition directly through an embedded Engine,
  then composes the same Definition with an Interaction as two heterogeneous
  child Processes. Both paths use the same public execution narrow waist.

Both examples use deterministic local models so they run without credentials or
network access:

```sh
GOWORK=off go run ./examples/direct_vs_managed
GOWORK=off go run ./examples/autonomous
GOWORK=off go run ./examples/composition
```
