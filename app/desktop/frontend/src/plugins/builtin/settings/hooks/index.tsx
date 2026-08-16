// Built-in plugin: "Hooks" settings pane. Registration only — the UI lives in
// ui/HooksPane, the hook-trust gateway install in adapters/, the RPC use cases
// in application/.

import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import { hooksSettingsPane } from "./application/hooksContributions";

const HooksPane = lazy(() =>
  import("./ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export default definePlugin({
  name: "lyra.builtin.hooks-pane",
  setup(ctx) {
    const disposeGateway = installHookTrustGateway();
    registerSettingsPane(ctx, hooksSettingsPane(HooksPane));
    ctx.cleanup(disposeGateway);
  },
});
