# Runtime Protocol consumer handoff

> Owner: the server-side contract cutover facts that frontend, TUI, CLI, and
> other Runtime Protocol consumers must adopt after the Runtime rewrite.
>
> This document records exact consumer cutover state. It is not a compatibility
> promise and does not authorize dual fields or fallback decoding in the server.
>
> Last verified against the Runtime-owned server, public Go contracts, and
> Desktop generated consumer: 2026-08-24, during the P153 Codebase removal cutover.
> Other consumers migrate independently and do not change the Runtime contract.

## Current server baseline

- Protocol version: exactly `2026-08-24`; there is no compatibility range.
- Session artifact version: `23`; every other version is rejected before
  any import write.
- Machine truth: [`../contract/`](../contract/) generated from the Go contract
  registry with `go generate ./...`; `go-api.json` freezes the complete public
  `protocol + embedded` Go surface.
- Go truth: the public `runtime/protocol` and `runtime/embedded` packages. The retired
  `internal/delivery/protocol` path no longer exists and has no forwarding shim.
- Product execution vocabulary is exclusively Run, Segment, Item, and
  Interrupt. Agent Framework Process/Execution/Member identity is not a wire concept.

## Breaking surface

The Runtime event vocabulary now contains only the seven variants production
publishes. The unproduced `custom` RunEvent, its `name`/`payload` fields, the
disabled `clientTools` feature, and both `toolResult` interrupt/response variants
are deleted rather than advertised as dormant extension points. Consumers must
use first-class Item or resource contracts for new facts.

Plan is no longer the sole member of a speculative shared-state registry. The
only spellings are `plan.updated`, `plan.changed`, `plan.get`,
`SessionSnapshot.plan`, and `SessionArtifact.plan`; discovery has no
`stateSnapshots`, RuntimeEvent has no state key, and Artifact v23 has no
`states[]` union. Consumers must not retain `state.snapshot`, `state.changed`,
`StateSnapshot`, the old `states[]` archive shape, or a generic shared-state Plan
reader. Desktop projects the Runtime Plan into the explicit `AgentSessionView.plan`;
its separate plugin companion-material map is not a protocol-state fallback.

`RunReplayScope` now has the sole value `runtimeInstanceRootSegment`. It means
that replay is bounded to one Runtime instance and one root Segment; a Runtime
restart or a new Segment owns a new buffer. The former
`processRootSegment` value is removed, not aliased.

The protocol and artifact version bumps are deliberate rejection boundaries.
Consumers must not retry with an older version or rewrite an old artifact's
version number: old artifacts must be re-exported by the build that owns their
schema.

P153 removes the standalone Codebase semantic-index contract in full. Consumers
must delete `GetCodebaseStatus`, `SearchCodebase`, `ReindexCodebase`, the
`codebase` feature, `codebase.changed`, every Codebase DTO/enum/sample, and all
status/search/reindex UI or commands. They must not retain a disabled capability,
old generated binding, `@codebase` alias, or compatibility adapter. The
embedding role remains valid only as optional Agent Memory ranking; keyword
memory search remains available without it.

Every public `Session` and `ArtifactSession` now carries required `provider` and
`model` fields. `sessions.update` changes them only as one complete pair;
provider-only, model-only, empty, or whitespace-padded identities are invalid.
Omitting a per-Run pair uses the durable Session pair, while an explicit pair is
committed as the Session's next default in the same opening write-set. Consumers
must compare both fields when resolving a catalog entry or its context window;
matching model id alone is incorrect when providers publish the same id. Fork,
scheduled admission, export, and import preserve the exact pair. There is no
model-only alias, provider inference, global-default fallback for an existing
Session, v22 artifact reader, or epoch-77 database migration.

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

The desktop consumes generated Runtime bindings, validators, and samples directly
from the private local `@scopeapp/runtime-contract` package rooted at
`app/runtime/contract/typescript`; it does not vendor a second generated tree.
Every later protocol batch regenerates this one package and updates handwritten
SDK semantics and fixtures against it:

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
Runs, open Interrupt sets, the optional Plan, and the optional Goal; it must not reconstruct that
material view with parallel `items.list` / `runs.list` / `interrupts.list` /
`plan.get` calls or retain that four-read path as a fallback. `includeDescendants`
has the same `features.subagents` gate as `runs.list`. The consumer commits the
Goal only when the same snapshot wins the mounted Agent view generation; an
independent `goals.get` writer remains valid only for an unmounted Goal read.

P138 adds the latest authoritative prompt footprint to durable `RunRef.contextTokens`
and the Session artifact. A mounted Desktop consumer still uses live
`segment.progress.contextTokens` while a Segment is running, then folds the durable
Run fact from `sessions.snapshot` after finish, renderer reload, Runtime restart, or
artifact import. The Session read model selects the newest positive root-run footprint;
it never substitutes cumulative `RunMetrics.usage.inputTokens`, and a newly admitted
successor with no model response does not erase the preceding proven footprint.

P143 makes Session selection an exact durable identity. Desktop's
`AgentSessionSummary` now projects both `provider` and `model`; Composer restore
and Context usage resolve the catalog by the pair, including duplicate model-id
fixtures. The generated package, handwritten adapter, smoke/wire fixtures, and
strict validators move together to Protocol `2026-08-24` and Artifact v23. No
second selection store or compatibility decoder was added.

- `app/runtime/contract/typescript/` generated bindings, validators, canonical
  samples, and the handwritten JSON Schema check vocabulary;
- `app/desktop/frontend/src/rpc/` SDK, preflight, and schema tests;
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

P153 synchronizes the in-tree CLI's direct Codebase consumer deletion: its
narrow port, embedded adapter, three TUI commands, change topic, feature gate,
tests, architecture inventory, and docs are gone. This is the only CLI scope
authorized by that batch; the CLI's separately accumulated Runtime-contract
drift remains a distinct migration and cannot justify a Runtime compatibility
shim. The affected backend/changefeed/runtimeprofile/terminal/arch/cmd packages
pass standalone tests; the broader `runtimeembedded` package still fails on
unrelated pre-existing Session/Question/Knowledge contract changes. Absence from
this list is not evidence that an out-of-tree consumer is compatible.

## Consumer acceptance

A consumer migration is complete only when it:

1. vendors or generates from the current Runtime-owned contract;
2. sends `protocolVersion: "2026-08-24"` and rejects any different discovered
   range instead of guessing compatibility;
3. accepts only `runtimeInstanceRootSegment` for `RunReplayScope`;
4. imports/exports Session artifact v23 with its required exact provider/model pair and explicit `plan` field, including durable root-run context footprints, authored AgentMessage phases, accepted Question answers, and exact human ToolCall approval decisions, without rewriting prior documents;
5. passes its strict fixture validation and HTTP integration suite.

An embedded Go consumer additionally passes an external-module compile test,
uses the concrete Runtime lifecycle exactly once, and verifies stream replay,
idempotency, structured errors, cancellation, and shutdown without a listener.
