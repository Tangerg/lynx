import { AGENT_RUN_OPTIONS, definePlugin } from "@/plugins/sdk";
import { composerModelRunOptions } from "./application/composerContributions";
import { selectedComposerModelPreference } from "./public/modelPreference";

export const composerRunOptions = definePlugin({
  name: "lyra.builtin.composer-run-options",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(
      AGENT_RUN_OPTIONS,
      composerModelRunOptions(() => {
        const { provider, model } = selectedComposerModelPreference();
        return provider && model ? { provider, model } : {};
      }),
    );
  },
});
