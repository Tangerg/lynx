import type { AbstractAgent } from "@ag-ui/client";
import type { BaseEvent } from "@ag-ui/core";
import { useEffect, useRef } from "react";
import { useAgentStore } from "./agentStore";

// Owns the AG-UI agent lifecycle for one session: instantiate, subscribe
// to events → useAgentStore.applyEvents (batched), expose imperative
// send / stop. Changing sessionId rebuilds; the previous session's view
// state stays in the store (so switching back shows what was there).
export interface AgentSession {
  send: (text: string) => void;
  stop: () => void;
}

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

    // Per-session rAF batcher. AG-UI streams ~30 token-deltas per
    // second; without batching each one triggers a store.set + React
    // commit. Coalescing into one flush per animation frame turns
    // that into ≤ 1 commit per frame (~60 Hz cap) without changing
    // perceived token latency.
    let queue: BaseEvent[] = [];
    let rafHandle: number | null = null;
    let cancelled = false;
    const flush = () => {
      rafHandle = null;
      if (cancelled || queue.length === 0) return;
      const batch = queue;
      queue = [];
      useAgentStore.getState().applyEvents(sessionId, batch);
    };

    const subscription = agent.subscribe({
      onEvent: ({ event }) => {
        if (cancelled) return;
        if (import.meta.env.DEV) {
          console.debug("[agui]", sessionId, event.type, event);
        }
        queue.push(event);
        if (rafHandle === null) {
          rafHandle = requestAnimationFrame(flush);
        }
      },
      onRunFailed: ({ error }) => {
        console.error("[agui] run failed:", sessionId, error);
      },
    });

    void agent.runAgent().catch((err) => {
      console.error("[agui] runAgent threw:", sessionId, err);
    });

    return () => {
      cancelled = true;
      if (rafHandle !== null) cancelAnimationFrame(rafHandle);
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
