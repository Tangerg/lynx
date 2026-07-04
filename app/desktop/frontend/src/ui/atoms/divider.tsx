import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// Divider — horizontal label flanked by faint hairlines.
// Used by Checkpoint, ApprovalCard's settled / declined states.
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
        "my-2 flex items-center gap-3 text-[11px] font-medium text-fg-faint",
        "before:flex-1 before:h-px before:content-[''] before:bg-fg/[0.08]",
        "after:flex-1  after:h-px  after:content-[''] after:bg-fg/[0.08]",
        className,
      )}
    >
      {icon && (
        <div
          className={cn(
            "grid h-4.5 w-4.5 place-items-center rounded-full bg-surface-2",
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
