import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { runtimeRunsGateway } from "./adapters/runtimeRunsGateway";
import { rpcAgentSource } from "./application/rpcAgentSource";

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  setup(ctx) {
    const gateway = runtimeRunsGateway();
    ctx.contribute(
      AGENT_SOURCE,
      rpcAgentSource(t, getActiveSessionId, () => gateway),
    );
    ctx.cleanup(() => gateway.dispose());
  },
});
