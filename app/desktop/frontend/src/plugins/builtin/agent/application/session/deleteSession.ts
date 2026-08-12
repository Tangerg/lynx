import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";

/**
 * Delete a backend session, close its tab (reselecting a neighbour if it was
 * active), and refetch the session summaries so the row drops. Counterpart to
 * {@link useCreateSession}.
 */
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    try {
      // Session membership drives active/open navigation reconciliation. Keep
      // the query authoritative until Runtime commits the delete; treating a
      // local cache mutation as a server read can move the user even if the
      // command subsequently fails.
      await agentRuntime().deleteSession(id);
      agentSessionState().closeSession(id);
      void invalidateAgentSessions();
    } catch (err) {
      reportSessionError("delete", err);
    }
  }, []);
}
