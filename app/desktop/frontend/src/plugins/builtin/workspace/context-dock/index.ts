import { definePlugin } from "@/plugins/sdk";
import { CONTEXT_DOCK_DESTINATION } from "@/plugins/sdk/kernelPoints";
import { builtinContextDockDestinations } from "../application/contextDockDestinations";

export default definePlugin({
  name: "lyra.builtin.context-dock-destinations",
  version: "1.0.0",
  setup({ host }) {
    for (const destination of builtinContextDockDestinations) {
      host.extensions.contribute(CONTEXT_DOCK_DESTINATION, destination);
    }
  },
});
