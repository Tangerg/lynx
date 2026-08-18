// Built-in plugin: mounts ShortcutsProvider on the overlay slot AND
// contributes the "Keyboard shortcuts" settings pane — a cheat-sheet for
// every registered shortcut, driven reactively off the plugin store so
// late-loaded plugins show up automatically.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ShortcutsProvider } from "@/plugins/host/ShortcutsProvider";
import { ShortcutsPane } from "./ShortcutsPane";

export default definePlugin({
  name: "lyra.builtin.shortcuts",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "shortcuts-provider",
      // Before toaster so the side-effect mount precedes visible overlays.
      order: 50,
      component: ShortcutsProvider,
    });

    ctx.contribute(SETTINGS_PANE, {
      id: "shortcuts",
      label: "settings.pane.shortcuts",
      description: "shortcuts.sub",
      group: "general",
      icon: "command",
      order: 10,
      component: ShortcutsPane,
    });
  },
});
