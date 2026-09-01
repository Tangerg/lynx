# Microsoft Agent Framework Go: streaming agents and workflow executors

Evidence baseline: `agent-framework-go` commit
`6aabdc7ea2d2af7ac5f673dd693a01d5e61bed35`.

## Framework-level judgment

Microsoft Agent Framework Go offers both a lightweight streaming agent
invocation and a more explicit Workflow and Executor layer. It does not require
an ordinary agent to enter a heavyweight execution model from the start, which
is directly relevant to Scope's own two-path design.

This round compares framework packages only. Samples, service shells, and the
number of prebuilt agents do not score the kernel.

## Reviewable evidence

- `RunFunc` returns streaming results as a Go iterator, forming a light agent
  execution surface.
- A concrete agent composes a client, tools, instructions, and middleware.
- Workflow and Executor organize more complex flows with nodes, edges, events,
  and run state.
- A checkpoint store provides the interface for saving workflow state.
- `RequestPort` expresses a human or external request pause as part of the
  workflow protocol.
- Types such as `PortableValue` serve data representation across execution
  boundaries.
- Middleware, context providers, and trace context provide extension and
  context propagation.

## The eight dimensions

| Dimension | Microsoft AF Go's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A unified agent and client content model; provider implementations cooperate with the framework | Scope isolates provider leaves more finely |
| Minimal contract | `RunFunc` is a very light streaming entry point | Scope's managed entry requires Definition and Execution, with a separate direct client path |
| State ownership | Agent session and workflow state are layered | Scope leaves product state explicitly to the host |
| Side effects | Model and tool calls happen directly inside an agent or executor | Scope unifies external work identity through Effects |
| Orchestration | Executors, nodes, edges, and RequestPort | Scope Workflow focuses on managed child Processes |
| Recovery | A checkpoint store plus workflow state | Arbitrary I/O inside an agent does not automatically gain replay semantics |
| Extension | Middleware, context providers, trace context | Scope also prefers middleware, but keeps OpenTelemetry a separate module |
| Dependencies | One framework carries both agents and workflows | Scope's modules are more fragmented, with purer boundaries and heavier governance |

## What Scope should learn

1. **Separating light invocation from workflow.** `RunFunc` shows that an
   ordinary call need not carry workflow concepts. Scope should keep
   strengthening the direct `chatclient` path.
2. **RequestPort.** Expressing human input or an external decision as an
   explicit pause point recovers better than burying the wait in an arbitrary
   callback.
3. **A portable-value boundary.** Checking serializability before a value is
   checkpointed or crosses an execution boundary moves errors earlier.
4. **Go-native streaming APIs.** Using `iter.Seq2` to carry a value and an
   error is a calling experience worth continuing to watch.

## What Scope should not copy

- Do not let direct side effects in an ordinary agent erode the Effect rules of
  a managed Execution.
- Do not merge the workflow executor and the existing process runtime into one
  broadly scoped container.
- Do not assume a checkpoint store automatically solves idempotent commit for
  models, tools, and external systems.

## Final placement

Microsoft Agent Framework Go strikes an instructive balance between a
lightweight agent and an explicit workflow. Scope's recovery semantics are
stricter, but its light path has to be equally clear — otherwise users will
mistake managed execution for the only entry point.
