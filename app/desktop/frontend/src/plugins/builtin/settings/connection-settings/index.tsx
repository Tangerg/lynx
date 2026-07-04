import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { connectionSettingsPane } from "./application/connectionContributions";
import { installRuntimeConnection } from "./application/runtimeConnection";
import { ConnectionPane } from "./ui/ConnectionPane";

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  version: "1.0.0",
  setup({ host }) {
    installRuntimeConnection(host);
    registerSettingsPane(host, connectionSettingsPane(ConnectionPane));
  },
});
