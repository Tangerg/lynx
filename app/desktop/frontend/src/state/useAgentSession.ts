import type { AbstractAgent } from "@ag-ui/client";
import type { BaseEvent } from "@ag-ui/core";
import { useEffect, useRef } from "react";
import { useAgentStore } from "./agentStore";
import { useSessionStore } from "./sessionStore";

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
    useAgentStore.getState().setSend(sessionId, (text: string) => sendVia(agent, sessionId, text));

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
    // A real run begins when the user sends; replaying an existing session's
    // history is a separate concern (messages.list).
    //
    // Exception: a message typed on the welcome screen (no active session)
    // was queued by useCreateSession against this freshly-created draft —
    // flush it now that the agent for this id is live.
    const pending = useSessionStore.getState().takePendingMessage(sessionId);
    if (pending) sendVia(agent, sessionId, pending);

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
      if (agent) sendVia(agent, sessionId, text);
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

// Send a message + graduate the session out of draft state (its first
// message means it's no longer an empty draft, so it should appear in the
// sidebar). Both send entry points — the store-side `setSend` and the
// hook's returned `send` — go through here so the behaviour can't drift.
function sendVia(agent: AbstractAgent, sessionId: string, text: string): void {
  sendText(agent, text);
  useSessionStore.getState().graduateDraft(sessionId);
}

// Append a user message then kick a new run.
function sendText(agent: AbstractAgent, text: string): void {
  agent.addMessage({
    id: `user_${Date.now()}`,
    role: "user",
    content: text,
  });
  void agent.runAgent();
}
