import { getContainer } from "@/main/container";
import {
  asSessionId,
  createUnaryMutationSettler,
  type Goal,
  type LyraClient,
  type SessionSnapshot,
  type UnaryMutationSettler,
} from "@/rpc";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { synchronizeMountedAgentSession } from "@/plugins/builtin/agent/public/session";
import { registerAgentSessionSharedMaterial } from "@/plugins/builtin/agent/public/sessionMaterial";
import type {
  GoalCommandReceipt,
  GoalCommandsGateway,
} from "../application/ports/goalCommandsGateway";
import { GoalCommandOwner } from "../application/goalCommands";
import type { GoalReadModel, GoalState } from "../application/goalReadModel";

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

/** Translate the Goal carried by sessions.snapshot before it joins the mounted
 * Session's single transactionally coherent material projection. */
export function runtimeGoalMaterial(goal: Goal | undefined, available: boolean): GoalState {
  return {
    available,
    goal: available && goal ? toGoalReadModel(goal) : null,
  };
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
  retireRuntimeGeneration(): void;
  dispose(): void;
}

export function installGoalRuntimeAdapter(
  hasRuntimeGeneration: boolean,
): GoalRuntimeAdapterInstallation {
  let gateway = hasRuntimeGeneration
    ? new RuntimeGoalCommandsGateway(getContainer().client())
    : null;
  const commandOwner = GoalCommandOwner.install(gateway, synchronizeMountedAgentSession);
  const disposeSharedMaterial = registerAgentSessionSharedMaterial<SessionSnapshot>(
    "goal",
    (_sessionId, snapshot) => {
      return runtimeGoalMaterial(snapshot.goal, runtimeCapability("goals"));
    },
  );
  return {
    replaceRuntimeGeneration() {
      const successor = new RuntimeGoalCommandsGateway(getContainer().client());
      if (!commandOwner.replaceRuntimeGeneration(successor)) {
        successor.dispose();
        return;
      }
      const predecessor = gateway;
      gateway = successor;
      predecessor?.dispose();
    },
    retireRuntimeGeneration() {
      if (!commandOwner.retireRuntimeGeneration()) return;
      const predecessor = gateway;
      gateway = null;
      predecessor?.dispose();
    },
    dispose() {
      disposeSharedMaterial();
      commandOwner.dispose();
      gateway?.dispose();
      gateway = null;
    },
  };
}
