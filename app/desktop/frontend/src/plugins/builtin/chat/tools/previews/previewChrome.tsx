// Presentation chrome shared by the built-in tool previews. Every tool keeps an
// independent component boundary; related tools reuse these material primitives
// instead of sharing one registry renderer.

// Shared container shape for the list/text inline previews. The disclosure's body
// carries no fill of its own — the card behind it is the ground — so a text preview
// is padding and typography, and only the panels that hold program output cut a
// well into it.
import { useT } from "@/lib/i18n";

export const TEXT_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-ui-md leading-body text-fg-muted";

// Mono code / terminal panel — the recessed well, same material as the
// ShikiCodeBlock atom, so program output reads as cut into the card rather than
// stacked on it. (Deliberately NOT a bg-fg dark panel: bg-fg inverts per theme,
// so it would turn bright in dark mode.)
//
// This is the ONLY fill inside a preview; the disclosure body remains the ground
// so nested output reads as a well rather than another flat surface.
export const CODE_PREVIEW_CLASS =
  "max-h-60 overflow-y-auto rounded-sm bg-sunken px-3 py-2.5 font-mono text-code leading-relaxed text-fg-soft";

// Rows shown inline in a specialised preview before the "… N more" footer.
export const INLINE_PREVIEW_ROW_LIMIT = 9;

// The "… N more" overflow footer shared across the specialised previews.
export function PreviewOverflow({ count }: { count: number }) {
  const t = useT();
  if (count <= 0) return null;
  return <div className="text-fg-faint">… {t("tools.overflow.more", { count })}</div>;
}
