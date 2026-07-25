import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function SectionLabel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        "px-2 pb-1 pt-4 font-sans text-ui-md font-normal leading-none text-fg-muted/58",
        className,
      )}
    >
      {children}
    </div>
  );
}
