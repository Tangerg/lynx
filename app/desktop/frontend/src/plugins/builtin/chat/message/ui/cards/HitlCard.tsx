import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { Divider, Icon } from "@/ui";
import { cn } from "@/lib/utils";

// Shared chrome for the HITL cards (ApprovalCard / QuestionCard). Only the
// container + header row + the settled "done" divider are centralised here;
// each card's body stays fully custom (approval has a risk badge / command /
// args; question has per-question selects), so this shell intentionally does
// NOT try to abstract the bodies.

const VARIANT_CLASS: Record<string, string> = {
  neutral: "bg-surface",
  warning: "border-[0.5px] border-warning/30 bg-warning/[0.03]",
};

/** Settled "done" row — shared by approval (approved) + question (answered). */
export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size={11} strokeWidth={3} />} intent="accent">
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
    <div data-slot={slot} className={cn("my-2 rounded-md px-4 py-3", VARIANT_CLASS[variant])}>
      <div className="mb-2 flex items-center gap-2 text-[13px] font-medium text-fg">
        <Icon name={icon} size={13} className={iconClassName} />
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
