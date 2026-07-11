import {
  getActiveSessionId,
  getAgentSessionLifecycleSnapshot,
  subscribeAgentSessionLifecycle,
  subscribeAgentSessionSelection,
} from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";
import {
  activateWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
  selectWorkspaceChat,
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
      subscribeSelection: subscribeAgentSessionSelection,
      subscribeLifecycle: subscribeAgentSessionLifecycle,
      activateSessionScope: activateWorkspaceSessionScope,
      forgetSessionScopes: forgetWorkspaceSessionScopes,
      selectChat: selectWorkspaceChat,
    });
  },
});
