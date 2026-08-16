import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { registerMCPDataProviders } from "./adapters/runtimeMcpDataProviders";
import { mcpServersSettingsPane } from "./application/mcpServersContributions";

const McpServersPane = lazy(() =>
  import("./ui/McpServersPane").then(({ McpServersPane }) => ({ default: McpServersPane })),
);

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  setup(ctx) {
    const disposeGateway = installMCPServerGateway();
    registerMCPDataProviders(ctx);
    registerSettingsPane(ctx, mcpServersSettingsPane(McpServersPane));
    ctx.cleanup(disposeGateway);
  },
});
