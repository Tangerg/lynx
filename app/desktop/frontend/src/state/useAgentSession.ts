import type { AgentDriver } from "@/plugins/sdk";
import type { InterruptResponse, RunEvent, RunId, StreamingResult } from "@/rpc";
import { useEffect, useRef } from "react";
import { asSessionId, errorDetail, errorType, RpcError } from "@/rpc";
import { LOCAL_MESSAGE_PREFIX } from "@/protocol/run/viewState";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
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

// Monotonic suffix for optimistic (client-only) user-message item ids, so each
// keeps a unique React key + dodges the fold's dedupe-by-id.
let localSeq = 0;

export function useAgentSession(makeDriver: () => AgentDriver, sessionId: string): AgentSession {
  const factoryRef = useRef(makeDriver);
  factoryRef.current = makeDriver;

  useEffect(() => {
    // Welcome screen (no active session) mounts the kernel chat with an empty
    // id — there is nothing to drive: no slice to seed, and items.list("")
    // would just be a guaranteed-failing RPC on every mount.
    if (!sessionId) return;
    const driver = factoryRef.current();
    const store = () => useAgentStore.getState();

    // Reset this session's slice before streaming so we don't carry state
    // from a previous mount of the same session id.
    store().resetSession(sessionId);

    let abort: AbortController | null = null;
    let currentRunId: RunId | null = null;
    let cancelled = false;
    // Set once a live send/resume writes to this slice. History hydration
    // (items.list) is async; if the user sends before it resolves, applying
    // the backfill afterwards would append the old turns *below* the new
    // message (the fold is arrival-ordered, not timestamp-sorted) and bleed
    // history agentMessages into the open live turn. Skip the late backfill
    // in that race — it'll hydrate cleanly on the next open.
    let interacted = false;

    // Hydrate history for an existing (non-draft) session: replay its
    // completed Items as `item.completed` events through the SAME fold the
    // live stream uses, so past turns render identically. Drafts have no
    // history (just created) — their queued first message is flushed below.
    if (!useSessionStore.getState().draftSessionIds.has(sessionId)) {
      void getContainer()
        .client()
        .items.list({ sessionId: asSessionId(sessionId) })
        .then((resp) => {
          if (cancelled || interacted || resp.data.length === 0) return;
          store().applyEvents(
            sessionId,
            resp.data.map((item): RunEvent["event"] => ({ type: "item.completed", item })),
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
      run: (
        signal: AbortSignal,
      ) => Promise<StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>>,
      onResult?: (result: { runId: RunId; userItemId?: string }) => void,
      onStartError?: () => void,
    ): void => {
      interacted = true; // a live run now owns this slice; gate late history
      abort?.abort(); // a new run supersedes any in-flight one
      const ctrl = new AbortController();
      abort = ctrl;
      // One coarse span for the whole run. `withSpan` makes it the active
      // context for the SYNCHRONOUS dispatch into transport.send, so the rpc
      // CLIENT span nests under it and the injected traceparent links the
      // backend trace to this run.
      const span = startRunSpan({ "lyra.session_id": sessionId });
      let failure: unknown;
      void withSpan(span, () => run(ctrl.signal))
        .then((stream) => {
          // Runs before pump() iterates events (the response resolves ahead of
          // the buffered stream frames), so a userItemId relabel lands before
          // the streamed userMessage Item is folded.
          if (!cancelled) onResult?.(stream.result);
          span.setAttribute("lyra.run_id", stream.result.runId);
          return pump(stream);
        })
        .catch((err: unknown) => {
          failure = err;
          if (cancelled) return;
          console.error("[agent] run failed to start:", sessionId, err);
          // Channel-a failure (API.md §8.1): the call rejected, so no stream
          // and no run.finished{error} will arrive — surface it on the banner
          // ourselves instead of failing silently.
          if (err instanceof RpcError)
            store().setError(sessionId, {
              message: errorDetail(err.data) ?? err.message,
              code: errorType(err.data),
            });
          // Let the caller roll back optimistic UI (send re-entrancy latch,
          // HITL card pending state) now that we know the run never opened.
          onStartError?.();
        })
        .finally(() => endSpan(span, failure));
    };

    // Synchronous re-entrancy latch for the pre-run.started window. The view's
    // run.running (the steady-state guard in useChatSend) only flips true when
    // the run.started event arrives — a full round-trip after send(). Without
    // this latch a second Enter inside that window passes the running guard and
    // fires a second runs.start: two optimistic bubbles, two backend runs, and
    // the first bubble orphaned (its localId never relabeled). Cleared the
    // moment the run starts (onResult) or fails to start (onStartError).
    let starting = false;

    const send = (text: string): void => {
      if (starting) return;
      starting = true;
      // Optimistically render the user's own bubble with a local id. The
      // runtime DOES stream the userMessage Item back (with its own server id),
      // a round-trip later — so when runs.start resolves we relabel this
      // placeholder to the returned `userItemId`, and the streamed Item then
      // dedupes by exact id (no duplicate, no content-text heuristic).
      const localId = `${LOCAL_MESSAGE_PREFIX}${++localSeq}`;
      store().applyEvents(sessionId, [
        {
          type: "item.completed",
          item: {
            id: localId,
            runId: "",
            status: "completed",
            createdAt: new Date().toISOString(),
            type: "userMessage",
            content: [{ type: "text", text }],
          },
        } as RunEvent["event"],
      ]);
      begin(
        (signal) => driver.start(text, signal),
        (result) => {
          starting = false;
          if (result.userItemId) store().relabelMessage(sessionId, localId, result.userItemId);
        },
        () => {
          starting = false;
        },
      );
      // First message graduates a draft session into the sidebar.
      useSessionStore.getState().graduateDraft(sessionId);
    };

    const resume = (
      parentRunId: RunId,
      responses: InterruptResponse[],
      onSettled?: () => void,
      onStartError?: () => void,
    ): void => {
      begin(
        (signal) => driver.resume(parentRunId, responses, signal),
        onSettled ? () => onSettled() : undefined,
        onStartError,
      );
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
