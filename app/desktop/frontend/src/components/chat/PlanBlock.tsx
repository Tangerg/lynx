import { Icon } from "@/components/common";
import type { PlanItem } from "@/protocol/agui/viewState";

export function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div className="plan-block">
      <div className="plan-head">
        <Icon name="list" size={12} />
        Plan · {done} of {plan.length} complete
      </div>
      {plan.map((p) => (
        <div key={p.id} className={`plan-item ${p.status}`}>
          <div className="check">
            {p.status === "done" && <Icon name="check" size={12} strokeWidth={3} />}
          </div>
          <div>{p.text}</div>
        </div>
      ))}
    </div>
  );
}
