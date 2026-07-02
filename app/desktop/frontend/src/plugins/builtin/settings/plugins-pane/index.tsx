// Built-in plugin: "Plugins" settings pane. Registration only — the UI lives in
// ui/PluginsPane.

import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { PluginsPane } from "./ui/PluginsPane";

export default definePlugin({
  name: "lyra.builtin.plugins-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "plugins",
      label: "settings.pane.plugins",
      group: "integrations",
      icon: "tool",
      order: 99,
      component: PluginsPane,
    });
  },
});
