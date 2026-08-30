# REFACTORING.md — the Scope refactoring yardstick

> The cross-module **refactoring yardstick and rhythm**. The *why* behind the
> design is in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md); the
> repository-wide laws are in [`AGENTS.md`](AGENTS.md). This file is the
> generalized checklist for "**what** to change, **how**, and at **what
> rhythm**" while refactoring, and it applies to **every sub-module**. It is
> stated entirely in the abstract, at the macro level, bound to no concrete
> implementation — apply the judgment when you meet the matching situation.

---

## 0. Where the yardstick comes from

- Measured against the **Go SDK design documents and the Go standard library**:
  minimal, idiomatic, *accept interfaces / return structs*, small interfaces, a
  `Config` value at construction, a useful zero value. Construction config for
  Scope's own abstractions in Core, A2A, and elsewhere passes a `Config` struct
  rather than `func(*T)` functional options. A lower-level provider adapter may
  keep its SDK's functional options, but that mechanism is never promoted into
  the shared cross-provider mental model.
- **Refinement is not a rewrite.** Surgical, reversible, and **fixed at the
  source** — never a patch stacked on a wrong design. (Fix the cause, not the
  symptom; see the second law in [`AGENTS.md`](AGENTS.md).) Prior art
  contributes ideas, **never a naming anchor**.
- **The only admissible debt is "the design is not thought through yet".**
  "Knowing the better shape and not changing it" is never admissible.

## 1. Naming: the name must match the thing

- A name must match **the data it carries and the work it does**. If a type name
  does not match its content, or a method name does not match its behavior,
  change it.
- A method name uses a plain verb describing this call's behavior or the fact it
  returns. A past participle such as `Merged` or `Processed` is reserved for
  genuinely persisted domain state and never poses as an operation.
- The field name equals the serialization tag. When they disagree, **rename the
  field rather than settling for the tag.**
- Eliminate **package-name stutter** (`pkg.Pkg…`).
- **A file name describes its contents.** A generic or Java-flavored file name
  (`interface.go`, `impl.go`, `util.go`, `helper.go`) becomes a name describing
  what is in it. Renaming a file is an in-package operation and changes no API.
- Refer to the de facto vocabulary of the domain, but adopt a term only when it
  is the best name under independent evaluation.

## 2. Comments

- Explain **why**, never *what* — the code states the what.
- Delete stale, migration-era, and misleading comments. **After a rename or a
  refactor, clean up every reference**, leaving no stale pointer behind.

## 3. Pointer versus value

- **Necessarily present, small, and read-only → pass by value.** A meaningless
  nil state plus one dereference is pure overhead.
- **Genuinely optional — nil is a meaningful state the code branches on → pass a
  pointer.**
- Values and pointers coexisting in one signature is a **principled distinction**
  when each satisfies the reason above, not an inconsistency.
- **Never store derived state a method can compute on demand.** Delete the
  redundant cache field and compute it.
- The exception: some constructors return a **value** to guarantee immutability —
  do not change one to return a pointer for uniformity.

## 4. Nil guards: pointer-receiver hygiene

- A pointer-receiver method **guards nil at the top**, so a caller does not have
  to check `!= nil` first.
- **Only on read accessors where nil is genuinely reachable**, returning a zero
  value or a sentinel.
- **Never on** a mutator, a service-style behavior method, or an internal helper
  — there, a nil receiver is a **construction bug** and must surface rather than
  silently no-op.
- **Do not duplicate a guard** where the method is already nil-safe: it returns a
  constant, never touches the receiver, delegates through something nil-safe, or
  already has one.

## 5. Free function versus method: controlling package scope

- A package-private function belongs as a **method** on a concrete type whenever
  it is **conceptually that type's behavior or policy**. The test is **not**
  "does it read the type's fields" — that is too narrow — but "is this something
  that type should do". Reading state counts, of course, and so does a
  **stateless policy, classification, or production**. Moving it out of package
  scope costs nothing: the compiler still inlines it.
- **Keep it a free function** where making it a method would manufacture a fake
  object:
  - `newXxx` and `parseXxx` **constructors and factories** — they produce the
    type, they are not its behavior;
  - a **pure assembly or builder shared across several types** — no single
    owner;
  - one operating on a slice or a sealed interface (nothing to hang it on,
    analogous to `slices.*`), or a helper over a type from another package (Go
    cannot add a method to an external type).
- **Hang it on the right owner, do not force it.** If a behavior describes the
  type of **its parameter**, it belongs to **that** type, not to whichever caller
  happened to be nearby. Attaching it to the wrong object is worse than leaving a
  free function. If the type cannot be changed because it is in another package,
  leave the free function.

### 5.1 The rich domain model — the semantic side of §5

§5 governs mechanical relocation (free function → method); this governs which
type the logic belongs to. Same principle, semantic side; they work together.

- **A domain entity or value object carries its own behavior.** Its
  **invariants, derived values, state transitions, and validation** hang on the
  type rather than scattering into service functions, SQL strings, or wire
  conversions. Anemic means a data bag with the logic living elsewhere; when you
  find that, pull the logic back onto the type — where it can be unit-tested as
  a pure function with no service and no database.
- **Data that is genuinely data is not anemic** — do not force methods onto it:
  configs, request and response DTOs, wire protocol types, plain parameters, and
  operation-result records. Zero methods is correct for them.
- **The key boundary: only move up logic with no I/O.** Derivations, invariants,
  and pure state transitions come back to the entity. But **a state write that is
  the adapter's atomic SQL** — a counter increment, a single-field UPDATE —
  **stays in the adapter**: moving it onto the entity degrades it into a
  load-modify-store read-modify-write race, which is both slower and unsafe. The
  test: "does this logic need I/O?" Yes → adapter; no → entity.
- **Library versus application** (as with ISP): an application's domain entities
  lean rich, but **do not stack a DDD layer for a single backend** — a
  repository, an application service, an explicit aggregate root, a
  domain-events framework. For one team and one backend that is pure ceremony
  (YAGNI). Rich means pulling the rules back onto the type, not adding a layer on
  top.

### 5.2 One exit per capability

- Find the capability's real owner first, then decide its single public entry. A
  method and a free function as synonyms, an old name coexisting with a new one,
  `New` coexisting with a builder or functional options, and a stream and a
  once-shot each implemented separately are all duplicate-exit signals.
- A streaming capability is preferred as the single underlying source of truth.
  The non-streaming form is allowed only to consume that stream fully and return
  the aggregate; it must not maintain a second parsing, error, or lifecycle path.
- When deleting an old exit, migrate every real consumer and example in the same
  batch. At this stage no alias, deprecated forwarder, or compatibility shim is
  kept.

## 6. Guard clauses and cyclomatic complexity

- `if cond { long body }` followed by trailing cleanup becomes
  `if !cond { return }` plus a flat body. Merge nested conditions, and extract a
  cohesive branch into a helper.
- But a streaming iterator's `func → func → for → if !yield { return }` is
  **structural nesting, not logical complexity** — leave it alone.

## 7. Modern Go, per the module's Go version

- Replace older forms with the current version's features: `any`, `min`/`max`,
  `slices.*` and `maps.*`, `iter.Seq2`, `range int` and `range slice`,
  `time.Since`, `omitzero` (rather than an ineffective `omitempty`), typed
  atomics, and `errors.AsType` (**only when the target is an error type**).
- Take current guidance from the use-modern-go skill. **Clean up only real
  stragglers, never change for the sake of changing** — mature code is usually
  already modern.

## 8. Organization and locality

- **Related code sits together, and shared code sinks to a shared place**, with
  reading experience as the goal.
- A **god file** — over the line threshold and mixing several concerns — splits
  by responsibility into **several files in the same package**. That is a
  purely in-package move: no API change, no import change.
- **A large but cohesive, single-responsibility file is not split** — a parser, a
  lexer, a tightly coupled builder family. Splitting one breaks the cohesion.

## 9. Hard Go idiom rules

- `errors.New` is preferred over `fmt.Errorf("constant")`; a wrapped error always
  uses `%w`, including on a panic path.
- A local result accumulator uses a `nil` slice rather than `make([]T, 0)`. An
  established house style of always-non-nil fields is kept.
- A constructor **returns nil on error**, never a half-built value plus an error.
- **A real bug found mid-refactor is fixed** — in its own commit, separate from
  the refactor.
- No speculative placeholder ("wire it later", a stub interface). Dead code is
  deleted immediately.

### 9.1 Magic values and dynamic data

- A finite stable vocabulary uses a named string value object that owns its own
  validity, parsing, string form, and codec. Only bit masks, counts, sequence
  numbers, and in-process state-machine discriminants stay numeric.
- Repetition is not the only reason to extract a constant: a value that appears
  once but carries a protocol, a version, a default policy, a time budget, or an
  observation attribute must also be named, because its meaning needs an owner.
- `map[string]any` belongs only at a genuinely open-world boundary such as JSON,
  YAML, or an SDK. The domain layer, the configuration layer, and cross-package
  calls convert to a named struct or value object as early as possible.

### 9.2 Replacing hand-rolled code with a third-party capability

- Before replacing, prove the library's maturity, maintenance activity, boundary
  behavior, and dependency cost. After replacing, **delete** the local parser,
  formatter, or codec and its duplicate tests — never hide the old
  implementation behind an adapter.
- A third-party type must not leak unintentionally through Scope's stable domain
  API. If it is itself the de facto type of an industry protocol it may be used
  directly; otherwise convert once at an adapter boundary.
- Do not wrap every standard-library or third-party function for "consistent
  style". Keep a narrow adapter only where it must carry a Scope policy, an error
  boundary, or a lifecycle.

## 10. Rhythm and discipline

1. **Audit deeply first** — grep, explore, read the files — rather than editing
   straight away.
2. **Classify** the findings (naming, coupling, cohesion, SOLID, DRY, modern Go,
   pointer versus value, nil, organization) and order them by impact.
3. **Propose the batches**, with a "change versus leave" trade-off for each item.
4. **Confirm a breaking or structural change first**: state the scope, the
   **blast radius** (every cross-module consumer), and the alternatives, and wait
   for a decision.
5. **One independently revertable commit per batch**, with
   `go build && go vet && go test ./...` **green between batches**. Push after
   committing.
6. The commit message states the **why**, including what the audit found and why
   anything was skipped.
7. **Admit a false positive.** If a closer look shows the finding was wrong, skip
   it and record the reason — that is normal, not a failure.

> The trigger signals, the Fowler-style checklist (dead code, guard clauses,
> lookup tables, interface narrowing, performance scans), and the small-versus-
> large refactoring rhythm are in the "Refactoring" section of
> [`AGENTS.md`](AGENTS.md).
