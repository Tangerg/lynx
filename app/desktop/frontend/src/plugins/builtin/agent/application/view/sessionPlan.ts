import { useMemo } from "react";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { useActiveSessionId } from "../session/activeSession";

export type { PlanStep } from "@/plugins/sdk/types/agentSessionView";

/**
 * One step of the session's plan, in this context's own words.
 *
 * The Plan is a Session projection written only by the root Run
 * (`plan.get` / `plan.updated`), not a transcript Item — so it has no run
 * of its own and nothing about it is per-turn.
 */
const TOOL_STEP_STATUS: Record<string, PlanStep["status"]> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

const NO_STEPS: readonly PlanStep[] = Object.freeze([]);

export function planSteps(plan: AgentPlan | undefined): readonly PlanStep[] {
  const steps = plan?.steps;
  if (!steps || steps.length === 0) return NO_STEPS;
  return Object.freeze(steps.map((step) => Object.freeze({ ...step })));
}

/** Immutable presentation of one exact Session-scoped Plan replacement. The
 * Runtime revision is a whole-replacement identity inside one projection
 * generation; generation prevents a server/recovery successor with the same
 * Session and revision from inheriting predecessor presentation state. */
export class SessionPlan {
  readonly identity: string;
  readonly generation: number;
  readonly revision: number;
  readonly steps: readonly PlanStep[];

  private constructor(
    sessionId: string,
    generation: number,
    revision: number,
    steps: readonly PlanStep[],
  ) {
    this.identity = JSON.stringify([sessionId, generation, revision]);
    this.generation = generation;
    this.revision = revision;
    this.steps = steps;
  }

  static fromSnapshot(
    sessionId: string,
    generation: number,
    plan: AgentPlan | undefined,
  ): SessionPlan {
    return new SessionPlan(sessionId, generation, plan?.revision ?? 0, planSteps(plan));
  }

  activeStep(): PlanStep | undefined {
    return activePlanStep(this.steps);
  }

  progress(): { done: number; total: number } {
    return planProgress(this.steps);
  }
}

/**
 * The plan a `set_plan` call wrote, read from the call's own arguments.
 *
 * The same steps in the same vocabulary as the session snapshot above, from the
 * other end: the snapshot is what the plan IS, these are what one call SAID. The
 * arguments carry `{steps:[{description,status}]}` with no ids — ids belong to the
 * runtime's plan, not to an argument list — so the index stands in, which is all a
 * list key needs.
 *
 * Structured arguments and not the rendered result text: the runtime also renders
 * the plan as `[x] …` lines for the model to read, and parsing that back would be
 * a second answer to "what are the steps" that goes stale the moment the marks
 * change.
 */
export function planStepsFromArguments(args: unknown): readonly PlanStep[] {
  if (typeof args !== "object" || args === null) return NO_STEPS;
  const steps = (args as { steps?: unknown }).steps;
  if (!Array.isArray(steps) || steps.length === 0) return NO_STEPS;
  const projected: PlanStep[] = [];
  for (const [index, step] of steps.entries()) {
    if (typeof step !== "object" || step === null) continue;
    const { description, status } = step as { description?: unknown; status?: unknown };
    if (typeof description !== "string" || description.length === 0) continue;
    projected.push({
      id: String(index),
      text: description,
      status: TOOL_STEP_STATUS[String(status)] ?? "pending",
    });
  }
  return projected;
}

/**
 * The steps in a `set_plan` call's accumulated argument TEXT.
 *
 * The transcript carries arguments as the text they streamed as, so a preview
 * reading a settled call has to parse it. Unparseable text (a call still
 * streaming) is not an empty plan, but it renders as one — a half-written plan is
 * not a thing to draw, and the preview's own pending state covers the gap.
 */
export function planStepsFromToolArgs(args: string): readonly PlanStep[] {
  if (args === "") return NO_STEPS;
  try {
    return planStepsFromArguments(JSON.parse(args));
  } catch {
    return NO_STEPS;
  }
}

/**
 * The step a reader is watching: the one marked active, else the first not
 * started, else none — a finished plan has no step in progress.
 *
 * The mark outranks position. "First one not done" reads the same on the common
 * plan, but on a plan whose active step sits after an untouched one it names the
 * wrong step — and that is the plan where a reader most needs to be told.
 */
export function activePlanStep(steps: readonly PlanStep[]): PlanStep | undefined {
  return (
    steps.find((step) => step.status === "active") ??
    steps.find((step) => step.status === "pending")
  );
}

export function planProgress(steps: readonly PlanStep[]): { done: number; total: number } {
  return { done: steps.filter((step) => step.status === "done").length, total: steps.length };
}

/**
 * The active Session's exact Plan replacement.
 *
 * Memoised on Session identity and the snapshot object the fold swaps in, so a
 * reader keeps one stable rich model across unrelated renders.
 */
export function useSessionPlan(): SessionPlan {
  const sessionId = useActiveSessionId();
  const material = agentSessionView().usePlan();
  return useMemo(
    () => SessionPlan.fromSnapshot(sessionId, material.generation, material.value),
    [material, sessionId],
  );
}
