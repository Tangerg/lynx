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
} from "@/plugins/builtin/workspace/application/navigation";

export default definePlugin({
  name: "lyra.builtin.workspace.session-navigation",
  version: "1.0.0",
  setup() {
    activateWorkspaceSessionScope(getActiveSessionId());
    forgetWorkspaceSessionScopes(getAgentSessionLifecycleSnapshot().openSessionIds);

    const unsubscribeSelection = subscribeAgentSessionSelection((state, prev) => {
      if (state.selectionEpoch !== prev.selectionEpoch) selectWorkspaceChat();
      if (state.activeSessionId !== prev.activeSessionId) {
        activateWorkspaceSessionScope(state.activeSessionId);
      }
    });
    const unsubscribeLifecycle = subscribeAgentSessionLifecycle((state) => {
      forgetWorkspaceSessionScopes(state.openSessionIds);
    });

    return () => {
      unsubscribeSelection();
      unsubscribeLifecycle();
    };
  },
});
