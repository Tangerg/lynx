// Built-in plugin: "Plugins" settings pane. Registration only — the UI lives in
// ui/PluginsPane.

import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { pluginsSettingsPane } from "./application/pluginsPaneContributions";
import { PluginsPane } from "./ui/PluginsPane";

export default definePlugin({
  name: "lyra.builtin.plugins-pane",
  version: "1.0.0",
  setup({ host }) {
    registerSettingsPane(host, pluginsSettingsPane(PluginsPane));
  },
});
