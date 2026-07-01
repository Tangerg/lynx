import type { TodoItem } from "@/rpc";
import { useServerFeature } from "@/state/runtimeStore";
import { useSharedState } from "@/plugins/builtin/agent/public/sharedState";

export interface WorkspaceTodo {
  id: string;
  text: string;
  status: "completed" | "in_progress" | "pending";
}

export function useWorkspaceTodos() {
  const enabled = useServerFeature("todos");
  const todos = useSharedState<TodoItem[]>("todos") ?? [];
  return {
    enabled,
    todos,
    done: todos.filter((todo) => todo.status === "completed").length,
  };
}
