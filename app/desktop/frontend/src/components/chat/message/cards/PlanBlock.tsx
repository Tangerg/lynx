import type { PlanItem } from "@/protocol/run/viewState";
import { memo } from "react";
import { PlanCheck, planItemRow } from "./PlanCheck";

// Plan block — shown when an assistant message describes a multi-step
// plan. Inline variant; the promoted workspace view uses PlanList. Both
// share the per-item check + row styling via ./PlanCheck.
export const PlanBlock = memo(function PlanBlock({ plan }: { plan: PlanItem[] }) {
  return (
    <div className="my-3" data-slot="plan-block">
      <div className="text-[13px] font-medium text-fg-muted mb-1.5">Plan</div>
      {plan.map((p) => (
        <div key={p.id} className={planItemRow(p.status)}>
          <PlanCheck status={p.status} />
          <div>{p.text}</div>
        </div>
      ))}
    </div>
  );
});
