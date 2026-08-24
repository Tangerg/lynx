import { AGENT_RUN_OPTIONS, definePlugin } from "@/plugins/sdk";
import { resolveComposerRunOptions } from "./application/modelSelection";
import { selectedComposerModelPreference } from "./public/modelPreference";

export const composerRunOptions = definePlugin({
  name: "lyra.builtin.composer-run-options",
  setup(ctx) {
    ctx.contribute(AGENT_RUN_OPTIONS, {
      id: "composer.model",
      priority: 0,
      resolve: () => resolveComposerRunOptions(selectedComposerModelPreference()),
    });
  },
});
