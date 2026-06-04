import type { AgentDriver } from "@/plugins/sdk";
import type { InterruptResponse, RunEvent, RunId, StreamingResult } from "@/rpc";
import { useEffect, useRef } from "react";
import { asSessionId } from "@/rpc";
import { getContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { useSessionStore } from "./sessionStore";

// Owns the agent driver lifecycle for one session: build the driver, expose
// imperative send / stop / resume, and pump each run's RunEvent stream into
// useAgentStore (batched per frame). Changing sessionId rebuilds; the
// previous session's view state stays in the store (so switching back shows
// what was there).
export interface AgentSession {
  send: (text: string) => void;
  stop: () => void;
}

export function useAgentSession(makeDriver: () => AgentDriver, sessionId: string): AgentSession {
  const factoryRef = useRef(makeDriver);
  factoryRef.current = makeDriver;

  useEffect(() => {
    const driver = factoryRef.current();
    const store = () => useAgentStore.getState();

    // Reset this session's slice before streaming so we don't carry state
    // from a previous mount of the same session id.
    store().resetSession(sessionId);

    let abort: AbortController | null = null;
    let currentRunId: RunId | null = null;
    let cancelled = false;

    // Hydrate history for an existing (non-draft) session: replay its
    // completed Items as `item.completed` events through the SAME fold the
    // live stream uses, so past turns render identically. Drafts have no
    // history (just created) — their queued first message is flushed below.
    if (!useSessionStore.getState().draftSessionIds.has(sessionId)) {
      void getContainer()
        .client()
        .items.list({ sessionId: asSessionId(sessionId) })
        .then((resp) => {
          if (cancelled || resp.items.length === 0) return;
          store().applyEvents(
            sessionId,
            resp.items.map((item): RunEvent["event"] => ({ type: "item.completed", item })),
          );
        })
        .catch((err: unknown) => {
          if (!cancelled) console.error("[agent] history load failed:", sessionId, err);
        });
    }

    // Per-session rAF batcher. A run streams many item.delta events per
    // second; without batching each one triggers a store.set + React
    // commit. Coalescing into one flush per animation frame caps that at
    // ~1 commit per frame without changing perceived token latency.
    let queue: RunEvent["event"][] = [];
    let raf: number | null = null;
    const flush = () => {
      raf = null;
      if (cancelled || queue.length === 0) return;
      const batch = queue;
      queue = [];
      store().applyEvents(sessionId, batch);
    };
    const enqueue = (event: RunEvent["event"]) => {
      queue.push(event);
      if (raf === null) raf = requestAnimationFrame(flush);
    };

    const pump = async (stream: StreamingResult<{ runId: RunId }, RunEvent>): Promise<void> => {
      currentRunId = stream.result.runId;
      try {
        for await (const ev of stream.events) {
          if (cancelled) break;
          enqueue(ev.event);
        }
      } catch (err) {
        if (!cancelled) console.error("[agent] run stream failed:", sessionId, err);
      }
    };

    const begin = (
      run: (signal: AbortSignal) => Promise<StreamingResult<{ runId: RunId }, RunEvent>>,
    ): void => {
      abort?.abort(); // a new run supersedes any in-flight one
      abort = new AbortController();
      void run(abort.signal)
        .then(pump)
        .catch((err: unknown) => {
          if (!cancelled) console.error("[agent] run failed to start:", sessionId, err);
        });
    };

    const send = (text: string): void => {
      begin((signal) => driver.start(text, signal));
      // First message graduates a draft session into the sidebar.
      useSessionStore.getState().graduateDraft(sessionId);
    };

    const resume = (parentRunId: RunId, responses: InterruptResponse[]): void => {
      begin((signal) => driver.resume(parentRunId, responses, signal));
    };

    const stop = (): void => {
      abort?.abort();
      if (currentRunId)
        void getContainer()
          .client()
          .runs.cancel(currentRunId)
          .catch(() => undefined);
    };

    store().setSend(sessionId, send);
    store().setStop(sessionId, stop);
    store().setResume(sessionId, resume);

    // A message typed on the welcome screen (no active session) was queued
    // by useCreateSession against this freshly-created draft — flush it now
    // that the driver for this id is live. Opening a session otherwise does
    // NOT auto-run; replaying history is a separate concern (items.list).
    const pending = useSessionStore.getState().takePendingMessage(sessionId);
    if (pending) send(pending);

    return () => {
      cancelled = true;
      if (raf !== null) cancelAnimationFrame(raf);
      abort?.abort();
      store().setSend(sessionId, null);
      store().setStop(sessionId, null);
      store().setResume(sessionId, null);
    };
  }, [sessionId]);

  return {
    send: (text: string) => useAgentStore.getState().sessions[sessionId]?.send?.(text),
    stop: () => useAgentStore.getState().sessions[sessionId]?.stop?.(),
  };
}
