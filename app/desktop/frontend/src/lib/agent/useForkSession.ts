import type { RunId } from "@/rpc";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient as appQueryClient } from "@/lib/data/queryClient";
import { useSessionStore } from "@/state/sessionStore";

/**
 * Fork a session (whole-history copy, sessions.fork) and jump into the new
 * branch. Forking from a run boundary (`fromRunId`, AUX_API §4.2) lands with
 * backend batch B4 — until then no position is passed here.
 *
 * The fork inherits the source's chat history, so unlike a fresh create it
 * is no draft — it shows in the sidebar immediately.
 */
/** Imperative fork for non-React callers (message context-menu actions).
 *  `fromRunId` = branch up to AND INCLUDING that root run (AUX_API §4.2);
 *  omitted = whole-session copy. Opens the new branch's tab. */
export async function forkSessionAt(id: string, fromRunId?: RunId): Promise<void> {
  try {
    const fork = await getContainer()
      .client()
      .sessions.fork({ sessionId: asSessionId(id), ...(fromRunId ? { fromRunId } : {}) });
    useSessionStore.getState().selectTab(fork.id);
    void appQueryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
  } catch (err) {
    console.error("[session] fork failed:", err);
  }
}

export function useForkSession(): (id: string) => Promise<void> {
  // Stable identity for React callers; the imperative core owns the logic.
  return useCallback((id) => forkSessionAt(id), []);
}
