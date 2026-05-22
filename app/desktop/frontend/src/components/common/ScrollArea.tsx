import { forwardRef, type CSSProperties, type ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
};

// Vertical scroll container with our project-wide scrollbar styling.
export const ScrollArea = forwardRef<HTMLDivElement, Props>(function ScrollArea(
  { className, style, children },
  ref,
) {
  return (
    <div ref={ref} className={cn("panel-scroll", className)} style={style}>
      {children}
    </div>
  );
});
