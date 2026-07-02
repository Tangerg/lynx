// Built-in plugin: "Usage" settings pane. Registration only — the UI lives in
// ui/UsagePane, the usage.summary RPC use cases in application/usageConfig.

import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { UsagePane } from "./ui/UsagePane";

export default definePlugin({
  name: "lyra.builtin.usage-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "usage",
      label: "settings.pane.usage",
      group: "models",
      icon: "chart",
      order: 55, // just after Providers (50)
      component: UsagePane,
    });
  },
});
