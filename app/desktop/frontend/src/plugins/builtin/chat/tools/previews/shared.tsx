// Bits shared by the built-in tool previews (the generic ones in index.tsx +
// the per-family specialised previews lsp / skill / task / askUser / glob /
// webSearch).

// Shared container shape for the list/text inline previews. The wrapper lives
// inside a bg-surface card (the expanded activity row), so it uses no
// additional background — just padding and typography.
export const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-[12px] leading-[1.55] text-fg-muted";

// Mono code / terminal panel — a bg-surface-2 slab that reads as a defined
// code block against the bg-surface card, matching the ShikiCodeBlock atom.
// (Deliberately NOT a bg-fg dark panel: bg-fg inverts per theme, so it would
// turn bright in dark mode — surface-2 stays a subtle step in both.)
export const CODE_PANEL =
  "max-h-60 overflow-y-auto rounded-[8px] bg-surface-2 px-3 py-2.5 font-mono text-[12px] leading-[1.6] text-fg-soft";

// Rows shown inline in a specialised preview before the "… N more" footer.
export const MAX_ROWS = 9;

// The "… N more" overflow footer shared across the specialised previews.
export function Overflow({ count }: { count: number }) {
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {count} more</div>;
}
