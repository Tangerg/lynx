// Built-in plugin: the default agent driver — drives the chat over the
// Lyra Runtime Protocol (JSON-RPC `runs.start` / `runs.resume`, streaming
// `notifications.run.event`). The factory binds a driver to the active
// session; `useAgentSession` pumps the returned RunEvent stream into the
// store and owns abort/cancel.

import type { AgentDriver } from "@/plugins/sdk";
import type { RunMode } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

// The built-in composer modes use wire RunMode values as their ids, so the
// picker selection forwards verbatim. COMPOSER_MODE is an open extension
// point though — a third-party mode id has no wire meaning, so anything
// outside the RunMode union falls back to "agent" instead of failing the
// run with invalid_params.
const WIRE_MODES: ReadonlySet<string> = new Set<RunMode>(["agent", "chat", "plan"]);

function makeDriver(sessionId: string): AgentDriver {
  const client = () => getContainer().client();
  return {
    start: (input, signal) => {
      // provider + model are a pair (API §7.3): send BOTH or NEITHER. Only one
      // → invalid_params. Both null (no enabled provider picked) = runtime
      // default provider+model.
      const { provider, model, mode } = useComposerStore.getState();
      return client().runs.start(
        {
          sessionId: asSessionId(sessionId),
          input,
          mode: WIRE_MODES.has(mode) ? (mode as RunMode) : "agent",
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
      factory: () => makeDriver(useSessionStore.getState().activeSessionId || "ses_default"),
    });
  },
});
