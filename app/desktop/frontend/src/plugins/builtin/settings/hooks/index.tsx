// Built-in plugin: "Hooks" settings pane. Registration only — the UI lives in
// ui/HooksPane, the hook-trust gateway install in adapters/, the RPC use cases
// in application/.

import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import { HooksPane } from "./ui/HooksPane";

export default definePlugin({
  name: "lyra.builtin.hooks-pane",
  version: "1.0.0",
  setup({ host }) {
    installHookTrustGateway();
    host.extensions.contribute(SETTINGS_PANE, {
      id: "hooks",
      label: "settings.pane.hooks",
      group: "agent",
      icon: "lightning",
      // After MCP servers (56) — both extend "what runs around the agent";
      // hooks are the lifecycle-command surface.
      order: 57,
      component: HooksPane,
    });
  },
});
