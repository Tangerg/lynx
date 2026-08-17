import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { registerMCPDataProviders } from "./adapters/runtimeMcpDataProviders";
import { mcpServersSettingsPane } from "./application/mcpServersContributions";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

const McpServersPane = lazy(() =>
  import("./ui/McpServersPane").then(({ McpServersPane }) => ({ default: McpServersPane })),
);

export default definePlugin({
  name: "lyra.builtin.mcp-servers-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installMCPServerGateway();
    let runtimeGeneration = ctx.runtime.runtimeGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.runtimeGeneration();
      if (next === runtimeGeneration) return;
      runtimeGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    registerMCPDataProviders(ctx);
    registerSettingsPane(ctx, mcpServersSettingsPane(McpServersPane));
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
