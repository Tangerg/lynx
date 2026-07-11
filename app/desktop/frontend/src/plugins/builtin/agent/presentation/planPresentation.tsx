import type { PlanItem } from "@/plugins/sdk/types/agentView";
import { Icon } from "@/ui";
import { cn } from "@/lib/utils";

export function PlanCheck({ status }: { status: PlanItem["status"] }) {
  return (
    <div className="grid h-4 w-4 shrink-0 place-items-center">
      {status === "done" && (
        <Icon name="check" size={14} className="text-success" strokeWidth={3} />
      )}
      {status === "doing" && (
        <div className="relative h-3 w-3 rounded-full border-[1.5px] border-accent">
          <div className="absolute inset-0.5 rounded-full bg-accent animate-pulse-dot" />
        </div>
      )}
      {status === "todo" && (
        <div className="h-3 w-3 rounded-full border-[1.5px] border-fg-faint/40" />
      )}
    </div>
  );
}

export const planItemRow = (status: PlanItem["status"]) =>
  cn(
    "flex items-center gap-2 text-[13px] py-0.5",
    status === "done" && "text-fg",
    status === "doing" && "text-fg font-medium",
    status === "todo" && "text-fg-muted",
  );
