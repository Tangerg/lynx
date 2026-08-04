# Runtime Architecture Hygiene Plan

> Status: Completed
> Started: 2026-07-22  
> Scope: `app/runtime`  
> Architecture baseline: [EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md)
>
> Historical implementation ledger. Names such as Todo or `task` below describe
> the code that existed during those completed batches and are not current
> contracts. Use [TOOL_SYSTEM_VNEXT.md](TOOL_SYSTEM_VNEXT.md) and the architecture
> baseline above for the normative Plan, Goal, and `delegate_task` vocabulary.

## 1. Objective

Bring `app/runtime` to a consistently clean execution-centered architecture:

- Domain values express business meaning without JSON-RPC, agent-framework, or storage shapes.
- Application use cases own their collaborators, lifecycle, ordering, and business policy.
- Delivery maps protocol requests and projections only; it does not assemble application workflows.
- Adapter and Infra expose only the capabilities their consumers require.
- Abstractions exist for demonstrated boundaries and variation, not for symmetry or tests alone.
- A completed Run stream means the full synchronous segment boundary has completed.
- Durable schedule state reports what actually happened and has deterministic restart semantics.
- Architecture fitness tests protect semantic ownership in addition to import direction.

## 2. Non-goals

This work will not:

- split cohesive packages or files solely because they are large;
- introduce a DI container, EventBus, Mediator, CQRS/Saga framework, generic Repository, or aggregate marker;
- add compatibility shims, dual-read/write paths, retry layers, or transient-error classification;
- change protocol wire shapes unless a batch explicitly proves that a protocol change is necessary;
- generalize a type or interface without at least two real consumers or implementations requiring it.

## 3. Baseline

The audit started from a green workspace:

- `go test ./...`
- `go vet ./...`
- `go test ./internal/arch`
- race tests for `application/{runs,sessions,goals,schedules}`, `adapter/agentexec/turn`, and `bootstrap`

The import-level dependency rule already holds. The remaining issues are semantic ownership,
lifecycle contracts, truthfulness of durable state, and abstraction quality.

## 4. Architectural decisions

The following decisions are fixed for this plan unless implementation evidence invalidates one;
any change must be recorded in the progress log before code is changed.

1. **Run completion is one boundary.** Closing the application event journal means terminal
   persistence, synchronous checkpoint maintenance, and admission release are complete.
2. **No consumer polling for local lifecycle gaps.** Goal mode waits on the Run contract; it does
   not retry `ErrSessionBusy` to compensate for an early completion signal.
3. **Schedule occurrence state is durable truth.** A failed start is never recorded as fired.
   Restart behavior must not depend on an in-memory retry counter.
4. **Application collaborators are constructor-owned.** Delivery does not pass one application
   coordinator into another application use case.
5. **Background use cases are application/bootstrap-owned.** Delivery may publish their protocol
   projections, but does not create their execution strategy or own their lifecycle.
6. **Business policy stays in Application/Domain.** Provider/model capability and configuration
   rules are not preconditions callers must remember to enforce.
7. **Interfaces are consumer-side and narrow.** Constructors return concrete types; each consumer
   names only the behavior it invokes.
8. **Wire serialization stays in adapters.** Domain/Application values use typed vocabulary and no
   JSON tags needed only by the agent suspension or runtime protocol.

## 5. Work batches

### Batch 1 — Run boundary semantics

Status: **Completed**

Scope:

- Close a Run journal only after synchronous terminal maintenance and admission release.
- Remove Goal's `ErrSessionBusy` backoff/retry loop.
- Treat a terminal Run without an outcome as an invariant violation, not `Completed`.
- Add deterministic tests covering back-to-back Goal runs and completion ordering.

Acceptance:

- Stream drain cannot complete while the previous segment still owns its admission fence.
- Goal contains no busy retry constants or timer loop.
- Malformed terminal state stops safely and is observable as an error path.

### Batch 2 — Application collaboration ownership

Status: **Completed**

Scope:

- Extract the shared session/working-tree admission responsibility from `runs.Registry` into one
  application-owned component used by Runs and Sessions.
- Inject admission into session coordination once; remove per-call `SessionClaimer` parameters.
- Separate schedule management from schedule execution ownership.
- Construct the scheduled-run launcher and worker outside Delivery; Delivery receives notifications
  only as protocol projections.
- Replace in-memory schedule retry state with one explicit durable occurrence policy.

Acceptance:

- Delivery passes no application coordinator as a collaborator to another application use case.
- Delivery exposes no background scheduler lifecycle method.
- Schedule state never says “fired” for a Run that was not admitted.
- Restart semantics are covered by persistence-level tests.

### Batch 3 — Model/provider policy ownership

Status: **Completed**

Scope:

- Move model enumeration policy, remote-probe fallback, catalog enrichment, provider support,
  configuration, base-URL, chat-role, and embedding-role validation into `application/models`.
- Introduce only the narrow catalog/provider metadata ports that application actually consumes.
- Return typed application/domain errors and leave protocol error mapping in Delivery.

Acceptance:

- `delivery/server` no longer imports the static model catalog.
- Every application entry point enforces its own business preconditions.
- A second non-protocol caller gets the same behavior without reproducing Delivery logic.

### Batch 4 — Abstraction and boundary cleanup

Status: **Completed**

Scope:

- Return a concrete turn dispatcher and define narrow interfaces at its consumers.
- Make the Run registry concrete over its actual payload.
- Keep the Journal generic only if another production event family is found; otherwise concretize it.
- Replace dependency-bag accessor interfaces with named dependencies or cohesive phase ports.
- Replace raw HITL remember-scope strings with the canonical typed domain vocabulary.
- Move agent-suspension JSON serialization to the agent adapter.

Acceptance:

- No producer-owned fat Dispatcher interface remains.
- Production generics are backed by multiple production types, or removed.
- Domain execution values contain no agent/wire-only JSON contract.
- No duplicate approval-scope vocabulary remains.

### Batch 5 — Fitness tests and final verification

Status: **Completed**

Scope:

- Add focused architecture tests for collaboration ownership, background lifecycle ownership,
  model policy placement, and wire-free domain values.
- Update architecture documentation and stale GoDoc to match the final ownership model.
- Run workspace and standalone-module build, vet, test, and relevant race suites.

Acceptance:

- `go build ./...`, `go vet ./...`, and `go test ./...` pass in `app/runtime`.
- `GOWORK=off go build ./...`, `GOWORK=off go vet ./...`, and `GOWORK=off go test ./...` pass.
- Relevant concurrency packages pass `go test -race`.
- Architecture tests fail when any removed leak is reintroduced.

### Batch 6 — Semantic boundary closure

Status: **Completed**

Scope:

- Move durable usage aggregation out of Delivery into `application/usage`.
- Move workspace path confinement, file/Git reads, project aggregation, and
  instruction-document discovery behind focused `application/workspace` ports.
- Make Bootstrap the sole owner of Delivery's application collaborator wiring
  and environment capability probe.
- Establish the initial Bootstrap-owned boundary for delegated-session
  continuation data, then remove the remaining domain carrier in Batch 7.

Acceptance:

- Production Delivery imports no workspace or prompt-source adapter and does
  not construct application coordinators.
- Usage and workspace policies are expressed with application values, never
  protocol DTOs.
- The transitional Batch 6 continuation boundary is isolated enough to be
  replaced without changing the Delivery or Run contracts.
- Architecture tests fail if the removed Delivery bypasses return.

### Batch 7 — Residual abstraction-leak eradication

Status: **Completed**

Scope:

- Move Git-state filesystem watching from Delivery to the workspace adapter and
  expose it through a workspace application use case.
- Remove agent continuation identity and opaque state from the Session domain;
  persist it as Bootstrap-owned opaque sidecar data at the storage boundary.
- Move agent-memory review/mutation and project-root policy into a dedicated
  application use case with an atomic storage update.
- Make codebase operations resolve their workspace root in Application, split
  the workspace aggregate into focused use cases, and give Delivery only its
  consumer-side ports.
- Replace adapter type/error re-exports and duplicate application values with
  boundary-owned vocabulary; add tests that prohibit Delivery filesystem tech
  and Session continuation state.

Acceptance:

- Delivery owns no filesystem watcher, path traversal, or filesystem-library
  dependency for workspace operations.
- Product Session values carry only product lineage/audit data; agent runtime
  continuation is opaque outside Bootstrap and storage.
- Every Delivery handler depends only on the use-case behavior it consumes;
  no catch-all workspace coordinator remains.
- The architecture suite fails if these ownership boundaries regress.

### Batch 8 — Delivery abstraction-leak closure

Status: **Completed**

Scope:

- Replace the archive's reuse of live `Session` / `RunRef` / `Item` protocol
  shapes with an explicit v6 durable document and an Application-owned portable
  snapshot validation/recovery model.
- Keep archive decoding in Delivery mechanical; move graph binding, offload
  checks, terminal-state derivation, and restore normalization into
  `application/sessions`.
- Move same-origin MCP credential-retention policy, startup mutation recovery,
  session activity precedence, hook activation, and skill-change publication to
  their owning Application or composition-root boundary.
- Keep `runs.StartCommand` to one canonical content representation and move
  user message/media materialization into Application.
- Remove runtime JSON tags and process-hook JSON protocol from Domain; let
  storage and subprocess adapters own their respective codecs.
- Remove Delivery port methods that no production handler consumes, and add
  fitness checks for every removed ownership seam.

Acceptance:

- Delivery holds no archive aggregate validation/recovery logic and no manual
  post-commit skill event publication.
- The portable artifact cannot carry live status, revision, workspace-derived,
  active, or interrupted executor state.
- Domain structs have no JSON field tags; adapters retain external JSON codecs.
- Delivery interfaces declare only methods driven by production handlers; tests
  reach concrete coordinators through test-only fixtures rather than widening a
  production consumer port.
- The runtime module, its standalone build, and the desktop protocol samples
  validate against the v6 contract.

### Batch 9 — Read-model, secret, and execution-boundary closure

Status: **Completed**

Scope:

- Make Application own the complete user-facing Session read model and the
  terminal portable-session identity; Delivery only paginates and projects it.
- Replace raw MCP registry/status values at the Delivery boundary with
  Application commands and safe read models; retain credentials only inside
  Application and never surface adapter errors verbatim.
- Normalize concrete tool results and activity text at the executor adapter so
  Delivery has no concrete-tool schema or activity registry.
- Collapse semantic single-sink notifier packages into the neutral generic
  `component/signal` primitive, with Application-owned payloads.
- Move provider/model catalog probing out of Bootstrap to its driven adapter;
  remove remaining transparent adapter aliases encountered on these paths and
  preserve ordering/cancellation with focused tests.

Acceptance:

- Delivery performs no session filesystem/state/default-model join, no MCP
  registry or live-pool readback, and no concrete-tool result transformation.
- Portable archive input cannot carry live Session lineage, kind, isolation, or
  revision fields.
- Raw MCP authorization and adapter error text have no route to a protocol
  response or event.
- Every migrated bridge is represented by `component/signal` plus an
  owning-layer payload, rather than a semantic notifier package.

### Batch 10 — Residual ownership closure and non-destructive abstraction audit

Status: **Completed**

Scope:

- Move behaviorful persistence, checkpoint, prompt-catalog, child-session, and
  turn-cleanup implementations out of Bootstrap; Bootstrap only assembles them.
- Make archive export/import, rollback presentation, model-role updates, and
  live-run subscription return one coherent application result, so Delivery
  cannot join separate application operations after a mutation/admission.
- Move JSON-RPC replay retention, secret masking, model-facing tool-result
  preview text, and question-response field encoding out of Domain.
- Remove transparent application facade aliases where direct domain ownership is
  clearer; make restore a named durable command rather than an alias of an
  export read model.
- Centralize application-owned run/segment/item resource namespaces and add a
  fitness test that forbids behaviorful Bootstrap receiver methods.
- Treat an unconsumed capability as a deletion candidate **only after** checking
  protocol routes, product contracts, and independent semantic use. A frontend
  that has not yet integrated a supported operation is not proof of dead code.

Acceptance:

- Bootstrap contains assembly, configuration, and host shutdown only; no hidden
  adapter implementation remains.
- A Delivery handler invokes one complete application use case for each response
  it presents; it never re-reads a changed aggregate to finish that response.
- Domain has no JSON-RPC replay, log/wire masking, executor-tool wording, or
  client response-key encoding vocabulary.
- No capability is removed merely because the current frontend has no caller.
- The expanded architecture suite, full module tests, and focused race suites
  pass before this batch is marked complete.

### Batch 11 — Final abstraction-leak remediation

Status: **Completed**

Scope:

- Replace the acknowledged-but-discarded `feedback.create` call with one
  validated Application use case and durable SQLite append-only ledger.
- Remove unconsumed query, ID-constant, Turn-event, transport, and test-only
  production facades rather than keeping compatibility aliases.
- Remove the unconnected sandbox snapshot/resume stack and its schema; retain
  the live isolated working-copy capability and its safe archive-copy helper.
- Distinguish the deleted sandbox snapshots from active agent continuation
  process snapshots, which remain part of the run-resume boundary.

Acceptance:

- A successful feedback acknowledgement means a durable receiver accepted the
  quality signal; invalid empty or unknown-rating signals map to `invalid_params`.
- Adapter packages do not re-export the Application Run event vocabulary, and
  no generic transport interface remains without a consumer.
- No SQLite table, store, or workspace API exists solely for unconnected
  sandbox snapshot/resume behavior.
- Full tests, vet, and directional static scans are clean.

### Batch 12 — Runtime ownership closure

Status: **Completed**

Scope:

- Delete Delivery-only dead dependency chains (workspace-root lookup, singular
  run-activity probe, unused pagination/title helpers) and narrow the remaining
  consumer ports to live handlers.
- Keep Goal mode and its session-mutation coordination intact: it is a product
  capability, not dead code.
- Remove executor-event correlation fields and lifecycle interfaces that are
  written only by the adapter and never consumed by the production run pipeline;
  retain the sealed application event family and the application-owned durable
  event cursor.
- Split schedule management from firing/worker execution so a complete runner
  is constructor-injected after Runs exists, with no mutable `BindRunner` seam.
- Move live model-role and MCP-policy synchronization behind owning application
  state types; move runtime policy resolution and compactor live-state projection
  to driven adapters so Bootstrap only loads startup values and assembles parts.

Acceptance:

- No Delivery consumer port carries an uncalled production method or a dead
  configuration chain.
- No product capability is removed based solely on current frontend adoption.
- Bootstrap imports no atomic synchronization primitive and holds no live
  policy/projection closure.
- Schedule execution cannot be observed before its Runner is fully constructed.
- Architecture tests guard the removed seams; full module and standalone
  verification are green.

### Batch 13 — Consumer-port and state-ownership closure

Status: **Completed**

Scope:

- Remove producer-owned broad interfaces from the Goal, Todo, Knowledge, and
  Schedule domains; define each persistence slice at its actual Application or
  Adapter consumer.
- Replace the Goal tool's direct persistence/CAS mutation with an
  application-owned `goals.State` report boundary shared through Bootstrap.
- Split schedule management, run-now, and worker persistence requirements;
  keep their composition-root union only at Bootstrap wiring.
- Make approval consumers name their independent management, tool-gate, and
  plan-exit views; retain the concrete `approval.RuntimePolicy` as the cohesive
  domain implementation.
- Remove Bootstrap's duplicate hook-trust relay and make prompt knowledge/todos
  use the same root-configured source as their corresponding use cases.

Acceptance:

- No Delivery/Tool adapter reads or writes Goal persistence directly.
- A consumer cannot accidentally obtain Todo write/cleanup, Knowledge write,
  Schedule firing, or Approval management methods it does not invoke.
- Goal, Todo, Knowledge, Schedule, and Approval do not reintroduce the removed
  producer-owned domain interface names.
- Existing product capability is preserved; no capability is removed merely
  because an individual client has not adopted it.

### Batch 14 — Residual consumer-port closure

Status: **Completed**

Scope:

- Move Feedback persistence, MCP registry mutation, and diagnostic-tool
  catalog/invocation contracts out of their Domain producers and into their
  actual Application or Bootstrap consumers.
- Delete Agent Memory's unused aggregate `Store`, leaving extraction, search,
  and human-review workflows with their independently owned narrow ports.
- Replace the producer-owned Codebase `Index` with Application and Tool Adapter
  consumer views; remove the unconsumed `EnsureIndexed` API and test through
  the real `Search` path instead.
- Extend the architecture fitness test to cover every removed producer-owned
  port from this cleanup.

Acceptance:

- Domain packages retain values, invariants, and only ports consumed directly
  by a Domain service; Feedback, MCP, Tool, Agent Memory, and Codebase consumer
  ports cannot return there.
- No production capability is deleted because its frontend consumer is absent;
  Goal, the explicitly retained in-process CLI/TUI transport, and test-only
  approval fixtures remain correctly classified.
- Workspace and standalone build/vet/test, focused race tests, static analysis,
  architecture tests, formatting, and dead-code scans are green or have only
  explicitly retained future/test findings.

### Batch 15 — Protocol, static-config, and content-codec closure

Status: **Completed**

Scope:

- Make every `memory.*` method honor the same negotiated-capability contract;
  Delivery must not fabricate an empty UI state for a disabled feature.
- Remove the unreachable test-fixture methods reported by `deadcode`.
- Treat the startup provider/model defaults as immutable configuration values,
  not live cross-application getter ports.
- Move recipe Markdown/YAML decoding and skill `SKILL.md` YAML encoding to the
  filesystem adapters that own those formats; Domain keeps only typed product
  values, validation, and lifecycle policy.
- Extend the purity fitness test so Domain/Application cannot import the YAML
  codec again.

Acceptance:

- `features.memory=false` makes all `memory.*` calls return
  `capability_not_negotiated`, matching the protocol contract.
- The test dead-code scan is empty; no removed test-fixture method remains.
- Session views and usage reports consume constructor-provided default values
  without a getter interface or a nil collaborator path.
- Production Domain imports no YAML codec; all recipe/skill frontmatter tests
  run from the owning adapter or infrastructure package.

### Batch 16 — Prompt and content-convention boundary closure

Status: **Completed**

Scope:

- Move Skills project-layout, draft/archive directory, and SKILL.md
  frontmatter-key conventions from Domain to their prompt-source and
  skill-authoring adapters.
- Move AGENTS.md, curated-memory, Todo, and edit-guard model/tool text
  rendering to the agent or tool adapters that consume it.
- Move the maintenance model's Markdown/sentinel fact parsing out of Agent
  Memory Domain while retaining its structured fact normalization invariant.
- Remove the Application unit test's direct adapter dependency and add fitness
  tests that prevent the removed presentation and layout seams from returning.

Acceptance:

- Domain imports no filesystem path package and declares no Skill layout or
  SKILL.md metadata helpers.
- Domain returns structured document, memory, Todo, and edit-guard values; no
  model prompt or tool-refusal text originates there.
- `NO_FACTS` / `NO_MEMORY`, fenced Markdown, and list-marker parsing occur only
  in the maintenance adapter.
- Application unit tests use consumer-side fakes, and architecture tests fail
  if a removed Domain presentation helper returns.

### Batch 17 — Admission and protocol fail-closed closure

Status: **Completed**

Scope:

- Make pending Run admission, live Run ownership, and terminal maintenance one
  atomic Gate lifecycle, so a working-tree mutation cannot enter between a
  Run's live execution and its synchronous terminal checkpoint work.
- Carry approval rememberability as an explicit capability through the domain
  interrupt, application response validation, Delivery protocol, and desktop
  materialization; do not let a client request unsupported durable policy.
- Remove test-only production seams and implicit application defaults that
  disguise missing composition: synthetic Run-coordinator probes, a resolver
  query used only by tests, and silent admission-Gate construction.
- Treat unsupported enum values at the artifact and presenter boundaries as
  failed projections instead of coercing them to ordinary product states.

Acceptance:

- A session/working-tree admission cannot be claimed by a sibling mutation
  while terminal maintenance owns the Run's tree fence.
- A remembered approval is accepted only when the pending approval explicitly
  permits it; unsupported requests remain resumable and return a typed error.
- Production APIs contain no test-observation/query method whose only caller
  is a test; composition errors fail explicitly at the owning use case.
- Delivery persistence and presentation never serialize an unrecognized
  domain enum as a valid ordinary wire value.

### Batch 18 — Composition-input and transparent-alias closure

Status: **Completed**

Scope:

- Remove Adapter aliases that make one ring appear to own an Infra interface;
  the composition root names the real owner directly.
- Remove Bootstrap-only Online/A2A/LSP DTOs and their second mapping pass;
  Bootstrap stores the final Adapter construction inputs it actually composes.
- Use one `MCPServerCandidate` concept for both create and test instead of an
  action-specific alias for the same value.
- Keep only the deliberate JSON-RPC aliases at the transport boundary and
  enforce that exception mechanically.

Acceptance:

- Production contains no transparent alias outside `delivery/transport`'s
  explicit JSON-RPC boundary set.
- Config-source values are projected once into their consuming Adapter inputs;
  `toolenv` does no field-for-field DTO relay.
- MCP create and test share the same canonical candidate type without changing
  their JSON shape.
- Focused build/test, architecture, static analysis, and exact-source scans pass;
  only the already-recorded frontend contract-fixture drift may remain in the
  full repository test.

### Batch 19 — Canonical runtime vocabulary

Status: **Completed**

Scope:

- Remove package-name repetition from internal identifiers: `turn.Handle`,
  `dispatch.Router`, `sessions.View`, `workspace.Summary`, and `models.Details`.
- Name values by what they mean rather than a generic state/config suffix:
  session `Activity`, config `Settings`, and focused config source records.
- Rename the Agent turn's in-process `memoryDispatcher` to an unexported
  `controller`; it controls a turn lifecycle and is not the Delivery JSON-RPC
  router. Keeping it private avoids widening the Adapter surface.
- Rename MCP `Connections` to `Pool` and remove stale filenames/comments that
  preserve an obsolete implementation term.
- Keep public wire vocabulary stable, including `protocol.ProtocolRange`, RPC
  method names, JSON members, and generated frontend type names.

Acceptance:

- Runtime-wide compilation finds no old identifier; exact source scans and a
  vocabulary fitness test prevent the removed names from returning.
- Session activity has one term through Application and its Delivery projection;
  Delivery does not regain the activity-precedence decision.
- Turn control and request routing use distinct, accurate concepts (the private
  turn `controller` versus `dispatch.Router`).
- Focused behavior tests, architecture checks, vet, static analysis, lint, and
  dead-code analysis pass without changing frontend files.

### Batch 20 — Responsibility and entrypoint convergence

Status: **Completed**

Scope:

- Separate Bootstrap object-graph construction, Host process lifetime, and
  resource-close mechanics into `assemble.go`, `host.go`, and `resources.go`.
- Replace Runs' catch-all `usecases.go` with focused Start, Resume, Cancel,
  Steer, and dependency-validation files while preserving one package and its
  transactional/state-machine boundaries.
- Remove the unused unbounded `sessions.ListViews` read; the bounded
  `ListViewPage` query is the one session-list entrypoint.
- Remove the package-level `workspacepath.ResolveExistingDir` convenience
  function; the `Resolver` method is the one port implementation.
- Retain cohesive large cancellation, transcript, SQLite Run, and contract-shape
  implementations where splitting would scatter one invariant rather than
  separate responsibilities.

Acceptance:

- Bootstrap lifetime and resource-closing declarations cannot return to the
  assembly file, and Runs' independent commands cannot return to a catch-all
  use-case file.
- There is one callable entrypoint for bounded session listing and existing-dir
  resolution; no compatibility wrapper remains.
- Runtime-wide compile/test, focused race tests, architecture checks, vet,
  static analysis, lint, dead-code analysis, formatting, and empty-directory /
  removed-source scans pass, aside from the known deferred frontend contract
  fixture drift.

### Batch 21 — Agent-memory domain closure

Status: **Completed**

Scope:

- Replace permissive integer scope/status/origin values with closed string
  vocabularies whose zero value is invalid and whose parsers reject unknown
  durable tokens.
- Model human review as `ReviewDecision`, not as a caller-selected target
  status, and permit only the pending-to-active/rejected transition.
- Validate durable Item identity, partition, provenance, state, content, and
  timestamps in Domain before persistence.
- Make SQLite apply review atomically and enforce the same vocabulary,
  partition, provenance/state, and boolean constraints in schema epoch 52.

Acceptance:

- Corrupt storage values cannot become project, active, or auto through a
  default branch.
- Direct Application callers cannot select an invalid scope or set arbitrary
  memory state; repeated/non-pending review reports a typed domain error.
- Domain/Application/SQLite/Delivery normal and race tests, runtime-wide
  compile, standalone build/vet, static analysis, lint, and dead-code analysis
  pass; the known deferred frontend contract/sample drift may remain.

### Batch 22 — Ambiguous-default closure

Status: **Completed**

Scope:

- Replace human-authored knowledge's permissive integer scope with a closed
  string vocabulary whose zero value is invalid.
- Make Delivery, Application, and filesystem storage reject unknown knowledge
  scopes instead of silently selecting project knowledge or treating the
  request as unavailable.
- Replace transcript page order's permissive integer value with a closed
  vocabulary and validate it at Domain, Application, and SQLite boundaries.
- Reject negative durable-sequence anchors and page limits instead of
  reinterpreting them as an unbounded first page.

Acceptance:

- Unknown knowledge scopes cannot read or overwrite project `LYRA.md` through
  a default branch, and corrupt projected scopes cannot be emitted on the wire.
- Unknown transcript orders cannot mint a cursor or choose SQL direction;
  invalid direct-store pagination controls fail before a query is built.
- Focused normal/race tests, runtime-wide compile, standalone build/vet,
  static analysis, lint, and dead-code analysis pass; the known deferred
  frontend contract/sample drift may remain.

### Batch 23 — Composition-boundary ownership

Status: **Completed**

Scope:

- Make cross-ring adapter constructors return their concrete implementation;
  Bootstrap assigns those implementations to Application-owned ports.
- Remove adapter imports used only to name a consumer interface at the return
  boundary and add an architecture fitness test for the concrete constructors.
- Split Assembly construction input from composition-root union ports, and
  replace the stuttering `RuntimeConfig` factory with the verb-shaped
  `ComposeConfig` entrypoint.
- Preserve Run scope, model-selection, and trace values when `TurnProcess`
  detaches cancellation for joins and framework-only auto-continuations.

Acceptance:

- Adapter constructors no longer hide concrete ownership behind the interface
  of the Application consumer; Bootstrap remains the only assignment point.
- Assembly input and composition ports live in files named for their distinct
  responsibilities; the removed catch-all config-types file does not return.
- Auto-continuation is request-cancellation-independent without severing
  execution context or tracing lineage.
- Focused normal/race tests, architecture fitness tests, runtime-wide compile,
  standalone build/vet, static analysis, lint, and dead-code analysis pass;
  the known deferred frontend contract/sample drift may remain.

### Batch 24 — Immutable runtime catalogs

Status: **Completed**

Scope:

- Replace exported mutable runtime-topic, canonical-sample, change-resource,
  CORS-origin, and replay-retention globals with same-concept query functions
  that return caller-owned slices or values.
- Keep capability advertisement, subscription validation, wire-enum
  generation, contract generation, change projection, and replay enforcement on
  the same catalogs without sharing writable backing state.
- Add behavior tests for caller ownership and an architecture fitness test that
  rejects future exported composite-literal vars.

Acceptance:

- Mutating any returned catalog/default cannot change what another runtime
  boundary validates, advertises, generates, or enforces.
- No exported slice, map, or struct-literal variable remains in production
  `app/runtime`; typed error sentinels and private immutable-by-convention tables
  remain allowed.
- Focused normal/race tests, architecture fitness tests, runtime-wide compile,
  standalone build/vet, static analysis, lint, and dead-code analysis pass;
  the known deferred frontend contract/sample drift may remain.

### Batch 25 — Mutable-value and atomic-write ownership

Status: **Completed**

Scope:

- Make wire-enum and system-invariant catalogs return deep caller-owned
  snapshots, including nested boundary slices.
- Clone mutable `RunCapabilities` at registry ingress and egress, and
  remove the unused Run-list surfaces that exposed additional state.
- Replace the process-global fixed `LYRA.md.tmp` convention with a unique
  same-directory temporary file, durable close, and atomic replacement.

Acceptance:

- A caller cannot mutate canonical wire enums, nested system invariants, or a
  live Run's interrupt profile through an input or returned slice.
- Concurrent knowledge-store instances never contend for or reserve one fixed
  temporary path, and the final `LYRA.md` is always one complete write.
- Focused normal and race tests for the affected catalogs, Run registry, and
  filesystem storage pass.

### Batch 26 — Process-path composition ownership

Status: **Completed**

Scope:

- Resolve User Home once at the executable composition root and fail startup
  when it is unavailable instead of silently constructing divergent defaults.
- Replace every `DefaultCwd` field and local with the exact
  `DefaultWorkspacePath` term; keep User Home and the default-workspace product
  choice distinct even when the current policy gives them the same path.
- Inject the same path snapshot into hooks, workspace use cases, scheduled Run
  starts, Agent prompt/recall, tool fallback workspaces, and server metadata.
- Remove Agent prompt's ambient `os.UserHomeDir` and `os.Getwd` fallbacks.

Acceptance:

- Production Bootstrap, Agent execution, and Delivery server code cannot query
  ambient User Home or process cwd; an architecture fitness test guards the
  composition boundary.
- The removed `DefaultCwd` / `defaultCwd` vocabulary has no Go identifier
  occurrence, and Bootstrap rejects either missing composition path.
- Focused normal and race tests plus the composition-boundary fitness test pass.

### Batch 27 — Item timestamp vocabulary closure

Status: **Completed**

Scope:

- Replace the durable transcript Item's misleading `CreatedAt` with the neutral
  `OccurredAt` fact and rename open ToolCall state to `startedAt`.
- Make live and artifact unions expose exactly one variant-specific timestamp:
  ToolCall uses `startedAt`; all other Item kinds use `createdAt`.
- Map variant time explicitly in Delivery, require the occurrence timestamp at
  Domain and SQLite boundaries, and retain `finishedAt` / `durationMs` only for
  ToolCall terminal frames.
- Carry the original Item occurrence through durable interrupt and drained-tool
  continuation hand-offs, so resume completes or reopens the same Item without
  consulting a new clock.
- Rename the generic history timestamp column to `occurred_at`, advance the
  incompatible SQLite schema epoch to 53, and advance the artifact schema to
  v12 without a migration or compatibility decoder.

Acceptance:

- A live or archived ToolCall cannot carry `createdAt`; a non-ToolCall cannot
  carry `startedAt`; JSON presentation and generated union validators prove the
  exclusivity.
- Domain and storage reject a transcript Item with no occurrence timestamp,
  and ToolCall duration is derived only from `OccurredAt` to `FinishedAt`.
- A cold-restart continuation preserves Question and ToolCall occurrence
  identity; SQLite rejects an attempted timestamp rewrite of an existing Item.
- Server-side contract artifacts and Go validators describe artifact v12 and
  the exclusive timing union; focused normal/race tests and architecture
  fitness checks pass. Frontend generated artifacts remain deferred by scope.

### Batch 28 — Ambient path ownership closure

Status: **Completed**

Scope:

- Extend the executable-owned path snapshot to distinguish User Home, default
  workspace, durable data directory, and launch directory. Resolve `LYRA_HOME`,
  process cwd, and User Home exactly once in `cmd/lyra`.
- Replace persistence's no-argument `Open`, the knowledge store's ambient
  constructor, and the ambiguous `Bundle.Home` with explicit configuration and
  `DataDirectory`; delete the former storage-home resolver entirely.
- Pass User Home explicitly into both sandbox confinement models, root the
  default local token in DataDirectory, and make config search directories
  executable-supplied absolute paths.
- Remove implicit `filepath.Abs` behavior from path identity, workspace path,
  hooks, prompt-source, sandbox archive, and read-only policy adapters. Relative
  resource paths remain valid only when paired with an explicit absolute root.
- Enforce absolute path invariants at runtime assembly and isolation
  construction, including optional skill, recipe, checkpoint, sandbox, and
  read-only roots; constructors own caller-supplied slices.

Acceptance:

- No production file under `internal/` calls `os.UserHomeDir`, `os.Getwd`, or
  `filepath.Abs`; an architecture fitness rule scans the entire inner runtime.
- Persistence, knowledge, toolset, sandbox, configuration, transport, workspace
  path, hooks, and prompt-source boundaries reject missing or relative process
  paths instead of consulting ambient host state.
- Focused normal/race tests, runtime-wide build/vet, standalone module checks,
  static analysis, lint, dead-code analysis, and final structural scans pass.

### Batch 29 — Delivery semantic boundary closure

Status: **Completed**

Scope:

- Keep runtime-event observation private to Delivery, move file-change notices
  into the Workspace application vocabulary, and use `Runs` consistently for
  the Run use-case dependency.
- Replace Goal's ordinal `ReasonCause` and Delivery-authored English sentence
  with a durable semantic `ReasonCode` plus optional detail, and expose the same
  closed vocabulary as `GoalReason { code, detail? }` on the wire.
- Persist Goal reason codes as text, advance the incompatible SQLite schema
  epoch to 54, and advance the single supported protocol version to
  `2026-08-04` without a migration, alias, or compatibility decoder.
- Inject the enforced idempotency replay limit into Delivery construction
  instead of reading a component implementation constant from the server.
- Remove the single-function tool-presentation file and stale comments that
  claimed Delivery reshaped tool results or named concrete inner adapters.

Acceptance:

- `delivery/server.Server` exports only `protocol.Runtime` plus `Close`, and
  production server code imports no concrete adapter, infrastructure,
  composition, or idempotency implementation package.
- Goal stopping context remains typed from Domain through SQLite and protocol;
  Delivery contains no user-facing reason prose and rejects unpublished codes.
- The Go-side contract, protocol documentation, build, vet, focused behavior
  tests, and architecture fitness checks pass. Frontend generated bindings and
  canonical samples remain the explicitly deferred integration block.

### Batch 30 — Delivery projection vocabulary closure

Status: **Completed**

Scope:

- Replace the mixed `xWire`, `xToWire`, and pointer-shape helper names in
  `delivery/server` with one outbound `presentX` vocabulary.
- Retain `xFromWire` only for inbound protocol-to-inner mapping, where the
  direction is meaningful and unambiguous.
- Merge any duplicate projection exposed by the rename instead of preserving
  two implementations behind new names.

Acceptance:

- No production package function in `delivery/server` ends in `Wire` or
  `ToWire` unless it explicitly ends in `FromWire`.
- Run and aggregate usage share one `presentModelUsage`; there is no duplicate
  token/cost field mapping.
- Server and architecture tests, runtime-wide build, and vet pass.

## 6. Progress

| Batch | Status | Started | Completed | Evidence |
|---|---|---|---|---|
| 1. Run boundary semantics | Completed | 2026-07-22 | 2026-07-22 | `go test -race ./internal/application/runs ./internal/application/goals`; `go vet` for both packages; `go test ./internal/arch`. |
| 2. Application collaboration ownership | Completed | 2026-07-22 | 2026-07-23 | `go test -race` for admission, runs, sessions, schedules, delivery/server, bootstrap; focused `go vet`; `go test ./internal/arch`. |
| 3. Model/provider policy ownership | Completed | 2026-07-23 | 2026-07-23 | `go test -race ./internal/application/models ./internal/delivery/server ./internal/bootstrap`; `go vet ./...`; `go test ./...`; `go test ./internal/arch`. |
| 4. Abstraction and boundary cleanup | Completed | 2026-07-23 | 2026-07-23 | Full build, vet, test, focused race suites, and architecture tests passed after dispatcher, registry, HITL, and suspension cleanup. |
| 5. Fitness tests and final verification | Completed | 2026-07-23 | 2026-07-23 | Workspace and standalone build/vet/test, relevant race suites, and expanded semantic architecture tests passed. |
| 6. Semantic boundary closure | Completed | 2026-07-23 | 2026-07-23 | Workspace/standalone build, vet, test; `go test ./internal/arch`; Delivery adapter-import and construction ownership checks. |
| 7. Residual abstraction-leak eradication | Completed | 2026-07-23 | 2026-07-23 | `go test ./...`; standalone vet/test; focused race suite; expanded filesystem and Session-domain architecture checks. |
| 8. Delivery abstraction-leak closure | Completed | 2026-07-23 | 2026-07-23 | Portable-session artifact tests, `go test ./internal/arch`, protocol sample validation, and full module verification. |
| 9. Read-model, secret, and execution-boundary closure | Completed | 2026-07-23 | 2026-07-23 | Delivery/server, application, adapter, architecture, and full-module test suites verified the ownership moves. |
| 10. Residual ownership closure and non-destructive abstraction audit | Completed | 2026-07-23 | 2026-07-23 | Full module tests and architecture tests verified the Bootstrap, archive, notifier, and consumer-audit changes. |
| 11. Final abstraction-leak remediation | Completed | 2026-07-23 | 2026-07-23 | `go test ./...`; `go vet ./...`; exact-symbol scans for removed query/Turn/Transport/sandbox facades; Application/Domain dependency-direction scan. |
| 12. Runtime ownership closure | Completed | 2026-07-23 | 2026-07-23 | Workspace and standalone test/vet/build; focused race suites; `staticcheck`; `golangci-lint`; `go test ./internal/arch`; and source scans for removed seams passed. |
| 13. Consumer-port and state-ownership closure | Completed | 2026-07-23 | 2026-07-23 | Workspace/standalone build, vet, and test; focused `-race` suites; `staticcheck`; `golangci-lint`; `go test ./internal/arch`; and source scans for removed domain interfaces passed. |
| 14. Residual consumer-port closure | Completed | 2026-07-23 | 2026-07-23 | Workspace/standalone build, vet, and test; focused `-race`; `staticcheck`; `golangci-lint`; architecture tests; exact-symbol scans; and classified `deadcode` output. |
| 15. Protocol, static-config, and content-codec closure | Completed | 2026-07-23 | 2026-07-23 | Workspace and standalone build/vet/test; focused race suite; architecture test; `staticcheck`; `golangci-lint`; exact-symbol scans; and `deadcode -test ./...` all passed. |
| 16. Prompt and content-convention boundary closure | Completed | 2026-07-23 | 2026-07-23 | Workspace/standalone build, vet, and test; focused race suite; `staticcheck`; `golangci-lint`; `deadcode -test`; architecture tests; and exact-source scans passed. |
| 17. Admission and protocol fail-closed closure | Completed | 2026-07-24 | 2026-07-24 | `go build ./...`; `go vet ./...`; `go test ./...`; focused `go test -race` for admission/runs/sessions/delivery; frontend typecheck, lint, and 811 tests passed. |
| 18. Composition-input and transparent-alias closure | Completed | 2026-08-04 | 2026-08-04 | Runtime-wide compile, focused tests, dependency/alias fitness tests, `go vet`, `staticcheck`, `golangci-lint`, `deadcode -test`, and exact alias scans passed; the full architecture suite retained only the known stale frontend-contract failures. |
| 19. Canonical runtime vocabulary | Completed | 2026-08-04 | 2026-08-04 | Runtime-wide compile, focused behavior tests, dependency/vocabulary/activity fitness tests, `go vet`, `staticcheck`, `golangci-lint`, `deadcode -test`, formatting, and exact removed-name scans passed. |
| 20. Responsibility and entrypoint convergence | Completed | 2026-08-04 | 2026-08-04 | Runtime-wide compile; focused normal/race tests; architecture checks; standalone build/vet; `staticcheck`; `golangci-lint`; `deadcode -test`; formatting and exact-source scans passed. Full tests retain only the known deferred frontend contract/sample drift. |
| 21. Agent-memory domain closure | Completed | 2026-08-04 | 2026-08-04 | Domain/Application/SQLite/Delivery normal and race tests, runtime-wide compile, standalone build/vet, `staticcheck`, `golangci-lint`, `deadcode -test`, schema-constraint tests, and full regression classification passed. |
| 22. Ambiguous-default closure | Completed | 2026-08-04 | 2026-08-04 | Knowledge and transcript Domain/Application/Storage/Delivery normal and race tests, runtime-wide compile, standalone build/vet, static analysis, lint, dead-code analysis, and full regression classification passed. |
| 23. Composition-boundary ownership | Completed | 2026-08-04 | 2026-08-04 | Adapter/Bootstrap/Agent execution normal and race tests, architecture constructor fitness tests, runtime-wide compile, standalone build/vet, static analysis, lint, dead-code analysis, and full regression classification passed. |
| 24. Immutable runtime catalogs | Completed | 2026-08-04 | 2026-08-04 | Catalog ownership tests, architecture mutable-global fitness test, focused normal/race tests, runtime-wide compile, standalone build/vet, static analysis, lint, dead-code analysis, and full regression classification passed. |
| 25. Mutable-value and atomic-write ownership | Completed | 2026-08-04 | 2026-08-04 | Deep-ownership tests plus focused normal/race suites for contracts, protocol enums, Run registry, execution profiles, and filesystem storage passed. |
| 26. Process-path composition ownership | Completed | 2026-08-04 | 2026-08-04 | Executable, Agent prompt, Workspace, Runs, Schedules, Bootstrap, and Delivery tests plus the ambient-path architecture fitness rule passed. |
| 27. Item timestamp vocabulary closure | Completed | 2026-08-04 | 2026-08-04 | Domain/Application/SQLite/Delivery normal and race tests, generated Go validators, server contract artifacts, JSON exclusivity assertions, and timestamp architecture fitness checks passed. |
| 28. Ambient path ownership closure | Completed | 2026-08-04 | 2026-08-04 | Explicit-path boundary tests, runtime-wide ambient-path fitness scan, focused normal/race suites, build/vet, static analysis, lint, dead-code analysis, and structural residue scans passed. |
| 29. Delivery semantic boundary closure | Completed | 2026-08-04 | 2026-08-04 | Runtime event/export and server dependency fitness checks, Goal reason projection/storage tests, Go contract generation, runtime-wide build/vet, and focused server/domain/application/storage tests passed; full drift remains limited to deferred frontend bindings and canonical samples. |
| 30. Delivery projection vocabulary closure | Completed | 2026-08-04 | 2026-08-04 | All server projection helpers use `presentX`, inbound mappers alone retain `FromWire`, duplicate model-usage mapping was removed, and server/architecture/build/vet checks passed. |
| 31. Concrete tool semantics ownership | Completed | 2026-08-04 | 2026-08-04 | Tool safety, approval-subject extraction, and offload reader naming moved to toolset/composition; Domain and architecture tests no longer know the built-in catalog. |
| 32. Semantic content and explicit storage codecs | Completed | 2026-08-04 | 2026-08-04 | Content is media-semantic inside the runtime; Delivery owns MIME/base64 wire decoding, SQLite owns explicit transcript/interrupt rows, and schema epoch 55 rejects the former implicit aggregate encoding. |
| 33. Run capability vocabulary closure | Completed | 2026-08-04 | 2026-08-04 | Domain, Application, adapters, and SQLite use `RunCapabilities`; the versioned Delivery DTO alone retains `RunProtocolProfile`, with one explicit mapping boundary and schema epoch 56. |
| 34. Semantic replay retention and interrupt integrity | Completed | 2026-08-04 | 2026-08-04 | Replay memory is charged from the closed event family rather than JSON encoding; pending approvals now require a valid tool, risk, and not-yet-executed invocation. |
| 35. Execution identity and Goal Run vocabulary closure | Completed | 2026-08-04 | 2026-08-04 | Domain/Application use `ExecutorRef`, `ExecutionScope`, `ExecutionControl`, and Goal `Run` accounting exclusively; adapter-local `Turn` no longer crosses inward, SQLite uses `executor_id`/`goal_runs`/`max_runs`, server contracts were regenerated, and semantic-boundary fitness tests pin the retired vocabulary. |

Allowed status values: `Pending`, `In progress`, `Completed`, `Blocked`, `Revised`.

## 7. Progress log

### 2026-08-04 — Batch 35 completed

- Replaced the Agent adapter's leaked Turn handle vocabulary in Domain and
  Application with `ExecutorRef{SessionID, ExecutorID}`, `ExecutionScope`,
  `ExecutionControl`, `StartExecution`, and Segment-scoped events. The concrete
  `adapter/agentexec/turn` package keeps its native `TurnID` internally and maps
  it explicitly at the adapter boundary.
- Unified autonomous Goal accounting on the existing product lifecycle unit:
  one completed autonomous `Run`. Domain, Application, tool schema, server API,
  reason codes, generated server contracts, and persistence now use `MaxRuns`,
  `Runs`, `RunRecord`, `runBudgetReached`, `max_runs`, and `goal_runs`; no
  compatibility aliases or dual read/write paths remain.
- Advanced the migration-free SQLite shape to epoch 57, removed concrete
  storage/adapter descriptions from Domain package documentation, and added an
  AST/source fitness rule that rejects retired Turn handles and Goal Turn
  vocabulary in inner rings and durable storage.
- Server-owned focused suites, Domain/Application/adapter/SQLite tests, and the
  server contract generator pass. The full contract drift gate intentionally
  remains blocked only by deferred frontend `goal.json` and generated TypeScript
  bindings, which this server-only batch does not modify.

### 2026-08-04 — Batch 34 completed

- Replaced JSON serialization as the replay window's memory proxy with explicit
  retained-memory accounting over the closed `RunEvent` family. Every new event
  variant must now implement the private accounting obligation; text, media,
  tool values, plan snapshots, nested interrupts, and slice/map backing storage
  are charged without importing a delivery or persistence encoding.
- Made approval integrity one end-to-end invariant. Application approval
  prompts and durable transcript approvals require a recognized risk level;
  durable approvals additionally reject blank/padded tool names and invocations
  that already carry a result or offload reference. Question payloads are also
  validated when a pending interrupt set is admitted.
- Removed the adapter's stale framework-qualified denial message, the orphaned
  hook-output comment, and the ambiguous `executor protocol` error name. Added
  architecture coverage preventing replay accounting from returning to JSON.

### 2026-08-04 — Batch 33 completed

- Replaced the inward-leaking `RunProtocolProfile` vocabulary with the semantic
  `RunCapabilities` value and standardized aggregate fields on `Capabilities` or
  `CallerCapabilities`. Resume, subscribe, pending reads, recovery, and portable
  snapshots now name the same concept the same way.
- Kept `RunProtocolProfile` only as the versioned Delivery DTO and made its
  conversion to `RunCapabilities` an explicit server-boundary operation. Domain,
  Application, adapters, and SQLite contain no protocol-profile identifiers.
- Renamed the SQLite columns and JSON row vocabulary to `capabilities` and
  `interruptKinds`, advanced directly to epoch 56, and added an architecture
  fitness rule that prevents the protocol terminology or former column from
  leaking inward again.

### 2026-08-04 — Batch 32 completed

- Replaced the internal image carrier's encoded `Mime`/`Data` strings with an
  ownership-isolated `MediaType` plus raw bytes. Delivery now exclusively
  validates MIME types and decodes/encodes base64 at HTTP and artifact
  boundaries; Application and Domain no longer interpret transport encodings.
- Replaced direct JSON serialization of transcript Items, pending Interrupts,
  committed tool Problems, and model selections with explicit SQLite-owned
  payload rows and exhaustive semantic mappings. Renaming a Domain field can no
  longer silently rewrite the durable format.
- Advanced the deliberately migration-free SQLite shape to epoch 55 and added
  deep-copy coverage for image bytes. The old encoded content and implicit Go
  aggregate shapes have no compatibility reader.

### 2026-08-04 — Batch 31 completed

- Removed the built-in name-to-safety table and `read_tool_result` name from
  Domain. `toolset.Semantics` now owns classification, remembered-rule subject
  extraction, and catastrophic shell-command projection next to the concrete
  tool schemas; unknown extension tools still fail closed as arbitrary exec.
- Approval policy now accepts the supplied safety class and already-derived
  subject. It retains mode, scope, precedence, bypass immunity, and durable rule
  invariants without interpreting `shell.command` or file-tool `path` fields.
- `agentexec/turn` consumes the new semantics through its own required narrow
  interface. Tool-result eviction receives its read-back capability name from
  Bootstrap, so neither Domain nor Agent execution hard-codes that tool.
- Added an architecture fitness check that rejects a concrete built-in inventory
  or known tool-name literals in approval Domain.

### 2026-08-04 — Batch 30 completed

- Standardized every outbound `delivery/server` mapper on `presentX`; removed
  the ambiguous `xWire`, `xToWire`, and shape-oriented `goalPtr` vocabulary.
  Inbound mapping consistently retains `xFromWire`.
- The convergence exposed two identical model-usage projections in
  `presenter_run.go` and `usage.go`. Deleted the aggregate-usage copy so token
  and cost fields have one projector shared by Run and usage APIs.
- Added a syntax-level fitness check that rejects any future outbound server
  helper ending in `Wire` or `ToWire` while allowing explicit `FromWire`
  decoders.

### 2026-08-04 — Batch 29 completed

- Closed Delivery's event ingress: runtime sources are constructor-only,
  file-change notices belong to `application/workspace`, and the concrete
  Server has no exported event-injection or capability method outside its
  protocol contract.
- Standardized the Run use-case dependency as `Runs` from Bootstrap through
  Delivery and removed the last coordinator-oriented production comments.
- Replaced Goal's ordinal cause with a stable string reason code across Domain,
  application, model-facing tools, SQLite, and protocol. The API now returns
  structured reason context and leaves localization to clients.
- Advanced SQLite to epoch 54 and the protocol to `2026-08-04`; regenerated
  only Go/server artifacts and updated the canonical protocol prose. No legacy
  column, decoder, dual shape, or compatibility branch remains.
- Removed Delivery's idempotency-component dependency, the redundant tool
  presentation file, and stale claims about tool-result reshaping. Added fitness
  checks for concrete Server exports, Workspace event ownership, typed Goal
  reasons, and server dependency boundaries.
- Runtime-wide build and vet plus focused architecture, Domain, application,
  adapter, storage, Delivery, Bootstrap, and command tests pass. Full contract
  drift is intentionally deferred only for frontend-generated bindings and
  canonical samples, which this server-only batch does not edit.

### 2026-08-04 — Batch 28 completed

- Moved the final host-path lookups into `cmd/lyra`: User Home, default
  workspace, DataDirectory, and launch directory now form one immutable process
  snapshot. `LYRA_HOME` is required to be absolute when explicitly configured.
- Replaced `persistence.Open()` and `storage.NewFileKnowledgeStore()` ambient
  discovery with required configs, renamed `Bundle.Home` to `DataDirectory`, and
  deleted `storage.Home` rather than retaining a compatibility path.
- Removed inner User Home/cwd lookups from sandbox and HTTP token issuance, and
  removed every internal `filepath.Abs` fallback. Config files, hooks, agent
  documents, workspace identity, path locks, archives, and sandbox policies now
  consume explicit absolute roots and fail closed on missing context.
- Replaced the workspace canonicalizer's silent empty-string fallback with an
  explicit error and tightened runtime assembly plus isolation constructors so
  relative configuration cannot survive until first filesystem use.
- Expanded the architecture fitness rule from three selected packages to all of
  `internal/`, covering `os.UserHomeDir`, `os.Getwd`, and `filepath.Abs` so this
  ownership cannot silently regress.

### 2026-08-04 — Batch 27 completed

- Replaced transcript Item's ambiguous `CreatedAt` state with `OccurredAt`, and
  renamed the reducer's open ToolCall boundary to `startedAt`. Delivery is now
  the only layer that selects the external time term from the Item variant.
- Changed live Item and portable ArtifactItem unions so ToolCall requires
  `startedAt` and forbids `createdAt`, while every other variant does the
  reverse. Terminal ToolCalls still require `finishedAt` and the derived
  non-negative `durationMs`.
- Advanced the artifact format directly to v12 and the SQLite schema epoch to
  53; `history_items.occurred_at` is the sole relational timestamp. No old
  field, old column, migration, dual decoder, or compatibility alias remains.
- Closed the restart edge exposed by immutable occurrence identity: Pending
  interrupts and drained-tool hand-offs now retain the referenced Item's
  original occurrence, and reducer resume paths reuse that fact for both
  Question completion and ToolCall reopening.
- Regenerated the server contract artifacts and Go wire validators only. The
  intentionally untouched frontend generated types, validators, and samples
  remain the known follow-up drift for the dedicated frontend wiring round.

### 2026-08-04 — Batch 26 completed

- Introduced one executable-owned runtime-path snapshot. User Home resolution
  now fails closed, and the same values reach Bootstrap construction and HTTP
  server metadata instead of being queried independently at five boundaries.
- Replaced the misleading `DefaultCwd` vocabulary throughout Run opening,
  schedules, workspace context, Delivery mapping, and Skill proposals with
  `DefaultWorkspacePath`. This is a deliberate breaking rename with no alias or
  compatibility field.
- Made Bootstrap the sole source for Agent `Workdir` and `UserHome`, removing
  per-prompt User Home and process-cwd lookups. Added behavior tests for the
  injected AGENTS.md home plus a fitness test that rejects ambient path queries
  from Bootstrap, Agent execution, or Delivery server code.

### 2026-08-04 — Batch 25 completed

- Closed the remaining shallow-copy leaks in generated wire enums and system
  invariants. Every public catalog query now returns a deep caller-owned
  snapshot rather than a new outer slice that still aliases nested state.
- Added an explicit clone operation to `RunCapabilities` and made the Run
  registry clone capability sets on both ingress and egress. Removed the unused
  registry/coordinator list APIs instead of maintaining another mutable-state
  exposure solely for symmetry.
- Reworked human-authored knowledge replacement around a unique temporary file
  in the target directory, followed by sync, close, and atomic rename. The
  fixed `LYRA.md.tmp` path is no longer a shared multi-instance lock or
  collision point; concurrent writers are verified under the race detector.

### 2026-08-04 — Batch 24 completed

- Replaced exported writable `RuntimeTopics`, `CanonicalSamples`, `Resources`,
  `DefaultCORSOrigins`, and `DefaultRetention` values with functions bearing
  the same domain term and returning an independent snapshot/value. No aliasing
  remains between caller configuration and canonical package facts.
- Updated capability discovery, runtime subscription validation/resync,
  wire-enum generation, contract sample generation, change projection, HTTP
  startup, and replay defaults to consume the immutable query surface.
- Added mutable-slice ownership tests plus an AST fitness rule that refuses any
  future exported composite-literal `var` in production runtime code.
- Re-ran the structural scans: no empty production directory, TODO/FIXME/HACK
  marker, removed composition name, constant `fmt.Errorf`, or exported mutable
  composite catalog remains. The ignored, user-owned empty `app/runtime/LYRA.md`
  knowledge file is intentionally outside source hygiene and was preserved.

### 2026-08-04 — Batch 23 completed

- Replaced three adapter factories that returned Application interfaces with
  concrete `DiagnosticRegistry`, `SessionCheckpoints`, and
  `SessionTurnCleanup` implementations. The hook factory likewise returns its
  concrete resolver; Bootstrap remains responsible for assigning each value to
  a consumer-owned port.
- Removed the tool registry's construction-only dependency on
  `application/tools` and the Agent turn cleanup adapter's construction-only
  dependency on `application/sessions`. Added a fitness test that rejects a
  return to consumer-interface construction at these seams.
- Split the former `runtime_config_types.go` catch-all into
  `assembly_config.go` and `composition_ports.go`; renamed the settings-to-input
  function and file to `ComposeConfig` / `config_composition.go` so the name
  describes an action rather than introducing a second Config concept.
- Replaced bare background contexts in `TurnProcess.Await` with a single
  cancellation-detached Run context. Joins and internally driven continuations
  now retain execution scope, model selection, and tracing values.
- Replaced the remaining constant `fmt.Errorf` with `errors.New`; cohesive
  state-machine, codec, SQL, and generated-shape files remain intact rather
  than being split by line count alone.

### 2026-08-04 — Batch 22 completed

- Made human-authored knowledge scope a closed string vocabulary. Unknown
  Domain values now fail in Application and filesystem storage; unknown wire
  scopes fail as invalid parameters; invalid stored projections fail rather
  than being emitted as project memory.
- Made transcript sequence order a closed string vocabulary and validated it
  before cursor construction and again at the SQLite adapter boundary. The
  zero value and unfamiliar directions no longer mean oldest-first.
- Tightened direct pagination inputs so negative sequence anchors and limits
  cannot silently become an unanchored or unbounded query; non-positive cursor
  sequence keys are rejected as invalid cursors.
- Kept the knowledge store's same-directory `os.Rename` deliberately: it is the
  atomic last-write-wins commit from a fixed temporary sibling, not a
  cross-filesystem move or conflict-preserving import, so `fileflow` would
  change the bounded-context semantics rather than improve them.

### 2026-08-04 — Batch 21 completed

- Replaced agent-memory's permissive integer enums with closed string values.
  Unknown stored scope, status, or origin now fails decoding instead of becoming
  project, active, or auto; the zero value is no longer a valid business fact.
- Replaced the `SetStatus` persistence seam and status-shaped Application
  command with one `ReviewDecision`. SQLite applies only
  pending-to-active/rejected transitions atomically and distinguishes missing
  from already-resolved items.
- Added Domain validation for durable Item identity, scope/project partition,
  content, provenance, lifecycle, and timestamps. Constructors now return only
  valid Items or an error.
- Advanced the single current SQLite shape to epoch 52 and encoded the same
  vocabulary, scope/project, user-origin, and pinned constraints in the table.
- Focused normal and race tests, runtime-wide compilation, schema constraint
  tests, and full regression classification pass. Full-test failures remain
  limited to the previously recorded frontend generated-contract and protocol
  sample drift.

### 2026-08-04 — Batch 20 completed

- Split Bootstrap by responsibility: `assemble.go` owns object-graph
  construction and validation, `host.go` owns process lifetime and startup
  recovery, and `resources.go` owns close ordering and pending-resource cleanup.
- Replaced Runs' catch-all `usecases.go` with focused opening, resume,
  cancellation, steering, and dependency files. Cancellation remains one
  cohesive state machine rather than being fragmented by file-size pressure.
- Removed the unused unbounded `sessions.ListViews` query and the package-level
  `workspacepath.ResolveExistingDir` convenience function. The bounded page
  query and the `Resolver` method are now the single entrypoints; no wrappers or
  compatibility paths remain.
- Added architecture fitness checks that prohibit the removed entrypoints and
  prevent lifecycle/resource declarations or independent Run commands from
  accumulating in catch-all files. Updated the continuation-fact guard to name
  its new owning file.
- Runtime-wide compilation, focused normal and race tests, architecture checks,
  standalone build/vet, `staticcheck`, `golangci-lint`, `deadcode -test`,
  formatting, empty-directory scans, and exact removed-source scans pass. The
  full suite fails only on the already-recorded frontend generated-contract and
  protocol-sample drift deferred to the frontend wiring round.

### 2026-08-04 — Batch 19 completed

- Replaced package-stuttering identifiers with one canonical vocabulary:
  `turn.Handle`, `dispatch.Router`, `sessions.Store/View/Activity/Admission`,
  `workspace.Resolved/Summary/Catalog`, `models.Details`, and
  `mcpconnection.Pool`.
- Recast loaded process configuration as `config.Settings` with focused
  `Server`, `Online`, `MCPServer`, `A2AAgent`, and `LSPServer` source records;
  the generic `types.go` became `settings.go`.
- Renamed the Agent turn's `memoryDispatcher` implementation and file to a
  private `controller`. It remains concrete and type-inferred outside the
  package, so the cleanup did not widen the Adapter API. Delivery request
  routing now consistently uses `dispatch.Router` and `router` fields.
- Session read models carry `Activity` rather than a generic `State`, and
  Delivery's `sessionActivityToWire` only projects that application decision.
  JSON members and RPC/generated wire vocabulary did not change.
- Renamed implementation files to match their roles (`controller.go`,
  `pool.go`, `router.go`) and added a fitness test for every removed term while
  deliberately retaining the public wire name `protocol.ProtocolRange`.
- Runtime-wide compile, focused behavior tests, dependency/vocabulary/activity
  fitness tests, `go vet`, `staticcheck`, `golangci-lint`, `deadcode -test`,
  formatting, and exact removed-name scans all pass.

### 2026-08-04 — Batch 18 completed

- Removed the MCP adapter's transparent OAuth-store alias; Bootstrap now names
  the Infra-owned consumer interface directly, as a composition root should.
- Removed Bootstrap's duplicate Online/A2A/LSP DTO layer. Source config is
  projected once into the final Toolset/Codeintel construction types, and
  `toolenv` passes those values without another field-copy relay.
- Removed `CreateMCPServerRequest`: create and test now use the one canonical
  `MCPServerCandidate` value already emitted by contract generation.
- Added a fitness test that permits transparent aliases only for the exact five
  external JSON-RPC transport types. Also fixed the sole pre-existing
  `staticcheck` finding in schedule deletion.
- Runtime-wide compile, focused tests, dependency/alias fitness tests, `go vet`,
  `staticcheck`, `golangci-lint`, `deadcode -test`, and exact-source scans pass.
  The full architecture suite still fails only on the frontend generated
  contract drift deliberately deferred by the server-only implementation round.

### 2026-07-24 — Batch 17 completed

- Replaced split live-Run and working-tree admission with one Gate-owned
  pending-to-live lease. Terminal maintenance now retains the same tree fence
  until its synchronous work releases, closing the restore/rollback race rather
  than relying on ordering at callers.
- Added the `rememberable` approval capability to the canonical transcript and
  protocol projection. Runs rejects a remembered response for a one-off
  approval before consuming its interrupt; the desktop hides the remember
  controls whenever that capability is absent.
- Removed production-only-for-tests observer and resolver methods, and removed
  silent Gate defaults. Tests now observe real application behavior or inject
  the actual Gate through test fixtures.
- Made artifact encoding and stream presentation reject unsupported enum
  values. Known optional values remain representable; unknown values can no
  longer degrade into `completed`, `running`, `text`, or another valid-looking
  protocol state.
- Goal and Plan were retained: absent frontend use alone is not evidence that a
  supported product capability is dead code.
- Verified `go build ./...`, `go vet ./...`, `go test ./...`, focused admission/
  runs/sessions/Delivery race tests, and frontend typecheck, lint, and tests.

### 2026-07-22 — Plan created

- Recorded the green validation baseline and the audit findings.
- Chose root-cause fixes over retry, fallback, compatibility, or caller-discipline patches.
- Preserved cohesive large packages; package/file size alone is not a refactoring target.

### 2026-07-23 — Batch 16 started

- Reopened the hygiene ledger after a post-closure audit found residual Skills
  file-layout/frontmatter vocabulary and model/tool rendering inside Domain.
- Classified JSON Schema and canonical tool JSON values as retained semantic
  values, not codecs to move; classified Goal as an active product capability,
  not an unconsumed frontend placeholder.

### 2026-07-23 — Batch 16 completed

- Moved Skills file-layout, draft/archive, and frontmatter conventions into the
  prompt-source and skill-authoring adapters. Domain now retains only skill
  values, lifecycle, and validation vocabulary.
- Moved agent-document, curated-memory, Todo, and edit-guard presentation to
  their consuming adapters. Domain now returns structured values and semantic
  guard verdicts rather than prompt or tool-response wording.
- Moved Markdown, sentinel, and list-marker fact parsing into the maintenance
  adapter, while preserving Agent Memory's canonical structured-fact invariant.
  Replaced the Application session test's adapter dependency with a
  consumer-side fake.
- Added architecture guards against filesystem layout and removed Domain
  presentation helpers. Workspace/standalone build, vet, and test; focused
  race suites; `staticcheck`; `golangci-lint`; `deadcode -test ./...`; exact
  source scans; and `go test ./internal/arch` all passed.

### 2026-07-23 — Batch 10 started

- Reopened the hygiene ledger for residual abstraction leaks found after the
  earlier closure. The audit explicitly distinguishes a truly superseded
  abstraction from an otherwise supported capability that has no current
  frontend consumer.
- Started moving behavior from Bootstrap into adapters and collapsing Delivery's
  post-mutation readbacks into application-owned coherent results.

### 2026-07-23 — Batch 10 completed

- Moved Bootstrap-local persistence, checkpoint, prompt-catalog, child-session,
  and turn-cleanup behavior into their owning adapters. Goal/session mutation
  coordination is now constructed before both coordinators rather than supplied
  through a Bootstrap late-binding proxy.
- Export/import/rollback, role updates, and live subscription now expose one
  coherent application result. Delivery only decodes and projects it; it no
  longer reads a Session or live Run again after the operation it presents.
- Removed domain-owned JSON-RPC replay, masking, model-tool preview text, and
  `qN` response-key generation. These now live respectively in a neutral
  component, application read-model code, the agent executor adapter, and the
  runs application contract.
- Replaced the `RestorePlan = Snapshot` alias with an explicit durable command,
  removed application transcript façade aliases, centralized run/segment/item
  namespaces in Runs, and strengthened the Bootstrap architecture guard so no
  hidden receiver-based adapter can return.
- Kept candidates with no current frontend invocation when they still express a
  protocol or product capability; no deletion was justified solely by absent
  caller evidence.
- Full module, standalone, focused race, architecture, formatting, and removed
  seam source scans passed.

### 2026-07-22 — Batch 1 started

- Began aligning Run journal closure with the actual synchronous segment boundary.
- Removed the planned Goal-mode busy-admission retry path and added malformed-terminal coverage.

### 2026-07-22 — Batch 1 completed

- Run journal closure now follows synchronous terminal maintenance and admission release.
- Goal delegates one next-run start to the Run contract and pauses on a malformed terminal outcome.
- Focused race, vet, and architecture checks passed.

### 2026-07-22 — Batch 2 started

- Began moving cross-use-case session admission into one application-owned gate.
- Began moving scheduled-run launch construction and worker lifetime out of Delivery.

### 2026-07-23 — Batch 2 completed

- Runs and Sessions now share one constructor-injected application admission gate;
  Delivery no longer supplies a Run coordinator to Session use cases.
- Bootstrap constructs the scheduled-run launcher, the outer command owns worker
  lifetime, and Delivery only projects accepted firings to workspace events.
- A failed schedule start leaves its durable occurrence due; focused race, vet,
  persistence, and architecture checks passed.

### 2026-07-23 — Batch 3 started

- Began tracing model/provider enumeration and configuration policy from the
  Delivery handlers into the application coordinator.

### 2026-07-23 — Batch 3 completed

- Application now owns static/remote model discovery, catalog enrichment,
  Provider support and endpoint validation, redacted provider results, and
  provider probing eligibility.
- Utility and embedding role writes now enforce supported, configured, and
  embedding-capable provider policy before client construction or persistence.
- Delivery maps application values and typed policy errors only; it no longer
  imports the static model catalog. Focused race plus full build, vet, test, and
  architecture checks passed.

### 2026-07-23 — Batch 4 started

- Began auditing producer-owned dispatcher interfaces, single-use generics,
  dependency accessor bags, and wire-shaped HITL/suspension values.

### 2026-07-23 — Batch 4 completed

- Turn construction now returns a concrete dispatcher; each consumer owns a narrow
  local port. The Run registry and event journal are concrete over their only
  production payloads, and dependency accessor bags were replaced by named ports
  or the cohesive session write-set phase port.
- Approval remembrance uses the canonical `approval.Scope` vocabulary end to end.
  Agent suspension prompt and resolution JSON codecs now live in the agent adapter,
  leaving Domain and Application values typed and wire-free.
- Full build, vet, test, focused race suites, and architecture tests passed.

### 2026-07-23 — Batch 5 started

- Began encoding the completed ownership decisions as architecture fitness checks and
  validating the module both inside and outside the workspace.

### 2026-07-23 — Batch 5 completed

- Added architecture checks that keep Delivery out of schedule-worker wiring and
  static model catalog policy, keep suspension values wire-free and approval
  scope typed, and prevent the removed dispatcher interface and one-use
  lifecycle generics from returning.
- Updated the architecture and extensibility references for concrete turn control
  with consumer-owned narrow ports, and corrected the stale Goal admission note.
- `go build ./...`, `go vet ./...`, `go test ./...`, the corresponding
  `GOWORK=off` commands, and race tests for the lifecycle-critical packages all
  passed.

### 2026-07-23 — Batch 6 completed

- `application/usage.Reporter` now owns durable run-metering aggregation;
  Delivery only maps its neutral report values to `usage.*` protocol DTOs.
- `application/workspace` owns effective-cwd resolution, path confinement,
  filesystem/Git browsing, project aggregation, and instruction-document
  discovery through narrow consumer-side ports. Delivery imports no concrete
  workspace or prompt-source adapter.
- Bootstrap supplies every application coordinator and the Git capability
  snapshot; `delivery/server.New` no longer creates disabled application
  coordinators or probes the process environment.
- Batch 6's transitional `DelegationMetadata` carrier was later removed in
  Batch 7: agent/core JSON is now an opaque Bootstrap sidecar persisted in
  `agent_session_state`, never a Session-domain value.
- Official OpenTelemetry API use within Application remains intentionally
  allowed by the repository boundary policy, so no speculative telemetry port
  was introduced.

### 2026-07-23 — Batch 7 completed

- Git-state watching now belongs to the workspace adapter; the application
  resolves and deduplicates requested roots, and Delivery publishes only its
  neutral resync callback.
- The Session domain now owns only subtask lineage and audit timestamps.
  Bootstrap serializes agent runtime continuation as opaque state in the
  `agent_session_state` storage sidecar, with identity consistency checked at
  that boundary.
- `application/agentmemory` now owns memory scope, review, and atomic update
  policy. `application/codebase` resolves project roots itself. The former
  workspace coordinator is split into independently wired, focused use cases.
- Delivery now names narrow consumer-side interfaces for every application
  capability; typed workspace vocabulary replaces Git adapter type/error
  leakage. Architecture checks prohibit Delivery filesystem technology and
  agent-continuation fields in the product Session domain.

### 2026-07-23 — Batch 8 started

- Re-opened the boundary audit after finding archive restoration code in
  `delivery/server/artifact_decode.go`. The audit treats protocol mapping as
  Delivery work but treats aggregate invariants, state reconstruction, durable
  recovery, and application policy as inward concerns.
- Chose a breaking artifact schema v6 rather than retaining a v5 compatibility
  branch: v6 has dedicated durable session/run/item records and excludes live
  projections by construction.
- Recorded the additional cross-boundary findings: Delivery-side MCP secret
  policy and skill notifications, startup recovery, session activity and hook
  policy, multi-representation run input, domain JSON tags, and dead consumer
  port methods. The batch is not complete until full verification is recorded.

### 2026-07-23 — Batch 8 completed

- `application/sessions` now owns the terminal-only portable snapshot, archive
  aggregate reconstruction, tool-result binding, run-tree checks, and typed
  validation. Delivery's v6 artifact codec is a protocol-format mapper only;
  the two obsolete Delivery aggregate decoders were removed.
- The archive contract is deliberately breaking: v6 has dedicated durable
  session/run/item records, excludes live and derived state, and has no v5
  compatibility path. Shared frontend samples, TypeScript shapes, and API docs
  were updated in the same change.
- Application now owns same-origin MCP credential retention, startup recovery,
  session state precedence, hook activation, canonical run-input materialization,
  and committed skill-library refresh signals. Domain hook/process and storage
  codecs reside in their adapters; all Domain JSON field tags are gone.
- Removed unconsumed Delivery port methods (including test-only admission and
  registry probes); tests reach concrete coordinators only through explicit
  test helpers. New architecture fitness checks guard the removed seams.
- Verification passed: module and standalone build/vet/test, focused race tests
  for integrations/runs/sessions/workspace/delivery/bootstrap, frontend
  typecheck, and the RPC sample contract test.

### 2026-07-23 — Batch 12 completed

- Removed Delivery-only workspace-root and singular active-session dependency
  chains, uncalled consumer-port members, and test-only title/pagination helpers.
  Goal mode and its shared Session mutation coordination remain intact because
  they are a planned product capability, not evidence of dead code.
- Replaced adapter-written, production-unread executor event metadata with a
  sealed application event family; durable event cursors remain owned by
  `application/runs` where replay actually consumes them.
- Split schedule management from firing and worker lifecycle, so the runner is
  fully constructor-injected after Runs exists rather than late-bound through a
  mutable setter.
- Moved live role and MCP policy synchronization behind Application-owned state
  types; client fallback, embedding resolution, and compactor live-state mapping
  now reside in driven adapters. Bootstrap only loads startup role values and
  wires collaborators.
- Added architecture checks against renewed Bootstrap live state, atomic
  constructor leakage, post-construction schedule wiring, and the removed
  Delivery port members. Module and standalone test/vet/build, focused race
  suites, `staticcheck`, and `golangci-lint` passed.

### 2026-07-23 — Batch 13 completed

- Goal terminal reporting now enters through `application/goals.State`; the
  model-facing tool has neither a Goal store nor CAS/domain-mutation code.
  Lease validation, status validation, revision advance, and conflict handling
  are all application-owned.
- Knowledge, Todo, Goal, and Schedule persistence contracts moved from broad
  producer/domain definitions to their real consumers. Schedule explicitly
  separates management, run-now, and worker capability slices; Bootstrap alone
  composes their union for one SQLite implementation.
- Approval now exposes a concrete `RuntimePolicy`; its independent consumers
  define their own management, gate, and plan-exit views. The session cleanup
  capability is likewise local to its consumer.
- Added a fitness rule preventing the removed Goal/Todo/Knowledge/Schedule/
  Approval producer-owned interface names from returning. Goal mode remains
  wired and tested as a product capability, not treated as dead code.
- Workspace and standalone build/vet/test, focused race suites, architecture
  tests, `staticcheck`, `golangci-lint`, formatting, and source scans passed.

### 2026-07-23 — Batch 14 completed

- Moved Feedback, MCP, and diagnostic-tool consumer ports to their Application
  or Bootstrap owners. Domain now retains their values and invariants; SQLite
  stores remain concrete driven adapters instead of declaring upstream ports.
- Deleted Agent Memory's unused aggregate store and Codebase's producer-owned
  index surface. Extraction, search, review, Application codebase use cases,
  and the tool adapter each name only the methods they consume; the uncalled
  `EnsureIndexed` API and its private relay are gone.
- Expanded the architecture fitness rule to prevent all removed producer-owned
  port names from returning. The audit reconfirmed Goal is frontend-integrated;
  the in-process transport is explicitly retained for future CLI/TUI use; and
  `approvaltest` is test-only.
- Workspace and standalone build/vet/test, focused race suites, architecture
  tests, `staticcheck`, `golangci-lint`, formatting, exact-symbol scans, and
  dead-code classification passed.

### 2026-07-23 — Batch 15 started

- Reopened the hygiene ledger for a protocol capability inconsistency, two
  static-config getter interfaces, test-only dead methods, and the residual
  recipe/skill YAML codec dependencies in Domain.
- Chose one coherent ownership rule: disabled protocol capabilities are never
  projected as UI-shaped success values; startup defaults are value snapshots;
  file-format parsing and rendering belong to the adapters that read or write
  those files.

### 2026-07-23 — Batch 15 completed

- `memory.list` now follows the same capability-gated error contract as
  `memory.get` and `memory.update`; an unwired store cannot produce a synthetic
  empty collection while discovery advertises `features.memory=false`.
- Removed the six unreachable `sqliteOpeningStores` fixture methods. The full
  test-aware dead-code scan is now empty.
- Session read models and usage aggregation receive the immutable startup
  default values directly. The former single-implementation getter interfaces
  and their nil collaborator path are gone.
- Recipe Markdown/YAML parsing now lives in `adapter/promptsource`; SKILL.md
  frontmatter encoding now lives in `infra/skillauthoring`. Domain retains
  recipe/skill values, validation, provenance meaning, and lifecycle rules.
- The architecture suite now rejects YAML codec imports in Domain/Application.
  Workspace and standalone build/vet/test, focused race tests, static analysis,
  exact-symbol scans, and formatting passed.

## 8. Completion definition

This plan is complete only when every batch is `Completed`, every acceptance criterion has evidence,
the progress log describes any revised decision, and both workspace and standalone verification are
green. A passing import-direction test alone is not sufficient.
