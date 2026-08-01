import type { Scheme } from "@/lib/appearance";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { systemAppearance } from "./ports/systemAppearance";
import { themePreference } from "./ports/themePreference";

/**
 * Which scheme a theme id paints in.
 *
 * Callers wanting "is this light?" (the Shiki preset, the Mermaid theme, the
 * accent variant) must resolve through here rather than comparing the id against
 * `"light"` — a contributed id like `"solarized-light"` would mis-classify.
 * Unregistered ids read as dark: that covers early boot and a saved id whose
 * plugin is gone.
 *
 * This lived in the kernel's theme selector, where nothing in the kernel used it
 * — the scheme of a contributed theme is this context's business, and it owns
 * both the COLOR_THEME point the answer comes from and the meaning of `"system"`.
 */
export function resolveThemeScheme(themeId: string): Scheme {
  if (themeId === "system") return systemAppearance().scheme();
  return lookupExtensionByKey(COLOR_THEME, themeId)?.scheme ?? "dark";
}

export function isLightTheme(themeId: string): boolean {
  return resolveThemeScheme(themeId) === "light";
}

/**
 * Flip to the primary theme of the opposite scheme.
 *
 * Lives here rather than on the store: picking *which* theme comes next needs
 * the COLOR_THEME registry and its contributed order, and a store that reaches into
 * the plugin registry is a store that knows about the plugin system. The store
 * keeps `setTheme` — it holds the value; this decides it.
 *
 * `lookupExtensionPoint` returns themes already sorted by `order`, so the first
 * match is the opposite scheme's primary — the same order the appearance pane
 * shows.
 */
export function toggleThemeScheme(): void {
  const preference = themePreference();
  const target = resolveThemeScheme(preference.activeTheme()) === "dark" ? "light" : "dark";
  const next = lookupExtensionPoint(COLOR_THEME).find((spec) => spec.scheme === target);
  if (next) preference.setTheme(next.id);
}
