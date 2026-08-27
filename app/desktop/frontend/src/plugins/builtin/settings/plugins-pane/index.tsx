// Built-in plugin: "Plugins" settings pane. Registration only — the UI lives in
// ui/PluginsPane.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { PLUGINS_PANE } from "../public/panes";

const PluginsPane = lazy(() =>
  import("./ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export default definePlugin({
  name: "scopeapp.builtin.plugins-pane",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: PLUGINS_PANE,
      label: "settings.pane.plugins",
      group: "integrations",
      icon: "tool",
      order: 99,
      component: PluginsPane,
    });
  },
});
