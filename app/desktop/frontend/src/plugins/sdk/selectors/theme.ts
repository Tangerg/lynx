// Theme helpers with real logic. Plain reads go through the generic substrate
// (`useExtensionPoint(THEME)` / `lookupExtensionByKey(ACCENT, id)`) — there is
// no per-domain alias.

import { THEME } from "../kernelPoints";
import { lookupExtensionByKey } from "./extensions";

/**
 * Resolve a theme id to its scheme (`"dark"` / `"light"`).
 *
 * Defaults to `"dark"` when the id isn't registered (early boot, or a saved id
 * that no longer exists). Callers wanting "is this light?" (Shiki preset,
 * Mermaid theme…) should resolve scheme through this rather than comparing the
 * id against `"light"` — custom ids like `"solarized-dark"` would mis-classify.
 */
export function resolveScheme(themeId: string): "dark" | "light" {
  return lookupExtensionByKey(THEME, themeId)?.scheme ?? "dark";
}
