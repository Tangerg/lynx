import { definePlugin } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { slashHintContributions } from "./application/slashHints";

export default definePlugin({
  name: "scopeapp.builtin.slash-hints",
  setup(ctx) {
    for (const { cmd, spec } of slashHintContributions()) {
      ctx.contribute(SLASH_COMMAND, spec, { key: cmd });
    }
  },
});
