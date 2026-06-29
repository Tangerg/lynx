// Window-drag affordances for Wails / WKWebView. The dual
// `[-webkit-app-region] [--wails-draggable]` utility pair is exported
// as ready-made Tailwind classes so it doesn't get repeated all over
// the tree:
//
//   className={cn(..., dragClasses)}     // mark a panel as draggable
//   className={cn(..., noDragClasses)}   // mark an element as no-drag
//
// `-webkit-app-region` is the native WKWebView property (macOS); Wails
// also reads `--wails-draggable` so Windows / Linux behave the same.

/** Tailwind utility pair marking an element as a window drag handle. */
export const dragClasses = "[-webkit-app-region:drag] [--wails-draggable:drag]";

/** Tailwind utility pair that opts an element out of the surrounding drag
 *  region — apply to interactive controls that sit inside a drag strip. */
export const noDragClasses = "[-webkit-app-region:no-drag] [--wails-draggable:no-drag]";
