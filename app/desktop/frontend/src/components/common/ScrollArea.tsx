import { forwardRef, type CSSProperties, type ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
};

// Vertical scroll container with our project-wide scrollbar styling.
// Native scrollbar — Radix ScrollArea would add virtual track overhead
// for no real benefit on the surfaces we use this on (Settings rail,
// workspace view bodies, etc.).
export const ScrollArea = forwardRef<HTMLDivElement, Props>(function ScrollArea(
  { className, style, children },
  ref,
) {
  return (
    <div
      ref={ref}
      className={cn(
        "flex-1 min-h-0 overflow-y-auto overflow-x-hidden " +
          "[scrollbar-width:thin] [scrollbar-color:var(--color-border-soft)_transparent]",
        className,
      )}
      style={style}
    >
      {children}
    </div>
  );
});
