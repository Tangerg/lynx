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
  reasoningEffort?: string;
  budget?: GoalCommandBudget;
}

export interface UpdateGoalInput {
  sessionId: string;
  objective: string;
}

/** Correlates a committed Goal lifecycle command with the Session it addressed.
 * The standing Goal projection is deliberately absent: only the mounted
 * sessions.snapshot material boundary owns that state. */
export interface GoalCommandReceipt {
  sessionId: string;
}

export interface GoalCommandsGateway {
  start(input: StartGoalInput): Promise<GoalCommandReceipt>;
  update(input: UpdateGoalInput): Promise<GoalCommandReceipt>;
  clear(sessionId: string): Promise<GoalCommandReceipt>;
  stop(sessionId: string): Promise<GoalCommandReceipt>;
  resume(sessionId: string): Promise<GoalCommandReceipt>;
}
