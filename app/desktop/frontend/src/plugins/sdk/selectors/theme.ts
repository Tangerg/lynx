// Theme / locale selectors — read-side of the theme + accent + locale
// extension points. `resolveScheme` is the canonical way to map an id to
// "dark"/"light" (callers should never compare id strings directly —
// custom themes like "solarized-dark" would otherwise mis-classify).

import type { LocaleSpec, ThemeAccentSpec, ThemeSpec } from "../types";
import { ACCENT, LOCALE, THEME } from "../kernelPoints";
import { lookupExtensionByKey, useExtensionPoint } from "./extensions";

export function useThemes(): ThemeSpec[] {
  return useExtensionPoint(THEME);
}

export function useAccents(): ThemeAccentSpec[] {
  return useExtensionPoint(ACCENT);
}

export function useLocales(): LocaleSpec[] {
  return useExtensionPoint(LOCALE);
}

/** Look up a theme spec by id. */
export function lookupTheme(id: string): ThemeSpec | undefined {
  return lookupExtensionByKey(THEME, id);
}

/** Look up an accent spec by id. */
export function lookupAccent(id: string): ThemeAccentSpec | undefined {
  return lookupExtensionByKey(ACCENT, id);
}

/**
 * Resolve a theme id to its scheme (`"dark"` / `"light"`).
 *
 * Defaults to `"dark"` when the id isn't registered (e.g. very early in
 * boot before built-in plugins finish, or the user has a saved id that no
 * longer exists). Callers wanting the binary "is this a light theme?"
 * distinction (Shiki preset, Mermaid theme, …) should read scheme via
 * this helper rather than comparing the id against `"light"` directly —
 * custom themes like `"solarized-dark"` would otherwise fall through.
 */
export function resolveScheme(themeId: string): "dark" | "light" {
  return lookupTheme(themeId)?.scheme ?? "dark";
}
