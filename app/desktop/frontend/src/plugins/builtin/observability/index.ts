import { definePlugin } from "@/plugins/sdk";
import { initFrontendObservability } from "./frontendObservability";
import { startObservability } from "./observabilityLifecycle";

export default definePlugin({
  name: "lyra.builtin.observability",
  setup(ctx) {
    ctx.cleanup(
      startObservability(initFrontendObservability, (error) => {
        console.warn("[observability] initialization failed; running without telemetry:", error);
      }),
    );
  },
});
