// Default chat session bootstrap.
//
// Resolves the active agent source from the plugin registry, then hands
// the factory to `useAgentSession`. Wraps the boilerplate that every
// "chat container" component would otherwise duplicate: pick source →
// factory → useAgentSession → done.
//
// `activeSessionId` from useUIStore is passed as the sessionKey so the
// agent is torn down + rebuilt when the user switches sessions. The
// http-agent built-in reads the same store inside its factory to bind
// the new agent's threadId, so the backend sees the new session id on
// first runAgent() and picks the right demo script.

import { useCallback } from "react";
import { pickAgentSource } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";
import { useAgentSession, type AgentSession } from "./useAgentSession";

export function useDefaultChatSession(): AgentSession {
  const activeSessionId = useUIStore((s) => s.activeSessionId);
  return useAgentSession(
    useCallback(() => {
      const source = pickAgentSource();
      if (!source) throw new Error("No agent source registered");
      return source.factory();
    }, [activeSessionId]),
    activeSessionId,
  );
}
