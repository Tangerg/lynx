import type {CSSProperties, ReactNode} from "react";
import {  forwardRef  } from "react";
import { cn } from "@/lib/utils";

interface Props {
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
  /**
   * Hide the scrollbar chrome while keeping the area scrollable.
   * Use on dense surfaces (sidebar lists) where macOS WebKit's
   * default overlay thumb visually crowds row content (e.g. time
   * badges that sit at the right edge). Users can still scroll via
   * trackpad / mouse wheel; the visual cue is the natural content
   * cutoff at the top/bottom of the column.
   */
  hideScrollbar?: boolean;
}

// Vertical scroll container with our project-wide scrollbar styling.
// Native scrollbar — Radix ScrollArea would add virtual track overhead
// for no real benefit on the surfaces we use this on (Settings rail,
// workspace view bodies, etc.).
//
// Reuses the `.panel-scroll` class so the WebKit thumb (10px wide,
// inset via `background-clip: content-box`) gets its own layout column.
// Pass `hideScrollbar` to suppress the chrome entirely on surfaces
// where a visible thumb fights with row content.
export const ScrollArea = forwardRef<HTMLDivElement, Props>((
  { className, style, children, hideScrollbar },
  ref,
) => {
  return (
    <div
      ref={ref}
      className={cn(
        "panel-scroll",
        hideScrollbar && "[scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        className,
      )}
      style={style}
    >
      {children}
    </div>
  );
});
