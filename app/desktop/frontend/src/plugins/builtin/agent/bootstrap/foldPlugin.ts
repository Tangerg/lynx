// Runtime Protocol fold composition — wire dispatch is an Adapter concern;
// product folds remain in Application and receive translated state values.

import { definePlugin } from "@/plugins/sdk";
import { RUNTIME_EVENT_HANDLERS } from "../adapters/runtimeEventHandlers";

export default definePlugin({
  name: "lyra.builtin.agent-fold",
  version: "1.0.0",
  setup({ host }) {
    for (const [type, handler] of RUNTIME_EVENT_HANDLERS) {
      host.events.onStream(type, handler);
    }
  },
});
