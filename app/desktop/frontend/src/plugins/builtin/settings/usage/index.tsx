// Built-in plugin: "Usage" settings pane. Registration only — the UI lives in
// ui/UsagePane, the usage.summary RPC use cases in application/usageConfig.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installUsageGateway } from "./adapters/runtimeUsageGateway";
import { usageSettingsPane } from "./application/usageContributions";

const UsagePane = lazy(() =>
  import("./ui/UsagePane").then(({ UsagePane }) => ({ default: UsagePane })),
);

export default definePlugin({
  name: "lyra.builtin.usage-pane",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installUsageGateway();
    registerSettingsPane(host, usageSettingsPane(UsagePane));
    return disposeGateway;
  },
});
