import { queryClient } from "@/lib/queryClient";
import { createKeyedSerialTaskQueue } from "@/lib/serialTaskQueue";
import { GOAL_KEY, type GoalReadModel, type GoalState } from "./goalQueries";
import { goalCommandsGateway, type StartGoalInput } from "./ports/goalCommandsGateway";

function invalidateGoal(sessionId: string): Promise<void> {
  return queryClient
    .invalidateQueries({ queryKey: [GOAL_KEY, { sessionId }], exact: true })
    .then(() => undefined);
}

const goalMutations = createKeyedSerialTaskQueue<string>();

function fractionalNanosecondsAfterMillis(timestamp: string): number {
  const fraction = /\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/.exec(timestamp)?.[1] ?? "";
  return Number((fraction.slice(3, 9) + "000000").slice(0, 6));
}

function isLaterTimestamp(held: string, arriving: string): boolean {
  const heldMillis = Date.parse(held);
  const arrivingMillis = Date.parse(arriving);
  if (!Number.isFinite(heldMillis) || !Number.isFinite(arrivingMillis)) return held > arriving;
  if (heldMillis !== arrivingMillis) return heldMillis > arrivingMillis;
  return fractionalNanosecondsAfterMillis(held) > fractionalNanosecondsAfterMillis(arriving);
}

function commitGoal(goal: GoalReadModel): void {
  const queryKey = [GOAL_KEY, { sessionId: goal.sessionId }] as const;
  queryClient.setQueryData<GoalState>(queryKey, (current) => {
    const held = current?.goal;
    if (held && isLaterTimestamp(held.updatedAt, goal.updatedAt)) return current;
    return { available: true, goal };
  });
}

async function mutateGoal(sessionId: string, command: () => Promise<GoalReadModel>): Promise<void> {
  await goalMutations.run(sessionId, async () => {
    try {
      commitGoal(await command());
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
    // Revalidation is part of the serialized lifecycle settlement. Starting the
    // next command while this read is still in flight could let an intermediate
    // projection race the newer command response.
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
