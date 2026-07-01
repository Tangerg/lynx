// Agent lifecycle hook — bridges the plugin-registered AgentDriver (rpc-agent)
// to the Zustand agentStore. Owns the run state machine (idle → running → waiting
// → running → idle), pipes streaming RunEvents into the protocol fold, and
// provides the imperative send/stop/resume actions the UI binds to buttons.
//
// Kept as a plain hook (not a class / state machine library) because the run
// lifecycle is essentially one rAF-loop subscription + a few transition guards;
// a formal FSM would be more ceremony than the problem warrants.
import type { AgentDriver, AgentRunStartOptions } from "@/plugins/sdk/types";
import type { ContentBlock, InterruptResponse, RunEvent, RunId, StreamingResult } from "@/rpc";
import { useEffect, useRef } from "react";
import { errorDetail, errorType, RpcError } from "@/rpc";
import { LOCAL_MESSAGE_PREFIX } from "@/protocol/run/viewState";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/data/queryClient";
import { USAGE_SESSION_KEY } from "@/lib/data/useUsage";
import { useAgentStore } from "./agentStore";
import { startAgentSessionRecovery } from "./agentSessionRecovery";
import { createRunEventBatcher } from "./runEventBatcher";
import { useAgentSessionStore } from "./agentSessionStore";

// Owns the agent driver lifecycle for one session: build the driver, expose
// imperative send / stop / resume, and pump each run's RunEvent stream into
// useAgentStore (batched per frame). Changing sessionId rebuilds; the
// previous session's view state stays in the store (so switching back shows
// what was there).
export interface AgentSession {
  send: (input: ContentBlock[], options?: AgentRunStartOptions) => void;
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

    const eventBatcher = createRunEventBatcher({
      readEpoch: () => store().sessions[sessionId]?.viewEpoch ?? 0,
      apply: (batch) => store().applyEvents(sessionId, batch),
      onRunFinished: () => {
        void queryClient.invalidateQueries({ queryKey: [USAGE_SESSION_KEY, sessionId] });
      },
    });

    const pump = async (
      stream: StreamingResult<{ runId: RunId }, RunEvent>,
      signal: AbortSignal,
    ): Promise<void> => {
      const runId = stream.result.runId;
      currentRunId = runId;
      try {
        for await (const ev of stream.events) {
          if (cancelled || signal.aborted) break;
          eventBatcher.enqueue(ev.event, ev.runId);
        }
      } catch (err) {
        if (!cancelled && !signal.aborted)
          console.error("[agent] run stream failed:", sessionId, err);
      } finally {
        if (currentRunId === runId) currentRunId = null;
      }
    };

    if (!useAgentSessionStore.getState().draftSessionIds.has(sessionId)) {
      startAgentSessionRecovery({
        client: getContainer().client(),
        sessionId,
        isCancelled: () => cancelled,
        hasInteracted: () => interacted,
        applyEvents: (events) => store().applyEvents(sessionId, events),
        setAbortController: (ctrl) => {
          abort = ctrl;
        },
        pump,
      });
    }

    // Start re-entrancy latch. The steady-state guard (run.running) only flips
    // true once run.started is folded, so send/resume also need a synchronous
    // guard for the request→first-fold window.
    let starting = false;
    let beginSeq = 0;

    const begin = (
      run: (
        signal: AbortSignal,
      ) => Promise<StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>>,
      onResult?: (result: { runId: RunId; userItemId?: string }) => void,
      onStartError?: () => void,
    ): void => {
      starting = true;
      const beginId = ++beginSeq;
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
      let opening: Promise<StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>>;
      try {
        opening = withSpan(span, () => run(ctrl.signal));
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then((stream) => {
          if (cancelled || ctrl.signal.aborted || beginId !== beginSeq) return;
          // Runs before pump() iterates events (the response resolves ahead of
          // the buffered stream frames), so a userItemId relabel lands before
          // the streamed userMessage Item is folded.
          onResult?.(stream.result);
          span.setAttribute("lyra.run_id", stream.result.runId);
          return pump(stream, ctrl.signal);
        })
        .catch((err: unknown) => {
          if (cancelled || ctrl.signal.aborted || beginId !== beginSeq) return;
          failure = err;
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
        .finally(() => {
          // The run has settled (started+streamed, failed to start, or
          // interrupted) — release the re-entrancy latch only for the latest
          // begin; a superseded run's finally must not unlock its successor.
          if (beginId === beginSeq) starting = false;
          endSpan(span, failure);
        });
    };

    const send = (input: ContentBlock[], options: AgentRunStartOptions = {}): void => {
      if (starting) return;
      // Optimistically render the user's own bubble with a local id. The
      // runtime DOES stream the userMessage Item back (with its own server id),
      // a round-trip later — so when runs.start resolves we relabel this
      // placeholder to the returned `userItemId`, and the streamed Item then
      // dedupes by exact id (no duplicate, no content-text heuristic). The
      // bubble carries the SAME input the run does, so inlined images show
      // immediately and survive the relabel (which only swaps the id).
      const localId = `${LOCAL_MESSAGE_PREFIX}${++localSeq}`;
      store().applyEvents(sessionId, [
        {
          event: {
            type: "item.completed",
            item: {
              id: localId,
              runId: "",
              status: "completed",
              createdAt: new Date().toISOString(),
              type: "userMessage",
              content: input,
            },
          } as RunEvent["event"],
        },
      ]);
      begin(
        (signal) => driver.start(input, options, signal),
        (result) => {
          if (result.userItemId) store().relabelMessage(sessionId, localId, result.userItemId);
        },
        // The run never opened (channel-a error, e.g. session_busy because the
        // session has a run in flight / an open interrupt) — drop the optimistic
        // bubble so it doesn't strand below an error banner for a message that
        // wasn't accepted. The banner (set in begin's catch) carries the reason.
        () => store().dropMessage(sessionId, localId),
      );
      // First message graduates a draft session into the sidebar.
      useAgentSessionStore.getState().graduateDraft(sessionId);
    };

    const resume = (
      parentRunId: RunId,
      responses: InterruptResponse[],
      onSettled?: () => void,
      onStartError?: () => void,
    ): void => {
      if (starting) return;
      begin(
        (signal) => driver.resume(parentRunId, responses, signal),
        onSettled ? () => onSettled() : undefined,
        onStartError,
      );
    };

    const stop = (): void => {
      abort?.abort();
      // The abort closes the event channel, so the backend's run.finished
      // {canceled} never reaches the fold — settle the run locally or the view
      // stays stuck "running" (Stop button latched, next send blocked).
      store().cancelRun(sessionId);
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
    const pending = useAgentSessionStore.getState().takePendingMessage(sessionId);
    if (pending && pending.input.length > 0) send(pending.input, pending.runOptions);

    return () => {
      cancelled = true;
      eventBatcher.dispose();
      abort?.abort();
      store().setSend(sessionId, null);
      store().setStop(sessionId, null);
      store().setResume(sessionId, null);
    };
  }, [sessionId]);

  return {
    send: (input: ContentBlock[], options?: AgentRunStartOptions) =>
      useAgentStore.getState().sessions[sessionId]?.send?.(input, options),
    stop: () => useAgentStore.getState().sessions[sessionId]?.stop?.(),
  };
}
