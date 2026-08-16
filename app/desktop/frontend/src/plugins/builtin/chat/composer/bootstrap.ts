import { definePlugin } from "@/plugins/sdk";
import { installComposerStatePorts } from "./adapters/composerStatePorts";

export const composerBootstrap = definePlugin({
  name: "lyra.builtin.composer-bootstrap",
  setup(ctx) {
    ctx.cleanup(installComposerStatePorts());
  },
});
