import { definePlugin } from "@/plugins/sdk";
import { installComposerStatePorts } from "./adapters/composerStatePorts";

export const composerBootstrap = definePlugin({
  name: "lyra.builtin.composer-bootstrap",
  version: "1.0.0",
  setup() {
    return installComposerStatePorts();
  },
});
