// AG-UI protocol semantics — every RUN_* / TEXT_MESSAGE_* / TOOL_CALL_* /
// REASONING_* case lives in handlers.ts. Splitting this into a plugin
// lets a custom dialect register a higher-priority core-reducer.

import { definePlugin } from "@/plugins/sdk";
import { HANDLERS } from "./handlers";

export default definePlugin({
  name: "lyra.builtin.core-reducer",
  version: "1.0.0",
  setup({ host }) {
    for (const [type, handler] of HANDLERS) {
      host.agui.onCore(type, handler);
    }
  },
});
