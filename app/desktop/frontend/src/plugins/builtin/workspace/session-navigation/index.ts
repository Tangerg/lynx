import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeActiveSessionId,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";
import {
  activateWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { bindWorkspaceSessionNavigation } from "./application/sessionNavigationSync";

export default definePlugin({
  name: "lyra.builtin.workspace.session-navigation",
  version: "1.0.0",
  requires: ["lyra.builtin.agent-bootstrap", "lyra.builtin.workspace-bootstrap"],
  setup() {
    return bindWorkspaceSessionNavigation({
      activeSessionId: getActiveSessionId,
      lifecycleSnapshot: getAgentSessionLifecycleSnapshot,
      subscribeActiveSessionId,
      subscribeLifecycle: subscribeAgentSessionLifecycle,
      activateSessionScope: activateWorkspaceSessionScope,
      forgetSessionScopes: forgetWorkspaceSessionScopes,
    });
  },
});
