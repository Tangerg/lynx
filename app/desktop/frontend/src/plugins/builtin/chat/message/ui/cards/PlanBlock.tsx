import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { memo } from "react";
import { useT } from "@/lib/i18n";
import { PlanCheck, planItemRow } from "@/plugins/builtin/agent/public/planPresentation";

// Plan block — shown when an assistant message describes a multi-step plan.
// Inline variant; the promoted workspace view uses PlanList. Both share the
// per-item check + row styling from the agent presentation contract. Rendered
// as a lifted surface card (header + step rows) so multi-step progress reads as
// one structured unit in the stream.
export const PlanBlock = memo(function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const t = useT();
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div
      className="my-3 overflow-hidden rounded-lg border border-line-soft bg-surface"
      data-slot="plan-block"
    >
      <div className="flex items-center justify-between gap-2 border-b border-line-soft px-3.5 py-2.5">
        <span className="text-[13px] font-medium text-fg">{t("plan.title")}</span>
        <span className="font-mono text-[11.5px] tabular-nums text-fg-faint">
          {done}/{plan.length}
        </span>
      </div>
      <div className="flex flex-col gap-0.5 px-3.5 py-2.5">
        {plan.map((p) => (
          <div key={p.id} className={planItemRow(p.status)}>
            <PlanCheck status={p.status} />
            <span className="min-w-0 flex-1">{p.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
});
