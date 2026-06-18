import type { ContentBlock } from "@/rpc";
import { useCallback } from "react";
import { useAgentAction, useAgentRunning } from "@/state/agentStore";
import { useQueueStore } from "@/state/queueStore";
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
 *   - a run is already streaming → QUEUE the message (queueStore); it's
 *     auto-sent when the run settles cleanly (T2.1). The Send button shows Stop
 *     in that state, so this is the textarea Enter path.
 */
export function useChatSend(): (input: ContentBlock[]) => void {
  const createSession = useCreateSession();
  const send = useAgentAction("send");
  const running = useAgentRunning();
  return useCallback(
    (input: ContentBlock[]) => {
      const sid = useSessionStore.getState().activeSessionId;
      if (running) {
        if (sid) useQueueStore.getState().enqueue(sid, input);
        return;
      }
      if (sid && send) send(input);
      else void createSession({ firstInput: input });
    },
    [send, running, createSession],
  );
}
