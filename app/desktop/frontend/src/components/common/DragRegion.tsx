// Window-drag affordances for Wails / WKWebView. Two halves of the same
// abstraction so the dual `[-webkit-app-region] [--wails-draggable]`
// utility pair doesn't get repeated all over the tree:
//
//   <DragStrip height={48} />   // an invisible strip you can drag
//   className={cn(..., noDragClasses)}   // mark an element as no-drag
//
// `-webkit-app-region` is the native WKWebView property (macOS); Wails
// also reads `--wails-draggable` so Windows / Linux behave the same.

import { cn } from "@/lib/utils";

/** Tailwind utility pair marking an element as a window drag handle. */
export const dragClasses = "[-webkit-app-region:drag] [--wails-draggable:drag]";

/** Tailwind utility pair that opts an element out of the surrounding drag
 *  region — apply to interactive controls that sit inside a drag strip. */
export const noDragClasses = "[-webkit-app-region:no-drag] [--wails-draggable:no-drag]";

interface DragStripProps {
  /** Strip height in px. macOS titlebar shim is 48 (full) / 36 (rail);
   *  PanelTabBar uses its own tab-strip height inline. */
  height: number;
  className?: string;
}

/**
 * Absolutely-positioned drag region pinned to the top of the nearest
 * positioned ancestor. Renders nothing visible — it just declares "this
 * 48×fullWidth rectangle is the macOS titlebar drag handle".
 */
export function DragStrip({ height, className }: DragStripProps) {
  return (
    <div
      className={cn("absolute top-0 left-0 right-0", dragClasses, className)}
      style={{ height }}
    />
  );
}
