# DESIGN_PHILOSOPHY.md — the *why* behind Scope's design

> Scope's design documentation has three layers, each answering one question:
>
> - [`AGENTS.md`](AGENTS.md): the **quick red lines** — laws, negative
>   invariants, trigger signals. "**May** I write it this way?"
> - [`REFACTORING.md`](REFACTORING.md): the **refactoring yardstick** — naming,
>   comments, pointer versus value, nil guards, free function versus method,
>   guard clauses, locality, rhythm. "**What** do I change and **how**?"
> - **This document**: the **organizing philosophy behind those red lines**.
>   "**Should** it be designed this way, and why?"
>
> Before designing a new capability, a new package, or a public API change, run
> it through the framework here. When actually refactoring, use
> [`REFACTORING.md`](REFACTORING.md).
>
> This document states principles only and binds to no concrete implementation —
> concrete cases change as the code evolves and live in the code and in git. This
> philosophy is not one person's taste: it has **two independent external
> corroborations** — convergent design with **embabel-agent**, an older GOAP
> agent framework, and a point-by-point match with the trade-offs in the **MCP Go
> SDK design document**, co-written by the Go core team and Anthropic.

---

## 0. The whole thing, in one line

> **A thin, indivisible kernel + every capability reducing to that kernel + no
> capability rebuilding its own infrastructure.**

The MCP Go SDK states it as the last item on its requirements list: "the SDK
should be **minimal**. However, it should admit **extensibility using simple
interfaces, middleware, or hooks**." A minimal kernel with simple extension
points, not a new machine per capability.

---

## 1. Three shapes of variation, and the litmus test

When a new capability is added, it is necessarily a re-expression of the core,
not a new machine. Three shapes, in priority order:

| Shape | What varies | Pattern |
|---|---|---|
| **① Parameterize** | One parameter or policy | Strategy |
| **② Compose** | Existing primitives assembled, compiling back to the core | Composition |
| **③ Decorate** | An existing thing wrapped | Decorator |

**The litmus test, asked in order when designing a capability:**

1. Can it be done by **turning one knob**? → ① (best)
2. No knob, but can it be **assembled from existing primitives**? → ② (write a
   shim that compiles back to the core)
3. Neither, and it is really just **wrapping something**? → ③
4. **None of the above fits, and it needs a whole new runtime, scheduler, or
   state carrier?** → ⚠️ **This is a design-error signal.** Stop and ask whether
   this actually belongs in the kernel, rather than starting a second one.

> This is "composition over inheritance", pushed to its Go extreme:
> **composition over inheritance, base capability as a library first; build an
> explicit framework only when a lifecycle genuinely must be unified, and prefer
> static over dynamic** (Go has no inheritance, no DI container, and uses
> generics rather than reflection). A framework is justified when it really owns
> the main loop, the state transitions, and the recovery invariants — not because
> something needs a bigger configuration entry.

---

## 2. Package design

### 2.1 Internally: a strict DAG, no reverse dependency

- A cross-package dependency must form a one-way acyclic graph. **The interface
  is defined by the consumer**: the consuming package declares a narrow
  interface, the consumed concrete type satisfies it implicitly, and nothing
  exports "an interface for you to use".
- **Machine-verifiable**: export the internal dependency edges, and any edge
  pointing upward is a regression.
- **The composition root assembles the concrete implementations.** The execution
  kernel depends on abstractions only, and concrete implementations are injected
  at the composition root — so adding or removing one has zero effect on the
  kernel, and every implementation is treated identically.

### 2.2 On the user-facing side: one owner and one entry per meaning

- A user imports the package that owns the meaning directly. The same capability
  is never re-exposed through a root façade, an alias, or a thin forwarder —
  that would create two public contracts with two documentation sets, two type
  identities, and two evolution rhythms.
- Discoverability is solved by package documentation, runnable examples, a
  provider catalog, and consistent naming — never by duplicating an API to save
  an import.
- Only an upper package that genuinely owns a composition lifecycle, state, and
  invariants may offer a new composition entry. Merely aggregating lower-level
  symbols is not a composition capability.

### 2.3 One extension mechanism beats a pile of hooks and SPIs

- Prefer **one** homogeneous mechanism plus type or middleware dispatch — one
  `Middleware func(Handler) Handler`, one generic type dispatcher — over a named
  slot per kind of extension.
- More slots means more surface area and more cognitive load. The MCP Go SDK
  using one middleware and explicitly refusing dozens of rarely-used hooks is the
  same trade-off.

### 2.4 A big package is not automatically a god package

- **An inherently cohesive large package is not force-split** — an execution
  engine or a parser family is naturally large. The test: extract only if the
  extraction **genuinely severs a coupling** and **does not break the public
  API**; otherwise keep it, because splitting for tidiness has negative return.
  The split signals are in [`AGENTS.md`](AGENTS.md) and
  [`REFACTORING.md`](REFACTORING.md).

### 2.5 Abstraction is broader the lower it sits; concreteness only flows upward

*(Learned twice the hard way, so it gets its own rule.)*

> **Which layer a type, interface, or field belongs to is decided by how many
> layers use it — not by which layer it seems to belong to.**

- **The rule**: a universal contract every consumer must obey **sinks to the
  bottom** (the thin kernel); a concrete capability only one consumer or driver
  uses **stays at that consumer's layer and never sinks into the base
  abstraction**. The bottom carries only universal, indivisible contracts, and
  each layer upward gets more concrete. **Concreteness accumulates upward; it is
  never poured downward.**
- **The anti-pattern this rule guards against**: pushing something
  consumer-specific down into the bottom layer, for "sharing" or because it looks
  like it belongs on the base type. The result is a bloated bottom polluted by
  one consumer's concern, and **every other** consumer forced to carry a concept
  it does not use. **"Lower means more general" is an illusion — lower is not
  more abstract; lower imposes one consumer's specifics on everyone.**
- **The test, in one sentence**: ask "**does every consumer need this, or only
  this one?**" Only one means it belongs to that consumer's layer, not the base
  type.
- **Distinguish two kinds of capability (the key part):**
  - A **mandatory control-flow contract across all implementations** — every
    driver must obey it, and ignoring it is a bug → **sink it into the kernel.**
  - An **optional capability one driver consumes** — a driver may ignore it and
    still be correct → **keep it at that driver's or consumer's layer.** The
    consumer defines the interface, a provider implements it structurally
    (without importing the concrete driver in reverse), and the kernel stays
    minimal.
- This rule makes §0's thin kernel, §2.1's consumer-defined interfaces, and the
  library-versus-application distinction explicit. It is one yardstick against
  one failure mode: pouring specifics downward.

### 2.6 Converging, inlining, or deleting a function needs a concrete redundancy signal

*(Learned the hard way, so it gets its own rule.)*

> The dual of §2.4. §2.4 says splitting for tidiness has negative return; this
> says **converging, inlining, or deleting for tidiness has negative return
> too.** Before touching a helper or a static function, you must be able to point
> at a **concrete redundancy signal**. If you cannot, leave it. **Function count
> is not a metric; redundancy is.**

**Act (only with a signal):**

| Redundancy signal | Action |
|---|---|
| **A dead function** (zero callers) | Delete |
| **One caller, a trivial body, and a redundant parameter that is always the same constant** | Inline (and switch a constant error to `errors.New` while you are there) |
| **A generic `func F[T]` whose `T` is exactly a type's receiver type parameter** | Make it a method on that type — the receiver already provides `T`, so the generic is redundant |
| **An implementation detail serving one type only, sitting at package scope** | Move it in as a method or a local constant, rather than polluting package scope |

**Do not act (these are not ceremony; inlining or deleting them is a
regression):**

| Case | Why it stays |
|---|---|
| **Two or more callers** | That is DRY; inlining recreates the duplication |
| **A generic helper instantiated at several `T`** | A method's type parameters can only come from the receiver, so it cannot be a method; a free generic function is correct |
| **No receiver exists** (a constructor, or `unmarshalX(bytes) (X, error)` creating a value from nothing) | There is nothing to hang it on |
| **A cohesive named abstraction, a predicate with a why-comment, or a symmetric pair** (marshal ↔ unmarshal) | The name is the documentation; keep it even with one caller, because inlining bloats the call site and breaks the symmetry |
| **A test helper living in `_test.go`** | Visible only to tests, not part of the production package API, and not package pollution |

- **The test, in one sentence**: ask "**can I point at one of the redundancy
  signals above?**" If yes, act. If it is shared, polymorphically generic,
  receiverless, a named documenting abstraction, or a test helper, leave it.
  **Converging with no signal is convergence as ceremony** — the same
  over-rotation as mechanically extracting interfaces (see §2.4 and
  [`REFACTORING.md`](REFACTORING.md)).
- **The anti-pattern this rule guards against**: mistaking one **real** local
  de-duplication for a whole-package health metric, and inlining cohesive named
  helpers one by one in pursuit of "fewer functions". That is the same mistake as
  mechanically applying one ISP template across a whole module — **a real local
  signal is not a global uniform action.**

---

## 3. Coding principles

*(The mandatory red lines are in [`AGENTS.md`](AGENTS.md); the hands-on details
are in [`REFACTORING.md`](REFACTORING.md).)*

Below are the **principles** — the *why* — and they apply across modules. The red
lines (`errors.New`, `%w`, no Java flavor, modern Go, OTel logging) are in
AGENTS.md; how to refactor toward them is in REFACTORING.md. This document does
not restate either; it gives the reason, noting where the MCP SDK independently
made the same call.

| Principle | Why |
|---|---|
| **An options struct beats variadic `WithXxx` and builder chains** | More readable, simpler to document, and adding a field does not break a caller. |
| **Accept interfaces, return structs** | Maximum compatibility on input, maximum information on output. An interface is a boundary; a struct is an implementation. |
| **Make zero values useful** | Fewer constructors, fewer mistakes; a struct of exported fields is usable at its zero value. |
| **`iter.Seq` and `iter.Seq2` beat channels** | A pull model, the context can be checked before the loop, and no goroutine leaks. |
| **Hide the protocol and transport; business code never sees the wire shape** | Business logic should not know about JSON-RPC or an SDK DTO. Envelope I/O is decoupled from business. A shared model is exposed directly by its one owner — never re-wrapped in an empty forwarder just to hide an import or a type identity. |
| **The smallest interface** | "The bigger the interface, the weaker the abstraction." A lower-level interface is easier to implement and easier to replace. |
| **Layered errors** | A protocol error carries a code; a tool or business error goes in the result, not in a Go error. |
| **A stateless kernel with differences as parameters** | One kernel serves many connections and sessions; per-session concerns arrive through a factory or a parameter, not an instance per session. |

---

## 4. When principles conflict

Follow "when principles conflict" in [`AGENTS.md`](AGENTS.md), plus one specific
to this document:

- **Discoverability versus a single entry**: keep the semantic owner and the
  public entry unique. Raise discoverability through documentation, examples, and
  naming — never manufacture a second API through a façade re-export (§2.2).

---

## 5. Design self-check

*(Before a new capability, a new package, or a public API change.)*

> This is the checklist for **designing something new**. **Refactoring existing
> code** has its own — naming, comments, pointer versus value, nil guards, guard
> clauses, locality, rhythm — in [`REFACTORING.md`](REFACTORING.md).

- [ ] Does this capability **reduce to the kernel**? Use the §1 litmus test to
      pick the shape (①/②/③) — or did it trip the "rebuilding the foundation"
      signal?
- [ ] Does the cross-package dependency it introduces point **only downward**?
      (The DAG holds.)
- [ ] Does the consumer depend on a **narrow interface it defined itself**, or
      did it grab a whole concrete type?
- [ ] Is the extension point **one homogeneous mechanism**, or another named
      slot?
- [ ] Is configuration an **options struct** rather than variadics or a builder
      chain?
- [ ] Is streaming an **iterator** rather than a channel?
- [ ] Has the public API change been discussed with the user, and does it migrate
      every workspace consumer in the same batch?
- [ ] Is this real DRY or false DRY? Would the abstraction force two pieces of
      code that evolve for different reasons to change together? (Prefer the
      duplication.)
- [ ] Before converging, inlining, or deleting a helper: can you point at a
      **concrete redundancy signal** — dead, single-caller with a redundant
      parameter, a generic duplicating a receiver type parameter, or an
      implementation detail leaked to package scope? If not, leave it, and do not
      inline a cohesive named abstraction for the sake of "fewer functions"
      (§2.6).
- [ ] Which layer: **does every consumer need this, or only this one?** If only
      one, do not sink it into the base abstraction — keep it at that consumer's
      layer (§2.5).

---

## In one sentence

**Scope's design is not personal taste: it is a set of organizing principles
corroborated both by embabel (convergent) and by the Go team's MCP SDK
(authoritative) — a thin kernel, three shapes of variation, a narrow waist, one
extension mechanism, base capability as a library first, and an explicit
lifecycle framework.** Run the §1 litmus test and the §5 checklist before
designing (answering "should I"), and use
[`REFACTORING.md`](REFACTORING.md) while refactoring (answering "how"), and you
will not drift.
