import { queryClient } from "@/lib/queryClient";
import { GOAL_KEY } from "./goalQueries";
import { goalCommandsGateway, type StartGoalInput } from "./ports/goalCommandsGateway";

function invalidateGoal(sessionId: string): Promise<void> {
  return queryClient
    .invalidateQueries({ queryKey: [GOAL_KEY, { sessionId }] })
    .then(() => undefined);
}

async function mutateGoal(sessionId: string, command: () => Promise<void>): Promise<void> {
  try {
    await command();
  } catch (error) {
    // A command rejection may be a revision/admission race with another client,
    // or a response lost after Runtime committed. Preserve the command error for
    // its caller while independently converging the standing Goal read model.
    try {
      await invalidateGoal(sessionId);
    } catch {
      // Revalidation has its own query lifecycle. It must neither replace the
      // command failure nor escape as a second unhandled rejection.
    }
    throw error;
  }
  await invalidateGoal(sessionId);
}

export async function startGoal(input: StartGoalInput): Promise<void> {
  await mutateGoal(input.sessionId, () => goalCommandsGateway().start(input));
}

export async function stopGoal(sessionId: string): Promise<void> {
  await mutateGoal(sessionId, () => goalCommandsGateway().stop(sessionId));
}

export async function resumeGoal(sessionId: string): Promise<void> {
  await mutateGoal(sessionId, () => goalCommandsGateway().resume(sessionId));
}
