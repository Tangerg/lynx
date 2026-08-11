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

export interface GoalCommandsGateway {
  start(input: StartGoalInput): Promise<void>;
  stop(sessionId: string): Promise<void>;
  resume(sessionId: string): Promise<void>;
}

const port = createSingletonPort<GoalCommandsGateway>("Goal commands gateway is not configured");

export const configureGoalCommandsGateway = port.configure;
export const goalCommandsGateway = port.get;
