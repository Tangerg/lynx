// Built-in plugin: "Plugins" settings pane. Registration only — the UI lives in
// ui/PluginsPane.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { pluginsSettingsPane } from "./application/pluginsPaneContributions";

const PluginsPane = lazy(() =>
  import("./ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export default definePlugin({
  name: "lyra.builtin.plugins-pane",
  setup(ctx) {
    registerSettingsPane(ctx, pluginsSettingsPane(PluginsPane));
  },
});
