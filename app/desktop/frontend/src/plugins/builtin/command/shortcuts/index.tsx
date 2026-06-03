// Built-in plugin: mounts ShortcutsProvider on the overlay slot AND
// contributes the "Keyboard shortcuts" settings pane — a cheat-sheet for
// every registered shortcut, driven reactively off the plugin store so
// late-loaded plugins show up automatically.

import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ShortcutsProvider } from "@/plugins/host/ShortcutsProvider";
import { ShortcutsPane } from "./ShortcutsPane";

export default definePlugin({
  name: "lyra.builtin.shortcuts",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "shortcuts-provider",
      // Render before toaster (which is order 100) — order here doesn't
      // matter functionally (the component returns null) but keeping it
      // first puts the side-effect mount before any visible overlays.
      order: 50,
      component: ShortcutsProvider,
    });

    host.extensions.contribute(SETTINGS_PANE, {
      id: "shortcuts",
      label: "Shortcuts",
      icon: "command",
      order: 50,
      component: ShortcutsPane,
    });
  },
});
