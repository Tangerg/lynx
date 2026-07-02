export type WorkSessionStatus = "running" | "waiting" | "idle";

export interface WorkSession {
  id: string;
  title: string;
  status: WorkSessionStatus;
  model: string;
  cwd?: string;
  cwdMissing?: boolean;
  usage?: { inputTokens?: number; outputTokens?: number; costUsd?: number };
  favorite?: boolean;
  time: string;
}

export interface WorkProject {
  id: string;
  name: string;
  branch: string;
  sessionCount: number;
  cwdMissing?: boolean;
}

export interface WorkGroup {
  project: WorkProject;
  sessions: WorkSession[];
}

export interface WorkIndex {
  groups: WorkGroup[] | undefined;
  recentSessions: WorkSession[];
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  isError: boolean;
}
