// Built-in plugin: "Usage" settings pane. Registration only — the UI lives in
// ui/UsagePane, the usage.summary RPC use cases in application/usageConfig.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { USAGE_PANE } from "../public/panes";
import { installUsageGateway } from "./adapters/runtimeUsageGateway";

const UsagePane = lazy(() =>
  import("./ui/UsagePane").then(({ UsagePane }) => ({ default: UsagePane })),
);

export default definePlugin({
  name: "lyra.builtin.usage-pane",
  setup(ctx) {
    const disposeGateway = installUsageGateway();
    registerSettingsPane(ctx, {
      id: USAGE_PANE,
      label: "settings.pane.usage",
      group: "models",
      icon: "chart",
      order: 55,
      component: UsagePane,
    });
    ctx.cleanup(disposeGateway);
  },
});
