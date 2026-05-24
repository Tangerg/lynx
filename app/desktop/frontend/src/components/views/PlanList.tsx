import { Icon, PillButton } from "@/components/common";
import { PlanCheck, planItemRow } from "@/components/chat/PlanCheck";
import type { PlanItem } from "@/protocol/agui/viewState";

// Plan view workspace tab. Same per-item visual as the inline PlanBlock
// — both share ./PlanCheck for the check icon + row styling.
export function PlanList({ plan }: { plan: PlanItem[] }) {
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-fg-faint">
        Task plan
      </div>
      {plan.map((p) => (
        <div key={p.id} className={planItemRow(p.status)}>
          <PlanCheck status={p.status} />
          <div>{p.text}</div>
        </div>
      ))}
      <ApprovalNote />
    </div>
  );
}

function ApprovalNote() {
  return (
    <div className="mt-4 rounded-lg bg-surface px-3.5 py-3 text-[12px] leading-[1.5] text-fg-muted">
      <div className="mb-1.5 flex items-center gap-2">
        <Icon name="shield" size={13} className="text-warning" />
        <span className="font-mono text-[11.5px] font-bold uppercase tracking-[0.04em] text-fg">
          Approval required
        </span>
      </div>
      Agent will run{" "}
      <code className="rounded-xs bg-surface-2 px-1.5 py-px font-mono text-fg">
        pnpm test --filter=auth
      </code>{" "}
      after typecheck passes.
      <div className="mt-2.5 flex gap-1.5">
        <PillButton variant="accent" size="sm">
          Approve
        </PillButton>
        <PillButton size="sm">Skip</PillButton>
      </div>
    </div>
  );
}
