# Runtime Protocol consumer handoff

> Owner: the server-side contract cutover facts that frontend, TUI, CLI, and
> other Runtime Protocol consumers must adopt after the Runtime rewrite.
>
> This document records exact consumer cutover state. It is not a compatibility
> promise and does not authorize dual fields or fallback decoding in the server.
>
> Last verified against the Runtime-owned server, public Go contracts, and
> Desktop generated consumer: 2026-08-21, during the P138 durable context-footprint audit.
> Other consumers migrate independently and do not change the Runtime contract.

## Current server baseline

- Protocol version: `2026-08-17`; `minSupported` is the same value.
- Session artifact version: `21`; versions 20 and earlier are rejected before
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

The optional `runs.cancel.reason` is bounded to 1024 Unicode characters. The
Runtime normalizes the durable note at its Application boundary, while generated
Go, JSON Schema, OpenRPC, and TypeScript validators reject an oversized request
before it reaches execution.

`schedules.update` workspace edits have three exact branches: omit both fields
to preserve, send a non-empty `workspace` ref to set an explicit binding, or send
`workspaceMode: "default"` to remove it. `workspace` and `workspaceMode` are
mutually exclusive; an empty ref is invalid and never means clear.

## Desktop follow-up

The desktop vendors generated Runtime bindings and samples from
`app/runtime/contract/typescript`. P25 synchronized the projection/runtime
contract; every later protocol batch must continue replacing those generated
copies atomically with handwritten SDK semantics and fixtures:

P33 synchronized the generated Schedule request and validator, changed the
handwritten `schedules.update` wrapper to consume `UpdateScheduleRequest`
directly, and connected the default-workspace branch to the Schedule settings
product form.

P37 synchronized the generated Goal status and validator. During the valid
terminal settlement window, `goals.get` returns `status:"completing"`; a consumer
keeps the Goal visible, offers no stop/resume/start action, and waits for the
following `goals.changed` read to return `null`.

P56 added the Runtime-owned HTTP endpoint registry to `manifest.json` and the
generated TypeScript surface. Desktop consumes generated RPC/info/liveness/readiness
method, path, status, response type, and validator facts; it no longer maintains
handwritten sidecar paths or response schemas. A runtime-event consumer must also
intersect the topics it can fold with `runtime.discover.capabilities.runtimeTopics`:
asking an older Runtime for a newly added topic rejects the entire subscription.

P57 makes Desktop discovery a supervised connection lifecycle rather than a
bootstrap-only read. A consumer must publish service state and capabilities from
one coherent info/liveness/readiness/discovery inspection, withdraw capabilities
on failure, and retry with bounded backoff. Long-lived consumers report a stream
ending through the Runtime service port so that this supervisor verifies the
connection; they do not copy sidecar/discovery retry policy. Workspace target
identity changes revoke the old watch immediately, while a projection update for
the same active Session keeps the existing watch until a different target is
resolved, preventing duplicate subscriptions during cache catch-up.

P83-22 adds the stable query `sessions.snapshot`, and P112 extends its existing
material boundary to the Session Goal. A mounted Session consumer must
use this single transactionally coherent response for complete Items, durable
Runs, open Interrupt sets, optional Plan state, and the optional Goal; it must not reconstruct that
material view with parallel `items.list` / `runs.list` / `interrupts.list` /
`plan.get` calls or retain that four-read path as a fallback. `includeDescendants`
has the same `features.subagents` gate as `runs.list`. The consumer commits the
Goal only when the same snapshot wins the mounted Agent view generation; an
independent `goals.get` writer remains valid only for an unmounted Goal read.

P138 adds the latest authoritative prompt footprint to durable `RunRef.contextTokens`
and Session artifact v21. A mounted Desktop consumer still uses live
`segment.progress.contextTokens` while a Segment is running, then folds the durable
Run fact from `sessions.snapshot` after finish, renderer reload, Runtime restart, or
artifact import. The Session read model selects the newest positive root-run footprint;
it never substitutes cumulative `RunMetrics.usage.inputTokens`, and a newly admitted
successor with no model response does not erase the preceding proven footprint.

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

An HTTP or embedded Runtime may share one canonical private data directory with
another Runtime process. A client still binds to exactly one Runtime instance;
CLI and desktop may each embed their own instance. Concurrent writes to the same
Session fail with `session_busy`, active Runs may share one physical working tree,
and rollback/restore wait for exclusive working-tree ownership. Commits made by
the peer Runtime produce a scoped Runtime resync so mounted read models reload
durable state. All sharers must run a build that accepts the same persistence and
checkpoint contracts; there is no compatibility reader or global-directory
single-instance fallback. TUI code may consume the CLI-owned narrow port and
protocol values rather than opening an unnecessary third Runtime.

No CLI or TUI source is changed by P19. Absence from this list is not evidence
that an in-tree or out-of-tree consumer is compatible.

## Consumer acceptance

A consumer migration is complete only when it:

1. vendors or generates from the current Runtime-owned contract;
2. sends `protocolVersion: "2026-08-17"` and rejects any different discovered
   range instead of guessing compatibility;
3. accepts only `runtimeInstanceRootSegment` for `RunReplayScope`;
4. imports/exports Session artifact v21, including durable root-run context footprints, authored AgentMessage phases, accepted Question answers, and exact human ToolCall approval decisions, without rewriting prior documents;
5. passes its strict fixture validation and HTTP integration suite.

An embedded Go consumer additionally passes an external-module compile test,
uses the concrete Runtime lifecycle exactly once, and verifies stream replay,
idempotency, structured errors, cancellation, and shutdown without a listener.
