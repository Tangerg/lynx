import { definePlugin } from "@/plugins/sdk";
import { CONTEXT_DOCK_DESTINATION } from "@/plugins/sdk/kernelPoints";
import { builtinContextDockDestinations } from "../application/contextDockDestinations";

export default definePlugin({
  name: "scopeapp.builtin.context-dock-destinations",
  setup(ctx) {
    for (const destination of builtinContextDockDestinations) {
      ctx.contribute(CONTEXT_DOCK_DESTINATION, destination);
    }
  },
});
