import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { McpServersPane } from "./ui/McpServersPane";

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  version: "1.0.0",
  setup({ host }) {
    installMCPServerGateway();
    registerSettingsPane(host, {
      id: "mcp-servers",
      label: "settings.pane.mcpServers",
      group: "integrations",
      icon: "tool",
      order: 56,
      component: McpServersPane,
    });
  },
});
