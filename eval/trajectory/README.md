# eval/trajectory

`trajectory` is the Agent-domain integration for Scope's subject-agnostic
evaluation kernel. It records existing Agent and Interaction observation facts
and evaluates terminal success, exact Tool calls, deterministic replay
consistency, Framework usage, model tokens, and latency.

Install it independently from the eval kernel:

```bash
go get github.com/Tangerg/scope/eval/trajectory
```

Use one `Recorder` at both existing observation boundaries, then consume the
completed root run:

```go
recorder := &trajectory.Recorder{}

engine, err := agent.NewEngine(agent.EngineConfig{
    EventListeners: []agent.EventListener{recorder},
})
dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
    Client: modelClient,
    Tools: tools,
    Observer: recorder,
})

result, err := engine.Run(ctx, deployment, input)
actual, err := recorder.Take(result)
```

`Evaluator` consumes a typed `Sample`. Its expectation can assert a terminal
status and output, an exact ordered Tool sequence, a semantic replay baseline,
and optional upper bounds for Steps, Effects, Signals, dropped deltas, tokens,
and duration.

The module records and evaluates facts only. It does not schedule execution or
replay, persist datasets or artifacts, track experiments, render reports, or
own application workflows.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for its dependency and behavior
contracts.
