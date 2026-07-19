export interface ScheduleConfig {
  id: string;
  title: string;
  prompt: string;
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
  prompt: string;
  cwd: string;
  cron: string;
}
