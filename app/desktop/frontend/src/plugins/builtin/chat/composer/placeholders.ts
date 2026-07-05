import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_PLACEHOLDER } from "@/plugins/sdk/kernelPoints";
import { composerPlaceholderSpecs } from "./application/composerContributions";

export const composerPlaceholders = definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    for (const placeholder of composerPlaceholderSpecs()) {
      host.extensions.contribute(COMPOSER_PLACEHOLDER, placeholder);
    }
  },
});
