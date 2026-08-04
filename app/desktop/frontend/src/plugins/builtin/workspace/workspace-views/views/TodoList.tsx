import { useT } from "@/lib/i18n";
import { SectionLabel, StepRow, type StepState } from "@/ui";
import type { WorkspaceTodo } from "@/plugins/builtin/workspace/application/todoViewModel";

// TodoSnapshot.status → the shared step vocabulary, so the agent's working checklist
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
      <SectionLabel className="px-0 pt-0">{t("todos.list.heading")}</SectionLabel>
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
