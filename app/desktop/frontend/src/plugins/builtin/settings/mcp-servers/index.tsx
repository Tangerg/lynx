import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { mcpServersSettingsPane } from "./application/mcpServersContributions";

const McpServersPane = lazy(() =>
  import("./ui/McpServersPane").then(({ McpServersPane }) => ({ default: McpServersPane })),
);

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  version: "1.0.0",
  setup({ host }) {
    const disposeGateway = installMCPServerGateway();
    registerSettingsPane(host, mcpServersSettingsPane(McpServersPane));
    return disposeGateway;
  },
});
