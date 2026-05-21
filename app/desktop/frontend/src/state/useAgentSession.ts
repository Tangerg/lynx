import { useEffect, useRef } from "react";
import type { AbstractAgent } from "@ag-ui/client";
import { useAgentStore } from "./agentStore";

// useAgentSession owns the AG-UI agent lifecycle:
//   1. Instantiate the agent (factory).
//   2. Subscribe to every event via AgentSubscriber.onEvent.
//   3. Pipe events through `useAgentStore.applyEvent` so any component (and
//      any plugin) can read view-state slices via `useAgentStore((s) => …)`.
//
// `sessionKey` controls when to rebuild the agent. Pass the active
// thread / session id; when it changes the previous agent is aborted +
// unsubscribed and a fresh one is constructed from `makeAgent()`. The
// agent store is reset across the boundary so the new session starts
// with an empty view.
//
// The hook returns only the imperative actions (`send`, `stop`); state is
// read via the store from anywhere in the tree.
export type AgentSession = {
  send: (text: string) => void;
  stop: () => void;
};

export function useAgentSession(
  makeAgent: () => AbstractAgent,
  sessionKey?: unknown,
): AgentSession {
  const agentRef = useRef<AbstractAgent | null>(null);

  const factoryRef = useRef(makeAgent);
  factoryRef.current = makeAgent;

  useEffect(() => {
    const agent = factoryRef.current();
    agentRef.current = agent;

    // Reset before subscribing so a new session doesn't see leftover
    // state from the previous one.
    useAgentStore.getState().reset();

    useAgentStore.getState().setStop(() => {
      try { agent.abortRun(); } catch { /* ignore */ }
    });
    useAgentStore.getState().setSend((text: string) => {
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
          console.debug("[agui]", event.type, event);
        }
        useAgentStore.getState().applyEvent(event);
      },
      onRunFailed: ({ error }) => {
        // eslint-disable-next-line no-console
        console.error("[agui] run failed:", error);
      },
    });

    void agent.runAgent().catch((err) => {
      // eslint-disable-next-line no-console
      console.error("[agui] runAgent threw:", err);
    });

    return () => {
      subscription.unsubscribe();
      try { agent.abortRun(); } catch { /* may not be running */ }
      useAgentStore.getState().setStop(null);
      useAgentStore.getState().setSend(null);
      agentRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionKey]);

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
      try { agentRef.current?.abortRun(); } catch { /* ignore */ }
    },
  };
}
