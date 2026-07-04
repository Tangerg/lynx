export type TaskReadoutStatus = "running" | "succeeded" | "failed";

export interface TaskReadoutTask {
  id: string;
  label: string;
  progress: number | null;
  message: string | null;
  status: TaskReadoutStatus;
  error?: string;
  startedAt: number;
}

export interface TaskReadoutPort {
  useTasks(): Map<string, TaskReadoutTask>;
}

let port: TaskReadoutPort | null = null;

export function configureTaskReadoutPort(next: TaskReadoutPort): void {
  port = next;
}

export function taskReadoutPort(): TaskReadoutPort {
  if (!port) throw new Error("task readout port is not configured");
  return port;
}
