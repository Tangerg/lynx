# Runtime Architecture Hygiene Plan

> Status: active  
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

Status: **pending**

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

## 6. Progress

| Batch | Status | Started | Completed | Evidence |
|---|---|---|---|---|
| 1. Run boundary semantics | Completed | 2026-07-22 | 2026-07-22 | `go test -race ./internal/application/runs ./internal/application/goals`; `go vet` for both packages; `go test ./internal/arch`. |
| 2. Application collaboration ownership | Completed | 2026-07-22 | 2026-07-23 | `go test -race` for admission, runs, sessions, schedules, delivery/server, bootstrap; focused `go vet`; `go test ./internal/arch`. |
| 3. Model/provider policy ownership | Completed | 2026-07-23 | 2026-07-23 | `go test -race ./internal/application/models ./internal/delivery/server ./internal/bootstrap`; `go vet ./...`; `go test ./...`; `go test ./internal/arch`. |
| 4. Abstraction and boundary cleanup | Completed | 2026-07-23 | 2026-07-23 | Full build, vet, test, focused race suites, and architecture tests passed after dispatcher, registry, HITL, and suspension cleanup. |
| 5. Fitness tests and final verification | Pending | — | — | — |

Allowed status values: `Pending`, `In progress`, `Completed`, `Blocked`, `Revised`.

## 7. Progress log

### 2026-07-22 — Plan created

- Recorded the green validation baseline and the audit findings.
- Chose root-cause fixes over retry, fallback, compatibility, or caller-discipline patches.
- Preserved cohesive large packages; package/file size alone is not a refactoring target.

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

## 8. Completion definition

This plan is complete only when every batch is `Completed`, every acceptance criterion has evidence,
the progress log describes any revised decision, and both workspace and standalone verification are
green. A passing import-direction test alone is not sufficient.
