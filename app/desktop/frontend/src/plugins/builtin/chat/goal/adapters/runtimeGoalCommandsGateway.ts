import { getContainer } from "@/main/container";
import {
  asSessionId,
  createUnaryMutationSettler,
  type Goal,
  type LyraClient,
  type SessionSnapshot,
  type UnaryMutationSettler,
} from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { registerAgentSessionMaterialCommitter } from "@/plugins/builtin/agent/public/sessionMaterial";
import type {
  GoalCommandReceipt,
  GoalCommandsGateway,
} from "../application/ports/goalCommandsGateway";
import { GoalCommandOwner } from "../application/goalCommands";
import {
  GOAL_KEY,
  type GoalQuery,
  type GoalReadModel,
  type GoalState,
} from "../application/goalQueries";

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

class RuntimeGoalCommandsGateway implements GoalCommandsGateway {
  readonly #client: LyraClient;
  #mutations: UnaryMutationSettler = createUnaryMutationSettler();

  constructor(client: LyraClient) {
    this.#client = client;
  }

  async start(input: Parameters<GoalCommandsGateway["start"]>[0]) {
    const goal = await this.#mutations.settle(goalMutationIdentity("start", input), (signal) =>
      this.#client.goals.start({ ...input, sessionId: asSessionId(input.sessionId) }, signal),
    );
    return toGoalCommandReceipt(goal);
  }

  async stop(sessionId: string) {
    const goal = await this.#mutations.settle(goalMutationIdentity("stop", sessionId), (signal) =>
      this.#client.goals.stop(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  }

  async resume(sessionId: string) {
    const goal = await this.#mutations.settle(goalMutationIdentity("resume", sessionId), (signal) =>
      this.#client.goals.resume(asSessionId(sessionId), signal),
    );
    return toGoalCommandReceipt(goal);
  }

  dispose(): void {
    this.#mutations.dispose();
  }
}

export interface GoalRuntimeAdapterInstallation {
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

export function installGoalRuntimeAdapter(ctx: Contributor): GoalRuntimeAdapterInstallation {
  let gateway = new RuntimeGoalCommandsGateway(getContainer().client());
  const commandOwner = GoalCommandOwner.install(gateway);
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
  return {
    replaceRuntimeGeneration() {
      const successor = new RuntimeGoalCommandsGateway(getContainer().client());
      if (!commandOwner.replaceRuntimeGeneration(successor)) {
        successor.dispose();
        return;
      }
      const predecessor = gateway;
      gateway = successor;
      predecessor.dispose();
    },
    dispose() {
      disposeMaterialCommitter();
      commandOwner.dispose();
      gateway.dispose();
    },
  };
}
