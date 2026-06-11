import { useCallback } from "react";
import { useAgentAction, useAgentSlice } from "@/state/agentStore";
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
 *   - a run is already streaming → no-op (one run per session, §6.11; the
 *     Send button shows Stop in that state).
 */
export function useChatSend(): (text: string) => void {
  const createSession = useCreateSession();
  const send = useAgentAction("send");
  const running = useAgentSlice((v) => v.run.running);
  return useCallback(
    (text: string) => {
      if (running) return;
      if (useSessionStore.getState().activeSessionId && send) send(text);
      else void createSession({ firstMessage: text });
    },
    [send, running, createSession],
  );
}
