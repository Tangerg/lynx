import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/utils";

export function AgentComposerSurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "rounded-[22px] bg-canvas px-6 py-4 shadow-[var(--shadow-composer)]",
        "transition-[box-shadow] duration-[160ms] ease-out focus-within:shadow-[var(--shadow-popover)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
