// Built-in plugin: "Usage" settings pane. Registration only — the UI lives in
// ui/UsagePane, the usage.summary RPC use cases in application/usageConfig.

import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { usageSettingsPane } from "./application/usageContributions";
import { UsagePane } from "./ui/UsagePane";

export default definePlugin({
  name: "lyra.builtin.usage-pane",
  version: "1.0.0",
  setup({ host }) {
    registerSettingsPane(host, usageSettingsPane(UsagePane));
  },
});
