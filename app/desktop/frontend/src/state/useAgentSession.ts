import { useEffect, useRef } from "react";
import type { AbstractAgent } from "@ag-ui/client";
import { useAgentStore } from "./agentStore";

// Owns the AG-UI agent lifecycle for one session: instantiate, subscribe
// to events → useAgentStore.applyEvent, expose imperative send / stop.
// Changing sessionId rebuilds; the previous session's view state stays
// in the store (so switching back shows what was there).
export type AgentSession = {
  send: (text: string) => void;
  stop: () => void;
};

export function useAgentSession(makeAgent: () => AbstractAgent, sessionId: string): AgentSession {
  const agentRef = useRef<AbstractAgent | null>(null);

  const factoryRef = useRef(makeAgent);
  factoryRef.current = makeAgent;

  useEffect(() => {
    const agent = factoryRef.current();
    agentRef.current = agent;

    // Reset this session's slice before subscribing so we don't carry
    // state from a previous mount of the same session id.
    useAgentStore.getState().resetSession(sessionId);

    useAgentStore.getState().setStop(sessionId, () => {
      try {
        agent.abortRun();
      } catch {
        /* ignore */
      }
    });
    useAgentStore.getState().setSend(sessionId, (text: string) => {
      agent.addMessage({
        id: `user_${Date.now()}`,
        role: "user",
        content: text,
      });
      void agent.runAgent();
    });

    const subscription = agent.subscribe({
      onEvent: ({ event }) => {
        if (import.meta.env.DEV) {
           
          console.debug("[agui]", sessionId, event.type, event);
        }
        useAgentStore.getState().applyEvent(sessionId, event);
      },
      onRunFailed: ({ error }) => {
         
        console.error("[agui] run failed:", sessionId, error);
      },
    });

    void agent.runAgent().catch((err) => {
       
      console.error("[agui] runAgent threw:", sessionId, err);
    });

    return () => {
      subscription.unsubscribe();
      try {
        agent.abortRun();
      } catch {
        /* may not be running */
      }
      useAgentStore.getState().setStop(sessionId, null);
      useAgentStore.getState().setSend(sessionId, null);
      agentRef.current = null;
    };
     
  }, [sessionId]);

  return {
    send: (text: string) => {
      const agent = agentRef.current;
      if (!agent) return;
      agent.addMessage({
        id: `user_${Date.now()}`,
        role: "user",
        content: text,
      });
      void agent.runAgent();
    },
    stop: () => {
      try {
        agentRef.current?.abortRun();
      } catch {
        /* ignore */
      }
    },
  };
}
