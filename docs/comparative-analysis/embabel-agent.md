# Embabel Agent: blackboard and GOAP dynamic planning

Evidence baseline: `embabel-agent` commit
`6988f286544bb792bed35d8ae45812c446be082d`.

## Framework-level judgment

Embabel Agent's center is neither a fixed DAG nor a single model-tool loop. An
action participates in planning dynamically, based on the blackboard's current
state, its preconditions, and the goal. It pushes further along a line Scope
treats as one opt-in strategy rather than a kernel default: searching for a
feasible action sequence at runtime.

The Embabel shell, sample applications, and the Spring Boot release experience
are application or ecosystem concerns and do not enter the framework score.

## Reviewable evidence

- The blackboard holds running objects and the facts a condition check needs.
- An action describes inputs, outputs, preconditions, cost, and execution
  logic.
- `AgentProcess` expresses one agent run and its state.
- `AgentPlatform` assembles agents, actions, and the run infrastructure.
- The GOAP planner selects an action path from the current state and goal
  rather than requiring the author to fix a complete flow in advance.
- Spring annotations, events, and beans are the primary extension and assembly
  mechanisms.

## The eight dimensions

| Dimension | Embabel's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | Domain objects and the Spring ecosystem come first | Scope emphasizes independent protocols and provider isolation |
| Minimal contract | Action, condition, goal, and process form the planning model | Scope's Definition and Execution are smaller and do not plan automatically |
| State ownership | The blackboard and AgentProcess hold shared run facts | Scope's Execution snapshot is more closed, and the host holds product facts |
| Side effects | Happen directly when an action executes | A Scope Step emits an Effect and performs no I/O |
| Orchestration | GOAP selects actions dynamically from the goal | Scope Workflow uses closed stages and explicit child Processes |
| Recovery | Process and blackboard persistence exist as a direction | External work inside an action does not automatically gain an effect replay identity |
| Extension | Annotations, Spring beans, events | Scope middleware and listeners are more host-neutral |
| Dependencies | Strong Spring and JVM ecosystem integration | Scope assembles explicitly, with lower container coupling |

## What Scope should learn

1. **Separating the goal from current facts.** Why an execution completed, and
   which condition is still missing, should be observable — in the kernel as
   much as in `planning/goap`.
2. **Action metadata.** Preconditions, products, and cost are a shared language
   for planning, explanation, and testing.
3. **Explaining the execution path.** When selection is dynamic, recording
   "why this action" audits far better than recording only the final call.
4. **The consumer's view of a blackboard.** When several actions cooperate, the
   query experience over shared facts is worth learning from, though ownership
   must still be explicit.

## What Scope should not copy

- Do not push GOAP into the general kernel; it is a costly, strongly opinionated
  planning strategy and belongs in `planning/goap` as one admitted strategy.
- Do not replace typed `ExecutionState` with an unconstrained shared
  blackboard.
- Do not let arbitrary side effects inside an action bypass the effect
  semantics that long-running execution depends on.
- Do not introduce annotation scanning or implicit container assembly in place
  of explicit Go dependencies.

## Final placement

When a task path must be planned dynamically from running facts and a goal,
Embabel's model is more natural than Scope's closed Workflow. When the emphasis
is deterministic recovery, identifiable side effects, and child execution
lifecycle, Scope's semantics are stronger. Dynamic planning belongs on top of
the kernel, never inside it.
