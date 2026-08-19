import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

export function AgentComposerTopTraySurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      data-slot="composer-top-tray-surface"
      className={cn(
        "relative -mb-px w-full min-w-0 overflow-clip rounded-t-composer border-x border-t border-[var(--composer-tray-edge-color)]",
        "bg-[var(--app-composer-tray-surface)] text-fg [-webkit-backdrop-filter:var(--composer-tray-backdrop)] [backdrop-filter:var(--composer-tray-backdrop)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
