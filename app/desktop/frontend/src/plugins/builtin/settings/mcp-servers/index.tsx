import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { mcpServersSettingsPane } from "./application/mcpServersContributions";
import { McpServersPane } from "./ui/McpServersPane";

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installMCPServerGateway();
    registerSettingsPane(host, mcpServersSettingsPane(McpServersPane));
    return disposeGateway;
  },
});
