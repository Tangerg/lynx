// Built-in plugin: registers Settings as a workspace view, openable as a
// main-area tab (Cmd+K → "View: Settings" or the user-card "More
// settings…" link). Replaces the older modal version that lived on the
// app.overlay slot.

import { SettingsPage } from "@/components/settings/SettingsPage";
import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.shell-settings",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "settings",
      title: "Settings",
      icon: "settings",
      // Not auto-opened on first launch — only when the user invokes it.
      openByDefault: false,
      component: SettingsPage,
    });
  },
});
