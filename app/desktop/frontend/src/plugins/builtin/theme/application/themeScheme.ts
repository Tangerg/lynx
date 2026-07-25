import { THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { resolveScheme } from "@/plugins/sdk/selectors/theme";
import { themePreference } from "./ports/themePreference";

export type ThemeScheme = "dark" | "light";

export function resolveThemeScheme(themeId: string): ThemeScheme {
  return resolveScheme(themeId);
}

export function isLightTheme(themeId: string): boolean {
  return resolveThemeScheme(themeId) === "light";
}

/**
 * Flip to the primary theme of the opposite scheme.
 *
 * Lives here rather than on the store: picking *which* theme comes next needs
 * the THEME registry and its contributed order, and a store that reaches into
 * the plugin registry is a store that knows about the plugin system. The store
 * keeps `setTheme` — it holds the value; this decides it.
 *
 * `lookupExtensionPoint` returns themes already sorted by `order`, so the first
 * match is the opposite scheme's primary — the same order the appearance pane
 * shows.
 */
export function toggleThemeScheme(): void {
  const preference = themePreference();
  const target = resolveScheme(preference.activeTheme()) === "dark" ? "light" : "dark";
  const next = lookupExtensionPoint(THEME).find((spec) => spec.scheme === target);
  if (next) preference.setTheme(next.id);
}
