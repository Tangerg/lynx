import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { t } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { OPTIMISTIC_STEER_MESSAGE_PREFIX } from "../view/optimisticMessageIdentity";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { getActiveSessionId, useActiveSessionId } from "../session/activeSession";
import { selectCurrentRootRun } from "../view/runTree";
import { agentCommandOwner } from "../agentCommandOwner";
import { useCurrentRootMaterial } from "../run/runReadModel";

type SendToAgent = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
/**
 * The single send entry point for the composer — both the textarea Enter
 * path and the Send button route through here so they can't diverge.
 *
 *   - active session present → send into it.
 *   - no active session (welcome screen) → reject submission without clearing
 *     the draft; selecting a project creates and mounts the Session first.
 *   - a run is already streaming → STEER the running turn (runs.steer): the
 *     message injects into the active loop and the model reads it on its next
 *     tool round (true mid-run steer). The steered message is rendered
 *     OPTIMISTICALLY (a local-* user bubble) the moment it's sent, so the user
 *     sees their input land immediately instead of waiting for the next round
 *     boundary; the runtime streams the real userMessage Item back when it
 *     drains the steer, and the fold reconciles the placeholder by content. If
 *     the run finished between typing and sending (run_not_found), the steer is
 *     no longer deliverable — roll the optimistic bubble back and fall back to a
 *     fresh turn so the message is never lost (and never duplicated).
 */
export function useChatSend(): (input: AgentInput) => boolean {
  const send = agentSessionView().useAction("send");
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId();
      const runOptions = resolveAgentRunStartOptions();
      // Admission is decided at event time, not from the render that created
      // this callback. A Run can park for HITL between the last paint and an
      // Enter keydown; steering a captured `running` identity would clear the
      // composer before the Runtime rejects it as no longer addressable.
      const root = selectCurrentRootRun(agentSessionView().getCurrentView());
      // A steer needs the segment as well as the run: without it there is nothing to
      // address, and a fresh turn is the honest fallback.
      if (root?.status === "running" && sessionId && root.activeSegmentId) {
        if (
          steerRunningTurn({
            sessionId,
            runId: root.id,
            segmentId: root.activeSegmentId,
            input,
            send,
            runOptions,
          })
        ) {
          return true;
        }
      }
      return sendFreshTurn({ sessionId, send, input, runOptions });
    },
    [send],
  );
}

export function useCanSendToAgent(): boolean {
  const sessionId = useActiveSessionId();
  const send = agentSessionView().useAction("send");
  const root = useCurrentRootMaterial();
  return canAcceptChatInput(sessionId, Boolean(send), root.status);
}

export function canAcceptChatInput(
  sessionId: string,
  mountedSendAvailable: boolean,
  rootStatus: "idle" | "running" | "waiting" | "finished",
): boolean {
  // Only the mounted Session lifecycle may accept input. The projectless welcome
  // screen deliberately keeps the draft but cannot send it; a parked root must
  // be resumed through its interrupt rather than opened as a competing turn.
  return Boolean(sessionId) && mountedSendAvailable && rootStatus !== "waiting";
}

// Optimistic steer bubble: render the user's steered message immediately under a
// local-* id (a distinct "steer-" suffix so it can't collide with send()'s own
// local-N counter). The fold reconciles it against the streamed userMessage Item
// by content match (appendUserMessage) once the runtime drains the steer — no
// explicit relabel, since runs.steer returns no item id.
let steerSeq = 0;

interface SteerRunningTurnInput {
  sessionId: string;
  runId: string;
  segmentId: string;
  input: AgentInput;
  send: SendToAgent | null;
  runOptions: AgentRunStartOptions;
}

function steerRunningTurn({
  sessionId,
  runId,
  segmentId,
  input,
  send,
  runOptions,
}: SteerRunningTurnInput): boolean {
  if (input.parts.length === 0) return false;
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const view = agentSessionView();
  const localId = mintSteerBubble(view, sessionId, input);
  const effect = owner.trackEffect(() => view.dropMessage(sessionId, localId));
  void owner.settle(runtime.steerRun(runId, segmentId, input)).then(
    () => {
      if (owner.isCurrent()) effect.settle();
    },
    (err: unknown) => {
      if (!owner.isCurrent()) return;
      effect.rollback();
      // The run this steer addressed is no longer executing: it finished, parked, or
      // moved to another segment while the person was typing. Sending the input as a
      // fresh turn is what they meant, and it is the runtime — not a guess here —
      // that says which of those happened.
      if (runtime.isRunGone(err)) {
        if (send?.(input, runOptions) !== true) {
          // The Run parked rather than finished. The input remains in composer
          // history, but it was not accepted as a new turn; make that explicit
          // instead of silently discarding an optimistic steer.
          notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
        }
        return;
      }
      // Any other failure means the steer may or may not have reached the loop, so
      // the optimistic bubble goes either way: if it did land, the runtime streams
      // the real userMessage Item back and the fold shows it. Leaving the bubble up
      // was the one outcome that lies — the message sits there looking sent, with
      // no reply and nothing said about why.
      console.error("[session] steer failed:", err);
      notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
    },
  );
  return true;
}

interface SendFreshTurnInput {
  sessionId: string;
  send: SendToAgent | null;
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

function sendFreshTurn({ sessionId, send, input, runOptions }: SendFreshTurnInput): boolean {
  if (sessionId && send) {
    return send(input, runOptions);
  }
  return false;
}

function mintSteerBubble(
  view: ReturnType<typeof agentSessionView>,
  sessionId: string,
  input: AgentInput,
): string {
  const id = `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${++steerSeq}`;
  view.appendLocalUserMessage(sessionId, id, input);
  return id;
}
