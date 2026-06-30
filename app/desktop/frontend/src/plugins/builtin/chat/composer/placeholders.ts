import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_PLACEHOLDER } from "@/plugins/sdk/kernelPoints";

const placeholders = [
  { id: "ask", text: "composer.placeholder.fallback" },
  { id: "debug", text: "composer.placeholder.debug" },
  { id: "implement", text: "composer.placeholder.implement" },
  { id: "refactor", text: "composer.placeholder.refactor" },
];

export const composerPlaceholders = definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    for (const placeholder of placeholders) {
      host.extensions.contribute(COMPOSER_PLACEHOLDER, placeholder);
    }
  },
});
