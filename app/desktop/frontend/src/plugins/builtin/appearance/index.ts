// Built-in plugin: Appearance settings pane.
//
// Sections (theme / accent / font / message style / language) live in
// sibling files. This file is only the plugin manifest.

import { definePlugin } from "@/plugins/sdk";
import { AppearancePane } from "./AppearancePane";

export default definePlugin({
  name: "lyra.builtin.appearance",
  version: "1.0.0",
  setup({ host }) {
    host.settings.registerPane({
      id: "appearance",
      label: "Appearance",
      icon: "spark",
      order: 0,
      component: AppearancePane,
    });
  },
});
