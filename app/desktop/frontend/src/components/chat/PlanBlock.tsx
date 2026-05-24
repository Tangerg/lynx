import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import type { PlanItem } from "@/protocol/agui/viewState";

// Plan block — shown when an assistant message describes a multi-step
// plan. Each item is `todo` → `doing` → `done` with a custom check
// state. The `doing` state's inner square pulses to show forward
// motion (`animate-pulse-dot` keyframe from globals.css).
export function PlanBlock({ plan }: { plan: PlanItem[] }) {
  const done = plan.filter((p) => p.status === "done").length;
  return (
    <div className="rounded-lg border border-line-soft bg-transparent px-3.5 py-2.5 my-2">
      <div className="mb-2.5 flex items-center gap-2 font-mono text-[11px] font-semibold text-fg-faint tabular-nums">
        <Icon name="list" size={12} />
        Plan · {done} of {plan.length} complete
      </div>
      {plan.map((p) => (
        <div
          key={p.id}
          className={cn(
            "grid grid-cols-[18px_1fr] items-start gap-2.5 py-1 text-[13.5px] leading-[1.45]",
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
    </div>
  );
}
