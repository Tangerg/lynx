/**
 * Historical turns may opt into the browser's off-screen rendering skip. The
 * tail turn may not: it owns the current Run outcome or HITL action, and a cold
 * restore can place that action inside the viewport before Chrome has ever
 * measured it. Applying content-visibility there leaves only the 220px
 * intrinsic placeholder in layout and removes the real controls from the
 * accessibility tree, so even an exact scroll-to-bottom cannot reveal them.
 */
export function transcriptTurnContentVisibility(isLast: boolean): string | undefined {
  return isLast ? undefined : "[content-visibility:auto] [contain-intrinsic-size:auto_220px]";
}
