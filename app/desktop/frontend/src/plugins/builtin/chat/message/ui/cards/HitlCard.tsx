import type { ReactNode } from "react";
import type { IconName } from "@/ui";
import { Divider, Icon } from "@/ui";

// QuestionCard keeps its own compact HITL shell. Approval requests deliberately
// do not share it: Codex gives approvals a larger, role-specific request surface
// whose identity, material and scoped actions form a different hierarchy.

/** Settled "done" row — shared by approval (approved) + question (answered). */
export function HitlSettledRow({ label }: { label: string }) {
  return (
    <Divider icon={<Icon name="check" size="xs" />} intent="accent">
      {label}
    </Divider>
  );
}

interface ShellProps {
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
  icon,
  iconClassName,
  label,
  trailing,
  children,
  "data-slot": slot = "hitl-shell",
}: ShellProps) {
  return (
    <div data-slot={slot} className="rounded-lg bg-card p-3">
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
