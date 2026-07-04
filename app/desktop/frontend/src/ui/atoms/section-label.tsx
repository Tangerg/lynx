import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function SectionLabel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        "px-2.5 pb-1 pt-4 font-sans text-[12px] font-medium leading-none text-fg-muted",
        className,
      )}
    >
      {children}
    </div>
  );
}
