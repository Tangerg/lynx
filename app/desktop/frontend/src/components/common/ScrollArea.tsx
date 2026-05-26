import type {CSSProperties, ReactNode} from "react";
import {  forwardRef  } from "react";
import { cn } from "@/lib/utils";

interface Props {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
}

// Vertical scroll container with our project-wide scrollbar styling.
// Native scrollbar — Radix ScrollArea would add virtual track overhead
// for no real benefit on the surfaces we use this on (Settings rail,
// workspace view bodies, etc.).
//
// Reuses the `.panel-scroll` class so the WebKit thumb (10px wide,
// inset via `background-clip: content-box`) gets its own layout column.
// Without it, macOS WebKit falls back to system overlay scrollbars
// that float semi-transparently over content — making row contents
// (e.g. SessionRow's time badge at the far right) visually crowd /
// cover the scrollbar.
export const ScrollArea = forwardRef<HTMLDivElement, Props>((
  { className, style, children },
  ref,
) => {
  return (
    <div ref={ref} className={cn("panel-scroll", className)} style={style}>
      {children}
    </div>
  );
});
