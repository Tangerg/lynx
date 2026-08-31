# 003 — Value semantics, pointers, and ownership

**Status**: open. One documentation gap and one judgment call. Most of this
report records why the current shapes are correct, so a later pass does not
"unify" them into something worse.

**Scope**: the protocol value types in `core`, and the ownership rules that
`chatclient`, `agent`, and `rag` rely on.

## Why this report exists

The value-versus-pointer choices across the protocol types read as fragmented.
They are not: there are three categories with three reasons. What is genuinely
missing is the ownership model that explains them, which is currently spread
across four places and is wrong if you only know three of them.

---

## The central fact: value semantics are not immutability

Every "value" protocol type in `chat` carries reference fields:

```go
Message.Parts      []Part
Part.Signature     []byte     Part.Media *media.Media     Part.ToolCall *ToolCall  …
Options.Stop       []string   + six *T fields + Extensions
ToolOutput.Content []Part
Usage              three *int64
```

So `copy := original` is not a snapshot: both share the backing arrays and the
pointers. **The value-versus-pointer decision in this repository is about
presence and nil-meaning, not about immutability.** Immutability is produced by
two other mechanisms, described below.

Conflating the two is what makes the shapes look arbitrary.

---

## Ruling 1 — the three categories are correct

| Category | Types | Shape | Reason |
|---|---|---|---|
| Composed inline | `Message`, `Part`, `Options`, `Usage`, `ToolCall`, `ToolOutput` | value | Never optional, nil carries no meaning, embedded into larger values |
| SPI boundary | `Request`, `Response`, `Output` | pointer | nil must be rejectable, decode writes in place, crosses the `Model` interface |
| Cross-package carrier | `media.Media`, `document.Document` | pointer | Genuinely optional at several sites (`Part.Media`), and large |

This matches the rule already stated in `REFACTORING.md`: necessarily present,
small, and read-only passes by value; genuinely optional, where nil is a state
the code branches on, passes a pointer.

**Do not unify these.** Making `Message` or `Part` a pointer would turn a
contiguous `[]Part` into one heap allocation per part and would introduce a nil
part as a new illegal state.

---

## Ruling 2 — deep `Clone` with shallow constructors is deliberate

`Clone` recurses all the way down:

```go
Request.Clone → Message.Clone → Part.Clone → Media.Clone
```

A constructor clones only its immediate slice:

```go
func NewUserMessage(parts ...Part) Message {
	return Message{Role: RoleUser, Parts: slices.Clone(parts)}
}
```

So after `NewUserMessage(part)`, the caller still holds a live handle to
`part.Media` and everything else the part points at.

That is the right trade. Deep-cloning at every constructor would copy the same
media blob once per nesting level — part, then message, then request — for every
build. The guarantee is instead taken exactly once, at the boundary that needs
it. `chatclient` documents it:

> Client snapshots each request before invoking middleware or a provider, so
> those layers cannot mutate caller-owned protocol values.

A provider reached directly, without the client, owes the same guarantee through
its contract: `Model.Call` implementations "must not retain or mutate request".

**Do not add deep cloning to constructors.**

---

## Ruling 3 — `Output.Message` is correctly a pointer

```go
message := chat.NewAssistantMessage(part)     // returns a value
output, err := chat.NewOutput(&message, ...)  // takes a pointer
```

The `&` reads as friction, and `Message` is a value type everywhere else. But
`Output.Validate` shows the pointer carries real meaning:

```go
if o.Message == nil && o.FinishReason == "" && o.Metadata == nil {
	return fmt.Errorf("%w: output has no message, finish reason, or metadata", ErrInvalidResponse)
}
```

An output may legitimately carry only a finish reason or only metadata, which a
stream needs. `Message` is optional here, so the pointer is correct and changing
it to a value would delete that state.

**Do not change the type.** The godoc on `Output.Message` should state why it is
optional, so the next reader does not read the `&` as an accident.

---

## Finding 1 — the two clone-visibility tiers are undocumented

**Module / package**: `core/chat`

**Observed behavior**

```
Message   Clone      Output              clone
Options   Clone      OutputMetadata      clone
Part      Clone      ResponseMetadata    clone
Request   Clone      ResponseAccumulator clone
Response  Clone      Usage               clone
ToolDefinition Clone
ToolOutput     Clone
ToolResult     Clone
OutputFormat   Clone
```

There is a coherent rule behind this, and it is written down nowhere: a value
the **caller** constructs and owns exposes `Clone`; a value only a **provider**
produces and the caller only reads clones privately.

**Note on a correction**: an earlier reading of this called `Usage` the only
value type with reference fields whose clone is private. That was wrong —
`Output`, `OutputMetadata`, and `ResponseMetadata` are in the same tier, and
`Usage` is consistent with them rather than an outlier.

**Expected behavior**

The tier is defensible, but it has one reachable consequence. `Response.Output`
and `Response.Metadata` are exported fields, so a caller can hold
`response.Metadata.Usage`. Copying that struct shares its three `*int64`, and
there is no supported way to take an independent copy of a `Usage` alone — the
caller has to clone the whole `Response` and read the field back out.

For a Host aggregating token usage for billing across many responses, that is
the exact operation it wants, and the API does not offer it.

**Blast radius**

Additive if resolved by exporting. Exporting `Usage.Clone` alone would break the
tier's symmetry, so either export the response-side tier together or leave it
and document the rule.

**Suggested fix layer**

`core/chat`. This is a judgment call, not a defect:

- Leave as is and state the tier rule in `core/ARCHITECTURE.md`; a Host clones
  the whole `Response`. Lowest churn.
- Or export `Clone` on `Usage`, `Output`, `OutputMetadata`, and
  `ResponseMetadata` together, which makes one rule instead of two and serves
  the billing case directly.

The pointer fields on `Usage` mark "reported zero" against "not supported", and
nothing writes through them today, so the aliasing risk is currently
theoretical.

---

## Finding 2 — the ownership model is not stated anywhere

**Module / package**: `core`

**Observed behavior**

Four rules together form the model, and each is documented separately or not at
all:

1. A constructor clones one level, not recursively.
2. `Clone` is deep.
3. The snapshot is taken once, at the `chatclient` boundary — not at
   construction.
4. A `Model` implementation reached directly owes the same guarantee through its
   contract.

Knowing any three of these produces a wrong mental model. Knowing 1 and 2
without 3 suggests the constructors are buggy. Knowing 2 and 3 without 1
suggests a constructed value is already an owned snapshot, which it is not.

**Expected behavior**

`core/ARCHITECTURE.md` already states "Pointers express presence, not anemia",
which covers the shape question. It does not cover the ownership question, which
is the one that actually determines whether a caller can mutate a value another
layer is holding.

**Blast radius**

Documentation only.

**Suggested fix layer**

`core/ARCHITECTURE.md`, next to the existing pointer rule. Proposed text:

> **Value semantics are not immutability.** Every protocol value carries
> reference fields, so copying a struct shares its backing arrays and pointers.
> Ownership is established by `Clone`, which is deep, and taken once at the
> boundary that needs it: `Client` snapshots a request before middleware or a
> provider observes it. A constructor clones only its immediate slice, because
> deep-cloning at every nesting level would copy the same media payload once per
> layer. A `Model` implementation reached directly, without the client, owes the
> same guarantee through its contract — it must not retain or mutate the
> request.

---

## Deliberately not findings

- **Making `Message` or `Part` a pointer for uniformity.** Value semantics buy a
  contiguous `[]Part`, allocation-free construction, and a usable zero value.
  Pointers would add a heap allocation per part and a nil-part illegal state.
- **Deep cloning inside constructors.** It converts O(1) construction into
  O(depth) copying, for a guarantee the client boundary already provides.
- **Unifying the `Clone` return shape.** `Message.Clone() Message`,
  `Media.Clone() *Media`, and `Request.Clone() *Request` each match their own
  type's shape, which is what a caller expects.
- **Removing the `&` at `NewOutput`.** It is the visible cost of a real optional
  field, not an accident.

## Priority

| # | Item | Class | Breaking |
|---|---|---|---|
| 2 | Ownership model undocumented | Documentation | No |
| 1 | Clone-visibility tiers undocumented; `Usage` not independently cloneable | Judgment call | No, if the tier is exported together |

Neither is a defect. Finding 2 is the one worth doing, because the current
fragmentation is a reading problem rather than a code problem, and writing the
rule down is what fixes it.
