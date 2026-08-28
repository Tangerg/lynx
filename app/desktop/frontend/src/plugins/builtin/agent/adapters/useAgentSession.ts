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
import { agentProblemFromRpcFailure } from "./rpcProblem";
import { createSessionProjectionSynchronization } from "../application/session/sessionProjectionSynchronization";
import { createRunCancellationController } from "./runCancellationController";
import { revalidateRunTermination } from "../application/run/revalidateRunTermination";

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

    // Explicit selection already holds the Session open, but a URL deep-link
    // or browser history move mounts this lifecycle without passing through
    // that action. Establish the same invariant before creating its material
    // view: open-set subscribers use membership to prune Agent/composer state,
    // so an active-but-unheld Session could otherwise be deleted underneath
    // this still-mounted driver by an unrelated reconciliation.
    const sessionMemory = useAgentSessionStore.getState();
    sessionMemory.holdOpen(sessionId);
    sessionMemory.rememberSession(sessionId);
    // Preserve an already-materialized view while the authoritative refresh
    // is in flight; a first mount creates the empty slice.
    store().ensureSession(sessionId);

    let abort: AbortController | null = null;
    let cancelled = false;
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
        recoverProjection: (signal) =>
          refreshAgentSessionProjection(sessionId, {
            canCommit: () => !cancelled && !signal.aborted,
            signal,
          }).then(() => undefined),
      }),
      onIdle: () => projectionSynchronization?.liveStreamSettled(),
    });

    // Persisted draft ownership survives reload, but the proof that this process
    // just created an empty Session does not. Only a same-process fresh draft may
    // skip its initial durable read; a cold draft must verify whether another
    // client added history while this one was away.
    const recoverExistingSession = !useAgentSessionStore
      .getState()
      .freshDraftSessionIds.has(sessionId);
    let guardInitialInteraction = recoverExistingSession;
    projectionSynchronization = createSessionProjectionSynchronization({
      isLiveStreamActive: runPump.isActive,
      synchronize: (signal) => {
        const guardInteraction = guardInitialInteraction;
        guardInitialInteraction = false;
        return startAgentSessionRecovery({
          client: client(),
          sessionId,
          signal,
          isCancelled: () => cancelled,
          hasInteracted: () => guardInteraction && interacted,
          isFollowing: runPump.isFollowing,
          setAbortController: (controller) => {
            abort?.abort();
            abort = controller;
          },
          pump: runPump.pump,
        }).then((authoritativeView) => {
          const state = useAgentSessionStore.getState();
          if (state.draftSessionIds.has(sessionId) && authoritativeView?.messages.length) {
            state.graduateDraft(sessionId);
          }
          return authoritativeView !== null;
        });
      },
    });

    if (recoverExistingSession) void projectionSynchronization.request();

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

    const send = (input: AgentInput, options: AgentRunStartOptions = {}): boolean => {
      if (runOpening.isStarting()) return false;
      const currentRoot = store().sessions[sessionId]?.view;
      if (currentRoot && selectCurrentRootRun(currentRoot)?.status === "waiting") return false;
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
          // graduates out of draft and into the session list only at acceptance.
          useAgentSessionStore.getState().graduateDraft(sessionId);
        },
        // The run never opened (channel-a error, e.g. session_busy because the
        // session has a run in flight / an open interrupt) — drop the optimistic
        // bubble so it doesn't strand below an error banner for a message that
        // wasn't accepted. The banner (set in begin's catch) carries the reason.
        () => store().dropMessage(sessionId, optimistic.localId),
      );
      return true;
    };

    const resume = (
      runId: string,
      responses: InterruptResumeInput[],
      onSettled?: () => void,
      onStartError?: () => boolean | void,
    ): boolean => {
      if (runOpening.isStarting()) return false;
      const wireResponses: InterruptResponse[] = responses.map((response) => ({
        itemId: asItemId(response.itemId),
        response: response.response,
      }));
      runOpening.begin(
        (signal) => driver.resume(asRunId(runId), { responses: wireResponses }, signal),
        onSettled ? () => onSettled() : undefined,
        onStartError,
      );
      return true;
    };

    const createCancellationGeneration = () => {
      // A cancellation command and every fact it derives belong to the client
      // which first admits work into this generation. Capture lazily so an idle
      // Session does not require a client, but never resolve `client()` again
      // after an RPC has crossed the generation boundary.
      let generationClient: ReturnType<typeof client> | null = null;
      const admittedClient = () => (generationClient ??= client());
      return createRunCancellationController({
        markInteracted: () => {
          interacted = true;
        },
        readTarget: (runId) => {
          const entry = store().sessions[sessionId];
          const run = entry?.view.runsById[runId];
          return entry && run
            ? {
                terminal: run.status === "finished",
                viewEpoch: entry.viewEpoch,
                viewRevision: entry.viewRevision,
              }
            : null;
        },
        execute: (runId) => admittedClient().runs.cancel(asRunId(runId)),
        commitIfCurrent: (response, target) =>
          store().commitCancelResponse(
            sessionId,
            { viewEpoch: target.viewEpoch, viewRevision: target.viewRevision },
            response,
          ),
        revalidateTerminal: (runId) => revalidateRunTermination(sessionId, runId),
        onSettled: () => {
          // Root cancellation ends the stream; child cancellation advances the
          // parent onto a new segment. In both cases the currently attached
          // segment has lost ownership, so release it before reconciliation.
          abort?.abort();
          projectionSynchronization?.request();
          void queryClient.invalidateQueries({
            queryKey: [AGENT_SESSION_USAGE_KEY, sessionId],
          });
        },
        onFailure: (runId, error) => {
          console.error("[agent] run cancellation failed:", sessionId, runId, error);
          const problem = agentProblemFromRpcFailure(error);
          if (problem) store().setCommandError(sessionId, problem);
        },
      });
    };
    let runCancellation = createCancellationGeneration();

    const cancelRun = (runId: string): void => runCancellation.cancel(runId);

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
    store().setSynchronize(sessionId, (ownership) => {
      if (
        ownership === "replace-live" ||
        ownership === "retire-live" ||
        ownership === "replace-server"
      ) {
        runOpening.retire();
        runCancellation.retire();
        if (ownership === "retire-live") {
          projectionSynchronization?.retire();
          return Promise.resolve(false);
        }
        runCancellation = createCancellationGeneration();
        return projectionSynchronization?.replace() ?? Promise.resolve(false);
      }
      return projectionSynchronization?.request() ?? Promise.resolve(false);
    });
    store().setCancelRun(sessionId, cancelRun);

    return () => {
      cancelled = true;
      runOpening.retire();
      runCancellation.retire();
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
      useAgentStore.getState().sessions[sessionId]?.send?.(input, options) ?? false,
    stop: () => {
      useAgentStore.getState().sessions[sessionId]?.stop?.();
    },
  };
}
