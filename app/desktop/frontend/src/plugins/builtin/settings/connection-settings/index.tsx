import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { connectionSettingsPane } from "./application/connectionContributions";
import { ConnectionPane } from "./ui/ConnectionPane";

export default definePlugin({
  name: "lyra.builtin.connection-settings",
  version: "1.0.0",
  requires: ["lyra.builtin.runtime"],
  setup({ host }) {
    registerSettingsPane(host, connectionSettingsPane(ConnectionPane));
  },
});
