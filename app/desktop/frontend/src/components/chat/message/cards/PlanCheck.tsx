import type { PlanItem } from "@/protocol/run/viewState";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// Plan-item check icon — shared between the inline PlanBlock and the
// promoted PlanList workspace view. Encodes the three states:
//   done  → solid accent square + check glyph
//   doing → outlined accent ring + pulsing inner square
//   todo  → outlined faint ring (empty)
export function PlanCheck({ status }: { status: PlanItem["status"] }) {
  return (
    <div
      className={cn(
        "mt-px grid h-4.5 w-4.5 shrink-0 place-items-center rounded",
        status === "done" && "bg-accent text-on-accent",
        status === "doing" &&
          "border-[1.5px] border-accent relative " +
            "after:content-[''] after:h-2 after:w-2 after:rounded-[2px] after:bg-accent after:animate-pulse-dot",
        status === "todo" && "border-[1.5px] border-line-soft",
      )}
    >
      {status === "done" && <Icon name="check" size={12} strokeWidth={3} />}
    </div>
  );
}

// Class names for the surrounding plan-item text row, parameterised by
// status. Used by both PlanBlock and PlanList.
export const planItemRow = (status: PlanItem["status"]) =>
  cn(
    "grid grid-cols-[18px_1fr] items-start gap-2.5 py-1 text-[13px] leading-[1.45]",
    status === "done" && "text-fg-faint line-through decoration-line-soft",
    status === "doing" && "text-fg font-semibold",
    status === "todo" && "text-fg-soft",
  );
