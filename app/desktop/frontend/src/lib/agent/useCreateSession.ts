import type { QueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getContainer } from "@/main/container";
import { queryClient as appQueryClient } from "@/lib/data/queryClient";
import { SESSIONS_KEY } from "@/lib/data/queries";
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
async function createAndOpen(qc: QueryClient, firstMessage?: string): Promise<string | null> {
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
    void qc.invalidateQueries({ queryKey: [SESSIONS_KEY] });
    return session.id;
  } catch (err) {
    console.error("[session] create failed:", err);
    return null;
  }
}

// In-flight latch: every "New" entry point (rail "+", ⌘N, palette command,
// welcome composer) fires bare, and sessions.create is a full round-trip — a
// double-click inside that window would otherwise create two backend sessions
// and two tabs. Re-entrant calls join the pending create instead.
let inflight: Promise<string | null> | null = null;

function doCreate(qc: QueryClient, firstMessage?: string): Promise<string | null> {
  if (inflight) return inflight;
  inflight = createAndOpen(qc, firstMessage).finally(() => {
    inflight = null;
  });
  return inflight;
}

/** Imperative create for non-React callers (palette commands, keymap) — uses
 *  the app's shared QueryClient. React components use {@link useCreateSession}. */
export function createSession(firstMessage?: string): Promise<string | null> {
  return doCreate(appQueryClient, firstMessage);
}

export function useCreateSession(): (firstMessage?: string) => Promise<string | null> {
  const queryClient = useQueryClient();
  return useCallback((firstMessage) => doCreate(queryClient, firstMessage), [queryClient]);
}
