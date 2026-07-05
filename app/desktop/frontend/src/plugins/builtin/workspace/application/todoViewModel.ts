import { useSharedState } from "@/plugins/builtin/agent/public/sharedState";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export interface WorkspaceTodo {
  id: string;
  text: string;
  status: "completed" | "in_progress" | "pending";
}

export type WorkspaceTodosState = "unavailable" | "empty" | "ready";

export interface WorkspaceTodosViewModel {
  enabled: boolean;
  todos: readonly WorkspaceTodo[];
  done: number;
  total: number;
  state: WorkspaceTodosState;
}

export function useWorkspaceTodos(): WorkspaceTodosViewModel {
  const enabled = useWorkspaceCapability("todos");
  const todos = useSharedState<WorkspaceTodo[]>("todos") ?? [];
  return workspaceTodosViewModel(enabled, todos);
}

export function workspaceTodosViewModel(
  enabled: boolean,
  todos: readonly WorkspaceTodo[],
): WorkspaceTodosViewModel {
  let done = 0;
  for (const todo of todos) {
    if (todo.status === "completed") {
      done += 1;
    }
  }

  return {
    enabled,
    todos,
    done,
    total: todos.length,
    state: !enabled ? "unavailable" : todos.length === 0 ? "empty" : "ready",
  };
}

export function workspaceTodosSubtext({
  done,
  total,
}: Pick<WorkspaceTodosViewModel, "done" | "total">): string | undefined {
  if (total === 0) {
    return undefined;
  }
  return `${done} of ${total} done`;
}
