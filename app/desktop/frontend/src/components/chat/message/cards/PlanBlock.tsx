import type { PlanItem } from "@/protocol/run/viewState";
import { memo } from "react";
import { Icon } from "@/components/common";
import { PlanCheck, planItemRow } from "./PlanCheck";

// Plan block — shown when an assistant message describes a multi-step
// plan. Inline variant; the promoted workspace view uses PlanList. Both
// share the per-item check + row styling via ./PlanCheck.
export const PlanBlock = memo(function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div className="rounded-lg border border-line-soft bg-transparent px-3.5 py-2.5 my-2">
      <div className="mb-2.5 flex items-center gap-2 font-mono text-[11px] font-semibold text-fg-faint">
        <Icon name="list" size={12} />
        Plan · {done} of {plan.length} complete
      </div>
      {plan.map((p) => (
        <div key={p.id} className={planItemRow(p.status)}>
          <PlanCheck status={p.status} />
          <div>{p.text}</div>
        </div>
      ))}
    </div>
  );
});
