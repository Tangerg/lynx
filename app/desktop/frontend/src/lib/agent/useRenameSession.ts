import { useCallback } from "react";
import type { SidebarSession } from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateSessions, SESSIONS_KEY } from "@/lib/data/queries";
import { reportSessionError } from "./reportSessionError";

/** Rename a session (sessions.update title) and refresh the sidebar list.
 *  Empty titles are rejected server-side (invalid_params) — callers trim
 *  and skip no-op submissions before getting here. */
export function useRenameSession(): (id: string, title: string) => Promise<void> {
  return useCallback(async (id, title) => {
    // Optimistic: paint the new title in the sidebar list right away so the
    // row doesn't flash back to the old title while the RPC + refetch settle.
    // Snapshot first so a failed update rolls back cleanly.
    const prev = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
    queryClient.setQueryData<SidebarSession[]>([SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, title } : s)),
    );
    try {
      await getContainer()
        .client()
        .sessions.update({ sessionId: asSessionId(id), title });
      void invalidateSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([SESSIONS_KEY], prev);
      reportSessionError("rename", err);
    }
  }, []);
}
