import { queryClient } from "@/lib/queryClient";
import { GOAL_KEY } from "./goalQueries";
import { goalCommandsGateway, type StartGoalInput } from "./ports/goalCommandsGateway";

function invalidateGoal(sessionId: string): Promise<void> {
  return queryClient
    .invalidateQueries({ queryKey: [GOAL_KEY, { sessionId }] })
    .then(() => undefined);
}

export async function startGoal(input: StartGoalInput): Promise<void> {
  await goalCommandsGateway().start(input);
  await invalidateGoal(input.sessionId);
}

export async function stopGoal(sessionId: string): Promise<void> {
  await goalCommandsGateway().stop(sessionId);
  await invalidateGoal(sessionId);
}

export async function resumeGoal(sessionId: string): Promise<void> {
  await goalCommandsGateway().resume(sessionId);
  await invalidateGoal(sessionId);
}
