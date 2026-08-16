import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { WORKSPACE_SCOPE_PORTS } from "@/plugins/builtin/workspace/public/ports";
import { bindWorkspaceSessionNavigation } from "./application/sessionNavigationSync";

export default definePlugin({
  name: "lyra.builtin.workspace.session-navigation",
  requires: { sessions: AGENT_SESSION_PORTS, scopes: WORKSPACE_SCOPE_PORTS },
  setup(ctx) {
    ctx.cleanup(bindWorkspaceSessionNavigation({ ...ctx.sessions, ...ctx.scopes }));
  },
});
