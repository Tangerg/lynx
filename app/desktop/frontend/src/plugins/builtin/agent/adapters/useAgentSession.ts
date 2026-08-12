import type { AgentDriver, AgentRunStartOptions } from "@/plugins/sdk/types";
import { asItemId, asRunId, type InterruptResponse } from "@/rpc";
import { useEffect, useEffectEvent } from "react";
import { queryClient } from "@/lib/queryClient";
import type { AgentInput } from "@/plugins/builtin/agent/domain/input";
import type { AgentSession } from "../application/ports/defaultSession";
import type { InterruptResumeInput } from "../application/ports/sessionView";
import { selectCurrentRootRun } from "../application/view/runTree";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import { agentInputToContentBlocks } from "@/plugins/builtin/agent/adapters/wireInput";
import { getContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { createAgentRunPump } from "./agentRunPump";
import { createRunStreamReattach } from "./runStreamReattach";
import { refreshAgentSessionProjection } from "../application/session/refreshSessionProjection";
import { startAgentSessionRecovery } from "./agentSessionRecovery";
import { useAgentSessionStore } from "./agentSessionStore";
import { createOptimisticUserMessage } from "./optimisticUserMessage";
import { createRunOpeningController } from "./runOpeningController";
import { agentProblemFromRpcError } from "./rpcProblem";
import { createSessionProjectionSynchronization } from "../application/session/sessionProjectionSynchronization";

export function useAgentSession(makeDriver: () => AgentDriver, sessionId: string): AgentSession {
  // Driver construction belongs to the session effect, but the adapter factory
  // is supplied during render and may change independently. An Effect Event
  // gives the effect the latest factory without turning factory identity into a
  // second, accidental lifecycle key.
  const createDriver = useEffectEvent(makeDriver);

  useEffect(() => {
    // Welcome screen (no active session) mounts the kernel chat with an empty
    // id — there is nothing to drive: no slice to seed, and items.list("")
    // would just be a guaranteed-failing RPC on every mount.
    if (!sessionId) return;
    const driver = createDriver();
    const client = () => getContainer().client();
    const store = () => useAgentStore.getState();

    // Preserve an already-materialized view while the authoritative refresh
    // is in flight; a first mount creates the empty slice.
    store().ensureSession(sessionId);

    let abort: AbortController | null = null;
    let cancelled = false;
    const cancelRequests = new Set<string>();
    // Set as soon as this driver accepts a local command. The initial durable
    // read must not commit after that command, even before its first stream
    // event has advanced the store revision.
    let interacted = false;
    let projectionSynchronization: ReturnType<
      typeof createSessionProjectionSynchronization
    > | null = null;

    const runPump = createAgentRunPump({
      sessionId,
      isCancelled: () => cancelled,
      readEpoch: () => store().sessions[sessionId]?.viewEpoch ?? 0,
      applyEvents: (events) => store().applyRunEvents(sessionId, events),
      readRunSnapshot: (runId, signal) => client().runs.get(runId, signal),
      applyRunSnapshot: (run) => store().applyRunSnapshot(sessionId, run),
      // A run keeps executing when its stream drops. Reattaching is what makes that a
      // gap instead of a transcript that stops moving until the next reload.
      reattach: createRunStreamReattach({
        sessionId,
        client,
        isCancelled: () => cancelled,
        recoverProjection: () =>
          refreshAgentSessionProjection(sessionId, {
            canCommit: () => !cancelled,
          }).then(() => undefined),
      }),
      onIdle: () => projectionSynchronization?.liveStreamSettled(),
    });

    const recoverExistingSession = !useAgentSessionStore.getState().draftSessionIds.has(sessionId);
    let guardInitialInteraction = recoverExistingSession;
    projectionSynchronization = createSessionProjectionSynchronization({
      isLiveStreamActive: runPump.isActive,
      synchronize: () => {
        const guardInteraction = guardInitialInteraction;
        guardInitialInteraction = false;
        return startAgentSessionRecovery({
          client: client(),
          sessionId,
          isCancelled: () => cancelled,
          hasInteracted: () => guardInteraction && interacted,
          isFollowing: runPump.isFollowing,
          setAbortController: (controller) => {
            abort?.abort();
            abort = controller;
          },
          pump: runPump.pump,
        });
      },
    });

    if (recoverExistingSession) projectionSynchronization.request();

    const runOpening = createRunOpeningController({
      sessionId,
      isCancelled: () => cancelled,
      markInteracted: () => {
        interacted = true;
      },
      abortCurrent: () => abort?.abort(),
      setAbortController: (ctrl) => {
        abort = ctrl;
      },
      pump: runPump.pump,
      setStartError: (error) => store().setCommandError(sessionId, error),
    });

    const send = (input: AgentInput, options: AgentRunStartOptions = {}): void => {
      if (runOpening.isStarting()) return;
      const wireInput = agentInputToContentBlocks(input);
      // Optimistically render the user's own bubble with a local id. The
      // runtime DOES stream the userMessage Item back (with its own server id),
      // a round-trip later — so when runs.start resolves we relabel this
      // placeholder to the returned `userItemId`, and the streamed Item then
      // dedupes by exact id (no duplicate, no content-text heuristic). The
      // bubble carries the SAME input the run does, so inlined images show
      // immediately and survive the relabel (which only swaps the id).
      const optimistic = createOptimisticUserMessage(wireInput);
      store().appendLocalMessage(sessionId, optimistic.message);
      runOpening.begin(
        (signal) => driver.start(wireInput, options, signal),
        (result) => {
          store().reconcileMessageIdentity(sessionId, optimistic.localId, result.userItemId);
          // The run was accepted, so this session now holds a conversation: it
          // graduates out of draft and into the session list. This used to fire
          // the moment the attempt started, which promoted a session whose only
          // message was then dropped by the failure path below — an empty row in
          // the sidebar for a message the runtime never took.
          useAgentSessionStore.getState().graduateDraft(sessionId);
        },
        // The run never opened (channel-a error, e.g. session_busy because the
        // session has a run in flight / an open interrupt) — drop the optimistic
        // bubble so it doesn't strand below an error banner for a message that
        // wasn't accepted. The banner (set in begin's catch) carries the reason.
        () => store().dropMessage(sessionId, optimistic.localId),
      );
    };

    const resume = (
      runId: string,
      responses: InterruptResumeInput[],
      onSettled?: () => void,
      onStartError?: () => void,
    ): boolean => {
      if (runOpening.isStarting()) return false;
      const wireResponses: InterruptResponse[] = responses.map((response) => ({
        itemId: asItemId(response.itemId),
        response: response.response,
      }));
      runOpening.begin(
        (signal) => driver.resume(asRunId(runId), wireResponses, signal),
        onSettled ? () => onSettled() : undefined,
        onStartError,
      );
      return true;
    };

    const cancelRun = (runId: string): void => {
      const run = store().sessions[sessionId]?.view.runsById[runId];
      if (!run || run.status === "finished" || cancelRequests.has(runId)) return;
      interacted = true;
      cancelRequests.add(runId);
      void client()
        .runs.cancel(asRunId(runId))
        .then((response) => {
          if (cancelled) return;
          // A cancel command commits a concrete RunRef (and, for a child,
          // the root's exact post-cancel state). Fold only those facts; the
          // following snapshot fills in every query-owned descendant.
          store().applyCancelResponse(sessionId, response);
          // Root cancellation ends the stream; child cancellation advances
          // the parent onto a new segment. In both cases the currently attached
          // segment has lost ownership, so release it before reconciliation.
          abort?.abort();
          projectionSynchronization?.request();
          void queryClient.invalidateQueries({
            queryKey: [AGENT_SESSION_USAGE_KEY, sessionId],
          });
        })
        .catch((error: unknown) => {
          if (cancelled) return;
          console.error("[agent] run cancellation failed:", sessionId, runId, error);
          const problem = agentProblemFromRpcError(error);
          if (problem) store().setCommandError(sessionId, problem);
        })
        .finally(() => {
          cancelRequests.delete(runId);
        });
    };

    const stop = (): boolean => {
      const view = store().sessions[sessionId]?.view;
      const root = view ? selectCurrentRootRun(view) : null;
      if (root?.status !== "running") return false;
      cancelRun(root.id);
      return true;
    };

    store().setSend(sessionId, send);
    store().setStop(sessionId, stop);
    store().setResume(sessionId, resume);
    store().setSynchronize(sessionId, () => projectionSynchronization?.request());
    store().setCancelRun(sessionId, cancelRun);

    // A message typed on the welcome screen (no active session) was queued
    // by useCreateSession against this freshly-created draft — flush it now
    // that the driver for this id is live. Opening a session otherwise does
    // NOT auto-run; rebuilding the durable projection is a separate concern.
    const pending = useAgentSessionStore.getState().takePendingMessage(sessionId);
    if (pending && pending.input.parts.length > 0) send(pending.input, pending.runOptions);

    return () => {
      cancelled = true;
      projectionSynchronization?.dispose();
      runPump.dispose();
      abort?.abort();
      store().setSend(sessionId, null);
      store().setStop(sessionId, null);
      store().setResume(sessionId, null);
      store().setSynchronize(sessionId, null);
      store().setCancelRun(sessionId, null);
    };
  }, [sessionId]);

  return {
    send: (input: AgentInput, options?: AgentRunStartOptions) =>
      useAgentStore.getState().sessions[sessionId]?.send?.(input, options),
    stop: () => {
      useAgentStore.getState().sessions[sessionId]?.stop?.();
    },
  };
}
