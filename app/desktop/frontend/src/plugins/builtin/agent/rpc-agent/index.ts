// Built-in plugin: the default agent driver — drives the chat over the
// Lyra Runtime Protocol (JSON-RPC `runs.start` / `runs.resume`, streaming
// `notifications.run.event`). The factory binds a driver to the active
// session; `useAgentSession` pumps the returned RunEvent stream into the
// store and owns abort/cancel.

import type { AgentDriver } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getContainer } from "@/main/container";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { asSessionId } from "@/rpc";

function makeDriver(sessionId: string): AgentDriver {
  const client = () => getContainer().client();
  return {
    start: (input, options, signal) => {
      // provider + model are a pair (API §7.3): send BOTH or NEITHER. Only one
      // → invalid_params. Both null (no enabled provider picked) = runtime
      // default provider+model. There is no run `mode` — the run is always the
      // agent loop; "plan" is a global approval stance (approval.setMode), not a
      // per-run mode.
      const { provider, model } = options;
      return client().runs.start(
        {
          sessionId: asSessionId(sessionId),
          input,
          ...(provider && model ? { provider, model } : {}),
        },
        signal,
      );
    },
    resume: (parentRunId, responses, signal) =>
      client().runs.resume({ parentRunId, responses }, signal),
  };
}

export default definePlugin({
  name: "lyra.builtin.rpc-agent",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(AGENT_SOURCE, {
      id: "rpc",
      label: t("agentSource.rpc"),
      priority: 1,
      factory: () => makeDriver(getActiveSessionId() || "ses_default"),
    });
  },
});
