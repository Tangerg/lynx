import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface GoalCommandBudget {
  maxRuns?: number;
  maxCostUsd?: number;
  maxSteps?: number;
}

export interface StartGoalInput {
  sessionId: string;
  objective: string;
  provider?: string;
  model?: string;
  budget?: GoalCommandBudget;
}

/** Correlates a committed Goal lifecycle command with the Session it addressed.
 * The standing Goal projection is deliberately absent: only the goals.get read
 * boundary owns that state. */
export interface GoalCommandReceipt {
  sessionId: string;
}

export interface GoalCommandsGateway {
  start(input: StartGoalInput): Promise<GoalCommandReceipt>;
  stop(sessionId: string): Promise<GoalCommandReceipt>;
  resume(sessionId: string): Promise<GoalCommandReceipt>;
}

const port = createSingletonPort<GoalCommandsGateway>("Goal commands gateway is not configured");

export const configureGoalCommandsGateway = port.configure;
export const goalCommandsGateway = port.get;
