import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { t } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { LOCAL_STEER_PREFIX } from "@/plugins/builtin/agent/domain/messageIdentity";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { getActiveSessionId } from "../session/activeSession";
import { type CreateSessionOptions, useCreateSession } from "../session/createSession";

type SendToAgent = (input: AgentInput, options?: AgentRunStartOptions) => void;
type CreateSession = (opts?: CreateSessionOptions) => Promise<string | null>;

/**
 * The single send entry point for the composer — both the textarea Enter
 * path and the Send button route through here so they can't diverge.
 *
 *   - active session present → send into it.
 *   - no active session (welcome screen) → spin up a draft session and queue
 *     the message (useCreateSession); the chat remounts on the new id and
 *     flushes it.
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
export function useChatSend(): (input: AgentInput) => void {
  const createSession = useCreateSession();
  const send = agentSessionView().useAction("send");
  const running = agentSessionView().useCurrentRootAttention().status === "running";
  const runId = agentSessionView().useCurrentRootRunId();
  const segmentId = agentSessionView().useCurrentRootSegmentId();
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId();
      const runOptions = resolveAgentRunStartOptions();
      // A steer needs the segment as well as the run: without it there is nothing to
      // address, and a fresh turn is the honest fallback.
      if (running && sessionId && runId && segmentId) {
        if (steerRunningTurn({ sessionId, runId, segmentId, input, send, runOptions })) {
          return;
        }
      }
      sendFreshTurn({ sessionId, send, createSession, input, runOptions });
    },
    [send, running, runId, segmentId, createSession],
  );
}

export function useCanSendToAgent(): boolean {
  return Boolean(agentSessionView().useAction("send"));
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
  const localId = mintSteerBubble(sessionId, input);
  const runtime = agentRuntime();
  void runtime.steerRun(runId, segmentId, input).catch((err: unknown) => {
    agentSessionView().dropMessage(sessionId, localId);
    // The run this steer addressed is no longer executing: it finished, parked, or
    // moved to another segment while the person was typing. Sending the input as a
    // fresh turn is what they meant, and it is the runtime — not a guess here —
    // that says which of those happened.
    if (runtime.isRunGone(err)) {
      send?.(input, runOptions);
      return;
    }
    // Any other failure means the steer may or may not have reached the loop, so
    // the optimistic bubble goes either way: if it did land, the runtime streams
    // the real userMessage Item back and the fold shows it. Leaving the bubble up
    // was the one outcome that lies — the message sits there looking sent, with
    // no reply and nothing said about why.
    console.error("[session] steer failed:", err);
    notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
  });
  return true;
}

interface SendFreshTurnInput {
  sessionId: string;
  send: SendToAgent | null;
  createSession: CreateSession;
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

function sendFreshTurn({
  sessionId,
  send,
  createSession,
  input,
  runOptions,
}: SendFreshTurnInput): void {
  if (sessionId && send) {
    send(input, runOptions);
    return;
  }
  void createSession({ firstInput: input, firstRunOptions: runOptions });
}

function mintSteerBubble(sessionId: string, input: AgentInput): string {
  const id = `${LOCAL_STEER_PREFIX}${++steerSeq}`;
  agentSessionView().appendLocalUserMessage(sessionId, id, input);
  return id;
}
