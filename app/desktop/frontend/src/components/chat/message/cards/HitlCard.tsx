import type { ReactNode } from "react";
import type { IconName } from "@/components/common";
import { Divider, Icon } from "@/components/common";
import { cn } from "@/lib/utils";

// Shared chrome for the HITL cards (ApprovalCard / QuestionCard). Only the
// container + header row + the settled "done" divider are centralised here;
// each card's body stays fully custom (approval has a risk badge / command /
// args; question has per-question selects), so this shell intentionally does
// NOT try to abstract the bodies.

type Tone = "warning" | "accent";

const TONE_CARD: Record<Tone, string> = {
  warning: "border-warning/20 bg-warning/[0.03]",
  accent: "border-accent/20 bg-accent/[0.03]",
};

const TONE_TEXT: Record<Tone, string> = {
  warning: "text-warning",
  accent: "text-accent",
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
  tone: Tone;
  icon: IconName;
  label: string;
  /** Optional trailing header content, pushed to the right (e.g. the
   *  approval card's risk badge). */
  trailing?: ReactNode;
  children: ReactNode;
}

export function HitlCardShell({ tone, icon, label, trailing, children }: ShellProps) {
  return (
    <div className={cn("my-2 rounded-md border px-3 py-2.5", TONE_CARD[tone])}>
      <div
        className={cn(
          "mb-1.5 flex items-center gap-2 font-mono text-[11px] font-semibold",
          TONE_TEXT[tone],
        )}
      >
        <Icon name={icon} size={12} />
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
