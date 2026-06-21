// System font discovery for the Appearance → Font picker.
//
// `queryLocalFonts()` would give us the full enumeration but is gated
// behind a permission prompt and not supported in WebKit (Wails 2 ships
// WebKit on macOS, WebView2 on Windows). Instead we use a curated
// cross-platform candidate list and filter it down via the synchronous
// `document.fonts.check()` API, which reports whether a font is loaded
// + matchable for the given size. The app ships no bundled webfont — the
// default is the OS stack (SF Pro / PingFang on macOS); this picker only
// offers an explicit override.

import { useMemo } from "react";

// Sans-serif / proportional candidates. Order is "best Mac default → wide
// availability"; the empty default ("") in uiStore already resolves to the
// native system stack, so these are opt-in overrides.
const CANDIDATE_UI_FONTS = [
  "SF Pro Text",
  "SF Pro Display",
  "Inter",
  "Helvetica Neue",
  "Segoe UI",
  "Roboto",
  "Ubuntu",
  "Cantarell",
  "Arial",
];

// Monospace candidates — the usual dev-font lineup. SF Mono / Menlo lead on
// macOS; the rest cover other platforms + popular code-editor picks.
const CANDIDATE_CODE_FONTS = [
  "SF Mono",
  "Menlo",
  "JetBrains Mono",
  "Fira Code",
  "Cascadia Code",
  "Cascadia Mono",
  "Monaco",
  "Consolas",
  "Source Code Pro",
  "Hack",
  "IBM Plex Mono",
  "DejaVu Sans Mono",
];

function isAvailable(family: string): boolean {
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
