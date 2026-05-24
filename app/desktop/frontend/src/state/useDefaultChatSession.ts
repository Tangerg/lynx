// Default chat session bootstrap.
//
// Resolves the active agent source from the plugin registry, then hands
// the factory to `useAgentSession`. Wraps the boilerplate that every
// "chat container" component would otherwise duplicate: pick source →
// factory → useAgentSession → done.
//
// `activeSessionId` from useSessionStore is passed as the sessionId so
// the agent is torn down + rebuilt when the user switches sessions, and so
// the agentStore knows which session's slice to write events into. The
// http-agent built-in reads the same store inside its factory to bind
// the new agent's threadId, so the backend sees the new session id on
// first runAgent() and picks the right demo script.

import { useCallback } from "react";
import { pickAgentSource } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { useAgentSession, type AgentSession } from "./useAgentSession";

export function useDefaultChatSession(): AgentSession {
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  return useAgentSession(
    useCallback(() => {
      const source = pickAgentSource();
      if (!source) throw new Error("No agent source registered");
      return source.factory();
      // activeSessionId is intentionally pinned in deps: the callback
      // closes over no session id directly, but useAgentSession uses
      // the callback identity as its rebuild key. Re-creating the
      // callback when the active session changes tears down the old
      // agent and stands up a fresh one bound to the new session.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [activeSessionId]),
    activeSessionId,
  );
}
