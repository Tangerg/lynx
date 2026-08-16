// Built-in plugin: Appearance settings pane.
//
// Sections (theme / accent / contrast / font / shape / motion / language)
// live in sibling files. This file is only the plugin manifest.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { appearanceSettingsPane } from "./application/appearanceContributions";
import { installBrowserFontAvailability } from "./adapters/browserFontAvailability";
import { installAppearancePreferencesPort } from "./adapters/uiAppearancePreferences";

const AppearancePane = lazy(() =>
  import("./ui/AppearancePane").then(({ AppearancePane }) => ({ default: AppearancePane })),
);

export default definePlugin({
  name: "lyra.builtin.appearance",
  setup(ctx) {
    const disposePreferences = installAppearancePreferencesPort();
    const disposeFonts = installBrowserFontAvailability();
    registerSettingsPane(ctx, appearanceSettingsPane(AppearancePane));
    ctx.cleanup(() => {
      disposeFonts();
      disposePreferences();
    });
  },
});
