import type { ContentBlock, RunEvent } from "@/rpc";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asRunId, isErrorType } from "@/rpc";
import { useAgentAction, useAgentRunId, useAgentRunning, useAgentStore } from "@/state/agentStore";
import { LOCAL_STEER_PREFIX } from "@/protocol/run/viewState";
import { useSessionStore } from "@/state/sessionStore";
import { useCreateSession } from "../session/createSession";

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
          const localId = mintSteerBubble(sid, input);
          void getContainer()
            .client()
            .runs.steer(asRunId(runId), text)
            .catch((err) => {
              if (isErrorType(err, "run_not_found")) {
                // Run ended → the steer can't land. Drop the optimistic bubble
                // (else the fallback send would mint a second one and leave this
                // orphaned) and restart as a fresh turn.
                useAgentStore.getState().dropMessage(sid, localId);
                if (send) send(input);
              }
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

// Optimistic steer bubble: render the user's steered message immediately under a
// local-* id (a distinct "steer-" suffix so it can't collide with send()'s own
// local-N counter). The fold reconciles it against the streamed userMessage Item
// by content match (appendUserMessage) once the runtime drains the steer — no
// explicit relabel, since runs.steer returns no item id.
let steerSeq = 0;
function mintSteerBubble(sessionId: string, input: ContentBlock[]): string {
  const id = `${LOCAL_STEER_PREFIX}${++steerSeq}`;
  useAgentStore.getState().applyEvents(sessionId, [
    {
      event: {
        type: "item.completed",
        item: {
          id,
          runId: "",
          status: "completed",
          createdAt: new Date().toISOString(),
          type: "userMessage",
          content: input,
        },
      } as RunEvent["event"],
    },
  ]);
  return id;
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
