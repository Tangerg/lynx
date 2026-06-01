// Kernel extension points — the typed handles for every contribution
// surface the kernel itself owns. Each `host.X.register*` facade and its
// matching selector route through one of these onto the shared `extensions`
// substrate, so built-in contributions and third-party ones use the exact
// same mechanism (the JetBrains "kernel is just another extension consumer"
// property).
//
// Points are migrated domain-by-domain (L3); this file grows one block per
// migrated domain. `single` = one entry per `keyOf` (override + warn);
// `multi` = every contribution coexists.

import type { LocaleSpec, ThemeAccentSpec, ThemeSpec } from "./types";
import { defineExtensionPoint } from "./defineExtensionPoint";

// ---- theme domain --------------------------------------------------------
export const THEME = defineExtensionPoint<ThemeSpec>({ id: "lyra.theme", keying: "single" });
export const ACCENT = defineExtensionPoint<ThemeAccentSpec>({
  id: "lyra.accent",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({ id: "lyra.locale", keying: "single" });
