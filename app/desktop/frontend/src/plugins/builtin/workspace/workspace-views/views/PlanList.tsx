import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { PlanCheck, planItemRow } from "@/components/chat/message";
import { useT } from "@/lib/i18n";

// Plan view workspace tab. Same per-item visual as the inline PlanBlock
// — both share ./PlanCheck for the check icon + row styling.
export function PlanList({ plan }: { plan: PlanItem[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-[11px] font-semibold text-fg-faint">
        {t("plan.list.heading")}
      </div>
      {plan.map((p) => (
        <div key={p.id} className={planItemRow(p.status)}>
          <PlanCheck status={p.status} />
          <div>{p.text}</div>
        </div>
      ))}
    </div>
  );
}
