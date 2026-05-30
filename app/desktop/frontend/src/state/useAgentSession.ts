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
    useAgentStore.getState().setSend(sessionId, (text: string) => sendText(agent, text));

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
        // No per-event console.debug: AG-UI emits ~30 events/sec during
        // streaming and each console call retains a reference to the
        // event payload. Over a long session the DevTools console
        // buffer can hold tens of thousands of full event objects,
        // which manifests as visible UI lag + memory growth. Inspect
        // events from the Diagnostics view instead.
        queue.push(event);
        if (rafHandle === null) {
          rafHandle = requestAnimationFrame(flush);
        }
      },
      onRunFailed: ({ error }) => {
        console.error("[agui] run failed:", sessionId, error);
      },
    });

    // No auto-run on mount. Opening a session must NOT start a run — that
    // was demo-only behaviour (the mock played a script for empty messages).
    // A real run begins when the user sends (sendText below); replaying an
    // existing session's history is a separate concern (messages.list).

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
      if (agent) sendText(agent, text);
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

// Append a user message then kick a new run. Used by both the
// store-side `setSend` and the hook's returned `send` so the
// "append + run" pair can't drift across the two call sites.
function sendText(agent: AbstractAgent, text: string): void {
  agent.addMessage({
    id: `user_${Date.now()}`,
    role: "user",
    content: text,
  });
  void agent.runAgent();
}
