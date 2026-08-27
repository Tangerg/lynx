// The plugin that paints appearance preferences onto the document.
//
// It installs during setup rather than at module eval, which is also when it
// first becomes useful: the painter resolves palette and style contributions
// registry, so a module-eval install (where this lived before) ran before any
// theme had registered and applied nothing, relying on a follow-up repaint. The
// scheme class itself is not at risk either way — index.html sets it inline from
// localStorage before any module loads, so first paint is never unstyled.

import { definePlugin } from "@/plugins/sdk";
import { disposeOnHmr } from "@/lib/hmr";
import { useUiStore } from "@/state/uiStore";
import { installDocumentAppearance } from "./adapters/documentAppearance";
import { installSystemAppearance } from "./adapters/systemAppearance";
import { installThemePreferencePort } from "./adapters/uiThemePreference";

export const appearancePainter = definePlugin({
  name: "scopeapp.builtin.appearance-painter",
  setup(ctx) {
    const releasePreference = installThemePreferencePort();
    // Before the painter: its first paint resolves the scheme, which asks this.
    const releaseSystem = installSystemAppearance();
    const stopPainting = installDocumentAppearance(useUiStore);
    const uninstall = () => {
      stopPainting();
      releaseSystem();
      releasePreference();
    };
    disposeOnHmr(uninstall);
    ctx.cleanup(uninstall);
  },
});
