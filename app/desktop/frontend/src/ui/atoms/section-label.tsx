import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

export function SectionLabel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        "px-2 pb-1.5 pt-2 font-sans text-ui-sm font-normal leading-none text-fg-muted/58",
        className,
      )}
    >
      {children}
    </div>
  );
}
