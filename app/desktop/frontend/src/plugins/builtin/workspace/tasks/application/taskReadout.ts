import { useMemo } from "react";
import type { TaskReadoutTask } from "./ports/taskReadoutPort";
import { taskReadoutPort } from "./ports/taskReadoutPort";

export interface TaskReadout {
  tasks: TaskReadoutTask[];
  runningCount: number;
  head: TaskReadoutTask;
  label: string;
  title: string;
}

export function useTaskReadout(): TaskReadout | null {
  const tasks = taskReadoutPort().useTasks();
  return useMemo(() => taskReadout(tasks), [tasks]);
}

export function taskReadout(tasks: Map<string, TaskReadoutTask>): TaskReadout | null {
  if (tasks.size === 0) return null;

  const list = Array.from(tasks.values()).sort((a, b) => a.startedAt - b.startedAt);
  const running = list.filter((task) => task.status === "running");
  const head = running[0] ?? list.at(-1);
  if (!head) return null;

  return {
    tasks: list,
    runningCount: running.length,
    head,
    label: running.length > 1 ? `${head.label} +${running.length - 1}` : head.label,
    title: running.length > 0 ? `${running.length} running task(s)` : "Recent tasks",
  };
}

export function taskProgressPercent(task: TaskReadoutTask): number | null {
  if (task.progress === null || task.status === "failed") return null;
  return Math.round(Math.max(0, Math.min(1, task.progress)) * 100);
}
