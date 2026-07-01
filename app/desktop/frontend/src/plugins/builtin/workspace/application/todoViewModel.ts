import { useSharedState } from "@/plugins/builtin/agent/public/sharedState";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export interface WorkspaceTodo {
  id: string;
  text: string;
  status: "completed" | "in_progress" | "pending";
}

export function useWorkspaceTodos() {
  const enabled = useWorkspaceCapability("todos");
  const todos = useSharedState<WorkspaceTodo[]>("todos") ?? [];
  return {
    enabled,
    todos,
    done: todos.filter((todo) => todo.status === "completed").length,
  };
}
