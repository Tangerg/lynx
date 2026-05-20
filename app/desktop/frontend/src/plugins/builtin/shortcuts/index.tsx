// Built-in plugin: mounts ShortcutsProvider on the overlay slot.
//
// The single hardcoded shortcut we used to ship — Escape closing the
// settings modal — went away with the modal itself when Settings
// became a workspace view. Other plugins (command palette in particular)
// register their own shortcuts; this plugin's job is just to mount the
// global key listener.

import { ShortcutsProvider } from "@/plugins/ShortcutsProvider";
import { definePlugin } from "@/plugins/sdk";

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
  },
});
