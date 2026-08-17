import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { runtimeRunsGateway } from "./adapters/runtimeRunsGateway";
import { rpcAgentSource } from "./application/rpcAgentSource";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = runtimeRunsGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    ctx.contribute(
      AGENT_SOURCE,
      rpcAgentSource(t, getActiveSessionId, () => gateway),
    );
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
