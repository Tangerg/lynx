import { installAgentStatePorts } from "@/plugins/builtin/agent/adapters/agentStatePorts";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { useAgentStore } from "@/plugins/builtin/agent/adapters/agentStore";
import { projectAgentSessionSnapshot } from "@/plugins/builtin/agent/application/session/sessionSnapshot";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { installRuntimeCapabilityPort } from "@/plugins/builtin/runtime/adapters/runtimeCapabilityStore";
import {
  AGENT_SESSION_SNAPSHOTS,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";

export function installVisualAgentFixture(state: VisualAgentState): AgentSessionView {
  installRuntimeCapabilityPort();
  installAgentStatePorts();

  const view = projectAgentSessionSnapshot(AGENT_SESSION_SNAPSHOTS[state]);
  useAgentSessionStore.setState({
    activeSessionId: VISUAL_SESSION_ID,
    openSessionIds: [VISUAL_SESSION_ID],
    draftSessionIds: new Set(),
    pendingMessages: {},
  });

  const store = useAgentStore.getState();
  store.ensureSession(VISUAL_SESSION_ID);
  const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
  if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, view)) {
    throw new Error(`Failed to install visual agent state "${state}"`);
  }

  return view;
}
