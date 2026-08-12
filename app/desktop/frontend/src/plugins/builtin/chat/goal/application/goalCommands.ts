import { queryClient } from "@/lib/queryClient";
import { createKeyedSerialTaskQueue } from "@/lib/serialTaskQueue";
import { GOAL_KEY } from "./goalQueries";
import {
  goalCommandsGateway,
  type GoalCommandReceipt,
  type StartGoalInput,
} from "./ports/goalCommandsGateway";

function invalidateGoal(sessionId: string): Promise<void> {
  return queryClient
    .invalidateQueries({ queryKey: [GOAL_KEY, { sessionId }], exact: true })
    .then(() => undefined);
}

const goalMutations = createKeyedSerialTaskQueue<string>();

export class GoalCommandSessionMismatchError extends Error {
  constructor(
    readonly expectedSessionId: string,
    readonly actualSessionId: string,
  ) {
    super();
    this.name = "GoalCommandSessionMismatchError";
  }
}

async function mutateGoal(
  sessionId: string,
  command: () => Promise<GoalCommandReceipt>,
): Promise<void> {
  await goalMutations.run(sessionId, async () => {
    try {
      const committed = await command();
      if (committed.sessionId !== sessionId) {
        throw new GoalCommandSessionMismatchError(sessionId, committed.sessionId);
      }
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
    // Mutation responses are point-in-time acknowledgements, not the standing
    // Goal read model. Only goals.get may write that model: a delayed response can
    // otherwise overwrite a newer autonomous or remote transition, and updatedAt
    // cannot establish authority across equal timestamps or a clock correction.
    // Keep the next local command behind this read so intent order and projection
    // order have one settlement boundary.
    await invalidateGoal(sessionId);
  });
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
