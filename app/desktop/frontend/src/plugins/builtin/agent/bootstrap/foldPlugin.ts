// Runtime Protocol fold composition — wire dispatch is an Adapter concern;
// product folds remain in Application and receive translated state values.

import { definePlugin, STREAM_EVENT_HANDLER } from "@/plugins/sdk";
import { RUNTIME_EVENT_HANDLERS } from "../adapters/runtimeEventHandlers";

export default definePlugin({
  name: "lyra.builtin.agent-fold",
  setup(ctx) {
    for (const [type, handler] of RUNTIME_EVENT_HANDLERS) {
      ctx.contribute(STREAM_EVENT_HANDLER, { eventType: type, handler: handler });
    }
  },
});
