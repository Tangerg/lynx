// Resolves the active agent source from the plugin registry and hands
// the factory to useAgentSession. Switching activeSessionId rebuilds
// the agent so the backend sees the new session id on first runAgent().

import type { AgentSession } from "./useAgentSession";
import { useCallback } from "react";
import { pickAgentSource } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { useAgentSession } from "./useAgentSession";

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
      // eslint-disable-next-line react/exhaustive-deps
    }, [activeSessionId]),
    activeSessionId,
  );
}
