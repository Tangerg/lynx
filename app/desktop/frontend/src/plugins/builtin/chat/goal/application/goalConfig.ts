import { queryClient } from "@/lib/data/queryClient";
import { goalGateway, type GoalStartInput } from "./ports/goalGateway";
import { GOAL_KEY, useGoalStateQuery } from "./goalData";

export function useGoal(enabled: boolean, sessionId: string | undefined) {
  return useGoalStateQuery(enabled && sessionId ? { sessionId } : undefined);
}

// A goal's live budget advances via server-launched runs the client can't
// observe, so the query polls while active (see goalData). Mutations invalidate
// so start/stop/resume reflect immediately without waiting for the poll tick.
async function invalidate(): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: [GOAL_KEY] });
}

export async function startGoal(input: GoalStartInput): Promise<void> {
  await goalGateway().start(input);
  await invalidate();
}

export async function stopGoal(sessionId: string): Promise<void> {
  await goalGateway().stop(sessionId);
  await invalidate();
}

export async function resumeGoal(sessionId: string): Promise<void> {
  await goalGateway().resume(sessionId);
  await invalidate();
}
