import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asRunId, isErrorType } from "@/rpc";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { agentInputText } from "../../domain/input";
import { LOCAL_STEER_PREFIX } from "@/plugins/builtin/agent/public/viewState";
import { agentViewState } from "../ports/viewState";
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
  const send = agentViewState().useAction("send");
  const running = agentViewState().useRunning();
  const runId = agentViewState().useRunId();
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId();
      const runOptions = resolveAgentRunStartOptions();
      if (running && sessionId && runId) {
        if (steerRunningTurn({ sessionId, runId, input, send, runOptions })) {
          return;
        }
      }
      sendFreshTurn({ sessionId, send, createSession, input, runOptions });
    },
    [send, running, runId, createSession],
  );
}

export function useCanSendToAgent(): boolean {
  return Boolean(agentViewState().useAction("send"));
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
  input: AgentInput;
  send: SendToAgent | null;
  runOptions: AgentRunStartOptions;
}

function steerRunningTurn({
  sessionId,
  runId,
  input,
  send,
  runOptions,
}: SteerRunningTurnInput): boolean {
  const text = steerText(input);
  if (!text) return false;
  const localId = mintSteerBubble(sessionId, input);
  void getContainer()
    .client()
    .runs.steer(asRunId(runId), text)
    .catch((err) => {
      if (isErrorType(err, "run_not_found")) {
        agentViewState().dropMessage(sessionId, localId);
        send?.(input, runOptions);
      }
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
  agentViewState().appendLocalUserMessage(sessionId, id, input);
  return id;
}

// steerText flattens the composer's text blocks into one message — steering
// carries text only (images can't ride a steer; see useChatSend).
function steerText(input: AgentInput): string {
  return agentInputText(input);
}
