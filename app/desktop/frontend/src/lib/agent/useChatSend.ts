import type { ContentBlock } from "@/rpc";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asRunId, errorType, RpcError } from "@/rpc";
import { useAgentAction, useAgentRunId, useAgentRunning } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { useCreateSession } from "./useCreateSession";

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
 *     tool round (true mid-run steer, T2.1). The runtime surfaces it as a user
 *     turn, so it renders like any other message. If the run finished between
 *     typing and sending (run_not_found), it's no longer steerable — fall back
 *     to starting a fresh turn so the message is never lost.
 */
export function useChatSend(): (input: ContentBlock[]) => void {
  const createSession = useCreateSession();
  const send = useAgentAction("send");
  const running = useAgentRunning();
  const runId = useAgentRunId();
  return useCallback(
    (input: ContentBlock[]) => {
      const sid = useSessionStore.getState().activeSessionId;
      if (running && sid && runId) {
        const text = steerText(input);
        // Steering is text-only; an image-only compose mid-run falls through to
        // the normal path (which a busy session rejects — a rare edge).
        if (text) {
          void getContainer()
            .client()
            .runs.steer(asRunId(runId), text)
            .catch((err) => {
              if (isRunNotFound(err) && send) send(input); // run ended → fresh turn
            });
          return;
        }
      }
      if (sid && send) send(input);
      else void createSession({ firstInput: input });
    },
    [send, running, runId, createSession],
  );
}

// steerText flattens the composer's text blocks into one message — steering
// carries text only (images can't ride a steer; see useChatSend).
function steerText(input: ContentBlock[]): string {
  return input
    .filter((b) => b.type === "text")
    .map((b) => ("text" in b ? b.text : ""))
    .join("\n")
    .trim();
}

function isRunNotFound(err: unknown): boolean {
  return err instanceof RpcError && errorType(err.data) === "run_not_found";
}
