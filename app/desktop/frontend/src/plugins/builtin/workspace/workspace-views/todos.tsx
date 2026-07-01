import { EmptyState } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useWorkspaceTodos } from "@/plugins/builtin/workspace/application/todoViewModel";
import { TodoList } from "./views/TodoList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

// The model's working checklist (B11, 613). Reads live from the agent's
// shared state — the backend pushes the list via state.snapshot{todos} (no new
// event type), which the fold already lands in view.shared. Gated by
// features.todos so a runtime without it shows an explicit "unavailable" state
// rather than a perpetually-empty tab.
function TodosTab() {
  const t = useT();
  const { done, enabled, todos } = useWorkspaceTodos();

  return (
    <WorkspaceViewLayout
      icon="check"
      titleStrong
      title="todos.title"
      sub={todos.length ? `${done} of ${todos.length} done` : undefined}
    >
      {!enabled ? (
        <EmptyState
          icon="check"
          title={t("todos.unavailable.title")}
          sub={t("todos.unavailable.sub")}
        />
      ) : todos.length === 0 ? (
        <EmptyState icon="check" title={t("todos.empty.title")} sub={t("todos.empty.sub")} />
      ) : (
        <TodoList todos={todos} />
      )}
    </WorkspaceViewLayout>
  );
}

export const todosView = defineWorkspaceView({
  id: "todos",
  title: "workspace.view.title.todos",
  icon: "check",
  openByDefault: false,
  order: 32,
  splittable: true,
  component: TodosTab,
});
