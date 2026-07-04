import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { slashHintContributions } from "./application/slashHints";

export default definePlugin({
  name: "lyra.builtin.slash-hints",
  version: "1.0.0",
  setup({ host }) {
    for (const { cmd, spec } of slashHintContributions(t)) {
      host.extensions.contribute(SLASH_COMMAND, spec, { key: cmd });
    }
  },
});
