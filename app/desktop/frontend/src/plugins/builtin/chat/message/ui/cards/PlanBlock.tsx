import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { StepRow, type StepState } from "@/ui";
import { memo } from "react";
import { useT } from "@/lib/i18n";

// Plan block — shown when an assistant message describes a multi-step plan.
// Inline variant; the promoted workspace view uses PlanList. Both share the
// per-item check + row styling from the agent presentation contract. Rendered
// flush on the canvas (no card): a quiet title row + status-icon step list, so
// progress reads as part of the prose rather than a boxed widget.
// The agent's plan vocabulary → the row's. The row is shared with the plan view
// and the working checklist, so it speaks in step states, not plan statuses.
const STEP_STATE: Record<PlanItem["status"], StepState> = {
  done: "done",
  doing: "active",
  todo: "pending",
};

export const PlanBlock = memo(function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const t = useT();
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div className="my-3 flex flex-col gap-1" data-slot="plan-block">
      <div className="flex items-center justify-between gap-2">
        <span className="text-ui-lg font-medium text-fg">{t("plan.title")}</span>
        <span className="font-mono text-ui-sm tabular-nums text-fg-faint">
          {done}/{plan.length}
        </span>
      </div>
      <div className="flex flex-col gap-0.5">
        {plan.map((p) => (
          <StepRow key={p.id} state={STEP_STATE[p.status]}>
            {p.text}
          </StepRow>
        ))}
      </div>
    </div>
  );
});
