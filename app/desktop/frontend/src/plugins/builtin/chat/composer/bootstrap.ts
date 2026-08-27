import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { installComposerStatePorts } from "./adapters/composerStatePorts";

export const composerBootstrap = definePlugin({
  name: "scopeapp.builtin.composer-bootstrap",
  requires: { sessions: AGENT_SESSION_PORTS },
  setup(ctx) {
    ctx.cleanup(installComposerStatePorts(ctx.sessions));
  },
});
