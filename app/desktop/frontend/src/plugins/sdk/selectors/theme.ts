// Theme / locale selectors — read-side of the theme + accent + locale
// registries. `resolveScheme` is the canonical way to map an id to
// "dark"/"light" (callers should never compare id strings directly —
// custom themes like "solarized-dark" would otherwise mis-classify).

import type { LocaleSpec, ThemeAccentSpec, ThemeSpec } from "../types";
import { usePluginStore } from "../registry";
import { useSortedList } from "./_helpers";

export function useThemes(): ThemeSpec[] {
  return useSortedList(usePluginStore((s) => s.themes));
}

export function useAccents(): ThemeAccentSpec[] {
  return useSortedList(usePluginStore((s) => s.accents));
}

export function useLocales(): LocaleSpec[] {
  return useSortedList(usePluginStore((s) => s.locales));
}

/** Look up a theme spec by id. */
export function lookupTheme(id: string): ThemeSpec | undefined {
  return usePluginStore.getState().themes.get(id)?.value;
}

/** Look up an accent spec by id. */
export function lookupAccent(id: string): ThemeAccentSpec | undefined {
  return usePluginStore.getState().accents.get(id)?.value;
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
