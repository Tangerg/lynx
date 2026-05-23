// Built-in plugin: registers the default HttpAgent against the local Go
// backend as the active AG-UI agent source. Other plugins can override by
// registering a higher-priority source (e.g. a WebSocket transport, a
// recorded-fixture replayer, etc.).
//
// The factory reads the active session id from useSessionStore each
// time it's called — useDefaultChatSession re-invokes the factory
// whenever the active session changes, so the new agent's threadId
// tracks the user's current selection. Backend uses that threadId to
// pick a demo script from `internal/agui/demos.go`.

import { HttpAgent } from "@ag-ui/client";
import { AGUI_BASE } from "@/lib/http";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

const AGUI_URL = `${AGUI_BASE}/run`;

export default definePlugin({
  name: "lyra.builtin.http-agent",
  version: "1.0.0",
  setup({ host }) {
    host.agent.registerSource({
      id: "http",
      label: "HTTP (local backend)",
      priority: 0,
      factory: () => {
        const sessionId = useSessionStore.getState().activeSessionId ?? "s1";
        return new HttpAgent({ url: AGUI_URL, threadId: sessionId });
      },
    });
  },
});
