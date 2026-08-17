import { getContainer } from "@/main/container";
import { asSessionId, createUnaryMutationSettler, type Goal, type SessionSnapshot } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { registerAgentSessionMaterialCommitter } from "@/plugins/builtin/agent/public/sessionMaterial";
import type {
  GoalCommandReceipt,
  GoalCommandsGateway,
} from "../application/ports/goalCommandsGateway";
import { configureGoalCommandsGateway } from "../application/ports/goalCommandsGateway";
import {
  GOAL_KEY,
  type GoalQuery,
  type GoalReadModel,
  type GoalState,
} from "../application/goalQueries";

const goalMutationSettler = createUnaryMutationSettler();

function goalMutationIdentity(method: "start" | "stop" | "resume", input: unknown): string {
  return JSON.stringify([`goals.${method}`, input]);
}

export function toGoalReadModel(goal: Goal): GoalReadModel {
  return {
    sessionId: goal.sessionId,
    objective: goal.objective,
    status: goal.status,
    stop: goal.reason ? { code: goal.reason.code, detail: goal.reason.detail ?? "" } : null,
    provider: goal.provider ?? "",
    model: goal.model ?? "",
    budget: {
      maxRuns: goal.budget.maxRuns ?? 0,
      maxCostUsd: goal.budget.maxCostUsd ?? 0,
      maxSteps: goal.budget.maxSteps ?? 0,
    },
    used: {
      runs: goal.used.runs,
      costUsd: goal.used.costUsd,
      steps: goal.used.steps,
    },
    createdAt: goal.createdAt,
    updatedAt: goal.updatedAt,
  };
}

export function toGoalCommandReceipt(goal: Pick<Goal, "sessionId">): GoalCommandReceipt {
  return { sessionId: goal.sessionId };
}

/** Commit the Goal carried by sessions.snapshot. That snapshot is the mounted
 * Session's transactionally coherent owner; this function keeps QueryClient
 * mechanics and Runtime DTO translation outside Agent Application. */
export function commitRuntimeGoalMaterial(
  sessionId: string,
  goal: Goal | undefined,
  available: boolean,
): void {
  queryClient.setQueryData<GoalState>([GOAL_KEY, { sessionId }], {
    available,
    goal: available && goal ? toGoalReadModel(goal) : null,
  });
}

const gateway: GoalCommandsGateway = {
  async start(input) {
    const client = getContainer().client();
    const goal = await goalMutationSettler.settle(goalMutationIdentity("start", input), (signal) =>
      client.goals.start({ ...input, sessionId: asSessionId(input.sessionId) }, signal),
    );
    return toGoalCommandReceipt(goal);
  },
  async stop(sessionId) {
    const client = getContainer().client();
    const goal = await goalMutationSettler.settle(
      goalMutationIdentity("stop", sessionId),
      (signal) => client.goals.stop(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  },
  async resume(sessionId) {
    const client = getContainer().client();
    const goal = await goalMutationSettler.settle(
      goalMutationIdentity("resume", sessionId),
      (signal) => client.goals.resume(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  },
};

export function installGoalRuntimeAdapter(ctx: Contributor): () => void {
  const disposeMaterialCommitter = registerAgentSessionMaterialCommitter<SessionSnapshot>(
    (sessionId, snapshot) => {
      const available = runtimeCapability("goals");
      return () => commitRuntimeGoalMaterial(sessionId, snapshot.goal, available);
    },
  );
  ctx.contribute(DATA_PROVIDER, {
    key: GOAL_KEY,
    fetcher: async (params, signal) => {
      const query = params as GoalQuery | undefined;
      if (!query) throw new Error(`Data provider "${GOAL_KEY}" requires parameters`);
      if (!runtimeCapability("goals")) {
        return { available: false, goal: null } satisfies GoalState;
      }
      const goal = await getContainer().client().goals.get(asSessionId(query.sessionId), signal);
      return {
        available: true,
        goal: goal ? toGoalReadModel(goal) : null,
      } satisfies GoalState;
    },
  });
  const disposeGateway = configureGoalCommandsGateway(gateway);
  return () => {
    disposeMaterialCommitter();
    disposeGateway();
  };
}
