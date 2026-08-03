import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { Divider, Icon } from "@/ui";
import { cn } from "@/lib/classNames";

// Shared chrome for the HITL cards (ApprovalCard / QuestionCard). Only the
// container + header row + the settled "done" divider are centralised here;
// each card's body stays fully custom (approval has a risk badge / command /
// args; question has per-question selects), so this shell intentionally does
// NOT try to abstract the bodies.

// Both variants are the same quiet card material; the difference is the EDGE.
//
// A region never gets a line and a card never gets one — but a card that has
// stopped the run to wait for a decision is neither. It is an object asking to be
// looked at, and until this variant carried the semantic edge the two entries here
// were byte-identical: the "warning" a caller passed had no rendering at all, and
// the whole cue was a header icon's colour. The fill stays neutral rather than
// tinted because this card runs 200px tall — a wash at that size is a lot of
// colour for "please look", which is the same reason the small inline notices do
// tint and this does not.
//
// Only the PENDING state reaches this shell (both cards collapse to a settled row
// once decided), so the edge cannot outlive what it is asking for.
const VARIANT_CLASS: Record<string, string> = {
  neutral: "bg-card",
  warning: "border border-warning-edge bg-card",
};

/** Settled "done" row — shared by approval (approved) + question (answered). */
export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </Divider>
  );
}

interface ShellProps {
  variant?: "neutral" | "warning";
  icon: IconName;
  iconClassName?: string;
  label: string;
  /** Optional trailing header content, pushed to the right (e.g. the
   *  approval card's risk badge). */
  trailing?: ReactNode;
  children: ReactNode;
  "data-slot"?: string;
}

export function HitlCardShell({
  variant = "neutral",
  icon,
  iconClassName,
  label,
  trailing,
  children,
  "data-slot": slot = "hitl-shell",
}: ShellProps) {
  return (
    <div data-slot={slot} className={cn("my-2 rounded-lg p-3", VARIANT_CLASS[variant])}>
      <div className="mb-1.5 flex items-center gap-2 text-ui-md font-medium text-fg">
        <Icon name={icon} size="sm" className={iconClassName} />
        <span>{label}</span>
        {trailing != null && (
          <>
            <span className="flex-1" />
            {trailing}
          </>
        )}
      </div>
      {children}
    </div>
  );
}
