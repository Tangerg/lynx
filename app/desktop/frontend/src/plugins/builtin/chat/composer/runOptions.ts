import { AGENT_RUN_OPTIONS, definePlugin } from "@/plugins/sdk";
import { selectedComposerModelPreference } from "./public/modelPreference";

export const composerRunOptions = definePlugin({
  name: "lyra.builtin.composer-run-options",
  setup(ctx) {
    ctx.contribute(AGENT_RUN_OPTIONS, {
      id: "composer.model",
      priority: 0,
      resolve: () => {
        const { provider, model } = selectedComposerModelPreference();
        return provider && model ? { provider, model } : {};
      },
    });
  },
});
