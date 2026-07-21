import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface GoalStartInput {
  sessionId: string;
  objective: string;
  budget?: { maxTurns?: number; maxCostUsd?: number; maxSteps?: number };
}

// GoalGateway drives Goal mode for a session: start an autonomous loop toward an
// objective, stop (pause) it, or resume a paused/blocked goal. The runtime
// adapter drives goals.* over RPC.
export interface GoalGateway {
  start(input: GoalStartInput): Promise<void>;
  stop(sessionId: string): Promise<void>;
  resume(sessionId: string): Promise<void>;
}

const port = createSingletonPort<GoalGateway>("Goal gateway is not configured");

export const configureGoalGateway = port.configure;
export const goalGateway = port.get;
