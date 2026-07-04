import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getContainer } from "@/main/container";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { asSessionId } from "@/rpc";
import { activeRpcSessionId, createRpcAgentDriver } from "./application/rpcAgentDriver";

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(AGENT_SOURCE, {
      id: "rpc",
      label: t("agentSource.rpc"),
      priority: 1,
      factory: () =>
        createRpcAgentDriver(activeRpcSessionId(getActiveSessionId()), () => ({
          start: ({ sessionId, ...params }, signal) =>
            getContainer()
              .client()
              .runs.start({ ...params, sessionId: asSessionId(sessionId) }, signal),
          resume: (params, signal) => getContainer().client().runs.resume(params, signal),
        })),
    });
  },
});
