import { subscribeAgentSessionSelection } from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";
import {
  clearWorkspaceSessionState,
  selectWorkspaceChat,
} from "@/plugins/builtin/workspace/application/navigation";

export default definePlugin({
  name: "lyra.builtin.workspace.session-navigation",
  version: "1.0.0",
  setup() {
    return subscribeAgentSessionSelection((state, prev) => {
      if (state.selectionEpoch !== prev.selectionEpoch) selectWorkspaceChat();
      if (state.activeSessionId !== prev.activeSessionId) clearWorkspaceSessionState();
    });
  },
});
