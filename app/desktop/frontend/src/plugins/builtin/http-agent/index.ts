// Built-in plugin: registers the default HttpAgent against the local Go
// backend as the active AG-UI agent source. Other plugins can override by
// registering a higher-priority source (e.g. a WebSocket transport, a
// recorded-fixture replayer, etc.).

import { HttpAgent } from "@ag-ui/client";
import { AGUI_BASE } from "@/lib/http";
import { definePlugin } from "@/plugins/sdk";

const AGUI_URL = `${AGUI_BASE}/run`;

export default definePlugin({
  name: "lyra.builtin.http-agent",
  version: "1.0.0",
  setup({ host }) {
    host.agent.registerSource({
      id: "http",
      label: "HTTP (local backend)",
      priority: 0,
      factory: () => new HttpAgent({ url: AGUI_URL, threadId: "thread_demo" }),
    });
  },
});
