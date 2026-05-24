import { Icon, PillButton } from "@/components/common";
import { cn } from "@/lib/utils";
import type { PlanItem } from "@/protocol/agui/viewState";

// Plan view workspace tab. Same item visual as the inline PlanBlock —
// todo / doing / done with custom check states.
export function PlanList({ plan }: { plan: PlanItem[] }) {
  return (
    <div className="px-4.5 py-3.5">
      <div className="mb-3 font-mono text-[10.5px] font-bold uppercase tracking-[0.14em] text-fg-faint">
        Task plan
      </div>
      {plan.map((p) => (
        <div
          key={p.id}
          className={cn(
            "grid grid-cols-[18px_1fr] items-start gap-2.5 py-2 text-[13.5px] leading-[1.45]",
            p.status === "done" && "text-fg-faint line-through decoration-line-soft",
            p.status === "doing" && "text-fg font-semibold",
            p.status === "todo" && "text-fg-soft",
          )}
        >
          <div
            className={cn(
              "mt-px grid h-[18px] w-[18px] shrink-0 place-items-center rounded",
              p.status === "done" && "bg-accent text-on-accent",
              p.status === "doing" &&
                "border-[1.5px] border-accent relative " +
                "after:content-[''] after:h-2 after:w-2 after:rounded-[2px] after:bg-accent after:animate-pulse-dot",
              p.status === "todo" && "border-[1.5px] border-line-soft",
            )}
          >
            {p.status === "done" && <Icon name="check" size={12} strokeWidth={3} />}
          </div>
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
        <PillButton variant="accent" size="sm">Approve</PillButton>
        <PillButton size="sm">Skip</PillButton>
      </div>
    </div>
  );
}
