# Spring AI: model composition inside an ecosystem

Evidence baseline: `spring-ai` commit
`e5e277fd08a017c5a3efc57aef377d2a067e91dd`.

## Framework-level judgment

Spring AI's first goal is letting models, ChatClient, Advisors, ToolCallbacks,
Memory, VectorStore, and RAG compose the way Spring users expect. It optimizes
integration consistency and declarative extension inside enterprise
applications, not a standalone durable agent process kernel.

Comparing Spring's starter count, RAG components, or ecosystem size directly
against the Scope kernel is meaningless. This round analyzes its composition
model only.

## Reviewable evidence

- `Model`, `StreamingModel`, and `ChatModel` form the model invocation
  abstraction.
- `ChatClient` organizes prompts, advisors, tools, and return values behind a
  fluent API.
- `Advisor` joins an ordered call chain and can modify the request and response
  before and after a call.
- `ToolCallingAdvisor` implements the tool loop as a composable advisor.
- Chat Memory, VectorStore, and Modular RAG cooperate through Spring beans and
  modules.
- Observability integrates with the Spring Observability ecosystem.

## The eight dimensions

| Dimension | Spring AI's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A unified model interface that follows Spring types and the container closely | Scope depends on no application container and is more host-neutral |
| Minimal contract | An ordinary ChatClient or Model call is very direct | Scope's managed Execution is heavier, with a separate client layer |
| State ownership | Memory, beans, and application services hold it jointly | Scope draws an explicit host and execution-snapshot boundary |
| Side effects | Clients, advisors, and tool callbacks execute directly | Scope produces an Effect first, then executes |
| Orchestration | An advisor chain plus application workflow | No Scope-style managed child Process tree |
| Recovery | Memory integrated with external storage | No unified execution snapshot or effect replay |
| Extension | Ordered advisors, beans, observations | Scope middleware is more standalone, with less ecosystem autoconfiguration |
| Dependencies | Starters and modules consistent with the Spring ecosystem | Scope's leaf modules avoid container binding, so assembly is more explicit |

## What Scope should learn

1. **The discoverable composition experience of advisors.** Call
   augmentation, the tool loop, and observation can all be understood along one
   chain.
2. **Interface-oriented capability assembly.** Memory, VectorStore, and RAG are
   replaceable collaborators rather than global services that must enter the
   agent kernel.
3. **Ordinary calls first.** A simple model call should not require building an
   execution graph or persisting a Process.
4. **Consistent observability conventions.** Ecosystem-wide naming and metrics
   are more useful than exposing many low-level hooks.

## What Scope should not copy

- Do not introduce a container lifecycle or global bean-style dependency
  resolution.
- Do not re-aggregate application capabilities such as RAG, Memory, and
  VectorStore into the agent kernel.
- Do not use an advisor chain in place of effect execution that needs exact
  identity.
- Do not hide the real dependency direction for the convenience of
  autoconfiguration.

## Final placement

Spring AI provides lower-friction model and tool composition inside Spring
applications. Scope suits systems that need host-independent, recoverable
execution semantics. The core difference is ecosystem composition versus
execution kernel, not language or feature count.
