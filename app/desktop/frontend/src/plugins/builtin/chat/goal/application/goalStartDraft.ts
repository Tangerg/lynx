import type { GoalCommandBudget } from "./ports/goalCommandsGateway";

export interface GoalStartDraft {
  objective: string;
  maxRuns: string;
  maxCostUsd: string;
  maxSteps: string;
}

export type GoalStartDraftField = keyof GoalStartDraft;

export type ParseGoalStartDraftResult =
  | { ok: true; objective: string; budget?: GoalCommandBudget }
  | { ok: false; field: GoalStartDraftField };

function optionalLimit(value: string, integer: boolean): number | undefined | null {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) && parsed > 0 && (!integer || Number.isInteger(parsed))
    ? parsed
    : null;
}

export function parseGoalStartDraft(draft: GoalStartDraft): ParseGoalStartDraftResult {
  const objective = draft.objective.trim();
  if (!objective) return { ok: false, field: "objective" };

  const budget: GoalCommandBudget = {};
  for (const field of ["maxRuns", "maxCostUsd", "maxSteps"] as const) {
    const limit = optionalLimit(draft[field], field !== "maxCostUsd");
    if (limit === null) return { ok: false, field };
    if (limit !== undefined) budget[field] = limit;
  }

  return Object.keys(budget).length > 0 ? { ok: true, objective, budget } : { ok: true, objective };
}
