import type { AgentDriver, AgentRunStartOptions } from "@/plugins/sdk/types";
import { asItemId, asRunId, type InterruptResponse } from "@/rpc";
import { useEffect, useRef } from "react";
import type { AgentInput } from "@/plugins/builtin/agent/domain/input";
import type { AgentSession } from "../application/ports/defaultSession";
import type { InterruptResumeInput } from "../application/ports/viewState";
import { agentInputToContentBlocks } from "@/plugins/builtin/agent/adapters/wireInput";
import { getContainer } from "@/main/container";
import { useAgentStore } from "./agentStore";
import { createAgentRunPump } from "./agentRunPump";
import { startAgentSessionRecovery } from "./agentSessionRecovery";
import { useAgentSessionStore } from "./agentSessionStore";
import { createOptimisticUserMessage } from "./optimisticUserMessage";
import { createRunOpeningController } from "./runOpeningController";

export function useAgentSession(makeDriver: () => AgentDriver, sessionId: string): AgentSession {
  const factoryRef = useRef(makeDriver);
  factoryRef.current = makeDriver;

  useEffect(() => {
    // Welcome screen (no active session) mounts the kernel chat with an empty
    // id — there is nothing to drive: no slice to seed, and items.list("")
    // would just be a guaranteed-failing RPC on every mount.
    if (!sessionId) return;
    const driver = factoryRef.current();
    const client = () => getContainer().client();
    const store = () => useAgentStore.getState();

    // Reset this session's slice before streaming so we don't carry state
    // from a previous mount of the same session id.
    store().resetSession(sessionId);

    let abort: AbortController | null = null;
    let cancelled = false;
    // Set once a live send/resume writes to this slice. History hydration
    // (items.list) is async; if the user sends before it resolves, applying
    // the backfill afterwards would append the old turns *below* the new
    // message (the fold is arrival-ordered, not timestamp-sorted) and bleed
    // history agentMessages into the open live turn. Skip the late backfill
    // in that race — it'll hydrate cleanly on the next open.
    let interacted = false;

    const runPump = createAgentRunPump({
      sessionId,
      isCancelled: () => cancelled,
      readEpoch: () => store().sessions[sessionId]?.viewEpoch ?? 0,
      applyEvents: (events) => store().applyEvents(sessionId, events),
    });

    if (!useAgentSessionStore.getState().draftSessionIds.has(sessionId)) {
      startAgentSessionRecovery({
        client: client(),
        sessionId,
        isCancelled: () => cancelled,
        hasInteracted: () => interacted,
        applyEvents: (events) => store().applyEvents(sessionId, events),
        setAbortController: (ctrl) => {
          abort = ctrl;
        },
        pump: runPump.pump,
      });
    }

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
      setStartError: (error) => store().setError(sessionId, error),
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
      store().applyEvents(sessionId, [{ event: optimistic.event }]);
      runOpening.begin(
        (signal) => driver.start(wireInput, options, signal),
        (result) => {
          if (result.userItemId)
            store().relabelMessage(sessionId, optimistic.localId, result.userItemId);
        },
        // The run never opened (channel-a error, e.g. session_busy because the
        // session has a run in flight / an open interrupt) — drop the optimistic
        // bubble so it doesn't strand below an error banner for a message that
        // wasn't accepted. The banner (set in begin's catch) carries the reason.
        () => store().dropMessage(sessionId, optimistic.localId),
      );
      // First message graduates a draft session into the sidebar.
      useAgentSessionStore.getState().graduateDraft(sessionId);
    };

    const resume = (
      runId: string,
      responses: InterruptResumeInput[],
      onSettled?: () => void,
      onStartError?: () => void,
    ): void => {
      if (runOpening.isStarting()) return;
      const wireResponses: InterruptResponse[] = responses.map((response) => ({
        itemId: asItemId(response.itemId),
        response: response.response,
      }));
      runOpening.begin(
        (signal) => driver.resume(asRunId(runId), wireResponses, signal),
        onSettled ? () => onSettled() : undefined,
        onStartError,
      );
    };

    const stop = (): void => {
      abort?.abort();
      // The abort closes the event channel, so the backend's segment.finished
      // {canceled} never reaches the fold — settle the run locally or the view
      // stays stuck "running" (Stop button latched, next send blocked).
      store().cancelRun(sessionId);
      runPump.cancelCurrentRun((runId) => client().runs.cancel(runId));
    };

    store().setSend(sessionId, send);
    store().setStop(sessionId, stop);
    store().setResume(sessionId, resume);

    // A message typed on the welcome screen (no active session) was queued
    // by useCreateSession against this freshly-created draft — flush it now
    // that the driver for this id is live. Opening a session otherwise does
    // NOT auto-run; replaying history is a separate concern (items.list).
    const pending = useAgentSessionStore.getState().takePendingMessage(sessionId);
    if (pending && pending.input.parts.length > 0) send(pending.input, pending.runOptions);

    return () => {
      cancelled = true;
      runPump.dispose();
      abort?.abort();
      store().setSend(sessionId, null);
      store().setStop(sessionId, null);
      store().setResume(sessionId, null);
    };
  }, [sessionId]);

  return {
    send: (input: AgentInput, options?: AgentRunStartOptions) =>
      useAgentStore.getState().sessions[sessionId]?.send?.(input, options),
    stop: () => useAgentStore.getState().sessions[sessionId]?.stop?.(),
  };
}
