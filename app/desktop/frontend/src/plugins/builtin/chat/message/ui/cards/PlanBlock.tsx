import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { Icon, ProgressBar, StepRow, type StepState } from "@/ui";
import { memo } from "react";
import { useT } from "@/lib/i18n";

// Plan block — shown when an assistant message describes a multi-step plan.
// Inline variant; the promoted workspace view uses PlanList. Both share the
// per-item check + row styling from the agent presentation contract.
//
// A card, not flush prose: the plan is the one block in a turn that stays true
// after you have read past it — it is the answer to "where is this going", and a
// reader scrolling back for that should find an object, not a paragraph. The
// header carries the count twice, as a fraction and as a bar, because the
// fraction answers "how many" and the bar answers "how far" at a glance.
const STEP_STATE: Record<PlanItem["status"], StepState> = {
  done: "done",
  doing: "active",
  todo: "pending",
};

export const PlanBlock = memo(function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const t = useT();
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div className="my-3 overflow-hidden rounded-[var(--surface-card-radius)] bg-card">
      <div className="flex items-center gap-2.5 px-3 py-2.5">
        <Icon name="list" size="sm" className="shrink-0 text-fg-muted" />
        <span className="shrink-0 text-ui-sm font-semibold text-fg">{t("plan.title")}</span>
        <span className="shrink-0 font-mono text-ui-xs tabular-nums text-fg-faint">
          {done}/{plan.length}
        </span>
        <span className="min-w-4 flex-1" />
        {plan.length > 0 && (
          <ProgressBar value={(done / plan.length) * 100} className="w-24 shrink-0" />
        )}
      </div>
      <div className="flex flex-col gap-1 px-3.5 pb-3">
        {plan.map((p) => (
          <StepRow key={p.id} state={STEP_STATE[p.status]}>
            {p.text}
          </StepRow>
        ))}
      </div>
    </div>
  );
});
