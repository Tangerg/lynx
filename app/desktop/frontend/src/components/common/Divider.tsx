import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// Divider — horizontal label flanked by faint gradient lines.
// Used by Checkpoint, ApprovalCard's settled / declined states. The
// gradient (transparent → border-soft 50% → transparent) makes the
// line feel like it "appears from nothing" instead of a hard rule.
//
// `intent` only tunes the icon container color (bg-surface-2 always,
// icon color varies). The label text always uses fg-faint.
export function Divider({
  icon,
  intent = "neutral",
  className,
  children,
}: {
  icon?: ReactNode;
  intent?: "neutral" | "accent";
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "my-2 flex items-center gap-3 font-mono text-[10.5px] font-semibold text-fg-faint",
        "before:flex-1 before:h-px before:content-[''] before:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]",
        "after:flex-1  after:h-px  after:content-[''] after:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]",
        className,
      )}
    >
      {icon && (
        <div
          className={cn(
            "grid h-[18px] w-[18px] place-items-center rounded-full bg-surface-2",
            intent === "accent" ? "text-accent" : "text-fg-faint",
          )}
        >
          {icon}
        </div>
      )}
      <span>{children}</span>
    </div>
  );
}
