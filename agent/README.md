# agent

`agent` is the Scope Agent execution kernel: an embeddable Go framework that
gives any execution strategy one Process lifecycle, one signal path, one effect
protocol, and one portable snapshot format.

It owns execution semantics. It does not own a database, a queue, a scheduler,
a user model, or a product session — a Host composes those around it.

## Install

```bash
go get github.com/Tangerg/scope/agent
```

## When you need it

A direct model call is always the first-class path. Reach for `core/chatclient`
until one of these becomes true:

| You need | Use |
|---|---|
| One or a few explicit model calls | `core/chatclient` |
| The model to choose tools autonomously | `Engine` + an `interaction` Definition |
| Pause, resume, budgets, and child tasks | `Engine` + snapshots + child Processes |
| Multi-definition deployment, routing, governance | `platform` + a Host |

## Packages

| Package | Owns |
|---|---|
| `agent` | The kernel: `Definition`, `Execution`, `Engine`, `Process`, `Signal`, `Effect`, `Transition`, and the tree snapshot format |
| `interaction` | The model-driven strategy: tool calls, delegation, structured input waits |
| `planning` | Goal and action vocabulary shared by planners |
| `planning/goap` | Goal-oriented action planning over that vocabulary |
| `workflow` | Deterministic staged composition of Definitions |
| `platform` | Deployment registry, routing, and lifecycle above a single Engine |
| `agenttest` | Conformance suites a Definition, dispatcher, or durability port must pass |

## The kernel in one paragraph

A `Definition` owns immutable behavior and mints a serializable `Execution`. The
`Engine` owns everything else: it starts a Process, delivers `Signal` values
through a bounded mailbox, drives `Execution.Step`, prepares and settles
`Effect` batches, admits child Processes, enforces `Budget`, publishes
observation, and produces snapshots. Strategy payloads stay opaque to the
kernel; persistence stays a Host responsibility behind the `TreeDurability`
port.

```go
engine, err := agent.NewEngine(agent.EngineConfig{ /* ... */ })
if err != nil {
    return err
}

outcome, err := engine.Start(ctx, agent.StartRequest{ /* ... */ })
if err != nil {
    return err
}
```

## Durability

A Process tree is restored from a `TreeSnapshot`, not replayed from a log. The
snapshot carries the committed execution state, the effect phases
(`planned → pending → settled`), the waiting subtree, and a `TreeIncarnationID`
that fences a stale writer out on activation. A Host implements
`TreeDurability`; the kernel never opens a transaction, owns a lease, or writes
an outbox.

## Conformance

`agenttest` is how an implementation proves it belongs:

```go
func TestDefinition(t *testing.T) {
    agenttest.DefinitionConformance{ /* ... */ }.Run(t)
}
```

The same package holds the durability and dispatcher suites, so a Host store and
a strategy dispatcher are held to the kernel's contract rather than to a
reviewer's memory.

## Examples

`examples/` holds independent consumers of the public API. They share no
test-only helper and import no internal type, so a build or test failure there
is a real consumer-facing contract problem — they are contract evidence, not a
recommendation to turn every function into a Process.

All of them use deterministic local components and run without credentials or
network access:

```sh
GOWORK=off go run ./examples/direct_vs_managed
```

| Example | What it proves |
|---|---|
| `direct_vs_managed` | A direct `chatclient` call beside an Engine-owned Interaction Process: direct stays the smallest embedding level, managed adds lifecycle, signals, effects, snapshots, limits, and events |
| `autonomous` | A model-directed `model → Tool → model` loop where the model picks the tool and the stop point while the Definition supplies a hard local call limit |
| `composition` | One Definition run directly, then composed with an Interaction as two heterogeneous child Processes — both through the same public waist |
| `workflow` | A managed Call plus a two-branch Fork: one root and three exact child Processes, so ordered orchestration creates no second runtime and hides no branch lifecycle |
| `orchestrator_workers` | An Interaction decomposing an objective, a bounded Workflow Map running three workers in stable order, and another Interaction synthesizing typed results — no supervisor strategy, worker registry, or shared blackboard |
| `evaluator_optimizer` | A bounded Workflow Loop with optimizer and evaluator child Processes, where consumer-owned typed state carries attempts, feedback, best-so-far, and acceptance separately from exhaustion |
| `workflow_patterns` | Prompt chaining, routing, sectioning, declaration-order aggregation, and a consumer-owned tie break in one tree, with no pattern-specific framework type |
| `embedded_vs_platform` | The same Deployments run with a caller-owned resolver and with Platform discovery: output, terminal status, usage, tree, admission facts, and observation semantics must match, because Platform does not wrap or replace Engine |

## Boundaries

The kernel never imports a Host application, never adds a retry layer over a
provider SDK, and never grows a second extension chain. See
[`ARCHITECTURE.md`](ARCHITECTURE.md) for the accepted architecture and the
engineering standard, both enforced by executable tests in this module.
