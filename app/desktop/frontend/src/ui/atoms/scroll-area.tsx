import type { CSSProperties, ReactNode } from "react";
import { forwardRef } from "react";
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
// Native scrollbar — a headless scroll-area primitive would add virtual track overhead
// for no real benefit on the surfaces we use this on (Settings rail,
// workspace view bodies, etc.).
//
// Reuses the `.panel-scroll` class so the WebKit thumb (10px wide,
// inset via `background-clip: content-box`) gets its own layout column.
// Pass `hideScrollbar` to suppress the chrome entirely on surfaces
// where a visible thumb fights with row content.
export const ScrollArea = forwardRef<HTMLDivElement, Props>(
  ({ className, style, children, hideScrollbar }, ref) => {
    // When `hideScrollbar` is set we deliberately drop the `.panel-scroll`
    // class — its `::-webkit-scrollbar { width: 10px }` rule is defined
    // in layout.css, which comes after Tailwind utilities in the cascade
    // and would otherwise override `[&::-webkit-scrollbar]:hidden` (both
    // selectors have identical specificity; source order wins). Using
    // utility-only layout sidesteps the conflict.
    return (
      <div
        ref={ref}
        className={cn(
          hideScrollbar
            ? "flex-1 min-h-0 overflow-y-auto overflow-x-hidden overscroll-contain [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
            : "panel-scroll",
          className,
        )}
        style={style}
      >
        {children}
      </div>
    );
  },
);
