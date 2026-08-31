# 002 — API consistency and ergonomics audit

**Status**: open. Findings 1–3 are non-breaking and can land immediately;
findings 4–9 are breaking public API changes and need an explicit decision
before any of them is applied.

**Scope**: every functional module — `core`, `agent`, `rag`, `etl`, `eval`,
`tools`, `skills`, `mcp`, `a2a`, `otel`. The provider namespaces (`models/*`,
`vectorstores/*`, `historystores/*`, `tokenizers/*`) are excluded: they are leaf
adapters whose surface is fixed by the contracts audited here.

## Why this is one report

Findings 4–9 are all name and signature alignment on the same axis. Applying
them one at a time would leave the repository in a state where two vocabularies
coexist, which is worse for a reader than the current single inconsistency. If
they are accepted, they should land as one batch.

## What is already consistent

Stating the baseline matters, because the findings below are exceptions rather
than a pattern.

The seven modality protocol packages are structurally identical: every one
exposes `NewRequest(payload...)`, carries `Options` as a struct field with
`json:"options,omitzero"`, uses a value receiver for `Options.Validate` and a
pointer receiver for `Request`/`Response`/`Output`, and exports exactly
`ErrInvalidOptions`, `ErrInvalidRequest`, and `ErrInvalidResponse`. Of 164
constructors across the functional modules, 70 take a `Config` struct and the
remainder are value constructors where positional arguments are correct.

---

## 1. `otel/chat` skips the validation its whole family performs

**Module / package**: `otel/chat`
**Symbol**: `Middleware.Call`, `Middleware.Stream`

**Observed behavior**

```go
// otel/chat/chat.go:125
func (m Middleware) Call(next corechat.Model) corechat.Model {
	if lo.IsNil(next) {
		return nil
	}
	...
}
```

A nil model returns a nil model, and a zero-value `Middleware` — one not built
by `NewMiddleware` — is not checked at all, so it produces a wrapper with no
tracer or instruments.

Every sibling adapter checks both and returns an error:

```go
// otel/embedding/embedding.go:95
func (m Middleware) Wrap(next coreembedding.Model) (coreembedding.Model, error) {
	if lo.IsNil(m.tracer) || lo.IsNil(m.duration.Inst()) || lo.IsNil(m.tokens.Inst()) {
		return nil, fmt.Errorf("%w: middleware must be constructed with NewMiddleware", ErrInvalidConfig)
	}
	if lo.IsNil(next) {
		return nil, fmt.Errorf("%w: value must not be nil", ErrInvalidModel)
	}
	...
}
```

**Expected behavior**

The repository refuses silent degradation everywhere else: `rag` requires
"never degrade a nil or empty output silently into identity", `vectorstore`
rejects an unsupported search mode rather than falling back to semantic, and
`etl` returns `ErrSourceTooLarge` rather than a truncated corpus.
`otel/chat` is the one place that degrades quietly.

**Blast radius**

`Call` and `Stream` are the documented composition entry in `otel/README.md`.
Adding an error return is a breaking signature change, so this finding overlaps
with finding 4 and should land with it.

**Suggested fix layer**

`otel/chat`. Align the two checks with the rest of the family.

---

## 2. `document.Document` has no validating JSON boundary

**Module / package**: `core/document`
**Symbol**: `Document`

**Observed behavior**

```
metadata.Map          Clone Equal IsZero MarshalJSON Merge Set UnmarshalJSON Validate Values
metadata.Extensions   Clone Equal IsZero MarshalJSON Merge Set UnmarshalJSON Validate
media.Media           Bytes Clone MarshalJSON Reference URI UnmarshalJSON Validate
chat.Request          Clone MarshalJSON UnmarshalJSON Validate
chat.Response         Clone MarshalJSON UnmarshalJSON Validate
document.Document     Clone Validate
```

`core/document` contains exactly `doc.go` and `document.go`, and `Document`
declares only `Clone` and `Validate`. It marshals and unmarshals through the
default `encoding/json` path, so a decoded value is never validated.

**Expected behavior**

`core/ARCHITECTURE.md` states that metadata and extensions "must stay JSON-safe
and are validated explicitly at every I/O boundary", and every other core value
that crosses a boundary enforces that in `UnmarshalJSON`.

`Document` is the value with the widest boundary crossing in the repository: it
is produced by ETL, persisted and read back by 24 vector-store backends, and
consumed by RAG. A row read back from a database becomes a `Document` and enters
the retrieval path without `Validate` ever running. The value with the weakest
protection is the only one that regularly crosses a trust boundary.

**Blast radius**

Additive. Adding `MarshalJSON`/`UnmarshalJSON` changes no signature. It will
start rejecting documents that were already invalid, which is the point, so the
backends should be run against the conformance suites after the change.

**Suggested fix layer**

`core/document`. The same validate-then-assign shape the modality packages use.

---

## 3. `chat.ToolOutput` has no JSON boundary

**Module / package**: `core/chat`
**Symbol**: `ToolOutput`

**Observed behavior**

```
core/chat/tool.go:73  func (t ToolOutput) Clone() ToolOutput
core/chat/tool.go:84  func (t ToolOutput) Validate() error
core/chat/tool.go:104 func (t ToolOutput) Text() (string, bool)
```

Its siblings `Part`, `Message`, `Request`, and `Response` all carry
`MarshalJSON` and `UnmarshalJSON`.

**Expected behavior**

`ToolOutput` holds `Details json.RawMessage` and `Content []Part`, and it
crosses two boundaries: the MCP mapping to `CallToolResult`, and the Agent
`ExecutionState` snapshot. It should validate on decode like the rest of the
family.

**Blast radius**

Additive.

**Suggested fix layer**

`core/chat`.

---

## 4. The `otel` adapters expose four different composition shapes

**Module / package**: `otel/*`
**Symbol**: `Middleware` methods

**Observed behavior**

| Adapter | Method | Returns |
|---|---|---|
| `chat` | `Call` / `Stream` | `T` |
| `speech` | `Wrap` / `WrapStream` | `(T, error)` |
| `embedding`, `image`, `rag`, `tool`, `eval` | `Wrap` | `(T, error)` |
| `history` | `Store` / `Conversations` | `T` |
| `vectorstore` | `Index` / `Search` / `DeleteIDs` / `DeleteWhere` | `T` |

`chat` and `speech` wrap structurally identical contracts — a `Model` plus an
independent `Streamer` — yet use different verbs, and only one of them returns
an error:

```go
observed := instrumentation.Call(model)         // otel/chat
observed, err := instrumentation.Wrap(model)    // otel/speech, same shape
```

**Expected behavior**

`AGENTS.md`: "One concept, one term." A user who has learned one adapter should
be able to use the next one without reading it.

**Blast radius**

Every composition root that installs an `otel` adapter. All 13 adapters should
change together.

**Suggested fix layer**

`otel`. Proposed alignment: `Wrap` and `WrapStream` returning `(T, error)`
everywhere a single capability is decorated. `history` and `vectorstore` keep
capability-named methods, because they decorate several distinct capabilities
and the name carries information there — but they should return `(T, error)`
like the rest.

---

## 5. `rag.Candidate.Score` is a bare `float64` between two validated types

**Module / package**: `rag`
**Symbol**: `Candidate.Score`

**Observed behavior**

```go
core/vectorstore.Score  float64  // 7 ScoreFrom* constructors, Validate
core/rerank.Score       float64  // Float64, MarshalJSON, UnmarshalJSON, Validate
rag.Candidate.Score     float64  // nothing
```

A retrieval path runs `vectorstore.Search` → `rag.Candidate` → `rerank.Model` →
`rag.Candidate`, losing the type at both hand-offs. Reciprocal-rank fusion,
`TopK` truncation, and `MinScore` filtering all operate on the unconstrained
form, so NaN, negative, and out-of-range values are representable.

**Expected behavior**

`vectorstore.Score` documents that scores "preserve ordering but are not
comparable across providers or search modes" — a constraint that needs a type to
carry it precisely where fusion happens.

**Blast radius**

`rag.Candidate` is a public struct with exported fields; changing the field type
breaks every consumer that constructs one.

**Suggested fix layer**

`rag`. Either reuse one of the existing `Score` types or define
`rag.Score` with `Validate`.

---

## 6. `workflow` uses two vocabularies for the same kind of bound

**Module / package**: `agent/workflow`
**Symbol**: `MapConfig.ItemLimit`, `LoopConfig.MaxIterations`

**Observed behavior**

```go
// agent/workflow/map.go:32
// ItemLimit is the positive maximum accepted input length.
ItemLimit uint32

// agent/workflow/loop.go:41
// MaxIterations is the positive hard upper bound on body child Processes.
MaxIterations uint32
```

Both are positive hard upper bounds, in one package, named two ways. Across the
functional modules, 33 of 35 bound fields use the `Max*` prefix — `MaxSteps`,
`MaxEffects`, `MaxDepth`, `MaxChildren`, `MaxModelCalls`, `MaxActiveChildren`,
and so on. `ItemLimit` is the outlier.

A call site has to hold both conventions at once:

```go
WindowSize: workerParallelism, ItemLimit: maximumPlannedTasks,   // Map
MaxIterations: rounds,                                            // Loop
```

**Expected behavior**

One prefix for one meaning.

**Blast radius**

`agent/workflow` consumers and the examples.

**Suggested fix layer**

`agent/workflow`. `ItemLimit` becomes `MaxItems`. `WindowSize` stays: it is a
batch size, not a bound, and the different name is carrying a real difference.

---

## 7. `a2a.Agent.Run` is a third verb for invoking a capability

**Module / package**: `a2a`
**Symbol**: `Agent.Run`

**Observed behavior**

```go
core/*.Model      Call(ctx, request)                        // 7 modalities
core/tool.Tool    Call(ctx, invocation)
a2a.Agent         Run(ctx, input string) iter.Seq2[string, error]
```

Invoking a capability is already `Call` in Core and `Start`/`Await` in `agent`.
`a2a` introduces a third word for the same act.

Two neighbouring verbs were checked and are **not** findings:
`tools/shell.Executor.Run` is the domain verb for running a command, and
`planning.Execute` is the domain verb for executing an action. Both carry
meaning their alternatives would lose.

**Expected behavior**

`agent/ARCHITECTURE.md`: "One concept, one term. Never keep plan and todo, run
and execution, sub-agent and child-agent as parallel public concepts."

**Blast radius**

Every `a2a.Agent` implementation.

**Suggested fix layer**

`a2a`. `Stream` matches the `iter.Seq2` return and aligns with
`chat.Streamer.Stream`.

---

## 8. `Min` and `Minimum` coexist inside `eval`

**Module / package**: `eval`
**Symbol**: `CompositeConfig.MinimumPassed`, `Distribution.Minimum`

**Observed behavior**

```go
eval/composite.go:65   MinimumPassed int      // a policy threshold
eval/summary.go:31     Minimum       float64  // a distribution statistic
core/vectorstore       MinScore               // a policy threshold
```

One package uses two spellings of the same word root for two different kinds of
thing, and the threshold spelling disagrees with the threshold spelling used in
`core`.

**Expected behavior**

Thresholds read the same everywhere.

**Blast radius**

`eval` consumers.

**Suggested fix layer**

`eval`. `MinimumPassed` becomes `MinPassed`, matching `MinScore`. `Minimum`
stays: it is a statistic, not a threshold.

---

## 9. `embeddingclient.New` is the only client constructor without a `Config`

**Module / package**: `core/embeddingclient`
**Symbol**: `New`

**Observed behavior**

```go
chatclient.New(model chat.Model, config Config) (Client, error)
embeddingclient.New(model embedding.Model) (Client, error)
```

**Expected behavior**

`REFACTORING.md`: "Construction config for Scope's own abstractions passes a
`Config` struct." `embeddingclient` has nothing to configure today, so the
shorter signature is locally reasonable — but it means the two clients cannot be
learned once, and adding any option later is a breaking change that an empty
`Config{}` would have absorbed.

**Blast radius**

Every `embeddingclient.New` call site. The change is mechanical.

**Suggested fix layer**

`core/embeddingclient`. Add an empty `Config` and align the signature.

---

## 10. `history.NewWindowStore` takes a bare policy argument

**Module / package**: `core/history`
**Symbol**: `NewWindowStore`

**Observed behavior**

```go
func NewWindowStore(store Store, limit int) (WindowStore, error)
```

Most positional constructors in the repository build values — `NewTextPart`,
`NewBytes`, `NewCondition`. This one builds a component with a policy, and
`ErrWindowTooSmall` shows the policy already has constraints.

**Expected behavior**

A component with a policy takes a `Config`, so a later policy — keep the system
message, bound by tokens rather than count — does not need a second constructor
or a signature break.

**Blast radius**

`core/history` consumers.

**Suggested fix layer**

`core/history`. `NewWindowStore(store, WindowConfig{Limit: n})`.

---

## Deliberately not a finding

`tools` treats `New(nil)` differently per capability: `shell` and `fs` fall back
to the local backend, while `web` and `httpreq` return an error. That reads as an
inconsistency but is a principled distinction, documented in
`tools/ARCHITECTURE.md`: a capability with a safe local implementation works out
of the box, and one that reaches the network must be configured explicitly —
`httpreq` has no default allowlist because a permissive default is an SSRF hole.
Unifying these would manufacture a dangerous default.

## Priority

| # | Finding | Class | Breaking |
|---|---|---|---|
| 1 | `otel/chat` skips validation and returns nil silently | Correctness | Yes (with 4) |
| 2 | `document.Document` has no validating JSON boundary | Correctness | No |
| 3 | `chat.ToolOutput` has no JSON boundary | Consistency | No |
| 4 | `otel` exposes four composition shapes | Consistency | Yes |
| 5 | `rag.Candidate.Score` is a bare `float64` | Type safety | Yes |
| 6 | `workflow.ItemLimit` versus `MaxIterations` | Consistency | Yes |
| 7 | `a2a.Agent.Run` is a third invocation verb | Consistency | Yes |
| 8 | `eval.MinimumPassed` versus `MinScore` | Consistency | Yes |
| 9 | `embeddingclient.New` has no `Config` | Consistency | Yes |
| 10 | `history.NewWindowStore` takes a bare limit | Extensibility | Yes |

Findings 2 and 3 close real validation gaps and change no signature. Findings 1
and 4–10 are breaking public API changes and, per `AGENTS.md`, need the scope,
blast radius, and alternatives agreed before any of them is applied.
