export interface ScheduleConfig {
  id: string;
  title: string;
  instructions: string;
  cwd?: string;
  cron: string;
  enabled: boolean;
  provider?: string;
  model?: string;
  createdAt?: string;
  nextRunAt?: string;
  lastRunAt?: string;
  revision: number;
}

export interface ScheduleConfigInput {
  title: string;
  instructions: string;
  cwd: string;
  cron: string;
}

export interface ScheduledRunIdentity {
  sessionId: string;
  runId: string;
}
