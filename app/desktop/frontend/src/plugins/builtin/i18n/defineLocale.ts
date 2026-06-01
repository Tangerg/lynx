// Convenience wrapper for the built-in locale plugins: each is just
// `definePlugin` → `contribute(LOCALE, spec)` with a name derived from the
// language tag. Lives in this plugin package — NOT the core SDK — mirroring
// `defineThemePlugin` / `defineWorkspaceView`: the kernel exposes only the
// generic `contribute` write path; per-domain ergonomics belong to the domain.

import type { LocaleSpec } from "@/plugins/sdk/types";
import type { PluginSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";

/**
 * A built-in locale = the picker entry (`LocaleSpec`) plus, for every language
 * except the bootstrapped fallback (English), its translation `dict` registered
 * via `host.i18n.addBundle`. Pass `dict` to ship a bundle; omit it when the
 * dictionary is already loaded elsewhere (English, bootstrapped by lib/i18n).
 */
export function defineLocale(spec: LocaleSpec & { dict?: Record<string, string> }): PluginSpec {
  const { dict, ...locale } = spec;
  return definePlugin({
    name: `lyra.builtin.locale-${locale.id}`,
    version: "1.0.0",
    setup({ host }) {
      if (dict) host.i18n.addBundle(locale.id, dict);
      host.extensions.contribute(LOCALE, locale);
    },
  });
}
