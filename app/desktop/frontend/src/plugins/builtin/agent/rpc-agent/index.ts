// Built-in plugin: the default agent driver — drives the chat over the
// Lyra Runtime Protocol (JSON-RPC `runs.start` / `runs.resume`, streaming
// `notifications.run.event`). The factory binds a driver to the active
// session; `useAgentSession` pumps the returned RunEvent stream into the
// store and owns abort/cancel.

import type { AgentDriver } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SOURCE } from "@/plugins/sdk/kernelPoints";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";

function makeDriver(sessionId: string): AgentDriver {
  const client = () => getContainer().client();
  return {
    start: (text, signal) =>
      client().runs.start(
        { sessionId: asSessionId(sessionId), input: [{ type: "text", text }], mode: "agent" },
        signal,
      ),
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
      label: "Runtime Protocol (JSON-RPC)",
      priority: 1,
      factory: () => makeDriver(useSessionStore.getState().activeSessionId || "ses_default"),
    });
  },
});
