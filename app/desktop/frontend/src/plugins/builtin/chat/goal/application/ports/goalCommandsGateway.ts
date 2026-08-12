import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { GoalReadModel } from "../goalQueries";

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
  start(input: StartGoalInput): Promise<GoalReadModel>;
  stop(sessionId: string): Promise<GoalReadModel>;
  resume(sessionId: string): Promise<GoalReadModel>;
}

const port = createSingletonPort<GoalCommandsGateway>("Goal commands gateway is not configured");

export const configureGoalCommandsGateway = port.configure;
export const goalCommandsGateway = port.get;
