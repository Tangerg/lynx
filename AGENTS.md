# AGENTS.md — repository context for the scope monorepo

> A Go monorepo of independently versioned sub-modules. This file holds only the
> rules shared across modules — the macro shape, never the specifics. Concrete
> cases, symbols, and file names live in the code, in git, and in each module's
> own `ARCHITECTURE.md`, because they change as the code evolves. The *why*
> behind the design is in [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md); the
> yardstick for refactoring is in [`REFACTORING.md`](REFACTORING.md).

---

## First law — never take on debt for convenience

> **Highest priority. It overrides everything else in this file.**

The project is in rapid development. There is no legacy debt and no external
compatibility burden: the store schema, the exposed API (exported functions,
types, fields, protocol wire shape), and the naming are all still open. Because
of that:

- ❌ **Never leave debt to "touch fewer places", "do less work now", "avoid a
  migration", or "hit a date"** — no naming bent around a third-party library or
  an old name, no field kept for compatibility, no speculative shim, no
  "clean it up later" TODO.
- ✅ **When a design turns out to be wrong, fix it at the source.** Do not stack
  a patch on a wrong design. **Fixing it now is the cheapest it will ever be.**
- Naming and shape follow from first principles. Prior art contributes ideas,
  never a naming anchor — if a name matches the industry's, that is only because
  it was the best name under independent evaluation, not for compatibility or to
  save a migration.
- **The only admissible debt is "the design is not thought through yet".**
  "Knowing the better shape and not changing it" is never admissible.
- This law does **not** waive the rule that a breaking public API change is
  discussed first: **ask, but default to fixing it right rather than settling.**

---

## Second law — fix the root cause, never the symptom

> **Equal in priority to the first law.** The first law is about not incurring
> debt; the second is about repairing all the way down.

Fix every bug at its root cause and in the correct layer. Never patch at the
symptom, and never reach for a hack. A workaround that makes the symptom
disappear while the cause remains is not a fix — it hides debt behind the
appearance of a fix, and the next adjacent bug will surface from the same cause.

- ❌ **Never treat the symptom**: an `if` at the point of failure, a workaround
  on the consumer side while the source stays broken, "added a log line" or
  "stopped swallowing the error" as a fix, reactive coercion or retry or
  fallback that masks an upstream bad state, or leaving an invariant to "the
  caller remembers not to get it wrong".
- ✅ **Fix the cause**: find the layer the cause lives in and fix it there. The
  cause is usually lower or further in — an SDK primitive, an interface
  contract, a store schema — so a real fix often has to move a public API or a
  schema. That is exactly what the first law permits. **Ask first, per the
  breaking-change rule, but default to the real fix rather than settling in an
  outer layer.**
- **The test, in one sentence**: ask "is the cause gone, or has only this symptom
  stopped appearing?" If only the symptom is gone, it is a patch — send it back.

---

## Position and map

`scope` is Go infrastructure for AI agents, RAG, and LLM integration. The
repository root is a workspace and publishes no empty root module.

`core` is the base module with a strict dependency direction and a stable
protocol, partitioned by meaning into packages. It owns the multimodal
protocols, the minimal SPIs, the clients, the Tool contract and registry, the
tokenizer contract, the history contract, and the in-memory reference
implementations.

`agent`, `rag`, `etl`, `eval`, `tools`, and `skills` are capability modules.
`a2a` and `mcp` are protocol integrations. `otel` is the observability
integration.

`models/<provider>`, `vectorstores/<provider>`, and `historystores/<provider>`
are independent leaf modules because their third-party dependencies and release
cadences differ. `tools/web/<provider>` is only a package split, because those
providers share one lightweight dependency set and lifecycle.

The dependency hierarchy expresses import direction; it does not replace these
module roles. A complete application belongs to the separate Flame repository:
this repository contains no `app` module and does not fold an application layer
into its architecture. Each module family's own invariants live in its
`ARCHITECTURE.md`.

---

## Shared hard rules (violating one is a regression)

- **One project identity.** The brand is `Scope`; the repository and Go module
  path is `github.com/Tangerg/scope`. No old brand, old module path, dual path,
  brand alias, or compatibility `replace` survives. An external protocol or
  provider brand appears only when stating an objective integration fact, never
  as the naming anchor for a Scope concept.
- **One Go version.** Every module's `go.mod` stays in sync, and code uses the
  modern standard library of that version — `iter.Seq2`, `slices.*`, `maps.*`,
  typed atomics.
- **Do not hand-roll general capability.** The standard library comes first;
  where it falls short, an evaluated and well-maintained third-party library is
  allowed, including in Core. The condition for adopting one is that it removes
  parsing, edge cases, and maintenance surface on net — not that it wraps one
  function in a new dependency — and no extra layer is added to hide a mature
  library. The gates constrain dependency direction, public API leakage, and
  known high-risk categories; they do not enumerate allowed library names. A
  local wrapper, a copied implementation, or "Core is special" is not a reason
  to reinvent a wheel. Domain semantics and boundary policy still belong to the
  domain types.
- **Domain behavior belongs to the domain owner.** Domain entities and value
  objects own their invariants, validity, derived values, and pure state
  transitions; they must not be scattered into procedural service or helper
  scripts. Configs, request and response DTOs, wire values, and fact records
  stay data models — no method or empty service object invented to make them
  look rich.
- **One capability, one entry.** Each capability keeps exactly one canonical
  public representation and one primary call path. A free function, a method, a
  façade, an alias, a builder, and a compatibility wrapper must never coexist as
  synonyms. A convenience API is justified only when it hides a real lifetime or
  type-erasure boundary and still delegates to the single owner.
- **No magic.** A stable vocabulary uses a semantic named string value object or
  constant, validated by its owner; protocol numbers, defaults, timeouts,
  versions, and attribute keys must be named. An anonymous `map[string]any` may
  only stop at a genuinely dynamic wire or third-party SDK boundary; once inside
  the domain it becomes a typed model immediately, and it is never internal
  config, domain state, or a cross-layer argument bag.
- **Modules and packages have different jobs.** A module expresses an
  independent dependency set, release cadence, and version boundary; a package
  expresses responsibility. The same dependencies and lifecycle do not get split
  into modules for symmetry, and different heavy SDKs are not forced into one
  aggregate module. The dependency set is how a boundary is judged, not a budget
  limiting how many mature libraries may be used.
- **Singular and plural follow meaning.** An importable single-capability
  package prefers a singular or uncountable domain name — `tool`, `history`,
  `web`. A plural is for a namespace that only carries a set of sibling
  implementations and forms no package itself — `models`, `vectorstores`,
  `tokenizers`. A protocol or industry proper name keeps its own form (Agent
  Skills) rather than being mechanically singularized.
- **Depend on interfaces, not concrete types**, and **define the interface in the
  consumer**, not in the provider. Before taking a whole `*Engine`, `*Client`, or
  `*Store`, stop and ask whether only a few methods are actually used.
- **Split interfaces (ISP).** An interface holds only the methods its caller
  actually uses. A fat interface is split into composable sub-interfaces: the
  assembly point unions them, and each consumer depends on its own slice.
- **Errors.** Error text must be readable without the surrounding log context: it
  states at minimum the failing operation, the object identity or the key
  boundary, and the cause. Keep it lowercase and without a trailing period.
  `errors.New` is preferred over `fmt.Errorf("constant")`; `fmt.Errorf` is for
  genuine formatting, and a wrapped error always uses `%w` so `errors.Is/As`
  keep working. A stable classification is owned by a sentinel or typed error,
  never by matching a string.
- **No Java flavor.** No empty-suffix type names (`Impl`, `Service`, `Manager`,
  `Helper`, `Handler`), no generic file names (`impl.go`), no `GetX`/`SetX`
  getters, no builder chains. A file name describes its contents; a struct name
  describes its essence.
- **Modern Go.** Typed atomics and `sync.Map` (write-rare) over a hand-rolled
  wrapper; `slices.*` and `maps.*` over a hand-written loop; `iter.Seq2` over a
  channel used as a stream.
- **Naming mechanics.** A method receiver is the lowercase initial of its type,
  with no exceptions. A parameter never shadows an imported package name. Both
  are enforced by `dev/repoarch`.
- **Observability is the three OTel signals, sunk into `log/slog`
  (vendor-neutral).** Observation is traces (spans) plus metrics (instruments)
  plus logs. Applications, integrations, and the standalone `otel` module use the
  official OTel API directly and invent no tracer or meter abstraction. **Core
  never imports OTel**: the `otel` wrappers and decorators add instrumentation at
  the protocol call boundary. The composition root binds the exporters and the
  W3C propagator once at startup.
  - **Do not sprinkle `slog` through domain code.** If an event deserves
    observation, open a span with attributes or record a metric in the layer that
    owns the runtime semantics, rather than adding a log line. Errors go through
    `span.RecordError` and `SetStatus`. Core only propagates `context.Context`
    and owns no observation policy.
  - **Logs remain a first-class OTel signal** — slog reaches the LoggerProvider
    through a bridge. The point is *replaceability*: swapping in an OTLP exporter
    in production sends spans, metrics, and logs to a backend with zero change to
    domain code. It is not an invitation to log everywhere.
  - **Attribute keys carry no brand.** Prefer semconv; otherwise a bare domain
    name with no project prefix. The instrumentation scope name keeps the library
    path — that is a library identifier, not data.
  - **End to end.** The trace ID is generated at the entry point, and a detached
    background goroutine keeps its span with `context.WithoutCancel`, never
    `context.Background()`.
  - **Dependency boundary.** The official OTel API is itself the vendor-neutral
    layer. "Bolted on" means not changing the import direction — it does not mean
    building another `core/observation`. A capability module must not import
    `otel` or the official OTel API; the `otel` integration depends on them in
    reverse and decorates them. `a2a` and `mcp` are protocol integrations and may
    use the official OTel API directly at the protocol call boundary they own;
    that exemption does not spread to the capabilities they adapt.
  - Module boundaries are in [`otel/ARCHITECTURE.md`](otel/ARCHITECTURE.md).
- **Design principles** (high cohesion and low coupling, SOLID, DRY, KISS,
  YAGNI) are in the section below. They are judgment criteria, not slogans.
- **The public API is adjustable, but not unilaterally.** During development
  there is no legacy compatibility and no migration: when a schema, an exported
  type, or a signature changes, it is replaced outright and no comment says
  "Legacy …". **But any breaking public API change — including one signature,
  one deleted type, one changed field — is discussed with the user first**, with
  the scope, the blast radius, and the alternatives stated, and waits for
  confirmation. This applies to every sub-module.
- **Ask before adding a document.** Each module already has `README.md`,
  `ARCHITECTURE.md`, and `doc.go`; nothing else is written by default.

## Shared negative invariants (directions already known to be wrong)

- ❌ A retry layer or a transient/non-transient classification — the SDKs already
  retry.
- ❌ A second converter chain for structured output — the existing parser family
  covers it, and reasoning is first-class.
- ❌ An immutable constructor returning a pointer where it should return a value.
- ❌ A hand-written `fmt.Errorf("xxx is nil")` — use `errors.New`, and always
  wrap with `%w`.
- ❌ A new module depending on a whole `*Engine`, `*Client`, or `*Store` — define
  the narrow interface you need in the consuming package.
- ❌ A fat interface holding every method — split it per consumer.
- ❌ A duplicated public type, typically an enum declared twice — keep one and
  import it.
- ❌ A stub interface or a speculative placeholder "for later" — define it when
  it is actually needed.
- ❌ OAuth or token refresh for an LLM provider — the user supplies a key, and a
  401 is a prompt to re-enter it.

## Design principles

The hard yardstick for "should this code be written this way" and "should this
PR merge". **The *why* behind it — a thin kernel, three shapes of variation, a
narrow waist, one extension mechanism, base capability as a library first, and
an explicit lifecycle framework — is in
[`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md); the refactoring yardstick is in
[`REFACTORING.md`](REFACTORING.md).**

- **High cohesion, low coupling.** Everything inside a package or struct serves
  one purpose (cohesion); a cross-package dependency goes through the smallest
  interface rather than a concrete type (coupling). When the two conflict,
  prefer more packages and more interfaces over one package spanning several
  domains or one concrete type becoming a cross-package hub.
- **SRP.** A struct or function has exactly one reason to change. Signals: many
  fields plus many methods, a long function, a large file. Decide first whether
  it is inherent complexity — if it is, express it with field-zone comments
  rather than a forced split.
- **OCP.** A new capability arrives as a new type, not as an edit to an old
  dispatch loop. In Go that is an interface plus a type assertion, or generic
  type dispatch — never inheritance.
- **LSP.** Implementing an interface means satisfying its semantics and its
  **behavioral contract**, not just its signatures. An implementation cannot
  honor some methods or parameters and not others. Guarantee it with a
  compile-time assertion and a stub.
- **ISP.** An interface holds only the methods its caller uses. **Crossing a
  boundary versus internal implementation is the key distinction**: a
  consumer-side narrow interface is for multiple implementations, testability,
  and module boundaries. Inside a library's own cohesive subsystem, a
  single-implementation dependency uses the concrete type — no interface
  extracted mechanically for tidiness. Narrow interfaces are reserved for public
  SPIs, application consumption boundaries, and real substitution points.
- **DIP.** High-level code depends on abstractions, not on low-level concrete
  types, and the interface is defined by the consumer, not the provider.
- **DRY.** Consider an abstraction only when the same logic, type, or string
  appears **three or more times** — below that, the abstraction is worse than
  the repetition. DRY exists to remove the fragility of "change one place, change
  N"; it is not character elimination. Similar code that will evolve
  independently for different reasons **must not** be DRYed — false DRY costs
  more than repetition.
- **KISS.** Simple beats clever, because maintenance is 90% of the time. Signals:
  nested generics more than two deep, reflection, `interface{}`, a long tail of
  type switches, deeply nested closures — usually "can be written" rather than
  "should be".
- **YAGNI.** No abstraction, hook, config, or interface for a requirement that
  does not exist. Signals: a speculative placeholder, a single-implementation
  interface with no plan for a second, a default field that never changes —
  delete or inline. But YAGNI is not "never plan ahead": **an extension that has
  already happened several times is foresight, not speculation.**
- **Go-specific.** Accept interfaces, return structs. Make zero values useful.
  Composition over inheritance (embedding is has-a; use it sparingly). Smaller
  interfaces are better — a one-method interface is often the most useful. The
  interface is defined by the consumer.

**When principles conflict**: DRY versus low coupling — if extracting a helper
introduces an unwanted cross-package dependency, prefer the repetition. ISP
versus KISS — count the actual callers: one caller using every method means do
not split; several callers each using a slice means do. YAGNI versus OCP — has
the extension actually happened before? If yes, keep the extension point; if it
is only a guess, delete it. **The baseline judgment**: always prefer "this is
fine as written, change it when it needs to extend" over "add a layer now in
case it is useful later" — the future can be refactored, over-abstraction is
hard to reverse.

## Refactoring

**Refactoring is a rhythm, not an option.** It comes in two sizes:

- **Small (every few feature rounds).** Focus on recently changed files: scan for
  overlong files, local duplication, dead comments, naming drift, and whether a
  newly exported API breaks an existing abstraction. The output is small — small
  net change, few files touched. With no breaking public API, finish it, run
  everything green, and commit.
- **Large (every ten to twenty feature rounds).** Scan across modules for dead
  code, oversized files that need an SRP split, cross-package duplication, god
  structs, concrete types exposed across packages that should be narrowed, and
  package splits or merges. The output is a multi-batch plan; after the user
  confirms, commit batch by batch with everything green between batches.

**The purpose**: the small size prevents local entropy (no single file gets out
of control); the large size prevents architectural entropy (the whole does not
become unwieldy). **The trigger signals, the Fowler-style checklist (dead code,
guard clauses, lookup tables, interface narrowing, performance scans), and the
full yardstick for naming, comments, pointer versus value, nil guards, locality,
and rhythm discipline are in [`REFACTORING.md`](REFACTORING.md)** — read it
before refactoring.

## Comment discipline: do not write comments lightly

Make the code explain itself through **naming, structure, and abstraction**
first. Before writing a comment, ask whether a rename, an extracted function, or
a restructure would make it unnecessary. A comment is the **last resort** after
expressiveness runs out. Write one only in these cases, where it carries
information the code cannot:

1. **A key public contract (godoc).** A key domain type, an interface, and a
   non-obvious implementation state their invariants, ownership, lifetime, error
   semantics, side effects, and concurrency constraints. An ordinary
   constructor, `Validate`, or `MarshalJSON` gets no template comment restating
   its name. If a tool demands a comment, express the real rule through a
   package-level exemption or by adjusting the gate — never by manufacturing
   information-free noise.
2. **A special convention.** A business rule, a historical reason, a
   compatibility requirement, an external system constraint (a protocol clause,
   a third-party SDK quirk) — present in the context, not in the code.
3. **A special algorithm.** The idea, the edge conditions, the complexity, a
   non-obvious optimization.
4. **A counter-intuitive implementation.** Why the more common, simpler form
   **cannot** be used — so the next person does not helpfully change it back.
5. **A concurrency, transaction, or safety constraint.** Goroutine ownership and
   lifetime, lock ordering, who closes a channel, context cancellation
   semantics, a trust boundary — violating one does not fail to compile, it
   fails in production.

- **The test, in one sentence**: a comment states *why* and *what constrains*,
  never *what* and *how* — a comment that restates the code necessarily rots
  into a lie when the code changes.
- Changing code means changing its comment; delete rather than leave a stale one.
  A comment addresses the **next reader**, not a reviewer — never "fixes bug X
  here" or "adjusted per review".

## Working agreements

- **Reply in Chinese** (user preference); code and comments stay English.
- Before a breaking or structural change, state the scope, the blast radius, and
  the alternatives, and wait for confirmation. One independently revertable
  commit per batch.
- `go build && go vet && go test ./...` must be green before a commit. A commit
  message states the *why*, not only the *what*. Push after committing by
  default.
- Commit trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
