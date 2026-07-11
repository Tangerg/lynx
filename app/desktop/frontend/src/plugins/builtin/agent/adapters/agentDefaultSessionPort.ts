import { useCallback } from "react";
import { pickAgentSource } from "@/plugins/sdk";
import { configureAgentDefaultSessionPort } from "../application/ports/defaultSession";
import { useAgentSessionStore } from "./agentSessionStore";
import { useAgentSession } from "./useAgentSession";

export function installAgentDefaultSessionPort(): () => void {
  return configureAgentDefaultSessionPort({
    useDefaultChatSession,
  });
}

function useDefaultChatSession() {
  const activeSessionId = useAgentSessionStore((s) => s.activeSessionId);
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
