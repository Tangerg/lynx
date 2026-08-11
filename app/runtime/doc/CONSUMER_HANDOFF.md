# Runtime Protocol consumer handoff

> Owner: the server-side contract cutover facts that frontend, TUI, CLI, and
> other Runtime Protocol consumers must adopt after the Runtime rewrite.
>
> This document records exact consumer cutover state. It is not a compatibility
> promise and does not authorize dual fields or fallback decoding in the server.
>
> Last verified against the Runtime-owned server, public Go contracts, and
> Desktop generated consumer: 2026-08-12, during the post-P25 adversarial audit.
> Other consumers migrate independently and do not change the Runtime contract.

## Current server baseline

- Protocol version: `2026-08-12`; `minSupported` is the same value.
- Session artifact version: `17`; versions 16 and earlier are rejected before
  any import write.
- Machine truth: [`../contract/`](../contract/) generated from the Go contract
  registry with `go generate ./...`; `go-api.json` freezes the complete public
  `protocol + embedded` Go surface.
- Go truth: the public `runtime/protocol` and `runtime/embedded` packages. The retired
  `internal/delivery/protocol` path no longer exists and has no forwarding shim.
- Product execution vocabulary is exclusively Run, Segment, Item, and
  Interrupt. Agent Framework Process/Execution/Member identity is not a wire concept.

## Breaking surface

`RunReplayScope` now has the sole value `runtimeInstanceRootSegment`. It means
that replay is bounded to one Runtime instance and one root Segment; a Runtime
restart or a new Segment owns a new buffer. The former
`processRootSegment` value is removed, not aliased.

The protocol and artifact version bumps are deliberate rejection boundaries.
Consumers must not retry with an older version or rewrite an old artifact's
version number: old artifacts must be re-exported by the build that owns their
schema.

Transcript events now publish user messages, questions, and compaction as
complete facts without a synthetic `item.started` event. Agent-message and
reasoning streams retain provisional starts for rendering; ToolCall remains the
only durable running Item. A question's outstanding-answer lifecycle belongs to
its `PendingInterruptSet`, not to the historical Item.

ToolCall `startedAt` / `finishedAt` describe the visible Item lifecycle. Optional
`durationMillis` is exact Tool execution time, excludes approval and other
pre-execution waits, and remains absent when recovery cannot prove the interval.

## Desktop follow-up

The desktop vendors generated Runtime bindings and samples from
`app/runtime/contract/typescript`. P25 synchronized the projection/runtime
contract; every later protocol batch must continue replacing those generated
copies atomically with handwritten SDK semantics and fixtures:

- `app/desktop/frontend/src/rpc/` generated bindings, validators, samples, SDK,
  preflight, and schema tests;
- `app/desktop/frontend/src/plugins/builtin/runtime/` discovery and capability
  store tests;
- `app/desktop/frontend/visual/installVisualWorkspaceFixture.ts`;
- `app/desktop/frontend/CONTENT_RENDERING.md`.

## CLI and TUI follow-up

The CLI may directly own a concrete `*embedded.Runtime`; it must open it once,
keep it alive above individual commands, and complete `Close` before process
exit. Its adapter should import `runtime/embedded` and `runtime/protocol`, while
the command/terminal consumers define the narrow interfaces they need. It must
not copy Session/Run/Item/Event/Interrupt DTOs, expose Runtime internals, or
route an embedded call through JSON-RPC.

`CallOptions`, `CommandOptions`, `RunCommandOptions`,
`RunSubscriptionOptions`, and `SubscriptionOptions` are deliberately distinct.
The consumer maps its request identity, replay cursor, and negotiated metadata
into the matching option; it does not create a generic header bag. Stable
operation failures support `errors.Is` against protocol sentinels and
`errors.As` to `protocol.ProblemError`.

Only one HTTP or embedded owner may open a canonical data directory. If CLI,
desktop, or another process must operate the same durable Runtime concurrently,
they must share one host over IPC rather than bypassing the lease. TUI code may
consume the CLI-owned narrow port and protocol values; it does not open a second
Runtime.

No CLI or TUI source is changed by P19. Absence from this list is not evidence
that an in-tree or out-of-tree consumer is compatible.

## Consumer acceptance

A consumer migration is complete only when it:

1. vendors or generates from the current Runtime-owned contract;
2. sends `protocolVersion: "2026-08-12"` and rejects any different discovered
   range instead of guessing compatibility;
3. accepts only `runtimeInstanceRootSegment` for `RunReplayScope`;
4. imports/exports Session artifact v17 without rewriting prior documents;
5. passes its strict fixture validation and HTTP integration suite.

An embedded Go consumer additionally passes an external-module compile test,
uses the concrete Runtime lifecycle exactly once, and verifies stream replay,
idempotency, structured errors, cancellation, and shutdown without a listener.
