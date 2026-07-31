// Presentation chrome shared by the built-in tool previews (the generic ones
// in index.tsx and the per-family previews lsp / skill / task / askUser /
// glob / webSearch).

// Shared container shape for the list/text inline previews. The wrapper lives
// inside a bg-surface card (the expanded activity row), so it uses no
// additional background — just padding and typography.
import { useT } from "@/lib/i18n";

export const TEXT_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted";

// Mono code / terminal panel — a bg-surface-2 slab that reads as a defined
// code block against the bg-surface card, matching the ShikiCodeBlock atom.
// (Deliberately NOT a bg-fg dark panel: bg-fg inverts per theme, so it would
// turn bright in dark mode — surface-2 stays a subtle step in both.)
export const CODE_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto rounded-sm bg-surface-2 px-3 py-2.5 font-mono text-ui-md leading-relaxed text-fg-soft";

// Rows shown inline in a specialised preview before the "… N more" footer.
export const INLINE_PREVIEW_ROW_LIMIT = 9;

// The "… N more" overflow footer shared across the specialised previews.
export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {t("tools.overflow.more", { count })}</div>;
}
