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
