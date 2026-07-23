# Runtime Architecture Hygiene Plan

> Status: completed
> Started: 2026-07-22  
> Scope: `app/runtime`  
> Architecture baseline: [EXECUTION_CENTERED_ARCHITECTURE.md](EXECUTION_CENTERED_ARCHITECTURE.md)

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

Allowed status values: `Pending`, `In progress`, `Completed`, `Blocked`, `Revised`.

## 7. Progress log

### 2026-07-22 — Plan created

- Recorded the green validation baseline and the audit findings.
- Chose root-cause fixes over retry, fallback, compatibility, or caller-discipline patches.
- Preserved cohesive large packages; package/file size alone is not a refactoring target.

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
