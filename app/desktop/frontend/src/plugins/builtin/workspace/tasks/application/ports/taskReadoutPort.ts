import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<TaskReadoutPort>("task readout port is not configured");

export const configureTaskReadoutPort = port.configure;
export const taskReadoutPort = port.get;
