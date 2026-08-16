// Convenience wrapper for the built-in locale plugins: each is just
// `definePlugin` → `contribute(LOCALE, spec)` with a name derived from the
// language tag. Lives in this plugin package — NOT the core SDK — mirroring
// `defineThemePlugin` / `defineWorkspaceView`: the kernel exposes only the
// generic `contribute` write path; per-domain ergonomics belong to the domain.

import type { LocaleSpec } from "@/plugins/sdk/types";
import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { activeLocale, addLocaleBundle } from "@/lib/i18n";

/**
 * A built-in locale = the picker entry (`LocaleSpec`), whose `load` fetches the
 * dictionary the first time that language is selected. English omits `load`:
 * lib/i18n bootstraps it so first paint always has strings.
 *
 * Registration is the entry only — no dictionary is read at setup. Eight
 * languages statically imported at setup meant eight dictionaries in the entry
 * payload, seven of which the reader will never see.
 */
export function defineLocale(spec: LocaleSpec): AnyPlugin {
  return definePlugin({
    name: `lyra.builtin.locale-${spec.id}`,
    setup(ctx) {
      ctx.contribute(LOCALE, spec);
      // Cold start with a persisted non-English locale: this plugin is the only
      // thing that knows how to fetch its own dictionary, so it does — during
      // setup, which runs before first paint.
      if (spec.load && spec.id === activeLocale()) {
        void spec.load().then((dict) => {
          addLocaleBundle(spec.id, dict);
        });
      }
    },
  });
}
