// v2 Runtime Protocol fold — every run.* / item.* / state.* StreamEvent
// case lives in handlers.ts. Pluginifying this lets a custom dialect
// register a higher-priority fold.

import { definePlugin } from "@/plugins/sdk";
import { HANDLERS } from "../application/fold/handlers";

export default definePlugin({
  name: "lyra.builtin.agent-fold",
  version: "1.0.0",
  setup({ host }) {
    for (const [type, handler] of HANDLERS) {
      host.events.onStream(type, handler);
    }
  },
});
