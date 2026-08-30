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

## Boundaries

The kernel never imports a Host application, never adds a retry layer over a
provider SDK, and never grows a second extension chain. See
[`ARCHITECTURE.md`](ARCHITECTURE.md) for the accepted architecture and
[`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md) for the implementation
standard both are enforced by executable tests in this module.
