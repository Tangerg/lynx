import type { Translate } from "@/lib/i18n";
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
  // The shared state holds the whole snapshot — revision included — because that is
  // what says which of two deliveries is later. This view reads only the list, in its
  // own language: the runtime's wire type does not belong in a workspace view model.
  const snapshot = useSharedState<{ todos?: readonly WorkspaceTodo[] }>("todos");
  return workspaceTodosViewModel(enabled, snapshot?.todos ?? []);
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

export function workspaceTodosSubtext(
  t: Translate,
  { done, total }: Pick<WorkspaceTodosViewModel, "done" | "total">,
): string | undefined {
  if (total === 0) {
    return undefined;
  }
  return t("todos.progress", { done, total });
}
