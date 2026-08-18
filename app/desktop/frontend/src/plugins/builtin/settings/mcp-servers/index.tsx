import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { MCP_SERVERS_PANE } from "../public/panes";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { registerMCPDataProviders } from "./adapters/runtimeMcpDataProviders";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const McpServersPane = lazy(() =>
  import("./ui/McpServersPane").then(({ McpServersPane }) => ({ default: McpServersPane })),
);

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installMCPServerGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerMCPDataProviders(ctx);
    registerSettingsPane(ctx, {
      id: MCP_SERVERS_PANE,
      label: "settings.pane.mcpServers",
      group: "integrations",
      icon: "tool",
      order: 56,
      component: McpServersPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
