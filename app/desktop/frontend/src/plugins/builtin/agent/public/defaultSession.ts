// Resolves the active agent source from the plugin registry and hands
// the factory to useAgentSession. Switching activeSessionId rebuilds
// the agent so the backend sees the new session id on first runAgent().

import { agentDefaultSession, type AgentSession } from "../application/ports/defaultSession";

export type { AgentSession } from "../application/ports/defaultSession";

export function useDefaultChatSession(): AgentSession {
  return agentDefaultSession().useDefaultChatSession();
}
