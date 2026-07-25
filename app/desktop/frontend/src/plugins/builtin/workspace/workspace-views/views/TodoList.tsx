import { useT } from "@/lib/i18n";
import { StepRow, type StepState } from "@/ui";
import type { WorkspaceTodo } from "@/plugins/builtin/workspace/application/todoViewModel";

// TodoItem.status → the shared step vocabulary, so the agent's working checklist
// (B11) renders identically to a plan — same mark, same row.
const STEP_STATE: Record<WorkspaceTodo["status"], StepState> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

export function TodoList({ todos }: { todos: readonly WorkspaceTodo[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-ui-sm font-semibold text-fg-faint">
        {t("todos.list.heading")}
      </div>
      {todos.map((t) => {
        return (
          <StepRow key={t.id} state={STEP_STATE[t.status]}>
            {t.text}
          </StepRow>
        );
      })}
    </div>
  );
}
