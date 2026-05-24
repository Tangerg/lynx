import { useEffect, useRef } from "react";
import type { AbstractAgent } from "@ag-ui/client";
import { useAgentStore } from "./agentStore";

// useAgentSession owns the AG-UI agent lifecycle for ONE session:
//   1. Instantiate the agent (factory).
//   2. Subscribe to every event via AgentSubscriber.onEvent.
//   3. Pipe events through `useAgentStore.applyEvent(sessionId, event)`
//      so each session keeps its own view-state slice.
//
// `sessionId` controls when to rebuild the agent. When it changes the
// previous agent is aborted + unsubscribed and a fresh one is constructed
// from `makeAgent()`. The departing session's view state stays in the
// store (so going back to it shows what was there), but its `stop`/`send`
// bindings are cleared.
//
// Returns the imperative actions (`send`, `stop`) for the CURRENT
// session. Other components can read the same pair off the store via
// `useAgentAction("stop")` / `useAgentAction("send")` without prop drilling.
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
          // eslint-disable-next-line no-console
          console.debug("[agui]", sessionId, event.type, event);
        }
        useAgentStore.getState().applyEvent(sessionId, event);
      },
      onRunFailed: ({ error }) => {
        // eslint-disable-next-line no-console
        console.error("[agui] run failed:", sessionId, error);
      },
    });

    void agent.runAgent().catch((err) => {
      // eslint-disable-next-line no-console
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
