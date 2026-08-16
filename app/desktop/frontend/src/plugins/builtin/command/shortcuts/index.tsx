// Built-in plugin: mounts ShortcutsProvider on the overlay slot AND
// contributes the "Keyboard shortcuts" settings pane — a cheat-sheet for
// every registered shortcut, driven reactively off the plugin store so
// late-loaded plugins show up automatically.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ShortcutsProvider } from "@/plugins/host/ShortcutsProvider";
import { shortcutsProviderSlot, shortcutsSettingsPane } from "./application/shortcutContributions";
import { ShortcutsPane } from "./ShortcutsPane";

export default definePlugin({
  name: "lyra.builtin.shortcuts",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", shortcutsProviderSlot(ShortcutsProvider));

    ctx.contribute(SETTINGS_PANE, shortcutsSettingsPane(ShortcutsPane));
  },
});
