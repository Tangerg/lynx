// Built-in plugin: Appearance settings pane.
//
// Sections (theme / accent / contrast / font / shape / motion / language)
// live in sibling files. This file is only the plugin manifest.

import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { appearanceSettingsPane } from "./application/appearanceContributions";
import { installAppearancePreferencesPort } from "./adapters/uiAppearancePreferences";
import { AppearancePane } from "./ui/AppearancePane";

export default definePlugin({
  name: "lyra.builtin.appearance",
  version: "1.0.0",
  setup({ host }) {
    installAppearancePreferencesPort();
    registerSettingsPane(host, appearanceSettingsPane(AppearancePane));
  },
});
