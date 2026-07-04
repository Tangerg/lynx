import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installRuntimeConnection } from "./application/runtimeConnection";
import { ConnectionPane } from "./ui/ConnectionPane";

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  version: "1.0.0",
  setup({ host }) {
    installRuntimeConnection(host);
    registerSettingsPane(host, {
      id: "connection",
      label: "settings.pane.connection",
      group: "general",
      icon: "globe",
      order: 5,
      component: ConnectionPane,
    });
  },
});
