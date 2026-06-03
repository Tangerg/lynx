import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getContainer } from "@/main/container";
import { useSessionStore } from "@/state/sessionStore";

/**
 * Create a fresh backend session as a hidden **draft**, open it as the
 * active tab, and optionally queue its first message. Returns the new id
 * (or null if the create failed).
 *
 * A draft is a real session (so runs.start works immediately) that stays
 * out of the sidebar list until its first message graduates it — the
 * ChatGPT/Claude/Proma pattern. The "New" button calls this with no text
 * (an empty draft ready to type into); the welcome composer calls it with
 * the typed text, which the chat flushes on remount (useAgentSession).
 */
export function useCreateSession(): (firstMessage?: string) => Promise<string | null> {
  const queryClient = useQueryClient();
  return useCallback(
    async (firstMessage) => {
      try {
        const session = await getContainer().client().sessions.create({});
        const store = useSessionStore.getState();
        // Mark draft + queue the message BEFORE selecting, so the remount
        // useAgentSession triggers sees both already in place.
        store.markDraft(session.id);
        if (firstMessage?.trim()) store.setPendingMessage(session.id, firstMessage);
        store.selectTab(session.id); // opens tab + sets active → remounts chat
        // Draft is filtered out of the sidebar; refetch so its graduation
        // (and any backend-assigned title) lands promptly.
        void queryClient.invalidateQueries({ queryKey: ["sessions"] });
        return session.id;
      } catch (err) {
        console.error("[session] create failed:", err);
        return null;
      }
    },
    [queryClient],
  );
}
