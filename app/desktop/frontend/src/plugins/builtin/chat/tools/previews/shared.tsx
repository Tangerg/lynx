// Bits shared by the built-in tool previews (the generic ones in index.tsx +
// the per-family specialised previews lsp / skill / task / askUser / glob /
// webSearch).

// Shared container shape for every inline tool preview. The wrapper lives
// inside a bg-surface card (the expanded activity row), so it uses no
// additional background — just padding and typography.
export const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-[12px] leading-[1.55] text-fg-muted";

// Rows shown inline in a specialised preview before the "… N more" footer.
export const MAX_ROWS = 9;

// The "… N more" overflow footer shared across the specialised previews.
export function Overflow({ count }: { count: number }) {
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {count} more</div>;
}
