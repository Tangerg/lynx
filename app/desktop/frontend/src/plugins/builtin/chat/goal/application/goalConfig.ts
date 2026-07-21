import { useEffect } from "react";
import { queryClient } from "@/lib/data/queryClient";
import { subscribeAgentRunSettlements } from "@/plugins/builtin/agent/public/run";
import { goalGateway, type GoalStartInput } from "./ports/goalGateway";
import { GOAL_KEY, useGoalStateQuery } from "./goalData";

export function useGoal(enabled: boolean, sessionId: string | undefined) {
  return useGoalStateQuery(enabled && sessionId ? { sessionId } : undefined);
}

async function invalidate(): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: [GOAL_KEY] });
}

// The autonomous loop advances the goal's budget usage each turn (= each run),
// and there is no goal push channel — so refetch whenever any run settles. Only
// armed while a goal is actually driving, to avoid a listener on idle sessions.
export function useGoalLiveRefetch(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    return subscribeAgentRunSettlements(() => void invalidate());
  }, [active]);
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
