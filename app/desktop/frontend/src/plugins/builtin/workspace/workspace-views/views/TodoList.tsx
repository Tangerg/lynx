import { PlanCheck, planItemRow } from "@/plugins/builtin/agent/public/planPresentation";
import { useT } from "@/lib/i18n";
import type { WorkspaceTodo } from "@/plugins/builtin/workspace/application/todoViewModel";

// TodoItem.status → the plan-row visual vocabulary. PlanCheck / planItemRow are
// shared with the Plan view + inline PlanBlock, so the agent's working checklist
// (B11) renders identically to a plan — same check glyph, same row styling.
const TODO_STATUS = {
  completed: "done",
  in_progress: "doing",
  pending: "todo",
} as const;

export function TodoList({ todos }: { todos: WorkspaceTodo[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-[11px] font-semibold text-fg-faint">
        {t("todos.list.heading")}
      </div>
      {todos.map((t) => {
        const status = TODO_STATUS[t.status];
        return (
          <div key={t.id} className={planItemRow(status)}>
            <PlanCheck status={status} />
            <div>{t.text}</div>
          </div>
        );
      })}
    </div>
  );
}
