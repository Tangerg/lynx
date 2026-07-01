import type { PlanItem } from "@/plugins/sdk/types/agentView";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// Plan-item check icon — shared between the inline PlanBlock and the
// promoted PlanList workspace view. Encodes the three states:
//   done  → accent check glyph
//   doing → accent spinner (pulse dot)
//   todo  → open circle outline
export function PlanCheck({ status }: { status: PlanItem["status"] }) {
  return (
    <div className="grid h-4 w-4 shrink-0 place-items-center">
      {status === "done" && <Icon name="check" size={14} className="text-accent" strokeWidth={3} />}
      {status === "doing" && (
        <div className="relative h-3 w-3 rounded-full border-[1.5px] border-accent">
          <div className="absolute inset-0.5 rounded-full bg-accent animate-pulse-dot" />
        </div>
      )}
      {status === "todo" && (
        <div className="h-3 w-3 rounded-full border-[1.5px] border-line-soft" />
      )}
    </div>
  );
}

// Class names for the surrounding plan-item text row, parameterised by
// status. Used by both PlanBlock and PlanList.
export const planItemRow = (status: PlanItem["status"]) =>
  cn(
    "flex items-center gap-2 text-[13px] py-0.5",
    status === "done" && "text-fg",
    status === "doing" && "text-fg font-medium",
    status === "todo" && "text-fg-muted",
  );
