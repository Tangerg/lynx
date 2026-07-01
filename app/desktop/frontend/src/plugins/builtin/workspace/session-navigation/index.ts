import { subscribeAgentSessionSelection } from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";

export default definePlugin({
  name: "lyra.builtin.workspace.session-navigation",
  version: "1.0.0",
  setup() {
    return subscribeAgentSessionSelection((state, prev) => {
      const workspace = useWorkspaceNavigationStore.getState();
      if (state.selectionEpoch !== prev.selectionEpoch) workspace.selectChat();
      if (state.activeSessionId !== prev.activeSessionId) workspace.clearSessionScopedState();
    });
  },
});
