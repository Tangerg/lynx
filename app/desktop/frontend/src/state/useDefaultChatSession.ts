// Default chat session bootstrap.
//
// Resolves the active agent source from the plugin registry once at
// mount, then hands the factory to `useAgentSession`. Wraps the
// boilerplate that every "chat container" component would otherwise
// duplicate: pick source → factory → useAgentSession → done.
//
// Exposing it as a hook keeps the agent lifecycle decoupled from the
// kernel component — `kernel-chat` only has to call this hook, not know
// about agent sources or factories.

import { useCallback } from "react";
import { pickAgentSource } from "@/plugins/sdk";
import { useAgentSession, type AgentSession } from "./useAgentSession";

export function useDefaultChatSession(): AgentSession {
  return useAgentSession(
    useCallback(() => {
      const source = pickAgentSource();
      if (!source) throw new Error("No agent source registered");
      return source.factory();
    }, []),
  );
}
