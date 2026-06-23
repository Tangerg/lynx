// Agent lifecycle hook — bridges the plugin-registered AgentDriver (rpc-agent)
// to the Zustand agentStore. Owns the run state machine (idle → running → waiting
// → running → idle), pipes streaming RunEvents into the protocol fold, and
// provides the imperative send/stop/resume actions the UI binds to buttons.
//
// Kept as a plain hook (not a class / state machine library) because the run
// lifecycle is essentially one rAF-loop subscription + a few transition guards;
// a formal FSM would be more ceremony than the problem warrants.
import type { AgentDriver } from "@/plugins/sdk";
import type {
  ContentBlock,
  InterruptResponse,
  RunEvent,
  RunId,
  RunRef,
  StreamingResult,
} from "@/rpc";
import { useEffect, useRef } from "react";
import { asSessionId, errorDetail, errorType, RpcError } from "@/rpc";
import { LOCAL_MESSAGE_PREFIX } from "@/protocol/run/viewState";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { getContainer } from "@/main/container";
import { queryClient } from "@/lib/data/queryClient";
import { USAGE_SESSION_KEY } from "@/lib/data/useUsage";
import { useAgentStore, type FoldEvent } from "./agentStore";
import { useSessionStore } from "./sessionStore";

// Owns the agent driver lifecycle for one session: build the driver, expose
// imperative send / stop / resume, and pump each run's RunEvent stream into
// useAgentStore (batched per frame). Changing sessionId rebuilds; the
// previous session's view state stays in the store (so switching back shows
// what was there).
export interface AgentSession {
  send: (input: ContentBlock[]) => void;
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

    // Per-session rAF batcher. A run streams many item.delta events per
    // second; without batching each one triggers a store.set + React
    // commit. Coalescing into one flush per animation frame caps that at
    // ~1 commit per frame without changing perceived token latency.
    let queue: FoldEvent[] = [];
    let raf: number | null = null;
    // The queue is stamped with the view epoch it was filled under. An
    // external resetView (sessions.rollback re-hydration) bumps the epoch;
    // a flush scheduled BEFORE the reset must drop its batch — otherwise
    // the old run's tail events would append below the rebuilt history.
    const epochOf = () => store().sessions[sessionId]?.viewEpoch ?? 0;
    let queueEpoch = epochOf();
    const flush = () => {
      raf = null;
      if (cancelled || queue.length === 0) return;
      const batch = queue;
      queue = [];
      if (epochOf() !== queueEpoch) {
        queueEpoch = epochOf();
        return; // stale batch from before a view reset
      }
      store().applyEvents(sessionId, batch);
      // A finished run changed this session's durable metering — refetch its
      // cumulative usage chip, on the authoritative wire signal rather than an
      // active-session running-flag transition. Fires only for a session whose
      // stream is live here (the active one, or one re-subscribed on return); a
      // run that finishes while its session is purely backgrounded has no live
      // subscription, so its chip refreshes on the next visit, not instantly.
      // Ordering note: the server
      // persists the run blob AFTER it appends run.finished to the hub (pump
      // reorder), but that persist is a sub-ms local DB write that completes long
      // before this invalidate's usage.session HTTP refetch round-trips back to
      // the server — so the refetch reads the finished run. If it ever lost that
      // race, the next refetch (staleTime / a later run) heals it.
      if (batch.some((e) => e.event.type === "run.finished")) {
        void queryClient.invalidateQueries({ queryKey: [USAGE_SESSION_KEY, sessionId] });
      }
    };
    const enqueue = (event: RunEvent["event"], runId?: string) => {
      const epoch = epochOf();
      if (epoch !== queueEpoch) {
        queue = []; // events queued before the reset are stale too
        queueEpoch = epoch;
      }
      queue.push({ event, runId });
      if (raf === null) raf = requestAnimationFrame(flush);
    };

    const pump = async (stream: StreamingResult<{ runId: RunId }, RunEvent>): Promise<void> => {
      currentRunId = stream.result.runId;
      try {
        for await (const ev of stream.events) {
          if (cancelled) break;
          enqueue(ev.event, ev.runId);
        }
      } catch (err) {
        if (!cancelled) console.error("[agent] run stream failed:", sessionId, err);
      }
    };

    // Replay the session's durable Items as `item.completed` events through
    // the SAME fold the live stream uses, so past turns render identically.
    // Idempotent (the fold upserts by item id) — safe to re-apply after a
    // reattach race below.
    const applyHistory = async (): Promise<void> => {
      const resp = await getContainer()
        .client()
        .items.list({ sessionId: asSessionId(sessionId) });
      if (cancelled || interacted || resp.data.length === 0) return;
      store().applyEvents(
        sessionId,
        resp.data.map((item): FoldEvent => ({ event: { type: "item.completed", item } })),
      );
    };

    // Reattach to a run that survived a reload (API.md §10.1): runs.subscribe
    // streams events from now on, so the view's "running" state is seeded from
    // the RunRef — the original run.started fired before we were listening.
    const attach = async (run: RunRef): Promise<void> => {
      // Register the controller BEFORE the call so a user send() supersedes
      // the reattached stream exactly like it supersedes a started one
      // (begin() aborts `abort`).
      const ctrl = new AbortController();
      abort = ctrl;
      let stream: StreamingResult<{ runId: RunId }, RunEvent>;
      try {
        stream = await getContainer().client().runs.subscribe(run.id, ctrl.signal);
      } catch (err) {
        if (cancelled || ctrl.signal.aborted) return;
        // Most likely the run finished between runs.list and subscribe — its
        // tail items missed the first items.list, so re-apply history.
        console.warn("[agent] run reattach failed:", sessionId, err);
        void applyHistory().catch(() => undefined);
        return;
      }
      if (cancelled || ctrl.signal.aborted) return;
      store().applyEvents(sessionId, [{ event: { type: "run.started", run } }]);
      await pump(stream);
    };

    // Hydrate + recover an existing (non-draft) session (API.md §10.2):
    // durable history, then unresolved HITL interrupts (their cards must come
    // back after a reload), then reattach a still-running run. Drafts have no
    // history (just created) — their queued first message is flushed below.
    // Each step re-checks `interacted`: once the user sends, the live run owns
    // the slice and late backfill would interleave below it (see above).
    if (!useSessionStore.getState().draftSessionIds.has(sessionId)) {
      void (async () => {
        const client = getContainer().client();
        const sid = asSessionId(sessionId);
        await applyHistory();
        if (cancelled || interacted) return;
        // Durable HITL recovery: synthesize the exact wire sequence the live
        // path produces (run.started + run.finished{interrupt}) per envelope,
        // so the cards rebuild through the same idempotent fold.
        const open = await client.runs.listOpenInterrupts(sid);
        if (cancelled || interacted) return;
        for (const oi of open.data) {
          store().applyEvents(sessionId, [
            {
              event: {
                type: "run.started",
                run: { id: oi.parentRunId, sessionId: oi.sessionId, createdAt: oi.createdAt },
              },
            },
            {
              event: {
                type: "run.finished",
                outcome: { type: "interrupt", interrupts: oi.interrupts },
              },
            },
          ]);
        }
        // At most one root run can be in flight (session_busy), so reattach
        // the first non-subagent run if any.
        const running = await client.runs.list(sid);
        if (cancelled || interacted) return;
        const root = running.data.find((r) => !r.spawnedByItemId);
        if (root) await attach(root);
      })().catch((err: unknown) => {
        if (!cancelled) console.error("[agent] session recovery failed:", sessionId, err);
      });
    }

    // Send re-entrancy latch. The steady-state guard (useChatSend's run.running)
    // only flips true once run.started is FOLDED — a frame after runs.start
    // resolves (run.started streams in and folds via the rAF batcher). A second
    // Enter in that window would fire a second runs.start; the backend now
    // rejects it with session_busy (one run per session, API.md §7.3), but the
    // latch still matters — it avoids the wasted round-trip + the
    // optimistic-bubble-then-rollback churn for that in-flight-but-not-yet-folded
    // window. So the latch spans the WHOLE window send()→run-settled: set in
    // send(), cleared in begin()'s finally (run started+streamed, errored, or
    // interrupted). Earlier
    // (onResult) was too soon and reopened the gap. Unused by resume.
    let starting = false;

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
        .finally(() => {
          // The run has settled (started+streamed, failed to start, or
          // interrupted) — release the send re-entrancy latch. No-op for resume.
          starting = false;
          endSpan(span, failure);
        });
    };

    const send = (input: ContentBlock[]): void => {
      if (starting) return;
      starting = true;
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
        (signal) => driver.start(input, signal),
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
    const pending = useSessionStore.getState().takePendingMessage(sessionId);
    if (pending && pending.length > 0) send(pending);

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
    send: (input: ContentBlock[]) => useAgentStore.getState().sessions[sessionId]?.send?.(input),
    stop: () => useAgentStore.getState().sessions[sessionId]?.stop?.(),
  };
}
