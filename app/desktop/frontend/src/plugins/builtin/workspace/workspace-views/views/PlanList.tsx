import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { StepRow, type StepState } from "@/ui";
import { useT } from "@/lib/i18n";

// Plan view workspace tab. Same per-item visual as the inline PlanBlock
// — both share the agent plan presentation contract.
// The agent's plan vocabulary → the row's. The row is shared with the plan view
// and the working checklist, so it speaks in step states, not plan statuses.
const STEP_STATE: Record<PlanItem["status"], StepState> = {
  done: "done",
  doing: "active",
  todo: "pending",
};

export function PlanList({ plan }: { plan: readonly PlanItem[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-ui-sm font-semibold text-fg-faint">
        {t("plan.list.heading")}
      </div>
      {plan.map((p) => (
        <StepRow key={p.id} state={STEP_STATE[p.status]}>
          {p.text}
        </StepRow>
      ))}
    </div>
  );
}
