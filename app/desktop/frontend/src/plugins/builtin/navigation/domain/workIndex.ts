export type WorkSessionAttention = "running" | "waiting" | "none";

export interface WorkSession {
  id: string;
  revision: number;
  title: string;
  attention: WorkSessionAttention;
  favorite?: boolean;
  time: string;
}

export interface WorkProject {
  id: string;
  name: string;
  cwdMissing?: boolean;
}

export interface WorkGroup {
  project: WorkProject;
  sessions: WorkSession[];
}

export interface WorkIndex {
  groups: WorkGroup[] | undefined;
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  isError: boolean;
}
