import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { reportSessionError } from "./reportSessionError";

/** Rename a session (sessions.update title) and refresh the sidebar list.
 *  Empty titles are rejected server-side (invalid_params) — callers trim
 *  and skip no-op submissions before getting here. */
export function useRenameSession(): (id: string, title: string) => Promise<void> {
  return useCallback(async (id, title) => {
    try {
      await getContainer()
        .client()
        .sessions.update({ sessionId: asSessionId(id), title });
      void queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
    } catch (err) {
      reportSessionError("rename", err);
    }
  }, []);
}
