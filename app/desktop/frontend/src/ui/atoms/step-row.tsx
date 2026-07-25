import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon } from "@/ui/icons";

/**
 * A step's completion state, in the design system's own words.
 *
 * Deliberately not the agent's `PlanItem["status"]`: three contexts render this
 * row — an inline plan, the plan view, the agent's working checklist — and each
 * maps its own vocabulary in. A shared row typed against one caller's domain
 * would make the other two speak a language they don't own.
 */
export type StepState = "done" | "active" | "pending";

const MARK = "grid h-4 w-4 shrink-0 place-items-center";

/**
 * The mark on its own, for a row the caller must lay out itself — the collapsed
 * plan banner strikes completed items through, which is its own reading of the
 * same state, not this row's.
 */
export function StepMark({ state }: { state: StepState }) {
  return (
    <div className={MARK}>
      {state === "done" && <Icon name="check" size={14} strokeWidth={3} className="text-success" />}
      {state === "active" && (
        <div className="relative h-3 w-3 rounded-full border-[1.5px] border-accent">
          <div className="absolute inset-0.5 animate-pulse-dot rounded-full bg-accent" />
        </div>
      )}
      {state === "pending" && (
        <div className="h-3 w-3 rounded-full border-[1.5px] border-fg-faint/40" />
      )}
    </div>
  );
}

// One row of a checklist: the mark, then whatever the caller is checking off.
//
// A component rather than a class-string helper. The mark's placement and the
// row's ink both follow from the state, and a caller assembling those itself is
// a caller reimplementing the row — which is how the previous version worked:
// `<div className={planItemRow(status)}><PlanCheck status={status} />`, spelled
// four times.
export function StepRow({
  state,
  className,
  children,
}: {
  state: StepState;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 py-0.5 text-ui-lg",
        state === "done" && "text-fg",
        state === "active" && "font-medium text-fg",
        state === "pending" && "text-fg-muted",
        className,
      )}
    >
      <StepMark state={state} />
      <span className="min-w-0 flex-1">{children}</span>
    </div>
  );
}
