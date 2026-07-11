import { useTasksStore } from "@/state/tasksStore";
import { configureTaskReadoutPort } from "../application/ports/taskReadoutPort";

export function installTaskReadoutPort(): () => void {
  return configureTaskReadoutPort({
    useTasks: () => useTasksStore((state) => state.tasks),
  });
}
