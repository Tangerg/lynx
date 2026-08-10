# Runtime Protocol consumer handoff

> Owner: the server-side contract cutover facts that frontend, TUI, CLI, and
> other Runtime Protocol consumers must adopt after the Runtime rewrite.
>
> This document records work deliberately not performed by the Runtime goal. It
> is not a compatibility promise and does not authorize dual fields or fallback
> decoding in the server.
>
> Last verified against the server contract and in-repository consumer source:
> 2026-08-11, at Runtime P19-02 completion. Consumer modules remain intentionally
> untouched; the Runtime now publishes the canonical Go values at
> `github.com/Tangerg/lynx/app/runtime/protocol`.

## Current server baseline

- Protocol version: `2026-08-10`; `minSupported` is the same value.
- Session artifact version: `15`; versions 14 and earlier are rejected before
  any import write.
- Machine truth: [`../contract/`](../contract/) generated from the Go contract
  registry with `go generate ./...`.
- Go truth: the public `runtime/protocol` package. The retired
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

## Desktop follow-up

The desktop currently vendors generated Runtime bindings and samples. Its
follow-up must replace those copies from `app/runtime/contract/typescript`, then
update handwritten fixtures and assertions that still state
`processRootSegment`:

- `app/desktop/frontend/src/rpc/` generated bindings, validators, samples, SDK,
  preflight, and schema tests;
- `app/desktop/frontend/src/plugins/builtin/runtime/` discovery and capability
  store tests;
- `app/desktop/frontend/visual/installVisualWorkspaceFixture.ts`;
- `app/desktop/frontend/CONTENT_RENDERING.md`.

No desktop source was changed by this Runtime goal.

## CLI and TUI follow-up

The current repository scan found no direct copy of the removed replay-scope
value in `app/cli`, and no standalone TUI protocol binding in this checkout.
Those consumers must still negotiate `2026-08-10` and regenerate or vendor the
current machine contract when their dedicated consumer-wiring work begins. Absence from this
list is not evidence that an out-of-tree consumer is compatible.

## Consumer acceptance

A consumer migration is complete only when it:

1. vendors or generates from the current Runtime-owned contract;
2. sends `protocolVersion: "2026-08-10"` and rejects any different discovered
   range instead of guessing compatibility;
3. accepts only `runtimeInstanceRootSegment` for `RunReplayScope`;
4. imports/exports Session artifact v15 without rewriting prior documents;
5. passes its strict fixture validation and HTTP integration suite.
