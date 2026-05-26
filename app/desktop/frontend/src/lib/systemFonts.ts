// System font discovery for the Appearance → Font picker.
//
// `queryLocalFonts()` would give us the full enumeration but is gated
// behind a permission prompt and not supported in WebKit (Wails 2 ships
// WebKit on macOS, WebView2 on Windows). Instead we use a curated
// cross-platform candidate list and filter it down via the synchronous
// `document.fonts.check()` API, which reports whether a font is loaded
// + matchable for the given size. Fonts that ship via Google Fonts
// `@import` in globals.css (Geist / Geist Mono) are always in the list
// regardless — they're guaranteed available.

import { useMemo } from "react";

// Sans-serif / proportional candidates. Order is "best default → wide
// availability"; Geist anchors the top because it's bundled.
const CANDIDATE_UI_FONTS = [
  "Geist",
  "Inter",
  "SF Pro Text",
  "SF Pro Display",
  "Helvetica Neue",
  "Segoe UI",
  "Roboto",
  "Ubuntu",
  "Cantarell",
  "Arial",
];

// Monospace candidates — the usual dev-font lineup. JetBrains Mono and
// Fira Code lead because they're the most common code-editor choices.
const CANDIDATE_CODE_FONTS = [
  "Geist Mono",
  "JetBrains Mono",
  "Fira Code",
  "Cascadia Code",
  "Cascadia Mono",
  "SF Mono",
  "Menlo",
  "Monaco",
  "Consolas",
  "Source Code Pro",
  "Hack",
  "IBM Plex Mono",
  "DejaVu Sans Mono",
];

// Bundled fonts that we never want to filter out — they're loaded via
// `@import` in globals.css so `document.fonts.check` may race the load
// on first paint. Hard-listing them guarantees they show up.
const ALWAYS_AVAILABLE = new Set(["Geist", "Geist Mono"]);

function isAvailable(family: string): boolean {
  if (ALWAYS_AVAILABLE.has(family)) return true;
  if (typeof document === "undefined") return false;
  const check = document.fonts?.check;
  if (typeof check !== "function") return true; // assume available when API missing
  try {
    return document.fonts.check(`12px "${family}"`);
  } catch {
    return false;
  }
}

/**
 * The list of font families to show in the picker, filtered down to
 * what's actually installed on the user's machine. Memoised per `mono`
 * so the picker doesn't re-detect on every render.
 */
export function useSystemFonts(mono: boolean): string[] {
  return useMemo(() => {
    const candidates = mono ? CANDIDATE_CODE_FONTS : CANDIDATE_UI_FONTS;
    return candidates.filter(isAvailable);
  }, [mono]);
}
