// The plugin that paints appearance preferences onto the document.
//
// It installs during setup rather than at module eval, which is also when it
// first becomes useful: the painter resolves theme tokens through the THEME
// registry, so a module-eval install (where this lived before) ran before any
// theme had registered and applied nothing, relying on a follow-up repaint. The
// scheme class itself is not at risk either way — index.html sets it inline from
// localStorage before any module loads, so first paint is never unstyled.

import { definePlugin } from "@/plugins/sdk";
import { disposeOnHmr } from "@/lib/hmr";
import { useUiStore } from "@/state/uiStore";
import { installDocumentTheme } from "./adapters/documentTheme";
import { installThemePreferencePort } from "./adapters/uiThemePreference";

export const appearancePainter = definePlugin({
  name: "lyra.builtin.appearance-painter",
  version: "1.0.0",
  setup() {
    const releasePort = installThemePreferencePort();
    const stopPainting = installDocumentTheme(useUiStore);
    const uninstall = () => {
      stopPainting();
      releasePort();
    };
    disposeOnHmr(uninstall);
    return uninstall;
  },
});
