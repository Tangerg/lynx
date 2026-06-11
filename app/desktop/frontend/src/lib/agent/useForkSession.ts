import type { RunId } from "@/rpc";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { useSessionStore } from "@/state/sessionStore";
import { reportSessionError } from "./reportSessionError";

/** Imperative fork for non-React callers (message context-menu actions).
 *  `fromRunId` = branch up to AND INCLUDING that root run (AUX_API §4.2);
 *  omitted = whole-session copy. The fork inherits the source's chat history,
 *  so unlike a fresh create it is no draft — it shows in the sidebar
 *  immediately, and we open its tab. */
export async function forkSessionAt(id: string, fromRunId?: RunId): Promise<void> {
  try {
    const fork = await getContainer()
      .client()
      .sessions.fork({ sessionId: asSessionId(id), ...(fromRunId ? { fromRunId } : {}) });
    useSessionStore.getState().selectTab(fork.id);
    void queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
  } catch (err) {
    reportSessionError("fork", err);
  }
}

export function useForkSession(): (id: string) => Promise<void> {
  // Stable identity for React callers; the imperative core owns the logic.
  return useCallback((id) => forkSessionAt(id), []);
}
