// Built-in plugin: "Hooks" settings pane. Registration only — the UI lives in
// ui/HooksPane, the hook-trust gateway install in adapters/, the RPC use cases
// in application/.

import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import { hooksSettingsPane } from "./application/hooksContributions";
import { HooksPane } from "./ui/HooksPane";

export default definePlugin({
  name: "lyra.builtin.hooks-pane",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installHookTrustGateway();
    registerSettingsPane(host, hooksSettingsPane(HooksPane));
    return disposeGateway;
  },
});
