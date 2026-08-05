import { useMemo } from "react";
import type { PlanSnapshot } from "@/rpc";
import { agentSessionView } from "../ports/sessionView";

/**
 * One step of the session's plan, in this context's own words.
 *
 * The plan is a session-scoped state snapshot written only by the root Run
 * (`plan.get` / `state.snapshot{plan}`), not a transcript Item — so it has no run
 * of its own and nothing about it is per-turn.
 */
export interface PlanStep {
  id: string;
  text: string;
  /** The checklist vocabulary, which is also the step row's. Translated once
   *  here so no surface renders the wire's `in_progress`. */
  status: "done" | "active" | "pending";
}

const STEP_STATUS: Record<PlanSnapshot["status"], PlanStep["status"]> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

const NO_STEPS: PlanStep[] = [];

/** The snapshot the fold lands under `shared.plan`. Only the list is read here:
 *  `revision` decides which of two deliveries is later, which is the fold's
 *  business, not a reader's. */
interface SharedPlan {
  plan?: readonly PlanSnapshot[];
}

export function planSteps(snapshot: SharedPlan | undefined): PlanStep[] {
  const steps = snapshot?.plan;
  if (!steps || steps.length === 0) return NO_STEPS;
  return steps.map((step) => ({
    id: step.id,
    text: step.description,
    status: STEP_STATUS[step.status],
  }));
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
export function planStepsFromArguments(args: unknown): PlanStep[] {
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
      status: STEP_STATUS[status as PlanSnapshot["status"]] ?? "pending",
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
export function planStepsFromToolArgs(args: string): PlanStep[] {
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
 * The active session's plan.
 *
 * Memoised on the snapshot object the fold swaps in, so a reader that feeds a
 * render context (ChatStream's ctx, a `memo` boundary) keeps a stable array
 * across unrelated renders.
 */
export function useSessionPlan(): PlanStep[] {
  const snapshot = agentSessionView().useSharedState<SharedPlan>("plan");
  return useMemo(() => planSteps(snapshot), [snapshot]);
}
